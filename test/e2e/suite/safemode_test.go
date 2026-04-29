//go:build e2e

// safemode_test.go verifies that Safe Mode intercepts mutating tools and
// returns a structured preview instead of executing. It also verifies that
// read-only tools still function normally through the safe-mode session.
package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issues"
)

// TestSafeMode exercises the GITLAB_SAFE_MODE feature via the safe-mode
// session. It creates a real project (via the individual session), then
// verifies that mutating calls return a SafeModePreview instead of executing,
// and that read-only calls still work.
func TestSafeMode(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Create a project using the individual session (not safe-mode).
	proj := createProject(ctx, t, sess.individual)

	t.Run("MutatingToolReturnsPreview", func(t *testing.T) {
		preview, err := callToolOn[tools.SafeModePreview](ctx, sess.safeMode, "gitlab_issue_create", issues.CreateInput{
			ProjectID:   proj.pidOf(),
			Title:       "Safe mode test issue",
			Description: "This issue should NOT be created",
		})
		requireNoError(t, err, "call gitlab_issue_create in safe mode")
		requireTruef(t, preview.Status == "blocked", "expected status 'blocked', got %q", preview.Status)
		requireTruef(t, preview.Mode == "safe", "expected mode 'safe', got %q", preview.Mode)
		requireTruef(t, preview.Tool == "gitlab_issue_create", "expected tool 'gitlab_issue_create', got %q", preview.Tool)
		requireTruef(t, len(preview.Params) > 0, "expected non-empty params")
		requireTruef(t, preview.Hint != "", "expected non-empty hint")
	})

	t.Run("NoIssueCreated", func(t *testing.T) {
		// Verify no issue was actually created by listing via the individual session.
		out, err := callToolOn[issues.ListOutput](ctx, sess.individual, "gitlab_issue_list", issues.ListInput{
			ProjectID: proj.pidOf(),
		})
		requireNoError(t, err, "list issues")
		requireTruef(t, len(out.Issues) == 0, "expected 0 issues, got %d", len(out.Issues))
	})

	t.Run("ReadOnlyToolStillWorks", func(t *testing.T) {
		// Read-only tools should pass through Safe Mode unchanged.
		out, err := callToolOn[issues.ListOutput](ctx, sess.safeMode, "gitlab_issue_list", issues.ListInput{
			ProjectID: proj.pidOf(),
		})
		requireNoError(t, err, "list issues via safe-mode session")
		requireTruef(t, len(out.Issues) == 0, "expected 0 issues via safe-mode, got %d", len(out.Issues))
	})
}
