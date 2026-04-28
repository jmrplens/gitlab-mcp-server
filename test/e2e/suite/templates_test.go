//go:build e2e

// templates_test.go tests GitLab template listing and markdown rendering
// MCP tools via the gitlab_template and gitlab_repository meta-tools
// against a live GitLab instance.
package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/gitignoretemplates"
	markdowntool "github.com/jmrplens/gitlab-mcp-server/internal/tools/markdown"
)

// TestMeta_Templates exercises template listing via the gitlab_template meta-tool.
func TestMeta_Templates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("Meta/Template/GitignoreList", func(t *testing.T) {
		out, err := callToolOn[gitignoretemplates.ListOutput](ctx, sess.meta, "gitlab_template", map[string]any{
			"action": "gitignore_list",
			"params": map[string]any{},
		})
		requireNoError(t, err, "gitignore template list")
		requireTruef(t, len(out.Templates) > 0, "expected at least 1 gitignore template")
		t.Logf("Listed %d gitignore templates", len(out.Templates))
	})

	t.Run("Meta/Template/CIYmlList", func(t *testing.T) {
		out, err := callToolOn[gitignoretemplates.ListOutput](ctx, sess.meta, "gitlab_template", map[string]any{
			"action": "ci_yml_list",
			"params": map[string]any{},
		})
		requireNoError(t, err, "CI yml template list")
		requireTruef(t, len(out.Templates) > 0, "expected at least 1 CI yml template")
		t.Logf("Listed %d CI yml templates", len(out.Templates))
	})
}

// TestMeta_MarkdownRender exercises markdown rendering via the gitlab_repository meta-tool.
func TestMeta_MarkdownRender(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("Meta/Markdown/Render", func(t *testing.T) {
		const maxRetries = 3
		var out markdowntool.RenderOutput
		var err error
		for attempt := range maxRetries {
			out, err = callToolOn[markdowntool.RenderOutput](ctx, sess.meta, "gitlab_repository", map[string]any{
				"action": "markdown_render",
				"params": map[string]any{
					"text": "**bold** text",
				},
			})
			if err == nil || !isTransientNetworkError(err) {
				break
			}
			t.Logf("markdown render attempt %d failed, retrying: %v", attempt+1, err)
			time.Sleep(time.Duration(attempt+1) * time.Second)
		}
		requireNoError(t, err, "markdown render")
		requireTruef(t, out.HTML != "", "expected non-empty HTML output")
		t.Logf("Rendered markdown: %s", out.HTML)
	})
}
