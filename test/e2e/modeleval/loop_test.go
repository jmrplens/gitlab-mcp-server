//go:build e2e

// loop_test.go drives the conversation's endings without a GitLab, a server
// child or a provider.
//
// Every ending the loop can reach is here, because the ending is the one thing
// about an attempt that no later reading can recover: a scorer can be
// corrected and a record re-scored, but an attempt written down as having run
// out of turns when it ran out of patience is wrong forever. The calls are
// answered by a table and the model is a stub, so what is under test is the
// loop and nothing else.

package modeleval

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelrecord"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/modeleval/internal/provider"
)

// TestTurnCap_IsLongerOnTheSurfaceThatTakesTwoCallsToReachAnAction checks the
// bound one attempt is held to.
//
// Dynamic gets more because reaching one catalog action there is find and then
// execute, so a case of three steps is six calls before a single retry.
func TestTurnCap_IsLongerOnTheSurfaceThatTakesTwoCallsToReachAnAction(t *testing.T) {
	tests := []struct {
		name    string
		steps   int
		surface harness.Surface
		want    int
	}{
		{name: "one step on dynamic", steps: 1, surface: harness.SurfaceDynamic, want: 7},
		{name: "three steps on dynamic", steps: 3, surface: harness.SurfaceDynamic, want: 13},
		{name: "one step on meta", steps: 1, surface: harness.SurfaceMeta, want: 6},
		{name: "three steps on individual", steps: 3, surface: harness.SurfaceIndividual, want: 10},
		{name: "a case with no step still gets a turn", steps: 0, surface: harness.SurfaceMeta, want: 6},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := turnCap(tc.steps, tc.surface); got != tc.want {
				t.Errorf("turnCap(%d, %s) = %d, want %d", tc.steps, tc.surface, got, tc.want)
			}
		})
	}
}

// TestConversation_Endings drives each ending the loop decides.
func TestConversation_Endings(t *testing.T) {
	tests := []struct {
		name      string
		answers   []stubAnswer
		cap       int
		wantEnded string
		wantTurns int
		wantCalls int
		wantIn    string
	}{
		{
			name:      "a model that answers in text having called nothing ends there",
			answers:   []stubAnswer{{text: "That operation is not available here."}},
			cap:       6,
			wantEnded: modelrecord.EndedNoToolCall,
			wantTurns: 1,
			wantIn:    "not available",
		},
		{
			name:      "a tool call whose arguments are not JSON ends it and nothing is repaired",
			answers:   []stubAnswer{{malformed: true}},
			cap:       6,
			wantEnded: modelrecord.EndedMalformed,
			wantTurns: 1,
			wantIn:    "not JSON",
		},
		{
			name:      "a provider that will not answer ends it as the provider's",
			answers:   []stubAnswer{{status: modelrecord.TurnRequestError, err: errors.New("no such model")}},
			cap:       6,
			wantEnded: modelrecord.EndedProviderError,
			wantTurns: 1,
			wantIn:    "no such model",
		},
		{
			name: "a model that keeps calling runs out of turns",
			answers: []stubAnswer{
				{tool: "gitlab_find_action"}, {tool: "gitlab_find_action"}, {tool: "gitlab_find_action"},
			},
			cap:       2,
			wantEnded: modelrecord.EndedOverBudget,
			wantTurns: 2,
			wantCalls: 2,
			wantIn:    "cap of 2",
		},
		{
			name: "a model that calls and then says what it did ended its own conversation",
			answers: []stubAnswer{
				{tool: "gitlab_execute_action"},
				{text: "Done."},
			},
			cap:       6,
			wantEnded: modelrecord.EndedCompleted,
			wantTurns: 2,
			wantCalls: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			talk := newTestConversation(&stubProvider{answers: tc.answers})
			talk.cap = tc.cap

			result := talk.run(context.Background())
			if result.EndedBy != tc.wantEnded {
				t.Errorf("EndedBy = %q, want %q (reason %q)", result.EndedBy, tc.wantEnded, result.Reason)
			}
			if len(result.Turns) != tc.wantTurns {
				t.Errorf("recorded %d turn(s), want %d", len(result.Turns), tc.wantTurns)
			}
			if len(result.Calls) != tc.wantCalls {
				t.Errorf("recorded %d call(s), want %d", len(result.Calls), tc.wantCalls)
			}
			if tc.wantIn != "" && !strings.Contains(result.Reason, tc.wantIn) {
				t.Errorf("reason = %q, want it to mention %q", result.Reason, tc.wantIn)
			}
		})
	}
}

