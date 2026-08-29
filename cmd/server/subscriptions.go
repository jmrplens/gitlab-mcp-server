package main

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/resources"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/subscriptions"
)

// resourceReader reads a subscribed URI through the very handler the MCP
// router would dispatch to.
//
// Going through the registered handler rather than calling GitLab directly
// is what makes change detection mean something: "the content changed" has
// to mean "what resources/read returns changed", or a watcher would be
// comparing one thing and the subscriber reading another.
type resourceReader struct {
	index resources.HandlerIndex
}

func (r resourceReader) Read(ctx context.Context, uri string) ([]byte, error) {
	kind, ok := subscriptions.Classify(uri)
	if !ok {
		return nil, subscriptions.ErrNotSubscribable
	}
	content, err := r.index.Read(ctx, kind.Template(), uri)
	if err != nil {
		return nil, subscriptions.TranslateReadError(err)
	}
	return content, nil
}

// serverNotifier forwards a change to every session subscribed to a URI.
//
// The server pointer arrives late by necessity: mcp.ServerOptions carries
// the subscription handlers and must be built before mcp.NewServer returns
// the server those handlers notify through. It is stored atomically because
// watchers run on their own goroutines, even though in practice the pointer
// is set before the server accepts its first connection.
type serverNotifier struct {
	server atomic.Pointer[mcp.Server]
}

func (n *serverNotifier) attach(server *mcp.Server) {
	n.server.Store(server)
}

// watchMetaKey namespaces this server's watch state inside a notification's
// _meta. MCP reserves the io.modelcontextprotocol/ prefix for registered
// keys and defines none for subscription lifetime, so this is a vendor key
// under a domain this project controls.
const watchMetaKey = "io.github.jmrplens/watch"

func (n *serverNotifier) ResourceUpdated(ctx context.Context, update subscriptions.Update) error {
	server := n.server.Load()
	if server == nil {
		// Unreachable once attach has run; a watcher cannot exist before
		// the server does, since subscribing requires a live session.
		return nil
	}
	return server.ResourceUpdated(ctx, &mcp.ResourceUpdatedNotificationParams{
		URI:  update.URI,
		Meta: watchMeta(update),
	})
}

// watchMeta describes the watch that produced a notification.
//
// It is built fresh on every call, never cached: the SDK shallow-copies
// these params per subscribed session and stamps its own subscription ID
// into this same map, so a shared map would be overwritten per session and
// the mutations would leak back here.
//
// Nothing obliges a client to read any of it — no client known today does —
// and everything it says is true whether or not anyone looks: the state the
// watch is in, when it slows down, and how often it is reading. What it
// deliberately does not do is dress any of this up as a resource change:
// the notification means what the schema says it means, that the content
// changed and is worth reading again.
func watchMeta(update subscriptions.Update) mcp.Meta {
	state := "active"
	if update.Slow {
		state = "slow"
	}
	watch := map[string]any{
		"state":          state,
		"pollIntervalMs": update.Interval.Milliseconds(),
		// Any request on this session pushes the deadline out again, so a
		// client that is being used never has to do anything about it.
		"renewedByActivity": true,
	}
	if !update.RenewBy.IsZero() {
		watch["renewBy"] = update.RenewBy.UTC().Format(time.RFC3339)
	}
	return mcp.Meta{watchMetaKey: watch}
}

// sessionBridge ties a watch to the MCP session that asked for it.
//
// The session is the subscriber identity the manager counts by, which is
// what makes subscribing twice idempotent and stops one session's
// unsubscribe from touching another's watch. What the bridge adds on top is
// the ending the SDK does not report: a session that disconnects is dropped
// from the server's subscriber table without the unsubscribe handler ever
// firing, so a watch would otherwise outlive every client that could
// receive its notifications — and at least one shipping client never sends
// resources/unsubscribe at all.
type sessionBridge struct {
	manager *subscriptions.Manager[*mcp.ServerSession]

	mu      sync.Mutex
	awaited map[*mcp.ServerSession]struct{}
	// holds records which listen streams are holding each watch, so a watch
	// outlives the first stream to let go of it. See [sessionBridge.Unsubscribe].
	holds map[watchHold]map[*listenStream]struct{}
}

