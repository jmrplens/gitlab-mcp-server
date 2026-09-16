//go:build e2e

// hook_test.go drives the project hook builder and the hook events reader
// against the stub: what a create sends, the two endings a deletion has, a
// listing that is empty at first and holds a delivery later, the 404 GitLab
// answers before any delivery exists, and a budget that runs out.

package fixture

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// fastHookPolls makes the reader ask the stub as fast as a unit test can
// afford, restoring the real cadence afterwards.
func fastHookPolls(t *testing.T) {
	t.Helper()
	saved := hookEventPollInterval
	hookEventPollInterval = 10 * time.Millisecond
	t.Cleanup(func() { hookEventPollInterval = saved })
}

// TestCreateProjectHook_Created_SwitchesSSLVerificationOff checks the one
// field a fixture hook cannot leave at its default: the fixture service
// speaks plain HTTP, and GitLab refuses to deliver to an unverifiable
// endpoint with verification on.
func TestCreateProjectHook_Created_SwitchesSSLVerificationOff(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/projects/1/hooks", stubCreated(map[string]any{
		"id": 14, "url": "http://e2e-fixture:8080/hook",
	}))

	got, err := createProjectHook(t.Context(), client, 1, "http://e2e-fixture:8080/hook")
	if err != nil {
		t.Fatalf("createProjectHook() error = %v, want nil", err)
	}
	if got != (ProjectHook{ID: 14, URL: "http://e2e-fixture:8080/hook"}) {
		t.Errorf("createProjectHook() = %+v, want the hook GitLab answered with", got)
	}

	requests := stub.recordedRequests()
	if len(requests) != 1 {
		t.Fatalf("createProjectHook() sent %d requests, want 1", len(requests))
	}
	if requests[0].Body["enable_ssl_verification"] != false {
		t.Errorf("createProjectHook() sent enable_ssl_verification %v, want false", requests[0].Body["enable_ssl_verification"])
	}
	if requests[0].Body["push_events"] != true {
		t.Errorf("createProjectHook() sent push_events %v, want true", requests[0].Body["push_events"])
	}
}

// TestDeleteProjectHook_Endings_ToleratesOneACaseDeleted checks that a hook a
// case already deleted is not a cleanup failure and any other refusal is.
func TestDeleteProjectHook_Endings_ToleratesOneACaseDeleted(t *testing.T) {
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
			stub.answers(http.MethodDelete, "/api/v4/projects/1/hooks/14", tc.answer)

			err := deleteProjectHook(t.Context(), client, 1, 14)
			if (err != nil) != tc.wantErr {
				t.Errorf("deleteProjectHook() error = %v, wantErr = %t", err, tc.wantErr)
			}
		})
	}
}

// TestFirstGroupHookEvent_DeliveryRecordedLater_ReturnsItsID checks that
// the reader waits through an empty listing and through a 404 and answers
// the first delivery once GitLab records one.
func TestFirstGroupHookEvent_DeliveryRecordedLater_ReturnsItsID(t *testing.T) {
	fastHookPolls(t)
	stub, client := newStubGitLab(t)
	stub.configure(func() {
		stub.hookEventAnswers = []string{"not yet", `[]`, `[{"id":31},{"id":32}]`}
	})

	got, err := firstGroupHookEvent(t.Context(), client, 4, 9)
	if err != nil {
		t.Fatalf("firstGroupHookEvent() error = %v, want nil", err)
	}
	if got != 31 {
		t.Errorf("firstGroupHookEvent() = %d, want the first delivery, 31", got)
	}
}

// TestFirstGroupHookEvent_NothingRecorded_TimesOutNamingTheState checks
// that a hook nothing was ever delivered for ends with the poll's timeout
// and the last state seen, rather than with a zero the caller would send
// on as an event id.
func TestFirstGroupHookEvent_NothingRecorded_TimesOutNamingTheState(t *testing.T) {
	fastHookPolls(t)
	stub, client := newStubGitLab(t)
	stub.configure(func() {
		stub.hookEventAnswers = []string{`[]`}
	})
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()

	got, err := firstGroupHookEvent(ctx, client, 4, 9)
	if got != 0 {
		t.Errorf("firstGroupHookEvent() = %d, want 0 when nothing was recorded", got)
	}
	if err == nil || !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, harness.ErrPollTimeout) {
		t.Errorf("firstGroupHookEvent() error = %v, want the wait to run out", err)
	}
}
