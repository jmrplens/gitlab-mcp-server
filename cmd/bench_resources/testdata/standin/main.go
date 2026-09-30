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
//
// A gitlab_execute_action call running project.get reads the project from the
// instance --gitlab-url names and answers once that read has, which is what
// makes a call the held-request mode's stand-in instance holds a call this
// process holds too; the call must carry the action in Mcp-Param-Action, as the
// real transport demands. STANDIN_HELD_LIMIT, when positive, answers every
// POST past that many in flight with a 503, the shape the real server's held
// ceiling refuses with.
//
// --stateless=false serves the stateful transport the sessions mode drives: an
// initialize posted with no Mcp-Session-Id opens a session and answers with
// its id, a request on an id the stand-in does not hold is answered 404, a GET
// on a held id opens that session's standalone stream and keeps it open, and
// a DELETE ends the session. STANDIN_SESSION_LIMIT, when positive, answers an
// initialize past that many sessions with a 503, the shape the real server's
// session ceiling refuses with.
//
// --auth-mode=oauth admits a bearer token the way the real bearer guard does:
// one it has verified before at once, and any other only once the instance
// --gitlab-url names has answered GET /api/v4/user for it, with at most
// STANDIN_VERIFY_SLOTS of those verifications at once when that is positive.
// A token that waits STANDIN_VERIFY_WAIT (100ms unless set) for a slot is
// refused 503, one the instance refuses is refused 401, both in the real
// guard's codes and words, which is what the verification bound's fairness
// run has to tell apart.
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
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

const (
	failEnv = "STANDIN_FAIL"
	// refuseEnv names the method the crude bound meters; tools/call when unset,
	// which is what the shipped limiter meters first.
	refuseEnv = "STANDIN_REFUSE_METHOD"
	// noVersionEnv makes /health answer with an empty version, which is the
	// shape of a build that does not name itself: an older server, or one
	// behind a proxy that rewrote the document. The harness records the build
	// from whichever scenario could ask, so what it does with an answer that
	// names none is a branch only a server like this reaches.
	noVersionEnv = "STANDIN_NO_VERSION"
	version      = "standin"
	commit       = "0123456789abcdef0123456789abcdef01234567"
	// The refusal the real limiter writes, in both of its shapes.
	refusalPrefix = "rate limit exceeded for "
	refusalSuffix = "; retry after a short backoff"
	refusalCode   = -42900
	// heldLimitEnv caps the POSTs served at once, as the real held ceiling
	// does, and heldRefusal is the first line of the answer past it.
	heldLimitEnv = "STANDIN_HELD_LIMIT"
	heldRefusal  = "This server is busy. Retry later."
	// sessionLimitEnv caps the sessions held at once, as the real session
	// ceiling does, and refuses past it with the same words.
	sessionLimitEnv = "STANDIN_SESSION_LIMIT"
	sessionHeader   = "Mcp-Session-Id"
	// The one tool and action whose call reaches the instance.
	executeTool   = "gitlab_execute_action"
	projectAction = "project.get"
	// verifySlotsEnv caps the verifications run at once in OAuth mode, and
	// verifyWaitEnv is how long a token waits for one of them.
	verifySlotsEnv = "STANDIN_VERIFY_SLOTS"
	verifyWaitEnv  = "STANDIN_VERIFY_WAIT"
	// The real guard's refusals of a token it could not verify and of one the
	// instance refused, in its codes and its leading words.
	busyCode     = -50300
	busyRefusal  = "GitLab could not verify this token right now. Retry shortly. The token itself has not been rejected."
	upstreamLost = "GitLab could not verify this token right now; the instance is unreachable or throttling."
	rejectedCode = -40100
	rejected     = "GitLab rejected this token. Check that it is valid, unexpired, and issued by the target instance."
)

// request is the subset of a JSON-RPC request the stand-in reads.
type request struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params struct {
		Name      string `json:"name"`
		Arguments struct {
			Action string `json:"action"`
			Params struct {
				ProjectID string `json:"project_id"`
			} `json:"params"`
		} `json:"arguments"`
	} `json:"params"`
	// credential and tool come from the request's headers rather than its body,
	// because the bound is per credential and the refusal names the tool.
	credential string
	tool       string
}

// gitlabURL is the instance a project read goes to, from --gitlab-url.
var gitlabURL string

// inFlight counts the POSTs being served now, against heldLimit.
var (
	inFlight  atomic.Int64
	heldLimit int64
)

// sessions are what --stateless=false holds: every id the stand-in gave out and
// has not seen deleted, each with a channel closed when it ends, so a
// standalone stream open on it ends with it.
var sessions = struct {
	sync.Mutex
	stateful bool
	limit    int
	next     int
	open     map[string]chan struct{}
}{open: map[string]chan struct{}{}}

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

// oauthGate admits bearer tokens in OAuth mode, verifying each one the first
// time it is seen under a ceiling on how many are verified at once.
type oauthGate struct {
	on bool
	// slots is the ceiling, nil for none; wait is how long a token waits for
	// a slot before it is refused.
	slots chan struct{}
	wait  time.Duration

	mu       sync.Mutex
	verified map[string]bool
}

