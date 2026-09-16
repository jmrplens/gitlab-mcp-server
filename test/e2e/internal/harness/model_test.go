//go:build e2e

// model_test.go drives the model verb the way a conversation drives it: the
// real binary on a real surface against a stub GitLab, one call at a time,
// with the server's own span coming back over the loopback receiver before the
// answer is returned.
//
// The four things it has to be right about are the four a scorer reads and
// cannot check for itself: that a dispatch is returned per call rather than at
// the flush, that a call whose span never came says so instead of reporting an
// empty action as fact, that a discovery call is distinguishable from an
// execute call on the server's word, and that a call the server withheld
// carries the server's own reason.

package harness

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dynamic"
)

// modelArguments spells one model's argument object, failing the test when it
// cannot be encoded.
//
// A model produces JSON text and this package's tests produce Go values, so
// the encoding happens once here rather than as a string literal per case,
// where a missing brace would read as a defect in the verb.
func modelArguments(t *testing.T, arguments map[string]any) json.RawMessage {
	t.Helper()

	encoded, err := json.Marshal(arguments)
	if err != nil {
		t.Fatalf("encoding the model's arguments: %v", err)
	}
	return encoded
}

// TestCallAsModel_ExecutedAction_ReturnsTheServersOwnDispatch is the assertion
// the verb exists to make.
//
// The dispatch has to come back with the call and not at the flush, because a
// conversation is scored turn by turn: what the server ran for one call is what
// decides whether the next one is a repair, a retry or the next step, and by
// the time the flush resolves the spans the conversation is over.
func TestCallAsModel_ExecutedAction_ReturnsTheServersOwnDispatch(t *testing.T) {
	env := newEnv(t, stubInstance(t))
	session := env.Session(ServerConfig{Surface: SurfaceDynamic, Private: true})

	answer := session.CallAsModel(env.Ctx, ModelCall{
		Tool: dynamictools.ExecuteActionToolName,
		Arguments: modelArguments(t, map[string]any{
			"action": "project.get",
			"params": map[string]any{"project_id": "group/project"},
		}),
	})

	if !answer.DispatchObserved {
		t.Fatalf("DispatchObserved = false, want true: the server's span did not arrive, so nothing here is about "+
			"what ran (outcome %q, trace %q)", answer.Outcome, answer.TraceID)
	}
	if answer.Dispatch.Action != "project.get" {
		t.Errorf("Dispatch.Action = %q, want project.get", answer.Dispatch.Action)
	}
	if answer.Dispatch.Tool != dynamictools.ExecuteActionToolName {
		t.Errorf("Dispatch.Tool = %q, want %q", answer.Dispatch.Tool, dynamictools.ExecuteActionToolName)
	}
	if answer.Dispatch.Domain != "project" {
		t.Errorf("Dispatch.Domain = %q, want project", answer.Dispatch.Domain)
	}
	// The three facts nothing else here reads, against a live span. The stub
	// answers this path with a 404, so the handler returned a tool error and
	// the server's own span says so; a scorer reading either of them as empty
	// would read a refused call as a call that succeeded.
	if answer.Dispatch.ErrorType != "tool_error" {
		t.Errorf("Dispatch.ErrorType = %q, want tool_error: the stub refused the action with a 404",
			answer.Dispatch.ErrorType)
	}
	if answer.Dispatch.Status != "STATUS_CODE_ERROR" {
		t.Errorf("Dispatch.Status = %q, want STATUS_CODE_ERROR", answer.Dispatch.Status)
	}
	// A floor rather than an equality, because the count is one: the client
	// spans end before the server's and are exported before it, so they have
	// landed by the time this returns, but a batch queue that overflowed drops
	// them silently and a run must not fail for that. Zero is the defect this
	// catches, and it is the one that would write no count on every line.
	if answer.Requests < 1 {
		t.Errorf("Requests = %d, want at least the one request the handler made", answer.Requests)
	}
	if answer.TraceID == "" {
		t.Error("TraceID is empty: the call carried no trace, so no span could ever be joined to it")
	}
	// The stub answers every path but two with a 404, so the action ran and
	// GitLab refused. That is the point: a dispatch is what the server did
	// with the call, and it is observable whether or not GitLab said yes.
	if answer.Outcome == "" {
		t.Error("Outcome is empty: every answer falls into a class")
	}
	if answer.Duration <= 0 {
		t.Errorf("Duration = %v, want a positive elapsed time", answer.Duration)
	}
}

