package issuenotes

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for issue note actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		issueNoteCreateSpec("note_create", toolutil.RouteAction(client, Create), "gitlab_issue_note_create"),
		issueNoteReadSpec("note_list", toolutil.RouteAction(client, List), "gitlab_issue_note_list"),
		issueNoteReadSpec("note_get", toolutil.RouteAction(client, GetNote), "gitlab_issue_note_get"),
		issueNoteUpdateSpec("note_update", toolutil.RouteAction(client, Update), "gitlab_issue_note_update"),
		issueNoteDeleteSpec("note_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_issue_note_delete"),
	}
}

func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult(fmt.Sprintf("note %d from issue #%d in project %s", input.NoteID, input.IssueIID, input.ProjectID))
	return out, nil
}

func issueNoteReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, issueNoteOptions(individualTool))
}

func issueNoteCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, issueNoteOptions(individualTool))
}

func issueNoteUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, issueNoteOptions(individualTool))
}

func issueNoteDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, issueNoteOptions(individualTool))
}

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
