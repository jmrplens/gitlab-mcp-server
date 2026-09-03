// bearerguard_test.go verifies the OAuth pre-authentication guard: what it
// costs upstream, what it charges to the rate limiter, and what it tells the
// client in the RFC 6750 challenge.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
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
		t.Run(want, func(t *testing.T) {
			t.Parallel()
			if !strings.Contains(challenge, want) {
				t.Errorf("challenge %q is missing %s", challenge, want)
			}
		})
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
		t.Run(want, func(t *testing.T) {
			t.Parallel()
			if !strings.Contains(challenge, want) {
				t.Errorf("challenge %q is missing %s", challenge, want)
			}
		})
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

	tests := []struct{ name, in, want string }{
		{"no_special_characters", "plain", "plain"},
		{"double_quotes", `with "quotes"`, `with \"quotes\"`},
		{"backslash", `back\slash`, `back\\slash`},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := quotedStringEscape(tt.in); got != tt.want {
				t.Errorf("quotedStringEscape(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
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

// TestBearerGuard_RecipientRefusals covers the two ways the --oauth-client-uid
// pin can refuse, which must not be answered the same way.
//
// Both were wrong when the pin shipped, because every refusal wrapped
// auth.ErrInvalidToken and became indistinguishable from GitLab's own verdict:
//
//   - A genuine, unexpired, instance-valid credential belonging to another
//     application was told "the access token is expired, revoked, or not valid
//     for this GitLab instance". Every clause of that is false, and a client
//     acting on it reauthorizes and returns with the same token. It also charged
//     the authentication-failure budget, so a handful of attempts locked the
//     address out of the endpoint, including for tokens the deployment admits.
//   - An introspection that never answered was reported as the same verdict and
//     cached for the whole TTL, so a transient upstream outage rejected a
//     perfectly admissible token and told its holder it belonged to somebody
//     else.
//
// The first is a verdict on the token: 401 with invalid_token, which RFC 6750
// section 3.1 gives to a token "invalid for other reasons". The second is a
// failed check: 503 with Retry-After, uncached, because RejectedTokens is
// documented for definitive rejections only. Neither charges the budget.
func TestBearerGuard_RecipientRefusals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantInBody string
		// wantChallenge is checked only for the 401; a 503 carries no
		// WWW-Authenticate because the credential was never judged.
		wantChallenge string
		notInResponse string
	}{
		{
			name:          "another application is a verdict on the token",
			err:           fmt.Errorf("token was issued to another OAuth application: %w", oauth.ErrUnacceptedRecipient),
			wantStatus:    http.StatusUnauthorized,
			wantInBody:    "not issued to an OAuth application",
			wantChallenge: `error="invalid_token"`,
			notInResponse: "expired, revoked",
		},
		{
			name:          "an unanswered introspection is an upstream failure",
			err:           fmt.Errorf("introspection did not answer: %w", oauth.ErrRecipientUnverifiable),
			wantStatus:    http.StatusServiceUnavailable,
			wantInBody:    "has not been rejected",
			notInResponse: "issued to another",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			g := newTestGuard(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
				return nil, tt.err
			})

			failure := g.check(guardRequest(t, "gloas-probe"))
			if failure == nil || failure.status != tt.wantStatus {
				t.Fatalf("want %d, got %+v", tt.wantStatus, failure)
			}
			assertRefusalWording(t, failure, tt.wantInBody, tt.notInResponse)
			assertRefusalChallenge(t, failure, tt.wantChallenge, tt.notInResponse)
			if tt.wantStatus == http.StatusServiceUnavailable && failure.header.Get(headerRetryAfter) == "" {
				t.Error("a 503 must advertise Retry-After, or the client has no idea when to come back")
			}
			assertSpendsNoBudget(t, g)
		})
	}
}

// assertRefusalWording checks that a refusal says what is true of it and does
// not say what is true of a different one.
func assertRefusalWording(t *testing.T, failure *gateFailure, want, unwanted string) {
	t.Helper()
	if !strings.Contains(failure.message, want) {
		t.Errorf("message %q does not tell the holder what is actually true", failure.message)
	}
	if unwanted != "" && strings.Contains(failure.message, unwanted) {
		t.Errorf("message %q says something untrue about this refusal", failure.message)
	}
}

// assertRefusalChallenge checks the WWW-Authenticate value, where one belongs.
// A 503 carries none: the credential was never judged.
func assertRefusalChallenge(t *testing.T, failure *gateFailure, want, unwanted string) {
	t.Helper()
	if want == "" {
		return
	}
	challenge := failure.header.Get("WWW-Authenticate")
	if !strings.Contains(challenge, want) {
		t.Errorf("challenge %q is missing %s", challenge, want)
	}
	if unwanted != "" && strings.Contains(challenge, unwanted) {
		t.Errorf("challenge %q claims GitLab rejected the token; it did not", challenge)
	}
}

