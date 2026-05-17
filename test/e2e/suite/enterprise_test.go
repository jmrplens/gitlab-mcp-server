//go:build e2e

// enterprise_test.go tests GitLab Premium/Ultimate (Enterprise) MCP tools against a live
// instance. Each test requires GITLAB_ENTERPRISE=true and gracefully skips via
// requirePremiumFeature when the feature is unavailable.
package suite

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/attestations"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/auditevents"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/compliancepolicy"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/dependencies"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/enterpriseusers"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/geo"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupscim"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/memberroles"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mergetrains"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projectaliases"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/securityattributes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/securitycategories"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/securityfindings"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestEnterpriseSecurityTools_NotRegisteredOnCE verifies that CE E2E runs do
// not expose Premium/Ultimate security classification tools at all.
func TestEnterpriseSecurityTools_NotRegisteredOnCE(t *testing.T) {
	t.Parallel()
	if sess.enterprise {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	individual, err := sess.individual.ListTools(ctx, nil)
	requireNoError(t, err, "list individual tools")
	individualForbidden := map[string]bool{
		"gitlab_bulk_update_security_attributes":   true,
		"gitlab_create_security_attribute":         true,
		"gitlab_create_security_category":          true,
		"gitlab_delete_security_attribute":         true,
		"gitlab_delete_security_category":          true,
		"gitlab_project_update_security_attribute": true,
		"gitlab_update_security_attribute":         true,
		"gitlab_update_security_category":          true,
	}
	for _, tool := range individual.Tools {
		if individualForbidden[tool.Name] {
			t.Fatalf("CE individual surface exposed enterprise tool %q", tool.Name)
		}
	}

	meta, err := sess.meta.ListTools(ctx, nil)
	requireNoError(t, err, "list meta-tools")
	metaForbidden := map[string]bool{
		"gitlab_security_attribute": true,
		"gitlab_security_category":  true,
	}
	for _, tool := range meta.Tools {
		if metaForbidden[tool.Name] {
			t.Fatalf("CE meta surface exposed enterprise tool %q", tool.Name)
		}
	}
}

// TestMeta_MergeTrains exercises merge train tools via the gitlab_merge_train meta-tool.
// Requires GitLab Premium/Ultimate (GITLAB_ENTERPRISE=true).
func TestMeta_MergeTrains(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}

	ctx := context.Background()
	proj := createProjectMeta(ctx, t, sess.meta)

	t.Run("Meta/MergeTrain/ListProject", func(t *testing.T) {
		_, err := callToolOn[mergetrains.ListOutput](ctx, sess.meta, "gitlab_merge_train", map[string]any{
			"action": "list_project",
			"params": map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		requirePremiumFeature(t, err, "merge trains")
		t.Log("Merge train list OK")
	})
}

// TestMeta_AuditEvents exercises audit event tools via the gitlab_audit_event meta-tool.
// Requires GitLab Premium/Ultimate (GITLAB_ENTERPRISE=true).
func TestMeta_AuditEvents(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}

	ctx := context.Background()
	proj := createProjectMeta(ctx, t, sess.meta)

	t.Run("Meta/AuditEvent/ListProject", func(t *testing.T) {
		_, err := callToolOn[auditevents.ListOutput](ctx, sess.meta, "gitlab_audit_event", map[string]any{
			"action": "list_project",
			"params": map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		requirePremiumFeature(t, err, "audit events")
		t.Log("Audit event list OK")
	})
}

// TestMeta_DORAMetrics exercises DORA metrics via the gitlab_dora_metrics meta-tool.
// Requires GitLab Premium/Ultimate (GITLAB_ENTERPRISE=true).
func TestMeta_DORAMetrics(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}

	ctx := context.Background()
	proj := createProjectMeta(ctx, t, sess.meta)

	t.Run("Meta/DORA/Project", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_dora_metrics", map[string]any{
			"action": "project",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"metric":     "deployment_frequency",
			},
		})
		requirePremiumFeature(t, err, "DORA metrics")
		t.Log("DORA metrics OK")
	})
}

// TestMeta_Dependencies exercises dependency tools via the gitlab_dependency meta-tool.
// Requires GitLab Premium/Ultimate (GITLAB_ENTERPRISE=true).
func TestMeta_Dependencies(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}

	ctx := context.Background()
	proj := createProjectMeta(ctx, t, sess.meta)

	t.Run("Meta/Dependency/List", func(t *testing.T) {
		_, err := callToolOn[dependencies.ListOutput](ctx, sess.meta, "gitlab_dependency", map[string]any{
			"action": "list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		requirePremiumFeature(t, err, "dependencies")
		t.Log("Dependency list OK")
	})
}

// TestMeta_ExternalStatusChecks exercises external status check tools via
// the gitlab_external_status_check meta-tool.
// Requires GitLab Premium/Ultimate (GITLAB_ENTERPRISE=true).
func TestMeta_ExternalStatusChecks(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}

	ctx := context.Background()
	proj := createProjectMeta(ctx, t, sess.meta)

	t.Run("Meta/ExternalStatusCheck/List", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_external_status_check", map[string]any{
			"action": "list_project_checks",
			"params": map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		requirePremiumFeature(t, err, "external status checks")
		t.Log("External status check list OK")
	})
}

