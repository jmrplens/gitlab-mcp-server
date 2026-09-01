package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/serverpool"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Authentication rate limiting for HTTP mode: a client IP is blocked after
// authFailureLimit failed authentications inside authFailureWindow.
const (
	authFailureLimit  = 10
	authFailureWindow = 1 * time.Minute
)

// JSON-RPC error codes emitted by the request gate.
//
// The MCP specification partitions the JSON-RPC implementation-defined range:
// -32000 to -32019 is legacy and must not be used by new implementations, and
// -32020 to -32099 is reserved for the specification itself. Codes for purposes
// the specification does not define must be allocated outside the whole
// reserved range (-32768 to -32000), so the gate's own codes mirror their HTTP
// status multiplied by -100, well below -32768.
const (
	errCodeInvalidRequest = -32600 // standard JSON-RPC "Invalid Request"
	// codeUnsupportedProtocolVersion is defined by the MCP specification, not
	// by this server, so it sits inside the reserved range where the spec put
	// it rather than following the mirrored-status convention below.
	codeUnsupportedProtocolVersion = -32022
	errCodeUnauthorized            = -40100 // mirrors HTTP 401
	errCodeForbidden               = -40300 // mirrors HTTP 403
	errCodeTooManyRequests         = -42900 // mirrors HTTP 429
	errCodeUpstreamUnavailable     = -50300 // mirrors HTTP 503
)

// legacyAuthChallenge is the WWW-Authenticate challenge for --auth-mode=legacy.
//
// It deliberately omits the resource_metadata parameter. Clients discover an
// OAuth authorization server through that parameter, and legacy mode has none:
// /.well-known/oauth-protected-resource is mounted only by
// registerOAuthMCPHandlers. Advertising it here would send clients into a
// discovery flow that cannot complete. Bearer is still the honest scheme name
// because [serverpool.ExtractToken] accepts "Authorization: Bearer <pat>"
// alongside the PRIVATE-TOKEN header.
//
// RFC 6750 section 3.1 says a challenge for a request that carries no
// credential at all should not include an error code, so the actionable
// instructions travel in the JSON-RPC error message instead.
const legacyAuthChallenge = `Bearer realm="gitlab-mcp-server"`

// missingTokenMessage tells the caller how to authenticate against this
// deployment. It names both accepted headers because legacy mode takes either.
const missingTokenMessage = "Authentication required: send a GitLab personal access token as " +
	"'Authorization: Bearer <glpat-...>' or 'PRIVATE-TOKEN: <glpat-...>'. " +
	"This server uses static token authentication; no OAuth authorization server is configured."

// oauthMissingTokenMessage is the equivalent for --auth-mode=oauth, where
// PRIVATE-TOKEN is not an accepted alias and the client is expected to
// discover the authorization server rather than be handed a token by a human.
const oauthMissingTokenMessage = "Authentication required: send an OAuth access token as " +
	"'Authorization: Bearer <token>'. Discover the authorization server through the " +
	"resource_metadata URL in this response's WWW-Authenticate header."

// resolvedServerContextKey carries the pool server that [mcpServerGate] resolved
// for a request through to the SDK's getServer callback.
type resolvedServerContextKey struct{}

// serverFromRequestContext is the getServer callback handed to
// [mcp.NewStreamableHTTPHandler]. [mcpServerGate] has already resolved and
// validated the server, so this only reads it back out of the request context.
//
// A nil return makes the SDK answer "400 no server available" in plain text,
// which is what this whole gate exists to avoid; it can only happen if the
// handler is mounted without the gate in front of it.
func serverFromRequestContext(r *http.Request) *mcp.Server {
	server, _ := r.Context().Value(resolvedServerContextKey{}).(*mcp.Server)
	if server == nil {
		slog.Error("MCP handler reached without a gated server; check handler wiring")
	}
	return server
}

// gateFailure is a request rejection ready to be written as an HTTP response.
type gateFailure struct {
	status  int
	code    int
	message string
	// header carries response headers the status requires, such as
	// WWW-Authenticate on 401 or Retry-After on 429.
	header http.Header
}

