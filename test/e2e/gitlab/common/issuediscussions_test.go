// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

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
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mrdiscussions"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/snippetdiscussions"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// discussionThread is the thread type of each discussion package these
// helpers read: each is a type of its own over the one shared thread shape,
// so one helper converts any of them to that shape and reads the three
// listings.
type discussionThread interface {
	issuediscussions.Output | mrdiscussions.Output | snippetdiscussions.Output
}

// discussionIDs lists the IDs of a discussion listing.
func discussionIDs[T discussionThread](discussions []T) []string {
	ids := make([]string, 0, len(discussions))
	for _, discussion := range discussions {
		ids = append(ids, toolutil.DiscussionThreadOutput(discussion).ID)
	}
	return ids
}

// threadHoldsNote reports whether the named thread of a listing still
// carries the note.
func threadHoldsNote[T discussionThread](discussions []T, threadID string, noteID int64) bool {
	for _, thread := range discussions {
		discussion := toolutil.DiscussionThreadOutput(thread)
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

		created, said := harness.DoAnswer[issuediscussions.Output](s, actionIssueDiscussionCreate, withParams(params, map[string]any{"body": body}))
		if created.ID == "" || len(created.Notes) == 0 || created.Notes[0].Body != body {
			e.T.Fatalf("discussion_create answered %+v, want a thread with an ID opened by the note %q", created, body)
		}
		// The thread card is the issue discussion's own, not the commit
		// discussion card the registry used to render every shared thread
		// shape with (issue 1291). Next steps are named in the dynamic
		// surface's call form, the canonical ID, which the other surfaces may
		// spell their own way.
		if containsAny(said, "commit_discussion") {
			e.T.Errorf("the issue thread card points at commit discussion actions: %s", said)
		}
		if surface == harness.SurfaceDynamic {
			assertMentions(e, "the issue thread card's next steps", said, "issue.discussion_add_note", "issue.discussion_update_note")
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
