// groupimportexport_test.go contains unit tests for the group import/export MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.

package groupimportexport

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const errExpectedErr = "expected error"

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
	if err := os.WriteFile(tmpFile, []byte("fake-archive"), 0644); err != nil {
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
	if err := os.WriteFile(tmpFile, []byte("fake"), 0644); err != nil {
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

const errExpCancelledCtx = "expected error for canceled context"

// ---------------------------------------------------------------------------
// ScheduleExport — canceled context
// ---------------------------------------------------------------------------.

// TestScheduleExport_CancelledContext verifies the behavior of schedule export cancelled context.
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
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/1/export/download" && r.Method == http.MethodGet {
			// Hijack the connection to send a partial HTTP response and close abruptly.
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("response writer does not support hijacking")
			}
			conn, bufrw, err := hj.Hijack()
			if err != nil {
				t.Fatalf("hijack: %v", err)
			}
			_, _ = bufrw.WriteString("HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nTransfer-Encoding: chunked\r\n\r\n")
			_, _ = bufrw.WriteString("5\r\nhello\r\n")
			_ = bufrw.Flush()
			conn.Close()
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

// TestExportDownload_CancelledContext verifies the behavior of export download cancelled context.
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

// TestImportFile_CancelledContext verifies the behavior of import file cancelled context.
func TestImportFile_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)

	tmpFile := filepath.Join(t.TempDir(), "export.tar.gz")
	if err := os.WriteFile(tmpFile, []byte("fake"), 0644); err != nil {
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

// TestImportFile_WithParentID verifies the behavior of import file with parent i d.
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
	if err := os.WriteFile(tmpFile, []byte("fake-archive"), 0644); err != nil {
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

// TestFormatMarkdown_ScheduleExportOutput verifies the behavior of format markdown schedule export output.
func TestFormatMarkdown_ScheduleExportOutput(t *testing.T) {
	result := FormatMarkdown(ScheduleExportOutput{Message: "Group export scheduled successfully"})
	if result == nil {
		t.Fatal("expected non-nil result for ScheduleExportOutput")
	}
}

// TestFormatMarkdown_ExportDownloadOutput verifies the behavior of format markdown export download output.
func TestFormatMarkdown_ExportDownloadOutput(t *testing.T) {
	result := FormatMarkdown(ExportDownloadOutput{ContentBase64: "dGVzdA==", SizeBytes: 4})
	if result == nil {
		t.Fatal("expected non-nil result for ExportDownloadOutput")
	}
}

// TestFormatMarkdown_ImportFileOutput verifies the behavior of format markdown import file output.
func TestFormatMarkdown_ImportFileOutput(t *testing.T) {
	result := FormatMarkdown(ImportFileOutput{Message: "Group import started successfully"})
	if result == nil {
		t.Fatal("expected non-nil result for ImportFileOutput")
	}
}

// TestFormatMarkdown_UnknownType verifies the behavior of format markdown unknown type.
func TestFormatMarkdown_UnknownType(t *testing.T) {
	result := FormatMarkdown("unknown type")
	if result != nil {
		t.Error("expected nil for unknown type")
	}
}

// TestFormatMarkdown_EmptyScheduleExportOutput verifies the behavior of format markdown empty schedule export output.
func TestFormatMarkdown_EmptyScheduleExportOutput(t *testing.T) {
	result := FormatMarkdown(ScheduleExportOutput{})
	if result != nil {
		t.Error("expected nil for empty ScheduleExportOutput")
	}
}

// TestFormatMarkdown_EmptyExportDownloadOutput verifies the behavior of format markdown empty export download output.
func TestFormatMarkdown_EmptyExportDownloadOutput(t *testing.T) {
	result := FormatMarkdown(ExportDownloadOutput{})
	if result != nil {
		t.Error("expected nil for empty ExportDownloadOutput")
	}
}

// TestFormatMarkdown_EmptyImportFileOutput verifies the behavior of format markdown empty import file output.
func TestFormatMarkdown_EmptyImportFileOutput(t *testing.T) {
	result := FormatMarkdown(ImportFileOutput{})
	if result != nil {
		t.Error("expected nil for empty ImportFileOutput")
	}
}

// ---------------------------------------------------------------------------
// FormatExportDownloadMarkdown — content check
// ---------------------------------------------------------------------------.

// TestFormatExportDownloadMarkdown_ContentCheck verifies the behavior of format export download markdown content check.
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

// TestValidActions verifies the behavior of valid actions.
func TestValidActions(t *testing.T) {
	actions := ValidActions()
	for _, expected := range []string{"schedule_export", "export_download", "import_file"} {
		if !strings.Contains(actions, expected) {
			t.Errorf("ValidActions() missing %q, got %q", expected, actions)
		}
	}
}

// ---------------------------------------------------------------------------
// RegisterTools — no panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// RegisterMeta — no panic
// ---------------------------------------------------------------------------.

// TestRegisterMeta_NoPanic verifies the behavior of register meta no panic.
func TestRegisterMeta_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)
}

// ---------------------------------------------------------------------------
// RegisterToolsCallAllThroughMCP — full MCP roundtrip for all 3 tools
// ---------------------------------------------------------------------------.

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newGroupImportExportMCPSession(t)
	ctx := context.Background()

	tmpFile := filepath.Join(t.TempDir(), "export.tar.gz")
	if err := os.WriteFile(tmpFile, []byte("fake-archive"), 0644); err != nil {
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
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.tool,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.tool, err)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(%s) returned error: %s", tt.tool, tc.Text)
					}
				}
				t.Fatalf("CallTool(%s) returned IsError=true", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helper: MCP session factory
// ---------------------------------------------------------------------------.

// newGroupImportExportMCPSession is an internal helper for the groupimportexport package.
func newGroupImportExportMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	handler := http.NewServeMux()

	// Schedule group export
	handler.HandleFunc("POST /api/v4/groups/1/export", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})

	// Download group export
	handler.HandleFunc("GET /api/v4/groups/1/export/download", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("fake-group-tar-gz"))
	})

	// Import group from file
	handler.HandleFunc("POST /api/v4/groups/import", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})

	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}
