// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// session_owner_test.go covers the table that records which pooled credential
// each MCP session belongs to, and the sending middleware that keeps a
// resource-updated notification to that credential's own sessions.
//
// Both used to be structural. A server served one credential, so its session
// IDs were tagged with that credential and its notifications could only reach
// its own subscribers. One server per configuration shape removes both
// guarantees at once: the ownership check and the notification filter are now
// the only things standing between two tenants sharing a server, which is why
// every branch of both is driven here rather than assumed.
package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// identifiedSessions hands out live MCP sessions whose IDs are real ones.
//
// Only the streamable HTTP transport mints session IDs: its connection is the
// sole implementor of the SDK's internal session-ID interface, so a session
// from the in-memory transport reports "" and [sessionOwners.record] files no
// ID for it. Every test that needs ownerOfID to answer therefore needs a
// session from a real streamable server, which is what this is.
type identifiedSessions struct {
	url string

	mu   sync.Mutex
	seen map[*mcp.ServerSession]struct{}
	// captured carries each session exactly once, so a second connect cannot
	// read the first connect's session back out of it. A later request on an
	// already-seen session is dropped by the seen set rather than queued.
	captured chan *mcp.ServerSession
}

// newIdentifiedSessions starts a streamable HTTP MCP server that hands its
// server-side sessions to the test.
func newIdentifiedSessions(t *testing.T) *identifiedSessions {
	t.Helper()

	sessions := &identifiedSessions{
		seen:     make(map[*mcp.ServerSession]struct{}),
		captured: make(chan *mcp.ServerSession, 16),
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "session-owner-test", Version: "0"}, nil)
	// The only place a server-side session object is reachable from outside the
	// SDK is a request that arrived on it. No assertion is made here: this runs
	// on the SDK's own goroutine, where an abort would kill that goroutine and
	// leave the connect below hanging instead of failing.
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			// Only the session a connection initializes on. A client opens a
			// short-lived session of its own for server/discover first, which
			// ends as soon as that answer is written, so taking the first
			// session seen handed every test here a session that was already
			// over: its Wait returns at once and sessionOwners forgets it,
			// which is correct for a session that ended and useless as a stand
			// -in for a pooled one.
			if session, ok := req.GetSession().(*mcp.ServerSession); ok && method == "initialize" {
				sessions.note(session)
			}
			return next(ctx, method, req)
		}
	})

	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server }, nil,
	))
	t.Cleanup(httpServer.Close)
	sessions.url = httpServer.URL
	return sessions
}

// note offers a session to the test once, and only once.
func (s *identifiedSessions) note(session *mcp.ServerSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, known := s.seen[session]; known {
		return
	}
	s.seen[session] = struct{}{}
	select {
	case s.captured <- session:
	default:
		// The buffer is sized well above the number of sessions any test here
		// opens; a drop would surface as the connect below timing out.
	}
}

