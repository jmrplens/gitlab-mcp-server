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
	"sort"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
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
	MRIID     int64       `json:"mr_iid"`
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
		"project_id": "42",
		"mr_iid":     "17",
		"message":    "merge commit",
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
		"project_id": float64(42),
		"mr_iid":     float64(17),
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
		"project_id": "my-project",
		"mr_iid":     "not-a-number",
	}
	_, err := UnmarshalParams[testInt64Input](params)
	if err == nil {
		t.Fatal("expected error for non-numeric string in int64 field")
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

// MetaToolOutputSchema.

// TestMetaToolOutputSchema_TypedRoutesProduceAnyOf verifies that routes
// created via the typed RouteAction[T,R] family contribute one anyOf branch
// per action, with the action name as branch title.
func TestMetaToolOutputSchema_TypedRoutesProduceAnyOf(t *testing.T) {
	type fooOut struct {
		Foo string `json:"foo"`
	}
	type barOut struct {
		Bar int `json:"bar"`
	}
	routes := ActionMap{
		"foo": RouteAction[struct{}, fooOut](nil, func(_ context.Context, _ *gitlabclient.Client, _ struct{}) (fooOut, error) {
			return fooOut{}, nil
		}),
		"bar": RouteAction[struct{}, barOut](nil, func(_ context.Context, _ *gitlabclient.Client, _ struct{}) (barOut, error) {
			return barOut{}, nil
		}),
	}
	out := MetaToolOutputSchema(routes)
	if out == nil {
		t.Fatal("expected non-nil output schema")
	}
	s, ok := out.(*jsonschema.Schema)
	if !ok {
		t.Fatalf("expected *jsonschema.Schema, got %T", out)
	}
	if s.Type != "object" {
		t.Errorf("type = %q, want object", s.Type)
	}
	if len(s.AnyOf) != 2 {
		t.Fatalf("anyOf len = %d, want 2", len(s.AnyOf))
	}
	titles := []string{s.AnyOf[0].Title, s.AnyOf[1].Title}
	sort.Strings(titles)
	if titles[0] != "bar" || titles[1] != "foo" {
		t.Errorf("titles = %v, want [bar foo]", titles)
	}
}

// TestMetaToolOutputSchema_UntypedRoutesAddFallback verifies that when at
// least one route is type-erased (Route / DestructiveRoute), the schema
// includes a permissive untyped_action_result fallback branch.
func TestMetaToolOutputSchema_UntypedRoutesAddFallback(t *testing.T) {
	type fooOut struct {
		Foo string `json:"foo"`
	}
	routes := ActionMap{
		"foo": RouteAction[struct{}, fooOut](nil, func(_ context.Context, _ *gitlabclient.Client, _ struct{}) (fooOut, error) {
			return fooOut{}, nil
		}),
		"untyped": Route(func(_ context.Context, _ map[string]any) (any, error) { return struct{}{}, nil }),
	}
	out := MetaToolOutputSchema(routes)
	s := out.(*jsonschema.Schema)
	if len(s.AnyOf) != 2 {
		t.Fatalf("anyOf len = %d, want 2 (1 typed + 1 fallback)", len(s.AnyOf))
	}
	hasFallback := false
	for _, b := range s.AnyOf {
		if b.Title == "untyped_action_result" {
			hasFallback = true
		}
	}
	if !hasFallback {
		t.Error("expected untyped_action_result fallback branch")
	}
}

// TestMetaToolOutputSchema_EmptyOrAllUntypedReturnsNil verifies that the
// helper returns untyped nil (assignable cleanly to a *mcp.Tool's any field)
// when there are no typed routes.
func TestMetaToolOutputSchema_EmptyOrAllUntypedReturnsNil(t *testing.T) {
	if got := MetaToolOutputSchema(ActionMap{}); got != nil {
		t.Errorf("empty routes should return nil, got %v", got)
	}
	all := ActionMap{
		"a": Route(func(_ context.Context, _ map[string]any) (any, error) { return struct{}{}, nil }),
		"b": Route(func(_ context.Context, _ map[string]any) (any, error) { return struct{}{}, nil }),
	}
	if got := MetaToolOutputSchema(all); got != nil {
		t.Errorf("all-untyped routes should return nil, got %v", got)
	}
}

// TestMetaToolOutputSchema_RelaxesAdditionalProperties verifies that the
// schemaFor pre-processing clears additionalProperties:false on object
// branches so the dispatcher's next_steps injection does not break SDK
// output validation.
func TestMetaToolOutputSchema_RelaxesAdditionalProperties(t *testing.T) {
	type out struct {
		Field string `json:"field"`
	}
	routes := ActionMap{
		"a": RouteAction[struct{}, out](nil, func(_ context.Context, _ *gitlabclient.Client, _ struct{}) (out, error) {
			return out{}, nil
		}),
	}
	s := MetaToolOutputSchema(routes).(*jsonschema.Schema)
	branch := s.AnyOf[0]
	if branch.AdditionalProperties != nil {
		t.Errorf("expected AdditionalProperties to be nil (relaxed), got %+v", branch.AdditionalProperties)
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
