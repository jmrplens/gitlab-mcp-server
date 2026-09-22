//go:build e2e

// record.go writes down what the suite actually exercised.
//
// The rule the whole rebuild rests on is that coverage is what the server
// dispatched, in a test that passed, on a named runtime, surface and mode. The
// command this replaces credited an action because its identifier appeared on
// a line of a test file, which is how eighteen actions nothing ever ran were
// counted as covered.
//
// Two halves make one line. The client half is a sending middleware on every
// harness session: it stamps a fresh traceparent into _meta, records what was
// asked for and what came back, and attributes it to the test that asked
// through the context. The server half is the span that call produced, read by
// the receiver in otlp.go and joined on the trace id.
//
// Records are buffered per test and written from the first-registered cleanup,
// which therefore runs last: by then the ledger has undone what the test
// created, so the cleanup's own calls are in the buffer too, and the test's
// final status is settled.
//
// Writing is off unless GITLAB_MCP_TEST_E2E_CALLS_DIR names a directory. The
// dispatch assertion is not: a call that ran something other than what it
// asked for fails its test in every run, recorded or not, because that is a
// defect in this server or in this harness rather than a gap in a report.

package harness

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"maps"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls"
)

// The MCP methods the recorder names. They are spelled here because the SDK
// keeps its own constants unexported, and a record's method field is part of
// the contract with cmd/audit_e2e_coverage rather than an internal detail.
const (
	methodCallTool        = "tools/call"
	methodReadResource    = "resources/read"
	methodGetPrompt       = "prompts/get"
	methodComplete        = "completion/complete"
	methodElicit          = "elicitation/create"
	methodResourceUpdated = "notifications/resources/updated"
	// methodSubscribe is the logical resources/subscribe the [Session.Subscribe]
	// verb represents. It is recorded by the verb rather than by the sending
	// middleware: over protocol 2026-07-28 the SDK opens the subscription on a
	// background context of its own, so the middleware never sees the caller's
	// attribution and would record nothing. The audit routes it to the
	// subscription capability whether the wire carried resources/subscribe or
	// its 2026-07-28 replacement, subscriptions/listen.
	methodSubscribe = "resources/subscribe"
	// methodSubscriptionsAcknowledged is what the server sends first on a
	// subscriptions/listen stream, once every subscription the stream asked
	// for succeeded. It is not recorded as a call of its own: it is the
	// answer [Session.TrySubscribe] waits for.
	methodSubscriptionsAcknowledged = "notifications/subscriptions/acknowledged"
)

// traceParentKey is the _meta key W3C trace context travels in.
//
// MCP reserves it unprefixed by name, and this server already reads it there,
// so one key is the whole of the client side of the join. Nothing else is put
// in _meta: a key the server does not read would be noise on every call.
const traceParentKey = "traceparent"

// sampledTraceFlags marks a traceparent as sampled.
//
// Every call this harness makes is sampled, because a span that was not
// recorded is a call whose dispatch nothing can report, and the whole point of
// stamping the trace is to read that dispatch back.
const sampledTraceFlags = "01"

// callAttribution is what only the caller of a request knows: which test made
// it, on which session, what for, and what it expected.
//
// It travels on the context because the middleware that records the call sits
// on the session's client, which many tests share, while the answer to "whose
// call is this" belongs to the one Env that made it. The alternative the old
// suite used was to walk the stack for a test name, which cannot see a purpose
// or an expectation at all.
type callAttribution struct {
	// rec is the buffer the call line goes into.
	rec *envRecorder
	// conn is the session the call was made on.
	conn *sessionConn
	// purpose says what the call was made for.
	purpose Purpose
	// expectation is what the caller asked to happen, in the record's
	// vocabulary: ok, any, or a failure class named by a verb.
	expectation string
	// action is the canonical catalog ID a tool call named, empty for the
	// methods that name none.
	action ActionID
	// target names what a non-tool call addressed: a resource URI, a prompt,
	// or a completion reference.
	target string
	// wantDispatch is the route the caller declared this call would run, when
	// it is not the action named. Empty means the action itself.
	wantDispatch ActionID
	// traceOut, when it is not nil, receives the trace id this call was
	// stamped with, so the caller can read the server's own span back while
	// the call is still its business.
	//
	// Every other caller reads a dispatch at the flush, where the whole test's
	// calls are resolved at once and the trace id never has to leave the
	// middleware. [Session.CallAsModel] cannot wait that long: what the server
	// dispatched for one call is what the next turn of a conversation is
	// scored on, and by the flush the conversation is over.
	traceOut *string
}

