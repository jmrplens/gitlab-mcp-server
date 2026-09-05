package issuenotes

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for issue note actions exposed as
// MCP tools. The create/list/get/update/delete routes are projected into
// the dynamic, meta, individual, and audit surfaces by the action
// catalog (ADR-0004).
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_issue_note_create — add a comment (note) to an issue.
		issueNoteCreateSpec("note_create", toolutil.RouteAction(client, Create), "gitlab_issue_note_create"),
		// gitlab_issue_note_list — list all notes attached to an issue.
		issueNoteReadSpec("note_list", toolutil.RouteAction(client, List), "gitlab_issue_note_list"),
		// gitlab_issue_note_get — fetch a single note by its ID.
		issueNoteReadSpec("note_get", toolutil.RouteAction(client, GetNote), "gitlab_issue_note_get"),
		// gitlab_issue_note_update — replace the body of an existing note.
		issueNoteUpdateSpec("note_update", toolutil.RouteAction(client, Update), "gitlab_issue_note_update"),
		// gitlab_issue_note_delete — remove a note (destructive, requires confirmation).
		issueNoteDeleteSpec("note_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_issue_note_delete"),
	}
}

// deleteOutput adapts the package's [Delete] handler to the
// [toolutil.DestructiveAction] contract, returning a structured success
// result that includes the project/issue/note triple in the message.
func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult(fmt.Sprintf("note %d from issue #%d in project %s", input.NoteID, input.IssueIID, input.ProjectID))
	return out, nil
}

// issueNoteReadSpec builds a read-only [toolutil.ActionSpec] for an issue
// note action, applying the package's default [issueNoteOptions].
func issueNoteReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, issueNoteOptions(individualTool))
}

// issueNoteCreateSpec builds a create-style [toolutil.ActionSpec] for an
// issue note action, applying the package's default [issueNoteOptions].
func issueNoteCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, issueNoteOptions(individualTool))
}

// issueNoteUpdateSpec builds an update-style [toolutil.ActionSpec] for an
// issue note action, applying the package's default [issueNoteOptions].
func issueNoteUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, issueNoteOptions(individualTool))
}

// issueNoteDeleteSpec builds a destructive [toolutil.ActionSpec] for an
// issue note action, applying the package's default [issueNoteOptions].
func issueNoteDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, issueNoteOptions(individualTool))
}

// Canonical action IDs referenced by issue note RelatedActions. The issue note
// actions are projected under the gitlab_issue catalog group, so their base
// domain is "issue".
const (
	actionIssueGet         = "issue.get"
	actionIssueNoteList    = "issue.note_list"
	actionIssueNoteGet     = "issue.note_get"
	actionIssueNoteCreate  = "issue.note_create"
	actionIssueNoteUpdate  = "issue.note_update"
	actionIssueNoteDelete  = "issue.note_delete"
	actionIssueDiscussions = "issue.discussion_list"
	actionIssueLinkList    = "issue.link_list"
)

// projectIDGuidance is the shared parameter guidance for the project_id input
// that every issue note action accepts.
func projectIDGuidance() toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:     "scope_project",
		ValueSource:      "Project ID or full namespace path that owns the issue.",
		ExampleBinding:   `params.project_id:"group/project"`,
		CommonConfusions: []string{"Use the issue's parent project here, not a group path or global issue ID."},
	}
}

// issueIIDGuidance is the shared parameter guidance for the issue_iid input.
func issueIIDGuidance() toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:     "issue_iid",
		ValueSource:      "Issue number visible in the project, from the issue URL or prior issue list output.",
		ExampleBinding:   "params.issue_iid:42",
		CommonConfusions: []string{"Use the project-scoped issue IID, not the global issue database ID."},
	}
}

// noteIDGuidance is the shared parameter guidance for the note_id input.
func noteIDGuidance() toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:     "note_id",
		ValueSource:      "Numeric note ID from a prior issue.note_list or issue.note_create result.",
		ExampleBinding:   "params.note_id:100",
		CommonConfusions: []string{"note_id is the comment ID, not the issue_iid. Obtain it from issue.note_list."},
	}
}

// issueNoteOptions returns the [toolutil.ActionSpecOptions] for one issue note
// action, filled with non-generic Usage, natural-language Aliases,
// RelatedActions, ParameterGuidance, and the "Returns: … See also: …"
// individual-tool description (1:1 audit R-META).
func issueNoteOptions(individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases:        []string{individualTool},
		Usage:          "Use to execute issuenotes domain action.",
		Tags:           []string{"issue", "note"},
		OpenWorld:      true,
		OwnerPackage:   "issuenotes",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	decorateIssueNoteMeta(&options, individualTool)
	return options
}

