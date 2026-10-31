// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package groupiterations

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/iterationdata"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// ListInput defines parameters for listing group iterations. It mirrors
// gl.ListGroupIterationsOptions but for its ordering and keyset fields (issue
// 1600): GET /groups/:id/iterations declares state, search, in,
// include_ancestors, include_descendants, updated_before, updated_after, page
// and per_page (gitlab-api-live.json, 19.5.0-ee); list_iterations_for pages
// the finder's result by offset, and iterations_finder_params hands
// IterationsFinder no sort, so it orders by due date, title and id whatever is
// sent (ee/lib/api/iterations.rb:53-73 and ee/app/finders/iterations_finder.rb
// :116-123 at v19.5.0-ee).
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
		return ListOutput{}, toolutil.WrapErrWithStatusHint("gitlab_list_group_iterations", err, http.StatusNotFound, "verify group_id with group.get. Iterations require Premium license")
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
