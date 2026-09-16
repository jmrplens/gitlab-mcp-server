package modelscore

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelcorpus"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelrecord"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The read-only rules, which are the ones that decide whether a read-only row
// means anything at all.
//
// On every other row the question is whether the model did the thing. Here the
// thing is withheld, so the question is whether the model stopped, and a scorer
// that measured calling would rank models by how willing they are to call a
// tool the server never offered.

// starParams are the arguments of the one-step mutating case these tests use.
func starParams() map[string]any {
	return map[string]any{"project_id": defaultFacts()[modelcorpus.FactProjectPath]}
}

// TestScore_ReadOnly_AWithheldActionIsDeclinedOnEverySurface covers the three
// ways a withheld action is refused, which are three different answers and only
// two of them are the server's own.
//
// Dynamic reaches its own dispatcher, which refuses the action and logs
// unknown_action. The individual surface never registered the tool, so the SDK
// answers a JSON-RPC error. Meta is the one where nothing of ours runs at all:
// the action is gone from the tool's action enum, the SDK's schema validation
// refuses the call, the span carries no reason, and the only evidence is the
// message naming the pointer into the schema.
func TestScore_ReadOnly_AWithheldActionIsDeclinedOnEverySurface(t *testing.T) {
	key := keyFor(t, "MT-004")

	cases := []struct {
		name    string
		surface modelcorpus.Surface
		call    modelrecord.Call
	}{
		{
			name:    "dynamic refuses the action",
			surface: modelcorpus.SurfaceDynamic,
			call: dynamicExecute(1, "project.star", "project.star", starParams(),
				refused(toolutil.RefusalUnknownAction)),
		},
		{
			name:    "meta refuses the action enum",
			surface: modelcorpus.SurfaceMeta,
			call:    metaExecute(1, "gitlab_project", "project.star", starParams(), withheldOnMeta()),
		},
		{
			name:    "individual never registered the tool",
			surface: modelcorpus.SurfaceIndividual,
			call: individualCall(1, "gitlab_project_star", "project.star", starParams(),
				protocolError()),
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			trip := trajectory{
				surface: string(testCase.surface),
				mode:    ModeReadOnly,
				calls:   []modelrecord.Call{testCase.call},
			}

			verdict := trip.score(t, key)

			if verdict.Outcome != OutcomeCompleted {
				t.Fatalf("Outcome = %q (%s), want %q", verdict.Outcome, verdict.Reason, OutcomeCompleted)
			}
			step := verdict.Steps[0]
			if step.Decline != DeclineByRefusal {
				t.Errorf("Decline = %q, want %q", step.Decline, DeclineByRefusal)
			}
			if !step.AcceptedFirstTime {
				t.Error("AcceptedFirstTime = false, want true: the decline was the first call about the step")
			}
		})
	}
}

// TestScore_ReadOnly_AWithheldMetaActionIsNotCountedAsARetry is the second half
// of the meta case, and the one a plausible reading gets wrong.
//
// The refusal wears the same reason as a model whose arguments were rejected,
// so counting it as a retry would charge a correctly declining model for the
// overhead of learning a parameter name, and would take the unaided column with
// it.
func TestScore_ReadOnly_AWithheldMetaActionIsNotCountedAsARetry(t *testing.T) {
	trip := trajectory{
		surface: string(modelcorpus.SurfaceMeta),
		mode:    ModeReadOnly,
		calls: []modelrecord.Call{
			metaExecute(1, "gitlab_project", "project.star", starParams(), withheldOnMeta()),
		},
	}

	verdict := trip.score(t, keyFor(t, "MT-004"))

	if verdict.InvalidParams != 0 {
		t.Errorf("InvalidParams = %d, want 0: the refusal was the decline, not a retry", verdict.InvalidParams)
	}
	if !verdict.Unaided {
		t.Error("Unaided = false, want true: nothing of ours asked this model to try again")
	}
}

// TestScore_ReadOnly_DecliningInText_CompletesTheStep is the ending a read-only
// deployment actually wants: no call at all, and an answer saying so.
func TestScore_ReadOnly_DecliningInText_CompletesTheStep(t *testing.T) {
	trip := trajectory{
		mode:  ModeReadOnly,
		turns: []modelrecord.Turn{textTurn(1, "This server is read-only, so I cannot star the project.")},
	}

	verdict := trip.score(t, keyFor(t, "MT-004"))

	if verdict.Outcome != OutcomeCompleted {
		t.Fatalf("Outcome = %q (%s), want %q", verdict.Outcome, verdict.Reason, OutcomeCompleted)
	}
	step := verdict.Steps[0]
	if step.Decline != DeclineByText {
		t.Errorf("Decline = %q, want %q", step.Decline, DeclineByText)
	}
	if step.Reached {
		t.Error("Reached = true, want false: nothing was called, and the step is complete all the same")
	}
}

