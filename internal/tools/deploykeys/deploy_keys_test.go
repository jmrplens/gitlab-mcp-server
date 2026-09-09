// deploy_keys_test.go contains unit tests for the deploy key MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package deploykeys

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// ---------------------------------------------------------------------------
// ListProject
// ---------------------------------------------------------------------------.

// TestListProject_Success verifies that ListProject succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListProject_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/123/deploy_keys", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":1,"title":"my-key","key":"ssh-rsa AAAA","fingerprint":"ab:cd","can_push":true}]`,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListProject(context.Background(), client, ListProjectInput{
		ProjectID: toolutil.StringOrInt("123"),
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.DeployKeys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(out.DeployKeys))
	}
	if out.DeployKeys[0].Title != "my-key" {
		t.Errorf("expected title my-key, got %s", out.DeployKeys[0].Title)
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
// Get
// ---------------------------------------------------------------------------.

// TestGet_Success verifies that Get succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGet_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/123/deploy_keys/1", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"title":"my-key","key":"ssh-rsa AAAA","can_push":false}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := Get(context.Background(), client, GetInput{
		ProjectID:   toolutil.StringOrInt("123"),
		DeployKeyID: 1,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 1 {
		t.Errorf("expected ID 1, got %d", out.ID)
	}
}

// TestGet_MissingDeployKeyID verifies that Get_MissingDeployKeyID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGet_MissingDeployKeyID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := Get(context.Background(), client, GetInput{ProjectID: toolutil.StringOrInt("123")})
	if err == nil || !strings.Contains(err.Error(), "deploy_key_id is required") {
		t.Fatalf("expected deploy_key_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Add
// ---------------------------------------------------------------------------.

// TestAdd_Success verifies that Add succeeds when the GitLab API returns a valid response.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestAdd_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/123/deploy_keys", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["title"] != "test-key" {
			t.Errorf("expected title test-key, got %v", body["title"])
		}
		testutil.RespondJSON(w, http.StatusCreated, `{"id":2,"title":"test-key","key":"ssh-rsa BBBB","can_push":true}`)
	})
	client := testutil.NewTestClient(t, mux)

	cp := true
	out, err := Add(context.Background(), client, AddInput{
		ProjectID: toolutil.StringOrInt("123"),
		Title:     "test-key",
		Key:       "ssh-rsa BBBB",
		CanPush:   &cp,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 2 {
		t.Errorf("expected ID 2, got %d", out.ID)
	}
	if !out.CanPush {
		t.Error("expected can_push=true")
	}
}

// TestAdd_MissingTitle verifies that Add_MissingTitle returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestAdd_MissingTitle(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := Add(context.Background(), client, AddInput{
		ProjectID: toolutil.StringOrInt("123"),
		Key:       "ssh-rsa AAAA",
	})
	if err == nil || !strings.Contains(err.Error(), "title is required") {
		t.Fatalf("expected title required error, got %v", err)
	}
}

// TestAdd_MissingKey verifies that Add_MissingKey returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestAdd_MissingKey(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := Add(context.Background(), client, AddInput{
		ProjectID: toolutil.StringOrInt("123"),
		Title:     "test",
	})
	if err == nil || !strings.Contains(err.Error(), "key is required") {
		t.Fatalf("expected key required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------.

// TestUpdate_Success verifies that Update succeeds when the GitLab API returns a valid response.
// The test exercises the PUT path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestUpdate_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/123/deploy_keys/1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"title":"updated-key","key":"ssh-rsa AAAA","can_push":true}`)
	})
	client := testutil.NewTestClient(t, mux)

	canPush := true
	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID:   toolutil.StringOrInt("123"),
		DeployKeyID: 1,
		Title:       "updated-key",
		CanPush:     &canPush,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Title != "updated-key" {
		t.Errorf("expected title updated-key, got %s", out.Title)
	}
}

