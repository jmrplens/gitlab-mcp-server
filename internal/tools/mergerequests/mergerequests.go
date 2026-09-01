package mergerequests

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/pipelines"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// hintVerifyMR is the 404 hint shared by MR tools.
const hintVerifyMR = "verify project_id and merge_request_iid with gitlab_mr_get"

// CreateInput defines parameters for creating a merge request.
type CreateInput struct {
	// Basic metadata
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	SourceBranch string               `json:"source_branch" jsonschema:"Source branch name,required"`
	TargetBranch string               `json:"target_branch" jsonschema:"Target branch name. If not specified by the user use the project default branch from gitlab_project_get (do NOT assume main),required"`
	Title        string               `json:"title" jsonschema:"Merge request title,required"`
	Description  string               `json:"description,omitempty" jsonschema:"Merge request description (Markdown supported)"`

	// Assignment and tracking
	AssigneeID  int64    `json:"assignee_id,omitempty" jsonschema:"Single user ID to assign (use assignee_ids for multiple)"`
	AssigneeIDs []int64  `json:"assignee_ids,omitempty" jsonschema:"User IDs to assign"`
	ReviewerIDs []int64  `json:"reviewer_ids,omitempty" jsonschema:"User IDs to add as reviewers"`
	Labels      []string `json:"labels,omitempty" jsonschema:"Label names to apply"`
	MilestoneID int64    `json:"milestone_id,omitempty" jsonschema:"Milestone ID to associate with the merge request"`

	// Merge behavior
	RemoveSourceBranch *bool `json:"remove_source_branch,omitempty" jsonschema:"Delete source branch after merge. Only set if explicitly requested by the user. Omit to preserve repository defaults"`
	Squash             *bool `json:"squash,omitempty" jsonschema:"Squash commits on merge. Only set if explicitly requested by the user. Omit to preserve repository defaults"`
	AllowCollaboration *bool `json:"allow_collaboration,omitempty" jsonschema:"Allow commits from upstream members who can merge to target branch"`

	// Cross-project
	TargetProjectID int64 `json:"target_project_id,omitempty" jsonschema:"Target project ID (for cross-project/fork MRs)"`

	// Deprecated approvals control (use the Merge Request Approvals API instead)
	ApprovalsBeforeMerge int64 `json:"approvals_before_merge,omitempty" tier:"premium" jsonschema:"Number of approvals required before this MR can be merged (deprecated. Use the approval rules API)"`
}

// Output is the canonical merge-request output shape. The authoritative
// definition lives in toolutil.MergeRequestOutput; this alias keeps all
// existing call sites within this package unchanged (ADR-0004).
type Output = toolutil.MergeRequestOutput

// DiffRefsOutput is the diff refs object (base, head, start SHAs) of a merge
// request. The canonical definition lives in toolutil.DiffRefsOutput; this
// alias preserves all existing references within the package unchanged.
type DiffRefsOutput = toolutil.DiffRefsOutput

// GetInput defines parameters for retrieving a merge request.
type GetInput struct {
	ProjectID                   toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID                       int64                `json:"merge_request_iid"     jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	IncludeDivergedCommitsCount *bool                `json:"include_diverged_commits_count,omitempty" jsonschema:"Include the count of commits the source branch is behind the target branch (diverged_commits_count)"`
	IncludeRebaseInProgress     *bool                `json:"include_rebase_in_progress,omitempty"     jsonschema:"Include whether a rebase is currently in progress (rebase_in_progress)"`
	RenderHTML                  *bool                `json:"render_html,omitempty"                    jsonschema:"Return the title and description rendered to HTML"`
}

// ListInput defines filters for listing merge requests.
type ListInput struct {
	ProjectID              toolutil.StringOrInt       `json:"project_id"              jsonschema:"Project ID or URL-encoded path,required"`
	State                  string                     `json:"state,omitempty"         jsonschema:"Filter by state (opened, closed, merged, all)"`
	Labels                 []string                   `json:"labels,omitempty"        jsonschema:"Label names to filter by"`
	NotLabels              []string                   `json:"not_labels,omitempty"    jsonschema:"Label names to exclude"`
	Milestone              string                     `json:"milestone,omitempty"     jsonschema:"Milestone title to filter by"`
	Scope                  string                     `json:"scope,omitempty"         jsonschema:"Filter by scope (created_by_me, assigned_to_me, all)"`
	Search                 string                     `json:"search,omitempty"        jsonschema:"Search in title and description"`
	SourceBranch           string                     `json:"source_branch,omitempty" jsonschema:"Filter by source branch name"`
	TargetBranch           string                     `json:"target_branch,omitempty" jsonschema:"Filter by target branch name"`
	AuthorUsername         string                     `json:"author_username,omitempty"     jsonschema:"Filter by author username"`
	NotAuthorUsername      string                     `json:"not_author_username,omitempty" jsonschema:"Exclude MRs authored by this username"`
	ReviewerUsername       string                     `json:"reviewer_username,omitempty"   jsonschema:"Filter by reviewer username"`
	Environment            string                     `json:"environment,omitempty"         jsonschema:"Filter by deployment environment name"`
	MyReactionEmoji        string                     `json:"my_reaction_emoji,omitempty"   jsonschema:"Filter by MRs the caller reacted to with this emoji (e.g. thumbsup)"`
	View                   string                     `json:"view,omitempty"                jsonschema:"Set to 'simple' to return only basic MR fields"`
	WIP                    string                     `json:"wip,omitempty"                 jsonschema:"Filter by draft/WIP status: 'yes' for draft MRs, 'no' for non-draft"`
	AuthorID               int64                      `json:"author_id,omitempty"           jsonschema:"Filter by author user ID"`
	AssigneeID             int64                      `json:"assignee_id,omitempty"         jsonschema:"Filter by assignee user ID"`
	ReviewerID             int64                      `json:"reviewer_id,omitempty"         jsonschema:"Filter by reviewer user ID"`
	ApproverIDs            toolutil.ApproverIDsFilter `json:"approver_ids,omitempty"        jsonschema:"Filter by MRs with all listed users as eligible approvers. Accepts user IDs, or exactly one of \"Any\" (has approvers) or \"None\" (has none)"`
	ApprovedByIDs          toolutil.ApproverIDsFilter `json:"approved_by_ids,omitempty"     jsonschema:"Filter by MRs approved by all listed user IDs. Accepts user IDs, or exactly one of \"Any\" (approved by someone) or \"None\" (unapproved)"`
	ApprovedByUsernames    []string                   `json:"approved_by_usernames,omitempty" jsonschema:"Filter by MRs approved by all listed usernames"`
	WithLabelsDetails      *bool                      `json:"with_labels_details,omitempty"        jsonschema:"Include full label details (color, description) in the response"`
	WithMergeStatusRecheck *bool                      `json:"with_merge_status_recheck,omitempty"  jsonschema:"Asynchronously recalculate each MR's merge_status before returning"`
	Draft                  *bool                      `json:"draft,omitempty"         jsonschema:"Filter by draft status (true=only drafts, false=only non-drafts)"`
	NonArchived            *bool                      `json:"non_archived,omitempty"  jsonschema:"Return merge requests from non-archived projects only. Default is true"`
	IIDs                   []int64                    `json:"iids,omitempty"          jsonschema:"Filter by merge request internal IDs"`
	CreatedAfter           string                     `json:"created_after,omitempty"  jsonschema:"Return MRs created after date (ISO 8601 format, e.g. 2025-01-01T00:00:00Z)"`
	CreatedBefore          string                     `json:"created_before,omitempty" jsonschema:"Return MRs created before date (ISO 8601 format, e.g. 2025-12-31T23:59:59Z)"`
	UpdatedAfter           string                     `json:"updated_after,omitempty"  jsonschema:"Return MRs updated after date (ISO 8601 format, e.g. 2025-01-01T00:00:00Z)"`
	UpdatedBefore          string                     `json:"updated_before,omitempty" jsonschema:"Return MRs updated before date (ISO 8601 format, e.g. 2025-12-31T23:59:59Z)"`
	DeployedAfter          string                     `json:"deployed_after,omitempty"  jsonschema:"Return MRs deployed after date (ISO 8601 format)"`
	DeployedBefore         string                     `json:"deployed_before,omitempty" jsonschema:"Return MRs deployed before date (ISO 8601 format)"`
	OrderBy                string                     `json:"order_by,omitempty"      jsonschema:"Order by field (created_at, updated_at, title)"`
	Sort                   string                     `json:"sort,omitempty"          jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListOutput holds a paginated list of merge requests.
type ListOutput struct {
	toolutil.HintableOutput
	MergeRequests []Output                  `json:"merge_requests"`
	Pagination    toolutil.PaginationOutput `json:"pagination"`
}

type mergeRequestListFilters struct {
	State               string
	Labels              []string
	NotLabels           []string
	Milestone           string
	Scope               string
	Search              string
	SourceBranch        string
	TargetBranch        string
	AuthorUsername      string
	NotAuthorUsername   string
	ReviewerUsername    string
	Approved            string
	In                  string
	MyReactionEmoji     string
	View                string
	WIP                 string
	Environment         string
	AuthorID            int64
	AssigneeID          int64
	ReviewerID          int64
	ApproverIDs         toolutil.ApproverIDsFilter
	ApprovedByIDs       toolutil.ApproverIDsFilter
	ApprovedByUsernames []string
	WithLabelsDetails   *bool
	WithMergeRecheck    *bool
	Draft               *bool
	NonArchived         *bool
	CreatedAfter        string
	CreatedBefore       string
	UpdatedAfter        string
	UpdatedBefore       string
	DeployedAfter       string
	DeployedBefore      string
	OrderBy             string
	Sort                string
	Page                int
	PerPage             int
	Keyset              toolutil.KeysetPaginationInput
}

type mergeRequestListTarget struct {
	state               func(*string)
	labels              func(*gl.LabelOptions)
	notLabels           func(*gl.LabelOptions)
	milestone           func(*string)
	scope               func(*string)
	search              func(*string)
	sourceBranch        func(*string)
	targetBranch        func(*string)
	authorUsername      func(*string)
	notAuthorUsername   func(*string)
	reviewerUsername    func(*string)
	approved            func(*string)
	in                  func(*string)
	myReactionEmoji     func(*string)
	view                func(*string)
	wip                 func(*string)
	environment         func(*string)
	authorID            func(int64)
	assigneeID          func(int64)
	reviewerID          func(int64)
	approverIDs         func(*gl.ApproverIDsValue)
	approvedByIDs       func(*gl.ApproverIDsValue)
	approvedByUsernames func([]string)
	withLabelsDetails   func(*bool)
	withMergeRecheck    func(*bool)
	draft               func(*bool)
	nonArchived         func(*bool)
	createdAfter        func(*time.Time)
	createdBefore       func(*time.Time)
	updatedAfter        func(*time.Time)
	updatedBefore       func(*time.Time)
	deployedAfter       func(*time.Time)
	deployedBefore      func(*time.Time)
	orderBy             func(*string)
	sort                func(*string)
	listOptions         *gl.ListOptions
}

func mergeRequestListOutput(mrs []*gl.BasicMergeRequest, resp *gl.Response) ListOutput {
	out := make([]Output, len(mrs))
	for i, m := range mrs {
		out[i] = BasicToOutput(m)
	}
	return ListOutput{MergeRequests: out, Pagination: toolutil.PaginationFromResponse(resp)}
}

// labelOptions converts a label-name slice into a *gl.LabelOptions, or nil when
// empty.
func labelOptions(values []string) *gl.LabelOptions {
	if len(values) == 0 {
		return nil
	}
	labels := gl.LabelOptions(values)
	return &labels
}

func applyMergeRequestListFilters(input mergeRequestListFilters, target mergeRequestListTarget) error {
	setString(input.State, target.state)
	setString(input.Milestone, target.milestone)
	setString(input.Scope, target.scope)
	setString(input.Search, target.search)
	setString(input.SourceBranch, target.sourceBranch)
	setString(input.TargetBranch, target.targetBranch)
	setString(input.AuthorUsername, target.authorUsername)
	setString(input.NotAuthorUsername, target.notAuthorUsername)
	setString(input.ReviewerUsername, target.reviewerUsername)
	setString(input.Approved, target.approved)
	setString(input.In, target.in)
	setString(input.MyReactionEmoji, target.myReactionEmoji)
	setString(input.View, target.view)
	setString(input.WIP, target.wip)
	setString(input.Environment, target.environment)
	setString(input.OrderBy, target.orderBy)
	setString(input.Sort, target.sort)
	setInt64(input.AuthorID, target.authorID)
	setInt64(input.AssigneeID, target.assigneeID)
	setInt64(input.ReviewerID, target.reviewerID)
	if err := setApproverIDs(input.ApproverIDs, target.approverIDs); err != nil {
		return fmt.Errorf("approver_ids: %w", err)
	}
	if err := setApproverIDs(input.ApprovedByIDs, target.approvedByIDs); err != nil {
		return fmt.Errorf("approved_by_ids: %w", err)
	}
	setStringSlice(input.ApprovedByUsernames, target.approvedByUsernames)
	if input.WithLabelsDetails != nil && target.withLabelsDetails != nil {
		target.withLabelsDetails(input.WithLabelsDetails)
	}
	if input.WithMergeRecheck != nil && target.withMergeRecheck != nil {
		target.withMergeRecheck(input.WithMergeRecheck)
	}
	if labels := labelOptions(input.Labels); labels != nil && target.labels != nil {
		target.labels(labels)
	}
	if labels := labelOptions(input.NotLabels); labels != nil && target.notLabels != nil {
		target.notLabels(labels)
	}
	if input.Draft != nil && target.draft != nil {
		target.draft(input.Draft)
	}
	if input.NonArchived != nil && target.nonArchived != nil {
		target.nonArchived(input.NonArchived)
	}
	setTime(toolutil.ParseOptionalTime(input.CreatedAfter), target.createdAfter)
	setTime(toolutil.ParseOptionalTime(input.CreatedBefore), target.createdBefore)
	setTime(toolutil.ParseOptionalTime(input.UpdatedAfter), target.updatedAfter)
	setTime(toolutil.ParseOptionalTime(input.UpdatedBefore), target.updatedBefore)
	setTime(toolutil.ParseOptionalTime(input.DeployedAfter), target.deployedAfter)
	setTime(toolutil.ParseOptionalTime(input.DeployedBefore), target.deployedBefore)
	if target.listOptions != nil {
		toolutil.ApplyListOptions(target.listOptions, toolutil.PaginationInput{Page: input.Page, PerPage: input.PerPage}, input.Keyset)
	}
	return nil
}

// setApproverIDs converts an approver filter to its SDK value and hands it to
// the setter, leaving the option untouched when the filter is empty.
func setApproverIDs(value toolutil.ApproverIDsFilter, setter func(*gl.ApproverIDsValue)) error {
	if len(value) == 0 || setter == nil {
		return nil
	}
	converted, err := value.ApproverIDsValue()
	if err != nil {
		return err
	}
	setter(converted)
	return nil
}

func setInt64(value int64, setter func(int64)) {
	if value != 0 && setter != nil {
		setter(value)
	}
}

func setStringSlice(value []string, setter func([]string)) {
	if len(value) > 0 && setter != nil {
		setter(value)
	}
}

func setString(value string, setter func(*string)) {
	if value != "" && setter != nil {
		setter(&value)
	}
}

func setTime(value *time.Time, setter func(*time.Time)) {
	if value != nil && setter != nil {
		setter(value)
	}
}

// UpdateInput defines parameters for updating a merge request.
type UpdateInput struct {
	ProjectID          toolutil.StringOrInt `json:"project_id"                    jsonschema:"Project ID or URL-encoded path,required"`
	MRIID              int64                `json:"merge_request_iid"                        jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	Title              string               `json:"title,omitempty"               jsonschema:"New title"`
	Description        string               `json:"description,omitempty"         jsonschema:"New description"`
	TargetBranch       string               `json:"target_branch,omitempty"       jsonschema:"New target branch"`
	AssigneeID         int64                `json:"assignee_id,omitempty"          jsonschema:"Single user ID to assign (use assignee_ids for multiple)"`
	AssigneeIDs        []int64              `json:"assignee_ids,omitempty"         jsonschema:"User IDs to assign as merge request assignees"`
	ReviewerIDs        []int64              `json:"reviewer_ids,omitempty"         jsonschema:"User IDs to add as reviewers"`
	Labels             []string             `json:"labels,omitempty"               jsonschema:"Label names to replace all labels on the merge request"`
	AddLabels          []string             `json:"add_labels,omitempty"          jsonschema:"Label names to add without removing existing"`
	RemoveLabels       []string             `json:"remove_labels,omitempty"       jsonschema:"Label names to remove"`
	MilestoneID        int64                `json:"milestone_id,omitempty"        jsonschema:"Milestone ID (0 to unset)"`
	RemoveSourceBranch *bool                `json:"remove_source_branch,omitempty" jsonschema:"Delete source branch after merge. Only set if explicitly requested"`
	Squash             *bool                `json:"squash,omitempty"              jsonschema:"Squash commits on merge. Only set if explicitly requested"`
	DiscussionLocked   *bool                `json:"discussion_locked,omitempty"   jsonschema:"Lock discussions on the merge request"`
	AllowCollaboration *bool                `json:"allow_collaboration,omitempty" jsonschema:"Allow commits from upstream members who can merge to target branch"`
	StateEvent         string               `json:"state_event,omitempty"         jsonschema:"State transition (close, reopen)"`
}

// MergeInput defines parameters for merging a merge request.
type MergeInput struct {
	ProjectID                 toolutil.StringOrInt `json:"project_id"                              jsonschema:"Project ID or URL-encoded path,required"`
	MRIID                     int64                `json:"merge_request_iid"                                  jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	MergeCommitMessage        string               `json:"merge_commit_message,omitempty"           jsonschema:"Custom merge commit message"`
	Squash                    *bool                `json:"squash,omitempty"                         jsonschema:"Squash commits on merge. Only set if explicitly requested by the user. Omit to preserve repository defaults"`
	ShouldRemoveSourceBranch  *bool                `json:"should_remove_source_branch,omitempty"   jsonschema:"Delete source branch after merge. Only set if explicitly requested by the user. Omit to preserve repository defaults"`
	AutoMerge                 *bool                `json:"auto_merge,omitempty"                     jsonschema:"Automatically merge when pipeline succeeds (auto-merge)"`
	MergeWhenPipelineSucceeds *bool                `json:"merge_when_pipeline_succeeds,omitempty"   jsonschema:"Deprecated alias for auto_merge: merge when the pipeline succeeds. Prefer auto_merge"`
	SHA                       string               `json:"sha,omitempty"                            jsonschema:"Head SHA of the merge request — merge only if HEAD matches (safety check)"`
	SquashCommitMessage       string               `json:"squash_commit_message,omitempty"          jsonschema:"Custom squash commit message (used when squash is enabled)"`
}

// ApproveInput defines parameters for approving a merge request.
type ApproveInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"merge_request_iid"     jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	SHA       string               `json:"sha,omitempty"         jsonschema:"Head SHA of the merge request — approve only if HEAD matches (safety check, applies to approve only)"`
}