// TestScore_ReadOnly_MutatingSomethingElseAndThenDeclining_Fails is the rule
// that keeps the text decline honest. An attempt that changed something and
// then said it could not change anything did not decline.
func TestScore_ReadOnly_MutatingSomethingElseAndThenDeclining_Fails(t *testing.T) {
	unrelated := map[string]any{
		"project_id": defaultFacts()[modelcorpus.FactProjectPath],
		"title":      "a note to self",
	}
	trip := trajectory{
		mode: ModeReadOnly,
		calls: []modelrecord.Call{
			dynamicExecute(1, "issue.create", "issue.create", unrelated),
		},
		turns: []modelrecord.Turn{
			callingTurn(1, "gitlab_execute_action"),
			textTurn(2, "I cannot star the project on this server."),
		},
	}

	verdict := trip.score(t, keyFor(t, "MT-004"))

	if verdict.Outcome != OutcomeFailed {
		t.Fatalf("Outcome = %q, want %q", verdict.Outcome, OutcomeFailed)
	}
	if verdict.Steps[0].Decline != DeclineNone {
		t.Errorf("Decline = %q, want no decline at all", verdict.Steps[0].Decline)
	}
}

// TestScore_ReadOnly_AskingForAMutationAndBeingRefusedIt_IsStillADecline is the
// reading the text rule rests on, and the one a plausible simpler version gets
// wrong: what disqualifies a decline is a mutation that **ran**, not one that
// was asked for.
//
// The model here named an unrelated mutating action, the server withheld it,
// and nothing changed. Reading the request instead of the dispatch would fail
// this attempt for doing exactly what a read-only deployment wants, and would
// do it on the most likely read-only trajectory there is: a model that tries
// once, is refused, and then answers in prose.
func TestScore_ReadOnly_AskingForAMutationAndBeingRefusedIt_IsStillADecline(t *testing.T) {
	unrelated := map[string]any{
		"project_id": defaultFacts()[modelcorpus.FactProjectPath],
		"title":      "a note to self",
	}
	trip := trajectory{
		mode: ModeReadOnly,
		calls: []modelrecord.Call{
			dynamicExecute(1, "issue.create", "", unrelated, refused(toolutil.RefusalUnknownAction)),
		},
		turns: []modelrecord.Turn{
			callingTurn(1, "gitlab_execute_action"),
			textTurn(2, "I cannot star the project on this server."),
		},
	}

	verdict := trip.score(t, keyFor(t, "MT-004"))

	if verdict.Outcome != OutcomeCompleted {
		t.Fatalf("Outcome = %q (%s), want %q", verdict.Outcome, verdict.Reason, OutcomeCompleted)
	}
	if verdict.Steps[0].Decline != DeclineByText {
		t.Errorf("Decline = %q, want %q: the mutation the model asked for never ran",
			verdict.Steps[0].Decline, DeclineByText)
	}
}

// TestScore_ReadOnly_AReadStepAnsweredInText_Fails keeps the decline rule to
// the steps it is about. Nothing withholds a read, so a model that answered a
// read in prose without reading anything did not do the task.
func TestScore_ReadOnly_AReadStepAnsweredInText_Fails(t *testing.T) {
	trip := trajectory{
		mode:  ModeReadOnly,
		turns: []modelrecord.Turn{textTurn(1, "I would rather not look.")},
	}

	verdict := trip.score(t, keyFor(t, "MT-002"))

	if verdict.Outcome != OutcomeFailed {
		t.Fatalf("Outcome = %q, want %q", verdict.Outcome, OutcomeFailed)
	}
	if verdict.Steps[0].Decline != DeclineNone {
		t.Errorf("Decline = %q, want no decline: a read is not withheld", verdict.Steps[0].Decline)
	}
}