// assertSpendsNoBudget checks that repeating a refusal does not push the
// address towards a lockout. A client holding a token this deployment does
// admit would be caught by that lockout, which is the whole objection.
func assertSpendsNoBudget(t *testing.T, g *bearerGuard) {
	t.Helper()
	for range 10 {
		got := g.check(guardRequest(t, "gloas-probe-repeat"))
		if got == nil {
			t.Fatal("the refusal must be repeatable")
		}
		if got.status == http.StatusTooManyRequests {
			t.Fatal("this refusal charged the authentication-failure budget")
		}
	}
}

// TestBearerGuard_UnpublishedInstance_IsForbiddenAndNotChargedToTheLimiter
// covers a request that names an instance this deployment does not serve.
//
// It is a 403 about the instance rather than a 401 about the credential,
// because the caller misaddressed the request and nothing was learned about the
// token. It is also not charged to the per-address budget: charging it would
// let a client with a perfectly good token lock its own address out by
// mistyping a hostname ten times.
func TestBearerGuard_UnpublishedInstance_IsForbiddenAndNotChargedToTheLimiter(t *testing.T) {
	t.Parallel()

	g := newTestGuard(okVerifier(oauth.ScopeAPI))
	g.resolveInstance = func(*http.Request) (string, error) {
		return "", &serverpool.DisallowedGitLabURLError{Allowed: []string{"https://gitlab.com"}}
	}

	failure := g.check(guardRequest(t, "gloas-valid"))

	if failure == nil || failure.status != http.StatusForbidden {
		t.Fatalf("failure = %+v, want a 403 naming the instance", failure)
	}
	if !strings.Contains(failure.message, "GITLAB-URL") {
		t.Errorf("message = %q, want it to name the header that selected the instance", failure.message)
	}
	// The limiter's budget is three; ten more misaddressed requests must all
	// come back as the same 403 rather than turning into a 429.
	for range 10 {
		if next := g.check(guardRequest(t, "gloas-valid")); next == nil || next.status != http.StatusForbidden {
			t.Fatalf("a misaddressed request was rate limited: %+v", next)
		}
	}
}

// TestBearerGuard_GitLabReportsAnInsufficientScope_IsForbiddenAndNotCached
// covers the scope verdict that comes back from GitLab rather than from the
// token's own scope list.
//
// The token is valid, so the client is told to ask for the named scope rather
// than to discard a working credential, and neither the address budget nor the
// negative cache is charged: caching it would keep refusing that token for the
// whole TTL after the user granted the missing scope.
func TestBearerGuard_GitLabReportsAnInsufficientScope_IsForbiddenAndNotCached(t *testing.T) {
	t.Parallel()

	g := newTestGuard(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
		return nil, oauth.ErrInsufficientScope
	})

	failure := g.check(guardRequest(t, "gloas-narrow"))

	if failure == nil || failure.status != http.StatusForbidden {
		t.Fatalf("failure = %+v, want a 403 about the scope", failure)
	}
	challenge := failure.header.Get(headerWWWAuthenticate)
	for _, want := range []string{`error="insufficient_scope"`, oauth.MinimumScope} {
		t.Run(want, func(t *testing.T) {
			t.Parallel()

			if !strings.Contains(challenge, want) {
				t.Errorf("challenge %q is missing %s", challenge, want)
			}
		})
	}
	if g.rejected.Contains("", "gloas-narrow") {
		t.Error("a scope refusal was cached; the token would keep being refused after the user granted the scope")
	}
}

// newProxiedGuard returns a guard wired the way a deployment behind a reverse
// proxy is: the caller's key comes from a trusted header, which is what makes
// the coarse transport budget necessary in the first place.
//
// The budgets carry their production sizes rather than the small ones the rest
// of this file uses, because the defect these tests pin is entirely about the
// ratio between them.
func newProxiedGuard(verify auth.TokenVerifier) *bearerGuard {
	g := newTestGuard(verify)
	g.limiter = serverpool.NewAuthRateLimiter(authFailureLimit, authFailureWindow)
	g.sourceBudget = newTransportBudget(serverpool.NewAuthRateLimiter(transportFailureLimit, authFailureWindow))
	g.trustedProxyHeader = "X-Forwarded-For"
	return g
}

