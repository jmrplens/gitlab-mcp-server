//go:build e2e

// env.go is what a test holds: a context that ends with it, names nothing else
// will take, and a ledger of what it must undo.
//
// Creating an Env is also the first harness entry point, so it is what
// triggers the bootstrap. A package whose tests are all filtered out therefore
// never reaches GitLab at all, which is the property that makes -test.list
// offline and a filtered run free.

package harness

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"
)

// errLedgerClosed is returned by a ledger asked to record a cleanup after it
// has already run its cleanups.
var errLedgerClosed = errors.New("cleanup ledger closed")

// cleanupBudget bounds everything one test undoes, and enterpriseCleanupBudget
// is the same bound on a licensed instance, where deleting a project or a
// group takes measurably longer.
const (
	cleanupBudget           = 60 * time.Second
	enterpriseCleanupBudget = 180 * time.Second
)

// Env is one test's handle on the harness.
type Env struct {
	// T is the test this Env belongs to.
	T *testing.T
	// Ctx is cancelled when the test ends, so anything the test started stops
	// with it.
	Ctx context.Context

	runID  string
	ledger *ledger
	inst   *instance
}

// New prepares the harness for one test and returns its Env.
//
// The first call of a package runs the bootstrap: the probe, the runtime guard
// and the preparation the instance needs before the first write. A test whose
// declared Needs this run does not provide is skipped here, before it has
// changed anything.
func New(t *testing.T, opts ...Option) *Env {
	t.Helper()

	inst := bootstrap(t)

	var options envOptions
	for _, opt := range opts {
		opt(&options)
	}
	requireNeeds(t, inst, options.needs)

	// Registered in this order on purpose: t.Cleanup runs last-in-first-out,
	// so the ledger's undo work still holds every lock the test declared, and
	// the gate is only given back once that work is done.
	release := gate.enter(topLevelTest(t.Name()), options.serial)
	t.Cleanup(release)
	unlock := acquireLocks(options.locks)
	t.Cleanup(unlock)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	env := &Env{T: t, Ctx: ctx, runID: inst.runID, ledger: &ledger{}, inst: inst}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), inst.cleanupBudget())
		defer cleanupCancel()
		env.ledger.cleanupAll(cleanupCtx, t)
	})
	return env
}

// RunID returns the identifier every name this test hands out carries.
func (e *Env) RunID() string { return e.runID }

// Name returns a GitLab-safe name for a resource this test is about to
// create, scoped to the run so a sweep can recognize it later.
func (e *Env) Name(prefix string) string {
	return uniqueName(e.runID, prefix+"-"+sanitizeTestName(e.T.Name()))
}

// Defer records how to undo something the test created. Cleanups run in
// reverse order when the test ends, inside one bounded context, so a child
// resource is removed before the parent it hangs off.
//
// A failure is reported and does not stop the rest: the whole point of the
// ledger is that one object nobody can delete does not strand every object
// registered before it.
func (e *Env) Defer(label string, cleanup func(context.Context) error) {
	e.T.Helper()
	err := e.ledger.register(ledgerRecord{
		Label:     label,
		OwnerTest: e.T.Name(),
		RunID:     e.runID,
		CreatedAt: time.Now(),
		Cleanup:   cleanup,
	})
	if err != nil {
		e.T.Errorf("recording cleanup: %v", err)
	}
}

// ledgerRecord is one thing a test has to undo.
type ledgerRecord struct {
	// Label says what the resource is, in words a failure message can carry.
	Label string
	// OwnerTest is the test that created it.
	OwnerTest string
	// RunID scopes it to one run.
	RunID string
	// CreatedAt is when it was recorded.
	CreatedAt time.Time
	// Cleanup removes it. A nil Cleanup records the resource without
	// promising to delete it, which is what a fixture that outlives the test
	// does.
	Cleanup func(context.Context) error
}

// describe returns a label for a cleanup failure that names the resource and
// carries no credential: a record's own fields are the only thing printed,
// never the URL a token might be embedded in.
func (r ledgerRecord) describe() string {
	return fmt.Sprintf("label=%q owner=%q run_id=%q", r.Label, r.OwnerTest, r.RunID)
}

// ledger records what one test must undo and undoes it once.
type ledger struct {
	mu      sync.Mutex
	records []ledgerRecord
	cleaned bool
}

// register adds a record, refusing one offered after the cleanups have run: a
// late registration would be silently dropped otherwise, and the resource it
// names would be left behind with nothing to say so.
func (l *ledger) register(record ledgerRecord) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.cleaned {
		return fmt.Errorf("register %s: %w", record.describe(), errLedgerClosed)
	}
	l.records = append(l.records, record)
	return nil
}

// list returns a copy of what has been registered, so a caller reading the
// ledger cannot change it.
func (l *ledger) list() []ledgerRecord {
	l.mu.Lock()
	defer l.mu.Unlock()

	return slices.Clone(l.records)
}

// cleanupAll runs every cleanup in reverse registration order and returns what
// failed. It is idempotent: a second call does nothing, so a test that cleans
// up early is not cleaned up twice by its own t.Cleanup.
//
// It stops at a cancelled context rather than working through the rest with a
// context nothing can be done with, and reports that as a failure of its own.
func (l *ledger) cleanupAll(ctx context.Context, tb testing.TB) []error {
	tb.Helper()

	l.mu.Lock()
	if l.cleaned {
		l.mu.Unlock()
		return nil
	}
	l.cleaned = true
	records := slices.Clone(l.records)
	l.mu.Unlock()

	failures := make([]error, 0)
	for _, record := range slices.Backward(records) {
		if record.Cleanup == nil {
			continue
		}
		if err := record.Cleanup(ctx); err != nil {
			failure := fmt.Errorf("cleanup %s: %w", record.describe(), err)
			failures = append(failures, failure)
			tb.Logf("e2e cleanup failed: %v", failure)
		}
		if ctx.Err() != nil {
			failures = append(failures, ctx.Err())
			tb.Logf("e2e cleanup stopped: %v", ctx.Err())
			break
		}
	}
	return failures
}
