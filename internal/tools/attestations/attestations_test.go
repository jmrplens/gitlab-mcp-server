// attestations_test.go contains unit tests for GitLab build attestation
// operations. Tests use httptest to mock the GitLab Attestations API.
package attestations

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// --- toOutput ---.

// TestToOutput validates the toOutput conversion function.
// Covers nil input, full fields with all timestamps, and partial fields
// where optional time pointers are nil.
func TestToOutput(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		input    *gl.Attestation
		validate func(t *testing.T, got Output)
	}{
		{
			name:  "nil attestation returns zero output",
			input: nil,
			validate: func(t *testing.T, got Output) {
				t.Helper()
				if got.ID != 0 {
					t.Errorf("ID = %d, want 0", got.ID)
				}
				if got.Status != "" {
					t.Errorf("Status = %q, want empty", got.Status)
				}
			},
		},
		{
			name: "all fields populated including all timestamps",
			input: &gl.Attestation{
				ID:            42,
				IID:           7,
				ProjectID:     10,
				BuildID:       200,
				Status:        "success",
				PredicateKind: "slsa_provenance",
				PredicateType: "https://slsa.dev/provenance/v0.2",
				SubjectDigest: "sha256:deadbeef",
				DownloadURL:   "https://gitlab.example.com/download/42",
				CreatedAt:     &now,
				UpdatedAt:     &now,
				ExpireAt:      &now,
			},
			validate: func(t *testing.T, got Output) {
				t.Helper()
				if got.ID != 42 {
					t.Errorf("ID = %d, want 42", got.ID)
				}
				if got.IID != 7 {
					t.Errorf("IID = %d, want 7", got.IID)
				}
				if got.ProjectID != 10 {
					t.Errorf("ProjectID = %d, want 10", got.ProjectID)
				}
				if got.BuildID != 200 {
					t.Errorf("BuildID = %d, want 200", got.BuildID)
				}
				if got.CreatedAt == "" {
					t.Error("CreatedAt should not be empty")
				}
				if got.UpdatedAt == "" {
					t.Error("UpdatedAt should not be empty")
				}
				if got.ExpireAt == "" {
					t.Error("ExpireAt should not be empty")
				}
				if got.DownloadURL != "https://gitlab.example.com/download/42" {
					t.Errorf("DownloadURL = %q, want download URL", got.DownloadURL)
				}
			},
		},
		{
			name: "nil timestamps remain empty strings",
			input: &gl.Attestation{
				ID:     1,
				Status: "pending",
			},
			validate: func(t *testing.T, got Output) {
				t.Helper()
				if got.CreatedAt != "" {
					t.Errorf("CreatedAt = %q, want empty", got.CreatedAt)
				}
				if got.UpdatedAt != "" {
					t.Errorf("UpdatedAt = %q, want empty", got.UpdatedAt)
				}
				if got.ExpireAt != "" {
					t.Errorf("ExpireAt = %q, want empty", got.ExpireAt)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toOutput(tt.input)
			tt.validate(t, got)
		})
	}
}

// --- FormatOutputMarkdown ---.

// TestFormatOutputMarkdown validates the single-attestation markdown renderer.
// Covers zero-ID (empty), full output with all optional fields, and partial
// output with some optional fields omitted.
func TestFormatOutputMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    Output
		want     string
		notWant  []string
		contains []string
	}{
		{
			name:  "zero ID returns empty string",
			input: Output{},
			want:  "",
		},
		{
			name: "full output includes all fields",
			input: Output{
				ID:            42,
				IID:           7,
				ProjectID:     10,
				BuildID:       200,
				Status:        "success",
				PredicateKind: "slsa_provenance",
				PredicateType: "https://slsa.dev/provenance/v0.2",
				SubjectDigest: "sha256:deadbeef",
				DownloadURL:   "https://gitlab.example.com/download",
				CreatedAt:     "2026-06-15T12:00:00Z",
				ExpireAt:      "2026-06-15T12:00:00Z",
			},
			contains: []string{
				"## Attestation #42 (IID 7)",
				"**Project ID**: 10",
				"**Build ID**: 200",
				"**Status**: success",
				"**Predicate Kind**: slsa_provenance",
				"**Predicate Type**: https://slsa.dev/provenance/v0.2",
				"`sha256:deadbeef`",
				"**Download URL**",
				"**Created**: 2026-06-15T12:00:00Z",
				"**Expires**: 2026-06-15T12:00:00Z",
				"gitlab_download_attestation",
			},
		},
		{
			name: "partial output omits empty optional fields",
			input: Output{
				ID:      1,
				IID:     1,
				BuildID: 100,
				Status:  "pending",
			},
			contains: []string{
				"## Attestation #1 (IID 1)",
				"**Status**: pending",
			},
			notWant: []string{
				"Predicate Kind",
				"Predicate Type",
				"Subject Digest",
				"Download URL",
				"Created",
				"Expires",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatOutputMarkdown(tt.input)
			if tt.want != "" || (len(tt.contains) == 0 && len(tt.notWant) == 0) {
				if got != tt.want {
					t.Errorf("got %q, want %q", got, tt.want)
				}
			}
			for _, s := range tt.contains {
				if !strings.Contains(got, s) {
					t.Errorf("output missing %q", s)
				}
			}
			for _, s := range tt.notWant {
				if strings.Contains(got, s) {
					t.Errorf("output should not contain %q", s)
				}
			}
		})
	}
}

