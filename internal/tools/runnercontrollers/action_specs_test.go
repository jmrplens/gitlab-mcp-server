// action_specs_test.go contains canonical-route tests for runner controller actions.
package runnercontrollers

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_CallAllRoutes exercises every runner controller tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := runnerControllerSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, runnerControllerActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_runner_controller_list", map[string]any{}},
		{"gitlab_runner_controller_get", map[string]any{"controller_id": 1}},
		{"gitlab_runner_controller_create", map[string]any{"description": "new"}},
		{"gitlab_runner_controller_update", map[string]any{"controller_id": 1, "description": "updated"}},
		{"gitlab_runner_controller_delete", map[string]any{"controller_id": 1}},
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

// TestActionSpecs_DeleteOutput verifies the delete route preserves its success message.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	byTool := runnerControllerSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, runnerControllerActionHandler())))

	result, err := byTool["gitlab_runner_controller_delete"].Route.Handler(t.Context(), map[string]any{"controller_id": 1})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_runner_controller_delete) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_runner_controller_delete) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted runner controller." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestActionSpecs_DeleteAPIError verifies the delete route propagates backend failures.
func TestActionSpecs_DeleteAPIError(t *testing.T) {
	byTool := runnerControllerSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))))

	_, err := byTool["gitlab_runner_controller_delete"].Route.Handler(t.Context(), map[string]any{"controller_id": 1})
	if err == nil {
		t.Fatal("expected route error")
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
	byTool := runnerControllerSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_runner_controller_delete"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test runner controller destructive confirmation.",
		Icons:       toolutil.IconRunner,
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
		Name:      "gitlab_runner_controller_delete",
		Arguments: map[string]any{"controller_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

func runnerControllerActionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/runner_controllers":
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+sampleControllerJSON+`]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/runner_controllers/1":
			testutil.RespondJSON(w, http.StatusOK, sampleDetailsJSON)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/runner_controllers":
			testutil.RespondJSON(w, http.StatusCreated, sampleControllerJSON)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/runner_controllers/1":
			testutil.RespondJSON(w, http.StatusOK, sampleControllerJSON)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/runner_controllers/1":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
}

func runnerControllerSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
