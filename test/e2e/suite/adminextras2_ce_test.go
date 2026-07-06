//go:build e2e && !enterprise

// adminextras2_ce_test.go covers the GitLab direct-transfer (bulk import)
// action family of the gitlab_admin meta-tool with a self-to-self migration:
// the Docker GitLab instance imports one of its own groups through its
// internal compose-network URL, so no second instance is needed. Covered
// actions: bulk_import_start, bulk_import_list, bulk_import_get,
// bulk_import_entity_list, bulk_import_entity_get,
// bulk_import_entity_failures, and bulk_import_cancel.
//
// Build tag: e2e && !enterprise.
package suite

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/bulkimports"
)

// admBulkSourceURL returns the GitLab URL the instance can use to reach
// itself as a direct-transfer source. The host-facing localhost URL is not
// resolvable from inside the compose network, so the internal service name
// is required.
func admBulkSourceURL() string {
	if fromEnv := os.Getenv("E2E_GITLAB_INTERNAL_URL"); fromEnv != "" {
		return fromEnv
	}
	return defaultE2EGitLabInternalURL
}

// admBulkStartMigration starts one self-to-self direct transfer of the given
// source group into a fresh top-level group and registers deletion of that
// destination group. Cleanup tolerates a missing group because canceled
// migrations usually never create it.
func admBulkStartMigration(ctx context.Context, e2e *E2EContext, sourceFullPath, destinationSlug string) bulkimports.MigrationOutput {
	t := e2e.T
	t.Helper()

	token := os.Getenv("GITLAB_TOKEN")
	requireTruef(t, token != "", "GITLAB_TOKEN is required as the direct-transfer source token")

	out, err := callToolOn[bulkimports.MigrationOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
		"action": "bulk_import_start",
		"params": map[string]any{
			"configuration": map[string]any{
				"url":          admBulkSourceURL(),
				"access_token": token,
			},
			"entities": []map[string]any{{
				"source_type":           "group_entity",
				"source_full_path":      sourceFullPath,
				"destination_slug":      destinationSlug,
				"destination_namespace": "",
			}},
		},
	})
	requireNoError(t, err, "bulk_import_start")
	requireTruef(t, out.ID > 0, "expected bulk import ID > 0")

	requireNoError(t, e2e.Ledger.Register(ResourceRecord{
		Kind:      ResourceKindGroup,
		ID:        destinationSlug,
		Path:      destinationSlug,
		Name:      destinationSlug,
		OwnerTest: e2e.Name,
		RunID:     e2e.RunID,
		CreatedAt: time.Now(),
		Cleanup: func(cleanupCtx context.Context) error {
			_, delErr := sess.glClient.GL().Groups.DeleteGroup(destinationSlug, nil, gl.WithContext(cleanupCtx))
			if delErr != nil && isHTTPStatus(delErr, 404) {
				return nil
			}
			return delErr
		},
	}), "register destination group cleanup")

	return out
}

// admBulkWaitTerminal polls bulk_import_get until the migration reaches a
// terminal status and returns the last observed status. Waiting for the
// terminal state keeps destination-group cleanup deterministic (the group
// cannot appear after the ledger already tried to delete it); if the budget
// runs out, the migration is canceled for the same reason.
func admBulkWaitTerminal(ctx context.Context, t *testing.T, importID int64) string {
	t.Helper()
	lastStatus := "unknown"
	err := Poll(ctx, 3*time.Second, e2eTimeout(180*time.Second, 300*time.Second), func() (bool, string, error) {
		out, getErr := callToolOn[bulkimports.MigrationSummary](ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "bulk_import_get",
			"params": map[string]any{"id": importID},
		})
		if getErr != nil {
			//nolint:nilerr // Transient lookup failures are retried until the poll deadline.
			return false, "bulk_import_get error: " + getErr.Error(), nil
		}
		lastStatus = out.Status
		terminal := out.Status == "finished" || out.Status == "failed" || out.Status == "timeout" || out.Status == "canceled"
		return terminal, "status=" + out.Status, nil
	})
	if err != nil {
		t.Logf("bulk import %d did not reach a terminal status in time (last %q); canceling to keep cleanup deterministic", importID, lastStatus)
		_, _ = callToolOn[bulkimports.MigrationSummary](ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "bulk_import_cancel",
			"params": map[string]any{"id": importID},
		})
	}
	return lastStatus
}

