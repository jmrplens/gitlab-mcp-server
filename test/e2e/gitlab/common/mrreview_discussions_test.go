//go:build e2e

// mrreview_discussions_test.go covers the review group's threaded
// discussions on a merge request: open a thread, find it in the listing,
// read it, reply, edit the opening note, resolve the thread and delete the
// opening note, once per surface on one shared request.

package common

import (
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mrdiscussions"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// threadResolved reports whether a thread reads as resolved: on the thread
// itself, or on every note GitLab marks resolvable, which is where the API
// records a resolution when the thread entity carries no flag of its own.
func threadResolved(thread mrdiscussions.Output) bool {
	if thread.Resolved {
		return true
	}
	resolvable := 0
	for _, note := range thread.Notes {
		if note == nil || !note.Resolvable {
			continue
		}
		resolvable++
		if !note.Resolved {
			return false
		}
	}
	return resolvable > 0
}

// TestMRReviewDiscussions_Lifecycle_CreateReplyUpdateResolveDelete opens
// a thread on the fixture request on every surface, finds it in the
// listing, reads it back, replies, edits the opening note, resolves the
// thread and reads that it is resolved, then deletes the opening note and
// checks the thread no longer carries it. The resolve is the answer the old
// suite threw away.
//
// Replaces: TestIndividual_MRDiscussions, TestMeta_MRDiscussions, TestMeta_MRReviewDiscussionNoteUpdate
func TestMRReviewDiscussions_Lifecycle_CreateReplyUpdateResolveDelete(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) mergeRequestFixture {
		return newMergeRequestFixture(e, "mrdisc")
	}, func(e *harness.Env, surface harness.Surface, f mergeRequestFixture) {
		s := e.On(surface)
		params := f.params()
		body := "thread from the " + string(surface) + " surface"

		created := harness.Do[mrdiscussions.Output](s, actionMRReviewDiscussionCreate, withParams(params, map[string]any{"body": body}))
		if created.ID == "" || len(created.Notes) == 0 || created.Notes[0] == nil || created.Notes[0].Body != body {
			e.T.Fatalf("discussion_create answered %+v, want a thread with an ID opened by the note %q", created, body)
		}
		noteID := created.Notes[0].ID
		threadParams := withParams(params, map[string]any{"discussion_id": created.ID})

		listed := harness.Do[mrdiscussions.ListOutput](s, actionMRReviewDiscussionList, params)
		if !slices.Contains(discussionIDs(listed.Discussions), created.ID) {
			e.T.Errorf("the request's discussions do not hold %s: %v", created.ID, discussionIDs(listed.Discussions))
		}

		got := harness.Do[mrdiscussions.Output](s, actionMRReviewDiscussionGet, threadParams)
		if got.ID != created.ID || len(got.Notes) == 0 {
			e.T.Errorf("discussion_get answered %+v, want thread %s with its note", got, created.ID)
		}

		reply := harness.Do[mrdiscussions.NoteOutput](s, actionMRReviewDiscussionReply, withParams(threadParams, map[string]any{"body": "a reply"}))
		if reply.ID == 0 || reply.Body != "a reply" {
			e.T.Errorf("discussion_reply answered %+v, want the reply with an ID", reply)
		}

		updated := harness.Do[mrdiscussions.NoteOutput](s, actionMRReviewDiscussionNoteUpdate, withParams(threadParams, map[string]any{
			"note_id": noteID, "body": body + " (edited)",
		}))
		if updated.ID != noteID || updated.Body != body+" (edited)" {
			e.T.Errorf("discussion_note_update answered %+v, want note %d with the edited body", updated, noteID)
		}

		resolved := harness.Do[mrdiscussions.Output](s, actionMRReviewDiscussionResolve, withParams(threadParams, map[string]any{"resolved": true}))
		if resolved.ID != created.ID || !threadResolved(resolved) {
			e.T.Errorf("discussion_resolve answered %+v, want thread %s resolved", resolved, created.ID)
		}

		harness.DoVoid(s, actionMRReviewDiscussionNoteDelete, withParams(threadParams, map[string]any{"note_id": noteID}))
		after := harness.Do[mrdiscussions.ListOutput](s, actionMRReviewDiscussionList, params)
		if threadHoldsNote(after.Discussions, created.ID, noteID) {
			e.T.Errorf("note %d is still in thread %s after its delete", noteID, created.ID)
		}
	})
}
