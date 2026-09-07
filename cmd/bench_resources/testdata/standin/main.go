// Command standin is the server the benchmark's tests measure instead of the
// real one.
//
// The harness in cmd/bench_resources starts a binary, waits for its health
// document, drives it over HTTP or over its pipes, samples its resident set and
// finally sends it a traceback signal and counts what it printed. Every one of
// those steps is process-level and none of them can be exercised against an
// in-process fake; and building the real server for every test run would cost
// a minute of linking to learn nothing about the harness. This program answers
// the same contract in a few hundred lines: the flags the HTTP target passes,
// the environment the stdio target sets, a /health document, JSON-RPC over SSE
// and over newline-delimited pipes, and a runtime that dumps its goroutines on
// SIGQUIT because nothing here catches it.
//
// STANDIN_FAIL names one JSON-RPC method the stand-in refuses with an error
// result, which is how the tests reach the harness's failure branches without
// a server that is actually broken.
//
// A positive --rate-limit-rps turns on a per-credential token bucket over the
// method STANDIN_REFUSE_METHOD names, refusing in the two shapes the real
// limiter uses. It exists so the fairness scenario's accounting, its positive
// control and the difference between its two arms can be driven without a real
// server; the shapes are exact because telling a refusal from a failure is the
// one thing that harness does which a canned answer would let drift.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const (
	failEnv = "STANDIN_FAIL"
	// refuseEnv names the method the crude bound meters; tools/call when unset,
	// which is what the shipped limiter meters first.
	refuseEnv = "STANDIN_REFUSE_METHOD"
	version   = "standin"
	commit    = "0123456789abcdef0123456789abcdef01234567"
	// The refusal the real limiter writes, in both of its shapes.
	refusalPrefix = "rate limit exceeded for "
	refusalSuffix = "; retry after a short backoff"
	refusalCode   = -42900
)

// request is the subset of a JSON-RPC request the stand-in reads.
type request struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	// credential and tool come from the request's headers rather than its body,
	// because the bound is per credential and the refusal names the tool.
	credential string
	tool       string
}

// bound is the per-credential limit a positive --rate-limit-rps turns on.
//
// One token bucket per credential and method, which is the real limiter's
// shape and not an approximation of it. A counter would have been simpler and
// wrong for this harness in particular: the fairness scenario spends an
// unmeasured lead-in draining the burst so that its measured window reports
// the bound rather than a bucket that started full, and a cumulative counter
// has nothing to drain and never refills, so a quiet population offering well
// under the limit would be refused as surely as a noisy one.
type bound struct {
	mu      sync.Mutex
	on      bool
	method  string
	rps     float64
	burst   int
	buckets map[string]*rate.Limiter
}

// allow reports whether this credential may have this request now.
func (b *bound) allow(req request) bool {
	if !b.on || req.Method != b.method {
		return true
	}
	b.mu.Lock()
	key := req.credential + " " + req.Method
	bucket, ok := b.buckets[key]
	if !ok {
		bucket = rate.NewLimiter(rate.Limit(b.rps), b.burst)
		b.buckets[key] = bucket
	}
	b.mu.Unlock()
	return bucket.Allow()
}

// limit is the process-wide bound, built from the flags in main.
var limit = &bound{buckets: map[string]*rate.Limiter{}}

// rpcError is a JSON-RPC error object.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func main() {
	httpMode := flag.Bool("http", false, "serve HTTP instead of stdio")
	addr := flag.String("http-addr", "", "listen address in HTTP mode")
	flag.String("gitlab-url", "", "accepted and ignored")
	flag.String("tool-surface", "", "accepted and ignored")
	rps := flag.Float64("rate-limit-rps", 0, "positive turns on the crude per-credential bound below")
	burst := flag.Int("rate-limit-burst", 1, "requests of the metered method served per credential before the bound refuses")
	flag.Int("max-http-clients", 0, "accepted and ignored")
	telemetry := flag.Bool("telemetry", false, "send one export to OTEL_EXPORTER_OTLP_ENDPOINT, as the real server's exporters would")
	pprofAddr := flag.String("pprof-addr", "", "serve net/http/pprof on this address, on a listener of its own, as the real server does")
	flag.Parse()

	limit.on = *rps > 0
	limit.rps = *rps
	limit.burst = max(1, *burst)
	limit.method = os.Getenv(refuseEnv)
	if limit.method == "" {
		limit.method = "tools/call"
	}

	if *telemetry {
		exportTelemetry(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	}
	if *pprofAddr != "" {
		// The real thing rather than a stand-in for it: the series reads CPU
		// and heap profiles and the goroutine total off this listener, and a
		// canned answer would let the harness's parsing drift from what the
		// runtime actually prints.
		listener, err := net.Listen("tcp", *pprofAddr)
		if err != nil {
			fmt.Fprintln(os.Stderr, "standin: pprof:", err)
			os.Exit(1)
		}
		mux := http.NewServeMux()
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		go func() {
			_ = (&http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}).Serve(listener)
		}()
	}

	if *httpMode {
		if err := serveHTTP(*addr); err != nil {
			fmt.Fprintln(os.Stderr, "standin:", err)
			os.Exit(1)
		}
		return
	}
	// The stdio target configures its process through the environment, so a
	// stand-in that started without it would hide a harness that stopped
	// setting it.
	if os.Getenv("GITLAB_URL") == "" || os.Getenv("GITLAB_TOKEN") == "" {
		fmt.Fprintln(os.Stderr, "standin: stdio mode needs GITLAB_URL and GITLAB_TOKEN in the environment")
		os.Exit(1)
	}
	serveStdio(os.Stdin, os.Stdout)
}

