// action_specs_test.go contains canonical-route tests for user actions.
package users

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_CallRoutes exercises user actions through their canonical routes.
func TestActionSpecs_CallRoutes(t *testing.T) {
	byTool := userSpecsByTool(t, ActionSpecs(newUserActionSpecClient(t), true))

	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"current_user", "gitlab_user_current", map[string]any{}},
		{"list_users", "gitlab_list_users", map[string]any{}},
		{"get_user", "gitlab_get_user", map[string]any{"user_id": 42}},
		{"get_user_status", "gitlab_get_user_status", map[string]any{"user_id": 42}},
		{"set_user_status", "gitlab_set_user_status", map[string]any{"emoji": "coffee", "message": "Working"}},
		{"list_ssh_keys", "gitlab_list_ssh_keys", map[string]any{}},
		{"list_emails", "gitlab_list_emails", map[string]any{}},
		{"list_contribution_events", "gitlab_list_user_contribution_events", map[string]any{"user_id": 42}},
		{"get_associations_count", "gitlab_get_user_associations_count", map[string]any{"user_id": 42}},
		{"block_user", "gitlab_block_user", map[string]any{"user_id": 42}},
		{"unblock_user", "gitlab_unblock_user", map[string]any{"user_id": 42}},
		{"ban_user", "gitlab_ban_user", map[string]any{"user_id": 42}},
		{"unban_user", "gitlab_unban_user", map[string]any{"user_id": 42}},
		{"activate_user", "gitlab_activate_user", map[string]any{"user_id": 42}},
		{"deactivate_user", "gitlab_deactivate_user", map[string]any{"user_id": 42}},
		{"approve_user", "gitlab_approve_user", map[string]any{"user_id": 42}},
		{"reject_user", "gitlab_reject_user", map[string]any{"user_id": 42}},
		{"disable_2fa", "gitlab_disable_two_factor", map[string]any{"user_id": 42}},
		{"create_user", "gitlab_create_user", map[string]any{"email": "new@test.com", "name": "New", "username": "newu"}},
		{"modify_user", "gitlab_modify_user", map[string]any{"user_id": 42, "bio": "Updated"}},
		{"delete_user", "gitlab_delete_user", map[string]any{"user_id": 42}},
		{"list_ssh_keys_for_user", "gitlab_list_ssh_keys_for_user", map[string]any{"user_id": 42}},
		{"get_ssh_key", "gitlab_get_ssh_key", map[string]any{"key_id": 1}},
		{"get_ssh_key_for_user", "gitlab_get_ssh_key_for_user", map[string]any{"user_id": 42, "key_id": 1}},
		{"add_ssh_key", "gitlab_add_ssh_key", map[string]any{"title": "k", "key": "ssh-rsa AAA"}},
		{"add_ssh_key_for_user", "gitlab_add_ssh_key_for_user", map[string]any{"user_id": 42, "title": "k", "key": "ssh-rsa AAA"}},
		{"delete_ssh_key", "gitlab_delete_ssh_key", map[string]any{"key_id": 1}},
		{"delete_ssh_key_for_user", "gitlab_delete_ssh_key_for_user", map[string]any{"user_id": 42, "key_id": 1}},
		{"current_user_status", "gitlab_current_user_status", map[string]any{}},
		{"get_activities", "gitlab_get_user_activities", map[string]any{}},
		{"get_memberships", "gitlab_get_user_memberships", map[string]any{"user_id": 42}},
		{"create_runner", "gitlab_create_user_runner", map[string]any{"runner_type": "instance_type"}},
		{"delete_identity", "gitlab_delete_user_identity", map[string]any{"user_id": 42, "provider": "ldap"}},
		{"create_svc", "gitlab_create_service_account", map[string]any{"name": "svc", "username": "svc"}},
		{"list_svc", "gitlab_list_service_accounts", map[string]any{}},
		{"create_pat", "gitlab_create_current_user_pat", map[string]any{"name": "pat", "scopes": []string{"api"}}},
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

// TestActionSpecs_PrimaryMetadata verifies richer metadata for core user actions.
func TestActionSpecs_PrimaryMetadata(t *testing.T) {
	byTool := userSpecsByTool(t, ActionSpecs(newUserActionSpecClient(t), true))

	currentSpec := byTool["gitlab_user_current"]
	if !slices.Contains(currentSpec.Aliases, "who am i") {
		t.Fatalf("current Aliases = %v, want who am i", currentSpec.Aliases)
	}
	if !strings.Contains(currentSpec.Usage, "authenticated user profile") {
		t.Fatalf("current Usage = %q", currentSpec.Usage)
	}

	listSpec := byTool["gitlab_list_users"]
	if guidance := listSpec.ParameterGuidance["search"]; guidance.ExampleBinding == "" {
		t.Fatalf("list search guidance = %+v, want example binding", guidance)
	}
	if !slices.Contains(listSpec.RelatedActions, "user.create") {
		t.Fatalf("list RelatedActions = %v, want user.create", listSpec.RelatedActions)
	}

	getSpec := byTool["gitlab_get_user"]
	if guidance := getSpec.ParameterGuidance["user_id"]; guidance.SemanticRole != "scope_user" {
		t.Fatalf("get user_id guidance = %+v, want scope_user", guidance)
	}
	if !strings.Contains(getSpec.IndividualTool.Description, "Returns:") || !strings.Contains(getSpec.IndividualTool.Description, "See also:") {
		t.Fatalf("get description = %q, want Returns/See also", getSpec.IndividualTool.Description)
	}

	createSpec := byTool["gitlab_create_user"]
	if !slices.Contains(createSpec.Aliases, "create user") {
		t.Fatalf("create Aliases = %v, want create user", createSpec.Aliases)
	}
	if guidance := createSpec.ParameterGuidance["email"]; guidance.SemanticRole != "email_address" {
		t.Fatalf("create email guidance = %+v, want email_address", guidance)
	}
}

