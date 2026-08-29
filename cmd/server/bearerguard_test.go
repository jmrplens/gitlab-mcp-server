// bearerguard_test.go verifies the OAuth pre-authentication guard: what it
// costs upstream, what it charges to the rate limiter, and what it tells the
// client in the RFC 6750 challenge.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/oauth"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/serverpool"
)

const testMetadataURL = "https://mcp.example.com/.well-known/oauth-protected-resource"

// newTestGuard returns a guard whose verifier is the supplied stub, wired to
// live rejection and limiter state the test can inspect afterwards.
func newTestGuard(verify auth.TokenVerifier) *bearerGuard {
	return &bearerGuard{
		verify:          verify,
		rejected:        oauth.NewRejectedTokens(16, time.Minute),
		limiter:         serverpool.NewAuthRateLimiter(3, time.Minute),
		metadataURL:     testMetadataURL,
		minimumScope:    oauth.MinimumScope,
		advertisedScope: oauth.ScopeAPI,
	}
}

// guardRequest builds a POST carrying the given bearer token, or none when
// token is empty.
func guardRequest(t *testing.T, token string) *http.Request {
	t.Helper()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", http.NoBody)
	r.RemoteAddr = "192.0.2.10:5555"
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	return r
}

// okVerifier answers every token as a valid identity carrying scopes.
func okVerifier(scopes ...string) auth.TokenVerifier {
	return func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
		return &auth.TokenInfo{UserID: "7", Scopes: scopes, Expiration: time.Now().Add(time.Hour)}, nil
	}
}

// TestBearerGuard_MissingCredential_ChallengesWithoutAnErrorCode verifies RFC
// 6750 section 3.1: a challenge answering a request that carried no
// credential must not name an error code, because the client has not got
// anything wrong yet — it simply has not authenticated.
func TestBearerGuard_MissingCredential_ChallengesWithoutAnErrorCode(t *testing.T) {
	t.Parallel()

	g := newTestGuard(okVerifier(oauth.ScopeAPI))
	failure := g.check(guardRequest(t, ""))

	if failure == nil {
		t.Fatal("a request with no credential must be refused")
	}
	if failure.status != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", failure.status, http.StatusUnauthorized)
	}
	challenge := failure.header.Get("WWW-Authenticate")
	if strings.Contains(challenge, "error=") {
		t.Errorf("challenge names an error for a request that carried no credential: %q", challenge)
	}
	if !strings.Contains(challenge, `resource_metadata="`+testMetadataURL+`"`) {
		t.Errorf("challenge must point at the metadata URL, got %q", challenge)
	}
}

// TestBearerGuard_RejectedToken_IsAnsweredFromCache verifies the
// amplification defense: the second and later attempts with a token GitLab
// already refused must not reach GitLab again. Without this, a public
// deployment relays unauthenticated traffic upstream one for one.
func TestBearerGuard_RejectedToken_IsAnsweredFromCache(t *testing.T) {
	t.Parallel()

	var upstreamCalls atomic.Int32
	g := newTestGuard(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
		upstreamCalls.Add(1)
		return nil, auth.ErrInvalidToken
	})
	// A limiter generous enough that it is the cache, not the block, doing
	// the work here.
	g.limiter = serverpool.NewAuthRateLimiter(100, time.Minute)

	for range 5 {
		failure := g.check(guardRequest(t, "gloas-bad"))
		if failure == nil || failure.status != http.StatusUnauthorized {
			t.Fatalf("every attempt with a rejected token must be 401, got %+v", failure)
		}
	}

	if got := upstreamCalls.Load(); got != 1 {
		t.Errorf("upstream verification calls = %d, want 1 — the rest should come from the rejection cache", got)
	}
}

