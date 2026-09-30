//go:build httpe2e

// held_test.go holds the ceiling on the calls the process holds open
// (register row HLD-011, issue 951) on the wire.
//
// A held call is a tools/call waiting on a GitLab that has not answered. The
// stand-in instance below holds every read of project 1 until the test lets it
// go, so the test can put exactly as many calls in flight as the ceiling
// allows and then ask for more, in every shape a POST can carry one.
package httpe2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// heldRefusalLine is the operator's line for a refusal at the ceiling.
const heldRefusalLine = "request refused: too many requests held across the process"

// heldBusy is what every refusal of the ceiling says: that the server is busy
// and to retry, naming no bound and no figure.
const heldBusy = "This server is busy. Retry later."

// heldDescriptorLimit is the descriptor limit the server is started under, soft
// and hard, where the platform has one to set: the limit a small container is
// given, and the one the issue's measurement asks the process to survive.
const heldDescriptorLimit = 1024

// underDescriptorLimit starts the server through a shell that lowers its
// descriptor limit and then puts the server in its own place, so the process
// the harness signals is the server's. Windows has no such limit to set, and
// the server sizes its ceiling there against the register's fallback, which
// is the figure a limit of 1024 gives.
func underDescriptorLimit(limit int) launcher {
	return func(bin string, args []string) (string, []string) {
		if runtime.GOOS == "windows" {
			return bin, args
		}
		return "/bin/sh", append([]string{"-c", `ulimit -n "$0" && exec "$@"`, strconv.Itoa(limit), bin}, args...)
	}
}

// holdingGitLab is a stand-in instance that holds every read of project 1
// until release is called, answers project 2 at once, and counts the reads it
// is holding.
type holdingGitLab struct {
	url     string
	held    func() int64
	release func()
}

