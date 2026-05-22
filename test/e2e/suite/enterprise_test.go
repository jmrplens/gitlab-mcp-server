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

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/attestations"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/auditevents"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/compliancepolicy"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dependencies"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/enterpriseusers"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/externalstatuschecks"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/geo"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupscim"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupstoragemoves"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/memberroles"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/mergetrains"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projectaliases"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projectstoragemoves"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/securityattributes"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/securitycategories"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/securityfindings"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/snippets"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/snippetstoragemoves"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	grpName := uniqueName("audit-events")
	grpOut, setupErr := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "create",
		"params": map[string]any{
			"name":       grpName,
			"path":       grpName,
			"visibility": "private",
		},
	})
	requireNoError(t, setupErr, "create group for audit events")
	groupIDStr := strconv.FormatInt(grpOut.ID, 10)
	defer func() {
		_ = callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "delete",
			"params": map[string]any{"group_id": groupIDStr},
		})
	}()

	_, setupErr = callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "update",
		"params": map[string]any{
			"group_id":    groupIDStr,
			"description": "E2E audit event fixture",
		},
	})
	requireNoError(t, setupErr, "update group for audit event")

	proj := createProjectMeta(ctx, t, sess.meta)
	_, setupErr = callToolOn[projects.Output](ctx, sess.meta, "gitlab_project", map[string]any{
		"action": "update",
		"params": map[string]any{
			"project_id":  proj.pidStr(),
			"description": "E2E audit event fixture",
		},
	})
	requireNoError(t, setupErr, "update project for audit event")

	var instanceEventID int64
	var groupEventID int64
	var projectEventID int64

	t.Run("Meta/AuditEvent/ListProject", func(t *testing.T) {
		out := waitForAuditEvents(ctx, t, "list_project", map[string]any{
			"project_id": proj.pidStr(),
			"per_page":   20,
		})
		projectEventID = out.AuditEvents[0].ID
		t.Logf("Project audit events: %d", len(out.AuditEvents))
	})

	t.Run("Meta/AuditEvent/GetProject", func(t *testing.T) {
		requireTruef(t, projectEventID > 0, "projectEventID not set")
		out, err := callToolOn[auditevents.Output](ctx, sess.meta, "gitlab_audit_event", map[string]any{
			"action": "get_project",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"event_id":   projectEventID,
			},
		})
		requireNoError(t, err, "audit event get_project")
		requireTruef(t, out.ID == projectEventID, "project audit event ID = %d, want %d", out.ID, projectEventID)
	})

	t.Run("Meta/AuditEvent/ListGroup", func(t *testing.T) {
		out := waitForAuditEvents(ctx, t, "list_group", map[string]any{
			"group_id": groupIDStr,
			"per_page": 20,
		})
		groupEventID = out.AuditEvents[0].ID
		t.Logf("Group audit events: %d", len(out.AuditEvents))
	})

	t.Run("Meta/AuditEvent/GetGroup", func(t *testing.T) {
		requireTruef(t, groupEventID > 0, "groupEventID not set")
		out, err := callToolOn[auditevents.Output](ctx, sess.meta, "gitlab_audit_event", map[string]any{
			"action": "get_group",
			"params": map[string]any{
				"group_id": groupIDStr,
				"event_id": groupEventID,
			},
		})
		requireNoError(t, err, "audit event get_group")
		requireTruef(t, out.ID == groupEventID, "group audit event ID = %d, want %d", out.ID, groupEventID)
	})

	t.Run("Meta/AuditEvent/ListInstance", func(t *testing.T) {
		out := waitForAuditEvents(ctx, t, "list_instance", map[string]any{"per_page": 20})
		instanceEventID = out.AuditEvents[0].ID
		t.Logf("Instance audit events: %d", len(out.AuditEvents))
	})

	t.Run("Meta/AuditEvent/GetInstance", func(t *testing.T) {
		requireTruef(t, instanceEventID > 0, "instanceEventID not set")
		out, err := callToolOn[auditevents.Output](ctx, sess.meta, "gitlab_audit_event", map[string]any{
			"action": "get_instance",
			"params": map[string]any{"event_id": instanceEventID},
		})
		requireNoError(t, err, "audit event get_instance")
		requireTruef(t, out.ID == instanceEventID, "instance audit event ID = %d, want %d", out.ID, instanceEventID)
	})
}

