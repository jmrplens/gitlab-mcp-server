package commitdiscussions

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical related-action ids referenced by commit discussion discovery
// metadata. Commit discussion specs own the commit_discussion.* namespace;
// cross-domain references to the commit itself use the repository.* namespace.
const (
	actionList       = "commit_discussion.commit_discussion_list"
	actionGet        = "commit_discussion.commit_discussion_get"
	actionCreate     = "commit_discussion.commit_discussion_create"
	actionAddNote    = "commit_discussion.commit_discussion_add_note"
	actionUpdateNote = "commit_discussion.commit_discussion_update_note"
	actionDeleteNote = "commit_discussion.commit_discussion_delete_note"
	actionCommitGet  = "repository.commit_get"
	actionCommitDiff = "repository.commit_diff"
)

// ActionSpecs returns canonical specs for commit discussion actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		commitDiscussionReadSpec("commit_discussion_list", toolutil.RouteAction(client, List), "gitlab_list_commit_discussions"),
		commitDiscussionReadSpec("commit_discussion_get", toolutil.RouteAction(client, Get), "gitlab_get_commit_discussion"),
		commitDiscussionCreateSpec("commit_discussion_create", toolutil.RouteAction(client, Create), "gitlab_create_commit_discussion"),
		commitDiscussionCreateSpec("commit_discussion_add_note", toolutil.RouteAction(client, AddNote), "gitlab_add_commit_discussion_note"),
		commitDiscussionUpdateSpec("commit_discussion_update_note", toolutil.RouteAction(client, UpdateNote), "gitlab_update_commit_discussion_note"),
		commitDiscussionDeleteSpec("commit_discussion_delete_note", toolutil.DestructiveAction(client, DeleteNoteOutput), "gitlab_delete_commit_discussion_note"),
	}
}

func commitDiscussionReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, commitDiscussionOptions(individualTool))
}

func commitDiscussionCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, commitDiscussionOptions(individualTool))
}

func commitDiscussionUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, commitDiscussionOptions(individualTool))
}

func commitDiscussionDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, commitDiscussionOptions(individualTool))
}

// commitDiscussionOptions builds the shared [toolutil.ActionSpecOptions] for a
// commit discussion action and decorates it with action-specific usage,
// natural-language aliases, related actions, parameter guidance, and an
// individual-tool description (R-META; 1:1 audit).
func commitDiscussionOptions(individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases:        []string{individualTool},
		Tags:           []string{"repository", "commit", "discussion"},
		OpenWorld:      true,
		OwnerPackage:   "commitdiscussions",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	decorateCommitDiscussionMeta(&options, individualTool)
	return options
}

// commitScopeGuidance returns the parameter guidance shared by every commit
// discussion action for the project_id and commit_sha scoping parameters.
func commitScopeGuidance() map[string]toolutil.ParameterGuidance {
	return map[string]toolutil.ParameterGuidance{
		"project_id": {
			SemanticRole:   "scope_project",
			ValueSource:    "Project ID or full namespace path that owns the commit.",
			ExampleBinding: `params.project_id:"group/project"`,
		},
		"commit_sha": {
			SemanticRole:   "commit_sha",
			ValueSource:    "Commit SHA where the discussion lives, from a prior gitlab_commit_get or branch/tag response.",
			ExampleBinding: `params.commit_sha:"abc123def"`,
		},
	}
}

// discussionIDGuidance returns the parameter guidance for the discussion_id
// parameter used by discussion-scoped actions.
func discussionIDGuidance() toolutil.ParameterGuidance {
	return toolutil.DiscussionIDParamGuidance("Discussion thread id from a prior gitlab_list_commit_discussions response.")
}

// noteIDGuidance returns the parameter guidance for the note_id parameter used
// by note update/delete actions.
func noteIDGuidance() toolutil.ParameterGuidance {
	return toolutil.DiscussionNoteIDParamGuidance("Numeric note id within the discussion thread, from a prior discussion response.")
}

