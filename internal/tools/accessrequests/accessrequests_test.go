// accessrequests_test.go contains unit tests for the access request MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package accessrequests

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// ListProject
// ---------------------------------------------------------------------------.

// TestListProject_Success verifies the behavior of list project success.
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

// TestListProject_MissingProjectID verifies the behavior of list project missing project i d.
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

// TestListGroup_Success verifies the behavior of list group success.
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

// TestListGroup_MissingGroupID verifies the behavior of list group missing group i d.
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

// TestRequestProject_Success verifies the behavior of request project success.
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

// TestRequestProject_MissingProjectID verifies the behavior of request project missing project i d.
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

// TestRequestGroup_Success verifies the behavior of request group success.
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

// TestRequestGroup_MissingGroupID verifies the behavior of request group missing group i d.
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

// TestApproveProject_Success verifies the behavior of approve project success.
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

// TestApproveProject_MissingUserID verifies the behavior of approve project missing user i d.
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

// TestApproveGroup_Success verifies the behavior of approve group success.
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

// TestApproveGroup_MissingUserID verifies the behavior of approve group missing user i d.
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

// TestDenyProject_Success verifies the behavior of deny project success.
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

// TestDenyProject_MissingUserID verifies the behavior of deny project missing user i d.
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

// TestDenyGroup_Success verifies the behavior of deny group success.
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

// TestDenyGroup_MissingUserID verifies the behavior of deny group missing user i d.
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

const errExpectedAPI = "expected API error, got nil"

// ---------------------------------------------------------------------------
// ListProject — API error, pagination params
// ---------------------------------------------------------------------------.

// TestListProject_APIError verifies the behavior of list project a p i error.
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

