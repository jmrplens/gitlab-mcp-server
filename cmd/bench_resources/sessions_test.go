// sessions_test.go covers the stateful-session mode: the flags it reads, the
// client that opens, pings and ends a session, how a step files each open and
// counts the sessions the server still holds, and the whole ladder driven
// against the stand-in.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// sessionFake is a stateful transport in the test process, with a switch for
// every way the harness's client can be answered: a session limit, a status
// to refuse a method with, and an initialize answered with no session id.
type sessionFake struct {
	url     string
	limit   int
	refuse  map[string]int
	noID    bool
	deletes atomic.Int64
	pings   atomic.Int64

	mu   sync.Mutex
	open map[string]chan struct{}
	next int
	// seen is the header of every request, in arrival order.
	seen []http.Header
}

// startSessionFake starts the fake with configure applied first.
func startSessionFake(t *testing.T, configure func(*sessionFake)) *sessionFake {
	t.Helper()
	f := &sessionFake{refuse: map[string]int{}, open: map[string]chan struct{}{}}
	if configure != nil {
		configure(f)
	}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	f.url = srv.URL + "/mcp"
	return f
}

// serve answers one request the way the stateful transport does.
func (f *sessionFake) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.seen = append(f.seen, r.Header.Clone())
	f.mu.Unlock()
	id := r.Header.Get(sessionIDHeader)
	switch r.Method {
	case http.MethodGet:
		f.stream(w, r, id)
	case http.MethodDelete:
		f.deletes.Add(1)
		f.mu.Lock()
		ended, ok := f.open[id]
		delete(f.open, id)
		f.mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		close(ended)
		w.WriteHeader(http.StatusNoContent)
	default:
		f.post(w, r, id)
	}
}

// post answers a POST: an initialize opens a session, a notification is taken,
// and any other request is answered with an empty result.
func (f *sessionFake) post(w http.ResponseWriter, r *http.Request, id string) {
	var msg struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
	}
	_ = json.NewDecoder(r.Body).Decode(&msg)
	if status := f.refuse[msg.Method]; status != 0 {
		http.Error(w, "This server is busy. Retry later.", status)
		return
	}
	if msg.Method == methodPing {
		f.pings.Add(1)
	}
	if id == "" {
		f.mu.Lock()
		if f.limit > 0 && len(f.open) >= f.limit {
			f.mu.Unlock()
			http.Error(w, "This server is busy. Retry later.", http.StatusServiceUnavailable)
			return
		}
		f.next++
		id = "fake-" + strconv.Itoa(f.next)
		f.open[id] = make(chan struct{})
		f.mu.Unlock()
		if !f.noID {
			w.Header().Set(sessionIDHeader, id)
		}
	} else if !f.holds(id) {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	if len(msg.ID) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	w.Header().Set(headerContentType, mediaEventStream)
	_, _ = fmt.Fprintf(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":%s,\"result\":{}}\n\n", msg.ID)
}

// stream holds a session's standalone stream open until the client leaves or
// the session ends.
func (f *sessionFake) stream(w http.ResponseWriter, r *http.Request, id string) {
	if status := f.refuse[http.MethodGet]; status != 0 {
		http.Error(w, "This server is busy. Retry later.", status)
		return
	}
	f.mu.Lock()
	ended, ok := f.open[id]
	f.mu.Unlock()
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	w.Header().Set(headerContentType, mediaEventStream)
	w.WriteHeader(http.StatusOK)
	_ = http.NewResponseController(w).Flush()
	select {
	case <-r.Context().Done():
	case <-ended:
	}
}

// holds reports whether the fake holds session id.
func (f *sessionFake) holds(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.open[id]
	return ok
}

// openCount is how many sessions the fake holds.
func (f *sessionFake) openCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.open)
}

// headers is every request header the fake saw.
func (f *sessionFake) headers() []http.Header {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]http.Header(nil), f.seen...)
}

// sessionsOptions are the flags of a sessions run against the stand-in,
// writing where the caller says.
func sessionsOptions(binary, out, counts string) options {
	return options{
		binary:              binary,
		record:              "site/src/data/resource-benchmark.json",
		sampleInterval:      20 * time.Millisecond,
		sessions:            counts,
		sessionsCredentials: 2,
		sessionsStream:      true,
		sessionsJSON:        out,
	}
}

