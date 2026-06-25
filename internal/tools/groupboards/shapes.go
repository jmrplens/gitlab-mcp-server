package groupboards

import (
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// Canonical output shapes mirrored from client-go sub-objects. Per the 1:1
// audit policy (full nested objects) these surface every identifying field of
// the SDK struct on the canonical JSON keys and are replicated here rather than
// imported from sibling packages to preserve the zero-import-cycle constraint
// (C-IMPORTS).
//
// This file covers the group-board sub-objects: the board group, milestone,
// labels and lists (gl.GroupIssueBoard) and the per-list assignee, label,
// iteration and milestone scope objects (gl.BoardList). The previously
// flattened scalars (*_id / *_name / *_title) are dropped in favor of these
// nested objects.

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

// GroupRefOutput mirrors the identifying fields of gl.Group as embedded in a
// gl.GroupIssueBoard. The full Group payload carries dozens of administrative,
// statistics, LDAP/SAML, and runner fields that are not relevant in board
// context; the identifying subset matches the mrapprovals.GroupOutput shape.
type GroupRefOutput struct {
	ID                   int64  `json:"id"`
	Name                 string `json:"name"`
	Path                 string `json:"path"`
	FullPath             string `json:"full_path,omitempty"`
	FullName             string `json:"full_name,omitempty"`
	Description          string `json:"description,omitempty"`
	Visibility           string `json:"visibility,omitempty"`
	WebURL               string `json:"web_url,omitempty"`
	AvatarURL            string `json:"avatar_url,omitempty"`
	ParentID             int64  `json:"parent_id,omitempty"`
	RequestAccessEnabled bool   `json:"request_access_enabled"`
	LFSEnabled           bool   `json:"lfs_enabled"`
	CreatedAt            string `json:"created_at,omitempty"`
}

func groupRefOutput(g *gl.Group) *GroupRefOutput {
	if g == nil {
		return nil
	}
	return &GroupRefOutput{
		ID: g.ID, Name: g.Name, Path: g.Path, FullPath: g.FullPath, FullName: g.FullName,
		Description: g.Description, Visibility: string(g.Visibility), WebURL: g.WebURL,
		AvatarURL: g.AvatarURL, ParentID: g.ParentID,
		RequestAccessEnabled: g.RequestAccessEnabled, LFSEnabled: g.LFSEnabled,
		CreatedAt: formatTimePtr(g.CreatedAt),
	}
}

// MilestoneOutput mirrors gl.Milestone (the board / list milestone scope).
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
		StartDate: formatISOTimePtr(m.StartDate), DueDate: formatISOTimePtr(m.DueDate),
		CreatedAt: formatTimePtr(m.CreatedAt), UpdatedAt: formatTimePtr(m.UpdatedAt),
		Expired: m.Expired,
	}
}

// LabelOutput mirrors gl.Label (a board scope label or a list label column).
// gl.GroupLabel is a defined type identical to gl.Label, so the same shape
// covers both the board's GroupLabel slice and a list's Label.
type LabelOutput struct {
	ID                     int64  `json:"id"`
	Name                   string `json:"name"`
	Color                  string `json:"color"`
	TextColor              string `json:"text_color,omitempty"`
	Description            string `json:"description,omitempty"`
	OpenIssuesCount        int64  `json:"open_issues_count,omitempty"`
	ClosedIssuesCount      int64  `json:"closed_issues_count,omitempty"`
	OpenMergeRequestsCount int64  `json:"open_merge_requests_count,omitempty"`
	Subscribed             bool   `json:"subscribed,omitempty"`
	Priority               *int64 `json:"priority,omitempty"`
	IsProjectLabel         bool   `json:"is_project_label,omitempty"`
	Archived               bool   `json:"archived,omitempty"`
}

func labelOutput(l *gl.Label) *LabelOutput {
	if l == nil {
		return nil
	}
	out := &LabelOutput{
		ID: l.ID, Name: l.Name, Color: l.Color, TextColor: l.TextColor,
		Description: l.Description, OpenIssuesCount: l.OpenIssuesCount,
		ClosedIssuesCount: l.ClosedIssuesCount, OpenMergeRequestsCount: l.OpenMergeRequestsCount,
		Subscribed: l.Subscribed, IsProjectLabel: l.IsProjectLabel, Archived: l.Archived,
	}
	if p, err := l.Priority.Get(); err == nil {
		out.Priority = &p
	}
	return out
}

// groupLabelOutputs converts a slice of gl.GroupLabel (a defined type aliasing
// gl.Label) to LabelOutput, skipping nil elements.
func groupLabelOutputs(labels []*gl.GroupLabel) []*LabelOutput {
	if len(labels) == 0 {
		return nil
	}
	out := make([]*LabelOutput, 0, len(labels))
	for _, l := range labels {
		if l == nil {
			continue
		}
		lbl := gl.Label(*l)
		out = append(out, labelOutput(&lbl))
	}
	return out
}

// BoardListAssigneeOutput mirrors gl.BoardListAssignee (a list assignee scope).
type BoardListAssigneeOutput struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
}

func boardListAssigneeOutput(a *gl.BoardListAssignee) *BoardListAssigneeOutput {
	if a == nil {
		return nil
	}
	return &BoardListAssigneeOutput{ID: a.ID, Name: a.Name, Username: a.Username}
}

// IterationOutput mirrors gl.ProjectIteration (a list iteration scope).
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

func iterationOutput(it *gl.ProjectIteration) *IterationOutput {
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
