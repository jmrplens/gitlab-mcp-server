//go:build e2e

// env_test.go covers the cleanup ledger every test's fixtures hang off,
// ported from the suite this replaces (resource_ledger_ce_test.go).

package harness

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// TestLedger_MultipleRecords_CleansInReverseOrder checks that cleanups run
// last-in-first-out.
//
// The order is not a preference: a project created inside a group has to be
// removed before the group, and the ledger's only knowledge of that dependency
// is the order the two were registered in.
func TestLedger_MultipleRecords_CleansInReverseOrder(t *testing.T) {
	var l ledger
	var cleaned []string

	// sequential: two records registered in order, which is what the cleanup order is read from
	for _, label := range []string{"project", "group"} {
		if err := l.register(ledgerRecord{Label: label, Cleanup: func(context.Context) error {
			cleaned = append(cleaned, label)
			return nil
		}}); err != nil {
			t.Fatalf("register(%s) error = %v, want nil", label, err)
		}
	}

	if failures := l.cleanupAll(context.Background(), t); len(failures) != 0 {
		t.Fatalf("cleanupAll() failures = %v, want none", failures)
	}
	if want := []string{"group", "project"}; strings.Join(cleaned, ",") != strings.Join(want, ",") {
		t.Fatalf("cleanup order = %v, want %v", cleaned, want)
	}
}

// TestLedger_List_ReturnsACopy checks that a caller reading the ledger cannot
// change what it will clean up.
func TestLedger_List_ReturnsACopy(t *testing.T) {
	var l ledger
	if err := l.register(ledgerRecord{Label: "project"}); err != nil {
		t.Fatalf("register() error = %v, want nil", err)
	}

	records := l.list()
	records[0].Label = "changed"

	if got := l.list()[0].Label; got != "project" {
		t.Fatalf("ledger record label = %q, want the original value", got)
	}
}

// TestLedger_ConcurrentRegister_KeepsEveryRecord checks that the ledger is
// safe for the fixtures a test builds from several goroutines.
//
// A dropped record is a resource nobody deletes, which the next run then trips
// over as a name that is already taken.
func TestLedger_ConcurrentRegister_KeepsEveryRecord(t *testing.T) {
	var l ledger
	var wg sync.WaitGroup

	for i := range 50 {
		wg.Go(func() {
			if err := l.register(ledgerRecord{Label: strconv.Itoa(i)}); err != nil {
				t.Errorf("register() error = %v, want nil", err)
				return
			}
		})
	}
	wg.Wait()

	if got := len(l.list()); got != 50 {
		t.Fatalf("registered records = %d, want 50", got)
	}
}

// TestLedger_FailingCleanup_ReportsTheRecord checks that a cleanup failure
// names the resource it could not remove and carries the original cause.
//
// The label is deliberately built from the record's own fields and never from
// a URL: a cleanup failure is printed in CI logs, and a GitLab URL carrying a
// token in it would be printed with it.
func TestLedger_FailingCleanup_ReportsTheRecord(t *testing.T) {
	var l ledger
	if err := l.register(ledgerRecord{
		Label:     "project e2e-1",
		OwnerTest: "TestCommon_Projects",
		RunID:     "run-1",
		Cleanup:   func(context.Context) error { return errors.New("delete failed") },
	}); err != nil {
		t.Fatalf("register() error = %v, want nil", err)
	}

	failures := l.cleanupAll(context.Background(), t)
	if len(failures) != 1 {
		t.Fatalf("failures = %d, want 1", len(failures))
	}
	for _, fragment := range []string{`label="project e2e-1"`, `owner="TestCommon_Projects"`, `run_id="run-1"`, "delete failed"} {
		t.Run(fragment, func(t *testing.T) {
			if !strings.Contains(failures[0].Error(), fragment) {
				t.Fatalf("failure message = %q, want it to carry %q", failures[0].Error(), fragment)
			}
		})
	}
}

// TestLedger_SecondCleanup_DoesNothing checks that cleanup is idempotent.
//
// A test that cleans up early and then ends would otherwise have its t.Cleanup
// delete everything a second time, which on GitLab is a 404 for every record
// and a wall of failures that mean nothing.
func TestLedger_SecondCleanup_DoesNothing(t *testing.T) {
	var l ledger
	calls := 0
	if err := l.register(ledgerRecord{Label: "project", Cleanup: func(context.Context) error {
		calls++
		return nil
	}}); err != nil {
		t.Fatalf("register() error = %v, want nil", err)
	}

	l.cleanupAll(context.Background(), t)
	l.cleanupAll(context.Background(), t)

	if calls != 1 {
		t.Fatalf("cleanup calls = %d, want 1", calls)
	}
}

// TestLedger_RegisterAfterCleanup_IsRefused checks that a record offered after
// the cleanups have run is reported rather than accepted.
//
// Accepting it would mean the resource is recorded and never removed, and
// nothing would say so: the ledger would look as though it had done its work.
func TestLedger_RegisterAfterCleanup_IsRefused(t *testing.T) {
	var l ledger

	l.cleanupAll(context.Background(), t)
	err := l.register(ledgerRecord{Label: "late"})

	if !errors.Is(err, errLedgerClosed) {
		t.Fatalf("register() error = %v, want %v", err, errLedgerClosed)
	}
}

// TestLedger_CancelledContext_StopsAndReportsIt checks that cleanup stops when
// its budget is gone rather than working through the rest with a context
// nothing can be done with.
//
// It runs under synctest so the budget can expire without the test waiting for
// it: the ledger's work here is in-process, so the fake clock advances the
// moment every goroutine is blocked.
func TestLedger_CancelledContext_StopsAndReportsIt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var l ledger
		calls := 0
		for range 3 {
			if err := l.register(ledgerRecord{Label: "slow", Cleanup: func(ctx context.Context) error {
				calls++
				<-ctx.Done()
				return nil
			}}); err != nil {
				t.Fatalf("register() error = %v, want nil", err)
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		failures := l.cleanupAll(ctx, t)

		if calls != 1 {
			t.Fatalf("cleanup calls = %d, want 1: the ledger should stop at the first expired context", calls)
		}
		if len(failures) != 1 || !errors.Is(failures[0], context.DeadlineExceeded) {
			t.Fatalf("failures = %v, want the expired context", failures)
		}
	})
}
