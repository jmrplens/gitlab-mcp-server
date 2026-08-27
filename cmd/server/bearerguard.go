// bearerguard.go authenticates OAuth-mode requests before the SDK's bearer
// middleware runs.
//
// The SDK's [auth.RequireBearerToken] is kept in the chain — it is the only
// thing that can populate the token info the streamable handler reads, since
// its context key is unexported — but it is not enough on its own. It relays
// every attempt straight upstream, it answers in plain text rather than the
// JSON-RPC shape the rest of this endpoint uses, and its challenge carries
// neither the RFC 6750 error code a client needs to tell "re-authenticate"
// apart from "ask for more scope", nor any way to say "this was not about
// your token at all".
//
// The guard runs first and answers those cases itself. A request it lets
// through reaches the SDK middleware, whose own verification is a hit on the
// cache this guard just populated, so the upstream cost is one call either
// way.
package main

import (
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/oauth"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/serverpool"
)

// Bounds for the rejected-token cache. The TTL is deliberately short: a
// rejection is definitive for a given credential, but keeping one for hours
// would mean a token GitLab rejected during a misconfiguration stays refused
// long after the instance was fixed. The size cap bounds attacker-supplied
// keys — see [oauth.RejectedTokens].
const (
	rejectedTokenTTL     = 5 * time.Minute
	rejectedTokenMaxSize = 4096
)

// upstreamRetryAfter is the delay advertised when GitLab throttled or failed
// the verification without saying when to come back.
const upstreamRetryAfter = 30 * time.Second

// Response header names used by more than one rejection path.
const (
	headerRetryAfter      = "Retry-After"
	headerWWWAuthenticate = "WWW-Authenticate"
)

// bearerGuard authenticates a request before the SDK bearer middleware sees
// it, so that a rejection is cheap, rate limited, and precisely described.
type bearerGuard struct {
	verify   auth.TokenVerifier
	rejected *oauth.RejectedTokens
	limiter  *serverpool.AuthRateLimiter
	// trustedProxyHeader names the header carrying the real client IP, so
	// the limiter counts per caller rather than per reverse proxy.
	trustedProxyHeader string
	// metadataURL is the RFC 9728 protected-resource metadata URL every
	// challenge points at.
	metadataURL string
	// requiredScope is the GitLab scope this deployment needs; a token
	// without it is refused with insufficient_scope rather than executed
	// and failed later by GitLab.
	requiredScope string
}

