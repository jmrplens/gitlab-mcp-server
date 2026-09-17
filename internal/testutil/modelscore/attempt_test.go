package modelscore

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelcorpus"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelrecord"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The scorer's own tests, and the builders every one of them is written with.
//
// Every record here is written by hand. That is the point of the package: a
// verdict is a function of lines and a key, so a rule can be exercised against
// a trajectory nobody has ever paid a provider to produce, including the ones a
// real run would produce only by accident.
//
// The keys are of two kinds, deliberately. A rule is tested against a key
// written here, so that a case being rewritten upstream cannot change what a
// rule is held to; the join with the real corpus is tested apart, against cases
// named by identifier, so that a key this package can no longer score is a
// failure here rather than a surprise in a paid run.

// testAttemptID is what every line of a built attempt names.
const testAttemptID = "attempt-1"

// defaultFacts are the values a recipe is taken to have produced. Both
// spellings of the project and the group are here because the scorer accepts
// either for the path facts, and a test that only ever offered one could not
// tell the acceptance from a comparison that never happened.
func defaultFacts() map[string]string {
	return map[string]string{
		modelcorpus.FactProjectPath: "eval-group/eval-project",
		modelcorpus.FactProjectID:   "42",
		modelcorpus.FactGroupPath:   "eval-group",
		modelcorpus.FactGroupID:     "7",
		modelcorpus.FactIssueIID:    "3",
	}
}

// trajectory is one attempt under construction: the session shape it ran in
// and the lines it wrote.
type trajectory struct {
	surface  string
	mode     string
	tier     string
	endedBy  string
	reason   string
	facts    map[string]string
	calls    []modelrecord.Call
	turns    []modelrecord.Turn
	verifies []modelrecord.Verify
}

// build assembles the attempt, filling in the defaults a case that says
// nothing about the session means: the default surface, neither protection on,
// a licensed instance so that every action of the corpus exists, and a
// conversation the model ended itself.
func (tr trajectory) build() Attempt {
	surface := tr.surface
	if surface == "" {
		surface = string(modelcorpus.SurfaceDynamic)
	}
	mode := tr.mode
	if mode == "" {
		mode = ModeDefault
	}
	tier := tr.tier
	if tier == "" {
		tier = string(modelcorpus.TierUltimate)
	}
	ended := tr.endedBy
	if ended == "" {
		ended = modelrecord.EndedCompleted
	}
	facts := tr.facts
	if facts == nil {
		facts = defaultFacts()
	}
	return Attempt{
		Run:     modelrecord.Run{RunID: "run-1", Tier: tier},
		Session: modelrecord.Session{Label: "session-1", Surface: surface, Mode: mode},
		Line: modelrecord.Attempt{
			ID:       testAttemptID,
			Case:     "CASE-1",
			Model:    "fake:test",
			Surface:  surface,
			Session:  "session-1",
			Repeat:   1,
			Facts:    facts,
			EndedBy:  ended,
			Reason:   tr.reason,
			Stimulus: "the stimulus as sent",
		},
		Turns:    tr.turns,
		Calls:    tr.calls,
		Verifies: tr.verifies,
	}
}

// score builds the attempt and scores it, failing the test on a record this
// tree cannot read at all.
func (tr trajectory) score(t *testing.T, key modelcorpus.Key) Verdict {
	t.Helper()
	verdict, err := Score(tr.build(), key)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	return verdict
}

// callOption changes one built call.
type callOption func(*modelrecord.Call)

// refused makes the call one this server declined, for its own reason. The
// dispatch is cleared with it, because every refusal this server makes happens
// before the route is recorded.
func refused(reason string) callOption {
	return func(call *modelrecord.Call) {
		call.Outcome = modelrecord.RefusedOutcome(reason)
		call.RefusalReason = reason
		call.DispatchedAction = ""
	}
}

// refusedKeepingDispatch makes the call a refusal that the span still names an
// action for, which is how a confirmation refusal is recorded: the route was
// chosen and then the handler asked for the approval.
func refusedKeepingDispatch(reason string) callOption {
	return func(call *modelrecord.Call) {
		call.Outcome = modelrecord.RefusedOutcome(reason)
		call.RefusalReason = reason
	}
}

// withheldOnMeta makes the call what a withheld action looks like on the meta
// surface: the SDK's schema validation refuses the action enum before any
// handler of ours runs, so the span carries no reason at all.
func withheldOnMeta() callOption {
	return func(call *modelrecord.Call) {
		call.Outcome = modelrecord.RefusedOutcome(toolutil.RefusalInvalidParams)
		call.RefusalReason = ""
		call.DispatchedAction = ""
		call.Text = `jsonschema validating "arguments": at /properties/action: value must be one of ...`
	}
}

// protocolError makes the call a JSON-RPC error, which is what naming a tool
// the surface does not register looks like.
func protocolError() callOption {
	return func(call *modelrecord.Call) {
		call.Outcome = modelrecord.OutcomeProtocolError
		call.DispatchedAction = ""
		call.Text = "tool \"" + call.Tool + "\" not found"
	}
}

// answered replaces the call's outcome, for the classes that have no shorthand.
func answered(outcome string) callOption {
	return func(call *modelrecord.Call) { call.Outcome = outcome }
}

// resulting attaches the structured content the call answered with, which is
// what a later step's Produced argument is read from.
func resulting(content any) callOption {
	return func(call *modelrecord.Call) { call.Result = mustJSON(content) }
}

// truncated marks the recorded answer as cut off at the record's cap.
func truncated() callOption {
	return func(call *modelrecord.Call) { call.ResultTruncated = true }
}

// unobserved makes the call one whose span never arrived, which is the state
// every verdict derived from it is reported in.
func unobserved() callOption {
	return func(call *modelrecord.Call) {
		call.DispatchObserved = false
		call.DispatchedAction = ""
		call.DispatchedTool = ""
	}
}

// callOf builds one recorded call.
func callOf(index int, tool string, args map[string]any, opts ...callOption) modelrecord.Call {
	call := modelrecord.Call{
		Attempt:          testAttemptID,
		Turn:             1,
		Index:            index,
		Tool:             tool,
		Arguments:        mustJSON(args),
		DispatchedTool:   tool,
		Outcome:          modelrecord.OutcomeOK,
		DispatchObserved: true,
	}
	for _, opt := range opts {
		opt(&call)
	}
	return call
}

// dynamicFind is a catalog search on the dynamic surface.
func dynamicFind(index int, query string, opts ...callOption) modelrecord.Call {
	return callOf(index, dynamictools.FindActionToolName, map[string]any{"query": query}, opts...)
}

