// metatool_test.go tests the generic meta-tool dispatch infrastructure:
// UnmarshalParams, WrapAction, WrapVoidAction, MakeMetaHandler,
// defaultFormatResult, ValidActionsString, MetaToolSchema,
// Route, DestructiveRoute, DeriveAnnotations,
// and composite wrappers (RouteAction, RouteVoidAction,
// RouteActionWithRequest, DestructiveAction, DestructiveVoidAction,
// DestructiveActionWithRequest).
package toolutil

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Helpers.

// testInput defines parameters for the test operation.
type testInput struct {
	Name string `json:"name"`
	ID   int    `json:"id"`
}

// testInt64Input defines parameters for the test int64 operation.
type testInt64Input struct {
	ProjectID StringOrInt `json:"project_id"`
	MRIID     int64       `json:"merge_request_iid"`
	Message   string      `json:"message,omitempty"`
}

// testOutput represents the response from the test operation.
type testOutput struct {
	Result string `json:"result"`
}

// UnmarshalParams tests.

// TestUnmarshalParams verifies successful round-trip from map → JSON → struct.
func TestUnmarshalParams(t *testing.T) {
	params := map[string]any{"name": "proj", "id": float64(42)}
	got, err := UnmarshalParams[testInput](params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "proj" || got.ID != 42 {
		t.Errorf("got %+v, want {Name:proj ID:42}", got)
	}
}

// TestUnmarshalParams_InvalidType verifies UnmarshalParams returns an error
// when the params map contains a value incompatible with the target type.
func TestUnmarshalParams_InvalidType(t *testing.T) {
	params := map[string]any{"id": "not-a-number"}
	_, err := UnmarshalParams[testInput](params)
	if err == nil {
		t.Fatal("expected error for type mismatch, got nil")
	}
}

// TestUnmarshalParams_CoercesStringToInt64 verifies that numeric strings
// like "17" are coerced to int64 values, fixing the common LLM behavior
// of sending numbers as JSON strings.
func TestUnmarshalParams_CoercesStringToInt64(t *testing.T) {
	params := map[string]any{
		"project_id":        "42",
		"merge_request_iid": "17",
		"message":           "merge commit",
	}
	got, err := UnmarshalParams[testInt64Input](params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ProjectID.String() != "42" {
		t.Errorf("ProjectID = %q, want %q", got.ProjectID, "42")
	}
	if got.MRIID != 17 {
		t.Errorf("MRIID = %d, want 17", got.MRIID)
	}
	if got.Message != "merge commit" {
		t.Errorf("Message = %q, want %q", got.Message, "merge commit")
	}
}

// TestUnmarshalParams_CoercionNotNeeded verifies that params with correct
// types (numbers as numbers) still work without coercion.
func TestUnmarshalParams_CoercionNotNeeded(t *testing.T) {
	params := map[string]any{
		"project_id":        float64(42),
		"merge_request_iid": float64(17),
	}
	got, err := UnmarshalParams[testInt64Input](params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.MRIID != 17 {
		t.Errorf("MRIID = %d, want 17", got.MRIID)
	}
}

// TestUnmarshalParams_CoercionInvalidString verifies that non-numeric strings
// in int64 fields still produce an error after coercion retry.
func TestUnmarshalParams_CoercionInvalidString(t *testing.T) {
	params := map[string]any{
		"project_id":        "my-project",
		"merge_request_iid": "not-a-number",
	}
	_, err := UnmarshalParams[testInt64Input](params)
	if err == nil {
		t.Fatal("expected error for non-numeric string in int64 field")
	}
}

// TestUnmarshalParams_RejectsUnknownField verifies that params containing a
// key that is not declared on the target type produce an actionable error
// (mirroring the JSON Schema additionalProperties:false lockdown applied to
// tools/list responses) so an LLM that mistypes a parameter name receives a
// clear "unknown field" diagnostic instead of having the value silently
// dropped.
func TestUnmarshalParams_RejectsUnknownField(t *testing.T) {
	params := map[string]any{
		"name":           "proj",
		"id":             float64(42),
		"unknown_field!": "should-fail",
	}
	_, err := UnmarshalParams[testInput](params)
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Errorf("expected error to mention 'unknown field', got: %v", err)
	}
}

// TestCoerceNumericStrings verifies the coercion helper directly.
func TestCoerceNumericStrings(t *testing.T) {
	params := map[string]any{
		"int_val":    "42",
		"float_val":  "3.14",
		"str_val":    "hello",
		"number_val": float64(99),
		"bool_val":   true,
	}
	got := coerceNumericStrings(params)

	if v, ok := got["int_val"].(int64); !ok || v != 42 {
		t.Errorf("int_val = %v (%T), want int64(42)", got["int_val"], got["int_val"])
	}
	if v, ok := got["float_val"].(float64); !ok || v != 3.14 {
		t.Errorf("float_val = %v (%T), want float64(3.14)", got["float_val"], got["float_val"])
	}
	if v, ok := got["str_val"].(string); !ok || v != "hello" {
		t.Errorf("str_val = %v (%T), want string(hello)", got["str_val"], got["str_val"])
	}
	if v, ok := got["number_val"].(float64); !ok || v != 99 {
		t.Errorf("number_val = %v (%T), want float64(99)", got["number_val"], got["number_val"])
	}
	if v, ok := got["bool_val"].(bool); !ok || !v {
		t.Errorf("bool_val = %v (%T), want bool(true)", got["bool_val"], got["bool_val"])
	}
}

// WrapAction / WrapVoidAction tests.

// TestWrapAction verifies that WrapAction produces an ActionFunc that
// deserializes params, calls the typed handler, and returns its result.
func TestWrapAction(t *testing.T) {
	fn := func(_ context.Context, _ *gitlabclient.Client, in testInput) (testOutput, error) {
		return testOutput{Result: "hello " + in.Name}, nil
	}
	action := WrapAction(nil, fn)
	got, err := action(context.Background(), map[string]any{"name": "world"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out, ok := got.(testOutput)
	if !ok {
		t.Fatalf("result type = %T, want testOutput", got)
	}
	if out.Result != "hello world" {
		t.Errorf("Result = %q, want %q", out.Result, "hello world")
	}
}

// TestWrapAction_UnmarshalError verifies WrapAction returns an error when
// params cannot be deserialized into the input struct.
func TestWrapAction_UnmarshalError(t *testing.T) {
	fn := func(_ context.Context, _ *gitlabclient.Client, in testInput) (testOutput, error) {
		return testOutput{}, nil
	}
	action := WrapAction(nil, fn)
	_, err := action(context.Background(), map[string]any{"id": "bad"})
	if err == nil {
		t.Fatal("expected error for bad params, got nil")
	}
}

// TestWrapVoidAction verifies that WrapVoidAction wraps a void handler
// and returns nil result on success.
func TestWrapVoidAction(t *testing.T) {
	called := false
	fn := func(_ context.Context, _ *gitlabclient.Client, in testInput) error {
		called = true
		return nil
	}
	action := WrapVoidAction(nil, fn)
	got, err := action(context.Background(), map[string]any{"name": "x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil result, got %v", got)
	}
	if !called {
		t.Error("handler was not called")
	}
}

// TestWrapVoidAction_UnmarshalError verifies WrapVoidAction returns an error
// when params cannot be deserialized.
func TestWrapVoidAction_UnmarshalError(t *testing.T) {
	fn := func(_ context.Context, _ *gitlabclient.Client, in testInput) error {
		return nil
	}
	action := WrapVoidAction(nil, fn)
	_, err := action(context.Background(), map[string]any{"id": "bad"})
	if err == nil {
		t.Fatal("expected error for bad params, got nil")
	}
}

// TestWrapActionWithRequest verifies that WrapActionWithRequest extracts the
// MCP request from context and passes it to the handler.
func TestWrapActionWithRequest(t *testing.T) {
	var gotReq *mcp.CallToolRequest
	fn := func(_ context.Context, req *mcp.CallToolRequest, _ *gitlabclient.Client, in testInput) (testOutput, error) {
		gotReq = req
		return testOutput{Result: "hello " + in.Name}, nil
	}
	action := WrapActionWithRequest(nil, fn)

	fakeReq := &mcp.CallToolRequest{}
	ctx := ContextWithRequest(context.Background(), fakeReq)
	got, err := action(ctx, map[string]any{"name": "world"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotReq != fakeReq {
		t.Error("expected handler to receive the request from context")
	}
	out, ok := got.(testOutput)
	if !ok {
		t.Fatalf("result type = %T, want testOutput", got)
	}
	if out.Result != "hello world" {
		t.Errorf("Result = %q, want %q", out.Result, "hello world")
	}
}

// TestWrapActionWithRequest_NilContext verifies that WrapActionWithRequest
// passes nil when no request is stored in context.
func TestWrapActionWithRequest_NilContext(t *testing.T) {
	var gotReq *mcp.CallToolRequest
	fn := func(_ context.Context, req *mcp.CallToolRequest, _ *gitlabclient.Client, in testInput) (testOutput, error) {
		gotReq = req
		return testOutput{Result: "ok"}, nil
	}
	action := WrapActionWithRequest(nil, fn)
	_, err := action(context.Background(), map[string]any{"name": "x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotReq != nil {
		t.Error("expected nil request when context has no request")
	}
}

// TestRequestFromContext_Absent verifies that RequestFromContext returns nil
// when no request is stored in the context.
func TestRequestFromContext_Absent(t *testing.T) {
	if RequestFromContext(context.Background()) != nil {
		t.Error("expected nil from empty context")
	}
}

// MakeMetaHandler.

// TestMakeMetaHandler_ValidAction verifies MakeMetaHandler dispatches to
// the correct action handler and returns a formatted result.
func TestMakeMetaHandler_ValidAction(t *testing.T) {
	routes := ActionMap{
		"greet": Route(func(_ context.Context, params map[string]any) (any, error) {
			return map[string]string{"msg": "hi"}, nil
		}),
	}
	handler := MakeMetaHandler("test_tool", routes, nil)
	req := &mcp.CallToolRequest{}
	input := MetaToolInput{Action: "greet", Params: map[string]any{}}
	result, raw, err := handler(context.Background(), req, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	m, ok := raw.(map[string]string)
	if !ok || m["msg"] != "hi" {
		t.Errorf("raw = %v, want map[msg:hi]", raw)
	}
}

// TestMakeMetaHandler_EmptyAction verifies MakeMetaHandler returns an error
// when the action field is empty.
func TestMakeMetaHandler_EmptyAction(t *testing.T) {
	routes := ActionMap{
		"list": Route(func(_ context.Context, _ map[string]any) (any, error) {
			return struct{}{}, nil
		}),
	}
	handler := MakeMetaHandler("test_tool", routes, nil)
	_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, MetaToolInput{})
	if err == nil {
		t.Fatal("expected error for empty action, got nil")
	}
}

// TestMakeMetaHandler_UnknownAction verifies MakeMetaHandler returns an error
// for an unrecognized action name.
func TestMakeMetaHandler_UnknownAction(t *testing.T) {
	routes := ActionMap{
		"list": Route(func(_ context.Context, _ map[string]any) (any, error) {
			return struct{}{}, nil
		}),
	}
	handler := MakeMetaHandler("test_tool", routes, nil)
	_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, MetaToolInput{Action: "bogus"})
	if err == nil {
		t.Fatal("expected error for unknown action, got nil")
	}
}

// TestMakeMetaHandler_CustomFormatter verifies MakeMetaHandler uses a custom
// FormatResultFunc when provided.
func TestMakeMetaHandler_CustomFormatter(t *testing.T) {
	routes := ActionMap{
		"ping": Route(func(_ context.Context, _ map[string]any) (any, error) {
			return "pong", nil
		}),
	}
	customFmt := func(raw any) *mcp.CallToolResult {
		return SuccessResult("CUSTOM:" + raw.(string))
	}
	handler := MakeMetaHandler("test_tool", routes, customFmt)
	result, _, err := handler(context.Background(), &mcp.CallToolRequest{}, MetaToolInput{Action: "ping"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok || tc.Text != "CUSTOM:pong" {
		t.Errorf("result text = %q, want %q", tc.Text, "CUSTOM:pong")
	}
}

func TestMakeMetaHandler_IsErrorResultOmitsStructuredContent(t *testing.T) {
	routes := ActionMap{
		"blocked": Route(func(_ context.Context, _ map[string]any) (any, error) {
			return map[string]string{"status": "blocked"}, nil
		}),
	}
	formatter := func(any) *mcp.CallToolResult {
		return ErrorResult("blocked")
	}
	handler := MakeMetaHandler("test_tool", routes, formatter)

	result, raw, err := handler(context.Background(), &mcp.CallToolRequest{}, MetaToolInput{Action: "blocked"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("result = %#v, want IsError result", result)
	}
	if raw != nil {
		t.Fatalf("structured content = %#v, want nil for IsError result", raw)
	}
}

// defaultFormatResult.

// TestDefaultFormatResult_NilResult verifies "ok" text for nil result.
func TestDefaultFormatResult_Nil(t *testing.T) {
	got := defaultFormatResult(nil)
	tc := got.Content[0].(*mcp.TextContent)
	if tc.Text != "ok" {
		t.Errorf("text = %q, want %q", tc.Text, "ok")
	}
}

// TestDefaultFormatResult_JSONResult verifies JSON serialization for non-nil.
func TestDefaultFormatResult_JSON(t *testing.T) {
	got := defaultFormatResult(map[string]int{"count": 5})
	tc := got.Content[0].(*mcp.TextContent)
	var m map[string]int
	if err := json.Unmarshal([]byte(tc.Text), &m); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if m["count"] != 5 {
		t.Errorf("count = %d, want 5", m["count"])
	}
}

// ValidActionsString.

// TestValidActionsString verifies sorted comma-separated output.
func TestValidActionsString(t *testing.T) {
	routes := ActionMap{
		"delete": Route(nil),
		"create": Route(nil),
		"list":   Route(nil),
	}
	got := ValidActionsString(routes)
	if got != "create, delete, list" {
		t.Errorf("got %q, want %q", got, "create, delete, list")
	}
}

// MetaToolSchema.

// TestMetaToolSchema verifies the generated JSON Schema contains the
// action enum and params property.
func TestMetaToolSchema(t *testing.T) {
	routes := ActionMap{
		"get":  Route(nil),
		"list": Route(nil),
	}
	schema := MetaToolSchema(routes)
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("missing properties")
	}
	actionProp := props["action"].(map[string]any)
	enumVals := actionProp["enum"].([]string)
	if len(enumVals) != 2 || enumVals[0] != "get" || enumVals[1] != "list" {
		t.Errorf("enum = %v, want [get list]", enumVals)
	}
	required := schema["required"].([]any)
	if len(required) != 1 || required[0] != "action" {
		t.Errorf("required = %v, want [action]", required)
	}
}

// TestMetaToolSchema_OpaqueDefault verifies that the default opaque mode
// does NOT emit a oneOf branch list and keeps params as an open object.
func TestMetaToolSchema_OpaqueDefault(t *testing.T) {
	routes := ActionMap{
		"get":  Route(nil),
		"list": Route(nil),
	}
	schema := MetaToolSchema(routes)
	if _, has := schema["oneOf"]; has {
		t.Error("opaque schema should not contain oneOf")
	}
	props := schema["properties"].(map[string]any)
	paramsProp := props["params"].(map[string]any)
	if paramsProp["additionalProperties"] != true {
		t.Errorf("params.additionalProperties = %v, want true", paramsProp["additionalProperties"])
	}
	desc, _ := paramsProp["description"].(string)
	if !strings.Contains(desc, "gitlab://schema/meta/{tool}/{action}") {
		t.Error("params.description should mention the schema resource URI")
	}
}

// TestBuildMetaToolSchema_FullEmitsOneOf verifies that full mode produces
// a oneOf branch per action with action pinned to a const.
func TestBuildMetaToolSchema_FullEmitsOneOf(t *testing.T) {
	routes := ActionMap{
		"create": RouteAction[testInput, testOutput](nil, nil),
		"get":    RouteAction[testInput, testOutput](nil, nil),
	}
	schema := BuildMetaToolSchema(routes, MetaParamSchemaFull)

	branches, ok := schema["oneOf"].([]any)
	if !ok {
		t.Fatalf("oneOf missing or wrong type: %T", schema["oneOf"])
	}
	if len(branches) != 2 {
		t.Fatalf("oneOf len = %d, want 2", len(branches))
	}
	wantActions := []string{"create", "get"} // sorted
	for i, b := range branches {
		bm := b.(map[string]any)
		bp := bm["properties"].(map[string]any)
		ap := bp["action"].(map[string]any)
		if ap["const"] != wantActions[i] {
			t.Errorf("branch[%d].action.const = %v, want %q", i, ap["const"], wantActions[i])
		}
		paramsBranch := bp["params"].(map[string]any)
		// Full mode should preserve the reflected schema, which carries a
		// type or a $ref pointing into $defs.
		_, hasType := paramsBranch["type"]
		_, hasRef := paramsBranch["$ref"]
		_, hasProps := paramsBranch["properties"]
		if !hasType && !hasRef && !hasProps {
			t.Errorf("branch[%d].params lacks type/$ref/properties: %v", i, paramsBranch)
		}
	}
}

// TestBuildMetaToolSchema_CompactStripsDescriptions verifies that compact
// mode drops description strings from params property entries.
func TestBuildMetaToolSchema_CompactStripsDescriptions(t *testing.T) {
	routes := ActionMap{
		"get": RouteAction[testInput, testOutput](nil, nil),
	}
	schema := BuildMetaToolSchema(routes, MetaParamSchemaCompact)

	branches := schema["oneOf"].([]any)
	if len(branches) != 1 {
		t.Fatalf("oneOf len = %d, want 1", len(branches))
	}
	bp := branches[0].(map[string]any)["properties"].(map[string]any)
	paramsBranch := bp["params"].(map[string]any)
	if paramsBranch["additionalProperties"] != true {
		t.Errorf("compact params.additionalProperties = %v, want true", paramsBranch["additionalProperties"])
	}
	props, ok := paramsBranch["properties"].(map[string]any)
	if !ok {
		t.Fatalf("compact params has no properties map: %v", paramsBranch)
	}
	for name, raw := range props {
		entry := raw.(map[string]any)
		if _, hasDesc := entry["description"]; hasDesc {
			t.Errorf("compact field %q retains description", name)
		}
	}
}

// TestBuildMetaToolSchema_UnknownModeFallsBackToOpaque verifies unknown
// modes silently degrade to the opaque envelope.
func TestBuildMetaToolSchema_UnknownModeFallsBackToOpaque(t *testing.T) {
	routes := ActionMap{"get": Route(nil)}
	schema := BuildMetaToolSchema(routes, "verbose")
	if _, has := schema["oneOf"]; has {
		t.Error("unknown mode should not emit oneOf")
	}
}

// TestMetaToolDescriptionPrefix_FormatsLiteralExample checks that the prefix
// embeds the alphabetically first action and the resource pointer for the
// given tool name. Empty routes return an empty string.
func TestMetaToolDescriptionPrefix_FormatsLiteralExample(t *testing.T) {
	routes := ActionMap{"create": Route(nil), "list": Route(nil), "delete": Route(nil)}
	got := MetaToolDescriptionPrefix("gitlab_widget", routes)

	wantExample := `Example: {"action":"create","params":{...}}`
	if !strings.Contains(got, wantExample) {
		t.Errorf("prefix missing literal example, got: %q", got)
	}
	wantPointer := "gitlab://schema/meta/gitlab_widget/<action>"
	if !strings.Contains(got, wantPointer) {
		t.Errorf("prefix missing resource pointer, got: %q", got)
	}
	if !strings.HasSuffix(got, "\n\n") {
		t.Errorf("prefix should end with blank line separator, got: %q", got)
	}

	if MetaToolDescriptionPrefix("gitlab_empty", ActionMap{}) != "" {
		t.Error("empty routes should yield empty prefix")
	}
}

// enrichWithHints.

// TestEnrichWithHints_AddsNextSteps verifies that enrichWithHints injects
// a next_steps field into the structured JSON content of an MCP tool result.
func TestEnrichWithHints_AddsNextSteps(t *testing.T) {
	type sampleOutput struct {
		Items []string `json:"items"`
		Count int      `json:"count"`
	}
	callResult := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "## Results\n\n---\n💡 **Next steps:**\n- Get details\n- Delete item\n"},
		},
	}
	result := sampleOutput{Items: []string{"a", "b"}, Count: 2}
	enriched := enrichWithHints(result, callResult)

	raw, ok := enriched.(json.RawMessage)
	if !ok {
		t.Fatalf("expected json.RawMessage, got %T", enriched)
	}

	// Verify next_steps is the first field in the JSON.
	const prefix = `{"next_steps":`
	if !strings.HasPrefix(string(raw), prefix) {
		t.Errorf("JSON should start with %s, got: %.60s", prefix, string(raw))
	}

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	stepsAny, ok := m["next_steps"].([]any)
	if !ok || len(stepsAny) != 2 {
		t.Fatalf("next_steps = %v, want 2 strings", m["next_steps"])
	}
	if stepsAny[0] != "Get details" || stepsAny[1] != "Delete item" {
		t.Errorf("steps = %v", stepsAny)
	}
	if m["count"] != float64(2) {
		t.Errorf("count = %v, want 2", m["count"])
	}
}

// TestEnrichWithHints_NoHintsSection verifies that enrichWithHints leaves
// the result unchanged when the markdown contains no hints section.
func TestEnrichWithHints_NoHintsSection(t *testing.T) {
	callResult := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "## Just a title\n"},
		},
	}
	original := map[string]string{"key": "val"}
	enriched := enrichWithHints(original, callResult)
	m, ok := enriched.(map[string]string)
	if !ok || m["key"] != "val" {
		t.Error("expected unchanged result when no hints")
	}
}

// TestEnrichWithHints_NilResult verifies that enrichWithHints handles a nil
// tool result without panicking.
func TestEnrichWithHints_NilResult(t *testing.T) {
	callResult := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "---\n💡 **Next steps:**\n- hint\n"},
		},
	}
	if got := enrichWithHints(nil, callResult); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

