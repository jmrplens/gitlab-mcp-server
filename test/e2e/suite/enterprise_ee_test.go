//go:build e2e && enterprise

// enterprise_test.go tests GitLab Premium/Ultimate (Enterprise) MCP tools against a live
// instance. Each test requires GITLAB_ENTERPRISE=true and gracefully skips via
// requirePremiumFeature when the feature is unavailable.
package suite

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/attestations"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/auditevents"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/compliancepolicy"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dependencies"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dorametrics"
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

// TestMeta_MergeTrains exercises merge train tools via the gitlab_merge_train meta-tool.
// Requires GitLab Premium/Ultimate (GITLAB_ENTERPRISE=true).
func TestMeta_MergeTrains(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}

	ctx := context.Background()

	// Create a group so the project lives under a Premium/Ultimate
	// group namespace. Merge trains are a Premium/Ultimate feature and
	// GitLab does not persist merge_trains_enabled on projects in
	// personal namespaces on self-managed EE — the API call succeeds
	// but the value is dropped because the namespace lacks the
	// required license feature flag.
	grpName := uniqueName("mt-group")
	e2e := NewE2EContext(t)
	grp := CreateGroupMeta(ctx, e2e, sess.meta, grpName)
	proj := CreateProjectUnderGroupMeta(ctx, e2e, sess.meta, grp.ID)

	// Enable merge trains on the project so we can add MRs to the train
	// and list trains per branch. Per the GitLab merge_trains docs
	// (https://docs.gitlab.com/ci/pipelines/merge_trains/), BOTH the
	// `merge_pipelines_enabled` (Enable merged results pipelines) and
	// `merge_trains_enabled` settings must be enabled. Setting only
	// merge_trains_enabled is silently dropped on Premium/Ultimate
	// when the project lives in a group namespace.
	t.Run("EnableMergeTrainsOnProject", func(t *testing.T) {
		enable := true
		_, _, err := sess.glClient.GL().Projects.EditProject(
			proj.Path,
			&gl.EditProjectOptions{
				MergePipelinesEnabled: &enable,
				MergeTrainsEnabled:    &enable,
			},
			gl.WithContext(ctx),
		)
		requireNoError(t, err, "glClient.EditProject enable merge trains")
		raw, _, rawErr := sess.glClient.GL().Projects.GetProject(proj.Path, &gl.GetProjectOptions{})
		requireNoError(t, rawErr, "glClient.GetProject after enabling merge trains")
		if !raw.MergePipelinesEnabled || !raw.MergeTrainsEnabled {
			t.Skipf("merge trains prerequisites not persisted on project %s (MergePipelinesEnabled=%v, MergeTrainsEnabled=%v); falling back to error-path assertions",
				proj.Path, raw.MergePipelinesEnabled, raw.MergeTrainsEnabled)
		}
		t.Logf("Project %s now has merge_pipelines_enabled=true and merge_trains_enabled=true", proj.Path)
	})

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

	// Build a real MR lifecycle so we have an MR to add to a merge train
	// when the feature is enabled. When merge_trains is not enabled on
	// the project the add call returns 400 "Merge trains are not enabled
	// for this project", which still validates the routing.
	var mrIID int64
	t.Run("Meta/MergeTrain/Add", func(t *testing.T) {
		branch := uniqueName("mt-branch")
		createBranchMeta(ctx, t, sess.meta, proj, branch)
		commitFileMeta(ctx, t, sess.meta, proj, branch, "merge-train.txt", "merge train fixture", "add to merge train")
		mr := createMRMeta(ctx, t, sess.meta, proj, branch, defaultBranch, "merge train fixture")
		waitForMRReady(ctx, t, sess.glClient, proj.ID, mr.IID)
		mrIID = mr.IID

		out, err := callToolOn[mergetrains.Output](ctx, sess.meta, "gitlab_merge_train", map[string]any{
			"action": "add",
			"params": map[string]any{
				"project_id":        proj.pidStr(),
				"merge_request_iid": mrIID,
			},
		})
		if err != nil {
			// 400 "Merge trains are not enabled" is the expected routing
			// outcome on a self-managed instance where the flag does not
			// persist. Both 200 (merge train enabled) and 400 (routing OK
			// but feature off) are acceptable for this assertion.
			if isHTTPStatus(err, 400) {
				t.Logf("merge train add returned 400 (merge trains disabled on this project — routing validated): %v", err)
				return
			}
			requirePremiumFeature(t, err, "merge train add")
		}
		requireTruef(t, out.ID > 0, "merge train entry ID should be > 0")
		t.Logf("Added MR !%d to merge train (entry ID %d)", mrIID, out.ID)
	})

	t.Run("Meta/MergeTrain/ListBranch", func(t *testing.T) {
		if mrIID == 0 {
			t.Skip("no MR was added to the train (previous sub-test failed)")
		}
		out, err := callToolOn[mergetrains.ListOutput](ctx, sess.meta, "gitlab_merge_train", map[string]any{
			"action": "list_branch",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"target_branch": defaultBranch,
			},
		})
		requireNoError(t, err, "list merge train for branch")
		t.Logf("Merge train for branch %q has %d entries", defaultBranch, len(out.Trains))
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

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	startDate := time.Now().UTC().AddDate(0, -1, 0).Format("2006-01-02")
	endDate := time.Now().UTC().Format("2006-01-02")

	grpName := uniqueName("dora-metrics")
	grpOut, setupErr := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "create",
		"params": map[string]any{
			"name":       grpName,
			"path":       grpName,
			"visibility": "private",
		},
	})
	requireNoError(t, setupErr, "create group for DORA metrics")
	groupIDStr := strconv.FormatInt(grpOut.ID, 10)
	defer func() {
		_ = callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "delete",
			"params": map[string]any{"group_id": groupIDStr},
		})
	}()

	proj := createProjectMeta(ctx, t, sess.meta)

	t.Run("Meta/DORA/Project", func(t *testing.T) {
		out, err := callToolOn[dorametrics.Output](ctx, sess.meta, "gitlab_dora_metrics", map[string]any{
			"action": "project",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"metric":     "deployment_frequency",
				"start_date": startDate,
				"end_date":   endDate,
				"interval":   "daily",
			},
		})
		requirePremiumFeature(t, err, "DORA metrics")
		t.Logf("Project DORA metrics: %d", len(out.Metrics))
	})

	t.Run("Meta/DORA/ProjectInvalidEnvironmentTiers", func(t *testing.T) {
		_, err := callToolOn[dorametrics.Output](ctx, sess.meta, "gitlab_dora_metrics", map[string]any{
			"action": "project",
			"params": map[string]any{
				"project_id":        proj.pidStr(),
				"metric":            "deployment_frequency",
				"start_date":        startDate,
				"end_date":          endDate,
				"interval":          "daily",
				"environment_tiers": []string{"production"},
			},
		})
		requireErrorContainsAll(t, err, "environment_tiers", "omit environment_tiers", "deployment environment tiers")
	})

	t.Run("Meta/DORA/Group", func(t *testing.T) {
		out, err := callToolOn[dorametrics.Output](ctx, sess.meta, "gitlab_dora_metrics", map[string]any{
			"action": "group",
			"params": map[string]any{
				"group_id":   groupIDStr,
				"metric":     "lead_time_for_changes",
				"start_date": startDate,
				"end_date":   endDate,
				"interval":   "all",
			},
		})
		requirePremiumFeature(t, err, "DORA group metrics")
		t.Logf("Group DORA metrics: %d", len(out.Metrics))
	})
}

