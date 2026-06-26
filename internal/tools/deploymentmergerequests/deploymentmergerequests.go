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

	State                  string   `json:"state,omitempty"           jsonschema:"Filter by state: opened, closed, merged, all"`
	OrderBy                string   `json:"order_by,omitempty"        jsonschema:"Order by: created_at or updated_at"`
	Sort                   string   `json:"sort,omitempty"            jsonschema:"Sort order: asc or desc"`
	Approved               string   `json:"approved,omitempty"        jsonschema:"Filter by approval status: yes or no"`
	ApprovedByIDs          []int64  `json:"approved_by_ids,omitempty" jsonschema:"Filter by MRs approved by all listed user IDs"`
	ApproverIDs            []int64  `json:"approver_ids,omitempty"    jsonschema:"Filter by MRs with all listed users as eligible approvers"`
	AssigneeID             int64    `json:"assignee_id,omitempty"     jsonschema:"Filter by assignee user ID"`
	AuthorID               int64    `json:"author_id,omitempty"       jsonschema:"Filter by author user ID"`
	AuthorUsername         string   `json:"author_username,omitempty" jsonschema:"Filter by author username"`
	NotAuthorUsername      string   `json:"not_author_username,omitempty" jsonschema:"Exclude MRs authored by this username"`
	ReviewerID             int64    `json:"reviewer_id,omitempty"         jsonschema:"Filter by reviewer user ID"`
	ReviewerUsername       string   `json:"reviewer_username,omitempty"   jsonschema:"Filter by reviewer username"`
	Labels                 []string `json:"labels,omitempty"          jsonschema:"Label names to filter by"`
	NotLabels              []string `json:"not_labels,omitempty"      jsonschema:"Label names to exclude"`
	Milestone              string   `json:"milestone,omitempty"       jsonschema:"Milestone title to filter by"`
	Scope                  string   `json:"scope,omitempty"           jsonschema:"Filter by scope (created_by_me, assigned_to_me, all)"`
	Search                 string   `json:"search,omitempty"          jsonschema:"Search in title and description"`
	SourceBranch           string   `json:"source_branch,omitempty"   jsonschema:"Filter by source branch name"`
	TargetBranch           string   `json:"target_branch,omitempty"   jsonschema:"Filter by target branch name"`
	MyReactionEmoji        string   `json:"my_reaction_emoji,omitempty"   jsonschema:"Filter by MRs the caller reacted to with this emoji (e.g. thumbsup)"`
	View                   string   `json:"view,omitempty"                jsonschema:"Set to 'simple' to return only basic MR fields"`
	WIP                    string   `json:"wip,omitempty"                 jsonschema:"Filter by draft/WIP status: 'yes' for draft MRs, 'no' for non-draft"`
	WithLabelsDetails      *bool    `json:"with_labels_details,omitempty"       jsonschema:"Include full label details (color, description) in the response"`
	WithMergeStatusRecheck *bool    `json:"with_merge_status_recheck,omitempty" jsonschema:"Asynchronously recalculate each MR's merge_status before returning"`
	CreatedAfter           string   `json:"created_after,omitempty"   jsonschema:"Return MRs created after date (ISO 8601, e.g. 2025-01-01T00:00:00Z)"`
	CreatedBefore          string   `json:"created_before,omitempty"  jsonschema:"Return MRs created before date (ISO 8601, e.g. 2025-12-31T23:59:59Z)"`
	UpdatedAfter           string   `json:"updated_after,omitempty"   jsonschema:"Return MRs updated after date (ISO 8601, e.g. 2025-01-01T00:00:00Z)"`
	UpdatedBefore          string   `json:"updated_before,omitempty"  jsonschema:"Return MRs updated before date (ISO 8601, e.g. 2025-12-31T23:59:59Z)"`
	Draft                  *bool    `json:"draft,omitempty"           jsonschema:"Filter by draft status (true=only drafts, false=only non-drafts)"`
	In                     string   `json:"in,omitempty"              jsonschema:"Modify the scope of the search attribute (title, description, or title,description)"`

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

	applyStringFilters(input, opts)
	applyLabelAndBoolFilters(input, opts)
	applyIDFilters(input, opts)
	applyDateFilters(input, opts)

	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	return opts
}

