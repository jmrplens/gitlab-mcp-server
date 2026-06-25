package groupstoragemoves

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical action IDs for group repository storage moves. The base domain is
// "storage_move" (derived from the gitlab_storage_move catalog tool name), so
// these IDs are used in RelatedActions cross-links.
const (
	actionRetrieveAllGroup = "storage_move.retrieve_all_group"
	actionRetrieveGroup    = "storage_move.retrieve_group"
	actionGetGroup         = "storage_move.get_group"
	actionGetGroupForGroup = "storage_move.get_group_for_group"
	actionScheduleGroup    = "storage_move.schedule_group"
	actionScheduleAllGroup = "storage_move.schedule_all_group"
)

// groupStorageMoveMeta carries the per-action discovery metadata applied to
// each group storage move spec: a purpose-specific usage sentence, natural
// language aliases, RelatedActions cross-links, and the "Returns: … See also:
// …" individual-tool description.
type groupStorageMoveMeta struct {
	usage       string
	aliases     []string
	related     []string
	description string
}

// ActionSpecs returns canonical specs for group repository storage move actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupStorageMoveReadSpec("retrieve_all_group", toolutil.RouteAction(client, RetrieveAll), "gitlab_retrieve_all_group_storage_moves", groupStorageMoveMeta{
			usage:   "List every group repository storage move across the instance (admin, Premium/Ultimate, self-managed).",
			aliases: []string{"list all group repository storage moves", "all group storage moves", "group gitaly moves"},
			related: []string{actionRetrieveGroup, actionGetGroup, actionScheduleAllGroup},
			description: "List all group repository storage moves across the instance. Returns: storage moves with state, source and destination storage names, group reference, creation time, and pagination metadata. " +
				"See also: gitlab_retrieve_group_storage_moves, gitlab_get_group_storage_move, gitlab_schedule_all_group_storage_moves.",
		}),
		groupStorageMoveReadSpec("retrieve_group", toolutil.RouteAction(client, RetrieveForGroup), "gitlab_retrieve_group_storage_moves", groupStorageMoveMeta{
			usage:   "List repository storage moves for one group by numeric group ID.",
			aliases: []string{"list storage moves for a group", "group repository storage moves", "moves for group"},
			related: []string{actionRetrieveAllGroup, actionGetGroupForGroup, actionScheduleGroup},
			description: "List repository storage moves for a single group. Returns: the group's storage moves with state, source and destination storage names, creation time, and pagination metadata. " +
				"See also: gitlab_retrieve_all_group_storage_moves, gitlab_get_group_storage_move_for_group, gitlab_schedule_group_storage_move.",
		}),
		groupStorageMoveReadSpec("get_group", toolutil.RouteAction(client, Get), "gitlab_get_group_storage_move", groupStorageMoveMeta{
			usage:   "Get one group repository storage move by its numeric storage move ID.",
			aliases: []string{"get a group storage move", "show group storage move", "group storage move by id"},
			related: []string{actionRetrieveAllGroup, actionGetGroupForGroup},
			description: "Get a single group repository storage move by ID. Returns: the storage move with state, source and destination storage names, group reference, and creation time. " +
				"See also: gitlab_retrieve_all_group_storage_moves, gitlab_get_group_storage_move_for_group.",
		}),
		groupStorageMoveReadSpec("get_group_for_group", toolutil.RouteAction(client, GetForGroup), "gitlab_get_group_storage_move_for_group", groupStorageMoveMeta{
			usage:   "Get one repository storage move scoped to a specific group by group ID and storage move ID.",
			aliases: []string{"get a storage move for a group", "show group-scoped storage move", "group storage move for group"},
			related: []string{actionRetrieveGroup, actionGetGroup},
			description: "Get a single repository storage move for a specific group. Returns: the storage move with state, source and destination storage names, group reference, and creation time. " +
				"See also: gitlab_retrieve_group_storage_moves, gitlab_get_group_storage_move.",
		}),
		groupStorageMoveCreateSpec("schedule_group", toolutil.RouteAction(client, Schedule), "gitlab_schedule_group_storage_move", groupStorageMoveMeta{
			usage:   "Schedule a repository storage move for one group to a destination Gitaly shard.",
			aliases: []string{"schedule a storage move for a group", "move group repository to another shard", "migrate group gitaly storage"},
			related: []string{actionRetrieveGroup, actionGetGroupForGroup, actionScheduleAllGroup},
			description: "Schedule a repository storage move for a single group. Returns: the scheduled storage move with state, source and destination storage names, and group reference. " +
				"See also: gitlab_retrieve_group_storage_moves, gitlab_get_group_storage_move_for_group, gitlab_schedule_all_group_storage_moves.",
		}),
		groupStorageMoveCreateSpec("schedule_all_group", toolutil.RouteAction(client, ScheduleAll), "gitlab_schedule_all_group_storage_moves", groupStorageMoveMeta{
			usage:   "Schedule storage moves for all groups on a source Gitaly shard to a destination shard (bulk).",
			aliases: []string{"schedule storage moves for all groups", "bulk move groups off a shard", "drain group gitaly shard"},
			related: []string{actionRetrieveAllGroup, actionScheduleGroup},
			description: "Schedule repository storage moves for all groups on a source storage shard. Returns: a confirmation that the bulk move has been scheduled. " +
				"See also: gitlab_retrieve_all_group_storage_moves, gitlab_schedule_group_storage_move.",
		}),
	}
}

func groupStorageMoveReadSpec(name string, route toolutil.ActionRoute, individualTool string, meta groupStorageMoveMeta) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, groupStorageMoveOptions(individualTool, meta))
}

func groupStorageMoveCreateSpec(name string, route toolutil.ActionRoute, individualTool string, meta groupStorageMoveMeta) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, groupStorageMoveOptions(individualTool, meta))
}

func groupStorageMoveOptions(individualTool string, meta groupStorageMoveMeta) toolutil.ActionSpecOptions {
	aliases := make([]string, 0, len(meta.aliases)+1)
	aliases = append(aliases, individualTool)
	aliases = append(aliases, meta.aliases...)
	return toolutil.ActionSpecOptions{
		Aliases:        aliases,
		Usage:          meta.usage,
		Tags:           []string{"storage_move", "group"},
		RelatedActions: meta.related,
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "groupstoragemoves",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:        individualTool,
			Title:       toolutil.TitleFromName(individualTool),
			Description: meta.description,
		},
	}
}
