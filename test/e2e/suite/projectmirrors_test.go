//go:build e2e

package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projectmirrors"
)

// TestMeta_ProjectRemoteMirrors exercises the remote mirror CRUD actions
// (mirror_list, mirror_add, mirror_get, mirror_get_public_key, mirror_edit,
// mirror_force_push, mirror_delete) via the gitlab_project meta-tool.
// Remote mirrors are a Free-tier feature (push mirrors).
func TestMeta_ProjectRemoteMirrors(t *testing.T) {
	t.Parallel()

	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	target := createProjectMeta(ctx, t, sess.meta)
	mirrorURL := remoteMirrorTargetURL(t, target)

	// List — should be empty on a fresh project.
	t.Run("MirrorList_Empty", func(t *testing.T) {
		out, err := callToolOn[projectmirrors.ListOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "mirror_list",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "mirror_list (empty)")
		requireTruef(t, len(out.Mirrors) == 0, "expected 0 mirrors, got %d", len(out.Mirrors))
		t.Logf("Mirrors: %d (expected 0)", len(out.Mirrors))
	})

	// Add a remote push mirror.
	var mirrorID int64
	t.Run("MirrorAdd", func(t *testing.T) {
		out, err := callToolOn[projectmirrors.Output](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "mirror_add",
			"params": map[string]any{
				"project_id":              proj.pidStr(),
				"url":                     mirrorURL,
				"enabled":                 true,
				"auth_method":             "password",
				"only_protected_branches": true,
			},
		})
		requireNoError(t, err, "mirror_add")
		requireTruef(t, out.ID > 0, "mirror_add: expected ID > 0")
		mirrorID = out.ID
		t.Logf("Added mirror %d", mirrorID)
	})

	// Get mirror by ID.
	t.Run("MirrorGet", func(t *testing.T) {
		requireTruef(t, mirrorID > 0, "mirrorID not set")
		out, err := callToolOn[projectmirrors.Output](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "mirror_get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"mirror_id":  mirrorID,
			},
		})
		requireNoError(t, err, "mirror_get")
		requireTruef(t, out.ID == mirrorID, "mirror_get: ID mismatch")
		t.Logf("Got mirror %d, enabled=%v", out.ID, out.Enabled)
	})

	// Password-authenticated mirrors do not expose SSH public keys.
	t.Run("MirrorGetPublicKey_PasswordMirror", func(t *testing.T) {
		requireTruef(t, mirrorID > 0, "mirrorID not set")
		_, err := callToolOn[projectmirrors.PublicKeyOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "mirror_get_public_key",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"mirror_id":  mirrorID,
			},
		})
		requireTruef(t, err != nil, "expected mirror_get_public_key to fail for password-authenticated mirror")
		t.Logf("Expected public key error for password-authenticated mirror: %v", err)
	})

	// Edit mirror.
	t.Run("MirrorEdit", func(t *testing.T) {
		requireTruef(t, mirrorID > 0, "mirrorID not set")
		out, err := callToolOn[projectmirrors.Output](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "mirror_edit",
			"params": map[string]any{
				"project_id":              proj.pidStr(),
				"mirror_id":               mirrorID,
				"only_protected_branches": false,
			},
		})
		requireNoError(t, err, "mirror_edit")
		requireTruef(t, out.ID == mirrorID, "mirror_edit: ID mismatch")
		t.Logf("Edited mirror %d", out.ID)
	})

	// List — should now have one mirror.
	t.Run("MirrorList_One", func(t *testing.T) {
		out, err := callToolOn[projectmirrors.ListOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "mirror_list",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "mirror_list (one)")
		requireTruef(t, len(out.Mirrors) == 1, "expected 1 mirror, got %d", len(out.Mirrors))
		t.Logf("Mirrors: %d (expected 1)", len(out.Mirrors))
	})

	// Delete mirror.
	t.Run("MirrorDelete", func(t *testing.T) {
		requireTruef(t, mirrorID > 0, "mirrorID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "mirror_delete",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"mirror_id":  mirrorID,
			},
		})
		requireNoError(t, err, "mirror_delete")
		t.Logf("Deleted mirror %d", mirrorID)
	})

	// Verify deletion.
	t.Run("MirrorList_AfterDelete", func(t *testing.T) {
		out, err := callToolOn[projectmirrors.ListOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "mirror_list",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "mirror_list (after delete)")
		requireTruef(t, len(out.Mirrors) == 0, "expected 0 mirrors after delete, got %d", len(out.Mirrors))
		t.Logf("Mirrors after delete: %d (expected 0)", len(out.Mirrors))
	})
}
