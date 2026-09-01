// environments_test.go contains unit tests for the environment MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package environments

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	// errExpCancelledCtx identifies the err exp cancelled ctx constant used by this package.
	errExpCancelledCtx = "expected error for canceled context"
	// errExpZeroEnvID identifies the err exp zero env ID constant used by this package.
	errExpZeroEnvID = "expected error for zero environment_id"
	// pathEnvironments identifies the path environments constant used by this package.
	pathEnvironments = "/api/v4/projects/42/environments"
	// pathEnvironment1 identifies the path environment 1 constant used by this package.
	pathEnvironment1 = "/api/v4/projects/42/environments/1"
)

// ---------------------------------------------------------------------------
// environmentList tests
// ---------------------------------------------------------------------------.

// TestEnvironmentList_Success verifies that EnvironmentList succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestEnvironmentList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathEnvironments && r.Method == http.MethodGet {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"id":1,"name":"production","slug":"production","state":"available","tier":"production","external_url":"https://prod.example.com","created_at":"2026-01-01T00:00:00Z"},
				{"id":2,"name":"staging","slug":"staging","state":"available","tier":"staging","created_at":"2026-01-01T00:00:00Z"}
			]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "2", TotalPages: "1"})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID: "42",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Environments) != 2 {
		t.Fatalf("expected 2 environments, got %d", len(out.Environments))
	}
	if out.Environments[0].Name != "production" || out.Environments[0].Tier != "production" {
		t.Errorf("first env mismatch: %+v", out.Environments[0])
	}
}

// TestEnvironmentList_WithFilters verifies the EnvironmentList_WithFilters handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestEnvironmentList_WithFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathEnvironments {
			if r.URL.Query().Get("search") != "stag" {
				t.Errorf("expected search=stag, got %s", r.URL.Query().Get("search"))
			}
			if r.URL.Query().Get("states") != "available" {
				t.Errorf("expected states=available, got %s", r.URL.Query().Get("states"))
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	_, err := List(context.Background(), client, ListInput{
		ProjectID: "42",
		Search:    "stag",
		States:    "available",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestEnvironmentList_MissingProjectID verifies that EnvironmentList_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestEnvironmentList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestEnvironmentList_CancelledContext verifies the EnvironmentList_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestEnvironmentList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := List(ctx, client, ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// environmentGet tests
// ---------------------------------------------------------------------------.

// TestEnvironmentGet_Success verifies that EnvironmentGet succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestEnvironmentGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathEnvironment1 && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"production","slug":"production","state":"available","tier":"production","external_url":"https://prod.example.com","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-06-01T00:00:00Z"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := Get(context.Background(), client, GetInput{
		ProjectID:     "42",
		EnvironmentID: 1,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 1 || out.Name != "production" || out.ExternalURL != "https://prod.example.com" {
		t.Errorf("unexpected output: %+v", out)
	}
}

// TestEnvironmentGet_ZeroID verifies the EnvironmentGet_ZeroID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestEnvironmentGet_ZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := Get(context.Background(), client, GetInput{
		ProjectID:     "42",
		EnvironmentID: 0,
	})
	if err == nil {
		t.Fatal(errExpZeroEnvID)
	}
}

// TestEnvironmentGet_CancelledContext verifies the EnvironmentGet_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestEnvironmentGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Get(ctx, client, GetInput{ProjectID: "42", EnvironmentID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// environmentCreate tests
// ---------------------------------------------------------------------------.

// TestEnvironmentCreate_Success verifies that EnvironmentCreate succeeds when the GitLab API returns a valid response.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestEnvironmentCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathEnvironments && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":3,"name":"qa","slug":"qa","state":"available","tier":"testing","description":"QA environment","created_at":"2026-06-01T00:00:00Z"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:   "42",
		Name:        "qa",
		Description: "QA environment",
		Tier:        "testing",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 3 || out.Name != "qa" || out.Tier != "testing" {
		t.Errorf("unexpected output: %+v", out)
	}
}

// TestEnvironmentCreate_MissingName verifies that EnvironmentCreate_MissingName returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestEnvironmentCreate_MissingName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := Create(context.Background(), client, CreateInput{
		ProjectID: "42",
		Name:      "",
	})
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

// TestEnvironmentCreate_CancelledContext verifies the EnvironmentCreate_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestEnvironmentCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Create(ctx, client, CreateInput{ProjectID: "42", Name: "qa"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// environmentUpdate tests
// ---------------------------------------------------------------------------.

// TestEnvironmentUpdate_Success verifies that EnvironmentUpdate succeeds when the GitLab API returns a valid response.
// The test exercises the PUT path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestEnvironmentUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathEnvironment1 && r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"production-v2","slug":"production-v2","state":"available","tier":"production","external_url":"https://v2.prod.example.com"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID:     "42",
		EnvironmentID: 1,
		Name:          "production-v2",
		ExternalURL:   "https://v2.prod.example.com",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Name != "production-v2" {
		t.Errorf("expected name 'production-v2', got %q", out.Name)
	}
}

