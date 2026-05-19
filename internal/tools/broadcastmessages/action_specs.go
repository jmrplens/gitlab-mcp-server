package broadcastmessages

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for broadcast message tools.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		broadcastMessageReadSpec("broadcast_message_list", toolutil.RouteAction(client, List), "gitlab_list_broadcast_messages"),
		broadcastMessageReadSpec("broadcast_message_get", toolutil.RouteAction(client, Get), "gitlab_get_broadcast_message"),
		broadcastMessageCreateSpec("broadcast_message_create", toolutil.RouteAction(client, Create), "gitlab_create_broadcast_message"),
		broadcastMessageUpdateSpec("broadcast_message_update", toolutil.RouteAction(client, Update), "gitlab_update_broadcast_message"),
		broadcastMessageDeleteSpec("broadcast_message_delete", toolutil.DestructiveAction(client, DeleteOutput), "gitlab_delete_broadcast_message"),
	}
}

func broadcastMessageReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := broadcastMessageOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func broadcastMessageCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, broadcastMessageOptions(individualTool))
}

func broadcastMessageUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := broadcastMessageOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func broadcastMessageDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := broadcastMessageOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func broadcastMessageOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"admin", "broadcast"},
		OpenWorld:      true,
		OwnerPackage:   "broadcastmessages",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

// DeleteOutput deletes a broadcast message and returns the legacy success message shape.
func DeleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted broadcast_message."}, nil
}