// TestActionSpecs_GetUserNotFound verifies get_user preserves NotFoundResult details.
func TestActionSpecs_GetUserNotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 User Not Found"}`)
	}))
	byTool := userSpecsByTool(t, ActionSpecs(client, false))

	result, err := byTool["gitlab_get_user"].Route.Handler(t.Context(), map[string]any{"user_id": 999})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_get_user) error: %v", err)
	}
	out, ok := result.(userNotFoundOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_get_user) returned %T, want userNotFoundOutput", result)
	}
	if out.Identifier != "ID 999" {
		t.Fatalf("identifier = %q", out.Identifier)
	}
}

// TestFormatUserNotFound verifies not-found result formatting for user lookups.
func TestFormatUserNotFound(t *testing.T) {
	result := formatUserNotFound(userNotFoundOutput{Identifier: "ID 999"})
	if result == nil || !result.IsError {
		t.Fatalf("formatUserNotFound() = %+v, want error result", result)
	}
	content, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content type = %T, want *mcp.TextContent", result.Content[0])
	}
	if !strings.Contains(content.Text, "User") || !strings.Contains(content.Text, "ID 999") {
		t.Fatalf("content = %q, want user identifier", content.Text)
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
	byTool := userSpecsByTool(t, ActionSpecs(client, false))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_delete_user"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test user destructive confirmation.",
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
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})

	result, callErr := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_delete_user",
		Arguments: map[string]any{"user_id": 42},
	})
	if callErr != nil {
		t.Fatalf("CallTool error: %v", callErr)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

// TestMarkdownForResult_ManagementOutputs verifies management outputs have markdown formatters.
func TestMarkdownForResult_ManagementOutputs(t *testing.T) {
	tests := []struct {
		name   string
		result any
	}{
		{"admin", AdminActionOutput{UserID: 42, Action: "blocked", Success: true}},
		{"delete_user", DeleteOutput{UserID: 42, Deleted: true}},
		{"delete_ssh_key", DeleteSSHKeyOutput{KeyID: 1, Deleted: true}},
		{"activities", UserActivitiesOutput{}},
		{"memberships", UserMembershipsOutput{}},
		{"runner", UserRunnerOutput{ID: 101, Token: "glrt-abc"}},
		{"delete_identity", DeleteUserIdentityOutput{UserID: 42, Provider: "ldap", Deleted: true}},
		{"service_accounts", ServiceAccountListOutput{}},
		{"current_pat", CurrentUserPATOutput{ID: 10, Name: "pat", Active: true, Scopes: []string{"api"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callResult := toolutil.MarkdownForResult(tt.result)
			if callResult == nil {
				t.Fatal("MarkdownForResult returned nil")
			}
			if len(callResult.Content) == 0 {
				t.Fatal("MarkdownForResult returned no content")
			}
		})
	}
}

func newUserActionSpecClient(t *testing.T) *gitlabclient.Client {
	t.Helper()

	userJSON := `{"id":42,"username":"testuser","email":"test@example.com","name":"Test User","state":"active","web_url":"https://gitlab.example.com/testuser","avatar_url":"https://gitlab.example.com/avatar.png","is_admin":false,"bio":"Developer"}`
	statusJSON := `{"emoji":"coffee","message":"Working","availability":"busy"}`
	sshKeyJSON := `{"id":1,"title":"key","key":"ssh-rsa AAA","created_at":"2026-01-01T00:00:00Z"}`

	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, userJSON)
	})
	handler.HandleFunc("GET /api/v4/users", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+userJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/users/42", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, userJSON)
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
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"email":"test@example.com","confirmed_at":"2026-01-01T00:00:00Z"}]`)
	})
	handler.HandleFunc("GET /api/v4/users/42/events", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":100,"project_id":10,"action_name":"pushed","target_type":"Project","created_at":"2026-06-01T12:00:00Z"}]`)
	})
	handler.HandleFunc("GET /api/v4/users/42/associations_count", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"groups_count":5,"projects_count":12,"issues_count":45,"merge_requests_count":30}`)
	})
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
	handler.HandleFunc("POST /api/v4/users", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, userJSON)
	})
	handler.HandleFunc("PUT /api/v4/users/42", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, userJSON)
	})
	handler.HandleFunc("DELETE /api/v4/users/42", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
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
	handler.HandleFunc("POST /api/v4/service_accounts", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, userJSON)
	})
	handler.HandleFunc("GET /api/v4/service_accounts", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"username":"svc-1","name":"Service 1"}]`)
	})
	handler.HandleFunc("POST /api/v4/user/personal_access_tokens", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":10,"name":"pat","active":true,"token":"glpat-t","scopes":["api"],"revoked":false,"user_id":1}`)
	})

	return testutil.NewTestClient(t, handler)
}

func userSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		toolName := spec.IndividualTool.Name
		if toolName == "" {
			t.Fatalf("spec %s missing IndividualTool.Name", spec.Name)
		}
		if _, exists := byTool[toolName]; exists {
			if toolName == "gitlab_user_current" {
				continue
			}
			t.Fatalf("duplicate individual tool %q", toolName)
		}
		byTool[toolName] = spec
	}
	return byTool
}
