// deployments_test.go contains unit tests for the deployment MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package deployments

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// errExpCancelledCtx identifies the err exp cancelled ctx constant used by this package.
const errExpCancelledCtx = "expected error for canceled context"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// deploymentList tests
// ---------------------------------------------------------------------------.

// TestDeploymentList_Success verifies DeploymentList when success.
func TestDeploymentList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/deployments" && r.Method == http.MethodGet {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"id":1,"iid":1,"ref":"main","sha":"abc123","status":"success","user":{"username":"admin"},"environment":{"name":"production"},"created_at":"2026-01-01T00:00:00Z"},
				{"id":2,"iid":2,"ref":"develop","sha":"def456","status":"running","user":{"username":"dev"},"environment":{"name":"staging"},"created_at":"2026-01-02T00:00:00Z"}
			]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "2", TotalPages: "1"})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Deployments) != 2 {
		t.Fatalf("expected 2 deployments, got %d", len(out.Deployments))
	}
	if out.Deployments[0].Status != "success" || out.Deployments[0].UserName != "admin" {
		t.Errorf("first deployment mismatch: %+v", out.Deployments[0])
	}
	if out.Deployments[1].EnvironmentName != "staging" {
		t.Errorf("second deployment env mismatch: %+v", out.Deployments[1])
	}
}

// TestDeploymentList_WithFilters verifies DeploymentList when with filters.
func TestDeploymentList_WithFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/deployments" {
			if r.URL.Query().Get("environment") != "production" {
				t.Errorf("expected environment=production, got %s", r.URL.Query().Get("environment"))
			}
			if r.URL.Query().Get("status") != "success" {
				t.Errorf("expected status=success, got %s", r.URL.Query().Get("status"))
			}
			if r.URL.Query().Get("order_by") != "created_at" {
				t.Errorf("expected order_by=created_at, got %s", r.URL.Query().Get("order_by"))
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	_, err := List(context.Background(), client, ListInput{
		ProjectID:   "42",
		Environment: "production",
		Status:      "success",
		OrderBy:     "created_at",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeploymentList_MissingProjectID verifies DeploymentList when missing project ID.
func TestDeploymentList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestDeploymentList_CancelledContext verifies DeploymentList when cancelled context.
func TestDeploymentList_CancelledContext(t *testing.T) {
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
// deploymentGet tests
// ---------------------------------------------------------------------------.

// TestDeploymentGet_WithPipelineWebURL verifies that toOutput populates
// PipelineWebURL when the deployable.pipeline.web_url field is non-empty.
func TestDeploymentGet_WithPipelineWebURL(t *testing.T) {
	const pipelineURL = "https://gitlab.example.com/my-org/project/-/pipelines/123"

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/deployments/1" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":1,"iid":1,"ref":"main","sha":"abc123","status":"success",
				"user":{"username":"admin"},
				"environment":{"name":"production"},
				"created_at":"2026-01-01T00:00:00Z",
				"deployable":{"pipeline":{"web_url":"`+pipelineURL+`"}}
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", DeploymentID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.PipelineWebURL != pipelineURL {
		t.Errorf("PipelineWebURL = %q, want %q", out.PipelineWebURL, pipelineURL)
	}
}

// TestDeploymentGet_Success verifies DeploymentGet when success.
func TestDeploymentGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/deployments/1" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"iid":1,"ref":"main","sha":"abc123","status":"success","user":{"username":"admin"},"environment":{"name":"production"},"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T01:00:00Z"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", DeploymentID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 1 || out.Ref != "main" || out.SHA != "abc123" || out.Status != "success" {
		t.Errorf("unexpected output: %+v", out)
	}
}

// TestDeploymentGet_NilDeployable verifies DeploymentGet when deployable is absent.
func TestDeploymentGet_NilDeployable(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/deployments/1" && r.Method == http.MethodGet {
			// deployable field is absent — zero-value DeploymentDeployable has empty Pipeline
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"iid":1,"ref":"main","sha":"abc123","status":"success","user":{"username":"admin"},"environment":{"name":"production"},"created_at":"2026-01-01T00:00:00Z"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", DeploymentID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.PipelineWebURL != "" {
		t.Errorf("PipelineWebURL = %q, want empty string", out.PipelineWebURL)
	}
}

// TestDeploymentGet_NilPipeline verifies DeploymentGet when deployable exists but pipeline is absent.
func TestDeploymentGet_NilPipeline(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/deployments/1" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"iid":1,"ref":"main","sha":"abc123","status":"success","user":{"username":"admin"},"environment":{"name":"production"},"created_at":"2026-01-01T00:00:00Z","deployable":{"id":10,"status":"success","stage":"deploy"}}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", DeploymentID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.PipelineWebURL != "" {
		t.Errorf("PipelineWebURL = %q, want empty string", out.PipelineWebURL)
	}
}

// TestDeploymentGet_EmptyWebURL verifies DeploymentGet when pipeline exists but web_url is empty.
func TestDeploymentGet_EmptyWebURL(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/deployments/1" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"iid":1,"ref":"main","sha":"abc123","status":"success","user":{"username":"admin"},"environment":{"name":"production"},"created_at":"2026-01-01T00:00:00Z","deployable":{"pipeline":{"web_url":""}}}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", DeploymentID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.PipelineWebURL != "" {
		t.Errorf("PipelineWebURL = %q, want empty string", out.PipelineWebURL)
	}
}

// TestDeploymentGet_ZeroID verifies DeploymentGet when zero ID.
func TestDeploymentGet_ZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := Get(context.Background(), client, GetInput{ProjectID: "42", DeploymentID: 0})
	if err == nil {
		t.Fatal("expected error for zero deployment_id")
	}
}