// TestCallAsModel_ExecutedAction_IsRecordedAsAModelCall checks the purpose the
// call line carries.
//
// It is the whole of what keeps a model run out of the coverage report: the
// audit refuses credit by the purpose alone, so a call recorded under any
// other word would raise this suite's coverage by whatever the model tried.
func TestCallAsModel_ExecutedAction_IsRecordedAsAModelCall(t *testing.T) {
	env := newEnv(t, stubInstance(t))
	session := env.Session(ServerConfig{Surface: SurfaceDynamic, Private: true})

	session.CallAsModel(env.Ctx, ModelCall{
		Tool: dynamictools.ExecuteActionToolName,
		Arguments: modelArguments(t, map[string]any{
			"action": "project.get",
			"params": map[string]any{"project_id": "group/project"},
		}),
	})

	reporter := &capturedReporter{}
	lines := env.recorder.finish(reporter, e2ecalls.StatusPassed)

	var found *e2ecalls.Call
	for _, line := range lines {
		call, isCall := line.(*e2ecalls.Call)
		if isCall && call.Tool == dynamictools.ExecuteActionToolName {
			found = call
		}
	}
	if found == nil {
		t.Fatalf("the record holds no call of %s: %s", dynamictools.ExecuteActionToolName, describeLines(lines))
	}
	if found.Purpose != e2ecalls.PurposeModel {
		t.Errorf("purpose = %q, want %q: the coverage audit refuses credit by this word alone",
			found.Purpose, e2ecalls.PurposeModel)
	}
	if found.Action != "" {
		t.Errorf("action = %q, want empty: a model call names a tool, and what action that is is the "+
			"measurement rather than a declaration", found.Action)
	}
	if reporter.count() != 0 {
		t.Errorf("the flush failed the test that made a model call: %s", reporter.reported())
	}
}

// TestCallAsModel_DiscoveryCall_ReturnsAToolOnlyDispatch checks that a find is
// told from an execute on the server's own word.
//
// It matters because the published dynamic rows of the evaluator this replaces
// measured the find call under the name of the catalog call: the expected-step
// list had a find step prepended on dynamic, so three columns were reporting
// the discovery. A dispatch that names a tool and no action is what lets the
// scorer count discovery as overhead and match nothing against it.
func TestCallAsModel_DiscoveryCall_ReturnsAToolOnlyDispatch(t *testing.T) {
	env := newEnv(t, stubInstance(t))
	session := env.Session(ServerConfig{Surface: SurfaceDynamic, Private: true})

	answer := session.CallAsModel(env.Ctx, ModelCall{
		Tool:      dynamictools.FindActionToolName,
		Arguments: modelArguments(t, map[string]any{"query": "list the issues of a project"}),
	})

	if !answer.DispatchObserved {
		t.Fatalf("DispatchObserved = false, want true (outcome %q)", answer.Outcome)
	}
	if answer.Dispatch.Tool != dynamictools.FindActionToolName {
		t.Errorf("Dispatch.Tool = %q, want %q", answer.Dispatch.Tool, dynamictools.FindActionToolName)
	}
	if answer.Dispatch.Action != "" {
		t.Errorf("Dispatch.Action = %q, want empty: a discovery call runs no action, and one reported here "+
			"would be counted as a step the model reached", answer.Dispatch.Action)
	}
	if answer.Outcome != e2ecalls.OutcomeOK {
		t.Errorf("Outcome = %q, want ok: a good find is an answer and never an error", answer.Outcome)
	}
}

