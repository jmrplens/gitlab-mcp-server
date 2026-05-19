// action_specs_test.go contains canonical-route tests for runner controller token actions.
package runnercontrollertokens

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_CallAllRoutes exercises every runner controller token tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := runnerControllerTokenSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, runnerControllerTokenActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_runner_controller_token_list", map[string]any{"controller_id": 1}},
		{"gitlab_runner_controller_token_get", map[string]any{"controller_id": 1, "token_id": 10}},
		{"gitlab_runner_controller_token_create", map[string]any{"controller_id": 1, "description": "new"}},
		{"gitlab_runner_controller_token_rotate", map[string]any{"controller_id": 1, "token_id": 10}},
		{"gitlab_runner_controller_token_revoke", map[string]any{"controller_id": 1, "token_id": 10}},
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

// TestActionSpecs_RevokeOutput verifies the revoke route preserves its success message.
func TestActionSpecs_RevokeOutput(t *testing.T) {
	byTool := runnerControllerTokenSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, runnerControllerTokenActionHandler())))

	result, err := byTool["gitlab_runner_controller_token_revoke"].Route.Handler(t.Context(), map[string]any{
		"controller_id": 1,
		"token_id":      10,
	})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_runner_controller_token_revoke) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_runner_controller_token_revoke) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted runner controller token." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestActionSpecs_RevokeAPIError verifies the revoke route propagates backend failures.
func TestActionSpecs_RevokeAPIError(t *testing.T) {
	byTool := runnerControllerTokenSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))))

	_, err := byTool["gitlab_runner_controller_token_revoke"].Route.Handler(t.Context(), map[string]any{
		"controller_id": 1,
		"token_id":      10,
	})
	if err == nil {
		t.Fatal("expected route error")
	}
}

// TestCatalogSurface_RevokeConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_RevokeConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
	byTool := runnerControllerTokenSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_runner_controller_token_revoke"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test runner controller token destructive confirmation.",
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
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_runner_controller_token_revoke",
		Arguments: map[string]any{"controller_id": 1, "token_id": 10},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

func runnerControllerTokenActionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/runner_controllers/1/tokens":
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+sampleTokenJSON+`]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/runner_controllers/1/tokens/10":
			testutil.RespondJSON(w, http.StatusOK, sampleTokenJSON)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/runner_controllers/1/tokens":
			testutil.RespondJSON(w, http.StatusCreated, sampleTokenJSON)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/runner_controllers/1/tokens/10/rotate":
			testutil.RespondJSON(w, http.StatusOK, sampleTokenJSON)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/runner_controllers/1/tokens/10":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
}

func runnerControllerTokenSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