// ApproveOutput holds the approval state after approve/unapprove.
type ApproveOutput struct {
	toolutil.HintableOutput
	ApprovalsRequired int  `json:"approvals_required"`
	ApprovedBy        int  `json:"approved_by_count"`
	Approved          bool `json:"approved"`
}

// ToOutput converts a GitLab API [gl.MergeRequest] (the get endpoint payload)
// to the MCP tool output format. It first projects the embedded
// BasicMergeRequest, then layers on the MergeRequest-only fields.
func ToOutput(m *gl.MergeRequest) Output {
	out := BasicToOutput(&m.BasicMergeRequest)
	out.MergeError = m.MergeError
	out.ChangesCount = m.ChangesCount
	out.RebaseInProgress = m.RebaseInProgress
	out.DivergedCommitsCount = m.DivergedCommitsCount
	out.Subscribed = m.Subscribed
	out.FirstContribution = m.FirstContribution
	out.WorkInProgress = m.WorkInProgress //nolint:staticcheck // SA1019: mirrored for 1:1 SDK fidelity; use Draft.
	out.User = mergeRequestUserOutputPtr(m.User)
	if m.DiffRefs.BaseSha != "" || m.DiffRefs.HeadSha != "" || m.DiffRefs.StartSha != "" {
		out.DiffRefs = &DiffRefsOutput{
			BaseSHA:  m.DiffRefs.BaseSha,
			HeadSHA:  m.DiffRefs.HeadSha,
			StartSHA: m.DiffRefs.StartSha,
		}
	}
	out.Pipeline = toolutil.NewPipelineInfoOutput(m.Pipeline)
	out.HeadPipeline = toolutil.NewPipelineOutput(m.HeadPipeline)
	out.LatestBuildStartedAt = toolutil.FormatTimePtr(m.LatestBuildStartedAt)
	out.LatestBuildFinishedAt = toolutil.FormatTimePtr(m.LatestBuildFinishedAt)
	out.FirstDeployedToProductionAt = toolutil.FormatTimePtr(m.FirstDeployedToProductionAt)
	return out
}

// BasicToOutput converts a GitLab API [gl.BasicMergeRequest] to the MCP tool
// output format. BasicMergeRequest is used in list endpoints that return a
// lighter payload than the full MergeRequest object.
func BasicToOutput(m *gl.BasicMergeRequest) Output {
	out := Output{
		ID:                          m.ID,
		IID:                         m.IID,
		ProjectID:                   m.ProjectID,
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
		SHA:                         m.SHA,
		MergeCommitSHA:              m.MergeCommitSHA,
		UserNotesCount:              m.UserNotesCount,
	}
	populatePeople(&out, m)
	return out
}

// populatePeople extracts author, assignees, reviewers, labels, and metadata
// from a BasicMergeRequest into the Output, mirroring the SDK sub-objects on
// their canonical json keys.
func populatePeople(out *Output, m *gl.BasicMergeRequest) {
	out.SourceProjectID = m.SourceProjectID
	out.TargetProjectID = m.TargetProjectID
	out.DiscussionLocked = m.DiscussionLocked
	out.MergeWhenPipelineSucceeds = m.MergeWhenPipelineSucceeds
	out.ShouldRemoveSourceBranch = m.ShouldRemoveSourceBranch
	out.ForceRemoveSourceBranch = m.ForceRemoveSourceBranch
	out.AllowCollaboration = m.AllowCollaboration
	out.AllowMaintainerToPush = m.AllowMaintainerToPush
	out.SquashOnMerge = m.SquashOnMerge
	out.SquashCommitSHA = m.SquashCommitSHA
	out.Upvotes = m.Upvotes
	out.Downvotes = m.Downvotes
	out.Author = toolutil.NewBasicUserOutput(m.Author)
	out.Assignee = toolutil.NewBasicUserOutput(m.Assignee)
	out.MergeUser = toolutil.NewBasicUserOutput(m.MergeUser)
	out.ClosedBy = toolutil.NewBasicUserOutput(m.ClosedBy)
	out.MergedBy = toolutil.NewBasicUserOutput(m.MergedBy) //nolint:staticcheck // SA1019: mirrored for 1:1 SDK fidelity; use MergeUser.
	out.Assignees = toolutil.NewBasicUserOutputs(m.Assignees)
	if out.Assignees == nil {
		out.Assignees = []*toolutil.BasicUserOutput{}
	}
	out.Reviewers = toolutil.NewBasicUserOutputs(m.Reviewers)
	if out.Reviewers == nil {
		out.Reviewers = []*toolutil.BasicUserOutput{}
	}
	out.Labels = []string(m.Labels)
	if out.Labels == nil {
		out.Labels = []string{}
	}
	out.LabelDetails = toolutil.NewLabelDetailsOutputs(m.LabelDetails)
	out.Milestone = mrMilestoneOutputPtr(m.Milestone)
	out.TaskCompletionStatus = toolutil.NewTaskCompletionStatusOutput(m.TaskCompletionStatus)
	out.TimeStats = timeStatsPtr(m.TimeStats)
	populateTimestamps(out, m)
}

// populateTimestamps extracts timestamps and references from a
// BasicMergeRequest into the Output.
func populateTimestamps(out *Output, m *gl.BasicMergeRequest) {
	out.MergeAfter = toolutil.FormatTimePtr(m.MergeAfter)
	out.CreatedAt = toolutil.FormatTimePtr(m.CreatedAt)
	out.UpdatedAt = toolutil.FormatTimePtr(m.UpdatedAt)
	out.MergedAt = toolutil.FormatTimePtr(m.MergedAt)
	out.ClosedAt = toolutil.FormatTimePtr(m.ClosedAt)
	out.PreparedAt = toolutil.FormatTimePtr(m.PreparedAt)
	out.References = toolutil.NewReferencesOutput(m.References)
}

// mrMilestoneOutputPtr converts a gl.Milestone pointer into the
// canonical MRMilestoneOutput. It exists as a package-local wrapper so
// the call site reads naturally next to the other conversion helpers
// in this file (e.g. timeStatsPtr) without scattering toolutil.*
// references throughout the body.
func mrMilestoneOutputPtr(m *gl.Milestone) *toolutil.MRMilestoneOutput {
	out := toolutil.NewMRMilestoneOutputs([]*gl.Milestone{m})
	if len(out) == 0 {
		return nil
	}
	return out[0]
}

