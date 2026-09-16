//go:build e2e

// draft_note_test.go drives the draft note builder's pure halves against the
// stub: what a create sends, the two endings a deletion has, and what the
// read-back answers for a note a case published.

package fixture

import (
	"net/http"
	"testing"
)

// TestCreateDraftNote_Created_SendsTheBodyAndReadsItBack checks that the
// builder writes the note and returns what a case addresses it by.
func TestCreateDraftNote_Created_SendsTheBodyAndReadsItBack(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/projects/5/merge_requests/3/draft_notes", stubCreated(map[string]any{
		"id": 17, "note": draftNoteBody,
	}))

	got, err := createDraftNote(t.Context(), client, 5, 3)
	if err != nil {
		t.Fatalf("createDraftNote() error = %v, want nil", err)
	}
	want := DraftNote{ID: 17, Note: draftNoteBody}
	if got != want {
		t.Errorf("createDraftNote() = %+v, want %+v", got, want)
	}

	requests := stub.recordedRequests()
	if len(requests) != 1 || requests[0].Body["note"] != draftNoteBody {
		t.Errorf("createDraftNote() sent %v, want the note body", requests)
	}
}

// TestDeleteDraftNote_Endings_ToleratesOneACasePublished checks that a note a
// case published or deleted is not a cleanup failure and any other refusal is.
func TestDeleteDraftNote_Endings_ToleratesOneACasePublished(t *testing.T) {
	cases := []struct {
		name    string
		answer  scriptedAnswer
		wantErr bool
	}{
		{name: "deleted", answer: stubNoContent()},
		{name: "published", answer: stubRefusal(http.StatusNotFound, "404 Not found")},
		{name: "refused", answer: stubRefusal(http.StatusForbidden, "403 Forbidden"), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodDelete, "/api/v4/projects/5/merge_requests/3/draft_notes/17", tc.answer)

			err := deleteDraftNote(t.Context(), client, 5, 3, 17)
			if (err != nil) != tc.wantErr {
				t.Errorf("deleteDraftNote() error = %v, wantErr = %t", err, tc.wantErr)
			}
		})
	}
}

// TestDraftNoteExists_Answers covers the three answers the read-back
// distinguishes, which is what a case that publishes the note is judged on.
func TestDraftNoteExists_Answers(t *testing.T) {
	cases := []struct {
		name    string
		answer  scriptedAnswer
		want    bool
		wantErr bool
	}{
		{name: "unpublished", answer: stubOK(map[string]any{"id": 17, "note": draftNoteBody}), want: true},
		{name: "published", answer: stubRefusal(http.StatusNotFound, "404 Not found")},
		{name: "refused", answer: stubRefusal(http.StatusForbidden, "403 Forbidden"), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodGet, "/api/v4/projects/5/merge_requests/3/draft_notes/17", tc.answer)

			got, err := DraftNoteExists(t.Context(), client, 5, 3, 17)
			if (err != nil) != tc.wantErr {
				t.Fatalf("DraftNoteExists() error = %v, wantErr = %t", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("DraftNoteExists() = %t, want %t", got, tc.want)
			}
		})
	}
}
