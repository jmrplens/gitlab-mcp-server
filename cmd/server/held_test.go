package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// TestHeldRequestsFor_SizesTheCeilingFromTheDescriptorLimit pins the
// derivation at the limits a deployment meets: an eighth of the limit spare,
// the listen streams reserved at their process ceiling, and two descriptors a
// held call. A hard limit of 1024 gives the 192 the measurement was taken
// under; the limits a default systemd host or container gives leave the
// ceiling far above what a rate-limited caller reaches; and a limit too small
// to leave room for one call after the reservation still serves one at a time.
func TestHeldRequestsFor_SizesTheCeilingFromTheDescriptorLimit(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		descriptors uint64
		want        int64
	}{
		{name: "no descriptors at all", descriptors: 0, want: 1},
		{name: "less than the listen reservation", descriptors: 512, want: 1},
		{name: "exactly the reservation", descriptors: 585, want: 1},
		{name: "one call past the reservation", descriptors: 587, want: 1},
		{name: "two calls past the reservation", descriptors: 589, want: 2},
		{name: "a hard limit of 1024", descriptors: 1024, want: 192},
		{name: "a hard limit of 4096", descriptors: 4096, want: 1536},
		{name: "the systemd default hard limit", descriptors: 524288, want: 229120},
		{name: "a container's usual hard limit", descriptors: 1048576, want: 458496},
		{name: "the largest limit there is", descriptors: ^uint64(0), want: 8070450532247928576},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := heldRequestsFor(tc.descriptors); got != tc.want {
				t.Errorf("heldRequestsFor(%d) = %d, want %d", tc.descriptors, got, tc.want)
			}
		})
	}
}

// TestHeldRequestsCeiling_ReadsTheLimitOrFallsBack covers the two answers a
// platform gives: the limit it reads is the one sized from, and a platform
// with none to read is sized against the register's fallback, which gives the
// same 192 a 1024 limit does.
func TestHeldRequestsCeiling_ReadsTheLimitOrFallsBack(t *testing.T) {
	t.Parallel()
	read := func() (uint64, bool) { return 4096, true }
	if got := heldRequestsCeiling(read); got != 1536 {
		t.Errorf("ceiling under a limit of 4096 = %d, want 1536", got)
	}
	unread := func() (uint64, bool) { return 4096, false }
	if got, want := heldRequestsCeiling(unread), heldRequestsFor(tenancy.FallbackDescriptorLimit); got != want || got != 192 {
		t.Errorf("ceiling with no limit to read = %d, want the fallback's %d (192)", got, want)
	}
}

// TestProcessHeldRequests_IsSizedFromThisProcessLimit pins the count every
// server and gate shares to the ceiling this process's own limit gives.
func TestProcessHeldRequests_IsSizedFromThisProcessLimit(t *testing.T) {
	t.Parallel()
	if want := heldRequestsCeiling(descriptorLimit); processHeldRequests.limit != want {
		t.Errorf("processHeldRequests.limit = %d, want %d from this process's descriptor limit",
			processHeldRequests.limit, want)
	}
}

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

// TestHeldRequests_Acquire_UnderChurn_NeverHoldsMoreThanTheLimit races callers
// that take and give back slots over and over, which is what a busy process
// does, and holds two things at every step: no more calls hold a slot at once
// than the limit, and every slot taken is given back, so the count ends at
// zero. The churn is also what makes one caller's compare-and-swap lose to
// another's and retry, the branch a race of single acquires rarely reaches;
// the assertions do not depend on reaching it.
func TestHeldRequests_Acquire_UnderChurn_NeverHoldsMoreThanTheLimit(t *testing.T) {
	t.Parallel()
	const limit, callers, rounds = 8, 32, 2000
	held := &heldRequests{limit: limit}

	var inside, most, admitted atomic.Int64
	var start sync.WaitGroup
	start.Add(1)
	var done sync.WaitGroup
	for range callers {
		done.Go(func() {
			start.Wait()
			for range rounds {
				if !held.acquire() {
					continue
				}
				admitted.Add(1)
				raiseTo(&most, inside.Add(1))
				inside.Add(-1)
				held.release()
			}
		})
	}
	start.Done()
	done.Wait()

	if got := most.Load(); got > limit {
		t.Errorf("%d calls held a slot at once, want at most %d", got, limit)
	}
	if admitted.Load() == 0 {
		t.Error("no acquire was admitted under churn")
	}
	if got := held.open.Load(); got != 0 {
		t.Errorf("open = %d once every slot was given back, want 0", got)
	}
}

// raiseTo records v in peak when it is the highest value seen so far.
func raiseTo(peak *atomic.Int64, v int64) {
	for {
		seen := peak.Load()
		if v <= seen || peak.CompareAndSwap(seen, v) {
			return
		}
	}
}

