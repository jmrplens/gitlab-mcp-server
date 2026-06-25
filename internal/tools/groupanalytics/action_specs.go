package groupanalytics

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for group analytics actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupAnalyticsReadSpec("analytics_issues_count", toolutil.RouteAction(client, GetIssuesCount), "gitlab_get_recently_created_issues_count"),
		groupAnalyticsReadSpec("analytics_mr_count", toolutil.RouteAction(client, GetMRCount), "gitlab_get_recently_created_mr_count"),
		groupAnalyticsReadSpec("analytics_members_count", toolutil.RouteAction(client, GetMembersCount), "gitlab_get_recently_added_members_count"),
	}
}

// groupAnalyticsReadSpec builds a read-only [toolutil.ActionSpec] for a group
// analytics action and decorates it with non-generic discovery metadata
// (action-specific Usage, natural-language Aliases, canonical RelatedActions,
// and a "Returns: … See also: …" individual-tool description) drawn from
// [groupAnalyticsActionMeta] (1:1 audit R-META).
func groupAnalyticsReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupAnalyticsOptions(individualTool)
	decorateGroupAnalyticsMeta(&options, individualTool)
	return toolutil.NewReadActionSpec(name, route, options)
}

func groupAnalyticsOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute groupanalytics domain action.", Tags: []string{"group", "analytics"},
		RelatedActions: []string{"group.get"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "groupanalytics",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

// decorateGroupAnalyticsMeta fills non-generic Usage, natural-language Aliases,
// canonical RelatedActions, and the "Returns: … See also: …" individual-tool
// description for a group analytics action that would otherwise inherit the
// generic placeholder metadata from [groupAnalyticsOptions]. It is a no-op for
// tools with no entry in [groupAnalyticsActionMeta].
func decorateGroupAnalyticsMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	meta, ok := groupAnalyticsActionMeta[individualTool]
	if !ok {
		return
	}
	if meta.usage != "" {
		options.Usage = meta.usage
	}
	if len(meta.aliases) > 0 {
		options.Aliases = append([]string(nil), meta.aliases...)
	}
	if len(meta.related) > 0 {
		options.RelatedActions = append([]string(nil), meta.related...)
	}
	if meta.description != "" {
		options.IndividualTool.Description = meta.description
	}
}

// groupAnalyticsActionMetaEntry is the discovery metadata for one group
// analytics action.
type groupAnalyticsActionMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	description string
}

// groupAnalyticsActionMeta maps each individual group analytics tool to its
// discovery metadata. Aliases use group-activity analytics phrasing and
// RelatedActions point at canonical group.* actions so discovery and the
// individual-tool surface mirror the GitLab Group Activity Analytics API 1:1.
var groupAnalyticsActionMeta = map[string]groupAnalyticsActionMetaEntry{
	"gitlab_get_recently_created_issues_count": {
		usage:       "Report how many issues were recently created across a group and its subgroups. Use for group activity dashboards or recent-activity summaries scoped to a group path; this returns a count, not the issue list.",
		aliases:     []string{"recently created issues count", "group new issues count", "group issue activity count", "count recent issues in group"},
		related:     []string{"groupanalytics.analytics_mr_count", "groupanalytics.analytics_members_count", "group.get"},
		description: "Get the count of recently created issues across a group. Returns: the group path and recently created issues count (Premium). See also: gitlab_get_recently_created_mr_count, gitlab_get_recently_added_members_count, gitlab_group_get.",
	},
	"gitlab_get_recently_created_mr_count": {
		usage:       "Report how many merge requests were recently created across a group and its subgroups. Use for group activity dashboards or recent-activity summaries scoped to a group path; this returns a count, not the merge request list.",
		aliases:     []string{"recently created merge requests count", "group new MR count", "group merge request activity count", "count recent merge requests in group"},
		related:     []string{"groupanalytics.analytics_issues_count", "groupanalytics.analytics_members_count", "group.get"},
		description: "Get the count of recently created merge requests across a group. Returns: the group path and recently created merge requests count (Premium). See also: gitlab_get_recently_created_issues_count, gitlab_get_recently_added_members_count, gitlab_group_get.",
	},
	"gitlab_get_recently_added_members_count": {
		usage:       "Report how many members were recently added across a group and its subgroups. Use for group activity dashboards or onboarding-trend summaries scoped to a group path; this returns a count, not the member list.",
		aliases:     []string{"recently added members count", "group new members count", "group membership activity count", "count recent members added to group"},
		related:     []string{"groupanalytics.analytics_issues_count", "groupanalytics.analytics_mr_count", "group.get"},
		description: "Get the count of recently added members across a group. Returns: the group path and recently added members count (Premium). See also: gitlab_get_recently_created_issues_count, gitlab_get_recently_created_mr_count, gitlab_group_get.",
	},
}
