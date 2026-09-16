//go:build e2e

package provider

import (
	"encoding/json"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelrecord"
)

// thinkingSignature is the signature the first answer's thinking block carries.
//
// It is in the fixture because it is the thing this exchange is about: this API
// signs a thinking block and validates the signature on the next request, and
// an adapter that re-marshals the block through [anthropicBlock], which has no
// field for it, drops the signature while every one-turn assertion still
// passes. A fixture with no signature in it cannot tell the two apart.
const thinkingSignature = "SIG-ABC-123"

// TestAnthropic_TwoTurns is the exchange this adapter actually has to survive.
//
// One request is not a contract check: every adapter's failure surface is the
// second turn, where the model's own call and the server's answer to it have to
// go back in the provider's own shape. This drives both, and asserts the second
// request carries the thinking block with its signature and the tool_result
// block addressed to the call by id, which is where the knowledge the old
// adapter kept in one function lived.
func TestAnthropic_TwoTurns(t *testing.T) {
	backend := newRecorder(
		`{"content":[{"type":"thinking","thinking":"which tool","signature":"`+thinkingSignature+`"},
		  {"type":"tool_use","id":"toolu_1","name":"gitlab_execute_action",
		   "input":{"action":"issue.list","params":{"project_id":7}}}],
		  "usage":{"input_tokens":120,"output_tokens":30,
		           "cache_creation_input_tokens":11,"cache_read_input_tokens":90}}`,
		`{"content":[{"type":"text","text":"There are two open issues."}],
		  "usage":{"input_tokens":200,"output_tokens":12}}`,
	)
	adapter := adapterFor(t, "anthropic:claude-haiku-4-5-20251001;max_tokens=512", backend.server(t))
	tools := sampleTools(t)

	first, err := adapter.Call(t.Context(), Request{
		Tools:    tools,
		System:   "You are driving a GitLab MCP server.",
		Messages: []Message{{Role: RoleUser, Text: "List the open issues."}},
	})
	if err != nil {
		t.Fatalf("the first turn: %v", err)
	}
	calls := first.ToolCalls()
	if len(calls) != 1 || calls[0].Tool != "gitlab_execute_action" || calls[0].CallID != "toolu_1" {
		t.Fatalf("the first turn returned %+v, want one call of gitlab_execute_action", first.Blocks)
	}
	if first.Blocks[0].Kind != modelrecord.BlockThinking {
		t.Errorf("the thinking block was dropped: %+v", first.Blocks[0])
	}
	if first.Usage != (modelrecord.Usage{Input: 120, Output: 30, CacheCreated: 11, CacheRead: 90}) {
		t.Errorf("Usage = %+v, want all four numbers as the provider reported them", first.Usage)
	}
	if first.Status != modelrecord.TurnOK || first.RequestDigest == "" || len(first.ResponseBody) == 0 {
		t.Errorf("the turn kept no status, digest or body: %+v", first)
	}

	assertAnthropicRequest(t, backend.sent(t, 0))

	// The second turn: the call goes back with the server's answer to it.
	second, err := adapter.Call(t.Context(), Request{
		Tools:  tools,
		System: "You are driving a GitLab MCP server.",
		Messages: []Message{
			{Role: RoleUser, Text: "List the open issues."},
			{Role: RoleAssistant, Blocks: first.Blocks, Echo: first.Echo},
			{Role: RoleTool, Results: []ToolResult{{
				CallID: "toolu_1", Tool: "gitlab_execute_action", Content: "| IID | Title |\n| 1 | First |",
			}}},
		},
	})
	if err != nil {
		t.Fatalf("the second turn: %v", err)
	}
	if len(second.ToolCalls()) != 0 || second.Blocks[0].Kind != modelrecord.BlockText {
		t.Errorf("the second turn returned %+v, want the model's prose", second.Blocks)
	}

	assertAnthropicSecondRequest(t, backend.sent(t, 1))
}

// assertAnthropicRequest checks the shape of a first request.
func assertAnthropicRequest(t *testing.T, sent map[string]any) {
	t.Helper()
	if sent["max_tokens"] != float64(512) {
		t.Errorf("max_tokens = %v, want 512", sent["max_tokens"])
	}
	if sent["temperature"] != float64(0) {
		t.Errorf("temperature = %v, want 0", sent["temperature"])
	}
	if sent["system"] != "You are driving a GitLab MCP server." {
		t.Errorf("system = %v", sent["system"])
	}
	if _, forced := sent["tool_choice"]; forced {
		t.Error("the request forces a tool call, so a model that should decline could not")
	}
}

