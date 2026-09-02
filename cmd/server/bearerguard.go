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

// Cadence for sweeping expired entries out of the verified-token cache, as a
// fraction of the configured TTL with a floor. A quarter keeps a dead entry
// from outliving its usefulness by much; the floor keeps a 1-minute TTL — the
// lowest --oauth-cache-ttl accepts — from turning the sweep into a hot loop on
// a map that every authenticated request reads.
const (
	tokenCacheSweepDivisor     = 4
	tokenCacheSweepMinInterval = 30 * time.Second
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
	// sourceBudget is the coarse secondary budget charged to the transport
	// source, set only when trustedProxyHeader makes the primary key
	// caller-controlled. Shared with [mcpServerGate], and shared as the budget
	// rather than as its limiter: the accounting is the point of it, and this
	// guard is the layer that spends it first.
	sourceBudget *transportBudget
	// trustedProxyHeader names the header carrying the real client IP, so
	// the limiter counts per caller rather than per reverse proxy.
	trustedProxyHeader string
	// metadataURL is the RFC 9728 protected-resource metadata URL every
	// challenge points at.
	metadataURL string
	// resolveInstance reports which published GitLab instance a request
	// selected, or an error when it named one this deployment does not
	// serve. Nil means "one fixed instance", the single-instance case.
	//
	// The guard runs it before verifying, so a request naming an instance
	// that is not published is refused here rather than several layers later —
	// and, more to the point, before the bearer token is sent anywhere. The
	// verifier runs the same resolver again for the URL to verify against;
	// it is a pure function of the request, so the two cannot disagree.
	resolveInstance oauth.InstanceResolver
	// instances is the published set, named back to a caller that failed to
	// select one. Safe to echo here and only here: oauth mode already serves
	// the same list unauthenticated as RFC 9728 authorization_servers.
	instances []string
	// minimumScope is the least a token must carry to be admitted. A token
	// without it is refused with insufficient_scope rather than executed and
	// failed later by GitLab.
	//
	// It is deliberately not the deployment's own scope. Admission asks for
	// what every action needs; whether a given action may write is decided
	// against the surface the pool built for this token's real authority, so
	// a read_api token gets a read-only catalog instead of a closed door.
	minimumScope string
	// advertisedScope is the scope the challenge recommends: the one that
	// buys this deployment's full surface. A client asking for less is
	// served less, not refused, so this is guidance rather than the check.
	advertisedScope string
}

