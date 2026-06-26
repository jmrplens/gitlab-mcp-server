package groupmarkdownuploads

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionGroupUploadList           = "group_upload_list"
	actionGroupUploadDeleteByID     = "group_upload_delete_by_id"
	actionGroupUploadDeleteBySecret = "group_upload_delete_by_secret"
	actionGroupGet                  = "group.get"
)

// ActionSpecs returns canonical specs for group markdown upload actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupUploadReadSpec(actionGroupUploadList, toolutil.RouteAction(client, List), "gitlab_list_group_markdown_uploads",
			"Use to enumerate files uploaded into a group's Markdown attachments store.",
			"List all Markdown uploads in a group. Returns: each upload's id, filename, size, creation time, and uploader (id, username, name, state, avatar, web URL), plus pagination metadata. See also: gitlab_delete_group_markdown_upload_by_id, gitlab_delete_group_markdown_upload_by_secret, gitlab_group_get.",
			[]string{actionGroupGet, actionGroupUploadDeleteByID, actionGroupUploadDeleteBySecret},
			[]string{"list group markdown uploads", "show a group's uploaded attachments", "enumerate group description attachments", "browse group markdown files"}),
		groupUploadDeleteSpec(actionGroupUploadDeleteByID, toolutil.DestructiveAction(client, deleteByIDOutput), "gitlab_delete_group_markdown_upload_by_id",
			"Use to permanently remove a group Markdown upload identified by its numeric upload ID.",
			"Delete a group Markdown upload by its numeric upload ID. Returns: a success confirmation. See also: gitlab_list_group_markdown_uploads, gitlab_delete_group_markdown_upload_by_secret.",
			[]string{actionGroupUploadList, actionGroupUploadDeleteBySecret, actionGroupGet},
			[]string{"delete group markdown upload by id", "remove group attachment by upload id", "purge group markdown file by numeric id"}),
		groupUploadDeleteSpec(actionGroupUploadDeleteBySecret, toolutil.DestructiveAction(client, deleteBySecretAndFilenameOutput), "gitlab_delete_group_markdown_upload_by_secret",
			"Use to permanently remove a group Markdown upload identified by its secret and filename.",
			"Delete a group Markdown upload by its 32-character secret and filename. Returns: a success confirmation. See also: gitlab_list_group_markdown_uploads, gitlab_delete_group_markdown_upload_by_id.",
			[]string{actionGroupUploadList, actionGroupUploadDeleteByID, actionGroupGet},
			[]string{"delete group markdown upload by secret", "remove group attachment by secret and filename", "purge group markdown file by secret"}),
	}
}

func groupUploadReadSpec(name string, route toolutil.ActionRoute, individualTool, usage, description string, related, aliases []string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, groupUploadOptions(individualTool, usage, description, related, aliases))
}

func groupUploadDeleteSpec(name string, route toolutil.ActionRoute, individualTool, usage, description string, related, aliases []string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, groupUploadOptions(individualTool, usage, description, related, aliases))
}

func groupUploadOptions(individualTool, usage, description string, related, aliases []string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: append([]string{individualTool}, aliases...), Usage: usage, Tags: []string{"group", "upload"},
		RelatedActions: related,
		OpenWorld:      true,
		OwnerPackage:   "groupmarkdownuploads",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:        individualTool,
			Title:       toolutil.TitleFromName(individualTool),
			Description: description,
		},
	}
}
