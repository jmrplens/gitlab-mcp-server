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
		{
			name: "a page of a longer list says so",
			input: ListOutput{
				Attestations: []Output{{ID: 1, IID: 1, BuildID: 100, Status: "success", CreatedAt: "2026-01-01T00:00:00Z"}},
				Pagination:   toolutil.PaginationOutput{Page: 1, PerPage: 1, TotalItems: 2, TotalPages: 2, NextPage: 2, HasMore: true},
			},
			want: "## Attestations (2)\n\n" +
				"Showing 1 of 2 results (page 1 of 2)\n\n" +
				"| ID | IID | Build | Status | Predicate Kind | Created |\n" +
				"| --- | --- | --- | --- | --- | --- |\n" +
				"| 1 | 1 | 100 | success |  | 1 Jan 2026 00:00 UTC |\n" +
				"\nPage 1 of 2 | 2 items total | 1 per page\n" +
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
// The mock answers only the digest's route in the lower case GitLab stores
// digests in, so the call reaches it only with the OCI sha256: prefix the
// caller wrote removed and the upper-case hex they wrote lowered.
// It asserts the returned output matches the expected fields.
func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == testDigestPath {
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
		SubjectDigest: "sha256:" + strings.ToUpper(testDigest),
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

// testDigest is a digest in the form GitLab documents for the route: the 64
// hex characters of the artifact's SHA-256 hash, the example of
// doc/api/attestations.md.
const testDigest = "5db1fee4b5703808c48078a76768b155b421b210c0761cd6a5d223f4d99f1eaa"

// testDigestPath is the route the list tests answer on. A request carrying
// the digest in any other form does not reach it, which is how a test here
// tells a digest sent as GitLab takes it from one sent as the caller wrote it.
const testDigestPath = "/api/v4/projects/10/attestations/" + testDigest

// TestList_PageAndPerPage_ReachTheRequest holds that the page a caller asks
// for is the page GitLab is asked for. GitLab pages the attestations of a
// digest although the route declares neither page nor per_page, and
// client-go's ListAttestations takes no options struct, so until the input
// carried the two this action could only ever read the first twenty.
func TestList_PageAndPerPage_ReachTheRequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, testDigestPath)
		testutil.AssertQueryParam(t, r, "page", "2")
		testutil.AssertQueryParam(t, r, "per_page", "1")
		testutil.RespondJSON(w, http.StatusOK, `[{"id":2,"iid":2,"project_id":10,"build_id":101,"status":"success"}]`)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID: toolutil.StringOrInt("10"), SubjectDigest: testDigest,
		Page: 2, PerPage: 1,
	})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(out.Attestations) != 1 || out.Attestations[0].IID != 2 {
		t.Errorf("attestations = %+v, want the one of page 2", out.Attestations)
	}
}

// TestList_NoPageAsked_SendsNeither holds that a caller who asks for no page
// leaves the choice to GitLab rather than sending a zero.
func TestList_NoPageAsked_SendsNeither(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("page") || r.URL.Query().Has("per_page") {
			t.Errorf("query = %q, want neither page nor per_page", r.URL.RawQuery)
		}
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	if _, err := List(context.Background(), client, ListInput{ProjectID: toolutil.StringOrInt("10"), SubjectDigest: testDigest}); err != nil {
		t.Fatalf("List() error: %v", err)
	}
}

// TestList_NextPageHeader_PublishesThePaginationBlock holds that the page
// GitLab answers is published as a page, so a caller holding the first page
// of attestations can tell more exist and which page to ask for.
func TestList_NextPageHeader_PublishesThePaginationBlock(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, testDigestPath)
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":1,"iid":1,"project_id":10,"build_id":100,"status":"success"}]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "1", Total: "2", TotalPages: "2", NextPage: "2"})
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: toolutil.StringOrInt("10"), SubjectDigest: testDigest})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	want := toolutil.PaginationOutput{Page: 1, PerPage: 1, TotalItems: 2, TotalPages: 2, NextPage: 2, HasMore: true}
	if out.Pagination != want {
		t.Errorf("pagination = %+v, want %+v", out.Pagination, want)
	}
}

