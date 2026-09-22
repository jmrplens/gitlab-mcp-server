//go:build e2e

// snippet_test.go drives the personal snippet builder's pure halves against
// the stub: what it asks GitLab for, and the two endings a deletion has.

package fixture

import (
	"net/http"
	"testing"
)

// TestCreatePersonalSnippet_Answers_SendsTheOneFilePrivately checks the
// creator asks for a private snippet carrying the one fixture file under the
// title and description it was given, reads back what GitLab made, and hands a
// refusal back as it came.
func TestCreatePersonalSnippet_Answers_SendsTheOneFilePrivately(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/snippets",
		stubCreated(map[string]any{"id": 10, "title": "snippet-run"}),
		stubRefusal(http.StatusForbidden, "403 Forbidden"))

	got, err := createPersonalSnippet(t.Context(), client, "snippet-run", "the World's")
	if err != nil {
		t.Fatalf("createPersonalSnippet() error = %v, want nil", err)
	}
	if want := (Snippet{ID: 10, Title: "snippet-run"}); got != want {
		t.Errorf("createPersonalSnippet() = %+v, want %+v", got, want)
	}
	sent := stub.recordedRequests()[0].Body
	wantSent := map[string]any{
		"title": "snippet-run", "description": "the World's", "visibility": "private",
		"file_name": snippetFileName, "content": snippetContent,
	}
	for field, want := range wantSent {
		t.Run(field, func(t *testing.T) {
			if sent[field] != want {
				t.Errorf("createPersonalSnippet() sent %s = %v, want %v", field, sent[field], want)
			}
		})
	}

	got, err = createPersonalSnippet(t.Context(), client, "snippet-run", "the World's")
	if !IsStatus(err, http.StatusForbidden) || got != (Snippet{}) {
		t.Errorf("createPersonalSnippet() on a refusal = %+v, %v; want nothing and GitLab's 403", got, err)
	}
}

// TestDeletePersonalSnippet_Endings_ToleratesOneACaseDeleted checks that a
// snippet a case already deleted is not a cleanup failure and any other
// refusal is, named by the snippet.
func TestDeletePersonalSnippet_Endings_ToleratesOneACaseDeleted(t *testing.T) {
	cases := []struct {
		name    string
		answer  scriptedAnswer
		wantErr bool
	}{
		{name: "deleted", answer: stubNoContent()},
		{name: "already gone", answer: stubRefusal(http.StatusNotFound, "404 Snippet Not Found")},
		{name: "refused", answer: stubRefusal(http.StatusForbidden, "403 Forbidden"), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodDelete, "/api/v4/snippets/10", tc.answer)

			err := deletePersonalSnippet(t.Context(), client, 10)
			if (err != nil) != tc.wantErr {
				t.Errorf("deletePersonalSnippet() error = %v, wantErr = %t", err, tc.wantErr)
			}
			if tc.wantErr && !IsStatus(err, http.StatusForbidden) {
				t.Errorf("deletePersonalSnippet() error = %v, want GitLab's 403 carried", err)
			}
		})
	}
}
