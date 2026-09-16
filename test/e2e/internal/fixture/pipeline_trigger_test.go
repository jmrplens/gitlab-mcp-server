//go:build e2e

// pipeline_trigger_test.go drives the trigger builder's pure halves against
// the stub: what a create reads back, including the token a firing case
// needs, and the two endings a deletion has.

package fixture

import (
	"net/http"
	"testing"
)

// TestCreatePipelineTrigger_Created_ReadsTheTokenBack checks that the secret
// is carried out of the answer: a trigger whose token was dropped is a
// fixture a firing case cannot use.
func TestCreatePipelineTrigger_Created_ReadsTheTokenBack(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/projects/2/triggers", stubCreated(map[string]any{
		"id": 8, "description": "e2e-trigger", "token": "glptt-fixture",
	}))

	got, err := createPipelineTrigger(t.Context(), client, 2, "e2e-trigger")
	if err != nil {
		t.Fatalf("createPipelineTrigger() error = %v, want nil", err)
	}
	want := PipelineTrigger{ID: 8, Description: "e2e-trigger", Token: "glptt-fixture"}
	if got != want {
		t.Errorf("createPipelineTrigger() = %+v, want %+v", got, want)
	}

	requests := stub.recordedRequests()
	if len(requests) != 1 || requests[0].Body["description"] != "e2e-trigger" {
		t.Errorf("createPipelineTrigger() sent %v, want one create carrying the description", requests)
	}
}

// TestDeletePipelineTrigger_Endings_ToleratesOneACaseDeleted checks that a
// trigger a case already deleted is not a cleanup failure and any other
// refusal is.
func TestDeletePipelineTrigger_Endings_ToleratesOneACaseDeleted(t *testing.T) {
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
			stub.answers(http.MethodDelete, "/api/v4/projects/2/triggers/8", tc.answer)

			err := deletePipelineTrigger(t.Context(), client, 2, 8)
			if (err != nil) != tc.wantErr {
				t.Errorf("deletePipelineTrigger() error = %v, wantErr = %t", err, tc.wantErr)
			}
		})
	}
}
