//go:build e2e

// call.go is how a test asks a server to run an action.
//
// A test names a canonical action ID and the parameters; which tool that is,
// and what the arguments look like around them, is the projection's business.
// The verbs differ only in what they do with the answer: Do decodes it and
// fails the test if anything went wrong, Try hands both back, Refused asserts
// that a named class of refusal happened, and Eventually keeps asking until a
// predicate holds.
//
// Three things are checked before a call is sent, because each of them
// produces a failure that reads as something else once it has been sent. An ID
// the catalog does not have looks like a tool error from the server. An action
// above the tier the package declared looks like a permissions problem with
// the instance. An action this session does not serve looks like the server
// forgetting to register it, when the session was configured not to have it.

package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Expectation classes a call record can carry beside a [Failure], spelled as
// the record spells them.
const (
	// ExpectationOK is a call the test expected to succeed.
	ExpectationOK = e2ecalls.ExpectationOK
	// ExpectationAny is a call whose outcome the test did not constrain.
	ExpectationAny = e2ecalls.ExpectationAny
)

// Purpose says what a call was made for. A call made to build or tear down
// fixture state is worth less as coverage than one a test asserted on, so it
// is recorded rather than guessed at from the test name.
type Purpose string

// The four purposes, spelled as the shard record spells them.
const (
	// PurposeTest is a call the test body made and asserted on.
	PurposeTest Purpose = e2ecalls.PurposeTest
	// PurposeCleanup is a call made from a cleanup, after the test body.
	PurposeCleanup Purpose = e2ecalls.PurposeCleanup
	// PurposeSweep is a call made by a sweep over what a session serves.
	PurposeSweep Purpose = e2ecalls.PurposeSweep
	// PurposeRaw is a call made through Raw, where the protocol rather than
	// the action is what the test is about.
	PurposeRaw Purpose = e2ecalls.PurposeRaw
)

// String returns the purpose as a record spells it.
func (p Purpose) String() string { return string(p) }

// Failure is a class of unhappy answer a test can ask for by name.
//
// The classes are the server's own vocabulary wherever it has one: four of
// them are the refusal reasons the dispatchers log, so a test asking for
// FailureNeedsConfirmation is asking for exactly what the server counts under
// that name rather than for a string that happens to appear in a message.
type Failure string

// The failure classes. Anything a test expects that is not plain success is
// one of these, and the name is what the call record carries as its
// expectation.
const (
	// FailureToolError is an error result the classes below do not name: the
	// action ran and GitLab or the handler refused it.
	FailureToolError Failure = "tool_error"
	// FailureUnknownAction is a dispatcher told to run an action it has no
	// route for, which is also what a withheld action answers.
	FailureUnknownAction Failure = Failure(toolutil.RefusalUnknownAction)
	// FailureInvalidParams is a call whose arguments the dispatcher rejected
	// before any handler ran.
	FailureInvalidParams Failure = Failure(toolutil.RefusalInvalidParams)
	// FailureNeedsConfirmation is a destructive action refused for want of an
	// explicit confirmation.
	FailureNeedsConfirmation Failure = Failure(toolutil.RefusalNeedsConfirmation)
	// FailureSafeMode is a mutating action answered with a preview of itself.
	FailureSafeMode Failure = Failure(toolutil.RefusalSafeMode)
	// FailureNotFound is GitLab answering 404 for the object named.
	FailureNotFound Failure = "not_found"
	// FailureForbidden is GitLab answering 401 or 403 for the credential used.
	FailureForbidden Failure = "forbidden"
	// FailureProtocolError is a JSON-RPC error rather than a tool result: the
	// call never became a tool outcome at all.
	FailureProtocolError Failure = "protocol_error"
	// FailureWithheld is an action the session was configured not to serve,
	// declined on the wire. It is the expectation [Withheld] records, and it
	// names the configuration rather than the answer because the surfaces
	// answer differently: the dispatchers refuse the action as unknown or
	// unavailable, and the individual surface never registered the tool.
	FailureWithheld Failure = "withheld"
)