// TestDeploymentGet_CancelledContext verifies DeploymentGet when cancelled context.
func TestDeploymentGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Get(ctx, client, GetInput{ProjectID: "42", DeploymentID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// deploymentCreate tests
// ---------------------------------------------------------------------------.

// TestDeploymentCreate_Success verifies DeploymentCreate when success.
func TestDeploymentCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/deployments" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":3,"iid":3,"ref":"main","sha":"abc123","status":"created","environment":{"name":"staging"},"created_at":"2026-06-01T00:00:00Z"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:   "42",
		Environment: "staging",
		Ref:         "main",
		SHA:         "abc123",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 3 || out.Status != "created" || out.EnvironmentName != "staging" {
		t.Errorf("unexpected output: %+v", out)
	}
}

// TestDeploymentCreate_MissingFields covers DeploymentCreate with table-driven subtests for missing fields.
func TestDeploymentCreate_MissingFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	tests := []struct {
		name  string
		input CreateInput
	}{
		{"missing project_id", CreateInput{Environment: "e", Ref: "r", SHA: "s"}},
		{"missing environment", CreateInput{ProjectID: "42", Ref: "r", SHA: "s"}},
		{"missing ref", CreateInput{ProjectID: "42", Environment: "e", SHA: "s"}},
		{"missing sha", CreateInput{ProjectID: "42", Environment: "e", Ref: "r"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Create(context.Background(), client, tt.input)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

// TestDeploymentCreate_CancelledContext verifies DeploymentCreate when cancelled context.
func TestDeploymentCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Create(ctx, client, CreateInput{ProjectID: "42", Environment: "e", Ref: "r", SHA: "s"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// deploymentUpdate tests
// ---------------------------------------------------------------------------.

// TestDeploymentUpdate_Success verifies DeploymentUpdate when success.
func TestDeploymentUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/deployments/1" && r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"iid":1,"ref":"main","sha":"abc123","status":"success"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID:    "42",
		DeploymentID: 1,
		Status:       "success",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Status != "success" {
		t.Errorf("expected status 'success', got %q", out.Status)
	}
}

// TestDeploymentUpdate_ZeroID verifies DeploymentUpdate when zero ID.
func TestDeploymentUpdate_ZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "42", DeploymentID: 0, Status: "success"})
	if err == nil {
		t.Fatal("expected error for zero deployment_id")
	}
}

// TestDeploymentUpdate_MissingStatus verifies DeploymentUpdate when missing status.
func TestDeploymentUpdate_MissingStatus(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "42", DeploymentID: 1, Status: ""})
	if err == nil {
		t.Fatal("expected error for missing status")
	}
}