// dynamicExecute is one action run through the dynamic execute tool. ran names
// the action the server dispatched, which is the same as the one the call
// carries except where an alias is rewritten.
func dynamicExecute(index int, named, ran string, params map[string]any, opts ...callOption) modelrecord.Call {
	args := map[string]any{"action": named, "params": params}
	return callOf(index, dynamictools.ExecuteActionToolName, args,
		append([]callOption{func(call *modelrecord.Call) { call.DispatchedAction = ran }}, opts...)...)
}

// dynamicConfirm is dynamicExecute with the approval at the position the
// dynamic surface reads one: top level, beside the action and its params.
func dynamicConfirm(index int, action string, params map[string]any, opts ...callOption) modelrecord.Call {
	args := map[string]any{"action": action, "params": params, "confirm": true}
	return callOf(index, dynamictools.ExecuteActionToolName, args,
		append([]callOption{func(call *modelrecord.Call) { call.DispatchedAction = action }}, opts...)...)
}

// metaExecute is one action run through a meta group tool. The canonical
// identifier is passed whole and the action argument is the half after the dot,
// which is how the meta surface spells the same call.
func metaExecute(index int, tool, canonical string, params map[string]any, opts ...callOption) modelrecord.Call {
	args := map[string]any{"action": actionName(canonical), "params": params}
	return callOf(index, tool, args,
		append([]callOption{func(call *modelrecord.Call) { call.DispatchedAction = canonical }}, opts...)...)
}

// individualCall is one action run through its own registered tool, where the
// arguments sit at the top level and the span's action is the middleware's
// reading of the tool name.
func individualCall(index int, tool, canonical string, args map[string]any, opts ...callOption) modelrecord.Call {
	return callOf(index, tool, args,
		append([]callOption{func(call *modelrecord.Call) { call.DispatchedAction = canonical }}, opts...)...)
}

// actionName is the half of a canonical identifier a meta tool takes.
func actionName(canonical string) string {
	for at := len(canonical) - 1; at >= 0; at-- {
		if canonical[at] == '.' {
			return canonical[at+1:]
		}
	}
	return canonical
}

// textTurn is a turn in which the model wrote prose and called nothing, which
// is how an attempt ends in text.
func textTurn(index int, text string) modelrecord.Turn {
	return modelrecord.Turn{
		Attempt: testAttemptID,
		Index:   index,
		Try:     1,
		Blocks:  []modelrecord.Block{{Kind: modelrecord.BlockText, Text: text}},
		Status:  modelrecord.TurnOK,
	}
}

// callingTurn is a turn in which the model made a call.
func callingTurn(index int, tool string) modelrecord.Turn {
	return modelrecord.Turn{
		Attempt: testAttemptID,
		Index:   index,
		Try:     1,
		Blocks:  []modelrecord.Block{{Kind: modelrecord.BlockToolCall, Tool: tool}},
		Status:  modelrecord.TurnOK,
	}
}

// retriedTurn marks a turn as an earlier try of its index, which is what a
// request the provider refused for rate looks like beside the one it answered.
func retriedTurn(try int, turn modelrecord.Turn) modelrecord.Turn {
	turn.Try = try
	turn.Status = modelrecord.TurnRateLimited
	return turn
}

// mustJSON encodes a fixture value, failing the process rather than the test on
// a value that cannot be encoded: every value here is a literal in this file,
// so a failure is a typo and not a condition worth carrying an error for.
func mustJSON(value any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return encoded
}

// keyFor returns one real corpus case's key, so the join between the two
// packages is exercised rather than assumed.
func keyFor(t *testing.T, id string) modelcorpus.Key {
	t.Helper()
	key, found := modelcorpus.Keys()[id]
	if !found {
		t.Fatalf("the corpus has no case %s, which this test is written against", id)
	}
	return key
}

// projectParams are the arguments of a step that works in the fixture project.
func projectParams() map[string]any {
	return map[string]any{"project_id": defaultFacts()[modelcorpus.FactProjectPath]}
}

// oneStep is a key of a single step with the arguments given.
func oneStep(action modelcorpus.Action, args ...modelcorpus.Arg) modelcorpus.Key {
	return modelcorpus.Key{Steps: []modelcorpus.Step{{Action: action, Args: args}}}
}

// requiredFact is one argument bound to a recipe fact.
func requiredFact(name, fact string) modelcorpus.Arg {
	return modelcorpus.Arg{Name: name, Truth: modelcorpus.Truth{Fact: fact}, Required: true}
}

// TestScore_DynamicTrajectoryWithAFind_CountsTheSearchApartFromTheStep is the
// fix for the defect that made the old dynamic column meaningless.
//
// There, the key carried a find step, so the discovery call was matched as
// though it were a catalog call and the column that counted steps counted it.
// Here the key names the action and nothing else, the find is read from the
// server's own span, and it lands in the overhead column where the cost of the
// dynamic surface belongs.
func TestScore_DynamicTrajectoryWithAFind_CountsTheSearchApartFromTheStep(t *testing.T) {
	trip := trajectory{calls: []modelrecord.Call{
		dynamicFind(1, "get a project"),
		dynamicExecute(2, "project.get", "project.get", projectParams()),
	}}

	verdict := trip.score(t, keyFor(t, "MT-002"))

	if verdict.Outcome != OutcomeCompleted {
		t.Fatalf("Outcome = %q (%s), want %q", verdict.Outcome, verdict.Reason, OutcomeCompleted)
	}
	if verdict.Discovery != 1 {
		t.Errorf("Discovery = %d, want 1", verdict.Discovery)
	}
	if len(verdict.Steps) != 1 {
		t.Fatalf("Steps = %d, want the one step the case declares", len(verdict.Steps))
	}
	step := verdict.Steps[0]
	if !step.Reached || step.MatchedBy != MatchDispatched {
		t.Errorf("step reached = %v by %q, want it reached by the server's dispatch", step.Reached, step.MatchedBy)
	}
	if step.Call != 2 {
		t.Errorf("reaching call = %d, want call 2: the find is not a step", step.Call)
	}
	if !step.AcceptedFirstTime {
		t.Error("AcceptedFirstTime = false, want true: the first call about the step was the one that ran it")
	}
}

