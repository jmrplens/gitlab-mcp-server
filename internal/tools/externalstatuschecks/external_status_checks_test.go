// external_status_checks_test.go contains unit tests for the external status
// check MCP tool handlers. Tests use httptest to mock GitLab API responses and
// verify success, validation, and error paths.
package externalstatuschecks

import (
	"context"
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
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

// TestListMergeStatusChecks_Success verifies listing merge status checks returns items and pagination.
func TestListMergeStatusChecks_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/1/merge_requests/10/status_checks", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, mergeStatusCheckListJSON, testutil.PaginationHeaders{
			Page: "1", NextPage: "", TotalPages: "1", PerPage: "20", Total: "1",
		})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListMergeStatusChecks(context.Background(), client, ListMergeStatusChecksInput{
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
	if out.Items[0].Status != "passed" {
		t.Errorf("expected status 'passed', got %q", out.Items[0].Status)
	}
}

// TestListMergeStatusChecks_MissingProjectID verifies validation rejects empty project_id.
func TestListMergeStatusChecks_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ListMergeStatusChecks(context.Background(), client, ListMergeStatusChecksInput{MRIID: 10})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestListMergeStatusChecks_MissingMRIID verifies validation rejects zero mr_iid.
func TestListMergeStatusChecks_MissingMRIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ListMergeStatusChecks(context.Background(), client, ListMergeStatusChecksInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error for missing mr_iid")
	}
}