// newHeader builds a canonical single-value header set.
//
// Constructing an http.Header from a map literal skips canonicalization, so a
// key spelled the way its RFC spells it — "WWW-Authenticate" — never answers
// Header.Get, whose lookup key is canonicalized to "Www-Authenticate". The
// wire output stays correct either way because [gateFailure.write] uses Add,
// but anything reading a failure back sees an empty header. Pairs arrive as
// alternating name and value; an odd trailing name is ignored.
func newHeader(pairs ...string) http.Header {
	h := http.Header{}
	for i := 0; i+1 < len(pairs); i += 2 {
		h.Set(pairs[i], pairs[i+1])
	}
	return h
}

// jsonRPCError is the wire shape of a transport-level rejection.
//
// It carries the id of the request it refuses, recovered from the body by
// [requestIDFromBody], and omits the member when there is none to carry. The id
// used to be a *string that nothing ever set, so every rejection went out as
// null: unmatchable by a client that routes on id, and not a legal RequestId
// under 2026-07-28, which admits a string or an integer and marks the member
// optional. A *string could not have carried a numeric id in any case.
//
// Emitting this instead of plain text also matters for version negotiation. A
// client that receives 400 with a body that is not a recognized JSON-RPC error
// is told by the specification to conclude the server is initialization-era and
// downgrade, so an opaque 400 turns a missing header into a false protocol
// diagnosis. A client that recognizes the body by validating it against the
// published schema rather than by reading error.code is the case that makes the
// id shape matter: null fails that validation, and the failure it produces is
// the exact downgrade this body exists to prevent.
type jsonRPCError struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      json.RawMessage  `json:"id,omitempty"`
	Error   jsonRPCErrorBody `json:"error"`
}

type jsonRPCErrorBody struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// write emits the failure as a JSON-RPC error response with the mapped status.
//
// The request is taken so the refusal can name what it refuses. Every caller
// returns immediately afterwards, which is what makes reading the body here
// safe.
func (f *gateFailure) write(w http.ResponseWriter, r *http.Request) {
	for name, values := range f.header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(f.status)
	if err := json.NewEncoder(w).Encode(jsonRPCError{
		JSONRPC: "2.0",
		ID:      requestIDFromBody(r),
		Error:   jsonRPCErrorBody{Code: f.code, Message: f.message},
	}); err != nil {
		slog.Error("failed to write gate error response", "error", err)
	}
}

// mcpServerGate resolves the pool server for each MCP request before the SDK
// handler runs.
//
// The SDK's getServer callback can only signal failure by returning nil, and it
// answers every nil with "400 no server available" in plain text. That single
// response conflates four unrelated conditions — rate limiting, a missing
// credential, a malformed GITLAB-URL header, and an unavailable backend — and
// uses the one status code the current transport specification reserves for
// protocol negotiation. Resolving ahead of the handler lets each condition
// carry its own status, headers, and machine-readable reason.
type mcpServerGate struct {
	pool *serverpool.ServerPool
	// sessionTags maps each pooled server to the tag prefixing the session
	// IDs it mints, so a stateful request presenting a session ID can be
	// checked against the credential that minted it. Nil in tests and in any
	// mode without a pool, where the check is skipped.
	sessionTags *sync.Map
	// gitlabURLs are the instances this deployment publishes. Empty means
	// the caller chooses freely (legacy, unfixed); one is the pinned
	// instance every request reaches; several make the GITLAB-URL header a
	// selection among them, refused when it names anything else.
	gitlabURLs []string
	// limiter blocks IPs with repeated authentication failures. In OAuth
	// mode it is the same limiter [bearerGuard] uses, so the two layers
	// share one per-address budget and a caller cannot earn a fresh
	// allowance by failing at whichever layer it has not exhausted yet.
	limiter            *serverpool.AuthRateLimiter
	trustedProxyHeader string
	// challenge is the WWW-Authenticate value sent with a 401.
	challenge string
	// oauthMode reports whether this gate sits behind the bearer guard, which
	// decides whether an RFC 6750 error code belongs in the challenge: legacy
	// mode's challenge advertises no metadata URL and names no error, since
	// there is no authorization server for the client to go back to.
	oauthMode bool
	// bearerOnly restricts credential extraction to Authorization: Bearer
	// and ignores PRIVATE-TOKEN. Set in oauth mode: the SDK middleware
	// verifies the Bearer token, so the gate must execute as that same
	// identity — never as an unverified PRIVATE-TOKEN a request might also
	// carry, which ExtractToken would otherwise prefer.
	bearerOnly bool
	// stateless mirrors Config.Stateless, and decides whether GET and DELETE
	// may skip authentication.
	//
	// On a stateless deployment the SDK answers both with 405 whatever they
	// carry, so there is nothing to protect and letting them through is how
	// the specified answer gets emitted. On a stateful one they are live
	// operations on a session someone else created — GET opens that session's
	// standalone SSE stream and reads its server-initiated messages, DELETE
	// terminates it — so they must present the credential that owns it, like
	// every POST does.
	stateless bool
}

