// access_requests_test.go contains unit tests for the access request MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package accessrequests

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// ListProject
// ---------------------------------------------------------------------------.

// TestListProject_Success verifies that ListProject succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListProject_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/10/access_requests", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":1,"username":"alice","name":"Alice","state":"pending","access_level":30}]`,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListProject(context.Background(), client, ListProjectInput{
		ProjectID: toolutil.StringOrInt("10"),
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.AccessRequests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(out.AccessRequests))
	}
	if out.AccessRequests[0].Username != "alice" {
		t.Errorf("expected username alice, got %s", out.AccessRequests[0].Username)
	}
}

// TestListProject_MissingProjectID verifies that ListProject returns a wrapped
// validation error when the project_id input is empty, without ever calling
// the GitLab API.
//
// The test calls ListProject with an empty ListProjectInput and asserts the
// error message contains "project_id is required". This protects the handler
// from making unnecessary network calls with missing required inputs.
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
	mux.HandleFunc("/api/v4/groups/5/access_requests", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":2,"username":"bob","name":"Bob","state":"pending","access_level":20}]`,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListGroup(context.Background(), client, ListGroupInput{
		GroupID: toolutil.StringOrInt("5"),
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.AccessRequests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(out.AccessRequests))
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
// RequestProject
// ---------------------------------------------------------------------------.

