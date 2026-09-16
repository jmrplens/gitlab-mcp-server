//go:build e2e

// call_test.go covers the adapter offline, from hand-written answers.
//
// It can be offline because the adapter is pure: what the server did is
// already decided by the time it is called, and the question here is only
// whether the record says so. Three of its cases are the ones the record was
// designed around, and each was a defect in the evaluator this replaces: the
// values a model sent have to survive into the record, a call that named one
// action and ran another has to carry both, and the text a safe-mode preview
// answered with is the card the model read rather than the harness's summary
// of it.

package modeleval

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelrecord"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// modelPosition is the position every case here records at.
var modelPosition = callPosition{Attempt: "MT-001/claude/dynamic/1", Turn: 2, Index: 3}

// errUnknownTool is how the SDK refuses a tool name no surface registers,
// which is the whole of what the model is told in that case.
var errUnknownTool = errors.New("unknown tool \"gitlab_project_delete\"")

// TestCallLine_AnsweredCall_RecordsWhatTheModelSentAndWhatTheServerRan is the
// line the whole record is built around.
//
// Both halves are needed and neither substitutes for the other. What the model
// sent is the measurement of the model; what the server dispatched is the only
// evidence about the server, and a call that named issue.list and ran
// issue.get proves nothing about issue.list.
func TestCallLine_AnsweredCall_RecordsWhatTheModelSentAndWhatTheServerRan(t *testing.T) {
	arguments := json.RawMessage(`{"action":"issue.list","params":{"project_id":42,"state":"opened"}}`)
	line := callLine(modelPosition,
		harness.ModelCall{Tool: "gitlab_execute_action", Arguments: arguments},
		"issue.list",
		harness.ModelAnswer{
			Result: &mcp.CallToolResult{
				Content:           []mcp.Content{&mcp.TextContent{Text: "| IID | Title |"}},
				StructuredContent: map[string]any{"issues": []any{map[string]any{"iid": 7.0}}},
			},
			Outcome:  modelrecord.OutcomeOK,
			Text:     "| IID | Title |",
			Duration: 1500 * time.Microsecond,
			TraceID:  "0af7651916cd43dd8448eb211c80319c",
			Dispatch: harness.DispatchFacts{
				Tool:   "gitlab_execute_action",
				Action: "issue.list",
				Domain: "issue",
				Status: "STATUS_CODE_OK",
			},
			Requests:         2,
			DispatchObserved: true,
		})

	if string(line.Arguments) != string(arguments) {
		t.Errorf("Arguments = %s, want the bytes the model produced, unchanged: %s", line.Arguments, arguments)
	}
	cases := []struct {
		name string
		got  any
		want any
	}{
		{name: "attempt", got: line.Attempt, want: modelPosition.Attempt},
		{name: "turn", got: line.Turn, want: modelPosition.Turn},
		{name: "index", got: line.Index, want: modelPosition.Index},
		{name: "tool", got: line.Tool, want: "gitlab_execute_action"},
		{name: "requested action", got: line.RequestedAction, want: "issue.list"},
		{name: "dispatched action", got: line.DispatchedAction, want: "issue.list"},
		{name: "dispatched tool", got: line.DispatchedTool, want: "gitlab_execute_action"},
		{name: "outcome", got: line.Outcome, want: modelrecord.OutcomeOK},
		{name: "text", got: line.Text, want: "| IID | Title |"},
		{name: "duration", got: line.DurationMS, want: 1.5},
		{name: "trace", got: line.TraceID, want: "0af7651916cd43dd8448eb211c80319c"},
		{name: "requests", got: line.Requests, want: 2},
		{name: "dispatch observed", got: line.DispatchObserved, want: true},
		{name: "result truncated", got: line.ResultTruncated, want: false},
		{name: "text truncated", got: line.TextTruncated, want: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.got != testCase.want {
				t.Errorf("%s = %v, want %v", testCase.name, testCase.got, testCase.want)
			}
		})
	}
	if !strings.Contains(string(line.Result), `"iid":7`) {
		t.Errorf("Result = %s, want the structured content the call answered with: an argument of a later "+
			"step bound to a field of this answer can only be checked against it", line.Result)
	}
}

// TestCallLine_RewrittenRoute_KeepsBothTheRequestAndTheDispatch checks the
// case the two action fields exist for.
//
// The server rewrites some calls: environment.get on an environment named
// rather than numbered runs environment.protected_get. A record carrying only
// what was asked would credit the model with reaching a step the server never
// ran, and one carrying only what ran would lose the fact that the model asked
// for something else.
func TestCallLine_RewrittenRoute_KeepsBothTheRequestAndTheDispatch(t *testing.T) {
	line := callLine(modelPosition,
		harness.ModelCall{Tool: "gitlab_environment", Arguments: json.RawMessage(`{"action":"get"}`)},
		"environment.get",
		harness.ModelAnswer{
			Outcome:          modelrecord.OutcomeOK,
			Dispatch:         harness.DispatchFacts{Tool: "gitlab_environment", Action: "environment.protected_get"},
			DispatchObserved: true,
		})

	if line.RequestedAction != "environment.get" {
		t.Errorf("RequestedAction = %q, want environment.get", line.RequestedAction)
	}
	if line.DispatchedAction != "environment.protected_get" {
		t.Errorf("DispatchedAction = %q, want environment.protected_get", line.DispatchedAction)
	}
}