// attributionKey is the private context key the attribution travels under.
type attributionKey struct{}

// withAttribution returns a context carrying who is making the next request.
func withAttribution(ctx context.Context, attr callAttribution) context.Context {
	return context.WithValue(ctx, attributionKey{}, attr)
}

// attributionFrom reads the attribution off a context, if the caller set one.
//
// A request with none is one the SDK made for itself, such as the initialize
// handshake or the listings a session start reads. Those are not coverage and
// are deliberately neither stamped nor recorded: stamping them would issue
// trace ids nothing ever joins.
func attributionFrom(ctx context.Context) (callAttribution, bool) {
	attr, carried := ctx.Value(attributionKey{}).(callAttribution)
	return attr, carried
}

// attribute returns a context that records the next request against this
// session and this test, built on the context the call runs under.
func (s *Session) attribute(base context.Context, purpose Purpose, expectation string, attr callAttribution) context.Context {
	attr.rec = s.env.recorder
	attr.conn = s.conn
	attr.purpose = purpose
	attr.expectation = expectation
	return withAttribution(base, attr)
}

// newTraceParent mints a trace and returns its id with the header value that
// carries it.
//
// crypto/rand because the ids must not collide across the processes of one
// run, and because the W3C format forbids an all-zero id, which a seeded
// counter would have to special-case.
func newTraceParent() (traceID, traceParent string) {
	var ids [24]byte
	// rand.Read fills the slice or panics; it cannot fail on any platform this
	// suite runs on, and Go 1.24 removed its error return for that reason.
	rand.Read(ids[:])
	traceID = hex.EncodeToString(ids[:16])
	spanID := hex.EncodeToString(ids[16:])
	return traceID, "00-" + traceID + "-" + spanID + "-" + sampledTraceFlags
}

// stampTraceParent writes the trace context into a request's _meta, reporting
// whether it could.
//
// The params are copied rather than mutated in place, because Raw hands the
// caller's own struct over and a test that reuses one should not find a stale
// traceparent on its second call.
func stampTraceParent(req mcp.Request, traceParent string) bool {
	params := req.GetParams()
	if params == nil {
		return false
	}
	// A typed nil pointer in a non-nil interface is what the SDK hands a
	// middleware for a method whose params may be missing. Comparing the
	// interface against nil says false, and the first method call on it
	// dereferences the pointer.
	if value := reflect.ValueOf(params); value.Kind() == reflect.Pointer && value.IsNil() {
		return false
	}

	meta := maps.Clone(params.GetMeta())
	if meta == nil {
		meta = map[string]any{}
	}
	meta[traceParentKey] = traceParent
	params.SetMeta(meta)
	return true
}

// pendingCall is one recorded call plus what its test's flush still has to
// resolve about it.
type pendingCall struct {
	// line is the record as the client knows it, before the span arrives.
	line *e2ecalls.Call
	// conn is the session the call went to, which the flush marks
	// dispatch-observed when the span lands.
	conn *sessionConn
	// wantDispatch is the route the caller declared, empty when the requested
	// action is what should have run.
	wantDispatch ActionID
}

// envRecorder buffers one test's record and writes it when that test ends.
type envRecorder struct {
	env *Env

	mu      sync.Mutex
	calls   []*pendingCall
	skips   []*e2ecalls.Skip
	flushed bool
}

// newEnvRecorder returns the buffer one Env records into.
func newEnvRecorder(env *Env) *envRecorder {
	return &envRecorder{env: env}
}

// record buffers one call, dropping it when the test has already been written.
//
// A late arrival is possible and is not an error: a resource-updated
// notification can reach the client after the test that subscribed has ended.
// Dropping it is right, because the line it would produce carries a status
// that has already been reported for every other call of that test.
func (r *envRecorder) record(call *pendingCall) {
	if r == nil || call == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.flushed {
		return
	}
	r.calls = append(r.calls, call)
}