// TestSubjectDigest_TheFormGitLabsRouteAccepts pins the one form the digest
// reaches GitLab in: 64 lower-case hex characters and no algorithm prefix.
// GitLab's route declares the hex pattern as a requirement and answers 404 to
// any other form, so a digest this cannot bring to that form is refused before
// a request is sent. Upper case passes the route and matches nothing behind
// it, since the lookup is an exact comparison against digests stored in lower
// case, so it is lowered rather than sent as written.
func TestSubjectDigest_TheFormGitLabsRouteAccepts(t *testing.T) {
	upper := strings.ToUpper(testDigest)
	mixed := strings.ToUpper(testDigest[:32]) + testDigest[32:]
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "the bare hex digest", raw: testDigest, want: testDigest},
		{name: "the OCI prefix removed", raw: "sha256:" + testDigest, want: testDigest},
		{name: "upper-case hex lowered", raw: upper, want: testDigest},
		{name: "mixed-case hex lowered", raw: mixed, want: testDigest},
		{name: "upper-case hex after the prefix lowered", raw: "sha256:" + upper, want: testDigest},
		{name: "surrounding space trimmed", raw: "  sha256:" + testDigest + "\n", want: testDigest},
		{name: "a digest one character short", raw: testDigest[1:]},
		{name: "a digest one character long", raw: testDigest + "0"},
		{name: "a non-hex character in place", raw: "g" + testDigest[1:]},
		{name: "the prefix alone", raw: "sha256:"},
		{name: "another algorithm's prefix", raw: "sha512:" + testDigest},
		{name: "the prefix in upper case", raw: "SHA256:" + testDigest},
		{name: "a Git commit SHA", raw: "0123456789abcdef0123456789abcdef01234567"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := subjectDigest(tc.raw)
			if tc.want == "" {
				if err == nil || !strings.Contains(err.Error(), "64 hex characters") {
					t.Fatalf("subjectDigest(%q) = %q, %v, want a refusal naming the form", tc.raw, got, err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Errorf("subjectDigest(%q) = %q, %v, want %q", tc.raw, got, err, tc.want)
			}
		})
	}
}

// TestList_ADigestGitLabWouldNotRoute_IsRefusedWithoutARequest verifies that
// a digest that is not a SHA-256 in any spelling is refused by name rather
// than sent, since GitLab's 404 for it would read as an artifact with no
// attestations.
func TestList_ADigestGitLabWouldNotRoute_IsRefusedWithoutARequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("request %s %s sent for a digest GitLab would not route", r.Method, r.URL.Path)
		http.NotFound(w, r)
	}))

	_, err := List(context.Background(), client, ListInput{ProjectID: toolutil.StringOrInt("10"), SubjectDigest: "sha256:abc123"})
	if err == nil || !strings.Contains(err.Error(), `subject_digest "sha256:abc123" is not a SHA-256 digest`) {
		t.Fatalf("List() error = %v, want the refusal naming the digest as written", err)
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
		SubjectDigest: "sha256:" + testDigest,
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestList_APIError verifies that List returns a wrapped error when the GitLab API responds with an error status.
// The mock answers the digest's route with HTTP Forbidden.
// It asserts that the returned error is wrapped and, unlike the
// missing-project 404 below, carries no hint about the project id.
//
// The hint is attached per status, so which statuses receive it is a property
// worth pinning from both sides: a 403 is the instance refusing a caller whose
// project it resolved perfectly well, and telling them to verify the project
// id would send them after the wrong thing.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testDigestPath {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := List(context.Background(), client, ListInput{
		ProjectID:     toolutil.StringOrInt("10"),
		SubjectDigest: "sha256:" + testDigest,
	})
	if err == nil {
		t.Fatal("expected error for 403 response, got nil")
	}
	if strings.Contains(err.Error(), "project.get") {
		t.Errorf("403 error carries the 404-only project hint: %v", err)
	}
}

