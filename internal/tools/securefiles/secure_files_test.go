// secure_files_test.go contains unit tests for the secure file MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package securefiles

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// errExpectedErr identifies the err expected err constant used by this package.
const errExpectedErr = "expected error"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// testFileName identifies the test file name constant used by this package.
const testFileName = "key.pem"

// TestList verifies List.
func TestList(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/secure_files" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"name":"key.pem","checksum":"abc","checksum_algorithm":"sha256"}]`)
	}))
	out, err := List(t.Context(), client, ListInput{ProjectID: "1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Files) != 1 || out.Files[0].Name != testFileName {
		t.Errorf("unexpected files: %+v", out.Files)
	}
}

// TestList_Error verifies List when error.
func TestList_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))
	_, err := List(t.Context(), client, ListInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestShow verifies Show.
func TestShow(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/secure_files/1" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"key.pem","checksum":"abc","checksum_algorithm":"sha256","file_extension":"pem"}`)
	}))
	out, err := Show(t.Context(), client, ShowInput{ProjectID: "1", FileID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Name != testFileName {
		t.Errorf("expected key.pem, got %s", out.Name)
	}
	if out.FileExtension != "pem" {
		t.Errorf("expected file_extension 'pem', got %q", out.FileExtension)
	}
}

// TestShow_Error verifies Show when error.
func TestShow_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))
	_, err := Show(t.Context(), client, ShowInput{ProjectID: "1", FileID: 999})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestShow_InvalidFileID verifies Show when invalid file ID.
func TestShow_InvalidFileID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := Show(t.Context(), client, ShowInput{ProjectID: "1", FileID: 0})
	if err == nil {
		t.Fatal("expected error for zero FileID")
	}
	if !strings.Contains(err.Error(), "file_id") {
		t.Errorf("expected error to mention file_id, got: %v", err)
	}
}

// TestRemove_InvalidFileID verifies Remove answers a non-positive file_id
// itself rather than forwarding it. Zero is the case the guard hangs on: it is
// an id GitLab never issues, and a boundary that let it through would send
// DELETE .../secure_files/0 on behalf of a caller who named no file.
func TestRemove_InvalidFileID(t *testing.T) {
	for _, tt := range []struct {
		name   string
		fileID int64
	}{
		{"zero", 0},
		{"negative", -1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int64
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.WriteHeader(http.StatusNoContent)
			}))
			err := Remove(t.Context(), client, RemoveInput{ProjectID: "1", FileID: tt.fileID})
			if err == nil {
				t.Fatalf("Remove(file_id=%d) = nil, want an error", tt.fileID)
			}
			if !strings.Contains(err.Error(), "file_id") {
				t.Errorf("expected error to mention file_id, got: %v", err)
			}
			if got := calls.Load(); got != 0 {
				t.Errorf("Remove(file_id=%d) reached GitLab %d times, want 0", tt.fileID, got)
			}
		})
	}
}

// TestCreate verifies Create.
func TestCreate(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusCreated, `{"id":2,"name":"cert.pem","checksum":"def","checksum_algorithm":"sha256"}`)
	}))
	content := base64.StdEncoding.EncodeToString([]byte("cert-data"))
	out, err := Create(t.Context(), client, CreateInput{ProjectID: "1", Name: "cert.pem", ContentBase64: content})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 2 {
		t.Errorf("expected ID 2, got %d", out.ID)
	}
}

// TestCreate_Error verifies Create when error.
func TestCreate_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	content := base64.StdEncoding.EncodeToString([]byte("data"))
	_, err := Create(t.Context(), client, CreateInput{ProjectID: "1", Name: "x", ContentBase64: content})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestCreate_FilePath_Success verifies create with file_path.