// decorateIssueNoteMeta fills the per-action discovery metadata for each issue
// note individual tool, mirroring the style of the issues package.
func decorateIssueNoteMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	switch individualTool {
	case "gitlab_issue_note_create":
		options.Usage = "Add a comment (note) to an issue. Use when the task asks to comment on, reply to, or annotate an existing issue. Set internal=true for a confidential note visible only to project members."
		options.Aliases = []string{"comment on issue", "add issue comment", "reply to issue", "post issue note"}
		options.RelatedActions = []string{actionIssueGet, actionIssueNoteList, actionIssueDiscussions}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": projectIDGuidance(),
			"issue_iid":  issueIIDGuidance(),
			"body": {
				ValueSource:      "The comment text the user wants to post. Markdown is supported.",
				ExampleBinding:   `params.body:"Thanks, merging now."`,
				CommonConfusions: []string{"body is the comment content, not a description update. Updating the issue body is issue.update."},
			},
			"internal": {
				ValueSource:      "Set true only when the user asks for a private/internal note.",
				CommonConfusions: []string{"internal notes are visible to project members only. Do not set for public replies."},
			},
		}
		options.IndividualTool.Description = "Add a comment (note) to an issue. Returns: the created note with id, author, body, timestamps, and resolvable/internal flags. See also: gitlab_issue_note_list, gitlab_issue_get, gitlab_create_issue_discussion."
	case "gitlab_issue_note_list":
		options.Usage = "List all comments (notes) on an issue, including system notes. Use when the task asks to read an issue's discussion, recent comments, or activity. Supports order_by, sort, and keyset pagination."
		options.Aliases = []string{"list issue comments", "show issue notes", "read issue discussion", "get issue comments"}
		options.RelatedActions = []string{actionIssueNoteGet, actionIssueNoteCreate, actionIssueGet, actionIssueDiscussions}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": projectIDGuidance(),
			"issue_iid":  issueIIDGuidance(),
			"order_by": {
				ValueSource:      "Field to order notes by: created_at or updated_at.",
				ExampleBinding:   `params.order_by:"created_at"`,
				CommonConfusions: []string{"Combine order_by with sort. Pass the field name, not a phrase like 'newest first'."},
			},
		}
		options.IndividualTool.Description = "List all notes (comments) on an issue. Returns: notes with author, body, system/internal flags, and pagination metadata. See also: gitlab_issue_note_get, gitlab_issue_note_create, gitlab_get_issue_discussion."
		// https://docs.gitlab.com/api/notes/ (list project issue notes): notes order by created_at or updated_at only.
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaEnumOverride("order_by", "created_at", "updated_at"),
		}
	case "gitlab_issue_note_get":
		options.Usage = "Get one issue note by params.note_id. Use when the task references a specific comment or note ID on an issue."
		options.Aliases = []string{"get issue comment", "show issue note", "fetch issue note"}
		options.RelatedActions = []string{actionIssueNoteList, actionIssueNoteUpdate, actionIssueNoteDelete}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": projectIDGuidance(),
			"issue_iid":  issueIIDGuidance(),
			"note_id":    noteIDGuidance(),
		}
		options.IndividualTool.Description = "Get a single issue note by its ID. Returns: the note with author, body, timestamps, position, and resolvable/internal flags. See also: gitlab_issue_note_list, gitlab_issue_note_update, gitlab_issue_note_delete."
	case "gitlab_issue_note_update":
		options.Usage = "Replace the body of an existing issue note. Only the original author can edit a note. System notes cannot be edited. Use when the task asks to edit, fix, or amend a comment."
		options.Aliases = []string{"edit issue comment", "update issue note", "amend issue comment"}
		options.RelatedActions = []string{actionIssueNoteGet, actionIssueNoteList, actionIssueNoteDelete}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": projectIDGuidance(),
			"issue_iid":  issueIIDGuidance(),
			"note_id":    noteIDGuidance(),
			"body": {
				ValueSource:      "The new comment text that replaces the existing note body. Markdown is supported.",
				ExampleBinding:   `params.body:"Updated: resolved in !123."`,
				CommonConfusions: []string{"This replaces the whole note body. It does not append to it."},
			},
		}
		options.IndividualTool.Description = "Update an issue note's body. Returns: the updated note with new body and updated_at timestamp. See also: gitlab_issue_note_get, gitlab_issue_note_list, gitlab_issue_note_delete."
	case "gitlab_issue_note_delete":
		options.Usage = "Permanently delete an issue note. Destructive and irreversible. Only the note author or a project Maintainer can delete a note. System notes cannot be deleted. Requires explicit confirmation."
		options.Aliases = []string{"delete issue comment", "remove issue note", "delete issue note"}
		options.RelatedActions = []string{actionIssueNoteGet, actionIssueNoteList, actionIssueLinkList}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": projectIDGuidance(),
			"issue_iid":  issueIIDGuidance(),
			"note_id":    noteIDGuidance(),
		}
		options.IndividualTool.Description = "Delete an issue note permanently. Returns: a success confirmation naming the note, issue, and project. See also: gitlab_issue_note_get, gitlab_issue_note_list."
	}
}
