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
	options := issueEventOptions(individualTool)
	decorateEventMeta(&options, individualTool)
	return toolutil.NewReadActionSpec(name, route, options)
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
	options := mergeRequestEventOptions(individualTool)
	decorateEventMeta(&options, individualTool)
	return toolutil.NewReadActionSpec(name, route, options)
}

func mergeRequestEventOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute resourceevents domain action.", Tags: []string{"merge_request", "resource_event"},
		OpenWorld:      true,
		OwnerPackage:   "resourceevents",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

// eventActionMetaEntry is the discovery metadata for one resource-event action.
type eventActionMetaEntry struct {
	usage       string
	aliases     []string
	description string
}

// decorateEventMeta fills non-generic Usage, natural-language Aliases, and the
// "Returns: … See also: …" individual-tool description for a resource-event
// action from the shared eventActionMeta table. It is shared (DRY) across the
// parallel issue and merge-request event variants. It is a no-op for tools with
// no table entry.
func decorateEventMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	meta, ok := eventActionMeta[individualTool]
	if !ok {
		return
	}
	if meta.usage != "" {
		options.Usage = meta.usage
	}
	if len(meta.aliases) > 0 {
		options.Aliases = append([]string(nil), meta.aliases...)
	}
	if meta.description != "" {
		options.IndividualTool.Description = meta.description
	}
}

// eventActionMeta maps each individual resource-event tool to its discovery
// metadata (1:1 audit R-META). Resource-event endpoints are read-only audit
// trails of label, milestone, state, iteration, and weight changes on issues
// and merge requests.
var eventActionMeta = map[string]eventActionMetaEntry{
	"gitlab_issue_label_event_list": {
		usage:       "List the label-change audit events for one issue (when each label was added or removed and by whom).",
		aliases:     []string{"list issue label events", "issue label history", "label changes on issue"},
		description: "List label events for an issue. Returns: label events with action, the label object, the acting user, and pagination metadata. See also: gitlab_issue_label_event_get, gitlab_issue_get.",
	},
	"gitlab_issue_label_event_get": {
		usage:       "Fetch one issue label-change event by id (a single add/remove of a label).",
		aliases:     []string{"get issue label event", "show issue label change"},
		description: "Get a single label event for an issue. Returns: the event with action, the label object, and the acting user. See also: gitlab_issue_label_event_list.",
	},
	"gitlab_mr_label_event_list": {
		usage:       "List the label-change audit events for one merge request (when each label was added or removed and by whom).",
		aliases:     []string{"list mr label events", "merge request label history", "label changes on mr"},
		description: "List label events for a merge request. Returns: label events with action, the label object, the acting user, and pagination metadata. See also: gitlab_mr_label_event_get, gitlab_mr_get.",
	},
	"gitlab_mr_label_event_get": {
		usage:       "Fetch one merge request label-change event by id (a single add/remove of a label).",
		aliases:     []string{"get mr label event", "show merge request label change"},
		description: "Get a single label event for a merge request. Returns: the event with action, the label object, and the acting user. See also: gitlab_mr_label_event_list.",
	},
	"gitlab_issue_milestone_event_list": {
		usage:       "List the milestone-change audit events for one issue (when the milestone was assigned or removed and by whom).",
		aliases:     []string{"list issue milestone events", "issue milestone history", "milestone changes on issue"},
		description: "List milestone events for an issue. Returns: milestone events with action, the milestone object, the acting user, and pagination metadata. See also: gitlab_issue_milestone_event_get, gitlab_issue_get.",
	},
	"gitlab_issue_milestone_event_get": {
		usage:       "Fetch one issue milestone-change event by id (a single milestone assignment or removal).",
		aliases:     []string{"get issue milestone event", "show issue milestone change"},
		description: "Get a single milestone event for an issue. Returns: the event with action, the milestone object, and the acting user. See also: gitlab_issue_milestone_event_list.",
	},
	"gitlab_mr_milestone_event_list": {
		usage:       "List the milestone-change audit events for one merge request (when the milestone was assigned or removed and by whom).",
		aliases:     []string{"list mr milestone events", "merge request milestone history", "milestone changes on mr"},
		description: "List milestone events for a merge request. Returns: milestone events with action, the milestone object, the acting user, and pagination metadata. See also: gitlab_mr_milestone_event_get, gitlab_mr_get.",
	},
	"gitlab_mr_milestone_event_get": {
		usage:       "Fetch one merge request milestone-change event by id (a single milestone assignment or removal).",
		aliases:     []string{"get mr milestone event", "show merge request milestone change"},
		description: "Get a single milestone event for a merge request. Returns: the event with action, the milestone object, and the acting user. See also: gitlab_mr_milestone_event_list.",
	},
	"gitlab_issue_state_event_list": {
		usage:       "List the state-change audit events for one issue (close and reopen transitions and who made them).",
		aliases:     []string{"list issue state events", "issue state history", "issue close reopen history"},
		description: "List state events for an issue. Returns: state events with the state value, the acting user, and pagination metadata. See also: gitlab_issue_state_event_get, gitlab_issue_get.",
	},
	"gitlab_issue_state_event_get": {
		usage:       "Fetch one issue state-change event by id (a single close or reopen transition).",
		aliases:     []string{"get issue state event", "show issue state change"},
		description: "Get a single state event for an issue. Returns: the event with the state value and the acting user. See also: gitlab_issue_state_event_list.",
	},
	"gitlab_mr_state_event_list": {
		usage:       "List the state-change audit events for one merge request (close, reopen, and merge transitions and who made them).",
		aliases:     []string{"list mr state events", "merge request state history", "mr close reopen merge history"},
		description: "List state events for a merge request. Returns: state events with the state value, the acting user, and pagination metadata. See also: gitlab_mr_state_event_get, gitlab_mr_get.",
	},
	"gitlab_mr_state_event_get": {
		usage:       "Fetch one merge request state-change event by id (a single close, reopen, or merge transition).",
		aliases:     []string{"get mr state event", "show merge request state change"},
		description: "Get a single state event for a merge request. Returns: the event with the state value and the acting user. See also: gitlab_mr_state_event_list.",
	},
	"gitlab_issue_iteration_event_list": {
		usage:       "List the iteration-change audit events for one issue (when the iteration was assigned or removed and by whom). Requires GitLab Premium/Ultimate.",
		aliases:     []string{"list issue iteration events", "issue iteration history", "iteration changes on issue"},
		description: "List iteration events for an issue (Premium/Ultimate). Returns: iteration events with action, the iteration object, the acting user, and pagination metadata. See also: gitlab_issue_iteration_event_get, gitlab_issue_get.",
	},
	"gitlab_issue_iteration_event_get": {
		usage:       "Fetch one issue iteration-change event by id (a single iteration assignment or removal). Requires GitLab Premium/Ultimate.",
		aliases:     []string{"get issue iteration event", "show issue iteration change"},
		description: "Get a single iteration event for an issue (Premium/Ultimate). Returns: the event with action, the iteration object, and the acting user. See also: gitlab_issue_iteration_event_list.",
	},
	"gitlab_issue_weight_event_list": {
		usage:       "List the weight-change audit events for one issue (each weight value set and by whom). Requires GitLab Premium/Ultimate.",
		aliases:     []string{"list issue weight events", "issue weight history", "weight changes on issue"},
		description: "List weight events for an issue (Premium/Ultimate). Returns: weight events with the weight value, the acting user, and pagination metadata. See also: gitlab_issue_get.",
	},
}
