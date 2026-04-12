// projects_test.go contains unit tests for GitLab project operations
// (create, get, list, delete, update). Tests use httptest to mock the GitLab
// API and verify both success and error paths including pagination.
package projects

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// Test endpoint paths and fixture values used across project operation tests.
const (
	errExpectedValidation = "expected validation error"
	pathProjects          = "/api/v4/projects"
	testRepoName          = "my-repo"
	pathProject42         = "/api/v4/projects/42"
	testRenamedRepo       = "renamed-repo"
	fmtProjectListErr     = "List() unexpected error: %v"

	fmtUnexpErr           = "unexpected error: %v"
	errEmptyProjID        = "expected error for empty project_id, got nil"
	errExpectedAPI        = "expected API error"
	testHookURL           = "https://example.com/hook"
	errExpectedCtxErr     = "expected context error"
	errExpectedNonEmptyMD = "expected non-empty markdown"
	errEmptyHookID        = "expected error for empty hook_id, got nil"
	errEmptyUserID        = "expected error for empty user_id, got nil"

	pathProject42Hooks = "/api/v4/projects/42/hooks"
	pathProject42Hook1 = "/api/v4/projects/42/hooks/1"
	pathProject42Forks = "/api/v4/projects/42/forks"
	pathProject42Fork  = "/api/v4/projects/42/fork"

	testPrivate         = "private"
	testPublic          = "public"
	testSortAsc         = "asc"
	testBranchDevelop   = "develop"
	testUserJohn        = "john"
	testUserJdoe        = "jdoe"
	testForkA           = "fork-a"
	testMyProject       = "myproject"
	testMyHook          = "my-hook"
	testMyGroup         = "my-group"
	testAccessDeveloper = "Developer"
	testMyFork          = "my-fork"
	testProjectID9999   = "9999"
	testHookName        = "test-hook"
	testCommitRegex     = `^(feat|fix|docs):`
	testAlice           = "Alice"
	testBob             = "Bob"
	testDate20250101    = "2025-01-01"
	testSuccess         = "success"
	testPathNS          = "jmrplens/my-repo"

	errVisibilityNotSet  = "Visibility not set"
	fmtLenProjectsWant1  = "len(Projects) = %d, want 1"
	fmtIDWant1           = "ID = %d, want 1"
	fmtNameWantQ         = "Name = %q, want %q"
	fmtDeleteStatusWantQ = "Delete() status = %q, want %q"

	fmtGetUnexpErr    = "Get() unexpected error: %v"
	fmtDeleteUnexpErr = "Delete() unexpected error: %v"
	fmtLenGroupsWant1 = "len(Groups) = %d, want 1"

	testDate20250410  = "2025-04-10"
	testDate20250601  = "2025-06-01"
	testImportURL     = "https://github.com/example/repo.git"
	testSuggestionMsg = "Apply suggestion"
	testHookURL2      = "https://example.com/hook2"
	testProjectName   = "my-project"
	testDescProject   = "A test project"
	testProjA         = "proj-a"
	testProjB         = "proj-b"
	testPermRemoved   = "Permanently removed"
	mdOpenIssues      = "Open Issues"
	mdHTTPClone       = "HTTP Clone"
	mdSSHClone        = "SSH Clone"

	headerXTotal      = "X-Total"
	headerXTotalPages = "X-Total-Pages"
	headerXPage       = "X-Page"
	headerXPerPage    = "X-Per-Page"
)

// TestProjectCreate_Success verifies that Create creates a project and
// returns the correct ID, name, path, and visibility. The mock returns
// HTTP 201 with a valid project JSON response.
func TestProjectCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProjects {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":42,"name":"my-repo","path_with_namespace":"jmrplens/my-repo","visibility":"private","default_branch":"main","web_url":"https://gitlab.example.com/jmrplens/my-repo","description":""}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		Name:                 testRepoName,
		Visibility:           testPrivate,
		InitializeWithReadme: true,
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.ID != 42 {
		t.Errorf("out.ID = %d, want 42", out.ID)
	}
	if out.Name != testRepoName {
		t.Errorf("out.Name = %q, want %q", out.Name, testRepoName)
	}
	if out.PathWithNamespace != testPathNS {
		t.Errorf("out.PathWithNamespace = %q, want %q", out.PathWithNamespace, testPathNS)
	}
	if out.Visibility != testPrivate {
		t.Errorf("out.Visibility = %q, want %q", out.Visibility, testPrivate)
	}
}

// TestProjectCreateName_Conflict verifies that Create returns an error
// when the GitLab API reports the project name is already taken (HTTP 400).
func TestProjectCreateName_Conflict(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":{"name":["has already been taken"]}}`)
	}))

	_, err := Create(context.Background(), client, CreateInput{Name: "existing-repo"})
	if err == nil {
		t.Fatal("Create() expected error for duplicate name, got nil")
	}
}

// TestProjectCreate_EmptyName verifies that Create returns an error
// when called with an empty project name.
func TestProjectCreate_EmptyName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))

	_, err := Create(context.Background(), client, CreateInput{Name: ""})
	if err == nil {
		t.Fatal("Create() expected error for empty name, got nil")
	}
}

// TestProjectGet_Success verifies that Get retrieves a project by its
// ID and correctly maps all output fields including description.
func TestProjectGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathProject42 {
			testutil.RespondJSON(w, http.StatusOK, `{"id":42,"name":"my-repo","path_with_namespace":"jmrplens/my-repo","visibility":"private","default_branch":"main","web_url":"https://gitlab.example.com/jmrplens/my-repo","description":"A test repo"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtGetUnexpErr, err)
	}
	if out.ID != 42 {
		t.Errorf("out.ID = %d, want 42", out.ID)
	}
	if out.Description != "A test repo" {
		t.Errorf("out.Description = %q, want %q", out.Description, "A test repo")
	}
}

// TestProjectGet_NotFound verifies that Get returns an error when the
// requested project does not exist. The mock returns HTTP 404.
func TestProjectGet_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Project Not Found"}`)
	}))

	_, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID9999})
	if err == nil {
		t.Fatal("Get() expected error for non-existent project, got nil")
	}
}

// TestProjectList_Success verifies that List returns multiple projects
// with correct names from a mocked GitLab API response.
func TestProjectList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathProjects {
			testutil.AssertRequestMethod(t, r, http.MethodGet)
			testutil.AssertQueryParam(t, r, "owned", "true")
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"name":"repo-a","path_with_namespace":"jmrplens/repo-a","visibility":"private","default_branch":"main","web_url":"https://gitlab.example.com/jmrplens/repo-a","description":""},{"id":2,"name":"repo-b","path_with_namespace":"jmrplens/repo-b","visibility":"public","default_branch":"main","web_url":"https://gitlab.example.com/jmrplens/repo-b","description":""}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{Owned: true})
	if err != nil {
		t.Fatalf(fmtProjectListErr, err)
	}
	if len(out.Projects) != 2 {
		t.Errorf("len(out.Projects) = %d, want 2", len(out.Projects))
	}
	if out.Projects[0].Name != "repo-a" {
		t.Errorf("out.Projects[0].Name = %q, want %q", out.Projects[0].Name, "repo-a")
	}
}

// TestProjectList_Empty verifies that List handles an empty API
// response gracefully, returning zero projects without error.
func TestProjectList_Empty(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	out, err := List(context.Background(), client, ListInput{})
	if err != nil {
		t.Fatalf("List() unexpected error for empty list: %v", err)
	}
	if len(out.Projects) != 0 {
		t.Errorf("len(out.Projects) = %d, want 0", len(out.Projects))
	}
}

// TestProjectList_IncludePendingDelete verifies that List sends
// the include_pending_delete query parameter and returns projects with
// marked_for_deletion_on field populated.
func TestProjectList_IncludePendingDelete(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathProjects {
			if got := r.URL.Query().Get("include_pending_delete"); got != "true" {
				t.Errorf("query param include_pending_delete = %q, want %q", got, "true")
			}
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id":1,"name":"active-repo","path_with_namespace":"jmrplens/active-repo","visibility":"private","default_branch":"main","web_url":"https://gitlab.example.com/jmrplens/active-repo"},
				{"id":2,"name":"pending-delete-repo","path_with_namespace":"jmrplens/pending-delete-repo","visibility":"private","default_branch":"main","web_url":"https://gitlab.example.com/jmrplens/pending-delete-repo","marked_for_deletion_on":"2025-04-10"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	includePending := true
	out, err := List(context.Background(), client, ListInput{
		Owned:                true,
		IncludePendingDelete: &includePending,
	})
	if err != nil {
		t.Fatalf(fmtProjectListErr, err)
	}
	if len(out.Projects) != 2 {
		t.Errorf("len(out.Projects) = %d, want 2", len(out.Projects))
	}
	if out.Projects[1].MarkedForDeletionOn != testDate20250410 {
		t.Errorf("out.Projects[1].MarkedForDeletionOn = %q, want %q", out.Projects[1].MarkedForDeletionOn, testDate20250410)
	}
	if out.Projects[0].MarkedForDeletionOn != "" {
		t.Errorf("out.Projects[0].MarkedForDeletionOn = %q, want empty", out.Projects[0].MarkedForDeletionOn)
	}
}

// TestProjectGet_MarkedForDeletion verifies that Get correctly
// returns the marked_for_deletion_on field for projects scheduled for deletion.
func TestProjectGet_MarkedForDeletion(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProject42 {
			testutil.RespondJSON(w, http.StatusOK, `{"id":42,"name":"doomed-repo","path_with_namespace":"jmrplens/doomed-repo","visibility":"private","default_branch":"main","web_url":"https://gitlab.example.com/jmrplens/doomed-repo","marked_for_deletion_on":"2025-04-15"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtGetUnexpErr, err)
	}
	if out.MarkedForDeletionOn != "2025-04-15" {
		t.Errorf("Get() MarkedForDeletionOn = %q, want %q", out.MarkedForDeletionOn, "15 Apr 2025")
	}
}

// TestProjectList_PaginationQueryParamsAndMetadata verifies that List
// forwards page/per_page query parameters and correctly parses all pagination
// metadata (Page, PerPage, TotalItems, TotalPages, NextPage, PrevPage).
func TestProjectList_PaginationQueryParamsAndMetadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathProjects {
			if got := r.URL.Query().Get("page"); got != "2" {
				t.Errorf("query param page = %q, want %q", got, "2")
			}
			if got := r.URL.Query().Get("per_page"); got != "5" {
				t.Errorf("query param per_page = %q, want %q", got, "5")
			}

			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"id":6,"name":"repo-f","path_with_namespace":"user/repo-f","visibility":"private","default_branch":"main","web_url":"https://gitlab.example.com/user/repo-f","description":""}]`,
				testutil.PaginationHeaders{
					Page:       "2",
					PerPage:    "5",
					Total:      "11",
					TotalPages: "3",
					NextPage:   "3",
					PrevPage:   "1",
				})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 5},
	})
	if err != nil {
		t.Fatalf(fmtProjectListErr, err)
	}
	if len(out.Projects) != 1 {
		t.Fatalf("len(out.Projects) = %d, want 1", len(out.Projects))
	}
	assertPaginationFields(t, out.Pagination, toolutil.PaginationOutput{
		Page:       2,
		PerPage:    5,
		TotalItems: 11,
		TotalPages: 3,
		NextPage:   3,
		PrevPage:   1,
	})
}

// assertPaginationFields is a test helper that compares all fields of a
// [toolutil.PaginationOutput] against expected values and reports mismatches.
func assertPaginationFields(t *testing.T, got, want toolutil.PaginationOutput) {
	t.Helper()
	if got.Page != want.Page {
		t.Errorf("Pagination.Page = %d, want %d", got.Page, want.Page)
	}
	if got.PerPage != want.PerPage {
		t.Errorf("Pagination.PerPage = %d, want %d", got.PerPage, want.PerPage)
	}
	if got.TotalItems != want.TotalItems {
		t.Errorf("Pagination.TotalItems = %d, want %d", got.TotalItems, want.TotalItems)
	}
	if got.TotalPages != want.TotalPages {
		t.Errorf("Pagination.TotalPages = %d, want %d", got.TotalPages, want.TotalPages)
	}
	if got.NextPage != want.NextPage {
		t.Errorf("Pagination.NextPage = %d, want %d", got.NextPage, want.NextPage)
	}
	if got.PrevPage != want.PrevPage {
		t.Errorf("Pagination.PrevPage = %d, want %d", got.PrevPage, want.PrevPage)
	}
}

// TestProjectDelete_Success verifies that Delete removes a project
// without error. The mock returns HTTP 202 Accepted and then 404 on subsequent
// GET (project permanently deleted, not delayed).
func TestProjectDelete_Success(t *testing.T) {
	deleted := false
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathProject42 {
			deleted = true
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == pathProject42 && deleted {
			testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Project Not Found"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Delete(context.Background(), client, DeleteInput{ProjectID: "42"})
	if err != nil {
		t.Errorf(fmtDeleteUnexpErr, err)
	}
	if out.Status != testSuccess {
		t.Errorf(fmtDeleteStatusWantQ, out.Status, testSuccess)
	}
	if !out.PermanentlyRemoved {
		t.Errorf("Delete() permanently_removed = false, want true")
	}
}

// TestProjectDelete_DelayedDeletion verifies that Delete correctly
// reports when a project is scheduled for delayed deletion rather than
// being immediately removed.
func TestProjectDelete_DelayedDeletion(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathProject42 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == pathProject42 {
			testutil.RespondJSON(w, http.StatusOK, `{"id":42,"name":"my-repo","path_with_namespace":"jmrplens/my-repo","visibility":"private","default_branch":"main","web_url":"https://gitlab.example.com/jmrplens/my-repo","marked_for_deletion_on":"2025-04-10"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Delete(context.Background(), client, DeleteInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtDeleteUnexpErr, err)
	}
	if out.Status != "scheduled" {
		t.Errorf(fmtDeleteStatusWantQ, out.Status, "scheduled")
	}
	if out.MarkedForDeletionOn != testDate20250410 {
		t.Errorf("Delete() marked_for_deletion_on = %q, want %q", out.MarkedForDeletionOn, testDate20250410)
	}
	if out.PermanentlyRemoved {
		t.Errorf("Delete() permanently_removed = true, want false")
	}
}

// TestProjectDelete_PermanentlyRemove verifies that Delete sends
// the permanently_remove option when requested.
func TestProjectDelete_PermanentlyRemove(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathProject42 {
			if err := r.ParseForm(); err == nil {
				if r.Form.Get("permanently_remove") == "" {
					t.Error("expected permanently_remove parameter to be set")
				}
			}
			w.WriteHeader(http.StatusAccepted)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Delete(context.Background(), client, DeleteInput{
		ProjectID:         "42",
		PermanentlyRemove: true,
		FullPath:          testPathNS,
	})
	if err != nil {
		t.Fatalf(fmtDeleteUnexpErr, err)
	}
	if out.Status != testSuccess {
		t.Errorf(fmtDeleteStatusWantQ, out.Status, testSuccess)
	}
	if !out.PermanentlyRemoved {
		t.Errorf("Delete() permanently_removed = false, want true")
	}
}

// TestProjectDelete_NotFound verifies that Delete returns an error
// when the target project does not exist. The mock returns HTTP 404.
func TestProjectDelete_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Project Not Found"}`)
	}))

	_, err := Delete(context.Background(), client, DeleteInput{ProjectID: testProjectID9999})
	if err == nil {
		t.Fatal("Delete() expected error for non-existent project, got nil")
	}
}

// TestProjectDelete_EmptyID verifies that Delete returns an error
// when project_id is empty.
func TestProjectDelete_EmptyID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not make API call with empty project_id")
	}))

	_, err := Delete(context.Background(), client, DeleteInput{ProjectID: ""})
	if err == nil {
		t.Fatal("Delete() expected error for empty project_id, got nil")
	}
}

// TestProjectDelete_AlreadyMarked verifies that Delete returns a helpful
// message instead of a raw error when the project is already scheduled
// for deletion.
func TestProjectDelete_AlreadyMarked(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"Project has already been marked for deletion"}`)
	}))

	out, err := Delete(context.Background(), client, DeleteInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("Delete() expected no error for already-marked project, got: %v", err)
	}
	if out.Status != "already_scheduled" {
		t.Errorf("Delete() Status = %q, want %q", out.Status, "already_scheduled")
	}
	if !strings.Contains(out.Message, "permanently_remove=true") {
		t.Error("Delete() Message should suggest permanently_remove=true")
	}
	if !strings.Contains(out.Message, "gitlab_project_restore") {
		t.Error("Delete() Message should suggest gitlab_project_restore")
	}
}

// TestProjectRestore_Success verifies that Restore restores a project
// that was marked for deletion.
func TestProjectRestore_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/restore" {
			testutil.RespondJSON(w, http.StatusOK, `{"id":42,"name":"my-repo","path_with_namespace":"jmrplens/my-repo","visibility":"private","default_branch":"main","web_url":"https://gitlab.example.com/jmrplens/my-repo"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Restore(context.Background(), client, RestoreInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("Restore() unexpected error: %v", err)
	}
	if out.ID != 42 {
		t.Errorf("Restore() ID = %d, want 42", out.ID)
	}
	if out.Name != testRepoName {
		t.Errorf("Restore() Name = %q, want %q", out.Name, testRepoName)
	}
}

// TestProjectRestore_NotFound verifies that Restore returns an error
// when the project does not exist.
func TestProjectRestore_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Project Not Found"}`)
	}))

	_, err := Restore(context.Background(), client, RestoreInput{ProjectID: testProjectID9999})
	if err == nil {
		t.Fatal("Restore() expected error for non-existent project, got nil")
	}
}

// TestProjectRestore_EmptyID verifies that Restore returns an error
// when project_id is empty.
func TestProjectRestore_EmptyID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not make API call with empty project_id")
	}))

	_, err := Restore(context.Background(), client, RestoreInput{ProjectID: ""})
	if err == nil {
		t.Fatal("Restore() expected error for empty project_id, got nil")
	}
}

// TestProjectUpdate_Success verifies that Update renames a project and
// changes its visibility. The mock returns the updated project JSON.
func TestProjectUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathProject42 {
			testutil.RespondJSON(w, http.StatusOK, `{"id":42,"name":"renamed-repo","path_with_namespace":"jmrplens/renamed-repo","visibility":"public","default_branch":"main","web_url":"https://gitlab.example.com/jmrplens/renamed-repo","description":"Updated"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID:  "42",
		Name:       testRenamedRepo,
		Visibility: testPublic,
	})
	if err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}
	if out.Visibility != testPublic {
		t.Errorf("out.Visibility = %q, want %q", out.Visibility, testPublic)
	}
	if out.Name != testRenamedRepo {
		t.Errorf("out.Name = %q, want %q", out.Name, testRenamedRepo)
	}
}

// TestProjectCreate_ContextCancelled verifies that Create returns an
// error immediately when the context is already canceled, without making
// an API call.
func TestProjectCreate_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{}`)
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Create(ctx, client, CreateInput{Name: "ignored"})
	if err == nil {
		t.Fatal("Create() expected error for canceled context, got nil")
	}
}

// TestProjectGet_SuccessEnrichedFields verifies that Get maps all enriched
// fields including namespace, topics, timestamps, clone URLs, and counters.
func TestProjectGet_SuccessEnrichedFields(t *testing.T) {
	richJSON := `{
		"id":42,"name":"my-repo","path_with_namespace":"jmrplens/my-repo",
		"visibility":"private","default_branch":"main",
		"web_url":"https://gitlab.example.com/jmrplens/my-repo",
		"description":"A test repo","archived":true,"empty_repo":false,
		"forks_count":5,"star_count":12,"open_issues_count":3,
		"http_url_to_repo":"https://gitlab.example.com/jmrplens/my-repo.git",
		"ssh_url_to_repo":"git@gitlab.example.com:jmrplens/my-repo.git",
		"namespace":{"full_path":"jmrplens"},
		"topics":["go","mcp","gitlab"],
		"created_at":"2026-01-15T10:00:00Z",
		"last_activity_at":"2026-06-01T14:30:00Z"
	}`
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, richJSON)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtGetUnexpErr, err)
	}
	if !out.Archived {
		t.Error("out.Archived = false, want true")
	}
	if out.ForksCount != 5 {
		t.Errorf("out.ForksCount = %d, want 5", out.ForksCount)
	}
	if out.StarCount != 12 {
		t.Errorf("out.StarCount = %d, want 12", out.StarCount)
	}
	if out.OpenIssuesCount != 3 {
		t.Errorf("out.OpenIssuesCount = %d, want 3", out.OpenIssuesCount)
	}
	if out.HTTPURLToRepo != "https://gitlab.example.com/jmrplens/my-repo.git" {
		t.Errorf("out.HTTPURLToRepo = %q, want HTTPS clone URL", out.HTTPURLToRepo)
	}
	if out.SSHURLToRepo != "git@gitlab.example.com:jmrplens/my-repo.git" {
		t.Errorf("out.SSHURLToRepo = %q, want SSH clone URL", out.SSHURLToRepo)
	}
	if out.Namespace != "jmrplens" {
		t.Errorf("out.Namespace = %q, want %q", out.Namespace, "jmrplens")
	}
	if len(out.Topics) != 3 {
		t.Errorf("len(out.Topics) = %d, want 3", len(out.Topics))
	}
	if out.CreatedAt == "" {
		t.Error("out.CreatedAt is empty, want timestamp")
	}
	if out.LastActivityAt == "" {
		t.Error("out.LastActivityAt is empty, want timestamp")
	}
}

