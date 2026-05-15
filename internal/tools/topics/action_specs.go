package topics

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
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
	options := topicOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func topicCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, topicOptions(individualTool))
}

func topicUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := topicOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func topicDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := topicOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func topicOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"admin", "topic"},
		OpenWorld:      true,
		OwnerPackage:   "topics",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
