package main

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// TestHeldRequests_Acquire_RefusesAtTheLimitAndGivesTheSlotBack covers the
// counter on its own: it admits up to its limit, refuses the next, and admits
// again once a slot is given back.
func TestHeldRequests_Acquire_RefusesAtTheLimitAndGivesTheSlotBack(t *testing.T) {
	t.Parallel()
	held := &heldRequests{limit: 2}

	first, second := held.acquire(), held.acquire()
	if !first || !second {
		t.Fatalf("the first two acquires = %v, %v under a limit of two, want both admitted", first, second)
	}
	if held.acquire() {
		t.Fatal("a third acquire was admitted under a limit of two")
	}
	if got := held.open.Load(); got != 2 {
		t.Errorf("open = %d after a refused acquire, want 2: a refusal must not keep a slot", got)
	}
	held.release()
	if got := held.open.Load(); got != 1 {
		t.Errorf("open = %d after one release, want 1", got)
	}
	if !held.acquire() {
		t.Error("an acquire was refused after a slot was given back")
	}
}

// TestHeldRequests_Acquire_NeverAdmitsMoreThanTheLimit races many acquires at
// once against a small limit, with nothing given back, and holds the count to
// exactly the limit: a counter that went over and came back would, under the
// same race, refuse a request for a slot that was never used, and one that
// forgot to come back would admit more than it may.
func TestHeldRequests_Acquire_NeverAdmitsMoreThanTheLimit(t *testing.T) {
	t.Parallel()
	const limit, callers = 10, 64
	held := &heldRequests{limit: limit}

	var admitted atomic.Int64
	var start sync.WaitGroup
	start.Add(1)
	var done sync.WaitGroup
	for range callers {
		done.Go(func() {
			start.Wait()
			if held.acquire() {
				admitted.Add(1)
			}
		})
	}
	start.Done()
	done.Wait()

	if got := admitted.Load(); got != limit {
		t.Errorf("%d acquires admitted, want exactly %d", got, limit)
	}
	if got := held.open.Load(); got != limit {
		t.Errorf("open = %d, want exactly %d", got, limit)
	}
}

// TestHeldRequests_Nil_AdmitsEverything pins the one counter that counts
// nothing, which is what a gate built without one holds: the tests build such
// gates, and the server never does.
func TestHeldRequests_Nil_AdmitsEverything(t *testing.T) {
	t.Parallel()
	var held *heldRequests
	for range 3 {
		if !held.acquire() {
			t.Fatal("a nil counter refused a request")
		}
	}
	held.release()
}

// TestProcessHeldRequests_IsTheRegisterCeiling pins the counter every gate of
// the process shares to the register's value, the one row HLD-011 declares.
func TestProcessHeldRequests_IsTheRegisterCeiling(t *testing.T) {
	t.Parallel()
	if processHeldRequests.limit != tenancy.HeldRequestsPerProcess {
		t.Errorf("processHeldRequests.limit = %d, want the register's %d", processHeldRequests.limit,
			tenancy.HeldRequestsPerProcess)
	}
}

