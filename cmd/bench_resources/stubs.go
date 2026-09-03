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
	"sync/atomic"
)

// stubGitLab answers the handful of endpoints the server probes while it
// builds a pool entry: the version, the authenticated user, and a 404 for the
// scope and tier probes, which means "this instance will not say" and every
// caller handles.
type stubGitLab struct {
	url    string
	server *httptest.Server
	calls  atomic.Int64
}

// startStubGitLab starts the stand-in instance on loopback.
func startStubGitLab() *stubGitLab {
	stub := &stubGitLab{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		stub.calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"17.0.0","revision":"benchmark"}`))
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		stub.calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"username":"benchmark","name":"benchmark"}`))
	})
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
		w.Header().Set("Content-Type", "application/x-protobuf")
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
