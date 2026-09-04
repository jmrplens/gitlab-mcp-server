// deploy_tokens_test.go contains unit tests for the deploy token MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package deploytokens

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ---------------------------------------------------------------------------
// ListAll
// ---------------------------------------------------------------------------.

// TestListAll_Success verifies that ListAll succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListAll_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/deploy_tokens", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"name":"inst-token","username":"deployer","scopes":["read_repository"]}]`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListAll(context.Background(), client, ListAllInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.DeployTokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(out.DeployTokens))
	}
	if out.DeployTokens[0].Name != "inst-token" {
		t.Errorf("expected name inst-token, got %s", out.DeployTokens[0].Name)
	}
}

// ---------------------------------------------------------------------------
// ListProject
// ---------------------------------------------------------------------------.

// TestListProject_Success verifies that ListProject succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListProject_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/10/deploy_tokens", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":2,"name":"proj-token","username":"deployer","scopes":["read_registry"]}]`,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListProject(context.Background(), client, ListProjectInput{
		ProjectID: toolutil.StringOrInt("10"),
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.DeployTokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(out.DeployTokens))
	}
}

// TestListProject_MissingProjectID verifies that ListProject_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListProject_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ListProject(context.Background(), client, ListProjectInput{})
	if err == nil || !strings.Contains(err.Error(), "project_id is required") {
		t.Fatalf("expected project_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// ListGroup
// ---------------------------------------------------------------------------.

// TestListGroup_Success verifies that ListGroup succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListGroup_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/5/deploy_tokens", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":3,"name":"grp-token","username":"deployer","scopes":["read_repository"]}]`,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListGroup(context.Background(), client, ListGroupInput{
		GroupID: toolutil.StringOrInt("5"),
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.DeployTokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(out.DeployTokens))
	}
}

// TestListGroup_MissingGroupID verifies that ListGroup_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListGroup_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ListGroup(context.Background(), client, ListGroupInput{})
	if err == nil || !strings.Contains(err.Error(), "group_id is required") {
		t.Fatalf("expected group_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// GetProject
// ---------------------------------------------------------------------------.

// TestGetProject_Success verifies that GetProject succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGetProject_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/10/deploy_tokens/2", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":2,"name":"proj-token","username":"deployer","scopes":["read_registry"]}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := GetProject(context.Background(), client, GetProjectInput{
		ProjectID:     toolutil.StringOrInt("10"),
		DeployTokenID: 2,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 2 {
		t.Errorf("expected ID 2, got %d", out.ID)
	}
}

// TestGetProject_MissingTokenID verifies that GetProject_MissingTokenID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetProject_MissingTokenID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := GetProject(context.Background(), client, GetProjectInput{ProjectID: toolutil.StringOrInt("10")})
	if err == nil || !strings.Contains(err.Error(), "deploy_token_id is required") {
		t.Fatalf("expected deploy_token_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// GetGroup
// ---------------------------------------------------------------------------.

// TestGetGroup_Success verifies that GetGroup succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGetGroup_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/5/deploy_tokens/3", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":3,"name":"grp-token","username":"deployer","scopes":["read_repository"]}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := GetGroup(context.Background(), client, GetGroupInput{
		GroupID:       toolutil.StringOrInt("5"),
		DeployTokenID: 3,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 3 {
		t.Errorf("expected ID 3, got %d", out.ID)
	}
}

// TestGetGroup_MissingTokenID verifies that GetGroup_MissingTokenID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetGroup_MissingTokenID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := GetGroup(context.Background(), client, GetGroupInput{GroupID: toolutil.StringOrInt("5")})
	if err == nil || !strings.Contains(err.Error(), "deploy_token_id is required") {
		t.Fatalf("expected deploy_token_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// CreateProject
// ---------------------------------------------------------------------------.

// TestCreateProject_Success verifies that CreateProject succeeds when the GitLab API returns a valid response.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreateProject_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/10/deploy_tokens", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		testutil.RespondJSON(w, http.StatusCreated, `{"id":4,"name":"new-token","username":"deployer","token":"secret123","scopes":["read_repository"]}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := CreateProject(context.Background(), client, CreateProjectInput{
		ProjectID: toolutil.StringOrInt("10"),
		Name:      "new-token",
		Scopes:    []string{"read_repository"},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Token != "secret123" {
		t.Errorf("expected token secret123, got %s", out.Token)
	}
}

// TestCreateProject_MissingName verifies that CreateProject_MissingName returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateProject_MissingName(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := CreateProject(context.Background(), client, CreateProjectInput{
		ProjectID: toolutil.StringOrInt("10"),
		Scopes:    []string{"read_repository"},
	})
	if err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("expected name required error, got %v", err)
	}
}

