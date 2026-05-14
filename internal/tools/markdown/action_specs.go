package markdown

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for markdown rendering actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		toolutil.NewActionSpec("markdown_render", toolutil.RouteAction(client, Render), toolutil.ActionSpecOptions{
			Tags:           []string{"markdown", "render"},
			RelatedActions: []string{"repository.file_get", "wiki.get"},
			ReadOnly:       true,
			Idempotent:     true,
			OpenWorld:      true,
			OwnerPackage:   "markdown",
			IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_render_markdown", Title: toolutil.TitleFromName("gitlab_render_markdown")},
		}),
	}
}
