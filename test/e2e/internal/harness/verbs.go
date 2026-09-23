//go:build e2e

// verbs.go is everything a client can ask of a server that is not a tool call.
//
// They are here rather than beside the tool verbs because they are a different
// kind of question. A tool call is about an action, which the projection knows
// how to spell on three surfaces; a resource read, a prompt, a completion and a
// subscription each address the server directly, by a URI or a name, and are
// the same on every surface.
//
// Raw is the escape hatch, and the only way a test reaches tools/call without
// naming an action. It exists for the protocol tests, where the subject is the
// envelope rather than what is inside it: a malformed argument, a tool name no
// surface registers, a call made to see how the server refuses it.

package harness

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ReadResource reads one resource, failing the test when the server refuses.
func (s *Session) ReadResource(uri string) *mcp.ReadResourceResult {
	s.env.T.Helper()

	ctx := s.attribute(s.env.Ctx, PurposeTest, ExpectationOK, callAttribution{target: uri})
	result, err := s.conn.client().ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
	if err != nil {
		s.env.T.Fatalf("resources/read %s: %v%s", uri, err, s.conn.failureContext())
	}
	return result
}

// TryReadResource reads one resource and hands both halves back, for a test
// whose subject is the refusal.
func (s *Session) TryReadResource(uri string) (*mcp.ReadResourceResult, error) {
	s.env.T.Helper()

	ctx := s.attribute(s.env.Ctx, PurposeTest, ExpectationAny, callAttribution{target: uri})
	return s.conn.client().ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
}

// GetPrompt renders one prompt, failing the test when the server refuses.
func (s *Session) GetPrompt(name string, arguments map[string]string) *mcp.GetPromptResult {
	s.env.T.Helper()

	ctx := s.attribute(s.env.Ctx, PurposeTest, ExpectationOK, callAttribution{target: name})
	result, err := s.conn.client().GetPrompt(ctx, &mcp.GetPromptParams{Name: name, Arguments: arguments})
	if err != nil {
		s.env.T.Fatalf("prompts/get %s: %v%s", name, err, s.conn.failureContext())
	}
	return result
}

// TryGetPrompt renders one prompt and hands both halves back.
func (s *Session) TryGetPrompt(name string, arguments map[string]string) (*mcp.GetPromptResult, error) {
	s.env.T.Helper()

	ctx := s.attribute(s.env.Ctx, PurposeTest, ExpectationAny, callAttribution{target: name})
	return s.conn.client().GetPrompt(ctx, &mcp.GetPromptParams{Name: name, Arguments: arguments})
}

// CompletePrompt asks for the values one prompt argument offers.
func (s *Session) CompletePrompt(prompt, argument, value string) []string {
	s.env.T.Helper()
	return s.complete(&mcp.CompleteReference{Type: "ref/prompt", Name: prompt}, argument, value)
}

// CompleteResource asks for the values one resource template variable offers.
func (s *Session) CompleteResource(uriTemplate, argument, value string) []string {
	s.env.T.Helper()
	return s.complete(&mcp.CompleteReference{Type: "ref/resource", URI: uriTemplate}, argument, value)
}

// complete sends one completion request and returns the values it answered.
func (s *Session) complete(ref *mcp.CompleteReference, argument, value string) []string {
	s.env.T.Helper()

	ctx := s.attribute(s.env.Ctx, PurposeTest, ExpectationOK, callAttribution{target: completionCallTarget(completionTarget(ref), argument)})
	result, err := s.conn.client().Complete(ctx, &mcp.CompleteParams{
		Ref:      ref,
		Argument: mcp.CompleteParamsArgument{Name: argument, Value: value},
	})
	if err != nil {
		s.env.T.Fatalf("completion/complete %s %s: %v%s", completionTarget(ref), argument, err, s.conn.failureContext())
		return nil
	}
	return result.Completion.Values
}

// completionTarget names what a completion reference addressed, for a message.
func completionTarget(ref *mcp.CompleteReference) string {
	if ref.Name != "" {
		return ref.Name
	}
	return ref.URI
}

