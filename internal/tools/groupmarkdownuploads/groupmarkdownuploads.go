package groupmarkdownuploads

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// List.

// ListInput represents input for listing group markdown uploads. It mirrors
// gl.ListMarkdownUploadsOptions (the embedded gl.ListOptions) including offset
// and keyset pagination via order_by/sort/page_token.
type ListInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"The ID or URL-encoded path of the group,required"`
	OrderBy string               `json:"order_by,omitempty" jsonschema:"For keyset pagination, the column to order results by"`
	Sort    string               `json:"sort,omitempty"     jsonschema:"Sort order for keyset pagination: 'asc' or 'desc'"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// UploadedByOutput mirrors the short-user representation in a markdown
// upload's uploaded_by field (gl.MarkdownUpload.UploadedBy, a *gl.User).
type UploadedByOutput struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	State     string `json:"state"`
	AvatarURL string `json:"avatar_url"`
	WebURL    string `json:"web_url"`
}

// UploadItem represents a single markdown upload entry. It mirrors the canonical
// keys of gl.MarkdownUpload (id, size, filename, created_at, uploaded_by).
type UploadItem struct {
	ID         int64             `json:"id"`
	Size       int64             `json:"size"`
	Filename   string            `json:"filename"`
	CreatedAt  string            `json:"created_at,omitempty"`
	UploadedBy *UploadedByOutput `json:"uploaded_by,omitempty"`
}

// ListOutput represents the output of listing group markdown uploads.
type ListOutput struct {
	Uploads    []UploadItem              `json:"uploads"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// uploadedByOutput maps an SDK *gl.User embed onto the short-user output shape.
func uploadedByOutput(u *gl.User) *UploadedByOutput {
	if u == nil {
		return nil
	}
	return &UploadedByOutput{
		ID:        u.ID,
		Username:  u.Username,
		Name:      u.Name,
		State:     u.State,
		AvatarURL: u.AvatarURL,
		WebURL:    u.WebURL,
	}
}

// List retrieves group markdown uploads.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (*ListOutput, error) {
	opts := &gl.ListMarkdownUploadsOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
	}
	uploads, resp, err := client.GL().GroupMarkdownUploads.ListGroupMarkdownUploads(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return nil, toolutil.WrapErrWithStatusHint("gitlab_list_group_markdown_uploads", err, http.StatusNotFound,
			"verify group_id with gitlab_group_get; listing markdown uploads requires Maintainer role or higher")
	}
	items := make([]UploadItem, 0, len(uploads))
	for _, u := range uploads {
		item := UploadItem{
			ID:         u.ID,
			Size:       u.Size,
			Filename:   u.Filename,
			UploadedBy: uploadedByOutput(u.UploadedBy),
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
		return toolutil.WrapErrWithStatusHint("gitlab_delete_group_markdown_upload_by_id", err, http.StatusNotFound,
			"verify upload_id with gitlab_list_group_markdown_uploads; deleting requires Maintainer role or higher")
	}
	return nil
}

func deleteByIDOutput(ctx context.Context, client *gitlabclient.Client, input DeleteByIDInput) (toolutil.DeleteOutput, error) {
	if err := DeleteByID(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted group markdown upload."}, nil
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
		return toolutil.WrapErrWithStatusHint("gitlab_delete_group_markdown_upload_by_secret", err, http.StatusNotFound,
			"verify secret (32-char hex) and filename match an existing upload; deleting requires Maintainer role or higher")
	}
	return nil
}

func deleteBySecretAndFilenameOutput(ctx context.Context, client *gitlabclient.Client, input DeleteBySecretAndFilenameInput) (toolutil.DeleteOutput, error) {
	if err := DeleteBySecretAndFilename(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted group markdown upload."}, nil
}

// Markdown Formatters.

// FormatList formats the list of group markdown uploads as markdown.
func FormatList(out *ListOutput) string {
	if len(out.Uploads) == 0 {
		return "No group markdown uploads found.\n"
	}
	var sb strings.Builder
	sb.WriteString("| ID | Filename | Size | Created At | Uploaded By |\n")
	sb.WriteString("|---|---|---|---|---|\n")
	for _, u := range out.Uploads {
		fmt.Fprintf(&sb, "| %d | %s | %d | %s | %s |\n",
			u.ID,
			toolutil.EscapeMdTableCell(u.Filename),
			u.Size,
			toolutil.EscapeMdTableCell(u.CreatedAt),
			toolutil.EscapeMdTableCell(uploadedByLabel(u.UploadedBy)))
	}
	toolutil.WritePagination(&sb, out.Pagination)
	toolutil.WriteHints(&sb, "Use upload URLs in Markdown content to embed files")
	return sb.String()
}

// uploadedByLabel renders a markdown upload's uploader as "name (@username)",
// falling back gracefully when fields are empty or the user is absent.
func uploadedByLabel(u *UploadedByOutput) string {
	if u == nil {
		return ""
	}
	switch {
	case u.Name != "" && u.Username != "":
		return fmt.Sprintf("%s (@%s)", u.Name, u.Username)
	case u.Username != "":
		return "@" + u.Username
	default:
		return u.Name
	}
}