// middleware rejects what it can classify and passes everything else on.
func (g *bearerGuard) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failure := g.check(r); failure != nil {
			failure.write(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// check returns the failure that stops the request, or nil to let it through.
func (g *bearerGuard) check(r *http.Request) *gateFailure {
	// A CORS preflight carries no credential — the browser strips
	// Authorization from it by definition — so authenticating one means
	// counting every browser's routine permission question as a failed
	// authentication. Ten of them locked that client's address out of the
	// endpoint entirely, for something the user never did. It is let past
	// here to be answered by whatever serves the route.
	if isCORSPreflight(r) {
		return nil
	}

	ip := clientIP(r, g.trustedProxyHeader)
	source := transportSource(r)

	// Ordered before everything else on purpose: a blocked caller must cost
	// nothing, least of all an upstream call.
	if g.blockedByBudget(ip, source) {
		// Both addresses, because the refusal may be either one's doing. The
		// budget charged to the transport source is shared by everything
		// behind one proxy, so ip is frequently just whoever arrived next;
		// naming it alone pointed an operator at an innocent machine.
		slog.WarnContext(r.Context(), "request blocked: too many authentication failures", "ip", ip, "source", source) //#nosec G706 -- slog structured args are not interpolated
		return &gateFailure{
			status:  http.StatusTooManyRequests,
			code:    errCodeTooManyRequests,
			message: "Too many failed authentication attempts from this address. Retry later with a valid token.",
			header:  newHeader(headerRetryAfter, strconv.Itoa(int(authFailureWindow.Seconds()))),
		}
	}

	token := serverpool.ExtractBearerToken(r)
	if token == "" {
		g.recordFailure(ip, source)
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

	// The instance this request selected, captured rather than discarded: a
	// rejection is only meaningful against the GitLab that issued it, so both
	// the rejected-token cache and the verification are scoped to it.
	instance := ""
	if g.resolveInstance != nil {
		resolved, err := g.resolveInstance(r)
		// Neither branch is charged to the limiter: the caller misaddressed
		// the request, which says nothing about the credential.
		if errors.Is(err, errMissingGitLabURL) {
			// Refused rather than resolved to the first published instance:
			// answering would put this bearer on the wire to an instance the
			// caller never named, and it is answered BEFORE verification for
			// exactly that reason.
			slog.InfoContext(r.Context(), "request rejected: no instance selected on a multi-instance deployment",
				"instances", len(g.instances))
			return &gateFailure{
				status: http.StatusBadRequest,
				code:   errCodeInvalidRequest,
				message: "This deployment serves several GitLab instances, so the " +
					serverpool.RequestOptionGitLabURL + " header must name the one this token belongs to. It publishes: " +
					strings.Join(g.instances, ", ") + ".",
			}
		}
		if err != nil {
			slog.InfoContext(r.Context(), "request rejected: unpublished GitLab instance requested", "error", err)
			return &gateFailure{
				status:  http.StatusForbidden,
				code:    errCodeForbidden,
				message: "This deployment does not serve the GitLab instance the GITLAB-URL header names. " + err.Error() + ".",
			}
		}
		instance = resolved
	}

	if g.rejected != nil {
		// Serve the refusal that was actually made. Collapsing every cached
		// entry into GitLab's verdict would undo the distinction one request
		// later: an unadmitted recipient would be told its token is expired,
		// and would start being charged the budget the first refusal spared
		// it.
		if kind, cached := g.rejected.Lookup(instance, token); cached {
			if kind == oauth.RejectionUnaccepted {
				slog.InfoContext(r.Context(), "request rejected: token already known not to be issued to an admitted OAuth application",
					"token_suffix", safeTokenSuffix(token))
				return g.unacceptedRecipientFailure()
			}
			g.recordFailure(ip, source)
			slog.InfoContext(r.Context(), "request rejected: token already known to be invalid", "token_suffix", safeTokenSuffix(token))
			return g.invalidTokenFailure("GitLab rejected this token. Check that it is valid, unexpired, and issued by the target instance.")
		}
	}

	info, err := g.verify(r.Context(), token, r)
	if err != nil {
		return g.classify(err, ip, source, instance, token)
	}

	if !oauth.SatisfiesMinimum(info.Scopes, g.minimumScope) {
		// Not charged to the limiter: the credential is genuine and the
		// caller is who they say they are. Counting it would let a client
		// holding a valid but under-scoped token lock its own address out.
		slog.InfoContext(r.Context(), "request rejected: token cannot read the API",
			"minimum", g.minimumScope, "granted", strings.Join(info.Scopes, " "))
		return &gateFailure{
			status: http.StatusForbidden,
			code:   errCodeForbidden,
			message: "This token carries no GitLab API scope: " + g.minimumScope + " is the least this server can work with. " +
				"Reauthorize the application requesting it, granting " + g.advertisedScope + " for the full tool surface or " +
				g.minimumScope + " for a read-only one.",
			// The scope named here is the MINIMUM that satisfies this
			// request, not the deployment's recommended one. RFC 6750
			// section 3.1 defines the attribute as "the scope necessary to
			// access the protected resource", and the MCP specification
			// says an insufficient_scope challenge should carry "the
			// minimum scopes needed for the operation". Advertising the
			// write scope here contradicted this challenge's own
			// error_description, which names read_api.
			header: newHeader(headerWWWAuthenticate, oauthChallenge(
				g.minimumScope, g.metadataURL,
				"error", "insufficient_scope",
				"error_description", "the token lacks the "+g.minimumScope+" scope",
			)),
		}
	}

	return nil
}

// classify turns a verification error into the response it deserves, keeping
// "your credential is bad" and "GitLab could not tell us" apart.
func (g *bearerGuard) classify(err error, ip, source, instance, token string) *gateFailure {
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
			message: "GitLab could not verify this token right now; the instance is unreachable or throttling. Retry shortly. The token itself has not been rejected.",
			header:  newHeader(headerRetryAfter, strconv.Itoa(int(delay.Seconds()))),
		}
	}

	// A credential GitLab accepts as genuine but under-scoped is the same
	// condition the scope check above handles locally, noticed one layer
	// later — so it gets the same answer, for the same reasons: the client is
	// told to ask for the named scope rather than to discard a working token,
	// and neither the token nor the caller's address is penalized. Charging it
	// would let a client holding a valid token lock its own address out, and
	// caching it as rejected would keep refusing that token for five minutes
	// after the user granted the missing scope.
	if errors.Is(err, oauth.ErrInsufficientScope) {
		slog.Info("request rejected: gitlab says the token lacks the required scope",
			"minimum", g.minimumScope, "token_suffix", safeTokenSuffix(token))
		return &gateFailure{
			status: http.StatusForbidden,
			code:   errCodeForbidden,
			message: "GitLab rejected this token for lacking the scope this request needs. " +
				"Reauthorize granting " + g.advertisedScope + " for the full tool surface, or " +
				g.minimumScope + " for a read-only one. The token itself is valid.",
			header: newHeader(headerWWWAuthenticate, oauthChallenge(
				g.minimumScope, g.metadataURL,
				"error", "insufficient_scope",
				"error_description", "the token lacks the "+g.minimumScope+" scope",
			)),
		}
	}

	// The recipient check could not be made. The request is refused, because a
	// pin that admits what it cannot verify is not a pin, but it is reported as
	// what it is: an upstream problem, not a verdict on the credential. So it
	// carries 503 with Retry-After rather than 401, it is not cached, and it
	// does not charge the failure budget. A transient introspection outage
	// would otherwise lock a valid token out for the whole cache TTL while
	// telling its holder the token belongs to somebody else.
	if errors.Is(err, oauth.ErrRecipientUnverifiable) {
		slog.Warn("token recipient could not be verified: introspection did not answer",
			"token_suffix", safeTokenSuffix(token))
		return &gateFailure{
			status:  http.StatusServiceUnavailable,
			code:    errCodeUpstreamUnavailable,
			message: "This deployment admits only tokens issued to specific OAuth applications, and GitLab did not answer the introspection request needed to check this one. Retry shortly. The token itself has not been rejected.",
			header:  newHeader(headerRetryAfter, strconv.Itoa(int(upstreamRetryAfter.Seconds()))),
		}
	}

	// A token the instance accepts but this deployment does not admit, because
	// --oauth-client-uid pins the OAuth applications it will serve. The status
	// and RFC 6750 code are the same as for a rejected token — section 3.1
	// covers a token "invalid for other reasons" and gives it 401 — but every
	// other part of the answer has to differ, because GitLab rejected nothing.
	//
	// The old wording sent the holder of a genuine, unexpired, instance-valid
	// credential to reauthorize, which returns the same token. It is also kept
	// off the failure budget: this is not a guess at a secret, and charging it
	// let one client lock a shared address out of the endpoint while holding a
	// credential the deployment does admit.
	if errors.Is(err, oauth.ErrUnacceptedRecipient) {
		if g.rejected != nil {
			g.rejected.RecordKind(instance, token, oauth.RejectionUnaccepted)
		}
		slog.Info("request rejected: token was not issued to an admitted OAuth application",
			"token_suffix", safeTokenSuffix(token))
		return g.unacceptedRecipientFailure()
	}

	if errors.Is(err, auth.ErrInvalidToken) {
		if g.rejected != nil {
			g.rejected.Record(instance, token)
		}
		g.recordFailure(ip, source)
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
		message: "GitLab could not verify this token right now. Retry shortly. The token itself has not been rejected.",
		header:  newHeader(headerRetryAfter, strconv.Itoa(int(upstreamRetryAfter.Seconds()))),
	}
}

// unacceptedRecipientFailure builds the 401 for a token the instance accepts
// but --oauth-client-uid does not admit.
//
// RFC 6750 section 3.1 covers a token "invalid for other reasons" and gives it
// 401 with invalid_token, so the status and the code match a rejected token.
// Nothing else does: GitLab rejected nothing, so a description claiming the
// token is expired or revoked would send its holder to reauthorize and come
// back with the same credential.
func (g *bearerGuard) unacceptedRecipientFailure() *gateFailure {
	return &gateFailure{
		status:  http.StatusUnauthorized,
		code:    errCodeUnauthorized,
		message: "This token is valid for the GitLab instance, but it was not issued to an OAuth application this deployment admits. Obtain a token from the application the operator published; see the resource documentation named in the WWW-Authenticate challenge.",
		header: newHeader(headerWWWAuthenticate, g.challenge(
			"error", "invalid_token",
			"error_description", "the token was not issued to an OAuth application this deployment admits",
		)),
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

// recordFailure charges an authentication failure to both budgets: the
// caller's key, and — behind a trusted proxy header, where that key is
// caller-controlled — the transport source it arrived from.
//
// The source is charged through [transportBudget] rather than against its
// limiter directly, and that is the whole of the accounting: once per distinct
// key inside a window, never once per failure. Charging per failure here undid
// the correction the gate had already made, because this guard runs in front
// of the gate and so spends the shared budget first. See
// [transportFailureLimit].
func (g *bearerGuard) recordFailure(key, source string) {
	if g.limiter != nil {
		g.limiter.RecordFailure(key)
	}
	g.sourceBudget.charge(source, key)
}

// blockedByBudget reports whether either budget is exhausted.
func (g *bearerGuard) blockedByBudget(key, source string) bool {
	if g.limiter != nil && g.limiter.IsBlocked(key) {
		return true
	}
	return g.sourceBudget.blocked(source)
}

// challenge builds the WWW-Authenticate value, appending the given key/value
// parameters to the resource_metadata pointer every OAuth-mode challenge
// carries (RFC 9728 section 5.1). Parameters arrive as alternating key and
// value; an odd trailing key is ignored rather than emitted half-formed.
//
// Every challenge also names the scope this deployment requires (RFC 6750
// section 3), not only the insufficient_scope one. A client that reads the
// header and stops there would otherwise have to guess, and the guess that
// costs it is asking the authorization server for everything it advertises:
// GitLab answers that with invalid_scope. The protected-resource document
// says the same thing in scopes_supported, but a client is not obliged to
// fetch it before it authorizes.
func (g *bearerGuard) challenge(params ...string) string {
	return oauthChallenge(g.advertisedScope, g.metadataURL, params...)
}

// oauthChallenge builds an OAuth-mode WWW-Authenticate value. It is a package
// function rather than a method so the request gate behind the guard emits the
// identical shape: two builders would drift, and the parameter a client is
// most likely to act on is the one a rarely-exercised path would forget.
func oauthChallenge(advertisedScope, metadataURL string, params ...string) string {
	var b strings.Builder
	b.WriteString(`Bearer realm="gitlab-mcp-server"`)
	for i := 0; i+1 < len(params); i += 2 {
		b.WriteString(", ")
		b.WriteString(params[i])
		b.WriteString(`="`)
		b.WriteString(quotedStringEscape(params[i+1]))
		b.WriteString(`"`)
	}
	if advertisedScope != "" {
		b.WriteString(`, scope="`)
		b.WriteString(quotedStringEscape(advertisedScope))
		b.WriteString(`"`)
	}
	if metadataURL != "" {
		b.WriteString(`, resource_metadata="`)
		b.WriteString(quotedStringEscape(metadataURL))
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

// isCORSPreflight reports whether a request is a CORS preflight rather than a
// real call: the browser sends OPTIONS with Access-Control-Request-Method and
// no credential, and expects headers back rather than a resource.
func isCORSPreflight(r *http.Request) bool {
	return r.Method == http.MethodOptions && r.Header.Get(headerRequestMethod) != ""
}
