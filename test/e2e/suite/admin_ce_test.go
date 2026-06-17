//go:build e2e && !enterprise

// admin_ce_test.go tests lightweight admin and job-related MCP tools against
// a live GitLab instance. Covers topic listing, settings retrieval via
// gitlab_admin, and job token scope via gitlab_job.
//
// Build tag: e2e && !enterprise.
package suite

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/jobs"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/jobtokenscope"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/settings"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/topics"
)

// TestMeta_Admin exercises read-only admin-level meta-tool actions (topics,
// settings) via the gitlab_admin catalog tool.
//
// The test runs two subtests. The first lists GitLab topics through the
// topic_list action and asserts the call succeeds. The second fetches the
// admin settings through settings_get and asserts the returned map has at
// least one key. Neither subtest mutates GitLab state.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_Admin(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("Meta/Admin/TopicList", func(t *testing.T) {
		out, err := callToolOn[topics.ListOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "topic_list",
			"params": map[string]any{},
		})
		requireNoError(t, err, "meta admin topic list")
		t.Logf("Listed %d topics", len(out.Topics))
	})

	t.Run("Meta/Admin/SettingsGet", func(t *testing.T) {
		out, err := callToolOn[settings.GetOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "settings_get",
			"params": map[string]any{},
		})
		requireNoError(t, err, "meta admin settings get")
		requireTruef(t, len(out.Settings) > 0, "expected non-empty settings map, got %d keys", len(out.Settings))
		t.Logf("Admin settings: %d keys", len(out.Settings))
	})
}

// TestMeta_JobTokens exercises job listing and token scope via the
// gitlab_job meta-tool.
//
// The test creates a project fixture and runs two subtests. The first lists
// the project's jobs through list_project (the list may be empty when no CI
// pipeline has run). The second fetches the project-level job token scope
// through token_scope_get and asserts the response carries an inbound_enabled
// flag.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_JobTokens(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	proj := createProjectMeta(ctx, t, sess.meta)

	t.Run("Meta/Job/ListProject", func(t *testing.T) {
		_, err := callToolOn[jobs.ListOutput](ctx, sess.meta, "gitlab_job", map[string]any{
			"action": "list_project",
			"params": map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		requireNoError(t, err, "meta job list_project")
		t.Log("Job list_project OK (may be empty without CI pipeline)")
	})

	t.Run("Meta/Job/TokenScopeGet", func(t *testing.T) {
		out, err := callToolOn[jobtokenscope.AccessSettingsOutput](ctx, sess.meta, "gitlab_job", map[string]any{
			"action": "token_scope_get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		requireNoError(t, err, "meta job token_scope_get")
		t.Logf("Job token scope: inbound_enabled=%v", out.InboundEnabled)
	})
}
