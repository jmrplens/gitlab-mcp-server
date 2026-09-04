package projecttemplates

import (
	"context"
	"errors"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// List.

// ListInput contains parameters for listing project templates of a given type.
// It mirrors gl.ListProjectTemplatesOptions (id, type) and the embedded
// gl.ListOptions (order_by, sort, pagination, page_token) so every SDK filter
// and pagination knob is available on the MCP surface.
type ListInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	TemplateType string               `json:"template_type" jsonschema:"Template family in the request path: dockerfiles, gitignores, gitlab_ci_ymls, licenses, issues, merge_requests,required"`
	ID           int64                `json:"id,omitempty" jsonschema:"Filter to a single template by numeric id (maps to gl.ListProjectTemplatesOptions.ID)"`
	Type         string               `json:"type,omitempty" jsonschema:"Optional secondary template type filter passed as the 'type' query parameter (maps to gl.ListProjectTemplatesOptions.Type)"`
	OrderBy      string               `json:"order_by,omitempty" jsonschema:"Column by which to order keyset-paginated results"`
	Sort         string               `json:"sort,omitempty" jsonschema:"Sort direction for keyset-paginated results (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
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
	opts := &gl.ListProjectTemplatesOptions{
		ID:   optInt64(input.ID),
		Type: optStr(input.Type),
	}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
	}
	templates, resp, err := client.GL().ProjectTemplates.ListTemplates(
		string(input.ProjectID), input.TemplateType, opts, gl.WithContext(ctx),
	)
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("gitlab_list_project_templates", err, http.StatusNotFound, "verify project_id and template_type (dockerfiles, gitignores, gitlab_ci_ymls, licenses, issues, merge_requests)")
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
	TemplateType string               `json:"template_type" jsonschema:"Template family in the request path: dockerfiles, gitignores, gitlab_ci_ymls, licenses, issues, merge_requests,required"`
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
		return GetOutput{}, toolutil.WrapErrWithStatusHint("gitlab_get_project_template", err, http.StatusNotFound, "verify template_type and template_name with gitlab_list_project_templates")
	}
	return GetOutput{TemplateItem: templateFromGL(tpl)}, nil
}

// helpers.

// optStr returns a pointer to s, or nil when s is empty, so optional string
// query parameters are omitted from the request when not supplied.
func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// optInt64 returns a pointer to v, or nil when v is zero, so optional numeric
// query parameters are omitted from the request when not supplied.
func optInt64(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}

// templateFromGL maps template from gl between API and evaluator models.
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
