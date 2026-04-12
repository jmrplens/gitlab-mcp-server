// Package uploads implements GitLab project file upload operations. It supports
// two input modes: base64-encoded content (for small files via JSON) and
// file_path (for larger files read directly from the local filesystem).
// Also provides list and delete operations for project markdown uploads.
package uploads

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/progress"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

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
		return UploadOutput{}, fmt.Errorf("context canceled: %w", err)
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

// ListInput defines input for listing a project's markdown uploads.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// ListItem represents a single markdown upload entry.
type ListItem struct {
	ID         int64  `json:"id"`
	Size       int64  `json:"size"`
	Filename   string `json:"filename"`
	CreatedAt  string `json:"created_at,omitempty"`
	UploadedBy string `json:"uploaded_by,omitempty"`
}

// ListOutput contains the list of markdown uploads for a project.
type ListOutput struct {
	toolutil.HintableOutput
	Uploads []ListItem `json:"uploads"`
}

// List lists all markdown uploads for a GitLab project.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, fmt.Errorf("context canceled: %w", err)
	}
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("projectUploadList: project_id is required")
	}

	uploads, _, err := client.GL().ProjectMarkdownUploads.ListProjectMarkdownUploads(string(input.ProjectID))
	if err != nil {
		return ListOutput{}, fmt.Errorf("list uploads for project %s: %w", input.ProjectID, err)
	}

	items := make([]ListItem, 0, len(uploads))
	for _, u := range uploads {
		item := ListItem{
			ID:       u.ID,
			Size:     u.Size,
			Filename: u.Filename,
		}
		if u.CreatedAt != nil {
			item.CreatedAt = u.CreatedAt.String()
		}
		if u.UploadedBy != nil {
			item.UploadedBy = u.UploadedBy.Username
		}
		items = append(items, item)
	}

	return ListOutput{Uploads: items}, nil
}

// DeleteInput defines input for deleting a project markdown upload.
type DeleteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	UploadID  int64                `json:"upload_id" jsonschema:"ID of the upload to delete,required"`
}

// Delete deletes a markdown upload from a GitLab project.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context canceled: %w", err)
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
