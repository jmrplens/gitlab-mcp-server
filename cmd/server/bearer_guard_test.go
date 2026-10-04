// bearer_guard_test.go verifies the OAuth pre-authentication guard: what it
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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/mcpotel"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/oauth"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/serverpool"
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

// TestBearerGuard_BlockedAddress_StillServesATokenAlreadyVerified pins the
// oauth-mode half of what an address block refuses: an authentication, not an
// address.
//
// The budget is keyed on the address, and behind a NAT, a campus, a carrier or
// a proxy without --trusted-proxy-header one address is many people. Consulting
// the block before the credential was read therefore answered 429 to a caller
// whose token this deployment had verified minutes earlier and was holding in
// its token cache, because somebody sharing their address was spraying.
//
// The exemption is exactly that cache, so it is worth nothing to the sprayer:
// every token the cache does not hold stays refused, and the refusal still
// costs no upstream call, which is what the budget exists to bound. The stub
// verifier reads the same cache first, as the real one does, so "no upstream
// call" is measured rather than asserted.
func TestBearerGuard_BlockedAddress_StillServesATokenAlreadyVerified(t *testing.T) {
	t.Parallel()

	const verifiedToken = "gloas-neighbor"
	cached := map[string]*auth.TokenInfo{
		verifiedToken: {UserID: "7", Scopes: []string{oauth.ScopeAPI}, Expiration: time.Now().Add(time.Hour)},
	}

	var upstreamCalls atomic.Int32
	g := newTestGuard(func(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		if info, ok := cached[token]; ok {
			return info, nil
		}
		upstreamCalls.Add(1)
		return nil, auth.ErrInvalidToken
	})
	g.verified = func(_, token string) (*auth.TokenInfo, bool) {
		info, ok := cached[token]
		return info, ok
	}

	// The neighbor is served before the spray begins, which is what puts its
	// identity in the cache in the first place.
	if failure := g.check(guardRequest(t, verifiedToken)); failure != nil {
		t.Fatalf("the neighbor's first request was refused: %+v", failure)
	}

	// The sprayer spends the shared address's budget. Distinct tokens, so the
	// rejection cache cannot absorb them and only the limiter can.
	for i := range 4 {
		if g.check(guardRequest(t, "gloas-invented-"+string(rune('a'+i)))) == nil {
			t.Fatalf("spray attempt %d was admitted", i)
		}
	}

	spent := upstreamCalls.Load()

	blocked := g.check(guardRequest(t, "gloas-invented-past-the-budget"))
	if blocked == nil || blocked.status != http.StatusTooManyRequests {
		t.Fatalf("an unverified token from a blocked address = %+v, want 429 — the budget no longer bounds the spray", blocked)
	}
	if got := upstreamCalls.Load(); got != spent {
		t.Errorf("the blocked request cost %d upstream call(s); a block must cost nothing", got-spent)
	}

	if failure := g.check(guardRequest(t, verifiedToken)); failure != nil {
		t.Fatalf("a token this deployment had already verified was refused for its neighbor's spending: %+v", failure)
	}
	if got := upstreamCalls.Load(); got != spent {
		t.Errorf("serving the verified token cost %d upstream call(s); it is a cache hit, not a verification", got-spent)
	}
}

// TestBearerGuard_BlockedAddress_RefusesAVerifiedTokenThatIsUnderScoped keeps
// the exemption to what it claims to be: a credential this deployment is
// already serving.
//
// A token the instance vouches for but that carries too little scope is
// answered 403 whenever the address is clear, so it is not one this deployment
// serves, and a block must go on covering it. Reading the cache alone would
// have admitted it, since the cache holds every identity GitLab returned.
func TestBearerGuard_BlockedAddress_RefusesAVerifiedTokenThatIsUnderScoped(t *testing.T) {
	t.Parallel()

	const underScoped = "gloas-under-scoped"
	cached := map[string]*auth.TokenInfo{
		underScoped: {UserID: "7", Scopes: []string{"read_user"}, Expiration: time.Now().Add(time.Hour)},
	}

	g := newTestGuard(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
		return nil, auth.ErrInvalidToken
	})
	g.verified = func(_, token string) (*auth.TokenInfo, bool) {
		info, ok := cached[token]
		return info, ok
	}

	for i := range 4 {
		g.check(guardRequest(t, "gloas-invented-"+string(rune('a'+i))))
	}

	failure := g.check(guardRequest(t, underScoped))
	if failure == nil || failure.status != http.StatusTooManyRequests {
		t.Fatalf("an under-scoped token = %+v, want 429: the block covers every credential this deployment does not serve", failure)
	}
}

// TestBearerGuard_BlockedAddress_StillServesAVerifiedReadAPIToken holds the
// exemption to the scope the door admits rather than the one the deployment
// advertises. A deployment serving writes advertises api and still serves a
// read_api token, on the read-only surface (ADR-0018), so such a token is one
// it is already serving and a neighbor's spray must not take it away. Judged
// on the advertised scope instead, the block would cover every read-only
// credential the deployment had admitted.
func TestBearerGuard_BlockedAddress_StillServesAVerifiedReadAPIToken(t *testing.T) {
	t.Parallel()

	const readOnly = "gloas-read-only"
	cached := map[string]*auth.TokenInfo{
		readOnly: {UserID: "7", Scopes: []string{oauth.ScopeReadAPI}, Expiration: time.Now().Add(time.Hour)},
	}
	g := newTestGuard(func(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		if info, ok := cached[token]; ok {
			return info, nil
		}
		return nil, auth.ErrInvalidToken
	})
	g.verified = func(_, token string) (*auth.TokenInfo, bool) {
		info, ok := cached[token]
		return info, ok
	}
	if g.advertisedScope == g.minimumScope {
		t.Fatalf("the fixture advertises %q, the scope it admits at; it must advertise more for this to tell them apart", g.advertisedScope)
	}

	for i := range 4 {
		g.check(guardRequest(t, "gloas-invented-"+string(rune('a'+i))))
	}
	if blocked := g.check(guardRequest(t, "gloas-invented-past-the-budget")); blocked == nil || blocked.status != http.StatusTooManyRequests {
		t.Fatalf("the address is not blocked (%+v), so this test would show nothing", blocked)
	}

	if failure := g.check(guardRequest(t, readOnly)); failure != nil {
		t.Errorf("a verified read_api token at a blocked address = %+v, want it served: a writing deployment serves read_api tokens", failure)
	}
}