// TestBearerGuard_InvalidToken_ChallengesWithInvalidTokenCode verifies that a
// refused credential is described as such, so a client knows to reauthorize
// rather than to ask for more scope.
func TestBearerGuard_InvalidToken_ChallengesWithInvalidTokenCode(t *testing.T) {
	t.Parallel()

	g := newTestGuard(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
		return nil, auth.ErrInvalidToken
	})
	failure := g.check(guardRequest(t, "gloas-bad"))

	if failure == nil || failure.status != http.StatusUnauthorized {
		t.Fatalf("want 401, got %+v", failure)
	}
	challenge := failure.header.Get("WWW-Authenticate")
	for _, want := range []string{`error="invalid_token"`, `error_description="`, `resource_metadata="`} {
		if !strings.Contains(challenge, want) {
			t.Errorf("challenge %q is missing %s", challenge, want)
		}
	}
}

// TestBearerGuard_RepeatedFailures_BlockTheAddress verifies that the guard
// charges failures to the caller's address and stops answering once the
// budget is spent — and that a blocked caller costs nothing upstream, which
// is the point of checking the limiter first.
func TestBearerGuard_RepeatedFailures_BlockTheAddress(t *testing.T) {
	t.Parallel()

	var upstreamCalls atomic.Int32
	g := newTestGuard(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
		upstreamCalls.Add(1)
		return nil, auth.ErrInvalidToken
	})

	statuses := make([]int, 0, 6)
	for i := range 6 {
		// Distinct tokens so the rejection cache cannot absorb them; only
		// the limiter can.
		failure := g.check(guardRequest(t, "gloas-bad-"+string(rune('a'+i))))
		if failure == nil {
			t.Fatal("an invalid token must be refused")
		}
		statuses = append(statuses, failure.status)
	}

	if statuses[len(statuses)-1] != http.StatusTooManyRequests {
		t.Errorf("last status = %d, want %d once the failure budget is spent", statuses[len(statuses)-1], http.StatusTooManyRequests)
	}
	if got := upstreamCalls.Load(); got >= 6 {
		t.Errorf("upstream calls = %d; blocking must stop them reaching GitLab", got)
	}
}

// TestBearerGuard_BlockedAddress_AdvertisesRetryAfter verifies that a blocked
// caller is told when to come back rather than left to guess.
func TestBearerGuard_BlockedAddress_AdvertisesRetryAfter(t *testing.T) {
	t.Parallel()

	g := newTestGuard(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
		return nil, auth.ErrInvalidToken
	})
	for i := range 4 {
		g.check(guardRequest(t, "gloas-bad-"+string(rune('a'+i))))
	}

	failure := g.check(guardRequest(t, "gloas-anything"))
	if failure == nil || failure.status != http.StatusTooManyRequests {
		t.Fatalf("want 429, got %+v", failure)
	}
	if failure.header.Get("Retry-After") == "" {
		t.Error("a 429 must advertise Retry-After")
	}
}

// TestBearerGuard_UpstreamFailure_IsNotBlamedOnTheToken verifies the
// classification that keeps a GitLab outage from looking like a credential
// problem: 503 rather than 401, GitLab's own Retry-After when it gave one,
// no WWW-Authenticate to send the client back through authorization, no
// entry in the rejection cache, and nothing charged to the limiter.
func TestBearerGuard_UpstreamFailure_IsNotBlamedOnTheToken(t *testing.T) {
	t.Parallel()

	var upstreamCalls atomic.Int32
	g := newTestGuard(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
		upstreamCalls.Add(1)
		return nil, &oauth.UpstreamError{
			Status:     http.StatusTooManyRequests,
			RetryAfter: 17 * time.Second,
			Err:        errors.New("rate limit exceeded"),
		}
	})

	for range 3 {
		failure := g.check(guardRequest(t, "gloas-good"))
		if failure == nil || failure.status != http.StatusServiceUnavailable {
			t.Fatalf("want 503, got %+v", failure)
		}
		if got := failure.header.Get("Retry-After"); got != "17" {
			t.Errorf("Retry-After = %q, want GitLab's own %q", got, "17")
		}
		if challenge := failure.header.Get("WWW-Authenticate"); challenge != "" {
			t.Errorf("an upstream failure must not challenge the client to reauthorize, got %q", challenge)
		}
	}

	if got := upstreamCalls.Load(); got != 3 {
		t.Errorf("upstream calls = %d, want 3 — an upstream failure must never be cached as a rejection", got)
	}
	if g.rejected.Len() != 0 {
		t.Errorf("rejection cache holds %d entries after upstream failures; it must hold none", g.rejected.Len())
	}
}

