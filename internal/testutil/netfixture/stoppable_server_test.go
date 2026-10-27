// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package netfixture

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
)

// TestStoppableServer_BeforeStop_AnswersThroughItsHandler verifies a server
// nobody has stopped is an ordinary one: its handler answers.
func TestStoppableServer_BeforeStop_AnswersThroughItsHandler(t *testing.T) {
	t.Parallel()
	server := NewStoppableServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	status, err := get(t, &http.Client{Timeout: requestWait}, server.URL)
	if err != nil || status != http.StatusTeapot {
		t.Errorf("GET = %d, %v; want the handler's %d", status, err, http.StatusTeapot)
	}
}

// TestStoppableServer_AfterStop_HangsUpOnEveryRequestAndKeepsItsPort verifies
// what Stop changes: a request on the connection the client kept from before
// is hung up on, so is one on a new connection, the handler is never reached
// again, and the port stays held, which is the difference from closing the
// server. Stopping a second time changes nothing.
func TestStoppableServer_AfterStop_HangsUpOnEveryRequestAndKeepsItsPort(t *testing.T) {
	t.Parallel()
	var reached atomic.Int64
	server := NewStoppableServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	kept := &http.Client{Timeout: requestWait}
	if status, err := get(t, kept, server.URL); err != nil || status != http.StatusOK {
		t.Fatalf("GET before Stop = %d, %v; want 200", status, err)
	}

	server.Stop()
	server.Stop()

	status, err := get(t, kept, server.URL)
	assertHungUp(t, "a request on the client that was answered before Stop", status, err)
	status, err = get(t, &http.Client{Timeout: requestWait}, server.URL)
	assertHungUp(t, "a request from a new client", status, err)
	if got := reached.Load(); got != 1 {
		t.Errorf("the handler was reached %d times, want only the request before Stop", got)
	}
	assertPortHeld(t, server.URL)
}

// TestStoppableServer_Stop_EndsAResponseInProgress verifies Stop closes the
// connections already open as well, so a response being streamed when the
// test stops the server ends there rather than going on to its end.
func TestStoppableServer_Stop_EndsAResponseInProgress(t *testing.T) {
	t.Parallel()
	server := NewStoppableServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "first line\n")
		http.NewResponseController(w).Flush()
		<-r.Context().Done()
	}))
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, http.NoBody)
	if err != nil {
		t.Fatalf("build the request: %v", err)
	}
	resp, err := (&http.Client{Timeout: requestWait}).Do(req)
	if err != nil {
		t.Fatalf("GET = %v, want the stream opened", err)
	}
	defer resp.Body.Close()
	body := bufio.NewReader(resp.Body)
	if line, readErr := body.ReadString('\n'); readErr != nil || line != "first line\n" {
		t.Fatalf("first line = %q, %v; want the one the handler flushed", line, readErr)
	}

	server.Stop()

	rest, readErr := io.ReadAll(body)
	if readErr == nil {
		t.Fatalf("the stream went on to its end after Stop (%q), want it cut", rest)
	}
	var netErr net.Error
	if errors.As(readErr, &netErr) && netErr.Timeout() {
		t.Errorf("the rest of the stream waited out its deadline (%v), want it cut at once", readErr)
	}
}

// TestStoppableServer_TheTestEnding_ReleasesThePort verifies the server is
// closed with the test that started it, stopped or not. The subtest has to
// have ended before the port is asked for again, so neither runs in parallel.
func TestStoppableServer_TheTestEnding_ReleasesThePort(t *testing.T) {
	var url string
	t.Run("stopped", func(t *testing.T) {
		server := NewStoppableServer(t, http.NotFoundHandler())
		server.Stop()
		url = server.URL
	})
	if url == "" {
		t.Fatal("the subtest returned no address")
	}
	assertPortFree(t, url)
}
