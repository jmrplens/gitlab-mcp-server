//go:build e2e

package provider

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelrecord"
)

// TestGoogle_TwoTurns drives the third of the three second-turn shapes.
//
// A tool result here is a functionResponse part inside a user content, named by
// the function it answers rather than only by an identifier, and the assistant
// turn it follows carries a thought part and a thoughtSignature this provider
// refuses the next request without. All three are asserted, because they are
// invisible to a one-turn contract check and they are what this adapter exists
// to get right. The thought marker is in the fixture for the reason the
// Anthropic signature is: [googlePart] has no field for it, so an adapter that
// re-marshals the turn hands the model's own reasoning back as ordinary model
// text and nothing local notices.
func TestGoogle_TwoTurns(t *testing.T) {
	backend := newRecorder(
		`{"candidates":[{"content":{"role":"model","parts":[
		   {"text":"which tool fits","thought":true},
		   {"thoughtSignature":"sig-1","functionCall":{"id":"fc_1","name":"gitlab_execute_action",
		    "args":{"action":"issue.list","params":{"project_id":7}}}}]}}],
		  "usageMetadata":{"promptTokenCount":500,"candidatesTokenCount":20,
		                   "cachedContentTokenCount":400,"thoughtsTokenCount":15}}`,
		`{"candidates":[{"content":{"role":"model","parts":[{"text":"There are two open issues."}]}}],
		  "usageMetadata":{"promptTokenCount":600,"candidatesTokenCount":8}}`,
	)
	adapter := adapterFor(t, "google:gemini-flash-latest;max_tokens=256", backend.server(t))
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
	if len(calls) != 1 || calls[0].CallID != "fc_1" || calls[0].Tool != "gitlab_execute_action" {
		t.Fatalf("the first turn returned %+v, want one call", first.Blocks)
	}
	// The candidates count excludes the thinking this model was billed for,
	// and the prompt count includes what was served from the cache, so
	// neither is the number a cost column wants as it stands.
	if first.Usage != (modelrecord.Usage{Input: 100, Output: 35, CacheRead: 400}) {
		t.Errorf("Usage = %+v, want the thinking counted as output and the cache taken out of the input",
			first.Usage)
	}

	sent := backend.sent(t, 0)
	if _, forced := sent["tool_config"]; forced {
		t.Error("the request forces a function call, so a model that should decline could not")
	}
	generation, _ := sent["generation_config"].(map[string]any)
	if generation["max_output_tokens"] != float64(256) || generation["temperature"] != float64(0) {
		t.Errorf("generation_config = %v", generation)
	}
	system, _ := sent["system_instruction"].(map[string]any)
	systemParts, _ := system["parts"].([]any)
	firstPart, _ := systemParts[0].(map[string]any)
	if firstPart["text"] != "contract" {
		t.Errorf("system_instruction = %v, want the contract", system)
	}

	second, err := adapter.Call(t.Context(), Request{
		Tools:  tools,
		System: "contract",
		Messages: []Message{
			{Role: RoleUser, Text: "List the open issues."},
			{Role: RoleAssistant, Blocks: first.Blocks, Echo: first.Echo},
			{Role: RoleTool, Results: []ToolResult{{CallID: "fc_1", Content: "| IID |"}}},
		},
	})
	if err != nil {
		t.Fatalf("the second turn: %v", err)
	}
	if len(second.ToolCalls()) != 0 || second.Blocks[0].Text != "There are two open issues." {
		t.Errorf("the second turn returned %+v", second.Blocks)
	}

	assertGoogleSecondRequest(t, backend.sent(t, 1))
}

// assertGoogleSecondRequest checks that the model's own turn and the answer to
// it went back in this API's shape: the turn under the model role with the
// parts exactly as they arrived, and the result as a functionResponse naming
// the function it answers.
func assertGoogleSecondRequest(t *testing.T, next map[string]any) {
	t.Helper()
	contents, _ := next["contents"].([]any)
	if len(contents) != 3 {
		t.Fatalf("the second request carries %d contents, want 3: %v", len(contents), next["contents"])
	}

	model, _ := contents[1].(map[string]any)
	if model["role"] != roleModel {
		t.Errorf("the assistant turn travels as %v, want the model role", model["role"])
	}
	modelParts, _ := model["parts"].([]any)
	if len(modelParts) != 2 {
		t.Fatalf("the echoed turn carries %d parts, want the thought and the call: %v",
			len(modelParts), modelParts)
	}
	thought, _ := modelParts[0].(map[string]any)
	if thought["thought"] != true {
		t.Errorf("the thought marker was dropped: %v; the part goes back as ordinary model text, "+
			"which is not what the model wrote", thought)
	}
	call, _ := modelParts[1].(map[string]any)
	if call["thoughtSignature"] != "sig-1" {
		t.Errorf("the thought signature was dropped: %v; this provider refuses the next request without it",
			call)
	}

	answer, _ := contents[2].(map[string]any)
	if answer["role"] != RoleUser {
		t.Errorf("the tool result travels as %v, want a user content on this API", answer["role"])
	}
	answerParts, _ := answer["parts"].([]any)
	part, _ := answerParts[0].(map[string]any)
	response, _ := part["functionResponse"].(map[string]any)
	if response["name"] != "gitlab_execute_action" {
		t.Errorf("the functionResponse names %v, want the function it answers", response["name"])
	}
	if response["id"] != "fc_1" {
		t.Errorf("the functionResponse is addressed to %v, want fc_1", response["id"])
	}
}

