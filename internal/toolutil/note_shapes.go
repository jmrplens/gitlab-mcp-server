package toolutil

import (
	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// NoteUserOutput mirrors GitLab's UserBasic entity, which is what a note's
// author and resolver are rendered with: the identity fields, the account
// state and lock, and the two URLs. It carries no email, since UserBasic
// exposes `public_email` and never `email`; client-go's NoteAuthor carries
// an `email` GitLab does not send, and `public_email` and `locked` it does
// not model, which is why the two shared values come from the captured
// response (see [NoteUserExtra]).
type NoteUserOutput = UserBasicOutput

// NoteUserExtra is what a note's author or resolver carries that client-go's
// NoteAuthor and NoteResolvedBy do not model, read from the captured
// response.
type NoteUserExtra struct {
	PublicEmail string `json:"public_email"`
	Locked      bool   `json:"locked"`
}

// NewNoteUserOutputFromAuthor converts a gl.NoteAuthor value into the
// additive author object. The author is always present on a note, so this
// returns a pointer to a populated value (never nil) to keep the canonical
// `author` key stable.
func NewNoteUserOutputFromAuthor(a gl.NoteAuthor, extra NoteUserExtra) *NoteUserOutput {
	return &NoteUserOutput{
		ID: a.ID, Username: a.Username, PublicEmail: extra.PublicEmail, Name: a.Name,
		State: a.State, Locked: extra.Locked, AvatarURL: a.AvatarURL, WebURL: a.WebURL,
	}
}

// NewNoteUserOutputFromResolvedBy converts a gl.NoteResolvedBy value into
// the resolved-by object, returning nil when no user has resolved the note
// (zero ID + empty username).
func NewNoteUserOutputFromResolvedBy(r gl.NoteResolvedBy, extra NoteUserExtra) *NoteUserOutput {
	if r.ID == 0 && r.Username == "" {
		return nil
	}
	return &NoteUserOutput{
		ID: r.ID, Username: r.Username, PublicEmail: extra.PublicEmail, Name: r.Name,
		State: r.State, Locked: extra.Locked, AvatarURL: r.AvatarURL, WebURL: r.WebURL,
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

// SuggestionOutput mirrors GitLab's Suggestion entity, the code suggestion a
// merge request diff note carries.
type SuggestionOutput struct {
	ID          int64  `json:"id"`
	FromLine    int64  `json:"from_line"`
	ToLine      int64  `json:"to_line"`
	Appliable   bool   `json:"appliable"`
	Applied     bool   `json:"applied"`
	FromContent string `json:"from_content"`
	ToContent   string `json:"to_content"`
}

// NoteExtra is what GitLab's Note entity sends that client-go's Note does not
// model, read from the captured response beside the SDK's own decoding (see
// [CapturedNote]). `confidential` is here too: client-go carries it as a
// deprecated field some converters read and others mirror from `internal`,
// and reading GitLab's own value ends that difference.
type NoteExtra struct {
	Confidential    bool               `json:"confidential"`
	Imported        bool               `json:"imported"`
	ImportedFrom    string             `json:"imported_from"`
	CommandsChanges map[string]any     `json:"commands_changes"`
	Suggestions     []SuggestionOutput `json:"suggestions"`
	Author          NoteUserExtra      `json:"author"`
	ResolvedBy      NoteUserExtra      `json:"resolved_by"`
}

// NoteOutput mirrors every field of GitLab's Note entity as the REST Notes
// API sends it (notes on merge requests, issues, snippets, etc.): the fields
// client-go's Note models, and the ones only the captured response carries.
// It is the canonical shared output shape for standalone note tools
// (mrnotes, issuenotes). Field order and JSON tags are locked; any change is
// a breaking wire-format change.
//
// `attachment`, `title`, `file_name` and `expires_at`, which client-go's
// Note still carries, are not here: GitLab's entity does not expose them,
// and the R-PATH type grain reported them as fields no operation sends.
type NoteOutput struct {
	HintableOutput
	ID              int64               `json:"id"`
	Body            string              `json:"body"`
	Author          *NoteUserOutput     `json:"author,omitempty"`
	CreatedAt       string              `json:"created_at"`
	UpdatedAt       string              `json:"updated_at"`
	System          bool                `json:"system"`
	Internal        bool                `json:"internal"`
	Resolvable      bool                `json:"resolvable,omitempty"`
	Resolved        bool                `json:"resolved,omitempty"`
	ResolvedAt      string              `json:"resolved_at,omitempty"`
	ResolvedBy      *NoteUserOutput     `json:"resolved_by,omitempty"`
	NoteableType    string              `json:"noteable_type,omitempty"`
	NoteableID      int64               `json:"noteable_id,omitempty"`
	NoteableIID     int64               `json:"noteable_iid,omitempty"`
	CommitID        string              `json:"commit_id,omitempty"`
	Type            string              `json:"type,omitempty"`
	Position        *NotePositionOutput `json:"position,omitempty"`
	ProjectID       int64               `json:"project_id,omitempty"`
	Confidential    bool                `json:"confidential"`
	Imported        bool                `json:"imported"`
	ImportedFrom    string              `json:"imported_from,omitempty"`
	CommandsChanges map[string]any      `json:"commands_changes,omitempty"`
	Suggestions     []SuggestionOutput  `json:"suggestions,omitempty"`
}

// NoteOutputFromGitLab converts a [gl.Note] and what the captured response
// adds to it into the canonical [NoteOutput] shape used by standalone note
// tools (mrnotes, issuenotes). Timestamps are formatted as RFC 3339 strings.
func NoteOutputFromGitLab(n *gl.Note, extra NoteExtra) NoteOutput {
	out := NoteOutput{
		ID:              n.ID,
		Body:            n.Body,
		Author:          NewNoteUserOutputFromAuthor(n.Author, extra.Author),
		System:          n.System,
		Internal:        n.Internal,
		Resolvable:      n.Resolvable,
		Resolved:        n.Resolved,
		ResolvedBy:      NewNoteUserOutputFromResolvedBy(n.ResolvedBy, extra.ResolvedBy),
		NoteableType:    n.NoteableType,
		NoteableID:      n.NoteableID,
		NoteableIID:     n.NoteableIID,
		CommitID:        n.CommitID,
		Type:            string(n.Type),
		Position:        NewNotePositionOutput(n.Position),
		ProjectID:       n.ProjectID,
		Confidential:    extra.Confidential,
		Imported:        extra.Imported,
		ImportedFrom:    extra.ImportedFrom,
		CommandsChanges: extra.CommandsChanges,
		Suggestions:     extra.Suggestions,
	}
	out.CreatedAt = FormatTimePtr(n.CreatedAt)
	out.UpdatedAt = FormatTimePtr(n.UpdatedAt)
	out.ResolvedAt = FormatTimePtr(n.ResolvedAt)
	return out
}

// DiscussionThreadNoteOutput mirrors every field of GitLab's Note entity as
// it appears within a REST discussion thread (mrdiscussions,
// commitdiscussions, issuediscussions, snippetdiscussions). The shape differs
// from [NoteOutput] in omitempty semantics: UpdatedAt carries omitempty
// (absent on notes created without an update), while Resolved and Resolvable
// are always emitted (never omitted) because thread resolution state is always
// meaningful. Field order and JSON tags are locked.
type DiscussionThreadNoteOutput struct {
	HintableOutput
	ID              int64               `json:"id"`
	Body            string              `json:"body"`
	Author          *NoteUserOutput     `json:"author,omitempty"`
	CreatedAt       string              `json:"created_at"`
	UpdatedAt       string              `json:"updated_at,omitempty"`
	Resolved        bool                `json:"resolved"`
	Resolvable      bool                `json:"resolvable"`
	ResolvedAt      string              `json:"resolved_at,omitempty"`
	ResolvedBy      *NoteUserOutput     `json:"resolved_by,omitempty"`
	System          bool                `json:"system"`
	Internal        bool                `json:"internal"`
	Confidential    bool                `json:"confidential"`
	Type            string              `json:"type,omitempty"`
	NoteableType    string              `json:"noteable_type,omitempty"`
	NoteableID      int64               `json:"noteable_id,omitempty"`
	NoteableIID     int64               `json:"noteable_iid,omitempty"`
	CommitID        string              `json:"commit_id,omitempty"`
	Position        *NotePositionOutput `json:"position,omitempty"`
	ProjectID       int64               `json:"project_id,omitempty"`
	Imported        bool                `json:"imported"`
	ImportedFrom    string              `json:"imported_from,omitempty"`
	CommandsChanges map[string]any      `json:"commands_changes,omitempty"`
	Suggestions     []SuggestionOutput  `json:"suggestions,omitempty"`
}

// DiscussionThreadNoteOutputFromGitLab converts a [gl.Note] within a
// discussion thread, and what the captured response adds to it, into the
// thread note shape. A nil note converts to the zero value, since the SDK
// returns []*gl.Note and an element may be nil.
func DiscussionThreadNoteOutputFromGitLab(n *gl.Note, extra NoteExtra) DiscussionThreadNoteOutput {
	if n == nil {
		return DiscussionThreadNoteOutput{}
	}
	return DiscussionThreadNoteOutput{
		ID:              n.ID,
		Body:            n.Body,
		Author:          NewNoteUserOutputFromAuthor(n.Author, extra.Author),
		CreatedAt:       FormatTimePtr(n.CreatedAt),
		UpdatedAt:       FormatTimePtr(n.UpdatedAt),
		Resolved:        n.Resolved,
		Resolvable:      n.Resolvable,
		ResolvedAt:      FormatTimePtr(n.ResolvedAt),
		ResolvedBy:      NewNoteUserOutputFromResolvedBy(n.ResolvedBy, extra.ResolvedBy),
		System:          n.System,
		Internal:        n.Internal,
		Confidential:    extra.Confidential,
		Type:            string(n.Type),
		NoteableType:    n.NoteableType,
		NoteableID:      n.NoteableID,
		NoteableIID:     n.NoteableIID,
		CommitID:        n.CommitID,
		Position:        NewNotePositionOutput(n.Position),
		ProjectID:       n.ProjectID,
		Imported:        extra.Imported,
		ImportedFrom:    extra.ImportedFrom,
		CommandsChanges: extra.CommandsChanges,
		Suggestions:     extra.Suggestions,
	}
}

// MarkdownNote returns the shared Markdown view model for a thread note,
// naming the author by username.
func (n DiscussionThreadNoteOutput) MarkdownNote() DiscussionNoteMarkdown {
	author := ""
	if n.Author != nil {
		author = n.Author.Username
	}
	return NewDiscussionNoteMarkdown(n.ID, n.Body, author, n.CreatedAt)
}

// DiscussionExtra is what GitLab's Discussion entity sends that client-go's
// Discussion does not model, the thread's own resolution state, read from the
// captured response beside the notes' own extras (see [CapturedDiscussion]).
type DiscussionExtra struct {
	Resolvable bool        `json:"resolvable"`
	Resolved   bool        `json:"resolved"`
	Notes      []NoteExtra `json:"notes"`
}

// DiscussionThreadOutput mirrors GitLab's Discussion entity with its full note
// payloads as returned by the REST Discussions API. Notes are pointer elements
// because the SDK returns []*gl.Note and individual notes may be nil in edge
// cases. `resolvable` and `resolved` are the thread's own, which client-go
// does not model; `resolved` is sent only for a resolvable thread.
type DiscussionThreadOutput struct {
	HintableOutput
	ID             string                        `json:"id"`
	IndividualNote bool                          `json:"individual_note"`
	Resolvable     bool                          `json:"resolvable"`
	Resolved       bool                          `json:"resolved,omitempty"`
	Notes          []*DiscussionThreadNoteOutput `json:"notes"`
}

// DiscussionThreadOutputFromGitLab converts a [gl.Discussion] and what the
// captured response adds to it into the thread shape, every note included.
// A note the capture holds no extra for, which cannot happen for an answer
// the SDK decoded, converts with none; a nil discussion converts to the zero
// value.
func DiscussionThreadOutputFromGitLab(d *gl.Discussion, extra DiscussionExtra) DiscussionThreadOutput {
	if d == nil {
		return DiscussionThreadOutput{}
	}
	notes := make([]*DiscussionThreadNoteOutput, len(d.Notes))
	for i, n := range d.Notes {
		var noteExtra NoteExtra
		if i < len(extra.Notes) {
			noteExtra = extra.Notes[i]
		}
		note := DiscussionThreadNoteOutputFromGitLab(n, noteExtra)
		notes[i] = &note
	}
	return DiscussionThreadOutput{
		ID:             d.ID,
		IndividualNote: d.IndividualNote,
		Resolvable:     extra.Resolvable,
		Resolved:       extra.Resolved,
		Notes:          notes,
	}
}

// DiscussionThreadOutputsFromGitLab converts a list of discussions with the
// extras the captured list answer holds for each, by position.
func DiscussionThreadOutputsFromGitLab(discussions []*gl.Discussion, extras []DiscussionExtra) []DiscussionThreadOutput {
	out := make([]DiscussionThreadOutput, 0, len(discussions))
	for i, d := range discussions {
		var extra DiscussionExtra
		if i < len(extras) {
			extra = extras[i]
		}
		out = append(out, DiscussionThreadOutputFromGitLab(d, extra))
	}
	return out
}

// MarkdownDiscussion returns the shared Markdown view model for a thread.
func (d DiscussionThreadOutput) MarkdownDiscussion() DiscussionMarkdown {
	notes := make([]DiscussionNoteMarkdown, 0, len(d.Notes))
	for _, note := range d.Notes {
		if note != nil {
			notes = append(notes, note.MarkdownNote())
		}
	}
	return NewDiscussionMarkdown(d.ID, notes)
}

// DiscussionThreadOutputMarkdowns maps thread outputs to Markdown view
// models.
func DiscussionThreadOutputMarkdowns(discussions []DiscussionThreadOutput) []DiscussionMarkdown {
	return DiscussionMarkdowns(discussions, DiscussionThreadOutput.MarkdownDiscussion)
}
