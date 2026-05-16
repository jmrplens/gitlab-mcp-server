package groupboards

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for group issue board actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupBoardReadSpec("group_board_list", toolutil.RouteAction(client, ListGroupBoards), "gitlab_group_board_list"),
		groupBoardReadSpec("group_board_get", toolutil.RouteAction(client, GetGroupBoard), "gitlab_group_board_get"),
		groupBoardCreateSpec("group_board_create", toolutil.RouteAction(client, CreateGroupBoard), "gitlab_group_board_create"),
		groupBoardUpdateSpec("group_board_update", toolutil.RouteAction(client, UpdateGroupBoard), "gitlab_group_board_update"),
		groupBoardDeleteSpec("group_board_delete", toolutil.DestructiveAction(client, deleteGroupBoardOutput), "gitlab_group_board_delete"),
		groupBoardReadSpec("group_board_list_lists", toolutil.RouteAction(client, ListGroupBoardLists), "gitlab_group_board_list_lists"),
		groupBoardReadSpec("group_board_get_list", toolutil.RouteAction(client, GetGroupBoardList), "gitlab_group_board_list_get"),
		groupBoardCreateSpec("group_board_create_list", toolutil.RouteAction(client, CreateGroupBoardList), "gitlab_group_board_list_create"),
		groupBoardUpdateSpec("group_board_update_list", toolutil.RouteAction(client, UpdateGroupBoardList), "gitlab_group_board_list_update"),
		groupBoardDeleteSpec("group_board_delete_list", toolutil.DestructiveAction(client, deleteGroupBoardListOutput), "gitlab_group_board_list_delete"),
	}
}

func deleteGroupBoardOutput(ctx context.Context, client *gitlabclient.Client, input DeleteGroupBoardInput) (toolutil.DeleteOutput, error) {
	if err := DeleteGroupBoard(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted group board."}, nil
}

func deleteGroupBoardListOutput(ctx context.Context, client *gitlabclient.Client, input DeleteGroupBoardListInput) (toolutil.DeleteOutput, error) {
	if err := DeleteGroupBoardList(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted group board list."}, nil
}

func groupBoardReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupBoardOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func groupBoardCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, groupBoardOptions(individualTool))
}

func groupBoardUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupBoardOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func groupBoardDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupBoardOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func groupBoardOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"group", "board"},
		RelatedActions: []string{"group.group_label_list", "group.issues"},
		OpenWorld:      true,
		OwnerPackage:   "groupboards",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
