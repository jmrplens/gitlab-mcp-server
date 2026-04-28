// Package mergetrains implements MCP tool handlers for GitLab merge trains.
package mergetrains

import (
	"context"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// ListProjectInput defines parameters for listing project merge trains.
type ListProjectInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Scope     string               `json:"scope,omitempty" jsonschema:"Filter by scope: active, complete"`
	Sort      string               `json:"sort,omitempty" jsonschema:"Sort order: asc or desc"`
	toolutil.PaginationInput
}

// ListBranchInput defines parameters for listing MRs in a merge train for a specific branch.
type ListBranchInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	TargetBranch string               `json:"target_branch" jsonschema:"Target branch name,required"`
	Scope        string               `json:"scope,omitempty" jsonschema:"Filter by scope: active, complete"`
	Sort         string               `json:"sort,omitempty" jsonschema:"Sort order: asc or desc"`
	toolutil.PaginationInput
}

// GetInput defines parameters for getting a merge request on a merge train.
type GetInput struct {
	ProjectID      toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MergeRequestID int64                `json:"merge_request_iid" jsonschema:"Merge request internal ID,required"`
}

// AddInput defines parameters for adding a merge request to a merge train.
type AddInput struct {
	ProjectID      toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MergeRequestID int64                `json:"merge_request_iid" jsonschema:"Merge request internal ID,required"`
	AutoMerge      bool                 `json:"auto_merge,omitempty" jsonschema:"Enable auto-merge when pipeline succeeds"`
	SHA            string               `json:"sha,omitempty" jsonschema:"Head SHA of the merge request to verify"`
	Squash         bool                 `json:"squash,omitempty" jsonschema:"Squash commits when merging"`
}

// MergeRequestOutput represents the MR embedded in a merge train entry.
type MergeRequestOutput struct {
	ID        int64  `json:"id"`
	IID       int64  `json:"iid"`
	ProjectID int64  `json:"project_id"`
	Title     string `json:"title"`
	State     string `json:"state"`
	WebURL    string `json:"web_url,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// Output represents a merge train entry.
type Output struct {
	toolutil.HintableOutput
	ID           int64              `json:"id"`
	MergeRequest MergeRequestOutput `json:"merge_request"`
	User         string             `json:"user,omitempty"`
	PipelineID   int64              `json:"pipeline_id,omitempty"`
	TargetBranch string             `json:"target_branch"`
	Status       string             `json:"status"`
	Duration     int64              `json:"duration"`
	CreatedAt    string             `json:"created_at,omitempty"`
	UpdatedAt    string             `json:"updated_at,omitempty"`
	MergedAt     string             `json:"merged_at,omitempty"`
}

// ListOutput wraps a list of merge train entries.
type ListOutput struct {
	toolutil.HintableOutput
	Trains     []Output                  `json:"trains"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

func toOutput(mt *gl.MergeTrain) Output {
	if mt == nil {
		return Output{}
	}
	out := Output{
		ID:           mt.ID,
		TargetBranch: mt.TargetBranch,
		Status:       mt.Status,
		Duration:     mt.Duration,
	}
	if mt.MergeRequest != nil {
		out.MergeRequest = MergeRequestOutput{
			ID:        mt.MergeRequest.ID,
			IID:       mt.MergeRequest.IID,
			ProjectID: mt.MergeRequest.ProjectID,
			Title:     mt.MergeRequest.Title,
			State:     mt.MergeRequest.State,
			WebURL:    mt.MergeRequest.WebURL,
		}
		if mt.MergeRequest.CreatedAt != nil {
			out.MergeRequest.CreatedAt = mt.MergeRequest.CreatedAt.Format(toolutil.DateTimeFormat)
		}
		if mt.MergeRequest.UpdatedAt != nil {
			out.MergeRequest.UpdatedAt = mt.MergeRequest.UpdatedAt.Format(toolutil.DateTimeFormat)
		}
	}
	if mt.User != nil {
		out.User = mt.User.Username
	}
	if mt.Pipeline != nil {
		out.PipelineID = mt.Pipeline.ID
	}
	if mt.CreatedAt != nil {
		out.CreatedAt = mt.CreatedAt.Format(toolutil.DateTimeFormat)
	}
	if mt.UpdatedAt != nil {
		out.UpdatedAt = mt.UpdatedAt.Format(toolutil.DateTimeFormat)
	}
	if mt.MergedAt != nil {
		out.MergedAt = mt.MergedAt.Format(toolutil.DateTimeFormat)
	}
	return out
}

// ListProjectMergeTrains lists all merge trains for a project.
func ListProjectMergeTrains(ctx context.Context, client *gitlabclient.Client, input ListProjectInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	opts := &gl.ListMergeTrainsOptions{
		ListOptions: gl.ListOptions{Page: int64(input.Page), PerPage: int64(input.PerPage)},
	}
	if input.Scope != "" {
		opts.Scope = new(input.Scope)
	}
	if input.Sort != "" {
		opts.Sort = new(input.Sort)
	}
	trains, resp, err := client.GL().MergeTrains.ListProjectMergeTrains(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("gitlab_list_project_merge_trains", err, http.StatusNotFound, "verify project_id with gitlab_project_get \u2014 merge trains require Premium license")
	}
	return toListOutput(trains, resp), nil
}

// ListMergeRequestInMergeTrain lists merge requests in a merge train for a branch.
func ListMergeRequestInMergeTrain(ctx context.Context, client *gitlabclient.Client, input ListBranchInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.TargetBranch == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("target_branch")
	}
	opts := &gl.ListMergeTrainsOptions{
		ListOptions: gl.ListOptions{Page: int64(input.Page), PerPage: int64(input.PerPage)},
	}
	if input.Scope != "" {
		opts.Scope = new(input.Scope)
	}
	if input.Sort != "" {
		opts.Sort = new(input.Sort)
	}
	trains, resp, err := client.GL().MergeTrains.ListMergeRequestInMergeTrain(string(input.ProjectID), input.TargetBranch, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("gitlab_list_merge_request_in_merge_train", err, http.StatusNotFound, "verify project_id and target_branch \u2014 merge trains require Premium license")
	}
	return toListOutput(trains, resp), nil
}