// TestCallLine_RefusedCall_CarriesTheServersOwnReason checks that a refusal is
// recorded as the server's and not as the client's reading of its prose.
//
// It is what the read-only rows rest on: an action a protective mode removed
// is refused before anything dispatches, so the reason lives on the span
// alone, and whether the model declined correctly is decided by that word.
func TestCallLine_RefusedCall_CarriesTheServersOwnReason(t *testing.T) {
	line := callLine(modelPosition,
		harness.ModelCall{Tool: "gitlab_execute_action", Arguments: json.RawMessage(`{"action":"project.delete"}`)},
		"project.delete",
		harness.ModelAnswer{
			Result:  &mcp.CallToolResult{IsError: true},
			Outcome: modelrecord.RefusedOutcome("unknown_action"),
			Failure: harness.FailureUnknownAction,
			Text:    "unknown action project.delete",
			Dispatch: harness.DispatchFacts{
				Tool:          "gitlab_execute_action",
				RefusalReason: "unknown_action",
				ErrorType:     "unknown_action",
				Status:        "STATUS_CODE_ERROR",
			},
			DispatchObserved: true,
		})

	if line.RefusalReason != "unknown_action" {
		t.Errorf("RefusalReason = %q, want unknown_action", line.RefusalReason)
	}
	if line.DispatchedAction != "" {
		t.Errorf("DispatchedAction = %q, want empty: the server refused before it dispatched anything",
			line.DispatchedAction)
	}
	if line.Outcome != modelrecord.RefusedOutcome("unknown_action") {
		t.Errorf("Outcome = %q, want the refusal spelled with its reason", line.Outcome)
	}
}

// TestCallLine_SafeModePreview_RecordsTheCardTheModelRead checks the one
// answer whose text the harness replaces.
//
// classify puts a summary of the blocked mutation in its text field, which is
// the right thing for a failure message and the wrong thing for a record of
// what a model read: the model was shown the preview card and its next move
// was based on that. A record carrying "safe mode blocked gitlab_project"
// would make every safe-mode turn unreadable after the fact.
func TestCallLine_SafeModePreview_RecordsTheCardTheModelRead(t *testing.T) {
	card := "### Safe mode\n\nThis would create the project `demo`."
	line := callLine(modelPosition,
		harness.ModelCall{Tool: "gitlab_project", Arguments: json.RawMessage(`{"action":"create"}`)},
		"project.create",
		harness.ModelAnswer{
			Result:           &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: card}}},
			Outcome:          modelrecord.OutcomePreview,
			Failure:          harness.FailureSafeMode,
			Text:             "safe mode blocked gitlab_project",
			Dispatch:         harness.DispatchFacts{Tool: "gitlab_project", Action: "project.create", RefusalReason: "safe_mode"},
			DispatchObserved: true,
		})

	if line.Text != card {
		t.Errorf("Text = %q, want the preview card the model read", line.Text)
	}
	if line.Outcome != modelrecord.OutcomePreview {
		t.Errorf("Outcome = %q, want %q", line.Outcome, modelrecord.OutcomePreview)
	}
}

// TestCallLine_ProtocolError_RecordsTheMessageAsTheAnswer checks what an
// unregistered tool name leaves behind.
//
// It is how every withheld action looks on the individual surface, and a
// JSON-RPC error carries no result at all, so the only text the model was
// handed is the error's message. A line with an empty text would say the model
// was told nothing, which is not what happened and is not what it answered to.
func TestCallLine_ProtocolError_RecordsTheMessageAsTheAnswer(t *testing.T) {
	line := callLine(modelPosition,
		harness.ModelCall{Tool: "gitlab_project_delete", Arguments: json.RawMessage(`{}`)},
		"project.delete",
		harness.ModelAnswer{
			Err:              errUnknownTool,
			Outcome:          modelrecord.OutcomeProtocolError,
			Failure:          harness.FailureProtocolError,
			DispatchObserved: false,
		})

	if line.Text != errUnknownTool.Error() {
		t.Errorf("Text = %q, want the error the server refused with", line.Text)
	}
	if line.Result != nil {
		t.Errorf("Result = %s, want nothing: a JSON-RPC error carries no result", line.Result)
	}
	if line.DispatchObserved {
		t.Error("DispatchObserved = true, want false: nothing was dispatched, so nothing here is about what ran")
	}
}