// TestConversation_RateLimited_IsAskedAgainAndEveryTryIsRecorded checks the one
// thing the runner does retry, and that it writes down what the retries cost.
//
// A request that succeeded on the third try cost three requests, and a record
// holding only the one that worked would say a rate-limited run was as cheap
// as a clean one.
func TestConversation_RateLimited_IsAskedAgainAndEveryTryIsRecorded(t *testing.T) {
	withoutBackoff(t)

	talk := newTestConversation(&stubProvider{answers: []stubAnswer{
		{status: modelrecord.TurnRateLimited, err: errors.New("slow down")},
		{status: modelrecord.TurnRateLimited, err: errors.New("slow down")},
		{text: "Nothing here can do that."},
	}})

	result := talk.run(context.Background())
	if result.EndedBy != modelrecord.EndedNoToolCall {
		t.Errorf("EndedBy = %q, want the third try's own ending", result.EndedBy)
	}
	if len(result.Turns) != 3 {
		t.Fatalf("recorded %d turn(s), want one per try", len(result.Turns))
	}
	for index, turn := range result.Turns {
		if turn.Index != 1 {
			t.Errorf("turn %d is at index %d, want 1: three tries are one turn of the conversation",
				index, turn.Index)
		}
		if turn.Try != index+1 {
			t.Errorf("turn %d says try %d, want %d", index, turn.Try, index+1)
		}
	}
}

// TestConversation_RequestRefused_IsNotAskedAgain checks the other half: a
// request the provider rejected as malformed is this side's fault, and sending
// it again would spend the budget twice to learn the same thing.
func TestConversation_RequestRefused_IsNotAskedAgain(t *testing.T) {
	withoutBackoff(t)

	stub := &stubProvider{answers: []stubAnswer{
		{status: modelrecord.TurnRequestError, err: errors.New("unknown parameter")},
	}}
	talk := newTestConversation(stub)

	result := talk.run(context.Background())
	if result.EndedBy != modelrecord.EndedProviderError {
		t.Errorf("EndedBy = %q, want %q", result.EndedBy, modelrecord.EndedProviderError)
	}
	if stub.calls != 1 {
		t.Errorf("the provider was asked %d times, want once", stub.calls)
	}
}

// TestConversation_RateLimitedToTheEnd_GivesUpAfterTheTries checks that the
// retries are bounded: a provider having a bad day must not hold a run open.
func TestConversation_RateLimitedToTheEnd_GivesUpAfterTheTries(t *testing.T) {
	withoutBackoff(t)

	stub := &stubProvider{always: stubAnswer{status: modelrecord.TurnRateLimited, err: errors.New("slow down")}}
	talk := newTestConversation(stub)

	result := talk.run(context.Background())
	if result.EndedBy != modelrecord.EndedProviderError {
		t.Errorf("EndedBy = %q, want %q", result.EndedBy, modelrecord.EndedProviderError)
	}
	if stub.calls != providerTries {
		t.Errorf("the provider was asked %d times, want %d", stub.calls, providerTries)
	}
}

// TestConversation_CarriesTheAnswersForwardAndTheStructureBeside checks the
// shape of the second request: the model's own turn goes back as the provider
// rendered it, the tool result goes back paired with the call, and the
// structured content travels beside the conversation rather than in it.
func TestConversation_CarriesTheAnswersForwardAndTheStructureBeside(t *testing.T) {
	stub := &stubProvider{answers: []stubAnswer{
		{tool: "gitlab_execute_action", echo: `{"provider":"rendering"}`},
		{text: "Done."},
	}}
	talk := newTestConversation(stub)
	talk.dispatch = answering(harness.ModelAnswer{Outcome: modelrecord.OutcomeOK, Text: "one issue"},
		json.RawMessage(`{"issue":{"iid":7}}`))

	talk.run(context.Background())
	if len(stub.seen) != 2 {
		t.Fatalf("the provider saw %d request(s), want 2", len(stub.seen))
	}

	second := stub.seen[1]
	if len(second.Messages) != 3 {
		t.Fatalf("the second request carries %d message(s), want the prompt, the turn and the result",
			len(second.Messages))
	}
	if got := string(second.Messages[1].Echo); got != `{"provider":"rendering"}` {
		t.Errorf("the assistant turn went back as %q, want the provider's own rendering", got)
	}
	if len(second.Messages[2].Results) != 1 || second.Messages[2].Results[0].Content != "one issue" {
		t.Errorf("the tool result went back as %+v, want what the model read", second.Messages[2].Results)
	}
	if len(second.Replay.Produced) != 1 || string(second.Replay.Produced[0]) != `{"issue":{"iid":7}}` {
		t.Errorf("the structured content went as %v, want the call's own result", second.Replay.Produced)
	}
}

