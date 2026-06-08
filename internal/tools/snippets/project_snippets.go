package snippets

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ---------------------------------------------------------------------------
// Project Snippet Handlers (ProjectSnippetsService)
// ---------------------------------------------------------------------------.

// ProjectListInput selects a project snippet page.
type ProjectListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	toolutil.PaginationInput
}

// ProjectList lists snippets for a project.
func ProjectList(ctx context.Context, client *gitlabclient.Client, input ProjectListInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	opts := &gl.ListProjectSnippetsOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(input.Page),
			PerPage: int64(input.PerPage),
		},
	}
	snippets, resp, err := client.GL().ProjectSnippets.ListSnippets(
		string(input.ProjectID), opts, gl.WithContext(ctx),
	)
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("project_snippet_list", err, http.StatusNotFound,
			"verify project_id with gitlab_project_get; the project must have snippets enabled")
	}
	out := ListOutput{Pagination: toolutil.PaginationFromResponse(resp)}
	for _, s := range snippets {
		out.Snippets = append(out.Snippets, convertSnippet(s))
	}
	return out, nil
}

// ProjectGetInput identifies a snippet within its owning project.
type ProjectGetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	SnippetID int64                `json:"snippet_id" jsonschema:"Snippet ID,required"`
}

// ProjectGet retrieves a single project snippet.
func ProjectGet(ctx context.Context, client *gitlabclient.Client, input ProjectGetInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.SnippetID == 0 {
		return Output{}, toolutil.ErrFieldRequired("snippet_id")
	}
	snippet, _, err := client.GL().ProjectSnippets.GetSnippet(
		string(input.ProjectID), input.SnippetID, gl.WithContext(ctx),
	)
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("project_snippet_get", err, http.StatusNotFound,
			"verify snippet_id with gitlab_project_snippet_list; project_id must match the project that owns the snippet")
	}
	return convertSnippet(snippet), nil
}

// ProjectContentInput identifies the raw content for a project snippet.
type ProjectContentInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	SnippetID int64                `json:"snippet_id" jsonschema:"Snippet ID,required"`
}

// ProjectContent retrieves the raw content of a project snippet.
func ProjectContent(ctx context.Context, client *gitlabclient.Client, input ProjectContentInput) (ContentOutput, error) {
	if input.ProjectID == "" {
		return ContentOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.SnippetID == 0 {
		return ContentOutput{}, toolutil.ErrFieldRequired("snippet_id")
	}
	data, _, err := client.GL().ProjectSnippets.SnippetContent(
		string(input.ProjectID), input.SnippetID, gl.WithContext(ctx),
	)
	if err != nil {
		return ContentOutput{}, toolutil.WrapErrWithStatusHint("project_snippet_content", err, http.StatusNotFound,
			"verify snippet_id with gitlab_project_snippet_list")
	}
	return ContentOutput{SnippetID: input.SnippetID, Content: string(data)}, nil
}

// ProjectCreateInput describes a project snippet and its initial file content.
type ProjectCreateInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	Title       string               `json:"title" jsonschema:"Snippet title,required"`
	Description string               `json:"description,omitempty" jsonschema:"Snippet description"`
	Visibility  string               `json:"visibility,omitempty" jsonschema:"Visibility: private, internal, or public; defaults to private when omitted"`
	Files       []CreateFileInput    `json:"files,omitempty" jsonschema:"Files to include in the snippet"`
	FileName    string               `json:"file_name,omitempty" jsonschema:"File name (single-file, deprecated in favor of files)"`
	ContentBody string               `json:"content,omitempty" jsonschema:"Content (single-file, deprecated in favor of files)"`
}

// ProjectCreate creates a new project snippet.
func ProjectCreate(ctx context.Context, client *gitlabclient.Client, input ProjectCreateInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.Title == "" {
		return Output{}, toolutil.ErrFieldRequired("title")
	}
	if err := validateCreateSnippetContent(input.FileName, input.ContentBody, input.Files); err != nil {
		return Output{}, err
	}
	opts := &gl.CreateProjectSnippetOptions{
		Title: new(input.Title),
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	opts.Visibility = snippetVisibility(input.Visibility)
	if files := createSnippetFiles(input.Files); files != nil {
		opts.Files = files
	} else if input.FileName != "" || input.ContentBody != "" {
		file := &gl.CreateSnippetFileOptions{}
		if input.FileName != "" {
			file.FilePath = new(input.FileName)
		}
		if input.ContentBody != "" {
			file.Content = new(input.ContentBody)
		}
		files := []*gl.CreateSnippetFileOptions{file}
		opts.Files = &files
	}
	snippet, _, err := client.GL().ProjectSnippets.CreateSnippet(
		string(input.ProjectID), opts, gl.WithContext(ctx),
	)
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("project_snippet_create", err, http.StatusBadRequest,
			"title, file_name, and content are required; visibility must be 'private', 'internal', or 'public'; creating project snippets requires Developer role or higher")
	}
	return convertSnippet(snippet), nil
}

