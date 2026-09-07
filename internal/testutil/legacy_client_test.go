// legacy_client_test.go covers the failure paths of the fake legacy client,
// none of which a passing run takes: the transport is a net.Pipe in this
// process, so it connects, writes and closes without ever failing. They are
// reached here by replacing the calls that cannot fail and by reporting through
// a recorder instead of the test.
package testutil

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// recordingLegacyReporter stands in for *testing.T so a failure the fake client
// reports is recorded rather than failing the test that provoked it. Its
// Fatalf does not abort, which is why every reporting site in the client
// returns as well.
type recordingLegacyReporter struct {
	mu       sync.Mutex
	messages []string
	cleanups []func()
}

// Helper satisfies the reporter and does nothing: there is no test goroutine to
// attribute a line to.
func (*recordingLegacyReporter) Helper() {}

// Cleanup keeps the function so a test can run it when it chooses, which is
// what makes the cleanup path itself observable.
func (r *recordingLegacyReporter) Cleanup(fn func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cleanups = append(r.cleanups, fn)
}

// Error records what would have been reported.
func (r *recordingLegacyReporter) Error(args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = append(r.messages, fmt.Sprint(args...))
}

// Fatalf records what would have been reported.
func (r *recordingLegacyReporter) Fatalf(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = append(r.messages, fmt.Sprintf(format, args...))
}

// joined returns every recorded message as one string for substring assertions.
func (r *recordingLegacyReporter) joined() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.messages, "\n")
}

// runCleanups runs what was registered, newest first, the way testing does.
func (r *recordingLegacyReporter) runCleanups() {
	r.mu.Lock()
	registered := r.cleanups
	r.cleanups = nil
	r.mu.Unlock()
	for _, cleanup := range slices.Backward(registered) {
		cleanup()
	}
}

// fakeConn is a connection whose reads, writes and closes are whatever the test
// needs them to be. Write failures are counted so a test can fail the second
// one, which is the notification the handshake ends with.
type fakeConn struct {
	mu        sync.Mutex
	writes    int
	failWrite int
	writeErr  error
	messages  []jsonrpc.Message
	readErr   error
	blockRead chan struct{}
}

// Read returns the queued messages in order, then the configured error, and
// blocks until released when the test asked it to.
func (c *fakeConn) Read(ctx context.Context) (jsonrpc.Message, error) {
	c.mu.Lock()
	if len(c.messages) > 0 {
		msg := c.messages[0]
		c.messages = c.messages[1:]
		c.mu.Unlock()
		return msg, nil
	}
	block := c.blockRead
	err := c.readErr
	c.mu.Unlock()
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
		}
	}
	return nil, err
}

// Write fails the nth call when the test asked for it, and otherwise succeeds.
func (c *fakeConn) Write(context.Context, jsonrpc.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writes++
	if c.failWrite == c.writes {
		return c.writeErr
	}
	return nil
}

// Close satisfies [mcp.Connection] and releases nothing: a test that wants the
// serve goroutine to end releases blockRead itself.
func (*fakeConn) Close() error { return nil }

// SessionID satisfies [mcp.Connection].
func (*fakeConn) SessionID() string { return "fake" }

// TestConnectLegacyElicitationClient_TransportFailures_AreReported verifies
// that a connection this helper could not make is reported rather than handed
// back half built, for either end of the in-memory pair.
func TestConnectLegacyElicitationClient_TransportFailures_AreReported(t *testing.T) {
	cases := []struct {
		name    string
		arrange func(*testing.T)
		want    string
	}{
		{
			name: "the server end",
			arrange: func(t *testing.T) {
				t.Helper()
				original := connectSession
				connectSession = func(context.Context, *mcp.Server, mcp.Transport) (*mcp.ServerSession, error) {
					return nil, errors.New("server refused")
				}
				t.Cleanup(func() { connectSession = original })
			},
			want: "server connect: server refused",
		},
		{
			name: "the client end",
			arrange: func(t *testing.T) {
				t.Helper()
				original := connectClient
				connectClient = func(context.Context, mcp.Transport) (mcp.Connection, error) {
					return nil, errors.New("client refused")
				}
				t.Cleanup(func() { connectClient = original })
			},
			want: "transport connect: client refused",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.arrange(t)
			reporter := &recordingLegacyReporter{}

			session := connectLegacyElicitationClient(t.Context(), reporter, testServer(), acceptingHandler, LegacyClientOptions{})

			if session != nil {
				t.Error("a session was returned for a connection that failed")
			}
			if !strings.Contains(reporter.joined(), testCase.want) {
				t.Errorf("report = %q, want it to contain %q", reporter.joined(), testCase.want)
			}
		})
	}
}

