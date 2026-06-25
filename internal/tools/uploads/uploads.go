package uploads

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/progress"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const fmtContextCanceled = "context canceled: %w"

// UploadInput defines input for uploading a file to a GitLab project.
// Exactly one of FilePath or ContentBase64 must be provided.
type UploadInput struct {
	ProjectID     toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Filename      string               `json:"filename" jsonschema:"Name of the file to upload (e.g. screenshot.png),required"`
	FilePath      string               `json:"file_path,omitempty" jsonschema:"Absolute path to a local file on the MCP server filesystem. Alternative to content_base64 for files too large to base64-encode. Only one of file_path or content_base64 should be provided."`
	ContentBase64 string               `json:"content_base64,omitempty" jsonschema:"Base64-encoded file content. Only one of file_path or content_base64 should be provided."`
}

// UploadOutput contains the result of a file upload.
type UploadOutput struct {
	toolutil.HintableOutput
	Alt      string `json:"alt"`
	URL      string `json:"url"`
	FullPath string `json:"full_path"`
	Markdown string `json:"markdown"`
	FullURL  string `json:"full_url,omitempty"`
}

// Upload uploads a file to a GitLab project's markdown uploads area.
// Accepts either file_path (local file) or content_base64 (base64-encoded
// string). Returns the upload metadata including the Markdown-embeddable
// reference or an error if validation, decoding, or upload fails.
func Upload(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input UploadInput) (UploadOutput, error) {
	if err := ctx.Err(); err != nil {
		return UploadOutput{}, fmt.Errorf(fmtContextCanceled, err)
	}
	if input.ProjectID == "" {
		return UploadOutput{}, errors.New("projectUpload: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}

	hasFilePath := input.FilePath != ""
	hasBase64 := input.ContentBase64 != ""

	if hasFilePath && hasBase64 {
		return UploadOutput{}, errors.New("projectUpload: provide either file_path or content_base64, not both")
	}
	if !hasFilePath && !hasBase64 {
		return UploadOutput{}, errors.New("projectUpload: either file_path or content_base64 is required")
	}

	var reader *bytes.Reader

	if hasFilePath {
		cfg := toolutil.GetUploadConfig()
		f, info, err := toolutil.OpenAndValidateFile(input.FilePath, cfg.MaxFileSize)
		if err != nil {
			return UploadOutput{}, fmt.Errorf("projectUpload: %w", err)
		}
		defer f.Close()

		data := make([]byte, info.Size())
		if _, err = io.ReadFull(f, data); err != nil {
			return UploadOutput{}, fmt.Errorf("projectUpload: reading file: %w", err)
		}
		reader = bytes.NewReader(data)
	} else {
		decoded, err := base64.StdEncoding.DecodeString(input.ContentBase64)
		if err != nil {
			return UploadOutput{}, fmt.Errorf("invalid base64 content: %w", err)
		}
		reader = bytes.NewReader(decoded)
	}

	tracker := progress.FromRequest(req)
	var uploadReader interface {
		Read([]byte) (int, error)
	}
	if tracker.IsActive() {
		uploadReader = toolutil.NewProgressReader(ctx, reader, int64(reader.Len()), tracker)
	} else {
		uploadReader = reader
	}

	uploaded, _, err := client.GL().ProjectMarkdownUploads.UploadProjectMarkdown(
		string(input.ProjectID),
		uploadReader,
		input.Filename,
	)
	if err != nil {
		return UploadOutput{}, fmt.Errorf("upload file to project %s: %w", input.ProjectID, err)
	}

	fullURL := strings.TrimRight(client.GL().BaseURL().String(), "/") + uploaded.FullPath

	return UploadOutput{
		Alt:      uploaded.Alt,
		URL:      uploaded.URL,
		FullPath: uploaded.FullPath,
		Markdown: uploaded.Markdown,
		FullURL:  fullURL,
	}, nil
}

// Markdown Upload List/Delete.

// ListInput defines input for listing a project's markdown uploads. It mirrors
// the offset and keyset pagination supported by the GitLab uploads list endpoint
// (page/per_page plus order_by/sort/page_token for keyset traversal).
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"For keyset pagination, the column to order results by"`
	Sort      string               `json:"sort,omitempty"     jsonschema:"Sort order for keyset pagination: 'asc' or 'desc'"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// UploadedByOutput mirrors the short-user representation in a markdown upload's
// uploaded_by field (gl.MarkdownUpload.UploadedBy, a *gl.User). It is the
// project-side mirror of groupmarkdownuploads.UploadedByOutput.
type UploadedByOutput struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	State     string `json:"state"`
	AvatarURL string `json:"avatar_url"`
	WebURL    string `json:"web_url"`
}

// ListItem represents a single markdown upload entry. It mirrors the canonical
// keys of gl.MarkdownUpload (id, size, filename, created_at, uploaded_by).
type ListItem struct {
	ID         int64             `json:"id"`
	Size       int64             `json:"size"`
	Filename   string            `json:"filename"`
	CreatedAt  string            `json:"created_at,omitempty"`
	UploadedBy *UploadedByOutput `json:"uploaded_by,omitempty"`
}

// ListOutput contains the list of markdown uploads for a project.
type ListOutput struct {
	toolutil.HintableOutput
	Uploads    []ListItem                `json:"uploads"`
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

// listQueryParameters renders the offset, keyset, and ordering parameters as a
// gl.RequestOptionFunc. The ListProjectMarkdownUploads SDK method has no options
// struct, so pagination is applied via query parameters mirrored from a
// gl.ListMarkdownUploadsOptions wired with toolutil.ApplyListOptions.
func listQueryParameters(input ListInput) gl.RequestOptionFunc {
	opts := &gl.ListMarkdownUploadsOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)

	q := url.Values{}
	if opts.Page > 0 {
		q.Set("page", strconv.FormatInt(opts.Page, 10))
	}
	if opts.PerPage > 0 {
		q.Set("per_page", strconv.FormatInt(opts.PerPage, 10))
	}
	if opts.Pagination != "" {
		q.Set("pagination", opts.Pagination)
	}
	if opts.PageToken != "" {
		q.Set("page_token", opts.PageToken)
	}
	if input.OrderBy != "" {
		q.Set("order_by", input.OrderBy)
	}
	if input.Sort != "" {
		q.Set("sort", input.Sort)
	}
	return gl.WithKeysetPaginationParameters("?" + q.Encode())
}

// List lists all markdown uploads for a GitLab project.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, fmt.Errorf(fmtContextCanceled, err)
	}
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("projectUploadList: project_id is required")
	}

	uploads, resp, err := client.GL().ProjectMarkdownUploads.ListProjectMarkdownUploads(
		string(input.ProjectID),
		gl.WithContext(ctx),
		listQueryParameters(input),
	)
	if err != nil {
		return ListOutput{}, fmt.Errorf("list uploads for project %s: %w", input.ProjectID, err)
	}

	items := make([]ListItem, 0, len(uploads))
	for _, u := range uploads {
		item := ListItem{
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

	return ListOutput{
		Uploads:    items,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// DeleteInput defines input for deleting a project markdown upload.
type DeleteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	UploadID  int64                `json:"upload_id" jsonschema:"ID of the upload to delete,required"`
}

// Delete deletes a markdown upload from a GitLab project.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf(fmtContextCanceled, err)
	}
	if input.ProjectID == "" {
		return errors.New("projectUploadDelete: project_id is required")
	}
	if input.UploadID <= 0 {
		return errors.New("projectUploadDelete: upload_id is required and must be positive")
	}

	_, err := client.GL().ProjectMarkdownUploads.DeleteProjectMarkdownUploadByID(string(input.ProjectID), input.UploadID)
	if err != nil {
		return fmt.Errorf("delete upload %d from project %s: %w", input.UploadID, input.ProjectID, err)
	}

	return nil
}

// UploadToolResult builds a CallToolResult for upload operations. For image
// files it appends a Markdown image embed with the full URL so capable MCP
// clients can render the image inline. Non-image uploads return text only.
func UploadToolResult(u UploadOutput) *mcp.CallToolResult {
	md := FormatUploadMarkdown(u)
	if toolutil.IsImageFile(u.Alt) && u.FullURL != "" {
		md += fmt.Sprintf("\n![%s](%s)\n", u.Alt, u.FullURL)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: md},
		},
	}
}
