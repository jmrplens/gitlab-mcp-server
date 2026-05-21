// runners_test.go contains unit tests for the runner MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package runners

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/jobs"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	// errExpMissingProjectID identifies the err exp missing project ID constant used by this package.
	errExpMissingProjectID = "expected error for missing project_id"
	// errExpectedNil identifies the err expected nil constant used by this package.
	errExpectedNil = "expected error, got nil"
	// errExpMissingRunnerID identifies the err exp missing runner ID constant used by this package.
	errExpMissingRunnerID = "expected error for missing runner_id"
	// errExpMissingToken identifies the err exp missing token constant used by this package.
	errExpMissingToken = "expected error for missing token"
	// pathRunners identifies the path runners constant used by this package.
	pathRunners = "/api/v4/runners"
	// pathRunner10 identifies the path runner 10 constant used by this package.
	pathRunner10 = "/api/v4/runners/10"
)

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------.

// TestList_Success verifies List when success.
func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathRunners && r.Method == http.MethodGet {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"id":1,"description":"runner-1","name":"r1","paused":false,"is_shared":true,"runner_type":"instance_type","online":true,"status":"online"},
				{"id":2,"description":"runner-2","name":"r2","paused":true,"is_shared":false,"runner_type":"project_type","online":false,"status":"offline"}
			]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "2", TotalPages: "1"})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := List(context.Background(), client, ListInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Runners) != 2 {
		t.Fatalf("expected 2 runners, got %d", len(out.Runners))
	}
	if out.Runners[0].ID != 1 {
		t.Errorf("Runners[0].ID = %d, want 1", out.Runners[0].ID)
	}
	if out.Runners[0].Status != "online" {
		t.Errorf("Runners[0].Status = %q, want %q", out.Runners[0].Status, "online")
	}
	if out.Runners[0].Description != "runner-1" {
		t.Errorf("Runners[0].Description = %q, want %q", out.Runners[0].Description, "runner-1")
	}
	if !out.Runners[0].IsShared {
		t.Error("Runners[0].IsShared should be true")
	}
	if out.Runners[0].RunnerType != "instance_type" {
		t.Errorf("Runners[0].RunnerType = %q, want %q", out.Runners[0].RunnerType, "instance_type")
	}
	if !out.Runners[1].Paused {
		t.Error("second runner should be paused")
	}
	if out.Runners[1].RunnerType != "project_type" {
		t.Errorf("Runners[1].RunnerType = %q, want %q", out.Runners[1].RunnerType, "project_type")
	}
}

// TestList_WithFilters verifies List when with filters.
func TestList_WithFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathRunners {
			if r.URL.Query().Get("type") != "project_type" {
				t.Errorf("expected type=project_type, got %s", r.URL.Query().Get("type"))
			}
			if r.URL.Query().Get("status") != "online" {
				t.Errorf("expected status=online, got %s", r.URL.Query().Get("status"))
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := List(context.Background(), client, ListInput{Type: "project_type", Status: "online"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Runners) != 0 {
		t.Errorf("expected 0 runners, got %d", len(out.Runners))
	}
}

// TestList_APIError verifies List when API error.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------.

// TestGet_Success verifies Get when success.
func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathRunner10 && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":10,"description":"my-runner","name":"mr-10","paused":false,"is_shared":false,
				"runner_type":"project_type","online":true,"status":"online",
				"tag_list":["docker","linux"],"run_untagged":true,"locked":false,
				"access_level":"not_protected","maximum_timeout":3600,
				"projects":[{"id":1},{"id":2}],"groups":[{"id":5}],
				"contacted_at":"2026-01-15T10:00:00Z",
				"maintenance_note":"test note"
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := Get(context.Background(), client, GetInput{RunnerID: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 10 {
		t.Errorf("ID = %d, want 10", out.ID)
	}
	if out.Name != "mr-10" {
		t.Errorf("Name = %q, want %q", out.Name, "mr-10")
	}
	if out.Description != "my-runner" {
		t.Errorf("Description = %q, want %q", out.Description, "my-runner")
	}
	if out.RunnerType != "project_type" {
		t.Errorf("RunnerType = %q, want %q", out.RunnerType, "project_type")
	}
	if len(out.TagList) != 2 || out.TagList[0] != "docker" {
		t.Errorf("tags mismatch: %v", out.TagList)
	}
	if out.ProjectCount != 2 || out.GroupCount != 1 {
		t.Errorf("project/group count mismatch: proj=%d group=%d", out.ProjectCount, out.GroupCount)
	}
	if out.MaintenanceNote != "test note" {
		t.Errorf("maintenance_note mismatch: %s", out.MaintenanceNote)
	}
	if !out.RunUntagged {
		t.Error("RunUntagged should be true")
	}
	if out.Locked {
		t.Error("Locked should be false")
	}
	if out.AccessLevel != "not_protected" {
		t.Errorf("AccessLevel = %q, want %q", out.AccessLevel, "not_protected")
	}
	if out.MaximumTimeout != 3600 {
		t.Errorf("MaximumTimeout = %d, want 3600", out.MaximumTimeout)
	}
}

// TestGet_MissingID verifies Get when missing ID.
func TestGet_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := Get(context.Background(), client, GetInput{})
	if err == nil {
		t.Fatal(errExpMissingRunnerID)
	}
}

// TestGet_APIError verifies Get when API error.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	_, err := Get(context.Background(), client, GetInput{RunnerID: 999})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------.

// TestUpdate_Success verifies Update when success.
func TestUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathRunner10 && r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":10,"description":"updated","name":"r-10","paused":true,"is_shared":false,
				"runner_type":"project_type","online":true,"status":"online",
				"tag_list":["docker"],"run_untagged":false,"locked":true,
				"access_level":"ref_protected","maximum_timeout":7200
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	paused := true
	out, err := Update(context.Background(), client, UpdateInput{
		RunnerID:    10,
		Description: "updated",
		Paused:      &paused,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Description != "updated" {
		t.Errorf("Description = %q, want %q", out.Description, "updated")
	}
	if !out.Paused {
		t.Error("Paused should be true")
	}
	if out.RunnerType != "project_type" {
		t.Errorf("RunnerType = %q, want %q", out.RunnerType, "project_type")
	}
	if out.AccessLevel != "ref_protected" {
		t.Errorf("AccessLevel = %q, want %q", out.AccessLevel, "ref_protected")
	}
}

// TestUpdate_MissingID verifies Update when missing ID.
func TestUpdate_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := Update(context.Background(), client, UpdateInput{})
	if err == nil {
		t.Fatal(errExpMissingRunnerID)
	}
}

// ---------------------------------------------------------------------------
// Remove
// ---------------------------------------------------------------------------.

// TestRemove_Success verifies Remove when success.
func TestRemove_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathRunner10 && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	err := Remove(context.Background(), client, RemoveInput{RunnerID: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestRemove_MissingID verifies Remove when missing ID.
func TestRemove_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	err := Remove(context.Background(), client, RemoveInput{})
	if err == nil {
		t.Fatal(errExpMissingRunnerID)
	}
}

// ---------------------------------------------------------------------------
// ListJobs
// ---------------------------------------------------------------------------.

// TestListJobs_Success verifies ListJobs when success.
func TestListJobs_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathRunner10+"/jobs" && r.Method == http.MethodGet {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"id":100,"name":"build","status":"success","ref":"main","stage":"build","pipeline":{"id":50},"web_url":"https://example.com/jobs/100"},
				{"id":101,"name":"test","status":"running","ref":"main","stage":"test","pipeline":{"id":50},"web_url":"https://example.com/jobs/101"}
			]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "2", TotalPages: "1"})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := ListJobs(context.Background(), client, ListJobsInput{RunnerID: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(out.Jobs))
	}
	if out.Jobs[0].ID != 100 {
		t.Errorf("Jobs[0].ID = %d, want 100", out.Jobs[0].ID)
	}
	if out.Jobs[0].Status != "success" {
		t.Errorf("Jobs[0].Status = %q, want %q", out.Jobs[0].Status, "success")
	}
	if out.Jobs[0].Name != "build" {
		t.Errorf("Jobs[0].Name = %q, want %q", out.Jobs[0].Name, "build")
	}
	if out.Jobs[0].Stage != "build" {
		t.Errorf("Jobs[0].Stage = %q, want %q", out.Jobs[0].Stage, "build")
	}
	if out.Jobs[0].Ref != "main" {
		t.Errorf("Jobs[0].Ref = %q, want %q", out.Jobs[0].Ref, "main")
	}
	if out.Jobs[1].Status != "running" {
		t.Errorf("Jobs[1].Status = %q, want %q", out.Jobs[1].Status, "running")
	}
}