// TestProjectListInput_EnrichedFilters verifies new list filters (archived, order_by,
// sort, topic) are forwarded as query parameters.
func TestProjectListInput_EnrichedFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if got := q.Get("archived"); got != "true" {
			t.Errorf("query param archived = %q, want %q", got, "true")
		}
		if got := q.Get("order_by"); got != "name" {
			t.Errorf("query param order_by = %q, want %q", got, "name")
		}
		if got := q.Get("sort"); got != testSortAsc {
			t.Errorf("query param sort = %q, want %q", got, testSortAsc)
		}
		if got := q.Get("topic"); got != "go" {
			t.Errorf("query param topic = %q, want %q", got, "go")
		}
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	archived := true
	_, err := List(context.Background(), client, ListInput{
		Archived: &archived,
		OrderBy:  "name",
		Sort:     testSortAsc,
		Topic:    "go",
	})
	if err != nil {
		t.Fatalf(fmtProjectListErr, err)
	}
}

// TestProjectCreate_EnrichedFeatureOpts verifies that Create passes the
// new feature fields to the API.
func TestProjectCreate_EnrichedFeatureOpts(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProjects {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("failed to decode request body: %v", err)
			}
			assertCreateFeatureBody(t, body)
			testutil.RespondJSON(w, http.StatusCreated, `{"id":99,"name":"feature-proj","path_with_namespace":"ns/feature-proj","visibility":"private","default_branch":"main","web_url":"https://gitlab.example.com/ns/feature-proj","description":""}`)
			return
		}
		http.NotFound(w, r)
	}))

	ciForward := true
	sharedRunners := true
	publicBuilds := false
	packages := true
	_, err := Create(context.Background(), client, CreateInput{
		Name:                         "feature-proj",
		ImportURL:                    testImportURL,
		BuildTimeout:                 3600,
		CIForwardDeploymentEnabled:   &ciForward,
		SharedRunnersEnabled:         &sharedRunners,
		PublicBuilds:                 &publicBuilds,
		PackagesEnabled:              &packages,
		SuggestionCommitMessage:      testSuggestionMsg,
		PagesAccessLevel:             testPrivate,
		ContainerRegistryAccessLevel: "enabled",
		SnippetsAccessLevel:          testPrivate,
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
}

// assertCreateFeatureBody is an internal helper for the projects package.
func assertCreateFeatureBody(t *testing.T, body map[string]any) {
	t.Helper()
	assertCreateFeatureBodyCI(t, body)
	assertCreateFeatureBodyAccess(t, body)
}

// assertCreateFeatureBodyCI is an internal helper for the projects package.
func assertCreateFeatureBodyCI(t *testing.T, body map[string]any) {
	t.Helper()
	if v, ok := body["import_url"].(string); !ok || v != testImportURL {
		t.Errorf("import_url = %v, want %q", body["import_url"], testImportURL)
	}
	if v, ok := body["build_timeout"].(float64); !ok || int64(v) != 3600 {
		t.Errorf("build_timeout = %v, want 3600", body["build_timeout"])
	}
	if v, ok := body["shared_runners_enabled"].(bool); !ok || !v {
		t.Errorf("shared_runners_enabled = %v, want true", body["shared_runners_enabled"])
	}
	if v, ok := body["public_builds"].(bool); !ok || v {
		t.Errorf("public_builds = %v, want false", body["public_builds"])
	}
}

// assertCreateFeatureBodyAccess is an internal helper for the projects package.
func assertCreateFeatureBodyAccess(t *testing.T, body map[string]any) {
	t.Helper()
	if v, ok := body["packages_enabled"].(bool); !ok || !v {
		t.Errorf("packages_enabled = %v, want true", body["packages_enabled"])
	}
	if v, ok := body["suggestion_commit_message"].(string); !ok || v != testSuggestionMsg {
		t.Errorf("suggestion_commit_message = %v, want %q", body["suggestion_commit_message"], testSuggestionMsg)
	}
	if v, ok := body["pages_access_level"].(string); !ok || v != testPrivate {
		t.Errorf("pages_access_level = %v, want %q", body["pages_access_level"], testPrivate)
	}
	if v, ok := body["container_registry_access_level"].(string); !ok || v != "enabled" {
		t.Errorf("container_registry_access_level = %v, want %q", body["container_registry_access_level"], "enabled")
	}
	if v, ok := body["snippets_access_level"].(string); !ok || v != testPrivate {
		t.Errorf("snippets_access_level = %v, want %q", body["snippets_access_level"], testPrivate)
	}
}

// TestProjectList_EnrichedFilterOpts verifies that List passes new filter
// fields (WithProgrammingLanguage, IDAfter, IDBefore) as query parameters.
func TestProjectList_EnrichedFilterOpts(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathProjects {
			q := r.URL.Query()
			if got := q.Get("with_programming_language"); got != "Go" {
				t.Errorf("query param with_programming_language = %q, want %q", got, "Go")
			}
			if got := q.Get("id_after"); got != "100" {
				t.Errorf("query param id_after = %q, want %q", got, "100")
			}
			if got := q.Get("id_before"); got != "500" {
				t.Errorf("query param id_before = %q, want %q", got, "500")
			}
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := List(context.Background(), client, ListInput{
		WithProgrammingLanguage: "Go",
		IDAfter:                 100,
		IDBefore:                500,
	})
	if err != nil {
		t.Fatalf(fmtProjectListErr, err)
	}
}

// TestProjectGet_EnrichedOutputFields verifies that Get maps the enriched
// output fields: ContainerRegistryEnabled, SharedRunnersEnabled, PublicBuilds,
// SnippetsEnabled, PackagesEnabled, BuildTimeout, SuggestionCommitMessage,
// ComplianceFrameworks, ImportURL.
func TestProjectGet_EnrichedOutputFields(t *testing.T) {
	richJSON := `{
		"id":42,"name":"my-repo","path_with_namespace":"jmrplens/my-repo",
		"visibility":"private","default_branch":"main",
		"web_url":"https://gitlab.example.com/jmrplens/my-repo",
		"description":"A test repo",
		"container_registry_enabled":true,
		"container_registry_access_level":"enabled",
		"shared_runners_enabled":true,
		"public_builds":false,
		"snippets_enabled":true,
		"snippets_access_level":"enabled",
		"packages_enabled":true,
		"build_timeout":7200,
		"suggestion_commit_message":"Apply suggestion to %{file_path}",
		"compliance_frameworks":["SOC2","HIPAA"],
		"import_url":"https://github.com/upstream/repo.git"
	}`
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathProject42 {
			testutil.RespondJSON(w, http.StatusOK, richJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtGetUnexpErr, err)
	}
	if !out.ContainerRegistryEnabled {
		t.Error("out.ContainerRegistryEnabled = false, want true")
	}
	if !out.SharedRunnersEnabled {
		t.Error("out.SharedRunnersEnabled = false, want true")
	}
	if out.PublicBuilds {
		t.Error("out.PublicBuilds = true, want false")
	}
	if !out.SnippetsEnabled {
		t.Error("out.SnippetsEnabled = false, want true")
	}
	if !out.PackagesEnabled {
		t.Error("out.PackagesEnabled = false, want true")
	}
	if out.BuildTimeout != 7200 {
		t.Errorf("out.BuildTimeout = %d, want 7200", out.BuildTimeout)
	}
	if out.SuggestionCommitMessage != "Apply suggestion to %{file_path}" {
		t.Errorf("out.SuggestionCommitMessage = %q, want %q", out.SuggestionCommitMessage, "Apply suggestion to %{file_path}")
	}
	if len(out.ComplianceFrameworks) != 2 || out.ComplianceFrameworks[0] != "SOC2" {
		t.Errorf("out.ComplianceFrameworks = %v, want [SOC2 HIPAA]", out.ComplianceFrameworks)
	}
	if out.ImportURL != "https://github.com/upstream/repo.git" {
		t.Errorf("out.ImportURL = %q, want %q", out.ImportURL, "https://github.com/upstream/repo.git")
	}
}

// TestProjectGet_MergeRequestTitleRegex verifies that the MergeRequestTitleRegex
// and MergeRequestTitleRegexDescription fields are correctly mapped from the API.
func TestProjectGet_MergeRequestTitleRegex(t *testing.T) {
	mrTitleJSON := `{
		"id":42,"name":"my-repo","path_with_namespace":"jmrplens/my-repo",
		"visibility":"private","default_branch":"main",
		"web_url":"https://gitlab.example.com/jmrplens/my-repo",
		"description":"A test repo",
		"merge_request_title_regex":"^(feat|fix):",
		"merge_request_title_regex_description":"MR title must start with feat: or fix:"
	}`
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathProject42 {
			testutil.RespondJSON(w, http.StatusOK, mrTitleJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtGetUnexpErr, err)
	}
	if out.MergeRequestTitleRegex != "^(feat|fix):" {
		t.Errorf("MergeRequestTitleRegex = %q, want %q", out.MergeRequestTitleRegex, "^(feat|fix):")
	}
	if out.MergeRequestTitleRegexDescription != "MR title must start with feat: or fix:" {
		t.Errorf("MergeRequestTitleRegexDescription = %q, want %q", out.MergeRequestTitleRegexDescription, "MR title must start with feat: or fix:")
	}
}

// ---------------------------------------------------------------------------
// Fork
// ---------------------------------------------------------------------------.

// TestProjectFork_Success verifies the behavior of project fork success.
func TestProjectFork_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProject42Fork {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":99,"name":"my-fork","path":"my-fork","path_with_namespace":"user/my-fork","visibility":"private","web_url":"https://gitlab.example.com/user/my-fork","default_branch":"main"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Fork(context.Background(), client, ForkInput{ProjectID: "42", Name: testMyFork})
	if err != nil {
		t.Fatalf("Fork() unexpected error: %v", err)
	}
	if out.ID != 99 {
		t.Errorf("ID = %d, want 99", out.ID)
	}
	if out.Name != testMyFork {
		t.Errorf(fmtNameWantQ, out.Name, testMyFork)
	}
}

// TestProjectFork_EmptyProjectID verifies the behavior of project fork empty project i d.
func TestProjectFork_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	_, err := Fork(context.Background(), client, ForkInput{})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// ---------------------------------------------------------------------------
// Star / Unstar
// ---------------------------------------------------------------------------.

// TestProjectStar_Success verifies the behavior of project star success.
func TestProjectStar_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/star" {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":42,"name":"myproject","path":"myproject","path_with_namespace":"user/myproject","visibility":"private","web_url":"https://gitlab.example.com/user/myproject","default_branch":"main","star_count":5}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Star(context.Background(), client, StarInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("Star() unexpected error: %v", err)
	}
	if out.StarCount != 5 {
		t.Errorf("StarCount = %d, want 5", out.StarCount)
	}
}

// TestProjectStar_EmptyProjectID verifies the behavior of project star empty project i d.
func TestProjectStar_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	_, err := Star(context.Background(), client, StarInput{})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestProjectUnstar_Success verifies the behavior of project unstar success.
func TestProjectUnstar_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/unstar" {
			testutil.RespondJSON(w, http.StatusOK, `{"id":42,"name":"myproject","path":"myproject","path_with_namespace":"user/myproject","visibility":"private","web_url":"https://gitlab.example.com/user/myproject","default_branch":"main","star_count":4}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Unstar(context.Background(), client, UnstarInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("Unstar() unexpected error: %v", err)
	}
	if out.StarCount != 4 {
		t.Errorf("StarCount = %d, want 4", out.StarCount)
	}
}

// TestProjectUnstar_EmptyProjectID verifies the behavior of project unstar empty project i d.
func TestProjectUnstar_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	_, err := Unstar(context.Background(), client, UnstarInput{})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// ---------------------------------------------------------------------------
// Archive / Unarchive
// ---------------------------------------------------------------------------.

// TestProjectArchive_Success verifies the behavior of project archive success.
func TestProjectArchive_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/archive" {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":42,"name":"myproject","path":"myproject","path_with_namespace":"user/myproject","visibility":"private","web_url":"https://gitlab.example.com/user/myproject","default_branch":"main","archived":true}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Archive(context.Background(), client, ArchiveInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("Archive() unexpected error: %v", err)
	}
	if !out.Archived {
		t.Error("Archived = false, want true")
	}
}

// TestProjectArchive_EmptyProjectID verifies the behavior of project archive empty project i d.
func TestProjectArchive_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	_, err := Archive(context.Background(), client, ArchiveInput{})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestProjectUnarchive_Success verifies the behavior of project unarchive success.
func TestProjectUnarchive_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/unarchive" {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":42,"name":"myproject","path":"myproject","path_with_namespace":"user/myproject","visibility":"private","web_url":"https://gitlab.example.com/user/myproject","default_branch":"main","archived":false}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Unarchive(context.Background(), client, UnarchiveInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("Unarchive() unexpected error: %v", err)
	}
	if out.Archived {
		t.Error("Archived = true, want false")
	}
}

// TestProjectUnarchive_EmptyProjectID verifies the behavior of project unarchive empty project i d.
func TestProjectUnarchive_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	_, err := Unarchive(context.Background(), client, UnarchiveInput{})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// ---------------------------------------------------------------------------
// Transfer
// ---------------------------------------------------------------------------.

// TestProjectTransfer_Success verifies the behavior of project transfer success.
func TestProjectTransfer_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/projects/42/transfer" {
			testutil.RespondJSON(w, http.StatusOK, `{"id":42,"name":"myproject","path":"myproject","path_with_namespace":"newns/myproject","visibility":"private","web_url":"https://gitlab.example.com/newns/myproject","default_branch":"main"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Transfer(context.Background(), client, TransferInput{ProjectID: "42", Namespace: "newns"})
	if err != nil {
		t.Fatalf("Transfer() unexpected error: %v", err)
	}
	if out.PathWithNamespace != "newns/myproject" {
		t.Errorf("PathWithNamespace = %q, want %q", out.PathWithNamespace, "newns/myproject")
	}
}

// TestProjectTransfer_EmptyProjectID verifies the behavior of project transfer empty project i d.
func TestProjectTransfer_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	_, err := Transfer(context.Background(), client, TransferInput{Namespace: "ns"})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestProjectTransfer_EmptyNamespace verifies the behavior of project transfer empty namespace.
func TestProjectTransfer_EmptyNamespace(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	_, err := Transfer(context.Background(), client, TransferInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for empty namespace, got nil")
	}
}

// ---------------------------------------------------------------------------
// ListForks
// ---------------------------------------------------------------------------.

// TestProjectListForks_Success verifies the behavior of project list forks success.
func TestProjectListForks_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProject42Forks {
			w.Header().Set(headerXTotal, "2")
			w.Header().Set(headerXTotalPages, "1")
			w.Header().Set(headerXPage, "1")
			w.Header().Set(headerXPerPage, "20")
			testutil.RespondJSON(w, http.StatusOK, `[{"id":100,"name":"fork-a","path":"fork-a","path_with_namespace":"alice/fork-a","visibility":"private","web_url":"https://gitlab.example.com/alice/fork-a","default_branch":"main"},{"id":101,"name":"fork-b","path":"fork-b","path_with_namespace":"bob/fork-b","visibility":"internal","web_url":"https://gitlab.example.com/bob/fork-b","default_branch":"main"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListForks(context.Background(), client, ListForksInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("ListForks() unexpected error: %v", err)
	}
	if len(out.Forks) != 2 {
		t.Fatalf("len(Forks) = %d, want 2", len(out.Forks))
	}
	if out.Forks[0].Name != testForkA {
		t.Errorf("Forks[0].Name = %q, want %q", out.Forks[0].Name, testForkA)
	}
}

// TestProjectListForks_EmptyProjectID verifies the behavior of project list forks empty project i d.
func TestProjectListForks_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := ListForks(context.Background(), client, ListForksInput{})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// ---------------------------------------------------------------------------
// GetLanguages
// ---------------------------------------------------------------------------.

// TestProjectGetLanguages_Success verifies the behavior of project get languages success.
func TestProjectGetLanguages_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/languages" {
			testutil.RespondJSON(w, http.StatusOK, `{"Go":85.5,"Shell":10.2,"Makefile":4.3}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetLanguages(context.Background(), client, GetLanguagesInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("GetLanguages() unexpected error: %v", err)
	}
	if len(out.Languages) != 3 {
		t.Fatalf("len(Languages) = %d, want 3", len(out.Languages))
	}
	found := false
	for _, l := range out.Languages {
		if l.Name == "Go" {
			found = true
			if l.Percentage < 85.0 || l.Percentage > 86.0 {
				t.Errorf("Go percentage = %.1f, want ~85.5", l.Percentage)
			}
		}
	}
	if !found {
		t.Error("expected Go language in output")
	}
}

// TestProjectGetLanguages_EmptyProjectID verifies the behavior of project get languages empty project i d.
func TestProjectGetLanguages_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	_, err := GetLanguages(context.Background(), client, GetLanguagesInput{})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// ---------------------------------------------------------------------------
// ListHooks
// ---------------------------------------------------------------------------.

var hookJSON = `{"id":1,"url":"https://example.com/hook","name":"my-hook","project_id":42,"push_events":true,"issues_events":false,"merge_requests_events":true,"tag_push_events":false,"note_events":true,"job_events":false,"pipeline_events":true,"wiki_page_events":false,"deployment_events":false,"releases_events":true,"enable_ssl_verification":true,"created_at":"2024-01-01T00:00:00Z"}`

// TestProjectListHooks_Success verifies the behavior of project list hooks success.
func TestProjectListHooks_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProject42Hooks {
			w.Header().Set(headerXTotal, "1")
			w.Header().Set(headerXTotalPages, "1")
			w.Header().Set(headerXPage, "1")
			w.Header().Set(headerXPerPage, "20")
			testutil.RespondJSON(w, http.StatusOK, "["+hookJSON+"]")
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListHooks(context.Background(), client, ListHooksInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("ListHooks() unexpected error: %v", err)
	}
	if len(out.Hooks) != 1 {
		t.Fatalf("len(Hooks) = %d, want 1", len(out.Hooks))
	}
	if out.Hooks[0].URL != testHookURL {
		t.Errorf("URL = %q, want %q", out.Hooks[0].URL, testHookURL)
	}
	if !out.Hooks[0].PushEvents {
		t.Error("PushEvents = false, want true")
	}
	if !out.Hooks[0].EnableSSLVerification {
		t.Error("EnableSSLVerification = false, want true")
	}
}

// TestProjectListHooks_EmptyProjectID verifies the behavior of project list hooks empty project i d.
func TestProjectListHooks_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := ListHooks(context.Background(), client, ListHooksInput{})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// ---------------------------------------------------------------------------
// GetHook
// ---------------------------------------------------------------------------.

// TestProjectGetHook_Success verifies the behavior of project get hook success.
func TestProjectGetHook_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProject42Hook1 {
			testutil.RespondJSON(w, http.StatusOK, hookJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetHook(context.Background(), client, GetHookInput{ProjectID: "42", HookID: 1})
	if err != nil {
		t.Fatalf("GetHook() unexpected error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf(fmtIDWant1, out.ID)
	}
	if out.Name != testMyHook {
		t.Errorf(fmtNameWantQ, out.Name, testMyHook)
	}
}

// TestProjectGetHook_EmptyProjectID verifies the behavior of project get hook empty project i d.
func TestProjectGetHook_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	_, err := GetHook(context.Background(), client, GetHookInput{HookID: 1})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestProjectGetHook_EmptyHookID verifies the behavior of project get hook empty hook i d.
func TestProjectGetHook_EmptyHookID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	_, err := GetHook(context.Background(), client, GetHookInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errEmptyHookID)
	}
}

// ---------------------------------------------------------------------------
// AddHook
// ---------------------------------------------------------------------------.

// TestProjectAddHook_Success verifies the behavior of project add hook success.
func TestProjectAddHook_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProject42Hooks {
			testutil.RespondJSON(w, http.StatusCreated, hookJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := AddHook(context.Background(), client, AddHookInput{
		ProjectID:  "42",
		URL:        testHookURL,
		PushEvents: new(true),
	})
	if err != nil {
		t.Fatalf("AddHook() unexpected error: %v", err)
	}
	if out.URL != testHookURL {
		t.Errorf("URL = %q, want %q", out.URL, testHookURL)
	}
}

// TestProjectAddHook_EmptyProjectID verifies the behavior of project add hook empty project i d.
func TestProjectAddHook_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	_, err := AddHook(context.Background(), client, AddHookInput{URL: "https://example.com"})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestProjectAddHook_EmptyURL verifies the behavior of project add hook empty u r l.
func TestProjectAddHook_EmptyURL(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	_, err := AddHook(context.Background(), client, AddHookInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for empty url, got nil")
	}
}

// ---------------------------------------------------------------------------
// EditHook
// ---------------------------------------------------------------------------.

// TestProjectEditHook_Success verifies the behavior of project edit hook success.
func TestProjectEditHook_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathProject42Hook1 {
			testutil.RespondJSON(w, http.StatusOK, hookJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := EditHook(context.Background(), client, EditHookInput{ProjectID: "42", HookID: 1, URL: testHookURL})
	if err != nil {
		t.Fatalf("EditHook() unexpected error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf(fmtIDWant1, out.ID)
	}
}

// TestProjectEditHook_EmptyProjectID verifies the behavior of project edit hook empty project i d.
func TestProjectEditHook_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	_, err := EditHook(context.Background(), client, EditHookInput{HookID: 1})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestProjectEditHook_EmptyHookID verifies the behavior of project edit hook empty hook i d.
func TestProjectEditHook_EmptyHookID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	_, err := EditHook(context.Background(), client, EditHookInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errEmptyHookID)
	}
}

// ---------------------------------------------------------------------------
// DeleteHook
// ---------------------------------------------------------------------------.

// TestProjectDeleteHook_Success verifies the behavior of project delete hook success.
func TestProjectDeleteHook_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathProject42Hook1 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := DeleteHook(context.Background(), client, DeleteHookInput{ProjectID: "42", HookID: 1})
	if err != nil {
		t.Fatalf("DeleteHook() unexpected error: %v", err)
	}
}

// TestProjectDeleteHook_EmptyProjectID verifies the behavior of project delete hook empty project i d.
func TestProjectDeleteHook_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	err := DeleteHook(context.Background(), client, DeleteHookInput{HookID: 1})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestProjectDeleteHook_EmptyHookID verifies the behavior of project delete hook empty hook i d.
func TestProjectDeleteHook_EmptyHookID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	err := DeleteHook(context.Background(), client, DeleteHookInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errEmptyHookID)
	}
}

