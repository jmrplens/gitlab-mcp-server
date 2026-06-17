package issuestatistics

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for issue statistics actions exposed
// as MCP tools. The global, group, and project read routes are projected
// into the dynamic, meta, individual, and audit surfaces by the action
// catalog (ADR-0004).
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_get_issue_statistics — read global issue counts.
		issueStatisticsReadSpec("statistics_get", toolutil.RouteAction(client, Get), "gitlab_get_issue_statistics"),
		// gitlab_get_group_issue_statistics — read group issue counts.
		issueStatisticsReadSpec("statistics_get_group", toolutil.RouteAction(client, GetGroup), "gitlab_get_group_issue_statistics"),
		// gitlab_get_project_issue_statistics — read project issue counts.
		issueStatisticsReadSpec("statistics_get_project", toolutil.RouteAction(client, GetProject), "gitlab_get_project_issue_statistics"),
	}
}

// issueStatisticsReadSpec builds a read-only [toolutil.ActionSpec] for an
// issue statistics action using the package's default
// [issueStatisticsOptions].
func issueStatisticsReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, issueStatisticsOptions(individualTool))
}

// issueStatisticsOptions returns the base [toolutil.ActionSpecOptions]
// shared by every issue statistics action (tags, owner, individual tool
// metadata).
func issueStatisticsOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute issuestatistics domain action.", Tags: []string{"issue", "statistics"},
		OpenWorld:      true,
		OwnerPackage:   "issuestatistics",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
