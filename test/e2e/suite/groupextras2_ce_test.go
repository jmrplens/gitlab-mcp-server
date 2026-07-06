//go:build e2e && !enterprise

// groupextras2_ce_test.go covers the remaining gitlab_group meta-tool
// actions from the e2e gap audit: group board list (column) CRUD, group
// markdown uploads, group export/import, group relations export, and
// service-account PAT rotation.
//
// Build tag: e2e && !enterprise.
package suite

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupboards"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupimportexport"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/grouplabels"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupmarkdownuploads"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/grouprelationsexport"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupserviceaccounts"
)

// TestMeta_GroupBoardListColumns exercises group_board_create_list,
// group_board_get_list, group_board_update_list, and group_board_delete_list
// through the gitlab_group meta-tool.
//
// The test creates a group and a group label, provisions an issue board
// through the raw SDK (board creation is Premium-gated on the meta surface,
// and CE allows a single board per group), then creates a label-backed
// column, fetches it, reorders it, and deletes it. When no board can be
// provisioned (older CE builds without the board create route) the test
// documents that with a skip.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_GroupBoardListColumns(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	e2e := NewE2EContext(t)
	grp := CreateGroupMeta(ctx, e2e, sess.meta, "grp-board-cols")

	boardID, ok := groupExtrasEnsureBoard(ctx, t, grp)
	if !ok {
		t.Skip("no group issue board available: board provisioning via API is unsupported on this GitLab edition/version")
	}
	labelID := groupExtrasCreateGroupLabel(ctx, t, grp)

	var listID int64

	t.Run("CreateList", func(t *testing.T) {
		out, err := callToolOn[groupboards.BoardListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_board_create_list",
			"params": map[string]any{
				"group_id": grp.gidStr(),
				"board_id": boardID,
				"label_id": labelID,
			},
		})
		requireNoError(t, err, "group_board_create_list")
		requireTruef(t, out.ID > 0, "expected positive board list ID")
		listID = out.ID
		t.Logf("Created board list %d on board %d", listID, boardID)
	})

	t.Run("GetList", func(t *testing.T) {
		requireTruef(t, listID > 0, "listID not set")
		out, err := callToolOn[groupboards.BoardListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_board_get_list",
			"params": map[string]any{
				"group_id": grp.gidStr(),
				"board_id": boardID,
				"list_id":  listID,
			},
		})
		requireNoError(t, err, "group_board_get_list")
		requireTruef(t, out.ID == listID, "got board list %d, want %d", out.ID, listID)
		t.Logf("Got board list %d at position %d", out.ID, out.Position)
	})

	t.Run("UpdateList", func(t *testing.T) {
		requireTruef(t, listID > 0, "listID not set")
		out, err := callToolOn[groupboards.BoardListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_board_update_list",
			"params": map[string]any{
				"group_id": grp.gidStr(),
				"board_id": boardID,
				"list_id":  listID,
				"position": 0,
			},
		})
		requireNoError(t, err, "group_board_update_list")
		requireTruef(t, out.ID > 0, "expected a board list in the update response")
		t.Logf("Reordered board list %d to position %d", out.ID, out.Position)
	})

	t.Run("DeleteList", func(t *testing.T) {
		requireTruef(t, listID > 0, "listID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_board_delete_list",
			"params": map[string]any{
				"group_id": grp.gidStr(),
				"board_id": boardID,
				"list_id":  listID,
			},
		})
		requireNoError(t, err, "group_board_delete_list")
		t.Logf("Deleted board list %d", listID)
	})
}

// TestMeta_GroupMarkdownUploads exercises group_upload_list,
// group_upload_delete_by_id, and group_upload_delete_by_secret through the
// gitlab_group meta-tool.
//
// The test attempts to seed uploads through the raw POST /groups/:id/uploads
// endpoint (client-go has no create wrapper for group markdown uploads).
// When the endpoint accepts files, the test asserts the list shows them and
// deletes one by ID and one by secret+filename. When the endpoint is
// unavailable on the running GitLab version, the test asserts the list is
// empty and that both delete actions surface their documented 404 error
// paths instead.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_GroupMarkdownUploads(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	e2e := NewE2EContext(t)
	grp := CreateGroupMeta(ctx, e2e, sess.meta, "grp-uploads")

	first, ok := groupExtrasUploadGroupFile(ctx, t, grp.ID, "e2e-upload-1.txt")
	if ok {
		runGroupExtrasUploadHappyPath(t, ctx, grp, first)
		return
	}
	runGroupExtrasUploadUnavailablePath(t, ctx, grp)
}

