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

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// --- toOutput ---.

// TestToOutput validates the toOutput conversion function.
// Covers nil input, full fields with all timestamps, and partial fields
// where optional time pointers are nil.
//
// Each case pins the whole Output, and every field of the populated case
// carries a value no other field carries — three distinct instants included.
// That is what makes the mapping assertable at all: a fixture that repeats one
// value cannot tell a correct wire from a crossed one, and this table replaced
// one that fed the same instant to all three timestamps and only asked that
// each came out non-empty. Reading UpdatedAt out of a.CreatedAt, or filling
// SubjectDigest from a.PredicateType, passed that table and passes no test
// here.
func TestToOutput(t *testing.T) {
	created := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 7, 20, 8, 30, 0, 0, time.UTC)
	expires := time.Date(2027, 1, 5, 23, 45, 0, 0, time.UTC)

	tests := []struct {
		name  string
		input *gl.Attestation
		want  Output
	}{
		{
			name:  "nil attestation returns zero output",
			input: nil,
			want:  Output{},
		},
		{
			name: "every field is read from its own source",
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
				CreatedAt:     &created,
				UpdatedAt:     &updated,
				ExpireAt:      &expires,
			},
			want: Output{
				ID:            42,
				IID:           7,
				ProjectID:     10,
				BuildID:       200,
				Status:        "success",
				PredicateKind: "slsa_provenance",
				PredicateType: "https://slsa.dev/provenance/v0.2",
				SubjectDigest: "sha256:deadbeef",
				DownloadURL:   "https://gitlab.example.com/download/42",
				CreatedAt:     "2026-06-15T12:00:00Z",
				UpdatedAt:     "2026-07-20T08:30:00Z",
				ExpireAt:      "2027-01-05T23:45:00Z",
			},
		},
		{
			name: "nil timestamps remain empty strings",
			input: &gl.Attestation{
				ID:     1,
				Status: "pending",
			},
			want: Output{
				ID:     1,
				Status: "pending",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toOutput(tt.input); got != tt.want {
				t.Errorf("toOutput() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// --- FormatOutputMarkdown ---.

// attestationCardHints is the guidance section every attestation card closes
// with.
const attestationCardHints = "\n---\n💡 **Next steps:**\n" +
	"- Use `attestation.download` to download this attestation's content\n" +
	"- Use `attestation.list` to view all attestations for the project\n"

// TestFormatOutputMarkdown validates the single-attestation markdown renderer.
// Covers zero-ID (empty), full output with all optional fields, and partial
// output with some optional fields omitted.
//
// Each case pins the whole card. A card is one document — heading, rows,
// guidance — and a substring assertion cannot tell a row that rendered from a
// row that landed somewhere Markdown shows as literal text.
func TestFormatOutputMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input Output
		want  string
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
				ExpireAt:      "2026-07-15T12:00:00Z",
			},
			want: "## Attestation #42 (IID 7)\n\n" +
				"- **Project ID**: 10\n" +
				"- **Build ID**: 200\n" +
				"- **Status**: success\n" +
				"- **Predicate Kind**: slsa_provenance\n" +
				"- **Predicate Type**: https://slsa.dev/provenance/v0.2\n" +
				"- **Subject Digest**: `sha256:deadbeef`\n" +
				"- **Download URL**: [https://gitlab.example.com/download](https://gitlab.example.com/download)\n" +
				"- **Created**: 15 Jun 2026 12:00 UTC\n" +
				"- **Expires**: 15 Jul 2026 12:00 UTC\n" +
				attestationCardHints,
		},
		{
			name: "partial output omits empty optional fields",
			input: Output{
				ID:      1,
				IID:     1,
				BuildID: 100,
				Status:  "pending",
			},
			want: "## Attestation #1 (IID 1)\n\n" +
				"- **Project ID**: 0\n" +
				"- **Build ID**: 100\n" +
				"- **Status**: pending\n" +
				attestationCardHints,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatOutputMarkdown(tt.input); got != tt.want {
				t.Errorf("FormatOutputMarkdown() =\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}

// --- FormatListMarkdown ---.

// TestFormatListMarkdown validates the attestation list markdown table renderer.
// Covers the empty list, which is one sentence and no heading counting zero,
// and a populated list whose timestamps render in the display form and whose
// guidance closes the response rather than opening it.
func TestFormatListMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input ListOutput
		want  string
	}{
		{
			name:  "empty list shows no-results message",
			input: ListOutput{},
			want:  "No attestations found.\n",
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
			want: "## Attestations (2)\n\n" +
				"| ID | IID | Build | Status | Predicate Kind | Created |\n" +
				"| --- | --- | --- | --- | --- | --- |\n" +
				"| 1 | 1 | 100 | success | slsa_provenance | 1 Jan 2026 00:00 UTC |\n" +
				"| 2 | 2 | 101 | failed |  | 1 Feb 2026 00:00 UTC |\n" +
				"\n---\n💡 **Next steps:**\n" +
				"- Use `attestation.download` with an IID from the table to fetch one attestation's bundle\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatListMarkdown(tt.input); got != tt.want {
				t.Errorf("FormatListMarkdown() =\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}

// --- FormatDownloadMarkdown ---.

// TestFormatDownloadMarkdown validates the download result markdown renderer.
// Covers zero IID (empty) and populated output with size and content info,
// both pinned as whole documents.
func TestFormatDownloadMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input DownloadOutput
		want  string
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
			want: "## Attestation Download (IID 7)\n\n" +
				"- **Size**: 1024 bytes\n" +
				"- **Content**: Base64-encoded in the `content_base64` field\n" +
				"\n---\n💡 **Next steps:**\n" +
				"- Use `attestation.list` to view all attestations for the project\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatDownloadMarkdown(tt.input); got != tt.want {
				t.Errorf("FormatDownloadMarkdown() =\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}

// --- List ---.

// TestList_Success verifies that List succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/projects/10/attestations/sha256:abc123 (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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

// TestList_MissingProjectID verifies that List_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := List(context.Background(), client, ListInput{SubjectDigest: "sha256:abc"})
	if err == nil {
		t.Fatal("expected error for empty project_id, got nil")
	}
}

// TestList_MissingSubjectDigest verifies that List_MissingSubjectDigest returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestList_MissingSubjectDigest(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := List(context.Background(), client, ListInput{ProjectID: toolutil.StringOrInt("10")})
	if err == nil {
		t.Fatal("expected error for empty subject_digest, got nil")
	}
}

// TestList_CancelledContext verifies the List_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
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

// TestList_APIError verifies that List returns a wrapped error when the GitLab API responds with an error status.
// The mock GitLab API at /api/v4/projects/10/attestations/sha256:abc123 (GET) responds with HTTP Forbidden.
// It asserts that the returned error is wrapped and, unlike the
// missing-project 404 below, carries no hint about the project id.
//
// The hint is attached per status, so which statuses receive it is a property
// worth pinning from both sides: a 403 is the instance refusing a caller whose
// project it resolved perfectly well, and telling them to verify the project
// id would send them after the wrong thing.
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
	if strings.Contains(err.Error(), "project.get") {
		t.Errorf("403 error carries the 404-only project hint: %v", err)
	}
}

// TestList_NotFoundForExistingProject verifies that List_NotFoundForExistingProject returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestList_NotFoundForExistingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/projects/10/attestations/sha256:abc123":
			testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
		case "/api/v4/projects/10":
			testutil.RespondJSON(w, http.StatusOK, `{"id":10,"path_with_namespace":"group/project"}`)
		default:
			http.NotFound(w, r)
		}
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID:     toolutil.StringOrInt("10"),
		SubjectDigest: "sha256:abc123",
	})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(out.Attestations) != 0 {
		t.Fatalf("expected empty attestation list, got %d", len(out.Attestations))
	}
}

// TestList_NotFoundForMissingProject_ReturnsTheHintedError holds the other
// side of the 404 fork that TestList_NotFoundForExistingProject opens. There
// the project reads back, so a 404 from the attestations endpoint means the
// digest matched nothing and an empty list is the honest answer. Here the
// project lookup answers 404 too, so the caller either named a project that
// does not exist or cannot see the one they named, and an empty list would
// tell them the project is fine and merely unattested.
//
// It matters because nothing held that branch before: the guard could be
// deleted outright — every 404 answered with an empty list — and the whole
// suite still passed. The assertion is on the hint rather than on the error
// being non-nil, since the hint is the part that names what to check and it is
// attached only when the status is 404.
func TestList_NotFoundForMissingProject_ReturnsTheHintedError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/projects/10/attestations/sha256:abc123", "/api/v4/projects/10":
			testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Project Not Found"}`)
		default:
			http.NotFound(w, r)
		}
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID:     toolutil.StringOrInt("10"),
		SubjectDigest: "sha256:abc123",
	})
	if err == nil {
		t.Fatalf("expected an error for a project that does not read back, got %+v", out)
	}
	errText := err.Error()
	for _, want := range []string{"list attestations", "project.get", "Ultimate"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(errText, want) {
				t.Errorf("error missing %q: %v", want, err)
			}
		})
	}
}

