// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package groupmarkdownuploads

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/progress"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Upload.

// opGroupUpload is the operation an upload's errors are named by, the
// individual tool's name as the package's other handlers name theirs.
const opGroupUpload = "gitlab_group_markdown_upload"

// UploadInput represents input for uploading a file to a group's Markdown
// attachments store. Exactly one of FilePath or ContentBase64 carries the
// content.
type UploadInput struct {
	GroupID       toolutil.StringOrInt `json:"group_id" jsonschema:"The ID or URL-encoded path of the group,required"`
	Filename      string               `json:"filename" jsonschema:"Name of the file to upload (e.g. diagram.png),required"`
	FilePath      string               `json:"file_path,omitempty" jsonschema:"Absolute path to a local file on the MCP server filesystem. Alternative to content_base64 for files too large to base64-encode. Only one of file_path or content_base64 should be provided."`
	ContentBase64 string               `json:"content_base64,omitempty" jsonschema:"Base64-encoded file content. Only one of file_path or content_base64 should be provided."`
}

// UploadOutput contains the result of a group upload: the keys
// API::Entities::GroupUpload sends (id, alt, url, full_path, markdown, at
// v19.5.0-ee) and the full address built from the full path. They are a
// project upload's, key for key, so the type is [uploads.UploadOutput]'s
// field for field and is rendered by its card ([UploadToolResult]).
type UploadOutput struct {
	toolutil.HintableOutput
	ID       int64  `json:"id,omitempty"`
	Alt      string `json:"alt"`
	URL      string `json:"url"`
	FullPath string `json:"full_path"`
	Markdown string `json:"markdown"`
	FullURL  string `json:"full_url,omitempty"`
}

// apiVersionPath is the segment client-go appends to the configured instance
// URL, which a web address does not start from.
const apiVersionPath = "api/v4/"

// instanceRootURL returns the address the instance serves its web interface
// at: the SDK's base URL without the API version path or a trailing slash. A
// group upload's full_path ("/-/group/<id>/uploads/<secret>/<name>") is a path
// from that root, as a project upload's is, which is why the uploads package
// builds its full address the same way.
func instanceRootURL(client *gitlabclient.Client) string {
	base := client.GL().BaseURL()
	base.Path = strings.TrimSuffix(base.Path, apiVersionPath)
	return strings.TrimRight(base.String(), "/")
}

// Upload uploads a file to a group's Markdown attachments store through
// POST /groups/:id/uploads, which GitLab serves from 19.0 and client-go
// models from v3.42.0, and answers with what GitLab sends and the full
// address of the file.
func Upload(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input UploadInput) (UploadOutput, error) {
	if err := ctx.Err(); err != nil {
		return UploadOutput{}, err
	}
	if string(input.GroupID) == "" {
		return UploadOutput{}, toolutil.ErrRequiredString(opGroupUpload, "group_id")
	}
	if input.Filename == "" {
		return UploadOutput{}, toolutil.ErrRequiredString(opGroupUpload, "filename")
	}
	reader, err := toolutil.ReadFileOrBase64(opGroupUpload, input.FilePath, input.ContentBase64)
	if err != nil {
		return UploadOutput{}, err
	}
	var body io.Reader = reader
	if tracker := progress.FromRequest(req); tracker.IsActive() {
		body = toolutil.NewProgressReader(ctx, reader, int64(reader.Len()), tracker)
	}
	uploaded, _, err := client.GL().GroupMarkdownUploads.UploadGroupMarkdown(string(input.GroupID), body, input.Filename, gl.WithContext(ctx))
	if err != nil {
		return UploadOutput{}, toolutil.WrapErrWithStatusHint(opGroupUpload, err, http.StatusNotFound,
			"verify group_id with group.get; GitLab serves this route from 19.0, and an older instance answers 404")
	}
	return UploadOutput{
		ID:       uploaded.ID,
		Alt:      uploaded.Alt,
		URL:      uploaded.URL,
		FullPath: uploaded.FullPath,
		Markdown: uploaded.Markdown,
		FullURL:  instanceRootURL(client) + uploaded.FullPath,
	}, nil
}

