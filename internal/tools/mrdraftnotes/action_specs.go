package mrdraftnotes

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for merge request draft note actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		mrDraftNoteReadSpec("draft_note_list", toolutil.RouteAction(client, List), "gitlab_mr_draft_note_list"),
		mrDraftNoteReadSpec("draft_note_get", toolutil.RouteAction(client, Get), "gitlab_mr_draft_note_get"),
		mrDraftNoteCreateSpec("draft_note_create", toolutil.RouteAction(client, Create), "gitlab_mr_draft_note_create"),
		mrDraftNoteUpdateSpec("draft_note_update", toolutil.RouteAction(client, Update), "gitlab_mr_draft_note_update"),
		mrDraftNoteDeleteSpec("draft_note_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_mr_draft_note_delete"),
		mrDraftNoteCreateSpec("draft_note_publish", toolutil.RouteVoidAction(client, Publish), "gitlab_mr_draft_note_publish"),
		mrDraftNoteCreateSpec("draft_note_publish_all", toolutil.RouteVoidAction(client, PublishAll), "gitlab_mr_draft_note_publish_all"),
	}
}

func mrDraftNoteReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := mrDraftNoteOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func mrDraftNoteCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, mrDraftNoteOptions(individualTool))
}

func mrDraftNoteUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := mrDraftNoteOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func mrDraftNoteDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := mrDraftNoteOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func mrDraftNoteOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"merge_request", "review", "draft_note"},
		OpenWorld:      true,
		OwnerPackage:   "mrdraftnotes",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
