package mrdraftnotes

import (
	"context"
	"fmt"

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
		mrDraftNoteDeleteSpec("draft_note_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_mr_draft_note_delete"),
		mrDraftNoteUpdateSpec("draft_note_publish", toolutil.RouteAction(client, publishOutput), "gitlab_mr_draft_note_publish"),
		mrDraftNoteUpdateSpec("draft_note_publish_all", toolutil.RouteAction(client, publishAllOutput), "gitlab_mr_draft_note_publish_all"),
	}
}

func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult(fmt.Sprintf("draft note %d from MR !%d in project %s", input.NoteID, input.MRIID, input.ProjectID))
	return out, nil
}

func publishOutput(ctx context.Context, client *gitlabclient.Client, input PublishInput) (toolutil.DeleteOutput, error) {
	if err := Publish(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: fmt.Sprintf("Draft note %d published on MR !%d in project %s", input.NoteID, input.MRIID, input.ProjectID)}, nil
}

func publishAllOutput(ctx context.Context, client *gitlabclient.Client, input PublishAllInput) (toolutil.DeleteOutput, error) {
	if err := PublishAll(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: fmt.Sprintf("All draft notes published on MR !%d in project %s", input.MRIID, input.ProjectID)}, nil
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
