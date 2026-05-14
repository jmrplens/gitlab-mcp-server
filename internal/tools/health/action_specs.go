package health

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for MCP server health actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		healthSpec("status", client, "gitlab_server_status"),
		healthSpec("health_check", client, ""),
	}
}

func healthSpec(name string, client *gitlabclient.Client, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, toolutil.RouteAction(client, Check), toolutil.ActionSpecOptions{
		Tags:           []string{"server", "health", "diagnostics"},
		ReadOnly:       true,
		Idempotent:     true,
		OpenWorld:      true,
		OwnerPackage:   "health",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	})
}
