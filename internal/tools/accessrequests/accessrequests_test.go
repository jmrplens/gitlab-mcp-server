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

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// ListProject
// ---------------------------------------------------------------------------.

// TestListProject_Success verifies ListProject when success.
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

// TestListGroup_MissingGroupID verifies ListGroup when missing group ID.
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

// TestRequestProject_Success verifies RequestProject when success.
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

// TestRequestProject_MissingProjectID verifies RequestProject when missing project ID.
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

// TestRequestGroup_Success verifies RequestGroup when success.
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

// TestRequestGroup_MissingGroupID verifies RequestGroup when missing group ID.
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

// TestApproveProject_Success verifies ApproveProject when success.
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

// TestApproveProject_MissingUserID verifies ApproveProject when missing user ID.
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

// TestApproveGroup_Success verifies ApproveGroup when success.
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

// TestApproveGroup_MissingUserID verifies ApproveGroup when missing user ID.
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

// TestDenyProject_Success verifies DenyProject when success.
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

// TestDenyProject_MissingUserID verifies DenyProject when missing user ID.
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

// TestDenyGroup_Success verifies DenyGroup when success.
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

// TestDenyGroup_MissingUserID verifies DenyGroup when missing user ID.
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

// TestListProject_APIError verifies ListProject when API error.
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

// TestListProject_PaginationParams verifies ListProject when pagination params.
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

// TestListGroup_APIError verifies ListGroup when API error.
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

// TestListGroup_PaginationParams verifies ListGroup when pagination params.
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

// TestRequestProject_APIError verifies RequestProject when API error.
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

// TestRequestGroup_APIError verifies RequestGroup when API error.
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

// TestApproveProject_APIError verifies ApproveProject when API error.
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

// TestApproveProject_MissingProjectID verifies ApproveProject when missing project ID.
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

// TestApproveGroup_APIError verifies ApproveGroup when API error.
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

// TestApproveGroup_MissingGroupID verifies ApproveGroup when missing group ID.
func TestApproveGroup_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ApproveGroup(context.Background(), client, ApproveGroupInput{
		UserID: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "group_id is required") {
		t.Fatalf("expected group_id required error, got %v", err)
	}
}

// TestApproveGroup_WithAccessLevel verifies ApproveGroup when with access level.
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

// TestDenyProject_APIError verifies DenyProject when API error.
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

// TestDenyProject_MissingProjectID verifies DenyProject when missing project ID.
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

// TestDenyGroup_APIError verifies DenyGroup when API error.
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

// TestDenyGroup_MissingGroupID verifies DenyGroup when missing group ID.
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

// TestConvertAccessRequest_WithDates verifies ConvertAccessRequest when with dates.
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

// TestConvertAccessRequest_WithoutDates verifies ConvertAccessRequest when without dates.
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

// TestFormatOutputMarkdown_AllFields verifies FormatOutputMarkdown when all fields.
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

// TestFormatOutputMarkdown_MinimalFields verifies FormatOutputMarkdown when minimal fields.
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

// TestFormatListMarkdown_WithItems verifies FormatListMarkdown when with items.
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

// TestFormatListMarkdown_Empty verifies FormatListMarkdown when empty.
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

// TestActionSpecs_Metadata verifies canonical metadata for access request actions.
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

// TestActionSpecs_CallAllRoutes validates all access request routes through the canonical specs.
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
