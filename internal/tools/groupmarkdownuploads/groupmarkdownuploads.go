// Package groupmarkdownuploads implements MCP tools for GitLab group markdown upload operations.
package groupmarkdownuploads

import (
	"context"
	"fmt"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// List.

// ListInput represents input for listing group markdown uploads.
type ListInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"The ID or URL-encoded path of the group,required"`
	Page    int64                `json:"page,omitempty" jsonschema:"Page number for pagination"`
	PerPage int64                `json:"per_page,omitempty" jsonschema:"Number of items per page"`
}

// UploadItem represents a single markdown upload entry.
type UploadItem struct {
	ID        int64  `json:"id"`
	Size      int64  `json:"size"`
	Filename  string `json:"filename"`
	CreatedAt string `json:"created_at,omitempty"`
}

// ListOutput represents the output of listing group markdown uploads.
type ListOutput struct {
	Uploads    []UploadItem              `json:"uploads"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// List retrieves group markdown uploads.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (*ListOutput, error) {
	opts := &gl.ListMarkdownUploadsOptions{
		ListOptions: gl.ListOptions{
			Page:    input.Page,
			PerPage: input.PerPage,
		},
	}
	uploads, resp, err := client.GL().GroupMarkdownUploads.ListGroupMarkdownUploads(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return nil, toolutil.WrapErrWithMessage("gitlab_list_group_markdown_uploads", err)
	}
	items := make([]UploadItem, 0, len(uploads))
	for _, u := range uploads {
		item := UploadItem{
			ID:       u.ID,
			Size:     u.Size,
			Filename: u.Filename,
		}
		if u.CreatedAt != nil {
			item.CreatedAt = u.CreatedAt.String()
		}
		items = append(items, item)
	}
	pag := toolutil.PaginationFromResponse(resp)
	return &ListOutput{
		Uploads:    items,
		Pagination: pag,
	}, nil
}

// Delete by ID.

// DeleteByIDInput represents input for deleting a group markdown upload by ID.
type DeleteByIDInput struct {
	GroupID  toolutil.StringOrInt `json:"group_id" jsonschema:"The ID or URL-encoded path of the group,required"`
	UploadID int64                `json:"upload_id" jsonschema:"The ID of the upload to delete,required"`
}

// DeleteByID deletes a group markdown upload by its ID.
func DeleteByID(ctx context.Context, client *gitlabclient.Client, input DeleteByIDInput) error {
	if input.UploadID <= 0 {
		return toolutil.ErrRequiredInt64("gitlab_delete_group_markdown_upload_by_id", "upload_id")
	}
	_, err := client.GL().GroupMarkdownUploads.DeleteGroupMarkdownUploadByID(string(input.GroupID), input.UploadID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("gitlab_delete_group_markdown_upload_by_id", err)
	}
	return nil
}

// Delete by Secret and Filename.

// DeleteBySecretAndFilenameInput represents input for deleting a group markdown upload by secret and filename.
type DeleteBySecretAndFilenameInput struct {
	GroupID  toolutil.StringOrInt `json:"group_id" jsonschema:"The ID or URL-encoded path of the group,required"`
	Secret   string               `json:"secret" jsonschema:"The secret of the upload,required"`
	Filename string               `json:"filename" jsonschema:"The filename of the upload,required"`
}

// DeleteBySecretAndFilename deletes a group markdown upload by secret and filename.
func DeleteBySecretAndFilename(ctx context.Context, client *gitlabclient.Client, input DeleteBySecretAndFilenameInput) error {
	_, err := client.GL().GroupMarkdownUploads.DeleteGroupMarkdownUploadBySecretAndFilename(string(input.GroupID), input.Secret, input.Filename, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("gitlab_delete_group_markdown_upload_by_secret", err)
	}
	return nil
}

// Markdown Formatters.

// FormatList formats the list of group markdown uploads as markdown.
func FormatList(out *ListOutput) string {
	if len(out.Uploads) == 0 {
		return "No group markdown uploads found.\n"
	}
	var sb strings.Builder
	sb.WriteString("| ID | Filename | Size | Created At |\n")
	sb.WriteString("|---|---|---|---|\n")
	for _, u := range out.Uploads {
		fmt.Fprintf(&sb, "| %d | %s | %d | %s |\n",
			u.ID,
			toolutil.EscapeMdTableCell(u.Filename),
			u.Size,
			toolutil.EscapeMdTableCell(u.CreatedAt))
	}
	toolutil.WritePagination(&sb, out.Pagination)
	toolutil.WriteHints(&sb, "Use upload URLs in Markdown content to embed files")
	return sb.String()
}
