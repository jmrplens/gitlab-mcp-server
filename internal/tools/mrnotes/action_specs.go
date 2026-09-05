package mrnotes

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for merge request note actions exposed as
// MCP tools. The create/list/get/update/delete routes are projected into the
// dynamic, meta, individual, and audit surfaces by the action catalog
// (ADR-0004).
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		mrNoteCreateSpec("note_create", toolutil.RouteAction(client, Create), "gitlab_mr_note_create"),
		mrNoteReadSpec("note_list", toolutil.RouteAction(client, List), "gitlab_mr_notes_list"),
		mrNoteReadSpec("note_get", toolutil.RouteAction(client, GetNote), "gitlab_mr_note_get"),
		mrNoteUpdateSpec("note_update", toolutil.RouteAction(client, Update), "gitlab_mr_note_update"),
		mrNoteDeleteSpec("note_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_mr_note_delete"),
	}
}

func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult(fmt.Sprintf("note %d from MR !%d in project %s", input.NoteID, input.MRIID, input.ProjectID))
	return out, nil
}

func mrNoteReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, mrNoteOptions(individualTool))
}

func mrNoteCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, mrNoteOptions(individualTool))
}

func mrNoteUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, mrNoteOptions(individualTool))
}

func mrNoteDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, mrNoteOptions(individualTool))
}

// Canonical action IDs referenced by merge request note RelatedActions. The MR
// note actions are projected under the gitlab_merge_request catalog group, so
// their base domain is "merge_request".
const (
	actionMRGet              = "merge_request.get"
	actionMRNoteList         = "merge_request.note_list"
	actionMRNoteGet          = "merge_request.note_get"
	actionMRNoteCreate       = "merge_request.note_create"
	actionMRNoteUpdate       = "merge_request.note_update"
	actionMRNoteDelete       = "merge_request.note_delete"
	actionMRDiscussionList   = "merge_request.discussion_list"
	actionMRDiscussionCreate = "merge_request.discussion_create"
	actionMRDraftNoteCreate  = "merge_request.draft_note_create"
)

// projectIDGuidance is the shared parameter guidance for the project_id input
// that every merge request note action accepts.
func projectIDGuidance() toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:     "scope_project",
		ValueSource:      "Project ID or full namespace path that owns the merge request.",
		ExampleBinding:   `params.project_id:"group/project"`,
		CommonConfusions: []string{"Use the merge request's parent project here, not a group path or global MR ID."},
	}
}

// mrIIDGuidance is the shared parameter guidance for the merge_request_iid input.
func mrIIDGuidance() toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:     "merge_request_iid",
		ValueSource:      "Merge request number visible in the project, from the MR URL or prior MR list output.",
		ExampleBinding:   "params.merge_request_iid:7",
		CommonConfusions: []string{"Use the project-scoped MR IID (the !N number), not the global merge request database ID."},
	}
}

// noteIDGuidance is the shared parameter guidance for the note_id input.
func noteIDGuidance() toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:     "note_id",
		ValueSource:      "Numeric note ID from a prior merge_request.note_list or merge_request.note_create result.",
		ExampleBinding:   "params.note_id:200",
		CommonConfusions: []string{"note_id is the comment ID, not the merge_request_iid. Obtain it from merge_request.note_list."},
	}
}

// mrNoteOptions returns the [toolutil.ActionSpecOptions] for one merge request
// note action, filled with non-generic Usage, natural-language Aliases,
// RelatedActions, ParameterGuidance, and the "Returns: … See also: …"
// individual-tool description (1:1 audit R-META).
func mrNoteOptions(individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases:        []string{individualTool},
		Usage:          "Use to execute mrnotes domain action.",
		Tags:           []string{"merge_request", "review", "note"},
		OpenWorld:      true,
		OwnerPackage:   "mrnotes",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	decorateMRNoteMeta(&options, individualTool)
	return options
}