// TestCallAsModel_WithheldAction_CarriesTheServersRefusal checks the answer a
// model gets for an action the mode removed, on each dispatcher surface.
//
// Read-only is where calling is the wrong behavior and the scoring has to say
// so, which it can only do from the server's own account: whatever the model
// asked for, the server dispatched nothing, and a harness reporting the
// requested action here would let a model be scored as having reached a step
// that never ran.
//
// The two surfaces refuse in two different ways, and the difference is not
// cosmetic. On dynamic the execute dispatcher refuses the action itself and
// answers with the withheld message, so the span carries the server's own
// reason. On meta the action is gone from the tool's own action enum, so the
// SDK's schema validation refuses the call before any handler of ours runs:
// the span carries the tool and the domain and no reason at all, and the only
// evidence of what happened is the message naming /properties/action. A scorer
// that recognized unknown_action alone would read every correctly declined
// meta step as a model failure.
func TestCallAsModel_WithheldAction_CarriesTheServersRefusal(t *testing.T) {
	inst := stubInstance(t)

	cases := []struct {
		name          string
		surface       Surface
		tool          string
		args          map[string]any
		wantFailure   Failure
		wantReason    string
		wantTextNames string
	}{
		{
			name:    "dynamic",
			surface: SurfaceDynamic,
			tool:    dynamictools.ExecuteActionToolName,
			args: map[string]any{
				"action": "project.create",
				"params": map[string]any{"name": "withheld"},
			},
			wantFailure: FailureUnknownAction,
			// The dispatcher reached its own refusal, so the span says why.
			wantReason:    "unknown_action",
			wantTextNames: "withhold",
		},
		{
			name:    "meta",
			surface: SurfaceMeta,
			tool:    "gitlab_project",
			args: map[string]any{
				"action": "create",
				"params": map[string]any{"name": "withheld"},
			},
			wantFailure: FailureInvalidParams,
			// Nothing of ours ran, so there is no reason of ours to record.
			wantReason:    "",
			wantTextNames: "/properties/action",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			env := newEnv(t, inst)
			session := env.Session(ServerConfig{Surface: testCase.surface, Mode: ModeReadOnly, Private: true})

			answer := session.CallAsModel(env.Ctx, ModelCall{
				Tool:      testCase.tool,
				Arguments: modelArguments(t, testCase.args),
			})

			if !answer.DispatchObserved {
				t.Fatalf("DispatchObserved = false, want true (outcome %q)", answer.Outcome)
			}
			if answer.Dispatch.Tool != testCase.tool {
				t.Errorf("Dispatch.Tool = %q, want %q", answer.Dispatch.Tool, testCase.tool)
			}
			if answer.Dispatch.Action != "" {
				t.Errorf("Dispatch.Action = %q, want empty: the server dispatched nothing",
					answer.Dispatch.Action)
			}
			if answer.Dispatch.RefusalReason != testCase.wantReason {
				t.Errorf("Dispatch.RefusalReason = %q, want %q", answer.Dispatch.RefusalReason, testCase.wantReason)
			}
			if answer.Failure != testCase.wantFailure {
				t.Errorf("Failure = %q, want %q", answer.Failure, testCase.wantFailure)
			}
			if answer.Outcome != e2ecalls.RefusedOutcome(string(testCase.wantFailure)) {
				t.Errorf("Outcome = %q, want the refusal spelled with its reason", answer.Outcome)
			}
			if !strings.Contains(answer.Text, testCase.wantTextNames) {
				t.Errorf("Text = %.200q, want it to name %q: the model reads the server's own words, and a "+
					"scorer reads what it read", answer.Text, testCase.wantTextNames)
			}
		})
	}
}

