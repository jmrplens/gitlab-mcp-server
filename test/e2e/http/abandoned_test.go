//go:build httpe2e

package httpe2e

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// abandonedRetryInterval is the wait client-go takes between attempts here.
//
// internal/gitlab clamps every backoff at five seconds, and a 429 naming a
// RateLimit-Reset further out than that takes the whole clamp. So a second
// attempt lands five seconds after the first, and a third five seconds after
// that.
const abandonedRetryInterval = 5 * time.Second

// abandonedWatchWindow is how long the test watches for a retry that must not
// happen. It sits between one retry interval and two, so an unfixed server is
// caught by its second attempt and a fixed one is not kept waiting for a third.
const abandonedWatchWindow = abandonedRetryInterval + 3*time.Second

// abandonedToken is the credential the abandoned call carries. Every request in
// one subtest uses it, so the handshake and the call under test share a pool
// entry and the catalog is built once.
const abandonedToken = "glpat-abandoned"

// TestAbandonedToolCall_StopsReachingGitLab drives a tools/call on a
// pre-2026-07-28 protocol, abandons the POST once the call has reached the fake
// instance, and asserts nothing further reaches it.
//
// The old protocol is the whole point. From 2026-07-28 a client cancels by
// sending notifications/cancelled, which the SDK turns into a cancel on the
// request's own id, and that path already worked. An older client has no such
// message: it can only stop listening, and the handler's context does not
// descend from the HTTP request, so nothing used to notice. The call kept
// running, and client-go kept re-sending it with the caller's credential to
// their instance for a result nobody would read.
//
// Both transports are exercised because the SDK's own answer covers neither
// case here: [mcp.StreamableHTTPOptions.PropagateRequestCancellation] applies
// to 2026-07-28 requests only, and stateful mode does not serve that revision
// at all.
//
// This has to run against the real binary. The behavior is a property of the
// assembled HTTP chain: where the carrier token is stamped, which context the
// SDK connects the session with, and how far down the handler stack the
// cancellation reaches. A test that rebuilt any part of that chain would be
// testing its own copy.
func TestAbandonedToolCall_StopsReachingGitLab(t *testing.T) {
	tests := []struct {
		name  string
		flags []string
		// open prepares the transport and returns the headers the abandoned
		// call must carry.
		open func(t *testing.T, srv *server) map[string]string
	}{
		{name: "stateless, the default transport", open: openStatelessSession},
		{name: "stateful sessions", flags: []string{"--stateless=false"}, open: openStatefulSession},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var attempts atomic.Int64
			gitlab := startRateLimitedGitLab(t, &attempts)
			srv := startServer(t, nil, append([]string{"--gitlab-url=" + gitlab}, tt.flags...)...)
			headers := tt.open(t, srv)

			callCtx, abandon := context.WithCancel(context.Background())
			defer abandon()
			go issueAbandonedCall(callCtx, srv, headers)

			// Wait for the call to actually reach GitLab. Abandoning before it
			// does would prove nothing: there would be no retry chain in
			// flight to stop.
			if !waitForAttempts(&attempts, 1, time.Minute) {
				t.Fatalf("the tool call never reached the fake instance; upstream saw %d requests", attempts.Load())
			}
			abandon()

			// From here the client is gone. A fixed server cancels the handler
			// and the retry chain unwinds where it sleeps; an unfixed one
			// re-sends at the clamped interval, and it is that second request
			// the window catches.
			if waitForAttempts(&attempts, 2, abandonedWatchWindow) {
				t.Errorf("upstream saw %d requests after the client abandoned the call; it should have seen 1", attempts.Load())
			}
		})
	}
}

// legacyClientHeaders spell a pre-2026-07-28 client: a credential, and none of
// the protocol headers the harness adds for the current revision. The absent
// MCP-Protocol-Version is what makes the SDK negotiate 2025-03-26, and
// Mcp-Method is required only from 2026-07-28.
func legacyClientHeaders() map[string]string {
	return map[string]string{
		"PRIVATE-TOKEN":        abandonedToken,
		"MCP-Protocol-Version": "",
		"Mcp-Method":           "",
	}
}

