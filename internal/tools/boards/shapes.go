package boards

import (
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// Canonical output shapes mirrored from client-go sub-objects. Per the 1:1
// audit policy (full nested objects on canonical keys) these surface every
// field of the SDK struct and are replicated here rather than imported from
// sibling packages to preserve the zero-import-cycle constraint (C-IMPORTS).
//
// This file covers the issue-board sub-objects surfaced on the canonical json
// keys: project, milestone, assignee, label, iteration, and label details.

// ProjectOutput is a documented reference subset per doc/api/boards.md.
// The boards API embeds a project reference on the board's `project` key that
// contains only the fields the documented response examples list (identity,
// repo URLs, and the extended fields shown in the update example) — not the
// full gl.Project payload.
type ProjectOutput struct {
	ID                int64    `json:"id"`
	Name              string   `json:"name,omitempty"`
	NameWithNamespace string   `json:"name_with_namespace,omitempty"`
	Path              string   `json:"path,omitempty"`
	PathWithNamespace string   `json:"path_with_namespace,omitempty"`
	HTTPURLToRepo     string   `json:"http_url_to_repo,omitempty"`
	WebURL            string   `json:"web_url,omitempty"`
	CreatedAt         string   `json:"created_at,omitempty"`
	DefaultBranch     string   `json:"default_branch,omitempty"`
	TagList           []string `json:"tag_list,omitempty"`
	Topics            []string `json:"topics,omitempty"`
	SSHURLToRepo      string   `json:"ssh_url_to_repo,omitempty"`
	ReadmeURL         string   `json:"readme_url,omitempty"`
	AvatarURL         string   `json:"avatar_url,omitempty"`
	StarCount         int64    `json:"star_count,omitempty"`
	ForksCount        int64    `json:"forks_count,omitempty"`
	LastActivityAt    string   `json:"last_activity_at,omitempty"`
}

func projectOutput(p *gl.Project) *ProjectOutput {
	if p == nil {
		return nil
	}
	return &ProjectOutput{
		ID: p.ID, Name: p.Name, NameWithNamespace: p.NameWithNamespace,
		Path: p.Path, PathWithNamespace: p.PathWithNamespace,
		HTTPURLToRepo: p.HTTPURLToRepo, WebURL: p.WebURL,
		CreatedAt: toolutil.FormatTimePtr(p.CreatedAt), DefaultBranch: p.DefaultBranch,
		//nolint:staticcheck // tag_list is documented (deprecated alias of topics) in doc/api/boards.md
		TagList: p.TagList, Topics: p.Topics, SSHURLToRepo: p.SSHURLToRepo,
		ReadmeURL: p.ReadmeURL, AvatarURL: p.AvatarURL,
		StarCount: p.StarCount, ForksCount: p.ForksCount,
		LastActivityAt: toolutil.FormatTimePtr(p.LastActivityAt),
	}
}

// MilestoneOutput is a documented reference subset per doc/api/boards.md.
// The board/list `milestone` object surfaces only the fields the documented
// update-board response lists (id, iid, project_id, title, description, state,
// timestamps, dates, web_url); gl.Milestone's group_id and expired are not part
// of the documented board milestone shape.
type MilestoneOutput struct {
	ID          int64  `json:"id"`
	IID         int64  `json:"iid"`
	ProjectID   int64  `json:"project_id"`
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
		ID: m.ID, IID: m.IID, ProjectID: m.ProjectID,
		Title: m.Title, Description: m.Description, State: m.State, WebURL: m.WebURL,
		StartDate: toolutil.FormatISOTimePtr(m.StartDate), DueDate: toolutil.FormatISOTimePtr(m.DueDate),
		CreatedAt: toolutil.FormatTimePtr(m.CreatedAt), UpdatedAt: toolutil.FormatTimePtr(m.UpdatedAt),
	}
}

// BasicUserOutput is a documented reference subset per doc/api/boards.md.
// The board's `assignee` object surfaces only the fields the documented
// update-board response lists (id, name, username, state, avatar_url, web_url);
// gl.BasicUser's created_at is not part of the documented board assignee shape.
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

// BoardListAssigneeOutput is a documented reference subset per
// doc/api/boards.md. A board list's `assignee` object (Premium/Ultimate
// assignee list type) is surfaced with id, name, and username, matching the
// compact gl.BoardListAssignee struct. The documented response examples cover
// only label lists, so this premium list-type sub-object has no fuller
// documented shape to trim against.
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

// LabelOutput is a documented reference subset per doc/api/boards.md.
// Every documented board-list response shows the list's `label` object with
// only name, color, and description; gl.Label's id, text_color, counts,
// subscribed, priority, is_project_label, and archived fields are not part of
// the documented board-list label shape.
type LabelOutput struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

func labelOutput(l *gl.Label) *LabelOutput {
	if l == nil {
		return nil
	}
	return &LabelOutput{
		Name: l.Name, Color: l.Color, Description: l.Description,
	}
}

// LabelDetailsOutput is a documented reference subset per doc/api/boards.md.
// The board's `labels[]` entries in the documented update-board response show
// only id, name, color, and description; gl.LabelDetails's description_html and
// text_color are not part of the documented board labels shape.
type LabelDetailsOutput struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
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
		})
	}
	return out
}

// IterationOutput is a documented reference subset per doc/api/boards.md.
// A board list's `iteration` object (Premium/Ultimate iteration list type)
// mirrors gl.ProjectIteration. The documented response examples cover only
// label lists, so this premium list-type sub-object has no fuller documented
// shape to trim against.
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
