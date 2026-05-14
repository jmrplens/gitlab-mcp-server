package files

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for repository file actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		fileReadSpec("file_get", toolutil.RouteAction(client, Get), "gitlab_file_get"),
		fileCreateSpec("file_create", toolutil.RouteAction(client, Create), "gitlab_file_create"),
		fileUpdateSpec("file_update", toolutil.RouteAction(client, Update), "gitlab_file_update"),
		fileDeleteSpec("file_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_file_delete"),
		fileReadSpec("file_blame", toolutil.RouteAction(client, Blame), "gitlab_file_blame"),
		fileReadSpec("file_metadata", toolutil.RouteAction(client, GetMetaData), "gitlab_file_metadata"),
		fileReadSpec("file_raw", toolutil.RouteAction(client, GetRaw), "gitlab_file_raw"),
		fileReadSpec("file_raw_metadata", toolutil.RouteAction(client, GetRawFileMetaData), "gitlab_file_raw_metadata"),
	}
}

func fileReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := fileOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func fileCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, fileOptions(individualTool))
}

func fileUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := fileOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func fileDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := fileOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func fileOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"repository", "file"},
		RelatedActions: []string{"repository.tree", "repository.commit_list"},
		OpenWorld:      true,
		OwnerPackage:   "files",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
