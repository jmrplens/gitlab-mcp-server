//go:build httpe2e

// session_not_found_test.go holds what the go-sdk client this module is built
// against does when this server answers 404 to a request that carries a
// session ID. It is the third item of issue 961 and row 68 of
// docs/development/upstream-bugs.md.
//
// The 2025-11-25 transport says a client receiving that 404 MUST start a new
// session by sending initialize without a session ID. The server's half is the
// gate's session refusal, which answers exactly that 404 both to a session
// presented by another credential and to one the server no longer holds, and
// its comment counts on the client's half to make the refusal heal itself. The
// client's half belongs to go-sdk, so these tests drive a real go-sdk client
// against the real binary and pin what it does today. The bump that changes it
// fails here, in its own pull request, with a message saying what moved and
// what to update.
//
// What the client does depends on which request meets the 404, because the
// gate writes a JSON-RPC error body on every refusal and can name the request
// only when the request carries an id. A call carrying one gets a body go-sdk
// decodes, and the client keeps the dead session and surfaces the gate's words.
// A request carrying none, the standalone stream's GET and every notification,
// gets a body go-sdk cannot decode, and the client fails the connection with
// ErrSessionMissing. Neither starts a new session.
package httpe2e

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// sessionEraProtocol is the revision whose transport carries the MUST these
// tests are about. 2026-07-28 has no sessions at all, and a stateful handler
// refuses it at initialize.
const sessionEraProtocol = "2025-11-25"

// sessionNotFoundFix is what a failure here asks the pull request to do, since
// every assertion below fails for the same reason: go-sdk changed what its
// client does with this server's 404.
const sessionNotFoundFix = "Update row 68 of docs/development/upstream-bugs.md and its section, the comment on " +
	"checkSessionOwnership in cmd/server/auth_gate.go, F-22 on row ADM-007 in internal/tenancy/decisions_admit.go " +
	"if the client now recovers, the statements of what a client does once its session is gone (the idle timeout " +
	"and pool eviction steps of the session lifecycle list in site/src/content/docs/operations/http-server.mdx and " +
	"its es copy, and ADR-0020's eviction section and NEG-005), " +
	"and these tests, in this same pull request."

// gateRefusalFix is what a failure about the refusal's own code or words asks
// for: that is this server's text, which row 68 quotes, and not go-sdk's.
const gateRefusalFix = "The gate's session refusal changed: update the quotation of it in row 68 of " +
	"docs/development/upstream-bugs.md and these tests' expectation of it, in this same pull request."

// Gate refusal the tests expect to reach the application unchanged on the
// path that decodes it: the code and a phrase of the message
// sessionOwnershipFailure writes in cmd/server/auth_gate.go. They are restated
// rather than imported, because this module drives the binary from outside.
const (
	gateSessionRefusalCode   = -32600
	gateSessionRefusalPhrase = "does not belong to the presented credential"
)

// sessionIdleTimeout is the --session-timeout the expiry tests start the
// server with, and sessionExpiryWait how long they stay idle so the server has
// certainly closed the session and forgotten its owner. The margin is wide on
// purpose: nothing a test can observe without touching the session says it
// has expired, and a request that touches it before it expires restarts the
// timer.
const (
	sessionIdleTimeout = 2 * time.Second
	sessionExpiryWait  = 3 * sessionIdleTimeout
)

// sessionFailureBound is how long a test waits for the client to fail its
// connection after the server ended the session: the idle timeout, then
// go-sdk's first reconnect delay of the standalone stream (one to two seconds),
// with room left over.
const sessionFailureBound = 30 * time.Second

// sentRequest is one HTTP request the client made, as the recording transport
// saw it leave and come back.
type sentRequest struct {
	httpMethod string
	rpcMethod  string
	sessionID  string
	status     int
}

// sessionRecordingTransport sends every request with whichever credential it
// currently holds, and records what the client sent.
//
// Switching the credential under a live session is how the foreign-session
// test makes the gate refuse a session that still exists: the gate binds a
// session to the credential that opened it, and refuses it to any other with
// the terminated-session status. The expiry tests never switch it.
type sessionRecordingTransport struct {
	mu    sync.Mutex
	token string
	sent  []sentRequest
}

// use makes every later request carry token.
func (c *sessionRecordingTransport) use(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = token
}

