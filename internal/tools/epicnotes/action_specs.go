package epicnotes

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for epic note actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		epicNoteReadSpec("epic_note_list", toolutil.RouteAction(client, List), "gitlab_epic_note_list"),
		epicNoteReadSpec("epic_note_get", toolutil.RouteAction(client, Get), "gitlab_epic_note_get"),
		epicNoteCreateSpec("epic_note_create", toolutil.RouteAction(client, Create), "gitlab_epic_note_create"),
		epicNoteUpdateSpec("epic_note_update", toolutil.RouteAction(client, Update), "gitlab_epic_note_update"),
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

func epicNoteReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, epicNoteOptions(individualTool))
}

func epicNoteCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, epicNoteOptions(individualTool))
}

func epicNoteUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, epicNoteOptions(individualTool))
}

func epicNoteDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, epicNoteOptions(individualTool))
}

func epicNoteOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute epicnotes domain action.", Tags: []string{"group", "epic", "note"},
		RelatedActions: []string{"group.epic_get"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "epicnotes",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