// serveHTTP answers /health and /mcp until the process is killed.
// exportTelemetry posts one metrics export to the OTLP endpoint the harness
// configured, so a run with telemetry on reaches the sink the way the real
// server's exporters do and a harness that stopped wiring the endpoint is
// caught. Best effort: the sink's answer changes nothing here, and an
// unreachable one is reported and otherwise ignored, as an exporter would.
func exportTelemetry(endpoint string) {
	if endpoint == "" {
		fmt.Fprintln(os.Stderr, "standin: --telemetry without OTEL_EXPORTER_OTLP_ENDPOINT, nothing exported")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	body := strings.NewReader(`{"resourceMetrics":[]}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(endpoint, "/")+"/v1/metrics", body)
	if err != nil {
		fmt.Fprintln(os.Stderr, "standin: telemetry export:", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "standin: telemetry export:", err)
		return
	}
	_ = resp.Body.Close()
}

func serveHTTP(addr string) error {
	if addr == "" {
		return fmt.Errorf("--http needs --http-addr")
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "ok", "version": version, "commit": commit,
		})
	})
	mux.HandleFunc("POST /mcp", handleMCP)
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	return server.Serve(listener)
}

// handleMCP checks the headers a 2026-07-28 client must send and answers in
// the SSE shape the real server uses by default.
func handleMCP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("PRIVATE-TOKEN") == "" {
		http.Error(w, "missing credential", http.StatusUnauthorized)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "unreadable body", http.StatusBadRequest)
		return
	}
	var req request
	if unmarshalErr := json.Unmarshal(body, &req); unmarshalErr != nil {
		http.Error(w, "malformed request", http.StatusBadRequest)
		return
	}
	if r.Header.Get("Mcp-Method") != req.Method {
		http.Error(w, "Mcp-Method does not name the request's method", http.StatusBadRequest)
		return
	}
	req.credential = r.Header.Get("PRIVATE-TOKEN")
	req.tool = r.Header.Get("Mcp-Name")
	w.Header().Set("Content-Type", "text/event-stream")
	fmt.Fprintf(w, "event: message\ndata: %s\n\n", respond(req))
}

// serveStdio answers one newline-delimited request per line until the input
// closes.
func serveStdio(in io.Reader, out io.Writer) {
	var mu sync.Mutex
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 64*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			continue
		}
		mu.Lock()
		_, _ = out.Write(append(respond(req), '\n'))
		mu.Unlock()
	}
}

// respond builds the response for one request.
func respond(req request) []byte {
	var result any
	var failure *rpcError
	switch {
	case !limit.allow(req):
		result, failure = refusal(req)
	case os.Getenv(failEnv) == req.Method:
		failure = &rpcError{Code: -32000, Message: "the stand-in refused " + req.Method}
	case req.Method == "tools/list":
		result = map[string]any{"tools": []map[string]any{
			{"name": "gitlab_find_action", "description": "find"},
			{"name": "gitlab_execute_action", "description": "execute"},
		}}
	case req.Method == "resources/list":
		result = map[string]any{"resources": []any{}}
	case req.Method == "tools/call":
		result = map[string]any{
			"content": []map[string]any{{"type": "text", "text": "ok"}},
			"isError": false,
		}
	default:
		failure = &rpcError{Code: -32601, Message: "method not found: " + req.Method}
	}

	return encode(req, result, failure)
}

// refusal is what the bound answers with, in whichever of the two shapes the
// method's result can carry.
//
// tools/call comes back as a **successful** response whose result is flagged
// isError, with the message as its only mark; every other method comes back as
// a JSON-RPC error carrying the 429-mirroring code. Both are HTTP 200, which
// is what separates them from the per-address lockout the harness must not
// count as this bound's refusal.
func refusal(req request) (any, *rpcError) {
	if req.Method == "tools/call" {
		name := req.tool
		if name == "" {
			name = req.Method
		}
		return map[string]any{
			"content": []map[string]any{{"type": "text", "text": refusalPrefix + name + refusalSuffix}},
			"isError": true,
		}, nil
	}
	return nil, &rpcError{Code: refusalCode, Message: refusalPrefix + req.Method + refusalSuffix}
}

// encode wraps a result or a failure in the JSON-RPC envelope.
func encode(req request, result any, failure *rpcError) []byte {
	envelope := map[string]any{"jsonrpc": "2.0", "id": req.ID}
	if failure != nil {
		envelope["error"] = failure
	} else {
		envelope["result"] = result
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		return []byte(`{"jsonrpc":"2.0","id":null,"error":{"code":-32603,"message":"encode"}}`)
	}
	return payload
}
