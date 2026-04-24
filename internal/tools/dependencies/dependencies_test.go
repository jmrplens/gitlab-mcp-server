// dependencies_test.go contains unit tests for GitLab project dependency
// listing operations. Tests use httptest to mock the GitLab Dependencies API.

package dependencies

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestListDeps validates the ListDeps handler across success, validation,
// context cancellation, API error, empty results, pagination, and filter paths.
func TestListDeps(t *testing.T) {
	tests := []struct {
		name     string
		input    ListInput
		handler  http.HandlerFunc
		wantErr  bool
		validate func(t *testing.T, out ListOutput)
	}{
		{
			name:  "returns dependencies with vulnerabilities and licenses",
			input: ListInput{ProjectID: "42"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, "/api/v4/projects/42/dependencies")
				testutil.RespondJSONWithPagination(w, http.StatusOK, `[
					{"name":"rails","version":"7.0.4","package_manager":"bundler","dependency_file_path":"Gemfile.lock","vulnerabilities":[{"name":"CVE-2026-001","severity":"high","id":1,"url":"https://vuln.example.com/1"}],"licenses":[{"name":"MIT","url":"https://mit.example.com"}]}
				]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			}),
			validate: func(t *testing.T, out ListOutput) {
				t.Helper()
				if len(out.Dependencies) != 1 {
					t.Fatalf("got %d deps, want 1", len(out.Dependencies))
				}
				d := out.Dependencies[0]
				if d.Name != "rails" {
					t.Errorf("name = %q, want %q", d.Name, "rails")
				}
				if d.PackageManager != "bundler" {
					t.Errorf("pm = %q, want %q", d.PackageManager, "bundler")
				}
				if d.Version != "7.0.4" {
					t.Errorf("version = %q, want %q", d.Version, "7.0.4")
				}
				if d.DependencyFilePath != "Gemfile.lock" {
					t.Errorf("path = %q, want %q", d.DependencyFilePath, "Gemfile.lock")
				}
				if len(d.Vulnerabilities) != 1 {
					t.Fatalf("got %d vulns, want 1", len(d.Vulnerabilities))
				}
				if d.Vulnerabilities[0].Name != "CVE-2026-001" {
					t.Errorf("vuln name = %q, want %q", d.Vulnerabilities[0].Name, "CVE-2026-001")
				}
				if d.Vulnerabilities[0].Severity != "high" {
					t.Errorf("vuln severity = %q, want %q", d.Vulnerabilities[0].Severity, "high")
				}
				if d.Vulnerabilities[0].URL != "https://vuln.example.com/1" {
					t.Errorf("vuln url = %q", d.Vulnerabilities[0].URL)
				}
				if len(d.Licenses) != 1 {
					t.Fatalf("got %d licenses, want 1", len(d.Licenses))
				}
				if d.Licenses[0].Name != "MIT" {
					t.Errorf("license = %q, want %q", d.Licenses[0].Name, "MIT")
				}
				if out.Pagination.TotalItems != 1 {
					t.Errorf("total = %d, want 1", out.Pagination.TotalItems)
				}
			},
		},
		{
			name:  "sends package_manager filter to API",
			input: ListInput{ProjectID: "42", PackageManager: "npm"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestPath(t, r, "/api/v4/projects/42/dependencies")
				pm := r.URL.Query().Get("package_manager")
				if pm != "npm" {
					t.Errorf("package_manager query = %q, want %q", pm, "npm")
				}
				testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
					testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
			}),
			validate: func(t *testing.T, out ListOutput) {
				t.Helper()
				if len(out.Dependencies) != 0 {
					t.Errorf("got %d deps, want 0", len(out.Dependencies))
				}
			},
		},
		{
			name: "sends pagination parameters to API",
			input: ListInput{
				ProjectID:       "42",
				PaginationInput: paginationInput(2, 50),
			},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertQueryParam(t, r, "page", "2")
				testutil.AssertQueryParam(t, r, "per_page", "50")
				testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
					testutil.PaginationHeaders{Page: "2", PerPage: "50", Total: "100", TotalPages: "2"})
			}),
			validate: func(t *testing.T, out ListOutput) {
				t.Helper()
				if out.Pagination.Page != 2 {
					t.Errorf("page = %d, want 2", out.Pagination.Page)
				}
			},
		},
		{
			name:  "returns empty list when no dependencies found",
			input: ListInput{ProjectID: "42"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
					testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
			}),
			validate: func(t *testing.T, out ListOutput) {
				t.Helper()
				if len(out.Dependencies) != 0 {
					t.Errorf("got %d deps, want 0", len(out.Dependencies))
				}
			},
		},
		{
			name:    "returns error when project_id is empty",
			input:   ListInput{},
			wantErr: true,
		},
		{
			name:  "returns error on API failure",
			input: ListInput{ProjectID: "42"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			}),
			wantErr: true,
		},
		{
			name:  "returns error on 404 not found",
			input: ListInput{ProjectID: "999"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Project Not Found"}`)
			}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := tt.handler
			if handler == nil {
				handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					t.Fatal("handler should not be called")
				})
			}
			client := testutil.NewTestClient(t, handler)
			out, err := ListDeps(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.validate != nil {
				tt.validate(t, out)
			}
		})
	}
}