// TestScoreCase_ResolvesTheKeyForACallerThatMayNotReadOne covers the entry
// point a run scores an attempt through.
//
// The runner may not read an answer, and a run is meant to say what each
// attempt came to as it ends. Both hold only if the lookup happens on this
// side, so what is checked here is that it happens and that it resolves the
// same key a caller holding one would have passed.
func TestScoreCase_ResolvesTheKeyForACallerThatMayNotReadOne(t *testing.T) {
	trip := trajectory{calls: []modelrecord.Call{
		dynamicFind(1, "get a project"),
		dynamicExecute(2, "project.get", "project.get", projectParams()),
	}}
	attempt := trip.build()
	attempt.Line.Case = "MT-002"

	resolved, err := ScoreCase(attempt)
	if err != nil {
		t.Fatalf("ScoreCase error = %v, want nil", err)
	}
	given, err := Score(attempt, keyFor(t, "MT-002"))
	if err != nil {
		t.Fatalf("Score error = %v, want nil", err)
	}
	if resolved.Outcome != OutcomeCompleted {
		t.Errorf("Outcome = %q (%s), want %q", resolved.Outcome, resolved.Reason, OutcomeCompleted)
	}
	if resolved.Outcome != given.Outcome || len(resolved.Steps) != len(given.Steps) ||
		resolved.Discovery != given.Discovery {
		t.Errorf("ScoreCase = %+v, want the verdict Score gives for the corpus's own key %+v", resolved, given)
	}

	unknown := attempt
	unknown.Line.Case = "MT-nothing-has-this-id"
	verdict, refused := ScoreCase(unknown)
	if refused == nil {
		t.Fatalf("ScoreCase of a case the corpus does not have = %+v, want an error", verdict)
	}
	if !strings.Contains(refused.Error(), unknown.Line.Case) {
		t.Errorf("the refusal is %v, want it to name the case it could not find", refused)
	}
}

// TestScore_DynamicTrajectoryWithNoFind_ReachesTheStepWithNoOverhead is the
// other half of the same fix: a model that knows the action and calls it
// directly pays nothing, where the old key would have reported a missing step.
func TestScore_DynamicTrajectoryWithNoFind_ReachesTheStepWithNoOverhead(t *testing.T) {
	trip := trajectory{calls: []modelrecord.Call{
		dynamicExecute(1, "project.get", "project.get", projectParams()),
	}}

	verdict := trip.score(t, keyFor(t, "MT-002"))

	if verdict.Outcome != OutcomeCompleted {
		t.Fatalf("Outcome = %q (%s), want %q", verdict.Outcome, verdict.Reason, OutcomeCompleted)
	}
	if verdict.Discovery != 0 {
		t.Errorf("Discovery = %d, want 0", verdict.Discovery)
	}
	step := verdict.Steps[0]
	if !step.Reached {
		t.Error("the step was not reached by a direct call that ran it")
	}
	if step.Rewritten {
		t.Error("Rewritten = true, want false: the model named the action the server ran")
	}
	if !step.Complete || step.Reason != "" {
		t.Errorf("complete=%v reason=%q, want a step that went the way the case asks and says nothing",
			step.Complete, step.Reason)
	}
}

// TestScore_AnAliasRewrite_KeepsTheRequestAndTheDispatchApart pins the reading
// the whole matching rule exists for.
//
// gitlab_environment with action get and an environment name runs
// protected_get. A scorer reading the request would credit environment.get,
// which never ran; one reading only the dispatch would lose the fact that the
// model named something else. Both are recorded, and the step is reached by the
// action that ran.
func TestScore_AnAliasRewrite_KeepsTheRequestAndTheDispatchApart(t *testing.T) {
	params := map[string]any{
		"project_id":  defaultFacts()[modelcorpus.FactProjectPath],
		"environment": "production",
	}
	trip := trajectory{calls: []modelrecord.Call{
		dynamicExecute(1, "environment.get", "environment.protected_get", params),
	}}

	t.Run("the key names the action that ran", func(t *testing.T) {
		verdict := trip.score(t, oneStep("environment.protected_get",
			requiredFact("project_id", modelcorpus.FactProjectPath)))

		step := verdict.Steps[0]
		if !step.Reached {
			t.Fatalf("the step was not reached: %s", step.Reason)
		}
		if step.Requested != "environment.get" || step.Dispatched != "environment.protected_get" {
			t.Errorf("requested %q and dispatched %q, want the two read apart",
				step.Requested, step.Dispatched)
		}
		if !step.Rewritten {
			t.Error("Rewritten = false, want true: the dispatcher ran an action the model did not name")
		}
	})

	t.Run("the key names the action the model asked for", func(t *testing.T) {
		verdict := trip.score(t, oneStep("environment.get",
			requiredFact("project_id", modelcorpus.FactProjectPath)))

		if verdict.Steps[0].Reached {
			t.Error("the step was reached by a call that ran another action")
		}
		if verdict.Outcome != OutcomeFailed {
			t.Errorf("Outcome = %q, want %q", verdict.Outcome, OutcomeFailed)
		}
		// The span arrived and named another action, so "nothing named this
		// step" is a reading of what ran. The same trajectory with no span is
		// the unobserved test below, and the two differ only in that.
		if !verdict.Steps[0].Observed {
			t.Error("Observed = false, want true: every call of this attempt had its span arrive")
		}
	})
}

// TestScore_RightArgumentNameAndWrongValue_IsAMiss is the defect the old
// argument column could not see: it checked that the parameter was present.
func TestScore_RightArgumentNameAndWrongValue_IsAMiss(t *testing.T) {
	wrong := map[string]any{"project_id": "somebody-else/their-project"}
	trip := trajectory{calls: []modelrecord.Call{
		dynamicExecute(1, "project.get", "project.get", wrong),
	}}

	verdict := trip.score(t, keyFor(t, "MT-002"))

	if verdict.Outcome != OutcomeFailed {
		t.Fatalf("Outcome = %q, want %q", verdict.Outcome, OutcomeFailed)
	}
	step := verdict.Steps[0]
	if !step.Reached {
		t.Error("Reached = false: the call did run the action, and the value is what was wrong")
	}
	argument := step.Arguments[0]
	if !argument.Compared || argument.Matched {
		t.Errorf("argument %s compared=%v matched=%v, want compared and not matched",
			argument.Name, argument.Compared, argument.Matched)
	}
	if argument.Sent != "somebody-else/their-project" {
		t.Errorf("Sent = %q, want what the model actually sent", argument.Sent)
	}
	// Nothing of ours refused this attempt anything, so the only thing keeping
	// it out of the unaided column is that it did not complete. Unaided is
	// published as a completion, so an attempt that failed unaided is a
	// contradiction a reader would take for a success.
	if verdict.Unaided {
		t.Error("Unaided = true for an attempt that did not complete: the column counts completions")
	}
}

