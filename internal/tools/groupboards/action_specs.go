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
			"List all issue boards in a group with pagination. Returns: each board with its group, milestone, scope labels, lists, and pagination metadata. See also: gitlab_group_board_get, gitlab_group_board_create, gitlab_group_board_list_lists.",
			"Browse the issue boards configured for a group to see how the group plans and tracks work.",
			[]string{"list group issue boards", "show all boards in a group", "group kanban boards overview"}),
		groupBoardReadSpec("group_board_get", toolutil.RouteAction(client, GetGroupBoard), "gitlab_group_board_get",
			"Get a single group issue board by ID. Returns: the board with its group, milestone, scope labels, and lists. See also: gitlab_group_board_list, gitlab_group_board_update, gitlab_group_board_list_lists.",
			"Inspect one group issue board's milestone, scope labels, and columns by its board ID.",
			[]string{"get a group issue board", "show details of a group board", "view a group kanban board"}),
		groupBoardCreateSpec("group_board_create", toolutil.RouteAction(client, CreateGroupBoard), "gitlab_group_board_create",
			"Create a new group issue board (Premium/Ultimate). Returns: the created board with its group, milestone, labels, and lists. See also: gitlab_group_board_get, gitlab_group_board_update, gitlab_group_board_delete.",
			"Set up a new issue board for a group so the team can plan work across its projects.",
			[]string{"create a group issue board", "add a new board to a group", "set up a group kanban board"}),
		groupBoardUpdateSpec("group_board_update", toolutil.RouteAction(client, UpdateGroupBoard), "gitlab_group_board_update",
			"Update a group issue board's name and scope (assignee, milestone, labels, weight). Returns: the updated board with its group, milestone, labels, and lists. See also: gitlab_group_board_get, gitlab_group_board_list.",
			"Rename a group issue board or retune its assignee, milestone, label, and weight scope.",
			[]string{"update a group issue board", "rename a group board", "change a group board scope"}),
		groupBoardDeleteSpec("group_board_delete", toolutil.DestructiveAction(client, deleteGroupBoardOutput), "gitlab_group_board_delete",
			"Delete a group issue board permanently. Returns: a success confirmation. See also: gitlab_group_board_get, gitlab_group_board_list.",
			"Permanently remove an issue board from a group when the team no longer needs it.",
			[]string{"delete a group issue board", "remove a board from a group", "drop a group kanban board"}),
		groupBoardReadSpec("group_board_list_lists", toolutil.RouteAction(client, ListGroupBoardLists), "gitlab_group_board_list_lists",
			"List the lists (columns) of a group issue board with pagination. Returns: each list with its assignee, label, iteration, and milestone scope plus pagination metadata. See also: gitlab_group_board_list_get, gitlab_group_board_list_create, gitlab_group_board_get.",
			"Enumerate the columns of a group issue board to see how its workflow stages are arranged.",
			[]string{"list columns of a group issue board", "show lists on a group board", "view group board columns"}),
		groupBoardReadSpec("group_board_get_list", toolutil.RouteAction(client, GetGroupBoardList), "gitlab_group_board_list_get",
			"Get a single list (column) of a group issue board. Returns: the list with its assignee, label, iteration, and milestone scope. See also: gitlab_group_board_list_lists, gitlab_group_board_list_update, gitlab_group_board_list_delete.",
			"Inspect one column of a group issue board, including its label, assignee, and iteration scope.",
			[]string{"get a column of a group issue board", "show one list on a group board", "view a group board column"}),
		groupBoardCreateSpec("group_board_create_list", toolutil.RouteAction(client, CreateGroupBoardList), "gitlab_group_board_list_create",
			"Create a new list (column) on a group issue board from a group label. Returns: the created list with its label and scope. See also: gitlab_group_board_list_get, gitlab_group_board_list_update, gitlab_group_board_list_delete.",
			"Add a new column backed by a group label to a group issue board's workflow.",
			[]string{"add a column to a group issue board", "create a list on a group board", "new group board column from a label"}),
		groupBoardUpdateSpec("group_board_update_list", toolutil.RouteAction(client, UpdateGroupBoardList), "gitlab_group_board_list_update",
			"Reorder a list (column) within a group issue board. Returns: the repositioned list with its scope. See also: gitlab_group_board_list_get, gitlab_group_board_list_lists.",
			"Move a column to a new position in a group issue board to reorder its workflow stages.",
			[]string{"reorder a column on a group issue board", "move a list on a group board", "reposition a group board column"}),
		groupBoardDeleteSpec("group_board_delete_list", toolutil.DestructiveAction(client, deleteGroupBoardListOutput), "gitlab_group_board_list_delete",
			"Delete a list (column) from a group issue board. Returns: a success confirmation. See also: gitlab_group_board_list_get, gitlab_group_board_list_lists.",
			"Remove a column from a group issue board when that workflow stage is no longer used.",
			[]string{"delete a column from a group issue board", "remove a list from a group board", "drop a group board column"}),
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

func groupBoardReadSpec(name string, route toolutil.ActionRoute, individualTool, description, usage string, aliases []string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, groupBoardOptions(individualTool, description, usage, aliases))
}

func groupBoardCreateSpec(name string, route toolutil.ActionRoute, individualTool, description, usage string, aliases []string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, groupBoardOptions(individualTool, description, usage, aliases))
}

func groupBoardUpdateSpec(name string, route toolutil.ActionRoute, individualTool, description, usage string, aliases []string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, groupBoardOptions(individualTool, description, usage, aliases))
}

func groupBoardDeleteSpec(name string, route toolutil.ActionRoute, individualTool, description, usage string, aliases []string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, groupBoardOptions(individualTool, description, usage, aliases))
}

func groupBoardOptions(individualTool, description, usage string, aliases []string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: append([]string{individualTool}, aliases...), Usage: usage, Tags: []string{"group", "board"},
		RelatedActions: []string{"group.group_label_list", "group.issues"},
		OpenWorld:      true,
		OwnerPackage:   "groupboards",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool), Description: description},
	}
}
