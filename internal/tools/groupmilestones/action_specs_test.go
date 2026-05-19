// action_specs_test.go contains route and catalog-surface tests for behavior that
// used to live in register.go: mutation error paths and destructive confirmation.
package groupmilestones

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_DeleteError verifies the delete route returns an error when
// the GitLab API fails.
func TestActionSpecs_DeleteError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	})
	client := testutil.NewTestClient(t, mux)
	byTool := groupMilestoneSpecsByTool(t, ActionSpecs(client))

	_, err := byTool["gitlab_group_milestone_delete"].Route.Handler(t.Context(), map[string]any{"group_id": "42", "milestone_iid": 1})
	if err == nil {
		t.Fatal("expected error from gitlab_group_milestone_delete")
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers generic destructive
// confirmation for group milestone delete when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	for _, spec := range ActionSpecs(client) {
		if spec.IndividualTool.Name == "gitlab_group_milestone_delete" {
			toolutil.RegisterSurfaceToolFromSpec(server, spec, toolutil.SurfaceToolRegisterOptions{Description: "Test group milestone destructive confirmation.", Icons: toolutil.IconMilestone})
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
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_group_milestone_delete",
		Arguments: map[string]any{"group_id": "42", "milestone_iid": float64(1)},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			if tc.Text == "" {
				t.Error("expected non-empty cancellation message")
			}
			return
		}
	}
	t.Error("expected text content in cancellation result")
}