// assertAnthropicSecondRequest checks that the model's own call and the answer
// to it went back in this API's shape.
func assertAnthropicSecondRequest(t *testing.T, next map[string]any) {
	t.Helper()
	messages, _ := next["messages"].([]any)
	if len(messages) != 3 {
		t.Fatalf("the second request carries %d messages, want 3: %v", len(messages), next["messages"])
	}

	assistant, _ := messages[1].(map[string]any)
	if assistant["role"] != RoleAssistant {
		t.Errorf("the second message is %v, want the assistant turn", assistant["role"])
	}
	// The assistant turn is echoed, thinking block included: this provider
	// signs one and refuses the next request when the signature is missing.
	assistantBlocks, _ := assistant["content"].([]any)
	if len(assistantBlocks) != 2 {
		t.Fatalf("the echoed assistant turn carries %d blocks, want the thinking block and the call",
			len(assistantBlocks))
	}
	thinking, _ := assistantBlocks[0].(map[string]any)
	if thinking["signature"] != thinkingSignature {
		t.Errorf("the echoed thinking block is %v; this API validates the signature on the next "+
			"request, so a turn that goes back without it is refused", thinking)
	}

	result, _ := messages[2].(map[string]any)
	if result["role"] != RoleUser {
		t.Errorf("the tool result travels as %v, want a user message on this API", result["role"])
	}
	resultBlocks, _ := result["content"].([]any)
	block, _ := resultBlocks[0].(map[string]any)
	if block["type"] != "tool_result" || block["tool_use_id"] != "toolu_1" {
		t.Errorf("the tool result block is %v, want a tool_result addressed to toolu_1", block)
	}
}

func TestAnthropic_SendsTheSchemasItWasGiven(t *testing.T) {
	backend := newRecorder(`{"content":[],"usage":{}}`)
	adapter := adapterFor(t, "anthropic:claude-haiku-4-5-20251001", backend.server(t))
	tools := sampleTools(t)

	if _, err := adapter.Call(t.Context(), Request{Tools: tools}); err != nil {
		t.Fatalf("Call: %v", err)
	}
	sent := backend.sent(t, 0)
	wire, _ := sent["tools"].([]any)
	if len(wire) != len(tools) {
		t.Fatalf("the request carries %d tools, want %d", len(wire), len(tools))
	}
	first, _ := wire[0].(map[string]any)
	schema, err := json.Marshal(first["input_schema"])
	if err != nil {
		t.Fatalf("re-encoding the sent schema: %v", err)
	}
	if !sameJSON(t, schema, tools[0].Schema) {
		t.Errorf("the schema sent is %s, want the one it was given %s", schema, tools[0].Schema)
	}
}

func TestAnthropic_AnAnswerWithNoInputIsAMalformedCallAndIsNotRepaired(t *testing.T) {
	backend := newRecorder(
		`{"content":[{"type":"tool_use","id":"toolu_1","name":"gitlab_execute_action"}],"usage":{}}`,
	)
	adapter := adapterFor(t, "anthropic:claude-haiku-4-5-20251001", backend.server(t))

	answer, err := adapter.Call(t.Context(), Request{})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !answer.Malformed() {
		t.Fatalf("a call with no input is not reported malformed: %+v", answer.Blocks)
	}
	if backend.count() != 1 {
		t.Errorf("the backend saw %d requests: a malformed call was retried", backend.count())
	}
}

func TestAnthropic_ReportsTheProvidersOwnErrorObject(t *testing.T) {
	// A 200 carrying an error object is a shape this API has; a request that
	// read it as an answer would record a turn with no blocks and no reason.
	backend := newRecorder(`{"error":{"type":"overloaded_error","message":"try later"}}`)
	adapter := adapterFor(t, "anthropic:claude-haiku-4-5-20251001", backend.server(t))

	answer, err := adapter.Call(t.Context(), Request{})
	if err == nil {
		t.Fatal("an error object was read as an answer")
	}
	if answer.Status != modelrecord.TurnRequestError {
		t.Errorf("Status = %q, want %q", answer.Status, modelrecord.TurnRequestError)
	}
}

