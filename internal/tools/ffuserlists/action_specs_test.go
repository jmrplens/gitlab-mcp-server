// action_specs_test.go contains canonical-route tests for feature flag user list actions.
package ffuserlists

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_CallAllRoutes exercises every feature flag user list tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/projects/42/feature_flags_user_lists", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+covUserListJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/projects/42/feature_flags_user_lists/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covUserListJSON)
	})
	handler.HandleFunc("POST /api/v4/projects/42/feature_flags_user_lists", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, covUserListJSON)
	})
	handler.HandleFunc("PUT /api/v4/projects/42/feature_flags_user_lists/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covUserListJSON)
	})
	handler.HandleFunc("DELETE /api/v4/projects/42/feature_flags_user_lists/10", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	byTool := userListSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, handler)))

	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_ff_user_list_list", map[string]any{"project_id": "42"}},
		{"get", "gitlab_ff_user_list_get", map[string]any{"project_id": "42", "user_list_iid": 10}},
		{"create", "gitlab_ff_user_list_create", map[string]any{"project_id": "42", "name": "test", "user_xids": "u1"}},
		{"update", "gitlab_ff_user_list_update", map[string]any{"project_id": "42", "user_list_iid": 10, "name": "updated"}},
		{"delete", "gitlab_ff_user_list_delete", map[string]any{"project_id": "42", "user_list_iid": 10}},
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

// TestActionSpecs_UserListIIDGuidance verifies get, update, and delete actions
// tell models to use the returned IID instead of the user-list name.
func TestActionSpecs_UserListIIDGuidance(t *testing.T) {
	byTool := userListSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.NewServeMux())))

	for _, toolName := range []string{"gitlab_ff_user_list_get", "gitlab_ff_user_list_update", "gitlab_ff_user_list_delete"} {
		guidance := byTool[toolName].ParameterGuidance["user_list_iid"]
		if guidance.SemanticRole != "feature_flag_user_list_iid" {
			t.Fatalf("%s user_list_iid SemanticRole = %q, want feature_flag_user_list_iid", toolName, guidance.SemanticRole)
		}
		if !containsText(guidance.CommonConfusions, "user list name") {
			t.Fatalf("%s user_list_iid CommonConfusions = %v, want name warning", toolName, guidance.CommonConfusions)
		}
		description := schemaPropertyDescription(t, byTool[toolName].Route.InputSchema, "user_list_iid")
		if !strings.Contains(description, "do not use the user list name") {
			t.Fatalf("%s user_list_iid schema description = %q, want name warning", toolName, description)
		}
	}
}

// TestActionSpecs_DeleteError verifies that the delete route propagates backend errors.
func TestActionSpecs_DeleteError(t *testing.T) {
	byTool := userListSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))))

	_, err := byTool["gitlab_ff_user_list_delete"].Route.Handler(t.Context(), map[string]any{
		"project_id":    "my-project",
		"user_list_iid": 10,
	})
	if err == nil {
		t.Fatal("expected error from delete with failing backend")
	}
}

// TestActionSpecs_DeleteOutput verifies the delete route preserves its success message.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/feature_flags_user_lists/10" || r.Method != http.MethodDelete {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	byTool := userListSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_ff_user_list_delete"].Route.Handler(t.Context(), map[string]any{
		"project_id":    "42",
		"user_list_iid": 10,
	})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_ff_user_list_delete) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_ff_user_list_delete) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted feature flag user list." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
	byTool := userListSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_ff_user_list_delete"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test feature flag user list destructive confirmation.",
		Icons:       toolutil.IconUser,
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
		Name:      "gitlab_ff_user_list_delete",
		Arguments: map[string]any{"project_id": "p", "user_list_iid": float64(1)},
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

func userListSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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

func schemaPropertyDescription(t *testing.T, schema map[string]any, propertyName string) string {
	t.Helper()
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties = %T, want map[string]any", schema["properties"])
	}
	property, ok := properties[propertyName].(map[string]any)
	if !ok {
		t.Fatalf("schema property %q = %T, want map[string]any", propertyName, properties[propertyName])
	}
	description, ok := property["description"].(string)
	if !ok {
		t.Fatalf("schema property %q description = %T, want string", propertyName, property["description"])
	}
	return description
}

func containsText(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
