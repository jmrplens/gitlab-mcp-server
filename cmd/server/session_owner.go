package main

import (
	"context"
	"log/slog"
	"maps"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// sessionOwners records which pooled credential each MCP session belongs to.
//
// # Why it has to be recorded rather than derived
//
// It used to be derived. Every pooled server minted its session IDs under a tag
// of its own, so the tag in an ID was a statement about which credential the
// session belonged to, and the gate refused an ID whose tag did not match the
// server it had just resolved. That works only while a server serves one
// credential. [mcp.ServerOptions.GetSessionID] takes no request, so a server
// shared by a configuration shape mints under one tag for everybody, and a tag
// stops saying anything.
//
// So the fact is recorded where it is known: an MCP request arrives bound to
// its credential by [credentialStates.bindCredential], and that is where the
// session and the owner are written down together. Only the sessions something
// will ask about are recorded, which is what [worthRecording] decides.
//
// # What reads it
//
//   - The gate, to refuse a stateful GET, DELETE or POST presenting a session
//     ID minted for a different credential. Structurally the same check as
//     before, on a recorded fact instead of an inferred one.
//   - [sessionOwners.sendingMiddleware], to keep a resource-updated
//     notification to the sessions of the credential whose watcher produced it.
//
// # When it forgets
//
// When the session ends, which [mcp.ServerSession.Wait] reports, and when the
// pool evicts the entry, so a rebuilt credential's sessions cannot be answered
// for by the entry that replaced it.
type sessionOwners struct {
	// stateless records that this deployment runs the sessionless transport,
	// where a legacy resources/subscribe is refused before it can subscribe
	// anything. Recording the session it arrived on would then cost a map entry
	// and a goroutine parked on Wait for a fact nothing can ever read.
	stateless bool

	mu        sync.Mutex
	bySession map[*mcp.ServerSession]string
	byID      map[string]string
	// idsByOwner and sessionsByOwner make eviction a lookup rather than a walk
	// of every live session.
	idsByOwner      map[string]map[string]struct{}
	sessionsByOwner map[string]map[*mcp.ServerSession]struct{}
}

func newSessionOwners(stateless bool) *sessionOwners {
	return &sessionOwners{
		stateless:       stateless,
		bySession:       make(map[*mcp.ServerSession]string),
		byID:            make(map[string]string),
		idsByOwner:      make(map[string]map[string]struct{}),
		sessionsByOwner: make(map[string]map[*mcp.ServerSession]struct{}),
	}
}

// record notes that session belongs to owner, and arranges to forget it when
// the session ends.
//
// Recording is idempotent and cheap enough to attempt on every request, which
// is deliberate: the session ID is not reliably readable at every point a
// session could first be seen, and a session whose owner is unknown is refused
// rather than served, so a missed recording would be a refusal rather than a
// leak. Attempting it every time removes the question.
func (o *sessionOwners) record(session *mcp.ServerSession, owner string) {
	if o == nil || session == nil || owner == "" {
		return
	}
	id := session.ID()

	o.mu.Lock()
	existing, known := o.bySession[session]
	if known && existing == owner {
		// Already recorded. The ID may still be missing if it was not readable
		// the first time, so it is filled in here rather than skipped.
		o.rememberIDLocked(id, owner)
		o.mu.Unlock()
		return
	}
	if known {
		// Rebinding to another credential. The forward lookups are overwritten
		// below, but the reverse indexes are not: without this the previous
		// owner's eviction would still find this session and drop a live
		// credential's claim on it, and the ID would answer for whichever owner
		// was written last. No HTTP path reaches this today, because a session
		// is refused when it is presented with a different credential, and it
		// is written down rather than left to that: the map is what the refusal
		// is decided from.
		o.dropSessionLocked(existing, session)
		o.dropIDLocked(existing, id)
	}
	o.bySession[session] = owner
	if o.sessionsByOwner[owner] == nil {
		o.sessionsByOwner[owner] = make(map[*mcp.ServerSession]struct{}, 1)
	}
	o.sessionsByOwner[owner][session] = struct{}{}
	o.rememberIDLocked(id, owner)
	o.mu.Unlock()

	if known {
		// The session was already recorded under a different owner, so the
		// waiter that forgets it is already running.
		return
	}
	go func() {
		_ = session.Wait()
		o.forget(session)
	}()
}

// rememberIDLocked files a session ID under its owner. Callers hold o.mu.
func (o *sessionOwners) rememberIDLocked(id, owner string) {
	if id == "" {
		return
	}
	o.byID[id] = owner
	if o.idsByOwner[owner] == nil {
		o.idsByOwner[owner] = make(map[string]struct{}, 1)
	}
	o.idsByOwner[owner][id] = struct{}{}
}

// ownerOf returns the credential a session belongs to, or "" when this server
// never recorded one.
func (o *sessionOwners) ownerOf(session *mcp.ServerSession) string {
	if o == nil || session == nil {
		return ""
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.bySession[session]
}

// ownerOfID returns the credential a session ID belongs to, or "" when this
// server never minted or never recorded it.
func (o *sessionOwners) ownerOfID(id string) string {
	if o == nil || id == "" {
		return ""
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.byID[id]
}

// forget drops one session, for when it disconnects.
func (o *sessionOwners) forget(session *mcp.ServerSession) {
	if o == nil || session == nil {
		return
	}
	id := session.ID()

	o.mu.Lock()
	defer o.mu.Unlock()
	owner, ok := o.bySession[session]
	if !ok {
		return
	}
	delete(o.bySession, session)
	o.dropSessionLocked(owner, session)
	o.dropIDLocked(owner, id)
}

// forgetOwner drops every session of an evicted entry, and returns them.
//
// Without it a session outlives the credential it belonged to: the gate would
// go on accepting its ID, and a notification tagged with a rebuilt entry's
// owner would be filtered away from a session the client still holds. Both are
// answered by making the credential's disappearance end its sessions' claim.
//
// The sessions are returned because forgetting them is not the same as telling
// their client, and the eviction has to do both. This runs under the pool's
// write lock, where nothing may block, so the telling is done afterwards from
// [credentialState.close] and needs the list this took away.
func (o *sessionOwners) forgetOwner(owner string) []*mcp.ServerSession {
	if o == nil || owner == "" {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	orphaned := make([]*mcp.ServerSession, 0, len(o.sessionsByOwner[owner]))
	for session := range o.sessionsByOwner[owner] {
		delete(o.bySession, session)
		orphaned = append(orphaned, session)
	}
	delete(o.sessionsByOwner, owner)
	for id := range o.idsByOwner[owner] {
		delete(o.byID, id)
	}
	delete(o.idsByOwner, owner)
	return orphaned
}

// endSessionsWithoutStreams terminates the evicted credential's sessions that
// no open subscriptions/listen already ended.
//
// It closes the hole [listenStreams.closeOwner] cannot reach. A session-era
// resources/subscribe is not a request the client leaves open, so there is no
// stream to complete and no answer to write: the eviction stopped the watchers,
// which Manager.Close does without firing OnStop, and on --stateless=false the
// client kept a live session with a standalone SSE stream that would never
// carry anything again. It was not told, it could not tell, and ADR-0020's
// promise that an evicted client is told to re-subscribe was false for that one
// path.
//
// Terminating the session is the only ending the protocol offers a subscriber
// with no open request: the standalone stream ends, and the client's next
// request re-initializes. It is also what the gate would enforce a moment later
// anyway, since [sessionOwners.forgetOwner] has just taken the session's claim
// away — the difference is that a client whose only activity is a subscription
// makes no next request, so without this it learns nothing, ever.
//
// Two exclusions, both load-bearing:
//
//   - A session that held a listen is left alone. [listenStreams.closeOwner]
//     has already cancelled that stream, and the SDK writes its completion
//     result as the handler unwinds; closing the connection would race that
//     write and turn the graceful ending into a torn-down one.
//   - On the sessionless transport nothing is terminated at all. Each POST gets
//     a session of its own that closes with its response, so there is nothing
//     to save and closing it would race the response the SDK is writing. A
//     legacy subscribe is refused outright there
//     ([sessionBridge.subscribeUnlessStateless]), which is why nothing is lost.
func (o *sessionOwners) endSessionsWithoutStreams(orphaned []*mcp.ServerSession, told map[*mcp.ServerSession]struct{}) {
	if o == nil || o.stateless {
		return
	}
	for _, session := range orphaned {
		if session == nil {
			continue
		}
		if _, ended := told[session]; ended {
			continue
		}
		// The error is dropped rather than logged because there is only one:
		// the session was already gone, which is the state this is trying to
		// reach. A client disconnecting between the eviction and this call is
		// ordinary, not a fault.
		_ = session.Close()
	}
}

// dropSessionLocked and dropIDLocked remove one entry from the per-owner
// indexes and the index itself once it is empty. Callers hold o.mu.
func (o *sessionOwners) dropSessionLocked(owner string, session *mcp.ServerSession) {
	sessions := o.sessionsByOwner[owner]
	if sessions == nil {
		return
	}
	delete(sessions, session)
	if len(sessions) == 0 {
		delete(o.sessionsByOwner, owner)
	}
}

func (o *sessionOwners) dropIDLocked(owner, id string) {
	if id == "" {
		return
	}
	delete(o.byID, id)
	ids := o.idsByOwner[owner]
	if ids == nil {
		return
	}
	delete(ids, id)
	if len(ids) == 0 {
		delete(o.idsByOwner, owner)
	}
}

// recordingMiddleware notes the credential of every session it sees a request
// on.
//
// It runs inside [credentialStates.bindCredential], which is what put the
// credential on the context, and does nothing at all on a request nothing bound
// (stdio, the in-memory transport, a test's own server).
//
// # The one process-wide lock left on the request path
//
// [sessionOwners.record] takes o.mu, and there is one sessionOwners per
// process, so every stateful request serializes on it for the length of two map
// reads and up to four map writes. Nothing else on the request path does: the
// rate-limit bucket, the listen counter and the watchers are all per credential
// since ADR-0020, and the pool's own lock is taken by the gate before the MCP
// layer is reached.
//
// It is recorded rather than changed. The critical section is a few map
// operations with no allocation and no I/O, [worthRecording] keeps the default
// stateless transport out of it entirely (there the map is touched only by a
// session that subscribes), and the same-owner fast path returns after one read
// and one write. Sharding it by session pointer, or dropping to a sync.Map,
// would trade that for a structure whose eviction path can no longer be one
// atomic sweep, which is the operation correctness depends on. Measure before
// changing it: at the point this was written no profile showed contention here,
// and the reason to write it down is that a future middleware taking the same
// lock per request would inherit a bottleneck nobody documented.
func (o *sessionOwners) recordingMiddleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		owner := ownerOfRequest(ctx)
		if owner != "" && req != nil {
			if session, ok := req.GetSession().(*mcp.ServerSession); ok && o.worthRecording(session, method) {
				o.record(session, owner)
			}
		}
		return next(ctx, method, req)
	}
}

// worthRecording reports whether this server will ever have to ask who a
// session belongs to.
//
// Recording every session would be correct and wasteful. On the default
// stateless transport each POST gets a session of its own that closes with the
// response, so recording one costs a map entry and a goroutine parked on
// [mcp.ServerSession.Wait] for the length of one request, on every request, for
// a fact nothing will read: such a session has no ID for the gate to check and
// can receive no notification after its POST is over.
//
// The two cases that are read are therefore the two recorded. A session with an
// ID is a stateful one, and the gate checks every later request against it. A
// subscribe is the request that makes a session able to receive a resource
// update, which is what the delivery filter has to attribute.
//
// Which subscribe depends on the transport. subscriptions/listen counts on
// both, since the subscription is that open request. The session-era
// resources/subscribe counts only where it can be honored: on the sessionless
// transport it is refused before it subscribes anything, so a session recorded
// for it can never receive a notification, and recording one costs a map entry
// and a goroutine parked on Wait per request for a fact nothing reads.
func (o *sessionOwners) worthRecording(session *mcp.ServerSession, method string) bool {
	if session.ID() != "" {
		return true
	}
	if method == methodSubscriptionsListen {
		return true
	}
	return method == methodResourcesSubscribe && !o.stateless
}

// methodResourcesSubscribe is the session-era subscription method. The SDK
// keeps its own constant unexported.
const methodResourcesSubscribe = "resources/subscribe"

// sendingMiddleware keeps each credential's resource-updated notifications to
// that credential's own sessions.
//
// # Why a sending middleware is the place
//
// [mcp.Server.ResourceUpdated] is the only delivery the SDK exports, and it
// notifies every session subscribed to the URI: the subscription table is keyed
// by URI and session and knows nothing about credentials. [mcp.ServerSession]
// exposes no per-session equivalent, and the 2026-07-28 form needs the listen
// request ID the SDK stamps into `_meta` itself, so application code cannot
// send one either.
//
// Both of the SDK's delivery paths do run the notification through the server's
// sending middleware, one session at a time, with the session and the params in
// hand: the legacy path builds the request per session in notifySessions, and
// the 2026-07-28 path does the same in notifySubscribedSessions after stamping
// the subscription ID. So this is the one point at which "who is this for" can
// still be asked.
//
// The channel is the params, and it has to be: both of those functions create a
// fresh context.Background() with a ten second timeout for the send, so nothing
// the caller of ResourceUpdated put on its own context reaches here.
//
// # Why it fails closed
//
// A notification with no owner tag, or one for a session with no recorded
// owner, is dropped. Every notification this server emits is tagged
// ([serverNotifier.ResourceUpdated]) and every session that could hold a
// subscription was recorded when its subscribe arrived, so neither absence can
// happen in a correct wiring; treating either as "deliver anyway" would turn
// one wiring mistake into cross-tenant delivery.
//
// # Why it reads the params rather than the request
//
// The two delivery paths hand the middleware two different request types. The
// legacy one passes the concrete params, so its request is a
// `*mcp.ServerRequest[*mcp.ResourceUpdatedNotificationParams]`; the 2026-07-28
// one passes them through a `func() Params` and its request is a
// `*mcp.ServerRequest[mcp.Params]`. Asserting on the request therefore matches
// one path and silently drops the other, which is exactly what happened the
// first time this was written: every notification was sent, none was delivered,
// and the server logged "resource updated notification sent" for both. The
// params interface is the same on both paths, and the concrete params behind it
// are always `*mcp.ResourceUpdatedNotificationParams`, so that is what this
// reads.
//
// # Why it puts the key back
//
// notifySubscribedSessions shallow-copies the params per session and the legacy
// path does not copy at all, so on the legacy path one params value is handed to
// every subscriber in turn. Stripping the owner key and leaving it stripped
// would make the second session onwards look untagged, and they would all be
// dropped. Restoring it after the send is safe because delivery is sequential
// within one ResourceUpdated call, and because each call builds its own params
// and its own `_meta` map, so two watchers of the same URI never share one.
//
// The map itself is never mutated: metaWithout returns a new one, and the
// `_meta` map really is shared between the per-session copies, which is also
// where the SDK stamps its own subscription id.
func (o *sessionOwners) sendingMiddleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		if method != notificationResourceUpdated || req == nil {
			return next(ctx, method, req)
		}
		update, ok := req.GetParams().(*mcp.ResourceUpdatedNotificationParams)
		if !ok || update == nil {
			o.dropped(ctx, "", "the notification carried no resource-updated params")
			return nil, nil
		}
		session, _ := req.GetSession().(*mcp.ServerSession)
		tagged, _ := update.Meta[ownerMetaKey].(string)
		if tagged == "" {
			o.dropped(ctx, update.URI, "the notification carried no owner tag")
			return nil, nil
		}
		if tagged != o.ownerOf(session) {
			o.dropped(ctx, update.URI, "the receiving session belongs to another credential, or to none")
			return nil, nil
		}

		original := update.Meta
		update.Meta = metaWithout(original, ownerMetaKey)
		defer func() { update.Meta = original }()
		return next(ctx, method, req)
	}
}