// TestCallAsModel_UnregisteredTool_IsAProtocolErrorWithNoDispatch checks what
// naming a tool the surface does not serve produces.
//
// On the individual surface that is how every withheld action looks, so the
// class has to be right: a JSON-RPC error carrying the server's message, no
// dispatch, and a Failure a caller can switch on rather than a bare string to
// match against.
func TestCallAsModel_UnregisteredTool_IsAProtocolErrorWithNoDispatch(t *testing.T) {
	env := newEnv(t, stubInstance(t))
	session := env.Session(ServerConfig{Surface: SurfaceDynamic, Private: true})

	answer := session.CallAsModel(env.Ctx, ModelCall{
		Tool:      "gitlab_no_such_tool",
		Arguments: modelArguments(t, map[string]any{}),
	})

	if answer.Err == nil {
		t.Fatalf("Err = nil, want the server's refusal of an unregistered tool (outcome %q)", answer.Outcome)
	}
	if answer.Outcome != e2ecalls.OutcomeProtocolError {
		t.Errorf("Outcome = %q, want %q", answer.Outcome, e2ecalls.OutcomeProtocolError)
	}
	if answer.Failure != FailureProtocolError {
		t.Errorf("Failure = %q, want %q", answer.Failure, FailureProtocolError)
	}
	if answer.Dispatch.Action != "" {
		t.Errorf("Dispatch.Action = %q, want empty: nothing was dispatched", answer.Dispatch.Action)
	}
	if answer.Result != nil {
		t.Errorf("Result = %v, want nil: a JSON-RPC error carries no result", answer.Result)
	}
}

// TestDispatchFactsOf_ASpanCarryingEveryFact_CopiesEachIntoItsOwnField pins
// the copy a live run cannot check.
//
// Every field here is a plausible value for every other field, so a copy taken
// from the wrong one produces a record a reader would believe: a line naming
// the domain as the action, or an error type that is really a refusal reason,
// says something false about what the server did and looks like an ordinary
// line while doing it. Distinct values are what make the swap visible, and a
// hand-written span is what makes the assertion exact where the live tests can
// only say what the stub happened to answer.
func TestDispatchFactsOf_ASpanCarryingEveryFact_CopiesEachIntoItsOwnField(t *testing.T) {
	kept := traceSpans{
		dispatch: dispatchRecord{
			tool:          "gitlab_execute_action",
			action:        "issue.list",
			domain:        "issue",
			refusalReason: "safe_mode",
			errorType:     "tool_error",
			status:        "STATUS_CODE_ERROR",
		},
		requests: 3,
	}

	facts, requests := dispatchFactsOf(kept)

	want := DispatchFacts{
		Tool:          "gitlab_execute_action",
		Action:        "issue.list",
		Domain:        "issue",
		RefusalReason: "safe_mode",
		ErrorType:     "tool_error",
		Status:        "STATUS_CODE_ERROR",
	}
	if facts != want {
		t.Errorf("dispatchFactsOf() = %+v, want %+v", facts, want)
	}
	if requests != kept.requests {
		t.Errorf("dispatchFactsOf() reports %d requests, want %d: the count is written verbatim onto every "+
			"record line, so a zero here is a zero on every call of a run", requests, kept.requests)
	}
}

// TestDispatchFactsOf_ASpanThatSaidNothing_IsTheZeroValue is the other half:
// the facts of a trace the server reported nothing about are empty rather than
// whatever a previous copy left behind.
func TestDispatchFactsOf_ASpanThatSaidNothing_IsTheZeroValue(t *testing.T) {
	facts, requests := dispatchFactsOf(traceSpans{})

	if facts != (DispatchFacts{}) {
		t.Errorf("dispatchFactsOf() = %+v, want the zero value", facts)
	}
	if requests != 0 {
		t.Errorf("dispatchFactsOf() reports %d requests, want 0", requests)
	}
}

