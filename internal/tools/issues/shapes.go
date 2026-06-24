package issues

import (
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// Canonical output shapes mirrored from client-go sub-objects. Per the 1:1
// audit policy (full nested objects) these surface every field of the SDK
// struct and are replicated here rather than imported from sibling packages to
// preserve the zero-import-cycle constraint (C-IMPORTS).
//
// This file currently covers the additive issue sub-objects (_links, time
// tracking, task completion, label details, iteration). The author/assignee/
// milestone/references string fields remain flattened pending the dedicated
// strict-object migration slice (their json keys would otherwise collide).

// formatTimePtr renders an optional timestamp as RFC 3339, or "" when nil.
func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

// formatISOTimePtr renders an optional ISO date (gl.ISOTime) as YYYY-MM-DD.
func formatISOTimePtr(t *gl.ISOTime) string {
	if t == nil {
		return ""
	}
	return time.Time(*t).Format("2006-01-02")
}

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
		CreatedAt: formatTimePtr(it.CreatedAt), UpdatedAt: formatTimePtr(it.UpdatedAt),
		StartDate: formatISOTimePtr(it.StartDate), DueDate: formatISOTimePtr(it.DueDate),
	}
}
