// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package commitdiscussions

import (
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// NoteOutput is a note of a commit discussion: a type of its own over
// [toolutil.DiscussionThreadNoteOutput], the rich note shape the four REST
// discussion packages share, whose fields and JSON tags (UpdatedAt with
// omitempty, Resolved and Resolvable without) are defined there. It is a
// defined type rather than an alias because the Markdown registry is keyed by
// type: while the four packages aliased one, this package's card rendered
// every merge request, issue and snippet thread too, offering commit
// discussion actions as their next steps (issue 1291).
type NoteOutput toolutil.DiscussionThreadNoteOutput

// Output is a commit discussion thread: a type of its own over
// [toolutil.DiscussionThreadOutput], the thread shape with full note
// payloads, for the reason [NoteOutput] gives.
type Output toolutil.DiscussionThreadOutput

// NoteToOutput converts a GitLab API [gl.Note], and what the captured
// response adds to it, to a [NoteOutput]: the shared conversion in
// [toolutil.DiscussionThreadNoteOutputFromGitLab], kept under the package's
// name for its callers and tests.
func NoteToOutput(n *gl.Note, extra toolutil.NoteExtra) NoteOutput {
	return NoteOutput(toolutil.DiscussionThreadNoteOutputFromGitLab(n, extra))
}

// ToOutput converts a GitLab API [gl.Discussion], and what the captured
// response adds to it, to an [Output], including all notes within the
// thread.
func ToOutput(d *gl.Discussion, extra toolutil.DiscussionExtra) Output {
	return Output(toolutil.DiscussionThreadOutputFromGitLab(d, extra))
}
