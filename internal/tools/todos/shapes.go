package todos

import (
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// Canonical output shapes mirrored from client-go sub-objects. Per the 1:1
// audit policy (full nested objects) these surface every field of the SDK
// struct and are replicated locally rather than imported from sibling packages
// to preserve the zero-import-cycle constraint (C-IMPORTS).
//
// This file covers the to-do sub-objects surfaced on the canonical json keys:
// project (gl.BasicProject), author (gl.BasicUser), and target (gl.TodoTarget),
// including the nested issue/MR summary objects referenced from a target.

// formatTimePtr renders an optional timestamp as RFC 3339, or "" when nil.
func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

// BasicProjectOut mirrors gl.BasicProject (the project a to-do belongs to).
type BasicProjectOut struct {
	ID                int64  `json:"id"`
	Description       string `json:"description"`
	Name              string `json:"name"`
	NameWithNamespace string `json:"name_with_namespace"`
	Path              string `json:"path"`
	PathWithNamespace string `json:"path_with_namespace"`
	CreatedAt         string `json:"created_at,omitempty"`
}

func basicProjectOut(p *gl.BasicProject) *BasicProjectOut {
	if p == nil {
		return nil
	}
	return &BasicProjectOut{
		ID: p.ID, Description: p.Description, Name: p.Name,
		NameWithNamespace: p.NameWithNamespace, Path: p.Path,
		PathWithNamespace: p.PathWithNamespace, CreatedAt: formatTimePtr(p.CreatedAt),
	}
}

// BasicUserOut mirrors gl.BasicUser (the to-do author and target user objects).
type BasicUserOut struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	State     string `json:"state"`
	AvatarURL string `json:"avatar_url"`
	WebURL    string `json:"web_url"`
	CreatedAt string `json:"created_at,omitempty"`
}

func basicUserOut(u *gl.BasicUser) *BasicUserOut {
	if u == nil {
		return nil
	}
	return &BasicUserOut{
		ID: u.ID, Username: u.Username, Name: u.Name, State: u.State,
		AvatarURL: u.AvatarURL, WebURL: u.WebURL, CreatedAt: formatTimePtr(u.CreatedAt),
	}
}

func basicUserOuts(users []*gl.BasicUser) []*BasicUserOut {
	if len(users) == 0 {
		return nil
	}
	out := make([]*BasicUserOut, 0, len(users))
	for _, u := range users {
		if u == nil {
			continue
		}
		out = append(out, basicUserOut(u))
	}
	return out
}