// TestHoldsOpen_CountsWhatReachesGitLabButAListen covers which methods the
// ceiling counts: every method MeterFor charges to the tool-call or the
// completion bucket, since those reach GitLab and can wait on it, except a
// subscriptions/listen, which the listen ceilings count; nothing answered from
// memory, and no notification.
func TestHoldsOpen_CountsWhatReachesGitLabButAListen(t *testing.T) {
	t.Parallel()
	cases := []struct {
		method string
		want   bool
	}{
		{method: "tools/call", want: true},
		{method: "resources/read", want: true},
		{method: "resources/subscribe", want: true},
		{method: "prompts/get", want: true},
		{method: "completion/complete", want: true},
		{method: methodSubscriptionsListen},
		{method: "tools/list"},
		{method: "initialize"},
		{method: "ping"},
		{method: "notifications/cancelled"},
		{method: ""},
	}
	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			t.Parallel()
			if got := holdsOpen(tc.method); got != tc.want {
				t.Errorf("holdsOpen(%q) = %v, want %v", tc.method, got, tc.want)
			}
		})
	}
}

// TestGateCountsRequest_OnlyWhereTheHeaderIsTheMethod covers which POSTs the
// gate takes a slot for itself: one naming a counted method on a revision
// whose Mcp-Method header the SDK holds to the body, and nothing else. Before
// that revision a POST can carry a batch, a response or a notification
// whatever its headers say, so the calls on it are left to the count where
// the SDK dispatches them.
func TestGateCountsRequest_OnlyWhereTheHeaderIsTheMethod(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, method, version, mcpMethod string
		want                             bool
	}{
		{name: "a modern tools/call", method: http.MethodPost, version: "2026-07-28", mcpMethod: "tools/call", want: true},
		{name: "a later revision's read", method: http.MethodPost, version: "2027-01-01", mcpMethod: "resources/read", want: true},
		{name: "a modern listen", method: http.MethodPost, version: "2026-07-28", mcpMethod: methodSubscriptionsListen},
		{name: "a modern notification", method: http.MethodPost, version: "2026-07-28", mcpMethod: "notifications/cancelled"},
		{name: "a modern listing", method: http.MethodPost, version: "2026-07-28", mcpMethod: "tools/list"},
		{name: "a modern POST naming no method", method: http.MethodPost, version: "2026-07-28"},
		{name: "a call on the revision before", method: http.MethodPost, version: "2025-11-25", mcpMethod: "tools/call"},
		{name: "a call naming no revision", method: http.MethodPost, mcpMethod: "tools/call"},
		{name: "a stateful GET", method: http.MethodGet, version: "2026-07-28", mcpMethod: "tools/call"},
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
			if got := gateCountsRequest(req); got != tc.want {
				t.Errorf("gateCountsRequest = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestClaimGateSlot_HandsTheGateSlotToOneCall covers the handover: a POST the
// gate took a slot for gives it to its first call and to no other, and a POST
// with no such slot, or none at all, gives nothing.
func TestClaimGateSlot_HandsTheGateSlotToOneCall(t *testing.T) {
	t.Parallel()
	if claimGateSlot("a token no POST registered") {
		t.Error("a call whose POST is gone claimed a slot")
	}
	if claimGateSlot(registeredCarrier(t, t.Context())) {
		t.Error("a call on a POST the gate took no slot for claimed one")
	}
	slotted := registeredCarrier(t, context.WithValue(t.Context(), gateHeldSlotKey{}, &gateHeldSlot{}))
	if !claimGateSlot(slotted) {
		t.Error("the first call on a POST the gate took a slot for did not claim it")
	}
	if claimGateSlot(slotted) {
		t.Error("a second call on the same POST claimed the slot the first one holds")
	}
}

// registeredCarrier registers carrier as a POST's context under a fresh
// carrier token for the test's lifetime, and returns the token.
func registeredCarrier(t *testing.T, carrier context.Context) string {
	t.Helper()
	token := rand.Text()
	mcpCarriers.contexts.Store(token, carrier)
	t.Cleanup(func() { mcpCarriers.contexts.Delete(token) })
	return token
}

// carriedExtra is what the SDK hands a request that arrived on a POST the
// carrier registry stamped, registered under a fresh token for the test's
// lifetime with carrier as the POST's context.
func carriedExtra(t *testing.T, carrier context.Context) *mcp.RequestExtra {
	t.Helper()
	return &mcp.RequestExtra{Header: http.Header{carrierHeader: {registeredCarrier(t, carrier)}}}
}

// heldMiddlewareRun drives the middleware over one request and reports what
// the handler behind it saw: whether it ran, and how many slots were open
// while it did.
type heldMiddlewareRun struct {
	result      mcp.Result
	err         error
	reached     bool
	openInsider int64
}

// runHeldMiddleware passes req through the middleware counting on held, with a
// handler that records the count it runs under.
func runHeldMiddleware(held *heldRequests, method string, req mcp.Request) heldMiddlewareRun {
	var run heldMiddlewareRun
	next := func(context.Context, string, mcp.Request) (mcp.Result, error) {
		run.reached = true
		run.openInsider = held.open.Load()
		return &mcp.CallToolResult{}, nil
	}
	run.result, run.err = heldRequestsMiddleware(held)(next)(context.Background(), method, req)
	return run
}

// TestHeldRequestsMiddleware_CountsACarriedCallWhileItRuns covers the count
// itself: a call that arrived on a POST takes a slot for as long as its
// handler runs and gives it back when the handler returns.
func TestHeldRequestsMiddleware_CountsACarriedCallWhileItRuns(t *testing.T) {
	t.Parallel()
	held := &heldRequests{limit: 2}
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "t"}, Extra: carriedExtra(t, t.Context())}

	run := runHeldMiddleware(held, "tools/call", req)
	if !run.reached || run.err != nil {
		t.Fatalf("the call was not served: reached %v, error %v", run.reached, run.err)
	}
	if run.openInsider != 1 {
		t.Errorf("open = %d while the call ran, want its one slot", run.openInsider)
	}
	if got := held.open.Load(); got != 0 {
		t.Errorf("open = %d after the call returned, want its slot given back", got)
	}
}

// TestHeldRequestsMiddleware_LeavesWhatItDoesNotCount covers every request
// that passes untouched, with a full count to show none of them was asked: a
// request that arrived on no POST (stdio, an in-memory client), a method that
// answers from memory, a notification, and a listen.
func TestHeldRequestsMiddleware_LeavesWhatItDoesNotCount(t *testing.T) {
	t.Parallel()
	carried := carriedExtra(t, t.Context())
	cases := []struct {
		name, method string
		req          mcp.Request
	}{
		{name: "a call on no POST", method: "tools/call", req: &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "t"}}},
		{name: "a call with no header", method: "tools/call", req: &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{Name: "t"}, Extra: &mcp.RequestExtra{},
		}},
		{name: "a listing", method: "tools/list", req: &mcp.ListToolsRequest{Params: &mcp.ListToolsParams{}, Extra: carried}},
		{name: "a notification", method: "notifications/cancelled", req: &mcp.CallToolRequest{Extra: carried}},
		{name: "a listen", method: methodSubscriptionsListen, req: &mcp.CallToolRequest{Extra: carried}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			held := &heldRequests{limit: 1}
			held.open.Store(1)
			run := runHeldMiddleware(held, tc.method, tc.req)
			if !run.reached || run.err != nil {
				t.Errorf("refused with every slot taken: reached %v, error %v", run.reached, run.err)
			}
			if got := held.open.Load(); got != 1 {
				t.Errorf("open = %d, want the count untouched", got)
			}
		})
	}
}