// TestBearerGuard_BlockedAddress_RefusesARequestCarryingNoToken keeps the
// exemption to requests that actually present a credential.
//
// A request with no Authorization header has nothing for the cache to be
// keyed on, so the lookup must not be attempted at all: the key is a hash of
// the instance and the token, and hashing nothing produces a perfectly valid
// key that some entry could one day sit under. An exemption granted there
// would be granted to every anonymous request from the blocked address, which
// is the one thing the budget has to go on bounding.
//
// The stub cache answers a hit for anything it is asked about, the empty token
// included, so what the refusal rests on is the guard and not the accident that
// a real cache holds no entry under an empty key. It also records having been
// asked, which is the same claim stated directly: a request with no credential
// is refused before the lookup, not by it.
func TestBearerGuard_BlockedAddress_RefusesARequestCarryingNoToken(t *testing.T) {
	t.Parallel()

	var askedAboutNothing atomic.Bool
	g := newTestGuard(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
		return nil, auth.ErrInvalidToken
	})
	g.verified = func(_, token string) (*auth.TokenInfo, bool) {
		if token == "" {
			askedAboutNothing.Store(true)
		}
		return &auth.TokenInfo{UserID: "7", Scopes: []string{oauth.ScopeAPI}, Expiration: time.Now().Add(time.Hour)}, true
	}

	for i := range 4 {
		if g.check(guardRequest(t, "gloas-invented-"+string(rune('a'+i)))) == nil {
			t.Fatalf("spray attempt %d was admitted", i)
		}
	}

	failure := g.check(guardRequest(t, ""))
	if failure == nil || failure.status != http.StatusTooManyRequests {
		t.Fatalf("a request carrying no credential at a blocked address = %+v, want 429", failure)
	}
	if askedAboutNothing.Load() {
		t.Error("the verified-token cache was asked about an empty token; there is no credential there to recognize")
	}
}

// TestBearerGuard_BlockedAddress_ExemptionIsScopedToTheInstanceSelected pins
// what the oauth-mode exemption is keyed on.
//
// The verified-token cache is keyed by instance and token together, because a
// token means nothing away from the GitLab that issued it, so the exemption has
// to resolve the instance this request selected and ask about that pair. Three
// consequences, and each of them is a way the exemption could be too generous:
// a token cached against the instance the request selected is served; the same
// deployment's cache entry for another published instance does not serve it;
// and a request the resolver refuses an instance for is not exempted at all,
// however much the cache would have answered for the empty instance the lookup
// would otherwise have fallen back on.
//
// The last is the one that reads as pedantic and is not. Falling back would
// key every misaddressed request onto one shared bucket, so a token verified
// on a deployment that publishes no instance would exempt a blocked request
// naming a host this deployment refuses.
func TestBearerGuard_BlockedAddress_ExemptionIsScopedToTheInstanceSelected(t *testing.T) {
	t.Parallel()

	const (
		selected     = "https://gitlab.example.com"
		alsoServed   = "https://gitlab.other.example"
		cachedHere   = "gloas-cached-for-the-selected-instance"
		cachedThere  = "gloas-cached-for-the-other-instance"
		neverCached  = "gloas-never-verified"
		apiScopeOnly = oauth.ScopeAPI
	)
	// Keyed the way the real cache is: instance and token together, plus one
	// entry under the empty instance, which is what a lookup that ignored a
	// resolver error would land on.
	cached := map[string]map[string]*auth.TokenInfo{
		selected:   {cachedHere: {UserID: "7", Scopes: []string{apiScopeOnly}, Expiration: time.Now().Add(time.Hour)}},
		alsoServed: {cachedThere: {UserID: "8", Scopes: []string{apiScopeOnly}, Expiration: time.Now().Add(time.Hour)}},
		"":         {cachedThere: {UserID: "9", Scopes: []string{apiScopeOnly}, Expiration: time.Now().Add(time.Hour)}},
	}

	resolvesToSelected := func(*http.Request) (string, error) { return selected, nil }

	// newBlockedGuard returns a guard whose address is already blocked, by the
	// only thing that blocks one: authentications GitLab refused. A
	// misaddressed request is deliberately not charged to the budget, so a
	// spray of those would leave the address clear and prove nothing.
	newBlockedGuard := func(t *testing.T) *bearerGuard {
		t.Helper()
		var g *bearerGuard
		// The verifier reads the same cache first, as the real one does, so an
		// exempted request is answered from memory and a cache miss is the
		// upstream call the block exists to prevent.
		g = newTestGuard(func(_ context.Context, token string, r *http.Request) (*auth.TokenInfo, error) {
			instance, _ := g.resolveInstance(r)
			if info, ok := cached[instance][token]; ok {
				return info, nil
			}
			return nil, auth.ErrInvalidToken
		})
		g.instances = []string{selected, alsoServed}
		g.resolveInstance = resolvesToSelected
		g.verified = func(instance, token string) (*auth.TokenInfo, bool) {
			info, ok := cached[instance][token]
			return info, ok
		}
		for i := range 4 {
			failure := g.check(guardRequest(t, "gloas-invented-"+string(rune('a'+i))))
			if failure == nil {
				t.Fatalf("spray attempt %d was admitted", i)
			}
		}
		return g
	}

	cases := []struct {
		name    string
		resolve func(*http.Request) (string, error)
		token   string
		want    int // 0 means the request must not be refused at all
		why     string
	}{
		{
			name:    "cached against the instance this request selected",
			resolve: resolvesToSelected,
			token:   cachedHere,
			want:    0,
			why:     "this deployment verified this token at this instance and is serving it",
		},
		{
			name:    "cached against another published instance",
			resolve: resolvesToSelected,
			token:   cachedThere,
			want:    http.StatusTooManyRequests,
			why:     "verifying it at the instance this request names would be an upstream call",
		},
		{
			name:    "never verified anywhere",
			resolve: resolvesToSelected,
			token:   neverCached,
			want:    http.StatusTooManyRequests,
			why:     "this is the whole of what a sprayer can send, and the budget must go on refusing it",
		},
		{
			name: "an instance this deployment does not publish",
			resolve: func(*http.Request) (string, error) {
				return "", &serverpool.DisallowedGitLabURLError{Allowed: []string{selected}}
			},
			token: cachedThere,
			want:  http.StatusTooManyRequests,
			why:   "the resolver names no instance, so the exemption must not fall back to the empty one",
		},
		{
			name:    "no instance selected where two are published",
			resolve: func(*http.Request) (string, error) { return "", errMissingGitLabURL },
			token:   cachedThere,
			want:    http.StatusTooManyRequests,
			why:     "the resolver refuses to choose, so the exemption has nothing to ask about",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g := newBlockedGuard(t)
			// Set after the block: this is the instance the request being
			// judged selects, not the one that spent the budget.
			g.resolveInstance = tc.resolve

			failure := g.check(guardRequest(t, tc.token))
			if tc.want == 0 {
				if failure != nil {
					t.Fatalf("= %+v, want the request served: %s", failure, tc.why)
				}
				return
			}
			if failure == nil || failure.status != tc.want {
				t.Fatalf("= %+v, want %d: %s", failure, tc.want, tc.why)
			}
		})
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

// TestBearerGuard_BlockedAddress_RetryAfterIsTheBlockThatHoldsIt checks the
// value, not the presence: Retry-After is what the block holding the request
// has left, which for the distinct-token budget is its first rung of ten
// minutes here, not the one-minute failure window every other fixture blocks
// for. A client told a minute knocks for the other nine.
func TestBearerGuard_BlockedAddress_RetryAfterIsTheBlockThatHoldsIt(t *testing.T) {
	t.Parallel()

	g := newTestGuard(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
		return nil, auth.ErrInvalidToken
	})
	g.limiter = nil
	g.spray = serverpool.NewDistinctTokenBudget(2, time.Hour, 10*time.Minute)
	g.check(guardRequest(t, "gloas-refused-a"))
	g.check(guardRequest(t, "gloas-refused-b"))

	failure := g.check(guardRequest(t, "gloas-anything"))
	if failure == nil || failure.status != http.StatusTooManyRequests {
		t.Fatalf("want 429, got %+v", failure)
	}
	assertRetryAfterWithin(t, failure.header.Get("Retry-After"), 9*time.Minute, 10*time.Minute)
}