// TestScore_AnAuthoredValue_IsReportedAndNeverCompared keeps the one truth that
// has no right answer out of the fidelity column.
func TestScore_AnAuthoredValue_IsReportedAndNeverCompared(t *testing.T) {
	params := map[string]any{
		"project_id": defaultFacts()[modelcorpus.FactProjectPath],
		"title":      "whatever the model decided to call it",
	}
	key := oneStep("issue.create",
		requiredFact("project_id", modelcorpus.FactProjectPath),
		modelcorpus.Arg{Name: "title", Truth: modelcorpus.Truth{Authored: true}, Required: true},
	)
	trip := trajectory{calls: []modelrecord.Call{
		dynamicExecute(1, "issue.create", "issue.create", params),
	}}

	verdict := trip.score(t, key)

	if verdict.Outcome != OutcomeCompleted {
		t.Fatalf("Outcome = %q (%s), want %q", verdict.Outcome, verdict.Reason, OutcomeCompleted)
	}
	title := verdict.Steps[0].Arguments[1]
	if title.Truth != TruthAuthored {
		t.Fatalf("Truth = %q, want %q", title.Truth, TruthAuthored)
	}
	if title.Compared {
		t.Error("Compared = true, want false: a value the model writes has no right answer")
	}
	if title.Sent == "" {
		t.Error("Sent is empty, want the authored value reported")
	}
}

// TestScore_ADestructiveStep_ClassifiesHowTheConfirmationWasLearned is the
// measurement the server's own refusal text makes possible.
//
// It is also why nothing has to be exported from the server for this: the
// refusal the model read is the server's own, because the call went to the
// server.
func TestScore_ADestructiveStep_ClassifiesHowTheConfirmationWasLearned(t *testing.T) {
	key := keyFor(t, "MT-013")
	params := map[string]any{
		"project_id": defaultFacts()[modelcorpus.FactProjectPath],
		"issue_iid":  defaultFacts()[modelcorpus.FactIssueIID],
	}

	cases := []struct {
		name    string
		calls   []modelrecord.Call
		want    Confirmation
		first   bool
		unaided bool
	}{
		{
			name:    "confirmed on the first call",
			calls:   []modelrecord.Call{dynamicConfirm(1, "issue.delete", params)},
			want:    ConfirmationUnaided,
			first:   true,
			unaided: true,
		},
		{
			name: "refused and then confirmed",
			calls: []modelrecord.Call{
				dynamicExecute(1, "issue.delete", "issue.delete", params,
					refusedKeepingDispatch(toolutil.RefusalNeedsConfirmation)),
				dynamicConfirm(2, "issue.delete", params),
			},
			want:  ConfirmationServerAided,
			first: false,
		},
		{
			name: "never confirmed",
			calls: []modelrecord.Call{
				dynamicExecute(1, "issue.delete", "issue.delete", params,
					refusedKeepingDispatch(toolutil.RefusalNeedsConfirmation)),
			},
			want: ConfirmationNever,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			verdict := trajectory{calls: testCase.calls}.score(t, key)

			step := verdict.Steps[0]
			if step.Confirmation != testCase.want {
				t.Errorf("Confirmation = %q, want %q", step.Confirmation, testCase.want)
			}
			if step.AcceptedFirstTime != testCase.first {
				t.Errorf("AcceptedFirstTime = %v, want %v", step.AcceptedFirstTime, testCase.first)
			}
			if verdict.Unaided != testCase.unaided {
				t.Errorf("Unaided = %v, want %v", verdict.Unaided, testCase.unaided)
			}
		})
	}
}

// TestScore_ADestructiveStepNeverConfirmed_IsNotReached says what a step whose
// every call was sent back looks like, and why the reason has to say so.
//
// A model that asked three times and never approved did name the step, so "no
// call named this step" would be false; what happened is that the server asked
// for the call again each time and the model never sent one it would run.
func TestScore_ADestructiveStepNeverConfirmed_IsNotReached(t *testing.T) {
	params := map[string]any{
		"project_id": defaultFacts()[modelcorpus.FactProjectPath],
		"issue_iid":  defaultFacts()[modelcorpus.FactIssueIID],
	}
	trip := trajectory{calls: []modelrecord.Call{
		dynamicExecute(1, "issue.delete", "issue.delete", params,
			refusedKeepingDispatch(toolutil.RefusalNeedsConfirmation)),
		dynamicExecute(2, "issue.delete", "issue.delete", params,
			refusedKeepingDispatch(toolutil.RefusalNeedsConfirmation)),
	}}

	verdict := trip.score(t, keyFor(t, "MT-013"))

	step := verdict.Steps[0]
	if step.Reached {
		t.Fatal("Reached = true, want false: every call about the step was sent back")
	}
	if step.Retries != 2 {
		t.Errorf("Retries = %d, want 2", step.Retries)
	}
	if verdict.Outcome != OutcomeFailed {
		t.Errorf("Outcome = %q, want %q", verdict.Outcome, OutcomeFailed)
	}
	if step.Reason == "" || step.Reason == "no call named this step" {
		t.Errorf("Reason = %q, want it to say the calls were refused and re-asked", step.Reason)
	}
}

// TestScore_GitLabRefusingAfterACorrectDispatch_IsItsOwnOutcome keeps the
// instance's answer out of the model's column.
func TestScore_GitLabRefusingAfterACorrectDispatch_IsItsOwnOutcome(t *testing.T) {
	trip := trajectory{calls: []modelrecord.Call{
		dynamicExecute(1, "project.get", "project.get", projectParams(),
			answered(modelrecord.OutcomeToolError)),
	}}

	verdict := trip.score(t, keyFor(t, "MT-002"))

	if verdict.Outcome != OutcomeGitLabRefused {
		t.Fatalf("Outcome = %q, want %q", verdict.Outcome, OutcomeGitLabRefused)
	}
	step := verdict.Steps[0]
	if step.Answer != AnswerGitLabRefused {
		t.Errorf("Answer = %q, want %q", step.Answer, AnswerGitLabRefused)
	}
	if !strings.Contains(step.Reason, "GitLab refused") {
		t.Errorf("Reason = %q, want it to say the action ran and the far side refused it", step.Reason)
	}
}

// projectThenIssues is a two-step key of the shape most cases have: read the
// project, then read something inside it. The second step is what the first
// step's answer makes possible, which is what the refusal rule is about.
func projectThenIssues() modelcorpus.Key {
	return modelcorpus.Key{Steps: []modelcorpus.Step{
		{Action: "project.get", Args: []modelcorpus.Arg{
			requiredFact("project_id", modelcorpus.FactProjectPath),
		}},
		{Action: "issue.list", Args: []modelcorpus.Arg{
			requiredFact("project_id", modelcorpus.FactProjectPath),
		}},
	}}
}