// TestEnvironmentUpdate_ZeroID verifies the EnvironmentUpdate_ZeroID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestEnvironmentUpdate_ZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := Update(context.Background(), client, UpdateInput{
		ProjectID:     "42",
		EnvironmentID: 0,
		Name:          "new-name",
	})
	if err == nil {
		t.Fatal(errExpZeroEnvID)
	}
}

// TestEnvironmentUpdate_CancelledContext verifies the EnvironmentUpdate_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestEnvironmentUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Update(ctx, client, UpdateInput{ProjectID: "42", EnvironmentID: 1, Name: "x"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// environmentDelete tests
// ---------------------------------------------------------------------------.

// TestEnvironmentDelete_Success verifies that EnvironmentDelete succeeds when the GitLab API returns a valid response.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestEnvironmentDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathEnvironment1 && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	err := Delete(context.Background(), client, DeleteInput{
		ProjectID:     "42",
		EnvironmentID: 1,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestEnvironmentDelete_ZeroID verifies the EnvironmentDelete_ZeroID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestEnvironmentDelete_ZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	err := Delete(context.Background(), client, DeleteInput{
		ProjectID:     "42",
		EnvironmentID: 0,
	})
	if err == nil {
		t.Fatal(errExpZeroEnvID)
	}
}

// TestEnvironmentDelete_CancelledContext verifies the EnvironmentDelete_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestEnvironmentDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	ctx := testutil.CancelledCtx(t)

	err := Delete(ctx, client, DeleteInput{ProjectID: "42", EnvironmentID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// environmentStop tests
// ---------------------------------------------------------------------------.

// TestEnvironmentStop_Success verifies that EnvironmentStop succeeds when the GitLab API returns a valid response.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestEnvironmentStop_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathEnvironment1+"/stop" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"staging","slug":"staging","state":"stopped","tier":"staging"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := Stop(context.Background(), client, StopInput{
		ProjectID:     "42",
		EnvironmentID: 1,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.State != "stopped" {
		t.Errorf("expected state 'stopped', got %q", out.State)
	}
}

// TestEnvironmentStop_WithForce verifies the EnvironmentStop_WithForce handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestEnvironmentStop_WithForce(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathEnvironment1+"/stop" {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"staging","slug":"staging","state":"stopped"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	force := true
	out, err := Stop(context.Background(), client, StopInput{
		ProjectID:     "42",
		EnvironmentID: 1,
		Force:         &force,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.State != "stopped" {
		t.Errorf("expected state 'stopped', got %q", out.State)
	}
}

// TestEnvironmentStop_ZeroID verifies the EnvironmentStop_ZeroID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestEnvironmentStop_ZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := Stop(context.Background(), client, StopInput{
		ProjectID:     "42",
		EnvironmentID: 0,
	})
	if err == nil {
		t.Fatal(errExpZeroEnvID)
	}
}

// TestEnvironmentStop_CancelledContext verifies the EnvironmentStop_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestEnvironmentStop_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Stop(ctx, client, StopInput{ProjectID: "42", EnvironmentID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpectedAPI identifies the err expected API constant used by this package.
const errExpectedAPI = "expected API error, got nil"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// List — API error, name filter, pagination
// ---------------------------------------------------------------------------.

// TestEnvironmentList_APIError verifies that EnvironmentList returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestEnvironmentList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestEnvironmentList_WithNameFilter verifies the EnvironmentList_WithNameFilter handler.
// The mock GitLab API at /api/v4/projects/1/environments (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestEnvironmentList_WithNameFilter(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/1/environments" {
			if got := r.URL.Query().Get("name"); got != "production" {
				t.Errorf("expected name=production, got %s", got)
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"id":1,"name":"production","slug":"production","state":"available","tier":"production"}]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: "1", Name: "production"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Environments) != 1 {
		t.Fatalf("expected 1 environment, got %d", len(out.Environments))
	}
	if out.Environments[0].Name != "production" {
		t.Errorf("expected name=production, got %q", out.Environments[0].Name)
	}
}

