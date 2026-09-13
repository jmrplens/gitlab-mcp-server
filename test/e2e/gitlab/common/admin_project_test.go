//go:build e2e

// admin_project_test.go ports the project- and instance-scoped half of the old
// admin extras suite: CI secure files, integrated error tracking, alert metric
// images, Terraform state, the usage-data endpoints, the schema-migration mark
// and the direct-transfer bulk import family.
//
// Every scenario runs through the gitlab_admin meta tool on the meta surface,
// the surface the old suite drove and the one the run's administrator token
// serves the group on. Several need state no MCP action can create, seeded
// through client-go by the fixture library: a Terraform state pushed through
// the raw backend, an alert fired through the Free HTTP integration. The
// direct transfer and the migration-mark happy path need the disposable Docker
// instance, and skip with a reason elsewhere.

package common

import (
	"context"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/alertmanagement"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/bulkimports"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/errortracking"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/features"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/securefiles"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/terraformstates"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/usagedata"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestAdmin_SecureFiles uploads a project CI secure file, lists and reads it
// back, then deletes it and checks the list is empty again.
//
// Replaces: TestMeta_AdminSecureFiles
func TestAdmin_SecureFiles(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))
	s := e.On(harness.SurfaceMeta)
	project := fixture.NewProject(e, fixture.WithNamePrefix("adm-secure"))

	name := e.Name("secure") + ".txt"
	created := harness.Do[securefiles.SecureFileItem](s, actionAdminSecureFileCreate, map[string]any{
		"project_id": project.IDParam(), "name": name, "content_base64": "ZTJlIHNlY3VyZSBmaWxlIHBheWxvYWQ=",
	})
	if created.ID == 0 || created.Name != name || created.Checksum == "" {
		e.T.Fatalf("secure_file_create answered %+v, want a file named %q with an ID and a checksum", created, name)
	}

	list := harness.Do[securefiles.ListOutput](s, actionAdminSecureFileList, map[string]any{"project_id": project.IDParam()})
	if len(list.Files) != 1 || list.Files[0].ID != created.ID {
		e.T.Errorf("secure_file_list answered %d file(s), want the created %d", len(list.Files), created.ID)
	}

	got := harness.Do[securefiles.SecureFileItem](s, actionAdminSecureFileGet, map[string]any{"project_id": project.IDParam(), "file_id": created.ID})
	if got.ID != created.ID || got.ChecksumAlgorithm != "sha256" {
		e.T.Errorf("secure_file_get answered %+v, want file %d with a sha256 checksum", got, created.ID)
	}

	harness.DoVoid(s, actionAdminSecureFileDelete, map[string]any{"project_id": project.IDParam(), "file_id": created.ID})
	after := harness.Do[securefiles.ListOutput](s, actionAdminSecureFileList, map[string]any{"project_id": project.IDParam()})
	if len(after.Files) != 0 {
		e.T.Errorf("secure_file_list answered %d file(s) after the delete, want none", len(after.Files))
	}
}

// TestAdmin_ErrorTracking walks the integrated error tracking family: the
// settings read and update answer the documented 404 on a fresh project that
// has no settings record, while the client-key lifecycle works regardless.
//
// Replaces: TestMeta_AdminErrorTracking
func TestAdmin_ErrorTracking(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))
	s := e.On(harness.SurfaceMeta)
	project := fixture.NewProject(e, fixture.WithNamePrefix("adm-errtrack"))
	params := map[string]any{"project_id": project.IDParam()}

	// Both endpoints answer not_found!('Error Tracking Setting') while the
	// project carries no settings record, which a fresh project never does.
	e.T.Logf("error_tracking_get_settings on a fresh project: %s",
		firstLine(harness.Refused(s, actionAdminErrorTrackingGetSettings, params, harness.FailureNotFound)))
	e.T.Logf("error_tracking_update_settings on a fresh project: %s",
		firstLine(harness.Refused(s, actionAdminErrorTrackingUpdateSettings,
			withParams(params, map[string]any{"active": true, "integrated": true}), harness.FailureNotFound)))

	key := harness.Do[errortracking.ClientKeyItem](s, actionAdminErrorTrackingCreate, params)
	if key.ID == 0 || key.PublicKey == "" || key.SentryDsn == "" {
		e.T.Fatalf("error_tracking_create answered %+v, want a client key with an ID, a public key and a DSN", key)
	}
	adminDeferDelete(e, s, "error tracking client key", actionAdminErrorTrackingDelete, withParams(params, map[string]any{"key_id": key.ID}))

	keys := harness.Do[errortracking.ListClientKeysOutput](s, actionAdminErrorTrackingList, params)
	if len(keys.Keys) != 1 || keys.Keys[0].ID != key.ID {
		e.T.Errorf("error_tracking_list answered %d key(s), want the created %d", len(keys.Keys), key.ID)
	}

	harness.DoVoid(s, actionAdminErrorTrackingDelete, withParams(params, map[string]any{"key_id": key.ID}))
}

