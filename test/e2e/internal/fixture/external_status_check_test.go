//go:build e2e

// external_status_check_test.go drives the status check builder's pure halves
// against the stub: what a create sends, the two endings a deletion has, and
// the listing the read-back is made of.

package fixture

import (
	"net/http"
	"testing"
)

// TestCreateExternalStatusCheck_Created_SendsTheURLAndReadsItBack checks that
// the builder names the check and the address GitLab would post to.
func TestCreateExternalStatusCheck_Created_SendsTheURLAndReadsItBack(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/projects/6/external_status_checks", stubCreated(map[string]any{
		"id": 21, "name": "eval-check", "project_id": 6, "external_url": externalStatusCheckURL,
	}))

	got, err := createExternalStatusCheck(t.Context(), client, 6, "eval-check")
	if err != nil {
		t.Fatalf("createExternalStatusCheck() error = %v, want nil", err)
	}
	want := ExternalStatusCheck{ID: 21, Name: "eval-check", ExternalURL: externalStatusCheckURL}
	if got != want {
		t.Errorf("createExternalStatusCheck() = %+v, want %+v", got, want)
	}

	requests := stub.recordedRequests()
	if len(requests) != 1 {
		t.Fatalf("createExternalStatusCheck() sent %d requests, want 1", len(requests))
	}
	if requests[0].Body["external_url"] != externalStatusCheckURL {
		t.Errorf("createExternalStatusCheck() sent %v, want the external URL", requests[0].Body)
	}
}

// TestDeleteExternalStatusCheck_Endings_ToleratesOneACaseDeleted checks that a
// check a case already deleted is not a cleanup failure and any other refusal
// is.
func TestDeleteExternalStatusCheck_Endings_ToleratesOneACaseDeleted(t *testing.T) {
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
			stub.answers(http.MethodDelete, "/api/v4/projects/6/external_status_checks/21", tc.answer)

			err := deleteExternalStatusCheck(t.Context(), client, 6, 21)
			if (err != nil) != tc.wantErr {
				t.Errorf("deleteExternalStatusCheck() error = %v, wantErr = %t", err, tc.wantErr)
			}
		})
	}
}
