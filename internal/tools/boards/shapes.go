package boards

import (
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// Canonical output shapes mirrored from client-go sub-objects. Per the 1:1
// audit policy (full nested objects on canonical keys) these surface every
// field of the SDK struct and are replicated here rather than imported from
// sibling packages to preserve the zero-import-cycle constraint (C-IMPORTS).
//
// This file covers the issue-board sub-objects surfaced on the canonical json
// keys: project, milestone, assignee, label, iteration, and label details.

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

// ProjectOutput mirrors the compact project reference returned on the board's
// `project` key. The boards API returns only the project's identity fields
// (id, name, path, web URL, timestamps), not the full gl.Project payload.
type ProjectOutput struct {
	ID                int64  `json:"id"`
	Name              string `json:"name,omitempty"`
	NameWithNamespace string `json:"name_with_namespace,omitempty"`
	Path              string `json:"path,omitempty"`
	PathWithNamespace string `json:"path_with_namespace,omitempty"`
	WebURL            string `json:"web_url,omitempty"`
	Description       string `json:"description,omitempty"`
	CreatedAt         string `json:"created_at,omitempty"`
}

func projectOutput(p *gl.Project) *ProjectOutput {
	if p == nil {
		return nil
	}
	return &ProjectOutput{
		ID: p.ID, Name: p.Name, NameWithNamespace: p.NameWithNamespace,
		Path: p.Path, PathWithNamespace: p.PathWithNamespace, WebURL: p.WebURL,
		Description: p.Description, CreatedAt: formatTimePtr(p.CreatedAt),
	}
}

// MilestoneOutput mirrors gl.Milestone (the board/list milestone object).
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

// BasicUserOutput mirrors gl.BasicUser, the compact user object surfaced on the
// board's `assignee` key.
type BasicUserOutput struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	State     string `json:"state"`
	AvatarURL string `json:"avatar_url"`
	WebURL    string `json:"web_url"`
	CreatedAt string `json:"created_at,omitempty"`
}

func basicUserOutput(u *gl.BasicUser) *BasicUserOutput {
	if u == nil {
		return nil
	}
	return &BasicUserOutput{
		ID: u.ID, Username: u.Username, Name: u.Name, State: u.State,
		AvatarURL: u.AvatarURL, WebURL: u.WebURL, CreatedAt: formatTimePtr(u.CreatedAt),
	}
}

// BoardListAssigneeOutput mirrors gl.BoardListAssignee, the compact assignee
// object surfaced on a board list's `assignee` key.
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

// LabelOutput mirrors gl.Label (the board list's `label` object).
type LabelOutput struct {
	ID                     int64  `json:"id"`
	Name                   string `json:"name"`
	Color                  string `json:"color"`
	TextColor              string `json:"text_color"`
	Description            string `json:"description"`
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
	if l.Priority.IsSpecified() && !l.Priority.IsNull() {
		v := l.Priority.MustGet()
		out.Priority = &v
	}
	return out
}

// LabelDetailsOutput mirrors gl.LabelDetails (the board's `labels[]` objects).
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

// IterationOutput mirrors gl.ProjectIteration (the board list's `iteration`).
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