// TestListJobs_MissingID verifies ListJobs when missing ID.
func TestListJobs_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := ListJobs(context.Background(), client, ListJobsInput{})
	if err == nil {
		t.Fatal(errExpMissingRunnerID)
	}
}

// ---------------------------------------------------------------------------
// ListProject
// ---------------------------------------------------------------------------.

// TestListProject_Success verifies ListProject when success.
func TestListProject_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/runners" && r.Method == http.MethodGet {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"id":5,"description":"proj-runner","name":"pr-5","paused":false,"is_shared":false,"runner_type":"project_type","online":true,"status":"online"}
			]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := ListProject(context.Background(), client, ListProjectInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Runners) != 1 {
		t.Fatalf("expected 1 runner, got %d", len(out.Runners))
	}
	if out.Runners[0].ID != 5 {
		t.Errorf("Runners[0].ID = %d, want 5", out.Runners[0].ID)
	}
	if out.Runners[0].Description != "proj-runner" {
		t.Errorf("Runners[0].Description = %q, want %q", out.Runners[0].Description, "proj-runner")
	}
	if out.Runners[0].Status != "online" {
		t.Errorf("Runners[0].Status = %q, want %q", out.Runners[0].Status, "online")
	}
	if out.Runners[0].RunnerType != "project_type" {
		t.Errorf("Runners[0].RunnerType = %q, want %q", out.Runners[0].RunnerType, "project_type")
	}
}

// TestListProject_MissingID verifies ListProject when missing ID.
func TestListProject_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := ListProject(context.Background(), client, ListProjectInput{})
	if err == nil {
		t.Fatal(errExpMissingProjectID)
	}
}

// ---------------------------------------------------------------------------
// EnableProject
// ---------------------------------------------------------------------------.

// TestEnableProject_Success verifies EnableProject when success.
func TestEnableProject_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/runners" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":5,"description":"enabled","name":"pr-5","paused":false,"is_shared":false,"runner_type":"project_type","online":true,"status":"online"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := EnableProject(context.Background(), client, EnableProjectInput{ProjectID: "42", RunnerID: 5})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 5 {
		t.Errorf("ID = %d, want 5", out.ID)
	}
	if out.Description != "enabled" {
		t.Errorf("Description = %q, want %q", out.Description, "enabled")
	}
	if out.Status != "online" {
		t.Errorf("Status = %q, want %q", out.Status, "online")
	}
	if out.RunnerType != "project_type" {
		t.Errorf("RunnerType = %q, want %q", out.RunnerType, "project_type")
	}
}

// TestEnableProject_MissingFields verifies EnableProject when missing fields.
func TestEnableProject_MissingFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := EnableProject(context.Background(), client, EnableProjectInput{})
	if err == nil {
		t.Fatal(errExpMissingProjectID)
	}

	_, err = EnableProject(context.Background(), client, EnableProjectInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpMissingRunnerID)
	}
}

// ---------------------------------------------------------------------------
// DisableProject
// ---------------------------------------------------------------------------.

// TestDisableProject_Success verifies DisableProject when success.
func TestDisableProject_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/runners/5" && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	err := DisableProject(context.Background(), client, DisableProjectInput{ProjectID: "42", RunnerID: 5})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDisableProject_MissingFields verifies DisableProject when missing fields.
func TestDisableProject_MissingFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	err := DisableProject(context.Background(), client, DisableProjectInput{})
	if err == nil {
		t.Fatal(errExpMissingProjectID)
	}

	err = DisableProject(context.Background(), client, DisableProjectInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpMissingRunnerID)
	}
}

// ---------------------------------------------------------------------------
// ListGroup
// ---------------------------------------------------------------------------.