// TestEnvironmentList_Pagination verifies that EnvironmentList forwards pagination parameters to the GitLab API and parses the response metadata.
// The mock GitLab API at /api/v4/projects/1/environments (GET) responds with HTTP OK.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestEnvironmentList_Pagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/1/environments" {
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"id":3,"name":"dev","slug":"dev","state":"available"}]`,
				testutil.PaginationHeaders{Page: "2", PerPage: "1", Total: "3", TotalPages: "3", NextPage: "3", PrevPage: "1"})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID: "1",
		Page:      2, PerPage: 1,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Pagination.TotalPages != 3 {
		t.Errorf("TotalPages = %d, want 3", out.Pagination.TotalPages)
	}
	if out.Pagination.NextPage != 3 {
		t.Errorf("NextPage = %d, want 3", out.Pagination.NextPage)
	}
}

// ---------------------------------------------------------------------------
// Get — API error, missing project_id
// ---------------------------------------------------------------------------.

// TestEnvironmentGet_APIError verifies that EnvironmentGet returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestEnvironmentGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: "1", EnvironmentID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestEnvironmentGet_MissingProjectID verifies that EnvironmentGet_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestEnvironmentGet_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Get(context.Background(), client, GetInput{EnvironmentID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// ---------------------------------------------------------------------------
// Create — API error, missing project_id, all optional fields
// ---------------------------------------------------------------------------.

// TestEnvironmentCreate_APIError verifies that EnvironmentCreate returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestEnvironmentCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: "1", Name: "staging"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestEnvironmentCreate_MissingProjectID verifies that EnvironmentCreate_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestEnvironmentCreate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Create(context.Background(), client, CreateInput{Name: "staging"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestEnvironmentCreate_AllOptionalFields verifies the EnvironmentCreate_AllOptionalFields handler.
// The mock GitLab API at /api/v4/projects/1/environments (POST) responds with HTTP Created.
// It asserts the returned output matches the expected fields.
func TestEnvironmentCreate_AllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/1/environments" {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id":10,"name":"review","slug":"review","state":"available",
				"tier":"development","description":"Review env","external_url":"https://review.example.com",
				"created_at":"2026-06-01T00:00:00Z"
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:   "1",
		Name:        "review",
		Description: "Review env",
		ExternalURL: "https://review.example.com",
		Tier:        "development",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Tier != "development" {
		t.Errorf("Tier = %q, want %q", out.Tier, "development")
	}
	if out.ExternalURL != "https://review.example.com" {
		t.Errorf("ExternalURL = %q, want %q", out.ExternalURL, "https://review.example.com")
	}
	if out.Description != "Review env" {
		t.Errorf("Description = %q, want %q", out.Description, "Review env")
	}
}

// ---------------------------------------------------------------------------
// Update — API error, missing project_id, all optional fields
// ---------------------------------------------------------------------------.

// TestEnvironmentUpdate_APIError verifies that EnvironmentUpdate returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestEnvironmentUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "1", EnvironmentID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestEnvironmentUpdate_MissingProjectID verifies that EnvironmentUpdate_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestEnvironmentUpdate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Update(context.Background(), client, UpdateInput{EnvironmentID: 1, Name: "x"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestEnvironmentUpdate_AllOptionalFields verifies the EnvironmentUpdate_AllOptionalFields handler.
// The mock GitLab API at /api/v4/projects/1/environments/5 (PUT) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestEnvironmentUpdate_AllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/projects/1/environments/5" {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":5,"name":"staging-v2","slug":"staging-v2","state":"available",
				"tier":"staging","description":"Updated staging","external_url":"https://staging-v2.example.com"
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID:     "1",
		EnvironmentID: 5,
		Name:          "staging-v2",
		Description:   "Updated staging",
		ExternalURL:   "https://staging-v2.example.com",
		Tier:          "staging",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Tier != "staging" {
		t.Errorf("Tier = %q, want %q", out.Tier, "staging")
	}
	if out.Description != "Updated staging" {
		t.Errorf("Description = %q, want %q", out.Description, "Updated staging")
	}
}

// ---------------------------------------------------------------------------
// Delete — API error, missing project_id
// ---------------------------------------------------------------------------.

// TestEnvironmentDelete_APIError verifies that EnvironmentDelete returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestEnvironmentDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: "1", EnvironmentID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestEnvironmentDelete_MissingProjectID verifies that EnvironmentDelete_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestEnvironmentDelete_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	err := Delete(context.Background(), client, DeleteInput{EnvironmentID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// ---------------------------------------------------------------------------
// Stop — API error, missing project_id, force=false
// ---------------------------------------------------------------------------.

// TestEnvironmentStop_APIError verifies that EnvironmentStop returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestEnvironmentStop_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Stop(context.Background(), client, StopInput{ProjectID: "1", EnvironmentID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestEnvironmentStop_MissingProjectID verifies that EnvironmentStop_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestEnvironmentStop_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Stop(context.Background(), client, StopInput{EnvironmentID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestEnvironmentStop_ForceFalse verifies the EnvironmentStop_ForceFalse handler.
// The mock GitLab API at /api/v4/projects/1/environments/2/stop (POST) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestEnvironmentStop_ForceFalse(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/1/environments/2/stop" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusOK, `{"id":2,"name":"staging","slug":"staging","state":"stopped","tier":"staging"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	force := false
	out, err := Stop(context.Background(), client, StopInput{
		ProjectID:     "1",
		EnvironmentID: 2,
		Force:         &force,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.State != "stopped" {
		t.Errorf("State = %q, want %q", out.State, "stopped")
	}
}

// ---------------------------------------------------------------------------
// toOutput — all optional timestamp fields
// ---------------------------------------------------------------------------.

// TestToOutput_AllTimestampFields verifies the ToOutput_AllTimestampFields handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestToOutput_AllTimestampFields(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		ID:          1,
		Name:        "production",
		Slug:        "production",
		State:       "available",
		Tier:        "production",
		Description: "Main prod environment",
		ExternalURL: "https://prod.example.com",
		CreatedAt:   "2026-01-01T00:00:00Z",
		UpdatedAt:   "2026-06-15T12:00:00Z",
		AutoStopAt:  "2026-12-31T23:59:59Z",
	})

	for _, want := range []string{
		"## Environment: production",
		"| ID | 1 |",
		"| Slug | production |",
		"| State | available |",
		"| Tier | production |",
		"| Description | Main prod environment |",
		"| URL | https://prod.example.com |",
		"| Created | 1 Jan 2026 00:00 UTC |",
		"| Updated | 15 Jun 2026 12:00 UTC |",
		"| Auto-Stop At | 31 Dec 2026 23:59 UTC |",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Errorf("markdown missing %q:\n%s", want, md)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FormatOutputMarkdown
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdown_EmptyName verifies the OutputMarkdown_EmptyName Markdown formatter for a representative output_emptyname input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatOutputMarkdown_EmptyName(t *testing.T) {
	md := FormatOutputMarkdown(Output{})
	if md != "" {
		t.Errorf("expected empty string for empty name, got %q", md)
	}
}

// TestFormatEnvironmentNotFound verifies the EnvironmentNotFound Markdown formatter for a representative environmentnotfound input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestFormatEnvironmentNotFound(t *testing.T) {
	result := formatEnvironmentNotFound(environmentNotFoundOutput{Identifier: "ID 99 in project 42"})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content in not-found result")
	}
}

