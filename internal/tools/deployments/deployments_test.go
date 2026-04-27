// deployments_test.go contains unit tests for the deployment MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.

package deployments

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const errExpCancelledCtx = "expected error for canceled context"

const fmtUnexpErr = "unexpected error: %v"

// deploymentList tests
// ---------------------------------------------------------------------------.

// TestDeploymentList_Success verifies the behavior of deployment list success.
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

// TestDeploymentList_WithFilters verifies the behavior of deployment list with filters.
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

// TestDeploymentList_MissingProjectID verifies the behavior of deployment list missing project i d.
func TestDeploymentList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestDeploymentList_CancelledContext verifies the behavior of deployment list cancelled context.
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

// TestDeploymentGet_Success verifies the behavior of deployment get success.
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

// TestDeploymentGet_ZeroID verifies the behavior of deployment get zero i d.
func TestDeploymentGet_ZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := Get(context.Background(), client, GetInput{ProjectID: "42", DeploymentID: 0})
	if err == nil {
		t.Fatal("expected error for zero deployment_id")
	}
}

// TestDeploymentGet_CancelledContext verifies the behavior of deployment get cancelled context.
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

// TestDeploymentCreate_Success verifies the behavior of deployment create success.
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

// TestDeploymentCreate_MissingFields validates deployment create missing fields across multiple scenarios using table-driven subtests.
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

// TestDeploymentCreate_CancelledContext verifies the behavior of deployment create cancelled context.
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

// TestDeploymentUpdate_Success verifies the behavior of deployment update success.
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

// TestDeploymentUpdate_ZeroID verifies the behavior of deployment update zero i d.
func TestDeploymentUpdate_ZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "42", DeploymentID: 0, Status: "success"})
	if err == nil {
		t.Fatal("expected error for zero deployment_id")
	}
}

// TestDeploymentUpdate_MissingStatus verifies the behavior of deployment update missing status.
func TestDeploymentUpdate_MissingStatus(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "42", DeploymentID: 1, Status: ""})
	if err == nil {
		t.Fatal("expected error for missing status")
	}
}

// TestDeploymentUpdate_CancelledContext verifies the behavior of deployment update cancelled context.
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

// TestDeploymentDelete_Success verifies the behavior of deployment delete success.
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

// TestDeploymentDelete_ZeroID verifies the behavior of deployment delete zero i d.
func TestDeploymentDelete_ZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	err := Delete(context.Background(), client, DeleteInput{ProjectID: "42", DeploymentID: 0})
	if err == nil {
		t.Fatal("expected error for zero deployment_id")
	}
}

// TestDeploymentDelete_CancelledContext verifies the behavior of deployment delete cancelled context.
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

// TestDeploymentApprove_Success verifies the behavior of deployment approve success.
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

// TestDeploymentReject_Success verifies the behavior of deployment reject success.
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

// TestDeploymentApproveOrReject_MissingProjectID verifies the behavior of deployment approve or reject missing project i d.
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

// TestDeploymentApprove_OrRejectZeroDeploymentID verifies the behavior of deployment approve or reject zero deployment i d.
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

// TestDeploymentApproveOrReject_InvalidStatus verifies the behavior of deployment approve or reject invalid status.
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

// TestDeploymentApproveOrReject_APIError verifies the behavior of deployment approve or reject a p i error.
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

const errExpectedAPI = "expected API error, got nil"

// ---------------------------------------------------------------------------
// List — API error, missing project_id (via empty StringOrInt)
// ---------------------------------------------------------------------------.

// TestDeploymentList_APIError verifies the behavior of deployment list a p i error.
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

// TestDeploymentGet_APIError verifies the behavior of deployment get a p i error.
func TestDeploymentGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: "1", DeploymentID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDeploymentGet_MissingProjectID verifies the behavior of deployment get missing project i d.
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

// TestDeploymentCreate_APIError verifies the behavior of deployment create a p i error.
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