// noteSkip records why a test did not run.
//
// The testing package keeps a skip's reason to itself, so the harness has to
// be told: every skip it decides on reports here before it calls Skip, and a
// test that skipped itself is written with no reason rather than with a
// guessed one.
func (r *envRecorder) noteSkip(reason string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.flushed {
		return
	}
	r.skips = append(r.skips, &e2ecalls.Skip{Test: r.env.T.Name(), Reason: reason})
}

// flush is the cleanup the Env registers first, and which therefore runs last.
func (r *envRecorder) flush() {
	r.env.T.Helper()
	r.finish(r.env.T, testStatus(r.env.T))
}

// finish resolves every buffered call against the spans the server sent,
// asserts what ran, writes the lines and returns them.
//
// The reporter and the status are parameters rather than read from the test,
// so that the whole of this can be driven from a test of its own: a cleanup
// that fails its own test cannot be asserted on, because a failing subtest
// fails its parent.
func (r *envRecorder) finish(reporter e2ecalls.Reporter, status string) []e2ecalls.Line {
	r.mu.Lock()
	if r.flushed {
		r.mu.Unlock()
		return nil
	}
	r.flushed = true
	calls := r.calls
	skips := r.skips
	r.mu.Unlock()

	awaitDispatch(calls)

	lines := make([]e2ecalls.Line, 0, len(calls)*2+len(skips))
	for _, call := range calls {
		call.line.TestStatus = status
		if kept, arrived := dispatchFor(call); arrived {
			call.line.Dispatched = kept.dispatch.action
			lines = append(lines, dispatchLine(call.line.TraceID, kept))
			assertDispatch(reporter, call, kept.dispatch)
		}
		lines = append(lines, call.line)
	}
	for _, skip := range skips {
		lines = append(lines, skip)
	}

	e2ecalls.Open().Write(reporter, lines...)
	return lines
}

// testStatus names how a test ended, in the record's vocabulary.
//
// Failure wins over a skip, because a test that failed and then skipped told
// us something about the server and a skip line would hide it.
func testStatus(t *testing.T) string {
	t.Helper()
	switch {
	case t.Failed():
		return e2ecalls.StatusFailed
	case t.Skipped():
		return e2ecalls.StatusSkipped
	default:
		return e2ecalls.StatusPassed
	}
}

// dispatchWait is how long a test's flush waits for the spans of the calls it
// made. The children export every hundred milliseconds, so this is generous
// for the loopback hop and short enough that a stalled exporter is noticed.
const dispatchWait = 10 * time.Second

// dispatchGrace is what a run whose spans have never arrived waits instead.
//
// Without it, a run with telemetry broken would pay the full wait on every
// test and take hours to say so. One test pays it, says so once, and the rest
// wait long enough for a late first span to still be picked up.
const dispatchGrace = 250 * time.Millisecond

// dispatchPoll is how often the wait re-reads the receiver.
const dispatchPoll = 25 * time.Millisecond

// dispatchGaveUp is set once a flush has waited the full budget and seen no
// span at all, which is what turns the budget down for every flush after it.
var dispatchGaveUp atomic.Bool

// awaitDispatch waits until the server has reported on every call of this
// test, or until the budget runs out.
func awaitDispatch(calls []*pendingCall) {
	pending := make([]string, 0, len(calls))
	for _, call := range calls {
		if call.line.TraceID != "" {
			pending = append(pending, call.line.TraceID)
		}
	}
	awaitTraces(pending)
}

// awaitTraces waits for the server's own span of each of these traces.
//
// It is the flush's wait, factored out because [Session.CallAsModel] makes the
// same wait for one call rather than for a test's worth of them. One
// implementation and not two, because what the second would drift on is the
// budget and the give-up flag: a per-call waiter that kept its own copy would
// pay the full ten seconds on every call of a run whose telemetry is broken,
// which is the outcome dispatchGrace exists to prevent.
func awaitTraces(pending []string) {
	received := receiverIfStarted()
	if received == nil || len(pending) == 0 {
		return
	}

	budget := dispatchWait
	if !received.observed.Load() && dispatchGaveUp.Load() {
		budget = dispatchGrace
	}
	deadline := time.Now().Add(budget)
	for {
		missing := 0
		for _, traceID := range pending {
			if _, arrived := received.lookup(traceID); !arrived {
				missing++
			}
		}
		if missing == 0 || time.Now().After(deadline) {
			if missing > 0 && !received.observed.Load() && dispatchGaveUp.CompareAndSwap(false, true) {
				log.Printf("e2e: no server span has arrived; every call is recorded as what was asked for "+
					"rather than as what ran, and later tests will wait %s instead of %s", dispatchGrace, dispatchWait)
			}
			return
		}
		time.Sleep(dispatchPoll)
	}
}

