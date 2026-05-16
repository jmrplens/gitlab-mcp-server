package groupmarkdownuploads

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for group markdown upload actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupUploadReadSpec("group_upload_list", toolutil.RouteAction(client, List), "gitlab_list_group_markdown_uploads"),
		groupUploadDeleteSpec("group_upload_delete_by_id", toolutil.DestructiveAction(client, deleteByIDOutput), "gitlab_delete_group_markdown_upload_by_id"),
		groupUploadDeleteSpec("group_upload_delete_by_secret", toolutil.DestructiveAction(client, deleteBySecretAndFilenameOutput), "gitlab_delete_group_markdown_upload_by_secret"),
	}
}

func groupUploadReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupUploadOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func groupUploadDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupUploadOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func groupUploadOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"group", "upload"},
		RelatedActions: []string{"group.get"},
		OpenWorld:      true,
		OwnerPackage:   "groupmarkdownuploads",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