// middleware rejects what it can classify and passes everything else on.
func (g *bearerGuard) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failure := g.check(r); failure != nil {
			failure.write(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// check returns the failure that stops the request, or nil to let it through.
func (g *bearerGuard) check(r *http.Request) *gateFailure {
	ip := clientIP(r, g.trustedProxyHeader)

	// Ordered before everything else on purpose: a blocked caller must cost
	// nothing, least of all an upstream call.
	if g.limiter != nil && g.limiter.IsBlocked(ip) {
		slog.Warn("request blocked: too many authentication failures", "ip", ip) //#nosec G706 -- slog structured args are not interpolated
		return &gateFailure{
			status:  http.StatusTooManyRequests,
			code:    errCodeTooManyRequests,
			message: "Too many failed authentication attempts from this address. Retry later with a valid token.",
			header:  newHeader(headerRetryAfter, strconv.Itoa(int(authFailureWindow.Seconds()))),
		}
	}

	token := serverpool.ExtractBearerToken(r)
	if token == "" {
		g.recordFailure(ip)
		// RFC 6750 section 3.1: a challenge answering a request that carried
		// no credential at all must not name an error code, since the client
		// has not got anything wrong yet.
		return &gateFailure{
			status:  http.StatusUnauthorized,
			code:    errCodeUnauthorized,
			message: oauthMissingTokenMessage,
			header:  newHeader(headerWWWAuthenticate, g.challenge()),
		}
	}

	if g.rejected != nil && g.rejected.Contains(token) {
		g.recordFailure(ip)
		slog.Info("request rejected: token already known to be invalid", "token_suffix", safeTokenSuffix(token))
		return g.invalidTokenFailure("GitLab rejected this token. Check that it is valid, unexpired, and issued by the target instance.")
	}

	info, err := g.verify(r.Context(), token, r)
	if err != nil {
		return g.classify(err, ip, token)
	}

	if g.requiredScope != "" && !slices.Contains(info.Scopes, g.requiredScope) {
		// Not charged to the limiter: the credential is genuine and the
		// caller is who they say they are. Counting it would let a client
		// holding a valid but under-scoped token lock its own address out.
		slog.Info("request rejected: token lacks the required scope",
			"required", g.requiredScope, "granted", strings.Join(info.Scopes, " "))
		return &gateFailure{
			status:  http.StatusForbidden,
			code:    errCodeForbidden,
			message: "This token does not carry the " + g.requiredScope + " scope that this deployment requires. Reauthorize the application requesting it.",
			header: newHeader(headerWWWAuthenticate, g.challenge(
				"error", "insufficient_scope",
				"error_description", "the token lacks the "+g.requiredScope+" scope",
				"scope", g.requiredScope,
			)),
		}
	}

	return nil
}

// classify turns a verification error into the response it deserves, keeping
// "your credential is bad" and "GitLab could not tell us" apart.
func (g *bearerGuard) classify(err error, ip, token string) *gateFailure {
	if upstream, ok := errors.AsType[*oauth.UpstreamError](err); ok {
		// Deliberately not charged to the limiter and never cached: the
		// token was never judged, so counting this would let a GitLab
		// outage lock out clients holding valid credentials.
		delay := upstream.RetryAfter
		if delay <= 0 {
			delay = upstreamRetryAfter
		}
		slog.Warn("token verification unavailable", "status", upstream.Status, "error", err)
		return &gateFailure{
			status:  http.StatusServiceUnavailable,
			code:    errCodeUpstreamUnavailable,
			message: "GitLab could not verify this token right now; the instance is unreachable or throttling. Retry shortly — the token itself has not been rejected.",
			header:  newHeader(headerRetryAfter, strconv.Itoa(int(delay.Seconds()))),
		}
	}

	if errors.Is(err, auth.ErrInvalidToken) {
		if g.rejected != nil {
			g.rejected.Record(token)
		}
		g.recordFailure(ip)
		slog.Info("request rejected: gitlab rejected the supplied token", "token_suffix", safeTokenSuffix(token))
		return g.invalidTokenFailure("GitLab rejected this token. Check that it is valid, unexpired, and issued by the target instance.")
	}

	// Anything else means the verification round-trip itself went wrong —
	// an undecodable body, for instance. The credential was not judged, so
	// it is treated like any other upstream failure.
	slog.Error("token verification failed", "error", err)
	return &gateFailure{
		status:  http.StatusServiceUnavailable,
		code:    errCodeUpstreamUnavailable,
		message: "GitLab could not verify this token right now. Retry shortly — the token itself has not been rejected.",
		header:  newHeader(headerRetryAfter, strconv.Itoa(int(upstreamRetryAfter.Seconds()))),
	}
}

// invalidTokenFailure builds the 401 shared by a freshly rejected token and
// one answered from the rejected-token cache.
func (g *bearerGuard) invalidTokenFailure(message string) *gateFailure {
	return &gateFailure{
		status:  http.StatusUnauthorized,
		code:    errCodeUnauthorized,
		message: message,
		header: newHeader(headerWWWAuthenticate, g.challenge(
			"error", "invalid_token",
			"error_description", "the access token is expired, revoked, or not valid for this GitLab instance",
		)),
	}
}

// recordFailure charges an authentication failure to the caller's address.
func (g *bearerGuard) recordFailure(ip string) {
	if g.limiter != nil {
		g.limiter.RecordFailure(ip)
	}
}

// challenge builds the WWW-Authenticate value, appending the given key/value
// parameters to the resource_metadata pointer every OAuth-mode challenge
// carries (RFC 9728 section 5.1). Parameters arrive as alternating key and
// value; an odd trailing key is ignored rather than emitted half-formed.
func (g *bearerGuard) challenge(params ...string) string {
	var b strings.Builder
	b.WriteString(`Bearer realm="gitlab-mcp-server"`)
	for i := 0; i+1 < len(params); i += 2 {
		b.WriteString(", ")
		b.WriteString(params[i])
		b.WriteString(`="`)
		b.WriteString(quotedStringEscape(params[i+1]))
		b.WriteString(`"`)
	}
	if g.metadataURL != "" {
		b.WriteString(`, resource_metadata="`)
		b.WriteString(quotedStringEscape(g.metadataURL))
		b.WriteString(`"`)
	}
	return b.String()
}

// quotedStringEscape makes a value safe inside an RFC 9110 quoted-string.
// Every value used here is server-controlled, so this is belt and braces
// against a future caller passing something with a quote in it and silently
// producing a header a client cannot parse.
func quotedStringEscape(s string) string {
	if !strings.ContainsAny(s, `"\`) {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		if r == '"' || r == '\\' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
