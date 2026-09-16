package modelscore

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelcorpus"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelrecord"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Matching calls to steps, which is in order and reads the server's dispatch
// first.

// ranAction is one call that ran one action, with nothing else about it.
func ranAction(index int, action string) Event {
	return Event{
		Index: index, Tool: "gitlab_execute_action", Dispatched: action, Observed: true,
		Outcome: modelrecord.OutcomeOK,
	}
}

// TestMatchSteps_AreMatchedInOrder is why a model that ran the right actions in
// the wrong order does not score as one that ran them in the right order.
func TestMatchSteps_AreMatchedInOrder(t *testing.T) {
	key := modelcorpus.Key{Steps: []modelcorpus.Step{
		{Action: "project.get"},
		{Action: "issue.list"},
	}}

	t.Run("in the order the key declares", func(t *testing.T) {
		matches := matchSteps(key, []Event{ranAction(1, "project.get"), ranAction(2, "issue.list")})

		if !matches[0].reached || !matches[1].reached {
			t.Fatalf("reached = %v and %v, want both", matches[0].reached, matches[1].reached)
		}
		if matches[0].event.Index != 1 || matches[1].event.Index != 2 {
			t.Errorf("reached by calls %d and %d, want 1 and 2",
				matches[0].event.Index, matches[1].event.Index)
		}
	})

	t.Run("in the other order", func(t *testing.T) {
		matches := matchSteps(key, []Event{ranAction(1, "issue.list"), ranAction(2, "project.get")})

		if !matches[0].reached {
			t.Error("the first step was not reached by the call that ran it")
		}
		if matches[1].reached {
			t.Error("the second step was reached by a call that came before the first step's")
		}
	})
}

// TestMatchSteps_TheSameActionTwice_IsTwoCalls covers the shape a case takes
// when it asks for the same operation on two objects: two steps naming one
// action must not both be reached by one call.
func TestMatchSteps_TheSameActionTwice_IsTwoCalls(t *testing.T) {
	key := modelcorpus.Key{Steps: []modelcorpus.Step{
		{Action: "issue.create"},
		{Action: "issue.create"},
	}}

	t.Run("two calls reach two steps", func(t *testing.T) {
		matches := matchSteps(key, []Event{ranAction(1, "issue.create"), ranAction(2, "issue.create")})

		if matches[0].event.Index != 1 || matches[1].event.Index != 2 {
			t.Errorf("reached by calls %d and %d, want one each",
				matches[0].event.Index, matches[1].event.Index)
		}
	})

	t.Run("one call reaches one step", func(t *testing.T) {
		matches := matchSteps(key, []Event{ranAction(1, "issue.create")})

		if !matches[0].reached || matches[1].reached {
			t.Errorf("reached = %v and %v, want the first only", matches[0].reached, matches[1].reached)
		}
	})
}

// TestMatchSteps_AStepNothingReached_ConsumesNoCalls keeps one missing step
// from dragging every later step out of reach.
func TestMatchSteps_AStepNothingReached_ConsumesNoCalls(t *testing.T) {
	key := modelcorpus.Key{Steps: []modelcorpus.Step{
		{Action: "project.get"},
		{Action: "issue.list"},
	}}

	matches := matchSteps(key, []Event{ranAction(1, "issue.list")})

	if matches[0].reached {
		t.Error("the first step was reached by a call that ran another action")
	}
	if !matches[1].reached {
		t.Error("the second step was not reached: a step nothing answered must consume no calls")
	}
}

// TestMatchSteps_ADiscoveryCall_IsNeverAStep is the other half of keeping the
// find call out of the step columns: it is skipped before anything compares it.
func TestMatchSteps_ADiscoveryCall_IsNeverAStep(t *testing.T) {
	search := Event{Index: 1, Tool: "gitlab_find_action", Discovery: true, Observed: true}
	key := modelcorpus.Key{Steps: []modelcorpus.Step{{Action: "project.get"}}}

	matches := matchSteps(key, []Event{search, ranAction(2, "project.get")})

	if matches[0].event.Index != 2 {
		t.Errorf("reached by call %d, want the execute call", matches[0].event.Index)
	}
	if len(matches[0].prior) != 0 {
		t.Errorf("prior = %d call(s), want none: a search is not an attempt at the step", len(matches[0].prior))
	}
}

// TestMatchSteps_ARefusalAskingForTheCallAgain_IsNotTheReachingCall is the rule
// without which a destructive step could never be scored as reached: the
// confirmation arrives on the call after the refusal.
func TestMatchSteps_ARefusalAskingForTheCallAgain_IsNotTheReachingCall(t *testing.T) {
	cases := []struct {
		name   string
		reason string
	}{
		{name: "a rejected argument", reason: toolutil.RefusalInvalidParams},
		{name: "a missing confirmation", reason: toolutil.RefusalNeedsConfirmation},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			asked := ranAction(1, "issue.delete")
			asked.Outcome = modelrecord.RefusedOutcome(testCase.reason)
			key := modelcorpus.Key{Steps: []modelcorpus.Step{{Action: "issue.delete"}}}

			matches := matchSteps(key, []Event{asked, ranAction(2, "issue.delete")})

			if matches[0].event.Index != 2 {
				t.Errorf("reached by call %d, want the one the server answered", matches[0].event.Index)
			}
			if len(matches[0].prior) != 1 {
				t.Errorf("prior = %d, want the refusal recorded as the step's history", len(matches[0].prior))
			}
		})
	}
}