// completionCallTarget spells what one completion covers: the reference and
// the argument together, since a template whose project variable completes
// says nothing about its branch variable.
//
// A session line's completion list is spelled through the same function, so
// a call and the denominator it is counted against cannot name one completion
// two ways.
func completionCallTarget(reference, argument string) string {
	return reference + " " + argument
}

// Raw sends one tools/call exactly as given, naming no action.
//
// Every other verb goes through the projection, which is what keeps a test
// from hard-coding a tool name that the surface it runs on does not register.
// This one deliberately does not, because the protocol tests need to send what
// no projection would produce.
func (s *Session) Raw(params *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	s.env.T.Helper()

	// PurposeRaw, so the coverage report never credits an action for a call
	// that named a tool directly: the subject of a raw call is the envelope,
	// and what the server made of it is somebody else's evidence.
	ctx := s.attribute(s.env.Ctx, PurposeRaw, ExpectationAny, callAttribution{})
	return s.conn.client().CallTool(ctx, params)
}

// Subscription is one resource this test is watching.
type Subscription struct {
	session *Session
	uri     string
	updates <-chan struct{}
	// release unsubscribes and unhooks the notifications, and closeOnce is
	// shared by Close and the test's cleanup, so a subscription closed early
	// is not unsubscribed a second time when the test ends.
	release   func()
	closeOnce sync.Once
}

// URI returns the resource this subscription watches.
func (s *Subscription) URI() string { return s.uri }

// Close stops watching the resource now rather than when the test ends.
//
// On protocol 2026-07-28 it is fire and forget: the SDK cancels the listen it
// opened for the URI and tells the server so in a notifications/cancelled, and
// the server drops its watcher when that arrives, a moment after Close has
// returned. A test that subscribes and closes in turn therefore holds its one
// open subscription plus the few whose cancellation is still on its way,
// which is well inside the server's cap of ten watchers per credential. That
// lag is also why a URI closed here must not be subscribed again on the same
// session: a second listen that reaches the server before the first one's
// teardown is acknowledged and then never delivers ([Session.TrySubscribe]
// says why), so such a test takes a private session per subscription.
func (s *Subscription) Close() { s.closeOnce.Do(s.release) }

// Subscribe asks the server to notify this session when a resource changes,
// and unsubscribes when the test ends.
//
// The first read the server makes is its authorization check, so a subscribe
// that is accepted means the credential could read the resource; one it
// refuses fails the test here rather than at the first update that never
// arrives. What counts as accepted is spelled out on [Session.TrySubscribe].
func (s *Session) Subscribe(uri string) *Subscription {
	s.env.T.Helper()

	subscription, err := s.subscribe(uri, ExpectationOK)
	if err != nil {
		s.env.T.Fatalf("resources/subscribe %s: %v%s", uri, err, s.conn.failureContext())
	}
	return subscription
}

// TrySubscribe asks the server to notify this session when a resource
// changes, and hands back the refusal when it will not, for a test that
// subscribes to things the server may decline, such as the subscription
// sweep.
//
// Accepted means the server said so. On protocol 2026-07-28 a subscription is
// a subscriptions/listen stream whose answer the SDK discards, so the only
// word the client ever gets is the notifications/subscriptions/acknowledged
// the server sends once every subscription the stream asked for succeeded;
// this waits for it, and a subscription it never arrives for is a refusal:
// the first read failed, or the session holds as many watchers as the server
// allows. Silence is the only signal of that refusal, so it costs the whole of
// subscribeAckTimeout before this returns. On an older protocol the subscribe
// is an ordinary request and its error is the refusal. Either way the
// subscribe is recorded with what came of it, so a refused one is never
// credited as an accepted subscription.
//
// One test at a time may watch a URI on one session, because the SDK keeps a
// single listen per URI and per session and answers a second Subscribe from
// it without asking the server: a second test would be told nothing, and
// closing either would end both. A second one is refused here, before anything
// is sent. A test that needs a URI to itself asks for a private session.
//
// A URI released by [Subscription.Close] can be claimed here again, but on
// protocol 2026-07-28 it must not be subscribed again on the same session
// until the server has torn down the earlier listen, and nothing reports when
// it has, since the SDK discards the listen's answer. A subscribe that lands
// first is acknowledged and recorded ok, while the SDK's per-session delivery
// table on the server has already lost it, so no update ever reaches the
// session. That is the SDK defect recorded in docs/development/upstream-bugs.md
// as "A session's second listen on a URI overwrites the first's subscription,
// and its close deletes both"; cmd/server's bridge keeps the watch itself
// alive, so what is lost is the delivery. A test that subscribes, closes and
// subscribes one URI again takes a private session for each subscription.
func (s *Session) TrySubscribe(uri string) (*Subscription, error) {
	s.env.T.Helper()
	return s.subscribe(uri, ExpectationAny)
}

