package epicnotes

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for epic note actions exposed as MCP
// tools. The list/get/create/update/delete routes are projected into the
// dynamic, meta, individual, and audit surfaces by the action catalog
// (ADR-0004). Epic notes are a Premium/Ultimate (Edition: premium) capability
// served through the Work Items GraphQL API.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_epic_note_list — list all notes attached to an epic.
		epicNoteReadSpec("epic_note_list", toolutil.RouteAction(client, List), "gitlab_epic_note_list"),
		// gitlab_epic_note_get — fetch a single epic note by its ID.
		epicNoteReadSpec("epic_note_get", toolutil.RouteAction(client, Get), "gitlab_epic_note_get"),
		// gitlab_epic_note_create — add a comment (note) to an epic.
		epicNoteCreateSpec("epic_note_create", toolutil.RouteAction(client, Create), "gitlab_epic_note_create"),
		// gitlab_epic_note_update — replace the body of an existing epic note.
		epicNoteUpdateSpec("epic_note_update", toolutil.RouteAction(client, Update), "gitlab_epic_note_update"),
		// gitlab_epic_note_delete — remove an epic note (destructive, requires confirmation).
		epicNoteDeleteSpec("epic_note_delete", toolutil.DestructiveAction(client, DeleteOutput), "gitlab_epic_note_delete"),
	}
}

// DeleteOutput deletes an epic note and returns the canonical success message shape.
func DeleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{
		Status:  "success",
		Message: fmt.Sprintf("Successfully deleted note %d from epic &%d in group %s.", input.NoteID, input.IID, input.FullPath),
	}, nil
}

// epicNoteReadSpec builds a read-only [toolutil.ActionSpec] for an epic note
// action, applying the package's default [epicNoteOptions].
func epicNoteReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, epicNoteOptions(individualTool))
}

// epicNoteCreateSpec builds a create-style [toolutil.ActionSpec] for an epic
// note action, applying the package's default [epicNoteOptions].
func epicNoteCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, epicNoteOptions(individualTool))
}

// epicNoteUpdateSpec builds an update-style [toolutil.ActionSpec] for an epic
// note action, applying the package's default [epicNoteOptions].
func epicNoteUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, epicNoteOptions(individualTool))
}

// epicNoteDeleteSpec builds a destructive [toolutil.ActionSpec] for an epic
// note action, applying the package's default [epicNoteOptions].
func epicNoteDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, epicNoteOptions(individualTool))
}

// Canonical action IDs referenced by epic note RelatedActions. The epic note
// actions are projected under the gitlab_group catalog group, so their base
// domain is "group".
const (
	actionEpicGet           = "group.epic_get"
	actionEpicNoteList      = "group.epic_note_list"
	actionEpicNoteGet       = "group.epic_note_get"
	actionEpicNoteCreate    = "group.epic_note_create"
	actionEpicNoteUpdate    = "group.epic_note_update"
	actionEpicNoteDelete    = "group.epic_note_delete"
	actionEpicDiscussionGet = "group.epic_discussion_list"
)

// fullPathGuidance is the shared parameter guidance for the full_path input that
// every epic note action accepts.
func fullPathGuidance() toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:     "scope_group",
		ValueSource:      "Full group path that owns the epic, e.g. my-group or my-group/sub-group.",
		ExampleBinding:   `params.full_path:"my-group"`,
		CommonConfusions: []string{"Use the owning group path, not a project path or a numeric group ID."},
	}
}

// epicIIDGuidance is the shared parameter guidance for the epic_iid input.
func epicIIDGuidance() toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:     "epic_iid",
		ValueSource:      "Epic number visible in the group, from the epic URL (the &N reference) or a prior epic list result.",
		ExampleBinding:   "params.epic_iid:42",
		CommonConfusions: []string{"Use the group-scoped epic IID (the &N number), not the global epic database ID."},
	}
}

// noteIDGuidance is the shared parameter guidance for the note_id input.
func noteIDGuidance() toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:     "note_id",
		ValueSource:      "Numeric note ID from a prior group.epic_note_list or group.epic_note_create result.",
		ExampleBinding:   "params.note_id:100",
		CommonConfusions: []string{"note_id is the comment ID, not the epic_iid; obtain it from group.epic_note_list."},
	}
}

// epicNoteOptions returns the [toolutil.ActionSpecOptions] for one epic note
// action, filled with non-generic Usage, natural-language Aliases,
// RelatedActions, ParameterGuidance, and the "Returns: … See also: …"
// individual-tool description (1:1 audit R-META). Epic notes are gated to the
// Premium/Ultimate edition.
func epicNoteOptions(individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases:        []string{individualTool},
		Usage:          "Use to execute epicnotes domain action.",
		Tags:           []string{"group", "epic", "note"},
		RelatedActions: []string{actionEpicGet},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "epicnotes",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	decorateEpicNoteMeta(&options, individualTool)
	return options
}

