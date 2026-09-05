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
	options := userTodoOptions(individualTool)
	decorateTodoMeta(&options, individualTool)
	return toolutil.NewReadActionSpec(name, route, options)
}

// userTodoUpdateSpec builds the canonical update spec for a to-do tool.
func userTodoUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := userTodoOptions(individualTool)
	decorateTodoMeta(&options, individualTool)
	return toolutil.NewUpdateActionSpec(name, route, options)
}

func userTodoOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute todos domain action.", Tags: []string{"user", "todo"},
		OpenWorld:      true,
		OwnerPackage:   "todos",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

// todoActionMetaEntry is the discovery metadata for one to-do action.
type todoActionMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	description string
}

// todoActionMeta maps each individual to-do tool to its discovery metadata so
// no canonical action falls back to the generic placeholder usage/description
// (1:1 audit R-META).
var todoActionMeta = map[string]todoActionMetaEntry{
	"gitlab_todo_list": {
		usage:       "List the authenticated user's to-do items. Use filters such as action, state, project_id, group_id, author_id, type, order_by, sort, and pagination to narrow pending or done items.",
		aliases:     []string{"list todos", "show my to-do items", "list pending todos", "my todo list"},
		related:     []string{"todo.mark_done", "todo.mark_all_done"},
		description: "List the authenticated user's to-do items with optional filtering and pagination. Returns: to-do items with action, target object, project, author, state, and pagination metadata. See also: gitlab_todo_mark_done, gitlab_todo_mark_all_done.",
	},
	"gitlab_todo_mark_done": {
		usage:       "Mark a single pending to-do item as done by its ID. Find the ID with gitlab_todo_list first.",
		aliases:     []string{"mark todo done", "complete todo", "dismiss todo"},
		related:     []string{"todo.list", "todo.mark_all_done"},
		description: "Mark a single to-do item as done. Returns: a confirmation naming the to-do item ID. See also: gitlab_todo_list, gitlab_todo_mark_all_done.",
	},
	"gitlab_todo_mark_all_done": {
		usage:       "Mark every pending to-do item for the authenticated user as done in one call.",
		aliases:     []string{"mark all todos done", "clear all todos", "dismiss all todos"},
		related:     []string{"todo.list", "todo.mark_done"},
		description: "Mark all pending to-do items as done. Returns: a confirmation that all items were cleared. See also: gitlab_todo_list, gitlab_todo_mark_done.",
	},
}

// decorateTodoMeta fills non-generic Usage, natural-language Aliases,
// RelatedActions, and the "Returns: … See also: …" individual-tool description
// for a to-do action, replacing the generic placeholder metadata.
func decorateTodoMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	meta, ok := todoActionMeta[individualTool]
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
	if individualTool == "gitlab_todo_list" {
		// Fixed filter vocabularies from https://docs.gitlab.com/api/todos/
		// (GET /todos). The docs list more actions and target types than
		// client-go's TodoAction/TodoTargetType constants, and the handler
		// forwards the strings verbatim, so the documented set is the enum.
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaEnumOverride("action", "assigned", "mentioned", "build_failed", "marked", "approval_required", "unmergeable", "directly_addressed", "merge_train_removed", "member_access_requested"),
			toolutil.SchemaEnumOverride("state", "pending", "done"),
			toolutil.SchemaEnumOverride("type", "Issue", "MergeRequest", "Commit", "Epic", "DesignManagement::Design", "AlertManagement::Alert", "Project", "Namespace", "Vulnerability", "WikiPage::Meta"),
		}
	}
}
