package groupmarkdownuploads

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
	return toolutil.NewReadActionSpec(name, route, groupUploadOptions(individualTool))
}

func groupUploadDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, groupUploadOptions(individualTool))
}

func groupUploadOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute groupmarkdownuploads domain action.", Tags: []string{"group", "upload"},
		RelatedActions: []string{"group.get"},
		OpenWorld:      true,
		OwnerPackage:   "groupmarkdownuploads",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