// decorateEpicNoteMeta fills the per-action discovery metadata for each epic
// note individual tool, mirroring the style of the issuenotes package.
func decorateEpicNoteMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	switch individualTool {
	case "gitlab_epic_note_list":
		options.Usage = "List all comments (notes) on an epic, including system notes. Use when the task asks to read an epic's discussion, recent comments, or activity. Notes are served through the Work Items GraphQL API with keyset (cursor) pagination."
		options.Aliases = []string{"list epic comments", "show epic notes", "read epic discussion", "get epic comments"}
		options.RelatedActions = []string{actionEpicNoteGet, actionEpicNoteCreate, actionEpicGet, actionEpicDiscussionGet}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"full_path": fullPathGuidance(),
			"epic_iid":  epicIIDGuidance(),
		}
		options.IndividualTool.Description = "List all notes (comments) on an epic. Returns: notes with author, body, system flag, timestamps, and keyset pagination metadata. See also: gitlab_epic_note_get, gitlab_epic_note_create, gitlab_get_epic_discussion."
	case "gitlab_epic_note_get":
		options.Usage = "Get one epic note by params.note_id. Use when the task references a specific comment or note ID on an epic."
		options.Aliases = []string{"get epic comment", "show epic note", "fetch epic note"}
		options.RelatedActions = []string{actionEpicNoteList, actionEpicNoteUpdate, actionEpicNoteDelete}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"full_path": fullPathGuidance(),
			"epic_iid":  epicIIDGuidance(),
			"note_id":   noteIDGuidance(),
		}
		options.IndividualTool.Description = "Get a single epic note by its ID. Returns: the note with author, body, timestamps, and system flag. See also: gitlab_epic_note_list, gitlab_epic_note_update, gitlab_epic_note_delete."
	case "gitlab_epic_note_create":
		options.Usage = "Add a comment (note) to an epic. Use when the task asks to comment on, reply to, or annotate an existing epic. The body is rendered as GitLab Flavored Markdown."
		options.Aliases = []string{"comment on epic", "add epic comment", "reply to epic", "post epic note"}
		options.RelatedActions = []string{actionEpicGet, actionEpicNoteList, actionEpicDiscussionGet}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"full_path": fullPathGuidance(),
			"epic_iid":  epicIIDGuidance(),
			"body": {
				ValueSource:      "The comment text the user wants to post; Markdown is supported.",
				ExampleBinding:   `params.body:"Thanks, closing this epic."`,
				CommonConfusions: []string{"body is the comment content, not a description update; updating the epic body is group.epic_update."},
			},
		}
		options.IndividualTool.Description = "Add a comment (note) to an epic. Returns: the created note with id, author, body, and timestamps. See also: gitlab_epic_note_list, gitlab_epic_get, gitlab_create_epic_discussion."
	case "gitlab_epic_note_update":
		options.Usage = "Replace the body of an existing epic note. Only the original author or a Maintainer/Owner can edit a note; system notes cannot be edited. Use when the task asks to edit, fix, or amend a comment."
		options.Aliases = []string{"edit epic comment", "update epic note", "amend epic comment"}
		options.RelatedActions = []string{actionEpicNoteGet, actionEpicNoteList, actionEpicNoteDelete}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"full_path": fullPathGuidance(),
			"epic_iid":  epicIIDGuidance(),
			"note_id":   noteIDGuidance(),
			"body": {
				ValueSource:      "The new comment text that replaces the existing note body; Markdown is supported.",
				ExampleBinding:   `params.body:"Updated: resolved in &123."`,
				CommonConfusions: []string{"This replaces the whole note body; it does not append to it."},
			},
		}
		options.IndividualTool.Description = "Update an epic note's body. Returns: the updated note with new body and updated_at timestamp. See also: gitlab_epic_note_get, gitlab_epic_note_list, gitlab_epic_note_delete."
	case "gitlab_epic_note_delete":
		options.Usage = "Permanently delete an epic note. Destructive and irreversible. Only the note author or a group Maintainer/Owner can delete a note; system notes cannot be deleted. Requires explicit confirmation."
		options.Aliases = []string{"delete epic comment", "remove epic note", "delete epic note"}
		options.RelatedActions = []string{actionEpicNoteGet, actionEpicNoteList, actionEpicGet}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"full_path": fullPathGuidance(),
			"epic_iid":  epicIIDGuidance(),
			"note_id":   noteIDGuidance(),
		}
		options.IndividualTool.Description = "Delete an epic note permanently. Returns: a success confirmation naming the note, epic, and group. See also: gitlab_epic_note_get, gitlab_epic_note_list."
	}
}
