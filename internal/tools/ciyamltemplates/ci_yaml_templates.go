package ciyamltemplates

import (
	"context"
	"errors"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------.

// ListInput is the input for listing CI YAML templates. It mirrors
// gl.ListCIYMLTemplatesOptions, whose only field is the embedded gl.ListOptions.
// OrderBy and Sort map onto gl.ListOptions.OrderBy/Sort, while offset
// (page/per_page) and keyset (pagination/page_token) parameters map onto the
// embedded gl.ListOptions via toolutil.ApplyListOptions.
type ListInput struct {
	OrderBy string `json:"order_by,omitempty" jsonschema:"Column to order results by for keyset pagination (e.g. id, name, created_at, updated_at)"`
	Sort    string `json:"sort,omitempty"     jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// applyOrderSort copies the order_by and sort fields onto a gl.ListOptions,
// leaving unset fields untouched so GitLab applies its defaults.
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

// TemplateListItem represents a CI YAML template in a list.
type TemplateListItem = toolutil.TemplateMarkdown

// ListOutput is the output for listing CI YAML templates.
type ListOutput struct {
	toolutil.HintableOutput
	Templates  []TemplateListItem        `json:"templates"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// List lists all available CI YAML templates.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	opts := &gl.ListCIYMLTemplatesOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	applyOrderSort(&opts.ListOptions, input.OrderBy, input.Sort)
	items, resp, err := client.GL().CIYMLTemplate.ListAllTemplates(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list_ci_yml_templates", err, http.StatusForbidden, "verify your token has read_api scope")
	}
	templates := make([]TemplateListItem, 0, len(items))
	for _, t := range items {
		templates = append(templates, TemplateListItem{Key: t.Key, Name: t.Name})
	}
	return ListOutput{
		Templates:  templates,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------.

// GetInput is the input for getting a CI YAML template.
type GetInput struct {
	Key string `json:"key" jsonschema:"Template key (e.g. Go, Python),required"`
}

// GetOutput is the output for getting a CI YAML template.
type GetOutput struct {
	toolutil.HintableOutput
	Name    string `json:"name"`
	Content string `json:"content"`
}

// Get gets a single CI YAML template by key.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (GetOutput, error) {
	if input.Key == "" {
		return GetOutput{}, errors.New("get_ci_yml_template: key is required. Use list action to see available template keys")
	}
	t, _, err := client.GL().CIYMLTemplate.GetTemplate(input.Key, gl.WithContext(ctx))
	if err != nil {
		return GetOutput{}, toolutil.WrapErrWithStatusHint("get_ci_yml_template", err, http.StatusNotFound, "verify name with gitlab_list_ci_yml_templates")
	}
	return GetOutput{Name: t.Name, Content: t.Content}, nil
}

// ---------------------------------------------------------------------------
// Formatters
// ---------------------------------------------------------------------------.