// connect opens one client session and returns the server side of it, whose ID
// is the one the transport minted.
func (s *identifiedSessions) connect(t *testing.T) *mcp.ServerSession {
	t.Helper()

	clientSession, err := mcp.NewClient(&mcp.Implementation{Name: "session-owner-client", Version: "0"}, nil).
		Connect(t.Context(), &mcp.StreamableClientTransport{Endpoint: s.url}, nil)
	if err != nil {
		t.Fatalf("connecting a streamable client: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })

	select {
	case session := <-s.captured:
		if session.ID() == "" {
			t.Fatal("the streamable transport minted no session ID; sessionOwners would have nothing to file")
		}
		return session
	case <-time.After(10 * time.Second):
		t.Fatal("no server session was captured for a client that connected")
		return nil
	}
}

// recordUnder opens a session, records it under owner, and returns its ID.
func (s *identifiedSessions) recordUnder(t *testing.T, owners *sessionOwners, owner string) string {
	t.Helper()

	session := s.connect(t)
	owners.record(session, owner)
	id := session.ID()
	if got := owners.ownerOfID(id); got != owner {
		t.Fatalf("ownerOfID(%q) = %q right after recording it under %q", id, got, owner)
	}
	return id
}

// sdkSubscriptionIDKey is what the SDK stamps into the same `_meta` map the
// owner tag travels in, so a test can tell "the owner key was removed" from
// "the metadata was replaced".
const sdkSubscriptionIDKey = "io.modelcontextprotocol/subscriptionId"

// delivered is what a stub next handler returns, so that "the middleware
// forwarded this" and "the middleware answered on its own" are distinguishable
// in the result rather than only in a call count.
var delivered mcp.Result = &mcp.CallToolResult{}

// updateParams builds the params a watcher's notification carries, tagged for
// owner. An empty owner writes no key at all rather than an empty value, which
// is the shape a notification from anywhere but [serverNotifier] would have.
func updateParams(owner, uri string) *mcp.ResourceUpdatedNotificationParams {
	params := &mcp.ResourceUpdatedNotificationParams{URI: uri}
	params.Meta = mcp.Meta{sdkSubscriptionIDKey: "stamped-by-the-sdk"}
	if owner != "" {
		params.Meta[ownerMetaKey] = owner
	}
	return params
}

// deliveryShapes returns the notification in both of the request shapes the
// SDK's two delivery paths hand a sending middleware.
//
// This is not a detail: asserting on the request type matched the legacy path
// and silently dropped the 2026-07-28 one, which is the default transport. The
// legacy notifySessions passes the concrete params, so its request is a
// ServerRequest[*ResourceUpdatedNotificationParams]; notifySubscribedSessions
// passes them through a func() Params, so its request is a
// ServerRequest[Params]. Every case here runs through both.
func deliveryShapes(session *mcp.ServerSession, params *mcp.ResourceUpdatedNotificationParams) map[string]mcp.Request {
	return map[string]mcp.Request{
		"the legacy delivery path": &mcp.ServerRequest[*mcp.ResourceUpdatedNotificationParams]{
			Session: session, Params: params,
		},
		"the 2026-07-28 delivery path": &mcp.ServerRequest[mcp.Params]{
			Session: session, Params: params,
		},
	}
}

// TestSessionOwners_SendingMiddleware_DeliversOnlyToTheOwningSession covers the
// filter that keeps one credential's resource notifications away from another
// credential's sessions.
//
// The SDK's ResourceUpdated notifies every session subscribed to a URI: its
// subscription table is keyed by URI and session and knows nothing about
// credentials. On a server shared by a configuration shape that is delivery
// across tenants, and the sending middleware is the one point at which "who is
// this for" can still be asked, because both of the SDK's delivery paths run
// the notification through it once per session with the session in hand.
//
// It fails closed on purpose. An untagged notification and a session with no
// recorded owner are both impossible in a correct wiring, so treating either as
// "deliver anyway" would turn one wiring mistake into cross-tenant delivery.
func TestSessionOwners_SendingMiddleware_DeliversOnlyToTheOwningSession(t *testing.T) {
	const (
		mine   = "owner-mine"
		theirs = "owner-theirs"
		uri    = "gitlab://project/42/pipeline/99"
	)

	owned, _ := connectedSessions(t)
	unrecorded, _ := connectedSessions(t)

	tests := []struct {
		name      string
		session   *mcp.ServerSession
		tag       string
		wantSent  bool
		wantWhyNo string
	}{
		{
			name:     "the owning session receives it",
			session:  owned,
			tag:      mine,
			wantSent: true,
		},
		{
			name:      "another credential's notification is dropped",
			session:   owned,
			tag:       theirs,
			wantWhyNo: "a session was sent a notification produced by another credential's watcher",
		},
		{
			name:      "an untagged notification is dropped",
			session:   owned,
			tag:       "",
			wantWhyNo: "an untagged notification was delivered; nothing could say which credential produced it",
		},
		{
			name:      "a session with no recorded owner is dropped",
			session:   unrecorded,
			tag:       mine,
			wantWhyNo: "a notification reached a session whose credential this server never recorded",
		},
		{
			// The two absences together, which is the only case that pins the
			// untagged check as a check. Delete it and the value falls through
			// to the comparison below, where "" equals the "" an unrecorded
			// session answers with, and every untagged notification is
			// delivered to every session this server never recorded. The four
			// cases above all survive that deletion: each has exactly one of
			// the two absences, so the comparison still refuses them for the
			// other one.
			name:    "an untagged notification to a session with no recorded owner is dropped",
			session: unrecorded,
			tag:     "",
			wantWhyNo: "an untagged notification was delivered to a session this server never recorded, " +
				"which is the pair of absences that compare equal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := updateParams(tt.tag, uri)
			for shape, req := range deliveryShapes(tt.session, params) {
				t.Run(shape, func(t *testing.T) {
					owners := newSessionOwners(false)
					owners.record(owned, mine)

					got := runSendingMiddleware(t, owners, req)

					if !tt.wantSent {
						assertNotDelivered(t, got, tt.wantWhyNo)
						return
					}
					assertDelivered(t, got, req, params, uri)
				})
			}
		})
	}
}

// sendingOutcome is what one trip through [sessionOwners.sendingMiddleware]
// produced: what the middleware returned, and what the handler behind it saw.
type sendingOutcome struct {
	result mcp.Result
	// calls, forwarded, seenParams and seenMeta are what the next handler
	// observed.
	calls      int
	forwarded  mcp.Request
	seenParams *mcp.ResourceUpdatedNotificationParams
	seenMeta   mcp.Meta
	// handedMeta is the `_meta` of the params the middleware was handed, read
	// inside the next handler, which is while the send is in flight. On the
	// legacy path the SDK hands one params value to every subscriber, and
	// go-sdk's main after v1.8.0 sends to each on a goroutine of its own, so
	// whatever the middleware wrote there is what another session's send
	// reads at that moment; a write put back afterwards is invisible to any
	// check made once the middleware has returned.
	handedMeta mcp.Meta
}

// runSendingMiddleware drives one notification through the filter.
func runSendingMiddleware(t *testing.T, owners *sessionOwners, req mcp.Request) sendingOutcome {
	t.Helper()

	var got sendingOutcome
	handed, _ := req.GetParams().(*mcp.ResourceUpdatedNotificationParams)
	handler := owners.sendingMiddleware(func(_ context.Context, _ string, sent mcp.Request) (mcp.Result, error) {
		got.calls++
		got.forwarded = sent
		if params, ok := sent.GetParams().(*mcp.ResourceUpdatedNotificationParams); ok {
			got.seenParams = params
			got.seenMeta = params.Meta
		}
		if handed != nil {
			got.handedMeta = handed.Meta
		}
		return delivered, nil
	})

	result, err := handler(t.Context(), notificationResourceUpdated, req)
	if err != nil {
		t.Fatalf("sendingMiddleware returned %v; a filtered notification is not an error", err)
	}
	got.result = result
	return got
}

// assertNotDelivered checks that a notification the filter refused reached
// nothing at all, and says why it had to be refused when it did.
func assertNotDelivered(t *testing.T, got sendingOutcome, why string) {
	t.Helper()
	if got.calls != 0 {
		t.Error(why)
	}
	if got.result != nil {
		t.Errorf("result = %v, want nil; a dropped notification reaches no handler", got.result)
	}
}

// assertDelivered checks a notification the filter passed on: the result
// travels through untouched; what reaches the handler is a request of the same
// shape for the same session, carrying a copy of the params with the same URI,
// the SDK's own metadata and no private owner tag; and the params the
// middleware was handed are never written to, during the send or after it.
func assertDelivered(
	t *testing.T,
	got sendingOutcome,
	req mcp.Request,
	params *mcp.ResourceUpdatedNotificationParams,
	uri string,
) {
	t.Helper()

	if got.calls != 1 {
		t.Fatalf("next was called %d time(s), want 1 for the credential's own session", got.calls)
	}
	if got.result != delivered {
		t.Errorf("result = %v, want the next handler's own; the middleware swallowed it", got.result)
	}
	// Each delivery path's request is rebuilt as itself, so whatever the SDK
	// does with it after the middleware is what it would have done with the
	// request it handed in.
	if gotType, wantType := reflect.TypeOf(got.forwarded), reflect.TypeOf(req); gotType != wantType {
		t.Errorf("the middleware forwarded a %v, want a %v like the one it was given", gotType, wantType)
	}
	if got.forwarded.GetSession() != req.GetSession() {
		t.Error("the middleware forwarded the notification to another session than the one it was given")
	}
	if got.seenParams == nil {
		t.Fatal("the forwarded request carries no resource-updated params")
	}
	if got.seenParams == params {
		t.Error("the middleware forwarded the params it was handed; " +
			"the legacy path shares that value with every other subscriber's send")
	}
	if got.seenParams.URI != uri {
		t.Errorf("uri = %q, want %q carried through unchanged", got.seenParams.URI, uri)
	}
	if _, leaked := got.seenMeta[ownerMetaKey]; leaked {
		t.Error("the private owner tag reached the client; " +
			"it names a pool entry and is nobody's business but this server's")
	}
	if _, kept := got.seenMeta[sdkSubscriptionIDKey]; !kept {
		t.Error("stripping the owner tag also dropped the SDK's own metadata")
	}
	// The params handed in keep the tag while the send is in flight and after
	// it. Another subscriber's send reads the same value on the legacy path,
	// so a stripped tag there makes that session look untagged and be dropped.
	if _, kept := got.handedMeta[ownerMetaKey]; !kept {
		t.Error("the middleware wrote to the params it was handed during the send; " +
			"a concurrent send to another session of the same credential reads them as untagged")
	}
	if _, kept := params.Meta[ownerMetaKey]; !kept {
		t.Error("the params handed in lost their owner tag")
	}
}

// TestSessionOwners_SendingMiddleware_OneNotificationReachesEverySessionOfItsOwner
// hands the SAME params value to two sessions of one credential, which is what
// the legacy path does, since it does not copy the params at all, and checks
// that the middleware leaves that value exactly as it found it: the owner tag
// is still on it while each send is in flight, and the map is the same map.
//
// A middleware that stripped the owner key and left it stripped drops the
// second session onwards. One that stripped it and put it back after the send
// passes as long as the SDK delivers one session at a time, and loses
// notifications once it delivers to several at once, which go-sdk's main after
// v1.8.0 does: the second send then reads the params while the first has them
// stripped. Checking the params inside the handler is what tells the two apart
// without a concurrent SDK, and [TestSessionOwners_SendingMiddleware_RealServer_EveryLegacySessionOfAnOwnerReceivesEveryNotification]
// drives the real one.
func TestSessionOwners_SendingMiddleware_OneNotificationReachesEverySessionOfItsOwner(t *testing.T) {
	const owner = "owner-mine"
	owners := newSessionOwners(false)
	first, _ := connectedSessions(t)
	second, _ := connectedSessions(t)
	owners.record(first, owner)
	owners.record(second, owner)

	params := updateParams(owner, "gitlab://project/42/pipeline/99")
	originalMeta := params.Meta

	var sends, strippedWhileSending int
	handler := owners.sendingMiddleware(func(context.Context, string, mcp.Request) (mcp.Result, error) {
		sends++
		if params.Meta[ownerMetaKey] != owner {
			strippedWhileSending++
		}
		return delivered, nil
	})

	// Written out rather than looped, because these are two dependent steps
	// and not two cases: the second send is the assertion, and it only means
	// anything after the first has been made.
	send := func(session *mcp.ServerSession) {
		t.Helper()
		req := &mcp.ServerRequest[*mcp.ResourceUpdatedNotificationParams]{Session: session, Params: params}
		if _, err := handler(t.Context(), notificationResourceUpdated, req); err != nil {
			t.Fatalf("sendingMiddleware returned %v", err)
		}
	}
	send(first)
	send(second)

	if sends != 2 {
		t.Errorf("%d of 2 sessions of one credential received the notification; "+
			"the owner tag was missing from the shared params at the second send", sends)
	}
	if strippedWhileSending != 0 {
		t.Errorf("the params handed in had no owner tag during %d of %d sends; "+
			"a concurrent send to another session would read them as untagged and drop it",
			strippedWhileSending, sends)
	}
	if params.Meta[ownerMetaKey] != owner {
		t.Errorf("the tag left on the params is %v, want %q untouched", params.Meta[ownerMetaKey], owner)
	}
	if reflect.ValueOf(params.Meta).UnsafePointer() != reflect.ValueOf(originalMeta).UnsafePointer() {
		t.Error("the params handed in carry another _meta map than the one they arrived with")
	}
	if _, mutated := originalMeta[ownerMetaKey]; !mutated {
		t.Error("the caller's own _meta map was mutated; the SDK shares it between per-session copies " +
			"and stamps its own subscription id into it")
	}
}

// TestSessionOwners_SendingMiddleware_AnUnknownRequestShape_IsDropped covers
// the request the filter cannot rebuild. It forwards a new request of the
// shape it was handed, so a notification arriving in a third shape, which no
// delivery path of the SDK this server builds against produces, is dropped
// rather than forwarded as it came: forwarding it would hand the client the
// private owner tag, or write the stripped copy into params the SDK shares.
func TestSessionOwners_SendingMiddleware_AnUnknownRequestShape_IsDropped(t *testing.T) {
	const owner = "owner-mine"
	owners := newSessionOwners(false)
	session, _ := connectedSessions(t)
	owners.record(session, owner)

	params := updateParams(owner, "gitlab://project/42/pipeline/99")
	req := &mcp.ServerRequest[unfamiliarParams]{Session: session, Params: params}

	got := runSendingMiddleware(t, owners, req)

	assertNotDelivered(t, got, "a notification in a request shape the filter does not rebuild was delivered")
	if params.Meta[ownerMetaKey] != owner {
		t.Error("the params of a dropped notification were written to")
	}
}

// unfamiliarParams instantiates the SDK's request type with a params type of
// its own, which makes a request shape neither delivery path produces.
type unfamiliarParams interface{ mcp.Params }

// TestSessionOwners_SendingMiddleware_RealServer_EveryLegacySessionOfAnOwnerReceivesEveryNotification
// drives the filter through a real server, with several sessions of one
// credential subscribed to one URI over resources/subscribe on protocol
// 2025-11-25, which is the delivery path that hands every session the same
// params value.
//
// The unit tests above call the middleware by hand, one send at a time, which
// is how go-sdk v1.8.0 delivers. go-sdk's main after v1.8.0 sends to each
// session on a goroutine of its own, and a middleware that wrote a stripped
// `_meta` into the shared params and put it back after its send lost about
// four notifications in ten with two sessions, and with eight sent the owner
// tag to a client, in an ordinary run. This test is what fails on the go-sdk
// bump that brings that change if the middleware ever writes to the params
// again, since the race detector does not run on a pull request.
func TestSessionOwners_SendingMiddleware_RealServer_EveryLegacySessionOfAnOwnerReceivesEveryNotification(t *testing.T) {
	const (
		owner    = "owner-mine"
		uri      = "gitlab://project/42/pipeline/99"
		sessions = 4
		rounds   = 100
	)
	ctx := t.Context()
	owners := newSessionOwners(false)
	server := mcp.NewServer(&mcp.Implementation{Name: "host", Version: "1"}, &mcp.ServerOptions{
		SubscribeHandler: func(_ context.Context, req *mcp.SubscribeRequest) error {
			owners.record(req.Session, owner)
			return nil
		},
		UnsubscribeHandler: func(context.Context, *mcp.UnsubscribeRequest) error { return nil },
	})
	server.AddSendingMiddleware(owners.sendingMiddleware)

	// One count per session, because the promise is per session: a delivery
	// that sent some sessions a notification twice and others not at all
	// would reach the right total.
	var received [sessions]atomic.Int64
	var complete, leaked atomic.Int64
	all := make(chan struct{})
	var once sync.Once
	for i := range sessions {
		st, ct := mcp.NewInMemoryTransports()
		if _, err := server.Connect(ctx, st, nil); err != nil {
			t.Fatalf("server connect: %v", err)
		}
		client := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "1"}, &mcp.ClientOptions{
			ResourceUpdatedHandler: func(_ context.Context, req *mcp.ResourceUpdatedNotificationRequest) {
				if _, ok := req.Params.Meta[ownerMetaKey]; ok {
					leaked.Add(1)
				}
				if received[i].Add(1) == rounds && complete.Add(1) == sessions {
					once.Do(func() { close(all) })
				}
			},
		})
		session, err := client.Connect(ctx, ct, &mcp.ClientSessionOptions{ProtocolVersion: "2025-11-25"})
		if err != nil {
			t.Fatalf("client connect: %v", err)
		}
		t.Cleanup(func() { _ = session.Close() })
		// resources/subscribe is answered after the SDK has filed the
		// subscription, so every session is in its table once this returns.
		if subErr := session.Subscribe(ctx, &mcp.SubscribeParams{URI: uri}); subErr != nil {
			t.Fatalf("Subscribe: %v", subErr)
		}
	}

	for range rounds {
		if err := server.ResourceUpdated(ctx, updateParams(owner, uri)); err != nil {
			t.Fatalf("ResourceUpdated: %v", err)
		}
	}

	counts := func() []int64 {
		got := make([]int64, sessions)
		for i := range received {
			got[i] = received[i].Load()
		}
		return got
	}
	select {
	case <-all:
	case <-time.After(30 * time.Second):
		t.Fatalf("the %d sessions of one credential received %v of %d notifications each, %d of them carrying the owner tag",
			sessions, counts(), rounds, leaked.Load())
	}
	// A count past rounds after the last session completed is a duplicate,
	// which the wait above cannot see.
	for i, n := range counts() {
		if n != rounds {
			t.Errorf("session %d received %d notifications, want %d", i, n, rounds)
		}
	}
	if n := leaked.Load(); n != 0 {
		t.Errorf("the private owner tag reached a client %d time(s)", n)
	}
}

