package commitdiscussions

import (
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical output sub-objects mirrored from client-go's note structs. Per the
// 1:1 audit policy (full nested objects) these surface every field of the SDK
// struct and are replicated here rather than imported from sibling packages to
// preserve the zero-import-cycle constraint (C-IMPORTS).

// NoteOutput represents a single note within a commit discussion. Per the 1:1
// audit policy it mirrors every field of gl.Note, surfacing the full author /
// resolved_by / position sub-objects on their canonical keys. Timestamps are
// formatted as RFC 3339 strings.
type NoteOutput struct {
	toolutil.HintableOutput
	ID           int64               `json:"id"`
	Body         string              `json:"body"`
	Author       *NoteUserOutput     `json:"author,omitempty"`
	Attachment   string              `json:"attachment,omitempty"`
	Title        string              `json:"title,omitempty"`
	FileName     string              `json:"file_name,omitempty"`
	CreatedAt    string              `json:"created_at"`
	UpdatedAt    string              `json:"updated_at,omitempty"`
	ExpiresAt    string              `json:"expires_at,omitempty"`
	Resolved     bool                `json:"resolved"`
	Resolvable   bool                `json:"resolvable"`
	ResolvedAt   string              `json:"resolved_at,omitempty"`
	ResolvedBy   *NoteUserOutput     `json:"resolved_by,omitempty"`
	System       bool                `json:"system"`
	Internal     bool                `json:"internal"`
	Confidential bool                `json:"confidential"`
	Type         string              `json:"type,omitempty"`
	NoteableType string              `json:"noteable_type,omitempty"`
	NoteableID   int64               `json:"noteable_id,omitempty"`
	NoteableIID  int64               `json:"noteable_iid,omitempty"`
	CommitID     string              `json:"commit_id,omitempty"`
	Position     *NotePositionOutput `json:"position,omitempty"`
	ProjectID    int64               `json:"project_id,omitempty"`
}

// Output represents a commit discussion thread. Its Notes field is a local
// []*NoteOutput mirror of the SDK's []*gl.Note (C-IMPORTS replication; the
// auditor flags the local-mirror type, which is the intended behavior).
type Output struct {
	toolutil.HintableOutput
	ID             string        `json:"id"`
	IndividualNote bool          `json:"individual_note"`
	Notes          []*NoteOutput `json:"notes"`
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

// LinePositionOutput mirrors gl.LinePosition: one endpoint (start or end) of a
// multi-line diff note position.
type LinePositionOutput struct {
	LineCode string `json:"line_code,omitempty"`
	Type     string `json:"type,omitempty"`
	OldLine  int64  `json:"old_line,omitempty"`
	NewLine  int64  `json:"new_line,omitempty"`
}

// LineRangeOutput mirrors gl.LineRange (the start/end of a multi-line diff note
// position). The full *LinePositionOutput objects are surfaced on the canonical
// `start` / `end` keys (mirroring gl.LineRange.StartRange / gl.LineRange.EndRange,
// whose JSON tags are `start` / `end`).
type LineRangeOutput struct {
	Start *LinePositionOutput `json:"start,omitempty"`
	End   *LinePositionOutput `json:"end,omitempty"`
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

// NoteToOutput converts a GitLab API [gl.Note] to a [NoteOutput]. Per the 1:1
// audit policy it surfaces the full author object on the canonical `author`
// key, additively surfaces the resolved_by / position sub-objects, and mirrors
// every other gl.Note field. Timestamps are formatted as RFC 3339 strings.
func NoteToOutput(n *gl.Note) NoteOutput {
	if n == nil {
		return NoteOutput{}
	}
	return NoteOutput{
		ID:           n.ID,
		Body:         n.Body,
		Author:       noteAuthorOutput(n.Author),
		Attachment:   n.Attachment,
		Title:        n.Title,
		FileName:     n.FileName,
		CreatedAt:    formatTimePtr(n.CreatedAt),
		UpdatedAt:    formatTimePtr(n.UpdatedAt),
		ExpiresAt:    formatTimePtr(n.ExpiresAt),
		Resolved:     n.Resolved,
		Resolvable:   n.Resolvable,
		ResolvedAt:   formatTimePtr(n.ResolvedAt),
		ResolvedBy:   noteResolvedByOutput(n.ResolvedBy),
		System:       n.System,
		Internal:     n.Internal,
		Confidential: n.Confidential, //nolint:staticcheck // 1:1 audit: mirror the SDK's deprecated Confidential field verbatim.
		Type:         string(n.Type),
		NoteableType: n.NoteableType,
		NoteableID:   n.NoteableID,
		NoteableIID:  n.NoteableIID,
		CommitID:     n.CommitID,
		Position:     notePositionOutput(n.Position),
		ProjectID:    n.ProjectID,
	}
}

// ToOutput converts a GitLab API [gl.Discussion] to an [Output], including all
// notes within the thread.
func ToOutput(d *gl.Discussion) Output {
	if d == nil {
		return Output{}
	}
	notes := make([]*NoteOutput, len(d.Notes))
	for i, n := range d.Notes {
		note := NoteToOutput(n)
		notes[i] = &note
	}
	return Output{
		ID:             d.ID,
		IndividualNote: d.IndividualNote,
		Notes:          notes,
	}
}

// formatTimePtr renders an optional timestamp as RFC 3339, or "" when nil.
func formatTimePtr(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
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

// linePositionOutput converts a *gl.LinePosition into the canonical-key
// line-position object, returning nil when the SDK value is nil.
func linePositionOutput(p *gl.LinePosition) *LinePositionOutput {
	if p == nil {
		return nil
	}
	return &LinePositionOutput{
		LineCode: p.LineCode, Type: p.Type,
		OldLine: p.OldLine, NewLine: p.NewLine,
	}
}

// lineRangeOutput converts a *gl.LineRange into the line-range object,
// returning nil when absent.
func lineRangeOutput(lr *gl.LineRange) *LineRangeOutput {
	if lr == nil {
		return nil
	}
	out := &LineRangeOutput{
		Start: linePositionOutput(lr.StartRange),
		End:   linePositionOutput(lr.EndRange),
	}
	if out.Start == nil && out.End == nil {
		return nil
	}
	return out
}

// notePositionOutput converts a *gl.NotePosition into the position object,
// returning nil when the note has no diff position.
func notePositionOutput(p *gl.NotePosition) *NotePositionOutput {
	if p == nil {
		return nil
	}
	return &NotePositionOutput{
		BaseSHA: p.BaseSHA, StartSHA: p.StartSHA, HeadSHA: p.HeadSHA,
		PositionType: p.PositionType, NewPath: p.NewPath, NewLine: p.NewLine,
		OldPath: p.OldPath, OldLine: p.OldLine,
		LineRange: lineRangeOutput(p.LineRange),
	}
}