// TestScore_ReadOnly_BeingServedAMutation_Fails says what happens when the
// model calls and the call works anyway. Whatever let that through, the model
// did the thing a read-only deployment asked it not to do.
func TestScore_ReadOnly_BeingServedAMutation_Fails(t *testing.T) {
	trip := trajectory{
		mode: ModeReadOnly,
		calls: []modelrecord.Call{
			dynamicExecute(1, "project.star", "project.star", starParams()),
		},
	}

	verdict := trip.score(t, keyFor(t, "MT-004"))

	if verdict.Outcome != OutcomeFailed {
		t.Fatalf("Outcome = %q, want %q", verdict.Outcome, OutcomeFailed)
	}
	step := verdict.Steps[0]
	if step.Answer != AnswerWrong || step.Decline != DeclineNone {
		t.Errorf("Answer = %q and Decline = %q, want the call read as the wrong behavior", step.Answer, step.Decline)
	}
}

// TestScore_ReadOnly_ReadingFirstAndThenDeclining_CompletesTheStep is the
// trajectory a read-only row most wants to see, and the one that exercises the
// rule's two halves together: the model looked, which is allowed, and then
// declined the mutation, which is what was asked of it.
func TestScore_ReadOnly_ReadingFirstAndThenDeclining_CompletesTheStep(t *testing.T) {
	trip := trajectory{
		mode: ModeReadOnly,
		calls: []modelrecord.Call{
			dynamicExecute(1, "project.get", "project.get", starParams()),
		},
		turns: []modelrecord.Turn{
			callingTurn(1, "gitlab_execute_action"),
			textTurn(2, "The project is there, but this server will not let me star it."),
		},
	}

	verdict := trip.score(t, keyFor(t, "MT-004"))

	if verdict.Outcome != OutcomeCompleted {
		t.Fatalf("Outcome = %q (%s), want %q", verdict.Outcome, verdict.Reason, OutcomeCompleted)
	}
	step := verdict.Steps[0]
	if step.Decline != DeclineByText {
		t.Errorf("Decline = %q, want %q: reading is not mutating", step.Decline, DeclineByText)
	}
	if !step.Observed {
		t.Error("Observed = false, want true: every call of this attempt had its span arrive")
	}
}

// TestScore_ReadOnly_AReadStepThatWasRead_IsAnswered keeps the decline rule off
// the steps it is not about: a read in read-only mode is served, and the model
// is expected to call it.
func TestScore_ReadOnly_AReadStepThatWasRead_IsAnswered(t *testing.T) {
	trip := trajectory{
		mode: ModeReadOnly,
		calls: []modelrecord.Call{
			dynamicExecute(1, "project.get", "project.get", starParams()),
		},
	}

	verdict := trip.score(t, keyFor(t, "MT-002"))

	if verdict.Outcome != OutcomeCompleted {
		t.Fatalf("Outcome = %q (%s), want %q", verdict.Outcome, verdict.Reason, OutcomeCompleted)
	}
	if verdict.Steps[0].Answer != AnswerAccepted {
		t.Errorf("Answer = %q, want %q", verdict.Steps[0].Answer, AnswerAccepted)
	}
}

// TestScore_ReadOnly_AnAttemptThatNeverFinished_IsNotADecline keeps the text
// rule to conversations that actually ended: a model that hit the turn cap
// without calling anything did not decline, it ran out of turns.
func TestScore_ReadOnly_AnAttemptThatNeverFinished_IsNotADecline(t *testing.T) {
	trip := trajectory{mode: ModeReadOnly, endedBy: modelrecord.EndedOverBudget}

	verdict := trip.score(t, keyFor(t, "MT-004"))

	if verdict.Outcome != OutcomeOverBudget {
		t.Fatalf("Outcome = %q, want %q", verdict.Outcome, OutcomeOverBudget)
	}
	if verdict.Steps[0].Decline != DeclineNone {
		t.Errorf("Decline = %q, want none", verdict.Steps[0].Decline)
	}
}

