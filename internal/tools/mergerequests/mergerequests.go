// Package mergerequests implements GitLab merge request CRUD operations including
// create, get, list (project/global/group), update, merge, approve, unapprove,
// commits, pipelines, delete, rebase, participants, reviewers, create-pipeline,
// issues-closed-on-merge, and cancel-auto-merge. It exposes typed input/output
// structs and handler functions registered as MCP tools.
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

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/pipelines"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// CreateInput defines parameters for creating a merge request.
type CreateInput struct {
	// Basic metadata
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	SourceBranch string               `json:"source_branch" jsonschema:"Source branch name,required"`
	TargetBranch string               `json:"target_branch" jsonschema:"Target branch name. If not specified by the user use the project default branch from gitlab_project_get (do NOT assume main),required"`
	Title        string               `json:"title" jsonschema:"Merge request title,required"`
	Description  string               `json:"description,omitempty" jsonschema:"Merge request description (Markdown supported)"`

	// Assignment and tracking
	AssigneeID  int64   `json:"assignee_id,omitempty" jsonschema:"Single user ID to assign (use assignee_ids for multiple)"`
	AssigneeIDs []int64 `json:"assignee_ids,omitempty" jsonschema:"User IDs to assign"`
	ReviewerIDs []int64 `json:"reviewer_ids,omitempty" jsonschema:"User IDs to add as reviewers"`
	Labels      string  `json:"labels,omitempty" jsonschema:"Comma-separated labels to apply"`
	MilestoneID int64   `json:"milestone_id,omitempty" jsonschema:"Milestone ID to associate with the merge request"`

	// Merge behavior
	RemoveSourceBranch *bool `json:"remove_source_branch,omitempty" jsonschema:"Delete source branch after merge. Only set if explicitly requested by the user. Omit to preserve repository defaults"`
	Squash             *bool `json:"squash,omitempty" jsonschema:"Squash commits on merge. Only set if explicitly requested by the user. Omit to preserve repository defaults"`
	AllowCollaboration *bool `json:"allow_collaboration,omitempty" jsonschema:"Allow commits from upstream members who can merge to target branch"`

	// Cross-project
	TargetProjectID int64 `json:"target_project_id,omitempty" jsonschema:"Target project ID (for cross-project/fork MRs)"`
}

// Output represents a merge request.
type Output struct {
	toolutil.HintableOutput
	ID                          int64           `json:"id"`
	IID                         int64           `json:"mr_iid"`
	ProjectID                   int64           `json:"project_id"`
	ProjectPath                 string          `json:"project_path,omitempty"`
	SourceProjectID             int64           `json:"source_project_id,omitempty"`
	TargetProjectID             int64           `json:"target_project_id,omitempty"`
	Title                       string          `json:"title"`
	Description                 string          `json:"description"`
	State                       string          `json:"state"`
	SourceBranch                string          `json:"source_branch"`
	TargetBranch                string          `json:"target_branch"`
	WebURL                      string          `json:"web_url"`
	MergeStatus                 string          `json:"merge_status"`
	Draft                       bool            `json:"draft"`
	HasConflicts                bool            `json:"has_conflicts"`
	BlockingDiscussionsResolved bool            `json:"blocking_discussions_resolved"`
	Squash                      bool            `json:"squash,omitempty"`
	SquashOnMerge               bool            `json:"squash_on_merge,omitempty"`
	MergeWhenPipelineSucceeds   bool            `json:"merge_when_pipeline_succeeds,omitempty"`
	ShouldRemoveSourceBranch    bool            `json:"should_remove_source_branch,omitempty"`
	DiscussionLocked            bool            `json:"discussion_locked"`
	RebaseInProgress            bool            `json:"rebase_in_progress,omitempty"`
	Author                      string          `json:"author,omitempty"`
	MergedBy                    string          `json:"merged_by,omitempty"`
	Assignees                   []string        `json:"assignees"`
	Reviewers                   []string        `json:"reviewers"`
	Labels                      []string        `json:"labels"`
	Milestone                   string          `json:"milestone,omitempty"`
	References                  string          `json:"references,omitempty"`
	SHA                         string          `json:"sha,omitempty"`
	MergeCommitSHA              string          `json:"merge_commit_sha,omitempty"`
	MergeError                  string          `json:"merge_error,omitempty"`
	ChangesCount                string          `json:"changes_count,omitempty"`
	DivergedCommitsCount        int64           `json:"diverged_commits_count,omitempty"`
	Upvotes                     int64           `json:"upvotes,omitempty"`
	Downvotes                   int64           `json:"downvotes,omitempty"`
	SquashCommitSHA             string          `json:"squash_commit_sha,omitempty"`
	ForceRemoveSourceBranch     bool            `json:"force_remove_source_branch,omitempty"`
	AllowCollaboration          bool            `json:"allow_collaboration,omitempty"`
	ClosedBy                    string          `json:"closed_by,omitempty"`
	MergeAfter                  string          `json:"merge_after,omitempty"`
	TaskCompletionCount         int64           `json:"task_completion_count,omitempty"`
	TaskCompletionTotal         int64           `json:"task_completion_total,omitempty"`
	TimeEstimate                int64           `json:"time_estimate,omitempty"`
	TotalTimeSpent              int64           `json:"total_time_spent,omitempty"`
	Subscribed                  bool            `json:"subscribed,omitempty"`
	FirstContribution           bool            `json:"first_contribution,omitempty"`
	DiffRefs                    *DiffRefsOutput `json:"diff_refs,omitempty"`
	PipelineID                  int64           `json:"pipeline_id,omitempty"`
	PipelineWebURL              string          `json:"pipeline_web_url,omitempty"`
	PipelineName                string          `json:"pipeline_name,omitempty"`
	HeadPipelineID              int64           `json:"head_pipeline_id,omitempty"`
	LatestBuildStartedAt        string          `json:"latest_build_started_at,omitempty"`
	LatestBuildFinishedAt       string          `json:"latest_build_finished_at,omitempty"`
	CreatedAt                   string          `json:"created_at"`
	UpdatedAt                   string          `json:"updated_at"`
	MergedAt                    string          `json:"merged_at,omitempty"`
	ClosedAt                    string          `json:"closed_at,omitempty"`
	PreparedAt                  string          `json:"prepared_at,omitempty"`
	UserNotesCount              int64           `json:"user_notes_count,omitempty"`
}

