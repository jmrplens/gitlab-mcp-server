//go:build e2e

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
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/securityfindings"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

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
	requireTrue(t, grp.ID > 0, "group ID should be positive")
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
	requireTrue(t, grp.ID > 0, "group ID should be positive")
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
