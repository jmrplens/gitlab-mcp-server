// external_status_checks_test.go contains unit tests for the external status
// check MCP tool handlers. Tests use httptest to mock GitLab API responses and
// verify success, validation, and error paths.
package externalstatuschecks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

const fmtUnexpErr = "unexpected error: %v"

const mergeStatusCheckJSON = `{
	"id": 1,
	"name": "CI Check",
	"external_url": "https://ci.example.com",
	"status": "passed"
}`

const mergeStatusCheckListJSON = `[` + mergeStatusCheckJSON + `]`

const projectStatusCheckJSON = `{
	"id": 42,
	"name": "Security Scan",
	"project_id": 1,
	"external_url": "https://scan.example.com",
	"hmac": true,
	"protected_branches": [
		{
			"id": 100,
			"project_id": 1,
			"name": "main",
			"code_owner_approval_required": false
		}
	]
}`

const projectStatusCheckListJSON = `[` + projectStatusCheckJSON + `]`

// TestListProjectStatusChecks_Success verifies that ListProjectStatusChecks succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListProjectStatusChecks_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/1/external_status_checks", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, projectStatusCheckListJSON, testutil.PaginationHeaders{
			Page: "1", NextPage: "", TotalPages: "1", PerPage: "20", Total: "1",
		})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListProjectStatusChecks(context.Background(), client, ListProjectStatusChecksInput{
		ProjectID: "1",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(out.Items))
	}
	if out.Items[0].Name != "Security Scan" {
		t.Errorf("expected name 'Security Scan', got %q", out.Items[0].Name)
	}
	if !out.Items[0].HMAC {
		t.Error("expected HMAC=true")
	}
	if len(out.Items[0].ProtectedBranches) != 1 {
		t.Fatalf("expected 1 protected branch, got %d", len(out.Items[0].ProtectedBranches))
	}
	if out.Items[0].ProtectedBranches[0].Name != "main" {
		t.Errorf("expected branch 'main', got %q", out.Items[0].ProtectedBranches[0].Name)
	}
}

// TestListProjectStatusChecks_MissingProjectID verifies that ListProjectStatusChecks_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListProjectStatusChecks_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ListProjectStatusChecks(context.Background(), client, ListProjectStatusChecksInput{})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestListProjectMRExternalStatusChecks_Success verifies that ListProjectMRExternalStatusChecks succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListProjectMRExternalStatusChecks_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/1/merge_requests/10/status_checks", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, mergeStatusCheckListJSON, testutil.PaginationHeaders{
			Page: "1", NextPage: "", TotalPages: "1", PerPage: "20", Total: "1",
		})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListProjectMRExternalStatusChecks(context.Background(), client, ListProjectMRInput{
		ProjectID: "1",
		MRIID:     10,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(out.Items))
	}
	if out.Items[0].Name != "CI Check" {
		t.Errorf("expected name 'CI Check', got %q", out.Items[0].Name)
	}
}

// TestListProjectMRExternalStatusChecks_MissingFields verifies that ListProjectMRExternalStatusChecks_MissingFields returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListProjectMRExternalStatusChecks_MissingFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())

	_, err := ListProjectMRExternalStatusChecks(context.Background(), client, ListProjectMRInput{MRIID: 10})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
	_, err = ListProjectMRExternalStatusChecks(context.Background(), client, ListProjectMRInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error for missing merge_request_iid")
	}
}

// TestListProjectExternalStatusChecks_Success verifies that ListProjectExternalStatusChecks succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListProjectExternalStatusChecks_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/1/external_status_checks", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, projectStatusCheckListJSON, testutil.PaginationHeaders{
			Page: "1", NextPage: "", TotalPages: "1", PerPage: "20", Total: "1",
		})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListProjectExternalStatusChecks(context.Background(), client, ListProjectInput{
		ProjectID: "1",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(out.Items))
	}
	if out.Items[0].Name != "Security Scan" {
		t.Errorf("expected name 'Security Scan', got %q", out.Items[0].Name)
	}
	if !out.Items[0].HMAC {
		t.Error("expected HMAC=true")
	}
}

// TestListProjectExternalStatusChecks_MissingProjectID verifies that ListProjectExternalStatusChecks_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListProjectExternalStatusChecks_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ListProjectExternalStatusChecks(context.Background(), client, ListProjectInput{})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestCreateProjectExternalStatusCheck_Success verifies that CreateProjectExternalStatusCheck succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreateProjectExternalStatusCheck_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/projects/1/external_status_checks", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, projectStatusCheckJSON)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := CreateProjectExternalStatusCheck(context.Background(), client, CreateProjectInput{
		ProjectID:   "1",
		Name:        "Security Scan",
		ExternalURL: "https://scan.example.com",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 42 {
		t.Errorf("expected ID 42, got %d", out.ID)
	}
	if out.Name != "Security Scan" {
		t.Errorf("expected name 'Security Scan', got %q", out.Name)
	}
	if !out.HMAC {
		t.Error("expected HMAC=true")
	}
}

// TestCreateProjectExternalStatusCheck_WithOptionalFields verifies the CreateProjectExternalStatusCheck_WithOptionalFields handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreateProjectExternalStatusCheck_WithOptionalFields(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/projects/1/external_status_checks", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, projectStatusCheckJSON)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := CreateProjectExternalStatusCheck(context.Background(), client, CreateProjectInput{
		ProjectID:          "1",
		Name:               "Security Scan",
		ExternalURL:        "https://scan.example.com",
		SharedSecret:       "secret123",
		ProtectedBranchIDs: []int64{100},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 42 {
		t.Errorf("expected ID 42, got %d", out.ID)
	}
}