// assertRetryAfterWithin checks that a Retry-After header is a whole number of
// seconds in (low, high]: the remaining block, rounded up, measured a moment
// after it was raised.
func assertRetryAfterWithin(t *testing.T, header string, low, high time.Duration) {
	t.Helper()
	seconds, err := strconv.Atoi(header)
	if err != nil {
		t.Fatalf("Retry-After = %q, want a number of seconds: %v", header, err)
	}
	if got := time.Duration(seconds) * time.Second; got <= low || got > high {
		t.Errorf("Retry-After = %ds, want more than %v and at most %v", seconds, low, high)
	}
}

// sourceBlockWindow is the window the refusal tables build a transport budget
// with. It differs from authFailureWindow on purpose: a source block announced
// with the failure lockout's remaining time, or the reverse, must be a
// different number for the swap to show.
const sourceBlockWindow = 5 * time.Minute

// assertRetryAfterIsTheBlock checks a Retry-After against the block that holds
// the request: none where block is zero, and otherwise what is left of block,
// rounded up.
//
// The floor is half the block rather than a second under it. What is left is
// measured when the refusal is written, and a subtest stalled for a second
// under -race would fail an exact value for being slow; half still tells the
// failure lockout (one minute) and the source block (five) apart, and both
// from the single second a zero duration renders as.
func assertRetryAfterIsTheBlock(t *testing.T, header string, block time.Duration) {
	t.Helper()
	if block == 0 {
		if header != "" {
			t.Errorf("Retry-After = %q, want none", header)
		}
		return
	}
	assertRetryAfterWithin(t, header, block/2, block)
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

// TestBearerGuard_SaturatedVerification_IsARetryThatCostsNothing pins the
// refusal of a request that waited in vain for a verification slot (ADM-014):
// 503 with Retry-After, no challenge sending the client back through
// authorization, no entry in the rejection cache, and nothing charged to any
// of the three budgets however often it happens. Its text is the one a
// verification with no verdict is always answered with, word for word, so the
// wording tells a caller nothing the refusal itself does not (INV-019). A wait
// the request's own end cut short is answered the same way.
func TestBearerGuard_SaturatedVerification_IsARetryThatCostsNothing(t *testing.T) {
	t.Parallel()

	unverdicted := newTestGuard(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
		return nil, errors.New("decode GitLab user response: unexpected EOF")
	}).check(guardRequest(t, "gloas-undecodable"))

	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "the wait ran out", err: oauth.ErrVerificationBusy},
		{name: "the request ended while it waited", err: fmt.Errorf("token verification abandoned, the request ended first: %w", context.Canceled)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g := newGuardWithEveryBudget(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
				return nil, tc.err
			})
			failure := g.check(guardRequest(t, "gloas-waiting"))
			if failure == nil || failure.status != http.StatusServiceUnavailable || failure.code != errCodeUpstreamUnavailable {
				t.Fatalf("want 503 with %d, got %+v", errCodeUpstreamUnavailable, failure)
			}
			if challenge := failure.header.Get(headerWWWAuthenticate); challenge != "" {
				t.Errorf("a saturated verifier must not challenge the client to reauthorize, got %q", challenge)
			}
			if failure.message != unverdicted.message || failure.header.Get(headerRetryAfter) != unverdicted.header.Get(headerRetryAfter) {
				t.Errorf("refusal = %q with Retry-After %q; want a verification with no verdict's own, %q with %q, so the words add nothing",
					failure.message, failure.header.Get(headerRetryAfter), unverdicted.message, unverdicted.header.Get(headerRetryAfter))
			}
			if g.rejected.Len() != 0 {
				t.Errorf("rejection cache holds %d entries; a token nobody judged must not be remembered as refused", g.rejected.Len())
			}
			assertSpendsNoBudgetOfThree(t, g)
		})
	}
}

// newGuardWithEveryBudget is [newTestGuard] with all three authentication
// budgets in place and each set to block on its first charge: the per-address
// failure budget, the transport-source budget a trusted proxy header brings,
// and the distinct-credential budget.
func newGuardWithEveryBudget(verify auth.TokenVerifier) *bearerGuard {
	g := newProxiedGuard(verify)
	g.limiter = serverpool.NewAuthRateLimiter(1, time.Minute)
	g.sourceBudget = newTransportBudget(serverpool.NewAuthRateLimiter(1, time.Minute), time.Minute)
	g.spray = serverpool.NewDistinctTokenBudget(1, time.Minute, time.Minute)
	return g
}

// assertSpendsNoBudgetOfThree repeats the refusal from five forwarded
// addresses behind one trusted proxy, twice from each and with a new token
// every time, the shape of a flood spread over many sources. Every budget of
// [newGuardWithEveryBudget] blocks on its first charge, so a charge to any of
// them would answer a later request 429: the per-address and distinct-token
// budgets the second request from the same address, the transport-source
// budget the next request through the proxy.
func assertSpendsNoBudgetOfThree(t *testing.T, g *bearerGuard) {
	t.Helper()
	for i := range 5 {
		for j := range 2 {
			got := g.check(proxiedRequest(t, "198.51.100."+strconv.Itoa(i+1), "gloas-spread-"+strconv.Itoa(i)+"-"+strconv.Itoa(j)))
			if got == nil {
				t.Fatal("the refusal must be repeatable")
			}
			if got.status == http.StatusTooManyRequests {
				t.Fatalf("request %d from address %d was answered 429: this refusal charged a budget", j, i)
			}
		}
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

// TestBearerGuard_ReadAPIToken_IsAdmittedByAWritingDeployment pins the fix for
// the case that blocked a read-only OAuth application outright: a deployment
// serving writes advertises api, and used to refuse a read_api token at the
// door: the rejection landed on initialize, so the client could not even
// list the tools it was entitled to call.
//
// Admission now asks only for what every action needs. What the token may DO
// is settled per action, by the read-only surface the pool builds for it.
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

// TestBearerGuard_UnacceptedRecipient_NamesTheDocumentationPage pins the RFC
// 6750 error_uri on the one refusal whose remedy lives on a web page.
//
// The message tells the holder to obtain a token from the application the
// operator published and to read the resource documentation for which one. It
// used to say the documentation was "named in the WWW-Authenticate challenge",
// which named only resource_metadata: a document the holder had to fetch to
// find the page. error_uri is RFC 6750's parameter for exactly this, "a URI
// identifying a human-readable web page with information about the error".
//
// Three properties are checked. The fresh refusal carries it; the refusal
// answered from the rejected-token cache carries it too, since the second
// request is the one a person retrying actually reads; and a guard given no
// page emits no error_uri rather than an empty one.
func TestBearerGuard_UnacceptedRecipient_NamesTheDocumentationPage(t *testing.T) {
	// Not parallel: the two attempts below are one sequence, and the second
	// only means something after the first has populated the cache.
	const page = "https://ops.example.com/our-oauth-app"
	var calls atomic.Int32
	g := newTestGuard(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
		calls.Add(1)
		return nil, refusedRecipient()
	})
	g.documentationURL = page

	// sequential: the second attempt is only meaningful after the first has
	// populated the rejected-token cache.
	for _, attempt := range []string{"fresh", "answered from the rejected-token cache"} {
		t.Run(attempt, func(t *testing.T) {
			assertRecipientRefusalNamesPage(t, g.check(guardRequest(t, "gloas-other-app")), page)
		})
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("verifier called %d times for two attempts; the second must be answered from the rejected-token cache", got)
	}
}

// TestBearerGuard_UnacceptedRecipient_WithoutAPageEmitsNoErrorURI is the other
// half: a guard given no documentation page emits no error_uri rather than an
// empty one, which a client would try to open.
func TestBearerGuard_UnacceptedRecipient_WithoutAPageEmitsNoErrorURI(t *testing.T) {
	t.Parallel()

	g := newTestGuard(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
		return nil, refusedRecipient()
	})
	failure := g.check(guardRequest(t, "gloas-other-app"))
	if failure == nil || failure.status != http.StatusUnauthorized {
		t.Fatalf("want 401, got %+v", failure)
	}
	if challenge := failure.header.Get(headerWWWAuthenticate); strings.Contains(challenge, "error_uri") {
		t.Errorf("challenge %q carries an error_uri with no page to point at", challenge)
	}
}

