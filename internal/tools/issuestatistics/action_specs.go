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

// issueStatisticsOptions returns the [toolutil.ActionSpecOptions] for an
// issue statistics action: shared tags/owner metadata plus per-action
// discovery metadata (aliases, related actions, and a "Returns: … See also: …"
// individual-tool description) mirroring the issues domain conventions
// (R-META).
func issueStatisticsOptions(individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute issuestatistics domain action.", Tags: []string{"issue", "statistics"},
		OpenWorld:      true,
		OwnerPackage:   "issuestatistics",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	decorateIssueStatisticsMeta(&options, individualTool)
	return options
}

// decorateIssueStatisticsMeta applies per-action discovery metadata for the
// three issue-statistics read tools: natural-language aliases, related
// actions, and a "Returns: … See also: …" individual-tool description.
func decorateIssueStatisticsMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	switch individualTool {
	case "gitlab_get_issue_statistics":
		options.Usage = "Get aggregate issue counts (all, opened, closed) across every project visible to the authenticated user, optionally filtered by labels, milestone, assignee, author, dates, or search."
		options.Aliases = []string{individualTool, "issue statistics", "count issues", "global issue counts"}
		options.RelatedActions = []string{"issuestatistics.statistics_get_group", "issuestatistics.statistics_get_project", "issue.list_all"}
		options.IndividualTool.Description = "Get global issue count statistics across all visible projects. Returns: a statistics object with nested counts (all, opened, closed). See also: gitlab_get_group_issue_statistics, gitlab_get_project_issue_statistics, gitlab_issue_list."
	case "gitlab_get_group_issue_statistics":
		options.Usage = "Get aggregate issue counts (all, opened, closed) for a group and its descendant projects, optionally filtered by labels, milestone, assignee, author, dates, IIDs, or search."
		options.Aliases = []string{individualTool, "group issue statistics", "count group issues", "group issue counts"}
		options.RelatedActions = []string{"issuestatistics.statistics_get", "issuestatistics.statistics_get_project", "issue.list_group"}
		options.IndividualTool.Description = "Get issue count statistics for a group and its descendant projects. Returns: a statistics object with nested counts (all, opened, closed). See also: gitlab_get_issue_statistics, gitlab_get_project_issue_statistics, gitlab_group_get."
	case "gitlab_get_project_issue_statistics":
		options.Usage = "Get aggregate issue counts (all, opened, closed) for a single project, optionally filtered by labels, milestone, assignee, author, dates, IIDs, or search."
		options.Aliases = []string{individualTool, "project issue statistics", "count project issues", "project issue counts"}
		options.RelatedActions = []string{"issuestatistics.statistics_get", "issuestatistics.statistics_get_group", "issue.list"}
		options.IndividualTool.Description = "Get issue count statistics for a single project. Returns: a statistics object with nested counts (all, opened, closed). See also: gitlab_get_issue_statistics, gitlab_get_group_issue_statistics, gitlab_project_get."
	}
}
