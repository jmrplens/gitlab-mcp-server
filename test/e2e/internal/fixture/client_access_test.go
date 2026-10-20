//go:build e2e

// client_access_test.go drives the wait for a credential that is not a
// user's own token against the stub: it drains the queues first, waits
// through the 404 a project not yet granted is answered with, holds for
// every project named, and fails its test naming them when the wait ends
// without them.

package fixture

import (
	"context"
	"testing"
	"time"
)

// TestAwaitClientProjectAccess_ProjectGrantedLater_WaitsForEveryOneAfterTheQueues
// checks the wait a group token's session needs before its first call: the
// Sidekiq queues are drained first, since the grant is a background job, and
// a project answered 404 is read again until it reads, rather than ending the
// wait on the first answer or on the first project that reads.
func TestAwaitClientProjectAccess_ProjectGrantedLater_WaitsForEveryOneAfterTheQueues(t *testing.T) {
	stub, e := detachedStub(t)
	stub.configure(func() { stub.enqueued = 1 })
	stub.addProject(7, "readable", "e2e-group/readable")
	// The drain reads the stats twice, a quarter second apart, so the first
	// read of project 8 comes well before the grant and the next one, a poll
	// interval later, well after it.
	granted := time.AfterFunc(600*time.Millisecond, func() { stub.addProject(8, "granted", "e2e-group/granted") })
	t.Cleanup(func() { granted.Stop() })

	AwaitClientProjectAccess(e, e.Client(), Project{ID: 7}, Project{ID: 8})

	stub.configure(func() {
		if stub.enqueued != 0 {
			t.Errorf("the wait returned with %d job(s) still enqueued, want the queues drained first", stub.enqueued)
		}
		if _, seen := stub.projects[8]; !seen {
			t.Error("the wait returned before project 8 was readable")
		}
	})
}

// runClientAccessEnded is the fatal case of the wait: a context that has
// already ended stops the poll at once, which is the wait ending without the
// projects and without its budget's minutes.
func runClientAccessEnded(t *testing.T) {
	t.Helper()
	_, e := detachedStub(t)
	ended, cancel := context.WithCancel(e.Ctx)
	cancel()
	e.Ctx = ended
	AwaitClientProjectAccess(e, e.Client(), Project{ID: 7}, Project{ID: 8})
}
