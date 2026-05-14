package issues

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for issue lifecycle actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		issueCreateSpec("create", toolutil.RouteAction(client, Create), "gitlab_issue_create"),
		issueReadSpec("get", toolutil.RouteAction(client, Get), "gitlab_issue_get"),
		issueReadSpec("get_by_id", toolutil.RouteAction(client, GetByID), "gitlab_issue_get_by_id"),
		issueReadSpec("list", toolutil.RouteAction(client, List), "gitlab_issue_list"),
		issueReadSpec("list_all", toolutil.RouteAction(client, ListAll), "gitlab_issue_list_all"),
		issueReadSpec("list_group", toolutil.RouteAction(client, ListGroup), "gitlab_issue_list_group"),
		issueUpdateSpec("update", toolutil.RouteAction(client, Update), "gitlab_issue_update"),
		issueDeleteSpec("delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_issue_delete"),
		issueUpdateSpec("reorder", toolutil.RouteAction(client, Reorder), "gitlab_issue_reorder"),
		issueCreateSpec("move", toolutil.RouteAction(client, Move), "gitlab_issue_move"),
		issueUpdateSpec("subscribe", toolutil.RouteAction(client, Subscribe), "gitlab_issue_subscribe"),
		issueUpdateSpec("unsubscribe", toolutil.RouteAction(client, Unsubscribe), "gitlab_issue_unsubscribe"),
		issueCreateSpec("create_todo", toolutil.RouteAction(client, CreateTodo), "gitlab_issue_create_todo"),
		issueUpdateSpec("time_estimate_set", toolutil.RouteAction(client, SetTimeEstimate), "gitlab_issue_time_estimate_set"),
		issueUpdateSpec("time_estimate_reset", toolutil.RouteAction(client, ResetTimeEstimate), "gitlab_issue_time_estimate_reset"),
		issueCreateSpec("spent_time_add", toolutil.RouteAction(client, AddSpentTime), "gitlab_issue_spent_time_add"),
		issueUpdateSpec("spent_time_reset", toolutil.RouteAction(client, ResetSpentTime), "gitlab_issue_spent_time_reset"),
		issueReadSpec("time_stats_get", toolutil.RouteAction(client, GetTimeStats), "gitlab_issue_time_stats_get"),
		issueReadSpec("participants", toolutil.RouteAction(client, GetParticipants), "gitlab_issue_participants"),
		issueReadSpec("mrs_closing", toolutil.RouteAction(client, ListMRsClosing), "gitlab_issue_mrs_closing"),
		issueReadSpec("mrs_related", toolutil.RouteAction(client, ListMRsRelated), "gitlab_issue_mrs_related"),
	}
}

func issueReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := issueOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func issueCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, issueOptions(individualTool))
}

func issueUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := issueOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func issueDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := issueOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func issueOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"issue"},
		OpenWorld:      true,
		OwnerPackage:   "issues",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