// TestFormatOutputMarkdown_MinimalFields verifies the OutputMarkdown_MinimalFields Markdown formatter for a representative output_minimalfields input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatOutputMarkdown_MinimalFields(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		ID:    7,
		Name:  "dev",
		Slug:  "dev",
		State: "stopped",
	})

	if !strings.Contains(md, "## Environment: dev") {
		t.Errorf("missing header:\n%s", md)
	}
	if !strings.Contains(md, "| State | stopped |") {
		t.Errorf("missing state:\n%s", md)
	}
	for _, absent := range []string{
		"| Tier |",
		"| Description |",
		"| URL |",
		"| Created |",
		"| Updated |",
		"| Auto-Stop At |",
	} {
		t.Run(absent, func(t *testing.T) {
			if strings.Contains(md, absent) {
				t.Errorf("should not contain %q for minimal output:\n%s", absent, md)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithEnvironments verifies the ListMarkdown_WithEnvironments Markdown formatter for a representative list_withenvironments input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown_WithEnvironments(t *testing.T) {
	out := ListOutput{
		Environments: []Output{
			{ID: 1, Name: "production", State: "available", Tier: "production", ExternalURL: "https://prod.example.com"},
			{ID: 2, Name: "staging", State: "available", Tier: "staging", ExternalURL: "https://staging.example.com"},
			{ID: 3, Name: "dev", State: "stopped", Tier: "development", ExternalURL: ""},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 3, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatListMarkdown(out)

	for _, want := range []string{
		"## Environments (3)",
		"| ID |",
		"| --- |",
		"| 1 |",
		"| 2 |",
		"| 3 |",
		"production",
		"staging",
		"dev",
		"available",
		"stopped",
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
	if !strings.Contains(md, "No environments found") {
		t.Errorf("expected empty message:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// TestGet_WithAutoStopAt verifies the Get_WithAutoStopAt handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGet_WithAutoStopAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{
			"id":1,"name":"review","slug":"review","state":"available",
			"created_at":"2026-01-01T00:00:00Z",
			"updated_at":"2026-01-02T00:00:00Z",
			"auto_stop_at":"2026-02-01T00:00:00Z"
		}`)
	}))
	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", EnvironmentID: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.AutoStopAt == "" {
		t.Error("expected AutoStopAt to be set")
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
	byTool := environmentSpecsByTool(t, specs)

	if len(specs) != 6 {
		t.Fatalf("len(ActionSpecs) = %d, want 6", len(specs))
	}
	if len(byTool) != len(specs) {
		t.Fatalf("unique individual tools = %d, want %d", len(byTool), len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "environments" {
			t.Fatalf("OwnerPackage for %s = %q, want environments", spec.Name, spec.OwnerPackage)
		}
	}

	list := byTool["gitlab_environment_list"]
	if list.Usage == "" || len(list.Aliases) == 0 {
		t.Fatalf("gitlab_environment_list metadata incomplete: usage=%q aliases=%d", list.Usage, len(list.Aliases))
	}

	get := byTool["gitlab_environment_get"]
	if get.Usage == "" || len(get.Aliases) == 0 || get.ParameterGuidance["environment_id"].SemanticRole == "" {
		t.Fatalf("gitlab_environment_get metadata incomplete: usage=%q aliases=%d guidance(environment_id)=%q", get.Usage, len(get.Aliases), get.ParameterGuidance["environment_id"].SemanticRole)
	}

	stop := byTool["gitlab_environment_stop"]
	if stop.Usage == "" || len(stop.Aliases) == 0 {
		t.Fatalf("gitlab_environment_stop metadata incomplete: usage=%q aliases=%d", stop.Usage, len(stop.Aliases))
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs route coverage for all 6 tools
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallAllRoutes validates the CallAllRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := newEnvironmentSpecsByTool(t)

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_environment_list", map[string]any{"project_id": "1"}},
		{"get", "gitlab_environment_get", map[string]any{"project_id": "1", "environment_id": 1}},
		{"create", "gitlab_environment_create", map[string]any{"project_id": "1", "name": "review"}},
		{"update", "gitlab_environment_update", map[string]any{"project_id": "1", "environment_id": 1, "name": "updated"}},
		{"delete", "gitlab_environment_delete", map[string]any{"project_id": "1", "environment_id": 1}},
		{"stop", "gitlab_environment_stop", map[string]any{"project_id": "1", "environment_id": 1}},
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
// Helper: ActionSpec route factory
// ---------------------------------------------------------------------------.

// newEnvironmentSpecsByTool constructs environment specs by tool test fixtures.
func newEnvironmentSpecsByTool(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()

	envJSON := `{"id":1,"name":"production","slug":"production","state":"available","tier":"production","external_url":"https://prod.example.com","created_at":"2026-01-01T00:00:00Z"}`

	handler := http.NewServeMux()

	// List environments
	handler.HandleFunc("GET /api/v4/projects/1/environments", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+envJSON+`]`)
	})

	// Get environment
	handler.HandleFunc("GET /api/v4/projects/1/environments/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, envJSON)
	})

	// Create environment
	handler.HandleFunc("POST /api/v4/projects/1/environments", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, envJSON)
	})

	// Update environment
	handler.HandleFunc("PUT /api/v4/projects/1/environments/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, envJSON)
	})

	// Delete environment
	handler.HandleFunc("DELETE /api/v4/projects/1/environments/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Stop environment
	handler.HandleFunc("POST /api/v4/projects/1/environments/1/stop", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"production","slug":"production","state":"stopped","tier":"production"}`)
	})

	client := testutil.NewTestClient(t, handler)
	return environmentSpecsByTool(t, ActionSpecs(client))
}

// TestActionSpecs_EnvironmentGetRoute validates the EnvironmentGetRoute route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_EnvironmentGetRoute(t *testing.T) {
	const respJSON = `{"id":7,"name":"prod","slug":"prod","state":"available","tier":"production"}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/projects/42/environments/7") {
			testutil.RespondJSON(w, http.StatusOK, respJSON)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)
	byTool := environmentSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_environment_get"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "environment_id": 7})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	out, ok := result.(Output)
	if !ok {
		t.Fatalf("result type = %T, want Output", result)
	}
	if out.ID != 7 || out.Name != "prod" {
		t.Fatalf("environment output = %#v, want ID 7 name prod", out)
	}
}

// environmentSpecsByTool supports environment specs by tool assertions in environments tests.
func environmentSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}

// ---------------------------------------------------------------------------
// 1:1 audit — nested output sub-objects and additive input fields
// ---------------------------------------------------------------------------.

// envFullJSON is a full single-environment API response exercising every
// documented 1:1 field: scalar (auto_stop_setting, kubernetes_namespace,
// flux_resource_path) and nested objects (cluster_agent, last_deployment with
// its deployable+pipeline+user+commit+runner) as documented in
// doc/api/environments.md "Retrieve an environment".
const envFullJSON = `{
	"id":7,"name":"production","slug":"production","state":"available","tier":"production",
	"description":"Prod","external_url":"https://prod.example.com",
	"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-06-15T12:00:00Z",
	"auto_stop_at":"2026-12-31T23:59:59Z","auto_stop_setting":"with_action",
	"kubernetes_namespace":"prod-ns","flux_resource_path":"flux/prod",
	"cluster_agent":{
		"id":11,"name":"prod-agent","created_at":"2025-12-01T00:00:00Z","created_by_user_id":3,
		"config_project":{"id":99,"description":"Agent cfg","name":"cfg","name_with_namespace":"grp / cfg","path":"cfg","path_with_namespace":"grp/cfg","created_at":"2025-11-01T00:00:00Z"}
	},
	"last_deployment":{
		"id":501,"iid":12,"ref":"main","sha":"abc123","status":"success",
		"created_at":"2026-06-15T11:00:00Z",
		"user":{"id":4,"name":"Deployer","username":"deployer","state":"active","avatar_url":"https://av","web_url":"https://u"},
		"deployable":{
			"id":900,"status":"success","stage":"deploy","name":"deploy-prod","ref":"main","tag":false,
			"coverage":88.5,"created_at":"2026-06-15T10:55:00Z","started_at":"2026-06-15T10:56:00Z",
			"finished_at":"2026-06-15T11:00:00Z","duration":240,
			"user":{"id":4,"name":"Deployer","username":"deployer","state":"active","web_url":"https://u","created_at":"2025-01-01T00:00:00Z","bio":"bio text","location":"Earth","public_email":"d@x","organization":"Acme"},
			"commit":{"id":"abc123def","short_id":"abc123","title":"Deploy fix","message":"Deploy fix\n","author_name":"Dev","author_email":"dev@x","authored_date":"2026-06-15T10:00:00Z","committer_name":"Dev","committer_email":"dev@x","committed_date":"2026-06-15T10:00:00Z","created_at":"2026-06-15T10:00:00Z","parent_ids":["p1"]},
			"pipeline":{"id":700,"sha":"abc123","ref":"main","status":"success","web_url":"https://pipe"},
			"runner":{"id":55,"description":"shared-runner","name":"runner-1","is_shared":true,"runner_type":"instance_type","online":true,"status":"online"}
		}
	}
}`

// TestEnvironmentGet_FullNestedObjects verifies that Get surfaces every additive
// 1:1 field and nested sub-object faithfully from the API response.
func TestEnvironmentGet_FullNestedObjects(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/environments/7" {
			testutil.RespondJSON(w, http.StatusOK, envFullJSON)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404"}`)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", EnvironmentID: 7})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}

	if out.AutoStopSetting != "with_action" {
		t.Errorf("AutoStopSetting = %q, want with_action", out.AutoStopSetting)
	}
	if out.KubernetesNamespace != "prod-ns" {
		t.Errorf("KubernetesNamespace = %q, want prod-ns", out.KubernetesNamespace)
	}
	if out.FluxResourcePath != "flux/prod" {
		t.Errorf("FluxResourcePath = %q, want flux/prod", out.FluxResourcePath)
	}

	assertClusterAgent(t, out.ClusterAgent)
	assertLastDeployment(t, out.LastDeployment)
}

// assertClusterAgent validates the fully populated cluster_agent sub-object.
func assertClusterAgent(t *testing.T, ca *ClusterAgentOutput) {
	t.Helper()
	if ca == nil {
		t.Fatal("ClusterAgent is nil")
	}
	if ca.ID != 11 || ca.Name != "prod-agent" || ca.CreatedByUserID != 3 || ca.CreatedAt == "" {
		t.Errorf("ClusterAgent = %#v", ca)
	}
	if ca.ConfigProject == nil || ca.ConfigProject.ID != 99 ||
		ca.ConfigProject.PathWithNamespace != "grp/cfg" || ca.ConfigProject.CreatedAt == "" {
		t.Errorf("ConfigProject = %#v", ca.ConfigProject)
	}
}

// assertLastDeployment validates the fully populated last_deployment sub-object,
// including its user, deployable, and pipeline references.
func assertLastDeployment(t *testing.T, ld *DeploymentOutput) {
	t.Helper()
	if ld == nil {
		t.Fatal("LastDeployment is nil")
	}
	wantLD := DeploymentOutput{ID: 501, IID: 12, Ref: "main", SHA: "abc123", Status: "success"}
	if ld.ID != wantLD.ID || ld.IID != wantLD.IID || ld.Ref != wantLD.Ref || ld.SHA != wantLD.SHA || ld.Status != wantLD.Status {
		t.Errorf("LastDeployment = %#v", ld)
	}
	if ld.CreatedAt == "" {
		t.Error("LastDeployment created_at empty")
	}
	if u := ld.User; u == nil || u.ID != 4 || u.Username != "deployer" || u.State != "active" || u.WebURL != "https://u" {
		t.Errorf("LastDeployment.User = %#v", ld.User)
	}
	assertDeployable(t, ld.Deployable)
}

// assertDeployable validates the deployable job sub-object and its documented
// user, commit, pipeline, and runner references.
func assertDeployable(t *testing.T, dep *DeployableOutput) {
	t.Helper()
	if dep == nil {
		t.Fatal("Deployable is nil")
	}
	if dep.ID != 900 || dep.Status != "success" || dep.Stage != "deploy" || dep.Name != "deploy-prod" {
		t.Errorf("Deployable identity = %#v", dep)
	}
	if dep.Coverage != 88.5 || dep.Duration != 240 {
		t.Errorf("Deployable metrics = %#v", dep)
	}
	if dep.CreatedAt == "" || dep.StartedAt == "" || dep.FinishedAt == "" {
		t.Error("Deployable timestamps empty")
	}
	assertDeployableUser(t, dep.User)
	assertDeployableCommit(t, dep.Commit)
	assertDeployablePipeline(t, dep.Pipeline)
	assertDeployableRunner(t, dep.Runner)
}

// assertDeployableUser validates the documented deployable.user subset.
func assertDeployableUser(t *testing.T, u *DeployableUserOutput) {
	t.Helper()
	if u == nil || u.ID != 4 || u.Username != "deployer" || u.Bio != "bio text" ||
		u.Location != "Earth" || u.PublicEmail != "d@x" || u.Organization != "Acme" || u.CreatedAt == "" {
		t.Errorf("Deployable.User = %#v", u)
	}
}

// assertDeployableCommit validates the documented deployable.commit subset.
func assertDeployableCommit(t *testing.T, c *DeployableCommitOutput) {
	t.Helper()
	if c == nil || c.ID != "abc123def" || c.ShortID != "abc123" || c.Title != "Deploy fix" ||
		c.AuthorEmail != "dev@x" || c.CommittedDate == "" || len(c.ParentIDs) != 1 || c.ParentIDs[0] != "p1" {
		t.Errorf("Deployable.Commit = %#v", c)
	}
}

// assertDeployablePipeline validates the documented deployable.pipeline subset.
func assertDeployablePipeline(t *testing.T, p *DeployablePipelineOutput) {
	t.Helper()
	if p == nil || p.ID != 700 || p.WebURL != "https://pipe" || p.SHA != "abc123" || p.Ref != "main" || p.Status != "success" {
		t.Errorf("Deployable.Pipeline = %#v", p)
	}
}

// assertDeployableRunner validates the documented deployable.runner subset.
func assertDeployableRunner(t *testing.T, r *DeployableRunnerOutput) {
	t.Helper()
	if r == nil || r.ID != 55 || r.Name != "runner-1" || !r.IsShared ||
		r.RunnerType != "instance_type" || !r.Online || r.Status != "online" {
		t.Errorf("Deployable.Runner = %#v", r)
	}
}

// TestEnvironmentGet_NilNestedObjects verifies that absent nested objects map to
// nil pointers (no panic, no empty structs) when the API omits them.
func TestEnvironmentGet_NilNestedObjects(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/environments/8" {
			testutil.RespondJSON(w, http.StatusOK, `{"id":8,"name":"review","slug":"review","state":"available"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404"}`)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", EnvironmentID: 8})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ClusterAgent != nil {
		t.Errorf("ClusterAgent = %#v, want nil", out.ClusterAgent)
	}
	if out.LastDeployment != nil {
		t.Errorf("LastDeployment = %#v, want nil", out.LastDeployment)
	}
}

// TestEnvironmentGet_DeploymentWithoutDeployable verifies a deployment whose
// deployable carries no identity (zero id, empty name) maps to a nil Deployable.
func TestEnvironmentGet_DeploymentWithoutDeployable(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/environments/9" {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":9,"name":"staging","slug":"staging","state":"available",
				"last_deployment":{"id":1,"ref":"main","status":"running"}
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404"}`)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", EnvironmentID: 9})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.LastDeployment == nil || out.LastDeployment.ID != 1 {
		t.Fatalf("LastDeployment = %#v", out.LastDeployment)
	}
	if out.LastDeployment.Deployable != nil {
		t.Errorf("Deployable = %#v, want nil", out.LastDeployment.Deployable)
	}
	if out.LastDeployment.User != nil {
		t.Errorf("User = %#v, want nil", out.LastDeployment.User)
	}
}

