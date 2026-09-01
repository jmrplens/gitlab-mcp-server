package snippetnotes

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for project snippet note actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_snippet_note_list — list notes (comments) on a project snippet.
		snippetNoteReadSpec("note_list", toolutil.RouteAction(client, List), "gitlab_snippet_note_list"),
		// gitlab_snippet_note_get — fetch a single snippet note by ID.
		snippetNoteReadSpec("note_get", toolutil.RouteAction(client, Get), "gitlab_snippet_note_get"),
		// gitlab_snippet_note_create — add a new Markdown note to a snippet.
		snippetNoteCreateSpec("note_create", toolutil.RouteAction(client, Create), "gitlab_snippet_note_create"),
		// gitlab_snippet_note_update — edit the body of an existing snippet note.
		snippetNoteUpdateSpec("note_update", toolutil.RouteAction(client, Update), "gitlab_snippet_note_update"),
		// gitlab_snippet_note_delete — delete a snippet note (destructive).
		snippetNoteDeleteSpec("note_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_snippet_note_delete"),
	}
}

// deleteOutput adapts the void [Delete] handler into the catalog DeleteOutput contract
// so it composes with [toolutil.DestructiveAction] and surfaces a confirmation message.
func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult(fmt.Sprintf("note %d from snippet %d in project %s", input.NoteID, input.SnippetID, input.ProjectID))
	return out, nil
}

// snippetNoteReadSpec builds the canonical read-only spec for a snippet note tool.
func snippetNoteReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, snippetNoteOptions(individualTool))
}

// snippetNoteCreateSpec builds the canonical create spec for a snippet note tool.
func snippetNoteCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, snippetNoteOptions(individualTool))
}

// snippetNoteUpdateSpec builds the canonical update spec for a snippet note tool.
func snippetNoteUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, snippetNoteOptions(individualTool))
}

// snippetNoteDeleteSpec builds the canonical destructive delete spec for a snippet note tool.
func snippetNoteDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, snippetNoteOptions(individualTool))
}

// Canonical action IDs referenced by snippet note RelatedActions. The snippet
// note actions are projected under the gitlab_snippet catalog group, so their
// base domain is "snippet".
const (
	actionSnippetGet        = "snippet.get"
	actionSnippetList       = "snippet.list"
	actionSnippetNoteList   = "snippet.note_list"
	actionSnippetNoteGet    = "snippet.note_get"
	actionSnippetNoteCreate = "snippet.note_create"
	actionSnippetNoteUpdate = "snippet.note_update"
	actionSnippetNoteDelete = "snippet.note_delete"
)

// projectIDGuidance is the shared parameter guidance for the project_id input
// that every snippet note action accepts.
func projectIDGuidance() toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:     "scope_project",
		ValueSource:      "Project ID or full namespace path that owns the snippet.",
		ExampleBinding:   `params.project_id:"group/project"`,
		CommonConfusions: []string{"Use the snippet's parent project here, not a group path or global snippet ID."},
	}
}

// snippetIDGuidance is the shared parameter guidance for the snippet_id input.
func snippetIDGuidance() toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:     "snippet_id",
		ValueSource:      "Numeric snippet ID from a prior gitlab_snippet_list or gitlab_project_snippet_list result.",
		ExampleBinding:   "params.snippet_id:7",
		CommonConfusions: []string{"snippet_id is the project snippet ID, not the note_id."},
	}
}

// noteIDGuidance is the shared parameter guidance for the note_id input.
func noteIDGuidance() toolutil.ParameterGuidance {
	return toolutil.ParameterGuidance{
		SemanticRole:     "note_id",
		ValueSource:      "Numeric note ID from a prior snippet.note_list or snippet.note_create result.",
		ExampleBinding:   "params.note_id:100",
		CommonConfusions: []string{"note_id is the comment ID, not the snippet_id. Obtain it from snippet.note_list."},
	}
}

// snippetNoteOptions returns the [toolutil.ActionSpecOptions] for one snippet
// note action, filled with non-generic Usage, natural-language Aliases,
// RelatedActions, ParameterGuidance, and the "Returns: … See also: …"
// individual-tool description (1:1 audit R-META).
func snippetNoteOptions(individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases:        []string{individualTool},
		Usage:          "Use to execute snippetnotes domain action.",
		Tags:           []string{"snippet", "note"},
		OpenWorld:      true,
		OwnerPackage:   "snippetnotes",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	decorateSnippetNoteMeta(&options, individualTool)
	return options
}

