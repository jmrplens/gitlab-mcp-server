package snippetdiscussions

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical related-action ids. Snippet discussion specs are merged into the
// gitlab_snippet catalog group, so their canonical ids are snippet.<spec_name>
// and cross-domain references use the same snippet.* namespace.
const (
	actionDiscussionList       = "snippet.discussion_list"
	actionDiscussionGet        = "snippet.discussion_get"
	actionDiscussionCreate     = "snippet.discussion_create"
	actionDiscussionAddNote    = "snippet.discussion_add_note"
	actionDiscussionUpdateNote = "snippet.discussion_update_note"
	actionDiscussionDeleteNote = "snippet.discussion_delete_note"
	actionSnippetGet           = "snippet.project_get"
	actionSnippetNoteList      = "snippet.note_list"
)

// ActionSpecs returns canonical specs for snippet discussion actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		snippetDiscussionReadSpec("discussion_list", toolutil.RouteAction(client, List), "gitlab_list_snippet_discussions"),
		snippetDiscussionReadSpec("discussion_get", toolutil.RouteAction(client, Get), "gitlab_get_snippet_discussion"),
		snippetDiscussionCreateSpec("discussion_create", toolutil.RouteAction(client, Create), "gitlab_create_snippet_discussion"),
		snippetDiscussionCreateSpec("discussion_add_note", toolutil.RouteAction(client, AddNote), "gitlab_add_snippet_discussion_note"),
		snippetDiscussionUpdateSpec("discussion_update_note", toolutil.RouteAction(client, UpdateNote), "gitlab_update_snippet_discussion_note"),
		snippetDiscussionDeleteSpec("discussion_delete_note", toolutil.DestructiveAction(client, deleteNoteOutput), "gitlab_delete_snippet_discussion_note"),
	}
}

func deleteNoteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteNoteInput) (toolutil.DeleteOutput, error) {
	if err := DeleteNote(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("snippet discussion note")
	return out, nil
}

func snippetDiscussionReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, snippetDiscussionOptions(individualTool))
}

func snippetDiscussionCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, snippetDiscussionOptions(individualTool))
}

func snippetDiscussionUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, snippetDiscussionOptions(individualTool))
}

func snippetDiscussionDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, snippetDiscussionOptions(individualTool))
}

// snippetDiscussionOptions builds the shared [toolutil.ActionSpecOptions] for a
// snippet discussion action and decorates it with action-specific usage,
// natural-language aliases, related actions, parameter guidance, and an
// individual-tool description (R-META; 1:1 audit).
func snippetDiscussionOptions(individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Tags: []string{"snippet", "discussion"},
		OpenWorld:      true,
		OwnerPackage:   "snippetdiscussions",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	decorateSnippetDiscussionMeta(&options, individualTool)
	return options
}

// snippetScopeGuidance returns the parameter guidance shared by every snippet
// discussion action for the project_id and snippet_id scoping parameters.
func snippetScopeGuidance() map[string]toolutil.ParameterGuidance {
	return map[string]toolutil.ParameterGuidance{
		"project_id": {
			SemanticRole:     "scope_project",
			ValueSource:      "Project ID or full namespace path that owns the snippet.",
			ExampleBinding:   `params.project_id:"group/project"`,
			CommonConfusions: []string{"Use the snippet's parent project here, not a group path or a personal snippet id."},
		},
		"snippet_id": {
			SemanticRole:     "snippet_id",
			ValueSource:      "Numeric snippet id within the project, usually from a prior gitlab_project_snippet_list response.",
			ExampleBinding:   "params.snippet_id:42",
			CommonConfusions: []string{"Use the project snippet id, not a personal snippet id or the discussion id."},
		},
	}
}