// dispatchFor returns what the receiver kept about one call, marking its
// session dispatch-observed when the server's span arrived.
func dispatchFor(call *pendingCall) (traceSpans, bool) {
	received := receiverIfStarted()
	if received == nil || call.line.TraceID == "" {
		return traceSpans{}, false
	}
	kept, arrived := received.lookup(call.line.TraceID)
	if arrived && call.conn != nil {
		call.conn.dispatchObserved.Store(true)
	}
	return kept, arrived
}

// assertDispatch fails the test when the server ran something other than what
// the call asked for.
//
// This is the assertion the whole recorder exists to make. A call that named
// issue.list and ran issue.get proves nothing about issue.list, and the suite
// would otherwise report it as covered. A test that knows its call is
// rewritten declares the route with ExpectDispatch and is held to that instead.
//
// A sweep is the one exemption. It probes what a session serves rather than
// asserting one route, walks it with a non-constant id, and is credited to the
// action the server said it ran ([creditOf] reads the dispatched action). A
// route the server rewrites under a sweep is therefore credited correctly and
// asserts nothing false, so failing the sweep on the rewrite would only make a
// broad probe brittle against a rewrite it never claimed anything about.
func assertDispatch(reporter e2ecalls.Reporter, call *pendingCall, record dispatchRecord) {
	if call.line.Action == "" || record.action == "" || call.line.Purpose == e2ecalls.PurposeSweep {
		return
	}
	want := string(call.wantDispatch)
	if want == "" {
		want = call.line.Action
	}
	if record.action == want {
		return
	}
	if call.wantDispatch != "" {
		reporter.Errorf("%s on the %s surface dispatched %s, and the test declared it would dispatch %s",
			call.line.Action, call.line.Surface, record.action, want)
		return
	}
	reporter.Errorf("%s on the %s surface dispatched %s: the call did not run the action it named, so nothing it "+
		"asserts is about %s. Declare the route with ExpectDispatch if that is what the server should do.",
		call.line.Action, call.line.Surface, record.action, call.line.Action)
}

// dispatchLine turns what a trace's spans said into the record line the audit
// joins on the trace id.
//
// The request count rides here rather than on a line of its own because it
// answers a question about the action this line already names, and a second
// line type would be a schema bump for one integer. It is written at the same
// moment the dispatch is, which is normally after every client span of the
// trace has arrived: the children end before the parent does, so they are
// queued and exported first, and the wait for the parent is a wait for them
// too. Normally, not always — a batch queue that overflowed drops silently —
// which is why [e2ecalls.Dispatch.Requests] is documented as a floor.
func dispatchLine(traceID string, kept traceSpans) *e2ecalls.Dispatch {
	return &e2ecalls.Dispatch{
		TraceID:       traceID,
		Tool:          kept.dispatch.tool,
		Action:        kept.dispatch.action,
		Domain:        kept.dispatch.domain,
		RefusalReason: kept.dispatch.refusalReason,
		ErrorType:     kept.dispatch.errorType,
		Status:        kept.dispatch.status,
		Requests:      kept.requests,
	}
}

// recordSending is the client middleware every request of one session leaves
// through.
//
// It runs on the caller's goroutine, between the verb and the wire, which is
// the only place that holds all three of the attribution, the request as it
// will be sent, and the answer as it comes back.
func (c *sessionConn) recordSending() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			attr, attributed := attributionFrom(ctx)
			if !attributed || attr.rec == nil {
				return next(ctx, method, req)
			}

			traceID, traceParent := newTraceParent()
			if !stampTraceParent(req, traceParent) {
				traceID = ""
			} else if received := receiverIfStarted(); received != nil {
				received.issue(traceID)
			}
			// After the stamp and not before it: a request whose params could
			// not carry the trace has no trace, and a caller told otherwise
			// would wait the whole budget for a span that was never issued.
			if attr.traceOut != nil {
				*attr.traceOut = traceID
			}

			release := c.holdInFlight(attr)
			started := time.Now()
			result, err := next(ctx, method, req)
			elapsed := time.Since(started)
			release()

			attr.rec.record(c.callRecord(attr, method, req, result, err, elapsed, traceID))
			return result, err
		}
	}
}

