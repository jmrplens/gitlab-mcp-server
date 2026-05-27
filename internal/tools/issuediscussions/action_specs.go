package issuediscussions

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for issue discussion actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		issueDiscussionReadSpec("discussion_list", toolutil.RouteAction(client, List), "gitlab_list_issue_discussions"),
		issueDiscussionReadSpec("discussion_get", toolutil.RouteAction(client, Get), "gitlab_get_issue_discussion"),
		issueDiscussionCreateSpec("discussion_create", toolutil.RouteAction(client, Create), "gitlab_create_issue_discussion"),
		issueDiscussionCreateSpec("discussion_add_note", toolutil.RouteAction(client, AddNote), "gitlab_add_issue_discussion_note"),
		issueDiscussionUpdateSpec("discussion_update_note", toolutil.RouteAction(client, UpdateNote), "gitlab_update_issue_discussion_note"),
		issueDiscussionDeleteSpec("discussion_delete_note", toolutil.DestructiveAction(client, deleteNoteOutput), "gitlab_delete_issue_discussion_note"),
	}
}

func deleteNoteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteNoteInput) (toolutil.DeleteOutput, error) {
	if err := DeleteNote(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("issue discussion note")
	return out, nil
}

func issueDiscussionReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, issueDiscussionOptions(individualTool))
}

func issueDiscussionCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, issueDiscussionOptions(individualTool))
}

func issueDiscussionUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, issueDiscussionOptions(individualTool))
}

func issueDiscussionDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, issueDiscussionOptions(individualTool))
}

func issueDiscussionOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute issuediscussions domain action.", Tags: []string{"issue", "discussion"},
		OpenWorld:      true,
		OwnerPackage:   "issuediscussions",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
