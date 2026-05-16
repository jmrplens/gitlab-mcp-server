// register_test.go contains catalog-surface tests for access token actions.
package accesstokens

import (
	"context"
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestActionSpecs_RevokeErrors verifies revoke routes return errors when the GitLab API rejects them.
func TestActionSpecs_RevokeErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	client := testutil.NewTestClient(t, mux)
	byTool := accessTokenSpecsByTool(t, ActionSpecs(client))

	tests := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_project_access_token_revoke", map[string]any{"project_id": "my-project", "token_id": float64(1)}},
		{"gitlab_group_access_token_revoke", map[string]any{"group_id": "my-group", "token_id": float64(1)}},
		{"gitlab_personal_access_token_revoke", map[string]any{"token_id": float64(1)}},
		{"gitlab_personal_access_token_revoke_self", map[string]any{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := byTool[tt.name].Route.Handler(t.Context(), tt.args)
			if err == nil {
				t.Fatalf("Route.Handler(%s) expected error, got nil", tt.name)
			}
		})
	}
}

// TestCatalogSurface_RevokeConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_RevokeConfirmDeclined(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)
	byTool := accessTokenSpecsByTool(t, ActionSpecs(client))

	tests := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_project_access_token_revoke", map[string]any{"project_id": "p", "token_id": float64(1)}},
		{"gitlab_group_access_token_revoke", map[string]any{"group_id": "g", "token_id": float64(1)}},
		{"gitlab_personal_access_token_revoke", map[string]any{"token_id": float64(1)}},
		{"gitlab_personal_access_token_revoke_self", map[string]any{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
			toolutil.RegisterSurfaceToolFromSpec(server, byTool[tt.name], toolutil.SurfaceToolRegisterOptions{
				Description: "Test access token destructive confirmation.",
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