// TestBearerGuard_UpstreamFailureWithoutHint_UsesItsOwnDelay verifies that a
// 503 always carries a Retry-After, even when GitLab did not say when to
// return.
func TestBearerGuard_UpstreamFailureWithoutHint_UsesItsOwnDelay(t *testing.T) {
	t.Parallel()

	g := newTestGuard(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
		return nil, &oauth.UpstreamError{Err: errors.New("connection refused")}
	})
	failure := g.check(guardRequest(t, "gloas-good"))

	if failure == nil || failure.status != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %+v", failure)
	}
	if got := failure.header.Get("Retry-After"); got == "" || got == "0" {
		t.Errorf("Retry-After = %q, want the server's own default", got)
	}
}

// TestBearerGuard_UnclassifiedError_IsTreatedAsUpstream verifies that an
// error the verifier could not attribute — an undecodable response body, say
// — is not turned into a verdict on the credential.
func TestBearerGuard_UnclassifiedError_IsTreatedAsUpstream(t *testing.T) {
	t.Parallel()

	g := newTestGuard(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
		return nil, errors.New("decode GitLab user response: unexpected EOF")
	})
	failure := g.check(guardRequest(t, "gloas-good"))

	if failure == nil || failure.status != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %+v", failure)
	}
	if g.rejected.Contains("", "gloas-good") {
		t.Error("an unclassified failure must not be cached as a rejection")
	}
}

// TestBearerGuard_NoAPIScope_IsForbiddenNotUnauthorized verifies that a
// genuine credential carrying no GitLab API scope at all is refused with 403
// and the RFC 6750 insufficient_scope code — not 401, which would tell the
// client its token is bad, and not a limiter charge, which would let a valid
// token lock its own address out.
//
// The bar is "no API scope", not "not the deployment's scope". A read_api
// token on a deployment that writes is admitted and served a read-only
// surface; see TestBearerGuard_ReadAPIToken_IsAdmittedByAWritingDeployment.
func TestBearerGuard_NoAPIScope_IsForbiddenNotUnauthorized(t *testing.T) {
	t.Parallel()

	g := newTestGuard(okVerifier("read_user"))
	failure := g.check(guardRequest(t, "gloas-no-api"))

	if failure == nil || failure.status != http.StatusForbidden {
		t.Fatalf("want 403, got %+v", failure)
	}
	// The scope named is the MINIMUM that satisfies the request, not the
	// deployment's recommended one: RFC 6750 section 3.1 defines the
	// attribute as the scope necessary to access the resource, and naming
	// the write scope here contradicted this challenge's own
	// error_description.
	challenge := failure.header.Get("WWW-Authenticate")
	for _, want := range []string{`error="insufficient_scope"`, `scope="` + oauth.MinimumScope + `"`} {
		if !strings.Contains(challenge, want) {
			t.Errorf("challenge %q is missing %s", challenge, want)
		}
	}
	if strings.Contains(challenge, `scope="`+oauth.ScopeAPI+`"`) {
		t.Errorf("challenge %q demands the write scope for a request that only needs %s", challenge, oauth.MinimumScope)
	}
	// Six more attempts would exceed the limiter's budget of three if scope
	// failures were charged to it.
	for range 6 {
		if next := g.check(guardRequest(t, "gloas-no-api")); next == nil || next.status != http.StatusForbidden {
			t.Fatalf("a scope failure must not be rate limited, got %+v", next)
		}
	}
}