// ---------------------------------------------------------------------------
// TriggerTestHook
// ---------------------------------------------------------------------------.

// TestProjectTriggerTestHook_Success verifies the behavior of project trigger test hook success.
func TestProjectTriggerTestHook_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/hooks/1/test/push_events" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := TriggerTestHook(context.Background(), client, TriggerTestHookInput{ProjectID: "42", HookID: 1, Event: "push_events"})
	if err != nil {
		t.Fatalf("TriggerTestHook() unexpected error: %v", err)
	}
	if out.Message == "" {
		t.Error("expected non-empty message")
	}
}

// TestProjectTriggerTestHook_EmptyProjectID verifies the behavior of project trigger test hook empty project i d.
func TestProjectTriggerTestHook_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	_, err := TriggerTestHook(context.Background(), client, TriggerTestHookInput{HookID: 1, Event: "push_events"})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestProjectTriggerTestHook_EmptyEvent verifies the behavior of project trigger test hook empty event.
func TestProjectTriggerTestHook_EmptyEvent(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	_, err := TriggerTestHook(context.Background(), client, TriggerTestHookInput{ProjectID: "42", HookID: 1})
	if err == nil {
		t.Fatal("expected error for empty event, got nil")
	}
}

// ---------------------------------------------------------------------------
// ListUserProjects
// ---------------------------------------------------------------------------.

// TestProjectListUserProjects_Success verifies the behavior of project list user projects success.
func TestProjectListUserProjects_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/users/jdoe/projects" {
			w.Header().Set(headerXTotal, "1")
			w.Header().Set(headerXTotalPages, "1")
			w.Header().Set(headerXPage, "1")
			w.Header().Set(headerXPerPage, "20")
			testutil.RespondJSON(w, http.StatusOK, `[{"id":42,"name":"my-project","path_with_namespace":"jdoe/my-project"}]`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := ListUserProjects(context.Background(), client, ListUserProjectsInput{UserID: testUserJdoe})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Projects) != 1 {
		t.Fatalf(fmtLenProjectsWant1, len(out.Projects))
	}
	if out.Projects[0].Name != testProjectName {
		t.Errorf(fmtNameWantQ, out.Projects[0].Name, testProjectName)
	}
}

// TestProjectListUserProjects_EmptyUserID verifies the behavior of project list user projects empty user i d.
func TestProjectListUserProjects_EmptyUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := ListUserProjects(context.Background(), client, ListUserProjectsInput{})
	if err == nil {
		t.Fatal(errEmptyUserID)
	}
}

// ---------------------------------------------------------------------------
// ListProjectUsers
// ---------------------------------------------------------------------------.

// TestProjectListProjectUsers_Success verifies the behavior of project list project users success.
func TestProjectListProjectUsers_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/users" {
			w.Header().Set(headerXTotal, "1")
			w.Header().Set(headerXTotalPages, "1")
			w.Header().Set(headerXPage, "1")
			w.Header().Set(headerXPerPage, "20")
			testutil.RespondJSON(w, http.StatusOK, `[{"id":10,"name":"Jane Doe","username":"jdoe","state":"active","web_url":"https://gl.example.com/jdoe"}]`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := ListProjectUsers(context.Background(), client, ListProjectUsersInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Users) != 1 {
		t.Fatalf("len(Users) = %d, want 1", len(out.Users))
	}
	if out.Users[0].Username != testUserJdoe {
		t.Errorf("Username = %q, want %q", out.Users[0].Username, testUserJdoe)
	}
}

// TestProjectListProjectUsers_EmptyProjectID verifies the behavior of project list project users empty project i d.
func TestProjectListProjectUsers_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := ListProjectUsers(context.Background(), client, ListProjectUsersInput{})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// ---------------------------------------------------------------------------
// ListProjectGroups
// ---------------------------------------------------------------------------.

// TestProjectListProjectGroups_Success verifies the behavior of project list project groups success.
func TestProjectListProjectGroups_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/groups" {
			w.Header().Set(headerXTotal, "1")
			w.Header().Set(headerXTotalPages, "1")
			w.Header().Set(headerXPage, "1")
			w.Header().Set(headerXPerPage, "20")
			testutil.RespondJSON(w, http.StatusOK, `[{"id":5,"name":"my-group","full_name":"My Group","full_path":"my-group","web_url":"https://gl.example.com/groups/my-group"}]`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := ListProjectGroups(context.Background(), client, ListProjectGroupsInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Groups) != 1 {
		t.Fatalf(fmtLenGroupsWant1, len(out.Groups))
	}
	if out.Groups[0].FullPath != testMyGroup {
		t.Errorf("FullPath = %q, want %q", out.Groups[0].FullPath, testMyGroup)
	}
}

// TestProjectListProjectGroups_EmptyProjectID verifies the behavior of project list project groups empty project i d.
func TestProjectListProjectGroups_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := ListProjectGroups(context.Background(), client, ListProjectGroupsInput{})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// ---------------------------------------------------------------------------
// ListProjectStarrers
// ---------------------------------------------------------------------------.

// TestProjectListStarrers_Success verifies the behavior of project list starrers success.
func TestProjectListStarrers_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/starrers" {
			w.Header().Set(headerXTotal, "1")
			w.Header().Set(headerXTotalPages, "1")
			w.Header().Set(headerXPage, "1")
			w.Header().Set(headerXPerPage, "20")
			testutil.RespondJSON(w, http.StatusOK, `[{"starred_since":"2024-01-15T10:00:00Z","user":{"id":10,"name":"Jane Doe","username":"jdoe","state":"active"}}]`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := ListProjectStarrers(context.Background(), client, ListProjectStarrersInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Starrers) != 1 {
		t.Fatalf("len(Starrers) = %d, want 1", len(out.Starrers))
	}
	if out.Starrers[0].User.Username != testUserJdoe {
		t.Errorf("Username = %q, want %q", out.Starrers[0].User.Username, testUserJdoe)
	}
	if out.Starrers[0].StarredSince == "" {
		t.Error("StarredSince is empty, expected date string")
	}
}

// TestProjectListStarrers_EmptyProjectID verifies the behavior of project list starrers empty project i d.
func TestProjectListStarrers_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := ListProjectStarrers(context.Background(), client, ListProjectStarrersInput{})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// ---------------------------------------------------------------------------
// ShareProjectWithGroup
// ---------------------------------------------------------------------------.

// TestProjectShare_WithGroupSuccess verifies the behavior of project share with group success.
func TestProjectShare_WithGroupSuccess(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/share" {
			w.WriteHeader(http.StatusCreated)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := ShareProjectWithGroup(context.Background(), client, ShareProjectInput{
		ProjectID:   "42",
		GroupID:     5,
		GroupAccess: 30,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Message == "" {
		t.Error("expected non-empty message")
	}
}

// TestProjectShare_WithGroupEmptyProjectID verifies the behavior of project share with group empty project i d.
func TestProjectShare_WithGroupEmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	_, err := ShareProjectWithGroup(context.Background(), client, ShareProjectInput{GroupID: 5, GroupAccess: 30})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestProjectShare_WithGroupEmptyGroupID verifies the behavior of project share with group empty group i d.
func TestProjectShare_WithGroupEmptyGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	_, err := ShareProjectWithGroup(context.Background(), client, ShareProjectInput{ProjectID: "42", GroupAccess: 30})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestProjectShare_WithGroupEmptyGroupAccess verifies the behavior of project share with group empty group access.
func TestProjectShare_WithGroupEmptyGroupAccess(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	_, err := ShareProjectWithGroup(context.Background(), client, ShareProjectInput{ProjectID: "42", GroupID: 5})
	if err == nil {
		t.Fatal("expected error for empty group_access, got nil")
	}
}

// ---------------------------------------------------------------------------
// DeleteSharedProjectFromGroup
// ---------------------------------------------------------------------------.

// TestProjectDeleteSharedGroup_Success verifies the behavior of project delete shared group success.
func TestProjectDeleteSharedGroup_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v4/projects/42/share/5" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	err := DeleteSharedProjectFromGroup(context.Background(), client, DeleteSharedGroupInput{ProjectID: "42", GroupID: 5})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestProjectDeleteSharedGroup_EmptyProjectID verifies the behavior of project delete shared group empty project i d.
func TestProjectDeleteSharedGroup_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	err := DeleteSharedProjectFromGroup(context.Background(), client, DeleteSharedGroupInput{GroupID: 5})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestProjectDeleteSharedGroup_EmptyGroupID verifies the behavior of project delete shared group empty group i d.
func TestProjectDeleteSharedGroup_EmptyGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	err := DeleteSharedProjectFromGroup(context.Background(), client, DeleteSharedGroupInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// ---------------------------------------------------------------------------
// ListInvitedGroups
// ---------------------------------------------------------------------------.

// TestProjectListInvitedGroups_Success verifies the behavior of project list invited groups success.
func TestProjectListInvitedGroups_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/invited_groups" {
			w.Header().Set(headerXTotal, "1")
			w.Header().Set(headerXTotalPages, "1")
			w.Header().Set(headerXPage, "1")
			w.Header().Set(headerXPerPage, "20")
			testutil.RespondJSON(w, http.StatusOK, `[{"id":7,"name":"invited-grp","full_name":"Invited Group","full_path":"invited-grp"}]`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := ListInvitedGroups(context.Background(), client, ListInvitedGroupsInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Groups) != 1 {
		t.Fatalf(fmtLenGroupsWant1, len(out.Groups))
	}
	if out.Groups[0].FullPath != "invited-grp" {
		t.Errorf("FullPath = %q, want %q", out.Groups[0].FullPath, "invited-grp")
	}
}

// TestProjectListInvitedGroups_EmptyProjectID verifies the behavior of project list invited groups empty project i d.
func TestProjectListInvitedGroups_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := ListInvitedGroups(context.Background(), client, ListInvitedGroupsInput{})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// ---------------------------------------------------------------------------
// ListUserContributedProjects tests
// ---------------------------------------------------------------------------.

// TestListUserContributedProjects_Success verifies the behavior of list user contributed projects success.
func TestListUserContributedProjects_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/users/john/contributed_projects" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":10,"name":"contrib-proj","path_with_namespace":"john/contrib-proj","visibility":"public","web_url":"https://gitlab.example.com/john/contrib-proj"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListUserContributedProjects(context.Background(), client, ListUserContributedProjectsInput{
		UserID: testUserJohn,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Projects) != 1 {
		t.Fatalf(fmtLenProjectsWant1, len(out.Projects))
	}
	if out.Projects[0].Name != "contrib-proj" {
		t.Errorf(fmtNameWantQ, out.Projects[0].Name, "contrib-proj")
	}
}

// TestListUserContributedProjects_EmptyUserID verifies the behavior of list user contributed projects empty user i d.
func TestListUserContributedProjects_EmptyUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := ListUserContributedProjects(context.Background(), client, ListUserContributedProjectsInput{})
	if err == nil {
		t.Fatal(errEmptyUserID)
	}
}

// ---------------------------------------------------------------------------
// ListUserStarredProjects tests
// ---------------------------------------------------------------------------.

// TestListUserStarredProjects_Success verifies the behavior of list user starred projects success.
func TestListUserStarredProjects_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/users/jane/starred_projects" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":20,"name":"starred-proj","path_with_namespace":"jane/starred-proj","visibility":"internal","web_url":"https://gitlab.example.com/jane/starred-proj"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListUserStarredProjects(context.Background(), client, ListUserStarredProjectsInput{
		UserID: "jane",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Projects) != 1 {
		t.Fatalf(fmtLenProjectsWant1, len(out.Projects))
	}
	if out.Projects[0].Name != "starred-proj" {
		t.Errorf(fmtNameWantQ, out.Projects[0].Name, "starred-proj")
	}
}

// TestListUserStarredProjects_EmptyUserID verifies the behavior of list user starred projects empty user i d.
func TestListUserStarredProjects_EmptyUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := ListUserStarredProjects(context.Background(), client, ListUserStarredProjectsInput{})
	if err == nil {
		t.Fatal(errEmptyUserID)
	}
}

// ---------------------------------------------------------------------------
// Push Rules tests
// ---------------------------------------------------------------------------.

const (
	pathPushRules42 = "/api/v4/projects/42/push_rule"
	pushRuleJSON    = `{
		"id": 1,
		"project_id": 42,
		"commit_message_regex": "^(feat|fix|docs):",
		"commit_message_negative_regex": "WIP",
		"branch_name_regex": "^(main|release/.*)$",
		"deny_delete_tag": true,
		"member_check": false,
		"prevent_secrets": true,
		"author_email_regex": "@example\\.com$",
		"file_name_regex": "",
		"max_file_size": 10,
		"commit_committer_check": true,
		"commit_committer_name_check": false,
		"reject_unsigned_commits": false,
		"reject_non_dco_commits": false,
		"created_at": "2025-01-01T00:00:00Z"
	}`
)

// TestGetPushRules_Success verifies the behavior of get push rules success.
func TestGetPushRules_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathPushRules42 {
			testutil.RespondJSON(w, http.StatusOK, pushRuleJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetPushRules(context.Background(), client, GetPushRulesInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 1 {
		t.Errorf(fmtIDWant1, out.ID)
	}
	if out.ProjectID != 42 {
		t.Errorf("ProjectID = %d, want 42", out.ProjectID)
	}
	if out.CommitMessageRegex != testCommitRegex {
		t.Errorf("CommitMessageRegex = %q, want %q", out.CommitMessageRegex, testCommitRegex)
	}
	if !out.DenyDeleteTag {
		t.Error("DenyDeleteTag = false, want true")
	}
	if !out.PreventSecrets {
		t.Error("PreventSecrets = false, want true")
	}
	if out.MaxFileSize != 10 {
		t.Errorf("MaxFileSize = %d, want 10", out.MaxFileSize)
	}
}

// TestGetPushRules_EmptyProjectID verifies the behavior of get push rules empty project i d.
func TestGetPushRules_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, pushRuleJSON)
	}))
	_, err := GetPushRules(context.Background(), client, GetPushRulesInput{})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestAddPushRule_Success verifies the behavior of add push rule success.
func TestAddPushRule_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathPushRules42 {
			testutil.RespondJSON(w, http.StatusCreated, pushRuleJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := AddPushRule(context.Background(), client, AddPushRuleInput{
		ProjectID:          "42",
		CommitMessageRegex: testCommitRegex,
		PreventSecrets:     new(true),
		DenyDeleteTag:      new(true),
		MaxFileSize:        int64Ptr(10),
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 1 {
		t.Errorf(fmtIDWant1, out.ID)
	}
	if out.CommitMessageRegex != testCommitRegex {
		t.Errorf("CommitMessageRegex = %q, want %q", out.CommitMessageRegex, testCommitRegex)
	}
}

// TestAddPushRule_EmptyProjectID verifies the behavior of add push rule empty project i d.
func TestAddPushRule_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, pushRuleJSON)
	}))
	_, err := AddPushRule(context.Background(), client, AddPushRuleInput{})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestEditPushRule_Success verifies the behavior of edit push rule success.
func TestEditPushRule_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathPushRules42 {
			testutil.RespondJSON(w, http.StatusOK, pushRuleJSON)
			return
		}
		http.NotFound(w, r)
	}))

	newRegex := "^(feat|fix|docs|refactor):"
	out, err := EditPushRule(context.Background(), client, EditPushRuleInput{
		ProjectID:          "42",
		CommitMessageRegex: &newRegex,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 1 {
		t.Errorf(fmtIDWant1, out.ID)
	}
}

// TestEditPushRule_EmptyProjectID verifies the behavior of edit push rule empty project i d.
func TestEditPushRule_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, pushRuleJSON)
	}))
	_, err := EditPushRule(context.Background(), client, EditPushRuleInput{})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestDeletePushRule_Success verifies the behavior of delete push rule success.
