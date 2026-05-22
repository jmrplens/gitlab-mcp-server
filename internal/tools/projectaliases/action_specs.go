package projectaliases

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for project alias actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		projectAliasReadSpec("list", toolutil.RouteAction(client, List), "gitlab_list_project_aliases"),
		projectAliasReadSpec("get", toolutil.RouteAction(client, Get), "gitlab_get_project_alias"),
		projectAliasCreateSpec("create", toolutil.RouteAction(client, Create), "gitlab_create_project_alias"),
		projectAliasDeleteSpec("delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_delete_project_alias"),
	}
}

func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult(fmt.Sprintf("project alias %q", input.Name))
	return out, nil
}

func projectAliasReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, projectAliasOptions(individualTool))
}

func projectAliasCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, projectAliasOptions(individualTool))
}

func projectAliasDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, projectAliasOptions(individualTool))
}

func projectAliasOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"project", "alias"},
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "projectaliases",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
