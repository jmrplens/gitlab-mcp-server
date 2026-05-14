package mrnotes

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for merge request note actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		mrNoteCreateSpec("note_create", toolutil.RouteAction(client, Create), "gitlab_mr_note_create"),
		mrNoteReadSpec("note_list", toolutil.RouteAction(client, List), "gitlab_mr_notes_list"),
		mrNoteReadSpec("note_get", toolutil.RouteAction(client, GetNote), "gitlab_mr_note_get"),
		mrNoteUpdateSpec("note_update", toolutil.RouteAction(client, Update), "gitlab_mr_note_update"),
		mrNoteDeleteSpec("note_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_mr_note_delete"),
	}
}

func mrNoteReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := mrNoteOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func mrNoteCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, mrNoteOptions(individualTool))
}

func mrNoteUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := mrNoteOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func mrNoteDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := mrNoteOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func mrNoteOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"merge_request", "review", "note"},
		OpenWorld:      true,
		OwnerPackage:   "mrnotes",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
