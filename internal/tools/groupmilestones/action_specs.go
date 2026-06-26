package groupmilestones

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionGroupMilestoneGet    = "group_milestone.get"
	actionGroupMilestoneList   = "group_milestone.list"
	actionGroupMilestoneUpdate = "group_milestone.update"
	actionGroupMilestoneIssues = "group_milestone.issues"
)

// ActionSpecs returns canonical specs for group milestone actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupMilestoneReadSpec("group_milestone_list", toolutil.RouteAction(client, List), "gitlab_group_milestone_list"),
		groupMilestoneReadSpec("group_milestone_get", toolutil.RouteAction(client, Get), "gitlab_group_milestone_get"),
		groupMilestoneCreateSpec("group_milestone_create", toolutil.RouteAction(client, Create), "gitlab_group_milestone_create"),
		groupMilestoneUpdateSpec("group_milestone_update", toolutil.RouteAction(client, Update), "gitlab_group_milestone_update"),
		groupMilestoneDeleteSpec("group_milestone_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_group_milestone_delete"),
		groupMilestoneReadSpec("group_milestone_issues", toolutil.RouteAction(client, GetIssues), "gitlab_group_milestone_issues"),
		groupMilestoneReadSpec("group_milestone_merge_requests", toolutil.RouteAction(client, GetMergeRequests), "gitlab_group_milestone_merge_requests"),
		groupMilestoneBurndownSpec("group_milestone_burndown", toolutil.RouteAction(client, GetBurndownChartEvents), "gitlab_group_milestone_burndown_events"),
	}
}

func groupMilestoneReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupMilestoneOptions(individualTool)
	decorateGroupMilestoneMeta(&options, individualTool)
	return toolutil.NewReadActionSpec(name, route, options)
}

// groupMilestoneBurndownSpec builds the burndown-chart-events read spec. The
// burndown_events endpoint is Premium/Ultimate (group_milestones.md "Tier:
// Premium, Ultimate"), so the whole action is tier-gated.
func groupMilestoneBurndownSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupMilestoneOptions(individualTool)
	decorateGroupMilestoneMeta(&options, individualTool)
	options.Edition = "premium"
	return toolutil.NewReadActionSpec(name, route, options)
}

func groupMilestoneCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupMilestoneOptions(individualTool)
	decorateGroupMilestoneMeta(&options, individualTool)
	return toolutil.NewCreateActionSpec(name, route, options)
}

func groupMilestoneUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupMilestoneOptions(individualTool)
	decorateGroupMilestoneMeta(&options, individualTool)
	return toolutil.NewUpdateActionSpec(name, route, options)
}

func groupMilestoneDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupMilestoneOptions(individualTool)
	decorateGroupMilestoneMeta(&options, individualTool)
	return toolutil.NewDeleteActionSpec(name, route, options)
}

func groupMilestoneOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute groupmilestones domain action.", Tags: []string{"group", "milestone"},
		RelatedActions: []string{"group.get", "group.issues"},
		OpenWorld:      true,
		OwnerPackage:   "groupmilestones",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

// decorateGroupMilestoneMeta fills non-generic Usage, natural-language Aliases,
// RelatedActions, and the "Returns: … See also: …" individual-tool description
// for each group milestone action so none inherits the generic placeholder
// metadata from groupMilestoneOptions (1:1 audit R-META).
func decorateGroupMilestoneMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	meta, ok := groupMilestoneActionMeta[individualTool]
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

// groupMilestoneActionMetaEntry is the discovery metadata for one group
// milestone action.
type groupMilestoneActionMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	description string
}