func waitForAuditEvents(ctx context.Context, t *testing.T, action string, params map[string]any) auditevents.ListOutput {
	t.Helper()
	out, err := retryWithBackoff(ctx, t, "audit event "+action, 8, func(int) (auditevents.ListOutput, bool, string, error) {
		out, err := callToolOn[auditevents.ListOutput](ctx, sess.meta, "gitlab_audit_event", map[string]any{
			"action": action,
			"params": params,
		})
		if err != nil {
			return out, isRetryableError(err), "transient audit event API error", err
		}
		if len(out.AuditEvents) == 0 {
			return out, true, "audit events not indexed yet", fmt.Errorf("%s returned no audit events", action)
		}
		return out, false, "", nil
	})
	requirePremiumFeature(t, err, "audit events")
	return out
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

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	branch := uniqueName("external-status")
	createBranchMeta(ctx, t, sess.meta, proj, branch)
	commit := commitFileMeta(ctx, t, sess.meta, proj, branch, "external-status.txt", "external status check", "external status check fixture")
	mr := createMRMeta(ctx, t, sess.meta, proj, branch, defaultBranch, "external status check fixture")
	waitForMRReady(ctx, t, sess.glClient, proj.ID, mr.IID)

	var checkID int64
	var setStatusAccepted bool

	t.Run("Meta/ExternalStatusCheck/List", func(t *testing.T) {
		out, err := callToolOn[externalstatuschecks.ListProjectStatusCheckOutput](ctx, sess.meta, "gitlab_external_status_check", map[string]any{
			"action": "list_project_checks",
			"params": map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		requirePremiumFeature(t, err, "external status checks")
		t.Logf("Deprecated project status check list returned %d item(s)", len(out.Items))
	})

	t.Run("Meta/ExternalStatusCheck/Create", func(t *testing.T) {
		out, err := callToolOn[externalstatuschecks.ProjectStatusCheckOutput](ctx, sess.meta, "gitlab_external_status_check", map[string]any{
			"action": "create_project",
			"params": map[string]any{
				"project_id":   proj.pidStr(),
				"name":         uniqueName("e2e-external-status"),
				"external_url": "https://example.com/e2e/external-status",
			},
		})
		requirePremiumFeature(t, err, "external status check create")
		requireTruef(t, out.ID > 0, "external status check ID should be > 0")
		checkID = out.ID
		t.Logf("Created external status check %d", checkID)
	})

	t.Run("Meta/ExternalStatusCheck/ListProject", func(t *testing.T) {
		requireTruef(t, checkID > 0, "checkID not set")
		out, err := callToolOn[externalstatuschecks.ListProjectStatusCheckOutput](ctx, sess.meta, "gitlab_external_status_check", map[string]any{
			"action": "list_project",
			"params": map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		requireNoError(t, err, "external status check list_project")
		requireTruef(t, len(out.Items) > 0, "expected at least 1 project external status check")
		requireTruef(t, projectStatusCheckListed(out.Items, checkID), "created external status check %d not present in project list", checkID)
		t.Logf("Listed %d project external status check(s)", len(out.Items))
	})

	t.Run("Meta/ExternalStatusCheck/Update", func(t *testing.T) {
		requireTruef(t, checkID > 0, "checkID not set")
		out, err := callToolOn[externalstatuschecks.ProjectStatusCheckOutput](ctx, sess.meta, "gitlab_external_status_check", map[string]any{
			"action": "update_project",
			"params": map[string]any{
				"project_id":   proj.pidStr(),
				"check_id":     checkID,
				"name":         uniqueName("e2e-external-status-updated"),
				"external_url": "https://example.com/e2e/external-status-updated",
			},
		})
		requireNoError(t, err, "external status check update_project")
		requireTruef(t, out.ID == checkID, "external status check ID mismatch: want %d, got %d", checkID, out.ID)
	})

	t.Run("Meta/ExternalStatusCheck/ListMR", func(t *testing.T) {
		requireTruef(t, checkID > 0, "checkID not set")
		out, err := callToolOn[externalstatuschecks.ListMergeStatusCheckOutput](ctx, sess.meta, "gitlab_external_status_check", map[string]any{
			"action": "list_project_mr_checks",
			"params": map[string]any{
				"project_id":        proj.pidStr(),
				"merge_request_iid": mr.IID,
				"page":              1,
				"per_page":          10,
			},
		})
		requireNoError(t, err, "external status check list_project_mr_checks")
		requireTruef(t, mergeStatusCheckListed(out.Items, checkID), "created external status check %d not present in MR list", checkID)
		t.Logf("Listed %d MR external status check(s)", len(out.Items))
	})

	t.Run("Meta/ExternalStatusCheck/SetMRStatusPassed", func(t *testing.T) {
		requireTruef(t, checkID > 0, "checkID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_external_status_check", map[string]any{
			"action": "set_project_mr_status",
			"params": map[string]any{
				"project_id":               proj.pidStr(),
				"merge_request_iid":        mr.IID,
				"sha":                      commit.SHA,
				"external_status_check_id": checkID,
				"status":                   "passed",
			},
		})
		if err != nil {
			requireErrorContainsAll(t, err, "sha", "passed", "failed")
			return
		}
		t.Log("Set MR external status check status accepted")
		setStatusAccepted = true
	})

	t.Run("Meta/ExternalStatusCheck/ListMRPassedStatus", func(t *testing.T) {
		if !setStatusAccepted {
			t.Skip("set_project_mr_status was not accepted by GitLab")
		}
		out, err := callToolOn[externalstatuschecks.ListMergeStatusCheckOutput](ctx, sess.meta, "gitlab_external_status_check", map[string]any{
			"action": "list_project_mr_checks",
			"params": map[string]any{
				"project_id":        proj.pidStr(),
				"merge_request_iid": mr.IID,
			},
		})
		requireNoError(t, err, "external status check list_project_mr_checks after set status")
		status, ok := mergeStatusCheckStatus(out.Items, checkID)
		requireTruef(t, ok, "created external status check %d not present after status update", checkID)
		requireTruef(t, status == "passed", "external status check status = %q, want passed", status)
	})

	t.Run("Meta/ExternalStatusCheck/RetryRequiresFailedState", func(t *testing.T) {
		requireTruef(t, checkID > 0, "checkID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_external_status_check", map[string]any{
			"action": "retry_project",
			"params": map[string]any{
				"project_id":        proj.pidStr(),
				"merge_request_iid": mr.IID,
				"check_id":          checkID,
			},
		})
		if err != nil {
			requireErrorContainsAll(t, err, "failed", "gitlab_list_project_mr_external_status_checks")
			return
		}
		t.Log("Retry external status check accepted")
	})

	t.Run("Meta/ExternalStatusCheck/Delete", func(t *testing.T) {
		requireTruef(t, checkID > 0, "checkID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_external_status_check", map[string]any{
			"action": "delete_project",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"check_id":   checkID,
			},
		})
		requireNoError(t, err, "external status check delete_project")
	})
}

func projectStatusCheckListed(items []externalstatuschecks.ProjectStatusCheckOutput, checkID int64) bool {
	for _, item := range items {
		if item.ID == checkID {
			return true
		}
	}
	return false
}

func mergeStatusCheckListed(items []externalstatuschecks.MergeStatusCheckOutput, checkID int64) bool {
	_, ok := mergeStatusCheckStatus(items, checkID)
	return ok
}

func mergeStatusCheckStatus(items []externalstatuschecks.MergeStatusCheckOutput, checkID int64) (string, bool) {
	for _, item := range items {
		if item.ID == checkID {
			return item.Status, true
		}
	}
	return "", false
}

// TestMeta_MemberRoles exercises member role tools via the gitlab_member_role meta-tool.
// Requires GitLab Premium/Ultimate (GITLAB_ENTERPRISE=true).
func TestMeta_MemberRoles(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	grpName := uniqueName("member-role")
	grpOut, setupErr := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "create",
		"params": map[string]any{
			"name":       grpName,
			"path":       grpName,
			"visibility": "private",
		},
	})
	requireNoError(t, setupErr, "create group for member roles")
	defer func() {
		_ = callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "delete",
			"params": map[string]any{"group_id": strconv.FormatInt(grpOut.ID, 10)},
		})
	}()

	var instanceRoleID int64
	var groupRoleID int64
	var groupMemberRolesUnavailable bool
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if instanceRoleID > 0 {
			_ = callToolVoidOn(cleanupCtx, sess.meta, "gitlab_member_role", map[string]any{
				"action": "delete_instance",
				"params": map[string]any{"member_role_id": instanceRoleID},
			})
		}
		if groupRoleID > 0 {
			_ = callToolVoidOn(cleanupCtx, sess.meta, "gitlab_member_role", map[string]any{
				"action": "delete_group",
				"params": map[string]any{
					"group_id":       strconv.FormatInt(grpOut.ID, 10),
					"member_role_id": groupRoleID,
				},
			})
		}
	})

	t.Run("Meta/MemberRole/ListInstance", func(t *testing.T) {
		out, err := callToolOn[memberroles.ListOutput](ctx, sess.meta, "gitlab_member_role", map[string]any{
			"action": "list_instance",
			"params": map[string]any{},
		})
		requirePremiumFeature(t, err, "member roles")
		t.Logf("Listed %d instance member role(s)", len(out.Roles))
	})

	t.Run("Meta/MemberRole/CreateInstance", func(t *testing.T) {
		readCode := true
		out, err := callToolOn[memberroles.Output](ctx, sess.meta, "gitlab_member_role", map[string]any{
			"action": "create_instance",
			"params": map[string]any{
				"name":              uniqueName("e2e-instance-role"),
				"base_access_level": int64(10),
				"description":       "E2E instance custom role",
				"read_code":         readCode,
			},
		})
		requirePremiumFeature(t, err, "member role create_instance")
		requireTruef(t, out.ID > 0, "instance member role ID should be > 0")
		instanceRoleID = out.ID
	})

	t.Run("Meta/MemberRole/ListInstanceIncludesCreated", func(t *testing.T) {
		requireTruef(t, instanceRoleID > 0, "instanceRoleID not set")
		out, err := callToolOn[memberroles.ListOutput](ctx, sess.meta, "gitlab_member_role", map[string]any{
			"action": "list_instance",
			"params": map[string]any{},
		})
		requireNoError(t, err, "member role list_instance after create")
		requireTruef(t, memberRoleListed(out.Roles, instanceRoleID), "created instance member role %d not present in list", instanceRoleID)
	})

	t.Run("Meta/MemberRole/ListGroup", func(t *testing.T) {
		out, err := callToolOn[memberroles.ListOutput](ctx, sess.meta, "gitlab_member_role", map[string]any{
			"action": "list_group",
			"params": map[string]any{"group_id": strconv.FormatInt(grpOut.ID, 10)},
		})
		if err != nil {
			requireErrorContainsAll(t, err, "deprecated", "self-managed", "instance-level")
			groupMemberRolesUnavailable = true
			return
		}
		requireNoError(t, err, "member role list_group")
		t.Logf("Listed %d group member role(s)", len(out.Roles))
	})

	t.Run("Meta/MemberRole/CreateGroup", func(t *testing.T) {
		readCode := true
		out, err := callToolOn[memberroles.Output](ctx, sess.meta, "gitlab_member_role", map[string]any{
			"action": "create_group",
			"params": map[string]any{
				"group_id":          strconv.FormatInt(grpOut.ID, 10),
				"name":              uniqueName("e2e-group-role"),
				"base_access_level": int64(10),
				"description":       "E2E group custom role",
				"read_code":         readCode,
			},
		})
		if err != nil {
			requireErrorContainsAll(t, err, "deprecated", "self-managed", "instance-level")
			groupMemberRolesUnavailable = true
			return
		}
		requireNoError(t, err, "member role create_group")
		requireTruef(t, out.ID > 0, "group member role ID should be > 0")
		groupRoleID = out.ID
	})

	t.Run("Meta/MemberRole/ListGroupIncludesCreated", func(t *testing.T) {
		if groupMemberRolesUnavailable || groupRoleID <= 0 {
			return
		}
		requireTruef(t, groupRoleID > 0, "groupRoleID not set")
		out, err := callToolOn[memberroles.ListOutput](ctx, sess.meta, "gitlab_member_role", map[string]any{
			"action": "list_group",
			"params": map[string]any{"group_id": strconv.FormatInt(grpOut.ID, 10)},
		})
		requireNoError(t, err, "member role list_group after create")
		requireTruef(t, memberRoleListed(out.Roles, groupRoleID), "created group member role %d not present in list", groupRoleID)
	})

	t.Run("Meta/MemberRole/DeleteGroup", func(t *testing.T) {
		if groupMemberRolesUnavailable || groupRoleID <= 0 {
			err := callToolVoidOn(ctx, sess.meta, "gitlab_member_role", map[string]any{
				"action": "delete_group",
				"params": map[string]any{
					"group_id":       strconv.FormatInt(grpOut.ID, 10),
					"member_role_id": int64(1),
				},
			})
			requireErrorContainsAll(t, err, "deprecated", "self-managed", "instance-level")
			return
		}
		requireTruef(t, groupRoleID > 0, "groupRoleID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_member_role", map[string]any{
			"action": "delete_group",
			"params": map[string]any{
				"group_id":       strconv.FormatInt(grpOut.ID, 10),
				"member_role_id": groupRoleID,
			},
		})
		requireNoError(t, err, "member role delete_group")
		groupRoleID = 0
	})

	t.Run("Meta/MemberRole/DeleteInstance", func(t *testing.T) {
		requireTruef(t, instanceRoleID > 0, "instanceRoleID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_member_role", map[string]any{
			"action": "delete_instance",
			"params": map[string]any{"member_role_id": instanceRoleID},
		})
		requireNoError(t, err, "member role delete_instance")
		instanceRoleID = 0
	})
}

