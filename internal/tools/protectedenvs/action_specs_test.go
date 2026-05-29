// action_specs_test.go contains canonical-route tests for protected environment actions.
package protectedenvs

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_CallAllRoutes exercises every protected environment tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := protectedEnvironmentSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, protectedEnvironmentsActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_protected_environment_list", map[string]any{"project_id": "42"}},
		{"gitlab_protected_environment_get", map[string]any{"project_id": "42", "environment": "production"}},
		{"gitlab_protected_environment_protect", map[string]any{"project_id": "42", "name": "staging"}},
		{"gitlab_protected_environment_update", map[string]any{"project_id": "42", "environment": "production"}},
		{"gitlab_protected_environment_unprotect", map[string]any{"project_id": "42", "environment": "production"}},
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

// TestActionSpecs_ErrorPaths verifies get and unprotect routes propagate 404s.
func TestActionSpecs_ErrorPaths(t *testing.T) {
	byTool := protectedEnvironmentSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
	}))))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_protected_environment_get", map[string]any{"project_id": "42", "environment": "prod"}},
		{"gitlab_protected_environment_unprotect", map[string]any{"project_id": "42", "environment": "prod"}},
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

// TestActionSpecs_UnprotectOutput verifies the unprotect route preserves its success message.
func TestActionSpecs_UnprotectOutput(t *testing.T) {
	byTool := protectedEnvironmentSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, protectedEnvironmentsActionHandler())))

	result, err := byTool["gitlab_protected_environment_unprotect"].Route.Handler(t.Context(), map[string]any{
		"project_id":  "42",
		"environment": "production",
	})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_protected_environment_unprotect) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_protected_environment_unprotect) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted protected environment." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestActionSpecs_ProtectRequiresDeployAccessLevels verifies discovery schemas
// advertise the access rule required to create a protected environment.
func TestActionSpecs_ProtectRequiresDeployAccessLevels(t *testing.T) {
	byTool := protectedEnvironmentSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, protectedEnvironmentsActionHandler())))
	schema := byTool["gitlab_protected_environment_protect"].Route.InputSchema
	if !schemaRequiredIncludes(schema, "deploy_access_levels") {
		t.Fatalf("protect required fields = %v, want deploy_access_levels", schema["required"])
	}
}

// TestCatalogSurface_UnprotectConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_UnprotectConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
	byTool := protectedEnvironmentSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_protected_environment_unprotect"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test protected environment destructive confirmation.",
		Icons:       toolutil.IconShield,
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
		Name:      "gitlab_protected_environment_unprotect",
		Arguments: map[string]any{"project_id": "42", "environment": "production"},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

func protectedEnvironmentsActionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == pathProtectedEnvs:
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+envJSON+`]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
		case r.Method == http.MethodGet && r.URL.Path == pathProtectedEnv1:
			testutil.RespondJSON(w, http.StatusOK, envJSON)
		case r.Method == http.MethodPost && r.URL.Path == pathProtectedEnvs:
			testutil.RespondJSON(w, http.StatusCreated, envJSON)
		case r.Method == http.MethodPut && r.URL.Path == pathProtectedEnv1:
			testutil.RespondJSON(w, http.StatusOK, envJSON)
		case r.Method == http.MethodDelete && r.URL.Path == pathProtectedEnv1:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
}

func protectedEnvironmentSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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

func schemaRequiredIncludes(schema map[string]any, name string) bool {
	switch required := schema["required"].(type) {
	case []any:
		for _, raw := range required {
			if field, ok := raw.(string); ok && field == name {
				return true
			}
		}
	case []string:
		return slices.Contains(required, name)
	}
	return false
}
