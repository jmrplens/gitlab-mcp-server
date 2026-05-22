// action_specs_test.go contains canonical-route tests for runner actions.
package runners

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionSpecRunnerJSON         = `{"id":10,"description":"runner-1","name":"r1","paused":false,"is_shared":true,"runner_type":"instance_type","online":true,"status":"online"}`
	actionSpecRunnerDetailsJSON  = `{"id":10,"description":"runner-1","name":"r1","paused":false,"is_shared":true,"runner_type":"instance_type","online":true,"status":"online","tag_list":["docker"],"run_untagged":true,"locked":false,"access_level":"not_protected","maximum_timeout":3600}`
	actionSpecRunnerTokenJSON    = `{"token":"new-tok","token_expires_at":"2026-12-01T00:00:00Z"}`
	actionSpecRunnerRegTokenJSON = `{"token":"reg-tok-new","token_expires_at":"2026-12-01T00:00:00Z"}`
)

// TestActionSpecs_CallRunnerRoutes exercises runner tools through their canonical routes.
func TestActionSpecs_CallRunnerRoutes(t *testing.T) {
	byTool := runnerSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, runnerActionHandler())))

	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_runner_list", map[string]any{}},
		{"get", "gitlab_runner_get", map[string]any{"runner_id": 10}},
		{"update", "gitlab_runner_update", map[string]any{"runner_id": 10, "description": "updated"}},
		{"remove", "gitlab_runner_remove", map[string]any{"runner_id": 10}},
		{"jobs", "gitlab_runner_jobs", map[string]any{"runner_id": 10}},
		{"list_project", "gitlab_runner_list_project", map[string]any{"project_id": "42"}},
		{"enable_project", "gitlab_runner_enable_project", map[string]any{"project_id": "42", "runner_id": 5}},
		{"disable_project", "gitlab_runner_disable_project", map[string]any{"project_id": "42", "runner_id": 5}},
		{"list_group", "gitlab_runner_list_group", map[string]any{"group_id": "7"}},
		{"register", "gitlab_runner_register", map[string]any{"token": "reg-token-123"}},
		{"delete_registered", "gitlab_runner_delete_registered", map[string]any{"runner_id": 99}},
		{"verify", "gitlab_runner_verify", map[string]any{"token": "valid-token"}},
		{"reset_token", "gitlab_runner_reset_token", map[string]any{"runner_id": 10}},
		{"list_all", "gitlab_runner_list_all", map[string]any{}},
		{"delete_by_token", "gitlab_runner_delete_by_token", map[string]any{"token": "del-token"}},
		{"reset_instance_reg_token", "gitlab_runner_reset_instance_reg_token", map[string]any{}},
		{"reset_group_reg_token", "gitlab_runner_reset_group_reg_token", map[string]any{"group_id": "42"}},
		{"reset_project_reg_token", "gitlab_runner_reset_project_reg_token", map[string]any{"project_id": "99"}},
		{"list_managers", "gitlab_runner_list_managers", map[string]any{"runner_id": 10}},
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

// TestActionSpecs_DeleteOutputs verifies destructive routes preserve their success messages.
func TestActionSpecs_DeleteOutputs(t *testing.T) {
	byTool := runnerSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, runnerActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
		want string
	}{
		{"gitlab_runner_remove", map[string]any{"runner_id": 10}, "Successfully deleted runner."},
		{"gitlab_runner_disable_project", map[string]any{"project_id": "42", "runner_id": 5}, "Successfully deleted project runner assignment."},
		{"gitlab_runner_delete_registered", map[string]any{"runner_id": 99}, "Successfully deleted registered runner."},
		{"gitlab_runner_delete_by_token", map[string]any{"token": "del-token"}, "Successfully deleted registered runner."},
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
				t.Fatalf("delete message = %q", out.Message)
			}
		})
	}
}

// TestActionSpecs_VerifyOutput verifies the verify route preserves its success message.
func TestActionSpecs_VerifyOutput(t *testing.T) {
	byTool := runnerSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, runnerActionHandler())))

	result, err := byTool["gitlab_runner_verify"].Route.Handler(t.Context(), map[string]any{"token": "valid-token"})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_runner_verify) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_runner_verify) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Runner token is valid." {
		t.Fatalf("verify message = %q", out.Message)
	}
}

