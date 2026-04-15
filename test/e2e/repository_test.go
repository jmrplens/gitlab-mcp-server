//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/repository"
)

// TestIndividual_Repository exercises repository tree and compare operations
// using individual MCP tools.
func TestIndividual_Repository(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)
	unprotectMain(ctx, t, proj)
	commitFile(ctx, t, sess.individual, proj, defaultBranch, testFileMainGo,
		"package main\n\nfunc main() {}\n", "add main.go for repo tests")

	// Create a branch with a different commit for compare.
	const branchName = "feature/repo-test"
	br := createBranch(ctx, t, sess.individual, proj, branchName)
	_ = br
	commitFile(ctx, t, sess.individual, proj, branchName, "extra.go",
		"package main\n\nfunc extra() {}\n", "add extra.go on branch")

	t.Run("Tree", func(t *testing.T) {
		out, err := callToolOn[repository.TreeOutput](ctx, sess.individual, "gitlab_repository_tree", repository.TreeInput{
			ProjectID: proj.pidOf(),
			Ref:       defaultBranch,
		})
		requireNoError(t, err, "list repository tree")
		requireTrue(t, len(out.Tree) >= 1, "expected at least 1 tree node, got %d", len(out.Tree))

		found := false
		for _, n := range out.Tree {
			if n.Name == testFileMainGo {
				found = true
				break
			}
		}
		requireTrue(t, found, "main.go not found in repository tree")
		t.Logf("Repository tree has %d entries", len(out.Tree))
	})

	t.Run("Compare", func(t *testing.T) {
		out, err := callToolOn[repository.CompareOutput](ctx, sess.individual, "gitlab_repository_compare", repository.CompareInput{
			ProjectID: proj.pidOf(),
			From:      defaultBranch,
			To:        branchName,
		})
		requireNoError(t, err, "compare repository")
		requireTrue(t, len(out.Commits) >= 1, "expected at least 1 commit, got %d", len(out.Commits))
		requireTrue(t, len(out.Diffs) >= 1, "expected at least 1 diff, got %d", len(out.Diffs))
		t.Logf("Compare %s..%s: %d commits, %d file diffs", defaultBranch, branchName, len(out.Commits), len(out.Diffs))
	})
}

// TestMeta_Repository exercises repository operations using the gitlab_repository meta-tool.
func TestMeta_Repository(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	unprotectMain(ctx, t, proj)
	commitFileMeta(ctx, t, sess.meta, proj, defaultBranch, testFileMainGo,
		"package main\n\nfunc main() {}\n", "add main.go for repo meta tests")

	const branchName = "feature/repo-meta-test"
	createBranchMeta(ctx, t, sess.meta, proj, branchName)
	commitFileMeta(ctx, t, sess.meta, proj, branchName, "extra.go",
		"package main\n\nfunc extra() {}\n", "add extra.go on branch")

	t.Run("Tree", func(t *testing.T) {
		out, err := callToolOn[repository.TreeOutput](ctx, sess.meta, "gitlab_repository", map[string]any{
			"action": "tree",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"ref":        defaultBranch,
			},
		})
		requireNoError(t, err, "meta repository tree")
		requireTrue(t, len(out.Tree) >= 1, "expected at least 1 tree node")

		found := false
		for _, n := range out.Tree {
			if n.Name == testFileMainGo {
				found = true
				break
			}
		}
		requireTrue(t, found, "main.go not found in tree via meta-tool")
		t.Logf("Repository tree has %d entries via meta-tool", len(out.Tree))
	})

	t.Run("Compare", func(t *testing.T) {
		out, err := callToolOn[repository.CompareOutput](ctx, sess.meta, "gitlab_repository", map[string]any{
			"action": "compare",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"from":       defaultBranch,
				"to":         branchName,
			},
		})
		requireNoError(t, err, "meta repository compare")
		requireTrue(t, len(out.Commits) >= 1, "expected at least 1 commit")
		requireTrue(t, len(out.Diffs) >= 1, "expected at least 1 diff")
		t.Logf("Compare %s..%s via meta: %d commits, %d diffs", defaultBranch, branchName, len(out.Commits), len(out.Diffs))
	})
}