// TestMatchSteps_ARefusalOfTheActionItself_IsTheReachingCall is the other side
// of that rule. A refusal the server makes about the action, rather than about
// the call, ends the step where it fell, which is what makes a read-only
// decline reachable at all.
func TestMatchSteps_ARefusalOfTheActionItself_IsTheReachingCall(t *testing.T) {
	withheld := Event{
		Index:     1,
		Requested: "project.star",
		Outcome:   modelrecord.RefusedOutcome(toolutil.RefusalUnknownAction),
		Observed:  true,
	}
	key := modelcorpus.Key{Steps: []modelcorpus.Step{{Action: "project.star"}}}

	matches := matchSteps(key, []Event{withheld})

	if !matches[0].reached || matches[0].by != MatchRequested {
		t.Errorf("reached = %v by %q, want it reached by what the model requested",
			matches[0].reached, matches[0].by)
	}
}

// TestNamesStep_ReadsTheDispatchBeforeTheRequest pins the precedence the whole
// file rests on, including the case it exists for: a call that ran another
// action is not this step whatever the model asked for.
func TestNamesStep_ReadsTheDispatchBeforeTheRequest(t *testing.T) {
	cases := []struct {
		name  string
		event Event
		step  modelcorpus.Step
		by    MatchKind
		names bool
	}{
		{
			name:  "the dispatch names the step",
			event: Event{Dispatched: "project.get", Requested: "project.get"},
			step:  modelcorpus.Step{Action: "project.get"},
			by:    MatchDispatched,
			names: true,
		},
		{
			name:  "the dispatch names another action",
			event: Event{Dispatched: "environment.protected_get", Requested: "environment.get"},
			step:  modelcorpus.Step{Action: "environment.get"},
			by:    MatchDispatched,
			names: false,
		},
		{
			name:  "no dispatch, and the request names the step",
			event: Event{Requested: "project.star"},
			step:  modelcorpus.Step{Action: "project.star"},
			by:    MatchRequested,
			names: true,
		},
		{
			name:  "no dispatch and no request",
			event: Event{Tool: "gitlab_invented"},
			step:  modelcorpus.Step{Action: "project.star"},
			names: false,
		},
		{
			name:  "a tool outside the catalog, named by the span",
			event: Event{Tool: "gitlab_discover_project", DispatchedTool: "gitlab_discover_project"},
			step:  modelcorpus.Step{Standalone: "gitlab_discover_project"},
			by:    MatchTool,
			names: true,
		},
		{
			name:  "a tool outside the catalog, named by the model when no span arrived",
			event: Event{Tool: "gitlab_discover_project"},
			step:  modelcorpus.Step{Standalone: "gitlab_discover_project"},
			by:    MatchTool,
			names: true,
		},
		{
			name:  "a tool outside the catalog that the span says was another",
			event: Event{Tool: "gitlab_discover_project", DispatchedTool: "gitlab_interactive_issue_create"},
			step:  modelcorpus.Step{Standalone: "gitlab_discover_project"},
			by:    MatchTool,
			names: false,
		},
		{
			name:  "another tool named by the model when no span arrived",
			event: Event{Tool: "gitlab_interactive_issue_create"},
			step:  modelcorpus.Step{Standalone: "gitlab_discover_project"},
			by:    MatchTool,
			names: false,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			by, names := namesStep(testCase.event, testCase.step)
			if names != testCase.names {
				t.Fatalf("names = %v, want %v", names, testCase.names)
			}
			if names && by != testCase.by {
				t.Errorf("by = %q, want %q", by, testCase.by)
			}
		})
	}
}

// TestMatchSteps_AnOptionalStepNothingReached_LeavesTheRestAlone covers the
// declaration a key makes when a step is allowed to be missing.
func TestMatchSteps_AnOptionalStepNothingReached_LeavesTheRestAlone(t *testing.T) {
	key := modelcorpus.Key{Steps: []modelcorpus.Step{
		{Action: "project.get"},
		{Action: "issue.list", Optional: true},
		{Action: "project.star"},
	}}

	matches := matchSteps(key, []Event{ranAction(1, "project.get"), ranAction(2, "project.star")})

	if !matches[0].reached || matches[1].reached || !matches[2].reached {
		t.Errorf("reached = %v, %v, %v, want the two the model ran",
			matches[0].reached, matches[1].reached, matches[2].reached)
	}
}