// TestCallLine_LongAnswer_IsCappedAndSaysSo checks that a record stays a
// record.
//
// A single list call can answer with megabytes, and a shard whose size is
// decided by the largest project in the fixture is a shard nobody keeps. The
// flag is the half that matters to a scorer: a value bound into a truncated
// answer fails with the truncation as its reason rather than missing silently.
func TestCallLine_LongAnswer_IsCappedAndSaysSo(t *testing.T) {
	long := strings.Repeat("x", modelrecord.MaxTextBytes+1)
	structured, err := json.Marshal(map[string]string{"body": long + long})
	if err != nil {
		t.Fatalf("building the oversized answer: %v", err)
	}

	line := callLine(modelPosition,
		harness.ModelCall{Tool: "gitlab_execute_action", Arguments: json.RawMessage(`{"action":"file.get"}`)},
		"file.get",
		harness.ModelAnswer{
			Result:           &mcp.CallToolResult{StructuredContent: json.RawMessage(structured)},
			Outcome:          modelrecord.OutcomeOK,
			Text:             long,
			DispatchObserved: true,
		})

	if !line.TextTruncated {
		t.Error("TextTruncated = false, want true")
	}
	if len(line.Text) != modelrecord.MaxTextBytes {
		t.Errorf("len(Text) = %d, want %d", len(line.Text), modelrecord.MaxTextBytes)
	}
	if !line.ResultTruncated {
		t.Error("ResultTruncated = false, want true")
	}
}

// TestCallLine_UnencodableResult_KeepsTheCallAndDropsTheStructure checks that a
// result nothing can encode costs the structure and not the line.
//
// The call happened and the model read the text beside it, so a line that
// failed to write would lose the one record of a call that was paid for.
func TestCallLine_UnencodableResult_KeepsTheCallAndDropsTheStructure(t *testing.T) {
	line := callLine(modelPosition,
		harness.ModelCall{Tool: "gitlab_execute_action", Arguments: json.RawMessage(`{}`)},
		"",
		harness.ModelAnswer{
			Result:           &mcp.CallToolResult{StructuredContent: make(chan int), Content: []mcp.Content{&mcp.TextContent{Text: "answered"}}},
			Outcome:          modelrecord.OutcomeOK,
			Text:             "answered",
			DispatchObserved: true,
		})

	if line.Result != nil {
		t.Errorf("Result = %s, want nothing for content that cannot be encoded", line.Result)
	}
	if line.Text != "answered" {
		t.Errorf("Text = %q, want the text the model read", line.Text)
	}
}

// TestCallLine_SpanFactsTheRecordHasNoFieldFor_AreLeftOut checks that the
// three attributes the record does not carry stay out of it.
//
// The dispatch facts are the whole of what the span said, deliberately: the
// point of reading the span is that it is the server's account and not the
// client's. The record's call line has a field for the tool, the action and
// the refusal reason and for nothing else, and a domain or an error type
// smuggled into one of those would be a value a scorer would read as the field
// it was written in.
func TestCallLine_SpanFactsTheRecordHasNoFieldFor_AreLeftOut(t *testing.T) {
	facts := harness.DispatchFacts{
		Tool:          "gitlab_execute_action",
		Action:        "issue.create",
		Domain:        "issue",
		RefusalReason: "",
		ErrorType:     "*errors.errorString",
		Status:        "STATUS_CODE_ERROR",
	}
	line := callLine(modelPosition,
		harness.ModelCall{Tool: facts.Tool, Arguments: json.RawMessage(`{}`)},
		"issue.create",
		harness.ModelAnswer{
			Result:           &mcp.CallToolResult{IsError: true},
			Outcome:          modelrecord.OutcomeToolError,
			Failure:          harness.FailureToolError,
			Text:             "404 Project Not Found",
			Dispatch:         facts,
			DispatchObserved: true,
		})

	for _, absent := range []string{facts.Domain, facts.ErrorType, facts.Status} {
		t.Run("absent "+absent, func(t *testing.T) {
			for name, value := range map[string]string{
				"DispatchedTool":   line.DispatchedTool,
				"DispatchedAction": line.DispatchedAction,
				"RefusalReason":    line.RefusalReason,
				"Outcome":          line.Outcome,
				"Text":             line.Text,
			} {
				if value == absent {
					t.Errorf("%s = %q, which is the span's own %q and belongs to no field of this line",
						name, value, absent)
				}
			}
		})
	}
	if line.Outcome != modelrecord.OutcomeToolError {
		t.Errorf("Outcome = %q, want %q: a correct dispatch that GitLab refused is its own class and never a "+
			"model failure", line.Outcome, modelrecord.OutcomeToolError)
	}
}
