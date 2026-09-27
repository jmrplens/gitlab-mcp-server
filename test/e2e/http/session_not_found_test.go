//go:build httpe2e

// session_not_found_test.go holds what the go-sdk client this module is built
// against does when this server answers 404 to a request that carries a
// session ID. It is the third item of issue 961 and row 63 of
// docs/development/upstream-bugs.md.
//
// The 2025-11-25 transport says a client receiving that 404 MUST start a new
// session by sending initialize without a session ID. The server's half is the
// gate's foreign-session refusal, which answers exactly that 404, and its
// comment counts on the client's half to make the refusal heal itself. The
// client's half belongs to go-sdk, so this test drives a real go-sdk client
// against the real binary and pins what it does today. The bump that changes
// it fails here, in its own pull request, with a message saying what moved and
// what to update.
package httpe2e

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// sessionEraProtocol is the revision whose transport carries the MUST this
// test is about. 2026-07-28 has no sessions at all, and a stateful handler
// refuses it at initialize.
const sessionEraProtocol = "2025-11-25"

// sessionNotFoundFix is what a failure here asks the pull request to do, since
// every assertion below fails for the same reason: go-sdk changed what its
// client does with this server's 404.
const sessionNotFoundFix = "Update row 63 of docs/development/upstream-bugs.md and its section, the comment on " +
	"checkSessionOwnership in cmd/server/auth_gate.go, F-22 on row ADM-007 in internal/tenancy/decisions_admit.go " +
	"if the client now recovers, and this test, in this same pull request."

// sentRequest is one HTTP request the client made, as the switching transport
// saw it leave and come back.
type sentRequest struct {
	httpMethod string
	rpcMethod  string
	sessionID  string
	status     int
}

// credentialSwitchingTransport sends every request with whichever credential
// it currently holds, and records what the client sent.
//
// Switching the credential under a live session is the one way a test can make
// this server answer a session's own client with 404 without touching the
// server: the gate binds a session to the credential that opened it, and
// refuses it to any other with the terminated-session status.
type credentialSwitchingTransport struct {
	mu    sync.Mutex
	token string
	sent  []sentRequest
}

// use makes every later request carry token.
func (c *credentialSwitchingTransport) use(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = token
}

// RoundTrip sends a copy of req carrying the current credential, and records
// the JSON-RPC method its body names and the status it was answered with.
func (c *credentialSwitchingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	out := req.Clone(req.Context())
	var rpcMethod string
	if req.Body != nil && req.Body != http.NoBody {
		body, err := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, err
		}
		var msg struct {
			Method string `json:"method"`
		}
		if json.Unmarshal(body, &msg) == nil {
			rpcMethod = msg.Method
		}
		out.Body = io.NopCloser(bytes.NewReader(body))
	}

	c.mu.Lock()
	out.Header.Set("PRIVATE-TOKEN", c.token)
	c.mu.Unlock()

	resp, err := http.DefaultTransport.RoundTrip(out)
	record := sentRequest{httpMethod: out.Method, rpcMethod: rpcMethod, sessionID: out.Header.Get("Mcp-Session-Id")}
	if err == nil {
		record.status = resp.StatusCode
	}
	c.mu.Lock()
	c.sent = append(c.sent, record)
	c.mu.Unlock()
	return resp, err
}

// requests returns a copy of what the client has sent so far.
func (c *credentialSwitchingTransport) requests() []sentRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]sentRequest(nil), c.sent...)
}

// initializes counts the initialize requests among sent.
func initializes(sent []sentRequest) int {
	n := 0
	for _, r := range sent {
		if r.rpcMethod == "initialize" {
			n++
		}
	}
	return n
}

