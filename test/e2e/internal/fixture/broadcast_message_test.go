//go:build e2e

// broadcast_message_test.go drives the broadcast message builder against the
// stub, and pins the window the file's comment rests on: a message that is
// live from before now and ends after any run.

package fixture

import (
	"net/http"
	"testing"
	"time"
)

// TestCreateBroadcastMessage_Created_IsLiveFromBeforeNow checks that the
// message is active the moment it exists, whatever clock skew there is
// between this process and the instance, and that it outlives a run.
func TestCreateBroadcastMessage_Created_IsLiveFromBeforeNow(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/broadcast_messages", stubCreated(map[string]any{
		"id": 6, "message": "e2e broadcast fixture",
	}))

	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	got, err := createBroadcastMessage(t.Context(), client, "e2e broadcast fixture", now)
	if err != nil {
		t.Fatalf("createBroadcastMessage() error = %v, want nil", err)
	}
	if got != (BroadcastMessage{ID: 6, Message: "e2e broadcast fixture"}) {
		t.Errorf("createBroadcastMessage() = %+v, want the message GitLab answered with", got)
	}

	requests := stub.recordedRequests()
	if len(requests) != 1 {
		t.Fatalf("createBroadcastMessage() sent %d requests, want 1", len(requests))
	}
	startsAt, endsAt := parseSentTime(t, requests[0].Body, "starts_at"), parseSentTime(t, requests[0].Body, "ends_at")
	if !startsAt.Before(now) {
		t.Errorf("createBroadcastMessage() sent starts_at %s, want it before %s so the message is live at once", startsAt, now)
	}
	if !endsAt.After(now.Add(broadcastMessageLifetime - time.Minute)) {
		t.Errorf("createBroadcastMessage() sent ends_at %s, want it to outlive a run", endsAt)
	}
}

// TestDeleteBroadcastMessage_Endings_ToleratesOneACaseDeleted checks that a
// message a case already deleted is not a cleanup failure and any other
// refusal is.
func TestDeleteBroadcastMessage_Endings_ToleratesOneACaseDeleted(t *testing.T) {
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
			stub.answers(http.MethodDelete, "/api/v4/broadcast_messages/6", tc.answer)

			err := deleteBroadcastMessage(t.Context(), client, 6)
			if (err != nil) != tc.wantErr {
				t.Errorf("deleteBroadcastMessage() error = %v, wantErr = %t", err, tc.wantErr)
			}
		})
	}
}

// parseSentTime reads one RFC 3339 field out of a recorded request body.
func parseSentTime(t *testing.T, body map[string]any, field string) time.Time {
	t.Helper()
	raw, ok := body[field].(string)
	if !ok {
		t.Fatalf("the request carried no %s; body was %v", field, body)
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatalf("parsing the %s the request carried (%q): %v", field, raw, err)
	}
	return parsed
}
