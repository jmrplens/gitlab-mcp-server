package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/mcpotel"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/oauth"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/serverpool"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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

// gateStubGitLabByName respells a stub instance's address as a host name.
//
// The gate judges an address literal and leaves a name entirely to the dialer,
// so a name is what a row about that rule has to carry — and a name resolving
// to the very loopback address the literal rows are refused for states the
// rule more sharply than a public one would, since a gate that did resolve
// names would refuse this and admit that. The instance behind it is still the
// local stub, so the pool entry an admitted row goes on to build costs a round
// trip on loopback rather than a resolver timeout.
func gateStubGitLabByName(t *testing.T, stubURL string) string {
	t.Helper()
	_, port, err := net.SplitHostPort(strings.TrimPrefix(stubURL, "http://"))
	if err != nil {
		t.Fatalf("stub URL %q is not host:port: %v", stubURL, err)
	}
	return "http://localhost:" + port
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

// gateTokenGatedGitLab is a stub instance that accepts exactly one credential
// and answers 401 to every other, counting the credential probes it is asked
// for. The count is what makes "the block cost nothing upstream" observable:
// every probe is a request this deployment would be making to GitLab from its
// own address.
func gateTokenGatedGitLab(t *testing.T, accepted string, probes *atomic.Int64) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/user", func(w http.ResponseWriter, r *http.Request) {
		probes.Add(1)
		if r.Header.Get("PRIVATE-TOKEN") != accepted {
			http.Error(w, `{"message":"401 Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":42,"username":"neighbor"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

// TestMCPServerGate_BlockedAddress_StillServesACredentialThePoolHolds pins the
// two halves of what an address block means, which used to be one half.
//
// The budget is keyed on the address, and an address is not a client: a NAT, a
// campus, a carrier and a proxy without --trusted-proxy-header all present one
// for many people. Because the block was consulted before the credential was
// read, one client relaying invented tokens answered 429 to every neighbor for
// the rest of the window, a neighbor holding a token this server had already
// verified and was serving from a pool entry included.
//
// So the block must still refuse every credential the pool does not hold —
// which is the whole of what a sprayer can send, and the property the budget
// was built for — while a credential it does hold is served. Neither answer may
// reach GitLab, and the probe count is what says so: the exemption is a map
// read, not a second chance to spend the deployment's standing upstream.
func TestMCPServerGate_BlockedAddress_StillServesACredentialThePoolHolds(t *testing.T) {
	var probes atomic.Int64
	gate := newGateAgainst(t, okFactory, gateTokenGatedGitLab(t, gateTestToken, &probes))

	var reached atomic.Int64
	handler := gate.middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	post := func(token string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader("{}"))
		req.Header.Set("PRIVATE-TOKEN", token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	// The neighbor authenticates before the spray begins. That is the whole
	// of its claim: its credential was verified and is in the pool.
	if rec := post(gateTestToken); rec.Code != http.StatusOK {
		t.Fatalf("the neighbor's first request = %d, want %d", rec.Code, http.StatusOK)
	}

	// The sprayer spends the shared address's budget.
	for i := range authFailureLimit {
		if rec := post("glpat-invented-" + strconv.Itoa(i)); rec.Code != http.StatusUnauthorized {
			t.Fatalf("spray attempt %d = %d, want %d", i, rec.Code, http.StatusUnauthorized)
		}
	}

	spent := probes.Load()

	// An invented token from the blocked address is refused, and refused
	// without asking GitLab about it.
	blocked := post("glpat-invented-past-the-budget")
	if blocked.Code != http.StatusTooManyRequests {
		t.Fatalf("an unknown credential from a blocked address = %d, want %d — the budget no longer bounds the spray",
			blocked.Code, http.StatusTooManyRequests)
	}
	if got := probes.Load(); got != spent {
		t.Errorf("the blocked request cost %d upstream probe(s); a block must cost nothing", got-spent)
	}

	// The neighbor, on the same address, is still served.
	served := post(gateTestToken)
	if served.Code == http.StatusTooManyRequests {
		t.Fatalf("a credential the pool already holds was refused 429 for its neighbor's spending")
	}
	if served.Code != http.StatusOK {
		t.Fatalf("the neighbor's request = %d, want %d", served.Code, http.StatusOK)
	}
	if reached.Load() != 2 {
		t.Errorf("the handler was reached %d time(s), want 2 — once before the block and once during it", reached.Load())
	}
	if got := probes.Load(); got != spent {
		t.Errorf("serving the admitted credential cost %d upstream probe(s); it is a pool hit, not a verification", got-spent)
	}
}

// TestMCPServerGate_BlockedAddress_WithNoPool_StaysBlocked covers the gate that
// holds no pool, which is the one shape in which the exemption has nothing to
// consult.
//
// The block is the path that must cost nothing, so it is also the path that may
// not reach for state that is not there: asking a nil pool whether it holds
// this credential takes the process down, and it does so only once an address
// is already blocked, which is exactly when a deployment is under a spray. The
// gate answers 429 instead, because a deployment with no pool holds no
// credential, so there is nothing for the exemption to be true of.
//
// The budget is spent with requests carrying no credential at all, since those
// are refused before anything touches the pool; a request with a token would
// reach [serverpool.ServerPool.GetOrCreateEntry] and tell us nothing about the
// exemption.
func TestMCPServerGate_BlockedAddress_WithNoPool_StaysBlocked(t *testing.T) {
	gate := &mcpServerGate{
		gitlabURLs: []string{"https://gitlab.example.com"},
		limiter:    serverpool.NewAuthRateLimiter(authFailureLimit, authFailureWindow),
		challenge:  legacyAuthChallenge,
	}

	var reached atomic.Int64
	handler := gate.middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	post := func(token string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader("{}"))
		if token != "" {
			req.Header.Set("PRIVATE-TOKEN", token)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	for i := range authFailureLimit {
		if rec := post(""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("spending attempt %d = %d, want %d", i, rec.Code, http.StatusUnauthorized)
		}
	}

	// A credential arrives at a blocked address. There is no pool, so it
	// cannot be one this deployment already serves, and the gate must say so
	// rather than go looking.
	blocked := post(gateTestToken)
	if blocked.Code != http.StatusTooManyRequests {
		t.Fatalf("a credential at a blocked address with no pool = %d, want %d",
			blocked.Code, http.StatusTooManyRequests)
	}
	if reached.Load() != 0 {
		t.Errorf("the handler was reached %d time(s) from a blocked address", reached.Load())
	}
}

// TestMCPServerGate_BlockedAddress_ExemptionIsScopedToTheInstanceSelected pins
// what the exemption is keyed on, which is a credential and not a token.
//
// A pool entry belongs to one token at one instance, because a token means
// nothing away from the GitLab that issued it. So the exemption has to resolve
// the instance the request selected and ask about that pair, and the three ways
// a request can fail to name one it holds all have to stay blocked: a published
// instance the pool holds no entry for, an instance this deployment does not
// publish, and no selection at all where several are published.
//
// Only the first of those is a lookup. The other two are the resolver refusing
// to name an instance for the request, which leaves the exemption nothing to
// ask about and so leaves the block standing; the request is then answered the
// way any blocked request is, rather than being let through to be told which
// instances exist.
//
// The probe count is the second half of every case: none of these answers may
// spend the deployment's standing with GitLab, which is what the budget is
// there to protect.
func TestMCPServerGate_BlockedAddress_ExemptionIsScopedToTheInstanceSelected(t *testing.T) {
	var probes atomic.Int64
	published := gateTokenGatedGitLab(t, gateTestToken, &probes)
	alsoPublished := gateTokenGatedGitLab(t, gateTestToken, &probes)

	gate := newGateAgainst(t, okFactory, published)
	gate.gitlabURLs = []string{published, alsoPublished}

	var reached atomic.Int64
	handler := gate.middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	post := func(token, instance string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader("{}"))
		req.Header.Set("PRIVATE-TOKEN", token)
		if instance != "" {
			req.Header.Set(serverpool.RequestOptionGitLabURL, instance)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	// The credential earns its entry, at one of the two published instances.
	if rec := post(gateTestToken, published); rec.Code != http.StatusOK {
		t.Fatalf("warming the pool entry = %d, want %d", rec.Code, http.StatusOK)
	}

	for i := range authFailureLimit {
		if rec := post("glpat-invented-"+strconv.Itoa(i), published); rec.Code != http.StatusUnauthorized {
			t.Fatalf("spray attempt %d = %d, want %d", i, rec.Code, http.StatusUnauthorized)
		}
	}
	spent := probes.Load()

	cases := []struct {
		name     string
		instance string
		want     int
		why      string
	}{
		{
			name:     "the instance the entry belongs to",
			instance: published,
			want:     http.StatusOK,
			why:      "a credential the pool already holds must be served whoever shares its address",
		},
		{
			name:     "another published instance",
			instance: alsoPublished,
			want:     http.StatusTooManyRequests,
			why:      "the pool holds no entry for this token there, so serving it would mean verifying it upstream",
		},
		{
			name:     "an instance this deployment does not publish",
			instance: "https://gitlab.invented.example",
			want:     http.StatusTooManyRequests,
			why:      "the resolver names no instance for this request, so the exemption has nothing to ask about",
		},
		{
			name:     "no instance selected where two are published",
			instance: "",
			want:     http.StatusTooManyRequests,
			why:      "the resolver refuses to choose, so the exemption has nothing to ask about",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := probes.Load()
			rec := post(gateTestToken, tc.instance)
			if rec.Code != tc.want {
				t.Errorf("= %d, want %d: %s", rec.Code, tc.want, tc.why)
			}
			if got := probes.Load(); got != before {
				t.Errorf("cost %d upstream probe(s); no answer from a blocked address may reach GitLab", got-before)
			}
		})
	}

	if got := probes.Load(); got != spent {
		t.Errorf("the blocked address spent %d upstream probe(s) in total; it must spend none", got-spent)
	}
	if reached.Load() != 2 {
		t.Errorf("the handler was reached %d time(s), want 2: the warm-up and the one exempt request", reached.Load())
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
// "no server available" path the gate exists to prevent. It is logged as a
// wiring fault, since the SDK's own answer to the nil names no cause.
func TestServerFromRequestContext_WithoutGate_ReturnsNil(t *testing.T) {
	logged := testutil.CaptureSlog(t)
	if got := serverFromRequestContext(httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil)); got != nil {
		t.Errorf("server = %v, want nil when the gate did not run", got)
	}
	if !strings.Contains(logged.String(), "reached without a gated server") {
		t.Errorf("log = %q, want the missing gate reported", logged.String())
	}
}

// TestServerFromRequestContext_ReturnsTheGatedServerQuietly is the path every
// gated request takes: the server the gate resolved comes back, and nothing is
// logged, because a wiring fault reported on every healthy request would bury
// the one that is real.
func TestServerFromRequestContext_ReturnsTheGatedServerQuietly(t *testing.T) {
	logged := testutil.CaptureSlog(t)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	ctx := context.WithValue(t.Context(), resolvedServerContextKey{}, server)

	if got := serverFromRequestContext(httptest.NewRequestWithContext(ctx, http.MethodPost, "/", nil)); got != server {
		t.Errorf("server = %p, want the gated %p", got, server)
	}
	if strings.Contains(logged.String(), "reached without a gated server") {
		t.Errorf("a gated request was reported as a wiring fault: %s", logged.String())
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
		"errCodeForbidden":           errCodeForbidden,
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

// TestRefusalCodes_MirrorTheirStatusAndSitWhereTheSpecificationAllows pins the
// values of the codes themselves, which nothing else in this package does.
//
// Every test that reads a refusal off the wire compares the code it found
// against the same constant the handler wrote it from, so the two move
// together and a constant that changed value fails nothing.
// TestGateErrorCodes_AllocatedOutsideReservedRange is the closest thing to a
// check on the values, and it asks only whether a code falls inside
// -32768..-32000 — which a positive number does not, so a code whose sign was
// lost passes it.
//
// What these codes have to be is stated above them: one this server allocates
// mirrors the HTTP status it travels with, multiplied by -100, which is what
// puts it below the whole reserved range and lets a reader map a code back to
// a status without a table. Deriving the expected value from the status here
// rather than repeating the literal is the point: it asserts the convention,
// so a code that stopped following it is caught even if somebody updated the
// literal to match.
func TestRefusalCodes_MirrorTheirStatusAndSitWhereTheSpecificationAllows(t *testing.T) {
	t.Parallel()

	mirrored := map[string]struct {
		code   int
		status int
	}{
		"errCodeUnauthorized":        {errCodeUnauthorized, http.StatusUnauthorized},
		"errCodeForbidden":           {errCodeForbidden, http.StatusForbidden},
		"errCodeTooManyRequests":     {errCodeTooManyRequests, http.StatusTooManyRequests},
		"errCodeUpstreamUnavailable": {errCodeUpstreamUnavailable, http.StatusServiceUnavailable},
	}
	for name, mapping := range mirrored {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if want := -100 * mapping.status; mapping.code != want {
				t.Errorf("%s = %d, want %d: a code this server allocates mirrors its HTTP status times -100",
					name, mapping.code, want)
			}
			// Below the range rather than merely outside it. A positive code
			// is outside too, and is what a lost sign produces.
			if mapping.code > -32768 {
				t.Errorf("%s = %d is not below the reserved range -32768..-32000; "+
					"a code allocated by this server has to sit under the whole of it", name, mapping.code)
			}
		})
	}

	// The one code here the specification defines rather than this server, and
	// therefore the one that belongs inside the range the specification kept
	// for itself. Asserting the other direction for it is what keeps the two
	// conventions from being confused for each other.
	t.Run("codeUnsupportedProtocolVersion", func(t *testing.T) {
		t.Parallel()

		if codeUnsupportedProtocolVersion < -32099 || codeUnsupportedProtocolVersion > -32020 {
			t.Errorf("codeUnsupportedProtocolVersion = %d, want a code in -32099..-32020, "+
				"the band the JSON-RPC specification reserved for itself and the MCP specification allocated this one from",
				codeUnsupportedProtocolVersion)
		}
	})

	// errCodeInvalidRequest is not this server's to choose at all: it is the
	// standard JSON-RPC code for a request that is not well formed, and a
	// client matching on it is matching on the standard.
	t.Run("errCodeInvalidRequest", func(t *testing.T) {
		t.Parallel()

		if errCodeInvalidRequest != -32600 {
			t.Errorf("errCodeInvalidRequest = %d, want the standard JSON-RPC \"Invalid Request\" code -32600",
				errCodeInvalidRequest)
		}
	})
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
	registerLegacyMCPHandlers(t.Context(), cfg, newGateTestPool(t, okFactory, cfg.GitLabURL),
		poolBinding{credentials: &credentialStates{}, sessions: newSessionOwners(false)}, mux)

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

// TestMcpServerGate_Resolve_RefusesACallerNamedPrivateInstance covers the
// early refusal the gate makes when the header names an address this server
// would decline to dial anyway.
//
// The dialer is the authority and would refuse the same destination, but only
// after the credential has been admitted, a pool entry built and every action
// attempted. Refusing here costs one comparison and gives an operator a 400
// that names the flag instead of a server that starts and then fails
// everything (ADR-0022).
//
// The published-instance row is the other half of the rule and the one that
// keeps every ordinary deployment working: with an instance published, the
// header selects among the operator's own, so nothing here judges it. The
// gate never resolves a name either, which the hostname row pins: only the
// dialer sees what a name resolved to.
//
// Which address classes count as private is settled by the predicate's own
// table in internal/gitlab and is not restated here. A row naming a publicly
// routable literal would be admitted, and every admitted row goes on to build
// a pool entry against the instance it names: both therefore name the local
// stub, the hostname row through a name that resolves to it. Naming a host
// nothing answers on costs the two five-second probes that build the entry —
// ten seconds for an answer this test never reads.
func TestMcpServerGate_Resolve_RefusesACallerNamedPrivateInstance(t *testing.T) {
	stub := gateStubGitLab(t, false)

	tests := []struct {
		name        string
		published   []string
		header      string
		wantRefused bool
	}{
		{name: "an unpinned deployment refuses a loopback instance", header: "http://127.0.0.1:8080", wantRefused: true},
		{name: "an unpinned deployment refuses a metadata address", header: "http://169.254.169.254", wantRefused: true},
		{name: "an unpinned deployment leaves a host name to the dialer", header: gateStubGitLabByName(t, stub), wantRefused: false},
		{
			name:      "a published instance is the operator's own",
			published: []string{stub}, header: stub, wantRefused: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Retries off so that a row naming something unanswered is one
			// failed round trip rather than retryablehttp's linear backoff,
			// on a probe whose answer this test does not read either way.
			cfg := &config.Config{GitLabURL: "https://gitlab.example.com", IgnoreScopes: true, TierExplicit: true, DisableRetries: true}
			pool := serverpool.New(cfg, func(*gitlabclient.Client, *config.ServerConfig) (*mcp.Server, error) {
				return mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil), nil
			})
			t.Cleanup(pool.Close)
			gate := &mcpServerGate{pool: pool, gitlabURLs: tt.published, challenge: legacyAuthChallenge}

			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", http.NoBody)
			req.Header.Set("PRIVATE-TOKEN", "glpat-caller")
			req.Header.Set("GITLAB-URL", tt.header)

			_, failure := gate.resolve(req)

			refused := failure != nil && failure.status == http.StatusBadRequest &&
				strings.Contains(failure.message, "destination refused")
			if refused != tt.wantRefused {
				t.Fatalf("refused = %v, want %v (failure = %+v)", refused, tt.wantRefused, failure)
			}
		})
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

// gateTestEntry resolves one pooled credential, which is what the ownership
// check compares a session against now that a server is shared by every
// credential of a configuration shape.
func gateTestEntry(t *testing.T, pool *serverpool.ServerPool, token, gitlabURL string) *serverpool.Entry {
	t.Helper()
	entry, err := pool.GetOrCreateEntry(token, gitlabURL, nil)
	if err != nil {
		t.Fatalf("GetOrCreateEntry(%q): %v", token, err)
	}
	return entry
}

// TestMcpServerGate_CheckSessionOwnership_RefusesASessionFromAnotherCredential
// covers the check that makes a session ID insufficient on its own.
//
// A session belongs to the pooled credential whose first request opened it,
// which [sessionOwners] recorded at that moment; the same ID presented with a
// different credential is therefore somebody else's session. Waving it through
// would make the ID alone enough to read another user's server-initiated stream
// or to end their session. An ID this deployment never recorded is refused for
// the same reason: stateless mode issues none at all, so anything unrecorded is
// stale or forged, and so is anything belonging to an evicted credential. The
// refusal is a 404 rather than a 403, which is what the SDK answers for a
// session it does not know and therefore says nothing about which IDs exist.
func TestMcpServerGate_CheckSessionOwnership_RefusesASessionFromAnotherCredential(t *testing.T) {
	gitlab := gateStubGitLab(t, false)
	pool := newGateTestPool(t, okFactory, gitlab)
	mine := gateTestEntry(t, pool, gateTestToken, gitlab)
	theirs := gateTestEntry(t, pool, gateTestToken+"-other", gitlab)

	minted := newIdentifiedSessions(t)
	sessions := newSessionOwners(false)
	ownID := minted.recordUnder(t, sessions, mine.Owner())
	otherID := minted.recordUnder(t, sessions, theirs.Owner())

	// A credential the pool has since evicted: its sessions were forgotten with
	// it, so the ID is as unknown as one that was never opened.
	evicted := newSessionOwners(false)
	evictedID := minted.recordUnder(t, evicted, mine.Owner())
	evicted.forgetOwner(mine.Owner())

	tests := []struct {
		name        string
		owners      *sessionOwners
		sessionID   string
		wantRefusal bool
	}{
		{name: "no session id is nothing to own", owners: sessions},
		{name: "no ownership table at all is no check", owners: nil, sessionID: otherID},
		{name: "the credential's own session", owners: sessions, sessionID: ownID},
		{name: "a session opened by another credential", owners: sessions, sessionID: otherID, wantRefusal: true},
		{name: "an id this deployment never recorded", owners: sessions, sessionID: "never-opened-here", wantRefusal: true},
		{name: "a session whose credential has been evicted", owners: evicted, sessionID: evictedID, wantRefusal: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gate := &mcpServerGate{sessions: tt.owners, challenge: legacyAuthChallenge}
			r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", http.NoBody)
			if tt.sessionID != "" {
				r.Header.Set(mcpSessionIDHeader, tt.sessionID)
			}

			failure := gate.checkSessionOwnership(r, mine)

			if (failure != nil) != tt.wantRefusal {
				t.Fatalf("failure = %+v, want refused=%v", failure, tt.wantRefusal)
			}
			if failure != nil && failure.status != http.StatusNotFound {
				t.Errorf("status = %d, want %d so the refusal says nothing about which sessions exist", failure.status, http.StatusNotFound)
			}
		})
	}
}

// TestMcpServerGate_WithIdentity_WithoutAPoolOrAUsableURL_LeavesTheContext
// covers the two ways identity resolution declines to say anything.
//
// Both leave the context untouched rather than storing an empty identity,
// because a handler has to be able to tell "the lookup did not succeed" from
// "a user with no name" — the second would be logged as an authenticated call
// by nobody.
func TestMcpServerGate_WithIdentity_WithoutAPoolOrAUsableURL_LeavesTheContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		gate   *mcpServerGate
		header string
	}{
		{name: "no pool at all", gate: &mcpServerGate{}},
		{
			name:   "an instance this deployment does not publish",
			gate:   &mcpServerGate{pool: serverpool.New(&config.Config{GitLabURL: "https://gitlab.example.com", IgnoreScopes: true, TierExplicit: true}, okFactory), gitlabURLs: []string{"https://a.example.com", "https://b.example.com"}},
			header: "https://elsewhere.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", http.NoBody)
			r.Header.Set("PRIVATE-TOKEN", "glpat-something")
			if tt.header != "" {
				r.Header.Set(serverpool.RequestOptionGitLabURL, tt.header)
			}

			ctx := tt.gate.withIdentity(t.Context(), r)

			if toolutil.IdentityFromContext(ctx).IsAuthenticated() {
				t.Error("an identity was attached from a lookup that never succeeded")
			}
		})
	}
}

// TestMcpServerGate_InvalidURLMessage_SaysOnlyWhatTheCallerAlreadyKnows covers
// what a rejected GITLAB-URL is told.
//
// A client that sent no header is looking at the operator's own misconfigured
// --gitlab-url, and echoing the parse detail there would reflect server-side
// configuration back to anyone who asks. Naming the published instances is safe
// exactly in oauth mode, where the same list is already served unauthenticated
// as RFC 9728 authorization_servers; legacy mode publishes no such document and
// this rejection is reached before the credential is checked, so echoing the
// list would let any non-empty token enumerate the operator's hostnames.
func TestMcpServerGate_InvalidURLMessage_SaysOnlyWhatTheCallerAlreadyKnows(t *testing.T) {
	t.Parallel()

	disallowed := &serverpool.DisallowedGitLabURLError{Allowed: []string{"https://gitlab.com", "https://gitlab.example.com"}}

	tests := []struct {
		name      string
		gate      *mcpServerGate
		header    string
		err       error
		wantSays  string
		wantHides string
	}{
		{
			name:     "no header names the operator, not the caller",
			gate:     &mcpServerGate{},
			err:      disallowed,
			wantSays: "contact the operator",
		},
		{
			name:      "legacy mode does not list the instances",
			gate:      &mcpServerGate{},
			header:    "https://elsewhere.example.com",
			err:       disallowed,
			wantSays:  "does not serve",
			wantHides: "gitlab.example.com",
		},
		{
			name:     "oauth mode may name what it already publishes",
			gate:     &mcpServerGate{oauthMode: true},
			header:   "https://elsewhere.example.com",
			err:      disallowed,
			wantSays: "gitlab.example.com",
		},
		{
			name:     "another parse failure is returned as a sentence",
			gate:     &mcpServerGate{},
			header:   "://nonsense",
			err:      &serverpool.InvalidGitLabURLError{Reason: "malformed URL"},
			wantSays: "GITLAB-URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", http.NoBody)
			if tt.header != "" {
				r.Header.Set(serverpool.RequestOptionGitLabURL, tt.header)
			}

			message := tt.gate.invalidURLMessage(r, tt.err)

			if !strings.Contains(message, tt.wantSays) {
				t.Errorf("message = %q, want it to say %q", message, tt.wantSays)
			}
			if tt.wantHides != "" && strings.Contains(message, tt.wantHides) {
				t.Errorf("message = %q, want it to keep %q to itself", message, tt.wantHides)
			}
			if message != "" && message[0] >= 'a' && message[0] <= 'z' {
				t.Errorf("message = %q, want it to read as a sentence", message)
			}
		})
	}
}

// TestCapitalizeFirst_AnEmptyStringStaysEmpty covers the guard in front of the
// slice that would otherwise panic.
//
// The input is an error string, and an error whose message is empty is exactly
// the kind of thing that only turns up in production.
func TestCapitalizeFirst_AnEmptyStringStaysEmpty(t *testing.T) {
	t.Parallel()

	if got := capitalizeFirst(""); got != "" {
		t.Errorf("capitalizeFirst(\"\") = %q, want the empty string", got)
	}
	if got := capitalizeFirst("gitlab refused the token"); got != "Gitlab refused the token" {
		t.Errorf("capitalizeFirst = %q, want the first letter upper-cased", got)
	}
}

// TestMcpServerGate_InvalidTokenChallenge_NamesTheVerdictInOAuthMode covers the
// difference between the two modes' challenges for a credential GitLab refused.
//
// The bearer guard emits error="invalid_token" for the identical judgement, and
// the gate reaches this only for a pool rejection the guard could not see:
// answering the same verdict two different ways leaves a client unable to tell
// "reauthorize" from "you sent nothing". Legacy mode carries no such parameters
// because it publishes no metadata document for a client to act on.
func TestMcpServerGate_InvalidTokenChallenge_NamesTheVerdictInOAuthMode(t *testing.T) {
	t.Parallel()

	legacy := (&mcpServerGate{challenge: legacyAuthChallenge}).invalidTokenChallenge()
	if legacy != legacyAuthChallenge {
		t.Errorf("legacy challenge = %q, want it unchanged", legacy)
	}

	oauthed := (&mcpServerGate{challenge: `Bearer resource_metadata="https://mcp.example.com/.well-known/oauth-protected-resource"`, oauthMode: true}).invalidTokenChallenge()
	for _, want := range []string{`error="invalid_token"`, "error_description="} {
		t.Run(want, func(t *testing.T) {
			t.Parallel()

			if !strings.Contains(oauthed, want) {
				t.Errorf("oauth challenge %q is missing %s", oauthed, want)
			}
		})
	}
}

// TestMCPServerGate_SpoofedProxyHeaderRotation_StaysBounded verifies that a
// caller who supplies the trusted proxy header themselves cannot mint a fresh
// failure budget per request.
//
// When --trusted-proxy-header is set the limiter key comes from that header,
// which is correct for the intended topology and wrong the moment the server
// is also reachable directly: an attacker rotates the value, every request
// gets a distinct "client IP", the ten-failures-a-minute lockout never fires,
// and each invalid-token request is relayed one to one to GitLab as a /user
// verification. The secondary budget is charged to the transport source, which
// no header can change.
//
// It is deliberately far coarser than the per-caller one. Charging both to the
// same ten would break the correctly configured topology outright: behind a
// genuine proxy the transport source is the proxy for every client, so ten
// aggregate failures a minute would lock out the whole fleet.
func TestMCPServerGate_SpoofedProxyHeaderRotation_StaysBounded(t *testing.T) {
	for _, tc := range []struct {
		name        string
		header      string
		rotate      bool
		wantBlocked bool
	}{
		{"rotating_a_spoofed_header", "X-Forwarded-For", true, true},
		{"repeating_one_header_value", "X-Forwarded-For", false, true},
		{"no_trusted_header_configured", "", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gate := newGate(t, okFactory)
			gate.trustedProxyHeader = tc.header
			if tc.header != "" {
				gate.trustedProxies = trustedProxiesOf([]string{"203.0.113.7"})
				gate.sourceBudget = newTransportBudget(serverpool.NewAuthRateLimiter(transportFailureLimit, authFailureWindow), authFailureWindow)
			}
			handler := gate.middleware(http.NotFoundHandler())

			// One more than the coarse budget, which is the only one a
			// rotating caller can exhaust.
			blocked := false
			for i := range transportFailureLimit + 1 {
				req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader("{}"))
				req.RemoteAddr = "203.0.113.7:44444"
				if tc.header != "" {
					value := "198.51.100.1"
					if tc.rotate {
						// Two octets vary: the key is the address alone, a
						// port on the hop is stripped, and one octet gives
						// fewer distinct keys than the coarse budget holds.
						value = "198.51." + strconv.Itoa(i/250) + "." + strconv.Itoa(i%250+1)
					}
					req.Header.Set(tc.header, value)
				}
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
				if rec.Code == http.StatusTooManyRequests {
					blocked = true
					break
				}
			}
			if blocked != tc.wantBlocked {
				t.Errorf("blocked = %v after %d credential-less requests, want %v",
					blocked, transportFailureLimit+1, tc.wantBlocked)
			}
		})
	}
}

// TestMCPServerGate_TrustedProxyKeepsPerClientGranularity verifies that the
// coarse transport budget does not collapse a genuine proxy's clients into one
// bucket: one client behind the proxy exhausting its own ten failures must not
// refuse the next client through the same proxy.
//
// This is the pair to the test above, and it is the real acceptance criterion:
// a fix that bounded rotation by keying on the transport source alone would
// pass that one and fail this.
func TestMCPServerGate_TrustedProxyKeepsPerClientGranularity(t *testing.T) {
	gate := newGate(t, okFactory)
	gate.trustedProxyHeader = "X-Forwarded-For"
	gate.trustedProxies = trustedProxiesOf([]string{"203.0.113.7"})
	gate.sourceBudget = newTransportBudget(serverpool.NewAuthRateLimiter(transportFailureLimit, authFailureWindow), authFailureWindow)
	handler := gate.middleware(http.NotFoundHandler())

	post := func(client string) int {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader("{}"))
		req.RemoteAddr = "203.0.113.7:44444"
		req.Header.Set("X-Forwarded-For", client)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}

	for range authFailureLimit {
		post("198.51.100.1")
	}
	if got := post("198.51.100.1"); got != http.StatusTooManyRequests {
		t.Fatalf("the exhausted client got %d, want %d", got, http.StatusTooManyRequests)
	}
	if got := post("198.51.100.2"); got != http.StatusUnauthorized {
		t.Errorf("a second client behind the same proxy got %d, want %d — the fleet must not share one budget",
			got, http.StatusUnauthorized)
	}
}

// TestMCPServerGate_TrustedProxy_TheFleetBudgetCountsClientsNotFailures is the
// acceptance criterion the coarseness of [transportFailureLimit] was supposed
// to buy and did not.
//
// Behind a genuine proxy the transport source is the proxy for every client, so
// charging it once per failure aggregates the whole fleet: fifty clients
// failing their own ten times each spend the five hundred between them, and the
// budget is checked before the credential is read, so the next request through
// that proxy is refused whatever it carries. The rotation the budget exists to
// bound mints a fresh key every request, which is what makes counting keys
// rather than failures separate the two.
func TestMCPServerGate_TrustedProxy_TheFleetBudgetCountsClientsNotFailures(t *testing.T) {
	gate := newGate(t, okFactory)
	gate.trustedProxyHeader = "X-Forwarded-For"
	gate.trustedProxies = trustedProxiesOf([]string{"203.0.113.7"})
	gate.sourceBudget = newTransportBudget(serverpool.NewAuthRateLimiter(transportFailureLimit, authFailureWindow), authFailureWindow)
	handler := gate.middleware(http.NotFoundHandler())

	post := func(client, token string) int {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader("{}"))
		req.RemoteAddr = "203.0.113.7:44444"
		req.Header.Set("X-Forwarded-For", client)
		if token != "" {
			req.Header.Set("PRIVATE-TOKEN", token)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}

	// Every client stays inside its own budget, so nothing here is an abuse
	// the primary limiter would catch: it is an ordinary bad afternoon for a
	// fleet of fifty behind one proxy.
	clients := transportFailureLimit / authFailureLimit
	for client := range clients {
		address := "198.51.100." + strconv.Itoa(client+1)
		for range authFailureLimit {
			if got := post(address, ""); got != http.StatusUnauthorized {
				t.Fatalf("client %s got %d for a credential-less request, want %d", address, got, http.StatusUnauthorized)
			}
		}
	}

	if got := post("198.51.100.251", testToken); got == http.StatusTooManyRequests {
		t.Errorf("a client presenting a valid token through the same proxy got %d; %d failures spread over %d clients locked out the fleet",
			got, transportFailureLimit, clients)
	}
}

// TestTransportBudget_ChargesOncePerKeyPerWindow pins the accounting the gates
// share, without a request in sight.
//
// The source budget is a bound on how many distinct primary keys one transport
// source may mint, not on how often the clients behind it fail. A key that is
// already known keeps its own ten-a-minute budget and costs the source nothing
// further, so the fleet total tracks the number of failing clients.
func TestTransportBudget_ChargesOncePerKeyPerWindow(t *testing.T) {
	t.Parallel()

	budget := newTransportBudget(serverpool.NewAuthRateLimiter(2, authFailureWindow), authFailureWindow)
	const source = "203.0.113.7"

	for range 50 {
		budget.charge(source, "198.51.100.1")
	}
	if blocked, _ := budget.blockedFor(source); blocked {
		t.Error("fifty failures from one client exhausted a budget of two distinct clients")
	}

	budget.charge(source, "198.51.100.2")
	if blocked, _ := budget.blockedFor(source); !blocked {
		t.Error("a second distinct client did not reach a budget of two")
	}
}

// TestTransportBudget_NilBudgetIsInert covers the deployment without
// --trusted-proxy-header, where the primary key is already the transport source
// and a second budget over the same string would only halve it.
func TestTransportBudget_NilBudgetIsInert(t *testing.T) {
	t.Parallel()

	var budget *transportBudget
	budget.charge("203.0.113.7", "203.0.113.7")
	if blocked, remaining := budget.blockedFor("203.0.113.7"); blocked || remaining != 0 {
		t.Errorf("an absent budget answered (%v, %v), want (false, 0)", blocked, remaining)
	}
	if budget.rateLimiter() != nil {
		t.Error("an absent budget handed out a limiter")
	}
	budget.cleanup()
}

// TestTransportSource verifies that the connection's own address is read from
// RemoteAddr and never from a header, since that is the whole point of the
// secondary budget.
func TestTransportSource(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		remote string
		want   string
	}{
		{"host_and_port", "203.0.113.7:44444", "203.0.113.7"},
		{"ipv6", "[2001:db8::1]:443", "2001:db8::1"},
		// A peer without a port is what a unix socket listener reports; it
		// is charged as reported rather than to an empty key.
		{"no_port", "203.0.113.7", "203.0.113.7"},
		{"unix_socket", "@", "@"},
		{"empty", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", http.NoBody)
			req.RemoteAddr = tc.remote
			req.Header.Set("X-Forwarded-For", "198.51.100.9")
			if got := transportSource(req); got != tc.want {
				t.Errorf("transportSource(%q) = %q, want %q", tc.remote, got, tc.want)
			}
		})
	}
}

// TestGateFailure_Write_LogsARefusalTheClientNeverReceived covers the write
// failing under the refusal: the status is already committed, so nothing can
// be resent, and the log line is the only trace that a rejection was lost.
// The writer is the same broken connection the SSE heartbeat tests use.
func TestGateFailure_Write_LogsARefusalTheClientNeverReceived(t *testing.T) {
	logged := testutil.CaptureSlog(t)
	failure := &gateFailure{status: http.StatusUnauthorized, code: errCodeUnauthorized, message: "no token"}
	recorder := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", strings.NewReader(`{"id":7}`))

	failure.write(brokenResponseWriter{recorder}, r)

	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d committed before the body failed", recorder.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(logged.String(), "failed to write gate error response") {
		t.Errorf("log = %q, want the lost refusal reported", logged.String())
	}
}

// TestMcpServerGate_Middleware_RefusesASessionMintedForAnotherCredential
// drives the ownership check through the middleware rather than calling it
// directly, so the refusal is asserted the way a client sees it: a 404 with a
// JSON-RPC body, and the handler behind the gate never reached.
//
// GET and DELETE are covered beside the POST because on a stateful deployment
// they are the two methods that act on a session someone else created — GET
// opens its standalone SSE stream, DELETE terminates it — and because they used
// to be safe structurally rather than by this check: while each credential had a
// server of its own, a session ID could not name another credential's session on
// the server the request resolved to. One server per configuration shape removes
// that guarantee, so the check is the only thing left holding them.
func TestMcpServerGate_Middleware_RefusesASessionMintedForAnotherCredential(t *testing.T) {
	methods := map[string]string{
		http.MethodPost:   `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`,
		http.MethodGet:    "",
		http.MethodDelete: "",
	}

	for method, body := range methods {
		t.Run(method, func(t *testing.T) {
			gate := newGate(t, okFactory)
			// A stateful deployment: GET and DELETE address a live session, so
			// they are resolved and ownership-checked like a POST.
			gate.stateless = false
			// An ownership table that knows nothing about the ID presented, so
			// whatever arrives was opened by somebody else or never existed.
			gate.sessions = newSessionOwners(false)

			var reached atomic.Bool
			handler := gate.middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				reached.Store(true)
			}))

			var payload io.Reader = http.NoBody
			if body != "" {
				payload = strings.NewReader(body)
			}
			req := httptest.NewRequestWithContext(t.Context(), method, "/mcp", payload)
			req.Header.Set("PRIVATE-TOKEN", gateTestToken)
			req.Header.Set(mcpSessionIDHeader, "opened-by-somebody-else")
			recorder := httptest.NewRecorder()

			handler.ServeHTTP(recorder, req)

			if recorder.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d for a session that belongs to another credential: %s",
					recorder.Code, http.StatusNotFound, recorder.Body.String())
			}
			if reached.Load() {
				t.Error("the MCP handler ran on a session the credential does not own")
			}
			// Only the POST carries a JSON-RPC id to echo; the other two have no
			// body at all, and the refusal must omit the member rather than send
			// a null id, which is not a legal RequestId.
			decoded := decodeJSONRPCError(t, recorder.Body.String(), jsonRPCIDFor(method)...)
			if !strings.Contains(decoded.Error.Message, "does not belong") {
				t.Errorf("message = %q, want it to say the session belongs to somebody else", decoded.Error.Message)
			}
		})
	}
}

// jsonRPCIDFor is the id a refusal to method must echo: the one the POST body
// carried, and none for the methods that carry no body.
func jsonRPCIDFor(method string) []string {
	if method == http.MethodPost {
		return []string{"1"}
	}
	return nil
}

// TestMcpServerGate_Middleware_AcceptsASessionTheCredentialOwns is the positive
// half: an ID recorded under the very entry the request resolves to is not a
// refusal, so the check refuses foreign sessions rather than sessions.
func TestMcpServerGate_Middleware_AcceptsASessionTheCredentialOwns(t *testing.T) {
	gitlab := gateStubGitLab(t, false)
	gate := newGateAgainst(t, okFactory, gitlab)
	gate.stateless = false
	gate.sessions = newSessionOwners(false)

	// The entry the gate will resolve for this credential, so the session is
	// recorded under the owner the request arrives as.
	entry := gateTestEntry(t, gate.pool, gateTestToken, gitlab)
	sessionID := newIdentifiedSessions(t).recordUnder(t, gate.sessions, entry.Owner())

	var reached atomic.Bool
	handler := gate.middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached.Store(true)
		w.WriteHeader(http.StatusAccepted)
	}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/mcp", http.NoBody)
	req.Header.Set("PRIVATE-TOKEN", gateTestToken)
	req.Header.Set(mcpSessionIDHeader, sessionID)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if !reached.Load() {
		t.Fatalf("the request was refused with %d: %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d from the handler behind the gate", recorder.Code, http.StatusAccepted)
	}
}

// TestNewTransportBudget_WithoutALimiter_IsAbsent pins that no limiter means
// no budget at all rather than a budget that panics on first use: the nil
// budget is the documented shape of a deployment without a trusted proxy
// header, and every method already tolerates it.
func TestNewTransportBudget_WithoutALimiter_IsAbsent(t *testing.T) {
	t.Parallel()
	if budget := newTransportBudget(nil, authFailureWindow); budget != nil {
		t.Errorf("newTransportBudget(nil) = %+v, want nil", budget)
	}
	limiter := serverpool.NewAuthRateLimiter(2, authFailureWindow)
	if newTransportBudget(limiter, authFailureWindow).rateLimiter() != limiter {
		t.Error("rateLimiter() did not hand back the limiter the budget wraps")
	}
}

// TestLongestAuthBlock_AnswersWithTheLongestActiveBlock covers what a refused
// request is told when more than one budget holds it.
//
// Returning the first active budget understated whenever a shorter one was
// checked first: one failure can raise the minute-long lockout and the
// hour-long distinct-token block together, and a client told to come back in a
// minute spends the other fifty-nine being refused. Retry-After is a promise
// about when the next attempt can succeed.
func TestLongestAuthBlock_AnswersWithTheLongestActiveBlock(t *testing.T) {
	t.Parallel()

	const (
		lockout = time.Minute
		source  = 5 * time.Minute
		spray   = time.Hour
	)

	cases := []struct {
		name                              string
		lockedOut, sourceBlocked, sprayed bool
		wantBlocked                       bool
		wantAfter                         time.Duration
		wantReason                        string
	}{
		{name: "nothing active", wantBlocked: false},
		{name: "lockout alone", lockedOut: true, wantBlocked: true, wantAfter: lockout, wantReason: mcpotel.AuthBlockFailureLockout},
		{name: "source alone", sourceBlocked: true, wantBlocked: true, wantAfter: source, wantReason: mcpotel.AuthBlockTransportSource},
		{name: "spray alone", sprayed: true, wantBlocked: true, wantAfter: spray, wantReason: mcpotel.AuthBlockDistinctTokens},
		{
			name:      "lockout and spray together answer with the spray",
			lockedOut: true, sprayed: true,
			wantBlocked: true, wantAfter: spray, wantReason: mcpotel.AuthBlockDistinctTokens,
		},
		{
			name:      "lockout and source together answer with the source",
			lockedOut: true, sourceBlocked: true,
			wantBlocked: true, wantAfter: source, wantReason: mcpotel.AuthBlockTransportSource,
		},
		{
			name:      "all three answer with the longest",
			lockedOut: true, sourceBlocked: true, sprayed: true,
			wantBlocked: true, wantAfter: spray, wantReason: mcpotel.AuthBlockDistinctTokens,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			blocked, after, reason := longestAuthBlock(
				tc.lockedOut, lockout,
				tc.sourceBlocked, source,
				tc.sprayed, spray,
			)
			if blocked != tc.wantBlocked || after != tc.wantAfter || reason != tc.wantReason {
				t.Errorf("longestAuthBlock() = (%v, %v, %q), want (%v, %v, %q)",
					blocked, after, reason, tc.wantBlocked, tc.wantAfter, tc.wantReason)
			}
		})
	}
}

// TestLongestAuthBlock_EqualDurations_KeepTheEarlierReason pins the tie-break,
// which is the one thing the comparison cannot decide on length. The earlier
// budget wins, so the reason recorded for two blocks of the same length does
// not depend on evaluation order changing.
func TestLongestAuthBlock_EqualDurations_KeepTheEarlierReason(t *testing.T) {
	t.Parallel()

	const equal = 30 * time.Second
	_, after, reason := longestAuthBlock(true, equal, true, equal, true, equal)
	if after != equal {
		t.Errorf("retryAfter = %v, want %v", after, equal)
	}
	if reason != mcpotel.AuthBlockFailureLockout {
		t.Errorf("reason = %q, want the first budget's %q", reason, mcpotel.AuthBlockFailureLockout)
	}
}

// TestLongestAuthBlock_AShorterLaterBlock_KeepsTheLongerEarlierOne covers the
// order the table above never produces, since its blocks lengthen in the
// order they are checked: a budget checked first can hold a request longer
// than the ones after it, because a block is measured by what is left of it.
// A lockout with fifty minutes left and a transport-source block with five
// must still answer fifty, and an escalated distinct-token block that has
// almost run out must not shorten it either.
func TestLongestAuthBlock_AShorterLaterBlock_KeepsTheLongerEarlierOne(t *testing.T) {
	t.Parallel()

	const (
		lockout = 50 * time.Minute
		source  = 5 * time.Minute
		spray   = time.Minute
	)
	blocked, after, reason := longestAuthBlock(true, lockout, true, source, true, spray)
	if !blocked || after != lockout || reason != mcpotel.AuthBlockFailureLockout {
		t.Errorf("longestAuthBlock() = (%v, %v, %q), want (true, %v, %q)",
			blocked, after, reason, lockout, mcpotel.AuthBlockFailureLockout)
	}
}

// TestTransportBudget_Window_FallsBackToTheDefault covers the two shapes a
// caller can leave the effective window in: a nil budget, which is a
// deployment without a trusted proxy header, and one built with a
// non-positive window, which is what a configured zero resolves to. Both
// answer with the default, because that is what their limiter was built with,
// and Retry-After is read off this value.
func TestTransportBudget_Window_FallsBackToTheDefault(t *testing.T) {
	t.Parallel()

	var absent *transportBudget
	if got := absent.window(); got != authFailureWindow {
		t.Errorf("(nil).window() = %v, want %v", got, authFailureWindow)
	}

	limiter := serverpool.NewAuthRateLimiter(2, authFailureWindow)
	if got := newTransportBudget(limiter, 0).window(); got != authFailureWindow {
		t.Errorf("window() with a zero configured window = %v, want %v", got, authFailureWindow)
	}

	const configured = 5 * time.Minute
	if got := newTransportBudget(limiter, configured).window(); got != configured {
		t.Errorf("window() = %v, want the configured %v", got, configured)
	}
}

// TestTransportBudget_Cleanup_ForgetsLapsedPairsOnly covers the sweep the
// periodic cleanup runs: a pair whose window has passed is dropped, so the
// same key charges the source again on its next failure, and a pair still
// inside its window is kept, so it does not.
func TestTransportBudget_Cleanup_ForgetsLapsedPairsOnly(t *testing.T) {
	t.Parallel()

	budget := newTransportBudget(serverpool.NewAuthRateLimiter(3, authFailureWindow), authFailureWindow)
	const source = "203.0.113.7"
	budget.charge(source, "198.51.100.1")
	budget.charge(source, "198.51.100.2")

	budget.mu.Lock()
	budget.charged[source+"\x00"+"198.51.100.1"] = time.Now().Add(-2 * authFailureWindow)
	budget.mu.Unlock()

	budget.cleanup()

	budget.mu.Lock()
	remaining := len(budget.charged)
	budget.mu.Unlock()
	if remaining != 1 {
		t.Fatalf("cleanup left %d pair(s), want 1: the lapsed pair forgotten and the live one kept", remaining)
	}

	// The kept pair is still counted, so charging it again costs nothing.
	budget.charge(source, "198.51.100.2")
	if blocked, _ := budget.blockedFor(source); blocked {
		t.Fatal("a key still inside its window was charged a second time")
	}
	// The forgotten pair is a fresh key again, and it is the third one.
	budget.charge(source, "198.51.100.1")
	if blocked, _ := budget.blockedFor(source); !blocked {
		t.Error("a key whose window lapsed did not charge the source again after cleanup")
	}
}

// TestNewTransportBudget_AZeroWindow_DeduplicatesOverTheDefault covers the
// configuration that reaches the budget with no window of its own: a zero
// --auth-failure-window, which turns the primary budget off and leaves this one
// to fall back to the default.
//
// The fallback has to reach the accounting, not only the value the budget
// reports. A pair remembered for no time at all is forgotten at once, so every
// failure of one client charges the source again, and the fleet budget counts
// failures rather than clients: the very aggregation it exists to avoid.
func TestNewTransportBudget_AZeroWindow_DeduplicatesOverTheDefault(t *testing.T) {
	t.Parallel()

	budget := newTransportBudget(serverpool.NewAuthRateLimiter(2, authFailureWindow), 0)
	const source = "203.0.113.7"
	for range 5 {
		budget.charge(source, "198.51.100.1")
	}
	if blocked, _ := budget.blockedFor(source); blocked {
		t.Error("one client failing five times exhausted a budget of two distinct clients; a zero window deduplicated over nothing")
	}
}

// TestTransportBudget_Charge_RechargesAPairWhoseWindowLapsed covers a pair the
// sweep has not reached yet. Its window has passed, so the key is a fresh
// failing client again and the source pays for it, whether or not cleanup has
// run in between: the sweep bounds the map, and the window is what decides.
func TestTransportBudget_Charge_RechargesAPairWhoseWindowLapsed(t *testing.T) {
	t.Parallel()

	budget := newTransportBudget(serverpool.NewAuthRateLimiter(2, authFailureWindow), authFailureWindow)
	const source, key = "203.0.113.7", "198.51.100.1"
	budget.charge(source, key)

	budget.mu.Lock()
	budget.charged[source+"\x00"+key] = time.Now().Add(-2 * authFailureWindow)
	budget.mu.Unlock()

	budget.charge(source, key)
	if blocked, _ := budget.blockedFor(source); !blocked {
		t.Error("a key whose window lapsed was not charged again; the source is let off a client it has not paid for this window")
	}
}

// TestTransportBudget_Charge_KeepsAPairThroughTheLastInstantOfItsWindow pins
// the edge of the deduplication window to the limiter's own.
//
// The limiter still counts a failure exactly one window after it, and forgets
// it only once more than a window has passed, so the pair that failure came
// from has to stay remembered at that instant too. Re-opening the pair one
// instant early charges the source a second time for a client the limiter
// still holds against it, which is the per-failure aggregation the budget
// exists to avoid. Only a fake clock can land a charge on that instant, which
// is what the bubble is for: nothing here touches a socket.
func TestTransportBudget_Charge_KeepsAPairThroughTheLastInstantOfItsWindow(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		budget := newTransportBudget(serverpool.NewAuthRateLimiter(2, authFailureWindow), authFailureWindow)
		const source, key = "203.0.113.7", "198.51.100.1"
		budget.charge(source, key)

		time.Sleep(authFailureWindow)
		budget.charge(source, key)
		if blocked, _ := budget.blockedFor(source); blocked {
			t.Error("a client charged exactly one window ago was charged again, while the limiter still holds its first failure: the source paid twice for one client")
		}
	})
}

// TestTransportBudget_Cleanup_KeepsAPairThroughTheLastInstantOfItsWindow is
// the sweep's half of the same edge. A sweep that lands exactly one window
// after a pair was charged keeps it, as the limiter's own sweep keeps the
// failure, so the client failing again at that instant does not cost the
// source a second time.
func TestTransportBudget_Cleanup_KeepsAPairThroughTheLastInstantOfItsWindow(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		budget := newTransportBudget(serverpool.NewAuthRateLimiter(2, authFailureWindow), authFailureWindow)
		const source, key = "203.0.113.7", "198.51.100.1"
		budget.charge(source, key)

		time.Sleep(authFailureWindow)
		budget.cleanup()
		budget.charge(source, key)
		if blocked, _ := budget.blockedFor(source); blocked {
			t.Error("a sweep exactly one window after a charge forgot the pair while the limiter kept the failure, so the same client charged the source twice")
		}
	})
}

// TestTransportBudget_Charge_KeepsTheSourceAndKeyApart pins the separator in
// the pair a charge is remembered under.
//
// Two addresses concatenate into one string in more than one way: source
// 10.0.0.1 with key 23.4.5.6 and source 10.0.0.12 with key 3.4.5.6 both read
// 10.0.0.123.4.5.6. Without the separator the second pair is taken for the
// first, and a source that has never been charged is let off its first client.
func TestTransportBudget_Charge_KeepsTheSourceAndKeyApart(t *testing.T) {
	t.Parallel()

	budget := newTransportBudget(serverpool.NewAuthRateLimiter(1, authFailureWindow), authFailureWindow)
	budget.charge("10.0.0.1", "23.4.5.6")
	budget.charge("10.0.0.12", "3.4.5.6")

	for _, source := range []string{"10.0.0.1", "10.0.0.12"} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			if blocked, _ := budget.blockedFor(source); !blocked {
				t.Errorf("source %s was not charged for its one client", source)
			}
		})
	}
}

// TestNewHeader_IgnoresAnOddTrailingName pins what the pair builder does with
// a name that has no value: it is dropped, rather than emitted empty or read
// past the end of the list.
func TestNewHeader_IgnoresAnOddTrailingName(t *testing.T) {
	t.Parallel()

	h := newHeader("WWW-Authenticate", `Bearer realm="x"`, "Retry-After")

	if got := h.Get("WWW-Authenticate"); got != `Bearer realm="x"` {
		t.Errorf("WWW-Authenticate = %q, want the value it was paired with", got)
	}
	if _, present := h["Retry-After"]; present {
		t.Errorf("header = %v, want the unpaired Retry-After left out", h)
	}
	if len(h) != 1 {
		t.Errorf("header = %v, want exactly the one complete pair", h)
	}
}

// TestLongestAuthBlock_AnActiveBlockWithNoTimeLeftStillRefuses covers a budget
// that reports itself blocked with nothing left to wait.
//
// Whether a request is refused is the budget's answer, and the time is only
// what Retry-After says about it. A block ending this instant is still a block,
// and [retryAfterSeconds] answers it with one second; dropping it for having no
// length would admit the request the budget just refused.
func TestLongestAuthBlock_AnActiveBlockWithNoTimeLeftStillRefuses(t *testing.T) {
	t.Parallel()

	blocked, after, reason := longestAuthBlock(true, 0, false, 0, false, 0)
	if !blocked || after != 0 || reason != mcpotel.AuthBlockFailureLockout {
		t.Errorf("longestAuthBlock(active, 0) = (%v, %v, %q), want (true, 0, %q)",
			blocked, after, reason, mcpotel.AuthBlockFailureLockout)
	}
}

// TestMcpServerGate_WithIdentity_AnUnresolvedUserIsLeftAbsent covers the pool
// holding an entry whose user it could not name: GitLab accepted the token and
// answered /user without an id.
//
// The entry exists, so the lookup succeeds, and it is the identity inside it
// that is empty. Storing it would put an identity with an instance and no user
// on the context, which a handler reads as an authenticated call by nobody.
func TestMcpServerGate_WithIdentity_AnUnresolvedUserIsLeftAbsent(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"username":"no-id-sent"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	gate := newGateAgainst(t, okFactory, srv.URL)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", http.NoBody)
	req.Header.Set("PRIVATE-TOKEN", "glpat-anonymous")
	if _, failure := gate.resolve(req); failure != nil {
		t.Fatalf("resolve: %+v", failure)
	}
	if _, pooled := gate.pool.IdentityFor("glpat-anonymous", srv.URL); !pooled {
		t.Fatal("the pool holds no entry for the credential; the case under test is an entry with an unnamed user")
	}

	if got := toolutil.IdentityFromContext(gate.withIdentity(t.Context(), req)); got != (toolutil.UserIdentity{}) {
		t.Errorf("identity = %+v, want nothing stored for a user the pool could not name", got)
	}
}

// TestMCPServerGate_Blocked_RetryAfterIsTheBlockThatHoldsIt is the gate's
// half of the guard test of the same name: the 429 announces what the block
// holding the request has left, ten minutes for the distinct-token budget's
// first rung here, and not the one-minute failure window.
func TestMCPServerGate_Blocked_RetryAfterIsTheBlockThatHoldsIt(t *testing.T) {
	gate := newGateAgainst(t, okFactory, gateStubGitLab(t, true))
	gate.limiter = nil
	gate.spray = serverpool.NewDistinctTokenBudget(2, time.Hour, 10*time.Minute)
	handler := gate.middleware(http.NotFoundHandler())

	post := func(token string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", strings.NewReader("{}"))
		if token != "" {
			req.Header.Set("PRIVATE-TOKEN", token)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	post("glpat-refused-a")
	post("glpat-refused-b")

	rec := post("")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
	assertRetryAfterWithin(t, rec.Header().Get("Retry-After"), 9*time.Minute, 10*time.Minute)
}

// TestMCPServerGate_OAuthMode_EachUnauthorizedNamesItsOwnVerdict drives the
// gate's two 401s in oauth mode, where their challenges differ: RFC 6750
// section 3.1 forbids an error code on the answer to a request that carried no
// credential, and a credential GitLab refused is answered with invalid_token,
// the verdict the bearer guard gives the same judgement. In legacy mode the two
// challenges are the same string, so only this mode can tell them apart.
func TestMCPServerGate_OAuthMode_EachUnauthorizedNamesItsOwnVerdict(t *testing.T) {
	newOAuthGate := func(t *testing.T) *mcpServerGate {
		t.Helper()
		gate := newGateAgainst(t, okFactory, gateStubGitLab(t, true))
		gate.limiter = nil
		gate.oauthMode, gate.bearerOnly = true, true
		gate.challenge = oauthChallenge("api", testMetadataURL)
		return gate
	}
	post := func(t *testing.T, gate *mcpServerGate, bearer string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", strings.NewReader("{}"))
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		rec := httptest.NewRecorder()
		gate.middleware(http.NotFoundHandler()).ServeHTTP(rec, req)
		return rec
	}

	t.Run("no credential", func(t *testing.T) {
		gate := newOAuthGate(t)
		rec := post(t, gate, "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
		if got := rec.Header().Get("WWW-Authenticate"); got != gate.challenge {
			t.Errorf("challenge = %q, want the plain %q: a request that carried nothing has got nothing wrong", got, gate.challenge)
		}
	})

	t.Run("a credential GitLab refused", func(t *testing.T) {
		gate := newOAuthGate(t)
		rec := post(t, gate, "gloas-refused")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
		if got := rec.Header().Get("WWW-Authenticate"); !strings.Contains(got, `error="invalid_token"`) {
			t.Errorf("challenge = %q, want the invalid_token verdict", got)
		}
	})
}

// gateRequestCarrying builds a POST to the MCP endpoint carrying each header
// whose value is not empty.
func gateRequestCarrying(t *testing.T, headers map[string]string) *http.Request {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", strings.NewReader("{}"))
	for name, value := range headers {
		if value != "" {
			req.Header.Set(name, value)
		}
	}
	return req
}

// TestMCPServerGate_EachRefusal_CarriesItsOwnStatusAndCode drives every
// refusal the legacy gate makes through its middleware and reads what a client
// reads: the status, the JSON-RPC code, the challenge, the Retry-After and the
// sentence.
//
// The status and the code travel together and a client may act on either, so
// each is asserted apart; a refusal whose code was taken from a neighboring
// branch reads as a different condition to a client that routes on the code.
// Only a block answers Retry-After. The failure lockout and the transport
// source each have a row, with windows the other does not share, so a block
// announced with its neighbor's remaining time is a different number; the
// distinct-token block is held by
// TestMCPServerGate_Blocked_RetryAfterIsTheBlockThatHoldsIt.
func TestMCPServerGate_EachRefusal_CarriesItsOwnStatusAndCode(t *testing.T) {
	stub := gateStubGitLab(t, false)
	refusing := gateStubGitLab(t, true)

	cases := []struct {
		name string
		// instance is the stub the gate's pool asks about credentials.
		instance string
		// factory builds the pool's servers; nil is okFactory.
		factory serverpool.ServerFactory
		// arm puts the gate in the state under test, or nil for none.
		arm       func(*mcpServerGate)
		token     string
		header    string
		session   string
		status    int
		code      int
		challenge string
		// retryAfter is the length of the block that refused the request,
		// or zero where the refusal must carry no Retry-After; see
		// [assertRetryAfterIsTheBlock].
		retryAfter time.Duration
		says       string
	}{
		{
			name: "no credential", instance: stub,
			status: http.StatusUnauthorized, code: errCodeUnauthorized,
			challenge: legacyAuthChallenge, says: missingTokenMessage,
		},
		{
			name: "a blocked address", instance: stub,
			arm: func(g *mcpServerGate) {
				g.limiter = serverpool.NewAuthRateLimiter(1, authFailureWindow)
				g.limiter.RecordFailure("192.0.2.1")
			},
			token:  gateTestToken,
			status: http.StatusTooManyRequests, code: errCodeTooManyRequests,
			retryAfter: authFailureWindow, says: "Too many failed authentication attempts",
		},
		{
			// The gate's own failure limiter stays armed and unspent, so the
			// one budget holding the request is the source's, and a
			// Retry-After taken from the limiter's side reads as a second.
			name: "a blocked transport source", instance: stub,
			arm: func(g *mcpServerGate) {
				g.sourceBudget = newTransportBudget(serverpool.NewAuthRateLimiter(1, sourceBlockWindow), sourceBlockWindow)
				g.sourceBudget.charge("192.0.2.1", "198.51.100.1")
			},
			token:  gateTestToken,
			status: http.StatusTooManyRequests, code: errCodeTooManyRequests,
			retryAfter: sourceBlockWindow, says: "Too many failed authentication attempts",
		},
		{
			name: "no instance selected where several are published", instance: stub,
			arm:    func(g *mcpServerGate) { g.gitlabURLs = []string{stub, "https://gitlab.other.example"} },
			token:  gateTestToken,
			status: http.StatusBadRequest, code: errCodeInvalidRequest,
			says: "Ask the operator which instances it publishes.",
		},
		{
			name: "no instance named where none is published", instance: stub,
			arm:    func(g *mcpServerGate) { g.gitlabURLs = nil },
			token:  gateTestToken,
			status: http.StatusBadRequest, code: errCodeInvalidRequest,
			says: capitalizeFirst(serverpool.ErrMissingGitLabURL.Error()) + ".",
		},
		{
			name: "a destination the caller named that this server will not dial", instance: stub,
			arm:   func(g *mcpServerGate) { g.gitlabURLs = nil },
			token: gateTestToken, header: "http://169.254.169.254",
			status: http.StatusBadRequest, code: errCodeInvalidRequest,
			says: "destination refused",
		},
		{
			name: "a credential GitLab refused", instance: refusing,
			token:  "glpat-refused",
			status: http.StatusUnauthorized, code: errCodeUnauthorized,
			challenge: legacyAuthChallenge, says: "GitLab rejected this token.",
		},
		{
			name: "a pool that could not be built", instance: stub, factory: failingFactory,
			token:  gateTestToken,
			status: http.StatusServiceUnavailable, code: errCodeUpstreamUnavailable,
			says: "retry shortly",
		},
		{
			name: "a session another credential owns", instance: stub,
			arm:   func(g *mcpServerGate) { g.sessions = newSessionOwners(false) },
			token: gateTestToken, session: "opened-by-somebody-else",
			status: http.StatusNotFound, code: errCodeInvalidRequest,
			says: "does not belong to the presented credential",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			factory := tc.factory
			if factory == nil {
				factory = okFactory
			}
			gate := newGateAgainst(t, factory, tc.instance)
			if tc.arm != nil {
				tc.arm(gate)
			}
			rec := httptest.NewRecorder()
			gate.middleware(http.NotFoundHandler()).ServeHTTP(rec, gateRequestCarrying(t, map[string]string{
				"PRIVATE-TOKEN":                   tc.token,
				serverpool.RequestOptionGitLabURL: tc.header,
				mcpSessionIDHeader:                tc.session,
			}))

			if rec.Code != tc.status {
				t.Errorf("status = %d, want %d", rec.Code, tc.status)
			}
			decoded := decodeJSONRPCError(t, rec.Body.String())
			if decoded.Error.Code != tc.code {
				t.Errorf("error.code = %d, want %d", decoded.Error.Code, tc.code)
			}
			if got := rec.Header().Get("WWW-Authenticate"); got != tc.challenge {
				t.Errorf("WWW-Authenticate = %q, want %q", got, tc.challenge)
			}
			assertRetryAfterIsTheBlock(t, rec.Header().Get("Retry-After"), tc.retryAfter)
			if !strings.Contains(decoded.Error.Message, tc.says) {
				t.Errorf("message = %q, want it to carry %q", decoded.Error.Message, tc.says)
			}
		})
	}
}

// TestMCPServerGate_OAuthMode_BuildsTheEntryOnTheScopesTheBearerCarried covers
// the scopes the bearer middleware verified reaching the pool.
//
// The pool cannot ask GitLab what an OAuth access token may do, since the PAT
// self endpoint does not answer for one, so the scopes introspection already
// returned are the only account of its authority there is. A read_api bearer
// whose scopes were dropped on the way is built as a token of unknown
// authority, which is to say one that may write, and is served the writing
// surface. The stub answers /user and nothing else, which is what an instance
// asked about an OAuth token's scopes effectively does.
func TestMCPServerGate_OAuthMode_BuildsTheEntryOnTheScopesTheBearerCarried(t *testing.T) {
	stub := gateStubGitLab(t, false)
	pool := serverpool.New(&config.Config{GitLabURL: stub, Tier: edition.Free, TierExplicit: true}, okFactory)
	t.Cleanup(pool.Close)
	gate := &mcpServerGate{
		pool: pool, gitlabURLs: []string{stub},
		challenge: oauthChallenge("api", testMetadataURL), oauthMode: true, bearerOnly: true,
	}
	verify := func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
		return &auth.TokenInfo{UserID: "7", Scopes: []string{"read_api"}, Expiration: time.Now().Add(time.Hour)}, nil
	}
	handler := auth.RequireBearerToken(verify, nil)(gate.middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer gloas-read-only")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	entry := gateTestEntry(t, pool, "gloas-read-only", stub)
	if cfg := entry.Config(); !cfg.ReadAPIOnly || !slices.Equal(cfg.TokenScopes, []string{"read_api"}) {
		t.Errorf("entry built with scopes %v, read_api only %v; want the bearer's [read_api] and the surface read_api reaches",
			cfg.TokenScopes, cfg.ReadAPIOnly)
	}
}

// TestMCPServerGate_OAuthMode_TheExemptionIsJudgedOnTheBearer covers which
// credential the address-block exemption asks the pool about in oauth mode.
//
// The request executes as its bearer, never as a PRIVATE-TOKEN it may also
// carry, so the bearer is what has to be admitted. A request pairing an
// admitted PRIVATE-TOKEN with a bearer nobody has verified would otherwise be
// let past the block on the strength of a credential it will not run as, and
// the unverified bearer would be taken to GitLab from an address that is
// blocked.
func TestMCPServerGate_OAuthMode_TheExemptionIsJudgedOnTheBearer(t *testing.T) {
	stub := gateStubGitLab(t, false)
	gate := newGateAgainst(t, okFactory, stub)
	gate.oauthMode, gate.bearerOnly = true, true
	gate.challenge = oauthChallenge("api", testMetadataURL)
	gateTestEntry(t, gate.pool, "gloas-admitted", stub)
	gate.limiter = serverpool.NewAuthRateLimiter(1, authFailureWindow)
	gate.limiter.RecordFailure("192.0.2.1")

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", strings.NewReader("{}"))
	req.Header.Set("PRIVATE-TOKEN", "gloas-admitted")
	req.Header.Set("Authorization", "Bearer gloas-never-verified")
	rec := httptest.NewRecorder()
	gate.middleware(http.NotFoundHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("status = %d, want %d: the bearer is not admitted, whatever PRIVATE-TOKEN says", rec.Code, http.StatusTooManyRequests)
	}
}

// TestMCPServerGate_Middleware_HandsTheHandlerWhatItResolved covers what the
// gate puts on the request it lets through, read where the handler reads it:
// the server the SDK callback returns, the credential the binding middleware
// installs, the identity handlers log under with the instance it belongs to,
// and the mark that makes the request wait for the catalog.
//
// It runs in oauth mode with a PRIVATE-TOKEN beside the bearer, both valid and
// naming different users, so every one of those answers has to come from the
// bearer: the credential the request executes as.
func TestMCPServerGate_Middleware_HandsTheHandlerWhatItResolved(t *testing.T) {
	mux := http.NewServeMux()
	users := map[string]string{
		"gloas-user-1": `{"id":1,"username":"user-1"}`,
		"gloas-user-2": `{"id":2,"username":"user-2"}`,
	}
	mux.HandleFunc("GET /api/v4/user", func(w http.ResponseWriter, r *http.Request) {
		body, known := users[r.Header.Get("PRIVATE-TOKEN")]
		if !known {
			http.Error(w, `{"message":"401 Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	gate := newGateAgainst(t, okFactory, srv.URL)
	gate.oauthMode, gate.bearerOnly = true, true
	gate.challenge = oauthChallenge("api", testMetadataURL)
	entry := gateTestEntry(t, gate.pool, "gloas-user-2", srv.URL)
	state := &credentialState{owner: entry.Owner()}
	gate.credentials = &credentialStates{}
	gate.credentials.add(state)

	var reached bool
	handler := gate.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		if got := serverFromRequestContext(r); got != entry.Server() {
			t.Errorf("server = %p, want the bearer's entry's %p", got, entry.Server())
		}
		if got := credentialFromRequestContext(r.Context()); got != state {
			t.Errorf("credential = %v, want the bearer's entry's state", got)
		}
		want := toolutil.UserIdentity{UserID: "2", Username: "user-2", Instance: srv.URL}
		if got := toolutil.IdentityFromContext(r.Context()); got != want {
			t.Errorf("identity = %+v, want %+v", got, want)
		}
		if !readinessEnforced(r.Context()) {
			t.Error("the request was not marked to wait for the catalog")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", strings.NewReader("{}"))
	req.Header.Set("PRIVATE-TOKEN", "gloas-user-1")
	req.Header.Set("Authorization", "Bearer gloas-user-2")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !reached {
		t.Fatalf("the request was refused with %d: %s", rec.Code, rec.Body.String())
	}
}

// gateSpend is one request a test sends to put a budget in the state under
// test: the forwarded client address, and the credential it carries, if any.
type gateSpend struct{ client, token string }

// TestMCPServerGate_ABlockedRequest_IsCountedUnderTheBudgetThatRefusedIt holds
// the gate to the telemetry an operator reads: each refusal is counted once,
// under the budget that made it, and under no other.
//
// Every request arrives through one trusted proxy, so the client a failure is
// charged to and the source the fleet budget is charged to are different
// addresses, and a count filed under the wrong one of them is visible.
func TestMCPServerGate_ABlockedRequest_IsCountedUnderTheBudgetThatRefusedIt(t *testing.T) {
	const proxy = "203.0.113.7"

	cases := []struct {
		name    string
		arm     func(*mcpServerGate)
		spend   []gateSpend
		refused string
		want    mcpotel.AuthBlockCounts
	}{
		{
			name: "the failure lockout",
			arm: func(g *mcpServerGate) {
				g.limiter = serverpool.NewAuthRateLimiter(1, authFailureWindow)
			},
			spend:   []gateSpend{{"198.51.100.1", ""}},
			refused: "198.51.100.1",
			want:    mcpotel.AuthBlockCounts{FailureLockout: 1},
		},
		{
			name: "the transport source",
			arm: func(g *mcpServerGate) {
				g.limiter = nil
				g.sourceBudget = newTransportBudget(serverpool.NewAuthRateLimiter(1, authFailureWindow), authFailureWindow)
			},
			spend:   []gateSpend{{"198.51.100.1", ""}},
			refused: "198.51.100.2",
			want:    mcpotel.AuthBlockCounts{TransportSource: 1},
		},
		{
			name: "the distinct-token budget",
			arm: func(g *mcpServerGate) {
				g.limiter = nil
				g.spray = serverpool.NewDistinctTokenBudget(2, time.Minute, time.Minute)
			},
			spend:   []gateSpend{{"198.51.100.1", "glpat-refused-a"}, {"198.51.100.1", "glpat-refused-b"}},
			refused: "198.51.100.1",
			want:    mcpotel.AuthBlockCounts{DistinctTokens: 1},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gate := newGateAgainst(t, okFactory, gateStubGitLab(t, true))
			gate.trustedProxyHeader = "X-Forwarded-For"
			gate.trustedProxies = trustedProxiesOf([]string{proxy})
			gate.blocks = &authBlockCounters{}
			tc.arm(gate)
			handler := gate.middleware(http.NotFoundHandler())

			post := func(client, token string) int {
				req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", strings.NewReader("{}"))
				req.RemoteAddr = proxy + ":44444"
				req.Header.Set("X-Forwarded-For", client)
				if token != "" {
					req.Header.Set("PRIVATE-TOKEN", token)
				}
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
				return rec.Code
			}

			for _, s := range tc.spend {
				if got := post(s.client, s.token); got != http.StatusUnauthorized {
					t.Fatalf("spending the budget from %s got %d, want %d", s.client, got, http.StatusUnauthorized)
				}
			}
			if got := gate.blocks.counts(); got != (mcpotel.AuthBlockCounts{}) {
				t.Fatalf("counts before any refusal = %+v, want none: a failure is not a block", got)
			}
			if got := post(tc.refused, ""); got != http.StatusTooManyRequests {
				t.Fatalf("the request from %s got %d, want %d", tc.refused, got, http.StatusTooManyRequests)
			}
			if got := gate.blocks.counts(); got != tc.want {
				t.Errorf("counts = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// gatePermissionGitLab is a stub instance that answers the credential probe,
// GET /api/v4/user, with GitLab's refusal of a fine-grained permission
// carrying sentence, counting the probes it is asked.
func gatePermissionGitLab(t *testing.T, sentence string, probes *atomic.Int64) string {
	t.Helper()
	body, err := json.Marshal(map[string]string{"error": "insufficient_granular_scope", "error_description": sentence})
	if err != nil {
		t.Fatalf("marshal the refusal: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		probes.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write(body)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

// TestMCPServerGate_FineGrainedTokenWithoutUserRead_IsForbiddenUncharged
// covers the legacy door for a fine-grained token GitLab accepted and refused
// the permission to read its own user. It is answered 403 with no challenge
// and GitLab's sentence in the body, never charged however often it comes back
// (a request carrying no credential afterwards still gets the plain 401 rather
// than the 429 an exhausted budget would give), and remembered, so the
// instance is probed once for every one of those requests. Without a
// rejected-token structure it is answered the same and probed each time.
func TestMCPServerGate_FineGrainedTokenWithoutUserRead_IsForbiddenUncharged(t *testing.T) {
	const requests = authFailureLimit * 2
	tests := []struct {
		name       string
		remember   bool
		wantProbes int64
	}{
		{name: "remembered", remember: true, wantProbes: 1},
		{name: "not remembered", wantProbes: requests},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var probes atomic.Int64
			gate := newGateAgainst(t, okFactory, gatePermissionGitLab(t, userReadSentence, &probes))
			if tt.remember {
				gate.rejected = oauth.NewRejectedTokens(8, time.Minute)
			}
			handler := gate.middleware(http.NotFoundHandler())

			for i := range requests {
				assertLegacyPermissionRefusal(t, i, handler)
			}
			if got := probes.Load(); got != tt.wantProbes {
				t.Errorf("GET /api/v4/user was asked %d times for %d requests, want %d", got, requests, tt.wantProbes)
			}
			if gate.pool.Size() != 0 {
				t.Errorf("pool size = %d, want 0: a credential the probe was not answered for is not served", gate.pool.Size())
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader("{}")))
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d: the permission refusals must not have spent the failure budget", rec.Code, http.StatusUnauthorized)
			}
		})
	}
}

// TestMCPServerGate_BehindTheGuard_AnswersInTheGuardsWords covers the gate in
// oauth mode, which refuses a genuine credential only where the verifier in
// front could not judge it: a token below the minimum the verifier admitted
// on its own api assumption, a personal access token no introspection
// describes, and one refused User: Read where the pool's probe and the
// verifier's disagree. The first answer, from the pool, and the later one,
// from memory, are both the bearer guard's refusal with its insufficient_scope
// challenge (RFC 6750 section 3.1), the answer the guard itself gives every
// later request with that credential; before, the first one was the legacy
// gate's, challenge-free and, below the minimum, worded otherwise.
func TestMCPServerGate_BehindTheGuard_AnswersInTheGuardsWords(t *testing.T) {
	guard := newTestGuard(nil)
	tests := []struct {
		name     string
		instance func(t *testing.T, probes *atomic.Int64) string
		want     *gateFailure
	}{
		{
			name: "below the minimum",
			instance: func(t *testing.T, probes *atomic.Int64) string {
				t.Helper()
				return gateBelowMinimumGitLab(t, false, probes)
			},
			want: guard.insufficientScopeFailure(),
		},
		{
			name: "refused User: Read",
			instance: func(t *testing.T, probes *atomic.Int64) string {
				t.Helper()
				return gatePermissionGitLab(t, userReadSentence, probes)
			},
			want: guard.permissionMissingFailure(oauth.QuotedDescription(userReadSentence)),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var probes atomic.Int64
			gate := newGateAgainst(t, okFactory, tt.instance(t, &probes))
			gate.challenge = oauthChallenge(oauth.ScopeAPI, testMetadataURL)
			gate.oauthMode, gate.bearerOnly = true, true
			gate.rejected = oauth.NewRejectedTokens(8, time.Minute)
			gate.guard = guard

			for _, when := range []string{"found by the pool", "answered from memory"} {
				t.Run(when, func(t *testing.T) {
					assertGuardsAnswer(t, gate, tt.want)
				})
			}
			if got := probes.Load(); got != 1 {
				t.Errorf("GET /api/v4/user was asked %d times, want once: the second answer comes from memory", got)
			}
		})
	}
}

// assertGuardsAnswer resolves one bearer request through gate and holds the
// refusal to want, the guard's own: status, code, words and challenge.
func assertGuardsAnswer(t *testing.T, gate *mcpServerGate, want *gateFailure) {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", http.NoBody)
	req.Header.Set("Authorization", "Bearer glpat-narrow")
	_, failure := gate.resolve(req)
	if failure == nil || failure.status != want.status || failure.code != want.code || failure.message != want.message {
		t.Fatalf("resolve failure = %+v, want the guard's %+v", failure, want)
	}
	if got := failure.header.Get(headerWWWAuthenticate); got != want.header.Get(headerWWWAuthenticate) || !strings.Contains(got, `error="insufficient_scope"`) {
		t.Errorf("WWW-Authenticate = %q, want the guard's insufficient_scope challenge %q", got, want.header.Get(headerWWWAuthenticate))
	}
}

// gateBelowMinimumGitLab is an instance that knows the token and finds it
// below the read_api minimum, in one of the two ways GitLab says so: the
// credential probe refused for want of a scope (refuseProbe), or accepted with
// the token's own description naming read_user alone. Every probe is counted.
func gateBelowMinimumGitLab(t *testing.T, refuseProbe bool, probes *atomic.Int64) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		probes.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if refuseProbe {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"insufficient_scope","error_description":"The request requires higher privileges than provided by the access token.","scope":"api read_api read_user"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":42,"username":"reader"}`))
	})
	mux.HandleFunc("GET /api/v4/personal_access_tokens/self", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"scopes":["read_user"],"active":true}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

// assertBelowMinimumAnswers sends handler requests POSTs carrying the token
// below the minimum and holds each answer to the legacy door's refusal of it:
// 403 with no challenge, -40300 and the door's fixed words.
func assertBelowMinimumAnswers(t *testing.T, handler http.Handler, requests int) {
	t.Helper()
	for i := range requests {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader("{}"))
		req.Header.Set("PRIVATE-TOKEN", "glpat-narrow")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden || rec.Header().Get("WWW-Authenticate") != "" {
			t.Fatalf("request %d: status %d with challenge %q, want 403 with none", i, rec.Code, rec.Header().Get("WWW-Authenticate"))
		}
		if decoded := decodeJSONRPCError(t, rec.Body.String()); decoded.Error.Code != errCodeForbidden || decoded.Error.Message != belowMinimumMessage {
			t.Errorf("request %d: error = %d %q\nwant %d %q", i, decoded.Error.Code, decoded.Error.Message, errCodeForbidden, belowMinimumMessage)
		}
	}
}

// TestMCPServerGate_TokenBelowTheMinimum_IsForbiddenUnchargedAndRemembered
// covers the legacy door for a token GitLab accepted that carries neither
// read_api nor api (issue 952), whichever way GitLab says so. It is answered
// 403 with no challenge, in the door's fixed words, never charged however
// often it comes back (a request carrying no credential afterwards still gets
// the plain 401 rather than the 429 an exhausted budget would give), builds no
// entry, and is remembered, so the instance is probed once for every one of
// those requests. Before, the token GET /api/v4/user refused was answered 401
// and charged, and the read_user one was admitted and failed on every call. A
// gate with no rejected-token cache answers the same and stays uncharged, and
// asks the instance on every request, since nothing remembers the verdict.
func TestMCPServerGate_TokenBelowTheMinimum_IsForbiddenUnchargedAndRemembered(t *testing.T) {
	const requests = authFailureLimit * 2
	for _, tt := range []struct {
		name        string
		refuseProbe bool
		noCache     bool
	}{
		{name: "refused by the probe for want of a scope", refuseProbe: true},
		{name: "described as carrying read_user"},
		{name: "refused by the probe, with no rejected-token cache", refuseProbe: true, noCache: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var probes atomic.Int64
			gate := newGateAgainst(t, okFactory, gateBelowMinimumGitLab(t, tt.refuseProbe, &probes))
			wantProbes := int64(requests)
			if !tt.noCache {
				gate.rejected = oauth.NewRejectedTokens(8, time.Minute)
				wantProbes = 1
			}
			handler := gate.middleware(http.NotFoundHandler())

			assertBelowMinimumAnswers(t, handler, requests)
			if got := probes.Load(); got != wantProbes {
				t.Errorf("GET /api/v4/user was asked %d times for %d requests, want %d", got, requests, wantProbes)
			}
			if gate.pool.Size() != 0 {
				t.Errorf("pool size = %d, want 0: a token below the minimum is not served", gate.pool.Size())
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader("{}")))
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d: the refusals must not have spent the failure budget", rec.Code, http.StatusUnauthorized)
			}
		})
	}
}

// assertLegacyPermissionRefusal sends request i, carrying the fine-grained
// token, through handler and holds its answer to the legacy door's permission
// refusal: 403, no challenge, and the fixed words followed by GitLab's
// sentence.
func assertLegacyPermissionRefusal(t *testing.T, i int, handler http.Handler) {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader("{}"))
	req.Header.Set("PRIVATE-TOKEN", "glpat-fine-grained")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("request %d: status = %d, want %d", i, rec.Code, http.StatusForbidden)
	}
	if challenge := rec.Header().Get("WWW-Authenticate"); challenge != "" {
		t.Errorf("request %d: WWW-Authenticate = %q, want none on the legacy door's 403", i, challenge)
	}
	decoded := decodeJSONRPCError(t, rec.Body.String())
	if want := doorPermissionPrefix + doorPermissionAdvice + " GitLab said: " + userReadSentence; decoded.Error.Code != errCodeForbidden || decoded.Error.Message != want {
		t.Errorf("request %d: error = %d %q\nwant %d %q", i, decoded.Error.Code, decoded.Error.Message, errCodeForbidden, want)
	}
}