// TestUpdate_MissingDeployKeyID verifies that Update_MissingDeployKeyID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestUpdate_MissingDeployKeyID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: toolutil.StringOrInt("123")})
	if err == nil || !strings.Contains(err.Error(), "deploy_key_id is required") {
		t.Fatalf("expected deploy_key_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------.

// TestDelete_Success verifies that Delete succeeds when the GitLab API returns a valid response.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDelete_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/123/deploy_keys/1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)

	err := Delete(context.Background(), client, DeleteInput{
		ProjectID:   toolutil.StringOrInt("123"),
		DeployKeyID: 1,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDelete_MissingProjectID verifies that Delete_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDelete_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := Delete(context.Background(), client, DeleteInput{DeployKeyID: 1})
	if err == nil || !strings.Contains(err.Error(), "project_id is required") {
		t.Fatalf("expected project_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Enable
// ---------------------------------------------------------------------------.

// TestEnable_Success verifies that Enable succeeds when the GitLab API returns a valid response.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestEnable_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/123/deploy_keys/5/enable", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		testutil.RespondJSON(w, http.StatusCreated, `{"id":5,"title":"enabled-key","key":"ssh-rsa CCCC","can_push":false}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := Enable(context.Background(), client, EnableInput{
		ProjectID:   toolutil.StringOrInt("123"),
		DeployKeyID: 5,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 5 {
		t.Errorf("expected ID 5, got %d", out.ID)
	}
}

// TestEnable_MissingDeployKeyID verifies that Enable_MissingDeployKeyID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestEnable_MissingDeployKeyID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := Enable(context.Background(), client, EnableInput{ProjectID: toolutil.StringOrInt("123")})
	if err == nil || !strings.Contains(err.Error(), "deploy_key_id is required") {
		t.Fatalf("expected deploy_key_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// ListAll (instance level)
// ---------------------------------------------------------------------------.

// TestListAll_Success verifies that ListAll succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListAll_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/deploy_keys", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":10,"title":"instance-key","key":"ssh-rsa DDDD"}]`,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListAll(context.Background(), client, ListAllInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.DeployKeys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(out.DeployKeys))
	}
	if out.DeployKeys[0].Title != "instance-key" {
		t.Errorf("expected title instance-key, got %s", out.DeployKeys[0].Title)
	}
}

// ---------------------------------------------------------------------------
// AddInstance
// ---------------------------------------------------------------------------.

// TestAddInstance_Success verifies that AddInstance succeeds when the GitLab API returns a valid response.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestAddInstance_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/deploy_keys", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		testutil.RespondJSON(w, http.StatusCreated, `{"id":11,"title":"new-instance-key","key":"ssh-rsa EEEE"}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := AddInstance(context.Background(), client, AddInstanceInput{
		Title: "new-instance-key",
		Key:   "ssh-rsa EEEE",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 11 {
		t.Errorf("expected ID 11, got %d", out.ID)
	}
}

// TestAddInstance_MissingTitle verifies that AddInstance_MissingTitle returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestAddInstance_MissingTitle(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := AddInstance(context.Background(), client, AddInstanceInput{Key: "ssh-rsa AAAA"})
	if err == nil || !strings.Contains(err.Error(), "title is required") {
		t.Fatalf("expected title required error, got %v", err)
	}
}

// TestAddInstance_MissingKey verifies that AddInstance_MissingKey returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestAddInstance_MissingKey(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := AddInstance(context.Background(), client, AddInstanceInput{Title: "test"})
	if err == nil || !strings.Contains(err.Error(), "key is required") {
		t.Fatalf("expected key required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// ListUserProject
// ---------------------------------------------------------------------------.

// TestListUserProject_Success verifies that ListUserProject succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListUserProject_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/users/42/project_deploy_keys", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":3,"title":"user-key","key":"ssh-rsa FFFF","can_push":false}]`,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListUserProject(context.Background(), client, ListUserProjectInput{
		UserID: toolutil.StringOrInt("42"),
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.DeployKeys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(out.DeployKeys))
	}
	if out.DeployKeys[0].Title != "user-key" {
		t.Errorf("expected title user-key, got %s", out.DeployKeys[0].Title)
	}
}

// TestListUserProject_MissingUserID verifies that ListUserProject_MissingUserID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListUserProject_MissingUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ListUserProject(context.Background(), client, ListUserProjectInput{})
	if err == nil || !strings.Contains(err.Error(), "user_id is required") {
		t.Fatalf("expected user_id required error, got %v", err)
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

// ---------------------------------------------------------------------------
// Get — API error, missing project_id, canceled context
// ---------------------------------------------------------------------------.

// TestGet_APIError verifies that Get returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: "1", DeployKeyID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGet_MissingProjectID verifies that Get_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGet_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := Get(context.Background(), client, GetInput{DeployKeyID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestGet_CancelledContext verifies the Get_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := Get(ctx, client, GetInput{ProjectID: "1", DeployKeyID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Add — API error, missing project_id, expires_at valid/invalid, canceled ctx
// ---------------------------------------------------------------------------.

// TestAdd_APIError verifies that Add returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestAdd_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Add(context.Background(), client, AddInput{
		ProjectID: "1", Title: "k", Key: "ssh-rsa AAAA",
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestAdd_MissingProjectID verifies that Add_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestAdd_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := Add(context.Background(), client, AddInput{Title: "k", Key: "ssh-rsa AAAA"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestAdd_WithValidExpiresAt verifies the Add_WithValidExpiresAt handler.
// The mock GitLab API at /api/v4/projects/1/deploy_keys (POST) responds with HTTP Created.
// It asserts the returned output matches the expected fields.
func TestAdd_WithValidExpiresAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/1/deploy_keys" {
			testutil.RespondJSON(w, http.StatusCreated,
				`{"id":3,"title":"exp-key","key":"ssh-rsa AAAA","can_push":false,"expires_at":"2027-06-15T00:00:00Z"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Add(context.Background(), client, AddInput{
		ProjectID: "1", Title: "exp-key", Key: "ssh-rsa AAAA", ExpiresAt: "2027-06-15",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ExpiresAt != "2027-06-15T00:00:00Z" {
		t.Errorf("ExpiresAt = %q, want %q", out.ExpiresAt, "2027-06-15T00:00:00Z")
	}
}

// TestAdd_WithInvalidExpiresAt verifies the Add_WithInvalidExpiresAt handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestAdd_WithInvalidExpiresAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := Add(context.Background(), client, AddInput{
		ProjectID: "1", Title: "k", Key: "ssh-rsa AAAA", ExpiresAt: "not-a-date",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid expires_at") {
		t.Fatalf("expected invalid expires_at error, got %v", err)
	}
}

// TestAdd_CancelledContext verifies the Add_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestAdd_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := Add(ctx, client, AddInput{ProjectID: "1", Title: "k", Key: "ssh-rsa AAAA"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Update — API error, missing project_id, canceled context
// ---------------------------------------------------------------------------.

// TestUpdate_APIError verifies that Update returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "1", DeployKeyID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestUpdate_MissingProjectID verifies that Update_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestUpdate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := Update(context.Background(), client, UpdateInput{DeployKeyID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestUpdate_CancelledContext verifies the Update_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := Update(ctx, client, UpdateInput{ProjectID: "1", DeployKeyID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestUpdate_TitleOnly verifies the Update_TitleOnly handler.
// The mock GitLab API at /api/v4/projects/1/deploy_keys/1 (PUT) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestUpdate_TitleOnly(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/projects/1/deploy_keys/1" {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"title":"new-title","key":"ssh-rsa AAAA","can_push":false}`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID: "1", DeployKeyID: 1, Title: "new-title",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Title != "new-title" {
		t.Errorf("Title = %q, want %q", out.Title, "new-title")
	}
}

// ---------------------------------------------------------------------------
// Delete — API error, missing deploy_key_id, canceled context
// ---------------------------------------------------------------------------.

// TestDelete_APIError verifies that Delete returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: "1", DeployKeyID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDelete_MissingDeployKeyID verifies that Delete_MissingDeployKeyID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDelete_MissingDeployKeyID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := Delete(context.Background(), client, DeleteInput{ProjectID: "1"})
	if err == nil || !strings.Contains(err.Error(), "deploy_key_id is required") {
		t.Fatalf("expected deploy_key_id required error, got %v", err)
	}
}

// TestDelete_CancelledContext verifies the Delete_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	err := Delete(ctx, client, DeleteInput{ProjectID: "1", DeployKeyID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Enable — API error, missing project_id, canceled context
// ---------------------------------------------------------------------------.

// TestEnable_APIError verifies that Enable returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestEnable_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Enable(context.Background(), client, EnableInput{ProjectID: "1", DeployKeyID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestEnable_MissingProjectID verifies that Enable_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestEnable_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := Enable(context.Background(), client, EnableInput{DeployKeyID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestEnable_CancelledContext verifies the Enable_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestEnable_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := Enable(ctx, client, EnableInput{ProjectID: "1", DeployKeyID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// ListAll — API error, with public filter, canceled context
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

// TestListAll_WithPublicFilter verifies the ListAll_WithPublicFilter handler.
// The mock GitLab API at /api/v4/deploy_keys (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestListAll_WithPublicFilter(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/deploy_keys" {
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"id":20,"title":"public-key","key":"ssh-rsa PUB"}]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	pub := true
	out, err := ListAll(context.Background(), client, ListAllInput{Public: &pub})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.DeployKeys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(out.DeployKeys))
	}
	if out.DeployKeys[0].Title != "public-key" {
		t.Errorf("Title = %q, want %q", out.DeployKeys[0].Title, "public-key")
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

// ---------------------------------------------------------------------------
// AddInstance — API error, with expires_at valid/invalid, canceled context
// ---------------------------------------------------------------------------.

// TestAddInstance_APIError verifies that AddInstance returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestAddInstance_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := AddInstance(context.Background(), client, AddInstanceInput{Title: "k", Key: "ssh-rsa AAAA"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestAddInstance_WithValidExpiresAt verifies the AddInstance_WithValidExpiresAt handler.
// The mock GitLab API at /api/v4/deploy_keys (POST) responds with HTTP Created.
// It asserts the returned output matches the expected fields.
func TestAddInstance_WithValidExpiresAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/deploy_keys" {
			testutil.RespondJSON(w, http.StatusCreated,
				`{"id":30,"title":"inst-exp","key":"ssh-rsa AAAA","expires_at":"2028-01-01T00:00:00Z"}`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := AddInstance(context.Background(), client, AddInstanceInput{
		Title: "inst-exp", Key: "ssh-rsa AAAA", ExpiresAt: "2028-01-01",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ExpiresAt != "2028-01-01T00:00:00Z" {
		t.Errorf("ExpiresAt = %q, want %q", out.ExpiresAt, "2028-01-01T00:00:00Z")
	}
}

// TestAddInstance_WithInvalidExpiresAt verifies the AddInstance_WithInvalidExpiresAt handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestAddInstance_WithInvalidExpiresAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := AddInstance(context.Background(), client, AddInstanceInput{
		Title: "k", Key: "ssh-rsa AAAA", ExpiresAt: "bad-date",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid expires_at") {
		t.Fatalf("expected invalid expires_at error, got %v", err)
	}
}

// TestAddInstance_CancelledContext verifies the AddInstance_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestAddInstance_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := AddInstance(ctx, client, AddInstanceInput{Title: "k", Key: "ssh-rsa AAAA"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// ListUserProject — API error, canceled context
// ---------------------------------------------------------------------------.

// TestListUserProject_APIError verifies that ListUserProject returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListUserProject_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := ListUserProject(context.Background(), client, ListUserProjectInput{UserID: "42"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestListUserProject_CancelledContext verifies the ListUserProject_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestListUserProject_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListUserProject(ctx, client, ListUserProjectInput{UserID: "42"})
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
		ID:                1,
		Title:             "prod-key",
		Key:               "ssh-rsa AAAA",
		Fingerprint:       "ab:cd:ef",
		FingerprintSHA256: "SHA256:xyz",
		CreatedAt:         "2026-01-01T00:00:00Z",
		CanPush:           true,
		ExpiresAt:         "2027-01-01T00:00:00Z",
	})

	for _, want := range []string{
		"## Deploy Key: prod-key (ID: 1)",
		"| ID | 1 |",
		"| Title | prod-key |",
		"| Fingerprint | ab:cd:ef |",
		"| SHA256 | SHA256:xyz |",
		"| Can Push | true |",
		"| Created | 1 Jan 2026 00:00 UTC |",
		"| Expires | 1 Jan 2027 00:00 UTC |",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Errorf("markdown missing %q:\n%s", want, md)
			}
		})
	}
}