// TestDeploymentUpdate_CancelledContext verifies DeploymentUpdate when cancelled context.
func TestDeploymentUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Update(ctx, client, UpdateInput{ProjectID: "42", DeploymentID: 1, Status: "success"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// deploymentDelete tests
// ---------------------------------------------------------------------------.

// TestDeploymentDelete_Success verifies DeploymentDelete when success.
func TestDeploymentDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/deployments/1" && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	err := Delete(context.Background(), client, DeleteInput{ProjectID: "42", DeploymentID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeploymentDelete_ZeroID verifies DeploymentDelete when zero ID.
func TestDeploymentDelete_ZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	err := Delete(context.Background(), client, DeleteInput{ProjectID: "42", DeploymentID: 0})
	if err == nil {
		t.Fatal("expected error for zero deployment_id")
	}
}

// TestDeploymentDelete_CancelledContext verifies DeploymentDelete when cancelled context.
func TestDeploymentDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	ctx := testutil.CancelledCtx(t)

	err := Delete(ctx, client, DeleteInput{ProjectID: "42", DeploymentID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// Approve or Reject Tests.

// TestDeploymentApprove_Success verifies DeploymentApprove when success.
func TestDeploymentApprove_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/deployments/10/approval" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ApproveOrReject(context.Background(), client, ApproveOrRejectInput{
		ProjectID:    "42",
		DeploymentID: 10,
		Status:       "approved",
		Comment:      "LGTM",
	})
	if err != nil {
		t.Fatalf("ApproveOrReject() unexpected error: %v", err)
	}
	if out.Message == "" {
		t.Error("expected non-empty message")
	}
}

// TestDeploymentReject_Success verifies DeploymentReject when success.
func TestDeploymentReject_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/deployments/10/approval" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ApproveOrReject(context.Background(), client, ApproveOrRejectInput{
		ProjectID:    "42",
		DeploymentID: 10,
		Status:       "rejected",
	})
	if err != nil {
		t.Fatalf("ApproveOrReject() unexpected error: %v", err)
	}
	if out.Message == "" {
		t.Error("expected non-empty message")
	}
}

// TestDeploymentApproveOrReject_MissingProjectID verifies DeploymentApproveOrReject when missing project ID.
func TestDeploymentApproveOrReject_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := ApproveOrReject(context.Background(), client, ApproveOrRejectInput{
		DeploymentID: 10,
		Status:       "approved",
	})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestDeploymentApprove_OrRejectZeroDeploymentID verifies DeploymentApprove when or reject zero deployment ID.
func TestDeploymentApprove_OrRejectZeroDeploymentID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := ApproveOrReject(context.Background(), client, ApproveOrRejectInput{
		ProjectID: "42",
		Status:    "approved",
	})
	if err == nil {
		t.Fatal("expected error for zero deployment_id")
	}
}

// TestDeploymentApproveOrReject_InvalidStatus verifies DeploymentApproveOrReject when invalid status.
func TestDeploymentApproveOrReject_InvalidStatus(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := ApproveOrReject(context.Background(), client, ApproveOrRejectInput{
		ProjectID:    "42",
		DeploymentID: 10,
		Status:       "invalid",
	})
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
}

