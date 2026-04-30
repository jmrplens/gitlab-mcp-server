package suite

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

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
func (ledger *ResourceLedger) Register(record ResourceRecord) {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()

	ledger.records = append(ledger.records, record)
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
	for i := len(records) - 1; i >= 0; i-- {
		record := records[i]
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

func (record ResourceRecord) redactedLabel() string {
	return fmt.Sprintf("kind=%s id=%q path=%q name=%q owner=%q run_id=%q", record.Kind, record.ID, record.Path, record.Name, record.OwnerTest, record.RunID)
}

func TestResourceLedger_CleansInReverseRegistrationOrder(t *testing.T) {
	var ledger ResourceLedger
	var cleaned []string

	ledger.Register(ResourceRecord{Kind: ResourceKindProject, ID: "1", Cleanup: func(context.Context) error {
		cleaned = append(cleaned, "project")
		return nil
	}})
	ledger.Register(ResourceRecord{Kind: ResourceKindGroup, ID: "2", Cleanup: func(context.Context) error {
		cleaned = append(cleaned, "group")
		return nil
	}})

	failures := ledger.CleanupAll(context.Background(), t)
	if len(failures) != 0 {
		t.Fatalf("CleanupAll failures = %v, want none", failures)
	}
	want := []string{"group", "project"}
	if fmt.Sprint(cleaned) != fmt.Sprint(want) {
		t.Fatalf("cleanup order = %v, want %v", cleaned, want)
	}
}

func TestResourceLedger_RecordsReturnsCopy(t *testing.T) {
	var ledger ResourceLedger
	ledger.Register(ResourceRecord{Kind: ResourceKindProject, ID: "1"})

	records := ledger.Records()
	records[0].ID = "changed"

	got := ledger.Records()[0].ID
	if got != "1" {
		t.Fatalf("ledger record ID = %q, want original value", got)
	}
}

func TestResourceLedger_RegisterIsConcurrentSafe(t *testing.T) {
	var ledger ResourceLedger
	var wg sync.WaitGroup

	for i := range 50 {
		id := fmt.Sprintf("%d", i)
		wg.Go(func() {
			ledger.Register(ResourceRecord{Kind: ResourceKindProject, ID: id})
		})
	}
	wg.Wait()

	if got := len(ledger.Records()); got != 50 {
		t.Fatalf("registered records = %d, want 50", got)
	}
}

func TestResourceLedger_CleanupAllReportsFailures(t *testing.T) {
	var ledger ResourceLedger
	ledger.Register(ResourceRecord{Kind: ResourceKindProject, ID: "1", Cleanup: func(context.Context) error {
		return fmt.Errorf("delete failed")
	}})

	failures := ledger.CleanupAll(context.Background(), t)
	if len(failures) != 1 {
		t.Fatalf("failures = %d, want 1", len(failures))
	}
	if got := failures[0].Error(); got == "" || !containsAll(got, "kind=project", "id=\"1\"", "delete failed") {
		t.Fatalf("failure message = %q, want redacted resource label and cause", got)
	}
}

func TestResourceLedger_CleanupAllIsIdempotent(t *testing.T) {
	var ledger ResourceLedger
	var calls int
	ledger.Register(ResourceRecord{Kind: ResourceKindProject, ID: "1", Cleanup: func(context.Context) error {
		calls++
		return nil
	}})

	ledger.CleanupAll(context.Background(), t)
	ledger.CleanupAll(context.Background(), t)

	if calls != 1 {
		t.Fatalf("cleanup calls = %d, want 1", calls)
	}
}

func containsAll(value string, fragments ...string) bool {
	for _, fragment := range fragments {
		if !strings.Contains(value, fragment) {
			return false
		}
	}
	return true
}
