// stubs.go stands in for the two services the server talks to, so a benchmark
// needs neither a GitLab instance nor a collector.
//
// The rule this follows is the one cmd/internal/mcpsurface states for the
// generators: a published artifact must not depend on the machine that
// produced it. A run against a real instance would fold that instance's
// latency, its rate limits and its network into every figure, and two people
// re-measuring would compare their GitLab installations rather than this
// server.

package main

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
)

// stubGitLab answers the handful of endpoints the server probes while it
// builds a pool entry: the version, the authenticated user, and a 404 for the
// scope and tier probes, which means "this instance will not say" and every
// caller handles.
//
// It also answers a project read, which is the one call the held-request mode
// makes it keep waiting: while a hold is on, every project read is held until
// the hold is released, so the server under measurement holds the call that
// asked for it.
type stubGitLab struct {
	url    string
	server *httptest.Server
	calls  atomic.Int64

	// mu guards release, the channel a held read waits on, nil while no hold
	// is on; held counts the reads waiting on it now.
	mu      sync.Mutex
	release chan struct{}
	held    atomic.Int64
}

// hold makes every project read from now on wait, and returns the function
// that lets the waiting ones through and ends the hold.
func (s *stubGitLab) hold() (release func()) {
	gate := make(chan struct{})
	s.mu.Lock()
	s.release = gate
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		s.release = nil
		s.mu.Unlock()
		close(gate)
	}
}

// holding is how many project reads are waiting on the hold now.
func (s *stubGitLab) holding() int64 { return s.held.Load() }

// answerProject answers a project read, after the hold is released when one is
// on. The headers are written before the wait rather than after it: the server
// gives GitLab a minute to answer with headers and none to finish a body, so a
// read held before its headers would be abandoned and retried by the server
// after a minute, and the step would measure the retry instead of the hold.
func (s *stubGitLab) answerProject(w http.ResponseWriter, r *http.Request) {
	s.calls.Add(1)
	w.Header().Set(headerContentType, mediaJSON)
	s.mu.Lock()
	gate := s.release
	s.mu.Unlock()
	if gate != nil {
		w.WriteHeader(http.StatusOK)
		// A writer that cannot flush still delivers the body at the end.
		_ = http.NewResponseController(w).Flush()
		s.held.Add(1)
		select {
		case <-gate:
		case <-r.Context().Done():
		}
		s.held.Add(-1)
	}
	_, _ = w.Write([]byte(`{"id":1,"name":"benchmark","path":"benchmark","path_with_namespace":"benchmark/benchmark"}`))
}

// startStubGitLab starts the stand-in instance on loopback.
func startStubGitLab() *stubGitLab {
	stub := &stubGitLab{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		stub.calls.Add(1)
		w.Header().Set(headerContentType, mediaJSON)
		_, _ = w.Write([]byte(`{"version":"17.0.0","revision":"benchmark"}`))
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		stub.calls.Add(1)
		w.Header().Set(headerContentType, mediaJSON)
		_, _ = w.Write([]byte(`{"id":1,"username":"benchmark","name":"benchmark"}`))
	})
	mux.HandleFunc("GET /api/v4/projects/{id}", stub.answerProject)
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		stub.calls.Add(1)
		w.WriteHeader(http.StatusNotFound)
	})
	stub.server = httptest.NewServer(mux)
	stub.url = stub.server.URL
	return stub
}

// close shuts the stand-in instance down.
func (s *stubGitLab) close() { s.server.Close() }

// otlpSink accepts OTLP/HTTP exports and drops them.
//
// The telemetry scenarios measure what exporting costs the server, not what a
// collector does with the payload, so the sink answers the cheapest valid
// response there is: 200 with an empty body, which is a well-formed empty
// export response in protobuf and stops the exporter from retrying.
type otlpSink struct {
	url      string
	server   *httptest.Server
	requests atomic.Int64
	bytes    atomic.Int64
}

// startOTLPSink starts the receiver on loopback.
func startOTLPSink() *otlpSink {
	sink := &otlpSink{}
	sink.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sink.requests.Add(1)
		sink.bytes.Add(drain(r))
		w.Header().Set(headerContentType, mediaProtobuf)
		w.WriteHeader(http.StatusOK)
	}))
	sink.url = sink.server.URL
	return sink
}

// drain reads and discards a request body, returning how many bytes it held.
func drain(r *http.Request) int64 {
	if r.Body == nil {
		return 0
	}
	defer func() { _ = r.Body.Close() }()
	var total int64
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Body.Read(buf)
		total += int64(n)
		if err != nil {
			return total
		}
	}
}

// close shuts the sink down.
func (s *otlpSink) close() { s.server.Close() }
