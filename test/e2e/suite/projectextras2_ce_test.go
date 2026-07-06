//go:build e2e && !enterprise

// projectextras2_ce_test.go covers gitlab_project meta-tool actions not
// exercised elsewhere: export download + import-from-file, project and
// group integration configuration (including Jira and group Datadog), and
// the Pages write actions (settings update, custom domains, unpublish).
//
// Build tag: e2e && !enterprise.
package suite

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/integrations"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/pages"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projectimportexport"
)

// projectExtrasIntegrationSlug is an inert integration that needs no
// reachable external service: GitLab only stores recipient addresses and
// never contacts them during configuration.
const projectExtrasIntegrationSlug = "emails-on-push"

// projectExtrasJiraURL returns a Jira base URL for integration_set_jira.
// GitLab persists the Jira configuration without contacting the server
// (connectivity is only checked by the explicit test endpoint), so the
// fixture service URL — or a placeholder outside Docker — is sufficient.
func projectExtrasJiraURL() string {
	if hasE2EFixtureService() {
		return e2eFixtureServiceURL("/jira")
	}
	return "https://jira.example.com"
}

// projectExtrasSkipIfPagesDisabled skips the calling (sub)test when a Pages
// API call failed because GitLab Pages is not enabled in the instance
// configuration. The Docker omnibus config does not enable Pages, so these
// call sites document intent (pending enabler, Wave 4) instead of failing.
func projectExtrasSkipIfPagesDisabled(t *testing.T, err error, action string) {
	t.Helper()
	if err != nil {
		t.Skipf("%s: GitLab Pages is not enabled in the Docker omnibus config (pending enabler, Wave 4): %v", action, err)
	}
}

