//go:build e2e

package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/releaselinks"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/releases"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/tags"
)

// TestMeta_ReleaseLinksExtended exercises release link actions not covered by releases_test.go:
// link_get, link_update, link_create_batch.
func TestMeta_ReleaseLinksExtended(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	commitFileMeta(ctx, t, sess.meta, proj, "main", "rel-link.txt", "content", "init for release links")

	// Create tag + release
	tagName := uniqueName("rel-link-tag")
	_, setupErr := callToolOn[tags.Output](ctx, sess.meta, "gitlab_tag", map[string]any{
		"action": "create",
		"params": map[string]any{"project_id": proj.pidStr(), "tag_name": tagName, "ref": "main"},
	})
	requireNoError(t, setupErr, "create tag")

	_, setupErr = callToolOn[releases.Output](ctx, sess.meta, "gitlab_release", map[string]any{
		"action": "create",
		"params": map[string]any{
			"project_id":  proj.pidStr(),
			"tag_name":    tagName,
			"name":        "Release Link Test",
			"description": "For link extended tests",
		},
	})
	requireNoError(t, setupErr, "create release")
	defer func() {
		_ = callToolVoidOn(ctx, sess.meta, "gitlab_release", map[string]any{
			"action": "delete",
			"params": map[string]any{"project_id": proj.pidStr(), "tag_name": tagName},
		})
	}()

	// Create a link for get/update testing
	linkOut, setupErr := callToolOn[releaselinks.Output](ctx, sess.meta, "gitlab_release", map[string]any{
		"action": "link_create",
		"params": map[string]any{
			"project_id": proj.pidStr(),
			"tag_name":   tagName,
			"name":       "E2E Link",
			"url":        "https://example.com/artifact.zip",
			"link_type":  "other",
		},
	})
	requireNoError(t, setupErr, "link_create")
	linkID := linkOut.ID
	defer func() {
		_ = callToolVoidOn(ctx, sess.meta, "gitlab_release", map[string]any{
			"action": "link_delete",
			"params": map[string]any{"project_id": proj.pidStr(), "tag_name": tagName, "link_id": linkID},
		})
	}()

	t.Run("LinkGet", func(t *testing.T) {
		out, err := callToolOn[releaselinks.Output](ctx, sess.meta, "gitlab_release", map[string]any{
			"action": "link_get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"tag_name":   tagName,
				"link_id":    linkID,
			},
		})
		requireNoError(t, err, "link_get")
		requireTrue(t, out.ID == linkID, "link_get: ID mismatch")
		requireTrue(t, out.Name == "E2E Link", "link_get: name mismatch")
		t.Logf("Got link %d: %s", out.ID, out.Name)
	})

	t.Run("LinkUpdate", func(t *testing.T) {
		out, err := callToolOn[releaselinks.Output](ctx, sess.meta, "gitlab_release", map[string]any{
			"action": "link_update",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"tag_name":   tagName,
				"link_id":    linkID,
				"name":       "E2E Link Updated",
			},
		})
		requireNoError(t, err, "link_update")
		requireTrue(t, out.Name == "E2E Link Updated", "link_update: name not updated")
		t.Logf("Updated link %d: %s", out.ID, out.Name)
	})

	t.Run("LinkCreateBatch", func(t *testing.T) {
		out, err := callToolOn[releaselinks.CreateBatchOutput](ctx, sess.meta, "gitlab_release", map[string]any{
			"action": "link_create_batch",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"tag_name":   tagName,
				"links": []map[string]any{
					{"name": "Batch Link 1", "url": "https://example.com/batch1.zip"},
					{"name": "Batch Link 2", "url": "https://example.com/batch2.zip"},
				},
			},
		})
		requireNoError(t, err, "link_create_batch")
		requireTrue(t, len(out.Created) == 2, "link_create_batch: expected 2 created links, got %d", len(out.Created))
		t.Logf("Batch created %d links", len(out.Created))
	})
}
