package snippetnotes

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for project snippet note actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		snippetNoteReadSpec("note_list", toolutil.RouteAction(client, List), "gitlab_snippet_note_list"),
		snippetNoteReadSpec("note_get", toolutil.RouteAction(client, Get), "gitlab_snippet_note_get"),
		snippetNoteCreateSpec("note_create", toolutil.RouteAction(client, Create), "gitlab_snippet_note_create"),
		snippetNoteUpdateSpec("note_update", toolutil.RouteAction(client, Update), "gitlab_snippet_note_update"),
		snippetNoteDeleteSpec("note_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_snippet_note_delete"),
	}
}

func snippetNoteReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := snippetNoteOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func snippetNoteCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, snippetNoteOptions(individualTool))
}

func snippetNoteUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := snippetNoteOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func snippetNoteDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := snippetNoteOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func snippetNoteOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"snippet", "note"},
		OpenWorld:      true,
		OwnerPackage:   "snippetnotes",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
