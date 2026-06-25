package groupepicboards

import (
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// Canonical output shapes for group epic boards. The data source is the REST
// endpoints GET /groups/:id/epic_boards[/:board_id] served by client-go's
// gl.GroupEpicBoardsService. Per the bidirectional doc-grounded reconcile,
// every nested sub-object is trimmed to the documented sub-fields of
// doc/api/group_epic_boards.md, and documented fields that the client-go
// gl.GroupEpicBoard / gl.BoardList structs omit (board hide_*_list flags, the
// epic-board label group_id/project_id/template/created_at/updated_at, and each
// list's list_type/collapsed) are recovered through a raw REST superset fetch
// (see group_epic_boards.go) rather than from the SDK structs. Mirrors are
// replicated here rather than imported from sibling packages to preserve the
// zero-import-cycle constraint (C-IMPORTS).

// formatTimePtr renders an optional timestamp as RFC 3339, or "" when nil.
func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

// GroupRefOutput mirrors the documented `group` sub-object of an epic board.
// Documented reference subset per doc/api/group_epic_boards.md: the example
// response carries only id, name, and web_url, so the full gl.Group payload
// (dozens of administrative, statistics, LDAP/SAML, and runner fields) is
// trimmed to that identifying subset.
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

// LabelDetailsOutput mirrors the documented epic-board scope `labels` entry.
// Documented reference subset per doc/api/group_epic_boards.md: id, title,
// color, description, group_id, project_id, template, text_color, created_at,
// updated_at. The client-go gl.LabelDetails struct only decodes
// id/name/color/description/description_html/text_color, so the documented
// group_id, project_id, template, created_at, updated_at (and the documented
// `title` key) are recovered from the raw superset (labelDetailsAPI) rather
// than the SDK struct. DescriptionHTML is an additive SDK field kept on its
// canonical key; Title carries the documented `title` key alongside Name.
type LabelDetailsOutput struct {
	ID              int64  `json:"id"`
	Name            string `json:"name,omitempty"`
	Title           string `json:"title,omitempty"`
	Color           string `json:"color"`
	Description     string `json:"description,omitempty"`
	DescriptionHTML string `json:"description_html,omitempty"`
	TextColor       string `json:"text_color,omitempty"`
	GroupID         int64  `json:"group_id,omitempty"`
	ProjectID       int64  `json:"project_id,omitempty"`
	Template        bool   `json:"template,omitempty"`
	CreatedAt       string `json:"created_at,omitempty"`
	UpdatedAt       string `json:"updated_at,omitempty"`
}

// labelDetailsOutput converts a raw-superset epic-board label into output.
func labelDetailsOutput(l *labelDetailsAPI) *LabelDetailsOutput {
	if l == nil {
		return nil
	}
	return &LabelDetailsOutput{
		ID: l.ID, Name: l.Name, Title: l.Title, Color: l.Color, Description: l.Description,
		DescriptionHTML: l.DescriptionHTML, TextColor: l.TextColor,
		GroupID: l.GroupID, ProjectID: l.ProjectID, Template: l.Template,
		CreatedAt: l.CreatedAt, UpdatedAt: l.UpdatedAt,
	}
}

// labelDetailsOutputs converts a slice of raw-superset epic-board labels,
// skipping nil elements.
func labelDetailsOutputs(labels []*labelDetailsAPI) []*LabelDetailsOutput {
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

// ListLabelOutput mirrors the documented `label` column scope of an epic board
// list. Documented reference subset per doc/api/group_epic_boards.md: the list
// example carries only id, name, color, description for each label, so the full
// gl.Label payload (issue/MR counts, subscription, priority, project/archived
// flags) is trimmed to that subset.
type ListLabelOutput struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color,omitempty"`
	Description string `json:"description,omitempty"`
}

func listLabelOutput(l *gl.Label) *ListLabelOutput {
	if l == nil {
		return nil
	}
	return &ListLabelOutput{ID: l.ID, Name: l.Name, Color: l.Color, Description: l.Description}
}

// BoardListOutput represents a single list (column) within a group epic board.
// Documented reference subset per doc/api/group_epic_boards.md: the list
// example carries id, label, position, list_type, and collapsed. The client-go
// gl.BoardList struct adds assignee/iteration/milestone/max_issue_* scopes that
// the group-epic-board lists endpoint does not document, so those are trimmed;
// list_type and collapsed are documented but absent from gl.BoardList and are
// recovered through the raw superset (boardListAPI).
type BoardListOutput struct {
	ID        int64            `json:"id"`
	Label     *ListLabelOutput `json:"label,omitempty"`
	Position  int64            `json:"position"`
	ListType  string           `json:"list_type,omitempty"`
	Collapsed *bool            `json:"collapsed,omitempty"`
}

// convertBoardList maps a raw-superset epic board list into MCP output.
func convertBoardList(l *boardListAPI) BoardListOutput {
	if l == nil {
		return BoardListOutput{}
	}
	return BoardListOutput{
		ID:        l.ID,
		Label:     listLabelOutput(l.Label),
		Position:  l.Position,
		ListType:  l.ListType,
		Collapsed: l.Collapsed,
	}
}