// MilestoneOut mirrors gl.Milestone (the milestone of a to-do's target).
type MilestoneOut struct {
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

func milestoneOut(m *gl.Milestone) *MilestoneOut {
	if m == nil {
		return nil
	}
	return &MilestoneOut{
		ID: m.ID, IID: m.IID, GroupID: m.GroupID, ProjectID: m.ProjectID,
		Title: m.Title, Description: m.Description, State: m.State, WebURL: m.WebURL,
		StartDate: formatISOTimePtr(m.StartDate), DueDate: formatISOTimePtr(m.DueDate),
		CreatedAt: formatTimePtr(m.CreatedAt), UpdatedAt: formatTimePtr(m.UpdatedAt),
		Expired: m.Expired,
	}
}

// formatISOTimePtr renders an optional ISO date (gl.ISOTime) as YYYY-MM-DD.
func formatISOTimePtr(t *gl.ISOTime) string {
	if t == nil {
		return ""
	}
	return time.Time(*t).Format("2006-01-02")
}

// TaskCompletionStatusOut mirrors gl.TasksCompletionStatus.
type TaskCompletionStatusOut struct {
	Count          int64 `json:"count"`
	CompletedCount int64 `json:"completed_count"`
}

func taskCompletionStatusOut(t *gl.TasksCompletionStatus) *TaskCompletionStatusOut {
	if t == nil {
		return nil
	}
	return &TaskCompletionStatusOut{Count: t.Count, CompletedCount: t.CompletedCount}
}

// IssueLinksOut mirrors gl.IssueLinks (the _links object on an Issue target).
type IssueLinksOut struct {
	Self       string `json:"self"`
	Notes      string `json:"notes"`
	AwardEmoji string `json:"award_emoji"`
	Project    string `json:"project"`
}

func issueLinksOut(l *gl.IssueLinks) *IssueLinksOut {
	if l == nil {
		return nil
	}
	return &IssueLinksOut{Self: l.Self, Notes: l.Notes, AwardEmoji: l.AwardEmoji, Project: l.Project}
}

// TimeStatsOut mirrors gl.TimeStats (the time_stats object on an Issue target).
type TimeStatsOut struct {
	HumanTimeEstimate   string `json:"human_time_estimate"`
	HumanTotalTimeSpent string `json:"human_total_time_spent"`
	TimeEstimate        int64  `json:"time_estimate"`
	TotalTimeSpent      int64  `json:"total_time_spent"`
}

func timeStatsOut(t *gl.TimeStats) *TimeStatsOut {
	if t == nil {
		return nil
	}
	return &TimeStatsOut{
		HumanTimeEstimate: t.HumanTimeEstimate, HumanTotalTimeSpent: t.HumanTotalTimeSpent,
		TimeEstimate: t.TimeEstimate, TotalTimeSpent: t.TotalTimeSpent,
	}
}

// TodoTargetOut mirrors gl.TodoTarget, the issue or merge-request summary that
// a to-do points at. It surfaces every SDK field, including the type-specific
// blocks (Issue-only, MergeRequest-only, Design-only) carried by the SDK.
type TodoTargetOut struct {
	Assignees            []*BasicUserOut          `json:"assignees,omitempty"`
	Assignee             *BasicUserOut            `json:"assignee,omitempty"`
	Author               *BasicUserOut            `json:"author,omitempty"`
	CreatedAt            string                   `json:"created_at,omitempty"`
	Description          string                   `json:"description,omitempty"`
	Downvotes            int64                    `json:"downvotes,omitempty"`
	ID                   any                      `json:"id,omitempty"`
	IID                  int64                    `json:"iid,omitempty"`
	Labels               []string                 `json:"labels,omitempty"`
	Milestone            *MilestoneOut            `json:"milestone,omitempty"`
	ProjectID            int64                    `json:"project_id,omitempty"`
	State                string                   `json:"state,omitempty"`
	Subscribed           bool                     `json:"subscribed,omitempty"`
	TaskCompletionStatus *TaskCompletionStatusOut `json:"task_completion_status,omitempty"`
	Title                string                   `json:"title,omitempty"`
	UpdatedAt            string                   `json:"updated_at,omitempty"`
	Upvotes              int64                    `json:"upvotes,omitempty"`
	UserNotesCount       int64                    `json:"user_notes_count,omitempty"`
	WebURL               string                   `json:"web_url,omitempty"`

	// Only available for type Issue.
	Confidential bool           `json:"confidential,omitempty"`
	DueDate      string         `json:"due_date,omitempty"`
	HasTasks     bool           `json:"has_tasks,omitempty"`
	Links        *IssueLinksOut `json:"_links,omitempty"`
	MovedToID    int64          `json:"moved_to_id,omitempty"`
	TimeStats    *TimeStatsOut  `json:"time_stats,omitempty"`
	Weight       int64          `json:"weight,omitempty" tier:"premium"`

	// Only available for type MergeRequest.
	MergedAt                  string          `json:"merged_at,omitempty"`
	ApprovalsBeforeMerge      int64           `json:"approvals_before_merge,omitempty"`
	ForceRemoveSourceBranch   bool            `json:"force_remove_source_branch,omitempty"`
	MergeCommitSHA            string          `json:"merge_commit_sha,omitempty"`
	MergeWhenPipelineSucceeds bool            `json:"merge_when_pipeline_succeeds,omitempty"`
	MergeStatus               string          `json:"merge_status,omitempty"`
	Reference                 string          `json:"reference,omitempty"`
	Reviewers                 []*BasicUserOut `json:"reviewers,omitempty"`
	SHA                       string          `json:"sha,omitempty"`
	ShouldRemoveSourceBranch  bool            `json:"should_remove_source_branch,omitempty"`
	SourceBranch              string          `json:"source_branch,omitempty"`
	SourceProjectID           int64           `json:"source_project_id,omitempty"`
	Squash                    bool            `json:"squash,omitempty"`
	TargetBranch              string          `json:"target_branch,omitempty"`
	TargetProjectID           int64           `json:"target_project_id,omitempty"`
	WorkInProgress            bool            `json:"work_in_progress,omitempty"`

	// Only available for type DesignManagement::Design.
	FileName string `json:"filename,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}

func todoTargetOut(t *gl.TodoTarget) *TodoTargetOut {
	if t == nil {
		return nil
	}
	return &TodoTargetOut{
		Assignees:            basicUserOuts(t.Assignees),
		Assignee:             basicUserOut(t.Assignee),
		Author:               basicUserOut(t.Author),
		CreatedAt:            formatTimePtr(t.CreatedAt),
		Description:          t.Description,
		Downvotes:            t.Downvotes,
		ID:                   t.ID,
		IID:                  t.IID,
		Labels:               t.Labels,
		Milestone:            milestoneOut(t.Milestone),
		ProjectID:            t.ProjectID,
		State:                t.State,
		Subscribed:           t.Subscribed,
		TaskCompletionStatus: taskCompletionStatusOut(t.TaskCompletionStatus),
		Title:                t.Title,
		UpdatedAt:            formatTimePtr(t.UpdatedAt),
		Upvotes:              t.Upvotes,
		UserNotesCount:       t.UserNotesCount,
		WebURL:               t.WebURL,

		Confidential: t.Confidential,
		DueDate:      t.DueDate,
		HasTasks:     t.HasTasks,
		Links:        issueLinksOut(t.Links),
		MovedToID:    t.MovedToID,
		TimeStats:    timeStatsOut(t.TimeStats),
		Weight:       t.Weight,

		MergedAt:                  formatTimePtr(t.MergedAt),
		ApprovalsBeforeMerge:      t.ApprovalsBeforeMerge,
		ForceRemoveSourceBranch:   t.ForceRemoveSourceBranch,
		MergeCommitSHA:            t.MergeCommitSHA,
		MergeWhenPipelineSucceeds: t.MergeWhenPipelineSucceeds,
		MergeStatus:               t.MergeStatus,
		Reference:                 t.Reference,
		Reviewers:                 basicUserOuts(t.Reviewers),
		SHA:                       t.SHA,
		ShouldRemoveSourceBranch:  t.ShouldRemoveSourceBranch,
		SourceBranch:              t.SourceBranch,
		SourceProjectID:           t.SourceProjectID,
		Squash:                    t.Squash,
		TargetBranch:              t.TargetBranch,
		TargetProjectID:           t.TargetProjectID,
		WorkInProgress:            t.WorkInProgress,

		FileName: t.FileName,
		ImageURL: t.ImageURL,
	}
}
