// action_specs_test.go contains canonical-route tests for custom member role actions.
package memberroles

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionSpecRoleJSON     = `{"id":1,"name":"custom_role","base_access_level":30,"description":"A custom role"}`
	actionSpecRoleListJSON = `[{"id":1,"name":"custom_role","base_access_level":30}]`
)

// TestActionSpecs_CallAllRoutes exercises every member role tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := memberRoleSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, memberRolesActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_list_instance_member_roles", map[string]any{}},
		{"gitlab_create_instance_member_role", map[string]any{"name": "r", "base_access_level": 30}},
		{"gitlab_delete_instance_member_role", map[string]any{"member_role_id": 1}},
		{"gitlab_list_group_member_roles", map[string]any{"group_id": "mygroup"}},
		{"gitlab_create_group_member_role", map[string]any{"group_id": "mygroup", "name": "r", "base_access_level": 30}},
		{"gitlab_delete_group_member_role", map[string]any{"group_id": "mygroup", "member_role_id": 1}},
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

// TestActionSpecs_MutationErrors verifies delete routes propagate backend errors.
func TestActionSpecs_MutationErrors(t *testing.T) {
	byTool := memberRoleSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		{"gitlab_delete_instance_member_role", map[string]any{"member_role_id": 1}},
		{"gitlab_delete_group_member_role", map[string]any{"group_id": "mygroup", "member_role_id": 1}},
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

// TestActionSpecs_DeleteOutput verifies delete routes preserve their success messages.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	byTool := memberRoleSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, memberRolesActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
		want string
	}{
		{"gitlab_delete_instance_member_role", map[string]any{"member_role_id": 1}, "Successfully deleted instance member role 1."},
		{"gitlab_delete_group_member_role", map[string]any{"group_id": "mygroup", "member_role_id": 1}, "Successfully deleted member role 1 from group mygroup."},
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

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
	byTool := memberRoleSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	for _, toolName := range []string{"gitlab_delete_instance_member_role", "gitlab_delete_group_member_role"} {
		toolutil.RegisterSurfaceToolFromSpec(server, byTool[toolName], toolutil.SurfaceToolRegisterOptions{
			Description: "Test member role destructive confirmation.",
			Icons:       toolutil.IconSecurity,
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
		tool string
		args map[string]any
	}{
		{"gitlab_delete_instance_member_role", map[string]any{"member_role_id": float64(1)}},
		{"gitlab_delete_group_member_role", map[string]any{"group_id": "g", "member_role_id": float64(1)}},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			result, callErr := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.tool, Arguments: tt.args})
			if callErr != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.tool, callErr)
			}
			if result == nil {
				t.Fatalf("expected non-nil result for %s declined confirmation", tt.tool)
			}
			foundText := false
			for _, content := range result.Content {
				if text, ok := content.(*mcp.TextContent); ok && text.Text != "" {
					foundText = true
				}
			}
			if !foundText {
				t.Errorf("expected non-empty text content in %s cancellation result", tt.tool)
			}
		})
	}
}

func memberRolesActionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && (path == "/api/v4/member_roles" || strings.Contains(path, "/groups/") && strings.HasSuffix(path, "/member_roles")):
			testutil.RespondJSON(w, http.StatusOK, actionSpecRoleListJSON)
		case r.Method == http.MethodPost:
			testutil.RespondJSON(w, http.StatusCreated, actionSpecRoleJSON)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
}

func memberRoleSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
