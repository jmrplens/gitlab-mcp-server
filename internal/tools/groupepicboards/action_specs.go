package groupepicboards

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for group epic board actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupEpicBoardReadSpec("epic_board_list", toolutil.RouteAction(client, List), "gitlab_group_epic_board_list"),
		groupEpicBoardReadSpec("epic_board_get", toolutil.RouteAction(client, Get), "gitlab_group_epic_board_get"),
	}
}

func groupEpicBoardReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupEpicBoardOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func groupEpicBoardOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"group", "epic", "board"},
		RelatedActions: []string{"group.epic_list"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "groupepicboards",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
