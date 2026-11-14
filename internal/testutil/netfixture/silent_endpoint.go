// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package netfixture

import (
	"errors"
	"io"
	"net"
	"net/url"
	"sync"
	"testing"
	"time"
)

// SilentEndpoint returns the base URL, "http://127.0.0.1:port", of an address
// that accepts every connection and never answers on it, for the rest of the
// test.
//
// It is the instance that drops packets after the handshake, or a proxy that
// accepted the connection and lost its upstream: a request sent there is
// written, read and discarded, and waits for a response that never comes
// until the client's own deadline ends it. That is what a [DeadEndpoint] or a
// stopped [StoppableServer] cannot stand for, since both hang up at once, so
// a case built on them passes whether or not the code under test bounds its
// wait. A test of a deadline needs this one.
//
// Every connection is held open until the cleanup SilentEndpoint registers
// closes the listener and every connection it accepted, and waits for the
// fixture's goroutines to end. Cleanups run last registered first, so the
// port and the connections are held through every cleanup registered after
// the call, and released before any registered earlier.
func SilentEndpoint(tb testing.TB) string {
	tb.Helper()
	address, err := silentEndpoint(tb)
	if err != nil {
		tb.Fatalf("netfixture: open a silent endpoint: %v", err)
	}
	return address
}

// silentEndpoint opens the listener and starts the loop that holds each
// connection.
func silentEndpoint(tb testing.TB) (string, error) {
	tb.Helper()
	listener, err := listen(tb.Context())
	if err != nil {
		return "", err
	}
	held := &heldConns{}
	done := make(chan struct{})
	go holdEach(listener, held, done)
	tb.Cleanup(func() {
		_ = listener.Close()
		<-done
		held.closeAll()
	})
	return (&url.URL{Scheme: "http", Host: listener.Addr().String()}).String(), nil
}

// heldConns is every connection a silent endpoint accepted, kept so its
// cleanup can end them, and the goroutines that read them.
type heldConns struct {
	mu      sync.Mutex
	conns   []net.Conn
	readers sync.WaitGroup
}

// hold keeps conn open and reads whatever the client sends into nothing,
// until conn is closed, so a client writing its request never blocks on a
// full socket buffer and never reads a byte back.
func (h *heldConns) hold(conn net.Conn) {
	h.mu.Lock()
	h.conns = append(h.conns, conn)
	h.mu.Unlock()
	h.readers.Go(func() {
		_, _ = io.Copy(io.Discard, conn)
	})
}

// closeAll closes every connection held and waits for their readers.
func (h *heldConns) closeAll() {
	h.mu.Lock()
	for _, conn := range h.conns {
		_ = conn.Close()
	}
	h.mu.Unlock()
	h.readers.Wait()
}

// holdEach accepts connections and holds each one, until the listener is
// closed, and then closes done. It never touches the test, so it stays within
// the rule for code that runs off the test goroutine.
//
// An accept that fails for any other reason is retried after a pause, for the
// reason [hangUpOnEach] gives: a loop that ended there would leave the port
// held with nothing accepting.
func holdEach(listener net.Listener, held *heldConns, done chan<- struct{}) {
	defer close(done)
	for {
		conn, err := listener.Accept()
		if errors.Is(err, net.ErrClosed) {
			return
		}
		if err != nil {
			time.Sleep(time.Millisecond)
			continue
		}
		held.hold(conn)
	}
}
