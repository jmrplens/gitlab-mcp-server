package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/serverpool"
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
	errCodeInvalidRequest      = -32600 // standard JSON-RPC "Invalid Request"
	errCodeUnauthorized        = -40100 // mirrors HTTP 401
	errCodeTooManyRequests     = -42900 // mirrors HTTP 429
	errCodeUpstreamUnavailable = -50300 // mirrors HTTP 503
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

// jsonRPCError is the wire shape of a transport-level rejection.
//
// The id is null because the request body was never parsed: the Streamable HTTP
// transport explicitly permits an error response with no id for rejections at
// this layer, and JSON-RPC 2.0 represents an undeterminable id as null.
//
// Emitting this instead of plain text also matters for version negotiation. A
// client that receives 400 with a body that is not a recognized JSON-RPC error
// is told by the specification to conclude the server is initialization-era and
// downgrade, so an opaque 400 turns a missing header into a false protocol
// diagnosis.
type jsonRPCError struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *string          `json:"id"`
	Error   jsonRPCErrorBody `json:"error"`
}

type jsonRPCErrorBody struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// write emits the failure as a JSON-RPC error response with the mapped status.
func (f *gateFailure) write(w http.ResponseWriter) {
	for name, values := range f.header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(f.status)
	if err := json.NewEncoder(w).Encode(jsonRPCError{
		JSONRPC: "2.0",
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
	pool      *serverpool.ServerPool
	gitlabURL string
	// limiter blocks IPs with repeated authentication failures. It is nil in
	// OAuth mode, where the SDK's bearer middleware rejects unauthenticated
	// requests before they reach the gate.
	limiter            *serverpool.AuthRateLimiter
	trustedProxyHeader string
	// challenge is the WWW-Authenticate value sent with a 401.
	challenge string
}

// middleware resolves the request's MCP server and either passes the request on
// with the server attached, or writes the classified rejection.
func (g *mcpServerGate) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only POST carries MCP messages. GET and DELETE must keep reaching the
		// SDK so it answers 405 Method Not Allowed, which is what protocol
		// 2026-07-28 prescribes for them on a stateless endpoint.
		if r.Method != http.MethodPost {
			next.ServeHTTP(w, r)
			return
		}

		server, failure := g.resolve(r) //nolint:contextcheck // pool bounds per-token scope detection with its own timeout
		if failure != nil {
			failure.write(w)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), resolvedServerContextKey{}, server)))
	})
}

// resolve returns the pool server for the request, or the failure that stopped
// it. Exactly one of the two return values is non-nil.
func (g *mcpServerGate) resolve(r *http.Request) (*mcp.Server, *gateFailure) {
	ip := clientIP(r, g.trustedProxyHeader)

	if g.limiter != nil && g.limiter.IsBlocked(ip) {
		slog.Warn("request blocked: too many authentication failures", "ip", ip) //#nosec G706 -- slog structured args are not interpolated
		return nil, &gateFailure{
			status:  http.StatusTooManyRequests,
			code:    errCodeTooManyRequests,
			message: "Too many failed authentication attempts from this address. Retry later with a valid token.",
			header: http.Header{
				"Retry-After": []string{strconv.Itoa(int(authFailureWindow.Seconds()))},
			},
		}
	}

	token := serverpool.ExtractToken(r)
	if token == "" {
		if g.limiter != nil {
			g.limiter.RecordFailure(ip)
		}
		slog.Info("request rejected: missing authentication token (set PRIVATE-TOKEN header or Authorization: Bearer)")
		return nil, &gateFailure{
			status:  http.StatusUnauthorized,
			code:    errCodeUnauthorized,
			message: missingTokenMessage,
			header:  http.Header{"WWW-Authenticate": []string{g.challenge}},
		}
	}

	options, err := serverpool.ResolveRequestOptions(r, g.gitlabURL)
	if err != nil {
		slog.Error("request rejected: invalid GITLAB-URL header", "error", err)
		return nil, &gateFailure{
			status:  http.StatusBadRequest,
			code:    errCodeInvalidRequest,
			message: invalidURLMessage(r, err),
		}
	}
	logIgnoredRequestOptions(token, options)

	server, err := g.pool.GetOrCreate(token, options.GitLabURL)
	if err != nil {
		// Deliberately not charged to the authentication limiter. A pool
		// failure means the backend could not be reached or the server could
		// not be built — the credential was never judged. Counting it would
		// let a GitLab outage lock out clients holding valid tokens, which is
		// the same conflation of causes this gate exists to remove.
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
func invalidURLMessage(r *http.Request, err error) string {
	if r.Header.Get(serverpool.RequestOptionGitLabURL) == "" {
		return "The server's configured GitLab instance URL is invalid; contact the operator."
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