func TestDeletePushRule_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathPushRules42 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := DeletePushRule(context.Background(), client, DeletePushRuleInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeletePushRule_EmptyProjectID verifies the behavior of delete push rule empty project i d.
func TestDeletePushRule_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	err := DeletePushRule(context.Background(), client, DeletePushRuleInput{})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

// TestGetPushRules_NotFound verifies the behavior of get push rules not found.
func TestGetPushRules_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathPushRules42 {
			testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := GetPushRules(context.Background(), client, GetPushRulesInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
}

// ---------------------------------------------------------------------------
// FormatPushRuleMarkdown test
// ---------------------------------------------------------------------------.

// TestFormatPushRuleMarkdown verifies the behavior of format push rule markdown.
func TestFormatPushRuleMarkdown(t *testing.T) {
	out := PushRuleOutput{
		ID:                 1,
		ProjectID:          42,
		CommitMessageRegex: "^feat:",
		DenyDeleteTag:      true,
		PreventSecrets:     true,
		MaxFileSize:        10,
	}
	md := FormatPushRuleMarkdown(out)
	if md == "" {
		t.Fatal(errExpectedNonEmptyMD)
	}
	if !strings.Contains(md, "Push Rule") {
		t.Error("markdown should contain 'Push Rule'")
	}
	if !strings.Contains(md, "^feat:") {
		t.Error("markdown should contain commit message regex")
	}
}

// TestAccessLevelName validates access level name across multiple scenarios using table-driven subtests.
func TestAccessLevelName(t *testing.T) {
	tests := []struct {
		level int
		want  string
	}{
		{10, "Guest"},
		{20, "Reporter"},
		{30, "Developer"},
		{40, "Maintainer"},
		{50, "Owner"},
		{99, "Level 99"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			got := accessLevelName(tc.level)
			if got != tc.want {
				t.Errorf("accessLevelName(%d) = %q, want %q", tc.level, got, tc.want)
			}
		})
	}
}

// TestFormatShareProjectMarkdown verifies the behavior of format share project markdown.
func TestFormatShareProjectMarkdown(t *testing.T) {
	out := ShareProjectOutput{
		Message:     "Project 42 shared with group 5 as Developer",
		GroupID:     5,
		GroupAccess: 30,
		AccessRole:  testAccessDeveloper,
	}
	md := FormatShareProjectMarkdown(out)
	if !strings.Contains(md, "## Project Shared") {
		t.Error("expected markdown header")
	}
	if !strings.Contains(md, testAccessDeveloper) {
		t.Error("expected role name 'Developer' in markdown")
	}
	if !strings.Contains(md, "5") {
		t.Error("expected group ID in markdown")
	}
	if strings.Contains(md, "access level 30") {
		t.Error("should not contain raw numeric access level")
	}
}

// TestFormatShareProjectMarkdown_Minimal verifies the behavior of format share project markdown minimal.
func TestFormatShareProjectMarkdown_Minimal(t *testing.T) {
	out := ShareProjectOutput{
		Message: "Project shared",
	}
	md := FormatShareProjectMarkdown(out)
	if !strings.Contains(md, "## Project Shared") {
		t.Error("expected markdown header")
	}
	if strings.Contains(md, "| Field |") {
		t.Error("should not contain table when GroupID is zero")
	}
}

// TestShareProjectOutput_ContainsRoleName verifies the behavior of share project output contains role name.
func TestShareProjectOutput_ContainsRoleName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/share" {
			w.WriteHeader(http.StatusCreated)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := ShareProjectWithGroup(context.Background(), client, ShareProjectInput{
		ProjectID:   "42",
		GroupID:     5,
		GroupAccess: 30,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.AccessRole != testAccessDeveloper {
		t.Errorf("expected AccessRole=Developer, got %q", out.AccessRole)
	}
	if out.GroupID != 5 {
		t.Errorf("expected GroupID=5, got %d", out.GroupID)
	}
	if out.GroupAccess != 30 {
		t.Errorf("expected GroupAccess=30, got %d", out.GroupAccess)
	}
	if strings.Contains(out.Message, "access level 30") {
		t.Error("message should not contain raw 'access level 30'")
	}
	if !strings.Contains(out.Message, testAccessDeveloper) {
		t.Error("message should contain role name 'Developer'")
	}
}

// Test helpers
//
//go:fix inline
func int64Ptr(i int64) *int64 { return new(i) }

// ---------------------------------------------------------------------------
// Format function coverage tests
// ---------------------------------------------------------------------------.

// TestFormatMarkdown verifies the behavior of format markdown.
func TestFormatMarkdown(t *testing.T) {
	out := Output{
		ID:                1,
		Name:              "test-project",
		PathWithNamespace: "group/test-project",
		Visibility:        testPrivate,
		DefaultBranch:     "main",
		Description:       testDescProject,
		Namespace:         "group",
		Archived:          true,
		ForksCount:        5,
		StarCount:         10,
		OpenIssuesCount:   3,
		Topics:            []string{"go", "mcp"},
		CreatedAt:         "2025-01-01T00:00:00Z",
		WebURL:            "https://gitlab.example.com/group/test-project",
		HTTPURLToRepo:     "https://gitlab.example.com/group/test-project.git",
		SSHURLToRepo:      "git@gitlab.example.com:group/test-project.git",
	}
	md := FormatMarkdown(out)
	for _, want := range []string{"test-project", "group/test-project", testPrivate, "main", testDescProject, "group", "Archived", "Forks", "Stars", mdOpenIssues, "go, mcp", "1 Jan 2025", mdHTTPClone, mdSSHClone} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatMarkdown missing %q", want)
		}
	}
}

// TestFormatMarkdown_Minimal verifies the behavior of format markdown minimal.
func TestFormatMarkdown_Minimal(t *testing.T) {
	out := Output{ID: 1, Name: "minimal", Visibility: testPublic}
	md := FormatMarkdown(out)
	if !strings.Contains(md, "minimal") {
		t.Error("FormatMarkdown missing project name")
	}
}

// TestFormatDeleteMarkdown validates format delete markdown across multiple scenarios using table-driven subtests.
func TestFormatDeleteMarkdown(t *testing.T) {
	tests := []struct {
		name string
		out  DeleteOutput
		want []string
	}{
		{
			name: "permanently_removed",
			out:  DeleteOutput{Status: testSuccess, Message: "deleted", PermanentlyRemoved: true},
			want: []string{testSuccess, "deleted", testPermRemoved},
		},
		{
			name: "scheduled_deletion",
			out:  DeleteOutput{Status: testSuccess, Message: "scheduled", MarkedForDeletionOn: testDate20250601},
			want: []string{testSuccess, "scheduled", testDate20250601},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := FormatDeleteMarkdown(tt.out)
			for _, w := range tt.want {
				if !strings.Contains(md, w) {
					t.Errorf("FormatDeleteMarkdown missing %q", w)
				}
			}
		})
	}
}

// TestFormatListMarkdown verifies the behavior of format list markdown.
func TestFormatListMarkdown(t *testing.T) {
	t.Run("with_projects", func(t *testing.T) {
		out := ListOutput{
			Projects: []Output{
				{ID: 1, Name: testProjA, PathWithNamespace: "g/proj-a", Visibility: testPublic, StarCount: 5},
				{ID: 2, Name: testProjB, PathWithNamespace: "g/proj-b", Visibility: testPrivate, Archived: true},
			},
			Pagination: toolutil.PaginationOutput{TotalItems: 2},
		}
		md := FormatListMarkdown(out)
		for _, w := range []string{"Projects (2)", testProjA, testProjB, testPublic, testPrivate} {
			if !strings.Contains(md, w) {
				t.Errorf("FormatListMarkdown missing %q", w)
			}
		}
	})
	t.Run("empty", func(t *testing.T) {
		md := FormatListMarkdown(ListOutput{})
		if !strings.Contains(md, "No projects found") {
			t.Error("expected 'No projects found' message")
		}
	})
}

// TestFormatListForksMarkdown verifies the behavior of format list forks markdown.
func TestFormatListForksMarkdown(t *testing.T) {
	t.Run("with_forks", func(t *testing.T) {
		out := ListForksOutput{
			Forks:      []Output{{ID: 10, Name: "fork-1", PathWithNamespace: "u/fork-1", Visibility: "internal"}},
			Pagination: toolutil.PaginationOutput{TotalItems: 1},
		}
		md := FormatListForksMarkdown(out)
		if !strings.Contains(md, "fork-1") {
			t.Error("missing fork name")
		}
	})
	t.Run("empty", func(t *testing.T) {
		md := FormatListForksMarkdown(ListForksOutput{})
		if !strings.Contains(md, "No forks found") {
			t.Error("expected 'No forks found' message")
		}
	})
}

// TestFormatLanguagesMarkdown verifies the behavior of format languages markdown.
func TestFormatLanguagesMarkdown(t *testing.T) {
	t.Run("with_languages", func(t *testing.T) {
		out := LanguagesOutput{
			Languages: []LanguageEntry{
				{Name: "Go", Percentage: 85.5},
				{Name: "Shell", Percentage: 14.5},
			},
		}
		md := FormatLanguagesMarkdown(out)
		if !strings.Contains(md, "Go") || !strings.Contains(md, "85.5%") {
			t.Error("missing language data")
		}
	})
	t.Run("empty", func(t *testing.T) {
		md := FormatLanguagesMarkdown(LanguagesOutput{})
		if !strings.Contains(md, "No languages detected") {
			t.Error("expected 'No languages detected' message")
		}
	})
}

// TestFormatListHooksMarkdown verifies the behavior of format list hooks markdown.
func TestFormatListHooksMarkdown(t *testing.T) {
	t.Run("with_hooks", func(t *testing.T) {
		out := ListHooksOutput{
			Hooks: []HookOutput{
				{ID: 1, URL: testHookURL, PushEvents: true},
			},
			Pagination: toolutil.PaginationOutput{TotalItems: 1},
		}
		md := FormatListHooksMarkdown(out)
		if !strings.Contains(md, "example.com/hook") {
			t.Error("missing hook URL")
		}
	})
	t.Run("empty", func(t *testing.T) {
		md := FormatListHooksMarkdown(ListHooksOutput{})
		if !strings.Contains(md, "No webhooks found") {
			t.Error("expected 'No webhooks found' message")
		}
	})
}

// TestFormatHookMarkdown verifies the behavior of format hook markdown.
func TestFormatHookMarkdown(t *testing.T) {
	out := HookOutput{
		ID:                    1,
		URL:                   testHookURL,
		Name:                  testHookName,
		ProjectID:             42,
		PushEvents:            true,
		IssuesEvents:          true,
		MergeRequestsEvents:   true,
		EnableSSLVerification: true,
	}
	md := FormatHookMarkdown(out)
	for _, w := range []string{"test-hook", "example.com/hook", "Push", "Issues", "Merge Requests"} {
		if !strings.Contains(md, w) {
			t.Errorf("FormatHookMarkdown missing %q", w)
		}
	}
}

// TestFormatListProjectUsersMarkdown verifies the behavior of format list project users markdown.
func TestFormatListProjectUsersMarkdown(t *testing.T) {
	t.Run("with_users", func(t *testing.T) {
		out := ListProjectUsersOutput{
			Users: []ProjectUserOutput{
				{ID: 1, Username: testUserJohn, Name: "John Doe", State: "active", WebURL: "https://gitlab.example.com/john"},
			},
			Pagination: toolutil.PaginationOutput{TotalItems: 1},
		}
		md := FormatListProjectUsersMarkdown(out)
		if !strings.Contains(md, testUserJohn) || !strings.Contains(md, "John Doe") {
			t.Error("missing user data")
		}
	})
	t.Run("empty", func(t *testing.T) {
		md := FormatListProjectUsersMarkdown(ListProjectUsersOutput{})
		if !strings.Contains(md, "No users found") {
			t.Error("expected 'No users found' message")
		}
	})
}

// TestFormatListProjectGroupsMarkdown verifies the behavior of format list project groups markdown.
func TestFormatListProjectGroupsMarkdown(t *testing.T) {
	t.Run("with_groups", func(t *testing.T) {
		out := ListProjectGroupsOutput{
			Groups: []ProjectGroupOutput{
				{ID: 1, Name: "devs", FullPath: "company/devs"},
			},
			Pagination: toolutil.PaginationOutput{TotalItems: 1},
		}
		md := FormatListProjectGroupsMarkdown(out)
		if !strings.Contains(md, "devs") || !strings.Contains(md, "company/devs") {
			t.Error("missing group data")
		}
	})
	t.Run("empty", func(t *testing.T) {
		md := FormatListProjectGroupsMarkdown(ListProjectGroupsOutput{})
		if !strings.Contains(md, "No groups found") {
			t.Error("expected 'No groups found' message")
		}
	})
}

// TestFormatListStarrersMarkdown verifies the behavior of format list starrers markdown.
func TestFormatListStarrersMarkdown(t *testing.T) {
	t.Run("with_starrers", func(t *testing.T) {
		out := ListProjectStarrersOutput{
			Starrers: []StarrerOutput{
				{StarredSince: testDate20250101, User: ProjectUserOutput{ID: 1, Username: "jane", Name: "Jane Doe"}},
			},
			Pagination: toolutil.PaginationOutput{TotalItems: 1},
		}
		md := FormatListStarrersMarkdown(out)
		if !strings.Contains(md, "jane") || !strings.Contains(md, "1 Jan 2025") {
			t.Error("missing starrer data")
		}
	})
	t.Run("empty", func(t *testing.T) {
		md := FormatListStarrersMarkdown(ListProjectStarrersOutput{})
		if !strings.Contains(md, "No starrers found") {
			t.Error("expected 'No starrers found' message")
		}
	})
}

// ---------------------------------------------------------------------------
// Option builder coverage — exercising branches
// ---------------------------------------------------------------------------.

// TestFork_WithAllOptions verifies the behavior of fork with all options.
func TestFork_WithAllOptions(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProject42Fork {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":100,"name":"my-fork","path_with_namespace":"user/my-fork","visibility":"private","web_url":"https://gitlab.example.com/user/my-fork"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Fork(context.Background(), client, ForkInput{
		ProjectID:                     "42",
		Name:                          testMyFork,
		Path:                          testMyFork,
		NamespaceID:                   10,
		NamespacePath:                 "user",
		Description:                   "fork desc",
		Visibility:                    testPrivate,
		Branches:                      "main",
		MergeRequestDefaultTargetSelf: new(true),
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Name != testMyFork {
		t.Errorf(fmtNameWantQ, out.Name, testMyFork)
	}
}

// TestListForks_WithFilters verifies the behavior of list forks with filters.
func TestListForks_WithFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathProject42Forks {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":100,"name":"fork-1","path_with_namespace":"user/fork-1","visibility":"public","web_url":"https://example.com/fork-1"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListForks(context.Background(), client, ListForksInput{
		ProjectID:  "42",
		Owned:      true,
		Search:     "fork",
		Visibility: testPublic,
		OrderBy:    "name",
		Sort:       testSortAsc,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Forks) != 1 {
		t.Fatalf("len(Forks) = %d, want 1", len(out.Forks))
	}
}

// TestAddHook_WithAllEvents verifies the behavior of add hook with all events.
func TestAddHook_WithAllEvents(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProject42Hooks {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":1,"url":"https://example.com/hook","project_id":42,"push_events":true,"issues_events":true,"merge_requests_events":true,"tag_push_events":true,"note_events":true,"confidential_note_events":true,"job_events":true,"pipeline_events":true,"wiki_page_events":true,"deployment_events":true,"releases_events":true,"emoji_events":true,"resource_access_token_events":true}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := AddHook(context.Background(), client, AddHookInput{
		ProjectID:                 "42",
		URL:                       testHookURL,
		Name:                      "full-hook",
		Description:               "all events",
		Token:                     "secret",
		PushEvents:                new(true),
		PushEventsBranchFilter:    "main",
		IssuesEvents:              new(true),
		ConfidentialIssuesEvents:  new(true),
		MergeRequestsEvents:       new(true),
		TagPushEvents:             new(true),
		NoteEvents:                new(true),
		ConfidentialNoteEvents:    new(true),
		JobEvents:                 new(true),
		PipelineEvents:            new(true),
		WikiPageEvents:            new(true),
		DeploymentEvents:          new(true),
		ReleasesEvents:            new(true),
		EmojiEvents:               new(true),
		ResourceAccessTokenEvents: new(true),
		EnableSSLVerification:     new(true),
		CustomWebhookTemplate:     "tpl",
		BranchFilterStrategy:      "all",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 1 {
		t.Errorf(fmtIDWant1, out.ID)
	}
}

// TestEditHook_WithAllEvents verifies the behavior of edit hook with all events.
func TestEditHook_WithAllEvents(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathProject42Hook1 {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"url":"https://example.com/hook-updated","project_id":42,"push_events":true}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := EditHook(context.Background(), client, EditHookInput{
		ProjectID:                 "42",
		HookID:                    1,
		URL:                       "https://example.com/hook-updated",
		Name:                      "updated-hook",
		Description:               "updated",
		Token:                     "new-secret",
		PushEvents:                new(true),
		PushEventsBranchFilter:    testBranchDevelop,
		IssuesEvents:              new(false),
		ConfidentialIssuesEvents:  new(false),
		MergeRequestsEvents:       new(true),
		TagPushEvents:             new(true),
		NoteEvents:                new(true),
		ConfidentialNoteEvents:    new(false),
		JobEvents:                 new(true),
		PipelineEvents:            new(true),
		WikiPageEvents:            new(false),
		DeploymentEvents:          new(true),
		ReleasesEvents:            new(false),
		EmojiEvents:               new(true),
		ResourceAccessTokenEvents: new(false),
		EnableSSLVerification:     new(false),
		CustomWebhookTemplate:     "new-tpl",
		BranchFilterStrategy:      "regex",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.URL != "https://example.com/hook-updated" {
		t.Errorf("URL = %q, want updated URL", out.URL)
	}
}

// ---------------------------------------------------------------------------
// FormatMarkdown tests
// ---------------------------------------------------------------------------.

// TestFormatMarkdown_FullFields verifies the behavior of format markdown full fields.
func TestFormatMarkdown_FullFields(t *testing.T) {
	p := Output{
		ID:                42,
		Name:              testProjectName,
		PathWithNamespace: "ns/my-project",
		Visibility:        testPublic,
		DefaultBranch:     "main",
		Description:       testDescProject,
		Namespace:         "ns",
		Archived:          true,
		ForksCount:        5,
		StarCount:         10,
		OpenIssuesCount:   3,
		Topics:            []string{"go", "mcp"},
		CreatedAt:         "2025-01-01T00:00:00Z",
		WebURL:            "https://gitlab.example.com/ns/my-project",
		HTTPURLToRepo:     "https://gitlab.example.com/ns/my-project.git",
		SSHURLToRepo:      "git@gitlab.example.com:ns/my-project.git",
	}
	md := FormatMarkdown(p)
	for _, want := range []string{
		"## Project: my-project",
		"42",
		"ns/my-project",
		testPublic,
		"main",
		testDescProject,
		"Namespace",
		"Archived",
		"Forks",
		"Stars",
		mdOpenIssues,
		"go, mcp",
		"1 Jan 2025",
		mdHTTPClone,
		mdSSHClone,
	} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatMarkdown missing %q", want)
		}
	}
}

// TestFormatMarkdown_MinimalFields verifies the behavior of format markdown minimal fields.
func TestFormatMarkdown_MinimalFields(t *testing.T) {
	p := Output{
		ID:                1,
		Name:              "bare",
		PathWithNamespace: "ns/bare",
		Visibility:        testPrivate,
		DefaultBranch:     "main",
		WebURL:            "https://gitlab.example.com/ns/bare",
	}
	md := FormatMarkdown(p)
	if md == "" {
		t.Fatal(errExpectedNonEmptyMD)
	}
	// Optional fields should NOT appear
	for _, absent := range []string{"Namespace", "Archived", "Forks", "Stars", mdOpenIssues, "Topics", mdHTTPClone, mdSSHClone} {
		if strings.Contains(md, absent) {
			t.Errorf("FormatMarkdown should not contain %q for minimal output", absent)
		}
	}
}

// ---------------------------------------------------------------------------
// FormatDeleteMarkdown tests
// ---------------------------------------------------------------------------.

// TestFormatDeleteMarkdown_PermanentlyRemoved verifies the behavior of format delete markdown permanently removed.
func TestFormatDeleteMarkdown_PermanentlyRemoved(t *testing.T) {
	out := DeleteOutput{
		Status:             testSuccess,
		Message:            "Project deleted",
		PermanentlyRemoved: true,
	}
	md := FormatDeleteMarkdown(out)
	for _, want := range []string{"Project Deletion", testSuccess, "Project deleted", testPermRemoved} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatDeleteMarkdown missing %q", want)
		}
	}
}

// TestFormatDeleteMarkdown_MarkedForDeletion verifies the behavior of format delete markdown marked for deletion.
func TestFormatDeleteMarkdown_MarkedForDeletion(t *testing.T) {
	out := DeleteOutput{
		Status:              "scheduled",
		Message:             "marked for deletion",
		MarkedForDeletionOn: testDate20250601,
	}
	md := FormatDeleteMarkdown(out)
	if !strings.Contains(md, testDate20250601) {
		t.Error("FormatDeleteMarkdown should contain deletion date")
	}
	if strings.Contains(md, testPermRemoved) {
		t.Error("FormatDeleteMarkdown should not contain permanently removed for scheduled")
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown tests
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithProjects verifies the behavior of format list markdown with projects.
func TestFormatListMarkdown_WithProjects(t *testing.T) {
	out := ListOutput{
		Projects: []Output{
			{ID: 1, Name: testProjA, PathWithNamespace: "ns/proj-a", Visibility: testPublic, StarCount: 5},
			{ID: 2, Name: testProjB, PathWithNamespace: "ns/proj-b", Visibility: testPrivate, Archived: true},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2},
	}
	md := FormatListMarkdown(out)
	for _, want := range []string{"Projects (2)", testProjA, testProjB, testPublic, testPrivate} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatListMarkdown missing %q", want)
		}
	}
}

// TestFormatListMarkdown_Empty verifies the behavior of format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	out := ListOutput{
		Projects:   []Output{},
		Pagination: toolutil.PaginationOutput{TotalItems: 0},
	}
	md := FormatListMarkdown(out)
	if !strings.Contains(md, "No projects found") {
		t.Error("FormatListMarkdown should say no projects found for empty list")
	}
}

