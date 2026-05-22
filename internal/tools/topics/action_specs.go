package topics

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for topic tools.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		topicReadSpec("topic_list", toolutil.RouteAction(client, List), "gitlab_list_topics"),
		topicReadSpec("topic_get", toolutil.RouteAction(client, Get), "gitlab_get_topic"),
		topicCreateSpec("topic_create", toolutil.RouteAction(client, Create), "gitlab_create_topic"),
		topicUpdateSpec("topic_update", toolutil.RouteAction(client, Update), "gitlab_update_topic"),
		topicDeleteSpec("topic_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_delete_topic"),
	}
}

func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("topic")
	return out, nil
}

func topicReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, topicOptions(individualTool))
}

func topicCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, topicOptions(individualTool))
}

func topicUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, topicOptions(individualTool))
}

func topicDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, topicOptions(individualTool))
}

func topicOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"admin", "topic"},
		OpenWorld:      true,
		OwnerPackage:   "topics",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