// TestMeta_Dependencies exercises dependency tools via the gitlab_dependency meta-tool.
// Requires GitLab Premium/Ultimate (GITLAB_ENTERPRISE=true).
func TestMeta_Dependencies(t *testing.T) {
	if !sess.enterprise {
		return
	}

	ctx, cancel := e2eTimeoutContext(180*time.Second, 420*time.Second)
	defer cancel()
	proj := createProjectMeta(ctx, t, sess.meta)

	t.Run("Meta/Dependency/List", func(t *testing.T) {
		out, err := callToolOn[dependencies.ListOutput](ctx, sess.meta, "gitlab_dependency", map[string]any{
			"action": "list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		requirePremiumFeature(t, err, "dependencies")
		t.Logf("Dependencies: %d", len(out.Dependencies))
	})

	t.Run("Meta/Dependency/ListFiltered", func(t *testing.T) {
		out, err := callToolOn[dependencies.ListOutput](ctx, sess.meta, "gitlab_dependency", map[string]any{
			"action": "list",
			"params": map[string]any{
				"project_id":      proj.pidStr(),
				"package_manager": "npm",
				"per_page":        10,
			},
		})
		requireNoError(t, err, "dependency list with package_manager filter")
		t.Logf("Filtered dependencies: %d", len(out.Dependencies))
	})

	t.Run("Meta/Dependency/ExportCreateInvalidPipeline", func(t *testing.T) {
		_, err := callToolOn[dependencies.ExportOutput](ctx, sess.meta, "gitlab_dependency", map[string]any{
			"action": "export_create",
			"params": map[string]any{"pipeline_id": int64(999999999)},
		})
		requireErrorContainsAll(t, err, "pipeline_id", "gitlab_pipeline", "dependency scanning", "SBOM")
	})

	t.Run("Meta/Dependency/ExportGetInvalidID", func(t *testing.T) {
		_, err := callToolOn[dependencies.ExportOutput](ctx, sess.meta, "gitlab_dependency", map[string]any{
			"action": "export_get",
			"params": map[string]any{"export_id": int64(999999999)},
		})
		requireErrorContainsAll(t, err, "export_id", "gitlab_create_dependency_list_export", "finished")
	})

	t.Run("Meta/Dependency/ExportDownloadInvalidID", func(t *testing.T) {
		_, err := callToolOn[dependencies.DownloadOutput](ctx, sess.meta, "gitlab_dependency", map[string]any{
			"action": "export_download",
			"params": map[string]any{"export_id": int64(999999999)},
		})
		requireErrorContainsAll(t, err, "export_id", "gitlab_get_dependency_list_export", "CycloneDX")
	})
}

// TestMeta_ExternalStatusChecks exercises external status check tools via
// the gitlab_external_status_check meta-tool.
// Requires GitLab Premium/Ultimate (GITLAB_ENTERPRISE=true).
func TestMeta_ExternalStatusChecks(t *testing.T) {
	if !sess.enterprise {
		return
	}

	ctx, cancel := e2eTimeoutContext(180*time.Second, 600*time.Second)
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
		if !setStatusAccepted {
			t.Logf("set_project_mr_status was rejected by GitLab; listed status remains %q", status)
			return
		}
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
	subjectDigest := "sha256:0000000000000000000000000000000000000000000000000000000000000000"

	t.Run("Meta/Attestation/List", func(t *testing.T) {
		out, err := callToolOn[attestations.ListOutput](ctx, sess.meta, "gitlab_attestation", map[string]any{
			"action": "list",
			"params": map[string]any{
				"project_id":     proj.pidStr(),
				"subject_digest": subjectDigest,
			},
		})
		requirePremiumFeature(t, err, "attestations")
		t.Logf("Attestations: %d", len(out.Attestations))
	})

	t.Run("Meta/Attestation/DownloadInvalidIID", func(t *testing.T) {
		_, err := callToolOn[attestations.DownloadOutput](ctx, sess.meta, "gitlab_attestation", map[string]any{
			"action": "download",
			"params": map[string]any{
				"project_id":      proj.pidStr(),
				"attestation_iid": int64(999999999),
			},
		})
		requireErrorContainsAll(t, err, "attestation_iid", "gitlab_attestation", "gitlab_list_attestations")
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
		out, err := callToolOn[compliancepolicy.Output](ctx, sess.meta, "gitlab_compliance_policy", map[string]any{
			"action": "get",
			"params": map[string]any{},
		})
		requirePremiumFeature(t, err, "compliance policy")
		if out.CSPNamespaceID == nil {
			t.Log("Compliance policy CSP namespace is not set")
			return
		}
		t.Logf("Compliance policy CSP namespace: %d", *out.CSPNamespaceID)
	})

	t.Run("Meta/CompliancePolicy/UpdateMissingNamespace", func(t *testing.T) {
		_, err := callToolOn[compliancepolicy.Output](ctx, sess.meta, "gitlab_compliance_policy", map[string]any{
			"action": "update",
			"params": map[string]any{},
		})
		requireErrorContainsAll(t, err, "csp_namespace_id", "required")
	})

	t.Run("Meta/CompliancePolicy/UpdateInvalidNamespace", func(t *testing.T) {
		_, err := callToolOn[compliancepolicy.Output](ctx, sess.meta, "gitlab_compliance_policy", map[string]any{
			"action": "update",
			"params": map[string]any{"csp_namespace_id": int64(999999999)},
		})
		requireErrorContainsAll(t, err, "csp_namespace_id", "top-level group", "lock")
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

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	aliasName := uniqueName("e2e-project-alias")
	var aliasCreated bool
	t.Cleanup(func() {
		if !aliasCreated {
			return
		}
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanCancel()
		_ = callToolVoidOn(cleanCtx, sess.meta, "gitlab_project_alias", map[string]any{
			"action": "delete",
			"params": map[string]any{"name": aliasName},
		})
	})

	t.Run("Meta/ProjectAlias/List", func(t *testing.T) {
		out, err := callToolOn[projectaliases.ListOutput](ctx, sess.meta, "gitlab_project_alias", map[string]any{
			"action": "list",
			"params": map[string]any{},
		})
		requirePremiumFeature(t, err, "project aliases")
		t.Logf("Project aliases before create: %d", len(out.Aliases))
	})

	t.Run("Meta/ProjectAlias/Create", func(t *testing.T) {
		out, err := callToolOn[projectaliases.Output](ctx, sess.meta, "gitlab_project_alias", map[string]any{
			"action": "create",
			"params": map[string]any{
				"name":       aliasName,
				"project_id": proj.ID,
			},
		})
		requirePremiumFeature(t, err, "project alias create")
		requireTruef(t, out.Name == aliasName, "project alias name = %q, want %q", out.Name, aliasName)
		requireTruef(t, out.ProjectID == proj.ID, "project alias project_id = %d, want %d", out.ProjectID, proj.ID)
		aliasCreated = true
	})

	t.Run("Meta/ProjectAlias/Get", func(t *testing.T) {
		out, err := callToolOn[projectaliases.Output](ctx, sess.meta, "gitlab_project_alias", map[string]any{
			"action": "get",
			"params": map[string]any{"name": aliasName},
		})
		requireNoError(t, err, "project alias get")
		requireTruef(t, out.Name == aliasName, "project alias name = %q, want %q", out.Name, aliasName)
		requireTruef(t, out.ProjectID == proj.ID, "project alias project_id = %d, want %d", out.ProjectID, proj.ID)
	})

	t.Run("Meta/ProjectAlias/ListIncludesCreated", func(t *testing.T) {
		out, err := callToolOn[projectaliases.ListOutput](ctx, sess.meta, "gitlab_project_alias", map[string]any{
			"action": "list",
			"params": map[string]any{},
		})
		requireNoError(t, err, "project alias list after create")
		requireTruef(t, projectAliasListed(out.Aliases, aliasName, proj.ID), "created project alias %q not present in list", aliasName)
	})

	t.Run("Meta/ProjectAlias/Delete", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_project_alias", map[string]any{
			"action": "delete",
			"params": map[string]any{"name": aliasName},
		})
		requireNoError(t, err, "project alias delete")
		aliasCreated = false
	})

	t.Run("Meta/ProjectAlias/GetDeleted", func(t *testing.T) {
		_, err := callToolOn[projectaliases.Output](ctx, sess.meta, "gitlab_project_alias", map[string]any{
			"action": "get",
			"params": map[string]any{"name": aliasName},
		})
		requireErrorContainsAll(t, err, "alias", "gitlab_list_project_aliases")
	})
}

func projectAliasListed(aliases []projectaliases.Output, name string, projectID int64) bool {
	for _, alias := range aliases {
		if alias.Name == name && alias.ProjectID == projectID {
			return true
		}
	}
	return false
}

// TestMeta_Geo exercises Geo site tools via the gitlab_geo meta-tool.
// Requires GitLab Premium/Ultimate (GITLAB_ENTERPRISE=true).
func TestMeta_Geo(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	name := uniqueName("e2e-geo-site")
	updatedName := uniqueName("e2e-geo-site-updated")
	url := "https://geo-secondary.example.com/"
	var siteID int64
	t.Cleanup(func() {
		if siteID == 0 {
			return
		}
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanCancel()
		_ = callToolVoidOn(cleanCtx, sess.meta, "gitlab_geo", map[string]any{
			"action": "delete",
			"params": map[string]any{"id": siteID},
		})
	})

	t.Run("Meta/Geo/List", func(t *testing.T) {
		out, err := callToolOn[geo.ListOutput](ctx, sess.meta, "gitlab_geo", map[string]any{
			"action": "list",
			"params": map[string]any{},
		})
		requirePremiumFeature(t, err, "Geo sites")
		t.Logf("Geo sites before create: %d", len(out.Sites))
	})

	t.Run("Meta/Geo/Create", func(t *testing.T) {
		out, err := callToolOn[geo.Output](ctx, sess.meta, "gitlab_geo", map[string]any{
			"action": "create",
			"params": map[string]any{
				"name":         name,
				"url":          url,
				"internal_url": url,
				"enabled":      false,
			},
		})
		requirePremiumFeature(t, err, "Geo site create")
		requireTruef(t, out.ID > 0, "Geo site ID should be positive")
		requireTruef(t, out.Name == name, "Geo site name = %q, want %q", out.Name, name)
		requireTruef(t, !out.Enabled, "Geo site should be disabled")
		siteID = out.ID
	})

	t.Run("Meta/Geo/Get", func(t *testing.T) {
		requireTruef(t, siteID > 0, "siteID not set")
		out, err := callToolOn[geo.Output](ctx, sess.meta, "gitlab_geo", map[string]any{
			"action": "get",
			"params": map[string]any{"id": siteID},
		})
		requireNoError(t, err, "Geo site get")
		requireTruef(t, out.ID == siteID, "Geo site ID = %d, want %d", out.ID, siteID)
		requireTruef(t, out.Name == name, "Geo site name = %q, want %q", out.Name, name)
	})

	t.Run("Meta/Geo/ListIncludesCreated", func(t *testing.T) {
		requireTruef(t, siteID > 0, "siteID not set")
		out, err := callToolOn[geo.ListOutput](ctx, sess.meta, "gitlab_geo", map[string]any{
			"action": "list",
			"params": map[string]any{},
		})
		requireNoError(t, err, "Geo site list after create")
		requireTruef(t, geoSiteListed(out.Sites, siteID), "created Geo site %d not present in list", siteID)
	})

	t.Run("Meta/Geo/Edit", func(t *testing.T) {
		requireTruef(t, siteID > 0, "siteID not set")
		out, err := callToolOn[geo.Output](ctx, sess.meta, "gitlab_geo", map[string]any{
			"action": "edit",
			"params": map[string]any{
				"id":                 siteID,
				"name":               updatedName,
				"repos_max_capacity": int64(11),
			},
		})
		requireNoError(t, err, "Geo site edit")
		requireTruef(t, out.ID == siteID, "Geo site ID = %d, want %d", out.ID, siteID)
		requireTruef(t, out.Name == updatedName, "Geo site name = %q, want %q", out.Name, updatedName)
		requireTruef(t, out.ReposMaxCapacity == 11, "Geo site repos_max_capacity = %d, want 11", out.ReposMaxCapacity)
	})

	t.Run("Meta/Geo/ListStatus", func(t *testing.T) {
		out, err := callToolOn[geo.ListStatusOutput](ctx, sess.meta, "gitlab_geo", map[string]any{
			"action": "list_status",
			"params": map[string]any{},
		})
		requireNoError(t, err, "Geo site list_status")
		t.Logf("Geo statuses: %d", len(out.Statuses))
	})

	t.Run("Meta/Geo/GetStatusNotReported", func(t *testing.T) {
		requireTruef(t, siteID > 0, "siteID not set")
		_, err := callToolOn[geo.StatusOutput](ctx, sess.meta, "gitlab_geo", map[string]any{
			"action": "get_status",
			"params": map[string]any{"id": siteID},
		})
		requireErrorContainsAll(t, err, "reported status", "gitlab_list_geo_sites")
	})

	t.Run("Meta/Geo/Repair", func(t *testing.T) {
		requireTruef(t, siteID > 0, "siteID not set")
		out, err := callToolOn[geo.Output](ctx, sess.meta, "gitlab_geo", map[string]any{
			"action": "repair",
			"params": map[string]any{"id": siteID},
		})
		requireNoError(t, err, "Geo site repair")
		requireTruef(t, out.ID == siteID, "Geo site repair ID = %d, want %d", out.ID, siteID)
	})

	t.Run("Meta/Geo/Delete", func(t *testing.T) {
		requireTruef(t, siteID > 0, "siteID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_geo", map[string]any{
			"action": "delete",
			"params": map[string]any{"id": siteID},
		})
		requireNoError(t, err, "Geo site delete")
		siteID = 0
	})
}

func geoSiteListed(sites []geo.Output, siteID int64) bool {
	for _, site := range sites {
		if site.ID == siteID {
			return true
		}
	}
	return false
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

	// Geo-less single-node Docker returns 404 for these endpoints, but the
	// tool still has to route the call correctly. Each sub-test asserts
	// either 404 (no Geo) or 422 (invalid payload).

	t.Run("Meta/StorageMove/GetProject_NotFound", func(t *testing.T) {
		_, err := callToolOn[projectstoragemoves.Output](ctx, sess.meta, "gitlab_storage_move", map[string]any{
			"action": "get_project",
			"params": map[string]any{"id": int64(999999)},
		})
		if err == nil {
			t.Skip("Geo may be configured")
		}
		if !isHTTPStatus(err, 404) {
			t.Fatalf("get_project error was not 404: %v", err)
		}
		t.Logf("get_project routing validated: %v", err)
	})

	t.Run("Meta/StorageMove/GetGroup_NotFound", func(t *testing.T) {
		_, err := callToolOn[groupstoragemoves.Output](ctx, sess.meta, "gitlab_storage_move", map[string]any{
			"action": "get_group",
			"params": map[string]any{"id": int64(999999)},
		})
		if err == nil {
			t.Skip("Geo may be configured")
		}
		if !isHTTPStatus(err, 404) {
			t.Fatalf("get_group error was not 404: %v", err)
		}
		t.Logf("get_group routing validated: %v", err)
	})

	t.Run("Meta/StorageMove/GetSnippet_NotFound", func(t *testing.T) {
		_, err := callToolOn[snippetstoragemoves.Output](ctx, sess.meta, "gitlab_storage_move", map[string]any{
			"action": "get_snippet",
			"params": map[string]any{"id": int64(999999)},
		})
		if err == nil {
			t.Skip("Geo may be configured")
		}
		if !isHTTPStatus(err, 404) {
			t.Fatalf("get_snippet error was not 404: %v", err)
		}
		t.Logf("get_snippet routing validated: %v", err)
	})

	t.Run("Meta/StorageMove/GetProjectForProject_NotFound", func(t *testing.T) {
		_, err := callToolOn[projectstoragemoves.Output](ctx, sess.meta, "gitlab_storage_move", map[string]any{
			"action": "get_project_for_project",
			"params": map[string]any{
				"project_id": proj.ID,
				"id":         int64(999999),
			},
		})
		if err == nil {
			t.Skip("Geo may be configured")
		}
		if !isHTTPStatus(err, 404) {
			t.Fatalf("get_project_for_project error was not 404: %v", err)
		}
		t.Logf("get_project_for_project routing validated: %v", err)
	})

	t.Run("Meta/StorageMove/GetGroupForGroup_NotFound", func(t *testing.T) {
		_, err := callToolOn[groupstoragemoves.Output](ctx, sess.meta, "gitlab_storage_move", map[string]any{
			"action": "get_group_for_group",
			"params": map[string]any{
				"group_id": groupID,
				"id":       int64(999999),
			},
		})
		if err == nil {
			t.Skip("Geo may be configured")
		}
		if !isHTTPStatus(err, 404) {
			t.Fatalf("get_group_for_group error was not 404: %v", err)
		}
		t.Logf("get_group_for_group routing validated: %v", err)
	})

	t.Run("Meta/StorageMove/GetSnippetForSnippet_NotFound", func(t *testing.T) {
		_, err := callToolOn[snippetstoragemoves.Output](ctx, sess.meta, "gitlab_storage_move", map[string]any{
			"action": "get_snippet_for_snippet",
			"params": map[string]any{
				"snippet_id": snippetID,
				"id":         int64(999999),
			},
		})
		if err == nil {
			t.Skip("Geo may be configured")
		}
		if !isHTTPStatus(err, 404) {
			t.Fatalf("get_snippet_for_snippet error was not 404: %v", err)
		}
		t.Logf("get_snippet_for_snippet routing validated: %v", err)
	})

	t.Run("Meta/StorageMove/ScheduleAllProject_NotFound", func(t *testing.T) {
		// schedule_all_project operates on the entire GitLab instance, not
		// a single project. Without Geo, GitLab returns 404 for these
		// endpoints; with invalid shard names it returns 400. We provide
		// non-existent storage names so the request is well formed and
		// the routing assertion is meaningful.
		_, err := callToolOn[projectstoragemoves.ScheduleAllOutput](ctx, sess.meta, "gitlab_storage_move", map[string]any{
			"action": "schedule_all_project",
			"params": map[string]any{
				"source_storage_name":      "e2e-missing-source",
				"destination_storage_name": "e2e-missing-destination",
			},
		})
		if err == nil {
			t.Skip("Geo may be configured")
		}
		if !isHTTPStatus(err, 400) && !isHTTPStatus(err, 404) && !isHTTPStatus(err, 422) {
			t.Fatalf("schedule_all_project error was not 400/404/422: %v", err)
		}
		t.Logf("schedule_all_project routing validated: %v", err)
	})

	t.Run("Meta/StorageMove/ScheduleAllGroup_NotFound", func(t *testing.T) {
		_, err := callToolOn[groupstoragemoves.ScheduleAllOutput](ctx, sess.meta, "gitlab_storage_move", map[string]any{
			"action": "schedule_all_group",
			"params": map[string]any{
				"source_storage_name":      "e2e-missing-source",
				"destination_storage_name": "e2e-missing-destination",
			},
		})
		if err == nil {
			t.Skip("Geo may be configured")
		}
		if !isHTTPStatus(err, 400) && !isHTTPStatus(err, 404) && !isHTTPStatus(err, 422) {
			t.Fatalf("schedule_all_group error was not 400/404/422: %v", err)
		}
		t.Logf("schedule_all_group routing validated: %v", err)
	})

	t.Run("Meta/StorageMove/ScheduleAllSnippet_NotFound", func(t *testing.T) {
		_, err := callToolOn[snippetstoragemoves.ScheduleAllOutput](ctx, sess.meta, "gitlab_storage_move", map[string]any{
			"action": "schedule_all_snippet",
			"params": map[string]any{
				"source_storage_name":      "e2e-missing-source",
				"destination_storage_name": "e2e-missing-destination",
			},
		})
		if err == nil {
			t.Skip("Geo may be configured")
		}
		if !isHTTPStatus(err, 400) && !isHTTPStatus(err, 404) && !isHTTPStatus(err, 422) {
			t.Fatalf("schedule_all_snippet error was not 400/404/422: %v", err)
		}
		t.Logf("schedule_all_snippet routing validated: %v", err)
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
		out, err := callToolOn[securityfindings.ListOutput](ctx, sess.meta, "gitlab_security_finding", map[string]any{
			"action": "list",
			"params": map[string]any{
				"project_path": proj.Path,
				"pipeline_iid": "1",
			},
		})
		requirePremiumFeature(t, err, "security findings")
		t.Logf("Security findings: %d", len(out.Findings))
	})

	t.Run("Meta/SecurityFinding/ListFiltered", func(t *testing.T) {
		out, err := callToolOn[securityfindings.ListOutput](ctx, sess.meta, "gitlab_security_finding", map[string]any{
			"action": "list",
			"params": map[string]any{
				"project_path": proj.Path,
				"pipeline_iid": "1",
				"severity":     []string{"HIGH", "CRITICAL"},
				"report_type":  []string{"SAST"},
				"first":        10,
			},
		})
		requireNoError(t, err, "security findings list with filters")
		t.Logf("Filtered security findings: %d", len(out.Findings))
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
		out, err := callToolOn[groupscim.ListOutput](ctx, sess.meta, "gitlab_group_scim", map[string]any{
			"action": "list",
			"params": map[string]any{
				"group_id": groupID,
			},
		})
		requirePremiumFeature(t, err, "Group SCIM")
		t.Logf("Group SCIM identities: %d", len(out.Identities))
	})

	missingUID := uniqueName("e2e-missing-scim-")

	t.Run("Meta/GroupSCIM/GetMissingUID", func(t *testing.T) {
		_, err := callToolOn[groupscim.Output](ctx, sess.meta, "gitlab_group_scim", map[string]any{
			"action": "get",
			"params": map[string]any{
				"group_id": groupID,
				"uid":      missingUID,
			},
		})
		requireErrorContainsAll(t, err, "uid", "gitlab_group_scim", "SCIM provisioning")
	})

	t.Run("Meta/GroupSCIM/UpdateMissingUID", func(t *testing.T) {
		_, err := callToolOn[groupscim.UpdateOutput](ctx, sess.meta, "gitlab_group_scim", map[string]any{
			"action": "update",
			"params": map[string]any{
				"group_id":   groupID,
				"uid":        missingUID,
				"extern_uid": uniqueName("e2e-new-scim-"),
			},
		})
		requireErrorContainsAll(t, err, "uid", "gitlab_group_scim", "SCIM provisioning")
	})

	t.Run("Meta/GroupSCIM/DeleteMissingUID", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group_scim", map[string]any{
			"action": "delete",
			"params": map[string]any{
				"group_id": groupID,
				"uid":      missingUID,
			},
		})
		requireErrorContainsAll(t, err, "uid", "gitlab_group_scim", "SCIM provisioning")
	})
}

