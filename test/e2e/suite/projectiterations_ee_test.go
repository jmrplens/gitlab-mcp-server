//go:build e2e && enterprise

// projectiterations_ee_test.go tests project iteration list operations via the
// gitlab_issue meta-tool against a live GitLab EE/Ultimate instance.
//
// NOTE: The projectiterations package exposes only iteration_list_project (read-only list).
// Create/Update/Delete actions are not registered in the action spec catalog.
//
// Iterations require the project to be under a Group (not a User namespace).
// The test creates a group first, then a project under that group.
//
// CAN parallelize: separate project per test.
package suite

import (
	"context"
	"strconv"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projectiterations"
)

// TestMeta_ProjectIterations exercises project iteration list via gitlab_issue.
// Requires GitLab Premium (GITLAB_ENTERPRISE=true).
func TestMeta_ProjectIterations(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		t.Skip("project iterations require GitLab Premium/Ultimate")
	}
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx := context.Background()

	// Iterations only work on projects under a Group (not User namespace).
	// Create a group first, then a project under it.
	groupName := uniqueName("e2e-iterations-proj")
	grpOut, err := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "create",
		"params": map[string]any{
			"name":       groupName,
			"path":       groupName,
			"visibility": "private",
		},
	})
	requireNoError(t, err, "create group for project iteration test")
	groupIDStr := strconv.FormatInt(grpOut.ID, 10)

	t.Cleanup(func() {
		_ = callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "delete",
			"params": map[string]any{"group_id": groupIDStr},
		})
	})

	// Create project under the group.
	proj := createProjectUnderGroupMeta(ctx, t, sess.meta, grpOut.ID)

	t.Run("Meta/ProjectIteration/List", func(t *testing.T) {
		out, err := callToolOn[projectiterations.ListOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "iteration_list_project",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"state":      "opened",
			},
		})
		// 403 should not happen with the feature flag enabled in setup-gitlab.sh.
		// If it does, surface the error rather than skipping.
		requireNoError(t, err, "iteration_list_project")
		t.Logf("Project %s iterations: %d", proj.Path, len(out.Iterations))
	})
}