// applyStringFilters sets the scalar string filters that are forwarded verbatim
// to the GitLab API, mapping each non-empty ListInput field onto its
// gl.ListMergeRequestsOptions *string counterpart.
func applyStringFilters(input ListInput, opts *gl.ListMergeRequestsOptions) {
	filters := []struct {
		value  string
		target **string
	}{
		{input.State, &opts.State},
		{input.OrderBy, &opts.OrderBy},
		{input.Sort, &opts.Sort},
		{input.Approved, &opts.Approved},
		{input.AuthorUsername, &opts.AuthorUsername},
		{input.NotAuthorUsername, &opts.NotAuthorUsername},
		{input.ReviewerUsername, &opts.ReviewerUsername},
		{input.Milestone, &opts.Milestone},
		{input.Scope, &opts.Scope},
		{input.Search, &opts.Search},
		{input.SourceBranch, &opts.SourceBranch},
		{input.TargetBranch, &opts.TargetBranch},
		{input.MyReactionEmoji, &opts.MyReactionEmoji},
		{input.View, &opts.View},
		{input.WIP, &opts.WIP},
		{input.In, &opts.In},
	}
	for _, f := range filters {
		if f.value != "" {
			v := f.value
			*f.target = &v
		}
	}
}

// applyLabelAndBoolFilters sets the label-name list filters and the boolean
// label-detail/merge-recheck toggles.
func applyLabelAndBoolFilters(input ListInput, opts *gl.ListMergeRequestsOptions) {
	if labels := labelOptions(input.Labels); labels != nil {
		opts.Labels = labels
	}
	if labels := labelOptions(input.NotLabels); labels != nil {
		opts.NotLabels = labels
	}
	if input.WithLabelsDetails != nil {
		opts.WithLabelsDetails = input.WithLabelsDetails
	}
	if input.WithMergeStatusRecheck != nil {
		opts.WithMergeStatusRecheck = input.WithMergeStatusRecheck
	}
	if input.Draft != nil {
		opts.Draft = input.Draft
	}
}

// applyIDFilters sets the user-ID filters, wrapping interface-typed values via
// the SDK constructors (gl.AssigneeID, gl.ReviewerID, gl.ApproverIDs).
func applyIDFilters(input ListInput, opts *gl.ListMergeRequestsOptions) {
	if input.AuthorID != 0 {
		opts.AuthorID = new(input.AuthorID)
	}
	if input.AssigneeID != 0 {
		opts.AssigneeID = gl.AssigneeID(input.AssigneeID)
	}
	if input.ReviewerID != 0 {
		opts.ReviewerID = gl.ReviewerID(input.ReviewerID)
	}
	if len(input.ApproverIDs) > 0 {
		opts.ApproverIDs = gl.ApproverIDs(input.ApproverIDs)
	}
	if len(input.ApprovedByIDs) > 0 {
		opts.ApprovedByIDs = gl.ApproverIDs(input.ApprovedByIDs)
	}
}

// applyDateFilters parses the ISO 8601 created/updated bounds with
// toolutil.ParseOptionalTime and sets them when present.
func applyDateFilters(input ListInput, opts *gl.ListMergeRequestsOptions) {
	if t := toolutil.ParseOptionalTime(input.CreatedAfter); t != nil {
		opts.CreatedAfter = t
	}
	if t := toolutil.ParseOptionalTime(input.CreatedBefore); t != nil {
		opts.CreatedBefore = t
	}
	if t := toolutil.ParseOptionalTime(input.UpdatedAfter); t != nil {
		opts.UpdatedAfter = t
	}
	if t := toolutil.ParseOptionalTime(input.UpdatedBefore); t != nil {
		opts.UpdatedBefore = t
	}
}

// labelOptions converts a label-name slice into a *gl.LabelOptions, returning
// nil when the slice is empty so the filter is omitted.
func labelOptions(values []string) *gl.LabelOptions {
	if len(values) == 0 {
		return nil
	}
	labels := gl.LabelOptions(values)
	return &labels
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
		MergeAfter:                  toolutil.FormatTimePtr(m.MergeAfter),
		LatestBuildStartedAt:        toolutil.FormatTimePtr(m.LatestBuildStartedAt),
		LatestBuildFinishedAt:       toolutil.FormatTimePtr(m.LatestBuildFinishedAt),
		FirstDeployedToProductionAt: toolutil.FormatTimePtr(m.FirstDeployedToProductionAt),
		CreatedAt:                   toolutil.FormatTimePtr(m.CreatedAt),
		UpdatedAt:                   toolutil.FormatTimePtr(m.UpdatedAt),
		MergedAt:                    toolutil.FormatTimePtr(m.MergedAt),
		ClosedAt:                    toolutil.FormatTimePtr(m.ClosedAt),
		PreparedAt:                  toolutil.FormatTimePtr(m.PreparedAt),
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
