// Package projecttemplates implements MCP tools for GitLab project template operations.
package projecttemplates

import (
	"context"
	"errors"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// List.

// ListInput contains parameters for listing project templates of a given type.
type ListInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	TemplateType string               `json:"template_type" jsonschema:"Template type: dockerfiles, gitignores, gitlab_ci_ymls, licenses,required"`
	Page         int64                `json:"page" jsonschema:"Page number for pagination"`
	PerPage      int64                `json:"per_page" jsonschema:"Number of items per page"`
}

// ListOutput contains a list of project templates.
type ListOutput struct {
	toolutil.HintableOutput
	Templates  []TemplateItem            `json:"templates"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// TemplateItem represents a single project template entry.
type TemplateItem struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Nickname    string   `json:"nickname,omitempty"`
	Popular     bool     `json:"popular,omitempty"`
	HTMLURL     string   `json:"html_url,omitempty"`
	SourceURL   string   `json:"source_url,omitempty"`
	Description string   `json:"description,omitempty"`
	Conditions  []string `json:"conditions,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
	Limitations []string `json:"limitations,omitempty"`
	Content     string   `json:"content,omitempty"`
}

// List retrieves project templates of a given type.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	opts := &gl.ListProjectTemplatesOptions{}
	if input.Page > 0 || input.PerPage > 0 {
		opts.ListOptions = gl.ListOptions{Page: input.Page, PerPage: input.PerPage}
	}
	templates, resp, err := client.GL().ProjectTemplates.ListTemplates(
		string(input.ProjectID), input.TemplateType, opts, gl.WithContext(ctx),
	)
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("gitlab_list_project_templates", err)
	}
	items := make([]TemplateItem, 0, len(templates))
	for _, t := range templates {
		items = append(items, templateFromGL(t))
	}
	return ListOutput{
		Templates:  items,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// Get.

// GetInput contains parameters for getting a single project template.
type GetInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	TemplateType string               `json:"template_type" jsonschema:"Template type: dockerfiles, gitignores, gitlab_ci_ymls, licenses,required"`
	Key          string               `json:"key" jsonschema:"Template key/name,required"`
}

// GetOutput contains a single project template.
type GetOutput struct {
	toolutil.HintableOutput
	TemplateItem
}

// Get retrieves a single project template by type and key.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (GetOutput, error) {
	if input.Key == "" {
		return GetOutput{}, errors.New("get_project_template: key is required")
	}
	tpl, _, err := client.GL().ProjectTemplates.GetProjectTemplate(
		string(input.ProjectID), input.TemplateType, input.Key, gl.WithContext(ctx),
	)
	if err != nil {
		return GetOutput{}, toolutil.WrapErrWithMessage("gitlab_get_project_template", err)
	}
	return GetOutput{TemplateItem: templateFromGL(tpl)}, nil
}

// helpers.

// templateFromGL is an internal helper for the projecttemplates package.
func templateFromGL(t *gl.ProjectTemplate) TemplateItem {
	return TemplateItem{
		Key:         t.Key,
		Name:        t.Name,
		Nickname:    t.Nickname,
		Popular:     t.Popular,
		HTMLURL:     t.HTMLURL,
		SourceURL:   t.SourceURL,
		Description: t.Description,
		Conditions:  t.Conditions,
		Permissions: t.Permissions,
		Limitations: t.Limitations,
		Content:     t.Content,
	}
}

// formatters.