// String returns the class as a record spells it.
func (f Failure) String() string { return string(f) }

// callOptions is what the options a caller passed add up to.
type callOptions struct {
	purpose Purpose
	confirm bool
	timeout time.Duration
	// expectation is what the caller asked to happen, in the record's
	// vocabulary. Each verb sets it rather than the caller, because the verb
	// is the assertion: Do expects success, Refused expects a class.
	expectation string
	// dispatch is the route the caller declared this call would run, when the
	// server rewrites what was asked for.
	dispatch ActionID
}

// CallOption adjusts one call.
type CallOption func(*callOptions)

// For declares what a call was made for. Without it a call is PurposeTest.
func For(purpose Purpose) CallOption {
	return func(o *callOptions) { o.purpose = purpose }
}

// ExpectDispatch declares the route this call will actually run, for the cases
// where the server rewrites what it was asked for.
//
// Without it, a call whose span names another route fails its test, which is
// the assertion the whole recorder exists to make: a call that named issue.list
// and ran issue.get proves nothing about issue.list. The rewrites are real and
// deliberate, though: gitlab_environment get with an environment name runs
// protected_get, so a test of that behavior says so here and is held to it.
func ExpectDispatch(route ActionID) CallOption {
	return func(o *callOptions) { o.dispatch = route }
}

// WithoutConfirmation sends a destructive action with no explicit approval,
// which is how a test sees the refusal a client that cannot prompt is given.
// It changes nothing for an action that is not destructive.
func WithoutConfirmation() CallOption {
	return func(o *callOptions) { o.confirm = false }
}

// Within bounds one call, for an action whose own timeout is longer than the
// test is willing to wait.
func Within(timeout time.Duration) CallOption {
	return func(o *callOptions) { o.timeout = timeout }
}

// resolveCallOptions applies the caller's options over the defaults: a test
// call, confirmed if destructive, bounded by the test's own context, expected
// to succeed.
func resolveCallOptions(opts []CallOption) callOptions {
	resolved := callOptions{purpose: PurposeTest, confirm: true, expectation: e2ecalls.ExpectationOK}
	for _, opt := range opts {
		opt(&resolved)
	}
	return resolved
}

// expecting returns the options with the expectation a verb makes of its call,
// which is the verb's own and not the caller's to override.
func (o callOptions) expecting(expectation string) callOptions {
	o.expectation = expectation
	return o
}

// callResult is one answer, classified.
type callResult struct {
	// result is what the server returned, nil when nothing came back.
	result *mcp.CallToolResult
	// outcome is one of the e2ecalls Outcome constants, or a refusal spelled
	// with its reason.
	outcome string
	// failure is the class an unhappy answer falls into, empty on success.
	failure Failure
	// text is the first text content block, which is where every refusal this
	// server makes puts its reason.
	text string
	// duration is how long the call took.
	duration time.Duration
	// err is the transport or protocol error, nil when the server answered.
	err error
}

// ok reports whether the server ran the action and reported no failure.
func (r callResult) ok() bool { return r.err == nil && r.failure == "" }

// said returns the server's own words for an answer: the text of a tool
// result, or the message of a JSON-RPC error, which is the only text an
// unregistered tool is refused with.
func (r callResult) said() string {
	if r.text != "" {
		return r.text
	}
	if r.err != nil {
		return r.err.Error()
	}
	return ""
}

// describe is the one-line summary a failed assertion carries. The elapsed
// time is part of it because the two failures that look alike in a log, a
// refusal and a timeout, are told apart by it.
func (r callResult) describe() string {
	if r.err != nil {
		return fmt.Sprintf("%s after %s: %v", r.outcome, r.duration.Round(time.Millisecond), r.err)
	}
	if r.text != "" {
		return fmt.Sprintf("%s after %s: %s", r.outcome, r.duration.Round(time.Millisecond), firstLines(r.text, 6))
	}
	return fmt.Sprintf("%s after %s", r.outcome, r.duration.Round(time.Millisecond))
}