// TestEnrichWithHints_NilCallResult verifies that enrichWithHints handles
// a nil CallToolResult without panicking.
func TestEnrichWithHints_NilCallResult(t *testing.T) {
	original := map[string]string{"key": "val"}
	enriched := enrichWithHints(original, nil)
	m, ok := enriched.(map[string]string)
	if !ok || m["key"] != "val" {
		t.Error("expected unchanged result for nil callResult")
	}
}

// TestMakeMetaHandler_EnrichesStructuredContent verifies that the meta-tool
// handler wrapper enriches structured JSON output with next_steps hints.
func TestMakeMetaHandler_EnrichesStructuredContent(t *testing.T) {
	routes := ActionMap{
		"list": Route(func(_ context.Context, _ map[string]any) (any, error) {
			return map[string]any{"items": []string{"x"}}, nil
		}),
	}
	formatter := func(result any) *mcp.CallToolResult {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "## List\n\n---\n💡 **Next steps:**\n- View item\n"},
			},
		}
	}
	handler := MakeMetaHandler("test", routes, formatter)
	_, raw, err := handler(context.Background(), &mcp.CallToolRequest{}, MetaToolInput{Action: "list"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rawMsg, ok := raw.(json.RawMessage)
	if !ok {
		t.Fatalf("expected json.RawMessage, got %T", raw)
	}
	var m map[string]any
	if unmarshalErr := json.Unmarshal(rawMsg, &m); unmarshalErr != nil {
		t.Fatalf("failed to unmarshal: %v", unmarshalErr)
	}
	stepsAny, ok := m["next_steps"].([]any)
	if !ok || len(stepsAny) != 1 || stepsAny[0] != "View item" {
		t.Errorf("next_steps = %v", m["next_steps"])
	}
}

// TestEnrichWithHints_NonObjectJSON verifies that enrichWithHints returns
// the result unchanged when it serializes to a non-object JSON value
// (e.g. a string or array).
func TestEnrichWithHints_NonObjectJSON(t *testing.T) {
	callResult := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "---\n💡 **Next steps:**\n- hint\n"},
		},
	}
	original := "just a string"
	enriched := enrichWithHints(original, callResult)
	s, ok := enriched.(string)
	if !ok || s != "just a string" {
		t.Errorf("expected unchanged string, got %T: %v", enriched, enriched)
	}
}