// TestMCPServerGate_PermissionMissing_QuotesAHostileSentenceOnlyFilteredAndCut
// is the legacy door's half of the bearer guard's test of the same name. The
// instance's sentence, which under --allow-any-gitlab-url is the caller's own,
// carries quotes, a backslash, control characters and another script: the
// first answer, the one GitLab was asked for, quotes it only as printable
// ASCII within 512 bytes with its printable text kept, and so does the answer
// served from memory. A gate without a rejected-token structure answers the
// same, asking GitLab each time.
func TestMCPServerGate_PermissionMissing_QuotesAHostileSentenceOnlyFilteredAndCut(t *testing.T) {
	// As long as the probe allows: GitLab's whole error document around it
	// has to fit the 4 KiB the credential probe reads, or the probe reads a
	// truncated document and proves only a refusal.
	hostile := `Access denied: "quoted" \ back` + "\r\n\x00é" + strings.Repeat("x", 3900)
	for name, remember := range map[string]bool{"remembered": true, "not remembered": false} {
		t.Run(name, func(t *testing.T) {
			var probes atomic.Int64
			gate := newGateAgainst(t, okFactory, gatePermissionGitLab(t, hostile, &probes))
			if remember {
				gate.rejected = oauth.NewRejectedTokens(8, time.Minute)
			}
			handler := gate.middleware(http.NotFoundHandler())

			for i := range 2 {
				assertLegacyHostileQuotation(t, i, handler)
				wantProbes := int64(i + 1)
				if remember {
					wantProbes = 1
				}
				if got := probes.Load(); got != wantProbes {
					t.Errorf("request %d: GET /api/v4/user was asked %d times, want %d", i, got, wantProbes)
				}
			}
		})
	}
}

