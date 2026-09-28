package todos

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical catalog IDs for the three to-do actions, read both by the
// RelatedActions metadata below and by the HintAction calls in markdown.go.
// These specs are aggregated into the gitlab_user catalog group, so the domain
// is "user" and the action carries the "todo_" prefix; a "todo." spelling
// names no action and a model following one is answered "unknown action".
// TestTodoActionSpecs_PublishedActionIDs_NameCatalogActions holds them against
// the catalog.
const (
	actionList        = "user.todo_list"
	actionMarkDone    = "user.todo_mark_done"
	actionMarkAllDone = "user.todo_mark_all_done"
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
		related:     []string{actionMarkDone, actionMarkAllDone},
		description: "List the authenticated user's to-do items with optional filtering and pagination. Returns: to-do items with action, target object, project, author, state, and pagination metadata. See also: gitlab_todo_mark_done, gitlab_todo_mark_all_done.",
	},
	"gitlab_todo_mark_done": {
		usage:       "Mark a single pending to-do item as done by its ID. Find the ID with user.todo_list first.",
		aliases:     []string{"mark todo done", "complete todo", "dismiss todo"},
		related:     []string{actionList, actionMarkAllDone},
		description: "Mark a single to-do item as done. Returns: a confirmation naming the to-do item ID. See also: gitlab_todo_list, gitlab_todo_mark_all_done.",
	},
	"gitlab_todo_mark_all_done": {
		usage:       "Mark every pending to-do item for the authenticated user as done in one call.",
		aliases:     []string{"mark all todos done", "clear all todos", "dismiss all todos"},
		related:     []string{actionList, actionMarkDone},
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
	// Copied without a guard: userTodoOptions starts every spec with no
	// related actions, so an entry that names none leaves the field as it
	// was either way, and a guard here was a branch no answer could tell
	// apart from its absence.
	options.RelatedActions = append([]string(nil), meta.related...)
	if meta.description != "" {
		options.IndividualTool.Description = meta.description
	}
	if individualTool == "gitlab_todo_list" {
		// Fixed filter vocabularies of GET /todos, which declares its action
		// values as Todo.action_names and its types as TodosFinder.todo_types
		// (lib/api/todos.rb), each the CE set merged with the EE one
		// (app/models/todo.rb, ee/app/models/ee/todo.rb and the two finders).
		// Both hold more than client-go's TodoAction and TodoTargetType
		// constants, and the handler forwards the strings verbatim, so
		// GitLab's own set is the enum. transfer_failed is the item a
		// background transfer leaves when it fails, which project.transfer and
		// group.transfer send a model here to find.
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaEnumOverride("action",
				"assigned", "review_requested", "mentioned", "build_failed", "marked", "approval_required",
				"unmergeable", "directly_addressed", "member_access_requested", "review_submitted",
				"ssh_key_expired", "ssh_key_expiring_soon", "transfer_failed",
				"merge_train_removed", "okr_checkin_requested", "added_approver", "duo_pro_access_granted",
				"duo_enterprise_access_granted", "duo_core_access_granted", "duo_workflow_input_required"),
			toolutil.SchemaEnumOverride("state", "pending", "done"),
			toolutil.SchemaEnumOverride("type",
				"Commit", "Issue", "WorkItem", "MergeRequest", "DesignManagement::Design", "AlertManagement::Alert",
				"Namespace", "Project", "Key", "WikiPage::Meta",
				"Epic", "Vulnerability", "User", "ComplianceManagement::Projects::ComplianceViolation", "Ai::DuoWorkflows::Workflow"),
		}
	}
}