// TestSessionOwners_SendingMiddleware_LeavesEveryOtherMethodAlone verifies the
// filter is scoped to the one notification it exists for.
//
// It is a sending middleware, so every server-initiated message passes through
// it: a tools list-changed notification, a logging message, an elicitation
// request. Judging any of those by an owner tag they never carry would drop
// them all.
func TestSessionOwners_SendingMiddleware_LeavesEveryOtherMethodAlone(t *testing.T) {
	owners := newSessionOwners(false)

	methods := []string{
		"notifications/tools/list_changed",
		"notifications/resources/list_changed",
		"notifications/message",
		"elicitation/create",
	}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			passed := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{Params: &mcp.CallToolParamsRaw{Name: "irrelevant"}}
			var forwarded mcp.Request
			handler := owners.sendingMiddleware(func(_ context.Context, _ string, req mcp.Request) (mcp.Result, error) {
				forwarded = req
				return &mcp.CallToolResult{}, nil
			})

			result, err := handler(t.Context(), method, passed)
			if err != nil {
				t.Fatalf("handler(%s) = %v, want the message passed straight through", method, err)
			}
			if result == nil {
				t.Errorf("handler(%s) swallowed the result of the next handler", method)
			}
			if forwarded != mcp.Request(passed) {
				t.Errorf("handler(%s) did not forward the very request it was given", method)
			}
		})
	}
}