// watchHold identifies one watch as the manager counts it: by session and URI.
type watchHold struct {
	session *mcp.ServerSession
	uri     string
}

func newSessionBridge(manager *subscriptions.Manager[*mcp.ServerSession]) *sessionBridge {
	return &sessionBridge{
		manager: manager,
		awaited: make(map[*mcp.ServerSession]struct{}),
		holds:   make(map[watchHold]map[*listenStream]struct{}),
	}
}

// Subscribe starts or joins a watcher on behalf of one session.
func (b *sessionBridge) Subscribe(ctx context.Context, req *mcp.SubscribeRequest) error {
	// Arranged before the subscribe rather than after it, so a session that
	// disconnects while the first read is still in flight still has its
	// interest released.
	b.awaitEnd(req.Session)
	if err := wireSubscribeError(b.manager.Subscribe(ctx, req.Session, req.Params.URI)); err != nil {
		return err
	}
	b.hold(req.Session, req.Params.URI, listenStreamOf(ctx))
	b.renewWhileStreaming(ctx, req.Session)
	return nil
}

// renewWhileStreaming keeps a watch at full speed for as long as the
// subscriptions/listen request holding it stays open.
//
// The lease exists to answer "is anyone still there", and request traffic on
// the session is the evidence it normally uses. That evidence does not exist on
// the transport this server ships by default: every stateless POST is its own
// session, so a listen stream's session sees exactly one request — the listen
// itself — and nothing ever renews it. The watch would drop to the slow poll
// half an hour in and stay there, while the client sat on an open stream it was
// still reading.
//
// An open listen request is better evidence than traffic anyway: it is the
// subscriber, still connected, still waiting. So the stream renews the watch
// itself until its context ends, which happens when the client disconnects, the
// stream is torn down, or the server shuts down.
//
// Only the listen path takes this. A legacy resources/subscribe has no stream
// to be evidence of anything, and its session does see ordinary traffic.
func (b *sessionBridge) renewWhileStreaming(ctx context.Context, session *mcp.ServerSession) {
	if listenStreamOf(ctx) == nil {
		return
	}
	// Comfortably inside the lease, so a renewal is never the thing that
	// arrives late.
	interval := b.manager.Lease() / 3
	if interval <= 0 {
		return
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				b.manager.RenewAll(session)
			}
		}
	}()
}

// hold records that a stream is keeping a watch alive. A nil stream is the
// legacy resources/subscribe, which holds nothing: there is no second holder
// to protect it from, and the session ending is what releases it.
func (b *sessionBridge) hold(session *mcp.ServerSession, uri string, stream *listenStream) {
	if stream == nil {
		return
	}
	key := watchHold{session: session, uri: uri}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.holds[key] == nil {
		b.holds[key] = make(map[*listenStream]struct{}, 1)
	}
	b.holds[key][stream] = struct{}{}
}

// release drops one stream's hold and reports whether the watch is now
// unheld, which is when it may actually stop.
func (b *sessionBridge) release(session *mcp.ServerSession, uri string, stream *listenStream) bool {
	key := watchHold{session: session, uri: uri}

	b.mu.Lock()
	defer b.mu.Unlock()
	holders := b.holds[key]
	if holders == nil {
		return true
	}
	delete(holders, stream)
	if len(holders) > 0 {
		return false
	}
	delete(b.holds, key)
	return true
}

// releaseSession forgets every hold a session had, for when it disconnects.
func (b *sessionBridge) releaseSession(session *mcp.ServerSession) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for key := range b.holds {
		if key.session == session {
			delete(b.holds, key)
		}
	}
}