func TestCreate_FilePath_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusCreated, `{"id":3,"name":"key.pem","checksum":"abc","checksum_algorithm":"sha256"}`)
	}))
	tmpFile := t.TempDir() + "/key.pem"
	if err := os.WriteFile(tmpFile, []byte("private-key-data"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := Create(t.Context(), client, CreateInput{ProjectID: "1", Name: "key.pem", FilePath: tmpFile})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 3 {
		t.Errorf("expected ID 3, got %d", out.ID)
	}
}

// TestCreate_FilePath_NotFound verifies create with nonexistent file_path.
func TestCreate_FilePath_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(t.Context(), client, CreateInput{ProjectID: "1", Name: "x", FilePath: "/nonexistent/file.pem"})
	if err == nil {
		t.Fatal("expected error for nonexistent file_path, got nil")
	}
}

// TestCreate_BothFilePathAndBase64 verifies error when both inputs provided.
func TestCreate_BothFilePathAndBase64(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(t.Context(), client, CreateInput{ProjectID: "1", Name: "x", FilePath: "/tmp/x", ContentBase64: "dGVzdA=="})
	if err == nil {
		t.Fatal("expected error when both file_path and content_base64 provided, got nil")
	}
}

// TestCreate_NeitherInput verifies error when neither input provided.
func TestCreate_NeitherInput(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(t.Context(), client, CreateInput{ProjectID: "1", Name: "x"})
	if err == nil {
		t.Fatal("expected error when neither file_path nor content_base64 provided, got nil")
	}
}

// TestCreate_InvalidBase64 verifies error for invalid base64.
func TestCreate_InvalidBase64(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(t.Context(), client, CreateInput{ProjectID: "1", Name: "x", ContentBase64: "!!!invalid!!!"})
	if err == nil {
		t.Fatal("expected error for invalid base64, got nil")
	}
}

// TestRemove verifies Remove.
func TestRemove(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/secure_files/1" || r.Method != http.MethodDelete {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	err := Remove(t.Context(), client, RemoveInput{ProjectID: "1", FileID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestRemove_Error verifies Remove when error.
func TestRemove_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))
	err := Remove(t.Context(), client, RemoveInput{ProjectID: "1", FileID: 999})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// secureFileListHints is the guidance a secure file list closes with, and
// secureFileCardHints the guidance the detail card closes with. Neither names
// a download route: GitLab serves a secure file's contents only to a CI job
// and this server exposes no such action.
const (
	secureFileListHints = "\n---\n💡 **Next steps:**\n" +
		"- Use `gitlab_show_secure_file` to view details of a specific file\n"
	secureFileCardHints = "\n---\n💡 **Next steps:**\n" +
		"- Use `gitlab_list_secure_files` to see the other secure files in this project\n" +
		"- Use `gitlab_remove_secure_file` to delete it\n"
	secureFileTableHeader = "| ID | Name | Checksum Algorithm | Expires At |\n| --- | --- | --- | --- |\n"
)