// refusedRecipient is the verifier error for a token minted for an application
// the deployment does not admit.
func refusedRecipient() error {
	return fmt.Errorf("token was issued to another OAuth application: %w", oauth.ErrUnacceptedRecipient)
}

// assertRecipientRefusalNamesPage checks the shape of the unaccepted-recipient
// 401: the RFC 6750 error code, the error_uri naming page, and a message that
// says where the page is named.
func assertRecipientRefusalNamesPage(t *testing.T, failure *gateFailure, page string) {
	t.Helper()
	if failure == nil || failure.status != http.StatusUnauthorized {
		t.Fatalf("want 401, got %+v", failure)
	}
	challenge := failure.header.Get(headerWWWAuthenticate)
	if !strings.Contains(challenge, `error_uri="`+page+`"`) {
		t.Errorf("challenge %q does not name the documentation page as error_uri", challenge)
	}
	if !strings.Contains(challenge, `error="invalid_token"`) {
		t.Errorf("challenge %q lost the RFC 6750 error code", challenge)
	}
	if !strings.Contains(failure.message, "error_uri") {
		t.Errorf("message %q does not tell the holder where the page is named", failure.message)
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

// userReadSentence is GitLab's refusal of GET /api/v4/user for a fine-grained
// token granted no User: Read, as Authz::Tokens::AuthorizeGranularScopesService
// writes it at v19.4.1-ee.
const userReadSentence = "Access denied: This operation requires a fine-grained personal access token with the following user permissions: [User: Read]."

// TestBearerGuard_FineGrainedTokenWithoutUserRead_IsForbiddenUnchargedAndRemembered
// covers a fine-grained token GitLab accepted and refused the permission to
// read its own user. It is answered 403 with the insufficient_scope challenge,
// whose description is this server's own constant, and with GitLab's sentence
// in the body; it is charged nothing, however often it comes back, since the
// token is genuine; and it is remembered, so a repeat is answered from memory
// and costs no verification, which is what keeps one such token sent often
// from holding every verification slot.
func TestBearerGuard_FineGrainedTokenWithoutUserRead_IsForbiddenUnchargedAndRemembered(t *testing.T) {
	t.Parallel()

	const challenge = `Bearer realm="gitlab-mcp-server", error="insufficient_scope", ` +
		`error_description="GitLab refused this token the permission to read its own user", scope="read_api", ` +
		`resource_metadata="` + testMetadataURL + `"`
	var verifications atomic.Int32
	g := newTestGuard(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
		verifications.Add(1)
		return nil, &oauth.PermissionMissingError{Description: userReadSentence}
	})

	// Five times the limiter's budget of three: every one a 403, none a 429.
	for i := range 15 {
		failure := g.check(guardRequest(t, "glpat-fine-grained"))
		if failure == nil || failure.status != http.StatusForbidden || failure.code != errCodeForbidden {
			t.Fatalf("request %d: failure = %+v, want the uncharged 403", i, failure)
		}
		if got := failure.header.Get(headerWWWAuthenticate); got != challenge {
			t.Errorf("request %d: WWW-Authenticate = %q\nwant %q", i, got, challenge)
		}
		if want := doorPermissionPrefix + doorPermissionAdvice + " GitLab said: " + userReadSentence; failure.message != want {
			t.Errorf("request %d: message = %q\nwant %q", i, failure.message, want)
		}
	}
	if n := verifications.Load(); n != 1 {
		t.Errorf("the verifier was asked %d times, want once: the verdict is answered from memory after that", n)
	}
	if kind, sentence, known := g.rejected.LookupRefusal("", "glpat-fine-grained"); !known ||
		kind != oauth.RejectionPermissionMissing || sentence != userReadSentence {
		t.Errorf("rejected-token cache holds %v, %q, %v; want the permission refusal with GitLab's sentence", kind, sentence, known)
	}
	assertSpendsNoBudget(t, g)
}

// TestBearerGuard_PermissionMissing_QuotesAHostileSentenceOnlyFilteredAndCut
// feeds the guard the sentence a hostile instance could write, which under
// --allow-any-gitlab-url is the caller's own: 4000 bytes carrying quotes,
// backslashes, control characters and other scripts. None of it reaches the
// challenge, whose description stays the constant, and the body quotes it only
// as printable ASCII within 512 bytes. A guard without a rejected-token cache
// answers the same.
func TestBearerGuard_PermissionMissing_QuotesAHostileSentenceOnlyFilteredAndCut(t *testing.T) {
	t.Parallel()

	hostile := `Access denied: "quoted" \ back` + "\r\n\x00é" + strings.Repeat("x", 4000)
	for name, withCache := range map[string]bool{"with a cache": true, "without a cache": false} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			g := newTestGuard(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
				return nil, &oauth.PermissionMissingError{Description: hostile}
			})
			if !withCache {
				g.rejected = nil
			}
			failure := g.check(guardRequest(t, "glpat-hostile"))
			if failure == nil || failure.status != http.StatusForbidden {
				t.Fatalf("failure = %+v, want a 403", failure)
			}
			quoted, found := strings.CutPrefix(failure.message, doorPermissionPrefix+doorPermissionAdvice+" GitLab said: ")
			if !found {
				t.Fatalf("message = %q, want the prefix, the advice and GitLab's sentence", failure.message)
			}
			if len(quoted) > 512 || strings.ContainsFunc(quoted, func(r rune) bool { return r < ' ' || r > '~' }) {
				t.Errorf("quoted sentence is %d bytes and carries %q, want printable ASCII within 512 bytes", len(quoted), quoted)
			}
			if !strings.HasPrefix(quoted, `Access denied: "quoted" \ back x`) {
				t.Errorf("quoted sentence = %q, want its printable text kept", quoted)
			}
			if challenge := failure.header.Get(headerWWWAuthenticate); strings.Contains(challenge, "quoted") || strings.Contains(challenge, "xxxx") {
				t.Errorf("the challenge %q carries the instance's sentence", challenge)
			}
		})
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
	g.sourceBudget = newTransportBudget(serverpool.NewAuthRateLimiter(transportFailureLimit, authFailureWindow), authFailureWindow)
	g.trustedProxyHeader = "X-Forwarded-For"
	g.trustedProxies = trustedProxiesOf([]string{"203.0.113.7"})
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
		// Two octets vary: the key is the address alone, a port on the hop is
		// stripped, and one octet gives fewer distinct keys than the coarse
		// budget holds.
		failure := g.check(proxiedRequest(t, "198.51."+strconv.Itoa(i/250)+"."+strconv.Itoa(i%250+1), ""))
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
	// The line is throttled per message: a sibling test that spent a budget
	// inside the last minute would otherwise have the write, and this test
	// nothing to read.
	forgetRefusalLines()

	g := newProxiedGuard(okVerifier(oauth.ScopeAPI))
	// A tiny fleet budget, so the lockout is reached without five hundred
	// requests. The accounting under test is which address the line names.
	g.sourceBudget = newTransportBudget(serverpool.NewAuthRateLimiter(1, authFailureWindow), authFailureWindow)

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

