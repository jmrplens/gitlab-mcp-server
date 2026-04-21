// tags_test.go contains unit tests for GitLab tag operations (create, delete,
// get, list, signature, protected tag CRUD). Tests use httptest to mock the
// GitLab API and verify success, error, canceled-context, and markdown paths.
package tags

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// Test endpoint paths and fixture values used across tag operation tests.
const (
	errExpEmptyProjectID = "expected error for empty project_id, got nil"
	errExpEmptyTagName   = "expected error for empty tag_name, got nil"
	errExpAPIFailure     = "expected error for API failure"
	pathRepoTags         = "/api/v4/projects/42/repository/tags"
	testTagName          = "v1.2.0"
	testTagMessage       = "Release v1.2.0"
	testTagV100          = "v1.0.0"
	testReleaseName      = "Release v1.0.0"
	testEmailAddr        = "test@example.com"
	testCRLURL           = "https://example.com/crl"

	errCancelledCtx    = "expected error for canceled context"
	descMaintainers    = "Maintainers"
	fmtUnexpectedError = "unexpected error: %v"
	argProjectID       = "project_id"
	argTagName         = "tag_name"
	fmtNameWant        = "out.Name = %q, want %q"
	fmtListUnexpErr    = "List() unexpected error: %v"
)

// TestTagCreate_Success verifies that Create creates an annotated tag and
// returns the correct name, target commit, and message. The mock returns
// HTTP 201 with a valid tag JSON response.
func TestTagCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathRepoTags {
			testutil.RespondJSON(w, http.StatusCreated, `{"name":"v1.2.0","target":"abc123def456","message":"Release v1.2.0","protected":false}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID: "42",
		TagName:   testTagName,
		Ref:       "main",
		Message:   testTagMessage,
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.Name != testTagName {
		t.Errorf(fmtNameWant, out.Name, testTagName)
	}
	if out.Target != "abc123def456" {
		t.Errorf("out.Target = %q, want %q", out.Target, "abc123def456")
	}
	if out.Message != testTagMessage {
		t.Errorf("out.Message = %q, want %q", out.Message, testTagMessage)
	}
}

// TestTagCreate_InvalidRef verifies that Create returns an error when the
// ref (branch or commit) does not exist. The mock returns HTTP 404.
func TestTagCreate_InvalidRef(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Ref Not Found"}`)
	}))

	_, err := Create(context.Background(), client, CreateInput{
		ProjectID: "42",
		TagName:   testTagV100,
		Ref:       "nonexistent-branch",
	})
	if err == nil {
		t.Fatal("Create() expected error for invalid ref, got nil")
	}
}

// TestTagDelete_Success verifies that Delete removes a tag without error.
// The mock returns HTTP 204 No Content for the correct DELETE path.
func TestTagDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v4/projects/42/repository/tags/v1.2.0" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	if err := Delete(context.Background(), client, DeleteInput{
		ProjectID: "42",
		TagName:   testTagName,
	}); err != nil {
		t.Errorf("Delete() unexpected error: %v", err)
	}
}

// TestTagDelete_NotFound verifies that Delete returns an error when the
// target tag does not exist. The mock returns HTTP 404.
func TestTagDelete_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Tag Not Found"}`)
	}))

	if err := Delete(context.Background(), client, DeleteInput{
		ProjectID: "42",
		TagName:   "nonexistent",
	}); err == nil {
		t.Fatal("Delete() expected error for non-existent tag, got nil")
	}
}

