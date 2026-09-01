package issuediscussions

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical related-action ids. Issue discussion specs are merged into the
// gitlab_issue catalog group, so their canonical ids are issue.<spec_name>
// and cross-domain references use the same issue.* namespace.
const (
	actionDiscussionList       = "issue.discussion_list"
	actionDiscussionGet        = "issue.discussion_get"
	actionDiscussionCreate     = "issue.discussion_create"
	actionDiscussionAddNote    = "issue.discussion_add_note"
	actionDiscussionUpdateNote = "issue.discussion_update_note"
	actionDiscussionDeleteNote = "issue.discussion_delete_note"
	actionIssueGet             = "issue.get"
	actionIssueNoteList        = "issue.note_list"
	actionIssueLinkList        = "issue.link_list"
)

// ActionSpecs returns canonical specs for issue discussion actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		issueDiscussionReadSpec("discussion_list", toolutil.RouteAction(client, List), "gitlab_list_issue_discussions"),
		issueDiscussionReadSpec("discussion_get", toolutil.RouteAction(client, Get), "gitlab_get_issue_discussion"),
		issueDiscussionCreateSpec("discussion_create", toolutil.RouteAction(client, Create), "gitlab_create_issue_discussion"),
		issueDiscussionCreateSpec("discussion_add_note", toolutil.RouteAction(client, AddNote), "gitlab_add_issue_discussion_note"),
		issueDiscussionUpdateSpec("discussion_update_note", toolutil.RouteAction(client, UpdateNote), "gitlab_update_issue_discussion_note"),
		issueDiscussionDeleteSpec("discussion_delete_note", toolutil.DestructiveAction(client, deleteNoteOutput), "gitlab_delete_issue_discussion_note"),
	}
}

func deleteNoteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteNoteInput) (toolutil.DeleteOutput, error) {
	if err := DeleteNote(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("issue discussion note")
	return out, nil
}

func issueDiscussionReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, issueDiscussionOptions(individualTool))
}

func issueDiscussionCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, issueDiscussionOptions(individualTool))
}

func issueDiscussionUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, issueDiscussionOptions(individualTool))
}

func issueDiscussionDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, issueDiscussionOptions(individualTool))
}

// issueDiscussionOptions builds the shared [toolutil.ActionSpecOptions] for an
// issue discussion action and decorates it with action-specific usage,
// natural-language aliases, related actions, parameter guidance, and an
// individual-tool description (R-META; 1:1 audit).
func issueDiscussionOptions(individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Tags: []string{"issue", "discussion"},
		OpenWorld:      true,
		OwnerPackage:   "issuediscussions",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	decorateIssueDiscussionMeta(&options, individualTool)
	return options
}

// projectScopeGuidance returns the parameter guidance shared by every issue
// discussion action for the project_id and issue_iid scoping parameters.
func projectScopeGuidance() map[string]toolutil.ParameterGuidance {
	return map[string]toolutil.ParameterGuidance{
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
			CommonConfusions: []string{"Use issue_iid for the project-scoped issue number, not the global issue ID."},
		},
	}
}

// discussionIDGuidance returns the parameter guidance for the discussion_id
// parameter used by discussion-scoped actions.
func discussionIDGuidance() toolutil.ParameterGuidance {
	return toolutil.DiscussionIDParamGuidance("Discussion thread id from a prior gitlab_list_issue_discussions response.")
}

// noteIDGuidance returns the parameter guidance for the note_id parameter used
// by note update/delete actions.
func noteIDGuidance() toolutil.ParameterGuidance {
	return toolutil.DiscussionNoteIDParamGuidance("Numeric note id within the discussion thread, from a prior discussion response.")
}

