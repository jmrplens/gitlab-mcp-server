package main

import (
	"context"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/serverpool"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// credentialState is everything one pooled credential owns on a server it
// shares with every other credential of its configuration shape.
//
// Until this existed, each of these lived on the server: the client its
// handlers captured, the rate-limit bucket its middleware closed over, the
// watchers its subscription manager held, the counter its listen ceiling drew
// on. One server per credential made that correct by construction and cost a
// full registered catalog per credential. Sharing the server moves the
// per-credential half here, and the binding middleware is what puts it back in
// front of a handler.
type credentialState struct {
	// owner is the pool entry's opaque token. It is what a resource-updated
	// notification is stamped with and what a session is recorded under, so
	// that a shared server can tell whose is whose.
	owner string
	// client carries this credential, and with it the instance. It is the one
	// thing that genuinely differs between two entries of one configuration
	// shape, since the shape key deliberately leaves the instance URL out. On
	// the fallback state of a shape server it is the unbound client, which
	// refuses every request.
	client *gitlabclient.Client
	// limiter is this credential's own token bucket, or nil when the deployment
	// runs unlimited.
	limiter *toolutil.RateLimiter
	// listen counts this credential's open subscriptions/listen streams.
	listen *listenCounter
	// subs holds this credential's watchers, or nil on a capability surface
	// that offers no subscriptions.
	subs *subscriptionRuntime
	// streams is the shared server's registry of open subscriptions/listen
	// requests, which this credential's own are ended through when the pool
	// drops it. It is the shape's registry rather than a per-credential one
	// because a listen belongs to a session on this server, not to a watcher;
	// each stream records its owner.
	streams *listenStreams
	// sessions is the shared server's record of which credential each session
	// belongs to. Eviction reads it to end the sessions a session-era
	// resources/subscribe left holding nothing, which no stream can reach. Nil
	// on stdio and on a server built without one.
	sessions *sessionOwners
}

// close releases what the entry owns, for a credential the pool has evicted.
//
// It stops the watchers rather than letting them run on. While each credential
// had a server of its own, eviction dropped only the pool's reference and a
// live session kept working; on a shared server it cannot, because the same
// eviction forgets which credential that session belongs to and every later
// notification for it would be filtered away in silence.
//
// Stopping the watchers is not by itself the ending the specification asks for,
// and believing it was is what left an evicted client silent. Manager.Close is
// the one stop path that fires no OnStop, by contract and deliberately: the
// three endings it does announce are ones the subscriber did not ask for, and a
// manager being closed used to mean its whole server was going away. So nothing
// reached [listenStreams.stoppedFor], and the open subscriptions/listen was
// neither closed nor completed. Ending this credential's streams here is what
// turns eviction back into an ending the client is told about: each stream gets
// its completion result and the next request re-initializes.
//
// [github.com/jmrplens/gitlab-mcp-server/v2/internal/subscriptions.Manager.Close]
// waits for every watcher goroutine to unwind, so it must never run on the
// caller's goroutine here: eviction happens under the pool's write lock, which
// the callback contract forbids blocking. The streams are ended on that same
// goroutine, after the watchers, so a poll that is still in flight cannot
// notify a stream that has already been told it is over.
//
// A stream is not the only thing a subscriber can hold. On --stateless=false a
// session-era resources/subscribe leaves no open request at all, so
// [listenStreams.closeOwner] reaches nothing and that client was left with a
// live session and a standalone SSE stream that would never carry anything
// again. The sessions the eviction has just orphaned are therefore terminated
// too, minus the ones a stream has already ended gracefully. See
// [sessionOwners.endSessionsWithoutStreams].
//
// end is what the streams are told the ending was, which only the caller knows:
// the pool names a cause and [watchEndForCause] turns it into the reason the
// client reads. A nil end still ends everything, and is what an eviction path
// this server has no reason for produces.
func (s *credentialState) close(orphaned []*mcp.ServerSession, end *watchEnd) {
	if s == nil {
		return
	}
	subs, streams, sessions, owner := s.subs, s.streams, s.sessions, s.owner
	go func() {
		subs.close()
		told := streams.closeOwner(owner, end)
		sessions.endSessionsWithoutStreams(orphaned, told)
	}()
}

// busy reports whether this credential is doing work the pool cannot see.
//
// A watcher polls GitLab on its own, and an open subscriptions/listen is a
// request the client is holding rather than one it repeats, so neither refreshes
// the pool entry that owns them. Idle eviction asks this before dropping an
// entry, which is what stops a client that subscribed and then waited from being
// evicted for waiting. See [github.com/jmrplens/gitlab-mcp-server/v2/internal/serverpool.WithInUse].
func (s *credentialState) busy() bool {
	if s == nil {
		return false
	}
	if s.listen.count() > 0 {
		return true
	}
	return s.subs != nil && s.subs.manager.Len() > 0
}

// credentialStateKey carries the pool entry a request belongs to.
type credentialStateKey struct{}

// withCredentialState returns ctx carrying the credential a request runs under,
// and the GitLab client that credential holds.
//
// The client is installed in the same call rather than left to each reader,
// because those two facts must never disagree: a handler resolving its client
// through [gitlabclient.Client.For] and a middleware resolving the rate-limit
// bucket through [credentialStateFrom] have to be talking about the same
// tenant.
func withCredentialState(ctx context.Context, state *credentialState) context.Context {
	if state == nil {
		return ctx
	}
	return gitlabclient.WithClient(context.WithValue(ctx, credentialStateKey{}, state), state.client)
}

// credentialStateFrom returns the credential a request belongs to, or nil when
// nothing bound one.
func credentialStateFrom(ctx context.Context) *credentialState {
	state, _ := ctx.Value(credentialStateKey{}).(*credentialState)
	return state
}

// ownerOfRequest returns the pool entry a request belongs to, or "" when
// nothing bound one.
func ownerOfRequest(ctx context.Context) string {
	if state := credentialStateFrom(ctx); state != nil {
		return state.owner
	}
	return ""
}

// credentialStates maps a pool entry to the state it owns on the server it
// shares.
//
// It is keyed by the entry's owner token rather than by the entry pointer so
// that the notification filter, which only ever sees the token, and the request
// path, which sees the entry, agree without a second lookup table. The pool's
// insert and evict callbacks are what keep it in step with the pool, so it can
// never outgrow --max-http-clients.
type credentialStates struct {
	states sync.Map // owner string -> *credentialState
}

// add records a freshly built entry's state.
func (c *credentialStates) add(state *credentialState) {
	if state == nil || state.owner == "" {
		return
	}
	c.states.Store(state.owner, state)
}

// get returns the state of an entry, or nil when the pool no longer holds it.
func (c *credentialStates) get(owner string) *credentialState {
	if c == nil || owner == "" {
		return nil
	}
	state, ok := c.states.Load(owner)
	if !ok {
		return nil
	}
	typed, _ := state.(*credentialState)
	return typed
}

// inUse answers the pool's idle sweep: an entry whose credential still has work
// running is not idle, whatever its timestamp says.
//
// It is the registry rather than the entry that is asked, because the work in
// question (watchers, open listen streams) belongs to the per-credential state
// this holds and the pool knows nothing about it. An entry this registry has
// never heard of, or one already removed, is idle by this measure and the pool's
// own timestamp decides.
//
// It runs under the pool's write lock, so it reads two counters and nothing
// more. See [github.com/jmrplens/gitlab-mcp-server/v2/internal/serverpool.WithInUse].
func (c *credentialStates) inUse(entry *serverpool.Entry) bool {
	return c.get(entry.Owner()).busy()
}

// remove drops an evicted entry's state and releases what it held.
//
// orphaned is the credential's sessions, which the caller has already taken out
// of the ownership record: the record has to stop answering for them under the
// pool's write lock, while telling their clients must not happen there, so the
// two halves are split and the list travels between them.
//
// end is why, which travels the same way and for the same reason: the pool is
// the only thing that knows the cause, and the client is told about it well
// after the lock has been released.
func (c *credentialStates) remove(owner string, orphaned []*mcp.ServerSession, end *watchEnd) {
	if owner == "" {
		return
	}
	state, ok := c.states.LoadAndDelete(owner)
	if !ok {
		return
	}
	typed, _ := state.(*credentialState)
	typed.close(orphaned, end)
}

// bindCredential runs each MCP request under the credential the POST carrying
// it was authenticated as.
//
// The channel is the carrier header, for the reason [requestCarriers] already
// records: it is the only per-request value the SDK exposes on both transports,
// and a context value would be right in stateless mode and wrong in stateful
// mode, where the session is connected with the initialize POST's context.
// [mcpServerGate] resolves the pool entry and puts its state on the HTTP
// request context; the carrier registry maps the header's token back to that
// context; this reads it out and installs it where handlers and middlewares
// look.
//
// A request with no token, or one whose carrier is already gone, is left
// unbound. That is stdio, the in-memory transport the e2e suite drives, and
// every test that builds a server directly: there the server's own client is
// the answer, and [gitlabclient.Client.For] returns it. On a shape server the
// same absence means the request fails closed, because the client it falls back
// to refuses everything.
//
// It is added LAST of the receiving middlewares that need it, which makes it
// run FIRST: the telemetry, rate-limit and subscription middlewares all read
// what it installs, and a binding applied inside them would leave each of them
// answering for the wrong tenant, or for none.
func (c *credentialStates) bindCredential(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		// The lookup reads a value off the HTTP request's context, which is a
		// different context from the handler's and deliberately so: the handler
		// context does not descend from the request, which is the whole reason
		// the carrier exists. Only the value crosses over, never the context,
		// so nothing of the request's lifetime or cancellation comes with it.
		// That is what contextcheck objects to here, as it does at
		// [requestCarriers.bind], and it is the point.
		state := c.stateForRequest(req) //nolint:contextcheck // the carrier is a value channel, not a parent context
		if state == nil {
			return next(ctx, method, req)
		}
		return next(withCredentialState(ctx, state), method, req)
	}
}

