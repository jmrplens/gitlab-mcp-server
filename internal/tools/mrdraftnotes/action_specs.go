// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package mrdraftnotes

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical catalog IDs for the draft note actions, and the one block every
// file in this package reads: the related-action metadata below and the result
// hints in markdown.go alike. These specs are aggregated into the
// gitlab_mr_review catalog group, so the domain is mr_review and never the
// owner package name. markdown.go used to keep a second block of its own
// spelling the domain correctly while these spelled it "mrdraftnotes", so the
// served hints resolved and every related action here named an action no
// surface could execute.
const (
	actionDraftNoteList       = "mr_review.draft_note_list"
	actionDraftNoteGet        = "mr_review.draft_note_get"
	actionDraftNoteCreate     = "mr_review.draft_note_create"
	actionDraftNoteUpdate     = "mr_review.draft_note_update"
	actionDraftNoteDelete     = "mr_review.draft_note_delete"
	actionDraftNotePublish    = "mr_review.draft_note_publish"
	actionDraftNotePublishAll = "mr_review.draft_note_publish_all"
)

const (
	paramMergeRequestIID = "merge_request_iid"
	paramProjectID       = "project_id"
	paramNoteID          = "note_id"
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
	options := toolutil.ActionSpecOptions{
		Aliases:        []string{individualTool},
		Usage:          "Use to execute mrdraftnotes domain action.",
		Tags:           []string{"merge_request", "review", "draft_note"},
		OpenWorld:      true,
		OwnerPackage:   "mrdraftnotes",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	switch individualTool {
	case "gitlab_mr_draft_note_publish_all":
		// GitLab declares the two values on the bulk_publish route and refuses
		// any other with a 400; approving is a separate action.
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaEnumOverride("reviewer_state", "requested_changes", "reviewed"),
		}
	case "gitlab_mr_draft_note_create", "gitlab_mr_draft_note_update":
		// The create and update routes share the positional params of
		// lib/api/draft_notes.rb, which declare position_type with the three
		// values the merge request discussion route declares, and refuse any
		// other with a 400.
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaEnumOverride("position.position_type",
				toolutil.PositionTypeText, toolutil.PositionTypeImage, toolutil.PositionTypeFile),
		}
		if individualTool == "gitlab_mr_draft_note_update" {
			// An update changes the body, the position or both, and keeps
			// the one it is not given (Update), so one of the two is needed.
			options.InputSchemaOverrides = append(options.InputSchemaOverrides,
				toolutil.SchemaAnyOfRequired("note", "position"))
		}
	}
	return options
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

// positionValueSource says where the create and the update take each part of
// a position from. mr_review.changes_get lists the files and lines of the
// diff and none of its SHAs, which the merge request answers as its
// diff_refs, and a file outside the diff, which an image or file position may
// name, is listed by the repository tree.
const positionValueSource = "Inline diff location for a review comment. base_sha, start_sha and head_sha are the diff_refs of merge_request.get. The file's old_path and new_path, and a line for a text position, come from mr_review.changes_get. An image or file position may name a file outside the diff, whose path repository.tree lists at head_sha."

// positionPathsConfusion asks the create and the update for both of the
// file's paths, and says which positions are refused with one. GitLab
// documents both for a text position (doc/api/draft_notes.md) and takes one,
// but looks the file's diff up by the paths the position carries, where git
// pairs a rename only when both of its paths are given, so a text position
// on a renamed file is refused without both
// (toolutil.ValidateRenamedFilePosition). A file position needs both
// whatever the file (toolutil.ValidateFilePosition), and a text or image
// position on a file that was not renamed passes with either. A file outside
// the diff, which only an image or file position may name, has no entry in
// mr_review.changes_get, and is unchanged, so the one path repository.tree
// lists for it is both.
const positionPathsConfusion = "Send old_path and new_path, both from the file's entry in mr_review.changes_get, which repeats the path for a file that was not renamed. A file outside the diff has no entry there, so the path repository.tree lists at head_sha goes in both. A file position needs both, and so does a text position on a file the diff renames, since GitLab finds a renamed file's lines only by both. A text or image position on a file that was not renamed is taken with one, although GitLab documents both for a text position."

// noteIDGuidance is the shared parameter guidance for note_id.
var noteIDGuidance = toolutil.ParameterGuidance{
	SemanticRole:     "draft_note_id",
	ValueSource:      "Draft note ID from a prior mr_review.draft_note_list response.",
	ExampleBinding:   "params.note_id:10",
	CommonConfusions: []string{"Draft notes are author-private until published. List them first to obtain a valid note_id."},
}

