//go:build e2e

// pushrules_test.go tests the project push rule MCP tools against a live GitLab instance.
// Requires GitLab Premium/Ultimate (GITLAB_ENTERPRISE=true). Covers add → get → edit → delete
// for both individual tools and the gitlab_project meta-tool.
package suite

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projects"
)

// TestIndividual_PushRules exercises push rule CRUD via individual MCP tools.
// Requires GitLab Premium/Ultimate (GITLAB_ENTERPRISE=true).
func TestIndividual_PushRules(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}

	ctx := context.Background()
	proj := createProject(ctx, t, sess.individual)

	t.Run("Add", func(t *testing.T) {
		out, err := callToolOn[projects.PushRuleOutput](ctx, sess.individual, "gitlab_project_add_push_rule", projects.AddPushRuleInput{
			ProjectID:          proj.pidOf(),
			CommitMessageRegex: "^[A-Z].*",
			MaxFileSize:        new(int64(50)),
		})
		requireNoError(t, err, "add push rule")
		requireTruef(t, out.ID > 0, "push rule ID should be positive, got %d", out.ID)
		t.Logf("Added push rule %d", out.ID)
	})

	t.Run("Get", func(t *testing.T) {
		out, err := callToolOn[projects.PushRuleOutput](ctx, sess.individual, "gitlab_project_get_push_rules", projects.GetPushRulesInput{
			ProjectID: proj.pidOf(),
		})
		requireNoError(t, err, "get push rules")
		requireTruef(t, out.ID > 0, "push rule ID should be positive")
		requireTruef(t, out.MaxFileSize == 50, "expected max_file_size=50, got %d", out.MaxFileSize)
		t.Logf("Got push rules: max_file_size=%d", out.MaxFileSize)
	})

	t.Run("Edit", func(t *testing.T) {
		out, err := callToolOn[projects.PushRuleOutput](ctx, sess.individual, "gitlab_project_edit_push_rule", projects.EditPushRuleInput{
			ProjectID:   proj.pidOf(),
			MaxFileSize: new(int64(100)),
		})
		requireNoError(t, err, "edit push rule")
		requireTruef(t, out.MaxFileSize == 100, "expected max_file_size=100 after edit, got %d", out.MaxFileSize)
		t.Logf("Edited push rule: max_file_size=%d", out.MaxFileSize)
	})

	t.Run("Delete", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.individual, "gitlab_project_delete_push_rule", projects.DeletePushRuleInput{
			ProjectID: proj.pidOf(),
		})
		requireNoError(t, err, "delete push rule")
		t.Log("Deleted push rule")
	})
}

// TestMeta_PushRules exercises push rule CRUD via the gitlab_project meta-tool.
// Requires GitLab Premium/Ultimate (GITLAB_ENTERPRISE=true).
func TestMeta_PushRules(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}

	ctx := context.Background()
	proj := createProjectMeta(ctx, t, sess.meta)

	t.Run("Meta/PushRule/Add", func(t *testing.T) {
		out, err := callToolOn[projects.PushRuleOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "push_rule_add",
			"params": map[string]any{
				"project_id":           proj.pidStr(),
				"commit_message_regex": "^[A-Z].*",
				"max_file_size":        50,
			},
		})
		requireNoError(t, err, "meta add push rule")
		requireTruef(t, out.ID > 0, "push rule ID should be positive, got %d", out.ID)
		t.Logf("Added push rule %d via meta-tool", out.ID)
	})

	t.Run("Meta/PushRule/Get", func(t *testing.T) {
		out, err := callToolOn[projects.PushRuleOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "push_rule_get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		requireNoError(t, err, "meta get push rules")
		requireTruef(t, out.ID > 0, "push rule ID should be positive")
		requireTruef(t, out.MaxFileSize == 50, "expected max_file_size=50, got %d", out.MaxFileSize)
		t.Logf("Got push rules via meta-tool: max_file_size=%d", out.MaxFileSize)
	})

	t.Run("Meta/PushRule/Edit", func(t *testing.T) {
		out, err := callToolOn[projects.PushRuleOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "push_rule_edit",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"max_file_size": 100,
			},
		})
		requireNoError(t, err, "meta edit push rule")
		requireTruef(t, out.MaxFileSize == 100, "expected max_file_size=100, got %d", out.MaxFileSize)
		t.Logf("Edited push rule via meta-tool: max_file_size=%d", out.MaxFileSize)
	})

	t.Run("Meta/PushRule/Delete", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "push_rule_delete",
			"params": map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		requireNoError(t, err, "meta delete push rule")
		t.Log("Deleted push rule via meta-tool")
	})
}
