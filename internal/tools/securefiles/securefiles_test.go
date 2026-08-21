// securefiles_test.go contains unit tests for the secure file MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package securefiles

import (
	"encoding/base64"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"key.pem","checksum":"abc","checksum_algorithm":"sha256"}`)
	}))
	out, err := Show(t.Context(), client, ShowInput{ProjectID: "1", FileID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Name != testFileName {
		t.Errorf("expected key.pem, got %s", out.Name)
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

// TestRemove_InvalidFileID verifies Remove when invalid file ID.
func TestRemove_InvalidFileID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	err := Remove(t.Context(), client, RemoveInput{ProjectID: "1", FileID: -1})
	if err == nil {
		t.Fatal("expected error for negative FileID")
	}
	if !strings.Contains(err.Error(), "file_id") {
		t.Errorf("expected error to mention file_id, got: %v", err)
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

// TestFormatListMarkdown verifies FormatListMarkdown.
func TestFormatListMarkdown(t *testing.T) {
	md := FormatListMarkdown(ListOutput{Files: []SecureFileItem{{ID: 1, Name: testFileName}}})
	if md == "" {
		t.Error("expected non-empty markdown")
	}
}

// TestFormatShowMarkdown verifies FormatShowMarkdown.
func TestFormatShowMarkdown(t *testing.T) {
	md := FormatShowMarkdown(SecureFileItem{ID: 1, Name: testFileName})
	if md == "" {
		t.Error("expected non-empty markdown")
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

// TestFormatListMarkdown_Empty verifies FormatListMarkdown when empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No secure files found") {
		t.Errorf("expected empty message:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table when empty")
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown — with pagination
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithPagination verifies FormatListMarkdown when with pagination.
func TestFormatListMarkdown_WithPagination(t *testing.T) {
	md := FormatListMarkdown(ListOutput{
		Files: []SecureFileItem{
			{ID: 1, Name: "key.pem", ChecksumAlgorithm: "sha256"},
			{ID: 2, Name: "cert.pem", ChecksumAlgorithm: "sha256"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 5, Page: 1, PerPage: 2, TotalPages: 3},
	})
	for _, want := range []string{"| ID |", "| 1 |", "| 2 |", "key.pem", "cert.pem"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// fullSecureFileJSON is a secure file response with every SDK field populated,
// including the nested certificate metadata (issuer/subject/expires_at).
const fullSecureFileJSON = `{
	"id": 7,
	"name": "cert.cer",
	"checksum": "deadbeef",
	"checksum_algorithm": "sha256",
	"created_at": "2024-01-02T03:04:05Z",
	"expires_at": "2025-01-02T03:04:05Z",
	"metadata": {
		"id": "00:11:22",
		"issuer": {"C": "US", "O": "Acme", "CN": "Acme Root CA", "OU": "Sec"},
		"subject": {"C": "US", "O": "Acme", "CN": "service.example.com", "OU": "Eng", "UID": "u-1"},
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
	wantIssuer := SecureFileIssuer{C: "US", O: "Acme", CN: "Acme Root CA", OU: "Sec"}
	if out.Metadata.Issuer != wantIssuer {
		t.Errorf("Issuer = %+v, want %+v", out.Metadata.Issuer, wantIssuer)
	}
	wantSubject := SecureFileSubject{C: "US", O: "Acme", CN: "service.example.com", OU: "Eng", UID: "u-1"}
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
		if !strings.Contains(gotQuery, want) {
			t.Errorf("query %q missing %q", gotQuery, want)
		}
	}
}

// TestFormatShowMarkdown_FullMetadata verifies the detail markdown renders the
// timestamps (non-nil branch) and the certificate metadata section.
func TestFormatShowMarkdown_FullMetadata(t *testing.T) {
	created := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	expires := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	mdExpires := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	md := FormatShowMarkdown(SecureFileItem{
		ID:                7,
		Name:              "cert.cer",
		Checksum:          "deadbeef",
		ChecksumAlgorithm: "sha256",
		CreatedAt:         &created,
		ExpiresAt:         &expires,
		Metadata: &SecureFileMetadata{
			ID:        "00:11:22",
			Issuer:    SecureFileIssuer{CN: "Acme Root CA"},
			Subject:   SecureFileSubject{CN: "service.example.com"},
			ExpiresAt: &mdExpires,
		},
	})
	for _, want := range []string{
		"2024-01-02T03:04:05Z", "2025-01-02T03:04:05Z", "Certificate Metadata",
		"00:11:22", "Acme Root CA", "service.example.com", "2026-06-01T00:00:00Z",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatListMarkdown_ExpiresColumn verifies the list table renders the
// Expires At column, including the "—" placeholder for files without expiry.
func TestFormatListMarkdown_ExpiresColumn(t *testing.T) {
	expires := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	md := FormatListMarkdown(ListOutput{Files: []SecureFileItem{
		{ID: 1, Name: "a.pem", ChecksumAlgorithm: "sha256", ExpiresAt: &expires},
		{ID: 2, Name: "b.pem", ChecksumAlgorithm: "sha256"},
	}})
	if !strings.Contains(md, "Expires At") {
		t.Errorf("missing Expires At header:\n%s", md)
	}
	if !strings.Contains(md, "2025-01-02T03:04:05Z") {
		t.Errorf("missing rendered expiry:\n%s", md)
	}
	if !strings.Contains(md, "—") {
		t.Errorf("missing placeholder for absent expiry:\n%s", md)
	}
}