// TestHoldsRequest_CountsEveryPOSTButAListenTheSDKHoldsToItsHeader covers
// which requests take a slot: every POST, except one naming subscriptions/listen
// on a revision whose Mcp-Method header the SDK holds to the body, since the
// listen ceilings count that one already. A listen on an older revision is
// counted, because its header proves nothing there, and so is a POST naming
// any other method.
func TestHoldsRequest_CountsEveryPOSTButAListenTheSDKHoldsToItsHeader(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, method, version, mcpMethod string
		want                             bool
	}{
		{name: "a POST with no headers", method: http.MethodPost, want: true},
		{name: "a modern tools/call", method: http.MethodPost, version: "2026-07-28", mcpMethod: "tools/call", want: true},
		{name: "a modern listen", method: http.MethodPost, version: "2026-07-28", mcpMethod: methodSubscriptionsListen},
		{name: "a listen on a later revision", method: http.MethodPost, version: "2027-01-01", mcpMethod: methodSubscriptionsListen},
		{
			name: "a listen claimed on an older revision", method: http.MethodPost,
			version: "2025-11-25", mcpMethod: methodSubscriptionsListen, want: true,
		},
		{name: "a listen with no revision named", method: http.MethodPost, mcpMethod: methodSubscriptionsListen, want: true},
		{name: "a stateful GET", method: http.MethodGet, version: "2025-11-25"},
		{name: "a stateful DELETE", method: http.MethodDelete, version: "2025-11-25"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequestWithContext(t.Context(), tc.method, "/mcp", http.NoBody)
			if tc.version != "" {
				req.Header.Set("MCP-Protocol-Version", tc.version)
			}
			if tc.mcpMethod != "" {
				req.Header.Set("Mcp-Method", tc.mcpMethod)
			}
			if got := holdsRequest(req); got != tc.want {
				t.Errorf("holdsRequest = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestHeldRequestsFailure_IsAGate503ThatSaysToRetry pins the refusal's wire
// shape field by field: the status, the code mirroring it, the words the
// register declares, and the register's fixed Retry-After.
func TestHeldRequestsFailure_IsAGate503ThatSaysToRetry(t *testing.T) {
	t.Parallel()
	failure := heldRequestsFailure()
	if failure.status != http.StatusServiceUnavailable || failure.code != tenancy.CodeUnavailable {
		t.Errorf("status %d, code %d; want 503 and %d", failure.status, failure.code, tenancy.CodeUnavailable)
	}
	want := strconv.Itoa(int(tenancy.UpstreamRetryAfter.Seconds()))
	if got := failure.header.Get(headerRetryAfter); got != want {
		t.Errorf("Retry-After = %q, want %q", got, want)
	}
	if !strings.HasPrefix(failure.message, "This server is holding as many requests as it serves at once.") ||
		!strings.Contains(failure.message, "Retry later") {
		t.Errorf("message = %q, want the register's words and the next action", failure.message)
	}
}

// heldGate is a gate against a stand-in instance whose held-request count is
// its own, with room for one request, and a handler behind it that holds each
// request it is handed until the gate is opened.
type heldGate struct {
	handler http.Handler
	held    *heldRequests
	reached chan struct{}
	release chan struct{}
	opened  sync.Once
}

// open lets every held request through, once however often it is called. The
// test's cleanup calls it too, so a test that stops early leaves no request
// held behind it.
func (g *heldGate) open() { g.opened.Do(func() { close(g.release) }) }

// newHeldGate builds the gate and the holding handler.
func newHeldGate(t *testing.T) *heldGate {
	t.Helper()
	g := &heldGate{
		held:    &heldRequests{limit: 1},
		reached: make(chan struct{}, 8),
		release: make(chan struct{}),
	}
	t.Cleanup(g.open)
	gate := newGate(t, okFactory)
	gate.held = g.held
	g.handler = gate.middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		g.reached <- struct{}{}
		<-g.release
		w.WriteHeader(http.StatusOK)
	}))
	return g
}

// heldPost is an authenticated POST carrying a JSON-RPC request with an id,
// as a server receives it.
func heldPost(t *testing.T, id, mcpMethod string) *http.Request {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", strings.NewReader(heldBody(id, mcpMethod)))
	setHeldHeaders(req, mcpMethod)
	return req
}

// heldBody is the JSON-RPC request a held POST carries.
func heldBody(id, mcpMethod string) string {
	return `{"jsonrpc":"2.0","id":` + id + `,"method":"` + mcpMethod + `","params":{}}`
}

// setHeldHeaders sets what a 2026-07-28 client sends with a request.
func setHeldHeaders(req *http.Request, mcpMethod string) {
	req.Header.Set("PRIVATE-TOKEN", gateTestToken)
	req.Header.Set("MCP-Protocol-Version", "2026-07-28")
	req.Header.Set("Mcp-Method", mcpMethod)
}

// TestMcpServerGate_Middleware_RefusesARequestPastTheHeldCeiling drives the
// gate the way a caller meets the ceiling: one request holds the only slot, the
// next is refused in the gate with a 503 that echoes its id and carries
// Retry-After, before the handler behind the gate sees it, and once the first
// request ends the slot is free again. The operator is told which bound
// refused, in a line naming its scope and figure.
//
// Not parallel: it replaces the process-wide default logger.
func TestMcpServerGate_Middleware_RefusesARequestPastTheHeldCeiling(t *testing.T) {
	var logged bytes.Buffer
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, nil)))
	forgetRefusalLines()

	g := newHeldGate(t)
	first, firstReq := httptest.NewRecorder(), heldPost(t, "1", "tools/call")
	var firstDone sync.WaitGroup
	firstDone.Go(func() { g.handler.ServeHTTP(first, firstReq) })
	select {
	case <-g.reached:
	case <-time.After(10 * time.Second):
		t.Fatal("the first request never reached the handler")
	}

	// Served on a goroutine of its own and waited for with a deadline: a gate
	// that admitted it would park it in the handler behind the gate, which
	// holds every request until it is opened, and the test would hang instead
	// of failing.
	refused, refusedReq := httptest.NewRecorder(), heldPost(t, "2", "tools/call")
	refusedDone := make(chan struct{})
	go func() {
		defer close(refusedDone)
		g.handler.ServeHTTP(refused, refusedReq)
	}()
	select {
	case <-refusedDone:
	case <-g.reached:
		g.open()
		<-refusedDone
		t.Fatal("the request past the ceiling reached the handler behind the gate")
	case <-time.After(10 * time.Second):
		g.open()
		<-refusedDone
		t.Fatal("the request past the ceiling was neither refused nor served")
	}
	if refused.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d while the only slot is held, want 503: %s", refused.Code, refused.Body.String())
	}
	if got := refused.Header().Get(headerRetryAfter); got != strconv.Itoa(int(upstreamRetryAfter.Seconds())) {
		t.Errorf("Retry-After = %q, want the register's fixed pause", got)
	}
	decoded := decodeJSONRPCError(t, refused.Body.String(), "2")
	if decoded.Error.Code != errCodeUpstreamUnavailable {
		t.Errorf("code = %d, want %d", decoded.Error.Code, errCodeUpstreamUnavailable)
	}
	select {
	case <-g.reached:
		t.Error("the refused request reached the handler behind the gate")
	default:
	}
	line := logged.String()
	if !strings.Contains(line, "too many requests held across the process") || !strings.Contains(line, `"scope":"process"`) ||
		!strings.Contains(line, `"limit_held_requests":`+strconv.Itoa(maxHeldRequestsPerProcess)) {
		t.Errorf("the refusal left no line naming its scope and figure: %s", line)
	}

	g.open()
	firstDone.Wait()
	if first.Code != http.StatusOK {
		t.Errorf("the held request ended with %d, want the handler's 200", first.Code)
	}
	if got := g.held.open.Load(); got != 0 {
		t.Fatalf("open = %d after the held request ended, want its slot given back", got)
	}
	again := httptest.NewRecorder()
	g.handler.ServeHTTP(again, heldPost(t, "3", "tools/call"))
	if again.Code != http.StatusOK {
		t.Errorf("status = %d once the slot was given back, want the handler's 200", again.Code)
	}
}

