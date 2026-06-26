package snippets

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ---------------------------------------------------------------------------
// Shared output types
// ---------------------------------------------------------------------------.

// SnippetAuthorOutput mirrors gl.SnippetAuthor, the full author object embedded
// in snippet payloads. Per the 1:1 audit policy it is a full nested object
// surfaced on the canonical "author" key, replacing the previously flattened
// author scalars. It is replicated here rather than imported from a sibling
// package to preserve the zero-import-cycle constraint (C-IMPORTS).
type SnippetAuthorOutput struct {
	ID        int64      `json:"id"`
	Username  string     `json:"username"`
	Email     string     `json:"email"`
	Name      string     `json:"name"`
	State     string     `json:"state"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
}

// snippetAuthorOutput maps a gl.SnippetAuthor into its output shape.
func snippetAuthorOutput(a gl.SnippetAuthor) *SnippetAuthorOutput {
	return &SnippetAuthorOutput{
		ID:        a.ID,
		Username:  a.Username,
		Email:     a.Email,
		Name:      a.Name,
		State:     a.State,
		CreatedAt: a.CreatedAt,
	}
}

// FileOutput mirrors gl.SnippetFile, the file object embedded in snippet
// payloads (path and raw URL), surfaced on the canonical "files" key.
type FileOutput struct {
	Path   string `json:"path"`
	RawURL string `json:"raw_url"`
}

// Output represents a single snippet. It mirrors gl.Snippet, surfacing the full
// nested author object and repository_storage per the 1:1 audit policy.
type Output struct {
	toolutil.HintableOutput
	ID                int64                `json:"id"`
	Title             string               `json:"title"`
	FileName          string               `json:"file_name"`
	Description       string               `json:"description"`
	Visibility        string               `json:"visibility"`
	Author            *SnippetAuthorOutput `json:"author,omitempty"`
	ProjectID         int64                `json:"project_id,omitempty"`
	WebURL            string               `json:"web_url"`
	RawURL            string               `json:"raw_url"`
	RepositoryStorage string               `json:"repository_storage,omitempty" tier:"premium"`
	Files             []FileOutput         `json:"files,omitempty"`
	CreatedAt         *time.Time           `json:"created_at,omitempty"`
	UpdatedAt         *time.Time           `json:"updated_at,omitempty"`
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

// convertSnippet maps a GitLab snippet into the MCP output shape.
func convertSnippet(s *gl.Snippet) Output {
	out := Output{
		ID:                s.ID,
		Title:             s.Title,
		FileName:          s.FileName,
		Description:       s.Description,
		Visibility:        s.Visibility,
		ProjectID:         s.ProjectID,
		WebURL:            s.WebURL,
		RawURL:            s.RawURL,
		RepositoryStorage: s.RepositoryStorage,
		CreatedAt:         s.CreatedAt,
		UpdatedAt:         s.UpdatedAt,
	}
	out.Author = snippetAuthorOutput(s.Author)
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
	FilePath     string `json:"file_path" jsonschema:"Snippet file path to create/update/delete; for project_update use the file_path returned by project_get,required"`
	Content      string `json:"content,omitempty" jsonschema:"File content (for create/update)"`
	PreviousPath string `json:"previous_path,omitempty" jsonschema:"Previous file path (for move)"`
}

// snippetVisibility returns a GitLab visibility value, defaulting snippets to private.
func snippetVisibility(value string) *gl.VisibilityValue {
	if value == "" {
		value = "private"
	}
	visibility := gl.VisibilityValue(value)
	return &visibility
}

// applyOrderSort copies the order_by and sort fields onto a gl.ListOptions,
// setting only the values the caller supplied. These fields drive keyset
// ordering for the snippet list endpoints.
func applyOrderSort(opts *gl.ListOptions, orderBy, sort string) {
	if opts == nil {
		return
	}
	if orderBy != "" {
		opts.OrderBy = orderBy
	}
	if sort != "" {
		opts.Sort = sort
	}
}

// createSnippetFiles creates snippet files for the snippets package.
func createSnippetFiles(files []CreateFileInput) *[]*gl.CreateSnippetFileOptions {
	if len(files) == 0 {
		return nil
	}
	options := make([]*gl.CreateSnippetFileOptions, len(files))
	for i, file := range files {
		options[i] = &gl.CreateSnippetFileOptions{
			FilePath: new(file.FilePath),
			Content:  new(file.Content),
		}
	}
	return &options
}

// validateCreateSnippetContent validates the two supported snippet creation
// modes: single-file snippets require fileName and content when files is empty,
// while multi-file snippets require FilePath and Content on every file entry.
// Missing single-file fields return [toolutil.ErrFieldRequired]; invalid
// multi-file entries include the offending files[index] field in the error.
func validateCreateSnippetContent(fileName, content string, files []CreateFileInput) error {
	if len(files) == 0 {
		if fileName == "" {
			return toolutil.ErrFieldRequired("file_name")
		}
		if content == "" {
			return toolutil.ErrFieldRequired("content")
		}
		return nil
	}
	for index, file := range files {
		if file.FilePath == "" {
			return fmt.Errorf("files[%d].file_path is required", index)
		}
		if file.Content == "" {
			return fmt.Errorf("files[%d].content is required", index)
		}
	}
	return nil
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

// snippetsHaveProject reports whether any snippet output belongs to a project.
func snippetsHaveProject(snippets []Output) bool {
	for _, s := range snippets {
		if s.ProjectID != 0 {
			return true
		}
	}
	return false
}

// writeProjectSnippetTable writes project snippet table to disk.
func writeProjectSnippetTable(b *strings.Builder, snippets []Output) {
	b.WriteString("| ID | Title | Project | Visibility | Author | Files |\n")
	b.WriteString("|---|---|---|---|---|---|\n")
	for _, s := range snippets {
		proj := resolveProjectLabel(s)
		fmt.Fprintf(b, "| %d | %s | %s | %s | @%s | %d |\n",
			s.ID, toolutil.MdTitleLink(toolutil.EscapeMdTableCell(s.Title), s.WebURL), proj, s.Visibility, authorUsername(s.Author), len(s.Files))
	}
}

// resolveProjectLabel resolves project label for the snippets package.
func resolveProjectLabel(s Output) string {
	if s.ProjectID == 0 {
		return ""
	}
	if pp := extractProjectPath(s.WebURL); pp != "" {
		return pp
	}
	return strconv.FormatInt(s.ProjectID, 10)
}

// writeSimpleSnippetTable writes simple snippet table to disk.
func writeSimpleSnippetTable(b *strings.Builder, snippets []Output) {
	b.WriteString("| ID | Title | Visibility | Author | Files |\n")
	b.WriteString("|---|---|---|---|---|\n")
	for _, s := range snippets {
		fmt.Fprintf(b, "| %d | %s | %s | @%s | %d |\n",
			s.ID, toolutil.MdTitleLink(toolutil.EscapeMdTableCell(s.Title), s.WebURL), s.Visibility, authorUsername(s.Author), len(s.Files))
	}
}

// authorUsername returns the snippet author's username, or an empty string when
// the author object is nil (e.g. minimal payloads).
func authorUsername(a *SnippetAuthorOutput) string {
	if a == nil {
		return ""
	}
	return a.Username
}

// ---------------------------------------------------------------------------
// Personal Snippet Handlers (SnippetsService)
// ---------------------------------------------------------------------------.

// ListInput carries pagination and ordering for the authenticated user's
// snippets. OrderBy, Sort, pagination, and page_token map onto the embedded
// gl.ListOptions to mirror the SDK's keyset-capable list options.
type ListInput struct {
	OrderBy string `json:"order_by,omitempty" jsonschema:"Column to order results by for keyset pagination (e.g. id, created_at, updated_at)"`
	Sort    string `json:"sort,omitempty"     jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// List lists all snippets for the current user.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	opts := &gl.ListSnippetsOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	applyOrderSort(&opts.ListOptions, input.OrderBy, input.Sort)
	snippets, resp, err := client.GL().Snippets.ListSnippets(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("snippet_list", err, http.StatusUnauthorized,
			"this endpoint lists snippets owned by the authenticated user; ensure the access token has the 'api' scope")
	}
	out := ListOutput{Pagination: toolutil.PaginationFromResponse(resp)}
	for _, s := range snippets {
		out.Snippets = append(out.Snippets, convertSnippet(s))
	}
	return out, nil
}

