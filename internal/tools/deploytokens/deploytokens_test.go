// deploytokens_test.go contains unit tests for the deploy token MCP tool handlers.
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

// TestListAll_Success verifies ListAll when success.
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

// TestListProject_Success verifies ListProject when success.
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

// TestListProject_MissingProjectID verifies ListProject when missing project ID.
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

// TestListGroup_Success verifies ListGroup when success.
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

// TestListGroup_MissingGroupID verifies ListGroup when missing group ID.
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

// TestGetProject_Success verifies GetProject when success.
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

// TestGetProject_MissingTokenID verifies GetProject when missing token ID.
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

// TestGetGroup_Success verifies GetGroup when success.
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

// TestGetGroup_MissingTokenID verifies GetGroup when missing token ID.
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

// TestCreateProject_Success verifies CreateProject when success.
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

// TestCreateProject_MissingName verifies CreateProject when missing name.
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

// TestCreateProject_MissingScopes verifies CreateProject when missing scopes.
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

// TestCreateGroup_Success verifies CreateGroup when success.
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

// TestCreateGroup_MissingName verifies CreateGroup when missing name.
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

// TestDeleteProject_Success verifies DeleteProject when success.
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

// TestDeleteProject_MissingTokenID verifies DeleteProject when missing token ID.
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

// TestDeleteGroup_Success verifies DeleteGroup when success.
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

// TestDeleteGroup_MissingTokenID verifies DeleteGroup when missing token ID.
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

// TestListAll_APIError verifies ListAll when API error.
func TestListAll_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := ListAll(context.Background(), client, ListAllInput{})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestListAll_CancelledContext verifies ListAll when cancelled context.
func TestListAll_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListAll(ctx, client, ListAllInput{})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestListAll_EmptyResult verifies ListAll when empty result.
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

// TestListProject_APIError verifies ListProject when API error.
func TestListProject_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := ListProject(context.Background(), client, ListProjectInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestListProject_CancelledContext verifies ListProject when cancelled context.
func TestListProject_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListProject(ctx, client, ListProjectInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestListProject_EmptyResult verifies ListProject when empty result.
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

// TestListProject_WithPagination verifies ListProject when with pagination.
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
		ProjectID: "10", Page: 1, PerPage: 2,
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

// ---------------------------------------------------------------------------
// ListGroup — API error, canceled context
// ---------------------------------------------------------------------------.

// TestListGroup_APIError verifies ListGroup when API error.
func TestListGroup_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := ListGroup(context.Background(), client, ListGroupInput{GroupID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestListGroup_CancelledContext verifies ListGroup when cancelled context.
func TestListGroup_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListGroup(ctx, client, ListGroupInput{GroupID: "1"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestListGroup_EmptyResult verifies ListGroup when empty result.
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

// TestListGroup_WithPagination verifies ListGroup when with pagination.
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
		GroupID: "5", Page: 2, PerPage: 1,
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

// ---------------------------------------------------------------------------
// GetProject — API error, missing project_id, canceled context
// ---------------------------------------------------------------------------.

// TestGetProject_APIError verifies GetProject when API error.
func TestGetProject_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := GetProject(context.Background(), client, GetProjectInput{ProjectID: "1", DeployTokenID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGetProject_MissingProjectID verifies GetProject when missing project ID.
func TestGetProject_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := GetProject(context.Background(), client, GetProjectInput{DeployTokenID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestGetProject_CancelledContext verifies GetProject when cancelled context.
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

// TestGetGroup_APIError verifies GetGroup when API error.
func TestGetGroup_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := GetGroup(context.Background(), client, GetGroupInput{GroupID: "1", DeployTokenID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGetGroup_MissingGroupID verifies GetGroup when missing group ID.
func TestGetGroup_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := GetGroup(context.Background(), client, GetGroupInput{DeployTokenID: 1})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestGetGroup_CancelledContext verifies GetGroup when cancelled context.
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

// TestCreateProject_APIError verifies CreateProject when API error.
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

// TestCreateProject_MissingProjectID verifies CreateProject when missing project ID.
func TestCreateProject_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := CreateProject(context.Background(), client, CreateProjectInput{
		Name: "tok", Scopes: []string{"read_repository"},
	})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestCreateProject_WithOptionalFields verifies CreateProject when with optional fields.
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

// TestCreateProject_InvalidExpiresAt verifies CreateProject when invalid expires at.
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

// TestCreateProject_CancelledContext verifies CreateProject when cancelled context.
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

// TestCreateGroup_APIError verifies CreateGroup when API error.
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

// TestCreateGroup_MissingGroupID verifies CreateGroup when missing group ID.
func TestCreateGroup_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := CreateGroup(context.Background(), client, CreateGroupInput{
		Name: "tok", Scopes: []string{"read_repository"},
	})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestCreateGroup_MissingScopes verifies CreateGroup when missing scopes.
func TestCreateGroup_MissingScopes(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := CreateGroup(context.Background(), client, CreateGroupInput{
		GroupID: "5", Name: "tok",
	})
	if err == nil || !strings.Contains(err.Error(), "scopes is required") {
		t.Fatalf("expected scopes required error, got %v", err)
	}
}

// TestCreateGroup_WithOptionalFields verifies CreateGroup when with optional fields.
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

// TestCreateGroup_InvalidExpiresAt verifies CreateGroup when invalid expires at.
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

// TestCreateGroup_CancelledContext verifies CreateGroup when cancelled context.
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

// TestDeleteProject_APIError verifies DeleteProject when API error.
func TestDeleteProject_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := DeleteProject(context.Background(), client, DeleteProjectInput{ProjectID: "1", DeployTokenID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDeleteProject_MissingProjectID verifies DeleteProject when missing project ID.
func TestDeleteProject_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	err := DeleteProject(context.Background(), client, DeleteProjectInput{DeployTokenID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestDeleteProject_CancelledContext verifies DeleteProject when cancelled context.
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

// TestDeleteGroup_APIError verifies DeleteGroup when API error.
func TestDeleteGroup_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := DeleteGroup(context.Background(), client, DeleteGroupInput{GroupID: "1", DeployTokenID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDeleteGroup_MissingGroupID verifies DeleteGroup when missing group ID.
func TestDeleteGroup_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	err := DeleteGroup(context.Background(), client, DeleteGroupInput{DeployTokenID: 1})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestDeleteGroup_CancelledContext verifies DeleteGroup when cancelled context.
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

// TestFormatOutputMarkdown_AllFields verifies FormatOutputMarkdown when all fields.
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
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatOutputMarkdown_NoToken verifies FormatOutputMarkdown when no token.
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

// TestFormatOutputMarkdown_NoExpiresAt verifies FormatOutputMarkdown when no expires at.
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

// TestFormatOutputMarkdown_RevokedExpired verifies FormatOutputMarkdown when revoked expired.
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

// TestFormatListMarkdown_WithTokens verifies list output includes token rows and a Markdown table.
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
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatListMarkdown_Empty verifies empty list output omits the table header.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No deploy tokens found") {
		t.Errorf("expected empty message:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// TestFormatListMarkdown_ZeroTokens verifies an explicit zero-token list uses the empty message.
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

// TestTimeStr_NilInput verifies TimeStr when nil input.
func TestTimeStr_NilInput(t *testing.T) {
	result := timeStr(nil)
	if result != "" {
		t.Errorf("expected empty string for nil time, got %q", result)
	}
}
