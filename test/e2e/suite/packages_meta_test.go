//go:build e2e

package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/containerregistry"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/protectedpackages"
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
				"project_id":                     proj.pidStr(),
				"repository_path_pattern":        proj.Path + "/e2e-test",
				"minimum_access_level_for_push":  "maintainer",
				"minimum_access_level_for_delete": "maintainer",
			},
		})
		requireNoError(t, err, "registry_rule_create")
		requireTrue(t, out.ID > 0, "registry_rule_create: expected ID > 0")
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
				"project_id":           proj.pidStr(),
				"package_name_pattern": "e2e-test-*",
				"package_type":         "generic",
				"minimum_access_level_for_push": "maintainer",
			},
		})
		requireNoError(t, err, "protection_rule_create")
		requireTrue(t, createOut.ID > 0, "protection_rule_create: expected ID > 0")
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
			requireTrue(t, updateOut.ID == ruleID, "protection_rule_update: ID mismatch")
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
