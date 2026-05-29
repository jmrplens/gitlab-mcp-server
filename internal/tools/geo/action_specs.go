package geo

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for Geo site management actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		geoCreateSpec("create", toolutil.RouteAction(client, Create), "gitlab_create_geo_site"),
		geoReadSpec("list", toolutil.RouteAction(client, List), "gitlab_list_geo_sites"),
		geoReadSpec("get", toolutil.RouteAction(client, Get), "gitlab_get_geo_site"),
		geoUpdateSpec("edit", toolutil.RouteAction(client, Edit), "gitlab_edit_geo_site"),
		geoDeleteSpec("delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_delete_geo_site"),
		geoUpdateSpec("repair", toolutil.RouteAction(client, Repair), "gitlab_repair_geo_site"),
		geoReadSpec("list_status", toolutil.RouteAction(client, ListStatus), "gitlab_list_status_all_geo_sites"),
		geoReadSpec("get_status", toolutil.RouteAction(client, GetStatus), "gitlab_get_status_geo_site"),
	}
}

func deleteOutput(ctx context.Context, client *gitlabclient.Client, input IDInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{
		Status:  "success",
		Message: fmt.Sprintf("Successfully deleted Geo site %d.", input.ID),
	}, nil
}

func geoReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, geoOptions(individualTool))
}

func geoCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, geoOptions(individualTool))
}

func geoUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, geoOptions(individualTool))
}

func geoDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, geoOptions(individualTool))
}

func geoOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute geo domain action.", Tags: []string{"geo", "replication"},
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "geo",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
