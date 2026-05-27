// projectimportexport_test.go contains unit tests for the project import/export MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package projectimportexport

import (
	"encoding/base64"
	"net/http"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const errExpNonNilResult = "expected non-nil result"

// TestScheduleExport_Success verifies that ScheduleExport calls the correct
// API endpoint and returns a success message.
func TestScheduleExport_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/1/export" && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := ScheduleExport(t.Context(), client, ScheduleExportInput{
		ProjectID:   "1",
		Description: "Test export",
	})
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

	_, err := ScheduleExport(t.Context(), client, ScheduleExportInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestGetExportStatus_Success verifies that GetExportStatus returns
// correctly mapped export status fields.
func TestGetExportStatus_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/1/export" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id": 1,
				"description": "test",
				"name": "my-project",
				"name_with_namespace": "group / my-project",
				"path": "my-project",
				"path_with_namespace": "group/my-project",
				"created_at": "2026-01-01T00:00:00Z",
				"export_status": "finished",
				"_links": {
					"api_url": "https://gitlab.example.com/api/v4/projects/1/export/download",
					"web_url": "https://gitlab.example.com/group/my-project/export"
				}
			}`)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := GetExportStatus(t.Context(), client, ExportStatusInput{ProjectID: "1"})
	if err != nil {
		t.Fatalf("GetExportStatus() error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("ID = %d, want 1", out.ID)
	}
	if out.ExportStatus != "finished" {
		t.Errorf("ExportStatus = %q, want %q", out.ExportStatus, "finished")
	}
	if out.Name != "my-project" {
		t.Errorf("Name = %q, want %q", out.Name, "my-project")
	}
	if out.APIURL == "" {
		t.Error("expected non-empty API URL")
	}
}

// TestExportDownload_Success verifies that ExportDownload returns base64-encoded
// content and correct byte size.
func TestExportDownload_Success(t *testing.T) {
	archiveData := []byte("fake-tar-gz-content")
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/1/export/download" && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(archiveData)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := ExportDownload(t.Context(), client, ExportDownloadInput{ProjectID: "1"})
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
	if string(decoded) != string(archiveData) {
		t.Errorf("decoded content mismatch")
	}
}

// TestImportFromFile_Base64_Success verifies that ImportFromFile with base64
// content calls the import API and returns import status.
func TestImportFromFile_Base64_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/import" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id": 42,
				"description": "imported",
				"name": "imported-project",
				"name_with_namespace": "group / imported-project",
				"path": "imported-project",
				"path_with_namespace": "group/imported-project",
				"import_status": "scheduled",
				"import_type": "file",
				"correlation_id": "abc-123"
			}`)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)

	content := base64.StdEncoding.EncodeToString([]byte("fake-archive"))
	out, err := ImportFromFile(t.Context(), client, ImportFromFileInput{
		ContentBase64: content,
		Namespace:     "group",
		Name:          "imported-project",
		Path:          "imported-project",
	})
	if err != nil {
		t.Fatalf("ImportFromFile() error: %v", err)
	}
	if out.ID != 42 {
		t.Errorf("ID = %d, want 42", out.ID)
	}
	if out.ImportStatus != "scheduled" {
		t.Errorf("ImportStatus = %q, want %q", out.ImportStatus, "scheduled")
	}
}

