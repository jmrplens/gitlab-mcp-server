package oauth

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
)

// GitLab API scopes this server can operate under. api permits reads and
// writes; read_api is enough for a deployment that never mutates, and is
// what such a deployment asks for so users are not made to grant more.
const (
	ScopeAPI     = "api"
	ScopeReadAPI = "read_api"
)

// MinimumScope is the least a token must carry to be admitted at all. It is
// what the door checks.
//
// The door used to check the deployment's own scope instead, which made a
// property of the server into a demand on every caller: a deployment that
// serves writes refused a read_api token at initialize, before it could list
// the tools it was entitled to call. Whether a given action may write is
// known per action — the catalog says so — so admission asks only for the
// scope every action needs, and the tool surface an entry gets is narrowed
// to match the authority its token actually carries.
const MinimumScope = ScopeReadAPI

// RequiredScope reports the scope a client should ask for to get this
// deployment's full surface: read_api when no request can reach GitLab as a
// write, api otherwise. Safe mode counts as read-only here because it answers
// mutating calls with a preview instead of forwarding them.
//
// It is a recommendation, published in the challenge and as the first entry
// of [SupportedScopes] — not an admission requirement. A client that asks for
// less is served less, not refused.
func RequiredScope(readOnly, safeMode bool) string {
	if readOnly || safeMode {
		return ScopeReadAPI
	}
	return ScopeAPI
}

// SatisfiesMinimum reports whether the scopes GitLab granted a token meet the
// minimum a deployment admits.
//
// api is treated as covering read_api. [expandImpliedScopes] already writes
// that relationship into what the verifier returns, but this check is what
// the door depends on, and a plain set containment here would make admission
// hinge on a normalization step somewhere else: the day a token's scopes
// arrive by another route, every api-only token gets a 403 for lacking a
// scope it strictly supersedes.
func SatisfiesMinimum(granted []string, minimum string) bool {
	if minimum == "" {
		return true
	}
	if slices.Contains(granted, minimum) {
		return true
	}
	return minimum == ScopeReadAPI && slices.Contains(granted, ScopeAPI)
}

// SupportedScopes lists the scopes a client may authorize with, most capable
// first, for RFC 9728 scopes_supported.
//
// A deployment that can write advertises both: api for a client that wants
// the whole surface, read_api for one that deliberately wants a credential
// that cannot break anything — a browser-based inspector, a dashboard, any
// read-only integration. Listing only api forced every such client to hold a
// write-capable token or stay out.
func SupportedScopes(readOnly, safeMode bool) []string {
	if readOnly || safeMode {
		return []string{ScopeReadAPI}
	}
	return []string{ScopeAPI, ScopeReadAPI}
}

// UpstreamError reports that the GitLab instance could not answer the
// verification request, as opposed to answering that the token is bad.
//
// The distinction is not cosmetic. Reporting a throttled or unreachable
// GitLab as an invalid token tells a well-behaved MCP client to discard a
// perfectly good credential and start a fresh authorization flow — which
// generates more upstream traffic at the exact moment the instance asked for
// less, and asks the user to re-approve an application that was never the
// problem.
type UpstreamError struct {
	// Status is the HTTP status GitLab returned, or 0 when the request never
	// produced a response at all.
	Status int
	// RetryAfter is the delay GitLab asked for, or 0 when it did not say.
	RetryAfter time.Duration
	// Err is the underlying cause.
	Err error
}

// Error describes the failure, naming the status when GitLab answered with
// one and saying it was unreachable when nothing came back at all.
func (e *UpstreamError) Error() string {
	if e.Status == 0 {
		return fmt.Sprintf("gitlab unreachable for token verification: %v", e.Err)
	}
	return fmt.Sprintf("gitlab could not verify the token (HTTP %d): %v", e.Status, e.Err)
}

// Unwrap exposes the underlying cause to errors.Is and errors.As.
func (e *UpstreamError) Unwrap() error { return e.Err }