// TestScore_GitLabRefusingAFirstStep_DoesNotChargeTheStepsAfterIt is the other
// half of keeping the instance out of the model's column.
//
// A case's steps are a sequence: a model that asked for the project and was
// answered with an error has nothing to look the issues up by, so the step
// after it is unreached for a reason that is not the model's. Reading the steps
// one at a time reports that second step as "no call named this step" and fails
// the attempt, which moves the instance's fixture from the class built to hold
// it into the completion column.
func TestScore_GitLabRefusingAFirstStep_DoesNotChargeTheStepsAfterIt(t *testing.T) {
	trip := trajectory{calls: []modelrecord.Call{
		dynamicExecute(1, "project.get", "project.get", projectParams(),
			answered(modelrecord.OutcomeToolError)),
	}}

	verdict := trip.score(t, projectThenIssues())

	if verdict.Outcome != OutcomeGitLabRefused {
		t.Fatalf("Outcome = %q (%s), want %q", verdict.Outcome, verdict.Reason, OutcomeGitLabRefused)
	}
	if !strings.Contains(verdict.Reason, "step 1") || !strings.Contains(verdict.Reason, "step 2") {
		t.Errorf("Reason = %q, want it to name the step GitLab refused and the one that went unreached",
			verdict.Reason)
	}
	if verdict.Steps[0].Answer != AnswerGitLabRefused {
		t.Errorf("Answer = %q, want %q", verdict.Steps[0].Answer, AnswerGitLabRefused)
	}
	if verdict.Steps[1].Reached {
		t.Error("the second step was reached, which is not the trajectory this test is about")
	}
}

// TestScore_AStepReachedAndWrongAfterARefusal_IsStillAFailure is the boundary of
// the rule above: a refusal excuses the steps it made impossible, and nothing
// else. A model that did call the next action and called it wrongly could have
// called it rightly.
func TestScore_AStepReachedAndWrongAfterARefusal_IsStillAFailure(t *testing.T) {
	trip := trajectory{calls: []modelrecord.Call{
		dynamicExecute(1, "project.get", "project.get", projectParams(),
			answered(modelrecord.OutcomeToolError)),
		dynamicExecute(2, "issue.list", "issue.list", map[string]any{"project_id": "someone/else"}),
	}}

	verdict := trip.score(t, projectThenIssues())

	if verdict.Outcome != OutcomeFailed {
		t.Fatalf("Outcome = %q (%s), want %q", verdict.Outcome, verdict.Reason, OutcomeFailed)
	}
	if !strings.Contains(verdict.Reason, "step 2") {
		t.Errorf("Reason = %q, want it to name the step the model reached and got wrong", verdict.Reason)
	}
}

// TestScore_AnUnreachedStepWithAnUnobservedCallInTheWindow_IsUnobserved is the
// claim this package refuses to make about a call the server's span never
// described.
//
// The trajectory is the alias case with the span dropped. gitlab_environment
// with action get runs protected_get, so a key naming protected_get is reached
// by that call and by nothing the model typed. With no span there is no
// dispatch to read, the request names environment.get, and the step reads as
// one nothing named. That verdict is derived from the missing dispatch exactly
// as a reached one would be, so it is unobserved rather than a failure.
func TestScore_AnUnreachedStepWithAnUnobservedCallInTheWindow_IsUnobserved(t *testing.T) {
	params := map[string]any{
		"project_id":  defaultFacts()[modelcorpus.FactProjectPath],
		"environment": "production",
	}
	trip := trajectory{calls: []modelrecord.Call{
		dynamicFind(1, "read an environment"),
		dynamicExecute(2, "environment.get", "environment.protected_get", params, unobserved()),
	}}

	verdict := trip.score(t, oneStep("environment.protected_get",
		requiredFact("project_id", modelcorpus.FactProjectPath)))

	if verdict.Outcome != OutcomeUnobserved {
		t.Fatalf("Outcome = %q (%s), want %q", verdict.Outcome, verdict.Reason, OutcomeUnobserved)
	}
	step := verdict.Steps[0]
	if step.Reached {
		t.Error("Reached = true, want false: nothing the record holds names this step")
	}
	if step.Observed {
		t.Error("Observed = true, want false: the dispatch that never arrived might have named the step")
	}
}

// TestScore_AnUnreachedStepWithOnlyASearchUnobserved_IsStillAFailure keeps that
// rule off the one call that could not have named a step whatever its span
// said. The find tool dispatches no action, so a model that searched and then
// did nothing missed the step, and treating the missing span as a doubt would
// leave every model that searched once unable to be told it missed anything.
func TestScore_AnUnreachedStepWithOnlyASearchUnobserved_IsStillAFailure(t *testing.T) {
	trip := trajectory{calls: []modelrecord.Call{
		dynamicFind(1, "get a project", unobserved()),
	}}

	verdict := trip.score(t, keyFor(t, "MT-002"))

	if verdict.Outcome != OutcomeFailed {
		t.Fatalf("Outcome = %q (%s), want %q", verdict.Outcome, verdict.Reason, OutcomeFailed)
	}
	if !verdict.Steps[0].Observed {
		t.Error("Observed = false, want true: a search names no action, so no span of one could have")
	}
}

// TestScore_SafeMode_ExpectsThePreviewAndNotTheAnswer is the whole of safe-mode
// scoring: the server answers a mutation with a preview of it, so the preview
// is the success and an ordinary answer means the mutation happened.
func TestScore_SafeMode_ExpectsThePreviewAndNotTheAnswer(t *testing.T) {
	key := keyFor(t, "MT-004")

	cases := []struct {
		name    string
		outcome string
		want    Outcome
		reason  string
	}{
		{name: "a preview completes", outcome: modelrecord.OutcomePreview, want: OutcomeCompleted},
		{
			name:    "an ordinary answer does not",
			outcome: modelrecord.OutcomeOK,
			want:    OutcomeFailed,
			reason:  "not what this mode expects",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			trip := trajectory{mode: ModeSafe, calls: []modelrecord.Call{
				dynamicExecute(1, "project.star", "project.star", projectParams(),
					answered(testCase.outcome)),
			}}

			verdict := trip.score(t, key)

			if verdict.Outcome != testCase.want {
				t.Fatalf("Outcome = %q (%s), want %q", verdict.Outcome, verdict.Reason, testCase.want)
			}
			if testCase.reason != "" && !strings.Contains(verdict.Steps[0].Reason, testCase.reason) {
				t.Errorf("Reason = %q, want it to name the mode's expectation", verdict.Steps[0].Reason)
			}
		})
	}
}

// TestScore_SafeModeReadStep_ExpectsTheOrdinaryAnswer keeps the preview rule to
// the steps it is about. Safe mode changes nothing for a read, so a read
// answered normally still completes.
func TestScore_SafeModeReadStep_ExpectsTheOrdinaryAnswer(t *testing.T) {
	trip := trajectory{mode: ModeSafe, calls: []modelrecord.Call{
		dynamicExecute(1, "project.get", "project.get", projectParams()),
	}}

	verdict := trip.score(t, keyFor(t, "MT-002"))

	if verdict.Outcome != OutcomeCompleted {
		t.Errorf("Outcome = %q (%s), want %q", verdict.Outcome, verdict.Reason, OutcomeCompleted)
	}
}