// TestTagList_Success verifies that List returns multiple tags with their
// attributes correctly mapped from the GitLab API response.
func TestTagList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathRepoTags {
			testutil.RespondJSON(w, http.StatusOK, `[{"name":"v1.2.0","target":"abc123","message":null,"protected":false},{"name":"v1.1.0","target":"def456","message":null,"protected":false}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtListUnexpErr, err)
	}
	if len(out.Tags) != 2 {
		t.Errorf("len(out.Tags) = %d, want 2", len(out.Tags))
	}
	if out.Tags[0].Name != testTagName {
		t.Errorf("out.Tags[0].Name = %q, want %q", out.Tags[0].Name, testTagName)
	}
}

// TestTagList_Empty verifies that List handles an empty API response
// gracefully, returning zero tags without error.
func TestTagList_Empty(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("List() unexpected error for empty list: %v", err)
	}
	if len(out.Tags) != 0 {
		t.Errorf("len(out.Tags) = %d, want 0", len(out.Tags))
	}
}

// TestTagList_PaginationQueryParamsAndMetadata verifies that List forwards
// page and per_page query parameters and correctly parses pagination metadata
// (TotalItems, NextPage, PrevPage) from the GitLab response headers.
func TestTagList_PaginationQueryParamsAndMetadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathRepoTags {
			if got := r.URL.Query().Get("page"); got != "1" {
				t.Errorf("query param page = %q, want %q", got, "1")
			}
			if got := r.URL.Query().Get("per_page"); got != "3" {
				t.Errorf("query param per_page = %q, want %q", got, "3")
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"name":"v1.0.0","target":"aaa","message":"","protected":false}]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "3", Total: "7", TotalPages: "3", NextPage: "2"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: "42", PaginationInput: toolutil.PaginationInput{Page: 1, PerPage: 3}})
	if err != nil {
		t.Fatalf(fmtListUnexpErr, err)
	}
	if out.Pagination.TotalItems != 7 {
		t.Errorf("Pagination.TotalItems = %d, want 7", out.Pagination.TotalItems)
	}
	if out.Pagination.NextPage != 2 {
		t.Errorf("Pagination.NextPage = %d, want 2", out.Pagination.NextPage)
	}
	if out.Pagination.PrevPage != 0 {
		t.Errorf("Pagination.PrevPage = %d, want 0", out.Pagination.PrevPage)
	}
}

// TestTagGet_Success verifies that Get retrieves a single tag by name.
func TestTagGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathRepoTags+"/v1.0.0" {
			testutil.RespondJSON(w, http.StatusOK, `{
				"name":"v1.0.0",
				"message":"Release v1.0.0",
				"target":"abc123",
				"commit":{
					"id":"abc123","short_id":"abc123d","title":"Release v1.0.0",
					"author_name":"Test","committed_date":"2026-03-01T10:00:00Z",
					"web_url":"https://gitlab.example.com/-/commit/abc123"
				},
				"protected":false
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{
		ProjectID: "42",
		TagName:   testTagV100,
	})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.Name != testTagV100 {
		t.Errorf(fmtNameWant, out.Name, testTagV100)
	}
	if out.Message != testReleaseName {
		t.Errorf("out.Message = %q, want %q", out.Message, testReleaseName)
	}
}

// TestTagGetSuccess_EnrichedFields verifies that Get maps enriched fields:
// CommitSHA, CommitMessage, CreatedAt from the commit sub-object.
func TestTagGet_SuccessEnrichedFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathRepoTags+"/v2.0.0" {
			testutil.RespondJSON(w, http.StatusOK, `{
				"name":"v2.0.0",
				"message":"Annotated tag",
				"target":"aaa111",
				"commit":{"id":"aaa111bbb222","message":"feat: new feature"},
				"protected":true,
				"created_at":"2026-06-15T08:30:00Z"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{
		ProjectID: "42",
		TagName:   "v2.0.0",
	})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.CommitSHA != "aaa111bbb222" {
		t.Errorf("out.CommitSHA = %q, want %q", out.CommitSHA, "aaa111bbb222")
	}
	if out.CommitMessage != "feat: new feature" {
		t.Errorf("out.CommitMessage = %q, want %q", out.CommitMessage, "feat: new feature")
	}
	if out.CreatedAt == "" {
		t.Error("out.CreatedAt is empty, want timestamp")
	}
}

// TestTagGet_EmptyProjectID verifies Get returns an error for empty project_id.
func TestTagGet_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := Get(context.Background(), client, GetInput{TagName: testTagV100})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// TestTagGet_APIError verifies Get returns an error on API failure.
func TestTagGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Tag Not Found"}`)
	}))

	_, err := Get(context.Background(), client, GetInput{
		ProjectID: "42",
		TagName:   "nonexistent",
	})
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
}

// ---------------------------------------------------------------------------
// GetSignature tests
// ---------------------------------------------------------------------------.

const pathRepoTagSig = "/api/v4/projects/42/repository/tags/v1.0.0/signature"

