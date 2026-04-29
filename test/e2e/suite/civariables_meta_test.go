//go:build e2e

// civariables_meta_test.go tests the group-level CI variable MCP tools via
// the gitlab_ci_variable meta-tool against a live GitLab instance.
// Exercises the group variable lifecycle: list → create → get → update.
package suite

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupvariables"
)

// TestMeta_CIVariablesGroup exercises group-level CI variable CRUD via gitlab_ci_variable.
func TestMeta_CIVariablesGroup(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Create a group for testing
	grpName := uniqueName("civar-grp")
	grpOut, grpErr := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "create",
		"params": map[string]any{"name": grpName, "path": grpName},
	})
	requireNoError(t, grpErr, "create group")
	groupIDStr := strconv.FormatInt(grpOut.ID, 10)
	defer func() {
		_ = callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "delete",
			"params": map[string]any{"group_id": groupIDStr},
		})
	}()

	varKey := fmt.Sprintf("E2E_GRP_%d", time.Now().UnixMilli())

	t.Run("GroupList", func(t *testing.T) {
		out, err := callToolOn[groupvariables.ListOutput](ctx, sess.meta, "gitlab_ci_variable", map[string]any{
			"action": "group_list",
			"params": map[string]any{"group_id": groupIDStr},
		})
		requireNoError(t, err, "group_list")
		t.Logf("Group variables: %d", len(out.Variables))
	})

	t.Run("GroupCreate", func(t *testing.T) {
		out, err := callToolOn[groupvariables.Output](ctx, sess.meta, "gitlab_ci_variable", map[string]any{
			"action": "group_create",
			"params": map[string]any{
				"group_id": groupIDStr,
				"key":      varKey,
				"value":    "test-value",
			},
		})
		requireNoError(t, err, "group_create")
		requireTruef(t, out.Key == varKey, "group_create: key mismatch")
		t.Logf("Created group variable %s", out.Key)
	})
	defer func() {
		_ = callToolVoidOn(ctx, sess.meta, "gitlab_ci_variable", map[string]any{
			"action": "group_delete",
			"params": map[string]any{"group_id": groupIDStr, "key": varKey},
		})
	}()

	t.Run("GroupGet", func(t *testing.T) {
		out, err := callToolOn[groupvariables.Output](ctx, sess.meta, "gitlab_ci_variable", map[string]any{
			"action": "group_get",
			"params": map[string]any{"group_id": groupIDStr, "key": varKey},
		})
		requireNoError(t, err, "group_get")
		requireTruef(t, out.Key == varKey, "group_get: key mismatch")
	})

	t.Run("GroupUpdate", func(t *testing.T) {
		out, err := callToolOn[groupvariables.Output](ctx, sess.meta, "gitlab_ci_variable", map[string]any{
			"action": "group_update",
			"params": map[string]any{
				"group_id": groupIDStr,
				"key":      varKey,
				"value":    "updated-value",
			},
		})
		requireNoError(t, err, "group_update")
		requireTruef(t, out.Key == varKey, "group_update: key mismatch")
	})
}