// TestHeldRequestsMiddleware_ClaimsTheSlotTheGateTook covers the handover on
// the path where the gate already counted the POST: with the count full, the
// first call on that POST runs under the gate's slot rather than asking for a
// second one it would be refused, and a second call on the same POST, which
// only a batch the SDK let through could carry, is counted on its own.
func TestHeldRequestsMiddleware_ClaimsTheSlotTheGateTook(t *testing.T) {
	t.Parallel()
	held := &heldRequests{limit: 1}
	held.open.Store(1)
	carrier := context.WithValue(t.Context(), gateHeldSlotKey{}, &gateHeldSlot{})
	extra := carriedExtra(t, carrier)

	first := runHeldMiddleware(held, "tools/call", &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "t"}, Extra: extra})
	if !first.reached {
		t.Fatal("the call the gate counted was refused a second slot")
	}
	second := runHeldMiddleware(held, "tools/call", &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "t"}, Extra: extra})
	if second.reached {
		t.Error("a second call on the POST ran under the slot the first one claimed")
	}
	if got := held.open.Load(); got != 1 {
		t.Errorf("open = %d, want the gate's one slot and no other", got)
	}
}

// TestHeldRequestsMiddleware_RefusesEachMethodTheWayItsBucketDoes covers the
// refusals with every slot taken: a tools/call as a result flagged with
// isError that the model reads, a completion as an empty completion, and any
// other counted method as a JSON-RPC error with the in-band retry-later code.
// Every one of them says only that the server is busy, the handler behind the
// middleware never runs, and the count is left as it was.
func TestHeldRequestsMiddleware_RefusesEachMethodTheWayItsBucketDoes(t *testing.T) {
	t.Parallel()
	extra := carriedExtra(t, t.Context())
	cases := []struct {
		name, method string
		req          mcp.Request
		check        func(t *testing.T, result mcp.Result, err error)
	}{
		{
			name: "tools/call", method: "tools/call",
			req: &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "t"}, Extra: extra},
			check: func(t *testing.T, result mcp.Result, err error) {
				t.Helper()
				tool, ok := result.(*mcp.CallToolResult)
				if err != nil || !ok || !tool.IsError || len(tool.Content) != 1 {
					t.Fatalf("result %#v, error %v; want one result flagged with isError", result, err)
				}
				if text, _ := tool.Content[0].(*mcp.TextContent); text == nil || text.Text != heldRefusalText {
					t.Errorf("content %#v, want %q", tool.Content[0], heldRefusalText)
				}
			},
		},
		{
			name: "resources/read", method: "resources/read",
			req: &mcp.ReadResourceRequest{Params: &mcp.ReadResourceParams{URI: "gitlab://project/1"}, Extra: extra},
			check: func(t *testing.T, result mcp.Result, err error) {
				t.Helper()
				var rpc *jsonrpc.Error
				if result != nil || !errors.As(err, &rpc) || rpc.Code != tenancy.CodeTooManyRequests || rpc.Message != heldRefusalText {
					t.Errorf("result %#v, error %v; want a JSON-RPC error %d saying %q", result, err,
						tenancy.CodeTooManyRequests, heldRefusalText)
				}
			},
		},
		{
			name: "completion/complete", method: "completion/complete",
			req: &mcp.CompleteRequest{Params: &mcp.CompleteParams{}, Extra: extra},
			check: func(t *testing.T, result mcp.Result, err error) {
				t.Helper()
				complete, ok := result.(*mcp.CompleteResult)
				if err != nil || !ok || complete.Completion.Values == nil || len(complete.Completion.Values) != 0 {
					t.Errorf("result %#v, error %v; want an empty completion", result, err)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			held := &heldRequests{limit: 1}
			held.open.Store(1)
			run := runHeldMiddleware(held, tc.method, tc.req)
			if run.reached {
				t.Fatal("the handler ran with every slot taken")
			}
			tc.check(t, run.result, run.err)
			if got := held.open.Load(); got != 1 {
				t.Errorf("open = %d after a refusal, want the count untouched", got)
			}
		})
	}
}

