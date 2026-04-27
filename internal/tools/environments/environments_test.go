// environments_test.go contains unit tests for the environment MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.

package environments

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	errExpCancelledCtx = "expected error for canceled context"
	errExpZeroEnvID    = "expected error for zero environment_id"
	pathEnvironments   = "/api/v4/projects/42/environments"
	pathEnvironment1   = "/api/v4/projects/42/environments/1"
)

// ---------------------------------------------------------------------------
// environmentList tests
// ---------------------------------------------------------------------------.

// TestEnvironmentList_Success verifies the behavior of environment list success.
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

// TestEnvironmentList_WithFilters verifies the behavior of environment list with filters.
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

// TestEnvironmentList_MissingProjectID verifies the behavior of environment list missing project i d.
func TestEnvironmentList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestEnvironmentList_CancelledContext verifies the behavior of environment list cancelled context.
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

// TestEnvironmentGet_Success verifies the behavior of environment get success.
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

// TestEnvironmentGet_ZeroID verifies the behavior of environment get zero i d.
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

// TestEnvironmentGet_CancelledContext verifies the behavior of environment get cancelled context.
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

// TestEnvironmentCreate_Success verifies the behavior of environment create success.
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

// TestEnvironmentCreate_MissingName verifies the behavior of environment create missing name.
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

// TestEnvironmentCreate_CancelledContext verifies the behavior of environment create cancelled context.
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

// TestEnvironmentUpdate_Success verifies the behavior of environment update success.
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

// TestEnvironmentUpdate_ZeroID verifies the behavior of environment update zero i d.
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

// TestEnvironmentUpdate_CancelledContext verifies the behavior of environment update cancelled context.
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

// TestEnvironmentDelete_Success verifies the behavior of environment delete success.
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

// TestEnvironmentDelete_ZeroID verifies the behavior of environment delete zero i d.
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

// TestEnvironmentDelete_CancelledContext verifies the behavior of environment delete cancelled context.
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

// TestEnvironmentStop_Success verifies the behavior of environment stop success.
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

// TestEnvironmentStop_WithForce verifies the behavior of environment stop with force.
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

// TestEnvironmentStop_ZeroID verifies the behavior of environment stop zero i d.
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

// TestEnvironmentStop_CancelledContext verifies the behavior of environment stop cancelled context.
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

const errExpectedAPI = "expected API error, got nil"

const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// List — API error, name filter, pagination
// ---------------------------------------------------------------------------.

// TestEnvironmentList_APIError verifies the behavior of environment list a p i error.
func TestEnvironmentList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestEnvironmentList_WithNameFilter verifies the behavior of environment list with name filter.
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

// TestEnvironmentList_Pagination verifies the behavior of environment list pagination.
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
		ProjectID:       "1",
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 1},
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

// TestEnvironmentGet_APIError verifies the behavior of environment get a p i error.
func TestEnvironmentGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: "1", EnvironmentID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestEnvironmentGet_MissingProjectID verifies the behavior of environment get missing project i d.
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

// TestEnvironmentCreate_APIError verifies the behavior of environment create a p i error.
func TestEnvironmentCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: "1", Name: "staging"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestEnvironmentCreate_MissingProjectID verifies the behavior of environment create missing project i d.
func TestEnvironmentCreate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Create(context.Background(), client, CreateInput{Name: "staging"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestEnvironmentCreate_AllOptionalFields verifies the behavior of environment create all optional fields.
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

// TestEnvironmentUpdate_APIError verifies the behavior of environment update a p i error.
func TestEnvironmentUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "1", EnvironmentID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestEnvironmentUpdate_MissingProjectID verifies the behavior of environment update missing project i d.
func TestEnvironmentUpdate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Update(context.Background(), client, UpdateInput{EnvironmentID: 1, Name: "x"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestEnvironmentUpdate_AllOptionalFields verifies the behavior of environment update all optional fields.
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

// TestEnvironmentDelete_APIError verifies the behavior of environment delete a p i error.
func TestEnvironmentDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: "1", EnvironmentID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestEnvironmentDelete_MissingProjectID verifies the behavior of environment delete missing project i d.
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

// TestEnvironmentStop_APIError verifies the behavior of environment stop a p i error.
func TestEnvironmentStop_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Stop(context.Background(), client, StopInput{ProjectID: "1", EnvironmentID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestEnvironmentStop_MissingProjectID verifies the behavior of environment stop missing project i d.
func TestEnvironmentStop_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Stop(context.Background(), client, StopInput{EnvironmentID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestEnvironmentStop_ForceFalse verifies the behavior of environment stop force false.
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

// TestToOutput_AllTimestampFields verifies the behavior of to output all timestamp fields.
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
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// ---------------------------------------------------------------------------
// FormatOutputMarkdown
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdown_EmptyName verifies the behavior of format output markdown empty name.
func TestFormatOutputMarkdown_EmptyName(t *testing.T) {
	md := FormatOutputMarkdown(Output{})
	if md != "" {
		t.Errorf("expected empty string for empty name, got %q", md)
	}
}

// TestFormatOutputMarkdown_MinimalFields verifies the behavior of format output markdown minimal fields.
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
		if strings.Contains(md, absent) {
			t.Errorf("should not contain %q for minimal output:\n%s", absent, md)
		}
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithEnvironments verifies the behavior of format list markdown with environments.
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
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatListMarkdown_Empty verifies the behavior of format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No environments found") {
		t.Errorf("expected empty message:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// TestGet_WithAutoStopAt verifies toOutput covers the AutoStopAt nil guard.
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
// RegisterToolsCallAllThroughMCP — full MCP roundtrip for all 6 tools
// ---------------------------------------------------------------------------.

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newEnvironmentsMCPSession(t)
	ctx := context.Background()

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

// newEnvironmentsMCPSession is an internal helper for the environments package.
func newEnvironmentsMCPSession(t *testing.T) *mcp.ClientSession {
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

// TestEnvironmentGet_EmbedsCanonicalResource asserts gitlab_environment_get
// attaches an EmbeddedResource block with URI
// gitlab://project/{id}/environment/{env_id}.
func TestEnvironmentGet_EmbedsCanonicalResource(t *testing.T) {
	const respJSON = `{"id":7,"name":"prod","slug":"prod","state":"available","tier":"production"}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/projects/42/environments/7") {
			testutil.RespondJSON(w, http.StatusOK, respJSON)
			return
		}
		http.NotFound(w, r)
	})
	session, ctx := testutil.NewEmbedTestSession(t, handler, RegisterTools)
	args := map[string]any{"project_id": "42", "environment_id": 7}
	testutil.AssertEmbeddedResource(t, ctx, session, "gitlab_environment_get", args, "gitlab://project/42/environment/7", toolutil.EnableEmbeddedResources)
}
