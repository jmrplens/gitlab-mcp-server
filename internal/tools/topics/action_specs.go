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
	return toolutil.NewReadActionSpec(name, route, topicOptions(name, individualTool))
}

func topicCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, topicOptions(name, individualTool))
}

func topicUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, topicOptions(name, individualTool))
}

func topicDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, topicOptions(name, individualTool))
}

func topicOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	usage := "List topics configured in the GitLab instance."
	guidance := map[string]toolutil.ParameterGuidance{}
	if actionName == "topic_get" || actionName == "topic_update" || actionName == "topic_delete" {
		guidance["topic_id"] = toolutil.ParameterGuidance{
			SemanticRole:   "topic_id",
			ValueSource:    "Topic numeric ID from topic list/get outputs.",
			ExampleBinding: "params.topic_id:1",
		}
	}
	if actionName == "topic_create" {
		usage = "Create a new topic in the instance."
	}
	if actionName == "topic_update" {
		usage = "Update topic metadata by topic ID."
	}
	if actionName == "topic_delete" {
		usage = "Delete a topic by topic ID."
	}

	return toolutil.ActionSpecOptions{
		Aliases:           []string{individualTool},
		Tags:              []string{"admin", "topic"},
		Usage:             usage,
		RelatedActions:    []string{"project.list"},
		ParameterGuidance: guidance,
		OpenWorld:         true,
		OwnerPackage:      "topics",
		IndividualTool:    toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