// TestListDeps_CancelledContext verifies ListDeps returns error on cancelled context.
func TestListDeps_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called for cancelled context")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListDeps(ctx, client, ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestCreateExport validates the CreateExport handler across success,
// validation, context cancellation, API error, and export_type paths.
func TestCreateExport(t *testing.T) {
	tests := []struct {
		name     string
		input    CreateExportInput
		handler  http.HandlerFunc
		wantErr  bool
		validate func(t *testing.T, out ExportOutput)
	}{
		{
			name:  "creates export successfully",
			input: CreateExportInput{PipelineID: 100},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPost)
				testutil.AssertRequestPath(t, r, "/api/v4/pipelines/100/dependency_list_exports")
				testutil.RespondJSON(w, http.StatusCreated, `{"id":1,"has_finished":false,"self":"https://gitlab.example.com/api/v4/dependency_list_exports/1","download":""}`)
			}),
			validate: func(t *testing.T, out ExportOutput) {
				t.Helper()
				if out.ID != 1 {
					t.Errorf("ID = %d, want 1", out.ID)
				}
				if out.HasFinished {
					t.Error("expected has_finished to be false")
				}
				if out.Self == "" {
					t.Error("expected self URL to be set")
				}
			},
		},
		{
			name:  "creates export with explicit export_type",
			input: CreateExportInput{PipelineID: 200, ExportType: "sbom"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPost)
				testutil.RespondJSON(w, http.StatusCreated, `{"id":2,"has_finished":false,"self":"https://gitlab.example.com/api/v4/dependency_list_exports/2","download":""}`)
			}),
			validate: func(t *testing.T, out ExportOutput) {
				t.Helper()
				if out.ID != 2 {
					t.Errorf("ID = %d, want 2", out.ID)
				}
			},
		},
		{
			name:    "returns error when pipeline_id is zero",
			input:   CreateExportInput{PipelineID: 0},
			wantErr: true,
		},
		{
			name:    "returns error when pipeline_id is negative",
			input:   CreateExportInput{PipelineID: -1},
			wantErr: true,
		},
		{
			name:  "returns error on API failure",
			input: CreateExportInput{PipelineID: 100},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := tt.handler
			if handler == nil {
				handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					t.Fatal("handler should not be called")
				})
			}
			client := testutil.NewTestClient(t, handler)
			out, err := CreateExport(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.validate != nil {
				tt.validate(t, out)
			}
		})
	}
}

