package deploymentmergerequests

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ListInput is the input for listing deployment merge requests. The deployment
// merge requests endpoint shares its query options with the project merge
// request list endpoint (gl.ListMergeRequestsOptions), so this mirrors the same
// filters for 1:1 fidelity with client-go.
type ListInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id"    jsonschema:"Project ID or URL-encoded path,required"`
	DeploymentID int64                `json:"deployment_id" jsonschema:"Deployment ID,required"`

	State          string  `json:"state,omitempty"           jsonschema:"Filter by state: opened, closed, merged, all"`
	OrderBy        string  `json:"order_by,omitempty"        jsonschema:"Order by: created_at or updated_at"`
	Sort           string  `json:"sort,omitempty"            jsonschema:"Sort order: asc or desc"`
	Approved       string  `json:"approved,omitempty"        jsonschema:"Filter by approval status: yes or no"`
	ApprovedByIDs  []int64 `json:"approved_by_ids,omitempty" jsonschema:"Filter by MRs approved by all listed user IDs"`
	ApproverIDs    []int64 `json:"approver_ids,omitempty"    jsonschema:"Filter by MRs with all listed users as eligible approvers"`
	AssigneeID     int64   `json:"assignee_id,omitempty"     jsonschema:"Filter by assignee user ID"`
	AuthorID       int64   `json:"author_id,omitempty"       jsonschema:"Filter by author user ID"`
	AuthorUsername string  `json:"author_username,omitempty" jsonschema:"Filter by author username"`
	CreatedAfter   string  `json:"created_after,omitempty"   jsonschema:"Return MRs created after date (ISO 8601, e.g. 2025-01-01T00:00:00Z)"`
	CreatedBefore  string  `json:"created_before,omitempty"  jsonschema:"Return MRs created before date (ISO 8601, e.g. 2025-12-31T23:59:59Z)"`
	Draft          *bool   `json:"draft,omitempty"           jsonschema:"Filter by draft status (true=only drafts, false=only non-drafts)"`
	In             string  `json:"in,omitempty"              jsonschema:"Modify the scope of the search attribute (title, description, or title,description)"`

	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// Output mirrors gl.MergeRequest (the full merge-request payload returned by the
// deployment merge requests endpoint). It embeds the gl.BasicMergeRequest field
// set and layers on the MergeRequest-only fields, surfacing every SDK field on
// its canonical json key per the 1:1 audit policy. Sub-objects are mirrored as
// local types (see shapes.go) to avoid importing the mergerequests package.
type Output struct {
	toolutil.HintableOutput
	ID                          int64                       `json:"id"`
	IID                         int64                       `json:"iid"`
	ProjectID                   int64                       `json:"project_id"`
	SourceProjectID             int64                       `json:"source_project_id,omitempty"`
	TargetProjectID             int64                       `json:"target_project_id,omitempty"`
	Title                       string                      `json:"title"`
	Description                 string                      `json:"description"`
	State                       string                      `json:"state"`
	Imported                    bool                        `json:"imported,omitempty"`
	ImportedFrom                string                      `json:"imported_from,omitempty"`
	SourceBranch                string                      `json:"source_branch"`
	TargetBranch                string                      `json:"target_branch"`
	WebURL                      string                      `json:"web_url"`
	DetailedMergeStatus         string                      `json:"detailed_merge_status,omitempty"`
	Draft                       bool                        `json:"draft"`
	WorkInProgress              bool                        `json:"work_in_progress,omitempty"`
	HasConflicts                bool                        `json:"has_conflicts"`
	BlockingDiscussionsResolved bool                        `json:"blocking_discussions_resolved"`
	Squash                      bool                        `json:"squash,omitempty"`
	SquashOnMerge               bool                        `json:"squash_on_merge,omitempty"`
	MergeWhenPipelineSucceeds   bool                        `json:"merge_when_pipeline_succeeds,omitempty"`
	ShouldRemoveSourceBranch    bool                        `json:"should_remove_source_branch,omitempty"`
	AllowMaintainerToPush       bool                        `json:"allow_maintainer_to_push,omitempty"`
	DiscussionLocked            bool                        `json:"discussion_locked"`
	RebaseInProgress            bool                        `json:"rebase_in_progress,omitempty"`
	Author                      *BasicUserOutput            `json:"author,omitempty"`
	Assignee                    *BasicUserOutput            `json:"assignee,omitempty"`
	MergeUser                   *BasicUserOutput            `json:"merge_user,omitempty"`
	MergedBy                    *BasicUserOutput            `json:"merged_by,omitempty"`
	ClosedBy                    *BasicUserOutput            `json:"closed_by,omitempty"`
	Assignees                   []*BasicUserOutput          `json:"assignees"`
	Reviewers                   []*BasicUserOutput          `json:"reviewers"`
	Labels                      []string                    `json:"labels"`
	LabelDetails                []*LabelDetailsOutput       `json:"label_details,omitempty"`
	Milestone                   *MilestoneOutput            `json:"milestone,omitempty"`
	References                  *ReferencesOutput           `json:"references,omitempty"`
	SHA                         string                      `json:"sha,omitempty"`
	MergeCommitSHA              string                      `json:"merge_commit_sha,omitempty"`
	MergeError                  string                      `json:"merge_error,omitempty"`
	ChangesCount                string                      `json:"changes_count,omitempty"`
	DivergedCommitsCount        int64                       `json:"diverged_commits_count,omitempty"`
	Upvotes                     int64                       `json:"upvotes,omitempty"`
	Downvotes                   int64                       `json:"downvotes,omitempty"`
	SquashCommitSHA             string                      `json:"squash_commit_sha,omitempty"`
	ForceRemoveSourceBranch     bool                        `json:"force_remove_source_branch,omitempty"`
	AllowCollaboration          bool                        `json:"allow_collaboration,omitempty"`
	MergeAfter                  string                      `json:"merge_after,omitempty"`
	TaskCompletionStatus        *TaskCompletionStatusOutput `json:"task_completion_status,omitempty"`
	TimeStats                   *TimeStatsOutput            `json:"time_stats,omitempty"`
	Subscribed                  bool                        `json:"subscribed,omitempty"`
	FirstContribution           bool                        `json:"first_contribution,omitempty"`
	User                        *MergeRequestUserOutput     `json:"user,omitempty"`
	DiffRefs                    *DiffRefsOutput             `json:"diff_refs,omitempty"`
	Pipeline                    *PipelineInfoOutput         `json:"pipeline,omitempty"`
	HeadPipeline                *PipelineOutput             `json:"head_pipeline,omitempty"`
	LatestBuildStartedAt        string                      `json:"latest_build_started_at,omitempty"`
	LatestBuildFinishedAt       string                      `json:"latest_build_finished_at,omitempty"`
	FirstDeployedToProductionAt string                      `json:"first_deployed_to_production_at,omitempty"`
	CreatedAt                   string                      `json:"created_at"`
	UpdatedAt                   string                      `json:"updated_at"`
	MergedAt                    string                      `json:"merged_at,omitempty"`
	ClosedAt                    string                      `json:"closed_at,omitempty"`
	PreparedAt                  string                      `json:"prepared_at,omitempty"`
	UserNotesCount              int64                       `json:"user_notes_count,omitempty"`
}

