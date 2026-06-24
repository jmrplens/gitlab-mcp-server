package issuelinks

import (
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

// Canonical output shapes mirrored from client-go sub-objects. Per the 1:1
// audit policy (full nested objects) these surface every field of the SDK
// struct and are replicated here rather than imported from sibling packages to
// preserve the zero-import-cycle constraint (C-IMPORTS).

// formatTimePtr renders an optional timestamp as RFC 3339, or "" when nil.
func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

// formatISOTimePtr renders an optional ISO date (gitlab.ISOTime) as YYYY-MM-DD,
// or "" when nil.
func formatISOTimePtr(t *gitlab.ISOTime) string {
	if t == nil {
		return ""
	}
	return time.Time(*t).Format("2006-01-02")
}

// UserOutput mirrors gitlab.IssueAuthor / gitlab.IssueAssignee (they share the
// same shape). It surfaces the full user sub-object referenced by an issue
// relation's author and assignees.
type UserOutput struct {
	ID        int64  `json:"id"`
	State     string `json:"state"`
	WebURL    string `json:"web_url"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
	Username  string `json:"username"`
}

// authorOutput converts a *gitlab.IssueAuthor to a *UserOutput, or nil when the
// SDK value is nil.
func authorOutput(a *gitlab.IssueAuthor) *UserOutput {
	if a == nil {
		return nil
	}
	return &UserOutput{
		ID: a.ID, State: a.State, WebURL: a.WebURL,
		Name: a.Name, AvatarURL: a.AvatarURL, Username: a.Username,
	}
}

// assigneeOutput converts a *gitlab.IssueAssignee to a *UserOutput, or nil when
// the SDK value is nil.
func assigneeOutput(a *gitlab.IssueAssignee) *UserOutput {
	if a == nil {
		return nil
	}
	return &UserOutput{
		ID: a.ID, State: a.State, WebURL: a.WebURL,
		Name: a.Name, AvatarURL: a.AvatarURL, Username: a.Username,
	}
}

// assigneeOutputs converts a slice of *gitlab.IssueAssignee to a slice of
// *UserOutput, skipping nil elements and returning nil for an empty input.
func assigneeOutputs(in []*gitlab.IssueAssignee) []*UserOutput {
	if len(in) == 0 {
		return nil
	}
	out := make([]*UserOutput, 0, len(in))
	for _, a := range in {
		if a == nil {
			continue
		}
		out = append(out, assigneeOutput(a))
	}
	return out
}

// closerOutput converts a *gitlab.IssueCloser to a *UserOutput, or nil when the
// SDK value is nil. IssueCloser shares the user shape mirrored by UserOutput.
func closerOutput(c *gitlab.IssueCloser) *UserOutput {
	if c == nil {
		return nil
	}
	return &UserOutput{
		ID: c.ID, State: c.State, WebURL: c.WebURL,
		Name: c.Name, AvatarURL: c.AvatarURL, Username: c.Username,
	}
}

// epicAuthorOutput converts a *gitlab.EpicAuthor to a *UserOutput, or nil when
// the SDK value is nil. EpicAuthor shares the user shape mirrored by UserOutput.
func epicAuthorOutput(a *gitlab.EpicAuthor) *UserOutput {
	if a == nil {
		return nil
	}
	return &UserOutput{
		ID: a.ID, State: a.State, WebURL: a.WebURL,
		Name: a.Name, AvatarURL: a.AvatarURL, Username: a.Username,
	}
}

// ReferencesOutput mirrors gitlab.IssueReferences (the issue references object).
type ReferencesOutput struct {
	Short    string `json:"short"`
	Relative string `json:"relative"`
	Full     string `json:"full"`
}

// referencesOutput converts a *gitlab.IssueReferences to a *ReferencesOutput, or
// nil when the SDK value is nil.
func referencesOutput(r *gitlab.IssueReferences) *ReferencesOutput {
	if r == nil {
		return nil
	}
	return &ReferencesOutput{Short: r.Short, Relative: r.Relative, Full: r.Full}
}

// MilestoneOutput mirrors gitlab.Milestone (the milestone object attached to an
// issue relation).
type MilestoneOutput struct {
	ID          int64  `json:"id"`
	IID         int64  `json:"iid"`
	GroupID     int64  `json:"group_id"`
	ProjectID   int64  `json:"project_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	StartDate   string `json:"start_date,omitempty"`
	DueDate     string `json:"due_date,omitempty"`
	State       string `json:"state"`
	WebURL      string `json:"web_url"`
	UpdatedAt   string `json:"updated_at,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	Expired     *bool  `json:"expired,omitempty"`
}

// milestoneOutput converts a *gitlab.Milestone to a *MilestoneOutput, or nil
// when the SDK value is nil.
func milestoneOutput(m *gitlab.Milestone) *MilestoneOutput {
	if m == nil {
		return nil
	}
	return &MilestoneOutput{
		ID: m.ID, IID: m.IID, GroupID: m.GroupID, ProjectID: m.ProjectID,
		Title: m.Title, Description: m.Description,
		StartDate: formatISOTimePtr(m.StartDate), DueDate: formatISOTimePtr(m.DueDate),
		State: m.State, WebURL: m.WebURL,
		UpdatedAt: formatTimePtr(m.UpdatedAt), CreatedAt: formatTimePtr(m.CreatedAt),
		Expired: m.Expired,
	}
}

// LinksOutput mirrors gitlab.IssueLinks (the issue _links object).
type LinksOutput struct {
	Self       string `json:"self"`
	Notes      string `json:"notes"`
	AwardEmoji string `json:"award_emoji"`
	Project    string `json:"project"`
}

// linksOutput converts a *gitlab.IssueLinks to a *LinksOutput, or nil when the
// SDK value is nil.
func linksOutput(l *gitlab.IssueLinks) *LinksOutput {
	if l == nil {
		return nil
	}
	return &LinksOutput{Self: l.Self, Notes: l.Notes, AwardEmoji: l.AwardEmoji, Project: l.Project}
}

// TimeStatsOutput mirrors gitlab.TimeStats (the issue time-tracking object).
type TimeStatsOutput struct {
	HumanTimeEstimate   string `json:"human_time_estimate"`
	HumanTotalTimeSpent string `json:"human_total_time_spent"`
	TimeEstimate        int64  `json:"time_estimate"`
	TotalTimeSpent      int64  `json:"total_time_spent"`
}

// timeStatsOutput converts a *gitlab.TimeStats to a *TimeStatsOutput, or nil
// when the SDK value is nil.
func timeStatsOutput(t *gitlab.TimeStats) *TimeStatsOutput {
	if t == nil {
		return nil
	}
	return &TimeStatsOutput{
		HumanTimeEstimate:   t.HumanTimeEstimate,
		HumanTotalTimeSpent: t.HumanTotalTimeSpent,
		TimeEstimate:        t.TimeEstimate,
		TotalTimeSpent:      t.TotalTimeSpent,
	}
}

// TaskCompletionStatusOutput mirrors gitlab.TasksCompletionStatus.
type TaskCompletionStatusOutput struct {
	Count          int64 `json:"count"`
	CompletedCount int64 `json:"completed_count"`
}

// taskCompletionStatusOutput converts a *gitlab.TasksCompletionStatus to a
// *TaskCompletionStatusOutput, or nil when the SDK value is nil.
func taskCompletionStatusOutput(t *gitlab.TasksCompletionStatus) *TaskCompletionStatusOutput {
	if t == nil {
		return nil
	}
	return &TaskCompletionStatusOutput{Count: t.Count, CompletedCount: t.CompletedCount}
}

// LabelDetailsOutput mirrors gitlab.LabelDetails (a label object with full
// detail surfaced under the label_details key).
type LabelDetailsOutput struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	Color           string `json:"color"`
	Description     string `json:"description"`
	DescriptionHTML string `json:"description_html"`
	TextColor       string `json:"text_color"`
}

// labelDetailsOutputs converts a slice of *gitlab.LabelDetails to a slice of
// *LabelDetailsOutput, skipping nil elements and returning nil for an empty
// input.
func labelDetailsOutputs(details []*gitlab.LabelDetails) []*LabelDetailsOutput {
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
	return out
}

// IterationOutput mirrors gitlab.GroupIteration (the iteration assigned to an
// issue, EE only).
type IterationOutput struct {
	ID          int64  `json:"id"`
	IID         int64  `json:"iid"`
	Sequence    int64  `json:"sequence"`
	GroupID     int64  `json:"group_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	State       int64  `json:"state"`
	WebURL      string `json:"web_url"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
	StartDate   string `json:"start_date,omitempty"`
	DueDate     string `json:"due_date,omitempty"`
}

// iterationOutput converts a *gitlab.GroupIteration to a *IterationOutput, or
// nil when the SDK value is nil.
func iterationOutput(it *gitlab.GroupIteration) *IterationOutput {
	if it == nil {
		return nil
	}
	return &IterationOutput{
		ID: it.ID, IID: it.IID, Sequence: it.Sequence, GroupID: it.GroupID,
		Title: it.Title, Description: it.Description, State: it.State, WebURL: it.WebURL,
		CreatedAt: formatTimePtr(it.CreatedAt), UpdatedAt: formatTimePtr(it.UpdatedAt),
		StartDate: formatISOTimePtr(it.StartDate), DueDate: formatISOTimePtr(it.DueDate),
	}
}

// EpicOutput mirrors gitlab.Epic (the epic associated with an issue, EE only).
type EpicOutput struct {
	ID                      int64       `json:"id"`
	IID                     int64       `json:"iid"`
	GroupID                 int64       `json:"group_id"`
	ParentID                int64       `json:"parent_id"`
	Title                   string      `json:"title"`
	Description             string      `json:"description"`
	State                   string      `json:"state"`
	Confidential            bool        `json:"confidential"`
	WebURL                  string      `json:"web_url"`
	URL                     string      `json:"url"`
	Author                  *UserOutput `json:"author,omitempty"`
	Labels                  []string    `json:"labels,omitempty"`
	Upvotes                 int64       `json:"upvotes,omitempty"`
	Downvotes               int64       `json:"downvotes,omitempty"`
	UserNotesCount          int64       `json:"user_notes_count,omitempty"`
	StartDate               string      `json:"start_date,omitempty"`
	StartDateIsFixed        bool        `json:"start_date_is_fixed,omitempty"`
	StartDateFixed          string      `json:"start_date_fixed,omitempty"`
	StartDateFromMilestones string      `json:"start_date_from_milestones,omitempty"`
	DueDate                 string      `json:"due_date,omitempty"`
	DueDateIsFixed          bool        `json:"due_date_is_fixed,omitempty"`
	DueDateFixed            string      `json:"due_date_fixed,omitempty"`
	DueDateFromMilestones   string      `json:"due_date_from_milestones,omitempty"`
	CreatedAt               string      `json:"created_at,omitempty"`
	UpdatedAt               string      `json:"updated_at,omitempty"`
	ClosedAt                string      `json:"closed_at,omitempty"`
}

// epicOutput converts a *gitlab.Epic to a *EpicOutput, or nil when the SDK
// value is nil.
func epicOutput(e *gitlab.Epic) *EpicOutput {
	if e == nil {
		return nil
	}
	return &EpicOutput{
		ID: e.ID, IID: e.IID, GroupID: e.GroupID, ParentID: e.ParentID,
		Title: e.Title, Description: e.Description, State: e.State,
		Confidential: e.Confidential, WebURL: e.WebURL, URL: e.URL,
		Author:                  epicAuthorOutput(e.Author),
		Labels:                  e.Labels,
		Upvotes:                 e.Upvotes,
		Downvotes:               e.Downvotes,
		UserNotesCount:          e.UserNotesCount,
		StartDate:               formatISOTimePtr(e.StartDate),
		StartDateIsFixed:        e.StartDateIsFixed,
		StartDateFixed:          formatISOTimePtr(e.StartDateFixed),
		StartDateFromMilestones: formatISOTimePtr(e.StartDateFromMilestones),
		DueDate:                 formatISOTimePtr(e.DueDate),
		DueDateIsFixed:          e.DueDateIsFixed,
		DueDateFixed:            formatISOTimePtr(e.DueDateFixed),
		DueDateFromMilestones:   formatISOTimePtr(e.DueDateFromMilestones),
		CreatedAt:               formatTimePtr(e.CreatedAt),
		UpdatedAt:               formatTimePtr(e.UpdatedAt),
		ClosedAt:                formatTimePtr(e.ClosedAt),
	}
}

// IssueRefOutput mirrors the full gitlab.Issue struct surfaced for the
// source_issue and target_issue objects on a single issue link. Per the 1:1
// audit policy (full nested objects) every field of the SDK Issue is surfaced
// with the correct type: user-like objects (author/assignees/assignee/
// closed_by) as *UserOutput, and milestone/references/epic/iteration/
// time_stats/task_completion_status/label_details/_links as their full local
// mirror objects. labels is surfaced as []string. The existing flattened
// SourceIssueIID/SourceProjectID/TargetIssueIID/TargetProjectID scalars on
// Output remain additive for ergonomic consumers.
type IssueRefOutput struct {
	ID                   int64                       `json:"id"`
	IID                  int64                       `json:"iid"`
	ExternalID           string                      `json:"external_id,omitempty"`
	State                string                      `json:"state"`
	Description          string                      `json:"description,omitempty"`
	HealthStatus         string                      `json:"health_status,omitempty"`
	Author               *UserOutput                 `json:"author,omitempty"`
	Milestone            *MilestoneOutput            `json:"milestone,omitempty"`
	ProjectID            int64                       `json:"project_id"`
	Assignees            []*UserOutput               `json:"assignees,omitempty"`
	Assignee             *UserOutput                 `json:"assignee,omitempty"`
	UpdatedAt            string                      `json:"updated_at,omitempty"`
	ClosedAt             string                      `json:"closed_at,omitempty"`
	ClosedBy             *UserOutput                 `json:"closed_by,omitempty"`
	Title                string                      `json:"title"`
	CreatedAt            string                      `json:"created_at,omitempty"`
	MovedToID            int64                       `json:"moved_to_id,omitempty"`
	Labels               []string                    `json:"labels,omitempty"`
	LabelDetails         []*LabelDetailsOutput       `json:"label_details,omitempty"`
	Upvotes              int64                       `json:"upvotes,omitempty"`
	Downvotes            int64                       `json:"downvotes,omitempty"`
	DueDate              string                      `json:"due_date,omitempty"`
	WebURL               string                      `json:"web_url"`
	References           *ReferencesOutput           `json:"references,omitempty"`
	TimeStats            *TimeStatsOutput            `json:"time_stats,omitempty"`
	Confidential         bool                        `json:"confidential"`
	Weight               int64                       `json:"weight,omitempty"`
	DiscussionLocked     bool                        `json:"discussion_locked"`
	IssueType            string                      `json:"issue_type,omitempty"`
	Subscribed           bool                        `json:"subscribed"`
	UserNotesCount       int64                       `json:"user_notes_count,omitempty"`
	Links                *LinksOutput                `json:"_links,omitempty"`
	IssueLinkID          int64                       `json:"issue_link_id,omitempty"`
	MergeRequestCount    int64                       `json:"merge_request_count,omitempty"`
	EpicIssueID          int64                       `json:"epic_issue_id,omitempty"`
	Epic                 *EpicOutput                 `json:"epic,omitempty"`
	Iteration            *IterationOutput            `json:"iteration,omitempty"`
	TaskCompletionStatus *TaskCompletionStatusOutput `json:"task_completion_status,omitempty"`
	ServiceDeskReplyTo   string                      `json:"service_desk_reply_to,omitempty"`
}

// issueRefOutput converts a *gitlab.Issue to a *IssueRefOutput, or nil when the
// SDK value is nil. It mirrors every field of the SDK Issue with the correct
// type, dereferencing the SDK's *string IssueType into the issue_type scalar.
func issueRefOutput(i *gitlab.Issue) *IssueRefOutput {
	if i == nil {
		return nil
	}
	out := &IssueRefOutput{
		ID: i.ID, IID: i.IID, ExternalID: i.ExternalID, State: i.State,
		Description: i.Description, HealthStatus: i.HealthStatus,
		Author:               authorOutput(i.Author),
		Milestone:            milestoneOutput(i.Milestone),
		ProjectID:            i.ProjectID,
		Assignees:            assigneeOutputs(i.Assignees),
		Assignee:             assigneeOutput(i.Assignee), //nolint:staticcheck // SA1019: surfaced for 1:1 SDK fidelity
		UpdatedAt:            formatTimePtr(i.UpdatedAt),
		ClosedAt:             formatTimePtr(i.ClosedAt),
		ClosedBy:             closerOutput(i.ClosedBy),
		Title:                i.Title,
		CreatedAt:            formatTimePtr(i.CreatedAt),
		MovedToID:            i.MovedToID,
		Labels:               []string(i.Labels),
		LabelDetails:         labelDetailsOutputs(i.LabelDetails),
		Upvotes:              i.Upvotes,
		Downvotes:            i.Downvotes,
		DueDate:              formatISOTimePtr(i.DueDate),
		WebURL:               i.WebURL,
		References:           referencesOutput(i.References),
		TimeStats:            timeStatsOutput(i.TimeStats),
		Confidential:         i.Confidential,
		Weight:               i.Weight,
		DiscussionLocked:     i.DiscussionLocked,
		Subscribed:           i.Subscribed,
		UserNotesCount:       i.UserNotesCount,
		Links:                linksOutput(i.Links),
		IssueLinkID:          i.IssueLinkID,
		MergeRequestCount:    i.MergeRequestCount,
		EpicIssueID:          i.EpicIssueID,
		Epic:                 epicOutput(i.Epic),
		Iteration:            iterationOutput(i.Iteration),
		TaskCompletionStatus: taskCompletionStatusOutput(i.TaskCompletionStatus),
		ServiceDeskReplyTo:   i.ServiceDeskReplyTo,
	}
	if i.IssueType != nil {
		out.IssueType = *i.IssueType
	}
	return out
}
