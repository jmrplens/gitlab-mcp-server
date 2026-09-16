//go:build e2e

package provider

import (
	"encoding/json"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelrecord"
)

// TestOpenAI_TwoTurns drives the exchange both providers of this shape have.
//
// The second turn is the point: a tool result is a message of its own with the
// tool role here, addressed by tool_call_id, and the assistant turn that
// preceded it has to carry the tool_calls array the result answers. An adapter
// that got either wrong would fail on every attempt past the first, which is
// every attempt the corpus actually holds.
func TestOpenAI_TwoTurns(t *testing.T) {
	backend := newRecorder(
		`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call_1","type":"function",
		   "function":{"name":"gitlab_execute_action",
		               "arguments":"{\"action\":\"issue.list\",\"params\":{\"project_id\":7}}"}}]}}],
		  "usage":{"prompt_tokens":300,"completion_tokens":40,"prompt_tokens_details":{"cached_tokens":250}}}`,
		`{"choices":[{"message":{"role":"assistant","content":"There are two open issues."}}],
		  "usage":{"prompt_tokens":420,"completion_tokens":9}}`,
	)
	adapter := adapterFor(t, "openai:gpt-5.4-nano;max_tokens=256", backend.server(t))
	tools := sampleTools(t)

	first, err := adapter.Call(t.Context(), Request{
		Tools:    tools,
		System:   "contract",
		Messages: []Message{{Role: RoleUser, Text: "List the open issues."}},
	})
	if err != nil {
		t.Fatalf("the first turn: %v", err)
	}
	calls := first.ToolCalls()
	if len(calls) != 1 || calls[0].CallID != "call_1" {
		t.Fatalf("the first turn returned %+v, want one call", first.Blocks)
	}
	if string(calls[0].Arguments) != `{"action":"issue.list","params":{"project_id":7}}` {
		t.Errorf("the arguments were rewritten: %s", calls[0].Arguments)
	}
	// The cached tokens are inside this API's prompt total, so the uncached
	// input is the difference. Reporting the total as input would bill the
	// same tokens twice in every cost column derived from the record.
	if first.Usage != (modelrecord.Usage{Input: 50, Output: 40, CacheRead: 250}) {
		t.Errorf("Usage = %+v, want the cached tokens taken out of the input", first.Usage)
	}

	sent := backend.sent(t, 0)
	if sent["max_completion_tokens"] != float64(256) {
		t.Errorf("max_completion_tokens = %v, want 256", sent["max_completion_tokens"])
	}
	if _, present := sent["max_tokens"]; present {
		t.Error("the request carries max_tokens, which the reasoning models refuse")
	}
	if _, forced := sent["tool_choice"]; forced {
		t.Error("the request forces a tool call, so a model that should decline could not")
	}
	messages, _ := sent["messages"].([]any)
	system, _ := messages[0].(map[string]any)
	if system["role"] != "system" || system["content"] != "contract" {
		t.Errorf("the first message is %v, want the contract as the system message", messages[0])
	}

	second, err := adapter.Call(t.Context(), Request{
		Tools:  tools,
		System: "contract",
		Messages: []Message{
			{Role: RoleUser, Text: "List the open issues."},
			{Role: RoleAssistant, Blocks: first.Blocks, Echo: first.Echo},
			{Role: RoleTool, Results: []ToolResult{{CallID: "call_1", Content: "| IID |"}}},
		},
	})
	if err != nil {
		t.Fatalf("the second turn: %v", err)
	}
	if len(second.ToolCalls()) != 0 || second.Blocks[0].Text != "There are two open issues." {
		t.Errorf("the second turn returned %+v", second.Blocks)
	}

	next := backend.sent(t, 1)
	nextMessages, _ := next["messages"].([]any)
	if len(nextMessages) != 4 {
		t.Fatalf("the second request carries %d messages, want the system, the prompt, the call and "+
			"the result: %v", len(nextMessages), next["messages"])
	}
	assistant, _ := nextMessages[2].(map[string]any)
	toolCalls, _ := assistant["tool_calls"].([]any)
	if len(toolCalls) != 1 {
		t.Fatalf("the echoed assistant turn carries %d tool calls, want 1", len(toolCalls))
	}
	result, _ := nextMessages[3].(map[string]any)
	if result["role"] != RoleTool || result["tool_call_id"] != "call_1" {
		t.Errorf("the tool result is %v, want a tool message addressed to call_1", result)
	}
}

