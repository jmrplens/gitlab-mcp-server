package uploads

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionUpload       = "upload"
	actionUploadList   = "upload_list"
	actionUploadDelete = "upload_delete"
)

// ActionSpecs returns canonical specs for project upload actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_project_upload — upload a file (path or base64) to a project's markdown upload area.
		uploadCreateSpec(actionUpload, toolutil.RouteActionWithRequest(client, Upload), "gitlab_project_upload",
			"Use to upload a file into a project's Markdown attachments store and obtain a Markdown-embeddable reference.",
			"Upload a file (via local file_path or base64 content) to a project's Markdown attachments store. Returns: the upload's alt text, relative and full URLs, full path, and a Markdown embed reference. See also: gitlab_project_upload_list, gitlab_project_upload_delete, gitlab_project_get.",
			[]string{actionUploadList, "project.get"}),
		// gitlab_project_upload_list — list existing markdown uploads for a project.
		uploadReadSpec(actionUploadList, toolutil.RouteAction(client, List), "gitlab_project_upload_list",
			"Use to enumerate files uploaded into a project's Markdown attachments store.",
			"List all Markdown uploads in a project. Returns: each upload's id, filename, size, creation time, and uploader (id, username, name, state, avatar, web URL), plus pagination metadata. See also: gitlab_project_upload, gitlab_project_upload_delete, gitlab_project_get.",
			[]string{actionUpload, actionUploadDelete, "project.get"}),
		// gitlab_project_upload_delete — delete a markdown upload by ID (destructive).
		uploadDeleteSpec(actionUploadDelete, toolutil.DestructiveAction(client, deleteOutput), "gitlab_project_upload_delete",
			"Use to permanently remove a project Markdown upload identified by its numeric upload ID.",
			"Delete a project Markdown upload by its numeric upload ID. Returns: a success confirmation. See also: gitlab_project_upload_list, gitlab_project_upload, gitlab_project_get.",
			[]string{actionUploadList, "project.get"}),
	}
}

// deleteOutput adapts the void [Delete] handler into the catalog DeleteOutput contract
// so it composes with [toolutil.DestructiveAction] and surfaces a confirmation message.
func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult(fmt.Sprintf("upload %d from project %s", input.UploadID, input.ProjectID))
	return out, nil
}

// uploadReadSpec builds the canonical read-only spec for an upload tool.
func uploadReadSpec(name string, route toolutil.ActionRoute, individualTool, usage, description string, related []string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, uploadOptions(individualTool, usage, description, related))
}

// uploadCreateSpec builds the canonical create spec for an upload tool.
func uploadCreateSpec(name string, route toolutil.ActionRoute, individualTool, usage, description string, related []string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, uploadOptions(individualTool, usage, description, related))
}

// uploadDeleteSpec builds the canonical destructive delete spec for an upload tool.
func uploadDeleteSpec(name string, route toolutil.ActionRoute, individualTool, usage, description string, related []string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, uploadOptions(individualTool, usage, description, related))
}

func uploadOptions(individualTool, usage, description string, related []string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: usage, Tags: []string{"project", "upload"},
		RelatedActions: related,
		OpenWorld:      true,
		OwnerPackage:   "uploads",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:        individualTool,
			Title:       toolutil.TitleFromName(individualTool),
			Description: description,
		},
	}
}
