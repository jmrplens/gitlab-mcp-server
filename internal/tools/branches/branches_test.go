// branches_test.go contains unit tests for GitLab branch operations
// (create, list, get, delete, protect, unprotect, update, and list
// protected branches). Tests use httptest to mock the GitLab API and
// verify success, error, canceled-context, and markdown-formatter paths.

package branches

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
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

// TestBranchUnprotect_NotFound verifies that branchUnprotect returns an error
// when the target branch does not exist. The mock returns HTTP 404.
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
	if out.CommitID != "abc123def456" {
		t.Errorf("out.CommitID = %q, want %q", out.CommitID, "abc123def456")
	}
}

// TestBranchCreate_AlreadyExists verifies that branchCreate returns an error
// when the GitLab API reports the branch already exists (HTTP 400).
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

// TestBranchCreateRef_NotFound verifies that branchCreate returns an
// actionable error message when the source ref does not exist.
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

// TestBranchList_WithSearch verifies that branchList passes the search query
// parameter to the GitLab API and returns only matching branches.
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

// TestBranchList_Empty verifies that branchList handles an empty API response
// gracefully, returning zero branches without error.
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

// TestBranchGet_Success verifies that branchGet retrieves a single branch by name.
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

// TestBranchGet_EmptyProjectID verifies branchGet returns an error for empty project_id.
func TestBranchGet_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := Get(context.Background(), client, GetInput{BranchName: "main"})
	if err == nil {
		t.Fatal("expected error for empty project_id, got nil")
	}
}

// TestBranchDelete_Success verifies that branchDelete removes a branch.
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

// TestBranchDelete_EmptyProjectID verifies branchDelete returns an error for empty project_id.
func TestBranchDelete_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	err := Delete(context.Background(), client, DeleteInput{BranchName: "main"})
	if err == nil {
		t.Fatal("expected error for empty project_id, got nil")
	}
}

// TestBranchDelete_APIError verifies branchDelete returns an error on API failure.
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

// TestProtectedBranchGet_Success verifies the behavior of protected branch get success.
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
	if out.PushAccessLevel != 0 {
		t.Errorf("PushAccessLevel = %d, want 0", out.PushAccessLevel)
	}
	if out.MergeAccessLevel != 40 {
		t.Errorf("MergeAccessLevel = %d, want 40", out.MergeAccessLevel)
	}
	if !out.CodeOwnerApprovalRequired {
		t.Error("CodeOwnerApprovalRequired = false, want true")
	}
}

// TestProtectedBranchGet_MissingProjectID verifies the behavior of protected branch get missing project i d.
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

// TestProtectedBranchGet_MissingBranchName verifies the behavior of protected branch get missing branch name.
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

// TestProtectedBranchGet_CancelledContext verifies the behavior of protected branch get cancelled context.
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

// TestProtectedBranchUpdate_Success verifies the behavior of protected branch update success.
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

// TestProtectedBranchUpdate_MissingProjectID verifies the behavior of protected branch update missing project i d.
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

// TestProtectedBranchUpdate_MissingBranchName verifies the behavior of protected branch update missing branch name.
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

// TestProtectedBranchUpdate_CancelledContext verifies the behavior of protected branch update cancelled context.
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

// TestDeleteMerged_Success verifies the behavior of delete merged success.
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

// TestDeleteMerged_MissingProjectID verifies the behavior of delete merged missing project i d.
func TestDeleteMerged_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := DeleteMerged(context.Background(), client, DeleteMergedInput{ProjectID: ""})
	if err == nil {
		t.Fatal(errExpMissingProjectID)
	}
}

