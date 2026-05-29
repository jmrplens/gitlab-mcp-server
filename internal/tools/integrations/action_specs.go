package integrations

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for project integration actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		integrationReadSpec("integration_list", toolutil.RouteAction(client, List), "gitlab_list_integrations"),
		integrationReadSpec("integration_get", toolutil.RouteAction(client, Get), "gitlab_get_integration"),
		integrationDeleteSpec("integration_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_delete_integration"),
		integrationCreateSpec("integration_set_jira", toolutil.RouteAction(client, SetJira), "gitlab_set_jira_integration"),
	}
}

func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("integration")
	return out, nil
}

func integrationReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, integrationOptions(individualTool))
}

func integrationCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, integrationOptions(individualTool))
}

func integrationDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, integrationOptions(individualTool))
}

func integrationOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute integrations domain action.", Tags: []string{"project", "integration"},
		RelatedActions: []string{"project.get"},
		OpenWorld:      true,
		OwnerPackage:   "integrations",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