// TestListGroup_Success verifies ListGroup when success.
func TestListGroup_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/7/runners" && r.Method == http.MethodGet {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"id":3,"description":"group-runner","name":"gr-3","paused":false,"is_shared":true,"runner_type":"group_type","online":true,"status":"online"}
			]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := ListGroup(context.Background(), client, ListGroupInput{GroupID: "7"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Runners) != 1 {
		t.Fatalf("expected 1 runner, got %d", len(out.Runners))
	}
	if out.Runners[0].ID != 3 {
		t.Errorf("Runners[0].ID = %d, want 3", out.Runners[0].ID)
	}
	if out.Runners[0].Description != "group-runner" {
		t.Errorf("Runners[0].Description = %q, want %q", out.Runners[0].Description, "group-runner")
	}
	if out.Runners[0].RunnerType != "group_type" {
		t.Errorf("Runners[0].RunnerType = %q, want %q", out.Runners[0].RunnerType, "group_type")
	}
	if out.Runners[0].Status != "online" {
		t.Errorf("Runners[0].Status = %q, want %q", out.Runners[0].Status, "online")
	}
}

// TestListGroup_MissingID verifies ListGroup when missing ID.
func TestListGroup_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := ListGroup(context.Background(), client, ListGroupInput{})
	if err == nil {
		t.Fatal("expected error for missing group_id")
	}
}

// ---------------------------------------------------------------------------
// Register
// ---------------------------------------------------------------------------.

// TestRegister_Success verifies Register when success.
func TestRegister_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathRunners && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":99,"description":"new-runner","name":"nr-99","paused":false,"is_shared":false,"runner_type":"project_type","online":false,"status":"never_contacted"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := Register(context.Background(), client, RegisterInput{Token: "reg-token-123"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 99 {
		t.Errorf("ID = %d, want 99", out.ID)
	}
	if out.Status != "never_contacted" {
		t.Errorf("Status = %q, want %q", out.Status, "never_contacted")
	}
	if out.Description != "new-runner" {
		t.Errorf("Description = %q, want %q", out.Description, "new-runner")
	}
	if out.RunnerType != "project_type" {
		t.Errorf("RunnerType = %q, want %q", out.RunnerType, "project_type")
	}
}

// TestRegister_MissingToken verifies Register when missing token.
func TestRegister_MissingToken(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := Register(context.Background(), client, RegisterInput{})
	if err == nil {
		t.Fatal(errExpMissingToken)
	}
}

// ---------------------------------------------------------------------------
// DeleteByID
// ---------------------------------------------------------------------------.

// TestDeleteByID_Success verifies DeleteByID when success.
func TestDeleteByID_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/runners/99" && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	err := DeleteByID(context.Background(), client, DeleteByIDInput{RunnerID: 99})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeleteByID_MissingID verifies DeleteByID when missing ID.
func TestDeleteByID_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	err := DeleteByID(context.Background(), client, DeleteByIDInput{})
	if err == nil {
		t.Fatal(errExpMissingRunnerID)
	}
}

// ---------------------------------------------------------------------------
// Verify
// ---------------------------------------------------------------------------.

// TestVerify_Success verifies Verify when success.
func TestVerify_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/runners/verify" && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusOK)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	err := Verify(context.Background(), client, VerifyInput{Token: "valid-token"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestVerify_MissingToken verifies Verify when missing token.
func TestVerify_MissingToken(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	err := Verify(context.Background(), client, VerifyInput{})
	if err == nil {
		t.Fatal(errExpMissingToken)
	}
}

// TestVerify_InvalidToken verifies Verify when invalid token.
func TestVerify_InvalidToken(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	err := Verify(context.Background(), client, VerifyInput{Token: "bad-token"})
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

// ---------------------------------------------------------------------------
// ResetAuthToken
// ---------------------------------------------------------------------------.

// TestResetAuthToken_Success verifies ResetAuthToken when success.
func TestResetAuthToken_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathRunner10+"/reset_authentication_token" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusOK, `{"token":"new-token-abc","token_expires_at":"2026-01-01T00:00:00Z"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := ResetAuthToken(context.Background(), client, ResetAuthTokenInput{RunnerID: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Token != "new-token-abc" {
		t.Errorf("expected token new-token-abc, got %s", out.Token)
	}
	if out.ExpiresAt == "" {
		t.Error("expected non-empty expires_at")
	}
}

// TestResetAuthToken_MissingID verifies ResetAuthToken when missing ID.
func TestResetAuthToken_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := ResetAuthToken(context.Background(), client, ResetAuthTokenInput{})
	if err == nil {
		t.Fatal(errExpMissingRunnerID)
	}
}

// ---------------------------------------------------------------------------
// ListAll
// ---------------------------------------------------------------------------.

// TestListAll_Success verifies ListAll when success.
func TestListAll_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/runners/all" && r.Method == http.MethodGet {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"id":10,"description":"shared-1","name":"s1","paused":false,"is_shared":true,"runner_type":"instance_type","online":true,"status":"online"},
				{"id":20,"description":"project-1","name":"p1","paused":true,"is_shared":false,"runner_type":"project_type","online":false,"status":"offline"}
			]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "2", TotalPages: "1"})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := ListAll(context.Background(), client, ListAllInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Runners) != 2 {
		t.Fatalf("expected 2 runners, got %d", len(out.Runners))
	}
	if out.Runners[0].ID != 10 {
		t.Errorf("expected runner ID 10, got %d", out.Runners[0].ID)
	}
}

// TestListAll_APIError verifies ListAll when API error.
func TestListAll_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	_, err := ListAll(context.Background(), client, ListAllInput{})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// ---------------------------------------------------------------------------
// DeleteByToken
// ---------------------------------------------------------------------------.

// TestDeleteByToken_Success verifies DeleteByToken when success.
func TestDeleteByToken_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathRunners && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	err := DeleteByToken(context.Background(), client, DeleteByTokenInput{Token: "valid-token-123"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeleteByToken_MissingToken verifies DeleteByToken when missing token.
func TestDeleteByToken_MissingToken(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	err := DeleteByToken(context.Background(), client, DeleteByTokenInput{})
	if err == nil {
		t.Fatal(errExpMissingToken)
	}
}

// TestDeleteByToken_APIError verifies DeleteByToken when API error.
func TestDeleteByToken_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	err := DeleteByToken(context.Background(), client, DeleteByTokenInput{Token: "bad-token"})
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

// ---------------------------------------------------------------------------
// ResetInstanceRegToken
// ---------------------------------------------------------------------------.

// TestResetInstanceRegToken_Success verifies ResetInstanceRegToken when success.
func TestResetInstanceRegToken_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/runners/reset_registration_token" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, `{"token":"reg-token-inst","token_expires_at":"2026-06-01T00:00:00Z"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := ResetInstanceRegToken(context.Background(), client, ResetInstanceRegTokenInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Token != "reg-token-inst" {
		t.Errorf("expected token reg-token-inst, got %s", out.Token)
	}
	if out.ExpiresAt == "" {
		t.Error("expected non-empty expires_at")
	}
}

// TestResetInstanceRegToken_APIError verifies ResetInstanceRegToken when API error.
func TestResetInstanceRegToken_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	_, err := ResetInstanceRegToken(context.Background(), client, ResetInstanceRegTokenInput{})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// ---------------------------------------------------------------------------
// ResetGroupRegToken
// ---------------------------------------------------------------------------.

// TestResetGroupRegToken_Success verifies ResetGroupRegToken when success.
func TestResetGroupRegToken_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/42/runners/reset_registration_token" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, `{"token":"reg-token-grp","token_expires_at":"2026-06-01T00:00:00Z"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := ResetGroupRegToken(context.Background(), client, ResetGroupRegTokenInput{GroupID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Token != "reg-token-grp" {
		t.Errorf("expected token reg-token-grp, got %s", out.Token)
	}
}