// gate is the process's OAuth gate, built from the flags in main.
var gate = &oauthGate{verified: map[string]bool{}}

// admit answers whether a token may be served, with the status and the error a
// refusal carries.
func (g *oauthGate) admit(ctx context.Context, token string) (int, *rpcError) {
	if token == "" {
		return http.StatusUnauthorized, &rpcError{Code: rejectedCode, Message: "Authentication required: send an OAuth access token"}
	}
	if g.known(token) {
		return http.StatusOK, nil
	}
	if g.slots != nil {
		timer := time.NewTimer(g.wait)
		defer timer.Stop()
		select {
		case g.slots <- struct{}{}:
			defer func() { <-g.slots }()
		case <-timer.C:
			return http.StatusServiceUnavailable, &rpcError{Code: busyCode, Message: busyRefusal}
		case <-ctx.Done():
			return http.StatusServiceUnavailable, &rpcError{Code: busyCode, Message: busyRefusal}
		}
	}
	valid, err := askUser(ctx, token)
	switch {
	case err != nil:
		return http.StatusServiceUnavailable, &rpcError{Code: busyCode, Message: upstreamLost}
	case !valid:
		return http.StatusUnauthorized, &rpcError{Code: rejectedCode, Message: rejected}
	}
	g.mu.Lock()
	g.verified[token] = true
	g.mu.Unlock()
	return http.StatusOK, nil
}

// known reports a token verified before.
func (g *oauthGate) known(token string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.verified[token]
}

// askUser asks the instance who a token belongs to, and reports whether it
// answered with anybody.
func askUser(ctx context.Context, token string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(gitlabURL, "/")+"/api/v4/user", http.NoBody)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == http.StatusOK, nil
}

// writeGateRefusal answers a request the gate refused, the way the real gate
// does: the status, and a JSON-RPC error in the body.
func writeGateRefusal(w http.ResponseWriter, status int, failure *rpcError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": nil, "error": failure})
}

// rpcError is a JSON-RPC error object.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func main() {
	httpMode := flag.Bool("http", false, "serve HTTP instead of stdio")
	addr := flag.String("http-addr", "", "listen address in HTTP mode")
	flag.StringVar(&gitlabURL, "gitlab-url", "", "the instance a project read goes to")
	flag.String("tool-surface", "", "accepted and ignored")
	rps := flag.Float64("rate-limit-rps", 0, "positive turns on the crude per-credential bound below")
	burst := flag.Int("rate-limit-burst", 1, "requests of the metered method served per credential before the bound refuses")
	flag.Int("max-http-clients", 0, "accepted and ignored")
	stateless := flag.Bool("stateless", true, "false serves the stateful transport, with sessions")
	telemetry := flag.Bool("telemetry", false, "send one export to OTEL_EXPORTER_OTLP_ENDPOINT, as the real server's exporters would")
	pprofAddr := flag.String("pprof-addr", "", "serve net/http/pprof on this address, on a listener of its own, as the real server does")
	authMode := flag.String("auth-mode", "legacy", "oauth admits bearer tokens through the gate above")
	flag.String("public-url", "", "accepted and ignored")
	flag.String("trusted-proxies", "", "accepted and ignored")
	flag.String("trusted-proxy-header", "", "accepted and ignored")
	flag.Parse()

	gate.on = *authMode == "oauth"
	gate.wait = 100 * time.Millisecond
	if raw := os.Getenv(verifyWaitEnv); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 {
			fmt.Fprintln(os.Stderr, "standin: "+verifyWaitEnv+" must be a positive duration")
			os.Exit(1)
		}
		gate.wait = parsed
	}
	if raw := os.Getenv(verifySlotsEnv); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			fmt.Fprintln(os.Stderr, "standin: "+verifySlotsEnv+" must be a count, zero for no ceiling")
			os.Exit(1)
		}
		if parsed > 0 {
			gate.slots = make(chan struct{}, parsed)
		}
	}

	limit.on = *rps > 0
	limit.rps = *rps
	limit.burst = max(1, *burst)
	limit.method = os.Getenv(refuseEnv)
	if limit.method == "" {
		limit.method = "tools/call"
	}
	if raw := os.Getenv(heldLimitEnv); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed <= 0 {
			fmt.Fprintln(os.Stderr, "standin: "+heldLimitEnv+" must be a positive count")
			os.Exit(1)
		}
		heldLimit = parsed
	}
	sessions.stateful = !*stateless
	if raw := os.Getenv(sessionLimitEnv); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			fmt.Fprintln(os.Stderr, "standin: "+sessionLimitEnv+" must be a positive count")
			os.Exit(1)
		}
		sessions.limit = parsed
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
	reported := version
	if _, silent := os.LookupEnv(noVersionEnv); silent {
		reported = ""
	}
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "ok", "version": reported, "commit": commit,
		})
	})
	mux.HandleFunc("POST /mcp", handleMCP)
	if sessions.stateful {
		mux.HandleFunc("GET /mcp", handleStream)
		mux.HandleFunc("DELETE /mcp", handleDelete)
	}
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	return server.Serve(listener)
}