// codeServerBusy is the implementation-defined JSON-RPC server-error code
// (-32000..-32099 are reserved for these) this server uses for refusals
// that are about server state rather than the request: rate limiting, the
// watcher cap, and shutdown. The condition is transient, so unlike the
// invalid-params family a retry later can succeed.
const codeServerBusy = -32000

// wireSubscribeError classifies a subscription failure for the wire.
//
// The manager speaks in sentinels because its callers branch with
// errors.Is; the SDK, however, marshals any error that is not a
// *jsonrpc.Error with code 0, which generic clients render as "unknown
// error". This boundary is where the sentinel becomes a deliberate code:
// a URI that is deliberately not subscribable is an invalid parameter, an
// unreadable resource gets the code the SDK itself answers an unknown
// resources/read with, server-state refusals get
// the implementation-defined busy code, and anything else — a transient
// GitLab failure on the first read — is an internal error. Messages pass
// through untouched — they preserve the upstream failure detail — and the
// code alone carries the retry semantics.
func wireSubscribeError(err error) error {
	if err == nil {
		return nil
	}
	// The subscription sentinels are checked first, before any code the error
	// already carries.
	//
	// That order matters since resource reads began carrying a JSON-RPC code of
	// their own: internal/resources marks an upstream failure -32603 so it does
	// not reach a client as code 0, and an inaccessible resource arrives here
	// wrapped in ErrInaccessible around exactly such an error. Honoring the
	// carried code first would answer -32603 for a subscription the caller
	// simply may not have, which is -32602 and is what the caller has to act
	// on. The sentinel is the more specific fact, so it wins.
	var code int64
	switch {
	case errors.Is(err, subscriptions.ErrNotSubscribable):
		code = jsonrpc.CodeInvalidParams
	case errors.Is(err, subscriptions.ErrInaccessible):
		// The SDK's CodeResourceNotFound is deprecated in favor of exactly
		// this: from 1.7.0 "resource not found" IS invalid params (-32002
		// survives only behind a compat switch this server does not honor).
		code = jsonrpc.CodeInvalidParams
	case errors.Is(err, subscriptions.ErrRateLimited),
		errors.Is(err, subscriptions.ErrTooManySubscriptions),
		errors.Is(err, subscriptions.ErrClosed):
		code = codeServerBusy
	default:
		// No sentinel applies. A code chosen deliberately further down, such as
		// the stateless refusal building its own, is then the best answer there
		// is.
		if _, ok := errors.AsType[*jsonrpc.Error](err); ok {
			return err
		}
		code = jsonrpc.CodeInternalError
	}
	return &jsonrpc.Error{Code: code, Message: err.Error()}
}

// Unsubscribe drops one session's hold on a URI.
// Unsubscribe releases one holder's interest, stopping the watch only when no
// other listen stream is still holding it.
//
// The manager counts subscribers by session, which is right for the legacy
// resources/subscribe — there the session really is the subscription. It is
// wrong for 2026-07-28, where the subscription is the listen request and a
// session may hold several: the SDK unsubscribes every URI a listen carried
// when that listen ends, and with the session as the only identity, the first
// stream to close released a watch its sibling was still waiting on. The
// sibling's stream stayed open and acknowledged, and could never fire again.
func (b *sessionBridge) Unsubscribe(ctx context.Context, req *mcp.UnsubscribeRequest) error {
	if !b.release(req.Session, req.Params.URI, listenStreamOf(ctx)) {
		return nil
	}
	return b.manager.Unsubscribe(req.Session, req.Params.URI)
}

// awaitEnd releases a session's subscriptions when it disconnects, once per
// session.
func (b *sessionBridge) awaitEnd(session *mcp.ServerSession) {
	b.mu.Lock()
	_, already := b.awaited[session]
	if !already {
		b.awaited[session] = struct{}{}
	}
	b.mu.Unlock()
	if already {
		return
	}

	go func() {
		_ = session.Wait()
		b.mu.Lock()
		delete(b.awaited, session)
		b.mu.Unlock()
		b.releaseSession(session)
		b.manager.UnsubscribeAll(session)
	}()
}

