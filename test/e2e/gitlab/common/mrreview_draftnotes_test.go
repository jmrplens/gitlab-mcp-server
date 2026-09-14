//go:build e2e

// mrreview_draftnotes_test.go covers the review group's draft notes on a
// merge request: create, list, read and update one, delete a second, and
// the two ways a draft becomes a note, one at a time and all at once. Every
// surface works on one shared request: drafts belong to their author, and
// each surface publishes its own before the next starts.

package common

import (
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mrdraftnotes"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mrnotes"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// draftNoteIDs lists the IDs of a draft note listing.
func draftNoteIDs(drafts []mrdraftnotes.Output) []int64 {
	ids := make([]int64, 0, len(drafts))
	for _, draft := range drafts {
		ids = append(ids, draft.ID)
	}
	return ids
}

// TestMRReviewDraftNotes_Lifecycle_CreateUpdateDeletePublish writes a
// draft on the fixture request on every surface, finds it in the draft
// listing, reads it back, changes its text, writes a second draft and
// deletes it, then publishes the first one by itself and reads that it left
// the drafts and became a note. A third draft is then published through
// publish-all, and the same two reads confirm it.
//
// Replaces: TestIndividual_MRDraftNotes, TestMeta_MRDraftNotes, TestMeta_DraftNotePublish
func TestMRReviewDraftNotes_Lifecycle_CreateUpdateDeletePublish(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) mergeRequestFixture {
		return newMergeRequestFixture(e, "mrdrafts")
	}, func(e *harness.Env, surface harness.Surface, f mergeRequestFixture) {
		s := e.On(surface)
		params := f.params()
		text := "draft from the " + string(surface) + " surface"

		created := harness.Do[mrdraftnotes.Output](s, actionMRReviewDraftNoteCreate, withParams(params, map[string]any{"note": text}))
		if created.ID == 0 || created.Note != text {
			e.T.Fatalf("draft_note_create answered %+v, want the draft %q with an ID", created, text)
		}
		draftParams := withParams(params, map[string]any{"note_id": created.ID})

		listed := harness.Do[mrdraftnotes.ListOutput](s, actionMRReviewDraftNoteList, params)
		if !containsID(draftNoteIDs(listed.DraftNotes), created.ID) {
			e.T.Errorf("the request's drafts do not hold %d: %v", created.ID, draftNoteIDs(listed.DraftNotes))
		}

		got := harness.Do[mrdraftnotes.Output](s, actionMRReviewDraftNoteGet, draftParams)
		if got.ID != created.ID || got.Note != text {
			e.T.Errorf("draft_note_get answered %+v, want draft %d reading %q", got, created.ID, text)
		}

		updated := harness.Do[mrdraftnotes.Output](s, actionMRReviewDraftNoteUpdate, withParams(draftParams, map[string]any{"note": text + " (edited)"}))
		if updated.ID != created.ID || updated.Note != text+" (edited)" {
			e.T.Errorf("draft_note_update answered %+v, want draft %d with the edited text", updated, created.ID)
		}

		doomed := harness.Do[mrdraftnotes.Output](s, actionMRReviewDraftNoteCreate, withParams(params, map[string]any{"note": text + " (to delete)"}))
		if doomed.ID == 0 {
			e.T.Fatalf("draft_note_create answered %+v, want a second draft with an ID", doomed)
		}
		harness.DoVoid(s, actionMRReviewDraftNoteDelete, withParams(params, map[string]any{"note_id": doomed.ID}))
		afterDelete := harness.Do[mrdraftnotes.ListOutput](s, actionMRReviewDraftNoteList, params)
		if containsID(draftNoteIDs(afterDelete.DraftNotes), doomed.ID) {
			e.T.Errorf("draft %d is still listed after its delete", doomed.ID)
		}

		harness.DoVoid(s, actionMRReviewDraftNotePublish, draftParams)
		assertDraftPublished(e, s, params, created.ID, text+" (edited)")

		bulk := harness.Do[mrdraftnotes.Output](s, actionMRReviewDraftNoteCreate, withParams(params, map[string]any{"note": text + " (published in bulk)"}))
		if bulk.ID == 0 {
			e.T.Fatalf("draft_note_create answered %+v, want a third draft with an ID", bulk)
		}
		harness.DoVoid(s, actionMRReviewDraftNotePublishAll, params)
		assertDraftPublished(e, s, params, bulk.ID, text+" (published in bulk)")
	})
}

// assertDraftPublished checks the two halves of a publish: the draft left
// the draft listing, and a note with its text is on the request.
func assertDraftPublished(e *harness.Env, s *harness.Session, params map[string]any, draftID int64, text string) {
	e.T.Helper()

	drafts := harness.Do[mrdraftnotes.ListOutput](s, actionMRReviewDraftNoteList, params)
	if containsID(draftNoteIDs(drafts.DraftNotes), draftID) {
		e.T.Errorf("draft %d is still listed after its publish", draftID)
	}
	notes := harness.Do[mrnotes.ListOutput](s, actionMRReviewNoteList, params)
	if !slices.Contains(noteBodies(notes.Notes), text) {
		e.T.Errorf("no note reads %q after the publish of draft %d: %v", text, draftID, noteBodies(notes.Notes))
	}
}
