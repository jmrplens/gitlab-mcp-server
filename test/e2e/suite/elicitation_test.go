//go:build e2e

package suite

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/elicitationtools"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issues"
)

// TestElicitation exercises the interactive elicitation tools via the
// elicitation-enabled session (auto-accept mock handler).
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