// TestResetGroupRegToken_MissingGroupID verifies ResetGroupRegToken when missing group ID.
func TestResetGroupRegToken_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := ResetGroupRegToken(context.Background(), client, ResetGroupRegTokenInput{})
	if err == nil {
		t.Fatal("expected error for missing group_id")
	}
}

// TestResetGroupRegToken_APIError verifies ResetGroupRegToken when API error.
func TestResetGroupRegToken_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	_, err := ResetGroupRegToken(context.Background(), client, ResetGroupRegTokenInput{GroupID: "42"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// ---------------------------------------------------------------------------
// ResetProjectRegToken
// ---------------------------------------------------------------------------.

// TestResetProjectRegToken_Success verifies ResetProjectRegToken when success.
func TestResetProjectRegToken_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/99/runners/reset_registration_token" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, `{"token":"reg-token-proj","token_expires_at":"2026-06-01T00:00:00Z"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := ResetProjectRegToken(context.Background(), client, ResetProjectRegTokenInput{ProjectID: "99"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Token != "reg-token-proj" {
		t.Errorf("expected token reg-token-proj, got %s", out.Token)
	}
}

// TestResetProjectRegToken_MissingProjectID verifies ResetProjectRegToken when missing project ID.
func TestResetProjectRegToken_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := ResetProjectRegToken(context.Background(), client, ResetProjectRegTokenInput{})
	if err == nil {
		t.Fatal(errExpMissingProjectID)
	}
}

// TestResetProjectRegToken_APIError verifies ResetProjectRegToken when API error.
func TestResetProjectRegToken_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	_, err := ResetProjectRegToken(context.Background(), client, ResetProjectRegTokenInput{ProjectID: "99"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestRunnerStatusSpecificAPIErrors covers status-specific hints not exercised by generic API-error tests.
func TestRunnerStatusSpecificAPIErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		call   func(context.Context, *gitlabclient.Client) error
		want   string
	}{
		{name: "UpdateForbidden", status: http.StatusForbidden, want: "admin token", call: func(ctx context.Context, client *gitlabclient.Client) error {
			_, err := Update(ctx, client, UpdateInput{RunnerID: 1})
			return err
		}},
		{name: "EnableProjectForbidden", status: http.StatusForbidden, want: "locked", call: func(ctx context.Context, client *gitlabclient.Client) error {
			_, err := EnableProject(ctx, client, EnableProjectInput{ProjectID: "42", RunnerID: 1})
			return err
		}},
		{name: "RegisterForbidden", status: http.StatusForbidden, want: "registration token", call: func(ctx context.Context, client *gitlabclient.Client) error {
			_, err := Register(ctx, client, RegisterInput{Token: "reg-token"})
			return err
		}},
		{name: "ResetGroupNotFound", status: http.StatusNotFound, want: "gitlab_group_get", call: func(ctx context.Context, client *gitlabclient.Client) error {
			_, err := ResetGroupRegToken(ctx, client, ResetGroupRegTokenInput{GroupID: "42"})
			return err
		}},
		{name: "ResetProjectNotFound", status: http.StatusNotFound, want: "gitlab_project_get", call: func(ctx context.Context, client *gitlabclient.Client) error {
			_, err := ResetProjectRegToken(ctx, client, ResetProjectRegTokenInput{ProjectID: "99"})
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, tt.status, `{"message":"runner error"}`)
			}))
			err := tt.call(t.Context(), client)
			if err == nil {
				t.Fatal("expected API error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want hint containing %q", err.Error(), tt.want)
			}
		})
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
// List — canceled context, with optional filter branches (paused, tag_list, pagination)
// ---------------------------------------------------------------------------.

