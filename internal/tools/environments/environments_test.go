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

// TestEnvironmentList_Success verifies EnvironmentList when success.
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

// TestEnvironmentList_WithFilters verifies EnvironmentList when with filters.
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

// TestEnvironmentList_MissingProjectID verifies EnvironmentList when missing project ID.
func TestEnvironmentList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestEnvironmentList_CancelledContext verifies EnvironmentList when cancelled context.
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

// TestEnvironmentGet_Success verifies EnvironmentGet when success.
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

// TestEnvironmentGet_ZeroID verifies EnvironmentGet when zero ID.
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

// TestEnvironmentGet_CancelledContext verifies EnvironmentGet when cancelled context.
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

// TestEnvironmentCreate_Success verifies EnvironmentCreate when success.
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

// TestEnvironmentCreate_MissingName verifies EnvironmentCreate when missing name.
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

// TestEnvironmentCreate_CancelledContext verifies EnvironmentCreate when cancelled context.
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

// TestEnvironmentUpdate_Success verifies EnvironmentUpdate when success.
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

// TestEnvironmentUpdate_ZeroID verifies EnvironmentUpdate when zero ID.
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

// TestEnvironmentUpdate_CancelledContext verifies EnvironmentUpdate when cancelled context.
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

// TestEnvironmentDelete_Success verifies EnvironmentDelete when success.
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

// TestEnvironmentDelete_ZeroID verifies EnvironmentDelete when zero ID.
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

// TestEnvironmentDelete_CancelledContext verifies EnvironmentDelete when cancelled context.
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

// TestEnvironmentStop_Success verifies EnvironmentStop when success.
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

// TestEnvironmentStop_WithForce verifies EnvironmentStop when with force.
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

// TestEnvironmentStop_ZeroID verifies EnvironmentStop when zero ID.
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

// TestEnvironmentStop_CancelledContext verifies EnvironmentStop when cancelled context.
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

// TestEnvironmentList_APIError verifies EnvironmentList when API error.
func TestEnvironmentList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestEnvironmentList_WithNameFilter verifies EnvironmentList when with name filter.
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

// TestEnvironmentList_Pagination verifies EnvironmentList when pagination.
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

// TestEnvironmentGet_APIError verifies EnvironmentGet when API error.
func TestEnvironmentGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: "1", EnvironmentID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestEnvironmentGet_MissingProjectID verifies EnvironmentGet when missing project ID.
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

// TestEnvironmentCreate_APIError verifies EnvironmentCreate when API error.
func TestEnvironmentCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: "1", Name: "staging"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestEnvironmentCreate_MissingProjectID verifies EnvironmentCreate when missing project ID.
func TestEnvironmentCreate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Create(context.Background(), client, CreateInput{Name: "staging"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestEnvironmentCreate_AllOptionalFields verifies EnvironmentCreate when all optional fields.
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

// TestEnvironmentUpdate_APIError verifies EnvironmentUpdate when API error.
func TestEnvironmentUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "1", EnvironmentID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestEnvironmentUpdate_MissingProjectID verifies EnvironmentUpdate when missing project ID.
func TestEnvironmentUpdate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Update(context.Background(), client, UpdateInput{EnvironmentID: 1, Name: "x"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestEnvironmentUpdate_AllOptionalFields verifies EnvironmentUpdate when all optional fields.
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

// TestEnvironmentDelete_APIError verifies EnvironmentDelete when API error.
func TestEnvironmentDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: "1", EnvironmentID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestEnvironmentDelete_MissingProjectID verifies EnvironmentDelete when missing project ID.
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

// TestEnvironmentStop_APIError verifies EnvironmentStop when API error.
func TestEnvironmentStop_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Stop(context.Background(), client, StopInput{ProjectID: "1", EnvironmentID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestEnvironmentStop_MissingProjectID verifies EnvironmentStop when missing project ID.
func TestEnvironmentStop_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Stop(context.Background(), client, StopInput{EnvironmentID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestEnvironmentStop_ForceFalse verifies EnvironmentStop when force false.
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

// TestToOutput_AllTimestampFields verifies ToOutput when all timestamp fields.
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

// TestFormatOutputMarkdown_EmptyName verifies FormatOutputMarkdown when empty name.
func TestFormatOutputMarkdown_EmptyName(t *testing.T) {
	md := FormatOutputMarkdown(Output{})
	if md != "" {
		t.Errorf("expected empty string for empty name, got %q", md)
	}
}

// TestFormatEnvironmentNotFound verifies the special not-found formatter emits content.
func TestFormatEnvironmentNotFound(t *testing.T) {
	result := formatEnvironmentNotFound(environmentNotFoundOutput{Identifier: "ID 99 in project 42"})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content in not-found result")
	}
}

// TestFormatOutputMarkdown_MinimalFields verifies FormatOutputMarkdown when minimal fields.
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

// TestFormatListMarkdown_WithEnvironments verifies FormatListMarkdown when with environments.
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

// TestFormatListMarkdown_Empty verifies FormatListMarkdown when empty.
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
// ActionSpecs metadata
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata verifies canonical metadata for environment actions.
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

// TestActionSpecs_CallAllRoutes validates environment routes across multiple scenarios.
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

// TestActionSpecs_EnvironmentGetRoute verifies the canonical environment get route output.
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
