// Package deploymentmergerequests implements an MCP tool handler for listing
// merge requests associated with a GitLab deployment. It wraps the
// DeploymentMergeRequestsService from client-go v2.
package deploymentmergerequests

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ListInput is the input for listing deployment merge requests.
type ListInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	DeploymentID int64                `json:"deployment_id" jsonschema:"Deployment ID,required"`
	Page         int64                `json:"page,omitempty" jsonschema:"Page number for pagination"`
	PerPage      int64                `json:"per_page,omitempty" jsonschema:"Number of items per page (max 100)"`
	State        string               `json:"state,omitempty" jsonschema:"Filter by state: opened, closed, merged, all"`
	OrderBy      string               `json:"order_by,omitempty" jsonschema:"Order by: created_at or updated_at"`
	Sort         string               `json:"sort,omitempty" jsonschema:"Sort order: asc or desc"`
}

// MergeRequestItem is a summary of a merge request associated with a deployment.
type MergeRequestItem struct {
	IID          int64  `json:"merge_request_iid"`
	Title        string `json:"title"`
	State        string `json:"state"`
	Author       string `json:"author"`
	SourceBranch string `json:"source_branch"`
	TargetBranch string `json:"target_branch"`
	WebURL       string `json:"web_url"`
	MergedAt     string `json:"merged_at,omitempty"`
}

// ListOutput is the output for listing deployment merge requests.
type ListOutput struct {
	toolutil.HintableOutput
	MergeRequests []MergeRequestItem        `json:"merge_requests"`
	Pagination    toolutil.PaginationOutput `json:"pagination"`
}

// List returns merge requests associated with a deployment.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.DeploymentID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("list_deployment_merge_requests", "deployment_id")
	}

	opts := &gl.ListMergeRequestsOptions{
		ListOptions: gl.ListOptions{
			Page:    input.Page,
			PerPage: input.PerPage,
		},
	}
	if input.State != "" {
		opts.State = new(input.State)
	}
	if input.OrderBy != "" {
		opts.OrderBy = new(input.OrderBy)
	}
	if input.Sort != "" {
		opts.Sort = new(input.Sort)
	}

	mrs, resp, err := client.GL().DeploymentMergeRequests.ListDeploymentMergeRequests(string(input.ProjectID), input.DeploymentID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list_deployment_merge_requests", err, http.StatusNotFound, "verify project_id and deployment_id with gitlab_deployment_list")
	}

	items := make([]MergeRequestItem, 0, len(mrs))
	for _, mr := range mrs {
		item := MergeRequestItem{
			IID:          mr.IID,
			Title:        mr.Title,
			State:        mr.State,
			SourceBranch: mr.SourceBranch,
			TargetBranch: mr.TargetBranch,
			WebURL:       mr.WebURL,
		}
		if mr.Author != nil {
			item.Author = mr.Author.Username
		}
		if mr.MergedAt != nil {
			item.MergedAt = mr.MergedAt.String()
		}
		items = append(items, item)
	}

	return ListOutput{
		MergeRequests: items,
		Pagination:    toolutil.PaginationFromResponse(resp),
	}, nil
}
