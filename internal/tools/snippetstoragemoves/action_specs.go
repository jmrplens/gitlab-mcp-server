package snippetstoragemoves

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for snippet repository storage move actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_retrieve_all_snippet_storage_moves — list every snippet repository storage move.
		snippetStorageMoveReadSpec("retrieve_all_snippet", toolutil.RouteAction(client, RetrieveAll), "gitlab_retrieve_all_snippet_storage_moves"),
		// gitlab_retrieve_snippet_storage_moves — list storage moves for one snippet.
		snippetStorageMoveReadSpec("retrieve_snippet", toolutil.RouteAction(client, RetrieveForSnippet), "gitlab_retrieve_snippet_storage_moves"),
		// gitlab_get_snippet_storage_move — fetch a single snippet storage move by ID.
		snippetStorageMoveReadSpec("get_snippet", toolutil.RouteAction(client, Get), "gitlab_get_snippet_storage_move"),
		// gitlab_get_snippet_storage_move_for_snippet — fetch one storage move scoped to a snippet.
		snippetStorageMoveReadSpec("get_snippet_for_snippet", toolutil.RouteAction(client, GetForSnippet), "gitlab_get_snippet_storage_move_for_snippet"),
		// gitlab_schedule_snippet_storage_move — schedule a repository storage move for a single snippet.
		snippetStorageMoveCreateSpec("schedule_snippet", toolutil.RouteAction(client, Schedule), "gitlab_schedule_snippet_storage_move"),
		// gitlab_schedule_all_snippet_storage_moves — schedule moves for all snippets on a source shard.
		snippetStorageMoveCreateSpec("schedule_all_snippet", toolutil.RouteAction(client, ScheduleAll), "gitlab_schedule_all_snippet_storage_moves"),
	}
}

// snippetStorageMoveReadSpec builds the canonical read-only spec for a snippet storage move tool.
func snippetStorageMoveReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, snippetStorageMoveOptions(individualTool))
}

// snippetStorageMoveCreateSpec builds the canonical create spec for a snippet storage move tool.
func snippetStorageMoveCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, snippetStorageMoveOptions(individualTool))
}

func snippetStorageMoveOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute snippetstoragemoves domain action.", Tags: []string{"storage_move", "snippet"},
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "snippetstoragemoves",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