// List.

// ListInput represents input for listing group markdown uploads. It mirrors
// gl.ListMarkdownUploadsOptions (the embedded gl.ListOptions); GitLab pages
// the group uploads list by offset alone.
type ListInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"The ID or URL-encoded path of the group,required"`
	OrderBy string               `json:"order_by,omitempty" jsonschema:"Column to order results by"`
	Sort    string               `json:"sort,omitempty"     jsonschema:"Sort order: 'asc' or 'desc'"`
	toolutil.PaginationInput
}

// UploadedByOutput mirrors the short-user representation in a markdown
// upload's uploaded_by field (gl.MarkdownUpload.UploadedBy, a *gl.User).
//
// Documented reference subset per doc/api/group_markdown_uploads.md: the
// "List all uploads for a group" response documents only id, name, and
// username inside uploaded_by. The SDK *gl.User embed additionally carries
// state, avatar_url, and web_url; those are trimmed here to keep the nested
// reference at documented depth.
type UploadedByOutput struct {
	ID       int64  `json:"id"`       // Documented reference subset per doc/api/group_markdown_uploads.md
	Username string `json:"username"` // Documented reference subset per doc/api/group_markdown_uploads.md
	Name     string `json:"name"`     // Documented reference subset per doc/api/group_markdown_uploads.md
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
		ID:       u.ID,
		Username: u.Username,
		Name:     u.Name,
	}
}

// List retrieves group markdown uploads.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (*ListOutput, error) {
	if string(input.GroupID) == "" {
		return nil, toolutil.ErrRequiredString("gitlab_list_group_markdown_uploads", "group_id")
	}
	opts := &gl.ListMarkdownUploadsOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, toolutil.KeysetPaginationInput{})
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
	}
	uploads, resp, err := client.GL().GroupMarkdownUploads.ListGroupMarkdownUploads(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return nil, toolutil.WrapErrWithStatusHint("gitlab_list_group_markdown_uploads", err, http.StatusNotFound,
			"verify group_id with group.get; listing markdown uploads requires Maintainer role or higher")
	}
	items := make([]UploadItem, 0, len(uploads))
	for _, u := range uploads {
		item := UploadItem{
			ID:         u.ID,
			Size:       u.Size,
			Filename:   u.Filename,
			UploadedBy: uploadedByOutput(u.UploadedBy),
		}
		// RFC 3339, the form every other output in this tree carries and the
		// one toolutil.FormatTime reads; time.Time.String() rendered
		// "2001-02-03 04:05:06 +0000 UTC", which no display helper parses.
		item.CreatedAt = toolutil.RFC3339Ptr(u.CreatedAt)
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
	if string(input.GroupID) == "" {
		return toolutil.ErrRequiredString("gitlab_delete_group_markdown_upload_by_id", "group_id")
	}
	if input.UploadID <= 0 {
		return toolutil.ErrRequiredInt64("gitlab_delete_group_markdown_upload_by_id", "upload_id")
	}
	_, err := client.GL().GroupMarkdownUploads.DeleteGroupMarkdownUploadByID(string(input.GroupID), input.UploadID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("gitlab_delete_group_markdown_upload_by_id", err, http.StatusNotFound,
			"verify upload_id with group.group_upload_list; deleting requires Maintainer role or higher")
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
	if string(input.GroupID) == "" {
		return toolutil.ErrRequiredString("gitlab_delete_group_markdown_upload_by_secret", "group_id")
	}
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
//
// The rendering of the upload list lives in markdown.go, which is the
// formatter the registry serves. The second copy that used to sit here was
// registered for no type at all: the handler answers with a pointer and the
// registration named the value, so nothing ever called it, and it wrote its
// table with no heading and no pagination.

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