// callRecord builds the client half of one call line.
func (c *sessionConn) callRecord(
	attr callAttribution,
	method string,
	req mcp.Request,
	result mcp.Result,
	err error,
	elapsed time.Duration,
	traceID string,
) *pendingCall {
	tool, arguments := describeToolRequest(req)
	line := &e2ecalls.Call{
		Test:        attr.rec.env.T.Name(),
		Purpose:     string(attr.purpose),
		Expectation: attr.expectation,
		Method:      method,
		Tool:        tool,
		Action:      string(attr.action),
		Target:      attr.target,
		Arguments:   arguments,
		Outcome:     outcomeOf(method, result, err),
		DurationMS:  float64(elapsed.Microseconds()) / 1000,
		TraceID:     traceID,
	}
	c.describeSession(line)
	return &pendingCall{line: line, conn: c, wantDispatch: attr.wantDispatch}
}

// describeSession fills in the half of a call line that is the session's
// rather than the call's.
//
// The session's shape is repeated on every call line on purpose: the audit can
// then classify one line without joining it back to the session that served
// it, and a shard whose session line was lost still says what a call covered.
func (c *sessionConn) describeSession(line *e2ecalls.Call) {
	line.Session = c.label
	line.Surface = string(c.cfg.Surface)
	line.Mode = string(c.cfg.Mode)
	line.Capabilities = string(c.cfg.Capabilities)
	if c.inst != nil {
		line.Requirement = c.inst.requirement.token()
	}
}

// describeToolRequest returns the tool a request named and its top-level
// argument names, for the methods that name one.
//
// The values are left out deliberately: a value is a fixture and belongs to
// one run, while a name is part of the call and is what a coverage report
// compares against a schema.
func describeToolRequest(req mcp.Request) (tool string, arguments []string) {
	params, isToolCall := req.GetParams().(*mcp.CallToolParams)
	if !isToolCall || params == nil {
		return "", nil
	}
	named, isObject := params.Arguments.(map[string]any)
	if !isObject {
		return params.Name, nil
	}
	return params.Name, argumentNames(named)
}

// outcomeOf classifies one answer in the record's vocabulary.
//
// A tool call goes through the same classify the assertions use, so a record
// and a failure message can never disagree about whether the server refused
// something. Every other method has two outcomes: it answered, or it did not.
func outcomeOf(method string, result mcp.Result, err error) string {
	if method == methodCallTool {
		toolResult, _ := result.(*mcp.CallToolResult)
		return classify(toolResult, err).outcome
	}
	if err == nil {
		return e2ecalls.OutcomeOK
	}
	if isTransportError(err) {
		return e2ecalls.OutcomeTransportError
	}
	return e2ecalls.OutcomeProtocolError
}

// recordReceiving is the client middleware every request the server sends back
// arrives through.
//
// A resource-updated notification is worth recording: it is the other end of a
// subscription, and without it a subscription's coverage would be the
// subscribe call and nothing about whether anything was ever delivered.
//
// A subscription acknowledgement passes through here too, and only here: the
// SDK dispatches it into this chain and then to a handler of its own that does
// nothing and that no client option replaces, so this is the one place the
// answer to a subscribe on protocol 2026-07-28 can be seen. It is handed to
// whoever is waiting on the URI rather than recorded, because the subscribe it
// answers is recorded by the verb with what came of it.
func (c *sessionConn) recordReceiving() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			// An elicitation is deliberately not handled here. The SDK
			// delivers one to ClientOptions.ElicitationHandler rather than
			// through this chain, so a case for it would never fire; it is
			// recorded by [sessionConn.recordingElicitationHandler] instead.
			switch method {
			case methodResourceUpdated:
				c.recordResourceUpdate(req)
			case methodSubscriptionsAcknowledged:
				c.acknowledge(req)
			}
			return next(ctx, method, req)
		}
	}
}

// acknowledge wakes whoever is waiting on each URI a subscription
// acknowledgement names.
//
// The URIs are the ones the server agreed to watch, which is not always every
// one the stream asked for, so a URI missing from the list is one nobody is
// woken for.
func (c *sessionConn) acknowledge(req mcp.Request) {
	params, isAck := req.GetParams().(*mcp.SubscriptionsAcknowledgedParams)
	if !isAck || params == nil {
		return
	}
	for _, uri := range params.Notifications.ResourceSubscriptions {
		c.acks.deliver(uri)
	}
}

