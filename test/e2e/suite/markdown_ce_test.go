//go:build e2e && !enterprise

// markdown_ce_test.go tests the GitLab markdown rendering MCP tool against a live
// GitLab instance. Covers basic markdown-to-HTML rendering and GitLab Flavored
// Markdown (GFM) rendering with project context.
//
// Build tag: e2e && !enterprise.
package suite

import (
	"context"
	"testing"

	markdowntools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/markdown"
)

// TestIndividual_MarkdownRender exercises the gitlab_render_markdown
// individual tool against a live GitLab instance.
//
// The test renders inline markdown (bold + italic) and project-scoped
// GitLab Flavored Markdown (GFM) as subtests, asserting that the
// returned HTML is non-empty and that the rendered output contains
// expected tags. Each subtest runs without a project fixture so the
// render endpoint stays stateless.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual.
func TestIndividual_MarkdownRender(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("RenderBasic", func(t *testing.T) {
		out, err := callToolOn[markdowntools.RenderOutput](ctx, sess.individual, "gitlab_render_markdown", markdowntools.RenderInput{
			Text: "**bold** and _italic_",
		})
		requireNoError(t, err, "render markdown")
		requireTruef(t, out.HTML != "", "expected non-empty HTML output")
		t.Logf("Rendered HTML: %s", out.HTML)
	})

	t.Run("RenderGFM", func(t *testing.T) {
		proj := createProject(ctx, t, sess.individual)
		out, err := callToolOn[markdowntools.RenderOutput](ctx, sess.individual, "gitlab_render_markdown", markdowntools.RenderInput{
			Text:    "Check issue #1 and MR !1",
			GFM:     true,
			Project: proj.Path,
		})
		requireNoError(t, err, "render GFM markdown")
		requireTruef(t, out.HTML != "", "expected non-empty GFM HTML output")
		t.Logf("Rendered GFM HTML: %s", out.HTML)
	})
}
