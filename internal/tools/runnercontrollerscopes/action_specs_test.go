// action_specs_test.go contains canonical-route tests for runner controller scope actions.
package runnercontrollerscopes

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_CallAllRoutes exercises every runner controller scope tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := runnerControllerScopeSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, runnerControllerScopesActionHandler())))

	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_runner_controller_scope_list", map[string]any{"controller_id": 1}},
		{"add_instance", "gitlab_runner_controller_scope_add_instance", map[string]any{"controller_id": 1}},
		{"remove_instance", "gitlab_runner_controller_scope_remove_instance", map[string]any{"controller_id": 1}},
		{"add_runner", "gitlab_runner_controller_scope_add_runner", map[string]any{"controller_id": 1, "runner_id": 42}},
		{"remove_runner", "gitlab_runner_controller_scope_remove_runner", map[string]any{"controller_id": 1, "runner_id": 42}},
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

// TestActionSpecs_RemoveRoutes verifies remove routes are destructive and preserve success messages.
func TestActionSpecs_RemoveRoutes(t *testing.T) {
	byTool := runnerControllerScopeSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, runnerControllerScopesActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
		want string
	}{
		{"gitlab_runner_controller_scope_remove_instance", map[string]any{"controller_id": 1}, "Successfully deleted instance-level scope."},
		{"gitlab_runner_controller_scope_remove_runner", map[string]any{"controller_id": 1, "runner_id": 42}, "Successfully deleted runner scope."},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			spec := byTool[tt.tool]
			if !spec.Destructive {
				t.Fatalf("%s Destructive = false, want true", tt.tool)
			}
			result, err := spec.Route.Handler(t.Context(), tt.args)
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

// TestActionSpecs_RemoveErrors verifies remove routes propagate backend errors.
func TestActionSpecs_RemoveErrors(t *testing.T) {
	byTool := runnerControllerScopeSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_runner_controller_scope_remove_instance", map[string]any{"controller_id": 1}},
		{"gitlab_runner_controller_scope_remove_runner", map[string]any{"controller_id": 1, "runner_id": 42}},
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

// TestCatalogSurface_ConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_ConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
	byTool := runnerControllerScopeSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	for _, toolName := range []string{"gitlab_runner_controller_scope_remove_instance", "gitlab_runner_controller_scope_remove_runner"} {
		toolutil.RegisterSurfaceToolFromSpec(server, byTool[toolName], toolutil.SurfaceToolRegisterOptions{
			Description: "Test runner controller scope destructive confirmation.",
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
		{"gitlab_runner_controller_scope_remove_instance", map[string]any{"controller_id": float64(1)}},
		{"gitlab_runner_controller_scope_remove_runner", map[string]any{"controller_id": float64(1), "runner_id": float64(42)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, callErr := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if callErr != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.name, callErr)
			}
			if result == nil {
				t.Fatal("expected non-nil result for declined confirmation")
			}
		})
	}
}

func runnerControllerScopesActionHandler() http.Handler {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/runner_controllers/1/scopes", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, sampleScopesJSON)
	})
	handler.HandleFunc("POST /api/v4/runner_controllers/1/scopes/instance", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, sampleInstanceScopeJSON)
	})
	handler.HandleFunc("DELETE /api/v4/runner_controllers/1/scopes/instance", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("POST /api/v4/runner_controllers/1/scopes/runners/42", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, sampleRunnerScopeJSON)
	})
	handler.HandleFunc("DELETE /api/v4/runner_controllers/1/scopes/runners/42", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	return handler
}

func runnerControllerScopeSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