// proxiedRequest builds a POST that reached the server through one proxy,
// carrying client as the forwarded address and token as the bearer.
func proxiedRequest(t *testing.T, client, token string) *http.Request {
	t.Helper()

	r := guardRequest(t, token)
	r.RemoteAddr = "203.0.113.7:44444"
	r.Header.Set("X-Forwarded-For", client)
	return r
}

// TestBearerGuard_TrustedProxy_TheFleetBudgetCountsClientsNotFailures is the
// oauth half of the fleet lockout the gate already answers.
//
// The guard runs in front of the gate, and consults the budget before it even
// extracts the token, so a budget the gate charges correctly is still spent
// here first. Behind a genuine proxy the transport source is the proxy for
// every client, so charging it once per failure aggregates the fleet: fifty
// clients failing their own ten times each spend the five hundred between
// them, and the next request through that proxy is refused whatever it
// carries, valid tokens and never-failing clients included.
func TestBearerGuard_TrustedProxy_TheFleetBudgetCountsClientsNotFailures(t *testing.T) {
	t.Parallel()

	g := newProxiedGuard(okVerifier(oauth.ScopeAPI))

	// Every client stays inside its own ten-a-minute allowance, so none of
	// this is abuse the primary limiter would catch. It is one bad afternoon
	// for a fleet of fifty behind one proxy.
	clients := transportFailureLimit / authFailureLimit
	for client := range clients {
		address := "198.51.100." + strconv.Itoa(client+1)
		for range authFailureLimit {
			failure := g.check(proxiedRequest(t, address, ""))
			if failure == nil || failure.status != http.StatusUnauthorized {
				t.Fatalf("client %s got %+v for a credential-less request, want 401", address, failure)
			}
		}
	}

	if failure := g.check(proxiedRequest(t, "198.51.100.251", "gloas-valid")); failure != nil {
		t.Errorf("a client presenting a valid token through the same proxy was refused %+v; %d failures spread over %d clients locked the fleet out",
			failure, transportFailureLimit, clients)
	}
}

// TestBearerGuard_SpoofedProxyHeaderRotation_StaysBounded is the property the
// test above must not be fixed by discarding.
//
// The trusted header is caller-controlled the moment the server is reachable
// other than through the proxy. An attacker rotates it, every request mints a
// distinct primary key, the ten-a-minute lockout never fires, and each invalid
// token is relayed one to one to GitLab as a /user verification. Counting
// distinct keys per transport source is what separates that from the fleet
// above, where the keys are few and the failures many.
func TestBearerGuard_SpoofedProxyHeaderRotation_StaysBounded(t *testing.T) {
	t.Parallel()

	g := newProxiedGuard(okVerifier(oauth.ScopeAPI))

	blocked := false
	for i := range transportFailureLimit + 1 {
		failure := g.check(proxiedRequest(t, "198.51.100."+strconv.Itoa(i%250+1)+":"+strconv.Itoa(i), ""))
		if failure != nil && failure.status == http.StatusTooManyRequests {
			blocked = true
			break
		}
	}

	if !blocked {
		t.Errorf("a caller rotating the trusted header was still unblocked after %d requests", transportFailureLimit+1)
	}
}

// TestBearerGuard_BlockedByTheFleetBudget_NamesTheTransportSource verifies the
// 429 log line identifies whoever exhausted the budget.
//
// The refusal is charged to the transport source, so the caller in the line is
// very often not the cause: behind a proxy it is whichever client happened to
// arrive next. An operator reading "too many authentication failures" against
// an innocent forwarded address has been pointed at the wrong machine, and the
// address that actually matters, the one no header can change, was absent.
//
// Not parallel: it replaces the process-wide default logger.
func TestBearerGuard_BlockedByTheFleetBudget_NamesTheTransportSource(t *testing.T) {
	var logged bytes.Buffer
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, nil)))

	g := newProxiedGuard(okVerifier(oauth.ScopeAPI))
	// A tiny fleet budget, so the lockout is reached without five hundred
	// requests. The accounting under test is which address the line names.
	g.sourceBudget = newTransportBudget(serverpool.NewAuthRateLimiter(1, authFailureWindow))

	g.check(proxiedRequest(t, "198.51.100.1", ""))
	failure := g.check(proxiedRequest(t, "198.51.100.2", ""))

	if failure == nil || failure.status != http.StatusTooManyRequests {
		t.Fatalf("failure = %+v, want 429 once the fleet budget is spent", failure)
	}
	line := logged.String()
	if !strings.Contains(line, "203.0.113.7") {
		t.Errorf("the 429 line does not name the transport source that spent the budget: %s", line)
	}
}