// TestSessionNotFound_GoSDKClient_KeepsTheRefusedSessionAndStartsNoOther pins
// what a go-sdk v1.8.0 client does after this server answers 404 to a request
// carrying its session ID.
//
// Two facts, each held apart so a failure names the one that moved. The client
// starts no new session, which is the MUST go-sdk#1299 reports. And it does not
// even conclude that the session is gone: go-sdk reads a non-2xx body as a
// JSON-RPC error before it looks at the status, and this server's 404, like
// every refusal the gate writes, carries one, so the call fails with that error
// wrapped as a rejection that keeps the connection. ErrSessionMissing, which a
// bare 404 produces, is never raised. The client keeps the dead session ID and
// every later call is refused the same way, until the application reconnects.
//
// The standalone GET stream is disabled so that the only requests after the
// switch are the calls this test makes: the SDK opens that stream on a
// goroutine of its own, so its GET could land after the switch and be refused
// too, and what the client does with a refused standalone stream is a
// different path from the one row 63 describes.
func TestSessionNotFound_GoSDKClient_KeepsTheRefusedSessionAndStartsNoOther(t *testing.T) {
	gitlab := startTokenAwareGitLab(t)
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url, "--stateless=false")

	transport := &credentialSwitchingTransport{token: "glpat-session-owner"}
	client := mcp.NewClient(&mcp.Implementation{Name: "httpe2e-session-not-found", Version: "0"}, nil)
	cs, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{
		Endpoint:             srv.baseURL + "/mcp",
		HTTPClient:           &http.Client{Transport: transport},
		DisableStandaloneSSE: true,
	}, &mcp.ClientSessionOptions{ProtocolVersion: sessionEraProtocol})
	if err != nil {
		t.Fatalf("connecting at %s: %v\n%s", sessionEraProtocol, err, srv.logs())
	}
	t.Cleanup(func() { _ = cs.Close() })

	sessionID := cs.ID()
	if sessionID == "" {
		t.Fatalf("the stateful server minted no session ID, so nothing here can be about one")
	}
	// The control: the owner is served on its own session, so what follows is
	// about the switch and not about a session that never worked.
	if _, err = cs.ListTools(t.Context(), nil); err != nil {
		t.Fatalf("the session's owner was refused its own session: %v\n%s", err, srv.logs())
	}

	transport.use("glpat-another-credential")
	before := len(transport.requests())
	_, first := cs.ListTools(t.Context(), nil)
	_, second := cs.ListTools(t.Context(), nil)
	after := transport.requests()[before:]
	t.Logf("go-sdk's client after this server's 404: first call %v; second call %v", first, second)

	if first == nil || second == nil {
		t.Fatalf("a call under another credential succeeded (first: %v, second: %v), so go-sdk now recovers from this server's 404 on its own. %s",
			first, second, sessionNotFoundFix)
	}
	if got := initializes(transport.requests()); got != 1 {
		t.Errorf("the client sent %d initialize requests, want 1: go-sdk now starts a new session after a 404, which is the fix go-sdk#1299 asks for. %s",
			got, sessionNotFoundFix)
	}
	if errors.Is(first, mcp.ErrSessionMissing) {
		t.Errorf("the refused call returned ErrSessionMissing (%v): go-sdk now reads the 404 before the JSON-RPC error its body carries, so a client learns the session is gone. %s",
			first, sessionNotFoundFix)
	}
	if _, ok := errors.AsType[*jsonrpc.Error](first); !ok {
		t.Errorf("the refused call returned %v, want the JSON-RPC error the 404's body carries: go-sdk no longer surfaces that body. %s",
			first, sessionNotFoundFix)
	}

	// Both refused calls went out on the dead session and were answered 404,
	// and nothing else was sent: the client neither dropped the session ID nor
	// asked for a new one.
	if len(after) != 2 {
		t.Fatalf("after the switch the client sent %d requests, want the 2 calls this test made: %+v. %s", len(after), after, sessionNotFoundFix)
	}
	for i, r := range after {
		if r.rpcMethod != "tools/list" || r.sessionID != sessionID || r.status != http.StatusNotFound {
			t.Errorf("request %d after the switch was %s %q on session %q answered %d, want tools/list on the refused session %q answered 404. %s",
				i+1, r.httpMethod, r.rpcMethod, r.sessionID, r.status, sessionID, sessionNotFoundFix)
		}
	}
}
