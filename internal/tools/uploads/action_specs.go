package uploads

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for project upload actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		uploadCreateSpec("upload", toolutil.RouteActionWithRequest(client, Upload), "gitlab_project_upload"),
		uploadReadSpec("upload_list", toolutil.RouteAction(client, List), "gitlab_project_upload_list"),
		uploadDeleteSpec("upload_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_project_upload_delete"),
	}
}

func uploadReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := uploadOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func uploadCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, uploadOptions(individualTool))
}

func uploadDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := uploadOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func uploadOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"project", "upload"},
		RelatedActions: []string{"project.upload_list", "project.get"},
		OpenWorld:      true,
		OwnerPackage:   "uploads",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
