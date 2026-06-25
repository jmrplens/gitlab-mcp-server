package boards

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// boardMeta carries the discovery metadata distinguishing one board action
// from another: a "Returns: … See also: …" individual-tool description and the
// canonical related actions (R-META, 1:1 audit).
type boardMeta struct {
	description string
	related     []string
}

// boardMetaByTool maps each individual board tool name to its non-generic
// discovery metadata.
var boardMetaByTool = map[string]boardMeta{
	"gitlab_board_list": {
		description: "List a project's issue boards with pagination. Returns: each board with its project, milestone, assignee, scope labels, and lists. See also: gitlab_board_get, gitlab_board_create, gitlab_board_list_lists.",
		related:     []string{"project.board_get", "project.board_create", "project.board_list_list", "issue.list"},
	},
	"gitlab_board_get": {
		description: "Get a single issue board by board_id. Returns: the board with its project, milestone, assignee, scope labels, and ordered lists. See also: gitlab_board_list, gitlab_board_update, gitlab_board_delete.",
		related:     []string{"project.board_list", "project.board_update", "project.board_delete", "project.board_list_list"},
	},
	"gitlab_board_create": {
		description: "Create a new issue board in a project. Returns: the created board with its project, scope, and lists. See also: gitlab_board_get, gitlab_board_update, gitlab_board_list.",
		related:     []string{"project.board_get", "project.board_update", "project.board_list"},
	},
	"gitlab_board_update": {
		description: "Update an issue board's name, scope (assignee, milestone, labels, weight), or list visibility. Returns: the updated board with its project, scope, and lists. See also: gitlab_board_get, gitlab_board_delete, gitlab_board_list.",
		related:     []string{"project.board_get", "project.board_delete", "project.board_list"},
	},
	"gitlab_board_delete": {
		description: "Delete an issue board permanently. Returns: a success confirmation naming the board and project. See also: gitlab_board_get, gitlab_board_list.",
		related:     []string{"project.board_get", "project.board_list"},
	},
	"gitlab_board_list_lists": {
		description: "List the lists (columns) in an issue board with pagination. Returns: each list with its label, assignee, milestone, iteration, position, and issue limits. See also: gitlab_board_list_get, gitlab_board_list_create, gitlab_board_get.",
		related:     []string{"project.board_list_get", "project.board_list_create", "project.board_get"},
	},
	"gitlab_board_list_get": {
		description: "Get a single board list by list_id. Returns: the list with its label, assignee, milestone, iteration, position, and issue limits. See also: gitlab_board_list_lists, gitlab_board_list_update, gitlab_board_list_delete.",
		related:     []string{"project.board_list_list", "project.board_list_update", "project.board_list_delete"},
	},
	"gitlab_board_list_create": {
		description: "Create a board list (column) scoped to a label, assignee, milestone, or iteration. Returns: the created list with its scope, position, and issue limits. See also: gitlab_board_list_get, gitlab_board_list_update, gitlab_board_list_lists.",
		related:     []string{"project.board_list_get", "project.board_list_update", "project.board_list_list"},
	},
	"gitlab_board_list_update": {
		description: "Reorder a board list to a new position. Returns: the updated list with its scope, position, and issue limits. See also: gitlab_board_list_get, gitlab_board_list_lists, gitlab_board_list_delete.",
		related:     []string{"project.board_list_get", "project.board_list_list", "project.board_list_delete"},
	},
	"gitlab_board_list_delete": {
		description: "Delete a board list (column) permanently. Returns: a success confirmation naming the list. See also: gitlab_board_list_get, gitlab_board_list_lists.",
		related:     []string{"project.board_list_get", "project.board_list_list"},
	},
}

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
	related := []string{"project.label_list", "issue.list"}
	individual := toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)}
	if meta, ok := boardMetaByTool[individualTool]; ok {
		related = append([]string(nil), meta.related...)
		individual.Description = meta.description
	}
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute boards domain action.", Tags: []string{"project", "board"},
		RelatedActions: related,
		OpenWorld:      true,
		OwnerPackage:   "boards",
		IndividualTool: individual,
	}
}
