//go:build e2e

// push_rule_test.go drives the push rule builder's pure halves against the
// stub: what a create sends and reads back, the two endings a deletion has,
// and the three answers the read-back distinguishes.

package fixture

import (
	"net/http"
	"testing"
)

// TestCreateProjectPushRule_Created_SendsThePatternAndReadsItBack checks that
// the builder sends the branch pattern and returns what a case compares with.
func TestCreateProjectPushRule_Created_SendsThePatternAndReadsItBack(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/projects/4/push_rule", stubCreated(map[string]any{
		"id": 9, "project_id": 4, "branch_name_regex": pushRuleBranchPattern,
	}))

	got, err := createProjectPushRule(t.Context(), client, 4)
	if err != nil {
		t.Fatalf("createProjectPushRule() error = %v, want nil", err)
	}
	want := PushRule{ID: 9, BranchNameRegex: pushRuleBranchPattern}
	if got != want {
		t.Errorf("createProjectPushRule() = %+v, want %+v", got, want)
	}

	requests := stub.recordedRequests()
	if len(requests) != 1 {
		t.Fatalf("createProjectPushRule() sent %d requests, want 1", len(requests))
	}
	if requests[0].Body["branch_name_regex"] != pushRuleBranchPattern {
		t.Errorf("createProjectPushRule() sent %v, want the branch pattern", requests[0].Body)
	}
}

// TestDeleteProjectPushRule_Endings_ToleratesOneACaseDeleted checks that a rule
// a case already deleted is not a cleanup failure and any other refusal is.
func TestDeleteProjectPushRule_Endings_ToleratesOneACaseDeleted(t *testing.T) {
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
			stub.answers(http.MethodDelete, "/api/v4/projects/4/push_rule", tc.answer)

			err := deleteProjectPushRule(t.Context(), client, 4)
			if (err != nil) != tc.wantErr {
				t.Errorf("deleteProjectPushRule() error = %v, wantErr = %t", err, tc.wantErr)
			}
		})
	}
}
