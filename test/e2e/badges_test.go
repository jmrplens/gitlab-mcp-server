//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/badges"
)

func TestIndividual_Badges(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)

	var badgeID int64

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[badges.AddProjectOutput](ctx, sess.individual, "gitlab_add_project_badge", badges.AddProjectInput{
			ProjectID: proj.pidOf(),
			LinkURL:   "https://example.com/badge",
			ImageURL:  "https://img.shields.io/badge/test-passing-green",
		})
		requireNoError(t, err, "create badge")
		requireTrue(t, out.Badge.ID > 0, "expected badge ID")
		badgeID = out.Badge.ID
		t.Logf("Created badge %d", badgeID)
	})

	t.Run("Get", func(t *testing.T) {
		requireTrue(t, badgeID > 0, "badgeID not set")
		out, err := callToolOn[badges.GetProjectOutput](ctx, sess.individual, "gitlab_get_project_badge", badges.GetProjectInput{
			ProjectID: proj.pidOf(),
			BadgeID:   badgeID,
		})
		requireNoError(t, err, "get badge")
		requireTrue(t, out.Badge.ID == badgeID, "expected ID %d, got %d", badgeID, out.Badge.ID)
		t.Logf("Got badge %d", out.Badge.ID)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[badges.ListProjectOutput](ctx, sess.individual, "gitlab_list_project_badges", badges.ListProjectInput{
			ProjectID: proj.pidOf(),
		})
		requireNoError(t, err, "list badges")
		requireTrue(t, len(out.Badges) >= 1, "expected at least 1 badge, got %d", len(out.Badges))
		t.Logf("Listed %d badges", len(out.Badges))
	})

	t.Run("Update", func(t *testing.T) {
		requireTrue(t, badgeID > 0, "badgeID not set")
		out, err := callToolOn[badges.EditProjectOutput](ctx, sess.individual, "gitlab_edit_project_badge", badges.EditProjectInput{
			ProjectID: proj.pidOf(),
			BadgeID:   badgeID,
			LinkURL:   "https://example.com/badge-updated",
		})
		requireNoError(t, err, "update badge")
		requireTrue(t, out.Badge.LinkURL == "https://example.com/badge-updated", "expected updated link URL")
		t.Logf("Updated badge %d", out.Badge.ID)
	})

	t.Run("Delete", func(t *testing.T) {
		requireTrue(t, badgeID > 0, "badgeID not set")
		err := callToolVoidOn(ctx, sess.individual, "gitlab_delete_project_badge", badges.DeleteProjectInput{
			ProjectID: proj.pidOf(),
			BadgeID:   badgeID,
		})
		requireNoError(t, err, "delete badge")
		t.Log("Deleted badge")
	})
}

func TestMeta_Badges(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	var badgeID int64

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[badges.AddProjectOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "badge_add",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"link_url":   "https://example.com/meta-badge",
				"image_url":  "https://img.shields.io/badge/meta-passing-green",
			},
		})
		requireNoError(t, err, "meta create badge")
		requireTrue(t, out.Badge.ID > 0, "expected badge ID")
		badgeID = out.Badge.ID
		t.Logf("Created badge %d via meta-tool", badgeID)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[badges.ListProjectOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "badge_list",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "meta list badges")
		requireTrue(t, len(out.Badges) >= 1, "expected at least 1 badge")
		t.Logf("Listed %d badges via meta-tool", len(out.Badges))
	})

	t.Run("Update", func(t *testing.T) {
		requireTrue(t, badgeID > 0, "badgeID not set")
		out, err := callToolOn[badges.EditProjectOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "badge_edit",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"badge_id":   badgeID,
				"link_url":   "https://example.com/meta-badge-updated",
			},
		})
		requireNoError(t, err, "meta update badge")
		requireTrue(t, out.Badge.LinkURL == "https://example.com/meta-badge-updated", "expected updated link")
		t.Logf("Updated badge %d via meta-tool", out.Badge.ID)
	})

	t.Run("Delete", func(t *testing.T) {
		requireTrue(t, badgeID > 0, "badgeID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "badge_delete",
			"params": map[string]any{"project_id": proj.pidStr(), "badge_id": badgeID},
		})
		requireNoError(t, err, "meta delete badge")
		t.Log("Deleted badge via meta-tool")
	})
}