// TestTagGetSignature_Success verifies the behavior of tag get signature success.
func TestTagGetSignature_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathRepoTagSig {
			testutil.RespondJSON(w, http.StatusOK, `{
				"signature_type": "X509",
				"verification_status": "verified",
				"x509_certificate": {
					"id": 1,
					"subject": "CN=Test",
					"subject_key_identifier": "abc123",
					"email": "test@example.com",
					"serial_number": 12345,
					"certificate_status": "good",
					"x509_issuer": {
						"id": 2,
						"subject": "CN=Issuer",
						"subject_key_identifier": "def456",
						"crl_url": "https://example.com/crl"
					}
				}
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetSignature(context.Background(), client, SignatureInput{
		ProjectID: "42",
		TagName:   testTagV100,
	})
	if err != nil {
		t.Fatalf("GetSignature() unexpected error: %v", err)
	}
	if out.SignatureType != "X509" {
		t.Errorf("out.SignatureType = %q, want %q", out.SignatureType, "X509")
	}
	if out.VerificationStatus != "verified" {
		t.Errorf("out.VerificationStatus = %q, want %q", out.VerificationStatus, "verified")
	}
	if out.X509Certificate.Email != testEmailAddr {
		t.Errorf("out.X509Certificate.Email = %q, want %q", out.X509Certificate.Email, testEmailAddr)
	}
	if out.X509Certificate.X509Issuer.CrlURL != testCRLURL {
		t.Errorf("out.X509Certificate.X509Issuer.CrlURL = %q, want %q", out.X509Certificate.X509Issuer.CrlURL, testCRLURL)
	}
}

// TestTagGetSignature_EmptyProjectID verifies the behavior of tag get signature empty project i d.
func TestTagGetSignature_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	_, err := GetSignature(context.Background(), client, SignatureInput{TagName: testTagV100})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// TestTagGetSignature_EmptyTagName verifies the behavior of tag get signature empty tag name.
func TestTagGetSignature_EmptyTagName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	_, err := GetSignature(context.Background(), client, SignatureInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpEmptyTagName)
	}
}

// ---------------------------------------------------------------------------
// Protected Tags tests
// ---------------------------------------------------------------------------.

const pathProtectedTags = "/api/v4/projects/42/protected_tags"

// TestTagListProtected_Success verifies the behavior of tag list protected success.
func TestTagListProtected_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProtectedTags {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"name":"v*","create_access_levels":[{"id":1,"access_level":40,"access_level_description":"Maintainers"}]},
				{"name":"release-*","create_access_levels":[]}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListProtectedTags(context.Background(), client, ListProtectedTagsInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("ListProtectedTags() unexpected error: %v", err)
	}
	if len(out.Tags) != 2 {
		t.Fatalf("len(out.Tags) = %d, want 2", len(out.Tags))
	}
	if out.Tags[0].Name != "v*" {
		t.Errorf("out.Tags[0].Name = %q, want %q", out.Tags[0].Name, "v*")
	}
	if len(out.Tags[0].CreateAccessLevels) != 1 {
		t.Fatalf("len(CreateAccessLevels) = %d, want 1", len(out.Tags[0].CreateAccessLevels))
	}
	if out.Tags[0].CreateAccessLevels[0].AccessLevel != 40 {
		t.Errorf("AccessLevel = %d, want 40", out.Tags[0].CreateAccessLevels[0].AccessLevel)
	}
}

// TestTagListProtected_EmptyProjectID verifies the behavior of tag list protected empty project i d.
func TestTagListProtected_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := ListProtectedTags(context.Background(), client, ListProtectedTagsInput{})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// TestTagGetProtected_Success verifies the behavior of tag get protected success.
func TestTagGetProtected_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProtectedTags+"/v*" {
			testutil.RespondJSON(w, http.StatusOK, `{"name":"v*","create_access_levels":[{"id":1,"access_level":40,"access_level_description":"Maintainers"}]}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetProtectedTag(context.Background(), client, GetProtectedTagInput{ProjectID: "42", TagName: "v*"})
	if err != nil {
		t.Fatalf("GetProtectedTag() unexpected error: %v", err)
	}
	if out.Name != "v*" {
		t.Errorf(fmtNameWant, out.Name, "v*")
	}
}

// TestTagGetProtected_EmptyProjectID verifies the behavior of tag get protected empty project i d.
func TestTagGetProtected_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	_, err := GetProtectedTag(context.Background(), client, GetProtectedTagInput{TagName: "v*"})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// TestTagGetProtected_EmptyTagName verifies the behavior of tag get protected empty tag name.
func TestTagGetProtected_EmptyTagName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	_, err := GetProtectedTag(context.Background(), client, GetProtectedTagInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpEmptyTagName)
	}
}