// methodSubscriptionsListen is SEP-2575's long-lived subscription request,
// which replaces resources/subscribe from protocol 2026-07-28. The SDK
// keeps its own constant unexported.
const methodSubscriptionsListen = "subscriptions/listen"

// listenStream is one open subscriptions/listen request and the URIs it is
// still waiting on.
type listenStream struct {
	cancel context.CancelFunc
	live   map[string]struct{}
}

// listenStreams ends a subscription stream when the server stops watching
// everything that stream asked about.
//
// A subscription at protocol 2026-07-28 is a request the client leaves
// open, and the specification says a server that tears one down should
// answer it rather than go quiet. The SDK gives application code no way to
// send that answer — SubscriptionsListenResult embeds an unexported type,
// so it cannot be constructed here — but the SDK's own handler produces it
// when its context ends. Canceling that context from middleware is
// therefore the only way to close a stream properly, which is what this
// does.
//
// Granularity is the stream, not the URI, so a stream is only closed once
// none of its URIs are watched any more, and never if it also carries
// list-changed subscriptions: a client that batched several things into one
// request must not lose the rest of them because one resource went away.
type listenStreams struct {
	mu      sync.Mutex
	streams map[*listenStream]struct{}
}

func newListenStreams() *listenStreams {
	return &listenStreams{streams: make(map[*listenStream]struct{})}
}

// arm registers a stream and returns the function that unregisters it.
func (s *listenStreams) arm(uris []string, cancel context.CancelFunc) (stream *listenStream, release func()) {
	stream = &listenStream{cancel: cancel, live: make(map[string]struct{}, len(uris))}
	for _, uri := range uris {
		stream.live[uri] = struct{}{}
	}

	s.mu.Lock()
	s.streams[stream] = struct{}{}
	s.mu.Unlock()

	return stream, func() {
		s.mu.Lock()
		delete(s.streams, stream)
		s.mu.Unlock()
	}
}

// stopped records that a URI is no longer watched, closing any stream that
// was waiting only on URIs that have all stopped.
func (s *listenStreams) stopped(uri string, _ error) {
	s.mu.Lock()
	var finished []*listenStream
	for stream := range s.streams {
		if _, waiting := stream.live[uri]; !waiting {
			continue
		}
		delete(stream.live, uri)
		if len(stream.live) == 0 {
			finished = append(finished, stream)
			delete(s.streams, stream)
		}
	}
	s.mu.Unlock()

	// Canceling unblocks the SDK's handler, which then writes its normal
	// result: the graceful end the specification asks for. Done outside the
	// lock, since the handler unwinds through the unsubscribe path.
	for _, stream := range finished {
		stream.cancel()
	}
}

// closeAll ends every open listen stream.
//
// Called on shutdown, where the alternative is not exiting: the SDK's listen
// handler blocks until its context ends, and nothing else ends it, so a server
// with one open subscription ignored SIGTERM entirely and had to be killed —
// which is a worse outcome than the missing completion result, since a
// supervisor's next step is SIGKILL, possibly mid-write.
//
// Canceling is also how each stream gets its completion result: the SDK writes
// one when its handler's context ends, and application code cannot construct
// that result itself.
func (s *listenStreams) closeAll() {
	s.mu.Lock()
	open := make([]*listenStream, 0, len(s.streams))
	for stream := range s.streams {
		open = append(open, stream)
	}
	clear(s.streams)
	s.mu.Unlock()

	for _, stream := range open {
		stream.cancel()
	}
}