// TestAdmin_AlertMetricImages seeds an alert through the Free HTTP integration
// and walks the metric image lifecycle against it: upload, list, update and
// delete.
//
// Replaces: TestMeta_AdminAlertMetricImages
func TestAdmin_AlertMetricImages(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))
	s := e.On(harness.SurfaceMeta)
	project := fixture.NewProject(e, fixture.WithNamePrefix("adm-alert"))
	alertIID := fixture.SeedAlert(e, project)
	e.T.Logf("seeded alert IID %d on %s", alertIID, project.Path)
	base := map[string]any{"project_id": project.IDParam(), "alert_iid": alertIID}

	uploaded := harness.Do[alertmanagement.MetricImageItem](s, actionAdminAlertMetricImageUpload, withParams(base, map[string]any{
		"content_base64": fixture.PNGBase64(e), "filename": "e2e-metric.png",
		"url": "https://e2e-test.example.com/dashboard", "url_text": "e2e dashboard",
	}))
	if uploaded.ID == 0 || uploaded.Filename != "e2e-metric.png" {
		e.T.Fatalf("alert_metric_image_upload answered %+v, want an image with an ID named e2e-metric.png", uploaded)
	}

	list := harness.Do[alertmanagement.ListMetricImagesOutput](s, actionAdminAlertMetricImageList, base)
	if len(list.Images) != 1 || list.Images[0].ID != uploaded.ID {
		e.T.Errorf("alert_metric_image_list answered %d image(s), want the uploaded %d", len(list.Images), uploaded.ID)
	}

	updated := harness.Do[alertmanagement.MetricImageItem](s, actionAdminAlertMetricImageUpdate, withParams(base, map[string]any{
		"image_id": uploaded.ID, "url_text": "e2e dashboard updated",
	}))
	if updated.URLText != "e2e dashboard updated" {
		e.T.Errorf("alert_metric_image_update answered url_text %q, want the updated text", updated.URLText)
	}

	harness.DoVoid(s, actionAdminAlertMetricImageDelete, withParams(base, map[string]any{"image_id": uploaded.ID}))
}