// assertLegacyHostileQuotation sends request i, carrying the token a hostile
// instance refused, through handler and holds the legacy door's answer to
// quoting that instance's sentence only filtered and cut: 403, the fixed words,
// and then printable ASCII within 512 bytes that keeps the sentence's
// printable start.
func assertLegacyHostileQuotation(t *testing.T, i int, handler http.Handler) {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader("{}"))
	req.Header.Set("PRIVATE-TOKEN", "glpat-hostile")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("request %d: status = %d, want %d: %s", i, rec.Code, http.StatusForbidden, rec.Body.String())
	}
	message := decodeJSONRPCError(t, rec.Body.String()).Error.Message
	quoted, found := strings.CutPrefix(message, doorPermissionPrefix+doorPermissionAdvice+" GitLab said: ")
	if !found {
		t.Fatalf("request %d: message = %q, want the prefix, the advice and GitLab's sentence", i, message)
	}
	if len(quoted) > 512 || strings.ContainsFunc(quoted, func(r rune) bool { return r < ' ' || r > '~' }) {
		t.Errorf("request %d: quoted sentence is %d bytes and carries %q, want printable ASCII within 512 bytes", i, len(quoted), quoted)
	}
	if !strings.HasPrefix(quoted, `Access denied: "quoted" \ back x`) {
		t.Errorf("request %d: quoted sentence = %q, want its printable text kept", i, quoted)
	}
}