func TestGoogle_SendsTheSchemasItWasGivenWithoutSanitizingThemItself(t *testing.T) {
	// This adapter used to strip $schema and additionalProperties on its way
	// out, because this API refuses them. The removal now happens once in
	// NewTool, before any adapter sees the list, which is what makes the four
	// digests of one list agree.
	backend := newRecorder(`{"candidates":[{"content":{"parts":[{"text":"ok"}]}}],"usageMetadata":{}}`)
	adapter := adapterFor(t, "google:gemini-flash-latest", backend.server(t))
	tools := sampleTools(t)

	if _, err := adapter.Call(t.Context(), Request{Tools: tools}); err != nil {
		t.Fatalf("Call: %v", err)
	}
	sent := backend.sent(t, 0)
	wire, _ := sent["tools"].([]any)
	if len(wire) != 1 {
		t.Fatalf("the request carries %d tool entries, want the one this API takes", len(wire))
	}
	entry, _ := wire[0].(map[string]any)
	declarations, _ := entry["function_declarations"].([]any)
	if len(declarations) != len(tools) {
		t.Fatalf("the entry carries %d declarations, want %d", len(declarations), len(tools))
	}
	for index, declared := range declarations {
		declaration, _ := declared.(map[string]any)
		schema, err := json.Marshal(declaration["parameters"])
		if err != nil {
			t.Fatalf("re-encoding the sent schema: %v", err)
		}
		if !sameJSON(t, schema, tools[index].Schema) {
			t.Errorf("the schema of %v was rewritten on its way out:\n sent %s\n want %s",
				declaration["name"], schema, tools[index].Schema)
		}
		if strings.Contains(string(schema), "$schema") {
			t.Errorf("the schema of %v still carries a keyword this API refuses", declaration["name"])
		}
	}
}

