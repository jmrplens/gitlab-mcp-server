package mrdraftnotes

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionDraftNoteList       = "mrdraftnotes.draft_note_list"
	actionDraftNoteGet        = "mrdraftnotes.draft_note_get"
	actionDraftNotePublish    = "mrdraftnotes.draft_note_publish"
	actionDraftNotePublishAll = "mrdraftnotes.draft_note_publish_all"
	paramMergeRequestIID      = "merge_request_iid"
	paramProjectID            = "project_id"
	paramNoteID               = "note_id"
)

// ActionSpecs returns canonical specs for merge request draft note actions.
// The list, get, create, update, delete, publish, and publish_all routes are
// projected into the dynamic, meta, individual, and audit surfaces by the
// action catalog (ADR-0004).
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_mr_draft_note_list — list pending draft review notes on an MR.
		mrDraftNoteReadSpec("draft_note_list", toolutil.RouteAction(client, List), "gitlab_mr_draft_note_list"),
		// gitlab_mr_draft_note_get — fetch a single draft note by ID.
		mrDraftNoteReadSpec("draft_note_get", toolutil.RouteAction(client, Get), "gitlab_mr_draft_note_get"),
		// gitlab_mr_draft_note_create — create a pending draft review note.
		mrDraftNoteCreateSpec("draft_note_create", toolutil.RouteAction(client, Create), "gitlab_mr_draft_note_create"),
		// gitlab_mr_draft_note_update — edit a pending draft note before publishing.
		mrDraftNoteUpdateSpec("draft_note_update", toolutil.RouteAction(client, Update), "gitlab_mr_draft_note_update"),
		// gitlab_mr_draft_note_delete — discard a pending draft note (destructive).
		mrDraftNoteDeleteSpec("draft_note_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_mr_draft_note_delete"),
		// gitlab_mr_draft_note_publish — publish one draft note as a regular MR note.
		mrDraftNoteUpdateSpec("draft_note_publish", toolutil.RouteAction(client, publishOutput), "gitlab_mr_draft_note_publish"),
		// gitlab_mr_draft_note_publish_all — publish all pending draft notes at once.
		mrDraftNoteUpdateSpec("draft_note_publish_all", toolutil.RouteAction(client, publishAllOutput), "gitlab_mr_draft_note_publish_all"),
	}
}

func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult(fmt.Sprintf("draft note %d from MR !%d in project %s", input.NoteID, input.MRIID, input.ProjectID))
	return out, nil
}

func publishOutput(ctx context.Context, client *gitlabclient.Client, input PublishInput) (toolutil.DeleteOutput, error) {
	if err := Publish(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: fmt.Sprintf("Draft note %d published on MR !%d in project %s", input.NoteID, input.MRIID, input.ProjectID)}, nil
}

func publishAllOutput(ctx context.Context, client *gitlabclient.Client, input PublishAllInput) (toolutil.DeleteOutput, error) {
	if err := PublishAll(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: fmt.Sprintf("All draft notes published on MR !%d in project %s", input.MRIID, input.ProjectID)}, nil
}

func mrDraftNoteReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := mrDraftNoteOptions(individualTool)
	toolutil.ApplyActionMeta(&options, draftNoteActionMeta[individualTool])
	return toolutil.NewReadActionSpec(name, route, options)
}

func mrDraftNoteCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := mrDraftNoteOptions(individualTool)
	toolutil.ApplyActionMeta(&options, draftNoteActionMeta[individualTool])
	return toolutil.NewCreateActionSpec(name, route, options)
}

func mrDraftNoteUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := mrDraftNoteOptions(individualTool)
	toolutil.ApplyActionMeta(&options, draftNoteActionMeta[individualTool])
	return toolutil.NewUpdateActionSpec(name, route, options)
}

func mrDraftNoteDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := mrDraftNoteOptions(individualTool)
	toolutil.ApplyActionMeta(&options, draftNoteActionMeta[individualTool])
	return toolutil.NewDeleteActionSpec(name, route, options)
}

func mrDraftNoteOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases:        []string{individualTool},
		Usage:          "Use to execute mrdraftnotes domain action.",
		Tags:           []string{"merge_request", "review", "draft_note"},
		OpenWorld:      true,
		OwnerPackage:   "mrdraftnotes",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

// projectGuidance is the shared parameter guidance for project_id across draft
// note actions.
var projectGuidance = toolutil.ParameterGuidance{
	SemanticRole:     "scope_project",
	ValueSource:      "Project ID or full namespace path that owns the merge request.",
	ExampleBinding:   `params.project_id:"group/project"`,
	CommonConfusions: []string{"Use the merge request's parent project here, not a group path or global ID."},
}

// mrIIDGuidance is the shared parameter guidance for merge_request_iid.
var mrIIDGuidance = toolutil.ParameterGuidance{
	SemanticRole:     paramMergeRequestIID,
	ValueSource:      "Merge request number visible in the project, from the URL or a prior MR list.",
	ExampleBinding:   "params.merge_request_iid:7",
	CommonConfusions: []string{"Use the project-scoped merge_request_iid, not the global merge_request_id."},
}

// noteIDGuidance is the shared parameter guidance for note_id.
var noteIDGuidance = toolutil.ParameterGuidance{
	SemanticRole:     "draft_note_id",
	ValueSource:      "Draft note ID from a prior gitlab_mr_draft_note_list response.",
	ExampleBinding:   "params.note_id:10",
	CommonConfusions: []string{"Draft notes are author-private until published. List them first to obtain a valid note_id."},
}