// runGroupExtrasUploadHappyPath lists seeded uploads and deletes them by ID
// and by secret+filename.
func runGroupExtrasUploadHappyPath(t *testing.T, ctx context.Context, grp GroupFixture, first groupExtrasUpload) {
	t.Helper()

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[groupmarkdownuploads.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_upload_list",
			"params": map[string]any{"group_id": grp.gidStr()},
		})
		requireNoError(t, err, "group_upload_list")
		requireTruef(t, len(out.Uploads) >= 1, "expected at least 1 upload, got %d", len(out.Uploads))
		t.Logf("Listed %d group uploads", len(out.Uploads))
	})

	t.Run("DeleteByID", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_upload_delete_by_id",
			"params": map[string]any{
				"group_id":  grp.gidStr(),
				"upload_id": first.ID,
			},
		})
		requireNoError(t, err, "group_upload_delete_by_id")
		t.Logf("Deleted upload %d by ID", first.ID)
	})

	t.Run("DeleteBySecret", func(t *testing.T) {
		second, ok := groupExtrasUploadGroupFile(ctx, t, grp.ID, "e2e-upload-2.txt")
		requireTruef(t, ok, "second upload should succeed after the first one did")
		if second.Secret == "" {
			t.Skip("upload response carried no 32-hex secret in url/full_path; cannot address by secret")
		}
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_upload_delete_by_secret",
			"params": map[string]any{
				"group_id": grp.gidStr(),
				"secret":   second.Secret,
				"filename": second.Filename,
			},
		})
		requireNoError(t, err, "group_upload_delete_by_secret")
		t.Logf("Deleted upload %q by secret", second.Filename)
	})
}

// runGroupExtrasUploadUnavailablePath asserts the empty list plus the 404
// error paths of both delete actions when uploads cannot be seeded.
func runGroupExtrasUploadUnavailablePath(t *testing.T, ctx context.Context, grp GroupFixture) {
	t.Helper()

	t.Run("ListEmpty", func(t *testing.T) {
		out, err := callToolOn[groupmarkdownuploads.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_upload_list",
			"params": map[string]any{"group_id": grp.gidStr()},
		})
		requireNoError(t, err, "group_upload_list")
		requireTruef(t, len(out.Uploads) == 0, "expected 0 uploads, got %d", len(out.Uploads))
	})

	t.Run("DeleteByIDNotFound", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_upload_delete_by_id",
			"params": map[string]any{
				"group_id":  grp.gidStr(),
				"upload_id": int64(999999999),
			},
		})
		requireTruef(t, err != nil, "expected error deleting a non-existent upload by ID")
		t.Logf("Expected error deleting non-existent upload by ID: %v", err)
	})

	t.Run("DeleteBySecretNotFound", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_upload_delete_by_secret",
			"params": map[string]any{
				"group_id": grp.gidStr(),
				"secret":   strings.Repeat("0123", 8),
				"filename": "does-not-exist.txt",
			},
		})
		requireTruef(t, err != nil, "expected error deleting a non-existent upload by secret")
		t.Logf("Expected error deleting non-existent upload by secret: %v", err)
	})
}