// openStatelessSession builds the pool entry before the call under test starts.
//
// Each stateless POST is its own session, so there is no handshake to keep; the
// one thing worth doing in advance is the catalog. Otherwise the readiness gate
// holds the abandoned call for the second or two registration takes, and the
// test would be measuring startup rather than abandonment.
func openStatelessSession(t *testing.T, srv *server) map[string]string {
	t.Helper()

	warm := srv.do(t, request{body: legacyToolsListBody, headers: legacyClientHeaders()})
	if warm.status != http.StatusOK {
		t.Fatalf("warming the pool entry: tools/list = %d: %s", warm.status, warm.body)
	}
	return legacyClientHeaders()
}

// openStatefulSession performs the legacy handshake and returns the headers
// that address the session it created.
//
// A stateful server synthesizes no session state, so a tools/call arriving
// without a session id would be refused by the initialization gate before any
// handler ran. The tools/list at the end is the same warm-up the stateless case
// does, for the same reason.
func openStatefulSession(t *testing.T, srv *server) map[string]string {
	t.Helper()

	const initialize = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26",` +
		`"capabilities":{},"clientInfo":{"name":"httpe2e","version":"0"}}}`

	opened := srv.do(t, request{body: initialize, headers: legacyClientHeaders()})
	if opened.status != http.StatusOK {
		t.Fatalf("opening a stateful session: initialize = %d: %s", opened.status, opened.body)
	}
	sessionID := opened.header.Get("Mcp-Session-Id")
	if sessionID == "" {
		t.Fatalf("the stateful server answered initialize without a session id: %v", opened.header)
	}

	headers := legacyClientHeaders()
	headers["Mcp-Session-Id"] = sessionID

	ready := srv.do(t, request{body: `{"jsonrpc":"2.0","method":"notifications/initialized"}`, headers: headers})
	if ready.status != http.StatusOK && ready.status != http.StatusAccepted {
		t.Fatalf("completing the handshake: notifications/initialized = %d: %s", ready.status, ready.body)
	}

	warm := srv.do(t, request{body: legacyToolsListBody, headers: headers})
	if warm.status != http.StatusOK {
		t.Fatalf("warming the pool entry: tools/list = %d: %s", warm.status, warm.body)
	}
	return headers
}

// issueAbandonedCall sends the tools/call under test and returns when the
// context is cancelled or the server answers, whichever comes first.
//
// It runs on its own goroutine and so takes no *testing.T and asserts nothing:
// the test goroutine reads the upstream counter instead. A failed request is
// the expected outcome here, since the point of the test is to cut the response
// off.
func issueAbandonedCall(ctx context.Context, srv *server, headers map[string]string) {
	const body = `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"gitlab_execute_action",` +
		`"arguments":{"action":"project.get","params":{"project_id":"acme/widgets"}}}}`

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.baseURL+"/mcp", strings.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", acceptHeader)
	for name, value := range headers {
		if value == "" {
			continue
		}
		req.Header.Set(name, value)
	}

	resp, err := srv.httpClient().Do(req)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}

// waitForAttempts reports whether the counter reached want before the deadline.
func waitForAttempts(attempts *atomic.Int64, want int64, within time.Duration) bool {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if attempts.Load() >= want {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return attempts.Load() >= want
}

// startRateLimitedGitLab serves the endpoints the server probes while it builds
// a pool entry, and answers the project path 429 with a RateLimit-Reset an hour
// ahead, counting each such request.
//
// The 429 is what makes an abandoned call observable. A request that simply
// fails is retried twice at 700 ms and 1.4 s and is over in about two seconds,
// which is inside the noise of the test; a 429 naming a distant reset takes the
// five-second clamp for every wait, so an unfixed server is still visibly at
// work five seconds after the client has gone.
func startRateLimitedGitLab(t *testing.T, attempts *atomic.Int64) string {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"17.0.0","revision":"abcdef"}`))
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":7,"username":"someone","name":"Some One","state":"active"}`))
	})
	mux.HandleFunc("/api/v4/projects/", func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.Header().Set("RateLimit-Reset", strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10))
		w.WriteHeader(http.StatusTooManyRequests)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		// The scope and tier probes ask other paths; 404 means "unavailable",
		// which every caller of theirs handles.
		w.WriteHeader(http.StatusNotFound)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}