// TestToolResultOf_SaysWhetherTheCallWorked checks what a model is handed back.
//
// A refusal has to read as an error, because the one thing a read-only
// deployment wants a model to notice is that the operation was not offered.
func TestToolResultOf_SaysWhetherTheCallWorked(t *testing.T) {
	tests := []struct {
		name        string
		answer      harness.ModelAnswer
		wantError   bool
		wantContent string
	}{
		{
			name:        "an answered call",
			answer:      harness.ModelAnswer{Outcome: modelrecord.OutcomeOK, Text: "two issues"},
			wantContent: "two issues",
		},
		{
			name:        "a safe-mode preview is not an error",
			answer:      harness.ModelAnswer{Outcome: modelrecord.OutcomePreview, Text: "would create"},
			wantContent: "would create",
		},
		{
			name: "a refusal is",
			answer: harness.ModelAnswer{
				Outcome: modelrecord.RefusedOutcome("unknown_action"),
				Text:    "this deployment does not offer that",
			},
			wantError:   true,
			wantContent: "does not offer",
		},
		{
			name: "a protocol error carries the message the model was handed",
			answer: harness.ModelAnswer{
				Outcome: modelrecord.OutcomeProtocolError,
				Err:     errors.New("unknown tool"),
			},
			wantError:   true,
			wantContent: "unknown tool",
		},
		{
			name:        "an answer with no text at all still says something",
			answer:      harness.ModelAnswer{Outcome: modelrecord.OutcomeOK},
			wantContent: "no text",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			call := modelrecord.Block{Kind: modelrecord.BlockToolCall, Tool: "gitlab_issue", CallID: "call-1"}
			result := toolResultOf(call, tc.answer)
			if result.IsError != tc.wantError {
				t.Errorf("IsError = %t, want %t", result.IsError, tc.wantError)
			}
			if !strings.Contains(result.Content, tc.wantContent) {
				t.Errorf("Content = %q, want it to mention %q", result.Content, tc.wantContent)
			}
			if result.CallID != "call-1" || result.Tool != "gitlab_issue" {
				t.Errorf("the result was not paired with its call: %+v", result)
			}
		})
	}
}

// TestSpokenText_ReadsTheProseAndSkipsTheReasoning checks what a turn with no
// tool call is written down as having said.
func TestSpokenText_ReadsTheProseAndSkipsTheReasoning(t *testing.T) {
	blocks := []modelrecord.Block{
		{Kind: modelrecord.BlockThinking, Text: "the user wants an issue"},
		{Kind: modelrecord.BlockText, Text: "   "},
		{Kind: modelrecord.BlockText, Text: "I cannot do that here."},
	}
	if got := spokenText(blocks); got != "I cannot do that here." {
		t.Errorf("spokenText = %q, want the first non-blank prose", got)
	}
	if got := spokenText(nil); got != "" {
		t.Errorf("spokenText(nil) = %q, want nothing", got)
	}
}

// TestCostOf_ChargesFourNumbersAndNotOne checks that a cached conversation is
// not read as a frugal one.
func TestCostOf_ChargesFourNumbersAndNotOne(t *testing.T) {
	price := modelrecord.Price{
		InputPerMillionUSD:      3,
		OutputPerMillionUSD:     15,
		CacheWritePerMillionUSD: 3.75,
		CacheReadPerMillionUSD:  0.30,
	}
	usage := modelrecord.Usage{Input: 1_000_000, Output: 1_000_000, CacheCreated: 1_000_000, CacheRead: 1_000_000}
	if got := costOf(usage, price); got != 22.05 {
		t.Errorf("costOf = %v, want 22.05", got)
	}
	if got := costOf(modelrecord.Usage{}, price); got != 0 {
		t.Errorf("costOf of nothing = %v, want 0", got)
	}
}

// TestBudget_StopsAtTheCeilingAndNeverWithoutAPrice checks the accountant.
//
// A model with no price charges nothing, which is what the unpriced escape
// hatch asks for: a run allowed to start without a price cannot be stopped by
// a budget, and pretending otherwise would stop it at the wrong moment.
func TestBudget_StopsAtTheCeilingAndNeverWithoutAPrice(t *testing.T) {
	price := modelrecord.Price{InputPerMillionUSD: 10, OutputPerMillionUSD: 10}

	none := newBudget(0)
	none.charge(modelrecord.Usage{Input: 10_000_000}, &price)
	if none.Exhausted() {
		t.Errorf("a run with no ceiling is exhausted after spending %v", none.Spent())
	}

	bounded := newBudget(1)
	bounded.charge(modelrecord.Usage{Input: 50_000}, &price)
	if bounded.Exhausted() {
		t.Errorf("a run is exhausted after spending %v of $1", bounded.Spent())
	}
	bounded.charge(modelrecord.Usage{Output: 100_000}, &price)
	if !bounded.Exhausted() {
		t.Errorf("a run that has spent %v of $1 is not exhausted", bounded.Spent())
	}

	unpriced := newBudget(1)
	unpriced.charge(modelrecord.Usage{Input: 10_000_000}, nil)
	if unpriced.Spent() != 0 || unpriced.Exhausted() {
		t.Errorf("an unpriced model charged %v", unpriced.Spent())
	}
}