// middleware intercepts subscriptions/listen so its handler can be ended
// from outside.
func (s *listenStreams) middleware() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method != methodSubscriptionsListen {
				return next(ctx, method, req)
			}
			// Every listen is armed, not only the ones that can be closed by
			// their URIs stopping. Registration is what makes a stream
			// reachable at shutdown, and a stream nobody can reach is one that
			// blocks the process from exiting. Passing no URIs is what marks a
			// stream as not closable that way: stopped() only ever looks at
			// streams waiting on the URI it was given.
			uris, closableByURI := closableListenURIs(method, req)
			if !closableByURI {
				uris = nil
			}
			streamCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			stream, release := s.arm(uris, cancel)
			defer release()

			// The stream travels in the context for every listen, including
			// one that also carries list-changed subscriptions. It answers two
			// questions further down: which method a subscribe is serving, and
			// which stream is holding a watch. The SDK's deferred unsubscribe
			// runs with this same context, so the second answer is still there
			// when the stream is torn down.
			return next(withListenStream(streamCtx, stream), method, req)
		}
	}
}

// listenStreamKey carries the subscriptions/listen stream a request belongs to.
//
// It serves two purposes at once, both of which come from the SDK giving
// subscriptions/listen and resources/subscribe a single handler.
//
// The first is telling them apart: subscriptionsListen calls SubscribeHandler
// once per resource URI it carries and returns that handler's error before it
// acknowledges anything, so a handler that refuses every subscribe refuses
// subscriptions/listen too, whatever it meant to say.
//
// The second is telling two listens apart. A session may open several, and
// 2026-07-28 makes the listen request the subscription's identity; the SDK's
// own table holds one request ID per session per URI, and the session is what
// the watch manager counts by. So without this, one stream's teardown released
// a watch another stream was still holding.
type listenStreamKey struct{}

// withListenStream records which listen stream a request belongs to.
func withListenStream(ctx context.Context, stream *listenStream) context.Context {
	return context.WithValue(ctx, listenStreamKey{}, stream)
}

// listenStreamOf returns the listen stream a request belongs to, or nil for a
// client's own resources/subscribe.
func listenStreamOf(ctx context.Context) *listenStream {
	stream, _ := ctx.Value(listenStreamKey{}).(*listenStream)
	return stream
}

// isListenSubscribe reports whether this subscribe came from a
// subscriptions/listen request rather than a client's resources/subscribe.
func isListenSubscribe(ctx context.Context) bool {
	return listenStreamOf(ctx) != nil
}

// closableListenURIs reports the resource URIs of a listen request that
// exists only to carry them, and so can be ended when they stop.
func closableListenURIs(method string, req mcp.Request) ([]string, bool) {
	if method != methodSubscriptionsListen {
		return nil, false
	}
	listen, isListen := req.(*mcp.SubscriptionsListenRequest)
	if !isListen || listen.Params == nil || listen.Params.Notifications == nil {
		return nil, false
	}
	wanted := listen.Params.Notifications
	if len(wanted.ResourceSubscriptions) == 0 {
		return nil, false
	}
	if wanted.ToolsListChanged || wanted.PromptsListChanged || wanted.ResourcesListChanged {
		// The stream carries more than these resources; ending it would
		// silently cancel subscriptions that are still perfectly valid.
		return nil, false
	}
	return wanted.ResourceSubscriptions, true
}

// subscriptionRuntime holds the resource-subscription machinery that has to
// exist before the server — its handlers travel in mcp.ServerOptions — and
// be wired to it afterwards.
type subscriptionRuntime struct {
	manager  *subscriptions.Manager[*mcp.ServerSession]
	notifier *serverNotifier
	streams  *listenStreams
	// stateless records that this server runs on a sessionless HTTP
	// transport, where the legacy subscribe path cannot be honored — see
	// [subscriptionRuntime.handlers].
	stateless bool
}