// TestMeta_MemberRoles exercises member role tools via the gitlab_member_role meta-tool.
// Requires GitLab Premium/Ultimate (GITLAB_ENTERPRISE=true).
func TestMeta_MemberRoles(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}

	ctx := context.Background()

	t.Run("Meta/MemberRole/ListInstance", func(t *testing.T) {
		_, err := callToolOn[memberroles.ListOutput](ctx, sess.meta, "gitlab_member_role", map[string]any{
			"action": "list_instance",
			"params": map[string]any{},
		})
		requirePremiumFeature(t, err, "member roles")
		t.Log("Member role list OK")
	})
}

// TestMeta_Attestations exercises attestation tools via the gitlab_attestation meta-tool.
// Requires GitLab Premium/Ultimate (GITLAB_ENTERPRISE=true).
func TestMeta_Attestations(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}

	ctx := context.Background()
	proj := createProjectMeta(ctx, t, sess.meta)

	t.Run("Meta/Attestation/List", func(t *testing.T) {
		_, err := callToolOn[attestations.ListOutput](ctx, sess.meta, "gitlab_attestation", map[string]any{
			"action": "list",
			"params": map[string]any{
				"project_id":     proj.pidStr(),
				"subject_digest": "sha256:0000000000000000000000000000000000000000000000000000000000000000",
			},
		})
		requirePremiumFeature(t, err, "attestations")
		t.Log("Attestation list OK")
	})
}

// TestMeta_CompliancePolicy exercises compliance policy tools via the
// gitlab_compliance_policy meta-tool.
// Requires GitLab Premium/Ultimate (GITLAB_ENTERPRISE=true).
func TestMeta_CompliancePolicy(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}

	ctx := context.Background()

	t.Run("Meta/CompliancePolicy/Get", func(t *testing.T) {
		_, err := callToolOn[compliancepolicy.Output](ctx, sess.meta, "gitlab_compliance_policy", map[string]any{
			"action": "get",
			"params": map[string]any{},
		})
		requirePremiumFeature(t, err, "compliance policy")
		t.Log("Compliance policy get OK")
	})
}

// TestMeta_ProjectAliases exercises project alias tools via the
// gitlab_project_alias meta-tool.
// Requires GitLab Premium/Ultimate (GITLAB_ENTERPRISE=true).
func TestMeta_ProjectAliases(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}

	ctx := context.Background()

	t.Run("Meta/ProjectAlias/List", func(t *testing.T) {
		_, err := callToolOn[projectaliases.ListOutput](ctx, sess.meta, "gitlab_project_alias", map[string]any{
			"action": "list",
			"params": map[string]any{},
		})
		requirePremiumFeature(t, err, "project aliases")
		t.Log("Project alias list OK")
	})
}

// TestMeta_Geo exercises Geo site tools via the gitlab_geo meta-tool.
// Requires GitLab Premium/Ultimate (GITLAB_ENTERPRISE=true).
func TestMeta_Geo(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}

	ctx := context.Background()

	t.Run("Meta/Geo/List", func(t *testing.T) {
		_, err := callToolOn[geo.ListOutput](ctx, sess.meta, "gitlab_geo", map[string]any{
			"action": "list",
			"params": map[string]any{},
		})
		requirePremiumFeature(t, err, "Geo sites")
		t.Log("Geo list OK")
	})
}

// TestMeta_StorageMoves exercises storage move tools via the
// gitlab_storage_move meta-tool.
// Requires GitLab Premium/Ultimate (GITLAB_ENTERPRISE=true).
func TestMeta_StorageMoves(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}

	ctx := context.Background()

	t.Run("Meta/StorageMove/RetrieveAllProject", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_storage_move", map[string]any{
			"action": "retrieve_all_project",
			"params": map[string]any{},
		})
		requirePremiumFeature(t, err, "storage moves")
		t.Log("Storage move list OK")
	})
}

