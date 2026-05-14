package customemoji

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for custom emoji actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		customEmojiReadSpec("list", toolutil.RouteAction(client, List), "gitlab_list_custom_emoji"),
		customEmojiCreateSpec("create", toolutil.RouteAction(client, Create), "gitlab_create_custom_emoji"),
		customEmojiDeleteSpec("delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_delete_custom_emoji"),
	}
}

func customEmojiReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := customEmojiOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func customEmojiCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, customEmojiOptions(individualTool))
}

func customEmojiDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := customEmojiOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
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