// TestFormatListMarkdown_ClickableProjectLinks verifies that project names
// in the list are rendered as clickable Markdown links [name](weburl).
func TestFormatListMarkdown_ClickableProjectLinks(t *testing.T) {
	out := ListOutput{
		Projects: []Output{
			{ID: 1, Name: "My Project", PathWithNamespace: "ns/my-project",
				Visibility: testPublic, WebURL: "https://gitlab.example.com/ns/my-project"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	}
	md := FormatListMarkdown(out)
	if !strings.Contains(md, "[My Project](https://gitlab.example.com/ns/my-project)") {
		t.Errorf("expected clickable project name link, got:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// FormatListForksMarkdown tests
// ---------------------------------------------------------------------------.

// TestFormatListForksMarkdown_WithForks verifies the behavior of format list forks markdown with forks.
func TestFormatListForksMarkdown_WithForks(t *testing.T) {
	out := ListForksOutput{
		Forks: []Output{
			{ID: 10, Name: testForkA, PathWithNamespace: "user/fork-a", Visibility: testPublic, StarCount: 2},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	}
	md := FormatListForksMarkdown(out)
	for _, want := range []string{"Project Forks (1)", testForkA, "user/fork-a", testPublic} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatListForksMarkdown missing %q", want)
		}
	}
}

// TestFormatListForksMarkdown_Empty verifies the behavior of format list forks markdown empty.
func TestFormatListForksMarkdown_Empty(t *testing.T) {
	out := ListForksOutput{
		Forks:      []Output{},
		Pagination: toolutil.PaginationOutput{TotalItems: 0},
	}
	md := FormatListForksMarkdown(out)
	if !strings.Contains(md, "No forks found") {
		t.Error("FormatListForksMarkdown should say no forks found for empty list")
	}
}

// ---------------------------------------------------------------------------
// FormatLanguagesMarkdown tests
// ---------------------------------------------------------------------------.

// TestFormatLanguagesMarkdown_WithLanguages verifies the behavior of format languages markdown with languages.
func TestFormatLanguagesMarkdown_WithLanguages(t *testing.T) {
	out := LanguagesOutput{
		Languages: []LanguageEntry{
			{Name: "Go", Percentage: 72.5},
			{Name: "Shell", Percentage: 27.5},
		},
	}
	md := FormatLanguagesMarkdown(out)
	for _, want := range []string{"Project Languages", "Go", "72.5%", "Shell", "27.5%"} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatLanguagesMarkdown missing %q", want)
		}
	}
}

// TestFormatLanguagesMarkdown_Empty verifies the behavior of format languages markdown empty.
func TestFormatLanguagesMarkdown_Empty(t *testing.T) {
	out := LanguagesOutput{Languages: []LanguageEntry{}}
	md := FormatLanguagesMarkdown(out)
	if !strings.Contains(md, "No languages detected") {
		t.Error("FormatLanguagesMarkdown should indicate no languages")
	}
}

// ---------------------------------------------------------------------------
// FormatListHooksMarkdown tests
// ---------------------------------------------------------------------------.

// TestFormatListHooksMarkdown_WithHooks verifies the behavior of format list hooks markdown with hooks.
func TestFormatListHooksMarkdown_WithHooks(t *testing.T) {
	out := ListHooksOutput{
		Hooks: []HookOutput{
			{ID: 1, URL: testHookURL, Name: testMyHook, PushEvents: true, MergeRequestsEvents: true, EnableSSLVerification: true},
			{ID: 2, URL: testHookURL2, PipelineEvents: true},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2},
	}
	md := FormatListHooksMarkdown(out)
	for _, want := range []string{"Project Webhooks (2)", "my-hook", testHookURL, testHookURL2} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatListHooksMarkdown missing %q", want)
		}
	}
}

// TestFormatListHooksMarkdown_Empty verifies the behavior of format list hooks markdown empty.
func TestFormatListHooksMarkdown_Empty(t *testing.T) {
	out := ListHooksOutput{
		Hooks:      []HookOutput{},
		Pagination: toolutil.PaginationOutput{TotalItems: 0},
	}
	md := FormatListHooksMarkdown(out)
	if !strings.Contains(md, "No webhooks found") {
		t.Error("FormatListHooksMarkdown should say no webhooks found for empty list")
	}
}

// TestFormatListHooksMarkdown_HookWithoutName verifies the behavior of format list hooks markdown hook without name.
func TestFormatListHooksMarkdown_HookWithoutName(t *testing.T) {
	out := ListHooksOutput{
		Hooks: []HookOutput{
			{ID: 5, URL: "https://example.com/hook3"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	}
	md := FormatListHooksMarkdown(out)
	// Empty name should be rendered as "-"
	if !strings.Contains(md, "-") {
		t.Error("FormatListHooksMarkdown should render empty name as dash")
	}
}

// ---------------------------------------------------------------------------
// FormatHookMarkdown tests
// ---------------------------------------------------------------------------.

// TestFormatHookMarkdown_WithName verifies the behavior of format hook markdown with name.
func TestFormatHookMarkdown_WithName(t *testing.T) {
	out := HookOutput{
		ID:                    1,
		URL:                   testHookURL,
		Name:                  "deploy-hook",
		PushEvents:            true,
		IssuesEvents:          true,
		MergeRequestsEvents:   false,
		EnableSSLVerification: true,
	}
	md := FormatHookMarkdown(out)
	for _, want := range []string{"Webhook #1", "deploy-hook", testHookURL, "SSL Verification", "Event Triggers", "Push", "Issues", "Merge Requests"} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatHookMarkdown missing %q", want)
		}
	}
}

// TestFormatHookMarkdown_WithoutName verifies the behavior of format hook markdown without name.
func TestFormatHookMarkdown_WithoutName(t *testing.T) {
	out := HookOutput{
		ID:  2,
		URL: testHookURL2,
	}
	md := FormatHookMarkdown(out)
	if md == "" {
		t.Fatal(errExpectedNonEmptyMD)
	}
	if strings.Contains(md, "**Name:**") {
		t.Error("FormatHookMarkdown should not contain Name field when name is empty")
	}
}

// ---------------------------------------------------------------------------
// FormatListProjectUsersMarkdown tests
// ---------------------------------------------------------------------------.

// TestFormatListProjectUsersMarkdown_WithUsers verifies the behavior of format list project users markdown with users.
func TestFormatListProjectUsersMarkdown_WithUsers(t *testing.T) {
	out := ListProjectUsersOutput{
		Users: []ProjectUserOutput{
			{ID: 1, Name: testAlice, Username: "alice", State: "active"},
			{ID: 2, Name: testBob, Username: "bob", State: "blocked"},
		},
	}
	md := FormatListProjectUsersMarkdown(out)
	for _, want := range []string{"Project Users (2)", testAlice, "@alice", "active", testBob, "@bob", "blocked"} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatListProjectUsersMarkdown missing %q", want)
		}
	}
}

// TestFormatListProjectUsersMarkdown_Empty verifies the behavior of format list project users markdown empty.
func TestFormatListProjectUsersMarkdown_Empty(t *testing.T) {
	out := ListProjectUsersOutput{Users: []ProjectUserOutput{}}
	md := FormatListProjectUsersMarkdown(out)
	if !strings.Contains(md, "No users found") {
		t.Error("FormatListProjectUsersMarkdown should say no users found")
	}
}

// ---------------------------------------------------------------------------
// FormatListProjectGroupsMarkdown tests
// ---------------------------------------------------------------------------.

// TestFormatListProjectGroupsMarkdown_WithGroups verifies the behavior of format list project groups markdown with groups.
func TestFormatListProjectGroupsMarkdown_WithGroups(t *testing.T) {
	out := ListProjectGroupsOutput{
		Groups: []ProjectGroupOutput{
			{ID: 1, Name: "group-a", FullPath: "org/group-a"},
			{ID: 2, Name: "group-b", FullPath: "org/group-b"},
		},
	}
	md := FormatListProjectGroupsMarkdown(out)
	for _, want := range []string{"Project Groups (2)", "group-a", "org/group-a", "group-b", "org/group-b"} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatListProjectGroupsMarkdown missing %q", want)
		}
	}
}

// TestFormatListProjectGroupsMarkdown_Empty verifies the behavior of format list project groups markdown empty.
func TestFormatListProjectGroupsMarkdown_Empty(t *testing.T) {
	out := ListProjectGroupsOutput{Groups: []ProjectGroupOutput{}}
	md := FormatListProjectGroupsMarkdown(out)
	if !strings.Contains(md, "No groups found") {
		t.Error("FormatListProjectGroupsMarkdown should say no groups found")
	}
}

// ---------------------------------------------------------------------------
// FormatListStarrersMarkdown tests
// ---------------------------------------------------------------------------.

// TestFormatListStarrersMarkdown_WithStarrers verifies the behavior of format list starrers markdown with starrers.
func TestFormatListStarrersMarkdown_WithStarrers(t *testing.T) {
	out := ListProjectStarrersOutput{
		Starrers: []StarrerOutput{
			{StarredSince: testDate20250101, User: ProjectUserOutput{ID: 1, Name: testAlice, Username: "alice"}},
			{StarredSince: "2025-02-01", User: ProjectUserOutput{ID: 2, Name: testBob, Username: "bob"}},
		},
	}
	md := FormatListStarrersMarkdown(out)
	for _, want := range []string{"Project Starrers (2)", testAlice, "@alice", "1 Jan 2025", testBob, "@bob", "1 Feb 2025"} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatListStarrersMarkdown missing %q", want)
		}
	}
}

// TestFormatListStarrersMarkdown_Empty verifies the behavior of format list starrers markdown empty.
func TestFormatListStarrersMarkdown_Empty(t *testing.T) {
	out := ListProjectStarrersOutput{Starrers: []StarrerOutput{}}
	md := FormatListStarrersMarkdown(out)
	if !strings.Contains(md, "No starrers found") {
		t.Error("FormatListStarrersMarkdown should say no starrers found")
	}
}

// ---------------------------------------------------------------------------
// buildCreateOpts — additional branch coverage for optional fields
// ---------------------------------------------------------------------------.

// TestBuildCreateOpts_AllOptionalFields verifies the behavior of build create opts all optional fields.
func TestBuildCreateOpts_AllOptionalFields(t *testing.T) {
	issuesEnabled := true
	mrEnabled := false
	wikiEnabled := true
	jobsEnabled := false
	lfsEnabled := true
	requestAccess := true
	allowSkipped := false
	removeBranch := true
	autoclose := true

	input := CreateInput{
		Name:                             "full-project",
		NamespaceID:                      99,
		Description:                      "desc",
		Visibility:                       "internal",
		InitializeWithReadme:             true,
		DefaultBranch:                    testBranchDevelop,
		Path:                             "full-project-path",
		Topics:                           []string{"go", "test"},
		MergeMethod:                      "rebase_merge",
		SquashOption:                     "always",
		OnlyAllowMergeIfPipelineSucceeds: true,
		OnlyAllowMergeIfAllDiscussionsAreResolved: true,
		IssuesEnabled:                &issuesEnabled,
		MergeRequestsEnabled:         &mrEnabled,
		WikiEnabled:                  &wikiEnabled,
		JobsEnabled:                  &jobsEnabled,
		LFSEnabled:                   &lfsEnabled,
		RequestAccessEnabled:         &requestAccess,
		CIConfigPath:                 ".gitlab-ci.yml",
		AllowMergeOnSkippedPipeline:  &allowSkipped,
		RemoveSourceBranchAfterMerge: &removeBranch,
		AutocloseReferencedIssues:    &autoclose,
	}

	opts := buildCreateOpts(input)
	assertCreateProjectOpts(t, opts)
}

// assertCreateProjectOpts is an internal helper for the projects package.
func assertCreateProjectOpts(t *testing.T, opts *gl.CreateProjectOptions) {
	t.Helper()
	assertCreateProjectCoreOpts(t, opts)
	assertCreateProjectFeatureOpts(t, opts)
}

// assertCreateProjectCoreOpts is an internal helper for the projects package.
func assertCreateProjectCoreOpts(t *testing.T, opts *gl.CreateProjectOptions) {
	t.Helper()
	if opts.Name == nil || *opts.Name != "full-project" {
		t.Error("Name not set")
	}
	if opts.NamespaceID == nil || *opts.NamespaceID != 99 {
		t.Error("NamespaceID not set")
	}
	if opts.Visibility == nil {
		t.Error(errVisibilityNotSet)
	}
	if opts.InitializeWithReadme == nil || !*opts.InitializeWithReadme {
		t.Error("InitializeWithReadme not set")
	}
	if opts.DefaultBranch == nil || *opts.DefaultBranch != testBranchDevelop {
		t.Error("DefaultBranch not set")
	}
	if opts.Path == nil || *opts.Path != "full-project-path" {
		t.Error("Path not set")
	}
	if opts.Topics == nil || len(*opts.Topics) != 2 {
		t.Error("Topics not set")
	}
	if opts.MergeMethod == nil {
		t.Error("MergeMethod not set")
	}
	if opts.SquashOption == nil {
		t.Error("SquashOption not set")
	}
}

// assertCreateProjectFeatureOpts is an internal helper for the projects package.
func assertCreateProjectFeatureOpts(t *testing.T, opts *gl.CreateProjectOptions) {
	t.Helper()
	assertCreateProjectMergeOpts(t, opts)
	assertCreateProjectToggleOpts(t, opts)
}

// assertCreateProjectMergeOpts is an internal helper for the projects package.
func assertCreateProjectMergeOpts(t *testing.T, opts *gl.CreateProjectOptions) {
	t.Helper()
	if opts.OnlyAllowMergeIfPipelineSucceeds == nil || !*opts.OnlyAllowMergeIfPipelineSucceeds {
		t.Error("OnlyAllowMergeIfPipelineSucceeds not set")
	}
	if opts.OnlyAllowMergeIfAllDiscussionsAreResolved == nil || !*opts.OnlyAllowMergeIfAllDiscussionsAreResolved {
		t.Error("OnlyAllowMergeIfAllDiscussionsAreResolved not set")
	}
	if opts.IssuesAccessLevel == nil || *opts.IssuesAccessLevel != gl.EnabledAccessControl {
		t.Error("IssuesAccessLevel not set")
	}
	if opts.MergeRequestsAccessLevel == nil || *opts.MergeRequestsAccessLevel != gl.DisabledAccessControl {
		t.Error("MergeRequestsAccessLevel not set correctly")
	}
	if opts.WikiAccessLevel == nil || *opts.WikiAccessLevel != gl.EnabledAccessControl {
		t.Error("WikiAccessLevel not set")
	}
	if opts.BuildsAccessLevel == nil || *opts.BuildsAccessLevel != gl.DisabledAccessControl {
		t.Error("BuildsAccessLevel not set correctly")
	}
}

// assertCreateProjectToggleOpts is an internal helper for the projects package.
func assertCreateProjectToggleOpts(t *testing.T, opts *gl.CreateProjectOptions) {
	t.Helper()
	if opts.LFSEnabled == nil || !*opts.LFSEnabled {
		t.Error("LFSEnabled not set")
	}
	if opts.RequestAccessEnabled == nil || !*opts.RequestAccessEnabled {
		t.Error("RequestAccessEnabled not set")
	}
	if opts.CIConfigPath == nil || *opts.CIConfigPath != ".gitlab-ci.yml" {
		t.Error("CIConfigPath not set")
	}
	if opts.AllowMergeOnSkippedPipeline == nil || *opts.AllowMergeOnSkippedPipeline {
		t.Error("AllowMergeOnSkippedPipeline not set correctly")
	}
	if opts.RemoveSourceBranchAfterMerge == nil || !*opts.RemoveSourceBranchAfterMerge {
		t.Error("RemoveSourceBranchAfterMerge not set")
	}
	if opts.AutocloseReferencedIssues == nil || !*opts.AutocloseReferencedIssues {
		t.Error("AutocloseReferencedIssues not set")
	}
}

// ---------------------------------------------------------------------------
// buildUpdateOpts + applyUpdateFeatureOpts — branch coverage
// ---------------------------------------------------------------------------.

// TestBuildUpdateOpts_AllFields verifies the behavior of build update opts all fields.
func TestBuildUpdateOpts_AllFields(t *testing.T) {
	pipelineSucceeds := true
	allDiscussions := true
	issuesOn := false
	mrOn := true
	wikiOn := false
	jobsOn := true
	allowSkipped := true
	removeBranch := false
	autoclose := true
	mergePipelines := true
	mergeTrains := false
	resolveOutdated := true

	input := UpdateInput{
		ProjectID:                        "42",
		Name:                             "updated",
		Description:                      "new desc",
		Visibility:                       testPublic,
		DefaultBranch:                    testBranchDevelop,
		MergeMethod:                      "ff",
		Topics:                           []string{"ci", "cd"},
		SquashOption:                     "default_on",
		OnlyAllowMergeIfPipelineSucceeds: &pipelineSucceeds,
		OnlyAllowMergeIfAllDiscussionsAreResolved: &allDiscussions,
		IssuesEnabled:                  &issuesOn,
		MergeRequestsEnabled:           &mrOn,
		WikiEnabled:                    &wikiOn,
		JobsEnabled:                    &jobsOn,
		CIConfigPath:                   "custom-ci.yml",
		AllowMergeOnSkippedPipeline:    &allowSkipped,
		RemoveSourceBranchAfterMerge:   &removeBranch,
		AutocloseReferencedIssues:      &autoclose,
		MergeCommitTemplate:            "Merge: %{title}",
		SquashCommitTemplate:           "Squash: %{title}",
		MergePipelinesEnabled:          &mergePipelines,
		MergeTrainsEnabled:             &mergeTrains,
		ResolveOutdatedDiffDiscussions: &resolveOutdated,
		ApprovalsBeforeMerge:           2,
		LFSEnabled:                     &issuesOn,
		RequestAccessEnabled:           &mrOn,
		SharedRunnersEnabled:           &wikiOn,
		PublicBuilds:                   &jobsOn,
		PackagesEnabled:                &pipelineSucceeds,
		PagesAccessLevel:               "enabled",
		ContainerRegistryAccessLevel:   "disabled",
		SnippetsAccessLevel:            "private",
	}

	opts := buildUpdateOpts(input)
	assertEditProjectOpts(t, opts)
}

// assertEditProjectOpts is an internal helper for the projects package.
func assertEditProjectOpts(t *testing.T, opts *gl.EditProjectOptions) {
	t.Helper()
	assertEditProjectCoreOpts(t, opts)
	assertEditProjectAdvancedOpts(t, opts)
}

// assertEditProjectCoreOpts is an internal helper for the projects package.
func assertEditProjectCoreOpts(t *testing.T, opts *gl.EditProjectOptions) {
	t.Helper()
	if opts.Name == nil || *opts.Name != "updated" {
		t.Error("Name not set")
	}
	if opts.Description == nil {
		t.Error("Description not set")
	}
	if opts.Visibility == nil {
		t.Error(errVisibilityNotSet)
	}
	if opts.DefaultBranch == nil || *opts.DefaultBranch != testBranchDevelop {
		t.Error("DefaultBranch not set")
	}
	if opts.MergeMethod == nil {
		t.Error("MergeMethod not set")
	}
	if opts.Topics == nil || len(*opts.Topics) != 2 {
		t.Error("Topics not set")
	}
	if opts.SquashOption == nil {
		t.Error("SquashOption not set")
	}
	if opts.OnlyAllowMergeIfPipelineSucceeds == nil {
		t.Error("OnlyAllowMergeIfPipelineSucceeds not set")
	}
	if opts.OnlyAllowMergeIfAllDiscussionsAreResolved == nil {
		t.Error("OnlyAllowMergeIfAllDiscussionsAreResolved not set")
	}
	if opts.IssuesAccessLevel == nil {
		t.Error("IssuesAccessLevel not set")
	}
	if opts.MergeRequestsAccessLevel == nil {
		t.Error("MergeRequestsAccessLevel not set")
	}
	if opts.WikiAccessLevel == nil {
		t.Error("WikiAccessLevel not set")
	}
}

// assertEditProjectAdvancedOpts is an internal helper for the projects package.
func assertEditProjectAdvancedOpts(t *testing.T, opts *gl.EditProjectOptions) {
	t.Helper()
	if opts.BuildsAccessLevel == nil {
		t.Error("BuildsAccessLevel not set")
	}
	if opts.CIConfigPath == nil || *opts.CIConfigPath != "custom-ci.yml" {
		t.Error("CIConfigPath not set")
	}
	if opts.AllowMergeOnSkippedPipeline == nil {
		t.Error("AllowMergeOnSkippedPipeline not set")
	}
	if opts.RemoveSourceBranchAfterMerge == nil {
		t.Error("RemoveSourceBranchAfterMerge not set")
	}
	if opts.AutocloseReferencedIssues == nil {
		t.Error("AutocloseReferencedIssues not set")
	}
	if opts.MergeCommitTemplate == nil || *opts.MergeCommitTemplate != "Merge: %{title}" {
		t.Error("MergeCommitTemplate not set")
	}
	if opts.SquashCommitTemplate == nil || *opts.SquashCommitTemplate != "Squash: %{title}" {
		t.Error("SquashCommitTemplate not set")
	}
	if opts.MergePipelinesEnabled == nil {
		t.Error("MergePipelinesEnabled not set")
	}
	if opts.MergeTrainsEnabled == nil {
		t.Error("MergeTrainsEnabled not set")
	}
	if opts.ResolveOutdatedDiffDiscussions == nil {
		t.Error("ResolveOutdatedDiffDiscussions not set")
	}
	//lint:ignore SA1019 no replacement field, needs Merge Request Approvals API
	if opts.ApprovalsBeforeMerge == nil || *opts.ApprovalsBeforeMerge != 2 { //nolint:staticcheck // testing deprecated field intentionally
		t.Error("ApprovalsBeforeMerge not set")
	}
	if opts.LFSEnabled == nil {
		t.Error("LFSEnabled not set")
	}
	if opts.RequestAccessEnabled == nil {
		t.Error("RequestAccessEnabled not set")
	}
	if opts.SharedRunnersEnabled == nil {
		t.Error("SharedRunnersEnabled not set")
	}
	if opts.PublicJobs == nil {
		t.Error("PublicJobs not set")
	}
	if opts.PackagesEnabled == nil {
		t.Error("PackagesEnabled not set")
	}
	if opts.PagesAccessLevel == nil {
		t.Error("PagesAccessLevel not set")
	}
	if opts.ContainerRegistryAccessLevel == nil {
		t.Error("ContainerRegistryAccessLevel not set")
	}
	if opts.SnippetsAccessLevel == nil {
		t.Error("SnippetsAccessLevel not set")
	}
}

