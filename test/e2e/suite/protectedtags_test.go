//go:build e2e


package suite

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/tags"
)

// TestMeta_ProtectedTags exercises protected tag CRUD via the gitlab_tag meta-tool.
func TestMeta_ProtectedTags(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	proj := createProjectMeta(ctx, t, sess.meta)

	// Create a tag to protect.
	commitFileMeta(ctx, t, sess.meta, proj, "main", "ptag.txt", "content", "add file for protected tag test")
	tagName := "e2e-protected-tag"
	err := callToolVoidOn(ctx, sess.meta, "gitlab_tag", map[string]any{
		"action": "create",
		"params": map[string]any{
			"project_id": proj.pidStr(),
			"tag_name":   tagName,
			"ref":        "main",
		},
	})
	requireNoError(t, err, "create tag for protection")

	t.Run("Meta/ProtectedTag/Protect", func(t *testing.T) {
		out, err := callToolOn[tags.ProtectedTagOutput](ctx, sess.meta, "gitlab_tag", map[string]any{
			"action": "protect",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"tag_name":   tagName,
			},
		})
		requireNoError(t, err, "protect tag")
		requireTrue(t, out.Name == tagName, "expected tag name %q, got %q", tagName, out.Name)
		t.Logf("Protected tag %q", out.Name)
	})

	t.Run("Meta/ProtectedTag/ListProtected", func(t *testing.T) {
		out, err := callToolOn[tags.ListProtectedTagsOutput](ctx, sess.meta, "gitlab_tag", map[string]any{
			"action": "list_protected",
			"params": map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		requireNoError(t, err, "list protected tags")
		requireTrue(t, len(out.Tags) >= 1, "expected at least 1 protected tag")
		t.Logf("Listed %d protected tag(s)", len(out.Tags))
	})

	t.Run("Meta/ProtectedTag/GetProtected", func(t *testing.T) {
		out, err := callToolOn[tags.ProtectedTagOutput](ctx, sess.meta, "gitlab_tag", map[string]any{
			"action": "get_protected",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"tag_name":   tagName,
			},
		})
		requireNoError(t, err, "get protected tag")
		requireTrue(t, out.Name == tagName, "expected tag name %q, got %q", tagName, out.Name)
		t.Logf("Got protected tag %q", out.Name)
	})

	t.Run("Meta/ProtectedTag/Unprotect", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_tag", map[string]any{
			"action": "unprotect",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"tag_name":   tagName,
			},
		})
		requireNoError(t, err, "unprotect tag")
		t.Logf("Unprotected tag %q", tagName)
	})
}
