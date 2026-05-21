package projectiterations

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/iterationdata"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ListInput defines parameters for listing project iterations.
type ListInput struct {
	ProjectID        toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	State            string               `json:"state,omitempty" jsonschema:"Filter by state: opened, upcoming, current, closed, all"`
	Search           string               `json:"search,omitempty" jsonschema:"Search by title"`
	IncludeAncestors bool                 `json:"include_ancestors,omitempty" jsonschema:"Include ancestor iterations"`
	toolutil.PaginationInput
}

// Output represents a project iteration.
type Output = iterationdata.Output

// ListOutput wraps a list of project iterations.
type ListOutput struct {
	toolutil.HintableOutput
	Iterations []Output                  `json:"iterations"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

func toOutput(it *gl.ProjectIteration) Output {
	return iterationdata.ProjectOutput(it)
}

// List lists project iterations.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	opts := iterationdata.NewProjectListOptions(input.Page, input.PerPage, input.State, input.Search, input.IncludeAncestors)
	items, resp, err := client.GL().ProjectIterations.ListProjectIterations(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("gitlab_list_project_iterations", err, http.StatusNotFound, "verify project_id with gitlab_project_get \u2014 iterations require Premium license")
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
