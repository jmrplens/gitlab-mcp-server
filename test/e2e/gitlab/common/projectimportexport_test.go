//go:build e2e

// projectimportexport_test.go covers a project's export and import: the
// schedule and the two status reads, which every surface asks of a project
// of its own; the refusal of an import while the instance's gitlab_project
// import source is disabled, which is how a fresh instance stands; and the
// round trip of an export downloaded and imported back as a new project,
// which needs that source enabled for the length of the test.
//
// The import source is an application setting, so the two tests that touch
// or depend on it hold the instance-global lock. GitLab's settings cache
// serves the old value for up to a minute after a change, and both tests
// wait that lag out rather than assume the change is visible at once.

package common

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projectimportexport"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// projectImportSource is the import source POST /projects/import needs
// enabled, which a fresh instance leaves off.
const projectImportSource = "gitlab_project"

// The bounds of the waits this file makes: for a scheduled export to be
// picked up, for one to finish under the load of a whole suite, and for a
// settings change to reach GitLab's cache.
const (
	exportPollInterval  = 3 * time.Second
	exportStartedWait   = 90 * time.Second
	exportWait          = 5 * time.Minute
	settingsLagInterval = 5 * time.Second
	settingsLagWait     = 90 * time.Second
	// The import answers as soon as it is accepted, so its own completion is
	// waited for on the same budget the export takes.
	importStatusInterval = 3 * time.Second
	importStatusWait     = 5 * time.Minute
)

// gzipMagic opens every gzip stream, which is what an export archive is.
var gzipMagic = []byte{0x1f, 0x8b}

// TestProjectExport_ScheduleAndStatus_AnswerTheProject schedules the export
// of a project of each surface's own, reads the export status of the
// project and the import status of a project that was never imported.
//
// Replaces: TestMeta_ProjectExport
func TestProjectExport_ScheduleAndStatus_AnswerTheProject(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		project := fixture.NewProject(e, fixture.WithNamePrefix("exportstatus"))
		params := map[string]any{"project_id": project.IDParam()}

		scheduled := harness.Do[projectimportexport.ScheduleExportOutput](s, actionProjectExportSchedule, params)
		if !strings.Contains(strings.ToLower(scheduled.Message), "scheduled") {
			e.T.Errorf("export_schedule answered %+v, want a message saying the export was scheduled", scheduled)
		}
		// Which state the export has reached is Sidekiq's business; that
		// the project leaves the none of a project nobody asked to export
		// is what is assertable here, and it is waited for rather than read
		// once, because the schedule returns before the job is picked up.
		status := harness.Eventually(s, actionProjectExportStatus, params, exportPollInterval, exportStartedWait,
			func(status projectimportexport.ExportStatusOutput) bool {
				return status.ExportStatus != "" && status.ExportStatus != "none"
			})
		if status.ID != project.ID {
			e.T.Errorf("export_status answered %+v, want project %d", status, project.ID)
		}
		imported := harness.Do[projectimportexport.ImportStatusOutput](s, actionProjectImportStatus, params)
		if imported.ID != project.ID || imported.ImportStatus != "none" {
			e.T.Errorf("import_status answered %+v, want project %d with the import status none, since the fixture created it", imported, project.ID)
		}
	})
}