// TestEnrichWithHints_EmptyObject verifies that enrichWithHints correctly
// handles an empty JSON object (only "{}") by producing valid JSON with
// next_steps as the only field.
func TestEnrichWithHints_EmptyObject(t *testing.T) {
	callResult := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "---\n💡 **Next steps:**\n- do thing\n"},
		},
	}
	type empty struct{}
	enriched := enrichWithHints(empty{}, callResult)
	raw, ok := enriched.(json.RawMessage)
	if !ok {
		t.Fatalf("expected json.RawMessage, got %T", enriched)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("invalid JSON: %v — raw: %s", err, string(raw))
	}
	stepsAny, ok := m["next_steps"].([]any)
	if !ok || len(stepsAny) != 1 || stepsAny[0] != "do thing" {
		t.Errorf("next_steps = %v", m["next_steps"])
	}
}

// TestWrapActionWithRequest_UnmarshalError verifies that WrapActionWithRequest
// returns an error when params cannot be unmarshaled into the typed input.
func TestWrapActionWithRequest_UnmarshalError(t *testing.T) {
	fn := func(_ context.Context, _ *mcp.CallToolRequest, _ *gitlabclient.Client, in testInput) (testOutput, error) {
		return testOutput{Result: "should not reach"}, nil
	}
	action := WrapActionWithRequest(nil, fn)
	_, err := action(context.Background(), map[string]any{"name": 12345})
	if err == nil {
		t.Fatal("expected error for invalid params, got nil")
	}
}

// TestDefaultFormatResult_Unmarshalable verifies that defaultFormatResult
// falls back to fmt.Sprintf for types that cannot be JSON-marshaled.
func TestDefaultFormatResult_Unmarshalable(t *testing.T) {
	got := defaultFormatResult(func() {})
	tc, ok := got.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	if tc.Text == "" {
		t.Error("expected non-empty fallback text")
	}
}

// TestMakeMetaHandler_DestructiveActionConfirmBypass verifies that
// MakeMetaHandler intercepts destructive actions with confirmation,
// and that the "confirm" param bypasses the prompt.
func TestMakeMetaHandler_DestructiveActionConfirmBypass(t *testing.T) {
	called := false
	routes := ActionMap{
		"delete": DestructiveRoute(func(_ context.Context, _ map[string]any) (any, error) {
			called = true
			return map[string]string{"status": "deleted"}, nil
		}),
	}
	handler := MakeMetaHandler("test_tool", routes, nil)

	// With "confirm": true, the action should proceed without elicitation.
	input := MetaToolInput{
		Action: "delete",
		Params: map[string]any{"id": float64(1), "confirm": true},
	}
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "test_tool"}}
	result, _, err := handler(context.Background(), req, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("handler was not called — confirmation should have been bypassed")
	}
	if result == nil {
		t.Fatal("result is nil")
	}
}

// TestMakeMetaHandler_DestructiveActionYOLOMode verifies that YOLO_MODE
// bypasses confirmation for destructive meta-tool actions.
func TestMakeMetaHandler_DestructiveActionYOLOMode(t *testing.T) {
	t.Setenv("YOLO_MODE", "true")

	called := false
	routes := ActionMap{
		"token_revoke": DestructiveRoute(func(_ context.Context, _ map[string]any) (any, error) {
			called = true
			return map[string]string{"status": "revoked"}, nil
		}),
	}
	handler := MakeMetaHandler("test_tool", routes, nil)
	input := MetaToolInput{Action: "token_revoke", Params: map[string]any{}}
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "test_tool"}}
	_, _, err := handler(context.Background(), req, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("handler was not called — YOLO_MODE should bypass confirmation")
	}
}

// TestMakeMetaHandler_NonDestructiveSkipsConfirm verifies that non-destructive
// actions are dispatched without any confirmation prompt.
func TestMakeMetaHandler_NonDestructiveSkipsConfirm(t *testing.T) {
	called := false
	routes := ActionMap{
		"list": Route(func(_ context.Context, _ map[string]any) (any, error) {
			called = true
			return []string{"a", "b"}, nil
		}),
	}
	handler := MakeMetaHandler("test_tool", routes, nil)
	input := MetaToolInput{Action: "list", Params: map[string]any{}}
	_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("handler was not called")
	}
}

// TestMakeMetaHandler_DestructiveNoElicitation verifies that when the client
// does not support elicitation (nil request), destructive actions proceed
// without blocking — backward compatibility.
func TestMakeMetaHandler_DestructiveNoElicitation(t *testing.T) {
	called := false
	routes := ActionMap{
		"delete": DestructiveRoute(func(_ context.Context, _ map[string]any) (any, error) {
			called = true
			return map[string]string{"status": "deleted"}, nil
		}),
	}
	handler := MakeMetaHandler("test_tool", routes, nil)
	input := MetaToolInput{Action: "delete", Params: map[string]any{}}
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "test_tool"}}
	_, _, err := handler(context.Background(), req, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("handler was not called — should proceed when elicitation unsupported")
	}
}

