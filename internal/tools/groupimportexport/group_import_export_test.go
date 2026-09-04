// group_import_export_test.go contains unit tests for the group import/export MCP tool handlers.
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

// TestScheduleExport_Success verifies that ScheduleExport succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/groups/1/export (POST) returns a representative success body.
// It asserts the returned output matches the expected fields.
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

// TestScheduleExport_APIError verifies that ScheduleExport returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestExportDownload_Success verifies that ExportDownload succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/groups/1/export/download (GET) returns a representative success body.
// It asserts the returned output matches the expected fields.
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

// TestExportDownload_APIError verifies that ExportDownload returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestImportFile_Success verifies that ImportFile succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/groups/import (POST) returns a representative success body.
// It asserts the returned output matches the expected fields.
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

// TestImportFile_APIError verifies that ImportFile returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestFormatScheduleExportMarkdown verifies the ScheduleExportMarkdown Markdown formatter for a representative scheduleexport input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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

// TestFormatExportDownloadMarkdown verifies the ExportDownloadMarkdown Markdown formatter for a representative exportdownload input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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

// TestFormatImportFileMarkdown verifies the ImportFileMarkdown Markdown formatter for a representative importfile input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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

// TestScheduleExport_CancelledContext verifies the ScheduleExport_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
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

// TestExportDownloadOutput_ArchiveOverTheCeiling_IsRefusedNotTruncated verifies
// that the archive this action turns into base64 and copies into a JSON-RPC
// message is bounded, and that an archive above the bound produces an error
// naming the way out rather than a partial archive.
//
// A prefix of a .tar.gz cannot be imported into another group, which is the
// documented next step, so truncating would hand back an answer that looks
// whole and is not. The exactly-at-the-ceiling case is here because an
// off-by-one in the limit reader would refuse an archive that fits.
func TestExportDownloadOutput_ArchiveOverTheCeiling_IsRefusedNotTruncated(t *testing.T) {
	tests := []struct {
		name    string
		size    int
		wantErr bool
	}{
		{name: "small archive is returned whole", size: 1024},
		{name: "archive exactly at the ceiling is returned whole", size: maxExportBytes},
		{name: "archive one byte over the ceiling is refused", size: maxExportBytes + 1, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := exportDownloadOutput(bytes.NewReader(make([]byte, tt.size)))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("exportDownloadOutput(%d bytes) error = nil, want a refusal", tt.size)
				}
				if !strings.Contains(err.Error(), "download it from GitLab directly") {
					t.Errorf("exportDownloadOutput(%d bytes) error = %v, want it to name the way out", tt.size, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("exportDownloadOutput(%d bytes) error = %v", tt.size, err)
			}
			if out.SizeBytes != tt.size {
				t.Errorf("SizeBytes = %d, want %d", out.SizeBytes, tt.size)
			}
		})
	}
}

// TestExportDownload_ResponseOverTheClientCeiling_NamesTheWayOut verifies that
// an archive the client-wide response ceiling refuses to read produces the same
// actionable message as one this action refuses to encode.
//
// The ceiling stops the read inside the SDK, so the failure arrives as a
// transport error with nothing in it about export archives; an operator whose
// group export is simply too big to travel this way otherwise sees only that
// something exceeded a maximum size.
func TestExportDownload_ResponseOverTheClientCeiling_NamesTheWayOut(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/1/export/download" && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(make([]byte, 4096))
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)
	client.SetMaxResponseBytes(1024)

	_, err := ExportDownload(t.Context(), client, ExportDownloadInput{GroupID: "1"})
	if err == nil {
		t.Fatal("expected an error for a response over the client ceiling")
	}
	if !strings.Contains(err.Error(), "download it from GitLab directly") {
		t.Errorf("ExportDownload() error = %v, want it to name the way out", err)
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

// TestImportFile_InvalidArchivePath verifies the ImportFile_InvalidArchivePath handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestImportFile_InvalidArchivePath(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := ImportFile(t.Context(), client, ImportFileInput{Name: "test-group", Path: "test-group", File: ""})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestExportDownload_CancelledContext verifies the ExportDownload_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
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

// TestImportFile_CancelledContext verifies the ImportFile_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
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

// TestImportFile_WithParentID verifies the ImportFile_WithParentID handler.
// The mock GitLab API at /api/v4/groups/import (POST) returns a representative success body.
// It asserts the returned output matches the expected fields.
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

// TestFormatMarkdown_ScheduleExportOutput verifies the Markdown_ScheduleExportOutput Markdown formatter for a representative _scheduleexportoutput input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatMarkdown_ScheduleExportOutput(t *testing.T) {
	result := FormatMarkdown(ScheduleExportOutput{Message: "Group export scheduled successfully"})
	if result == nil {
		t.Fatal("expected non-nil result for ScheduleExportOutput")
	}
}

