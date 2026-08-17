// action_specs_test.go contains canonical-route tests for protected environment actions.
package protectedenvs

import (
	"context"
	"net/http"
	"slices"
	"strings"
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
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
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

// TestProtectedEnvironmentDescription_AllActions verifies each protected
// environment action carries a "Returns: … See also: …" individual-tool
// description (R-META) and that an unknown tool name yields an empty string.
func TestProtectedEnvironmentDescription_AllActions(t *testing.T) {
	tools := []string{
		"gitlab_protected_environment_list",
		"gitlab_protected_environment_get",
		"gitlab_protected_environment_protect",
		"gitlab_protected_environment_update",
		"gitlab_protected_environment_unprotect",
	}
	for _, name := range tools {
		desc := protectedEnvironmentDescription(name)
		if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
			t.Errorf("%s description missing Returns/See also: %q", name, desc)
		}
	}
	if got := protectedEnvironmentDescription("gitlab_unknown"); got != "" {
		t.Errorf("unknown tool description = %q, want empty", got)
	}
}

// TestProtectedEnvironmentMeta_PerToolDiscovery verifies every protected
// environment tool carries action-specific Usage, at least two
// natural-language Aliases beyond the bare tool name, and non-empty
// RelatedActions (R-META). It also covers decorateProtectedEnvironmentMeta's
// no-op path for an unknown tool name, which keeps the shared defaults.
func TestProtectedEnvironmentMeta_PerToolDiscovery(t *testing.T) {
	const sharedUsagePrefix = "Use project protected environment actions for project deployment gates."

	tools := []string{
		"gitlab_protected_environment_list",
		"gitlab_protected_environment_get",
		"gitlab_protected_environment_protect",
		"gitlab_protected_environment_update",
		"gitlab_protected_environment_unprotect",
	}
	for _, name := range tools {
		opts := protectedEnvironmentOptions(name)
		if strings.HasPrefix(opts.Usage, sharedUsagePrefix) {
			t.Errorf("%s still uses generic shared Usage: %q", name, opts.Usage)
		}
		if len(opts.Aliases) < 2 {
			t.Errorf("%s has %d aliases, want >= 2", name, len(opts.Aliases))
		}
		for _, alias := range opts.Aliases {
			if alias == name {
				t.Errorf("%s alias list still contains the bare tool name", name)
			}
			if strings.Contains(alias, "group") {
				t.Errorf("%s alias %q overlaps group-level wording", name, alias)
			}
		}
		if len(opts.RelatedActions) == 0 {
			t.Errorf("%s has empty RelatedActions", name)
		}
	}

	// Unknown tool: decorator is a no-op, so the shared defaults remain.
	def := protectedEnvironmentOptions("gitlab_unknown_tool")
	if !strings.HasPrefix(def.Usage, sharedUsagePrefix) {
		t.Errorf("unknown tool Usage = %q, want shared default", def.Usage)
	}
	if len(def.Aliases) != 1 || def.Aliases[0] != "gitlab_unknown_tool" {
		t.Errorf("unknown tool Aliases = %v, want only the tool name", def.Aliases)
	}
}