// TestMeta_GroupExportImport exercises group_export_schedule,
// group_export_download, and group_import_file through the gitlab_group
// meta-tool.
//
// The test schedules an export for a fresh group, polls the download action
// with a generous budget (export archives are built asynchronously by
// Sidekiq), and imports the downloaded archive as a new group. When the
// archive never becomes downloadable within the budget the schedule
// assertion stands and the download/import subtests skip with the reason.
// The imported group is deleted once GitLab finishes materializing it.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_GroupExportImport(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := e2eTimeoutContext(420*time.Second, 660*time.Second)
	defer cancel()

	e2e := NewE2EContext(t)
	grp := CreateGroupMeta(ctx, e2e, sess.meta, "grp-export")

	t.Run("ExportSchedule", func(t *testing.T) {
		out, err := callToolOn[groupimportexport.ScheduleExportOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_export_schedule",
			"params": map[string]any{"group_id": grp.gidStr()},
		})
		requireNoError(t, err, "group_export_schedule")
		requireTruef(t, strings.Contains(out.Message, "scheduled"), "unexpected schedule message %q", out.Message)
		t.Logf("Scheduled export for group %d", grp.ID)
	})

	var archive []byte

	t.Run("ExportDownload", func(t *testing.T) {
		budget := e2eTimeout(180*time.Second, 300*time.Second)
		pollCtx, pollCancel := context.WithTimeout(ctx, budget)
		defer pollCancel()
		pollErr := Poll(pollCtx, 3*time.Second, budget, func() (bool, string, error) {
			out, err := callToolOn[groupimportexport.ExportDownloadOutput](pollCtx, sess.meta, "gitlab_group", map[string]any{
				"action": "group_export_download",
				"params": map[string]any{"group_id": grp.gidStr()},
			})
			if err != nil {
				return false, fmt.Sprintf("export archive not ready: %v", err), nil
			}
			data, decErr := base64.StdEncoding.DecodeString(out.ContentBase64)
			if decErr != nil {
				return false, "", fmt.Errorf("decode export archive: %w", decErr)
			}
			archive = data
			return true, fmt.Sprintf("downloaded %d bytes", len(data)), nil
		})
		if pollErr != nil {
			t.Skipf("group export archive not downloadable within %s (schedule succeeded): %v", budget, pollErr)
		}
		requireTruef(t, len(archive) > 0, "expected non-empty export archive")
		t.Logf("Downloaded export archive: %d bytes", len(archive))
	})

	t.Run("ImportFile", func(t *testing.T) {
		if len(archive) == 0 {
			t.Skip("no export archive available to import (download skipped)")
		}
		archivePath := filepath.Join(t.TempDir(), "group-export.tar.gz")
		requireNoError(t, os.WriteFile(archivePath, archive, 0o600), "write export archive")

		importPath := uniqueName("grp-import")
		out, err := callToolOn[groupimportexport.ImportFileOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_import_file",
			"params": map[string]any{
				"name": importPath,
				"path": importPath,
				"file": archivePath,
			},
		})
		requireNoError(t, err, "group_import_file")
		requireTruef(t, strings.Contains(out.Message, "started"), "unexpected import message %q", out.Message)
		//nolint:contextcheck // Cleanup runs after the test context is canceled and needs its own bounded context.
		t.Cleanup(func() { groupExtrasDeleteImportedGroup(importPath) })
		t.Logf("Started import of group %q", importPath)
	})
}

// TestMeta_GroupRelationsExport exercises group_relations_schedule and
// group_relations_list_status through the gitlab_group meta-tool.
//
// The test schedules a relations (ndjson) export on a fresh group and polls
// the status list until GitLab records per-relation rows. The list assertion
// tolerates an empty result at the end of the budget (Sidekiq lag) because
// the schedule confirmation already proves the export pipeline accepted the
// request; the poll outcome is logged either way.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_GroupRelationsExport(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	e2e := NewE2EContext(t)
	grp := CreateGroupMeta(ctx, e2e, sess.meta, "grp-relations")

	t.Run("Schedule", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_relations_schedule",
			"params": map[string]any{"group_id": grp.gidStr()},
		})
		// Relations (direct-transfer) exports 404 while the bulk_import
		// feature is disabled — the instance-wide default. Enable it on the
		// disposable Docker instance and retry once; skip elsewhere.
		if err != nil && isHTTPStatus(err, 404) {
			if !isDockerMode() {
				t.Skipf("relations export requires the bulk_import setting: %v", err)
			}
			_, _, settingsErr := sess.glClient.GL().Settings.UpdateSettings(&gl.UpdateSettingsOptions{
				BulkImportEnabled: new(true),
			}, gl.WithContext(ctx))
			requireNoError(t, settingsErr, "enable bulk_import setting")
			// The application settings cache holds the old value for up to a
			// minute, so poll until the export endpoint accepts the request.
			_, err = retryWithBackoff(ctx, t, "group_relations_schedule", 8, func(int) (struct{}, bool, string, error) {
				retryErr := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
					"action": "group_relations_schedule",
					"params": map[string]any{"group_id": grp.gidStr()},
				})
				return struct{}{}, retryErr != nil && isHTTPStatus(retryErr, 404), "bulk_import setting not visible yet", retryErr
			})
		}
		requireNoError(t, err, "group_relations_schedule")
		t.Logf("Scheduled relations export for group %d", grp.ID)
	})

	t.Run("ListStatus", func(t *testing.T) {
		var statuses int
		_, err := retryWithBackoff(ctx, t, "group_relations_list_status", 5, func(int) (struct{}, bool, string, error) {
			out, listErr := callToolOn[grouprelationsexport.ListExportStatusOutput](ctx, sess.meta, "gitlab_group", map[string]any{
				"action": "group_relations_list_status",
				"params": map[string]any{"group_id": grp.gidStr()},
			})
			if listErr != nil && isHTTPStatus(listErr, 404) {
				t.Skipf("relations export status requires the bulk_import setting: %v", listErr)
			}
			if listErr != nil {
				return struct{}{}, false, "", listErr
			}
			statuses = len(out.Statuses)
			if statuses == 0 {
				return struct{}{}, true, "no relation statuses recorded yet", nil
			}
			return struct{}{}, false, "", nil
		})
		requireNoError(t, err, "group_relations_list_status")
		// Zero rows after the retry budget means Sidekiq has not picked the
		// export up yet; the successful list call is still asserted above.
		t.Logf("Relations export reports %d relation statuses", statuses)
	})
}

