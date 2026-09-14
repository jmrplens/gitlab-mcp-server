//go:build e2e

// b7_issue_noteemoji_test.go covers the award emoji of a note on an issue:
// the four routes that are their own, beside the four awardemoji_test.go
// already drives against the issue itself and against a snippet's note. The
// old CE suite asserted all four and the rebuilt suite reached none of them.
//
// The note is written through the server rather than built as a fixture,
// because the note id is the parameter the four routes hang on and the
// create is what hands it over.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issuenotes"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestAwardEmoji_IssueNote_AwardListGetDelete writes a note on an issue of
// its own on every surface and walks the four emoji routes on that note:
// award, find it in the listing, read it by its id, take it back and see it
// gone.
//
// A user awards a given emoji once per object, so each surface reacts to a
// note of its own rather than to a shared one.
func TestAwardEmoji_IssueNote_AwardListGetDelete(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("issuenoteemoji"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		issue := fixture.NewIssue(e, project, "note emoji fixture for the "+string(surface)+" surface")
		params := map[string]any{"project_id": project.IDParam(), "issue_iid": issue.IID}

		body := "the note the " + string(surface) + " surface reacts to"
		note := harness.Do[issuenotes.Output](s, actionIssueNoteCreate, withParams(params, map[string]any{"body": body}))
		if note.ID == 0 || note.Body != body {
			e.T.Fatalf("issue note_create answered %+v, want the note %q with an ID", note, body)
		}

		noteParams := withParams(params, map[string]any{"note_id": note.ID})
		awardLifecycle(e, s, noteParams, emojiThumbsUp,
			actionIssueNoteEmojiCreate, actionIssueNoteEmojiList, actionIssueNoteEmojiGet, actionIssueNoteEmojiDelete)
	})
}