// TestDeleteMerged_APIError verifies the behavior of delete merged a p i error.
func TestDeleteMerged_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	err := DeleteMerged(context.Background(), client, DeleteMergedInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestDeleteMerged_CancelledContext verifies the behavior of delete merged cancelled context.
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

// TestBranchCreate_CancelledContext verifies the behavior of branch create cancelled context.
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

// TestBranchList_CancelledContext verifies the behavior of branch list cancelled context.
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

// TestBranchGet_CancelledContext verifies the behavior of branch get cancelled context.
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

// TestBranchDelete_CancelledContext verifies the behavior of branch delete cancelled context.
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

// TestBranchProtect_CancelledContext verifies the behavior of branch protect cancelled context.
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

// TestBranchUnprotect_CancelledContext verifies the behavior of branch unprotect cancelled context.
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

// TestProtectedList_CancelledContext verifies the behavior of protected list cancelled context.
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

// TestBranchCreate_EmptyProjectID verifies the behavior of branch create empty project i d.
func TestBranchCreate_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{BranchName: "x", Ref: "main"})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// TestBranchList_EmptyProjectID verifies the behavior of branch list empty project i d.
func TestBranchList_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// TestBranchProtect_EmptyProjectID verifies the behavior of branch protect empty project i d.
func TestBranchProtect_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{}`)
	}))
	_, err := Protect(context.Background(), client, ProtectInput{BranchName: "main"})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// TestBranchUnprotect_EmptyProjectID verifies the behavior of branch unprotect empty project i d.
func TestBranchUnprotect_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	_, err := Unprotect(context.Background(), client, UnprotectInput{BranchName: "main"})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// TestProtectedList_EmptyProjectID verifies the behavior of protected list empty project i d.
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

// TestBranchProtect_APIError verifies the behavior of branch protect a p i error.
func TestBranchProtect_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := Protect(context.Background(), client, ProtectInput{ProjectID: "42", BranchName: "main"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestBranchList_APIError verifies the behavior of branch list a p i error.
func TestBranchList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestBranchGet_APIError verifies the behavior of branch get a p i error.
func TestBranchGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Branch Not Found"}`)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: "42", BranchName: "nonexistent"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestProtectedList_APIError verifies the behavior of protected list a p i error.
func TestProtectedList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := ProtectedList(context.Background(), client, ProtectedListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestProtectedBranchGet_APIError verifies the behavior of protected branch get a p i error.
func TestProtectedBranchGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))
	_, err := ProtectedGet(context.Background(), client, ProtectedGetInput{ProjectID: "42", BranchName: "nonexistent"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestProtectedBranchUpdate_APIError verifies the behavior of protected branch update a p i error.
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

// TestBranchUnprotect_APIError verifies the behavior of branch unprotect a p i error.
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

// TestBranchProtect_WithForcePushAndCodeOwner verifies the behavior of branch protect with force push and code owner.
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

// TestProtectedBranchUpdate_WithCodeOwner verifies the behavior of protected branch update with code owner.
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

// TestToOutput_NilCommit verifies the behavior of to output nil commit.
func TestToOutput_NilCommit(t *testing.T) {
	b := &gl.Branch{Name: "main", Protected: true}
	out := ToOutput(b)
	if out.CommitID != "" {
		t.Errorf("out.CommitID = %q, want empty for nil commit", out.CommitID)
	}
}

// TestProtectedToOutput_EmptyAccessLevels verifies the behavior of protected to output empty access levels.
func TestProtectedToOutput_EmptyAccessLevels(t *testing.T) {
	pb := &gl.ProtectedBranch{ID: 1, Name: "main"}
	out := ProtectedToOutput(pb)
	if out.PushAccessLevel != 0 {
		t.Errorf("PushAccessLevel = %d, want 0 for empty access levels", out.PushAccessLevel)
	}
	if out.MergeAccessLevel != 0 {
		t.Errorf("MergeAccessLevel = %d, want 0 for empty access levels", out.MergeAccessLevel)
	}
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdown verifies the behavior of format output markdown.
func TestFormatOutputMarkdown(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		Name:      "main",
		Protected: true,
		Default:   true,
		Merged:    false,
		CommitID:  "abc123",
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

// TestFormatOutputMarkdown_NoURL verifies the behavior of format output markdown no u r l.
func TestFormatOutputMarkdown_NoURL(t *testing.T) {
	md := FormatOutputMarkdown(Output{Name: "dev"})
	if !strings.Contains(md, "## Branch: dev") {
		t.Error("expected heading with branch name")
	}
	if strings.Contains(md, "URL") {
		t.Error("should not contain URL when empty")
	}
}

// TestFormatListMarkdown verifies the behavior of format list markdown.
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

// TestFormatListMarkdown_ClickableBranchLinks verifies that branch names
// appear as clickable Markdown links when WebURL is present.
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

// TestFormatListMarkdown_NoLinkWithoutWebURL verifies that branch names
// appear as plain text when WebURL is empty.
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

// TestFormatListMarkdown_Empty verifies the behavior of format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No branches found") {
		t.Error("expected 'No branches found' message")
	}
}

// TestFormatProtectedMarkdown verifies the behavior of format protected markdown.
func TestFormatProtectedMarkdown(t *testing.T) {
	md := FormatProtectedMarkdown(ProtectedOutput{
		ID:               1,
		Name:             "main",
		PushAccessLevel:  0,
		MergeAccessLevel: 40,
		AllowForcePush:   false,
	})
	if !strings.Contains(md, "## Protected Branch: main") {
		t.Error("expected heading with protected branch name")
	}
	if !strings.Contains(md, "Push Access Level") {
		t.Error("expected push access level")
	}
}

// TestFormatProtectedListMarkdown verifies the behavior of format protected list markdown.
func TestFormatProtectedListMarkdown(t *testing.T) {
	md := FormatProtectedListMarkdown(ProtectedListOutput{
		Branches: []ProtectedOutput{
			{ID: 1, Name: "main", PushAccessLevel: 0, MergeAccessLevel: 40},
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

// TestFormatProtectedListMarkdown_Empty verifies the behavior of format protected list markdown empty.
func TestFormatProtectedListMarkdown_Empty(t *testing.T) {
	md := FormatProtectedListMarkdown(ProtectedListOutput{})
	if !strings.Contains(md, "No protected branches found") {
		t.Error("expected 'No protected branches found' message")
	}
}

// ---------------------------------------------------------------------------
// List with pagination params
// ---------------------------------------------------------------------------.

// TestBranchList_PaginationQueryParams verifies the behavior of branch list pagination query params.
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
// RegisterTools + CallAllThroughMCP
// ---------------------------------------------------------------------------.

// branchMockResp holds a canned response for a mock branch endpoint.
type branchMockResp struct {
	status int
	body   string
	pgHdr  *testutil.PaginationHeaders
}

// newBranchMCPSession is an internal helper for the branches package.
func newBranchMCPSession(t *testing.T) *mcp.ClientSession {
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

// requireToolSuccess calls the named MCP tool and fails the test if the
// call returns an error or an IsError result with embedded text.
func requireToolSuccess(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) {
	t.Helper()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool(%s) error: %v", name, err)
	}
	if !result.IsError {
		return
	}
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			t.Fatalf("CallTool(%s) returned error: %s", name, tc.Text)
		}
	}
	t.Fatalf("CallTool(%s) returned IsError=true", name)
}

// ---------------------------------------------------------------------------
// Protection level combination edge cases
// ---------------------------------------------------------------------------.

// TestBranchProtect_AccessLevels_Developer_Maintainer verifies protection with
// push=30 (Developer) and merge=40 (Maintainer).
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

// TestBranchProtect_AccessLevels_Maintainer_Maintainer verifies protection with
// both push and merge at Maintainer level (40).
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

// TestBranchProtect_CodeOwner_WithAccessLevels verifies that code owner approval
// can be combined with non-default access levels.
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

// TestBranchProtect_ForcePush_WithRestrictiveAccess verifies that force push
// can be enabled even with restrictive (Maintainer) access levels.
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

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newBranchMCPSession(t)

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
			requireToolSuccess(t, session, tt.name, tt.args)
		})
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
	if !out.AlreadyProtected {
		t.Error("expected AlreadyProtected = true")
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

// TestBranchDelete_ProtectedBranch verifies the hint when deleting a protected branch.
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

// TestBranchDelete_NotFound verifies the hint when deleting a nonexistent branch.
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

// TestBranchCreate_GenericAPIError verifies that Create wraps generic server errors.
func TestBranchCreate_GenericAPIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: "42", BranchName: "x", Ref: "main"})
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

// TestBranchCreate_EmptyBranchName verifies validation for empty branch name.
func TestBranchCreate_EmptyBranchName(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := Create(context.Background(), client, CreateInput{ProjectID: "42", Ref: "main"})
	if err == nil {
		t.Fatal("expected error for empty branch_name")
	}
}

// TestBranchGet_EmptyBranchName verifies validation for empty branch name.
func TestBranchGet_EmptyBranchName(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := Get(context.Background(), client, GetInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for empty branch_name")
	}
}

// TestBranchDelete_EmptyBranchName verifies validation for empty branch name.
func TestBranchDelete_EmptyBranchName(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := Delete(context.Background(), client, DeleteInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for empty branch_name")
	}
}

// TestBranchProtect_EmptyBranchName verifies validation for empty branch name.
func TestBranchProtect_EmptyBranchName(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := Protect(context.Background(), client, ProtectInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for empty branch_name")
	}
}

// TestBranchUnprotect_EmptyBranchName verifies validation for empty branch name.
func TestBranchUnprotect_EmptyBranchName(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := Unprotect(context.Background(), client, UnprotectInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for empty branch_name")
	}
}
