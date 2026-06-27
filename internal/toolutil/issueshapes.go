// Package toolutil — issue shape types shared across the issue-cluster
// packages (issues). The four user-shape types inside the issues
// package (IssueAuthorOutput, IssueAssigneeOutput, IssueCloserOutput,
// EpicAuthorOutput) are structurally identical — the 6-field subset
// of gl.BasicUser with field names matching the issue-resource JSON
// documentation. They collapse to a single shared `IssueUserOutput`
// here; per-package call sites use the shared type directly.
//
// Issues-only shapes (no other package carries these structs):
//   - IssueLinksOutput (the GitLab _links object on an issue)
//   - IterationOutput  (gl.Iteration)
//   - EpicOutput       (gl.Epic)
package toolutil

import (
	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// IssueUserOutput is the documented 6-field user object that appears
// inside issue resources (author / assignees[] / closed_by) and inside
// the nested epic.author pointer. The fields mirror the per-resource
// JSON shape documented for issues (the full BasicUser shape omits
// `state` on this resource, so IssueUserOutput intentionally omits it
// too — see the package-local shapes.go history for the audit notes).
type IssueUserOutput struct {
	ID        int64  `json:"id"`
	State     string `json:"state"`
	WebURL    string `json:"web_url"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
	Username  string `json:"username"`
}

// NewIssueUserOutputFromBasicUser converts a gl.BasicUser into the
// canonical issue-user shape, populating only the documented fields.
func NewIssueUserOutputFromBasicUser(u gl.BasicUser) IssueUserOutput {
	return IssueUserOutput{
		ID:        u.ID,
		State:     u.State,
		WebURL:    u.WebURL,
		Name:      u.Name,
		AvatarURL: u.AvatarURL,
		Username:  u.Username,
	}
}

// NewIssueUserOutputFromPointer converts a *gl.BasicUser, returning
// nil for a nil source so call sites can pass through SDK pointers
// without an extra nil check.
func NewIssueUserOutputFromPointer(u *gl.BasicUser) *IssueUserOutput {
	if u == nil {
		return nil
	}
	v := NewIssueUserOutputFromBasicUser(*u)
	return &v
}

// NewIssueUserOutputFromEpicAuthor converts a *gl.EpicAuthor pointer
// into the canonical issue-user shape. gl.EpicAuthor has the same 6
// field layout as gl.BasicUser so the conversion is structural — the
// type distinction in the SDK is purely for documentation.
func NewIssueUserOutputFromEpicAuthor(u *gl.EpicAuthor) *IssueUserOutput {
	if u == nil {
		return nil
	}
	return &IssueUserOutput{
		ID:        u.ID,
		State:     u.State,
		WebURL:    u.WebURL,
		Name:      u.Name,
		AvatarURL: u.AvatarURL,
		Username:  u.Username,
	}
}

// NewIssueUserOutputFromIssueAuthor converts a *gl.IssueAuthor pointer
// into the canonical issue-user shape. gl.IssueAuthor is the dedicated
// author type on gl.Issue (distinct from gl.BasicUser so the documented
// "issues have an author" reference is preserved) — fields are the
// same 6 we surface elsewhere.
func NewIssueUserOutputFromIssueAuthor(u *gl.IssueAuthor) *IssueUserOutput {
	if u == nil {
		return nil
	}
	return &IssueUserOutput{
		ID:        u.ID,
		State:     u.State,
		WebURL:    u.WebURL,
		Name:      u.Name,
		AvatarURL: u.AvatarURL,
		Username:  u.Username,
	}
}

// NewIssueUserOutputsFromIssueAssignees converts a slice of
// gl.IssueAssignee, skipping nil entries and returning nil for an
// empty / all-nil input.
func NewIssueUserOutputsFromIssueAssignees(as []*gl.IssueAssignee) []*IssueUserOutput {
	if len(as) == 0 {
		return nil
	}
	out := make([]*IssueUserOutput, 0, len(as))
	for _, a := range as {
		if a == nil {
			continue
		}
		out = append(out, &IssueUserOutput{
			ID:        a.ID,
			State:     a.State,
			WebURL:    a.WebURL,
			Name:      a.Name,
			AvatarURL: a.AvatarURL,
			Username:  a.Username,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// NewIssueUserOutputFromIssueAssignee converts a single *gl.IssueAssignee
// pointer into the canonical issue-user shape (used for the deprecated
// singular Assignee field on gl.Issue).
func NewIssueUserOutputFromIssueAssignee(a *gl.IssueAssignee) *IssueUserOutput {
	if a == nil {
		return nil
	}
	return &IssueUserOutput{
		ID:        a.ID,
		State:     a.State,
		WebURL:    a.WebURL,
		Name:      a.Name,
		AvatarURL: a.AvatarURL,
		Username:  a.Username,
	}
}

// NewIssueUserOutputFromIssueCloser converts a *gl.IssueCloser pointer
// into the canonical issue-user shape. Same 6-field layout as the
// other issue-user types.
func NewIssueUserOutputFromIssueCloser(u *gl.IssueCloser) *IssueUserOutput {
	if u == nil {
		return nil
	}
	return &IssueUserOutput{
		ID:        u.ID,
		State:     u.State,
		WebURL:    u.WebURL,
		Name:      u.Name,
		AvatarURL: u.AvatarURL,
		Username:  u.Username,
	}
}

// IssueLinksOutput mirrors gl.Links (the _links object on an issue).
// Field set is intentionally smaller than the release _links object in
// toolutil.LinksOutput — issues expose only self / notes / award_emoji /
// project URLs.
type IssueLinksOutput struct {
	Self       string `json:"self"`
	Notes      string `json:"notes"`
	AwardEmoji string `json:"award_emoji"`
	Project    string `json:"project"`
}

// NewIssueLinksOutput converts a gl.IssueLinks value into the canonical
// issue-links object, returning nil for a nil source.
func NewIssueLinksOutput(l *gl.IssueLinks) *IssueLinksOutput {
	if l == nil {
		return nil
	}
	return &IssueLinksOutput{
		Self:       l.Self,
		Notes:      l.Notes,
		AwardEmoji: l.AwardEmoji,
		Project:    l.Project,
	}
}

// IterationOutput mirrors gl.Iteration (the iteration object surfaced
// on issue.iteration). State is an int per the SDK type (an enum of
// 0=upcoming, 1=current, 2=closed).
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

// NewIterationOutput converts a gl.Iteration pointer into the
// canonical iteration object, returning nil for a nil source.
func NewIterationOutput(it *gl.Iteration) *IterationOutput {
	if it == nil {
		return nil
	}
	return &IterationOutput{
		ID:          it.ID,
		IID:         it.IID,
		Sequence:    it.Sequence,
		GroupID:     it.GroupID,
		Title:       it.Title,
		Description: it.Description,
		State:       it.State,
		WebURL:      it.WebURL,
		CreatedAt:   FormatTimePtr(it.CreatedAt),
		UpdatedAt:   FormatTimePtr(it.UpdatedAt),
		StartDate:   FormatISOTimePtr(it.StartDate),
		DueDate:     FormatISOTimePtr(it.DueDate),
	}
}

// NewIterationOutputFromGroupIteration converts a gl.GroupIteration
// pointer (the type surfaced on gl.Issue.Iteration) into the canonical
// iteration object. gl.GroupIteration has the same 11 fields as
// gl.Iteration but with a separate type identity for documentation.
func NewIterationOutputFromGroupIteration(it *gl.GroupIteration) *IterationOutput {
	if it == nil {
		return nil
	}
	return &IterationOutput{
		ID:          it.ID,
		IID:         it.IID,
		Sequence:    it.Sequence,
		GroupID:     it.GroupID,
		Title:       it.Title,
		Description: it.Description,
		State:       it.State,
		WebURL:      it.WebURL,
		CreatedAt:   FormatTimePtr(it.CreatedAt),
		UpdatedAt:   FormatTimePtr(it.UpdatedAt),
		StartDate:   FormatISOTimePtr(it.StartDate),
		DueDate:     FormatISOTimePtr(it.DueDate),
	}
}

// EpicOutput mirrors gl.Epic (the nested epic object surfaced on
// issue.epic). The nested EpicAuthorOutput is the same IssueUserOutput
// shape used elsewhere in the issues package.
type EpicOutput struct {
	ID                      int64            `json:"id"`
	IID                     int64            `json:"iid"`
	GroupID                 int64            `json:"group_id"`
	ParentID                int64            `json:"parent_id"`
	Title                   string           `json:"title"`
	Description             string           `json:"description"`
	State                   string           `json:"state"`
	Confidential            bool             `json:"confidential"`
	WebURL                  string           `json:"web_url"`
	URL                     string           `json:"url"`
	Author                  *IssueUserOutput `json:"author,omitempty"`
	Labels                  []string         `json:"labels,omitempty"`
	Upvotes                 int64            `json:"upvotes,omitempty"`
	Downvotes               int64            `json:"downvotes,omitempty"`
	UserNotesCount          int64            `json:"user_notes_count,omitempty"`
	StartDate               string           `json:"start_date,omitempty"`
	StartDateIsFixed        bool             `json:"start_date_is_fixed,omitempty"`
	StartDateFixed          string           `json:"start_date_fixed,omitempty"`
	StartDateFromMilestones string           `json:"start_date_from_milestones,omitempty"`
	DueDate                 string           `json:"due_date,omitempty"`
	DueDateIsFixed          bool             `json:"due_date_is_fixed,omitempty"`
	DueDateFixed            string           `json:"due_date_fixed,omitempty"`
	DueDateFromMilestones   string           `json:"due_date_from_milestones,omitempty"`
	CreatedAt               string           `json:"created_at,omitempty"`
	UpdatedAt               string           `json:"updated_at,omitempty"`
	ClosedAt                string           `json:"closed_at,omitempty"`
}

// NewEpicOutput converts a gl.Epic pointer into the canonical epic
// object, returning nil for a nil source.
func NewEpicOutput(e *gl.Epic) *EpicOutput {
	if e == nil {
		return nil
	}
	return &EpicOutput{
		ID:                      e.ID,
		IID:                     e.IID,
		GroupID:                 e.GroupID,
		ParentID:                e.ParentID,
		Title:                   e.Title,
		Description:             e.Description,
		State:                   e.State,
		Confidential:            e.Confidential,
		WebURL:                  e.WebURL,
		URL:                     e.URL,
		Author:                  NewIssueUserOutputFromEpicAuthor(e.Author),
		Labels:                  e.Labels,
		Upvotes:                 e.Upvotes,
		Downvotes:               e.Downvotes,
		UserNotesCount:          e.UserNotesCount,
		StartDate:               FormatISOTimePtr(e.StartDate),
		StartDateIsFixed:        e.StartDateIsFixed,
		StartDateFixed:          FormatISOTimePtr(e.StartDateFixed),
		StartDateFromMilestones: FormatISOTimePtr(e.StartDateFromMilestones),
		DueDate:                 FormatISOTimePtr(e.DueDate),
		DueDateIsFixed:          e.DueDateIsFixed,
		DueDateFixed:            FormatISOTimePtr(e.DueDateFixed),
		DueDateFromMilestones:   FormatISOTimePtr(e.DueDateFromMilestones),
		CreatedAt:               FormatTimePtr(e.CreatedAt),
		UpdatedAt:               FormatTimePtr(e.UpdatedAt),
		ClosedAt:                FormatTimePtr(e.ClosedAt),
	}
}
