//go:build e2e && enterprise

// securitycategories_ee_test.go exercises the security categories
// meta-tool against a live GitLab EE Ultimate instance. Security
// categories and attributes are Ultimate-only. The test creates a
// real category, updates it, and deletes it via the per-test ledger.
//
// The catalog exposes only create/update/delete actions (no list).
// The category ID is captured in the outer scope and reused.
//
// CANNOT parallelize: shares the meta session with the rest of the
// EE suite.
package suite

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/securitycategories"
)

// TestMeta_SecurityCategories exercises security category lifecycle
// (create/update/delete) via the gitlab_security_category meta-tool.
func TestMeta_SecurityCategories(t *testing.T) {
	if !sess.enterprise {
		t.Skip("security categories require GitLab Ultimate")
	}
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx := context.Background()
	const groupName = "e2e-sec-cat"
	e2e := NewE2EContext(t)
	grp := CreateGroupMeta(ctx, e2e, sess.meta, groupName)

	// The category requires a namespace_id and name. The category is
	// created at the group level; capture its ID for subsequent actions.
	var categoryID int64
	const categoryName = "e2e-sec-cat"

	t.Run("Meta/SecurityCategory/Create", func(t *testing.T) {
		desc := "E2E test category"
		out, err := callToolOn[securitycategories.Output](ctx, sess.meta, "gitlab_security_category", map[string]any{
			"action": "create",
			"params": map[string]any{
				"namespace_id": grp.ID,
				"name":         categoryName,
				"description":  desc,
			},
		})
		requireNoError(t, err, "security_category_create")
		requireTruef(t, out.ID > 0, "expected category ID > 0")
		categoryID = out.ID
		t.Logf("Created security category %d (%q)", categoryID, out.Name)
	})

	t.Run("Meta/SecurityCategory/Update", func(t *testing.T) {
		requireTruef(t, categoryID > 0, "categoryID not set (Create must run first)")
		newName := categoryName + "-upd"
		out, err := callToolOn[securitycategories.Output](ctx, sess.meta, "gitlab_security_category", map[string]any{
			"action": "update",
			"params": map[string]any{
				"category_id":  categoryID,
				"namespace_id": grp.ID,
				"name":         newName,
			},
		})
		requireNoError(t, err, "security_category_update")
		// Assert the update was applied: the returned payload must
		// reflect the new name and the same category ID. Without this
		// check, an update endpoint that silently no-ops would pass.
		requireTruef(t, out.ID == categoryID, "updated category ID = %d, want %d", out.ID, categoryID)
		requireTruef(t, out.Name == newName, "updated category name = %q, want %q", out.Name, newName)
		t.Logf("Updated security category %d (renamed to %q)", categoryID, newName)
	})

	t.Run("Meta/SecurityCategory/Delete", func(t *testing.T) {
		requireTruef(t, categoryID > 0, "categoryID not set (Create must run first)")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_security_category", map[string]any{
			"action": "delete",
			"params": map[string]any{
				"category_id": categoryID,
			},
		})
		requireNoError(t, err, "security_category_delete")
		t.Logf("Deleted security category %d", categoryID)
	})
}