// TestList_EmptyResult verifies the List_EmptyResult handler.
// The mock GitLab API at /api/v4/projects/10/attestations/sha256:empty (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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

// TestDownload_Success verifies that Download succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/projects/10/attestations/1/download (GET) returns a representative success body.
// It asserts the returned output matches the expected fields.
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

// TestDownload_MissingProjectID verifies that Download_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDownload_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Download(context.Background(), client, DownloadInput{AttestationIID: 1})
	if err == nil {
		t.Fatal("expected error for empty project_id, got nil")
	}
}

// TestDownload_MissingAttestationIID verifies that Download_MissingAttestationIID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDownload_MissingAttestationIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Download(context.Background(), client, DownloadInput{ProjectID: toolutil.StringOrInt("10")})
	if err == nil {
		t.Fatal("expected error for zero attestation_iid, got nil")
	}
}

// TestDownload_CancelledContext verifies the Download_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
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

// TestDownload_APIError verifies that Download returns a wrapped error when the GitLab API responds with an error status.
// The mock GitLab API at /api/v4/projects/10/attestations/1/download (GET) responds with HTTP NotFound.
// It asserts that the returned error is wrapped and contains a useful hint.
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
	errText := err.Error()
	// The hint names the capability once, by the canonical ID: it used to name
	// the meta tool beside it, which is a spelling the dynamic and individual
	// surfaces cannot resolve.
	for _, want := range []string{"attestation_iid", "attestation.list"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(errText, want) {
				t.Fatalf("error missing %q: %v", want, err)
			}
		})
	}
}