// ---------------------------------------------------------------------------
// Fork — additional branch coverage for optional fields
// ---------------------------------------------------------------------------.

// TestProjectFork_AllOptionalFields verifies the behavior of project fork all optional fields.
func TestProjectFork_AllOptionalFields(t *testing.T) {
	const projectJSON = `{"id":99,"name":"forked","path_with_namespace":"user/forked","visibility":"private","default_branch":"main","web_url":"https://gitlab.example.com/user/forked","description":"forked desc"}`
	mrTarget := true
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProject42Fork {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("failed to decode request body: %v", err)
			}
			assertForkBody(t, body)
			testutil.RespondJSON(w, http.StatusCreated, projectJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Fork(context.Background(), client, ForkInput{
		ProjectID:                     "42",
		Name:                          "forked",
		Path:                          "forked-path",
		NamespaceID:                   10,
		NamespacePath:                 "user",
		Description:                   "forked desc",
		Visibility:                    testPrivate,
		Branches:                      "main",
		MergeRequestDefaultTargetSelf: &mrTarget,
	})
	if err != nil {
		t.Fatalf("Fork() unexpected error: %v", err)
	}
	if out.Name != "forked" {
		t.Errorf("out.Name = %q, want forked", out.Name)
	}
}

// assertForkBody is an internal helper for the projects package.
func assertForkBody(t *testing.T, body map[string]any) {
	t.Helper()
	if body["name"] != "forked" {
		t.Errorf("name = %v, want forked", body["name"])
	}
	if body["path"] != "forked-path" {
		t.Errorf("path = %v, want forked-path", body["path"])
	}
	if body["description"] != "forked desc" {
		t.Errorf("description = %v, want forked desc", body["description"])
	}
	if body["visibility"] != testPrivate {
		t.Errorf("visibility = %v, want private", body["visibility"])
	}
	if body["branches"] != "main" {
		t.Errorf("branches = %v, want main", body["branches"])
	}
}

// ---------------------------------------------------------------------------
// ListForks — additional branch coverage for optional fields
// ---------------------------------------------------------------------------.

// TestProjectListForks_AllOptionalFields verifies the behavior of project list forks all optional fields.
func TestProjectListForks_AllOptionalFields(t *testing.T) {
	const forksJSON = `[{"id":10,"name":"fork-a","path_with_namespace":"u/fork-a","visibility":"public","default_branch":"main","web_url":"https://gitlab.example.com/u/fork-a","description":""}]`
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProject42Forks {
			assertListForksQuery(t, r)
			testutil.RespondJSON(w, http.StatusOK, forksJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListForks(context.Background(), client, ListForksInput{
		ProjectID:  "42",
		Owned:      true,
		Search:     "fork",
		Visibility: testPublic,
		OrderBy:    "name",
		Sort:       testSortAsc,
	})
	if err != nil {
		t.Fatalf("ListForks() unexpected error: %v", err)
	}
	if len(out.Forks) != 1 {
		t.Fatalf("expected 1 fork, got %d", len(out.Forks))
	}
}

// assertListForksQuery is an internal helper for the projects package.
func assertListForksQuery(t *testing.T, r *http.Request) {
	t.Helper()
	q := r.URL.Query()
	if q.Get("owned") != "true" {
		t.Error("expected owned=true")
	}
	if q.Get("search") != "fork" {
		t.Error("expected search=fork")
	}
	if q.Get("visibility") != testPublic {
		t.Error("expected visibility=public")
	}
	if q.Get("order_by") != "name" {
		t.Error("expected order_by=name")
	}
	if q.Get("sort") != testSortAsc {
		t.Error("expected sort=asc")
	}
}

// ---------------------------------------------------------------------------
// AddHook — additional branch coverage for all optional event booleans
// ---------------------------------------------------------------------------.

// TestProjectAddHook_AllOptionalEvents verifies the behavior of project add hook all optional events.
func TestProjectAddHook_AllOptionalEvents(t *testing.T) {
	const hookJSON = `{"id":1,"url":"https://example.com/hook","project_id":42,"push_events":true,"issues_events":true,"merge_requests_events":true,"tag_push_events":true,"note_events":true,"confidential_note_events":true,"job_events":true,"pipeline_events":true,"wiki_page_events":true,"deployment_events":true,"releases_events":true,"confidential_issues_events":true,"emoji_events":true,"resource_access_token_events":true,"enable_ssl_verification":false}`
	pushEvents := true
	issuesEvents := true
	confidentialIssues := true
	mrEvents := true
	tagPush := true
	noteEvents := true
	confidentialNote := true
	jobEvents := true
	pipelineEvents := true
	wikiEvents := true
	deploymentEvents := true
	releasesEvents := true
	emojiEvents := true
	resourceTokenEvents := true
	sslVerify := false

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProject42Hooks {
			testutil.RespondJSON(w, http.StatusCreated, hookJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := AddHook(context.Background(), client, AddHookInput{
		ProjectID:                 "42",
		URL:                       testHookURL,
		Name:                      testHookName,
		Description:               "A test webhook",
		Token:                     "secret",
		PushEvents:                &pushEvents,
		PushEventsBranchFilter:    "main",
		IssuesEvents:              &issuesEvents,
		ConfidentialIssuesEvents:  &confidentialIssues,
		MergeRequestsEvents:       &mrEvents,
		TagPushEvents:             &tagPush,
		NoteEvents:                &noteEvents,
		ConfidentialNoteEvents:    &confidentialNote,
		JobEvents:                 &jobEvents,
		PipelineEvents:            &pipelineEvents,
		WikiPageEvents:            &wikiEvents,
		DeploymentEvents:          &deploymentEvents,
		ReleasesEvents:            &releasesEvents,
		EmojiEvents:               &emojiEvents,
		ResourceAccessTokenEvents: &resourceTokenEvents,
		EnableSSLVerification:     &sslVerify,
		CustomWebhookTemplate:     `{"text":"{{event}}"}`,
		BranchFilterStrategy:      "wildcard",
	})
	if err != nil {
		t.Fatalf("AddHook() unexpected error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("out.ID = %d, want 1", out.ID)
	}
}

// ---------------------------------------------------------------------------
// EditHook — additional branch coverage for all optional event booleans
// ---------------------------------------------------------------------------.

// TestProjectEditHook_AllOptionalEvents verifies the behavior of project edit hook all optional events.
func TestProjectEditHook_AllOptionalEvents(t *testing.T) {
	const hookJSON = `{"id":5,"url":"https://example.com/updated","project_id":42,"push_events":false,"issues_events":false,"merge_requests_events":false,"tag_push_events":false,"note_events":false,"confidential_note_events":false,"job_events":false,"pipeline_events":false,"wiki_page_events":false,"deployment_events":false,"releases_events":false,"confidential_issues_events":false,"emoji_events":false,"resource_access_token_events":false,"enable_ssl_verification":true}`
	pushEvents := false
	issuesEvents := false
	confidentialIssues := false
	mrEvents := false
	tagPush := false
	noteEvents := false
	confidentialNote := false
	jobEvents := false
	pipelineEvents := false
	wikiEvents := false
	deploymentEvents := false
	releasesEvents := false
	emojiEvents := false
	resourceTokenEvents := false
	sslVerify := true

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/projects/42/hooks/5" {
			testutil.RespondJSON(w, http.StatusOK, hookJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := EditHook(context.Background(), client, EditHookInput{
		ProjectID:                 "42",
		HookID:                    5,
		URL:                       "https://example.com/updated",
		Name:                      "updated-hook",
		Description:               "Updated hook",
		Token:                     "new-secret",
		PushEvents:                &pushEvents,
		PushEventsBranchFilter:    testBranchDevelop,
		IssuesEvents:              &issuesEvents,
		ConfidentialIssuesEvents:  &confidentialIssues,
		MergeRequestsEvents:       &mrEvents,
		TagPushEvents:             &tagPush,
		NoteEvents:                &noteEvents,
		ConfidentialNoteEvents:    &confidentialNote,
		JobEvents:                 &jobEvents,
		PipelineEvents:            &pipelineEvents,
		WikiPageEvents:            &wikiEvents,
		DeploymentEvents:          &deploymentEvents,
		ReleasesEvents:            &releasesEvents,
		EmojiEvents:               &emojiEvents,
		ResourceAccessTokenEvents: &resourceTokenEvents,
		EnableSSLVerification:     &sslVerify,
		CustomWebhookTemplate:     `{"text":"updated"}`,
		BranchFilterStrategy:      "regex",
	})
	if err != nil {
		t.Fatalf("EditHook() unexpected error: %v", err)
	}
	if out.ID != 5 {
		t.Errorf("out.ID = %d, want 5", out.ID)
	}
}

// ---------------------------------------------------------------------------
// buildUserProjectOpts — branch coverage for all option fields
// ---------------------------------------------------------------------------.

// TestBuildUserProjectOpts_AllFields verifies the behavior of build user project opts all fields.
func TestBuildUserProjectOpts_AllFields(t *testing.T) {
	archived := true
	opts := buildUserProjectOpts(userProjectFilter{
		Search: "query", Visibility: testPrivate, Archived: &archived,
		OrderBy: "name", Sort: "desc", Simple: true,
		Page: 2, PerPage: 25,
	})

	if opts.Search == nil || *opts.Search != "query" {
		t.Error("Search not set")
	}
	if opts.Visibility == nil {
		t.Error(errVisibilityNotSet)
	}
	if opts.Archived == nil || !*opts.Archived {
		t.Error("Archived not set")
	}
	if opts.OrderBy == nil || *opts.OrderBy != "name" {
		t.Error("OrderBy not set")
	}
	if opts.Sort == nil || *opts.Sort != "desc" {
		t.Error("Sort not set")
	}
	if opts.Simple == nil || !*opts.Simple {
		t.Error("Simple not set")
	}
	if opts.Page != 2 {
		t.Errorf("Page = %d, want 2", opts.Page)
	}
	if opts.PerPage != 25 {
		t.Errorf("PerPage = %d, want 25", opts.PerPage)
	}
}

// TestBuildUserProjectOpts_NoFields verifies the behavior of build user project opts no fields.
func TestBuildUserProjectOpts_NoFields(t *testing.T) {
	opts := buildUserProjectOpts(userProjectFilter{})
	if opts.Search != nil {
		t.Error("Search should be nil")
	}
	if opts.Visibility != nil {
		t.Error("Visibility should be nil")
	}
	if opts.Archived != nil {
		t.Error("Archived should be nil")
	}
	if opts.OrderBy != nil {
		t.Error("OrderBy should be nil")
	}
	if opts.Sort != nil {
		t.Error("Sort should be nil")
	}
	if opts.Simple != nil {
		t.Error("Simple should be nil")
	}
}

// ---------------------------------------------------------------------------
// RegisterTools — ensures registration doesn't panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client) // should not panic
}

// ---------------------------------------------------------------------------
// Context cancellation tests (covers ctx.Err() branches)
// ---------------------------------------------------------------------------.

// canceledCtx is an internal helper for the projects package.
func canceledCtx() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// TestGet_CtxCanceled verifies the behavior of get ctx canceled.
func TestGet_CtxCanceled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := Get(canceledCtx(), client, GetInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedCtxErr)
	}
}

// TestUpdate_CtxCanceled verifies the behavior of update ctx canceled.
func TestUpdate_CtxCanceled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := Update(canceledCtx(), client, UpdateInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedCtxErr)
	}
}

// TestList_CtxCanceled verifies the behavior of list ctx canceled.
func TestList_CtxCanceled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := List(canceledCtx(), client, ListInput{})
	if err == nil {
		t.Fatal(errExpectedCtxErr)
	}
}

// TestStar_CtxCanceled verifies the behavior of star ctx canceled.
func TestStar_CtxCanceled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := Star(canceledCtx(), client, StarInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedCtxErr)
	}
}

// TestUnstar_CtxCanceled verifies the behavior of unstar ctx canceled.
func TestUnstar_CtxCanceled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := Unstar(canceledCtx(), client, UnstarInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedCtxErr)
	}
}

// TestArchive_CtxCanceled verifies the behavior of archive ctx canceled.
func TestArchive_CtxCanceled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := Archive(canceledCtx(), client, ArchiveInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedCtxErr)
	}
}

// TestUnarchive_CtxCanceled verifies the behavior of unarchive ctx canceled.
func TestUnarchive_CtxCanceled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := Unarchive(canceledCtx(), client, UnarchiveInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedCtxErr)
	}
}

// ---------------------------------------------------------------------------
// AddPushRule with ALL optional fields set (covers all branches)
// ---------------------------------------------------------------------------.

// TestAddPushRule_AllFields verifies the behavior of add push rule all fields.
func TestAddPushRule_AllFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathPushRules42 {
			testutil.RespondJSON(w, http.StatusCreated, pushRuleJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := AddPushRule(context.Background(), client, AddPushRuleInput{
		ProjectID:                  "42",
		AuthorEmailRegex:           "@company.com$",
		BranchNameRegex:            "^(main|release/.*)$",
		CommitCommitterCheck:       new(true),
		CommitCommitterNameCheck:   new(false),
		CommitMessageNegativeRegex: "WIP",
		CommitMessageRegex:         "^(feat|fix):",
		DenyDeleteTag:              new(true),
		FileNameRegex:              "\\.(exe|dll)$",
		MaxFileSize:                int64Ptr(50),
		MemberCheck:                new(true),
		PreventSecrets:             new(true),
		RejectUnsignedCommits:      new(false),
		RejectNonDCOCommits:        new(false),
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 1 {
		t.Errorf(fmtIDWant1, out.ID)
	}
}

// ---------------------------------------------------------------------------
// EditPushRule with ALL optional fields set (covers all branches)
// ---------------------------------------------------------------------------.

// TestEditPushRule_AllFields verifies the behavior of edit push rule all fields.
func TestEditPushRule_AllFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathPushRules42 {
			testutil.RespondJSON(w, http.StatusOK, pushRuleJSON)
			return
		}
		http.NotFound(w, r)
	}))
	s := func(v string) *string { return &v }
	i := func(v int64) *int64 { return &v }
	out, err := EditPushRule(context.Background(), client, EditPushRuleInput{
		ProjectID:                  "42",
		AuthorEmailRegex:           s("@newdomain.com$"),
		BranchNameRegex:            s("^release/"),
		CommitCommitterCheck:       new(true),
		CommitCommitterNameCheck:   new(true),
		CommitMessageNegativeRegex: s("DO NOT MERGE"),
		CommitMessageRegex:         s("^(feat|fix|docs):"),
		DenyDeleteTag:              new(false),
		FileNameRegex:              s("\\.(bat|sh)$"),
		MaxFileSize:                i(100),
		MemberCheck:                new(false),
		PreventSecrets:             new(true),
		RejectUnsignedCommits:      new(true),
		RejectNonDCOCommits:        new(true),
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 1 {
		t.Errorf(fmtIDWant1, out.ID)
	}
}

// ---------------------------------------------------------------------------
// Get with optional params (Statistics, License, WithCustomAttributes)
// ---------------------------------------------------------------------------.

// TestGet_WithOptions verifies the behavior of get with options.
func TestGet_WithOptions(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathProject42 {
			testutil.RespondJSON(w, http.StatusOK, `{"id":42,"name":"test","visibility":"public"}`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Get(context.Background(), client, GetInput{
		ProjectID:            "42",
		Statistics:           new(true),
		License:              new(true),
		WithCustomAttributes: new(true),
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 42 {
		t.Errorf("ID = %d, want 42", out.ID)
	}
}

// ---------------------------------------------------------------------------
// ListProjectGroups with all filter options
// ---------------------------------------------------------------------------.

// TestListProjectGroups_AllFilters verifies the behavior of list project groups all filters.
func TestListProjectGroups_AllFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/10/groups" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"name":"grp","avatar_url":"","web_url":"https://g","full_name":"Group","full_path":"grp"}]`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := ListProjectGroups(context.Background(), client, ListProjectGroupsInput{
		ProjectID:            "10",
		Search:               "grp",
		WithShared:           new(true),
		SharedVisibleOnly:    new(false),
		SkipGroups:           []int64{99, 100},
		SharedMinAccessLevel: 30,
		PaginationInput:      toolutil.PaginationInput{Page: 1, PerPage: 20},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Groups) != 1 {
		t.Fatalf(fmtLenGroupsWant1, len(out.Groups))
	}
}

// ---------------------------------------------------------------------------
// ListInvitedGroups with all filter options
// ---------------------------------------------------------------------------.

// TestListInvitedGroups_AllFilters verifies the behavior of list invited groups all filters.
func TestListInvitedGroups_AllFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/10/invited_groups" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":2,"name":"invited","avatar_url":"","web_url":"https://g","full_name":"Invited","full_path":"invited"}]`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := ListInvitedGroups(context.Background(), client, ListInvitedGroupsInput{
		ProjectID:       "10",
		Search:          "invited",
		MinAccessLevel:  20,
		PaginationInput: toolutil.PaginationInput{Page: 1, PerPage: 20},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Groups) != 1 {
		t.Fatalf(fmtLenGroupsWant1, len(out.Groups))
	}
}

// ---------------------------------------------------------------------------
// ListUserProjects with all filter options
// ---------------------------------------------------------------------------.

// TestListUserProjects_AllFilters verifies the behavior of list user projects all filters.
func TestListUserProjects_AllFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/users/john/projects" {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":5,"name":"proj","visibility":"public"}]`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := ListUserProjects(context.Background(), client, ListUserProjectsInput{
		UserID:          testUserJohn,
		Search:          "proj",
		Visibility:      testPublic,
		Archived:        new(false),
		OrderBy:         "name",
		Sort:            testSortAsc,
		Simple:          true,
		PaginationInput: toolutil.PaginationInput{Page: 1, PerPage: 10},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Projects) != 1 {
		t.Fatalf(fmtLenProjectsWant1, len(out.Projects))
	}
}

// ---------------------------------------------------------------------------
// buildListOpts + applyListFilterOpts with all branches
// ---------------------------------------------------------------------------.

// TestBuildListOpts_AllBranches verifies the behavior of build list opts all branches.
func TestBuildListOpts_AllBranches(t *testing.T) {
	input := ListInput{
		Owned:                    true,
		Search:                   "test",
		Visibility:               testPrivate,
		Archived:                 new(true),
		OrderBy:                  "created_at",
		Sort:                     "desc",
		Topic:                    "go",
		Simple:                   true,
		MinAccessLevel:           30,
		LastActivityAfter:        testDate20250101,
		LastActivityBefore:       "2025-12-31",
		Starred:                  new(true),
		Membership:               new(true),
		WithIssuesEnabled:        new(true),
		WithMergeRequestsEnabled: new(true),
		SearchNamespaces:         new(true),
		Statistics:               new(true),
		WithProgrammingLanguage:  "Go",
		IncludePendingDelete:     new(true),
		IncludeHidden:            new(true),
		IDAfter:                  100,
		IDBefore:                 500,
	}
	input.Page = 2
	input.PerPage = 50

	opts := buildListOpts(input)
	assertListProjectOpts(t, opts)
}

// assertListProjectOpts is an internal helper for the projects package.
func assertListProjectOpts(t *testing.T, opts *gl.ListProjectsOptions) {
	t.Helper()
	assertListProjectCoreOpts(t, opts)
	assertListProjectFilterOpts(t, opts)
}

// assertListProjectCoreOpts is an internal helper for the projects package.
func assertListProjectCoreOpts(t *testing.T, opts *gl.ListProjectsOptions) {
	t.Helper()
	assertListProjectSearchOpts(t, opts)
	assertListProjectDisplayOpts(t, opts)
}

// assertListProjectSearchOpts is an internal helper for the projects package.
func assertListProjectSearchOpts(t *testing.T, opts *gl.ListProjectsOptions) {
	t.Helper()
	if opts.Owned == nil || !*opts.Owned {
		t.Error("Owned not set")
	}
	if opts.Search == nil || *opts.Search != "test" {
		t.Error("Search not set")
	}
	if opts.Visibility == nil {
		t.Error(errVisibilityNotSet)
	}
	if opts.Archived == nil {
		t.Error("Archived not set")
	}
	if opts.OrderBy == nil || *opts.OrderBy != "created_at" {
		t.Error("OrderBy not set")
	}
	if opts.Sort == nil || *opts.Sort != "desc" {
		t.Error("Sort not set")
	}
}

// assertListProjectDisplayOpts is an internal helper for the projects package.
func assertListProjectDisplayOpts(t *testing.T, opts *gl.ListProjectsOptions) {
	t.Helper()
	if opts.Topic == nil || *opts.Topic != "go" {
		t.Error("Topic not set")
	}
	if opts.Simple == nil || !*opts.Simple {
		t.Error("Simple not set")
	}
	if opts.MinAccessLevel == nil {
		t.Error("MinAccessLevel not set")
	}
	if opts.Starred == nil || !*opts.Starred {
		t.Error("Starred not set")
	}
	if opts.Membership == nil || !*opts.Membership {
		t.Error("Membership not set")
	}
}

// assertListProjectFilterOpts is an internal helper for the projects package.
func assertListProjectFilterOpts(t *testing.T, opts *gl.ListProjectsOptions) {
	t.Helper()
	assertListProjectFeatureFilterOpts(t, opts)
	assertListProjectPaginationFilterOpts(t, opts)
}

// assertListProjectFeatureFilterOpts is an internal helper for the projects package.
func assertListProjectFeatureFilterOpts(t *testing.T, opts *gl.ListProjectsOptions) {
	t.Helper()
	if opts.WithIssuesEnabled == nil || !*opts.WithIssuesEnabled {
		t.Error("WithIssuesEnabled not set")
	}
	if opts.WithMergeRequestsEnabled == nil || !*opts.WithMergeRequestsEnabled {
		t.Error("WithMergeRequestsEnabled not set")
	}
	if opts.SearchNamespaces == nil || !*opts.SearchNamespaces {
		t.Error("SearchNamespaces not set")
	}
	if opts.Statistics == nil || !*opts.Statistics {
		t.Error("Statistics not set")
	}
	if opts.WithProgrammingLanguage == nil || *opts.WithProgrammingLanguage != "Go" {
		t.Error("WithProgrammingLanguage not set")
	}
}

// assertListProjectPaginationFilterOpts is an internal helper for the projects package.
func assertListProjectPaginationFilterOpts(t *testing.T, opts *gl.ListProjectsOptions) {
	t.Helper()
	if opts.IncludePendingDelete == nil || !*opts.IncludePendingDelete {
		t.Error("IncludePendingDelete not set")
	}
	if opts.IncludeHidden == nil || !*opts.IncludeHidden {
		t.Error("IncludeHidden not set")
	}
	if opts.IDAfter == nil || *opts.IDAfter != 100 {
		t.Error("IDAfter not set")
	}
	if opts.IDBefore == nil || *opts.IDBefore != 500 {
		t.Error("IDBefore not set")
	}
	if opts.Page != 2 {
		t.Error("Page not set")
	}
	if opts.PerPage != 50 {
		t.Error("PerPage not set")
	}
}

// ---------------------------------------------------------------------------
// ToOutput with Namespace (covers the namespace branch)
// ---------------------------------------------------------------------------.

// TestToOutput_WithNamespace verifies the behavior of to output with namespace.
func TestToOutput_WithNamespace(t *testing.T) {
	p := &gl.Project{
		ID:         1,
		Name:       "test",
		Visibility: gl.PublicVisibility,
		Namespace:  &gl.ProjectNamespace{FullPath: testMyGroup},
	}
	out := ToOutput(p)
	if out.Namespace != testMyGroup {
		t.Errorf("Namespace = %q, want %q", out.Namespace, testMyGroup)
	}
}

// TestToOutput_WithCreatedAt verifies the behavior of to output with created at.
func TestToOutput_WithCreatedAt(t *testing.T) {
	now := time.Now()
	p := &gl.Project{
		ID:        1,
		Name:      "test",
		CreatedAt: &now,
	}
	out := ToOutput(p)
	if out.CreatedAt == "" {
		t.Error("expected CreatedAt to be set")
	}
}

// ---------------------------------------------------------------------------
// API error path tests (covers err != nil after API calls)
// ---------------------------------------------------------------------------.

// errMockHandler is an internal helper for the projects package.
func errMockHandler() http.Handler {
	// Return 404 (not 500) to avoid triggering GitLab client retry logic
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	})
}