// TestDeploymentApproveOrReject_APIError verifies DeploymentApproveOrReject when API error.
func TestDeploymentApproveOrReject_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := ApproveOrReject(context.Background(), client, ApproveOrRejectInput{
		ProjectID:    "42",
		DeploymentID: 10,
		Status:       "approved",
	})
	if err == nil {
		t.Fatal("expected error for API error")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpectedAPI identifies the err expected API constant used by this package.
const errExpectedAPI = "expected API error, got nil"

// ---------------------------------------------------------------------------
// List — API error, missing project_id (via empty StringOrInt)
// ---------------------------------------------------------------------------.

// TestDeploymentList_APIError verifies DeploymentList when API error.
func TestDeploymentList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// Get — API error, missing project_id
// ---------------------------------------------------------------------------.

// TestDeploymentGet_APIError verifies DeploymentGet when API error.
func TestDeploymentGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: "1", DeploymentID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDeploymentGet_MissingProjectID verifies DeploymentGet when missing project ID.
func TestDeploymentGet_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Get(context.Background(), client, GetInput{DeploymentID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// ---------------------------------------------------------------------------
// Create — API error, with optional fields (Tag + Status)
// ---------------------------------------------------------------------------.

// TestDeploymentCreate_APIError verifies DeploymentCreate when API error.
func TestDeploymentCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{
		ProjectID: "1", Environment: "staging", Ref: "main", SHA: "abc123",
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDeploymentCreate_StatusErrorBranches verifies status-specific create errors.
func TestDeploymentCreate_StatusErrorBranches(t *testing.T) {
	testCases := []struct {
		name       string
		statusCode int
		wantText   string
	}{
		{name: "bad request", statusCode: http.StatusBadRequest, wantText: "verify environment exists"},
		{name: "generic", statusCode: http.StatusUnprocessableEntity, wantText: opCreateDeployment},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, testCase.statusCode, `{"message":"failed"}`)
			}))
			_, err := Create(context.Background(), client, CreateInput{ProjectID: "1", Environment: "staging", Ref: "main", SHA: "abc123"})
			if err == nil {
				t.Fatal(errExpectedAPI)
			}
			if !strings.Contains(err.Error(), testCase.wantText) {
				t.Fatalf("error = %v, want %q", err, testCase.wantText)
			}
		})
	}
}

// TestDeploymentCreate_WithOptionalFields verifies DeploymentCreate when with optional fields.
func TestDeploymentCreate_WithOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/deployments" {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id":5,"iid":5,"ref":"v1.0.0","sha":"aaa111","status":"running",
				"user":{"username":"deployer"},
				"environment":{"name":"production"},
				"created_at":"2026-06-01T00:00:00Z"
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))

	tag := true
	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:   "42",
		Environment: "production",
		Ref:         "v1.0.0",
		SHA:         "aaa111",
		Tag:         &tag,
		Status:      "running",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ID != 5 {
		t.Errorf("ID = %d, want 5", out.ID)
	}
	if out.Status != "running" {
		t.Errorf("Status = %q, want %q", out.Status, "running")
	}
	if out.UserName != "deployer" {
		t.Errorf("UserName = %q, want %q", out.UserName, "deployer")
	}
	if out.EnvironmentName != "production" {
		t.Errorf("EnvironmentName = %q, want %q", out.EnvironmentName, "production")
	}
}

// ---------------------------------------------------------------------------
// Update — API error, missing project_id
// ---------------------------------------------------------------------------.

// TestDeploymentUpdate_APIError verifies DeploymentUpdate when API error.
func TestDeploymentUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "1", DeploymentID: 1, Status: "success"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDeploymentUpdate_BadRequest verifies invalid status transition hints.
func TestDeploymentUpdate_BadRequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad status"}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "1", DeploymentID: 1, Status: "blocked"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
	if !strings.Contains(err.Error(), "transitions out of terminal states") {
		t.Fatalf("error = %v, want transition hint", err)
	}
}

