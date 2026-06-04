//go:build e2e && enterprise

// securityattributes_ee_test.go exercises the security attributes
// meta-tool against a live GitLab EE Ultimate instance. Security
// attributes are Ultimate-only. The test creates a real category
// first (security attributes live under a category), then creates
// an attribute under it, updates it, and deletes it via the
// per-test ledger.
//
// The catalog exposes only create/update/delete actions (no list).
//
// CANNOT parallelize: shares the meta session with the rest of the
// EE suite.
package suite

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/securityattributes"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/securitycategories"
)

// TestMeta_SecurityAttributes exercises security attribute lifecycle
// (create/update/delete) under a test category, via the
// gitlab_security_attribute meta-tool.
func TestMeta_SecurityAttributes(t *testing.T) {
	if !sess.enterprise {
		t.Skip("security attributes require GitLab Ultimate")
	}
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx := context.Background()
	const groupName = "e2e-sec-attr"
	e2e := NewE2EContext(t)
	grp := CreateGroupMeta(ctx, e2e, sess.meta, groupName)

	// First create a category to hold the attribute.
	var categoryID int64
	const categoryName = "e2e-attr-cat"
	t.Run("Setup/Category", func(t *testing.T) {
		out, err := callToolOn[securitycategories.Output](ctx, sess.meta, "gitlab_security_category", map[string]any{
			"action": "create",
			"params": map[string]any{
				"namespace_id": grp.ID,
				"name":         categoryName,
			},
		})
		requireNoError(t, err, "setup category for security attribute test")
		requireTruef(t, out.ID > 0, "expected category ID > 0")
		categoryID = out.ID
		t.Logf("Created setup category %d", categoryID)
	})

	var attributeID int64
	const attrName = "e2e-attr"

	t.Run("Meta/SecurityAttribute/Create", func(t *testing.T) {
		requireTruef(t, categoryID > 0, "categoryID not set (Setup/Category must run first)")
		out, err := callToolOn[securityattributes.CreateOutput](ctx, sess.meta, "gitlab_security_attribute", map[string]any{
			"action": "create",
			"params": map[string]any{
				"namespace_id": grp.ID,
				"category_id":  categoryID,
				"attributes": []map[string]any{
					{
						"name":        attrName,
						"description": "E2E test attribute",
						"color":       "#FF0000",
					},
				},
			},
		})
		requireNoError(t, err, "security_attribute_create")
		requireTruef(t, len(out.Attributes) > 0, "expected at least 1 attribute created")
		attributeID = out.Attributes[0].ID
		t.Logf("Created security attribute %d (%q)", attributeID, out.Attributes[0].Name)
	})

	t.Run("Meta/SecurityAttribute/Update", func(t *testing.T) {
		requireTruef(t, attributeID > 0, "attributeID not set (Create must run first)")
		newName := attrName + "-upd"
		out, err := callToolOn[securityattributes.Output](ctx, sess.meta, "gitlab_security_attribute", map[string]any{
			"action": "update",
			"params": map[string]any{
				"attribute_id": attributeID,
				"name":         newName,
			},
		})
		requireNoError(t, err, "security_attribute_update")
		// Assert the update was applied: the returned payload must
		// reflect the new name and the same attribute ID. Without this
		// check, an update endpoint that silently no-ops would pass.
		requireTruef(t, out.ID == attributeID, "updated attribute ID = %d, want %d", out.ID, attributeID)
		requireTruef(t, out.Name == newName, "updated attribute name = %q, want %q", out.Name, newName)
		t.Logf("Updated security attribute %d (renamed to %q)", attributeID, newName)
	})

	t.Run("Meta/SecurityAttribute/Delete", func(t *testing.T) {
		requireTruef(t, attributeID > 0, "attributeID not set (Create must run first)")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_security_attribute", map[string]any{
			"action": "delete",
			"params": map[string]any{
				"attribute_id": attributeID,
			},
		})
		requireNoError(t, err, "security_attribute_delete")
		t.Logf("Deleted security attribute %d", attributeID)
	})

	t.Run("Teardown/Category", func(t *testing.T) {
		requireTruef(t, categoryID > 0, "categoryID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_security_category", map[string]any{
			"action": "delete",
			"params": map[string]any{
				"category_id": categoryID,
			},
		})
		requireNoError(t, err, "teardown category")
		t.Logf("Tore down category %d", categoryID)
	})
}
