package epics

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for group epic actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		epicReadSpec("epic_list", toolutil.RouteAction(client, List), "gitlab_epic_list"),
		epicReadSpec("epic_get", toolutil.RouteAction(client, Get), "gitlab_epic_get"),
		epicReadSpec("epic_get_links", toolutil.RouteAction(client, GetLinks), "gitlab_epic_get_links"),
		epicCreateSpec("epic_create", toolutil.RouteAction(client, Create), "gitlab_epic_create"),
		epicUpdateSpec("epic_update", toolutil.RouteAction(client, Update), "gitlab_epic_update"),
		epicDeleteSpec("epic_delete", toolutil.DestructiveAction(client, DeleteOutput), "gitlab_epic_delete"),
	}
}

// DeleteOutput deletes an epic and returns the canonical success message shape.
func DeleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{
		Status:  "success",
		Message: fmt.Sprintf("Successfully deleted epic &%d from group %s.", input.IID, input.FullPath),
	}, nil
}

func epicReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, epicOptions(individualTool))
}

func epicCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, epicOptions(individualTool))
}

func epicUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, epicOptions(individualTool))
}

func epicDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, epicOptions(individualTool))
}

func epicOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute epics domain action.", Tags: []string{"group", "epic"},
		RelatedActions: []string{"group.get"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "epics",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