// TestMeta_AdminBulkImports exercises the direct-transfer lifecycle through
// the gitlab_admin meta-tool.
//
// The test creates a small disposable source group, migrates it into a new
// top-level group via a self-to-self direct transfer (source URL is the
// instance's own compose-internal address), waits for the terminal status,
// and asserts the read surface: list, get, entity list, entity get, and
// entity failures. A second migration of the same source group is then
// started and canceled immediately, asserting the canceled status
// round-trips. Both destination groups are registered for deletion.
//
// Build tag: e2e && !enterprise. Mode: CE (Docker only). Surface: meta. Admin token required.
func TestMeta_AdminBulkImports(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	if !isDockerMode() {
		t.Skip("self-to-self direct transfer needs the compose-internal GitLab URL; Docker instance only")
	}
	RunWithCapabilities(t, []Capability{CapabilityAdmin}, func(e2e *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 420*time.Second)
		defer cancel()

		source := CreateGroupMeta(ctx, e2e, sess.meta, "adm-bisrc")
		destinationSlug := uniqueName("adm-bidst")

		started := admBulkStartMigration(ctx, e2e, source.Path, destinationSlug)
		t.Logf("Started bulk import %d (status=%s)", started.ID, started.Status)
		terminalStatus := admBulkWaitTerminal(ctx, t, started.ID)
		// A failed migration still exercises every read action below, so the
		// terminal status is logged rather than asserted; failures would point
		// at fixture drift, not at MCP routing.
		t.Logf("Bulk import %d reached terminal status %q", started.ID, terminalStatus)

		t.Run("List", func(t *testing.T) {
			out, err := callToolOn[bulkimports.ListOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "bulk_import_list",
				"params": map[string]any{},
			})
			requireNoError(t, err, "bulk_import_list")
			requireTruef(t, len(out.Migrations) >= 1, "expected at least 1 migration, got %d", len(out.Migrations))
		})

		t.Run("Get", func(t *testing.T) {
			out, err := callToolOn[bulkimports.MigrationSummary](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "bulk_import_get",
				"params": map[string]any{"id": started.ID},
			})
			requireNoError(t, err, "bulk_import_get")
			requireTruef(t, out.ID == started.ID, "expected import ID %d, got %d", started.ID, out.ID)
			requireTruef(t, out.SourceType == "gitlab", "expected source_type gitlab, got %q", out.SourceType)
		})

		var entityID int64

		t.Run("EntityList", func(t *testing.T) {
			out, err := callToolOn[bulkimports.ListEntitiesOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "bulk_import_entity_list",
				"params": map[string]any{"bulk_import_id": started.ID},
			})
			requireNoError(t, err, "bulk_import_entity_list")
			requireTruef(t, len(out.Entities) == 1, "expected 1 entity, got %d", len(out.Entities))
			requireTruef(t, out.Entities[0].SourceFullPath == source.Path,
				"expected entity source %q, got %q", source.Path, out.Entities[0].SourceFullPath)
			entityID = out.Entities[0].ID
		})

		t.Run("EntityGet", func(t *testing.T) {
			requireTruef(t, entityID > 0, "entityID not set")
			out, err := callToolOn[bulkimports.EntitySummary](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "bulk_import_entity_get",
				"params": map[string]any{
					"bulk_import_id": started.ID,
					"entity_id":      entityID,
				},
			})
			requireNoError(t, err, "bulk_import_entity_get")
			requireTruef(t, out.ID == entityID, "expected entity ID %d, got %d", entityID, out.ID)
			// The list endpoint reports "group_entity" while the detail
			// endpoint shortens it to "group"; both name the same entity type.
			requireTruef(t, out.EntityType == "group_entity" || out.EntityType == "group",
				"expected a group entity type, got %q", out.EntityType)
		})

		t.Run("EntityFailures", func(t *testing.T) {
			requireTruef(t, entityID > 0, "entityID not set")
			out, err := callToolOn[bulkimports.ListEntityFailuresOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "bulk_import_entity_failures",
				"params": map[string]any{
					"bulk_import_id": started.ID,
					"entity_id":      entityID,
				},
			})
			requireNoError(t, err, "bulk_import_entity_failures")
			t.Logf("Entity %s failures: %d", strconv.FormatInt(entityID, 10), len(out.Failures))
		})

		t.Run("Cancel", func(t *testing.T) {
			second := admBulkStartMigration(ctx, e2e, source.Path, uniqueName("adm-bicnl"))
			out, err := callToolOn[bulkimports.MigrationSummary](ctx, sess.meta, "gitlab_admin", map[string]any{
				"action": "bulk_import_cancel",
				"params": map[string]any{"id": second.ID},
			})
			requireNoError(t, err, "bulk_import_cancel")
			requireTruef(t, out.Status == "canceled", "expected status canceled, got %q", out.Status)
			t.Logf("Canceled bulk import %d", second.ID)
		})
	})
}