// TestDeploymentCreate_WithOptionalFields verifies the behavior of deployment create with optional fields.
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

// TestDeploymentUpdate_APIError verifies the behavior of deployment update a p i error.
func TestDeploymentUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "1", DeploymentID: 1, Status: "success"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDeploymentUpdate_MissingProjectID verifies the behavior of deployment update missing project i d.
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

// TestDeploymentDelete_APIError verifies the behavior of deployment delete a p i error.
func TestDeploymentDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: "1", DeploymentID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDeploymentDelete_MissingProjectID verifies the behavior of deployment delete missing project i d.
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

// TestDeploymentApproveOrReject_CancelledContext verifies the behavior of deployment approve or reject cancelled context.
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

// TestFormatOutputMarkdown_AllFields verifies the behavior of format output markdown all fields.
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

// TestFormatOutputMarkdown_ZeroID verifies the behavior of format output markdown zero i d.
func TestFormatOutputMarkdown_ZeroID(t *testing.T) {
	md := FormatOutputMarkdown(Output{})
	if md != "" {
		t.Errorf("expected empty string for zero ID, got %q", md)
	}
}

// TestFormatOutputMarkdown_MinimalFields verifies the behavior of format output markdown minimal fields.
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

// ---------------------------------------------------------------------------
// FormatListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithDeployments verifies the behavior of format list markdown with deployments.
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

// TestFormatListMarkdown_Empty verifies the behavior of format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No deployments found") {
		t.Errorf("expected empty message:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// ---------------------------------------------------------------------------
// FormatApproveOrRejectMarkdown
// ---------------------------------------------------------------------------.

// TestFormatApproveOrRejectMarkdown_Approved verifies the behavior of format approve or reject markdown approved.
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

// TestFormatApproveOrRejectMarkdown_Rejected verifies the behavior of format approve or reject markdown rejected.
func TestFormatApproveOrRejectMarkdown_Rejected(t *testing.T) {
	md := FormatApproveOrRejectMarkdown(ApproveOrRejectOutput{
		Message: "Deployment #10 rejected successfully",
	})
	if !strings.Contains(md, "Deployment #10 rejected successfully") {
		t.Errorf("markdown missing rejection message:\n%s", md)
	}
}

// TestFormatApproveOrRejectMarkdown_EmptyMessage verifies the behavior of format approve or reject markdown empty message.
func TestFormatApproveOrRejectMarkdown_EmptyMessage(t *testing.T) {
	md := FormatApproveOrRejectMarkdown(ApproveOrRejectOutput{})
	if md == "" {
		t.Error("expected non-empty markdown even for empty message")
	}
}

// ---------------------------------------------------------------------------
// toOutput — all optional fields
// ---------------------------------------------------------------------------.

// TestToOutput_AllOptionalFields verifies the behavior of to output all optional fields.
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
	session := newDeploymentsMCPSession(t)
	ctx := context.Background()

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

// newDeploymentsMCPSession is an internal helper for the deployments package.
func newDeploymentsMCPSession(t *testing.T) *mcp.ClientSession {
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

// TestDeploymentGet_EmbedsCanonicalResource asserts gitlab_deployment_get
// attaches an EmbeddedResource block with URI
// gitlab://project/{id}/deployment/{deployment_id}.
func TestDeploymentGet_EmbedsCanonicalResource(t *testing.T) {
	const respJSON = `{"id":17,"iid":1,"ref":"main","sha":"abc","status":"success","environment":{"name":"prod"}}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/projects/42/deployments/17") {
			testutil.RespondJSON(w, http.StatusOK, respJSON)
			return
		}
		http.NotFound(w, r)
	})
	session, ctx := testutil.NewEmbedTestSession(t, handler, RegisterTools)
	args := map[string]any{"project_id": "42", "deployment_id": 17}
	testutil.AssertEmbeddedResource(t, ctx, session, "gitlab_deployment_get", args, "gitlab://project/42/deployment/17", toolutil.EnableEmbeddedResources)
}
