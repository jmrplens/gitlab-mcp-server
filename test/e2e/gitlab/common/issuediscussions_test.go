//go:build e2e

// issuediscussions_test.go covers the threaded discussions of an issue
// through the server: open a thread, find it in the listing, read it, add a
// note, edit the opening note and delete it, once per surface on one shared
// issue.

package common

import (
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issuediscussions"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// discussionIDs lists the IDs of a discussion listing. The issue, merge
// request and snippet discussion packages all alias the one shared thread
// shape, so one helper reads the three listings.
func discussionIDs(discussions []toolutil.DiscussionThreadOutput) []string {
	ids := make([]string, 0, len(discussions))
	for _, discussion := range discussions {
		ids = append(ids, discussion.ID)
	}
	return ids
}

// threadHoldsNote reports whether the named thread of a listing still
// carries the note.
func threadHoldsNote(discussions []toolutil.DiscussionThreadOutput, threadID string, noteID int64) bool {
	for _, discussion := range discussions {
		if discussion.ID != threadID {
			continue
		}
		for _, note := range discussion.Notes {
			if note != nil && note.ID == noteID {
				return true
			}
		}
	}
	return false
}

// TestIssueDiscussions_Lifecycle_CreateListGetAddUpdateDeleteNote opens a
// thread on the fixture issue on every surface, finds it in the listing,
// reads it back, replies to it, edits the opening note and deletes that
// note, reading each answer for the ID or the body it promises.
//
// Replaces: TestIndividual_IssueDiscussions, TestMeta_IssueDiscussions
func TestIssueDiscussions_Lifecycle_CreateListGetAddUpdateDeleteNote(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) issueFixture {
		return newIssueFixture(e, "issuedisc")
	}, func(e *harness.Env, surface harness.Surface, f issueFixture) {
		s := e.On(surface)
		params := f.params()
		body := "thread from the " + string(surface) + " surface"

		created := harness.Do[issuediscussions.Output](s, actionIssueDiscussionCreate, withParams(params, map[string]any{"body": body}))
		if created.ID == "" || len(created.Notes) == 0 || created.Notes[0].Body != body {
			e.T.Fatalf("discussion_create answered %+v, want a thread with an ID opened by the note %q", created, body)
		}
		noteID := created.Notes[0].ID
		threadParams := withParams(params, map[string]any{"discussion_id": created.ID})

		listed := harness.Do[issuediscussions.ListOutput](s, actionIssueDiscussionList, params)
		if !slices.Contains(discussionIDs(listed.Discussions), created.ID) {
			e.T.Errorf("the issue's discussions do not hold %s: %v", created.ID, discussionIDs(listed.Discussions))
		}

		got := harness.Do[issuediscussions.Output](s, actionIssueDiscussionGet, threadParams)
		if got.ID != created.ID || len(got.Notes) == 0 {
			e.T.Errorf("discussion_get answered %+v, want thread %s with its note", got, created.ID)
		}

		reply := harness.Do[issuediscussions.NoteOutput](s, actionIssueDiscussionAddNote, withParams(threadParams, map[string]any{"body": "a reply"}))
		if reply.ID == 0 || reply.Body != "a reply" {
			e.T.Errorf("discussion_add_note answered %+v, want the reply with an ID", reply)
		}

		updated := harness.Do[issuediscussions.NoteOutput](s, actionIssueDiscussionUpdateNote, withParams(threadParams, map[string]any{
			"note_id": noteID, "body": body + " (edited)",
		}))
		if updated.ID != noteID || updated.Body != body+" (edited)" {
			e.T.Errorf("discussion_update_note answered %+v, want note %d with the edited body", updated, noteID)
		}

		// The listing rather than the thread read afterwards: the reply keeps
		// the thread alive once its opening note is gone, and the listing
		// answers either way, where a read of a thread GitLab folded would
		// fail for a reason that is not the delete's.
		harness.DoVoid(s, actionIssueDiscussionDeleteNote, withParams(threadParams, map[string]any{"note_id": noteID}))
		after := harness.Do[issuediscussions.ListOutput](s, actionIssueDiscussionList, params)
		if threadHoldsNote(after.Discussions, created.ID, noteID) {
			e.T.Errorf("note %d is still in thread %s after its delete", noteID, created.ID)
		}
	})
}
