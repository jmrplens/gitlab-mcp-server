package commitdiscussions

import (
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// NoteOutput is an alias of [toolutil.DiscussionThreadNoteOutput], the rich
// note shape used within discussion threads, shared with mrdiscussions. Field
// layout and JSON tags (including UpdatedAt with omitempty and
// Resolved/Resolvable without omitempty) are defined in toolutil.
type NoteOutput = toolutil.DiscussionThreadNoteOutput

// Output is an alias of [toolutil.DiscussionThreadOutput], the discussion
// thread shape with full note payloads, shared with mrdiscussions.
type Output = toolutil.DiscussionThreadOutput

// NoteToOutput converts a GitLab API [gl.Note], and what the captured
// response adds to it, to a [NoteOutput]: the shared conversion in
// [toolutil.DiscussionThreadNoteOutputFromGitLab], kept under the package's
// name for its callers and tests.
func NoteToOutput(n *gl.Note, extra toolutil.NoteExtra) NoteOutput {
	return toolutil.DiscussionThreadNoteOutputFromGitLab(n, extra)
}

// ToOutput converts a GitLab API [gl.Discussion], and what the captured
// response adds to it, to an [Output], including all notes within the
// thread.
func ToOutput(d *gl.Discussion, extra toolutil.DiscussionExtra) Output {
	return toolutil.DiscussionThreadOutputFromGitLab(d, extra)
}