// TestMeta_SecurityFindings exercises security finding tools via the
// gitlab_security_finding meta-tool.
// Requires GitLab Premium/Ultimate (GITLAB_ENTERPRISE=true).
func TestMeta_SecurityFindings(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}

	ctx := context.Background()
	proj := createProjectMeta(ctx, t, sess.meta)

	t.Run("Meta/SecurityFinding/List", func(t *testing.T) {
		_, err := callToolOn[securityfindings.ListOutput](ctx, sess.meta, "gitlab_security_finding", map[string]any{
			"action": "list",
			"params": map[string]any{
				"project_path": proj.Path,
				"pipeline_iid": "1",
			},
		})
		requirePremiumFeature(t, err, "security findings")
		t.Log("Security finding list OK")
	})
}

// TestMeta_SecurityClassifications exercises security category and security
// attribute lifecycle tools via their Premium/Ultimate meta-tools.
func TestMeta_SecurityClassifications(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	e2e := NewE2EContext(t)
	group := CreateGroupMeta(ctx, e2e, sess.meta, "e2e-sec-class-")

	projectName := uniqueName(e2eProjectPrefix + "sec-class-" + sanitizeTestName(t.Name()))
	project, err := callToolOn[projects.Output](ctx, sess.meta, "gitlab_project", map[string]any{
		"action": "create",
		"params": map[string]any{
			"name":                   projectName,
			"path":                   projectName,
			"namespace_id":           int(group.ID),
			"description":            "E2E security classification project",
			"visibility":             "private",
			"initialize_with_readme": true,
			"default_branch":         defaultBranch,
		},
	})
	requireNoError(t, err, "create project for security classification tests")
	requireTruef(t, project.ID > 0, "project ID should be positive")
	t.Cleanup(func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanCancel()
		_ = callToolVoidOn(cleanCtx, sess.meta, "gitlab_project", map[string]any{
			"action": "delete",
			"params": map[string]any{
				"project_id":         strconv.FormatInt(project.ID, 10),
				"permanently_remove": true,
				"full_path":          project.PathWithNamespace,
			},
		})
	})

	description := "E2E security classification category"
	multipleSelection := true
	category, err := callToolOn[securitycategories.Output](ctx, sess.meta, "gitlab_security_category", map[string]any{
		"action": "create",
		"params": map[string]any{
			"namespace_id":       group.ID,
			"name":               uniqueName("e2e-category-"),
			"description":        description,
			"multiple_selection": multipleSelection,
		},
	})
	requirePremiumFeature(t, err, "security categories")
	requireTruef(t, category.ID > 0, "category ID should be positive")
	requireTruef(t, category.MultipleSelection, "category should allow multiple selection")
	t.Cleanup(func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanCancel()
		_ = callToolVoidOn(cleanCtx, sess.meta, "gitlab_security_category", map[string]any{
			"action": "delete",
			"params": map[string]any{"category_id": category.ID},
		})
	})

	updatedDescription := "Updated E2E security classification category"
	updatedCategory, err := callToolOn[securitycategories.Output](ctx, sess.meta, "gitlab_security_category", map[string]any{
		"action": "update",
		"params": map[string]any{
			"category_id":  category.ID,
			"namespace_id": group.ID,
			"description":  updatedDescription,
		},
	})
	requirePremiumFeature(t, err, "security categories")
	requireTruef(t, updatedCategory.Description == updatedDescription, "category description = %q, want %q", updatedCategory.Description, updatedDescription)

	attributes, err := callToolOn[securityattributes.CreateOutput](ctx, sess.meta, "gitlab_security_attribute", map[string]any{
		"action": "create",
		"params": map[string]any{
			"namespace_id": group.ID,
			"category_id":  category.ID,
			"attributes": []map[string]any{
				{
					"name":        uniqueName("e2e-attribute-"),
					"description": "E2E security classification attribute",
					"color":       "#FF0000",
				},
			},
		},
	})
	requirePremiumFeature(t, err, "security attributes")
	requireTruef(t, len(attributes.Attributes) == 1, "created attributes = %d, want 1", len(attributes.Attributes))
	attribute := attributes.Attributes[0]
	requireTruef(t, attribute.ID > 0, "attribute ID should be positive")
	t.Cleanup(func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanCancel()
		_ = callToolVoidOn(cleanCtx, sess.meta, "gitlab_security_attribute", map[string]any{
			"action": "delete",
			"params": map[string]any{"attribute_id": attribute.ID},
		})
	})

	updatedColor := "#00FF00"
	updatedAttribute, err := callToolOn[securityattributes.Output](ctx, sess.meta, "gitlab_security_attribute", map[string]any{
		"action": "update",
		"params": map[string]any{
			"attribute_id": attribute.ID,
			"color":        updatedColor,
		},
	})
	requirePremiumFeature(t, err, "security attributes")
	requireTruef(t, updatedAttribute.Color == updatedColor, "attribute color = %q, want %q", updatedAttribute.Color, updatedColor)

	projectAdd, err := callToolOn[securityattributes.ProjectUpdateOutput](ctx, sess.meta, "gitlab_security_attribute", map[string]any{
		"action": "project_update",
		"params": map[string]any{
			"project_id":        project.ID,
			"add_attribute_ids": []int64{attribute.ID},
		},
	})
	requirePremiumFeature(t, err, "security attributes")
	requireTruef(t, projectAdd.AddedCount >= 1, "added count should be at least one")

	bulk, err := callToolOn[securityattributes.BulkUpdateOutput](ctx, sess.meta, "gitlab_security_attribute", map[string]any{
		"action": "bulk_update",
		"params": map[string]any{
			"project_ids":   []int64{project.ID},
			"attribute_ids": []int64{attribute.ID},
			"mode":          securityattributes.BulkUpdateModeAdd,
		},
	})
	requirePremiumFeature(t, err, "security attributes")
	requireTruef(t, bulk.Status == "success", "bulk update status = %q, want success", bulk.Status)

	projectRemove, err := callToolOn[securityattributes.ProjectUpdateOutput](ctx, sess.meta, "gitlab_security_attribute", map[string]any{
		"action": "project_update",
		"params": map[string]any{
			"project_id":           project.ID,
			"remove_attribute_ids": []int64{attribute.ID},
		},
	})
	requirePremiumFeature(t, err, "security attributes")
	requireTruef(t, projectRemove.RemovedCount >= 1, "removed count should be at least one")

	err = callToolVoidOn(ctx, sess.meta, "gitlab_security_attribute", map[string]any{
		"action": "delete",
		"params": map[string]any{"attribute_id": attribute.ID},
	})
	requirePremiumFeature(t, err, "security attributes")

	err = callToolVoidOn(ctx, sess.meta, "gitlab_security_category", map[string]any{
		"action": "delete",
		"params": map[string]any{"category_id": category.ID},
	})
	requirePremiumFeature(t, err, "security categories")
}