// errSubscribedOnSession is the refusal a second subscription to one URI on
// one session gets.
var errSubscribedOnSession = errors.New("another test on this session already watches the resource")

// errNotAcknowledged is a subscription the server never acknowledged, which is
// how a refusal reads on protocol 2026-07-28.
var errNotAcknowledged = errors.New("the server did not acknowledge the subscription")

// subscribeAckTimeout bounds the wait for the server's acknowledgement. It
// covers the server's first read of the resource, which is a GitLab call, and
// is a variable so the harness's own tests can wait less than a real GitLab
// may take.
var subscribeAckTimeout = 30 * time.Second

// listenProtocolVersion is the first protocol revision on which a subscribe is
// a subscriptions/listen stream, spelled here because the SDK keeps its own
// constant unexported.
const listenProtocolVersion = "2026-07-28"

// subscribesByListening reports whether a session subscribes through a listen
// stream, whose answer the SDK drops, rather than through a request whose
// error is the answer. Revisions are dates, so they order as strings.
func subscribesByListening(session *mcp.ClientSession) bool {
	result := session.InitializeResult()
	return result != nil && result.ProtocolVersion >= listenProtocolVersion
}

// subscribe is the one path both subscribe verbs take: claim the URI on this
// session, subscribe, wait for the answer where the protocol hides it, record
// what came of it, and hand back a subscription whose cleanup is registered.
func (s *Session) subscribe(uri, expectation string) (*Subscription, error) {
	s.env.T.Helper()

	// The index is what lets an update notification be recorded against this
	// test: it arrives on the SDK's own goroutine, where nothing says whose
	// subscription it answers.
	unregister, claimed := s.conn.subscribers.claim(uri, s.env.recorder)
	if !claimed {
		return nil, fmt.Errorf("%w: %s", errSubscribedOnSession, uri)
	}
	updates := s.conn.notifier.watch(uri)
	// Watched before the subscribe is sent, since the acknowledgement can
	// arrive before Subscribe returns.
	acknowledged := s.conn.acks.watch(uri)
	defer s.conn.acks.forget(uri, acknowledged)

	listening := subscribesByListening(s.conn.client())
	subscribeCtx := s.attribute(s.env.Ctx, PurposeTest, expectation, callAttribution{target: uri})
	err := s.conn.client().Subscribe(subscribeCtx, &mcp.SubscribeParams{URI: uri})
	if err == nil && listening {
		err = s.awaitAcknowledgement(acknowledged)
	}
	// Recorded here only on protocol 2026-07-28, where the SDK's Subscribe opens
	// the subscription on a background context, so the attribution on
	// subscribeCtx never reaches the sending middleware and the subscribe would
	// be credited to no test. On an older protocol the request leaves through
	// the middleware on subscribeCtx, which records it with the same answer,
	// and a second line here would count it twice. See
	// [sessionConn.recordSubscribe].
	if listening {
		s.conn.recordSubscribe(s.env.recorder, uri, expectation, outcomeOf(methodSubscribe, nil, err))
	}

	// There is something to unsubscribe when the server agreed, and also when
	// it did not on 2026-07-28: the SDK keeps the listen it opened for the URI
	// whatever the server answered, and would answer a later Subscribe for the
	// URI from it without asking, so releasing it is what lets the URI be asked
	// again. Asked again on the same session, though, it is not reliable until
	// the server has torn the released listen down, which nothing reports; see
	// [Session.TrySubscribe]. A request an older protocol refused subscribed
	// nothing.
	held := err == nil || listening
	subscription := &Subscription{session: s, uri: uri, updates: updates}
	subscription.release = func() {
		if held {
			s.conn.unsubscribe(s.env.recorder, uri)
		}
		s.conn.notifier.forget(uri, updates)
		unregister()
	}
	if err != nil {
		subscription.Close()
		return nil, err
	}
	s.env.T.Cleanup(subscription.Close)
	return subscription, nil
}