// DiffRefsOutput represents the diff refs (base, head, start SHAs) of a merge request.
type DiffRefsOutput struct {
	BaseSHA  string `json:"base_sha"`
	HeadSHA  string `json:"head_sha"`
	StartSHA string `json:"start_sha"`
}

// GetInput defines parameters for retrieving a merge request.
type GetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
}

// ListInput defines filters for listing merge requests.
type ListInput struct {
	ProjectID      toolutil.StringOrInt `json:"project_id"              jsonschema:"Project ID or URL-encoded path,required"`
	State          string               `json:"state,omitempty"         jsonschema:"Filter by state (opened, closed, merged, all)"`
	Labels         string               `json:"labels,omitempty"        jsonschema:"Comma-separated label names to filter by"`
	NotLabels      string               `json:"not_labels,omitempty"    jsonschema:"Comma-separated label names to exclude"`
	Milestone      string               `json:"milestone,omitempty"     jsonschema:"Milestone title to filter by"`
	Scope          string               `json:"scope,omitempty"         jsonschema:"Filter by scope (created_by_me, assigned_to_me, all)"`
	Search         string               `json:"search,omitempty"        jsonschema:"Search in title and description"`
	SourceBranch   string               `json:"source_branch,omitempty" jsonschema:"Filter by source branch name"`
	TargetBranch   string               `json:"target_branch,omitempty" jsonschema:"Filter by target branch name"`
	AuthorUsername string               `json:"author_username,omitempty" jsonschema:"Filter by author username"`
	Draft          *bool                `json:"draft,omitempty"         jsonschema:"Filter by draft status (true=only drafts, false=only non-drafts)"`
	IIDs           []int64              `json:"iids,omitempty"          jsonschema:"Filter by merge request internal IDs"`
	CreatedAfter   string               `json:"created_after,omitempty"  jsonschema:"Return MRs created after date (ISO 8601 format, e.g. 2025-01-01T00:00:00Z)"`
	CreatedBefore  string               `json:"created_before,omitempty" jsonschema:"Return MRs created before date (ISO 8601 format, e.g. 2025-12-31T23:59:59Z)"`
	UpdatedAfter   string               `json:"updated_after,omitempty"  jsonschema:"Return MRs updated after date (ISO 8601 format, e.g. 2025-01-01T00:00:00Z)"`
	UpdatedBefore  string               `json:"updated_before,omitempty" jsonschema:"Return MRs updated before date (ISO 8601 format, e.g. 2025-12-31T23:59:59Z)"`
	OrderBy        string               `json:"order_by,omitempty"      jsonschema:"Order by field (created_at, updated_at, title)"`
	Sort           string               `json:"sort,omitempty"          jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
}

// ListOutput holds a paginated list of merge requests.
type ListOutput struct {
	toolutil.HintableOutput
	MergeRequests []Output                  `json:"merge_requests"`
	Pagination    toolutil.PaginationOutput `json:"pagination"`
}

// UpdateInput defines parameters for updating a merge request.
type UpdateInput struct {
	ProjectID          toolutil.StringOrInt `json:"project_id"                    jsonschema:"Project ID or URL-encoded path,required"`
	MRIID              int64                `json:"mr_iid"                        jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	Title              string               `json:"title,omitempty"               jsonschema:"New title"`
	Description        string               `json:"description,omitempty"         jsonschema:"New description"`
	TargetBranch       string               `json:"target_branch,omitempty"       jsonschema:"New target branch"`
	AssigneeID         int64                `json:"assignee_id,omitempty"          jsonschema:"Single user ID to assign (use assignee_ids for multiple)"`
	AssigneeIDs        []int64              `json:"assignee_ids,omitempty"         jsonschema:"User IDs to assign as merge request assignees"`
	ReviewerIDs        []int64              `json:"reviewer_ids,omitempty"         jsonschema:"User IDs to add as reviewers"`
	Labels             string               `json:"labels,omitempty"               jsonschema:"Comma-separated label names to replace all labels on the merge request"`
	AddLabels          string               `json:"add_labels,omitempty"          jsonschema:"Comma-separated label names to add without removing existing"`
	RemoveLabels       string               `json:"remove_labels,omitempty"       jsonschema:"Comma-separated label names to remove"`
	MilestoneID        int64                `json:"milestone_id,omitempty"        jsonschema:"Milestone ID (0 to unset)"`
	RemoveSourceBranch *bool                `json:"remove_source_branch,omitempty" jsonschema:"Delete source branch after merge. Only set if explicitly requested"`
	Squash             *bool                `json:"squash,omitempty"              jsonschema:"Squash commits on merge. Only set if explicitly requested"`
	DiscussionLocked   *bool                `json:"discussion_locked,omitempty"   jsonschema:"Lock discussions on the merge request"`
	AllowCollaboration *bool                `json:"allow_collaboration,omitempty" jsonschema:"Allow commits from upstream members who can merge to target branch"`
	StateEvent         string               `json:"state_event,omitempty"         jsonschema:"State transition (close, reopen)"`
}

