//go:build e2e

// tag_test.go drives the tag builder's pure halves against the stub: that the
// tag is annotated rather than lightweight, the two endings a deletion has,
// and what the read-back answers.

package fixture

import (
	"net/http"
	"testing"
)

// TestCreateTag_Created_IsAnnotated checks that the builder sends a message, so
// that a case asking about the tag's message is asking about something.
func TestCreateTag_Created_IsAnnotated(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/projects/2/repository/tags", stubCreated(map[string]any{
		"name": "eval-tag", "message": tagMessage,
	}))

	got, err := createTag(t.Context(), client, 2, "eval-tag", "main")
	if err != nil {
		t.Fatalf("createTag() error = %v, want nil", err)
	}
	want := Tag{Name: "eval-tag", Ref: "main"}
	if got != want {
		t.Errorf("createTag() = %+v, want %+v", got, want)
	}

	requests := stub.recordedRequests()
	if len(requests) != 1 {
		t.Fatalf("createTag() sent %d requests, want 1", len(requests))
	}
	if requests[0].Body["message"] != tagMessage || requests[0].Body["ref"] != "main" {
		t.Errorf("createTag() sent %v, want the message and the ref", requests[0].Body)
	}
}

// TestDeleteTag_Endings_ToleratesOneACaseDeleted checks that a tag a case
// already deleted is not a cleanup failure and any other refusal is.
func TestDeleteTag_Endings_ToleratesOneACaseDeleted(t *testing.T) {
	cases := []struct {
		name    string
		answer  scriptedAnswer
		wantErr bool
	}{
		{name: "deleted", answer: stubNoContent()},
		{name: "already gone", answer: stubRefusal(http.StatusNotFound, "404 Tag Not Found")},
		{name: "refused", answer: stubRefusal(http.StatusForbidden, "403 Forbidden"), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodDelete, "/api/v4/projects/2/repository/tags/eval-tag", tc.answer)

			err := deleteTag(t.Context(), client, 2, "eval-tag")
			if (err != nil) != tc.wantErr {
				t.Errorf("deleteTag() error = %v, wantErr = %t", err, tc.wantErr)
			}
		})
	}
}

// TestTagExists_Answers covers the three answers the read-back distinguishes.
func TestTagExists_Answers(t *testing.T) {
	cases := []struct {
		name    string
		answer  scriptedAnswer
		want    bool
		wantErr bool
	}{
		{name: "there", answer: stubOK(map[string]any{"name": "eval-tag"}), want: true},
		{name: "gone", answer: stubRefusal(http.StatusNotFound, "404 Tag Not Found")},
		{name: "refused", answer: stubRefusal(http.StatusForbidden, "403 Forbidden"), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodGet, "/api/v4/projects/2/repository/tags/eval-tag", tc.answer)

			got, err := TagExists(t.Context(), client, 2, "eval-tag")
			if (err != nil) != tc.wantErr {
				t.Fatalf("TagExists() error = %v, wantErr = %t", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("TagExists() = %t, want %t", got, tc.want)
			}
		})
	}
}
