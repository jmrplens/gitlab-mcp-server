package groupscim

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for group SCIM identity actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	updateAction := func(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (UpdateOutput, error) {
		if err := Update(ctx, client, input); err != nil {
			return UpdateOutput{}, err
		}
		return UpdateOutput{Updated: true, Message: "SCIM identity updated successfully."}, nil
	}

	return []toolutil.ActionSpec{
		groupSCIMReadSpec("list", toolutil.RouteAction(client, List), "gitlab_list_group_scim_identities"),
		groupSCIMReadSpec("get", toolutil.RouteAction(client, Get), "gitlab_get_group_scim_identity"),
		groupSCIMUpdateSpec("update", toolutil.RouteAction(client, updateAction), "gitlab_update_group_scim_identity"),
		groupSCIMDeleteSpec("delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_delete_group_scim_identity"),
	}
}

func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{
		Status:  "success",
		Message: fmt.Sprintf("Successfully deleted SCIM identity %s from group %s.", input.UID, input.GroupID),
	}, nil
}

func groupSCIMReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, groupSCIMOptions(individualTool))
}

func groupSCIMUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, groupSCIMOptions(individualTool))
}

func groupSCIMDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, groupSCIMOptions(individualTool))
}

func groupSCIMOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute groupscim domain action.", Tags: []string{"scim", "identity"},
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "groupscim",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
