package epicnotes

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for epic note actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		epicNoteReadSpec("epic_note_list", toolutil.RouteAction(client, List), "gitlab_epic_note_list"),
		epicNoteReadSpec("epic_note_get", toolutil.RouteAction(client, Get), "gitlab_epic_note_get"),
		epicNoteCreateSpec("epic_note_create", toolutil.RouteAction(client, Create), "gitlab_epic_note_create"),
		epicNoteUpdateSpec("epic_note_update", toolutil.RouteAction(client, Update), "gitlab_epic_note_update"),
		epicNoteDeleteSpec("epic_note_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_epic_note_delete"),
	}
}

func epicNoteReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := epicNoteOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func epicNoteCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, epicNoteOptions(individualTool))
}

func epicNoteUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := epicNoteOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func epicNoteDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := epicNoteOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func epicNoteOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"group", "epic", "note"},
		RelatedActions: []string{"group.epic_get"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "epicnotes",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
