package pipelines

import (
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