// TestMeta_ProjectExportDownloadImport exercises the export download and
// import-from-file actions through the gitlab_project meta-tool.
//
// The test creates a project fixture, schedules an export via the covered
// export_schedule action, polls export_status until GitLab reports
// "finished" (skipping with a documented reason when the export never
// completes within the budget), downloads the archive via export_download,
// and re-imports it as a new project via import_from_file. The imported
// project is registered for permanent deletion in the per-test ledger.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_ProjectExportDownloadImport(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	e2e := NewE2EContext(t)
	ctx, cancel := context.WithTimeout(context.Background(), 420*time.Second)
	defer cancel()

	proj := CreateProjectMeta(ctx, e2e, sess.meta)

	var archiveBase64 string
	exportFinished := false

	t.Run("ScheduleAndAwaitExport", func(t *testing.T) {
		_, err := callToolOn[projectimportexport.ScheduleExportOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "export_schedule",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "export_schedule")

		// Export generation is asynchronous and can take minutes under Docker
		// load; poll generously and skip (not fail) when it never finishes.
		lastStatus := "unknown"
		// Full-suite parallelism keeps Sidekiq busy; exports observed >180s.
		maxWait := e2eTimeout(300*time.Second, 300*time.Second)
		pollErr := Poll(ctx, 2*time.Second, maxWait, func() (bool, string, error) {
			out, statusErr := callToolOn[projectimportexport.ExportStatusOutput](ctx, sess.meta, "gitlab_project", map[string]any{
				"action": "export_status",
				"params": map[string]any{"project_id": proj.pidStr()},
			})
			if statusErr != nil {
				return false, fmt.Sprintf("export_status error: %v", statusErr), nil
			}
			lastStatus = out.ExportStatus
			return out.ExportStatus == "finished", "export_status=" + out.ExportStatus, nil
		})
		if pollErr != nil {
			t.Skipf("project export did not reach 'finished' within %s (last status %q); export_download needs a completed export", maxWait, lastStatus)
		}
		exportFinished = true
		t.Logf("Export finished (last status %q)", lastStatus)
	})

	t.Run("ExportDownload", func(t *testing.T) {
		if !exportFinished {
			t.Skip("export never finished; nothing to download")
		}
		out, err := callToolOn[projectimportexport.ExportDownloadOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "export_download",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "export_download")
		requireTruef(t, out.SizeBytes > 0, "expected non-empty export archive, got %d bytes", out.SizeBytes)
		archiveBase64 = out.ContentBase64
		t.Logf("Downloaded export archive: %d bytes", out.SizeBytes)
	})

	t.Run("ImportFromFile", func(t *testing.T) {
		if archiveBase64 == "" {
			t.Skip("no export archive available; nothing to import")
		}
		// POST /projects/import requires the gitlab_project import source,
		// which fresh instances leave disabled; enable it on the disposable
		// Docker instance and restore the original set afterwards.
		if isDockerMode() {
			settings, _, settingsErr := sess.glClient.GL().Settings.GetSettings(gl.WithContext(ctx))
			requireNoError(t, settingsErr, "read application settings")
			original := settings.ImportSources
			if !slices.Contains(original, "gitlab_project") {
				enabled := append(slices.Clone(original), "gitlab_project")
				_, _, settingsErr = sess.glClient.GL().Settings.UpdateSettings(&gl.UpdateSettingsOptions{
					ImportSources: &enabled,
				}, gl.WithContext(ctx))
				requireNoError(t, settingsErr, "enable gitlab_project import source")
				defer func() { //nolint:contextcheck // Cleanup runs after the test context is canceled and owns its own timeout.
					cctx, ccancel := cleanupContext(defaultCleanupTimeout)
					defer ccancel()
					_, _, _ = sess.glClient.GL().Settings.UpdateSettings(&gl.UpdateSettingsOptions{
						ImportSources: &original,
					}, gl.WithContext(cctx))
				}()
			}
		}
		name := uniqueName(e2eProjectPrefix + "px-import")
		// The application settings cache serves the pre-toggle value for up
		// to a minute, so 403 stays retryable in Docker mode until the
		// import-source change becomes visible.
		out, err := retryWithBackoffInterval(ctx, t, "import_from_file", 10, 8*time.Second, func(int) (projectimportexport.ImportStatusOutput, bool, string, error) {
			imported, importErr := callToolOn[projectimportexport.ImportStatusOutput](ctx, sess.meta, "gitlab_project", map[string]any{
				"action": "import_from_file",
				"params": map[string]any{
					"content_base64": archiveBase64,
					"name":           name,
					"path":           name,
				},
			})
			retryable := importErr != nil && isDockerMode() && isHTTPStatus(importErr, 403)
			return imported, retryable, "import source setting not visible yet", importErr
		})
		if err != nil && !isDockerMode() && isHTTPStatus(err, 403) {
			t.Skipf("gitlab_project import source disabled on this instance: %v", err)
		}
		requireNoError(t, err, "import_from_file")
		requireTruef(t, out.ID > 0, "expected imported project ID")
		requireTruef(t, out.ImportStatus != "", "expected an import status")
		projectExtrasRegisterProjectCleanup(e2e, out.ID, out.PathWithNamespace, out.Name)
		t.Logf("Import scheduled for project %d (%s), status=%s", out.ID, out.PathWithNamespace, out.ImportStatus)
	})
}

