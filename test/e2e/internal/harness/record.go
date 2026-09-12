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
// session and this test.
func (s *Session) attribute(purpose Purpose, expectation string, attr callAttribution) context.Context {
	attr.rec = s.env.recorder
	attr.conn = s.conn
	attr.purpose = purpose
	attr.expectation = expectation
	return withAttribution(s.env.Ctx, attr)
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
		if record, arrived := dispatchFor(call); arrived {
			call.line.Dispatched = record.action
			lines = append(lines, dispatchLine(call.line.TraceID, record))
			assertDispatch(reporter, call, record)
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
	received := receiverIfStarted()
	if received == nil {
		return
	}
	pending := make([]string, 0, len(calls))
	for _, call := range calls {
		if call.line.TraceID != "" {
			pending = append(pending, call.line.TraceID)
		}
	}
	if len(pending) == 0 {
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

// dispatchFor returns what the server said about one call, marking its session
// dispatch-observed when a span arrived.
func dispatchFor(call *pendingCall) (dispatchRecord, bool) {
	received := receiverIfStarted()
	if received == nil || call.line.TraceID == "" {
		return dispatchRecord{}, false
	}
	record, arrived := received.lookup(call.line.TraceID)
	if arrived && call.conn != nil {
		call.conn.dispatchObserved.Store(true)
	}
	return record, arrived
}

// assertDispatch fails the test when the server ran something other than what
// the call asked for.
//
// This is the assertion the whole recorder exists to make. A call that named
// issue.list and ran issue.get proves nothing about issue.list, and the suite
// would otherwise report it as covered. A test that knows its call is
// rewritten declares the route with ExpectDispatch and is held to that instead.
func assertDispatch(reporter e2ecalls.Reporter, call *pendingCall, record dispatchRecord) {
	if call.line.Action == "" || record.action == "" {
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

// dispatchLine turns what a span said into the record line the audit joins on
// the trace id.
func dispatchLine(traceID string, record dispatchRecord) *e2ecalls.Dispatch {
	return &e2ecalls.Dispatch{
		TraceID:       traceID,
		Tool:          record.tool,
		Action:        record.action,
		Domain:        record.domain,
		RefusalReason: record.refusalReason,
		ErrorType:     record.errorType,
		Status:        record.status,
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
// Two of them are worth recording. An elicitation is the server asking the
// caller something mid-call, and it is the only evidence an interactive flow
// ran at all. A resource-updated notification is the other end of a
// subscription, and without it a subscription's coverage would be the
// subscribe call and nothing about whether anything was ever delivered.
func (c *sessionConn) recordReceiving() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			switch method {
			case methodElicit:
				c.recordElicitation(req)
			case methodResourceUpdated:
				c.recordResourceUpdate(req)
			}
			return next(ctx, method, req)
		}
	}
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

// recordResourceUpdate records one notification against every test watching
// that resource.
//
// Every test, rather than one: a session is shared, and two tests subscribed
// to the same URI both observe the change. The index is what makes this exact,
// since the notification itself says only which resource changed.
func (c *sessionConn) recordResourceUpdate(req mcp.Request) {
	params, isUpdate := req.GetParams().(*mcp.ResourceUpdatedNotificationParams)
	if !isUpdate || params == nil {
		return
	}
	for _, rec := range c.subscribers.recordersFor(params.URI) {
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
// recorded against the tests that asked for it.
//
// It is kept apart from the notifier that fans the channels out because the
// two answer different questions: the notifier wakes whoever is waiting, and
// this says whose record the notification belongs in. A watcher that has
// stopped waiting still has a record.
type subscriberIndex struct {
	mu       sync.Mutex
	watchers map[string][]*envRecorder
}

// newSubscriberIndex returns an index watching nothing.
func newSubscriberIndex() *subscriberIndex {
	return &subscriberIndex{watchers: map[string][]*envRecorder{}}
}

// add registers one test as a watcher of a URI and returns the removal.
func (i *subscriberIndex) add(uri string, rec *envRecorder) func() {
	if rec == nil {
		return func() {}
	}
	i.mu.Lock()
	i.watchers[uri] = append(i.watchers[uri], rec)
	i.mu.Unlock()

	return func() { i.remove(uri, rec) }
}

// remove drops one watcher, and the URI with it when it was the last.
func (i *subscriberIndex) remove(uri string, rec *envRecorder) {
	i.mu.Lock()
	defer i.mu.Unlock()

	remaining := make([]*envRecorder, 0, len(i.watchers[uri]))
	dropped := false
	for _, watcher := range i.watchers[uri] {
		if watcher == rec && !dropped {
			dropped = true
			continue
		}
		remaining = append(remaining, watcher)
	}
	if len(remaining) == 0 {
		delete(i.watchers, uri)
		return
	}
	i.watchers[uri] = remaining
}

// recordersFor returns the buffers a notification about one URI belongs in.
func (i *subscriberIndex) recordersFor(uri string) []*envRecorder {
	i.mu.Lock()
	defer i.mu.Unlock()
	return slices.Clone(i.watchers[uri])
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
			DispatchObserved:  conn.dispatchObserved.Load(),
		})
		return true
	})
	return lines
}

// lateDispatchLines writes every span the receiver holds.
//
// A call whose span arrived after its test's flush was written with no
// dispatched action, and this is what lets the audit join one to it anyway.
// Re-offering a line that was already written costs nothing: the writer drops
// a line whose JSON it has already seen.
func lateDispatchLines() []e2ecalls.Line {
	received := receiverIfStarted()
	if received == nil {
		return nil
	}
	all := received.all()
	lines := make([]e2ecalls.Line, 0, len(all))
	for traceID, record := range all {
		lines = append(lines, dispatchLine(traceID, record))
	}
	return lines
}