// TestDispatchOf_NoSpanArrives_ReportsUnobservedWithinTheGrace checks the
// answer for a call the server never reported on.
//
// It has to be false rather than an empty dispatch taken as fact: every
// verdict derived from a call whose span never came would be a claim about
// what the model asked for and nothing about what the server did, and the
// scorer marks those unobserved instead of scoring them. The grace is the
// other half: a run whose telemetry is broken must not pay the full wait on
// every call of a conversation.
func TestDispatchOf_NoSpanArrives_ReportsUnobservedWithinTheGrace(t *testing.T) {
	// A receiver of this test's own, with no listener and nothing received, so
	// the wait is the one a run with broken telemetry makes. Swapping the
	// package's pointer is safe here because these tests are sequential and
	// the original is put back before anything else runs.
	previous := startedSpans.Load()
	t.Cleanup(func() { startedSpans.Store(previous) })
	startedSpans.Store(&spanReceiver{issued: map[string]struct{}{}, seen: map[string]traceSpans{}})

	// The give-up flag is what turns the budget down, and it is restored for
	// the same reason the receiver is. It is process-wide and set once: left
	// set, every later wait on a receiver that has observed nothing takes the
	// grace instead of the full budget, and the once-only CompareAndSwap in
	// awaitTraces can never fire again, so a run whose telemetry breaks after
	// this test is never told. Under -shuffle this test is not last.
	previousGaveUp := dispatchGaveUp.Load()
	t.Cleanup(func() { dispatchGaveUp.Store(previousGaveUp) })
	dispatchGaveUp.Store(true)

	started := time.Now()
	facts, requests, observed := dispatchOf("00000000000000000000000000000001")
	elapsed := time.Since(started)

	if observed {
		t.Error("observed = true, want false: no span was ever sent for that trace")
	}
	if facts != (DispatchFacts{}) {
		t.Errorf("facts = %+v, want the zero value", facts)
	}
	if requests != 0 {
		t.Errorf("requests = %d, want 0", requests)
	}
	if elapsed >= dispatchWait {
		t.Errorf("the wait took %v, which is the full budget of %v: the grace of %v did not apply",
			elapsed, dispatchWait, dispatchGrace)
	}
}

// TestDispatchOf_NoTraceAndNoReceiver_ReportsUnobservedAtOnce covers the two
// ways there is nothing to wait for.
//
// A call whose params could carry no trace was never joined to anything, and a
// run with no receiver has nowhere for a span to arrive. Both answer at once:
// waiting a budget for a span that was never issued would cost a conversation
// its whole time limit for no information.
func TestDispatchOf_NoTraceAndNoReceiver_ReportsUnobservedAtOnce(t *testing.T) {
	t.Run("no trace", func(t *testing.T) {
		if _, _, observed := dispatchOf(""); observed {
			t.Error("observed = true, want false for a call that carried no trace")
		}
	})

	t.Run("no receiver", func(t *testing.T) {
		previous := startedSpans.Load()
		t.Cleanup(func() { startedSpans.Store(previous) })
		startedSpans.Store(nil)

		if _, _, observed := dispatchOf("00000000000000000000000000000002"); observed {
			t.Error("observed = true, want false for a run with no receiver")
		}
	})
}

// TestCallAsModel_CallWithADeadContext_ReportsATransportError checks that a
// model call fails nothing when the conversation is already over.
//
// The verb asserts nothing by design, so a cancelled context has to come back
// as an answer rather than as a failed test: a turn budget that ran out is a
// fact about the attempt and is recorded as one.
func TestCallAsModel_CallWithADeadContext_ReportsATransportError(t *testing.T) {
	env := newEnv(t, stubInstance(t))
	session := env.Session(ServerConfig{Surface: SurfaceDynamic, Private: true})

	ctx, cancel := context.WithCancel(env.Ctx)
	cancel()

	answer := session.CallAsModel(ctx, ModelCall{
		Tool:      dynamictools.FindActionToolName,
		Arguments: modelArguments(t, map[string]any{"query": "anything"}),
	})

	if answer.Err == nil {
		t.Fatal("Err = nil, want the cancellation")
	}
	if answer.DispatchObserved {
		t.Error("DispatchObserved = true, want false: the call never reached a dispatcher")
	}
}
