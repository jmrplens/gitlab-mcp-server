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
			Aliases: []string{
				"gitlab_render_markdown",
				"render markdown",
				"markdown to html",
				"preview markdown",
				"convert markdown to html",
				"gitlab flavored markdown render",
			},
			Usage:          "Render arbitrary Markdown text to HTML through GitLab's POST /markdown endpoint. Enable gfm for GitLab Flavored Markdown and set project to resolve issue/MR/user references against a project's context.",
			Tags:           []string{"markdown", "render"},
			RelatedActions: []string{"repository.file_get", "wiki.get"},
			OpenWorld:      true,
			OwnerPackage:   "markdown",
			IndividualTool: toolutil.IndividualToolSpec{
				Name:        "gitlab_render_markdown",
				Title:       toolutil.TitleFromName("gitlab_render_markdown"),
				Description: "Render arbitrary Markdown text to HTML using the GitLab Markdown API. Optionally apply GitLab Flavored Markdown and resolve references within a project's context. Returns: the rendered HTML string. See also: gitlab_file_get, gitlab_wiki_get.",
			},
		}),
	}
}
