//go:build e2e

// pipeline_schedule_test.go drives the schedule builder's pure halves against
// the stub, and pins the one property the file's comment rests on: a fixture
// schedule is created inactive, so nothing it describes ever runs by itself.

package fixture

import (
	"net/http"
	"testing"
)

// TestCreatePipelineSchedule_Created_IsInactiveAndOnTheGivenRef checks that
// the builder asks for an inactive schedule on the ref it was handed, and
// reads back what a case addresses it by.
func TestCreatePipelineSchedule_Created_IsInactiveAndOnTheGivenRef(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/projects/6/pipeline_schedules", stubCreated(map[string]any{
		"id": 21, "description": "e2e-schedule", "ref": "main", "cron": pipelineScheduleCron, "active": false,
	}))

	got, err := createPipelineSchedule(t.Context(), client, 6, "e2e-schedule", "main")
	if err != nil {
		t.Fatalf("createPipelineSchedule() error = %v, want nil", err)
	}
	want := PipelineSchedule{ID: 21, Description: "e2e-schedule", Ref: "main", Cron: pipelineScheduleCron}
	if got != want {
		t.Errorf("createPipelineSchedule() = %+v, want %+v", got, want)
	}

	requests := stub.recordedRequests()
	if len(requests) != 1 {
		t.Fatalf("createPipelineSchedule() sent %d requests, want 1", len(requests))
	}
	if requests[0].Body["active"] != false {
		t.Errorf("createPipelineSchedule() sent active %v, want false so nothing runs by itself", requests[0].Body["active"])
	}
	if requests[0].Body["ref"] != "main" {
		t.Errorf("createPipelineSchedule() sent ref %v, want the ref it was handed", requests[0].Body["ref"])
	}
}

// TestDeletePipelineSchedule_Endings_ToleratesOneACaseDeleted checks that a
// schedule a case already deleted is not a cleanup failure and any other
// refusal is.
func TestDeletePipelineSchedule_Endings_ToleratesOneACaseDeleted(t *testing.T) {
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
			stub.answers(http.MethodDelete, "/api/v4/projects/6/pipeline_schedules/21", tc.answer)

			err := deletePipelineSchedule(t.Context(), client, 6, 21)
			if (err != nil) != tc.wantErr {
				t.Errorf("deletePipelineSchedule() error = %v, wantErr = %t", err, tc.wantErr)
			}
		})
	}
}
