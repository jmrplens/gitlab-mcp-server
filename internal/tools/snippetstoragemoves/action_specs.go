package snippetstoragemoves

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// snippetStorageMoveMeta carries the non-generic discovery metadata for one
// snippet storage move action: its individual tool name, an action-specific
// usage sentence, distinctive natural-language aliases, a "Returns: … See
// also: …" description, and the related action tool names surfaced to clients.
type snippetStorageMoveMeta struct {
	tool        string
	usage       string
	aliases     []string
	description string
	related     []string
}

// ActionSpecs returns canonical specs for snippet repository storage move actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_retrieve_all_snippet_storage_moves — list every snippet repository storage move.
		snippetStorageMoveReadSpec("retrieve_all_snippet", toolutil.RouteAction(client, RetrieveAll), snippetStorageMoveMeta{
			tool:        "gitlab_retrieve_all_snippet_storage_moves",
			usage:       "List every snippet repository storage move across the whole instance. Use to audit in-flight or completed Gitaly shard migrations of snippet repositories, ordered and paginated, when you do not have a specific snippet in mind.",
			aliases:     []string{"list all snippet repository storage moves", "audit instance-wide snippet shard migrations", "show every snippet storage move"},
			description: "List every snippet repository storage move on the instance with ordering and pagination. Returns: storage moves with state, source and destination storage shards, the associated snippet, and pagination metadata. See also: gitlab_get_snippet_storage_move, gitlab_retrieve_snippet_storage_moves, gitlab_schedule_all_snippet_storage_moves.",
			related:     []string{"gitlab_get_snippet_storage_move", "gitlab_retrieve_snippet_storage_moves", "gitlab_schedule_all_snippet_storage_moves"},
		}),
		// gitlab_retrieve_snippet_storage_moves — list storage moves for one snippet.
		snippetStorageMoveReadSpec("retrieve_snippet", toolutil.RouteAction(client, RetrieveForSnippet), snippetStorageMoveMeta{
			tool:        "gitlab_retrieve_snippet_storage_moves",
			usage:       "List repository storage moves scoped to one snippet by snippet ID. Use to track the Gitaly shard migration history of a single snippet's repository, ordered and paginated.",
			aliases:     []string{"list storage moves for a snippet", "track one snippet's shard migrations", "show a snippet's repository storage moves"},
			description: "List repository storage moves for one snippet with ordering and pagination. Returns: storage moves scoped to the snippet with state, source and destination storage shards, and pagination metadata. See also: gitlab_get_snippet_storage_move_for_snippet, gitlab_retrieve_all_snippet_storage_moves, gitlab_schedule_snippet_storage_move.",
			related:     []string{"gitlab_get_snippet_storage_move_for_snippet", "gitlab_retrieve_all_snippet_storage_moves", "gitlab_schedule_snippet_storage_move"},
		}),
		// gitlab_get_snippet_storage_move — fetch a single snippet storage move by ID.
		snippetStorageMoveReadSpec("get_snippet", toolutil.RouteAction(client, Get), snippetStorageMoveMeta{
			tool:        "gitlab_get_snippet_storage_move",
			usage:       "Fetch a single snippet repository storage move by its global move ID. Use when you already have a move ID from a list and need its current state, source shard, and destination shard.",
			aliases:     []string{"get a snippet storage move by id", "show one snippet repository storage move", "check snippet shard migration status by move id"},
			description: "Get a single snippet repository storage move by its ID. Returns: the storage move with state, source and destination storage shards, and the associated snippet. See also: gitlab_retrieve_all_snippet_storage_moves, gitlab_get_snippet_storage_move_for_snippet.",
			related:     []string{"gitlab_retrieve_all_snippet_storage_moves", "gitlab_get_snippet_storage_move_for_snippet"},
		}),
		// gitlab_get_snippet_storage_move_for_snippet — fetch one storage move scoped to a snippet.
		snippetStorageMoveReadSpec("get_snippet_for_snippet", toolutil.RouteAction(client, GetForSnippet), snippetStorageMoveMeta{
			tool:        "gitlab_get_snippet_storage_move_for_snippet",
			usage:       "Fetch a single repository storage move scoped to a specific snippet, using both the snippet ID and the move ID. Use to confirm a particular move belongs to that snippet and inspect its state and shards.",
			aliases:     []string{"get a snippet's storage move by snippet and move id", "show one move scoped to a snippet", "verify a snippet repository storage move"},
			description: "Get a single repository storage move scoped to one snippet by snippet ID and move ID. Returns: the storage move with state, source and destination storage shards, and the associated snippet. See also: gitlab_retrieve_snippet_storage_moves, gitlab_get_snippet_storage_move.",
			related:     []string{"gitlab_retrieve_snippet_storage_moves", "gitlab_get_snippet_storage_move"},
		}),
		// gitlab_schedule_snippet_storage_move — schedule a repository storage move for a single snippet.
		snippetStorageMoveCreateSpec("schedule_snippet", toolutil.RouteAction(client, Schedule), snippetStorageMoveMeta{
			tool:        "gitlab_schedule_snippet_storage_move",
			usage:       "Schedule a repository storage move for one snippet onto a destination Gitaly storage shard. Use to migrate a single snippet's repository when rebalancing or evacuating a shard. The move runs asynchronously.",
			aliases:     []string{"migrate one snippet's repository to a shard", "schedule a snippet storage move", "move a snippet repository to another Gitaly shard"},
			description: "Schedule a repository storage move for one snippet to a destination Gitaly storage shard. Returns: the scheduled storage move with its initial state and the associated snippet. See also: gitlab_retrieve_snippet_storage_moves, gitlab_get_snippet_storage_move_for_snippet, gitlab_schedule_all_snippet_storage_moves.",
			related:     []string{"gitlab_retrieve_snippet_storage_moves", "gitlab_get_snippet_storage_move_for_snippet", "gitlab_schedule_all_snippet_storage_moves"},
		}),
		// gitlab_schedule_all_snippet_storage_moves — schedule moves for all snippets on a source shard.
		snippetStorageMoveCreateSpec("schedule_all_snippet", toolutil.RouteAction(client, ScheduleAll), snippetStorageMoveMeta{
			tool:        "gitlab_schedule_all_snippet_storage_moves",
			usage:       "Schedule repository storage moves for every snippet sitting on a given source Gitaly storage shard. Use to bulk-evacuate or drain a shard of all snippet repositories in one operation. Moves run asynchronously.",
			aliases:     []string{"bulk-migrate all snippets off a shard", "drain a Gitaly shard of snippet repositories", "schedule storage moves for all snippets on a source shard"},
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

// snippetStorageMoveAliases returns the action's distinctive natural-language
// aliases followed by its individual tool name, so discovery matches both the
// snippet-repository-storage-move phrasing and the canonical tool name.
func snippetStorageMoveAliases(meta snippetStorageMoveMeta) []string {
	aliases := append([]string(nil), meta.aliases...)
	return append(aliases, meta.tool)
}

func snippetStorageMoveOptions(meta snippetStorageMoveMeta) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: snippetStorageMoveAliases(meta), Usage: meta.usage, Tags: []string{"storage_move", "snippet"},
		// Snippet repository storage moves are a Free-tier admin API (self-managed
		// only). doc/api/snippet_repository_storage_moves.md page tier = Free, Premium, Ultimate.
		OpenWorld:      true,
		OwnerPackage:   "snippetstoragemoves",
		RelatedActions: meta.related,
		IndividualTool: toolutil.IndividualToolSpec{Name: meta.tool, Title: toolutil.TitleFromName(meta.tool), Description: meta.description},
	}
}