// TestMCPServerGate_KnownRefusalOfAnotherKind_IsNotTheDoorPermissionRefusal
// covers the gate reading a rejected-token structure that knows the presented
// credential under another kind. That is the structure's ordinary state in
// oauth mode, where the bearer guard shares it and records invalid tokens and
// tokens of an application the deployment does not admit there too. The gate
// answers only the missing permission from memory, so for either of the others
// it asks the pool, which probes GitLab and admits the credential; the
// permission row, recorded under the same key, is what shows the lookup finds
// the credential at all.
func TestMCPServerGate_KnownRefusalOfAnotherKind_IsNotTheDoorPermissionRefusal(t *testing.T) {
	tests := []struct {
		name       string
		record     func(rejected *oauth.RejectedTokens, instance string)
		wantStatus int
		wantAsked  bool
	}{
		{
			name:      "an invalid token",
			record:    func(rejected *oauth.RejectedTokens, instance string) { rejected.Record(instance, gateTestToken) },
			wantAsked: true,
		},
		{
			name: "a token of an application the deployment does not admit",
			record: func(rejected *oauth.RejectedTokens, instance string) {
				rejected.RecordKind(instance, gateTestToken, oauth.RejectionUnaccepted)
			},
			wantAsked: true,
		},
		{
			name: "the missing permission",
			record: func(rejected *oauth.RejectedTokens, instance string) {
				rejected.RecordPermissionMissing(instance, gateTestToken, userReadSentence)
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "a token below the minimum",
			record: func(rejected *oauth.RejectedTokens, instance string) {
				rejected.RecordBelowMinimum(instance, gateTestToken)
			},
			wantStatus: http.StatusForbidden,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var probes atomic.Int64
			mux := http.NewServeMux()
			mux.HandleFunc("GET /api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
				probes.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":42,"username":"testuser"}`))
			})
			instance := httptest.NewServer(mux)
			t.Cleanup(instance.Close)
			gate := newGateAgainst(t, okFactory, instance.URL)
			gate.rejected = oauth.NewRejectedTokens(8, time.Minute)
			tt.record(gate.rejected, instance.URL)

			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", http.NoBody)
			req.Header.Set("PRIVATE-TOKEN", gateTestToken)
			_, failure := gate.resolve(req)
			gotStatus := 0
			if failure != nil {
				gotStatus = failure.status
			}
			if gotStatus != tt.wantStatus {
				t.Errorf("resolve failure = %+v, want status %d (0 is admission)", failure, tt.wantStatus)
			}
			if n := probes.Load(); (n > 0) != tt.wantAsked {
				t.Errorf("GET /api/v4/user was asked %d times, want asked = %v", n, tt.wantAsked)
			}
		})
	}
}