// TestLegacyHandshake_Failures_AreReported verifies that every step of the
// initialize exchange reports the step it was, which is the only thing a reader
// has to go on when a fake client will not connect.
func TestLegacyHandshake_Failures_AreReported(t *testing.T) {
	cases := []struct {
		name    string
		arrange func(*testing.T) *fakeConn
		want    string
	}{
		{
			name: "the params do not marshal",
			arrange: func(t *testing.T) *fakeConn {
				t.Helper()
				original := marshalInitParams
				marshalInitParams = func(any) ([]byte, error) { return nil, errors.New("no encoding") }
				t.Cleanup(func() { marshalInitParams = original })
				return &fakeConn{}
			},
			want: "marshal initialize params: no encoding",
		},
		{
			name: "the request id is refused",
			arrange: func(t *testing.T) *fakeConn {
				t.Helper()
				original := makeRequestID
				makeRequestID = func(any) (jsonrpc.ID, error) { return jsonrpc.ID{}, errors.New("no id") }
				t.Cleanup(func() { makeRequestID = original })
				return &fakeConn{}
			},
			want: "make id: no id",
		},
		{
			name: "the initialize request cannot be written",
			arrange: func(t *testing.T) *fakeConn {
				t.Helper()
				return &fakeConn{failWrite: 1, writeErr: errors.New("pipe closed")}
			},
			want: "write initialize: pipe closed",
		},
		{
			name: "the response never arrives",
			arrange: func(t *testing.T) *fakeConn {
				t.Helper()
				return &fakeConn{readErr: errors.New("pipe closed")}
			},
			want: "read during handshake: pipe closed",
		},
		{
			name: "the initialized notification cannot be written",
			arrange: func(t *testing.T) *fakeConn {
				t.Helper()
				return &fakeConn{
					failWrite: 2,
					writeErr:  errors.New("pipe closed"),
					messages:  []jsonrpc.Message{&jsonrpc.Response{}},
				}
			},
			want: "write initialized notification: pipe closed",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			conn := testCase.arrange(t)
			reporter := &recordingLegacyReporter{}

			if legacyHandshake(t.Context(), reporter, conn, acceptingHandler, LegacyClientOptions{}) {
				t.Error("the handshake reported success after a failure")
			}
			if !strings.Contains(reporter.joined(), testCase.want) {
				t.Errorf("report = %q, want it to contain %q", reporter.joined(), testCase.want)
			}
		})
	}
}

// TestConnectLegacyElicitationClient_FailedHandshake_ReturnsNoSession verifies
// that a handshake which did not complete stops there: serving a connection the
// server never initialized would answer requests on behalf of a client that
// does not exist.
func TestConnectLegacyElicitationClient_FailedHandshake_ReturnsNoSession(t *testing.T) {
	original := connectClient
	connectClient = func(context.Context, mcp.Transport) (mcp.Connection, error) {
		return &fakeConn{failWrite: 1, writeErr: errors.New("pipe closed")}, nil
	}
	t.Cleanup(func() { connectClient = original })
	reporter := &recordingLegacyReporter{}

	session := connectLegacyElicitationClient(t.Context(), reporter, testServer(), acceptingHandler, LegacyClientOptions{})

	if session != nil {
		t.Error("a session was returned for a handshake that failed")
	}
	if !strings.Contains(reporter.joined(), "write initialize") {
		t.Errorf("report = %q, want the handshake step that failed", reporter.joined())
	}
}

// TestAwaitLegacyInitializeResponse_ErrorResponse_IsReported verifies the one
// handshake failure that is the server's answer rather than the transport's: an
// initialize the server refused has to name that refusal, or the helper looks
// like it hung.
func TestAwaitLegacyInitializeResponse_ErrorResponse_IsReported(t *testing.T) {
	conn := &fakeConn{messages: []jsonrpc.Message{
		&jsonrpc.Response{Error: &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: "unsupported protocol version"}},
	}}

	err := awaitLegacyInitializeResponse(t.Context(), conn, acceptingHandler)

	if err == nil || !strings.Contains(err.Error(), "unsupported protocol version") {
		t.Errorf("awaitLegacyInitializeResponse() error = %v, want the server's refusal", err)
	}
}

