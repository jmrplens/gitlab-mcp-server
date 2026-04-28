//go:build e2e

// users_test.go — E2E tests for user tools domain.
package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/users"
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
