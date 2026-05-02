// metatool_test.go contains unit tests for the meta-tool dispatch mechanism
// including action validation, parameter unmarshalling, and the wrapAction
// and wrapVoidAction generic helpers.
package tools

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/autoupdate"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/uploads"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	// fmtUnexpectedErr is the format string used when an unexpected error occurs.
	fmtUnexpectedErr = "unexpected error: %v"
)

// TestMakeMetaHandler_EmptyAction verifies that MakeMetaHandler returns a
// descriptive tool error when the action field is empty.
func TestMakeMetaHandler_EmptyAction(t *testing.T) {
	routes := actionMap{
		"create": route(func(_ context.Context, _ map[string]any) (any, error) {
			return "created", nil
		}),
	}

	handler := toolutil.MakeMetaHandler("test_tool", routes, markdownForResult)
	input := MetaToolInput{Action: ""}

	result, raw, err := handler(context.Background(), nil, input)
	if err != nil {
		t.Fatalf("unexpected protocol error: %v", err)
	}
	if raw != nil {
		t.Fatalf("raw result = %#v, want nil", raw)
	}
	got := metaErrorText(t, result)
	if got != "test_tool: 'action' is required. Valid actions: create" {
		t.Errorf("unexpected error: %s", got)
	}
}

// TestMakeMetaHandler_UnknownAction verifies that MakeMetaHandler returns a
// descriptive tool error listing valid actions when an unknown action is provided.
func TestMakeMetaHandler_UnknownAction(t *testing.T) {
	routes := actionMap{
		"create": route(func(_ context.Context, _ map[string]any) (any, error) {
			return "created", nil
		}),
		"list": route(func(_ context.Context, _ map[string]any) (any, error) {
			return "listed", nil
		}),
	}

	handler := toolutil.MakeMetaHandler("test_tool", routes, markdownForResult)
	input := MetaToolInput{Action: "destroy"}

	result, raw, err := handler(context.Background(), nil, input)
	if err != nil {
		t.Fatalf("unexpected protocol error: %v", err)
	}
	if raw != nil {
		t.Fatalf("raw result = %#v, want nil", raw)
	}
	got := metaErrorText(t, result)
	if got != `test_tool: unknown action "destroy". Valid actions: create, list` {
		t.Errorf("unexpected error: %s", got)
	}
}

func metaErrorText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if result == nil || !result.IsError {
		t.Fatalf("result = %#v, want IsError result", result)
	}
	if len(result.Content) == 0 {
		t.Fatal("error result content is empty")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] = %T, want TextContent", result.Content[0])
	}
	return text.Text
}

// TestMakeMetaHandler_ValidAction verifies that MakeMetaHandler dispatches
// to the correct handler and returns its result.
func TestMakeMetaHandler_ValidAction(t *testing.T) {
	routes := actionMap{
		"get": route(func(_ context.Context, params map[string]any) (any, error) {
			return params["id"], nil
		}),
	}

	handler := toolutil.MakeMetaHandler("test_tool", routes, func(any) *mcp.CallToolResult {
		return toolutil.SuccessResult("ok")
	})
	input := MetaToolInput{
		Action: "get",
		Params: map[string]any{"id": "42"},
	}

	_, result, err := handler(context.Background(), nil, input)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if result != "42" {
		t.Errorf("expected 42, got %v", result)
	}
}

// TestUnmarshalParams_ValidStruct verifies that unmarshalParams correctly
// deserializes a params map into the target struct type.
func TestUnmarshalParams_ValidStruct(t *testing.T) {
	params := map[string]any{
		"project_id": "group/repo",
	}

	input, err := unmarshalParams[projects.GetInput](params)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if input.ProjectID != "group/repo" {
		t.Errorf("expected group/repo, got %s", input.ProjectID)
	}
}

// TestUnmarshalParams_NilParams verifies that nil params unmarshal to a
// zero-value struct without error.
func TestUnmarshalParams_NilParams(t *testing.T) {
	_, err := unmarshalParams[projects.GetInput](nil)
	if err != nil {
		t.Fatalf("nil params should unmarshal to zero-value struct, got error: %v", err)
	}
}