// MergeInput defines parameters for merging a merge request.
type MergeInput struct {
	ProjectID                toolutil.StringOrInt `json:"project_id"                              jsonschema:"Project ID or URL-encoded path,required"`
	MRIID                    int64                `json:"mr_iid"                                  jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	MergeCommitMessage       string               `json:"merge_commit_message,omitempty"           jsonschema:"Custom merge commit message"`
	Squash                   *bool                `json:"squash,omitempty"                         jsonschema:"Squash commits on merge. Only set if explicitly requested by the user. Omit to preserve repository defaults"`
	ShouldRemoveSourceBranch *bool                `json:"should_remove_source_branch,omitempty"   jsonschema:"Delete source branch after merge. Only set if explicitly requested by the user. Omit to preserve repository defaults"`
	AutoMerge                *bool                `json:"auto_merge,omitempty"                     jsonschema:"Automatically merge when pipeline succeeds (auto-merge)"`
	SHA                      string               `json:"sha,omitempty"                            jsonschema:"Head SHA of the merge request — merge only if HEAD matches (safety check)"`
	SquashCommitMessage      string               `json:"squash_commit_message,omitempty"          jsonschema:"Custom squash commit message (used when squash is enabled)"`
}

// ApproveInput defines parameters for approving a merge request.
type ApproveInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
}

// ApproveOutput holds the approval state after approve/unapprove.
type ApproveOutput struct {
	toolutil.HintableOutput
	ApprovalsRequired int  `json:"approvals_required"`
	ApprovedBy        int  `json:"approved_by_count"`
	Approved          bool `json:"approved"`
}

// ToOutput converts a GitLab API [gl.MergeRequest] to the MCP tool output format.
func ToOutput(m *gl.MergeRequest) Output {
	out := Output{
		ID:                          m.ID,
		IID:                         m.IID,
		ProjectID:                   m.ProjectID,
		Title:                       m.Title,
		Description:                 m.Description,
		State:                       m.State,
		SourceBranch:                m.SourceBranch,
		TargetBranch:                m.TargetBranch,
		WebURL:                      m.WebURL,
		MergeStatus:                 m.DetailedMergeStatus,
		Draft:                       m.Draft,
		HasConflicts:                m.HasConflicts,
		BlockingDiscussionsResolved: m.BlockingDiscussionsResolved,
		Squash:                      m.Squash,
		SHA:                         m.SHA,
		MergeCommitSHA:              m.MergeCommitSHA,
		MergeError:                  m.MergeError,
		ChangesCount:                m.ChangesCount,
		RebaseInProgress:            m.RebaseInProgress,
		DivergedCommitsCount:        m.DivergedCommitsCount,
		UserNotesCount:              m.UserNotesCount,
		Subscribed:                  m.Subscribed,
		FirstContribution:           m.FirstContribution,
	}
	if m.DiffRefs.BaseSha != "" || m.DiffRefs.HeadSha != "" || m.DiffRefs.StartSha != "" {
		out.DiffRefs = &DiffRefsOutput{
			BaseSHA:  m.DiffRefs.BaseSha,
			HeadSHA:  m.DiffRefs.HeadSha,
			StartSHA: m.DiffRefs.StartSha,
		}
	}
	if m.Pipeline != nil {
		out.PipelineID = m.Pipeline.ID
		out.PipelineWebURL = m.Pipeline.WebURL
		out.PipelineName = m.Pipeline.Name
	}
	if m.HeadPipeline != nil {
		out.HeadPipelineID = m.HeadPipeline.ID
	}
	if m.LatestBuildStartedAt != nil {
		out.LatestBuildStartedAt = m.LatestBuildStartedAt.Format(time.RFC3339)
	}
	if m.LatestBuildFinishedAt != nil {
		out.LatestBuildFinishedAt = m.LatestBuildFinishedAt.Format(time.RFC3339)
	}
	populatePeople(&out, &m.BasicMergeRequest)
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
		SourceBranch:                m.SourceBranch,
		TargetBranch:                m.TargetBranch,
		WebURL:                      m.WebURL,
		MergeStatus:                 m.DetailedMergeStatus,
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
// from a BasicMergeRequest into the Output.
func populatePeople(out *Output, m *gl.BasicMergeRequest) {
	out.SourceProjectID = m.SourceProjectID
	out.TargetProjectID = m.TargetProjectID
	out.DiscussionLocked = m.DiscussionLocked
	out.MergeWhenPipelineSucceeds = m.MergeWhenPipelineSucceeds
	out.ShouldRemoveSourceBranch = m.ShouldRemoveSourceBranch
	out.ForceRemoveSourceBranch = m.ForceRemoveSourceBranch
	out.AllowCollaboration = m.AllowCollaboration
	out.SquashOnMerge = m.SquashOnMerge
	out.SquashCommitSHA = m.SquashCommitSHA
	out.Upvotes = m.Upvotes
	out.Downvotes = m.Downvotes
	if m.Author != nil {
		out.Author = m.Author.Username
	}
	if m.MergeUser != nil {
		out.MergedBy = m.MergeUser.Username
	}
	if m.ClosedBy != nil {
		out.ClosedBy = m.ClosedBy.Username
	}
	for _, a := range m.Assignees {
		out.Assignees = append(out.Assignees, a.Username)
	}
	if out.Assignees == nil {
		out.Assignees = []string{}
	}
	for _, r := range m.Reviewers {
		out.Reviewers = append(out.Reviewers, r.Username)
	}
	if out.Reviewers == nil {
		out.Reviewers = []string{}
	}
	out.Labels = []string(m.Labels)
	if out.Labels == nil {
		out.Labels = []string{}
	}
	if m.Milestone != nil {
		out.Milestone = m.Milestone.Title
	}
	if m.TaskCompletionStatus != nil {
		out.TaskCompletionCount = m.TaskCompletionStatus.CompletedCount
		out.TaskCompletionTotal = m.TaskCompletionStatus.Count
	}
	if m.TimeStats != nil {
		out.TimeEstimate = m.TimeStats.TimeEstimate
		out.TotalTimeSpent = m.TimeStats.TotalTimeSpent
	}
	populateTimestamps(out, m)
}

// populateTimestamps extracts timestamps and references from a
// BasicMergeRequest into the Output.
func populateTimestamps(out *Output, m *gl.BasicMergeRequest) {
	if m.MergeAfter != nil {
		out.MergeAfter = m.MergeAfter.Format(time.RFC3339)
	}
	if m.CreatedAt != nil {
		out.CreatedAt = m.CreatedAt.Format(time.RFC3339)
	}
	if m.UpdatedAt != nil {
		out.UpdatedAt = m.UpdatedAt.Format(time.RFC3339)
	}
	if m.MergedAt != nil {
		out.MergedAt = m.MergedAt.Format(time.RFC3339)
	}
	if m.ClosedAt != nil {
		out.ClosedAt = m.ClosedAt.Format(time.RFC3339)
	}
	if m.PreparedAt != nil {
		out.PreparedAt = m.PreparedAt.Format(time.RFC3339)
	}
	if m.References != nil {
		out.References = m.References.Full
		if idx := strings.LastIndex(m.References.Full, "!"); idx > 0 {
			out.ProjectPath = m.References.Full[:idx]
		}
	}
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
	if input.Labels != "" {
		labels := gl.LabelOptions(strings.Split(input.Labels, ","))
		opts.Labels = &labels
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
		return Output{}, toolutil.ErrRequiredInt64("mrGet", "mr_iid")
	}
	mr, _, err := client.GL().MergeRequests.GetMergeRequest(string(input.ProjectID), input.MRIID, &gl.GetMergeRequestsOptions{}, gl.WithContext(ctx))
	if err != nil {
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
	opts := buildListOptions(input)
	mrs, resp, err := client.GL().MergeRequests.ListProjectMergeRequests(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("mrList", err)
	}
	out := make([]Output, len(mrs))
	for i, m := range mrs {
		out[i] = BasicToOutput(m)
	}
	return ListOutput{MergeRequests: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// buildListOptions maps ListInput fields to the GitLab API list options,
// applying only non-zero values so that unset filters are omitted.
func buildListOptions(input ListInput) *gl.ListProjectMergeRequestsOptions {
	opts := &gl.ListProjectMergeRequestsOptions{}
	if input.State != "" {
		opts.State = new(input.State)
	}
	if input.Labels != "" {
		labels := gl.LabelOptions(strings.Split(input.Labels, ","))
		opts.Labels = &labels
	}
	if input.Milestone != "" {
		opts.Milestone = new(input.Milestone)
	}
	if input.Scope != "" {
		opts.Scope = new(input.Scope)
	}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.SourceBranch != "" {
		opts.SourceBranch = new(input.SourceBranch)
	}
	if input.TargetBranch != "" {
		opts.TargetBranch = new(input.TargetBranch)
	}
	if input.AuthorUsername != "" {
		opts.AuthorUsername = new(input.AuthorUsername)
	}
	if input.Draft != nil {
		opts.Draft = input.Draft
	}
	if len(input.IIDs) > 0 {
		opts.IIDs = &input.IIDs
	}
	if input.NotLabels != "" {
		labels := gl.LabelOptions(strings.Split(input.NotLabels, ","))
		opts.NotLabels = &labels
	}
	opts.CreatedAfter = toolutil.ParseOptionalTime(input.CreatedAfter)
	opts.CreatedBefore = toolutil.ParseOptionalTime(input.CreatedBefore)
	opts.UpdatedAfter = toolutil.ParseOptionalTime(input.UpdatedAfter)
	opts.UpdatedBefore = toolutil.ParseOptionalTime(input.UpdatedBefore)
	if input.OrderBy != "" {
		opts.OrderBy = new(input.OrderBy)
	}
	if input.Sort != "" {
		opts.Sort = new(input.Sort)
	}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	return opts
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
	if input.Labels != "" {
		labels := gl.LabelOptions(strings.Split(input.Labels, ","))
		opts.Labels = &labels
	}
	if input.AddLabels != "" {
		labels := gl.LabelOptions(strings.Split(input.AddLabels, ","))
		opts.AddLabels = &labels
	}
	if input.RemoveLabels != "" {
		labels := gl.LabelOptions(strings.Split(input.RemoveLabels, ","))
		opts.RemoveLabels = &labels
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
		return Output{}, toolutil.ErrRequiredInt64("mrUpdate", "mr_iid")
	}
	opts := buildUpdateOpts(input)
	mr, _, err := client.GL().MergeRequests.UpdateMergeRequest(string(input.ProjectID), input.MRIID, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return Output{}, toolutil.WrapErrWithHint("mrUpdate", err,
				"verify project_id and mr_iid. Use gitlab_mr_list to check available MRs")
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
		return Output{}, toolutil.ErrRequiredInt64("mrMerge", "mr_iid")
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
	if input.SHA != "" {
		opts.SHA = new(input.SHA)
	}
	if input.SquashCommitMessage != "" {
		opts.SquashCommitMessage = new(input.SquashCommitMessage)
	}
	mr, resp, err := client.GL().MergeRequests.AcceptMergeRequest(string(input.ProjectID), input.MRIID, opts, gl.WithContext(ctx))
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusMethodNotAllowed && fetchErr == nil {
			return Output{}, diagnoseMergeBlocker("mrMerge", input.MRIID, prefetched, err)
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
		return ApproveOutput{}, toolutil.ErrRequiredInt64("mrApprove", "mr_iid")
	}
	approvals, _, err := client.GL().MergeRequestApprovals.ApproveMergeRequest(string(input.ProjectID), input.MRIID, &gl.ApproveMergeRequestOptions{}, gl.WithContext(ctx))
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
		return toolutil.ErrRequiredInt64("mrUnapprove", "mr_iid")
	}
	_, err := client.GL().MergeRequestApprovals.UnapproveMergeRequest(string(input.ProjectID), input.MRIID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("mrUnapprove", err)
	}
	return nil
}

// CommitsInput defines parameters for listing commits in a merge request.
type CommitsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	toolutil.PaginationInput
}

// CommitsOutput holds a paginated list of commits for a merge request.
type CommitsOutput struct {
	toolutil.HintableOutput
	Commits    []commits.Output          `json:"commits"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// Commits retrieves the list of commits in a merge request.
func Commits(ctx context.Context, client *gitlabclient.Client, input CommitsInput) (CommitsOutput, error) {
	if err := ctx.Err(); err != nil {
		return CommitsOutput{}, err
	}
	if input.ProjectID == "" {
		return CommitsOutput{}, errors.New("mrCommits: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.MRIID <= 0 {
		return CommitsOutput{}, toolutil.ErrRequiredInt64("mrCommits", "mr_iid")
	}

	opts := &gl.GetMergeRequestCommitsOptions{}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}

	commitList, resp, err := client.GL().MergeRequests.GetMergeRequestCommits(string(input.ProjectID), input.MRIID, opts, gl.WithContext(ctx))
	if err != nil {
		return CommitsOutput{}, toolutil.WrapErrWithMessage("mrCommits", err)
	}

	out := make([]commits.Output, len(commitList))
	for i, c := range commitList {
		out[i] = commits.ToOutput(c)
	}
	return CommitsOutput{Commits: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// PipelinesInput defines parameters for listing pipelines of a merge request.
type PipelinesInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
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
		return PipelinesOutput{}, toolutil.ErrRequiredInt64("mrPipelines", "mr_iid")
	}

	pipelineList, _, err := client.GL().MergeRequests.ListMergeRequestPipelines(string(input.ProjectID), input.MRIID, gl.WithContext(ctx))
	if err != nil {
		return PipelinesOutput{}, toolutil.WrapErrWithMessage("mrPipelines", err)
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
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request IID to delete (project-scoped, not 'merge_request_id'),required"`
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
		return toolutil.ErrRequiredInt64("mrDelete", "mr_iid")
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
	MRIID     int64                `json:"mr_iid"            jsonschema:"Merge request IID to rebase (project-scoped, not 'merge_request_id'),required"`
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
		return RebaseOutput{}, toolutil.ErrRequiredInt64("mrRebase", "mr_iid")
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
	State            string `json:"state,omitempty"           jsonschema:"Filter by state (opened, closed, merged, all)"`
	Labels           string `json:"labels,omitempty"          jsonschema:"Comma-separated label names to filter by"`
	NotLabels        string `json:"not_labels,omitempty"      jsonschema:"Comma-separated label names to exclude"`
	Milestone        string `json:"milestone,omitempty"       jsonschema:"Milestone title to filter by"`
	Scope            string `json:"scope,omitempty"           jsonschema:"Filter by scope (created_by_me, assigned_to_me, all)"`
	Search           string `json:"search,omitempty"          jsonschema:"Search in title and description"`
	SourceBranch     string `json:"source_branch,omitempty"   jsonschema:"Filter by source branch name"`
	TargetBranch     string `json:"target_branch,omitempty"   jsonschema:"Filter by target branch name"`
	AuthorUsername   string `json:"author_username,omitempty" jsonschema:"Filter by author username"`
	ReviewerUsername string `json:"reviewer_username,omitempty" jsonschema:"Filter by reviewer username"`
	Draft            *bool  `json:"draft,omitempty"           jsonschema:"Filter by draft status (true=only drafts, false=only non-drafts)"`
	CreatedAfter     string `json:"created_after,omitempty"   jsonschema:"Return MRs created after date (ISO 8601)"`
	CreatedBefore    string `json:"created_before,omitempty"  jsonschema:"Return MRs created before date (ISO 8601)"`
	UpdatedAfter     string `json:"updated_after,omitempty"   jsonschema:"Return MRs updated after date (ISO 8601)"`
	UpdatedBefore    string `json:"updated_before,omitempty"  jsonschema:"Return MRs updated before date (ISO 8601)"`
	OrderBy          string `json:"order_by,omitempty"        jsonschema:"Order by field (created_at, updated_at)"`
	Sort             string `json:"sort,omitempty"            jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
}

// ListGlobal returns a paginated list of merge requests across all projects
// visible to the authenticated user.
func ListGlobal(ctx context.Context, client *gitlabclient.Client, input ListGlobalInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	opts := buildGlobalListOptions(input)
	mrs, resp, err := client.GL().MergeRequests.ListMergeRequests(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("mrListGlobal", err)
	}
	out := make([]Output, len(mrs))
	for i, m := range mrs {
		out[i] = BasicToOutput(m)
	}
	return ListOutput{MergeRequests: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// buildGlobalListOptions maps ListGlobalInput to the GitLab API list options.
func buildGlobalListOptions(input ListGlobalInput) *gl.ListMergeRequestsOptions {
	opts := &gl.ListMergeRequestsOptions{}
	if input.State != "" {
		opts.State = new(input.State)
	}
	if input.Labels != "" {
		labels := gl.LabelOptions(strings.Split(input.Labels, ","))
		opts.Labels = &labels
	}
	if input.NotLabels != "" {
		labels := gl.LabelOptions(strings.Split(input.NotLabels, ","))
		opts.NotLabels = &labels
	}
	if input.Milestone != "" {
		opts.Milestone = new(input.Milestone)
	}
	if input.Scope != "" {
		opts.Scope = new(input.Scope)
	}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.SourceBranch != "" {
		opts.SourceBranch = new(input.SourceBranch)
	}
	if input.TargetBranch != "" {
		opts.TargetBranch = new(input.TargetBranch)
	}
	if input.AuthorUsername != "" {
		opts.AuthorUsername = new(input.AuthorUsername)
	}
	if input.ReviewerUsername != "" {
		opts.ReviewerUsername = new(input.ReviewerUsername)
	}
	if input.Draft != nil {
		opts.Draft = input.Draft
	}
	opts.CreatedAfter = toolutil.ParseOptionalTime(input.CreatedAfter)
	opts.CreatedBefore = toolutil.ParseOptionalTime(input.CreatedBefore)
	opts.UpdatedAfter = toolutil.ParseOptionalTime(input.UpdatedAfter)
	opts.UpdatedBefore = toolutil.ParseOptionalTime(input.UpdatedBefore)
	if input.OrderBy != "" {
		opts.OrderBy = new(input.OrderBy)
	}
	if input.Sort != "" {
		opts.Sort = new(input.Sort)
	}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	return opts
}

// ListGroupInput defines filters for listing merge requests in a group.
type ListGroupInput struct {
	GroupID          toolutil.StringOrInt `json:"group_id"                    jsonschema:"Group ID or URL-encoded path,required"`
	State            string               `json:"state,omitempty"             jsonschema:"Filter by state (opened, closed, merged, all)"`
	Labels           string               `json:"labels,omitempty"            jsonschema:"Comma-separated label names to filter by"`
	NotLabels        string               `json:"not_labels,omitempty"        jsonschema:"Comma-separated label names to exclude"`
	Milestone        string               `json:"milestone,omitempty"         jsonschema:"Milestone title to filter by"`
	Scope            string               `json:"scope,omitempty"             jsonschema:"Filter by scope (created_by_me, assigned_to_me, all)"`
	Search           string               `json:"search,omitempty"            jsonschema:"Search in title and description"`
	SourceBranch     string               `json:"source_branch,omitempty"     jsonschema:"Filter by source branch name"`
	TargetBranch     string               `json:"target_branch,omitempty"     jsonschema:"Filter by target branch name"`
	AuthorUsername   string               `json:"author_username,omitempty"   jsonschema:"Filter by author username"`
	ReviewerUsername string               `json:"reviewer_username,omitempty" jsonschema:"Filter by reviewer username"`
	Draft            *bool                `json:"draft,omitempty"             jsonschema:"Filter by draft status (true=only drafts, false=only non-drafts)"`
	CreatedAfter     string               `json:"created_after,omitempty"     jsonschema:"Return MRs created after date (ISO 8601)"`
	CreatedBefore    string               `json:"created_before,omitempty"    jsonschema:"Return MRs created before date (ISO 8601)"`
	UpdatedAfter     string               `json:"updated_after,omitempty"     jsonschema:"Return MRs updated after date (ISO 8601)"`
	UpdatedBefore    string               `json:"updated_before,omitempty"    jsonschema:"Return MRs updated before date (ISO 8601)"`
	OrderBy          string               `json:"order_by,omitempty"          jsonschema:"Order by field (created_at, updated_at)"`
	Sort             string               `json:"sort,omitempty"              jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
}

// ListGroup returns a paginated list of merge requests in a group.
func ListGroup(ctx context.Context, client *gitlabclient.Client, input ListGroupInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.GroupID == "" {
		return ListOutput{}, errors.New("mrListGroup: group_id is required")
	}
	opts := buildGroupListOptions(input)
	mrs, resp, err := client.GL().MergeRequests.ListGroupMergeRequests(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("mrListGroup", err)
	}
	out := make([]Output, len(mrs))
	for i, m := range mrs {
		out[i] = BasicToOutput(m)
	}
	return ListOutput{MergeRequests: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// buildGroupListOptions maps ListGroupInput to the GitLab API list options.
func buildGroupListOptions(input ListGroupInput) *gl.ListGroupMergeRequestsOptions {
	opts := &gl.ListGroupMergeRequestsOptions{}
	if input.State != "" {
		opts.State = new(input.State)
	}
	if input.Labels != "" {
		labels := gl.LabelOptions(strings.Split(input.Labels, ","))
		opts.Labels = &labels
	}
	if input.NotLabels != "" {
		labels := gl.LabelOptions(strings.Split(input.NotLabels, ","))
		opts.NotLabels = &labels
	}
	if input.Milestone != "" {
		opts.Milestone = new(input.Milestone)
	}
	if input.Scope != "" {
		opts.Scope = new(input.Scope)
	}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.SourceBranch != "" {
		opts.SourceBranch = new(input.SourceBranch)
	}
	if input.TargetBranch != "" {
		opts.TargetBranch = new(input.TargetBranch)
	}
	if input.AuthorUsername != "" {
		opts.AuthorUsername = new(input.AuthorUsername)
	}
	if input.ReviewerUsername != "" {
		opts.ReviewerUsername = new(input.ReviewerUsername)
	}
	if input.Draft != nil {
		opts.Draft = input.Draft
	}
	opts.CreatedAfter = toolutil.ParseOptionalTime(input.CreatedAfter)
	opts.CreatedBefore = toolutil.ParseOptionalTime(input.CreatedBefore)
	opts.UpdatedAfter = toolutil.ParseOptionalTime(input.UpdatedAfter)
	opts.UpdatedBefore = toolutil.ParseOptionalTime(input.UpdatedBefore)
	if input.OrderBy != "" {
		opts.OrderBy = new(input.OrderBy)
	}
	if input.Sort != "" {
		opts.Sort = new(input.Sort)
	}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	return opts
}

// ---------------------------------------------------------------------------
// Participants & Reviewers
// ---------------------------------------------------------------------------.

// ParticipantsInput defines parameters for listing MR participants.
type ParticipantsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
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
		return ParticipantsOutput{}, toolutil.ErrRequiredInt64("mrParticipants", "mr_iid")
	}
	users, _, err := client.GL().MergeRequests.GetMergeRequestParticipants(string(input.ProjectID), input.MRIID, gl.WithContext(ctx))
	if err != nil {
		return ParticipantsOutput{}, toolutil.WrapErrWithMessage("mrParticipants", err)
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
		return ReviewersOutput{}, toolutil.ErrRequiredInt64("mrReviewers", "mr_iid")
	}
	reviewers, _, err := client.GL().MergeRequests.GetMergeRequestReviewers(string(input.ProjectID), input.MRIID, gl.WithContext(ctx))
	if err != nil {
		return ReviewersOutput{}, toolutil.WrapErrWithMessage("mrReviewers", err)
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
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
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
		return pipelines.Output{}, toolutil.ErrRequiredInt64("mrCreatePipeline", "mr_iid")
	}
	pi, _, err := client.GL().MergeRequests.CreateMergeRequestPipeline(string(input.ProjectID), input.MRIID, gl.WithContext(ctx))
	if err != nil {
		return pipelines.Output{}, toolutil.WrapErrWithMessage("mrCreatePipeline", err)
	}
	return pipelines.ToOutput(pi), nil
}

// ---------------------------------------------------------------------------
// Issues closed on merge
// ---------------------------------------------------------------------------.

// IssuesClosedInput defines parameters for listing issues that close on merge.
type IssuesClosedInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	toolutil.PaginationInput
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
	if err := ctx.Err(); err != nil {
		return IssuesClosedOutput{}, err
	}
	if input.ProjectID == "" {
		return IssuesClosedOutput{}, errors.New("mrIssuesClosed: project_id is required")
	}
	if input.MRIID <= 0 {
		return IssuesClosedOutput{}, toolutil.ErrRequiredInt64("mrIssuesClosed", "mr_iid")
	}
	opts := &gl.GetIssuesClosedOnMergeOptions{}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	issueList, resp, err := client.GL().MergeRequests.GetIssuesClosedOnMerge(string(input.ProjectID), input.MRIID, opts, gl.WithContext(ctx))
	if err != nil {
		return IssuesClosedOutput{}, toolutil.WrapErrWithMessage("mrIssuesClosed", err)
	}
	out := make([]issues.Output, len(issueList))
	for i, issue := range issueList {
		out[i] = issues.ToOutput(issue)
	}
	return IssuesClosedOutput{Issues: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
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
		return Output{}, toolutil.ErrRequiredInt64("mrCancelAutoMerge", "mr_iid")
	}
	mr, _, err := client.GL().MergeRequests.CancelMergeWhenPipelineSucceeds(string(input.ProjectID), input.MRIID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusMethodNotAllowed) || toolutil.IsHTTPStatus(err, http.StatusNotAcceptable) {
			return Output{}, toolutil.WrapErrWithHint("mrCancelAutoMerge", err,
				"the MR may already be merged/closed, or auto-merge was not enabled. Use gitlab_mr_get to check state and auto_merge_enabled")
		}
		return Output{}, toolutil.WrapErrWithMessage("mrCancelAutoMerge", err)
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
		return Output{}, toolutil.ErrRequiredInt64("mrSubscribe", "mr_iid")
	}
	mr, _, err := client.GL().MergeRequests.SubscribeToMergeRequest(string(input.ProjectID), input.MRIID, gl.WithContext(ctx))
	if err != nil {
		// GitLab returns 304 Not Modified with empty body when already subscribed,
		// which causes EOF during JSON decode. Fall back to Get.
		if errors.Is(err, io.EOF) || toolutil.IsHTTPStatus(err, http.StatusNotModified) {
			return Get(ctx, client, input)
		}
		return Output{}, toolutil.WrapErrWithMessage("mrSubscribe", err)
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
		return Output{}, toolutil.ErrRequiredInt64("mrUnsubscribe", "mr_iid")
	}
	mr, _, err := client.GL().MergeRequests.UnsubscribeFromMergeRequest(string(input.ProjectID), input.MRIID, gl.WithContext(ctx))
	if err != nil {
		if errors.Is(err, io.EOF) || toolutil.IsHTTPStatus(err, http.StatusNotModified) {
			return Get(ctx, client, input)
		}
		return Output{}, toolutil.WrapErrWithMessage("mrUnsubscribe", err)
	}
	return ToOutput(mr), nil
}

