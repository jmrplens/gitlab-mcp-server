//go:build e2e

// snippetdiscussions_test.go covers the threaded discussions of a project
// snippet through the server: open a thread, find it in the listing, read
// it, add a note, edit the opening note and delete it, once per surface on
// one shared snippet the fixture library built.

package common

import (
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/snippetdiscussions"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestSnippetDiscussions_Lifecycle_CreateListGetAddUpdateDeleteNote opens
// a thread on the fixture snippet on every surface, finds it in the
// listing, reads it back, replies to it, edits the opening note and deletes
// that note, checking the thread no longer carries it.
//
// Replaces: TestMeta_SnippetDiscussions
func TestSnippetDiscussions_Lifecycle_CreateListGetAddUpdateDeleteNote(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) projectSnippetFixture {
		return newProjectSnippetFixture(e, "snippetdisc")
	}, func(e *harness.Env, surface harness.Surface, f projectSnippetFixture) {
		s := e.On(surface)
		params := f.params()
		body := "thread from the " + string(surface) + " surface"

		created := harness.Do[snippetdiscussions.Output](s, actionSnippetDiscussionCreate, withParams(params, map[string]any{"body": body}))
		if created.ID == "" || len(created.Notes) == 0 || created.Notes[0] == nil || created.Notes[0].Body != body {
			e.T.Fatalf("discussion_create answered %+v, want a thread with an ID opened by the note %q", created, body)
		}
		noteID := created.Notes[0].ID
		threadParams := withParams(params, map[string]any{"discussion_id": created.ID})

		listed := harness.Do[snippetdiscussions.ListOutput](s, actionSnippetDiscussionList, params)
		if !slices.Contains(discussionIDs(listed.Discussions), created.ID) {
			e.T.Errorf("the snippet's discussions do not hold %s: %v", created.ID, discussionIDs(listed.Discussions))
		}

		got := harness.Do[snippetdiscussions.Output](s, actionSnippetDiscussionGet, threadParams)
		if got.ID != created.ID || len(got.Notes) == 0 {
			e.T.Errorf("discussion_get answered %+v, want thread %s with its note", got, created.ID)
		}

		reply := harness.Do[snippetdiscussions.NoteOutput](s, actionSnippetDiscussionAddNote, withParams(threadParams, map[string]any{"body": "a reply"}))
		if reply.ID == 0 || reply.Body != "a reply" {
			e.T.Errorf("discussion_add_note answered %+v, want the reply with an ID", reply)
		}

		updated := harness.Do[snippetdiscussions.NoteOutput](s, actionSnippetDiscussionUpdateNote, withParams(threadParams, map[string]any{
			"note_id": noteID, "body": body + " (edited)",
		}))
		if updated.ID != noteID || updated.Body != body+" (edited)" {
			e.T.Errorf("discussion_update_note answered %+v, want note %d with the edited body", updated, noteID)
		}

		harness.DoVoid(s, actionSnippetDiscussionDeleteNote, withParams(threadParams, map[string]any{"note_id": noteID}))
		after := harness.Do[snippetdiscussions.ListOutput](s, actionSnippetDiscussionList, params)
		if threadHoldsNote(after.Discussions, created.ID, noteID) {
			e.T.Errorf("note %d is still in thread %s after its delete", noteID, created.ID)
		}
	})
}
