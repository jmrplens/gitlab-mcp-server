package netfixture

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// StoppableServer is an HTTP server on a loopback port that answers through
// its handler until [StoppableServer.Stop] is called, and from then on hangs
// up on every request without answering. It keeps its port, stopped or not,
// until the cleanup [NewStoppableServer] registers closes it, which runs
// after every cleanup registered later and before any registered earlier.
//
// It is for the test whose instance has to answer first and then go away: a
// client bound to the address, a cache keyed on it. Closing an httptest
// server gave that test its "gone" by freeing the port, which anything else
// could then be given.
type StoppableServer struct {
	// URL is the server's base URL, "http://127.0.0.1:port", the same before
	// and after Stop.
	URL     string
	server  *httptest.Server
	stopped atomic.Bool
}

// NewStoppableServer starts a server answering through handler, closed by a
// cleanup it registers on tb.
func NewStoppableServer(tb testing.TB, handler http.Handler) *StoppableServer {
	tb.Helper()
	s := &StoppableServer{}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.stopped.Load() {
			// The server closes the connection without a response and without
			// logging, which is what net/http documents this value for.
			panic(http.ErrAbortHandler)
		}
		handler.ServeHTTP(w, r)
	}))
	s.URL = s.server.URL
	tb.Cleanup(s.server.Close)
	return s
}

// Stop makes the server hang up on every request from now on, including one
// sent on a connection a client kept open from before, and closes the
// connections already open, so a response in progress ends where it is. The
// port stays held. Calling it again changes nothing.
func (s *StoppableServer) Stop() {
	s.stopped.Store(true)
	s.server.CloseClientConnections()
}
