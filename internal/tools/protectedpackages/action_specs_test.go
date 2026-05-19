// action_specs_test.go contains canonical-route tests for package protection rule actions.
package protectedpackages

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_CallAllRoutes exercises every package protection rule tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := protectedPackageSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, protectedPackagesActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_list_package_protection_rules", map[string]any{"project_id": testProjectID}},
		{"gitlab_create_package_protection_rule", map[string]any{"project_id": testProjectID, "package_name_pattern": "@scope/pkg*", "package_type": "npm"}},
		{"gitlab_update_package_protection_rule", map[string]any{"project_id": testProjectID, "rule_id": 1}},
		{"gitlab_delete_package_protection_rule", map[string]any{"project_id": testProjectID, "rule_id": 1}},
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

// TestActionSpecs_DeleteError verifies the delete route propagates backend errors.
func TestActionSpecs_DeleteError(t *testing.T) {
	byTool := protectedPackageSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))))

	_, err := byTool["gitlab_delete_package_protection_rule"].Route.Handler(t.Context(), map[string]any{
		"project_id": "1",
		"rule_id":    1,
	})
	if err == nil {
		t.Fatal("expected delete route error")
	}
}

// TestActionSpecs_DeleteOutput verifies the delete route preserves its success message.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	byTool := protectedPackageSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, protectedPackagesActionHandler())))

	result, err := byTool["gitlab_delete_package_protection_rule"].Route.Handler(t.Context(), map[string]any{
		"project_id": testProjectID,
		"rule_id":    1,
	})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_delete_package_protection_rule) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_delete_package_protection_rule) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted package protection rule 1 from project myproject." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
	byTool := protectedPackageSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_delete_package_protection_rule"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test package protection rule destructive confirmation.",
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
		Name:      "gitlab_delete_package_protection_rule",
		Arguments: map[string]any{"project_id": "1", "rule_id": float64(1)},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

func protectedPackagesActionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && path == pathRules:
			testutil.RespondJSONWithPagination(w, http.StatusOK, "["+ruleJSON+"]",
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
		case r.Method == http.MethodPost && path == pathRules:
			testutil.RespondJSON(w, http.StatusCreated, ruleJSON)
		case r.Method == http.MethodPatch && path == pathRule1:
			testutil.RespondJSON(w, http.StatusOK, ruleJSON)
		case r.Method == http.MethodDelete && path == pathRule1:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
}

func protectedPackageSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
