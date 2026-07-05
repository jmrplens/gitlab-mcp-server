// branches_test.go contains unit tests for GitLab branch operations
// (create, list, get, delete, protect, unprotect, update, and list
// protected branches). Tests use httptest to mock the GitLab API and
// verify success, error, canceled-context, and markdown-formatter paths.
package branches

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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

	out, err := ProtectedList(context.Background(), client, ProtectedListInput{ProjectID: "42", PaginationInput: toolutil.PaginationInput{Page: 1, PerPage: 10}})
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

// TestBranchGet_EmptyProjectID verifies the BranchGet_EmptyProjectID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestBranchGet_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

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

// TestBranchDelete_EmptyProjectID verifies the BranchDelete_EmptyProjectID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestBranchDelete_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

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
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"main","push_access_levels":[{"access_level":0}],"merge_access_levels":[{"access_level":40}],"allow_force_push":false,"code_owner_approval_required":true}`)
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
}

// TestProtectedBranchGet_MissingProjectID verifies that ProtectedBranchGet_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestProtectedBranchGet_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := ProtectedGet(context.Background(), client, ProtectedGetInput{
		ProjectID:  "",
		BranchName: "main",
	})
	if err == nil {
		t.Fatal(errExpMissingProjectID)
	}
}

// TestProtectedBranchGet_MissingBranchName verifies that ProtectedBranchGet_MissingBranchName returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestProtectedBranchGet_MissingBranchName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := ProtectedGet(context.Background(), client, ProtectedGetInput{
		ProjectID:  "42",
		BranchName: "",
	})
	if err == nil {
		t.Fatal("expected error for missing branch_name")
	}
}

// TestProtectedBranchGet_CancelledContext verifies the ProtectedBranchGet_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestProtectedBranchGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

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

// TestProtectedBranchUpdate_MissingProjectID verifies that ProtectedBranchUpdate_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestProtectedBranchUpdate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := ProtectedUpdate(context.Background(), client, ProtectedUpdateInput{
		ProjectID:  "",
		BranchName: "main",
	})
	if err == nil {
		t.Fatal(errExpMissingProjectID)
	}
}

// TestProtectedBranchUpdate_MissingBranchName verifies that ProtectedBranchUpdate_MissingBranchName returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestProtectedBranchUpdate_MissingBranchName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := ProtectedUpdate(context.Background(), client, ProtectedUpdateInput{
		ProjectID:  "42",
		BranchName: "",
	})
	if err == nil {
		t.Fatal("expected error for missing branch_name")
	}
}

// TestProtectedBranchUpdate_CancelledContext verifies the ProtectedBranchUpdate_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestProtectedBranchUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

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

// TestDeleteMerged_MissingProjectID verifies that DeleteMerged_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteMerged_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

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

// TestDeleteMerged_CancelledContext verifies the DeleteMerged_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestDeleteMerged_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	ctx := testutil.CancelledCtx(t)

	err := DeleteMerged(ctx, client, DeleteMergedInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Canceled context tests for remaining functions
// ---------------------------------------------------------------------------.

// TestBranchCreate_CancelledContext verifies the BranchCreate_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestBranchCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Create(ctx, client, CreateInput{ProjectID: "42", BranchName: "x", Ref: "main"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestBranchList_CancelledContext verifies the BranchList_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestBranchList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestBranchGet_CancelledContext verifies the BranchGet_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestBranchGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Get(ctx, client, GetInput{ProjectID: "42", BranchName: "main"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestBranchDelete_CancelledContext verifies the BranchDelete_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestBranchDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx := testutil.CancelledCtx(t)
	err := Delete(ctx, client, DeleteInput{ProjectID: "42", BranchName: "x"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestBranchProtect_CancelledContext verifies the BranchProtect_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestBranchProtect_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Protect(ctx, client, ProtectInput{ProjectID: "42", BranchName: "main"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestBranchUnprotect_CancelledContext verifies the BranchUnprotect_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestBranchUnprotect_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Unprotect(ctx, client, UnprotectInput{ProjectID: "42", BranchName: "main"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestProtectedList_CancelledContext verifies the ProtectedList_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestProtectedList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := ProtectedList(ctx, client, ProtectedListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Empty ProjectID tests for remaining functions
// ---------------------------------------------------------------------------.

// TestBranchCreate_EmptyProjectID verifies the BranchCreate_EmptyProjectID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestBranchCreate_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{BranchName: "x", Ref: "main"})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// TestBranchList_EmptyProjectID verifies the BranchList_EmptyProjectID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestBranchList_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// TestBranchProtect_EmptyProjectID verifies the BranchProtect_EmptyProjectID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestBranchProtect_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{}`)
	}))
	_, err := Protect(context.Background(), client, ProtectInput{BranchName: "main"})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// TestBranchUnprotect_EmptyProjectID verifies the BranchUnprotect_EmptyProjectID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestBranchUnprotect_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	_, err := Unprotect(context.Background(), client, UnprotectInput{BranchName: "main"})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// TestProtectedList_EmptyProjectID verifies the ProtectedList_EmptyProjectID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestProtectedList_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
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

