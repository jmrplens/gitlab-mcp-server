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
	return toolutil.NewReadActionSpec(name, toolutil.RouteAction(client, Check), healthOptions(name, individualTool))
}

func healthOptions(name, individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Tags: []string{"server", "health", "diagnostics", "connectivity"},
		Usage:          "Verify MCP server connectivity to GitLab, authenticated identity, and response health before troubleshooting other tool failures.",
		RelatedActions: []string{"admin.metadata_get", "user.me"},
		OpenWorld:      true,
		OwnerPackage:   "health",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	switch name {
	case "status":
		options.Aliases = []string{"mcp server status", "gitlab server status", "gitlab connectivity status"}
		options.IndividualTool.Description = "Check MCP server connectivity, GitLab reachability, and authenticated identity details. Returns: the current server and GitLab health diagnostics object. See also: gitlab_get_metadata, gitlab_user_current."
	case "health_check":
		options.Aliases = []string{"health check", "server health check", "connectivity check", "gitlab health check", "server diagnostics", "run diagnostics", "diagnostics", "server status check"}
	}
	return options
}
