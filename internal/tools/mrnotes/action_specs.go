package mrnotes

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for merge request note actions.
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

func mrNoteOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute mrnotes domain action.", Tags: []string{"merge_request", "review", "note"},
		OpenWorld:      true,
		OwnerPackage:   "mrnotes",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