// callRetries is how many times a call that failed in transit is sent again.
// A Docker GitLab under the load of a whole suite drops connections, and the
// old suite retried four times for the same reason.
const callRetries = 4

// callRetryDelay is the step the delay between attempts grows by.
const callRetryDelay = 500 * time.Millisecond

// Do runs an action and decodes its answer, failing the test if the server did
// not run it.
func Do[O any](s *Session, id ActionID, params map[string]any, opts ...CallOption) O {
	s.env.T.Helper()

	var output O
	resolved := resolveCallOptions(opts)
	answer := s.invoke(id, params, resolved)
	if !answer.ok() {
		s.env.T.Fatalf("%s: %s%s", callLabel(id, resolved), answer.describe(), s.conn.failureContext())
		return output
	}
	if err := decodeResult(answer.result, &output); err != nil {
		s.env.T.Fatalf("%s: %v", callLabel(id, resolved), err)
	}
	return output
}

// DoVoid runs an action whose answer the test does not read, failing the test
// if the server did not run it.
func DoVoid(s *Session, id ActionID, params map[string]any, opts ...CallOption) {
	s.env.T.Helper()

	resolved := resolveCallOptions(opts)
	answer := s.invoke(id, params, resolved)
	if !answer.ok() {
		s.env.T.Fatalf("%s: %s%s", callLabel(id, resolved), answer.describe(), s.conn.failureContext())
	}
}

// Try runs an action and hands back both halves, for a test whose subject is
// the failure itself rather than a named class of it.
func Try[O any](s *Session, id ActionID, params map[string]any, opts ...CallOption) (O, error) {
	s.env.T.Helper()

	var output O
	answer := s.invoke(id, params, resolveCallOptions(opts).expecting(e2ecalls.ExpectationAny))
	if !answer.ok() {
		return output, errors.New(answer.describe())
	}
	if err := decodeResult(answer.result, &output); err != nil {
		return output, err
	}
	return output, nil
}

// Refused asserts that the server declined the call in the named class, and
// returns what it said so a test can assert on the wording it promises.
func Refused(s *Session, id ActionID, params map[string]any, want Failure, opts ...CallOption) string {
	s.env.T.Helper()

	resolved := resolveCallOptions(opts).expecting(string(want))
	answer := s.invoke(id, params, resolved)
	if answer.failure == "" {
		s.env.T.Fatalf("%s: the server ran the action, and the test expected it to be refused as %s",
			callLabel(id, resolved), want)
		return ""
	}
	if answer.failure != want {
		s.env.T.Fatalf("%s: refused as %s, and the test expected %s: %s",
			callLabel(id, resolved), answer.failure, want, firstLines(answer.text, 6))
	}
	return answer.text
}

// ExpectToolError asserts that the call came back as a tool error whose text
// carries the given substring.
//
// It is separate from Refused because the class is the same for every one of
// them: what distinguishes a GitLab refusal a test cares about is the message,
// and asserting on that message is the whole point of the call.
func ExpectToolError(s *Session, id ActionID, params map[string]any, contains string, opts ...CallOption) string {
	s.env.T.Helper()

	resolved := resolveCallOptions(opts).expecting(string(FailureToolError))
	answer := s.invoke(id, params, resolved)
	if answer.failure == "" {
		s.env.T.Fatalf("%s: the server ran the action, and the test expected an error mentioning %q",
			callLabel(id, resolved), contains)
		return ""
	}
	if !strings.Contains(strings.ToLower(answer.text), strings.ToLower(contains)) {
		s.env.T.Fatalf("%s: the error does not mention %q: %s",
			callLabel(id, resolved), contains, firstLines(answer.text, 6))
	}
	return answer.text
}