// TestMeta_GroupServiceAccountPATRotate exercises service_account_pat_rotate
// through the gitlab_group meta-tool.
//
// The test creates a group service account (skipping when the instance
// rejects service accounts for licensing/permission reasons), issues a
// personal access token for it, rotates that token, and asserts the rotation
// returns a fresh token value with a new token ID. The service account is
// hard-deleted afterwards. Docker-gated because service-account creation is
// an instance-admin operation on self-managed GitLab.
//
// Build tag: e2e && !enterprise. Mode: CE (Docker only). Surface: meta.
// Admin token required.
func TestMeta_GroupServiceAccountPATRotate(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	if !isDockerMode() {
		t.Skip("service-account provisioning requires the disposable Docker GitLab (E2E_MODE=docker)")
	}
	RequireCapabilities(t, CapabilityAdmin)

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	e2e := NewE2EContext(t)
	grp := CreateGroupMeta(ctx, e2e, sess.meta, "grp-svcacct")

	account, err := callToolOn[groupserviceaccounts.Output](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "service_account_create",
		"params": map[string]any{"group_id": grp.gidStr()},
	})
	if err != nil {
		if isHTTPStatus(err, http.StatusForbidden) || isHTTPStatus(err, http.StatusNotFound) ||
			strings.Contains(strings.ToLower(err.Error()), "license") {
			t.Skipf("group service accounts unavailable on this instance: %v", err)
		}
		requireNoError(t, err, "service_account_create")
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_ = callToolVoidOn(cleanupCtx, sess.meta, "gitlab_group", map[string]any{
			"action": "service_account_delete",
			"params": map[string]any{
				"group_id":           grp.gidStr(),
				"service_account_id": account.ID,
				"hard_delete":        true,
			},
		})
	})

	pat, err := callToolOn[groupserviceaccounts.PATOutput](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "service_account_pat_create",
		"params": map[string]any{
			"group_id":           grp.gidStr(),
			"service_account_id": account.ID,
			"name":               uniqueName("sa-pat"),
			"scopes":             []string{"read_api"},
			"expires_at":         time.Now().AddDate(0, 0, 30).Format("2006-01-02"),
		},
	})
	requireNoError(t, err, "service_account_pat_create")
	requireTruef(t, pat.ID > 0, "expected PAT ID")

	t.Run("RotatePAT", func(t *testing.T) {
		out, rotateErr := callToolOn[groupserviceaccounts.PATOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "service_account_pat_rotate",
			"params": map[string]any{
				"group_id":           grp.gidStr(),
				"service_account_id": account.ID,
				"token_id":           pat.ID,
			},
		})
		requireNoError(t, rotateErr, "service_account_pat_rotate")
		requireTruef(t, out.Token != "", "rotation should return the new token value")
		requireTruef(t, out.ID != pat.ID, "rotated token should have a new ID (old %d, new %d)", pat.ID, out.ID)
		t.Logf("Rotated service account PAT %d into %d", pat.ID, out.ID)
	})
}

// ---------------------------------------------------------------------------
// Fixture helpers (groupExtras prefix, shared with groupextras_ce_test.go)
// ---------------------------------------------------------------------------

