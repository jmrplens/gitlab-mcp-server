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

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/mcpotel"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/oauth"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/serverpool"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/telemetry"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Authentication rate limiting for HTTP mode: a client address, an IPv6 one by
// the /64 it lies in ([budgetKey]), is blocked after authFailureLimit failed
// authentications inside authFailureWindow.
//
// What the block refuses is an authentication, not an address. A credential
// this deployment is already serving is admitted from a blocked address too —
// [mcpServerGate.credentialAlreadyAdmitted] in legacy mode,
// [bearerGuard.credentialAlreadyVerified] in oauth mode — and both answer from
// memory, so the budget still bounds exactly what it was built to bound: the
// distinct credentials one address can have verified upstream.
//
// The distinction exists because an address is not a client. A corporate NAT, a
// campus, a mobile carrier and a reverse proxy without --trusted-proxy-header
// all present one address for many people, so charging the address alone let
// one client relaying invented tokens answer 429 to every legitimate neighbor
// for the rest of the window — including a request carrying a token this
// deployment had already verified and was holding a pool entry for. A sprayer
// gains nothing from the exemption: producing a credential the pool already
// holds means holding a credential GitLab accepts, which is not the state a
// spray is in.
const (
	// The two figures a deployment gets unless it says otherwise. They live in
	// internal/config with every other default, and are named here because
	// the tests below build guards directly rather than through a Config.
	authFailureLimit  = config.DefaultAuthFailureLimit
	authFailureWindow = config.DefaultAuthFailureWindow
	// transportFailureLimit is the secondary budget, charged to the address
	// the connection actually came from rather than to whatever a trusted
	// proxy header claims.
	//
	// It exists because the primary budget's key is whatever a trusted proxy
	// said once --trusted-proxy-header is set, and a proxy that copies a
	// caller's own header into it hands that key to the caller: rotate it
	// and every request gets a distinct key, the ten-a-minute lockout never
	// fires, and each invalid token is relayed one to one to GitLab as a
	// /user verification, spending the deployment's own throttle budget
	// there from a cheap client. --trusted-proxies closes the direct route
	// (a header from any other peer is ignored, see [clientIP]); this
	// budget covers the proxy that forwards what it was told.
	//
	// It counts distinct primary keys, not failures. See [transportBudget]:
	// charging it per failure aggregated a whole fleet behind one proxy, so
	// fifty clients failing their own ten times each exhausted it between
	// them and the next request through that proxy was refused before its
	// credential was read.
	//
	// Five hundred distinct failing client identities inside a minute from
	// one transport source is the rotation, or an incident either way. It
	// also bounds the primary limiter's map for a source this budget's own
	// limiter tracks: a blocked source is refused before a failure is
	// recorded, so it stops minting keys. And it bounds the pairs the budget
	// keeps for one source, at its value (issue 982): a source that has been
	// charged that many distinct keys is blocked, and remembering more of
	// them teaches nothing.
	transportFailureLimit = tenancy.TransportSourceDistinctKeys // register row AUB-002
	// transportTrackedSources is how many sources the transport-source
	// budget indexes before a new one makes it forget every source none of
	// whose pairs is live (issue 982). It is the ceiling of the limiter's own
	// table, since every source the budget holds a pair of was recorded
	// there when it was charged.
	transportTrackedSources = tenancy.AuthTrackedSources // register row AUB-004
)