// Withheld asserts that an action this session was configured not to serve is
// declined on the wire, and returns what the server said.
//
// Every other verb refuses to send a call for an action the session does not
// serve, because sent by mistake it reads as a server defect. This one sends
// it on purpose: the refusal is the subject, and it is the whole of what
// read-only mode, a narrowed credential and an operator's exclusion look like
// to a client. The three surfaces decline in three shapes and all three are
// accepted here, since each is that surface's correct answer: the dynamic
// dispatcher names the action as unknown or as withheld with its reason; the
// meta dispatcher's tool no longer admits the action in its schema, so the
// SDK refuses the argument before the dispatcher runs, or the whole group is
// gone and the tool is unregistered; and the individual surface answers a
// JSON-RPC error for a tool it never registered. An action the session does
// serve fails the test, because then there is no withholding to assert and
// the call belongs to Refused.
func Withheld(s *Session, id ActionID, params map[string]any, opts ...CallOption) string {
	s.env.T.Helper()

	resolved := resolveCallOptions(opts).expecting(string(FailureWithheld))
	call, err := s.project(id, params, resolved.confirm)
	if err != nil {
		s.env.T.Fatalf("%v", err)
		return ""
	}
	if s.Serves(id) {
		s.env.T.Fatalf("%s: the %s session in %s mode serves it, so there is no withholding to assert; "+
			"use Refused or ExpectToolError for an action the session serves", callLabel(id, resolved), s.Surface(), s.Mode())
		return ""
	}

	answer := s.send(id, call, resolved)
	if answer.failure == "" {
		s.env.T.Fatalf("%s: the %s session in %s mode ran an action it was configured not to serve: %s",
			callLabel(id, resolved), s.Surface(), s.Mode(), answer.describe())
		return ""
	}
	if !withheldAnswer(answer) {
		s.env.T.Fatalf("%s: declined as %s rather than as a withheld action: %s",
			callLabel(id, resolved), answer.failure, firstLines(answer.said(), 6))
	}
	return answer.said()
}

// withheldAnswer reports whether an answer is one of the shapes a withheld
// action is declined in.
//
// The invalid-params shape is admitted only when it is the action argument
// the schema refused: a meta tool whose group lost the action lists the rest
// in its enum, and that is the refusal. A parameter of the action refused
// for another reason means the action was admitted, and so served.
func withheldAnswer(answer callResult) bool {
	switch answer.failure {
	case FailureUnknownAction:
		return true
	case FailureInvalidParams:
		return strings.Contains(answer.text, "/properties/action")
	case FailureProtocolError:
		return answer.err != nil && strings.Contains(answer.err.Error(), "unknown tool")
	default:
		return false
	}
}

// Eventually runs an action until the predicate accepts its answer, or the
// timeout runs out.
//
// GitLab makes a great deal of what this server asks for asynchronously: a
// merge request becomes mergeable, a pipeline leaves created, a project finishes
// importing. The predicate is what a test says it is waiting for, and the
// failure names the last answer rather than only the timeout.
func Eventually[O any](
	s *Session,
	id ActionID,
	params map[string]any,
	interval, timeout time.Duration,
	until func(O) bool,
	opts ...CallOption,
) O {
	s.env.T.Helper()

	var output O
	resolved := resolveCallOptions(opts)
	err := Poll(s.env.Ctx, interval, timeout, func() (bool, string, error) {
		answer := s.invoke(id, params, resolved)
		if !answer.ok() {
			// A failed read is worth waiting through: the object a test is
			// waiting for often does not exist yet when the first call is made.
			return false, answer.describe(), nil
		}
		var current O
		if decodeErr := decodeResult(answer.result, &current); decodeErr != nil {
			return false, "", decodeErr
		}
		if !until(current) {
			return false, "the answer does not satisfy the condition yet", nil
		}
		output = current
		return true, "", nil
	})
	if err != nil {
		s.env.T.Fatalf("waiting for %s: %v%s", callLabel(id, resolved), err, s.conn.failureContext())
	}
	return output
}