// decorateMRNoteMeta fills the per-action discovery metadata for each merge
// request note individual tool, mirroring the style of the issuenotes package.
func decorateMRNoteMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	switch individualTool {
	case "gitlab_mr_note_create":
		options.Usage = "Add a general comment (note) to a merge request. Use when the task asks to comment on, reply to, or annotate an existing MR. Set internal=true for a confidential note visible only to project members."
		options.Aliases = []string{"comment on merge request", "add mr comment", "reply to merge request", "post mr note"}
		options.RelatedActions = []string{actionMRGet, actionMRNoteList, actionMRDiscussionCreate, actionMRDraftNoteCreate}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id":        projectIDGuidance(),
			"merge_request_iid": mrIIDGuidance(),
			"body": {
				ValueSource:      "The comment text the user wants to post. Markdown is supported.",
				ExampleBinding:   `params.body:"LGTM, merging now."`,
				CommonConfusions: []string{"body is the comment content, not an MR description update. Updating the MR body is merge_request.update."},
			},
			"internal": {
				ValueSource:      "Set true only when the user asks for a private/internal note.",
				CommonConfusions: []string{"internal notes are visible to project members only. Do not set for public replies."},
			},
		}
		options.IndividualTool.Description = "Add a general comment (note) to a merge request. Returns: the created note with id, author, body, timestamps, and resolvable/internal flags. See also: gitlab_mr_notes_list, gitlab_mr_get, gitlab_mr_discussion_create."
	case "gitlab_mr_notes_list":
		options.Usage = "List all comments (notes) on a merge request, including system notes. Use when the task asks to read an MR's discussion, recent comments, or activity. Supports order_by, sort, and keyset pagination."
		options.Aliases = []string{"list mr comments", "show merge request notes", "read mr discussion", "get merge request comments"}
		options.RelatedActions = []string{actionMRNoteGet, actionMRNoteCreate, actionMRGet, actionMRDiscussionList}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id":        projectIDGuidance(),
			"merge_request_iid": mrIIDGuidance(),
			"order_by": {
				ValueSource:      "Field to order notes by: created_at or updated_at.",
				ExampleBinding:   `params.order_by:"created_at"`,
				CommonConfusions: []string{"Combine order_by with sort. Pass the field name, not a phrase like 'newest first'."},
			},
		}
		options.IndividualTool.Description = "List all notes (comments) on a merge request. Returns: notes with author, body, system/internal flags, and pagination metadata. See also: gitlab_mr_note_get, gitlab_mr_note_create, gitlab_mr_discussion_list."
		// https://docs.gitlab.com/api/notes/ (list all merge request notes): notes order by created_at or updated_at only.
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaEnumOverride("order_by", "created_at", "updated_at"),
		}
	case "gitlab_mr_note_get":
		options.Usage = "Get one merge request note by params.note_id. Use when the task references a specific comment or note ID on an MR."
		options.Aliases = []string{"get mr comment", "show merge request note", "fetch mr note"}
		options.RelatedActions = []string{actionMRNoteList, actionMRNoteUpdate, actionMRNoteDelete}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id":        projectIDGuidance(),
			"merge_request_iid": mrIIDGuidance(),
			"note_id":           noteIDGuidance(),
		}
		options.IndividualTool.Description = "Get a single merge request note by its ID. Returns: the note with author, body, timestamps, position, and resolvable/internal flags. See also: gitlab_mr_notes_list, gitlab_mr_note_update, gitlab_mr_note_delete."
	case "gitlab_mr_note_update":
		options.Usage = "Replace the body of an existing merge request note. Only the original author can edit a note. System notes cannot be edited. Use when the task asks to edit, fix, or amend a comment."
		options.Aliases = []string{"edit mr comment", "update merge request note", "amend mr comment"}
		options.RelatedActions = []string{actionMRNoteGet, actionMRNoteList, actionMRNoteDelete}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id":        projectIDGuidance(),
			"merge_request_iid": mrIIDGuidance(),
			"note_id":           noteIDGuidance(),
			"body": {
				ValueSource:      "The new comment text that replaces the existing note body. Markdown is supported.",
				ExampleBinding:   `params.body:"Updated: addressed in latest push."`,
				CommonConfusions: []string{"This replaces the whole note body. It does not append to it."},
			},
		}
		options.IndividualTool.Description = "Update a merge request note's body. Returns: the updated note with new body and updated_at timestamp. See also: gitlab_mr_note_get, gitlab_mr_notes_list, gitlab_mr_note_delete."
	case "gitlab_mr_note_delete":
		options.Usage = "Permanently delete a merge request note. Destructive and irreversible. Only the note author or a project Maintainer can delete a note. System notes cannot be deleted. Requires explicit confirmation."
		options.Aliases = []string{"delete mr comment", "remove merge request note", "delete merge request note"}
		options.RelatedActions = []string{actionMRNoteGet, actionMRNoteList, actionMRDiscussionList}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id":        projectIDGuidance(),
			"merge_request_iid": mrIIDGuidance(),
			"note_id":           noteIDGuidance(),
		}
		options.IndividualTool.Description = "Delete a merge request note permanently. Returns: a success confirmation naming the note, MR, and project. See also: gitlab_mr_note_get, gitlab_mr_notes_list."
	}
}