// Create creates a new merge request in the specified GitLab project.
// It maps optional fields (description, assignees, reviewers, labels, squash)
// only when provided. Returns the created merge request details.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("mrCreate: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	opts := &gl.CreateMergeRequestOptions{
		SourceBranch: new(input.SourceBranch),
		TargetBranch: new(input.TargetBranch),
		Title:        new(input.Title),
	}
	if input.Description != "" {
		opts.Description = new(toolutil.NormalizeText(input.Description))
	}
	if input.AssigneeID != 0 {
		opts.AssigneeID = new(input.AssigneeID)
	}
	if len(input.AssigneeIDs) > 0 {
		opts.AssigneeIDs = &input.AssigneeIDs
	}
	if len(input.ReviewerIDs) > 0 {
		opts.ReviewerIDs = &input.ReviewerIDs
	}
	if labels := labelOptions(input.Labels); labels != nil {
		opts.Labels = labels
	}
	if input.RemoveSourceBranch != nil {
		opts.RemoveSourceBranch = input.RemoveSourceBranch
	}
	if input.Squash != nil {
		opts.Squash = input.Squash
	}
	if input.MilestoneID != 0 {
		opts.MilestoneID = new(input.MilestoneID)
	}
	if input.AllowCollaboration != nil {
		opts.AllowCollaboration = input.AllowCollaboration
	}
	if input.TargetProjectID != 0 {
		opts.TargetProjectID = new(input.TargetProjectID)
	}
	if input.ApprovalsBeforeMerge != 0 {
		opts.ApprovalsBeforeMerge = new(input.ApprovalsBeforeMerge) //nolint:staticcheck // SA1019: mirrored for 1:1 SDK fidelity; no replacement on CreateMergeRequestOptions.
	}
	mr, _, err := client.GL().MergeRequests.CreateMergeRequest(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusConflict) {
			return Output{}, toolutil.WrapErrWithHint("mrCreate", err,
				"an MR for this source branch may already exist. Use gitlab_mr_list with source_branch filter to find it")
		}
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return Output{}, toolutil.WrapErrWithHint("mrCreate", err,
				"verify both source_branch and target_branch exist. Use gitlab_branch_list to check")
		}
		return Output{}, toolutil.WrapErrWithMessage("mrCreate", err)
	}
	return ToOutput(mr), nil
}

// Get retrieves a single merge request by its internal ID within a project.
// Returns an error if the merge request does not exist.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("mrGet: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.MRIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("mrGet", "merge_request_iid")
	}
	opts := &gl.GetMergeRequestsOptions{}
	if input.IncludeDivergedCommitsCount != nil {
		opts.IncludeDivergedCommitsCount = input.IncludeDivergedCommitsCount
	}
	if input.IncludeRebaseInProgress != nil {
		opts.IncludeRebaseInProgress = input.IncludeRebaseInProgress
	}
	if input.RenderHTML != nil {
		opts.RenderHTML = input.RenderHTML
	}
	mr, _, err := client.GL().MergeRequests.GetMergeRequest(string(input.ProjectID), input.MRIID, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return Output{}, toolutil.WrapErrWithHint("mrGet", err,
				"verify project_id and merge_request_iid (the project-scoped IID, not the global merge_request_id); use gitlab_mr_list to find existing MRs")
		}
		return Output{}, toolutil.WrapErrWithMessage("mrGet", err)
	}
	return ToOutput(mr), nil
}

// List returns a paginated list of merge requests for a project.
// Results can be filtered by state and search terms, and ordered by
// the specified field and direction.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("mrList: project_id is required. Use gitlab_project_list to find the project ID first, then pass it as project_id")
	}
	opts, err := buildListOptions(input)
	if err != nil {
		return ListOutput{}, err
	}
	mrs, resp, err := client.GL().MergeRequests.ListProjectMergeRequests(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("mrList", err, http.StatusNotFound,
			"verify the project exists with gitlab_project_get")
	}
	return mergeRequestListOutput(mrs, resp), nil
}

// buildListOptions maps ListInput fields to the GitLab API list options,
// applying only non-zero values so that unset filters are omitted.
func buildListOptions(input ListInput) (*gl.ListProjectMergeRequestsOptions, error) {
	opts := &gl.ListProjectMergeRequestsOptions{}
	if err := applyMergeRequestListFilters(projectMRListFilters(input), projectMergeRequestListTarget(opts)); err != nil {
		return nil, err
	}
	if len(input.IIDs) > 0 {
		opts.IIDs = &input.IIDs
	}
	return opts, nil
}

func projectMRListFilters(input ListInput) mergeRequestListFilters {
	return mergeRequestListFilters{
		State: input.State, Labels: input.Labels, NotLabels: input.NotLabels, Milestone: input.Milestone,
		Scope: input.Scope, Search: input.Search, SourceBranch: input.SourceBranch, TargetBranch: input.TargetBranch,
		AuthorUsername: input.AuthorUsername, NotAuthorUsername: input.NotAuthorUsername, ReviewerUsername: input.ReviewerUsername,
		MyReactionEmoji: input.MyReactionEmoji, View: input.View, WIP: input.WIP, Environment: input.Environment,
		AuthorID: input.AuthorID, AssigneeID: input.AssigneeID, ReviewerID: input.ReviewerID,
		ApproverIDs: input.ApproverIDs, ApprovedByIDs: input.ApprovedByIDs, ApprovedByUsernames: input.ApprovedByUsernames,
		WithLabelsDetails: input.WithLabelsDetails, WithMergeRecheck: input.WithMergeStatusRecheck,
		Draft: input.Draft, NonArchived: input.NonArchived, CreatedAfter: input.CreatedAfter,
		CreatedBefore: input.CreatedBefore, UpdatedAfter: input.UpdatedAfter, UpdatedBefore: input.UpdatedBefore,
		DeployedAfter: input.DeployedAfter, DeployedBefore: input.DeployedBefore,
		OrderBy: input.OrderBy, Sort: input.Sort, Page: input.Page, PerPage: input.PerPage, Keyset: input.KeysetPaginationInput,
	}
}

func projectMergeRequestListTarget(opts *gl.ListProjectMergeRequestsOptions) mergeRequestListTarget {
	return newMergeRequestListTarget(mergeRequestListTargetFields{
		state: &opts.State, labels: &opts.Labels, notLabels: &opts.NotLabels, milestone: &opts.Milestone, scope: &opts.Scope,
		search: &opts.Search, sourceBranch: &opts.SourceBranch, targetBranch: &opts.TargetBranch, authorUsername: &opts.AuthorUsername,
		notAuthorUsername: &opts.NotAuthorUsername, reviewerUsername: &opts.ReviewerUsername, myReactionEmoji: &opts.MyReactionEmoji,
		view: &opts.View, wip: &opts.WIP, environment: &opts.Environment, authorID: &opts.AuthorID,
		assigneeID: &opts.AssigneeID, reviewerID: &opts.ReviewerID, approverIDs: &opts.ApproverIDs, approvedByIDs: &opts.ApprovedByIDs, approvedByUsernames: &opts.ApprovedByUsernames,
		withLabelsDetails: &opts.WithLabelsDetails, withMergeRecheck: &opts.WithMergeStatusRecheck,
		draft: &opts.Draft, nonArchived: &opts.NonArchived, createdAfter: &opts.CreatedAfter, createdBefore: &opts.CreatedBefore,
		updatedAfter: &opts.UpdatedAfter, updatedBefore: &opts.UpdatedBefore, deployedAfter: &opts.DeployedAfter, deployedBefore: &opts.DeployedBefore,
		orderBy: &opts.OrderBy, sort: &opts.Sort, listOptions: &opts.ListOptions,
	})
}

// buildUpdateOpts maps UpdateInput fields to the GitLab API update options,
// applying only non-zero values so that unset fields are omitted.
func buildUpdateOpts(input UpdateInput) *gl.UpdateMergeRequestOptions {
	opts := &gl.UpdateMergeRequestOptions{}
	if input.Title != "" {
		opts.Title = new(input.Title)
	}
	if input.Description != "" {
		opts.Description = new(toolutil.NormalizeText(input.Description))
	}
	if input.TargetBranch != "" {
		opts.TargetBranch = new(input.TargetBranch)
	}
	if input.StateEvent != "" {
		opts.StateEvent = new(input.StateEvent)
	}
	if input.AssigneeID != 0 {
		opts.AssigneeID = new(input.AssigneeID)
	}
	if len(input.AssigneeIDs) > 0 {
		opts.AssigneeIDs = &input.AssigneeIDs
	}
	if len(input.ReviewerIDs) > 0 {
		opts.ReviewerIDs = &input.ReviewerIDs
	}
	if labels := labelOptions(input.Labels); labels != nil {
		opts.Labels = labels
	}
	if labels := labelOptions(input.AddLabels); labels != nil {
		opts.AddLabels = labels
	}
	if labels := labelOptions(input.RemoveLabels); labels != nil {
		opts.RemoveLabels = labels
	}
	if input.MilestoneID > 0 {
		opts.MilestoneID = new(input.MilestoneID)
	}
	if input.RemoveSourceBranch != nil {
		opts.RemoveSourceBranch = input.RemoveSourceBranch
	}
	if input.Squash != nil {
		opts.Squash = input.Squash
	}
	if input.DiscussionLocked != nil {
		opts.DiscussionLocked = input.DiscussionLocked
	}
	if input.AllowCollaboration != nil {
		opts.AllowCollaboration = input.AllowCollaboration
	}
	return opts
}

// Update modifies an existing merge request. Only non-zero fields in the
// input are applied, allowing partial updates such as changing the title,
// description, target branch, assignees, reviewers, or triggering a state
// transition (close/reopen).
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("mrUpdate: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.MRIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("mrUpdate", "merge_request_iid")
	}
	opts := buildUpdateOpts(input)
	mr, _, err := client.GL().MergeRequests.UpdateMergeRequest(string(input.ProjectID), input.MRIID, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return Output{}, toolutil.WrapErrWithHint("mrUpdate", err,
				"verify project_id and merge_request_iid. Use gitlab_mr_list to check available MRs")
		}
		return Output{}, toolutil.WrapErrWithMessage("mrUpdate", err)
	}
	return ToOutput(mr), nil
}

// Merge accepts (merges) a merge request. When squash or
// should_remove_source_branch are not explicitly set by the caller, the
// function pre-fetches the MR to detect project-level requirements
// (squash_on_merge, force_remove_source_branch) and applies them
// automatically, avoiding merge rejections from enforced settings.
func Merge(ctx context.Context, client *gitlabclient.Client, input MergeInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("mrMerge: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.MRIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("mrMerge", "merge_request_iid")
	}

	// Always pre-fetch the MR to detect enforced project merge settings.
	// Projects can require squash (squash_option=always) or force source
	// branch deletion — the API rejects merge requests that violate these
	// constraints. LLMs tend to explicitly send squash=false even when
	// omitting it would be correct, so we override when the MR indicates
	// an enforced setting.
	prefetched, _, fetchErr := client.GL().MergeRequests.GetMergeRequest(string(input.ProjectID), input.MRIID, nil, gl.WithContext(ctx))
	if fetchErr == nil {
		if prefetched.SquashOnMerge {
			input.Squash = &prefetched.SquashOnMerge
		}
		if prefetched.ForceRemoveSourceBranch {
			input.ShouldRemoveSourceBranch = &prefetched.ForceRemoveSourceBranch
		}
	}

	opts := &gl.AcceptMergeRequestOptions{}
	if input.MergeCommitMessage != "" {
		opts.MergeCommitMessage = new(input.MergeCommitMessage)
	}
	if input.Squash != nil {
		opts.Squash = input.Squash
	}
	if input.ShouldRemoveSourceBranch != nil {
		opts.ShouldRemoveSourceBranch = input.ShouldRemoveSourceBranch
	}
	if input.AutoMerge != nil {
		opts.AutoMerge = input.AutoMerge
	}
	if input.MergeWhenPipelineSucceeds != nil {
		opts.MergeWhenPipelineSucceeds = input.MergeWhenPipelineSucceeds //nolint:staticcheck // SA1019: deprecated alias mirrored for 1:1 SDK fidelity; prefer auto_merge.
	}
	if input.SHA != "" {
		opts.SHA = new(input.SHA)
	}
	if input.SquashCommitMessage != "" {
		opts.SquashCommitMessage = new(input.SquashCommitMessage)
	}
	mr, resp, err := client.GL().MergeRequests.AcceptMergeRequest(string(input.ProjectID), input.MRIID, opts, gl.WithContext(ctx))
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusMethodNotAllowed && fetchErr == nil {
			return Output{}, diagnoseMergeBlocker(input.MRIID, prefetched, err)
		}
		return Output{}, toolutil.WrapErrWithMessage("mrMerge", err)
	}
	return ToOutput(mr), nil
}

