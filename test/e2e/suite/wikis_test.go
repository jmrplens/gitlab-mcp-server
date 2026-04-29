//go:build e2e

// wikis_test.go tests the GitLab wiki page MCP tools against a live GitLab
// instance. Covers the full CRUD lifecycle (create → get → list → update →
// delete) via both individual tools and the gitlab_wiki meta-tool.
package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/wikis"
)

// TestIndividual_Wikis exercises wiki page CRUD using individual MCP tools.
func TestIndividual_Wikis(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)

	var wikiSlug string

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[wikis.Output](ctx, sess.individual, "gitlab_wiki_create", wikis.CreateInput{
			ProjectID: proj.pidOf(),
			Title:     "E2E Test Page",
			Content:   "This is an E2E wiki page.",
		})
		requireNoError(t, err, "wiki create")
		requireTruef(t, out.Slug != "", "wiki slug should not be empty")
		wikiSlug = out.Slug
		t.Logf("Created wiki page: %s (slug=%s)", out.Title, out.Slug)
	})

	t.Run("Get", func(t *testing.T) {
		requireTruef(t, wikiSlug != "", "wikiSlug not set")
		out, err := callToolOn[wikis.Output](ctx, sess.individual, "gitlab_wiki_get", wikis.GetInput{
			ProjectID: proj.pidOf(),
			Slug:      wikiSlug,
		})
		requireNoError(t, err, "wiki get")
		requireTruef(t, out.Slug == wikiSlug, "expected slug %q, got %q", wikiSlug, out.Slug)
		t.Logf("Got wiki page: %s", out.Title)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[wikis.ListOutput](ctx, sess.individual, "gitlab_wiki_list", wikis.ListInput{
			ProjectID: proj.pidOf(),
		})
		requireNoError(t, err, "wiki list")
		requireTruef(t, len(out.WikiPages) >= 1, "expected at least 1 wiki page, got %d", len(out.WikiPages))
		t.Logf("Listed %d wiki pages", len(out.WikiPages))
	})

	t.Run("Update", func(t *testing.T) {
		requireTruef(t, wikiSlug != "", "wikiSlug not set")
		out, err := callToolOn[wikis.Output](ctx, sess.individual, "gitlab_wiki_update", wikis.UpdateInput{
			ProjectID: proj.pidOf(),
			Slug:      wikiSlug,
			Content:   "Updated E2E wiki content.",
		})
		requireNoError(t, err, "wiki update")
		t.Logf("Updated wiki page: %s", out.Title)
	})

	t.Run("Delete", func(t *testing.T) {
		requireTruef(t, wikiSlug != "", "wikiSlug not set")
		err := callToolVoidOn(ctx, sess.individual, "gitlab_wiki_delete", wikis.DeleteInput{
			ProjectID: proj.pidOf(),
			Slug:      wikiSlug,
		})
		requireNoError(t, err, "wiki delete")
		t.Log("Deleted wiki page")
	})
}

// TestMeta_Wikis exercises wiki page CRUD using the gitlab_wiki meta-tool.
func TestMeta_Wikis(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	var wikiSlug string

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[wikis.Output](ctx, sess.meta, "gitlab_wiki", map[string]any{
			"action": "create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"title":      "E2E Meta Wiki",
				"content":    "# Meta wiki\nCreated by E2E meta-tool test.",
			},
		})
		requireNoError(t, err, "meta wiki create")
		requireTruef(t, out.Slug != "", "expected non-empty wiki slug")
		wikiSlug = out.Slug
		t.Logf("Created wiki page: %s", out.Slug)
	})

	t.Run("Get", func(t *testing.T) {
		requireTruef(t, wikiSlug != "", "wikiSlug not set")
		out, err := callToolOn[wikis.Output](ctx, sess.meta, "gitlab_wiki", map[string]any{
			"action": "get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"slug":       wikiSlug,
			},
		})
		requireNoError(t, err, "meta wiki get")
		requireTruef(t, out.Slug == wikiSlug, "expected slug %q, got %q", wikiSlug, out.Slug)
		t.Logf("Got wiki page: %s", out.Title)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[wikis.ListOutput](ctx, sess.meta, "gitlab_wiki", map[string]any{
			"action": "list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		requireNoError(t, err, "meta wiki list")
		requireTruef(t, len(out.WikiPages) > 0, "expected at least one wiki page")
		t.Logf("Listed %d wiki pages", len(out.WikiPages))
	})

	t.Run("Update", func(t *testing.T) {
		requireTruef(t, wikiSlug != "", "wikiSlug not set")
		out, err := callToolOn[wikis.Output](ctx, sess.meta, "gitlab_wiki", map[string]any{
			"action": "update",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"slug":       wikiSlug,
				"content":    "# Updated Meta Wiki\nUpdated by E2E meta-tool test.",
			},
		})
		requireNoError(t, err, "meta wiki update")
		requireTruef(t, out.Slug == wikiSlug, "slug mismatch after update")
		t.Logf("Updated wiki page: %s", out.Slug)
	})

	t.Run("Delete", func(t *testing.T) {
		requireTruef(t, wikiSlug != "", "wikiSlug not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_wiki", map[string]any{
			"action": "delete",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"slug":       wikiSlug,
			},
		})
		requireNoError(t, err, "meta wiki delete")
		t.Logf("Deleted wiki page: %s", wikiSlug)
	})
}