func TestOpenAI_SendsTheSchemasItWasGivenAndNothingItInvented(t *testing.T) {
	// This is finding F8 at the wire: the adapter this replaces added
	// thirty-six parameter names, additionalProperties and a required list to
	// the execute tool's schema, for this provider and Qwen only.
	backend := newRecorder(`{"choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{}}`)
	adapter := adapterFor(t, "openai:gpt-5.4-nano", backend.server(t))
	tools := sampleTools(t)

	if _, err := adapter.Call(t.Context(), Request{Tools: tools}); err != nil {
		t.Fatalf("Call: %v", err)
	}
	sent := backend.sent(t, 0)
	wire, _ := sent["tools"].([]any)
	if len(wire) != len(tools) {
		t.Fatalf("the request carries %d tools, want %d", len(wire), len(tools))
	}
	for index, entry := range wire {
		tool, _ := entry.(map[string]any)
		function, _ := tool["function"].(map[string]any)
		schema, err := json.Marshal(function["parameters"])
		if err != nil {
			t.Fatalf("re-encoding the sent schema: %v", err)
		}
		if !sameJSON(t, schema, tools[index].Schema) {
			t.Errorf("the schema of %v was rewritten on its way out:\n sent %s\n want %s",
				function["name"], schema, tools[index].Schema)
		}
	}
}

func TestOpenAI_MalformedArgumentsAreKeptAsTheyArrivedAndNotRepaired(t *testing.T) {
	// Every one of these is a shape the old adapter repaired, in five layers,
	// and then retried up to four times before re-running the whole task. A
	// model that could not emit a parseable call therefore scored as one that
	// could, which is finding F11.
	for _, one := range []struct{ name, arguments string }{
		{"an unclosed object", `{\"action\": \"issue.list\"`},
		{"prose around it", `<tool_call>{\"action\":\"issue.list\"}</tool_call>`},
		{"a fragment with no braces", `\"action\": \"issue.list\"`},
		{"nothing at all", ``},
		{"a value that is not an object", `[1,2]`},
	} {
		t.Run(one.name, func(t *testing.T) {
			backend := newRecorder(`{"choices":[{"message":{"role":"assistant","tool_calls":[
			  {"id":"call_1","type":"function","function":{"name":"gitlab_execute_action",
			   "arguments":"` + one.arguments + `"}}]}}],"usage":{}}`)
			adapter := adapterFor(t, "openai:gpt-5.4-nano", backend.server(t))

			answer, err := adapter.Call(t.Context(), Request{})
			if err != nil {
				t.Fatalf("Call: %v", err)
			}
			if !answer.Malformed() {
				t.Fatalf("%q was accepted as a well-formed call: %+v", one.arguments, answer.Blocks)
			}
			call := answer.ToolCalls()[0]
			if len(call.Arguments) != 0 {
				t.Errorf("the malformed call carries arguments %s, so something parsed it", call.Arguments)
			}
			if call.Raw == "" && one.arguments != "" {
				t.Error("the malformed text was not kept, so nothing can say what the model wrote")
			}
			if backend.count() != 1 {
				t.Errorf("the backend saw %d requests: the call was retried", backend.count())
			}
		})
	}
}

