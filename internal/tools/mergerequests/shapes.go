package mergerequests

import (
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical output shapes mirrored from client-go sub-objects. Per the 1:1
// audit policy (full nested objects) these surface every field of the SDK
// struct and are replicated here as local types rather than imported from
// sibling packages to preserve the zero-import-cycle constraint (C-IMPORTS).
//
// This file covers the merge-request sub-objects surfaced on the canonical
// json keys: author/assignee/assignees/reviewers/merge_user/closed_by/merged_by
// (BasicUserOutput), milestone (MilestoneOutput), references (ReferencesOutput),
// label_details (LabelDetailsOutput), task_completion_status
// (TaskCompletionStatusOutput), head_pipeline (PipelineOutput), pipeline
// (PipelineInfoOutput), user (MergeRequestUserOutput), and diff_refs
// (DiffRefsOutput, defined in mergerequests.go).

// BasicUserOutput mirrors gl.BasicUser, the compact user object embedded in
// merge-request payloads (author, assignee, assignees, reviewers, merge_user,
// closed_by, merged_by).
type BasicUserOutput struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	State     string `json:"state"`
	AvatarURL string `json:"avatar_url"`
	WebURL    string `json:"web_url"`
	CreatedAt string `json:"created_at,omitempty"`
}

// basicUserOutput converts a single gl.BasicUser to its output shape, returning
// nil when the SDK value is nil.
func basicUserOutput(u *gl.BasicUser) *BasicUserOutput {
	if u == nil {
		return nil
	}
	return &BasicUserOutput{
		ID: u.ID, Username: u.Username, Name: u.Name, State: u.State,
		AvatarURL: u.AvatarURL, WebURL: u.WebURL, CreatedAt: toolutil.FormatTimePtr(u.CreatedAt),
	}
}