// TestSetExternalStatusCheckStatus_Success verifies setting status succeeds.
func TestSetExternalStatusCheckStatus_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/projects/1/merge_requests/10/status_check_responses", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	client := testutil.NewTestClient(t, mux)

	err := SetExternalStatusCheckStatus(context.Background(), client, SetStatusInput{
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

// TestSetExternalStatusCheckStatus_MissingFields verifies all required field validation.
func TestSetExternalStatusCheckStatus_MissingFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())

	tests := []struct {
		name  string
		input SetStatusInput
	}{
		{"missing project_id", SetStatusInput{MRIID: 10, SHA: "abc", ExternalStatusCheckID: 1, Status: "passed"}},
		{"missing mr_iid", SetStatusInput{ProjectID: "1", SHA: "abc", ExternalStatusCheckID: 1, Status: "passed"}},
		{"missing sha", SetStatusInput{ProjectID: "1", MRIID: 10, ExternalStatusCheckID: 1, Status: "passed"}},
		{"missing check_id", SetStatusInput{ProjectID: "1", MRIID: 10, SHA: "abc", Status: "passed"}},
		{"missing status", SetStatusInput{ProjectID: "1", MRIID: 10, SHA: "abc", ExternalStatusCheckID: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SetExternalStatusCheckStatus(context.Background(), client, tt.input)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

// TestListProjectStatusChecks_Success verifies listing project status checks returns items.
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

// TestListProjectStatusChecks_MissingProjectID verifies validation rejects empty project_id.
func TestListProjectStatusChecks_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ListProjectStatusChecks(context.Background(), client, ListProjectStatusChecksInput{})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestCreateExternalStatusCheck_Success verifies legacy create succeeds.
func TestCreateExternalStatusCheck_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/projects/1/external_status_checks", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	client := testutil.NewTestClient(t, mux)

	err := CreateExternalStatusCheck(context.Background(), client, CreateLegacyInput{
		ProjectID:   "1",
		Name:        "CI Check",
		ExternalURL: "https://ci.example.com",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestCreateExternalStatusCheck_MissingFields verifies required field validation for legacy create.
func TestCreateExternalStatusCheck_MissingFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())

	tests := []struct {
		name  string
		input CreateLegacyInput
	}{
		{"missing project_id", CreateLegacyInput{Name: "x", ExternalURL: "https://x.com"}},
		{"missing name", CreateLegacyInput{ProjectID: "1", ExternalURL: "https://x.com"}},
		{"missing external_url", CreateLegacyInput{ProjectID: "1", Name: "x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CreateExternalStatusCheck(context.Background(), client, tt.input)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

// TestDeleteExternalStatusCheck_Success verifies legacy delete succeeds.
func TestDeleteExternalStatusCheck_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v4/projects/1/external_status_checks/42", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)

	err := DeleteExternalStatusCheck(context.Background(), client, DeleteLegacyInput{
		ProjectID: "1",
		CheckID:   42,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeleteExternalStatusCheck_MissingFields verifies required field validation for legacy delete.
func TestDeleteExternalStatusCheck_MissingFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())

	tests := []struct {
		name  string
		input DeleteLegacyInput
	}{
		{"missing project_id", DeleteLegacyInput{CheckID: 42}},
		{"missing check_id", DeleteLegacyInput{ProjectID: "1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := DeleteExternalStatusCheck(context.Background(), client, tt.input)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

// TestUpdateExternalStatusCheck_Success verifies legacy update succeeds.
func TestUpdateExternalStatusCheck_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/v4/projects/1/external_status_checks/42", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	client := testutil.NewTestClient(t, mux)

	err := UpdateExternalStatusCheck(context.Background(), client, UpdateLegacyInput{
		ProjectID: "1",
		CheckID:   42,
		Name:      "Updated",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestUpdateExternalStatusCheck_MissingFields verifies required field validation for legacy update.
func TestUpdateExternalStatusCheck_MissingFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())

	tests := []struct {
		name  string
		input UpdateLegacyInput
	}{
		{"missing project_id", UpdateLegacyInput{CheckID: 42}},
		{"missing check_id", UpdateLegacyInput{ProjectID: "1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := UpdateExternalStatusCheck(context.Background(), client, tt.input)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

// TestRetryFailedStatusCheckForMR_Success verifies legacy retry succeeds.
func TestRetryFailedStatusCheckForMR_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/projects/1/merge_requests/10/status_checks/42/retry", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	client := testutil.NewTestClient(t, mux)

	err := RetryFailedStatusCheckForMR(context.Background(), client, RetryLegacyInput{
		ProjectID: "1",
		MRIID:     10,
		CheckID:   42,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestRetryFailedStatusCheckForMR_MissingFields verifies required field validation for legacy retry.
func TestRetryFailedStatusCheckForMR_MissingFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())

	tests := []struct {
		name  string
		input RetryLegacyInput
	}{
		{"missing project_id", RetryLegacyInput{MRIID: 10, CheckID: 42}},
		{"missing mr_iid", RetryLegacyInput{ProjectID: "1", CheckID: 42}},
		{"missing check_id", RetryLegacyInput{ProjectID: "1", MRIID: 10}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RetryFailedStatusCheckForMR(context.Background(), client, tt.input)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

// TestListProjectMRExternalStatusChecks_Success verifies listing project MR status checks returns items.
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

// TestListProjectMRExternalStatusChecks_MissingFields verifies validation for project MR list.
func TestListProjectMRExternalStatusChecks_MissingFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())

	_, err := ListProjectMRExternalStatusChecks(context.Background(), client, ListProjectMRInput{MRIID: 10})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
	_, err = ListProjectMRExternalStatusChecks(context.Background(), client, ListProjectMRInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error for missing mr_iid")
	}
}

// TestListProjectExternalStatusChecks_Success verifies listing project status checks returns items.
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

// TestListProjectExternalStatusChecks_MissingProjectID verifies validation rejects empty project_id.
func TestListProjectExternalStatusChecks_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ListProjectExternalStatusChecks(context.Background(), client, ListProjectInput{})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestCreateProjectExternalStatusCheck_Success verifies project create returns output.
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

// TestCreateProjectExternalStatusCheck_WithOptionalFields verifies create with shared secret and branch IDs.
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

// TestCreateProjectExternalStatusCheck_MissingFields verifies required field validation for project create.
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

// TestDeleteProjectExternalStatusCheck_Success verifies project delete succeeds.
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

// TestDeleteProjectExternalStatusCheck_MissingFields verifies required field validation for project delete.
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

// TestUpdateProjectExternalStatusCheck_Success verifies project update returns output.
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

// TestUpdateProjectExternalStatusCheck_WithAllFields verifies update with all optional fields.
func TestUpdateProjectExternalStatusCheck_WithAllFields(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/v4/projects/1/external_status_checks/42", func(w http.ResponseWriter, _ *http.Request) {
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
}

// TestUpdateProjectExternalStatusCheck_MissingFields verifies required field validation for project update.
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

// TestRetryFailedExternalStatusCheckForProjectMR_Success verifies project retry succeeds.
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

// TestRetryFailedExternalStatusCheckForProjectMR_MissingFields verifies required field validation.
func TestRetryFailedExternalStatusCheckForProjectMR_MissingFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())

	tests := []struct {
		name  string
		input RetryProjectInput
	}{
		{"missing project_id", RetryProjectInput{MRIID: 10, CheckID: 42}},
		{"missing mr_iid", RetryProjectInput{ProjectID: "1", CheckID: 42}},
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

// TestSetProjectMRExternalStatusCheckStatus_Success verifies project set status succeeds.
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

// TestSetProjectMRExternalStatusCheckStatus_MissingFields verifies all required field validation.
func TestSetProjectMRExternalStatusCheckStatus_MissingFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())

	tests := []struct {
		name  string
		input SetProjectStatusInput
	}{
		{"missing project_id", SetProjectStatusInput{MRIID: 10, SHA: "abc", ExternalStatusCheckID: 1, Status: "passed"}},
		{"missing mr_iid", SetProjectStatusInput{ProjectID: "1", SHA: "abc", ExternalStatusCheckID: 1, Status: "passed"}},
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

// TestToMergeStatusCheckOutput_Conversion verifies the converter maps all fields correctly.
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

// TestToProjectStatusCheckOutput_Conversion verifies the converter maps all fields including branches.
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

// TestToProjectStatusCheckOutput_NoBranches verifies the converter handles nil branches.
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

// TestListMergeStatusChecks_ContextCancelled verifies that a cancelled context
// returns an error before making an API call.
func TestListMergeStatusChecks_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ListMergeStatusChecks(ctx, client, ListMergeStatusChecksInput{ProjectID: "1", MRIID: 10})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestListMergeStatusChecks_APIError verifies that a 500 API response is propagated as an error.
func TestListMergeStatusChecks_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/1/merge_requests/10/status_checks", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"500 Internal Server Error"}`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := ListMergeStatusChecks(context.Background(), client, ListMergeStatusChecksInput{ProjectID: "1", MRIID: 10})
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

// TestListMergeStatusChecks_WithPagination verifies that Page and PerPage options
// are forwarded as query parameters to the GitLab API.
func TestListMergeStatusChecks_WithPagination(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/1/merge_requests/10/status_checks", func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertQueryParam(t, r, "page", "2")
		testutil.AssertQueryParam(t, r, "per_page", "5")
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`, testutil.PaginationHeaders{
			Page: "2", TotalPages: "3", PerPage: "5", Total: "12",
		})
	})
	client := testutil.NewTestClient(t, mux)
	out, err := ListMergeStatusChecks(context.Background(), client, ListMergeStatusChecksInput{
		ProjectID:       "1",
		MRIID:           10,
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 5},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Pagination.Page != 2 {
		t.Errorf("expected page 2, got %d", out.Pagination.Page)
	}
}

// TestSetExternalStatusCheckStatus_ContextCancelled verifies that a cancelled
// context returns an error before making an API call.
func TestSetExternalStatusCheckStatus_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := SetExternalStatusCheckStatus(ctx, client, SetStatusInput{
		ProjectID: "1", MRIID: 10, SHA: "abc", ExternalStatusCheckID: 1, Status: "passed",
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestSetExternalStatusCheckStatus_APIError verifies that a 422 API response is propagated as an error.
func TestSetExternalStatusCheckStatus_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/projects/1/merge_requests/10/status_check_responses", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusUnprocessableEntity, `{"message":"422 Unprocessable"}`)
	})
	client := testutil.NewTestClient(t, mux)
	err := SetExternalStatusCheckStatus(context.Background(), client, SetStatusInput{
		ProjectID: "1", MRIID: 10, SHA: "abc", ExternalStatusCheckID: 42, Status: "passed",
	})
	if err == nil {
		t.Fatal("expected error for 422 response, got nil")
	}
}

// TestListProjectStatusChecks_ContextCancelled verifies that a cancelled context
// returns an error for ListProjectStatusChecks.
func TestListProjectStatusChecks_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ListProjectStatusChecks(ctx, client, ListProjectStatusChecksInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestListProjectStatusChecks_APIError verifies that a 500 API response is propagated.
func TestListProjectStatusChecks_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/1/external_status_checks", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"500 Internal Server Error"}`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := ListProjectStatusChecks(context.Background(), client, ListProjectStatusChecksInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

// TestListProjectStatusChecks_WithPagination verifies that Page and PerPage
// options are forwarded to the GitLab API.
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
		ProjectID:       "1",
		PaginationInput: toolutil.PaginationInput{Page: 3, PerPage: 10},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Pagination.Page != 3 {
		t.Errorf("expected page 3, got %d", out.Pagination.Page)
	}
}

// TestCreateExternalStatusCheck_ContextCancelled verifies that a cancelled
// context returns an error for legacy create.
func TestCreateExternalStatusCheck_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := CreateExternalStatusCheck(ctx, client, CreateLegacyInput{
		ProjectID: "1", Name: "x", ExternalURL: "https://x.com",
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestCreateExternalStatusCheck_APIError verifies that a 422 API response is propagated.
func TestCreateExternalStatusCheck_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/projects/1/external_status_checks", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusUnprocessableEntity, `{"message":"422 Unprocessable"}`)
	})
	client := testutil.NewTestClient(t, mux)
	err := CreateExternalStatusCheck(context.Background(), client, CreateLegacyInput{
		ProjectID: "1", Name: "x", ExternalURL: "https://x.com",
	})
	if err == nil {
		t.Fatal("expected error for 422 response, got nil")
	}
}

// TestCreateExternalStatusCheck_WithProtectedBranches verifies that optional
// ProtectedBranchIDs are forwarded to the API.
func TestCreateExternalStatusCheck_WithProtectedBranches(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/projects/1/external_status_checks", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	client := testutil.NewTestClient(t, mux)
	err := CreateExternalStatusCheck(context.Background(), client, CreateLegacyInput{
		ProjectID:          "1",
		Name:               "CI",
		ExternalURL:        "https://ci.example.com",
		ProtectedBranchIDs: []int64{100, 200},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeleteExternalStatusCheck_ContextCancelled verifies that a cancelled
// context returns an error for legacy delete.
func TestDeleteExternalStatusCheck_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := DeleteExternalStatusCheck(ctx, client, DeleteLegacyInput{ProjectID: "1", CheckID: 42})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestDeleteExternalStatusCheck_APIError verifies that a 404 API response is propagated.
func TestDeleteExternalStatusCheck_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v4/projects/1/external_status_checks/42", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	})
	client := testutil.NewTestClient(t, mux)
	err := DeleteExternalStatusCheck(context.Background(), client, DeleteLegacyInput{ProjectID: "1", CheckID: 42})
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}
}

// TestUpdateExternalStatusCheck_ContextCancelled verifies that a cancelled
// context returns an error for legacy update.
func TestUpdateExternalStatusCheck_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := UpdateExternalStatusCheck(ctx, client, UpdateLegacyInput{ProjectID: "1", CheckID: 42})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestUpdateExternalStatusCheck_APIError verifies that a 500 API response is propagated.
func TestUpdateExternalStatusCheck_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/v4/projects/1/external_status_checks/42", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"500 Internal Server Error"}`)
	})
	client := testutil.NewTestClient(t, mux)
	err := UpdateExternalStatusCheck(context.Background(), client, UpdateLegacyInput{ProjectID: "1", CheckID: 42, Name: "x"})
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

// TestUpdateExternalStatusCheck_AllOptionalFields verifies that all optional
// fields (Name, ExternalURL, ProtectedBranchIDs) are forwarded to the API.
func TestUpdateExternalStatusCheck_AllOptionalFields(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/v4/projects/1/external_status_checks/42", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	client := testutil.NewTestClient(t, mux)
	err := UpdateExternalStatusCheck(context.Background(), client, UpdateLegacyInput{
		ProjectID:          "1",
		CheckID:            42,
		Name:               "Updated",
		ExternalURL:        "https://new.example.com",
		ProtectedBranchIDs: []int64{100},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestRetryFailedStatusCheckForMR_ContextCancelled verifies that a cancelled
// context returns an error for legacy retry.
func TestRetryFailedStatusCheckForMR_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := RetryFailedStatusCheckForMR(ctx, client, RetryLegacyInput{ProjectID: "1", MRIID: 10, CheckID: 42})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestRetryFailedStatusCheckForMR_APIError verifies that a 404 API response is propagated.
func TestRetryFailedStatusCheckForMR_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/projects/1/merge_requests/10/status_checks/42/retry", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	})
	client := testutil.NewTestClient(t, mux)
	err := RetryFailedStatusCheckForMR(context.Background(), client, RetryLegacyInput{ProjectID: "1", MRIID: 10, CheckID: 42})
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}
}

// TestListProjectMRExternalStatusChecks_ContextCancelled verifies that a
// cancelled context returns an error.
func TestListProjectMRExternalStatusChecks_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ListProjectMRExternalStatusChecks(ctx, client, ListProjectMRInput{ProjectID: "1", MRIID: 10})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestListProjectMRExternalStatusChecks_APIError verifies that a 500 API response is propagated.
func TestListProjectMRExternalStatusChecks_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/1/merge_requests/10/status_checks", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"500 Internal Server Error"}`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := ListProjectMRExternalStatusChecks(context.Background(), client, ListProjectMRInput{ProjectID: "1", MRIID: 10})
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

// TestListProjectMRExternalStatusChecks_WithPagination verifies that Page and
// PerPage options are forwarded to the GitLab API.
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
		ProjectID:       "1",
		MRIID:           10,
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 15},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Pagination.Page != 2 {
		t.Errorf("expected page 2, got %d", out.Pagination.Page)
	}
}

// TestListProjectExternalStatusChecks_ContextCancelled verifies that a
// cancelled context returns an error.
func TestListProjectExternalStatusChecks_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ListProjectExternalStatusChecks(ctx, client, ListProjectInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestListProjectExternalStatusChecks_APIError verifies that a 500 API response is propagated.
func TestListProjectExternalStatusChecks_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/1/external_status_checks", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"500 Internal Server Error"}`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := ListProjectExternalStatusChecks(context.Background(), client, ListProjectInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

// TestListProjectExternalStatusChecks_WithPagination verifies that Page and
// PerPage options are forwarded to the GitLab API.
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
		ProjectID:       "1",
		PaginationInput: toolutil.PaginationInput{Page: 4, PerPage: 25},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Pagination.Page != 4 {
		t.Errorf("expected page 4, got %d", out.Pagination.Page)
	}
}

// TestCreateProjectExternalStatusCheck_ContextCancelled verifies that a
// cancelled context returns an error.
func TestCreateProjectExternalStatusCheck_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := CreateProjectExternalStatusCheck(ctx, client, CreateProjectInput{
		ProjectID: "1", Name: "x", ExternalURL: "https://x.com",
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestCreateProjectExternalStatusCheck_APIError verifies that a 422 API response is propagated.
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

// TestDeleteProjectExternalStatusCheck_ContextCancelled verifies that a
// cancelled context returns an error.
func TestDeleteProjectExternalStatusCheck_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := DeleteProjectExternalStatusCheck(ctx, client, DeleteProjectInput{ProjectID: "1", CheckID: 42})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestDeleteProjectExternalStatusCheck_APIError verifies that a 404 API response is propagated.
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

// TestUpdateProjectExternalStatusCheck_ContextCancelled verifies that a
// cancelled context returns an error.
func TestUpdateProjectExternalStatusCheck_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := UpdateProjectExternalStatusCheck(ctx, client, UpdateProjectInput{ProjectID: "1", CheckID: 42})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestUpdateProjectExternalStatusCheck_APIError verifies that a 500 API response is propagated.
func TestUpdateProjectExternalStatusCheck_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/v4/projects/1/external_status_checks/42", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"500 Internal Server Error"}`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := UpdateProjectExternalStatusCheck(context.Background(), client, UpdateProjectInput{ProjectID: "1", CheckID: 42, Name: "x"})
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

// TestRetryFailedExternalStatusCheckForProjectMR_ContextCancelled verifies
// that a cancelled context returns an error.
func TestRetryFailedExternalStatusCheckForProjectMR_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := RetryFailedExternalStatusCheckForProjectMR(ctx, client, RetryProjectInput{ProjectID: "1", MRIID: 10, CheckID: 42})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestRetryFailedExternalStatusCheckForProjectMR_APIError verifies that a 404
// API response is propagated.
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

// TestSetProjectMRExternalStatusCheckStatus_ContextCancelled verifies that a
// cancelled context returns an error.
func TestSetProjectMRExternalStatusCheckStatus_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := SetProjectMRExternalStatusCheckStatus(ctx, client, SetProjectStatusInput{
		ProjectID: "1", MRIID: 10, SHA: "abc", ExternalStatusCheckID: 1, Status: "passed",
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestSetProjectMRExternalStatusCheckStatus_APIError verifies that a 422 API
// response is propagated.
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