// TestCreateProjectExternalStatusCheck_MissingFields verifies that CreateProjectExternalStatusCheck_MissingFields returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateProjectExternalStatusCheck_MissingFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())

	tests := []struct {
		name  string
		input CreateProjectInput
	}{
		{"missing project_id", CreateProjectInput{Name: "x", ExternalURL: "https://x.com"}},
		{"missing name", CreateProjectInput{ProjectID: "1", ExternalURL: "https://x.com"}},
		{"missing external_url", CreateProjectInput{ProjectID: "1", Name: "x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CreateProjectExternalStatusCheck(context.Background(), client, tt.input)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

// TestDeleteProjectExternalStatusCheck_Success verifies that DeleteProjectExternalStatusCheck succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDeleteProjectExternalStatusCheck_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v4/projects/1/external_status_checks/42", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)

	err := DeleteProjectExternalStatusCheck(context.Background(), client, DeleteProjectInput{
		ProjectID: "1",
		CheckID:   42,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeleteProjectExternalStatusCheck_MissingFields verifies that DeleteProjectExternalStatusCheck_MissingFields returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteProjectExternalStatusCheck_MissingFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())

	tests := []struct {
		name  string
		input DeleteProjectInput
	}{
		{"missing project_id", DeleteProjectInput{CheckID: 42}},
		{"missing check_id", DeleteProjectInput{ProjectID: "1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := DeleteProjectExternalStatusCheck(context.Background(), client, tt.input)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

// TestUpdateProjectExternalStatusCheck_Success verifies that UpdateProjectExternalStatusCheck succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestUpdateProjectExternalStatusCheck_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/v4/projects/1/external_status_checks/42", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, projectStatusCheckJSON)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := UpdateProjectExternalStatusCheck(context.Background(), client, UpdateProjectInput{
		ProjectID: "1",
		CheckID:   42,
		Name:      "Updated",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 42 {
		t.Errorf("expected ID 42, got %d", out.ID)
	}
}

// TestUpdateProjectExternalStatusCheck_WithAllFields verifies the UpdateProjectExternalStatusCheck_WithAllFields handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestUpdateProjectExternalStatusCheck_WithAllFields(t *testing.T) {
	var capturedBody string
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/v4/projects/1/external_status_checks/42", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			http.Error(w, "read request body", http.StatusInternalServerError)
			return
		}
		capturedBody = string(body)
		testutil.RespondJSON(w, http.StatusOK, projectStatusCheckJSON)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := UpdateProjectExternalStatusCheck(context.Background(), client, UpdateProjectInput{
		ProjectID:          "1",
		CheckID:            42,
		Name:               "Updated",
		ExternalURL:        "https://new.example.com",
		SharedSecret:       "newsecret",
		ProtectedBranchIDs: []int64{100, 200},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Name != "Security Scan" {
		t.Errorf("expected name 'Security Scan', got %q", out.Name)
	}
	for _, want := range []string{"name", "external_url", "shared_secret", "protected_branch_ids"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(capturedBody, want) {
				t.Errorf("request body missing field %q", want)
			}
		})
	}
}

// TestUpdateProjectExternalStatusCheck_MissingFields verifies that UpdateProjectExternalStatusCheck_MissingFields returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestUpdateProjectExternalStatusCheck_MissingFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())

	tests := []struct {
		name  string
		input UpdateProjectInput
	}{
		{"missing project_id", UpdateProjectInput{CheckID: 42}},
		{"missing check_id", UpdateProjectInput{ProjectID: "1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := UpdateProjectExternalStatusCheck(context.Background(), client, tt.input)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

// TestRetryFailedExternalStatusCheckForProjectMR_Success verifies that RetryFailedExternalStatusCheckForProjectMR succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestRetryFailedExternalStatusCheckForProjectMR_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/projects/1/merge_requests/10/status_checks/42/retry", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	client := testutil.NewTestClient(t, mux)

	err := RetryFailedExternalStatusCheckForProjectMR(context.Background(), client, RetryProjectInput{
		ProjectID: "1",
		MRIID:     10,
		CheckID:   42,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestRetryFailedExternalStatusCheckForProjectMR_MissingFields verifies that RetryFailedExternalStatusCheckForProjectMR_MissingFields returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestRetryFailedExternalStatusCheckForProjectMR_MissingFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())

	tests := []struct {
		name  string
		input RetryProjectInput
	}{
		{"missing project_id", RetryProjectInput{MRIID: 10, CheckID: 42}},
		{"missing merge_request_iid", RetryProjectInput{ProjectID: "1", CheckID: 42}},
		{"missing check_id", RetryProjectInput{ProjectID: "1", MRIID: 10}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RetryFailedExternalStatusCheckForProjectMR(context.Background(), client, tt.input)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

// TestSetProjectMRExternalStatusCheckStatus_Success verifies that SetProjectMRExternalStatusCheckStatus succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestSetProjectMRExternalStatusCheckStatus_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/projects/1/merge_requests/10/status_check_responses", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	client := testutil.NewTestClient(t, mux)

	err := SetProjectMRExternalStatusCheckStatus(context.Background(), client, SetProjectStatusInput{
		ProjectID:             "1",
		MRIID:                 10,
		SHA:                   "abc123",
		ExternalStatusCheckID: 42,
		Status:                "passed",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestSetProjectMRExternalStatusCheckStatus_MissingFields verifies that SetProjectMRExternalStatusCheckStatus_MissingFields returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestSetProjectMRExternalStatusCheckStatus_MissingFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())

	tests := []struct {
		name  string
		input SetProjectStatusInput
	}{
		{"missing project_id", SetProjectStatusInput{MRIID: 10, SHA: "abc", ExternalStatusCheckID: 1, Status: "passed"}},
		{"missing merge_request_iid", SetProjectStatusInput{ProjectID: "1", SHA: "abc", ExternalStatusCheckID: 1, Status: "passed"}},
		{"missing sha", SetProjectStatusInput{ProjectID: "1", MRIID: 10, ExternalStatusCheckID: 1, Status: "passed"}},
		{"missing check_id", SetProjectStatusInput{ProjectID: "1", MRIID: 10, SHA: "abc", Status: "passed"}},
		{"missing status", SetProjectStatusInput{ProjectID: "1", MRIID: 10, SHA: "abc", ExternalStatusCheckID: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SetProjectMRExternalStatusCheckStatus(context.Background(), client, tt.input)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

// TestToMergeStatusCheckOutput_Conversion verifies the ToMergeStatusCheckOutput_Conversion handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestToMergeStatusCheckOutput_Conversion(t *testing.T) {
	check := &gl.MergeStatusCheck{
		ID:          99,
		Name:        "Test",
		ExternalURL: "https://test.com",
		Status:      "failed",
	}
	out := toMergeStatusCheckOutput(check)
	if out.ID != 99 || out.Name != "Test" || out.ExternalURL != "https://test.com" || out.Status != "failed" {
		t.Errorf("unexpected output: %+v", out)
	}
}

// TestToProjectStatusCheckOutput_Conversion verifies the ToProjectStatusCheckOutput_Conversion handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestToProjectStatusCheckOutput_Conversion(t *testing.T) {
	check := &gl.ProjectStatusCheck{
		ID:          42,
		Name:        "Scan",
		ProjectID:   1,
		ExternalURL: "https://scan.com",
		HMAC:        true,
		ProtectedBranches: []gl.StatusCheckProtectedBranch{
			{ID: 100, ProjectID: 1, Name: "main", CodeOwnerApprovalRequired: true},
		},
	}
	out := toProjectStatusCheckOutput(check)
	if out.ID != 42 || out.Name != "Scan" || out.ProjectID != 1 {
		t.Errorf("unexpected output: %+v", out)
	}
	if !out.HMAC {
		t.Error("expected HMAC=true")
	}
	if len(out.ProtectedBranches) != 1 {
		t.Fatalf("expected 1 branch, got %d", len(out.ProtectedBranches))
	}
	if out.ProtectedBranches[0].Name != "main" || !out.ProtectedBranches[0].CodeOwnerApprovalRequired {
		t.Errorf("unexpected branch: %+v", out.ProtectedBranches[0])
	}
}

// TestToProjectStatusCheckOutput_NoBranches verifies the ToProjectStatusCheckOutput_NoBranches handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestToProjectStatusCheckOutput_NoBranches(t *testing.T) {
	check := &gl.ProjectStatusCheck{
		ID:   1,
		Name: "No Branches",
	}
	out := toProjectStatusCheckOutput(check)
	if len(out.ProtectedBranches) != 0 {
		t.Errorf("expected 0 branches, got %d", len(out.ProtectedBranches))
	}
}

// TestListProjectStatusChecks_ContextCancelled verifies the ListProjectStatusChecks_ContextCancelled handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestListProjectStatusChecks_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	ctx := testutil.CancelledCtx(t)
	_, err := ListProjectStatusChecks(ctx, client, ListProjectStatusChecksInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestListProjectStatusChecks_APIError verifies that ListProjectStatusChecks returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListProjectStatusChecks_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/1/external_status_checks", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := ListProjectStatusChecks(context.Background(), client, ListProjectStatusChecksInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

// TestListProjectStatusChecks_WithPagination verifies that ListProjectStatusChecks_WithPagination forwards pagination parameters to the GitLab API and parses the response metadata.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestListProjectStatusChecks_WithPagination(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/1/external_status_checks", func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertQueryParam(t, r, "page", "3")
		testutil.AssertQueryParam(t, r, "per_page", "10")
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`, testutil.PaginationHeaders{
			Page: "3", TotalPages: "5", PerPage: "10", Total: "50",
		})
	})
	client := testutil.NewTestClient(t, mux)
	out, err := ListProjectStatusChecks(context.Background(), client, ListProjectStatusChecksInput{
		ProjectID: "1",
		Page:      3, PerPage: 10,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Pagination.Page != 3 {
		t.Errorf("expected page 3, got %d", out.Pagination.Page)
	}
}

// TestListProjectExternalStatusChecks_Keyset verifies that ListProjectExternalStatusChecks forwards keyset pagination and order_by/sort query parameters to the GitLab API.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts pagination, page_token, order_by, and sort reach the request URL.
func TestListProjectExternalStatusChecks_Keyset(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/1/external_status_checks", func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertQueryParam(t, r, "pagination", "keyset")
		testutil.AssertQueryParam(t, r, "page_token", "tok123")
		testutil.AssertQueryParam(t, r, "order_by", "id")
		testutil.AssertQueryParam(t, r, "sort", "desc")
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := ListProjectExternalStatusChecks(context.Background(), client, ListProjectInput{
		ProjectID:  "1",
		OrderBy:    "id",
		Sort:       "desc",
		Pagination: "keyset", PageToken: "tok123",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestListProjectStatusChecks_Keyset verifies that the deprecated ListProjectStatusChecks forwards keyset pagination and order_by/sort query parameters to the GitLab API.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts pagination, page_token, order_by, and sort reach the request URL.
func TestListProjectStatusChecks_Keyset(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/1/external_status_checks", func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertQueryParam(t, r, "pagination", "keyset")
		testutil.AssertQueryParam(t, r, "page_token", "tok456")
		testutil.AssertQueryParam(t, r, "order_by", "id")
		testutil.AssertQueryParam(t, r, "sort", "asc")
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := ListProjectStatusChecks(context.Background(), client, ListProjectStatusChecksInput{
		ProjectID:  "1",
		OrderBy:    "id",
		Sort:       "asc",
		Pagination: "keyset", PageToken: "tok456",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestListProjectMRExternalStatusChecks_Keyset verifies that ListProjectMRExternalStatusChecks forwards keyset pagination and order_by/sort query parameters to the GitLab API.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts pagination, page_token, order_by, and sort reach the request URL.
func TestListProjectMRExternalStatusChecks_Keyset(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/1/merge_requests/10/status_checks", func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertQueryParam(t, r, "pagination", "keyset")
		testutil.AssertQueryParam(t, r, "page_token", "tok789")
		testutil.AssertQueryParam(t, r, "order_by", "id")
		testutil.AssertQueryParam(t, r, "sort", "desc")
		testutil.RespondJSON(w, http.StatusOK, mergeStatusCheckListJSON)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := ListProjectMRExternalStatusChecks(context.Background(), client, ListProjectMRInput{
		ProjectID:  "1",
		MRIID:      10,
		OrderBy:    "id",
		Sort:       "desc",
		Pagination: "keyset", PageToken: "tok789",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestListProjectMRExternalStatusChecks_ContextCancelled verifies the ListProjectMRExternalStatusChecks_ContextCancelled handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestListProjectMRExternalStatusChecks_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	ctx := testutil.CancelledCtx(t)
	_, err := ListProjectMRExternalStatusChecks(ctx, client, ListProjectMRInput{ProjectID: "1", MRIID: 10})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestListProjectMRExternalStatusChecks_APIError verifies that ListProjectMRExternalStatusChecks returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListProjectMRExternalStatusChecks_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/1/merge_requests/10/status_checks", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := ListProjectMRExternalStatusChecks(context.Background(), client, ListProjectMRInput{ProjectID: "1", MRIID: 10})
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

// TestListProjectMRExternalStatusChecks_WithPagination verifies that ListProjectMRExternalStatusChecks_WithPagination forwards pagination parameters to the GitLab API and parses the response metadata.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestListProjectMRExternalStatusChecks_WithPagination(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/1/merge_requests/10/status_checks", func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertQueryParam(t, r, "page", "2")
		testutil.AssertQueryParam(t, r, "per_page", "15")
		testutil.RespondJSONWithPagination(w, http.StatusOK, mergeStatusCheckListJSON, testutil.PaginationHeaders{
			Page: "2", TotalPages: "3", PerPage: "15", Total: "30",
		})
	})
	client := testutil.NewTestClient(t, mux)
	out, err := ListProjectMRExternalStatusChecks(context.Background(), client, ListProjectMRInput{
		ProjectID: "1",
		MRIID:     10,
		Page:      2, PerPage: 15,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Pagination.Page != 2 {
		t.Errorf("expected page 2, got %d", out.Pagination.Page)
	}
}

// TestListProjectExternalStatusChecks_ContextCancelled verifies the ListProjectExternalStatusChecks_ContextCancelled handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestListProjectExternalStatusChecks_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	ctx := testutil.CancelledCtx(t)
	_, err := ListProjectExternalStatusChecks(ctx, client, ListProjectInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestListProjectExternalStatusChecks_APIError verifies that ListProjectExternalStatusChecks returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListProjectExternalStatusChecks_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/1/external_status_checks", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := ListProjectExternalStatusChecks(context.Background(), client, ListProjectInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

// TestListProjectExternalStatusChecks_WithPagination verifies that ListProjectExternalStatusChecks_WithPagination forwards pagination parameters to the GitLab API and parses the response metadata.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestListProjectExternalStatusChecks_WithPagination(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/1/external_status_checks", func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertQueryParam(t, r, "page", "4")
		testutil.AssertQueryParam(t, r, "per_page", "25")
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`, testutil.PaginationHeaders{
			Page: "4", TotalPages: "5", PerPage: "25", Total: "120",
		})
	})
	client := testutil.NewTestClient(t, mux)
	out, err := ListProjectExternalStatusChecks(context.Background(), client, ListProjectInput{
		ProjectID: "1",
		Page:      4, PerPage: 25,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Pagination.Page != 4 {
		t.Errorf("expected page 4, got %d", out.Pagination.Page)
	}
}

// TestCreateProjectExternalStatusCheck_ContextCancelled verifies the CreateProjectExternalStatusCheck_ContextCancelled handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestCreateProjectExternalStatusCheck_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	ctx := testutil.CancelledCtx(t)
	_, err := CreateProjectExternalStatusCheck(ctx, client, CreateProjectInput{
		ProjectID: "1", Name: "x", ExternalURL: "https://x.com",
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestCreateProjectExternalStatusCheck_APIError verifies that CreateProjectExternalStatusCheck returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateProjectExternalStatusCheck_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/projects/1/external_status_checks", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusUnprocessableEntity, `{"message":"422 Unprocessable"}`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := CreateProjectExternalStatusCheck(context.Background(), client, CreateProjectInput{
		ProjectID: "1", Name: "x", ExternalURL: "https://x.com",
	})
	if err == nil {
		t.Fatal("expected error for 422 response, got nil")
	}
}

// TestDeleteProjectExternalStatusCheck_ContextCancelled verifies the DeleteProjectExternalStatusCheck_ContextCancelled handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestDeleteProjectExternalStatusCheck_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	ctx := testutil.CancelledCtx(t)
	err := DeleteProjectExternalStatusCheck(ctx, client, DeleteProjectInput{ProjectID: "1", CheckID: 42})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestDeleteProjectExternalStatusCheck_APIError verifies that DeleteProjectExternalStatusCheck returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteProjectExternalStatusCheck_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v4/projects/1/external_status_checks/42", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	})
	client := testutil.NewTestClient(t, mux)
	err := DeleteProjectExternalStatusCheck(context.Background(), client, DeleteProjectInput{ProjectID: "1", CheckID: 42})
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}
}

// TestUpdateProjectExternalStatusCheck_ContextCancelled verifies the UpdateProjectExternalStatusCheck_ContextCancelled handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestUpdateProjectExternalStatusCheck_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	ctx := testutil.CancelledCtx(t)
	_, err := UpdateProjectExternalStatusCheck(ctx, client, UpdateProjectInput{ProjectID: "1", CheckID: 42})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestUpdateProjectExternalStatusCheck_APIError verifies that UpdateProjectExternalStatusCheck returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestUpdateProjectExternalStatusCheck_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/v4/projects/1/external_status_checks/42", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := UpdateProjectExternalStatusCheck(context.Background(), client, UpdateProjectInput{ProjectID: "1", CheckID: 42, Name: "x"})
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

// TestRetryFailedExternalStatusCheckForProjectMR_ContextCancelled verifies the RetryFailedExternalStatusCheckForProjectMR_ContextCancelled handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestRetryFailedExternalStatusCheckForProjectMR_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	ctx := testutil.CancelledCtx(t)
	err := RetryFailedExternalStatusCheckForProjectMR(ctx, client, RetryProjectInput{ProjectID: "1", MRIID: 10, CheckID: 42})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestRetryFailedExternalStatusCheckForProjectMR_APIError verifies that RetryFailedExternalStatusCheckForProjectMR returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestRetryFailedExternalStatusCheckForProjectMR_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/projects/1/merge_requests/10/status_checks/42/retry", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	})
	client := testutil.NewTestClient(t, mux)
	err := RetryFailedExternalStatusCheckForProjectMR(context.Background(), client, RetryProjectInput{ProjectID: "1", MRIID: 10, CheckID: 42})
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}
}

// TestSetProjectMRExternalStatusCheckStatus_ContextCancelled verifies the SetProjectMRExternalStatusCheckStatus_ContextCancelled handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestSetProjectMRExternalStatusCheckStatus_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	ctx := testutil.CancelledCtx(t)
	err := SetProjectMRExternalStatusCheckStatus(ctx, client, SetProjectStatusInput{
		ProjectID: "1", MRIID: 10, SHA: "abc", ExternalStatusCheckID: 1, Status: "passed",
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestSetProjectMRExternalStatusCheckStatus_APIError verifies that SetProjectMRExternalStatusCheckStatus returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestSetProjectMRExternalStatusCheckStatus_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/projects/1/merge_requests/10/status_check_responses", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusUnprocessableEntity, `{"message":"422 Unprocessable"}`)
	})
	client := testutil.NewTestClient(t, mux)
	err := SetProjectMRExternalStatusCheckStatus(context.Background(), client, SetProjectStatusInput{
		ProjectID: "1", MRIID: 10, SHA: "abc", ExternalStatusCheckID: 42, Status: "passed",
	})
	if err == nil {
		t.Fatal("expected error for 422 response, got nil")
	}
}

// TestExternalStatusChecks_IdentifierAtZero_RefusedBeforeReachingGitLab
// verifies that every handler taking a numeric identifier refuses a zero one
// itself, naming the field, instead of building a request around it.
//
// Zero is the value a missing identifier arrives as: meta and dynamic dispatch
// decode the arguments into the input struct, so a model that spells
// `mr_iid` instead of `merge_request_iid` leaves `MRIID` at its zero and
// nothing else notices. The guards are written `<= 0` for that reason, and the
// difference between `<= 0` and `< 0` is invisible to a test that only asks
// for an error: with the guard weakened, the request goes out as
// `/merge_requests/0/…`, GitLab answers 404, and an error still comes back —
// one that tells the model to check its permissions rather than that it named
// the parameter wrongly. This test therefore asserts both halves: the message
// is the local one, and nothing reached the server.
func TestExternalStatusChecks_IdentifierAtZero_RefusedBeforeReachingGitLab(t *testing.T) {
	var requests atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		t.Errorf("a zero identifier reached GitLab as %s %s", r.Method, r.URL.Path)
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	})
	client := testutil.NewTestClient(t, mux)

	tests := []struct {
		name  string
		field string
		call  func(context.Context) error
	}{
		{"list mr checks", "merge_request_iid", func(ctx context.Context) error {
			_, err := ListProjectMRExternalStatusChecks(ctx, client, ListProjectMRInput{ProjectID: "1"})
			return err
		}},
		{"delete check", "check_id", func(ctx context.Context) error {
			return DeleteProjectExternalStatusCheck(ctx, client, DeleteProjectInput{ProjectID: "1"})
		}},
		{"update check", "check_id", func(ctx context.Context) error {
			_, err := UpdateProjectExternalStatusCheck(ctx, client, UpdateProjectInput{ProjectID: "1", Name: "Updated"})
			return err
		}},
		{"retry without merge_request_iid", "merge_request_iid", func(ctx context.Context) error {
			return RetryFailedExternalStatusCheckForProjectMR(ctx, client, RetryProjectInput{ProjectID: "1", CheckID: 42})
		}},
		{"retry without check_id", "check_id", func(ctx context.Context) error {
			return RetryFailedExternalStatusCheckForProjectMR(ctx, client, RetryProjectInput{ProjectID: "1", MRIID: 10})
		}},
		{"set status without merge_request_iid", "merge_request_iid", func(ctx context.Context) error {
			return SetProjectMRExternalStatusCheckStatus(ctx, client, SetProjectStatusInput{
				ProjectID: "1", SHA: "abc", ExternalStatusCheckID: 42, Status: "passed",
			})
		}},
		{"set status without external_status_check_id", "external_status_check_id", func(ctx context.Context) error {
			return SetProjectMRExternalStatusCheckStatus(ctx, client, SetProjectStatusInput{
				ProjectID: "1", MRIID: 10, SHA: "abc", Status: "passed",
			})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := requests.Load()

			err := tt.call(t.Context())
			if err == nil {
				t.Fatalf("expected an error for a zero %s, got nil", tt.field)
			}
			if !strings.Contains(err.Error(), tt.field) || !strings.Contains(err.Error(), "must be > 0") {
				t.Errorf("error = %q, want the local refusal naming %s and requiring > 0", err, tt.field)
			}
			if got := requests.Load() - before; got != 0 {
				t.Errorf("handler issued %d request(s) for a zero %s, want 0", got, tt.field)
			}
		})
	}
}

// TestCreateProjectExternalStatusCheck_OptionalFields_ReachTheRequestBody
// verifies that a shared secret and a protected-branch scope the caller gave
// are both sent, and sent as given.
//
// Each is guarded by an "is it set" check whose failure is silent: the check
// is created, GitLab answers 201, and the handler returns the created check,
// so the only evidence that the HMAC secret was dropped is that the external
// service's later callbacks are rejected, and the only evidence the branch
// scope was dropped is a check that fires on every branch. Nothing about the
// response says either happened, which is why the assertion is on the request.
func TestCreateProjectExternalStatusCheck_OptionalFields_ReachTheRequestBody(t *testing.T) {
	client, fields := captureRequestFields(t, "POST /api/v4/projects/1/external_status_checks", http.StatusCreated, projectStatusCheckJSON)

	if _, err := CreateProjectExternalStatusCheck(context.Background(), client, CreateProjectInput{
		ProjectID:          "1",
		Name:               "Security Scan",
		ExternalURL:        "https://scan.example.com",
		SharedSecret:       "secret123",
		ProtectedBranchIDs: []int64{100, 200},
	}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}

	body := fields()
	for _, tt := range []struct {
		field string
		want  string
	}{
		{"name", `"Security Scan"`},
		{"external_url", `"https://scan.example.com"`},
		{"shared_secret", `"secret123"`},
		{"protected_branch_ids", `[100,200]`},
	} {
		t.Run(tt.field, func(t *testing.T) {
			got, ok := body[tt.field]
			if !ok {
				t.Fatalf("request body %v carries no %s", body, tt.field)
			}
			if string(got) != tt.want {
				t.Errorf("%s = %s, want %s", tt.field, got, tt.want)
			}
		})
	}
}

// TestCreateProjectExternalStatusCheck_OptionalFieldsUnset_StayOutOfTheBody
// and its update sibling verify that a field the caller left empty is absent
// from the request rather than present and null.
//
// The SDK's optional fields are pointers, so taking the address of an empty
// slice unconditionally would satisfy `omitempty` and put
// `"protected_branch_ids": null` on the wire: a payload that speaks about the
// branch scope, sent by a caller who said nothing about it. The guard is the
// only thing keeping the two apart, and no assertion on the decoded body can
// see the difference, since an explicit null and an absent key both decode to
// nil — hence the raw-key comparison.
func TestCreateProjectExternalStatusCheck_OptionalFieldsUnset_StayOutOfTheBody(t *testing.T) {
	client, fields := captureRequestFields(t, "POST /api/v4/projects/1/external_status_checks", http.StatusCreated, projectStatusCheckJSON)

	if _, err := CreateProjectExternalStatusCheck(context.Background(), client, CreateProjectInput{
		ProjectID:   "1",
		Name:        "Security Scan",
		ExternalURL: "https://scan.example.com",
	}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}

	assertFieldsAbsent(t, fields(), "shared_secret", "protected_branch_ids")
}

// TestUpdateProjectExternalStatusCheck_OptionalFieldsUnset_StayOutOfTheBody
// verifies the same of the update, where it decides more: a PUT naming
// protected_branch_ids is a PUT that speaks about the branch scope, and a
// caller who only renamed a check must not be made to say anything about the
// branches it applies to.
func TestUpdateProjectExternalStatusCheck_OptionalFieldsUnset_StayOutOfTheBody(t *testing.T) {
	client, fields := captureRequestFields(t, "PUT /api/v4/projects/1/external_status_checks/42", http.StatusOK, projectStatusCheckJSON)

	if _, err := UpdateProjectExternalStatusCheck(context.Background(), client, UpdateProjectInput{
		ProjectID: "1",
		CheckID:   42,
		Name:      "Updated",
	}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}

	body := fields()
	if got, ok := body["name"]; !ok || string(got) != `"Updated"` {
		t.Errorf("name = %s (present: %t), want \"Updated\"", got, ok)
	}
	assertFieldsAbsent(t, body, "external_url", "shared_secret", "protected_branch_ids")
}

// captureRequestFields answers one request on pattern with response, and
// returns the client to drive it with alongside a reader of the top-level
// fields of the JSON body the handler was sent.
//
// The fields come back as raw messages rather than decoded into a struct
// because the callers ask whether a key is in the body at all: encoding/json
// gives an absent key and an explicit null the same nil, and the difference
// between them is the whole question for an optional field on a PUT.
func captureRequestFields(t *testing.T, pattern string, status int, response string) (*gitlabclient.Client, func() map[string]json.RawMessage) {
	t.Helper()

	var captured []byte
	mux := http.NewServeMux()
	mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			http.Error(w, "read request body", http.StatusInternalServerError)
			return
		}
		captured = body
		testutil.RespondJSON(w, status, response)
	})

	return testutil.NewTestClient(t, mux), func() map[string]json.RawMessage {
		t.Helper()
		fields := map[string]json.RawMessage{}
		if err := json.Unmarshal(captured, &fields); err != nil {
			t.Fatalf("decode captured request body %q: %v", captured, err)
		}
		return fields
	}
}

