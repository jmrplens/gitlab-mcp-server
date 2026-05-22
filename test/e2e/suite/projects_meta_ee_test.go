//go:build e2e && enterprise

// projects_meta_ee_test.go tests Enterprise project meta-tool actions.
package suite

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projectserviceaccounts"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/securitysettings"
)

// TestMeta_ProjectServiceAccounts exercises project service account CRUD and PAT
// management through the gitlab_project meta-tool. Project service accounts are
// Premium/Ultimate-only, so CE runs return before making GitLab calls.
func TestMeta_ProjectServiceAccounts(t *testing.T) {
	if !sess.enterprise {
		t.Parallel()
	}
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	if !sess.enterprise {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	var serviceAccountID int64
	var tokenID int64

	t.Run("ServiceAccountListEmpty", func(t *testing.T) {
		out, err := callToolOn[projectserviceaccounts.ListOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "service_account_list",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "project service_account_list (empty)")
		requireTruef(t, len(out.Accounts) == 0, "expected 0 service accounts, got %d", len(out.Accounts))
	})

	t.Run("ServiceAccountCreate", func(t *testing.T) {
		serviceAccountName := uniqueName("sa-proj")
		out, err := callToolOn[projectserviceaccounts.Output](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "service_account_create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"name":       serviceAccountName,
				"username":   serviceAccountName,
			},
		})
		requireNoError(t, err, "project service_account_create")
		requireTruef(t, out.ID > 0, "expected service account ID > 0")
		serviceAccountID = out.ID
		t.Logf("Created project service account %d", serviceAccountID)
	})

	t.Run("ServiceAccountUpdate", func(t *testing.T) {
		requireTruef(t, serviceAccountID > 0, "serviceAccountID not set")
		out, err := callToolOn[projectserviceaccounts.Output](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "service_account_update",
			"params": map[string]any{
				"project_id":         proj.pidStr(),
				"service_account_id": serviceAccountID,
				"name":               "Updated Project Service Account",
			},
		})
		requireNoError(t, err, "project service_account_update")
		requireTruef(t, out.ID == serviceAccountID, "service account ID mismatch: got %d want %d", out.ID, serviceAccountID)
	})

	t.Run("ServiceAccountListOne", func(t *testing.T) {
		out, err := callToolOn[projectserviceaccounts.ListOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "service_account_list",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "project service_account_list (one)")
		requireTruef(t, len(out.Accounts) >= 1, "expected at least 1 service account, got %d", len(out.Accounts))
	})

	t.Run("ServiceAccountPATCreate", func(t *testing.T) {
		requireTruef(t, serviceAccountID > 0, "serviceAccountID not set")
		expiresAt := time.Now().AddDate(0, 0, 7).Format("2006-01-02")
		out, err := callToolOn[projectserviceaccounts.PATOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "service_account_pat_create",
			"params": map[string]any{
				"project_id":         proj.pidStr(),
				"service_account_id": serviceAccountID,
				"name":               "e2e-project-pat",
				"scopes":             []string{"api"},
				"expires_at":         expiresAt,
			},
		})
		requireNoError(t, err, "project service_account_pat_create")
		requireTruef(t, out.ID > 0, "expected PAT ID > 0")
		tokenID = out.ID
		t.Logf("Created project service account PAT %d", tokenID)
	})

	t.Run("ServiceAccountPATList", func(t *testing.T) {
		requireTruef(t, serviceAccountID > 0, "serviceAccountID not set")
		out, err := callToolOn[projectserviceaccounts.ListPATOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "service_account_pat_list",
			"params": map[string]any{
				"project_id":         proj.pidStr(),
				"service_account_id": serviceAccountID,
			},
		})
		requireNoError(t, err, "project service_account_pat_list")
		requireTruef(t, len(out.Tokens) >= 1, "expected at least 1 PAT, got %d", len(out.Tokens))
	})

	t.Run("ServiceAccountPATRotate", func(t *testing.T) {
		requireTruef(t, serviceAccountID > 0, "serviceAccountID not set")
		requireTruef(t, tokenID > 0, "tokenID not set")
		expiresAt := time.Now().AddDate(0, 0, 14).Format("2006-01-02")
		out, err := callToolOn[projectserviceaccounts.PATOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "service_account_pat_rotate",
			"params": map[string]any{
				"project_id":         proj.pidStr(),
				"service_account_id": serviceAccountID,
				"token_id":           tokenID,
				"expires_at":         expiresAt,
			},
		})
		requireNoError(t, err, "project service_account_pat_rotate")
		requireTruef(t, out.ID > 0, "expected rotated PAT ID > 0")
		tokenID = out.ID
		t.Logf("Rotated project service account PAT to %d", tokenID)
	})

	t.Run("ServiceAccountPATRevoke", func(t *testing.T) {
		requireTruef(t, serviceAccountID > 0, "serviceAccountID not set")
		requireTruef(t, tokenID > 0, "tokenID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "service_account_pat_revoke",
			"params": map[string]any{
				"project_id":         proj.pidStr(),
				"service_account_id": serviceAccountID,
				"token_id":           tokenID,
			},
		})
		requireNoError(t, err, "project service_account_pat_revoke")
	})

	t.Run("ServiceAccountDelete", func(t *testing.T) {
		requireTruef(t, serviceAccountID > 0, "serviceAccountID not set")
		hardDelete := true
		err := callToolVoidOn(ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "service_account_delete",
			"params": map[string]any{
				"project_id":         proj.pidStr(),
				"service_account_id": serviceAccountID,
				"hard_delete":        hardDelete,
			},
		})
		requireNoError(t, err, "project service_account_delete")
	})
}

