//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupvariables"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestMeta_GroupVariables exercises group CI variable CRUD via the gitlab_ci_variable meta-tool.
func TestMeta_GroupVariables(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Create a dedicated group for this test.
	groupPath := fmt.Sprintf("e2e-grpvar-%d", time.Now().UnixMilli())
	grp, err := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "create",
		"params": map[string]any{
			"name":       groupPath,
			"path":       groupPath,
			"visibility": "public",
		},
	})
	requireNoError(t, err, "create group for variables")
	requireTrue(t, grp.ID > 0, "group ID should be positive")
	groupID := grp.ID
	t.Logf("Created group %d (%s) for variable tests", groupID, grp.FullPath)

	t.Cleanup(func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanCancel()
		_ = callToolVoidOn(cleanCtx, sess.individual, "gitlab_group_delete", groups.DeleteInput{
			GroupID: toolutil.StringOrInt(strconv.FormatInt(groupID, 10)),
		})
	})

	gid := strconv.FormatInt(groupID, 10)
	varKey := "E2E_GROUP_VAR"

	t.Run("Meta/GroupVariable/Create", func(t *testing.T) {
		out, err := callToolOn[groupvariables.Output](ctx, sess.meta, "gitlab_ci_variable", map[string]any{
			"action": "group_create",
			"params": map[string]any{
				"group_id": gid,
				"key":      varKey,
				"value":    "group-test-value",
			},
		})
		requireNoError(t, err, "group variable create")
		requireTrue(t, out.Key == varKey, "expected key %s, got %q", varKey, out.Key)
		t.Logf("Created group variable: %s", out.Key)
	})

	t.Run("Meta/GroupVariable/List", func(t *testing.T) {
		out, err := callToolOn[groupvariables.ListOutput](ctx, sess.meta, "gitlab_ci_variable", map[string]any{
			"action": "group_list",
			"params": map[string]any{
				"group_id": gid,
			},
		})
		requireNoError(t, err, "group variable list")
		requireTrue(t, len(out.Variables) >= 1, "expected at least 1 group variable")
		t.Logf("Listed %d group variable(s)", len(out.Variables))
	})

	t.Run("Meta/GroupVariable/Get", func(t *testing.T) {
		out, err := callToolOn[groupvariables.Output](ctx, sess.meta, "gitlab_ci_variable", map[string]any{
			"action": "group_get",
			"params": map[string]any{
				"group_id": gid,
				"key":      varKey,
			},
		})
		requireNoError(t, err, "group variable get")
		requireTrue(t, out.Key == varKey, "group variable key mismatch")
		t.Logf("Got group variable: %s=%s", out.Key, out.Value)
	})

	t.Run("Meta/GroupVariable/Update", func(t *testing.T) {
		out, err := callToolOn[groupvariables.Output](ctx, sess.meta, "gitlab_ci_variable", map[string]any{
			"action": "group_update",
			"params": map[string]any{
				"group_id": gid,
				"key":      varKey,
				"value":    "updated-group-value",
			},
		})
		requireNoError(t, err, "group variable update")
		requireTrue(t, out.Key == varKey, "expected key %s, got %q", varKey, out.Key)
		t.Logf("Updated group variable: %s", out.Key)
	})

	t.Run("Meta/GroupVariable/Delete", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_ci_variable", map[string]any{
			"action": "group_delete",
			"params": map[string]any{
				"group_id": gid,
				"key":      varKey,
			},
		})
		requireNoError(t, err, "group variable delete")
		t.Logf("Deleted group variable: %s", varKey)
	})
}