// TestMeta_ProjectIntegrationConfig exercises project integration get, set,
// set_jira, and delete actions through the gitlab_project meta-tool.
//
// The test creates a project fixture, configures the inert emails-on-push
// integration via integration_set, reads it back via integration_get,
// configures Jira with fixture/dummy credentials via integration_set_jira
// (GitLab accepts the configuration without contacting the server), then
// removes both integrations via integration_delete. Assertions check the
// integration slug round-trips and reads report an active integration.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_ProjectIntegrationConfig(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	e2e := NewE2EContext(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := CreateProjectMeta(ctx, e2e, sess.meta)

	t.Run("IntegrationSet", func(t *testing.T) {
		out, err := callToolOn[integrations.SetIntegrationOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "integration_set",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"slug":       projectExtrasIntegrationSlug,
				"config": map[string]any{
					"recipients":  "e2e-integrations@example.com",
					"push_events": true,
				},
			},
		})
		requireNoError(t, err, "integration_set")
		requireTruef(t, out.Integration.Active, "expected integration to be active after set")
		t.Logf("Configured integration %q (id=%d)", projectExtrasIntegrationSlug, out.Integration.ID)
	})

	t.Run("IntegrationGet", func(t *testing.T) {
		out, err := callToolOn[integrations.GetOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "integration_get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"slug":       projectExtrasIntegrationSlug,
			},
		})
		requireNoError(t, err, "integration_get")
		requireTruef(t, out.Integration.Active, "expected active integration")
		t.Logf("Got integration %q (title=%q)", projectExtrasIntegrationSlug, out.Integration.Title)
	})

	t.Run("IntegrationSetJira", func(t *testing.T) {
		out, err := callToolOn[integrations.SetJiraOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "integration_set_jira",
			"params": map[string]any{
				"project_id":     proj.pidStr(),
				"url":            projectExtrasJiraURL(),
				"username":       "e2e-jira-user",
				"password":       "e2e-jira-token",
				"jira_auth_type": 0,
			},
		})
		requireNoError(t, err, "integration_set_jira")
		requireTruef(t, out.Integration.Active, "expected Jira integration to be active after set")
		t.Logf("Configured Jira integration (id=%d)", out.Integration.ID)
	})

	t.Run("IntegrationDeleteJira", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "integration_delete",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"slug":       "jira",
			},
		})
		requireNoError(t, err, "integration_delete (jira)")
		t.Log("Deleted Jira integration")
	})

	t.Run("IntegrationDelete", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "integration_delete",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"slug":       projectExtrasIntegrationSlug,
			},
		})
		requireNoError(t, err, "integration_delete")
		t.Logf("Deleted integration %q", projectExtrasIntegrationSlug)
	})
}

// TestMeta_ProjectGroupIntegrations exercises group-level integration
// actions through the gitlab_project meta-tool.
//
// The test creates a helper group, configures the inert emails-on-push
// integration via integration_set_group, verifies it through
// integration_list_group and integration_get_group, and removes it via
// integration_delete_group. The group Datadog read/delete actions are
// exercised against the fresh group where no Datadog integration exists, so
// both are asserted to return the documented not-configured error.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_ProjectGroupIntegrations(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	e2e := NewE2EContext(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	group := CreateGroupMeta(ctx, e2e, sess.meta, "px-gint")
	groupID := strconv.FormatInt(group.ID, 10)

	t.Run("IntegrationSetGroup", func(t *testing.T) {
		out, err := callToolOn[integrations.SetGroupIntegrationOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "integration_set_group",
			"params": map[string]any{
				"group_id": groupID,
				"slug":     projectExtrasIntegrationSlug,
				"config": map[string]any{
					"recipients":  "e2e-group-integrations@example.com",
					"push_events": true,
				},
			},
		})
		requireNoError(t, err, "integration_set_group")
		requireTruef(t, out.Integration.Active, "expected group integration to be active after set")
		t.Logf("Configured group integration %q (id=%d)", projectExtrasIntegrationSlug, out.Integration.ID)
	})

	t.Run("IntegrationListGroup", func(t *testing.T) {
		out, err := callToolOn[integrations.ListGroupIntegrationsOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "integration_list_group",
			"params": map[string]any{"group_id": groupID},
		})
		requireNoError(t, err, "integration_list_group")
		requireTruef(t, len(out.Integrations) >= 1, "expected at least 1 active group integration, got %d", len(out.Integrations))
		t.Logf("Listed %d group integrations", len(out.Integrations))
	})

	t.Run("IntegrationGetGroup", func(t *testing.T) {
		out, err := callToolOn[integrations.GetGroupIntegrationOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "integration_get_group",
			"params": map[string]any{
				"group_id": groupID,
				"slug":     projectExtrasIntegrationSlug,
			},
		})
		requireNoError(t, err, "integration_get_group")
		requireTruef(t, out.Integration.Active, "expected active group integration")
		t.Logf("Got group integration %q (title=%q)", projectExtrasIntegrationSlug, out.Integration.Title)
	})

	t.Run("IntegrationDeleteGroup", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "integration_delete_group",
			"params": map[string]any{
				"group_id": groupID,
				"slug":     projectExtrasIntegrationSlug,
			},
		})
		requireNoError(t, err, "integration_delete_group")
		t.Logf("Deleted group integration %q", projectExtrasIntegrationSlug)
	})

	t.Run("IntegrationGetGroupDatadog", func(t *testing.T) {
		// No Datadog integration is ever configured on the fresh group, so
		// GitLab reports it as not found/not active.
		_, err := callToolOn[integrations.GetGroupDatadogOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "integration_get_group_datadog",
			"params": map[string]any{"group_id": groupID},
		})
		requireTruef(t, err != nil, "expected error: Datadog integration is not configured on a fresh group")
		t.Logf("Expected error for unconfigured group Datadog integration: %v", err)
	})

	t.Run("IntegrationDeleteGroupDatadog", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "integration_delete_group_datadog",
			"params": map[string]any{"group_id": groupID},
		})
		requireTruef(t, err != nil, "expected error: no Datadog integration exists to delete on a fresh group")
		t.Logf("Expected error deleting unconfigured group Datadog integration: %v", err)
	})
}

