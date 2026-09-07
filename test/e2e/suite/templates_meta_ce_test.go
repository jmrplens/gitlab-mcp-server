//go:build e2e && !enterprise

// templates_meta_ce_test.go tests GitLab template MCP tools (CI YAML, Dockerfile,
// .gitignore, license, project templates) via the gitlab_template meta-tool
// against a live GitLab instance.
package suite

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/ciyamltemplates"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dockerfiletemplates"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/gitignoretemplates"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/licensetemplates"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projecttemplates"
)

// TestMeta_TemplatesCIYml exercises CI YAML template list and get actions
// through the gitlab_template meta-tool against a live GitLab instance.
//
// The test calls ci_yaml_list and ci_yaml_get (for a known template) via
// {action, params} arguments through the catalog-backed tool. Each subtest
// asserts the meta-tool returns the expected template payload and that the
// fetched template content is non-empty.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
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
		requireTruef(t, len(out.Templates) > 0, "ci_yml_list: expected templates")
		t.Logf("CI YAML templates: %d", len(out.Templates))
	})

	t.Run("CIYmlGet", func(t *testing.T) {
		out, err := callToolOn[ciyamltemplates.GetOutput](ctx, sess.meta, "gitlab_template", map[string]any{
			"action": "ci_yml_get",
			"params": map[string]any{"key": "Auto-DevOps"},
		})
		requireNoError(t, err, "ci_yml_get")
		requireTruef(t, out.Name != "", "ci_yml_get: expected non-empty name")
		requireTruef(t, out.Content != "", "ci_yml_get: expected non-empty content")
		t.Logf("Got CI YAML template: %s", out.Name)
	})
}

// TestMeta_TemplatesDockerfile exercises Dockerfile template list and get
// actions through the gitlab_template meta-tool against a live GitLab instance.
//
// The test calls dockerfile_list and dockerfile_get (for a known template)
// via {action, params} arguments through the catalog-backed tool. Each
// subtest asserts the meta-tool returns the expected template payload and
// that the fetched Dockerfile content is non-empty.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
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
		requireTruef(t, len(out.Templates) > 0, "dockerfile_list: expected templates")
		t.Logf("Dockerfile templates: %d", len(out.Templates))
	})

	t.Run("DockerfileGet", func(t *testing.T) {
		out, err := callToolOn[dockerfiletemplates.GetOutput](ctx, sess.meta, "gitlab_template", map[string]any{
			"action": "dockerfile_get",
			"params": map[string]any{"key": "Binary"},
		})
		requireNoError(t, err, "dockerfile_get")
		requireTruef(t, out.Content != "", "dockerfile_get: expected non-empty content")
		t.Logf("Got Dockerfile template: %s", out.Name)
	})
}

// TestMeta_TemplatesGitignore exercises .gitignore template list and get
// actions through the gitlab_template meta-tool against a live GitLab instance.
//
// The test calls gitignore_list and gitignore_get (for a known template)
// via {action, params} arguments through the catalog-backed tool. Each
// subtest asserts the meta-tool returns the expected template payload and
// that the fetched gitignore content is non-empty.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
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
		requireTruef(t, len(out.Templates) > 0, "gitignore_list: expected templates")
		t.Logf("Gitignore templates: %d", len(out.Templates))
	})

	t.Run("GitignoreGet", func(t *testing.T) {
		out, err := callToolOn[gitignoretemplates.GetOutput](ctx, sess.meta, "gitlab_template", map[string]any{
			"action": "gitignore_get",
			"params": map[string]any{"key": "Go"},
		})
		requireNoError(t, err, "gitignore_get")
		requireTruef(t, out.Content != "", "gitignore_get: expected non-empty content")
		t.Logf("Got gitignore template: %s", out.Name)
	})
}

// TestMeta_TemplatesLicense exercises license template list and get actions
// through the gitlab_template meta-tool against a live GitLab instance.
//
// The test calls license_list and license_get (for a known template like
// "mit") via {action, params} arguments through the catalog-backed tool.
// Each subtest asserts the meta-tool returns the expected template payload
// and that the fetched license content is non-empty.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
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
		requireTruef(t, len(out.Licenses) > 0, "license_list: expected licenses")
		t.Logf("License templates: %d", len(out.Licenses))
	})

	t.Run("LicenseGet", func(t *testing.T) {
		out, err := callToolOn[licensetemplates.GetOutput](ctx, sess.meta, "gitlab_template", map[string]any{
			"action": "license_get",
			"params": map[string]any{"key": "mit"},
		})
		requireNoError(t, err, "license_get")
		requireTruef(t, out.Key != "", "license_get: expected non-empty key")
		t.Logf("Got license template: %s", out.Key)
	})
}

// templatePageBudget bounds how many pages of a hundred a template listing is
// walked for. GitLab's largest template family is well under a thousand
// entries, so exhausting the budget means the pagination is not advancing
// rather than that the family is long.
const templatePageBudget = 10

// TestMeta_TemplatesProject exercises project template list and get actions
// through the gitlab_template meta-tool against a live GitLab instance.
//
// The test calls project_list (when available) and validates that the
// meta-tool returns the expected template payload. Get actions verify the
// fetched project template content is non-empty.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_TemplatesProject(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	t.Run("ProjectTemplateList", func(t *testing.T) {
		// GitLab ships around two hundred gitignore templates and serves them
		// in alphabetical order, so Go is nowhere near the first default page
		// of twenty: the listing has to be walked for the assertion below to be
		// an assertion rather than a coin toss. The walk is bounded so a
		// pagination defect ends the subtest instead of spinning it.
		var keys []string
		for page := 1; page <= templatePageBudget; page++ {
			out, err := callToolOn[projecttemplates.ListOutput](ctx, sess.meta, "gitlab_template", map[string]any{
				"action": "project_template_list",
				"params": map[string]any{
					"project_id":    proj.pidStr(),
					"template_type": "gitignores",
					"per_page":      100,
					"page":          page,
				},
			})
			requireNoError(t, err, "project_template_list")
			for _, tpl := range out.Templates {
				keys = append(keys, tpl.Key)
			}
			if !out.Pagination.HasMore {
				break
			}
		}
		// The gitignore templates ship with GitLab, so the sibling subtest can
		// ask for Go by key only because this listing really carries it.
		requireTruef(t, slices.Contains(keys, "Go"), "gitignore templates do not include Go: %v", keys)
		t.Logf("Project templates (gitignores): %d", len(keys))
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
		requireTruef(t, out.Name == "Go", "template name = %q, want %q", out.Name, "Go")
		requireTruef(t, out.Content != "", "template %q came back with no content", out.Name)
		t.Logf("Got project template: %s", out.Name)
	})
}