func memberRoleListed(roles []memberroles.Output, roleID int64) bool {
	for _, role := range roles {
		if role.ID == roleID {
			return true
		}
	}
	return false
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

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	grpName := uniqueName("storage-move")
	grpOut, setupErr := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "create",
		"params": map[string]any{
			"name":       grpName,
			"path":       grpName,
			"visibility": "private",
		},
	})
	requireNoError(t, setupErr, "create group for storage moves")
	groupID := grpOut.ID
	groupIDStr := strconv.FormatInt(groupID, 10)
	defer func() {
		_ = callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "delete",
			"params": map[string]any{"group_id": groupIDStr},
		})
	}()

	proj := createProjectMeta(ctx, t, sess.meta)
	snippetOut, setupErr := callToolOn[snippets.Output](ctx, sess.meta, "gitlab_snippet", map[string]any{
		"action": "create",
		"params": map[string]any{
			"title":       uniqueName("storage-move-snippet"),
			"file_name":   "storage-move.txt",
			"content":     "storage move fixture",
			"visibility":  "private",
			"description": "E2E storage move fixture",
		},
	})
	requireNoError(t, setupErr, "create snippet for storage moves")
	snippetID := snippetOut.ID
	defer func() {
		_ = callToolVoidOn(ctx, sess.meta, "gitlab_snippet", map[string]any{
			"action": "delete",
			"params": map[string]any{"snippet_id": snippetID},
		})
	}()

	t.Run("Meta/StorageMove/RetrieveAllProject", func(t *testing.T) {
		out, err := callToolOn[projectstoragemoves.ListOutput](ctx, sess.meta, "gitlab_storage_move", map[string]any{
			"action": "retrieve_all_project",
			"params": map[string]any{},
		})
		requirePremiumFeature(t, err, "storage moves")
		t.Logf("Project storage moves: %d", len(out.Moves))
	})

	t.Run("Meta/StorageMove/RetrieveProject", func(t *testing.T) {
		out, err := callToolOn[projectstoragemoves.ListOutput](ctx, sess.meta, "gitlab_storage_move", map[string]any{
			"action": "retrieve_project",
			"params": map[string]any{"project_id": proj.ID},
		})
		requireNoError(t, err, "retrieve project storage moves")
		t.Logf("Project-specific storage moves: %d", len(out.Moves))
	})

	t.Run("Meta/StorageMove/RetrieveAllGroup", func(t *testing.T) {
		out, err := callToolOn[groupstoragemoves.ListOutput](ctx, sess.meta, "gitlab_storage_move", map[string]any{
			"action": "retrieve_all_group",
			"params": map[string]any{},
		})
		requireNoError(t, err, "retrieve all group storage moves")
		t.Logf("Group storage moves: %d", len(out.Moves))
	})

	t.Run("Meta/StorageMove/RetrieveGroup", func(t *testing.T) {
		out, err := callToolOn[groupstoragemoves.ListOutput](ctx, sess.meta, "gitlab_storage_move", map[string]any{
			"action": "retrieve_group",
			"params": map[string]any{"group_id": groupID},
		})
		requireNoError(t, err, "retrieve group storage moves")
		t.Logf("Group-specific storage moves: %d", len(out.Moves))
	})

	t.Run("Meta/StorageMove/RetrieveAllSnippet", func(t *testing.T) {
		out, err := callToolOn[snippetstoragemoves.ListOutput](ctx, sess.meta, "gitlab_storage_move", map[string]any{
			"action": "retrieve_all_snippet",
			"params": map[string]any{},
		})
		requireNoError(t, err, "retrieve all snippet storage moves")
		t.Logf("Snippet storage moves: %d", len(out.Moves))
	})

	t.Run("Meta/StorageMove/RetrieveSnippet", func(t *testing.T) {
		out, err := callToolOn[snippetstoragemoves.ListOutput](ctx, sess.meta, "gitlab_storage_move", map[string]any{
			"action": "retrieve_snippet",
			"params": map[string]any{"snippet_id": snippetID},
		})
		requireNoError(t, err, "retrieve snippet storage moves")
		t.Logf("Snippet-specific storage moves: %d", len(out.Moves))
	})

	t.Run("Meta/StorageMove/ScheduleProjectInvalidStorage", func(t *testing.T) {
		_, err := callToolOn[projectstoragemoves.Output](ctx, sess.meta, "gitlab_storage_move", map[string]any{
			"action": "schedule_project",
			"params": map[string]any{
				"project_id":               proj.ID,
				"destination_storage_name": "e2e-missing-storage",
			},
		})
		requireErrorContainsAll(t, err, "destination_storage_name", "Gitaly", "storage")
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
