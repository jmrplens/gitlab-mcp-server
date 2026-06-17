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

// issueNoteOptions returns the base [toolutil.ActionSpecOptions] shared
// by every issue note action (tags, owner, individual tool metadata). It
// customises the Usage string for the get action to highlight the
// note_id-based lookup.
func issueNoteOptions(individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases:        []string{individualTool},
		Usage:          "Use to execute issuenotes domain action.",
		Tags:           []string{"issue", "note"},
		OpenWorld:      true,
		OwnerPackage:   "issuenotes",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	if individualTool == "gitlab_issue_note_get" {
		options.Usage = "Get one issue note by params.note_id. Use when the task references a specific comment or note ID on an issue."
	}
	return options
}
