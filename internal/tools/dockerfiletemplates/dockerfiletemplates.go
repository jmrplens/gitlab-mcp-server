// Package dockerfiletemplates implements MCP tools for GitLab Dockerfile Templates API.
package dockerfiletemplates

import (
	"context"
	"errors"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ListInput is the input for listing Dockerfile templates.
type ListInput struct {
	Page    int64 `json:"page,omitempty" jsonschema:"Page number for pagination"`
	PerPage int64 `json:"per_page,omitempty" jsonschema:"Items per page"`
}

// TemplateListItem represents a Dockerfile template in a list.
type TemplateListItem struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// ListOutput is the output for listing Dockerfile templates.
type ListOutput struct {
	toolutil.HintableOutput
	Templates  []TemplateListItem        `json:"templates"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// List lists all available Dockerfile templates.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	opts := &gl.ListDockerfileTemplatesOptions{
		ListOptions: gl.ListOptions{Page: input.Page, PerPage: input.PerPage},
	}
	items, resp, err := client.GL().DockerfileTemplate.ListTemplates(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("list_dockerfile_templates", err)
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

// GetInput is the input for getting a Dockerfile template.
type GetInput struct {
	Key string `json:"key" jsonschema:"Template key,required"`
}

// GetOutput is the output for getting a Dockerfile template.
type GetOutput struct {
	toolutil.HintableOutput
	Name    string `json:"name"`
	Content string `json:"content"`
}

// Get gets a single Dockerfile template by key.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (GetOutput, error) {
	if input.Key == "" {
		return GetOutput{}, errors.New("get_dockerfile_template: key is required. Use list action to see available template keys")
	}
	t, _, err := client.GL().DockerfileTemplate.GetTemplate(input.Key, gl.WithContext(ctx))
	if err != nil {
		return GetOutput{}, toolutil.WrapErrWithMessage("get_dockerfile_template", err)
	}
	return GetOutput{Name: t.Name, Content: t.Content}, nil
}