// recordSubscribe records one subscribe against the test that made it, with
// what came of it.
//
// The sending middleware cannot: over protocol 2026-07-28 the SDK's Subscribe
// opens the listen stream on a background context, dropping the attribution the
// caller put on its own, so the one call the middleware would see carries no
// test to file it under. The verb records it here instead, once it knows the
// answer, so a subscribe the server declined is recorded as the refusal it was
// rather than credited; the resource-updated notification that may follow is
// recorded by [sessionConn.recordResourceUpdate], which is the delivery half.
func (c *sessionConn) recordSubscribe(rec *envRecorder, uri, expectation, outcome string) {
	line := &e2ecalls.Call{
		Test:        rec.env.T.Name(),
		Purpose:     string(PurposeTest),
		Expectation: expectation,
		Method:      methodSubscribe,
		Target:      uri,
		Outcome:     outcome,
	}
	c.describeSession(line)
	rec.record(&pendingCall{line: line, conn: c})
}

// recordElicitation records one elicitation request against the test whose
// call provoked it.
//
// The attribution is the session's in-flight call, because an elicitation
// arrives on the SDK's own receiving goroutine, inside a tools/call that is
// still waiting for an answer. When the session is serving several calls at
// once the harness declines to guess whose it is and records nothing: a line
// attributed to the wrong test is worse than a missing one.
func (c *sessionConn) recordElicitation(req mcp.Request) {
	attr, known := c.currentInFlight()
	if !known {
		return
	}
	params, isElicit := req.GetParams().(*mcp.ElicitParams)
	if !isElicit || params == nil {
		return
	}

	line := &e2ecalls.Call{
		Test:        attr.rec.env.T.Name(),
		Purpose:     string(attr.purpose),
		Expectation: e2ecalls.ExpectationAny,
		Method:      methodElicit,
		Target:      params.Message,
		Arguments:   elicitationKeys(params.RequestedSchema),
		Outcome:     e2ecalls.OutcomeOK,
	}
	c.describeSession(line)
	attr.rec.record(&pendingCall{line: line, conn: c})
}

// elicitationKeys returns the sorted property names an elicitation asked for.
//
// The schema arrives as whatever the server marshaled, which on the client
// side is a map. A schema that is not one is recorded as asking for nothing
// rather than as an error: what is being recorded is that the flow elicited,
// and the field list is detail beside it.
func elicitationKeys(schema any) []string {
	object, isObject := schema.(map[string]any)
	if !isObject {
		return nil
	}
	properties, hasProperties := object["properties"].(map[string]any)
	if !hasProperties {
		return nil
	}
	return slices.Sorted(maps.Keys(properties))
}

// recordResourceUpdate records one notification against the test watching
// that resource.
//
// The index is what makes this exact, since the notification itself says only
// which resource changed and a session is shared by many tests. A notification
// for a URI no test watches any more, which can arrive after the watcher's test
// has ended, is recorded nowhere.
func (c *sessionConn) recordResourceUpdate(req mcp.Request) {
	params, isUpdate := req.GetParams().(*mcp.ResourceUpdatedNotificationParams)
	if !isUpdate || params == nil {
		return
	}
	rec, watched := c.subscribers.recorderFor(params.URI)
	if !watched {
		return
	}
	line := &e2ecalls.Call{
		Test:        rec.env.T.Name(),
		Purpose:     string(PurposeTest),
		Expectation: e2ecalls.ExpectationAny,
		Method:      methodResourceUpdated,
		Target:      params.URI,
		Outcome:     e2ecalls.OutcomeOK,
	}
	c.describeSession(line)
	rec.record(&pendingCall{line: line, conn: c})
}

// holdInFlight registers a call as in flight on this session and returns the
// function that takes it off again.
func (c *sessionConn) holdInFlight(attr callAttribution) func() {
	id := c.inFlightNext.Add(1)

	c.inFlightMu.Lock()
	if c.inFlight == nil {
		c.inFlight = map[int64]callAttribution{}
	}
	c.inFlight[id] = attr
	c.inFlightMu.Unlock()

	return func() {
		c.inFlightMu.Lock()
		delete(c.inFlight, id)
		c.inFlightMu.Unlock()
	}
}

