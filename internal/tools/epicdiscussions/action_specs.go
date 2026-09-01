package epicdiscussions

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical related-action ids. Epic discussion specs are merged into the
// gitlab_group catalog group, so their canonical ids are group.<spec_name>
// and cross-domain references use the same group.* namespace.
const (
	actionDiscussionList       = "group.epic_discussion_list"
	actionDiscussionGet        = "group.epic_discussion_get"
	actionDiscussionCreate     = "group.epic_discussion_create"
	actionDiscussionAddNote    = "group.epic_discussion_add_note"
	actionDiscussionUpdateNote = "group.epic_discussion_update_note"
	actionDiscussionDeleteNote = "group.epic_discussion_delete_note"
	actionEpicGet              = "group.epic_get"
	actionEpicList             = "group.epic_list"
	actionEpicNoteList         = "group.epic_note_list"
)

// ActionSpecs returns canonical specs for epic discussion actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		epicDiscussionReadSpec("epic_discussion_list", toolutil.RouteAction(client, List), "gitlab_list_epic_discussions"),
		epicDiscussionReadSpec("epic_discussion_get", toolutil.RouteAction(client, Get), "gitlab_get_epic_discussion"),
		epicDiscussionCreateSpec("epic_discussion_create", toolutil.RouteAction(client, Create), "gitlab_create_epic_discussion"),
		epicDiscussionCreateSpec("epic_discussion_add_note", toolutil.RouteAction(client, AddNote), "gitlab_add_epic_discussion_note"),
		epicDiscussionUpdateSpec("epic_discussion_update_note", toolutil.RouteAction(client, UpdateNote), "gitlab_update_epic_discussion_note"),
		epicDiscussionDeleteSpec("epic_discussion_delete_note", toolutil.DestructiveAction(client, DeleteNoteOutput), "gitlab_delete_epic_discussion_note"),
	}
}

// DeleteNoteOutput deletes an epic discussion note and returns the canonical success message shape.
func DeleteNoteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteNoteInput) (toolutil.DeleteOutput, error) {
	if err := DeleteNote(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted epic discussion note."}, nil
}

func epicDiscussionReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, epicDiscussionOptions(individualTool))
}

func epicDiscussionCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, epicDiscussionOptions(individualTool))
}

func epicDiscussionUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, epicDiscussionOptions(individualTool))
}

func epicDiscussionDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, epicDiscussionOptions(individualTool))
}

// epicDiscussionOptions builds the shared [toolutil.ActionSpecOptions] for an
// epic discussion action and decorates it with action-specific usage,
// natural-language aliases, related actions, parameter guidance, and an
// individual-tool description (R-META; 1:1 audit). Epics are Premium/Ultimate
// Work Items, so every spec keeps the premium edition gate.
func epicDiscussionOptions(individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases:        []string{individualTool},
		Tags:           []string{"group", "epic", "discussion"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "epicdiscussions",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	decorateEpicDiscussionMeta(&options, individualTool)
	return options
}

// epicScopeGuidance returns the parameter guidance shared by every epic
// discussion action for the full_path and epic_iid scoping parameters.
func epicScopeGuidance() map[string]toolutil.ParameterGuidance {
	return map[string]toolutil.ParameterGuidance{
		"full_path": {
			SemanticRole:     "scope_group",
			ValueSource:      "Full path of the group that owns the epic, from a prior gitlab_group_list or gitlab_epic_list response.",
			ExampleBinding:   `params.full_path:"my-group/sub-group"`,
			CommonConfusions: []string{"Use the group full path here, not a project path or a numeric group ID. Epics live on groups, not projects."},
		},
		"epic_iid": {
			SemanticRole:     "epic_iid",
			ValueSource:      "Epic number visible within the group, usually from the epic URL or a prior gitlab_epic_list response.",
			ExampleBinding:   "params.epic_iid:42",
			CommonConfusions: []string{"Use epic_iid for the group-scoped epic number, not the global epic ID or a work item GID."},
		},
	}
}

// discussionIDGuidance returns the parameter guidance for the discussion_id
// parameter used by discussion-scoped actions.
func discussionIDGuidance() toolutil.ParameterGuidance {
	return toolutil.DiscussionIDParamGuidance("Discussion thread id (hex hash or full Discussion GID) from a prior gitlab_list_epic_discussions response.")
}

// noteIDGuidance returns the parameter guidance for the note_id parameter used
// by note update/delete actions.
func noteIDGuidance() toolutil.ParameterGuidance {
	return toolutil.DiscussionNoteIDParamGuidance("Numeric note id within the discussion thread, from a prior epic discussion response.")
}

// bodyGuidance returns the parameter guidance for the Markdown body parameter
// with action-specific value source text.
func bodyGuidance(valueSource, example string) toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:   "comment_body",
		ValueSource:    valueSource,
		ExampleBinding: example,
	}
}

