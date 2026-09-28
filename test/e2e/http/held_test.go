//go:build httpe2e

// held_test.go holds the ceiling on the requests the process holds open
// (register row HLD-011, issue 951) on the wire.
//
// A held request is a tools/call waiting on a GitLab that has not answered.
// The stand-in instance below holds every project read until the test lets it
// go, so the test can put exactly as many calls in flight as the ceiling allows
// and then ask for one more.
package httpe2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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

// holdingGitLab is a stand-in instance that holds every project read until
// release is called, and counts the reads it is holding.
type holdingGitLab struct {
	url     string
	held    func() int64
	release func()
}

// startHoldingProjectsGitLab serves the probes a pool entry needs and holds
// every project read. The headers are written before the wait, because the
// server gives GitLab a minute to answer with headers and would abandon and
// resend a read held before them.
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
		w.WriteHeader(http.StatusOK)
		_ = http.NewResponseController(w).Flush()
		held.Add(1)
		select {
		case <-gate:
		case <-r.Context().Done():
		}
		held.Add(-1)
		_, _ = fmt.Fprint(w, `{"id":1,"name":"proj","path_with_namespace":"g/p","web_url":"http://example.invalid/g/p"}`)
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

// heldCall is one tools/call that runs project.get through the dynamic
// surface, for a goroutine: it touches no *testing.T, and a failure to make
// the call at all is its error.
func heldCall(ctx context.Context, client *http.Client, baseURL string, id int) (response, error) {
	body := `{"jsonrpc":"2.0","id":` + strconv.Itoa(id) + `,"method":"tools/call","params":{"name":"gitlab_execute_action",` +
		`"arguments":{"action":"project.get","params":{"project_id":"1"}},` +
		`"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/mcp", strings.NewReader(body))
	if err != nil {
		return response{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", acceptHeader)
	req.Header.Set("MCP-Protocol-Version", protocolVersion)
	req.Header.Set("Mcp-Method", "tools/call")
	req.Header.Set("Mcp-Name", "gitlab_execute_action")
	req.Header.Set("Mcp-Param-Action", "project.get")
	req.Header.Set("PRIVATE-TOKEN", "glpat-held")
	resp, err := client.Do(req)
	if err != nil {
		return response{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	return response{status: resp.StatusCode, header: resp.Header, body: string(raw)}, err
}

// assertHeldRefusal checks the answer to a call past the held ceiling: a 503
// with the register's fixed Retry-After, a JSON-RPC body echoing the call's id
// with -50300 and the register's words, and the operator's log line naming the
// process as the scope and the ceiling as the figure.
func assertHeldRefusal(t *testing.T, srv *server, refused response) {
	t.Helper()
	if refused.status != http.StatusServiceUnavailable {
		t.Fatalf("the call past the ceiling = %d, want 503: %s", refused.status, truncate(refused.body))
	}
	if got, want := refused.header.Get("Retry-After"), strconv.Itoa(int(tenancy.UpstreamRetryAfter.Seconds())); got != want {
		t.Errorf("Retry-After = %q, want %q", got, want)
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
	if string(rpc.ID) != "1" || rpc.Error.Code != tenancy.CodeUnavailable ||
		!strings.HasPrefix(rpc.Error.Message, "This server is holding as many requests as it serves at once.") {
		t.Errorf("refusal id %s, code %d, message %q; want the request's id, %d and the register's words",
			rpc.ID, rpc.Error.Code, rpc.Error.Message, tenancy.CodeUnavailable)
	}
	line := awaitLogLine(t, srv, heldRefusalLine)
	if !strings.Contains(line, `"scope":"process"`) ||
		!strings.Contains(line, `"limit_held_requests":`+strconv.Itoa(tenancy.HeldRequestsPerProcess)) {
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

// TestLimit_ProcessBoundsHeldRequests fills the ceiling with calls GitLab
// keeps waiting and asks for one more.
//
// The one more is refused in the gate, before the SDK reads it: a 503 whose
// JSON-RPC body echoes its id and carries -50300, with the register's fixed
// Retry-After, in the words the register declares, and a line in the log
// naming the process as the scope. /health still answers, which is what the
// ceiling exists to keep true, and a subscriptions/listen still reaches the
// SDK, since the listen ceilings count it rather than this one. Once GitLab
// answers, every held call is served and the next call is served too.
//
// The rate limit is off so one credential can hold every slot: the per-caller
// bucket would otherwise refuse the calls first, which is a different bound.
func TestLimit_ProcessBoundsHeldRequests(t *testing.T) {
	gitlab := startHoldingProjectsGitLab(t)
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url, "--rate-limit-rps=0", "--capability-surface=full")
	const ceiling = tenancy.HeldRequestsPerProcess

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
			resp, err := heldCall(ctx, client, srv.baseURL, 1000+i)
			outcomes <- outcome{resp, err}
		}()
	}

	deadline := time.Now().Add(time.Minute)
	for gitlab.held() < ceiling && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if got := gitlab.held(); got != ceiling {
		t.Fatalf("GitLab is holding %d calls, want the ceiling's %d in flight before the next is offered", got, ceiling)
	}

	refused, err := heldCall(ctx, client, srv.baseURL, 1)
	if err != nil {
		t.Fatalf("the call past the ceiling could not be made: %v", err)
	}
	assertHeldRefusal(t, srv, refused)

	if health := srv.do(t, request{method: http.MethodGet, path: "/health"}); health.status != http.StatusOK {
		t.Errorf("/health = %d with every held slot taken, want 200", health.status)
	}

	const listenMeta = `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28",` +
		`"io.modelcontextprotocol/clientCapabilities":{},` +
		`"io.modelcontextprotocol/clientInfo":{"name":"probe","version":"1"}}`
	sseFrameContaining(t, srv, "subscriptions/listen", "2026-07-28",
		`{"jsonrpc":"2.0","id":2,"method":"subscriptions/listen","params":{"notifications":{"toolsListChanged":true},`+
			listenMeta+`}}`,
		"notifications/subscriptions/acknowledged")

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

	after, err := heldCall(ctx, client, srv.baseURL, 3)
	if err != nil || after.status != http.StatusOK {
		t.Errorf("a call after the held ones ended = %d (%v), want 200 now that the slots are free: %s",
			after.status, err, truncate(after.body))
	}
}
