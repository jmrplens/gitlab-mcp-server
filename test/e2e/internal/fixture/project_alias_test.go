//go:build e2e

// project_alias_test.go drives the alias builder's pure halves against the
// stub: what a create sends, the two endings a deletion has, and what the
// read-back answers.

package fixture

import (
	"net/http"
	"testing"
)

// TestCreateProjectAlias_Created_SendsBothHalvesAndReadsThemBack checks that
// the builder names the alias and the project it points at.
func TestCreateProjectAlias_Created_SendsBothHalvesAndReadsThemBack(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/project_aliases", stubCreated(map[string]any{
		"id": 3, "project_id": 11, "name": "eval-alias",
	}))

	got, err := createProjectAlias(t.Context(), client, 11, "eval-alias")
	if err != nil {
		t.Fatalf("createProjectAlias() error = %v, want nil", err)
	}
	want := ProjectAlias{Name: "eval-alias", ProjectID: 11}
	if got != want {
		t.Errorf("createProjectAlias() = %+v, want %+v", got, want)
	}

	requests := stub.recordedRequests()
	if len(requests) != 1 {
		t.Fatalf("createProjectAlias() sent %d requests, want 1", len(requests))
	}
	if requests[0].Body["name"] != "eval-alias" {
		t.Errorf("createProjectAlias() sent %v, want the alias name", requests[0].Body)
	}
}

// TestDeleteProjectAlias_Endings_ToleratesOneACaseDeleted checks that an alias
// a case deleted, or never created under a reserved name, is not a cleanup
// failure and any other refusal is.
func TestDeleteProjectAlias_Endings_ToleratesOneACaseDeleted(t *testing.T) {
	cases := []struct {
		name    string
		answer  scriptedAnswer
		wantErr bool
	}{
		{name: "deleted", answer: stubNoContent()},
		{name: "never created", answer: stubRefusal(http.StatusNotFound, "404 Not found")},
		{name: "refused", answer: stubRefusal(http.StatusForbidden, "403 Forbidden"), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodDelete, "/api/v4/project_aliases/eval-alias", tc.answer)

			err := deleteProjectAlias(t.Context(), client, "eval-alias")
			if (err != nil) != tc.wantErr {
				t.Errorf("deleteProjectAlias() error = %v, wantErr = %t", err, tc.wantErr)
			}
		})
	}
}
