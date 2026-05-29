package snippetdiscussions

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for snippet discussion actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		snippetDiscussionReadSpec("discussion_list", toolutil.RouteAction(client, List), "gitlab_list_snippet_discussions"),
		snippetDiscussionReadSpec("discussion_get", toolutil.RouteAction(client, Get), "gitlab_get_snippet_discussion"),
		snippetDiscussionCreateSpec("discussion_create", toolutil.RouteAction(client, Create), "gitlab_create_snippet_discussion"),
		snippetDiscussionCreateSpec("discussion_add_note", toolutil.RouteAction(client, AddNote), "gitlab_add_snippet_discussion_note"),
		snippetDiscussionUpdateSpec("discussion_update_note", toolutil.RouteAction(client, UpdateNote), "gitlab_update_snippet_discussion_note"),
		snippetDiscussionDeleteSpec("discussion_delete_note", toolutil.DestructiveAction(client, deleteNoteOutput), "gitlab_delete_snippet_discussion_note"),
	}
}

func deleteNoteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteNoteInput) (toolutil.DeleteOutput, error) {
	if err := DeleteNote(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("snippet discussion note")
	return out, nil
}

func snippetDiscussionReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, snippetDiscussionOptions(individualTool))
}

func snippetDiscussionCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, snippetDiscussionOptions(individualTool))
}

func snippetDiscussionUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, snippetDiscussionOptions(individualTool))
}

func snippetDiscussionDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, snippetDiscussionOptions(individualTool))
}

func snippetDiscussionOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute snippetdiscussions domain action.", Tags: []string{"snippet", "discussion"},
		OpenWorld:      true,
		OwnerPackage:   "snippetdiscussions",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