// TestRoute_CreatesNonDestructiveRoute verifies that Route() creates an
// ActionRoute with Destructive=false.
func TestRoute_CreatesNonDestructiveRoute(t *testing.T) {
	fn := func(_ context.Context, _ map[string]any) (any, error) { return struct{}{}, nil }
	r := Route(fn)
	if r.Destructive {
		t.Error("Route() should create non-destructive route")
	}
	if r.Handler == nil {
		t.Error("Route() should set Handler")
	}
}

// TestDestructiveRoute_CreatesDestructiveRoute verifies that DestructiveRoute()
// creates an ActionRoute with Destructive=true.
func TestDestructiveRoute_CreatesDestructiveRoute(t *testing.T) {
	fn := func(_ context.Context, _ map[string]any) (any, error) { return struct{}{}, nil }
	r := DestructiveRoute(fn)
	if !r.Destructive {
		t.Error("DestructiveRoute() should create destructive route")
	}
	if r.Handler == nil {
		t.Error("DestructiveRoute() should set Handler")
	}
}

// TestDeriveAnnotations_AllNonDestructive verifies that DeriveAnnotations returns
// NonDestructiveMetaAnnotations when no route is destructive.
func TestDeriveAnnotations_AllNonDestructive(t *testing.T) {
	routes := ActionMap{
		"list":   Route(func(_ context.Context, _ map[string]any) (any, error) { return struct{}{}, nil }),
		"get":    Route(func(_ context.Context, _ map[string]any) (any, error) { return struct{}{}, nil }),
		"create": Route(func(_ context.Context, _ map[string]any) (any, error) { return struct{}{}, nil }),
	}
	ann := DeriveAnnotations(routes)
	if ann.DestructiveHint == nil || *ann.DestructiveHint != false {
		t.Error("all non-destructive routes should produce DestructiveHint=false")
	}
}

// TestDeriveAnnotations_HasDestructive verifies that DeriveAnnotations returns
// MetaAnnotations when at least one route is destructive.
func TestDeriveAnnotations_HasDestructive(t *testing.T) {
	routes := ActionMap{
		"list":   Route(func(_ context.Context, _ map[string]any) (any, error) { return struct{}{}, nil }),
		"delete": DestructiveRoute(func(_ context.Context, _ map[string]any) (any, error) { return struct{}{}, nil }),
	}
	ann := DeriveAnnotations(routes)
	if ann.DestructiveHint == nil || *ann.DestructiveHint != true {
		t.Error("routes with destructive action should produce DestructiveHint=true")
	}
}

// TestDeriveAnnotations_EmptyMap verifies that DeriveAnnotations handles an empty
// ActionMap gracefully (no destructive routes → NonDestructiveMetaAnnotations).
func TestDeriveAnnotations_EmptyMap(t *testing.T) {
	ann := DeriveAnnotations(ActionMap{})
	if ann.DestructiveHint == nil || *ann.DestructiveHint != false {
		t.Error("empty map should produce DestructiveHint=false")
	}
}

// TestDeriveAnnotationsWithTitle verifies that DeriveAnnotationsWithTitle
// delegates to DeriveAnnotations and sets Title from the tool name.
// Covers both destructive and non-destructive route maps.
func TestDeriveAnnotationsWithTitle(t *testing.T) {
	noop := func(_ context.Context, _ map[string]any) (any, error) { return struct{}{}, nil }

	t.Run("non-destructive routes set title and DestructiveHint=false", func(t *testing.T) {
		routes := ActionMap{"list": Route(noop), "get": Route(noop)}
		ann := DeriveAnnotationsWithTitle("gitlab_branch", routes)
		if ann.Title != "Branch" {
			t.Errorf("Title = %q, want %q", ann.Title, "Branch")
		}
		if ann.DestructiveHint == nil || *ann.DestructiveHint != false {
			t.Error("non-destructive routes should produce DestructiveHint=false")
		}
	})

	t.Run("destructive routes set title and DestructiveHint=true", func(t *testing.T) {
		routes := ActionMap{"list": Route(noop), "delete": DestructiveRoute(noop)}
		ann := DeriveAnnotationsWithTitle("gitlab_merge_request", routes)
		if ann.Title != "Merge Request" {
			t.Errorf("Title = %q, want %q", ann.Title, "Merge Request")
		}
		if ann.DestructiveHint == nil || *ann.DestructiveHint != true {
			t.Error("destructive routes should produce DestructiveHint=true")
		}
	})
}

// TestReadOnlyMetaAnnotationsWithTitle verifies that ReadOnlyMetaAnnotationsWithTitle
// returns a copy of ReadOnlyMetaAnnotations with the Title set and all read-only
// fields preserved. Also verifies the shared singleton is not mutated.
func TestReadOnlyMetaAnnotationsWithTitle(t *testing.T) {
	ann := ReadOnlyMetaAnnotationsWithTitle("gitlab_search")

	if ann.Title != "Search" {
		t.Errorf("Title = %q, want %q", ann.Title, "Search")
	}
	if !ann.ReadOnlyHint {
		t.Error("ReadOnlyHint should be true")
	}
	if !ann.IdempotentHint {
		t.Error("IdempotentHint should be true")
	}
	if ann.DestructiveHint == nil || *ann.DestructiveHint != false {
		t.Error("DestructiveHint should be false")
	}
	if ann.OpenWorldHint == nil || *ann.OpenWorldHint != true {
		t.Error("OpenWorldHint should be true")
	}

	// Verify the shared singleton was not mutated.
	if ReadOnlyMetaAnnotations.Title != "" {
		t.Errorf("singleton Title mutated to %q, want empty", ReadOnlyMetaAnnotations.Title)
	}
}

// TestAddMetaTool_RegistersSharedMetadata verifies the shared registration
// helper applies the same metadata contract used by all action-dispatched
// meta-tools.
func TestAddMetaTool_RegistersSharedMetadata(t *testing.T) {
	ClearMetaRoutes()
	t.Cleanup(ClearMetaRoutes)

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	routes := ActionMap{
		"delete": DestructiveRoute(func(_ context.Context, _ map[string]any) (any, error) {
			return struct{}{}, nil
		}),
	}

	AddMetaTool(server, "gitlab_test_meta", "Manage test metadata.", routes, nil, nil)

	tool := findTool(t, listToolsViaClient(t, server), "gitlab_test_meta")
	if !strings.Contains(tool.Description, "gitlab://schema/meta/gitlab_test_meta/<action>") {
		t.Errorf("description missing schema resource hint: %q", tool.Description)
	}
	if !strings.Contains(tool.Description, "Manage test metadata.") {
		t.Errorf("description missing supplied body: %q", tool.Description)
	}
	if tool.Annotations == nil || tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint != true {
		t.Fatal("destructive meta-tool should have DestructiveHint=true")
	}
	if tool.Annotations.Title != "Test Meta" {
		t.Errorf("annotation title = %q, want %q", tool.Annotations.Title, "Test Meta")
	}
	if tool.InputSchema == nil {
		t.Fatal("input schema is nil")
	}
	if tool.OutputSchema == nil {
		t.Fatal("output schema is nil")
	}
	if _, ok := MetaRoutes()["gitlab_test_meta"]; !ok {
		t.Fatal("meta routes were not registered")
	}
}

// TestAddReadOnlyMetaTool_RegistersReadOnlyMetadata verifies the read-only
// helper preserves read-only annotations while sharing the common schema and
// description contract.
func TestAddReadOnlyMetaTool_RegistersReadOnlyMetadata(t *testing.T) {
	ClearMetaRoutes()
	t.Cleanup(ClearMetaRoutes)

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	routes := ActionMap{
		"list": Route(func(_ context.Context, _ map[string]any) (any, error) {
			return struct{}{}, nil
		}),
	}

	AddReadOnlyMetaTool(server, "gitlab_test_read", "List test metadata.", routes, nil, nil)

	tool := findTool(t, listToolsViaClient(t, server), "gitlab_test_read")
	if !strings.Contains(tool.Description, "gitlab://schema/meta/gitlab_test_read/<action>") {
		t.Errorf("description missing schema resource hint: %q", tool.Description)
	}
	if tool.Annotations == nil {
		t.Fatal("annotations are nil")
	}
	if !tool.Annotations.ReadOnlyHint {
		t.Error("ReadOnlyHint should be true")
	}
	if !tool.Annotations.IdempotentHint {
		t.Error("IdempotentHint should be true")
	}
	if tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint != false {
		t.Error("DestructiveHint should be false")
	}
	if tool.Annotations.Title != "Test Read" {
		t.Errorf("annotation title = %q, want %q", tool.Annotations.Title, "Test Read")
	}
	if _, ok := MetaRoutes()["gitlab_test_read"]; !ok {
		t.Fatal("meta routes were not registered")
	}
}

// TestMakeMetaHandler_MetadataDestructive_TriggersConfirm verifies that
// MakeMetaHandler reads route.Destructive to determine confirmation requirement.
func TestMakeMetaHandler_MetadataDestructive_TriggersConfirm(t *testing.T) {
	called := false
	routes := ActionMap{
		"delete": DestructiveRoute(func(_ context.Context, _ map[string]any) (any, error) {
			called = true
			return "ok", nil
		}),
	}
	handler := MakeMetaHandler("test_tool", routes, nil)
	input := MetaToolInput{Action: "delete", Params: map[string]any{"id": float64(1)}}
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "test_tool"}}

	// Without confirm=true, handler should still be called (elicitation unsupported in tests)
	// but the route is recognized as destructive.
	result, _, err := handler(context.Background(), req, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("handler was not called")
	}
	if result == nil {
		t.Fatal("result is nil")
	}
}