// TestMeta_ProjectSecuritySettings exercises Ultimate project security settings
// get/update actions through the gitlab_project meta-tool.
func TestMeta_ProjectSecuritySettings(t *testing.T) {
	if !sess.enterprise {
		t.Parallel()
	}
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	if !sess.enterprise {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	t.Run("SecuritySettingsGet", func(t *testing.T) {
		out, err := callToolOn[securitysettings.ProjectOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "security_settings_get",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "project security_settings_get")
		requireTruef(t, out.ProjectID == proj.ID, "unexpected project ID in security settings: got %d want %d", out.ProjectID, proj.ID)
	})

	t.Run("SecuritySettingsUpdate", func(t *testing.T) {
		out, err := callToolOn[securitysettings.ProjectOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "security_settings_update",
			"params": map[string]any{
				"project_id":                     proj.pidStr(),
				"secret_push_protection_enabled": true,
			},
		})
		requireNoError(t, err, "project security_settings_update")
		requireTruef(t, out.ProjectID == proj.ID, "unexpected project ID in updated security settings: got %d want %d", out.ProjectID, proj.ID)
		requireTruef(t, out.SecretPushProtectionEnabled, "expected secret push protection enabled after update")
	})
}

// TestMeta_ProjectMirroring tests pull mirror and housekeeping actions.
func TestMeta_ProjectMirroring(t *testing.T) {
	if !sess.enterprise {
		t.Parallel()
	}
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	if !sess.enterprise {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	upstream := createProjectMirrorSourceMeta(ctx, t)
	proj := createProjectMeta(ctx, t, sess.meta)
	mirrorURL := projectMirrorSourceURL(upstream)

	t.Run("PullMirrorGet", func(t *testing.T) {
		_, err := callToolOn[projects.PullMirrorOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "pull_mirror_get",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireErrorContainsAll(t, err, "not mirrored", "pull_mirror_configure", "pull_mirror_get")
		t.Logf("Expected error for pull_mirror_get without mirror: %v", err)
	})

	t.Run("PullMirrorConfigure", func(t *testing.T) {
		out, err := callToolOn[projects.PullMirrorOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "pull_mirror_configure",
			"params": map[string]any{
				"project_id":                          proj.pidStr(),
				"enabled":                             true,
				"url":                                 mirrorURL,
				"mirror_trigger_builds":               false,
				"only_mirror_protected_branches":      false,
				"mirror_overwrites_diverged_branches": true,
			},
		})
		requirePremiumFeature(t, err, "pull mirroring")
		requireTruef(t, out.Enabled, "pull mirror should be enabled")
		requireTruef(t, strings.Contains(out.URL, upstream.Path), "mirror URL %q should reference upstream %q", out.URL, upstream.Path)
	})

	t.Run("PullMirrorGetConfigured", func(t *testing.T) {
		out, err := callToolOn[projects.PullMirrorOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "pull_mirror_get",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "pull mirror get after configure")
		requireTruef(t, out.Enabled, "configured pull mirror should be enabled")
		requireTruef(t, strings.Contains(out.URL, upstream.Path), "mirror URL %q should reference upstream %q", out.URL, upstream.Path)
	})

	t.Run("StartMirroring", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "start_mirroring",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "start pull mirroring")
	})

	t.Run("PullMirrorDisable", func(t *testing.T) {
		out, err := callToolOn[projects.PullMirrorOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "pull_mirror_configure",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"enabled":    false,
			},
		})
		requireNoError(t, err, "disable pull mirroring")
		requireTruef(t, !out.Enabled, "pull mirror should be disabled")
	})
}

func createProjectMirrorSourceMeta(ctx context.Context, t *testing.T) projects.Output {
	t.Helper()
	name := uniqueName("mirror-source")
	out, err := callToolOn[projects.Output](ctx, sess.meta, "gitlab_project", map[string]any{
		"action": "create",
		"params": map[string]any{
			"name":                   name,
			"description":            "E2E public pull mirror source: " + t.Name(),
			"visibility":             "public",
			"initialize_with_readme": true,
			"default_branch":         defaultBranch,
		},
	})
	requireNoError(t, err, "create public mirror source project")
	//nolint:contextcheck // Cleanup must outlive the test context; uses a fresh background ctx with timeout.
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_ = callToolVoidOn(cleanupCtx, sess.meta, "gitlab_project", map[string]any{
			"action": "delete",
			"params": map[string]any{
				"project_id":         strconv.FormatInt(out.ID, 10),
				"permanently_remove": true,
				"full_path":          out.PathWithNamespace,
			},
		})
	})
	waitForBranchOn(ctx, t, sess.glClient, out.ID, defaultBranch)
	return out
}

func projectMirrorSourceURL(project projects.Output) string {
	baseURL := strings.TrimRight(os.Getenv("E2E_GITLAB_INTERNAL_URL"), "/")
	if baseURL == "" {
		return project.HTTPURLToRepo
	}
	return baseURL + "/" + project.PathWithNamespace + ".git"
}