// groupExtrasEnsureBoard returns an issue board ID for the group, reusing an
// existing board when present or provisioning the group's single CE board
// through the raw SDK. Returns false when no board can be obtained.
func groupExtrasEnsureBoard(ctx context.Context, t *testing.T, grp GroupFixture) (int64, bool) {
	t.Helper()
	boards, _, err := sess.glClient.GL().GroupIssueBoards.ListGroupIssueBoards(grp.ID, &gl.ListGroupIssueBoardsOptions{}, gl.WithContext(ctx))
	if err == nil && len(boards) > 0 {
		return boards[0].ID, true
	}
	board, _, err := sess.glClient.GL().GroupIssueBoards.CreateGroupIssueBoard(grp.ID, &gl.CreateGroupIssueBoardOptions{
		Name: new("E2E Board"),
	}, gl.WithContext(ctx))
	if err != nil {
		t.Logf("cannot provision a group issue board on this instance: %v", err)
		return 0, false
	}
	return board.ID, true
}

// groupExtrasCreateGroupLabel creates a group label through the meta-tool
// and returns its numeric ID for label-backed board lists.
func groupExtrasCreateGroupLabel(ctx context.Context, t *testing.T, grp GroupFixture) int64 {
	t.Helper()
	out, err := callToolOn[grouplabels.Output](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "group_label_create",
		"params": map[string]any{
			"group_id": grp.gidStr(),
			"name":     uniqueName("board-lbl"),
			"color":    "#0000FF",
		},
	})
	requireNoError(t, err, "create group label fixture")
	requireTruef(t, out.ID > 0, "expected group label ID")
	return out.ID
}

// groupExtrasUpload identifies one seeded group markdown upload.
type groupExtrasUpload struct {
	ID       int64
	Secret   string
	Filename string
}

// groupExtrasUploadSecretRe extracts the 32-hex upload secret from the
// upload's url or full_path.
var groupExtrasUploadSecretRe = regexp.MustCompile(`/uploads/([0-9a-f]{32})/`)

// groupExtrasUploadGroupFile seeds a markdown upload via the raw
// POST /groups/:id/uploads endpoint (no client-go wrapper exists). Returns
// false when the endpoint is unavailable on the running GitLab version so
// callers can fall back to the delete-action error paths.
func groupExtrasUploadGroupFile(ctx context.Context, t *testing.T, groupID int64, filename string) (groupExtrasUpload, bool) {
	t.Helper()
	req, err := sess.glClient.GL().UploadRequest(
		http.MethodPost,
		fmt.Sprintf("groups/%d/uploads", groupID),
		strings.NewReader("e2e group upload payload"),
		filename,
		gl.UploadFile,
		nil,
		[]gl.RequestOptionFunc{gl.WithContext(ctx)},
	)
	requireNoError(t, err, "build group upload request")

	var out struct {
		ID       int64  `json:"id"`
		URL      string `json:"url"`
		FullPath string `json:"full_path"`
	}
	if _, doErr := sess.glClient.GL().Do(req, &out); doErr != nil {
		t.Logf("group upload endpoint unavailable (POST /groups/:id/uploads): %v", doErr)
		return groupExtrasUpload{}, false
	}

	secret := ""
	if m := groupExtrasUploadSecretRe.FindStringSubmatch(out.URL + " " + out.FullPath); len(m) == 2 {
		secret = m[1]
	}
	return groupExtrasUpload{ID: out.ID, Secret: secret, Filename: filename}, true
}

// groupExtrasDeleteImportedGroup waits for an asynchronously imported group
// to become visible and deletes it. Best effort: when the import never
// materializes within the budget there is nothing to delete.
func groupExtrasDeleteImportedGroup(path string) {
	const budget = 90 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	_ = Poll(ctx, 3*time.Second, budget, func() (bool, string, error) {
		if _, _, err := sess.glClient.GL().Groups.GetGroup(path, nil, gl.WithContext(ctx)); err != nil {
			//nolint:nilerr // A lookup error means the async import has not materialized yet; keep polling.
			return false, "imported group not visible yet", nil
		}
		_, err := sess.glClient.GL().Groups.DeleteGroup(path, nil, gl.WithContext(ctx))
		return err == nil, "delete imported group", nil
	})
}
