package groupiterations

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/iterationdata"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
type Output = iterationdata.Output

// ListOutput wraps a list of group iterations.
type ListOutput struct {
	toolutil.HintableOutput
	Iterations []Output                  `json:"iterations"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

func toOutput(it *gl.GroupIteration) Output {
	return iterationdata.GroupOutput(it)
}

// List lists group iterations.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.GroupID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	opts := iterationdata.NewGroupListOptions(input.Page, input.PerPage, input.State, input.Search, input.IncludeAncestors)
	items, resp, err := client.GL().GroupIterations.ListGroupIterations(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("gitlab_list_group_iterations", err, http.StatusNotFound, "verify group_id with gitlab_group_get \u2014 iterations require Premium license")
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
