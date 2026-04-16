// register_management_test.go tests MCP roundtrip for all management tools
// (admin, CRUD, SSH keys, misc, service accounts) to cover the register_ closures.
package users

import (
	"context"
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestRegisterManagementTools_MCPRoundtrip validates that all management tools
// (admin state, CRUD, SSH keys, misc, service accounts) are reachable and
// return non-error results via the MCP protocol.
func TestRegisterManagementTools_MCPRoundtrip(t *testing.T) {
	session := newManagementMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		// Admin state actions
		{"block_user", "gitlab_block_user", map[string]any{"user_id": 42}},
		{"unblock_user", "gitlab_unblock_user", map[string]any{"user_id": 42}},
		{"ban_user", "gitlab_ban_user", map[string]any{"user_id": 42}},
		{"unban_user", "gitlab_unban_user", map[string]any{"user_id": 42}},
		{"activate_user", "gitlab_activate_user", map[string]any{"user_id": 42}},
		{"deactivate_user", "gitlab_deactivate_user", map[string]any{"user_id": 42}},
		{"approve_user", "gitlab_approve_user", map[string]any{"user_id": 42}},
		{"reject_user", "gitlab_reject_user", map[string]any{"user_id": 42}},
		{"disable_2fa", "gitlab_disable_two_factor", map[string]any{"user_id": 42}},
		// CRUD
		{"create_user", "gitlab_create_user", map[string]any{"email": "new@test.com", "name": "New", "username": "newu"}},
		{"modify_user", "gitlab_modify_user", map[string]any{"user_id": 42, "bio": "Updated"}},
		{"delete_user", "gitlab_delete_user", map[string]any{"user_id": 42}},
		// SSH keys
		{"list_ssh_keys_for_user", "gitlab_list_ssh_keys_for_user", map[string]any{"user_id": 42}},
		{"get_ssh_key", "gitlab_get_ssh_key", map[string]any{"key_id": 1}},
		{"get_ssh_key_for_user", "gitlab_get_ssh_key_for_user", map[string]any{"user_id": 42, "key_id": 1}},
		{"add_ssh_key", "gitlab_add_ssh_key", map[string]any{"title": "k", "key": "ssh-rsa AAA"}},
		{"add_ssh_key_for_user", "gitlab_add_ssh_key_for_user", map[string]any{"user_id": 42, "title": "k", "key": "ssh-rsa AAA"}},
		{"delete_ssh_key", "gitlab_delete_ssh_key", map[string]any{"key_id": 1}},
		{"delete_ssh_key_for_user", "gitlab_delete_ssh_key_for_user", map[string]any{"user_id": 42, "key_id": 1}},
		// Misc
		{"current_user_status", "gitlab_current_user_status", map[string]any{}},
		{"get_activities", "gitlab_get_user_activities", map[string]any{}},
		{"get_memberships", "gitlab_get_user_memberships", map[string]any{"user_id": 42}},
		{"create_runner", "gitlab_create_user_runner", map[string]any{"runner_type": "instance_type"}},
		{"delete_identity", "gitlab_delete_user_identity", map[string]any{"user_id": 42, "provider": "ldap"}},
		// Service accounts
		{"create_svc", "gitlab_create_service_account", map[string]any{"name": "svc", "username": "svc"}},
		{"list_svc", "gitlab_list_service_accounts", map[string]any{}},
		{"create_pat", "gitlab_create_current_user_pat", map[string]any{"name": "pat", "scopes": []string{"api"}}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.tool,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.tool, err)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(%s) returned error: %s", tt.tool, tc.Text)
					}
				}
				t.Fatalf("CallTool(%s) returned IsError=true", tt.tool)
			}
		})
	}
}

// newManagementMCPSession creates an MCP client session with all user tools
// registered including management tools.
func newManagementMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	uJSON := `{"id":42,"username":"testuser","email":"test@example.com","name":"Test User","state":"active","web_url":"https://gitlab.example.com/testuser"}`
	statusJSON := `{"emoji":"coffee","message":"Working","availability":"busy"}`
	sshKeyJSON := `{"id":1,"title":"key","key":"ssh-rsa AAA","created_at":"2026-01-01T00:00:00Z"}`

	handler := http.NewServeMux()

	// Core user routes (needed for tools registered in registerCoreTools)
	handler.HandleFunc("GET /api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, uJSON)
	})
	handler.HandleFunc("GET /api/v4/users", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+uJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/users/42", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, uJSON)
	})
	handler.HandleFunc("GET /api/v4/users/42/status", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, statusJSON)
	})
	handler.HandleFunc("PUT /api/v4/user/status", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, statusJSON)
	})
	handler.HandleFunc("GET /api/v4/user/keys", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+sshKeyJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/user/emails", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"email":"test@example.com"}]`)
	})
	handler.HandleFunc("GET /api/v4/users/42/events", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})
	handler.HandleFunc("GET /api/v4/users/42/associations_count", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"groups_count":0,"projects_count":0,"issues_count":0,"merge_requests_count":0}`)
	})

	// Admin state actions (POST)
	for _, action := range []string{"block", "unblock", "ban", "unban", "activate", "deactivate", "approve"} {
		handler.HandleFunc("POST /api/v4/users/42/"+action, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
		})
	}
	handler.HandleFunc("POST /api/v4/users/42/reject", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler.HandleFunc("PATCH /api/v4/users/42/disable_two_factor", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// CRUD
	handler.HandleFunc("POST /api/v4/users", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, uJSON)
	})
	handler.HandleFunc("PUT /api/v4/users/42", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, uJSON)
	})
	handler.HandleFunc("DELETE /api/v4/users/42", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// SSH keys for user
	handler.HandleFunc("GET /api/v4/users/42/keys", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+sshKeyJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/user/keys/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, sshKeyJSON)
	})
	handler.HandleFunc("GET /api/v4/users/42/keys/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, sshKeyJSON)
	})
	handler.HandleFunc("POST /api/v4/user/keys", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, sshKeyJSON)
	})
	handler.HandleFunc("POST /api/v4/users/42/keys", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, sshKeyJSON)
	})
	handler.HandleFunc("DELETE /api/v4/user/keys/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("DELETE /api/v4/users/42/keys/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Misc
	handler.HandleFunc("GET /api/v4/user/status", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, statusJSON)
	})
	handler.HandleFunc("GET /api/v4/user/activities", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})
	handler.HandleFunc("GET /api/v4/users/42/memberships", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"source_id":1,"source_name":"proj","source_type":"Project","access_level":30}]`)
	})
	handler.HandleFunc("POST /api/v4/user/runners", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":101,"token":"glrt-abc"}`)
	})
	handler.HandleFunc("DELETE /api/v4/users/42/identities/ldap", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Service accounts
	handler.HandleFunc("POST /api/v4/service_accounts", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, uJSON)
	})
	handler.HandleFunc("GET /api/v4/service_accounts", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"username":"svc-1","name":"Service 1"}]`)
	})
	handler.HandleFunc("POST /api/v4/user/personal_access_tokens", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":10,"name":"pat","active":true,"token":"glpat-t","scopes":["api"],"revoked":false,"user_id":1}`)
	})

	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test-mgmt", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
	RegisterEnterpriseTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}