// TestDoorPermissionDetail_QuotesGitLabOnlyWhenItSaidSomething holds the tail
// of the doors' permission refusal: the advice alone when GitLab gave no
// sentence, and the advice followed by the sentence when it did.
func TestDoorPermissionDetail_QuotesGitLabOnlyWhenItSaidSomething(t *testing.T) {
	if got := doorPermissionDetail(""); got != doorPermissionAdvice {
		t.Errorf("doorPermissionDetail(\"\") = %q, want the advice alone", got)
	}
	if got, want := doorPermissionDetail("Access denied."), doorPermissionAdvice+" GitLab said: Access denied."; got != want {
		t.Errorf("doorPermissionDetail = %q, want %q", got, want)
	}
}

// TestSentencePermissions_CountsTheListAndNamesAtMostThree holds what a door's
// log line reads of GitLab's sentence: every name of the bracketed list
// counted, at most three named, each cut at 64 bytes, and nothing read from a
// sentence with no list, an unclosed one or an empty one.
func TestSentencePermissions_CountsTheListAndNamesAtMostThree(t *testing.T) {
	long := strings.Repeat("n", maxLoggedPermissionBytes+10)
	tests := []struct {
		name      string
		sentence  string
		wantCount int
		wantNames []string
	}{
		{name: "one permission", sentence: userReadSentence, wantCount: 1, wantNames: []string{"User: Read"}},
		{
			name:      "more than three",
			sentence:  "Access denied: [Project: Read, Issue: Read, Issue: Create, Merge Request: Read]",
			wantCount: 4, wantNames: []string{"Project: Read", "Issue: Read", "Issue: Create"},
		},
		{name: "a long name is cut", sentence: "[" + long + "]", wantCount: 1, wantNames: []string{long[:maxLoggedPermissionBytes]}},
		{name: "the last list is read", sentence: "a [b] then [User: Read]", wantCount: 1, wantNames: []string{"User: Read"}},
		{name: "empty entries are skipped", sentence: "[, User: Read ,]", wantCount: 1, wantNames: []string{"User: Read"}},
		{name: "no list", sentence: "Access denied: Fine-grained personal access tokens are not yet supported."},
		{name: "an unclosed list", sentence: "Access denied: [User: Read"},
		{name: "an empty list", sentence: "Access denied: []"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, names := sentencePermissions(tt.sentence)
			if count != tt.wantCount || !slices.Equal(names, tt.wantNames) {
				t.Errorf("sentencePermissions(%q) = %d, %q; want %d, %q", tt.sentence, count, names, tt.wantCount, tt.wantNames)
			}
		})
	}
}

