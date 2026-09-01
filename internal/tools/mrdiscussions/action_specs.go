package mrdiscussions

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical related-action ids. MR discussion specs are merged into the
// gitlab_mr_review catalog group, so their canonical ids are
// mr_review.<spec_name>; cross-domain references to the merge request itself
// use the merge_request.* namespace.
const (
	actionDiscussionCreate     = "mr_review.discussion_create"
	actionDiscussionList       = "mr_review.discussion_list"
	actionDiscussionGet        = "mr_review.discussion_get"
	actionDiscussionReply      = "mr_review.discussion_reply"
	actionDiscussionResolve    = "mr_review.discussion_resolve"
	actionDiscussionNoteUpdate = "mr_review.discussion_note_update"
	actionDiscussionNoteDelete = "mr_review.discussion_note_delete"
	actionMRNoteCreate         = "mr_review.note_create"
	actionMRNoteList           = "mr_review.note_list"
	actionMRChangesGet         = "mr_review.changes_get"
	actionMRGet                = "merge_request.get"
)

// ActionSpecs returns canonical specs for merge request discussion actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		mrDiscussionCreateSpec("discussion_create", toolutil.RouteAction(client, Create), "gitlab_mr_discussion_create"),
		mrDiscussionReadSpec("discussion_list", toolutil.RouteAction(client, List), "gitlab_mr_discussion_list"),
		mrDiscussionReadSpec("discussion_get", toolutil.RouteAction(client, Get), "gitlab_mr_discussion_get"),
		mrDiscussionCreateSpec("discussion_reply", toolutil.RouteAction(client, Reply), "gitlab_mr_discussion_reply"),
		mrDiscussionUpdateSpec("discussion_resolve", toolutil.RouteAction(client, Resolve), "gitlab_mr_discussion_resolve"),
		mrDiscussionUpdateSpec("discussion_note_update", toolutil.RouteAction(client, UpdateNote), "gitlab_mr_discussion_note_update"),
		mrDiscussionDeleteSpec("discussion_note_delete", toolutil.DestructiveVoidAction(client, DeleteNote), "gitlab_mr_discussion_note_delete"),
	}
}

func mrDiscussionReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, mrDiscussionOptions(individualTool))
}

func mrDiscussionCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, mrDiscussionOptions(individualTool))
}

func mrDiscussionUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, mrDiscussionOptions(individualTool))
}

func mrDiscussionDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, mrDiscussionOptions(individualTool))
}

// mrDiscussionOptions builds the shared [toolutil.ActionSpecOptions] for an MR
// discussion action and decorates it with action-specific usage,
// natural-language aliases, related actions, parameter guidance, and an
// individual-tool description (R-META; 1:1 audit).
func mrDiscussionOptions(individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Tags: []string{"merge_request", "review", "discussion"},
		OpenWorld:      true,
		OwnerPackage:   "mrdiscussions",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	decorateMRDiscussionMeta(&options, individualTool)
	return options
}

// mrScopeGuidance returns the parameter guidance shared by every MR discussion
// action for the project_id and merge_request_iid scoping parameters.
func mrScopeGuidance() map[string]toolutil.ParameterGuidance {
	return map[string]toolutil.ParameterGuidance{
		"project_id": {
			SemanticRole:     "scope_project",
			ValueSource:      "Project ID or full namespace path that owns the merge request.",
			ExampleBinding:   `params.project_id:"group/project"`,
			CommonConfusions: []string{"Use the MR's parent project here, not a group path or global MR ID."},
		},
		"merge_request_iid": {
			SemanticRole:     "merge_request_iid",
			ValueSource:      "Merge request number visible in the project, usually from the URL or prior MR list output.",
			ExampleBinding:   "params.merge_request_iid:42",
			CommonConfusions: []string{"Use merge_request_iid for the project-scoped MR number, not the global merge_request_id."},
		},
	}
}

// discussionIDGuidance returns the parameter guidance for the discussion_id
// parameter used by discussion-scoped actions.
func discussionIDGuidance() toolutil.ParameterGuidance {
	return toolutil.DiscussionIDParamGuidance("Discussion thread id from a prior gitlab_mr_discussion_list response.")
}

// noteIDGuidance returns the parameter guidance for the note_id parameter used
// by note update/delete actions.
func noteIDGuidance() toolutil.ParameterGuidance {
	return toolutil.DiscussionNoteIDParamGuidance("Numeric note id within the discussion thread, from a prior discussion response.")
}