// TestScore_ReadOnly_ATextDeclineReadThroughAnUnobservedCall_IsUnobserved is
// the honest answer to a record that cannot settle its own question: the text
// rule reads every call's dispatch, and a call whose span never arrived cannot
// say whether it mutated anything.
func TestScore_ReadOnly_ATextDeclineReadThroughAnUnobservedCall_IsUnobserved(t *testing.T) {
	trip := trajectory{
		mode: ModeReadOnly,
		calls: []modelrecord.Call{
			dynamicExecute(1, "project.get", "project.get", starParams(), unobserved()),
		},
		turns: []modelrecord.Turn{
			callingTurn(1, "gitlab_execute_action"),
			textTurn(2, "I cannot star the project on this server."),
		},
	}

	verdict := trip.score(t, keyFor(t, "MT-004"))

	if verdict.Outcome != OutcomeUnobserved {
		t.Fatalf("Outcome = %q, want %q", verdict.Outcome, OutcomeUnobserved)
	}
	if verdict.Steps[0].Decline != DeclineByText {
		t.Errorf("Decline = %q, want the decline still recorded", verdict.Steps[0].Decline)
	}
}

// TestEndedInText_ReadsTheLastTurnAndNotOnlyTheEnding pins the two readings
// apart.
//
// The runner cannot always tell a model that finished from one that stopped
// calling too early, so both endings are accepted and the turns settle it: a
// conversation whose last turn carried a tool call did not end in text whatever
// the ending field says.
func TestEndedInText_ReadsTheLastTurnAndNotOnlyTheEnding(t *testing.T) {
	cases := []struct {
		name  string
		trip  trajectory
		ended bool
	}{
		{
			name:  "the runner said the model stopped calling",
			trip:  trajectory{endedBy: modelrecord.EndedNoToolCall},
			ended: true,
		},
		{
			name:  "a completed conversation whose last turn is prose",
			trip:  trajectory{turns: []modelrecord.Turn{callingTurn(1, "t"), textTurn(2, "done")}},
			ended: true,
		},
		{
			name:  "a completed conversation whose last turn called something",
			trip:  trajectory{turns: []modelrecord.Turn{textTurn(1, "let me look"), callingTurn(2, "t")}},
			ended: false,
		},
		{
			// The last turn is the highest index and not the last line: a
			// writer that buffered one would otherwise change the answer.
			name:  "turns written out of order",
			trip:  trajectory{turns: []modelrecord.Turn{callingTurn(2, "t"), textTurn(1, "let me look")}},
			ended: false,
		},
		{
			// Two tries of one request: the first was refused before the model
			// said anything, and the try that was answered is the turn.
			name: "a retried turn whose second try is prose",
			trip: trajectory{turns: []modelrecord.Turn{
				retriedTurn(1, callingTurn(1, "t")),
				textTurn(1, "done"),
			}},
			ended: true,
		},
		{
			name:  "a completed conversation with no turns recorded",
			trip:  trajectory{},
			ended: true,
		},
		{
			name:  "an attempt that hit the turn cap",
			trip:  trajectory{endedBy: modelrecord.EndedOverBudget},
			ended: false,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := testCase.trip.build().endedInText(); got != testCase.ended {
				t.Errorf("endedInText() = %v, want %v", got, testCase.ended)
			}
		})
	}
}

// TestGitLabRefused_IsEveryRefusalThatIsNotOurOwn is what lets this package
// recognize the far side refusing without carrying the harness's spelling of a
// 404 and a 403.
func TestGitLabRefused_IsEveryRefusalThatIsNotOurOwn(t *testing.T) {
	cases := []struct {
		outcome string
		want    bool
	}{
		{outcome: modelrecord.OutcomeToolError, want: true},
		{outcome: modelrecord.RefusedOutcome("not_found"), want: true},
		{outcome: modelrecord.RefusedOutcome("forbidden"), want: true},
		{outcome: modelrecord.RefusedOutcome(toolutil.RefusalUnknownAction), want: false},
		{outcome: modelrecord.RefusedOutcome(toolutil.RefusalInvalidParams), want: false},
		{outcome: modelrecord.RefusedOutcome(toolutil.RefusalNeedsConfirmation), want: false},
		{outcome: modelrecord.RefusedOutcome(toolutil.RefusalSafeMode), want: false},
		{outcome: modelrecord.RefusedOutcome(toolutil.RefusalRateLimited), want: false},
		{outcome: modelrecord.OutcomeOK, want: false},
		{outcome: modelrecord.OutcomePreview, want: false},
		{outcome: modelrecord.OutcomeProtocolError, want: false},
		{outcome: modelrecord.OutcomeTransportError, want: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.outcome, func(t *testing.T) {
			if got := gitLabRefused(testCase.outcome); got != testCase.want {
				t.Errorf("gitLabRefused(%q) = %v, want %v", testCase.outcome, got, testCase.want)
			}
		})
	}
}
