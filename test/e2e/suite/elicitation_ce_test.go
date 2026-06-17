//go:build e2e && !enterprise

// elicitation_ce_test.go tests the MCP elicitation capability against a live GitLab instance.
// Uses the elicitation-enabled session with an auto-accept mock handler.
//
// Build tag: e2e && !enterprise.
package suite

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/elicitationtools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/issues"
)

// TestElicitation exercises the interactive elicitation tools via the
// elicitation-enabled MCP session with an auto-accept mock handler.
//
// The test creates a project fixture on the elicitation session, commits
// a bootstrap file, then drives gitlab_interactive_issue_create. The
// server is expected to elicit a confirmation; the mock handler auto-
// accepts with plausible field values and the test asserts the resulting
// issue IID is positive.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: elicitation.
func TestElicitation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	proj := createProject(ctx, t, sess.elicitation)
	commitFile(ctx, t, sess.elicitation, proj, "main", "init.txt", "bootstrap", "init commit")

	t.Run("InteractiveIssueCreate", func(t *testing.T) {
		out, err := callToolOn[issues.Output](ctx, sess.elicitation, "gitlab_interactive_issue_create", elicitationtools.IssueInput{
			ProjectID: proj.pidOf(),
		})
		if err != nil {
			t.Fatalf("interactive issue create: %v", err)
		}
		if out.IID <= 0 {
			t.Fatalf("expected positive issue IID, got %d", out.IID)
		}
		if out.Title != "E2E elicitation test" {
			t.Fatalf("expected elicited title, got %q", out.Title)
		}
		t.Logf("Created issue via elicitation: IID=%d, title=%q", out.IID, out.Title)
	})
}
