package deploymentmergerequests

import (
	"context"
	"fmt"
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

	State                  string                     `json:"state,omitempty"           jsonschema:"Filter by state: opened, closed, locked, merged, or all (default all)"`
	OrderBy                string                     `json:"order_by,omitempty"        jsonschema:"Order by: created_at, updated_at, merged_at, label_priority, priority, milestone_due, popularity, or title (default created_at)"`
	Sort                   string                     `json:"sort,omitempty"            jsonschema:"Sort order: asc or desc"`
	Approved               string                     `json:"approved,omitempty"        jsonschema:"Filter by approval status: yes or no"`
	ApprovedByIDs          toolutil.ApproverIDsFilter `json:"approved_by_ids,omitempty" jsonschema:"Filter by MRs approved by all listed user IDs. Accepts user IDs, or exactly one of \"Any\" (approved by someone) or \"None\" (unapproved)"`
	ApprovedByUsernames    []string                   `json:"approved_by_usernames,omitempty" jsonschema:"Filter by MRs approved by all listed usernames"`
	ApproverIDs            toolutil.ApproverIDsFilter `json:"approver_ids,omitempty"    jsonschema:"Filter by MRs with all listed users as eligible approvers. Accepts user IDs, or exactly one of \"Any\" (has approvers) or \"None\" (has none)"`
	AssigneeID             int64                      `json:"assignee_id,omitempty"     jsonschema:"Filter by assignee user ID"`
	AuthorID               int64                      `json:"author_id,omitempty"       jsonschema:"Filter by author user ID"`
	AuthorUsername         string                     `json:"author_username,omitempty" jsonschema:"Filter by author username"`
	NotAuthorUsername      string                     `json:"not_author_username,omitempty" jsonschema:"Exclude MRs authored by this username"`
	ReviewerID             int64                      `json:"reviewer_id,omitempty"         jsonschema:"Filter by reviewer user ID"`
	ReviewerUsername       string                     `json:"reviewer_username,omitempty"   jsonschema:"Filter by reviewer username"`
	Labels                 []string                   `json:"labels,omitempty"          jsonschema:"Label names to filter by"`
	NotLabels              []string                   `json:"not_labels,omitempty"      jsonschema:"Label names to exclude"`
	Milestone              string                     `json:"milestone,omitempty"       jsonschema:"Milestone title to filter by"`
	Scope                  string                     `json:"scope,omitempty"           jsonschema:"Filter by scope: created_by_me, assigned_to_me, reviews_for_me, or all"`
	Search                 string                     `json:"search,omitempty"          jsonschema:"Search in title and description"`
	SourceBranch           string                     `json:"source_branch,omitempty"   jsonschema:"Filter by source branch name"`
	TargetBranch           string                     `json:"target_branch,omitempty"   jsonschema:"Filter by target branch name"`
	MyReactionEmoji        string                     `json:"my_reaction_emoji,omitempty"   jsonschema:"Filter by MRs the caller reacted to with this emoji (e.g. thumbsup)"`
	View                   string                     `json:"view,omitempty"                jsonschema:"Set to 'simple' to return only basic MR fields"`
	WIP                    string                     `json:"wip,omitempty"                 jsonschema:"Filter by draft/WIP status: 'yes' for draft MRs, 'no' for non-draft"`
	WithLabelsDetails      *bool                      `json:"with_labels_details,omitempty"       jsonschema:"Include full label details (color, description) in the response"`
	WithMergeStatusRecheck *bool                      `json:"with_merge_status_recheck,omitempty" jsonschema:"Asynchronously recalculate each MR's merge_status before returning"`
	CreatedAfter           string                     `json:"created_after,omitempty"   jsonschema:"Return MRs created after date (ISO 8601, e.g. 2025-01-01T00:00:00Z)"`
	CreatedBefore          string                     `json:"created_before,omitempty"  jsonschema:"Return MRs created before date (ISO 8601, e.g. 2025-12-31T23:59:59Z)"`
	UpdatedAfter           string                     `json:"updated_after,omitempty"   jsonschema:"Return MRs updated after date (ISO 8601, e.g. 2025-01-01T00:00:00Z)"`
	UpdatedBefore          string                     `json:"updated_before,omitempty"  jsonschema:"Return MRs updated before date (ISO 8601, e.g. 2025-12-31T23:59:59Z)"`
	Draft                  *bool                      `json:"draft,omitempty"           jsonschema:"Filter by draft status (true=only drafts, false=only non-drafts)"`
	NonArchived            *bool                      `json:"non_archived,omitempty"    jsonschema:"Return merge requests from non-archived projects only. Default is true"`
	In                     string                     `json:"in,omitempty"              jsonschema:"Fields the search parameter matches. Accepts title, description, or both joined with a comma. Default is title,description"`

	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// Output is the canonical merge-request output shape shared with the
// mergerequests domain. The single authoritative definition lives in
// toolutil.MergeRequestOutput; this alias keeps all existing call sites
// within this package unchanged (ADR-0004).
type Output = toolutil.MergeRequestOutput

// DiffRefsOutput mirrors gl.MergeRequestDiffRefs (the diff_refs object with the
// base, head, and start SHAs of a merge request). The canonical definition
// lives in toolutil.DiffRefsOutput; this alias preserves all existing
// references within the package unchanged.
type DiffRefsOutput = toolutil.DiffRefsOutput

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

	opts, err := buildListOptions(input)
	if err != nil {
		return ListOutput{}, err
	}

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
func buildListOptions(input ListInput) (*gl.ListMergeRequestsOptions, error) {
	opts := &gl.ListMergeRequestsOptions{}

	applyStringFilters(input, opts)
	applyLabelAndBoolFilters(input, opts)
	if err := applyIDFilters(input, opts); err != nil {
		return nil, err
	}
	applyDateFilters(input, opts)

	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	return opts, nil
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
		{input.Approved, &opts.Approved}, //nolint:staticcheck // SA1019: mirrored for 1:1 SDK fidelity; prefer approved_by_ids
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
	if input.NonArchived != nil {
		opts.NonArchived = input.NonArchived
	}
}

// applyIDFilters sets the user-ID filters, wrapping interface-typed values via
// the SDK constructors (gl.AssigneeID, gl.ReviewerID, gl.ApproverIDs).
func applyIDFilters(input ListInput, opts *gl.ListMergeRequestsOptions) error {
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
		converted, err := input.ApproverIDs.ApproverIDsValue()
		if err != nil {
			return fmt.Errorf("approver_ids: %w", err)
		}
		opts.ApproverIDs = converted
	}
	if len(input.ApprovedByIDs) > 0 {
		converted, err := input.ApprovedByIDs.ApproverIDsValue()
		if err != nil {
			return fmt.Errorf("approved_by_ids: %w", err)
		}
		opts.ApprovedByIDs = converted
	}
	if len(input.ApprovedByUsernames) > 0 {
		opts.ApprovedByUsernames = &input.ApprovedByUsernames
	}
	return nil
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
		Author:                      toolutil.NewBasicUserOutput(m.Author),
		Assignee:                    toolutil.NewBasicUserOutput(m.Assignee),
		MergeUser:                   toolutil.NewBasicUserOutput(m.MergeUser),
		ClosedBy:                    toolutil.NewBasicUserOutput(m.ClosedBy),
		MergedBy:                    toolutil.NewBasicUserOutput(m.MergedBy), //nolint:staticcheck // SA1019: mirrored for 1:1 SDK fidelity; use MergeUser.
		Assignees:                   toolutil.NewBasicUserOutputs(m.Assignees),
		Reviewers:                   toolutil.NewBasicUserOutputs(m.Reviewers),
		Labels:                      []string(m.Labels),
		LabelDetails:                toolutil.NewLabelDetailsOutputs(m.LabelDetails),
		Milestone:                   mrMilestoneOutputPtr(m.Milestone),
		References:                  toolutil.NewReferencesOutput(m.References),
		TaskCompletionStatus:        toolutil.NewTaskCompletionStatusOutput(m.TaskCompletionStatus),
		TimeStats:                   timeStatsPtr(m.TimeStats),
		MergeError:                  m.MergeError,
		ChangesCount:                m.ChangesCount,
		RebaseInProgress:            m.RebaseInProgress,
		DivergedCommitsCount:        m.DivergedCommitsCount,
		Subscribed:                  m.Subscribed,
		FirstContribution:           m.FirstContribution,
		WorkInProgress:              m.WorkInProgress, //nolint:staticcheck // SA1019: mirrored for 1:1 SDK fidelity; use Draft.
		User:                        mergeRequestUserOutputPtr(m.User),
		Pipeline:                    toolutil.NewPipelineInfoOutput(m.Pipeline),
		HeadPipeline:                toolutil.NewPipelineOutput(m.HeadPipeline),
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