// TestMcpServerGate_Middleware_LeavesAListenToItsOwnCeiling holds the other
// half of which requests count: with the only slot held, a subscriptions/listen
// on the revision whose header the SDK holds to the body still reaches the
// handler, since the listen ceilings count it, and takes no slot.
func TestMcpServerGate_Middleware_LeavesAListenToItsOwnCeiling(t *testing.T) {
	t.Parallel()
	g := newHeldGate(t)
	if !g.held.acquire() {
		t.Fatal("the only slot could not be taken")
	}
	g.open()

	listen := httptest.NewRecorder()
	g.handler.ServeHTTP(listen, heldPost(t, "1", methodSubscriptionsListen))
	if listen.Code != http.StatusOK {
		t.Errorf("a listen was answered %d with every slot held, want it to reach the handler: %s",
			listen.Code, listen.Body.String())
	}
	if got := g.held.open.Load(); got != 1 {
		t.Errorf("open = %d after a listen, want the one slot taken before it and no other", got)
	}
}

// TestMcpServerGate_Middleware_RefusalBeforeAdmissionTakesNoSlot holds where
// the slot is taken: after the credential is admitted. A request refused for
// its credential never reaches the count, so a caller with no credential
// cannot hold a slot while it waits to be refused.
func TestMcpServerGate_Middleware_RefusalBeforeAdmissionTakesNoSlot(t *testing.T) {
	t.Parallel()
	g := newHeldGate(t)
	g.open()

	req := heldPost(t, "1", "tools/call")
	req.Header.Del("PRIVATE-TOKEN")
	refused := httptest.NewRecorder()
	g.handler.ServeHTTP(refused, req)
	if refused.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d without a credential, want 401: %s", refused.Code, refused.Body.String())
	}
	if got := g.held.open.Load(); got != 0 {
		t.Errorf("open = %d after a refusal before admission, want no slot taken", got)
	}
}

// TestRegisterLegacyMCPHandlers_RefusesPastTheProcessHeldCeiling holds the
// wiring: the gate the legacy handlers mount counts against the one count the
// process shares, so with that count full an admitted request is refused in
// the gate with the ceiling's 503.
//
// Not parallel: it fills the process-wide count.
func TestRegisterLegacyMCPHandlers_RefusesPastTheProcessHeldCeiling(t *testing.T) {
	gitlab := gateStubGitLab(t, false)
	cfg := &config.Config{
		GitLabURL: gitlab, Tier: edition.Free, TierExplicit: true, IgnoreScopes: true, Stateless: true,
	}
	mux := http.NewServeMux()
	registerLegacyMCPHandlers(t.Context(), cfg, newGateTestPool(t, okFactory, gitlab),
		poolBinding{credentials: &credentialStates{}, sessions: newSessionOwners(false)}, mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	previous := processHeldRequests.open.Swap(processHeldRequests.limit)
	t.Cleanup(func() { processHeldRequests.open.Store(previous) })

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/mcp",
		strings.NewReader(heldBody("7", "tools/call")))
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	setHeldHeaders(req, "tools/call")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading the response: %v", err)
	}
	if resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(string(body), "holding as many requests") {
		t.Errorf("status %d, body %s; want the held ceiling's 503", resp.StatusCode, body)
	}
}
