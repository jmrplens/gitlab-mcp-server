// Package snippets implements MCP tools for GitLab personal and project
// snippets via the SnippetsService and ProjectSnippetsService APIs.
package snippets

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ---------------------------------------------------------------------------
// Shared output types
// ---------------------------------------------------------------------------.

// AuthorOutput represents a snippet author.
type AuthorOutput struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	State    string `json:"state"`
}

// FileOutput represents a file attached to a snippet.
type FileOutput struct {
	Path   string `json:"path"`
	RawURL string `json:"raw_url"`
}

// Output represents a single snippet.
type Output struct {
	toolutil.HintableOutput
	ID          int64        `json:"id"`
	Title       string       `json:"title"`
	FileName    string       `json:"file_name"`
	Description string       `json:"description"`
	Visibility  string       `json:"visibility"`
	Author      AuthorOutput `json:"author"`
	ProjectID   int64        `json:"project_id,omitempty"`
	WebURL      string       `json:"web_url"`
	RawURL      string       `json:"raw_url"`
	Files       []FileOutput `json:"files,omitempty"`
	CreatedAt   *time.Time   `json:"created_at,omitempty"`
	UpdatedAt   *time.Time   `json:"updated_at,omitempty"`
}

// ListOutput represents a list of snippets with pagination.
type ListOutput struct {
	toolutil.HintableOutput
	Snippets   []Output                  `json:"snippets"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ContentOutput represents raw snippet content.
type ContentOutput struct {
	toolutil.HintableOutput
	SnippetID int64  `json:"snippet_id"`
	Content   string `json:"content"`
}

// FileContentOutput represents raw snippet file content.
type FileContentOutput struct {
	toolutil.HintableOutput
	SnippetID int64  `json:"snippet_id"`
	Ref       string `json:"ref"`
	FileName  string `json:"file_name"`
	Content   string `json:"content"`
}

// convertSnippet is an internal helper for the snippets package.
func convertSnippet(s *gl.Snippet) Output {
	out := Output{
		ID:          s.ID,
		Title:       s.Title,
		FileName:    s.FileName,
		Description: s.Description,
		Visibility:  s.Visibility,
		ProjectID:   s.ProjectID,
		WebURL:      s.WebURL,
		RawURL:      s.RawURL,
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   s.UpdatedAt,
	}
	out.Author = AuthorOutput{
		ID:       s.Author.ID,
		Username: s.Author.Username,
		Name:     s.Author.Name,
		Email:    s.Author.Email,
		State:    s.Author.State,
	}
	for _, f := range s.Files {
		out.Files = append(out.Files, FileOutput{Path: f.Path, RawURL: f.RawURL})
	}
	return out
}

// ---------------------------------------------------------------------------
// Shared input types for file operations
// ---------------------------------------------------------------------------.

// CreateFileInput represents a file to include when creating a snippet.
type CreateFileInput struct {
	FilePath string `json:"file_path" jsonschema:"File path for the snippet file,required"`
	Content  string `json:"content" jsonschema:"Content of the file,required"`
}

// UpdateFileInput represents a file operation when updating a snippet.
type UpdateFileInput struct {
	Action       string `json:"action" jsonschema:"File action: create, update, delete, move,required"`
	FilePath     string `json:"file_path" jsonschema:"File path,required"`
	Content      string `json:"content,omitempty" jsonschema:"File content (for create/update)"`
	PreviousPath string `json:"previous_path,omitempty" jsonschema:"Previous file path (for move)"`
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.

// extractProjectPath extracts the project path from a GitLab snippet web URL.
// For project snippets the URL has the form https://host/group/project/-/snippets/ID.
// Returns an empty string for personal snippets or unparseable URLs.
func extractProjectPath(webURL string) string {
	const marker = "/-/snippets/"
	u, err := url.Parse(webURL)
	if err != nil || u.Scheme == "" {
		return ""
	}
	idx := strings.Index(u.Path, marker)
	if idx <= 0 {
		return ""
	}
	return strings.TrimPrefix(u.Path[:idx], "/")
}

// snippetsHaveProject is an internal helper for the snippets package.
func snippetsHaveProject(snippets []Output) bool {
	for _, s := range snippets {
		if s.ProjectID != 0 {
			return true
		}
	}
	return false
}

// writeProjectSnippetTable is an internal helper for the snippets package.
func writeProjectSnippetTable(b *strings.Builder, snippets []Output) {
	b.WriteString("| ID | Title | Project | Visibility | Author | Files |\n")
	b.WriteString("|---|---|---|---|---|---|\n")
	for _, s := range snippets {
		proj := resolveProjectLabel(s)
		fmt.Fprintf(b, "| %d | %s | %s | %s | @%s | %d |\n",
			s.ID, toolutil.MdTitleLink(toolutil.EscapeMdTableCell(s.Title), s.WebURL), proj, s.Visibility, s.Author.Username, len(s.Files))
	}
}

// resolveProjectLabel is an internal helper for the snippets package.
func resolveProjectLabel(s Output) string {
	if s.ProjectID == 0 {
		return ""
	}
	if pp := extractProjectPath(s.WebURL); pp != "" {
		return pp
	}
	return strconv.FormatInt(s.ProjectID, 10)
}

// writeSimpleSnippetTable is an internal helper for the snippets package.
func writeSimpleSnippetTable(b *strings.Builder, snippets []Output) {
	b.WriteString("| ID | Title | Visibility | Author | Files |\n")
	b.WriteString("|---|---|---|---|---|\n")
	for _, s := range snippets {
		fmt.Fprintf(b, "| %d | %s | %s | @%s | %d |\n",
			s.ID, toolutil.MdTitleLink(toolutil.EscapeMdTableCell(s.Title), s.WebURL), s.Visibility, s.Author.Username, len(s.Files))
	}
}

// ---------------------------------------------------------------------------
// Personal Snippet Handlers (SnippetsService)
// ---------------------------------------------------------------------------.

// ListInput represents the input for listing current user's snippets.
type ListInput struct {
	toolutil.PaginationInput
}

// List lists all snippets for the current user.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	opts := &gl.ListSnippetsOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(input.Page),
			PerPage: int64(input.PerPage),
		},
	}
	snippets, resp, err := client.GL().Snippets.ListSnippets(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("snippet_list", err)
	}
	out := ListOutput{Pagination: toolutil.PaginationFromResponse(resp)}
	for _, s := range snippets {
		out.Snippets = append(out.Snippets, convertSnippet(s))
	}
	return out, nil
}

// ListAllInput represents the input for listing all snippets (admin).
type ListAllInput struct {
	CreatedAfter  string `json:"created_after,omitempty" jsonschema:"Filter snippets created after (ISO 8601)"`
	CreatedBefore string `json:"created_before,omitempty" jsonschema:"Filter snippets created before (ISO 8601)"`
	toolutil.PaginationInput
}

// ListAll lists all snippets (admin endpoint).
func ListAll(ctx context.Context, client *gitlabclient.Client, input ListAllInput) (ListOutput, error) {
	opts := &gl.ListAllSnippetsOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(input.Page),
			PerPage: int64(input.PerPage),
		},
	}
	if input.CreatedAfter != "" {
		t, err := time.Parse(time.RFC3339, input.CreatedAfter)
		if err == nil {
			isoTime := gl.ISOTime(t)
			opts.CreatedAfter = &isoTime
		}
	}
	if input.CreatedBefore != "" {
		t, err := time.Parse(time.RFC3339, input.CreatedBefore)
		if err == nil {
			isoTime := gl.ISOTime(t)
			opts.CreatedBefore = &isoTime
		}
	}
	snippets, resp, err := client.GL().Snippets.ListAllSnippets(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("snippet_list_all", err)
	}
	out := ListOutput{Pagination: toolutil.PaginationFromResponse(resp)}
	for _, s := range snippets {
		out.Snippets = append(out.Snippets, convertSnippet(s))
	}
	return out, nil
}

// GetInput represents the input for getting a single snippet.
type GetInput struct {
	SnippetID int64 `json:"snippet_id" jsonschema:"Snippet ID,required"`
}

// Get retrieves a single snippet by ID.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if input.SnippetID == 0 {
		return Output{}, toolutil.ErrFieldRequired("snippet_id")
	}
	snippet, _, err := client.GL().Snippets.GetSnippet(input.SnippetID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("snippet_get", err)
	}
	return convertSnippet(snippet), nil
}

// ContentInput represents the input for getting snippet content.
type ContentInput struct {
	SnippetID int64 `json:"snippet_id" jsonschema:"Snippet ID,required"`
}

// Content retrieves the raw content of a snippet.
func Content(ctx context.Context, client *gitlabclient.Client, input ContentInput) (ContentOutput, error) {
	if input.SnippetID == 0 {
		return ContentOutput{}, toolutil.ErrFieldRequired("snippet_id")
	}
	data, _, err := client.GL().Snippets.SnippetContent(input.SnippetID, gl.WithContext(ctx))
	if err != nil {
		return ContentOutput{}, toolutil.WrapErrWithMessage("snippet_content", err)
	}
	return ContentOutput{SnippetID: input.SnippetID, Content: string(data)}, nil
}

// FileContentInput represents the input for getting a specific snippet file.
type FileContentInput struct {
	SnippetID int64  `json:"snippet_id" jsonschema:"Snippet ID,required"`
	Ref       string `json:"ref" jsonschema:"Git ref (branch, tag, or commit SHA),required"`
	FileName  string `json:"file_name" jsonschema:"File name to retrieve,required"`
}

// FileContent retrieves the raw content of a specific file in a snippet.
func FileContent(ctx context.Context, client *gitlabclient.Client, input FileContentInput) (FileContentOutput, error) {
	if input.SnippetID == 0 {
		return FileContentOutput{}, toolutil.ErrFieldRequired("snippet_id")
	}
	if input.Ref == "" {
		return FileContentOutput{}, toolutil.ErrFieldRequired("ref")
	}
	if input.FileName == "" {
		return FileContentOutput{}, toolutil.ErrFieldRequired("file_name")
	}
	data, _, err := client.GL().Snippets.SnippetFileContent(input.SnippetID, input.Ref, input.FileName, gl.WithContext(ctx))
	if err != nil {
		return FileContentOutput{}, toolutil.WrapErrWithMessage("snippet_file_content", err)
	}
	return FileContentOutput{
		SnippetID: input.SnippetID,
		Ref:       input.Ref,
		FileName:  input.FileName,
		Content:   string(data),
	}, nil
}

// CreateInput represents the input for creating a personal snippet.
type CreateInput struct {
	Title       string            `json:"title" jsonschema:"Snippet title,required"`
	FileName    string            `json:"file_name,omitempty" jsonschema:"File name (single-file snippet, deprecated in favor of files)"`
	Description string            `json:"description,omitempty" jsonschema:"Snippet description"`
	ContentBody string            `json:"content,omitempty" jsonschema:"Snippet content (single-file, deprecated in favor of files)"`
	Visibility  string            `json:"visibility,omitempty" jsonschema:"Visibility: private, internal, or public"`
	Files       []CreateFileInput `json:"files,omitempty" jsonschema:"Files to include in the snippet"`
}

// Create creates a new personal snippet.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if input.Title == "" {
		return Output{}, toolutil.ErrFieldRequired("title")
	}
	opts := &gl.CreateSnippetOptions{
		Title: new(input.Title),
	}
	if input.FileName != "" {
		opts.FileName = new(input.FileName)
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	if input.ContentBody != "" {
		opts.Content = new(input.ContentBody)
	}
	if input.Visibility != "" {
		v := gl.VisibilityValue(input.Visibility)
		opts.Visibility = &v
	}
	if len(input.Files) > 0 {
		files := make([]*gl.CreateSnippetFileOptions, len(input.Files))
		for i, f := range input.Files {
			files[i] = &gl.CreateSnippetFileOptions{
				FilePath: new(f.FilePath),
				Content:  new(f.Content),
			}
		}
		opts.Files = &files
	}
	snippet, _, err := client.GL().Snippets.CreateSnippet(opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("snippet_create", err)
	}
	return convertSnippet(snippet), nil
}

// UpdateInput represents the input for updating a personal snippet.
type UpdateInput struct {
	SnippetID   int64             `json:"snippet_id" jsonschema:"Snippet ID,required"`
	Title       string            `json:"title,omitempty" jsonschema:"New title"`
	FileName    string            `json:"file_name,omitempty" jsonschema:"New file name (single-file, deprecated in favor of files)"`
	Description string            `json:"description,omitempty" jsonschema:"New description"`
	ContentBody string            `json:"content,omitempty" jsonschema:"New content (single-file, deprecated in favor of files)"`
	Visibility  string            `json:"visibility,omitempty" jsonschema:"New visibility: private, internal, or public"`
	Files       []UpdateFileInput `json:"files,omitempty" jsonschema:"File operations to apply"`
}

// Update updates an existing personal snippet.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if input.SnippetID == 0 {
		return Output{}, toolutil.ErrFieldRequired("snippet_id")
	}
	opts := buildUpdateOpts(input)
	snippet, _, err := client.GL().Snippets.UpdateSnippet(input.SnippetID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("snippet_update", err)
	}
	return convertSnippet(snippet), nil
}

// buildUpdateOpts constructs the request parameters from the input.
func buildUpdateOpts(input UpdateInput) *gl.UpdateSnippetOptions {
	opts := &gl.UpdateSnippetOptions{}
	if input.Title != "" {
		opts.Title = new(input.Title)
	}
	if input.FileName != "" {
		opts.FileName = new(input.FileName)
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	if input.ContentBody != "" {
		opts.Content = new(input.ContentBody)
	}
	if input.Visibility != "" {
		v := gl.VisibilityValue(input.Visibility)
		opts.Visibility = &v
	}
	if len(input.Files) > 0 {
		opts.Files = buildUpdateFileOpts(input.Files)
	}
	return opts
}

// buildUpdateFileOpts constructs the request parameters from the input.
func buildUpdateFileOpts(files []UpdateFileInput) *[]*gl.UpdateSnippetFileOptions {
	out := make([]*gl.UpdateSnippetFileOptions, len(files))
	for i, f := range files {
		out[i] = &gl.UpdateSnippetFileOptions{
			Action:   new(f.Action),
			FilePath: new(f.FilePath),
		}
		if f.Content != "" {
			out[i].Content = new(f.Content)
		}
		if f.PreviousPath != "" {
			out[i].PreviousPath = new(f.PreviousPath)
		}
	}
	return &out
}

// DeleteInput represents the input for deleting a personal snippet.
type DeleteInput struct {
	SnippetID int64 `json:"snippet_id" jsonschema:"Snippet ID,required"`
}

// Delete deletes a personal snippet.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if input.SnippetID == 0 {
		return toolutil.ErrFieldRequired("snippet_id")
	}
	_, err := client.GL().Snippets.DeleteSnippet(input.SnippetID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("snippet_delete", err)
	}
	return nil
}

// ExploreInput represents the input for exploring public snippets.
type ExploreInput struct {
	toolutil.PaginationInput
}

// Explore lists all public snippets.
func Explore(ctx context.Context, client *gitlabclient.Client, input ExploreInput) (ListOutput, error) {
	opts := &gl.ExploreSnippetsOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(input.Page),
			PerPage: int64(input.PerPage),
		},
	}
	snippets, resp, err := client.GL().Snippets.ExploreSnippets(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("snippet_explore", err)
	}
	out := ListOutput{Pagination: toolutil.PaginationFromResponse(resp)}
	for _, s := range snippets {
		out.Snippets = append(out.Snippets, convertSnippet(s))
	}
	return out, nil
}
