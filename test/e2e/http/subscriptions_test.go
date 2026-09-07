//go:build httpe2e

package httpe2e

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestSubscriptionsListen_IsServedOnTheDefaultTransport pins that a
// subscriptions/listen request carrying resource URIs is acknowledged.
//
// The 2026-07-28 revision makes subscriptions/listen the only way a client can
// ask for resource updates, and this server advertises resources.subscribe in
// its handshake. On the shipped default (stateless HTTP) the resource half of
// that promise was dead: every such listen was refused with -32600 and its
// stream closed, while the capability went on being advertised.
//
// The cause was one handler serving two methods. The SDK's subscriptionsListen
// calls SubscribeHandler once per resource URI and returns the first error it
// gets before acknowledging anything, so the refusal written for the legacy
// resources/subscribe reached every listen that named a resource. A mixed
// listen was worse: its list-changed half died with the resource half.
//
// No handler-level test could see this, which is why it lived through a full
// unit suite: the refusal is correct when the handler is called directly, and
// wrong only because of who else calls it. The three cases below are therefore
// driven over the wire, and the third is the one that keeps the original
// refusal honest.
func TestSubscriptionsListen_IsServedOnTheDefaultTransport(t *testing.T) {
	const meta = `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28",` +
		`"io.modelcontextprotocol/clientCapabilities":{},` +
		`"io.modelcontextprotocol/clientInfo":{"name":"probe","version":"1"}}`

	tests := []struct {
		name        string
		rpcMethod   string
		protocol    string
		body        string
		wantInFrame string
	}{
		{
			name:        "a listen carrying a resource subscription is acknowledged",
			rpcMethod:   "subscriptions/listen",
			protocol:    "2026-07-28",
			body:        `{"jsonrpc":"2.0","id":7,"method":"subscriptions/listen","params":{"notifications":{"resourceSubscriptions":["gitlab://project/123"]},` + meta + `}}`,
			wantInFrame: "notifications/subscriptions/acknowledged",
		},
		{
			name:        "a listen mixing a resource with list-changed is acknowledged whole",
			rpcMethod:   "subscriptions/listen",
			protocol:    "2026-07-28",
			body:        `{"jsonrpc":"2.0","id":8,"method":"subscriptions/listen","params":{"notifications":{"toolsListChanged":true,"resourceSubscriptions":["gitlab://project/123"]},` + meta + `}}`,
			wantInFrame: "notifications/subscriptions/acknowledged",
		},
		{
			name:      "the legacy resources/subscribe is still refused on this transport",
			rpcMethod: "resources/subscribe",
			protocol:  "2025-11-25",
			body:      `{"jsonrpc":"2.0","id":9,"method":"resources/subscribe","params":{"uri":"gitlab://project/123"}}`,
			// The session really does end with the POST, so accepting would
			// leave the client waiting for a notification with nowhere to go.
			wantInFrame: "cannot be honored in stateless HTTP mode",
		},
	}

	gitlab := startFakeGitLabServingProjects(t)
	srv := startServer(t, nil, "--gitlab-url="+gitlab.URL, "--capability-surface=full")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := firstSSEFrame(t, srv, tt.rpcMethod, tt.protocol, tt.body)
			if !strings.Contains(frame, tt.wantInFrame) {
				t.Errorf("first frame does not contain %q:\n%s", tt.wantInFrame, frame)
			}
		})
	}
}

// firstSSEFrame sends one JSON-RPC request and returns the first data frame of
// the response stream.
//
// A listen holds its stream open for as long as the subscription lives, so the
// body cannot be read to EOF: the read has to stop at the first frame and close
// the response, or the test would wait out the subscription's whole lifetime.
func firstSSEFrame(t *testing.T, srv *server, rpcMethod, protocol, body string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.baseURL+"/mcp", strings.NewReader(body))
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", acceptHeader)
	req.Header.Set("MCP-Protocol-Version", protocol)
	req.Header.Set("Mcp-Method", rpcMethod)
	req.Header.Set("PRIVATE-TOKEN", "glpat-whatever")

	resp, err := srv.httpClient().Do(req)
	if err != nil {
		t.Fatalf("sending %s: %v", rpcMethod, err)
	}
	defer resp.Body.Close()

	var seen strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		seen.WriteString(line)
		seen.WriteString("\n")
		if after, ok := strings.CutPrefix(line, "data: "); ok {
			return after
		}
	}
	if scanErr := scanner.Err(); scanErr != nil {
		t.Fatalf("reading the response stream: %v", scanErr)
	}
	// A non-SSE body means the request was turned away before the stream
	// began, so it is the body, not the absence of a frame, that says why.
	t.Fatalf("the stream closed without a data frame (status %d):\n%s", resp.StatusCode, seen.String())
	return ""
}

