// action_specs_test.go contains canonical-route tests for deploy token actions.
package deploytokens

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_Metadata verifies canonical metadata for deploy token actions.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	byTool := deployTokenSpecsByTool(t, specs)

	if len(specs) != 9 {
		t.Fatalf("len(ActionSpecs) = %d, want 9", len(specs))
	}
	if len(byTool) != len(specs) {
		t.Fatalf("unique individual tools = %d, want %d", len(byTool), len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "deploytokens" {
			t.Fatalf("OwnerPackage for %s = %q, want deploytokens", spec.Name, spec.OwnerPackage)
		}
		if spec.Usage == "" {
			t.Fatalf("Usage for %s should not be empty", spec.Name)
		}
		if len(spec.Aliases) == 0 {
			t.Fatalf("Aliases for %s should not be empty", spec.Name)
		}
	}
	if !byTool["gitlab_deploy_token_delete_project"].Route.Destructive {
		t.Fatal("gitlab_deploy_token_delete_project should be destructive")
	}
	if byTool["gitlab_deploy_token_get_project"].ParameterGuidance["deploy_token_id"].SemanticRole == "" {
		t.Fatal("gitlab_deploy_token_get_project should define deploy_token_id parameter guidance")
	}
}

// TestActionSpecs_DeleteErrors verifies that canonical delete routes return errors when GitLab rejects deletion.
func TestActionSpecs_DeleteErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	client := testutil.NewTestClient(t, mux)
	byTool := deployTokenSpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_deploy_token_delete_project", map[string]any{"project_id": "42", "deploy_token_id": 1}},
		{"gitlab_deploy_token_delete_group", map[string]any{"group_id": "5", "deploy_token_id": 1}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			_, err := byTool[tt.name].Route.Handler(t.Context(), tt.args)
			if err == nil {
				t.Fatalf("Route.Handler(%s) expected error, got nil", tt.name)
			}
		})
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)
	byTool := deployTokenSpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_deploy_token_delete_project", map[string]any{"project_id": "p", "deploy_token_id": float64(1)}},
		{"gitlab_deploy_token_delete_group", map[string]any{"group_id": "g", "deploy_token_id": float64(1)}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
			toolutil.RegisterSurfaceToolFromSpec(server, byTool[tt.name], toolutil.SurfaceToolRegisterOptions{
				Description: "Test deploy token destructive confirmation.",
				Icons:       toolutil.IconToken,
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
			session, connectErr := mcpClient.Connect(ctx, ct, nil)
			if connectErr != nil {
				t.Fatalf("client connect: %v", connectErr)
			}
			t.Cleanup(func() {
				session.Close()
				_ = serverSession.Wait()
			})

			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("expected non-nil result for %s declined confirmation", tt.name)
			}
			found := false
			for _, c := range result.Content {
				if tc, ok := c.(*mcp.TextContent); ok && tc.Text != "" {
					found = true
				}
			}
			if !found {
				t.Errorf("expected non-empty text content in %s cancellation result", tt.name)
			}
		})
	}
}

// TestActionSpecs_CallAllRoutes verifies deploy token tools through canonical routes.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	tokenJSON := `{"id":1,"name":"inst-token","username":"deployer","scopes":["read_repository"]}`
	projTokenJSON := `{"id":2,"name":"proj-token","username":"deployer","scopes":["read_registry"]}`
	grpTokenJSON := `{"id":3,"name":"grp-token","username":"deployer","scopes":["read_repository"]}`
	createdProjJSON := `{"id":4,"name":"new-tok","username":"deployer","token":"secret123","scopes":["read_repository"]}`
	createdGrpJSON := `{"id":5,"name":"grp-tok","username":"deployer","token":"secret456","scopes":["read_repository"]}`

	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/deploy_tokens", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+tokenJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/projects/10/deploy_tokens", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+projTokenJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/groups/5/deploy_tokens", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+grpTokenJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/projects/10/deploy_tokens/2", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, projTokenJSON)
	})
	handler.HandleFunc("GET /api/v4/groups/5/deploy_tokens/3", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, grpTokenJSON)
	})
	handler.HandleFunc("POST /api/v4/projects/10/deploy_tokens", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, createdProjJSON)
	})
	handler.HandleFunc("POST /api/v4/groups/5/deploy_tokens", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, createdGrpJSON)
	})
	handler.HandleFunc("DELETE /api/v4/projects/10/deploy_tokens/2", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("DELETE /api/v4/groups/5/deploy_tokens/3", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	byTool := deployTokenSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, handler)))

	tools := []struct {
		name          string
		args          map[string]any
		deleteMessage string
	}{
		{"gitlab_deploy_token_list_all", map[string]any{}, ""},
		{"gitlab_deploy_token_list_project", map[string]any{"project_id": "10"}, ""},
		{"gitlab_deploy_token_list_group", map[string]any{"group_id": "5"}, ""},
		{"gitlab_deploy_token_get_project", map[string]any{"project_id": "10", "deploy_token_id": 2}, ""},
		{"gitlab_deploy_token_get_group", map[string]any{"group_id": "5", "deploy_token_id": 3}, ""},
		{"gitlab_deploy_token_create_project", map[string]any{"project_id": "10", "name": "new-tok", "scopes": []string{"read_repository"}}, ""},
		{"gitlab_deploy_token_create_group", map[string]any{"group_id": "5", "name": "grp-tok", "scopes": []string{"read_repository"}}, ""},
		{"gitlab_deploy_token_delete_project", map[string]any{"project_id": "10", "deploy_token_id": 2}, "Successfully deleted project deploy token."},
		{"gitlab_deploy_token_delete_group", map[string]any{"group_id": "5", "deploy_token_id": 3}, "Successfully deleted group deploy token."},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := byTool[tt.name].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.name)
			}
			if tt.deleteMessage != "" {
				out, ok := result.(toolutil.DeleteOutput)
				if !ok {
					t.Fatalf("Route.Handler(%s) returned %T, want toolutil.DeleteOutput", tt.name, result)
				}
				if out.Message != tt.deleteMessage {
					t.Fatalf("delete message = %q, want %q", out.Message, tt.deleteMessage)
				}
			}
		})
	}
}

func deployTokenSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
