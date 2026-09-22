//go:build e2e

// group_tracker_test.go drives the group label and group milestone creators
// against the stub: what each asks GitLab for, what it reads back, and a
// refusal handed back as it came.

package fixture

import (
	"net/http"
	"testing"
	"time"
)

// TestCreateGroupLabel_Answers_SendsTheFixtureColor checks the group label
// creator sends the name, the description and the one color every fixture
// label carries.
func TestCreateGroupLabel_Answers_SendsTheFixtureColor(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/groups/1/labels",
		stubCreated(map[string]any{"id": 11, "name": "label-run", "color": labelColor}),
		stubRefusal(http.StatusConflict, "Label already exists"))

	got, err := createGroupLabel(t.Context(), client, 1, "label-run", "the World's")
	if err != nil {
		t.Fatalf("createGroupLabel() error = %v, want nil", err)
	}
	if want := (GroupLabel{ID: 11, Name: "label-run", Color: labelColor}); got != want {
		t.Errorf("createGroupLabel() = %+v, want %+v", got, want)
	}
	sent := stub.recordedRequests()[0].Body
	if sent["name"] != "label-run" || sent["color"] != labelColor || sent["description"] != "the World's" {
		t.Errorf("createGroupLabel() sent %v, want the name, the fixture color and the description", sent)
	}

	got, err = createGroupLabel(t.Context(), client, 1, "label-run", "the World's")
	if !IsStatus(err, http.StatusConflict) || got != (GroupLabel{}) {
		t.Errorf("createGroupLabel() on a refusal = %+v, %v; want nothing and GitLab's 409", got, err)
	}
}

// TestCreateGroupMilestone_Answers_RunsTwoWeeksFromTheStart checks the group
// milestone creator dates the milestone from the start it was given and for
// two weeks, and reads back the IID the group milestone actions take.
func TestCreateGroupMilestone_Answers_RunsTwoWeeksFromTheStart(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/groups/1/milestones",
		stubCreated(map[string]any{"id": 12, "iid": 3, "title": "milestone-run"}),
		stubRefusal(http.StatusBadRequest, "title has already been taken"))
	start := time.Date(2026, time.September, 23, 10, 0, 0, 0, time.UTC)

	got, err := createGroupMilestone(t.Context(), client, 1, "milestone-run", "the World's", start)
	if err != nil {
		t.Fatalf("createGroupMilestone() error = %v, want nil", err)
	}
	if want := (GroupMilestone{ID: 12, IID: 3, Title: "milestone-run"}); got != want {
		t.Errorf("createGroupMilestone() = %+v, want %+v", got, want)
	}
	sent := stub.recordedRequests()[0].Body
	wantSent := map[string]any{"title": "milestone-run", "description": "the World's", "start_date": "2026-09-23", "due_date": "2026-10-07"}
	for field, want := range wantSent {
		t.Run(field, func(t *testing.T) {
			if sent[field] != want {
				t.Errorf("createGroupMilestone() sent %s = %v, want %v", field, sent[field], want)
			}
		})
	}

	got, err = createGroupMilestone(t.Context(), client, 1, "milestone-run", "the World's", start)
	if !IsStatus(err, http.StatusBadRequest) || got != (GroupMilestone{}) {
		t.Errorf("createGroupMilestone() on a refusal = %+v, %v; want nothing and GitLab's 400", got, err)
	}
}
