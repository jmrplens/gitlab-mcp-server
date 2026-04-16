//go:build e2e

package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/ciyamltemplates"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/dockerfiletemplates"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/gitignoretemplates"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/licensetemplates"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projecttemplates"
)

// TestMeta_TemplatesCIYml exercises CI YAML template list/get actions.
func TestMeta_TemplatesCIYml(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	t.Run("CIYmlList", func(t *testing.T) {
		out, err := callToolOn[ciyamltemplates.ListOutput](ctx, sess.meta, "gitlab_template", map[string]any{
			"action": "ci_yml_list",
			"params": map[string]any{},
		})
		requireNoError(t, err, "ci_yml_list")
		requireTrue(t, len(out.Templates) > 0, "ci_yml_list: expected templates")
		t.Logf("CI YAML templates: %d", len(out.Templates))
	})

	t.Run("CIYmlGet", func(t *testing.T) {
		out, err := callToolOn[ciyamltemplates.GetOutput](ctx, sess.meta, "gitlab_template", map[string]any{
			"action": "ci_yml_get",
			"params": map[string]any{"key": "Auto-DevOps"},
		})
		requireNoError(t, err, "ci_yml_get")
		requireTrue(t, out.Name != "", "ci_yml_get: expected non-empty name")
		requireTrue(t, out.Content != "", "ci_yml_get: expected non-empty content")
		t.Logf("Got CI YAML template: %s", out.Name)
	})
}

// TestMeta_TemplatesDockerfile exercises Dockerfile template list/get actions.
func TestMeta_TemplatesDockerfile(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	t.Run("DockerfileList", func(t *testing.T) {
		out, err := callToolOn[dockerfiletemplates.ListOutput](ctx, sess.meta, "gitlab_template", map[string]any{
			"action": "dockerfile_list",
			"params": map[string]any{},
		})
		requireNoError(t, err, "dockerfile_list")
		requireTrue(t, len(out.Templates) > 0, "dockerfile_list: expected templates")
		t.Logf("Dockerfile templates: %d", len(out.Templates))
	})

	t.Run("DockerfileGet", func(t *testing.T) {
		out, err := callToolOn[dockerfiletemplates.GetOutput](ctx, sess.meta, "gitlab_template", map[string]any{
			"action": "dockerfile_get",
			"params": map[string]any{"key": "Binary"},
		})
		requireNoError(t, err, "dockerfile_get")
		requireTrue(t, out.Content != "", "dockerfile_get: expected non-empty content")
		t.Logf("Got Dockerfile template: %s", out.Name)
	})
}

// TestMeta_TemplatesGitignore exercises .gitignore template list/get actions.
func TestMeta_TemplatesGitignore(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	t.Run("GitignoreList", func(t *testing.T) {
		out, err := callToolOn[gitignoretemplates.ListOutput](ctx, sess.meta, "gitlab_template", map[string]any{
			"action": "gitignore_list",
			"params": map[string]any{},
		})
		requireNoError(t, err, "gitignore_list")
		requireTrue(t, len(out.Templates) > 0, "gitignore_list: expected templates")
		t.Logf("Gitignore templates: %d", len(out.Templates))
	})

	t.Run("GitignoreGet", func(t *testing.T) {
		out, err := callToolOn[gitignoretemplates.GetOutput](ctx, sess.meta, "gitlab_template", map[string]any{
			"action": "gitignore_get",
			"params": map[string]any{"key": "Go"},
		})
		requireNoError(t, err, "gitignore_get")
		requireTrue(t, out.Content != "", "gitignore_get: expected non-empty content")
		t.Logf("Got gitignore template: %s", out.Name)
	})
}

// TestMeta_TemplatesLicense exercises license template list/get actions.
func TestMeta_TemplatesLicense(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	t.Run("LicenseList", func(t *testing.T) {
		out, err := callToolOn[licensetemplates.ListOutput](ctx, sess.meta, "gitlab_template", map[string]any{
			"action": "license_list",
			"params": map[string]any{},
		})
		requireNoError(t, err, "license_list")
		requireTrue(t, len(out.Licenses) > 0, "license_list: expected licenses")
		t.Logf("License templates: %d", len(out.Licenses))
	})

	t.Run("LicenseGet", func(t *testing.T) {
		out, err := callToolOn[licensetemplates.GetOutput](ctx, sess.meta, "gitlab_template", map[string]any{
			"action": "license_get",
			"params": map[string]any{"key": "mit"},
		})
		requireNoError(t, err, "license_get")
		requireTrue(t, out.Key != "", "license_get: expected non-empty key")
		t.Logf("Got license template: %s", out.Key)
	})
}

// TestMeta_TemplatesProject exercises project template list/get actions.
func TestMeta_TemplatesProject(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	t.Run("ProjectTemplateList", func(t *testing.T) {
		out, err := callToolOn[projecttemplates.ListOutput](ctx, sess.meta, "gitlab_template", map[string]any{
			"action": "project_template_list",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"template_type": "gitignores",
			},
		})
		requireNoError(t, err, "project_template_list")
		t.Logf("Project templates (gitignores): %d", len(out.Templates))
	})

	t.Run("ProjectTemplateGet", func(t *testing.T) {
		out, err := callToolOn[projecttemplates.GetOutput](ctx, sess.meta, "gitlab_template", map[string]any{
			"action": "project_template_get",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"template_type": "gitignores",
				"key":           "Go",
			},
		})
		requireNoError(t, err, "project_template_get")
		t.Logf("Got project template: %s", out.Name)
	})
}