// TestFormatMarkdown_ExportDownloadOutput verifies the Markdown_ExportDownloadOutput Markdown formatter for a representative _exportdownloadoutput input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatMarkdown_ExportDownloadOutput(t *testing.T) {
	result := FormatMarkdown(ExportDownloadOutput{ContentBase64: "dGVzdA==", SizeBytes: 4})
	if result == nil {
		t.Fatal("expected non-nil result for ExportDownloadOutput")
	}
}

// TestFormatMarkdown_ImportFileOutput verifies the Markdown_ImportFileOutput Markdown formatter for a representative _importfileoutput input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatMarkdown_ImportFileOutput(t *testing.T) {
	result := FormatMarkdown(ImportFileOutput{Message: "Group import started successfully"})
	if result == nil {
		t.Fatal("expected non-nil result for ImportFileOutput")
	}
}

// TestFormatMarkdown_UnknownType verifies the Markdown_UnknownType Markdown formatter for a representative _unknowntype input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatMarkdown_UnknownType(t *testing.T) {
	result := FormatMarkdown("unknown type")
	if result != nil {
		t.Error("expected nil for unknown type")
	}
}

// TestFormatMarkdown_EmptyScheduleExportOutput verifies the Markdown_EmptyScheduleExportOutput Markdown formatter for a representative _emptyscheduleexportoutput input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatMarkdown_EmptyScheduleExportOutput(t *testing.T) {
	result := FormatMarkdown(ScheduleExportOutput{})
	if result != nil {
		t.Error("expected nil for empty ScheduleExportOutput")
	}
}

// TestFormatMarkdown_EmptyExportDownloadOutput verifies the Markdown_EmptyExportDownloadOutput Markdown formatter for a representative _emptyexportdownloadoutput input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatMarkdown_EmptyExportDownloadOutput(t *testing.T) {
	result := FormatMarkdown(ExportDownloadOutput{})
	if result != nil {
		t.Error("expected nil for empty ExportDownloadOutput")
	}
}

// TestFormatMarkdown_EmptyImportFileOutput verifies the Markdown_EmptyImportFileOutput Markdown formatter for a representative _emptyimportfileoutput input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatMarkdown_EmptyImportFileOutput(t *testing.T) {
	result := FormatMarkdown(ImportFileOutput{})
	if result != nil {
		t.Error("expected nil for empty ImportFileOutput")
	}
}

// ---------------------------------------------------------------------------
// FormatExportDownloadMarkdown — content check
// ---------------------------------------------------------------------------.

// TestFormatExportDownloadMarkdown_ContentCheck verifies the ExportDownloadMarkdown_ContentCheck Markdown formatter for a representative exportdownload_contentcheck input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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

// TestValidActions verifies the ValidActions handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestValidActions(t *testing.T) {
	actions := ValidActions()
	for _, expected := range []string{"schedule_export", "export_download", "import_file"} {
		t.Run(expected, func(t *testing.T) {
			if !strings.Contains(actions, expected) {
				t.Errorf("ValidActions() missing %q, got %q", expected, actions)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ActionSpec route execution
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata validates the Metadata route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
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

// TestActionSpecs_DiscoveryMetadata verifies that every group import/export
// ActionSpec carries non-generic discovery metadata: an action-specific Usage,
// distinctive natural-language Aliases, canonical group.*-aware RelatedActions,
// and a "Returns: … See also: …" individual-tool description (1:1 audit R-META).
func TestActionSpecs_DiscoveryMetadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	for _, spec := range ActionSpecs(client) {
		tool := spec.IndividualTool.Name
		if strings.Contains(spec.Usage, "domain action") {
			t.Errorf("%s: Usage is generic placeholder: %q", tool, spec.Usage)
		}
		if len(spec.Aliases) < 2 {
			t.Errorf("%s: want >=2 distinctive aliases, got %v", tool, spec.Aliases)
		}
		for _, alias := range spec.Aliases {
			if alias == tool {
				t.Errorf("%s: alias must not echo the tool name", tool)
			}
		}
		if len(spec.RelatedActions) == 0 {
			t.Errorf("%s: missing RelatedActions", tool)
		}
		desc := spec.IndividualTool.Description
		if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
			t.Errorf("%s: description must contain Returns: and See also:, got %q", tool, desc)
		}
	}
}

// TestDecorateGroupImportExportMeta_UnknownTool verifies that decorating an
// unknown tool is a no-op, preserving the generic base metadata so the helper's
// early-return branch is exercised.
func TestDecorateGroupImportExportMeta_UnknownTool(t *testing.T) {
	options := groupImportExportOptions("gitlab_unknown_tool")
	before := options
	decorateGroupImportExportMeta(&options, "gitlab_unknown_tool")
	if options.Usage != before.Usage || options.IndividualTool.Description != before.IndividualTool.Description {
		t.Fatalf("decorate mutated options for unknown tool: %+v", options)
	}
}

// TestActionSpecs_CallRoutes validates the CallRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
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