// TestEnterpriseOrbit_NotRegisteredOnSelfManaged verifies that GitLab.com-only
// Orbit tools are omitted from self-managed Enterprise surfaces.
//
// The experimental GitLab Orbit Knowledge Graph is exposed only on
// gitlab.com. This test enumerates the expected meta-tool name
// (gitlab_orbit) and the six individual sub-tools
// (gitlab_orbit_status, gitlab_orbit_schema, gitlab_orbit_tools,
// gitlab_orbit_dsl, gitlab_orbit_query, gitlab_orbit_graph_status)
// and asserts that none of them is registered when running against
// a self-managed instance.
func TestEnterpriseOrbit_NotRegisteredOnSelfManaged(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}
	if strings.Contains(strings.ToLower(os.Getenv("GITLAB_URL")), "gitlab.com") {
		t.Skip("Orbit availability depends on GitLab.com Knowledge Graph feature flags")
	}

	const (
		orbitMetaTool   = "gitlab_orbit"
		orbitStatusTool = "gitlab_orbit_status"
		orbitSchemaTool = "gitlab_orbit_schema"
		orbitToolsTool  = "gitlab_orbit_tools"
		orbitDSLTool    = "gitlab_orbit_dsl"
		orbitQueryTool  = "gitlab_orbit_query"
		orbitGraphTool  = "gitlab_orbit_graph_status"
	)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	metaTools, err := sess.meta.ListTools(ctx, nil)
	requireNoError(t, err, "list meta tools")
	for _, tool := range metaTools.Tools {
		if tool.Name == orbitMetaTool {
			t.Fatalf("meta-tool %q should not be registered on self-managed Enterprise", orbitMetaTool)
		}
	}
	t.Logf("Meta-tool %q is correctly absent on self-managed Enterprise", orbitMetaTool)

	individualTools, err := sess.individual.ListTools(ctx, nil)
	requireNoError(t, err, "list individual tools")
	registeredOrbit := make(map[string]bool)
	for _, tool := range individualTools.Tools {
		registeredOrbit[tool.Name] = true
	}

	expectedOrbitIndividualTools := []string{
		orbitStatusTool,
		orbitSchemaTool,
		orbitToolsTool,
		orbitDSLTool,
		orbitQueryTool,
		orbitGraphTool,
	}
	for _, expected := range expectedOrbitIndividualTools {
		if registeredOrbit[expected] {
			t.Fatalf("individual tool %q should not be registered on self-managed Enterprise", expected)
		}
	}
	t.Logf("Confirmed none of the 6 expected gitlab_orbit_* individual tools are registered on self-managed Enterprise")

	// Defense in depth: any other tool starting with the orbit prefix
	// (including a future expansion) is also forbidden on self-managed.
	for name := range registeredOrbit {
		if strings.HasPrefix(name, "gitlab_orbit_") {
			t.Fatalf("unexpected gitlab_orbit_* tool %q registered on self-managed Enterprise", name)
		}
	}
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
		out, err := callToolOn[enterpriseusers.ListOutput](ctx, sess.meta, "gitlab_enterprise_user", map[string]any{
			"action": "list",
			"params": map[string]any{
				"group_id": groupID,
			},
		})
		requirePremiumFeature(t, err, "enterprise users")
		t.Logf("Enterprise users: %d", len(out.Users))
	})

	t.Run("Meta/EnterpriseUser/ListFiltered", func(t *testing.T) {
		out, err := callToolOn[enterpriseusers.ListOutput](ctx, sess.meta, "gitlab_enterprise_user", map[string]any{
			"action": "list",
			"params": map[string]any{
				"group_id":   groupID,
				"search":     "e2e-nonexistent-enterprise-user",
				"active":     true,
				"two_factor": "disabled",
				"per_page":   10,
			},
		})
		requireNoError(t, err, "enterprise users filtered list")
		t.Logf("Filtered enterprise users: %d", len(out.Users))
	})

	missingUserID := int64(999999999)

	t.Run("Meta/EnterpriseUser/GetMissingUser", func(t *testing.T) {
		_, err := callToolOn[enterpriseusers.Output](ctx, sess.meta, "gitlab_enterprise_user", map[string]any{
			"action": "get",
			"params": map[string]any{
				"group_id": groupID,
				"user_id":  missingUserID,
			},
		})
		requireErrorContainsAll(t, err, "user_id", "gitlab_enterprise_user", "enterprise namespace")
	})

	t.Run("Meta/EnterpriseUser/Disable2FAMissingUser", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_enterprise_user", map[string]any{
			"action": "disable_2fa",
			"params": map[string]any{
				"group_id": groupID,
				"user_id":  missingUserID,
			},
		})
		requireErrorContainsAll(t, err, "user_id", "gitlab_enterprise_user", "enterprise namespace")
	})

	t.Run("Meta/EnterpriseUser/DeleteMissingUser", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_enterprise_user", map[string]any{
			"action": "delete",
			"params": map[string]any{
				"group_id":    groupID,
				"user_id":     missingUserID,
				"hard_delete": false,
			},
		})
		requireErrorContainsAll(t, err, "user_id", "gitlab_enterprise_user", "irreversible")
	})
}