func TestQwen_SendsItsOwnCeilingFieldAndItsThinkingSwitch(t *testing.T) {
	backend := newRecorder(`{"choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{}}`)
	adapter := adapterFor(t, "qwen:qwen3.8-flash;max_tokens=128;enable_thinking=false", backend.server(t))

	if _, err := adapter.Call(t.Context(), Request{Tools: sampleTools(t)}); err != nil {
		t.Fatalf("Call: %v", err)
	}
	sent := backend.sent(t, 0)
	if sent["max_tokens"] != float64(128) {
		t.Errorf("max_tokens = %v, want 128: this endpoint does not take OpenAI's newer field",
			sent["max_tokens"])
	}
	if _, present := sent["max_completion_tokens"]; present {
		t.Error("the request carries max_completion_tokens, which this endpoint ignores")
	}
	if sent["enable_thinking"] != false {
		t.Errorf("enable_thinking = %v, want false", sent["enable_thinking"])
	}
	if _, present := sent["reasoning_effort"]; present {
		t.Error("the request carries a field this provider was never configured with")
	}
}

func TestOpenAI_SendsAReasoningEffortWithoutATemperature(t *testing.T) {
	backend := newRecorder(`{"choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{}}`)
	adapter := adapterFor(t, "openai:gpt-5.6-luna;reasoning_effort=none", backend.server(t))

	if _, err := adapter.Call(t.Context(), Request{}); err != nil {
		t.Fatalf("Call: %v", err)
	}
	sent := backend.sent(t, 0)
	if sent["reasoning_effort"] != "none" {
		t.Errorf("reasoning_effort = %v, want none, which is what this family needs to accept "+
			"function tools at all", sent["reasoning_effort"])
	}
	if _, present := sent["temperature"]; present {
		t.Error("the request carries a temperature, which this family refuses")
	}
}

func TestOpenAI_ReportsAnAnswerWithNoChoices(t *testing.T) {
	backend := newRecorder(`{"choices":[],"usage":{}}`)
	adapter := adapterFor(t, "openai:gpt-5.4-nano", backend.server(t))

	answer, err := adapter.Call(t.Context(), Request{})
	if err == nil {
		t.Fatal("an answer with no choices was read as a turn")
	}
	if answer.Status != modelrecord.TurnServerError {
		t.Errorf("Status = %q, want %q", answer.Status, modelrecord.TurnServerError)
	}
}

func TestOpenAI_ReportsTheProvidersOwnErrorObject(t *testing.T) {
	backend := newRecorder(`{"error":{"type":"invalid_request_error","message":"no such model"}}`)
	adapter := adapterFor(t, "openai:gpt-5.4-nano", backend.server(t))

	answer, err := adapter.Call(t.Context(), Request{})
	if err == nil {
		t.Fatal("an error object was read as an answer")
	}
	if answer.Status != modelrecord.TurnRequestError {
		t.Errorf("Status = %q, want %q", answer.Status, modelrecord.TurnRequestError)
	}
}

func TestOpenAIMessages_RebuildsATurnWhenThereIsNoEchoToHandBack(t *testing.T) {
	built := openAIMessages(Message{
		Role: RoleAssistant,
		Text: "first",
		Blocks: []modelrecord.Block{
			{Kind: modelrecord.BlockText, Text: "second"},
			{Kind: modelrecord.BlockToolCall, Tool: "t", CallID: "c1", Arguments: json.RawMessage(`{"a":1}`)},
		},
	})
	if len(built) != 1 {
		t.Fatalf("built %d messages, want 1", len(built))
	}
	if built[0].Content != "first\nsecond" {
		t.Errorf("content = %q, want both pieces of prose", built[0].Content)
	}
	if len(built[0].ToolCalls) != 1 || built[0].ToolCalls[0].Function.Arguments != `{"a":1}` {
		t.Errorf("tool calls = %+v", built[0].ToolCalls)
	}

	// An echo that does not decode falls back to the blocks rather than
	// sending nothing, which is what a record-assembled conversation needs.
	fallback := openAIMessages(Message{Role: RoleAssistant, Echo: json.RawMessage(`"not a message"`)})
	if len(fallback) != 1 || fallback[0].Role != RoleAssistant {
		t.Errorf("an undecodable echo produced %+v", fallback)
	}
}