// awaitAcknowledgement waits for the server to acknowledge the subscription
// that watch was registered for.
func (s *Session) awaitAcknowledgement(acknowledged <-chan struct{}) error {
	timer := time.NewTimer(subscribeAckTimeout)
	defer timer.Stop()
	select {
	case <-acknowledged:
		return nil
	case <-timer.C:
		return fmt.Errorf("%w within %s: it declines a resource its first read could not reach, and one past its watcher cap",
			errNotAcknowledged, subscribeAckTimeout)
	case <-s.env.Ctx.Done():
		return fmt.Errorf("waiting for the subscription to be acknowledged: %w", s.env.Ctx.Err())
	}
}

// unsubscribe asks the server to stop watching a resource for this session.
//
// A background context, because a subscription's cleanup runs after the
// test's own context is cancelled, and an unsubscribe that never reached the
// server would leave it polling GitLab for the rest of the run.
func (c *sessionConn) unsubscribe(rec *envRecorder, uri string) {
	ctx, cancel := context.WithTimeout(context.Background(), unsubscribeTimeout)
	defer cancel()
	ctx = withAttribution(ctx, callAttribution{
		rec: rec, conn: c, purpose: PurposeCleanup,
		expectation: ExpectationAny, target: uri,
	})
	_ = c.client().Unsubscribe(ctx, &mcp.UnsubscribeParams{URI: uri})
}

// unsubscribeTimeout bounds the unsubscribe a subscription's release sends.
const unsubscribeTimeout = 30 * time.Second

// Next waits for the next update notification for this resource, and fails the
// test when none arrives in time.
func (s *Subscription) Next(timeout time.Duration) {
	s.session.env.T.Helper()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-s.updates:
	case <-timer.C:
		s.session.env.T.Fatalf("no resources/updated notification for %s within %s", s.uri, timeout)
	case <-s.session.env.Ctx.Done():
		s.session.env.T.Fatalf("waiting for an update to %s: %v", s.uri, s.session.env.Ctx.Err())
	}
}

// TryNext waits for the next update and reports whether one arrived, for a
// test whose subject is that no notification should come.
func (s *Subscription) TryNext(timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-s.updates:
		return true
	case <-timer.C:
		return false
	}
}

// updateNotifier fans a session's resource-updated notifications out to the
// subscriptions its tests opened.
//
// One session is shared by many tests, and the SDK delivers a notification to
// the client rather than to whoever subscribed, so the fan-out has to happen
// here. Each watcher gets a buffered channel of its own and a delivery that
// would block is dropped: a test waiting for the next change is not made more
// correct by a queue of changes it never read.
//
// A session holds a second one for the subscription acknowledgements, which
// are the same shape of signal: something happened to one URI, and whoever is
// waiting on that URI is to be woken without the SDK's goroutine ever waiting
// for them.
type updateNotifier struct {
	mu       sync.Mutex
	watchers map[string][]chan struct{}
}

// updateBuffer is how many notifications one watcher may fall behind by.
const updateBuffer = 8

// newUpdateNotifier returns a notifier watching nothing.
func newUpdateNotifier() *updateNotifier {
	return &updateNotifier{watchers: map[string][]chan struct{}{}}
}

// watch registers a channel for one URI.
func (n *updateNotifier) watch(uri string) chan struct{} {
	n.mu.Lock()
	defer n.mu.Unlock()

	updates := make(chan struct{}, updateBuffer)
	n.watchers[uri] = append(n.watchers[uri], updates)
	return updates
}

// forget drops one watcher.
func (n *updateNotifier) forget(uri string, updates chan struct{}) {
	n.mu.Lock()
	defer n.mu.Unlock()

	remaining := n.watchers[uri][:0]
	for _, watcher := range n.watchers[uri] {
		if watcher != updates {
			remaining = append(remaining, watcher)
		}
	}
	if len(remaining) == 0 {
		delete(n.watchers, uri)
		return
	}
	n.watchers[uri] = remaining
}

