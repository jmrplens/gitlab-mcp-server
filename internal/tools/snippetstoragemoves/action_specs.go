package snippetstoragemoves

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// snippetStorageMoveMeta carries the non-generic discovery metadata for one
// snippet storage move action: its individual tool name, a "Returns: … See
// also: …" description, and the related action tool names surfaced to clients.
type snippetStorageMoveMeta struct {
	tool        string
	description string
	related     []string
}

// ActionSpecs returns canonical specs for snippet repository storage move actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_retrieve_all_snippet_storage_moves — list every snippet repository storage move.
		snippetStorageMoveReadSpec("retrieve_all_snippet", toolutil.RouteAction(client, RetrieveAll), snippetStorageMoveMeta{
			tool:        "gitlab_retrieve_all_snippet_storage_moves",
			description: "List every snippet repository storage move on the instance with ordering and pagination. Returns: storage moves with state, source and destination storage shards, the associated snippet, and pagination metadata. See also: gitlab_get_snippet_storage_move, gitlab_retrieve_snippet_storage_moves, gitlab_schedule_all_snippet_storage_moves.",
			related:     []string{"gitlab_get_snippet_storage_move", "gitlab_retrieve_snippet_storage_moves", "gitlab_schedule_all_snippet_storage_moves"},
		}),
		// gitlab_retrieve_snippet_storage_moves — list storage moves for one snippet.
		snippetStorageMoveReadSpec("retrieve_snippet", toolutil.RouteAction(client, RetrieveForSnippet), snippetStorageMoveMeta{
			tool:        "gitlab_retrieve_snippet_storage_moves",
			description: "List repository storage moves for one snippet with ordering and pagination. Returns: storage moves scoped to the snippet with state, source and destination storage shards, and pagination metadata. See also: gitlab_get_snippet_storage_move_for_snippet, gitlab_retrieve_all_snippet_storage_moves, gitlab_schedule_snippet_storage_move.",
			related:     []string{"gitlab_get_snippet_storage_move_for_snippet", "gitlab_retrieve_all_snippet_storage_moves", "gitlab_schedule_snippet_storage_move"},
		}),
		// gitlab_get_snippet_storage_move — fetch a single snippet storage move by ID.
		snippetStorageMoveReadSpec("get_snippet", toolutil.RouteAction(client, Get), snippetStorageMoveMeta{
			tool:        "gitlab_get_snippet_storage_move",
			description: "Get a single snippet repository storage move by its ID. Returns: the storage move with state, source and destination storage shards, and the associated snippet. See also: gitlab_retrieve_all_snippet_storage_moves, gitlab_get_snippet_storage_move_for_snippet.",
			related:     []string{"gitlab_retrieve_all_snippet_storage_moves", "gitlab_get_snippet_storage_move_for_snippet"},
		}),
		// gitlab_get_snippet_storage_move_for_snippet — fetch one storage move scoped to a snippet.
		snippetStorageMoveReadSpec("get_snippet_for_snippet", toolutil.RouteAction(client, GetForSnippet), snippetStorageMoveMeta{
			tool:        "gitlab_get_snippet_storage_move_for_snippet",
			description: "Get a single repository storage move scoped to one snippet by snippet ID and move ID. Returns: the storage move with state, source and destination storage shards, and the associated snippet. See also: gitlab_retrieve_snippet_storage_moves, gitlab_get_snippet_storage_move.",
			related:     []string{"gitlab_retrieve_snippet_storage_moves", "gitlab_get_snippet_storage_move"},
		}),
		// gitlab_schedule_snippet_storage_move — schedule a repository storage move for a single snippet.
		snippetStorageMoveCreateSpec("schedule_snippet", toolutil.RouteAction(client, Schedule), snippetStorageMoveMeta{
			tool:        "gitlab_schedule_snippet_storage_move",
			description: "Schedule a repository storage move for one snippet to a destination Gitaly storage shard. Returns: the scheduled storage move with its initial state and the associated snippet. See also: gitlab_retrieve_snippet_storage_moves, gitlab_get_snippet_storage_move_for_snippet, gitlab_schedule_all_snippet_storage_moves.",
			related:     []string{"gitlab_retrieve_snippet_storage_moves", "gitlab_get_snippet_storage_move_for_snippet", "gitlab_schedule_all_snippet_storage_moves"},
		}),
		// gitlab_schedule_all_snippet_storage_moves — schedule moves for all snippets on a source shard.
		snippetStorageMoveCreateSpec("schedule_all_snippet", toolutil.RouteAction(client, ScheduleAll), snippetStorageMoveMeta{
			tool:        "gitlab_schedule_all_snippet_storage_moves",
			description: "Schedule repository storage moves for all snippets on a source Gitaly storage shard. Returns: a confirmation that the bulk move was scheduled. See also: gitlab_retrieve_all_snippet_storage_moves, gitlab_schedule_snippet_storage_move.",
			related:     []string{"gitlab_retrieve_all_snippet_storage_moves", "gitlab_schedule_snippet_storage_move"},
		}),
	}
}

// snippetStorageMoveReadSpec builds the canonical read-only spec for a snippet storage move tool.
func snippetStorageMoveReadSpec(name string, route toolutil.ActionRoute, meta snippetStorageMoveMeta) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, snippetStorageMoveOptions(meta))
}

// snippetStorageMoveCreateSpec builds the canonical create spec for a snippet storage move tool.
func snippetStorageMoveCreateSpec(name string, route toolutil.ActionRoute, meta snippetStorageMoveMeta) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, snippetStorageMoveOptions(meta))
}

func snippetStorageMoveOptions(meta snippetStorageMoveMeta) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{meta.tool}, Usage: "Use to execute snippetstoragemoves domain action.", Tags: []string{"storage_move", "snippet"},
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "snippetstoragemoves",
		RelatedActions: meta.related,
		IndividualTool: toolutil.IndividualToolSpec{Name: meta.tool, Title: toolutil.TitleFromName(meta.tool), Description: meta.description},
	}
}