// TestAdmin_TerraformStates seeds two state versions through the raw backend,
// then lists and reads the state, locks and unlocks it, deletes the non-latest
// version and the whole state, and watches the read start answering not found.
//
// Replaces: TestMeta_AdminTerraformStates
func TestAdmin_TerraformStates(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))
	s := e.On(harness.SurfaceMeta)
	project := fixture.NewProject(e, fixture.WithNamePrefix("adm-tfstate"))
	stateName := e.Name("tfstate")
	fixture.PushTerraformState(e, project, stateName, 1)
	fixture.PushTerraformState(e, project, stateName, 2)

	list := harness.Do[terraformstates.ListOutput](s, actionAdminTerraformStateList, map[string]any{"project_path": project.Path})
	if len(list.States) != 1 || list.States[0].Name != stateName {
		e.T.Fatalf("terraform_state_list answered %d state(s), want the seeded %q", len(list.States), stateName)
	}

	got := harness.Do[terraformstates.StateItem](s, actionAdminTerraformStateGet, map[string]any{"project_path": project.Path, "name": stateName})
	if got.Name != stateName || got.LatestSerial != 2 {
		e.T.Errorf("terraform_state_get answered %+v, want %q at serial 2", got, stateName)
	}

	lockParams := map[string]any{"project_id": project.IDParam(), "name": stateName}
	// client-go sends no lock-info body, so the backend deterministically
	// rejects the lock with 400 on this stack; a future client-go that sends
	// the body succeeds, and both are accepted.
	if locked, err := harness.Try[terraformstates.LockOutput](s, actionAdminTerraformStateLock, lockParams); err != nil {
		assertMentions(e, "terraform_state_lock", err.Error(), "ID is missing", "Operation is missing")
		e.T.Logf("terraform_state_lock answered the documented bodyless-request error: %s", firstLine(err.Error()))
	} else if !locked.Success {
		e.T.Errorf("terraform_state_lock reported neither success nor an error: %+v", locked)
	}
	unlocked := harness.Do[terraformstates.LockOutput](s, actionAdminTerraformStateUnlock, lockParams)
	if !unlocked.Success {
		e.T.Errorf("terraform_state_unlock reported no success: %+v", unlocked)
	}

	harness.DoVoid(s, actionAdminTerraformVersionDel, map[string]any{"project_id": project.IDParam(), "name": stateName, "serial": 1})
	harness.DoVoid(s, actionAdminTerraformStateDelete, map[string]any{"project_id": project.IDParam(), "name": stateName})

	// The delete is asynchronous, so the read keeps answering until the purge
	// job runs; a purge merely late is accepted, since the delete itself
	// already succeeded.
	gone := harness.Poll(e.Ctx, 6*time.Second, 60*time.Second, func() (bool, string, error) {
		_, err := harness.Try[terraformstates.StateItem](s, actionAdminTerraformStateGet, map[string]any{"project_path": project.Path, "name": stateName})
		if err != nil {
			//nolint:nilerr // A failing read is the success condition here: the state is gone.
			return true, "state read now fails", nil
		}
		return false, "state still visible after delete", nil
	})
	if gone != nil {
		e.T.Logf("the Terraform state purge is still pending after the poll budget (the delete already succeeded): %v", gone)
	}
}

// TestAdmin_UsageData reads the usage-data endpoints: metric definitions, the
// service ping, the two track-event writes, the SQL queries endpoint behind a
// feature flag this test flips and restores, and the non-SQL metrics endpoint
// which answers the documented 404 on GitLab 19 CE.
//
// Replaces: TestMeta_AdminUsageData
func TestAdmin_UsageData(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin), harness.Locks(harness.LockInstanceGlobal))
	s := e.On(harness.SurfaceMeta)

	defs := harness.Do[usagedata.MetricDefinitionsOutput](s, actionAdminUsageDataMetricDefs, nil)
	if defs.YAML == "" {
		e.T.Errorf("usage_data_metric_definitions answered empty YAML")
	}
	ping := harness.Do[usagedata.GetServicePingOutput](s, actionAdminUsageDataServicePing, nil)
	e.T.Logf("service ping recorded_at=%q counts=%d", ping.RecordedAt, len(ping.Counts))

	tracked := harness.Do[usagedata.TrackEventOutput](s, actionAdminUsageDataTrackEvent, map[string]any{
		"event": "i_quickactions_approve", "additional_properties": map[string]any{"label": "e2e"},
	})
	e.T.Logf("track_event status=%q", tracked.Status)
	batch := harness.Do[usagedata.TrackEventsOutput](s, actionAdminUsageDataTrackEvents, map[string]any{
		"events": []map[string]any{{"event": "i_quickactions_approve"}, {"event": "i_quickactions_approve"}},
	})
	e.T.Logf("track_events status=%q count=%d", batch.Status, batch.Count)

	const queriesFlag = "usage_data_queries_api"
	before := harness.Do[features.ListOutput](s, actionAdminFeatureList, nil)
	restoreFeature(e, s, before.Features, queriesFlag)
	harness.DoVoid(s, actionAdminFeatureSet, map[string]any{"name": queriesFlag, "value": true})
	// Flipper caches flag reads for up to a minute, so the endpoint may keep
	// answering 404 briefly after the flip.
	queries := harness.Eventually[usagedata.QueriesOutput](s, actionAdminUsageDataQueries, nil,
		15*time.Second, 120*time.Second, func(usagedata.QueriesOutput) bool { return true })
	e.T.Logf("usage_data_queries recorded_at=%q", queries.RecordedAt)

	if nonSQL, err := harness.Try[usagedata.NonSQLMetricsOutput](s, actionAdminUsageDataNonSQL, nil); err != nil {
		e.T.Logf("usage_data_non_sql_metrics answered the documented GitLab 19 CE error: %v", err)
	} else {
		e.T.Logf("usage_data_non_sql_metrics served recorded_at=%q", nonSQL.RecordedAt)
	}
}

