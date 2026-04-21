// Package groupiterations implements MCP tool handlers for GitLab group iterations.
package groupiterations

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// ListInput defines parameters for listing group iterations.
type ListInput struct {
	GroupID          toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	State            string               `json:"state,omitempty" jsonschema:"Filter by state: opened, upcoming, current, closed, all"`
	Search           string               `json:"search,omitempty" jsonschema:"Search by title"`
	IncludeAncestors bool                 `json:"include_ancestors,omitempty" jsonschema:"Include ancestor iterations"`
	toolutil.PaginationInput
}

// Output represents a group iteration.
type Output struct {
	ID          int64  `json:"id"`
	IID         int64  `json:"iid"`
	Sequence    int64  `json:"sequence"`
	GroupID     int64  `json:"group_id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	State       int64  `json:"state"`
	WebURL      string `json:"web_url,omitempty"`
	StartDate   string `json:"start_date,omitempty"`
	DueDate     string `json:"due_date,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

// ListOutput wraps a list of group iterations.
type ListOutput struct {
	toolutil.HintableOutput
	Iterations []Output                  `json:"iterations"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

func toOutput(it *gl.GroupIteration) Output {
	if it == nil {
		return Output{}
	}
	out := Output{
		ID:          it.ID,
		IID:         it.IID,
		Sequence:    it.Sequence,
		GroupID:     it.GroupID,
		Title:       it.Title,
		Description: it.Description,
		State:       it.State,
		WebURL:      it.WebURL,
	}
	if it.StartDate != nil {
		out.StartDate = it.StartDate.String()
	}
	if it.DueDate != nil {
		out.DueDate = it.DueDate.String()
	}
	if it.CreatedAt != nil {
		out.CreatedAt = it.CreatedAt.Format("2006-01-02T15:04:05Z")
	}
	if it.UpdatedAt != nil {
		out.UpdatedAt = it.UpdatedAt.Format("2006-01-02T15:04:05Z")
	}
	return out
}

// List lists group iterations.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.GroupID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	opts := &gl.ListGroupIterationsOptions{
		ListOptions: gl.ListOptions{Page: int64(input.Page), PerPage: int64(input.PerPage)},
	}
	if input.State != "" {
		opts.State = new(input.State)
	}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.IncludeAncestors {
		opts.IncludeAncestors = new(true)
	}
	items, resp, err := client.GL().GroupIterations.ListGroupIterations(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("gitlab_list_group_iterations", err)
	}
	out := ListOutput{
		Iterations: make([]Output, 0, len(items)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for _, it := range items {
		out.Iterations = append(out.Iterations, toOutput(it))
	}
	return out, nil
}
