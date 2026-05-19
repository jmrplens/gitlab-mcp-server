// action_specs_test.go contains canonical-route tests for CI/CD job token scope actions.
package jobtokenscope

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionSpecSettingsJSON   = `{"inbound_enabled": true}`
	actionSpecProjectJSON    = `[{"id": 10, "name": "proj-a", "path_with_namespace": "grp/proj-a", "web_url": "https://gitlab.example.com/grp/proj-a"}]`
	actionSpecAddProjectJSON = `{"source_project_id": 42, "target_project_id": 99}`
	actionSpecGroupJSON      = `[{"id": 5, "name": "group-a", "full_path": "group-a", "web_url": "https://gitlab.example.com/groups/group-a"}]`
	actionSpecAddGroupJSON   = `{"source_project_id": 42, "target_group_id": 5}`
)

// TestActionSpecs_CallAllRoutes exercises every job token scope tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := jobTokenScopeSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, jobTokenScopeActionHandler())))

	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"get_access_settings", "gitlab_get_job_token_access_settings", map[string]any{"project_id": "42"}},
		{"patch_access_settings", "gitlab_patch_job_token_access_settings", map[string]any{"project_id": "42", "enabled": true}},
		{"list_inbound_allowlist", "gitlab_list_job_token_inbound_allowlist", map[string]any{"project_id": "42"}},
		{"add_project_allowlist", "gitlab_add_project_job_token_allowlist", map[string]any{"project_id": "42", "target_project_id": 99}},
		{"remove_project_allowlist", "gitlab_remove_project_job_token_allowlist", map[string]any{"project_id": "42", "target_project_id": 99}},
		{"list_group_allowlist", "gitlab_list_job_token_group_allowlist", map[string]any{"project_id": "42"}},
		{"add_group_allowlist", "gitlab_add_group_job_token_allowlist", map[string]any{"project_id": "42", "target_group_id": 5}},
		{"remove_group_allowlist", "gitlab_remove_group_job_token_allowlist", map[string]any{"project_id": "42", "target_group_id": 5}},
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

// TestActionSpecs_DeleteErrors verifies remove routes propagate backend errors.
func TestActionSpecs_DeleteErrors(t *testing.T) {
	byTool := jobTokenScopeSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_remove_project_job_token_allowlist", map[string]any{"project_id": "42", "target_project_id": 99}},
		{"gitlab_remove_group_job_token_allowlist", map[string]any{"project_id": "42", "target_group_id": 99}},
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

// TestActionSpecs_DeleteOutput verifies remove routes preserve their success messages.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	byTool := jobTokenScopeSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, jobTokenScopeActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
		want string
	}{
		{"gitlab_remove_project_job_token_allowlist", map[string]any{"project_id": "42", "target_project_id": 99}, "Successfully deleted project from job token allowlist."},
		{"gitlab_remove_group_job_token_allowlist", map[string]any{"project_id": "42", "target_group_id": 5}, "Successfully deleted group from job token allowlist."},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			result, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			out, ok := result.(toolutil.DeleteOutput)
			if !ok {
				t.Fatalf("Route.Handler(%s) returned %T, want toolutil.DeleteOutput", tt.tool, result)
			}
			if out.Message != tt.want {
				t.Fatalf("delete message = %q, want %q", out.Message, tt.want)
			}
		})
	}
}

// TestCatalogSurface_RemoveConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_RemoveConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
	byTool := jobTokenScopeSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	for _, toolName := range []string{"gitlab_remove_project_job_token_allowlist", "gitlab_remove_group_job_token_allowlist"} {
		toolutil.RegisterSurfaceToolFromSpec(server, byTool[toolName], toolutil.SurfaceToolRegisterOptions{
			Description: "Test job token scope destructive confirmation.",
			Icons:       toolutil.IconToken,
		})
	}

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

	tests := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_remove_project_job_token_allowlist", map[string]any{"project_id": "42", "target_project_id": 99}},
		{"gitlab_remove_group_job_token_allowlist", map[string]any{"project_id": "42", "target_group_id": 99}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, callErr := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if callErr != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.name, callErr)
			}
			if result == nil {
				t.Fatalf("expected non-nil result for declined confirmation on %s", tt.name)
			}
		})
	}
}

func jobTokenScopeActionHandler() http.Handler {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/projects/42/job_token_scope", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, actionSpecSettingsJSON)
	})
	handler.HandleFunc("PATCH /api/v4/projects/42/job_token_scope", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("GET /api/v4/projects/42/job_token_scope/allowlist", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, actionSpecProjectJSON)
	})
	handler.HandleFunc("POST /api/v4/projects/42/job_token_scope/allowlist", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, actionSpecAddProjectJSON)
	})
	handler.HandleFunc("DELETE /api/v4/projects/42/job_token_scope/allowlist/99", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("GET /api/v4/projects/42/job_token_scope/groups_allowlist", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, actionSpecGroupJSON)
	})
	handler.HandleFunc("POST /api/v4/projects/42/job_token_scope/groups_allowlist", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, actionSpecAddGroupJSON)
	})
	handler.HandleFunc("DELETE /api/v4/projects/42/job_token_scope/groups_allowlist/5", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	return handler
}

func jobTokenScopeSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