// verifiedScopes returns the scopes the OAuth layer already resolved for this
// request, or nil when nothing upstream resolved any.
//
// In oauth mode the SDK's bearer middleware runs before the gate and publishes
// the verified token info, whose scopes came from GitLab's own introspection.
// Handing them to the pool is what lets a read_api token be served at all: the
// PAT self endpoint the pool would otherwise ask does not answer for an OAuth
// access token, so the entry would be built as if the token's authority were
// unknown. In legacy mode nothing has verified anything yet and nil is the
// honest answer — the pool detects the PAT's scopes itself.
func verifiedScopes(r *http.Request) []string {
	info := auth.TokenInfoFromContext(r.Context())
	if info == nil {
		return nil
	}
	return info.Scopes
}

// extractCredential returns the credential the gate authenticates with,
// honoring bearerOnly.
func (g *mcpServerGate) extractCredential(r *http.Request) string {
	if g.bearerOnly {
		return serverpool.ExtractBearerToken(r)
	}
	return serverpool.ExtractToken(r)
}

// middleware resolves the request's MCP server and either passes the request on
// with the server attached, or writes the classified rejection.
func (g *mcpServerGate) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// POST always carries an MCP message, so it is always gated. GET and
		// DELETE depend on the transport:
		//
		// On a stateless deployment the SDK answers both with 405 Method Not
		// Allowed — what protocol 2026-07-28 prescribes — whatever they carry.
		// They address nothing, so gating them would only replace a correct
		// answer with a 401.
		//
		// On a stateful one they are not inert: GET opens a session's
		// standalone SSE stream and reads the server-initiated messages meant
		// for its owner, and DELETE terminates the session. Waving them through
		// would make a session ID sufficient to read or end someone else's
		// session, so they take the same resolution and ownership check a POST
		// takes.
		//
		// Every other method passes through unconditionally. OPTIONS is the one
		// that matters: it is a CORS preflight, which a browser sends without
		// credentials by definition, so gating it would refuse the request that
		// exists to ask whether the real request is allowed.
		if r.Method != http.MethodPost && !g.addressesSession(r.Method) {
			next.ServeHTTP(w, r)
			return
		}

		server, failure := g.resolve(r) //nolint:contextcheck // pool bounds per-token scope detection with its own timeout
		if failure != nil {
			failure.write(w, r)
			return
		}
		if sessionFailure := g.checkSessionOwnership(r, server); sessionFailure != nil {
			sessionFailure.write(w, r)
			return
		}
		ctx := context.WithValue(r.Context(), resolvedServerContextKey{}, server)
		ctx = g.withIdentity(ctx, r)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// addressesSession reports whether a non-POST method can act on a live MCP
// session in this deployment's transport mode.
func (g *mcpServerGate) addressesSession(method string) bool {
	if g.stateless {
		return false
	}
	return method == http.MethodGet || method == http.MethodDelete
}

