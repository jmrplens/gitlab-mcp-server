//go:build e2e

// mrreview_notes_test.go covers the review group's plain notes on a merge
// request: create, list, read, update and delete, once per surface on one
// shared request.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mrnotes"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestMRReviewNotes_Lifecycle_CreateListGetUpdateDelete writes a note on
// the fixture request on every surface, finds it in the listing, reads it
// back, changes its body, deletes it and checks the listing no longer holds
// it.
//
// Replaces: TestIndividual_MRNotes, TestMeta_MRNotes
func TestMRReviewNotes_Lifecycle_CreateListGetUpdateDelete(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) mergeRequestFixture {
		return newMergeRequestFixture(e, "mrnotes")
	}, func(e *harness.Env, surface harness.Surface, f mergeRequestFixture) {
		s := e.On(surface)
		params := f.params()
		body := "note from the " + string(surface) + " surface"

		created := harness.Do[mrnotes.Output](s, actionMRReviewNoteCreate, withParams(params, map[string]any{"body": body}))
		if created.ID == 0 || created.Body != body {
			e.T.Fatalf("note_create answered %+v, want the note %q with an ID", created, body)
		}
		noteParams := withParams(params, map[string]any{"note_id": created.ID})

		listed := harness.Do[mrnotes.ListOutput](s, actionMRReviewNoteList, params)
		if !containsID(noteIDs(listed.Notes), created.ID) {
			e.T.Errorf("the request's notes do not hold %d: %v", created.ID, noteIDs(listed.Notes))
		}

		got := harness.Do[mrnotes.Output](s, actionMRReviewNoteGet, noteParams)
		if got.ID != created.ID || got.Body != body {
			e.T.Errorf("note_get answered %+v, want note %d with body %q", got, created.ID, body)
		}

		updated := harness.Do[mrnotes.Output](s, actionMRReviewNoteUpdate, withParams(noteParams, map[string]any{"body": body + " (edited)"}))
		if updated.ID != created.ID || updated.Body != body+" (edited)" {
			e.T.Errorf("note_update answered %+v, want note %d with the edited body", updated, created.ID)
		}

		harness.DoVoid(s, actionMRReviewNoteDelete, noteParams)
		remaining := harness.Do[mrnotes.ListOutput](s, actionMRReviewNoteList, params)
		if containsID(noteIDs(remaining.Notes), created.ID) {
			e.T.Errorf("note %d is still listed after its delete", created.ID)
		}
	})
}