// assertFieldsAbsent reports every named key the request body carries, quoting
// what it carried: "present as null" and "present with a value" are different
// defects and the message says which one happened.
func assertFieldsAbsent(t *testing.T, body map[string]json.RawMessage, fields ...string) {
	t.Helper()
	for _, field := range fields {
		t.Run(field, func(t *testing.T) {
			if got, ok := body[field]; ok {
				t.Errorf("request body carries %s as %s, want the key absent", field, got)
			}
		})
	}
}

// statusCheckCalls drives each of the eight status check entry points once,
// named by what each one's refusals are about: the project routes, which
// refuse the license with 401 and, for the lists and the update, the role too
// (the create answers the role with 500 and the delete with 204), and the
// merge request routes, which refuse the license with 401 and the role with
// 403.
func statusCheckCalls() map[string]func(*gitlabclient.Client) error {
	ctx := context.Background()
	return map[string]func(*gitlabclient.Client) error{
		"list (deprecated)": func(c *gitlabclient.Client) error {
			_, err := ListProjectStatusChecks(ctx, c, ListProjectStatusChecksInput{ProjectID: "1"})
			return err
		},
		"list": func(c *gitlabclient.Client) error {
			_, err := ListProjectExternalStatusChecks(ctx, c, ListProjectInput{ProjectID: "1"})
			return err
		},
		"create": func(c *gitlabclient.Client) error {
			_, err := CreateProjectExternalStatusCheck(ctx, c, CreateProjectInput{ProjectID: "1", Name: "QA", ExternalURL: "https://qa.example.com"})
			return err
		},
		"update": func(c *gitlabclient.Client) error {
			_, err := UpdateProjectExternalStatusCheck(ctx, c, UpdateProjectInput{ProjectID: "1", CheckID: 42})
			return err
		},
		"delete": func(c *gitlabclient.Client) error {
			return DeleteProjectExternalStatusCheck(ctx, c, DeleteProjectInput{ProjectID: "1", CheckID: 42})
		},
		"list merge request checks": func(c *gitlabclient.Client) error {
			_, err := ListProjectMRExternalStatusChecks(ctx, c, ListProjectMRInput{ProjectID: "1", MRIID: 5})
			return err
		},
		"retry": func(c *gitlabclient.Client) error {
			return RetryFailedExternalStatusCheckForProjectMR(ctx, c, RetryProjectInput{ProjectID: "1", MRIID: 5, CheckID: 42})
		},
		"set status": func(c *gitlabclient.Client) error {
			return SetProjectMRExternalStatusCheckStatus(ctx, c, SetProjectStatusInput{
				ProjectID: "1", MRIID: 5, SHA: "abc", ExternalStatusCheckID: 42, Status: "passed",
			})
		},
	}
}

