package groupepicboards

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// epicBoardMeta carries the non-generic discovery metadata for a group epic
// board action: natural-language aliases, related action IDs, and the
// "Returns: … See also: …" individual-tool description (R-META).
type epicBoardMeta struct {
	aliases     []string
	related     []string
	description string
}

// epicBoardMetaByName maps each canonical action name to its discovery metadata.
var epicBoardMetaByName = map[string]epicBoardMeta{
	"epic_board_list": {
		aliases: []string{"list epic boards", "show group epic boards", "find epic boards in group"},
		related: []string{"group.epic_list", "group.epics"},
		description: "List all epic boards in a group with pagination (Premium/Ultimate). " +
			"Returns: each epic board with its group, scope labels, and lists (columns) with their assignee, label, iteration, and milestone scope, plus pagination metadata. " +
			"See also: gitlab_group_epic_board_get, gitlab_group_epic_list.",
	},
	"epic_board_get": {
		aliases: []string{"get epic board", "show epic board details", "fetch epic board"},
		related: []string{"group.epic_board_list", "group.epic_list"},
		description: "Get a single group epic board by ID (Premium/Ultimate). " +
			"Returns: the epic board with its group, scope labels, and lists (columns) with their assignee, label, iteration, and milestone scope. " +
			"See also: gitlab_group_epic_board_list, gitlab_group_epic_list.",
	},
}

// ActionSpecs returns canonical specs for group epic board actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupEpicBoardReadSpec("epic_board_list", toolutil.RouteAction(client, List), "gitlab_group_epic_board_list"),
		groupEpicBoardReadSpec("epic_board_get", toolutil.RouteAction(client, Get), "gitlab_group_epic_board_get"),
	}
}

func groupEpicBoardReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, groupEpicBoardOptions(name, individualTool))
}

func groupEpicBoardOptions(name, individualTool string) toolutil.ActionSpecOptions {
	opts := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute groupepicboards domain action.", Tags: []string{"group", "epic", "board"},
		RelatedActions: []string{"group.epic_list"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "groupepicboards",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	if meta, ok := epicBoardMetaByName[name]; ok {
		opts.Aliases = append([]string{individualTool}, meta.aliases...)
		opts.RelatedActions = append([]string(nil), meta.related...)
		opts.IndividualTool.Description = meta.description
	}
	return opts
}
