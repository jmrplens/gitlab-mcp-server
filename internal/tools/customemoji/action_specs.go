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
	return toolutil.NewReadActionSpec(name, route, customEmojiOptions(name, individualTool))
}

func customEmojiCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, customEmojiOptions(name, individualTool))
}

func customEmojiDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, customEmojiOptions(name, individualTool))
}

func customEmojiOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	guidance := map[string]toolutil.ParameterGuidance{}
	aliases := []string{}
	usage := ""

	switch actionName {
	case "list":
		aliases = []string{"list custom emoji", "group emoji list", "emoji management"}
		usage = "List group-level custom emoji using GraphQL-backed routes."
	case "create":
		aliases = []string{"create custom emoji", "add group emoji", "custom emoji create"}
		usage = "Create group-level custom emoji using GraphQL-backed routes."
	case "delete":
		aliases = []string{"delete custom emoji", "remove group emoji", "custom emoji delete"}
		usage = "Delete group-level custom emoji using GraphQL-backed routes."
	}

	if actionName == "list" || actionName == "create" {
		guidance["group_path"] = toolutil.ParameterGuidance{
			SemanticRole:   "scope_group",
			ValueSource:    "GitLab full group path.",
			ExampleBinding: `params.group_path:"my-group/subgroup"`,
		}
	}
	if actionName == "delete" {
		guidance["id"] = toolutil.ParameterGuidance{
			SemanticRole:   "custom_emoji_gid",
			ValueSource:    "Global ID returned by list/create operations for delete.",
			ExampleBinding: `params.id:"gid://gitlab/CustomEmoji/123"`,
		}
	}

	return toolutil.ActionSpecOptions{
		Aliases:           aliases,
		Tags:              []string{"custom_emoji", "group", "graphql"},
		Usage:             usage,
		RelatedActions:    []string{"group.get", "issue.emoji_issue_create", "merge_request.emoji_mr_create"},
		ParameterGuidance: guidance,
		OpenWorld:         true,
		OwnerPackage:      "customemoji",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:  individualTool,
			Title: toolutil.TitleFromName(individualTool),
		},
	}
}

// DeleteOutput deletes a custom emoji and returns the canonical success message shape.
func DeleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted custom emoji."}, nil
}