// TestMakeMetaHandler_NonDestructive_SkipsConfirm verifies that non-destructive
// routes do not trigger confirmation.
func TestMakeMetaHandler_NonDestructive_SkipsConfirm(t *testing.T) {
	called := false
	routes := ActionMap{
		"list": Route(func(_ context.Context, _ map[string]any) (any, error) {
			called = true
			return []string{"a", "b"}, nil
		}),
	}
	handler := MakeMetaHandler("test_tool", routes, nil)
	input := MetaToolInput{Action: "list", Params: map[string]any{}}
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "test_tool"}}

	result, _, err := handler(context.Background(), req, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("handler was not called")
	}
	if result == nil {
		t.Fatal("result is nil")
	}
}

// Composite wrapper metadata tests — verify that every wrapper type correctly
// sets (or clears) the Destructive flag on the resulting ActionRoute.

// TestCompositeWrappers_DestructiveMetadata verifies that all eight Route/DestructiveRoute
// wrapper functions produce ActionRoutes with the correct Destructive flag.
func TestCompositeWrappers_DestructiveMetadata(t *testing.T) {
	typedFn := func(_ context.Context, _ *gitlabclient.Client, _ testInput) (testOutput, error) {
		return testOutput{}, nil
	}
	voidFn := func(_ context.Context, _ *gitlabclient.Client, _ testInput) error {
		return nil
	}
	reqFn := func(_ context.Context, _ *mcp.CallToolRequest, _ *gitlabclient.Client, _ testInput) (testOutput, error) {
		return testOutput{}, nil
	}
	rawFn := func(_ context.Context, _ map[string]any) (any, error) { return struct{}{}, nil }

	tests := []struct {
		name            string
		route           ActionRoute
		wantDestructive bool
	}{
		{"Route", Route(rawFn), false},
		{"DestructiveRoute", DestructiveRoute(rawFn), true},
		{"RouteAction", RouteAction(nil, typedFn), false},
		{"RouteVoidAction", RouteVoidAction(nil, voidFn), false},
		{"RouteActionWithRequest", RouteActionWithRequest(nil, reqFn), false},
		{"DestructiveAction", DestructiveAction(nil, typedFn), true},
		{"DestructiveVoidAction", DestructiveVoidAction(nil, voidFn), true},
		{"DestructiveActionWithRequest", DestructiveActionWithRequest(nil, reqFn), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.route.Destructive != tt.wantDestructive {
				t.Errorf("Destructive = %v, want %v", tt.route.Destructive, tt.wantDestructive)
			}
			if tt.route.Handler == nil {
				t.Errorf("Handler is nil")
			}
		})
	}
}

// TestDeriveAnnotations_WithCompositeWrappers verifies that DeriveAnnotations
// correctly detects destructive routes produced by composite wrappers in a
// mixed route map (simulating real registration patterns).
func TestDeriveAnnotations_WithCompositeWrappers(t *testing.T) {
	typedFn := func(_ context.Context, _ *gitlabclient.Client, _ testInput) (testOutput, error) {
		return testOutput{}, nil
	}
	voidFn := func(_ context.Context, _ *gitlabclient.Client, _ testInput) error {
		return nil
	}

	tests := []struct {
		name                string
		routes              ActionMap
		wantDestructiveHint bool
	}{
		{
			name: "AllNonDestructive",
			routes: ActionMap{
				"list":   RouteAction(nil, typedFn),
				"get":    RouteAction(nil, typedFn),
				"create": RouteAction(nil, typedFn),
			},
			wantDestructiveHint: false,
		},
		{
			name: "OneDestructiveAction",
			routes: ActionMap{
				"list":   RouteAction(nil, typedFn),
				"get":    RouteAction(nil, typedFn),
				"delete": DestructiveVoidAction(nil, voidFn),
			},
			wantDestructiveHint: true,
		},
		{
			name: "MultipleDestructiveActions",
			routes: ActionMap{
				"list":   RouteAction(nil, typedFn),
				"delete": DestructiveVoidAction(nil, voidFn),
				"remove": DestructiveVoidAction(nil, voidFn),
				"revoke": DestructiveAction(nil, typedFn),
			},
			wantDestructiveHint: true,
		},
		{
			name: "OnlyDestructiveActions",
			routes: ActionMap{
				"delete": DestructiveVoidAction(nil, voidFn),
				"purge":  DestructiveVoidAction(nil, voidFn),
			},
			wantDestructiveHint: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ann := DeriveAnnotations(tt.routes)
			got := ann.DestructiveHint != nil && *ann.DestructiveHint
			if got != tt.wantDestructiveHint {
				t.Errorf("DestructiveHint = %v, want %v", got, tt.wantDestructiveHint)
			}
		})
	}
}

// TestMakeMetaHandler_CompositeWrapperConfirmation verifies that MakeMetaHandler
// correctly triggers (or skips) confirmation for routes built with composite
// wrappers, covering representative domain action patterns.
func TestMakeMetaHandler_CompositeWrapperConfirmation(t *testing.T) {
	typedFn := func(_ context.Context, _ *gitlabclient.Client, _ testInput) (testOutput, error) {
		return testOutput{Result: "ok"}, nil
	}
	voidFn := func(_ context.Context, _ *gitlabclient.Client, _ testInput) error {
		return nil
	}

	routes := ActionMap{
		"list":   RouteAction(nil, typedFn),
		"get":    RouteAction(nil, typedFn),
		"create": RouteAction(nil, typedFn),
		"update": RouteAction(nil, typedFn),
		"delete": DestructiveVoidAction(nil, voidFn),
		"remove": DestructiveAction(nil, typedFn),
	}

	formatter := func(result any) *mcp.CallToolResult {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "ok"}},
		}
	}
	handler := MakeMetaHandler("test_domain", routes, formatter)

	tests := []struct {
		name       string
		action     string
		params     map[string]any
		wantCalled bool
	}{
		{name: "list", action: "list", params: map[string]any{}, wantCalled: true},
		{name: "get", action: "get", params: map[string]any{}, wantCalled: true},
		{name: "create", action: "create", params: map[string]any{}, wantCalled: true},
		{name: "update", action: "update", params: map[string]any{}, wantCalled: true},
		// Destructive actions without elicitation support proceed via fallback
		{name: "delete_fallback", action: "delete", params: map[string]any{}, wantCalled: true},
		{name: "remove_fallback", action: "remove", params: map[string]any{}, wantCalled: true},
		// Destructive action with explicit confirm=true bypasses confirmation
		{name: "delete_confirm", action: "delete", params: map[string]any{"confirm": true}, wantCalled: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := MetaToolInput{
				Action: tt.action,
				Params: tt.params,
			}

			req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "test_domain"}}

			result, _, err := handler(context.Background(), req, input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantCalled && result == nil {
				t.Error("expected result but got nil")
			}
		})
	}
}

// --- OutputSchema tests (TASK-064/065/066/067/071) ---

// TestRouteAction_OutputSchema verifies RouteAction populates OutputSchema
// from the result type R.
func TestRouteAction_OutputSchema(t *testing.T) {
	client := &gitlabclient.Client{}
	route := RouteAction(client, func(_ context.Context, _ *gitlabclient.Client, _ testInput) (testOutput, error) {
		return testOutput{}, nil
	})
	if route.OutputSchema == nil {
		t.Fatal("expected OutputSchema to be non-nil for RouteAction[T,R]")
	}
	typ, ok := route.OutputSchema["type"]
	if !ok || typ != "object" {
		t.Errorf("expected OutputSchema type=object, got %v", typ)
	}
	props, propsOK := route.OutputSchema["properties"].(map[string]any)
	if !propsOK {
		t.Fatal("expected OutputSchema to have properties")
	}
	if _, hasResult := props["result"]; !hasResult {
		t.Error("expected OutputSchema to include 'result' property from testOutput struct")
	}
}

// TestDestructiveAction_OutputSchema verifies DestructiveAction populates OutputSchema.
func TestDestructiveAction_OutputSchema(t *testing.T) {
	client := &gitlabclient.Client{}
	route := DestructiveAction(client, func(_ context.Context, _ *gitlabclient.Client, _ testInput) (testOutput, error) {
		return testOutput{}, nil
	})
	if route.OutputSchema == nil {
		t.Fatal("expected OutputSchema to be non-nil for DestructiveAction[T,R]")
	}
	if !route.Destructive {
		t.Error("expected Destructive=true")
	}
}

// TestRouteActionWithRequest_OutputSchema verifies RouteActionWithRequest populates OutputSchema.
func TestRouteActionWithRequest_OutputSchema(t *testing.T) {
	client := &gitlabclient.Client{}
	route := RouteActionWithRequest(client, func(_ context.Context, _ *mcp.CallToolRequest, _ *gitlabclient.Client, _ testInput) (testOutput, error) {
		return testOutput{}, nil
	})
	if route.OutputSchema == nil {
		t.Fatal("expected OutputSchema to be non-nil for RouteActionWithRequest[T,R]")
	}
}

// TestDestructiveActionWithRequest_OutputSchema verifies DestructiveActionWithRequest populates OutputSchema.
func TestDestructiveActionWithRequest_OutputSchema(t *testing.T) {
	client := &gitlabclient.Client{}
	route := DestructiveActionWithRequest(client, func(_ context.Context, _ *mcp.CallToolRequest, _ *gitlabclient.Client, _ testInput) (testOutput, error) {
		return testOutput{}, nil
	})
	if route.OutputSchema == nil {
		t.Fatal("expected OutputSchema to be non-nil")
	}
	if !route.Destructive {
		t.Error("expected Destructive=true")
	}
}

// TestRouteVoidAction_OutputSchema verifies void variants expose typed output schemas.
func TestRouteVoidAction_OutputSchema(t *testing.T) {
	client := &gitlabclient.Client{}
	route := RouteVoidAction(client, func(_ context.Context, _ *gitlabclient.Client, _ testInput) error {
		return nil
	})
	if route.OutputSchema == nil {
		t.Fatal("expected OutputSchema to be non-nil for void action")
	}
}