// TestFormatListMarkdown verifies the list renders as a whole: the heading
// counting what GitLab reported, the table, and the guidance after the rows.
func TestFormatListMarkdown(t *testing.T) {
	got := FormatListMarkdown(ListOutput{Files: []SecureFileItem{{ID: 1, Name: testFileName}}})
	want := "## Secure Files (1)\n\n" + secureFileTableHeader +
		"| 1 | " + testFileName + " |  | - |\n" + secureFileListHints
	if got != want {
		t.Errorf("FormatListMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatShowMarkdown verifies the detail card of a file GitLab sent
// nothing but an id and a name for: no metadata section, and no label with
// nothing after it.
func TestFormatShowMarkdown(t *testing.T) {
	got := FormatShowMarkdown(SecureFileItem{ID: 1, Name: testFileName})
	want := "## Secure File: " + testFileName + "\n\n" +
		"- **ID**: 1\n" +
		"- **Name**: " + testFileName + "\n" +
		secureFileCardHints
	if got != want {
		t.Errorf("FormatShowMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// List — pagination branch (Page > 0 || PerPage > 0)
// ---------------------------------------------------------------------------.

// TestList_WithPagination verifies List when with pagination.
func TestList_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/secure_files" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSONWithPagination(
			w, http.StatusOK,
			`[{"id":1,"name":"key.pem","checksum":"abc","checksum_algorithm":"sha256"},{"id":2,"name":"cert.pem","checksum":"def","checksum_algorithm":"sha256"}]`,
			testutil.PaginationHeaders{Page: "2", PerPage: "2", Total: "5", TotalPages: "3", NextPage: "3", PrevPage: "1"},
		)
	}))
	out, err := List(t.Context(), client, ListInput{
		ProjectID: "1",
		Page:      2, PerPage: 2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Files) != 2 {
		t.Fatalf("len(Files) = %d, want 2", len(out.Files))
	}
	if out.Pagination.TotalPages != 3 {
		t.Errorf("TotalPages = %d, want 3", out.Pagination.TotalPages)
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown — empty list
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_Empty verifies an empty list is the one sentence and
// nothing else: no heading counting zero above it and no table header.
func TestFormatListMarkdown_Empty(t *testing.T) {
	got := FormatListMarkdown(ListOutput{})
	want := "No secure files found.\n"
	if got != want {
		t.Errorf("FormatListMarkdown() = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown — with pagination
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithPagination verifies the heading counts the total
// GitLab reported rather than the page length, and that the pagination footer
// follows the rows.
func TestFormatListMarkdown_WithPagination(t *testing.T) {
	got := FormatListMarkdown(ListOutput{
		Files: []SecureFileItem{
			{ID: 1, Name: "key.pem", ChecksumAlgorithm: "sha256"},
			{ID: 2, Name: "cert.pem", ChecksumAlgorithm: "sha256"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 5, Page: 1, PerPage: 2, TotalPages: 3},
	})
	want := "## Secure Files (5)\n\n" +
		"Showing 2 of 5 results (page 1 of 3)\n\n" + secureFileTableHeader +
		"| 1 | key.pem | sha256 | - |\n" +
		"| 2 | cert.pem | sha256 | - |\n" +
		"\nPage 1 of 3 | 5 items total | 2 per page\n" + secureFileListHints
	if got != want {
		t.Errorf("FormatListMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// fullSecureFileJSON is a secure file response with every SDK field populated,
// including the nested certificate metadata (issuer/subject/expires_at).
//
// No two values in it agree, issuer and subject included. They used to share a
// country and an organization, which is what a real certificate looks like and
// exactly what hides a converter reading the subject's key onto the issuer:
// with "US" on both sides the swap is unobservable, and no mutation or
// condition gate scores a straight-line assignment.
const fullSecureFileJSON = `{
	"id": 7,
	"name": "cert.cer",
	"checksum": "deadbeef",
	"checksum_algorithm": "sha256",
	"file_extension": "cer",
	"created_at": "2024-01-02T03:04:05Z",
	"expires_at": "2025-01-02T03:04:05Z",
	"metadata": {
		"id": "00:11:22",
		"issuer": {"C": "US", "O": "Acme Trust", "CN": "Acme Root CA", "OU": "Sec"},
		"subject": {"C": "ES", "O": "Acme Shop", "CN": "service.example.com", "OU": "Eng", "UID": "u-1"},
		"expires_at": "2026-06-01T00:00:00Z"
	}
}`

// TestShow_FullMetadata verifies Show maps every SecureFile field including the
// nested certificate metadata sub-object (issuer, subject, expires_at).
func TestShow_FullMetadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/secure_files/7" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, fullSecureFileJSON)
	}))
	out, err := Show(t.Context(), client, ShowInput{ProjectID: "1", FileID: 7})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	// The identity and the digest are asserted here because nothing else drives
	// them through a handler: the formatter tests build the item by hand, so a
	// converter that dropped the checksum would reach a caller unnoticed.
	if out.ID != 7 || out.Name != "cert.cer" {
		t.Errorf("identity = (%d, %q), want (7, \"cert.cer\")", out.ID, out.Name)
	}
	if out.Checksum != "deadbeef" || out.ChecksumAlgorithm != "sha256" {
		t.Errorf("digest = (%q, %q), want (\"deadbeef\", \"sha256\")", out.Checksum, out.ChecksumAlgorithm)
	}
	if out.FileExtension != "cer" {
		t.Errorf("FileExtension = %q, want cer", out.FileExtension)
	}
	if out.CreatedAt == nil || !out.CreatedAt.Equal(time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Errorf("CreatedAt = %v, want 2024-01-02T03:04:05Z", out.CreatedAt)
	}
	if out.ExpiresAt == nil || !out.ExpiresAt.Equal(time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Errorf("ExpiresAt = %v, want 2025-01-02T03:04:05Z", out.ExpiresAt)
	}
	if out.Metadata == nil {
		t.Fatal("expected non-nil Metadata")
	}
	if out.Metadata.ID != "00:11:22" {
		t.Errorf("Metadata.ID = %q, want 00:11:22", out.Metadata.ID)
	}
	wantIssuer := SecureFileIssuer{C: "US", O: "Acme Trust", CN: "Acme Root CA", OU: "Sec"}
	if out.Metadata.Issuer != wantIssuer {
		t.Errorf("Issuer = %+v, want %+v", out.Metadata.Issuer, wantIssuer)
	}
	wantSubject := SecureFileSubject{C: "ES", O: "Acme Shop", CN: "service.example.com", OU: "Eng", UID: "u-1"}
	if out.Metadata.Subject != wantSubject {
		t.Errorf("Subject = %+v, want %+v", out.Metadata.Subject, wantSubject)
	}
	if out.Metadata.ExpiresAt == nil || !out.Metadata.ExpiresAt.Equal(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("Metadata.ExpiresAt = %v, want 2026-06-01T00:00:00Z", out.Metadata.ExpiresAt)
	}
}

// TestList_KeysetPagination verifies List forwards keyset pagination
// (order_by, sort, pagination, page_token) onto the request query string.
func TestList_KeysetPagination(t *testing.T) {
	var gotQuery string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/secure_files" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		gotQuery = r.URL.RawQuery
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := List(t.Context(), client, ListInput{
		ProjectID:  "1",
		OrderBy:    "id",
		Sort:       "desc",
		Pagination: "keyset", PageToken: "42",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	for _, want := range []string{"order_by=id", "sort=desc", "pagination=keyset", "page_token=42"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(gotQuery, want) {
				t.Errorf("query %q missing %q", gotQuery, want)
			}
		})
	}
}

// TestFormatShowMarkdown_FullMetadata verifies the detail card renders the
// timestamps in the display form (non-nil branch) and the parsed metadata as a
// section of its own, whole.
func TestFormatShowMarkdown_FullMetadata(t *testing.T) {
	created := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	expires := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	mdExpires := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	got := FormatShowMarkdown(SecureFileItem{
		ID:                7,
		Name:              "cert.cer",
		Checksum:          "deadbeef",
		ChecksumAlgorithm: "sha256",
		CreatedAt:         &created,
		ExpiresAt:         &expires,
		FileExtension:     "cer",
		Metadata: &SecureFileMetadata{
			ID:        "00:11:22",
			Issuer:    SecureFileIssuer{CN: "Acme Root CA"},
			Subject:   SecureFileSubject{CN: "service.example.com"},
			ExpiresAt: &mdExpires,
		},
	})
	want := "## Secure File: cert.cer\n\n" +
		"- **ID**: 7\n" +
		"- **Name**: cert.cer\n" +
		"- **Checksum**: `deadbeef`\n" +
		"- **Algorithm**: sha256\n" +
		"- **Created At**: 2 Jan 2024 03:04 UTC\n" +
		"- **Expires At**: 2 Jan 2025 03:04 UTC\n" +
		"- **File Extension**: cer\n\n" +
		"### Certificate Metadata\n\n" +
		"- **ID**: 00:11:22\n" +
		"- **Expires At**: 1 Jun 2026 00:00 UTC\n" +
		"- **Issuer CN**: Acme Root CA\n" +
		"- **Subject CN**: service.example.com\n" +
		secureFileCardHints
	if got != want {
		t.Errorf("FormatShowMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatShowMarkdown_MetadataSectionNamesTheFileKind verifies each
// extension GitLab parses a provisioning profile from is headed as one, that
// everything else is headed as a certificate, and that metadata carrying
// nothing a reader would see opens no section at all: a heading over four
// empty labels is what the old renderer printed.
//
// Both Apple spellings are listed because the extension is what decides the
// heading, and a case that matches only one of them misnames every field under
// it for files carrying the other.
func TestFormatShowMarkdown_MetadataSectionNamesTheFileKind(t *testing.T) {
	for _, tt := range []struct {
		name    string
		ext     string
		heading string
	}{
		{"mobileprovision", "mobileprovision", "Provisioning Profile Metadata"},
		{"provisionprofile", "provisionprofile", "Provisioning Profile Metadata"},
		{"dotted and upper case", ".ProvisionProfile", "Provisioning Profile Metadata"},
		{"certificate", "cer", "Certificate Metadata"},
		{"no extension", "", "Certificate Metadata"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatShowMarkdown(SecureFileItem{
				ID: 3, Name: "app", FileExtension: tt.ext,
				Metadata: &SecureFileMetadata{ID: "aa-bb"},
			})
			extRow := ""
			if tt.ext != "" {
				extRow = "- **File Extension**: " + tt.ext + "\n"
			}
			want := "## Secure File: app\n\n" +
				"- **ID**: 3\n" +
				"- **Name**: app\n" + extRow + "\n" +
				"### " + tt.heading + "\n\n" +
				"- **ID**: aa-bb\n" +
				secureFileCardHints
			if got != want {
				t.Errorf("FormatShowMarkdown() =\n%q\nwant:\n%q", got, want)
			}
		})
	}

	t.Run("empty metadata opens no section", func(t *testing.T) {
		got := FormatShowMarkdown(SecureFileItem{ID: 4, Name: "blob.bin", Metadata: &SecureFileMetadata{}})
		want := "## Secure File: blob.bin\n\n" +
			"- **ID**: 4\n" +
			"- **Name**: blob.bin\n" +
			secureFileCardHints
		if got != want {
			t.Errorf("FormatShowMarkdown() =\n%q\nwant:\n%q", got, want)
		}
	})
}

// TestFormatShowMarkdown_FileExtension verifies the file extension reaches the
// rendered detail. It is read off the captured response because the SDK does
// not model it, so a formatter that dropped it would leave that read with
// nothing to show for itself.
func TestFormatShowMarkdown_FileExtension(t *testing.T) {
	withExt := FormatShowMarkdown(SecureFileItem{ID: 1, Name: "keystore.jks", FileExtension: "jks"})
	wantWith := "## Secure File: keystore.jks\n\n" +
		"- **ID**: 1\n" +
		"- **Name**: keystore.jks\n" +
		"- **File Extension**: jks\n" +
		secureFileCardHints
	if withExt != wantWith {
		t.Errorf("FormatShowMarkdown() =\n%q\nwant:\n%q", withExt, wantWith)
	}
	without := FormatShowMarkdown(SecureFileItem{ID: 1, Name: "keystore"})
	wantWithout := "## Secure File: keystore\n\n" +
		"- **ID**: 1\n" +
		"- **Name**: keystore\n" +
		secureFileCardHints
	if without != wantWithout {
		t.Errorf("FormatShowMarkdown() =\n%q\nwant:\n%q", without, wantWithout)
	}
}

// TestSecureFiles_UnreadableCapturedFileExtension verifies that every secure
// file handler returns an error rather than a half-filled file when GitLab
// sends file_extension as something that is not a string. The SDK ignores the
// key its own SecureFile does not model, so the read of the captured response
// is the only thing that can notice.
func TestSecureFiles_UnreadableCapturedFileExtension(t *testing.T) {
	// A list answers with an array and the rest with an object, so each case
	// drives a client of its own rather than one shared handler.
	poisoned := func(body string) *gitlabclient.Client {
		return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondJSON(w, http.StatusOK, body)
		}))
	}
	testutil.AssertCapturedDecodeFailures(t, []testutil.CapturedCase{
		{Name: "list", Call: func() error {
			client := poisoned(`[{"id":1,"name":"keystore.jks","file_extension":42}]`)
			_, err := List(context.Background(), client, ListInput{ProjectID: "42"})
			return err
		}},
		{Name: "show", Call: func() error {
			client := poisoned(`{"id":1,"name":"keystore.jks","file_extension":42}`)
			_, err := Show(context.Background(), client, ShowInput{ProjectID: "42", FileID: 1})
			return err
		}},
		{Name: "create", Call: func() error {
			client := poisoned(`{"id":1,"name":"keystore.jks","file_extension":42}`)
			_, err := Create(context.Background(), client, CreateInput{ProjectID: "42", Name: "keystore.jks", ContentBase64: "aGVsbG8="})
			return err
		}},
	})
}

// TestList_MapsEachFileFromItsOwnEntry verifies a list of two files carries
// each file's own fields rather than the first entry's. The extension is the
// one that can slip: it is read off the captured response into a slice indexed
// beside the decoded files, so a loop reusing one entry for every file answers
// correctly for a one-file fixture and silently mislabels every list after it.
func TestList_MapsEachFileFromItsOwnEntry(t *testing.T) {
	created := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)
	expires := time.Date(2025, 11, 12, 13, 14, 15, 0, time.UTC)
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/secure_files" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `[
			{"id":11,"name":"first.pem","checksum":"aaa","checksum_algorithm":"sha256","file_extension":"pem"},
			{"id":22,"name":"second.jks","checksum":"bbb","checksum_algorithm":"sha512","file_extension":"jks",
			 "created_at":"2024-05-06T07:08:09Z","expires_at":"2025-11-12T13:14:15Z"}
		]`)
	}))
	out, err := List(t.Context(), client, ListInput{ProjectID: "1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	want := []SecureFileItem{
		{ID: 11, Name: "first.pem", Checksum: "aaa", ChecksumAlgorithm: "sha256", FileExtension: "pem"},
		{
			ID: 22, Name: "second.jks", Checksum: "bbb", ChecksumAlgorithm: "sha512", FileExtension: "jks",
			CreatedAt: &created, ExpiresAt: &expires,
		},
	}
	if len(out.Files) != len(want) {
		t.Fatalf("len(Files) = %d, want %d", len(out.Files), len(want))
	}
	for i, w := range want {
		t.Run(w.Name, func(t *testing.T) {
			if !reflect.DeepEqual(out.Files[i], w) {
				t.Errorf("Files[%d] = %+v, want %+v", i, out.Files[i], w)
			}
		})
	}
}

// TestCreate_UploadsTheNamedFileWithItsContent verifies the name the caller
// asked for and the bytes it named are what GitLab receives, by both input
// routes. Nothing else in this package reads the request body, so a handler
// sending an empty name or somebody else's reader would still be answered by
// the fixture and would look correct from the output alone.
func TestCreate_UploadsTheNamedFileWithItsContent(t *testing.T) {
	const wantName = "deploy.cer"
	const wantContent = "certificate-bytes"
	for _, tt := range []struct {
		name string
		// input builds the call from a directory it may write the file into,
		// rather than from *testing.T, so the closure is an input builder and
		// not a second test helper.
		input func(dir string) CreateInput
	}{
		{"content_base64", func(string) CreateInput {
			return CreateInput{ProjectID: "1", Name: wantName, ContentBase64: base64.StdEncoding.EncodeToString([]byte(wantContent))}
		}},
		{"file_path", func(dir string) CreateInput {
			return CreateInput{ProjectID: "1", Name: wantName, FilePath: dir + "/deploy.cer"}
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(dir+"/deploy.cer", []byte(wantContent), 0o600); err != nil {
				t.Fatal(err)
			}
			var gotField, gotFilename, gotContent string
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v4/projects/1/secure_files" || r.Method != http.MethodPost {
					http.NotFound(w, r)
					return
				}
				gotField, gotFilename, gotContent = readUploadParts(t, r)
				testutil.RespondJSON(w, http.StatusCreated, `{"id":9,"name":"`+wantName+`"}`)
			}))
			if _, err := Create(t.Context(), client, tt.input(dir)); err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			// GitLab reads the name twice: as a form field and as the multipart
			// part's filename, and client-go fills both from the same option.
			if gotField != wantName || gotFilename != wantName {
				t.Errorf("name = (field %q, filename %q), want both %q", gotField, gotFilename, wantName)
			}
			if gotContent != wantContent {
				t.Errorf("uploaded content = %q, want %q", gotContent, wantContent)
			}
		})
	}
}

// readUploadParts returns the "name" form field, the uploaded part's filename
// and its bytes from a multipart create request. It reports through t.Errorf
// rather than aborting, because it runs on the httptest server's goroutine.
func readUploadParts(t *testing.T, r *http.Request) (field, filename, content string) {
	t.Helper()
	reader, readerErr := r.MultipartReader()
	if readerErr != nil {
		t.Errorf("MultipartReader() error: %v", readerErr)
		return "", "", ""
	}
	for {
		part, partErr := reader.NextPart()
		if errors.Is(partErr, io.EOF) {
			return field, filename, content
		}
		if partErr != nil {
			t.Errorf("NextPart() error: %v", partErr)
			return field, filename, content
		}
		body, readErr := io.ReadAll(part)
		if readErr != nil {
			t.Errorf("reading part %q: %v", part.FormName(), readErr)
			return field, filename, content
		}
		switch part.FormName() {
		case "name":
			field = string(body)
		case "file":
			filename, content = part.FileName(), string(body)
		}
	}
}

// TestList_ZeroExpiresAt_RendersTheDash verifies a file GitLab dates at the
// zero instant reads as "no expiry" in the table rather than as a year-one
// timestamp. The nil pointer and the zero time are two different answers from
// GitLab and the column has one placeholder for both, which only the whole
// chain from the response to the cell can show.
func TestList_ZeroExpiresAt_RendersTheDash(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/secure_files" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK,
			`[{"id":1,"name":"zero.pem","checksum_algorithm":"sha256","expires_at":"0001-01-01T00:00:00Z"}]`)
	}))
	out, err := List(t.Context(), client, ListInput{ProjectID: "1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Files[0].ExpiresAt == nil || !out.Files[0].ExpiresAt.IsZero() {
		t.Fatalf("ExpiresAt = %v, want a non-nil zero time", out.Files[0].ExpiresAt)
	}
	got := FormatListMarkdown(out)
	want := "## Secure Files (1)\n\n" + secureFileTableHeader +
		"| 1 | zero.pem | sha256 | - |\n" + secureFileListHints
	if got != want {
		t.Errorf("FormatListMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestSecureFiles_ErrorHints_NameTheActionThatFixesThem verifies each handler
// attaches its own suggestion, and attaches it only to the status it was
// written for. The hint is what a model does next, so one handler carrying
// another's would send it to look up the wrong identifier, and a hint about a
// missing file pinned to a server error would be advice about nothing.
func TestSecureFiles_ErrorHints_NameTheActionThatFixesThem(t *testing.T) {
	serving := func(status int) *gitlabclient.Client {
		return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondJSON(w, status, `{"message":"nope"}`)
		}))
	}
	for _, tt := range []struct {
		name   string
		status int
		hint   string
		call   func(client *gitlabclient.Client) error
	}{
		{"list", http.StatusNotFound, "verify project_id with gitlab_project_get", func(c *gitlabclient.Client) error {
			_, err := List(t.Context(), c, ListInput{ProjectID: "1"})
			return err
		}},
		{"show", http.StatusNotFound, "verify file_id with gitlab_list_secure_files", func(c *gitlabclient.Client) error {
			_, err := Show(t.Context(), c, ShowInput{ProjectID: "1", FileID: 1})
			return err
		}},
		{"remove", http.StatusNotFound, "verify file_id with gitlab_list_secure_files", func(c *gitlabclient.Client) error {
			return Remove(t.Context(), c, RemoveInput{ProjectID: "1", FileID: 1})
		}},
		{"create", http.StatusBadRequest, "check file content is valid base64 and name is unique within the project", func(c *gitlabclient.Client) error {
			_, err := Create(t.Context(), c, CreateInput{ProjectID: "1", Name: "x", ContentBase64: "ZGF0YQ=="})
			return err
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			want := "Suggestion: " + tt.hint
			t.Run("hinted status", func(t *testing.T) {
				err := tt.call(serving(tt.status))
				if err == nil {
					t.Fatal(errExpectedErr)
				}
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %v does not carry %q", err, want)
				}
			})
			t.Run("other status", func(t *testing.T) {
				err := tt.call(serving(http.StatusInternalServerError))
				if err == nil {
					t.Fatal(errExpectedErr)
				}
				if strings.Contains(err.Error(), want) {
					t.Errorf("error %v carries %q, which is advice for %d only", err, want, tt.status)
				}
			})
		})
	}
}

// TestFormatShowMarkdown_MetadataSectionOpensForAnyFieldAlone verifies each of
// the four values the section can render opens it on its own. GitLab fills
// what the uploaded file happens to carry, so a disjunction that lost a term
// would drop a whole section for the files whose only content is that term,
// and a fixture setting all four at once cannot tell the terms apart.
func TestFormatShowMarkdown_MetadataSectionOpensForAnyFieldAlone(t *testing.T) {
	expires := time.Date(2027, 3, 4, 5, 6, 7, 0, time.UTC)
	for _, tt := range []struct {
		name string
		meta SecureFileMetadata
		row  string
	}{
		{"id alone", SecureFileMetadata{ID: "aa-bb"}, "- **ID**: aa-bb\n"},
		{"expiry alone", SecureFileMetadata{ExpiresAt: &expires}, "- **Expires At**: 4 Mar 2027 05:06 UTC\n"},
		{"issuer alone", SecureFileMetadata{Issuer: SecureFileIssuer{CN: "Acme Root CA"}}, "- **Issuer CN**: Acme Root CA\n"},
		{"subject alone", SecureFileMetadata{Subject: SecureFileSubject{CN: "service.example.com"}}, "- **Subject CN**: service.example.com\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			meta := tt.meta
			got := FormatShowMarkdown(SecureFileItem{ID: 5, Name: "one.cer", Metadata: &meta})
			want := "## Secure File: one.cer\n\n" +
				"- **ID**: 5\n" +
				"- **Name**: one.cer\n\n" +
				"### Certificate Metadata\n\n" + tt.row + secureFileCardHints
			if got != want {
				t.Errorf("FormatShowMarkdown() =\n%q\nwant:\n%q", got, want)
			}
		})
	}
}

// TestFormatShowMarkdown_BlankName_HeadsTheCardWithTheResource verifies a file
// GitLab named nothing still gets a heading a reader can read, rather than one
// ending in a colon with nothing behind it. Whitespace is the same case: the
// name is trimmed before it is judged, and an untrimmed check would head the
// card "Secure File:    ".
func TestFormatShowMarkdown_BlankName_HeadsTheCardWithTheResource(t *testing.T) {
	for _, tt := range []struct {
		name     string
		fileName string
	}{
		{"absent", ""},
		{"whitespace", "   "},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatShowMarkdown(SecureFileItem{ID: 8, Name: tt.fileName})
			want := "## Secure File\n\n- **ID**: 8\n" + secureFileCardHints
			if got != want {
				t.Errorf("FormatShowMarkdown() =\n%q\nwant:\n%q", got, want)
			}
		})
	}
}

// TestFormatListMarkdown_ExpiresColumn verifies the list table renders the
// Expires At column in the display form every other table in this server
// shows, and the "-" placeholder for a file with no expiry.
func TestFormatListMarkdown_ExpiresColumn(t *testing.T) {
	expires := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	got := FormatListMarkdown(ListOutput{Files: []SecureFileItem{
		{ID: 1, Name: "a.pem", ChecksumAlgorithm: "sha256", ExpiresAt: &expires},
		{ID: 2, Name: "b.pem", ChecksumAlgorithm: "sha256"},
	}})
	want := "## Secure Files (2)\n\n" + secureFileTableHeader +
		"| 1 | a.pem | sha256 | 2 Jan 2025 03:04 UTC |\n" +
		"| 2 | b.pem | sha256 | - |\n" + secureFileListHints
	if got != want {
		t.Errorf("FormatListMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}
