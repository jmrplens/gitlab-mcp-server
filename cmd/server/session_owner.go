package main

import (
	"context"
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
	mu        sync.Mutex
	bySession map[*mcp.ServerSession]string
	byID      map[string]string
	// idsByOwner and sessionsByOwner make eviction a lookup rather than a walk
	// of every live session.
	idsByOwner      map[string]map[string]struct{}
	sessionsByOwner map[string]map[*mcp.ServerSession]struct{}
}

func newSessionOwners() *sessionOwners {
	return &sessionOwners{
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

// forgetOwner drops every session of an evicted entry.
//
// Without it a session outlives the credential it belonged to: the gate would
// go on accepting its ID, and a notification tagged with a rebuilt entry's
// owner would be filtered away from a session the client still holds. Both are
// answered by making the credential's disappearance end its sessions' claim.
func (o *sessionOwners) forgetOwner(owner string) {
	if o == nil || owner == "" {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	for session := range o.sessionsByOwner[owner] {
		delete(o.bySession, session)
	}
	delete(o.sessionsByOwner, owner)
	for id := range o.idsByOwner[owner] {
		delete(o.byID, id)
	}
	delete(o.idsByOwner, owner)
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
func (o *sessionOwners) recordingMiddleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		owner := ownerOfRequest(ctx)
		if owner != "" && req != nil {
			if session, ok := req.GetSession().(*mcp.ServerSession); ok && worthRecording(session, method) {
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
// subscribe, by either method, is the request that makes a session able to
// receive a resource update, which is what the delivery filter has to attribute.
func worthRecording(session *mcp.ServerSession, method string) bool {
	if session.ID() != "" {
		return true
	}
	return method == methodSubscriptionsListen || method == methodResourcesSubscribe
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
			return nil, nil
		}
		session, _ := req.GetSession().(*mcp.ServerSession)
		tagged, _ := update.Meta[ownerMetaKey].(string)
		if tagged == "" || tagged != o.ownerOf(session) {
			return nil, nil
		}

		original := update.Meta
		update.Meta = metaWithout(original, ownerMetaKey)
		defer func() { update.Meta = original }()
		return next(ctx, method, req)
	}
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