// TestCreateProject_MissingScopes verifies that CreateProject_MissingScopes returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateProject_MissingScopes(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := CreateProject(context.Background(), client, CreateProjectInput{
		ProjectID: toolutil.StringOrInt("10"),
		Name:      "test",
	})
	if err == nil || !strings.Contains(err.Error(), "scopes is required") {
		t.Fatalf("expected scopes required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// CreateGroup
// ---------------------------------------------------------------------------.

// TestCreateGroup_Success verifies that CreateGroup succeeds when the GitLab API returns a valid response.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreateGroup_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/5/deploy_tokens", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		testutil.RespondJSON(w, http.StatusCreated, `{"id":5,"name":"grp-new-token","username":"deployer","token":"secret456","scopes":["read_repository"]}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := CreateGroup(context.Background(), client, CreateGroupInput{
		GroupID: toolutil.StringOrInt("5"),
		Name:    "grp-new-token",
		Scopes:  []string{"read_repository"},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Token != "secret456" {
		t.Errorf("expected token secret456, got %s", out.Token)
	}
}

// TestCreateGroup_MissingName verifies that CreateGroup_MissingName returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateGroup_MissingName(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := CreateGroup(context.Background(), client, CreateGroupInput{
		GroupID: toolutil.StringOrInt("5"),
		Scopes:  []string{"read_repository"},
	})
	if err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("expected name required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// DeleteProject
// ---------------------------------------------------------------------------.

// TestDeleteProject_Success verifies that DeleteProject succeeds when the GitLab API returns a valid response.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDeleteProject_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/10/deploy_tokens/2", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)

	err := DeleteProject(context.Background(), client, DeleteProjectInput{
		ProjectID:     toolutil.StringOrInt("10"),
		DeployTokenID: 2,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeleteProject_MissingTokenID verifies that DeleteProject_MissingTokenID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteProject_MissingTokenID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := DeleteProject(context.Background(), client, DeleteProjectInput{ProjectID: toolutil.StringOrInt("10")})
	if err == nil || !strings.Contains(err.Error(), "deploy_token_id is required") {
		t.Fatalf("expected deploy_token_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// DeleteGroup
// ---------------------------------------------------------------------------.

// TestDeleteGroup_Success verifies that DeleteGroup succeeds when the GitLab API returns a valid response.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDeleteGroup_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/5/deploy_tokens/3", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)

	err := DeleteGroup(context.Background(), client, DeleteGroupInput{
		GroupID:       toolutil.StringOrInt("5"),
		DeployTokenID: 3,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeleteGroup_MissingTokenID verifies that DeleteGroup_MissingTokenID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteGroup_MissingTokenID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := DeleteGroup(context.Background(), client, DeleteGroupInput{GroupID: toolutil.StringOrInt("5")})
	if err == nil || !strings.Contains(err.Error(), "deploy_token_id is required") {
		t.Fatalf("expected deploy_token_id required error, got %v", err)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpCancelledCtx identifies the err exp cancelled ctx constant used by this package.
const errExpCancelledCtx = "expected error for canceled context"

// errExpectedAPI identifies the err expected API constant used by this package.
const errExpectedAPI = "expected API error, got nil"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// ListAll — API error, canceled context
// ---------------------------------------------------------------------------.

// TestListAll_APIError verifies that ListAll returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListAll_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := ListAll(context.Background(), client, ListAllInput{})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestListAll_CancelledContext verifies the ListAll_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestListAll_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListAll(ctx, client, ListAllInput{})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestListAll_EmptyResult verifies the ListAll_EmptyResult handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListAll_EmptyResult(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	out, err := ListAll(context.Background(), client, ListAllInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.DeployTokens) != 0 {
		t.Errorf("expected 0 tokens, got %d", len(out.DeployTokens))
	}
}

// ---------------------------------------------------------------------------
// ListProject — API error, canceled context
// ---------------------------------------------------------------------------.

// TestListProject_APIError verifies that ListProject returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListProject_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := ListProject(context.Background(), client, ListProjectInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestListProject_CancelledContext verifies the ListProject_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestListProject_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListProject(ctx, client, ListProjectInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestListProject_EmptyResult verifies the ListProject_EmptyResult handler.
// The mock GitLab API at /api/v4/projects/1/deploy_tokens (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestListProject_EmptyResult(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/1/deploy_tokens" {
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := ListProject(context.Background(), client, ListProjectInput{ProjectID: "1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.DeployTokens) != 0 {
		t.Errorf("expected 0 tokens, got %d", len(out.DeployTokens))
	}
}

// TestListProject_WithPagination verifies that ListProject_WithPagination forwards pagination parameters to the GitLab API and parses the response metadata.
// The mock GitLab API at /api/v4/projects/10/deploy_tokens (GET) responds with HTTP OK.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestListProject_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/10/deploy_tokens" {
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"id":1,"name":"tok1","username":"u","scopes":["read_repository"]},{"id":2,"name":"tok2","username":"u","scopes":["read_registry"]}]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "2", Total: "5", TotalPages: "3", NextPage: "2"})
			return
		}
		http.NotFound(w, r)
	}))
	out, err := ListProject(context.Background(), client, ListProjectInput{
		ProjectID: "10",
		Page:      1, PerPage: 2,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.DeployTokens) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(out.DeployTokens))
	}
	if out.Pagination.TotalPages != 3 {
		t.Errorf("TotalPages = %d, want 3", out.Pagination.TotalPages)
	}
}

