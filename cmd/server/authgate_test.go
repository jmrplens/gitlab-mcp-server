package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/serverpool"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const gateTestToken = "glpat-test-token-value"

// gateStubGitLab returns a GitLab stub for the pool's credential probe. It
// accepts every token unless reject is true, in which case it answers 401.
func gateStubGitLab(t *testing.T, reject bool) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		if reject {
			http.Error(w, `{"message":"401 Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":42,"username":"testuser"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

// newGateTestPool builds a pool against a stub instance. The tier is pinned and
// scope detection is off, so the only GitLab round-trip is the credential
// probe — and it is served locally rather than over the real network.
func newGateTestPool(t *testing.T, factory serverpool.ServerFactory, gitlabURL string) *serverpool.ServerPool {
	t.Helper()
	cfg := &config.Config{
		GitLabURL:    gitlabURL,
		Tier:         edition.Free,
		TierExplicit: true,
		IgnoreScopes: true,
	}
	return serverpool.New(cfg, factory)
}

// okFactory returns a minimal server, standing in for a fully registered one.
func okFactory(_ *gitlabclient.Client, _ *config.ServerConfig) (*mcp.Server, error) {
	return mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil), nil
}

func failingFactory(_ *gitlabclient.Client, _ *config.ServerConfig) (*mcp.Server, error) {
	return nil, errors.New("internal pool detail that must not reach the client")
}

// newGate wires a gate the way registerLegacyMCPHandlers does, against a stub
// GitLab that accepts any credential.
func newGate(t *testing.T, factory serverpool.ServerFactory) *mcpServerGate {
	t.Helper()
	return newGateAgainst(t, factory, gateStubGitLab(t, false))
}

// newGateAgainst wires a gate against a specific GitLab base URL.
func newGateAgainst(t *testing.T, factory serverpool.ServerFactory, gitlabURL string) *mcpServerGate {
	t.Helper()
	return &mcpServerGate{
		pool:       newGateTestPool(t, factory, gitlabURL),
		gitlabURLs: []string{gitlabURL},
		limiter:    serverpool.NewAuthRateLimiter(authFailureLimit, authFailureWindow),
		challenge:  legacyAuthChallenge,
	}
}

// decodeJSONRPCError asserts the body is a JSON-RPC error response and returns
// it.
//
// wantID is the id the response must carry, written as it appears on the wire;
// omit it for the requests that carry none, where the member must be absent
// rather than null. Null is what this used to send unconditionally, and it is
// not a legal RequestId: under 2026-07-28 the member is a string or an integer,
// and optional so that an unknown id can be left out.
func decodeJSONRPCError(t *testing.T, body string, wantID ...string) jsonRPCError {
	t.Helper()
	var decoded jsonRPCError
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("body is not JSON: %v (body=%q)", err, body)
	}
	if decoded.JSONRPC != "2.0" {
		t.Errorf("jsonrpc = %q, want \"2.0\"", decoded.JSONRPC)
	}
	want := ""
	if len(wantID) > 0 {
		want = wantID[0]
	}
	if got := string(decoded.ID); got != want {
		if want == "" {
			t.Errorf("id = %s, want the member omitted (the request carried no id)", got)
		} else {
			t.Errorf("id = %q, want %q echoed back from the request", got, want)
		}
	}
	if decoded.Error.Message == "" {
		t.Error("error.message is empty; the rejection must say what went wrong")
	}
	return decoded
}

// TestMCPServerGate_MissingCredential_Returns401WithBearerChallenge verifies the
// core fix: a request with no credential must be a 401 carrying a
// WWW-Authenticate challenge, never the SDK's opaque 400.
//
// The challenge must NOT advertise resource_metadata, because legacy mode
// mounts no protected-resource document and clients would start an OAuth
// discovery flow that cannot complete.
func TestMCPServerGate_MissingCredential_Returns401WithBearerChallenge(t *testing.T) {
	gate := newGate(t, okFactory)
	rec := httptest.NewRecorder()

	gate.middleware(http.NotFoundHandler()).ServeHTTP(
		rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader("{}")),
	)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	challenge := rec.Header().Get("WWW-Authenticate")
	if challenge == "" {
		t.Fatal("401 without WWW-Authenticate violates RFC 9110")
	}
	if !strings.HasPrefix(challenge, "Bearer ") {
		t.Errorf("challenge = %q, want a Bearer challenge", challenge)
	}
	if strings.Contains(challenge, "resource_metadata") {
		t.Errorf("challenge = %q must not advertise OAuth discovery in legacy mode", challenge)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}

	decoded := decodeJSONRPCError(t, rec.Body.String())
	if decoded.Error.Code != errCodeUnauthorized {
		t.Errorf("error.code = %d, want %d", decoded.Error.Code, errCodeUnauthorized)
	}
	// The message is the only place the caller learns the accepted headers.
	for _, want := range []string{"PRIVATE-TOKEN", "Bearer"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(decoded.Error.Message, want) {
				t.Errorf("message %q does not mention %q", decoded.Error.Message, want)
			}
		})
	}
}

// TestMCPServerGate_OAuthChallenge_AdvertisesResourceMetadata is the mirror of
// the legacy case: in OAuth mode the discovery document does exist, so the
// challenge must point at it.
func TestMCPServerGate_OAuthChallenge_AdvertisesResourceMetadata(t *testing.T) {
	gate := newGate(t, okFactory)
	gate.limiter = nil // OAuth mode has no gate-level limiter
	gate.challenge = `Bearer resource_metadata="http://localhost:8080/.well-known/oauth-protected-resource"`
	rec := httptest.NewRecorder()

	gate.middleware(http.NotFoundHandler()).ServeHTTP(
		rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader("{}")),
	)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if got := rec.Header().Get("WWW-Authenticate"); !strings.Contains(got, "resource_metadata=") {
		t.Errorf("challenge = %q, want a resource_metadata parameter", got)
	}
}

// TestMCPServerGate_InvalidGitLabURLHeader_Returns400WithReason checks the
// second nil-returning branch. It stays a 400 — the request really is
// malformed — but gains a JSON-RPC body naming the offending header, so a
// client can tell it apart from a protocol-version rejection.
func TestMCPServerGate_InvalidGitLabURLHeader_Returns400WithReason(t *testing.T) {
	gate := newGate(t, okFactory)
	gate.gitlabURLs = nil // no fixed instance, so the header is authoritative
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader("{}"))
	req.Header.Set("PRIVATE-TOKEN", gateTestToken)
	req.Header.Set("GITLAB-URL", "://not a url")
	rec := httptest.NewRecorder()

	gate.middleware(http.NotFoundHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	decoded := decodeJSONRPCError(t, rec.Body.String())
	if decoded.Error.Code != errCodeInvalidRequest {
		t.Errorf("error.code = %d, want %d", decoded.Error.Code, errCodeInvalidRequest)
	}
	if !strings.Contains(decoded.Error.Message, "GITLAB-URL") {
		t.Errorf("message %q does not name the offending header", decoded.Error.Message)
	}
}

// TestMCPServerGate_PoolFailure_Returns503WithoutLeakingDetail covers the third
// branch. A backend that cannot be initialized is 503, not 400, and the pool's
// internal error text must stay in the logs.
func TestMCPServerGate_PoolFailure_Returns503WithoutLeakingDetail(t *testing.T) {
	gate := newGate(t, failingFactory)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader("{}"))
	req.Header.Set("PRIVATE-TOKEN", gateTestToken)
	rec := httptest.NewRecorder()

	gate.middleware(http.NotFoundHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	decoded := decodeJSONRPCError(t, rec.Body.String())
	if decoded.Error.Code != errCodeUpstreamUnavailable {
		t.Errorf("error.code = %d, want %d", decoded.Error.Code, errCodeUpstreamUnavailable)
	}
	if strings.Contains(decoded.Error.Message, "internal pool detail") {
		t.Errorf("message %q leaks the internal pool error", decoded.Error.Message)
	}
}

// TestMCPServerGate_CredentialGitLabRefuses_Returns401 covers the admission
// check: a token GitLab answers 401 to is an authentication failure, so it must
// surface as 401 with a challenge — not as the 503 every other pool error gets.
//
// This is also what stops a stream of invented tokens from churning the pool:
// each one is refused before it can occupy an entry.
func TestMCPServerGate_CredentialGitLabRefuses_Returns401(t *testing.T) {
	gate := newGateAgainst(t, okFactory, gateStubGitLab(t, true))
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader("{}"))
	req.Header.Set("PRIVATE-TOKEN", "glpat-invented")
	rec := httptest.NewRecorder()

	gate.middleware(http.NotFoundHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d (a refused credential is not a backend failure)",
			rec.Code, http.StatusUnauthorized)
	}
	if rec.Header().Get("WWW-Authenticate") == "" {
		t.Error("401 without WWW-Authenticate violates RFC 9110")
	}
	decoded := decodeJSONRPCError(t, rec.Body.String())
	if decoded.Error.Code != errCodeUnauthorized {
		t.Errorf("error.code = %d, want %d", decoded.Error.Code, errCodeUnauthorized)
	}
	if gate.pool.Size() != 0 {
		t.Errorf("pool size = %d, want 0 — a refused credential must not occupy an entry", gate.pool.Size())
	}
}

// TestMCPServerGate_PoolFailures_DoNotConsumeAuthRateLimit pins the boundary
// between "the credential was rejected" and "the backend was unreachable".
//
// A pool failure never judged the credential, so charging it to the
// authentication limiter would let a GitLab outage lock out clients holding
// perfectly valid tokens — the same conflation of causes this gate removes.
func TestMCPServerGate_PoolFailures_DoNotConsumeAuthRateLimit(t *testing.T) {
	gate := newGate(t, failingFactory)
	handler := gate.middleware(http.NotFoundHandler())

	// Far more pool failures than the limiter's threshold.
	for range authFailureLimit * 2 {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader("{}"))
		req.Header.Set("PRIVATE-TOKEN", gateTestToken)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
		}
	}

	// The limiter must be untouched: a credential-less request still gets the
	// ordinary 401, not the 429 that an exhausted limiter would produce.
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader("{}")))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d — backend failures must not consume the auth rate limit",
			rec.Code, http.StatusUnauthorized)
	}
}

// TestMCPServerGate_RepeatedAuthFailures_Returns429WithRetryAfter checks the
// fourth branch. Exhausting the limiter must report 429 with Retry-After rather
// than reusing the credential-missing 401.
func TestMCPServerGate_RepeatedAuthFailures_Returns429WithRetryAfter(t *testing.T) {
	gate := newGate(t, okFactory)
	handler := gate.middleware(http.NotFoundHandler())

	// Each credential-less POST records one failure; the limit blocks the next.
	for range authFailureLimit {
		handler.ServeHTTP(httptest.NewRecorder(),
			httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader("{}")))
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader("{}")))

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d after %d failures", rec.Code, http.StatusTooManyRequests, authFailureLimit)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("429 without Retry-After leaves the client guessing")
	}
	decoded := decodeJSONRPCError(t, rec.Body.String())
	if decoded.Error.Code != errCodeTooManyRequests {
		t.Errorf("error.code = %d, want %d", decoded.Error.Code, errCodeTooManyRequests)
	}
}

// TestMCPServerGate_NonPOSTMethods_ReachTheHandler guards both halves of the
// non-POST rule.
//
// On the default stateless transport, GET and DELETE must still reach the SDK
// so it answers 405, which is what protocol 2026-07-28 prescribes for them;
// gating them as 401 would replace a correct answer with a misleading one.
//
// On a stateful deployment they are live operations on a session — GET opens
// its standalone SSE stream, DELETE terminates it — so they are gated there.
// OPTIONS is exempt in both modes: it is a CORS preflight, which a browser
// sends without credentials by definition, so refusing it for lacking one would
// refuse the request that exists to ask whether the real request is allowed.
func TestMCPServerGate_NonPOSTMethods_ReachTheHandler(t *testing.T) {
	passesThrough := func(t *testing.T, gate *mcpServerGate, method string) bool {
		t.Helper()
		var reached atomic.Bool
		handler := gate.middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			reached.Store(true)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), method, "/", nil))
		if reached.Load() && rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s reached the handler but the status was %d", method, rec.Code)
		}
		return reached.Load()
	}

	for _, method := range []string{http.MethodGet, http.MethodDelete, http.MethodOptions} {
		t.Run("stateless/"+method, func(t *testing.T) {
			gate := newGate(t, okFactory)
			gate.stateless = true
			if !passesThrough(t, gate, method) {
				t.Errorf("%s was gated; it must pass through so the SDK can answer 405", method)
			}
		})
	}

	t.Run("stateful/OPTIONS", func(t *testing.T) {
		gate := newGate(t, okFactory)
		if !passesThrough(t, gate, http.MethodOptions) {
			t.Error("a CORS preflight was gated; a browser sends it without credentials")
		}
	})

	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		t.Run("stateful/"+method, func(t *testing.T) {
			gate := newGate(t, okFactory)
			if passesThrough(t, gate, method) {
				t.Errorf("%s reached the session layer unauthenticated; a session ID would be enough to read or end someone else's session", method)
			}
		})
	}
}

// TestMCPServerGate_ValidToken_AttachesServerAndResolvesPoolOnce verifies the
// happy path and the reason the server travels in the context: the pool is
// consulted by the gate, not again by the SDK callback.
func TestMCPServerGate_ValidToken_AttachesServerAndResolvesPoolOnce(t *testing.T) {
	var factoryCalls atomic.Int32
	countingFactory := func(c *gitlabclient.Client, cfg *config.ServerConfig) (*mcp.Server, error) {
		factoryCalls.Add(1)
		return okFactory(c, cfg)
	}
	gate := newGate(t, countingFactory)

	var seen *mcp.Server
	handler := gate.middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = serverFromRequestContext(r)
	}))

	// No t.Parallel in these subtests: they share `seen` and the factory
	// counter asserted after the loop.
	for _, tc := range []struct{ header, value string }{
		{header: "PRIVATE-TOKEN", value: gateTestToken},
		{header: "Authorization", value: "Bearer " + gateTestToken},
	} {
		t.Run(tc.header, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader("{}"))
			req.Header.Set(tc.header, tc.value)
			rec := httptest.NewRecorder()

			seen = nil
			handler.ServeHTTP(rec, req)

			if seen == nil {
				t.Error("handler received no server from the request context")
			}
			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
			}
		})
	}

	// Both headers carry the same token, so the pool must build exactly one entry.
	if got := factoryCalls.Load(); got != 1 {
		t.Errorf("factory called %d times, want 1 (both requests share a pool key)", got)
	}
}

// TestServerFromRequestContext_WithoutGate_ReturnsNil documents the fallback:
// without the gate the callback has nothing to return, which is the very
// "no server available" path the gate exists to prevent.
func TestServerFromRequestContext_WithoutGate_ReturnsNil(t *testing.T) {
	if got := serverFromRequestContext(httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil)); got != nil {
		t.Errorf("server = %v, want nil when the gate did not run", got)
	}
}

// TestGateErrorCodes_AllocatedOutsideReservedRange enforces the MCP error-code
// policy: -32000..-32019 is legacy, -32020..-32099 belongs to the specification,
// and application codes must sit outside the whole -32768..-32000 reserved
// range. Only the standard JSON-RPC codes may fall inside it.
func TestGateErrorCodes_AllocatedOutsideReservedRange(t *testing.T) {
	standard := map[int]bool{-32700: true, -32600: true, -32601: true, -32602: true, -32603: true}
	for name, code := range map[string]int{
		"errCodeInvalidRequest":      errCodeInvalidRequest,
		"errCodeUnauthorized":        errCodeUnauthorized,
		"errCodeTooManyRequests":     errCodeTooManyRequests,
		"errCodeUpstreamUnavailable": errCodeUpstreamUnavailable,
	} {
		t.Run(name, func(t *testing.T) {
			if standard[code] {
				return
			}
			if code >= -32768 && code <= -32000 {
				t.Errorf("%s = %d is inside the JSON-RPC reserved range; application codes must be outside it", name, code)
			}
		})
	}
}

// TestRegisterLegacyMCPHandlers_UnauthenticatedPOST_NeverReturnsNoServerAvailable
// is the end-to-end guard for the reported defect. It exercises the real mux
// wiring rather than the gate in isolation, because the bug was never in a
// helper: it was that the SDK's getServer callback could return nil, and the
// SDK answers every nil with "400 no server available" in text/plain.
//
// That string is emitted inside go-sdk (mcp/streamable.go), so it cannot be
// caught by grepping this repository — only by asserting on a real response.
func TestRegisterLegacyMCPHandlers_UnauthenticatedPOST_NeverReturnsNoServerAvailable(t *testing.T) {
	cfg := &config.Config{
		GitLabURL:    "https://gitlab.example.com",
		Tier:         edition.Free,
		TierExplicit: true,
		IgnoreScopes: true,
		Stateless:    true,
	}
	mux := http.NewServeMux()
	registerLegacyMCPHandlers(t.Context(), cfg, newGateTestPool(t, okFactory, cfg.GitLabURL), &sync.Map{}, mux)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL,
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// io.ReadAll, not a single Read: a short read would truncate the JSON and
	// fail the decode below for reasons unrelated to the gate.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading response body: %v", err)
	}
	got := string(body)

	if strings.Contains(got, "no server available") {
		t.Errorf("response still carries the SDK's opaque rejection: %q", got)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
	if resp.Header.Get("WWW-Authenticate") == "" {
		t.Error("401 without WWW-Authenticate violates RFC 9110")
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json so clients can parse the reason", ct)
	}
	decodeJSONRPCError(t, got, "1")
}

// TestMcpServerGate_WithIdentity_AttachesThePooledUser verifies that HTTP mode
// now resolves an identity for tool handlers.
//
// toolutil.ResolveIdentity reads req.Extra.TokenInfo first and falls back to
// the context. Only the SDK's bearer middleware can fill that field, and
// legacy mode does not mount it, so every HTTP legacy request used to resolve
// the zero identity and log lines carried no user at all.
func TestMcpServerGate_WithIdentity_AttachesThePooledUser(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"17.0.0"}`))
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":77,"username":"legacy-user"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := &config.Config{GitLabURL: srv.URL, IgnoreScopes: true, TierExplicit: true}
	pool := serverpool.New(cfg, func(*gitlabclient.Client, *config.ServerConfig) (*mcp.Server, error) {
		return mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil), nil
	})
	gate := &mcpServerGate{pool: pool, gitlabURLs: []string{srv.URL}, challenge: legacyAuthChallenge}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", http.NoBody)
	req.Header.Set("PRIVATE-TOKEN", "glpat-legacy")

	if _, failure := gate.resolve(req); failure != nil {
		t.Fatalf("resolve: %+v", failure)
	}

	identity := toolutil.IdentityFromContext(gate.withIdentity(t.Context(), req))
	if !identity.IsAuthenticated() {
		t.Fatal("legacy mode should now resolve an identity")
	}
	if identity.UserID != "77" || identity.Username != "legacy-user" {
		t.Errorf("identity = %+v, want {77 legacy-user}", identity)
	}
}