// withoutBackoff removes the wait between retries, which is the only way the
// second and third tries are reachable in a test that finishes.
func withoutBackoff(t *testing.T) {
	t.Helper()
	previous := providerBackoff
	providerBackoff = func(int) time.Duration { return 0 }
	t.Cleanup(func() { providerBackoff = previous })
}

// newTestConversation builds a conversation whose calls are answered without a
// server.
func newTestConversation(adapter provider.Provider) *conversation {
	return &conversation{
		adapter:   adapter,
		dispatch:  answering(harness.ModelAnswer{Outcome: modelrecord.OutcomeOK, Text: "ok"}, nil),
		contract:  "a contract",
		stimulus:  stimulus{Text: "do the thing", Facts: map[string]string{"project_path": "org/tools"}},
		caseID:    "MT-000",
		surface:   harness.SurfaceDynamic,
		attempt:   "attempt-1",
		cap:       6,
		requested: func(string, json.RawMessage) string { return "issue.list" },
	}
}

// answering returns a dispatch that gives one answer to every call.
func answering(
	answer harness.ModelAnswer,
	structured json.RawMessage,
) func(context.Context, callPosition, harness.ModelCall, string) (harness.ModelAnswer, *modelrecord.Call) {
	return func(
		_ context.Context,
		at callPosition,
		call harness.ModelCall,
		requested string,
	) (harness.ModelAnswer, *modelrecord.Call) {
		return answer, &modelrecord.Call{
			Attempt:         at.Attempt,
			Turn:            at.Turn,
			Index:           at.Index,
			Tool:            call.Tool,
			Arguments:       call.Arguments,
			RequestedAction: requested,
			Outcome:         answer.Outcome,
			Result:          structured,
		}
	}
}

// stubAnswer is one turn a stub provider gives.
type stubAnswer struct {
	// text is prose the model wrote.
	text string
	// tool names a tool call it made.
	tool string
	// echo is the provider's own rendering of the turn.
	echo string
	// malformed makes the tool call one whose arguments are not JSON.
	malformed bool
	// status is the turn status, empty for a served request.
	status string
	// err is what the adapter returns beside the response.
	err error
}

// stubProvider answers from a table, and remembers what it was asked.
type stubProvider struct {
	answers []stubAnswer
	// always answers every turn when the table is empty, for the paths that
	// need an unbounded supply of one answer.
	always stubAnswer
	calls  int
	seen   []provider.Request
}

// Call answers the next turn in the table.
func (s *stubProvider) Call(_ context.Context, request provider.Request) (provider.Response, error) {
	s.seen = append(s.seen, request)
	answer := s.always
	if s.calls < len(s.answers) {
		answer = s.answers[s.calls]
	}
	s.calls++

	response := provider.Response{Status: modelrecord.TurnOK, Echo: json.RawMessage(answer.echo)}
	if answer.status != "" {
		response.Status = answer.status
	}
	switch {
	case answer.malformed:
		response.Blocks = []modelrecord.Block{{
			Kind: modelrecord.BlockToolCall, Tool: "gitlab_issue", Raw: "{not json", CallID: "call-x",
		}}
	case answer.tool != "":
		response.Blocks = []modelrecord.Block{{
			Kind: modelrecord.BlockToolCall, Tool: answer.tool,
			Arguments: json.RawMessage(`{"query":"issues"}`), CallID: "call-1",
		}}
	default:
		response.Blocks = []modelrecord.Block{{Kind: modelrecord.BlockText, Text: answer.text}}
	}
	if answer.err != nil {
		response.Detail = answer.err.Error()
		return response, answer.err
	}
	return response, nil
}

// Spec returns a fake spec, which nothing under test reads.
func (s *stubProvider) Spec() provider.Spec {
	return provider.Spec{Raw: "stub:model", Provider: "stub", Model: "model"}
}

// ToolDigest hashes nothing: the loop never asks a stub for one.
func (s *stubProvider) ToolDigest([]provider.Tool) string { return "stub-digest" }
