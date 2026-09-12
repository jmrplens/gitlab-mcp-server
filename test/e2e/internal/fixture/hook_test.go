//go:build e2e

// hook_test.go drives the hook events reader against the stub: a listing
// that is empty at first and holds a delivery later, the 404 GitLab answers
// before any delivery exists, and a budget that runs out.

package fixture

import (
	"context"
	"errors"
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