// newSubscriptionRuntime builds the machinery behind resources/subscribe,
// or nil when subscriptions are not offered.
//
// Nil is the case whenever the GitLab resources themselves are not
// registered: advertising the capability then would let a client subscribe
// to URIs this server never serves, and every such subscription would be
// refused.
func newSubscriptionRuntime(
	client *gitlabclient.Client,
	cfg *config.ServerConfig,
	opts subscriptions.Options,
) *subscriptionRuntime {
	if config.EffectiveCapabilitySurface(cfg.CapabilitySurface) != config.CapabilitySurfaceFull {
		return nil
	}
	notifier := &serverNotifier{}
	streams := newListenStreams()
	opts.OnStop = streams.stopped
	return &subscriptionRuntime{
		manager: subscriptions.New[*mcp.ServerSession](
			resourceReader{index: resources.NewHandlerIndex(client)},
			notifier,
			opts,
		),
		notifier:  notifier,
		streams:   streams,
		stateless: cfg.Stateless,
	}
}

// handlers adapts the runtime to the SDK's handler signatures, or returns a
// nil pair when there is no runtime.
//
// They are always produced together because the SDK panics at construction
// if only one of the two is set — a deliberate guard, since a server that
// accepts subscriptions but cannot cancel them would leak watchers.
func (r *subscriptionRuntime) handlers() (
	subscribe func(context.Context, *mcp.SubscribeRequest) error,
	unsubscribe func(context.Context, *mcp.UnsubscribeRequest) error,
) {
	if r == nil {
		return nil, nil
	}
	bridge := newSessionBridge(r.manager)
	if r.stateless {
		return bridge.subscribeUnlessStateless, bridge.Unsubscribe
	}
	return bridge.Subscribe, bridge.Unsubscribe
}

// ErrStatelessSubscribe explains why a legacy subscription cannot be
// honored on a sessionless transport. It is a *jsonrpc.Error so the wire
// carries a deliberate code: a plain error would reach the client as code
// 0, which generic clients render as "unknown error". -32601 would be
// wrong — the server does support subscriptions, just not via the
// session-era method on this transport — so the refusal is classified as
// an invalid request for the session state it arrived in.
var errStatelessSubscribe = &jsonrpc.Error{
	Code: jsonrpc.CodeInvalidRequest,
	Message: "resources/subscribe cannot be honored in stateless HTTP mode, because the session ends " +
		"with the request that created it and there is no stream left to notify on; " +
		"use protocol 2026-07-28 (subscriptions/listen), or run the server with --stateless=false",
}

// subscribeUnlessStateless refuses a legacy subscribe that this transport
// could never deliver on, and only that one.
//
// The capability itself stays advertised, because the SDK requires it for
// the 2026-07-28 subscriptions/listen path, which works in stateless mode
// precisely because the subscription IS an open request. What cannot work
// is the older resources/subscribe: the SDK gives each stateless POST its
// own session and closes it when the POST returns, so the subscription the
// server just acknowledged would be cancelled microseconds later — after
// having spent a GitLab round-trip on the authorization read. Refusing says
// so, where accepting would leave the client waiting forever for a
// notification with nowhere to go.
//
// The mark is what makes that distinction possible. Refusing unconditionally
// looked equivalent and was not: the SDK routes a listen's resource URIs
// through this same handler and returns the first error before acknowledging
// anything, so the refusal reached every listen carrying a resource — which
// is to say the whole feature, on the default configuration, while the
// handshake went on advertising resources.subscribe. A mixed listen was worse
// still: its list-changed half died with the resource half.
func (b *sessionBridge) subscribeUnlessStateless(ctx context.Context, req *mcp.SubscribeRequest) error {
	if isListenSubscribe(ctx) {
		return b.Subscribe(ctx, req)
	}
	return errStatelessSubscribe
}

// attach connects the runtime to the server it notifies through, once that
// server exists.
func (r *subscriptionRuntime) attach(ctx context.Context, server *mcp.Server) {
	if r == nil {
		return
	}
	r.notifier.attach(server)

	// Shutdown has to reach the open listen streams, and nothing else does:
	// their handlers block on contexts derived from each session, which the
	// SDK does not end while it is waiting for those same handlers to return.
	go func() {
		<-ctx.Done()
		r.streams.closeAll()
	}()
	server.AddReceivingMiddleware(
		// Traffic on a session is the only evidence this server gets that
		// a subscriber is still there, so it is what holds the watchers at
		// full speed.
		renewOnActivity(r.manager),
		r.streams.middleware(),
	)
}

