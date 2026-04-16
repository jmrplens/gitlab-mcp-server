//go:build e2e

package suite

import (
	"context"
	"testing"

	markdowntools "github.com/jmrplens/gitlab-mcp-server/internal/tools/markdown"
)

// TestIndividual_MarkdownRender exercises the gitlab_render_markdown individual tool.
func TestIndividual_MarkdownRender(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("RenderBasic", func(t *testing.T) {
		out, err := callToolOn[markdowntools.RenderOutput](ctx, sess.individual, "gitlab_render_markdown", markdowntools.RenderInput{
			Text: "**bold** and _italic_",
		})
		requireNoError(t, err, "render markdown")
		requireTrue(t, out.HTML != "", "expected non-empty HTML output")
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
		requireTrue(t, out.HTML != "", "expected non-empty GFM HTML output")
		t.Logf("Rendered GFM HTML: %s", out.HTML)
	})
}