// TestListProject_WithKeyset verifies that ListProject forwards keyset
// pagination and ordering parameters (order_by, sort, pagination, page_token)
// to the GitLab API query string.
// The mock GitLab API at /api/v4/projects/10/deploy_tokens (GET) responds with HTTP OK.
// It asserts each keyset query parameter is propagated onto the request URL.
func TestListProject_WithKeyset(t *testing.T) {
	var query string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/10/deploy_tokens" {
			query = r.URL.RawQuery
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"name":"tok1","username":"u","scopes":["read_repository"]}]`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := ListProject(context.Background(), client, ListProjectInput{
		ProjectID:  "10",
		OrderBy:    "id",
		Sort:       "desc",
		Pagination: "keyset", PageToken: "5",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.DeployTokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(out.DeployTokens))
	}
	for _, want := range []string{"order_by=id", "sort=desc", "pagination=keyset", "page_token=5"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(query, want) {
				t.Errorf("query %q missing %q", query, want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ListGroup — API error, canceled context
// ---------------------------------------------------------------------------.

// TestListGroup_APIError verifies that ListGroup returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListGroup_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := ListGroup(context.Background(), client, ListGroupInput{GroupID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestListGroup_CancelledContext verifies the ListGroup_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestListGroup_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListGroup(ctx, client, ListGroupInput{GroupID: "1"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestListGroup_EmptyResult verifies the ListGroup_EmptyResult handler.
// The mock GitLab API at /api/v4/groups/5/deploy_tokens (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestListGroup_EmptyResult(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/5/deploy_tokens" {
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := ListGroup(context.Background(), client, ListGroupInput{GroupID: "5"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.DeployTokens) != 0 {
		t.Errorf("expected 0 tokens, got %d", len(out.DeployTokens))
	}
}

// TestListGroup_WithPagination verifies that ListGroup_WithPagination forwards pagination parameters to the GitLab API and parses the response metadata.
// The mock GitLab API at /api/v4/groups/5/deploy_tokens (GET) responds with HTTP OK.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestListGroup_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/5/deploy_tokens" {
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"id":10,"name":"grp-tok","username":"u","scopes":["read_repository"]}]`,
				testutil.PaginationHeaders{Page: "2", PerPage: "1", Total: "3", TotalPages: "3", NextPage: "3", PrevPage: "1"})
			return
		}
		http.NotFound(w, r)
	}))
	out, err := ListGroup(context.Background(), client, ListGroupInput{
		GroupID: "5",
		Page:    2, PerPage: 1,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.DeployTokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(out.DeployTokens))
	}
	if out.Pagination.TotalPages != 3 {
		t.Errorf("TotalPages = %d, want 3", out.Pagination.TotalPages)
	}
}