// RoundTrip sends a copy of req carrying the current credential, and records
// the JSON-RPC method its body names and the status it was answered with.
func (c *sessionRecordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
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
func (c *sessionRecordingTransport) requests() []sentRequest {
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

// protocolErrorBodiesIgnored reports whether this test process runs go-sdk
// with MCPGODEBUG=noprotocolerrorbody=1, the compatibility switch that makes
// its client ignore the body of a non-2xx response. go-sdk reads the variable
// once, when its package initializes, and splits it the same way.
func protocolErrorBodiesIgnored() bool {
	for part := range strings.SplitSeq(os.Getenv("MCPGODEBUG"), ",") {
		key, value, _ := strings.Cut(part, "=")
		if strings.TrimSpace(key) == "noprotocolerrorbody" && strings.TrimSpace(value) == "1" {
			return true
		}
	}
	return false
}

// skipWhereProtocolErrorBodiesAreIgnored skips a test about the decoded body
// when the switch that turns the decoding off is set, so a developer who set
// it is told why rather than shown failures blaming go-sdk.
func skipWhereProtocolErrorBodiesAreIgnored(t *testing.T) {
	t.Helper()
	if protocolErrorBodiesIgnored() {
		t.Skip("MCPGODEBUG=noprotocolerrorbody=1 turns off the body decoding this test pins: a call carrying an id then " +
			"takes the path a request carrying none takes, which the stream and notification cases of " +
			"TestSessionNotFound_GoSDKClient_AfterTheSessionExpires pin")
	}
}

// newSessionClient is the go-sdk client every test here connects, with the
// SDK's default options: what is pinned is what a client gets without asking.
func newSessionClient() *mcp.Client {
	return mcp.NewClient(&mcp.Implementation{Name: "httpe2e-session-not-found", Version: "0"}, nil)
}

// connectAtSessionEra connects client to srv's MCP endpoint at 2025-11-25
// through transport, checks that the server minted a session, and makes one
// call the session's owner must be served, so that whatever follows is about
// the refusal and not about a session that never worked.
func connectAtSessionEra(t *testing.T, srv *server, client *mcp.Client, transport *sessionRecordingTransport, standaloneStream bool) (*mcp.ClientSession, string) {
	t.Helper()
	cs, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{
		Endpoint:             srv.baseURL + "/mcp",
		HTTPClient:           &http.Client{Transport: transport},
		DisableStandaloneSSE: !standaloneStream,
	}, &mcp.ClientSessionOptions{ProtocolVersion: sessionEraProtocol})
	if err != nil {
		t.Fatalf("connecting at %s: %v\n%s", sessionEraProtocol, err, srv.logs())
	}
	t.Cleanup(func() { _ = cs.Close() })

	sessionID := cs.ID()
	if sessionID == "" {
		t.Fatalf("the stateful server minted no session ID, so nothing here can be about one")
	}
	if _, err = cs.ListTools(t.Context(), nil); err != nil {
		t.Fatalf("the session's owner was refused its own session: %v\n%s", err, srv.logs())
	}
	return cs, sessionID
}

// assertKeptOnTheRefusedSession makes two calls and asserts what go-sdk does
// when the gate answers each with a 404 whose body is a JSON-RPC error naming
// the call: the call fails with that error, the connection is kept, no new
// session is started, and the second call goes out on the same dead session
// ID to be refused the same way.
func assertKeptOnTheRefusedSession(t *testing.T, cs *mcp.ClientSession, transport *sessionRecordingTransport, sessionID string) {
	t.Helper()
	before := len(transport.requests())
	_, first := cs.ListTools(t.Context(), nil)
	_, second := cs.ListTools(t.Context(), nil)
	after := transport.requests()[before:]
	t.Logf("go-sdk's client after this server's 404 to a call: first call %v; second call %v", first, second)

	if first == nil || second == nil {
		t.Fatalf("a call on the refused session succeeded (first: %v, second: %v), so go-sdk now recovers from this server's 404 on its own. %s",
			first, second, sessionNotFoundFix)
	}
	if got := initializes(transport.requests()); got != 1 {
		t.Errorf("the client sent %d initialize requests, want 1: go-sdk now starts a new session after a 404 to a call, which is the fix go-sdk#1299 asks for. %s",
			got, sessionNotFoundFix)
	}
	if errors.Is(first, mcp.ErrSessionMissing) {
		t.Errorf("the refused call returned ErrSessionMissing (%v): go-sdk now reads the 404 before the JSON-RPC error its body carries, so a client learns the session is gone. %s",
			first, sessionNotFoundFix)
	}
	refusal, ok := errors.AsType[*jsonrpc.Error](first)
	if !ok {
		t.Errorf("the refused call returned %v, want the JSON-RPC error the 404's body carries: go-sdk no longer surfaces that body. %s",
			first, sessionNotFoundFix)
	} else if refusal.Code != gateSessionRefusalCode || !strings.Contains(refusal.Message, gateSessionRefusalPhrase) {
		t.Errorf("the refused call returned JSON-RPC error %d %q, want the gate's %d carrying %q: either go-sdk now makes an error of its own for a 404, or the gate's words changed. %s %s",
			refusal.Code, refusal.Message, gateSessionRefusalCode, gateSessionRefusalPhrase, sessionNotFoundFix, gateRefusalFix)
	}

	if len(after) != 2 {
		t.Fatalf("after the refusal the client sent %d requests, want the 2 calls this test made: %+v. %s", len(after), after, sessionNotFoundFix)
	}
	for i, r := range after {
		if r.rpcMethod != "tools/list" || r.sessionID != sessionID || r.status != http.StatusNotFound {
			t.Errorf("request %d after the refusal was %s %q on session %q answered %d, want tools/list on the refused session %q answered 404. %s",
				i+1, r.httpMethod, r.rpcMethod, r.sessionID, r.status, sessionID, sessionNotFoundFix)
		}
	}
}

// awaitConnectionFailure waits for cs to fail its connection and returns the
// error it failed with. It fails the test when the client starts a new session
// instead, or when neither happens within sessionFailureBound.
//
// cs.Wait is called on a goroutine of its own because it blocks until the
// connection ends; the goroutine only sends on a buffered channel, and the
// session's cleanup closes the connection, so it ends whatever the test does.
func awaitConnectionFailure(t *testing.T, cs *mcp.ClientSession, transport *sessionRecordingTransport, what string) error {
	t.Helper()
	ended := make(chan error, 1)
	go func() { ended <- cs.Wait() }()

	poll := time.NewTicker(100 * time.Millisecond)
	defer poll.Stop()
	deadline := time.After(sessionFailureBound)
	for {
		select {
		case err := <-ended:
			return err
		case <-poll.C:
			if got := initializes(transport.requests()); got > 1 {
				t.Fatalf("after %s the client sent %d initialize requests: go-sdk now starts a new session after this server's 404, which is the fix go-sdk#1299 asks for. %s",
					what, got, sessionNotFoundFix)
			}
		case <-deadline:
			t.Fatalf("the connection neither failed nor started a new session within %s of %s: %+v. %s",
				sessionFailureBound, what, transport.requests(), sessionNotFoundFix)
		}
	}
}

// assertFailedWithSessionMissing checks what follows a connection failure
// caused by a 404 go-sdk could not decode: the failure wraps ErrSessionMissing,
// every later call fails with ErrConnectionClosed without being sent, and only
// the one initialize was ever sent. A later call does not wrap
// ErrSessionMissing itself: go-sdk formats the cause into its text, so an
// application that tests for the sentinel finds it on ClientSession.Wait and
// not on the call.
func assertFailedWithSessionMissing(t *testing.T, cs *mcp.ClientSession, transport *sessionRecordingTransport, failure error, what string) {
	t.Helper()
	if !errors.Is(failure, mcp.ErrSessionMissing) {
		t.Errorf("after %s the connection ended with %v, want an error wrapping ErrSessionMissing: go-sdk no longer reads an undecodable 404 as the session gone. %s",
			what, failure, sessionNotFoundFix)
	}
	sent := len(transport.requests())
	_, err := cs.ListTools(t.Context(), nil)
	if !errors.Is(err, mcp.ErrConnectionClosed) {
		t.Errorf("a call after the failure returned %v, want an error wrapping ErrConnectionClosed: the connection no longer stays failed. %s", err, sessionNotFoundFix)
	}
	if got := len(transport.requests()); got != sent {
		t.Errorf("a call after the failure sent %d requests, want none: the connection is no longer failed. %s", got-sent, sessionNotFoundFix)
	}
	if got := initializes(transport.requests()); got != 1 {
		t.Errorf("the client sent %d initialize requests, want 1. %s", got, sessionNotFoundFix)
	}
}

// refusedAfter reports whether sent holds, at or after index from, a request
// with httpMethod and rpcMethod on sessionID answered 404.
func refusedAfter(sent []sentRequest, from int, httpMethod, rpcMethod, sessionID string) bool {
	for _, r := range sent[from:] {
		if r.httpMethod == httpMethod && r.rpcMethod == rpcMethod && r.sessionID == sessionID && r.status == http.StatusNotFound {
			return true
		}
	}
	return false
}

// TestSessionNotFound_GoSDKClient_ForeignCredential_KeepsTheRefusedSession pins
// what a go-sdk v1.8.0 client does after this server refuses its live session
// because the credential presented with it is not the one that opened it.
//
// The call carries an id, so the gate's 404 carries a JSON-RPC error go-sdk
// decodes before it looks at the status: the call fails with that error
// wrapped as a rejection that keeps the connection, ErrSessionMissing is never
// raised, and the client keeps the refused session ID for every later call
// until the application reconnects.
//
// The standalone GET stream is disabled so that the only requests after the
// switch are the calls this test makes: the SDK opens that stream on a
// goroutine of its own, and what the client does with a refused stream is the
// other path, which the expiry test pins.
func TestSessionNotFound_GoSDKClient_ForeignCredential_KeepsTheRefusedSession(t *testing.T) {
	skipWhereProtocolErrorBodiesAreIgnored(t)
	gitlab := startTokenAwareGitLab(t)
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url, "--stateless=false")

	transport := &sessionRecordingTransport{token: "glpat-session-owner"}
	cs, sessionID := connectAtSessionEra(t, srv, newSessionClient(), transport, false)

	transport.use("glpat-another-credential")
	assertKeptOnTheRefusedSession(t, cs, transport, sessionID)
}

