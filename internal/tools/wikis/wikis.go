// Package wikis implements MCP tool handlers for GitLab project wiki operations
// including list, get, create, update, and delete pages.
// It wraps the Wikis service from client-go v2.
package wikis

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ListInput defines parameters for listing wiki pages in a GitLab project.
type ListInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id"              jsonschema:"Project ID or URL-encoded path,required"`
	WithContent bool                 `json:"with_content,omitempty"  jsonschema:"Include page content in the response"`
}

// GetInput defines parameters for retrieving a single wiki page.
type GetInput struct {
	ProjectID  toolutil.StringOrInt `json:"project_id"             jsonschema:"Project ID or URL-encoded path,required"`
	Slug       string               `json:"slug"                   jsonschema:"URL-encoded slug of the wiki page (e.g. 'my-page'),required"`
	RenderHTML bool                 `json:"render_html,omitempty"  jsonschema:"Return HTML-rendered content instead of raw format"`
	Version    string               `json:"version,omitempty"      jsonschema:"Wiki page version SHA to retrieve a specific revision"`
}

// CreateInput defines parameters for creating a new wiki page.
type CreateInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Title     string               `json:"title"             jsonschema:"Title of the wiki page,required"`
	Content   string               `json:"content"           jsonschema:"Content of the wiki page (Markdown, RDoc, AsciiDoc, or Org),required"`
	Format    string               `json:"format,omitempty"  jsonschema:"Content format: markdown (default), rdoc, asciidoc, or org"`
}

// UpdateInput defines parameters for updating an existing wiki page.
type UpdateInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Slug      string               `json:"slug"              jsonschema:"URL-encoded slug of the wiki page to update,required"`
	Title     string               `json:"title,omitempty"   jsonschema:"New title for the wiki page"`
	Content   string               `json:"content,omitempty" jsonschema:"New content for the wiki page"`
	Format    string               `json:"format,omitempty"  jsonschema:"Content format: markdown, rdoc, asciidoc, or org"`
}

// DeleteInput defines parameters for deleting a wiki page.
type DeleteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Slug      string               `json:"slug"       jsonschema:"URL-encoded slug of the wiki page to delete,required"`
}

// Output represents a single wiki page.
type Output struct {
	toolutil.HintableOutput
	Title    string `json:"title"`
	Slug     string `json:"slug"`
	Format   string `json:"format"`
	Content  string `json:"content,omitempty"`
	Encoding string `json:"encoding,omitempty"`
}

// ListOutput holds a list of wiki pages.
type ListOutput struct {
	toolutil.HintableOutput
	WikiPages []Output `json:"wiki_pages"`
}

// wikiToOutput converts a GitLab API [gl.Wiki] to MCP output format.
func toOutput(w *gl.Wiki) Output {
	return Output{
		Title:    w.Title,
		Slug:     w.Slug,
		Format:   string(w.Format),
		Content:  w.Content,
		Encoding: w.Encoding,
	}
}

// List retrieves all wiki pages for a GitLab project.
// Optionally includes page content when with_content is true.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("wikiList: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}

	opts := &gl.ListWikisOptions{}
	if input.WithContent {
		opts.WithContent = new(true)
	}

	wikiPages, _, err := client.GL().Wikis.ListWikis(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("wikiList", err, http.StatusNotFound,
			"verify project_id with gitlab_project_get; the project's wiki feature may be disabled")
	}

	out := make([]Output, len(wikiPages))
	for i, p := range wikiPages {
		out[i] = toOutput(p)
	}
	return ListOutput{WikiPages: out}, nil
}

// Get retrieves a single wiki page by slug from a GitLab project.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("wikiGet: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.Slug == "" {
		return Output{}, errors.New("wikiGet: slug is required. Use gitlab_wiki_list to find available pages first")
	}

	opts := &gl.GetWikiPageOptions{}
	if input.RenderHTML {
		opts.RenderHTML = new(true)
	}
	if input.Version != "" {
		opts.Version = new(input.Version)
	}

	w, _, err := client.GL().Wikis.GetWikiPage(string(input.ProjectID), input.Slug, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("wikiGet", err, http.StatusNotFound,
			"verify slug with gitlab_wiki_list; slugs are case-sensitive and use hyphens for spaces")
	}
	return toOutput(w), nil
}

// Create creates a new wiki page in the specified GitLab project.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("wikiCreate: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.Title == "" {
		return Output{}, errors.New("wikiCreate: title is required")
	}
	if input.Content == "" {
		return Output{}, errors.New("wikiCreate: content is required")
	}

	opts := &gl.CreateWikiPageOptions{
		Title:   new(input.Title),
		Content: new(toolutil.NormalizeText(input.Content)),
	}
	if input.Format != "" {
		opts.Format = new(gl.WikiFormatValue(input.Format))
	}

	w, _, err := client.GL().Wikis.CreateWikiPage(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return Output{}, toolutil.WrapErrWithHint("wikiCreate", err, "a wiki page with this slug may already exist, or the content format is invalid")
		}
		return Output{}, toolutil.WrapErrWithMessage("wikiCreate", err)
	}
	return toOutput(w), nil
}

