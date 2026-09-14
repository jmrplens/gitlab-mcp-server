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

	// The reference and the argument together are what a completion covers, so
	// the record names both: a template whose project variable completes says
	// nothing about its branch variable.
	ctx := s.attribute(s.env.Ctx, PurposeTest, ExpectationOK, callAttribution{target: completionTarget(ref) + " " + argument})
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
}

// URI returns the resource this subscription watches.
func (s *Subscription) URI() string { return s.uri }

// Subscribe asks the server to notify this session when a resource changes,
// and unsubscribes when the test ends.
//
// The first read the server makes is its authorization check, so a subscribe
// that is accepted means the credential could read the resource; one it
// refuses fails the test here rather than at the first update that never
// arrives.
func (s *Session) Subscribe(uri string) *Subscription {
	s.env.T.Helper()

	updates := s.conn.notifier.watch(uri)
	// The index is what lets an update notification be recorded against this
	// test: it arrives on the SDK's own goroutine, where nothing says whose
	// subscription it answers.
	unregister := s.conn.subscribers.add(uri, s.env.recorder)

	subscribeCtx := s.attribute(s.env.Ctx, PurposeTest, ExpectationOK, callAttribution{target: uri})
	if err := s.conn.client().Subscribe(subscribeCtx, &mcp.SubscribeParams{URI: uri}); err != nil {
		s.conn.notifier.forget(uri, updates)
		unregister()
		s.env.T.Fatalf("resources/subscribe %s: %v%s", uri, err, s.conn.failureContext())
		return nil
	}
	// Recorded here rather than by the sending middleware: the SDK's Subscribe
	// opens the subscription on a background context under protocol 2026-07-28,
	// so the attribution on subscribeCtx never reaches the middleware and the
	// subscribe would be credited to no test. See [sessionConn.recordSubscribe].
	s.conn.recordSubscribe(s.env.recorder, uri)
	s.env.T.Cleanup(func() {
		// A background context, because the test's own is cancelled by the
		// time cleanups run and an unsubscribe that never reached the server
		// would leave it polling GitLab for the rest of the run.
		ctx, cancel := context.WithTimeout(context.Background(), unsubscribeTimeout)
		defer cancel()
		ctx = withAttribution(ctx, callAttribution{
			rec: s.env.recorder, conn: s.conn, purpose: PurposeCleanup,
			expectation: ExpectationAny, target: uri,
		})
		_ = s.conn.client().Unsubscribe(ctx, &mcp.UnsubscribeParams{URI: uri})
		s.conn.notifier.forget(uri, updates)
		unregister()
	})
	return &Subscription{session: s, uri: uri, updates: updates}
}

// unsubscribeTimeout bounds the unsubscribe a subscription's cleanup sends.
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