// TestSessionNotFound_GoSDKClient_AfterTheSessionExpires pins what a go-sdk
// v1.8.0 client does when the server has ended its session, which is what an
// idle timeout, an eviction and a restart all come to: the session ID is
// unknown, and the gate answers any request carrying it with 404. The idle
// timeout is the one a test can trigger with the owner's own credential and
// nothing else, so each case starts the clock with --session-timeout and waits
// it out.
//
// Which request meets the 404 decides the outcome, and each case holds one:
//
//   - a call, which carries an id: the body is a JSON-RPC error go-sdk decodes,
//     the connection is kept and the client stays on the dead session, exactly
//     as with a foreign credential; the application is also told the session
//     does not belong to its credential, which is the gate's one message for
//     every session it will not serve;
//   - the standalone GET stream, which a default client keeps open: the
//     server's closing of the session ends it, the client reconnects, and the
//     404 to that GET carries a body with no id, which go-sdk cannot decode,
//     so the connection fails with ErrSessionMissing;
//   - a notification, which carries no id either: the same undecodable body,
//     the same failure.
//
// None of the three starts a new session.
func TestSessionNotFound_GoSDKClient_AfterTheSessionExpires(t *testing.T) {
	t.Parallel()
	gitlab := startTokenAwareGitLab(t)
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url, "--stateless=false",
		fmt.Sprintf("--session-timeout=%s", sessionIdleTimeout))

	t.Run("a call keeps the refused session", func(t *testing.T) {
		t.Parallel()
		skipWhereProtocolErrorBodiesAreIgnored(t)
		transport := &sessionRecordingTransport{token: "glpat-expired-call"}
		cs, sessionID := connectAtSessionEra(t, srv, newSessionClient(), transport, false)

		time.Sleep(sessionExpiryWait)
		assertKeptOnTheRefusedSession(t, cs, transport, sessionID)
	})

	t.Run("the standalone stream fails the connection with ErrSessionMissing", func(t *testing.T) {
		t.Parallel()
		transport := &sessionRecordingTransport{token: "glpat-expired-stream"}
		cs, sessionID := connectAtSessionEra(t, srv, newSessionClient(), transport, true)
		from := len(transport.requests())

		const what = "the server ended the session under an open standalone stream"
		failure := awaitConnectionFailure(t, cs, transport, what)
		t.Logf("go-sdk's client after the 404 to its standalone stream: %v", failure)
		if !refusedAfter(transport.requests(), from, http.MethodGet, "", sessionID) {
			t.Errorf("the connection ended without a GET on session %q answered 404: %+v. %s",
				sessionID, transport.requests()[from:], sessionNotFoundFix)
		}
		assertFailedWithSessionMissing(t, cs, transport, failure, what)
	})

	t.Run("a notification fails the connection with ErrSessionMissing", func(t *testing.T) {
		t.Parallel()
		transport := &sessionRecordingTransport{token: "glpat-expired-notification"}
		cs, sessionID := connectAtSessionEra(t, srv, newSessionClient(), transport, false)

		time.Sleep(sessionExpiryWait)
		from := len(transport.requests())
		// A progress notification is one the client sends when its application
		// asks, and the send is synchronous, so its error is the 404's.
		sent := cs.NotifyProgress(t.Context(), &mcp.ProgressNotificationParams{ProgressToken: "issue-961", Progress: 1})
		if !errors.Is(sent, mcp.ErrSessionMissing) {
			t.Errorf("the notification on the ended session returned %v, want an error wrapping ErrSessionMissing. %s", sent, sessionNotFoundFix)
		}

		const what = "a notification on the ended session"
		failure := awaitConnectionFailure(t, cs, transport, what)
		t.Logf("go-sdk's client after the 404 to its notification: %v", failure)
		if !refusedAfter(transport.requests(), from, http.MethodPost, "notifications/progress", sessionID) {
			t.Errorf("the connection ended without notifications/progress on session %q answered 404: %+v. %s",
				sessionID, transport.requests()[from:], sessionNotFoundFix)
		}
		assertFailedWithSessionMissing(t, cs, transport, failure, what)
	})
}