// TestList_NotFoundForExistingProject_NamesTheFeatureFlag holds what a 404
// on a project that reads back is: the attestations API refused, not a digest
// with nothing under it.
//
// A well-formed digest nothing was attested under answers 200 and an empty
// array, and List only ever sends a well-formed one, so on a project the
// caller can read the one 404 left is the routes' before block, which answers
// it for every project the slsa_provenance_statement flag is off for. The flag
// ships disabled, so this is what every self-managed instance answers by
// default, and reading it as an empty list told a model that an attested
// artifact had no attestations. The assertion is on the refusal naming the
// flag and the action that turns it on, since those are what the caller can
// act on, and on no list being returned in its place.
func TestList_NotFoundForExistingProject_NamesTheFeatureFlag(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case testDigestPath:
			testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
		case "/api/v4/projects/10":
			testutil.RespondJSON(w, http.StatusOK, `{"id":10,"path_with_namespace":"group/project"}`)
		default:
			http.NotFound(w, r)
		}
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID:     toolutil.StringOrInt("10"),
		SubjectDigest: "sha256:" + testDigest,
	})
	if err == nil {
		t.Fatalf("List() answered %+v for a route GitLab refused, want the refusal naming the feature flag", out)
	}
	if out.Attestations != nil {
		t.Errorf("List() returned a list beside the refusal: %+v", out.Attestations)
	}
	errText := err.Error()
	for _, want := range []string{"list attestations", FeatureFlag, "admin.feature_set", "ships disabled"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(errText, want) {
				t.Errorf("error missing %q: %v", want, err)
			}
		})
	}
	if strings.Contains(errText, "project.get") {
		t.Errorf("the refusal sends the caller to verify a project that read back: %v", err)
	}
}

// TestFeatureFlag_NamesTheFlagGitLabServesTheRoutesBehind pins the flag's
// name to the one lib/api/supply_chain/attestations.rb checks, since every
// other assertion about it reads the constant and would agree with a
// misspelling.
func TestFeatureFlag_NamesTheFlagGitLabServesTheRoutesBehind(t *testing.T) {
	if FeatureFlag != "slsa_provenance_statement" {
		t.Errorf("FeatureFlag = %q, want %q (gitlab_com_derisk, default_enabled: false)", FeatureFlag, "slsa_provenance_statement")
	}
}

// TestList_NotFoundForMissingProject_ReturnsTheHintedError holds the other
// side of the 404 fork that TestList_NotFoundForExistingProject_NamesTheFeatureFlag
// opens. There the project reads back, so the 404 is the attestations API
// refused by its feature flag. Here the project lookup answers 404 too, so the
// caller either named a project that does not exist or cannot see the one
// they named, and the hint sends them to verify it.
//
// The assertion is on the hint rather than on the error being non-nil, since
// the hint is the part that names what to check, it is attached only when the
// status is 404, and it is the part that tells this branch from the other.
func TestList_NotFoundForMissingProject_ReturnsTheHintedError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case testDigestPath, "/api/v4/projects/10":
			testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Project Not Found"}`)
		default:
			http.NotFound(w, r)
		}
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID:     toolutil.StringOrInt("10"),
		SubjectDigest: "sha256:" + testDigest,
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
	if strings.Contains(errText, FeatureFlag) {
		t.Errorf("a project that does not read back is blamed on the feature flag: %v", err)
	}
}

// TestList_EmptyResult verifies the List_EmptyResult handler.
// The mock answers the digest's route with an empty array.
// It asserts the returned output matches the expected fields.
func TestList_EmptyResult(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testDigestPath {
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID:     toolutil.StringOrInt("10"),
		SubjectDigest: testDigest,
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
	// surfaces cannot resolve. It names the feature flag too, since the routes
	// answer 404 for every IID while it is off and that is the default.
	for _, want := range []string{"attestation_iid", "attestation.list", FeatureFlag} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(errText, want) {
				t.Fatalf("error missing %q: %v", want, err)
			}
		})
	}
}
