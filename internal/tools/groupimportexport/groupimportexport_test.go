// groupimportexport_test.go contains unit tests for the group import/export MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package groupimportexport

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// errExpectedErr identifies the err expected err constant used by this package.
const errExpectedErr = "expected error"

// errExpNonNilResult identifies the err exp non nil result constant used by this package.
const errExpNonNilResult = "expected non-nil result"

// TestScheduleExport_Success verifies that ScheduleExport calls the correct
// API endpoint and returns a success message.
func TestScheduleExport_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/1/export" && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := ScheduleExport(t.Context(), client, ScheduleExportInput{GroupID: "1"})
	if err != nil {
		t.Fatalf("ScheduleExport() error: %v", err)
	}
	if out.Message == "" {
		t.Error("expected non-empty message")
	}
}

// TestScheduleExport_APIError verifies error handling when the API returns an error.
func TestScheduleExport_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)

	_, err := ScheduleExport(t.Context(), client, ScheduleExportInput{GroupID: "1"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestExportDownload_Success verifies that ExportDownload returns base64-encoded
// content and correct byte size.
func TestExportDownload_Success(t *testing.T) {
	archiveData := []byte("fake-group-tar-gz")
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/1/export/download" && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(archiveData)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := ExportDownload(t.Context(), client, ExportDownloadInput{GroupID: "1"})
	if err != nil {
		t.Fatalf("ExportDownload() error: %v", err)
	}
	if out.SizeBytes != len(archiveData) {
		t.Errorf("SizeBytes = %d, want %d", out.SizeBytes, len(archiveData))
	}
	decoded, err := base64.StdEncoding.DecodeString(out.ContentBase64)
	if err != nil {
		t.Fatalf("base64 decode error: %v", err)
	}
	if !bytes.Equal(decoded, archiveData) {
		t.Error("decoded content mismatch")
	}
}

// TestExportDownload_APIError verifies error handling when the API returns an error.
func TestExportDownload_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)

	_, err := ExportDownload(t.Context(), client, ExportDownloadInput{GroupID: "1"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestImportFile_Success verifies that ImportFile calls the correct API endpoint.
func TestImportFile_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/import" && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)

	tmpFile := filepath.Join(t.TempDir(), "export.tar.gz")
	if err := os.WriteFile(tmpFile, []byte("fake-archive"), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := ImportFile(t.Context(), client, ImportFileInput{
		Name: "test-group",
		Path: "test-group",
		File: tmpFile,
	})
	if err != nil {
		t.Fatalf("ImportFile() error: %v", err)
	}
	if out.Message == "" {
		t.Error("expected non-empty message")
	}
}

// TestImportFile_APIError verifies error handling when the API returns an error.
func TestImportFile_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)

	tmpFile := filepath.Join(t.TempDir(), "export.tar.gz")
	if err := os.WriteFile(tmpFile, []byte("fake"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := ImportFile(t.Context(), client, ImportFileInput{
		Name: "test-group",
		Path: "test-group",
		File: tmpFile,
	})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestFormatScheduleExportMarkdown verifies markdown formatting.
func TestFormatScheduleExportMarkdown(t *testing.T) {
	result := FormatScheduleExportMarkdown(ScheduleExportOutput{Message: "ok"})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
	result = FormatScheduleExportMarkdown(ScheduleExportOutput{})
	if result != nil {
		t.Error("expected nil for empty output")
	}
}

// TestFormatExportDownloadMarkdown verifies download markdown formatting.
func TestFormatExportDownloadMarkdown(t *testing.T) {
	result := FormatExportDownloadMarkdown(ExportDownloadOutput{SizeBytes: 512})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
	result = FormatExportDownloadMarkdown(ExportDownloadOutput{})
	if result != nil {
		t.Error("expected nil for empty output")
	}
}

// TestFormatImportFileMarkdown verifies import markdown formatting.
func TestFormatImportFileMarkdown(t *testing.T) {
	result := FormatImportFileMarkdown(ImportFileOutput{Message: "ok"})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
	result = FormatImportFileMarkdown(ImportFileOutput{})
	if result != nil {
		t.Error("expected nil for empty output")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpCancelledCtx identifies the err exp cancelled ctx constant used by this package.
const errExpCancelledCtx = "expected error for canceled context"

// ---------------------------------------------------------------------------
// ScheduleExport — canceled context
// ---------------------------------------------------------------------------.

// TestScheduleExport_CancelledContext verifies ScheduleExport when cancelled context.
func TestScheduleExport_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := ScheduleExport(ctx, client, ScheduleExportInput{GroupID: "1"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// ExportDownload — canceled context
// ---------------------------------------------------------------------------.

// TestExportDownload_ReadAllError verifies that ExportDownload returns an error
// when io.ReadAll fails due to an abruptly closed connection after partial write.
func TestExportDownload_ReadAllError(t *testing.T) {
	_, err := exportDownloadOutput(failingReader{})
	if err == nil {
		t.Fatal("expected error from failing reader")
	}
}

// TestExportDownload_HTTPShortBodyError verifies ExportDownload returns an error
// when GitLab advertises a longer archive body than it sends.
//
// The mock sets Content-Length to 10 but writes only five bytes. The expected
// read error protects callers from receiving truncated base64 archive content.
func TestExportDownload_HTTPShortBodyError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/1/export/download" && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Length", "10")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("short"))
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)

	_, err := ExportDownload(t.Context(), client, ExportDownloadInput{GroupID: "1"})
	if err == nil {
		t.Fatal("expected error from io.ReadAll with abruptly closed connection")
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}

// TestImportFile_InvalidArchivePath verifies ImportFile rejects invalid local archive paths before making an API call.
func TestImportFile_InvalidArchivePath(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("ImportFile should reject invalid file path before API request")
	}))
	_, err := ImportFile(t.Context(), client, ImportFileInput{Name: "test-group", Path: "test-group", File: ""})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestExportDownload_CancelledContext verifies ExportDownload when cancelled context.
func TestExportDownload_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := ExportDownload(ctx, client, ExportDownloadInput{GroupID: "1"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// ImportFile — canceled context, with parent_id
// ---------------------------------------------------------------------------.

// TestImportFile_CancelledContext verifies ImportFile when cancelled context.
func TestImportFile_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)

	tmpFile := filepath.Join(t.TempDir(), "export.tar.gz")
	if err := os.WriteFile(tmpFile, []byte("fake"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := ImportFile(ctx, client, ImportFileInput{
		Name: "test-group",
		Path: "test-group",
		File: tmpFile,
	})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestImportFile_WithParentID verifies ImportFile when with parent ID.
func TestImportFile_WithParentID(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/import" && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)

	tmpFile := filepath.Join(t.TempDir(), "export.tar.gz")
	if err := os.WriteFile(tmpFile, []byte("fake-archive"), 0o600); err != nil {
		t.Fatal(err)
	}

	parentID := int64(42)
	out, err := ImportFile(context.Background(), client, ImportFileInput{
		Name:     "child-group",
		Path:     "child-group",
		File:     tmpFile,
		ParentID: &parentID,
	})
	if err != nil {
		t.Fatalf("ImportFile() error: %v", err)
	}
	if out.Message == "" {
		t.Error("expected non-empty message")
	}
}

// ---------------------------------------------------------------------------
// FormatMarkdown — dispatch for all types and unknown type
// ---------------------------------------------------------------------------.

// TestFormatMarkdown_ScheduleExportOutput verifies FormatMarkdown when schedule export output.
func TestFormatMarkdown_ScheduleExportOutput(t *testing.T) {
	result := FormatMarkdown(ScheduleExportOutput{Message: "Group export scheduled successfully"})
	if result == nil {
		t.Fatal("expected non-nil result for ScheduleExportOutput")
	}
}

// TestFormatMarkdown_ExportDownloadOutput verifies FormatMarkdown when export download output.
func TestFormatMarkdown_ExportDownloadOutput(t *testing.T) {
	result := FormatMarkdown(ExportDownloadOutput{ContentBase64: "dGVzdA==", SizeBytes: 4})
	if result == nil {
		t.Fatal("expected non-nil result for ExportDownloadOutput")
	}
}

// TestFormatMarkdown_ImportFileOutput verifies FormatMarkdown when import file output.
func TestFormatMarkdown_ImportFileOutput(t *testing.T) {
	result := FormatMarkdown(ImportFileOutput{Message: "Group import started successfully"})
	if result == nil {
		t.Fatal("expected non-nil result for ImportFileOutput")
	}
}

// TestFormatMarkdown_UnknownType verifies FormatMarkdown when unknown type.
func TestFormatMarkdown_UnknownType(t *testing.T) {
	result := FormatMarkdown("unknown type")
	if result != nil {
		t.Error("expected nil for unknown type")
	}
}

// TestFormatMarkdown_EmptyScheduleExportOutput verifies FormatMarkdown when empty schedule export output.
func TestFormatMarkdown_EmptyScheduleExportOutput(t *testing.T) {
	result := FormatMarkdown(ScheduleExportOutput{})
	if result != nil {
		t.Error("expected nil for empty ScheduleExportOutput")
	}
}

// TestFormatMarkdown_EmptyExportDownloadOutput verifies FormatMarkdown when empty export download output.
func TestFormatMarkdown_EmptyExportDownloadOutput(t *testing.T) {
	result := FormatMarkdown(ExportDownloadOutput{})
	if result != nil {
		t.Error("expected nil for empty ExportDownloadOutput")
	}
}

// TestFormatMarkdown_EmptyImportFileOutput verifies FormatMarkdown when empty import file output.
func TestFormatMarkdown_EmptyImportFileOutput(t *testing.T) {
	result := FormatMarkdown(ImportFileOutput{})
	if result != nil {
		t.Error("expected nil for empty ImportFileOutput")
	}
}

// ---------------------------------------------------------------------------
// FormatExportDownloadMarkdown — content check
// ---------------------------------------------------------------------------.

// TestFormatExportDownloadMarkdown_ContentCheck verifies FormatExportDownloadMarkdown when content check.
func TestFormatExportDownloadMarkdown_ContentCheck(t *testing.T) {
	result := FormatExportDownloadMarkdown(ExportDownloadOutput{
		ContentBase64: "dGVzdA==",
		SizeBytes:     512,
	})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	found := false
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			if strings.Contains(tc.Text, "512 bytes") {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected markdown to contain '512 bytes'")
	}
}

// ---------------------------------------------------------------------------
// ValidActions
// ---------------------------------------------------------------------------.

// TestValidActions verifies ValidActions.
func TestValidActions(t *testing.T) {
	actions := ValidActions()
	for _, expected := range []string{"schedule_export", "export_download", "import_file"} {
		if !strings.Contains(actions, expected) {
			t.Errorf("ValidActions() missing %q, got %q", expected, actions)
		}
	}
}

// ---------------------------------------------------------------------------
// ActionSpec route execution
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata verifies group import/export action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 3 {
		t.Fatalf("len(ActionSpecs) = %d, want 3", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "groupimportexport" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
	}
}

// TestActionSpecs_CallRoutes validates all group import/export canonical routes.
func TestActionSpecs_CallRoutes(t *testing.T) {
	client := testutil.NewTestClient(t, groupImportExportHandler())
	specs := ActionSpecs(client)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}

	tmpFile := filepath.Join(t.TempDir(), "export.tar.gz")
	if err := os.WriteFile(tmpFile, []byte("fake-archive"), 0o600); err != nil {
		t.Fatal(err)
	}

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"schedule_export", "gitlab_schedule_group_export", map[string]any{"group_id": "1"}},
		{"download_export", "gitlab_download_group_export", map[string]any{"group_id": "1"}},
		{"import_file", "gitlab_import_group_from_file", map[string]any{"name": "test-group", "path": "test-group", "file": tmpFile}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := specByTool[tt.tool]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.tool)
			}
			result, err := spec.Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
			}
		})
	}
}

// groupImportExportHandler supports group import export handler assertions in groupimportexport tests.
func groupImportExportHandler() http.Handler {
	handler := http.NewServeMux()

	handler.HandleFunc("POST /api/v4/groups/1/export", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})

	handler.HandleFunc("GET /api/v4/groups/1/export/download", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("fake-group-tar-gz"))
	})

	handler.HandleFunc("POST /api/v4/groups/import", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})

	return handler
}
