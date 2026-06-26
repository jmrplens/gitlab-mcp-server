package mergetrains

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ListProjectInput defines parameters for listing project merge trains. It
// mirrors gl.ListMergeTrainsOptions (scope, sort) and the embedded gl.ListOptions
// (order_by, sort, pagination, page_token), exposing both offset and keyset
// pagination via the embedded PaginationInput and KeysetPaginationInput.
type ListProjectInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Scope     string               `json:"scope,omitempty" jsonschema:"Filter by scope: active, complete"`
	Sort      string               `json:"sort,omitempty" jsonschema:"Sort order: asc or desc"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Column to order results by (keyset pagination)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListBranchInput defines parameters for listing MRs in a merge train for a
// specific branch. It mirrors gl.ListMergeTrainsOptions (scope, sort) and the
// embedded gl.ListOptions (order_by, sort, pagination, page_token), exposing
// both offset and keyset pagination.
type ListBranchInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	TargetBranch string               `json:"target_branch" jsonschema:"Target branch name,required"`
	Scope        string               `json:"scope,omitempty" jsonschema:"Filter by scope: active, complete"`
	Sort         string               `json:"sort,omitempty" jsonschema:"Sort order: asc or desc"`
	OrderBy      string               `json:"order_by,omitempty" jsonschema:"Column to order results by (keyset pagination)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// GetInput defines parameters for getting a merge request on a merge train.
type GetInput struct {
	ProjectID      toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MergeRequestID int64                `json:"merge_request_iid" jsonschema:"Merge request internal ID,required"`
}

// AddInput defines parameters for adding a merge request to a merge train. It
// mirrors gl.AddMergeRequestToMergeTrainOptions (auto_merge, sha, squash, and
// the deprecated when_pipeline_succeeds).
type AddInput struct {
	ProjectID            toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MergeRequestID       int64                `json:"merge_request_iid" jsonschema:"Merge request internal ID,required"`
	AutoMerge            bool                 `json:"auto_merge,omitempty" jsonschema:"Enable auto-merge when pipeline succeeds"`
	SHA                  string               `json:"sha,omitempty" jsonschema:"Head SHA of the merge request to verify"`
	Squash               bool                 `json:"squash,omitempty" jsonschema:"Squash commits when merging"`
	WhenPipelineSucceeds bool                 `json:"when_pipeline_succeeds,omitempty" jsonschema:"Deprecated in 17.11; use auto_merge instead. Merge only when the pipeline succeeds"`
}

// MergeRequestOutput mirrors gl.MergeTrainMergeRequest, the merge request
// embedded on the merge-train "merge_request" key, surfacing every SDK field.
type MergeRequestOutput struct {
	ID          int64  `json:"id"`
	IID         int64  `json:"iid"`
	ProjectID   int64  `json:"project_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	State       string `json:"state"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
	WebURL      string `json:"web_url,omitempty"`
}

// Output represents a merge train entry, mirroring gl.MergeTrain with full
// nested objects for the merge_request, user, and pipeline sub-objects.
type Output struct {
	toolutil.HintableOutput
	ID           int64                     `json:"id"`
	MergeRequest MergeRequestOutput        `json:"merge_request"`
	User         *toolutil.BasicUserOutput `json:"user,omitempty"`
	Pipeline     *toolutil.PipelineOutput  `json:"pipeline,omitempty"`
	TargetBranch string                    `json:"target_branch"`
	Status       string                    `json:"status"`
	Duration     int64                     `json:"duration"`
	CreatedAt    string                    `json:"created_at,omitempty"`
	UpdatedAt    string                    `json:"updated_at,omitempty"`
	MergedAt     string                    `json:"merged_at,omitempty"`
}

// ListOutput wraps a list of merge train entries.
type ListOutput struct {
	toolutil.HintableOutput
	Trains     []Output                  `json:"trains"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// toOutput converts a [gl.MergeTrain] into the package's [Output], mirroring
// the embedded merge request, user, and pipeline sub-objects as full nested
// shapes and formatting every timestamp via [toolutil.DateTimeFormat].
func toOutput(mt *gl.MergeTrain) Output {
	if mt == nil {
		return Output{}
	}
	out := Output{
		ID:           mt.ID,
		User:         toolutil.NewBasicUserOutput(mt.User),
		Pipeline:     toolutil.NewPipelineOutput(mt.Pipeline),
		TargetBranch: mt.TargetBranch,
		Status:       mt.Status,
		Duration:     mt.Duration,
	}
	if mt.MergeRequest != nil {
		out.MergeRequest = MergeRequestOutput{
			ID:          mt.MergeRequest.ID,
			IID:         mt.MergeRequest.IID,
			ProjectID:   mt.MergeRequest.ProjectID,
			Title:       mt.MergeRequest.Title,
			Description: mt.MergeRequest.Description,
			State:       mt.MergeRequest.State,
			WebURL:      mt.MergeRequest.WebURL,
		}
		if mt.MergeRequest.CreatedAt != nil {
			out.MergeRequest.CreatedAt = mt.MergeRequest.CreatedAt.Format(toolutil.DateTimeFormat)
		}
		if mt.MergeRequest.UpdatedAt != nil {
			out.MergeRequest.UpdatedAt = mt.MergeRequest.UpdatedAt.Format(toolutil.DateTimeFormat)
		}
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

// ListProjectMergeTrains lists all merge trains in a project via the
// GitLab Merge trains API (GET /projects/:id/merge_trains). Merge
// trains require a Premium license.
func ListProjectMergeTrains(ctx context.Context, client *gitlabclient.Client, input ListProjectInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	opts := &gl.ListMergeTrainsOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
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

// ListMergeRequestInMergeTrain lists the merge requests currently
// sitting on the merge train for a specific target branch via the
// GitLab Merge trains API (GET /projects/:id/merge_trains/:target_branch).
func ListMergeRequestInMergeTrain(ctx context.Context, client *gitlabclient.Client, input ListBranchInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.TargetBranch == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("target_branch")
	}
	opts := &gl.ListMergeTrainsOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
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

// GetMergeRequestOnMergeTrain retrieves the merge train status for a
// single merge request via the GitLab Merge trains API
// (GET /projects/:id/merge_trains/merge_requests/:merge_request_iid).
// Returns the active train entry or a 404 when the MR is not on a
// merge train.
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

// AddMergeRequestToMergeTrain adds a merge request to a project's
// merge train via the GitLab Merge trains API
// (POST /projects/:id/merge_trains/merge_requests/:merge_request_iid).
// Optional AutoMerge/SHA/Squash parameters forward to the underlying
// client-go options. Requires the MR to be approved with a passing
// pipeline; merge trains require a Premium license.
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
	if input.WhenPipelineSucceeds {
		opts.WhenPipelineSucceeds = new(true) //nolint:staticcheck // SA1019: mirrored for 1:1 SDK fidelity; use auto_merge.
	}
	trains, resp, err := client.GL().MergeTrains.AddMergeRequestToMergeTrain(string(input.ProjectID), input.MergeRequestID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("gitlab_add_merge_request_to_merge_train", err, http.StatusBadRequest, "verify the MR is approved and pipeline passed \u2014 merge trains require Premium license")
	}
	return toListOutput(trains, resp), nil
}

// toListOutput converts a slice of [gl.MergeTrain] entries into a
// paginated [ListOutput] using the supplied GitLab [gl.Response] for
// pagination metadata.
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
