package gitignoretemplates

import (
	"context"
	"errors"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ListInput is the input for listing gitignore templates. OrderBy, Sort, and the
// embedded keyset parameters map onto the gl.ListTemplatesOptions embedded
// gl.ListOptions, mirroring the full client-go pagination surface.
type ListInput struct {
	OrderBy string `json:"order_by,omitempty" jsonschema:"Column to order results by for keyset pagination (e.g. id, name, created_at, updated_at)"`
	Sort    string `json:"sort,omitempty"     jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// TemplateListItem represents a gitignore template in a list.
type TemplateListItem = toolutil.TemplateMarkdown

// ListOutput is the output for listing gitignore templates.
type ListOutput struct {
	toolutil.HintableOutput
	Templates  []TemplateListItem        `json:"templates"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// List lists all available gitignore templates.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	opts := &gl.ListTemplatesOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
	}
	items, resp, err := client.GL().GitIgnoreTemplates.ListTemplates(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list_gitignore_templates", err, http.StatusForbidden, "verify your token has read_api scope")
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

// GetInput is the input for getting a gitignore template.
type GetInput struct {
	Key string `json:"key" jsonschema:"Template key (e.g. Go, Python, Node),required"`
}

// GetOutput is the output for getting a gitignore template.
type GetOutput struct {
	toolutil.HintableOutput
	Name    string `json:"name"`
	Content string `json:"content"`
}

// Get gets a single gitignore template by key.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (GetOutput, error) {
	if input.Key == "" {
		return GetOutput{}, errors.New("get_gitignore_template: key is required. Use list action to see available template keys")
	}
	t, _, err := client.GL().GitIgnoreTemplates.GetTemplate(input.Key, gl.WithContext(ctx))
	if err != nil {
		return GetOutput{}, toolutil.WrapErrWithStatusHint("get_gitignore_template", err, http.StatusNotFound, "verify name with gitlab_list_gitignore_templates")
	}
	return GetOutput{Name: t.Name, Content: t.Content}, nil
}
