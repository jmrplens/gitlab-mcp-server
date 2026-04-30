// resource_ledger_test.go defines the per-test resource cleanup ledger and
// verifies that cleanup is ordered, idempotent, and safe under concurrent use.
package suite

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

var errResourceLedgerClosed = errors.New("resource ledger closed")

// ResourceKind identifies a GitLab or MCP resource owned by an E2E test.
type ResourceKind string

// ResourceKind values cover the resource families currently created by E2E tests.
const (
	ResourceKindProject             ResourceKind = "project"
	ResourceKindGroup               ResourceKind = "group"
	ResourceKindUser                ResourceKind = "user"
	ResourceKindSSHKey              ResourceKind = "ssh_key"
	ResourceKindDeployKey           ResourceKind = "deploy_key"
	ResourceKindDeployToken         ResourceKind = "deploy_token"
	ResourceKindPersonalAccessToken ResourceKind = "personal_access_token"
	ResourceKindImpersonationToken  ResourceKind = "impersonation_token"
	ResourceKindTopic               ResourceKind = "topic"
	ResourceKindBroadcastMessage    ResourceKind = "broadcast_message"
	ResourceKindSystemHook          ResourceKind = "system_hook"
	ResourceKindApplication         ResourceKind = "application"
	ResourceKindFeatureFlag         ResourceKind = "feature_flag"
	ResourceKindCustomAttribute     ResourceKind = "custom_attribute"
	ResourceKindPipeline            ResourceKind = "pipeline"
	ResourceKindJob                 ResourceKind = "job"
	ResourceKindCurrentUserState    ResourceKind = "current_user_state"
)

// ResourceRecord describes one resource and its best-effort cleanup action.
type ResourceRecord struct {
	Kind      ResourceKind
	ID        string
	Path      string
	Name      string
	OwnerTest string
	RunID     string
	CreatedAt time.Time
	Cleanup   func(context.Context) error
}

// ResourceLedger records resources owned by one test and cleans them up once.
type ResourceLedger struct {
	mu      sync.Mutex
	records []ResourceRecord
	cleaned bool
}

// Register adds a resource cleanup record to the ledger.
func (ledger *ResourceLedger) Register(record ResourceRecord) error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()

	if ledger.cleaned {
		return fmt.Errorf("register %s: %w", record.redactedLabel(), errResourceLedgerClosed)
	}

	ledger.records = append(ledger.records, record)
	return nil
}

// Records returns a snapshot copy of registered resources.
func (ledger *ResourceLedger) Records() []ResourceRecord {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()

	return append([]ResourceRecord(nil), ledger.records...)
}

// CleanupAll runs registered cleanup actions in reverse registration order.
func (ledger *ResourceLedger) CleanupAll(ctx context.Context, t testing.TB) []error {
	t.Helper()

	ledger.mu.Lock()
	if ledger.cleaned {
		ledger.mu.Unlock()
		return nil
	}
	ledger.cleaned = true
	records := append([]ResourceRecord(nil), ledger.records...)
	ledger.mu.Unlock()

	failures := make([]error, 0)
	for _, record := range slices.Backward(records) {
		if record.Cleanup == nil {
			continue
		}
		if err := record.Cleanup(ctx); err != nil {
			failure := fmt.Errorf("cleanup %s: %w", record.redactedLabel(), err)
			failures = append(failures, failure)
			t.Logf("e2e cleanup failed: %v", failure)
		}
		if ctx.Err() != nil {
			failures = append(failures, ctx.Err())
			t.Logf("e2e cleanup stopped: %v", ctx.Err())
			break
		}
	}
	return failures
}

// redactedLabel returns a diagnostic label for cleanup failures without
// including secrets or credential-bearing URLs.
func (record ResourceRecord) redactedLabel() string {
	return fmt.Sprintf("kind=%s id=%q path=%q name=%q owner=%q run_id=%q", record.Kind, record.ID, record.Path, record.Name, record.OwnerTest, record.RunID)
}

// TestResourceLedger_CleansInReverseRegistrationOrder verifies that CleanupAll
// runs cleanup callbacks in last-in-first-out order.
//
// The test registers project and group cleanup callbacks, invokes CleanupAll,
// and asserts that the group cleanup runs before the project cleanup. This
// protects nested fixtures where dependent resources should be removed before
// their parents.
func TestResourceLedger_CleansInReverseRegistrationOrder(t *testing.T) {
	var ledger ResourceLedger
	var cleaned []string

	if err := ledger.Register(ResourceRecord{Kind: ResourceKindProject, ID: "1", Cleanup: func(context.Context) error {
		cleaned = append(cleaned, "project")
		return nil
	}}); err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	if err := ledger.Register(ResourceRecord{Kind: ResourceKindGroup, ID: "2", Cleanup: func(context.Context) error {
		cleaned = append(cleaned, "group")
		return nil
	}}); err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}

	failures := ledger.CleanupAll(context.Background(), t)
	if len(failures) != 0 {
		t.Fatalf("CleanupAll failures = %v, want none", failures)
	}
	want := []string{"group", "project"}
	if fmt.Sprint(cleaned) != fmt.Sprint(want) {
		t.Fatalf("cleanup order = %v, want %v", cleaned, want)
	}
}

