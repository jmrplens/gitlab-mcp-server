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

// LabelDetailsOutput mirrors gl.LabelDetails.

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

// BasicUserOutput mirrors gl.BasicUser, the compact user object embedded in
// merge-request payloads (author, assignee, reviewers, merge_user, closed_by).
// It differs from IssueAuthorOutput/IssueAssigneeOutput (which mirror the
// issue-specific gl.IssueAuthor/gl.IssueAssignee types) by carrying a
// created_at timestamp.

// basicUserOutput converts a single gl.BasicUser to its output shape, returning
// nil when the SDK value is nil.

// basicUserOutputs converts a slice of gl.BasicUser, skipping nil elements and
// returning nil for an empty or all-nil slice.

// EpicOutput mirrors gl.Epic (the epic associated with an issue, EE only).

// epicOutput delegates to toolutil.NewEpicOutput. The local wrapper
// exists so the call site reads naturally alongside the other package-local
// converters (issueAuthorOutput, etc.).

// mrMilestoneOutputPtr converts a gl.Milestone pointer into the canonical
// MRMilestoneOutput. Package-local wrapper so the call site reads naturally.
func mrMilestoneOutputPtr(m *gl.Milestone) *toolutil.MRMilestoneOutput {
	out := toolutil.NewMRMilestoneOutputs([]*gl.Milestone{m})
	if len(out) == 0 {
		return nil
	}
	return out[0]
}
