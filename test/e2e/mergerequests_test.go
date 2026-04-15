//go:build e2e

// mergerequests_test.go — E2E tests for merge request CRUD domain.
package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mergerequests"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// setupMRProject creates a project with a feature branch that has
// a commit diverging from main, suitable for creating MRs.
func setupMRProject(ctx context.Context, t *testing.T, session *mcp.ClientSession) (ProjectFixture, string) {
	t.Helper()
	proj := createProject(ctx, t, session)
	unprotectMain(ctx, t, proj)

	const branch = "feature/mr-e2e"
	createBranch(ctx, t, session, proj, branch)
	commitFile(ctx, t, session, proj, branch, "mr-test.txt", "MR test content", "MR feature commit")

	return proj, branch
}

// setupMRProjectMeta is the meta-tool equivalent of setupMRProject.
func setupMRProjectMeta(ctx context.Context, t *testing.T, session *mcp.ClientSession) (ProjectFixture, string) {
	t.Helper()
	proj := createProjectMeta(ctx, t, session)
	unprotectMain(ctx, t, proj)

	const branch = "feature/mr-e2e-meta"
	createBranchMeta(ctx, t, session, proj, branch)
	commitFileMeta(ctx, t, session, proj, branch, "mr-test-meta.txt", "MR meta test", "MR meta commit")

	return proj, branch
}

func TestIndividual_MergeRequests(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	proj, branch := setupMRProject(ctx, t, sess.individual)

	var mrIID int64

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[mergerequests.Output](ctx, sess.individual, "gitlab_mr_create", mergerequests.CreateInput{
			ProjectID:    proj.pidOf(),
			SourceBranch: branch,
			TargetBranch: defaultBranch,
			Title:        "E2E MR individual",
		})
		requireNoError(t, err, "create MR")
		requireTrue(t, out.IID > 0, "expected MR IID > 0, got %d", out.IID)
		mrIID = out.IID
		t.Logf("Created MR !%d", mrIID)
	})

	t.Run("Get", func(t *testing.T) {
		out, err := callToolOn[mergerequests.Output](ctx, sess.individual, "gitlab_mr_get", mergerequests.GetInput{
			ProjectID: proj.pidOf(),
			MRIID:     mrIID,
		})
		requireNoError(t, err, "get MR")
		requireTrue(t, out.Title == "E2E MR individual", "expected title %q, got %q", "E2E MR individual", out.Title)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[mergerequests.ListOutput](ctx, sess.individual, "gitlab_mr_list", mergerequests.ListInput{
			ProjectID: proj.pidOf(),
			State:     "opened",
		})
		requireNoError(t, err, "list MRs")
		requireTrue(t, len(out.MergeRequests) >= 1, "expected >=1 MR, got %d", len(out.MergeRequests))
	})

	t.Run("Update", func(t *testing.T) {
		out, err := callToolOn[mergerequests.Output](ctx, sess.individual, "gitlab_mr_update", mergerequests.UpdateInput{
			ProjectID: proj.pidOf(),
			MRIID:     mrIID,
			Title:     "E2E MR updated",
		})
		requireNoError(t, err, "update MR")
		requireTrue(t, out.Title == "E2E MR updated", "expected updated title, got %q", out.Title)
	})

	t.Run("Commits", func(t *testing.T) {
		var out mergerequests.CommitsOutput
		var err error
		for i := range 10 {
			time.Sleep(2 * time.Second)
			out, err = callToolOn[mergerequests.CommitsOutput](ctx, sess.individual, "gitlab_mr_commits", mergerequests.CommitsInput{
				ProjectID: proj.pidOf(),
				MRIID:     mrIID,
			})
			if err == nil && len(out.Commits) >= 1 {
				break
			}
			t.Logf("MR commits attempt %d: count=%d err=%v", i+1, len(out.Commits), err)
		}
		requireNoError(t, err, "MR commits")
		requireTrue(t, len(out.Commits) >= 1, "expected >=1 commit, got %d", len(out.Commits))
	})

	t.Run("Participants", func(t *testing.T) {
		out, err := callToolOn[mergerequests.ParticipantsOutput](ctx, sess.individual, "gitlab_mr_participants", mergerequests.ParticipantsInput{
			ProjectID: proj.pidOf(),
			MRIID:     mrIID,
		})
		requireNoError(t, err, "MR participants")
		requireTrue(t, len(out.Participants) >= 1, "expected >=1 participant, got %d", len(out.Participants))
	})

	t.Run("Delete", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.individual, "gitlab_mr_delete", mergerequests.DeleteInput{
			ProjectID: proj.pidOf(),
			MRIID:     mrIID,
		})
		requireNoError(t, err, "delete MR")
	})
}

func TestMeta_MergeRequests(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	proj, branch := setupMRProjectMeta(ctx, t, sess.meta)

	var mrIID int64

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[mergerequests.Output](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "create",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"source_branch": branch,
				"target_branch": defaultBranch,
				"title":         "E2E MR meta",
			},
		})
		requireNoError(t, err, "create MR meta")
		requireTrue(t, out.IID > 0, "expected MR IID > 0, got %d", out.IID)
		mrIID = out.IID
		t.Logf("Created MR (meta) !%d", mrIID)
	})

	t.Run("Get", func(t *testing.T) {
		out, err := callToolOn[mergerequests.Output](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"mr_iid":     mrIID,
			},
		})
		requireNoError(t, err, "get MR meta")
		requireTrue(t, out.Title == "E2E MR meta", "expected title %q, got %q", "E2E MR meta", out.Title)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[mergerequests.ListOutput](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"state":      "opened",
			},
		})
		requireNoError(t, err, "list MRs meta")
		requireTrue(t, len(out.MergeRequests) >= 1, "expected >=1 MR, got %d", len(out.MergeRequests))
	})

	t.Run("Update", func(t *testing.T) {
		out, err := callToolOn[mergerequests.Output](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "update",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"mr_iid":     mrIID,
				"title":      "E2E MR meta updated",
			},
		})
		requireNoError(t, err, "update MR meta")
		requireTrue(t, out.Title == "E2E MR meta updated", "expected updated title, got %q", out.Title)
	})

	t.Run("Commits", func(t *testing.T) {
		var out mergerequests.CommitsOutput
		var err error
		for i := range 10 {
			time.Sleep(2 * time.Second)
			out, err = callToolOn[mergerequests.CommitsOutput](ctx, sess.meta, "gitlab_merge_request", map[string]any{
				"action": "commits",
				"params": map[string]any{
					"project_id": proj.pidStr(),
					"mr_iid":     mrIID,
				},
			})
			if err == nil && len(out.Commits) >= 1 {
				break
			}
			t.Logf("MR commits meta attempt %d: count=%d err=%v", i+1, len(out.Commits), err)
		}
		requireNoError(t, err, "MR commits meta")
		requireTrue(t, len(out.Commits) >= 1, "expected >=1 commit, got %d", len(out.Commits))
	})

	t.Run("Participants", func(t *testing.T) {
		out, err := callToolOn[mergerequests.ParticipantsOutput](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "participants",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"mr_iid":     mrIID,
			},
		})
		requireNoError(t, err, "MR participants meta")
		requireTrue(t, len(out.Participants) >= 1, "expected >=1 participant, got %d", len(out.Participants))
	})

	t.Run("Delete", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "delete",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"mr_iid":     mrIID,
			},
		})
		requireNoError(t, err, "delete MR meta")
	})
}