// TestWrapAction_Integration verifies that wrapAction properly deserializes
// params and calls the underlying typed handler against a mock GitLab API.
func TestWrapAction_Integration(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		respondJSON(w, http.StatusOK, `{
			"id": 42,
			"name": "test-project",
			"path_with_namespace": "group/test-project",
			"visibility": "private",
			"default_branch": "main",
			"web_url": "https://gitlab.example.com/group/test-project",
			"description": "A test project"
		}`)
	})

	client := newTestClient(t, mux)
	action := wrapAction(client, projects.Get)

	result, err := action(context.Background(), map[string]any{
		"project_id": "42",
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	out, ok := result.(projects.Output)
	if !ok {
		t.Fatalf("expected projects.Output, got %T", result)
	}
	if out.Name != "test-project" {
		t.Errorf("expected test-project, got %s", out.Name)
	}
}

// TestWrapVoidAction_Integration verifies that wrapVoidAction properly
// deserializes params and returns nil result for void handlers.
func TestWrapVoidAction_Integration(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/uploads/5", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	client := newTestClient(t, mux)
	action := wrapVoidAction(client, uploads.Delete)

	result, err := action(context.Background(), map[string]any{
		"project_id": "42",
		"upload_id":  float64(5),
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if result != nil {
		t.Errorf("expected nil result for void action, got %v", result)
	}
}

// TestValidActions_StringSorted verifies that validActionsString returns
// action names in alphabetical order, separated by commas.
func TestValidActions_StringSorted(t *testing.T) {
	routes := actionMap{
		"delete": route(nil),
		"create": route(nil),
		"list":   route(nil),
	}

	got := validActionsString(routes)
	expected := "create, delete, list"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

// TestWrapUpdaterAction_UnmarshalError covers register_mcp_meta.go:58-60
// (unmarshalParams returns error for invalid params).
func TestWrapUpdaterAction_UnmarshalError(t *testing.T) {
	type testInput struct {
		Value string `json:"value"`
	}
	type testOutput struct {
		Result string `json:"result"`
	}

	fn := func(_ context.Context, _ *autoupdate.Updater, _ testInput) (testOutput, error) {
		t.Fatal("wrapped function should not be called on unmarshal error")
		return testOutput{}, nil
	}

	action := wrapUpdaterAction(nil, fn)
	// Pass type-incompatible params: array where string is expected.
	_, err := action(context.Background(), map[string]any{"value": []int{1, 2, 3}})
	if err == nil {
		t.Fatal("expected error for params with type-incompatible value")
	}
	if !strings.Contains(err.Error(), "invalid params") {
		t.Errorf("error = %v, want 'invalid params' context", err)
	}
}

// TestPackageMeta_UnmarshalErrors covers register_meta.go:2200-2240
// (unmarshalParams error branches in 6 inline package meta-tool action closures).
func TestPackageMeta_UnmarshalErrors(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `{}`)
	}))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterAllMeta(server, client, false)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, connectErr := mcpClient.Connect(ctx, ct, nil)
	if connectErr != nil {
		t.Fatalf("client connect: %v", connectErr)
	}
	t.Cleanup(func() { session.Close() })

	// Each action receives type-incompatible params to trigger unmarshalParams error.
	actions := []string{
		"publish", "download", "delete", "file_delete",
		"publish_and_link", "publish_directory",
	}
	for _, action := range actions {
		t.Run(action, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name: "gitlab_package",
				Arguments: map[string]any{
					"action": action,
					"params": map[string]any{
						"project_id": []int{1, 2, 3},
					},
				},
			})
			if err != nil {
				t.Fatalf("CallTool error: %v", err)
			}
			if !result.IsError {
				t.Error("expected IsError=true for invalid params")
			}
		})
	}
}

// TestSetMetaParamSchema_PropagatesToMetaToolSchema verifies that
// SetMetaParamSchema toggles the global mode read by toolutil.MetaToolSchema,
// so all sub-package meta-tool registrations honor the configured mode.
func TestSetMetaParamSchema_PropagatesToMetaToolSchema(t *testing.T) {
	t.Cleanup(func() { SetMetaParamSchema("opaque") })

	routes := actionMap{
		"create": route(func(_ context.Context, _ map[string]any) (any, error) {
			return "ok", nil
		}),
	}

	SetMetaParamSchema("full")
	schema := toolutil.MetaToolSchema(routes)
	if _, hasOneOf := schema["oneOf"]; !hasOneOf {
		t.Errorf("full mode should emit oneOf, got keys=%v", keysOf(schema))
	}

	SetMetaParamSchema("opaque")
	schema = toolutil.MetaToolSchema(routes)
	if _, hasOneOf := schema["oneOf"]; hasOneOf {
		t.Errorf("opaque mode should not emit oneOf, got keys=%v", keysOf(schema))
	}

	SetMetaParamSchema("garbage")
	schema = toolutil.MetaToolSchema(routes)
	if _, hasOneOf := schema["oneOf"]; hasOneOf {
		t.Errorf("unknown mode should fall back to opaque, got keys=%v", keysOf(schema))
	}
}