// ---------------------------------------------------------------------------
// Time Tracking
// ---------------------------------------------------------------------------.

// TimeStatsOutput represents time tracking statistics for a merge request.
type TimeStatsOutput struct {
	toolutil.HintableOutput
	HumanTimeEstimate   string `json:"human_time_estimate"`
	HumanTotalTimeSpent string `json:"human_total_time_spent"`
	TimeEstimate        int64  `json:"time_estimate"`
	TotalTimeSpent      int64  `json:"total_time_spent"`
}

// timeStatsToOutput converts the GitLab API response to the tool output format.
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
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
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
		return TimeStatsOutput{}, toolutil.ErrRequiredInt64("mrSetTimeEstimate", "mr_iid")
	}
	if input.Duration == "" {
		return TimeStatsOutput{}, errors.New("mrSetTimeEstimate: duration is required")
	}
	ts, _, err := client.GL().MergeRequests.SetTimeEstimate(string(input.ProjectID), input.MRIID,
		&gl.SetTimeEstimateOptions{Duration: new(input.Duration)}, gl.WithContext(ctx))
	if err != nil {
		return TimeStatsOutput{}, toolutil.WrapErrWithMessage("mrSetTimeEstimate", err)
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
		return TimeStatsOutput{}, toolutil.ErrRequiredInt64("mrResetTimeEstimate", "mr_iid")
	}
	ts, _, err := client.GL().MergeRequests.ResetTimeEstimate(string(input.ProjectID), input.MRIID, gl.WithContext(ctx))
	if err != nil {
		return TimeStatsOutput{}, toolutil.WrapErrWithMessage("mrResetTimeEstimate", err)
	}
	return timeStatsToOutput(ts), nil
}

