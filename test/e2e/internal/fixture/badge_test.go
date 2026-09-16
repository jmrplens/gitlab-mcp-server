//go:build e2e

// badge_test.go drives the badge builder's pure halves against the stub: what
// a create sends and reads back, and the two endings a deletion has.

package fixture

import (
	"net/http"
	"testing"
)

// TestCreateProjectBadge_Created_SendsBothURLsAndReadsThemBack checks that the
// builder sends the link and image GitLab stores and returns what a case
// compares with.
func TestCreateProjectBadge_Created_SendsBothURLsAndReadsThemBack(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/projects/3/badges", stubCreated(map[string]any{
		"id": 12, "name": "e2e-badge", "link_url": badgeLinkURL, "image_url": badgeImageURL,
	}))

	got, err := createProjectBadge(t.Context(), client, 3, "e2e-badge")
	if err != nil {
		t.Fatalf("createProjectBadge() error = %v, want nil", err)
	}
	want := Badge{ID: 12, Name: "e2e-badge", LinkURL: badgeLinkURL, ImageURL: badgeImageURL}
	if got != want {
		t.Errorf("createProjectBadge() = %+v, want %+v", got, want)
	}

	requests := stub.recordedRequests()
	if len(requests) != 1 {
		t.Fatalf("createProjectBadge() sent %d requests, want 1", len(requests))
	}
	if requests[0].Body["link_url"] != badgeLinkURL || requests[0].Body["image_url"] != badgeImageURL {
		t.Errorf("createProjectBadge() sent %v, want both badge URLs", requests[0].Body)
	}
}

// TestDeleteProjectBadge_Endings_ToleratesOneACaseDeleted checks that a badge
// a case already deleted is not a cleanup failure and any other refusal is.
func TestDeleteProjectBadge_Endings_ToleratesOneACaseDeleted(t *testing.T) {
	cases := []struct {
		name    string
		answer  scriptedAnswer
		wantErr bool
	}{
		{name: "deleted", answer: stubNoContent()},
		{name: "already gone", answer: stubRefusal(http.StatusNotFound, "404 Not found")},
		{name: "refused", answer: stubRefusal(http.StatusForbidden, "403 Forbidden"), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodDelete, "/api/v4/projects/3/badges/12", tc.answer)

			err := deleteProjectBadge(t.Context(), client, 3, 12)
			if (err != nil) != tc.wantErr {
				t.Errorf("deleteProjectBadge() error = %v, wantErr = %t", err, tc.wantErr)
			}
		})
	}
}
