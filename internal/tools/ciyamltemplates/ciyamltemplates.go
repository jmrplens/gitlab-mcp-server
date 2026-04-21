// Package ciyamltemplates implements MCP tools for GitLab CI YAML Templates API.
package ciyamltemplates

import (
	"context"
	"errors"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------.

// ListInput is the input for listing CI YAML templates.
type ListInput struct {
	Page    int64 `json:"page,omitempty" jsonschema:"Page number for pagination"`
	PerPage int64 `json:"per_page,omitempty" jsonschema:"Items per page"`
}

// TemplateListItem represents a CI YAML template in a list.
type TemplateListItem struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// ListOutput is the output for listing CI YAML templates.
type ListOutput struct {
	toolutil.HintableOutput
	Templates  []TemplateListItem        `json:"templates"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// List lists all available CI YAML templates.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	opts := &gl.ListCIYMLTemplatesOptions{
		ListOptions: gl.ListOptions{Page: input.Page, PerPage: input.PerPage},
	}
	items, resp, err := client.GL().CIYMLTemplate.ListAllTemplates(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("list_ci_yml_templates", err)
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
		return GetOutput{}, toolutil.WrapErrWithMessage("get_ci_yml_template", err)
	}
	return GetOutput{Name: t.Name, Content: t.Content}, nil
}

// ---------------------------------------------------------------------------
// Formatters
// ---------------------------------------------------------------------------.