// TestAwaitLegacyInitializeResponse_InterleavedRequest_IsAnswered verifies that
// a server which asks something before answering initialize is served rather
// than ignored: an unanswered request leaves the server blocked until its own
// timeout, which presents as a hang with no message.
func TestAwaitLegacyInitializeResponse_InterleavedRequest_IsAnswered(t *testing.T) {
	id, err := jsonrpc.MakeID("ping-1")
	if err != nil {
		t.Fatalf("MakeID: %v", err)
	}
	conn := &fakeConn{messages: []jsonrpc.Message{
		&jsonrpc.Request{ID: id, Method: "ping", Params: json.RawMessage("{}")},
		&jsonrpc.Response{},
	}}

	if handshakeErr := awaitLegacyInitializeResponse(t.Context(), conn, acceptingHandler); handshakeErr != nil {
		t.Fatalf("awaitLegacyInitializeResponse() error = %v", handshakeErr)
	}
	if conn.writes != 1 {
		t.Errorf("the client wrote %d message(s), want the ping acknowledged", conn.writes)
	}
}

// TestServeLegacyClient_NonRequestMessage_IsSkipped verifies that a response
// arriving on the connection is stepped over rather than treated as a request,
// and that the loop ends when the connection does.
func TestServeLegacyClient_NonRequestMessage_IsSkipped(t *testing.T) {
	conn := &fakeConn{
		messages: []jsonrpc.Message{&jsonrpc.Response{}},
		readErr:  errors.New("closed"),
	}

	serveLegacyClient(t.Context(), conn, acceptingHandler)

	if conn.writes != 0 {
		t.Errorf("the client wrote %d message(s), want none for a response", conn.writes)
	}
}

// TestRespondLegacyRequest_EveryMethod_GetsTheRightAnswer covers what the fake
// client answers, including the elicitation failures a handler can produce: a
// request answered with the wrong thing, or not at all, blocks the server until
// its own timeout.
func TestRespondLegacyRequest_EveryMethod_GetsTheRightAnswer(t *testing.T) {
	id, err := jsonrpc.MakeID("req-1")
	if err != nil {
		t.Fatalf("MakeID: %v", err)
	}

	cases := []struct {
		name       string
		request    *jsonrpc.Request
		handler    ElicitHandlerFunc
		wantWrites int
		wantCode   int64
	}{
		{
			name:       "a notification is ignored",
			request:    &jsonrpc.Request{Method: "notifications/progress"},
			handler:    acceptingHandler,
			wantWrites: 0,
		},
		{
			name:       "unparseable elicitation params",
			request:    &jsonrpc.Request{ID: id, Method: "elicitation/create", Params: json.RawMessage("not json")},
			handler:    acceptingHandler,
			wantWrites: 1,
			wantCode:   jsonrpc.CodeInvalidParams,
		},
		{
			name:    "a handler that refuses",
			request: &jsonrpc.Request{ID: id, Method: "elicitation/create", Params: json.RawMessage(`{"message":"ok?"}`)},
			handler: func(context.Context, *mcp.ElicitParams) (*mcp.ElicitResult, error) {
				return nil, errors.New("the user walked away")
			},
			wantWrites: 1,
			wantCode:   jsonrpc.CodeInternalError,
		},
		{
			name:    "a result that does not marshal",
			request: &jsonrpc.Request{ID: id, Method: "elicitation/create", Params: json.RawMessage(`{"message":"ok?"}`)},
			handler: func(context.Context, *mcp.ElicitParams) (*mcp.ElicitResult, error) {
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"cycle": make(chan int)}}, nil
			},
			wantWrites: 1,
			wantCode:   jsonrpc.CodeInternalError,
		},
		{
			name:       "an accepted elicitation",
			request:    &jsonrpc.Request{ID: id, Method: "elicitation/create", Params: json.RawMessage(`{"message":"ok?"}`)},
			handler:    acceptingHandler,
			wantWrites: 1,
		},
		{
			name:       "a ping",
			request:    &jsonrpc.Request{ID: id, Method: "ping", Params: json.RawMessage("{}")},
			handler:    acceptingHandler,
			wantWrites: 1,
		},
		{
			name:       "a method the client does not implement",
			request:    &jsonrpc.Request{ID: id, Method: "sampling/createMessage"},
			handler:    acceptingHandler,
			wantWrites: 1,
			wantCode:   jsonrpc.CodeMethodNotFound,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			conn := &writeCapturingConn{}

			respondLegacyRequest(t.Context(), conn, testCase.request, testCase.handler)

			if len(conn.written) != testCase.wantWrites {
				t.Fatalf("wrote %d message(s), want %d", len(conn.written), testCase.wantWrites)
			}
			if testCase.wantWrites == 0 {
				return
			}
			response, ok := conn.written[0].(*jsonrpc.Response)
			if !ok {
				t.Fatalf("wrote %T, want a response", conn.written[0])
			}
			if testCase.wantCode == 0 {
				if response.Error != nil {
					t.Errorf("response carries error %v, want a result", response.Error)
				}
				return
			}
			var rpcErr *jsonrpc.Error
			if !errors.As(response.Error, &rpcErr) || rpcErr.Code != testCase.wantCode {
				t.Errorf("response error = %v, want code %d", response.Error, testCase.wantCode)
			}
		})
	}
}