// Approve adds an approval to the specified merge request and returns the
// updated approval state including required count, approved-by count, and
// overall approved status.
func Approve(ctx context.Context, client *gitlabclient.Client, input ApproveInput) (ApproveOutput, error) {
	if err := ctx.Err(); err != nil {
		return ApproveOutput{}, err
	}
	if input.ProjectID == "" {
		return ApproveOutput{}, errors.New("mrApprove: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.MRIID <= 0 {
		return ApproveOutput{}, toolutil.ErrRequiredInt64("mrApprove", "merge_request_iid")
	}
	approveOpts := &gl.ApproveMergeRequestOptions{}
	if input.SHA != "" {
		approveOpts.SHA = new(input.SHA)
	}
	approvals, _, err := client.GL().MergeRequestApprovals.ApproveMergeRequest(string(input.ProjectID), input.MRIID, approveOpts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusUnauthorized) || toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return ApproveOutput{}, toolutil.WrapErrWithHint("mrApprove", err,
				"you may be the MR author (self-approval not allowed) or lack sufficient permissions")
		}
		if toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return ApproveOutput{}, toolutil.WrapErrWithHint("mrApprove", err,
				"MR not found or approval features require GitLab Premium. Use gitlab_mr_get to verify")
		}
		return ApproveOutput{}, toolutil.WrapErrWithMessage("mrApprove", err)
	}
	return ApproveOutput{
		ApprovalsRequired: int(approvals.ApprovalsRequired),
		ApprovedBy:        len(approvals.ApprovedBy),
		Approved:          approvals.Approved,
	}, nil
}

// Unapprove removes the current user's approval from the specified merge
// request. Returns an error if the API call fails.
func Unapprove(ctx context.Context, client *gitlabclient.Client, input ApproveInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("mrUnapprove: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.MRIID <= 0 {
		return toolutil.ErrRequiredInt64("mrUnapprove", "merge_request_iid")
	}
	_, err := client.GL().MergeRequestApprovals.UnapproveMergeRequest(string(input.ProjectID), input.MRIID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return toolutil.WrapErrWithHint("mrUnapprove", err,
				"verify project_id and merge_request_iid; unapproval requires you to have previously approved the MR (use gitlab_mr_approve first if you have not)")
		}
		return toolutil.WrapErrWithMessage("mrUnapprove", err)
	}
	return nil
}

// CommitsInput defines parameters for listing commits in a merge request.
type CommitsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"merge_request_iid"     jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	OrderBy   string               `json:"order_by,omitempty"    jsonschema:"Column to order results by (e.g. created_at)"`
	Sort      string               `json:"sort,omitempty"        jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// CommitsOutput holds a paginated list of commits for a merge request.
type CommitsOutput struct {
	toolutil.HintableOutput
	Commits    []commits.Output          `json:"commits"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// mrItemListOptions captures the offset/keyset pagination and order_by/sort
// parameters for the per-MR list helpers (commits, issues-closed,
// related-issues). applyTo wires the values onto a [gl.ListOptions] via
// ApplyListOptions, then sets OrderBy/Sort when supplied so an explicit
// value always wins.
type mrItemListOptions struct {
	pagination toolutil.PaginationInput
	keyset     toolutil.KeysetPaginationInput
	orderBy    string
	sort       string
}

func (o mrItemListOptions) applyTo(opts *gl.ListOptions) {
	toolutil.ApplyListOptions(opts, o.pagination, o.keyset)
	if o.orderBy != "" {
		opts.OrderBy = o.orderBy
	}
	if o.sort != "" {
		opts.Sort = o.sort
	}
}

type mergeRequestItemsListArgs struct {
	projectID         toolutil.StringOrInt
	mrIID             int64
	listOpts          mrItemListOptions
	operation         string
	missingProjectMsg string
	notFoundHint      string
}

func listMergeRequestItems[T, O, R any](ctx context.Context, args mergeRequestItemsListArgs, list func(string, int64, mrItemListOptions, ...gl.RequestOptionFunc) ([]T, *gl.Response, error), convert func(T) O, buildOutput func([]O, toolutil.PaginationOutput) R) (R, error) {
	var zero R
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if args.projectID == "" {
		return zero, errors.New(args.missingProjectMsg)
	}
	if args.mrIID <= 0 {
		return zero, toolutil.ErrRequiredInt64(args.operation, "merge_request_iid")
	}
	items, resp, err := list(string(args.projectID), args.mrIID, args.listOpts, gl.WithContext(ctx))
	if err != nil {
		return zero, toolutil.WrapErrWithStatusHint(args.operation, err, http.StatusNotFound, args.notFoundHint)
	}
	out := make([]O, len(items))
	for i, item := range items {
		out[i] = convert(item)
	}
	return buildOutput(out, toolutil.PaginationFromResponse(resp)), nil
}

// Commits retrieves the list of commits in a merge request.
func Commits(ctx context.Context, client *gitlabclient.Client, input CommitsInput) (CommitsOutput, error) {
	return listMergeRequestItems(ctx, mergeRequestItemsListArgs{
		projectID: input.ProjectID, mrIID: input.MRIID, operation: "mrCommits",
		listOpts: mrItemListOptions{
			pagination: input.PaginationInput, keyset: input.KeysetPaginationInput,
			orderBy: input.OrderBy, sort: input.Sort,
		},
		missingProjectMsg: "mrCommits: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id",
		notFoundHint:      "verify project_id and merge_request_iid (project-scoped IID, not global merge_request_id) with gitlab_mr_get",
	},
		func(projectID string, mrIID int64, lo mrItemListOptions, opts ...gl.RequestOptionFunc) ([]*gl.Commit, *gl.Response, error) {
			listOptions := &gl.GetMergeRequestCommitsOptions{}
			lo.applyTo(&listOptions.ListOptions)
			return client.GL().MergeRequests.GetMergeRequestCommits(projectID, mrIID, listOptions, opts...)
		}, commits.ToOutput, func(out []commits.Output, pagination toolutil.PaginationOutput) CommitsOutput {
			return CommitsOutput{Commits: out, Pagination: pagination}
		})
}

// PipelinesInput defines parameters for listing pipelines of a merge request.
type PipelinesInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"merge_request_iid"     jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
}

// PipelinesOutput holds the list of pipelines for a merge request.
type PipelinesOutput struct {
	toolutil.HintableOutput
	Pipelines []pipelines.Output `json:"pipelines"`
}

// Pipelines retrieves the list of pipelines for a merge request.
func Pipelines(ctx context.Context, client *gitlabclient.Client, input PipelinesInput) (PipelinesOutput, error) {
	if err := ctx.Err(); err != nil {
		return PipelinesOutput{}, err
	}
	if input.ProjectID == "" {
		return PipelinesOutput{}, errors.New("mrPipelines: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.MRIID <= 0 {
		return PipelinesOutput{}, toolutil.ErrRequiredInt64("mrPipelines", "merge_request_iid")
	}

	pipelineList, _, err := client.GL().MergeRequests.ListMergeRequestPipelines(string(input.ProjectID), input.MRIID, gl.WithContext(ctx))
	if err != nil {
		return PipelinesOutput{}, toolutil.WrapErrWithStatusHint("mrPipelines", err, http.StatusNotFound,
			"verify project_id and merge_request_iid with gitlab_mr_get \u2014 the MR may have no pipelines yet (use gitlab_mr_create_pipeline to trigger one)")
	}

	out := make([]pipelines.Output, len(pipelineList))
	for i, p := range pipelineList {
		out[i] = pipelines.ToOutput(p)
	}
	return PipelinesOutput{Pipelines: out}, nil
}

// DeleteInput defines parameters for deleting a merge request.
type DeleteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"merge_request_iid"     jsonschema:"Merge request IID to delete (project-scoped, not 'merge_request_id'),required"`
}

// Delete permanently deletes a merge request.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("mrDelete: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.MRIID <= 0 {
		return toolutil.ErrRequiredInt64("mrDelete", "merge_request_iid")
	}

	_, err := client.GL().MergeRequests.DeleteMergeRequest(string(input.ProjectID), input.MRIID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return toolutil.WrapErrWithHint("mrDelete", err,
				"only project owners can delete MRs. Use gitlab_mr_update with state_event='close' to close it instead")
		}
		return toolutil.WrapErrWithMessage("mrDelete", err)
	}
	return nil
}

// RebaseInput defines parameters for rebasing a merge request.
type RebaseInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id"        jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"merge_request_iid"            jsonschema:"Merge request IID to rebase (project-scoped, not 'merge_request_id'),required"`
	SkipCI    bool                 `json:"skip_ci,omitempty"  jsonschema:"Skip triggering CI pipeline after rebase"`
}

// RebaseOutput represents the result of a rebase operation.
type RebaseOutput struct {
	toolutil.HintableOutput
	RebaseInProgress bool `json:"rebase_in_progress"`
}

// Rebase triggers a rebase of the merge request's source branch.
func Rebase(ctx context.Context, client *gitlabclient.Client, input RebaseInput) (RebaseOutput, error) {
	if err := ctx.Err(); err != nil {
		return RebaseOutput{}, err
	}
	if input.ProjectID == "" {
		return RebaseOutput{}, errors.New("mrRebase: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.MRIID <= 0 {
		return RebaseOutput{}, toolutil.ErrRequiredInt64("mrRebase", "merge_request_iid")
	}

	opts := &gl.RebaseMergeRequestOptions{}
	if input.SkipCI {
		opts.SkipCI = new(true)
	}

	resp, err := client.GL().MergeRequests.RebaseMergeRequest(string(input.ProjectID), input.MRIID, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) || toolutil.IsHTTPStatus(err, http.StatusConflict) {
			return RebaseOutput{}, toolutil.WrapErrWithHint("mrRebase", err,
				"rebase may have conflicts requiring manual resolution, or a rebase is already in progress. Use gitlab_mr_get to check rebase_in_progress")
		}
		return RebaseOutput{}, toolutil.WrapErrWithMessage("mrRebase", err)
	}

	return RebaseOutput{RebaseInProgress: resp.StatusCode == http.StatusAccepted}, nil
}

// ---------------------------------------------------------------------------
// Global & Group MR listing
// ---------------------------------------------------------------------------.