// handleMCP checks the headers a 2026-07-28 client must send and answers in
// the SSE shape the real server uses by default.
func handleMCP(w http.ResponseWriter, r *http.Request) {
	if heldLimit > 0 {
		held := inFlight.Add(1)
		defer inFlight.Add(-1)
		if held > heldLimit {
			http.Error(w, heldRefusal, http.StatusServiceUnavailable)
			return
		}
	}
	credential := r.Header.Get("PRIVATE-TOKEN")
	if gate.on {
		credential, _ = strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if status, failure := gate.admit(r.Context(), credential); failure != nil {
			writeGateRefusal(w, status, failure)
			return
		}
	}
	if credential == "" {
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
	if req.Params.Name == executeTool && r.Header.Get("Mcp-Param-Action") != req.Params.Arguments.Action {
		http.Error(w, "Mcp-Param-Action does not name the call's action", http.StatusBadRequest)
		return
	}
	req.credential = credential
	req.tool = r.Header.Get("Mcp-Name")
	if sessions.stateful && !servesSession(w, r, &req) {
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	fmt.Fprintf(w, "event: message\ndata: %s\n\n", respond(req))
}

// servesSession applies the stateful transport to a POST, and reports whether
// the request is left to be answered: an initialize with no id opens a session
// and names it in the answer's header, a notification on a held id is taken
// with 202 and nothing else, and anything on an id the stand-in does not hold,
// or anything but an initialize on none, is refused.
func servesSession(w http.ResponseWriter, r *http.Request, req *request) bool {
	id := r.Header.Get(sessionHeader)
	if id == "" {
		if req.Method != "initialize" {
			http.Error(w, "only initialize opens a session", http.StatusBadRequest)
			return false
		}
		opened, ok := openSession()
		if !ok {
			http.Error(w, heldRefusal, http.StatusServiceUnavailable)
			return false
		}
		w.Header().Set(sessionHeader, opened)
		return true
	}
	if sessionEnded(id) == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return false
	}
	if len(req.ID) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return false
	}
	return true
}

// openSession gives out a new session id, or reports that the limit is reached.
func openSession() (string, bool) {
	sessions.Lock()
	defer sessions.Unlock()
	if sessions.limit > 0 && len(sessions.open) >= sessions.limit {
		return "", false
	}
	sessions.next++
	id := "standin-session-" + strconv.Itoa(sessions.next)
	sessions.open[id] = make(chan struct{})
	return id, true
}

// sessionEnded is the channel closed when a held session ends, or nil for an
// id the stand-in does not hold.
func sessionEnded(id string) chan struct{} {
	sessions.Lock()
	defer sessions.Unlock()
	return sessions.open[id]
}

// handleStream opens a session's standalone stream and holds it until the
// client leaves or the session ends.
func handleStream(w http.ResponseWriter, r *http.Request) {
	ended := sessionEnded(r.Header.Get(sessionHeader))
	if ended == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	_ = http.NewResponseController(w).Flush()
	select {
	case <-r.Context().Done():
	case <-ended:
	}
}

// handleDelete ends a session.
func handleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.Header.Get(sessionHeader)
	sessions.Lock()
	ended, ok := sessions.open[id]
	delete(sessions.open, id)
	sessions.Unlock()
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	close(ended)
	w.WriteHeader(http.StatusNoContent)
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
	case req.Method == "initialize":
		result = map[string]any{
			"protocolVersion": "2025-11-25",
			"capabilities":    map[string]any{},
			"serverInfo":      map[string]any{"name": "standin", "version": version},
		}
	case req.Method == "ping":
		result = map[string]any{}
	case req.Method == "tools/call":
		if err := readProjectFor(req); err != nil {
			failure = &rpcError{Code: -32603, Message: err.Error()}
			break
		}
		result = map[string]any{
			"content": []map[string]any{{"type": "text", "text": "ok"}},
			"isError": false,
		}
	default:
		failure = &rpcError{Code: -32601, Message: "method not found: " + req.Method}
	}

	return encode(req, result, failure)
}

// readProjectFor reads the project a project.get call names from the instance,
// and returns once the instance has answered in full; any other call reads
// nothing. The whole body is read because the instance holds a read after its
// headers, so the answer is only complete once the hold is released.
func readProjectFor(req request) error {
	if req.Params.Name != executeTool || req.Params.Arguments.Action != projectAction {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	target := strings.TrimSuffix(gitlabURL, "/") + "/api/v4/projects/" + req.Params.Arguments.Params.ProjectID
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, target, http.NoBody)
	if err != nil {
		return fmt.Errorf("project read: %w", err)
	}
	httpReq.Header.Set("PRIVATE-TOKEN", req.credential)
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("project read: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if _, copyErr := io.Copy(io.Discard, resp.Body); copyErr != nil {
		return fmt.Errorf("project read: %w", copyErr)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("project read: HTTP %d", resp.StatusCode)
	}
	return nil
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