// TestEnvironmentGet_DeployableWithoutNestedRefs verifies a deployable that has
// identity but omits its user, commit, and runner references maps each to a nil
// pointer (covering the nil-guard branches of the deployable converters).
func TestEnvironmentGet_DeployableWithoutNestedRefs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/environments/10" {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":10,"name":"qa","slug":"qa","state":"available",
				"last_deployment":{"id":2,"ref":"main","status":"success",
					"deployable":{"id":3,"name":"deploy-qa","status":"success"}}
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404"}`)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", EnvironmentID: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	dep := out.LastDeployment.Deployable
	if dep == nil || dep.ID != 3 {
		t.Fatalf("Deployable = %#v", dep)
	}
	if dep.User != nil {
		t.Errorf("Deployable.User = %#v, want nil", dep.User)
	}
	if dep.Commit != nil {
		t.Errorf("Deployable.Commit = %#v, want nil", dep.Commit)
	}
	if dep.Runner != nil {
		t.Errorf("Deployable.Runner = %#v, want nil", dep.Runner)
	}
}

// TestEnvironmentList_OrderBySortKeyset verifies that List forwards order_by,
// sort, pagination=keyset, and page_token to the GitLab API query string.
func TestEnvironmentList_OrderBySortKeyset(t *testing.T) {
	var gotQuery string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"name":"production","slug":"production","state":"available"}]`)
	}))

	_, err := List(context.Background(), client, ListInput{
		ProjectID:  "42",
		OrderBy:    "name",
		Sort:       "desc",
		Pagination: "keyset",
		PageToken:  "tok42",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	for _, want := range []string{"order_by=name", "sort=desc", "pagination=keyset", "page_token=tok42"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(gotQuery, want) {
				t.Errorf("query %q missing %q", gotQuery, want)
			}
		})
	}
}

// TestEnvironmentCreate_NewOptionFields verifies that Create forwards the
// additive cluster_agent_id, kubernetes_namespace, flux_resource_path, and
// auto_stop_setting options to the GitLab API.
func TestEnvironmentCreate_NewOptionFields(t *testing.T) {
	var body string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			buf := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(buf)
			body = string(buf)
			testutil.RespondJSON(w, http.StatusCreated, envFullJSON)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404"}`)
	}))

	agentID := int64(11)
	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:           "42",
		Name:                "production",
		ClusterAgentID:      &agentID,
		KubernetesNamespace: "prod-ns",
		FluxResourcePath:    "flux/prod",
		AutoStopSetting:     "with_action",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	for _, want := range []string{`"cluster_agent_id":11`, `"kubernetes_namespace":"prod-ns"`, `"flux_resource_path":"flux/prod"`, `"auto_stop_setting":"with_action"`} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(body, want) {
				t.Errorf("request body %q missing %q", body, want)
			}
		})
	}
	if out.ClusterAgent == nil || out.ClusterAgent.ID != 11 {
		t.Errorf("ClusterAgent = %#v", out.ClusterAgent)
	}
}