// AddSpentTimeInput defines parameters for adding spent time to an MR.
type AddSpentTimeInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
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
		return TimeStatsOutput{}, toolutil.ErrRequiredInt64("mrAddSpentTime", "mr_iid")
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
		return TimeStatsOutput{}, toolutil.WrapErrWithMessage("mrAddSpentTime", err)
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
		return TimeStatsOutput{}, toolutil.ErrRequiredInt64("mrResetSpentTime", "mr_iid")
	}
	ts, _, err := client.GL().MergeRequests.ResetSpentTime(string(input.ProjectID), input.MRIID, gl.WithContext(ctx))
	if err != nil {
		return TimeStatsOutput{}, toolutil.WrapErrWithMessage("mrResetSpentTime", err)
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
		return TimeStatsOutput{}, toolutil.ErrRequiredInt64("mrGetTimeStats", "mr_iid")
	}
	ts, _, err := client.GL().MergeRequests.GetTimeSpent(string(input.ProjectID), input.MRIID, gl.WithContext(ctx))
	if err != nil {
		return TimeStatsOutput{}, toolutil.WrapErrWithMessage("mrGetTimeStats", err)
	}
	return timeStatsToOutput(ts), nil
}

// ---------------------------------------------------------------------------
// Related Issues
// ---------------------------------------------------------------------------.