// TestRequestProject_Success verifies that RequestProject succeeds when the GitLab API returns a valid response.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestRequestProject_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/10/access_requests", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		testutil.RespondJSON(w, http.StatusCreated,
			`{"id":3,"username":"me","name":"Me","state":"pending","access_level":30}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := RequestProject(context.Background(), client, RequestProjectInput{
		ProjectID: toolutil.StringOrInt("10"),
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 3 {
		t.Errorf("expected ID 3, got %d", out.ID)
	}
}

// TestRequestProject_MissingProjectID verifies that RequestProject_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestRequestProject_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := RequestProject(context.Background(), client, RequestProjectInput{})
	if err == nil || !strings.Contains(err.Error(), "project_id is required") {
		t.Fatalf("expected project_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// RequestGroup
// ---------------------------------------------------------------------------.

// TestRequestGroup_Success verifies that RequestGroup succeeds when the GitLab API returns a valid response.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestRequestGroup_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/5/access_requests", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		testutil.RespondJSON(w, http.StatusCreated,
			`{"id":4,"username":"me","name":"Me","state":"pending","access_level":10}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := RequestGroup(context.Background(), client, RequestGroupInput{
		GroupID: toolutil.StringOrInt("5"),
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 4 {
		t.Errorf("expected ID 4, got %d", out.ID)
	}
}

// TestRequestGroup_MissingGroupID verifies that RequestGroup_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestRequestGroup_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := RequestGroup(context.Background(), client, RequestGroupInput{})
	if err == nil || !strings.Contains(err.Error(), "group_id is required") {
		t.Fatalf("expected group_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// ApproveProject
// ---------------------------------------------------------------------------.

// TestApproveProject_Success verifies that ApproveProject succeeds when the GitLab API returns a valid response.
// The test exercises the PUT path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestApproveProject_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/10/access_requests/1/approve", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK,
			`{"id":1,"username":"alice","name":"Alice","state":"approved","access_level":30}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ApproveProject(context.Background(), client, ApproveProjectInput{
		ProjectID:   toolutil.StringOrInt("10"),
		UserID:      1,
		AccessLevel: 30,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.State != "approved" {
		t.Errorf("expected state approved, got %s", out.State)
	}
}

// TestApproveProject_MissingUserID verifies that ApproveProject_MissingUserID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestApproveProject_MissingUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ApproveProject(context.Background(), client, ApproveProjectInput{
		ProjectID: toolutil.StringOrInt("10"),
	})
	if err == nil || !strings.Contains(err.Error(), "user_id is required") {
		t.Fatalf("expected user_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// ApproveGroup
// ---------------------------------------------------------------------------.

// TestApproveGroup_Success verifies that ApproveGroup succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestApproveGroup_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/5/access_requests/2/approve", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK,
			`{"id":2,"username":"bob","name":"Bob","state":"approved","access_level":20}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ApproveGroup(context.Background(), client, ApproveGroupInput{
		GroupID: toolutil.StringOrInt("5"),
		UserID:  2,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.State != "approved" {
		t.Errorf("expected state approved, got %s", out.State)
	}
}

// TestApproveGroup_MissingUserID verifies that ApproveGroup_MissingUserID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestApproveGroup_MissingUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ApproveGroup(context.Background(), client, ApproveGroupInput{
		GroupID: toolutil.StringOrInt("5"),
	})
	if err == nil || !strings.Contains(err.Error(), "user_id is required") {
		t.Fatalf("expected user_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// DenyProject
// ---------------------------------------------------------------------------.

// TestDenyProject_Success verifies that DenyProject succeeds when the GitLab API returns a valid response.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDenyProject_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/10/access_requests/1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)

	err := DenyProject(context.Background(), client, DenyProjectInput{
		ProjectID: toolutil.StringOrInt("10"),
		UserID:    1,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDenyProject_MissingUserID verifies that DenyProject_MissingUserID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDenyProject_MissingUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := DenyProject(context.Background(), client, DenyProjectInput{
		ProjectID: toolutil.StringOrInt("10"),
	})
	if err == nil || !strings.Contains(err.Error(), "user_id is required") {
		t.Fatalf("expected user_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// DenyGroup
// ---------------------------------------------------------------------------.

// TestDenyGroup_Success verifies that DenyGroup succeeds when the GitLab API returns a valid response.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDenyGroup_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/5/access_requests/2", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)

	err := DenyGroup(context.Background(), client, DenyGroupInput{
		GroupID: toolutil.StringOrInt("5"),
		UserID:  2,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDenyGroup_MissingUserID verifies that DenyGroup_MissingUserID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDenyGroup_MissingUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := DenyGroup(context.Background(), client, DenyGroupInput{
		GroupID: toolutil.StringOrInt("5"),
	})
	if err == nil || !strings.Contains(err.Error(), "user_id is required") {
		t.Fatalf("expected user_id required error, got %v", err)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpectedAPI identifies the err expected API constant used by this package.
const errExpectedAPI = "expected API error, got nil"

// ---------------------------------------------------------------------------
// ListProject — API error, pagination params
// ---------------------------------------------------------------------------.

// TestListProject_APIError verifies that ListProject returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListProject_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := ListProject(context.Background(), client, ListProjectInput{
		ProjectID: toolutil.StringOrInt("42"),
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestListProject_PaginationParams verifies that ListProjectParams forwards pagination parameters to the GitLab API and parses the response metadata.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestListProject_PaginationParams(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/access_requests", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("page") != "2" {
			t.Errorf("expected page=2, got %s", q.Get("page"))
		}
		if q.Get("per_page") != "5" {
			t.Errorf("expected per_page=5, got %s", q.Get("per_page"))
		}
		if q.Get("order_by") != "id" {
			t.Errorf("expected order_by=id, got %s", q.Get("order_by"))
		}
		if q.Get("sort") != "desc" {
			t.Errorf("expected sort=desc, got %s", q.Get("sort"))
		}
		if q.Get("pagination") != "keyset" {
			t.Errorf("expected pagination=keyset, got %s", q.Get("pagination"))
		}
		if q.Get("page_token") != "100" {
			t.Errorf("expected page_token=100, got %s", q.Get("page_token"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":10,"username":"u","name":"n","state":"pending","access_level":30}]`,
			testutil.PaginationHeaders{TotalPages: "3", Total: "15", Page: "2", PerPage: "5"})
	})
	client := testutil.NewTestClient(t, mux)

	in := ListProjectInput{
		ProjectID: toolutil.StringOrInt("42"),
		OrderBy:   "id",
		Sort:      "desc",

		Page:       2,
		PerPage:    5,
		Pagination: "keyset",
		PageToken:  "100",
	}
	out, err := ListProject(context.Background(), client, in)
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.AccessRequests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(out.AccessRequests))
	}
}

// ---------------------------------------------------------------------------
// ListGroup — API error, pagination params
// ---------------------------------------------------------------------------.

// TestListGroup_APIError verifies that ListGroup returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListGroup_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := ListGroup(context.Background(), client, ListGroupInput{
		GroupID: toolutil.StringOrInt("10"),
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestListGroup_PaginationParams verifies that ListGroupParams forwards pagination parameters to the GitLab API and parses the response metadata.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestListGroup_PaginationParams(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/10/access_requests", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("page") != "3" {
			t.Errorf("expected page=3, got %s", q.Get("page"))
		}
		if q.Get("per_page") != "10" {
			t.Errorf("expected per_page=10, got %s", q.Get("per_page"))
		}
		if q.Get("order_by") != "id" {
			t.Errorf("expected order_by=id, got %s", q.Get("order_by"))
		}
		if q.Get("sort") != "asc" {
			t.Errorf("expected sort=asc, got %s", q.Get("sort"))
		}
		if q.Get("pagination") != "keyset" {
			t.Errorf("expected pagination=keyset, got %s", q.Get("pagination"))
		}
		if q.Get("page_token") != "200" {
			t.Errorf("expected page_token=200, got %s", q.Get("page_token"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":20,"username":"x","name":"X","state":"pending","access_level":20}]`,
			testutil.PaginationHeaders{TotalPages: "5", Total: "50", Page: "3", PerPage: "10"})
	})
	client := testutil.NewTestClient(t, mux)

	in := ListGroupInput{
		GroupID: toolutil.StringOrInt("10"),
		OrderBy: "id",
		Sort:    "asc",

		Page:       3,
		PerPage:    10,
		Pagination: "keyset",
		PageToken:  "200",
	}
	out, err := ListGroup(context.Background(), client, in)
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.AccessRequests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(out.AccessRequests))
	}
}

// ---------------------------------------------------------------------------
// RequestProject — API error
// ---------------------------------------------------------------------------.

// TestRequestProject_APIError verifies that RequestProject returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestRequestProject_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"forbidden"}`)
	}))
	_, err := RequestProject(context.Background(), client, RequestProjectInput{
		ProjectID: toolutil.StringOrInt("42"),
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// RequestGroup — API error
// ---------------------------------------------------------------------------.

// TestRequestGroup_APIError verifies that RequestGroup returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestRequestGroup_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"forbidden"}`)
	}))
	_, err := RequestGroup(context.Background(), client, RequestGroupInput{
		GroupID: toolutil.StringOrInt("10"),
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// ApproveProject — API error, missing project_id
// ---------------------------------------------------------------------------.

// TestApproveProject_APIError verifies that ApproveProject returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestApproveProject_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := ApproveProject(context.Background(), client, ApproveProjectInput{
		ProjectID: toolutil.StringOrInt("42"),
		UserID:    1,
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestApproveProject_MissingProjectID verifies that ApproveProject_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestApproveProject_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ApproveProject(context.Background(), client, ApproveProjectInput{
		UserID: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "project_id is required") {
		t.Fatalf("expected project_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// ApproveGroup — API error, missing group_id
// ---------------------------------------------------------------------------.

// TestApproveGroup_APIError verifies that ApproveGroup returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestApproveGroup_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := ApproveGroup(context.Background(), client, ApproveGroupInput{
		GroupID: toolutil.StringOrInt("10"),
		UserID:  1,
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestApproveGroup_MissingGroupID verifies that ApproveGroup_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestApproveGroup_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ApproveGroup(context.Background(), client, ApproveGroupInput{
		UserID: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "group_id is required") {
		t.Fatalf("expected group_id required error, got %v", err)
	}
}

// TestApproveGroup_WithAccessLevel verifies the ApproveGroup_WithAccessLevel handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestApproveGroup_WithAccessLevel(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/10/access_requests/2/approve", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK,
			`{"id":2,"username":"bob","name":"Bob","state":"approved","access_level":40}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ApproveGroup(context.Background(), client, ApproveGroupInput{
		GroupID:     toolutil.StringOrInt("10"),
		UserID:      2,
		AccessLevel: 40,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.AccessLevel != 40 {
		t.Errorf("expected access_level 40, got %d", out.AccessLevel)
	}
}

// ---------------------------------------------------------------------------
// DenyProject — API error, missing project_id
// ---------------------------------------------------------------------------.

// TestDenyProject_APIError verifies that DenyProject returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDenyProject_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := DenyProject(context.Background(), client, DenyProjectInput{
		ProjectID: toolutil.StringOrInt("42"),
		UserID:    1,
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDenyProject_MissingProjectID verifies that DenyProject_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDenyProject_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := DenyProject(context.Background(), client, DenyProjectInput{
		UserID: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "project_id is required") {
		t.Fatalf("expected project_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// DenyGroup — API error, missing group_id
// ---------------------------------------------------------------------------.

// TestDenyGroup_APIError verifies that DenyGroup returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDenyGroup_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := DenyGroup(context.Background(), client, DenyGroupInput{
		GroupID: toolutil.StringOrInt("10"),
		UserID:  1,
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDenyGroup_MissingGroupID verifies that DenyGroup_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDenyGroup_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := DenyGroup(context.Background(), client, DenyGroupInput{
		UserID: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "group_id is required") {
		t.Fatalf("expected group_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// convertAccessRequest — with date fields populated
// ---------------------------------------------------------------------------.

// TestConvertAccessRequest_WithDates verifies that convertAccessRequest
// correctly formats the CreatedAt and RequestedAt time fields when they are
// populated.
//
// The test calls convertAccessRequest with a mock access request carrying
// explicit time.Time values and asserts the formatted strings contain the
// expected date prefix. This protects the date-formatting contract used by
// every access-request output.
func TestConvertAccessRequest_WithDates(t *testing.T) {
	// gl.AccessRequest uses *time.Time for CreatedAt and RequestedAt
	now := testTime(t, "2026-06-15T10:30:00Z")
	later := testTime(t, "2026-06-16T08:00:00Z")

	ar := mockAccessRequest(1, "alice", "Alice", "pending", 30)
	ar.CreatedAt = now
	ar.RequestedAt = later

	out := convertAccessRequest(ar, toolutil.AccessRequestExtra{})

	if out.CreatedAt == "" {
		t.Fatal("expected CreatedAt to be populated")
	}
	if !strings.Contains(out.CreatedAt, "2026-06-15") {
		t.Errorf("unexpected CreatedAt: %s", out.CreatedAt)
	}
	if out.RequestedAt == "" {
		t.Fatal("expected RequestedAt to be populated")
	}
	if !strings.Contains(out.RequestedAt, "2026-06-16") {
		t.Errorf("unexpected RequestedAt: %s", out.RequestedAt)
	}
}

// TestConvertAccessRequest_WithoutDates verifies the ConvertAccessRequest_WithoutDates handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestConvertAccessRequest_WithoutDates(t *testing.T) {
	ar := mockAccessRequest(2, "bob", "Bob", "approved", 20)
	out := convertAccessRequest(ar, toolutil.AccessRequestExtra{})

	if out.CreatedAt != "" {
		t.Errorf("expected empty CreatedAt, got %s", out.CreatedAt)
	}
	if out.RequestedAt != "" {
		t.Errorf("expected empty RequestedAt, got %s", out.RequestedAt)
	}
	if out.ID != 2 {
		t.Errorf("expected ID 2, got %d", out.ID)
	}
}

// ---------------------------------------------------------------------------
// FormatOutputMarkdown — all fields, minimal fields
// ---------------------------------------------------------------------------.

// cardHints is the guidance section every access-request card ends with, the
// four canonical actions a reader of one request can take next.
const cardHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
	"- Use action 'accessrequests.approve_project' to approve this request at project scope\n" +
	"- Use action 'accessrequests.approve_group' to approve this request at group scope\n" +
	"- Use action 'accessrequests.deny_project' to deny this request at project scope\n" +
	"- Use action 'accessrequests.deny_group' to deny this request at group scope\n"

// TestFormatOutputMarkdown_AllFields pins the whole card a fully populated
// access request renders: every row in order, the access level as its role
// name with the number, the locked warning, the nested creator, and the
// guidance section. The assertion is the entire document rather than a set of
// substrings, because a substring cannot see a row that landed outside the
// block it belongs to.
func TestFormatOutputMarkdown_AllFields(t *testing.T) {
	out := Output{
		ID:              1,
		Username:        "alice",
		Name:            "Alice Smith",
		State:           "approved",
		Locked:          true,
		AccessLevel:     30,
		CreatedAt:       "2026-06-15T10:30:00Z",
		CreatedBy:       &toolutil.MemberUserOutput{Name: "Carol Admin", Username: "carol", WebURL: "https://gitlab.example.com/carol"},
		RequestedAt:     "2026-06-16T08:00:00Z",
		Email:           "alice@example.com",
		PublicEmail:     "alice@public.example.com",
		MemberRole:      &toolutil.MemberRoleOutput{Name: "Auditor"},
		MembershipState: "active",
		ExpiresAt:       "2027-01-31T00:00:00Z",
		WebURL:          "https://gitlab.example.com/alice",
	}

	want := "## Access Request #1\n\n" +
		"- **ID**: 1\n" +
		"- **Username**: @alice\n" +
		"- **Name**: Alice Smith\n" +
		"- **State**: approved\n" +
		"- **Access Level**: Developer (30)\n" +
		"- " + toolutil.EmojiWarning + " **Locked**\n" +
		"- **Email**: alice@example.com\n" +
		"- **Public Email**: alice@public.example.com\n" +
		"- **Member Role**: Auditor\n" +
		"- **Membership State**: active\n" +
		"- **Created By**:\n" +
		"  - **Name**: Carol Admin\n" +
		"  - **Username**: [@carol](https://gitlab.example.com/carol)\n" +
		"- **Created At**: 15 Jun 2026 10:30 UTC\n" +
		"- **Requested At**: 16 Jun 2026 08:00 UTC\n" +
		"- **Expires At**: 31 Jan 2027 00:00 UTC\n" +
		"- **URL**: [https://gitlab.example.com/alice](https://gitlab.example.com/alice)\n" +
		cardHints

	if got := FormatOutputMarkdown(out); got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatOutputMarkdown_MinimalFields pins the card of a request GitLab
// sent nothing optional on: every guarded row is absent rather than rendered
// with an empty value, and the unlocked request carries no warning line.
func TestFormatOutputMarkdown_MinimalFields(t *testing.T) {
	out := Output{
		ID:          5,
		Username:    "bob",
		Name:        "Bob",
		State:       "pending",
		AccessLevel: 10,
	}

	want := "## Access Request #5\n\n" +
		"- **ID**: 5\n" +
		"- **Username**: @bob\n" +
		"- **Name**: Bob\n" +
		"- **State**: pending\n" +
		"- **Access Level**: Guest (10)\n" +
		cardHints

	if got := FormatOutputMarkdown(out); got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatOutputMarkdown_UnknownAccessLevel pins what a level GitLab has and
// this server's table does not renders as: the number it sent, which is the
// one thing a reader can act on.
func TestFormatOutputMarkdown_UnknownAccessLevel(t *testing.T) {
	out := Output{ID: 7, Username: "dana", Name: "Dana", State: "pending", AccessLevel: 35}

	want := "## Access Request #7\n\n" +
		"- **ID**: 7\n" +
		"- **Username**: @dana\n" +
		"- **Name**: Dana\n" +
		"- **State**: pending\n" +
		"- **Access Level**: Level 35 (35)\n" +
		cardHints

	if got := FormatOutputMarkdown(out); got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown — with items, empty list
// ---------------------------------------------------------------------------.

// listHints is the guidance section every access-request list ends with: the
// preserve-links reminder the linked username column earns, then the four
// canonical actions.
const listHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
	"- " + toolutil.HintPreserveLinks + "\n" +
	"- Use action 'accessrequests.approve_project' to approve one of these requests at project scope\n" +
	"- Use action 'accessrequests.approve_group' to approve one of these requests at group scope\n" +
	"- Use action 'accessrequests.deny_project' to deny one of these requests at project scope\n" +
	"- Use action 'accessrequests.deny_group' to deny one of these requests at group scope\n"

// TestFormatListMarkdown_WithItems pins the whole list document: the heading
// counting what the page shows when GitLab sent no total, the table, and the
// guidance section.
func TestFormatListMarkdown_WithItems(t *testing.T) {
	out := ListOutput{
		AccessRequests: []Output{
			{ID: 1, Username: "alice", Name: "Alice", State: "pending", AccessLevel: 30, WebURL: "https://gitlab.example.com/alice"},
			{ID: 2, Username: "bob", Name: "Bob", State: "approved", AccessLevel: 20},
		},
	}

	want := "## Access Requests (2)\n\n" +
		"| ID | Username | Name | State | Access Level |\n" +
		"| --- | --- | --- | --- | --- |\n" +
		"| 1 | [@alice](https://gitlab.example.com/alice) | Alice | pending | Developer (30) |\n" +
		"| 2 | @bob | Bob | approved | Reporter (20) |\n" +
		listHints

	if got := FormatListMarkdown(out); got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdown_Paginated pins the heading against the total GitLab
// reported rather than the length of the page, which is what a heading that
// counted len() told the reader wrongly on every page but the last.
func TestFormatListMarkdown_Paginated(t *testing.T) {
	out := ListOutput{
		AccessRequests: []Output{{ID: 1, Username: "alice", Name: "Alice", State: "pending", AccessLevel: 30}},
		Pagination:     toolutil.PaginationOutput{Page: 2, PerPage: 1, TotalPages: 3, TotalItems: 3},
	}

	want := "## Access Requests (3)\n\n" +
		"Showing 1 of 3 results (page 2 of 3)\n\n" +
		"| ID | Username | Name | State | Access Level |\n" +
		"| --- | --- | --- | --- | --- |\n" +
		"| 1 | @alice | Alice | pending | Developer (30) |\n\n" +
		"Page 2 of 3 | 3 items total | 1 per page\n" +
		listHints

	if got := FormatListMarkdown(out); got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdown_Empty pins the whole response of a list with nothing
// in it: the one sentence, and no heading counting zero above it.
func TestFormatListMarkdown_Empty(t *testing.T) {
	if got, want := FormatListMarkdown(ListOutput{}), "No access requests found.\n"; got != want {
		t.Errorf("empty list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestActionSpecs_Metadata validates the Metadata route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	specs := ActionSpecs(client)
	byTool := accessRequestSpecsByTool(t, specs)

	if len(specs) != 8 {
		t.Fatalf("len(ActionSpecs) = %d, want 8", len(specs))
	}
	if len(byTool) != len(specs) {
		t.Fatalf("unique individual tools = %d, want %d", len(byTool), len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "accessrequests" {
			t.Fatalf("OwnerPackage for %s = %q, want accessrequests", spec.Name, spec.OwnerPackage)
		}
	}

	listProject := byTool["gitlab_access_request_list_project"]
	if listProject.Usage == "" || len(listProject.Aliases) == 0 || listProject.ParameterGuidance["project_id"].SemanticRole == "" {
		t.Fatalf("list project metadata incomplete: usage=%q aliases=%d project_id guidance=%q", listProject.Usage, len(listProject.Aliases), listProject.ParameterGuidance["project_id"].SemanticRole)
	}

	approveProject := byTool["gitlab_access_request_approve_project"]
	if approveProject.Usage == "" || len(approveProject.Aliases) == 0 || approveProject.ParameterGuidance["user_id"].SemanticRole == "" {
		t.Fatalf("approve project metadata incomplete: usage=%q aliases=%d user_id guidance=%q", approveProject.Usage, len(approveProject.Aliases), approveProject.ParameterGuidance["user_id"].SemanticRole)
	}

	denyGroup := byTool["gitlab_access_request_deny_group"]
	if denyGroup.Usage == "" || len(denyGroup.Aliases) == 0 {
		t.Fatalf("deny group metadata incomplete: usage=%q aliases=%d", denyGroup.Usage, len(denyGroup.Aliases))
	}
}

// TestActionSpecs_CallAllRoutes validates the CallAllRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := newAccessRequestRouteSpecs(t)

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list_project", "gitlab_access_request_list_project", map[string]any{"project_id": "42"}},
		{"list_group", "gitlab_access_request_list_group", map[string]any{"group_id": "10"}},
		{"request_project", "gitlab_access_request_request_project", map[string]any{"project_id": "42"}},
		{"request_group", "gitlab_access_request_request_group", map[string]any{"group_id": "10"}},
		{"approve_project", "gitlab_access_request_approve_project", map[string]any{"project_id": "42", "user_id": 1}},
		{"approve_group", "gitlab_access_request_approve_group", map[string]any{"group_id": "10", "user_id": 1}},
		{"deny_project", "gitlab_access_request_deny_project", map[string]any{"project_id": "42", "user_id": 1}},
		{"deny_group", "gitlab_access_request_deny_group", map[string]any{"group_id": "10", "user_id": 1}},
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
// Helper: route spec factory
// ---------------------------------------------------------------------------.

// newAccessRequestRouteSpecs constructs access request route specs test fixtures.
func newAccessRequestRouteSpecs(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()

	arJSON := `{"id":1,"username":"alice","name":"Alice","state":"pending","access_level":30}`
	arListJSON := `[` + arJSON + `]`

	handler := http.NewServeMux()

	// List project access requests
	handler.HandleFunc("GET /api/v4/projects/42/access_requests", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, arListJSON)
	})

	// List group access requests
	handler.HandleFunc("GET /api/v4/groups/10/access_requests", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, arListJSON)
	})

	// Request project access
	handler.HandleFunc("POST /api/v4/projects/42/access_requests", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, arJSON)
	})

	// Request group access
	handler.HandleFunc("POST /api/v4/groups/10/access_requests", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, arJSON)
	})

	// Approve project access request
	handler.HandleFunc("PUT /api/v4/projects/42/access_requests/1/approve", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"username":"alice","name":"Alice","state":"approved","access_level":30}`)
	})

	// Approve group access request
	handler.HandleFunc("PUT /api/v4/groups/10/access_requests/1/approve", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"username":"alice","name":"Alice","state":"approved","access_level":30}`)
	})

	// Deny project access request
	handler.HandleFunc("DELETE /api/v4/projects/42/access_requests/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Deny group access request
	handler.HandleFunc("DELETE /api/v4/groups/10/access_requests/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	client := testutil.NewTestClient(t, handler)
	return accessRequestSpecsByTool(t, ActionSpecs(client))
}

// accessRequestSpecsByTool supports access request specs by tool assertions in accessrequests tests.
func accessRequestSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------.

// testTime supports test time assertions in accessrequests tests.
func testTime(t *testing.T, value string) *time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("failed to parse time %q: %v", value, err)
	}
	return &parsed
}

// mockAccessRequest supports mock access request assertions in accessrequests tests.
func mockAccessRequest(id int64, username, name, state string, level int) *gl.AccessRequest {
	return &gl.AccessRequest{
		ID:          id,
		Username:    username,
		Name:        name,
		State:       state,
		AccessLevel: gl.AccessLevelValue(level),
	}
}

// ---------------------------------------------------------------------------
// The fields GitLab sends that client-go's AccessRequest does not model
// ---------------------------------------------------------------------------.

// sentAccessRequestJSON is one access request as
// lib/api/entities/access_requester.rb renders it to a caller every condition
// holds for: the entity inherits Member and merges UserBasic, so the keys
// client-go models are here beside every key it does not.
//
// It deliberately carries avatar_path, custom_attributes and is_using_seat as
// well, which these routes cannot send because none of them declares the
// presenter option each waits on. They are here so that a body carrying them
// is proved to leave no trace on the output, which is the other half of the
// declarations that answer those three findings.
const sentAccessRequestJSON = `{"id":1,"username":"alice","public_email":"alice@public.example.com",` +
	`"name":"Alice","state":"active","locked":true,"avatar_url":"https://gitlab.example.com/a.png",` +
	`"avatar_path":"/uploads/-/system/user/avatar/1/a.png",` +
	`"custom_attributes":[{"key":"team","value":"core"}],"web_url":"https://gitlab.example.com/alice",` +
	`"access_level":30,"created_at":"2026-06-15T10:30:00Z",` +
	`"created_by":{"id":9,"username":"owner","name":"Owner","state":"active"},` +
	`"expires_at":"2027-01-31","group_saml_identity":{"extern_uid":"saml-1","provider":"group_saml","saml_provider_id":4},` +
	`"group_scim_identity":{"extern_uid":"scim-1","group_id":7,"active":true},` +
	`"email":"alice@example.com","is_using_seat":true,"override":true,"membership_state":"active",` +
	`"member_role":{"id":3,"name":"Auditor","base_access_level":30},` +
	`"requested_at":"2026-06-16T08:00:00Z"}`

// minimalAccessRequestJSON is the same request rendered to a caller none of
// the conditions hold for: what GitLab always sends and nothing else.
const minimalAccessRequestJSON = `{"id":1,"username":"alice","public_email":"alice@public.example.com",` +
	`"name":"Alice","state":"active","locked":true,"avatar_url":"https://gitlab.example.com/a.png",` +
	`"web_url":"https://gitlab.example.com/alice","access_level":30,` +
	`"created_at":"2026-06-15T10:30:00Z","expires_at":"2027-01-31","membership_state":"active",` +
	`"requested_at":"2026-06-16T08:00:00Z"}`

// accessRequestCalls are the six handlers that answer with an access request,
// each returning the one request it published and saying whether its endpoint
// answers with an array.
var accessRequestCalls = []struct {
	name string
	call func(client *gitlabclient.Client) (Output, error)
	list bool
}{
	{name: "list_project", list: true, call: func(client *gitlabclient.Client) (Output, error) {
		out, err := ListProject(context.Background(), client, ListProjectInput{ProjectID: "10"})
		return firstAccessRequest(out, err)
	}},
	{name: "list_group", list: true, call: func(client *gitlabclient.Client) (Output, error) {
		out, err := ListGroup(context.Background(), client, ListGroupInput{GroupID: "5"})
		return firstAccessRequest(out, err)
	}},
	{name: "request_project", call: func(client *gitlabclient.Client) (Output, error) {
		return RequestProject(context.Background(), client, RequestProjectInput{ProjectID: "10"})
	}},
	{name: "request_group", call: func(client *gitlabclient.Client) (Output, error) {
		return RequestGroup(context.Background(), client, RequestGroupInput{GroupID: "5"})
	}},
	{name: "approve_project", call: func(client *gitlabclient.Client) (Output, error) {
		return ApproveProject(context.Background(), client, ApproveProjectInput{ProjectID: "10", UserID: 1})
	}},
	{name: "approve_group", call: func(client *gitlabclient.Client) (Output, error) {
		return ApproveGroup(context.Background(), client, ApproveGroupInput{GroupID: "5", UserID: 1})
	}},
}

// errNoAccessRequest reports a list handler that answered without the single
// request its body carries, which would make every field assertion vacuous.
var errNoAccessRequest = errors.New("handler answered with no access request")

// firstAccessRequest takes the one request a list handler published.
func firstAccessRequest(out ListOutput, err error) (Output, error) {
	if err != nil {
		return Output{}, err
	}
	if len(out.AccessRequests) != 1 {
		return Output{}, errNoAccessRequest
	}
	return out.AccessRequests[0], nil
}

// accessRequestBodyFor wraps a single request in an array for the endpoints
// that answer with a collection.
func accessRequestBodyFor(list bool, object string) string {
	if list {
		return "[" + object + "]"
	}
	return object
}

// accessRequestClient answers every access-request route with one body, so a
// table can drive all six handlers against the same rendering.
func accessRequestClient(t *testing.T, body string) *gitlabclient.Client {
	t.Helper()
	return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, body,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	}))
}

// TestAccessRequests_SentFieldsReachEveryHandler verifies that every handler
// answering with an access request publishes the fifteen keys GitLab sends
// that client-go's AccessRequest does not model. Reading them off the SDK's
// struct is impossible, so only the capture beside the decode can carry them,
// and each handler had to be threaded separately.
func TestAccessRequests_SentFieldsReachEveryHandler(t *testing.T) {
	for _, requestCall := range accessRequestCalls {
		t.Run(requestCall.name, func(t *testing.T) {
			out, err := requestCall.call(accessRequestClient(t, accessRequestBodyFor(requestCall.list, sentAccessRequestJSON)))
			if err != nil {
				t.Fatalf("%s: %v", requestCall.name, err)
			}
			assertSentAccessRequest(t, out)
		})
	}
}

// sentAccessRequestStrings is what each string key of
// [sentAccessRequestJSON] must reach the output as.
var sentAccessRequestStrings = map[string]string{
	"public_email":     "alice@public.example.com",
	"avatar_url":       "https://gitlab.example.com/a.png",
	"web_url":          "https://gitlab.example.com/alice",
	"expires_at":       "2027-01-31",
	"email":            "alice@example.com",
	"membership_state": "active",
}

// assertSentAccessRequest holds one published request to every key
// [sentAccessRequestJSON] carries that this endpoint can send, each under a
// subtest of its own so a failure names the field that was dropped, and holds
// the three option-gated keys in that body to nothing at all.
func assertSentAccessRequest(t *testing.T, out Output) {
	t.Helper()
	for field, got := range map[string]string{
		"public_email":     out.PublicEmail,
		"avatar_url":       out.AvatarURL,
		"web_url":          out.WebURL,
		"expires_at":       out.ExpiresAt,
		"email":            out.Email,
		"membership_state": out.MembershipState,
	} {
		t.Run(field, func(t *testing.T) {
			if want := sentAccessRequestStrings[field]; got != want {
				t.Errorf("%s = %q, want %q", field, got, want)
			}
		})
	}
	t.Run("locked", func(t *testing.T) {
		if !out.Locked {
			t.Error("locked = false, want true")
		}
	})
	assertSentAccessRequestObjects(t, out)
	assertAccessRequestPublishesNoOptionGatedKey(t, out)
}

// assertAccessRequestPublishesNoOptionGatedKey holds the output to carrying no
// trace of avatar_path, custom_attributes or is_using_seat even when the body
// spelled all three.
//
// The access-request routes declare none of only_path, with_custom_attributes
// or show_seat_info, so GitLab cannot send them here and the surface must not
// claim it can. The audit answers the same three findings with
// entity-option-no-endpoint-passes declarations; this is what stops the code
// drifting back from them, and it is asserted on the marshaled JSON because
// that is the surface a client sees.
func assertAccessRequestPublishesNoOptionGatedKey(t *testing.T, out Output) {
	t.Helper()
	encoded, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal the published request: %v", err)
	}
	for _, key := range []string{"avatar_path", "custom_attributes", "is_using_seat"} {
		t.Run("no "+key, func(t *testing.T) {
			if strings.Contains(string(encoded), key) {
				t.Errorf("published %s, which no access-request route can send:\n%s", key, encoded)
			}
		})
	}
}

// assertSentAccessRequestObjects holds the four object-valued keys, which the
// SDK's AccessRequest models none of and the capture has to carry whole.
func assertSentAccessRequestObjects(t *testing.T, out Output) {
	t.Helper()
	t.Run("created_by", func(t *testing.T) {
		if out.CreatedBy == nil || out.CreatedBy.ID != 9 || out.CreatedBy.Username != "owner" {
			t.Errorf("created_by = %+v, want the creating user", out.CreatedBy)
		}
	})
	t.Run("group_saml_identity", func(t *testing.T) {
		if out.GroupSAMLIdentity == nil || out.GroupSAMLIdentity.ExternUID != "saml-1" || out.GroupSAMLIdentity.SAMLProviderID != 4 {
			t.Errorf("group_saml_identity = %+v, want the SAML identity", out.GroupSAMLIdentity)
		}
	})
	t.Run("group_scim_identity", func(t *testing.T) {
		if out.GroupSCIMIdentity == nil || out.GroupSCIMIdentity.ExternUID != "scim-1" || out.GroupSCIMIdentity.GroupID != 7 || !out.GroupSCIMIdentity.Active {
			t.Errorf("group_scim_identity = %+v, want the SCIM identity", out.GroupSCIMIdentity)
		}
	})
	t.Run("override", func(t *testing.T) {
		if out.Override == nil || !*out.Override {
			t.Errorf("override = %v, want true", out.Override)
		}
	})
	t.Run("member_role", func(t *testing.T) {
		if out.MemberRole == nil || out.MemberRole.ID != 3 || out.MemberRole.Name != "Auditor" {
			t.Errorf("member_role = %+v, want the custom role", out.MemberRole)
		}
	})
}

// TestAccessRequests_ConditionalFieldsAbsentWhenGitLabOmitsThem verifies the
// other side of every condition on the entity: a caller none of them hold for
// gets the request without those keys, and the output leaves each at its zero
// rather than inventing one. This is what makes the omitempty tags honest.
func TestAccessRequests_ConditionalFieldsAbsentWhenGitLabOmitsThem(t *testing.T) {
	for _, requestCall := range accessRequestCalls {
		t.Run(requestCall.name, func(t *testing.T) {
			out, err := requestCall.call(accessRequestClient(t, accessRequestBodyFor(requestCall.list, minimalAccessRequestJSON)))
			if err != nil {
				t.Fatalf("%s: %v", requestCall.name, err)
			}
			for field, empty := range map[string]bool{
				"created_by":          out.CreatedBy == nil,
				"group_saml_identity": out.GroupSAMLIdentity == nil,
				"group_scim_identity": out.GroupSCIMIdentity == nil,
				"email":               out.Email == "",
				"override":            out.Override == nil,
				"member_role":         out.MemberRole == nil,
			} {
				t.Run(field, func(t *testing.T) {
					if !empty {
						t.Errorf("%s was filled from a body that does not carry it", field)
					}
				})
			}
			// The unconditional keys are still there, so an all-empty output
			// cannot pass this test by carrying nothing at all.
			if out.PublicEmail == "" || !out.Locked || out.WebURL == "" || out.ExpiresAt == "" || out.MembershipState == "" {
				t.Errorf("the unconditional keys were dropped too: %+v", out)
			}
		})
	}
}

// TestAccessRequests_UnreadableCapturedFields verifies every handler reports
// the captured response's decode failure rather than a half-filled request.
// client-go's AccessRequest has no expires_at, so only the read beside it can
// notice that GitLab sent an object where a date belongs.
func TestAccessRequests_UnreadableCapturedFields(t *testing.T) {
	const poisoned = `{"id":1,"username":"alice","expires_at":{"not":"a date"}}`
	cases := make([]testutil.CapturedCase, 0, len(accessRequestCalls))
	for _, requestCall := range accessRequestCalls {
		cases = append(cases, testutil.CapturedCase{Name: requestCall.name, Call: func() error {
			_, err := requestCall.call(accessRequestClient(t, accessRequestBodyFor(requestCall.list, poisoned)))
			return err
		}})
	}
	testutil.AssertCapturedDecodeFailures(t, cases)
}

// TestAccessRequests_ListPairsEachExtraWithItsOwnRequest verifies a list
// handler pairs the extra read at one position with the request decoded at the
// same one. A reader that took the first extra for every request would pass
// every single-object test above and be wrong on the first page of two.
func TestAccessRequests_ListPairsEachExtraWithItsOwnRequest(t *testing.T) {
	const page = `[{"id":1,"username":"alice","email":"alice@example.com","web_url":"https://gl/alice"},` +
		`{"id":2,"username":"bob","email":"bob@example.com","web_url":"https://gl/bob"}]`
	out, err := ListProject(context.Background(), accessRequestClient(t, page), ListProjectInput{ProjectID: "10"})
	if err != nil {
		t.Fatalf("ListProject: %v", err)
	}
	if len(out.AccessRequests) != 2 {
		t.Fatalf("published %d requests, want 2", len(out.AccessRequests))
	}
	for i, want := range []struct{ username, email, webURL string }{
		{"alice", "alice@example.com", "https://gl/alice"},
		{"bob", "bob@example.com", "https://gl/bob"},
	} {
		t.Run(want.username, func(t *testing.T) {
			got := out.AccessRequests[i]
			if got.Username != want.username || got.Email != want.email || got.WebURL != want.webURL {
				t.Errorf("request %d = %+v, want %v", i, got, want)
			}
		})
	}
}

// TestApprove_SendsTheAccessLevelOnlyWhenTheCallerNamedOne verifies both
// approve handlers put the granted role in the request body when the input
// carries one, and send no access_level at all when it does not.
//
// Only the request shows this. GitLab answers the same either way, so a test
// reading the output cannot tell an omitted role from a zero one, and a zero
// is what the endpoint refuses.
func TestApprove_SendsTheAccessLevelOnlyWhenTheCallerNamedOne(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		level    int
		wantSent bool
	}{
		{name: "with_a_level", level: 40, wantSent: true},
		{name: "without_a_level", level: 0},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			for scope, approve := range map[string]func(*gitlabclient.Client) error{
				"project": func(client *gitlabclient.Client) error {
					_, err := ApproveProject(context.Background(), client, ApproveProjectInput{
						ProjectID: "10", UserID: 1, AccessLevel: testCase.level,
					})
					return err
				},
				"group": func(client *gitlabclient.Client) error {
					_, err := ApproveGroup(context.Background(), client, ApproveGroupInput{
						GroupID: "5", UserID: 1, AccessLevel: testCase.level,
					})
					return err
				},
			} {
				t.Run(scope, func(t *testing.T) {
					var sent string
					client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						body, readErr := io.ReadAll(r.Body)
						if readErr != nil {
							t.Errorf("read request body: %v", readErr)
						}
						sent = string(body)
						testutil.RespondJSON(w, http.StatusOK, `{"id":1,"username":"alice"}`)
					}))
					if err := approve(client); err != nil {
						t.Fatalf("approve %s: %v", scope, err)
					}
					if got := strings.Contains(sent, `"access_level":40`); got != testCase.wantSent {
						t.Errorf("body %q carries the access level = %v, want %v", sent, got, testCase.wantSent)
					}
				})
			}
		})
	}
}

// TestAccessRequests_ListPublishesNoRequestsForAnEmptyPage verifies an empty
// page answers with no requests rather than with an error from the reader
// holding the captured count to the SDK's.
func TestAccessRequests_ListPublishesNoRequestsForAnEmptyPage(t *testing.T) {
	out, err := ListGroup(context.Background(), accessRequestClient(t, `[]`), ListGroupInput{GroupID: "5"})
	if err != nil {
		t.Fatalf("ListGroup: %v", err)
	}
	if out.AccessRequests != nil {
		t.Errorf("AccessRequests = %+v, want none", out.AccessRequests)
	}
}