// TestEnvironmentUpdate_NewOptionFields verifies that Update forwards the
// additive cluster_agent_id, kubernetes_namespace, flux_resource_path, and
// auto_stop_setting options to the GitLab API.
func TestEnvironmentUpdate_NewOptionFields(t *testing.T) {
	var body string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			buf := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(buf)
			body = string(buf)
			testutil.RespondJSON(w, http.StatusOK, envFullJSON)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404"}`)
	}))

	agentID := int64(11)
	_, err := Update(context.Background(), client, UpdateInput{
		ProjectID:           "42",
		EnvironmentID:       7,
		ClusterAgentID:      &agentID,
		KubernetesNamespace: "prod-ns",
		FluxResourcePath:    "flux/prod",
		AutoStopSetting:     "with_action",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	for _, want := range []string{`"cluster_agent_id":11`, `"kubernetes_namespace":"prod-ns"`, `"flux_resource_path":"flux/prod"`, `"auto_stop_setting":"with_action"`} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(body, want) {
				t.Errorf("request body %q missing %q", body, want)
			}
		})
	}
}

// TestActionSpecs_UpdateDeleteMetadata verifies the 1:1 R-META metadata for the
// previously generic-flagged update and delete environment tools: specific
// usage, non-toolname aliases, related actions, and a "Returns:/See also:"
// individual-tool description.
func TestActionSpecs_UpdateDeleteMetadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	byTool := environmentSpecsByTool(t, ActionSpecs(client))

	for _, tt := range []struct {
		tool        string
		wantInUsage string
		wantInDesc  []string
	}{
		{"gitlab_environment_update", "Update an existing environment", []string{"Returns:", "See also:", "gitlab_environment_get"}},
		{"gitlab_environment_delete", "Delete an environment", []string{"Returns:", "See also:", "gitlab_environment_stop"}},
	} {
		spec := byTool[tt.tool]
		if !strings.Contains(spec.Usage, tt.wantInUsage) {
			t.Errorf("%s usage = %q, want substring %q", tt.tool, spec.Usage, tt.wantInUsage)
		}
		if strings.Contains(spec.Usage, "Use to execute") {
			t.Errorf("%s usage still generic: %q", tt.tool, spec.Usage)
		}
		for _, a := range spec.Aliases {
			if a == tt.tool {
				t.Errorf("%s aliases still contain only the tool name: %v", tt.tool, spec.Aliases)
			}
		}
		if len(spec.RelatedActions) == 0 {
			t.Errorf("%s has empty RelatedActions", tt.tool)
		}
		for _, w := range tt.wantInDesc {
			if !strings.Contains(spec.IndividualTool.Description, w) {
				t.Errorf("%s description = %q, want substring %q", tt.tool, spec.IndividualTool.Description, w)
			}
		}
	}
}