// DiffRefsOutput mirrors gl.MergeRequestDiffRefs (the diff_refs object with the
// base, head, and start SHAs of a merge request).
type DiffRefsOutput struct {
	BaseSHA  string `json:"base_sha"`
	HeadSHA  string `json:"head_sha"`
	StartSHA string `json:"start_sha"`
}

// ListOutput is the output for listing deployment merge requests.
type ListOutput struct {
	toolutil.HintableOutput
	MergeRequests []Output                  `json:"merge_requests"`
	Pagination    toolutil.PaginationOutput `json:"pagination"`
}

// List returns merge requests associated with a deployment.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.DeploymentID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("list_deployment_merge_requests", "deployment_id")
	}

	opts := buildListOptions(input)

	mrs, resp, err := client.GL().DeploymentMergeRequests.ListDeploymentMergeRequests(string(input.ProjectID), input.DeploymentID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list_deployment_merge_requests", err, http.StatusNotFound, "verify project_id and deployment_id with gitlab_deployment_list")
	}

	items := make([]Output, 0, len(mrs))
	for _, mr := range mrs {
		items = append(items, toOutput(mr))
	}

	return ListOutput{
		MergeRequests: items,
		Pagination:    toolutil.PaginationFromResponse(resp),
	}, nil
}

// buildListOptions maps ListInput onto the shared gl.ListMergeRequestsOptions,
// setting only the filters the caller supplied. Interface-typed ID filters are
// wrapped via the SDK constructors (gl.AssigneeID, gl.ApproverIDs); date filters
// are parsed with toolutil.ParseOptionalTime; offset and keyset pagination are
// applied via toolutil.ApplyListOptions.
func buildListOptions(input ListInput) *gl.ListMergeRequestsOptions {
	opts := &gl.ListMergeRequestsOptions{}

	if input.State != "" {
		opts.State = new(input.State)
	}
	if input.OrderBy != "" {
		opts.OrderBy = new(input.OrderBy)
	}
	if input.Sort != "" {
		opts.Sort = new(input.Sort)
	}
	if input.Approved != "" {
		opts.Approved = new(input.Approved)
	}
	if input.AuthorUsername != "" {
		opts.AuthorUsername = new(input.AuthorUsername)
	}
	if input.In != "" {
		opts.In = new(input.In)
	}
	if input.AuthorID != 0 {
		opts.AuthorID = new(input.AuthorID)
	}
	if input.AssigneeID != 0 {
		opts.AssigneeID = gl.AssigneeID(input.AssigneeID)
	}
	if len(input.ApproverIDs) > 0 {
		opts.ApproverIDs = gl.ApproverIDs(input.ApproverIDs)
	}
	if len(input.ApprovedByIDs) > 0 {
		opts.ApprovedByIDs = gl.ApproverIDs(input.ApprovedByIDs)
	}
	if input.Draft != nil {
		opts.Draft = input.Draft
	}
	if t := toolutil.ParseOptionalTime(input.CreatedAfter); t != nil {
		opts.CreatedAfter = t
	}
	if t := toolutil.ParseOptionalTime(input.CreatedBefore); t != nil {
		opts.CreatedBefore = t
	}

	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	return opts
}

