package issues

import (
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical output shapes mirrored from client-go sub-objects. Per the 1:1
// audit policy (full nested objects) these surface every field of the SDK
// struct and are replicated here rather than imported from sibling packages to
// preserve the zero-import-cycle constraint (C-IMPORTS).
//
// This file covers the issue sub-objects surfaced on the canonical json keys
// (author, assignees, assignee, closed_by, milestone, references, epic,
// _links, time tracking, task completion, label details, iteration). The old
// flattened scalars are preserved additively under *_username / *_title keys.

// LinksOutput mirrors gl.IssueLinks (the issue _links object).
type LinksOutput struct {
	Self       string `json:"self"`
	Notes      string `json:"notes"`
	AwardEmoji string `json:"award_emoji"`
	Project    string `json:"project"`
}

func linksOutput(l *gl.IssueLinks) *LinksOutput {
	if l == nil {
		return nil
	}
	return &LinksOutput{Self: l.Self, Notes: l.Notes, AwardEmoji: l.AwardEmoji, Project: l.Project}
}

// timeStatsPtr returns a pointer to the issue time-stats output (reusing the
// existing TimeStatsOutput type), or nil when the SDK value is nil.
func timeStatsPtr(t *gl.TimeStats) *TimeStatsOutput {
	if t == nil {
		return nil
	}
	ts := timeStatsToOutput(t)
	return &ts
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
	return out
}

// IterationOutput mirrors gl.GroupIteration.
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

func iterationOutput(it *gl.GroupIteration) *IterationOutput {
	if it == nil {
		return nil
	}
	return &IterationOutput{
		ID: it.ID, IID: it.IID, Sequence: it.Sequence, GroupID: it.GroupID,
		Title: it.Title, Description: it.Description, State: it.State, WebURL: it.WebURL,
		CreatedAt: toolutil.FormatTimePtr(it.CreatedAt), UpdatedAt: toolutil.FormatTimePtr(it.UpdatedAt),
		StartDate: toolutil.FormatISOTimePtr(it.StartDate), DueDate: toolutil.FormatISOTimePtr(it.DueDate),
	}
}

// BasicUserOutput mirrors gl.BasicUser, the compact user object embedded in
// merge-request payloads (author, assignee, reviewers, merge_user, closed_by).
// It differs from IssueAuthorOutput/IssueAssigneeOutput (which mirror the
// issue-specific gl.IssueAuthor/gl.IssueAssignee types) by carrying a
// created_at timestamp.
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
	return out
}

// IssueAuthorOutput mirrors gl.IssueAuthor (the issue author object).
type IssueAuthorOutput struct {
	ID        int64  `json:"id"`
	State     string `json:"state"`
	WebURL    string `json:"web_url"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
	Username  string `json:"username"`
}

func issueAuthorOutput(a *gl.IssueAuthor) *IssueAuthorOutput {
	if a == nil {
		return nil
	}
	return &IssueAuthorOutput{
		ID: a.ID, State: a.State, WebURL: a.WebURL,
		Name: a.Name, AvatarURL: a.AvatarURL, Username: a.Username,
	}
}

// IssueAssigneeOutput mirrors gl.IssueAssignee (an issue assignee object).
type IssueAssigneeOutput struct {
	ID        int64  `json:"id"`
	State     string `json:"state"`
	WebURL    string `json:"web_url"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
	Username  string `json:"username"`
}

func issueAssigneeOutput(a *gl.IssueAssignee) *IssueAssigneeOutput {
	if a == nil {
		return nil
	}
	return &IssueAssigneeOutput{
		ID: a.ID, State: a.State, WebURL: a.WebURL,
		Name: a.Name, AvatarURL: a.AvatarURL, Username: a.Username,
	}
}

func issueAssigneeOutputs(assignees []*gl.IssueAssignee) []*IssueAssigneeOutput {
	if len(assignees) == 0 {
		return nil
	}
	out := make([]*IssueAssigneeOutput, 0, len(assignees))
	for _, a := range assignees {
		if a == nil {
			continue
		}
		out = append(out, issueAssigneeOutput(a))
	}
	return out
}

