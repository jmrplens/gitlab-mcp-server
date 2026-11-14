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

// silentWait is the deadline the silent endpoint's tests give a request, short
// because the request is expected to wait it out.
const silentWait = 200 * time.Millisecond

// assertWaitedOut fails the test unless err is a request that waited out its
// deadline after silentWait at least, which is what a listener that accepted
// and then said nothing looks like, and not a hang-up.
func assertWaitedOut(t *testing.T, what string, status int, err error, waited time.Duration) {
	t.Helper()
	if err == nil {
		t.Errorf("%s was answered %d, want it left waiting", what, status)
		return
	}
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Errorf("%s ended with %v, want it to wait out its deadline: a hang-up is what DeadEndpoint gives", what, err)
	}
	if waited < silentWait {
		t.Errorf("%s ended after %v, before its %v deadline", what, waited, silentWait)
	}
}

// TestSilentEndpoint_EveryRequest_WaitsOutItsDeadline verifies the address
// accepts a request and never answers it, for every request rather than the
// first: each one waits out the client's deadline, the one after it on a new
// connection too, which only an accept loop that keeps going can give.
func TestSilentEndpoint_EveryRequest_WaitsOutItsDeadline(t *testing.T) {
	t.Parallel()
	url := SilentEndpoint(t)
	if !strings.HasPrefix(url, "http://127.0.0.1:") {
		t.Fatalf("SilentEndpoint() = %q, want a loopback http URL", url)
	}
	for i := range 2 {
		client := &http.Client{Timeout: silentWait}
		started := time.Now()
		status, err := get(t, client, url+"/api/v4/user")
		assertWaitedOut(t, fmt.Sprintf("request %d", i+1), status, err, time.Since(started))
		client.CloseIdleConnections()
	}
}

// TestSilentEndpoint_ThePortAndItsConnections_AreHeldUntilTheTestEnds verifies
// that while the test runs nothing else can be given the port and a
// connection the endpoint accepted stays open, and that once the test has
// ended the port is released and the connection closed by the endpoint, so no
// connection outlives the test that asked for it. The subtest is the test
// that ends, and it has to have ended before either is asked about again, so
// neither runs in parallel.
func TestSilentEndpoint_ThePortAndItsConnections_AreHeldUntilTheTestEnds(t *testing.T) {
	var (
		url  string
		conn net.Conn
	)
	t.Run("held while the test runs", func(t *testing.T) {
		url = SilentEndpoint(t)
		assertPortHeld(t, url)
		var err error
		conn, err = (&net.Dialer{}).DialContext(t.Context(), "tcp", strings.TrimPrefix(url, "http://"))
		if err != nil {
			t.Fatalf("dial the silent endpoint: %v", err)
		}
		if _, err = conn.Write([]byte("GET / HTTP/1.1\r\nHost: x\r\n\r\n")); err != nil {
			t.Fatalf("write a request: %v", err)
		}
		if err = conn.SetReadDeadline(time.Now().Add(silentWait)); err != nil {
			t.Fatalf("set the read deadline: %v", err)
		}
		var netErr net.Error
		if _, err = conn.Read(make([]byte, 1)); !errors.As(err, &netErr) || !netErr.Timeout() {
			t.Errorf("reading the connection while the test runs = %v, want nothing read until the deadline", err)
		}
	})
	if url == "" || conn == nil {
		t.Fatal("the subtest returned no address or no connection")
	}
	defer conn.Close()
	assertPortFree(t, url)
	if err := conn.SetReadDeadline(time.Now().Add(requestWait)); err != nil {
		t.Fatalf("set the read deadline: %v", err)
	}
	if _, err := conn.Read(make([]byte, 1)); err == nil {
		t.Error("the connection still answered after the test ended, want it closed by the endpoint")
	} else if netErr, ok := errors.AsType[net.Error](err); ok && netErr.Timeout() {
		t.Errorf("the connection was still open after the test ended (%v), want it closed by the endpoint", err)
	}
}

// TestHoldEach_AnAcceptThatFails_KeepsAcceptingUntilClosed verifies the loop
// survives an accept that fails for any reason but the listener being closed:
// it goes on to hold the connection accepted next, reading what it is sent and
// writing nothing back, and ends only once the listener is closed. The
// connection is closed when the cleanup's closeAll runs and not before.
func TestHoldEach_AnAcceptThatFails_KeepsAcceptingUntilClosed(t *testing.T) {
	t.Parallel()
	conn, far := net.Pipe()
	defer far.Close()
	listener := &scriptedListener{script: []accepted{
		{err: errors.New("accept: too many open files")},
		{conn: conn},
		{err: fmt.Errorf("accept: %w", net.ErrClosed)},
	}}
	held := &heldConns{}
	done := make(chan struct{})
	go holdEach(listener, held, done)

	select {
	case <-done:
	case <-time.After(requestWait):
		t.Fatal("the loop did not end once the listener was closed")
	}
	if err := far.SetDeadline(time.Now().Add(silentWait)); err != nil {
		t.Fatalf("set the deadline: %v", err)
	}
	if _, err := far.Write([]byte("GET / HTTP/1.1\r\n\r\n")); err != nil {
		t.Errorf("writing to the held connection = %v, want it read into nothing", err)
	}
	var netErr net.Error
	if _, err := far.Read(make([]byte, 1)); !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Errorf("reading the held connection = %v, want nothing written back until the deadline", err)
	}

	// The deadline set above has passed, so a connection closeAll left open
	// reads a timeout at once rather than blocking: a pipe whose far end was
	// closed reads its EOF first.
	held.closeAll()
	if _, err := far.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Errorf("reading the connection after closeAll = %v, want the EOF of a closed connection", err)
	}
}

// TestSilentEndpoint_APortThatCannotBeHad_FailsTheTest verifies a listener
// that could not be opened fails the test that asked for it, naming why,
// rather than handing it an address nothing holds. It replaces the package's
// listen and so does not run in parallel.
func TestSilentEndpoint_APortThatCannotBeHad_FailsTheTest(t *testing.T) {
	refused := errors.New("no port for you")
	original := listen
	listen = func(context.Context) (net.Listener, error) { return nil, refused }
	t.Cleanup(func() { listen = original })

	recorder := &fatalRecorder{TB: t}
	if url := SilentEndpoint(recorder); url != "" {
		t.Errorf("SilentEndpoint() = %q after a failed listen, want no address", url)
	}
	if !strings.Contains(recorder.fatal, refused.Error()) {
		t.Errorf("the test was failed with %q, want it to name %q", recorder.fatal, refused)
	}
}
