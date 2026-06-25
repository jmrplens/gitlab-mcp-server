package groupepicboards

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
// This file covers the group-epic-board sub-objects: the board group
// (gl.Group), the board scope labels (gl.LabelDetails), and the per-list
// assignee, label, iteration and milestone scope objects (gl.BoardList). Unlike
// the group issue board, gl.GroupEpicBoard carries its labels as
// gl.LabelDetails (a distinct, smaller shape than gl.Label), so a dedicated
// LabelDetailsOutput mirror is used for the board scope labels while the list
// columns keep the full gl.Label mirror via gl.BoardList.

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
// gl.GroupEpicBoard. The full Group payload carries dozens of administrative,
// statistics, LDAP/SAML, and runner fields that are not relevant in board
// context; the identifying subset matches the groupboards.GroupRefOutput shape.
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

// LabelDetailsOutput mirrors gl.LabelDetails (the epic board scope labels).
// gl.GroupEpicBoard.Labels is []*LabelDetails, a smaller shape than gl.Label,
// so this mirror carries exactly the LabelDetails fields.
type LabelDetailsOutput struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	Color           string `json:"color"`
	Description     string `json:"description,omitempty"`
	DescriptionHTML string `json:"description_html,omitempty"`
	TextColor       string `json:"text_color,omitempty"`
}

func labelDetailsOutput(l *gl.LabelDetails) *LabelDetailsOutput {
	if l == nil {
		return nil
	}
	return &LabelDetailsOutput{
		ID: l.ID, Name: l.Name, Color: l.Color, Description: l.Description,
		DescriptionHTML: l.DescriptionHTML, TextColor: l.TextColor,
	}
}

// labelDetailsOutputs converts a slice of gl.LabelDetails to LabelDetailsOutput,
// skipping nil elements.
func labelDetailsOutputs(labels []*gl.LabelDetails) []*LabelDetailsOutput {
	if len(labels) == 0 {
		return nil
	}
	out := make([]*LabelDetailsOutput, 0, len(labels))
	for _, l := range labels {
		if l == nil {
			continue
		}
		out = append(out, labelDetailsOutput(l))
	}
	return out
}

// LabelOutput mirrors gl.Label (a board list label column scope).
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

// MilestoneOutput mirrors gl.Milestone (the list milestone scope).
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

// BoardListOutput represents a single list within a group epic board. Nested
// objects (assignee, label, iteration, milestone) mirror the full client-go
// gl.BoardList sub-objects per the 1:1 audit policy.
type BoardListOutput struct {
	ID             int64                    `json:"id"`
	Assignee       *BoardListAssigneeOutput `json:"assignee,omitempty"`
	Iteration      *IterationOutput         `json:"iteration,omitempty"`
	Label          *LabelOutput             `json:"label,omitempty"`
	MaxIssueCount  int64                    `json:"max_issue_count,omitempty"`
	MaxIssueWeight int64                    `json:"max_issue_weight,omitempty"`
	Milestone      *MilestoneOutput         `json:"milestone,omitempty"`
	Position       int64                    `json:"position"`
}

// convertBoardList maps a GitLab board list into epic board MCP output.
func convertBoardList(l *gl.BoardList) BoardListOutput {
	if l == nil {
		return BoardListOutput{}
	}
	return BoardListOutput{
		ID:             l.ID,
		Assignee:       boardListAssigneeOutput(l.Assignee),
		Iteration:      iterationOutput(l.Iteration),
		Label:          labelOutput(l.Label),
		MaxIssueCount:  l.MaxIssueCount,
		MaxIssueWeight: l.MaxIssueWeight,
		Milestone:      milestoneOutput(l.Milestone),
		Position:       l.Position,
	}
}