// TestActionSpecs_ErrorOutputs verifies wrapper routes propagate backend failures.
func TestActionSpecs_ErrorOutputs(t *testing.T) {
	byTool := runnerSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_runner_remove", map[string]any{"runner_id": 10}},
		{"gitlab_runner_disable_project", map[string]any{"project_id": "42", "runner_id": 5}},
		{"gitlab_runner_delete_registered", map[string]any{"runner_id": 99}},
		{"gitlab_runner_delete_by_token", map[string]any{"token": "del-token"}},
		{"gitlab_runner_verify", map[string]any{"token": "valid-token"}},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			_, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err == nil {
				t.Fatalf("expected route error for %s", tt.tool)
			}
		})
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
	byTool := runnerSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	for _, toolName := range []string{"gitlab_runner_remove", "gitlab_runner_disable_project", "gitlab_runner_delete_registered", "gitlab_runner_delete_by_token"} {
		toolutil.RegisterSurfaceToolFromSpec(server, byTool[toolName], toolutil.SurfaceToolRegisterOptions{
			Description: "Test runner destructive confirmation.",
			Icons:       toolutil.IconRunner,
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
		{"gitlab_runner_remove", map[string]any{"runner_id": 10}},
		{"gitlab_runner_disable_project", map[string]any{"project_id": "42", "runner_id": 5}},
		{"gitlab_runner_delete_registered", map[string]any{"runner_id": 99}},
		{"gitlab_runner_delete_by_token", map[string]any{"token": "del-token"}},
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

func runnerActionHandler() http.Handler {
	handler := http.NewServeMux()
	handler.HandleFunc("/api/v4/runners", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testutil.RespondJSON(w, http.StatusOK, `[`+actionSpecRunnerJSON+`]`)
		case http.MethodPost:
			testutil.RespondJSON(w, http.StatusCreated, actionSpecRunnerJSON)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	handler.HandleFunc("GET /api/v4/runners/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, actionSpecRunnerDetailsJSON)
	})
	handler.HandleFunc("PUT /api/v4/runners/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, actionSpecRunnerDetailsJSON)
	})
	handler.HandleFunc("DELETE /api/v4/runners/10", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("GET /api/v4/runners/10/jobs", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":100,"name":"build","status":"success","ref":"main","stage":"build","pipeline":{"id":50},"web_url":"https://example.com/jobs/100"}]`)
	})
	handler.HandleFunc("GET /api/v4/projects/42/runners", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+actionSpecRunnerJSON+`]`)
	})
	handler.HandleFunc("POST /api/v4/projects/42/runners", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, actionSpecRunnerJSON)
	})
	handler.HandleFunc("DELETE /api/v4/projects/42/runners/5", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("GET /api/v4/groups/7/runners", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+actionSpecRunnerJSON+`]`)
	})
	handler.HandleFunc("DELETE /api/v4/runners/99", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("POST /api/v4/runners/verify", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler.HandleFunc("POST /api/v4/runners/10/reset_authentication_token", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, actionSpecRunnerTokenJSON)
	})
	handler.HandleFunc("GET /api/v4/runners/all", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+actionSpecRunnerJSON+`]`)
	})
	handler.HandleFunc("POST /api/v4/runners/reset_registration_token", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, actionSpecRunnerRegTokenJSON)
	})
	handler.HandleFunc("POST /api/v4/groups/42/runners/reset_registration_token", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, actionSpecRunnerRegTokenJSON)
	})
	handler.HandleFunc("POST /api/v4/projects/99/runners/reset_registration_token", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, actionSpecRunnerRegTokenJSON)
	})
	handler.HandleFunc("GET /api/v4/runners/10/managers", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":10,"system_id":"sys-01","version":"16.0","platform":"linux","architecture":"amd64","ip_address":"10.0.0.1","status":"online"}]`)
	})
	return handler
}

func runnerSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
