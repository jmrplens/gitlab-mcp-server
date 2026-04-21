// project_snippets implements project-scoped snippet CRUD operations.

package snippets

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ---------------------------------------------------------------------------
// Project Snippet Handlers (ProjectSnippetsService)
// ---------------------------------------------------------------------------.

// ProjectListInput represents the input for listing project snippets.
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
		string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("project_snippet_list", err)
	}
	out := ListOutput{Pagination: toolutil.PaginationFromResponse(resp)}
	for _, s := range snippets {
		out.Snippets = append(out.Snippets, convertSnippet(s))
	}
	return out, nil
}

// ProjectGetInput represents the input for getting a project snippet.
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
		string(input.ProjectID), input.SnippetID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("project_snippet_get", err)
	}
	return convertSnippet(snippet), nil
}

// ProjectContentInput represents the input for getting project snippet content.
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
		string(input.ProjectID), input.SnippetID, gl.WithContext(ctx))
	if err != nil {
		return ContentOutput{}, toolutil.WrapErrWithMessage("project_snippet_content", err)
	}
	return ContentOutput{SnippetID: input.SnippetID, Content: string(data)}, nil
}

// ProjectCreateInput represents the input for creating a project snippet.
type ProjectCreateInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	Title       string               `json:"title" jsonschema:"Snippet title,required"`
	Description string               `json:"description,omitempty" jsonschema:"Snippet description"`
	Visibility  string               `json:"visibility,omitempty" jsonschema:"Visibility: private, internal, or public"`
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
	opts := &gl.CreateProjectSnippetOptions{
		Title: new(input.Title),
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
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
		string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("project_snippet_create", err)
	}
	return convertSnippet(snippet), nil
}

// ProjectUpdateInput represents the input for updating a project snippet.
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
		string(input.ProjectID), input.SnippetID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("project_snippet_update", err)
	}
	return convertSnippet(snippet), nil
}

// ProjectDeleteInput represents the input for deleting a project snippet.
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
		string(input.ProjectID), input.SnippetID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("project_snippet_delete", err)
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
		files := make([]*gl.UpdateSnippetFileOptions, len(input.Files))
		for i, f := range input.Files {
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
		opts.Files = &files
	} else if input.FileName != "" || input.ContentBody != "" {
		file := &gl.UpdateSnippetFileOptions{
			Action: new("update"),
		}
		if input.FileName != "" {
			file.FilePath = new(input.FileName)
		}
		if input.ContentBody != "" {
			file.Content = new(input.ContentBody)
		}
		files := []*gl.UpdateSnippetFileOptions{file}
		opts.Files = &files
	}
	return opts
}
