//go:build e2e

// snippet_test.go drives the personal snippet builder against the stub, whole
// and through its halves: what it asks GitLab for, the deletion its test's
// end runs, and the endings a deletion has.

package fixture

import (
	"net/http"
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
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

// TestNewSnippet_Detached_IsDeletedWhenItsTestEnds checks the personal
// snippet builder whole: titled under the run and described by the test that
// made it, handed back as GitLab answered it, and deleted when that test
// ends, since nothing else that is torn down takes a user's snippet along.
func TestNewSnippet_Detached_IsDeletedWhenItsTestEnds(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/snippets", stubCreated(map[string]any{"id": 10, "title": "as-answered"}))
	stub.answers(http.MethodDelete, "/api/v4/snippets/10", stubNoContent())
	var got Snippet
	var e *harness.Env

	t.Run("made", func(t *testing.T) {
		e = harness.NewDetached(t, client)
		got = NewSnippet(e)
		if sent := sentPaths(stub); slices.Contains(sent, "DELETE /api/v4/snippets/10") {
			t.Errorf("the snippet was deleted while its test was still running: %v", sent)
		}
	})

	if want := (Snippet{ID: 10, Title: "as-answered"}); got != want {
		t.Errorf("NewSnippet() = %+v, want %+v", got, want)
	}
	sent := requestTo(t, stub, http.MethodPost, "/api/v4/snippets").Body
	if title, _ := sent["title"].(string); !isScopedName(title, "snippet", e) || sent["description"] != "e2e: "+t.Name()+"/made" {
		t.Errorf("NewSnippet() sent %v, want a title under the run described by the test that made it", sent)
	}
	if sent := sentPaths(stub); !slices.Equal(sent, []string{"POST /api/v4/snippets", "DELETE /api/v4/snippets/10"}) {
		t.Errorf("the builder and its test's end asked for %v, want the snippet made and then deleted", sent)
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