// currentInFlight returns the one call this session is serving, when there is
// exactly one.
func (c *sessionConn) currentInFlight() (callAttribution, bool) {
	c.inFlightMu.Lock()
	defer c.inFlightMu.Unlock()

	if len(c.inFlight) != 1 {
		return callAttribution{}, false
	}
	for _, attr := range c.inFlight {
		return attr, attr.rec != nil
	}
	return callAttribution{}, false
}

// subscriberIndex remembers which test is watching which resource on one
// session, so a notification that arrives on the SDK's goroutine can be
// recorded against the test that asked for it.
//
// It is kept apart from the notifier that fans the channels out because the
// two answer different questions: the notifier wakes whoever is waiting, and
// this says whose record the notification belongs in. A watcher that has
// stopped waiting still has a record.
//
// It holds one test per URI, because the SDK keeps one subscription per URI
// and per session: a second test would share the first one's, and could
// neither learn whether the server agreed nor close it without closing the
// first. [subscriberIndex.claim] is where the second one is turned away.
type subscriberIndex struct {
	mu       sync.Mutex
	watchers map[string]*envRecorder
}

// newSubscriberIndex returns an index watching nothing.
func newSubscriberIndex() *subscriberIndex {
	return &subscriberIndex{watchers: map[string]*envRecorder{}}
}

// claim registers one test as the watcher of a URI and returns the release,
// unless another test already watches it on this session, in which case it
// registers nothing and says so. The check and the registration are one step,
// so two tests racing for a URI cannot both win.
func (i *subscriberIndex) claim(uri string, rec *envRecorder) (func(), bool) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if _, taken := i.watchers[uri]; taken {
		return nil, false
	}
	i.watchers[uri] = rec
	return func() { i.release(uri) }, true
}

// release drops the watcher of a URI.
func (i *subscriberIndex) release(uri string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	delete(i.watchers, uri)
}

// recorderFor returns the buffer a notification about one URI belongs in, and
// whether any test watches it.
func (i *subscriberIndex) recorderFor(uri string) (*envRecorder, bool) {
	i.mu.Lock()
	defer i.mu.Unlock()
	rec, watched := i.watchers[uri]
	return rec, watched
}

// logReporter reports a shard failure at the end of a run, where there is no
// test left to fail.
//
// The per-test writes report through the test that was writing, which is what
// makes a broken shard directory fail one test rather than all of them. The
// run and session lines are written after the last test has ended, so the only
// thing left to tell is the log.
type logReporter struct{}

// Errorf writes one failure to the run's log.
func (logReporter) Errorf(format string, args ...any) {
	log.Printf("e2e: "+format, args...)
}

// flushRunRecords writes the lines that belong to the package rather than to
// any one test: what runtime it ran on, what each session served, and every
// span that arrived after the test that made the call had already been
// written.
//
// It runs after the last test, because that is when a session's
// dispatch-observed flag has settled. Writing a session line per test instead
// would write two versions of the same session, one before its first span
// arrived and one after.
func flushRunRecords() {
	writer := e2ecalls.Open()
	if writer == nil {
		return
	}

	lines := []e2ecalls.Line{runLine(&state)}
	lines = append(lines, sessionLines()...)
	lines = append(lines, lateDispatchLines()...)
	writer.Write(logReporter{}, lines...)
}

// runLine records what this package found and whether it went on to run.
//
// A refused run is written too. A missing run line and a run that refused to
// start are different answers to "was this runtime exercised", and a release
// gate that could not tell them apart would pass on a shard nothing produced.
//
// The state is a parameter rather than the package's own, because a runState
// carries the bootstrap's sync.Once and cannot be copied: a test that wanted
// to ask what a refused run writes would otherwise have to overwrite the run
// this process is in the middle of.
func runLine(s *runState) *e2ecalls.Run {
	run := &e2ecalls.Run{
		Package:     s.pkg,
		Requirement: s.requirement.token(),
		RunID:       s.runID,
		Commit:      s.settings.get(envCommit),
		Filter:      testRunFilter(),
		Status:      e2ecalls.RunStarted,
	}
	if s.kind != refusalNone {
		run.Status = e2ecalls.RunRefused
		run.Reason = s.refusal
	}
	if inst := s.inst; inst != nil {
		run.Edition = inst.facts.editionToken()
		run.Tier = inst.facts.Tier.String()
		run.TierConfirmed = inst.facts.TierConfirmed
		run.GitLabVersion = inst.facts.Version
		run.Fixtures = fixtureProfile(inst)
	}
	return run
}

