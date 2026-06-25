package pipelines

import (
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// Canonical output shapes mirrored from client-go sub-objects. Per the 1:1
// audit policy (full nested objects) these surface every field of the SDK
// struct and are replicated here rather than imported from sibling packages to
// preserve the zero-import-cycle constraint (C-IMPORTS).
//
// This file covers the pipeline sub-objects surfaced on the canonical json keys
// (user, detailed_status). The old flattened user_username scalar is removed
// because the data now lives in the user object.

// formatTimePtr renders an optional timestamp as RFC 3339, or "" when nil.
func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

// BasicUserOutput mirrors gl.BasicUser, the compact user object embedded in the
// pipeline payload under the "user" key. It surfaces every field of the SDK
// struct so callers no longer need the removed flattened user_username scalar.
type BasicUserOutput struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	State     string `json:"state"`
	AvatarURL string `json:"avatar_url"`
	WebURL    string `json:"web_url"`
	CreatedAt string `json:"created_at,omitempty"`
}

// basicUserOutput converts a single gl.BasicUser to its output shape, returning
// nil when the SDK value is nil.
func basicUserOutput(u *gl.BasicUser) *BasicUserOutput {
	if u == nil {
		return nil
	}
	return &BasicUserOutput{
		ID: u.ID, Username: u.Username, Name: u.Name, State: u.State,
		AvatarURL: u.AvatarURL, WebURL: u.WebURL, CreatedAt: formatTimePtr(u.CreatedAt),
	}
}

// IllustrationOutput mirrors gl.DetailedStatusIllustration (the optional status
// illustration embedded in detailed_status).
type IllustrationOutput struct {
	Image string `json:"image,omitempty"`
}

// StatusOutput mirrors gl.DetailedStatus (the pipeline detailed_status object).
type StatusOutput struct {
	Icon         string              `json:"icon"`
	Text         string              `json:"text"`
	Label        string              `json:"label"`
	Group        string              `json:"group"`
	Tooltip      string              `json:"tooltip"`
	HasDetails   bool                `json:"has_details"`
	DetailsPath  string              `json:"details_path,omitempty"`
	Illustration *IllustrationOutput `json:"illustration,omitempty"`
	Favicon      string              `json:"favicon,omitempty"`
}

// detailedStatusOutput converts a gl.DetailedStatus to its output shape,
// returning nil when the SDK value is nil.
func detailedStatusOutput(s *gl.DetailedStatus) *StatusOutput {
	if s == nil {
		return nil
	}
	out := &StatusOutput{
		Icon: s.Icon, Text: s.Text, Label: s.Label, Group: s.Group,
		Tooltip: s.Tooltip, HasDetails: s.HasDetails, DetailsPath: s.DetailsPath,
		Favicon: s.Favicon,
	}
	if s.Illustration.Image != "" {
		out.Illustration = &IllustrationOutput{Image: s.Illustration.Image}
	}
	return out
}
