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
		snippetNoteReadSpec("note_list", toolutil.RouteAction(client, List), "gitlab_snippet_note_list"),
		snippetNoteReadSpec("note_get", toolutil.RouteAction(client, Get), "gitlab_snippet_note_get"),
		snippetNoteCreateSpec("note_create", toolutil.RouteAction(client, Create), "gitlab_snippet_note_create"),
		snippetNoteUpdateSpec("note_update", toolutil.RouteAction(client, Update), "gitlab_snippet_note_update"),
		snippetNoteDeleteSpec("note_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_snippet_note_delete"),
	}
}

func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult(fmt.Sprintf("note %d from snippet %d in project %s", input.NoteID, input.SnippetID, input.ProjectID))
	return out, nil
}

func snippetNoteReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, snippetNoteOptions(individualTool))
}

func snippetNoteCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, snippetNoteOptions(individualTool))
}

func snippetNoteUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, snippetNoteOptions(individualTool))
}

func snippetNoteDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, snippetNoteOptions(individualTool))
}

func snippetNoteOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute snippetnotes domain action.", Tags: []string{"snippet", "note"},
		OpenWorld:      true,
		OwnerPackage:   "snippetnotes",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
