package issuelinks

import (
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

// Canonical output shapes mirrored from client-go sub-objects. Per the 1:1
// audit policy (full nested objects) these surface every field of the SDK
// struct and are replicated here rather than imported from sibling packages to
// preserve the zero-import-cycle constraint (C-IMPORTS).

// formatTimePtr renders an optional timestamp as RFC 3339, or "" when nil.
func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

// formatISOTimePtr renders an optional ISO date (gitlab.ISOTime) as YYYY-MM-DD,
// or "" when nil.
func formatISOTimePtr(t *gitlab.ISOTime) string {
	if t == nil {
		return ""
	}
	return time.Time(*t).Format("2006-01-02")
}

// UserOutput mirrors gitlab.IssueAuthor / gitlab.IssueAssignee (they share the
// same shape). It surfaces the full user sub-object referenced by an issue
// relation's author and assignees.
type UserOutput struct {
	ID        int64  `json:"id"`
	State     string `json:"state"`
	WebURL    string `json:"web_url"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
	Username  string `json:"username"`
}

// authorOutput converts a *gitlab.IssueAuthor to a *UserOutput, or nil when the
// SDK value is nil.
func authorOutput(a *gitlab.IssueAuthor) *UserOutput {
	if a == nil {
		return nil
	}
	return &UserOutput{
		ID: a.ID, State: a.State, WebURL: a.WebURL,
		Name: a.Name, AvatarURL: a.AvatarURL, Username: a.Username,
	}
}

// assigneeOutput converts a *gitlab.IssueAssignee to a *UserOutput, or nil when
// the SDK value is nil.
func assigneeOutput(a *gitlab.IssueAssignee) *UserOutput {
	if a == nil {
		return nil
	}
	return &UserOutput{
		ID: a.ID, State: a.State, WebURL: a.WebURL,
		Name: a.Name, AvatarURL: a.AvatarURL, Username: a.Username,
	}
}

// assigneeOutputs converts a slice of *gitlab.IssueAssignee to a slice of
// *UserOutput, skipping nil elements and returning nil for an empty input.
func assigneeOutputs(in []*gitlab.IssueAssignee) []*UserOutput {
	if len(in) == 0 {
		return nil
	}
	out := make([]*UserOutput, 0, len(in))
	for _, a := range in {
		if a == nil {
			continue
		}
		out = append(out, assigneeOutput(a))
	}
	return out
}

// ReferencesOutput mirrors gitlab.IssueReferences (the issue references object).
type ReferencesOutput struct {
	Short    string `json:"short"`
	Relative string `json:"relative"`
	Full     string `json:"full"`
}

// referencesOutput converts a *gitlab.IssueReferences to a *ReferencesOutput, or
// nil when the SDK value is nil.
func referencesOutput(r *gitlab.IssueReferences) *ReferencesOutput {
	if r == nil {
		return nil
	}
	return &ReferencesOutput{Short: r.Short, Relative: r.Relative, Full: r.Full}
}

// MilestoneOutput mirrors gitlab.Milestone (the milestone object attached to an
// issue relation).
type MilestoneOutput struct {
	ID          int64  `json:"id"`
	IID         int64  `json:"iid"`
	GroupID     int64  `json:"group_id"`
	ProjectID   int64  `json:"project_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	StartDate   string `json:"start_date,omitempty"`
	DueDate     string `json:"due_date,omitempty"`
	State       string `json:"state"`
	WebURL      string `json:"web_url"`
	UpdatedAt   string `json:"updated_at,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	Expired     *bool  `json:"expired,omitempty"`
}

// milestoneOutput converts a *gitlab.Milestone to a *MilestoneOutput, or nil
// when the SDK value is nil.
func milestoneOutput(m *gitlab.Milestone) *MilestoneOutput {
	if m == nil {
		return nil
	}
	return &MilestoneOutput{
		ID: m.ID, IID: m.IID, GroupID: m.GroupID, ProjectID: m.ProjectID,
		Title: m.Title, Description: m.Description,
		StartDate: formatISOTimePtr(m.StartDate), DueDate: formatISOTimePtr(m.DueDate),
		State: m.State, WebURL: m.WebURL,
		UpdatedAt: formatTimePtr(m.UpdatedAt), CreatedAt: formatTimePtr(m.CreatedAt),
		Expired: m.Expired,
	}
}

// IssueRefOutput mirrors the subset of gitlab.Issue surfaced for the
// source_issue and target_issue objects on a single issue link. The full Issue
// struct is large; this captures the identifying and descriptive fields that
// are useful when inspecting a link's endpoints, alongside the existing scalar
// accessors kept additively on Output.
type IssueRefOutput struct {
	ID           int64    `json:"id"`
	IID          int64    `json:"iid"`
	ProjectID    int64    `json:"project_id"`
	Title        string   `json:"title"`
	Description  string   `json:"description,omitempty"`
	State        string   `json:"state"`
	Confidential bool     `json:"confidential"`
	Labels       []string `json:"labels,omitempty"`
	WebURL       string   `json:"web_url"`
	CreatedAt    string   `json:"created_at,omitempty"`
	UpdatedAt    string   `json:"updated_at,omitempty"`
	ClosedAt     string   `json:"closed_at,omitempty"`
	DueDate      string   `json:"due_date,omitempty"`
	Weight       int64    `json:"weight,omitempty"`
}

// issueRefOutput converts a *gitlab.Issue to a *IssueRefOutput, or nil when the
// SDK value is nil.
func issueRefOutput(i *gitlab.Issue) *IssueRefOutput {
	if i == nil {
		return nil
	}
	return &IssueRefOutput{
		ID: i.ID, IID: i.IID, ProjectID: i.ProjectID, Title: i.Title,
		Description: i.Description, State: i.State, Confidential: i.Confidential,
		Labels: []string(i.Labels), WebURL: i.WebURL,
		CreatedAt: formatTimePtr(i.CreatedAt), UpdatedAt: formatTimePtr(i.UpdatedAt),
		ClosedAt: formatTimePtr(i.ClosedAt), DueDate: formatISOTimePtr(i.DueDate),
		Weight: i.Weight,
	}
}
