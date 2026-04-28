//go:build e2e

// cilint_test.go tests the CI lint MCP tools against a live GitLab instance
// using both individual tools and the gitlab_template meta-tool. Validates
// CI configuration content and project-level linting.
package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/cilint"
)

// TestIndividual_CILint exercises CI lint operations using individual MCP
// tools: validates inline YAML content and lints a project without a CI config.
func TestIndividual_CILint(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)

	t.Run("LintContent", func(t *testing.T) {
		out, err := callToolOn[cilint.Output](ctx, sess.individual, "gitlab_ci_lint", cilint.ContentInput{
			ProjectID: proj.pidOf(),
			Content:   "stages:\n  - build\nbuild_job:\n  stage: build\n  script:\n    - echo hello",
		})
		requireNoError(t, err, "CI lint content")
		requireTruef(t, out.Valid, "expected valid CI config, got invalid: %v", out.Errors)
	})

	t.Run("LintProject", func(t *testing.T) {
		out, err := callToolOn[cilint.Output](ctx, sess.individual, "gitlab_ci_lint_project", cilint.ProjectInput{
			ProjectID: proj.pidOf(),
		})
		requireNoError(t, err, "CI lint project")
		requireTruef(t, !out.Valid, "expected CI lint to return invalid for project without .gitlab-ci.yml")
		t.Logf("CI lint project: valid=%v, errors=%v", out.Valid, out.Errors)
	})
}

// TestMeta_CILint exercises the same CI lint operations via the
// gitlab_template meta-tool.
func TestMeta_CILint(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	t.Run("LintContent", func(t *testing.T) {
		out, err := callToolOn[cilint.Output](ctx, sess.meta, "gitlab_template", map[string]any{
			"action": "lint",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"content":    "stages:\n  - test\ntest_job:\n  stage: test\n  script:\n    - echo test",
			},
		})
		requireNoError(t, err, "CI lint content meta")
		requireTruef(t, out.Valid, "expected valid CI config, got invalid: %v", out.Errors)
	})

	t.Run("LintProject", func(t *testing.T) {
		out, err := callToolOn[cilint.Output](ctx, sess.meta, "gitlab_template", map[string]any{
			"action": "lint_project",
			"params": map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		requireNoError(t, err, "CI lint project meta")
		requireTruef(t, !out.Valid, "expected CI lint to return invalid for project without .gitlab-ci.yml")
		t.Logf("CI lint project: valid=%v, errors=%v", out.Valid, out.Errors)
	})
}