// callLabel names a call in a failure message: the action, and the purpose
// when it is not an ordinary test call. A failure inside a cleanup reads
// differently from one in the test body, and the message should say which.
func callLabel(id ActionID, opts callOptions) string {
	if opts.purpose == PurposeTest || opts.purpose == "" {
		return string(id)
	}
	return fmt.Sprintf("%s (%s)", id, opts.purpose)
}

// invoke resolves the action onto this session's surface and sends it.
func (s *Session) invoke(id ActionID, params map[string]any, opts callOptions) callResult {
	s.env.T.Helper()

	call, err := s.resolve(id, params, opts.confirm)
	if err != nil {
		s.env.T.Fatalf("%v", err)
		return callResult{}
	}
	return s.send(id, call, opts)
}

// resolve turns an action ID into the call this session takes, refusing the
// three cases that would otherwise be sent and misread.
func (s *Session) resolve(id ActionID, params map[string]any, confirm bool) (toolCall, error) {
	call, err := s.project(id, params, confirm)
	if err != nil {
		return toolCall{}, err
	}
	if !s.Serves(id) {
		return toolCall{}, fmt.Errorf("the %s session does not serve %s: the %s mode, the credential's scopes, the %s tier "+
			"its credential could read, or the operator's exclusions removed it", s.Surface(), id, s.Mode(), s.Tier())
	}
	return call, nil
}

// project spells the call this session's surface takes for an action, without
// asking whether the session serves it.
//
// The projection is asked first, because it knows the surface's own reason: an
// action whose individual tool name a sibling owns is unservable for that
// reason and not because a mode removed it, and the session's action set
// cannot tell the two apart. Whether the session serves the action is the
// caller's question, and [Withheld] deliberately asks the opposite one.
func (s *Session) project(id ActionID, params map[string]any, confirm bool) (toolCall, error) {
	inst := s.conn.inst
	projected, err := newProjection(inst.facts.Tier, inst.client.IsGitLabDotCom())
	if err != nil {
		return toolCall{}, err
	}
	action, known := projected.lookup(id)
	if !known {
		return toolCall{}, fmt.Errorf("no action %s in the catalog this instance serves (tier %s)", id, inst.facts.Tier)
	}
	if ceiling := inst.actionTierCeiling(); !ceiling.AtLeast(action.minimumTier) {
		return toolCall{}, fmt.Errorf("action %s needs %s, and the %s package runs on %s: move the scenario to a "+
			"package whose requirement provides it", id, action.minimumTier, inst.pkg, ceiling)
	}
	return action.callOn(s.Surface(), params, confirm)
}

// send makes the call, retrying what failed in transit.
//
// The context carries who is making it, which is what the recorder reads on
// the way out: the middleware sits on a client many tests share, and only the
// caller knows whose call this is, what it is for and what it expects. Each
// attempt passes through that middleware and so gets a trace and a record of
// its own, which is right: a retry is another call, and two identical lines
// would be one line in a record deduplicated by content.
func (s *Session) send(id ActionID, call toolCall, opts callOptions) callResult {
	ctx := s.attribute(opts.purpose, opts.expectation, callAttribution{
		action:       id,
		wantDispatch: opts.dispatch,
	})
	if opts.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.timeout)
		defer cancel()
	}

	var answer callResult
	for attempt := range callRetries {
		started := time.Now()
		result, err := s.conn.client().CallTool(ctx, &mcp.CallToolParams{Name: call.tool, Arguments: call.arguments})
		answer = classify(result, err)
		answer.duration = time.Since(started)

		if answer.err == nil || !retryable(answer, s.conn) || attempt == callRetries-1 {
			return answer
		}
		// A child that died takes its pipe with it, so reconnecting is the
		// only thing that can make the next attempt different.
		if !s.conn.alive() {
			if restartErr := s.conn.restart(); restartErr != nil {
				return answer
			}
		}
		select {
		case <-ctx.Done():
			return answer
		case <-time.After(time.Duration(attempt+1) * callRetryDelay):
		}
	}
	return answer
}

