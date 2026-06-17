package todos

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for todo actions exposed through gitlab_user.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_todo_list — list pending (or filtered) to-do items for the authenticated user.
		userTodoReadSpec("todo_list", toolutil.RouteAction(client, List), "gitlab_todo_list"),
		// gitlab_todo_mark_done — mark a single to-do item as done.
		userTodoUpdateSpec("todo_mark_done", toolutil.RouteAction(client, MarkDone), "gitlab_todo_mark_done"),
		// gitlab_todo_mark_all_done — mark every pending to-do item as done.
		userTodoUpdateSpec("todo_mark_all_done", toolutil.RouteAction(client, MarkAllDone), "gitlab_todo_mark_all_done"),
	}
}

// userTodoReadSpec builds the canonical read-only spec for a to-do tool.
func userTodoReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, userTodoOptions(individualTool))
}

// userTodoUpdateSpec builds the canonical update spec for a to-do tool.
func userTodoUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, userTodoOptions(individualTool))
}

func userTodoOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute todos domain action.", Tags: []string{"user", "todo"},
		OpenWorld:      true,
		OwnerPackage:   "todos",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
