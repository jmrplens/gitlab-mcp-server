//go:build e2e

// wiki_test.go drives the wiki builder's pure halves against the stub, and
// pins the reason the builder returns a slug rather than computing one:
// GitLab derives it from the title and answers with what it derived.

package fixture

import (
	"net/http"
	"testing"
)

// TestCreateWikiPage_Created_TakesTheSlugGitLabDerived checks that the slug a
// case addresses the page by comes out of the answer, even when GitLab spells
// it differently from the title that was sent.
func TestCreateWikiPage_Created_TakesTheSlugGitLabDerived(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/projects/8/wikis", stubCreated(map[string]any{
		"slug": "wiki-run-1-page", "title": "wiki run 1 page", "content": wikiContent, "format": "markdown",
	}))

	got, err := createWikiPage(t.Context(), client, 8, "wiki run 1 page")
	if err != nil {
		t.Fatalf("createWikiPage() error = %v, want nil", err)
	}
	want := WikiPage{Slug: "wiki-run-1-page", Title: "wiki run 1 page", Content: wikiContent}
	if got != want {
		t.Errorf("createWikiPage() = %+v, want %+v", got, want)
	}

	requests := stub.recordedRequests()
	if len(requests) != 1 {
		t.Fatalf("createWikiPage() sent %d requests, want 1", len(requests))
	}
	if requests[0].Body["format"] != "markdown" {
		t.Errorf("createWikiPage() sent format %v, want markdown", requests[0].Body["format"])
	}
}

// TestDeleteWikiPage_Endings_ToleratesOneACaseDeleted checks that a page a
// case already deleted is not a cleanup failure and any other refusal is.
func TestDeleteWikiPage_Endings_ToleratesOneACaseDeleted(t *testing.T) {
	cases := []struct {
		name    string
		answer  scriptedAnswer
		wantErr bool
	}{
		{name: "deleted", answer: stubNoContent()},
		{name: "already gone", answer: stubRefusal(http.StatusNotFound, "404 Wiki Page Not Found")},
		{name: "refused", answer: stubRefusal(http.StatusForbidden, "403 Forbidden"), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodDelete, "/api/v4/projects/8/wikis/wiki-run-1-page", tc.answer)

			err := deleteWikiPage(t.Context(), client, 8, "wiki-run-1-page")
			if (err != nil) != tc.wantErr {
				t.Errorf("deleteWikiPage() error = %v, wantErr = %t", err, tc.wantErr)
			}
		})
	}
}
