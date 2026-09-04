//go:build e2e && enterprise

// security_scan_profiles_ee_test.go exercises the security scan profile
// meta-tool against a live GitLab EE Ultimate instance. Security scan
// profiles are an Ultimate-only GraphQL feature (client-go v2.45.0).
//
// GitLab requirements exercised here (verified against GitLab EE 19.0.1):
//   - The target project/group must belong to a GROUP namespace, not a
//     personal namespace — attaching to a personal-namespace project raises
//     a server error inside GitLab's shared_root_namespace! resolver.
//   - Attach accepts a scan-type identifier (e.g. "dependency_scanning"),
//     which find-or-creates the namespace's default scan profile on the fly.
//   - Detach requires the persisted profile's numeric ID, discovered from
//     list_project_statuses after the attach.
//   - The security_scan_profiles_feature and
//     security_scan_profiles_dependency_scanning feature flags must be
//     enabled (setup-gitlab.sh enables them for EE runs).
//
// Like its EE meta-tool peers, the top-level test calls t.Parallel(); the
// shared meta session is safe for concurrent tool calls.
package suite

import (
	"context"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/securityscanprofiles"
)

// TestMeta_SecurityScanProfiles exercises the full attach → list → detach scan
// profile lifecycle through the gitlab_security_scan_profile meta-tool against a
// live GitLab EE Ultimate instance.
//
// The test creates a group and a project inside it (scan profiles require a
// group namespace), attaches the built-in dependency_scanning profile, reads
// the resulting per-project status to discover the persisted profile ID, and
// detaches it. When the running instance does not expose the scan profile
// GraphQL surface (feature disabled or an older EE version), the test skips
// rather than fails.
//
// Build tag: e2e && enterprise. Mode: EE. Surface: meta. Requires Ultimate.
func TestMeta_SecurityScanProfiles(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx := context.Background()
	e2e := NewE2EContext(t)

	// Scan profiles can only be attached to targets under a group namespace,
	// so create a group and a project inside it.
	grp := CreateGroupMeta(ctx, e2e, sess.meta, "e2e-scan-profile")
	proj, err := callToolOn[projects.Output](ctx, sess.meta, "gitlab_project", map[string]any{
		"action": "create",
		"params": map[string]any{
			"name":                   uniqueName("scan-profile-proj"),
			"namespace_id":           grp.ID,
			"visibility":             "private",
			"initialize_with_readme": true,
			"default_branch":         defaultBranch,
		},
	})
	requireNoError(t, err, "create project in group for scan profile test")
	t.Logf("Group %d, project %d (%s)", grp.ID, proj.ID, proj.PathWithNamespace)

	t.Run("Meta/SecurityScanProfile/Attach", func(t *testing.T) {
		out, attachErr := callToolOn[securityscanprofiles.MutationOutput](ctx, sess.meta, "gitlab_security_scan_profile", map[string]any{
			"action": "attach",
			"params": map[string]any{
				// A scan-type identifier find-or-creates the default profile.
				"security_scan_profile_id": "dependency_scanning",
				"project_ids":              []int64{proj.ID},
			},
		})
		if attachErr != nil {
			if isScanProfileFeatureUnavailable(attachErr) {
				t.Skipf("security scan profile attach unavailable on this instance: %v", attachErr)
			}
			requireNoError(t, attachErr, "security_scan_profile attach")
		}
		requireTruef(t, out.Status == "success", "attach status = %q, want success", out.Status)
		t.Logf("Attached dependency_scanning profile to project %d", proj.ID)
	})

	// Discover the persisted profile ID from the project's statuses; detach
	// requires the numeric/global profile ID, not the scan-type identifier.
	var profileID string
	t.Run("Meta/SecurityScanProfile/ListProjectStatuses", func(t *testing.T) {
		out, listErr := callToolOn[securityscanprofiles.ListProjectStatusesOutput](ctx, sess.meta, "gitlab_security_scan_profile", map[string]any{
			"action": "list_project_statuses",
			"params": map[string]any{
				"project_full_path": proj.PathWithNamespace,
			},
		})
		if listErr != nil {
			if isScanProfileFeatureUnavailable(listErr) {
				t.Skipf("security scan profile status query unavailable on this instance: %v", listErr)
			}
			requireNoError(t, listErr, "security_scan_profile list_project_statuses")
		}
		requireTruef(t, out.ProjectFullPath == proj.PathWithNamespace,
			"echoed project_full_path = %q, want %q", out.ProjectFullPath, proj.PathWithNamespace)
		for _, s := range out.Statuses {
			if strings.EqualFold(s.ScanProfile.ScanType, "dependency_scanning") {
				profileID = s.ScanProfile.ID
			}
		}
		requireTruef(t, profileID != "", "attached dependency_scanning profile not found in statuses: %+v", out.Statuses)
		t.Logf("Project %s reports %d scan profile status(es); dependency_scanning profile ID = %s",
			proj.PathWithNamespace, len(out.Statuses), profileID)
	})

	t.Run("Meta/SecurityScanProfile/Detach", func(t *testing.T) {
		requireTruef(t, profileID != "", "profileID not set (ListProjectStatuses must run first)")
		out, detachErr := callToolOn[securityscanprofiles.MutationOutput](ctx, sess.meta, "gitlab_security_scan_profile", map[string]any{
			"action": "detach",
			"params": map[string]any{
				"security_scan_profile_id": profileID,
				"project_ids":              []int64{proj.ID},
			},
		})
		requireNoError(t, detachErr, "security_scan_profile detach")
		requireTruef(t, out.Status == "success", "detach status = %q, want success", out.Status)
		t.Logf("Detached scan profile %s from project %d", profileID, proj.ID)
	})
}

// isScanProfileFeatureUnavailable reports whether err indicates the GitLab
// instance does not expose the security scan profile GraphQL surface, as
// opposed to a real failure worth failing the suite over. GitLab returns a
// GraphQL field/type error when the feature or query is unavailable.
func isScanProfileFeatureUnavailable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, marker := range []string{
		"scanprofilestatuses",
		"doesn't exist on type",
		"field 'scanprofilestatuses'",
		"not available",
		"undefined field",
		"cannot query field",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}