// toOutput converts a gl.MergeRequest (the full deployment merge requests
// payload) to the local Output mirror, surfacing every SDK field on its
// canonical json key.
func toOutput(m *gl.MergeRequest) Output {
	out := Output{
		ID:                          m.ID,
		IID:                         m.IID,
		ProjectID:                   m.ProjectID,
		SourceProjectID:             m.SourceProjectID,
		TargetProjectID:             m.TargetProjectID,
		Title:                       m.Title,
		Description:                 m.Description,
		State:                       m.State,
		Imported:                    m.Imported,
		ImportedFrom:                m.ImportedFrom,
		SourceBranch:                m.SourceBranch,
		TargetBranch:                m.TargetBranch,
		WebURL:                      m.WebURL,
		DetailedMergeStatus:         m.DetailedMergeStatus,
		Draft:                       m.Draft,
		HasConflicts:                m.HasConflicts,
		BlockingDiscussionsResolved: m.BlockingDiscussionsResolved,
		Squash:                      m.Squash,
		SquashOnMerge:               m.SquashOnMerge,
		MergeWhenPipelineSucceeds:   m.MergeWhenPipelineSucceeds,
		ShouldRemoveSourceBranch:    m.ShouldRemoveSourceBranch,
		AllowMaintainerToPush:       m.AllowMaintainerToPush,
		DiscussionLocked:            m.DiscussionLocked,
		ForceRemoveSourceBranch:     m.ForceRemoveSourceBranch,
		AllowCollaboration:          m.AllowCollaboration,
		SHA:                         m.SHA,
		MergeCommitSHA:              m.MergeCommitSHA,
		SquashCommitSHA:             m.SquashCommitSHA,
		Upvotes:                     m.Upvotes,
		Downvotes:                   m.Downvotes,
		UserNotesCount:              m.UserNotesCount,
		Author:                      basicUserOutput(m.Author),
		Assignee:                    basicUserOutput(m.Assignee),
		MergeUser:                   basicUserOutput(m.MergeUser),
		ClosedBy:                    basicUserOutput(m.ClosedBy),
		MergedBy:                    basicUserOutput(m.MergedBy), //nolint:staticcheck // SA1019: mirrored for 1:1 SDK fidelity; use MergeUser.
		Assignees:                   basicUserOutputs(m.Assignees),
		Reviewers:                   basicUserOutputs(m.Reviewers),
		Labels:                      []string(m.Labels),
		LabelDetails:                labelDetailsOutputs(m.LabelDetails),
		Milestone:                   milestoneOutput(m.Milestone),
		References:                  referencesOutput(m.References),
		TaskCompletionStatus:        taskCompletionStatusOutput(m.TaskCompletionStatus),
		TimeStats:                   timeStatsPtr(m.TimeStats),
		MergeError:                  m.MergeError,
		ChangesCount:                m.ChangesCount,
		RebaseInProgress:            m.RebaseInProgress,
		DivergedCommitsCount:        m.DivergedCommitsCount,
		Subscribed:                  m.Subscribed,
		FirstContribution:           m.FirstContribution,
		WorkInProgress:              m.WorkInProgress, //nolint:staticcheck // SA1019: mirrored for 1:1 SDK fidelity; use Draft.
		User:                        mergeRequestUserOutput(m.User),
		Pipeline:                    pipelineInfoOutput(m.Pipeline),
		HeadPipeline:                pipelineOutput(m.HeadPipeline),
		MergeAfter:                  formatTimePtr(m.MergeAfter),
		LatestBuildStartedAt:        formatTimePtr(m.LatestBuildStartedAt),
		LatestBuildFinishedAt:       formatTimePtr(m.LatestBuildFinishedAt),
		FirstDeployedToProductionAt: formatTimePtr(m.FirstDeployedToProductionAt),
		CreatedAt:                   formatTimePtr(m.CreatedAt),
		UpdatedAt:                   formatTimePtr(m.UpdatedAt),
		MergedAt:                    formatTimePtr(m.MergedAt),
		ClosedAt:                    formatTimePtr(m.ClosedAt),
		PreparedAt:                  formatTimePtr(m.PreparedAt),
	}
	if out.Labels == nil {
		out.Labels = []string{}
	}
	if m.DiffRefs.BaseSha != "" || m.DiffRefs.HeadSha != "" || m.DiffRefs.StartSha != "" {
		out.DiffRefs = &DiffRefsOutput{
			BaseSHA:  m.DiffRefs.BaseSha,
			HeadSHA:  m.DiffRefs.HeadSha,
			StartSHA: m.DiffRefs.StartSha,
		}
	}
	return out
}