// TestScore_TheEndingsTheRunnerRecorded_AreTheVerdict says that an attempt the
// conversation never finished is judged by how it stopped and not by the steps
// it happened to reach on the way.
func TestScore_TheEndingsTheRunnerRecorded_AreTheVerdict(t *testing.T) {
	cases := []struct {
		name  string
		ended string
		want  Outcome
	}{
		{name: "the turn cap", ended: modelrecord.EndedOverBudget, want: OutcomeOverBudget},
		{name: "an unparseable call", ended: modelrecord.EndedMalformed, want: OutcomeMalformed},
		{name: "the provider", ended: modelrecord.EndedProviderError, want: OutcomeProviderError},
		{name: "this side", ended: modelrecord.EndedHarnessError, want: OutcomeHarnessError},
		{name: "a need the instance did not meet", ended: modelrecord.EndedSkipped, want: OutcomeSkipped},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			trip := trajectory{
				endedBy: testCase.ended,
				reason:  "the reason the runner recorded",
				calls: []modelrecord.Call{
					dynamicExecute(1, "project.get", "project.get", projectParams()),
				},
			}

			verdict := trip.score(t, keyFor(t, "MT-002"))

			if verdict.Outcome != testCase.want {
				t.Errorf("Outcome = %q, want %q", verdict.Outcome, testCase.want)
			}
			if verdict.Reason != "the reason the runner recorded" {
				t.Errorf("Reason = %q, want the runner's own", verdict.Reason)
			}
			if !verdict.Steps[0].Reached {
				t.Error("the step it did reach was not recorded as reached")
			}
		})
	}
}

// TestScore_ACallWhoseSpanNeverArrived_IsUnobserved is the rule that keeps a
// claim about a request from being published as a claim about a dispatch.
func TestScore_ACallWhoseSpanNeverArrived_IsUnobserved(t *testing.T) {
	trip := trajectory{calls: []modelrecord.Call{
		dynamicExecute(1, "project.get", "project.get", projectParams(), unobserved()),
	}}

	verdict := trip.score(t, keyFor(t, "MT-002"))

	if verdict.Outcome != OutcomeUnobserved {
		t.Fatalf("Outcome = %q, want %q", verdict.Outcome, OutcomeUnobserved)
	}
	step := verdict.Steps[0]
	if step.Observed {
		t.Error("Observed = true, want false")
	}
	if !step.Reached || step.MatchedBy != MatchRequested {
		t.Errorf("reached = %v by %q, want it reached by what the model requested, which is all there is",
			step.Reached, step.MatchedBy)
	}
	if step.Rewritten {
		t.Error("Rewritten = true, want false: there is no dispatch to have rewritten anything")
	}
}

// TestScore_TheRecipeCheck_IsTheHalfNoToolCallCanGive: the right action with
// the right arguments can still have left GitLab in the wrong state, and the
// recipe that built the world is what knows how to look.
func TestScore_TheRecipeCheck_IsTheHalfNoToolCallCanGive(t *testing.T) {
	cases := []struct {
		name   string
		passed bool
		want   Outcome
	}{
		{name: "the check held", passed: true, want: OutcomeCompleted},
		{name: "the check did not", passed: false, want: OutcomeFailed},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			trip := trajectory{
				calls: []modelrecord.Call{
					dynamicExecute(1, "project.star", "project.star", projectParams()),
				},
				verifies: []modelrecord.Verify{{
					Attempt: testAttemptID,
					Name:    "the project is starred",
					Passed:  testCase.passed,
					Detail:  "what GitLab held afterwards",
				}},
			}

			verdict := trip.score(t, keyFor(t, "MT-004"))

			if verdict.Outcome != testCase.want {
				t.Errorf("Outcome = %q (%s), want %q", verdict.Outcome, verdict.Reason, testCase.want)
			}
			if verdict.Verified != testCase.passed {
				t.Errorf("Verified = %v, want %v", verdict.Verified, testCase.passed)
			}
		})
	}
}

// TestScore_AnOptionalStepNothingReached_DoesNotFailTheAttempt covers the
// declaration a key makes when a step is allowed to be missing.
func TestScore_AnOptionalStepNothingReached_DoesNotFailTheAttempt(t *testing.T) {
	key := modelcorpus.Key{Steps: []modelcorpus.Step{
		{Action: "project.get", Args: []modelcorpus.Arg{
			requiredFact("project_id", modelcorpus.FactProjectPath),
		}},
		{Action: "project.members", Optional: true},
	}}
	trip := trajectory{calls: []modelrecord.Call{
		dynamicExecute(1, "project.get", "project.get", projectParams()),
	}}

	verdict := trip.score(t, key)

	if verdict.Outcome != OutcomeCompleted {
		t.Fatalf("Outcome = %q (%s), want %q", verdict.Outcome, verdict.Reason, OutcomeCompleted)
	}
	optional := verdict.Steps[1]
	if optional.Reached || !optional.Complete {
		t.Errorf("the optional step reached=%v complete=%v, want it missing and not held against the attempt",
			optional.Reached, optional.Complete)
	}
	if optional.Reason != "" {
		t.Errorf("Reason = %q, want none: nothing was owed", optional.Reason)
	}
}

// TestScore_AFailingStepWithAnAuthoredArgument_NamesOnlyWhatWasCompared keeps
// the value the model was free to invent out of the reason a step failed.
func TestScore_AFailingStepWithAnAuthoredArgument_NamesOnlyWhatWasCompared(t *testing.T) {
	params := map[string]any{
		"project_id": "someone/else",
		"title":      "whatever the model decided to call it",
	}
	key := oneStep("issue.create",
		requiredFact("project_id", modelcorpus.FactProjectPath),
		modelcorpus.Arg{Name: "title", Truth: modelcorpus.Truth{Authored: true}, Required: true},
	)
	trip := trajectory{calls: []modelrecord.Call{
		dynamicExecute(1, "issue.create", "issue.create", params),
	}}

	verdict := trip.score(t, key)

	if verdict.Outcome != OutcomeFailed {
		t.Fatalf("Outcome = %q, want %q", verdict.Outcome, OutcomeFailed)
	}
	reason := verdict.Steps[0].Reason
	if !strings.Contains(reason, "project_id") || strings.Contains(reason, "title") {
		t.Errorf("Reason = %q, want it to name the compared argument and not the authored one", reason)
	}
}

// TestScore_GitLabRefusingAfterTheWrongArguments_IsAModelFailure keeps the
// instance's answer from excusing the model: the refusal is its own class only
// when the dispatch and the arguments were right.
func TestScore_GitLabRefusingAfterTheWrongArguments_IsAModelFailure(t *testing.T) {
	wrong := map[string]any{"project_id": "someone/else"}
	trip := trajectory{calls: []modelrecord.Call{
		dynamicExecute(1, "project.get", "project.get", wrong, answered(modelrecord.OutcomeToolError)),
	}}

	verdict := trip.score(t, keyFor(t, "MT-002"))

	if verdict.Outcome != OutcomeFailed {
		t.Errorf("Outcome = %q, want %q", verdict.Outcome, OutcomeFailed)
	}
}