// TestDeploymentUpdate_MissingProjectID verifies DeploymentUpdate when missing project ID.
func TestDeploymentUpdate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Update(context.Background(), client, UpdateInput{DeploymentID: 1, Status: "success"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// ---------------------------------------------------------------------------
// Delete — API error, missing project_id
// ---------------------------------------------------------------------------.

// TestDeploymentDelete_APIError verifies DeploymentDelete when API error.
func TestDeploymentDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: "1", DeploymentID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDeploymentDelete_NotFound verifies non-forbidden delete errors use the not-found hint path.
func TestDeploymentDelete_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: "1", DeploymentID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
	if !strings.Contains(err.Error(), "gitlab_deployment_list") {
		t.Fatalf("error = %v, want list hint", err)
	}
}

// TestDeploymentDelete_MissingProjectID verifies DeploymentDelete when missing project ID.
func TestDeploymentDelete_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	err := Delete(context.Background(), client, DeleteInput{DeploymentID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// ---------------------------------------------------------------------------
// ApproveOrReject — canceled context
// ---------------------------------------------------------------------------.

// TestDeploymentApproveOrReject_CancelledContext verifies DeploymentApproveOrReject when cancelled context.
func TestDeploymentApproveOrReject_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)

	_, err := ApproveOrReject(ctx, client, ApproveOrRejectInput{
		ProjectID: "42", DeploymentID: 10, Status: "approved",
	})
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

// ---------------------------------------------------------------------------
// FormatOutputMarkdown
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdown_AllFields verifies FormatOutputMarkdown when all fields.
func TestFormatOutputMarkdown_AllFields(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		ID:              1,
		IID:             10,
		Ref:             "main",
		SHA:             "abc123",
		Status:          "success",
		UserName:        "admin",
		EnvironmentName: "production",
		CreatedAt:       "2026-06-01T00:00:00Z",
		UpdatedAt:       "2026-06-01T01:00:00Z",
	})

	for _, want := range []string{
		"## Deployment #1",
		"| IID | 10 |",
		"| Ref | main |",
		"| SHA | abc123 |",
		"| Status | success |",
		"| User | admin |",
		"| Environment | production |",
		"| Created | 1 Jun 2026 00:00 UTC |",
		"| Updated | 1 Jun 2026 01:00 UTC |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatOutputMarkdown_ZeroID verifies FormatOutputMarkdown when zero ID.
func TestFormatOutputMarkdown_ZeroID(t *testing.T) {
	md := FormatOutputMarkdown(Output{})
	if md != "" {
		t.Errorf("expected empty string for zero ID, got %q", md)
	}
}

// TestFormatOutputMarkdown_MinimalFields verifies FormatOutputMarkdown when minimal fields.
func TestFormatOutputMarkdown_MinimalFields(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		ID:     2,
		IID:    2,
		Ref:    "develop",
		SHA:    "def456",
		Status: "running",
	})

	if !strings.Contains(md, "## Deployment #2") {
		t.Errorf("missing header:\n%s", md)
	}
	for _, absent := range []string{
		"| User |",
		"| Environment |",
		"| Created |",
		"| Updated |",
	} {
		if strings.Contains(md, absent) {
			t.Errorf("should not contain %q for minimal output:\n%s", absent, md)
		}
	}
}