// refusedBy answers every request with status and Grape's plain refusal body,
// the one ee/lib/api/status_checks.rb:16 renders for a project whose
// namespace lacks the Ultimate feature.
func refusedBy(t *testing.T, status int) *gitlabclient.Client {
	t.Helper()
	return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, status, `{"message":"`+http.StatusText(status)+`"}`)
	}))
}

// TestStatusChecks_PermissionRefusedWith401_NameTheUltimateLicense verifies
// that every status check entry point hints the license GitLab checks before
// anything else, on the 401 it answers every route with when the project's
// namespace lacks Ultimate. Only the two lists hinted at all, on a 403 GitLab
// never sends for it, and they said Premium/Ultimate: status checks are an
// Ultimate feature, so no error may name Premium.
//
// The project list and the update also refuse a missing Maintainer role with
// 401 (status_checks.rb:67, update_service.rb:40), so theirs names the role
// too. The delete's does not: that route answers a missing role with 204 and
// deletes nothing, so a hint naming the role would describe a refusal GitLab
// never sends.
func TestStatusChecks_PermissionRefusedWith401_NameTheUltimateLicense(t *testing.T) {
	namesTheRole := map[string]bool{"list (deprecated)": true, "list": true, "update": true}
	for name, call := range statusCheckCalls() {
		t.Run(name, func(t *testing.T) {
			err := call(refusedBy(t, http.StatusUnauthorized))
			if err == nil {
				t.Fatalf("%s error = nil, want the 401", name)
			}
			if !strings.Contains(err.Error(), "Ultimate license on the project's namespace") {
				t.Errorf("%s error = %q, want the Ultimate license named", name, err)
			}
			if strings.Contains(err.Error(), "Premium") {
				t.Errorf("%s error = %q, must not name Premium for an Ultimate feature", name, err)
			}
			if got := strings.Contains(err.Error(), "Maintainer"); got != namesTheRole[name] {
				t.Errorf("%s error = %q names the Maintainer role: %v, want %v", name, err, got, namesTheRole[name])
			}
		})
	}
}

