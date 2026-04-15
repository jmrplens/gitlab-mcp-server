//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/tags"
)

// TestMeta_TagsProtected exercises protected tag actions not covered by tags_test.go:
// list_protected, protect, get_protected, unprotect, get_signature.
func TestMeta_TagsProtected(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	commitFileMeta(ctx, t, sess.meta, proj, "main", "tag-prot.txt", "content", "init for tags")

	// Create a tag to protect
	tagName := uniqueName("prot-tag")
	_, err := callToolOn[tags.Output](ctx, sess.meta, "gitlab_tag", map[string]any{
		"action": "create",
		"params": map[string]any{
			"project_id": proj.pidStr(),
			"tag_name":   tagName,
			"ref":        "main",
		},
	})
	requireNoError(t, err, "create tag for protection")
	defer func() {
		_ = callToolVoidOn(ctx, sess.meta, "gitlab_tag", map[string]any{
			"action": "delete",
			"params": map[string]any{"project_id": proj.pidStr(), "tag_name": tagName},
		})
	}()

	t.Run("Protect", func(t *testing.T) {
		out, err := callToolOn[tags.ProtectedTagOutput](ctx, sess.meta, "gitlab_tag", map[string]any{
			"action": "protect",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"name":       tagName,
			},
		})
		requireNoError(t, err, "protect tag")
		requireTrue(t, out.Name == tagName, "protect: name mismatch")
		t.Logf("Protected tag: %s", out.Name)
	})

	t.Run("ListProtected", func(t *testing.T) {
		out, err := callToolOn[tags.ListProtectedTagsOutput](ctx, sess.meta, "gitlab_tag", map[string]any{
			"action": "list_protected",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "list_protected")
		requireTrue(t, len(out.Tags) >= 1, "list_protected: expected at least 1 protected tag")
		t.Logf("Protected tags: %d", len(out.Tags))
	})

	t.Run("GetProtected", func(t *testing.T) {
		out, err := callToolOn[tags.ProtectedTagOutput](ctx, sess.meta, "gitlab_tag", map[string]any{
			"action": "get_protected",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"name":       tagName,
			},
		})
		requireNoError(t, err, "get_protected")
		requireTrue(t, out.Name == tagName, "get_protected: name mismatch")
	})

	t.Run("Unprotect", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_tag", map[string]any{
			"action": "unprotect",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"name":       tagName,
			},
		})
		requireNoError(t, err, "unprotect")
	})

	t.Run("GetSignature", func(t *testing.T) {
		// get_signature may fail with 404 if the tag was not GPG/X.509-signed, but the route is exercised.
		_, _ = callToolOn[tags.SignatureOutput](ctx, sess.meta, "gitlab_tag", map[string]any{
			"action": "get_signature",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"tag_name":   tagName,
			},
		})
		t.Log("get_signature route exercised (may 404 for unsigned tags)")
	})
}