// stateForRequest resolves the credential behind an incoming MCP request.
func (c *credentialStates) stateForRequest(req mcp.Request) *credentialState {
	token := carrierTokenOf(req)
	if token == "" {
		return nil
	}
	carrier := mcpCarriers.lookup(token)
	if carrier == nil {
		return nil
	}
	return credentialFromRequestContext(carrier)
}

// requestCredentialKey carries the resolved credential on the HTTP request
// context, from the gate to [credentialStates.bindCredential].
type requestCredentialKey struct{}

// withRequestCredential stamps the HTTP request context with the credential the
// gate resolved for it.
func withRequestCredential(ctx context.Context, state *credentialState) context.Context {
	if state == nil {
		return ctx
	}
	return context.WithValue(ctx, requestCredentialKey{}, state)
}

// credentialFromRequestContext reads back what [withRequestCredential] stamped.
func credentialFromRequestContext(ctx context.Context) *credentialState {
	state, _ := ctx.Value(requestCredentialKey{}).(*credentialState)
	return state
}

// newCredentialState builds the per-credential half of a shared server for one
// pool entry.
//
// The shell supplies what the shape decides (the subscription machinery, the
// configured rate) and the entry supplies what the credential decides (its
// client and its owner token).
func (sh *serverShell) newCredentialState(entry *serverpool.Entry) *credentialState {
	cfg := entry.Config()
	if cfg == nil {
		cfg = sh.cfg
	}
	state := &credentialState{
		owner:    entry.Owner(),
		client:   entry.Client(),
		limiter:  toolutil.NewRateLimiter(cfg.RateLimitRPS, cfg.RateLimitBurst),
		listen:   &listenCounter{},
		subs:     sh.subs.newRuntime(entry.Owner(), entry.Client()),
		streams:  sh.streams,
		sessions: sh.sessions,
	}
	if state.subs != nil {
		state.subs.notifier.attach(sh.server)
	}
	return state
}