// keysOf returns the map keys for failure messages in schema-mode tests.
func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// metaSession sets the active param-schema mode, registers meta-tools, and
// resets the mode on cleanup so other tests see the default (opaque).
func metaSessionWithMode(t *testing.T, mode string, handler http.Handler) *mcp.ClientSession {
	t.Helper()
	SetMetaParamSchema(mode)
	t.Cleanup(func() { SetMetaParamSchema("opaque") })
	return newMetaMCPSession(t, handler, false)
}

// TestMetaSchema_DispatchParity verifies that the same {action, params}
// payload reaches the same handler in opaque, compact, and full modes and
// returns the same response body. A successful but divergent response in
// one mode would otherwise pass undetected.
func TestMetaSchema_DispatchParity(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/version":
			respondJSON(w, http.StatusOK, `{"version":"17.0.0"}`)
		case "/api/v4/projects/42":
			respondJSON(w, http.StatusOK, `{"id":42,"name":"test","path_with_namespace":"g/test","visibility":"private","default_branch":"main","web_url":"https://example.com","description":"d"}`)
		default:
			http.NotFound(w, r)
		}
	})

	collectText := func(result *mcp.CallToolResult) string {
		t.Helper()
		var sb strings.Builder
		for _, c := range result.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				sb.WriteString(tc.Text)
			}
		}
		return sb.String()
	}

	responses := map[string]string{}
	for _, mode := range []string{"opaque", "compact", "full"} {
		t.Run(mode, func(t *testing.T) {
			session := metaSessionWithMode(t, mode, handler)
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name: "gitlab_project",
				Arguments: map[string]any{
					"action": "get",
					"params": map[string]any{"project_id": "42"},
				},
			})
			if err != nil {
				t.Fatalf("CallTool(%s): %v", mode, err)
			}
			if result.IsError {
				t.Fatalf("CallTool(%s) returned error result", mode)
			}
			responses[mode] = collectText(result)
		})
	}

	// All three modes must return the same payload because dispatch is
	// independent of the advertised InputSchema shape.
	if responses["opaque"] != responses["compact"] {
		t.Errorf("opaque vs compact response differ:\n  opaque=%q\n  compact=%q", responses["opaque"], responses["compact"])
	}
	if responses["opaque"] != responses["full"] {
		t.Errorf("opaque vs full response differ:\n  opaque=%q\n  full=%q", responses["opaque"], responses["full"])
	}
}

// TestMetaSchema_FullModeAdvertisesOneOf verifies that ListTools in full
// mode emits a structured oneOf in the meta-tool InputSchema, while opaque
// mode does not. This is the LLM-facing visible difference.
func TestMetaSchema_FullModeAdvertisesOneOf(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"version":"17.0.0"}`)
	})

	cases := []struct {
		mode      string
		wantOneOf bool
	}{
		{"opaque", false},
		{"compact", true},
		{"full", true},
	}
	for _, c := range cases {
		t.Run(c.mode, func(t *testing.T) {
			session := metaSessionWithMode(t, c.mode, handler)
			result, err := session.ListTools(context.Background(), nil)
			if err != nil {
				t.Fatalf("ListTools: %v", err)
			}
			var found *mcp.Tool
			for _, tool := range result.Tools {
				if tool.Name == "gitlab_project" {
					found = tool
					break
				}
			}
			if found == nil {
				t.Fatal("gitlab_project not in tools list")
			}
			if found.InputSchema == nil {
				t.Fatal("InputSchema is nil")
			}
			schema, ok := found.InputSchema.(map[string]any)
			if !ok {
				t.Fatalf("InputSchema is not a map: %T", found.InputSchema)
			}
			_, hasOneOf := schema["oneOf"]
			if hasOneOf != c.wantOneOf {
				t.Errorf("mode=%s: oneOf present = %v, want %v", c.mode, hasOneOf, c.wantOneOf)
			}
		})
	}
}