// withIdentity attaches the GitLab user behind the request's credential to
// the context, so tool handlers resolve an identity in HTTP mode too.
//
// [toolutil.ResolveIdentity] reads req.Extra.TokenInfo first and falls back to
// the context. Only the SDK's bearer middleware can populate that field — its
// context key is unexported — and legacy mode does not mount it, so before
// this every HTTP legacy request resolved to the zero identity and log lines
// carried no user at all. The pool resolved the user when it built the entry,
// so this is a map lookup, not a round trip.
//
// An unresolved identity is left absent rather than stored empty: a handler
// must be able to tell "the lookup did not succeed" from "user with no name".
func (g *mcpServerGate) withIdentity(ctx context.Context, r *http.Request) context.Context {
	if g.pool == nil {
		return ctx
	}
	options, err := serverpool.ResolveRequestOptionsFor(r, g.gitlabURLs)
	if err != nil {
		return ctx
	}
	identity, ok := g.pool.IdentityFor(g.extractCredential(r), options.GitLabURL)
	if !ok || !identity.Resolved() {
		return ctx
	}
	return toolutil.IdentityToContext(ctx, toolutil.UserIdentity{
		UserID:   identity.UserID,
		Username: identity.Username,
		// Carried rather than resolved: the instance was already computed above
		// to choose the pool entry, and a GitLab user id is only unique within
		// one. See [toolutil.UserIdentity.Instance].
		Instance: options.GitLabURL,
	})
}

// resolve returns the pool server for the request, or the failure that stopped
// it. Exactly one of the two return values is non-nil.
// mcpSessionIDHeader is the streamable HTTP session header. The SDK keeps its
// own copy unexported, so the name is restated here rather than reached for.
const mcpSessionIDHeader = "Mcp-Session-Id"

// checkSessionOwnership refuses a request that presents a session ID minted for
// a different credential.
//
// The SDK short-circuits a stateful POST carrying Mcp-Session-Id straight to
// that session's own transport and never calls the server-resolution function,
// so without this check any credential the pool admits could drive another
// caller's session — executing against their GitLab instance, with their token,
// and being recorded in the audit log under their username. The MCP security
// guidance is explicit that a handle must be bound server-side to the
// authenticated principal and refused when presented by any other.
//
// 404 is the prescribed answer: it is the terminated-session signal, and a
// conforming client responds by starting a new session without an ID, so the
// refusal self-heals rather than stranding the caller.
func (g *mcpServerGate) checkSessionOwnership(r *http.Request, server *mcp.Server) *gateFailure {
	sessionID := r.Header.Get(mcpSessionIDHeader)
	if sessionID == "" || g.sessionTags == nil {
		return nil
	}
	presented, ok := sessionTagOf(sessionID)
	if !ok {
		// A session ID this deployment never minted. Stateless mode issues
		// none at all, so anything untagged is either stale or forged.
		return sessionOwnershipFailure()
	}
	owned, found := g.sessionTags.Load(server)
	if !found || owned != presented {
		slog.Warn("request rejected: session ID does not belong to the presented credential")
		return sessionOwnershipFailure()
	}
	return nil
}

// sessionOwnershipFailure is the refusal written for a session that belongs to
// another credential.
func sessionOwnershipFailure() *gateFailure {
	return &gateFailure{
		status:  http.StatusNotFound,
		code:    errCodeInvalidRequest,
		message: "This session does not belong to the presented credential. Start a new session by sending initialize without a session ID.",
	}
}

