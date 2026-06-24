package issuenotes

import (
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// Canonical output sub-objects mirrored from client-go's note structs. Per the
// 1:1 audit policy (full nested objects) these surface every field of the SDK
// struct and are replicated here rather than imported from sibling packages to
// preserve the zero-import-cycle constraint (C-IMPORTS).

// formatTimePtr renders an optional timestamp as RFC 3339, or "" when nil.
func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

// NoteUserOutput mirrors gl.NoteAuthor / gl.NoteResolvedBy (identical shapes):
// the user who authored or resolved a note.
type NoteUserOutput struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Email     string `json:"email,omitempty"`
	Name      string `json:"name"`
	State     string `json:"state,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
}

// noteAuthorOutput converts a gl.NoteAuthor value into the additive author
// object. The author is always present on a note, so this returns a pointer to
// a populated value (never nil) to keep the canonical `author` key stable.
func noteAuthorOutput(a gl.NoteAuthor) *NoteUserOutput {
	return &NoteUserOutput{
		ID: a.ID, Username: a.Username, Email: a.Email, Name: a.Name,
		State: a.State, AvatarURL: a.AvatarURL, WebURL: a.WebURL,
	}
}

// noteResolvedByOutput converts a gl.NoteResolvedBy value into the resolved-by
// object, returning nil when no user has resolved the note (zero ID).
func noteResolvedByOutput(r gl.NoteResolvedBy) *NoteUserOutput {
	if r.ID == 0 && r.Username == "" {
		return nil
	}
	return &NoteUserOutput{
		ID: r.ID, Username: r.Username, Email: r.Email, Name: r.Name,
		State: r.State, AvatarURL: r.AvatarURL, WebURL: r.WebURL,
	}
}

// LineRangeNotePositionOutput mirrors one endpoint of gl.LineRange.
type LineRangeNotePositionOutput struct {
	LineCode string `json:"line_code,omitempty"`
	Type     string `json:"type,omitempty"`
	OldLine  int64  `json:"old_line,omitempty"`
	NewLine  int64  `json:"new_line,omitempty"`
}

// LineRangeOutput mirrors gl.LineRange (the start/end of a multi-line diff
// note position).
type LineRangeOutput struct {
	StartRange *LineRangeNotePositionOutput `json:"start_range,omitempty"`
	EndRange   *LineRangeNotePositionOutput `json:"end_range,omitempty"`
}

// NotePositionOutput mirrors gl.NotePosition: the diff position of a note that
// is attached to a specific line of a file.
type NotePositionOutput struct {
	BaseSHA      string           `json:"base_sha,omitempty"`
	StartSHA     string           `json:"start_sha,omitempty"`
	HeadSHA      string           `json:"head_sha,omitempty"`
	PositionType string           `json:"position_type,omitempty"`
	NewPath      string           `json:"new_path,omitempty"`
	NewLine      int64            `json:"new_line,omitempty"`
	OldPath      string           `json:"old_path,omitempty"`
	OldLine      int64            `json:"old_line,omitempty"`
	LineRange    *LineRangeOutput `json:"line_range,omitempty"`
}

// notePositionOutput converts a *gl.NotePosition into the position object,
// returning nil when the note has no diff position.
func notePositionOutput(p *gl.NotePosition) *NotePositionOutput {
	if p == nil {
		return nil
	}
	out := &NotePositionOutput{
		BaseSHA: p.BaseSHA, StartSHA: p.StartSHA, HeadSHA: p.HeadSHA,
		PositionType: p.PositionType, NewPath: p.NewPath, NewLine: p.NewLine,
		OldPath: p.OldPath, OldLine: p.OldLine,
		LineRange: lineRangeOutput(p.LineRange),
	}
	return out
}

// lineRangeOutput converts a *gl.LineRange into the line-range object,
// returning nil when absent.
func lineRangeOutput(lr *gl.LineRange) *LineRangeOutput {
	if lr == nil {
		return nil
	}
	out := &LineRangeOutput{}
	if lr.StartRange != nil {
		out.StartRange = &LineRangeNotePositionOutput{
			LineCode: lr.StartRange.LineCode, Type: lr.StartRange.Type,
			OldLine: lr.StartRange.OldLine, NewLine: lr.StartRange.NewLine,
		}
	}
	if lr.EndRange != nil {
		out.EndRange = &LineRangeNotePositionOutput{
			LineCode: lr.EndRange.LineCode, Type: lr.EndRange.Type,
			OldLine: lr.EndRange.OldLine, NewLine: lr.EndRange.NewLine,
		}
	}
	if out.StartRange == nil && out.EndRange == nil {
		return nil
	}
	return out
}
