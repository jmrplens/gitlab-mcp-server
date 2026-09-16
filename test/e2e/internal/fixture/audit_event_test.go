//go:build e2e

// audit_event_test.go drives the audit event builder's pure halves against
// the stub: the change that causes an event, the project log's answer when
// nothing has been recorded yet, and the instance log's answer to the one
// question a case's world asks of it.

package fixture

import (
	"net/http"
	"testing"
)

// TestCauseProjectAuditEvent_Edit_ChangesTheDescription checks the builder
// makes the change GitLab records, and reports a refused edit rather than
// waiting for an event nothing caused.
func TestCauseProjectAuditEvent_Edit_ChangesTheDescription(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.addProject(2, "eval", "eval/eval")

	if err := causeProjectAuditEvent(t.Context(), client, 2); err != nil {
		t.Fatalf("causeProjectAuditEvent() error = %v, want nil", err)
	}
	if err := causeProjectAuditEvent(t.Context(), client, 99); err == nil {
		t.Error("causeProjectAuditEvent() on a project the instance does not have reported no error")
	}
}

// TestFirstProjectAuditEvent_Answers covers the three answers the wait
// distinguishes: an event, a log that holds none yet, and a refusal.
func TestFirstProjectAuditEvent_Answers(t *testing.T) {
	cases := []struct {
		name    string
		answer  scriptedAnswer
		want    AuditEvent
		wantOK  bool
		wantErr bool
	}{
		{
			name: "recorded",
			answer: stubOK([]map[string]any{
				{"id": 77, "entity_type": "Project", "entity_id": 2},
			}),
			want:   AuditEvent{ID: 77, EntityType: "Project", EntityID: 2},
			wantOK: true,
		},
		{name: "nothing yet", answer: stubOK([]map[string]any{})},
		// GitLab has been seen to answer a row with no identifier while it
		// is still writing one; that is "not yet" rather than an event.
		{name: "an event with no id", answer: stubOK([]map[string]any{{"entity_type": "Project"}})},
		{name: "refused", answer: stubRefusal(http.StatusForbidden, "403 Forbidden"), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodGet, "/api/v4/projects/2/audit_events", tc.answer)

			got, ok, err := firstProjectAuditEvent(t.Context(), client, 2)
			if (err != nil) != tc.wantErr {
				t.Fatalf("firstProjectAuditEvent() error = %v, wantErr = %t", err, tc.wantErr)
			}
			if ok != tc.wantOK {
				t.Errorf("firstProjectAuditEvent() ok = %t, want %t", ok, tc.wantOK)
			}
			if got != tc.want {
				t.Errorf("firstProjectAuditEvent() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestInstanceAuditLogHolds_Answers checks the instance half asks about the
// event the project log named rather than about the log being non-empty,
// which is the distinction the whole two-step wait exists for.
func TestInstanceAuditLogHolds_Answers(t *testing.T) {
	cases := []struct {
		name    string
		answer  scriptedAnswer
		want    bool
		wantErr bool
	}{
		{
			name:   "carries it",
			answer: stubOK([]map[string]any{{"id": 12}, {"id": 77}}),
			want:   true,
		},
		{
			name:   "busy with somebody else's events",
			answer: stubOK([]map[string]any{{"id": 12}, {"id": 13}}),
		},
		{name: "empty", answer: stubOK([]map[string]any{})},
		{name: "refused", answer: stubRefusal(http.StatusForbidden, "403 Forbidden"), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodGet, "/api/v4/audit_events", tc.answer)

			got, err := instanceAuditLogHolds(t.Context(), client, 77)
			if (err != nil) != tc.wantErr {
				t.Fatalf("instanceAuditLogHolds() error = %v, wantErr = %t", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("instanceAuditLogHolds() = %t, want %t", got, tc.want)
			}
		})
	}
}
