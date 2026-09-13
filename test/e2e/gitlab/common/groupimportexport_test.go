//go:build e2e

// groupimportexport_test.go covers a group's export and import as one round
// trip: the download refused before anything was scheduled, the schedule,
// the download throttled behind the read that came before it, the archive
// once it is ready, and the archive imported back as a new group.
//
// GitLab throttles the export download to one request per group per
// minute, so a surface's scenario takes a minute of waiting whatever the
// export itself takes; the throttle is asserted rather than avoided, since
// a client polling the download sees exactly that answer.

package common

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupimportexport"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The bounds of the waits this file makes: the download is asked again
// once the throttle's minute can have passed, and an imported group is
// looked for until GitLab has created it.
const (
	groupExportPollInterval = 30 * time.Second
	groupExportWait         = 4 * time.Minute
	importedGroupInterval   = 3 * time.Second
	importedGroupWait       = 45 * time.Second
	// The import job runs after the group exists, so the content it carries
	// is waited for well past the wait for the group itself.
	importedContentWait = 4 * time.Minute
)

// TestGroupExport_ScheduleDownloadAndImport_RoundTrips walks a group of
// each surface's own through its export and back: the download refused
// before a schedule, the schedule, the download throttled, the archive
// downloaded once GitLab lets it, and the archive imported as a new group.
//
// Replaces: TestMeta_GroupExportImport
func TestGroupExport_ScheduleDownloadAndImport_RoundTrips(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("export"))
		params := map[string]any{"group_id": group.IDParam()}
		// GitLab creates the destination group before it loads the archive, so
		// neither the import's own answer nor the group appearing says the
		// archive was read. A label put in the source before the export is what
		// the round trip is asserted on at the other end.
		labelName := seedGroupLabel(e, group)

		refused := harness.Refused(s, actionGroupExportDownload, params, harness.FailureNotFound)
		assertMentions(e, "the download of an export nobody scheduled", refused, "scheduled first")

		scheduled := harness.Do[groupimportexport.ScheduleExportOutput](s, actionGroupExportSchedule, params)
		if !strings.Contains(strings.ToLower(scheduled.Message), "scheduled") {
			e.T.Errorf("group_export_schedule answered %+v, want a message saying the export was scheduled", scheduled)
		}
		// The read before the schedule took this minute's one download, so
		// the next is throttled whether or not the export has finished.
		throttled := harness.ExpectToolError(s, actionGroupExportDownload, params, "too many requests")
		e.T.Logf("the second download within the minute is throttled: %s", firstLine(throttled))

		archive := harness.Eventually(s, actionGroupExportDownload, params, groupExportPollInterval, groupExportWait,
			func(download groupimportexport.ExportDownloadOutput) bool { return download.SizeBytes > 0 })
		decoded, err := base64.StdEncoding.DecodeString(archive.ContentBase64)
		if err != nil || len(decoded) != archive.SizeBytes || !bytes.HasPrefix(decoded, gzipMagic) {
			e.T.Fatalf("group_export_download answered %d bytes that decode to %d with error %v, want a gzip archive of the size it reports", archive.SizeBytes, len(decoded), err)
		}

		// The import takes a file the server reads, and the temporary
		// directory is one the server accepts an import from without an
		// allow-list.
		archivePath := filepath.Join(e.T.TempDir(), "group-export.tar.gz")
		if writeErr := os.WriteFile(archivePath, decoded, 0o600); writeErr != nil {
			e.T.Fatalf("writing the export archive: %v", writeErr)
		}
		name := e.Name("imported")
		imported := harness.Do[groupimportexport.ImportFileOutput](s, actionGroupImportFile, map[string]any{"file": archivePath, "name": name, "path": name})
		if !strings.Contains(strings.ToLower(imported.Message), "started") {
			e.T.Errorf("group_import_file answered %+v, want a message saying the import started", imported)
		}
		e.Defer("imported group "+name, func(ctx context.Context) error {
			return deleteImportedGroup(ctx, e.Client(), name)
		})

		assertImportedGroupCarriesLabel(e, name, labelName)
	})
}

// seedGroupLabel puts one label in the group under a name scoped to the run
// and returns that name, so the import can be asserted on something the
// archive had to carry. The label goes with the group, so nothing is
// registered.
func seedGroupLabel(e *harness.Env, group fixture.Group) string {
	e.T.Helper()

	name := e.Name("exported")
	if _, _, err := e.Client().GL().GroupLabels.CreateGroupLabel(group.ID, &gl.CreateGroupLabelOptions{
		Name: new(name), Color: new("#428BCA"),
	}, gl.WithContext(e.Ctx)); err != nil {
		e.T.Fatalf("seeding the label %q in group %d before the export: %v", name, group.ID, err)
	}
	return name
}

// assertImportedGroupCarriesLabel waits for the imported group to hold the
// label the source was seeded with, which is the first thing that proves the
// archive was read rather than merely accepted.
func assertImportedGroupCarriesLabel(e *harness.Env, path, labelName string) {
	e.T.Helper()

	last := "the imported group is not readable yet"
	err := harness.Poll(e.Ctx, importedGroupInterval, importedContentWait, func() (bool, string, error) {
		labels, _, listErr := e.Client().GL().GroupLabels.ListGroupLabels(path, &gl.ListGroupLabelsOptions{
			PerPage: 100,
		}, gl.WithContext(e.Ctx))
		if listErr != nil {
			if fixture.IsStatus(listErr, http.StatusNotFound) {
				return false, last, nil
			}
			return false, "", listErr
		}
		for _, label := range labels {
			if label.Name == labelName {
				return true, "", nil
			}
		}
		last = fmt.Sprintf("the imported group holds %d label(s) and not %q", len(labels), labelName)
		return false, last, nil
	})
	if err != nil {
		e.T.Errorf("the import never carried the seeded label into %q: %v", path, err)
	}
}

// deleteImportedGroup waits for the group an import creates to be readable
// under its path and deletes it. GitLab creates the group before the
// import job fills it, so the wait is short, and a group that never
// appears is reported rather than looked for past the cleanup budget.
func deleteImportedGroup(ctx context.Context, client *gitlabclient.Client, path string) error {
	var (
		id       int64
		fullPath string
	)
	err := harness.Poll(ctx, importedGroupInterval, importedGroupWait, func() (bool, string, error) {
		group, _, getErr := client.GL().Groups.GetGroup(path, nil, gl.WithContext(ctx))
		if getErr != nil {
			if fixture.IsStatus(getErr, http.StatusNotFound) {
				return false, "the imported group is not readable yet", nil
			}
			return false, "", getErr
		}
		id, fullPath = group.ID, group.FullPath
		return true, "", nil
	})
	if err != nil {
		return fmt.Errorf("finding the imported group %q: %w", path, err)
	}
	return fixture.DeleteGroup(ctx, client, id, fullPath)
}
