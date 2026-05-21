package customemoji

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for custom emoji actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		customEmojiReadSpec("list", toolutil.RouteAction(client, List), "gitlab_list_custom_emoji"),
		customEmojiCreateSpec("create", toolutil.RouteAction(client, Create), "gitlab_create_custom_emoji"),
		customEmojiDeleteSpec("delete", toolutil.DestructiveAction(client, DeleteOutput), "gitlab_delete_custom_emoji"),
	}
}

func customEmojiReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, customEmojiOptions(individualTool))
}

func customEmojiCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, customEmojiOptions(individualTool))
}

func customEmojiDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, customEmojiOptions(individualTool))
}

func customEmojiOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"custom_emoji", "group", "graphql"},
		RelatedActions: []string{"group.get", "issue.emoji_issue_create", "merge_request.emoji_mr_create"},
		OpenWorld:      true,
		OwnerPackage:   "customemoji",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

// DeleteOutput deletes a custom emoji and returns the canonical success message shape.
func DeleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted custom emoji."}, nil
}