// draftNoteActionMeta maps each individual draft note tool to its discovery metadata.
var draftNoteActionMeta = map[string]toolutil.ActionMetaEntry{
	"gitlab_mr_draft_note_list": {
		Usage:   "List the pending (unpublished) draft review notes on a merge request. Use before publishing to review what will be posted, or to find a draft note's ID for get/update/delete.",
		Aliases: []string{"list draft notes", "show pending mr review comments", "list unpublished mr notes"},
		Related: []string{actionDraftNoteGet, "mrdraftnotes.draft_note_create", actionDraftNotePublishAll},
		Guidance: map[string]toolutil.ParameterGuidance{
			paramProjectID:       projectGuidance,
			paramMergeRequestIID: mrIIDGuidance,
		},
		Description: "List pending draft notes on a merge request. Returns: each draft note with author, body, optional inline position, line code, and pagination metadata. See also: gitlab_mr_draft_note_get, gitlab_mr_draft_note_create, gitlab_mr_draft_note_publish_all.",
	},
	"gitlab_mr_draft_note_get": {
		Usage:   "Get one pending draft note by ID, including its full body and any inline diff position. Use after list to inspect a specific draft before updating or publishing it.",
		Aliases: []string{"get draft note", "show draft note details", "fetch pending mr comment"},
		Related: []string{actionDraftNoteList, "mrdraftnotes.draft_note_update", actionDraftNotePublish},
		Guidance: map[string]toolutil.ParameterGuidance{
			paramProjectID:       projectGuidance,
			paramMergeRequestIID: mrIIDGuidance,
			paramNoteID:          noteIDGuidance,
		},
		Description: "Get a single draft note from a merge request. Returns: the draft note with author, body, resolve flag, line code, and inline diff position. See also: gitlab_mr_draft_note_list, gitlab_mr_draft_note_update, gitlab_mr_draft_note_publish.",
	},
	"gitlab_mr_draft_note_create": {
		Usage:   "Create a pending draft review note on a merge request. Omit position for a general comment. Include position with base_sha/start_sha/head_sha plus new_path/old_path and new_line/old_line to anchor the note to a specific diff line. The note stays author-private until published.",
		Aliases: []string{"add draft note", "draft mr review comment", "create pending mr comment", "comment on mr diff line as draft"},
		Related: []string{actionDraftNoteList, actionDraftNotePublish, actionDraftNotePublishAll},
		Guidance: map[string]toolutil.ParameterGuidance{
			paramProjectID:       projectGuidance,
			paramMergeRequestIID: mrIIDGuidance,
			"note": {
				SemanticRole:   "comment_body",
				ValueSource:    "The Markdown review comment the user wants to draft.",
				ExampleBinding: `params.note:"Consider extracting this into a helper."`,
			},
			"position": {
				SemanticRole:     "diff_position",
				ValueSource:      "Inline diff location for a code comment. Mirror the MR diff SHAs and the target file path and line.",
				CommonConfusions: []string{"Omit position entirely for a general MR comment. Only set it to anchor the note to a changed line."},
			},
		},
		Description: "Create a pending draft note on a merge request. Returns: the created draft note with ID, author, body, and inline position. See also: gitlab_mr_draft_note_list, gitlab_mr_draft_note_publish, gitlab_mr_draft_note_publish_all.",
	},
	"gitlab_mr_draft_note_update": {
		Usage:   "Edit the body or inline position of a pending draft note before it is published. Only the draft's author can update it.",
		Aliases: []string{"edit draft note", "update pending mr comment", "modify draft review note"},
		Related: []string{actionDraftNoteGet, actionDraftNoteList, actionDraftNotePublish},
		Guidance: map[string]toolutil.ParameterGuidance{
			paramProjectID:       projectGuidance,
			paramMergeRequestIID: mrIIDGuidance,
			paramNoteID:          noteIDGuidance,
			"note": {
				SemanticRole:   "comment_body",
				ValueSource:    "The revised Markdown body for the draft note.",
				ExampleBinding: `params.note:"Updated: use the shared helper instead."`,
			},
		},
		Description: "Update a pending draft note's body or position. Returns: the updated draft note. See also: gitlab_mr_draft_note_get, gitlab_mr_draft_note_publish.",
	},
	"gitlab_mr_draft_note_delete": {
		Usage:   "Discard a pending draft note so it is never published. Destructive and irreversible. Only the draft's author can delete it.",
		Aliases: []string{"delete draft note", "discard pending mr comment", "remove draft review note"},
		Related: []string{actionDraftNoteList, actionDraftNoteGet},
		Guidance: map[string]toolutil.ParameterGuidance{
			paramProjectID:       projectGuidance,
			paramMergeRequestIID: mrIIDGuidance,
			paramNoteID:          noteIDGuidance,
		},
		Description: "Delete a pending draft note permanently. Returns: a success confirmation naming the note, merge request, and project. See also: gitlab_mr_draft_note_list, gitlab_mr_draft_note_get.",
	},
	"gitlab_mr_draft_note_publish": {
		Usage:   "Publish one pending draft note, turning it into a regular merge request note visible to everyone. Cannot be undone. Only the draft's author can publish it.",
		Aliases: []string{"publish draft note", "post draft mr comment", "submit single review comment"},
		Related: []string{actionDraftNoteList, actionDraftNotePublishAll, actionDraftNoteGet},
		Guidance: map[string]toolutil.ParameterGuidance{
			paramProjectID:       projectGuidance,
			paramMergeRequestIID: mrIIDGuidance,
			paramNoteID:          noteIDGuidance,
		},
		Description: "Publish a single draft note as a regular merge request note. Returns: a success confirmation naming the note, merge request, and project. See also: gitlab_mr_draft_note_publish_all, gitlab_mr_draft_note_list.",
	},
	"gitlab_mr_draft_note_publish_all": {
		Usage:   "Publish all of the current user's pending draft notes on a merge request in one call, submitting a complete review. Cannot be undone. Review with gitlab_mr_draft_note_list first.",
		Aliases: []string{"publish all draft notes", "submit mr review", "post all pending comments", "finish mr review"},
		Related: []string{actionDraftNoteList, actionDraftNotePublish},
		Guidance: map[string]toolutil.ParameterGuidance{
			paramProjectID:       projectGuidance,
			paramMergeRequestIID: mrIIDGuidance,
		},
		Description: "Publish all of the user's pending draft notes on a merge request. Returns: a success confirmation naming the merge request and project. See also: gitlab_mr_draft_note_list, gitlab_mr_draft_note_publish.",
	},
}