// decorateSnippetNoteMeta fills the per-action discovery metadata for each
// snippet note individual tool, mirroring the style of the issuenotes package.
func decorateSnippetNoteMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	switch individualTool {
	case "gitlab_snippet_note_create":
		options.Usage = "Add a comment (note) to a project snippet. Use when the task asks to comment on, reply to, or annotate an existing snippet."
		options.Aliases = []string{"comment on snippet", "add snippet comment", "reply to snippet", "post snippet note"}
		options.RelatedActions = []string{actionSnippetGet, actionSnippetNoteList, actionSnippetNoteGet}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": projectIDGuidance(),
			"snippet_id": snippetIDGuidance(),
			"body": {
				ValueSource:      "The comment text the user wants to post. Markdown is supported.",
				ExampleBinding:   `params.body:"Nice snippet, thanks!"`,
				CommonConfusions: []string{"body is the comment content, not a snippet content update. Updating the snippet body is snippet.update."},
			},
			"created_at": {
				ValueSource:      "Optional RFC 3339 timestamp to backdate the note. Requires admin or owner permissions.",
				CommonConfusions: []string{"created_at is ignored unless you have administrator or project/group owner permissions."},
			},
		}
		options.IndividualTool.Description = "Add a comment (note) to a project snippet. Returns: the created note with id, author, body, and timestamps. See also: gitlab_snippet_note_list, gitlab_snippet_note_get, gitlab_snippet_get."
	case "gitlab_snippet_note_list":
		options.Usage = "List all comments (notes) on a project snippet, including system notes. Use when the task asks to read a snippet's discussion, recent comments, or activity. Supports order_by, sort, and keyset pagination."
		options.Aliases = []string{"list snippet comments", "show snippet notes", "read snippet discussion", "get snippet comments"}
		options.RelatedActions = []string{actionSnippetNoteGet, actionSnippetNoteCreate, actionSnippetGet}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": projectIDGuidance(),
			"snippet_id": snippetIDGuidance(),
			"order_by": {
				ValueSource:      "Field to order notes by: created_at or updated_at.",
				ExampleBinding:   `params.order_by:"created_at"`,
				CommonConfusions: []string{"Combine order_by with sort. Pass the field name, not a phrase like 'newest first'."},
			},
		}
		options.IndividualTool.Description = "List all notes (comments) on a project snippet. Returns: notes with author, body, system flag, and pagination metadata. See also: gitlab_snippet_note_get, gitlab_snippet_note_create, gitlab_snippet_get."
	case "gitlab_snippet_note_get":
		options.Usage = "Get one snippet note by params.note_id. Use when the task references a specific comment or note ID on a snippet."
		options.Aliases = []string{"get snippet comment", "show snippet note", "fetch snippet note"}
		options.RelatedActions = []string{actionSnippetNoteList, actionSnippetNoteUpdate, actionSnippetNoteDelete}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": projectIDGuidance(),
			"snippet_id": snippetIDGuidance(),
			"note_id":    noteIDGuidance(),
		}
		options.IndividualTool.Description = "Get a single snippet note by its ID. Returns: the note with author, body, timestamps, and position. See also: gitlab_snippet_note_list, gitlab_snippet_note_update, gitlab_snippet_note_delete."
	case "gitlab_snippet_note_update":
		options.Usage = "Replace the body of an existing snippet note. Only the original author or a Maintainer/Owner can edit a note. System notes cannot be edited. Use when the task asks to edit, fix, or amend a comment."
		options.Aliases = []string{"edit snippet comment", "update snippet note", "amend snippet comment"}
		options.RelatedActions = []string{actionSnippetNoteGet, actionSnippetNoteList, actionSnippetNoteDelete}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": projectIDGuidance(),
			"snippet_id": snippetIDGuidance(),
			"note_id":    noteIDGuidance(),
			"body": {
				ValueSource:      "The new comment text that replaces the existing note body. Markdown is supported.",
				ExampleBinding:   `params.body:"Updated: fixed the typo."`,
				CommonConfusions: []string{"This replaces the whole note body. It does not append to it."},
			},
		}
		options.IndividualTool.Description = "Update a snippet note's body. Returns: the updated note with new body and updated_at timestamp. See also: gitlab_snippet_note_get, gitlab_snippet_note_list, gitlab_snippet_note_delete."
	case "gitlab_snippet_note_delete":
		options.Usage = "Permanently delete a snippet note. Destructive and irreversible. Only the note author or a project Maintainer/Owner can delete a note. System notes cannot be deleted. Requires explicit confirmation."
		options.Aliases = []string{"delete snippet comment", "remove snippet note", "delete snippet note"}
		options.RelatedActions = []string{actionSnippetNoteGet, actionSnippetNoteList, actionSnippetList}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"project_id": projectIDGuidance(),
			"snippet_id": snippetIDGuidance(),
			"note_id":    noteIDGuidance(),
		}
		options.IndividualTool.Description = "Delete a snippet note permanently. Returns: a success confirmation naming the note, snippet, and project. See also: gitlab_snippet_note_get, gitlab_snippet_note_list."
	}
}