// JSON-RPC error codes emitted by the request gate.
//
// The MCP specification (2026-07-28, basic) partitions the JSON-RPC
// implementation-defined range: -32000 to -32019 is legacy, where new codes
// must not be allocated, new implementations should not use any, and a
// receiver must not assume a meaning, and -32020 to -32099 is reserved for the
// specification itself. Codes for purposes the specification does not define
// belong outside the whole reserved range (-32768 to -32000), so the gate's own
// codes mirror their HTTP status multiplied by -100, well below -32768, and the
// in-band refusals of the same things carry the same codes (issue 956).
const (
	errCodeInvalidRequest = -32600 // standard JSON-RPC "Invalid Request"
	// codeUnsupportedProtocolVersion is defined by the MCP specification, not
	// by this server, so it sits inside the reserved range where the spec put
	// it rather than following the mirrored-status convention below.
	codeUnsupportedProtocolVersion = -32022
	errCodeUnauthorized            = tenancy.CodeUnauthorized    // mirrors HTTP 401; register row AUB-001
	errCodeForbidden               = tenancy.CodeForbidden       // mirrors HTTP 403; register row AUB-001
	errCodeTooManyRequests         = tenancy.CodeTooManyRequests // mirrors HTTP 429; register row AUB-001
	errCodeUpstreamUnavailable     = tenancy.CodeUnavailable     // mirrors HTTP 503; register row AUB-001
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
		slog.ErrorContext(r.Context(), "MCP handler reached without a gated server; check handler wiring")
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
		slog.ErrorContext(r.Context(), "failed to write gate error response", "error", err)
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
	// sessions records which pooled credential each MCP session belongs to, so
	// a stateful request presenting a session ID can be checked against the
	// credential that opened it. Nil in tests and in any mode without a pool,
	// where the check is skipped.
	//
	// It replaces a tag prefixed to every session ID by the server that minted
	// it. That worked while a server served one credential and stopped meaning
	// anything when one server started serving a configuration shape: the SDK
	// asks for a session ID with no request in hand, so a shared server mints
	// under one tag for everybody.
	sessions *sessionOwners
	// credentials maps a resolved pool entry to the state it owns on the server
	// it shares, which the gate stamps onto the request context for the MCP
	// middleware to bind. Nil wherever sessions is.
	credentials *credentialStates
	// gitlabURLs are the instances this deployment publishes. Empty means
	// the caller chooses freely (legacy, unfixed); one is the pinned
	// instance every request reaches; several make the GITLAB-URL header a
	// selection among them, refused when it names anything else.
	gitlabURLs []string
	// limiter blocks IPs with repeated authentication failures. In OAuth
	// mode it is the same limiter [bearerGuard] uses, so the two layers
	// share one per-address budget and a caller cannot earn a fresh
	// allowance by failing at whichever layer it has not exhausted yet.
	limiter *serverpool.AuthRateLimiter
	// sourceBudget is the secondary, far coarser budget charged to the
	// transport source. Set only when trustedProxyHeader is configured,
	// which is the only case in which the primary key is caller-controlled;
	// without a trusted header the two keys are the same string and one
	// budget is the whole story. Its limiter is shared with [bearerGuard],
	// like limiter.
	sourceBudget *transportBudget
	// spray is the slow budget: distinct credentials refused per address over
	// a longer window, answered with a block that lengthens each time. Shared
	// with [bearerGuard] for the same reason limiter is, so a sprayer cannot
	// earn a fresh allowance by moving between the two layers. Nil when the
	// deployment turns it off, and every method tolerates that.
	spray *serverpool.DistinctTokenBudget
	// network is the aggregate one level above the address (AUB-006): the
	// distinct /64s of one IPv6 /48 that had a credential refused, counted on
	// the slow budget's machinery with the /64 in place of the credential.
	// Shared with [bearerGuard] like spray. Nil only in tests that build a
	// gate without it; a deployment always has one ([authNetworkBudget]).
	network *serverpool.DistinctTokenBudget
	// blocks counts the refusals each budget produced, for telemetry. Shared
	// with [bearerGuard], since the two layers share the budgets themselves.
	blocks             *authBlockCounters
	trustedProxyHeader string
	// trustedProxies are the peers trustedProxyHeader is believed from; from
	// anybody else the header is ignored and the peer is charged.
	trustedProxies trustedProxies
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
	// held is the count of calls the process holds open, shared by every gate
	// and server of the process (HLD-011), which the gate takes a slot of for
	// a POST whose headers name one held call. A gate built without one, which
	// only the tests build, takes none and leaves every call to the servers'
	// own count.
	held *processSlots
	// statefulSessions is the count of stateful sessions the process keeps,
	// shared by every gate of the process (HLD-010), which the gate takes a
	// slot of for a POST that would open one, together with a slot of held
	// for the session's standalone stream. A gate built without one, which
	// only the tests build, counts no session.
	statefulSessions *processSlots
	// rejected remembers the credentials GitLab accepted and refused the
	// permission to read their own user, so the gate answers the same
	// credential again from memory instead of building an entry that fails
	// the same probe (ADM-006). In oauth mode it is the bearer guard's
	// structure, which records that verdict first. Nil in tests that build a
	// gate without one, where nothing is remembered.
	rejected *oauth.RejectedTokens
	// guard is the bearer guard in front of this gate in oauth mode, and nil in
	// legacy mode. A credential this gate refuses as genuine is given the
	// guard's answer for it, with its insufficient_scope challenge: below the
	// admission minimum, which in oauth mode is a token the verifier admitted
	// on its own api assumption (a personal access token, whose scopes no
	// introspection answers), or refused the permission to read its own user,
	// which reaches the gate only when the pool's probe and the verifier's
	// disagree. The guard gives the same answer to every later request with
	// the credential from memory, so the first answer carries the challenge
	// RFC 6750 section 3.1 asks of an OAuth door and words it as the later
	// ones do.
	guard *bearerGuard
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

// verifiedFacts is [verifiedScopes] with the token's own id, when the OAuth
// layer's introspection read one from the self endpoint ([oauth.TokenIDKey]):
// what the pool needs to read a fine-grained token's grant without asking the
// self endpoint a second time. It is nil whenever verifiedScopes is, so the
// pool detects the token itself exactly when it used to. When no introspection
// endpoint answered, the scopes are the verifier's assumption
// ([oauth.ScopesAssumedKey]) and say nothing of the token's kind, so the facts
// say the kind is unknown, which has the pool ask the self endpoint itself.
func verifiedFacts(r *http.Request) *gitlabclient.TokenFacts {
	scopes := verifiedScopes(r)
	if scopes == nil {
		return nil
	}
	extra := auth.TokenInfoFromContext(r.Context()).Extra
	id, _ := extra[oauth.TokenIDKey].(int64)
	facts := gitlabclient.FactsFromScopes(scopes, id)
	facts.KindUnknown, _ = extra[oauth.ScopesAssumedKey].(bool)
	return &facts
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

		entry, failure := g.resolve(r)
		if failure != nil {
			failure.write(w, r)
			return
		}
		if sessionFailure := g.checkSessionOwnership(r, entry); sessionFailure != nil {
			sessionFailure.write(w, r)
			return
		}
		ctx := context.WithValue(r.Context(), resolvedServerContextKey{}, entry.Server())
		// A POST that would open a stateful session takes its slots here,
		// before the SDK creates anything, and is refused with a 503 when one
		// is not free: a session slot (HLD-010), and a held slot for the
		// standalone stream the session may hold open (HLD-011). The session
		// keeps both once the first request on it is dispatched, and a POST
		// whose session did not survive it gives them back as it returns.
		// Like the held slot below, they are taken once the credential is
		// admitted, the order both rows record.
		if g.statefulSessions != nil && g.opensSession(r) {
			slot := g.takeSessionSlot(w, r)
			if slot == nil {
				return
			}
			defer slot.releaseUnkept()
			ctx = context.WithValue(ctx, sessionSlotKey{}, slot)
		}
		// A POST whose headers name one held call takes its slot here, before
		// the SDK reads a byte of it, and is refused with a 503 when none is
		// free; every other call is counted where the SDK dispatches it
		// (HLD-011). A session's standalone stream, the GET a stateful
		// session holds open, takes none here: its slot was taken with the
		// session's, so a session the process keeps is never refused it.
		// Either way the slot is taken once the credential is admitted and
		// before anything keyed on it runs, so a refusal spends none of the
		// credential's rate. That departs from PAT-003, which takes the
		// process slot before anything per key: admission comes first,
		// because a slot taken before it would let a caller with no credential
		// hold one for as long as its verification takes, and the register
		// records what a refused newcomer has spent by then.
		if g.held != nil && gateCountsRequest(r) {
			if !g.held.acquire() {
				refuseHeldRequest(w, r, g.held.limit)
				return
			}
			defer g.held.release()
			ctx = context.WithValue(ctx, gateHeldSlotKey{}, &gateHeldSlot{})
		}
		// The credential travels on the HTTP request context, which the carrier
		// registry then makes reachable from inside the MCP handler chain. It
		// cannot be put on the handler's own context here: that context does not
		// descend from this request. See [requestCarriers].
		ctx = withRequestCredential(ctx, g.credentials.get(entry.Owner()))
		ctx = g.withIdentity(ctx, r)
		// Every request that reaches the MCP handler is subject to the
		// readiness gate, and only requests that reach it. A pooled entry
		// registers its catalog in the background, so a request arriving in
		// that window must wait for it rather than be answered from an empty
		// one; the same server also talks MCP to itself while registering,
		// over in-memory clients that never pass through here, and those must
		// not wait for the step they are part of.
		ctx = withReadinessGate(ctx)
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

// mcpSessionIDHeader is the streamable HTTP session header. The SDK keeps its
// own copy unexported, so the name is restated here rather than reached for.
const mcpSessionIDHeader = "Mcp-Session-Id"

// checkSessionOwnership refuses a request that presents a session ID naming no
// session open for the presented credential: one minted for a different
// credential, or one this process does not hold.
//
// The SDK short-circuits a stateful POST carrying Mcp-Session-Id straight to
// that session's own transport and never calls the server-resolution function,
// so without this check any credential the pool admits could drive another
// caller's session — executing against their GitLab instance, with their token,
// and being recorded in the audit log under their username. The MCP security
// guidance is explicit that a handle must be bound server-side to the
// authenticated principal and refused when presented by any other.
//
// 404 is the prescribed answer: it is the terminated-session signal, and the
// 2025-11-25 transport requires a client that receives it to start a new
// session without an ID, so for a conforming client the refusal self-heals.
// The go-sdk client does not yet (row 68 of docs/development/upstream-bugs.md,
// pinned in test/e2e/http/session_not_found_test.go): it keeps the refused
// session on a call and fails the connection on its stream or a notification.
//
// Both cases get the same answer (issue 1102, row ADM-007). Telling them apart
// on the wire would tell a caller whether another credential holds the ID,
// which the tenant policy's INV-019 forbids, and would need a record of each
// credential's ended session IDs, state keyed on a value the caller mints. The
// log does tell them apart, each case on a line of its own, so an operator can
// see which one a client met.
func (g *mcpServerGate) checkSessionOwnership(r *http.Request, entry *serverpool.Entry) *gateFailure {
	sessionID := r.Header.Get(mcpSessionIDHeader)
	if sessionID == "" || g.sessions == nil {
		return nil
	}
	owner := g.sessions.ownerOfID(sessionID)
	if owner == "" {
		// A session ID this process never recorded, or one it no longer
		// holds: the session idled out or was deleted, its credential's pool
		// entry was evicted, or it was opened before a restart or on another
		// replica. That is an everyday ending rather than a caller presenting
		// somebody else's handle, so the line is INFO; it is throttled with
		// the gates' other refusal lines, since a caller can send any ID it
		// likes. Stateless mode issues no IDs at all, so anything presented
		// there lands here too.
		refusalLog.log(r.Context(), slog.LevelInfo, "request rejected: session ID names no session open in this process")
		return sessionOwnershipFailure()
	}
	if owner != entry.Owner() {
		refusalLog.log(r.Context(), slog.LevelWarn, "request rejected: session ID does not belong to the presented credential")
		return sessionOwnershipFailure()
	}
	return nil
}

// sessionOwnershipFailure is the refusal written for a session ID that names
// no session open for the presented credential, in words true both of another
// credential's session and of one that ended.
func sessionOwnershipFailure() *gateFailure {
	return &gateFailure{
		status:  http.StatusNotFound,
		code:    errCodeInvalidRequest,
		message: "This session is not open for the presented credential. Start a new session by sending initialize without a session ID.",
	}
}

// resolve returns the pool server for the request, or the failure that stopped
// it. Exactly one of the two return values is non-nil.
func (g *mcpServerGate) resolve(r *http.Request) (*serverpool.Entry, *gateFailure) {
	ip := clientIP(r, g.trustedProxyHeader, g.trustedProxies)
	source := transportSource(r)

	if blocked, retryAfter, reason := g.blockedByBudget(ip, source); blocked && !g.credentialAlreadyAdmitted(r) {
		g.blocks.record(reason)
		// Named apart from the bearer guard's line: the throttle keys on the
		// message, and one gate's window must not swallow the other's first
		// report. The source is what spent the budget; behind a proxy the ip
		// is often whoever arrived next.
		refusalLog.log(r.Context(), slog.LevelWarn, "request blocked: too many authentication failures (legacy auth mode)", "ip", ip, "source", source, "budget", reason) //#nosec G706 -- slog structured args are not interpolated
		return nil, &gateFailure{
			status:  http.StatusTooManyRequests,
			code:    errCodeTooManyRequests,
			message: "Too many failed authentication attempts from this address. Retry later with a valid token.",
			header:  newHeader("Retry-After", strconv.Itoa(retryAfterSeconds(retryAfter))),
		}
	}

	token := g.extractCredential(r)
	if token == "" {
		// No credential to count as distinct, and chargeFailure is given the
		// empty one deliberately: a client that forgot its header must not
		// move the escalation ladder.
		g.chargeFailure(ip, source, token)
		refusalLog.log(r.Context(), slog.LevelInfo, "request rejected: missing authentication token (set PRIVATE-TOKEN header or Authorization: Bearer)")
		return nil, &gateFailure{
			status:  http.StatusUnauthorized,
			code:    errCodeUnauthorized,
			message: missingTokenMessage,
			header:  newHeader("WWW-Authenticate", g.challenge),
		}
	}

	if err := requireExplicitInstance(r, g.gitlabURLs); err != nil {
		refusalLog.log(r.Context(), slog.LevelInfo, "request rejected: no instance selected on a multi-instance deployment",
			"instances", len(g.gitlabURLs))
		return nil, &gateFailure{
			status:  http.StatusBadRequest,
			code:    errCodeInvalidRequest,
			message: g.missingURLMessage(),
		}
	}

	options, err := serverpool.ResolveRequestOptionsFor(r, g.gitlabURLs)
	if err != nil {
		// A request that named no instance on a deployment that publishes none
		// is the caller's omission, not a broken server. It reaches this branch
		// with an empty header, which used to mean only one thing: a
		// --gitlab-url the operator typed wrong. Kept apart so the caller is
		// told to send the header rather than to go and find the operator, and
		// logged at INFO because nothing here is the deployment's fault.
		if errors.Is(err, serverpool.ErrMissingGitLabURL) {
			refusalLog.log(r.Context(), slog.LevelInfo, "request rejected: no instance selected and none is published")
			return nil, &gateFailure{
				status:  http.StatusBadRequest,
				code:    errCodeInvalidRequest,
				message: capitalizeFirst(err.Error()) + ".",
			}
		}
		refusalLog.log(r.Context(), slog.LevelError, "request rejected: invalid GITLAB-URL header", "error", err)
		return nil, &gateFailure{
			status:  http.StatusBadRequest,
			code:    errCodeInvalidRequest,
			message: g.invalidURLMessage(r, err),
		}
	}
	// A caller who spells a private or a cloud-metadata address in the header
	// is refused at the door rather than at the dialer. Both would refuse, and
	// the dialer is still the authority — this consults the same predicate —
	// but reaching it means admitting the credential, building a pool entry
	// and failing every action afterwards, for a destination this server was
	// never going to connect to.
	//
	// Only where the header chooses the instance. With one or more published,
	// the header selects among the operator's own, which is exempt by
	// construction: see ADR-0022.
	if len(g.gitlabURLs) == 0 {
		if refusal := gitlabclient.CheckCallerNamedInstance(options.GitLabURL); refusal != nil {
			refusalLog.log(r.Context(), slog.LevelInfo, "request rejected: the GITLAB-URL header named a destination this server will not connect to", "error", refusal)
			return nil, &gateFailure{
				status:  http.StatusBadRequest,
				code:    errCodeInvalidRequest,
				message: capitalizeFirst(refusal.Error()) + ".",
			}
		}
	}
	logIgnoredRequestOptions(token, options)

	// A credential GitLab accepted and refused the permission to read its own
	// user, or one that carries neither read_api nor api, is answered from
	// memory: it builds no entry and is charged nothing, so without this every
	// request carrying it would hold one of the pool's probe slots for another
	// GET /user with the same answer.
	if kind, sentence, known := g.knownDoorRefusal(options.GitLabURL, token); known {
		if kind == oauth.RejectionPermissionMissing {
			logDoorPermissionRefusal(r.Context(), "request rejected at the gate: token already known to lack the permission to read its own user", sentence)
			if g.guard != nil {
				return nil, g.guard.permissionMissingFailure(sentence)
			}
			return nil, doorPermissionFailure(sentence)
		}
		refusalLog.log(r.Context(), slog.LevelInfo, "request rejected at the gate: token already known to carry neither read_api nor api",
			telemetry.LogFieldCredentialHash, serverpool.CredentialHash(token))
		if g.guard != nil {
			return nil, g.guard.insufficientScopeFailure()
		}
		return nil, belowMinimumFailure()
	}

	entry, err := g.pool.GetOrCreateEntryWithFacts(token, options.GitLabURL, verifiedFacts(r)) //nolint:contextcheck // the pool bounds per-token scope detection with its own timeout, deliberately outliving this request
	if err != nil {
		return nil, g.classify(r.Context(), err, ip, source, options.GitLabURL, token)
	}
	return entry, nil
}

// classify turns the pool's refusal to build an entry into the gate's answer,
// as the bearer guard's classify does with the verifier's: a credential GitLab
// rejected, one it accepted and refused at the door, or one nothing judged.
// Only the first is the caller's doing and charged; the other two are
// remembered or retried, never counted against the address.
func (g *mcpServerGate) classify(ctx context.Context, err error, ip, source, instance, token string) *gateFailure {
	if errors.Is(err, serverpool.ErrCredentialBelowMinimum) {
		// GitLab accepted the token, and it carries neither read_api nor api:
		// the probe was refused for want of a scope, or the token's own
		// description named only scopes below the minimum. Not charged
		// (INV-007), for the reason the permission refusal below is not, and
		// remembered, since a token's scopes cannot change after it is
		// created (issue 952).
		if g.rejected != nil {
			g.rejected.RecordBelowMinimum(instance, token)
		}
		refusalLog.log(ctx, slog.LevelInfo, "request rejected at the gate: gitlab accepted the token, which carries neither read_api nor api",
			telemetry.LogFieldCredentialHash, serverpool.CredentialHash(token))
		if g.guard != nil {
			return g.guard.insufficientScopeFailure()
		}
		return belowMinimumFailure()
	}
	if missing, isMissing := errors.AsType[*serverpool.PermissionMissingError](err); isMissing {
		// GitLab accepted the token and judged its fine-grained grant, which
		// lacks User: Read. Not charged (INV-007): the credential is genuine,
		// and charging it would let its holder lock their own address out.
		// Remembered, so the next request with it asks GitLab nothing.
		sentence := oauth.QuotedDescription(missing.Description)
		if g.rejected != nil {
			g.rejected.RecordPermissionMissing(instance, token, missing.Description)
		}
		logDoorPermissionRefusal(ctx, "request rejected at the gate: gitlab accepted the token and refused it the permission to read its own user", sentence)
		if g.guard != nil {
			return g.guard.permissionMissingFailure(sentence)
		}
		return doorPermissionFailure(sentence)
	}
	if errors.Is(err, serverpool.ErrInvalidCredential) {
		// GitLab itself rejected the token, so this is an authentication
		// failure in the full sense: 401, and it does count against the
		// limiter. This is the path that stops a stream of invented tokens
		// from churning the pool.
		g.chargeFailure(ip, source, token)
		refusalLog.log(ctx, slog.LevelInfo, "request rejected: gitlab rejected the supplied token",
			telemetry.LogFieldCredentialHash, serverpool.CredentialHash(token))
		return &gateFailure{
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
	// Deliberately not charged to the authentication limiter. Any other pool
	// failure means the backend could not be reached or the server could not
	// be built: the credential was never judged. Counting it would let a
	// GitLab outage lock out clients holding valid tokens, which is the same
	// conflation of causes this gate exists to remove.
	slog.ErrorContext(ctx, "failed to create server for token", "error", err)
	// The pool error can name internal state, so it is logged but not
	// returned to the caller.
	return &gateFailure{
		status:  http.StatusServiceUnavailable,
		code:    errCodeUpstreamUnavailable,
		message: "Could not initialize a GitLab session for this token. The instance may be unreachable; retry shortly.",
	}
}

// credentialAlreadyAdmitted reports whether this request carries a credential
// the pool is already serving, which is what exempts it from an address block.
//
// It is the whole of the exemption, and it is deliberately narrow: an entry
// exists only because GitLab accepted that credential, and the lookup is a hash
// and a map read that reaches no network. So a request it admits costs the
// deployment nothing upstream, however much its address has spent — while every
// credential the pool does not already hold is still refused, which is exactly
// the set a token sprayer can produce.
//
// Both halves of the pool key are needed, so the instance is resolved here.
// The resolver is a pure function of the request and the published allow-list,
// and [mcpServerGate.resolve] runs it again below for the request proper, so
// the two cannot disagree; a request it refuses — an unpublished instance, or
// none named where the deployment publishes several — stays blocked.
//
// What this decides is the block and nothing else. A request it exempts carries
// on through the whole of [mcpServerGate.resolve], so the instance checks, the
// caller-named-destination guard and the pool lookup all still run, and the
// only step it skips is the 429.
func (g *mcpServerGate) credentialAlreadyAdmitted(r *http.Request) bool {
	if g.pool == nil {
		return false
	}
	token := g.extractCredential(r)
	if token == "" {
		return false
	}
	options, err := serverpool.ResolveRequestOptionsFor(r, g.gitlabURLs)
	if err != nil {
		return false
	}
	return g.pool.Admitted(token, options.GitLabURL)
}

// blockedByBudget reports whether any budget is exhausted: the caller's key,
// the transport source it arrived from, the distinct credentials that key has
// had refused, or the /48 the caller's address lies in. The duration is what
// Retry-After should say.
//
// address is the client address [clientIP] returned, and the keys are read off
// it here ([budgetKey], [networkKey]); source is already keyed.
//
// It is a duration rather than a bare bool because the escalating budgets'
// blocks outlast the window the other two are bounded by, and a 429 that
// announces the short one is telling the caller to come back while it is still
// refused. A well-behaved client then knocks for the rest of the block.
// The reason is what telemetry records the refusal under, and is the only
// thing recorded about it: an address is who was refused, which a counter must
// not carry.
func (g *mcpServerGate) blockedByBudget(address, source string) (blocked bool, retryAfter time.Duration, reason string) {
	key := budgetKey(address)
	lockout := authBlock{reason: mcpotel.AuthBlockFailureLockout}
	if g.limiter != nil {
		lockout.active, lockout.remaining = g.limiter.BlockedFor(key)
	}
	sourceBlock := authBlock{reason: mcpotel.AuthBlockTransportSource}
	sourceBlock.active, sourceBlock.remaining = g.sourceBudget.blockedFor(source)
	spray := authBlock{reason: mcpotel.AuthBlockDistinctTokens}
	spray.active, spray.remaining = g.spray.Blocked(key)
	network := authBlock{reason: mcpotel.AuthBlockNetworkAggregate}
	network.active, network.remaining = g.network.Blocked(networkKey(address))
	return longestAuthBlock(lockout, sourceBlock, spray, network)
}

// authBlock is one budget's answer about a request: whether it holds the
// request, for how much longer, and the reason telemetry records a refusal it
// made under.
type authBlock struct {
	active    bool
	remaining time.Duration
	reason    string
}

// longestAuthBlock picks the block a refused request should be told about when
// more than one budget holds it.
//
// Answering with the first one found is what both guards used to do, and it
// understates whenever a shorter block is checked first: one failure can raise
// the minute-long failure lockout and the hour-long distinct-token block
// together, and a client told to come back in a minute spends the other
// fifty-nine being refused. Retry-After is a promise about when the next
// attempt can succeed, so the longest active block is the only honest answer,
// and its reason is the one worth recording.
//
// A later block only wins on a strict improvement, so the order the guards
// pass them in breaks a tie: the lockout is the cheapest to explain, then the
// transport source, the distinct-token block and the /48 aggregate, the most
// specific last.
func longestAuthBlock(blocks ...authBlock) (blocked bool, retryAfter time.Duration, reason string) {
	for _, b := range blocks {
		if !b.active || b.remaining < retryAfter || (blocked && b.remaining == retryAfter) {
			continue
		}
		blocked, retryAfter, reason = true, b.remaining, b.reason
	}
	return blocked, retryAfter, reason
}

// chargeFailure charges one authentication failure to every budget.
//
// address is the client address [clientIP] returned, and its keys are read off
// it here, as [mcpServerGate.blockedByBudget] reads them; source is already
// keyed. token is the credential that was refused, and is empty for a request
// that carried none. The distinct-token budget ignores an empty one, and the
// /48 aggregate is not charged for one, so a client that simply forgot its
// header moves neither: only a credential GitLab refused does, which is the
// thing a sprayer cannot avoid producing.
func (g *mcpServerGate) chargeFailure(address, source, token string) {
	key := budgetKey(address)
	if g.limiter != nil {
		g.limiter.RecordFailure(key)
	}
	g.sourceBudget.charge(source, key)
	g.spray.Charge(key, token)
	if token != "" {
		g.network.Charge(networkKey(address), key)
	}
}

// transportBudget is the secondary authentication budget of
// [transportFailureLimit], charged to the transport source.
//
// It charges once per distinct primary key inside a window rather than once
// per failure, and that distinction is the whole design. The budget exists to
// bound header rotation, which mints a fresh primary key on every request; a
// genuine proxy's clients keep theirs, and a client whose key is already known
// is already bounded by its own ten-a-minute allowance. Counting failures
// instead aggregated the fleet: fifty clients failing their own ten times each
// spent all five hundred between them, and since both gates consult the budget
// before the credential is read, the next request through that proxy was
// refused whatever it carried, including a valid token, and including the
// clients that had never failed at all.
//
// What it remembers to charge that way is bounded, because both halves of a
// pair are the caller's to choose: the source by reaching the listener from an
// IPv6 range, each /64 of which is a source of its own, and the key by the
// header a trusted proxy forwards. A source holds at most its limit's worth of
// pairs (register row AUB-002, issue 982). A pair is kept only for a failure
// the limiter recorded, and once the budget indexes as many sources as the
// limiter's table can hold (AUB-004), a new source first makes it forget every
// source none of whose pairs is live, at most once every eighth of a window.
// Past that many, the sources it holds were therefore all charged within one
// span of a window and an eighth, each while the limiter held a record of it,
// and a record stays in that table for a window at least, so they are at most
// three times what the table can hold, whatever the window. The sweep alone
// used to bound the pairs: a blocked source adds none, but a source arriving
// once the limiter's table is full is never recorded and so never blocked, and
// the budget kept a pair for every key it failed with. A source the limiter
// recorded and then let go kept its pairs until the sweep too, while the table
// took a new source in its place every window.
//
// The zero value is not usable; a nil *transportBudget is, and is what a
// deployment without --trusted-proxy-header gets. Every method tolerates it.
type transportBudget struct {
	limiter *serverpool.AuthRateLimiter
	// effectiveWindow is the window the limiter above was built with, which
	// is the configured one or the default when that was zero or less. It is
	// held here because charge and cleanup below have to lapse a pair on the
	// same schedule the limiter forgets a failure on: deduplicating for longer
	// than the limiter remembers undercharges a rotating source, and
	// deduplicating for less recharges one pair inside a single window and
	// blocks a proxy's legitimate clients early.
	effectiveWindow time.Duration
	// pairsPerSource is how many pairs one source may hold, the limiter's own
	// limit: a source charged that many distinct keys is blocked, so a pair
	// past them is one nothing would learn from.
	pairsPerSource int

	mu sync.Mutex
	// charged records when a (source, key) pair last opened a window, so a
	// key already counted against a source is not counted again until its
	// window lapses. It is keyed on the source and the key joined by a NUL,
	// so that two pairs whose halves concatenate alike stay apart.
	charged map[string]time.Time
	// bySource holds, for each source with a pair in charged, the pairs it
	// holds there and when it was last charged. That is what bounds a
	// source's share of the map, finds its oldest pair when it is full, and
	// finds the sources with no live pair without reading their pairs. A
	// source leaves it once its last pair has.
	bySource map[string]*sourcePairs
	// lastForgetAt is when the sources with no live pair were last forgotten
	// on the way to indexing a new one, which happens at most once every
	// eighth of a window ([transportBudget.forgetLapsedSourcesLocked]).
	lastForgetAt time.Time
}

// sourcePairs is one source's share of the transport-source budget's record.
type sourcePairs struct {
	// pairs are the source's entries in the budget's charged map.
	pairs map[string]struct{}
	// lastAt is when the source was last charged. None of its pairs was
	// charged later, so once lastAt has lapsed none of them is live.
	lastAt time.Time
}

// newTransportBudget wraps a limiter in the per-key accounting above.
//
// window is the one the limiter was built with, not the one the operator
// configured: the two differ when the configured value was zero or less, which
// [transportFailureBudget] resolves to the default for both, and the
// accounting here has to follow the limiter rather than the configuration.
func newTransportBudget(limiter *serverpool.AuthRateLimiter, window time.Duration) *transportBudget {
	if limiter == nil {
		return nil
	}
	return &transportBudget{
		limiter:         limiter,
		effectiveWindow: window,
		pairsPerSource:  limiter.Limit(),
		charged:         make(map[string]time.Time),
		bySource:        make(map[string]*sourcePairs),
	}
}

// window is the effective window this budget lapses pairs on, for the guards
// that answer Retry-After with it. A nil budget reports the default, which is
// what its own limiter would have used. A budget that exists was built from
// the window [transportFailureBudget] resolved, a configured zero or less
// already the default, so its own window is the answer as it stands.
func (b *transportBudget) window() time.Duration {
	if b == nil {
		return authFailureWindow
	}
	return b.effectiveWindow
}

// rateLimiter is the underlying limiter, for the layer that shares this budget
// rather than owning it.
func (b *transportBudget) rateLimiter() *serverpool.AuthRateLimiter {
	if b == nil {
		return nil
	}
	return b.limiter
}

// blockedFor reports whether a transport source has minted more distinct
// primary keys than its budget allows, and how much of its block is left,
// which is what a refusal answers Retry-After with.
func (b *transportBudget) blockedFor(source string) (bool, time.Duration) {
	if b == nil {
		return false, 0
	}
	return b.limiter.BlockedFor(source)
}

// charge counts one failure against the source, unless this key has already
// been counted against it inside the current window.
//
// The pair is kept only when the limiter recorded the failure, which it does
// not for a new source once its table is full. Kept then, it would charge
// nothing and grow with every key the unrecorded source fails with, and it
// would stand in for a charge that never happened: the same key failing again
// once the limiter had room would not be counted. Not kept, the next failure
// is charged as a first one (INV-007 holds, since the caller caused it). The
// limiter is asked under the budget's own lock, so two concurrent failures of
// one fresh pair are charged once, as they were when the pair was stored
// before the limiter was asked.
func (b *transportBudget) charge(source, key string) {
	if b == nil {
		return
	}
	pair := source + "\x00" + key
	b.mu.Lock()
	defer b.mu.Unlock()
	if first, seen := b.charged[pair]; seen && time.Since(first) <= b.effectiveWindow {
		return
	}
	if !b.limiter.RecordFailure(source) {
		return
	}
	b.keepLocked(source, pair)
}

// keepLocked remembers that pair was charged now, inside its source's bound.
//
// A pair the source already holds is re-opened in place, which is not growth.
// A fresh one at the bound takes the place of the source's own oldest pair
// (issue 982), never another source's, since a holding of one key is not taken
// to admit another (INV-005); stopping at the bound instead would let a source
// just released from its block fail with fresh keys uncharged until its old
// pairs lapse. A source the budget does not index yet is a new one, and may
// first make it forget the sources with no live pair. The caller holds b.mu.
func (b *transportBudget) keepLocked(source, pair string) {
	now := time.Now()
	held, tracked := b.bySource[source]
	if !tracked {
		b.forgetLapsedSourcesLocked(now)
		held = &sourcePairs{pairs: make(map[string]struct{})}
		b.bySource[source] = held
	}
	if _, kept := held.pairs[pair]; !kept && len(held.pairs) >= b.pairsPerSource {
		b.reclaimOldestLocked(held.pairs)
	}
	held.pairs[pair] = struct{}{}
	held.lastAt = now
	b.charged[pair] = now
}

// forgetLapsedSourcesLocked forgets every source none of whose pairs is live,
// with its pairs, once the index holds as many sources as the limiter's table
// can (issue 982).
//
// The limiter's full table lets a lapsed record go at most an eighth of a
// window after it lapses and takes a new source in its place, while the sweep
// forgets a lapsed pair only every few minutes (AUB-005), so each table's
// worth of sources the limiter took in between used to stay beside the next,
// and the shorter the window the more of them. Forgetting runs at most once
// every eighth of a window, as the limiter's own sweep at its cap does, so that
// a flood of new sources cannot make each of them pay for a pass over the
// index, and the pass reads each source's last charge rather than its pairs.
// The caller holds b.mu.
func (b *transportBudget) forgetLapsedSourcesLocked(now time.Time) {
	if len(b.bySource) < transportTrackedSources || now.Sub(b.lastForgetAt) < b.effectiveWindow/8 {
		return
	}
	b.lastForgetAt = now
	for source, held := range b.bySource {
		if now.Sub(held.lastAt) > b.effectiveWindow {
			for pair := range held.pairs {
				delete(b.charged, pair)
			}
			delete(b.bySource, source)
		}
	}
}

// reclaimOldestLocked forgets the oldest of one source's pairs, to make room
// for the one about to be kept. It scans the source's pairs, which are at most
// its limit, and only a source at its bound pays for that: one never blocked
// whose lapsed pairs the sweep (AUB-005) has not reached yet, one whose block
// has lifted while the pairs that earned it are still held, live or lapsed and
// not yet swept, or one charged by a request that passed the gates before the
// block. The caller holds b.mu.
func (b *transportBudget) reclaimOldestLocked(pairs map[string]struct{}) {
	oldest := ""
	var oldestAt time.Time
	for pair := range pairs {
		if at := b.charged[pair]; oldest == "" || at.Before(oldestAt) {
			oldest, oldestAt = pair, at
		}
	}
	delete(pairs, oldest)
	delete(b.charged, oldest)
}

// cleanup drops lapsed pairs, every source left without one, and the limiter's
// own expired records. It stands in for the limiter's Cleanup wherever a
// budget owns one.
func (b *transportBudget) cleanup() {
	if b == nil {
		return
	}
	b.mu.Lock()
	now := time.Now()
	for source, held := range b.bySource {
		for pair := range held.pairs {
			if now.Sub(b.charged[pair]) > b.effectiveWindow {
				delete(held.pairs, pair)
				delete(b.charged, pair)
			}
		}
		if len(held.pairs) == 0 {
			delete(b.bySource, source)
		}
	}
	b.mu.Unlock()
	b.limiter.Cleanup()
}

// transportSource is the key of the address the connection actually came
// from.
//
// It is [clientIP] with every header ignored, and it is the whole point of the
// secondary budget: RemoteAddr is set by the kernel from the accepted socket,
// so nothing a caller sends can change it. It is keyed as every budget's
// address is ([budgetKey]): a proxy reaching this server over IPv6 is its /64,
// which rotating its own low bits does not change, and a connection on a unix
// socket is [unixPeer] (issue 954).
func transportSource(r *http.Request) string {
	return budgetKey(remoteHost(r))
}

// errMissingGitLabURL is returned when a deployment publishes several
// instances and the request named none of them.
var errMissingGitLabURL = errors.New("no GitLab instance was selected")

// requireExplicitInstance refuses to choose an instance on the caller's
// behalf when the deployment publishes more than one.
//
// The resolver refuses the same request (serverpool.UnnamedInstanceError), so
// this is not the only place the decision is made; it is the place it is made
// before the credential is judged, and the place that knows which wording the
// auth mode allows, since the published set is public in oauth mode and not in
// legacy mode. The resolver's older answer was the first published instance,
// on the reasoning that a client which does not care keeps working. That holds
// for a client which does not care, and not for one that cares and failed to say so:
// a proxy stripped the header, a client library cannot set custom headers, or
// the configuration was copied wrong. The credential is then transmitted in
// full to an instance its holder never named. In oauth mode this is worse
// rather than better, because the verifier uses the same resolution, so the
// bearer is POSTed to instance A's /api/v4/user before anything notices it
// belongs to instance B — and both instances are advertised in the RFC 9728
// authorization_servers array, while neither RFC 9728 nor MCP gives a client
// an in-band way to say which one it used.
//
// A single published instance is unaffected: there is nothing to choose, and a
// header naming something else is recorded as ignored exactly as before.
// Do not "fix" the multi-instance case by verifying against each instance in
// turn, which broadcasts the credential to all of them.
func requireExplicitInstance(r *http.Request, instances []string) error {
	if len(instances) < 2 {
		return nil
	}
	if strings.TrimSpace(r.Header.Get(serverpool.RequestOptionGitLabURL)) != "" {
		return nil
	}
	return errMissingGitLabURL
}

// missingURLMessage tells a caller which header to set, naming the published
// instances only where that list is already public.
//
// The set is served unauthenticated as RFC 9728 authorization_servers in oauth
// mode, so naming it there gives nothing away and saves a client a guess.
// Legacy mode publishes no metadata document and this rejection is reached
// before the credential is judged, so echoing the list there would let any
// non-empty token enumerate the operator's instance hostnames.
func (g *mcpServerGate) missingURLMessage() string {
	base := "This deployment serves several GitLab instances, so the " +
		serverpool.RequestOptionGitLabURL + " header must name the one this request is for."
	if !g.oauthMode {
		return base + " Ask the operator which instances it publishes."
	}
	return base + " It publishes: " + strings.Join(g.gitlabURLs, ", ") + "."
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

// knownDoorRefusal reports whether this deployment already knows GitLab
// accepted this credential and this door refuses it uncharged, and which way:
// refused the permission to read its own user, with the sentence it was
// refused with as it was quoted then ([oauth.RejectionPermissionMissing]), or
// carrying neither read_api nor api, recorded by this gate or by the pool once
// it learned that of a credential it served ([oauth.RejectionBelowMinimum]).
// Any other cached refusal, a GitLab rejection included, is not known here:
// only the bearer guard in front answers that one from memory.
func (g *mcpServerGate) knownDoorRefusal(instance, token string) (kind oauth.RejectionKind, sentence string, known bool) {
	if g.rejected == nil {
		return kind, "", false
	}
	kind, sentence, known = g.rejected.LookupRefusal(instance, token)
	return kind, sentence, known && (kind == oauth.RejectionPermissionMissing || kind == oauth.RejectionBelowMinimum)
}

// belowMinimumMessage is the whole refusal the legacy gate gives a credential
// GitLab accepted that carries neither read_api nor api (register rows ADM-001
// and ADM-006), the bearer guard's own answer in OAuth mode aside. It says a
// new token rather than a changed one because GitLab has no route that edits a
// personal access token's scopes, and an OAuth token granted more is a new
// token too. It is one literal rather than a concatenation, for the reason
// [doorPermissionAdvice] gives.
const belowMinimumMessage = "GitLab accepted this token, which carries neither the read_api nor the api scope this server needs at least. A token's scopes cannot be changed after it is created: create one with read_api, or with api to write as well. The token itself is valid."

// belowMinimumFailure is the 403 a credential below the admission minimum is
// given, with no challenge, the legacy gate's shape for a genuine credential
// it refuses (the permission refusal's too), and uncharged.
func belowMinimumFailure() *gateFailure {
	return &gateFailure{
		status:  http.StatusForbidden,
		code:    errCodeForbidden,
		message: belowMinimumMessage,
	}
}

// doorPermissionPrefix is the stable leading text of the refusal both doors
// give a credential GitLab accepted and refused the permission to read its own
// user (register rows ADM-001, ADM-002 and ADM-006).
const doorPermissionPrefix = "GitLab accepted this token and refused it the permission to read its own user."

// doorPermissionAdvice follows the prefix: why the door needs the permission,
// and the two ways out. It says a new token rather than a changed one because
// nothing at GitLab 19.4 edits a fine-grained grant after its token is created,
// and it qualifies the classic token as every way out this server offers does:
// an instance that requires fine-grained tokens no longer lets anyone create a
// classic one, and a group that requires them refuses one.
// It is one literal rather than a concatenation: a constant has no statement to
// cover, so the mutation gate reports every operator of a concatenation as a
// mutant nothing can reach.
const doorPermissionAdvice = " This server identifies every caller with GET /api/v4/user, which a fine-grained personal access token reaches only when it grants User: Read, and a token's grant cannot be changed after it is created: create a fine-grained token that grants User: Read, or use a classic token with the read_api or api scope (an existing one, on an instance that no longer lets you create them), where the group does not refuse classic tokens. The token itself is valid."

// doorPermissionChallengeDescription is the error_description the bearer
// guard's challenge carries for the same refusal. It is this server's constant
// and never GitLab's sentence: RFC 6749 section 5.2 allows error_description a
// narrower set of characters than a JSON body, and the sentence is the
// instance's, which under --allow-any-gitlab-url is the caller's own.
const doorPermissionChallengeDescription = "GitLab refused this token the permission to read its own user"

// doorPermissionDetail is everything after [doorPermissionPrefix]: the advice,
// and GitLab's own sentence when there is one, already bounded and filtered
// by [oauth.QuotedDescription]. GitLab's sentence is what tells the holder
// which of its refusals this was: the missing permission, or fine-grained
// tokens not yet enabled for the user.
func doorPermissionDetail(sentence string) string {
	if sentence == "" {
		return doorPermissionAdvice
	}
	return doorPermissionAdvice + " GitLab said: " + sentence
}

// doorPermissionFailure is the legacy gate's 403 for a credential GitLab
// accepted and refused the permission to read its own user. It carries no
// challenge, as no other legacy refusal but a 401 does: the challenge legacy
// mode sends names no error and no authorization server a client could act on.
func doorPermissionFailure(sentence string) *gateFailure {
	return &gateFailure{
		status:  http.StatusForbidden,
		code:    errCodeForbidden,
		message: doorPermissionPrefix + doorPermissionDetail(sentence),
	}
}

// maxLoggedPermissions and maxLoggedPermissionBytes bound what a door's log
// line names of the permissions GitLab's sentence lists: how many, and how
// long each may be. The sentence is the instance's, so its list is too.
const (
	maxLoggedPermissions     = 3
	maxLoggedPermissionBytes = 64
)

// logDoorPermissionRefusal writes the operator's line for a permission refusal
// at either door: the number of permissions GitLab's sentence lists and at
// most three of them, each cut short, and never the sentence, the token or
// anything else about the caller (VAL-010).
func logDoorPermissionRefusal(ctx context.Context, message, sentence string) {
	count, named := sentencePermissions(sentence)
	refusalLog.log(ctx, slog.LevelInfo, message, "permissions", count, "named", strings.Join(named, "; "))
}

// sentencePermissions reads the bracketed list GitLab's refusal of a missing
// permission ends with ("... permissions: [User: Read, Project: Read]."), and
// returns how many names it holds and the first [maxLoggedPermissions] of
// them, each cut at [maxLoggedPermissionBytes]. A sentence without a list,
// GitLab's other refusals among them, names none. The sentence has already
// been through [oauth.QuotedDescription], so it is ASCII and a cut splits no
// rune.
func sentencePermissions(sentence string) (count int, named []string) {
	_, bracketed, opened := strings.CutLast(sentence, "[")
	if !opened {
		return 0, nil
	}
	list, _, closed := strings.Cut(bracketed, "]")
	if !closed {
		return 0, nil
	}
	for name := range strings.SplitSeq(list, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		count++
		if len(named) < maxLoggedPermissions {
			named = append(named, name[:min(len(name), maxLoggedPermissionBytes)])
		}
	}
	return count, named
}