// retryable reports whether an answer is worth sending again: a connection
// GitLab or the child dropped, and nothing else. A tool error is an answer.
func retryable(answer callResult, conn *sessionConn) bool {
	if answer.err == nil {
		return false
	}
	if !conn.alive() {
		return true
	}
	message := answer.err.Error()
	for _, transient := range []string{"EOF", "connection reset by peer", "broken pipe", "connection refused"} {
		if strings.Contains(message, transient) {
			return true
		}
	}
	return false
}

// classify reads one answer into the vocabulary the record and the assertions
// share.
func classify(result *mcp.CallToolResult, err error) callResult {
	if err != nil {
		answer := callResult{err: err, failure: FailureProtocolError, outcome: e2ecalls.OutcomeProtocolError}
		if isTransportError(err) {
			answer.outcome = e2ecalls.OutcomeTransportError
		}
		return answer
	}
	if result == nil {
		return callResult{
			err:     errors.New("the server answered with no result and no error"),
			failure: FailureProtocolError,
			outcome: e2ecalls.OutcomeProtocolError,
		}
	}

	answer := callResult{result: result, text: resultText(result), outcome: e2ecalls.OutcomeOK}
	if preview, isPreview := safeModePreview(result, answer.text); isPreview {
		answer.outcome = e2ecalls.OutcomePreview
		answer.failure = FailureSafeMode
		answer.text = preview
		return answer
	}
	if !result.IsError {
		return answer
	}
	answer.failure = classifyToolError(answer.text)
	if answer.failure == FailureToolError {
		answer.outcome = e2ecalls.OutcomeToolError
		return answer
	}
	answer.outcome = e2ecalls.RefusedOutcome(string(answer.failure))
	return answer
}

// isTransportError reports whether the call never reached a handler at all.
func isTransportError(err error) bool {
	message := err.Error()
	for _, transient := range []string{"EOF", "connection reset by peer", "broken pipe", "connection refused", "file already closed"} {
		if strings.Contains(message, transient) {
			return true
		}
	}
	return false
}

// safeModePreview reports whether a result is a safe-mode preview, and returns
// the name of the action it previewed.
//
// Both shapes are read, because the surfaces answer differently: the
// dispatchers return the preview as structured content, and the individual
// surface returns the card. Reading only one of them would classify half the
// previews as ordinary successes.
func safeModePreview(result *mcp.CallToolResult, text string) (string, bool) {
	if result.StructuredContent != nil {
		encoded, err := json.Marshal(result.StructuredContent)
		if err == nil {
			var preview toolutil.SafeModePreview
			if json.Unmarshal(encoded, &preview) == nil && preview.Status == "blocked" && preview.Mode == "safe" {
				return "safe mode blocked " + preview.Tool, true
			}
		}
	}
	if preview, isPreview := toolutil.ParseSafeModePreview(text); isPreview {
		return "safe mode blocked " + preview.Tool, true
	}
	return "", false
}

// answeredStatus matches the status client-go writes after the request it
// made, "METHOD URL: 404 body", which is the one GitLab actually answered. It
// asks for the colon and the space around the number so that a group or a
// project whose id happens to be 404 is not read as a refusal.
var answeredStatus = regexp.MustCompile(`: (40[134])\b`)