// TestHeldRequestsRefusal_NamesTheBoundOnlyInTheLog covers the operator's
// line an in-band refusal leaves: the scope and the figure, which the caller's
// words leave out.
//
// Not parallel: it replaces the process-wide default logger.
func TestHeldRequestsRefusal_NamesTheBoundOnlyInTheLog(t *testing.T) {
	var logged bytes.Buffer
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, nil)))
	forgetRefusalLines()

	if _, err := heldRequestsRefusal(t.Context(), "prompts/get", 7); err == nil || strings.Contains(err.Error(), "7") {
		t.Errorf("error %v; want the busy words with no figure", err)
	}
	line := logged.String()
	if !strings.Contains(line, "too many requests held across the process") || !strings.Contains(line, `"scope":"process"`) ||
		!strings.Contains(line, `"limit_held_requests":7`) {
		t.Errorf("the refusal left no line naming its scope and figure: %s", line)
	}
}

// TestHeldRequestsFailure_IsAGate503ThatSaysToRetryAndCloses pins the gate's
// refusal field by field: the status, the code mirroring it, the busy words,
// the register's fixed Retry-After, and a closed connection, so a refused
// caller does not keep a descriptor of the limit the ceiling protects.
func TestHeldRequestsFailure_IsAGate503ThatSaysToRetryAndCloses(t *testing.T) {
	t.Parallel()
	failure := processBusyFailure()
	if failure.status != http.StatusServiceUnavailable || failure.code != tenancy.CodeUnavailable {
		t.Errorf("status %d, code %d; want 503 and %d", failure.status, failure.code, tenancy.CodeUnavailable)
	}
	if failure.message != heldRefusalText || heldRefusalText != "This server is busy. Retry later." {
		t.Errorf("message = %q, want words that name no bound and say to retry", failure.message)
	}
	want := strconv.Itoa(int(tenancy.UpstreamRetryAfter.Seconds()))
	if got := failure.header.Get(headerRetryAfter); got != want {
		t.Errorf("Retry-After = %q, want %q", got, want)
	}
	if got := failure.header.Get("Connection"); got != "close" {
		t.Errorf("Connection = %q, want close", got)
	}
}

// heldGate is a gate against a stand-in instance whose held-request count is
// its own, with room for one request, and a handler behind it that holds each
// request it is handed until the gate is opened, recording whether the gate
// handed it a slot of its own.
type heldGate struct {
	handler http.Handler
	held    *heldRequests
	reached chan bool
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
		reached: make(chan bool, 8),
		release: make(chan struct{}),
	}
	t.Cleanup(g.open)
	gate := newGate(t, okFactory)
	gate.held = g.held
	g.handler = gate.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, slotted := r.Context().Value(gateHeldSlotKey{}).(*gateHeldSlot)
		g.reached <- slotted
		<-g.release
		w.WriteHeader(http.StatusOK)
	}))
	return g
}