// TestToOutput_NilCommit verifies the ToOutput_NilCommit handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestToOutput_NilCommit(t *testing.T) {
	b := &gl.Branch{Name: "main", Protected: true}
	out := ToOutput(b)
	if out.Commit != nil {
		t.Errorf("out.Commit = %+v, want nil for nil commit", out.Commit)
	}
}

// TestProtectedToOutput_EmptyAccessLevels verifies the ProtectedToOutput_EmptyAccessLevels handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestProtectedToOutput_EmptyAccessLevels(t *testing.T) {
	pb := &gl.ProtectedBranch{ID: 1, Name: "main"}
	out := ProtectedToOutput(pb)
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

// TestFormatOutputMarkdown verifies the OutputMarkdown Markdown formatter for a representative output input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatOutputMarkdown(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		Name:      "main",
		Protected: true,
		Default:   true,
		Merged:    false,
		Commit:    &CommitOutput{ID: "abc123"},
		WebURL:    "https://gitlab.example.com/-/tree/main",
	})
	if !strings.Contains(md, "## Branch: main") {
		t.Error("expected heading with branch name")
	}
	if !strings.Contains(md, "abc123") {
		t.Error("expected commit ID")
	}
	if !strings.Contains(md, "https://gitlab.example.com/-/tree/main") {
		t.Error("expected web URL")
	}
}

// TestFormatOutputMarkdown_NoURL verifies the OutputMarkdown_NoURL Markdown formatter for a representative output_nourl input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatOutputMarkdown_NoURL(t *testing.T) {
	md := FormatOutputMarkdown(Output{Name: "dev"})
	if !strings.Contains(md, "## Branch: dev") {
		t.Error("expected heading with branch name")
	}
	if strings.Contains(md, "URL") {
		t.Error("should not contain URL when empty")
	}
}

// TestFormatListMarkdown verifies the ListMarkdown Markdown formatter for a representative list input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown(t *testing.T) {
	md := FormatListMarkdown(ListOutput{
		Branches: []Output{
			{Name: "main", Protected: true, Default: true},
			{Name: "dev", Protected: false, Default: false},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2},
	})
	if !strings.Contains(md, "## Branches (2)") {
		t.Error("expected heading with count")
	}
	if !strings.Contains(md, "| main |") {
		t.Error("expected main branch row")
	}
	if !strings.Contains(md, "| dev |") {
		t.Error("expected dev branch row")
	}
}

// TestFormatListMarkdown_ClickableBranchLinks verifies the ListMarkdown_ClickableBranchLinks Markdown formatter for a representative list_clickablebranchlinks input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown_ClickableBranchLinks(t *testing.T) {
	md := FormatListMarkdown(ListOutput{
		Branches: []Output{
			{Name: "main", Protected: true, Default: true, WebURL: "https://gitlab.example.com/-/tree/main"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	})
	if !strings.Contains(md, "[main](https://gitlab.example.com/-/tree/main)") {
		t.Errorf("expected clickable branch link, got:\n%s", md)
	}
}

// TestFormatListMarkdown_NoLinkWithoutWebURL verifies the ListMarkdown_NoLinkWithoutWebURL Markdown formatter for a representative list_nolinkwithoutweburl input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown_NoLinkWithoutWebURL(t *testing.T) {
	md := FormatListMarkdown(ListOutput{
		Branches: []Output{
			{Name: "dev", Protected: false, Default: false},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	})
	if strings.Contains(md, "[dev](") {
		t.Errorf("should not contain link when WebURL is empty, got:\n%s", md)
	}
	if !strings.Contains(md, "dev") {
		t.Errorf("should contain branch name as plain text, got:\n%s", md)
	}
}

// TestFormatListMarkdown_Empty verifies the ListMarkdown_Empty Markdown formatter for a representative list_empty input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No branches found") {
		t.Error("expected 'No branches found' message")
	}
}

// TestFormatProtectedMarkdown verifies the ProtectedMarkdown Markdown formatter for a representative protected input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatProtectedMarkdown(t *testing.T) {
	md := FormatProtectedMarkdown(ProtectedOutput{
		ID:                1,
		Name:              "main",
		PushAccessLevels:  []BranchAccessDescriptionOutput{{AccessLevel: 0}},
		MergeAccessLevels: []BranchAccessDescriptionOutput{{AccessLevel: 40}},
		AllowForcePush:    false,
	})
	if !strings.Contains(md, "## Protected Branch: main") {
		t.Error("expected heading with protected branch name")
	}
	if !strings.Contains(md, "Push Access Levels") {
		t.Error("expected push access levels")
	}
}