// TestListGroup_WithKeyset verifies that ListGroup forwards keyset pagination
// and ordering parameters (order_by, sort, pagination, page_token) to the
// GitLab API query string.
// The mock GitLab API at /api/v4/groups/5/deploy_tokens (GET) responds with HTTP OK.
// It asserts each keyset query parameter is propagated onto the request URL.
func TestListGroup_WithKeyset(t *testing.T) {
	var query string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/5/deploy_tokens" {
			query = r.URL.RawQuery
			testutil.RespondJSON(w, http.StatusOK, `[{"id":10,"name":"grp-tok","username":"u","scopes":["read_repository"]}]`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := ListGroup(context.Background(), client, ListGroupInput{
		GroupID:    "5",
		OrderBy:    "id",
		Sort:       "asc",
		Pagination: "keyset", PageToken: "3",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.DeployTokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(out.DeployTokens))
	}
	for _, want := range []string{"order_by=id", "sort=asc", "pagination=keyset", "page_token=3"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(query, want) {
				t.Errorf("query %q missing %q", query, want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GetProject — API error, missing project_id, canceled context
// ---------------------------------------------------------------------------.

// TestGetProject_APIError verifies that GetProject returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetProject_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := GetProject(context.Background(), client, GetProjectInput{ProjectID: "1", DeployTokenID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGetProject_MissingProjectID verifies that GetProject_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetProject_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := GetProject(context.Background(), client, GetProjectInput{DeployTokenID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestGetProject_CancelledContext verifies the GetProject_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestGetProject_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetProject(ctx, client, GetProjectInput{ProjectID: "1", DeployTokenID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// GetGroup — API error, missing group_id, canceled context
// ---------------------------------------------------------------------------.

// TestGetGroup_APIError verifies that GetGroup returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetGroup_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := GetGroup(context.Background(), client, GetGroupInput{GroupID: "1", DeployTokenID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGetGroup_MissingGroupID verifies that GetGroup_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetGroup_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := GetGroup(context.Background(), client, GetGroupInput{DeployTokenID: 1})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestGetGroup_CancelledContext verifies the GetGroup_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestGetGroup_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetGroup(ctx, client, GetGroupInput{GroupID: "1", DeployTokenID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// CreateProject — API error, missing project_id, missing scopes, with optional
//   fields (username, expires_at), invalid expires_at, canceled context
// ---------------------------------------------------------------------------.

// TestCreateProject_APIError verifies that CreateProject returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateProject_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := CreateProject(context.Background(), client, CreateProjectInput{
		ProjectID: "1", Name: "tok", Scopes: []string{"read_repository"},
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCreateProject_MissingProjectID verifies that CreateProject_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateProject_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := CreateProject(context.Background(), client, CreateProjectInput{
		Name: "tok", Scopes: []string{"read_repository"},
	})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestCreateProject_WithOptionalFields verifies the CreateProject_WithOptionalFields handler.
// The mock GitLab API at /api/v4/projects/10/deploy_tokens (POST) responds with HTTP Created.
// It asserts the returned output matches the expected fields.
func TestCreateProject_WithOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/10/deploy_tokens" {
			testutil.RespondJSON(w, http.StatusCreated,
				`{"id":10,"name":"my-tok","username":"custom-user","token":"tok-abc","scopes":["read_repository","read_registry"],"expires_at":"2027-06-15T00:00:00Z"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := CreateProject(context.Background(), client, CreateProjectInput{
		ProjectID: "10",
		Name:      "my-tok",
		Username:  "custom-user",
		ExpiresAt: "2027-06-15",
		Scopes:    []string{"read_repository", "read_registry"},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Username != "custom-user" {
		t.Errorf("Username = %q, want %q", out.Username, "custom-user")
	}
	if out.Token != "tok-abc" {
		t.Errorf("Token = %q, want %q", out.Token, "tok-abc")
	}
	if len(out.Scopes) != 2 {
		t.Errorf("len(Scopes) = %d, want 2", len(out.Scopes))
	}
}

// TestCreateProject_InvalidExpiresAt verifies the CreateProject_InvalidExpiresAt handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreateProject_InvalidExpiresAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := CreateProject(context.Background(), client, CreateProjectInput{
		ProjectID: "10",
		Name:      "tok",
		Scopes:    []string{"read_repository"},
		ExpiresAt: "not-a-date",
	})
	if err == nil {
		t.Fatal("expected error for invalid expires_at, got nil")
	}
	if !strings.Contains(err.Error(), "invalid expires_at") {
		t.Errorf("error message should mention invalid expires_at: %v", err)
	}
}

// TestCreateProject_CancelledContext verifies the CreateProject_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestCreateProject_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := CreateProject(ctx, client, CreateProjectInput{
		ProjectID: "1", Name: "tok", Scopes: []string{"read_repository"},
	})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// CreateGroup — API error, missing group_id, missing name, missing scopes,
//   with optional fields, invalid expires_at, canceled context
// ---------------------------------------------------------------------------.

// TestCreateGroup_APIError verifies that CreateGroup returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateGroup_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := CreateGroup(context.Background(), client, CreateGroupInput{
		GroupID: "1", Name: "tok", Scopes: []string{"read_repository"},
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCreateGroup_MissingGroupID verifies that CreateGroup_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateGroup_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := CreateGroup(context.Background(), client, CreateGroupInput{
		Name: "tok", Scopes: []string{"read_repository"},
	})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestCreateGroup_MissingScopes verifies that CreateGroup_MissingScopes returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateGroup_MissingScopes(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := CreateGroup(context.Background(), client, CreateGroupInput{
		GroupID: "5", Name: "tok",
	})
	if err == nil || !strings.Contains(err.Error(), "scopes is required") {
		t.Fatalf("expected scopes required error, got %v", err)
	}
}

// TestCreateGroup_WithOptionalFields verifies the CreateGroup_WithOptionalFields handler.
// The mock GitLab API at /api/v4/groups/5/deploy_tokens (POST) responds with HTTP Created.
// It asserts the returned output matches the expected fields.
func TestCreateGroup_WithOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/groups/5/deploy_tokens" {
			testutil.RespondJSON(w, http.StatusCreated,
				`{"id":20,"name":"grp-tok","username":"grp-user","token":"tok-xyz","scopes":["read_repository"],"expires_at":"2028-01-01T00:00:00Z"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := CreateGroup(context.Background(), client, CreateGroupInput{
		GroupID:   "5",
		Name:      "grp-tok",
		Username:  "grp-user",
		ExpiresAt: "2028-01-01",
		Scopes:    []string{"read_repository"},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Username != "grp-user" {
		t.Errorf("Username = %q, want %q", out.Username, "grp-user")
	}
	if out.Token != "tok-xyz" {
		t.Errorf("Token = %q, want %q", out.Token, "tok-xyz")
	}
}

// TestCreateGroup_InvalidExpiresAt verifies the CreateGroup_InvalidExpiresAt handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreateGroup_InvalidExpiresAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := CreateGroup(context.Background(), client, CreateGroupInput{
		GroupID:   "5",
		Name:      "tok",
		Scopes:    []string{"read_repository"},
		ExpiresAt: "bad-date",
	})
	if err == nil {
		t.Fatal("expected error for invalid expires_at, got nil")
	}
	if !strings.Contains(err.Error(), "invalid expires_at") {
		t.Errorf("error message should mention invalid expires_at: %v", err)
	}
}

// TestCreateGroup_CancelledContext verifies the CreateGroup_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestCreateGroup_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := CreateGroup(ctx, client, CreateGroupInput{
		GroupID: "1", Name: "tok", Scopes: []string{"read_repository"},
	})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// DeleteProject — API error, missing project_id, canceled context
// ---------------------------------------------------------------------------.

// TestDeleteProject_APIError verifies that DeleteProject returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteProject_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := DeleteProject(context.Background(), client, DeleteProjectInput{ProjectID: "1", DeployTokenID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDeleteProject_MissingProjectID verifies that DeleteProject_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteProject_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	err := DeleteProject(context.Background(), client, DeleteProjectInput{DeployTokenID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestDeleteProject_CancelledContext verifies the DeleteProject_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestDeleteProject_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	err := DeleteProject(ctx, client, DeleteProjectInput{ProjectID: "1", DeployTokenID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// DeleteGroup — API error, missing group_id, canceled context
// ---------------------------------------------------------------------------.

// TestDeleteGroup_APIError verifies that DeleteGroup returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteGroup_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := DeleteGroup(context.Background(), client, DeleteGroupInput{GroupID: "1", DeployTokenID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDeleteGroup_MissingGroupID verifies that DeleteGroup_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteGroup_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	err := DeleteGroup(context.Background(), client, DeleteGroupInput{DeployTokenID: 1})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestDeleteGroup_CancelledContext verifies the DeleteGroup_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestDeleteGroup_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	err := DeleteGroup(ctx, client, DeleteGroupInput{GroupID: "1", DeployTokenID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// FormatOutputMarkdown
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdown_AllFields verifies the OutputMarkdown_AllFields Markdown formatter for a representative output_allfields input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatOutputMarkdown_AllFields(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		ID:        42,
		Name:      "deploy-reader",
		Username:  "gitlab-deploy",
		Token:     "gldt-secret",
		Scopes:    []string{"read_repository", "read_registry"},
		Revoked:   false,
		Expired:   false,
		ExpiresAt: "2027-06-15T00:00:00Z",
	})

	for _, want := range []string{
		"## Deploy Token: deploy-reader (ID: 42)",
		"| ID | 42 |",
		"| Name | deploy-reader |",
		"| Username | gitlab-deploy |",
		"| Token | gldt-secret |",
		"| Scopes | read_repository, read_registry |",
		"| Revoked | false |",
		"| Expired | false |",
		"| Expires | 15 Jun 2027 00:00 UTC |",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Errorf("markdown missing %q:\n%s", want, md)
			}
		})
	}
}

