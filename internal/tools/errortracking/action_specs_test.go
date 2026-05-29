// action_specs_test.go contains canonical-route tests for error tracking actions.
package errortracking

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_Metadata verifies canonical metadata for error tracking actions.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	byTool := errorTrackingSpecsByTool(t, specs)

	if len(specs) != 5 {
		t.Fatalf("len(ActionSpecs) = %d, want 5", len(specs))
	}
	if len(byTool) != len(specs) {
		t.Fatalf("unique individual tools = %d, want %d", len(byTool), len(specs))
	}
	if !byTool["gitlab_delete_error_tracking_client_key"].Route.Destructive {
		t.Fatal("gitlab_delete_error_tracking_client_key should be destructive")
	}
	if byTool["gitlab_get_error_tracking_settings"].Usage == "" {
		t.Fatal("gitlab_get_error_tracking_settings should define usage")
	}
	if len(byTool["gitlab_list_error_tracking_client_keys"].Aliases) == 0 {
		t.Fatal("gitlab_list_error_tracking_client_keys should define aliases")
	}
	if byTool["gitlab_delete_error_tracking_client_key"].ParameterGuidance["key_id"].SemanticRole == "" {
		t.Fatal("gitlab_delete_error_tracking_client_key should define key_id parameter guidance")
	}
}

// TestActionSpecs_CallAllRoutes exercises every error tracking tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/projects/1/error_tracking/settings", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covSettingsJSON)
	})
	handler.HandleFunc("PATCH /api/v4/projects/1/error_tracking/settings", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covSettingsJSON)
	})
	handler.HandleFunc("GET /api/v4/projects/1/error_tracking/client_keys", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+covKeyJSON+`]`)
	})
	handler.HandleFunc("POST /api/v4/projects/1/error_tracking/client_keys", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, covKeyJSON)
	})
	handler.HandleFunc("DELETE /api/v4/projects/1/error_tracking/client_keys/10", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	byTool := errorTrackingSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, handler)))

	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"get_settings", "gitlab_get_error_tracking_settings", map[string]any{"project_id": "1"}},
		{"enable_disable", "gitlab_enable_disable_error_tracking", map[string]any{"project_id": "1", "active": true, "integrated": true}},
		{"list_client_keys", "gitlab_list_error_tracking_client_keys", map[string]any{"project_id": "1", "page": 1, "per_page": 20}},
		{"create_client_key", "gitlab_create_error_tracking_client_key", map[string]any{"project_id": "1"}},
		{"delete_client_key", "gitlab_delete_error_tracking_client_key", map[string]any{"project_id": "1", "key_id": 10}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

// TestActionSpecs_ErrorPaths verifies canonical routes propagate backend errors.
func TestActionSpecs_ErrorPaths(t *testing.T) {
	byTool := errorTrackingSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))))

	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"get_settings", "gitlab_get_error_tracking_settings", map[string]any{"project_id": "1"}},
		{"enable_disable", "gitlab_enable_disable_error_tracking", map[string]any{"project_id": "1", "active": true, "integrated": true}},
		{"list_client_keys", "gitlab_list_error_tracking_client_keys", map[string]any{"project_id": "1", "page": 1, "per_page": 20}},
		{"create_client_key", "gitlab_create_error_tracking_client_key", map[string]any{"project_id": "1"}},
		{"delete_client_key", "gitlab_delete_error_tracking_client_key", map[string]any{"project_id": "1", "key_id": 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err == nil {
				t.Fatalf("Route.Handler(%s) expected error", tt.tool)
			}
		})
	}
}

// TestActionSpecs_DeleteOutput verifies the delete route preserves its success message.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/error_tracking/client_keys/10" || r.Method != http.MethodDelete {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	byTool := errorTrackingSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_delete_error_tracking_client_key"].Route.Handler(t.Context(), map[string]any{
		"project_id": "1",
		"key_id":     10,
	})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_delete_error_tracking_client_key) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_delete_error_tracking_client_key) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted error tracking client key." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
	byTool := errorTrackingSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_delete_error_tracking_client_key"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test error tracking destructive confirmation.",
		Icons:       toolutil.IconAlert,
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
		Name: "gitlab_delete_error_tracking_client_key",
		Arguments: map[string]any{
			"project_id": "1",
			"key_id":     float64(10),
		},
	})
	if err != nil {
		t.Fatalf("CallTool returned transport error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result when confirmation is declined")
	}
	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			if textContent.Text == "" {
				t.Error("expected non-empty cancellation message")
			}
			return
		}
	}
	t.Error("expected text content in cancellation result")
}

func errorTrackingSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
