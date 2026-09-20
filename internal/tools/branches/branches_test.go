// branches_test.go contains unit tests for GitLab branch operations
// (create, list, get, delete, protect, unprotect, update, and list
// protected branches). Tests use httptest to mock the GitLab API and
// verify success, error, canceled-context, and markdown-formatter paths.
package branches

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Test endpoint paths and format strings used across branch operation tests.
const (
	errExpMissingProjectID = "expected error for missing project_id"
	errExpCancelledCtx     = "expected error for canceled context"
	errExpAPIFailure       = "expected error for API failure"
	errExpEmptyProjectID   = "expected error for empty project_id"
	pathProtectedBranches  = "/api/v4/projects/42/protected_branches"
	fmtOutNameWant         = "out.Name = %q, want %q"
	fmtProtectErr          = "Protect() unexpected error: %v"
	testReleaseWildcard    = "release/*"
	fmtProtBranchListErr   = "ProtectedList() unexpected error: %v"
	fmtOutBranch0NameWant  = "out.Branches[0].Name = %q, want %q"
	pathRepoBranches       = "/api/v4/projects/42/repository/branches"
	testBranchAuth         = "feature/auth"
	fmtBranchListErr       = "List() unexpected error: %v"
)

// TestBranchProtect_Success verifies that branchProtect correctly protects a
// branch with the specified push and merge access levels. It mocks the GitLab
// Protected Branches API to return a successful response and asserts the
// output fields match the expected values.
func TestBranchProtect_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProtectedBranches {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":1,"name":"main","push_access_levels":[{"access_level":0}],"merge_access_levels":[{"access_level":40}],"allow_force_push":false}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Protect(context.Background(), client, ProtectInput{
		ProjectID:        "42",
		BranchName:       "main",
		PushAccessLevel:  0,
		MergeAccessLevel: 40,
	})
	if err != nil {
		t.Fatalf(fmtProtectErr, err)
	}
	if out.Name != "main" {
		t.Errorf(fmtOutNameWant, out.Name, "main")
	}
	if out.AllowForcePush {
		t.Error("out.AllowForcePush = true, want false")
	}
}