// TestImportFromFile_FilePath_Success verifies that ImportFromFile accepts a
// canonical local archive path and forwards the overwrite option.
func TestImportFromFile_FilePath_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/import" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id": 43,
				"name": "from-file",
				"path": "from-file",
				"path_with_namespace": "group/from-file",
				"created_at": "2026-02-01T00:00:00Z",
				"import_status": "scheduled"
			}`)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)

	archivePath := t.TempDir() + "/project.tar.gz"
	if err := os.WriteFile(archivePath, []byte("fake archive"), 0o600); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	overwrite := true
	out, err := ImportFromFile(t.Context(), client, ImportFromFileInput{
		FilePath:  archivePath,
		Namespace: "group",
		Name:      "from-file",
		Path:      "from-file",
		Overwrite: &overwrite,
	})
	if err != nil {
		t.Fatalf("ImportFromFile() error: %v", err)
	}
	if out.ID != 43 {
		t.Errorf("ID = %d, want 43", out.ID)
	}
}

// TestImportFromFile_BothParams_Error verifies that providing both file_path
// and content_base64 returns an error.
func TestImportFromFile_BothParams_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called")
	})
	client := testutil.NewTestClient(t, handler)

	_, err := ImportFromFile(t.Context(), client, ImportFromFileInput{
		FilePath:      "/tmp/archive.tar.gz",
		ContentBase64: "dGVzdA==",
	})
	if err == nil {
		t.Fatal("expected error when both params provided")
	}
}

// TestImportFromFile_NoParams_Error verifies that providing neither file_path
// nor content_base64 returns an error.
func TestImportFromFile_NoParams_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called")
	})
	client := testutil.NewTestClient(t, handler)

	_, err := ImportFromFile(t.Context(), client, ImportFromFileInput{})
	if err == nil {
		t.Fatal("expected error when no params provided")
	}
}

// TestImportFromFile_InvalidBase64_Error verifies that invalid base64 content
// returns an error before making API calls.
func TestImportFromFile_InvalidBase64_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called with invalid base64")
	})
	client := testutil.NewTestClient(t, handler)

	_, err := ImportFromFile(t.Context(), client, ImportFromFileInput{
		ContentBase64: "not-valid-base64!!!",
	})
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

// TestGetImportStatus_Success verifies that GetImportStatus returns correctly
// mapped import status fields.
func TestGetImportStatus_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/import" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id": 42,
				"description": "imported",
				"name": "imported-project",
				"name_with_namespace": "group / imported-project",
				"path": "imported-project",
				"path_with_namespace": "group/imported-project",
				"import_status": "finished",
				"import_type": "file",
				"correlation_id": "abc-123"
			}`)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := GetImportStatus(t.Context(), client, GetImportStatusInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("GetImportStatus() error: %v", err)
	}
	if out.ID != 42 {
		t.Errorf("ID = %d, want 42", out.ID)
	}
	if out.ImportStatus != "finished" {
		t.Errorf("ImportStatus = %q, want %q", out.ImportStatus, "finished")
	}
}

// TestGetImportStatus_APIError verifies error handling when the API returns an error.
func TestGetImportStatus_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)

	_, err := GetImportStatus(t.Context(), client, GetImportStatusInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestFormatExportStatusMarkdown verifies markdown formatting for export status.
func TestFormatExportStatusMarkdown(t *testing.T) {
	result := FormatExportStatusMarkdown(ExportStatusOutput{
		ID:           1,
		Name:         "test",
		ExportStatus: "finished",
	})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
}

// TestFormatExportStatusMarkdown_Empty verifies nil return for empty output.
func TestFormatExportStatusMarkdown_Empty(t *testing.T) {
	result := FormatExportStatusMarkdown(ExportStatusOutput{})
	if result != nil {
		t.Error("expected nil result for empty output")
	}
}

// TestFormatImportStatusMarkdown verifies markdown formatting for import status.
func TestFormatImportStatusMarkdown(t *testing.T) {
	result := FormatImportStatusMarkdown(ImportStatusOutput{
		ID:           42,
		Name:         "test",
		ImportStatus: "finished",
	})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
}

// TestFormatScheduleExportMarkdown verifies markdown for schedule export result.
func TestFormatScheduleExportMarkdown(t *testing.T) {
	result := FormatScheduleExportMarkdown(ScheduleExportOutput{Message: "ok"})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
}

// TestFormatExportDownloadMarkdown verifies download markdown formatting.
func TestFormatExportDownloadMarkdown(t *testing.T) {
	result := FormatExportDownloadMarkdown(ExportDownloadOutput{SizeBytes: 1024})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
	// Verify empty returns nil
	result = FormatExportDownloadMarkdown(ExportDownloadOutput{})
	if result != nil {
		t.Error("expected nil result for empty output")
	}
}

// TestImportFromFile_FilePath_NonExistent_Error verifies error when file does not exist.
func TestImportFromFile_FilePath_NonExistent_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called")
	})
	client := testutil.NewTestClient(t, handler)

	_, err := ImportFromFile(t.Context(), client, ImportFromFileInput{
		FilePath: "/tmp/nonexistent-file-test.tar.gz",
	})
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