// TestMcpServerGate_WithIdentity_UnknownTokenLeavesContextAlone verifies that
// an unresolved identity is left absent rather than stored empty, so a handler
// can tell "the lookup did not succeed" from "a user with no name".
func TestMcpServerGate_WithIdentity_UnknownTokenLeavesContextAlone(t *testing.T) {
	cfg := &config.Config{GitLabURL: "https://gitlab.example.com", IgnoreScopes: true, TierExplicit: true}
	pool := serverpool.New(cfg, func(*gitlabclient.Client, *config.ServerConfig) (*mcp.Server, error) {
		return mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil), nil
	})
	gate := &mcpServerGate{pool: pool, gitlabURLs: []string{cfg.GitLabURL}, challenge: legacyAuthChallenge}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", http.NoBody)
	req.Header.Set("PRIVATE-TOKEN", "glpat-never-pooled")

	// No GetOrCreate ran for this token, so the pool holds nothing.
	if identity := toolutil.IdentityFromContext(gate.withIdentity(t.Context(), req)); identity.IsAuthenticated() {
		t.Errorf("identity = %+v, want the zero value for a token the pool never built", identity)
	}
}

// TestGate_AllowListIsOnlyNamedWhereItIsAlreadyPublic pins who gets to see the
// set of instances a deployment publishes.
//
// Naming them helps a client that guessed wrong, and it costs nothing in oauth
// mode: the same list is served unauthenticated as RFC 9728
// `authorization_servers`, and the bearer guard has already verified the caller
// before the gate runs. Legacy mode has neither property — no metadata document
// publishes the list, and this rejection is reached before the credential is
// validated — so any non-empty token would have enumerated the operator's
// instance hostnames, internal ones included.
func TestGate_AllowListIsOnlyNamedWhereItIsAlreadyPublic(t *testing.T) {
	t.Parallel()

	const published = "https://gitlab.internal.example"

	newRequest := func(t *testing.T) *http.Request {
		t.Helper()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", strings.NewReader("{}"))
		req.Header.Set("PRIVATE-TOKEN", "glpat-anything")
		req.Header.Set(serverpool.RequestOptionGitLabURL, "https://gitlab.attacker.example")
		return req
	}

	// Two published instances, because with exactly one the header is ignored
	// by design and never reaches the rejection under test.
	withAllowList := func(t *testing.T) *mcpServerGate {
		t.Helper()
		gate := newGateAgainst(t, okFactory, published)
		gate.gitlabURLs = []string{published, "https://gitlab.other.example"}
		return gate
	}

	t.Run("legacy mode redacts it", func(t *testing.T) {
		t.Parallel()
		gate := withAllowList(t)
		rec := httptest.NewRecorder()
		gate.middleware(http.NotFoundHandler()).ServeHTTP(rec, newRequest(t))

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
		if strings.Contains(rec.Body.String(), "gitlab.internal.example") {
			t.Errorf("the published instance leaked to an unverified caller: %s", rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "does not serve") {
			t.Errorf("the caller was not told what went wrong: %s", rec.Body.String())
		}
	})

	t.Run("oauth mode names it", func(t *testing.T) {
		t.Parallel()
		gate := withAllowList(t)
		gate.oauthMode = true
		rec := httptest.NewRecorder()
		gate.middleware(http.NotFoundHandler()).ServeHTTP(rec, newRequest(t))

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
		if !strings.Contains(rec.Body.String(), "gitlab.internal.example") {
			t.Errorf("oauth mode must keep naming the list it already publishes: %s", rec.Body.String())
		}
	})
}