// TestResourceLedger_RecordsReturnsCopy verifies that Records returns a copy of
// the internal slice rather than exposing mutable ledger state.
//
// The test mutates the returned record and then reads the ledger again,
// expecting the original ID to remain unchanged.
func TestResourceLedger_RecordsReturnsCopy(t *testing.T) {
	var ledger ResourceLedger
	if err := ledger.Register(ResourceRecord{Kind: ResourceKindProject, ID: "1"}); err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}

	records := ledger.Records()
	records[0].ID = "changed"

	got := ledger.Records()[0].ID
	if got != "1" {
		t.Fatalf("ledger record ID = %q, want original value", got)
	}
}

// TestResourceLedger_RegisterIsConcurrentSafe verifies that concurrent Register
// calls preserve every cleanup record.
//
// The test launches 50 goroutines that register project records and asserts
// that the final snapshot contains all 50 entries.
func TestResourceLedger_RegisterIsConcurrentSafe(t *testing.T) {
	var ledger ResourceLedger
	var wg sync.WaitGroup

	for i := range 50 {
		id := fmt.Sprintf("%d", i)
		wg.Go(func() {
			if err := ledger.Register(ResourceRecord{Kind: ResourceKindProject, ID: id}); err != nil {
				t.Errorf("Register() error = %v, want nil", err)
			}
		})
	}
	wg.Wait()

	if got := len(ledger.Records()); got != 50 {
		t.Fatalf("registered records = %d, want 50", got)
	}
}

// TestResourceLedger_CleanupAllReportsFailures verifies that CleanupAll returns
// cleanup errors with the redacted resource label and original cause.
//
// The test registers a failing cleanup callback and asserts that the returned
// error includes kind, ID, and failure text for actionable diagnostics.
func TestResourceLedger_CleanupAllReportsFailures(t *testing.T) {
	var ledger ResourceLedger
	if err := ledger.Register(ResourceRecord{Kind: ResourceKindProject, ID: "1", Cleanup: func(context.Context) error {
		return fmt.Errorf("delete failed")
	}}); err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}

	failures := ledger.CleanupAll(context.Background(), t)
	if len(failures) != 1 {
		t.Fatalf("failures = %d, want 1", len(failures))
	}
	if got := failures[0].Error(); got == "" || !containsAll(got, "kind=project", "id=\"1\"", "delete failed") {
		t.Fatalf("failure message = %q, want redacted resource label and cause", got)
	}
}

// TestResourceLedger_CleanupAllIsIdempotent verifies that calling CleanupAll
// more than once does not repeat cleanup callbacks.
//
// The test registers one cleanup callback, calls CleanupAll twice, and expects
// the callback counter to increment only once.
func TestResourceLedger_CleanupAllIsIdempotent(t *testing.T) {
	var ledger ResourceLedger
	var calls int
	if err := ledger.Register(ResourceRecord{Kind: ResourceKindProject, ID: "1", Cleanup: func(context.Context) error {
		calls++
		return nil
	}}); err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}

	ledger.CleanupAll(context.Background(), t)
	ledger.CleanupAll(context.Background(), t)

	if calls != 1 {
		t.Fatalf("cleanup calls = %d, want 1", calls)
	}
}

// TestResourceLedger_RegisterAfterCleanupReturnsError verifies that the ledger
// rejects records registered after cleanup has started.
//
// The test runs CleanupAll first, then attempts to add another record and
// expects a closed-ledger error. This prevents late fixture registrations from
// being silently skipped by idempotent cleanup.
func TestResourceLedger_RegisterAfterCleanupReturnsError(t *testing.T) {
	var ledger ResourceLedger

	ledger.CleanupAll(context.Background(), t)
	err := ledger.Register(ResourceRecord{Kind: ResourceKindProject, ID: "late"})
	if !errors.Is(err, errResourceLedgerClosed) {
		t.Fatalf("Register() error = %v, want %v", err, errResourceLedgerClosed)
	}
}

// containsAll reports whether value contains every fragment in order-insensitive
// fashion for compact failure-message assertions.
func containsAll(value string, fragments ...string) bool {
	for _, fragment := range fragments {
		if !strings.Contains(value, fragment) {
			return false
		}
	}
	return true
}