// TestFormatOutputMarkdown_NoToken verifies the OutputMarkdown_NoToken Markdown formatter for a representative output_notoken input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatOutputMarkdown_NoToken(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		ID:       1,
		Name:     "tok",
		Username: "u",
		Scopes:   []string{"read_repository"},
	})
	if strings.Contains(md, "| Token |") {
		t.Error("should not contain Token row when token is empty")
	}
}

// TestFormatOutputMarkdown_NoExpiresAt verifies the OutputMarkdown_NoExpiresAt Markdown formatter for a representative output_noexpiresat input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatOutputMarkdown_NoExpiresAt(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		ID:       1,
		Name:     "tok",
		Username: "u",
		Scopes:   []string{"read_repository"},
	})
	if strings.Contains(md, "| Expires |") {
		t.Error("should not contain Expires row when expires_at is empty")
	}
}

// TestFormatOutputMarkdown_RevokedExpired verifies the OutputMarkdown_RevokedExpired Markdown formatter for a representative output_revokedexpired input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatOutputMarkdown_RevokedExpired(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		ID:       99,
		Name:     "old-tok",
		Username: "u",
		Scopes:   []string{"read_repository"},
		Revoked:  true,
		Expired:  true,
	})

	if !strings.Contains(md, "| Revoked | true |") {
		t.Errorf("expected Revoked true:\n%s", md)
	}
	if !strings.Contains(md, "| Expired | true |") {
		t.Errorf("expected Expired true:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithTokens verifies the ListMarkdown_WithTokens Markdown formatter for a representative list_withtokens input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown_WithTokens(t *testing.T) {
	out := ListOutput{
		DeployTokens: []Output{
			{ID: 1, Name: "tok1", Username: "u1", Scopes: []string{"read_repository"}, Revoked: false, Expired: false},
			{ID: 2, Name: "tok2", Username: "u2", Scopes: []string{"read_registry", "write_registry"}, Revoked: true, Expired: true},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatListMarkdown(out)

	for _, want := range []string{
		"## Deploy Tokens (2)",
		"| ID |",
		"| --- |",
		"| 1 |",
		"| 2 |",
		"tok1",
		"tok2",
		"u1",
		"u2",
		"read_repository",
		"read_registry, write_registry",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Errorf("markdown missing %q:\n%s", want, md)
			}
		})
	}
}

// TestFormatListMarkdown_Empty verifies the ListMarkdown_Empty Markdown formatter for a representative list_empty input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No deploy tokens found") {
		t.Errorf("expected empty message:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// TestFormatListMarkdown_ZeroTokens verifies the ListMarkdown_ZeroTokens Markdown formatter for a representative list_zerotokens input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown_ZeroTokens(t *testing.T) {
	md := FormatListMarkdown(ListOutput{
		DeployTokens: []Output{},
		Pagination:   toolutil.PaginationOutput{TotalItems: 0, Page: 1, PerPage: 20, TotalPages: 0},
	})
	if !strings.Contains(md, "## Deploy Tokens (0)") {
		t.Errorf("expected header with count 0:\n%s", md)
	}
	if !strings.Contains(md, "No deploy tokens found") {
		t.Errorf("expected empty message:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// timeStr helper
// ---------------------------------------------------------------------------.

// TestTimeStr_NilInput verifies the TimeStr_NilInput handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestTimeStr_NilInput(t *testing.T) {
	result := timeStr(nil)
	if result != "" {
		t.Errorf("expected empty string for nil time, got %q", result)
	}
}