// TestScore_ADestructiveStepServedWithNoConfirmation_IsNeverConfirmed covers
// the classification of a destructive step that was reached without an
// approval, which is a different shape from one that was refused and abandoned.
func TestScore_ADestructiveStepServedWithNoConfirmation_IsNeverConfirmed(t *testing.T) {
	params := map[string]any{
		"project_id": defaultFacts()[modelcorpus.FactProjectPath],
		"issue_iid":  defaultFacts()[modelcorpus.FactIssueIID],
	}
	trip := trajectory{calls: []modelrecord.Call{
		dynamicExecute(1, "issue.delete", "issue.delete", params),
	}}

	verdict := trip.score(t, keyFor(t, "MT-013"))

	step := verdict.Steps[0]
	if !step.Reached {
		t.Fatalf("the step was not reached: %s", step.Reason)
	}
	if step.Confirmation != ConfirmationNever {
		t.Errorf("Confirmation = %q, want %q", step.Confirmation, ConfirmationNever)
	}
}

// TestScore_ARejectedArgumentBeforeAConfirmedCall_IsStillUnaided keeps the two
// refusals apart. Only a refused confirmation makes a confirmation
// server-aided; a rejected argument taught the model a parameter name and
// nothing about approving anything.
func TestScore_ARejectedArgumentBeforeAConfirmedCall_IsStillUnaided(t *testing.T) {
	params := map[string]any{
		"project_id": defaultFacts()[modelcorpus.FactProjectPath],
		"issue_iid":  defaultFacts()[modelcorpus.FactIssueIID],
	}
	trip := trajectory{calls: []modelrecord.Call{
		dynamicExecute(1, "issue.delete", "issue.delete", map[string]any{"issue": 3},
			refused(toolutil.RefusalInvalidParams)),
		dynamicConfirm(2, "issue.delete", params),
	}}

	verdict := trip.score(t, keyFor(t, "MT-013"))

	step := verdict.Steps[0]
	if step.Confirmation != ConfirmationUnaided {
		t.Errorf("Confirmation = %q, want %q: nothing refused this model a confirmation",
			step.Confirmation, ConfirmationUnaided)
	}
	if verdict.InvalidParams != 1 {
		t.Errorf("InvalidParams = %d, want the one rejected call", verdict.InvalidParams)
	}
}

// TestScore_ARecordThisTreeCannotRead_IsAnErrorAndNotAVerdict keeps the two
// kinds of failure apart: a model that did badly is a verdict, and a record
// naming a surface or a tier this tree cannot build a catalog for is an error.
func TestScore_ARecordThisTreeCannotRead_IsAnErrorAndNotAVerdict(t *testing.T) {
	cases := []struct {
		name string
		trip trajectory
	}{
		{name: "a surface that is not one of the three", trip: trajectory{surface: "everything"}},
		{name: "a tier no license names", trip: trajectory{tier: "enterprise-plus"}},
		{name: "no tier at all", trip: trajectory{tier: " "}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := Score(testCase.trip.build(), keyFor(t, "MT-002")); err == nil {
				t.Fatal("Score returned no error for a record this tree cannot read")
			}
		})
	}
}

// TestScore_AStandaloneTool_IsReachedByItsName covers the steps that belong to
// no catalog action: project discovery and the interactive creation flows are
// registered under one name on every surface, so the name is what a step names
// and the arguments sit at the top level whatever the surface.
func TestScore_AStandaloneTool_IsReachedByItsName(t *testing.T) {
	remote := "https://gitlab.example.com/eval-group/eval-project.git"
	facts := defaultFacts()
	facts[modelcorpus.FactRemoteURL] = remote
	key := modelcorpus.Key{Steps: []modelcorpus.Step{{
		Standalone: "gitlab_discover_project",
		Args: []modelcorpus.Arg{
			{Name: "remote_url", Truth: modelcorpus.Truth{Fact: modelcorpus.FactRemoteURL}, Required: true},
		},
	}}}
	trip := trajectory{
		facts: facts,
		calls: []modelrecord.Call{
			callOf(1, "gitlab_discover_project", map[string]any{"remote_url": remote}),
		},
	}

	verdict := trip.score(t, key)

	if verdict.Outcome != OutcomeCompleted {
		t.Fatalf("Outcome = %q (%s), want %q", verdict.Outcome, verdict.Reason, OutcomeCompleted)
	}
	step := verdict.Steps[0]
	if step.Name != "gitlab_discover_project" || !step.Standalone {
		t.Errorf("step named %q (standalone %v), want the tool's own name", step.Name, step.Standalone)
	}
	if step.MatchedBy != MatchTool {
		t.Errorf("MatchedBy = %q, want %q", step.MatchedBy, MatchTool)
	}
}

// TestScore_ARejectedArgumentAndARetry_IsCountedAsOverhead is the number that
// says what the default opaque meta schema costs: a model learning a parameter
// name from a refusal rather than from a published schema.
func TestScore_ARejectedArgumentAndARetry_IsCountedAsOverhead(t *testing.T) {
	trip := trajectory{
		surface: string(modelcorpus.SurfaceMeta),
		calls: []modelrecord.Call{
			metaExecute(1, "gitlab_project", "project.get", map[string]any{"project": "eval-group/eval-project"},
				refused(toolutil.RefusalInvalidParams)),
			metaExecute(2, "gitlab_project", "project.get", projectParams()),
		},
	}

	verdict := trip.score(t, keyFor(t, "MT-002"))

	if verdict.Outcome != OutcomeCompleted {
		t.Fatalf("Outcome = %q (%s), want %q", verdict.Outcome, verdict.Reason, OutcomeCompleted)
	}
	if verdict.InvalidParams != 1 {
		t.Errorf("InvalidParams = %d, want the one refusal the model learned from", verdict.InvalidParams)
	}
	if verdict.Unaided {
		t.Error("Unaided = true, want false: the server told this model what it had got wrong")
	}
	if verdict.Steps[0].AcceptedFirstTime {
		t.Error("AcceptedFirstTime = true, want false: the first call about the step was sent back")
	}
}