// RelatedIssuesInput defines parameters for listing issues related to an MR.
type RelatedIssuesInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	toolutil.PaginationInput
}

// RelatedIssuesOutput holds the list of issues related to a merge request.
type RelatedIssuesOutput struct {
	toolutil.HintableOutput
	Issues     []issues.Output           `json:"issues"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// RelatedIssues retrieves the list of issues related to a merge request.
func RelatedIssues(ctx context.Context, client *gitlabclient.Client, input RelatedIssuesInput) (RelatedIssuesOutput, error) {
	if err := ctx.Err(); err != nil {
		return RelatedIssuesOutput{}, err
	}
	if input.ProjectID == "" {
		return RelatedIssuesOutput{}, errors.New("mrRelatedIssues: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.MRIID <= 0 {
		return RelatedIssuesOutput{}, toolutil.ErrRequiredInt64("mrRelatedIssues", "mr_iid")
	}
	opts := &gl.ListRelatedIssuesOptions{}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	issueList, resp, err := client.GL().MergeRequests.ListRelatedIssues(string(input.ProjectID), input.MRIID, opts, gl.WithContext(ctx))
	if err != nil {
		return RelatedIssuesOutput{}, toolutil.WrapErrWithMessage("mrRelatedIssues", err)
	}
	out := make([]issues.Output, len(issueList))
	for i, issue := range issueList {
		out[i] = issues.ToOutput(issue)
	}
	return RelatedIssuesOutput{Issues: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// ---------------------------------------------------------------------------
// Create To-Do for MR
// ---------------------------------------------------------------------------.

// CreateTodoInput defines parameters for creating a to-do item on a merge request.
type CreateTodoInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
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
		return CreateTodoOutput{}, toolutil.ErrRequiredInt64("mrCreateTodo", "mr_iid")
	}
	todo, _, err := client.GL().MergeRequests.CreateTodo(string(input.ProjectID), input.MRIID, gl.WithContext(ctx))
	if err != nil {
		return CreateTodoOutput{}, toolutil.WrapErrWithMessage("mrCreateTodo", err)
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
	MRIID                  int64                `json:"mr_iid"                    jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
	BlockingMergeRequestID int64                `json:"blocking_merge_request_id" jsonschema:"ID of the merge request that blocks this one"`
}