// ProjectUpdateInput identifies a project snippet and the metadata or file operations to apply.
type ProjectUpdateInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	SnippetID   int64                `json:"snippet_id" jsonschema:"Snippet ID,required"`
	Title       string               `json:"title,omitempty" jsonschema:"New title"`
	Description string               `json:"description,omitempty" jsonschema:"New description"`
	Visibility  string               `json:"visibility,omitempty" jsonschema:"New visibility: private, internal, or public"`
	Files       []UpdateFileInput    `json:"files,omitempty" jsonschema:"File operations to apply"`
	FileName    string               `json:"file_name,omitempty" jsonschema:"New file name (single-file, deprecated in favor of files)"`
	ContentBody string               `json:"content,omitempty" jsonschema:"New content (single-file, deprecated in favor of files)"`
}

// ProjectUpdate updates an existing project snippet.
func ProjectUpdate(ctx context.Context, client *gitlabclient.Client, input ProjectUpdateInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.SnippetID == 0 {
		return Output{}, toolutil.ErrFieldRequired("snippet_id")
	}
	opts := buildProjectUpdateOptions(input)
	snippet, _, err := client.GL().ProjectSnippets.UpdateSnippet(
		string(input.ProjectID), input.SnippetID, opts, gl.WithContext(ctx),
	)
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("project_snippet_update", err, http.StatusForbidden,
			"updating a project snippet requires being the author or Maintainer role; verify snippet_id with gitlab_project_snippet_list")
	}
	return convertSnippet(snippet), nil
}

// ProjectDeleteInput identifies the project snippet to delete.
type ProjectDeleteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	SnippetID int64                `json:"snippet_id" jsonschema:"Snippet ID,required"`
}

// ProjectDelete deletes a project snippet.
func ProjectDelete(ctx context.Context, client *gitlabclient.Client, input ProjectDeleteInput) error {
	if input.ProjectID == "" {
		return toolutil.ErrFieldRequired("project_id")
	}
	if input.SnippetID == 0 {
		return toolutil.ErrFieldRequired("snippet_id")
	}
	_, err := client.GL().ProjectSnippets.DeleteSnippet(
		string(input.ProjectID), input.SnippetID, gl.WithContext(ctx),
	)
	if err != nil {
		return toolutil.WrapErrWithStatusHint("project_snippet_delete", err, http.StatusForbidden,
			"deleting a project snippet requires being the author or Maintainer role")
	}
	return nil
}

// buildProjectUpdateOptions constructs the request parameters from the input.
func buildProjectUpdateOptions(input ProjectUpdateInput) *gl.UpdateProjectSnippetOptions {
	opts := &gl.UpdateProjectSnippetOptions{}
	if input.Title != "" {
		opts.Title = new(input.Title)
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	if input.Visibility != "" {
		v := gl.VisibilityValue(input.Visibility)
		opts.Visibility = &v
	}
	if len(input.Files) > 0 {
		files := updateSnippetFileOptions(input.Files)
		opts.Files = &files
		return opts
	}
	if input.FileName == "" && input.ContentBody == "" {
		return opts
	}
	files := []*gl.UpdateSnippetFileOptions{legacyUpdateSnippetFileOptions(input)}
	opts.Files = &files
	return opts
}

func updateSnippetFileOptions(inputFiles []UpdateFileInput) []*gl.UpdateSnippetFileOptions {
	files := make([]*gl.UpdateSnippetFileOptions, len(inputFiles))
	for i, f := range inputFiles {
		files[i] = &gl.UpdateSnippetFileOptions{
			Action:   new(f.Action),
			FilePath: new(f.FilePath),
		}
		if f.Content != "" {
			files[i].Content = new(f.Content)
		}
		if f.PreviousPath != "" {
			files[i].PreviousPath = new(f.PreviousPath)
		}
	}
	return files
}

func legacyUpdateSnippetFileOptions(input ProjectUpdateInput) *gl.UpdateSnippetFileOptions {
	file := &gl.UpdateSnippetFileOptions{Action: new("update")}
	if input.FileName != "" {
		file.FilePath = new(input.FileName)
	}
	if input.ContentBody != "" {
		file.Content = new(input.ContentBody)
	}
	return file
}