// TestMeta_GroupSCIM exercises Group SCIM tools via the gitlab_group_scim meta-tool.
// Requires GitLab Premium/Ultimate (GITLAB_ENTERPRISE=true).
func TestMeta_GroupSCIM(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	groupPath := fmt.Sprintf("e2e-scim-%d", time.Now().UnixMilli())
	grp, grpErr := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "create",
		"params": map[string]any{
			"name":       groupPath,
			"path":       groupPath,
			"visibility": "public",
		},
	})
	requireNoError(t, grpErr, "create group for SCIM tests")
	requireTruef(t, grp.ID > 0, "group ID should be positive")
	groupID := strconv.FormatInt(grp.ID, 10)
	t.Logf("Created group %d (%s) for SCIM tests", grp.ID, grp.FullPath)

	t.Cleanup(func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanCancel()
		_ = callToolVoidOn(cleanCtx, sess.individual, "gitlab_group_delete", groups.DeleteInput{
			GroupID: toolutil.StringOrInt(groupID),
		})
	})

	t.Run("Meta/GroupSCIM/List", func(t *testing.T) {
		_, err := callToolOn[groupscim.ListOutput](ctx, sess.meta, "gitlab_group_scim", map[string]any{
			"action": "list",
			"params": map[string]any{
				"group_id": groupID,
			},
		})
		requirePremiumFeature(t, err, "Group SCIM")
		t.Log("Group SCIM list OK")
	})
}

// TestMeta_EnterpriseUsers exercises enterprise user tools via the
// gitlab_enterprise_user meta-tool.
// Requires GitLab Premium/Ultimate (GITLAB_ENTERPRISE=true).
func TestMeta_EnterpriseUsers(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	groupPath := fmt.Sprintf("e2e-entusers-%d", time.Now().UnixMilli())
	grp, grpErr := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "create",
		"params": map[string]any{
			"name":       groupPath,
			"path":       groupPath,
			"visibility": "public",
		},
	})
	requireNoError(t, grpErr, "create group for enterprise users")
	requireTruef(t, grp.ID > 0, "group ID should be positive")
	groupID := strconv.FormatInt(grp.ID, 10)
	t.Logf("Created group %d (%s) for enterprise user tests", grp.ID, grp.FullPath)

	t.Cleanup(func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanCancel()
		_ = callToolVoidOn(cleanCtx, sess.individual, "gitlab_group_delete", groups.DeleteInput{
			GroupID: toolutil.StringOrInt(groupID),
		})
	})

	t.Run("Meta/EnterpriseUser/List", func(t *testing.T) {
		_, err := callToolOn[enterpriseusers.ListOutput](ctx, sess.meta, "gitlab_enterprise_user", map[string]any{
			"action": "list",
			"params": map[string]any{
				"group_id": groupID,
			},
		})
		requirePremiumFeature(t, err, "enterprise users")
		t.Log("Enterprise user list OK")
	})
}