// TestMCPServerGate_ARefusedToken_IsLoggedByItsHandleAndNoneOfIt covers the
// legacy gate's lines about a credential it refuses, the refusal of a
// fine-grained token's missing permission aside, which names nothing about the
// caller: GitLab's rejection, and a token carrying neither read_api nor api,
// remembered or learned from the pool.
//
// The rejection used to carry the last four characters of whatever was sent,
// often a mistyped or expired token and sometimes not a token at all, and the
// two below the minimum carried nothing, while the bearer guard's lines for
// the same verdict named the token. All three carry the keyed handle the
// bearer guard's lines and the pool's own line carry.
func TestMCPServerGate_ARefusedToken_IsLoggedByItsHandleAndNoneOfIt(t *testing.T) {
	const (
		token    = "glpat-rejected-token-" + loggedTokenTail
		instance = "https://gitlab.example.com"
		address  = "192.0.2.10"
	)
	tests := []struct {
		name       string
		refuse     func(ctx context.Context, gate *mcpServerGate) *gateFailure
		wantStatus int
		msg        string
	}{
		{
			name: "gitlab rejected the token",
			refuse: func(ctx context.Context, gate *mcpServerGate) *gateFailure {
				return gate.classify(ctx, serverpool.ErrInvalidCredential, address, address, instance, token)
			},
			wantStatus: http.StatusUnauthorized,
			msg:        "request rejected: gitlab rejected the supplied token",
		},
		{
			name: "the pool learned the token is below the minimum",
			refuse: func(ctx context.Context, gate *mcpServerGate) *gateFailure {
				return gate.classify(ctx, serverpool.ErrCredentialBelowMinimum, address, address, instance, token)
			},
			wantStatus: http.StatusForbidden,
			msg:        "request rejected at the gate: gitlab accepted the token, which carries neither read_api nor api",
		},
		{
			name: "the token is remembered as below the minimum",
			refuse: func(ctx context.Context, gate *mcpServerGate) *gateFailure {
				gate.rejected.RecordBelowMinimum(instance, token)
				req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/mcp", http.NoBody)
				req.Header.Set("PRIVATE-TOKEN", token)
				_, failure := gate.resolve(req)
				return failure
			},
			wantStatus: http.StatusForbidden,
			msg:        "request rejected at the gate: token already known to carry neither read_api nor api",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logged := captureJSONLog(t)
			gate := newGateAgainst(t, okFactory, instance)
			gate.rejected = oauth.NewRejectedTokens(8, time.Minute)

			failure := tt.refuse(t.Context(), gate)

			if failure == nil || failure.status != tt.wantStatus {
				t.Fatalf("failure = %+v, want status %d", failure, tt.wantStatus)
			}
			assertNamesTheTokenByItsHandle(t, logged, tt.msg, token)
		})
	}
}