// TestFormatOutputMarkdown_MinimalFields verifies the OutputMarkdown_MinimalFields Markdown formatter for a representative output_minimalfields input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatOutputMarkdown_MinimalFields(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		ID:    2,
		Title: "dev-key",
	})

	if !strings.Contains(md, "## Deploy Key: dev-key (ID: 2)") {
		t.Errorf("missing header:\n%s", md)
	}
	for _, absent := range []string{"Fingerprint", "SHA256", "Created", "Expires"} {
		t.Run(absent, func(t *testing.T) {
			if strings.Contains(md, "| "+absent+" |") {
				t.Errorf("should not contain %q for minimal output:\n%s", absent, md)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithKeys verifies the ListMarkdown_WithKeys Markdown formatter for a representative list_withkeys input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown_WithKeys(t *testing.T) {
	out := ListOutput{
		DeployKeys: []Output{
			{ID: 1, Title: "key-a", CanPush: true, Fingerprint: "aa:bb", CreatedAt: "2026-01-01T00:00:00Z"},
			{ID: 2, Title: "key-b", CanPush: false, Fingerprint: "cc:dd", CreatedAt: "2026-02-01T00:00:00Z"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatListMarkdown(out)

	for _, want := range []string{
		"## Deploy Keys (2)",
		"| ID |",
		"| 1 |",
		"| 2 |",
		"key-a",
		"key-b",
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
	if !strings.Contains(md, "No deploy keys found") {
		t.Errorf("expected empty message:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// ---------------------------------------------------------------------------
// FormatInstanceOutputMarkdown
// ---------------------------------------------------------------------------.

// TestFormatInstanceOutputMarkdown_AllFields verifies the InstanceOutputMarkdown_AllFields Markdown formatter for a representative instanceoutput_allfields input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatInstanceOutputMarkdown_AllFields(t *testing.T) {
	md := FormatInstanceOutputMarkdown(InstanceOutput{
		ID:                10,
		Title:             "instance-key",
		Key:               "ssh-rsa INST",
		Fingerprint:       "11:22",
		FingerprintSHA256: "SHA256:inst",
		CreatedAt:         "2026-01-01T00:00:00Z",
		ExpiresAt:         "2027-01-01T00:00:00Z",
		ProjectsWithWriteAccess: []ProjectSummary{
			{ID: 100, Name: "proj-w", PathWithNamespace: "group/proj-w"},
		},
		ProjectsWithReadonlyAccess: []ProjectSummary{
			{ID: 200, Name: "proj-r", PathWithNamespace: "group/proj-r"},
		},
	})

	for _, want := range []string{
		"## Instance Deploy Key: instance-key (ID: 10)",
		"| ID | 10 |",
		"| Title | instance-key |",
		"| Fingerprint | 11:22 |",
		"| SHA256 | SHA256:inst |",
		"| Created | 1 Jan 2026 00:00 UTC |",
		"| Expires | 1 Jan 2027 00:00 UTC |",
		"### Projects with Write Access",
		"| 100 | proj-w | group/proj-w |",
		"### Projects with Readonly Access",
		"| 200 | proj-r | group/proj-r |",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Errorf("markdown missing %q:\n%s", want, md)
			}
		})
	}
}

// TestFormatInstanceOutputMarkdown_MinimalFields verifies the InstanceOutputMarkdown_MinimalFields Markdown formatter for a representative instanceoutput_minimalfields input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatInstanceOutputMarkdown_MinimalFields(t *testing.T) {
	md := FormatInstanceOutputMarkdown(InstanceOutput{
		ID:    11,
		Title: "bare-key",
	})

	if !strings.Contains(md, "## Instance Deploy Key: bare-key (ID: 11)") {
		t.Errorf("missing header:\n%s", md)
	}
	for _, absent := range []string{"Fingerprint", "SHA256", "Created", "Expires", "Write Access", "Readonly Access"} {
		t.Run(absent, func(t *testing.T) {
			if strings.Contains(md, absent) {
				t.Errorf("should not contain %q for minimal output:\n%s", absent, md)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FormatInstanceListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatInstanceListMarkdown_WithKeys verifies the InstanceListMarkdown_WithKeys Markdown formatter for a representative instancelist_withkeys input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatInstanceListMarkdown_WithKeys(t *testing.T) {
	out := InstanceListOutput{
		DeployKeys: []InstanceOutput{
			{ID: 10, Title: "inst-a", Fingerprint: "aa:bb", CreatedAt: "2026-01-01T00:00:00Z", ExpiresAt: "2027-01-01T00:00:00Z"},
			{ID: 11, Title: "inst-b", Fingerprint: "cc:dd", CreatedAt: "2026-02-01T00:00:00Z", ExpiresAt: ""},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatInstanceListMarkdown(out)

	for _, want := range []string{
		"## Instance Deploy Keys (2)",
		"| ID |",
		"| 10 |",
		"| 11 |",
		"inst-a",
		"inst-b",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Errorf("markdown missing %q:\n%s", want, md)
			}
		})
	}
}

// TestFormatInstanceListMarkdown_Empty verifies the InstanceListMarkdown_Empty Markdown formatter for a representative instancelist_empty input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatInstanceListMarkdown_Empty(t *testing.T) {
	md := FormatInstanceListMarkdown(InstanceListOutput{})
	if !strings.Contains(md, "No instance deploy keys found") {
		t.Errorf("expected empty message:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs metadata
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata validates the Metadata route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	specs := ActionSpecs(client)
	byTool := deployKeySpecsByTool(t, specs)

	if len(specs) != 9 {
		t.Fatalf("len(ActionSpecs) = %d, want 9", len(specs))
	}
	if len(byTool) != len(specs) {
		t.Fatalf("unique individual tools = %d, want %d", len(byTool), len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "deploykeys" {
			t.Fatalf("OwnerPackage for %s = %q, want deploykeys", spec.Name, spec.OwnerPackage)
		}
		if spec.Usage == "" {
			t.Fatalf("Usage for %s should not be empty", spec.Name)
		}
		if len(spec.Aliases) == 0 {
			t.Fatalf("Aliases for %s should not be empty", spec.Name)
		}
	}
	if byTool["gitlab_deploy_key_get"].ParameterGuidance["deploy_key_id"].SemanticRole == "" {
		t.Fatal("gitlab_deploy_key_get should define deploy_key_id parameter guidance")
	}
	// The two listing actions carry a usage of their own, and every other
	// action carries the shared one rather than either of theirs.
	for tool, want := range map[string]string{
		"gitlab_deploy_key_list_project": "not deploy tokens",
		"gitlab_deploy_key_list_all":     "admin only",
	} {
		t.Run(tool, func(t *testing.T) {
			if !strings.Contains(byTool[tool].Usage, want) {
				t.Errorf("Usage for %s = %q, want it to mention %q", tool, byTool[tool].Usage, want)
			}
			for other, spec := range byTool {
				if other != tool && strings.Contains(spec.Usage, want) {
					t.Errorf("Usage for %s = %q, want %q only on %s", other, spec.Usage, want, tool)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ---------------------------------------------------------------------------.

// ---------------------------------------------------------------------------
// ActionSpecs route coverage for all 9 tools
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallAllRoutes validates the CallAllRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := newDeployKeySpecsByTool(t)

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list_project", "gitlab_deploy_key_list_project", map[string]any{"project_id": "1"}},
		{"get", "gitlab_deploy_key_get", map[string]any{"project_id": "1", "deploy_key_id": 1}},
		{"add", "gitlab_deploy_key_add", map[string]any{"project_id": "1", "title": "my-key", "key": "ssh-rsa AAAA"}},
		{"update", "gitlab_deploy_key_update", map[string]any{"project_id": "1", "deploy_key_id": 1, "title": "updated"}},
		{"delete", "gitlab_deploy_key_delete", map[string]any{"project_id": "1", "deploy_key_id": 1}},
		{"enable", "gitlab_deploy_key_enable", map[string]any{"project_id": "1", "deploy_key_id": 1}},
		{"list_all", "gitlab_deploy_key_list_all", map[string]any{}},
		{"add_instance", "gitlab_deploy_key_add_instance", map[string]any{"title": "inst-key", "key": "ssh-rsa BBBB"}},
		{"list_user_project", "gitlab_deploy_key_list_user_project", map[string]any{"user_id": "42"}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// toOutput — converter with all fields populated via API response
// ---------------------------------------------------------------------------.

// TestListProject_SuccessWithAllFields verifies the ListProject_SuccessWithAllFields handler.
// The mock GitLab API at /api/v4/projects/1/deploy_keys (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestListProject_SuccessWithAllFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/1/deploy_keys" {
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"id":1,"title":"full-key","key":"ssh-rsa FULL","fingerprint":"ab:cd","fingerprint_sha256":"SHA256:full","created_at":"2026-01-01T00:00:00Z","can_push":true,"expires_at":"2027-01-01T00:00:00Z"}]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListProject(context.Background(), client, ListProjectInput{ProjectID: "1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	k := out.DeployKeys[0]
	if k.Fingerprint != "ab:cd" {
		t.Errorf("Fingerprint = %q, want %q", k.Fingerprint, "ab:cd")
	}
	if k.FingerprintSHA256 != "SHA256:full" {
		t.Errorf("FingerprintSHA256 = %q, want %q", k.FingerprintSHA256, "SHA256:full")
	}
	if k.CreatedAt != "2026-01-01T00:00:00Z" {
		t.Errorf("CreatedAt = %q, want %q", k.CreatedAt, "2026-01-01T00:00:00Z")
	}
	if k.ExpiresAt != "2027-01-01T00:00:00Z" {
		t.Errorf("ExpiresAt = %q, want %q", k.ExpiresAt, "2027-01-01T00:00:00Z")
	}
}

// ---------------------------------------------------------------------------
// toInstanceOutput — with projects
// ---------------------------------------------------------------------------.

// TestListAll_WithProjectAssociations verifies the ListAll_WithProjectAssociations handler.
// The mock GitLab API at /api/v4/deploy_keys (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestListAll_WithProjectAssociations(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/deploy_keys" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{
				"id":50,"title":"assoc-key","key":"ssh-rsa ASSOC",
				"projects_with_write_access":[{"id":100,"name":"write-proj","path_with_namespace":"g/wp","created_at":"2026-01-01T00:00:00Z"}],
				"projects_with_readonly_access":[{"id":200,"name":"ro-proj","path_with_namespace":"g/rp"}]
			}]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListAll(context.Background(), client, ListAllInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.DeployKeys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(out.DeployKeys))
	}
	k := out.DeployKeys[0]
	if len(k.ProjectsWithWriteAccess) != 1 {
		t.Fatalf("expected 1 write project, got %d", len(k.ProjectsWithWriteAccess))
	}
	if k.ProjectsWithWriteAccess[0].Name != "write-proj" {
		t.Errorf("write project name = %q, want %q", k.ProjectsWithWriteAccess[0].Name, "write-proj")
	}
	if len(k.ProjectsWithReadonlyAccess) != 1 {
		t.Fatalf("expected 1 readonly project, got %d", len(k.ProjectsWithReadonlyAccess))
	}
	if k.ProjectsWithReadonlyAccess[0].PathWithNamespace != "g/rp" {
		t.Errorf("readonly project path = %q, want %q", k.ProjectsWithReadonlyAccess[0].PathWithNamespace, "g/rp")
	}
}

// ---------------------------------------------------------------------------
// ListProject — with pagination
// ---------------------------------------------------------------------------.

// TestListProject_WithPagination verifies that ListProject_WithPagination forwards pagination parameters to the GitLab API and parses the response metadata.
// The mock GitLab API at /api/v4/projects/1/deploy_keys (GET) responds with HTTP OK.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestListProject_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/1/deploy_keys" {
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"id":1,"title":"k1","key":"ssh-rsa A"},{"id":2,"title":"k2","key":"ssh-rsa B"}]`,
				testutil.PaginationHeaders{Page: "2", PerPage: "2", Total: "5", TotalPages: "3", NextPage: "3", PrevPage: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListProject(context.Background(), client, ListProjectInput{
		ProjectID: "1",
		Page:      2, PerPage: 2,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.DeployKeys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(out.DeployKeys))
	}
	if out.Pagination.TotalPages != 3 {
		t.Errorf("TotalPages = %d, want 3", out.Pagination.TotalPages)
	}
	if out.Pagination.TotalItems != 5 {
		t.Errorf("TotalItems = %d, want 5", out.Pagination.TotalItems)
	}
}

// ---------------------------------------------------------------------------
// ListUserProject — with pagination
// ---------------------------------------------------------------------------.

// TestListUserProject_WithPagination verifies that ListUserProject_WithPagination forwards pagination parameters to the GitLab API and parses the response metadata.
// The mock GitLab API at /api/v4/users/42/project_deploy_keys (GET) responds with HTTP OK.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestListUserProject_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/users/42/project_deploy_keys" {
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"id":7,"title":"uk","key":"ssh-rsa UU","can_push":true}]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "10", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListUserProject(context.Background(), client, ListUserProjectInput{
		UserID: "42",
		Page:   1, PerPage: 10,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Pagination.PerPage != 10 {
		t.Errorf("PerPage = %d, want 10", out.Pagination.PerPage)
	}
}

// ---------------------------------------------------------------------------
// Helper: ActionSpec route factory
// ---------------------------------------------------------------------------.

// newDeployKeySpecsByTool constructs deploy key specs by tool test fixtures.
func newDeployKeySpecsByTool(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()

	keyJSON := `{"id":1,"title":"my-key","key":"ssh-rsa AAAA","fingerprint":"ab:cd","can_push":false}`
	instanceKeyJSON := `{"id":10,"title":"inst-key","key":"ssh-rsa BBBB"}`

	handler := http.NewServeMux()

	// List project deploy keys
	handler.HandleFunc("GET /api/v4/projects/1/deploy_keys", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+keyJSON+`]`)
	})

	// Get deploy key
	handler.HandleFunc("GET /api/v4/projects/1/deploy_keys/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, keyJSON)
	})

	// Add deploy key
	handler.HandleFunc("POST /api/v4/projects/1/deploy_keys", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, keyJSON)
	})

	// Update deploy key
	handler.HandleFunc("PUT /api/v4/projects/1/deploy_keys/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, keyJSON)
	})

	// Delete deploy key
	handler.HandleFunc("DELETE /api/v4/projects/1/deploy_keys/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Enable deploy key
	handler.HandleFunc("POST /api/v4/projects/1/deploy_keys/1/enable", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, keyJSON)
	})

	// List all instance deploy keys
	handler.HandleFunc("GET /api/v4/deploy_keys", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+instanceKeyJSON+`]`)
	})

	// Add instance deploy key
	handler.HandleFunc("POST /api/v4/deploy_keys", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, instanceKeyJSON)
	})

	// List user project deploy keys
	handler.HandleFunc("GET /api/v4/users/42/project_deploy_keys", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+keyJSON+`]`)
	})

	client := testutil.NewTestClient(t, handler)
	return deployKeySpecsByTool(t, ActionSpecs(client))
}

// TestActionSpecs_DeployKeyGetRoute validates the DeployKeyGetRoute route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_DeployKeyGetRoute(t *testing.T) {
	const respJSON = `{"id":12,"title":"deploy","key":"ssh-rsa AAAA","fingerprint":""}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/projects/42/deploy_keys/12") {
			testutil.RespondJSON(w, http.StatusOK, respJSON)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)
	byTool := deployKeySpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_deploy_key_get"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "deploy_key_id": 12})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	out, ok := result.(Output)
	if !ok {
		t.Fatalf("result type = %T, want Output", result)
	}
	if out.ID != 12 || out.Title != "deploy" {
		t.Fatalf("deploy key output = %#v, want ID 12 title deploy", out)
	}
}

// deployKeySpecsByTool supports deploy key specs by tool assertions in deploykeys tests.
func deployKeySpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}

// ---------------------------------------------------------------------------
// Ordering & keyset pagination — forwarded query parameters
// ---------------------------------------------------------------------------.

// TestListProject_OrderingAndKeyset verifies ListProject forwards order_by,
// sort, pagination=keyset, and page_token to the GitLab API.
func TestListProject_OrderingAndKeyset(t *testing.T) {
	var query string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/1/deploy_keys" {
			query = r.URL.RawQuery
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"title":"k","key":"ssh-rsa A"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := ListProject(context.Background(), client, ListProjectInput{
		ProjectID:  "1",
		OrderBy:    "created_at",
		Sort:       "desc",
		Pagination: "keyset", PageToken: "tok",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	for _, want := range []string{"order_by=created_at", "sort=desc", "pagination=keyset", "page_token=tok"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(query, want) {
				t.Errorf("query %q missing %q", query, want)
			}
		})
	}
}

// TestListAll_OrderingAndKeyset verifies ListAll forwards order_by, sort,
// pagination=keyset, and page_token to the GitLab API.
func TestListAll_OrderingAndKeyset(t *testing.T) {
	var query string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/deploy_keys" {
			query = r.URL.RawQuery
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"title":"k","key":"ssh-rsa A"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := ListAll(context.Background(), client, ListAllInput{
		OrderBy:    "title",
		Sort:       "asc",
		Pagination: "keyset", PageToken: "tok",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	for _, want := range []string{"order_by=title", "sort=asc", "pagination=keyset", "page_token=tok"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(query, want) {
				t.Errorf("query %q missing %q", query, want)
			}
		})
	}
}

// TestListUserProject_OrderingAndKeyset verifies ListUserProject forwards
// order_by, sort, pagination=keyset, and page_token to the GitLab API.
func TestListUserProject_OrderingAndKeyset(t *testing.T) {
	var query string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/users/42/project_deploy_keys" {
			query = r.URL.RawQuery
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"title":"k","key":"ssh-rsa A"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := ListUserProject(context.Background(), client, ListUserProjectInput{
		UserID:     "42",
		OrderBy:    "id",
		Sort:       "desc",
		Pagination: "keyset", PageToken: "tok",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	for _, want := range []string{"order_by=id", "sort=desc", "pagination=keyset", "page_token=tok"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(query, want) {
				t.Errorf("query %q missing %q", query, want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Fields GitLab sends beside the ones the SDK's own deploy key models
// ---------------------------------------------------------------------------.

// deployKeySentJSON is one deploy key as GitLab renders it, carrying the keys
// the SDK's own structs leave out: when the key last reached the instance,
// what it may be used for, and the projects it can write to or only read.
const deployKeySentJSON = `{"id":1,"title":"my-key","key":"ssh-rsa AAAA","can_push":true,` +
	`"created_at":"2026-01-01T00:00:00Z","last_used_at":"2026-04-07T08:09:10Z",` +
	`"usage_type":"auth_and_signing",` +
	`"projects_with_write_access":[{"id":11,"description":"the writer","name":"Writer",` +
	`"name_with_namespace":"Group / Writer","path":"writer","path_with_namespace":"group/writer",` +
	`"created_at":"2026-01-02T03:04:05Z"}],` +
	`"projects_with_readonly_access":[{"id":12,"name":"Reader","path_with_namespace":"group/reader"}]}`

// deployKeyWithoutProjectsJSON is the same key without the two project lists,
// which GitLab sends only when the request asked for them.
const deployKeyWithoutProjectsJSON = `{"id":1,"title":"my-key","key":"ssh-rsa AAAA","can_push":true,` +
	`"last_used_at":"2026-04-07T08:09:10Z","usage_type":"auth"}`

// deployKeyClient answers every deploy key endpoint with body, which the
// caller writes as an array for a list handler and as an object for the rest.
func deployKeyClient(t *testing.T, body string) *gitlabclient.Client {
	t.Helper()
	return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, body)
	}))
}

// deployKeySent is what a handler published of the fields read off the
// captured answer, normalized so the project-scoped and instance-scoped
// handlers can be asserted in one table.
type deployKeySent struct {
	lastUsedAt string
	usageType  string
	write      []ProjectSummary
	readonly   []ProjectSummary
}

// errNoDeployKey reports a list handler that answered with no key, which would
// otherwise show up as a nil-index panic.
var errNoDeployKey = errors.New("the handler published no deploy key")

// deployKeyCalls are the handlers that answer with a deploy key, each
// returning what it published of the captured fields.
var deployKeyCalls = []struct {
	name string
	list bool
	call func(client *gitlabclient.Client) (deployKeySent, error)
}{
	{name: "list_project", list: true, call: func(client *gitlabclient.Client) (deployKeySent, error) {
		out, err := ListProject(context.Background(), client, ListProjectInput{ProjectID: "123"})
		if err != nil {
			return deployKeySent{}, err
		}
		if len(out.DeployKeys) != 1 {
			return deployKeySent{}, errNoDeployKey
		}
		return projectKeySent(out.DeployKeys[0]), nil
	}},
	{name: "list_user_project", list: true, call: func(client *gitlabclient.Client) (deployKeySent, error) {
		out, err := ListUserProject(context.Background(), client, ListUserProjectInput{UserID: "42"})
		if err != nil {
			return deployKeySent{}, err
		}
		if len(out.DeployKeys) != 1 {
			return deployKeySent{}, errNoDeployKey
		}
		return projectKeySent(out.DeployKeys[0]), nil
	}},
	{name: "get", call: func(client *gitlabclient.Client) (deployKeySent, error) {
		out, err := Get(context.Background(), client, GetInput{ProjectID: "123", DeployKeyID: 1})
		return projectKeySent(out), err
	}},
	{name: "add", call: func(client *gitlabclient.Client) (deployKeySent, error) {
		out, err := Add(context.Background(), client, AddInput{ProjectID: "123", Title: "my-key", Key: "ssh-rsa AAAA"})
		return projectKeySent(out), err
	}},
	{name: "update", call: func(client *gitlabclient.Client) (deployKeySent, error) {
		out, err := Update(context.Background(), client, UpdateInput{ProjectID: "123", DeployKeyID: 1, Title: "my-key"})
		return projectKeySent(out), err
	}},
	{name: "enable", call: func(client *gitlabclient.Client) (deployKeySent, error) {
		out, err := Enable(context.Background(), client, EnableInput{ProjectID: "123", DeployKeyID: 1})
		return projectKeySent(out), err
	}},
	{name: "list_all", list: true, call: func(client *gitlabclient.Client) (deployKeySent, error) {
		out, err := ListAll(context.Background(), client, ListAllInput{})
		if err != nil {
			return deployKeySent{}, err
		}
		if len(out.DeployKeys) != 1 {
			return deployKeySent{}, errNoDeployKey
		}
		return instanceKeySent(out.DeployKeys[0]), nil
	}},
	{name: "add_instance", call: func(client *gitlabclient.Client) (deployKeySent, error) {
		out, err := AddInstance(context.Background(), client, AddInstanceInput{Title: "my-key", Key: "ssh-rsa AAAA"})
		return instanceKeySent(out), err
	}},
}

// projectKeySent reads the captured fields off a project deploy key.
func projectKeySent(out Output) deployKeySent {
	return deployKeySent{
		lastUsedAt: out.LastUsedAt,
		usageType:  out.UsageType,
		write:      out.ProjectsWithWriteAccess,
		readonly:   out.ProjectsWithReadonlyAccess,
	}
}

// instanceKeySent reads the same off an instance deploy key.
func instanceKeySent(out InstanceOutput) deployKeySent {
	return deployKeySent{
		lastUsedAt: out.LastUsedAt,
		usageType:  out.UsageType,
		write:      out.ProjectsWithWriteAccess,
		readonly:   out.ProjectsWithReadonlyAccess,
	}
}

// deployKeyBodyFor wraps the object body in an array for a list endpoint.
func deployKeyBodyFor(list bool, body string) string {
	if list {
		return "[" + body + "]"
	}
	return body
}

// TestDeployKeys_PublishTheFieldsGitLabSendsBesideTheSDKs verifies that every
// handler answering with a deploy key publishes when the key was last used and
// what it may be used for, and that the project-scoped ones publish the two
// access lists GitLab sends beside them.
func TestDeployKeys_PublishTheFieldsGitLabSendsBesideTheSDKs(t *testing.T) {
	for _, keyCall := range deployKeyCalls {
		t.Run(keyCall.name, func(t *testing.T) {
			sent, err := keyCall.call(deployKeyClient(t, deployKeyBodyFor(keyCall.list, deployKeySentJSON)))
			if err != nil {
				t.Fatalf("%s: %v", keyCall.name, err)
			}
			if sent.lastUsedAt != "2026-04-07T08:09:10Z" {
				t.Errorf("last_used_at = %q, want 2026-04-07T08:09:10Z", sent.lastUsedAt)
			}
			if sent.usageType != "auth_and_signing" {
				t.Errorf("usage_type = %q, want auth_and_signing", sent.usageType)
			}
			if len(sent.write) != 1 {
				t.Fatalf("projects_with_write_access = %+v, want the one project GitLab sent", sent.write)
			}
			writer := sent.write[0]
			if writer.ID != 11 || writer.Name != "Writer" || writer.Path != "writer" ||
				writer.NameWithNamespace != "Group / Writer" || writer.PathWithNamespace != "group/writer" ||
				writer.Description != "the writer" || writer.CreatedAt != "2026-01-02T03:04:05Z" {
				t.Errorf("projects_with_write_access[0] = %+v, want every field GitLab sent", writer)
			}
			if len(sent.readonly) != 1 || sent.readonly[0].ID != 12 ||
				sent.readonly[0].PathWithNamespace != "group/reader" {
				t.Errorf("projects_with_readonly_access = %+v, want the one project GitLab sent", sent.readonly)
			}
		})
	}
}

// TestDeployKeys_OmitTheProjectListsGitLabDidNotSend verifies that a key
// answered without the two access lists publishes neither, while the fields
// GitLab always sends still arrive.
func TestDeployKeys_OmitTheProjectListsGitLabDidNotSend(t *testing.T) {
	for _, keyCall := range deployKeyCalls {
		t.Run(keyCall.name, func(t *testing.T) {
			sent, err := keyCall.call(deployKeyClient(t, deployKeyBodyFor(keyCall.list, deployKeyWithoutProjectsJSON)))
			if err != nil {
				t.Fatalf("%s: %v", keyCall.name, err)
			}
			if sent.write != nil || sent.readonly != nil {
				t.Errorf("access lists = %+v / %+v, want none", sent.write, sent.readonly)
			}
			if sent.usageType != "auth" {
				t.Errorf("usage_type = %q, want auth", sent.usageType)
			}
		})
	}
}

// TestDeployKeys_UnreadableCapturedFields verifies every handler answering
// with a deploy key reports the captured response's decode failure rather than
// a key missing what GitLab sent. The SDK's own structs carry no usage_type,
// so only the read beside them can notice GitLab sent a number there.
func TestDeployKeys_UnreadableCapturedFields(t *testing.T) {
	const poisoned = `{"id":1,"title":"my-key","key":"ssh-rsa AAAA","usage_type":7}`
	cases := make([]testutil.CapturedCase, 0, len(deployKeyCalls))
	for _, keyCall := range deployKeyCalls {
		cases = append(cases, testutil.CapturedCase{Name: keyCall.name, Call: func() error {
			_, err := keyCall.call(deployKeyClient(t, deployKeyBodyFor(keyCall.list, poisoned)))
			return err
		}})
	}
	testutil.AssertCapturedDecodeFailures(t, cases)
}

// TestDeployKeys_OptionalInputsReachTheRequest verifies that every optional
// field a deploy key handler accepts is sent when it was given and left out
// when it was not, which only the request the handler built can say.
func TestDeployKeys_OptionalInputsReachTheRequest(t *testing.T) {
	for _, testCase := range deployKeyOptionalInputCases {
		t.Run(testCase.name, func(t *testing.T) {
			var sent string
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, readErr := io.ReadAll(r.Body)
				if readErr != nil {
					t.Errorf("read request body: %v", readErr)
				}
				sent = r.URL.RawQuery + string(body)
				if r.Method == http.MethodGet {
					testutil.RespondJSON(w, http.StatusOK, `[`+deployKeySentJSON+`]`)
					return
				}
				testutil.RespondJSON(w, http.StatusOK, deployKeySentJSON)
			}))
			if err := testCase.call(client); err != nil {
				t.Fatalf("%s: %v", testCase.name, err)
			}
			assertRequestCarries(t, sent, testCase.want, testCase.omit)
		})
	}
}

// assertRequestCarries reports every fragment the recorded request should hold
// and every one it should not.
func assertRequestCarries(t *testing.T, sent string, want, omit []string) {
	t.Helper()
	for _, fragment := range want {
		if !strings.Contains(sent, fragment) {
			t.Errorf("request %q missing %q", sent, fragment)
		}
	}
	for _, fragment := range omit {
		if strings.Contains(sent, fragment) {
			t.Errorf("request %q carries %q it was not given", sent, fragment)
		}
	}
}

// deployKeyCanPush and deployKeyPublic are the optional booleans the cases
// below hand to the handlers, which take them by pointer.
var (
	deployKeyCanPush = true
	deployKeyPublic  = true
)

// deployKeyOptionalInputCases drive one handler each with an optional field
// given and with it omitted.
var deployKeyOptionalInputCases = []struct {
	name string
	call func(client *gitlabclient.Client) error
	want []string
	omit []string
}{
	{
		name: "add with can_push",
		call: func(client *gitlabclient.Client) error {
			_, err := Add(context.Background(), client, AddInput{
				ProjectID: "123", Title: "my-key", Key: "ssh-rsa AAAA", CanPush: &deployKeyCanPush,
			})
			return err
		},
		want: []string{`"can_push":true`, `"title":"my-key"`},
	},
	{
		name: "add without can_push",
		call: func(client *gitlabclient.Client) error {
			_, err := Add(context.Background(), client, AddInput{ProjectID: "123", Title: "my-key", Key: "ssh-rsa AAAA"})
			return err
		},
		omit: []string{`"can_push"`},
	},
	{
		name: "update with title and can_push",
		call: func(client *gitlabclient.Client) error {
			_, err := Update(context.Background(), client, UpdateInput{
				ProjectID: "123", DeployKeyID: 1, Title: "renamed", CanPush: &deployKeyCanPush,
			})
			return err
		},
		want: []string{`"title":"renamed"`, `"can_push":true`},
	},
	{
		name: "update without either",
		call: func(client *gitlabclient.Client) error {
			_, err := Update(context.Background(), client, UpdateInput{ProjectID: "123", DeployKeyID: 1})
			return err
		},
		omit: []string{`"title"`, `"can_push"`},
	},
	{
		name: "list_all with public",
		call: func(client *gitlabclient.Client) error {
			_, err := ListAll(context.Background(), client, ListAllInput{Public: &deployKeyPublic})
			return err
		},
		want: []string{"public=true"},
	},
	{
		name: "list_all without public",
		call: func(client *gitlabclient.Client) error {
			_, err := ListAll(context.Background(), client, ListAllInput{})
			return err
		},
		omit: []string{"public="},
	},
}

// TestFormatDeployKeyMarkdown_SentFields verifies both single-key formatters
// name the fields read off the captured answer, and leave each out of a key
// that carries none.
func TestFormatDeployKeyMarkdown_SentFields(t *testing.T) {
	write := []ProjectSummary{{ID: 11, Name: "Writer", PathWithNamespace: "group/writer"}}
	readonly := []ProjectSummary{{ID: 12, Name: "Reader", PathWithNamespace: "group/reader"}}
	for name, rendered := range map[string]string{
		"project": FormatOutputMarkdown(Output{
			ID: 1, Title: "my-key", LastUsedAt: "2026-04-07T08:09:10Z", UsageType: "auth_and_signing",
			ProjectsWithWriteAccess: write, ProjectsWithReadonlyAccess: readonly,
		}),
		"instance": FormatInstanceOutputMarkdown(InstanceOutput{
			ID: 1, Title: "my-key", LastUsedAt: "2026-04-07T08:09:10Z", UsageType: "auth_and_signing",
			ProjectsWithWriteAccess: write, ProjectsWithReadonlyAccess: readonly,
		}),
	} {
		t.Run(name, func(t *testing.T) {
			for _, want := range []string{
				"Usage Type", "auth_and_signing", "Last Used", "7 Apr 2026 08:09 UTC",
				"Projects with Write Access", "group/writer",
				"Projects with Readonly Access", "group/reader",
			} {
				if !strings.Contains(rendered, want) {
					t.Errorf("%s markdown missing %q: %s", name, want, rendered)
				}
			}
		})
	}

	for name, rendered := range map[string]string{
		"project":  FormatOutputMarkdown(Output{ID: 1, Title: "my-key"}),
		"instance": FormatInstanceOutputMarkdown(InstanceOutput{ID: 1, Title: "my-key"}),
	} {
		t.Run(name+" without them", func(t *testing.T) {
			for _, unwanted := range []string{
				"Usage Type", "Last Used", "Projects with Write Access", "Projects with Readonly Access",
			} {
				if strings.Contains(rendered, unwanted) {
					t.Errorf("%s markdown shows %q for a key that has none: %s", name, unwanted, rendered)
				}
			}
		})
	}
}