// TestStatusChecks_CreateRefusedWith500NotAllowed_NamesTheMaintainerRole
// verifies the create names the role on the answer GitLab gives a caller
// without it, which is neither 401 nor 403: the create service builds its
// refusal with no status, so Grape answers it 500 with the service's "Not
// allowed" (create_service.rb:32-38, status_checks.rb:53). Without the hint a
// model reads a fault of the instance and retries. A 500 without that message
// is the instance's own and names no role.
func TestStatusChecks_CreateRefusedWith500NotAllowed_NamesTheMaintainerRole(t *testing.T) {
	create := statusCheckCalls()["create"]
	cases := []struct {
		name      string
		body      string
		wantsRole bool
	}{
		{"the create service refused the role", `{"message":["Not allowed"]}`, true},
		{"the instance failed", `{"message":"500 Internal Server Error"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusInternalServerError, tc.body)
			}))
			err := create(client)
			if err == nil {
				t.Fatal("create error = nil, want the 500")
			}
			if got := strings.Contains(err.Error(), "needs the Maintainer role"); got != tc.wantsRole {
				t.Errorf("create error = %q names the Maintainer role: %v, want %v", err, got, tc.wantsRole)
			}
		})
	}
}

// TestStatusChecks_MergeRequestRoutesRefusedWith403_NameTheRoleNotTheLicense
// verifies the merge request routes keep their 403 apart from their 401.
// Those routes answer a caller whose role on the merge request falls short
// with 403 from authorize! (status_checks.rb:172, 204 and 235), which is not
// the license, so the license hint must not follow it; the project routes
// answer no 403 of their own, so a plain one there is read like their 401.
func TestStatusChecks_MergeRequestRoutesRefusedWith403_NameTheRoleNotTheLicense(t *testing.T) {
	roles := map[string]string{
		"list merge request checks": "Reporter role",
		"retry":                     "Developer role",
		"set status":                "permission to approve the merge request",
	}
	for name, call := range statusCheckCalls() {
		t.Run(name, func(t *testing.T) {
			err := call(refusedBy(t, http.StatusForbidden))
			if err == nil {
				t.Fatalf("%s error = nil, want the 403", name)
			}
			role, mergeRequestRoute := roles[name]
			if !mergeRequestRoute {
				if !strings.Contains(err.Error(), "Ultimate license") {
					t.Errorf("%s error = %q, want a plain 403 read like the 401", name, err)
				}
				return
			}
			if !strings.Contains(err.Error(), role) {
				t.Errorf("%s error = %q, want the role hint %q", name, err, role)
			}
			if strings.Contains(err.Error(), "Ultimate license") {
				t.Errorf("%s error = %q, must not blame the license for a role refusal", name, err)
			}
		})
	}
}

// TestStatusChecks_CredentialRefused_CarryNoPermissionHint verifies that a
// 401 GitLab said was about the token itself, and the API guard's 403 about a
// token scope, get neither the license nor a role hint on any entry point.
func TestStatusChecks_CredentialRefused_CarryNoPermissionHint(t *testing.T) {
	bodies := map[int]string{
		http.StatusUnauthorized: `{"error":"invalid_token","error_description":"Token is expired. You can either do re-authorization or token refresh."}`,
		http.StatusForbidden:    `{"error":"insufficient_scope","error_description":"The request requires higher privileges than provided by the access token.","scope":"api"}`,
	}
	for status, body := range bodies {
		for name, call := range statusCheckCalls() {
			t.Run(fmt.Sprintf("%s %d", name, status), func(t *testing.T) {
				client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondJSON(w, status, body)
				}))
				err := call(client)
				if err == nil {
					t.Fatalf("%s error = nil, want the %d", name, status)
				}
				if strings.Contains(err.Error(), "Suggestion") {
					t.Errorf("%s error = %q, must carry no hint after a refusal of the credential", name, err)
				}
			})
		}
	}
}

// TestDeleteProjectExternalStatusCheck_NotFound_NamesTheCheckList verifies
// the 404 hint the delete took over from its old 403 hint, which named a
// role this route never refuses.
func TestDeleteProjectExternalStatusCheck_NotFound_NamesTheCheckList(t *testing.T) {
	err := DeleteProjectExternalStatusCheck(context.Background(), refusedBy(t, http.StatusNotFound), DeleteProjectInput{ProjectID: "1", CheckID: 42})
	if err == nil {
		t.Fatal("DeleteProjectExternalStatusCheck() error = nil, want the 404")
	}
	if !strings.Contains(err.Error(), "external_status_check.list_project") {
		t.Errorf("DeleteProjectExternalStatusCheck() error = %q, want the check list named", err)
	}
}
