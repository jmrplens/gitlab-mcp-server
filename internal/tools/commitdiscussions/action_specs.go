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
	return toolutil.NewReadActionSpec(name, route, commitDiscussionOptions(individualTool))
}

func commitDiscussionCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, commitDiscussionOptions(individualTool))
}

func commitDiscussionUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, commitDiscussionOptions(individualTool))
}

func commitDiscussionDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, commitDiscussionOptions(individualTool))
}

func commitDiscussionOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases:        []string{individualTool},
		Tags:           []string{"repository", "commit", "discussion"},
		Usage:          "Manage commit discussions and discussion notes (list/get/create/add/update/delete). Use this for threaded review context tied to a commit SHA.",
		RelatedActions: []string{"repository.commit_get", "repository.commit_diff"},
		ParameterGuidance: map[string]toolutil.ParameterGuidance{
			"project_id": {
				SemanticRole:   "scope_project",
				ValueSource:    "Project ID or path containing the commit.",
				ExampleBinding: `params.project_id:"group/project"`,
			},
			"commit_sha": {
				SemanticRole:   "commit_sha",
				ValueSource:    "Commit SHA where the discussion lives.",
				ExampleBinding: `params.commit_sha:"abc123def"`,
			},
		},
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
