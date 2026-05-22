// action_specs_test.go contains canonical-route tests for group credential actions.
package groupcredentials

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
	actionSpecPATJSON    = `[{"id":99,"name":"test-pat","scopes":["api"],"state":"active","created_at":"2026-01-01T00:00:00Z","expires_at":"2026-01-01"}]`
	actionSpecSSHKeyJSON = `[{"id":5,"title":"test-key","key":"ssh-rsa AAAA...","created_at":"2026-01-01T00:00:00Z"}]`
)

// TestActionSpecs_CallAllRoutes exercises every group credential tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.Contains(path, "/personal_access_tokens"):
			testutil.RespondJSON(w, http.StatusOK, actionSpecPATJSON)
		case r.Method == http.MethodGet && strings.Contains(path, "/ssh_keys"):
			testutil.RespondJSON(w, http.StatusOK, actionSpecSSHKeyJSON)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	byTool := groupCredentialSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, handler)))

	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list_pats", "gitlab_list_group_personal_access_tokens", map[string]any{"group_id": "mygroup"}},
		{"list_ssh_keys", "gitlab_list_group_ssh_keys", map[string]any{"group_id": "mygroup"}},
		{"revoke_pat", "gitlab_revoke_group_personal_access_token", map[string]any{"group_id": "mygroup", "token_id": 99}},
		{"delete_ssh_key", "gitlab_delete_group_ssh_key", map[string]any{"group_id": "mygroup", "key_id": 5}},
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

// TestActionSpecs_ReadErrorPaths verifies read routes propagate backend errors.
func TestActionSpecs_ReadErrorPaths(t *testing.T) {
	byTool := groupCredentialSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))))

	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list_pats", "gitlab_list_group_personal_access_tokens", map[string]any{"group_id": "42"}},
		{"list_ssh_keys", "gitlab_list_group_ssh_keys", map[string]any{"group_id": "42"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err == nil {
				t.Fatalf("Route.Handler(%s) expected error", tt.tool)
			}
		})
	}
}

// TestActionSpecs_DeleteOutputs verifies destructive routes preserve their success messages.
func TestActionSpecs_DeleteOutputs(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("DELETE /api/v4/groups/mygroup/manage/personal_access_tokens/99", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("DELETE /api/v4/groups/mygroup/manage/ssh_keys/5", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	byTool := groupCredentialSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, handler)))

	tests := []struct {
		tool    string
		args    map[string]any
		message string
	}{
		{"gitlab_revoke_group_personal_access_token", map[string]any{"group_id": "mygroup", "token_id": 99}, "Successfully deleted personal access token 99 from group mygroup."},
		{"gitlab_delete_group_ssh_key", map[string]any{"group_id": "mygroup", "key_id": 5}, "Successfully deleted SSH key 5 from group mygroup."},
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
			if out.Message != tt.message {
				t.Fatalf("delete message = %q", out.Message)
			}
		})
	}
}

// TestActionSpecs_DeleteOutputErrors verifies destructive wrappers propagate validation errors.
func TestActionSpecs_DeleteOutputErrors(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called when required identifiers are missing")
	}))
	tests := []struct {
		name string
		fn   func() (toolutil.DeleteOutput, error)
	}{
		{name: "revoke_pat", fn: func() (toolutil.DeleteOutput, error) {
			return revokePATOutput(context.Background(), client, RevokePATInput{GroupID: toolutil.StringOrInt("mygroup")})
		}},
		{name: "delete_ssh_key", fn: func() (toolutil.DeleteOutput, error) {
			return deleteSSHKeyOutput(context.Background(), client, DeleteSSHKeyInput{GroupID: toolutil.StringOrInt("mygroup")})
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := tt.fn()
			if err == nil {
				t.Fatal("expected validation error")
			}
			if out.Status != "" || out.Message != "" {
				t.Fatalf("output = %+v, want zero output", out)
			}
		})
	}
}

// TestCatalogSurface_ConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_ConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
	byTool := groupCredentialSpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
		icon []mcp.Icon
	}{
		{"gitlab_revoke_group_personal_access_token", map[string]any{"group_id": "g", "token_id": 1}, toolutil.IconToken},
		{"gitlab_delete_group_ssh_key", map[string]any{"group_id": "g", "key_id": 1}, toolutil.IconKey},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
			toolutil.RegisterSurfaceToolFromSpec(server, byTool[tt.name], toolutil.SurfaceToolRegisterOptions{
				Description: "Test group credential destructive confirmation.",
				Icons:       tt.icon,
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

			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool returned transport error: %v", err)
			}
			if result == nil {
				t.Fatal("expected non-nil result when confirmation is declined")
			}
		})
	}
}

func groupCredentialSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