// TestAdmin_DBMigrationMark asserts the schema-migration mark on its error
// path everywhere, and on its mutating happy path where the Docker setup
// seeded a pending migration.
//
// Replaces: TestMeta_AdminDBMigrationMark
func TestAdmin_DBMigrationMark(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin), harness.Locks(harness.LockInstanceGlobal))
	s := e.On(harness.SurfaceMeta)

	// A nonexistent version must reach GitLab, which refuses it: the
	// confirmation guard is not what stops it, and the answer is not found.
	refused := harness.Refused(s, actionAdminDBMigrationMark, map[string]any{"version": int64(19000101000000)}, harness.FailureNotFound)
	e.T.Logf("marking a nonexistent migration version is refused: %s", firstLine(refused))

	if !e.DockerMode() {
		e.T.Logf("the mutating migration-mark path runs only against the disposable Docker instance")
		return
	}
	version := fixture.DBMigrationVersion(e)
	harness.DoVoid(s, actionAdminDBMigrationMark, map[string]any{"version": version})
	e.T.Logf("marked migration %s as executed", version)
}

// TestAdmin_BulkImports migrates a disposable source group into a new
// top-level group through a self-to-self direct transfer, waits for the
// terminal status, and asserts the read surface: list, get, entity list,
// entity get and entity failures. A second migration is started and canceled
// to assert the canceled status round-trips.
//
// Replaces: TestMeta_AdminBulkImports
func TestAdmin_BulkImports(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))
	s := e.On(harness.SurfaceMeta)
	if !e.DockerMode() {
		e.Skipf("a self-to-self direct transfer needs the compose-internal GitLab URL, which only the Docker stack provides")
	}

	source := fixture.NewGroup(e, fixture.WithGroupNamePrefix("adm-bisrc"))
	started := adminStartBulkImport(e, s, source.Path, e.Name("adm-bidst"))
	e.T.Logf("started bulk import %d (status=%s)", started.ID, started.Status)
	adminWaitBulkTerminal(e, s, started.ID)

	list := harness.Do[bulkimports.ListOutput](s, actionAdminBulkImportList, nil)
	if len(list.Migrations) == 0 {
		e.T.Errorf("bulk_import_list answered no migrations, want at least the one just started")
	}

	got := harness.Do[bulkimports.MigrationSummary](s, actionAdminBulkImportGet, map[string]any{"id": started.ID})
	if got.ID != started.ID || got.SourceType != "gitlab" {
		e.T.Errorf("bulk_import_get answered %+v, want migration %d of source_type gitlab", got, started.ID)
	}
	// The entity records below describe a transfer that ran; a failed, timed
	// out or canceled migration would have them describe nothing.
	if got.Status != "finished" {
		e.T.Fatalf("bulk import %d ended in status %q, want finished", started.ID, got.Status)
	}

	entities := harness.Do[bulkimports.ListEntitiesOutput](s, actionAdminBulkImportEntityList, map[string]any{"bulk_import_id": started.ID})
	if len(entities.Entities) != 1 || entities.Entities[0].SourceFullPath != source.Path {
		e.T.Fatalf("bulk_import_entity_list answered %d entity(ies), want the one for %q", len(entities.Entities), source.Path)
	}
	entityID := entities.Entities[0].ID

	entity := harness.Do[bulkimports.EntitySummary](s, actionAdminBulkImportEntityGet, map[string]any{"bulk_import_id": started.ID, "entity_id": entityID})
	if entity.ID != entityID {
		e.T.Errorf("bulk_import_entity_get answered entity %d, want %d", entity.ID, entityID)
	}

	failures := harness.Do[bulkimports.ListEntityFailuresOutput](s, actionAdminBulkImportEntityFailures, map[string]any{"bulk_import_id": started.ID, "entity_id": entityID})
	e.T.Logf("entity %d failures: %d", entityID, len(failures.Failures))

	second := adminStartBulkImport(e, s, source.Path, e.Name("adm-bicnl"))
	canceled := harness.Do[bulkimports.MigrationSummary](s, actionAdminBulkImportCancel, map[string]any{"id": second.ID})
	if canceled.Status != "canceled" {
		e.T.Errorf("bulk_import_cancel answered status %q, want canceled", canceled.Status)
	}
}