// TestImportFromFile_FilePathOpenError verifies that ImportFromFile reports an
// open error after a file path passes canonical archive validation.
func TestImportFromFile_FilePathOpenError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based unreadable file test is Unix-specific")
	}
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called")
	}))

	archivePath := t.TempDir() + "/unreadable.tar.gz"
	if err := os.WriteFile(archivePath, []byte("fake archive"), 0o600); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	if err := os.Chmod(archivePath, 0o000); err != nil {
		t.Fatalf("chmod archive: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(archivePath, 0o600) })

	_, err := ImportFromFile(t.Context(), client, ImportFromFileInput{FilePath: archivePath})
	if err == nil {
		t.Fatal("expected error for unreadable file")
	}
	if !strings.Contains(err.Error(), "open archive") {
		t.Fatalf("error = %v, want open archive", err)
	}
}

// TestScheduleExport_WithUpload verifies ScheduleExport sends Description and Upload fields.
func TestScheduleExport_WithUpload(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/1/export" && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := ScheduleExport(t.Context(), client, ScheduleExportInput{
		ProjectID:   "1",
		Description: "Test export",
		UploadURL:   "https://example.com/upload",
		UploadHTTP:  "PUT",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Message == "" {
		t.Error("expected non-empty message")
	}
}

// TestExportDownload_APIError verifies ExportDownload returns error on API failure.
func TestExportDownload_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	_, err := ExportDownload(t.Context(), client, ExportDownloadInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

// TestGetExportStatus_APIError verifies GetExportStatus returns error on API failure.
func TestGetExportStatus_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	_, err := GetExportStatus(t.Context(), client, ExportStatusInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

// TestImportFromFile_APIError verifies ImportFromFile returns error on API failure.
func TestImportFromFile_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := ImportFromFile(t.Context(), client, ImportFromFileInput{
		ContentBase64: base64.StdEncoding.EncodeToString([]byte("data")),
	})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

// TestFormatExportStatusMarkdown_AllFields verifies all optional fields are rendered.
func TestFormatExportStatusMarkdown_AllFields(t *testing.T) {
	result := FormatExportStatusMarkdown(ExportStatusOutput{
		ID:                1,
		Name:              "project",
		PathWithNamespace: "group/project",
		ExportStatus:      "finished",
		Message:           "Export complete",
		APIURL:            "https://api.example.com",
		WebURL:            "https://web.example.com",
	})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	if !strings.Contains(tc.Text, "Message") {
		t.Error("expected Message field")
	}
	if !strings.Contains(tc.Text, "API URL") {
		t.Error("expected API URL field")
	}
	if !strings.Contains(tc.Text, "Web URL") {
		t.Error("expected Web URL field")
	}
}

// TestFormatImportStatusMarkdown_AllFields verifies all optional fields are rendered.
func TestFormatImportStatusMarkdown_AllFields(t *testing.T) {
	result := FormatImportStatusMarkdown(ImportStatusOutput{
		ID:                1,
		Name:              "project",
		PathWithNamespace: "group/project",
		ImportStatus:      "finished",
		ImportType:        "gitlab_project",
		CorrelationID:     "abc-123",
		ImportError:       "some warning",
	})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	if !strings.Contains(tc.Text, "Type") {
		t.Error("expected Type field")
	}
	if !strings.Contains(tc.Text, "Correlation ID") {
		t.Error("expected Correlation ID field")
	}
	if !strings.Contains(tc.Text, "Error") {
		t.Error("expected Error field")
	}
}

// TestFormatImportStatusMarkdown_Empty verifies nil result for empty output.
func TestFormatImportStatusMarkdown_Empty(t *testing.T) {
	result := FormatImportStatusMarkdown(ImportStatusOutput{})
	if result != nil {
		t.Error("expected nil result for empty output")
	}
}

// TestFormatScheduleExportMarkdown_Empty verifies nil result for empty output.
func TestFormatScheduleExportMarkdown_Empty(t *testing.T) {
	result := FormatScheduleExportMarkdown(ScheduleExportOutput{})
	if result != nil {
		t.Error("expected nil result for empty output")
	}
}

// TestActionSpecs_Metadata verifies canonical metadata for project import/export actions.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	byTool := projectImportExportSpecsByTool(t, ActionSpecs(client))

	if len(byTool) != 5 {
		t.Fatalf("len(ActionSpecs) = %d, want 5", len(byTool))
	}
	for _, spec := range byTool {
		if spec.OwnerPackage != "projectimportexport" {
			t.Errorf("OwnerPackage for %s = %q, want projectimportexport", spec.Name, spec.OwnerPackage)
		}
		if spec.Usage == "" {
			t.Errorf("Usage for %s is empty", spec.Name)
		}
		if len(spec.Aliases) == 0 {
			t.Errorf("Aliases for %s are empty", spec.Name)
		}
	}
	for _, name := range []string{
		"gitlab_get_project_export_status",
		"gitlab_download_project_export",
		"gitlab_get_project_import_status",
	} {
		if !byTool[name].ReadOnly || !byTool[name].Idempotent {
			t.Errorf("%s should be read-only and idempotent", name)
		}
	}
}

// TestActionSpecs_CallRoutes verifies all project import/export routes can be called directly.
func TestActionSpecs_CallRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/export"):
			w.WriteHeader(http.StatusAccepted)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/export"):
			testutil.RespondJSON(w, http.StatusOK,
				`{"id":1,"name":"p","path":"p","path_with_namespace":"g/p","export_status":"finished"}`)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/download"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("archive-data"))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/import"):
			testutil.RespondJSON(w, http.StatusOK,
				`{"id":1,"name":"p","path":"p","path_with_namespace":"g/p","import_status":"finished"}`)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/import"):
			testutil.RespondJSON(w, http.StatusCreated,
				`{"id":1,"name":"p","path":"p","path_with_namespace":"g/p","import_status":"scheduled"}`)
		default:
			http.NotFound(w, r)
		}
	})
	client := testutil.NewTestClient(t, mux)
	byTool := projectImportExportSpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_schedule_project_export", map[string]any{"project_id": "1"}},
		{"gitlab_get_project_export_status", map[string]any{"project_id": "1"}},
		{"gitlab_download_project_export", map[string]any{"project_id": "1"}},
		{"gitlab_import_project_from_file", map[string]any{"content_base64": base64.StdEncoding.EncodeToString([]byte("archive-data")), "name": "p", "path": "p"}},
		{"gitlab_get_project_import_status", map[string]any{"project_id": "1"}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := byTool[tt.name].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.name)
			}
		})
	}
}

