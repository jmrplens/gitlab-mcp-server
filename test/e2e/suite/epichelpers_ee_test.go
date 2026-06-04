//go:build e2e && enterprise

// epichelpers_ee_test.go provides a shared helper for creating an
// Epic on a test group. Epic tests (epicdiscussions, epicnotes,
// epicissues) all need a real epic IID to operate on; the helper
// creates one via the meta-tool and registers the cleanup in the
// per-test ledger so the next batch of test runs starts clean.
//
// CANNOT parallelize: shares the meta session with the rest of the
// EE suite.
package suite

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/epics"
)

// createEpicInGroup creates a real Epic in the given group via the
// gitlab_epic meta-tool and returns its IID. The cleanup is registered
// with the ledger so the epic is deleted at the end of the run.
func createEpicInGroup(ctx context.Context, t *testing.T, e2e *E2EContext, groupFullPath, titlePrefix string) int64 {
	t.Helper()
	out, err := callToolOn[epics.Output](ctx, sess.meta, "gitlab_epic", map[string]any{
		"action": "epic_create",
		"params": map[string]any{
			"full_path": groupFullPath,
			"title":     uniqueName(titlePrefix),
		},
	})
	requireNoError(t, err, "epic_create for helper")
	requireTruef(t, out.IID > 0, "expected epic IID > 0")
	requireNoError(t, e2e.Ledger.Register(ResourceRecord{
		Kind:      ResourceKindEpic, // epics live in a group namespace
		ID:        strconv.FormatInt(int64(out.IID), 10),
		Path:      groupFullPath,
		Name:      titlePrefix,
		OwnerTest: e2e.Name,
		RunID:     e2e.RunID,
		CreatedAt: time.Now(),
		Cleanup: func(cleanupCtx context.Context) error {
			return callToolVoidOn(cleanupCtx, sess.meta, "gitlab_epic", map[string]any{
				"action": "epic_delete",
				"params": map[string]any{
					"full_path": groupFullPath,
					"iid":       out.IID,
				},
			})
		},
	}), "register epic cleanup")
	t.Logf("Created epic IID=%d in %s", out.IID, groupFullPath)
	return int64(out.IID)
}
