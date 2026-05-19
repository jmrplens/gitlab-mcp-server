// action_specs_test.go contains route and catalog-surface tests for behavior that
// used to live in register.go: mutation error paths and destructive confirmation.
package boards

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_DeleteError verifies that the board and board-list delete
// routes return errors when the GitLab API fails.
func TestActionSpecs_DeleteError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	client := testutil.NewTestClient(t, mux)
	byTool := boardSpecsByTool(t, ActionSpecs(client))

	tests := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_board_delete", map[string]any{"project_id": "my-project", "board_id": float64(1)}},
		{"gitlab_board_list_delete", map[string]any{"project_id": "my-project", "board_id": float64(1), "list_id": float64(100)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := byTool[tt.name].Route.Handler(t.Context(), tt.args)
			if err == nil {
				t.Fatalf("expected error from %s with failing backend", tt.name)
			}
		})
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers generic destructive
// confirmation for board and board-list delete when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	for _, spec := range ActionSpecs(client) {
		switch spec.IndividualTool.Name {
		case "gitlab_board_delete", "gitlab_board_list_delete":
			toolutil.RegisterSurfaceToolFromSpec(server, spec, toolutil.SurfaceToolRegisterOptions{Description: "Test board destructive confirmation.", Icons: toolutil.IconBoard})
		}
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
	session, connectErr := mcpClient.Connect(ctx, ct, nil)
	if connectErr != nil {
		t.Fatalf("client connect: %v", connectErr)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_board_delete", map[string]any{"project_id": "my-project", "board_id": float64(1)}},
		{"gitlab_board_list_delete", map[string]any{"project_id": "my-project", "board_id": float64(1), "list_id": float64(100)}},
	}
	for _, tt := range tools {
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