// classifyToolError names the class an error result falls into, from the text
// the server put in it.
//
// The markers are the server's own wording. Reading them is a compromise the
// alternative does not improve on: the refusal reason is on the span rather
// than in the result, so the only thing a client holds at the moment it has to
// classify is the message, and a test that named no class at all could not
// tell a refused confirmation from a 404.
//
// GitLab's own status is read off the last one the text answers with, and
// only then off the words: the hints this server writes name the statuses an
// endpoint can answer ("can return 401 or 404"), and a 401 refusal whose hint
// mentioned 404 used to be classified as not found on the strength of the
// hint alone.
func classifyToolError(text string) Failure {
	lowered := strings.ToLower(text)
	switch {
	case strings.Contains(lowered, "re-send with confirm=true"):
		return FailureNeedsConfirmation
	case strings.Contains(lowered, "unknown action"),
		// The dynamic dispatcher's answer for an action a scope or the
		// operator withheld. It is logged on the span as unknown_action, so
		// it is classified as the server counts it and not as a plain
		// tool error, which is what its wording would otherwise read as.
		strings.Contains(lowered, "exists but is not available"):
		return FailureUnknownAction
	case strings.Contains(lowered, "is required for this action"),
		strings.Contains(lowered, "missing required params"),
		strings.Contains(lowered, "'action' is required"),
		// The SDK's own schema validation, which runs before any handler:
		// an argument the tool's input schema refuses never reaches the
		// dispatcher, and reads as the same class as a dispatcher refusing it.
		strings.Contains(lowered, `validating "arguments"`):
		return FailureInvalidParams
	}
	if answered := answeredStatus.FindAllStringSubmatch(lowered, -1); len(answered) > 0 {
		if answered[len(answered)-1][1] == "404" {
			return FailureNotFound
		}
		return FailureForbidden
	}
	switch {
	case strings.Contains(lowered, "not found"):
		return FailureNotFound
	case strings.Contains(lowered, "forbidden"), strings.Contains(lowered, "unauthorized"):
		return FailureForbidden
	default:
		return FailureToolError
	}
}

// resultText returns the first text block of a result, which is where every
// message this server writes for a person or a model ends up.
func resultText(result *mcp.CallToolResult) string {
	for _, content := range result.Content {
		if text, isText := content.(*mcp.TextContent); isText {
			return text.Text
		}
	}
	return ""
}

// decodeResult reads a tool result into the output type, preferring the
// structured content and falling back to the text block.
//
// The fallback is not theoretical: a tool whose output schema the SDK could
// not derive answers with text alone, and a decoder that only read structured
// content would report an empty object for it.
func decodeResult(result *mcp.CallToolResult, output any) error {
	if result == nil {
		return errors.New("no result to decode")
	}
	if result.StructuredContent != nil {
		encoded, err := json.Marshal(result.StructuredContent)
		if err != nil {
			return fmt.Errorf("re-encoding the structured content: %w", err)
		}
		if err = json.Unmarshal(encoded, output); err != nil {
			return fmt.Errorf("decoding the structured content into %T: %w", output, err)
		}
		return nil
	}
	text := resultText(result)
	if text == "" {
		return fmt.Errorf("the result carries neither structured content nor text to decode into %T", output)
	}
	if err := json.Unmarshal([]byte(text), output); err != nil {
		return fmt.Errorf("decoding the text result into %T: %w", output, err)
	}
	return nil
}

// actionTierCeiling is the highest tier an action may need for this package to
// be allowed to call it.
//
// A package that declares no license runs Free actions only, whatever the
// instance turns out to be. That is the runtime half of the rule the static
// gate enforces: an action above Free named in the common or ce package would
// pass on a licensed instance and fail on an unlicensed one, and a suite whose
// result depends on which runtime happened to run it is not a gate.
func (inst *instance) actionTierCeiling() edition.Tier {
	if inst.requirement == Licensed {
		return inst.facts.Tier
	}
	return edition.Free
}

// firstLines returns at most n lines of text, for a failure message that has
// to carry a server's answer without burying its own first line.
func firstLines(text string, n int) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[:n], "\n") + fmt.Sprintf("\n... (%d more lines)", len(lines)-n)
}