// adminStartBulkImport starts one self-to-self direct transfer of a source
// group into a fresh destination and registers deletion of that destination.
func adminStartBulkImport(e *harness.Env, s *harness.Session, sourceFullPath, destinationSlug string) bulkimports.MigrationOutput {
	e.T.Helper()

	token := e.Setting(fixture.SettingGitLabToken)
	if token == "" {
		e.T.Fatalf("%s is required as the direct-transfer source token", fixture.SettingGitLabToken)
	}
	started := harness.Do[bulkimports.MigrationOutput](s, actionAdminBulkImportStart, map[string]any{
		"configuration": map[string]any{"url": fixture.InternalGitLabURL(e), "access_token": token},
		"entities": []map[string]any{{
			"source_type": "group_entity", "source_full_path": sourceFullPath,
			"destination_slug": destinationSlug, "destination_namespace": "",
		}},
	})
	if started.ID == 0 {
		e.T.Fatalf("bulk_import_start answered %+v, want a migration with an ID", started)
	}
	// The destination is a fresh top-level group the migration creates by
	// slug; a canceled migration usually never creates it, so a not-found is
	// tolerated. It is addressed by its slug, which client-go accepts as the
	// group id, since the migration answers no numeric id at start time.
	e.Defer("bulk import destination "+destinationSlug, func(ctx context.Context) error {
		if _, err := e.Client().GL().Groups.DeleteGroup(destinationSlug, nil, gl.WithContext(ctx)); err != nil && !fixture.IsStatus(err, 404) {
			return err
		}
		return nil
	})
	return started
}

// adminWaitBulkTerminal polls the migration until it reaches a terminal
// status, so destination cleanup is deterministic. A migration that never
// settles is canceled for the same reason.
func adminWaitBulkTerminal(e *harness.Env, s *harness.Session, importID int64) {
	e.T.Helper()

	last := "unknown"
	err := harness.Poll(e.Ctx, 3*time.Second, 240*time.Second, func() (bool, string, error) {
		got, tryErr := harness.Try[bulkimports.MigrationSummary](s, actionAdminBulkImportGet, map[string]any{"id": importID})
		if tryErr != nil {
			//nolint:nilerr // A transient lookup failure is retried until the poll deadline.
			return false, "bulk_import_get error", nil
		}
		last = got.Status
		return bulkImportTerminal(got.Status), "status=" + got.Status, nil
	})
	if err != nil {
		e.T.Logf("bulk import %d did not settle in time (last %q); canceling to keep cleanup deterministic", importID, last)
		if canceled, cancelErr := harness.Try[bulkimports.MigrationSummary](s, actionAdminBulkImportCancel, map[string]any{"id": importID}, harness.For(harness.PurposeCleanup)); cancelErr != nil {
			e.T.Logf("canceling bulk import %d answered: %v", importID, cancelErr)
		} else {
			e.T.Logf("canceled bulk import %d (status %q)", importID, canceled.Status)
		}
		return
	}
	e.T.Logf("bulk import %d reached terminal status %q", importID, last)
}

// bulkImportTerminal reports whether a direct-transfer status is one it will
// not move on from.
func bulkImportTerminal(status string) bool {
	switch status {
	case "finished", "failed", "timeout", "canceled":
		return true
	default:
		return false
	}
}
