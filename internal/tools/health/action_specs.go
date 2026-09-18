package health

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	// statusToolName is the individual tool the status action projects as. The
	// health check action projects none: both actions run the one Check
	// handler, so a second tool would be that call registered twice.
	statusToolName = "gitlab_server_status"

	// statusToolDescription is what a model reads about that tool before
	// calling it.
	statusToolDescription = "Check MCP server connectivity, GitLab reachability, and authenticated identity details. Returns: the current server and GitLab health diagnostics object. See also: gitlab_get_metadata, gitlab_user_current."
)

// ActionSpecs returns canonical specs for MCP server health actions.
//
// Both actions route to the one Check handler and differ only in how a model
// reaches them: status is the one projected as an individual tool, and
// health_check carries the phrases a model reaches for once something is
// already wrong. Each action's aliases and its individual-tool projection are
// written here, at the one place that knows both names, rather than resolved
// from the name again inside the options builder: that switch had one arm per
// name and no caller could pass a third, so its second test was decided one way
// for ever and the aliases it guarded were assigned and then overwritten by
// whichever arm ran.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		healthSpec("status", client, toolutil.IndividualToolSpec{
			Name:        statusToolName,
			Title:       toolutil.TitleFromName(statusToolName),
			Description: statusToolDescription,
		}, "mcp server status", "gitlab server status", "gitlab connectivity status"),
		healthSpec("health_check", client, toolutil.IndividualToolSpec{},
			"health check", "server health check", "connectivity check", "gitlab health check",
			"server diagnostics", "run diagnostics", "diagnostics", "server status check"),
	}
}

// healthSpec builds one read-only action over the shared Check handler.
//
// Read-only is a classification rather than a detail: --read-only and a
// read_api token both narrow the surface per action, and this is the action a
// caller reaches for when something is already wrong, so it has to survive
// both.
func healthSpec(name string, client *gitlabclient.Client, individual toolutil.IndividualToolSpec, aliases ...string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, toolutil.RouteAction(client, Check), toolutil.ActionSpecOptions{
		Aliases:        aliases,
		Tags:           []string{"server", "health", "diagnostics", "connectivity"},
		Usage:          "Verify MCP server connectivity to GitLab, authenticated identity, and response health before troubleshooting other tool failures.",
		RelatedActions: []string{"admin.metadata_get", "user.me"},
		OpenWorld:      true,
		OwnerPackage:   "health",
		IndividualTool: individual,
	})
}