// renewOnActivity returns middleware that keeps a session's subscriptions
// at full speed for as long as that session is being used.
//
// A subscription's lease is really a proxy for "is anyone still there", and
// the only evidence of that this server ever gets is traffic. Renewing on
// any request — not on reads of the watched URI — is deliberate: a watcher
// only speaks up when something actually changed, so a quiet resource
// produces no notification and no re-read, and a URI-scoped renewal would
// let the watch slow down during precisely the wait its subscriber cares
// about.
func renewOnActivity(manager *subscriptions.Manager[*mcp.ServerSession]) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			result, err := next(ctx, method, req)
			// Renewing after the handler rather than before it is what
			// keeps the watcher cap workable: a subscribe request is
			// itself activity, so renewing first would un-demote every
			// watcher a fraction of a second before that same request
			// looked for a demoted one to evict, and the eviction could
			// never fire. Renewing afterwards means the request still
			// counts, but only once it has had its slot.
			if session, ok := activeSession(method, req); ok {
				manager.RenewAll(session)
			}
			return result, err
		}
	}
}

// activeSession reports the session a request proves is still being used,
// or false when the request is not evidence of that.
func activeSession(method string, req mcp.Request) (*mcp.ServerSession, bool) {
	if !isClientActivity(method) || req == nil {
		return nil, false
	}
	session, ok := req.GetSession().(*mcp.ServerSession)
	return session, ok
}

// isClientActivity reports whether a method means a client is still working
// through this session.
//
// Keep-alive traffic and the handshake do not count: a ping proves a socket
// is open, not that anyone is waiting on the other end of it, and treating
// it as activity would make the lease unreachable for any connected client
// — which is the same as having no lease at all.
func isClientActivity(method string) bool {
	switch method {
	case "ping", "initialize":
		return false
	}
	return !strings.HasPrefix(method, "notifications/")
}

// serverOption adjusts how [createServer] builds a server, for the few
// knobs that are not part of the user-facing configuration.
type serverOption func(*serverSettings)

// serverSettings holds those knobs. The zero value is what production runs
// on: every field falls back to its package's own defaults.
type serverSettings struct {
	subscriptions subscriptions.Options
	// sessionTag prefixes every session ID this server mints, so a stateful
	// request presenting a session ID can be checked against the credential
	// that minted it. Empty in stdio mode, where there is no pool and no
	// session to steal.
	sessionTag string
	// keepAlive overrides the server-initiated ping interval. Nil leaves the
	// transport default in place; a pointer to zero disables the ping.
	keepAlive *time.Duration
}

// withKeepAlive overrides the server keepalive interval for this server.
func withKeepAlive(d time.Duration) serverOption {
	return func(s *serverSettings) { s.keepAlive = &d }
}

// withSessionTag makes this server mint session IDs carrying tag as a prefix.
func withSessionTag(tag string) serverOption {
	return func(s *serverSettings) { s.sessionTag = tag }
}

func newServerSettings(opts []serverOption) serverSettings {
	var settings serverSettings
	for _, opt := range opts {
		opt(&settings)
	}
	return settings
}

// withSubscriptionOptions overrides the resource-subscription manager's
// polling cadence and limits.
//
// Only tests pass this. Production polls on intervals derived from GitLab's
// rate limits — fifteen seconds between reads, five at the floor — which is
// far longer than a test can reasonably wait for a notification. Threading
// it through as an option rather than a package-level variable keeps the
// production defaults immutable and scopes the override to one server, so
// tests running in parallel cannot change each other's cadence.
func withSubscriptionOptions(opts subscriptions.Options) serverOption {
	return func(settings *serverSettings) { settings.subscriptions = opts }
}
