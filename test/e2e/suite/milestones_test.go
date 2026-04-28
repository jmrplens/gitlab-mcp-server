//go:build e2e

// milestones_test.go tests the project milestone MCP tools against a live
// GitLab instance. Covers milestone create, get, update (with close), and
// delete for both individual and meta-tool modes.
package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/milestones"
)

// TestIndividual_Milestones exercises milestone CRUD using individual MCP tools.
func TestIndividual_Milestones(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)

	var milestoneIID int64

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[milestones.Output](ctx, sess.individual, "gitlab_milestone_create", milestones.CreateInput{
			ProjectID:   proj.pidOf(),
			Title:       "e2e-milestone-v1",
			Description: "E2E test milestone",
		})
		requireNoError(t, err, "milestone create")
		requireTruef(t, out.IID > 0, "milestone IID should be positive")
		milestoneIID = out.IID
		t.Logf("Created milestone: %s (IID=%d)", out.Title, out.IID)
	})

	t.Run("Get", func(t *testing.T) {
		requireTruef(t, milestoneIID > 0, "milestoneIID not set")
		out, err := callToolOn[milestones.Output](ctx, sess.individual, "gitlab_milestone_get", milestones.GetInput{
			ProjectID:    proj.pidOf(),
			MilestoneIID: milestoneIID,
		})
		requireNoError(t, err, "milestone get")
		requireTruef(t, out.Title == "e2e-milestone-v1", "expected title e2e-milestone-v1, got %s", out.Title)
		t.Logf("Got milestone: %s (state=%s)", out.Title, out.State)
	})

	t.Run("Update", func(t *testing.T) {
		requireTruef(t, milestoneIID > 0, "milestoneIID not set")
		out, err := callToolOn[milestones.Output](ctx, sess.individual, "gitlab_milestone_update", milestones.UpdateInput{
			ProjectID:    proj.pidOf(),
			MilestoneIID: milestoneIID,
			Description:  "Updated by E2E test",
			StateEvent:   "close",
		})
		requireNoError(t, err, "milestone update")
		requireTruef(t, out.State == "closed", "expected state closed, got %s", out.State)
		t.Logf("Updated milestone: %s (state=%s)", out.Title, out.State)
	})

	t.Run("Delete", func(t *testing.T) {
		requireTruef(t, milestoneIID > 0, "milestoneIID not set")
		err := callToolVoidOn(ctx, sess.individual, "gitlab_milestone_delete", milestones.DeleteInput{
			ProjectID:    proj.pidOf(),
			MilestoneIID: milestoneIID,
		})
		requireNoError(t, err, "milestone delete")
		t.Log("Deleted milestone")
	})
}

// TestMeta_Milestones exercises milestone CRUD using the gitlab_project meta-tool.
func TestMeta_Milestones(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	var milestoneIID int64

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[milestones.Output](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "milestone_create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"title":      "e2e-meta-milestone",
			},
		})
		requireNoError(t, err, "meta milestone create")
		requireTruef(t, out.IID > 0, "expected positive milestone IID")
		milestoneIID = out.IID
		t.Logf("Created milestone: %s (IID=%d)", out.Title, out.IID)
	})

	t.Run("Get", func(t *testing.T) {
		requireTruef(t, milestoneIID > 0, "milestoneIID not set")
		out, err := callToolOn[milestones.Output](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "milestone_get",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"milestone_iid": milestoneIID,
			},
		})
		requireNoError(t, err, "meta milestone get")
		requireTruef(t, out.IID == milestoneIID, "milestone IID mismatch")
		t.Logf("Got milestone: %s (state=%s)", out.Title, out.State)
	})

	t.Run("Update", func(t *testing.T) {
		requireTruef(t, milestoneIID > 0, "milestoneIID not set")
		out, err := callToolOn[milestones.Output](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "milestone_update",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"milestone_iid": milestoneIID,
				"description":   "Updated by E2E meta-tool test",
			},
		})
		requireNoError(t, err, "meta milestone update")
		requireTruef(t, out.IID == milestoneIID, "milestone IID mismatch after update")
		t.Logf("Updated milestone: %s", out.Title)
	})

	t.Run("Delete", func(t *testing.T) {
		requireTruef(t, milestoneIID > 0, "milestoneIID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "milestone_delete",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"milestone_iid": milestoneIID,
			},
		})
		requireNoError(t, err, "meta milestone delete")
		t.Logf("Deleted milestone IID=%d", milestoneIID)
	})
}