// draftNoteActionMeta maps each individual draft note tool to its discovery metadata.
var draftNoteActionMeta = map[string]toolutil.ActionMetaEntry{
	"gitlab_mr_draft_note_list": {
		Usage:   "List the pending (unpublished) draft review notes on a merge request. Use before publishing to review what will be posted, or to find a draft note's ID for get/update/delete.",
		Aliases: []string{"list draft notes", "show pending mr review comments", "list unpublished mr notes"},
		Related: []string{actionDraftNoteGet, actionDraftNoteCreate, actionDraftNotePublishAll},
		Guidance: map[string]toolutil.ParameterGuidance{
			paramProjectID:       projectGuidance,
			paramMergeRequestIID: mrIIDGuidance,
		},
		Description: "List pending draft notes on a merge request. Returns: each draft note with author, body, optional inline position, line code, and pagination metadata. See also: gitlab_mr_draft_note_get, gitlab_mr_draft_note_create, gitlab_mr_draft_note_publish_all.",
	},
	"gitlab_mr_draft_note_get": {
		Usage:   "Get one pending draft note by ID, including its full body and any inline diff position. Use after list to inspect a specific draft before updating or publishing it.",
		Aliases: []string{"get draft note", "show draft note details", "fetch pending mr comment"},
		Related: []string{actionDraftNoteList, actionDraftNoteUpdate, actionDraftNotePublish},
		Guidance: map[string]toolutil.ParameterGuidance{
			paramProjectID:       projectGuidance,
			paramMergeRequestIID: mrIIDGuidance,
			paramNoteID:          noteIDGuidance,
		},
		Description: "Get a single draft note from a merge request. Returns: the draft note with author, body, resolve flag, line code, and inline diff position. See also: gitlab_mr_draft_note_list, gitlab_mr_draft_note_update, gitlab_mr_draft_note_publish.",
	},
	"gitlab_mr_draft_note_create": {
		Usage:   "Create a pending draft review note on a merge request. Omit position for a general comment. A position takes base_sha/start_sha/head_sha from the MR diff refs and the file's old_path and new_path from mr_review.changes_get, or both from repository.tree for a file outside the diff. A text position (the default) also needs a line of the diff: new_line for an added line, old_line for a removed one, both for an unchanged one. An image position needs width, height, x and y in whole pixels. A file position needs both old_path and new_path. The note stays author-private until published.",
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
				ValueSource:      positionValueSource,
				CommonConfusions: []string{"Omit position entirely for a general MR comment. A position missing start_sha, head_sha, a file path or a field its position_type needs is refused before any request, since GitLab would save the draft and publish it as a general comment with the position dropped.", positionPathsConfusion},
			},
		},
		Description: "Create a pending draft note on a merge request. Returns: the created draft note with ID, author, body, and inline position. See also: gitlab_mr_draft_note_list, gitlab_mr_draft_note_publish, gitlab_mr_draft_note_publish_all.",
	},
	"gitlab_mr_draft_note_update": {
		Usage:   "Edit the body, the inline position or both of a pending draft note before it is published. Give note, position or both. The one left out is kept as the draft has it. Only the draft's author can update it.",
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
			"position": {
				SemanticRole:     "diff_position",
				ValueSource:      positionValueSource,
				CommonConfusions: []string{"A position given replaces the draft's whole position, so it carries every field one needs: base_sha, start_sha and head_sha, the file's path, and by position_type a line of the diff (text, the default: new_line for an added line, old_line for a removed one, both for an unchanged one), width, height, x and y in whole pixels (image), or both old_path and new_path (file). One missing a field is refused before any request. Omit position to keep the draft's current one.", positionPathsConfusion},
			},
		},
		Description: "Update a pending draft note's body, position or both, keeping the one left out. Returns: the updated draft note. See also: gitlab_mr_draft_note_get, gitlab_mr_draft_note_publish.",
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
		Usage:   "Publish all of the current user's pending draft notes on a merge request in one call, submitting a complete review. Optionally post a summary note with it (note, internal) and record the caller's reviewer state (reviewer_state: requested_changes or reviewed, which is not an approval). Cannot be undone. Review with mr_review.draft_note_list first.",
		Aliases: []string{"publish all draft notes", "submit mr review", "post all pending comments", "finish mr review", "submit review with summary", "request changes on mr review"},
		Related: []string{actionDraftNoteList, actionDraftNotePublish, "merge_request.approve"},
		Guidance: map[string]toolutil.ParameterGuidance{
			paramProjectID:       projectGuidance,
			paramMergeRequestIID: mrIIDGuidance,
			"note": {
				SemanticRole:     "comment_body",
				ValueSource:      "The Markdown summary of the review the user wants posted once the drafts are published.",
				ExampleBinding:   `params.note:"Two blocking comments inline; the rest are nits."`,
				CommonConfusions: []string{"This is one extra note beside the drafts. It does not edit or replace any draft note."},
			},
			"reviewer_state": {
				SemanticRole:     "review_state",
				ValueSource:      "requested_changes when the review blocks the merge, reviewed when it does not.",
				ExampleBinding:   `params.reviewer_state:"requested_changes"`,
				CommonConfusions: []string{"reviewed is not an approval. Approve with merge_request.approve instead."},
			},
		},
		Description: "Publish all of the user's pending draft notes on a merge request, optionally with a summary note and a reviewer state. Returns: a success confirmation naming the merge request and project. See also: gitlab_mr_draft_note_list, gitlab_mr_draft_note_publish.",
	},
}
