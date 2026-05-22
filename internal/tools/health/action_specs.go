package health

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for MCP server health actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		healthSpec("status", client, "gitlab_server_status"),
		healthSpec("health_check", client, ""),
	}
}

func healthSpec(name string, client *gitlabclient.Client, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, toolutil.RouteAction(client, Check), toolutil.ActionSpecOptions{
		Tags:           []string{"server", "health", "diagnostics"},
		OpenWorld:      true,
		OwnerPackage:   "health",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	})
}