// decorateCommitDiscussionMeta fills in action-specific discovery metadata for
// each commit discussion individual tool, mirroring the style of the MR and
// issue discussion domains (R-META; 1:1 audit). Each individual-tool
// description follows the Returns:/See also: form.
//
//nolint:funlen // One self-contained switch arm per individual tool keeps the discovery metadata co-located and readable.
func decorateCommitDiscussionMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	switch individualTool {
	case "gitlab_list_commit_discussions":
		options.Usage = "List all discussion threads tied to a commit SHA, including inline diff comments, system notes, and threaded replies. Use this when the prompt asks for a commit's review conversation, or before replying to a thread. Supports order_by, sort, and keyset pagination."
		options.Aliases = []string{"gitlab_list_commit_discussions", "list commit discussions", "show commit review threads", "get commit conversation"}
		options.RelatedActions = []string{actionGet, actionCreate, actionCommitGet, actionCommitDiff}
		options.ParameterGuidance = commitScopeGuidance()
		options.ParameterGuidance["order_by"] = toolutil.ParameterGuidance{
			SemanticRole:     "discussion_list_sort_field",
			ValueSource:      "Column requested for ordering threads, such as created_at or updated_at.",
			ExampleBinding:   `params.order_by:"created_at"`,
			CommonConfusions: []string{"Combine order_by with sort. Do not pass natural-language phrases as the field value."},
		}
		options.IndividualTool.Description = "List discussion threads on a commit with ordering and keyset pagination. Returns: discussion threads with their notes (author, body, system flag, resolvable state, diff position) and pagination metadata. See also: gitlab_get_commit_discussion, gitlab_create_commit_discussion, gitlab_commit_get."
	case "gitlab_get_commit_discussion":
		options.Usage = "Fetch one discussion thread on a commit by its discussion_id, returning every note in the thread. Use this after gitlab_list_commit_discussions when the target thread is already known."
		options.Aliases = []string{"gitlab_get_commit_discussion", "get commit discussion", "show commit discussion thread", "fetch commit discussion"}
		options.RelatedActions = []string{actionList, actionAddNote, actionCommitGet}
		options.ParameterGuidance = commitScopeGuidance()
		options.ParameterGuidance["discussion_id"] = discussionIDGuidance()
		options.IndividualTool.Description = "Get a single commit discussion thread by its discussion id. Returns: the thread with every note (author, body, system flag, resolvable/resolved state, diff position). See also: gitlab_list_commit_discussions, gitlab_add_commit_discussion_note, gitlab_commit_get."
	case "gitlab_create_commit_discussion":
		options.Usage = "Open a new discussion thread on a commit. Provide a position to attach an inline diff comment to a specific file and line, or omit it for a general commit discussion. Supports backdating via created_at for admins/owners."
		options.Aliases = []string{"gitlab_create_commit_discussion", "create commit discussion", "comment on commit diff", "start commit review thread"}
		options.RelatedActions = []string{actionAddNote, actionList, actionCommitDiff, actionCommitGet}
		options.ParameterGuidance = commitScopeGuidance()
		options.ParameterGuidance["body"] = toolutil.ParameterGuidance{
			SemanticRole:   "comment_body",
			ValueSource:    "Markdown text for the first note of the new thread.",
			ExampleBinding: `params.body:"This change needs a test for the edge case"`,
		}
		options.ParameterGuidance["position"] = toolutil.ParameterGuidance{
			SemanticRole:     "diff_position",
			ValueSource:      "Diff anchor (base_sha, head_sha, start_sha, new_path/old_path and line) from gitlab_commit_diff. Omit for a general discussion.",
			ExampleBinding:   `params.position:{"base_sha":"abc","head_sha":"def","start_sha":"abc","position_type":"text","new_path":"main.go","new_line":12}`,
			CommonConfusions: []string{"Inline comments require the full SHA triple plus a valid path/line from the commit diff. Omit position entirely for a thread that is not tied to a line."},
		}
		options.IndividualTool.Description = "Create a new discussion thread on a commit, optionally as an inline diff comment. Returns: the created thread with its first note (author, body, resolvable state, diff position). See also: gitlab_add_commit_discussion_note, gitlab_list_commit_discussions, gitlab_commit_diff."
	case "gitlab_add_commit_discussion_note":
		options.Usage = "Reply to an existing commit discussion thread by adding a note. Use this after gitlab_list_commit_discussions or gitlab_create_commit_discussion to continue a thread. Supports backdating via created_at for admins/owners."
		options.Aliases = []string{"gitlab_add_commit_discussion_note", "reply to commit discussion", "add note to commit discussion", "comment on commit thread"}
		options.RelatedActions = []string{actionCreate, actionGet, actionUpdateNote, actionList}
		options.ParameterGuidance = commitScopeGuidance()
		options.ParameterGuidance["discussion_id"] = discussionIDGuidance()
		options.ParameterGuidance["body"] = toolutil.ParameterGuidance{
			SemanticRole:   "comment_body",
			ValueSource:    "Markdown text of the reply to append to the thread.",
			ExampleBinding: `params.body:"Good catch, pushing a fix now"`,
		}
		options.IndividualTool.Description = "Add a reply note to an existing commit discussion thread. Returns: the created note (author, body, timestamps, resolvable state). See also: gitlab_create_commit_discussion, gitlab_get_commit_discussion, gitlab_update_commit_discussion_note."
	case "gitlab_update_commit_discussion_note":
		options.Usage = "Edit the body of a note in a commit discussion thread. Only the note author can edit the body. Identify the note with discussion_id plus note_id. Supports overriding created_at for admins/owners."
		options.Aliases = []string{"gitlab_update_commit_discussion_note", "edit commit discussion note", "update commit discussion reply", "modify commit thread comment"}
		options.RelatedActions = []string{actionAddNote, actionDeleteNote, actionGet}
		options.ParameterGuidance = commitScopeGuidance()
		options.ParameterGuidance["discussion_id"] = discussionIDGuidance()
		options.ParameterGuidance["note_id"] = noteIDGuidance()
		options.ParameterGuidance["body"] = toolutil.ParameterGuidance{
			SemanticRole:   "comment_body",
			ValueSource:    "New Markdown body that replaces the current note text.",
			ExampleBinding: `params.body:"Updated: resolved in the latest push"`,
		}
		options.IndividualTool.Description = "Update the body of a note in a commit discussion thread. Returns: the updated note (author, body, timestamps). See also: gitlab_add_commit_discussion_note, gitlab_delete_commit_discussion_note, gitlab_get_commit_discussion."
	case "gitlab_delete_commit_discussion_note":
		options.Usage = "Permanently delete a note from a commit discussion thread (destructive, requires confirmation). Only the note author or a Maintainer can delete a note. Identify the note with discussion_id plus note_id."
		options.Aliases = []string{"gitlab_delete_commit_discussion_note", "delete commit discussion note", "remove commit discussion reply", "delete commit thread comment"}
		options.RelatedActions = []string{actionUpdateNote, actionGet, actionList}
		options.ParameterGuidance = commitScopeGuidance()
		options.ParameterGuidance["discussion_id"] = discussionIDGuidance()
		options.ParameterGuidance["note_id"] = noteIDGuidance()
		options.IndividualTool.Description = "Delete a note from a commit discussion thread (destructive). Returns: a deletion confirmation message. See also: gitlab_update_commit_discussion_note, gitlab_get_commit_discussion, gitlab_list_commit_discussions."
	default:
		options.Usage = "Manage commit discussions and discussion notes (list/get/create/add/update/delete). Use this for threaded review context tied to a commit SHA."
		options.RelatedActions = []string{actionCommitGet, actionCommitDiff}
		options.ParameterGuidance = commitScopeGuidance()
	}
}

// DeleteNoteOutput deletes a commit discussion note and returns the canonical success message shape.
func DeleteNoteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteNoteInput) (toolutil.DeleteOutput, error) {
	if err := DeleteNote(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted commit discussion note."}, nil
}
