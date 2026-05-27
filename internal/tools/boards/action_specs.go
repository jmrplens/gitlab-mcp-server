package boards

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for project issue board actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		boardReadSpec("board_list", toolutil.RouteAction(client, ListBoards), "gitlab_board_list"),
		boardReadSpec("board_get", toolutil.RouteAction(client, GetBoard), "gitlab_board_get"),
		boardCreateSpec("board_create", toolutil.RouteAction(client, CreateBoard), "gitlab_board_create"),
		boardUpdateSpec("board_update", toolutil.RouteAction(client, UpdateBoard), "gitlab_board_update"),
		boardDeleteSpec("board_delete", toolutil.DestructiveVoidAction(client, DeleteBoard), "gitlab_board_delete"),
		boardReadSpec("board_list_list", toolutil.RouteAction(client, ListBoardLists), "gitlab_board_list_lists"),
		boardReadSpec("board_list_get", toolutil.RouteAction(client, GetBoardList), "gitlab_board_list_get"),
		boardCreateSpec("board_list_create", toolutil.RouteAction(client, CreateBoardList), "gitlab_board_list_create"),
		boardUpdateSpec("board_list_update", toolutil.RouteAction(client, UpdateBoardList), "gitlab_board_list_update"),
		boardDeleteSpec("board_list_delete", toolutil.DestructiveVoidAction(client, DeleteBoardList), "gitlab_board_list_delete"),
	}
}

func boardReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, boardOptions(individualTool))
}

func boardCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, boardOptions(individualTool))
}

func boardUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, boardOptions(individualTool))
}

func boardDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, boardOptions(individualTool))
}

func boardOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute boards domain action.", Tags: []string{"project", "board"},
		RelatedActions: []string{"project.label_list", "issue.list"},
		OpenWorld:      true,
		OwnerPackage:   "boards",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