// discussionIDGuidance returns the parameter guidance for the discussion_id
// parameter used by discussion-scoped actions.
func discussionIDGuidance() toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:     "discussion_id",
		ValueSource:      "Discussion thread id from a prior gitlab_list_snippet_discussions response.",
		ExampleBinding:   `params.discussion_id:"6a9c1750b37d513a43987b574953fceb50b03ce7"`,
		CommonConfusions: []string{"The discussion_id is the thread hash, not a note id; pass note_id separately for note actions."},
	}
}

// noteIDGuidance returns the parameter guidance for the note_id parameter used
// by note update/delete actions.
func noteIDGuidance() toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:     "note_id",
		ValueSource:      "Numeric note id within the discussion thread, from a prior discussion response.",
		ExampleBinding:   "params.note_id:300",
		CommonConfusions: []string{"note_id is the numeric id of a single note; discussion_id is the thread hash."},
	}
}

// decorateSnippetDiscussionMeta fills in action-specific discovery metadata for
// each snippet discussion individual tool, mirroring the style of the issue
// discussion domain (R-META; 1:1 audit).
func decorateSnippetDiscussionMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	switch individualTool {
	case "gitlab_list_snippet_discussions":
		options.Usage = "List all discussion threads on one project snippet, including system notes and threaded replies. Use this when the prompt asks for a snippet's comment threads or conversation history, or before replying to a thread with gitlab_add_snippet_discussion_note. Supports order_by, sort, and keyset pagination."
		options.Aliases = []string{"gitlab_list_snippet_discussions", "list snippet discussions", "show snippet comment threads", "get snippet conversation"}
		options.RelatedActions = []string{actionDiscussionGet, actionDiscussionCreate, actionSnippetGet, actionSnippetNoteList}
		options.ParameterGuidance = snippetScopeGuidance()
		options.ParameterGuidance["order_by"] = toolutil.ParameterGuidance{
			SemanticRole:     "discussion_list_sort_field",
			ValueSource:      "Column requested for ordering threads, such as created_at or updated_at.",
			ExampleBinding:   `params.order_by:"created_at"`,
			CommonConfusions: []string{"Combine order_by with sort; do not pass natural-language phrases as the field value."},
		}
		options.IndividualTool.Description = "List discussion threads on a project snippet with ordering and keyset pagination. Returns: discussion threads with their notes (author, body, system flag, resolvable state) and pagination metadata. See also: gitlab_get_snippet_discussion, gitlab_create_snippet_discussion, gitlab_project_snippet_get, gitlab_snippet_note_list."
	case "gitlab_get_snippet_discussion":
		options.Usage = "Fetch one discussion thread on a project snippet by its discussion_id, returning every note in the thread. Use this after gitlab_list_snippet_discussions when the target thread is already known."
		options.Aliases = []string{"gitlab_get_snippet_discussion", "get snippet discussion", "show snippet discussion thread", "fetch snippet discussion"}
		options.RelatedActions = []string{actionDiscussionList, actionDiscussionAddNote, actionSnippetGet}
		options.ParameterGuidance = snippetScopeGuidance()
		options.ParameterGuidance["discussion_id"] = discussionIDGuidance()
		options.IndividualTool.Description = "Get a single snippet discussion thread by its discussion id. Returns: the thread with every note (author, body, system flag, resolvable/resolved state). See also: gitlab_list_snippet_discussions, gitlab_add_snippet_discussion_note, gitlab_project_snippet_get."
	case "gitlab_create_snippet_discussion":
		options.Usage = "Open a new discussion thread on a project snippet with an initial note. Use this to start a threaded conversation rather than a flat comment (use gitlab_snippet_note_create for a non-threaded note). Supports backdating via created_at for admins/owners."
		options.Aliases = []string{"gitlab_create_snippet_discussion", "create snippet discussion", "start snippet discussion thread", "open snippet discussion"}
		options.RelatedActions = []string{actionDiscussionAddNote, actionDiscussionList, actionSnippetNoteList, actionSnippetGet}
		options.ParameterGuidance = snippetScopeGuidance()
		options.ParameterGuidance["body"] = toolutil.ParameterGuidance{
			SemanticRole:   "comment_body",
			ValueSource:    "Markdown text for the first note of the new thread.",
			ExampleBinding: `params.body:"Reviewing this snippet"`,
		}
		options.IndividualTool.Description = "Create a new discussion thread on a project snippet with an initial note. Returns: the created thread with its first note. See also: gitlab_add_snippet_discussion_note, gitlab_list_snippet_discussions, gitlab_snippet_note_create."
	case "gitlab_add_snippet_discussion_note":
		options.Usage = "Reply to an existing snippet discussion thread by adding a note. Use this after gitlab_list_snippet_discussions or gitlab_create_snippet_discussion to continue a thread. Supports backdating via created_at for admins/owners."
		options.Aliases = []string{"gitlab_add_snippet_discussion_note", "reply to snippet discussion", "add note to snippet discussion", "comment on snippet thread"}
		options.RelatedActions = []string{actionDiscussionCreate, actionDiscussionGet, actionDiscussionUpdateNote, actionSnippetNoteList}
		options.ParameterGuidance = snippetScopeGuidance()
		options.ParameterGuidance["discussion_id"] = discussionIDGuidance()
		options.ParameterGuidance["body"] = toolutil.ParameterGuidance{
			SemanticRole:   "comment_body",
			ValueSource:    "Markdown text of the reply to append to the thread.",
			ExampleBinding: `params.body:"Confirmed, fix is on the way"`,
		}
		options.IndividualTool.Description = "Add a reply note to an existing snippet discussion thread. Returns: the created note (author, body, timestamps). See also: gitlab_create_snippet_discussion, gitlab_get_snippet_discussion, gitlab_update_snippet_discussion_note."
	case "gitlab_update_snippet_discussion_note":
		options.Usage = "Edit the body of an existing note in a snippet discussion thread. Only the note author can edit a note. Identify the note with discussion_id plus note_id."
		options.Aliases = []string{"gitlab_update_snippet_discussion_note", "edit snippet discussion note", "update snippet discussion reply", "modify snippet thread comment"}
		options.RelatedActions = []string{actionDiscussionAddNote, actionDiscussionDeleteNote, actionDiscussionGet}
		options.ParameterGuidance = snippetScopeGuidance()
		options.ParameterGuidance["discussion_id"] = discussionIDGuidance()
		options.ParameterGuidance["note_id"] = noteIDGuidance()
		options.IndividualTool.Description = "Update the body of a note in a snippet discussion thread. Returns: the updated note. See also: gitlab_add_snippet_discussion_note, gitlab_delete_snippet_discussion_note, gitlab_get_snippet_discussion."
	case "gitlab_delete_snippet_discussion_note":
		options.Usage = "Permanently delete a note from a snippet discussion thread (destructive, requires confirmation). Only the note author or a Maintainer can delete a note. Identify the note with discussion_id plus note_id."
		options.Aliases = []string{"gitlab_delete_snippet_discussion_note", "delete snippet discussion note", "remove snippet discussion reply", "delete snippet thread comment"}
		options.RelatedActions = []string{actionDiscussionUpdateNote, actionDiscussionGet, actionDiscussionList}
		options.ParameterGuidance = snippetScopeGuidance()
		options.ParameterGuidance["discussion_id"] = discussionIDGuidance()
		options.ParameterGuidance["note_id"] = noteIDGuidance()
		options.IndividualTool.Description = "Delete a note from a snippet discussion thread (destructive). Returns: a deletion confirmation message. See also: gitlab_update_snippet_discussion_note, gitlab_get_snippet_discussion, gitlab_list_snippet_discussions."
	default:
		options.Usage = "Use to execute snippetdiscussions domain action."
		options.RelatedActions = []string{actionSnippetGet, actionSnippetNoteList}
	}
}
