package epicnotes

import "github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"

// Canonical output sub-objects for epic notes. Epic notes are served through the
// Work Items GraphQL API (epics are migrated to work items), so the author is
// mirrored from the GraphQL User type rather than the REST gl.NoteAuthor struct.
// Per the locked 1:1 audit policy the full author object is surfaced on the
// canonical `author` key, and only fields the GraphQL query actually returns are
// populated (no invented output scalars). The shape is replicated here rather
// than imported from a sibling package to preserve the zero-import-cycle
// constraint (C-IMPORTS).

// NoteUserOutput mirrors the GraphQL User fields selected for a note author:
// the user who authored the note.
type NoteUserOutput struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

// noteAuthorOutput converts a gqlNoteAuthor value into the additive author
// object. The author is effectively always present on a note, so this returns a
// pointer to a populated value to keep the canonical `author` key stable; the
// numeric ID is parsed from the GraphQL global ID when present.
func noteAuthorOutput(a gqlNoteAuthor) *NoteUserOutput {
	out := &NoteUserOutput{
		Username:  a.Username,
		Name:      a.Name,
		WebURL:    a.WebURL,
		AvatarURL: a.AvatarURL,
	}
	if a.ID != "" {
		if _, id, err := toolutil.ParseGID(a.ID); err == nil {
			out.ID = id
		}
	}
	return out
}
