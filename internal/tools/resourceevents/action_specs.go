package resourceevents

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// IssueActionSpecs returns canonical specs for issue resource event actions.
func IssueActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		issueEventReadSpec("event_issue_label_list", toolutil.RouteAction(client, ListIssueLabelEvents), "gitlab_issue_label_event_list"),
		issueEventReadSpec("event_issue_label_get", toolutil.RouteAction(client, GetIssueLabelEvent), "gitlab_issue_label_event_get"),
		issueEventReadSpec("event_issue_milestone_list", toolutil.RouteAction(client, ListIssueMilestoneEvents), "gitlab_issue_milestone_event_list"),
		issueEventReadSpec("event_issue_milestone_get", toolutil.RouteAction(client, GetIssueMilestoneEvent), "gitlab_issue_milestone_event_get"),
		issueEventReadSpec("event_issue_state_list", toolutil.RouteAction(client, ListIssueStateEvents), "gitlab_issue_state_event_list"),
		issueEventReadSpec("event_issue_state_get", toolutil.RouteAction(client, GetIssueStateEvent), "gitlab_issue_state_event_get"),
		issueEventReadSpec("event_issue_iteration_list", toolutil.RouteAction(client, ListIssueIterationEvents), "gitlab_issue_iteration_event_list"),
		issueEventReadSpec("event_issue_iteration_get", toolutil.RouteAction(client, GetIssueIterationEvent), "gitlab_issue_iteration_event_get"),
		issueEventReadSpec("event_issue_weight_list", toolutil.RouteAction(client, ListIssueWeightEvents), "gitlab_issue_weight_event_list"),
	}
}

func issueEventReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, issueEventOptions(individualTool))
}

func issueEventOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute resourceevents domain action.", Tags: []string{"issue", "resource_event"},
		OpenWorld:      true,
		OwnerPackage:   "resourceevents",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

// MergeRequestActionSpecs returns canonical specs for merge request resource event actions.
func MergeRequestActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		mergeRequestEventReadSpec("event_mr_label_list", toolutil.RouteAction(client, ListMRLabelEvents), "gitlab_mr_label_event_list"),
		mergeRequestEventReadSpec("event_mr_label_get", toolutil.RouteAction(client, GetMRLabelEvent), "gitlab_mr_label_event_get"),
		mergeRequestEventReadSpec("event_mr_milestone_list", toolutil.RouteAction(client, ListMRMilestoneEvents), "gitlab_mr_milestone_event_list"),
		mergeRequestEventReadSpec("event_mr_milestone_get", toolutil.RouteAction(client, GetMRMilestoneEvent), "gitlab_mr_milestone_event_get"),
		mergeRequestEventReadSpec("event_mr_state_list", toolutil.RouteAction(client, ListMRStateEvents), "gitlab_mr_state_event_list"),
		mergeRequestEventReadSpec("event_mr_state_get", toolutil.RouteAction(client, GetMRStateEvent), "gitlab_mr_state_event_get"),
	}
}

func mergeRequestEventReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, mergeRequestEventOptions(individualTool))
}

func mergeRequestEventOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute resourceevents domain action.", Tags: []string{"merge_request", "resource_event"},
		OpenWorld:      true,
		OwnerPackage:   "resourceevents",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
