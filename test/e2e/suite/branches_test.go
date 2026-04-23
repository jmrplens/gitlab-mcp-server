//go:build e2e

// branches_test.go contains self-contained E2E tests for the branches domain.
// Each top-level test function creates its own project fixture and runs all
// subtests independently of any other domain test file.
package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/branches"
)

// TestIndividual_Branches exercises the branch lifecycle using individual
// MCP tools (gitlab_branch_create, gitlab_branch_get, etc.).
func TestIndividual_Branches(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)
	unprotectMain(ctx, t, proj)

	const featureBranch = "feature/branches-e2e"

	// Create
	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[branches.Output](ctx, sess.individual, "gitlab_branch_create", branches.CreateInput{
			ProjectID:  proj.pidOf(),
			BranchName: featureBranch,
			Ref:        defaultBranch,
		})
		requireNoError(t, err, "create branch")
		requireTrue(t, out.Name == featureBranch, "expected branch %q, got %q", featureBranch, out.Name)
		t.Logf("Created branch %s (commit=%s)", out.Name, out.CommitID)
	})

	// Get
	t.Run("Get", func(t *testing.T) {
		out, err := callToolOn[branches.Output](ctx, sess.individual, "gitlab_branch_get", branches.GetInput{
			ProjectID:  proj.pidOf(),
			BranchName: featureBranch,
		})
		requireNoError(t, err, "get branch")
		requireTrue(t, out.Name == featureBranch, "expected branch %q, got %q", featureBranch, out.Name)
		requireTrue(t, out.CommitID != "", msgCommitIDEmpty)
		t.Logf("Got branch %s (commit=%s)", out.Name, out.CommitID)
	})

	// List
	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[branches.ListOutput](ctx, sess.individual, "gitlab_branch_list", branches.ListInput{
			ProjectID: proj.pidOf(),
		})
		requireNoError(t, err, "list branches")
		requireTrue(t, len(out.Branches) >= 2, "expected at least 2 branches, got %d", len(out.Branches))
		t.Logf("Listed %d branches", len(out.Branches))
	})

	// Protect
	t.Run("Protect", func(t *testing.T) {
		out, err := callToolOn[branches.ProtectedOutput](ctx, sess.individual, "gitlab_branch_protect", branches.ProtectInput{
			ProjectID:        proj.pidOf(),
			BranchName:       featureBranch,
			PushAccessLevel:  40,
			MergeAccessLevel: 30,
		})
		requireNoError(t, err, "protect branch")
		requireTrue(t, out.Name == featureBranch, "expected protected branch %q, got %q", featureBranch, out.Name)
		t.Logf("Protected branch %s (push=%d, merge=%d)", out.Name, out.PushAccessLevel, out.MergeAccessLevel)
	})

	// ListProtected
	t.Run("ListProtected", func(t *testing.T) {
		out, err := callToolOn[branches.ProtectedListOutput](ctx, sess.individual, "gitlab_protected_branches_list", branches.ProtectedListInput{
			ProjectID: proj.pidOf(),
		})
		requireNoError(t, err, "list protected branches")
		requireTrue(t, len(out.Branches) >= 1, "expected at least 1 protected branch, got %d", len(out.Branches))

		found := false
		for _, b := range out.Branches {
			if b.Name == featureBranch {
				found = true
				break
			}
		}
		requireTrue(t, found, "%s not in protected branches list", featureBranch)
		t.Logf("Listed %d protected branches", len(out.Branches))
	})

	// Unprotect
	t.Run("Unprotect", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.individual, "gitlab_branch_unprotect", branches.UnprotectInput{
			ProjectID:  proj.pidOf(),
			BranchName: featureBranch,
		})
		requireNoError(t, err, "unprotect branch")

		// Verify not in protected list anymore (may need a few seconds to propagate).
		var stillProtected bool
		var lastListErr error
		for attempt := range 5 {
			out, listErr := callToolOn[branches.ProtectedListOutput](ctx, sess.individual, "gitlab_protected_branches_list", branches.ProtectedListInput{
				ProjectID: proj.pidOf(),
			})
			if listErr != nil {
				lastListErr = listErr
				t.Logf("unprotect verify: attempt %d/5 — list error (retrying): %v", attempt+1, listErr)
				time.Sleep(time.Duration(attempt+1) * time.Second)
				continue
			}
			lastListErr = nil
			stillProtected = false
			for _, b := range out.Branches {
				if b.Name == featureBranch {
					stillProtected = true
					break
				}
			}
			if !stillProtected {
				break
			}
			t.Logf("unprotect verify: attempt %d/5 — branch still protected, retrying", attempt+1)
			time.Sleep(time.Duration(attempt+1) * time.Second)
		}
		requireNoError(t, lastListErr, "list protected after unprotect")
		if stillProtected {
			t.Fatalf("branch %q still appears in protected list after unprotect", featureBranch)
		}
		t.Log("Unprotected branch (verified)")
	})

	// Delete
	t.Run("Delete", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.individual, "gitlab_branch_delete", branches.DeleteInput{
			ProjectID:  proj.pidOf(),
			BranchName: featureBranch,
		})
		requireNoError(t, err, "delete branch")
		t.Logf("Deleted branch %s", featureBranch)
	})
}

