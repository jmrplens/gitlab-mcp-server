package markdown

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for Markdown rendering actions
// exposed as MCP tools. The render route is projected into the dynamic,
// meta, individual, and audit surfaces by the action catalog
// (ADR-0004).
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_render_markdown — render Markdown text to HTML via the GitLab API.
		toolutil.NewReadActionSpec("markdown_render", toolutil.RouteAction(client, Render), toolutil.ActionSpecOptions{
			Aliases: []string{"gitlab_render_markdown"}, Usage: "Use to execute markdown_render action.", Tags: []string{"markdown", "render"},
			RelatedActions: []string{"repository.file_get", "wiki.get"},
			OpenWorld:      true,
			OwnerPackage:   "markdown",
			IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_render_markdown", Title: toolutil.TitleFromName("gitlab_render_markdown")},
		}),
	}
}
