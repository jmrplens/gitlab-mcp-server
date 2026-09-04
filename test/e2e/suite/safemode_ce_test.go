//go:build e2e && !enterprise

// safemode_ce_test.go verifies that Safe Mode intercepts mutating tools and
// returns a structured preview instead of executing. It also verifies that
// read-only tools still function normally through the safe-mode session.
package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/issues"
)

// TestSafeMode exercises the GITLAB_SAFE_MODE feature through the safe-mode
// session against a live GitLab CE instance.
//
// The test creates a real project via the individual session (not the
// safe-mode session, so the creation actually happens), then walks
// mutating and read-only calls on the safe-mode session. Mutating calls
// must return a SafeModePreview describing the would-be operation without
// executing it; read-only calls must pass through and return the
// expected payload.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: safe-mode.
func TestSafeMode(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Create a project using the individual session (not safe-mode).
	proj := createProject(ctx, t, sess.individual)

	t.Run("MutatingToolReturnsPreview", func(t *testing.T) {
		// The preview is an error result on purpose: the tool did not run,
		// and a result that read as success had models reporting the write
		// as done. So it is read from the result's text rather than through
		// callToolOn, which treats IsError as the failure it usually is, and
		// the flag itself is asserted: a preview served as a success would
		// carry the same text and be the very defect this guards against.
		text, isError := modeCall(ctx, t, sess.safeMode, "gitlab_issue_create", map[string]any{
			"project_id":  proj.pidOf(),
			"title":       "Safe mode test issue",
			"description": "This issue should NOT be created",
		})
		requireTruef(t, isError, "the safe-mode preview was returned as a successful result: %s", text)
		preview, isPreview := modeSafePreview(text)
		requireTruef(t, isPreview, "gitlab_issue_create in safe mode did not return a preview: %s", text)
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