func TestAnthropic_ReportsAContentThatIsNotBlocksAndEchoesNothingWithoutOne(t *testing.T) {
	// The content is read twice, as bytes to echo and as blocks to record, so
	// each reading needs its own answer: one that is not a block list at all,
	// and one that is not there.
	t.Run("a content that is not a list", func(t *testing.T) {
		backend := newRecorder(`{"content":{"type":"text","text":"ok"},"usage":{}}`)
		adapter := adapterFor(t, "anthropic:claude-haiku-4-5-20251001", backend.server(t))

		answer, err := adapter.Call(t.Context(), Request{})
		if err == nil {
			t.Fatal("a content that is not a block list was read as a turn")
		}
		if answer.Status != modelrecord.TurnServerError {
			t.Errorf("Status = %q, want %q", answer.Status, modelrecord.TurnServerError)
		}
	})

	t.Run("no content at all", func(t *testing.T) {
		backend := newRecorder(`{"usage":{"input_tokens":5}}`)
		adapter := adapterFor(t, "anthropic:claude-haiku-4-5-20251001", backend.server(t))

		answer, err := adapter.Call(t.Context(), Request{})
		if err != nil {
			t.Fatalf("Call: %v", err)
		}
		if len(answer.Blocks) != 0 || len(answer.Echo) != 0 {
			t.Errorf("an answer with no content produced %+v and the echo %s; an echo of nothing "+
				"would be handed back as a turn saying nothing", answer.Blocks, answer.Echo)
		}
	})
}

func TestAnthropic_ReportsAnAnswerThatIsNotJSON(t *testing.T) {
	backend := newRecorder(`<html>maintenance</html>`)
	adapter := adapterFor(t, "anthropic:claude-haiku-4-5-20251001", backend.server(t))

	answer, err := adapter.Call(t.Context(), Request{})
	if err == nil {
		t.Fatal("a body that is not JSON was read as an answer")
	}
	if answer.Status != modelrecord.TurnServerError {
		t.Errorf("Status = %q, want %q", answer.Status, modelrecord.TurnServerError)
	}
	if len(answer.ResponseBody) != 0 {
		t.Error("a body that is not JSON was kept in a field the record encodes as JSON")
	}
}

func TestAnthropicMessages_RebuildsATurnWhenThereIsNoEchoToHandBack(t *testing.T) {
	// The echo is what a live conversation carries. A conversation assembled
	// from the record has blocks and no echo, and must still be sendable.
	built := anthropicMessages([]Message{
		{Role: RoleUser, Text: "do it"},
		{Role: RoleAssistant, Text: "calling", Blocks: []modelrecord.Block{{
			Kind: modelrecord.BlockToolCall, Tool: "t", CallID: "c1", Arguments: json.RawMessage(`{"a":1}`),
		}}},
		{Role: RoleTool, Results: []ToolResult{{CallID: "c1", Content: "done", IsError: true}}},
		{Role: RoleAssistant, Echo: json.RawMessage(`not json`)},
		{Role: RoleUser},
	})
	if len(built) != 3 {
		t.Fatalf("built %d messages, want 3: an empty one is dropped rather than sent", len(built))
	}
	assistant := anthropicBuiltBlocks(t, built[1])
	if assistant[0].Type != "text" || assistant[1].Type != "tool_use" {
		t.Errorf("the rebuilt assistant turn is %+v", assistant)
	}
	if !anthropicBuiltBlocks(t, built[2])[0].IsError {
		t.Error("a failed tool result was fed back as a successful one")
	}
}

// anthropicBuiltBlocks reads back the blocks of a turn this package assembled.
//
// A turn carrying an echo has bytes rather than blocks, which is the whole
// point of [wireValue] and is why this says so rather than returning nothing.
func anthropicBuiltBlocks(t *testing.T, message anthropicMessage) []anthropicBlock {
	t.Helper()
	blocks, built := message.Content.built.([]anthropicBlock)
	if !built {
		t.Fatalf("the turn carries %T, want the blocks this package assembled", message.Content.built)
	}
	return blocks
}

// sameJSON reports whether two documents are the same value.
func sameJSON(t *testing.T, left, right []byte) bool {
	t.Helper()
	return canonicalJSON(t, left) == canonicalJSON(t, right)
}

// canonicalJSON re-encodes one document so two spellings of one value compare
// equal.
func canonicalJSON(t *testing.T, document []byte) string {
	t.Helper()
	var decoded any
	if err := json.Unmarshal(document, &decoded); err != nil {
		t.Fatalf("%s is not JSON: %v", document, err)
	}
	encoded, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("re-encoding %s: %v", document, err)
	}
	return string(encoded)
}
