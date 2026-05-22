//go:build e2e && !enterprise

// users_ce_test.go — E2E tests for user tools domain.
package suite

import (
	"context"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/users"
)

// TestIndividual_Users exercises user tools via individual MCP tools:
// get current user, list all users, then get a specific user by ID.
func TestIndividual_Users(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var userID int64

	t.Run("Current", func(t *testing.T) {
		out, err := callToolOn[users.Output](ctx, sess.individual, "gitlab_user_current", users.CurrentInput{})
		requireNoError(t, err, "get current user")
		requireTruef(t, out.ID > 0, "expected user ID > 0, got %d", out.ID)
		requireTruef(t, out.Username != "", "expected non-empty username")
		userID = out.ID
		t.Logf("Current user: %s (ID=%d)", out.Username, userID)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[users.ListOutput](ctx, sess.individual, "gitlab_list_users", users.ListInput{})
		requireNoError(t, err, "list users")
		requireTruef(t, len(out.Users) >= 1, "expected >=1 user, got %d", len(out.Users))
	})

	t.Run("Get", func(t *testing.T) {
		out, err := callToolOn[users.Output](ctx, sess.individual, "gitlab_get_user", users.GetInput{
			UserID: userID,
		})
		requireNoError(t, err, "get user")
		requireTruef(t, out.ID == userID, "expected user ID %d, got %d", userID, out.ID)
	})
}

// TestIndividual_UserManagement exercises catalog-projected individual tools
// that used to be registered by the user management registration layer.
func TestIndividual_UserManagement(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}
	RunWithCapabilities(t, []Capability{CapabilityAdmin, CapabilityInstanceGlobal}, func(_ *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
		defer cancel()

		username := uniqueName("usr-ind")
		skipConfirmation := true
		forceRandomPassword := false
		var userID int64
		deleted := false
		defer func() {
			if userID == 0 || deleted {
				return
			}
			_, _ = callToolOn[users.AdminActionOutput](ctx, sess.individual, "gitlab_unblock_user", users.AdminActionInput{UserID: userID})
			_, _ = callToolOn[users.DeleteOutput](ctx, sess.individual, "gitlab_delete_user", users.DeleteInput{UserID: userID})
		}()

		t.Run("CreateUser", func(t *testing.T) {
			out, err := callToolOn[users.Output](ctx, sess.individual, "gitlab_create_user", users.CreateInput{
				Email:               username + "@e2e-test.local",
				Name:                "E2E Individual " + username,
				Username:            username,
				Password:            "E2eT!Gx9K#p2mNq$8BcZ",
				SkipConfirmation:    &skipConfirmation,
				ForceRandomPassword: &forceRandomPassword,
			})
			requireNoError(t, err, "create user via individual tool")
			requireTruef(t, out.ID > 0, "created user ID > 0")
			requireTruef(t, out.Username == username, "created username = %q, want %q", out.Username, username)
			userID = out.ID
		})

		t.Run("ModifyUser", func(t *testing.T) {
			requireTruef(t, userID > 0, "userID not set")
			out, err := callToolOn[users.Output](ctx, sess.individual, "gitlab_modify_user", users.ModifyInput{
				UserID: userID,
				Bio:    "E2E individual management user",
			})
			requireNoError(t, err, "modify user via individual tool")
			requireTruef(t, out.ID == userID, "modify user ID = %d, want %d", out.ID, userID)
		})

		t.Run("BlockUser", func(t *testing.T) {
			requireTruef(t, userID > 0, "userID not set")
			out, err := callToolOn[users.AdminActionOutput](ctx, sess.individual, "gitlab_block_user", users.AdminActionInput{UserID: userID})
			requireNoError(t, err, "block user via individual tool")
			requireTruef(t, out.Success, "block user should succeed")
			requireTruef(t, out.Action == "blocked", "block action = %q, want blocked", out.Action)
		})

		t.Run("UnblockUser", func(t *testing.T) {
			requireTruef(t, userID > 0, "userID not set")
			out, err := callToolOn[users.AdminActionOutput](ctx, sess.individual, "gitlab_unblock_user", users.AdminActionInput{UserID: userID})
			requireNoError(t, err, "unblock user via individual tool")
			requireTruef(t, out.Success, "unblock user should succeed")
			requireTruef(t, out.Action == "unblocked", "unblock action = %q, want unblocked", out.Action)
		})

		t.Run("DeleteUser", func(t *testing.T) {
			requireTruef(t, userID > 0, "userID not set")
			out, err := callToolOn[users.DeleteOutput](ctx, sess.individual, "gitlab_delete_user", users.DeleteInput{UserID: userID})
			requireNoError(t, err, "delete user via individual tool")
			requireTruef(t, out.UserID == userID, "delete user ID = %d, want %d", out.UserID, userID)
			requireTruef(t, out.Deleted, "delete user should report deleted")
			deleted = true
		})
	})
}

