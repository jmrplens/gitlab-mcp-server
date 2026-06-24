package mrdraftnotes

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionDraftNoteList = "mrdraftnotes.draft_note_list"
	actionDraftNoteGet  = "mrdraftnotes.draft_note_get"
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
	decorateDraftNoteMeta(&options, individualTool)
	return toolutil.NewReadActionSpec(name, route, options)
}

func mrDraftNoteCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := mrDraftNoteOptions(individualTool)
	decorateDraftNoteMeta(&options, individualTool)
	return toolutil.NewCreateActionSpec(name, route, options)
}

func mrDraftNoteUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := mrDraftNoteOptions(individualTool)
	decorateDraftNoteMeta(&options, individualTool)
	return toolutil.NewUpdateActionSpec(name, route, options)
}

func mrDraftNoteDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := mrDraftNoteOptions(individualTool)
	decorateDraftNoteMeta(&options, individualTool)
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

// draftNoteMetaEntry is the discovery metadata for one draft note action.
type draftNoteMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	guidance    map[string]toolutil.ParameterGuidance
	description string
}

// decorateDraftNoteMeta fills non-generic Usage, natural-language Aliases,
// canonical RelatedActions, ParameterGuidance, and the "Returns: … See also: …"
// individual-tool description for each draft note action (1:1 audit R-META).
func decorateDraftNoteMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	meta, ok := draftNoteActionMeta[individualTool]
	if !ok {
		return
	}
	if meta.usage != "" {
		options.Usage = meta.usage
	}
	if len(meta.aliases) > 0 {
		options.Aliases = append([]string(nil), meta.aliases...)
	}
	if len(meta.related) > 0 {
		options.RelatedActions = append([]string(nil), meta.related...)
	}
	if len(meta.guidance) > 0 {
		options.ParameterGuidance = meta.guidance
	}
	if meta.description != "" {
		options.IndividualTool.Description = meta.description
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
	SemanticRole:     "merge_request_iid",
	ValueSource:      "Merge request number visible in the project, from the URL or a prior MR list.",
	ExampleBinding:   "params.merge_request_iid:7",
	CommonConfusions: []string{"Use the project-scoped merge_request_iid, not the global merge_request_id."},
}

// noteIDGuidance is the shared parameter guidance for note_id.
var noteIDGuidance = toolutil.ParameterGuidance{
	SemanticRole:     "draft_note_id",
	ValueSource:      "Draft note ID from a prior gitlab_mr_draft_note_list response.",
	ExampleBinding:   "params.note_id:10",
	CommonConfusions: []string{"Draft notes are author-private until published; list them first to obtain a valid note_id."},
}

