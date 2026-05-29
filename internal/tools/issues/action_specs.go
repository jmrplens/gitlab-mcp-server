package issues

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionIssueList    = "issue.list"
	actionSearchIssues = "search.issues"
	actionIssueGet     = "issue.get"
)

// ActionSpecs returns canonical specs for issue lifecycle actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		issueCreateSpec("create", toolutil.RouteAction(client, Create), "gitlab_issue_create"),
		issueReadSpec("get", toolutil.RouteAction(client, getWithEmbeddedResource), "gitlab_issue_get"),
		issueReadSpec("get_by_id", toolutil.RouteAction(client, GetByID), "gitlab_issue_get_by_id"),
		issueReadSpec("list", toolutil.RouteAction(client, List), "gitlab_issue_list"),
		issueReadSpec("list_all", toolutil.RouteAction(client, ListAll), "gitlab_issue_list_all"),
		issueReadSpec("list_group", toolutil.RouteAction(client, ListGroup), "gitlab_issue_list_group"),
		issueUpdateActionSpec(client),
		issueDeleteSpec("delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_issue_delete"),
		issueUpdateSpec("reorder", toolutil.RouteAction(client, Reorder), "gitlab_issue_reorder"),
		issueUpdateSpec("move", toolutil.RouteAction(client, Move), "gitlab_issue_move"),
		issueUpdateSpec("subscribe", toolutil.RouteAction(client, Subscribe), "gitlab_issue_subscribe"),
		issueUpdateSpec("unsubscribe", toolutil.RouteAction(client, Unsubscribe), "gitlab_issue_unsubscribe"),
		issueCreateSpec("create_todo", toolutil.RouteAction(client, CreateTodo), "gitlab_issue_create_todo"),
		issueUpdateSpec("time_estimate_set", toolutil.RouteAction(client, SetTimeEstimate), "gitlab_issue_time_estimate_set"),
		issueUpdateSpec("time_estimate_reset", toolutil.RouteAction(client, ResetTimeEstimate), "gitlab_issue_time_estimate_reset"),
		issueUpdateSpec("spent_time_add", toolutil.RouteAction(client, AddSpentTime), "gitlab_issue_spent_time_add"),
		issueUpdateSpec("spent_time_reset", toolutil.RouteAction(client, ResetSpentTime), "gitlab_issue_spent_time_reset"),
		issueReadSpec("time_stats_get", toolutil.RouteAction(client, GetTimeStats), "gitlab_issue_time_stats_get"),
		issueReadSpec("participants", toolutil.RouteAction(client, GetParticipants), "gitlab_issue_participants"),
		issueReadSpec("mrs_closing", toolutil.RouteAction(client, ListMRsClosing), "gitlab_issue_mrs_closing"),
		issueReadSpec("mrs_related", toolutil.RouteAction(client, ListMRsRelated), "gitlab_issue_mrs_related"),
	}
}

type getOutput struct {
	Output
}

func getWithEmbeddedResource(ctx context.Context, client *gitlabclient.Client, input GetInput) (getOutput, error) {
	out, err := Get(ctx, client, input)
	return getOutput{Output: out}, err
}

func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult(fmt.Sprintf("issue #%d from project %s", input.IssueIID, input.ProjectID))
	return out, nil
}

// GroupActionSpecs returns canonical specs for issue actions exposed through the group meta-tool.
func GroupActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupIssueReadSpec("issues", toolutil.RouteAction(client, ListGroup), "gitlab_issue_list_group"),
	}
}

func issueReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := issueOptions(individualTool)
	switch individualTool {
	case "gitlab_issue_get":
		options.Usage = "Get one exact issue by project_id plus issue_iid. Use this after list/search results or when the prompt already names a concrete issue number; prefer issue.get over issue.list when the target issue is already known."
		options.RelatedActions = []string{actionIssueList, "issue.update", "issue.delete", "issue.notes_list"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": {
				SemanticRole:     "scope_project",
				ValueSource:      "Project ID or full namespace path that owns the issue.",
				ExampleBinding:   `params.project_id:"group/project"`,
				CommonConfusions: []string{"Use the issue's parent project here, not a group path or global issue ID."},
			},
			"issue_iid": {
				SemanticRole:     "issue_iid",
				ValueSource:      "Issue number visible in the project, usually from the URL or prior issue list output.",
				ExampleBinding:   "params.issue_iid:42",
				CommonConfusions: []string{"Use issue_iid for project-scoped issue numbers; issue_id is only for the global issue ID action."},
			},
		}
		options.IndividualTool.Description = "Get a single issue from a project by issue IID. Returns: issue metadata, state, labels, assignees, author, due date, task completion, and web URL. See also: gitlab_issue_list, gitlab_issue_update, gitlab_issue_delete, gitlab_issue_notes_list."
	case "gitlab_issue_list":
		options.Usage = "List issues in one project. Use filters such as state, labels, search, assignee_username, milestone, order_by, sort, and pagination when the prompt asks for matching or recent issues in a known project."
		options.Aliases = []string{"list project issues", "find issues in project", "show project issues"}
		options.RelatedActions = []string{actionIssueGet, "issue.create", actionSearchIssues}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": {
				SemanticRole:     "scope_project",
				ValueSource:      "Project ID or namespace path whose issues should be listed.",
				ExampleBinding:   `params.project_id:"group/project"`,
				CommonConfusions: []string{"Use project_id for the project scope; use group_id only with issue.list_group."},
			},
			"search": {
				ValueSource:      "Keywords from the user's issue title or description request.",
				ExampleBinding:   `params.search:"oauth timeout"`,
				CommonConfusions: []string{"search narrows within the selected project; it does not replace project_id."},
			},
			"order_by": {
				SemanticRole:     "issue_list_sort_field",
				ValueSource:      "Field requested for sorting recent or oldest issues, such as created_at or updated_at.",
				ExampleBinding:   `params.order_by:"updated_at"`,
				CommonConfusions: []string{"Combine order_by with sort; do not pass natural-language phrases like newest first as the field value."},
			},
		}
		options.IndividualTool.Description = "List issues in one project with filtering and pagination. Returns: matching issues with state, labels, assignees, author, and pagination metadata. See also: gitlab_issue_get, gitlab_issue_create, gitlab_search_issues."
	case "gitlab_issue_list_all":
		options.Usage = "List issues visible to the authenticated user across all accessible projects. Use this when the user asks for their open issues, assigned issues, or a cross-project issue overview."
		options.Aliases = []string{"list all issues", "show my issues across projects", "list visible issues"}
		options.RelatedActions = []string{actionIssueList, "issue.list_group", actionSearchIssues}
		options.IndividualTool.Description = "List issues across accessible projects. Returns: visible issues with project context and pagination metadata. See also: gitlab_issue_list, gitlab_issue_list_group, gitlab_search_issues."
	case "gitlab_issue_time_stats_get":
		options.RelatedActions = []string{"issue.time_estimate_set", "issue.time_estimate_reset", "issue.spent_time_add", "issue.spent_time_reset"}
	case "gitlab_issue_participants":
		options.RelatedActions = []string{actionIssueGet, "issue.notes_list"}
	case "gitlab_issue_mrs_closing", "gitlab_issue_mrs_related":
		options.RelatedActions = []string{actionIssueGet, actionIssueList, "merge_request.get"}
	}
	return toolutil.NewReadActionSpec(name, route, options)
}

func issueCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := issueOptions(individualTool)
	if individualTool == "gitlab_issue_create" {
		options.Usage = "Create a new issue in a known project. Provide project_id and a clear title, then add description, labels, assignee_ids, milestone_id, due_date, confidential, or task metadata only when requested."
		options.Aliases = []string{"open issue", "create bug report", "file issue"}
		options.RelatedActions = []string{actionIssueGet, actionIssueList, "issue.update"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": {
				SemanticRole:     "scope_project",
				ValueSource:      "Project where the issue should be created.",
				ExampleBinding:   `params.project_id:"group/project"`,
				CommonConfusions: []string{"Use the target project path or numeric ID; do not substitute group_id or repository URL."},
			},
			"title": {
				SemanticRole:   "issue_title",
				ValueSource:    "Short issue summary from the user's request.",
				ExampleBinding: `params.title:"OAuth login fails after redirect"`,
			},
			"due_date": {
				SemanticRole:     "calendar_date",
				ValueSource:      "Requested due date in ISO format when the user specifies one.",
				ExampleBinding:   `params.due_date:"2026-06-01"`,
				CommonConfusions: []string{"Use YYYY-MM-DD; natural-language dates must be normalized before calling the tool."},
			},
		}
		options.IndividualTool.Description = "Create a new issue in a project. Returns: the created issue with IID, state, labels, assignees, milestone, due date, and web URL. See also: gitlab_issue_get, gitlab_issue_list, gitlab_issue_update."
	}
	return toolutil.NewCreateActionSpec(name, route, options)
}

func issueUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, issueOptions(individualTool))
}

func issueUpdateActionSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	options := issueOptions("gitlab_issue_update")
	options.Usage = "Update issue fields. To close or reopen an issue with issue.update, set params.state_event to close or reopen; dynamic execute also accepts issue.close and issue.reopen aliases that fill state_event automatically."
	options.Aliases = []string{"close issue", "reopen issue", "change issue state", "transition issue"}
	options.RelatedActions = []string{actionIssueGet, "issue.delete", actionIssueList}
	options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		"state_event": {
			SemanticRole:     "issue_state_transition",
			ValueSource:      "task intent when closing or reopening an issue",
			CommonConfusions: []string{"Do not use state=closed/opened for transitions; use state_event=close or state_event=reopen."},
			ExampleBinding:   `{"state_event":"close"}`,
		},
	}
	options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
		{
			PropertyPath: "state_event",
			Values: map[string]any{
				"enum":        []any{"close", "reopen"},
				"description": "State transition; set to close or reopen when changing issue state.",
			},
		},
	}
	return toolutil.NewUpdateActionSpec("update", toolutil.RouteAction(client, Update), options)
}

func issueDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, issueOptions(individualTool))
}

func issueOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute issues domain action.", Tags: []string{"issue"},
		OpenWorld:      true,
		OwnerPackage:   "issues",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

func groupIssueReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupIssueOptions(individualTool)
	if individualTool == "gitlab_issue_list_group" {
		options.Usage = "List issues across a group and its projects. Use this when the prompt scopes work to a group or subgroup rather than a single project."
		options.Aliases = []string{"list group issues", "show issues in group", "find issues across group"}
		options.RelatedActions = []string{actionIssueList, "group.get", actionSearchIssues}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"group_id": {
				SemanticRole:     "scope_group",
				ValueSource:      "Group ID or full group path from the user's request.",
				ExampleBinding:   `params.group_id:"platform/backend"`,
				CommonConfusions: []string{"Use group_id for group paths or IDs; use project_id only with issue.list for a single project."},
			},
		}
		options.IndividualTool.Description = "List issues across a group. Returns: matching issues from projects in the group with pagination metadata. See also: gitlab_issue_list, gitlab_group_get, gitlab_search_issues."
	}
	return toolutil.NewReadActionSpec(name, route, options)
}

func groupIssueOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute issues domain action.", Tags: []string{"group", "issue"},
		OpenWorld:      true,
		OwnerPackage:   "issues",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
