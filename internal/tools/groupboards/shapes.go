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
// gl.GroupIssueBoard omits the assignee field entirely. Canonical shape shared
// via toolutil.
type BasicUserOutput = toolutil.BoardUserOutput

// LabelDetailsOutput is a documented reference subset per
// doc/api/group_boards.md. The board's `labels[]` entries in the documented
// update-board response show only id, name, color, and description. Canonical
// shape shared via toolutil.
type LabelDetailsOutput = toolutil.BoardLabelDetailsOutput

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
// compact gl.BoardListAssignee struct. Canonical shape shared via toolutil.
type BoardListAssigneeOutput = toolutil.BoardListAssigneeOutput

// LabelOutput is a documented reference subset per doc/api/group_boards.md.
// Every documented board-list response shows the list's `label` object with
// only name, color, and description (no id). Canonical shape shared via
// toolutil.
type LabelOutput = toolutil.BoardLabelOutput

// IterationOutput is a documented reference subset per doc/api/group_boards.md.
// A board list's `iteration` object (Premium/Ultimate iteration list type)
// mirrors gl.ProjectIteration. Canonical shape shared via toolutil.
type IterationOutput = toolutil.IterationOutput