// IssueCloserOutput mirrors gl.IssueCloser (the user that closed the issue).
type IssueCloserOutput struct {
	ID        int64  `json:"id"`
	State     string `json:"state"`
	WebURL    string `json:"web_url"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
	Username  string `json:"username"`
}

func issueCloserOutput(c *gl.IssueCloser) *IssueCloserOutput {
	if c == nil {
		return nil
	}
	return &IssueCloserOutput{
		ID: c.ID, State: c.State, WebURL: c.WebURL,
		Name: c.Name, AvatarURL: c.AvatarURL, Username: c.Username,
	}
}

// MilestoneOutput mirrors gl.Milestone (the issue milestone object).
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

// ReferencesOutput mirrors gl.IssueReferences (the issue references object).
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

// EpicAuthorOutput mirrors gl.EpicAuthor (the author of an epic).
type EpicAuthorOutput struct {
	ID        int64  `json:"id"`
	State     string `json:"state"`
	WebURL    string `json:"web_url"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
	Username  string `json:"username"`
}

func epicAuthorOutput(a *gl.EpicAuthor) *EpicAuthorOutput {
	if a == nil {
		return nil
	}
	return &EpicAuthorOutput{
		ID: a.ID, State: a.State, WebURL: a.WebURL,
		Name: a.Name, AvatarURL: a.AvatarURL, Username: a.Username,
	}
}

// EpicOutput mirrors gl.Epic (the epic associated with an issue, EE only).
type EpicOutput struct {
	ID                      int64             `json:"id"`
	IID                     int64             `json:"iid"`
	GroupID                 int64             `json:"group_id"`
	ParentID                int64             `json:"parent_id"`
	Title                   string            `json:"title"`
	Description             string            `json:"description"`
	State                   string            `json:"state"`
	Confidential            bool              `json:"confidential"`
	WebURL                  string            `json:"web_url"`
	URL                     string            `json:"url"`
	Author                  *EpicAuthorOutput `json:"author,omitempty"`
	Labels                  []string          `json:"labels,omitempty"`
	Upvotes                 int64             `json:"upvotes,omitempty"`
	Downvotes               int64             `json:"downvotes,omitempty"`
	UserNotesCount          int64             `json:"user_notes_count,omitempty"`
	StartDate               string            `json:"start_date,omitempty"`
	StartDateIsFixed        bool              `json:"start_date_is_fixed,omitempty"`
	StartDateFixed          string            `json:"start_date_fixed,omitempty"`
	StartDateFromMilestones string            `json:"start_date_from_milestones,omitempty"`
	DueDate                 string            `json:"due_date,omitempty"`
	DueDateIsFixed          bool              `json:"due_date_is_fixed,omitempty"`
	DueDateFixed            string            `json:"due_date_fixed,omitempty"`
	DueDateFromMilestones   string            `json:"due_date_from_milestones,omitempty"`
	CreatedAt               string            `json:"created_at,omitempty"`
	UpdatedAt               string            `json:"updated_at,omitempty"`
	ClosedAt                string            `json:"closed_at,omitempty"`
}

func epicOutput(e *gl.Epic) *EpicOutput {
	if e == nil {
		return nil
	}
	out := &EpicOutput{
		ID: e.ID, IID: e.IID, GroupID: e.GroupID, ParentID: e.ParentID,
		Title: e.Title, Description: e.Description, State: e.State,
		Confidential: e.Confidential, WebURL: e.WebURL, URL: e.URL,
		Author:                  epicAuthorOutput(e.Author),
		Labels:                  e.Labels,
		Upvotes:                 e.Upvotes,
		Downvotes:               e.Downvotes,
		UserNotesCount:          e.UserNotesCount,
		StartDate:               toolutil.FormatISOTimePtr(e.StartDate),
		StartDateIsFixed:        e.StartDateIsFixed,
		StartDateFixed:          toolutil.FormatISOTimePtr(e.StartDateFixed),
		StartDateFromMilestones: toolutil.FormatISOTimePtr(e.StartDateFromMilestones),
		DueDate:                 toolutil.FormatISOTimePtr(e.DueDate),
		DueDateIsFixed:          e.DueDateIsFixed,
		DueDateFixed:            toolutil.FormatISOTimePtr(e.DueDateFixed),
		DueDateFromMilestones:   toolutil.FormatISOTimePtr(e.DueDateFromMilestones),
		CreatedAt:               toolutil.FormatTimePtr(e.CreatedAt),
		UpdatedAt:               toolutil.FormatTimePtr(e.UpdatedAt),
		ClosedAt:                toolutil.FormatTimePtr(e.ClosedAt),
	}
	return out
}
