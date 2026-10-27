// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package netfixture

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// requestWait bounds every request these tests send. A dead endpoint and a
// stopped server hang up at once, so a request that is still waiting when it
// expires found a listener that accepted and then said nothing, which is the
// failure these tests exist to tell apart from a hang-up.
const requestWait = 5 * time.Second

// get sends one GET through client and returns the error it ended in, or nil
// with the status when something answered.
func get(t *testing.T, client *http.Client, url string) (int, error) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatalf("build the request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

// assertHungUp fails the test unless err is a request that ended without an
// answer, promptly: an error, and not the one a request that waited out its
// deadline ends in.
func assertHungUp(t *testing.T, what string, status int, err error) {
	t.Helper()
	if err == nil {
		t.Errorf("%s was answered %d, want the connection ended without an answer", what, status)
		return
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		t.Errorf("%s waited out its deadline (%v), want it hung up on at once", what, err)
	}
}

// assertPortHeld fails the test unless the address in url cannot be listened
// on, which is what a port still held looks like from beside it.
func assertPortHeld(t *testing.T, url string) {
	t.Helper()
	addr := strings.TrimPrefix(url, "http://")
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", addr)
	if err == nil {
		_ = listener.Close()
		t.Errorf("listening on %s succeeded, want the port still held", addr)
	}
}

// assertPortFree fails the test unless the address in url can be listened on
// again, which is what a released port looks like.
func assertPortFree(t *testing.T, url string) {
	t.Helper()
	addr := strings.TrimPrefix(url, "http://")
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", addr)
	if err != nil {
		t.Errorf("listening on %s after the test ended = %v, want the port released", addr, err)
		return
	}
	_ = listener.Close()
}

// TestDeadEndpoint_EveryRequest_EndsWithoutAnAnswer verifies the address is
// one nothing answers on, for every request rather than the first: a request
// fails before any response, promptly, and so does the one after it on a new
// connection, which only an accept loop that keeps going can give.
func TestDeadEndpoint_EveryRequest_EndsWithoutAnAnswer(t *testing.T) {
	t.Parallel()
	url := DeadEndpoint(t)
	if !strings.HasPrefix(url, "http://127.0.0.1:") {
		t.Fatalf("DeadEndpoint() = %q, want a loopback http URL", url)
	}
	for i := range 3 {
		client := &http.Client{Timeout: requestWait}
		status, err := get(t, client, url+"/health")
		assertHungUp(t, fmt.Sprintf("request %d", i+1), status, err)
		client.CloseIdleConnections()
	}
}

// TestDeadEndpoint_ThePort_IsHeldUntilTheTestEnds verifies the property the
// helper exists for: while the test runs, nothing else can be given the port,
// and once it has ended the port is released. The subtest is the test that
// ends, and it has to have ended before the port is asked for again, so
// neither runs in parallel.
func TestDeadEndpoint_ThePort_IsHeldUntilTheTestEnds(t *testing.T) {
	var url string
	t.Run("held while the test runs", func(t *testing.T) {
		url = DeadEndpoint(t)
		assertPortHeld(t, url)
	})
	if url == "" {
		t.Fatal("the subtest returned no address")
	}
	assertPortFree(t, url)
}

// fatalRecorder stands in for a test whose helper fails it, so the failure is
// recorded rather than ending the test that checks it.
type fatalRecorder struct {
	testing.TB
	fatal string
}

// Fatalf records the failure and returns, which a real test's Fatalf never
// does; the helper returns right after calling it either way.
func (r *fatalRecorder) Fatalf(format string, args ...any) {
	r.fatal = fmt.Sprintf(format, args...)
}

// accepted is one answer a scripted listener gives to Accept.
type accepted struct {
	conn net.Conn
	err  error
}

// scriptedListener answers Accept from a script, in order, and only Accept:
// it stands in for a listener whose accepts fail for a while, the way they do
// while the process has no file descriptor left, without exhausting any.
type scriptedListener struct {
	net.Listener
	script []accepted
}

// Accept gives the next answer of the script. Only the loop under test calls
// it, from its one goroutine.
func (l *scriptedListener) Accept() (net.Conn, error) {
	next := l.script[0]
	l.script = l.script[1:]
	return next.conn, next.err
}

// TestHangUpOnEach_AnAcceptThatFails_KeepsAcceptingUntilClosed verifies the
// loop survives an accept that fails for any reason but the listener being
// closed: it goes on to hang up on the connection accepted next, and ends only
// once the listener is closed. A loop that ended on the first failure would
// leave the port held with nothing accepting, and the kernel would queue every
// connection made there, so a request would wait out its deadline instead of
// failing at once.
func TestHangUpOnEach_AnAcceptThatFails_KeepsAcceptingUntilClosed(t *testing.T) {
	t.Parallel()
	conn, far := net.Pipe()
	defer far.Close()
	listener := &scriptedListener{script: []accepted{
		{err: errors.New("accept: too many open files")},
		{conn: conn},
		{err: fmt.Errorf("accept: %w", net.ErrClosed)},
	}}
	done := make(chan struct{})
	go hangUpOnEach(listener, done)

	if err := far.SetReadDeadline(time.Now().Add(requestWait)); err != nil {
		t.Fatalf("set the read deadline: %v", err)
	}
	if _, err := far.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Errorf("reading the connection accepted after a failed accept = %v, want the EOF of a hang-up", err)
	}
	select {
	case <-done:
	case <-time.After(requestWait):
		t.Fatal("the loop did not end once the listener was closed")
	}
}

// TestDeadEndpoint_APortThatCannotBeHad_FailsTheTest verifies a listener that
// could not be opened fails the test that asked for it, naming why, rather
// than handing it an address nothing holds. It replaces the package's listen
// and so does not run in parallel.
func TestDeadEndpoint_APortThatCannotBeHad_FailsTheTest(t *testing.T) {
	refused := errors.New("no port for you")
	original := listen
	listen = func(context.Context) (net.Listener, error) { return nil, refused }
	t.Cleanup(func() { listen = original })

	recorder := &fatalRecorder{TB: t}
	if url := DeadEndpoint(recorder); url != "" {
		t.Errorf("DeadEndpoint() = %q after a failed listen, want no address", url)
	}
	if !strings.Contains(recorder.fatal, refused.Error()) {
		t.Errorf("the test was failed with %q, want it to name %q", recorder.fatal, refused)
	}
}