// retryAfter reads RFC 9110 Retry-After, which is either delta-seconds or an
// HTTP-date. An absent, malformed, or past value yields zero, meaning "the
// server did not tell us"; the caller supplies its own default rather than
// inventing one here.
//
// An HTTP-date is rounded up to the next whole second. The delay is rendered
// into a Retry-After header with second granularity, so truncating 0.4s to 0
// would invite a retry before the boundary GitLab asked for.
func retryAfter(resp *http.Response) time.Duration {
	raw := strings.TrimSpace(resp.Header.Get("Retry-After"))
	if raw == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(raw); err == nil {
		if seconds <= 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(raw); err == nil {
		if d := time.Until(when); d > 0 {
			return time.Duration(math.Ceil(d.Seconds())) * time.Second
		}
	}
	return 0
}

// ErrInsufficientScope reports a credential GitLab accepts as genuine but which
// does not carry the scope the request needed.
//
// It is deliberately distinct from [auth.ErrInvalidToken]. Reporting the two the
// same way tells a client to discard a working credential and re-run its
// authorization flow, which returns the same under-scoped token and loops; the
// action that resolves it is a step-up request naming the scope. It also matters
// upstream of that: an invalid token is charged against the caller's
// authentication-failure budget, and charging a genuine one lets a client lock
// its own address out of the endpoint while holding a perfectly good token.
var ErrInsufficientScope = errors.New("token lacks the required GitLab scope")

// insufficientScopeLimit bounds how much of a rejection body is read. The body
// is attacker-adjacent — it comes from whatever host the request selected — and
// nothing legitimate needs more than this to name an error code.
const insufficientScopeLimit = 4 << 10

// isInsufficientScope reports whether GitLab's 403 says the token is genuine but
// under-scoped, rather than that the caller may not do this at all.
//
// The distinction is only in the body. GitLab does not send WWW-Authenticate on
// a 403: the Forbidden class it inherits never builds that header, which its own
// source marks as a known standards deviation. The quote and the upstream
// reference are in docs/development/upstream-bugs.md. A server reading the
// challenge would therefore fall through on every real rejection while looking
// correct. What it does send
// is rack-oauth2's JSON error document. A 403 raised by Grape's own forbidden!
// (an account blocked, an IP restriction) carries a "message" key instead and no
// "error", so it keeps being treated as an invalid credential.
func isInsufficientScope(resp *http.Response) bool {
	var payload struct {
		Error string `json:"error"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, insufficientScopeLimit)).Decode(&payload) != nil {
		return false
	}
	// GitLab emits both spellings, the second for granular PAT scopes.
	return payload.Error == "insufficient_scope" || payload.Error == "insufficient_granular_scope"
}

// gitlabUserResponse holds the minimal fields from GitLab's /api/v4/user endpoint.
type gitlabUserResponse struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}

// NewGitLabVerifier returns an [auth.TokenVerifier] that validates Bearer
// tokens by calling the GitLab /api/v4/user endpoint. Verified identities
// are cached in cache (if non-nil) to avoid redundant API calls.
//
// The returned verifier populates [auth.TokenInfo] with:
//   - UserID: the GitLab user's numeric ID (as string)
//   - Extra["username"]: the GitLab user's login name
//   - Extra["token"]: the raw token (for downstream GitLab client creation)
//   - Expiration: now + cacheTTL (so the SDK middleware honors TTL)
func NewGitLabVerifier(gitlabURL string, skipTLS bool, cacheTTL time.Duration, cache *TokenCache, clientUIDs ...string) auth.TokenVerifier {
	return NewGitLabVerifierFor(
		func(*http.Request) (string, error) { return gitlabURL, nil },
		skipTLS, cacheTTL, cache, clientUIDs...,
	)
}

// InstanceResolver reports which GitLab instance a request must be verified
// against. It is a pure function of the request: the guard in front calls it
// to reject an instance the deployment does not publish, and the verifier
// calls it again to know where to send the verification, and the two must
// agree without sharing state.
type InstanceResolver func(*http.Request) (string, error)

// NewGitLabVerifierFor is [NewGitLabVerifier] for a deployment that publishes
// more than one instance.
//
// Verification is per instance because a token means nothing away from the
// GitLab that issued it: sending it to the wrong one either fails or, worse,
// succeeds against an instance where the same string happens to be valid for
// somebody else. The resolver is the operator's allow-list made executable,
// and the cache is keyed by instance and token together for the same reason.
// clientUIDs, when non-empty, pins the OAuth applications whose tokens this
// deployment admits — see [acceptedRecipient]. It is a set rather than a scalar
// because --gitlab-url is repeatable and each published instance has its own
// OAuth application with its own uid.
func NewGitLabVerifierFor(resolve InstanceResolver, skipTLS bool, cacheTTL time.Duration, cache *TokenCache, clientUIDs ...string) auth.TokenVerifier {
	baseTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		baseTransport = &http.Transport{}
	}
	transport := baseTransport.Clone()
	if skipTLS {
		transport.TLSClientConfig = &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: true, //#nosec G402 //nolint:gosec // user-configured opt-in for self-signed certificates
		}
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	return func(ctx context.Context, token string, r *http.Request) (*auth.TokenInfo, error) {
		gitlabURL, err := resolve(r)
		if err != nil {
			return nil, err
		}

		if cache != nil {
			if info, cached := cache.Get(gitlabURL, token); cached {
				return info, nil
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, gitlabURL+"/api/v4/user", http.NoBody)
		if err != nil {
			return nil, fmt.Errorf("create verification request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := client.Do(req)
		if err != nil {
			return nil, &UpstreamError{Err: fmt.Errorf("token verification request failed: %w", err)}
		}
		defer resp.Body.Close()

		switch {
		case resp.StatusCode == http.StatusOK:
			// success — parse below
		case resp.StatusCode == http.StatusUnauthorized:
			return nil, fmt.Errorf("token rejected by GitLab (HTTP %d): %w", resp.StatusCode, auth.ErrInvalidToken)
		case resp.StatusCode == http.StatusForbidden:
			if isInsufficientScope(resp) {
				return nil, fmt.Errorf("token rejected by GitLab (HTTP %d): %w", resp.StatusCode, ErrInsufficientScope)
			}
			return nil, fmt.Errorf("token rejected by GitLab (HTTP %d): %w", resp.StatusCode, auth.ErrInvalidToken)
		case resp.StatusCode == http.StatusTooManyRequests:
			// Not an invalid token: GitLab declined to answer the question.
			return nil, &UpstreamError{
				Status:     resp.StatusCode,
				RetryAfter: retryAfter(resp),
				Err:        errors.New("rate limit exceeded"),
			}
		case resp.StatusCode >= 500:
			return nil, &UpstreamError{Status: resp.StatusCode, Err: errors.New("server error")}
		default:
			// Only 401 and 403 are GitLab judging the credential. Anything
			// else — a 404 from a misrouted proxy, a 408, whatever an
			// intermediary invents — is a question that never got answered,
			// and calling it invalid_token would cache a valid token as
			// rejected and charge the caller's failure budget for someone
			// else's routing mistake.
			return nil, &UpstreamError{Status: resp.StatusCode, Err: errors.New("unexpected response")}
		}

		var user gitlabUserResponse
		if decErr := json.NewDecoder(resp.Body).Decode(&user); decErr != nil {
			return nil, fmt.Errorf("decode GitLab user response: %w", decErr)
		}
		if user.ID == 0 {
			return nil, fmt.Errorf("GitLab returned invalid user: %w", auth.ErrInvalidToken)
		}

		// The token's REAL scopes, introspected rather than assumed: a
		// read_user-scoped token passes /user, and stamping it api-scoped
		// would let the SDK's scope check wave through a token GitLab will
		// then reject on every write. PATs answer
		// /personal_access_tokens/self; OAuth tokens answer
		// /oauth/token/info. When neither endpoint yields scopes (older or
		// restricted instances), the historical api assumption is kept and
		// logged, so exotic deployments keep working while mainstream
		// tokens are checked for what they actually carry.
		result := introspectToken(ctx, client, gitlabURL, token)

		// Recipient verification, when the operator asked for it. Checked
		// before anything is cached, so a refused token never becomes a
		// cached identity.
		if recipientErr := acceptedRecipient(clientUIDs, result); recipientErr != nil {
			return nil, recipientErr
		}
		return admitToken(cache, gitlabURL, token, cacheTTL, user, result), nil
	}
}

// admitToken builds the admission record and caches it for as long as the
// credential itself lives.
func admitToken(cache *TokenCache, gitlabURL, token string, cacheTTL time.Duration, user gitlabUserResponse, result introspection) *auth.TokenInfo {
	// The cached admission must not outlive the credential it was taken from.
	// Caching for the configured TTL alone would keep answering 200 for a token
	// that expired a moment after it was verified, for as long as fifteen
	// minutes by default — and the specification requires an expired token to
	// be answered 401.
	ttl := effectiveCacheTTL(cacheTTL, result.expiry)

	info := &auth.TokenInfo{
		UserID:     strconv.Itoa(user.ID),
		Scopes:     result.scopes,
		Expiration: time.Now().Add(ttl),
		Extra: map[string]any{
			"username": user.Username,
			"token":    token,
		},
	}

	if cache != nil {
		cache.Put(gitlabURL, token, info, ttl)
	}

	return info
}

// introspectScopes resolves a token's granted scopes. Personal access
// tokens answer GET /api/v4/personal_access_tokens/self; OAuth access
// tokens answer GET /oauth/token/info. Either endpoint failing to yield
// scopes falls back to the historical {"api"} assumption with a debug log —
// refusal here would brick instances where introspection is restricted.
// effectiveCacheTTL is the configured TTL, shortened when the token itself dies
// sooner. A zero expiry means the token does not expire or the instance did not
// say, in which case the configured TTL stands. A token already past its expiry
// gets the smallest useful lifetime rather than a negative one; GitLab will have
// answered 401 for it anyway, so this only guards the arithmetic.
func effectiveCacheTTL(configured time.Duration, tokenExpiry time.Time) time.Duration {
	if tokenExpiry.IsZero() {
		return configured
	}
	remaining := time.Until(tokenExpiry)
	if remaining <= 0 {
		return time.Second
	}
	return min(configured, remaining)
}

// introspection is what one round trip to GitLab's token endpoints yields.
//
// applicationUID names the OAuth application the token was minted for, and is
// empty for a personal access token, which belongs to no application. It is the
// only recipient signal GitLab offers: its authorization server publishes no
// resource_indicators_supported, so RFC 8707 audience restriction is not
// available and this is the "otherwise verify that they are the intended
// recipient" alternative the specification allows.
type introspection struct {
	scopes         []string
	expiry         time.Time
	applicationUID string
	// answered records whether an endpoint actually replied with scopes. A
	// pinned deployment must refuse when it did not, because the scope
	// fallback below is deliberately fail-open and reusing its shape would let
	// anything that breaks introspection walk past the pin.
	answered bool
}

// introspectToken returns the token's real scopes and its own expiry.
//
// The expiry is what stops a cached admission from outliving the credential it
// was taken from: the specification requires an expired token to be answered
// 401, and a cache keyed only on a locally-chosen TTL would keep serving one for
// up to that TTL after it died. A zero time means the token does not expire, or
// the instance did not say — the caller then falls back to its own TTL.
//
// The two endpoints disagree about how to express it, which is easy to get
// wrong: /oauth/token/info gives expires_in, a number of seconds from now, while
// /api/v4/personal_access_tokens/self gives expires_at as a plain YYYY-MM-DD
// date. Parsing the latter as RFC 3339 fails and silently degrades to "no
// expiry", which is exactly the bug this exists to prevent.
func introspectToken(ctx context.Context, client *http.Client, gitlabURL, token string) introspection {
	if payload := fetchIntrospection(ctx, client, gitlabURL+"/api/v4/personal_access_tokens/self", token); payload != nil {
		if scopes := stringSlice(payload["scopes"]); scopes != nil {
			return introspection{
				scopes:   expandImpliedScopes(scopes),
				expiry:   expiryFromDate(payload["expires_at"]),
				answered: true,
			}
		}
	}
	if payload := fetchIntrospection(ctx, client, gitlabURL+"/oauth/token/info", token); payload != nil {
		if scopes := stringSlice(payload["scope"]); scopes != nil {
			return introspection{
				scopes:         expandImpliedScopes(scopes),
				expiry:         expiryFromSeconds(payload["expires_in"]),
				applicationUID: applicationUID(payload["application"]),
				answered:       true,
			}
		}
	}
	slog.Debug("token scope introspection unavailable; assuming api scope")
	return introspection{scopes: expandImpliedScopes([]string{"api"})}
}

// applicationUID reads the uid of the OAuth application a token was issued to,
// from the nested "application" object /oauth/token/info returns.
func applicationUID(raw any) string {
	app, ok := raw.(map[string]any)
	if !ok {
		return ""
	}
	uid, _ := app["uid"].(string)
	return uid
}

// acceptedRecipient reports whether a token may be admitted under an operator's
// recipient pin, and says why when it may not.
//
// An empty pin admits everything: that is the default, because pinning refuses
// personal access tokens outright — a PAT belongs to no application — and a PAT
// is a supported credential here.
//
// With a pin set, the check inverts the surrounding fail-open behavior on
// purpose. An absent, unreadable or unmatched uid is a refusal, and so is an
// introspection that never answered: treating "we could not ask" as "it is
// fine" would make breaking introspection the way around the pin.
func acceptedRecipient(pinned []string, result introspection) error {
	if len(pinned) == 0 {
		return nil
	}
	if !result.answered {
		return fmt.Errorf("token recipient could not be verified: introspection did not answer: %w", auth.ErrInvalidToken)
	}
	if result.applicationUID == "" {
		return fmt.Errorf("token names no OAuth application, and this deployment admits only tokens issued to its own: %w", auth.ErrInvalidToken)
	}
	if !slices.Contains(pinned, result.applicationUID) {
		return fmt.Errorf("token was issued to another OAuth application: %w", auth.ErrInvalidToken)
	}
	return nil
}

// expiryFromDate reads a GitLab personal access token's expires_at, which is a
// calendar date rather than a timestamp.
//
// The date is the instant the token dies, not the last day it works: GitLab
// expires a personal access token at 00:00:00 UTC **on** the stated date, so a
// token reading 2030-01-02 is already refused at the first moment of the 2nd.
// time.Parse with time.DateOnly yields exactly that instant, so the value is
// returned as parsed. Adding a day — reading the date as "valid through" —
// would let a cached admission outlive the credential by up to 24 hours, which
// is the one thing [effectiveCacheTTL] exists to prevent.
func expiryFromDate(raw any) time.Time {
	s, ok := raw.(string)
	if !ok || s == "" {
		return time.Time{}
	}
	day, err := time.Parse(time.DateOnly, s)
	if err != nil {
		slog.Debug("token expiry not parsable as a date; treating the token as non-expiring", "value", s)
		return time.Time{}
	}
	return day
}

// expiryFromSeconds reads an OAuth token's expires_in, a count of seconds from
// now. It is null for a token that does not expire.
func expiryFromSeconds(raw any) time.Time {
	seconds, ok := raw.(float64)
	if !ok || seconds <= 0 {
		return time.Time{}
	}
	return time.Now().Add(time.Duration(seconds) * time.Second)
}

// stringSlice reads a JSON array of strings, returning nil when the field is
// absent, not an array, or carries no strings.
func stringSlice(raw any) []string {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, v := range items {
		if text, isString := v.(string); isString {
			out = append(out, text)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// expandImpliedScopes adds the scopes GitLab grants implicitly. api is a
// strict superset of read_api, but GitLab reports only the granted name, and
// the SDK's scope check is a plain set containment: without this, a
// read-only deployment asking for read_api would reject the very tokens that
// carry more authority than it needs.
func expandImpliedScopes(scopes []string) []string {
	if slices.Contains(scopes, ScopeAPI) && !slices.Contains(scopes, ScopeReadAPI) {
		return append(slices.Clone(scopes), ScopeReadAPI)
	}
	return scopes
}

// fetchScopes reads one introspection endpoint and returns the named
// string-array field, or nil when the endpoint does not answer for this
// token kind.
func fetchIntrospection(ctx context.Context, client *http.Client, endpoint, token string) map[string]any {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var payload map[string]any
	if json.NewDecoder(resp.Body).Decode(&payload) != nil {
		return nil
	}
	return payload
}
