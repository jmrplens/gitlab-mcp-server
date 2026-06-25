package groupboards

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for group issue board actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupBoardReadSpec("group_board_list", toolutil.RouteAction(client, ListGroupBoards), "gitlab_group_board_list",
			"List all issue boards in a group with pagination. Returns: each board with its group, milestone, scope labels, lists, and pagination metadata. See also: gitlab_group_board_get, gitlab_group_board_create, gitlab_group_board_list_lists."),
		groupBoardReadSpec("group_board_get", toolutil.RouteAction(client, GetGroupBoard), "gitlab_group_board_get",
			"Get a single group issue board by ID. Returns: the board with its group, milestone, scope labels, and lists. See also: gitlab_group_board_list, gitlab_group_board_update, gitlab_group_board_list_lists."),
		groupBoardCreateSpec("group_board_create", toolutil.RouteAction(client, CreateGroupBoard), "gitlab_group_board_create",
			"Create a new group issue board (Premium/Ultimate). Returns: the created board with its group, milestone, labels, and lists. See also: gitlab_group_board_get, gitlab_group_board_update, gitlab_group_board_delete."),
		groupBoardUpdateSpec("group_board_update", toolutil.RouteAction(client, UpdateGroupBoard), "gitlab_group_board_update",
			"Update a group issue board's name and scope (assignee, milestone, labels, weight). Returns: the updated board with its group, milestone, labels, and lists. See also: gitlab_group_board_get, gitlab_group_board_list."),
		groupBoardDeleteSpec("group_board_delete", toolutil.DestructiveAction(client, deleteGroupBoardOutput), "gitlab_group_board_delete",
			"Delete a group issue board permanently. Returns: a success confirmation. See also: gitlab_group_board_get, gitlab_group_board_list."),
		groupBoardReadSpec("group_board_list_lists", toolutil.RouteAction(client, ListGroupBoardLists), "gitlab_group_board_list_lists",
			"List the lists (columns) of a group issue board with pagination. Returns: each list with its assignee, label, iteration, and milestone scope plus pagination metadata. See also: gitlab_group_board_list_get, gitlab_group_board_list_create, gitlab_group_board_get."),
		groupBoardReadSpec("group_board_get_list", toolutil.RouteAction(client, GetGroupBoardList), "gitlab_group_board_list_get",
			"Get a single list (column) of a group issue board. Returns: the list with its assignee, label, iteration, and milestone scope. See also: gitlab_group_board_list_lists, gitlab_group_board_list_update, gitlab_group_board_list_delete."),
		groupBoardCreateSpec("group_board_create_list", toolutil.RouteAction(client, CreateGroupBoardList), "gitlab_group_board_list_create",
			"Create a new list (column) on a group issue board from a group label. Returns: the created list with its label and scope. See also: gitlab_group_board_list_get, gitlab_group_board_list_update, gitlab_group_board_list_delete."),
		groupBoardUpdateSpec("group_board_update_list", toolutil.RouteAction(client, UpdateGroupBoardList), "gitlab_group_board_list_update",
			"Reorder a list (column) within a group issue board. Returns: the repositioned list with its scope. See also: gitlab_group_board_list_get, gitlab_group_board_list_lists."),
		groupBoardDeleteSpec("group_board_delete_list", toolutil.DestructiveAction(client, deleteGroupBoardListOutput), "gitlab_group_board_list_delete",
			"Delete a list (column) from a group issue board. Returns: a success confirmation. See also: gitlab_group_board_list_get, gitlab_group_board_list_lists."),
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

func groupBoardReadSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, groupBoardOptions(individualTool, description))
}

func groupBoardCreateSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, groupBoardOptions(individualTool, description))
}

func groupBoardUpdateSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, groupBoardOptions(individualTool, description))
}

func groupBoardDeleteSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, groupBoardOptions(individualTool, description))
}

func groupBoardOptions(individualTool, description string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute groupboards domain action.", Tags: []string{"group", "board"},
		RelatedActions: []string{"group.group_label_list", "group.issues"},
		OpenWorld:      true,
		OwnerPackage:   "groupboards",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool), Description: description},
	}
}