// TestCreateExport_CancelledContext verifies CreateExport returns error on cancelled context.
func TestCreateExport_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called for cancelled context")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := CreateExport(ctx, client, CreateExportInput{PipelineID: 100})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestGetExport validates the GetExport handler across success,
// validation, context cancellation, and API error paths.
func TestGetExport(t *testing.T) {
	tests := []struct {
		name     string
		input    GetExportInput
		handler  http.HandlerFunc
		wantErr  bool
		validate func(t *testing.T, out ExportOutput)
	}{
		{
			name:  "returns finished export with download URL",
			input: GetExportInput{ExportID: 1},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, "/api/v4/dependency_list_exports/1")
				testutil.RespondJSON(w, http.StatusOK, `{"id":1,"has_finished":true,"self":"https://gitlab.example.com/api/v4/dependency_list_exports/1","download":"https://gitlab.example.com/api/v4/dependency_list_exports/1/download"}`)
			}),
			validate: func(t *testing.T, out ExportOutput) {
				t.Helper()
				if out.ID != 1 {
					t.Errorf("ID = %d, want 1", out.ID)
				}
				if !out.HasFinished {
					t.Error("expected has_finished to be true")
				}
				if out.Download == "" {
					t.Error("expected download URL to be set")
				}
			},
		},
		{
			name:  "returns unfinished export",
			input: GetExportInput{ExportID: 2},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, `{"id":2,"has_finished":false,"self":"https://gitlab.example.com/api/v4/dependency_list_exports/2","download":""}`)
			}),
			validate: func(t *testing.T, out ExportOutput) {
				t.Helper()
				if out.HasFinished {
					t.Error("expected has_finished to be false")
				}
			},
		},
		{
			name:    "returns error when export_id is zero",
			input:   GetExportInput{ExportID: 0},
			wantErr: true,
		},
		{
			name:    "returns error when export_id is negative",
			input:   GetExportInput{ExportID: -5},
			wantErr: true,
		},
		{
			name:  "returns error on API 404",
			input: GetExportInput{ExportID: 999},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
			}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := tt.handler
			if handler == nil {
				handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					t.Fatal("handler should not be called")
				})
			}
			client := testutil.NewTestClient(t, handler)
			out, err := GetExport(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.validate != nil {
				tt.validate(t, out)
			}
		})
	}
}

// TestGetExport_CancelledContext verifies GetExport returns error on cancelled context.
func TestGetExport_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called for cancelled context")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetExport(ctx, client, GetExportInput{ExportID: 1})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestDownloadExport validates the DownloadExport handler across success,
// validation, context cancellation, and API error paths.
func TestDownloadExport(t *testing.T) {
	tests := []struct {
		name     string
		input    DownloadExportInput
		handler  http.HandlerFunc
		wantErr  bool
		validate func(t *testing.T, out DownloadOutput)
	}{
		{
			name:  "downloads SBOM content successfully",
			input: DownloadExportInput{ExportID: 1},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, "/api/v4/dependency_list_exports/1/download")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = io.WriteString(w, `{"bomFormat":"CycloneDX","specVersion":"1.4","components":[]}`)
			}),
			validate: func(t *testing.T, out DownloadOutput) {
				t.Helper()
				if !strings.Contains(out.Content, "CycloneDX") {
					t.Errorf("content missing CycloneDX, got %q", out.Content)
				}
			},
		},
		{
			name:  "downloads empty content",
			input: DownloadExportInput{ExportID: 2},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}),
			validate: func(t *testing.T, out DownloadOutput) {
				t.Helper()
				if out.Content != "" {
					t.Errorf("content = %q, want empty", out.Content)
				}
			},
		},
		{
			name:    "returns error when export_id is zero",
			input:   DownloadExportInput{ExportID: 0},
			wantErr: true,
		},
		{
			name:    "returns error when export_id is negative",
			input:   DownloadExportInput{ExportID: -1},
			wantErr: true,
		},
		{
			name:  "returns error on API 404",
			input: DownloadExportInput{ExportID: 999},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
			}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := tt.handler
			if handler == nil {
				handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					t.Fatal("handler should not be called")
				})
			}
			client := testutil.NewTestClient(t, handler)
			out, err := DownloadExport(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.validate != nil {
				tt.validate(t, out)
			}
		})
	}
}

// TestDownloadExport_CancelledContext verifies DownloadExport returns error on cancelled context.
func TestDownloadExport_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called for cancelled context")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := DownloadExport(ctx, client, DownloadExportInput{ExportID: 1})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestDownloadExport_ReadError covers the io.ReadAll error path in DownloadExport
// by sending a chunked response that abruptly closes the connection mid-stream.
func TestDownloadExport_ReadError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "999999")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("partial"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		if hj, ok := w.(http.Hijacker); ok {
			conn, _, _ := hj.Hijack()
			conn.Close()
		}
	}))
	_, err := DownloadExport(context.Background(), client, DownloadExportInput{ExportID: 1})
	if err == nil {
		t.Fatal("expected error from io.ReadAll on broken body, got nil")
	}
}

// paginationInput is a helper that builds a toolutil.PaginationInput with page and perPage.
func paginationInput(page, perPage int) toolutil.PaginationInput {
	return toolutil.PaginationInput{Page: page, PerPage: perPage}
}

// --- Markdown Formatter Tests ---

