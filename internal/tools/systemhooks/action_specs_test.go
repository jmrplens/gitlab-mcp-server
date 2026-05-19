// action_specs_test.go contains canonical-route tests for system hook actions.
package systemhooks

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_CallAllRoutes exercises every system hook tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := systemHookSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, systemHookActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_list_system_hooks", map[string]any{}},
		{"gitlab_get_system_hook", map[string]any{"id": 1}},
		{"gitlab_add_system_hook", map[string]any{"url": testHookURL}},
		{"gitlab_edit_system_hook", map[string]any{"id": 1, "url": testHookURL}},
		{"gitlab_test_system_hook", map[string]any{"id": 1}},
		{"gitlab_set_system_hook_url_variable", map[string]any{"id": 1, "key": "env", "value": "prod"}},
		{"gitlab_delete_system_hook_url_variable", map[string]any{"id": 1, "key": "env"}},
		{"gitlab_delete_system_hook", map[string]any{"id": 1}},
	}

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			result, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
			}
		})
	}
}

// TestActionSpecs_ErrorPaths verifies canonical routes propagate backend failures.
func TestActionSpecs_ErrorPaths(t *testing.T) {
	byTool := systemHookSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_list_system_hooks", map[string]any{}},
		{"gitlab_get_system_hook", map[string]any{"id": 1}},
		{"gitlab_add_system_hook", map[string]any{"url": "https://example.com"}},
		{"gitlab_edit_system_hook", map[string]any{"id": 1, "url": "https://example.com"}},
		{"gitlab_set_system_hook_url_variable", map[string]any{"id": 1, "key": "env", "value": "prod"}},
		{"gitlab_delete_system_hook_url_variable", map[string]any{"id": 1, "key": "env"}},
		{"gitlab_delete_system_hook", map[string]any{"id": 1}},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			_, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err == nil {
				t.Fatal("expected route error")
			}
		})
	}
}

// TestActionSpecs_DeleteOutput verifies the delete route preserves its success message.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	byTool := systemHookSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, systemHookActionHandler())))

	result, err := byTool["gitlab_delete_system_hook"].Route.Handler(t.Context(), map[string]any{"id": 1})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_delete_system_hook) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_delete_system_hook) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted system hook." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestActionSpecs_URLVariableOutputs verifies URL variable routes return clear success messages.
func TestActionSpecs_URLVariableOutputs(t *testing.T) {
	byTool := systemHookSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, systemHookActionHandler())))

	setResult, err := byTool["gitlab_set_system_hook_url_variable"].Route.Handler(t.Context(), map[string]any{"id": 1, "key": "env", "value": "prod"})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_set_system_hook_url_variable) error: %v", err)
	}
	setOut, ok := setResult.(toolutil.VoidOutput)
	if !ok {
		t.Fatalf("set route returned %T, want toolutil.VoidOutput", setResult)
	}
	if !strings.Contains(setOut.Message, "env") {
		t.Fatalf("set message = %q, want variable key", setOut.Message)
	}

	deleteResult, err := byTool["gitlab_delete_system_hook_url_variable"].Route.Handler(t.Context(), map[string]any{"id": 1, "key": "env"})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_delete_system_hook_url_variable) error: %v", err)
	}
	deleteOut, ok := deleteResult.(toolutil.VoidOutput)
	if !ok {
		t.Fatalf("delete route returned %T, want toolutil.VoidOutput", deleteResult)
	}
	if !strings.Contains(deleteOut.Message, "env") {
		t.Fatalf("delete message = %q, want variable key", deleteOut.Message)
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
	byTool := systemHookSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_delete_system_hook"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test system hook destructive confirmation.",
		Icons:       toolutil.IconIntegration,
	})

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "decline"}, nil
		},
	})
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_delete_system_hook",
		Arguments: map[string]any{"id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

func systemHookActionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/hooks":
			testutil.RespondJSON(w, http.StatusOK, `[`+hookJSON+`]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/hooks/1":
			testutil.RespondJSON(w, http.StatusOK, hookJSON)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/hooks":
			testutil.RespondJSON(w, http.StatusCreated, hookJSON)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/hooks/1":
			testutil.RespondJSON(w, http.StatusOK, hookJSON)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/hooks/1":
			testutil.RespondJSON(w, http.StatusOK, `{"event_name":"project_create","project_id":42}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/hooks/1/url_variables/env":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/hooks/1/url_variables/env":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/hooks/1":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
}

func systemHookSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		toolName := spec.IndividualTool.Name
		if toolName == "" {
			t.Fatalf("spec %s missing IndividualTool.Name", spec.Name)
		}
		if _, exists := byTool[toolName]; exists {
			t.Fatalf("duplicate individual tool %q", toolName)
		}
		byTool[toolName] = spec
	}
	return byTool
}
