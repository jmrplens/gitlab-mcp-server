package epicdiscussions

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for epic discussion actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		epicDiscussionReadSpec("epic_discussion_list", toolutil.RouteAction(client, List), "gitlab_list_epic_discussions"),
		epicDiscussionReadSpec("epic_discussion_get", toolutil.RouteAction(client, Get), "gitlab_get_epic_discussion"),
		epicDiscussionCreateSpec("epic_discussion_create", toolutil.RouteAction(client, Create), "gitlab_create_epic_discussion"),
		epicDiscussionCreateSpec("epic_discussion_add_note", toolutil.RouteAction(client, AddNote), "gitlab_add_epic_discussion_note"),
		epicDiscussionUpdateSpec("epic_discussion_update_note", toolutil.RouteAction(client, UpdateNote), "gitlab_update_epic_discussion_note"),
		epicDiscussionDeleteSpec("epic_discussion_delete_note", toolutil.DestructiveAction(client, DeleteNoteOutput), "gitlab_delete_epic_discussion_note"),
	}
}

// DeleteNoteOutput deletes an epic discussion note and returns the canonical success message shape.
func DeleteNoteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteNoteInput) (toolutil.DeleteOutput, error) {
	if err := DeleteNote(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted epic discussion note."}, nil
}

func epicDiscussionReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, epicDiscussionOptions(individualTool))
}

func epicDiscussionCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, epicDiscussionOptions(individualTool))
}

func epicDiscussionUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, epicDiscussionOptions(individualTool))
}

func epicDiscussionDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, epicDiscussionOptions(individualTool))
}

func epicDiscussionOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute epicdiscussions domain action.", Tags: []string{"group", "epic", "discussion"},
		RelatedActions: []string{"group.epic_get"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "epicdiscussions",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