// deliver hands one notification to every watcher of a URI.
func (n *updateNotifier) deliver(uri string) {
	n.mu.Lock()
	watchers := make([]chan struct{}, len(n.watchers[uri]))
	copy(watchers, n.watchers[uri])
	n.mu.Unlock()

	for _, watcher := range watchers {
		select {
		case watcher <- struct{}{}:
		default:
		}
	}
}

// watching reports how many watchers one URI has, which is what a test of the
// fan-out asserts on.
func (n *updateNotifier) watching(uri string) int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.watchers[uri])
}

// ProgressNote is one progress notification a call provoked.
type ProgressNote struct {
	// Progress is how far the work had got when the server sent this.
	Progress float64
	// Total is what Progress counts towards, zero when the server sent none.
	Total float64
	// Message is the server's own description of the step, if it sent one.
	Message string
}

// progressCollector keeps the progress notifications of the calls that asked
// for them, keyed by the token the call minted.
//
// It collects rather than fans out, unlike [updateNotifier]: a subscription's
// update arrives long after the call that subscribed returned, so a test waits
// for it, while progress arrives while its own call is still in flight and is
// read once that call comes back.
type progressCollector struct {
	mu    sync.Mutex
	notes map[string][]ProgressNote
}

// newProgressCollector returns a collector holding nothing.
func newProgressCollector() *progressCollector {
	return &progressCollector{notes: map[string][]ProgressNote{}}
}

// expect starts collecting for one token.
func (c *progressCollector) expect(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.notes[token] = nil
}

// deliver files one notification under the token it names.
//
// A notification for a token nothing is collecting is dropped: the server is
// free to send progress for a call this harness did not ask about, and a
// collector that grew for those would leak for the life of the session.
func (c *progressCollector) deliver(params *mcp.ProgressNotificationParams) {
	if params == nil {
		return
	}
	token, isString := params.ProgressToken.(string)
	if !isString {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, expected := c.notes[token]; !expected {
		return
	}
	c.notes[token] = append(c.notes[token], ProgressNote{
		Progress: params.Progress,
		Total:    params.Total,
		Message:  params.Message,
	})
}

// collect returns what arrived for one token and stops collecting for it.
func (c *progressCollector) collect(token string) []ProgressNote {
	c.mu.Lock()
	defer c.mu.Unlock()
	notes := c.notes[token]
	delete(c.notes, token)
	return notes
}

// WithProgress runs one action asking the server to report progress, and
// returns the answer beside the notifications that arrived for it.
//
// It is the only way a test sees the progress capability end to end. The
// handlers' own trackers are unit tested in process, which proves the tracker
// fires and nothing about whether a notification reaches a client: the token
// has to survive _meta, the dispatcher, the handler and the transport, and on
// HTTP it has to reach the session the call came in on.
//
// The call is otherwise an ordinary [Do]: it fails the test when the action
// does, and the notifications are whatever the server chose to send, which for
// an action that reports none is an empty slice rather than a failure. What a
// scenario asserts about them is its own business.
func WithProgress[O any](s *Session, id ActionID, params map[string]any, opts ...CallOption) (O, []ProgressNote) {
	s.env.T.Helper()

	token := newProgressToken()
	s.conn.progress.expect(token)
	defer func() { s.conn.progress.collect(token) }()

	output := Do[O](s, id, params, append(opts, withProgressToken(token))...)
	return output, s.conn.progress.collect(token)
}

// withProgressToken is the internal option WithProgress sets. It is not
// exported because a token nobody collects would be asked for and dropped.
func withProgressToken(token string) CallOption {
	return func(o *callOptions) { o.progressToken = token }
}

// newProgressToken mints a token unique within this run.
//
// crypto/rand for the same reason the trace ids use it: the tokens of two
// sessions must not collide, and a counter would have to be shared across
// them to promise that.
func newProgressToken() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		// rand.Read never returns an error on any supported platform, and a
		// token that repeats would only mix two calls' notifications.
		return "progress-fallback"
	}
	return "progress-" + hex.EncodeToString(raw[:])
}
