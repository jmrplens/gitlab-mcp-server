package issuestatistics

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// catalogDomain is the domain half of every canonical ID these actions are
// registered under. These specs are aggregated into the gitlab_issue catalog
// group, so the domain is issue and never the owner package name: an ID built
// as OwnerPackage + "." + Name resolves to nothing on any surface.
const catalogDomain = "issue"

// Canonical catalog IDs for the issue statistics actions, and the one block
// the related-action metadata below and markdown.go's result hint both read.
const (
	actionStatisticsGet        = catalogDomain + ".statistics_get"
	actionStatisticsGetGroup   = catalogDomain + ".statistics_get_group"
	actionStatisticsGetProject = catalogDomain + ".statistics_get_project"
	// actionIssueList is owned by the issues package rather than this one, and
	// is declared here so every ID this package publishes has one home.
	actionIssueList = catalogDomain + ".list"
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
// actions, a "Returns: … See also: …" individual-tool description, and
// JSON Schema enum constraints for fixed-vocabulary filter parameters.
func decorateIssueStatisticsMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	switch individualTool {
	case "gitlab_get_issue_statistics":
		options.Usage = "Get aggregate issue counts (all, opened, closed) across every project visible to the authenticated user, optionally filtered by labels, milestone, assignee, author, dates, or search."
		options.Aliases = []string{individualTool, "issue statistics", "count issues", "global issue counts"}
		options.RelatedActions = []string{actionStatisticsGetGroup, actionStatisticsGetProject, "issue.list_all"}
		options.IndividualTool.Description = "Get global issue count statistics across all visible projects. Returns: a statistics object with nested counts (all, opened, closed). See also: gitlab_get_group_issue_statistics, gitlab_get_project_issue_statistics, gitlab_issue_list."
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaEnumOverride("scope", "created_by_me", "assigned_to_me", "all"),
			toolutil.SchemaEnumOverride("in", "title", "description", "title,description"),
		}
	case "gitlab_get_group_issue_statistics":
		options.Usage = "Get aggregate issue counts (all, opened, closed) for a group and its descendant projects, optionally filtered by labels, milestone, assignee, author, dates, IIDs, or search."
		options.Aliases = []string{individualTool, "group issue statistics", "count group issues", "group issue counts"}
		options.RelatedActions = []string{actionStatisticsGet, actionStatisticsGetProject, "issue.list_group"}
		options.IndividualTool.Description = "Get issue count statistics for a group and its descendant projects. Returns: a statistics object with nested counts (all, opened, closed). See also: gitlab_get_issue_statistics, gitlab_get_project_issue_statistics, gitlab_group_get."
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaEnumOverride("scope", "created_by_me", "assigned_to_me", "all"),
		}
	case "gitlab_get_project_issue_statistics":
		options.Usage = "Get aggregate issue counts (all, opened, closed) for a single project, optionally filtered by labels, milestone, assignee, author, dates, IIDs, or search."
		options.Aliases = []string{individualTool, "project issue statistics", "count project issues", "project issue counts"}
		options.RelatedActions = []string{actionStatisticsGet, actionStatisticsGetGroup, actionIssueList}
		options.IndividualTool.Description = "Get issue count statistics for a single project. Returns: a statistics object with nested counts (all, opened, closed). See also: gitlab_get_issue_statistics, gitlab_get_group_issue_statistics, gitlab_project_get."
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaEnumOverride("scope", "created_by_me", "assigned_to_me", "all"),
		}
	}
}