// decorateIssueDiscussionMeta fills in action-specific discovery metadata for
// each issue discussion individual tool, mirroring the style of the issues
// domain (R-META; 1:1 audit).
func decorateIssueDiscussionMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	switch individualTool {
	case "gitlab_list_issue_discussions":
		options.Usage = "List all discussion threads on one issue, including system notes and threaded replies. Use this when the prompt asks for an issue's comment threads or conversation history, or before replying to a thread with gitlab_add_issue_discussion_note. Supports order_by, sort, and keyset pagination."
		options.Aliases = []string{"gitlab_list_issue_discussions", "list issue discussions", "show issue comment threads", "get issue conversation"}
		options.RelatedActions = []string{actionDiscussionGet, actionDiscussionCreate, actionIssueGet, actionIssueNoteList}
		options.ParameterGuidance = projectScopeGuidance()
		options.ParameterGuidance["order_by"] = toolutil.ParameterGuidance{
			SemanticRole:     "discussion_list_sort_field",
			ValueSource:      "Column requested for ordering threads, such as created_at or updated_at.",
			ExampleBinding:   `params.order_by:"created_at"`,
			CommonConfusions: []string{"Combine order_by with sort. Do not pass natural-language phrases as the field value."},
		}
		options.IndividualTool.Description = "List discussion threads on an issue with ordering and keyset pagination. Returns: discussion threads with their notes (author, body, system flag, resolvable state) and pagination metadata. See also: gitlab_get_issue_discussion, gitlab_create_issue_discussion, gitlab_issue_get, gitlab_issue_note_list."
	case "gitlab_get_issue_discussion":
		options.Usage = "Fetch one discussion thread on an issue by its discussion_id, returning every note in the thread. Use this after gitlab_list_issue_discussions when the target thread is already known."
		options.Aliases = []string{"gitlab_get_issue_discussion", "get issue discussion", "show issue discussion thread", "fetch issue discussion"}
		options.RelatedActions = []string{actionDiscussionList, actionDiscussionAddNote, actionIssueGet}
		options.ParameterGuidance = projectScopeGuidance()
		options.ParameterGuidance["discussion_id"] = discussionIDGuidance()
		options.IndividualTool.Description = "Get a single issue discussion thread by its discussion id. Returns: the thread with every note (author, body, system flag, resolvable/resolved state). See also: gitlab_list_issue_discussions, gitlab_add_issue_discussion_note, gitlab_issue_get."
	case "gitlab_create_issue_discussion":
		options.Usage = "Open a new discussion thread on an issue with an initial note. Use this to start a threaded conversation rather than a flat comment (use gitlab_issue_note_create for a non-threaded note). Supports backdating via created_at for admins/owners."
		options.Aliases = []string{"gitlab_create_issue_discussion", "create issue discussion", "start issue discussion thread", "open issue discussion"}
		options.RelatedActions = []string{actionDiscussionAddNote, actionDiscussionList, actionIssueNoteList, actionIssueGet}
		options.ParameterGuidance = projectScopeGuidance()
		options.ParameterGuidance["body"] = toolutil.ParameterGuidance{
			SemanticRole:   "comment_body",
			ValueSource:    "Markdown text for the first note of the new thread.",
			ExampleBinding: `params.body:"Investigating this regression"`,
		}
		options.IndividualTool.Description = "Create a new discussion thread on an issue with an initial note. Returns: the created thread with its first note. See also: gitlab_add_issue_discussion_note, gitlab_list_issue_discussions, gitlab_issue_note_create."
	case "gitlab_add_issue_discussion_note":
		options.Usage = "Reply to an existing issue discussion thread by adding a note. Use this after gitlab_list_issue_discussions or gitlab_create_issue_discussion to continue a thread. Supports backdating via created_at for admins/owners."
		options.Aliases = []string{"gitlab_add_issue_discussion_note", "reply to issue discussion", "add note to issue discussion", "comment on issue thread"}
		options.RelatedActions = []string{actionDiscussionCreate, actionDiscussionGet, actionDiscussionUpdateNote, actionIssueNoteList}
		options.ParameterGuidance = projectScopeGuidance()
		options.ParameterGuidance["discussion_id"] = discussionIDGuidance()
		options.ParameterGuidance["body"] = toolutil.ParameterGuidance{
			SemanticRole:   "comment_body",
			ValueSource:    "Markdown text of the reply to append to the thread.",
			ExampleBinding: `params.body:"Confirmed, fix is on the way"`,
		}
		options.IndividualTool.Description = "Add a reply note to an existing issue discussion thread. Returns: the created note (author, body, timestamps). See also: gitlab_create_issue_discussion, gitlab_get_issue_discussion, gitlab_update_issue_discussion_note."
	case "gitlab_update_issue_discussion_note":
		options.Usage = "Edit the body of an existing note in an issue discussion thread. Only the note author can edit a note. Identify the note with discussion_id plus note_id."
		options.Aliases = []string{"gitlab_update_issue_discussion_note", "edit issue discussion note", "update issue discussion reply", "modify issue thread comment"}
		options.RelatedActions = []string{actionDiscussionAddNote, actionDiscussionDeleteNote, actionDiscussionGet}
		options.ParameterGuidance = projectScopeGuidance()
		options.ParameterGuidance["discussion_id"] = discussionIDGuidance()
		options.ParameterGuidance["note_id"] = noteIDGuidance()
		options.IndividualTool.Description = "Update the body of a note in an issue discussion thread. Returns: the updated note. See also: gitlab_add_issue_discussion_note, gitlab_delete_issue_discussion_note, gitlab_get_issue_discussion."
	case "gitlab_delete_issue_discussion_note":
		options.Usage = "Permanently delete a note from an issue discussion thread (destructive, requires confirmation). Only the note author or a Maintainer can delete a note. Identify the note with discussion_id plus note_id."
		options.Aliases = []string{"gitlab_delete_issue_discussion_note", "delete issue discussion note", "remove issue discussion reply", "delete issue thread comment"}
		options.RelatedActions = []string{actionDiscussionUpdateNote, actionDiscussionGet, actionDiscussionList}
		options.ParameterGuidance = projectScopeGuidance()
		options.ParameterGuidance["discussion_id"] = discussionIDGuidance()
		options.ParameterGuidance["note_id"] = noteIDGuidance()
		options.IndividualTool.Description = "Delete a note from an issue discussion thread (destructive). Returns: a deletion confirmation message. See also: gitlab_update_issue_discussion_note, gitlab_get_issue_discussion, gitlab_list_issue_discussions."
	default:
		options.Usage = "Use to execute issuediscussions domain action."
		options.RelatedActions = []string{actionIssueGet, actionIssueNoteList, actionIssueLinkList}
	}
}