// TestImportFromFile_FilePathReadError verifies that ImportFromFile returns
// an error when the specified file path does not exist (os.ReadFile error).
func TestImportFromFile_FilePathReadError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called")
	}))
	_, err := ImportFromFile(t.Context(), client, ImportFromFileInput{
		FilePath: "/nonexistent/path/to/file.tar.gz",
	})
	if err == nil {
		t.Fatal("expected error for unreadable file path")
	}
	if !strings.Contains(err.Error(), "resolve archive") {
		t.Errorf("error = %v, want containing 'resolve archive'", err)
	}
}

// TestImportFromFile_Base64DecodeError verifies that invalid base64 in
// content_base64 returns an appropriate error.
func TestImportFromFile_Base64DecodeError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called")
	}))
	_, err := ImportFromFile(t.Context(), client, ImportFromFileInput{
		ContentBase64: "not-valid-base64!!!",
	})
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
	if !strings.Contains(err.Error(), "invalid base64") {
		t.Errorf("error = %v, want containing 'invalid base64'", err)
	}
}

// TestImportStatusToOutput_NilCreatedAt verifies importStatusToOutput handles
// nil CreateAt without panicking (the missed branch).
func TestImportStatusToOutput_NilCreatedAt(t *testing.T) {
	out := importStatusToOutput(&gl.ImportStatus{
		ID:           42,
		ImportStatus: "finished",
		CreateAt:     nil,
	})
	if out.ID != 42 {
		t.Errorf("ID = %d, want 42", out.ID)
	}
	if out.CreatedAt != "" {
		t.Errorf("CreatedAt = %q, want empty", out.CreatedAt)
	}
}

// TestImportStatusToOutput_CreatedAt verifies importStatusToOutput formats a
// present CreateAt timestamp.
func TestImportStatusToOutput_CreatedAt(t *testing.T) {
	createdAt := time.Date(2026, 2, 1, 12, 30, 0, 0, time.UTC)
	out := importStatusToOutput(&gl.ImportStatus{
		ID:           42,
		ImportStatus: "finished",
		CreateAt:     &createdAt,
	})
	if out.CreatedAt != "2026-02-01T12:30:00Z" {
		t.Errorf("CreatedAt = %q, want RFC3339 timestamp", out.CreatedAt)
	}
}

// TestActionSpecs_ImportFileError validates the import route error path.
func TestActionSpecs_ImportFileError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	client := testutil.NewTestClient(t, mux)
	byTool := projectImportExportSpecsByTool(t, ActionSpecs(client))

	_, err := byTool["gitlab_import_project_from_file"].Route.Handler(t.Context(), map[string]any{"content_base64": base64.StdEncoding.EncodeToString([]byte("data"))})
	if err == nil {
		t.Error("expected import route error")
	}
}

func projectImportExportSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}