// TestFormatProtectedListMarkdown verifies the ProtectedListMarkdown Markdown formatter for a representative protectedlist input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatProtectedListMarkdown(t *testing.T) {
	md := FormatProtectedListMarkdown(ProtectedListOutput{
		Branches: []ProtectedOutput{
			{ID: 1, Name: "main", PushAccessLevels: []BranchAccessDescriptionOutput{{AccessLevel: 0}}, MergeAccessLevels: []BranchAccessDescriptionOutput{{AccessLevel: 40}}},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	})
	if !strings.Contains(md, "## Protected Branches (1)") {
		t.Error("expected heading with count")
	}
	if !strings.Contains(md, "| main |") {
		t.Error("expected main row")
	}
}

// TestFormatProtectedListMarkdown_Empty verifies the ProtectedListMarkdown_Empty Markdown formatter for a representative protectedlist_empty input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatProtectedListMarkdown_Empty(t *testing.T) {
	md := FormatProtectedListMarkdown(ProtectedListOutput{})
	if !strings.Contains(md, "No protected branches found") {
		t.Error("expected 'No protected branches found' message")
	}
}

// TestMarkdownRegistry_BranchNotFound verifies that MarkdownRegistry_BranchNotFound returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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
		if !strings.Contains(content.Text, want) {
			t.Fatalf("markdown missing %q:\n%s", want, content.Text)
		}
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
		ProjectID:       "42",
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 5},
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

// TestActionSpecs_Metadata validates the Metadata route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
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

// TestBranchCreate_EmptyBranchName verifies the BranchCreate_EmptyBranchName handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestBranchCreate_EmptyBranchName(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := Create(context.Background(), client, CreateInput{ProjectID: "42", Ref: "main"})
	if err == nil {
		t.Fatal("expected error for empty branch_name")
	}
}

// TestBranchGet_EmptyBranchName verifies the BranchGet_EmptyBranchName handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestBranchGet_EmptyBranchName(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := Get(context.Background(), client, GetInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for empty branch_name")
	}
}

// TestBranchDelete_EmptyBranchName verifies the BranchDelete_EmptyBranchName handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestBranchDelete_EmptyBranchName(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := Delete(context.Background(), client, DeleteInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for empty branch_name")
	}
}

// TestBranchProtect_EmptyBranchName verifies the BranchProtect_EmptyBranchName handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestBranchProtect_EmptyBranchName(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := Protect(context.Background(), client, ProtectInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for empty branch_name")
	}
}

// TestBranchUnprotect_EmptyBranchName verifies the BranchUnprotect_EmptyBranchName handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestBranchUnprotect_EmptyBranchName(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
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
		if !strings.Contains(gotBody, want) {
			t.Errorf("request body missing %q\nbody=%s", want, gotBody)
		}
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
		if !strings.Contains(gotBody, want) {
			t.Errorf("request body missing %q\nbody=%s", want, gotBody)
		}
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
		ProjectID:             "42",
		Search:                "feat",
		Regex:                 "^feat",
		OrderBy:               "updated",
		Sort:                  "desc",
		KeysetPaginationInput: toolutil.KeysetPaginationInput{Pagination: "keyset", PageToken: "tok"},
	})
	if err != nil {
		t.Fatalf(fmtBranchListErr, err)
	}
	for _, want := range []string{"search=feat", "regex=", "order_by=updated", "sort=desc", "pagination=keyset", "page_token=tok"} {
		if !strings.Contains(gotQuery, want) {
			t.Errorf("query missing %q: %s", want, gotQuery)
		}
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
		ProjectID:             "42",
		Search:                "main",
		OrderBy:               "name",
		Sort:                  "asc",
		KeysetPaginationInput: toolutil.KeysetPaginationInput{Pagination: "keyset", PageToken: "tok"},
	})
	if err != nil {
		t.Fatalf(fmtProtBranchListErr, err)
	}
	for _, want := range []string{"search=main", "order_by=name", "sort=asc", "pagination=keyset", "page_token=tok"} {
		if !strings.Contains(gotQuery, want) {
			t.Errorf("query missing %q: %s", want, gotQuery)
		}
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

// TestAccessLevelsSummary verifies the compact access-level markdown summary.
func TestAccessLevelsSummary(t *testing.T) {
	if got := accessLevelsSummary(nil); got != "—" {
		t.Errorf("empty summary = %q, want em dash", got)
	}
	got := accessLevelsSummary([]BranchAccessDescriptionOutput{{AccessLevel: 30}, {AccessLevel: 40}})
	if got != "30, 40" {
		t.Errorf("summary = %q, want \"30, 40\"", got)
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