// TestList_CancelledContext verifies List when cancelled context.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestList_WithAllFilters verifies List when with all filters.
func TestList_WithAllFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/runners" && r.Method == http.MethodGet {
			q := r.URL.Query()
			if q.Get("paused") == "" {
				t.Error("expected paused param")
			}
			if q.Get("tag_list") == "" {
				t.Error("expected tag_list param")
			}
			if q.Get("page") != "2" {
				t.Errorf("expected page=2, got %s", q.Get("page"))
			}
			if q.Get("per_page") != "5" {
				t.Errorf("expected per_page=5, got %s", q.Get("per_page"))
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
				testutil.PaginationHeaders{Page: "2", PerPage: "5", Total: "0", TotalPages: "0"})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	paused := true
	_, err := List(context.Background(), client, ListInput{
		Paused:          &paused,
		TagList:         "docker, linux",
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 5},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// ---------------------------------------------------------------------------
// Get — canceled context
// ---------------------------------------------------------------------------.

// TestGet_CancelledContext verifies Get when cancelled context.
func TestGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := Get(ctx, client, GetInput{RunnerID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Update — canceled context, API error, all optional fields
// ---------------------------------------------------------------------------.

// TestUpdate_CancelledContext verifies Update when cancelled context.
func TestUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := Update(ctx, client, UpdateInput{RunnerID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestUpdate_APIError verifies Update when API error.
func TestUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{RunnerID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestUpdate_AllOptionalFields verifies Update when all optional fields.
func TestUpdate_AllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/runners/10" && r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":10,"description":"desc","name":"r-10","paused":true,"is_shared":false,
				"runner_type":"project_type","online":true,"status":"online",
				"tag_list":["docker","linux"],"run_untagged":false,"locked":true,
				"access_level":"ref_protected","maximum_timeout":7200,
				"maintenance_note":"under repair"
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	paused := true
	runUntagged := false
	locked := true
	maxTimeout := int64(7200)
	out, err := Update(context.Background(), client, UpdateInput{
		RunnerID:        10,
		Description:     "desc",
		Paused:          &paused,
		TagList:         []string{"docker", "linux"},
		RunUntagged:     &runUntagged,
		Locked:          &locked,
		AccessLevel:     "ref_protected",
		MaximumTimeout:  &maxTimeout,
		MaintenanceNote: "under repair",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.MaintenanceNote != "under repair" {
		t.Errorf("MaintenanceNote = %q, want %q", out.MaintenanceNote, "under repair")
	}
	if len(out.TagList) != 2 {
		t.Errorf("TagList len = %d, want 2", len(out.TagList))
	}
}

// ---------------------------------------------------------------------------
// Remove — canceled context, API error
// ---------------------------------------------------------------------------.

// TestRemove_CancelledContext verifies Remove when cancelled context.
func TestRemove_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	err := Remove(ctx, client, RemoveInput{RunnerID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestRemove_APIError verifies Remove when API error.
func TestRemove_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	err := Remove(context.Background(), client, RemoveInput{RunnerID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// ListJobs — canceled context, API error, all optional filters
// ---------------------------------------------------------------------------.

// TestListJobs_CancelledContext verifies ListJobs when cancelled context.
func TestListJobs_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListJobs(ctx, client, ListJobsInput{RunnerID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestListJobs_APIError verifies ListJobs when API error.
func TestListJobs_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := ListJobs(context.Background(), client, ListJobsInput{RunnerID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestListJobs_WithAllFilters verifies ListJobs when with all filters.
func TestListJobs_WithAllFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/runners/10/jobs" && r.Method == http.MethodGet {
			q := r.URL.Query()
			if q.Get("status") != "running" {
				t.Errorf("expected status=running, got %s", q.Get("status"))
			}
			if q.Get("order_by") != "id" {
				t.Errorf("expected order_by=id, got %s", q.Get("order_by"))
			}
			if q.Get("sort") != "desc" {
				t.Errorf("expected sort=desc, got %s", q.Get("sort"))
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
				testutil.PaginationHeaders{Page: "3", PerPage: "10", Total: "0", TotalPages: "0"})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	_, err := ListJobs(context.Background(), client, ListJobsInput{
		RunnerID:        10,
		Status:          "running",
		OrderBy:         "id",
		Sort:            "desc",
		PaginationInput: toolutil.PaginationInput{Page: 3, PerPage: 10},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// ---------------------------------------------------------------------------
// ListProject — canceled context, API error, all optional filters
// ---------------------------------------------------------------------------.

// TestListProject_CancelledContext verifies ListProject when cancelled context.
func TestListProject_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListProject(ctx, client, ListProjectInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestListProject_APIError verifies ListProject when API error.
func TestListProject_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := ListProject(context.Background(), client, ListProjectInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestListProject_AllFilters verifies ListProject when all filters.
func TestListProject_AllFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/runners" && r.Method == http.MethodGet {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "5", Total: "0", TotalPages: "0"})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	_, err := ListProject(context.Background(), client, ListProjectInput{
		ProjectID:       "42",
		Type:            "group_type",
		Status:          "online",
		TagList:         "docker, linux",
		PaginationInput: toolutil.PaginationInput{Page: 1, PerPage: 5},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// ---------------------------------------------------------------------------
// EnableProject — canceled context, API error
// ---------------------------------------------------------------------------.

// TestEnableProject_CancelledContext verifies EnableProject when cancelled context.
func TestEnableProject_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := EnableProject(ctx, client, EnableProjectInput{ProjectID: "1", RunnerID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestEnableProject_APIError verifies EnableProject when API error.
func TestEnableProject_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := EnableProject(context.Background(), client, EnableProjectInput{ProjectID: "1", RunnerID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// DisableProject — canceled context, API error
// ---------------------------------------------------------------------------.

// TestDisableProject_CancelledContext verifies DisableProject when cancelled context.
func TestDisableProject_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	err := DisableProject(ctx, client, DisableProjectInput{ProjectID: "1", RunnerID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestDisableProject_APIError verifies DisableProject when API error.
func TestDisableProject_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	err := DisableProject(context.Background(), client, DisableProjectInput{ProjectID: "1", RunnerID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// ListGroup — canceled context, API error, all optional filters
// ---------------------------------------------------------------------------.

// TestListGroup_CancelledContext verifies ListGroup when cancelled context.
func TestListGroup_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListGroup(ctx, client, ListGroupInput{GroupID: "1"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestListGroup_APIError verifies ListGroup when API error.
func TestListGroup_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := ListGroup(context.Background(), client, ListGroupInput{GroupID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestListGroup_AllFilters verifies ListGroup when all filters.
func TestListGroup_AllFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/7/runners" && r.Method == http.MethodGet {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "5", Total: "0", TotalPages: "0"})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	_, err := ListGroup(context.Background(), client, ListGroupInput{
		GroupID:         "7",
		Type:            "instance_type",
		Status:          "offline",
		TagList:         "ci, nightly",
		PaginationInput: toolutil.PaginationInput{Page: 1, PerPage: 5},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// ---------------------------------------------------------------------------
// Register — canceled context, API error, all optional fields
// ---------------------------------------------------------------------------.

// TestRegister_CancelledContext verifies Register when cancelled context.
func TestRegister_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := Register(ctx, client, RegisterInput{Token: "tok"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestRegister_APIError verifies Register when API error.
func TestRegister_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := Register(context.Background(), client, RegisterInput{Token: "tok"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestRegister_AllOptionalFields verifies Register when all optional fields.
func TestRegister_AllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/runners" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id":99,"description":"test","name":"nr-99","paused":true,
				"is_shared":false,"runner_type":"project_type","online":false,"status":"never_contacted"
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	paused := true
	locked := false
	runUntagged := true
	maxTimeout := int64(3600)
	out, err := Register(context.Background(), client, RegisterInput{
		Token:           "reg-token",
		Description:     "test runner",
		Paused:          &paused,
		Locked:          &locked,
		RunUntagged:     &runUntagged,
		TagList:         []string{"docker", "linux"},
		AccessLevel:     "ref_protected",
		MaximumTimeout:  &maxTimeout,
		MaintenanceNote: "new runner",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 99 {
		t.Errorf("ID = %d, want 99", out.ID)
	}
}

// ---------------------------------------------------------------------------
// DeleteByID — canceled context, API error
// ---------------------------------------------------------------------------.

// TestDeleteByID_CancelledContext verifies DeleteByID when cancelled context.
func TestDeleteByID_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	err := DeleteByID(ctx, client, DeleteByIDInput{RunnerID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestDeleteByID_APIError verifies DeleteByID when API error.
func TestDeleteByID_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	err := DeleteByID(context.Background(), client, DeleteByIDInput{RunnerID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// Verify — canceled context
// ---------------------------------------------------------------------------.

// TestVerify_CancelledContext verifies Verify when cancelled context.
func TestVerify_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	err := Verify(ctx, client, VerifyInput{Token: "tok"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// ResetAuthToken — canceled context, API error, nil token/expires
// ---------------------------------------------------------------------------.

// TestResetAuthToken_CancelledContext verifies ResetAuthToken when cancelled context.
func TestResetAuthToken_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := ResetAuthToken(ctx, client, ResetAuthTokenInput{RunnerID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestResetAuthToken_APIError verifies ResetAuthToken when API error.
func TestResetAuthToken_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := ResetAuthToken(context.Background(), client, ResetAuthTokenInput{RunnerID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestResetAuthToken_NilFields verifies ResetAuthToken when nil fields.
func TestResetAuthToken_NilFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/runners/10/reset_authentication_token" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusOK, `{}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := ResetAuthToken(context.Background(), client, ResetAuthTokenInput{RunnerID: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Token != "" {
		t.Errorf("Token = %q, want empty", out.Token)
	}
	if out.ExpiresAt != "" {
		t.Errorf("ExpiresAt = %q, want empty", out.ExpiresAt)
	}
}

// ---------------------------------------------------------------------------
// ListAll — canceled context, all optional filters
// ---------------------------------------------------------------------------.

// TestListAll_CancelledContext verifies ListAll when cancelled context.
func TestListAll_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListAll(ctx, client, ListAllInput{})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestListAll_AllFilters verifies ListAll when all filters.
func TestListAll_AllFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/runners/all" && r.Method == http.MethodGet {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "5", Total: "0", TotalPages: "0"})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	paused := false
	_, err := ListAll(context.Background(), client, ListAllInput{
		Type:            "instance_type",
		Status:          "online",
		Paused:          &paused,
		TagList:         "docker, ci",
		PaginationInput: toolutil.PaginationInput{Page: 1, PerPage: 5},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// ---------------------------------------------------------------------------
// DeleteByToken — canceled context
// ---------------------------------------------------------------------------.

// TestDeleteByToken_CancelledContext verifies DeleteByToken when cancelled context.
func TestDeleteByToken_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	err := DeleteByToken(ctx, client, DeleteByTokenInput{Token: "tok"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// ResetInstanceRegToken — canceled context
// ---------------------------------------------------------------------------.

// TestResetInstanceRegToken_CancelledContext verifies ResetInstanceRegToken when cancelled context.
func TestResetInstanceRegToken_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := ResetInstanceRegToken(ctx, client, ResetInstanceRegTokenInput{})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// ResetGroupRegToken — canceled context
// ---------------------------------------------------------------------------.

// TestResetGroupRegToken_CancelledContext verifies ResetGroupRegToken when cancelled context.
func TestResetGroupRegToken_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := ResetGroupRegToken(ctx, client, ResetGroupRegTokenInput{GroupID: "1"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// ResetProjectRegToken — canceled context
// ---------------------------------------------------------------------------.

// TestResetProjectRegToken_CancelledContext verifies ResetProjectRegToken when cancelled context.
func TestResetProjectRegToken_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := ResetProjectRegToken(ctx, client, ResetProjectRegTokenInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// FormatOutputMarkdown
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdown verifies FormatOutputMarkdown.
func TestFormatOutputMarkdown(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		ID:          5,
		Name:        "my-runner",
		Description: "test runner",
		RunnerType:  "project_type",
		Status:      "online",
		Paused:      false,
		IsShared:    true,
		Online:      true,
	})

	for _, want := range []string{
		"## Runner #5",
		"| Name | my-runner |",
		"| Description | test runner |",
		"| Type | project_type |",
		"| Status | online |",
		"| Paused | ❌ |",
		"| Shared | ✅ |",
		"| Online | ✅ |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// ---------------------------------------------------------------------------
// FormatDetailsMarkdown — all optional fields present and absent
// ---------------------------------------------------------------------------.

// TestFormatDetailsMarkdown_Full verifies FormatDetailsMarkdown when full.
func TestFormatDetailsMarkdown_Full(t *testing.T) {
	md := FormatDetailsMarkdown(DetailsOutput{
		ID:              10,
		Name:            "detail-runner",
		Description:     "detailed",
		RunnerType:      "group_type",
		Status:          "offline",
		Paused:          true,
		IsShared:        false,
		Online:          false,
		Locked:          true,
		AccessLevel:     "ref_protected",
		RunUntagged:     false,
		TagList:         []string{"docker", "linux"},
		MaximumTimeout:  7200,
		MaintenanceNote: "under repair",
		ContactedAt:     "2026-01-15T10:00:00Z",
	})

	for _, want := range []string{
		"## Runner #10 — Details",
		"| Name | detail-runner |",
		"| Description | detailed |",
		"| Type | group_type |",
		"| Status | offline |",
		"| Paused | ✅ |",
		"| Shared | ❌ |",
		"| Online | ❌ |",
		"| Locked | ✅ |",
		"| Access Level | ref_protected |",
		"| Run Untagged | ❌ |",
		"| Tags | docker, linux |",
		"| Max Timeout | 7200s |",
		"| Maintenance Note | under repair |",
		"| Last Contact | 15 Jan 2026 10:00 UTC |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatDetailsMarkdown_Minimal verifies FormatDetailsMarkdown when minimal.
func TestFormatDetailsMarkdown_Minimal(t *testing.T) {
	md := FormatDetailsMarkdown(DetailsOutput{
		ID:   1,
		Name: "min",
	})

	if !strings.Contains(md, "## Runner #1 — Details") {
		t.Errorf("missing header:\n%s", md)
	}
	for _, absent := range []string{
		"| Tags |",
		"| Max Timeout |",
		"| Maintenance Note |",
		"| Last Contact |",
	} {
		if strings.Contains(md, absent) {
			t.Errorf("should not contain %q for minimal output:\n%s", absent, md)
		}
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown — with data and empty
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithData verifies FormatListMarkdown when with data.
func TestFormatListMarkdown_WithData(t *testing.T) {
	md := FormatListMarkdown(ListOutput{
		Runners: []Output{
			{ID: 1, Name: "r1", RunnerType: "instance_type", Status: "online", Paused: false, IsShared: true},
			{ID: 2, Name: "r2", RunnerType: "project_type", Status: "offline", Paused: true, IsShared: false},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	})

	for _, want := range []string{
		"## Runners (2)",
		"| ID |",
		"| --- |",
		"| 1 |",
		"| 2 |",
		"r1",
		"r2",
		"instance_type",
		"project_type",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatListMarkdown_Empty verifies FormatListMarkdown when empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No runners found") {
		t.Errorf("expected empty message:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// ---------------------------------------------------------------------------
// FormatJobListMarkdown — with data and empty
// ---------------------------------------------------------------------------.

// TestFormatJobListMarkdown_WithData verifies FormatJobListMarkdown when with data.
func TestFormatJobListMarkdown_WithData(t *testing.T) {
	md := FormatJobListMarkdown(JobListOutput{
		Jobs: []jobs.Output{
			{ID: 100, Name: "build", Status: "success", Stage: "build", Ref: "main", Duration: 12.5},
			{ID: 101, Name: "test", Status: "running", Stage: "test", Ref: "develop", Duration: 0.0},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	})

	for _, want := range []string{
		"## Runner Jobs (2)",
		"| ID |",
		"| --- |",
		"| 100 |",
		"| 101 |",
		"build",
		"test",
		"success",
		"running",
		"12.5s",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatJobListMarkdown_Empty verifies FormatJobListMarkdown when empty.
func TestFormatJobListMarkdown_Empty(t *testing.T) {
	md := FormatJobListMarkdown(JobListOutput{})
	if !strings.Contains(md, "No jobs found") {
		t.Errorf("expected empty message:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// ---------------------------------------------------------------------------
// FormatAuthTokenMarkdown — with and without ExpiresAt
// ---------------------------------------------------------------------------.

// TestFormatAuthTokenMarkdown_Full verifies FormatAuthTokenMarkdown when full.
func TestFormatAuthTokenMarkdown_Full(t *testing.T) {
	md := FormatAuthTokenMarkdown(AuthTokenOutput{
		Token:     "glrt-abc123",
		ExpiresAt: "2026-12-31T23:59:59Z",
	})

	for _, want := range []string{
		"## Runner Authentication Token",
		"**Token**: glrt-abc123",
		"**Expires At**: 31 Dec 2026 23:59 UTC",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatAuthTokenMarkdown_NoExpiry verifies FormatAuthTokenMarkdown when no expiry.
func TestFormatAuthTokenMarkdown_NoExpiry(t *testing.T) {
	md := FormatAuthTokenMarkdown(AuthTokenOutput{Token: "tok"})
	if strings.Contains(md, "Expires At") {
		t.Errorf("should not contain Expires At when empty:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// FormatRegTokenMarkdown — with and without ExpiresAt
// ---------------------------------------------------------------------------.

// TestFormatRegTokenMarkdown_Full verifies FormatRegTokenMarkdown when full.
func TestFormatRegTokenMarkdown_Full(t *testing.T) {
	md := FormatRegTokenMarkdown(AuthTokenOutput{
		Token:     "reg-tok-123",
		ExpiresAt: "2026-06-01T00:00:00Z",
	})

	for _, want := range []string{
		"## Runner Registration Token",
		"**Token**: reg-tok-123",
		"**Expires At**: 1 Jun 2026 00:00 UTC",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatRegTokenMarkdown_NoExpiry verifies FormatRegTokenMarkdown when no expiry.
func TestFormatRegTokenMarkdown_NoExpiry(t *testing.T) {
	md := FormatRegTokenMarkdown(AuthTokenOutput{Token: "tok"})
	if strings.Contains(md, "Expires At") {
		t.Errorf("should not contain Expires At when empty:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// RegisterMeta — no panic
// ---------------------------------------------------------------------------.

// TestRegisterMeta_NoPanic verifies RegisterMeta when no panic.
func TestRegisterMeta_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	registerRunnerMetaForTest(t, server, client)
}

// TestRegisterMeta_UsesActionSpecs verifies that gitlab_runner meta routes are
// projected from the canonical ActionSpec definitions.
func TestRegisterMeta_UsesActionSpecs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	got := runnerActionSpecRoutes(t, client)
	want, err := toolutil.ActionSpecsToMapWithError(ActionSpecs(client))
	if err != nil {
		t.Fatalf("ActionSpecsToMapWithError() error = %v", err)
	}

	if len(got) != len(want) {
		t.Fatalf("registered runner route count = %d, want %d", len(got), len(want))
	}
	for actionName, wantRoute := range want {
		t.Run(actionName, func(t *testing.T) {
			gotRoute, ok := got[actionName]
			if !ok {
				t.Fatalf("registered meta routes missing %q", actionName)
			}
			if gotRoute.Destructive != wantRoute.Destructive {
				t.Fatalf("destructive = %t, want %t", gotRoute.Destructive, wantRoute.Destructive)
			}
			if !reflect.DeepEqual(gotRoute.ParameterGuidance, wantRoute.ParameterGuidance) {
				t.Fatalf("parameter guidance = %+v, want %+v", gotRoute.ParameterGuidance, wantRoute.ParameterGuidance)
			}
			if !reflect.DeepEqual(gotRoute.InputSchema, wantRoute.InputSchema) {
				t.Fatal("input schema differs from ActionSpec projection")
			}
			if !reflect.DeepEqual(gotRoute.OutputSchema, wantRoute.OutputSchema) {
				t.Fatal("output schema differs from ActionSpec projection")
			}
		})
	}
}

// TestActionSpecs_RunnerProjectGuidance verifies runner/project identifier
// guidance on actions where those IDs are commonly confused.
func TestActionSpecs_RunnerProjectGuidance(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	routes, err := toolutil.ActionSpecsToMapWithError(ActionSpecs(client))
	if err != nil {
		t.Fatalf("ActionSpecsToMapWithError() error = %v", err)
	}

	enableProject := routes["enable_project"].ParameterGuidance
	if enableProject["runner_id"].SemanticRole != "runner_identifier" {
		t.Fatalf("enable_project runner_id guidance = %+v", enableProject["runner_id"])
	}
	if enableProject["project_id"].SemanticRole != "scope_owner_project" {
		t.Fatalf("enable_project project_id guidance = %+v", enableProject["project_id"])
	}
	if routes["remove"].ParameterGuidance["runner_id"].SemanticRole != "runner_identifier" {
		t.Fatalf("remove runner_id guidance = %+v", routes["remove"].ParameterGuidance["runner_id"])
	}
}

// runnerActionSpecRoutes supports runner action spec routes assertions in runners tests.
func runnerActionSpecRoutes(t *testing.T, client *gitlabclient.Client) toolutil.ActionMap {
	t.Helper()
	routes, err := toolutil.ActionSpecsToMapWithError(ActionSpecs(client))
	if err != nil {
		t.Fatalf("ActionSpecsToMapWithError() error = %v", err)
	}
	return routes
}

// registerRunnerMetaForTest supports register runner meta for test assertions in runners tests.
func registerRunnerMetaForTest(t *testing.T, server *mcp.Server, client *gitlabclient.Client) {
	t.Helper()
	toolutil.AddMetaTool(server, "gitlab_runner", "Manage GitLab CI/CD runners.", runnerActionSpecRoutes(t, client), toolutil.IconRunner, nil)
}

// TestListManagers_Success verifies that ListManagers returns runner
// managers when the API responds successfully.
func TestListManagers_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/runners/1/managers" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK, `[
			{"id":10,"system_id":"sys-01","version":"16.0","platform":"linux","architecture":"amd64","ip_address":"10.0.0.1","status":"online"},
			{"id":11,"system_id":"sys-02","version":"16.0","platform":"darwin","architecture":"arm64","ip_address":"10.0.0.2","status":"offline"}
		]`)
	}))
	out, err := ListManagers(context.Background(), client, ListManagersInput{RunnerID: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Managers) != 2 {
		t.Fatalf("got %d managers, want 2", len(out.Managers))
	}
	if out.Managers[0].SystemID != "sys-01" {
		t.Errorf("SystemID = %q, want %q", out.Managers[0].SystemID, "sys-01")
	}
}

// TestListManagers_ZeroRunnerID verifies validation of zero runner ID.
func TestListManagers_ZeroRunnerID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("API should not be called")
	}))
	_, err := ListManagers(context.Background(), client, ListManagersInput{RunnerID: 0})
	if err == nil {
		t.Fatal("expected error for zero runner_id")
	}
}

// TestListManagers_CancelledContext verifies cancellation before the API request.
func TestListManagers_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("API should not be called")
	}))
	_, err := ListManagers(testutil.CancelledCtx(t), client, ListManagersInput{RunnerID: 1})
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

// TestListManagers_APIError verifies that ListManagers wraps API errors.
func TestListManagers_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Runner Not Found"}`)
	}))
	_, err := ListManagers(context.Background(), client, ListManagersInput{RunnerID: 999})
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

// TestFormatManagerListMarkdown_WithManagers verifies markdown output for
// a non-empty list of runner managers.
func TestFormatManagerListMarkdown_WithManagers(t *testing.T) {
	out := ManagerListOutput{
		Managers: []ManagerOutput{
			{ID: 10, SystemID: "sys-01", Version: "16.0", Platform: "linux", Architecture: "amd64", Status: "online", IPAddress: "10.0.0.1"},
		},
	}
	md := FormatManagerListMarkdown(out)
	if !strings.Contains(md, "sys-01") {
		t.Error("expected sys-01 in markdown output")
	}
	if !strings.Contains(md, "Runner Managers") {
		t.Error("expected header in markdown output")
	}
}

// TestFormatManagerListMarkdown_Empty verifies markdown output for
// an empty managers list.
func TestFormatManagerListMarkdown_Empty(t *testing.T) {
	md := FormatManagerListMarkdown(ManagerListOutput{})
	if !strings.Contains(md, "No runner managers found") {
		t.Error("expected empty message")
	}
}

// TestToManagerOutput_Timestamps verifies timestamp fields are formatted when present.
func TestToManagerOutput_Timestamps(t *testing.T) {
	createdAt := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	contactedAt := time.Date(2026, 1, 15, 11, 30, 0, 0, time.UTC)
	out := toManagerOutput(&gl.RunnerManager{
		ID:          10,
		CreatedAt:   &createdAt,
		ContactedAt: &contactedAt,
	})
	if out.CreatedAt != "2026-01-15T10:00:00Z" {
		t.Fatalf("CreatedAt = %q, want RFC3339 timestamp", out.CreatedAt)
	}
	if out.ContactedAt != "2026-01-15T11:30:00Z" {
		t.Fatalf("ContactedAt = %q, want RFC3339 timestamp", out.ContactedAt)
	}
}