// groupMilestoneActionMeta maps each individual group milestone tool to its
// discovery metadata.
var groupMilestoneActionMeta = map[string]groupMilestoneActionMetaEntry{
	"gitlab_group_milestone_list": {
		usage:       "List milestones in a group with filtering, ordering, and pagination. Use filters such as state, title, search, include_ancestors, include_descendants, order_by, and sort when the prompt asks for matching group milestones.",
		aliases:     []string{"list group milestones", "show group milestones", "find milestones in group"},
		related:     []string{actionGroupMilestoneGet, "group_milestone.create", "milestone.list"},
		description: "List milestones in a group with filtering and pagination. Returns: matching group milestones with state, dates, expiry, and pagination metadata. See also: gitlab_group_milestone_get, gitlab_group_milestone_create, gitlab_milestone_list.",
	},
	"gitlab_group_milestone_get": {
		usage:       "Get one exact group milestone by group_id plus milestone_iid. Use after list results or when the prompt already names a concrete group milestone IID.",
		aliases:     []string{"get group milestone", "show group milestone details", "fetch group milestone"},
		related:     []string{actionGroupMilestoneList, actionGroupMilestoneUpdate, actionGroupMilestoneIssues},
		description: "Get a single group milestone by IID. Returns: milestone metadata, state, start and due dates, expiry, and timestamps. See also: gitlab_group_milestone_list, gitlab_group_milestone_update, gitlab_group_milestone_issues.",
	},
	"gitlab_group_milestone_create": {
		usage:       "Create a new milestone in a group. Provide title and optional description, start_date, and due_date.",
		aliases:     []string{"create group milestone", "add group milestone", "new group milestone"},
		related:     []string{actionGroupMilestoneGet, actionGroupMilestoneList, actionGroupMilestoneUpdate},
		description: "Create a new group milestone. Returns: the created milestone with ID, IID, state, start and due dates. See also: gitlab_group_milestone_get, gitlab_group_milestone_list, gitlab_group_milestone_update.",
	},
	"gitlab_group_milestone_update": {
		usage:       "Update an existing group milestone. Only non-empty fields are applied; use state_event to close or activate.",
		aliases:     []string{"update group milestone", "edit group milestone", "close group milestone", "activate group milestone"},
		related:     []string{actionGroupMilestoneGet, actionGroupMilestoneList, "group_milestone.delete"},
		description: "Update an existing group milestone. Returns: the updated milestone with state, dates, and expiry. See also: gitlab_group_milestone_get, gitlab_group_milestone_list, gitlab_group_milestone_delete.",
	},
	"gitlab_group_milestone_delete": {
		usage:       "Permanently delete a group milestone. Destructive and irreversible; confirm group_id and milestone_iid before calling. Requires Owner role.",
		aliases:     []string{"delete group milestone", "remove group milestone"},
		related:     []string{actionGroupMilestoneGet, actionGroupMilestoneList, actionGroupMilestoneUpdate},
		description: "Delete a group milestone permanently. Returns: a success confirmation. See also: gitlab_group_milestone_get, gitlab_group_milestone_update.",
	},
	"gitlab_group_milestone_issues": {
		usage:       "List issues assigned to a group milestone, with ordering and pagination. Use to inspect the issue scope of a group milestone.",
		aliases:     []string{"group milestone issues", "issues in group milestone", "list group milestone issues"},
		related:     []string{actionGroupMilestoneGet, "group_milestone.merge_requests", "issue.list_group"},
		description: "List issues assigned to a group milestone. Returns: assigned issues with state, web URL, and pagination metadata. See also: gitlab_group_milestone_get, gitlab_group_milestone_merge_requests, gitlab_issue_list_group.",
	},
	"gitlab_group_milestone_merge_requests": {
		usage:       "List merge requests assigned to a group milestone, with ordering and pagination. Use to inspect the MR scope of a group milestone.",
		aliases:     []string{"group milestone merge requests", "merge requests in group milestone", "list group milestone MRs"},
		related:     []string{actionGroupMilestoneGet, actionGroupMilestoneIssues, "group_milestone.burndown"},
		description: "List merge requests assigned to a group milestone. Returns: assigned merge requests with state, source and target branches, and pagination metadata. See also: gitlab_group_milestone_get, gitlab_group_milestone_issues, gitlab_group_milestone_burndown_events.",
	},
	"gitlab_group_milestone_burndown_events": {
		usage:       "List burndown chart events for a group milestone, with ordering and pagination. Requires GitLab Premium or higher.",
		aliases:     []string{"group milestone burndown", "burndown chart events", "group milestone burndown events"},
		related:     []string{actionGroupMilestoneGet, actionGroupMilestoneIssues, "group_milestone.merge_requests"},
		description: "List burndown chart events for a group milestone. Returns: dated burndown events with weight, action, and pagination metadata. Requires GitLab Premium or higher. See also: gitlab_group_milestone_get, gitlab_group_milestone_issues, gitlab_group_milestone_merge_requests.",
	},
}
