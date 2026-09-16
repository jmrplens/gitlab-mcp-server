//go:build e2e

// runner_test.go drives the disposable runner builder against the stub, and
// pins the two properties that keep it disposable: it is created paused and
// tagged, so nothing in the suite is ever scheduled onto it.

package fixture

import (
	"net/http"
	"testing"
)

// TestCreateProjectRunner_Created_IsPausedAndTagged checks the shape the
// runner is asked for, which is what stops it picking up the jobs the Docker
// runner is meant to run.
func TestCreateProjectRunner_Created_IsPausedAndTagged(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/user/runners", stubCreated(map[string]any{
		"id": 31, "token": "glrt-fixture",
	}))

	got, err := createProjectRunner(t.Context(), client, 5, "e2e-runner")
	if err != nil {
		t.Fatalf("createProjectRunner() error = %v, want nil", err)
	}
	want := Runner{ID: 31, Description: "e2e-runner", Token: "glrt-fixture"}
	if got != want {
		t.Errorf("createProjectRunner() = %+v, want %+v", got, want)
	}

	requests := stub.recordedRequests()
	if len(requests) != 1 {
		t.Fatalf("createProjectRunner() sent %d requests, want 1", len(requests))
	}
	body := requests[0].Body
	if body["paused"] != true {
		t.Errorf("createProjectRunner() sent paused %v, want true", body["paused"])
	}
	if body["run_untagged"] != false {
		t.Errorf("createProjectRunner() sent run_untagged %v, want false", body["run_untagged"])
	}
	if body["runner_type"] != "project_type" || body["project_id"] != float64(5) {
		t.Errorf("createProjectRunner() sent %v, want a project runner for project 5", body)
	}
	tags, _ := body["tag_list"].([]any)
	if len(tags) != 1 || tags[0] != DisposableRunnerTag {
		t.Errorf("createProjectRunner() sent tags %v, want [%s]", body["tag_list"], DisposableRunnerTag)
	}
}

// TestDeleteRunner_Endings_ToleratesOneACaseDeleted checks that a runner a
// case already deleted is not a cleanup failure and any other refusal is.
func TestDeleteRunner_Endings_ToleratesOneACaseDeleted(t *testing.T) {
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
			stub.answers(http.MethodDelete, "/api/v4/runners/31", tc.answer)

			err := deleteRunner(t.Context(), client, 31)
			if (err != nil) != tc.wantErr {
				t.Errorf("deleteRunner() error = %v, wantErr = %t", err, tc.wantErr)
			}
		})
	}
}