// basicUserOutputs converts a slice of gl.BasicUser, skipping nil elements and
// returning nil for an empty or all-nil slice.
func basicUserOutputs(users []*gl.BasicUser) []*BasicUserOutput {
	if len(users) == 0 {
		return nil
	}
	out := make([]*BasicUserOutput, 0, len(users))
	for _, u := range users {
		if u == nil {
			continue
		}
		out = append(out, basicUserOutput(u))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// MilestoneOutput mirrors gl.Milestone (the merge-request milestone object).
type MilestoneOutput struct {
	ID          int64  `json:"id"`
	IID         int64  `json:"iid"`
	GroupID     int64  `json:"group_id"`
	ProjectID   int64  `json:"project_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	State       string `json:"state"`
	WebURL      string `json:"web_url"`
	StartDate   string `json:"start_date,omitempty"`
	DueDate     string `json:"due_date,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
	Expired     *bool  `json:"expired,omitempty"`
}

func milestoneOutput(m *gl.Milestone) *MilestoneOutput {
	if m == nil {
		return nil
	}
	return &MilestoneOutput{
		ID: m.ID, IID: m.IID, GroupID: m.GroupID, ProjectID: m.ProjectID,
		Title: m.Title, Description: m.Description, State: m.State, WebURL: m.WebURL,
		StartDate: toolutil.FormatISOTimePtr(m.StartDate), DueDate: toolutil.FormatISOTimePtr(m.DueDate),
		CreatedAt: toolutil.FormatTimePtr(m.CreatedAt), UpdatedAt: toolutil.FormatTimePtr(m.UpdatedAt),
		Expired: m.Expired,
	}
}

// ReferencesOutput mirrors gl.IssueReferences (the merge-request references object).
type ReferencesOutput struct {
	Short    string `json:"short"`
	Relative string `json:"relative"`
	Full     string `json:"full"`
}

func referencesOutput(r *gl.IssueReferences) *ReferencesOutput {
	if r == nil {
		return nil
	}
	return &ReferencesOutput{Short: r.Short, Relative: r.Relative, Full: r.Full}
}

// LabelDetailsOutput mirrors gl.LabelDetails.
type LabelDetailsOutput struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	Color           string `json:"color"`
	Description     string `json:"description"`
	DescriptionHTML string `json:"description_html"`
	TextColor       string `json:"text_color"`
}

func labelDetailsOutputs(details []*gl.LabelDetails) []*LabelDetailsOutput {
	if len(details) == 0 {
		return nil
	}
	out := make([]*LabelDetailsOutput, 0, len(details))
	for _, d := range details {
		if d == nil {
			continue
		}
		out = append(out, &LabelDetailsOutput{
			ID: d.ID, Name: d.Name, Color: d.Color, Description: d.Description,
			DescriptionHTML: d.DescriptionHTML, TextColor: d.TextColor,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// TaskCompletionStatusOutput mirrors gl.TasksCompletionStatus.
type TaskCompletionStatusOutput struct {
	Count          int64 `json:"count"`
	CompletedCount int64 `json:"completed_count"`
}

func taskCompletionStatusOutput(t *gl.TasksCompletionStatus) *TaskCompletionStatusOutput {
	if t == nil {
		return nil
	}
	return &TaskCompletionStatusOutput{Count: t.Count, CompletedCount: t.CompletedCount}
}

// timeStatsPtr converts gl.TimeStats to the package TimeStatsOutput pointer,
// reusing the standalone TimeStatsOutput type, or nil when the SDK value is nil.
func timeStatsPtr(t *gl.TimeStats) *TimeStatsOutput {
	if t == nil {
		return nil
	}
	ts := timeStatsToOutput(t)
	return &ts
}

// MergeRequestUserOutput mirrors gl.MergeRequestUser (the MR "user" object,
// describing the current user's relationship to the merge request).
type MergeRequestUserOutput struct {
	CanMerge bool `json:"can_merge"`
}

func mergeRequestUserOutput(u gl.MergeRequestUser) *MergeRequestUserOutput {
	return &MergeRequestUserOutput{CanMerge: u.CanMerge}
}

// PipelineInfoOutput mirrors gl.PipelineInfo (the compact pipeline object on the
// merge-request "pipeline" key).
type PipelineInfoOutput struct {
	ID        int64  `json:"id"`
	IID       int64  `json:"iid"`
	ProjectID int64  `json:"project_id"`
	Status    string `json:"status"`
	Source    string `json:"source"`
	Ref       string `json:"ref"`
	SHA       string `json:"sha"`
	Name      string `json:"name"`
	WebURL    string `json:"web_url"`
	UpdatedAt string `json:"updated_at,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

func pipelineInfoOutput(p *gl.PipelineInfo) *PipelineInfoOutput {
	if p == nil {
		return nil
	}
	return &PipelineInfoOutput{
		ID: p.ID, IID: p.IID, ProjectID: p.ProjectID, Status: p.Status,
		Source: p.Source, Ref: p.Ref, SHA: p.SHA, Name: p.Name, WebURL: p.WebURL,
		UpdatedAt: toolutil.FormatTimePtr(p.UpdatedAt), CreatedAt: toolutil.FormatTimePtr(p.CreatedAt),
	}
}

// PipelineDetailedStatusOutput mirrors gl.DetailedStatus (the head_pipeline
// detailed_status object).
type PipelineDetailedStatusOutput struct {
	Icon         string                                    `json:"icon"`
	Text         string                                    `json:"text"`
	Label        string                                    `json:"label"`
	Group        string                                    `json:"group"`
	Tooltip      string                                    `json:"tooltip"`
	HasDetails   bool                                      `json:"has_details"`
	DetailsPath  string                                    `json:"details_path"`
	Illustration *PipelineDetailedStatusIllustrationOutput `json:"illustration,omitempty"`
	Favicon      string                                    `json:"favicon"`
}

// PipelineDetailedStatusIllustrationOutput mirrors gl.DetailedStatusIllustration
// (the detailed_status.illustration object).
type PipelineDetailedStatusIllustrationOutput struct {
	Image string `json:"image"`
}

func pipelineDetailedStatusOutput(s *gl.DetailedStatus) *PipelineDetailedStatusOutput {
	if s == nil {
		return nil
	}
	out := &PipelineDetailedStatusOutput{
		Icon: s.Icon, Text: s.Text, Label: s.Label, Group: s.Group,
		Tooltip: s.Tooltip, HasDetails: s.HasDetails, DetailsPath: s.DetailsPath, Favicon: s.Favicon,
	}
	if s.Illustration.Image != "" {
		out.Illustration = &PipelineDetailedStatusIllustrationOutput{Image: s.Illustration.Image}
	}
	return out
}

// PipelineOutput mirrors gl.Pipeline (the full head_pipeline object).
type PipelineOutput struct {
	ID             int64                         `json:"id"`
	IID            int64                         `json:"iid"`
	ProjectID      int64                         `json:"project_id"`
	Status         string                        `json:"status"`
	Source         string                        `json:"source"`
	Ref            string                        `json:"ref"`
	Name           string                        `json:"name"`
	SHA            string                        `json:"sha"`
	BeforeSHA      string                        `json:"before_sha"`
	Tag            bool                          `json:"tag"`
	YamlErrors     string                        `json:"yaml_errors"`
	User           *BasicUserOutput              `json:"user,omitempty"`
	UpdatedAt      string                        `json:"updated_at,omitempty"`
	CreatedAt      string                        `json:"created_at,omitempty"`
	StartedAt      string                        `json:"started_at,omitempty"`
	FinishedAt     string                        `json:"finished_at,omitempty"`
	CommittedAt    string                        `json:"committed_at,omitempty"`
	Duration       int64                         `json:"duration"`
	QueuedDuration int64                         `json:"queued_duration"`
	Coverage       string                        `json:"coverage"`
	WebURL         string                        `json:"web_url"`
	DetailedStatus *PipelineDetailedStatusOutput `json:"detailed_status,omitempty"`
}

func pipelineOutput(p *gl.Pipeline) *PipelineOutput {
	if p == nil {
		return nil
	}
	return &PipelineOutput{
		ID: p.ID, IID: p.IID, ProjectID: p.ProjectID, Status: p.Status,
		Source: string(p.Source), Ref: p.Ref, Name: p.Name, SHA: p.SHA, BeforeSHA: p.BeforeSHA,
		Tag: p.Tag, YamlErrors: p.YamlErrors, User: basicUserOutput(p.User),
		UpdatedAt: toolutil.FormatTimePtr(p.UpdatedAt), CreatedAt: toolutil.FormatTimePtr(p.CreatedAt),
		StartedAt: toolutil.FormatTimePtr(p.StartedAt), FinishedAt: toolutil.FormatTimePtr(p.FinishedAt),
		CommittedAt: toolutil.FormatTimePtr(p.CommittedAt), Duration: p.Duration, QueuedDuration: p.QueuedDuration,
		Coverage: p.Coverage, WebURL: p.WebURL, DetailedStatus: pipelineDetailedStatusOutput(p.DetailedStatus),
	}
}

// BlockingMergeRequestOutput mirrors gl.BlockingMergeRequest (the
// blocking_merge_request object on a merge-request dependency). It is a
// BasicMergeRequest-shaped object surfaced 1:1 with the SDK field set.
type BlockingMergeRequestOutput struct {
	ID                          int64                       `json:"id"`
	IID                         int64                       `json:"iid"`
	TargetBranch                string                      `json:"target_branch"`
	SourceBranch                string                      `json:"source_branch"`
	ProjectID                   int64                       `json:"project_id"`
	Title                       string                      `json:"title"`
	State                       string                      `json:"state"`
	CreatedAt                   string                      `json:"created_at,omitempty"`
	UpdatedAt                   string                      `json:"updated_at,omitempty"`
	Upvotes                     int64                       `json:"upvotes"`
	Downvotes                   int64                       `json:"downvotes"`
	Author                      *BasicUserOutput            `json:"author,omitempty"`
	Assignee                    *BasicUserOutput            `json:"assignee,omitempty"`
	Assignees                   []*BasicUserOutput          `json:"assignees,omitempty"`
	Reviewers                   []*BasicUserOutput          `json:"reviewers,omitempty"`
	SourceProjectID             int64                       `json:"source_project_id"`
	TargetProjectID             int64                       `json:"target_project_id"`
	Labels                      []string                    `json:"labels"`
	Description                 string                      `json:"description"`
	Draft                       bool                        `json:"draft"`
	Milestone                   string                      `json:"milestone,omitempty"`
	AutoMerge                   bool                        `json:"auto_merge"`
	DetailedMergeStatus         string                      `json:"detailed_merge_status"`
	MergedAt                    string                      `json:"merged_at,omitempty"`
	ClosedBy                    *BasicUserOutput            `json:"closed_by,omitempty"`
	ClosedAt                    string                      `json:"closed_at,omitempty"`
	SHA                         string                      `json:"sha"`
	MergeCommitSHA              string                      `json:"merge_commit_sha"`
	SquashCommitSHA             string                      `json:"squash_commit_sha"`
	UserNotesCount              int64                       `json:"user_notes_count"`
	ShouldRemoveSourceBranch    *bool                       `json:"should_remove_source_branch,omitempty"`
	ForceRemoveSourceBranch     bool                        `json:"force_remove_source_branch"`
	WebURL                      string                      `json:"web_url"`
	References                  *ReferencesOutput           `json:"references,omitempty"`
	DiscussionLocked            *bool                       `json:"discussion_locked,omitempty"`
	TimeStats                   *TimeStatsOutput            `json:"time_stats,omitempty"`
	Squash                      bool                        `json:"squash"`
	TaskCompletionStatus        *TaskCompletionStatusOutput `json:"task_completion_status,omitempty"`
	HasConflicts                bool                        `json:"has_conflicts"`
	BlockingDiscussionsResolved bool                        `json:"blocking_discussions_resolved"`
	MergeUser                   *BasicUserOutput            `json:"merge_user,omitempty"`
	MergeAfter                  string                      `json:"merge_after,omitempty"`
	Imported                    bool                        `json:"imported"`
	ImportedFrom                string                      `json:"imported_from"`
	PreparedAt                  string                      `json:"prepared_at,omitempty"`
	SquashOnMerge               bool                        `json:"squash_on_merge"`
	WorkInProgress              bool                        `json:"work_in_progress"`
	MergeWhenPipelineSucceeds   bool                        `json:"merge_when_pipeline_succeeds"`
	MergedBy                    *BasicUserOutput            `json:"merged_by,omitempty"`
	ApprovalsBeforeMerge        *int64                      `json:"approvals_before_merge,omitempty" tier:"premium"`
	Reference                   string                      `json:"reference"`
	MergeStatus                 string                      `json:"merge_status"`
}

// blockingMergeRequestOutput converts gl.BlockingMergeRequest to its output
// shape. The SDK passes the value by value (non-pointer) so this always
// returns a populated pointer.
func blockingMergeRequestOutput(b gl.BlockingMergeRequest) *BlockingMergeRequestOutput {
	out := &BlockingMergeRequestOutput{
		ID: b.ID, IID: b.Iid, TargetBranch: b.TargetBranch, SourceBranch: b.SourceBranch,
		ProjectID: b.ProjectID, Title: b.Title, State: b.State,
		CreatedAt: formatTimeValue(b.CreatedAt), UpdatedAt: formatTimeValue(b.UpdatedAt),
		Upvotes: b.Upvotes, Downvotes: b.Downvotes,
		Author: basicUserOutput(b.Author), Assignee: basicUserOutput(b.Assignee),
		Assignees: basicUserOutputs(b.Assignees), Reviewers: basicUserOutputs(b.Reviewers),
		SourceProjectID: b.SourceProjectID, TargetProjectID: b.TargetProjectID,
		Labels: labelOptionsToStrings(b.Labels), Description: b.Description, Draft: b.Draft,
		Milestone: derefString(b.Milestone), AutoMerge: b.AutoMerge, DetailedMergeStatus: b.DetailedMergeStatus,
		MergedAt: toolutil.FormatTimePtr(b.MergedAt), ClosedBy: basicUserOutput(b.ClosedBy), ClosedAt: toolutil.FormatTimePtr(b.ClosedAt),
		SHA: b.Sha, MergeCommitSHA: b.MergeCommitSha, SquashCommitSHA: b.SquashCommitSha,
		UserNotesCount: b.UserNotesCount, ShouldRemoveSourceBranch: b.ShouldRemoveSourceBranch,
		ForceRemoveSourceBranch: b.ForceRemoveSourceBranch, WebURL: b.WebURL,
		References: referencesOutput(b.References), DiscussionLocked: b.DiscussionLocked,
		TimeStats: timeStatsPtr(b.TimeStats), Squash: b.Squash,
		TaskCompletionStatus: taskCompletionStatusOutput(b.TaskCompletionStatus),
		HasConflicts:         b.HasConflicts, BlockingDiscussionsResolved: b.BlockingDiscussionsResolved,
		MergeUser: basicUserOutput(b.MergeUser), MergeAfter: formatTimeValue(b.MergeAfter),
		Imported: b.Imported, ImportedFrom: b.ImportedFrom, PreparedAt: toolutil.FormatTimePtr(b.PreparedAt),
		SquashOnMerge:             b.SquashOnMerge,
		WorkInProgress:            b.WorkInProgress,            //nolint:staticcheck // SA1019: mirrored for 1:1 SDK fidelity; use Draft.
		MergeWhenPipelineSucceeds: b.MergeWhenPipelineSucceeds, //nolint:staticcheck // SA1019: mirrored for 1:1 SDK fidelity; use AutoMerge.
		MergedBy:                  basicUserOutput(b.MergedBy), //nolint:staticcheck // SA1019: mirrored for 1:1 SDK fidelity; use MergeUser.
		ApprovalsBeforeMerge:      b.ApprovalsBeforeMerge,      //nolint:staticcheck // SA1019: mirrored for 1:1 SDK fidelity.
		Reference:                 b.Reference,                 //nolint:staticcheck // SA1019: mirrored for 1:1 SDK fidelity; use References.
		MergeStatus:               b.MergeStatus,               //nolint:staticcheck // SA1019: mirrored for 1:1 SDK fidelity; use DetailedMergeStatus.
	}
	return out
}

// formatTimeValue renders a non-pointer time.Time as RFC 3339, or "" when zero.
func formatTimeValue(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

// derefString returns the pointed-to string or "" when nil.
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// labelOptionsToStrings converts a *gl.LabelOptions to a []string, returning an
// empty slice when nil.
func labelOptionsToStrings(l *gl.LabelOptions) []string {
	if l == nil {
		return []string{}
	}
	return []string(*l)
}