// TestSessionOwners_SendingMiddleware_MalformedResourceUpdate_IsDropped covers
// the two shapes a resource-updated notification can arrive in that carry no
// owner to judge: the wrong request type under that method name, and a request
// with no params at all.
//
// Neither can happen from this server, which is exactly why they are dropped
// rather than forwarded: an unattributable notification on the one method this
// filter exists for is a wiring defect, and delivering it would defeat the
// filter.
func TestSessionOwners_SendingMiddleware_MalformedResourceUpdate_IsDropped(t *testing.T) {
	owners := newSessionOwners(false)
	session, clientSession := connectedSessions(t)
	owners.record(session, "owner-mine")

	tests := map[string]mcp.Request{
		"params of another type under that method": &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
			Session: session,
			Params:  &mcp.CallToolParamsRaw{Name: "not-a-notification"},
		},
		"a resource update with no params": &mcp.ServerRequest[*mcp.ResourceUpdatedNotificationParams]{
			Session: session,
		},
		"a resource update addressed to no server session": &mcp.ClientRequest[*mcp.ResourceUpdatedNotificationParams]{
			Session: clientSession,
			Params:  updateParams("owner-mine", "gitlab://project/42/pipeline/99"),
		},
	}

	for name, req := range tests {
		t.Run(name, func(t *testing.T) {
			var calls int
			handler := owners.sendingMiddleware(func(context.Context, string, mcp.Request) (mcp.Result, error) {
				calls++
				return delivered, nil
			})

			if _, err := handler(t.Context(), notificationResourceUpdated, req); err != nil {
				t.Fatalf("handler returned %v, want a silent drop", err)
			}
			if calls != 0 {
				t.Error("a notification that could not be attributed to a credential was delivered anyway")
			}
		})
	}
}

