package pipelinetriggers

import (
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// Canonical output shapes mirrored from client-go sub-objects. Per the 1:1
// audit policy (full nested objects) these surface every field of the SDK
// struct on the canonical json keys and are replicated here rather than
// imported from sibling packages to preserve the zero-import-cycle
// constraint (C-IMPORTS).
//
// This file covers the pipeline-trigger sub-objects:
//   - owner   (gl.User, the trigger owner)
//   - user    (gl.BasicUser, the user that started a triggered pipeline)
//   - detailed_status (gl.DetailedStatus on a triggered pipeline)

// formatTimePtr renders an optional timestamp as RFC 3339, or "" when nil.
func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

// UserOutput mirrors gl.User, the full user object returned as a pipeline
// trigger's owner. Only the stable, non-sensitive identity fields are
// surfaced; administrative and session fields (sign-in IPs, 2FA, license
// seat, identities) are intentionally omitted as they are never populated on
// the trigger owner payload.
type UserOutput struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	State     string `json:"state"`
	AvatarURL string `json:"avatar_url,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
	Email     string `json:"email,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	Bot       bool   `json:"bot,omitempty"`
	Locked    bool   `json:"locked,omitempty"`
}

// userOutput converts a gl.User to its output shape, returning nil when the
// SDK value is nil.
func userOutput(u *gl.User) *UserOutput {
	if u == nil {
		return nil
	}
	return &UserOutput{
		ID: u.ID, Username: u.Username, Name: u.Name, State: u.State,
		AvatarURL: u.AvatarURL, WebURL: u.WebURL, Email: u.Email,
		CreatedAt: formatTimePtr(u.CreatedAt), Bot: u.Bot, Locked: u.Locked,
	}
}

// BasicUserOutput mirrors gl.BasicUser, the compact user object embedded on a
// triggered pipeline (the user that started it).
type BasicUserOutput struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	State     string `json:"state"`
	AvatarURL string `json:"avatar_url,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

// basicUserOutput converts a gl.BasicUser to its output shape, returning nil
// when the SDK value is nil.
func basicUserOutput(u *gl.BasicUser) *BasicUserOutput {
	if u == nil {
		return nil
	}
	return &BasicUserOutput{
		ID: u.ID, Username: u.Username, Name: u.Name, State: u.State,
		AvatarURL: u.AvatarURL, WebURL: u.WebURL, CreatedAt: formatTimePtr(u.CreatedAt),
	}
}

// DetailedStatusOutput mirrors gl.DetailedStatus, the rich CI status object on
// a triggered pipeline.
type DetailedStatusOutput struct {
	Icon        string `json:"icon"`
	Text        string `json:"text"`
	Label       string `json:"label"`
	Group       string `json:"group"`
	Tooltip     string `json:"tooltip"`
	HasDetails  bool   `json:"has_details"`
	DetailsPath string `json:"details_path"`
	Favicon     string `json:"favicon,omitempty"`
	Image       string `json:"illustration_image,omitempty"`
}

// detailedStatusOutput converts a gl.DetailedStatus to its output shape,
// returning nil when the SDK value is nil.
func detailedStatusOutput(d *gl.DetailedStatus) *DetailedStatusOutput {
	if d == nil {
		return nil
	}
	return &DetailedStatusOutput{
		Icon: d.Icon, Text: d.Text, Label: d.Label, Group: d.Group,
		Tooltip: d.Tooltip, HasDetails: d.HasDetails, DetailsPath: d.DetailsPath,
		Favicon: d.Favicon, Image: d.Illustration.Image,
	}
}