// TestBearerGuard_MultiInstanceWithoutASelection_IsRefusedBeforeVerification
// covers a deployment publishing several instances and a request naming none.
//
// The refusal is answered before the verifier runs, on purpose: verifying
// would put this bearer on the wire to an instance the caller never chose. It
// is a 400 that names the published set, which oauth mode may do because the
// RFC 9728 metadata serves the same list unauthenticated, and it is not
// charged to the limiter, since nothing was learned about the credential.
func TestBearerGuard_MultiInstanceWithoutASelection_IsRefusedBeforeVerification(t *testing.T) {
	t.Parallel()

	var verified atomic.Int64
	g := newTestGuard(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
		verified.Add(1)
		return nil, errors.New("must not be reached")
	})
	g.instances = []string{"https://gitlab.com", "https://gitlab.example.com"}
	g.resolveInstance = func(*http.Request) (string, error) { return "", errMissingGitLabURL }

	failure := g.check(guardRequest(t, "gloas-valid"))

	if failure == nil || failure.status != http.StatusBadRequest {
		t.Fatalf("failure = %+v, want a 400 asking for the GITLAB-URL header", failure)
	}
	for _, instance := range g.instances {
		if !strings.Contains(failure.message, instance) {
			t.Errorf("message = %q, want it to publish %s", failure.message, instance)
		}
	}
	if verified.Load() != 0 {
		t.Error("the verifier ran for a request that named no instance; the bearer went on the wire")
	}
	assertSpendsNoBudget(t, g)
}

// TestDescribeScopeShortfall_NamesWhatTheTokenHolds pins the wording of the
// scope refusal for each shape a token's scope list can take, because the
// reader is somebody who believes they granted the right thing and has to be
// told what they actually hold.
func TestDescribeScopeShortfall_NamesWhatTheTokenHolds(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		granted []string
		want    []string
		unwant  []string
	}{
		{name: "no scope at all", granted: nil, want: []string{"no GitLab API scope"}},
		{name: "one scope", granted: []string{"read_user"}, want: []string{"the read_user scope,"}},
		{name: "several scopes", granted: []string{"read_user", "profile"}, want: []string{"the read_user, profile scopes"}},
		{name: "GitLab's own mcp scope is explained", granted: []string{"mcp"}, want: []string{"the mcp scope", "GitLab's mcp scope is for its own MCP server"}},
		{name: "the orbit variant is explained too", granted: []string{"mcp_orbit", "profile"}, want: []string{"GitLab's mcp scope is for its own MCP server"}},
		{name: "an unrelated scope gets no mcp aside", granted: []string{"profile"}, unwant: []string{"GitLab's mcp scope"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := describeScopeShortfall(tc.granted, oauth.MinimumScope, oauth.ScopeAPI)
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("describeScopeShortfall(%v) = %q, want it to carry %q", tc.granted, got, want)
				}
			}
			for _, unwanted := range tc.unwant {
				if strings.Contains(got, unwanted) {
					t.Errorf("describeScopeShortfall(%v) = %q, must not carry %q", tc.granted, got, unwanted)
				}
			}
		})
	}
}

// TestRejectedTokenCacheBounds_StillRememberARejectionTheyJustRecorded asserts
// what the two constants the guard's cache is built from have to make true.
//
// Every other test here builds its own cache with its own bounds
// (newTestGuard passes 16 and a minute), so the pair the process actually
// wires in registerOAuthMCPHandlers is used by nothing that asserts anything:
// either of them collapsing to zero or below changes no test, while
// [oauth.NewRejectedTokens] documents that a non-positive capacity or TTL
// disables the cache outright. That is the amplification defense gone — every
// replay of a token GitLab already refused becomes another round trip to
// GitLab, which is the exact traffic the cache exists to absorb — and it
// disappears silently, because a disabled cache still answers every call and
// simply never reports a hit.
func TestRejectedTokenCacheBounds_StillRememberARejectionTheyJustRecorded(t *testing.T) {
	t.Parallel()

	const instance = "https://gitlab.example.com"

	cache := oauth.NewRejectedTokens(rejectedTokenMaxSize, rejectedTokenTTL)
	cache.Record(instance, "glpat-refused-by-gitlab")

	if !cache.Contains(instance, "glpat-refused-by-gitlab") {
		t.Errorf("a rejection recorded a moment ago is already forgotten: "+
			"rejectedTokenMaxSize=%d and rejectedTokenTTL=%s build a cache that stores nothing, "+
			"so every replay of a refused token is asked of GitLab again",
			rejectedTokenMaxSize, rejectedTokenTTL)
	}
	if got := cache.Len(); got != 1 {
		t.Errorf("cache holds %d entries after one rejection, want 1", got)
	}
}

