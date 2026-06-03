//go:build e2e && enterprise

// groupprotectedbranches_ee_test.go exercises the group protected
// branches meta-tool against a live GitLab EE Ultimate instance.
// Group protected branches are Premium/Ultimate-only. The test creates
// a real protection rule, lists, gets, updates, and unprotects it
// via the per-test ledger.
//
// CANNOT parallelize: shares the meta session with the rest of the
// EE suite.
package suite

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupprotectedbranches"
)

// TestMeta_GroupProtectedBranchesEE exercises the group protected
// branches lifecycle (protect/list/get/update/unprotect) via the
// gitlab_group meta-tool.
func TestMeta_GroupProtectedBranchesEE(t *testing.T) {
	if !sess.enterprise {
		t.Skip("group protected branches require GitLab Premium/Ultimate")
	}
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx := context.Background()
	groupName := uniqueName("e2e-gpb")
	e2e := NewE2EContext(t)
	grp := CreateGroupMeta(ctx, e2e, sess.meta, groupName)

	branchName := "release-" + uniqueName("ee") + "/*"

	t.Run("Meta/GroupProtectedBranch/Protect", func(t *testing.T) {
		out, err := callToolOn[groupprotectedbranches.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "protected_branch_protect",
			"params": map[string]any{
				"group_id":               grp.gidStr(),
				"name":                   branchName,
				"push_access_level":      40,
				"merge_access_level":     40,
				"unprotect_access_level": 40,
				"allow_force_push":       false,
			},
		})
		requireNoError(t, err, "protected_branch_protect")
		requireTruef(t, out.Name == branchName, "name mismatch: got %q want %q", out.Name, branchName)
		t.Logf("Protected group branch %s", out.Name)
	})

	t.Run("Meta/GroupProtectedBranch/List", func(t *testing.T) {
		out, err := callToolOn[groupprotectedbranches.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "protected_branch_list",
			"params": map[string]any{
				"group_id": grp.gidStr(),
				"search":   branchName,
			},
		})
		requireNoError(t, err, "protected_branch_list")
		found := false
		for _, b := range out.Branches {
			if b.Name == branchName {
				found = true
				break
			}
		}
		requireTruef(t, found, "protected branch %q not in list", branchName)
		t.Logf("Group %s has %d protected branch(es) (created rule present)", groupName, len(out.Branches))
	})

	t.Run("Meta/GroupProtectedBranch/Get", func(t *testing.T) {
		out, err := callToolOn[groupprotectedbranches.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "protected_branch_get",
			"params": map[string]any{"group_id": grp.gidStr(), "branch": branchName},
		})
		requireNoError(t, err, "protected_branch_get")
		requireTruef(t, out.Name == branchName, "name mismatch: got %q want %q", out.Name, branchName)
		t.Logf("Got group protected branch %s", out.Name)
	})

	t.Run("Meta/GroupProtectedBranch/Update", func(t *testing.T) {
		_, err := callToolOn[groupprotectedbranches.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "protected_branch_update",
			"params": map[string]any{
				"group_id":         grp.gidStr(),
				"branch":           branchName,
				"allow_force_push": true,
			},
		})
		requireNoError(t, err, "protected_branch_update")
		t.Logf("Updated group protected branch %s (allow_force_push=true)", branchName)
	})

	t.Run("Meta/GroupProtectedBranch/Unprotect", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "protected_branch_unprotect",
			"params": map[string]any{"group_id": grp.gidStr(), "branch": branchName},
		})
		requireNoError(t, err, "protected_branch_unprotect")
		t.Logf("Unprotected group branch %s", branchName)
	})
}
