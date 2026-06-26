package groupboards

import (
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical output shapes mirrored from client-go sub-objects, trimmed to the
// documented response shapes. Per the bidirectional 1:1 audit each nested
// reference object surfaces exactly the sub-fields the GitLab group_boards API
// documents (doc/api/group_boards.md) rather than the full SDK struct, and is
// replicated here rather than imported from sibling packages to preserve the
// zero-import-cycle constraint (C-IMPORTS).
//
// This file covers the group-board sub-objects: the board group, milestone,
// assignee, labels and lists (gl.GroupIssueBoard) and the per-list assignee,
// label, iteration and milestone scope objects (gl.BoardList).

// GroupRefOutput is a documented reference subset per doc/api/group_boards.md.
// Every documented group-board response shows the board's `group` object with
// only id, name and web_url; gl.Group's remaining administrative, statistics,
// LDAP/SAML, and runner fields are not part of the documented board group shape.
type GroupRefOutput struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	WebURL string `json:"web_url,omitempty"`
}

func groupRefOutput(g *gl.Group) *GroupRefOutput {
	if g == nil {
		return nil
	}
	return &GroupRefOutput{ID: g.ID, Name: g.Name, WebURL: g.WebURL}
}

// MilestoneOutput is a documented reference subset per doc/api/group_boards.md.
// The board/list `milestone` object surfaces only the fields the documented
// update-board and create-list responses list (id, iid, group_id, title,
// description, state, timestamps, dates, web_url); gl.Milestone's project_id and
// expired are not part of the documented group-board milestone shape.
type MilestoneOutput struct {
	ID          int64  `json:"id"`
	IID         int64  `json:"iid"`
	GroupID     int64  `json:"group_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	State       string `json:"state"`
	WebURL      string `json:"web_url"`
	StartDate   string `json:"start_date,omitempty"`
	DueDate     string `json:"due_date,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

func milestoneOutput(m *gl.Milestone) *MilestoneOutput {
	if m == nil {
		return nil
	}
	return &MilestoneOutput{
		ID: m.ID, IID: m.IID, GroupID: m.GroupID,
		Title: m.Title, Description: m.Description, State: m.State, WebURL: m.WebURL,
		StartDate: toolutil.FormatISOTimePtr(m.StartDate), DueDate: toolutil.FormatISOTimePtr(m.DueDate),
		CreatedAt: toolutil.FormatTimePtr(m.CreatedAt), UpdatedAt: toolutil.FormatTimePtr(m.UpdatedAt),
	}
}

// BasicUserOutput is a documented reference subset per doc/api/group_boards.md.
// The board's `assignee` object in the documented update-board response shows
// only id, name, username, state, avatar_url, and web_url; gl.BasicUser's
// created_at is not part of the documented board assignee shape. The board-level
// assignee is decoded via the raw-superset fetch path (groupIssueBoardAPI), as
// gl.GroupIssueBoard omits the assignee field entirely.
type BasicUserOutput struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	State     string `json:"state"`
	AvatarURL string `json:"avatar_url"`
	WebURL    string `json:"web_url"`
}

func basicUserOutput(u *gl.BasicUser) *BasicUserOutput {
	if u == nil {
		return nil
	}
	return &BasicUserOutput{
		ID: u.ID, Username: u.Username, Name: u.Name, State: u.State,
		AvatarURL: u.AvatarURL, WebURL: u.WebURL,
	}
}

// LabelDetailsOutput is a documented reference subset per
// doc/api/group_boards.md. The board's `labels[]` entries in the documented
// update-board response show only id, name, color, and description; gl.Label's
// text_color, counts, subscribed, priority, is_project_label, and archived
// fields are not part of the documented board labels shape.
type LabelDetailsOutput struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

// groupLabelOutputs converts a slice of gl.GroupLabel (a defined type aliasing
// gl.Label) to the documented board label subset, skipping nil elements.
func groupLabelOutputs(labels []*gl.GroupLabel) []*LabelDetailsOutput {
	if len(labels) == 0 {
		return nil
	}
	out := make([]*LabelDetailsOutput, 0, len(labels))
	for _, l := range labels {
		if l == nil {
			continue
		}
		out = append(out, &LabelDetailsOutput{
			ID: l.ID, Name: l.Name, Color: l.Color, Description: l.Description,
		})
	}
	return out
}

// BoardListAssigneeOutput is a documented reference subset per
// doc/api/group_boards.md. A board list's `assignee` object (Premium/Ultimate
// assignee list type) is surfaced with id, name, and username, matching the
// compact gl.BoardListAssignee struct. The documented response examples cover
// only label and milestone lists, so this premium list-type sub-object has no
// fuller documented shape to trim against.
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

// LabelOutput is a documented reference subset per doc/api/group_boards.md.
// Every documented board-list response shows the list's `label` object with
// only name, color, and description (no id); gl.Label's remaining fields are not
// part of the documented board-list label shape.
type LabelOutput struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

func labelOutput(l *gl.Label) *LabelOutput {
	if l == nil {
		return nil
	}
	return &LabelOutput{Name: l.Name, Color: l.Color, Description: l.Description}
}

// IterationOutput is a documented reference subset per doc/api/group_boards.md.
// A board list's `iteration` object (Premium/Ultimate iteration list type)
// mirrors gl.ProjectIteration. The documented response examples cover only
// label and milestone lists, so this premium list-type sub-object has no fuller
// documented shape to trim against.
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
		CreatedAt: toolutil.FormatTimePtr(it.CreatedAt), UpdatedAt: toolutil.FormatTimePtr(it.UpdatedAt),
		StartDate: toolutil.FormatISOTimePtr(it.StartDate), DueDate: toolutil.FormatISOTimePtr(it.DueDate),
	}
}