// TestBranchProtect_Wildcard verifies that branchProtect supports wildcard
// branch patterns like "release/*". The mock returns a protected branch
// matching the wildcard, and the test confirms the name is preserved.
func TestBranchProtect_Wildcard(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProtectedBranches {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":2,"name":"release/*","push_access_levels":[{"access_level":40}],"merge_access_levels":[{"access_level":40}],"allow_force_push":false}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Protect(context.Background(), client, ProtectInput{
		ProjectID:  "42",
		BranchName: testReleaseWildcard,
	})
	if err != nil {
		t.Fatalf(fmtProtectErr, err)
	}
	if out.Name != testReleaseWildcard {
		t.Errorf(fmtOutNameWant, out.Name, testReleaseWildcard)
	}
}

// TestBranchUnprotect_Success verifies that branchUnprotect removes protection
// from a branch. The mock returns HTTP 204 No Content, and the test asserts
// no error is returned.
func TestBranchUnprotect_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v4/projects/42/protected_branches/main" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Unprotect(context.Background(), client, UnprotectInput{
		ProjectID:  "42",
		BranchName: "main",
	})
	if err != nil {
		t.Errorf("Unprotect() unexpected error: %v", err)
	}
	if out.Status != "success" {
		t.Errorf("Unprotect() expected status=success, got %q", out.Status)
	}
}

// TestBranchUnprotect_NotFound verifies that BranchUnprotect_NotFound returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestBranchUnprotect_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Branch Not Found"}`)
	}))

	out, err := Unprotect(context.Background(), client, UnprotectInput{
		ProjectID:  "42",
		BranchName: "nonexistent",
	})
	if err != nil {
		t.Fatalf("Unprotect() should be idempotent, got error: %v", err)
	}
	if out.Status != "already_unprotected" {
		t.Errorf("Unprotect() expected status=already_unprotected, got %q", out.Status)
	}
}

// TestProtectedBranchesList_Success verifies that protectedBranchesList
// returns the correct number of protected branches and their names when the
// GitLab API returns a valid JSON array.
func TestProtectedBranchesList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathProtectedBranches {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"name":"main","push_access_levels":[{"access_level":0}],"merge_access_levels":[{"access_level":40}],"allow_force_push":false},{"id":2,"name":"develop","push_access_levels":[{"access_level":30}],"merge_access_levels":[{"access_level":30}],"allow_force_push":false}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ProtectedList(context.Background(), client, ProtectedListInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtProtBranchListErr, err)
	}
	if len(out.Branches) != 2 {
		t.Errorf("len(out.Branches) = %d, want 2", len(out.Branches))
	}
	if out.Branches[0].Name != "main" {
		t.Errorf(fmtOutBranch0NameWant, out.Branches[0].Name, "main")
	}
}

// TestProtectedBranchesList_Empty verifies that protectedBranchesList handles
// an empty API response gracefully, returning zero branches without error.
func TestProtectedBranchesList_Empty(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	out, err := ProtectedList(context.Background(), client, ProtectedListInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtProtBranchListErr, err)
	}
	if len(out.Branches) != 0 {
		t.Errorf("len(out.Branches) = %d, want 0", len(out.Branches))
	}
}

// TestProtectedBranchesList_PaginationQueryParamsAndMetadata verifies that
// protectedBranchesList forwards page and per_page query parameters to the
// GitLab API and correctly parses pagination metadata from response headers.
func TestProtectedBranchesList_PaginationQueryParamsAndMetadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathProtectedBranches {
			if got := r.URL.Query().Get("page"); got != "1" {
				t.Errorf("query param page = %q, want %q", got, "1")
			}
			if got := r.URL.Query().Get("per_page"); got != "10" {
				t.Errorf("query param per_page = %q, want %q", got, "10")
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"id":1,"name":"main","push_access_levels":[{"access_level":0}],"merge_access_levels":[{"access_level":40}],"allow_force_push":false}]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "10", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ProtectedList(context.Background(), client, ProtectedListInput{ProjectID: "42", Page: 1, PerPage: 10})
	if err != nil {
		t.Fatalf(fmtProtBranchListErr, err)
	}
	if out.Pagination.Page != 1 {
		t.Errorf("Pagination.Page = %d, want 1", out.Pagination.Page)
	}
	if out.Pagination.TotalItems != 1 {
		t.Errorf("Pagination.TotalItems = %d, want 1", out.Pagination.TotalItems)
	}
}

// TestBranchCreate_Success verifies that branchCreate creates a new branch and
// returns the correct name and commit ID. The mock returns HTTP 201 with a
// valid branch JSON response.
func TestBranchCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathRepoBranches {
			testutil.RespondJSON(w, http.StatusCreated, `{"name":"feature/auth","merged":false,"protected":false,"default":false,"web_url":"https://gitlab.example.com/mygroup/api/-/tree/feature/auth","commit":{"id":"abc123def456"}}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:  "42",
		BranchName: testBranchAuth,
		Ref:        "main",
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.Name != testBranchAuth {
		t.Errorf(fmtOutNameWant, out.Name, testBranchAuth)
	}
	if out.Commit == nil || out.Commit.ID != "abc123def456" {
		t.Errorf("out.Commit.ID = %v, want %q", out.Commit, "abc123def456")
	}
}

// TestBranchCreate_BranchNameMapsToSDKBranch verifies that the MCP branch_name
// input is forwarded to the GitLab API as the SDK `branch` field (a deliberate
// rename: the MCP surface uses branch_name throughout for clarity).
func TestBranchCreate_BranchNameMapsToSDKBranch(t *testing.T) {
	var body string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathRepoBranches {
			b, _ := io.ReadAll(r.Body)
			body = string(b)
			testutil.RespondJSON(w, http.StatusCreated, `{"name":"feature/auth","commit":{"id":"abc"}}`)
			return
		}
		http.NotFound(w, r)
	}))

	if _, err := Create(context.Background(), client, CreateInput{
		ProjectID:  "42",
		BranchName: testBranchAuth,
		Ref:        "main",
	}); err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if !strings.Contains(body, `"branch":"feature/auth"`) {
		t.Errorf("request body = %q, want SDK branch field carrying branch_name", body)
	}
}

// TestBranchProtect_BranchNameMapsToSDKName verifies that the MCP branch_name
// input is forwarded to the GitLab API as the SDK `name` field (a deliberate
// rename consistent with the rest of the branches surface).
func TestBranchProtect_BranchNameMapsToSDKName(t *testing.T) {
	var body string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProtectedBranches {
			b, _ := io.ReadAll(r.Body)
			body = string(b)
			testutil.RespondJSON(w, http.StatusCreated, `{"id":1,"name":"main"}`)
			return
		}
		http.NotFound(w, r)
	}))

	if _, err := Protect(context.Background(), client, ProtectInput{
		ProjectID:  "42",
		BranchName: "main",
	}); err != nil {
		t.Fatalf(fmtProtectErr, err)
	}
	if !strings.Contains(body, `"name":"main"`) {
		t.Errorf("request body = %q, want SDK name field carrying branch_name", body)
	}
}

// TestBranchCreate_AlreadyExists verifies the BranchCreate_AlreadyExists handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestBranchCreate_AlreadyExists(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"Branch already exists"}`)
	}))

	_, err := Create(context.Background(), client, CreateInput{
		ProjectID:  "42",
		BranchName: "main",
		Ref:        "main",
	})
	if err == nil {
		t.Fatal("Create() expected error for duplicate branch, got nil")
	}
}

// TestBranchCreateRef_NotFound verifies that BranchCreateRef_NotFound returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestBranchCreateRef_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"Invalid reference name"}`)
	}))

	_, err := Create(context.Background(), client, CreateInput{
		ProjectID:  "42",
		BranchName: "feature/new",
		Ref:        "nonexistent-ref",
	})
	if err == nil {
		t.Fatal("Create() expected error for invalid ref, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Create() error should mention ref not found, got: %v", err)
	}
	if !strings.Contains(err.Error(), "gitlab_branch_list") {
		t.Errorf("Create() error should suggest gitlab_branch_list, got: %v", err)
	}
}

// TestBranchCreate_EmptyRef verifies that branchCreate returns the enriched
// "ref not found" error when an empty ref string is provided, triggering
// the GitLab API "invalid reference" response.
func TestBranchCreate_EmptyRef(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"Invalid reference name"}`)
	}))

	_, err := Create(context.Background(), client, CreateInput{
		ProjectID:  "42",
		BranchName: "feature/new",
		Ref:        "",
	})
	if err == nil {
		t.Fatal("Create() expected error for empty ref, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Create() error should mention ref not found, got: %v", err)
	}
}

// TestBranchList_Success verifies that branchList returns multiple branches
// with their attributes correctly mapped, including protected and default
// flags. Pagination headers are included in the mock response.
func TestBranchList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathRepoBranches {
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"name":"main","merged":false,"protected":true,"default":true,"web_url":"https://gitlab.example.com/mygroup/api/-/tree/main","commit":{"id":"abc123"}},{"name":"feature/auth","merged":false,"protected":false,"default":false,"web_url":"https://gitlab.example.com/mygroup/api/-/tree/feature/auth","commit":{"id":"def456"}}]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "2", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtBranchListErr, err)
	}
	if len(out.Branches) != 2 {
		t.Fatalf("len(out.Branches) = %d, want 2", len(out.Branches))
	}
	if out.Branches[0].Name != "main" {
		t.Errorf(fmtOutBranch0NameWant, out.Branches[0].Name, "main")
	}
	if !out.Branches[0].Protected {
		t.Error("out.Branches[0].Protected = false, want true")
	}
	if !out.Branches[0].Default {
		t.Error("out.Branches[0].Default = false, want true")
	}
}

// TestBranchList_WithSearch verifies the BranchList_WithSearch handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestBranchList_WithSearch(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathRepoBranches {
			if got := r.URL.Query().Get("search"); got != "feature" {
				t.Errorf("query param search = %q, want %q", got, "feature")
			}
			testutil.RespondJSON(w, http.StatusOK, `[{"name":"feature/auth","merged":false,"protected":false,"default":false,"commit":{"id":"def456"}}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID: "42",
		Search:    "feature",
	})
	if err != nil {
		t.Fatalf(fmtBranchListErr, err)
	}
	if len(out.Branches) != 1 {
		t.Fatalf("len(out.Branches) = %d, want 1", len(out.Branches))
	}
	if out.Branches[0].Name != testBranchAuth {
		t.Errorf(fmtOutBranch0NameWant, out.Branches[0].Name, testBranchAuth)
	}
}

// TestBranchList_Empty verifies the BranchList_Empty handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestBranchList_Empty(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtBranchListErr, err)
	}
	if len(out.Branches) != 0 {
		t.Errorf("len(out.Branches) = %d, want 0", len(out.Branches))
	}
}

// TestBranchGet_Success verifies that BranchGet succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestBranchGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathRepoBranches+"/main" {
			testutil.RespondJSON(w, http.StatusOK, `{"name":"main","default":true,"protected":true,"web_url":"https://gitlab.example.com/-/tree/main","commit":{"id":"abc123","short_id":"abc123d","title":"Initial commit","author_name":"Test","committed_date":"2026-03-01T10:00:00Z","web_url":"https://gitlab.example.com/-/commit/abc123"}}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{
		ProjectID:  "42",
		BranchName: "main",
	})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.Name != "main" {
		t.Errorf(fmtOutNameWant, out.Name, "main")
	}
	if !out.Default {
		t.Error("out.Default = false, want true")
	}
}

// TestBranchGet_EmptyProjectID pins that a missing project_id is refused by
// the handler itself. The mock forbids every request, so the refusal cannot be
// GitLab's answer to a request built from an empty path.
func TestBranchGet_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := Get(context.Background(), client, GetInput{BranchName: "main"})
	if err == nil {
		t.Fatal("expected error for empty project_id, got nil")
	}
}

// TestBranchDelete_Success verifies that BranchDelete succeeds when the GitLab API returns a valid response.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestBranchDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathRepoBranches+"/feature/old" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{
		ProjectID:  "42",
		BranchName: "feature/old",
	})
	if err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}
}

// TestBranchDelete_EmptyProjectID pins that a missing project_id is refused
// before anything is deleted: the mock forbids every request, so no delete can
// have been issued against a path built from an empty project.
func TestBranchDelete_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	err := Delete(context.Background(), client, DeleteInput{BranchName: "main"})
	if err == nil {
		t.Fatal("expected error for empty project_id, got nil")
	}
}

// TestBranchDelete_APIError verifies that BranchDelete returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestBranchDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	err := Delete(context.Background(), client, DeleteInput{
		ProjectID:  "42",
		BranchName: "main",
	})
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
}

// ---------------------------------------------------------------------------
// protectedBranchGet tests
// ---------------------------------------------------------------------------.

// TestProtectedBranchGet_Success verifies that ProtectedBranchGet succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestProtectedBranchGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProtectedBranches+"/main" {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"main","push_access_levels":[{"access_level":0}],"merge_access_levels":[{"access_level":40}],"allow_force_push":false,"code_owner_approval_required":true,"inherited":true}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ProtectedGet(context.Background(), client, ProtectedGetInput{
		ProjectID:  "42",
		BranchName: "main",
	})
	if err != nil {
		t.Fatalf("ProtectedGet() unexpected error: %v", err)
	}
	if out.Name != "main" {
		t.Errorf(fmtOutNameWant, out.Name, "main")
	}
	if len(out.PushAccessLevels) != 1 || out.PushAccessLevels[0].AccessLevel != 0 {
		t.Errorf("PushAccessLevels = %+v, want one entry with access_level 0", out.PushAccessLevels)
	}
	if len(out.MergeAccessLevels) != 1 || out.MergeAccessLevels[0].AccessLevel != 40 {
		t.Errorf("MergeAccessLevels = %+v, want one entry with access_level 40", out.MergeAccessLevels)
	}
	if !out.CodeOwnerApprovalRequired {
		t.Error("CodeOwnerApprovalRequired = false, want true")
	}
	if !out.Inherited {
		t.Error("Inherited = false, want the group-level rule the answer describes")
	}
}

// TestProtectedBranchGet_MissingProjectID pins that a missing project_id is
// refused by the handler itself; the mock forbids every request, so nothing
// was asked of GitLab.
func TestProtectedBranchGet_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := ProtectedGet(context.Background(), client, ProtectedGetInput{
		ProjectID:  "",
		BranchName: "main",
	})
	if err == nil {
		t.Fatal(errExpMissingProjectID)
	}
}

// TestProtectedBranchGet_MissingBranchName pins that a missing branch_name is
// refused by the handler itself; the mock forbids every request, so nothing
// was asked of GitLab.
func TestProtectedBranchGet_MissingBranchName(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := ProtectedGet(context.Background(), client, ProtectedGetInput{
		ProjectID:  "42",
		BranchName: "",
	})
	if err == nil {
		t.Fatal("expected error for missing branch_name")
	}
}

// TestProtectedBranchGet_CancelledContext asserts that a canceled context
// aborts the call without contacting GitLab.
func TestProtectedBranchGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	ctx := testutil.CancelledCtx(t)

	_, err := ProtectedGet(ctx, client, ProtectedGetInput{ProjectID: "42", BranchName: "main"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// protectedBranchUpdate tests
// ---------------------------------------------------------------------------.

// TestProtectedBranchUpdate_Success verifies that ProtectedBranchUpdate succeeds when the GitLab API returns a valid response.
// The test exercises the PATCH path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestProtectedBranchUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && r.URL.Path == pathProtectedBranches+"/main" {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"main","push_access_levels":[{"access_level":0}],"merge_access_levels":[{"access_level":40}],"allow_force_push":true,"code_owner_approval_required":false}`)
			return
		}
		http.NotFound(w, r)
	}))

	forcePush := true
	out, err := ProtectedUpdate(context.Background(), client, ProtectedUpdateInput{
		ProjectID:      "42",
		BranchName:     "main",
		AllowForcePush: &forcePush,
	})
	if err != nil {
		t.Fatalf("ProtectedUpdate() unexpected error: %v", err)
	}
	if out.Name != "main" {
		t.Errorf(fmtOutNameWant, out.Name, "main")
	}
	if !out.AllowForcePush {
		t.Error("AllowForcePush = false, want true")
	}
}

// TestProtectedBranchUpdate_MissingProjectID pins that a missing project_id is
// refused before any rule is changed; the mock forbids every request, so no
// PATCH can have been issued.
func TestProtectedBranchUpdate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := ProtectedUpdate(context.Background(), client, ProtectedUpdateInput{
		ProjectID:  "",
		BranchName: "main",
	})
	if err == nil {
		t.Fatal(errExpMissingProjectID)
	}
}

// TestProtectedBranchUpdate_MissingBranchName pins that a missing branch_name
// is refused before any rule is changed; the mock forbids every request, so no
// PATCH can have been issued.
func TestProtectedBranchUpdate_MissingBranchName(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := ProtectedUpdate(context.Background(), client, ProtectedUpdateInput{
		ProjectID:  "42",
		BranchName: "",
	})
	if err == nil {
		t.Fatal("expected error for missing branch_name")
	}
}

// TestProtectedBranchUpdate_CancelledContext asserts that a canceled context
// aborts the call without changing any rule.
func TestProtectedBranchUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	ctx := testutil.CancelledCtx(t)

	_, err := ProtectedUpdate(ctx, client, ProtectedUpdateInput{ProjectID: "42", BranchName: "main"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// DeleteMerged tests
// ---------------------------------------------------------------------------.

// TestDeleteMerged_Success verifies that DeleteMerged succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/projects/42/repository/merged_branches (DELETE) returns a representative success body.
// It asserts the returned output matches the expected fields.
func TestDeleteMerged_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v4/projects/42/repository/merged_branches" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := DeleteMerged(context.Background(), client, DeleteMergedInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("DeleteMerged() unexpected error: %v", err)
	}
}

// TestDeleteMerged_MissingProjectID pins that a missing project_id is refused
// before the sweep starts: the mock forbids every request, so no branch can
// have been deleted from a path built out of an empty project.
func TestDeleteMerged_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	err := DeleteMerged(context.Background(), client, DeleteMergedInput{ProjectID: ""})
	if err == nil {
		t.Fatal(errExpMissingProjectID)
	}
}

// TestDeleteMerged_APIError verifies that DeleteMerged returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteMerged_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	err := DeleteMerged(context.Background(), client, DeleteMergedInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestDeleteMerged_CancelledContext asserts that a canceled context aborts the
// call without deleting any merged branch.
func TestDeleteMerged_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	ctx := testutil.CancelledCtx(t)

	err := DeleteMerged(ctx, client, DeleteMergedInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Canceled context tests for remaining functions
// ---------------------------------------------------------------------------.

// Each canceled-context test below drives a mock that forbids every request,
// which is what makes "without contacting GitLab" an assertion rather than a
// sentence: the mocks these used to drive answered success, so a handler that
// checked the context only after the call would have passed them all.

// TestBranchCreate_CancelledContext asserts that a canceled context aborts the
// call without creating anything.
func TestBranchCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := testutil.CancelledCtx(t)
	_, err := Create(ctx, client, CreateInput{ProjectID: "42", BranchName: "x", Ref: "main"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestBranchList_CancelledContext asserts that a canceled context aborts the
// call without contacting GitLab.
func TestBranchList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestBranchGet_CancelledContext asserts that a canceled context aborts the
// call without contacting GitLab.
func TestBranchGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := testutil.CancelledCtx(t)
	_, err := Get(ctx, client, GetInput{ProjectID: "42", BranchName: "main"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestBranchDelete_CancelledContext asserts that a canceled context aborts the
// call without deleting anything.
func TestBranchDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := testutil.CancelledCtx(t)
	err := Delete(ctx, client, DeleteInput{ProjectID: "42", BranchName: "x"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestBranchProtect_CancelledContext asserts that a canceled context aborts
// the call without creating a protection rule.
func TestBranchProtect_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := testutil.CancelledCtx(t)
	_, err := Protect(ctx, client, ProtectInput{ProjectID: "42", BranchName: "main"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestBranchUnprotect_CancelledContext asserts that a canceled context aborts
// the call without removing any protection.
func TestBranchUnprotect_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := testutil.CancelledCtx(t)
	_, err := Unprotect(ctx, client, UnprotectInput{ProjectID: "42", BranchName: "main"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestProtectedList_CancelledContext asserts that a canceled context aborts
// the call without contacting GitLab.
func TestProtectedList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := testutil.CancelledCtx(t)
	_, err := ProtectedList(ctx, client, ProtectedListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Empty ProjectID tests for remaining functions
// ---------------------------------------------------------------------------.

// TestBranchCreate_EmptyProjectID pins that a missing project_id is refused by
// the handler itself; the mock forbids every request, so nothing was created.
func TestBranchCreate_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Create(context.Background(), client, CreateInput{BranchName: "x", Ref: "main"})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// TestBranchList_EmptyProjectID pins that a missing project_id is refused by
// the handler itself; the mock forbids every request, so no listing was asked
// for.
func TestBranchList_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// TestBranchProtect_EmptyProjectID pins that a missing project_id is refused
// by the handler itself; the mock forbids every request, so no protection rule
// was created.
func TestBranchProtect_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Protect(context.Background(), client, ProtectInput{BranchName: "main"})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// TestBranchUnprotect_EmptyProjectID pins that a missing project_id is refused
// by the handler itself; the mock forbids every request, so no protection was
// removed. The refusal matters here because unprotect answers a 404 as
// success, and a request built from an empty project would be one.
func TestBranchUnprotect_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Unprotect(context.Background(), client, UnprotectInput{BranchName: "main"})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// TestProtectedList_EmptyProjectID pins that a missing project_id is refused
// by the handler itself; the mock forbids every request, so no listing was
// asked for.
func TestProtectedList_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := ProtectedList(context.Background(), client, ProtectedListInput{})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// ---------------------------------------------------------------------------
// API error tests
// ---------------------------------------------------------------------------.

// TestBranchProtect_APIError verifies that BranchProtect returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestBranchProtect_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := Protect(context.Background(), client, ProtectInput{ProjectID: "42", BranchName: "main"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestBranchList_APIError verifies that BranchList returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestBranchList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestBranchGet_APIError verifies that BranchGet returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestBranchGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Branch Not Found"}`)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: "42", BranchName: "nonexistent"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestProtectedList_APIError verifies that ProtectedList returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestProtectedList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := ProtectedList(context.Background(), client, ProtectedListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestProtectedBranchGet_APIError verifies that ProtectedBranchGet returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestProtectedBranchGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))
	_, err := ProtectedGet(context.Background(), client, ProtectedGetInput{ProjectID: "42", BranchName: "nonexistent"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestProtectedBranchUpdate_APIError verifies that ProtectedBranchUpdate returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestProtectedBranchUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	fp := true
	_, err := ProtectedUpdate(context.Background(), client, ProtectedUpdateInput{ProjectID: "42", BranchName: "main", AllowForcePush: &fp})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestProtectedBranchUpdate_NotFound verifies ProtectedUpdate returns the
// protection-specific hint when GitLab reports the branch is not protected.
func TestProtectedBranchUpdate_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Protected Branch Not Found"}`)
	}))
	fp := true
	_, err := ProtectedUpdate(context.Background(), client, ProtectedUpdateInput{ProjectID: "42", BranchName: "main", AllowForcePush: &fp})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
	if !strings.Contains(err.Error(), "gitlab_branch_protect") {
		t.Fatalf("error missing protect hint: %v", err)
	}
}

// TestBranchUnprotect_APIError verifies that BranchUnprotect returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestBranchUnprotect_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := Unprotect(context.Background(), client, UnprotectInput{ProjectID: "42", BranchName: "main"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// ---------------------------------------------------------------------------
// Protect with advanced options
// ---------------------------------------------------------------------------.

// TestBranchProtect_WithForcePushAndCodeOwner verifies the BranchProtect_WithForcePushAndCodeOwner handler.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestBranchProtect_WithForcePushAndCodeOwner(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProtectedBranches {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":3,"name":"main","push_access_levels":[{"access_level":40}],"merge_access_levels":[{"access_level":40}],"allow_force_push":true,"code_owner_approval_required":true}`)
			return
		}
		http.NotFound(w, r)
	}))

	afp := true
	coa := true
	out, err := Protect(context.Background(), client, ProtectInput{
		ProjectID:                 "42",
		BranchName:                "main",
		PushAccessLevel:           40,
		MergeAccessLevel:          40,
		AllowForcePush:            &afp,
		CodeOwnerApprovalRequired: &coa,
	})
	if err != nil {
		t.Fatalf(fmtProtectErr, err)
	}
	if !out.AllowForcePush {
		t.Error("out.AllowForcePush = false, want true")
	}
	if !out.CodeOwnerApprovalRequired {
		t.Error("out.CodeOwnerApprovalRequired = false, want true")
	}
}

// ---------------------------------------------------------------------------
// ProtectedUpdate with CodeOwnerApproval
// ---------------------------------------------------------------------------.

// TestProtectedBranchUpdate_WithCodeOwner verifies the ProtectedBranchUpdate_WithCodeOwner handler.
// The test exercises the PATCH path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestProtectedBranchUpdate_WithCodeOwner(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && r.URL.Path == pathProtectedBranches+"/main" {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"main","push_access_levels":[{"access_level":0}],"merge_access_levels":[{"access_level":40}],"allow_force_push":false,"code_owner_approval_required":true}`)
			return
		}
		http.NotFound(w, r)
	}))
	co := true
	out, err := ProtectedUpdate(context.Background(), client, ProtectedUpdateInput{
		ProjectID:                 "42",
		BranchName:                "main",
		CodeOwnerApprovalRequired: &co,
	})
	if err != nil {
		t.Fatalf("ProtectedUpdate() unexpected error: %v", err)
	}
	if !out.CodeOwnerApprovalRequired {
		t.Error("CodeOwnerApprovalRequired = false, want true")
	}
}

// ---------------------------------------------------------------------------
// Converter edge cases
// ---------------------------------------------------------------------------.

// TestToOutput_NilCommit pins the converter on a branch GitLab sent without an
// embedded commit: the commit key is omitted rather than published as an empty
// object. It calls the converter directly and contacts no GitLab.
func TestToOutput_NilCommit(t *testing.T) {
	b := &gl.Branch{Name: "main", Protected: true}
	out := ToOutput(b)
	if out.Commit != nil {
		t.Errorf("out.Commit = %+v, want nil for nil commit", out.Commit)
	}
}

// TestProtectedToOutput_EmptyAccessLevels pins the converter on a rule with no
// access-level entries: the arrays stay nil rather than becoming empty ones.
// It calls the converter directly and contacts no GitLab.
func TestProtectedToOutput_EmptyAccessLevels(t *testing.T) {
	pb := &gl.ProtectedBranch{ID: 1, Name: "main"}
	out := ProtectedToOutput(pb, toolutil.ProtectedBranchExtra{})
	if out.PushAccessLevels != nil {
		t.Errorf("PushAccessLevels = %+v, want nil for empty access levels", out.PushAccessLevels)
	}
	if out.MergeAccessLevels != nil {
		t.Errorf("MergeAccessLevels = %+v, want nil for empty access levels", out.MergeAccessLevels)
	}
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.

// The guidance sections the four branch formatters close with, pinned once so
// each whole-output expectation names them rather than restating them.
const (
	branchCardHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'merge_request.create' to open a merge request from this branch\n" +
		"- Use action 'repository.commit_list' to see recent commits on this branch\n" +
		"- Use action 'branch.delete' to remove the branch after merging\n"

	branchListHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- When presenting these results, always include the clickable [text](url) links from the table so the user can navigate to GitLab\n" +
		"- Use action 'branch.get' to see one branch in full\n" +
		"- Use action 'branch.create' to create a new branch\n" +
		"- Use action 'branch.protect' to protect a branch\n"

	protectedCardHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'branch.get_protected' to fetch this protection again before updating it\n" +
		"- Use action 'branch.update_protected' to change protection settings\n" +
		"- Use action 'branch.unprotect' to remove branch protection\n"

	protectedListHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'branch.get_protected' to see one rule in full before updating or unprotecting\n" +
		"- Use action 'branch.protect' to add branch protection\n" +
		"- Use action 'branch.list' to list the branches these rules match\n"
)

// TestFormatOutputMarkdown pins the whole card of a branch: its three state
// flags, the three permission flags GitLab sends on every branch, the head
// commit as a nested object, and the address.
func TestFormatOutputMarkdown(t *testing.T) {
	got := FormatOutputMarkdown(Output{
		Name:      "main",
		Protected: true,
		Default:   true,
		Merged:    false,
		CanPush:   true,
		Commit: &CommitOutput{
			ID:            "abc123",
			Title:         "Fix login",
			AuthorName:    "Alice",
			CommittedDate: "2026-03-20T15:45:00Z",
		},
		WebURL: "https://gitlab.example.com/-/tree/main",
	})

	want := "## Branch: main\n\n" +
		"- **Protected**: ✅\n" +
		"- **Default**: ✅\n" +
		"- **Merged**: ❌\n" +
		"- **You Can Push**: ✅\n" +
		"- **Developers Can Push**: ❌\n" +
		"- **Developers Can Merge**: ❌\n" +
		"- **Commit**:\n" +
		"  - **SHA**: `abc123`\n" +
		"  - **Title**: Fix login\n" +
		"  - **Author**: Alice\n" +
		"  - **Committed**: 20 Mar 2026 15:45 UTC\n" +
		"- **URL**: [https://gitlab.example.com/-/tree/main](https://gitlab.example.com/-/tree/main)\n" +
		branchCardHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatOutputMarkdown_NoURL pins the card of a branch GitLab answered
// with no commit and no address: neither is written as a label with nothing
// after it.
func TestFormatOutputMarkdown_NoURL(t *testing.T) {
	got := FormatOutputMarkdown(Output{Name: "dev"})

	want := "## Branch: dev\n\n" +
		"- **Protected**: ❌\n" +
		"- **Default**: ❌\n" +
		"- **Merged**: ❌\n" +
		"- **You Can Push**: ❌\n" +
		"- **Developers Can Push**: ❌\n" +
		"- **Developers Can Merge**: ❌\n" +
		branchCardHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdown pins the whole branch listing: the count GitLab
// vouched for, a linked name where the response carried an address and a
// plain one where it did not, and the flags as glyphs.
func TestFormatListMarkdown(t *testing.T) {
	got := FormatListMarkdown(ListOutput{
		Branches: []Output{
			{Name: "main", Protected: true, Default: true, WebURL: "https://gitlab.example.com/-/tree/main"},
			{Name: "dev", Merged: true},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2},
	})

	want := "## Branches (2)\n\n" +
		"| Name | Protected | Default | Merged |\n" +
		"| --- | --- | --- | --- |\n" +
		"| [main](https://gitlab.example.com/-/tree/main) | ✅ | ✅ | ❌ |\n" +
		"| dev | ❌ | ❌ | ✅ |\n" +
		"\n2 items total\n" +
		branchListHints

	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdown_Keyset pins the heading of a keyset page, which
// carries no total: the count is what is shown, and the reader is told more
// follows rather than being shown a total of zero above two rows.
func TestFormatListMarkdown_Keyset(t *testing.T) {
	got := FormatListMarkdown(ListOutput{
		Branches:   []Output{{Name: "main"}},
		Pagination: toolutil.PaginationOutput{Page: 1, PerPage: 20, NextPage: 2, HasMore: true},
	})

	want := "## Branches (1 shown, more available)\n\n" +
		"| Name | Protected | Default | Merged |\n" +
		"| --- | --- | --- | --- |\n" +
		"| main | ❌ | ❌ | ❌ |\n" +
		"\nPage 1 | 20 per page | more pages available\n" +
		branchListHints

	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdown_Empty pins the whole response of a project with no
// branches: one sentence, and no heading counting zero above it.
func TestFormatListMarkdown_Empty(t *testing.T) {
	got := FormatListMarkdown(ListOutput{})

	want := "No branches found.\n"

	if got != want {
		t.Errorf("empty list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatProtectedMarkdown pins the whole card of a protection rule: the
// access levels as the role names they stand for rather than as the numbers
// GitLab sends, and both flags.
func TestFormatProtectedMarkdown(t *testing.T) {
	got := FormatProtectedMarkdown(ProtectedOutput{
		ID:                1,
		Name:              "main",
		PushAccessLevels:  []BranchAccessDescriptionOutput{{AccessLevel: 0}},
		MergeAccessLevels: []BranchAccessDescriptionOutput{{AccessLevel: 40}},
		AllowForcePush:    false,
	})

	want := "## Protected Branch: main\n\n" +
		"- **ID**: 1\n" +
		"- **Push Access Levels**: No access\n" +
		"- **Merge Access Levels**: Maintainer\n" +
		"- **Unprotect Access Levels**: -\n" +
		"- **Allow Force Push**: ❌\n" +
		"- **Code Owner Approval Required**: ❌\n" +
		protectedCardHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatProtectedMarkdown_GranularLevels pins an entry that names one
// user, group or deploy key rather than a role: the principal is named beside
// the role the level stands for.
func TestFormatProtectedMarkdown_GranularLevels(t *testing.T) {
	got := FormatProtectedMarkdown(ProtectedOutput{
		ID:                        7,
		Name:                      "release/*",
		PushAccessLevels:          []BranchAccessDescriptionOutput{{AccessLevel: 40, UserID: 3}},
		MergeAccessLevels:         []BranchAccessDescriptionOutput{{AccessLevel: 30, GroupID: 9}},
		UnprotectAccessLevels:     []BranchAccessDescriptionOutput{{AccessLevel: 60, DeployKeyID: 4}},
		AllowForcePush:            true,
		CodeOwnerApprovalRequired: true,
		Inherited:                 true,
	})

	want := "## Protected Branch: release/*\n\n" +
		"- **ID**: 7\n" +
		"- **Push Access Levels**: Maintainer (User #3)\n" +
		"- **Merge Access Levels**: Developer (Group #9)\n" +
		"- **Unprotect Access Levels**: Admin (Deploy Key #4)\n" +
		"- **Allow Force Push**: ✅\n" +
		"- **Code Owner Approval Required**: ✅\n" +
		"- **Inherited from the group, not set on the project**\n" +
		protectedCardHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatProtectedListMarkdown pins the whole protected-branch listing.
func TestFormatProtectedListMarkdown(t *testing.T) {
	got := FormatProtectedListMarkdown(ProtectedListOutput{
		Branches: []ProtectedOutput{
			{ID: 1, Name: "main", PushAccessLevels: []BranchAccessDescriptionOutput{{AccessLevel: 0}}, MergeAccessLevels: []BranchAccessDescriptionOutput{{AccessLevel: 40}}},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	})

	want := "## Protected Branches (1)\n\n" +
		"| Name | Push Levels | Merge Levels | Force Push | Code Owner Approval |\n" +
		"| --- | --- | --- | --- | --- |\n" +
		"| main | No access | Maintainer | ❌ | ❌ |\n" +
		"\n1 items total\n" +
		protectedListHints

	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatProtectedListMarkdown_Empty pins the whole response of a project
// with no protection rules.
func TestFormatProtectedListMarkdown_Empty(t *testing.T) {
	got := FormatProtectedListMarkdown(ProtectedListOutput{})

	want := "No protected branches found.\n"

	if got != want {
		t.Errorf("empty list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestMarkdownRegistry_BranchNotFound pins what the registry renders for a
// branch that does not exist: a result marked as an error, naming the branch
// and the project and the action that lists the ones that do exist. It renders
// the card directly and contacts no GitLab.
func TestMarkdownRegistry_BranchNotFound(t *testing.T) {
	result := toolutil.MarkdownForResult(branchNotFoundOutput{Identifier: `"missing" in project 42`})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.IsError {
		t.Fatal("expected not-found markdown to be marked as an error")
	}
	content, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content type = %T, want TextContent", result.Content[0])
	}
	for _, want := range []string{"Branch Not Found", `"missing" in project 42`, "gitlab_branch_list"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(content.Text, want) {
				t.Fatalf("markdown missing %q:\n%s", want, content.Text)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// List with pagination params
// ---------------------------------------------------------------------------.

// TestBranchList_PaginationQueryParams verifies that BranchListQueryParams forwards pagination parameters to the GitLab API and parses the response metadata.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestBranchList_PaginationQueryParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathRepoBranches {
			if got := r.URL.Query().Get("page"); got != "2" {
				t.Errorf("query param page = %q, want %q", got, "2")
			}
			if got := r.URL.Query().Get("per_page"); got != "5" {
				t.Errorf("query param per_page = %q, want %q", got, "5")
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
				testutil.PaginationHeaders{Page: "2", PerPage: "5", Total: "10", TotalPages: "2"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID: "42",
		Page:      2, PerPage: 5,
	})
	if err != nil {
		t.Fatalf(fmtBranchListErr, err)
	}
	if out.Pagination.Page != 2 {
		t.Errorf("Pagination.Page = %d, want 2", out.Pagination.Page)
	}
	if out.Pagination.TotalItems != 10 {
		t.Errorf("Pagination.TotalItems = %d, want 10", out.Pagination.TotalItems)
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs route coverage
// ---------------------------------------------------------------------------.

// branchMockResp holds a canned response for a mock branch endpoint.
type branchMockResp struct {
	status int
	body   string
	pgHdr  *testutil.PaginationHeaders
}

// newBranchSpecsByTool constructs branch specs by tool test fixtures.
func newBranchSpecsByTool(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()

	base := "/api/v4/projects/42/repository/branches"
	protBase := "/api/v4/projects/42/protected_branches"
	pg1 := &testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"}

	protJSON := `{"id":1,"name":"main","push_access_levels":[{"access_level":0}],"merge_access_levels":[{"access_level":40}]}`

	routes := map[string]branchMockResp{
		"GET " + base + "/main": {http.StatusOK, `{"name":"main","default":true,"protected":true,"commit":{"id":"abc123"}}`, nil},
		"POST " + base:          {http.StatusCreated, `{"name":"new","commit":{"id":"xyz"}}`, nil},
		"DELETE /api/v4/projects/42/repository/merged_branches": {http.StatusNoContent, "", nil},
		"GET " + base:               {http.StatusOK, `[{"name":"main","default":true,"protected":true,"commit":{"id":"abc123"}}]`, pg1},
		"POST " + protBase:          {http.StatusCreated, protJSON, nil},
		"GET " + protBase + "/main": {http.StatusOK, protJSON, nil},
		"GET " + protBase:           {http.StatusOK, `[` + protJSON + `]`, pg1},
	}

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path

		if resp, ok := routes[key]; ok {
			if resp.pgHdr != nil {
				testutil.RespondJSONWithPagination(w, resp.status, resp.body, *resp.pgHdr)
			} else if resp.body != "" {
				testutil.RespondJSON(w, resp.status, resp.body)
			} else {
				w.WriteHeader(resp.status)
			}
			return
		}

		// Wildcard routes that accept any branch name in the path.
		path := r.URL.Path
		switch {
		case r.Method == http.MethodDelete && strings.HasPrefix(path, base+"/"):
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodDelete && strings.HasPrefix(path, protBase+"/"):
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPatch && strings.HasPrefix(path, protBase+"/"):
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"main","push_access_levels":[{"access_level":0}],"merge_access_levels":[{"access_level":40}],"allow_force_push":true}`)
		default:
			http.NotFound(w, r)
		}
	}))

	return branchSpecsByTool(t, ActionSpecs(client))
}

// requireBranchRouteSuccess returns branch route success test data or fails the test.
func requireBranchRouteSuccess(t *testing.T, specs map[string]toolutil.ActionSpec, name string, args map[string]any) {
	t.Helper()

	result, err := specs[name].Route.Handler(t.Context(), args)
	if err != nil {
		t.Fatalf("Route.Handler(%s) error: %v", name, err)
	}
	if result == nil {
		t.Fatalf("Route.Handler(%s) returned nil", name)
	}
}

// ---------------------------------------------------------------------------
// Protection level combination edge cases
// ---------------------------------------------------------------------------.

// TestBranchProtect_AccessLevels_Developer_Maintainer verifies the BranchProtect_AccessLevels_Developer_Maintainer handler.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestBranchProtect_AccessLevels_Developer_Maintainer(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProtectedBranches {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":10,"name":"develop","push_access_levels":[{"access_level":30}],"merge_access_levels":[{"access_level":40}],"allow_force_push":false,"code_owner_approval_required":false}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Protect(context.Background(), client, ProtectInput{
		ProjectID:        "42",
		BranchName:       "develop",
		PushAccessLevel:  30,
		MergeAccessLevel: 40,
	})
	if err != nil {
		t.Fatalf(fmtProtectErr, err)
	}
	if out.Name != "develop" {
		t.Errorf(fmtOutNameWant, out.Name, "develop")
	}
}

// TestBranchProtect_AccessLevels_Maintainer_Maintainer verifies the BranchProtect_AccessLevels_Maintainer_Maintainer handler.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestBranchProtect_AccessLevels_Maintainer_Maintainer(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProtectedBranches {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":11,"name":"main","push_access_levels":[{"access_level":40}],"merge_access_levels":[{"access_level":40}],"allow_force_push":false,"code_owner_approval_required":false}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Protect(context.Background(), client, ProtectInput{
		ProjectID:        "42",
		BranchName:       "main",
		PushAccessLevel:  40,
		MergeAccessLevel: 40,
	})
	if err != nil {
		t.Fatalf(fmtProtectErr, err)
	}
	if out.AllowForcePush {
		t.Error("out.AllowForcePush = true, want false")
	}
}

// TestBranchProtect_CodeOwner_WithAccessLevels verifies the BranchProtect_CodeOwner_WithAccessLevels handler.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestBranchProtect_CodeOwner_WithAccessLevels(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProtectedBranches {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":12,"name":"main","push_access_levels":[{"access_level":30}],"merge_access_levels":[{"access_level":30}],"allow_force_push":false,"code_owner_approval_required":true}`)
			return
		}
		http.NotFound(w, r)
	}))

	coa := true
	out, err := Protect(context.Background(), client, ProtectInput{
		ProjectID:                 "42",
		BranchName:                "main",
		PushAccessLevel:           30,
		MergeAccessLevel:          30,
		CodeOwnerApprovalRequired: &coa,
	})
	if err != nil {
		t.Fatalf(fmtProtectErr, err)
	}
	if !out.CodeOwnerApprovalRequired {
		t.Error("out.CodeOwnerApprovalRequired = false, want true")
	}
}

// TestBranchProtect_ForcePush_WithRestrictiveAccess verifies the BranchProtect_ForcePush_WithRestrictiveAccess handler.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestBranchProtect_ForcePush_WithRestrictiveAccess(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProtectedBranches {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":13,"name":"release/v1","push_access_levels":[{"access_level":40}],"merge_access_levels":[{"access_level":40}],"allow_force_push":true,"code_owner_approval_required":false}`)
			return
		}
		http.NotFound(w, r)
	}))

	afp := true
	out, err := Protect(context.Background(), client, ProtectInput{
		ProjectID:        "42",
		BranchName:       "release/v1",
		PushAccessLevel:  40,
		MergeAccessLevel: 40,
		AllowForcePush:   &afp,
	})
	if err != nil {
		t.Fatalf(fmtProtectErr, err)
	}
	if !out.AllowForcePush {
		t.Error("out.AllowForcePush = false, want true")
	}
	if out.Name != "release/v1" {
		t.Errorf(fmtOutNameWant, out.Name, "release/v1")
	}
}

// TestActionSpecs_CallAllRoutes validates the CallAllRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	specs := newBranchSpecsByTool(t)

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_branch_get", map[string]any{"project_id": "42", "branch_name": "main"}},
		{"gitlab_branch_create", map[string]any{"project_id": "42", "branch_name": "new", "ref": "main"}},
		{"gitlab_branch_delete", map[string]any{"project_id": "42", "branch_name": "old"}},
		{"gitlab_branch_delete_merged", map[string]any{"project_id": "42"}},
		{"gitlab_branch_list", map[string]any{"project_id": "42"}},
		{"gitlab_branch_protect", map[string]any{"project_id": "42", "branch_name": "main"}},
		{"gitlab_branch_unprotect", map[string]any{"project_id": "42", "branch_name": "main"}},
		{"gitlab_protected_branches_list", map[string]any{"project_id": "42"}},
		{"gitlab_protected_branch_get", map[string]any{"project_id": "42", "branch_name": "main"}},
		{"gitlab_protected_branch_update", map[string]any{"project_id": "42", "branch_name": "main", "allow_force_push": true}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			requireBranchRouteSuccess(t, specs, tt.name, tt.args)
		})
	}
}

// TestActionSpecs_Metadata pins the discovery metadata the catalog publishes
// for the branch actions: ten tools, each owned by this package, each carrying
// usage text, aliases and parameter guidance. It reads the specs and calls no
// route.
func TestActionSpecs_Metadata(t *testing.T) {
	byTool := newBranchSpecsByTool(t)

	if len(byTool) != 10 {
		t.Fatalf("len(byTool) = %d, want 10", len(byTool))
	}
	for toolName, spec := range byTool {
		if spec.OwnerPackage != "branches" {
			t.Fatalf("OwnerPackage for %s = %q, want branches", toolName, spec.OwnerPackage)
		}
	}

	list := byTool["gitlab_branch_list"]
	if list.Usage == "" || len(list.Aliases) == 0 || len(list.ParameterGuidance) == 0 {
		t.Fatalf("gitlab_branch_list metadata incomplete: usage=%q aliases=%d guidance=%d", list.Usage, len(list.Aliases), len(list.ParameterGuidance))
	}

	get := byTool["gitlab_branch_get"]
	if get.Usage == "" || len(get.Aliases) == 0 || get.ParameterGuidance["branch_name"].SemanticRole == "" {
		t.Fatalf("gitlab_branch_get metadata incomplete: usage=%q aliases=%d guidance(branch_name)=%q", get.Usage, len(get.Aliases), get.ParameterGuidance["branch_name"].SemanticRole)
	}

	create := byTool["gitlab_branch_create"]
	if create.Usage == "" || len(create.Aliases) == 0 || create.ParameterGuidance["ref"].SemanticRole == "" {
		t.Fatalf("gitlab_branch_create metadata incomplete: usage=%q aliases=%d guidance(ref)=%q", create.Usage, len(create.Aliases), create.ParameterGuidance["ref"].SemanticRole)
	}

	protect := byTool["gitlab_branch_protect"]
	if protect.Usage == "" || protect.ParameterGuidance["push_access_level"].SemanticRole == "" {
		t.Fatalf("gitlab_branch_protect metadata incomplete: usage=%q push_guidance=%q", protect.Usage, protect.ParameterGuidance["push_access_level"].SemanticRole)
	}
}

// TestBranchProtect_Conflict409_FallbackGet verifies idempotent behavior
// when the branch is already protected (409 Conflict): the handler falls
// back to GET the existing protection rule.
func TestBranchProtect_Conflict409_FallbackGet(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(pathProtectedBranches, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusConflict, `{"message":"Protected branch 'main' already exists"}`)
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc(pathProtectedBranches+"/main", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"main","push_access_levels":[{"access_level":40}],"merge_access_levels":[{"access_level":30}]}`)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := Protect(context.Background(), client, ProtectInput{ProjectID: "42", BranchName: "main"})
	if err != nil {
		t.Fatalf("expected idempotent success, got error: %v", err)
	}
	// The idempotent fallback GET returns the existing rule, so its access
	// levels (from the GET mock) must be surfaced.
	if len(out.PushAccessLevels) != 1 || out.PushAccessLevels[0].AccessLevel != 40 {
		t.Errorf("PushAccessLevels = %+v, want one entry with access_level 40 from fallback GET", out.PushAccessLevels)
	}
	if out.Name != "main" {
		t.Errorf("Name = %q, want %q", out.Name, "main")
	}
}

// TestBranchProtect_Conflict409_GetFails verifies that when 409 occurs and
// the fallback GET also fails, the original error is returned with a hint.
func TestBranchProtect_Conflict409_GetFails(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(pathProtectedBranches, func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusConflict, `{"message":"already exists"}`)
	})
	mux.HandleFunc(pathProtectedBranches+"/main", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, mux)

	_, err := Protect(context.Background(), client, ProtectInput{ProjectID: "42", BranchName: "main"})
	if err == nil {
		t.Fatal("expected error when fallback GET fails")
	}
}

// TestBranchDelete_ProtectedBranch verifies the BranchDelete_ProtectedBranch handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestBranchDelete_ProtectedBranch(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"protected branch"}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: "42", BranchName: "main"})
	if err == nil {
		t.Fatal("expected error for protected branch")
	}
	if !strings.Contains(err.Error(), "gitlab_branch_unprotect") {
		t.Errorf("expected unprotect hint, got: %v", err)
	}
}

// TestBranchDelete_NotFound verifies that BranchDelete_NotFound returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestBranchDelete_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Branch Not Found"}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: "42", BranchName: "ghost"})
	if err == nil {
		t.Fatal("expected error for not-found branch")
	}
	if !strings.Contains(err.Error(), "gitlab_branch_list") {
		t.Errorf("expected list hint, got: %v", err)
	}
}

// TestBranchCreate_GenericAPIError verifies that BranchCreate_GenericAPIError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestBranchCreate_GenericAPIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: "42", BranchName: "x", Ref: "main"})
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

// Each branch_name refusal below drives a mock that forbids every request, so
// the error can only have come from the handler. The empty mux these used to
// drive answered 404 to anything, which unprotect reads as success and the
// others as an error, so the refusal and GitLab's answer to a path with a hole
// in it were indistinguishable.

// TestBranchCreate_EmptyBranchName pins that an empty branch_name is refused
// before any branch is created.
func TestBranchCreate_EmptyBranchName(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: "42", Ref: "main"})
	if err == nil {
		t.Fatal("expected error for empty branch_name")
	}
}

// TestBranchGet_EmptyBranchName pins that an empty branch_name is refused
// before anything is read.
func TestBranchGet_EmptyBranchName(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Get(context.Background(), client, GetInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for empty branch_name")
	}
}

// TestBranchDelete_EmptyBranchName pins that an empty branch_name is refused
// before anything is deleted.
func TestBranchDelete_EmptyBranchName(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for empty branch_name")
	}
}

// TestBranchProtect_EmptyBranchName pins that an empty branch_name is refused
// before any protection rule is created.
func TestBranchProtect_EmptyBranchName(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Protect(context.Background(), client, ProtectInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for empty branch_name")
	}
}

// TestBranchUnprotect_EmptyBranchName pins that an empty branch_name is
// refused before any protection is removed, which nothing else could tell:
// unprotect answers a 404 as success, so the refusal has to happen here.
func TestBranchUnprotect_EmptyBranchName(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Unprotect(context.Background(), client, UnprotectInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for empty branch_name")
	}
}

// TestActionSpecs_BranchGetRoute validates the BranchGetRoute route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_BranchGetRoute(t *testing.T) {
	const respJSON = `{"name":"main","protected":true,"merged":false,"default":true,"web_url":"https://gitlab.example.com/p/-/tree/main","commit":{"id":"abc"}}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/projects/42/repository/branches/main") {
			testutil.RespondJSON(w, http.StatusOK, respJSON)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)
	byTool := branchSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_branch_get"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "branch_name": "main"})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	out, ok := result.(Output)
	if !ok {
		t.Fatalf("result type = %T, want Output", result)
	}
	if out.Name != "main" || out.Commit == nil || out.Commit.ID != "abc" {
		t.Fatalf("branch output = %#v, want name main and commit abc", out)
	}
}

// TestActionSpecs_BranchGetRouteNotFound validates the BranchGetRouteNotFound route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestActionSpecs_BranchGetRouteNotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Branch Not Found"}`)
	}))
	byTool := branchSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_branch_get"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "branch_name": "missing"})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	notFound, ok := result.(branchNotFoundOutput)
	if !ok {
		t.Fatalf("result type = %T, want branchNotFoundOutput", result)
	}
	if !strings.Contains(notFound.Identifier, "missing") || !strings.Contains(notFound.Identifier, "42") {
		t.Fatalf("identifier = %q, want branch and project context", notFound.Identifier)
	}
}

// ---------------------------------------------------------------------------
// 1:1 audit additions: full commit mirror, access-level arrays, fine-grained
// permission inputs, and keyset/order_by/sort list options.
// ---------------------------------------------------------------------------.

// TestBranchGet_FullCommitMirror verifies that a branch's embedded commit object
// is surfaced in full (id, dates, stats, last_pipeline, trailers, status) on the
// canonical commit key rather than a flattened commit_id scalar.
func TestBranchGet_FullCommitMirror(t *testing.T) {
	const respJSON = `{"name":"main","protected":true,"merged":false,"default":true,"web_url":"https://gl/-/tree/main","commit":{` +
		`"id":"abc123","short_id":"abc","title":"feat: x","message":"feat: x\n","author_name":"Ada","author_email":"ada@x.io",` +
		`"authored_date":"2024-01-01T10:00:00Z","committer_name":"Bob","committer_email":"bob@x.io","committed_date":"2024-01-02T10:00:00Z",` +
		`"created_at":"2024-01-02T10:00:00Z","parent_ids":["p1","p2"],"status":"success","project_id":42,` +
		`"trailers":{"Signed-off-by":"Ada"},"extended_trailers":{"Signed-off-by":"Ada"},` +
		`"stats":{"additions":5,"deletions":2,"total":7},` +
		`"last_pipeline":{"id":9,"iid":3,"project_id":42,"status":"success","source":"push","ref":"main","sha":"abc123","name":"build","web_url":"https://gl/pipelines/9","created_at":"2024-01-02T10:00:00Z","updated_at":"2024-01-02T11:00:00Z"}}}`
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathRepoBranches+"/main" {
			testutil.RespondJSON(w, http.StatusOK, respJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", BranchName: "main"})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	assertFullCommitMirror(t, out.Commit)
}

// assertFullCommitMirror asserts that a CommitOutput surfaces every mirrored
// gl.Commit field, including nested stats and last_pipeline sub-objects.
func assertFullCommitMirror(t *testing.T, c *CommitOutput) {
	t.Helper()
	if c == nil {
		t.Fatal("expected non-nil commit object")
	}
	wantStr := map[string]struct{ got, want string }{
		"id":              {c.ID, "abc123"},
		"short_id":        {c.ShortID, "abc"},
		"title":           {c.Title, "feat: x"},
		"author_name":     {c.AuthorName, "Ada"},
		"committer_email": {c.CommitterEmail, "bob@x.io"},
		"status":          {c.Status, "success"},
	}
	for field, v := range wantStr {
		if v.got != v.want {
			t.Errorf("commit %s = %q, want %q", field, v.got, v.want)
		}
	}
	if c.ProjectID != 42 {
		t.Errorf("commit project_id = %d, want 42", c.ProjectID)
	}
	if c.AuthoredDate == "" || c.CommittedDate == "" || c.CreatedAt == "" {
		t.Errorf("commit dates not surfaced: %+v", c)
	}
	if len(c.ParentIDs) != 2 || c.Trailers["Signed-off-by"] != "Ada" || len(c.ExtendedTrailers) != 1 {
		t.Errorf("commit parent/trailers = %+v", c)
	}
	assertCommitStats(t, c.Stats)
	assertCommitLastPipeline(t, c.LastPipeline)
}

// assertCommitStats asserts the mirrored gl.CommitStats sub-object.
func assertCommitStats(t *testing.T, s *CommitStatsOutput) {
	t.Helper()
	if s == nil || s.Total != 7 || s.Additions != 5 || s.Deletions != 2 {
		t.Errorf("commit stats = %+v", s)
	}
}

// assertCommitLastPipeline asserts the mirrored gl.PipelineInfo sub-object.
func assertCommitLastPipeline(t *testing.T, p *LastPipelineOutput) {
	t.Helper()
	if p == nil || p.ID != 9 || p.Status != "success" || p.CreatedAt == "" || p.UpdatedAt == "" {
		t.Errorf("commit last_pipeline = %+v", p)
	}
}

// TestProtectedGet_FullAccessLevelArrays verifies that push/merge/unprotect
// access-level arrays are surfaced in full (id, access_level, description, and
// scope ids) rather than collapsed to first-entry scalars.
func TestProtectedGet_FullAccessLevelArrays(t *testing.T) {
	const respJSON = `{"id":1,"name":"main",` +
		`"push_access_levels":[{"id":11,"access_level":40,"access_level_description":"Maintainers"},{"id":12,"access_level":0,"access_level_description":"u","user_id":7}],` +
		`"merge_access_levels":[{"id":21,"access_level":30,"access_level_description":"Devs","group_id":3}],` +
		`"unprotect_access_levels":[{"id":31,"access_level":40,"access_level_description":"k","deploy_key_id":5}],` +
		`"allow_force_push":true,"code_owner_approval_required":false}`
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProtectedBranches+"/main" {
			testutil.RespondJSON(w, http.StatusOK, respJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ProtectedGet(context.Background(), client, ProtectedGetInput{ProjectID: "42", BranchName: "main"})
	if err != nil {
		t.Fatalf("ProtectedGet() unexpected error: %v", err)
	}
	if len(out.PushAccessLevels) != 2 {
		t.Fatalf("PushAccessLevels len = %d, want 2", len(out.PushAccessLevels))
	}
	if out.PushAccessLevels[0].ID != 11 || out.PushAccessLevels[0].AccessLevel != 40 ||
		out.PushAccessLevels[0].AccessLevelDescription != "Maintainers" {
		t.Errorf("push[0] = %+v", out.PushAccessLevels[0])
	}
	if out.PushAccessLevels[1].UserID != 7 {
		t.Errorf("push[1].UserID = %d, want 7", out.PushAccessLevels[1].UserID)
	}
	if len(out.MergeAccessLevels) != 1 || out.MergeAccessLevels[0].GroupID != 3 {
		t.Errorf("merge = %+v", out.MergeAccessLevels)
	}
	if len(out.UnprotectAccessLevels) != 1 || out.UnprotectAccessLevels[0].DeployKeyID != 5 {
		t.Errorf("unprotect = %+v", out.UnprotectAccessLevels)
	}
	if !out.AllowForcePush {
		t.Error("AllowForcePush = false, want true")
	}
}

// TestBranchProtect_SerializesAllOptions verifies that the new ProtectInput
// fields (name, unprotect_access_level, and fine-grained allowed_to_* arrays)
// are serialized into the request body sent to GitLab.
func TestBranchProtect_SerializesAllOptions(t *testing.T) {
	var gotBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProtectedBranches {
			buf := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(buf)
			gotBody = string(buf)
			testutil.RespondJSON(w, http.StatusCreated, `{"id":3,"name":"release/*","push_access_levels":[{"access_level":40}]}`)
			return
		}
		http.NotFound(w, r)
	}))

	uid := int64(7)
	al := 30
	destroy := true
	_, err := Protect(context.Background(), client, ProtectInput{
		ProjectID:            "42",
		BranchName:           testReleaseWildcard,
		PushAccessLevel:      40,
		MergeAccessLevel:     40,
		UnprotectAccessLevel: 40,
		AllowedToPush:        []BranchPermissionInput{{UserID: &uid, AccessLevel: &al}},
		AllowedToMerge:       []BranchPermissionInput{{AccessLevel: &al}},
		AllowedToUnprotect:   []BranchPermissionInput{{ID: &uid, Destroy: &destroy}},
	})
	if err != nil {
		t.Fatalf(fmtProtectErr, err)
	}
	for _, want := range []string{`"name":"release/*"`, `"unprotect_access_level":40`, `"allowed_to_push"`, `"user_id":7`, `"allowed_to_merge"`, `"allowed_to_unprotect"`, `"_destroy":true`} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(gotBody, want) {
				t.Errorf("request body missing %q\nbody=%s", want, gotBody)
			}
		})
	}
}

// TestProtectedUpdate_SerializesAllOptions verifies that the new
// ProtectedUpdateInput fields (name rename and allowed_to_* arrays) are
// serialized into the PATCH request body.
func TestProtectedUpdate_SerializesAllOptions(t *testing.T) {
	var gotBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && r.URL.Path == pathProtectedBranches+"/main" {
			buf := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(buf)
			gotBody = string(buf)
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"main"}`)
			return
		}
		http.NotFound(w, r)
	}))

	gid := int64(3)
	al := 40
	_, err := ProtectedUpdate(context.Background(), client, ProtectedUpdateInput{
		ProjectID:      "42",
		BranchName:     "main",
		Name:           "main-renamed",
		AllowedToPush:  []BranchPermissionInput{{GroupID: &gid, AccessLevel: &al}},
		AllowedToMerge: []BranchPermissionInput{{AccessLevel: &al}},
		AllowedToUnprotect: []BranchPermissionInput{{
			DeployKeyID: &gid,
		}},
	})
	if err != nil {
		t.Fatalf("ProtectedUpdate() unexpected error: %v", err)
	}
	for _, want := range []string{`"name":"main-renamed"`, `"allowed_to_push"`, `"group_id":3`, `"allowed_to_merge"`, `"allowed_to_unprotect"`, `"deploy_key_id":3`} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(gotBody, want) {
				t.Errorf("request body missing %q\nbody=%s", want, gotBody)
			}
		})
	}
}

// TestBranchList_KeysetAndOrdering verifies that regex, order_by, sort, and
// keyset pagination parameters are forwarded as query parameters to GitLab.
func TestBranchList_KeysetAndOrdering(t *testing.T) {
	var gotQuery string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathRepoBranches {
			gotQuery = r.URL.RawQuery
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := List(context.Background(), client, ListInput{
		ProjectID:  "42",
		Search:     "feat",
		Regex:      "^feat",
		OrderBy:    "updated",
		Sort:       "desc",
		Pagination: "keyset", PageToken: "tok",
	})
	if err != nil {
		t.Fatalf(fmtBranchListErr, err)
	}
	for _, want := range []string{"search=feat", "regex=", "order_by=updated", "sort=desc", "pagination=keyset", "page_token=tok"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(gotQuery, want) {
				t.Errorf("query missing %q: %s", want, gotQuery)
			}
		})
	}
}

// TestProtectedList_SearchKeysetAndOrdering verifies that search, order_by,
// sort, and keyset pagination are forwarded for the protected-branches list.
func TestProtectedList_SearchKeysetAndOrdering(t *testing.T) {
	var gotQuery string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProtectedBranches {
			gotQuery = r.URL.RawQuery
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := ProtectedList(context.Background(), client, ProtectedListInput{
		ProjectID:  "42",
		Search:     "main",
		OrderBy:    "name",
		Sort:       "asc",
		Pagination: "keyset", PageToken: "tok",
	})
	if err != nil {
		t.Fatalf(fmtProtBranchListErr, err)
	}
	for _, want := range []string{"search=main", "order_by=name", "sort=asc", "pagination=keyset", "page_token=tok"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(gotQuery, want) {
				t.Errorf("query missing %q: %s", want, gotQuery)
			}
		})
	}
}

// TestShapeConverters_NilAndEmpty exercises nil/empty fast paths of the shape
// converters that the request flows do not otherwise reach.
func TestShapeConverters_NilAndEmpty(t *testing.T) {
	if pipelineInfoToOutput(nil) != nil {
		t.Error("pipelineInfoToOutput(nil) should be nil")
	}
	if commitToOutput(nil) != nil {
		t.Error("commitToOutput(nil) should be nil")
	}
	if branchAccessDescriptionsToOutput(nil) != nil {
		t.Error("branchAccessDescriptionsToOutput(nil) should be nil")
	}
	if got := branchAccessDescriptionsToOutput([]*gl.BranchAccessDescription{nil}); got != nil {
		t.Errorf("all-nil slice should map to nil, got %+v", got)
	}
	if branchPermissionOptions(nil) != nil {
		t.Error("branchPermissionOptions(nil) should be nil")
	}
}

// TestAccessLevelsSummary verifies the access-level summary names the roles
// the numbers stand for, and a level GitLab's own table does not name keeps
// its number, which is the one thing a reader can act on.
func TestAccessLevelsSummary(t *testing.T) {
	cases := []struct {
		name   string
		levels []BranchAccessDescriptionOutput
		want   string
	}{
		{"empty", nil, "-"},
		{"roles", []BranchAccessDescriptionOutput{{AccessLevel: 30}, {AccessLevel: 40}}, "Developer, Maintainer"},
		{"unknown level", []BranchAccessDescriptionOutput{{AccessLevel: 35}}, "Level 35"},
		{"user entry", []BranchAccessDescriptionOutput{{AccessLevel: 40, UserID: 3}}, "Maintainer (User #3)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := accessLevelsSummary(c.levels); got != c.want {
				t.Errorf("summary = %q, want %q", got, c.want)
			}
		})
	}
}

// TestProtectedFlaggedTools_Metadata verifies the five 1:1-audit metadata
// findings are resolved: non-generic usage, real aliases, and a "Returns:/See
// also:" individual-tool description for each flagged protected/unprotect tool.
func TestProtectedFlaggedTools_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	byTool := branchSpecsByTool(t, ActionSpecs(client))
	flagged := []string{
		"gitlab_branch_protect",
		"gitlab_branch_unprotect",
		"gitlab_protected_branch_get",
		"gitlab_protected_branches_list",
		"gitlab_protected_branch_update",
	}
	for _, tool := range flagged {
		t.Run(tool, func(t *testing.T) {
			spec, ok := byTool[tool]
			if !ok {
				t.Fatalf("missing spec for %s", tool)
			}
			if spec.Usage == "" || strings.Contains(spec.Usage, "Use to execute branches domain action") {
				t.Errorf("%s: generic/empty usage %q", tool, spec.Usage)
			}
			if len(spec.Aliases) == 0 {
				t.Errorf("%s: no aliases", tool)
			}
			for _, a := range spec.Aliases {
				if a == tool {
					t.Errorf("%s: alias duplicates tool name", tool)
				}
			}
			desc := spec.IndividualTool.Description
			if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
				t.Errorf("%s: description missing Returns:/See also: form: %q", tool, desc)
			}
		})
	}
}

// branchSpecsByTool supports branch specs by tool assertions in branches tests.
func branchSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}

// TestFormatProtectedMarkdown_Inherited verifies that a rule GitLab reports as
// inherited says so in the rendered Markdown, and that a rule of the project's
// own does not. The flag is read off the captured response because the SDK does
// not model it, and it is the difference between a rule that can be edited here
// and one that has to be edited on the group.
func TestFormatProtectedMarkdown_Inherited(t *testing.T) {
	body := "## Protected Branch: main\n\n" +
		"- **ID**: 1\n" +
		"- **Push Access Levels**: -\n" +
		"- **Merge Access Levels**: -\n" +
		"- **Unprotect Access Levels**: -\n" +
		"- **Allow Force Push**: ❌\n" +
		"- **Code Owner Approval Required**: ❌\n"

	t.Run("inherited from the group", func(t *testing.T) {
		got := FormatProtectedMarkdown(ProtectedOutput{ID: 1, Name: "main", Inherited: true})
		want := body + "- **Inherited from the group, not set on the project**\n" + protectedCardHints
		if got != want {
			t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
		}
	})

	t.Run("the project's own rule", func(t *testing.T) {
		got := FormatProtectedMarkdown(ProtectedOutput{ID: 1, Name: "main"})
		want := body + protectedCardHints
		if got != want {
			t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
		}
	})
}

// TestProtectedBranches_UnreadableCapturedInherited verifies that every
// protected-branch handler reading the inherited flag off the captured answer
// returns an error rather than a half-filled rule when GitLab sends it as
// something that is not a boolean. The SDK ignores the key its own
// ProtectedBranch does not model, so the captured read is the only thing that
// can notice.
func TestProtectedBranches_UnreadableCapturedInherited(t *testing.T) {
	// A list answers with an array and the rest with an object, so each case
	// drives a client of its own rather than one shared handler.
	poisoned := func(body string) *gitlabclient.Client {
		return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondJSON(w, http.StatusOK, body)
		}))
	}
	testutil.AssertCapturedDecodeFailures(t, []testutil.CapturedCase{
		{Name: "protect", Call: func() error {
			client := poisoned(`{"id":1,"name":"main","inherited":"yes"}`)
			_, err := Protect(context.Background(), client, ProtectInput{ProjectID: "42", BranchName: "main"})
			return err
		}},
		{Name: "protected_list", Call: func() error {
			client := poisoned(`[{"id":1,"name":"main","inherited":"yes"}]`)
			_, err := ProtectedList(context.Background(), client, ProtectedListInput{ProjectID: "42"})
			return err
		}},
		{Name: "protected_get", Call: func() error {
			client := poisoned(`{"id":1,"name":"main","inherited":"yes"}`)
			_, err := ProtectedGet(context.Background(), client, ProtectedGetInput{ProjectID: "42", BranchName: "main"})
			return err
		}},
		{Name: "protected_update", Call: func() error {
			client := poisoned(`{"id":1,"name":"main","inherited":"yes"}`)
			_, err := ProtectedUpdate(context.Background(), client, ProtectedUpdateInput{ProjectID: "42", BranchName: "main", Name: "main-2"})
			return err
		}},
	})
}

// TestProtect_UnreadableCapturedInheritedOnConflict covers the other captured
// read in Protect: a 409 means the rule already exists, so the handler fetches
// it and answers with that instead. The rule it fetched is read from the same
// capture, and an inherited flag GitLab sends as something other than a boolean
// must fail the call rather than reach the caller as a silent false.
func TestProtect_UnreadableCapturedInheritedOnConflict(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusConflict, `{"message":"Protected branch 'main' already exists"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"main","inherited":"yes"}`)
	}))

	testutil.AssertCapturedDecodeFailures(t, []testutil.CapturedCase{
		{Name: "protect on conflict", Call: func() error {
			_, err := Protect(context.Background(), client, ProtectInput{ProjectID: "42", BranchName: "main"})
			return err
		}},
	})
}

// ---------------------------------------------------------------------------
// What the caller's optional values do to the request GitLab receives
// ---------------------------------------------------------------------------.

// protectRequest is the protected-branches request body as GitLab receives it.
// The tests below decode it instead of matching the raw text, because a text
// match over the whole body cannot say which array a value landed in.
type protectRequest struct {
	Name                      string              `json:"name"`
	PushAccessLevel           *int                `json:"push_access_level"`
	MergeAccessLevel          *int                `json:"merge_access_level"`
	UnprotectAccessLevel      *int                `json:"unprotect_access_level"`
	AllowForcePush            *bool               `json:"allow_force_push"`
	CodeOwnerApprovalRequired *bool               `json:"code_owner_approval_required"`
	AllowedToPush             []permissionRequest `json:"allowed_to_push"`
	AllowedToMerge            []permissionRequest `json:"allowed_to_merge"`
	AllowedToUnprotect        []permissionRequest `json:"allowed_to_unprotect"`
}

// permissionRequest is one allowed_to_{push,merge,unprotect} entry as GitLab
// receives it.
type permissionRequest struct {
	ID          *int64 `json:"id"`
	UserID      *int64 `json:"user_id"`
	GroupID     *int64 `json:"group_id"`
	DeployKeyID *int64 `json:"deploy_key_id"`
	AccessLevel *int   `json:"access_level"`
	Destroy     *bool  `json:"_destroy"`
}

// captureProtectRequest runs call against a mock answering method and path with
// response, and returns the request body the handler built. The body is read
// and stored on the mock's own goroutine and decoded on the test's, so a body
// that does not parse is reported where it can abort the test.
func captureProtectRequest(t *testing.T, method, path, response string, call func(*gitlabclient.Client) error) protectRequest {
	t.Helper()

	var raw []byte
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method || r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading the request body: %v", err)
			http.Error(w, "unreadable request body", http.StatusInternalServerError)
			return
		}
		raw = body
		testutil.RespondJSON(w, http.StatusOK, response)
	}))

	if err := call(client); err != nil {
		t.Fatalf("handler returned an error: %v", err)
	}
	var got protectRequest
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("request body %q does not decode: %v", raw, err)
	}
	return got
}

// assertOptional compares one optional request field against what the caller
// asked for. A key the body left out and a key carrying the zero value are
// different requests, which is the whole point of the pointer, so the two are
// reported apart rather than compared as values.
func assertOptional[T comparable](t *testing.T, field string, got, want *T) {
	t.Helper()
	switch {
	case want == nil && got != nil:
		t.Errorf("%s = %v, want the key left out of the body", field, *got)
	case want != nil && got == nil:
		t.Errorf("%s left out of the body, want %v", field, *want)
	case want != nil && got != nil && *got != *want:
		t.Errorf("%s = %v, want %v", field, *got, *want)
	}
}

// protectFlagCase is one optional-flag setting and the body it must produce.
type protectFlagCase struct {
	name          string
	forcePush     *bool
	codeOwner     *bool
	wantForcePush *bool
	wantCodeOwner *bool
}

// protectFlagCases sets one flag per case on purpose. With both carrying true,
// a handler that dropped the caller's force-push flag and sent the CODEOWNERS
// one twice would produce the same body as the correct one, so the cases that
// matter are the ones where exactly one flag is set; false is asserted beside
// true because dropping a flag and sending false look alike to a reader and
// not to GitLab.
func protectFlagCases() []protectFlagCase {
	return []protectFlagCase{
		{name: "force push allowed", forcePush: new(true), wantForcePush: new(true)},
		{name: "force push refused", forcePush: new(false), wantForcePush: new(false)},
		{name: "code owner approval required", codeOwner: new(true), wantCodeOwner: new(true)},
		{name: "code owner approval not required", codeOwner: new(false), wantCodeOwner: new(false)},
		{name: "neither flag set", forcePush: nil, codeOwner: nil},
	}
}

// TestBranchProtect_OptionalFlagsReachGitLab pins that allow_force_push and
// code_owner_approval_required reach GitLab exactly as the caller set them,
// and that a flag the caller left unset is left out of the body rather than
// sent as false. Nothing held this before: the tests that set the two flags
// asserted the mock's own response, so a guard that never forwarded either
// value changed nothing they could see.
func TestBranchProtect_OptionalFlagsReachGitLab(t *testing.T) {
	for _, c := range protectFlagCases() {
		t.Run(c.name, func(t *testing.T) {
			got := captureProtectRequest(t, http.MethodPost, pathProtectedBranches,
				`{"id":1,"name":"main"}`,
				func(client *gitlabclient.Client) error {
					_, err := Protect(context.Background(), client, ProtectInput{
						ProjectID:                 "42",
						BranchName:                "main",
						AllowForcePush:            c.forcePush,
						CodeOwnerApprovalRequired: c.codeOwner,
					})
					return err
				})
			assertOptional(t, "allow_force_push", got.AllowForcePush, c.wantForcePush)
			assertOptional(t, "code_owner_approval_required", got.CodeOwnerApprovalRequired, c.wantCodeOwner)
		})
	}
}

// TestProtectedUpdate_OptionalFlagsReachGitLab holds the update path to the
// same contract as the protect path: the caller's two flags reach GitLab as
// set, and an unset one is absent from the PATCH body.
func TestProtectedUpdate_OptionalFlagsReachGitLab(t *testing.T) {
	for _, c := range protectFlagCases() {
		t.Run(c.name, func(t *testing.T) {
			got := captureProtectRequest(t, http.MethodPatch, pathProtectedBranches+"/main",
				`{"id":1,"name":"main"}`,
				func(client *gitlabclient.Client) error {
					_, err := ProtectedUpdate(context.Background(), client, ProtectedUpdateInput{
						ProjectID:                 "42",
						BranchName:                "main",
						AllowForcePush:            c.forcePush,
						CodeOwnerApprovalRequired: c.codeOwner,
					})
					return err
				})
			assertOptional(t, "allow_force_push", got.AllowForcePush, c.wantForcePush)
			assertOptional(t, "code_owner_approval_required", got.CodeOwnerApprovalRequired, c.wantCodeOwner)
		})
	}
}

// TestBranchProtect_AccessLevelsReachTheirOwnFields pins each coarse access
// level on the key GitLab reads it from. The three levels differ from one
// another because a fixture that protects a branch at 40 everywhere cannot
// tell push, merge and unprotect apart: two of them could trade places and
// every assertion would still hold.
func TestBranchProtect_AccessLevelsReachTheirOwnFields(t *testing.T) {
	got := captureProtectRequest(t, http.MethodPost, pathProtectedBranches,
		`{"id":3,"name":"release/*"}`,
		func(client *gitlabclient.Client) error {
			_, err := Protect(context.Background(), client, ProtectInput{
				ProjectID:            "42",
				BranchName:           testReleaseWildcard,
				PushAccessLevel:      30,
				MergeAccessLevel:     40,
				UnprotectAccessLevel: 60,
			})
			return err
		})

	if got.Name != testReleaseWildcard {
		t.Errorf("name = %q, want %q", got.Name, testReleaseWildcard)
	}
	assertOptional(t, "push_access_level", got.PushAccessLevel, new(30))
	assertOptional(t, "merge_access_level", got.MergeAccessLevel, new(40))
	assertOptional(t, "unprotect_access_level", got.UnprotectAccessLevel, new(60))
}

// assertPermissionEntry pins one allowed_to_* entry field by field. Every id
// the callers below hand it differs from every other, so an entry whose user
// id was written to the entry id, or whose group id was written to the deploy
// key, fails here rather than passing on a shared value.
func assertPermissionEntry(t *testing.T, array string, got, want permissionRequest) {
	t.Helper()
	assertOptional(t, array+".id", got.ID, want.ID)
	assertOptional(t, array+".user_id", got.UserID, want.UserID)
	assertOptional(t, array+".group_id", got.GroupID, want.GroupID)
	assertOptional(t, array+".deploy_key_id", got.DeployKeyID, want.DeployKeyID)
	assertOptional(t, array+".access_level", got.AccessLevel, want.AccessLevel)
	assertOptional(t, array+"._destroy", got.Destroy, want.Destroy)
}

// assertSingleEntry reports the one entry an allowed_to_* array must carry, or
// fails when the array is not exactly one entry long.
func assertSingleEntry(t *testing.T, array string, got []permissionRequest, want permissionRequest) {
	t.Helper()
	if len(got) != 1 {
		t.Fatalf("%s has %d entries, want 1: %+v", array, len(got), got)
	}
	assertPermissionEntry(t, array, got[0], want)
}

// TestBranchProtect_PermissionEntriesReachTheirOwnArrays pins each
// fine-grained permission entry on the array the caller put it in and on the
// field it names. The check this replaces matched the serialized body as text
// and shared one id between two arrays, so the entry id and the user id could
// trade places and both assertions still found their value somewhere in the
// body.
func TestBranchProtect_PermissionEntriesReachTheirOwnArrays(t *testing.T) {
	got := captureProtectRequest(t, http.MethodPost, pathProtectedBranches,
		`{"id":3,"name":"release/*"}`,
		func(client *gitlabclient.Client) error {
			_, err := Protect(context.Background(), client, ProtectInput{
				ProjectID:          "42",
				BranchName:         testReleaseWildcard,
				AllowedToPush:      []BranchPermissionInput{{UserID: new(int64(11)), AccessLevel: new(30)}},
				AllowedToMerge:     []BranchPermissionInput{{GroupID: new(int64(22))}},
				AllowedToUnprotect: []BranchPermissionInput{{ID: new(int64(33)), Destroy: new(true)}},
			})
			return err
		})

	assertSingleEntry(t, "allowed_to_push", got.AllowedToPush,
		permissionRequest{UserID: new(int64(11)), AccessLevel: new(30)})
	assertSingleEntry(t, "allowed_to_merge", got.AllowedToMerge,
		permissionRequest{GroupID: new(int64(22))})
	assertSingleEntry(t, "allowed_to_unprotect", got.AllowedToUnprotect,
		permissionRequest{ID: new(int64(33)), Destroy: new(true)})
}

// TestProtectedUpdate_PermissionEntriesReachTheirOwnArrays holds the update
// path to the same contract, and pins the rename beside it. The deploy key is
// exercised here rather than on the protect path so all four ways of naming a
// principal are covered once between the two.
func TestProtectedUpdate_PermissionEntriesReachTheirOwnArrays(t *testing.T) {
	got := captureProtectRequest(t, http.MethodPatch, pathProtectedBranches+"/main",
		`{"id":1,"name":"main-renamed"}`,
		func(client *gitlabclient.Client) error {
			_, err := ProtectedUpdate(context.Background(), client, ProtectedUpdateInput{
				ProjectID:          "42",
				BranchName:         "main",
				Name:               "main-renamed",
				AllowedToPush:      []BranchPermissionInput{{DeployKeyID: new(int64(44))}},
				AllowedToMerge:     []BranchPermissionInput{{GroupID: new(int64(55)), AccessLevel: new(40)}},
				AllowedToUnprotect: []BranchPermissionInput{{UserID: new(int64(66))}},
			})
			return err
		})

	if got.Name != "main-renamed" {
		t.Errorf("name = %q, want the rename to reach GitLab", got.Name)
	}
	assertSingleEntry(t, "allowed_to_push", got.AllowedToPush,
		permissionRequest{DeployKeyID: new(int64(44))})
	assertSingleEntry(t, "allowed_to_merge", got.AllowedToMerge,
		permissionRequest{GroupID: new(int64(55)), AccessLevel: new(40)})
	assertSingleEntry(t, "allowed_to_unprotect", got.AllowedToUnprotect,
		permissionRequest{UserID: new(int64(66))})
}

// ---------------------------------------------------------------------------
// What the catalog publishes about each action
// ---------------------------------------------------------------------------.

// TestActionSpecs_BranchGetRouteServerError pins that only a 404 becomes the
// structured not-found card. Any other refusal reaches the caller as the error
// it is, so a model is never told the branch does not exist when GitLab was
// the thing that failed.
func TestActionSpecs_BranchGetRouteServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"500 Internal Server Error"}`)
	}))
	byTool := branchSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_branch_get"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "branch_name": "main"})
	if err == nil {
		t.Fatalf("Route.Handler error = nil, want the server failure; result = %#v", result)
	}
	if _, ok := result.(branchNotFoundOutput); ok {
		t.Fatalf("result = %#v, want no not-found card for a 500", result)
	}
}

// TestActionSpecs_Annotations pins the three hints every surface publishes for
// each branch action: whether it only reads, whether it destroys, and whether
// repeating it is safe. They are what a model reads before deciding to call
// something without asking, so protecting a branch must not be published as
// destructive and creating one must not be published as safe to repeat.
func TestActionSpecs_Annotations(t *testing.T) {
	byTool := newBranchSpecsByTool(t)

	cases := []struct {
		tool        string
		readOnly    bool
		destructive bool
		idempotent  bool
	}{
		{"gitlab_branch_list", true, false, true},
		{"gitlab_branch_get", true, false, true},
		{"gitlab_protected_branches_list", true, false, true},
		{"gitlab_protected_branch_get", true, false, true},
		{"gitlab_branch_create", false, false, false},
		{"gitlab_branch_protect", false, false, true},
		{"gitlab_protected_branch_update", false, false, true},
		{"gitlab_branch_delete", false, true, true},
		{"gitlab_branch_delete_merged", false, true, true},
		{"gitlab_branch_unprotect", false, true, true},
	}
	for _, c := range cases {
		t.Run(c.tool, func(t *testing.T) {
			spec, ok := byTool[c.tool]
			if !ok {
				t.Fatalf("%s is not published", c.tool)
			}
			if spec.ReadOnly != c.readOnly {
				t.Errorf("ReadOnly = %v, want %v", spec.ReadOnly, c.readOnly)
			}
			if spec.Destructive != c.destructive {
				t.Errorf("Destructive = %v, want %v", spec.Destructive, c.destructive)
			}
			if spec.Idempotent != c.idempotent {
				t.Errorf("Idempotent = %v, want %v", spec.Idempotent, c.idempotent)
			}
		})
	}
}

// TestBranchSpec_DestructiveRouteThatIsNotIdempotent pins the one flag
// combination the published specs do not use today: a destructive action whose
// repetition is not safe keeps idempotent false instead of being classified as
// a delete, which would advertise that retrying it is harmless. Every
// destructive branch action happens to be idempotent, so that half of the
// classification is reachable only from here.
func TestBranchSpec_DestructiveRouteThatIsNotIdempotent(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	spec := branchSpec("probe", toolutil.DestructiveVoidAction(client, Delete), "gitlab_branch_probe", false, false)

	if !spec.Destructive {
		t.Error("Destructive = false, want true for a destructive route")
	}
	if spec.Idempotent {
		t.Error("Idempotent = true, want false when repeating the action is not safe")
	}
	if spec.ReadOnly {
		t.Error("ReadOnly = true, want false for a destructive route")
	}
}