// --- FormatListMarkdown ---.

// TestFormatListMarkdown validates the attestation list markdown table renderer.
// Covers empty list (no attestations message) and populated list with table headers.
func TestFormatListMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    ListOutput
		contains []string
	}{
		{
			name:     "empty list shows no-results message",
			input:    ListOutput{},
			contains: []string{"No attestations found."},
		},
		{
			name: "populated list renders markdown table",
			input: ListOutput{
				Attestations: []Output{
					{
						ID:            1,
						IID:           1,
						BuildID:       100,
						Status:        "success",
						PredicateKind: "slsa_provenance",
						CreatedAt:     "2026-01-01T00:00:00Z",
					},
					{
						ID:        2,
						IID:       2,
						BuildID:   101,
						Status:    "failed",
						CreatedAt: "2026-02-01T00:00:00Z",
					},
				},
			},
			contains: []string{
				"## Attestations (2)",
				"| ID | IID | Build | Status | Predicate Kind | Created |",
				"| 1 | 1 | 100 | success | slsa_provenance | 2026-01-01T00:00:00Z |",
				"| 2 | 2 | 101 | failed |",
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
		})
	}
}

// --- FormatDownloadMarkdown ---.

// TestFormatDownloadMarkdown validates the download result markdown renderer.
// Covers zero IID (empty) and populated output with size and content info.
func TestFormatDownloadMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    DownloadOutput
		want     string
		contains []string
	}{
		{
			name:  "zero IID returns empty string",
			input: DownloadOutput{},
			want:  "",
		},
		{
			name: "populated output shows size and content note",
			input: DownloadOutput{
				AttestationIID: 7,
				Size:           1024,
				ContentBase64:  "dGVzdA==",
			},
			contains: []string{
				"## Attestation Download (IID 7)",
				"**Size**: 1024 bytes",
				"Base64-encoded",
				"content_base64",
				"gitlab_list_attestations",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatDownloadMarkdown(tt.input)
			if tt.want != "" || len(tt.contains) == 0 {
				if tt.want != "" && got != tt.want {
					t.Errorf("got %q, want %q", got, tt.want)
				}
			}
			for _, s := range tt.contains {
				if !strings.Contains(got, s) {
					t.Errorf("output missing %q\ngot:\n%s", s, got)
				}
			}
		})
	}
}

// --- List ---.

// TestList_Success verifies that List returns the expected output when the GitLab API responds successfully.
func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/10/attestations/sha256:abc123" {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id":1,"iid":1,"project_id":10,"build_id":100,"status":"success","predicate_kind":"slsa_provenance","predicate_type":"https://slsa.dev/provenance/v0.2","subject_digest":"sha256:abc123","created_at":"2026-01-01T00:00:00Z"},
				{"id":2,"iid":2,"project_id":10,"build_id":101,"status":"success","predicate_kind":"slsa_provenance","subject_digest":"sha256:abc123","created_at":"2026-02-01T00:00:00Z","expire_at":"2026-02-01T00:00:00Z"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID:     toolutil.StringOrInt("10"),
		SubjectDigest: "sha256:abc123",
	})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(out.Attestations) != 2 {
		t.Fatalf("expected 2 attestations, got %d", len(out.Attestations))
	}
	if out.Attestations[0].Status != "success" {
		t.Errorf("expected status success, got %s", out.Attestations[0].Status)
	}
	if out.Attestations[0].PredicateKind != "slsa_provenance" {
		t.Errorf("expected predicate_kind slsa_provenance, got %s", out.Attestations[0].PredicateKind)
	}
	if out.Attestations[1].ExpireAt == "" {
		t.Error("expected expire_at to be set for second attestation")
	}
}