// TestParseFlags_SessionsFlags_AreRead verifies the sessions mode's flags reach
// the options, and what a run that types none of them gets.
func TestParseFlags_SessionsFlags_AreRead(t *testing.T) {
	opts := withArgs(t, "-sessions=10,20", "-sessions-credentials=4", "-sessions-nofile=512", "-sessions-stream",
		"-sessions-json=/tmp/sessions.json")
	if opts.sessions != "10,20" || opts.sessionsCredentials != 4 || opts.sessionsNoFile != 512 || !opts.sessionsStream ||
		opts.sessionsJSON != "/tmp/sessions.json" {
		t.Errorf("sessions %q, credentials %d, nofile %d, stream %v, json %q; want what was typed",
			opts.sessions, opts.sessionsCredentials, opts.sessionsNoFile, opts.sessionsStream, opts.sessionsJSON)
	}
	defaults := withArgs(t)
	if defaults.sessions != "" || defaults.sessionsCredentials != 1 || defaults.sessionsNoFile != 0 ||
		defaults.sessionsStream || defaults.sessionsJSON != defaultSessionsRecord {
		t.Errorf("defaults %+v; want off, one credential, the inherited limit, no stream and %s", defaults, defaultSessionsRecord)
	}
}

// TestOptionsValidate_SessionsNoFile_IsRefusedOnWindows verifies a descriptor
// limit is refused where the shell that would set it does not exist, and
// accepted wherever it does.
func TestOptionsValidate_SessionsNoFile_IsRefusedOnWindows(t *testing.T) {
	previous := runtimeGOOS
	t.Cleanup(func() { runtimeGOOS = previous })
	base := options{sessions: "1", sessionsCredentials: 1, rounds: 1, sampleInterval: time.Millisecond, stepDuration: time.Second}

	runtimeGOOS = "windows"
	limited := base
	limited.sessionsNoFile = 64
	if err := limited.validate(); err == nil || !strings.Contains(err.Error(), "Windows") {
		t.Errorf("validate = %v, want a Windows refusal of -sessions-nofile", err)
	}
	if err := base.validate(); err != nil {
		t.Errorf("validate = %v, want a Windows run with no limit accepted", err)
	}
	runtimeGOOS = "darwin"
	if err := limited.validate(); err != nil {
		t.Errorf("validate = %v, want a limit accepted where /bin/sh exists", err)
	}
}

// TestExecute_SessionsRun_AnswersBeforeTheRecordIsRead verifies the dispatch: a
// sessions run returns from its own mode, so a refusal only that mode makes is
// what comes back.
func TestExecute_SessionsRun_AnswersBeforeTheRecordIsRead(t *testing.T) {
	err := execute(options{
		rounds: 1, sampleInterval: time.Millisecond, stepDuration: time.Millisecond,
		sessions: "1", sessionsCredentials: 1, sessionsJSON: defaultRecord,
	})
	if err == nil || !strings.Contains(err.Error(), "-sessions-json") {
		t.Errorf("execute = %v, want the sessions mode's refusal to write over the record", err)
	}
}

