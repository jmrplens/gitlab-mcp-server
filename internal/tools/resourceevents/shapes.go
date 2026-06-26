package resourceevents

import (
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// Canonical output shapes mirrored from client-go sub-objects. Per the 1:1
// audit policy (full nested objects) these surface every field of the SDK
// struct and are replicated here rather than imported from sibling packages to
// preserve the zero-import-cycle constraint (C-IMPORTS).
//
// This file covers the resource-event sub-objects surfaced on the canonical
// json keys: the acting user (gl.BasicUser, embedded on every event type), the
// label (gl.LabelEventLabel), the milestone (gl.Milestone), and the iteration
// (gl.Iteration). The previously flattened user_id/username/milestone_* scalars
// are removed in favor of these full nested objects.

// EventUserOutput mirrors gl.BasicUser, the user object embedded on every
// resource event (the actor that performed the change).
type EventUserOutput struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	State     string `json:"state"`
	AvatarURL string `json:"avatar_url"`
	WebURL    string `json:"web_url"`
	CreatedAt string `json:"created_at,omitempty"`
}

// eventUserOutput converts a *gl.BasicUser to its output shape, returning nil
// when the SDK value is nil.
func eventUserOutput(u *gl.BasicUser) *EventUserOutput {
	if u == nil {
		return nil
	}
	return &EventUserOutput{
		ID: u.ID, Username: u.Username, Name: u.Name, State: u.State,
		AvatarURL: u.AvatarURL, WebURL: u.WebURL, CreatedAt: toolutil.FormatTimePtr(u.CreatedAt),
	}
}

// LabelEventLabelOutput mirrors gl.LabelEventLabel (the compact label object on
// a resource label event).
type LabelEventLabelOutput struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	TextColor   string `json:"text_color"`
	Description string `json:"description"`
}

// labelEventLabelOutput converts gl.LabelEventLabel to its output shape.
func labelEventLabelOutput(l gl.LabelEventLabel) *LabelEventLabelOutput {
	return &LabelEventLabelOutput{
		ID: l.ID, Name: l.Name, Color: l.Color,
		TextColor: l.TextColor, Description: l.Description,
	}
}

// MilestoneOutput mirrors gl.Milestone (the milestone object on a resource
// milestone event).
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

// milestoneOutput converts *gl.Milestone to its output shape, returning nil
// when the SDK value is nil.
func milestoneOutput(m *gl.Milestone) *MilestoneOutput {
	if m == nil {
		return nil
	}
	return &MilestoneOutput{
		ID: m.ID, IID: m.IID, GroupID: m.GroupID, ProjectID: m.ProjectID,
		Title: m.Title, Description: m.Description, State: m.State, WebURL: m.WebURL,
		StartDate: toolutil.FormatISOTimePtr(m.StartDate), DueDate: toolutil.FormatISOTimePtr(m.DueDate),
		CreatedAt: toolutil.FormatTimePtr(m.CreatedAt), UpdatedAt: toolutil.FormatTimePtr(m.UpdatedAt),
		Expired: m.Expired,
	}
}

// IterationOutput mirrors gl.Iteration (the iteration object on a resource
// iteration event).
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

// iterationOutput converts *gl.Iteration to its output shape, returning nil
// when the SDK value is nil.
func iterationOutput(it *gl.Iteration) *IterationOutput {
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
