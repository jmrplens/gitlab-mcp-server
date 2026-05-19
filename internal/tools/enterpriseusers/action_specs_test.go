// action_specs_test.go contains canonical-route tests for enterprise user actions.
package enterpriseusers

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const registerUserJSON = `{
	"id": 1,
	"username": "jdoe",
	"name": "John Doe",
	"email": "jdoe@example.com",
	"state": "active",
	"created_at": "2026-01-01T00:00:00Z"
}`

// TestActionSpecs_CallAllRoutes verifies enterprise user tools through canonical routes.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/42/enterprise_users", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+registerUserJSON+`]`)
	})
	mux.HandleFunc("GET /api/v4/groups/42/enterprise_users/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, registerUserJSON)
	})
	mux.HandleFunc("PATCH /api/v4/groups/42/enterprise_users/1/disable_two_factor", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /api/v4/groups/42/enterprise_users/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	byTool := enterpriseUserSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, mux)))

	tools := []struct {
		name string
		args map[string]any
		want string
	}{
		{"gitlab_list_enterprise_users", map[string]any{"group_id": "42"}, ""},
		{"gitlab_get_enterprise_user", map[string]any{"group_id": "42", "user_id": 1}, ""},
		{"gitlab_disable_2fa_enterprise_user", map[string]any{"group_id": "42", "user_id": 1}, "Disabled 2FA for enterprise user 1 in group 42."},
		{"gitlab_delete_enterprise_user", map[string]any{"group_id": "42", "user_id": 1}, "Successfully deleted enterprise user 1 from group 42."},
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
			if tt.want != "" {
				message := ""
				switch out := result.(type) {
				case toolutil.VoidOutput:
					message = out.Message
				case toolutil.DeleteOutput:
					message = out.Message
				default:
					t.Fatalf("Route.Handler(%s) returned %T, want success output", tt.name, result)
				}
				if message != tt.want {
					t.Fatalf("message = %q, want %q", message, tt.want)
				}
			}
		})
	}
}

// TestActionSpecs_MutationErrors verifies mutation routes return errors when GitLab rejects them.
func TestActionSpecs_MutationErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch || r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	})
	byTool := enterpriseUserSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, mux)))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_disable_2fa_enterprise_user", map[string]any{"group_id": "42", "user_id": 1}},
		{"gitlab_delete_enterprise_user", map[string]any{"group_id": "42", "user_id": 1}},
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

// TestCatalogSurface_ConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_ConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	byTool := enterpriseUserSpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_disable_2fa_enterprise_user", map[string]any{"group_id": "42", "user_id": 1}},
		{"gitlab_delete_enterprise_user", map[string]any{"group_id": "42", "user_id": 1}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
			toolutil.RegisterSurfaceToolFromSpec(server, byTool[tt.name], toolutil.SurfaceToolRegisterOptions{
				Description: "Test enterprise user confirmation.",
				Icons:       toolutil.IconUser,
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
				t.Fatalf("expected non-nil result for declined confirmation on %s", tt.name)
			}
		})
	}
}

func enterpriseUserSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