// heldPost is an authenticated POST carrying a JSON-RPC request with an id,
// with the headers a client of version sends.
func heldPost(t *testing.T, id, mcpMethod, version string) *http.Request {
	t.Helper()
	body := `{"jsonrpc":"2.0","id":` + id + `,"method":"` + mcpMethod + `","params":{}}`
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("PRIVATE-TOKEN", gateTestToken)
	req.Header.Set("MCP-Protocol-Version", version)
	req.Header.Set("Mcp-Method", mcpMethod)
	return req
}

// TestMcpServerGate_Middleware_RefusesAModernCallPastTheHeldCeiling drives the
// gate the way a modern caller meets the ceiling: one call holds the only
// slot and reaches the handler with the gate's slot on its context, the next
// is refused in the gate with a 503 that echoes its id, carries Retry-After and
// closes the connection, before the handler sees it, and once the first call
// ends the slot is free again. The operator is told which bound refused, in a
// line naming its scope and figure.
//
// Not parallel: it replaces the process-wide default logger.
func TestMcpServerGate_Middleware_RefusesAModernCallPastTheHeldCeiling(t *testing.T) {
	var logged bytes.Buffer
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, nil)))
	forgetRefusalLines()

	g := newHeldGate(t)
	first, firstReq := httptest.NewRecorder(), heldPost(t, "1", "tools/call", protocolVersionStandardHeaders)
	var firstDone sync.WaitGroup
	firstDone.Go(func() { g.handler.ServeHTTP(first, firstReq) })
	select {
	case slotted := <-g.reached:
		if !slotted {
			t.Error("the call the gate counted reached the handler without the gate's slot on its context")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the first request never reached the handler")
	}

	// Served on a goroutine of its own and waited for with a deadline: a gate
	// that admitted it would park it in the handler behind the gate, which
	// holds every request until it is opened, and the test would hang instead
	// of failing.
	refused, refusedReq := httptest.NewRecorder(), heldPost(t, "2", "tools/call", protocolVersionStandardHeaders)
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
	if got := refused.Header().Get("Connection"); got != "close" {
		t.Errorf("Connection = %q, want the refused caller's connection closed", got)
	}
	decoded := decodeJSONRPCError(t, refused.Body.String(), "2")
	if decoded.Error.Code != errCodeUpstreamUnavailable || decoded.Error.Message != heldRefusalText {
		t.Errorf("code %d, message %q; want %d and %q", decoded.Error.Code, decoded.Error.Message,
			errCodeUpstreamUnavailable, heldRefusalText)
	}
	line := logged.String()
	if !strings.Contains(line, "too many requests held across the process") || !strings.Contains(line, `"scope":"process"`) ||
		!strings.Contains(line, `"limit_held_requests":1`) {
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
	g.handler.ServeHTTP(again, heldPost(t, "3", "tools/call", protocolVersionStandardHeaders))
	if again.Code != http.StatusOK {
		t.Errorf("status = %d once the slot was given back, want the handler's 200", again.Code)
	}
}

// TestMcpServerGate_Middleware_LeavesToTheSDKWhatItCannotReadAsACall holds the
// other half of what the gate counts: with the only slot held, a modern listen,
// a modern notification and a POST on an older revision, which may carry a
// batch, a response or a notification whatever its headers say, all reach the
// handler with no slot of the gate's, and the count is left alone. The calls
// among them are counted where the SDK dispatches them.
func TestMcpServerGate_Middleware_LeavesToTheSDKWhatItCannotReadAsACall(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, method, version string
	}{
		{name: "a modern listen", method: methodSubscriptionsListen, version: protocolVersionStandardHeaders},
		{name: "a modern notification", method: "notifications/cancelled", version: protocolVersionStandardHeaders},
		{name: "a call on an older revision", method: "tools/call", version: "2025-11-25"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := newHeldGate(t)
			g.held.open.Store(1)
			g.open()

			rec := httptest.NewRecorder()
			g.handler.ServeHTTP(rec, heldPost(t, "1", tc.method, tc.version))
			if rec.Code != http.StatusOK {
				t.Errorf("answered %d with every slot held, want it to reach the handler: %s", rec.Code, rec.Body.String())
			}
			// ServeHTTP has returned, so a handler that ran has already
			// recorded it: waiting for a record that is not there would hang
			// the test on a gate that refused the POST instead of failing it.
			select {
			case slotted := <-g.reached:
				if slotted {
					t.Error("the gate handed a slot to a POST it cannot read as a call")
				}
			default:
				t.Error("the POST never reached the handler behind the gate")
			}
			if got := g.held.open.Load(); got != 1 {
				t.Errorf("open = %d, want the count untouched", got)
			}
		})
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

	req := heldPost(t, "1", "tools/call", protocolVersionStandardHeaders)
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

// TestMcpServerGate_Middleware_WithoutACountTakesNoSlot covers a gate built
// with no count, which only tests build: it hands no slot on, so the servers'
// own count, the one that is exact, is left to count the call.
func TestMcpServerGate_Middleware_WithoutACountTakesNoSlot(t *testing.T) {
	t.Parallel()
	g := newHeldGate(t)
	g.open()
	gate := newGate(t, okFactory)
	var slotted bool
	handler := gate.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, slotted = r.Context().Value(gateHeldSlotKey{}).(*gateHeldSlot)
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, heldPost(t, "1", "tools/call", protocolVersionStandardHeaders))
	if rec.Code != http.StatusOK || slotted {
		t.Errorf("status %d, slot handed on %v; want the call served with no slot of the gate's", rec.Code, slotted)
	}
}