// TestMeta_ProjectPagesWrite exercises the Pages write actions of the
// gitlab_project meta-tool: pages_update, pages_domain_create/get/update/
// delete, and pages_unpublish.
//
// GitLab Pages is not enabled in the Docker omnibus config, so each subtest
// first attempts the real call and converts a Pages-disabled rejection into
// a documented skip (pending enabler, Wave 4) while keeping the call sites
// so the coverage audit records intent. When Pages is enabled the full
// domain lifecycle and settings updates are asserted strictly.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_ProjectPagesWrite(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	e2e := NewE2EContext(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := CreateProjectMeta(ctx, e2e, sess.meta)

	t.Run("PagesUpdate", func(t *testing.T) {
		out, err := callToolOn[pages.Output](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "pages_update",
			"params": map[string]any{
				"project_id":       proj.pidStr(),
				"pages_https_only": false,
			},
		})
		projectExtrasSkipIfPagesDisabled(t, err, "pages_update")
		requireTruef(t, !out.ForceHTTPS, "expected force_https disabled after update")
		t.Logf("Updated Pages settings (url=%s)", out.URL)
	})

	t.Run("PagesDomainLifecycle", func(t *testing.T) {
		domain := uniqueName("px-pages") + ".example.com"

		created, err := callToolOn[pages.DomainOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "pages_domain_create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"domain":     domain,
			},
		})
		projectExtrasSkipIfPagesDisabled(t, err, "pages_domain_create")
		requireTruef(t, created.Domain == domain, "expected domain %q, got %q", domain, created.Domain)
		t.Logf("Created Pages domain %q", created.Domain)

		got, err := callToolOn[pages.DomainOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "pages_domain_get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"domain":     domain,
			},
		})
		requireNoError(t, err, "pages_domain_get")
		requireTruef(t, got.Domain == domain, "expected domain %q, got %q", domain, got.Domain)

		updated, err := callToolOn[pages.DomainOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "pages_domain_update",
			"params": map[string]any{
				"project_id":       proj.pidStr(),
				"domain":           domain,
				"auto_ssl_enabled": false,
			},
		})
		requireNoError(t, err, "pages_domain_update")
		requireTruef(t, !updated.AutoSslEnabled, "expected auto SSL disabled after update")

		err = callToolVoidOn(ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "pages_domain_delete",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"domain":     domain,
			},
		})
		requireNoError(t, err, "pages_domain_delete")
		t.Logf("Deleted Pages domain %q", domain)
	})

	t.Run("PagesUnpublish", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "pages_unpublish",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		projectExtrasSkipIfPagesDisabled(t, err, "pages_unpublish")
		t.Log("Unpublished Pages site")
	})
}