// TestScore_ARefusalInTheDefaultMode_IsTheWrongAnswer covers the mode where
// nothing is withheld and a refusal of the action is therefore a model naming
// something this deployment does not serve, rather than a decline.
func TestScore_ARefusalInTheDefaultMode_IsTheWrongAnswer(t *testing.T) {
	trip := trajectory{calls: []modelrecord.Call{
		dynamicExecute(1, "project.get", "project.get", projectParams(),
			refused(toolutil.RefusalUnknownAction)),
	}}

	verdict := trip.score(t, keyFor(t, "MT-002"))

	if verdict.Outcome != OutcomeFailed {
		t.Fatalf("Outcome = %q, want %q", verdict.Outcome, OutcomeFailed)
	}
	step := verdict.Steps[0]
	if step.Answer != AnswerWrong {
		t.Errorf("Answer = %q, want %q", step.Answer, AnswerWrong)
	}
	if step.Decline != DeclineNone {
		t.Errorf("Decline = %q, want none: nothing is withheld in this mode", step.Decline)
	}
}

// TestScore_AStepNamingSomethingTheCatalogLacks_IsReportedAsACorpusDefect keeps
// a corpus mistake from reading as a model failure.
func TestScore_AStepNamingSomethingTheCatalogLacks_IsReportedAsACorpusDefect(t *testing.T) {
	trip := trajectory{calls: []modelrecord.Call{
		dynamicExecute(1, "project.get", "project.get", projectParams()),
	}}

	verdict := trip.score(t, oneStep("project.invented_action"))

	step := verdict.Steps[0]
	if step.Known {
		t.Fatal("Known = true for an action the catalog does not have")
	}
	if step.Reason == "" {
		t.Error("Reason is empty, want it to say the catalog has no such action")
	}
}

// cleanVerdict is an attempt that satisfies every term [Verdict.Clean] names,
// which each case below then breaks in exactly one place.
//
// The step is deliberately the plainest one there is: not optional, not
// destructive, reached, observed, and carrying one argument that was compared
// and matched. Every field Clean reads therefore has a value here rather than a
// zero it happens to agree with, so a case that flips one is flipping the only
// thing that changed.
func cleanVerdict() Verdict {
	return Verdict{
		Case:     "MT-001",
		Outcome:  OutcomeCompleted,
		Unaided:  true,
		Verified: true,
		Steps: []StepVerdict{{
			Position:  1,
			Name:      "project.get",
			Reached:   true,
			Observed:  true,
			Complete:  true,
			Arguments: []ArgumentVerdict{{Name: "project_id", Compared: true, Matched: true}},
		}},
	}
}

// TestVerdict_Clean_EachTermItNames_DecidesTheHeadlineFigure holds the one
// number a reader takes away to the six terms its own comment promises.
//
// It matters because Clean is a conjunction and nothing else in this package
// asks it a question. Aggregate counts it, and the fixtures that reach that
// counter all fail some other term first, so until this test existed the
// condition was false on every attempt the suite scored and true on none: any
// term could have been dropped, inverted, or joined to its neighbor by the
// wrong operator, and the published column would have kept reading zero, which
// is exactly what a model that never works also reads.
//
// Each case therefore states a property rather than repeating a field: an
// attempt that went right end to end is clean; an attempt that was helped, did
// not finish, or did not check out afterwards is not; a step that was declined
// or that was optional and never reached leaves the answer alone; and a step
// that was unobserved, unreached, unconfirmed or sent the wrong value takes it
// away. The honest reading of "I could not see it" is not "it was fine", which
// is why the unobserved case wants false rather than true.
func TestVerdict_Clean_EachTermItNames_DecidesTheHeadlineFigure(t *testing.T) {
	cases := []struct {
		name  string
		spoil func(*Verdict)
		want  bool
		why   string
	}{
		{
			name:  "went right end to end",
			spoil: func(*Verdict) {},
			want:  true,
			why:   "every term holds, so the attempt is what the column counts",
		},
		{
			name:  "the attempt did not complete",
			spoil: func(v *Verdict) { v.Outcome = OutcomeFailed },
			want:  false,
			why:   "an attempt that did not finish the task did not just work",
		},
		{
			name:  "the server had to help",
			spoil: func(v *Verdict) { v.Unaided = false },
			want:  false,
			why:   "a refusal of ours taught the model something, so it did not just work",
		},
		{
			name:  "a recipe check did not hold afterwards",
			spoil: func(v *Verdict) { v.Verified = false },
			want:  false,
			why:   "what the attempt did to GitLab is the half no tool call can report",
		},
		{
			name: "a required step nothing reached",
			spoil: func(v *Verdict) {
				v.Steps[0].Reached = false
				v.Steps[0].Arguments = nil
			},
			want: false,
			why:  "a step the case declares and nothing answered is work not done",
		},
		{
			name: "a required step correctly declined in text",
			spoil: func(v *Verdict) {
				v.Steps[0].Reached = false
				v.Steps[0].Decline = DeclineByText
				v.Steps[0].Arguments = nil
			},
			want: true,
			why:  "declining is the right answer in read-only, so the step is complete",
		},
		{
			name: "an optional step nothing reached",
			spoil: func(v *Verdict) {
				v.Steps[0].Optional = true
				v.Steps[0].Reached = false
				v.Steps[0].Arguments = nil
			},
			want: true,
			why:  "nothing was owed there, so the step is passed over rather than judged",
		},
		{
			name: "an optional step that was reached and sent the wrong value",
			spoil: func(v *Verdict) {
				v.Steps[0].Optional = true
				v.Steps[0].Arguments[0].Matched = false
			},
			want: false,
			why:  "optional is about reaching the step, not about being excused once reached",
		},
		{
			name:  "a step whose dispatch was never observed",
			spoil: func(v *Verdict) { v.Steps[0].Observed = false },
			want:  false,
			why:   "a step the server never described cannot be said to have gone right",
		},
		{
			name: "a destructive step confirmed on the first call",
			spoil: func(v *Verdict) {
				v.Steps[0].Destructive = true
				v.Steps[0].Confirmation = ConfirmationUnaided
			},
			want: true,
			why:  "the approval was there, which is all the term asks",
		},
		{
			name: "a destructive step confirmed after the server asked",
			spoil: func(v *Verdict) {
				v.Steps[0].Destructive = true
				v.Steps[0].Confirmation = ConfirmationServerAided
			},
			want: true,
			why:  "learning the word from our refusal still carries the approval",
		},
		{
			name: "a destructive step that never carried its approval",
			spoil: func(v *Verdict) {
				v.Steps[0].Destructive = true
				v.Steps[0].Confirmation = ConfirmationNever
			},
			want: false,
			why:  "a destructive action run without approval is the term's whole point",
		},
		{
			name:  "an argument that did not carry the value the case declares",
			spoil: func(v *Verdict) { v.Steps[0].Arguments[0].Matched = false },
			want:  false,
			why:   "the right action with the wrong project is not the task",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			verdict := cleanVerdict()
			testCase.spoil(&verdict)
			if got := verdict.Clean(); got != testCase.want {
				t.Errorf("Clean() = %v, want %v: %s", got, testCase.want, testCase.why)
			}
		})
	}
}