// TestBearerGuard_ReadAPIToken_IsAdmittedByAWritingDeployment pins the fix for
// the case that blocked a read-only OAuth application outright: a deployment
// serving writes advertises api, and used to refuse a read_api token at the
// door — the rejection landed on initialize, so the client could not even
// list the tools it was entitled to call.
//
// Admission now asks only for what every action needs. What the token may DO
// is settled per action, by the read-only surface the pool builds for it.
// TestBearerGuard_PreflightIsNotAnAuthenticationFailure pins that a CORS
// preflight is let past untouched.
//
// The browser strips Authorization from a preflight by definition, so
// authenticating one counted every browser's routine permission question as a
// failed authentication: the limiter's budget is ten per minute, so ten
// preflights locked that address out of the endpoint for something the user
// never did. The preflight must reach the route instead, which may serve its
// own answer.
func TestBearerGuard_PreflightIsNotAnAuthenticationFailure(t *testing.T) {
	t.Parallel()

	g := newTestGuard(okVerifier(oauth.ScopeAPI))

	for range 15 {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodOptions, "/mcp", http.NoBody)
		req.Header.Set("Origin", "https://claude.ai")
		req.Header.Set("Access-Control-Request-Method", http.MethodPost)
		if failure := g.check(req); failure != nil {
			t.Fatalf("a preflight must pass the guard untouched, got %+v", failure)
		}
	}

	// The budget is intact: a real request from the same address still works.
	if failure := g.check(guardRequest(t, "gloas-good")); failure != nil {
		t.Errorf("preflights consumed the authentication budget: %+v", failure)
	}
}

func TestBearerGuard_ReadAPIToken_IsAdmittedByAWritingDeployment(t *testing.T) {
	t.Parallel()

	g := newTestGuard(okVerifier(oauth.ScopeReadAPI))
	g.advertisedScope = oauth.ScopeAPI

	if failure := g.check(guardRequest(t, "gloas-read-only")); failure != nil {
		t.Fatalf("a read_api token must be admitted, got %+v", failure)
	}
}

// TestBearerGuard_SufficientScope_PassesThrough verifies the happy path: a
// token carrying the required scope is let through so the SDK middleware can
// publish its identity, and the deployment that requires only read_api
// accepts the api token that supersedes it.
func TestBearerGuard_SufficientScope_PassesThrough(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		required string
		granted  []string
	}{
		{"exact scope", oauth.ScopeAPI, []string{oauth.ScopeAPI}},
		{"read-only deployment with api token", oauth.ScopeReadAPI, []string{oauth.ScopeAPI, oauth.ScopeReadAPI}},
		{"read-only deployment with read_api token", oauth.ScopeReadAPI, []string{oauth.ScopeReadAPI}},
		// The case this whole split exists for: a writing deployment
		// admitting a read_api token, which it used to answer 403 at
		// initialize. What it may then DO is settled per action.
		{"writing deployment with read_api token", oauth.MinimumScope, []string{oauth.ScopeReadAPI}},
		{"no scope required", "", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := newTestGuard(okVerifier(tt.granted...))
			g.minimumScope = tt.required
			if failure := g.check(guardRequest(t, "gloas-good")); failure != nil {
				t.Errorf("request should pass, got %+v", failure)
			}
		})
	}
}

// TestBearerGuard_Middleware_WritesJSONRPCAndStopsTheChain verifies that a
// rejection reaches the client in the same JSON-RPC shape the rest of this
// endpoint uses — the SDK's own middleware answers in plain text — and that
// the wrapped handler never runs.
func TestBearerGuard_Middleware_WritesJSONRPCAndStopsTheChain(t *testing.T) {
	t.Parallel()

	var reached atomic.Bool
	g := newTestGuard(okVerifier(oauth.ScopeAPI))
	handler := g.middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		reached.Store(true)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, guardRequest(t, ""))

	if reached.Load() {
		t.Error("the wrapped handler ran despite a failed authentication")
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	var body jsonRPCError
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("response is not a JSON-RPC error: %v", err)
	}
	if body.Error.Code != errCodeUnauthorized {
		t.Errorf("error code = %d, want %d", body.Error.Code, errCodeUnauthorized)
	}
}

