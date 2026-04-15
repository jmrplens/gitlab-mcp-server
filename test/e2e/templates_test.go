//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/gitignoretemplates"
	markdowntool "github.com/jmrplens/gitlab-mcp-server/internal/tools/markdown"
)

// TestMeta_Templates exercises template listing via the gitlab_template meta-tool.
func TestMeta_Templates(t *testing.T) {
	ctx := context.Background()

	t.Run("Meta/Template/GitignoreList", func(t *testing.T) {
		out, err := callToolOn[gitignoretemplates.ListOutput](ctx, sess.meta, "gitlab_template", map[string]any{
			"action": "gitignore_list",
			"params": map[string]any{},
		})
		requireNoError(t, err, "gitignore template list")
		requireTrue(t, len(out.Templates) > 0, "expected at least 1 gitignore template")
		t.Logf("Listed %d gitignore templates", len(out.Templates))
	})

	t.Run("Meta/Template/CIYmlList", func(t *testing.T) {
		out, err := callToolOn[gitignoretemplates.ListOutput](ctx, sess.meta, "gitlab_template", map[string]any{
			"action": "ci_yml_list",
			"params": map[string]any{},
		})
		requireNoError(t, err, "CI yml template list")
		requireTrue(t, len(out.Templates) > 0, "expected at least 1 CI yml template")
		t.Logf("Listed %d CI yml templates", len(out.Templates))
	})
}

// TestMeta_MarkdownRender exercises markdown rendering via the gitlab_repository meta-tool.
func TestMeta_MarkdownRender(t *testing.T) {
	ctx := context.Background()

	t.Run("Meta/Markdown/Render", func(t *testing.T) {
		out, err := callToolOn[markdowntool.RenderOutput](ctx, sess.meta, "gitlab_repository", map[string]any{
			"action": "markdown_render",
			"params": map[string]any{
				"text": "**bold** text",
			},
		})
		requireNoError(t, err, "markdown render")
		requireTrue(t, out.HTML != "", "expected non-empty HTML output")
		t.Logf("Rendered markdown: %s", out.HTML)
	})
}