// DependencyOutput represents a merge request dependency.
type DependencyOutput struct {
	toolutil.HintableOutput
	ID                   int64  `json:"id"`
	BlockingMRID         int64  `json:"blocking_mr_id"`
	BlockingMRIID        int64  `json:"blocking_mr_iid"`
	BlockingMRTitle      string `json:"blocking_mr_title"`
	BlockingMRState      string `json:"blocking_mr_state"`
	BlockingMRProjectID  int64  `json:"blocking_mr_project_id"`
	BlockingSourceBranch string `json:"blocking_source_branch"`
	BlockingTargetBranch string `json:"blocking_target_branch"`
	ProjectID            int64  `json:"project_id"`
}

// DependenciesOutput holds a list of merge request dependencies.
type DependenciesOutput struct {
	toolutil.HintableOutput
	Dependencies []DependencyOutput `json:"dependencies"`
}

// dependencyToOutput converts the GitLab API response to the tool output format.
func dependencyToOutput(d *gl.MergeRequestDependency) DependencyOutput {
	out := DependencyOutput{
		ID:        d.ID,
		ProjectID: d.ProjectID,
	}
	out.BlockingMRID = d.BlockingMergeRequest.ID
	out.BlockingMRIID = d.BlockingMergeRequest.Iid
	out.BlockingMRTitle = d.BlockingMergeRequest.Title
	out.BlockingMRState = d.BlockingMergeRequest.State
	out.BlockingMRProjectID = d.BlockingMergeRequest.ProjectID
	out.BlockingSourceBranch = d.BlockingMergeRequest.SourceBranch
	out.BlockingTargetBranch = d.BlockingMergeRequest.TargetBranch
	return out
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
		return DependencyOutput{}, toolutil.ErrRequiredInt64("mrCreateDependency", "mr_iid")
	}
	dep, _, err := client.GL().MergeRequests.CreateMergeRequestDependency(string(input.ProjectID), input.MRIID,
		gl.CreateMergeRequestDependencyOptions{BlockingMergeRequestID: new(input.BlockingMergeRequestID)}, gl.WithContext(ctx))
	if err != nil {
		return DependencyOutput{}, toolutil.WrapErrWithMessage("mrCreateDependency", err)
	}
	return dependencyToOutput(dep), nil
}