// TestDestructiveVoidAction_OutputSchema verifies destructive void variants expose typed output schemas.
func TestDestructiveVoidAction_OutputSchema(t *testing.T) {
	client := &gitlabclient.Client{}
	route := DestructiveVoidAction(client, func(_ context.Context, _ *gitlabclient.Client, _ testInput) error {
		return nil
	})
	if route.OutputSchema == nil {
		t.Fatal("expected OutputSchema to be non-nil for destructive void action")
	}
	if !route.Destructive {
		t.Error("expected Destructive=true")
	}
}

// TestSchemaForRoute_Caching verifies that SchemaForRoute returns the same
// map instance across multiple calls (cache hit).
func TestSchemaForRoute_Caching(t *testing.T) {
	s1 := SchemaForRoute[testOutput]()
	s2 := SchemaForRoute[testOutput]()
	if s1 == nil {
		t.Fatal("expected non-nil schema")
	}
	if s2 == nil {
		t.Fatal("expected non-nil schema on second call")
	}
	j1, err := json.Marshal(s1)
	if err != nil {
		t.Fatalf("marshal s1: %v", err)
	}
	j2, err := json.Marshal(s2)
	if err != nil {
		t.Fatalf("marshal s2: %v", err)
	}
	if string(j1) != string(j2) {
		t.Errorf("cached schemas differ:\n%s\n%s", j1, j2)
	}
}

// TestMetaToolOutputSchema_IsEnvelope verifies the envelope schema returned
// by MetaToolOutputSchema() contains cross-cutting fields and does NOT contain
// per-action schemas (regression test for TASK-067).
func TestMetaToolOutputSchema_IsEnvelope(t *testing.T) {
	schema := MetaToolOutputSchema()
	if schema["type"] != "object" {
		t.Errorf("expected type=object, got %v", schema["type"])
	}
	addProps, hasAddProps := schema["additionalProperties"]
	if !hasAddProps || addProps != true {
		t.Error("envelope schema must have additionalProperties=true")
	}
	props, propsOK := schema["properties"].(map[string]any)
	if !propsOK {
		t.Fatal("expected properties map")
	}
	if _, hasNextSteps := props["next_steps"]; !hasNextSteps {
		t.Error("expected next_steps in envelope properties")
	}
	if _, hasPagination := props["pagination"]; !hasPagination {
		t.Error("expected pagination in envelope properties")
	}
	// Envelope must NOT contain per-action output schemas.
	if _, hasResult := props["result"]; hasResult {
		t.Error("envelope should not contain per-action fields like 'result'")
	}
}

// TestRoute_OutputSchema_Nil verifies plain Route() has nil OutputSchema.
func TestRoute_OutputSchema_Nil(t *testing.T) {
	r := Route(func(_ context.Context, _ map[string]any) (any, error) {
		return "", nil
	})
	if r.OutputSchema != nil {
		t.Error("expected nil OutputSchema for plain Route()")
	}
}

// TestDestructiveRoute_OutputSchema_Nil verifies plain DestructiveRoute() has nil OutputSchema.
func TestDestructiveRoute_OutputSchema_Nil(t *testing.T) {
	r := DestructiveRoute(func(_ context.Context, _ map[string]any) (any, error) {
		return "", nil
	})
	if r.OutputSchema != nil {
		t.Error("expected nil OutputSchema for plain DestructiveRoute()")
	}
}

// TestRegisterRoutes_MetaRoutes_ClearMetaRoutes verifies the meta-tool route
// registry lifecycle: RegisterRoutes stores routes, MetaRoutes returns a
// snapshot, and ClearMetaRoutes empties the registry.
func TestRegisterRoutes_MetaRoutes_ClearMetaRoutes(t *testing.T) {
	ClearMetaRoutes()
	t.Cleanup(ClearMetaRoutes)

	routes := ActionMap{
		"list": Route(func(_ context.Context, _ map[string]any) (any, error) {
			return "", nil
		}),
	}
	RegisterRoutes("gitlab_test_tool", routes)

	snap := MetaRoutes()
	if len(snap) != 1 {
		t.Fatalf("MetaRoutes() returned %d entries, want 1", len(snap))
	}
	if _, ok := snap["gitlab_test_tool"]; !ok {
		t.Error("MetaRoutes() missing key 'gitlab_test_tool'")
	}

	ClearMetaRoutes()
	snap = MetaRoutes()
	if len(snap) != 0 {
		t.Fatalf("MetaRoutes() after ClearMetaRoutes() returned %d entries, want 0", len(snap))
	}
}

// TestMetaRoutes_ReturnsSnapshot verifies that the map returned by
// MetaRoutes is a copy — mutations do not affect the internal registry.
func TestMetaRoutes_ReturnsSnapshot(t *testing.T) {
	ClearMetaRoutes()
	t.Cleanup(ClearMetaRoutes)

	RegisterRoutes("gitlab_snap", ActionMap{
		"get": Route(func(_ context.Context, _ map[string]any) (any, error) {
			return "", nil
		}),
	})

	snap := MetaRoutes()
	delete(snap, "gitlab_snap")

	snap2 := MetaRoutes()
	if _, ok := snap2["gitlab_snap"]; !ok {
		t.Error("deleting from snapshot must not affect the internal registry")
	}
}

// TestCaptureMetaRoutes_ReturnsOnlyRoutesRegisteredInCallback verifies that
// per-server schema resources can capture a local route catalog without
// inheriting older global registry entries.
func TestCaptureMetaRoutes_ReturnsOnlyRoutesRegisteredInCallback(t *testing.T) {
	ClearMetaRoutes()
	t.Cleanup(ClearMetaRoutes)

	RegisterRoutes("gitlab_existing", ActionMap{
		"get": Route(func(_ context.Context, _ map[string]any) (any, error) {
			return "existing", nil
		}),
	})

	captured := CaptureMetaRoutes(func() {
		RegisterRoutes("gitlab_captured", ActionMap{
			"list": Route(func(_ context.Context, _ map[string]any) (any, error) {
				return "captured", nil
			}),
		})
	})

	if _, ok := captured["gitlab_captured"]; !ok {
		t.Fatal("captured routes missing gitlab_captured")
	}
	if _, ok := captured["gitlab_existing"]; ok {
		t.Fatal("captured routes should not include pre-existing global routes")
	}
	if _, ok := MetaRoutes()["gitlab_captured"]; !ok {
		t.Fatal("CaptureMetaRoutes should still populate the global audit registry")
	}

	delete(captured["gitlab_captured"], "list")
	if _, ok := MetaRoutes()["gitlab_captured"]["list"]; !ok {
		t.Fatal("mutating captured inner maps should not affect the global registry")
	}
}

// --- Coverage tests for BuildMetaToolSchema helpers and supporting paths ---
// The following tests target branches not exercised by the higher-level
// tests above:
//
//   - SetMetaParamSchemaMode: package-level mode setter.
//   - resolveTopLevelRef: every fallback branch when $ref / $defs are
//     missing or malformed.
//   - compactParamsSchema: nil input, missing/non-object properties,
//     non-map property entries, and $ref resolution.
//   - buildMetaOneOf: per-action InputSchema = nil fallback.
//   - schemaForType: pointer dereference and cache hit paths.
//   - stripReservedKeys: presence of reserved keys mixed with real fields
//     (covers the "out[k] = v" copy branch).
//   - UnmarshalParams: double-failure path preserves the original error.
//   - enrichWithHints: non-object JSON short-circuit and non-text content
//     iteration.

// TestSetMetaParamSchemaMode_ValidValues verifies that each documented mode
// is accepted and round-trips through currentMetaParamSchemaMode.
func TestSetMetaParamSchemaMode_ValidValues(t *testing.T) {
	t.Cleanup(func() { SetMetaParamSchemaMode(MetaParamSchemaOpaque) })

	for _, mode := range []string{MetaParamSchemaOpaque, MetaParamSchemaCompact, MetaParamSchemaFull} {
		t.Run(mode, func(t *testing.T) {
			SetMetaParamSchemaMode(mode)
			if got := currentMetaParamSchemaMode(); got != mode {
				t.Errorf("currentMetaParamSchemaMode() = %q, want %q", got, mode)
			}
		})
	}
}

// TestSetMetaParamSchemaMode_InvalidCoercesToOpaque verifies that an unknown
// mode is silently coerced to "opaque" so misconfiguration cannot break the
// tools/list payload.
func TestSetMetaParamSchemaMode_InvalidCoercesToOpaque(t *testing.T) {
	t.Cleanup(func() { SetMetaParamSchemaMode(MetaParamSchemaOpaque) })

	SetMetaParamSchemaMode(MetaParamSchemaFull)
	SetMetaParamSchemaMode("nonsense")
	if got := currentMetaParamSchemaMode(); got != MetaParamSchemaOpaque {
		t.Errorf("invalid mode should coerce to opaque, got %q", got)
	}
}

// TestResolveTopLevelRef_NoRef returns the schema unchanged when no
// top-level $ref is present.
func TestResolveTopLevelRef_NoRef(t *testing.T) {
	s := map[string]any{"type": "object"}
	got := resolveTopLevelRef(s)
	if !reflect.DeepEqual(got, s) {
		t.Errorf("expected schema returned unchanged, got %v", got)
	}
}

// TestResolveTopLevelRef_RefWithoutDefs returns the schema unchanged when
// $defs is absent; we cannot resolve the reference so the original wins.
func TestResolveTopLevelRef_RefWithoutDefs(t *testing.T) {
	s := map[string]any{"$ref": "#/$defs/Foo"}
	got := resolveTopLevelRef(s)
	if !reflect.DeepEqual(got, s) {
		t.Errorf("expected schema returned unchanged, got %v", got)
	}
}

// TestResolveTopLevelRef_RefWrongPrefix returns the schema unchanged when
// the $ref does not match the supported "#/$defs/" prefix.
func TestResolveTopLevelRef_RefWrongPrefix(t *testing.T) {
	s := map[string]any{
		"$ref":  "https://example.com/schema.json",
		"$defs": map[string]any{"Foo": map[string]any{"type": "object"}},
	}
	got := resolveTopLevelRef(s)
	if !reflect.DeepEqual(got, s) {
		t.Errorf("non-internal $ref should be returned unchanged, got %v", got)
	}
}

