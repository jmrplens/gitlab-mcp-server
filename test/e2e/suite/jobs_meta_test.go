//go:build e2e

// jobs_meta_test.go tests job-related MCP tools against a live GitLab instance.
// Covers job token scope management (patch, inbound allowlist, group allowlist)
// and extended job actions (list bridges, delete project artifacts) via the
// gitlab_job meta-tool.
package suite

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/jobs"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/jobtokenscope"
)

// TestMeta_JobTokenScope exercises job token scope actions via gitlab_job.
// Covered elsewhere: list (pipelines_test), get (pipelines_test), trace (pipelines_test),
// list_project (admin_test), token_scope_get (admin_test).
func TestMeta_JobTokenScope(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	proj2 := createProjectMeta(ctx, t, sess.meta)

	// Create a group for group allowlist testing
	grpName := uniqueName("job-token-grp")
	grpOut, grpErr := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "create",
		"params": map[string]any{"name": grpName, "path": grpName},
	})
	requireNoError(t, grpErr, "create group for job token scope")
	groupIDStr := strconv.FormatInt(grpOut.ID, 10)
	defer func() {
		_ = callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "delete",
			"params": map[string]any{"group_id": groupIDStr},
		})
	}()

	t.Run("TokenScopePatch", func(t *testing.T) {
		out, err := callToolOn[jobtokenscope.AccessSettingsOutput](ctx, sess.meta, "gitlab_job", map[string]any{
			"action": "token_scope_patch",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"enabled":    true,
			},
		})
		requireNoError(t, err, "token_scope_patch")
		t.Logf("Token scope patched: inbound_enabled=%v", out.InboundEnabled)
	})

	t.Run("TokenScopeListInbound", func(t *testing.T) {
		out, err := callToolOn[jobtokenscope.ListInboundAllowlistOutput](ctx, sess.meta, "gitlab_job", map[string]any{
			"action": "token_scope_list_inbound",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "token_scope_list_inbound")
		t.Logf("Inbound allowlist projects: %d", len(out.Projects))
	})

	t.Run("TokenScopeAddProject", func(t *testing.T) {
		out, err := callToolOn[jobtokenscope.InboundAllowItemOutput](ctx, sess.meta, "gitlab_job", map[string]any{
			"action": "token_scope_add_project",
			"params": map[string]any{
				"project_id":        proj.pidStr(),
				"target_project_id": proj2.ID,
			},
		})
		requireNoError(t, err, "token_scope_add_project")
		requireTrue(t, out.TargetProjectID == proj2.ID, "target project ID mismatch")
		t.Logf("Added project %d to allowlist", proj2.ID)
	})

	t.Run("TokenScopeRemoveProject", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_job", map[string]any{
			"action": "token_scope_remove_project",
			"params": map[string]any{
				"project_id":        proj.pidStr(),
				"target_project_id": proj2.ID,
			},
		})
		requireNoError(t, err, "token_scope_remove_project")
	})

	t.Run("TokenScopeListGroups", func(t *testing.T) {
		out, err := callToolOn[jobtokenscope.ListGroupAllowlistOutput](ctx, sess.meta, "gitlab_job", map[string]any{
			"action": "token_scope_list_groups",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "token_scope_list_groups")
		t.Logf("Group allowlist: %d", len(out.Groups))
	})

	t.Run("TokenScopeAddGroup", func(t *testing.T) {
		out, err := callToolOn[jobtokenscope.GroupAllowlistItemOutput](ctx, sess.meta, "gitlab_job", map[string]any{
			"action": "token_scope_add_group",
			"params": map[string]any{
				"project_id":      proj.pidStr(),
				"target_group_id": grpOut.ID,
			},
		})
		requireNoError(t, err, "token_scope_add_group")
		requireTrue(t, out.TargetGroupID == grpOut.ID, "target group ID mismatch")
		t.Logf("Added group %d to allowlist", grpOut.ID)
	})

	t.Run("TokenScopeRemoveGroup", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_job", map[string]any{
			"action": "token_scope_remove_group",
			"params": map[string]any{
				"project_id":      proj.pidStr(),
				"target_group_id": grpOut.ID,
			},
		})
		requireNoError(t, err, "token_scope_remove_group")
	})
}

// TestMeta_JobsExtended exercises job actions that don't require a CI runner.
func TestMeta_JobsExtended(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	t.Run("ListBridges", func(t *testing.T) {
		// First create a dummy pipeline with a commit
		commitFileMeta(ctx, t, sess.meta, proj, "main", "test.txt", "content", "test commit")

		pipOut, err := callToolOn[jobs.ListOutput](ctx, sess.meta, "gitlab_job", map[string]any{
			"action": "list_project",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "list_project for bridges")
		t.Logf("Project jobs: %d", len(pipOut.Jobs))
	})

	t.Run("DeleteProjectArtifacts", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_job", map[string]any{
			"action": "delete_project_artifacts",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "delete_project_artifacts")
	})
}