// TestSessionMessages_AreWhatAStatefulClientSends pins the three messages a
// session's life sends, on the revision the stateful transport speaks and with
// none of the per-request _meta 2026-07-28 added.
func TestSessionMessages_AreWhatAStatefulClientSends(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, got, want string }{
		{
			"initialize", initializeMessage(7),
			`{"jsonrpc":"2.0","id":7,"method":"initialize","params":{"protocolVersion":"2025-11-25",` +
				`"capabilities":{},"clientInfo":{"name":"bench-resources","version":"1"}}}`,
		},
		{"initialized", initializedMessage, `{"jsonrpc":"2.0","method":"notifications/initialized"}`},
		{"ping", callMessage(3, methodPing), `{"jsonrpc":"2.0","id":3,"method":"ping"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.got != tc.want {
				t.Errorf("message = %s, want %s", tc.got, tc.want)
			}
			if !json.Valid([]byte(tc.got)) {
				t.Errorf("message %s is not JSON", tc.got)
			}
		})
	}
}

// TestSessionList_KeepsEverySessionGivenAnID verifies an open that got no
// session adds nothing, and that what is read back is a copy.
func TestSessionList_KeepsEverySessionGivenAnID(t *testing.T) {
	t.Parallel()
	var list sessionList
	list.add(nil)
	first, second := &benchSession{id: "a"}, &benchSession{id: "b"}
	list.add(first)
	list.add(second)
	got := list.all()
	if len(got) != 2 || got[0] != first || got[1] != second {
		t.Fatalf("all = %v, want the two sessions in order", got)
	}
	got[0] = nil
	if list.all()[0] != first {
		t.Error("changing what all returned changed the list")
	}
}

// TestSessionClient_Send_ReadsEachAnswer covers the answers a POST can get: a
// request answered in an event stream or a bare body, a notification taken
// with 202, and every status that is not the one expected, kept with the
// status; and the headers every POST carries.
func TestSessionClient_Send_ReadsEachAnswer(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, method, message, answer, contentType string
		status                                     int
		wantErr                                    string
		wantStatus                                 int
	}{
		{
			name: "a request in an event stream", method: methodPing, message: callMessage(1, methodPing),
			status: http.StatusOK, contentType: mediaEventStream, answer: "event: message\ndata: {\"id\":1,\"result\":{}}\n\n",
		},
		{
			name: "a request in a bare body", method: methodPing, message: callMessage(1, methodPing),
			status: http.StatusOK, contentType: mediaJSON, answer: `{"id":1,"result":{}}`,
		},
		{name: "a notification", method: methodInitialized, message: initializedMessage, status: http.StatusAccepted},
		{
			name: "a refused request", method: methodPing, message: callMessage(1, methodPing),
			status: http.StatusServiceUnavailable, answer: "busy\nsecond line", wantStatus: http.StatusServiceUnavailable,
		},
		{
			name: "a notification answered 200", method: methodInitialized, message: initializedMessage,
			status: http.StatusOK, wantStatus: http.StatusOK,
		},
		{
			name: "an event stream with no data", method: methodPing, message: callMessage(1, methodPing),
			status: http.StatusOK, contentType: mediaEventStream, answer: "event: message\n\n", wantErr: "no data line",
		},
		{
			name: "a JSON-RPC error", method: methodPing, message: callMessage(1, methodPing),
			status: http.StatusOK, contentType: mediaJSON, answer: `{"id":1,"error":{"code":-32600,"message":"no"}}`,
			wantErr: "jsonrpc error -32600",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			seen := make(chan http.Header, 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen <- r.Header.Clone()
				w.Header().Set(sessionIDHeader, "answered-on")
				if tc.contentType != "" {
					w.Header().Set(headerContentType, tc.contentType)
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.answer)
			}))
			t.Cleanup(srv.Close)
			client := newSessionClient(srv.URL, "token-a")

			id, err := client.send(t.Context(), "held-session", tc.method, tc.message)
			var status *httpStatusError
			switch {
			case tc.wantStatus != 0:
				if !errors.As(err, &status) || status.Status != tc.wantStatus || status.Method != tc.method {
					t.Errorf("send = %v, want the %d kept with the method", err, tc.wantStatus)
				}
			case tc.wantErr != "":
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) || !strings.HasPrefix(err.Error(), tc.method) {
					t.Errorf("send = %v, want an error naming %s and %q", err, tc.method, tc.wantErr)
				}
			case err != nil || id != "answered-on":
				t.Errorf("send = %q, %v; want the session id the answer carried", id, err)
			}
			assertSessionHeaders(t, <-seen, "held-session", tc.method)
		})
	}
}

// assertSessionHeaders checks what every POST of a session carries.
func assertSessionHeaders(t *testing.T, seen http.Header, session, method string) {
	t.Helper()
	for name, want := range map[string]string{
		"Accept": mediaJSON + ", " + mediaEventStream, "MCP-Protocol-Version": sessionProtocol,
		"PRIVATE-TOKEN": "token-a", sessionIDHeader: session, headerContentType: mediaJSON, "Mcp-Method": method,
	} {
		if got := seen.Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

// truncatingServer answers every request with a body shorter than the length
// it announced, so reading it fails.
func truncatingServer(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "short")
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// closedEndpoint is the address of a server that is no longer listening.
func closedEndpoint(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	return srv.URL
}

// TestSessionClient_Send_ReportsWhatItCouldNotSend covers a request that could
// not be built, one that reached no server, and an answer that could not be
// read.
func TestSessionClient_Send_ReportsWhatItCouldNotSend(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, endpoint, want string }{
		{"a request it cannot build", "http://[::1", "build POST request"},
		{"a server that is gone", closedEndpoint(t), methodPing + ": "},
		{"an answer cut short", truncatingServer(t), "read " + methodPing + " response"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := newSessionClient(tc.endpoint, "token").send(t.Context(), "", methodPing, callMessage(1, methodPing))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("send = %v, want an error naming %q", err, tc.want)
			}
		})
	}
}

// TestSessionClient_Open_OpensTheSessionAndItsStream drives a whole session:
// initialize, the handshake, the standalone stream held open, a ping, and the
// close that ends the stream and deletes the session.
func TestSessionClient_Open_OpensTheSessionAndItsStream(t *testing.T) {
	t.Parallel()
	fake := startSessionFake(t, nil)
	client := newSessionClient(fake.url, "token-a")

	session, err := client.open(t.Context(), true)
	if err != nil || session == nil || session.id != "fake-1" {
		t.Fatalf("open = %+v, %v; want session fake-1 with its stream", session, err)
	}
	if session.stop == nil || session.done == nil {
		t.Fatal("the session opened no stream")
	}
	if pingErr := session.ping(t.Context()); pingErr != nil || fake.pings.Load() != 1 {
		t.Errorf("ping = %v after %d pings, want the session answered", pingErr, fake.pings.Load())
	}
	session.close(t.Context())
	session.closeStream()
	select {
	case <-session.done:
	default:
		t.Error("closing the session left its stream's reader running")
	}
	if fake.deletes.Load() != 1 || fake.openCount() != 0 {
		t.Errorf("%d deletes, %d sessions left; want the session deleted once", fake.deletes.Load(), fake.openCount())
	}
	for _, header := range fake.headers() {
		if header.Get("PRIVATE-TOKEN") != "token-a" || header.Get("MCP-Protocol-Version") != sessionProtocol {
			t.Errorf("a request carried %v, want the credential and the revision on every one", header)
		}
	}
}

// TestSessionClient_Open_ReportsHowTheOpeningStopped covers every way an open
// stops short: an initialize refused or answered with no session id, which
// leave no session to close, and a handshake or a stream refused, which leave
// one.
func TestSessionClient_Open_ReportsHowTheOpeningStopped(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		configure   func(*sessionFake)
		wantSession bool
		wantErr     string
	}{
		{
			name: "the initialize refused", configure: func(f *sessionFake) { f.refuse[methodInitialize] = 503 },
			wantErr: "HTTP 503",
		},
		{name: "no session id", configure: func(f *sessionFake) { f.noID = true }, wantErr: "no session id"},
		{
			name: "the handshake refused", configure: func(f *sessionFake) { f.refuse[methodInitialized] = 503 },
			wantSession: true, wantErr: methodInitialized,
		},
		{
			name: "the stream refused", configure: func(f *sessionFake) { f.refuse[http.MethodGet] = 503 },
			wantSession: true, wantErr: methodStream + ": HTTP 503: This server is busy.",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake := startSessionFake(t, tc.configure)
			session, err := newSessionClient(fake.url, "token").open(t.Context(), true)
			if (session != nil) != tc.wantSession {
				t.Errorf("open returned session %+v, want one: %v", session, tc.wantSession)
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("open = %v, want an error naming %q", err, tc.wantErr)
			}
			if session != nil && session.stop != nil {
				t.Error("a refused stream left a reader to stop")
			}
		})
	}
}

// TestBenchSession_OpenStream_ReportsWhatItCouldNotOpen covers a stream whose
// request could not be built and one that reached no server.
func TestBenchSession_OpenStream_ReportsWhatItCouldNotOpen(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, endpoint, want string }{
		{"a request it cannot build", "http://[::1", "build GET request"},
		{"a server that is gone", closedEndpoint(t), methodStream + ": "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			session := &benchSession{client: newSessionClient(tc.endpoint, "token"), id: "s"}
			if err := session.openStream(t.Context()); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("openStream = %v, want an error naming %q", err, tc.want)
			}
		})
	}
}

// TestSessionClient_Remove_ReportsWhatItCouldNotSend covers the DELETE: a
// request it could not build, one that reached no server, and one answered.
func TestSessionClient_Remove_ReportsWhatItCouldNotSend(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, endpoint, want string }{
		{"a request it cannot build", "http://[::1", "build DELETE request"},
		{"a server that is gone", closedEndpoint(t), methodDelete + ": "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := newSessionClient(tc.endpoint, "token").remove(t.Context(), "s"); err == nil ||
				!strings.Contains(err.Error(), tc.want) {
				t.Errorf("remove = %v, want an error naming %q", err, tc.want)
			}
		})
	}
	fake := startSessionFake(t, nil)
	if err := newSessionClient(fake.url, "token").remove(t.Context(), "not-held"); err != nil || fake.deletes.Load() != 1 {
		t.Errorf("remove of a session the server no longer holds = %v after %d deletes, want it sent and no error",
			err, fake.deletes.Load())
	}
}

// TestCountLive_StopsAtTheFirstPingNoOneAnswered verifies the count: a
// session answered is live, one the server answered for and no longer holds
// is not and the count goes on, and a ping that got no answer at all ends the
// count, leaving the rest unasked.
func TestCountLive_StopsAtTheFirstPingNoOneAnswered(t *testing.T) {
	t.Parallel()
	fake := startSessionFake(t, nil)
	live := newSessionClient(fake.url, "token")
	var held []*benchSession
	for range 3 {
		session, err := live.open(t.Context(), false)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		held = append(held, session)
	}
	gone := &benchSession{client: live, id: "not-held"}
	dead := &benchSession{client: newSessionClient(closedEndpoint(t), "token"), id: "dead"}

	sessions := []*benchSession{held[0], gone, held[1], dead, held[2]}
	if got := countLive(t.Context(), sessions); got != 2 {
		t.Errorf("countLive = %d, want the two sessions before the one no server answered for", got)
	}
	if got := fake.pings.Load(); got != 3 {
		t.Errorf("%d pings reached the server, want 3: the session after the unanswered one is not asked", got)
	}
}

// TestBenchSession_Ping_WaitsNoLongerThanAHealthProbe verifies a ping to a
// server that never answers gives up after the probe's timeout.
func TestBenchSession_Ping_WaitsNoLongerThanAHealthProbe(t *testing.T) {
	previous := healthTimeout
	t.Cleanup(func() { healthTimeout = previous })
	healthTimeout = 50 * time.Millisecond
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })

	session := &benchSession{client: newSessionClient(srv.URL, "token"), id: "s"}
	var err error
	finishWithin(t, 10*time.Second, "a ping to a server that never answers", func() { err = session.ping(t.Context()) })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("ping = %v, want the probe's deadline", err)
	}
}

// TestBenchSession_OpenStream_ARefusalThatNeverEnds_IsAbandoned verifies a
// stream refused with a body that stays open is reported after the probe's
// timeout, with what the body said by then, rather than holding the step.
// Reading such a body to its end is also what a stream answered 200 would do
// if the status test were ever inverted, which is why the wait is bounded.
func TestBenchSession_OpenStream_ARefusalThatNeverEnds_IsAbandoned(t *testing.T) {
	previous := healthTimeout
	t.Cleanup(func() { healthTimeout = previous })
	healthTimeout = 50 * time.Millisecond
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, "busy\n")
		_ = http.NewResponseController(w).Flush()
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })

	session := &benchSession{client: newSessionClient(srv.URL, "token"), id: "s"}
	var err error
	finishWithin(t, 10*time.Second, "a refused stream whose body never ends", func() { err = session.openStream(t.Context()) })
	status, ok := errors.AsType[*httpStatusError](err)
	if !ok || status.Status != http.StatusServiceUnavailable || status.Snippet != "busy" {
		t.Errorf("openStream = %v, want the 503 with what its body said before it was abandoned", err)
	}
	if session.stop != nil || session.done != nil {
		t.Error("a refused stream left a reader to stop")
	}
}

// TestAdmitSessions_AdmitsEachCredentialOrSaysWhichItCouldNot covers the
// warm-up: every credential opens a session, lists its tools and deletes it;
// a credential whose session is refused, or whose listing is, is named, and a
// session opened is deleted either way.
func TestAdmitSessions_AdmitsEachCredentialOrSaysWhichItCouldNot(t *testing.T) {
	t.Parallel()
	t.Run("every credential admitted", func(t *testing.T) {
		t.Parallel()
		fake := startSessionFake(t, nil)
		clients := []*sessionClient{newSessionClient(fake.url, "a"), newSessionClient(fake.url, "b")}
		if err := admitSessions(t.Context(), clients); err != nil {
			t.Fatalf("admitSessions: %v", err)
		}
		if fake.deletes.Load() != 2 || fake.openCount() != 0 {
			t.Errorf("%d deletes, %d sessions left; want each credential's session deleted", fake.deletes.Load(), fake.openCount())
		}
	})
	for _, tc := range []struct {
		name        string
		method      string
		wantDeletes int64
	}{
		{name: "the session refused", method: methodInitialize},
		{name: "the listing refused", method: methodToolsList, wantDeletes: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake := startSessionFake(t, func(f *sessionFake) { f.refuse[tc.method] = http.StatusServiceUnavailable })
			err := admitSessions(t.Context(), []*sessionClient{newSessionClient(fake.url, "a")})
			if err == nil || !strings.Contains(err.Error(), "admit credential 0") || !strings.Contains(err.Error(), "HTTP 503") {
				t.Errorf("admitSessions = %v, want credential 0 named with the refusal", err)
			}
			if fake.deletes.Load() != tc.wantDeletes {
				t.Errorf("%d deletes, want %d", fake.deletes.Load(), tc.wantDeletes)
			}
		})
	}
}

// TestRunSessionsStep_FilesEachOpenAndCountsWhatTheServerHolds drives one step
// over a fake holding at most two sessions: three opens with their streams,
// two opened and one refused with its text kept, both open and live when the
// step is sampled, every session deleted afterwards, and every figure the
// sample could not take noted.
func TestRunSessionsStep_FilesEachOpenAndCountsWhatTheServerHolds(t *testing.T) {
	fastHeldSettling(t)
	fake := startSessionFake(t, func(f *sessionFake) { f.limit = 2 })
	failing := startSessionFake(t, func(f *sessionFake) { f.refuse[methodInitialize] = http.StatusBadGateway })
	r := &runner{}
	in := heldInput{tgt: &httpTarget{}, profiler: newPprofClient("http://127.0.0.1:1")}
	previousGOOS := runtimeGOOS
	t.Cleanup(func() { runtimeGOOS = previousGOOS })
	runtimeGOOS = "plan9"
	clients := []*sessionClient{
		newSessionClient(fake.url, "a"), newSessionClient(fake.url, "b"), newSessionClient(failing.url, "c"),
	}

	step := r.runSessionsStep(t.Context(), in, clients, 4, true)
	if step.Offered != 4 || step.Open != 2 || step.Live != 2 || step.Settled != settledAll {
		t.Errorf("step = %s, want four offered and two open and live", step.summary())
	}
	if step.Opened != 2 || step.Refused != 1 || step.Failed != 1 {
		t.Errorf("opened, refused, failed = %d, %d, %d; want 2, 1, 1", step.Opened, step.Refused, step.Failed)
	}
	if !strings.Contains(step.FirstRefusal, "This server is busy.") || !strings.Contains(step.FirstFailure, "HTTP 502") {
		t.Errorf("first refusal %q, first failure %q; want the fakes' own", step.FirstRefusal, step.FirstFailure)
	}
	if fake.openCount() != 0 || fake.deletes.Load() != 2 {
		t.Errorf("%d sessions left and %d deletes after the step, want every one deleted", fake.openCount(), fake.deletes.Load())
	}
	if step.Sample.Descriptors != -1 || step.Sample.Health != healthUnanswered || len(step.Sample.Notes) != 4 {
		t.Errorf("sample = %+v, want every figure it could not take noted", step.Sample)
	}
}

// TestSessionsStep_Summary_RendersEveryFigure pins the progress line, the
// per-session figures included.
func TestSessionsStep_Summary_RendersEveryFigure(t *testing.T) {
	t.Parallel()
	sample := HeldSample{Descriptors: 110, Goroutines: 400, HeapBytes: 50 << 20, RSSBytes: 190 << 20, Health: "200", HealthMs: 1}
	step := SessionsStep{Offered: 100, Open: 96, Live: 96, Settled: settledAll, Opened: 96, Refused: 4, Sample: sample}
	bare := "100 offered, 96 open (settled): " + sample.summary() + "; 96 live; then 96 opened, 4 refused, 0 failed"
	if got := step.summary(); got != bare {
		t.Errorf("summary = %q, want %q", got, bare)
	}
	step.PerSession = &HeldCost{Descriptors: 1.01, Goroutines: 4.02, HeapKiB: 12.3, RSSKiB: 80.4}
	if got, want := step.summary(), bare+"; per session 1.01 descriptors, 4.02 goroutines, 12.3 KiB heap, 80.4 KiB resident"; got != want {
		t.Errorf("summary = %q, want %q", got, want)
	}
}

// readSessions decodes a document a -sessions run wrote.
func readSessions(t *testing.T, path string) SessionsDoc {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the sessions document: %v", err)
	}
	var doc SessionsDoc
	if decodeErr := json.Unmarshal(raw, &doc); decodeErr != nil {
		t.Fatalf("decode the sessions document: %v", decodeErr)
	}
	return doc
}

// TestRunSessions_MeasuresTheLadderAndWritesItsOwnDocument drives the whole
// mode against the stand-in on its stateful transport: two steps, the second
// past a session limit of three, each session with its standalone stream, a
// descriptor limit the server was started under, and a document written where
// it was asked for.
func TestRunSessions_MeasuresTheLadderAndWritesItsOwnDocument(t *testing.T) {
	binary := standinBinary(t)
	fastHeldSettling(t)
	t.Setenv("STANDIN_SESSION_LIMIT", "3")
	root := t.TempDir()
	opts := sessionsOptions(binary, filepath.Join(root, "out", "sessions.json"), "2,4")
	if runtime.GOOS == "linux" {
		opts.sessionsNoFile = 256
	}

	var runErr error
	finishWithin(t, 2*time.Minute, "a two-step sessions run", func() { runErr = runSessions(opts, root) })
	if runErr != nil {
		t.Fatalf("runSessions: %v", runErr)
	}
	doc := readSessions(t, opts.sessionsJSON)
	assertSessionsHeader(t, doc, opts)
	if len(doc.Steps) != 2 {
		t.Fatalf("steps = %+v, want two", doc.Steps)
	}
	assertSessionsSteps(t, doc.Steps[0], doc.Steps[1])
}

// assertSessionsHeader checks what a sessions document says about its run
// and about the idle process every step is priced against.
func assertSessionsHeader(t *testing.T, doc SessionsDoc, opts options) {
	t.Helper()
	if doc.Schema != sessionsSchema || doc.Credentials != 2 || !doc.Streams || doc.NoFile != opts.sessionsNoFile {
		t.Errorf("document header %+v, want the schema, two credentials, streams and the limit asked for", doc)
	}
	if doc.Server.Version != "standin" || doc.GeneratedAt == "" || doc.Idle.Health != "200" {
		t.Errorf("server %+v generated %q idle %+v, want the build measured, a timestamp and a healthy idle process",
			doc.Server, doc.GeneratedAt, doc.Idle)
	}
}

// assertSessionsSteps checks the two steps of the stand-in ladder: one under
// the stand-in's session limit of three and one past it.
func assertSessionsSteps(t *testing.T, under, over SessionsStep) {
	t.Helper()
	if under.Open != 2 || under.Live != 2 || under.Opened != 2 || under.Refused != 0 || under.Failed != 0 {
		t.Errorf("first step = %s, want both sessions opened and live", under.summary())
	}
	if over.Open != 3 || over.Live != 3 || over.Opened != 3 || over.Refused != 1 || over.Failed != 0 ||
		!strings.Contains(over.FirstRefusal, "This server is busy.") {
		t.Errorf("second step = %s, want three opened and live and one refused", over.summary())
	}
	if under.PerSession == nil || over.PerSession == nil || over.Sample.Health != "200" {
		t.Errorf("per session %+v and %+v, /health %q; want both steps priced and the process answering",
			under.PerSession, over.PerSession, over.Sample.Health)
	}
}

// TestRunSessions_ReportsWhatItCouldNotDo covers every way the mode stops
// short: counts it cannot read, a document that would overwrite the published
// record, a server it could not build, start or admit a credential to, and a
// document it could not write.
func TestRunSessions_ReportsWhatItCouldNotDo(t *testing.T) {
	fastHeldSettling(t)
	root := t.TempDir()
	out := filepath.Join(root, "sessions.json")

	t.Run("counts it cannot read", func(t *testing.T) {
		if err := runSessions(sessionsOptions("/unused", out, "3,1"), root); err == nil || !strings.Contains(err.Error(), "-sessions:") {
			t.Errorf("runSessions = %v, want the -sessions refusal", err)
		}
	})
	for name, target := range map[string]string{"the record it was given": "custom/record.json", "the default record": defaultRecord} {
		t.Run("a document naming "+name, func(t *testing.T) {
			opts := sessionsOptions("/unused", target, "1")
			opts.record = "custom/record.json"
			if err := runSessions(opts, root); err == nil || !strings.Contains(err.Error(), "published record") {
				t.Errorf("runSessions = %v, want the refusal to write over the record", err)
			}
		})
	}
	t.Run("a build that failed", func(t *testing.T) {
		original := buildServerBinary
		t.Cleanup(func() { buildServerBinary = original })
		buildServerBinary = func(string) (string, error) { return "", errors.New("no toolchain") }
		if err := runSessions(sessionsOptions("", out, "1"), root); err == nil || !strings.Contains(err.Error(), "no toolchain") {
			t.Errorf("runSessions = %v, want the build failure", err)
		}
	})
	t.Run("a server that does not start", func(t *testing.T) {
		if err := runSessions(sessionsOptions(filepath.Join(root, "no-such-server"), out, "1"), root); err == nil {
			t.Error("runSessions measured a server that does not exist")
		}
		if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
			t.Errorf("a run that measured nothing wrote a document: %v", statErr)
		}
	})
	t.Run("a credential that is not admitted", func(t *testing.T) {
		binary := standinBinary(t)
		t.Setenv("STANDIN_FAIL", methodInitialize)
		err := runSessions(sessionsOptions(binary, out, "1"), root)
		if err == nil || !strings.Contains(err.Error(), "admit credential 0") {
			t.Errorf("runSessions = %v, want the admission failure", err)
		}
	})
	t.Run("a document it cannot write", func(t *testing.T) {
		binary := standinBinary(t)
		blocker := filepath.Join(root, "a-file")
		if err := os.WriteFile(blocker, nil, 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		opts := sessionsOptions(binary, filepath.Join(blocker, "sessions.json"), "1")
		var err error
		finishWithin(t, time.Minute, "a one-step sessions run", func() { err = runSessions(opts, root) })
		if err == nil || !strings.Contains(err.Error(), blocker) {
			t.Errorf("runSessions = %v, want the write refused where a file stands in the way", err)
		}
	})
}

// TestNewSessionClient_BoundsTheWaitForAnAnswer pins the one bound a client
// with no timeout of its own keeps: the wait for any response's headers.
func TestNewSessionClient_BoundsTheWaitForAnAnswer(t *testing.T) {
	t.Parallel()
	client := newSessionClient("http://127.0.0.1:1/mcp", "token")
	transport, ok := client.client.Transport.(*http.Transport)
	if !ok || transport.ResponseHeaderTimeout != callTimeout || client.client.Timeout != 0 {
		t.Errorf("transport %+v, timeout %s; want the header wait bounded and the stream not", transport, client.client.Timeout)
	}
}