// TestTagProtect_Success verifies the behavior of tag protect success.
func TestTagProtect_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProtectedTags {
			testutil.RespondJSON(w, http.StatusCreated, `{"name":"v*","create_access_levels":[{"id":1,"access_level":30,"access_level_description":"Developers + Maintainers"}]}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ProtectTag(context.Background(), client, ProtectTagInput{
		ProjectID:         "42",
		TagName:           "v*",
		CreateAccessLevel: 30,
	})
	if err != nil {
		t.Fatalf("ProtectTag() unexpected error: %v", err)
	}
	if out.Name != "v*" {
		t.Errorf(fmtNameWant, out.Name, "v*")
	}
	if len(out.CreateAccessLevels) != 1 {
		t.Fatalf("len(CreateAccessLevels) = %d, want 1", len(out.CreateAccessLevels))
	}
}

// TestTagProtect_EmptyProjectID verifies the behavior of tag protect empty project i d.
func TestTagProtect_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	_, err := ProtectTag(context.Background(), client, ProtectTagInput{TagName: "v*"})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// TestTagProtect_EmptyName verifies the behavior of tag protect empty name.
func TestTagProtect_EmptyName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	_, err := ProtectTag(context.Background(), client, ProtectTagInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpEmptyTagName)
	}
}

// TestTagUnprotect_Success verifies the behavior of tag unprotect success.
func TestTagUnprotect_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathProtectedTags+"/v*" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := UnprotectTag(context.Background(), client, UnprotectTagInput{ProjectID: "42", TagName: "v*"})
	if err != nil {
		t.Fatalf("UnprotectTag() unexpected error: %v", err)
	}
}

// TestTagUnprotect_EmptyProjectID verifies the behavior of tag unprotect empty project i d.
func TestTagUnprotect_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	err := UnprotectTag(context.Background(), client, UnprotectTagInput{TagName: "v*"})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// TestTagUnprotect_EmptyTagName verifies the behavior of tag unprotect empty tag name.
func TestTagUnprotect_EmptyTagName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	err := UnprotectTag(context.Background(), client, UnprotectTagInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpEmptyTagName)
	}
}

// ---------------------------------------------------------------------------
// Canceled context tests for ALL functions
// ---------------------------------------------------------------------------.

// TestTagCreate_CancelledContext verifies the behavior of tag create cancelled context.
func TestTagCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Create(ctx, client, CreateInput{ProjectID: "42", TagName: "v0", Ref: "main"})
	if err == nil {
		t.Fatal(errCancelledCtx)
	}
}

// TestTagDelete_CancelledContext verifies the behavior of tag delete cancelled context.
func TestTagDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx := testutil.CancelledCtx(t)
	err := Delete(ctx, client, DeleteInput{ProjectID: "42", TagName: "v0"})
	if err == nil {
		t.Fatal(errCancelledCtx)
	}
}

// TestTagList_CancelledContext verifies the behavior of tag list cancelled context.
func TestTagList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errCancelledCtx)
	}
}

// TestTagGet_CancelledContext verifies the behavior of tag get cancelled context.
func TestTagGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Get(ctx, client, GetInput{ProjectID: "42", TagName: "v0"})
	if err == nil {
		t.Fatal(errCancelledCtx)
	}
}

// TestTagGetSignature_CancelledContext verifies the behavior of tag get signature cancelled context.
func TestTagGetSignature_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetSignature(ctx, client, SignatureInput{ProjectID: "42", TagName: "v0"})
	if err == nil {
		t.Fatal(errCancelledCtx)
	}
}

// TestTagListProtected_CancelledContext verifies the behavior of tag list protected cancelled context.
func TestTagListProtected_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListProtectedTags(ctx, client, ListProtectedTagsInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errCancelledCtx)
	}
}

// TestTagGetProtected_CancelledContext verifies the behavior of tag get protected cancelled context.
func TestTagGetProtected_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetProtectedTag(ctx, client, GetProtectedTagInput{ProjectID: "42", TagName: "v*"})
	if err == nil {
		t.Fatal(errCancelledCtx)
	}
}

// TestTagProtect_CancelledContext verifies the behavior of tag protect cancelled context.
func TestTagProtect_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := ProtectTag(ctx, client, ProtectTagInput{ProjectID: "42", TagName: "v*"})
	if err == nil {
		t.Fatal(errCancelledCtx)
	}
}

// TestTagUnprotect_CancelledContext verifies the behavior of tag unprotect cancelled context.
func TestTagUnprotect_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx := testutil.CancelledCtx(t)
	err := UnprotectTag(ctx, client, UnprotectTagInput{ProjectID: "42", TagName: "v*"})
	if err == nil {
		t.Fatal(errCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Empty ProjectID for Create and Delete
// ---------------------------------------------------------------------------.

// TestTagCreate_EmptyProjectID verifies the behavior of tag create empty project i d.
func TestTagCreate_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{TagName: "v0", Ref: "main"})
	if err == nil {
		t.Fatal("expected error for empty project_id")
	}
}

// TestTagDelete_EmptyProjectID verifies the behavior of tag delete empty project i d.
func TestTagDelete_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	err := Delete(context.Background(), client, DeleteInput{TagName: "v0"})
	if err == nil {
		t.Fatal("expected error for empty project_id")
	}
}

// ---------------------------------------------------------------------------
// API error tests
// ---------------------------------------------------------------------------.

// TestTagList_APIError verifies the behavior of tag list a p i error.
func TestTagList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"500"}`)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestTagGetSignature_APIError verifies the behavior of tag get signature a p i error.
func TestTagGetSignature_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404"}`)
	}))
	_, err := GetSignature(context.Background(), client, SignatureInput{ProjectID: "42", TagName: "v0"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestTagListProtected_APIError verifies the behavior of tag list protected a p i error.
func TestTagListProtected_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := ListProtectedTags(context.Background(), client, ListProtectedTagsInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestTagGetProtected_APIError verifies the behavior of tag get protected a p i error.
func TestTagGetProtected_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404"}`)
	}))
	_, err := GetProtectedTag(context.Background(), client, GetProtectedTagInput{ProjectID: "42", TagName: "v*"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestTagProtect_APIError verifies the behavior of tag protect a p i error.
func TestTagProtect_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := ProtectTag(context.Background(), client, ProtectTagInput{ProjectID: "42", TagName: "v*"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestTagUnprotect_APIError verifies the behavior of tag unprotect a p i error.
func TestTagUnprotect_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	err := UnprotectTag(context.Background(), client, UnprotectTagInput{ProjectID: "42", TagName: "v*"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// ---------------------------------------------------------------------------
// ProtectTag with AllowedToCreate granular permissions
// ---------------------------------------------------------------------------.

// TestTagProtect_WithAllowedToCreate verifies the behavior of tag protect with allowed to create.
func TestTagProtect_WithAllowedToCreate(t *testing.T) {
	var capturedBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProtectedTags {
			body, _ := io.ReadAll(r.Body)
			capturedBody = string(body)
			testutil.RespondJSON(w, http.StatusCreated, `{"name":"v*","create_access_levels":[{"id":1,"access_level":30,"access_level_description":"Developers + Maintainers","user_id":5}]}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ProtectTag(context.Background(), client, ProtectTagInput{
		ProjectID: "42",
		TagName:   "v*",
		AllowedToCreate: []TagPermission{
			{UserID: 5, AccessLevel: 30},
			{GroupID: 10},
			{DeployKeyID: 3},
		},
	})
	if err != nil {
		t.Fatalf("ProtectTag() unexpected error: %v", err)
	}
	if out.Name != "v*" {
		t.Errorf(fmtNameWant, out.Name, "v*")
	}
	for _, want := range []string{"allowed_to_create", "user_id", "group_id", "deploy_key_id"} {
		if !strings.Contains(capturedBody, want) {
			t.Errorf("request body missing field %q", want)
		}
	}
}

// ---------------------------------------------------------------------------
// ListProtectedTags with pagination
// ---------------------------------------------------------------------------.

// TestTagListProtected_Pagination verifies the behavior of tag list protected pagination.
func TestTagListProtected_Pagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProtectedTags {
			if got := r.URL.Query().Get("page"); got != "2" {
				t.Errorf("query param page = %q, want %q", got, "2")
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"name":"v*","create_access_levels":[]}]`,
				testutil.PaginationHeaders{Page: "2", PerPage: "5", Total: "6", TotalPages: "2"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListProtectedTags(context.Background(), client, ListProtectedTagsInput{
		ProjectID:       "42",
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 5},
	})
	if err != nil {
		t.Fatalf("ListProtectedTags() unexpected error: %v", err)
	}
	if out.Pagination.Page != 2 {
		t.Errorf("Pagination.Page = %d, want 2", out.Pagination.Page)
	}
	if out.Pagination.TotalItems != 6 {
		t.Errorf("Pagination.TotalItems = %d, want 6", out.Pagination.TotalItems)
	}
}

// ---------------------------------------------------------------------------
// Converter edge cases
// ---------------------------------------------------------------------------.

// TestToOutput_NilCommitAndZeroTime verifies the behavior of to output nil commit and zero time.
func TestToOutput_NilCommitAndZeroTime(t *testing.T) {
	out := toOutput(&gl.Tag{Name: "v0.0.1", Target: "abc"})
	if out.CommitSHA != "" {
		t.Errorf("out.CommitSHA = %q, want empty for nil commit", out.CommitSHA)
	}
	if out.CreatedAt != "" {
		t.Errorf("out.CreatedAt = %q, want empty for zero time", out.CreatedAt)
	}
}

// TestProtectedTagOutput_FromGLEmptyLevels verifies the behavior of protected tag output from g l empty levels.
func TestProtectedTagOutput_FromGLEmptyLevels(t *testing.T) {
	out := protectedTagOutputFromGL(&gl.ProtectedTag{Name: "v*"})
	if len(out.CreateAccessLevels) != 0 {
		t.Errorf("len(CreateAccessLevels) = %d, want 0", len(out.CreateAccessLevels))
	}
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdownString verifies the behavior of format output markdown string.
func TestFormatOutputMarkdownString(t *testing.T) {
	md := FormatOutputMarkdownString(Output{
		Name:          testTagV100,
		Target:        "abc123",
		Protected:     true,
		Message:       testReleaseName,
		CommitSHA:     "abc123def456",
		CommitMessage: "feat: init",
		CreatedAt:     "2026-01-01T00:00:00Z",
	})
	if !strings.Contains(md, "## Tag: v1.0.0") {
		t.Error("expected heading with tag name")
	}
	if !strings.Contains(md, "abc123def456") {
		t.Error("expected commit SHA")
	}
	if !strings.Contains(md, testReleaseName) {
		t.Error("expected message")
	}
	if !strings.Contains(md, "1 Jan 2026") {
		t.Error("expected created at")
	}
}

// TestFormatOutputMarkdownString_Minimal verifies the behavior of format output markdown string minimal.
func TestFormatOutputMarkdownString_Minimal(t *testing.T) {
	md := FormatOutputMarkdownString(Output{Name: "v0", Target: "x"})
	if !strings.Contains(md, "## Tag: v0") {
		t.Error("expected heading")
	}
	if strings.Contains(md, "Message") {
		t.Error("should not contain Message when empty")
	}
}

// TestFormatListMarkdownString verifies the behavior of format list markdown string.
func TestFormatListMarkdownString(t *testing.T) {
	md := FormatListMarkdownString(ListOutput{
		Tags: []Output{
			{Name: testTagV100, Target: "abc", Protected: true},
			{Name: "v0.9.0", Target: "def", Protected: false},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2},
	})
	if !strings.Contains(md, "## Tags (2)") {
		t.Error("expected heading with count")
	}
	if !strings.Contains(md, "| v1.0.0 |") {
		t.Error("expected v1.0.0 row")
	}
}

// TestFormatListMarkdownString_Empty verifies the behavior of format list markdown string empty.
func TestFormatListMarkdownString_Empty(t *testing.T) {
	md := FormatListMarkdownString(ListOutput{})
	if !strings.Contains(md, "No tags found") {
		t.Error("expected 'No tags found' message")
	}
}

// TestFormatSignatureMarkdownString verifies the behavior of format signature markdown string.
func TestFormatSignatureMarkdownString(t *testing.T) {
	md := FormatSignatureMarkdownString(SignatureOutput{
		SignatureType:      "X509",
		VerificationStatus: "verified",
		X509Certificate: X509CertificateOutput{
			Subject:           "CN=Test",
			Email:             testEmailAddr,
			CertificateStatus: "good",
			SerialNumber:      "12345",
			X509Issuer: X509IssuerOutput{
				Subject: "CN=Issuer",
				CrlURL:  testCRLURL,
			},
		},
	})
	if !strings.Contains(md, "## Tag Signature") {
		t.Error("expected signature heading")
	}
	if !strings.Contains(md, "X509") {
		t.Error("expected signature type")
	}
	if !strings.Contains(md, testEmailAddr) {
		t.Error("expected email")
	}
	if !strings.Contains(md, "12345") {
		t.Error("expected serial number")
	}
	if !strings.Contains(md, testCRLURL) {
		t.Error("expected CRL URL")
	}
}

// TestFormatSignatureMarkdownString_Minimal verifies the behavior of format signature markdown string minimal.
func TestFormatSignatureMarkdownString_Minimal(t *testing.T) {
	md := FormatSignatureMarkdownString(SignatureOutput{
		SignatureType:      "X509",
		VerificationStatus: "unverified",
	})
	if !strings.Contains(md, "unverified") {
		t.Error("expected verification status")
	}
	if strings.Contains(md, "Serial Number") {
		t.Error("should not contain serial number when empty")
	}
}

// TestFormatProtectedTagMarkdownString verifies the behavior of format protected tag markdown string.
func TestFormatProtectedTagMarkdownString(t *testing.T) {
	md := FormatProtectedTagMarkdownString(ProtectedTagOutput{
		Name: "v*",
		CreateAccessLevels: []TagAccessLevelOutput{
			{ID: 1, AccessLevel: 40, AccessLevelDescription: descMaintainers},
		},
	})
	if !strings.Contains(md, "## Protected Tag: v*") {
		t.Error("expected heading")
	}
	if !strings.Contains(md, descMaintainers) {
		t.Error("expected access level description")
	}
	if !strings.Contains(md, "Deploy Key ID") {
		t.Error("expected Deploy Key ID column header")
	}
	if !strings.Contains(md, "| - | - | - |") {
		t.Error("expected '-' for zero UserID, GroupID, and DeployKeyID")
	}
}

// TestFormatProtectedTagMarkdownString_Empty verifies the behavior of format protected tag markdown string empty.
func TestFormatProtectedTagMarkdownString_Empty(t *testing.T) {
	md := FormatProtectedTagMarkdownString(ProtectedTagOutput{Name: "release-*"})
	if !strings.Contains(md, "No create access levels") {
		t.Error("expected no access levels message")
	}
}

// TestFormatListProtectedTagsMarkdownString verifies the behavior of format list protected tags markdown string.
func TestFormatListProtectedTagsMarkdownString(t *testing.T) {
	md := FormatListProtectedTagsMarkdownString(ListProtectedTagsOutput{
		Tags: []ProtectedTagOutput{
			{Name: "v*", CreateAccessLevels: []TagAccessLevelOutput{{AccessLevelDescription: descMaintainers}}},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	})
	if !strings.Contains(md, "## Protected Tags (1)") {
		t.Error("expected heading with count")
	}
	if !strings.Contains(md, descMaintainers) {
		t.Error("expected access level description")
	}
}

// TestFormatProtectedTagMarkdownString_WithUserID verifies the behavior of format protected tag markdown string with user i d.
func TestFormatProtectedTagMarkdownString_WithUserID(t *testing.T) {
	md := FormatProtectedTagMarkdownString(ProtectedTagOutput{
		Name: "v*",
		CreateAccessLevels: []TagAccessLevelOutput{
			{ID: 1, AccessLevel: 40, AccessLevelDescription: descMaintainers, UserID: 5},
		},
	})
	if !strings.Contains(md, "| 5 |") {
		t.Error("expected User ID 5 in table")
	}
	if !strings.Contains(md, "| - | - |") {
		t.Error("expected '-' for zero GroupID and DeployKeyID")
	}
}

// TestFormatProtectedTagMarkdownString_WithGroupID verifies the behavior of format protected tag markdown string with group i d.
func TestFormatProtectedTagMarkdownString_WithGroupID(t *testing.T) {
	md := FormatProtectedTagMarkdownString(ProtectedTagOutput{
		Name: "release-*",
		CreateAccessLevels: []TagAccessLevelOutput{
			{ID: 2, AccessLevel: 30, AccessLevelDescription: "Developers", GroupID: 10},
		},
	})
	if !strings.Contains(md, "| 10 |") {
		t.Error("expected Group ID 10 in table")
	}
}

// TestFormatProtectedTagMarkdownString_WithDeployKeyID verifies the behavior of format protected tag markdown string with deploy key i d.
func TestFormatProtectedTagMarkdownString_WithDeployKeyID(t *testing.T) {
	md := FormatProtectedTagMarkdownString(ProtectedTagOutput{
		Name: "deploy-*",
		CreateAccessLevels: []TagAccessLevelOutput{
			{ID: 3, AccessLevel: 40, AccessLevelDescription: "Deploy Key", DeployKeyID: 7},
		},
	})
	if !strings.Contains(md, "| 7 |") {
		t.Error("expected Deploy Key ID 7 in table")
	}
}

// TestFormatListProtectedTags_WithUserContext verifies the behavior of format list protected tags with user context.
func TestFormatListProtectedTags_WithUserContext(t *testing.T) {
	md := FormatListProtectedTagsMarkdownString(ListProtectedTagsOutput{
		Tags: []ProtectedTagOutput{
			{Name: "v*", CreateAccessLevels: []TagAccessLevelOutput{
				{AccessLevelDescription: "Developer", UserID: 5},
			}},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	})
	if !strings.Contains(md, "Developer (User #5)") {
		t.Error("expected 'Developer (User #5)' context in list view")
	}
}

// TestFormatListProtectedTags_WithGroupContext verifies the behavior of format list protected tags with group context.
func TestFormatListProtectedTags_WithGroupContext(t *testing.T) {
	md := FormatListProtectedTagsMarkdownString(ListProtectedTagsOutput{
		Tags: []ProtectedTagOutput{
			{Name: "v*", CreateAccessLevels: []TagAccessLevelOutput{
				{AccessLevelDescription: descMaintainers, GroupID: 10},
			}},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	})
	if !strings.Contains(md, "Maintainers (Group #10)") {
		t.Error("expected 'Maintainers (Group #10)' context in list view")
	}
}

// TestFormatListProtectedTags_WithDeployKeyContext verifies the behavior of format list protected tags with deploy key context.
func TestFormatListProtectedTags_WithDeployKeyContext(t *testing.T) {
	md := FormatListProtectedTagsMarkdownString(ListProtectedTagsOutput{
		Tags: []ProtectedTagOutput{
			{Name: "v*", CreateAccessLevels: []TagAccessLevelOutput{
				{AccessLevelDescription: "Deploy Key", DeployKeyID: 7},
			}},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	})
	if !strings.Contains(md, "Deploy Key (Deploy Key #7)") {
		t.Error("expected 'Deploy Key (Deploy Key #7)' context in list view")
	}
}

// TestFormatIDCell verifies the behavior of format i d cell.
func TestFormatIDCell(t *testing.T) {
	if got := formatIDCell(0); got != "-" {
		t.Errorf("formatIDCell(0) = %q, want %q", got, "-")
	}
	if got := formatIDCell(42); got != "42" {
		t.Errorf("formatIDCell(42) = %q, want %q", got, "42")
	}
}

// TestFormatAccessLevelSummary validates format access level summary across multiple scenarios using table-driven subtests.
func TestFormatAccessLevelSummary(t *testing.T) {
	tests := []struct {
		name  string
		input TagAccessLevelOutput
		want  string
	}{
		{"plain", TagAccessLevelOutput{AccessLevelDescription: descMaintainers}, "Maintainers"},
		{"user", TagAccessLevelOutput{AccessLevelDescription: "Dev", UserID: 5}, "Dev (User #5)"},
		{"group", TagAccessLevelOutput{AccessLevelDescription: "Dev", GroupID: 10}, "Dev (Group #10)"},
		{"deploy_key", TagAccessLevelOutput{AccessLevelDescription: "Key", DeployKeyID: 3}, "Key (Deploy Key #3)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatAccessLevelSummary(tt.input); got != tt.want {
				t.Errorf("formatAccessLevelSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestFormatListProtectedTagsMarkdownString_Empty verifies the behavior of format list protected tags markdown string empty.
func TestFormatListProtectedTagsMarkdownString_Empty(t *testing.T) {
	md := FormatListProtectedTagsMarkdownString(ListProtectedTagsOutput{})
	if !strings.Contains(md, "No protected tags found") {
		t.Error("expected 'No protected tags found' message")
	}
}

// ---------------------------------------------------------------------------
// List with search/order/sort params
// ---------------------------------------------------------------------------.

// TestTagList_WithSearchOrderSort verifies the behavior of tag list with search order sort.
func TestTagList_WithSearchOrderSort(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathRepoTags {
			if got := r.URL.Query().Get("search"); got != "v1" {
				t.Errorf("search = %q, want %q", got, "v1")
			}
			if got := r.URL.Query().Get("order_by"); got != "version" {
				t.Errorf("order_by = %q, want %q", got, "version")
			}
			if got := r.URL.Query().Get("sort"); got != "desc" {
				t.Errorf("sort = %q, want %q", got, "desc")
			}
			testutil.RespondJSON(w, http.StatusOK, `[{"name":"v1.2.0","target":"abc","protected":false}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID: "42",
		Search:    "v1",
		OrderBy:   "version",
		Sort:      "desc",
	})
	if err != nil {
		t.Fatalf(fmtListUnexpErr, err)
	}
	if len(out.Tags) != 1 {
		t.Fatalf("len(out.Tags) = %d, want 1", len(out.Tags))
	}
}

// ---------------------------------------------------------------------------
// RegisterTools + CallAllThroughMCP
// ---------------------------------------------------------------------------.

// tagRouteHandler returns an http.HandlerFunc that dispatches requests
// to canned tag API responses based on method and path.
func tagRouteHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/signature"):
			testutil.RespondJSON(w, http.StatusOK, `{
				"signature_type":"X509","verification_status":"verified",
				"x509_certificate":{"id":1,"subject":"CN=T","subject_key_identifier":"a",
					"email":"t@t.com","serial_number":1,"certificate_status":"good",
					"x509_issuer":{"id":2,"subject":"CN=I","subject_key_identifier":"b"}}
			}`)

		case r.Method == http.MethodGet && strings.Contains(path, "/repository/tags/"):
			testutil.RespondJSON(w, http.StatusOK, `{"name":"v1.0.0","target":"abc","protected":false}`)

		case r.Method == http.MethodPost && strings.HasSuffix(path, "/repository/tags"):
			testutil.RespondJSON(w, http.StatusCreated, `{"name":"v2.0.0","target":"def","protected":false}`)

		case r.Method == http.MethodDelete && strings.Contains(path, "/repository/tags/"):
			w.WriteHeader(http.StatusNoContent)

		case r.Method == http.MethodGet && strings.HasSuffix(path, "/repository/tags"):
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"name":"v1.0.0","target":"abc","protected":false}]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})

		case r.Method == http.MethodGet && strings.HasSuffix(path, "/protected_tags/v*"):
			testutil.RespondJSON(w, http.StatusOK, `{"name":"v*","create_access_levels":[]}`)

		case r.Method == http.MethodGet && strings.HasSuffix(path, "/protected_tags"):
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"name":"v*","create_access_levels":[]}]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})

		case r.Method == http.MethodPost && strings.HasSuffix(path, "/protected_tags"):
			testutil.RespondJSON(w, http.StatusCreated, `{"name":"v*","create_access_levels":[]}`)

		case r.Method == http.MethodDelete && strings.Contains(path, "/protected_tags/"):
			w.WriteHeader(http.StatusNoContent)

		default:
			http.NotFound(w, r)
		}
	}
}

// newTagMCPSession is an internal helper for the tags package.
func newTagMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	client := testutil.NewTestClient(t, tagRouteHandler())

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

// assertToolCallSuccess is an internal helper for the tags package.
func assertToolCallSuccess(t *testing.T, session *mcp.ClientSession, ctx context.Context, name string, args map[string]any) {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool(%s) error: %v", name, err)
	}
	if result.IsError {
		for _, c := range result.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				t.Fatalf("CallTool(%s) returned error: %s", name, tc.Text)
			}
		}
		t.Fatalf("CallTool(%s) returned IsError=true", name)
	}
}

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newTagMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_tag_get", map[string]any{argProjectID: "42", argTagName: testTagV100}},
		{"gitlab_tag_create", map[string]any{argProjectID: "42", argTagName: "v2.0.0", "ref": "main"}},
		{"gitlab_tag_delete", map[string]any{argProjectID: "42", argTagName: testTagV100}},
		{"gitlab_tag_list", map[string]any{argProjectID: "42"}},
		{"gitlab_tag_get_signature", map[string]any{argProjectID: "42", argTagName: testTagV100}},
		{"gitlab_tag_list_protected", map[string]any{argProjectID: "42"}},
		{"gitlab_tag_get_protected", map[string]any{argProjectID: "42", argTagName: "v*"}},
		{"gitlab_tag_protect", map[string]any{argProjectID: "42", argTagName: "v*"}},
		{"gitlab_tag_unprotect", map[string]any{argProjectID: "42", argTagName: "v*"}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			assertToolCallSuccess(t, session, ctx, tt.name, tt.args)
		})
	}
}