func (g *mcpServerGate) resolve(r *http.Request) (*mcp.Server, *gateFailure) {
	ip := clientIP(r, g.trustedProxyHeader)

	if g.limiter != nil && g.limiter.IsBlocked(ip) {
		slog.Warn("request blocked: too many authentication failures", "ip", ip) //#nosec G706 -- slog structured args are not interpolated
		return nil, &gateFailure{
			status:  http.StatusTooManyRequests,
			code:    errCodeTooManyRequests,
			message: "Too many failed authentication attempts from this address. Retry later with a valid token.",
			header:  newHeader("Retry-After", strconv.Itoa(int(authFailureWindow.Seconds()))),
		}
	}

	token := g.extractCredential(r)
	if token == "" {
		if g.limiter != nil {
			g.limiter.RecordFailure(ip)
		}
		slog.Info("request rejected: missing authentication token (set PRIVATE-TOKEN header or Authorization: Bearer)")
		return nil, &gateFailure{
			status:  http.StatusUnauthorized,
			code:    errCodeUnauthorized,
			message: missingTokenMessage,
			header:  newHeader("WWW-Authenticate", g.challenge),
		}
	}

	options, err := serverpool.ResolveRequestOptionsFor(r, g.gitlabURLs)
	if err != nil {
		slog.Error("request rejected: invalid GITLAB-URL header", "error", err)
		return nil, &gateFailure{
			status:  http.StatusBadRequest,
			code:    errCodeInvalidRequest,
			message: g.invalidURLMessage(r, err),
		}
	}
	logIgnoredRequestOptions(token, options)

	server, err := g.pool.GetOrCreateWithScopes(token, options.GitLabURL, verifiedScopes(r))
	if errors.Is(err, serverpool.ErrInvalidCredential) {
		// GitLab itself rejected the token, so this is an authentication
		// failure in the full sense: 401, and it does count against the
		// limiter — this is the path that stops a stream of invented tokens
		// from churning the pool.
		if g.limiter != nil {
			g.limiter.RecordFailure(ip)
		}
		slog.Info("request rejected: gitlab rejected the supplied token", "token_suffix", safeTokenSuffix(token))
		return nil, &gateFailure{
			status:  http.StatusUnauthorized,
			code:    errCodeUnauthorized,
			message: "GitLab rejected this token. Check that it is valid, unexpired, and issued by the target instance.",
			// Same verdict as the bearer guard's, so it carries the same RFC
			// 6750 error code. Without it a client cannot tell this apart
			// from "you sent no credential", and the two call for different
			// actions: reauthorize versus authorize.
			header: newHeader("WWW-Authenticate", g.invalidTokenChallenge()),
		}
	}
	if err != nil {
		// Deliberately not charged to the authentication limiter. Any other
		// pool failure means the backend could not be reached or the server
		// could not be built — the credential was never judged. Counting it
		// would let a GitLab outage lock out clients holding valid tokens,
		// which is the same conflation of causes this gate exists to remove.
		slog.Error("failed to create server for token", "error", err)
		// The pool error can name internal state, so it is logged but not
		// returned to the caller.
		return nil, &gateFailure{
			status:  http.StatusServiceUnavailable,
			code:    errCodeUpstreamUnavailable,
			message: "Could not initialize a GitLab session for this token. The instance may be unreachable; retry shortly.",
		}
	}
	return server, nil
}

// invalidURLMessage describes a GITLAB-URL rejection to the caller.
//
// The parse detail is echoed only when the client actually sent the header, so
// a misconfigured server-side --gitlab-url is never reflected back. The
// underlying error already names the header, so it is returned verbatim rather
// than prefixed.
func (g *mcpServerGate) invalidURLMessage(r *http.Request, err error) string {
	if r.Header.Get(serverpool.RequestOptionGitLabURL) == "" {
		return "The server's configured GitLab instance URL is invalid; contact the operator."
	}
	// Naming the published instances is a help to a client that guessed wrong,
	// and it is safe exactly when that list is already public — which is oauth
	// mode, where the same set is served unauthenticated as RFC 9728
	// authorization_servers. Legacy mode publishes no metadata document, and
	// this rejection is reached before the credential is validated, so echoing
	// the list there would let any non-empty token enumerate the operator's
	// instance hostnames.
	var disallowed *serverpool.DisallowedGitLabURLError
	if errors.As(err, &disallowed) && !g.oauthMode {
		return "The GITLAB-URL header names an instance this deployment does not serve. Ask the operator which instances it publishes."
	}
	return capitalizeFirst(err.Error())
}

// capitalizeFirst upper-cases the first letter so Go's lowercase error strings
// read as sentences in a client-facing message.
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// invalidTokenChallenge is the gate's challenge for a credential GitLab
// refused, carrying the RFC 6750 error code that verdict deserves.
//
// The bearer guard emits error="invalid_token" for the identical judgement;
// the gate reaches this only for a pool rejection the guard could not see, and
// answering the same verdict two different ways leaves a client unable to tell
// "reauthorize" from "you sent nothing".
func (g *mcpServerGate) invalidTokenChallenge() string {
	if !g.oauthMode {
		return g.challenge
	}
	return g.challenge + `, error="invalid_token", error_description="the access token is expired, revoked, or not valid for this GitLab instance"`
}
