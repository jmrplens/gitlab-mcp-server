package boards

import (
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
// Canonical shape shared via toolutil.
type BasicUserOutput = toolutil.BoardUserOutput

// BoardListAssigneeOutput is a documented reference subset per
// doc/api/boards.md. A board list's `assignee` object (Premium/Ultimate
// assignee list type) is surfaced with id, name, and username, matching the
// compact gl.BoardListAssignee struct. Canonical shape shared via toolutil.
type BoardListAssigneeOutput = toolutil.BoardListAssigneeOutput

// LabelOutput is a documented reference subset per doc/api/boards.md.
// Every documented board-list response shows the list's `label` object with
// only name, color, and description. Canonical shape shared via toolutil.
type LabelOutput = toolutil.BoardLabelOutput

// LabelDetailsOutput is a documented reference subset per doc/api/boards.md.
// The board's `labels[]` entries in the documented update-board response show
// only id, name, color, and description. Canonical shape shared via toolutil.
type LabelDetailsOutput = toolutil.BoardLabelDetailsOutput

// IterationOutput is a documented reference subset per doc/api/boards.md.
// A board list's `iteration` object (Premium/Ultimate iteration list type)
// mirrors gl.ProjectIteration. Canonical shape shared via toolutil.
type IterationOutput = toolutil.IterationOutput

// basicUserOutput converts a gl.BasicUser into the documented board assignee
// subset, or nil.
func basicUserOutput(u *gl.BasicUser) *BasicUserOutput {
	return toolutil.NewBoardUserOutput(u)
}

// boardListAssigneeOutput converts a gl.BoardListAssignee into the shared
// output shape, or nil.
func boardListAssigneeOutput(a *gl.BoardListAssignee) *BoardListAssigneeOutput {
	return toolutil.NewBoardListAssigneeOutput(a)
}

// labelOutput converts a gl.Label into the documented board-list label
// subset, or nil.
func labelOutput(l *gl.Label) *LabelOutput {
	return toolutil.NewBoardLabelOutput(l)
}

// labelDetailsOutputs converts a slice of gl.LabelDetails into the documented
// board label subset.
func labelDetailsOutputs(details []*gl.LabelDetails) []*LabelDetailsOutput {
	return toolutil.NewBoardLabelDetailsOutputs(details)
}

// iterationOutput converts a gl.ProjectIteration into the canonical iteration
// object, or nil.
func iterationOutput(it *gl.ProjectIteration) *IterationOutput {
	return toolutil.NewIterationOutputFromProjectIteration(it)
}
