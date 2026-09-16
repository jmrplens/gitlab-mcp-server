//go:build e2e

// award_emoji_test.go drives the award builders' pure halves against the
// stub. The interesting path is not the create: it is what happens when
// GitLab refuses a second award of the same emoji, which is what a retried
// attempt sees and what the builder has to read back rather than report.

package fixture

import (
	"net/http"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

// TestCreateMergeRequestAward_Created_ReadsTheAwardBack checks the ordinary
// ending: one POST and the award GitLab answered with.
func TestCreateMergeRequestAward_Created_ReadsTheAwardBack(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/projects/1/merge_requests/2/award_emoji",
		stubCreated(map[string]any{"id": 55, "name": AwardEmojiName}))

	got, err := createMergeRequestAward(t.Context(), client, 1, 2)
	if err != nil {
		t.Fatalf("createMergeRequestAward() error = %v, want nil", err)
	}
	if got != (Award{ID: 55, Name: AwardEmojiName}) {
		t.Errorf("createMergeRequestAward() = %+v, want the award GitLab answered with", got)
	}
}

// TestCreateMergeRequestAward_AlreadyTaken_ReadsTheExistingOneOutOfTheListing
// checks the retried-attempt ending: the award is already there, GitLab
// refuses the second, and the listing is what says which one it is.
func TestCreateMergeRequestAward_AlreadyTaken_ReadsTheExistingOneOutOfTheListing(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/projects/1/merge_requests/2/award_emoji",
		stubRefusal(http.StatusBadRequest, "name has already been taken"))
	stub.answers(http.MethodGet, "/api/v4/projects/1/merge_requests/2/award_emoji", stubOK([]any{
		map[string]any{"id": 12, "name": "thumbsup"},
		map[string]any{"id": 55, "name": AwardEmojiName},
	}))

	got, err := createMergeRequestAward(t.Context(), client, 1, 2)
	if err != nil {
		t.Fatalf("createMergeRequestAward() error = %v, want nil", err)
	}
	if got.ID != 55 {
		t.Errorf("createMergeRequestAward() = %+v, want the award already on the merge request", got)
	}
}

// TestCreateIssueAward_AlreadyTakenAndNotListed_IsAnErrorNamingTheEmoji
// checks that a listing which does not hold the emoji is reported, since a
// zero award ID would send the cleanup to delete nothing and a case to
// address nothing.
func TestCreateIssueAward_AlreadyTakenAndNotListed_IsAnErrorNamingTheEmoji(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/projects/1/issues/3/award_emoji",
		stubRefusal(http.StatusBadRequest, "name has already been taken"))
	stub.answers(http.MethodGet, "/api/v4/projects/1/issues/3/award_emoji", stubOK([]any{
		map[string]any{"id": 12, "name": "thumbsup"},
	}))

	_, err := createIssueAward(t.Context(), client, 1, 3)
	if err == nil {
		t.Fatal("createIssueAward() error = nil, want the missing award reported")
	}
	if !strings.Contains(err.Error(), AwardEmojiName) {
		t.Errorf("createIssueAward() error = %q, want it to name the emoji", err)
	}
}

// TestCreateIssueAward_RefusedForAnotherReason_IsReportedUnchanged checks
// that a refusal which is not the duplicate is handed back rather than sent
// to the listing, which would turn a permission failure into a confusing one
// about an emoji.
func TestCreateIssueAward_RefusedForAnotherReason_IsReportedUnchanged(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/projects/1/issues/3/award_emoji",
		stubRefusal(http.StatusForbidden, "403 Forbidden"))

	_, err := createIssueAward(t.Context(), client, 1, 3)
	if err == nil || !IsStatus(err, http.StatusForbidden) {
		t.Errorf("createIssueAward() error = %v, want the 403 handed back", err)
	}
}

// TestFirstAwardNamed_Listings_PicksTheFixturesOwnEmoji checks the picker on
// its own: an award of another name is not the fixture's, and a nil entry in
// a listing is skipped rather than dereferenced.
func TestFirstAwardNamed_Listings_PicksTheFixturesOwnEmoji(t *testing.T) {
	cases := []struct {
		name    string
		awards  []*gl.AwardEmoji
		wantID  int64
		wantErr bool
	}{
		{name: "holds it", awards: []*gl.AwardEmoji{{ID: 7, Name: AwardEmojiName}}, wantID: 7},
		{name: "nil entry first", awards: []*gl.AwardEmoji{nil, {ID: 8, Name: AwardEmojiName}}, wantID: 8},
		{name: "another emoji", awards: []*gl.AwardEmoji{{ID: 9, Name: "thumbsup"}}, wantErr: true},
		{name: "empty", awards: nil, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := firstAwardNamed(tc.awards, 4)
			if (err != nil) != tc.wantErr {
				t.Fatalf("firstAwardNamed() error = %v, wantErr = %t", err, tc.wantErr)
			}
			if got.ID != tc.wantID {
				t.Errorf("firstAwardNamed() = %+v, want award %d", got, tc.wantID)
			}
		})
	}
}

// TestToleratingGoneAward_Endings_ToleratesOneACaseRemoved checks that an
// award a case already removed is not a cleanup failure and any other refusal
// is.
func TestToleratingGoneAward_Endings_ToleratesOneACaseRemoved(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		wantErr bool
	}{
		{name: "removed", err: nil},
		{name: "already gone", err: statusError(http.StatusNotFound, "404 Not found")},
		{name: "refused", err: statusError(http.StatusForbidden, "403 Forbidden"), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := toleratingGoneAward(tc.err, 55); (err != nil) != tc.wantErr {
				t.Errorf("toleratingGoneAward() error = %v, wantErr = %t", err, tc.wantErr)
			}
		})
	}
}