// decorateEpicDiscussionMeta fills in action-specific discovery metadata for
// each epic discussion individual tool, mirroring the style of the issue
// discussions domain (R-META; 1:1 audit).
func decorateEpicDiscussionMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	switch individualTool {
	case "gitlab_list_epic_discussions":
		options.Usage = "List all discussion threads on one group epic, including system notes and threaded replies, via the Work Items GraphQL API. Use this when the prompt asks for an epic's comment threads or conversation history, or before replying to a thread with gitlab_add_epic_discussion_note. Supports cursor-based keyset pagination (first/after)."
		options.Aliases = []string{"gitlab_list_epic_discussions", "list epic discussions", "show epic comment threads", "get epic conversation"}
		options.RelatedActions = []string{actionDiscussionGet, actionDiscussionCreate, actionEpicGet, actionEpicNoteList}
		options.ParameterGuidance = epicScopeGuidance()
		options.IndividualTool.Description = "List discussion threads on a group epic with cursor-based keyset pagination. Returns: discussion threads with their notes (id, author username, body, system flag, timestamps) and pagination metadata. See also: gitlab_get_epic_discussion, gitlab_create_epic_discussion, gitlab_epic_get, gitlab_epic_note_list."
	case "gitlab_get_epic_discussion":
		options.Usage = "Fetch one discussion thread on a group epic by its discussion_id, returning every note in the thread. Use this after gitlab_list_epic_discussions when the target thread is already known."
		options.Aliases = []string{"gitlab_get_epic_discussion", "get epic discussion", "show epic discussion thread", "fetch epic discussion"}
		options.RelatedActions = []string{actionDiscussionList, actionDiscussionAddNote, actionEpicGet}
		options.ParameterGuidance = epicScopeGuidance()
		options.ParameterGuidance["discussion_id"] = discussionIDGuidance()
		options.IndividualTool.Description = "Get a single epic discussion thread by its discussion id. Returns: the thread with every note (id, author username, body, system flag, timestamps). See also: gitlab_list_epic_discussions, gitlab_add_epic_discussion_note, gitlab_epic_get."
	case "gitlab_create_epic_discussion":
		options.Usage = "Open a new discussion thread on a group epic with an initial note via the createNote GraphQL mutation. Use this to start a threaded conversation rather than a flat comment (use gitlab_epic_note_create for a non-threaded note)."
		options.Aliases = []string{"gitlab_create_epic_discussion", "create epic discussion", "start epic discussion thread", "open epic discussion"}
		options.RelatedActions = []string{actionDiscussionAddNote, actionDiscussionList, actionEpicNoteList, actionEpicGet}
		options.ParameterGuidance = epicScopeGuidance()
		options.ParameterGuidance["body"] = bodyGuidance(
			"Markdown text (GitLab Flavored Markdown) for the first note of the new thread.",
			`params.body:"Investigating this regression"`,
		)
		options.IndividualTool.Description = "Create a new discussion thread on a group epic with an initial note. Returns: the created thread (discussion id) with its first note. See also: gitlab_add_epic_discussion_note, gitlab_list_epic_discussions, gitlab_epic_note_create."
	case "gitlab_add_epic_discussion_note":
		options.Usage = "Reply to an existing epic discussion thread by adding a note via the createNote GraphQL mutation. Use this after gitlab_list_epic_discussions or gitlab_create_epic_discussion to continue a thread. Cannot reply to a system-generated discussion."
		options.Aliases = []string{"gitlab_add_epic_discussion_note", "reply to epic discussion", "add note to epic discussion", "comment on epic thread"}
		options.RelatedActions = []string{actionDiscussionCreate, actionDiscussionGet, actionDiscussionUpdateNote, actionEpicNoteList}
		options.ParameterGuidance = epicScopeGuidance()
		options.ParameterGuidance["discussion_id"] = discussionIDGuidance()
		options.ParameterGuidance["body"] = bodyGuidance(
			"Markdown text (GitLab Flavored Markdown) of the reply to append to the thread.",
			`params.body:"Confirmed, fix is on the way"`,
		)
		options.IndividualTool.Description = "Add a reply note to an existing epic discussion thread. Returns: the created note (id, author username, body, timestamps). See also: gitlab_create_epic_discussion, gitlab_get_epic_discussion, gitlab_update_epic_discussion_note."
	case "gitlab_update_epic_discussion_note":
		options.Usage = "Edit the body of an existing note in an epic discussion thread via the updateNote GraphQL mutation. Only the note author or a Maintainer/Owner can edit a note. Identify the note with note_id."
		options.Aliases = []string{"gitlab_update_epic_discussion_note", "edit epic discussion note", "update epic discussion reply", "modify epic thread comment"}
		options.RelatedActions = []string{actionDiscussionAddNote, actionDiscussionDeleteNote, actionDiscussionGet}
		options.ParameterGuidance = epicScopeGuidance()
		options.ParameterGuidance["note_id"] = noteIDGuidance()
		options.ParameterGuidance["body"] = bodyGuidance(
			"Replacement Markdown text (GitLab Flavored Markdown) for the note body.",
			`params.body:"Updated: the fix has merged"`,
		)
		options.IndividualTool.Description = "Update the body of a note in an epic discussion thread. Returns: the updated note (id, author username, body, timestamps). See also: gitlab_add_epic_discussion_note, gitlab_delete_epic_discussion_note, gitlab_get_epic_discussion."
	case "gitlab_delete_epic_discussion_note":
		options.Usage = "Permanently delete a note from an epic discussion thread (destructive, requires confirmation) via the destroyNote GraphQL mutation. Only the note author or a Maintainer/Owner can delete a note. System-generated notes cannot be removed. Identify the note with note_id."
		options.Aliases = []string{"gitlab_delete_epic_discussion_note", "delete epic discussion note", "remove epic discussion reply", "delete epic thread comment"}
		options.RelatedActions = []string{actionDiscussionUpdateNote, actionDiscussionGet, actionDiscussionList}
		options.ParameterGuidance = epicScopeGuidance()
		options.ParameterGuidance["note_id"] = noteIDGuidance()
		options.IndividualTool.Description = "Delete a note from an epic discussion thread (destructive). Returns: a deletion confirmation message. See also: gitlab_update_epic_discussion_note, gitlab_get_epic_discussion, gitlab_list_epic_discussions."
	default:
		options.Usage = "Use to execute epicdiscussions domain action."
		options.RelatedActions = []string{actionEpicGet, actionEpicList, actionEpicNoteList}
	}
}
