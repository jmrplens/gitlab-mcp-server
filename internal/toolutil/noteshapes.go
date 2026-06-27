package toolutil

import (
	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// NoteUserOutput mirrors gl.NoteAuthor / gl.NoteResolvedBy (identical
// shapes): the user who authored or resolved a note.
type NoteUserOutput struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Email     string `json:"email,omitempty"`
	Name      string `json:"name"`
	State     string `json:"state,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
}

// NewNoteUserOutputFromAuthor converts a gl.NoteAuthor value into the
// additive author object. The author is always present on a note, so this
// returns a pointer to a populated value (never nil) to keep the canonical
// `author` key stable.
func NewNoteUserOutputFromAuthor(a gl.NoteAuthor) *NoteUserOutput {
	return &NoteUserOutput{
		ID: a.ID, Username: a.Username, Email: a.Email, Name: a.Name,
		State: a.State, AvatarURL: a.AvatarURL, WebURL: a.WebURL,
	}
}

// NewNoteUserOutputFromResolvedBy converts a gl.NoteResolvedBy value into
// the resolved-by object, returning nil when no user has resolved the note
// (zero ID + empty username).
func NewNoteUserOutputFromResolvedBy(r gl.NoteResolvedBy) *NoteUserOutput {
	if r.ID == 0 && r.Username == "" {
		return nil
	}
	return &NoteUserOutput{
		ID: r.ID, Username: r.Username, Email: r.Email, Name: r.Name,
		State: r.State, AvatarURL: r.AvatarURL, WebURL: r.WebURL,
	}
}

// LinePositionOutput mirrors gl.LinePosition: one endpoint (start or end)
// of a multi-line diff note position.
type LinePositionOutput struct {
	LineCode string `json:"line_code,omitempty"`
	Type     string `json:"type,omitempty"`
	OldLine  int64  `json:"old_line,omitempty"`
	NewLine  int64  `json:"new_line,omitempty"`
}

// LineRangeOutput mirrors gl.LineRange (the start/end of a multi-line diff
// note position). The full *LinePositionOutput objects are surfaced on the
// canonical `start` / `end` keys (mirroring gl.LineRange.StartRange /
// gl.LineRange.EndRange, whose JSON tags are `start` / `end`).
type LineRangeOutput struct {
	Start *LinePositionOutput `json:"start,omitempty"`
	End   *LinePositionOutput `json:"end,omitempty"`
}

// NewLinePositionOutput converts a *gl.LinePosition into the canonical-key
// line-position object, returning nil when the SDK value is nil.
func NewLinePositionOutput(p *gl.LinePosition) *LinePositionOutput {
	if p == nil {
		return nil
	}
	return &LinePositionOutput{
		LineCode: p.LineCode, Type: p.Type,
		OldLine: p.OldLine, NewLine: p.NewLine,
	}
}

// NotePositionOutput mirrors gl.NotePosition: the diff position of a note
// that is attached to a specific line of a file.
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

// NewNotePositionOutput converts a *gl.NotePosition into the position
// object, returning nil when the note has no diff position.
func NewNotePositionOutput(p *gl.NotePosition) *NotePositionOutput {
	if p == nil {
		return nil
	}
	return &NotePositionOutput{
		BaseSHA: p.BaseSHA, StartSHA: p.StartSHA, HeadSHA: p.HeadSHA,
		PositionType: p.PositionType, NewPath: p.NewPath, NewLine: p.NewLine,
		OldPath: p.OldPath, OldLine: p.OldLine,
		LineRange: NewLineRangeOutput(p.LineRange),
	}
}

// NewLineRangeOutput converts a *gl.LineRange into the line-range object,
// returning nil when absent (or when both endpoints are nil).
func NewLineRangeOutput(lr *gl.LineRange) *LineRangeOutput {
	if lr == nil {
		return nil
	}
	out := &LineRangeOutput{
		Start: NewLinePositionOutput(lr.StartRange),
		End:   NewLinePositionOutput(lr.EndRange),
	}
	if out.Start == nil && out.End == nil {
		return nil
	}
	return out
}
