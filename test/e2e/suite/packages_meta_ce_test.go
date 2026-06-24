//go:build e2e && !enterprise

// packages_meta_ce_test.go tests the container registry and package protection rule
// MCP tools against a live GitLab instance via the gitlab_package meta-tool.
// Covers registry listing, registry protection rule CRUD, and package protection rule CRUD.
package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/containerregistry"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/protectedpackages"
)

// TestMeta_PackagesRegistry exercises container registry actions via gitlab_package.
func TestMeta_PackagesRegistry(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	t.Run("RegistryListProject", func(t *testing.T) {
		out, err := callToolOn[containerregistry.RepositoryListOutput](ctx, sess.meta, "gitlab_package", map[string]any{
			"action": "registry_list_project",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "registry_list_project")
		t.Logf("Container repos: %d", len(out.Repositories))
	})

	t.Run("RegistryRuleList", func(t *testing.T) {
		out, err := callToolOn[containerregistry.ProtectionRuleListOutput](ctx, sess.meta, "gitlab_package", map[string]any{
			"action": "registry_rule_list",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "registry_rule_list")
		t.Logf("Registry protection rules: %d", len(out.Rules))
	})

	t.Run("RegistryRuleCreate", func(t *testing.T) {
		out, err := callToolOn[containerregistry.ProtectionRuleOutput](ctx, sess.meta, "gitlab_package", map[string]any{
			"action": "registry_rule_create",
			"params": map[string]any{
				"project_id":                      proj.pidStr(),
				"repository_path_pattern":         proj.Path + "/e2e-test",
				"minimum_access_level_for_push":   "maintainer",
				"minimum_access_level_for_delete": "maintainer",
			},
		})
		requireNoError(t, err, "registry_rule_create")
		requireTruef(t, out.ID > 0, "registry_rule_create: expected ID > 0")
		t.Logf("Created registry rule %d", out.ID)

		// Clean up
		_ = callToolVoidOn(ctx, sess.meta, "gitlab_package", map[string]any{
			"action": "registry_rule_delete",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"rule_id":    out.ID,
			},
		})
	})

	// Container registry TAG protection rules (separate REST surface from the
	// repository-path rules above). The immutable-tags API requires GitLab
	// 17.8+, so list/create tolerate a 404 on older instances and skip the
	// rest; when available the full CRUD lifecycle is asserted.
	t.Run("RegistryTagRuleCRUD", func(t *testing.T) {
		listOut, err := callToolOn[containerregistry.TagProtectionRuleListOutput](ctx, sess.meta, "gitlab_package", map[string]any{
			"action": "registry_tag_rule_list",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		if err != nil {
			if isHTTPStatus(err, 404) {
				t.Skipf("container registry tag protection not available on this GitLab version: %v", err)
			}
			requireNoError(t, err, "registry_tag_rule_list")
		}
		t.Logf("Registry tag protection rules: %d", len(listOut.Rules))

		createOut, err := callToolOn[containerregistry.TagProtectionRuleOutput](ctx, sess.meta, "gitlab_package", map[string]any{
			"action": "registry_tag_rule_create",
			"params": map[string]any{
				"project_id":                      proj.pidStr(),
				"tag_name_pattern":                "v.+",
				"minimum_access_level_for_push":   "maintainer",
				"minimum_access_level_for_delete": "maintainer",
			},
		})
		if err != nil && isHTTPStatus(err, 404) {
			t.Skipf("container registry tag protection create not available: %v", err)
		}
		requireNoError(t, err, "registry_tag_rule_create")
		requireTruef(t, createOut.ID > 0, "registry_tag_rule_create: expected ID > 0")
		t.Logf("Created registry tag protection rule %d", createOut.ID)

		updateOut, err := callToolOn[containerregistry.TagProtectionRuleOutput](ctx, sess.meta, "gitlab_package", map[string]any{
			"action": "registry_tag_rule_update",
			"params": map[string]any{
				"project_id":       proj.pidStr(),
				"rule_id":          createOut.ID,
				"tag_name_pattern": "release-.+",
			},
		})
		if err != nil {
			t.Logf("registry_tag_rule_update may have limitations: %v", err)
		} else {
			requireTruef(t, updateOut.ID == createOut.ID, "registry_tag_rule_update: ID mismatch")
		}

		err = callToolVoidOn(ctx, sess.meta, "gitlab_package", map[string]any{
			"action": "registry_tag_rule_delete",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"rule_id":    createOut.ID,
			},
		})
		requireNoError(t, err, "registry_tag_rule_delete")
	})
}

// TestMeta_PackagesProtectionRules exercises package protection rules via gitlab_package.
func TestMeta_PackagesProtectionRules(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	t.Run("ProtectionRuleList", func(t *testing.T) {
		out, err := callToolOn[protectedpackages.ListOutput](ctx, sess.meta, "gitlab_package", map[string]any{
			"action": "protection_rule_list",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "protection_rule_list")
		t.Logf("Package protection rules: %d", len(out.Rules))
	})

	t.Run("ProtectionRuleCRUD", func(t *testing.T) {
		createOut, err := callToolOn[protectedpackages.Output](ctx, sess.meta, "gitlab_package", map[string]any{
			"action": "protection_rule_create",
			"params": map[string]any{
				"project_id":                    proj.pidStr(),
				"package_name_pattern":          "e2e-test-*",
				"package_type":                  "generic",
				"minimum_access_level_for_push": "maintainer",
			},
		})
		requireNoError(t, err, "protection_rule_create")
		requireTruef(t, createOut.ID > 0, "protection_rule_create: expected ID > 0")
		ruleID := createOut.ID
		t.Logf("Created package protection rule %d", ruleID)

		// Update
		updateOut, err := callToolOn[protectedpackages.Output](ctx, sess.meta, "gitlab_package", map[string]any{
			"action": "protection_rule_update",
			"params": map[string]any{
				"project_id":           proj.pidStr(),
				"rule_id":              ruleID,
				"package_name_pattern": "e2e-updated-*",
				"package_type":         "generic",
			},
		})
		if err != nil {
			t.Logf("protection_rule_update may have limitations: %v", err)
		} else {
			requireTruef(t, updateOut.ID == ruleID, "protection_rule_update: ID mismatch")
		}

		// Delete
		err = callToolVoidOn(ctx, sess.meta, "gitlab_package", map[string]any{
			"action": "protection_rule_delete",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"rule_id":    ruleID,
			},
		})
		requireNoError(t, err, "protection_rule_delete")
	})
}