// TestResolveTopLevelRef_RefMissingTarget returns the schema unchanged when
// the referenced $defs entry does not exist.
func TestResolveTopLevelRef_RefMissingTarget(t *testing.T) {
	s := map[string]any{
		"$ref":  "#/$defs/Missing",
		"$defs": map[string]any{"Other": map[string]any{"type": "object"}},
	}
	got := resolveTopLevelRef(s)
	if !reflect.DeepEqual(got, s) {
		t.Errorf("missing target should return original schema, got %v", got)
	}
}

// TestResolveTopLevelRef_ResolvesValidRef returns the referenced $defs
// entry when the reference is well-formed.
func TestResolveTopLevelRef_ResolvesValidRef(t *testing.T) {
	target := map[string]any{
		"type":       "object",
		"properties": map[string]any{"id": map[string]any{"type": "integer"}},
	}
	s := map[string]any{
		"$ref":  "#/$defs/Foo",
		"$defs": map[string]any{"Foo": target},
	}
	got := resolveTopLevelRef(s)
	if !reflect.DeepEqual(got, target) {
		t.Errorf("expected ref to resolve to target, got %v", got)
	}
}

// TestCompactParamsSchema_Nil returns nil for a nil schema.
func TestCompactParamsSchema_Nil(t *testing.T) {
	if got := compactParamsSchema(nil); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

// TestCompactParamsSchema_NoProperties returns a permissive open object
// schema when the input has no `properties` field.
func TestCompactParamsSchema_NoProperties(t *testing.T) {
	got := compactParamsSchema(map[string]any{"type": "object"})
	if got["type"] != "object" {
		t.Errorf("type = %v, want object", got["type"])
	}
	if got["additionalProperties"] != true {
		t.Errorf("additionalProperties = %v, want true", got["additionalProperties"])
	}
	if _, hasProps := got["properties"]; hasProps {
		t.Errorf("expected no properties field, got %v", got["properties"])
	}
}

// TestCompactParamsSchema_NonObjectProperty replaces non-map property values
// (e.g. arrays, scalars) with empty schemas rather than panicking.
func TestCompactParamsSchema_NonObjectProperty(t *testing.T) {
	got := compactParamsSchema(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"weird": "not-a-map",
			"id":    map[string]any{"type": "integer"},
		},
	})
	props, _ := got["properties"].(map[string]any)
	weird, _ := props["weird"].(map[string]any)
	if weird == nil {
		t.Fatalf("expected empty map for non-object property, got %v", props["weird"])
	}
	if len(weird) != 0 {
		t.Errorf("expected empty schema for non-object property, got %v", weird)
	}
	id, _ := props["id"].(map[string]any)
	if id["type"] != "integer" {
		t.Errorf("id type = %v, want integer", id["type"])
	}
}

// TestCompactParamsSchema_PropertyWithEnum keeps only type and enum fields
// per property; description and other metadata are dropped.
func TestCompactParamsSchema_PropertyWithEnum(t *testing.T) {
	got := compactParamsSchema(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"state": map[string]any{
				"type":        "string",
				"enum":        []any{"open", "closed"},
				"description": "should be stripped",
			},
		},
	})
	props, _ := got["properties"].(map[string]any)
	state, _ := props["state"].(map[string]any)
	if state["type"] != "string" {
		t.Errorf("type = %v, want string", state["type"])
	}
	enum, ok := state["enum"].([]any)
	if !ok || len(enum) != 2 {
		t.Errorf("enum = %v, want [open closed]", state["enum"])
	}
	if _, has := state["description"]; has {
		t.Errorf("expected description to be stripped, got %v", state["description"])
	}
}

// TestCompactParamsSchema_ResolvesTopLevelRef inlines a $ref before
// compacting so the final schema reflects the referenced definition.
func TestCompactParamsSchema_ResolvesTopLevelRef(t *testing.T) {
	s := map[string]any{
		"$ref": "#/$defs/Foo",
		"$defs": map[string]any{
			"Foo": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{"type": "integer", "description": "x"},
				},
			},
		},
	}
	got := compactParamsSchema(s)
	props, _ := got["properties"].(map[string]any)
	id, _ := props["id"].(map[string]any)
	if id["type"] != "integer" {
		t.Errorf("expected $ref to resolve before compacting, got %v", got)
	}
	if _, has := got["$defs"]; has {
		t.Error("expected $defs to be dropped from compacted schema")
	}
}

// TestBuildMetaOneOf_NilInputSchemaFallsBackToOpenObject substitutes a
// permissive object schema when a route does not declare an InputSchema.
func TestBuildMetaOneOf_NilInputSchemaFallsBackToOpenObject(t *testing.T) {
	routes := ActionMap{
		"act": {Handler: func(_ context.Context, _ map[string]any) (any, error) {
			return struct{}{}, nil
		}},
	}
	branches := buildMetaOneOf(routes, []string{"act"}, false)
	if len(branches) != 1 {
		t.Fatalf("len(branches) = %d, want 1", len(branches))
	}
	branch, _ := branches[0].(map[string]any)
	props, _ := branch["properties"].(map[string]any)
	params, _ := props["params"].(map[string]any)
	if params["type"] != "object" {
		t.Errorf("nil InputSchema should fall back to type:object, got %v", params)
	}
	if params["additionalProperties"] != true {
		t.Errorf("nil InputSchema should set additionalProperties:true, got %v", params)
	}
}

// TestSchemaForType_PointerType dereferences pointer types so *T and T
// produce the same cached schema.
func TestSchemaForType_PointerType(t *testing.T) {
	type sample struct {
		Name string `json:"name"`
	}
	direct := schemaForType(reflect.TypeFor[sample]())
	pointer := schemaForType(reflect.TypeFor[*sample]())
	if direct == nil || pointer == nil {
		t.Fatal("expected non-nil schemas for both T and *T")
	}
	if !reflect.DeepEqual(direct, pointer) {
		t.Errorf("schemaForType(T) and schemaForType(*T) should be equal,\n  T  = %v\n  *T = %v", direct, pointer)
	}
}

// TestSchemaForType_CacheHit returns the cached schema for repeated calls
// with the same reflect.Type.
func TestSchemaForType_CacheHit(t *testing.T) {
	type cached struct {
		ID int `json:"id"`
	}
	first := schemaForType(reflect.TypeFor[cached]())
	second := schemaForType(reflect.TypeFor[cached]())
	if first == nil || second == nil {
		t.Fatal("expected non-nil schemas")
	}
	// Cache hit should return the same map pointer.
	if reflect.ValueOf(first).Pointer() != reflect.ValueOf(second).Pointer() {
		t.Error("expected cached schema map to be reused on second call")
	}
}

// TestStripReservedKeys_MultipleKeys verifies that real keys are preserved
// when reserved keys are also present (covers the "copy" branch).
func TestStripReservedKeys_MultipleKeys(t *testing.T) {
	in := map[string]any{
		"confirm": true,
		"name":    "proj",
		"id":      42,
	}
	out := stripReservedKeys(in)
	if _, has := out["confirm"]; has {
		t.Error("expected confirm to be stripped")
	}
	if out["name"] != "proj" {
		t.Errorf("name = %v, want proj", out["name"])
	}
	if out["id"] != 42 {
		t.Errorf("id = %v, want 42", out["id"])
	}
	// Original map must not be mutated.
	if _, has := in["confirm"]; !has {
		t.Error("stripReservedKeys mutated the input map")
	}
}

// TestEnrichWithHints_NonObjectJSONFromArray returns the result unchanged
// when the marshaled JSON is an array (does not start with `{`).
func TestEnrichWithHints_NonObjectJSONFromArray(t *testing.T) {
	callResult := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "Some output\n\n💡 **Next steps:**\n- do thing\n"},
		},
	}
	result := []string{"a", "b"}
	got := enrichWithHints(result, callResult)
	// Array inputs must be returned unchanged because we only enrich JSON
	// objects to keep the {next_steps, ...} contract well-defined.
	gotSlice, ok := got.([]string)
	if !ok || len(gotSlice) != 2 {
		t.Errorf("expected array result returned unchanged, got %v (%T)", got, got)
	}
}

// TestEnrichWithHints_NonTextContentSkipped iterates past non-text content
// blocks when looking for hints; only TextContent contributes to extraction.
func TestEnrichWithHints_NonTextContentSkipped(t *testing.T) {
	callResult := &mcp.CallToolResult{
		Content: []mcp.Content{
			// Non-text content (e.g. resource link) must not panic and must
			// not be inspected for hints.
			&mcp.ResourceLink{URI: "gitlab://resource"},
		},
	}
	result := map[string]any{"ok": true}
	got := enrichWithHints(result, callResult)
	// No hints found → input must be returned unchanged.
	if !reflect.DeepEqual(got, result) {
		t.Errorf("expected result unchanged when no text content, got %v", got)
	}
}

// TestUnmarshalParams_DoubleFailureReturnsOriginalError confirms that when
// neither the strict pass nor the numeric-string-coerced retry succeed, the
// original error message is preserved (rather than the retry's).
func TestUnmarshalParams_DoubleFailureReturnsOriginalError(t *testing.T) {
	type strictInput struct {
		ID int `json:"id"`
	}
	// "id" cannot be coerced from a non-numeric string to int even after
	// numeric-string coercion, so both passes fail.
	_, err := UnmarshalParams[strictInput](map[string]any{"id": "not-a-number"})
	if err == nil {
		t.Fatal("expected error from double-failure path")
	}
	if !strings.Contains(err.Error(), "invalid params for this action") {
		t.Errorf("expected wrapped error message, got %q", err.Error())
	}
}

type routeSchemaTestInput struct {
	ID int `json:"id"`
}