// Update updates an existing wiki page identified by slug.
// At least one of title, content, or format must be provided.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("wikiUpdate: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.Slug == "" {
		return Output{}, errors.New("wikiUpdate: slug is required. Use gitlab_wiki_list to find available pages first")
	}

	opts := &gl.EditWikiPageOptions{}
	if input.Title != "" {
		opts.Title = new(input.Title)
	}
	if input.Content != "" {
		opts.Content = new(toolutil.NormalizeText(input.Content))
	}
	if input.Format != "" {
		opts.Format = new(gl.WikiFormatValue(input.Format))
	}

	w, _, err := client.GL().Wikis.EditWikiPage(string(input.ProjectID), input.Slug, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("wikiUpdate", err, http.StatusNotFound,
			"verify slug with gitlab_wiki_list; slugs are case-sensitive")
	}
	return toOutput(w), nil
}

// Delete deletes a wiki page identified by slug from a GitLab project.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("wikiDelete: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.Slug == "" {
		return errors.New("wikiDelete: slug is required. Use gitlab_wiki_list to find available pages first")
	}

	_, err := client.GL().Wikis.DeleteWikiPage(string(input.ProjectID), input.Slug, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("wikiDelete", err, http.StatusForbidden,
			"deleting wiki pages requires Maintainer or Owner role")
	}
	return nil
}

// Markdown formatting.

// Upload Attachment.

// UploadAttachmentInput defines parameters for uploading an attachment to a wiki.
type UploadAttachmentInput struct {
	ProjectID     toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Filename      string               `json:"filename" jsonschema:"Name of the file to upload (e.g. diagram.png),required"`
	ContentBase64 string               `json:"content_base64,omitempty" jsonschema:"Base64-encoded file content. Provide either content_base64 or file_path"`
	FilePath      string               `json:"file_path,omitempty" jsonschema:"Absolute path to a local file. Provide either file_path or content_base64"`
	Branch        string               `json:"branch,omitempty" jsonschema:"Branch to upload the attachment to"`
}

// AttachmentOutput represents the result of a wiki attachment upload.
type AttachmentOutput struct {
	toolutil.HintableOutput
	FileName string `json:"file_name"`
	FilePath string `json:"file_path"`
	Branch   string `json:"branch"`
	URL      string `json:"url"`
	Markdown string `json:"markdown"`
}

// resolveAttachmentReader builds a bytes.Reader from either a file path or base64 content.
func resolveAttachmentReader(filePath, contentBase64 string) (*bytes.Reader, error) {
	if filePath != "" {
		cfg := toolutil.GetUploadConfig()
		f, info, err := toolutil.OpenAndValidateFile(filePath, cfg.MaxFileSize)
		if err != nil {
			return nil, fmt.Errorf("upload_wiki_attachment: %w", err)
		}
		defer f.Close()

		data := make([]byte, info.Size())
		if _, err = io.ReadFull(f, data); err != nil {
			return nil, fmt.Errorf("upload_wiki_attachment: reading file: %w", err)
		}
		return bytes.NewReader(data), nil
	}

	decoded, err := base64.StdEncoding.DecodeString(contentBase64)
	if err != nil {
		return nil, fmt.Errorf("upload_wiki_attachment: invalid base64 content: %w", err)
	}
	return bytes.NewReader(decoded), nil
}

// UploadAttachment uploads a file attachment to a project wiki.
func UploadAttachment(ctx context.Context, client *gitlabclient.Client, input UploadAttachmentInput) (AttachmentOutput, error) {
	if input.ProjectID == "" {
		return AttachmentOutput{}, errors.New("upload_wiki_attachment: project_id is required")
	}
	if input.Filename == "" {
		return AttachmentOutput{}, errors.New("upload_wiki_attachment: filename is required")
	}

	hasFilePath := input.FilePath != ""
	hasBase64 := input.ContentBase64 != ""

	if hasFilePath && hasBase64 {
		return AttachmentOutput{}, errors.New("upload_wiki_attachment: provide either file_path or content_base64, not both")
	}
	if !hasFilePath && !hasBase64 {
		return AttachmentOutput{}, errors.New("upload_wiki_attachment: either file_path or content_base64 is required")
	}

	reader, err := resolveAttachmentReader(input.FilePath, input.ContentBase64)
	if err != nil {
		return AttachmentOutput{}, err
	}

	opts := &gl.UploadWikiAttachmentOptions{}
	if input.Branch != "" {
		opts.Branch = new(input.Branch)
	}

	attachment, _, err := client.GL().Wikis.UploadWikiAttachment(
		string(input.ProjectID), reader, input.Filename, opts, gl.WithContext(ctx),
	)
	if err != nil {
		return AttachmentOutput{}, toolutil.WrapErrWithStatusHint("upload_wiki_attachment", err, http.StatusBadRequest,
			"filename must be non-empty and content must be valid binary; uploading wiki attachments requires Developer role or higher")
	}

	return AttachmentOutput{
		FileName: attachment.FileName,
		FilePath: attachment.FilePath,
		Branch:   attachment.Branch,
		URL:      attachment.Link.URL,
		Markdown: attachment.Link.Markdown,
	}, nil
}
