// action_specs_test.go contains canonical-route tests for project alias actions.
package projectaliases

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_Metadata verifies canonical metadata for project alias actions.
func TestActionSpecs_Metadata(t *testing.T) {
	byTool := projectAliasSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, projectAliasesActionHandler())))
	if len(byTool) != 4 {
		t.Fatalf("len(ActionSpecs) = %d, want 4", len(byTool))
	}
	for _, spec := range byTool {
		if spec.OwnerPackage != "projectaliases" {
			t.Fatalf("OwnerPackage for %s = %q, want projectaliases", spec.Name, spec.OwnerPackage)
		}
		if spec.Usage == "" {
			t.Fatalf("Usage for %s is empty", spec.Name)
		}
		if len(spec.Aliases) == 0 {
			t.Fatalf("Aliases for %s are empty", spec.Name)
		}
	}
}

// TestActionSpecs_CallAllRoutes exercises every project alias tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := projectAliasSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, projectAliasesActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_list_project_aliases", map[string]any{}},
		{"gitlab_get_project_alias", map[string]any{"name": "alias-one"}},
		{"gitlab_create_project_alias", map[string]any{"name": "new-alias", "project_id": 42}},
		{"gitlab_delete_project_alias", map[string]any{"name": "alias-one"}},
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
	byTool := projectAliasSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))))

	_, err := byTool["gitlab_delete_project_alias"].Route.Handler(t.Context(), map[string]any{"name": "alias-one"})
	if err == nil {
		t.Fatal("expected delete route error")
	}
}

// TestActionSpecs_DeleteOutput verifies the delete route preserves its success message.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	byTool := projectAliasSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, projectAliasesActionHandler())))

	result, err := byTool["gitlab_delete_project_alias"].Route.Handler(t.Context(), map[string]any{"name": "alias-one"})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_delete_project_alias) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_delete_project_alias) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted project alias \"alias-one\"." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
	byTool := projectAliasSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_delete_project_alias"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test project alias destructive confirmation.",
		Icons:       toolutil.IconProject,
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
		Name:      "gitlab_delete_project_alias",
		Arguments: map[string]any{"name": "my-alias"},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			if text.Text == "" {
				t.Error("expected non-empty cancellation message")
			}
			return
		}
	}
	t.Error("expected text content in cancellation result")
}

func projectAliasesActionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/project_aliases"):
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"project_id":100,"name":"alias-one"}]`)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/project_aliases/"):
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"project_id":100,"name":"alias-one"}`)
		case r.Method == http.MethodPost:
			testutil.RespondJSON(w, http.StatusCreated, `{"id":2,"project_id":42,"name":"new-alias"}`)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
}

func projectAliasSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
