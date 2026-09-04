//go:build e2e && enterprise

// group_protected_envs_ee_test.go exercises the group protected
// environments meta-tool against a live GitLab EE Ultimate instance.
// Group protected environments are Premium/Ultimate-only. The test
// creates a real protection rule, lists, gets, updates, and
// unprotects it via the per-test ledger.
//
// CANNOT parallelize: shares the meta session with the rest of the
// EE suite.
package suite

import (
	"context"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupprotectedenvs"
)

// TestMeta_GroupProtectedEnvironmentsEE exercises the group
// protected environments lifecycle (protect/list/get/update/unprotect)
// via the gitlab_group meta-tool.
func TestMeta_GroupProtectedEnvironmentsEE(t *testing.T) {
	if !sess.enterprise {
		t.Skip("group protected environments require GitLab Premium/Ultimate")
	}
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx := context.Background()
	groupName := uniqueName("e2e-gpe")
	e2e := NewE2EContext(t)
	grp := CreateGroupMeta(ctx, e2e, sess.meta, groupName)

	const envName = "production"

	t.Run("Meta/GroupProtectedEnv/InvalidTier_Hint", func(t *testing.T) {
		_, err := callToolOn[groupprotectedenvs.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "protected_env_protect",
			"params": map[string]any{
				"group_id": grp.gidStr(),
				"name":     uniqueName("invalid-") + "-env",
				"deploy_access_levels": []map[string]any{
					{"access_level": 40},
				},
			},
		})
		requireTruef(t, err != nil, "expected protected_env_protect invalid tier error")
		requireTruef(t, strings.Contains(err.Error(), "valid group protected environment tiers"), "expected tier hint, got %v", err)
		t.Logf("Invalid tier error path validated: %v", err)
	})

	t.Run("Meta/GroupProtectedEnv/Protect", func(t *testing.T) {
		out, err := callToolOn[groupprotectedenvs.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "protected_env_protect",
			"params": map[string]any{
				"group_id": grp.gidStr(),
				"name":     envName,
				"deploy_access_levels": []map[string]any{
					{"access_level": 40},
				},
			},
		})
		requireNoError(t, err, "protected_env_protect")
		requireTruef(t, out.Name == envName, "name mismatch: got %q want %q", out.Name, envName)
		requireTruef(t, len(out.DeployAccessLevels) >= 1, "expected at least 1 deploy access level")
		t.Logf("Protected group environment %s", out.Name)
	})

	t.Run("Meta/GroupProtectedEnv/List", func(t *testing.T) {
		out, err := callToolOn[groupprotectedenvs.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "protected_env_list",
			"params": map[string]any{"group_id": grp.gidStr()},
		})
		requireNoError(t, err, "protected_env_list")
		found := false
		for _, e := range out.Environments {
			if e.Name == envName {
				found = true
				break
			}
		}
		requireTruef(t, found, "protected env %q not in list", envName)
		t.Logf("Group %s has %d protected env(s) (created env present)", groupName, len(out.Environments))
	})

	t.Run("Meta/GroupProtectedEnv/Get", func(t *testing.T) {
		out, err := callToolOn[groupprotectedenvs.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "protected_env_get",
			"params": map[string]any{"group_id": grp.gidStr(), "environment": envName},
		})
		requireNoError(t, err, "protected_env_get")
		requireTruef(t, out.Name == envName, "name mismatch: got %q want %q", out.Name, envName)
		t.Logf("Got group protected env %s", out.Name)
	})

	t.Run("Meta/GroupProtectedEnv/Update", func(t *testing.T) {
		requireTruef(t, envName != "", "envName not set")
		// Note: required_approval_count is deprecated as of EPIC
		// gitlab-org#9662; the modern equivalent is required_approvals
		// (an array of approval-rule objects) but a self-managed EE
		// sandbox without the fix applied rejects it with 422. We
		// exercise the update path with a benign no-op payload (just
		// re-declaring deploy_access_levels) so the routing and
		// wrapping are validated without depending on a deprecated
		// field that the test sandbox can't accept.
		out, err := callToolOn[groupprotectedenvs.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "protected_env_update",
			"params": map[string]any{
				"group_id":    grp.gidStr(),
				"environment": envName,
				"deploy_access_levels": []map[string]any{
					{"access_level": 30},
				},
			},
		})
		requireNoError(t, err, "protected_env_update")
		requireTruef(t, out.Name == envName, "updated env name = %q, want %q", out.Name, envName)
		t.Logf("Updated group protected env %s", out.Name)
	})

	t.Run("Meta/GroupProtectedEnv/Unprotect", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "protected_env_unprotect",
			"params": map[string]any{"group_id": grp.gidStr(), "environment": envName},
		})
		requireNoError(t, err, "protected_env_unprotect")
		t.Logf("Unprotected group env %s", envName)
	})
}