// TestProjectImport_FromFile_RefusedWhileTheSourceIsDisabled shows, on every
// surface, that an import is refused as forbidden while the instance does
// not enable the gitlab_project import source, which is how the Docker
// instance stands. The refusal comes before the archive is read, so the
// archive is a few bytes that are no archive at all.
//
// Replaces: TestMeta_ProjectExportDownloadImport
func TestProjectImport_FromFile_RefusedWhileTheSourceIsDisabled(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin), harness.Locks(harness.LockInstanceGlobal))
	if slices.Contains(readImportSources(e), projectImportSource) {
		e.Skipf("this instance enables the %s import source, and the scenario asserts the refusal of a disabled one", projectImportSource)
	}

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		name := e.Name("refusedimport")
		params := map[string]any{"content_base64": base64.StdEncoding.EncodeToString([]byte("not an archive")), "name": name, "path": name}

		// Polled rather than asked once, because the settings cache can
		// still serve an enabled source for a minute after a test restored
		// the disabled one; an answer other than the refusal is the lag.
		var refused string
		err := harness.Poll(e.Ctx, settingsLagInterval, settingsLagWait, func() (bool, string, error) {
			_, tryErr := harness.Try[projectimportexport.ImportStatusOutput](s, actionProjectImportFromFile, params)
			if tryErr == nil {
				return false, "the import was accepted, so the disabled source is not visible yet", nil
			}
			if !strings.Contains(tryErr.Error(), "403") {
				return false, "the import was refused for another reason than the disabled source: " + firstLine(tryErr.Error()), nil
			}
			refused = tryErr.Error()
			return true, "", nil
		})
		if err != nil {
			e.T.Fatalf("waiting for the import to be refused as forbidden: %v", err)
		}
		assertMentions(e, "the refusal of an import while the source is disabled", refused, "access denied")
	})
}

// TestProjectExport_DownloadAndImport_RoundTrips schedules the export of a
// project of each surface's own, waits for it to finish, downloads the
// archive and imports it back as a new project, with the gitlab_project
// import source enabled for the length of the test.
//
// Replaces: TestMeta_ProjectExportDownloadImport
func TestProjectExport_DownloadAndImport_RoundTrips(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin), harness.Locks(harness.LockInstanceGlobal))
	enableImportSource(e)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		project := fixture.NewProject(e, fixture.WithNamePrefix("roundtrip"))
		params := map[string]any{"project_id": project.IDParam()}

		scheduled := harness.Do[projectimportexport.ScheduleExportOutput](s, actionProjectExportSchedule, params)
		if !strings.Contains(strings.ToLower(scheduled.Message), "scheduled") {
			e.T.Errorf("export_schedule answered %+v, want a message saying the export was scheduled", scheduled)
		}
		finished := harness.Eventually(s, actionProjectExportStatus, params, exportPollInterval, exportWait,
			func(status projectimportexport.ExportStatusOutput) bool { return status.ExportStatus == "finished" })
		if finished.ID != project.ID {
			e.T.Errorf("export_status answered %+v, want project %d", finished, project.ID)
		}

		archive := harness.Do[projectimportexport.ExportDownloadOutput](s, actionProjectExportDownload, params)
		decoded, err := base64.StdEncoding.DecodeString(archive.ContentBase64)
		if err != nil || archive.SizeBytes == 0 || len(decoded) != archive.SizeBytes || !bytes.HasPrefix(decoded, gzipMagic) {
			e.T.Fatalf("export_download answered %d bytes that decode to %d with error %v, want a gzip archive of the size it reports", archive.SizeBytes, len(decoded), err)
		}

		name := e.Name("imported")
		imported := importArchive(e, s, archive.ContentBase64, name)
		if imported.ID == 0 || imported.ImportStatus == "" || imported.Path != name {
			e.T.Errorf("import_from_file answered %+v, want the project %q with an ID and an import status", imported, name)
		}
		// The answer above is the import accepted, not the import done, so the
		// status is read until GitLab says the archive was loaded.
		assertImportFinished(e, s, imported.ID)
	})
}