// TestListProject_PaginationParams verifies the behavior of list project pagination params.
func TestListProject_PaginationParams(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/42/access_requests", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "2" {
			t.Errorf("expected page=2, got %s", r.URL.Query().Get("page"))
		}
		if r.URL.Query().Get("per_page") != "5" {
			t.Errorf("expected per_page=5, got %s", r.URL.Query().Get("per_page"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":10,"username":"u","name":"n","state":"pending","access_level":30}]`,
			testutil.PaginationHeaders{TotalPages: "3", Total: "15", Page: "2", PerPage: "5"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListProject(context.Background(), client, ListProjectInput{
		ProjectID: toolutil.StringOrInt("42"),
		Page:      2,
		PerPage:   5,
	})
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

// TestListGroup_APIError verifies the behavior of list group a p i error.
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

// TestListGroup_PaginationParams verifies the behavior of list group pagination params.
func TestListGroup_PaginationParams(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/10/access_requests", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "3" {
			t.Errorf("expected page=3, got %s", r.URL.Query().Get("page"))
		}
		if r.URL.Query().Get("per_page") != "10" {
			t.Errorf("expected per_page=10, got %s", r.URL.Query().Get("per_page"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":20,"username":"x","name":"X","state":"pending","access_level":20}]`,
			testutil.PaginationHeaders{TotalPages: "5", Total: "50", Page: "3", PerPage: "10"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListGroup(context.Background(), client, ListGroupInput{
		GroupID: toolutil.StringOrInt("10"),
		Page:    3,
		PerPage: 10,
	})
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

// TestRequestProject_APIError verifies the behavior of request project a p i error.
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

// TestRequestGroup_APIError verifies the behavior of request group a p i error.
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

// TestApproveProject_APIError verifies the behavior of approve project a p i error.
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

// TestApproveProject_MissingProjectID verifies the behavior of approve project missing project i d.
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

// TestApproveGroup_APIError verifies the behavior of approve group a p i error.
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

// TestApproveGroup_MissingGroupID verifies the behavior of approve group missing group i d.
func TestApproveGroup_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ApproveGroup(context.Background(), client, ApproveGroupInput{
		UserID: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "group_id is required") {
		t.Fatalf("expected group_id required error, got %v", err)
	}
}

// TestApproveGroup_WithAccessLevel verifies the behavior of approve group with access level.
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

// TestDenyProject_APIError verifies the behavior of deny project a p i error.
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

// TestDenyProject_MissingProjectID verifies the behavior of deny project missing project i d.
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

// TestDenyGroup_APIError verifies the behavior of deny group a p i error.
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

// TestDenyGroup_MissingGroupID verifies the behavior of deny group missing group i d.
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

// TestConvertAccessRequest_WithDates verifies the behavior of convert access request with dates.
func TestConvertAccessRequest_WithDates(t *testing.T) {
	// gl.AccessRequest uses *time.Time for CreatedAt and RequestedAt
	now := testTime(t, "2026-06-15T10:30:00Z")
	later := testTime(t, "2026-06-16T08:00:00Z")

	ar := mockAccessRequest(1, "alice", "Alice", "pending", 30)
	ar.CreatedAt = now
	ar.RequestedAt = later

	out := convertAccessRequest(ar)

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

// TestConvertAccessRequest_WithoutDates verifies the behavior of convert access request without dates.
func TestConvertAccessRequest_WithoutDates(t *testing.T) {
	ar := mockAccessRequest(2, "bob", "Bob", "approved", 20)
	out := convertAccessRequest(ar)

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

// TestFormatOutputMarkdown_AllFields verifies the behavior of format output markdown all fields.
func TestFormatOutputMarkdown_AllFields(t *testing.T) {
	out := Output{
		ID:          1,
		Username:    "alice",
		Name:        "Alice Smith",
		State:       "approved",
		AccessLevel: 30,
		CreatedAt:   "2026-06-15T10:30:00Z",
		RequestedAt: "2026-06-16T08:00:00Z",
	}
	md := FormatOutputMarkdown(out)

	checks := []string{
		"## Access Request #1",
		"| ID | 1 |",
		"| Username | alice |",
		"| Name | Alice Smith |",
		"| State | approved |",
		"| Access Level | 30 |",
		"| Created At | 15 Jun 2026 10:30 UTC |",
		"| Requested At | 16 Jun 2026 08:00 UTC |",
	}
	for _, c := range checks {
		if !strings.Contains(md, c) {
			t.Errorf("expected markdown to contain %q:\n%s", c, md)
		}
	}
}

// TestFormatOutputMarkdown_MinimalFields verifies the behavior of format output markdown minimal fields.
func TestFormatOutputMarkdown_MinimalFields(t *testing.T) {
	out := Output{
		ID:          5,
		Username:    "bob",
		Name:        "Bob",
		State:       "pending",
		AccessLevel: 10,
	}
	md := FormatOutputMarkdown(out)

	if !strings.Contains(md, "## Access Request #5") {
		t.Errorf("expected heading:\n%s", md)
	}
	if strings.Contains(md, "Created At") {
		t.Error("should not contain Created At when empty")
	}
	if strings.Contains(md, "Requested At") {
		t.Error("should not contain Requested At when empty")
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown — with items, empty list
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithItems verifies the behavior of format list markdown with items.
func TestFormatListMarkdown_WithItems(t *testing.T) {
	out := ListOutput{
		AccessRequests: []Output{
			{ID: 1, Username: "alice", Name: "Alice", State: "pending", AccessLevel: 30},
			{ID: 2, Username: "bob", Name: "Bob", State: "approved", AccessLevel: 20},
		},
	}
	md := FormatListMarkdown(out)

	checks := []string{
		"## Access Requests (2)",
		"| ID | Username | Name | State | Access Level |",
		"| 1 | alice | Alice | pending | 30 |",
		"| 2 | bob | Bob | approved | 20 |",
	}
	for _, c := range checks {
		if !strings.Contains(md, c) {
			t.Errorf("expected markdown to contain %q:\n%s", c, md)
		}
	}
}

// TestFormatListMarkdown_Empty verifies the behavior of format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	out := ListOutput{}
	md := FormatListMarkdown(out)

	if !strings.Contains(md, "## Access Requests (0)") {
		t.Errorf("expected heading with 0 count:\n%s", md)
	}
	if !strings.Contains(md, "No access requests found") {
		t.Errorf("expected empty message:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// ---------------------------------------------------------------------------
// RegisterTools — no panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// RegisterMeta — no panic
// ---------------------------------------------------------------------------.

// TestRegisterMeta_NoPanic verifies the behavior of register meta no panic.
func TestRegisterMeta_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)
}

// ---------------------------------------------------------------------------
// RegisterToolsCallAllThroughMCP — full MCP roundtrip for all 8 tools
// ---------------------------------------------------------------------------.

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newAccessRequestsMCPSession(t)
	ctx := context.Background()

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
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.tool,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.tool, err)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(%s) returned error: %s", tt.tool, tc.Text)
					}
				}
				t.Fatalf("CallTool(%s) returned IsError=true", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helper: MCP session factory
// ---------------------------------------------------------------------------.

// newAccessRequestsMCPSession is an internal helper for the accessrequests package.
func newAccessRequestsMCPSession(t *testing.T) *mcp.ClientSession {
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

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------.

// testTime is an internal helper for the accessrequests package.
func testTime(t *testing.T, value string) *time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("failed to parse time %q: %v", value, err)
	}
	return &parsed
}

// mockAccessRequest is an internal helper for the accessrequests package.
func mockAccessRequest(id int64, username, name, state string, level int) *gl.AccessRequest {
	return &gl.AccessRequest{
		ID:          id,
		Username:    username,
		Name:        name,
		State:       state,
		AccessLevel: gl.AccessLevelValue(level),
	}
}
