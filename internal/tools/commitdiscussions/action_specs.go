package commitdiscussions

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for commit discussion actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		commitDiscussionReadSpec("commit_discussion_list", toolutil.RouteAction(client, List), "gitlab_list_commit_discussions"),
		commitDiscussionReadSpec("commit_discussion_get", toolutil.RouteAction(client, Get), "gitlab_get_commit_discussion"),
		commitDiscussionCreateSpec("commit_discussion_create", toolutil.RouteAction(client, Create), "gitlab_create_commit_discussion"),
		commitDiscussionCreateSpec("commit_discussion_add_note", toolutil.RouteAction(client, AddNote), "gitlab_add_commit_discussion_note"),
		commitDiscussionUpdateSpec("commit_discussion_update_note", toolutil.RouteAction(client, UpdateNote), "gitlab_update_commit_discussion_note"),
		commitDiscussionDeleteSpec("commit_discussion_delete_note", toolutil.DestructiveAction(client, DeleteNoteOutput), "gitlab_delete_commit_discussion_note"),
	}
}

func commitDiscussionReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := commitDiscussionOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func commitDiscussionCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, commitDiscussionOptions(individualTool))
}

func commitDiscussionUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := commitDiscussionOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func commitDiscussionDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := commitDiscussionOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func commitDiscussionOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"repository", "commit", "discussion"},
		RelatedActions: []string{"repository.commit_get", "repository.commit_diff"},
		OpenWorld:      true,
		OwnerPackage:   "commitdiscussions",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

// DeleteNoteOutput deletes a commit discussion note and returns the canonical success message shape.
func DeleteNoteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteNoteInput) (toolutil.DeleteOutput, error) {
	if err := DeleteNote(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted commit discussion note."}, nil
}