// TestIndividual_UserManagementCatalogProjection verifies user management tools
// are present in tools/list with catalog-derived schemas and annotations.
func TestIndividual_UserManagementCatalogProjection(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := sess.individual.ListTools(ctx, nil)
	requireNoError(t, err, "list individual tools")

	tests := []struct {
		name            string
		required        []string
		wantConfirm     bool
		wantDestructive bool
	}{
		{name: "gitlab_create_user", required: []string{"email", "name", "username"}},
		{name: "gitlab_modify_user", required: []string{"user_id"}},
		{name: "gitlab_block_user", required: []string{"user_id"}, wantConfirm: true},
		{name: "gitlab_unblock_user", required: []string{"user_id"}},
		{name: "gitlab_delete_user", required: []string{"user_id"}, wantConfirm: true, wantDestructive: true},
		{name: "gitlab_create_current_user_pat", required: []string{"name", "scopes"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertUserToolProjection(t, result.Tools, tt.name, tt.required, tt.wantConfirm, tt.wantDestructive)
		})
	}
}

func assertUserToolProjection(t *testing.T, tools []*mcp.Tool, name string, required []string, wantConfirm, wantDestructive bool) {
	t.Helper()
	tool := findE2ETool(t, tools, name)
	schema := schemaMapFromAny(t, tool.InputSchema)
	requireTruef(t, tool.OutputSchema != nil, "%s OutputSchema is nil", name)
	if tool.Annotations == nil || tool.Annotations.DestructiveHint == nil {
		t.Fatalf("%s destructive annotation missing", name)
	}
	if *tool.Annotations.DestructiveHint != wantDestructive {
		t.Fatalf("%s destructiveHint = %t, want %t", name, *tool.Annotations.DestructiveHint, wantDestructive)
	}
	for _, field := range required {
		requireSchemaRequired(t, schema, field)
	}
	requireSchemaConfirmProperty(t, schema, wantConfirm)
}

// TestMeta_Users exercises the same user operations via the gitlab_user meta-tool:
// current, list, and get actions.
func TestMeta_Users(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var userID int64

	t.Run("Current", func(t *testing.T) {
		out, err := callToolOn[users.Output](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "current",
			"params": map[string]any{},
		})
		requireNoError(t, err, "get current user meta")
		requireTruef(t, out.ID > 0, "expected user ID > 0, got %d", out.ID)
		userID = out.ID
		t.Logf("Current user (meta): %s (ID=%d)", out.Username, userID)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[users.ListOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "list",
			"params": map[string]any{},
		})
		requireNoError(t, err, "list users meta")
		requireTruef(t, len(out.Users) >= 1, "expected >=1 user, got %d", len(out.Users))
	})

	t.Run("Get", func(t *testing.T) {
		out, err := callToolOn[users.Output](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "get",
			"params": map[string]any{
				"user_id": userID,
			},
		})
		requireNoError(t, err, "get user meta")
		requireTruef(t, out.ID == userID, "expected user ID %d, got %d", userID, out.ID)
	})
}

func requireSchemaRequired(t *testing.T, schema map[string]any, name string) {
	t.Helper()
	requiredValues, ok := schema["required"].([]any)
	if !ok {
		t.Fatalf("schema required field missing or invalid: %#v", schema["required"])
	}
	for _, value := range requiredValues {
		if value == name {
			return
		}
	}
	t.Fatalf("schema missing required field %q; required=%v", name, requiredValues)
}

func requireSchemaConfirmProperty(t *testing.T, schema map[string]any, want bool) {
	t.Helper()
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties missing or invalid: %#v", schema["properties"])
	}
	_, hasConfirm := properties["confirm"]
	if hasConfirm != want {
		t.Fatalf("confirm property presence = %t, want %t", hasConfirm, want)
	}
}