// TestElicitationCapability_URLMode_IsAdvertisedOnRequest verifies the option
// that decides whether the fake client claims URL elicitation, which is what a
// server consults before choosing that mode.
func TestElicitationCapability_URLMode_IsAdvertisedOnRequest(t *testing.T) {
	cases := []struct {
		name    string
		opts    LegacyClientOptions
		wantURL bool
	}{
		{name: "form only", opts: LegacyClientOptions{}, wantURL: false},
		{name: "form and url", opts: LegacyClientOptions{URLElicitation: true}, wantURL: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			capability := elicitationCapability(testCase.opts)

			if _, ok := capability["form"]; !ok {
				t.Error("the capability does not advertise form elicitation")
			}
			if _, ok := capability["url"]; ok != testCase.wantURL {
				t.Errorf("url advertised = %v, want %v", ok, testCase.wantURL)
			}
		})
	}
}

// TestAwaitServeExit_GoroutineThatNeverEnds_IsReported verifies the wait that
// does not end. A serve goroutine still reading a connection nobody closed
// would otherwise be carried silently into the next test.
func TestAwaitServeExit_GoroutineThatNeverEnds_IsReported(t *testing.T) {
	original := serveExitTimeout
	serveExitTimeout = time.Millisecond
	t.Cleanup(func() { serveExitTimeout = original })
	reporter := &recordingLegacyReporter{}

	awaitServeExit(reporter, make(chan struct{}))

	if !strings.Contains(reporter.joined(), "serve goroutine did not exit") {
		t.Errorf("report = %q, want the goroutine that did not exit", reporter.joined())
	}
}

// TestConnectLegacyElicitationClient_Cleanup_JoinsTheServeGoroutine verifies
// the whole assembly on the fake connection: the handshake completes, the serve
// goroutine starts, and the cleanup closes the connection and waits for it,
// reporting nothing because it ended.
func TestConnectLegacyElicitationClient_Cleanup_JoinsTheServeGoroutine(t *testing.T) {
	released := make(chan struct{})
	conn := &fakeConn{
		messages:  []jsonrpc.Message{&jsonrpc.Response{}},
		readErr:   errors.New("closed"),
		blockRead: released,
	}
	original := connectClient
	connectClient = func(context.Context, mcp.Transport) (mcp.Connection, error) { return conn, nil }
	t.Cleanup(func() { connectClient = original })
	reporter := &recordingLegacyReporter{}

	session := connectLegacyElicitationClient(t.Context(), reporter, testServer(), acceptingHandler, LegacyClientOptions{})
	if session == nil {
		t.Fatal("no session was returned for a handshake that succeeded")
	}
	close(released)
	reporter.runCleanups()

	if reporter.joined() != "" {
		t.Errorf("report = %q, want nothing from a clean shutdown", reporter.joined())
	}
}

// writeCapturingConn records what the client wrote and reads nothing.
type writeCapturingConn struct {
	written []jsonrpc.Message
}

// Read reports the connection as finished, which is all these tests need.
func (*writeCapturingConn) Read(context.Context) (jsonrpc.Message, error) {
	return nil, errors.New("closed")
}

// Write records the message.
func (c *writeCapturingConn) Write(_ context.Context, msg jsonrpc.Message) error {
	c.written = append(c.written, msg)
	return nil
}

// Close satisfies [mcp.Connection].
func (*writeCapturingConn) Close() error { return nil }

// SessionID satisfies [mcp.Connection].
func (*writeCapturingConn) SessionID() string { return "capturing" }

// testServer is the server end these tests connect, which answers nothing
// because every case ends before a call reaches it.
func testServer() *mcp.Server {
	return mcp.NewServer(&mcp.Implementation{Name: "legacy-test-server", Version: "0.0.1"}, nil)
}

// acceptingHandler accepts every elicitation, which is the shape a test that
// cares about something else wants.
func acceptingHandler(context.Context, *mcp.ElicitParams) (*mcp.ElicitResult, error) {
	return &mcp.ElicitResult{Action: "accept"}, nil
}