// TestUpdate_APIError verifies the behavior of update a p i error.
func TestUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, errMockHandler())
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "1", Name: "x"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestTransfer_APIError verifies the behavior of transfer a p i error.
func TestTransfer_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, errMockHandler())
	_, err := Transfer(context.Background(), client, TransferInput{ProjectID: "1", Namespace: "ns"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestTransfer_EmptyNamespace verifies the behavior of transfer empty namespace.
func TestTransfer_EmptyNamespace(t *testing.T) {
	client := testutil.NewTestClient(t, errMockHandler())
	_, err := Transfer(context.Background(), client, TransferInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected validation error for empty namespace")
	}
}

// TestGetLanguages_APIError verifies the behavior of get languages a p i error.
func TestGetLanguages_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, errMockHandler())
	_, err := GetLanguages(context.Background(), client, GetLanguagesInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGetLanguages_EmptyProjectID verifies the behavior of get languages empty project i d.
func TestGetLanguages_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, errMockHandler())
	_, err := GetLanguages(context.Background(), client, GetLanguagesInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestListHooks_APIError verifies the behavior of list hooks a p i error.
func TestListHooks_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, errMockHandler())
	_, err := ListHooks(context.Background(), client, ListHooksInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGetHook_APIError verifies the behavior of get hook a p i error.
func TestGetHook_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, errMockHandler())
	_, err := GetHook(context.Background(), client, GetHookInput{ProjectID: "1", HookID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGetHook_EmptyProject verifies the behavior of get hook empty project.
func TestGetHook_EmptyProject(t *testing.T) {
	client := testutil.NewTestClient(t, errMockHandler())
	_, err := GetHook(context.Background(), client, GetHookInput{HookID: 1})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestDeleteHook_APIError verifies the behavior of delete hook a p i error.
func TestDeleteHook_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, errMockHandler())
	err := DeleteHook(context.Background(), client, DeleteHookInput{ProjectID: "1", HookID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDeleteHook_CtxCanceled verifies the behavior of delete hook ctx canceled.
func TestDeleteHook_CtxCanceled(t *testing.T) {
	client := testutil.NewTestClient(t, errMockHandler())
	err := DeleteHook(canceledCtx(), client, DeleteHookInput{ProjectID: "1", HookID: 1})
	if err == nil {
		t.Fatal(errExpectedCtxErr)
	}
}

// TestTriggerTestHook_Success verifies the behavior of trigger test hook success.
func TestTriggerTestHook_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := TriggerTestHook(context.Background(), client, TriggerTestHookInput{
		ProjectID: "1",
		HookID:    10,
		Event:     "push_events",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !strings.Contains(out.Message, "push_events") {
		t.Errorf("expected message to contain 'push_events', got %q", out.Message)
	}
}

// TestTriggerTestHook_CtxCanceled verifies the behavior of trigger test hook ctx canceled.
func TestTriggerTestHook_CtxCanceled(t *testing.T) {
	client := testutil.NewTestClient(t, errMockHandler())
	_, err := TriggerTestHook(canceledCtx(), client, TriggerTestHookInput{ProjectID: "1", HookID: 1, Event: "push_events"})
	if err == nil {
		t.Fatal(errExpectedCtxErr)
	}
}

// TestTriggerTestHook_EmptyEvent verifies the behavior of trigger test hook empty event.
func TestTriggerTestHook_EmptyEvent(t *testing.T) {
	client := testutil.NewTestClient(t, errMockHandler())
	_, err := TriggerTestHook(context.Background(), client, TriggerTestHookInput{ProjectID: "1", HookID: 1})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestTriggerTestHook_EmptyHookID verifies the behavior of trigger test hook empty hook i d.
func TestTriggerTestHook_EmptyHookID(t *testing.T) {
	client := testutil.NewTestClient(t, errMockHandler())
	_, err := TriggerTestHook(context.Background(), client, TriggerTestHookInput{ProjectID: "1", Event: "push_events"})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestListProjectUsers_WithFilters verifies the behavior of list project users with filters.
func TestListProjectUsers_WithFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("search") != testUserJohn {
			t.Errorf("search = %q, want %q", q.Get("search"), testUserJohn)
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"username":"john","name":"John","state":"active","web_url":"https://g/john"}]`)
	}))
	out, err := ListProjectUsers(context.Background(), client, ListProjectUsersInput{
		ProjectID:       "10",
		Search:          testUserJohn,
		PaginationInput: toolutil.PaginationInput{Page: 1, PerPage: 20},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Users) != 1 {
		t.Fatalf("len(Users) = %d, want 1", len(out.Users))
	}
}

// TestListProjectUsers_APIError verifies the behavior of list project users a p i error.
func TestListProjectUsers_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, errMockHandler())
	_, err := ListProjectUsers(context.Background(), client, ListProjectUsersInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestListProjectStarrers_WithFilters verifies the behavior of list project starrers with filters.
func TestListProjectStarrers_WithFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"starred_since":"2025-01-01T00:00:00Z","user":{"id":1,"username":"alice","name":"Alice","state":"active","web_url":"https://g/alice"}}]`)
	}))
	out, err := ListProjectStarrers(context.Background(), client, ListProjectStarrersInput{
		ProjectID:       "10",
		Search:          "alice",
		PaginationInput: toolutil.PaginationInput{Page: 1, PerPage: 20},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Starrers) != 1 {
		t.Fatalf("len(Starrers) = %d, want 1", len(out.Starrers))
	}
}

// TestListProjectStarrers_APIError verifies the behavior of list project starrers a p i error.
func TestListProjectStarrers_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, errMockHandler())
	_, err := ListProjectStarrers(context.Background(), client, ListProjectStarrersInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestShareProject_APIError verifies the behavior of share project a p i error.
func TestShareProject_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, errMockHandler())
	_, err := ShareProjectWithGroup(context.Background(), client, ShareProjectInput{
		ProjectID: "1", GroupID: 10, GroupAccess: 30,
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDelete_SharedGroupAPIError verifies the behavior of delete shared group a p i error.
func TestDelete_SharedGroupAPIError(t *testing.T) {
	client := testutil.NewTestClient(t, errMockHandler())
	err := DeleteSharedProjectFromGroup(context.Background(), client, DeleteSharedGroupInput{ProjectID: "1", GroupID: 10})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDeletePushRule_APIError verifies the behavior of delete push rule a p i error.
func TestDeletePushRule_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, errMockHandler())
	err := DeletePushRule(context.Background(), client, DeletePushRuleInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestRestore_APIError verifies the behavior of restore a p i error.
func TestRestore_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, errMockHandler())
	_, err := Restore(context.Background(), client, RestoreInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestRestore_EmptyProjectID verifies the behavior of restore empty project i d.
func TestRestore_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, errMockHandler())
	_, err := Restore(context.Background(), client, RestoreInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestStar_APIError verifies the behavior of star a p i error.
func TestStar_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, errMockHandler())
	_, err := Star(context.Background(), client, StarInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestUnstar_APIError verifies the behavior of unstar a p i error.
func TestUnstar_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, errMockHandler())
	_, err := Unstar(context.Background(), client, UnstarInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestArchive_APIError verifies the behavior of archive a p i error.
func TestArchive_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, errMockHandler())
	_, err := Archive(context.Background(), client, ArchiveInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestUnarchive_APIError verifies the behavior of unarchive a p i error.
func TestUnarchive_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, errMockHandler())
	_, err := Unarchive(context.Background(), client, UnarchiveInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestListProjectGroups_APIError verifies the behavior of list project groups a p i error.
func TestListProjectGroups_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, errMockHandler())
	_, err := ListProjectGroups(context.Background(), client, ListProjectGroupsInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestListInvitedGroups_APIError verifies the behavior of list invited groups a p i error.
func TestListInvitedGroups_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, errMockHandler())
	_, err := ListInvitedGroups(context.Background(), client, ListInvitedGroupsInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// MCP integration tests — exercise RegisterTools handler closures
// ---------------------------------------------------------------------------.

const projectJSON = `{"id":42,"name":"test","path_with_namespace":"g/test","visibility":"private","default_branch":"main","web_url":"https://example.com","description":"desc","merge_request_title_regex":"^(feat|fix):","merge_request_title_regex_description":"MR title must start with feat: or fix:"}`
const hookJSON42 = `{"id":1,"url":"https://example.com/hook","project_id":42,"push_events":true,"created_at":"2024-01-01T00:00:00Z"}`
const pushRuleJSON42 = `{"id":1,"commit_message_regex":".*","branch_name_regex":".*","max_file_size":100}`

// mockRoute holds a canned HTTP response for a test mock endpoint.
type mockRoute struct {
	status int
	body   string
}

// mcpMockHandler returns an HTTP handler that responds with valid JSON for all
// project-related GitLab API endpoints used by the 33 registered tools.
// Routes are organized as a map to keep cognitive complexity low.
func mcpMockHandler() http.Handler {
	// Method-specific routes: key = "METHOD /path"
	methodRoutes := map[string]mockRoute{
		// Push rules
		"GET /api/v4/projects/42/push_rule":    {http.StatusOK, pushRuleJSON42},
		"POST /api/v4/projects/42/push_rule":   {http.StatusCreated, pushRuleJSON42},
		"PUT /api/v4/projects/42/push_rule":    {http.StatusOK, pushRuleJSON42},
		"DELETE /api/v4/projects/42/push_rule": {http.StatusNoContent, ""},
		// Hooks
		"GET /api/v4/projects/42/hooks":      {http.StatusOK, "[]"},
		"GET /api/v4/projects/42/hooks/1":    {http.StatusOK, hookJSON42},
		"POST /api/v4/projects/42/hooks":     {http.StatusCreated, hookJSON42},
		"PUT /api/v4/projects/42/hooks/1":    {http.StatusOK, hookJSON42},
		"DELETE /api/v4/projects/42/hooks/1": {http.StatusNoContent, ""},
		// Share
		"POST /api/v4/projects/42/share":     {http.StatusCreated, "{}"},
		"DELETE /api/v4/projects/42/share/1": {http.StatusNoContent, ""},
		// Project CRUD
		"POST /api/v4/projects":              {http.StatusCreated, projectJSON},
		"GET /api/v4/projects":               {http.StatusOK, "[]"},
		"DELETE /api/v4/projects/42":         {http.StatusAccepted, "{}"},
		"POST /api/v4/projects/42/restore":   {http.StatusOK, projectJSON},
		"POST /api/v4/projects/42/fork":      {http.StatusCreated, projectJSON},
		"POST /api/v4/projects/42/star":      {http.StatusOK, projectJSON},
		"POST /api/v4/projects/42/unstar":    {http.StatusOK, projectJSON},
		"POST /api/v4/projects/42/archive":   {http.StatusOK, projectJSON},
		"POST /api/v4/projects/42/unarchive": {http.StatusOK, projectJSON},
		"PUT /api/v4/projects/42/transfer":   {http.StatusOK, projectJSON},
	}

	// Path-only routes: matched when no method-specific route exists
	pathRoutes := map[string]mockRoute{
		"/api/v4/projects/42/invited_groups":   {http.StatusOK, "[]"},
		"/api/v4/projects/42/users":            {http.StatusOK, "[]"},
		"/api/v4/projects/42/groups":           {http.StatusOK, "[]"},
		"/api/v4/projects/42/starrers":         {http.StatusOK, "[]"},
		pathProject42Forks:                     {http.StatusOK, "[]"},
		"/api/v4/projects/42/languages":        {http.StatusOK, `{"Go":80.5,"Markdown":19.5}`},
		"/api/v4/users/1/projects":             {http.StatusOK, "[]"},
		"/api/v4/users/1/contributed_projects": {http.StatusOK, "[]"},
		"/api/v4/users/1/starred_projects":     {http.StatusOK, "[]"},
		// Catch-all GET for project 42 (also handles PUT /projects/42)
		pathProject42: {http.StatusOK, projectJSON},
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// TriggerTestHook uses a prefix match
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v4/projects/42/hooks/1/test/") {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{}`))
			return
		}

		key := r.Method + " " + r.URL.Path
		if route, ok := methodRoutes[key]; ok {
			w.WriteHeader(route.status)
			if route.body != "" {
				_, _ = w.Write([]byte(route.body))
			}
			return
		}

		if route, ok := pathRoutes[r.URL.Path]; ok {
			w.WriteHeader(route.status)
			if route.body != "" {
				_, _ = w.Write([]byte(route.body))
			}
			return
		}

		http.NotFound(w, r)
	})
}

// newProjectsMCPSession creates an in-memory MCP session with only the
// projects sub-package tools registered. Returns the client session for
// calling tools.
func newProjectsMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	client := testutil.NewTestClient(t, mcpMockHandler())
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

// TestRegisterTools_CallAllThroughMCP exercises every handler closure registered
// by RegisterTools by calling each tool through the MCP in-memory transport.
// This covers the start-timer → call-function → log → return-markdown paths
// in register.go that unit tests cannot reach.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newProjectsMCPSession(t)
	ctx := context.Background()

	// Each entry: tool name → minimal arguments that produce a successful call
	tools := []struct {
		name string
		args map[string]any
	}{
		// Project CRUD.
		{"gitlab_project_create", map[string]any{"name": "test"}},
		{"gitlab_project_get", map[string]any{"project_id": "42"}},
		{"gitlab_project_list", map[string]any{}},
		{"gitlab_project_delete", map[string]any{"project_id": "42"}},
		{"gitlab_project_restore", map[string]any{"project_id": "42"}},
		{"gitlab_project_update", map[string]any{"project_id": "42", "name": "renamed"}},
		{"gitlab_project_fork", map[string]any{"project_id": "42"}},
		{"gitlab_project_star", map[string]any{"project_id": "42"}},
		{"gitlab_project_unstar", map[string]any{"project_id": "42"}},
		{"gitlab_project_archive", map[string]any{"project_id": "42"}},
		{"gitlab_project_unarchive", map[string]any{"project_id": "42"}},
		{"gitlab_project_transfer", map[string]any{"project_id": "42", "namespace": "new-ns"}},
		{"gitlab_project_list_forks", map[string]any{"project_id": "42"}},
		{"gitlab_project_languages", map[string]any{"project_id": "42"}},

		// Hooks.
		{"gitlab_project_hook_list", map[string]any{"project_id": "42"}},
		{"gitlab_project_hook_get", map[string]any{"project_id": "42", "hook_id": float64(1)}},
		{"gitlab_project_hook_add", map[string]any{"project_id": "42", "url": testHookURL}},
		{"gitlab_project_hook_edit", map[string]any{"project_id": "42", "hook_id": float64(1), "url": testHookURL}},
		{"gitlab_project_hook_delete", map[string]any{"project_id": "42", "hook_id": float64(1)}},
		{"gitlab_project_hook_test", map[string]any{"project_id": "42", "hook_id": float64(1), "event": "push_events"}},

		// User/group listings.
		{"gitlab_project_list_user_projects", map[string]any{"user_id": "1"}},
		{"gitlab_project_list_users", map[string]any{"project_id": "42"}},
		{"gitlab_project_list_groups", map[string]any{"project_id": "42"}},
		{"gitlab_project_list_starrers", map[string]any{"project_id": "42"}},

		// Share.
		{"gitlab_project_share_with_group", map[string]any{"project_id": "42", "group_id": float64(1), "group_access": float64(30)}},
		{"gitlab_project_delete_shared_group", map[string]any{"project_id": "42", "group_id": float64(1)}},
		{"gitlab_project_list_invited_groups", map[string]any{"project_id": "42"}},

		// User-scoped listings.
		{"gitlab_project_list_user_contributed", map[string]any{"user_id": "1"}},
		{"gitlab_project_list_user_starred", map[string]any{"user_id": "1"}},

		// Push rules.
		{"gitlab_project_get_push_rules", map[string]any{"project_id": "42"}},
		{"gitlab_project_add_push_rule", map[string]any{"project_id": "42"}},
		{"gitlab_project_edit_push_rule", map[string]any{"project_id": "42"}},
		{"gitlab_project_delete_push_rule", map[string]any{"project_id": "42"}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			assertToolCallOK(t, session, ctx, tt.name, tt.args)
		})
	}
}

// ---------------------------------------------------------------------------
// Create field combination edge cases
// ---------------------------------------------------------------------------.

// TestProjectCreate_MergeMethod_FastForward verifies that Create sends the
// merge_method=ff option without squash commits.
func TestProjectCreate_MergeMethod_FastForward(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProjects {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("failed to decode body: %v", err)
			}
			if v, ok := body["merge_method"].(string); !ok || v != "ff" {
				t.Errorf("merge_method = %v, want %q", body["merge_method"], "ff")
			}
			if _, ok := body["squash_option"]; ok {
				t.Errorf("squash_option should not be set, got %v", body["squash_option"])
			}
			testutil.RespondJSON(w, http.StatusCreated, `{"id":99,"name":"ff-proj","path_with_namespace":"ns/ff-proj","visibility":"private","default_branch":"main","web_url":"https://gitlab.example.com/ns/ff-proj","description":"","merge_method":"ff"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		Name:        "ff-proj",
		MergeMethod: "ff",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.MergeMethod != "ff" {
		t.Errorf("MergeMethod = %q, want %q", out.MergeMethod, "ff")
	}
}

// TestProjectCreate_MergePoliciesCombined verifies that Create sends
// merge_method + squash_option + only_allow_merge_if_pipeline_succeeds together.
func TestProjectCreate_MergePoliciesCombined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProjects {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("failed to decode body: %v", err)
			}
			if v := body["merge_method"]; v != "rebase_merge" {
				t.Errorf("merge_method = %v, want rebase_merge", v)
			}
			if v := body["squash_option"]; v != "always" {
				t.Errorf("squash_option = %v, want always", v)
			}
			if v, ok := body["only_allow_merge_if_pipeline_succeeds"].(bool); !ok || !v {
				t.Errorf("only_allow_merge_if_pipeline_succeeds = %v, want true", body["only_allow_merge_if_pipeline_succeeds"])
			}
			testutil.RespondJSON(w, http.StatusCreated, `{"id":100,"name":"policies-proj","path_with_namespace":"ns/policies-proj","visibility":"private","default_branch":"main","web_url":"https://gitlab.example.com/ns/policies-proj","description":"","merge_method":"rebase_merge"}`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := Create(context.Background(), client, CreateInput{
		Name:                             "policies-proj",
		MergeMethod:                      "rebase_merge",
		SquashOption:                     "always",
		OnlyAllowMergeIfPipelineSucceeds: true,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestProjectCreate_RemoveSourceBranch verifies that Create sends
// remove_source_branch_after_merge when set to true.
func TestProjectCreate_RemoveSourceBranch(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProjects {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("failed to decode body: %v", err)
			}
			if v, ok := body["remove_source_branch_after_merge"].(bool); !ok || !v {
				t.Errorf("remove_source_branch_after_merge = %v, want true", body["remove_source_branch_after_merge"])
			}
			testutil.RespondJSON(w, http.StatusCreated, `{"id":101,"name":"cleanup-proj","path_with_namespace":"ns/cleanup-proj","visibility":"private","default_branch":"main","web_url":"https://gitlab.example.com/ns/cleanup-proj","description":"","remove_source_branch_after_merge":true}`)
			return
		}
		http.NotFound(w, r)
	}))

	remove := true
	out, err := Create(context.Background(), client, CreateInput{
		Name:                         "cleanup-proj",
		RemoveSourceBranchAfterMerge: &remove,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !out.RemoveSourceBranchAfterMerge {
		t.Error("RemoveSourceBranchAfterMerge = false, want true")
	}
}

// TestProjectCreate_FeatureTogglesDisabled verifies that Create correctly sends
// feature toggles set to false (issues, wiki, jobs, snippets disabled).
func TestProjectCreate_FeatureTogglesDisabled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProjects {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("failed to decode body: %v", err)
			}
			// boolToAccessLevel(false) maps to "disabled"
			if v := body["issues_access_level"]; v != "disabled" {
				t.Errorf("issues_access_level = %v, want disabled", v)
			}
			if v := body["wiki_access_level"]; v != "disabled" {
				t.Errorf("wiki_access_level = %v, want disabled", v)
			}
			if v := body["builds_access_level"]; v != "disabled" {
				t.Errorf("builds_access_level = %v, want disabled", v)
			}
			if v := body["snippets_access_level"]; v != "disabled" {
				t.Errorf("snippets_access_level = %v, want disabled", v)
			}
			testutil.RespondJSON(w, http.StatusCreated, `{"id":102,"name":"minimal-proj","path_with_namespace":"ns/minimal-proj","visibility":"private","default_branch":"main","web_url":"https://gitlab.example.com/ns/minimal-proj","description":""}`)
			return
		}
		http.NotFound(w, r)
	}))

	issues := false
	wiki := false
	jobs := false
	snippets := "disabled"
	_, err := Create(context.Background(), client, CreateInput{
		Name:                "minimal-proj",
		IssuesEnabled:       &issues,
		WikiEnabled:         &wiki,
		JobsEnabled:         &jobs,
		SnippetsAccessLevel: snippets,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// assertToolCallOK is an internal helper for the projects package.
func assertToolCallOK(t *testing.T, session *mcp.ClientSession, ctx context.Context, name string, args map[string]any) {
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

// Test paths for webhook customization, fork relations, avatars, maintenance, and admin operations.
const (
	pathProject42CustomHeaders = "/api/v4/projects/42/hooks/1/custom_headers/X-Custom"
	pathProject42URLVars       = "/api/v4/projects/42/hooks/1/url_variables/my_var"
	pathProject42ForkRelation  = "/api/v4/projects/42/fork"
	pathProject42Avatar        = "/api/v4/projects/42/avatar"
	pathProject42Housekeeping  = "/api/v4/projects/42/housekeeping"
	pathProject42Storage       = "/api/v4/projects/42/storage"
	pathProjectForUser5        = "/api/v4/projects/user/5"

	testKeyXCustom = "X-Custom"
	testKeyMyVar   = "my_var"
	testValueHdr   = "header-value"
	testValueVar   = "var-value"

	forkRelationJSON = `{"id":1,"forked_to_project_id":42,"forked_from_project_id":99,"created_at":"2025-01-01T00:00:00Z","updated_at":"2025-01-01T00:00:00Z"}`

	repoStorageJSON = `{
		"project_id":42,
		"disk_path":"@hashed/d4/73/d4735e3a265e16eee03f59718b9b5d03019c07d8b6c51f90da3a666eec13ab35",
		"repository_storage":"default",
		"created_at":"2025-01-01T00:00:00Z"
	}`

	extProjectJSON = `{"id":42,"name":"my-repo","path":"my-repo","path_with_namespace":"jmrplens/my-repo","visibility":"private","default_branch":"main","web_url":"https://gitlab.example.com/jmrplens/my-repo","description":"","topics":[]}`
)

func TestSetCustomHeader_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathProject42CustomHeaders {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	err := SetCustomHeader(context.Background(), client, SetCustomHeaderInput{
		ProjectID: "42",
		HookID:    1,
		Key:       testKeyXCustom,
		Value:     testValueHdr,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

func TestSetCustomHeader_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := SetCustomHeader(context.Background(), client, SetCustomHeaderInput{HookID: 1, Key: "k"})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

func TestSetCustomHeader_EmptyHookID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := SetCustomHeader(context.Background(), client, SetCustomHeaderInput{ProjectID: "42", Key: "k"})
	if err == nil {
		t.Fatal(errEmptyHookID)
	}
}

func TestSetCustomHeader_EmptyKey(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := SetCustomHeader(context.Background(), client, SetCustomHeaderInput{ProjectID: "42", HookID: 1})
	if err == nil {
		t.Fatal("expected error for empty key, got nil")
	}
}

func TestSetCustomHeader_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	err := SetCustomHeader(context.Background(), client, SetCustomHeaderInput{
		ProjectID: "42", HookID: 1, Key: testKeyXCustom, Value: testValueHdr,
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

func TestSetCustomHeader_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := SetCustomHeader(ctx, client, SetCustomHeaderInput{
		ProjectID: "42", HookID: 1, Key: testKeyXCustom, Value: testValueHdr,
	})
	if err == nil {
		t.Fatal(errExpectedCtxErr)
	}
}

func TestDeleteCustomHeader_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathProject42CustomHeaders {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	err := DeleteCustomHeader(context.Background(), client, DeleteCustomHeaderInput{
		ProjectID: "42", HookID: 1, Key: testKeyXCustom,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

func TestDeleteCustomHeader_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := DeleteCustomHeader(context.Background(), client, DeleteCustomHeaderInput{HookID: 1, Key: "k"})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

func TestDeleteCustomHeader_EmptyHookID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := DeleteCustomHeader(context.Background(), client, DeleteCustomHeaderInput{ProjectID: "42", Key: "k"})
	if err == nil {
		t.Fatal(errEmptyHookID)
	}
}

func TestDeleteCustomHeader_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	err := DeleteCustomHeader(context.Background(), client, DeleteCustomHeaderInput{
		ProjectID: "42", HookID: 1, Key: testKeyXCustom,
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

func TestSetWebhookURLVariable_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathProject42URLVars {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	err := SetWebhookURLVariable(context.Background(), client, SetWebhookURLVariableInput{
		ProjectID: "42", HookID: 1, Key: testKeyMyVar, Value: testValueVar,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

func TestSetWebhookURLVariable_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := SetWebhookURLVariable(context.Background(), client, SetWebhookURLVariableInput{HookID: 1, Key: "k"})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

func TestSetWebhookURLVariable_EmptyHookID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := SetWebhookURLVariable(context.Background(), client, SetWebhookURLVariableInput{ProjectID: "42", Key: "k"})
	if err == nil {
		t.Fatal(errEmptyHookID)
	}
}

func TestSetWebhookURLVariable_EmptyKey(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := SetWebhookURLVariable(context.Background(), client, SetWebhookURLVariableInput{ProjectID: "42", HookID: 1})
	if err == nil {
		t.Fatal("expected error for empty key, got nil")
	}
}

func TestSetWebhookURLVariable_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	err := SetWebhookURLVariable(context.Background(), client, SetWebhookURLVariableInput{
		ProjectID: "42", HookID: 1, Key: testKeyMyVar, Value: testValueVar,
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

func TestDeleteWebhookURLVariable_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathProject42URLVars {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	err := DeleteWebhookURLVariable(context.Background(), client, DeleteWebhookURLVariableInput{
		ProjectID: "42", HookID: 1, Key: testKeyMyVar,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

func TestDeleteWebhookURLVariable_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := DeleteWebhookURLVariable(context.Background(), client, DeleteWebhookURLVariableInput{HookID: 1, Key: "k"})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

func TestDeleteWebhookURLVariable_EmptyHookID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := DeleteWebhookURLVariable(context.Background(), client, DeleteWebhookURLVariableInput{ProjectID: "42", Key: "k"})
	if err == nil {
		t.Fatal(errEmptyHookID)
	}
}

func TestDeleteWebhookURLVariable_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	err := DeleteWebhookURLVariable(context.Background(), client, DeleteWebhookURLVariableInput{
		ProjectID: "42", HookID: 1, Key: testKeyMyVar,
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

func TestDeleteWebhookURLVariable_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := DeleteWebhookURLVariable(ctx, client, DeleteWebhookURLVariableInput{
		ProjectID: "42", HookID: 1, Key: testKeyMyVar,
	})
	if err == nil {
		t.Fatal(errExpectedCtxErr)
	}
}

func TestCreateForkRelation_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, pathProject42ForkRelation) {
			testutil.RespondJSON(w, http.StatusCreated, forkRelationJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := CreateForkRelation(context.Background(), client, CreateForkRelationInput{
		ProjectID: "42", ForkedFromID: 99,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 1 {
		t.Errorf("ID = %d, want 1", out.ID)
	}
	if out.ForkedFromProjectID != 99 {
		t.Errorf("ForkedFromProjectID = %d, want 99", out.ForkedFromProjectID)
	}
	if out.ForkedToProjectID != 42 {
		t.Errorf("ForkedToProjectID = %d, want 42", out.ForkedToProjectID)
	}
}

func TestCreateForkRelation_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := CreateForkRelation(context.Background(), client, CreateForkRelationInput{ForkedFromID: 99})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

func TestCreateForkRelation_EmptyForkedFromID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := CreateForkRelation(context.Background(), client, CreateForkRelationInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for empty forked_from_id, got nil")
	}
}

func TestCreateForkRelation_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := CreateForkRelation(context.Background(), client, CreateForkRelationInput{
		ProjectID: "42", ForkedFromID: 99,
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

func TestDeleteForkRelation_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathProject42ForkRelation {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	err := DeleteForkRelation(context.Background(), client, DeleteForkRelationInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

func TestDeleteForkRelation_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := DeleteForkRelation(context.Background(), client, DeleteForkRelationInput{})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

func TestDeleteForkRelation_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	err := DeleteForkRelation(context.Background(), client, DeleteForkRelationInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

func TestDeleteForkRelation_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := DeleteForkRelation(ctx, client, DeleteForkRelationInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpectedCtxErr)
	}
}

func TestUploadAvatar_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathProject42 {
			testutil.RespondJSON(w, http.StatusOK, extProjectJSON)
			return
		}
		http.NotFound(w, r)
	}))
	content := base64.StdEncoding.EncodeToString([]byte("fake-image-data"))
	out, err := UploadAvatar(context.Background(), client, UploadAvatarInput{
		ProjectID: "42", Filename: "avatar.png", ContentBase64: content,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 42 {
		t.Errorf("ID = %d, want 42", out.ID)
	}
}

func TestUploadAvatar_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := UploadAvatar(context.Background(), client, UploadAvatarInput{
		Filename: "a.png", ContentBase64: "dGVzdA==",
	})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

func TestUploadAvatar_EmptyFilename(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := UploadAvatar(context.Background(), client, UploadAvatarInput{
		ProjectID: "42", ContentBase64: "dGVzdA==",
	})
	if err == nil {
		t.Fatal("expected error for empty filename, got nil")
	}
}

func TestUploadAvatar_EmptyContentBase64(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := UploadAvatar(context.Background(), client, UploadAvatarInput{
		ProjectID: "42", Filename: "a.png",
	})
	if err == nil {
		t.Fatal("expected error when neither file_path nor content_base64 provided, got nil")
	}
}

func TestUploadAvatar_BothFilePathAndBase64(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := UploadAvatar(context.Background(), client, UploadAvatarInput{
		ProjectID: "42", Filename: "a.png", FilePath: "/tmp/a.png", ContentBase64: "dGVzdA==",
	})
	if err == nil {
		t.Fatal("expected error when both file_path and content_base64 provided, got nil")
	}
}

func TestUploadAvatar_FilePath_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathProject42 {
			testutil.RespondJSON(w, http.StatusOK, extProjectJSON)
			return
		}
		http.NotFound(w, r)
	}))
	tmpFile := t.TempDir() + "/avatar.png"
	if err := os.WriteFile(tmpFile, []byte("fake-image-data"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := UploadAvatar(context.Background(), client, UploadAvatarInput{
		ProjectID: "42", Filename: "avatar.png", FilePath: tmpFile,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 42 {
		t.Errorf("ID = %d, want 42", out.ID)
	}
}

func TestUploadAvatar_FilePath_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := UploadAvatar(context.Background(), client, UploadAvatarInput{
		ProjectID: "42", Filename: "a.png", FilePath: "/nonexistent/avatar.png",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent file_path, got nil")
	}
}

func TestUploadAvatar_InvalidBase64(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := UploadAvatar(context.Background(), client, UploadAvatarInput{
		ProjectID: "42", Filename: "a.png", ContentBase64: "!!!not-valid-base64!!!",
	})
	if err == nil {
		t.Fatal("expected error for invalid base64, got nil")
	}
}

func TestUploadAvatar_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	content := base64.StdEncoding.EncodeToString([]byte("data"))
	_, err := UploadAvatar(context.Background(), client, UploadAvatarInput{
		ProjectID: "42", Filename: "a.png", ContentBase64: content,
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

func TestUploadAvatar_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	content := base64.StdEncoding.EncodeToString([]byte("data"))
	_, err := UploadAvatar(ctx, client, UploadAvatarInput{
		ProjectID: "42", Filename: "a.png", ContentBase64: content,
	})
	if err == nil {
		t.Fatal(errExpectedCtxErr)
	}
}

func TestDownloadAvatar_Success(t *testing.T) {
	rawBytes := []byte("fake-png-image-bytes")
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProject42Avatar {
			w.Header().Set("Content-Type", "image/png")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(rawBytes)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := DownloadAvatar(context.Background(), client, DownloadAvatarInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.SizeBytes != len(rawBytes) {
		t.Errorf("SizeBytes = %d, want %d", out.SizeBytes, len(rawBytes))
	}
	expectedB64 := base64.StdEncoding.EncodeToString(rawBytes)
	if out.ContentBase64 != expectedB64 {
		t.Errorf("ContentBase64 mismatch")
	}
}

func TestDownloadAvatar_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := DownloadAvatar(context.Background(), client, DownloadAvatarInput{})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

func TestDownloadAvatar_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := DownloadAvatar(context.Background(), client, DownloadAvatarInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

func TestStartHousekeeping_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProject42Housekeeping {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	err := StartHousekeeping(context.Background(), client, StartHousekeepingInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

func TestStartHousekeeping_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := StartHousekeeping(context.Background(), client, StartHousekeepingInput{})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

func TestStartHousekeeping_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	err := StartHousekeeping(context.Background(), client, StartHousekeepingInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

func TestStartHousekeeping_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := StartHousekeeping(ctx, client, StartHousekeepingInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpectedCtxErr)
	}
}

func TestGetRepositoryStorage_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProject42Storage {
			testutil.RespondJSON(w, http.StatusOK, repoStorageJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := GetRepositoryStorage(context.Background(), client, GetRepositoryStorageInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ProjectID != 42 {
		t.Errorf("ProjectID = %d, want 42", out.ProjectID)
	}
	if out.RepositoryStorage != "default" {
		t.Errorf("RepositoryStorage = %q, want %q", out.RepositoryStorage, "default")
	}
	if out.DiskPath == "" {
		t.Error("DiskPath is empty")
	}
	if out.CreatedAt == "" {
		t.Error("CreatedAt is empty")
	}
}

func TestGetRepositoryStorage_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := GetRepositoryStorage(context.Background(), client, GetRepositoryStorageInput{})
	if err == nil {
		t.Fatal(errEmptyProjID)
	}
}

func TestGetRepositoryStorage_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := GetRepositoryStorage(context.Background(), client, GetRepositoryStorageInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

func TestGetRepositoryStorage_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := GetRepositoryStorage(ctx, client, GetRepositoryStorageInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpectedCtxErr)
	}
}

func TestCreateForUser_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProjectForUser5 {
			testutil.RespondJSON(w, http.StatusCreated, extProjectJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := CreateForUser(context.Background(), client, CreateForUserInput{
		UserID: 5, Name: testRepoName,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 42 {
		t.Errorf("ID = %d, want 42", out.ID)
	}
	if out.Name != testRepoName {
		t.Errorf("Name = %q, want %q", out.Name, testRepoName)
	}
}

func TestCreateForUser_EmptyUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := CreateForUser(context.Background(), client, CreateForUserInput{Name: "repo"})
	if err == nil {
		t.Fatal(errEmptyUserID)
	}
}

func TestCreateForUser_EmptyName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := CreateForUser(context.Background(), client, CreateForUserInput{UserID: 5})
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

func TestCreateForUser_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	_, err := CreateForUser(context.Background(), client, CreateForUserInput{
		UserID: 5, Name: "repo",
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

func TestCreateForUser_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := CreateForUser(ctx, client, CreateForUserInput{
		UserID: 5, Name: "repo",
	})
	if err == nil {
		t.Fatal(errExpectedCtxErr)
	}
}

func TestFormatForkRelationMarkdown_NonEmpty(t *testing.T) {
	md := FormatForkRelationMarkdown(ForkRelationOutput{
		ID: 1, ForkedToProjectID: 42, ForkedFromProjectID: 99,
		CreatedAt: "2025-01-01T00:00:00Z",
	})
	if md == "" {
		t.Fatal(errExpectedNonEmptyMD)
	}
	if !strings.Contains(md, "42") {
		t.Error("markdown missing ForkedToProjectID")
	}
}

func TestFormatDownloadAvatarMarkdown_NonEmpty(t *testing.T) {
	md := FormatDownloadAvatarMarkdown(DownloadAvatarOutput{
		ContentBase64: "dGVzdA==", SizeBytes: 4,
	})
	if md == "" {
		t.Fatal(errExpectedNonEmptyMD)
	}
	if !strings.Contains(md, "4") {
		t.Error("markdown missing size")
	}
}

func TestFormatRepositoryStorageMarkdown_NonEmpty(t *testing.T) {
	md := FormatRepositoryStorageMarkdown(RepositoryStorageOutput{
		ProjectID: 42, DiskPath: "/data/repos", RepositoryStorage: "default",
	})
	if md == "" {
		t.Fatal(errExpectedNonEmptyMD)
	}
	if !strings.Contains(md, "default") {
		t.Error("markdown missing repository storage name")
	}
}
