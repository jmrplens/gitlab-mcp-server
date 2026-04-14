//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/files"
)

// TestIndividual_Commits exercises commit operations using individual MCP tools.
func TestIndividual_Commits(t *testing.T) {
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)
	unprotectMain(ctx, t, proj)

	// Commit a file to have content.
	cfix := commitFile(ctx, t, sess.individual, proj, defaultBranch, testFileMainGo,
		"package main\n\nfunc main() {}\n", "add main.go for commit tests")

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[commits.ListOutput](ctx, sess.individual, "gitlab_commit_list", commits.ListInput{
			ProjectID: proj.pidOf(),
			RefName:   defaultBranch,
		})
		requireNoError(t, err, "list commits")
		requireTrue(t, len(out.Commits) >= 1, "expected at least 1 commit, got %d", len(out.Commits))
		t.Logf("Listed %d commits on %s", len(out.Commits), defaultBranch)
	})

	t.Run("Get", func(t *testing.T) {
		out, err := callToolOn[commits.DetailOutput](ctx, sess.individual, "gitlab_commit_get", commits.GetInput{
			ProjectID: proj.pidOf(),
			SHA:       cfix.SHA,
		})
		requireNoError(t, err, "get commit")
		requireTrue(t, out.ID == cfix.SHA, "expected SHA %s, got %s", cfix.SHA, out.ID)
		requireTrue(t, out.Title != "", "commit title should not be empty")
		t.Logf("Got commit %s: %s", out.ShortID, out.Title)
	})

	t.Run("Diff", func(t *testing.T) {
		out, err := callToolOn[commits.DiffOutput](ctx, sess.individual, "gitlab_commit_diff", commits.DiffInput{
			ProjectID: proj.pidOf(),
			SHA:       cfix.SHA,
		})
		requireNoError(t, err, "get commit diff")
		requireTrue(t, len(out.Diffs) >= 1, "expected at least 1 diff, got %d", len(out.Diffs))
		t.Logf("Commit %s has %d file diffs", cfix.SHA[:8], len(out.Diffs))
	})

	t.Run("FileGet", func(t *testing.T) {
		out, err := callToolOn[files.Output](ctx, sess.individual, "gitlab_file_get", files.GetInput{
			ProjectID: proj.pidOf(),
			FilePath:  testFileMainGo,
			Ref:       defaultBranch,
		})
		requireNoError(t, err, "get file")
		requireTrue(t, out.FileName == testFileMainGo, "expected file %s, got %s", testFileMainGo, out.FileName)
		t.Logf("Got file %s (size=%d)", out.FileName, out.Size)
	})
}

// TestMeta_Commits exercises commit operations using the gitlab_repository meta-tool.
func TestMeta_Commits(t *testing.T) {
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	unprotectMain(ctx, t, proj)

	// Commit a file to have content.
	cfix := commitFileMeta(ctx, t, sess.meta, proj, defaultBranch, testFileMainGo,
		"package main\n\nfunc main() {}\n", "add main.go for commit meta tests")

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[commits.ListOutput](ctx, sess.meta, "gitlab_repository", map[string]any{
			"action": "commit_list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"ref_name":   defaultBranch,
			},
		})
		requireNoError(t, err, "meta commit list")
		requireTrue(t, len(out.Commits) >= 1, "expected at least 1 commit")
		t.Logf("Listed %d commits via meta-tool", len(out.Commits))
	})

	t.Run("Get", func(t *testing.T) {
		out, err := callToolOn[commits.DetailOutput](ctx, sess.meta, "gitlab_repository", map[string]any{
			"action": "commit_get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"sha":        cfix.SHA,
			},
		})
		requireNoError(t, err, "meta commit get")
		requireTrue(t, out.ID == cfix.SHA, "expected SHA %s, got %s", cfix.SHA, out.ID)
		requireTrue(t, out.Title != "", "commit title should not be empty")
		t.Logf("Got commit %s via meta-tool: %s", out.ShortID, out.Title)
	})

	t.Run("Diff", func(t *testing.T) {
		out, err := callToolOn[commits.DiffOutput](ctx, sess.meta, "gitlab_repository", map[string]any{
			"action": "commit_diff",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"sha":        cfix.SHA,
			},
		})
		requireNoError(t, err, "meta commit diff")
		requireTrue(t, len(out.Diffs) >= 1, "expected at least 1 diff")
		t.Logf("Commit %s has %d diffs via meta-tool", cfix.SHA[:8], len(out.Diffs))
	})

	t.Run("FileGet", func(t *testing.T) {
		out, err := callToolOn[files.Output](ctx, sess.meta, "gitlab_repository", map[string]any{
			"action": "file_get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"file_path":  testFileMainGo,
				"ref":        defaultBranch,
			},
		})
		requireNoError(t, err, "meta file get")
		requireTrue(t, out.FileName == testFileMainGo, "expected %s, got %s", testFileMainGo, out.FileName)
		t.Logf("Got file %s via meta-tool (size=%d)", out.FileName, out.Size)
	})
}