// draftNoteActionMeta maps each individual draft note tool to its discovery metadata.
var draftNoteActionMeta = map[string]draftNoteMetaEntry{
	"gitlab_mr_draft_note_list": {
		usage:   "List the pending (unpublished) draft review notes on a merge request. Use before publishing to review what will be posted, or to find a draft note's ID for get/update/delete.",
		aliases: []string{"list draft notes", "show pending mr review comments", "list unpublished mr notes"},
		related: []string{actionDraftNoteGet, "mrdraftnotes.draft_note_create", "mrdraftnotes.draft_note_publish_all"},
		guidance: map[string]toolutil.ParameterGuidance{
			"project_id":        projectGuidance,
			"merge_request_iid": mrIIDGuidance,
		},
		description: "List pending draft notes on a merge request. Returns: each draft note with author, body, optional inline position, line code, and pagination metadata. See also: gitlab_mr_draft_note_get, gitlab_mr_draft_note_create, gitlab_mr_draft_note_publish_all.",
	},
	"gitlab_mr_draft_note_get": {
		usage:   "Get one pending draft note by ID, including its full body and any inline diff position. Use after list to inspect a specific draft before updating or publishing it.",
		aliases: []string{"get draft note", "show draft note details", "fetch pending mr comment"},
		related: []string{actionDraftNoteList, "mrdraftnotes.draft_note_update", "mrdraftnotes.draft_note_publish"},
		guidance: map[string]toolutil.ParameterGuidance{
			"project_id":        projectGuidance,
			"merge_request_iid": mrIIDGuidance,
			"note_id":           noteIDGuidance,
		},
		description: "Get a single draft note from a merge request. Returns: the draft note with author, body, resolve flag, line code, and inline diff position. See also: gitlab_mr_draft_note_list, gitlab_mr_draft_note_update, gitlab_mr_draft_note_publish.",
	},
	"gitlab_mr_draft_note_create": {
		usage:   "Create a pending draft review note on a merge request. Omit position for a general comment; include position with base_sha/start_sha/head_sha plus new_path/old_path and new_line/old_line to anchor the note to a specific diff line. The note stays author-private until published.",
		aliases: []string{"add draft note", "draft mr review comment", "create pending mr comment", "comment on mr diff line as draft"},
		related: []string{actionDraftNoteList, "mrdraftnotes.draft_note_publish", "mrdraftnotes.draft_note_publish_all"},
		guidance: map[string]toolutil.ParameterGuidance{
			"project_id":        projectGuidance,
			"merge_request_iid": mrIIDGuidance,
			"note": {
				SemanticRole:   "comment_body",
				ValueSource:    "The Markdown review comment the user wants to draft.",
				ExampleBinding: `params.note:"Consider extracting this into a helper."`,
			},
			"position": {
				SemanticRole:     "diff_position",
				ValueSource:      "Inline diff location for a code comment; mirror the MR diff SHAs and the target file path and line.",
				CommonConfusions: []string{"Omit position entirely for a general MR comment; only set it to anchor the note to a changed line."},
			},
		},
		description: "Create a pending draft note on a merge request. Returns: the created draft note with ID, author, body, and inline position. See also: gitlab_mr_draft_note_list, gitlab_mr_draft_note_publish, gitlab_mr_draft_note_publish_all.",
	},
	"gitlab_mr_draft_note_update": {
		usage:   "Edit the body or inline position of a pending draft note before it is published. Only the draft's author can update it.",
		aliases: []string{"edit draft note", "update pending mr comment", "modify draft review note"},
		related: []string{actionDraftNoteGet, actionDraftNoteList, "mrdraftnotes.draft_note_publish"},
		guidance: map[string]toolutil.ParameterGuidance{
			"project_id":        projectGuidance,
			"merge_request_iid": mrIIDGuidance,
			"note_id":           noteIDGuidance,
			"note": {
				SemanticRole:   "comment_body",
				ValueSource:    "The revised Markdown body for the draft note.",
				ExampleBinding: `params.note:"Updated: use the shared helper instead."`,
			},
		},
		description: "Update a pending draft note's body or position. Returns: the updated draft note. See also: gitlab_mr_draft_note_get, gitlab_mr_draft_note_publish.",
	},
	"gitlab_mr_draft_note_delete": {
		usage:   "Discard a pending draft note so it is never published. Destructive and irreversible; only the draft's author can delete it.",
		aliases: []string{"delete draft note", "discard pending mr comment", "remove draft review note"},
		related: []string{actionDraftNoteList, actionDraftNoteGet},
		guidance: map[string]toolutil.ParameterGuidance{
			"project_id":        projectGuidance,
			"merge_request_iid": mrIIDGuidance,
			"note_id":           noteIDGuidance,
		},
		description: "Delete a pending draft note permanently. Returns: a success confirmation naming the note, merge request, and project. See also: gitlab_mr_draft_note_list, gitlab_mr_draft_note_get.",
	},
	"gitlab_mr_draft_note_publish": {
		usage:   "Publish one pending draft note, turning it into a regular merge request note visible to everyone. Cannot be undone; only the draft's author can publish it.",
		aliases: []string{"publish draft note", "post draft mr comment", "submit single review comment"},
		related: []string{actionDraftNoteList, "mrdraftnotes.draft_note_publish_all", actionDraftNoteGet},
		guidance: map[string]toolutil.ParameterGuidance{
			"project_id":        projectGuidance,
			"merge_request_iid": mrIIDGuidance,
			"note_id":           noteIDGuidance,
		},
		description: "Publish a single draft note as a regular merge request note. Returns: a success confirmation naming the note, merge request, and project. See also: gitlab_mr_draft_note_publish_all, gitlab_mr_draft_note_list.",
	},
	"gitlab_mr_draft_note_publish_all": {
		usage:   "Publish all of the current user's pending draft notes on a merge request in one call, submitting a complete review. Cannot be undone; review with gitlab_mr_draft_note_list first.",
		aliases: []string{"publish all draft notes", "submit mr review", "post all pending comments", "finish mr review"},
		related: []string{actionDraftNoteList, "mrdraftnotes.draft_note_publish"},
		guidance: map[string]toolutil.ParameterGuidance{
			"project_id":        projectGuidance,
			"merge_request_iid": mrIIDGuidance,
		},
		description: "Publish all of the user's pending draft notes on a merge request. Returns: a success confirmation naming the merge request and project. See also: gitlab_mr_draft_note_list, gitlab_mr_draft_note_publish.",
	},
}