// dropped records a notification the filter refused to deliver.
//
// Every drop here is a wiring defect rather than a normal outcome: this server
// tags everything it sends and records every session that could be subscribed,
// so in a correct wiring the filter forwards or has nothing to do. Failing
// closed is right, but doing it silently is what made the first version of this
// so hard to see, since the SDK logs that it sent the notification either way
// and the client simply never hears anything. Debug rather than warn because a
// session that closed while a notification was in flight can produce one
// legitimately.
//
// The owner token is deliberately not logged. It is what a notification is
// attributed by, and a log is not the place to publish it; the URI and the
// reason are what identify the problem.
func (o *sessionOwners) dropped(ctx context.Context, uri, reason string) {
	slog.DebugContext(ctx, "resource-updated notification not delivered", "uri", uri, "reason", reason)
}

// notificationResourceUpdated is the method name of a resource-updated
// notification. The SDK keeps its own constant unexported.
const notificationResourceUpdated = "notifications/resources/updated"

// metaWithout returns a copy of meta with one key removed, leaving the original
// untouched.
func metaWithout(meta mcp.Meta, key string) mcp.Meta {
	if meta == nil {
		return nil
	}
	clone := make(mcp.Meta, len(meta))
	maps.Copy(clone, meta)
	delete(clone, key)
	return clone
}
