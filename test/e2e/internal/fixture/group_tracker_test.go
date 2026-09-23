//go:build e2e

// group_tracker_test.go drives the group label and group milestone builders
// against the stub, whole and through the halves that take a client: what each
// asks GitLab for, what it reads back, and a refusal handed back as it came.

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

// TestNewGroupLabel_Detached_AsksForItUnderTheRunForThisTest checks the group
// label builder whole: asked for in the group it was given, named under the
// run and described by the test that made it, and handed back as GitLab
// answered it.
func TestNewGroupLabel_Detached_AsksForItUnderTheRunForThisTest(t *testing.T) {
	stub, e := detachedStub(t)
	stub.answers(http.MethodPost, "/api/v4/groups/1/labels", stubCreated(map[string]any{"id": 11, "name": "as-answered", "color": labelColor}))

	got := NewGroupLabel(e, Group{ID: 1}, "label")

	if want := (GroupLabel{ID: 11, Name: "as-answered", Color: labelColor}); got != want {
		t.Errorf("NewGroupLabel() = %+v, want %+v", got, want)
	}
	sent := requestTo(t, stub, http.MethodPost, "/api/v4/groups/1/labels").Body
	if name, _ := sent["name"].(string); !isScopedName(name, "label", e) || sent["description"] != "e2e: "+t.Name() {
		t.Errorf("NewGroupLabel() sent %v, want a name under the run described by this test", sent)
	}
}

// TestNewGroupMilestone_Detached_RunsTwoWeeksFromToday checks the group
// milestone builder whole: asked for in the group it was given, named under
// the run, and dated from today for two weeks.
func TestNewGroupMilestone_Detached_RunsTwoWeeksFromToday(t *testing.T) {
	stub, e := detachedStub(t)
	stub.answers(http.MethodPost, "/api/v4/groups/1/milestones", stubCreated(map[string]any{"id": 12, "iid": 3, "title": "as-answered"}))
	before := time.Now().UTC()

	got := NewGroupMilestone(e, Group{ID: 1}, "milestone")

	after := time.Now().UTC()
	if want := (GroupMilestone{ID: 12, IID: 3, Title: "as-answered"}); got != want {
		t.Errorf("NewGroupMilestone() = %+v, want %+v", got, want)
	}
	sent := requestTo(t, stub, http.MethodPost, "/api/v4/groups/1/milestones").Body
	if title, _ := sent["title"].(string); !isScopedName(title, "milestone", e) || sent["description"] != "e2e: "+t.Name() {
		t.Errorf("NewGroupMilestone() sent %v, want a title under the run described by this test", sent)
	}
	start, _ := sent["start_date"].(string)
	due, _ := sent["due_date"].(string)
	if start != before.Format(time.DateOnly) && start != after.Format(time.DateOnly) {
		t.Errorf("NewGroupMilestone() starts on %q, want today", start)
	}
	if length := mustDate(t, due).Sub(mustDate(t, start)); length != groupMilestoneLength {
		t.Errorf("NewGroupMilestone() runs from %s to %s, %s, want %s", start, due, length, groupMilestoneLength)
	}
}

// mustDate reads a date the way GitLab sends one, failing the test on
// anything else.
func mustDate(t *testing.T, value string) time.Time {
	t.Helper()
	date, err := time.Parse(time.DateOnly, value)
	if err != nil {
		t.Fatalf("reading date %q: %v", value, err)
	}
	return date
}