// TestSessionOwners_SendingMiddleware_ANilRequest_IsPassedOn covers the guard
// in front of the params read.
//
// Nothing this server sends arrives with no request at all, and the middleware
// wraps every server-initiated message rather than only the notification it
// filters, so the guard is what keeps it from dereferencing one on the way to
// the SDK's own error handling.
func TestSessionOwners_SendingMiddleware_ANilRequest_IsPassedOn(t *testing.T) {
	owners := newSessionOwners(false)
	var calls int
	handler := owners.sendingMiddleware(func(context.Context, string, mcp.Request) (mcp.Result, error) {
		calls++
		return delivered, nil
	})

	if _, err := handler(t.Context(), notificationResourceUpdated, nil); err != nil {
		t.Fatalf("handler returned %v", err)
	}
	if calls != 1 {
		t.Error("a request with nothing to filter was swallowed rather than passed on")
	}
}

// TestMetaWithout_CopiesRatherThanMutating covers the helper the filter clones
// with, including the nil map the SDK is free to hand it.
func TestMetaWithout_CopiesRatherThanMutating(t *testing.T) {
	t.Run("nil stays nil", func(t *testing.T) {
		if got := metaWithout(nil, ownerMetaKey); got != nil {
			t.Errorf("metaWithout(nil) = %v, want nil rather than an empty map", got)
		}
	})

	t.Run("the original keeps the key", func(t *testing.T) {
		original := mcp.Meta{ownerMetaKey: "owner", "keep": "me"}

		stripped := metaWithout(original, ownerMetaKey)

		if _, gone := stripped[ownerMetaKey]; gone {
			t.Error("the copy still carries the key it was asked to drop")
		}
		if stripped["keep"] != "me" {
			t.Error("the copy dropped a key it was not asked about")
		}
		if _, still := original[ownerMetaKey]; !still {
			t.Error("the original map was mutated; the SDK shares it between per-session copies")
		}
	})
}

// TestSessionOwners_Record_IsIdempotentAndFilesTheSessionID covers the
// recording that replaced the session tag.
//
// Recording is attempted on every request rather than once, because the session
// ID is not reliably readable at every point a session could first be seen and
// a session whose owner is unknown is refused rather than served. So a repeat
// must be free, and it must still fill in an ID that was missing the first
// time.
func TestSessionOwners_Record_IsIdempotentAndFilesTheSessionID(t *testing.T) {
	const owner = "owner-mine"
	owners := newSessionOwners(false)
	session := newIdentifiedSessions(t).connect(t)

	owners.record(session, owner)
	owners.record(session, owner)

	if got := owners.ownerOf(session); got != owner {
		t.Errorf("ownerOf = %q, want %q", got, owner)
	}
	if got := owners.ownerOfID(session.ID()); got != owner {
		t.Errorf("ownerOfID(%q) = %q, want %q", session.ID(), got, owner)
	}

	owners.mu.Lock()
	sessions, ids := len(owners.sessionsByOwner[owner]), len(owners.idsByOwner[owner])
	owners.mu.Unlock()
	if sessions != 1 || ids != 1 {
		t.Errorf("the owner holds %d session(s) and %d id(s) after recording the same one twice, want 1 and 1",
			sessions, ids)
	}
}

// TestSessionOwners_Record_ASessionRebound_FollowsTheNewCredential covers a
// session whose next request arrives as a different pool entry, which is what a
// credential rebuilt after eviction looks like: the token is the same and the
// owner is not.
//
// The latest record wins, for both lookups. Anything else would leave the gate
// comparing a live session against a credential the pool no longer holds.
//
// The reverse indexes have to follow too, and asserting only the forward
// lookups is what let them drift: the previous owner went on listing this
// session, so its eviction would have dropped a claim belonging to the
// credential that now holds it, and the session ID would have been forgotten
// from under a live session. No HTTP path reaches the rebinding today, since a
// session presented with a different credential is refused, and that refusal is
// decided from this very map.
func TestSessionOwners_Record_ASessionRebound_FollowsTheNewCredential(t *testing.T) {
	owners := newSessionOwners(false)
	session := newIdentifiedSessions(t).connect(t)

	owners.record(session, "owner-before")
	owners.record(session, "owner-after")

	if got := owners.ownerOf(session); got != "owner-after" {
		t.Errorf("ownerOf = %q, want the credential that most recently claimed the session", got)
	}
	if got := owners.ownerOfID(session.ID()); got != "owner-after" {
		t.Errorf("ownerOfID = %q, want the credential that most recently claimed the session", got)
	}

	owners.mu.Lock()
	staleSessions := len(owners.sessionsByOwner["owner-before"])
	staleIDs := len(owners.idsByOwner["owner-before"])
	owners.mu.Unlock()
	if staleSessions != 0 || staleIDs != 0 {
		t.Errorf("the previous owner still lists %d session(s) and %d id(s) of a session it no longer holds",
			staleSessions, staleIDs)
	}

	t.Run("the previous owner's eviction leaves it alone", func(t *testing.T) {
		owners.forgetOwner("owner-before")

		if got := owners.ownerOf(session); got != "owner-after" {
			t.Errorf("ownerOf = %q after the previous owner was evicted, want the current one", got)
		}
		if got := owners.ownerOfID(session.ID()); got != "owner-after" {
			t.Errorf("ownerOfID = %q after the previous owner was evicted, want the current one", got)
		}
	})

	t.Run("the current owner's eviction takes it", func(t *testing.T) {
		owners.forgetOwner("owner-after")

		if got := owners.ownerOf(session); got != "" {
			t.Errorf("ownerOf = %q after the owning credential was evicted, want none", got)
		}
	})
}