// holdingGitLab is a stand-in instance for the in-process tests below: it
// answers the probes a pool entry needs, answers project 2 at once, and holds
// every read of project 1 until released, counting the reads it holds.
type holdingGitLab struct {
	url     string
	held    atomic.Int64
	gate    chan struct{}
	release func()
}

// newHoldingGitLab starts the stand-in. The headers of a held read are
// written before the wait, because the server gives GitLab a minute to answer
// with headers and would abandon and resend a read held before them.
func newHoldingGitLab(t *testing.T) *holdingGitLab {
	t.Helper()
	g := &holdingGitLab{gate: make(chan struct{})}
	var once sync.Once
	g.release = func() { once.Do(func() { close(g.gate) }) }
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":42,"username":"testuser"}`))
	})
	mux.HandleFunc("GET /api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"17.0.0","revision":"test"}`))
	})
	mux.HandleFunc("GET /api/v4/projects/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.PathValue("id") == "1" {
			w.WriteHeader(http.StatusOK)
			_ = http.NewResponseController(w).Flush()
			g.held.Add(1)
			select {
			case <-g.gate:
			case <-r.Context().Done():
			}
			g.held.Add(-1)
		}
		_, _ = fmt.Fprintf(w, `{"id":%s,"name":"proj","path_with_namespace":"g/p","web_url":"https://example.invalid/g/p"}`,
			r.PathValue("id"))
	})
	srv := httptest.NewServer(mux)
	// Registered after the close, so it runs first: a read still held when
	// the test ends is let go before the server waits for it.
	t.Cleanup(srv.Close)
	t.Cleanup(g.release)
	g.url = srv.URL
	return g
}

// startHeldServer mounts the handler chain a legacy deployment mounts over the
// shaped pool, against the stand-in instance and capped at maxClients
// entries, and serves it: each call runs through the gate, the carrier, the
// SDK and the real server shape, counted on the process's own count. It
// returns the server, the pool's binding, and a lookup of the owner a
// credential's entry carries.
func startHeldServer(t *testing.T, gitlab *holdingGitLab, maxClients int) (*httptest.Server, poolBinding, func(token string) string) {
	t.Helper()
	cfg := &config.Config{
		GitLabURL: gitlab.url, Tier: edition.Free, TierExplicit: true, IgnoreScopes: true, Stateless: true,
		ToolSurface: config.ToolSurfaceDynamic, CapabilitySurface: config.CapabilitySurfaceFull,
		MaxHTTPClients: maxClients,
	}
	binding, pool := newShapedServerPool(t.Context(), cfg)
	t.Cleanup(pool.Close)
	mux := http.NewServeMux()
	registerLegacyMCPHandlers(t.Context(), cfg, pool, binding, mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	// Registered after the close, so it runs first: a call still held when
	// the test stops early is let go before the server waits for it, which
	// would otherwise wait for ever on a release registered before it.
	t.Cleanup(gitlab.release)
	owner := func(token string) string {
		entry, err := pool.GetOrCreateEntry(token, gitlab.url, nil)
		if err != nil {
			t.Fatalf("GetOrCreateEntry: %v", err)
		}
		return entry.Owner()
	}
	return srv, binding, owner
}

// modernCall is a tools/call running project.get on project, the way a
// 2026-07-28 client sends it.
func modernCall(id, project string) (string, http.Header) {
	body := `{"jsonrpc":"2.0","id":` + id + `,"method":"tools/call","params":{"name":"gitlab_execute_action",` +
		`"arguments":{"action":"project.get","params":{"project_id":"` + project + `"}},` +
		`"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`
	return body, http.Header{
		"Mcp-Protocol-Version": {protocolVersionStandardHeaders},
		"Mcp-Method":           {"tools/call"},
		"Mcp-Name":             {"gitlab_execute_action"},
		"Mcp-Param-Action":     {"project.get"},
	}
}

// legacyHeader is what a client of an older revision sends.
func legacyHeader(version string) http.Header {
	return http.Header{"Mcp-Protocol-Version": {version}}
}

// postHeld sends body with header and token to srv and returns the status,
// the response header and the body.
func postHeld(t *testing.T, srv *httptest.Server, token, body string, header http.Header) (int, http.Header, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/mcp", strings.NewReader(body))
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	maps.Copy(req.Header, header)
	req.Header.Set("Content-Type", mimeJSON)
	req.Header.Set("Accept", mimeJSONSSE)
	req.Header.Set("PRIVATE-TOKEN", token)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading the response: %v", err)
	}
	return resp.StatusCode, resp.Header, string(raw)
}