func TestGoogle_NamesTheModelInThePath(t *testing.T) {
	backend := newRecorder(`{"candidates":[{"content":{"parts":[{"text":"ok"}]}}],"usageMetadata":{}}`)
	endpoint := backend.server(t) + "/v1beta/models"
	adapter := adapterFor(t, "google:gemini-flash-latest", endpoint)

	if _, err := adapter.Call(t.Context(), Request{}); err != nil {
		t.Fatalf("Call: %v", err)
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if want := "/v1beta/models/gemini-flash-latest:generateContent"; backend.paths[0] != want {
		t.Errorf("path = %q, want %q", backend.paths[0], want)
	}
	if backend.headers[0].Get("x-goog-api-key") != testKey {
		t.Error("the credential was not sent in the header this API reads")
	}
}

func TestGoogle_NamesEveryCallEvenWhenTheProviderDoesNot(t *testing.T) {
	// A model that mints no identifier would otherwise leave two calls of one
	// turn sharing the empty one, and every tool result would answer the
	// first.
	backend := newRecorder(`{"candidates":[{"content":{"parts":[
	  {"functionCall":{"name":"a","args":{}}},
	  {"functionCall":{"name":"b","args":{}}}]}}],"usageMetadata":{}}`)
	adapter := adapterFor(t, "google:gemini-flash-latest", backend.server(t))

	answer, err := adapter.Call(t.Context(), Request{})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	calls := answer.ToolCalls()
	if len(calls) != 2 {
		t.Fatalf("the turn returned %d calls, want 2", len(calls))
	}
	if calls[0].CallID == calls[1].CallID {
		t.Errorf("both calls are named %q", calls[0].CallID)
	}
}

func TestGoogle_AnAnswerWithNoArgumentsIsAMalformedCall(t *testing.T) {
	backend := newRecorder(
		`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"a"}}]}}],"usageMetadata":{}}`,
	)
	adapter := adapterFor(t, "google:gemini-flash-latest", backend.server(t))

	answer, err := adapter.Call(t.Context(), Request{})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !answer.Malformed() {
		t.Errorf("a call with no args was accepted as well formed: %+v", answer.Blocks)
	}
}

func TestGoogle_ReportsAnAnswerWithNoCandidateAndSaysWhy(t *testing.T) {
	backend := newRecorder(`{"candidates":[],"promptFeedback":{"blockReason":"SAFETY"}}`)
	adapter := adapterFor(t, "google:gemini-flash-latest", backend.server(t))

	answer, err := adapter.Call(t.Context(), Request{})
	if err == nil {
		t.Fatal("an answer with no candidate was read as a turn")
	}
	if answer.Status != modelrecord.TurnServerError {
		t.Errorf("Status = %q, want %q", answer.Status, modelrecord.TurnServerError)
	}
	if !strings.Contains(answer.Detail, "SAFETY") {
		t.Errorf("the detail does not carry the only explanation there was: %q", answer.Detail)
	}
}

func TestGoogle_ReportsAContentThatDoesNotDecodeAndEchoesNothingWithoutOne(t *testing.T) {
	// The candidate's content is read twice, as bytes to echo and as parts to
	// record, so each reading needs its own answer.
	t.Run("a content that is not an object", func(t *testing.T) {
		backend := newRecorder(`{"candidates":[{"content":"ok"}],"usageMetadata":{}}`)
		adapter := adapterFor(t, "google:gemini-flash-latest", backend.server(t))

		answer, err := adapter.Call(t.Context(), Request{})
		if err == nil {
			t.Fatal("a content that is not an object was read as a turn")
		}
		if answer.Status != modelrecord.TurnServerError {
			t.Errorf("Status = %q, want %q", answer.Status, modelrecord.TurnServerError)
		}
	})

	t.Run("a candidate with no content", func(t *testing.T) {
		backend := newRecorder(`{"candidates":[{"finishReason":"STOP"}],"usageMetadata":{}}`)
		adapter := adapterFor(t, "google:gemini-flash-latest", backend.server(t))

		answer, err := adapter.Call(t.Context(), Request{})
		if err != nil {
			t.Fatalf("Call: %v", err)
		}
		if len(answer.Blocks) != 0 || len(answer.Echo) != 0 {
			t.Errorf("a candidate with no content produced %+v and the echo %s", answer.Blocks, answer.Echo)
		}
	})
}

func TestGoogle_ReportsTheProvidersOwnErrorObject(t *testing.T) {
	backend := newRecorder(`{"error":{"status":"INVALID_ARGUMENT","message":"unknown name"}}`)
	adapter := adapterFor(t, "google:gemini-flash-latest", backend.server(t))

	answer, err := adapter.Call(t.Context(), Request{})
	if err == nil {
		t.Fatal("an error object was read as an answer")
	}
	if answer.Status != modelrecord.TurnRequestError || !strings.Contains(answer.Detail, "INVALID_ARGUMENT") {
		t.Errorf("Status = %q, Detail = %q", answer.Status, answer.Detail)
	}
}

func TestGoogleContents_RebuildsATurnWhenThereIsNoEchoToHandBack(t *testing.T) {
	built := googleContents([]Message{
		{Role: RoleUser, Text: "do it"},
		{Role: RoleAssistant, Blocks: []modelrecord.Block{{
			Kind: modelrecord.BlockToolCall, Tool: "t", CallID: "c1", Arguments: json.RawMessage(`{"a":1}`),
		}}},
		{Role: RoleTool, Results: []ToolResult{{CallID: "c1", Content: "done"}}},
		{Role: RoleAssistant, Echo: json.RawMessage(`{"parts":[]}`)},
		// An echo that is not a content, and one whose parts are not parts, are
		// both rebuilt rather than sent as whatever they are: the bytes go back
		// untouched, so the check on the way in is the only one there will be.
		{Role: RoleAssistant, Echo: json.RawMessage(`["which tool"]`)},
		{Role: RoleAssistant, Echo: json.RawMessage(`{"parts":"which tool"}`)},
		{Role: RoleUser},
	})
	if len(built) != 3 {
		t.Fatalf("built %d contents, want 3: an empty one is dropped rather than sent", len(built))
	}
	if call := googleBuiltParts(t, built[1])[0]; call.FunctionCall.Name != "t" {
		t.Errorf("the rebuilt call is %+v", call)
	}
	// The name is remembered from the call, which is the only place this API
	// can learn what the result answers.
	if result := googleBuiltParts(t, built[2])[0]; result.FunctionResponse.Name != "t" {
		t.Errorf("the rebuilt result names %q, want the function it answers",
			result.FunctionResponse.Name)
	}

	// A result whose call this conversation never saw falls back to the name
	// the result itself carries.
	orphan := googleContents([]Message{{Role: RoleTool, Results: []ToolResult{{CallID: "x", Tool: "u"}}}})
	if named := googleBuiltParts(t, orphan[0])[0].FunctionResponse.Name; named != "u" {
		t.Errorf("an orphan result names %q, want the tool it records", named)
	}
}

// googleBuiltParts reads back the parts of a turn this package assembled.
//
// A turn carrying an echo has bytes rather than parts, which is the whole point
// of [wireValue] and is why this says so rather than returning nothing.
func googleBuiltParts(t *testing.T, turn googleTurn) []googlePart {
	t.Helper()
	parts, built := turn.Parts.built.([]googlePart)
	if !built {
		t.Fatalf("the turn carries %T, want the parts this package assembled", turn.Parts.built)
	}
	return parts
}