// ListAllInput carries admin-only snippet listing filters, ordering, and
// pagination. CreatedAfter/CreatedBefore/RepositoryStorage mirror
// gl.ListAllSnippetsOptions, while OrderBy/Sort/pagination/page_token map onto
// the embedded gl.ListOptions.
type ListAllInput struct {
	CreatedAfter      string `json:"created_after,omitempty"      jsonschema:"Filter snippets created after (ISO 8601)"`
	CreatedBefore     string `json:"created_before,omitempty"     jsonschema:"Filter snippets created before (ISO 8601)"`
	RepositoryStorage string `json:"repository_storage,omitempty" tier:"premium" jsonschema:"Filter by repository storage name (admin only)"`
	OrderBy           string `json:"order_by,omitempty"           jsonschema:"Column to order results by for keyset pagination (e.g. id, created_at, updated_at)"`
	Sort              string `json:"sort,omitempty"               jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListAll lists all snippets (admin endpoint).
func ListAll(ctx context.Context, client *gitlabclient.Client, input ListAllInput) (ListOutput, error) {
	opts := &gl.ListAllSnippetsOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	applyOrderSort(&opts.ListOptions, input.OrderBy, input.Sort)
	if input.RepositoryStorage != "" {
		opts.RepositoryStorage = new(input.RepositoryStorage)
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
		return ListOutput{}, toolutil.WrapErrWithStatusHint("snippet_list_all", err, http.StatusForbidden,
			"listing all public snippets across the instance requires admin privileges")
	}
	out := ListOutput{Pagination: toolutil.PaginationFromResponse(resp)}
	for _, s := range snippets {
		out.Snippets = append(out.Snippets, convertSnippet(s))
	}
	return out, nil
}

// GetInput identifies a personal snippet by global snippet ID.
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
		return Output{}, toolutil.WrapErrWithStatusHint("snippet_get", err, http.StatusNotFound,
			"verify snippet_id with gitlab_snippet_list; private snippets are only accessible to the author")
	}
	return convertSnippet(snippet), nil
}

// ContentInput identifies the single-file snippet content to retrieve.
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
		return ContentOutput{}, toolutil.WrapErrWithStatusHint("snippet_content", err, http.StatusNotFound,
			"verify snippet_id with gitlab_snippet_list; for multi-file snippets use gitlab_snippet_file_content with a specific file_path")
	}
	return ContentOutput{SnippetID: input.SnippetID, Content: string(data)}, nil
}

// FileContentInput identifies a file inside a multi-file snippet at a Git ref.
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
		return FileContentOutput{}, toolutil.WrapErrWithStatusHint("snippet_file_content", err, http.StatusNotFound,
			"verify snippet_id and file_path; ref defaults to 'main' but the snippet may use a different default branch")
	}
	return FileContentOutput{
		SnippetID: input.SnippetID,
		Ref:       input.Ref,
		FileName:  input.FileName,
		Content:   string(data),
	}, nil
}

// CreateInput describes a personal snippet and its initial file content.
type CreateInput struct {
	Title       string            `json:"title" jsonschema:"Snippet title,required"`
	FileName    string            `json:"file_name,omitempty" jsonschema:"File name (single-file snippet, deprecated in favor of files)"`
	Description string            `json:"description,omitempty" jsonschema:"Snippet description"`
	ContentBody string            `json:"content,omitempty" jsonschema:"Snippet content (single-file, deprecated in favor of files)"`
	Visibility  string            `json:"visibility,omitempty" jsonschema:"Visibility: private, internal, or public; defaults to private when omitted"`
	Files       []CreateFileInput `json:"files,omitempty" jsonschema:"Files to include in the snippet"`
}

// Create creates a new personal snippet.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if input.Title == "" {
		return Output{}, toolutil.ErrFieldRequired("title")
	}
	if err := validateCreateSnippetContent(input.FileName, input.ContentBody, input.Files); err != nil {
		return Output{}, err
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
	opts.Visibility = snippetVisibility(input.Visibility)
	if files := createSnippetFiles(input.Files); files != nil {
		opts.Files = files
	}
	snippet, _, err := client.GL().Snippets.CreateSnippet(opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("snippet_create", err, http.StatusBadRequest,
			"title and content are required; visibility must be 'private', 'internal', or 'public'; instance may have disabled snippet creation")
	}
	return convertSnippet(snippet), nil
}

// UpdateInput identifies a personal snippet and the metadata or file operations to apply.
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
		return Output{}, toolutil.WrapErrWithStatusHint("snippet_update", err, http.StatusForbidden,
			"updating a snippet requires being the author or having admin privileges; verify snippet_id with gitlab_snippet_list")
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

// DeleteInput identifies the personal snippet to delete.
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
		return toolutil.WrapErrWithStatusHint("snippet_delete", err, http.StatusForbidden,
			"deleting a snippet requires being the author or having admin privileges")
	}
	return nil
}

// ExploreInput carries pagination and ordering for public snippet discovery.
// OrderBy/Sort/pagination/page_token map onto the embedded gl.ListOptions.
type ExploreInput struct {
	OrderBy string `json:"order_by,omitempty" jsonschema:"Column to order results by for keyset pagination (e.g. id, created_at, updated_at)"`
	Sort    string `json:"sort,omitempty"     jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// Explore lists all public snippets.
func Explore(ctx context.Context, client *gitlabclient.Client, input ExploreInput) (ListOutput, error) {
	opts := &gl.ExploreSnippetsOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	applyOrderSort(&opts.ListOptions, input.OrderBy, input.Sort)
	snippets, resp, err := client.GL().Snippets.ExploreSnippets(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("snippet_explore", err, http.StatusForbidden,
			"exploring all public snippets may be restricted by instance configuration")
	}
	out := ListOutput{Pagination: toolutil.PaginationFromResponse(resp)}
	for _, s := range snippets {
		out.Snippets = append(out.Snippets, convertSnippet(s))
	}
	return out, nil
}