// TestFormatListMarkdown validates Markdown rendering for dependency lists,
// covering empty lists, single items, and multiple items with vulnerabilities/licenses.
func TestFormatListMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    ListOutput
		contains []string
		excludes []string
	}{
		{
			name:  "renders empty list",
			input: ListOutput{},
			contains: []string{
				"## Project Dependencies",
				"No dependencies found.",
			},
			excludes: []string{"| Name |"},
		},
		{
			name: "renders single dependency without vulns or licenses",
			input: ListOutput{
				Dependencies: []Output{
					{Name: "lodash", Version: "4.17.21", PackageManager: "npm", DependencyFilePath: "package-lock.json"},
				},
			},
			contains: []string{
				"| Name | Version | Package Manager | Vulns | Licenses |",
				"| lodash | 4.17.21 | npm | 0 | 0 |",
			},
		},
		{
			name: "renders dependency with vulnerabilities and licenses",
			input: ListOutput{
				Dependencies: []Output{
					{
						Name: "rails", Version: "7.0.4", PackageManager: "bundler",
						DependencyFilePath: "Gemfile.lock",
						Vulnerabilities:    []VulnerabilityOutput{{Name: "CVE-1", Severity: "high", ID: 1}},
						Licenses:           []LicenseOutput{{Name: "MIT", URL: "https://mit.example.com"}},
					},
				},
			},
			contains: []string{
				"| rails | 7.0.4 | bundler | 1 | 1 |",
			},
		},
		{
			name: "renders multiple dependencies",
			input: ListOutput{
				Dependencies: []Output{
					{Name: "react", Version: "18.2.0", PackageManager: "npm"},
					{Name: "vue", Version: "3.3.0", PackageManager: "npm"},
				},
			},
			contains: []string{
				"| react |",
				"| vue |",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatListMarkdown(tt.input)
			for _, s := range tt.contains {
				if !strings.Contains(got, s) {
					t.Errorf("output missing %q\ngot:\n%s", s, got)
				}
			}
			for _, s := range tt.excludes {
				if strings.Contains(got, s) {
					t.Errorf("output should not contain %q\ngot:\n%s", s, got)
				}
			}
		})
	}
}

// TestFormatExportMarkdown validates Markdown rendering for export status,
// covering exports with and without download URLs and self links.
func TestFormatExportMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    ExportOutput
		contains []string
		excludes []string
	}{
		{
			name:  "renders finished export with download URL",
			input: ExportOutput{ID: 1, HasFinished: true, Self: "https://example.com/self", Download: "https://example.com/download"},
			contains: []string{
				"## Dependency List Export",
				"| ID | 1 |",
				"| Self | https://example.com/self |",
				"| Download | https://example.com/download |",
			},
		},
		{
			name:  "renders unfinished export without download URL",
			input: ExportOutput{ID: 2, HasFinished: false, Self: "https://example.com/self"},
			contains: []string{
				"| ID | 2 |",
			},
			excludes: []string{"| Download |"},
		},
		{
			name:  "renders export without self or download URLs",
			input: ExportOutput{ID: 3, HasFinished: false},
			excludes: []string{
				"| Self |",
				"| Download |",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatExportMarkdown(tt.input)
			for _, s := range tt.contains {
				if !strings.Contains(got, s) {
					t.Errorf("output missing %q\ngot:\n%s", s, got)
				}
			}
			for _, s := range tt.excludes {
				if strings.Contains(got, s) {
					t.Errorf("output should not contain %q\ngot:\n%s", s, got)
				}
			}
		})
	}
}

// TestFormatDownloadMarkdown validates Markdown rendering of downloaded SBOM content.
func TestFormatDownloadMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    DownloadOutput
		contains []string
	}{
		{
			name:  "renders SBOM content in JSON code block",
			input: DownloadOutput{Content: `{"bomFormat":"CycloneDX"}`},
			contains: []string{
				"## Dependency List Export (CycloneDX SBOM)",
				"```json",
				`{"bomFormat":"CycloneDX"}`,
				"```",
			},
		},
		{
			name:  "renders empty content",
			input: DownloadOutput{Content: ""},
			contains: []string{
				"```json",
				"```",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatDownloadMarkdown(tt.input)
			for _, s := range tt.contains {
				if !strings.Contains(got, s) {
					t.Errorf("output missing %q\ngot:\n%s", s, got)
				}
			}
		})
	}
}
