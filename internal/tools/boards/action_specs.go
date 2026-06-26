package boards

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionBoardList     = "project.board_list"
	actionBoardGet      = "project.board_get"
	actionBoardListList = "project.board_list_list"
	actionBoardListGet  = "project.board_list_get"
)

// boardMeta carries the discovery metadata distinguishing one board action
// from another: a "Returns: … See also: …" individual-tool description, an
// action-specific usage sentence, distinctive natural-language aliases, and the
// canonical related actions (R-META, 1:1 audit).
type boardMeta struct {
	description string
	usage       string
	aliases     []string
	related     []string
}

// boardMetaByTool maps each individual board tool name to its non-generic
// discovery metadata.
var boardMetaByTool = map[string]boardMeta{
	"gitlab_board_list": {
		description: "List a project's issue boards with pagination. Returns: each board with its project, milestone, assignee, scope labels, and lists. See also: gitlab_board_get, gitlab_board_create, gitlab_board_list_lists.",
		usage:       "List every issue board configured in a project to see how the team's kanban work is organized.",
		aliases:     []string{"list project issue boards", "show kanban boards for a project", "view all boards in a project"},
		related:     []string{actionBoardGet, "project.board_create", actionBoardListList, "issue.list"},
	},
	"gitlab_board_get": {
		description: "Get a single issue board by board_id. Returns: the board with its project, milestone, assignee, scope labels, and ordered lists. See also: gitlab_board_list, gitlab_board_update, gitlab_board_delete.",
		usage:       "Fetch one issue board by its ID to inspect its scope filters and the ordered columns it contains.",
		aliases:     []string{"get an issue board by id", "show a specific kanban board", "inspect one project board"},
		related:     []string{actionBoardList, "project.board_update", "project.board_delete", actionBoardListList},
	},
	"gitlab_board_create": {
		description: "Create a new issue board in a project. Returns: the created board with its project, scope, and lists. See also: gitlab_board_get, gitlab_board_update, gitlab_board_list.",
		usage:       "Create a new issue board so a team can track work in a fresh kanban view of the project.",
		aliases:     []string{"create a project issue board", "add a new kanban board", "set up an issue board"},
		related:     []string{actionBoardGet, "project.board_update", actionBoardList},
	},
	"gitlab_board_update": {
		description: "Update an issue board's name, scope (assignee, milestone, labels, weight), or list visibility. Returns: the updated board with its project, scope, and lists. See also: gitlab_board_get, gitlab_board_delete, gitlab_board_list.",
		usage:       "Rename an issue board or retune its assignee, milestone, label, and weight scope, or toggle the Open and Closed columns.",
		aliases:     []string{"rename an issue board", "change board scope filters", "edit a kanban board's settings"},
		related:     []string{actionBoardGet, "project.board_delete", actionBoardList},
	},
	"gitlab_board_delete": {
		description: "Delete an issue board permanently. Returns: a success confirmation naming the board and project. See also: gitlab_board_get, gitlab_board_list.",
		usage:       "Permanently remove an issue board from a project when the team no longer tracks work on that kanban view.",
		aliases:     []string{"delete a project issue board", "remove a kanban board", "destroy an issue board"},
		related:     []string{actionBoardGet, actionBoardList},
	},
	"gitlab_board_list_lists": {
		description: "List the lists (columns) in an issue board with pagination. Returns: each list with its label, assignee, milestone, iteration, position, and issue limits. See also: gitlab_board_list_get, gitlab_board_list_create, gitlab_board_get.",
		usage:       "Enumerate the columns of an issue board in order to see how the kanban workflow stages are arranged.",
		aliases:     []string{"list columns on an issue board", "show board lists in a kanban board", "view an issue board's stages"},
		related:     []string{actionBoardListGet, "project.board_list_create", actionBoardGet},
	},
	"gitlab_board_list_get": {
		description: "Get a single board list by list_id. Returns: the list with its label, assignee, milestone, iteration, position, and issue limits. See also: gitlab_board_list_lists, gitlab_board_list_update, gitlab_board_list_delete.",
		usage:       "Fetch one column of an issue board to inspect its label, assignee, milestone, or iteration scope and issue limits.",
		aliases:     []string{"get one issue board column", "inspect a single board list", "show a kanban column's scope"},
		related:     []string{actionBoardListList, "project.board_list_update", "project.board_list_delete"},
	},
	"gitlab_board_list_create": {
		description: "Create a board list (column) scoped to a label, assignee, milestone, or iteration. Returns: the created list with its scope, position, and issue limits. See also: gitlab_board_list_get, gitlab_board_list_update, gitlab_board_list_lists.",
		usage:       "Add a new column to an issue board scoped to a label, assignee, milestone, or iteration to capture a workflow stage.",
		aliases:     []string{"add a column to an issue board", "create a board list for a label", "add a kanban stage to a board"},
		related:     []string{actionBoardListGet, "project.board_list_update", actionBoardListList},
	},
	"gitlab_board_list_update": {
		description: "Reorder a board list to a new position. Returns: the updated list with its scope, position, and issue limits. See also: gitlab_board_list_get, gitlab_board_list_lists, gitlab_board_list_delete.",
		usage:       "Move an issue board column to a new position to reorder the kanban workflow stages.",
		aliases:     []string{"reorder an issue board column", "move a board list position", "rearrange kanban columns"},
		related:     []string{actionBoardListGet, actionBoardListList, "project.board_list_delete"},
	},
	"gitlab_board_list_delete": {
		description: "Delete a board list (column) permanently. Returns: a success confirmation naming the list. See also: gitlab_board_list_get, gitlab_board_list_lists.",
		usage:       "Permanently remove a column from an issue board when that kanban workflow stage is no longer needed.",
		aliases:     []string{"delete an issue board column", "remove a board list", "drop a kanban column from a board"},
		related:     []string{actionBoardListGet, actionBoardListList},
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
	aliases := []string{individualTool}
	usage := "Use to execute boards domain action."
	individual := toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)}
	if meta, ok := boardMetaByTool[individualTool]; ok {
		related = append([]string(nil), meta.related...)
		individual.Description = meta.description
		if meta.usage != "" {
			usage = meta.usage
		}
		if len(meta.aliases) > 0 {
			aliases = append([]string{individualTool}, meta.aliases...)
		}
	}
	return toolutil.ActionSpecOptions{
		Aliases: aliases, Usage: usage, Tags: []string{"project", "board"},
		RelatedActions: related,
		OpenWorld:      true,
		OwnerPackage:   "boards",
		IndividualTool: individual,
	}
}