// fillProcessHeldRequests takes every slot of the process's count but free,
// and gives them back when the test ends.
func fillProcessHeldRequests(t *testing.T, free int64) {
	t.Helper()
	previous := processHeldRequests.open.Swap(processHeldRequests.limit - free)
	t.Cleanup(func() { processHeldRequests.open.Store(previous) })
}

// TestHeldCeiling_CountsEachCallTheSDKDispatches drives the whole chain with
// every slot of the process taken, and holds the count to what a POST carries
// rather than to the POST: a modern tools/call is refused in the gate with the
// 503; a tools/call on an older revision reaches the SDK and is refused there
// as a result flagged with isError; each call of an older revision's batch is
// refused on its own, so a batch cannot carry more calls than there are slots;
// a read is refused with the in-band code; while a notification and a
// response to a request of the server's own, which hold nothing, are accepted
// as the SDK accepts them, so a client's answer to an elicitation still gets
// through to the call waiting on it.
//
// Not parallel: it fills the process-wide count.
func TestHeldCeiling_CountsEachCallTheSDKDispatches(t *testing.T) {
	gitlab := newHoldingGitLab(t)
	srv, _, _ := startHeldServer(t, gitlab, config.DefaultMaxHTTPClients)
	fillProcessHeldRequests(t, 0)

	const busy = "This server is busy. Retry later."
	oldCall := `{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"gitlab_execute_action",` +
		`"arguments":{"action":"project.get","params":{"project_id":"2"}}}}`

	t.Run("a modern call is refused in the gate", func(t *testing.T) {
		body, header := modernCall("7", "2")
		status, respHeader, got := postHeld(t, srv, gateTestToken, body, header)
		if status != http.StatusServiceUnavailable || !strings.Contains(got, busy) || !strings.Contains(got, `"id":7`) {
			t.Errorf("status %d, body %s; want the gate's 503 echoing id 7", status, got)
		}
		if respHeader.Get("Retry-After") == "" {
			t.Error("the gate's refusal carries no Retry-After")
		}
	})
	t.Run("an older revision's call is refused in the SDK", func(t *testing.T) {
		status, _, got := postHeld(t, srv, gateTestToken, fmt.Sprintf(oldCall, 8), legacyHeader("2025-11-25"))
		if status != http.StatusOK || !strings.Contains(got, busy) || !strings.Contains(got, `"isError":true`) {
			t.Errorf("status %d, body %s; want a result flagged with isError saying the server is busy", status, got)
		}
	})
	t.Run("each call of a batch is refused on its own", func(t *testing.T) {
		batch := "[" + fmt.Sprintf(oldCall, 9) + "," + fmt.Sprintf(oldCall, 10) + "]"
		status, _, got := postHeld(t, srv, gateTestToken, batch, legacyHeader("2025-03-26"))
		if status != http.StatusOK || strings.Count(got, busy) != 2 {
			t.Errorf("status %d, body %s; want both calls of the batch refused", status, got)
		}
	})
	t.Run("a read is refused with the in-band code", func(t *testing.T) {
		read := `{"jsonrpc":"2.0","id":11,"method":"resources/read","params":{"uri":"gitlab://project/2"}}`
		status, _, got := postHeld(t, srv, gateTestToken, read, legacyHeader("2025-11-25"))
		if status != http.StatusOK || !strings.Contains(got, busy) ||
			!strings.Contains(got, strconv.Itoa(tenancy.CodeTooManyRequests)) {
			t.Errorf("status %d, body %s; want a JSON-RPC error %d", status, got, tenancy.CodeTooManyRequests)
		}
	})
	for _, tc := range []struct{ name, body string }{
		{name: "a notification", body: `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":1}}`},
		{name: "a response to a request of the server's", body: `{"jsonrpc":"2.0","id":12,"result":{"action":"accept"}}`},
	} {
		t.Run(tc.name+" still gets through", func(t *testing.T) {
			status, _, got := postHeld(t, srv, gateTestToken, tc.body, legacyHeader("2025-11-25"))
			if status != http.StatusAccepted {
				t.Errorf("status %d, body %s; want 202, since it holds nothing", status, got)
			}
		})
	}
}