// ListGlobalInput defines filters for listing merge requests across all projects.
type ListGlobalInput struct {
	State                  string                     `json:"state,omitempty"           jsonschema:"Filter by state (opened, closed, merged, all)"`
	Labels                 []string                   `json:"labels,omitempty"          jsonschema:"Label names to filter by"`
	NotLabels              []string                   `json:"not_labels,omitempty"      jsonschema:"Label names to exclude"`
	Milestone              string                     `json:"milestone,omitempty"       jsonschema:"Milestone title to filter by"`
	Scope                  string                     `json:"scope,omitempty"           jsonschema:"Filter by scope (created_by_me, assigned_to_me, all)"`
	Search                 string                     `json:"search,omitempty"          jsonschema:"Search in title and description"`
	SourceBranch           string                     `json:"source_branch,omitempty"   jsonschema:"Filter by source branch name"`
	TargetBranch           string                     `json:"target_branch,omitempty"   jsonschema:"Filter by target branch name"`
	AuthorUsername         string                     `json:"author_username,omitempty"     jsonschema:"Filter by author username"`
	NotAuthorUsername      string                     `json:"not_author_username,omitempty" jsonschema:"Exclude MRs authored by this username"`
	ReviewerUsername       string                     `json:"reviewer_username,omitempty"   jsonschema:"Filter by reviewer username"`
	Approved               string                     `json:"approved,omitempty"            jsonschema:"Filter by approval status: 'yes' or 'no' (Premium)"`
	In                     string                     `json:"in,omitempty"                  jsonschema:"Scope of the search filter (e.g. title, description, or title,description)"`
	MyReactionEmoji        string                     `json:"my_reaction_emoji,omitempty"   jsonschema:"Filter by MRs the caller reacted to with this emoji (e.g. thumbsup)"`
	View                   string                     `json:"view,omitempty"                jsonschema:"Set to 'simple' to return only basic MR fields"`
	WIP                    string                     `json:"wip,omitempty"                 jsonschema:"Filter by draft/WIP status: 'yes' for draft MRs, 'no' for non-draft"`
	AuthorID               int64                      `json:"author_id,omitempty"           jsonschema:"Filter by author user ID"`
	AssigneeID             int64                      `json:"assignee_id,omitempty"         jsonschema:"Filter by assignee user ID"`
	ReviewerID             int64                      `json:"reviewer_id,omitempty"         jsonschema:"Filter by reviewer user ID"`
	ApproverIDs            toolutil.ApproverIDsFilter `json:"approver_ids,omitempty"        jsonschema:"Filter by MRs with all listed users as eligible approvers. Accepts user IDs, or exactly one of \"Any\" (has approvers) or \"None\" (has none)"`
	ApprovedByIDs          toolutil.ApproverIDsFilter `json:"approved_by_ids,omitempty"     jsonschema:"Filter by MRs approved by all listed user IDs. Accepts user IDs, or exactly one of \"Any\" (approved by someone) or \"None\" (unapproved)"`
	ApprovedByUsernames    []string                   `json:"approved_by_usernames,omitempty" jsonschema:"Filter by MRs approved by all listed usernames"`
	WithLabelsDetails      *bool                      `json:"with_labels_details,omitempty"       jsonschema:"Include full label details (color, description) in the response"`
	WithMergeStatusRecheck *bool                      `json:"with_merge_status_recheck,omitempty" jsonschema:"Asynchronously recalculate each MR's merge_status before returning"`
	Draft                  *bool                      `json:"draft,omitempty"           jsonschema:"Filter by draft status (true=only drafts, false=only non-drafts)"`
	NonArchived            *bool                      `json:"non_archived,omitempty"    jsonschema:"Return merge requests from non-archived projects only. Default is true"`
	CreatedAfter           string                     `json:"created_after,omitempty"   jsonschema:"Return MRs created after date (ISO 8601)"`
	CreatedBefore          string                     `json:"created_before,omitempty"  jsonschema:"Return MRs created before date (ISO 8601)"`
	UpdatedAfter           string                     `json:"updated_after,omitempty"   jsonschema:"Return MRs updated after date (ISO 8601)"`
	UpdatedBefore          string                     `json:"updated_before,omitempty"  jsonschema:"Return MRs updated before date (ISO 8601)"`
	OrderBy                string                     `json:"order_by,omitempty"        jsonschema:"Order by field (created_at, updated_at)"`
	Sort                   string                     `json:"sort,omitempty"            jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListGlobal returns a paginated list of merge requests across all projects
// visible to the authenticated user.
func ListGlobal(ctx context.Context, client *gitlabclient.Client, input ListGlobalInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	opts, err := buildGlobalListOptions(input)
	if err != nil {
		return ListOutput{}, err
	}
	mrs, resp, err := client.GL().MergeRequests.ListMergeRequests(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("mrListGlobal", err, http.StatusUnauthorized,
			"global MR listing requires an authenticated token; results are scoped to MRs visible to the calling user (use scope=created_by_me or scope=assigned_to_me to narrow further)")
	}
	return mergeRequestListOutput(mrs, resp), nil
}

// buildGlobalListOptions maps ListGlobalInput to the GitLab API list options.
func buildGlobalListOptions(input ListGlobalInput) (*gl.ListMergeRequestsOptions, error) {
	opts := &gl.ListMergeRequestsOptions{}
	if err := applyMergeRequestListFilters(globalMRListFilters(input), globalMergeRequestListTarget(opts)); err != nil {
		return nil, err
	}
	return opts, nil
}

func globalMRListFilters(input ListGlobalInput) mergeRequestListFilters {
	return mergeRequestListFilters{
		State: input.State, Labels: input.Labels, NotLabels: input.NotLabels, Milestone: input.Milestone,
		Scope: input.Scope, Search: input.Search, SourceBranch: input.SourceBranch, TargetBranch: input.TargetBranch,
		AuthorUsername: input.AuthorUsername, NotAuthorUsername: input.NotAuthorUsername, ReviewerUsername: input.ReviewerUsername,
		Approved: input.Approved, In: input.In, MyReactionEmoji: input.MyReactionEmoji, View: input.View, WIP: input.WIP,
		AuthorID: input.AuthorID, AssigneeID: input.AssigneeID, ReviewerID: input.ReviewerID,
		ApproverIDs: input.ApproverIDs, ApprovedByIDs: input.ApprovedByIDs, ApprovedByUsernames: input.ApprovedByUsernames,
		WithLabelsDetails: input.WithLabelsDetails, WithMergeRecheck: input.WithMergeStatusRecheck,
		Draft: input.Draft, NonArchived: input.NonArchived, CreatedAfter: input.CreatedAfter, CreatedBefore: input.CreatedBefore, UpdatedAfter: input.UpdatedAfter,
		UpdatedBefore: input.UpdatedBefore, OrderBy: input.OrderBy, Sort: input.Sort, Page: input.Page, PerPage: input.PerPage,
		Keyset: input.KeysetPaginationInput,
	}
}

func globalMergeRequestListTarget(opts *gl.ListMergeRequestsOptions) mergeRequestListTarget {
	return newMergeRequestListTarget(mergeRequestListTargetFields{
		state: &opts.State, labels: &opts.Labels, notLabels: &opts.NotLabels, milestone: &opts.Milestone, scope: &opts.Scope,
		search: &opts.Search, sourceBranch: &opts.SourceBranch, targetBranch: &opts.TargetBranch, authorUsername: &opts.AuthorUsername,
		notAuthorUsername: &opts.NotAuthorUsername, reviewerUsername: &opts.ReviewerUsername,
		approved:        &opts.Approved, //nolint:staticcheck // SA1019: mirrored for 1:1 SDK fidelity; prefer approved_by_ids.
		in:              &opts.In,
		myReactionEmoji: &opts.MyReactionEmoji, view: &opts.View, wip: &opts.WIP, authorID: &opts.AuthorID,
		assigneeID: &opts.AssigneeID, reviewerID: &opts.ReviewerID, approverIDs: &opts.ApproverIDs, approvedByIDs: &opts.ApprovedByIDs, approvedByUsernames: &opts.ApprovedByUsernames,
		withLabelsDetails: &opts.WithLabelsDetails, withMergeRecheck: &opts.WithMergeStatusRecheck,
		draft: &opts.Draft, nonArchived: &opts.NonArchived, createdAfter: &opts.CreatedAfter, createdBefore: &opts.CreatedBefore,
		updatedAfter: &opts.UpdatedAfter, updatedBefore: &opts.UpdatedBefore, orderBy: &opts.OrderBy, sort: &opts.Sort,
		listOptions: &opts.ListOptions,
	})
}

type mergeRequestListTargetFields struct {
	state               **string
	labels              **gl.LabelOptions
	notLabels           **gl.LabelOptions
	milestone           **string
	scope               **string
	search              **string
	sourceBranch        **string
	targetBranch        **string
	authorUsername      **string
	notAuthorUsername   **string
	reviewerUsername    **string
	approved            **string
	in                  **string
	myReactionEmoji     **string
	view                **string
	wip                 **string
	environment         **string
	authorID            **int64
	assigneeID          **gl.AssigneeIDValue
	reviewerID          **gl.ReviewerIDValue
	approverIDs         **gl.ApproverIDsValue
	approvedByIDs       **gl.ApproverIDsValue
	approvedByUsernames **[]string
	withLabelsDetails   **bool
	withMergeRecheck    **bool
	draft               **bool
	nonArchived         **bool
	createdAfter        **time.Time
	createdBefore       **time.Time
	updatedAfter        **time.Time
	updatedBefore       **time.Time
	deployedAfter       **time.Time
	deployedBefore      **time.Time
	orderBy             **string
	sort                **string
	listOptions         *gl.ListOptions
}

func newMergeRequestListTarget(fields mergeRequestListTargetFields) mergeRequestListTarget {
	return mergeRequestListTarget{
		state: setStringPtr(fields.state), labels: setLabelOptionsPtr(fields.labels), notLabels: setLabelOptionsPtr(fields.notLabels),
		milestone: setStringPtr(fields.milestone), scope: setStringPtr(fields.scope), search: setStringPtr(fields.search),
		sourceBranch: setStringPtr(fields.sourceBranch), targetBranch: setStringPtr(fields.targetBranch),
		authorUsername: setStringPtr(fields.authorUsername), notAuthorUsername: setStringPtr(fields.notAuthorUsername),
		reviewerUsername: setStringPtr(fields.reviewerUsername), approved: setStringPtr(fields.approved), in: setStringPtr(fields.in),
		myReactionEmoji: setStringPtr(fields.myReactionEmoji), view: setStringPtr(fields.view), wip: setStringPtr(fields.wip),
		environment: setStringPtr(fields.environment), authorID: setInt64Ptr(fields.authorID),
		assigneeID: setAssigneeIDPtr(fields.assigneeID), reviewerID: setReviewerIDPtr(fields.reviewerID),
		approverIDs: setApproverIDsPtr(fields.approverIDs), approvedByIDs: setApproverIDsPtr(fields.approvedByIDs),
		approvedByUsernames: setStringSlicePtr(fields.approvedByUsernames),
		withLabelsDetails:   setBoolPtr(fields.withLabelsDetails), withMergeRecheck: setBoolPtr(fields.withMergeRecheck),
		draft: setBoolPtr(fields.draft), nonArchived: setBoolPtr(fields.nonArchived),
		createdAfter: setTimePtr(fields.createdAfter), createdBefore: setTimePtr(fields.createdBefore),
		updatedAfter: setTimePtr(fields.updatedAfter), updatedBefore: setTimePtr(fields.updatedBefore),
		deployedAfter: setTimePtr(fields.deployedAfter), deployedBefore: setTimePtr(fields.deployedBefore),
		orderBy: setStringPtr(fields.orderBy), sort: setStringPtr(fields.sort), listOptions: fields.listOptions,
	}
}

// setInt64Ptr returns a setter that wraps a scalar int64 into a *int64 stored
// on the target options field (used for author_id).
func setInt64Ptr(target **int64) func(int64) {
	return func(value int64) {
		v := value
		*target = &v
	}
}

// setAssigneeIDPtr returns a setter that wraps an int64 user ID into the SDK's
// *AssigneeIDValue via gl.AssigneeID.
func setAssigneeIDPtr(target **gl.AssigneeIDValue) func(int64) {
	return func(value int64) { *target = gl.AssigneeID(value) }
}

// setReviewerIDPtr returns a setter that wraps an int64 user ID into the SDK's
// *ReviewerIDValue via gl.ReviewerID.
func setReviewerIDPtr(target **gl.ReviewerIDValue) func(int64) {
	return func(value int64) { *target = gl.ReviewerID(value) }
}

// setApproverIDsPtr returns a setter that wraps a slice of user IDs into the
// SDK's *ApproverIDsValue via gl.ApproverIDs (used for approver_ids and
// approved_by_ids).
func setApproverIDsPtr(target **gl.ApproverIDsValue) func(*gl.ApproverIDsValue) {
	return func(value *gl.ApproverIDsValue) { *target = value }
}

func setStringSlicePtr(target **[]string) func([]string) {
	return func(value []string) { *target = &value }
}

func setStringPtr(target **string) func(*string) {
	return func(value *string) { *target = value }
}

func setLabelOptionsPtr(target **gl.LabelOptions) func(*gl.LabelOptions) {
	return func(value *gl.LabelOptions) { *target = value }
}

func setBoolPtr(target **bool) func(*bool) {
	return func(value *bool) { *target = value }
}

func setTimePtr(target **time.Time) func(*time.Time) {
	return func(value *time.Time) { *target = value }
}

// ListGroupInput defines filters for listing merge requests in a group.
type ListGroupInput struct {
	GroupID                toolutil.StringOrInt       `json:"group_id"                    jsonschema:"Group ID or URL-encoded path,required"`
	State                  string                     `json:"state,omitempty"             jsonschema:"Filter by state (opened, closed, merged, all)"`
	Labels                 []string                   `json:"labels,omitempty"            jsonschema:"Label names to filter by"`
	NotLabels              []string                   `json:"not_labels,omitempty"        jsonschema:"Label names to exclude"`
	Milestone              string                     `json:"milestone,omitempty"         jsonschema:"Milestone title to filter by"`
	Scope                  string                     `json:"scope,omitempty"             jsonschema:"Filter by scope (created_by_me, assigned_to_me, all)"`
	Search                 string                     `json:"search,omitempty"            jsonschema:"Search in title and description"`
	SourceBranch           string                     `json:"source_branch,omitempty"     jsonschema:"Filter by source branch name"`
	TargetBranch           string                     `json:"target_branch,omitempty"     jsonschema:"Filter by target branch name"`
	AuthorUsername         string                     `json:"author_username,omitempty"     jsonschema:"Filter by author username"`
	NotAuthorUsername      string                     `json:"not_author_username,omitempty" jsonschema:"Exclude MRs authored by this username"`
	ReviewerUsername       string                     `json:"reviewer_username,omitempty"   jsonschema:"Filter by reviewer username"`
	In                     string                     `json:"in,omitempty"                  jsonschema:"Scope of the search filter (e.g. title, description, or title,description)"`
	MyReactionEmoji        string                     `json:"my_reaction_emoji,omitempty"   jsonschema:"Filter by MRs the caller reacted to with this emoji (e.g. thumbsup)"`
	View                   string                     `json:"view,omitempty"                jsonschema:"Set to 'simple' to return only basic MR fields"`
	WIP                    string                     `json:"wip,omitempty"                 jsonschema:"Filter by draft/WIP status: 'yes' for draft MRs, 'no' for non-draft"`
	AuthorID               int64                      `json:"author_id,omitempty"           jsonschema:"Filter by author user ID"`
	AssigneeID             int64                      `json:"assignee_id,omitempty"         jsonschema:"Filter by assignee user ID"`
	ReviewerID             int64                      `json:"reviewer_id,omitempty"         jsonschema:"Filter by reviewer user ID"`
	ApproverIDs            toolutil.ApproverIDsFilter `json:"approver_ids,omitempty"        jsonschema:"Filter by MRs with all listed users as eligible approvers. Accepts user IDs, or exactly one of \"Any\" (has approvers) or \"None\" (has none)"`
	ApprovedByIDs          toolutil.ApproverIDsFilter `json:"approved_by_ids,omitempty"     jsonschema:"Filter by MRs approved by all listed user IDs. Accepts user IDs, or exactly one of \"Any\" (approved by someone) or \"None\" (unapproved)"`
	ApprovedByUsernames    []string                   `json:"approved_by_usernames,omitempty" jsonschema:"Filter by MRs approved by all listed usernames"`
	WithLabelsDetails      *bool                      `json:"with_labels_details,omitempty"       jsonschema:"Include full label details (color, description) in the response"`
	WithMergeStatusRecheck *bool                      `json:"with_merge_status_recheck,omitempty" jsonschema:"Asynchronously recalculate each MR's merge_status before returning"`
	Draft                  *bool                      `json:"draft,omitempty"             jsonschema:"Filter by draft status (true=only drafts, false=only non-drafts)"`
	NonArchived            *bool                      `json:"non_archived,omitempty"      jsonschema:"Return merge requests from non-archived projects only. Default is true"`
	CreatedAfter           string                     `json:"created_after,omitempty"     jsonschema:"Return MRs created after date (ISO 8601)"`
	CreatedBefore          string                     `json:"created_before,omitempty"    jsonschema:"Return MRs created before date (ISO 8601)"`
	UpdatedAfter           string                     `json:"updated_after,omitempty"     jsonschema:"Return MRs updated after date (ISO 8601)"`
	UpdatedBefore          string                     `json:"updated_before,omitempty"    jsonschema:"Return MRs updated before date (ISO 8601)"`
	OrderBy                string                     `json:"order_by,omitempty"          jsonschema:"Order by field (created_at, updated_at)"`
	Sort                   string                     `json:"sort,omitempty"              jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListGroup returns a paginated list of merge requests in a group.
func ListGroup(ctx context.Context, client *gitlabclient.Client, input ListGroupInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.GroupID == "" {
		return ListOutput{}, errors.New("mrListGroup: group_id is required")
	}
	opts, err := buildGroupListOptions(input)
	if err != nil {
		return ListOutput{}, err
	}
	mrs, resp, err := client.GL().MergeRequests.ListGroupMergeRequests(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("mrListGroup", err, http.StatusNotFound,
			"verify the group exists with gitlab_group_get \u2014 use full_path or numeric ID")
	}
	out := make([]Output, len(mrs))
	for i, m := range mrs {
		out[i] = BasicToOutput(m)
	}
	return ListOutput{MergeRequests: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// buildGroupListOptions maps ListGroupInput to the GitLab API list options.
func buildGroupListOptions(input ListGroupInput) (*gl.ListGroupMergeRequestsOptions, error) {
	opts := &gl.ListGroupMergeRequestsOptions{}
	if err := applyMergeRequestListFilters(groupMRListFilters(input), groupMergeRequestListTarget(opts)); err != nil {
		return nil, err
	}
	return opts, nil
}

func groupMRListFilters(input ListGroupInput) mergeRequestListFilters {
	return mergeRequestListFilters{
		State: input.State, Labels: input.Labels, NotLabels: input.NotLabels, Milestone: input.Milestone,
		Scope: input.Scope, Search: input.Search, SourceBranch: input.SourceBranch, TargetBranch: input.TargetBranch,
		AuthorUsername: input.AuthorUsername, NotAuthorUsername: input.NotAuthorUsername, ReviewerUsername: input.ReviewerUsername,
		In: input.In, MyReactionEmoji: input.MyReactionEmoji, View: input.View, WIP: input.WIP,
		AuthorID: input.AuthorID, AssigneeID: input.AssigneeID, ReviewerID: input.ReviewerID,
		ApproverIDs: input.ApproverIDs, ApprovedByIDs: input.ApprovedByIDs, ApprovedByUsernames: input.ApprovedByUsernames,
		WithLabelsDetails: input.WithLabelsDetails, WithMergeRecheck: input.WithMergeStatusRecheck,
		Draft: input.Draft, NonArchived: input.NonArchived, CreatedAfter: input.CreatedAfter, CreatedBefore: input.CreatedBefore, UpdatedAfter: input.UpdatedAfter,
		UpdatedBefore: input.UpdatedBefore, OrderBy: input.OrderBy, Sort: input.Sort, Page: input.Page, PerPage: input.PerPage,
		Keyset: input.KeysetPaginationInput,
	}
}

func groupMergeRequestListTarget(opts *gl.ListGroupMergeRequestsOptions) mergeRequestListTarget {
	return newMergeRequestListTarget(mergeRequestListTargetFields{
		state: &opts.State, labels: &opts.Labels, notLabels: &opts.NotLabels, milestone: &opts.Milestone, scope: &opts.Scope,
		search: &opts.Search, sourceBranch: &opts.SourceBranch, targetBranch: &opts.TargetBranch, authorUsername: &opts.AuthorUsername,
		notAuthorUsername: &opts.NotAuthorUsername, reviewerUsername: &opts.ReviewerUsername, in: &opts.In,
		myReactionEmoji: &opts.MyReactionEmoji, view: &opts.View, wip: &opts.WIP, authorID: &opts.AuthorID,
		assigneeID: &opts.AssigneeID, reviewerID: &opts.ReviewerID, approverIDs: &opts.ApproverIDs, approvedByIDs: &opts.ApprovedByIDs, approvedByUsernames: &opts.ApprovedByUsernames,
		withLabelsDetails: &opts.WithLabelsDetails, withMergeRecheck: &opts.WithMergeStatusRecheck,
		draft: &opts.Draft, nonArchived: &opts.NonArchived, createdAfter: &opts.CreatedAfter, createdBefore: &opts.CreatedBefore,
		updatedAfter: &opts.UpdatedAfter, updatedBefore: &opts.UpdatedBefore, orderBy: &opts.OrderBy, sort: &opts.Sort,
		listOptions: &opts.ListOptions,
	})
}

// ---------------------------------------------------------------------------
// Participants & Reviewers
// ---------------------------------------------------------------------------.

// ParticipantsInput defines parameters for listing MR participants.
type ParticipantsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"merge_request_iid"     jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
}

// ParticipantOutput represents a single MR participant.
type ParticipantOutput struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	State     string `json:"state"`
	AvatarURL string `json:"avatar_url,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
}

// ParticipantsOutput holds the list of participants for a merge request.
type ParticipantsOutput struct {
	toolutil.HintableOutput
	Participants []ParticipantOutput `json:"participants"`
}

// Participants retrieves the list of users who participated in a merge request.
func Participants(ctx context.Context, client *gitlabclient.Client, input ParticipantsInput) (ParticipantsOutput, error) {
	if err := ctx.Err(); err != nil {
		return ParticipantsOutput{}, err
	}
	if input.ProjectID == "" {
		return ParticipantsOutput{}, errors.New("mrParticipants: project_id is required")
	}
	if input.MRIID <= 0 {
		return ParticipantsOutput{}, toolutil.ErrRequiredInt64("mrParticipants", "merge_request_iid")
	}
	users, _, err := client.GL().MergeRequests.GetMergeRequestParticipants(string(input.ProjectID), input.MRIID, gl.WithContext(ctx))
	if err != nil {
		return ParticipantsOutput{}, toolutil.WrapErrWithStatusHint("mrParticipants", err, http.StatusNotFound,
			hintVerifyMR)
	}
	out := make([]ParticipantOutput, len(users))
	for i, u := range users {
		out[i] = ParticipantOutput{
			ID:        u.ID,
			Username:  u.Username,
			Name:      u.Name,
			State:     u.State,
			AvatarURL: u.AvatarURL,
			WebURL:    u.WebURL,
		}
	}
	return ParticipantsOutput{Participants: out}, nil
}

// ReviewerOutput represents a single MR reviewer with review state.
type ReviewerOutput struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	State     string `json:"state"`
	AvatarURL string `json:"avatar_url,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
	Review    string `json:"review_state,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

// ReviewersOutput holds the list of reviewers for a merge request.
type ReviewersOutput struct {
	toolutil.HintableOutput
	Reviewers []ReviewerOutput `json:"reviewers"`
}

// Reviewers retrieves the list of reviewers assigned to a merge request.
func Reviewers(ctx context.Context, client *gitlabclient.Client, input ParticipantsInput) (ReviewersOutput, error) {
	if err := ctx.Err(); err != nil {
		return ReviewersOutput{}, err
	}
	if input.ProjectID == "" {
		return ReviewersOutput{}, errors.New("mrReviewers: project_id is required")
	}
	if input.MRIID <= 0 {
		return ReviewersOutput{}, toolutil.ErrRequiredInt64("mrReviewers", "merge_request_iid")
	}
	reviewers, _, err := client.GL().MergeRequests.GetMergeRequestReviewers(string(input.ProjectID), input.MRIID, gl.WithContext(ctx))
	if err != nil {
		return ReviewersOutput{}, toolutil.WrapErrWithStatusHint("mrReviewers", err, http.StatusNotFound,
			"verify project_id and merge_request_iid with gitlab_mr_get \u2014 use gitlab_mr_update with reviewer_ids to assign reviewers")
	}
	out := make([]ReviewerOutput, len(reviewers))
	for i, r := range reviewers {
		ro := ReviewerOutput{
			Review: r.State,
		}
		if r.CreatedAt != nil {
			ro.CreatedAt = r.CreatedAt.Format(time.RFC3339)
		}
		if r.User != nil {
			ro.ID = r.User.ID
			ro.Username = r.User.Username
			ro.Name = r.User.Name
			ro.State = r.User.State
			ro.AvatarURL = r.User.AvatarURL
			ro.WebURL = r.User.WebURL
		}
		out[i] = ro
	}
	return ReviewersOutput{Reviewers: out}, nil
}

// ---------------------------------------------------------------------------
// Create MR pipeline
// ---------------------------------------------------------------------------.

// CreatePipelineInput defines parameters for creating a pipeline for a merge request.
type CreatePipelineInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"merge_request_iid"     jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
}

// CreatePipeline triggers a new pipeline for the specified merge request.
func CreatePipeline(ctx context.Context, client *gitlabclient.Client, input CreatePipelineInput) (pipelines.Output, error) {
	if err := ctx.Err(); err != nil {
		return pipelines.Output{}, err
	}
	if input.ProjectID == "" {
		return pipelines.Output{}, errors.New("mrCreatePipeline: project_id is required")
	}
	if input.MRIID <= 0 {
		return pipelines.Output{}, toolutil.ErrRequiredInt64("mrCreatePipeline", "merge_request_iid")
	}
	pi, _, err := client.GL().MergeRequests.CreateMergeRequestPipeline(string(input.ProjectID), input.MRIID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return pipelines.Output{}, toolutil.WrapErrWithHint("mrCreatePipeline", err,
				"creating MR pipelines requires Developer role on the project; the project may also have CI/CD disabled")
		}
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return pipelines.Output{}, toolutil.WrapErrWithHint("mrCreatePipeline", err,
				"the MR's source branch may have no .gitlab-ci.yml or the pipeline configuration is invalid; use gitlab_ci_lint to validate the YAML")
		}
		return pipelines.Output{}, toolutil.WrapErrWithStatusHint("mrCreatePipeline", err, http.StatusNotFound,
			hintVerifyMR)
	}
	return pipelines.ToOutput(pi), nil
}

// ---------------------------------------------------------------------------
// Issues closed on merge
// ---------------------------------------------------------------------------.

// IssuesClosedInput defines parameters for listing issues that close on merge.
type IssuesClosedInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"merge_request_iid"     jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	OrderBy   string               `json:"order_by,omitempty"    jsonschema:"Column to order results by (e.g. created_at)"`
	Sort      string               `json:"sort,omitempty"        jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// IssuesClosedOutput holds the list of issues that would be closed by merging an MR.
type IssuesClosedOutput struct {
	toolutil.HintableOutput
	Issues     []issues.Output           `json:"issues"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// IssuesClosed retrieves the list of issues that would be closed when
// the specified merge request is merged.
func IssuesClosed(ctx context.Context, client *gitlabclient.Client, input IssuesClosedInput) (IssuesClosedOutput, error) {
	return listMergeRequestIssues(ctx, mergeRequestItemsListArgs{
		projectID: input.ProjectID, mrIID: input.MRIID, operation: "mrIssuesClosed",
		listOpts: mrItemListOptions{
			pagination: input.PaginationInput, keyset: input.KeysetPaginationInput,
			orderBy: input.OrderBy, sort: input.Sort,
		},
		missingProjectMsg: "mrIssuesClosed: project_id is required",
		notFoundHint:      "verify project_id and merge_request_iid with gitlab_mr_get - only issues referenced via 'Closes #N' in MR description/commits are returned",
	},
		func(projectID string, mrIID int64, lo mrItemListOptions, opts ...gl.RequestOptionFunc) ([]*gl.Issue, *gl.Response, error) {
			listOptions := &gl.GetIssuesClosedOnMergeOptions{}
			lo.applyTo(&listOptions.ListOptions)
			return client.GL().MergeRequests.GetIssuesClosedOnMerge(projectID, mrIID, listOptions, opts...)
		})
}

func listMergeRequestIssues(ctx context.Context, args mergeRequestItemsListArgs, list func(string, int64, mrItemListOptions, ...gl.RequestOptionFunc) ([]*gl.Issue, *gl.Response, error)) (IssuesClosedOutput, error) {
	return listMergeRequestItems(ctx, args,
		list, issues.ToOutput, func(out []issues.Output, pagination toolutil.PaginationOutput) IssuesClosedOutput {
			return IssuesClosedOutput{Issues: out, Pagination: pagination}
		})
}

// ---------------------------------------------------------------------------
// Cancel auto-merge
// ---------------------------------------------------------------------------.

// CancelAutoMerge cancels the "merge when pipeline succeeds" (auto-merge)
// setting on a merge request. Returns the updated merge request.
func CancelAutoMerge(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("mrCancelAutoMerge: project_id is required")
	}
	if input.MRIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("mrCancelAutoMerge", "merge_request_iid")
	}
	mr, _, err := client.GL().MergeRequests.CancelMergeWhenPipelineSucceeds(string(input.ProjectID), input.MRIID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusMethodNotAllowed) || toolutil.IsHTTPStatus(err, http.StatusNotAcceptable) {
			return Output{}, toolutil.WrapErrWithHint("mrCancelAutoMerge", err,
				"the MR may already be merged/closed, or auto-merge was not enabled. Use gitlab_mr_get to check state and auto_merge_enabled")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("mrCancelAutoMerge", err, http.StatusNotFound,
			hintVerifyMR)
	}
	return ToOutput(mr), nil
}

// ---------------------------------------------------------------------------
// Subscribe / Unsubscribe
// ---------------------------------------------------------------------------.

// Subscribe subscribes the authenticated user to the given merge request
// to receive notifications. Returns the updated merge request.
func Subscribe(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("mrSubscribe: project_id is required")
	}
	if input.MRIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("mrSubscribe", "merge_request_iid")
	}
	mr, _, err := client.GL().MergeRequests.SubscribeToMergeRequest(string(input.ProjectID), input.MRIID, gl.WithContext(ctx))
	if err != nil {
		// GitLab returns 304 Not Modified with empty body when already subscribed,
		// which causes EOF during JSON decode. Fall back to Get.
		if errors.Is(err, io.EOF) || toolutil.IsHTTPStatus(err, http.StatusNotModified) {
			return Get(ctx, client, input)
		}
		return Output{}, toolutil.WrapErrWithStatusHint("mrSubscribe", err, http.StatusNotFound,
			hintVerifyMR)
	}
	return ToOutput(mr), nil
}

// Unsubscribe unsubscribes the authenticated user from the given merge request.
// Returns the updated merge request.
func Unsubscribe(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("mrUnsubscribe: project_id is required")
	}
	if input.MRIID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("mrUnsubscribe", "merge_request_iid")
	}
	mr, _, err := client.GL().MergeRequests.UnsubscribeFromMergeRequest(string(input.ProjectID), input.MRIID, gl.WithContext(ctx))
	if err != nil {
		if errors.Is(err, io.EOF) || toolutil.IsHTTPStatus(err, http.StatusNotModified) {
			return Get(ctx, client, input)
		}
		return Output{}, toolutil.WrapErrWithStatusHint("mrUnsubscribe", err, http.StatusNotFound,
			hintVerifyMR)
	}
	return ToOutput(mr), nil
}

// ---------------------------------------------------------------------------
// Time Tracking
// ---------------------------------------------------------------------------.

// TimeStatsOutput is the standalone return type for the five time-tracking
// handlers (SetTimeEstimate, ResetTimeEstimate, AddSpentTime, ResetSpentTime,
// GetTimeStats). It embeds both HintableOutput (adds next_steps at the top
// level) and toolutil.TimeStatsOutput (the pure four-field sub-object). JSON
// serialization flattens both embeds, producing:
//
//	{ "next_steps": [...], "human_time_estimate": "3h", ... }
//
// The nested Output.TimeStats field uses *toolutil.TimeStatsOutput (pure, no
// next_steps) to avoid schema noise inside a compound response.
type TimeStatsOutput struct {
	toolutil.HintableOutput
	toolutil.TimeStatsOutput
}

// timeStatsToOutput converts the GitLab API response to the standalone
// TimeStatsOutput used by the five time-tracking handlers.
func timeStatsToOutput(ts *gl.TimeStats) TimeStatsOutput {
	if ts == nil {
		return TimeStatsOutput{}
	}
	return TimeStatsOutput{
		HumanTimeEstimate:   ts.HumanTimeEstimate,
		HumanTotalTimeSpent: ts.HumanTotalTimeSpent,
		TimeEstimate:        ts.TimeEstimate,
		TotalTimeSpent:      ts.TotalTimeSpent,
	}
}

// SetTimeEstimateInput defines parameters for setting a time estimate on an MR.
type SetTimeEstimateInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"merge_request_iid"     jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	Duration  string               `json:"duration"   jsonschema:"Human-readable duration (e.g. 3h30m, 1w2d),required"`
}

// SetTimeEstimate sets the time estimate for a merge request.
func SetTimeEstimate(ctx context.Context, client *gitlabclient.Client, input SetTimeEstimateInput) (TimeStatsOutput, error) {
	if err := ctx.Err(); err != nil {
		return TimeStatsOutput{}, err
	}
	if input.ProjectID == "" {
		return TimeStatsOutput{}, errors.New("mrSetTimeEstimate: project_id is required")
	}
	if input.MRIID <= 0 {
		return TimeStatsOutput{}, toolutil.ErrRequiredInt64("mrSetTimeEstimate", "merge_request_iid")
	}
	if input.Duration == "" {
		return TimeStatsOutput{}, errors.New("mrSetTimeEstimate: duration is required")
	}
	ts, _, err := client.GL().MergeRequests.SetTimeEstimate(string(input.ProjectID), input.MRIID,
		&gl.SetTimeEstimateOptions{Duration: new(input.Duration)}, gl.WithContext(ctx))
	if err != nil {
		return TimeStatsOutput{}, toolutil.WrapErrWithStatusHint("mrSetTimeEstimate", err, http.StatusBadRequest,
			"duration must be a GitLab time-tracking string like '3h30m', '1d', or '2w' (units: M=mo, w=wk, d=day, h=hr, m=min)")
	}
	return timeStatsToOutput(ts), nil
}

// ResetTimeEstimate resets the time estimate for a merge request.
func ResetTimeEstimate(ctx context.Context, client *gitlabclient.Client, input GetInput) (TimeStatsOutput, error) {
	if err := ctx.Err(); err != nil {
		return TimeStatsOutput{}, err
	}
	if input.ProjectID == "" {
		return TimeStatsOutput{}, errors.New("mrResetTimeEstimate: project_id is required")
	}
	if input.MRIID <= 0 {
		return TimeStatsOutput{}, toolutil.ErrRequiredInt64("mrResetTimeEstimate", "merge_request_iid")
	}
	ts, _, err := client.GL().MergeRequests.ResetTimeEstimate(string(input.ProjectID), input.MRIID, gl.WithContext(ctx))
	if err != nil {
		return TimeStatsOutput{}, toolutil.WrapErrWithStatusHint("mrResetTimeEstimate", err, http.StatusNotFound,
			hintVerifyMR)
	}
	return timeStatsToOutput(ts), nil
}

// AddSpentTimeInput defines parameters for adding spent time to an MR.
type AddSpentTimeInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"merge_request_iid"     jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	Duration  string               `json:"duration"   jsonschema:"Human-readable duration (e.g. 1h, 30m, 1w2d),required"`
	Summary   string               `json:"summary,omitempty" jsonschema:"Optional summary of work done"`
}

// AddSpentTime adds spent time for a merge request.
func AddSpentTime(ctx context.Context, client *gitlabclient.Client, input AddSpentTimeInput) (TimeStatsOutput, error) {
	if err := ctx.Err(); err != nil {
		return TimeStatsOutput{}, err
	}
	if input.ProjectID == "" {
		return TimeStatsOutput{}, errors.New("mrAddSpentTime: project_id is required")
	}
	if input.MRIID <= 0 {
		return TimeStatsOutput{}, toolutil.ErrRequiredInt64("mrAddSpentTime", "merge_request_iid")
	}
	if input.Duration == "" {
		return TimeStatsOutput{}, errors.New("mrAddSpentTime: duration is required")
	}
	opts := &gl.AddSpentTimeOptions{Duration: new(input.Duration)}
	if input.Summary != "" {
		opts.Summary = new(input.Summary)
	}
	ts, _, err := client.GL().MergeRequests.AddSpentTime(string(input.ProjectID), input.MRIID, opts, gl.WithContext(ctx))
	if err != nil {
		return TimeStatsOutput{}, toolutil.WrapErrWithStatusHint("mrAddSpentTime", err, http.StatusBadRequest,
			"duration must be a GitLab time-tracking string like '3h30m'; use a leading '-' to subtract logged time (e.g. '-1h')")
	}
	return timeStatsToOutput(ts), nil
}

// ResetSpentTime resets the total spent time for a merge request.
func ResetSpentTime(ctx context.Context, client *gitlabclient.Client, input GetInput) (TimeStatsOutput, error) {
	if err := ctx.Err(); err != nil {
		return TimeStatsOutput{}, err
	}
	if input.ProjectID == "" {
		return TimeStatsOutput{}, errors.New("mrResetSpentTime: project_id is required")
	}
	if input.MRIID <= 0 {
		return TimeStatsOutput{}, toolutil.ErrRequiredInt64("mrResetSpentTime", "merge_request_iid")
	}
	ts, _, err := client.GL().MergeRequests.ResetSpentTime(string(input.ProjectID), input.MRIID, gl.WithContext(ctx))
	if err != nil {
		return TimeStatsOutput{}, toolutil.WrapErrWithStatusHint("mrResetSpentTime", err, http.StatusNotFound,
			hintVerifyMR)
	}
	return timeStatsToOutput(ts), nil
}

// GetTimeStats retrieves total time tracking statistics for a merge request.
func GetTimeStats(ctx context.Context, client *gitlabclient.Client, input GetInput) (TimeStatsOutput, error) {
	if err := ctx.Err(); err != nil {
		return TimeStatsOutput{}, err
	}
	if input.ProjectID == "" {
		return TimeStatsOutput{}, errors.New("mrGetTimeStats: project_id is required")
	}
	if input.MRIID <= 0 {
		return TimeStatsOutput{}, toolutil.ErrRequiredInt64("mrGetTimeStats", "merge_request_iid")
	}
	ts, _, err := client.GL().MergeRequests.GetTimeSpent(string(input.ProjectID), input.MRIID, gl.WithContext(ctx))
	if err != nil {
		return TimeStatsOutput{}, toolutil.WrapErrWithStatusHint("mrGetTimeStats", err, http.StatusNotFound,
			hintVerifyMR)
	}
	return timeStatsToOutput(ts), nil
}

// ---------------------------------------------------------------------------
// Related Issues
// ---------------------------------------------------------------------------.

// RelatedIssuesInput defines parameters for listing issues related to an MR.
type RelatedIssuesInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"merge_request_iid"     jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	OrderBy   string               `json:"order_by,omitempty"    jsonschema:"Column to order results by (e.g. created_at)"`
	Sort      string               `json:"sort,omitempty"        jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// RelatedIssuesOutput holds the list of issues related to a merge request.
type RelatedIssuesOutput struct {
	toolutil.HintableOutput
	Issues     []issues.Output           `json:"issues"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// RelatedIssues retrieves the list of issues related to a merge request.
func RelatedIssues(ctx context.Context, client *gitlabclient.Client, input RelatedIssuesInput) (RelatedIssuesOutput, error) {
	return listMergeRequestRelatedIssues(ctx, mergeRequestItemsListArgs{
		projectID: input.ProjectID, mrIID: input.MRIID, operation: "mrRelatedIssues",
		listOpts: mrItemListOptions{
			pagination: input.PaginationInput, keyset: input.KeysetPaginationInput,
			orderBy: input.OrderBy, sort: input.Sort,
		},
		missingProjectMsg: "mrRelatedIssues: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id",
		notFoundHint:      "verify project_id and merge_request_iid with gitlab_mr_get - only issues referenced in MR description/commits/notes are returned",
	},
		func(projectID string, mrIID int64, lo mrItemListOptions, opts ...gl.RequestOptionFunc) ([]*gl.Issue, *gl.Response, error) {
			listOptions := &gl.ListRelatedIssuesOptions{}
			lo.applyTo(&listOptions.ListOptions)
			return client.GL().MergeRequests.ListRelatedIssues(projectID, mrIID, listOptions, opts...)
		})
}

func listMergeRequestRelatedIssues(ctx context.Context, args mergeRequestItemsListArgs, list func(string, int64, mrItemListOptions, ...gl.RequestOptionFunc) ([]*gl.Issue, *gl.Response, error)) (RelatedIssuesOutput, error) {
	return listMergeRequestItems(ctx, args,
		list, issues.ToOutput, func(out []issues.Output, pagination toolutil.PaginationOutput) RelatedIssuesOutput {
			return RelatedIssuesOutput{Issues: out, Pagination: pagination}
		})
}

// ---------------------------------------------------------------------------
// Create To-Do for MR
// ---------------------------------------------------------------------------.

// CreateTodoInput defines parameters for creating a to-do item on a merge request.
type CreateTodoInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"merge_request_iid"     jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
}

// CreateTodoOutput holds the created to-do item details.
type CreateTodoOutput struct {
	toolutil.HintableOutput
	ID          int64  `json:"id"`
	ActionName  string `json:"action_name"`
	TargetType  string `json:"target_type"`
	TargetTitle string `json:"target_title"`
	TargetURL   string `json:"target_url"`
	State       string `json:"state"`
	ProjectName string `json:"project_name,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
}

// CreateTodo creates a to-do item on the specified merge request for the
// authenticated user. Returns the created to-do item details.
func CreateTodo(ctx context.Context, client *gitlabclient.Client, input CreateTodoInput) (CreateTodoOutput, error) {
	if err := ctx.Err(); err != nil {
		return CreateTodoOutput{}, err
	}
	if input.ProjectID == "" {
		return CreateTodoOutput{}, errors.New("mrCreateTodo: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.MRIID <= 0 {
		return CreateTodoOutput{}, toolutil.ErrRequiredInt64("mrCreateTodo", "merge_request_iid")
	}
	todo, resp, err := client.GL().MergeRequests.CreateTodo(string(input.ProjectID), input.MRIID, gl.WithContext(ctx))
	if err != nil {
		if resp != nil && resp.Response != nil && resp.StatusCode == http.StatusNotModified {
			err = &gl.ErrorResponse{Response: resp.Response, Message: "a pending todo for this MR already exists"}
			return CreateTodoOutput{}, toolutil.WrapErrWithHint("mrCreateTodo", err,
				"a pending todo for this MR already exists for the authenticated user \u2014 use gitlab_todo_list to inspect it")
		}
		return CreateTodoOutput{}, toolutil.WrapErrWithStatusHint("mrCreateTodo", err, http.StatusNotFound,
			hintVerifyMR)
	}
	out := CreateTodoOutput{
		ID:         todo.ID,
		ActionName: string(todo.ActionName),
		TargetType: string(todo.TargetType),
		TargetURL:  todo.TargetURL,
		State:      todo.State,
	}
	if todo.Target != nil {
		out.TargetTitle = todo.Target.Title
	}
	if todo.Project != nil {
		out.ProjectName = todo.Project.Name
	}
	if todo.CreatedAt != nil {
		out.CreatedAt = todo.CreatedAt.Format(time.RFC3339)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Merge Request Dependencies
// ---------------------------------------------------------------------------.

// DependencyInput defines parameters for creating a merge request dependency.
type DependencyInput struct {
	ProjectID              toolutil.StringOrInt `json:"project_id"                jsonschema:"Project ID or URL-encoded path,required"`
	MRIID                  int64                `json:"merge_request_iid"                    jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	BlockingMergeRequestID int64                `json:"blocking_merge_request_id" jsonschema:"ID of the merge request that blocks this one"`
}

// DependencyOutput mirrors gl.MergeRequestDependency. The blocking merge
// request is surfaced as a full nested object on the canonical
// blocking_merge_request key rather than flattened scalars.
type DependencyOutput struct {
	toolutil.HintableOutput
	ID                   int64                       `json:"id"`
	BlockingMergeRequest *BlockingMergeRequestOutput `json:"blocking_merge_request,omitempty"`
	ProjectID            int64                       `json:"project_id"`
}

// DependenciesOutput holds a list of merge request dependencies.
type DependenciesOutput struct {
	toolutil.HintableOutput
	Dependencies []DependencyOutput `json:"dependencies"`
}

// dependencyToOutput converts the GitLab API response to the tool output format.
func dependencyToOutput(d *gl.MergeRequestDependency) DependencyOutput {
	return DependencyOutput{
		ID:                   d.ID,
		ProjectID:            d.ProjectID,
		BlockingMergeRequest: blockingMergeRequestOutput(d.BlockingMergeRequest),
	}
}

// CreateDependency creates a new dependency (blocker) on a merge request.
func CreateDependency(ctx context.Context, client *gitlabclient.Client, input DependencyInput) (DependencyOutput, error) {
	if err := ctx.Err(); err != nil {
		return DependencyOutput{}, err
	}
	if input.ProjectID == "" {
		return DependencyOutput{}, errors.New("mrCreateDependency: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.MRIID <= 0 {
		return DependencyOutput{}, toolutil.ErrRequiredInt64("mrCreateDependency", "merge_request_iid")
	}
	dep, _, err := client.GL().MergeRequests.CreateMergeRequestDependency(string(input.ProjectID), input.MRIID,
		gl.CreateMergeRequestDependencyOptions{BlockingMergeRequestID: new(input.BlockingMergeRequestID)}, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return DependencyOutput{}, toolutil.WrapErrWithHint("mrCreateDependency", err,
				"MR dependencies require GitLab Premium or Ultimate; on Free tier the endpoint returns 403")
		}
		if toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity) || toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return DependencyOutput{}, toolutil.WrapErrWithHint("mrCreateDependency", err,
				"dependency would create a cycle, the blocking MR does not exist, or this dependency already exists \u2014 use gitlab_mr_dependencies_list to inspect current dependencies")
		}
		return DependencyOutput{}, toolutil.WrapErrWithStatusHint("mrCreateDependency", err, http.StatusNotFound,
			"verify project_id and merge_request_iid with gitlab_mr_get; blocking_merge_request_id is a global database ID, not an IID")
	}
	return dependencyToOutput(dep), nil
}

// DeleteDependencyInput defines parameters for deleting a merge request dependency.
type DeleteDependencyInput struct {
	ProjectID              toolutil.StringOrInt `json:"project_id"                jsonschema:"Project ID or URL-encoded path,required"`
	MRIID                  int64                `json:"merge_request_iid"                    jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	BlockingMergeRequestID int64                `json:"blocking_merge_request_id" jsonschema:"ID of the blocking merge request to remove"`
}

// DeleteDependency removes a dependency (blocker) from a merge request.
func DeleteDependency(ctx context.Context, client *gitlabclient.Client, input DeleteDependencyInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("mrDeleteDependency: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.MRIID <= 0 {
		return toolutil.ErrRequiredInt64("mrDeleteDependency", "merge_request_iid")
	}
	_, err := client.GL().MergeRequests.DeleteMergeRequestDependency(string(input.ProjectID), input.MRIID, input.BlockingMergeRequestID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return toolutil.WrapErrWithHint("mrDeleteDependency", err,
				"MR dependencies require GitLab Premium or Ultimate")
		}
		return toolutil.WrapErrWithStatusHint("mrDeleteDependency", err, http.StatusNotFound,
			"dependency not currently active \u2014 use gitlab_mr_dependencies_list to inspect existing dependencies")
	}
	return nil
}

// GetDependenciesInput defines parameters for listing merge request dependencies.
type GetDependenciesInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"merge_request_iid"     jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
}

// GetDependencies retrieves all dependencies (blockers) for a merge request.
func GetDependencies(ctx context.Context, client *gitlabclient.Client, input GetDependenciesInput) (DependenciesOutput, error) {
	if err := ctx.Err(); err != nil {
		return DependenciesOutput{}, err
	}
	if input.ProjectID == "" {
		return DependenciesOutput{}, errors.New("mrGetDependencies: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.MRIID <= 0 {
		return DependenciesOutput{}, toolutil.ErrRequiredInt64("mrGetDependencies", "merge_request_iid")
	}
	deps, _, err := client.GL().MergeRequests.GetMergeRequestDependencies(string(input.ProjectID), input.MRIID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return DependenciesOutput{}, toolutil.WrapErrWithHint("mrGetDependencies", err,
				"MR dependencies require GitLab Premium or Ultimate")
		}
		return DependenciesOutput{}, toolutil.WrapErrWithStatusHint("mrGetDependencies", err, http.StatusNotFound,
			hintVerifyMR)
	}
	out := make([]DependencyOutput, len(deps))
	for i := range deps {
		out[i] = dependencyToOutput(&deps[i])
	}
	return DependenciesOutput{Dependencies: out}, nil
}

// mergeStatusHints maps GitLab detailed_merge_status values to human-readable
// explanations with actionable next steps for the LLM.
var mergeStatusHints = map[string]string{ // #nosec G101 -- not credentials, these are GitLab merge status values
	"blocked_status":           "merge is blocked by another merge request that must be merged first",
	"broken_status":            "the source branch is broken (e.g. failed to compile). Fix the branch and push again",
	"checking":                 "GitLab is still checking mergeability. Wait a moment and retry",
	"ci_must_pass":             "a CI/CD pipeline must succeed before merge. Use auto_merge=true to merge automatically when the pipeline passes, or wait for the pipeline to finish",
	"ci_still_running":         "CI/CD pipeline is still running. Use auto_merge=true to merge automatically when the pipeline passes, or wait for completion",
	"conflict":                 "there are merge conflicts with the target branch. Rebase or resolve conflicts before merging (use gitlab_mr_rebase)",
	"discussions_not_resolved": "all threads/discussions must be resolved before merge. Resolve pending discussions first",
	"draft_status":             "the merge request is a draft. Mark it as ready (remove draft status) before merging",
	"external_status_checks":   "external status checks must pass before merge. Wait for all external checks to complete",
	"jira_association_missing": "the title or description must reference a Jira issue. Add a Jira issue key to the title or description",
	"need_rebase":              "the source branch needs to be rebased onto the target branch (use gitlab_mr_rebase)",
	"not_approved":             "the merge request has not received the required approvals. Request reviewers to approve it first",
	"not_open":                 "the merge request is not open (it may be closed or already merged). Only open MRs can be merged",
	"policies_denied":          "merge policies deny this merge request. Check project merge policies",
	"requested_changes":        "reviewers have requested changes. Address the requested changes and get re-approval",
	"unchecked":                "GitLab has not yet checked mergeability. Wait a moment and retry",
}

// diagnoseMergeBlocker builds a rich error message when a merge request cannot
// be merged (HTTP 405). It inspects the pre-fetched MR state to identify the
// exact blocker and suggests actionable next steps.
func diagnoseMergeBlocker(mrIID int64, mr *gl.MergeRequest, originalErr error) error {
	const op = "mrMerge"

	if mr == nil {
		return toolutil.WrapErrWithMessage(op, originalErr)
	}

	var reasons []string

	if hint, ok := mergeStatusHints[mr.DetailedMergeStatus]; ok && mr.DetailedMergeStatus != "mergeable" {
		reasons = append(reasons, hint)
	}

	// Supplement with field-level checks for cases where DetailedMergeStatus
	// may not be granular enough or is "unchecked"/"checking".
	if mr.Draft && !containsReason(reasons, "draft") {
		reasons = append(reasons, "the merge request is a draft")
	}
	if mr.HasConflicts && !containsReason(reasons, "conflict") {
		reasons = append(reasons, "there are merge conflicts")
	}
	if !mr.BlockingDiscussionsResolved && !containsReason(reasons, "discussion") {
		reasons = append(reasons, "unresolved blocking discussions")
	}
	if mr.State != "opened" && !containsReason(reasons, "not open") {
		reasons = append(reasons, fmt.Sprintf("merge request state is %q (must be opened)", mr.State))
	}
	if mr.MergeError != "" {
		reasons = append(reasons, "GitLab merge error: "+mr.MergeError)
	}

	if len(reasons) == 0 {
		return toolutil.WrapErrWithMessage(op, originalErr)
	}

	return fmt.Errorf("%s: merge request !%d cannot be merged — %s: %w",
		op, mrIID, strings.Join(reasons, "; "), originalErr)
}

// containsReason checks if any accumulated reason already mentions a keyword.
func containsReason(reasons []string, keyword string) bool {
	for _, r := range reasons {
		if strings.Contains(r, keyword) {
			return true
		}
	}
	return false
}