// TestList_MissingProjectID verifies that List returns a validation error when project_id is missing.
func TestList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := List(context.Background(), client, ListInput{SubjectDigest: "sha256:abc"})
	if err == nil {
		t.Fatal("expected error for empty project_id, got nil")
	}
}

// TestList_MissingSubjectDigest verifies that List returns a validation error when subject_digest is missing.
func TestList_MissingSubjectDigest(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := List(context.Background(), client, ListInput{ProjectID: toolutil.StringOrInt("10")})
	if err == nil {
		t.Fatal("expected error for empty subject_digest, got nil")
	}
}

// TestList_CancelledContext verifies that List returns an error when the context is already cancelled.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := List(ctx, client, ListInput{
		ProjectID:     toolutil.StringOrInt("10"),
		SubjectDigest: "sha256:abc123",
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestList_APIError verifies that List returns an error when the GitLab API responds with a failure status.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/10/attestations/sha256:abc123" {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := List(context.Background(), client, ListInput{
		ProjectID:     toolutil.StringOrInt("10"),
		SubjectDigest: "sha256:abc123",
	})
	if err == nil {
		t.Fatal("expected error for 403 response, got nil")
	}
}

// TestList_EmptyResult verifies that List handles an empty API response and returns a non-nil empty result.
func TestList_EmptyResult(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/10/attestations/sha256:empty" {
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID:     toolutil.StringOrInt("10"),
		SubjectDigest: "sha256:empty",
	})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(out.Attestations) != 0 {
		t.Fatalf("expected 0 attestations, got %d", len(out.Attestations))
	}
}

// --- Download ---.

// TestDownload_Success verifies that Download returns the expected output when the GitLab API responds successfully.
func TestDownload_Success(t *testing.T) {
	content := "attestation-binary-content"
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/10/attestations/1/download" {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(content))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Download(context.Background(), client, DownloadInput{
		ProjectID:      toolutil.StringOrInt("10"),
		AttestationIID: 1,
	})
	if err != nil {
		t.Fatalf("Download() error: %v", err)
	}
	if out.AttestationIID != 1 {
		t.Errorf("expected IID 1, got %d", out.AttestationIID)
	}
	if out.Size != len(content) {
		t.Errorf("expected size %d, got %d", len(content), out.Size)
	}
	decoded, err := base64.StdEncoding.DecodeString(out.ContentBase64)
	if err != nil {
		t.Fatalf("base64 decode error: %v", err)
	}
	if string(decoded) != content {
		t.Errorf("expected content %q, got %q", content, string(decoded))
	}
}

// TestDownload_MissingProjectID verifies that Download returns a validation error when project_id is missing.
func TestDownload_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Download(context.Background(), client, DownloadInput{AttestationIID: 1})
	if err == nil {
		t.Fatal("expected error for empty project_id, got nil")
	}
}

// TestDownload_MissingAttestationIID verifies that Download returns a validation error when attestation_iid is missing.
func TestDownload_MissingAttestationIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Download(context.Background(), client, DownloadInput{ProjectID: toolutil.StringOrInt("10")})
	if err == nil {
		t.Fatal("expected error for zero attestation_iid, got nil")
	}
}

// TestDownload_CancelledContext verifies that Download returns an error when the context is already cancelled.
func TestDownload_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Download(ctx, client, DownloadInput{
		ProjectID:      toolutil.StringOrInt("10"),
		AttestationIID: 1,
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestDownload_APIError verifies that Download returns an error when the GitLab API responds with a failure status.
func TestDownload_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/10/attestations/1/download" {
			testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := Download(context.Background(), client, DownloadInput{
		ProjectID:      toolutil.StringOrInt("10"),
		AttestationIID: 1,
	})
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}
}