// TestFormatOutputMarkdown_WithPipelineWebURL verifies that pipeline_web_url renders a link.
func TestFormatOutputMarkdown_WithPipelineWebURL(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		ID:             5,
		IID:            5,
		Ref:            "main",
		SHA:            "abc123",
		Status:         "success",
		PipelineWebURL: "https://gitlab.example.com/my-org/project/-/pipelines/123",
	})

	if !strings.Contains(md, "| Pipeline |") {
		t.Errorf("markdown missing Pipeline row:\n%s", md)
	}
	if !strings.Contains(md, "https://gitlab.example.com/my-org/project/-/pipelines/123") {
		t.Errorf("markdown missing pipeline URL:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithDeployments verifies FormatListMarkdown when with deployments.
func TestFormatListMarkdown_WithDeployments(t *testing.T) {
	out := ListOutput{
		Deployments: []Output{
			{ID: 1, IID: 1, Ref: "main", SHA: "abc", Status: "success", EnvironmentName: "production", UserName: "admin"},
			{ID: 2, IID: 2, Ref: "develop", SHA: "def", Status: "running", EnvironmentName: "staging", UserName: "dev"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatListMarkdown(out)

	for _, want := range []string{
		"## Deployments (2)",
		"| ID |",
		"| --- |",
		"| 1 |",
		"| 2 |",
		"main",
		"develop",
		"success",
		"running",
		"production",
		"staging",
		"admin",
		"dev",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatListMarkdown_Empty verifies FormatListMarkdown when empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No deployments found") {
		t.Errorf("expected empty message:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// TestFormatDeploymentNotFound verifies the deployment not-found Markdown adapter.
func TestFormatDeploymentNotFound(t *testing.T) {
	result := formatDeploymentNotFound(deploymentNotFoundOutput{Identifier: "17"})
	if result == nil || !result.IsError {
		t.Fatalf("formatDeploymentNotFound() = %#v, want error result", result)
	}
	if len(result.Content) == 0 {
		t.Fatal("formatDeploymentNotFound() returned no content")
	}
}

// ---------------------------------------------------------------------------
// FormatApproveOrRejectMarkdown
// ---------------------------------------------------------------------------.

// TestFormatApproveOrRejectMarkdown_Approved verifies FormatApproveOrRejectMarkdown when approved.
func TestFormatApproveOrRejectMarkdown_Approved(t *testing.T) {
	md := FormatApproveOrRejectMarkdown(ApproveOrRejectOutput{
		Message: "Deployment #10 approved successfully",
	})
	if !strings.Contains(md, "Deployment #10 approved successfully") {
		t.Errorf("markdown missing approval message:\n%s", md)
	}
	if !strings.Contains(md, "✅") {
		t.Errorf("markdown missing checkmark:\n%s", md)
	}
}

// TestFormatApproveOrRejectMarkdown_Rejected verifies FormatApproveOrRejectMarkdown when rejected.
func TestFormatApproveOrRejectMarkdown_Rejected(t *testing.T) {
	md := FormatApproveOrRejectMarkdown(ApproveOrRejectOutput{
		Message: "Deployment #10 rejected successfully",
	})
	if !strings.Contains(md, "Deployment #10 rejected successfully") {
		t.Errorf("markdown missing rejection message:\n%s", md)
	}
}

// TestFormatApproveOrRejectMarkdown_EmptyMessage verifies FormatApproveOrRejectMarkdown when empty message.
func TestFormatApproveOrRejectMarkdown_EmptyMessage(t *testing.T) {
	md := FormatApproveOrRejectMarkdown(ApproveOrRejectOutput{})
	if md == "" {
		t.Error("expected non-empty markdown even for empty message")
	}
}

// ---------------------------------------------------------------------------
// toOutput — all optional fields
// ---------------------------------------------------------------------------.

// TestToOutput_AllOptionalFields verifies ToOutput when all optional fields.
func TestToOutput_AllOptionalFields(t *testing.T) {
	out := FormatOutputMarkdown(Output{
		ID:              100,
		IID:             50,
		Ref:             "v2.0.0",
		SHA:             "deadbeef",
		Status:          "failed",
		UserName:        "deployer",
		EnvironmentName: "canary",
		CreatedAt:       "2026-12-01T00:00:00Z",
		UpdatedAt:       "2026-12-01T12:00:00Z",
	})

	for _, want := range []string{
		"## Deployment #100",
		"| IID | 50 |",
		"| Ref | v2.0.0 |",
		"| SHA | deadbeef |",
		"| Status | failed |",
		"| User | deployer |",
		"| Environment | canary |",
		"| Created | 1 Dec 2026 00:00 UTC |",
		"| Updated | 1 Dec 2026 12:00 UTC |",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown missing %q:\n%s", want, out)
		}
	}
}

// TestList_WithSortField verifies the Sort option is passed to the API.
func TestList_WithSortField(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("sort") != "desc" {
			t.Errorf("expected sort=desc, got %q", r.URL.Query().Get("sort"))
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"iid":1,"ref":"main","sha":"abc","status":"success","user":{"username":"admin"},"environment":{"name":"prod"},"created_at":"2026-01-01T00:00:00Z"}]`)
	}))
	out, err := List(context.Background(), client, ListInput{ProjectID: "42", Sort: "desc"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Deployments) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(out.Deployments))
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs metadata
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata verifies canonical metadata for deployment actions.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	specs := ActionSpecs(client)
	byTool := deploymentSpecsByTool(t, specs)

	if len(specs) != 6 {
		t.Fatalf("len(ActionSpecs) = %d, want 6", len(specs))
	}
	if len(byTool) != len(specs) {
		t.Fatalf("unique individual tools = %d, want %d", len(byTool), len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "deployments" {
			t.Fatalf("OwnerPackage for %s = %q, want deployments", spec.Name, spec.OwnerPackage)
		}
	}

	list := byTool["gitlab_deployment_list"]
	if list.Usage == "" || len(list.Aliases) == 0 {
		t.Fatalf("gitlab_deployment_list metadata incomplete: usage=%q aliases=%d", list.Usage, len(list.Aliases))
	}

	get := byTool["gitlab_deployment_get"]
	if get.Usage == "" || len(get.Aliases) == 0 || get.ParameterGuidance["deployment_id"].SemanticRole == "" {
		t.Fatalf("gitlab_deployment_get metadata incomplete: usage=%q aliases=%d guidance(deployment_id)=%q", get.Usage, len(get.Aliases), get.ParameterGuidance["deployment_id"].SemanticRole)
	}

	approveReject := byTool["gitlab_deployment_approve_or_reject"]
	if approveReject.Usage == "" || len(approveReject.Aliases) == 0 {
		t.Fatalf("gitlab_deployment_approve_or_reject metadata incomplete: usage=%q aliases=%d", approveReject.Usage, len(approveReject.Aliases))
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs route coverage for all 6 tools
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallAllRoutes validates deployment routes across multiple scenarios.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := newDeploymentSpecsByTool(t)

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_deployment_list", map[string]any{"project_id": "1"}},
		{"get", "gitlab_deployment_get", map[string]any{"project_id": "1", "deployment_id": 1}},
		{"create", "gitlab_deployment_create", map[string]any{"project_id": "1", "environment": "staging", "ref": "main", "sha": "abc123"}},
		{"update", "gitlab_deployment_update", map[string]any{"project_id": "1", "deployment_id": 1, "status": "success"}},
		{"delete", "gitlab_deployment_delete", map[string]any{"project_id": "1", "deployment_id": 1}},
		{"approve_or_reject", "gitlab_deployment_approve_or_reject", map[string]any{"project_id": "1", "deployment_id": 1, "status": "approved"}},
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

// newDeploymentSpecsByTool constructs deployment specs by tool test fixtures.
func newDeploymentSpecsByTool(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()

	deploymentJSON := `{"id":1,"iid":1,"ref":"main","sha":"abc123","status":"success","user":{"username":"admin"},"environment":{"name":"production"},"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T01:00:00Z"}`

	handler := http.NewServeMux()

	// List deployments
	handler.HandleFunc("GET /api/v4/projects/1/deployments", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+deploymentJSON+`]`)
	})

	// Get deployment
	handler.HandleFunc("GET /api/v4/projects/1/deployments/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, deploymentJSON)
	})

	// Create deployment
	handler.HandleFunc("POST /api/v4/projects/1/deployments", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, deploymentJSON)
	})

	// Update deployment
	handler.HandleFunc("PUT /api/v4/projects/1/deployments/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, deploymentJSON)
	})

	// Delete deployment
	handler.HandleFunc("DELETE /api/v4/projects/1/deployments/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Approve or reject deployment
	handler.HandleFunc("POST /api/v4/projects/1/deployments/1/approval", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	client := testutil.NewTestClient(t, handler)
	return deploymentSpecsByTool(t, ActionSpecs(client))
}

// TestActionSpecs_DeploymentGetRoute verifies the canonical deployment get route output.
func TestActionSpecs_DeploymentGetRoute(t *testing.T) {
	const respJSON = `{"id":17,"iid":1,"ref":"main","sha":"abc","status":"success","environment":{"name":"prod"}}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/projects/42/deployments/17") {
			testutil.RespondJSON(w, http.StatusOK, respJSON)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)
	byTool := deploymentSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_deployment_get"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "deployment_id": 17})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	out, ok := result.(Output)
	if !ok {
		t.Fatalf("result type = %T, want Output", result)
	}
	if out.ID != 17 || out.EnvironmentName != "prod" {
		t.Fatalf("deployment output = %#v, want ID 17 environment prod", out)
	}
}

// deploymentSpecsByTool supports deployment specs by tool assertions in deployments tests.
func deploymentSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}