func TestRouteVoidActionReturnsTypedOutput(t *testing.T) {
	t.Parallel()

	route := RouteVoidAction((*gitlabclient.Client)(nil), func(_ context.Context, _ *gitlabclient.Client, input routeSchemaTestInput) error {
		if input.ID != 7 {
			t.Fatalf("input.ID = %d, want 7", input.ID)
		}
		return nil
	})

	if route.OutputSchema == nil {
		t.Fatal("RouteVoidAction OutputSchema is nil")
	}
	if route.Destructive {
		t.Fatal("RouteVoidAction marked route destructive")
	}

	result, err := route.Handler(context.Background(), map[string]any{"id": 7})
	if err != nil {
		t.Fatalf("route handler returned error: %v", err)
	}
	out, ok := result.(VoidOutput)
	if !ok {
		t.Fatalf("route handler result type = %T, want VoidOutput", result)
	}
	if out.Status != "success" {
		t.Fatalf("VoidOutput.Status = %q, want success", out.Status)
	}
	if out.Message == "" {
		t.Fatal("VoidOutput.Message is empty")
	}
}

func TestRouteVoidActionInvalidInput(t *testing.T) {
	t.Parallel()

	route := RouteVoidAction((*gitlabclient.Client)(nil), func(context.Context, *gitlabclient.Client, routeSchemaTestInput) error {
		t.Fatal("handler should not be called for invalid input")
		return nil
	})

	result, err := route.Handler(context.Background(), map[string]any{"unknown": true})
	if err == nil {
		t.Fatal("route handler returned nil error for invalid input")
	}
	if result != nil {
		t.Fatalf("route handler result = %#v, want nil", result)
	}
}

func TestDestructiveVoidActionReturnsTypedOutput(t *testing.T) {
	t.Parallel()

	route := DestructiveVoidAction((*gitlabclient.Client)(nil), func(_ context.Context, _ *gitlabclient.Client, input routeSchemaTestInput) error {
		if input.ID != 11 {
			t.Fatalf("input.ID = %d, want 11", input.ID)
		}
		return nil
	})

	if route.OutputSchema == nil {
		t.Fatal("DestructiveVoidAction OutputSchema is nil")
	}
	if !route.Destructive {
		t.Fatal("DestructiveVoidAction did not mark route destructive")
	}

	result, err := route.Handler(context.Background(), map[string]any{"id": 11})
	if err != nil {
		t.Fatalf("route handler returned error: %v", err)
	}
	out, ok := result.(DeleteOutput)
	if !ok {
		t.Fatalf("route handler result type = %T, want DeleteOutput", result)
	}
	if out.Status != "success" {
		t.Fatalf("DeleteOutput.Status = %q, want success", out.Status)
	}
	if out.Message == "" {
		t.Fatal("DeleteOutput.Message is empty")
	}
}

func TestDestructiveVoidActionPropagatesError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("delete failed")
	route := DestructiveVoidAction((*gitlabclient.Client)(nil), func(context.Context, *gitlabclient.Client, routeSchemaTestInput) error {
		return wantErr
	})

	result, err := route.Handler(context.Background(), map[string]any{"id": 1})
	if !errors.Is(err, wantErr) {
		t.Fatalf("route handler error = %v, want %v", err, wantErr)
	}
	if result != nil {
		t.Fatalf("route handler result = %#v, want nil", result)
	}
}

func TestWithVoidOutput_NilResult_ReturnsSuccessOutput(t *testing.T) {
	t.Parallel()

	sentinel := struct{ OK bool }{OK: true}
	inner := func(_ context.Context, _ map[string]any) (any, error) { return nil, nil }
	wrapped := withVoidOutput(inner, sentinel)

	result, err := wrapped(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != sentinel {
		t.Fatalf("result = %#v, want %#v", result, sentinel)
	}
}

func TestWithVoidOutput_NonNilResult_PassesThrough(t *testing.T) {
	t.Parallel()

	original := struct{ Val int }{Val: 42}
	inner := func(_ context.Context, _ map[string]any) (any, error) { return original, nil }
	wrapped := withVoidOutput(inner, struct{ OK bool }{OK: true})

	result, err := wrapped(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != original {
		t.Fatalf("result = %#v, want %#v", result, original)
	}
}

func TestWithVoidOutput_InnerError_PropagatesError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("inner failure")
	inner := func(_ context.Context, _ map[string]any) (any, error) { return nil, wantErr }
	wrapped := withVoidOutput(inner, struct{}{})

	result, err := wrapped(context.Background(), nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if result != nil {
		t.Fatalf("result = %#v, want nil", result)
	}
}

func TestDestructiveVoidActionWithRequest_ReturnsDeleteOutput(t *testing.T) {
	t.Parallel()

	route := DestructiveVoidActionWithRequest((*gitlabclient.Client)(nil),
		func(_ context.Context, _ *mcp.CallToolRequest, _ *gitlabclient.Client, _ routeSchemaTestInput) error {
			return nil
		})

	if !route.Destructive {
		t.Fatal("expected Destructive = true")
	}
	if route.OutputSchema == nil {
		t.Fatal("expected OutputSchema to be set")
	}
	result, err := route.Handler(context.Background(), map[string]any{"id": 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out, ok := result.(DeleteOutput)
	if !ok {
		t.Fatalf("result type = %T, want DeleteOutput", result)
	}
	if out.Status != "success" {
		t.Fatalf("Status = %q, want \"success\"", out.Status)
	}
}

func TestMetaToolVoidActionsReturnProtocolStructuredContent(t *testing.T) {
	t.Parallel()

	routes := ActionMap{
		"delete": DestructiveVoidAction((*gitlabclient.Client)(nil), func(context.Context, *gitlabclient.Client, routeSchemaTestInput) error {
			return nil
		}),
		"void": RouteVoidAction((*gitlabclient.Client)(nil), func(context.Context, *gitlabclient.Client, routeSchemaTestInput) error {
			return nil
		}),
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	AddMetaTool(server, "test_meta", "Test meta tool.", routes, nil, nil)
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	voidResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "test_meta",
		Arguments: map[string]any{
			"action": "void",
			"params": map[string]any{"id": 1},
		},
	})
	if err != nil {
		t.Fatalf("CallTool(void): %v", err)
	}
	if voidResult.IsError {
		t.Fatalf("CallTool(void) returned IsError result: %#v", voidResult)
	}
	if len(voidResult.Content) == 0 {
		t.Fatal("CallTool(void) returned no content")
	}
	var voidOut VoidOutput
	rawVoid, err := json.Marshal(voidResult.StructuredContent)
	if err != nil {
		t.Fatalf("marshal void structured content: %v", err)
	}
	err = json.Unmarshal(rawVoid, &voidOut)
	if err != nil {
		t.Fatalf("unmarshal void structured content: %v", err)
	}
	if voidOut.Status != "success" || voidOut.Message == "" {
		t.Fatalf("void structured content = %+v, want success status and message", voidOut)
	}

	deleteResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "test_meta",
		Arguments: map[string]any{
			"action": "delete",
			"params": map[string]any{"id": 1, "confirm": true},
		},
	})
	if err != nil {
		t.Fatalf("CallTool(delete): %v", err)
	}
	if deleteResult.IsError {
		t.Fatalf("CallTool(delete) returned IsError result: %#v", deleteResult)
	}
	var deleteOut DeleteOutput
	rawDelete, err := json.Marshal(deleteResult.StructuredContent)
	if err != nil {
		t.Fatalf("marshal delete structured content: %v", err)
	}
	err = json.Unmarshal(rawDelete, &deleteOut)
	if err != nil {
		t.Fatalf("unmarshal delete structured content: %v", err)
	}
	if deleteOut.Status != "success" || deleteOut.Message == "" {
		t.Fatalf("delete structured content = %+v, want success status and message", deleteOut)
	}
}

func TestDestructiveVoidActionWithRequest_PropagatesError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("request delete failed")
	route := DestructiveVoidActionWithRequest((*gitlabclient.Client)(nil),
		func(_ context.Context, _ *mcp.CallToolRequest, _ *gitlabclient.Client, _ routeSchemaTestInput) error {
			return wantErr
		})

	result, err := route.Handler(context.Background(), map[string]any{"id": 1})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if result != nil {
		t.Fatalf("result = %#v, want nil", result)
	}
}

func TestWrapVoidActionWithRequest_Success_ReturnsNil(t *testing.T) {
	t.Parallel()

	wrapped := WrapVoidActionWithRequest((*gitlabclient.Client)(nil),
		func(_ context.Context, _ *mcp.CallToolRequest, _ *gitlabclient.Client, _ routeSchemaTestInput) error {
			return nil
		})

	result, err := wrapped(context.Background(), map[string]any{"id": 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatalf("result = %#v, want nil", result)
	}
}

func TestWrapVoidActionWithRequest_Error_PropagatesError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("wrap void request error")
	wrapped := WrapVoidActionWithRequest((*gitlabclient.Client)(nil),
		func(_ context.Context, _ *mcp.CallToolRequest, _ *gitlabclient.Client, _ routeSchemaTestInput) error {
			return wantErr
		})

	result, err := wrapped(context.Background(), map[string]any{"id": 1})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if result != nil {
		t.Fatalf("result = %#v, want nil", result)
	}
}

func TestWrapVoidActionWithRequest_UnmarshalError_ReturnsError(t *testing.T) {
	t.Parallel()

	wrapped := WrapVoidActionWithRequest((*gitlabclient.Client)(nil),
		func(_ context.Context, _ *mcp.CallToolRequest, _ *gitlabclient.Client, _ routeSchemaTestInput) error {
			return nil
		})

	// Pass a param with the wrong type to trigger UnmarshalParams error.
	result, err := wrapped(context.Background(), map[string]any{"id": "not-an-int"})
	if err == nil {
		t.Fatal("expected error for invalid params, got nil")
	}
	if result != nil {
		t.Fatalf("result = %#v, want nil on error", result)
	}
}