// decorateMRDiscussionMeta fills in action-specific discovery metadata for each
// MR discussion individual tool, mirroring the style of the issue discussions
// domain (R-META; 1:1 audit).
//
//nolint:funlen // One self-contained switch arm per individual tool keeps the discovery metadata co-located and readable.
func decorateMRDiscussionMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	switch individualTool {
	case "gitlab_mr_discussion_create":
		options.Usage = "Open a new discussion thread on a merge request. Provide a position to attach an inline diff comment to a specific file and line, or omit it for a general MR discussion. Supports anchoring to a commit_id and backdating via created_at for admins/owners."
		options.Aliases = []string{"gitlab_mr_discussion_create", "create merge request discussion", "comment on merge request diff", "start MR review thread"}
		options.RelatedActions = []string{actionDiscussionReply, actionDiscussionList, actionMRChangesGet, actionMRNoteCreate}
		options.ParameterGuidance = mrScopeGuidance()
		options.ParameterGuidance["body"] = toolutil.ParameterGuidance{
			SemanticRole:   "comment_body",
			ValueSource:    "Markdown text for the first note of the new thread.",
			ExampleBinding: `params.body:"This branch needs a test for the edge case"`,
		}
		options.ParameterGuidance["position"] = toolutil.ParameterGuidance{
			SemanticRole:     "diff_position",
			ValueSource:      "Diff anchor (base_sha, head_sha, start_sha, new_path/old_path and line) from gitlab_mr_changes_get. Omit for a general discussion.",
			ExampleBinding:   `params.position:{"base_sha":"abc","head_sha":"def","start_sha":"abc","new_path":"main.go","new_line":12}`,
			CommonConfusions: []string{"Inline comments require the full SHA triple plus a valid path/line from the MR diff. Omit position entirely for a thread that is not tied to a line."},
		}
		options.IndividualTool.Description = "Create a new discussion thread on a merge request, optionally as an inline diff comment. Returns: the created thread with its first note (author, body, resolvable state, diff position). See also: gitlab_mr_discussion_reply, gitlab_mr_discussion_list, gitlab_mr_changes_get."
	case "gitlab_mr_discussion_list":
		options.Usage = "List all discussion threads on one merge request, including inline diff comments, system notes, and threaded replies. Use this when the prompt asks for an MR's review conversation, or before replying to or resolving a thread. Supports order_by, sort, and keyset pagination."
		options.Aliases = []string{"gitlab_mr_discussion_list", "list merge request discussions", "show MR review threads", "get merge request conversation"}
		options.RelatedActions = []string{actionDiscussionGet, actionDiscussionCreate, actionMRGet, actionMRNoteList}
		options.ParameterGuidance = mrScopeGuidance()
		options.ParameterGuidance["order_by"] = toolutil.ParameterGuidance{
			SemanticRole:     "discussion_list_sort_field",
			ValueSource:      "Column requested for ordering threads, such as created_at or updated_at.",
			ExampleBinding:   `params.order_by:"created_at"`,
			CommonConfusions: []string{"Combine order_by with sort. Do not pass natural-language phrases as the field value."},
		}
		options.IndividualTool.Description = "List discussion threads on a merge request with ordering and keyset pagination. Returns: discussion threads with their notes (author, body, system flag, resolvable state, diff position) and pagination metadata. See also: gitlab_mr_discussion_get, gitlab_mr_discussion_create, gitlab_mr_get, gitlab_mr_notes_list."
	case "gitlab_mr_discussion_get":
		options.Usage = "Fetch one discussion thread on a merge request by its discussion_id, returning every note in the thread. Use this after gitlab_mr_discussion_list when the target thread is already known."
		options.Aliases = []string{"gitlab_mr_discussion_get", "get merge request discussion", "show MR discussion thread", "fetch merge request discussion"}
		options.RelatedActions = []string{actionDiscussionList, actionDiscussionReply, actionDiscussionResolve}
		options.ParameterGuidance = mrScopeGuidance()
		options.ParameterGuidance["discussion_id"] = discussionIDGuidance()
		options.IndividualTool.Description = "Get a single merge request discussion thread by its discussion id. Returns: the thread with every note (author, body, system flag, resolvable/resolved state, diff position). See also: gitlab_mr_discussion_list, gitlab_mr_discussion_reply, gitlab_mr_discussion_resolve."
	case "gitlab_mr_discussion_reply":
		options.Usage = "Reply to an existing merge request discussion thread by adding a note. Use this after gitlab_mr_discussion_list or gitlab_mr_discussion_create to continue a thread. Supports backdating via created_at for admins/owners."
		options.Aliases = []string{"gitlab_mr_discussion_reply", "reply to merge request discussion", "add note to MR discussion", "comment on merge request thread"}
		options.RelatedActions = []string{actionDiscussionCreate, actionDiscussionGet, actionDiscussionNoteUpdate, actionDiscussionResolve}
		options.ParameterGuidance = mrScopeGuidance()
		options.ParameterGuidance["discussion_id"] = discussionIDGuidance()
		options.ParameterGuidance["body"] = toolutil.ParameterGuidance{
			SemanticRole:   "comment_body",
			ValueSource:    "Markdown text of the reply to append to the thread.",
			ExampleBinding: `params.body:"Good catch, pushing a fix now"`,
		}
		options.IndividualTool.Description = "Add a reply note to an existing merge request discussion thread. Returns: the created note (author, body, timestamps, resolvable state). See also: gitlab_mr_discussion_create, gitlab_mr_discussion_get, gitlab_mr_discussion_note_update."
	case "gitlab_mr_discussion_resolve":
		options.Usage = "Resolve or unresolve a merge request discussion thread. Set resolved=true to mark the thread resolved or resolved=false to reopen it. Only resolvable (thread) discussions can be toggled."
		options.Aliases = []string{"gitlab_mr_discussion_resolve", "resolve merge request discussion", "mark MR thread resolved", "reopen merge request thread"}
		options.RelatedActions = []string{actionDiscussionGet, actionDiscussionList, actionDiscussionReply}
		options.ParameterGuidance = mrScopeGuidance()
		options.ParameterGuidance["discussion_id"] = discussionIDGuidance()
		options.ParameterGuidance["resolved"] = toolutil.ParameterGuidance{
			SemanticRole:     "resolve_flag",
			ValueSource:      "Boolean: true resolves the thread, false reopens it.",
			ExampleBinding:   "params.resolved:true",
			CommonConfusions: []string{"resolved is a boolean toggle, not a note body. Only resolvable thread discussions can be resolved."},
		}
		options.IndividualTool.Description = "Resolve or unresolve a merge request discussion thread. Returns: the thread with its updated resolved state and notes. See also: gitlab_mr_discussion_get, gitlab_mr_discussion_list, gitlab_mr_discussion_reply."
	case "gitlab_mr_discussion_note_update":
		options.Usage = "Edit the body of a note in a merge request discussion thread, or resolve/unresolve a single resolvable note. Only the note author can edit the body. Only Maintainers can change the resolved flag. Identify the note with discussion_id plus note_id."
		options.Aliases = []string{"gitlab_mr_discussion_note_update", "edit merge request discussion note", "update MR discussion reply", "modify merge request thread comment"}
		options.RelatedActions = []string{actionDiscussionReply, actionDiscussionNoteDelete, actionDiscussionGet}
		options.ParameterGuidance = mrScopeGuidance()
		options.ParameterGuidance["discussion_id"] = discussionIDGuidance()
		options.ParameterGuidance["note_id"] = noteIDGuidance()
		options.IndividualTool.Description = "Update the body or resolved state of a note in a merge request discussion thread. Returns: the updated note. See also: gitlab_mr_discussion_reply, gitlab_mr_discussion_note_delete, gitlab_mr_discussion_get."
	case "gitlab_mr_discussion_note_delete":
		options.Usage = "Permanently delete a note from a merge request discussion thread (destructive, requires confirmation). Only the note author or a Maintainer can delete a note. Identify the note with discussion_id plus note_id."
		options.Aliases = []string{"gitlab_mr_discussion_note_delete", "delete merge request discussion note", "remove MR discussion reply", "delete merge request thread comment"}
		options.RelatedActions = []string{actionDiscussionNoteUpdate, actionDiscussionGet, actionDiscussionList}
		options.ParameterGuidance = mrScopeGuidance()
		options.ParameterGuidance["discussion_id"] = discussionIDGuidance()
		options.ParameterGuidance["note_id"] = noteIDGuidance()
		options.IndividualTool.Description = "Delete a note from a merge request discussion thread (destructive). Returns: a deletion confirmation message. See also: gitlab_mr_discussion_note_update, gitlab_mr_discussion_get, gitlab_mr_discussion_list."
	default:
		options.Usage = "Use to execute mrdiscussions domain action."
		options.RelatedActions = []string{actionMRGet, actionMRNoteList, actionMRChangesGet}
	}
}