// TestHeldCall_TakesOneSlotAndLeavesItsEntryEvictable drives a modern call
// GitLab keeps waiting through the whole chain and holds three things about
// it. It takes one slot of the process's count, the gate's, which the call
// claims rather than taking a second. It makes its pool entry no busier, the
// answer POL-003 gives for a held call, so the pool's size bound can evict the
// entry for another credential. And the call outlives that eviction: once
// GitLab answers, the caller gets its project.
//
// Not parallel: it reads the process-wide count.
func TestHeldCall_TakesOneSlotAndLeavesItsEntryEvictable(t *testing.T) {
	gitlab := newHoldingGitLab(t)
	srv, binding, owner := startHeldServer(t, gitlab, 1)
	before := processHeldRequests.open.Load()

	type outcome struct {
		status int
		body   string
	}
	done := make(chan outcome, 1)
	go func() {
		body, header := modernCall("1", "1")
		status, _, got := postHeldFromGoroutine(srv, gateTestToken, body, header)
		done <- outcome{status, got}
	}()
	deadline := time.Now().Add(30 * time.Second)
	for gitlab.held.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if gitlab.held.Load() != 1 {
		t.Fatal("the call never reached GitLab")
	}
	if got := processHeldRequests.open.Load() - before; got != 1 {
		t.Errorf("the held call took %d slots, want the gate's one", got)
	}
	heldOwner := owner(gateTestToken)
	if state := binding.credentials.get(heldOwner); state == nil || state.busy() {
		t.Errorf("the held call's entry is busy (%v), want it evictable like any quiet entry", state)
	}

	// A second credential at a pool bound of one: the quiet entry of the
	// first, which only holds a call, is the one evicted to make room.
	body, header := modernCall("2", "2")
	if status, _, got := postHeld(t, srv, "glpat-second-credential", body, header); status != http.StatusOK {
		t.Fatalf("the second credential's call = %d: %s", status, got)
	}
	if binding.credentials.get(heldOwner) != nil {
		t.Error("the held call's entry survived a second credential at a pool bound of one")
	}

	gitlab.release()
	select {
	case o := <-done:
		if o.status != http.StatusOK || !strings.Contains(o.body, "g/p") || strings.Contains(o.body, `"isError":true`) {
			t.Errorf("the held call ended %d after its entry was evicted: %s", o.status, o.body)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the held call never ended once GitLab answered")
	}
	if got := processHeldRequests.open.Load(); got != before {
		t.Errorf("open = %d after the call ended, want %d", got, before)
	}
}

// postHeldFromGoroutine is postHeld for a goroutine: it touches no
// *testing.T and reports a failure to make the call as its body.
func postHeldFromGoroutine(srv *httptest.Server, token, body string, header http.Header) (int, http.Header, string) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/mcp", strings.NewReader(body))
	if err != nil {
		return 0, nil, err.Error()
	}
	maps.Copy(req.Header, header)
	req.Header.Set("Content-Type", mimeJSON)
	req.Header.Set("Accept", mimeJSONSSE)
	req.Header.Set("PRIVATE-TOKEN", token)
	resp, err := srv.Client().Do(req)
	if err != nil {
		return 0, nil, err.Error()
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, resp.Header, err.Error()
	}
	return resp.StatusCode, resp.Header, string(raw)
}

// TestRegisterOAuthMCPHandlers_RefusesPastTheProcessHeldCeiling holds the
// oauth mode's wiring of the gate: with every slot of the process taken, a
// modern call with a verified bearer is refused in the gate with the 503, as
// it is in legacy mode.
//
// Not parallel: it fills the process-wide count.
func TestRegisterOAuthMCPHandlers_RefusesPastTheProcessHeldCeiling(t *testing.T) {
	gitlab := newMockGitLabServerWithUser(t)
	cfg := &config.Config{
		GitLabURL:      gitlab.URL,
		MaxHTTPClients: config.DefaultMaxHTTPClients,
		SessionTimeout: config.DefaultSessionTimeout,
		ToolSurface:    config.ToolSurfaceDynamic,
		AuthMode:       config.AuthModeOAuth,
		PublicURL:      "https://mcp.example.com",
		OAuthCacheTTL:  config.DefaultOAuthCacheTTL,
		Stateless:      true,
		Tier:           edition.Free,
		TierExplicit:   true,
		IgnoreScopes:   true,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addr, errCh := oauthAddr(t, ctx, cfg)
	fillProcessHeldRequests(t, 0)

	body, header := modernCall("5", "2")
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+addr+"/mcp", strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	maps.Copy(req.Header, header)
	req.Header.Set(hdrContentType, mimeJSON)
	req.Header.Set("Accept", mimeJSONSSE)
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp, err := testHTTPClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	got := readAndCloseBody(t, resp)
	if resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(got, heldRefusalText) {
		t.Errorf("status %d, body %s; want the held ceiling's 503", resp.StatusCode, got)
	}

	cancel()
	select {
	case srvErr := <-errCh:
		if srvErr != nil {
			t.Fatalf("serveHTTP error: %v", srvErr)
		}
	case <-time.After(testHTTPLivenessTimeout):
		t.Fatal("shutdown timeout")
	}
}