// TestBearerGuard_Middleware_AuthenticatedRequestContinues verifies the other
// half: a request the guard accepts reaches the handler behind it.
func TestBearerGuard_Middleware_AuthenticatedRequestContinues(t *testing.T) {
	t.Parallel()

	var reached atomic.Bool
	g := newTestGuard(okVerifier(oauth.ScopeAPI))
	handler := g.middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached.Store(true)
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, guardRequest(t, "gloas-good"))

	if !reached.Load() {
		t.Error("an authenticated request must reach the wrapped handler")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// TestQuotedStringEscape_EscapesQuotesAndBackslashes verifies that a value
// placed inside an RFC 9110 quoted-string cannot terminate it early, which
// would produce a WWW-Authenticate header a client cannot parse.
func TestQuotedStringEscape_EscapesQuotesAndBackslashes(t *testing.T) {
	t.Parallel()

	tests := []struct{ in, want string }{
		{"plain", "plain"},
		{`with "quotes"`, `with \"quotes\"`},
		{`back\slash`, `back\\slash`},
		{"", ""},
	}
	for _, tt := range tests {
		if got := quotedStringEscape(tt.in); got != tt.want {
			t.Errorf("quotedStringEscape(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestBearerGuard_Challenge_IgnoresAnOddTrailingKey verifies that a caller
// passing an unpaired parameter gets a well-formed header rather than a
// half-written one.
func TestBearerGuard_Challenge_IgnoresAnOddTrailingKey(t *testing.T) {
	t.Parallel()

	g := newTestGuard(okVerifier(oauth.ScopeAPI))
	got := g.challenge("error", "invalid_token", "orphan")

	if strings.Contains(got, "orphan") {
		t.Errorf("challenge %q emitted an unpaired parameter", got)
	}
	if !strings.Contains(got, `error="invalid_token"`) {
		t.Errorf("challenge %q dropped the complete pair", got)
	}
}

// TestBearerGuard_UnacceptedRecipient_TellsTheTruthAndSpendsNoBudget pins the
// answer to a token the instance accepts but --oauth-client-uid does not admit.
//
// Both halves were wrong when the pin shipped, because the refusal wrapped
// auth.ErrInvalidToken and became indistinguishable from GitLab's own verdict:
//
//   - The holder of a genuine, unexpired, instance-valid credential was told
//     "the access token is expired, revoked, or not valid for this GitLab
//     instance". Every clause of that is false, and a client acting on it
//     reauthorizes and comes back with the same token.
//   - It charged the authentication-failure budget, so a handful of attempts
//     locked the address out of the endpoint — including for tokens the
//     deployment does admit. That is the failure mode ErrInsufficientScope's
//     own doc comment refuses to accept, and it applies here for the same
//     reason: this is not a guess at a secret.
//
// The status and RFC 6750 code stay 401 and invalid_token; section 3.1 covers a
// token "invalid for other reasons". Only the words had to change.
func TestBearerGuard_UnacceptedRecipient_TellsTheTruthAndSpendsNoBudget(t *testing.T) {
	t.Parallel()

	g := newTestGuard(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
		return nil, fmt.Errorf("token was issued to another OAuth application: %w", oauth.ErrUnacceptedRecipient)
	})

	failure := g.check(guardRequest(t, "gloas-other-app"))
	if failure == nil || failure.status != http.StatusUnauthorized {
		t.Fatalf("want 401, got %+v", failure)
	}

	challenge := failure.header.Get("WWW-Authenticate")
	if !strings.Contains(challenge, `error="invalid_token"`) {
		t.Errorf("challenge %q must keep the RFC 6750 code for a token invalid for other reasons", challenge)
	}
	if strings.Contains(challenge, "expired, revoked") {
		t.Errorf("challenge %q still claims GitLab rejected the token; it did not", challenge)
	}
	if !strings.Contains(challenge, "OAuth application") {
		t.Errorf("challenge %q does not name the actual cause", challenge)
	}
	if !strings.Contains(failure.message, "not issued to an OAuth application") {
		t.Errorf("message %q does not tell the holder what to do differently", failure.message)
	}

	// The budget is the half that hurts a bystander: these attempts must not
	// push the address towards a lockout that also refuses admitted tokens.
	for range 10 {
		if got := g.check(guardRequest(t, "gloas-other-app-again")); got == nil {
			t.Fatal("an unadmitted token must keep being refused")
		} else if got.status == http.StatusTooManyRequests {
			t.Fatal("refusing an unadmitted recipient charged the failure budget; a client holding a good token can lock out its own address")
		}
	}
}
