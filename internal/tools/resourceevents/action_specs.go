package resourceevents

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

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
	options := mergeRequestEventOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func mergeRequestEventOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"merge_request", "resource_event"},
		OpenWorld:      true,
		OwnerPackage:   "resourceevents",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