// DeleteDependencyInput defines parameters for deleting a merge request dependency.
type DeleteDependencyInput struct {
	ProjectID              toolutil.StringOrInt `json:"project_id"                jsonschema:"Project ID or URL-encoded path,required"`
	MRIID                  int64                `json:"mr_iid"                    jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
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
		return toolutil.ErrRequiredInt64("mrDeleteDependency", "mr_iid")
	}
	_, err := client.GL().MergeRequests.DeleteMergeRequestDependency(string(input.ProjectID), input.MRIID, input.BlockingMergeRequestID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("mrDeleteDependency", err)
	}
	return nil
}

// GetDependenciesInput defines parameters for listing merge request dependencies.
type GetDependenciesInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request IID (project-scoped, not 'merge_request_id'),required"`
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
		return DependenciesOutput{}, toolutil.ErrRequiredInt64("mrGetDependencies", "mr_iid")
	}
	deps, _, err := client.GL().MergeRequests.GetMergeRequestDependencies(string(input.ProjectID), input.MRIID, gl.WithContext(ctx))
	if err != nil {
		return DependenciesOutput{}, toolutil.WrapErrWithMessage("mrGetDependencies", err)
	}
	out := make([]DependencyOutput, len(deps))
	for i := range deps {
		out[i] = dependencyToOutput(&deps[i])
	}
	return DependenciesOutput{Dependencies: out}, nil
}

// mergeStatusHints maps GitLab detailed_merge_status values to human-readable
// explanations with actionable next steps for the LLM.
var mergeStatusHints = map[string]string{ //nolint:gosec // not credentials, these are GitLab merge status values
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
func diagnoseMergeBlocker(op string, mrIID int64, mr *gl.MergeRequest, originalErr error) error {
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
		reasons = append(reasons, fmt.Sprintf("GitLab merge error: %s", mr.MergeError))
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