// GetMergeRequestOnMergeTrain gets the merge train status for a specific MR.
func GetMergeRequestOnMergeTrain(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.MergeRequestID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("gitlab_get_merge_request_on_merge_train", "merge_request_iid")
	}
	train, _, err := client.GL().MergeTrains.GetMergeRequestOnAMergeTrain(string(input.ProjectID), input.MergeRequestID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("gitlab_get_merge_request_on_merge_train", err, http.StatusNotFound, "verify project_id and merge_request_iid \u2014 the MR must be on a merge train")
	}
	return toOutput(train), nil
}

// AddMergeRequestToMergeTrain adds a merge request to a merge train.
func AddMergeRequestToMergeTrain(ctx context.Context, client *gitlabclient.Client, input AddInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.MergeRequestID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("gitlab_add_merge_request_to_merge_train", "merge_request_iid")
	}
	opts := &gl.AddMergeRequestToMergeTrainOptions{}
	if input.AutoMerge {
		opts.AutoMerge = new(true)
	}
	if input.SHA != "" {
		opts.SHA = new(input.SHA)
	}
	if input.Squash {
		opts.Squash = new(true)
	}
	trains, resp, err := client.GL().MergeTrains.AddMergeRequestToMergeTrain(string(input.ProjectID), input.MergeRequestID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("gitlab_add_merge_request_to_merge_train", err, http.StatusBadRequest, "verify the MR is approved and pipeline passed \u2014 merge trains require Premium license")
	}
	return toListOutput(trains, resp), nil
}

func toListOutput(trains []*gl.MergeTrain, resp *gl.Response) ListOutput {
	out := ListOutput{
		Trains:     make([]Output, 0, len(trains)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for _, t := range trains {
		out.Trains = append(out.Trains, toOutput(t))
	}
	return out
}
