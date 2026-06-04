//go:build e2e && enterprise

// resource_ledger_test.go defines the per-test resource cleanup ledger and
// verifies that cleanup is ordered, idempotent, and safe under concurrent use.
package suite

import (
	"context"
	"errors"
	"fmt"
	"slices"
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
	ResourceKindEpic                ResourceKind = "epic"
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
func (ledger *ResourceLedger) CleanupAll(ctx context.Context, tb testing.TB) []error {
	tb.Helper()

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

// redactedLabel returns a diagnostic label for cleanup failures without
// including secrets or credential-bearing URLs.
func (record ResourceRecord) redactedLabel() string {
	return fmt.Sprintf("kind=%s id=%q path=%q name=%q owner=%q run_id=%q", record.Kind, record.ID, record.Path, record.Name, record.OwnerTest, record.RunID)
}