// TestMeta_Branches exercises the branch lifecycle using the gitlab_branch
// meta-tool.
func TestMeta_Branches(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	unprotectMain(ctx, t, proj)

	const featureBranch = "feature/branches-meta-e2e"

	// Create
	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[branches.Output](ctx, sess.meta, "gitlab_branch", map[string]any{
			"action": "create",
			"params": map[string]any{
				"project_id":  proj.pidStr(),
				"branch_name": featureBranch,
				"ref":         defaultBranch,
			},
		})
		requireNoError(t, err, "meta branch create")
		requireTrue(t, out.Name == featureBranch, "expected branch %q, got %q", featureBranch, out.Name)
		t.Logf("Created branch %s", out.Name)
	})

	// Get
	t.Run("Get", func(t *testing.T) {
		out, err := callToolOn[branches.Output](ctx, sess.meta, "gitlab_branch", map[string]any{
			"action": "get",
			"params": map[string]any{
				"project_id":  proj.pidStr(),
				"branch_name": featureBranch,
			},
		})
		requireNoError(t, err, "meta branch get")
		requireTrue(t, out.Name == featureBranch, "expected branch %q, got %q", featureBranch, out.Name)
		requireTrue(t, out.CommitID != "", msgCommitIDEmpty)
		t.Logf("Got branch %s (commit=%s)", out.Name, out.CommitID[:8])
	})

	// List
	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[branches.ListOutput](ctx, sess.meta, "gitlab_branch", map[string]any{
			"action": "list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		requireNoError(t, err, "meta branch list")
		requireTrue(t, len(out.Branches) >= 2, "expected at least 2 branches, got %d", len(out.Branches))
		t.Logf("Listed %d branches", len(out.Branches))
	})

	// Protect
	t.Run("Protect", func(t *testing.T) {
		out, err := callToolOn[branches.ProtectedOutput](ctx, sess.meta, "gitlab_branch", map[string]any{
			"action": "protect",
			"params": map[string]any{
				"project_id":         proj.pidStr(),
				"branch_name":        featureBranch,
				"push_access_level":  40,
				"merge_access_level": 30,
			},
		})
		requireNoError(t, err, "meta branch protect")
		requireTrue(t, out.Name == featureBranch, "expected protected branch %q, got %q", featureBranch, out.Name)
		t.Logf("Protected branch %s", out.Name)
	})

	// GetProtected
	t.Run("GetProtected", func(t *testing.T) {
		out, err := callToolOn[branches.ProtectedOutput](ctx, sess.meta, "gitlab_branch", map[string]any{
			"action": "get_protected",
			"params": map[string]any{
				"project_id":  proj.pidStr(),
				"branch_name": featureBranch,
			},
		})
		requireNoError(t, err, "meta branch get_protected")
		requireTrue(t, out.Name == featureBranch, "expected protected branch %q, got %q", featureBranch, out.Name)
		t.Logf("Got protected branch %s (allow_force_push=%v)", out.Name, out.AllowForcePush)
	})

	// UpdateProtected
	t.Run("UpdateProtected", func(t *testing.T) {
		out, err := callToolOn[branches.ProtectedOutput](ctx, sess.meta, "gitlab_branch", map[string]any{
			"action": "update_protected",
			"params": map[string]any{
				"project_id":       proj.pidStr(),
				"branch_name":      featureBranch,
				"allow_force_push": false,
			},
		})
		requireNoError(t, err, "meta branch update_protected")
		requireTrue(t, out.Name == featureBranch, "expected protected branch %q, got %q", featureBranch, out.Name)
		t.Logf("Updated protected branch %s", out.Name)
	})

	// ListProtected
	t.Run("ListProtected", func(t *testing.T) {
		out, err := callToolOn[branches.ProtectedListOutput](ctx, sess.meta, "gitlab_branch", map[string]any{
			"action": "list_protected",
			"params": map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		requireNoError(t, err, "meta list protected branches")
		requireTrue(t, len(out.Branches) >= 1, "expected at least 1 protected branch")

		found := false
		for _, b := range out.Branches {
			if b.Name == featureBranch {
				found = true
				break
			}
		}
		requireTrue(t, found, "%s not in protected branches", featureBranch)
		t.Logf("Listed %d protected branches", len(out.Branches))
	})

	// Unprotect
	t.Run("Unprotect", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_branch", map[string]any{
			"action": "unprotect",
			"params": map[string]any{
				"project_id":  proj.pidStr(),
				"branch_name": featureBranch,
			},
		})
		requireNoError(t, err, "meta branch unprotect")

		// Verify not in protected list anymore (may need a few seconds to propagate).
		var stillProtected bool
		var lastListErr error
		for attempt := range 5 {
			out, listErr := callToolOn[branches.ProtectedListOutput](ctx, sess.meta, "gitlab_branch", map[string]any{
				"action": "list_protected",
				"params": map[string]any{
					"project_id": proj.pidStr(),
				},
			})
			if listErr != nil {
				lastListErr = listErr
				t.Logf("meta unprotect verify: attempt %d/5 — list error (retrying): %v", attempt+1, listErr)
				time.Sleep(time.Duration(attempt+1) * time.Second)
				continue
			}
			lastListErr = nil
			stillProtected = false
			for _, b := range out.Branches {
				if b.Name == featureBranch {
					stillProtected = true
					break
				}
			}
			if !stillProtected {
				break
			}
			t.Logf("meta unprotect verify: attempt %d/5 — branch still protected, retrying", attempt+1)
			time.Sleep(time.Duration(attempt+1) * time.Second)
		}
		requireNoError(t, lastListErr, "meta list protected after unprotect")
		if stillProtected {
			t.Fatalf("branch %q still appears in protected list after unprotect", featureBranch)
		}
		t.Log("Unprotected branch (verified)")
	})

	// DeleteMerged
	t.Run("DeleteMerged", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_branch", map[string]any{
			"action": "delete_merged",
			"params": map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		requireNoError(t, err, "delete_merged")
		t.Log("Deleted merged branches")
	})

	// Delete
	t.Run("Delete", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_branch", map[string]any{
			"action": "delete",
			"params": map[string]any{
				"project_id":  proj.pidStr(),
				"branch_name": featureBranch,
			},
		})
		requireNoError(t, err, "meta branch delete")
		t.Logf("Deleted branch %s", featureBranch)
	})
}