// TestSessionOwners_OwnerOfID_AnswersOnlyForWhatItRecorded pins the lookup the
// gate refuses on.
//
// An ID this deployment never recorded must read as unowned rather than as
// "owned by nobody in particular": the gate compares the answer to the entry's
// own token, and an empty answer that compared equal to an entry with no token
// would hand somebody else's session over.
func TestSessionOwners_OwnerOfID_AnswersOnlyForWhatItRecorded(t *testing.T) {
	owners := newSessionOwners(false)
	recorded := newIdentifiedSessions(t).recordUnder(t, owners, "owner-mine")

	tests := []struct {
		name   string
		owners *sessionOwners
		id     string
		want   string
	}{
		{name: "a recorded id", owners: owners, id: recorded, want: "owner-mine"},
		{name: "an id from another deployment", owners: owners, id: "never-seen-here"},
		{name: "no id at all", owners: owners, id: ""},
		{name: "no table at all", owners: nil, id: recorded},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.owners.ownerOfID(tt.id); got != tt.want {
				t.Errorf("ownerOfID(%q) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}

// TestSessionOwners_Forget_DropsOneSessionAndLeavesTheRest covers the
// disconnect path: one session ending must not take its neighbours' claims with
// it, and forgetting a session nobody recorded must be a no-op rather than a
// panic.
func TestSessionOwners_Forget_DropsOneSessionAndLeavesTheRest(t *testing.T) {
	const owner = "owner-mine"
	owners := newSessionOwners(false)
	minted := newIdentifiedSessions(t)

	first, second := minted.connect(t), minted.connect(t)
	owners.record(first, owner)
	owners.record(second, owner)
	firstID, secondID := first.ID(), second.ID()

	owners.forget(first)

	if got := owners.ownerOf(first); got != "" {
		t.Errorf("ownerOf(forgotten) = %q, want it dropped", got)
	}
	if got := owners.ownerOfID(firstID); got != "" {
		t.Errorf("ownerOfID(forgotten) = %q, want it dropped", got)
	}
	if got := owners.ownerOf(second); got != owner {
		t.Errorf("ownerOf(other) = %q, want %q; one disconnect took another session's claim with it", got, owner)
	}
	if got := owners.ownerOfID(secondID); got != owner {
		t.Errorf("ownerOfID(other) = %q, want %q", got, owner)
	}

	// Forgetting again, and forgetting one that was never recorded, are both
	// reached in production: the disconnect waiter runs regardless of whether
	// eviction already forgot the owner.
	owners.forget(first)
	owners.forget(minted.connect(t))

	// The session left behind is still filed under its owner, so the owner's
	// eviction still finds it: a disconnect that took the owner's whole index
	// with it would leave the survivor's ID accepted by the gate after the
	// credential was gone, and its client never told.
	orphaned := owners.forgetOwner(owner)
	if len(orphaned) != 1 || orphaned[0] != second {
		t.Errorf("forgetOwner after one disconnect returned %v, want only the session still connected", orphaned)
	}
	if got := owners.ownerOfID(secondID); got != "" {
		t.Errorf("ownerOfID(survivor) = %q after its credential was evicted, want it dropped", got)
	}
}

// TestSessionOwners_Forget_TheLastSessionOfAnOwner_LeavesNoIndexBehind verifies
// the per-owner indexes empty out with the sessions they hold.
//
// Owners are minted per pool entry, and every credential the pool ever saw is
// a new one, so an index kept after its last session is gone is an entry per
// departed credential for the life of the process.
func TestSessionOwners_Forget_TheLastSessionOfAnOwner_LeavesNoIndexBehind(t *testing.T) {
	const owner = "owner-departed"
	owners := newSessionOwners(false)
	session := newIdentifiedSessions(t).connect(t)
	owners.record(session, owner)

	owners.forget(session)

	owners.mu.Lock()
	_, sessionsKept := owners.sessionsByOwner[owner]
	_, idsKept := owners.idsByOwner[owner]
	owners.mu.Unlock()
	if sessionsKept || idsKept {
		t.Errorf("after its last session ended the owner still has a session index (%t) or an id index (%t)",
			sessionsKept, idsKept)
	}
}

// TestSessionOwners_Forget_ASessionMissingFromThePerOwnerIndexes_IsStillDropped
// covers the two guards inside forget's bookkeeping.
//
// record writes all three maps under one lock, so no wiring produces a session
// recorded in bySession that the per-owner indexes have never heard of. The
// state is built here directly, because the guards exist for exactly that
// inconsistency and the alternative to them is a delete on a nil map while the
// primary index has already dropped the session: the entry would be gone from
// one map and left in another, and the next lookup would answer for a session
// nobody holds.
func TestSessionOwners_Forget_ASessionMissingFromThePerOwnerIndexes_IsStillDropped(t *testing.T) {
	const owner = "owner-mine"
	owners := newSessionOwners(false)
	session := newIdentifiedSessions(t).connect(t)

	// Recorded in the primary indexes only.
	owners.mu.Lock()
	owners.bySession[session] = owner
	owners.byID[session.ID()] = owner
	owners.mu.Unlock()

	owners.forget(session)

	if got := owners.ownerOf(session); got != "" {
		t.Errorf("ownerOf = %q after forget, want it dropped", got)
	}
	if got := owners.ownerOfID(session.ID()); got != "" {
		t.Errorf("ownerOfID = %q after forget, want it dropped", got)
	}
}

// TestSessionOwners_ForgetOwner_DropsEverySessionOfAnEvictedCredential covers
// what eviction does.
//
// Without it a session outlives the credential it belonged to: the gate would
// go on accepting its ID, and a notification tagged with the rebuilt entry's
// owner would be filtered away from a session the client still holds. Another
// credential's sessions must survive it, or one eviction would end every
// tenant's session on the shared server.
func TestSessionOwners_ForgetOwner_DropsEverySessionOfAnEvictedCredential(t *testing.T) {
	owners := newSessionOwners(false)
	minted := newIdentifiedSessions(t)

	evicted := map[string]*mcp.ServerSession{
		"the first session of the evicted credential":  minted.connect(t),
		"the second session of the evicted credential": minted.connect(t),
	}
	for _, session := range evicted {
		owners.record(session, "owner-evicted")
	}
	survivor := minted.connect(t)
	owners.record(survivor, "owner-kept")

	owners.forgetOwner("owner-evicted")

	for name, session := range evicted {
		t.Run(name, func(t *testing.T) {
			if got := owners.ownerOf(session); got != "" {
				t.Errorf("ownerOf = %q, want it forgotten with its credential", got)
			}
			if got := owners.ownerOfID(session.ID()); got != "" {
				t.Errorf("ownerOfID = %q, want it forgotten with its credential", got)
			}
		})
	}
	if got := owners.ownerOf(survivor); got != "owner-kept" {
		t.Errorf("ownerOf(other credential) = %q, want %q; one eviction ended another tenant's session", got, "owner-kept")
	}

	owners.mu.Lock()
	sessions, ids := len(owners.sessionsByOwner["owner-evicted"]), len(owners.idsByOwner["owner-evicted"])
	owners.mu.Unlock()
	if sessions != 0 || ids != 0 {
		t.Errorf("the evicted owner's indexes still hold %d session(s) and %d id(s), want none", sessions, ids)
	}
}

// TestSessionOwners_ForgetOwner_ReturnsTheSessionsItOrphaned covers the answer
// eviction needs and cannot look up afterwards.
//
// Forgetting has to happen under the pool's write lock, so the gate stops
// accepting the ID at once; telling the client must not happen there, because
// the callback contract forbids blocking. The list is how the second half finds
// the sessions the first half has already taken out of the table.
func TestSessionOwners_ForgetOwner_ReturnsTheSessionsItOrphaned(t *testing.T) {
	owners := newSessionOwners(false)
	minted := newIdentifiedSessions(t)

	first, second := minted.connect(t), minted.connect(t)
	owners.record(first, "owner-evicted")
	owners.record(second, "owner-evicted")
	owners.record(minted.connect(t), "owner-kept")

	orphaned := owners.forgetOwner("owner-evicted")

	if len(orphaned) != 2 {
		t.Fatalf("forgetOwner returned %d sessions, want 2; the ones it left out are never told", len(orphaned))
	}
	returned := map[*mcp.ServerSession]bool{}
	for _, session := range orphaned {
		returned[session] = true
	}
	if !returned[first] || !returned[second] {
		t.Error("forgetOwner returned sessions other than the evicted credential's own")
	}
	if got := owners.forgetOwner("owner-never-pooled"); len(got) != 0 {
		t.Errorf("forgetOwner(unknown) returned %d sessions, want none", len(got))
	}
}

// TestSessionOwners_EndSessionsWithoutStreams_TellsOnlyTheClientsNoStreamTold
// covers the ending a session-era resources/subscribe gets.
//
// A legacy subscribe is not a request the client leaves open, so there is no
// stream for listenStreams.closeOwner to complete. Eviction stopped its
// watchers, Manager.Close fires no OnStop, and on --stateless=false the client
// was left holding a live session and a standalone SSE stream that would never
// carry anything again: not told, unable to tell, and promised the opposite by
// ADR-0020. Terminating the session is the only ending the protocol offers a
// subscriber with no open request.
//
// The two exclusions are what the cases below are for. A session a stream
// already ended must be left alone, because the SDK writes that stream's
// completion result as the handler unwinds and closing the connection would
// race it; and on the sessionless transport nothing is terminated at all, since
// each POST's session closes with its own response and a legacy subscribe is
// refused there anyway.
func TestSessionOwners_EndSessionsWithoutStreams_TellsOnlyTheClientsNoStreamTold(t *testing.T) {
	tests := []struct {
		name      string
		stateless bool
		// toldByStream marks the session as one listenStreams.closeOwner
		// already ended.
		toldByStream bool
		wantEnded    bool
	}{
		{
			name:      "a session holding only a legacy subscribe is terminated",
			wantEnded: true,
		},
		{
			name:         "a session a listen stream already ended is left alone",
			toldByStream: true,
		},
		{
			name:      "the sessionless transport terminates nothing",
			stateless: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owners := newSessionOwners(tt.stateless)
			session, client := connectedSessions(t)

			told := map[*mcp.ServerSession]struct{}{}
			if tt.toldByStream {
				told[session] = struct{}{}
			}

			// The nil entry rides along in every case: closeOwner reports no
			// session for the process-wide sentinel stream, and a nil in the
			// orphan list must not take the loop down.
			owners.endSessionsWithoutStreams([]*mcp.ServerSession{nil, session}, told)

			if tt.wantEnded {
				// The close is made in the call above, so a session that was
				// going to be closed already is; the window only covers the
				// connection unwinding, and is generous because it is only
				// spent when this fails.
				if !sessionEnded(t, session, 5*time.Second) {
					t.Error("the session was left open, so a client whose only activity was a " +
						"session-era subscribe is never told its credential is gone")
				}
				return
			}
			// A session left alone is shown to be alive rather than watched
			// for a while in case it dies: the decision was made in the call
			// above, so anything it closed is closed by now, and a round trip
			// the client completes says so at once. It says more than a window
			// does, too, since a session ended in a way its own Wait is slow
			// to notice would sit out any window and read as left alone.
			if err := pingWithin(t, client, 5*time.Second); err != nil {
				t.Errorf("the session was terminated (%v), which races the ending it was already being given", err)
			}
		})
	}

	t.Run("a table that does not exist ends nothing", func(t *testing.T) {
		var absent *sessionOwners
		absent.endSessionsWithoutStreams([]*mcp.ServerSession{nil}, nil)
	})

	t.Run("a session that is already gone is an ordinary outcome", func(t *testing.T) {
		// The client can disconnect between the eviction and this call, which
		// is why closing is done for its effect and not for its answer.
		owners := newSessionOwners(false)
		session, _ := connectedSessions(t)
		if err := session.Close(); err != nil {
			t.Fatalf("closing the session for the second-close case: %v", err)
		}
		owners.endSessionsWithoutStreams([]*mcp.ServerSession{session}, nil)
	})
}

// pingWithin asks the server over the client's own session and reports what
// came back, which is how a test says a session is still connected without
// waiting to see whether it stops being.
//
// The bound is what keeps a failure a failure: a session torn down under the
// client leaves the request with nobody to answer it, and without a deadline
// the test would hang instead of reporting.
func pingWithin(t *testing.T, client *mcp.ClientSession, within time.Duration) error {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), within)
	defer cancel()
	return client.Ping(ctx, nil)
}

// sessionEnded reports whether a server session has been closed, by waiting for
// its connection to unwind.
//
// The wait runs on a goroutine of its own because Wait blocks until the
// connection ends, and the verdict is taken on the test's goroutine. A session
// that is never closed leaves that goroutine parked until connectedSessions'
// cleanup closes the client end, which is what unblocks it.
func sessionEnded(t *testing.T, session *mcp.ServerSession, within time.Duration) bool {
	t.Helper()

	done := make(chan struct{})
	go func() {
		_ = session.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(within):
		return false
	}
}

// TestSessionOwners_ADisconnectedSession_IsForgotten covers the waiter record
// starts.
//
// Nothing else forgets an ordinary disconnect: eviction covers the credential
// going away, and a client simply closing its transport is far more common. A
// table that kept those would grow for the life of the process, and each entry
// keeps a session pointer reachable.
func TestSessionOwners_ADisconnectedSession_IsForgotten(t *testing.T) {
	owners := newSessionOwners(false)
	session, clientSession := connectedSessions(t)
	owners.record(session, "owner-mine")

	if err := clientSession.Close(); err != nil {
		t.Fatalf("closing the client session: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for owners.ownerOf(session) != "" {
		if time.Now().After(deadline) {
			t.Fatal("a session that disconnected is still recorded; the table grows for the life of the process")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestSessionOwners_NothingToRecord_IsANoOp covers every guard that makes the
// table safe to call unconditionally.
//
// It is called from a middleware that runs on every request of every transport,
// including the ones that bind no credential at all, so a nil table, a nil
// session and an empty owner all have to be ordinary answers rather than
// panics.
func TestSessionOwners_NothingToRecord_IsANoOp(t *testing.T) {
	var absent *sessionOwners
	populated := newSessionOwners(false)
	session, _ := connectedSessions(t)

	absent.record(session, "owner")
	absent.forget(session)
	absent.forgetOwner("owner")
	populated.record(nil, "owner")
	populated.record(session, "")
	populated.forget(nil)
	populated.forgetOwner("")

	if got := absent.ownerOf(session); got != "" {
		t.Errorf("ownerOf on a nil table = %q, want %q", got, "")
	}
	if got := populated.ownerOf(nil); got != "" {
		t.Errorf("ownerOf(nil session) = %q, want %q", got, "")
	}
	if got := populated.ownerOf(session); got != "" {
		t.Errorf("ownerOf = %q after recording under no owner, want nothing recorded", got)
	}
}

// TestSessionOwners_RecordingMiddleware_RecordsOnlyWhatWillBeRead covers the
// middleware that does the recording, and the filter in front of it.
//
// It runs inside the credential binding, so what it sees on a request nothing
// bound is no owner at all: stdio, the in-memory transport the e2e suite
// drives, and every test that builds a server directly. Recording those under
// an empty owner would make the gate compare a live session against "", which
// is what an unrecorded session already reads as.
//
// The filter is the other half. On the default stateless transport every POST
// gets a session of its own that closes with the response, so recording one
// costs a map entry and a goroutine parked on Wait for the length of one
// request, for a fact nothing will read: it has no ID for the gate to check and
// can receive no notification after its POST is over. The two cases that ARE
// read are the two recorded — a session with an ID, which the gate checks on
// every later request, and a subscribe by either method, which is what makes a
// session able to receive a resource update.
func TestSessionOwners_RecordingMiddleware_RecordsOnlyWhatWillBeRead(t *testing.T) {
	ephemeral, clientSession := connectedSessions(t)
	identified := newIdentifiedSessions(t).connect(t)

	tests := []struct {
		name    string
		bind    bool
		method  string
		session *mcp.ServerSession
		// req overrides the request built from session; noRequest asks for a
		// literal nil, which a zero req could not express.
		req       mcp.Request
		noRequest bool
		stateless bool
		wantOwner string
	}{
		{
			name:      "a stateful session, whose id the gate will check",
			bind:      true,
			method:    "tools/call",
			session:   identified,
			wantOwner: "owner-mine",
		},
		{
			name:      "a listen, which makes the session able to receive updates",
			bind:      true,
			method:    methodSubscriptionsListen,
			session:   ephemeral,
			wantOwner: "owner-mine",
		},
		{
			name:      "a legacy subscribe, for the same reason",
			bind:      true,
			method:    methodResourcesSubscribe,
			session:   ephemeral,
			wantOwner: "owner-mine",
		},
		{
			// The sessionless transport refuses this method before it
			// subscribes anything, so the session it arrived on can never
			// receive a notification and recording it buys nothing.
			name:      "a legacy subscribe on the sessionless transport",
			bind:      true,
			stateless: true,
			method:    methodResourcesSubscribe,
			session:   ephemeral,
		},
		{
			// The listen path works there, so it is recorded on both.
			name:      "a listen on the sessionless transport",
			bind:      true,
			stateless: true,
			method:    methodSubscriptionsListen,
			session:   ephemeral,
			wantOwner: "owner-mine",
		},
		{
			name:    "an ordinary call on a session with no id",
			bind:    true,
			method:  "tools/call",
			session: ephemeral,
		},
		{
			name:    "a request nothing bound",
			method:  "tools/call",
			session: identified,
		},
		{
			name:      "a bound request with no request at all",
			bind:      true,
			method:    "tools/call",
			session:   identified,
			noRequest: true,
		},
		{
			name:    "a bound request whose session is not a server session",
			bind:    true,
			method:  methodSubscriptionsListen,
			session: ephemeral,
			req:     &mcp.ClientRequest[*mcp.CallToolParamsRaw]{Session: clientSession, Params: &mcp.CallToolParamsRaw{}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owners := newSessionOwners(tt.stateless)
			var reached bool
			handler := owners.recordingMiddleware(func(context.Context, string, mcp.Request) (mcp.Result, error) {
				reached = true
				return &mcp.CallToolResult{}, nil
			})

			ctx := t.Context()
			if tt.bind {
				ctx = withCredentialState(ctx, &credentialState{owner: "owner-mine"})
			}
			var req mcp.Request
			switch {
			case tt.noRequest:
			case tt.req != nil:
				req = tt.req
			default:
				req = &mcp.ServerRequest[*mcp.CallToolParamsRaw]{Session: tt.session, Params: &mcp.CallToolParamsRaw{}}
			}

			if _, err := handler(ctx, tt.method, req); err != nil {
				t.Fatalf("recordingMiddleware returned %v; it records, it does not judge", err)
			}
			if !reached {
				t.Error("the request was not passed on")
			}
			if got := owners.ownerOf(tt.session); got != tt.wantOwner {
				t.Errorf("ownerOf = %q, want %q", got, tt.wantOwner)
			}
		})
	}
}
