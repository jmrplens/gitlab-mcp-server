package uploads

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionUpload               = "upload"
	actionUploadList           = "upload_list"
	actionUploadDelete         = "upload_delete"
	actionUploadDeleteBySecret = "upload_delete_by_secret"
	actionProjectGet           = "project.get"
)

// ActionSpecs returns canonical specs for project upload actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_project_upload — upload a file (path or base64) to a project's markdown upload area.
		uploadCreateSpec(actionUpload, toolutil.RouteActionWithRequest(client, Upload), "gitlab_project_upload",
			"Use to upload a file into a project's Markdown attachments store and obtain a Markdown-embeddable reference.",
			"Upload a file (via local file_path or base64 content) to a project's Markdown attachments store. Returns: the upload's id (GitLab 17.3+), alt text, relative and full URLs, full path, and a Markdown embed reference. See also: gitlab_project_upload_list, gitlab_project_upload_delete, gitlab_project_get.",
			[]string{actionUploadList, actionProjectGet}),
		// gitlab_project_upload_list — list existing markdown uploads for a project.
		uploadReadSpec(actionUploadList, toolutil.RouteAction(client, List), "gitlab_project_upload_list",
			"Use to enumerate files uploaded into a project's Markdown attachments store.",
			"List all Markdown uploads in a project. Returns: each upload's id, filename, size, creation time, and uploader (id, username, name), plus pagination metadata. See also: gitlab_project_upload, gitlab_project_upload_delete, gitlab_project_get.",
			[]string{actionUpload, actionUploadDelete, actionProjectGet}),
		// gitlab_project_upload_delete — delete a markdown upload by ID (destructive).
		uploadDeleteSpec(actionUploadDelete, toolutil.DestructiveAction(client, deleteOutput), "gitlab_project_upload_delete",
			"Use to permanently remove a project Markdown upload identified by its numeric upload ID.",
			"Delete a project Markdown upload by its numeric upload ID. Returns: a success confirmation. See also: gitlab_project_upload_list, gitlab_project_upload, gitlab_project_get.",
			[]string{actionUploadList, actionProjectGet}),
		// gitlab_project_upload_delete_by_secret — delete a markdown upload by its secret and filename (destructive).
		uploadDeleteSpec(actionUploadDeleteBySecret, toolutil.DestructiveAction(client, deleteBySecretOutput), "gitlab_project_upload_delete_by_secret",
			"Use to permanently remove a project Markdown upload identified by the secret and filename from its /uploads/<secret>/<filename> URL when the numeric upload ID is unknown.",
			"Delete a project Markdown upload by its 32-character secret and filename (the /uploads/<secret>/<filename> reference). Returns: a success confirmation. See also: gitlab_project_upload_delete, gitlab_project_upload_list, gitlab_project_upload.",
			[]string{actionUploadList, actionUploadDelete, actionProjectGet}),
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

// deleteBySecretOutput adapts the void [DeleteBySecret] handler into the catalog
// DeleteOutput contract so it composes with [toolutil.DestructiveAction] and
// surfaces a confirmation message that names the secret/filename reference.
func deleteBySecretOutput(ctx context.Context, client *gitlabclient.Client, input DeleteBySecretInput) (toolutil.DeleteOutput, error) {
	if err := DeleteBySecret(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult(fmt.Sprintf("upload %s/%s from project %s", input.Secret, input.Filename, input.ProjectID))
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
	options := toolutil.ActionSpecOptions{
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
	decorateUploadMeta(&options, individualTool)
	return options
}

// decorateUploadMeta appends distinctive natural-language aliases to the project
// upload action identified by individualTool, mirroring the per-tool meta map
// pattern used by the issues domain (1:1 audit metadata). The aliases use
// project Markdown attachment phrasing kept deliberately distinct from the
// group-level gitlab_group_markdown_upload* tools to avoid discovery collisions.
func decorateUploadMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	aliases, ok := uploadActionAliases[individualTool]
	if !ok {
		return
	}
	options.Aliases = append(options.Aliases, aliases...)
}

// uploadActionAliases maps each individual project-upload tool to its
// distinctive natural-language aliases beyond the canonical tool name.
var uploadActionAliases = map[string][]string{
	"gitlab_project_upload": {
		"attach file to project markdown",
		"upload project attachment",
		"add markdown image to project",
	},
	"gitlab_project_upload_list": {
		"list project markdown attachments",
		"show project uploaded files",
		"browse project upload area",
	},
	"gitlab_project_upload_delete": {
		"delete project markdown attachment",
		"remove project uploaded file",
		"purge project upload by id",
	},
	"gitlab_project_upload_delete_by_secret": {
		"delete project markdown attachment by secret",
		"remove project uploaded file by secret and filename",
		"purge project upload by secret",
		"delete project upload from uploads url",
	},
}