// TestBearerGuard_ABlockedRequest_IsCountedUnderTheBudgetThatRefusedIt is the
// guard's half of the telemetry the gate is held to: each refusal is counted
// once, under the budget that made it, and under no other. The guard runs in
// front of the gate and refuses first, so a count it files wrongly is the
// count an operator reads.
//
// Every request arrives through one trusted proxy, so the client a failure is
// charged to and the source the fleet budget is charged to are different
// addresses, and a count filed under the wrong one of them is visible.
func TestBearerGuard_ABlockedRequest_IsCountedUnderTheBudgetThatRefusedIt(t *testing.T) {
	t.Parallel()

	refusing := func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
		return nil, auth.ErrInvalidToken
	}
	cases := []struct {
		name    string
		arm     func(*bearerGuard)
		spend   []gateSpend
		refused string
		want    mcpotel.AuthBlockCounts
	}{
		{
			name:    "the failure lockout",
			arm:     func(g *bearerGuard) { g.limiter = serverpool.NewAuthRateLimiter(1, authFailureWindow) },
			spend:   []gateSpend{{"198.51.100.1", ""}},
			refused: "198.51.100.1",
			want:    mcpotel.AuthBlockCounts{FailureLockout: 1},
		},
		{
			name: "the transport source",
			arm: func(g *bearerGuard) {
				g.sourceBudget = newTransportBudget(serverpool.NewAuthRateLimiter(1, authFailureWindow), authFailureWindow)
			},
			spend:   []gateSpend{{"198.51.100.1", ""}},
			refused: "198.51.100.2",
			want:    mcpotel.AuthBlockCounts{TransportSource: 1},
		},
		{
			name:    "the distinct-token budget",
			arm:     func(g *bearerGuard) { g.spray = serverpool.NewDistinctTokenBudget(2, time.Minute, time.Minute) },
			spend:   []gateSpend{{"198.51.100.1", "gloas-refused-a"}, {"198.51.100.1", "gloas-refused-b"}},
			refused: "198.51.100.1",
			want:    mcpotel.AuthBlockCounts{DistinctTokens: 1},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := newProxiedGuard(refusing)
			g.limiter = nil
			g.sourceBudget = nil
			g.blocks = &authBlockCounters{}
			tc.arm(g)

			for _, s := range tc.spend {
				if failure := g.check(proxiedRequest(t, s.client, s.token)); failure == nil || failure.status != http.StatusUnauthorized {
					t.Fatalf("spending the budget from %s got %+v, want 401", s.client, failure)
				}
			}
			if got := g.blocks.counts(); got != (mcpotel.AuthBlockCounts{}) {
				t.Fatalf("counts before any refusal = %+v, want none: a failure is not a block", got)
			}
			if failure := g.check(proxiedRequest(t, tc.refused, "")); failure == nil || failure.status != http.StatusTooManyRequests {
				t.Fatalf("the request from %s got %+v, want 429", tc.refused, failure)
			}
			if got := g.blocks.counts(); got != tc.want {
				t.Errorf("counts = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// guardRefusal is what one refusal of the guard must carry on the wire.
type guardRefusal struct {
	status int
	code   int
	// challenge is the exact WWW-Authenticate value, or "" where the refusal
	// must carry none because the credential was never judged.
	challenge string
	// retryAfter is the exact Retry-After value, or "" where there is none or
	// where blockedFor holds it instead.
	retryAfter string
	// blockedFor is the length of the block that refused the request, for a
	// refusal whose Retry-After is what is left of that block and so depends
	// on when it was measured; see [assertRetryAfterIsTheBlock].
	blockedFor time.Duration
	// says are fragments the message must carry.
	says []string
}

// TestBearerGuard_EachRefusal_CarriesItsOwnStatusCodeAndChallenge holds every
// refusal the guard makes to the whole of what a client reads from it.
//
// The refusals differ in exactly the parts a client acts on: the status and the
// code decide whether to reauthorize, ask for more scope, wait or give up; the
// challenge names the scope to ask for and the RFC 6750 verdict; Retry-After
// says how long to wait. Asserting only the status, as the tests beside this one
// do, lets any of the rest be swapped with a neighbor's and nobody notices.
// The challenges are written out in full rather than built with the functions
// under test, so a parameter that moved, vanished or named the wrong scope is a
// difference in the string.
func TestBearerGuard_EachRefusal_CarriesItsOwnStatusCodeAndChallenge(t *testing.T) {
	t.Parallel()

	const (
		metadata     = `, resource_metadata="` + testMetadataURL + `"`
		advertised   = `Bearer realm="gitlab-mcp-server"`
		invalid      = advertised + `, error="invalid_token", error_description="the access token is expired, revoked, or not valid for this GitLab instance", scope="api"` + metadata
		insufficient = advertised + `, error="insufficient_scope", error_description="the token lacks the read_api scope", scope="read_api"` + metadata
	)
	upstreamDefault := strconv.Itoa(int(upstreamRetryAfter.Seconds()))
	failing := func(err error) auth.TokenVerifier {
		return func(context.Context, string, *http.Request) (*auth.TokenInfo, error) { return nil, err }
	}
	unpublished := &serverpool.DisallowedGitLabURLError{Allowed: []string{"https://gitlab.com"}}

	cases := []struct {
		name  string
		guard func() *bearerGuard
		token string
		want  guardRefusal
	}{
		{
			name:  "no credential",
			guard: func() *bearerGuard { return newTestGuard(okVerifier(oauth.ScopeAPI)) },
			want: guardRefusal{
				status: http.StatusUnauthorized, code: errCodeUnauthorized,
				challenge: advertised + `, scope="api"` + metadata,
				says:      []string{oauthMissingTokenMessage},
			},
		},
		{
			name: "a blocked address",
			guard: func() *bearerGuard {
				g := newTestGuard(okVerifier(oauth.ScopeAPI))
				g.limiter = serverpool.NewAuthRateLimiter(1, authFailureWindow)
				g.check(guardRequest(t, ""))
				return g
			},
			token: "gloas-anything",
			want:  guardRefusal{status: http.StatusTooManyRequests, code: errCodeTooManyRequests, blockedFor: authFailureWindow, says: []string{"Too many failed authentication attempts"}},
		},
		{
			// The failure limiter stays armed and unspent, so the one budget
			// holding the request is the source's, and a Retry-After taken
			// from the limiter's side reads as a second.
			name: "a blocked transport source",
			guard: func() *bearerGuard {
				g := newTestGuard(okVerifier(oauth.ScopeAPI))
				g.sourceBudget = newTransportBudget(serverpool.NewAuthRateLimiter(1, sourceBlockWindow), sourceBlockWindow)
				g.sourceBudget.charge("192.0.2.10", "198.51.100.1")
				return g
			},
			token: "gloas-anything",
			want:  guardRefusal{status: http.StatusTooManyRequests, code: errCodeTooManyRequests, blockedFor: sourceBlockWindow, says: []string{"Too many failed authentication attempts"}},
		},
		{
			name: "no instance selected where several are published",
			guard: func() *bearerGuard {
				g := newTestGuard(okVerifier(oauth.ScopeAPI))
				g.instances = []string{"https://a.example.com", "https://b.example.com"}
				g.resolveInstance = func(*http.Request) (string, error) { return "", errMissingGitLabURL }
				return g
			},
			token: "gloas-valid",
			want:  guardRefusal{status: http.StatusBadRequest, code: errCodeInvalidRequest, says: []string{"https://a.example.com, https://b.example.com"}},
		},
		{
			name: "an instance this deployment does not publish",
			guard: func() *bearerGuard {
				g := newTestGuard(okVerifier(oauth.ScopeAPI))
				g.resolveInstance = func(*http.Request) (string, error) { return "", unpublished }
				return g
			},
			token: "gloas-valid",
			want:  guardRefusal{status: http.StatusForbidden, code: errCodeForbidden, says: []string{unpublished.Error()}},
		},
		{
			name:  "a token carrying no API scope",
			guard: func() *bearerGuard { return newTestGuard(okVerifier("read_user")) },
			token: "gloas-narrow",
			want: guardRefusal{
				status: http.StatusForbidden, code: errCodeForbidden, challenge: insufficient,
				says: []string{"read_api is the least", "granting api for the full tool surface or read_api for a read-only one"},
			},
		},
		{
			name:  "GitLab reporting an insufficient scope",
			guard: func() *bearerGuard { return newTestGuard(failing(oauth.ErrInsufficientScope)) },
			token: "gloas-narrow",
			want: guardRefusal{
				status: http.StatusForbidden, code: errCodeForbidden, challenge: insufficient,
				says: []string{"Reauthorize granting api for the full tool surface, or read_api for a read-only one"},
			},
		},
		{
			name:  "GitLab refusing the token",
			guard: func() *bearerGuard { return newTestGuard(failing(auth.ErrInvalidToken)) },
			token: "gloas-refused",
			want:  guardRefusal{status: http.StatusUnauthorized, code: errCodeUnauthorized, challenge: invalid, says: []string{"GitLab rejected this token"}},
		},
		{
			name:  "a token issued to an application this deployment does not admit",
			guard: func() *bearerGuard { return newTestGuard(failing(refusedRecipient())) },
			token: "gloas-other-app",
			want: guardRefusal{
				status: http.StatusUnauthorized, code: errCodeUnauthorized,
				challenge: advertised + `, error="invalid_token", error_description="the token was not issued to an OAuth application this deployment admits", scope="api"` + metadata,
				says:      []string{"not issued to an OAuth application this deployment admits"},
			},
		},
		{
			name: "GitLab unavailable, naming a delay",
			guard: func() *bearerGuard {
				return newTestGuard(failing(&oauth.UpstreamError{Status: http.StatusTooManyRequests, RetryAfter: 17 * time.Second, Err: errors.New("throttled")}))
			},
			token: "gloas-good",
			want:  guardRefusal{status: http.StatusServiceUnavailable, code: errCodeUpstreamUnavailable, retryAfter: "17", says: []string{"has not been rejected"}},
		},
		{
			name: "GitLab unavailable, naming none",
			guard: func() *bearerGuard {
				return newTestGuard(failing(&oauth.UpstreamError{Err: errors.New("connection refused")}))
			},
			token: "gloas-good",
			want:  guardRefusal{status: http.StatusServiceUnavailable, code: errCodeUpstreamUnavailable, retryAfter: upstreamDefault, says: []string{"has not been rejected"}},
		},
		{
			name:  "an introspection that did not answer",
			guard: func() *bearerGuard { return newTestGuard(failing(oauth.ErrRecipientUnverifiable)) },
			token: "gloas-good",
			want:  guardRefusal{status: http.StatusServiceUnavailable, code: errCodeUpstreamUnavailable, retryAfter: upstreamDefault, says: []string{"introspection"}},
		},
		{
			name:  "a verification that failed for no stated reason",
			guard: func() *bearerGuard { return newTestGuard(failing(errors.New("decode: unexpected EOF"))) },
			token: "gloas-good",
			want:  guardRefusal{status: http.StatusServiceUnavailable, code: errCodeUpstreamUnavailable, retryAfter: upstreamDefault, says: []string{"has not been rejected"}},
		},
		{
			name:  "every verification slot busy",
			guard: func() *bearerGuard { return newTestGuard(failing(oauth.ErrVerificationBusy)) },
			token: "gloas-good",
			want: guardRefusal{
				status: http.StatusServiceUnavailable, code: errCodeUpstreamUnavailable, retryAfter: upstreamDefault,
				says: []string{"GitLab could not verify this token right now.", "has not been rejected"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			failure := tc.guard().check(guardRequest(t, tc.token))
			if failure == nil {
				t.Fatal("the request was let through, want a refusal")
			}
			if failure.status != tc.want.status || failure.code != tc.want.code {
				t.Errorf("status, code = %d, %d, want %d, %d", failure.status, failure.code, tc.want.status, tc.want.code)
			}
			if got := failure.header.Get(headerWWWAuthenticate); got != tc.want.challenge {
				t.Errorf("WWW-Authenticate = %q\nwant              %q", got, tc.want.challenge)
			}
			switch got := failure.header.Get(headerRetryAfter); {
			case tc.want.blockedFor > 0:
				assertRetryAfterIsTheBlock(t, got, tc.want.blockedFor)
			case got != tc.want.retryAfter:
				t.Errorf("Retry-After = %q, want %q", got, tc.want.retryAfter)
			}
			for _, fragment := range tc.want.says {
				if !strings.Contains(failure.message, fragment) {
					t.Errorf("message = %q, want it to carry %q", failure.message, fragment)
				}
			}
		})
	}
}

// TestBearerGuard_ARefusedToken_IsChargedToTheClientThatSentIt pins which
// address pays for a credential GitLab refused, on both paths that refuse one:
// the first time, when GitLab is asked, and every later time, when the answer
// comes out of the rejected-token cache.
//
// Behind a trusted proxy the two addresses in play differ: the client the
// proxy vouches for, and the proxy itself. The failure is the client's. The
// cached path charges too, since replaying a token already refused is still an
// authentication failure; it only costs GitLab nothing.
func TestBearerGuard_ARefusedToken_IsChargedToTheClientThatSentIt(t *testing.T) {
	t.Parallel()

	g := newProxiedGuard(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
		return nil, auth.ErrInvalidToken
	})
	g.limiter = serverpool.NewAuthRateLimiter(2, authFailureWindow)
	g.sourceBudget = nil

	// sequential: the second attempt is only answered from the cache because the first put the refusal there
	for attempt, path := range []string{"asked of GitLab", "answered from the cache"} {
		if failure := g.check(proxiedRequest(t, "198.51.100.1", "gloas-refused")); failure == nil || failure.status != http.StatusUnauthorized {
			t.Fatalf("attempt %d (%s) = %+v, want 401", attempt+1, path, failure)
		}
	}
	if failure := g.check(proxiedRequest(t, "198.51.100.1", "")); failure == nil || failure.status != http.StatusTooManyRequests {
		t.Errorf("the client that sent the refused token twice = %+v, want 429: both refusals are its failures", failure)
	}
	if failure := g.check(proxiedRequest(t, "198.51.100.2", "")); failure == nil || failure.status != http.StatusUnauthorized {
		t.Errorf("another client behind the same proxy = %+v, want 401: it failed nothing", failure)
	}
}

// TestBearerGuard_RejectedTokenCache_IsKeyedOnTheInstance pins that a refusal
// is remembered against the GitLab that made it, for both kinds of refusal the
// cache records.
//
// A token means nothing away from the instance that issued it, so the same
// string refused by one published instance may be valid at another. The cache
// must answer a repeat at the instance that refused it, which is the point of
// having it, and must not answer for the other instance, which would refuse a
// credential no GitLab ever judged.
func TestBearerGuard_RejectedTokenCache_IsKeyedOnTheInstance(t *testing.T) {
	t.Parallel()

	const (
		refusing = "https://gitlab.refusing.example"
		other    = "https://gitlab.other.example"
	)
	for _, refusal := range []struct {
		name string
		err  error
	}{
		{name: "a token GitLab refused", err: auth.ErrInvalidToken},
		{name: "a token issued to an application not admitted", err: refusedRecipient()},
	} {
		t.Run(refusal.name, func(t *testing.T) {
			t.Parallel()

			asked := map[string]int{}
			var mu sync.Mutex
			g := newTestGuard(func(_ context.Context, _ string, r *http.Request) (*auth.TokenInfo, error) {
				mu.Lock()
				defer mu.Unlock()
				asked[r.Header.Get(serverpool.RequestOptionGitLabURL)]++
				return nil, refusal.err
			})
			g.limiter = nil
			g.resolveInstance = func(r *http.Request) (string, error) {
				return r.Header.Get(serverpool.RequestOptionGitLabURL), nil
			}
			at := func(instance string) *http.Request {
				r := guardRequest(t, "gloas-same-string")
				r.Header.Set(serverpool.RequestOptionGitLabURL, instance)
				return r
			}

			for range 2 {
				if failure := g.check(at(refusing)); failure == nil || failure.status != http.StatusUnauthorized {
					t.Fatalf("the refusing instance = %+v, want 401", failure)
				}
			}
			g.check(at(other))

			mu.Lock()
			defer mu.Unlock()
			if asked[refusing] != 1 {
				t.Errorf("the refusing instance was asked %d times for two attempts, want 1: the repeat is the cache's to answer", asked[refusing])
			}
			if asked[other] != 1 {
				t.Errorf("the other instance was asked %d times, want 1: another GitLab's refusal says nothing about this one", asked[other])
			}
		})
	}
}

// TestBearerGuard_Exemption_RequiresAnEntryTheCacheActuallyHolds covers the two
// ways the verified-token lookup can answer without holding a credential: a
// miss that still hands back an identity, and a hit that hands back nothing. A
// block may only be lifted for an entry that is both there and complete, since
// anything less is an exemption granted on a guess.
func TestBearerGuard_Exemption_RequiresAnEntryTheCacheActuallyHolds(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		info   *auth.TokenInfo
		cached bool
	}{
		{name: "a miss carrying an identity", info: &auth.TokenInfo{UserID: "7", Scopes: []string{oauth.ScopeAPI}}, cached: false},
		{name: "a hit carrying nothing", info: nil, cached: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := newTestGuard(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
				return nil, auth.ErrInvalidToken
			})
			g.verified = func(string, string) (*auth.TokenInfo, bool) { return tc.info, tc.cached }
			for i := range 3 {
				g.check(guardRequest(t, "gloas-invented-"+strconv.Itoa(i)))
			}

			if failure := g.check(guardRequest(t, "gloas-looked-up")); failure == nil || failure.status != http.StatusTooManyRequests {
				t.Errorf("= %+v, want 429: the lookup holds no complete entry for this credential", failure)
			}
		})
	}
}

// TestIsCORSPreflight_NeedsTheRequestMethodHeader pins what makes an OPTIONS a
// preflight. The exemption exists because a browser sends a preflight without
// credentials; an OPTIONS that does not ask the preflight question is an
// ordinary request with no credential, and is authenticated like one.
func TestIsCORSPreflight_NeedsTheRequestMethodHeader(t *testing.T) {
	t.Parallel()

	g := newTestGuard(okVerifier(oauth.ScopeAPI))
	req := httptest.NewRequestWithContext(t.Context(), http.MethodOptions, "/mcp", http.NoBody)
	req.RemoteAddr = "192.0.2.10:5555"

	if failure := g.check(req); failure == nil || failure.status != http.StatusUnauthorized {
		t.Errorf("a bare OPTIONS = %+v, want 401: only a preflight skips authentication", failure)
	}
}

// TestOAuthChallenge_EscapesEveryQuotedValue covers the three places a value
// is written inside quotes: a parameter, the scope and the metadata URL. Each
// is server-controlled today, and each is escaped anyway, so a quote reaching
// any of them later still yields a header a client can parse.
func TestOAuthChallenge_EscapesEveryQuotedValue(t *testing.T) {
	t.Parallel()

	got := oauthChallenge(`sco"pe`, `https://x.example/m"d`, "error_description", `a "quoted" \ value`)
	want := `Bearer realm="gitlab-mcp-server", error_description="a \"quoted\" \\ value", scope="sco\"pe", resource_metadata="https://x.example/m\"d"`
	if got != want {
		t.Errorf("oauthChallenge =\n%s\nwant\n%s", got, want)
	}
}

// TestLogUnverified_EachCause_WritesItsOwnLine pins the operator's half of a
// refusal the caller is told about in one set of words (ADM-014 and ADM-002).
// Saturation is a warning, whether or not the request's context ended in the
// same instant, since the slots were what refused it; a wait the request's own
// end cut short is information and says so, because naming it saturation would
// give the operator a cause that was not there; and a round trip that went
// wrong is the error it always was. Neither slot line names the token.
//
// Not parallel: it replaces the process-wide default logger.
func TestLogUnverified_EachCause_WritesItsOwnLine(t *testing.T) {
	ended, cancel := context.WithCancel(t.Context())
	cancel()

	for _, tc := range []struct {
		name  string
		ctx   context.Context
		err   error
		level string
		msg   string
	}{
		{
			name: "the slots stayed taken", ctx: t.Context(), err: oauth.ErrVerificationBusy,
			level: "WARN", msg: "token verification refused: every verification slot stayed busy",
		},
		{
			name: "the slots stayed taken as the request ended", ctx: ended, err: oauth.ErrVerificationBusy,
			level: "WARN", msg: "token verification refused: every verification slot stayed busy",
		},
		{
			name: "the request ended while it waited", ctx: ended,
			err:   fmt.Errorf("token verification abandoned, the request ended first: %w", context.Canceled),
			level: "INFO", msg: "token verification abandoned: the request ended before it was verified",
		},
		{
			name: "the round trip went wrong", ctx: t.Context(), err: errors.New("decode GitLab user response: unexpected EOF"),
			level: "ERROR", msg: "token verification failed",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logged bytes.Buffer
			previous := slog.Default()
			t.Cleanup(func() { slog.SetDefault(previous) })
			slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, &slog.HandlerOptions{Level: slog.LevelDebug})))
			forgetRefusalLines()

			logUnverified(tc.ctx, tc.err)

			var line struct {
				Level string `json:"level"`
				Msg   string `json:"msg"`
			}
			if err := json.Unmarshal(logged.Bytes(), &line); err != nil {
				t.Fatalf("one JSON line expected, got %q: %v", logged.String(), err)
			}
			if line.Level != tc.level || line.Msg != tc.msg {
				t.Errorf("logged %s %q, want %s %q", line.Level, line.Msg, tc.level, tc.msg)
			}
		})
	}
}