// startFakeGitLabServingProjects is startFakeGitLab plus the projects these
// subscriptions watch.
//
// The shared fake answers 404 to everything but /user and /version, which is
// right for tests about admission and wrong here: a watcher's first act is to
// read the resource, and a 404 there is refused as an inaccessible resource
// before the acknowledgment is ever reached. That refusal is correct behavior,
// so the resource has to exist for the acknowledgment to be the thing under
// test.
//
// Any numeric project id is served, because the watcher ceiling is reached by
// watching distinct resources: a second subscription to a URI already watched
// joins the existing watcher rather than taking a slot.
func startFakeGitLabServingProjects(t *testing.T) *httptest.Server {
	t.Helper()

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
		id := r.PathValue("id")
		if _, err := strconv.Atoi(id); err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w,
			`{"id":%s,"name":"proj","path_with_namespace":"g/p","web_url":"http://example.invalid/g/p"}`, id)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// TestSubscriptionsListen_PastTheWatcherCeiling_IsRefusedWhole drives a listen
// that asks for more watchers than the credential may hold, and pins two things
// about the refusal on the wire.
//
// The first is the code. A watcher ceiling is server state, not a bad request:
// the listen is well formed and a retry later can succeed, so it takes the
// implementation-defined -32000 the rate limit and the stream ceiling already
// answer with. The manager speaks in sentinels and the SDK marshals an
// unrecognized error with code 0, which generic clients render as "unknown
// error", so the mapping is the whole difference between a client that backs
// off and a client that gives up.
//
// The second is that the refusal is whole. The SDK subscribes each URI in turn
// and returns the first error before acknowledging anything, unwinding the ones
// it had already taken, so a listen refused at its last URI must leave nothing
// watching. The check for that is the listen after it: a credential holding ten
// stranded watchers could not be granted an eleventh.
//
// The process-wide ceiling this release adds takes exactly this path, with
// "server-wide" in the message instead of the count. It is not what this test
// reaches, because reaching 512 live watchers over the wire needs 52
// credentials to hold them, so the ceiling itself is pinned in the unit tests
// of internal/subscriptions and cmd/server, and what is proven here is the road
// their refusal travels to the client.
func TestSubscriptionsListen_PastTheWatcherCeiling_IsRefusedWhole(t *testing.T) {
	const meta = `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28",` +
		`"io.modelcontextprotocol/clientCapabilities":{},` +
		`"io.modelcontextprotocol/clientInfo":{"name":"probe","version":"1"}}`
	// One more than DefaultMaxWatchers, which is what a credential may watch.
	const asked = 11

	uris := make([]string, 0, asked)
	for i := range asked {
		uris = append(uris, fmt.Sprintf(`"gitlab://project/%d"`, 900+i))
	}

	gitlab := startFakeGitLabServingProjects(t)
	srv := startServer(t, nil, "--gitlab-url="+gitlab.URL, "--capability-surface=full")

	refused := firstSSEFrame(t, srv, "subscriptions/listen", "2026-07-28",
		`{"jsonrpc":"2.0","id":11,"method":"subscriptions/listen","params":{"notifications":{"resourceSubscriptions":[`+
			strings.Join(uris, ",")+`]},`+meta+`}}`)
	for _, want := range []string{`"code":-32000`, "too many active subscriptions", "limit 10"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(refused, want) {
				t.Errorf("the refusal does not contain %q:\n%s", want, refused)
			}
		})
	}

	granted := firstSSEFrame(t, srv, "subscriptions/listen", "2026-07-28",
		`{"jsonrpc":"2.0","id":12,"method":"subscriptions/listen","params":{"notifications":{"resourceSubscriptions":`+
			`["gitlab://project/950"]},`+meta+`}}`)
	if !strings.Contains(granted, "notifications/subscriptions/acknowledged") {
		t.Errorf("a listen for one resource after the refusal was not acknowledged, so the refused listen left "+
			"watchers behind:\n%s", granted)
	}
}