// startHoldingProjectsGitLab serves the probes a pool entry needs and holds
// every read of project 1. The headers are written before the wait, because
// the server gives GitLab a minute to answer with headers and would abandon
// and resend a read held before them.
func startHoldingProjectsGitLab(t *testing.T) *holdingGitLab {
	t.Helper()

	var held atomic.Int64
	gate := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(gate) }) }

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"17.0.0","revision":"abcdef"}`))
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":7,"username":"someone"}`))
	})
	mux.HandleFunc("/api/v4/projects/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.PathValue("id") == "1" {
			w.WriteHeader(http.StatusOK)
			_ = http.NewResponseController(w).Flush()
			held.Add(1)
			select {
			case <-gate:
			case <-r.Context().Done():
			}
			held.Add(-1)
		}
		_, _ = fmt.Fprintf(w, `{"id":%s,"name":"proj","path_with_namespace":"g/p","web_url":"http://example.invalid/g/p"}`,
			r.PathValue("id"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	srv := httptest.NewServer(mux)
	// Registered after the close, so it runs first: a read still held when
	// the test ends is let go before the server waits for it.
	t.Cleanup(srv.Close)
	t.Cleanup(release)
	return &holdingGitLab{url: srv.URL, held: held.Load, release: release}
}

// heldPost sends body to the MCP endpoint with the headers given and the
// test's credential, for a goroutine: it touches no *testing.T, and a failure
// to make the call at all is its error.
func heldPost(ctx context.Context, client *http.Client, baseURL, body string, header http.Header) (response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/mcp", strings.NewReader(body))
	if err != nil {
		return response{}, err
	}
	maps.Copy(req.Header, header)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", acceptHeader)
	req.Header.Set("PRIVATE-TOKEN", "glpat-held")
	resp, err := client.Do(req)
	if err != nil {
		return response{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if resp.Close {
		// net/http takes Connection out of the header it hands back and
		// records it here instead.
		resp.Header.Set("Connection", "close")
	}
	return response{status: resp.StatusCode, header: resp.Header, body: string(raw)}, err
}

// modernHeldCall is one tools/call running project.get on project through the
// dynamic surface, the way a 2026-07-28 client sends it.
func modernHeldCall(ctx context.Context, client *http.Client, baseURL string, id int, project string) (response, error) {
	body := `{"jsonrpc":"2.0","id":` + strconv.Itoa(id) + `,"method":"tools/call","params":{"name":"gitlab_execute_action",` +
		`"arguments":{"action":"project.get","params":{"project_id":"` + project + `"}},` +
		`"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`
	return heldPost(ctx, client, baseURL, body, http.Header{
		"Mcp-Protocol-Version": {protocolVersion},
		"Mcp-Method":           {"tools/call"},
		"Mcp-Name":             {"gitlab_execute_action"},
		"Mcp-Param-Action":     {"project.get"},
	})
}

// legacyCall is one tools/call running project.get on project 2 without the
// protocol's _meta, the way a client of an older revision sends it.
func legacyCall(id int) string {
	return `{"jsonrpc":"2.0","id":` + strconv.Itoa(id) + `,"method":"tools/call","params":{"name":"gitlab_execute_action",` +
		`"arguments":{"action":"project.get","params":{"project_id":"2"}}}}`
}

// heldCeilingOf reads the ceiling the server announced at startup, which is
// what its descriptor limit gave it.
func heldCeilingOf(t *testing.T, srv *server) int {
	t.Helper()
	line := awaitLogLine(t, srv, `"held_requests_per_process":`)
	match := regexp.MustCompile(`"held_requests_per_process":(\d+)`).FindStringSubmatch(line)
	if match == nil {
		t.Fatalf("the startup line announces no held-request ceiling: %q", line)
	}
	ceiling, err := strconv.Atoi(match[1])
	if err != nil {
		t.Fatalf("the announced ceiling %q is not a number: %v", match[1], err)
	}
	return ceiling
}

// assertHeldGateRefusal checks the answer to a modern call past the ceiling: a
// 503 with the register's fixed Retry-After and the connection closed, a
// JSON-RPC body echoing the call's id with -50300 and the busy words, and the
// operator's log line naming the process as the scope and the ceiling as the
// figure.
func assertHeldGateRefusal(t *testing.T, srv *server, refused response, ceiling int) {
	t.Helper()
	if refused.status != http.StatusServiceUnavailable {
		t.Fatalf("the call past the ceiling = %d, want 503: %s", refused.status, truncate(refused.body))
	}
	if got, want := refused.header.Get("Retry-After"), strconv.Itoa(int(tenancy.UpstreamRetryAfter.Seconds())); got != want {
		t.Errorf("Retry-After = %q, want %q", got, want)
	}
	if got := refused.header.Get("Connection"); got != "close" {
		t.Errorf("Connection = %q, want the refused caller's connection closed", got)
	}
	var rpc struct {
		ID    json.RawMessage `json:"id"`
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if decodeErr := json.Unmarshal([]byte(refused.body), &rpc); decodeErr != nil {
		t.Fatalf("the refusal is not a JSON-RPC body: %v: %s", decodeErr, truncate(refused.body))
	}
	if string(rpc.ID) != "1" || rpc.Error.Code != tenancy.CodeUnavailable || rpc.Error.Message != heldBusy {
		t.Errorf("refusal id %s, code %d, message %q; want the request's id, %d and %q",
			rpc.ID, rpc.Error.Code, rpc.Error.Message, tenancy.CodeUnavailable, heldBusy)
	}
	line := awaitLogLine(t, srv, heldRefusalLine)
	if !strings.Contains(line, `"scope":"process"`) || !strings.Contains(line, `"limit_held_requests":`+strconv.Itoa(ceiling)) {
		t.Errorf("the refusal line does not name its scope and figure: %q", line)
	}
}

// heldCallServed reports whether a held call came back with the project GitLab
// answered once it was let go, and says why not when it did not.
func heldCallServed(t *testing.T, resp response, err error) bool {
	t.Helper()
	switch {
	case err != nil:
		t.Errorf("a held call failed: %v", err)
	case resp.status != http.StatusOK:
		t.Errorf("a held call ended %d: %s", resp.status, truncate(resp.body))
	case strings.Contains(resp.body, `"isError":true`) || !strings.Contains(resp.body, "g/p"):
		t.Errorf("a held call did not come back with the project GitLab answered: %s", truncate(resp.body))
	default:
		return true
	}
	return false
}

// assertOlderRevisionsAtTheCeiling checks what a client of an older revision
// meets with every slot taken: its call is refused by the SDK's dispatch as a
// result flagged with isError, each call of its batch is refused on its own,
// so a batch cannot carry more calls than there are slots, and its
// notification is still accepted, since it holds nothing.
func assertOlderRevisionsAtTheCeiling(t *testing.T, ctx context.Context, client *http.Client, baseURL string) {
	t.Helper()
	older, err := heldPost(ctx, client, baseURL, legacyCall(2), http.Header{"Mcp-Protocol-Version": {"2025-11-25"}})
	if err != nil || older.status != http.StatusOK || !strings.Contains(older.body, heldBusy) ||
		!strings.Contains(older.body, `"isError":true`) {
		t.Errorf("an older revision's call past the ceiling = %d (%v): %s; want a result flagged with isError saying %q",
			older.status, err, truncate(older.body), heldBusy)
	}
	batch, err := heldPost(ctx, client, baseURL, "["+legacyCall(3)+","+legacyCall(4)+"]",
		http.Header{"Mcp-Protocol-Version": {"2025-03-26"}})
	if err != nil || batch.status != http.StatusOK || strings.Count(batch.body, heldBusy) != 2 {
		t.Errorf("a batch of two calls past the ceiling = %d (%v): %s; want each call refused on its own",
			batch.status, err, truncate(batch.body))
	}
	notification, err := heldPost(ctx, client, baseURL,
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":1}}`,
		http.Header{"Mcp-Protocol-Version": {"2025-11-25"}})
	if err != nil || notification.status != http.StatusAccepted {
		t.Errorf("a notification with every slot taken = %d (%v), want 202: it holds nothing", notification.status, err)
	}
}

// windowsFloodWidth is how many of a flood's connections are open at once on
// Windows. The flood exists to outrun a descriptor limit Windows does not
// have, and there a burst of hundreds of loopback connections most likely
// overflows the listen queue, which answers the rest with WSAECONNREFUSED
// before the server accepts them and says nothing about the server; in waves
// of this width every connection reaches it.
const windowsFloodWidth = 64

// floodResult is how the requests of a flood ended: the ones refused with the
// gate's 503, and every other ending counted by what it was, so a failure
// says whether the server answered something else or never answered.
type floodResult struct {
	refused  int
	others   map[string]int
	firstErr string
}

// String names the endings that were not a 503, for a failure message.
func (r floodResult) String() string {
	if len(r.others) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(r.others))
	for _, ending := range slices.Sorted(maps.Keys(r.others)) {
		parts = append(parts, fmt.Sprintf("%d %s", r.others[ending], ending))
	}
	if r.firstErr != "" {
		parts = append(parts, "the first failure: "+r.firstErr)
	}
	return strings.Join(parts, ", ")
}

// flood sends count requests, each on a connection of its own, all at once
// except on Windows (windowsFloodWidth), and sorts how each ended. It touches
// no *testing.T, since the requests run on goroutines of their own.
func flood(count int, send func(client *http.Client, i int) (response, error)) floodResult {
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	width := count
	if runtime.GOOS == "windows" {
		width = min(count, windowsFloodWidth)
	}
	slots := make(chan struct{}, width)
	var mu sync.Mutex
	result := floodResult{others: map[string]int{}}
	var wait sync.WaitGroup
	for i := range count {
		slots <- struct{}{}
		wait.Go(func() {
			defer func() { <-slots }()
			resp, err := send(client, i)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err != nil:
				result.others["failed"]++
				if result.firstErr == "" {
					result.firstErr = err.Error()
				}
			case resp.status == http.StatusServiceUnavailable:
				result.refused++
			default:
				result.others["answered "+strconv.Itoa(resp.status)]++
			}
		})
	}
	wait.Wait()
	return result
}

// offerPastTheCeiling sends count modern calls through a flood and reports
// how they ended.
func offerPastTheCeiling(ctx context.Context, baseURL string, count int) floodResult {
	return flood(count, func(client *http.Client, i int) (response, error) {
		return modernHeldCall(ctx, client, baseURL, 5000+i, "2")
	})
}

// TestLimit_ProcessBoundsHeldRequests starts the server under a descriptor
// limit of 1024, fills the ceiling that limit gives it with calls GitLab keeps
// waiting, and then asks for more in every shape a POST can carry a call.
//
// A modern call is refused in the gate, before the SDK reads it: a 503 whose
// JSON-RPC body echoes its id and carries -50300, with the register's fixed
// Retry-After, the connection closed, words that name no bound, and a line in
// the log naming the process as the scope. A call on an older revision is
// refused by the SDK's dispatch as a result flagged with isError, and each
// call of an older revision's batch on its own, so a batch cannot carry more
// calls than there are slots. A notification is still accepted, since it
// holds nothing, and a listen still reaches the SDK, since the listen ceilings
// count it rather than this one.
//
// Then it offers six hundred more modern calls at once: held, they would need
// more descriptors than the limit allows, and the process would stop accepting
// connections, /health among them. Refused and closed, they cost nothing that
// stays, and /health answers. (Windows has no descriptor limit to set, so
// there the last half shows only that the refusals are made, and the calls
// go in waves the platform's listen queue can take.) Once GitLab
// answers, every held call is served, and the next call is served too.
//
// The rate limit is off so one credential can hold every slot: the per-caller
// bucket would otherwise refuse the calls first, which is a different bound.
func TestLimit_ProcessBoundsHeldRequests(t *testing.T) {
	gitlab := startHoldingProjectsGitLab(t)
	srv, err := tryStartServerLaunched(t, ephemeralPort, underDescriptorLimit(heldDescriptorLimit), nil,
		"--gitlab-url="+gitlab.url, "--rate-limit-rps=0", "--capability-surface=full")
	if err != nil {
		t.Fatalf("starting the server under a descriptor limit of %d: %v", heldDescriptorLimit, err)
	}
	ceiling := heldCeilingOf(t, srv)
	if ceiling != 192 {
		t.Fatalf("the server announced a ceiling of %d under a descriptor limit of %d, want 192", ceiling, heldDescriptorLimit)
	}

	// One client with room for every held call's connection, and no timeout
	// of its own: the calls are held on purpose, and the test's context ends
	// them if the test fails first.
	client := &http.Client{Transport: &http.Transport{MaxIdleConnsPerHost: ceiling}}
	defer client.CloseIdleConnections()
	ctx := t.Context()

	type outcome struct {
		resp response
		err  error
	}
	outcomes := make(chan outcome, ceiling)
	for i := range ceiling {
		go func() {
			resp, callErr := modernHeldCall(ctx, client, srv.baseURL, 1000+i, "1")
			outcomes <- outcome{resp, callErr}
		}()
	}

	deadline := time.Now().Add(time.Minute)
	for gitlab.held() < int64(ceiling) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if got := gitlab.held(); got != int64(ceiling) {
		t.Fatalf("GitLab is holding %d calls, want the ceiling's %d in flight before the next is offered", got, ceiling)
	}

	refused, err := modernHeldCall(ctx, client, srv.baseURL, 1, "2")
	if err != nil {
		t.Fatalf("the call past the ceiling could not be made: %v", err)
	}
	assertHeldGateRefusal(t, srv, refused, ceiling)

	assertOlderRevisionsAtTheCeiling(t, ctx, client, srv.baseURL)

	const listenMeta = `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28",` +
		`"io.modelcontextprotocol/clientCapabilities":{},` +
		`"io.modelcontextprotocol/clientInfo":{"name":"probe","version":"1"}}`
	sseFrameContaining(t, srv, "subscriptions/listen", "2026-07-28",
		`{"jsonrpc":"2.0","id":5,"method":"subscriptions/listen","params":{"notifications":{"toolsListChanged":true},`+
			listenMeta+`}}`,
		"notifications/subscriptions/acknowledged")

	// More modern calls than the descriptor limit could hold, on
	// connections of their own: all at once where the limit is set, in
	// waves on Windows (flood).
	const extra = 600
	if got := offerPastTheCeiling(ctx, srv.baseURL, extra); got.refused != extra {
		t.Errorf("%d of the %d calls offered past the ceiling were refused with 503, want every one; the rest: %s",
			got.refused, extra, got)
	}
	if health := srv.do(t, request{method: http.MethodGet, path: "/health"}); health.status != http.StatusOK {
		t.Errorf("/health = %d after %d calls were offered past the ceiling, want 200", health.status, extra)
	}

	gitlab.release()
	served := 0
	for range ceiling {
		o := <-outcomes
		if heldCallServed(t, o.resp, o.err) {
			served++
		}
	}
	if served != ceiling {
		t.Fatalf("%d of the %d held calls were served once GitLab answered", served, ceiling)
	}

	after, err := modernHeldCall(ctx, client, srv.baseURL, 6, "2")
	if err != nil || after.status != http.StatusOK {
		t.Errorf("a call after the held ones ended = %d (%v), want 200 now that the slots are free: %s",
			after.status, err, truncate(after.body))
	}
}