// fixtureProfile records what the runtime had available.
//
// A scenario that needs a CI runner is absent rather than failing when there
// is none, so a coverage figure that cannot say which fixtures were up is not
// worth reading: the same shard means two different things on a stack with a
// runner and on one without.
func fixtureProfile(inst *instance) e2ecalls.FixtureProfile {
	return e2ecalls.FixtureProfile{
		Runner:         inst.hasRunner(),
		FixtureService: inst.settings.get(envFixtureURL) != "",
		Bitbucket:      inst.settings.get(envBitbucketServerURL) != "",
		GHToken:        inst.settings.get(envGitHubToken) != "",
		Seeds:          seedNames(inst.settings.get(envSeeds)),
	}
}

// seedNames splits the seed list the provisioning script publishes.
func seedNames(configured string) []string {
	var seeds []string
	for seed := range strings.SplitSeq(configured, ",") {
		if trimmed := strings.TrimSpace(seed); trimmed != "" {
			seeds = append(seeds, trimmed)
		}
	}
	slices.Sort(seeds)
	return slices.Compact(seeds)
}

// sessionLines records what every session this package started served.
//
// The served sets are the denominator of the whole report: an action no
// session served is absent rather than untested, and those are different
// findings about different things.
func sessionLines() []e2ecalls.Line {
	var lines []e2ecalls.Line
	sessions.Range(func(_, value any) bool {
		entry, isEntry := value.(*sessionEntry)
		if !isEntry || entry.conn == nil {
			return true
		}
		conn := entry.conn
		lines = append(lines, &e2ecalls.Session{
			Label:             conn.label,
			Surface:           string(conn.cfg.Surface),
			Mode:              string(conn.cfg.Mode),
			Capabilities:      string(conn.cfg.Capabilities),
			Transport:         string(conn.cfg.Transport),
			Tools:             slices.Clone(conn.served.tools),
			Resources:         slices.Clone(conn.served.resources),
			ResourceTemplates: slices.Clone(conn.served.templates),
			Prompts:           slices.Clone(conn.served.prompts),
			Completions:       completionReferences(conn.served),
			DispatchObserved:  conn.dispatchObserved.Load(),
		})
		return true
	})
	return lines
}

// lateDispatchLines writes every trace the receiver holds that the server
// spoke about.
//
// A call whose span arrived after its test's flush was written with no
// dispatched action, and this is what lets the audit join one to it anyway.
// Re-offering a line that was already written costs nothing: the writer drops
// a line whose JSON it has already seen, and a line whose request count has
// since grown is written a second time, which is what makes a late client span
// correct itself.
//
// Both readers fold those two lines back into one trace — the coverage
// command's join keeps the last line per trace id, and R-PATH's observation
// keeps the one that saw the most requests — so the correction is what a reader
// sees. A diagnostic that counts lines rather than traces counts both, which is
// why the coverage command's own is called DispatchLines.
func lateDispatchLines() []e2ecalls.Line {
	received := receiverIfStarted()
	if received == nil {
		return nil
	}
	return dispatchLinesOf(received.all())
}

// dispatchLinesOf turns what a receiver holds into the dispatch lines a record
// carries, one per trace that named an action.
//
// A trace holding only GitLab request spans is skipped. It names no action, so
// there is nothing for the audit to attribute those requests to, and a dispatch
// line with an empty action would be a record of a call nobody can identify —
// counted as a dispatch by the coverage command's diagnostics and then skipped
// by its join, which is a disagreement with no signal behind it.
//
// It is a function of the map rather than a loop inside its caller so that the
// skip can be tested: its caller reads a package-level receiver that only a
// started suite has.
func dispatchLinesOf(all map[string]traceSpans) []e2ecalls.Line {
	lines := make([]e2ecalls.Line, 0, len(all))
	for traceID, kept := range all {
		if !kept.dispatch.carriesFacts() {
			continue
		}
		lines = append(lines, dispatchLine(traceID, kept))
	}
	return lines
}