// assertImportFinished polls the import status of a project until GitLab
// reports the archive loaded, failing the test on the status it reports for an
// import that will not finish.
func assertImportFinished(e *harness.Env, s *harness.Session, projectID int64) {
	e.T.Helper()

	params := map[string]any{"project_id": projectID}
	last := "none"
	// The read is allowed to fail transiently while GitLab settles, and an
	// earlier version treated every failure as transient and dropped it. A run
	// where every read failed then reported a five-minute timeout with a last
	// status of "none" and no reason at all, which is the least actionable
	// thing an e2e failure can say. The error is kept, reported, and stops the
	// poll once it has repeated enough times to be the answer rather than a
	// hiccup.
	var lastErr error
	consecutive := 0
	err := harness.Poll(e.Ctx, importStatusInterval, importStatusWait, func() (bool, string, error) {
		status, tryErr := harness.Try[projectimportexport.ImportStatusOutput](s, actionProjectImportStatus, params)
		if tryErr != nil {
			lastErr = tryErr
			consecutive++
			if consecutive >= importStatusFailuresAllowed {
				return false, "", fmt.Errorf("import_status failed %d times in a row: %w", consecutive, tryErr)
			}
			return false, "import_status error: " + firstLine(tryErr.Error()), nil
		}
		consecutive = 0
		last = status.ImportStatus
		if last == "failed" {
			return false, "", fmt.Errorf("the import failed: %s", status.ImportError)
		}
		return last == "finished", "status=" + last, nil
	})
	if err != nil {
		e.T.Errorf("the import of project %d never finished (last status %q, last read error %v): %v",
			projectID, last, lastErr, err)
	}
}

// importStatusFailuresAllowed is how many consecutive failed status reads are
// taken as settling rather than as the answer. Beyond it the poll ends naming
// the error, so a broken read costs one interval times this rather than the
// whole window.
const importStatusFailuresAllowed = 10

// importArchive imports an archive as a new project, retrying the refusal
// GitLab's settings cache makes while the enabled source is not visible
// yet, and registers the imported project's deletion.
func importArchive(e *harness.Env, s *harness.Session, contentBase64, name string) projectimportexport.ImportStatusOutput {
	e.T.Helper()
	params := map[string]any{"content_base64": contentBase64, "name": name, "path": name}

	imported, err := harness.Retry(e.Ctx, e.T, "import_from_file", 6, 10*time.Second, func(int) (projectimportexport.ImportStatusOutput, bool, string, error) {
		out, tryErr := harness.Try[projectimportexport.ImportStatusOutput](s, actionProjectImportFromFile, params)
		retryable := tryErr != nil && strings.Contains(tryErr.Error(), "403")
		return out, retryable, "the enabled import source is not visible yet", tryErr
	})
	if err != nil {
		e.T.Fatalf("importing the archive as %q: %v", name, err)
	}
	e.Defer("imported project "+imported.PathWithNamespace, func(ctx context.Context) error {
		return fixture.DeleteProject(ctx, e.Client(), imported.ID, imported.PathWithNamespace)
	})
	return imported
}

// readImportSources reads the instance's enabled import sources.
func readImportSources(e *harness.Env) []string {
	e.T.Helper()
	settings, _, err := e.Client().GL().Settings.GetSettings(gl.WithContext(e.Ctx))
	if err != nil {
		e.T.Fatalf("reading the application settings: %v", err)
	}
	return settings.ImportSources
}

// enableImportSource adds the gitlab_project import source to the instance
// for the length of the test, restoring the sources it found when the test
// ends. An instance that already enables it is left alone.
func enableImportSource(e *harness.Env) {
	e.T.Helper()
	original := readImportSources(e)
	if slices.Contains(original, projectImportSource) {
		return
	}
	enabled := append(slices.Clone(original), projectImportSource)
	if _, _, err := e.Client().GL().Settings.UpdateSettings(&gl.UpdateSettingsOptions{ImportSources: &enabled}, gl.WithContext(e.Ctx)); err != nil {
		e.T.Fatalf("enabling the %s import source: %v", projectImportSource, err)
	}
	e.Defer("import sources restored", func(ctx context.Context) error {
		_, _, err := e.Client().GL().Settings.UpdateSettings(&gl.UpdateSettingsOptions{ImportSources: &original}, gl.WithContext(ctx))
		return err
	})
}
