// audit_events_test.go contains unit tests for GitLab audit event listing
// operations. Tests use httptest to mock the GitLab Audit Events API.
package auditevents

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const fmtUnexpErr = "unexpected error: %v"

// TestListInstance_Success verifies that ListInstance succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/audit_events (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestListInstance_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/audit_events" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"id":1,"author_id":10,"entity_id":0,"entity_type":"User","event_name":"user_login","event_type":"auth","details":{"author_name":"admin","ip_address":"127.0.0.1"},"created_at":"2026-01-15T10:00:00Z"}
		]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	}))

	out, err := ListInstance(context.Background(), client, ListInstanceInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.AuditEvents) != 1 {
		t.Fatalf("got %d events, want 1", len(out.AuditEvents))
	}
	if out.AuditEvents[0].ID != 1 {
		t.Errorf("got ID %d, want 1", out.AuditEvents[0].ID)
	}
	if out.AuditEvents[0].EventName != "user_login" {
		t.Errorf("got event_name %q, want %q", out.AuditEvents[0].EventName, "user_login")
	}
	if out.AuditEvents[0].Details.IPAddress != "127.0.0.1" {
		t.Errorf("got ip_address %q, want %q", out.AuditEvents[0].Details.IPAddress, "127.0.0.1")
	}
}

// TestGetInstance_Success verifies that GetInstance succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/audit_events/1 (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestGetInstance_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/audit_events/1" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"author_id":10,"entity_id":0,"entity_type":"User","event_name":"user_login","event_type":"auth","details":{"author_name":"admin"},"created_at":"2026-01-15T10:00:00Z"}`)
	}))

	out, err := GetInstance(context.Background(), client, GetInstanceInput{EventID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 1 {
		t.Errorf("got ID %d, want 1", out.ID)
	}
	if out.Details.AuthorName != "admin" {
		t.Errorf("got author_name %q, want %q", out.Details.AuthorName, "admin")
	}
}

// TestGetInstance_ValidationError verifies that GetInstance_ValidationError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetInstance_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := GetInstance(context.Background(), client, GetInstanceInput{EventID: 0})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

// TestListGroup_Success verifies that ListGroup succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/groups/5/audit_events (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestListGroup_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/groups/5/audit_events" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"id":2,"author_id":10,"entity_id":5,"entity_type":"Group","event_name":"group_update","event_type":"auth","details":{"entity_path":"my-group"},"created_at":"2026-01-16T10:00:00Z"}
		]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	}))

	out, err := ListGroup(context.Background(), client, ListGroupInput{GroupID: "5"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.AuditEvents) != 1 {
		t.Fatalf("got %d events, want 1", len(out.AuditEvents))
	}
	if out.AuditEvents[0].EntityType != "Group" {
		t.Errorf("got entity_type %q, want %q", out.AuditEvents[0].EntityType, "Group")
	}
}

// TestListGroup_ValidationError verifies that ListGroup_ValidationError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListGroup_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := ListGroup(context.Background(), client, ListGroupInput{})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

// TestGetGroup_Success verifies that GetGroup succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/groups/5/audit_events/2 (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestGetGroup_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/groups/5/audit_events/2" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{"id":2,"author_id":10,"entity_id":5,"entity_type":"Group","event_name":"group_update","event_type":"auth","details":{},"created_at":"2026-01-16T10:00:00Z"}`)
	}))

	out, err := GetGroup(context.Background(), client, GetGroupInput{GroupID: "5", EventID: 2})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 2 {
		t.Errorf("got ID %d, want 2", out.ID)
	}
}

// TestGetGroup_ValidationError_MissingGroup verifies that GetGroup_ValidationError_MissingGroup returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetGroup_ValidationError_MissingGroup(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := GetGroup(context.Background(), client, GetGroupInput{EventID: 1})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

// TestListProject_Success verifies that ListProject succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/projects/42/audit_events (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestListProject_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/audit_events" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"id":3,"author_id":10,"entity_id":42,"entity_type":"Project","event_name":"project_update","event_type":"auth","details":{"target_type":"Project","target_details":"my-project"},"created_at":"2026-01-17T10:00:00Z"}
		]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	}))

	out, err := ListProject(context.Background(), client, ListProjectInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.AuditEvents) != 1 {
		t.Fatalf("got %d events, want 1", len(out.AuditEvents))
	}
	if out.AuditEvents[0].Details.TargetType != "Project" {
		t.Errorf("got target_type %q, want %q", out.AuditEvents[0].Details.TargetType, "Project")
	}
}

// TestListProject_ValidationError verifies that ListProject_ValidationError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListProject_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := ListProject(context.Background(), client, ListProjectInput{})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

// TestGetProject_Success verifies that GetProject succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/projects/42/audit_events/3 (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestGetProject_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/audit_events/3" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{"id":3,"author_id":10,"entity_id":42,"entity_type":"Project","event_name":"project_update","event_type":"auth","details":{},"created_at":"2026-01-17T10:00:00Z"}`)
	}))

	out, err := GetProject(context.Background(), client, GetProjectInput{ProjectID: "42", EventID: 3})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 3 {
		t.Errorf("got ID %d, want 3", out.ID)
	}
}

// TestGetProject_ValidationError_MissingProject verifies that GetProject_ValidationError_MissingProject returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetProject_ValidationError_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := GetProject(context.Background(), client, GetProjectInput{EventID: 1})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

// TestListInstance_WithDateFilter verifies the ListInstance_WithDateFilter handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListInstance_WithDateFilter(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("created_after") == "" {
			t.Error("expected created_after query param")
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
	}))

	out, err := ListInstance(context.Background(), client, ListInstanceInput{
		CreatedAfter: "2026-01-01",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.AuditEvents) != 0 {
		t.Errorf("got %d events, want 0", len(out.AuditEvents))
	}
}

// --- Context cancellation tests ---

// TestListInstance_ContextCancelled verifies the ListInstance_ContextCancelled handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestListInstance_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := testutil.CancelledCtx(t)

	_, err := ListInstance(ctx, client, ListInstanceInput{})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestGetInstance_ContextCancelled verifies the GetInstance_ContextCancelled handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestGetInstance_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := testutil.CancelledCtx(t)

	_, err := GetInstance(ctx, client, GetInstanceInput{EventID: 1})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestListGroup_ContextCancelled verifies the ListGroup_ContextCancelled handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestListGroup_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := testutil.CancelledCtx(t)

	_, err := ListGroup(ctx, client, ListGroupInput{GroupID: "5"})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestGetGroup_ContextCancelled verifies the GetGroup_ContextCancelled handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestGetGroup_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := testutil.CancelledCtx(t)

	_, err := GetGroup(ctx, client, GetGroupInput{GroupID: "5", EventID: 1})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestListProject_ContextCancelled verifies the ListProject_ContextCancelled handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestListProject_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := testutil.CancelledCtx(t)

	_, err := ListProject(ctx, client, ListProjectInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestGetProject_ContextCancelled verifies the GetProject_ContextCancelled handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestGetProject_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := testutil.CancelledCtx(t)

	_, err := GetProject(ctx, client, GetProjectInput{ProjectID: "42", EventID: 1})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// --- API error tests ---

// TestListInstance_APIError verifies that ListInstance returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListInstance_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	_, err := ListInstance(context.Background(), client, ListInstanceInput{})
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
	if !strings.Contains(err.Error(), "auditListInstance") {
		t.Errorf("error should contain tool name, got: %v", err)
	}
}

// TestGetInstance_APIError verifies that GetInstance returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetInstance_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := GetInstance(context.Background(), client, GetInstanceInput{EventID: 999})
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
	if !strings.Contains(err.Error(), "auditGetInstance") {
		t.Errorf("error should contain tool name, got: %v", err)
	}
}

// TestListGroup_APIError verifies that ListGroup returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListGroup_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	_, err := ListGroup(context.Background(), client, ListGroupInput{GroupID: "5"})
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
	if !strings.Contains(err.Error(), "auditListGroup") {
		t.Errorf("error should contain tool name, got: %v", err)
	}
}

// TestGetGroup_APIError verifies that GetGroup returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetGroup_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := GetGroup(context.Background(), client, GetGroupInput{GroupID: "5", EventID: 999})
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
	if !strings.Contains(err.Error(), "auditGetGroup") {
		t.Errorf("error should contain tool name, got: %v", err)
	}
}

// TestListProject_APIError verifies that ListProject returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListProject_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	_, err := ListProject(context.Background(), client, ListProjectInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
	if !strings.Contains(err.Error(), "auditListProject") {
		t.Errorf("error should contain tool name, got: %v", err)
	}
}

// TestGetProject_APIError verifies that GetProject returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetProject_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := GetProject(context.Background(), client, GetProjectInput{ProjectID: "42", EventID: 999})
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
	if !strings.Contains(err.Error(), "auditGetProject") {
		t.Errorf("error should contain tool name, got: %v", err)
	}
}

// --- Additional validation tests ---

// TestGetGroup_ValidationError_MissingEvent verifies that GetGroup_ValidationError_MissingEvent returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetGroup_ValidationError_MissingEvent(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := GetGroup(context.Background(), client, GetGroupInput{GroupID: "5", EventID: 0})
	if err == nil {
		t.Fatal("expected validation error for event_id=0, got nil")
	}
}

// TestGetProject_ValidationError_MissingEvent verifies that GetProject_ValidationError_MissingEvent returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetProject_ValidationError_MissingEvent(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := GetProject(context.Background(), client, GetProjectInput{ProjectID: "42", EventID: 0})
	if err == nil {
		t.Fatal("expected validation error for event_id=0, got nil")
	}
}

// TestGetInstance_NegativeEventID verifies the GetInstance_NegativeEventID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGetInstance_NegativeEventID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := GetInstance(context.Background(), client, GetInstanceInput{EventID: -1})
	if err == nil {
		t.Fatal("expected validation error for negative event_id, got nil")
	}
}

// --- buildListOpts and date/pagination parameter tests ---

// TestListInstance_WithCreatedBefore verifies the ListInstance_WithCreatedBefore handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListInstance_WithCreatedBefore(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("created_before") == "" {
			t.Error("expected created_before query param")
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
	}))

	_, err := ListInstance(context.Background(), client, ListInstanceInput{
		CreatedBefore: "2026-12-31",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestListInstance_WithPagination verifies that ListInstance_WithPagination forwards pagination parameters to the GitLab API and parses the response metadata.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestListInstance_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertQueryParam(t, r, "page", "2")
		testutil.AssertQueryParam(t, r, "per_page", "10")
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{Page: "2", PerPage: "10", Total: "15", TotalPages: "2"})
	}))

	out, err := ListInstance(context.Background(), client, ListInstanceInput{
		Page: 2, PerPage: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Pagination.Page != 2 {
		t.Errorf("pagination page = %d, want 2", out.Pagination.Page)
	}
}

// TestListInstance_InvalidDateSilentlyIgnored verifies the ListInstance_InvalidDateSilentlyIgnored handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListInstance_InvalidDateSilentlyIgnored(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("created_after") != "" {
			t.Error("invalid date should not be sent")
		}
		if q.Get("created_before") != "" {
			t.Error("invalid date should not be sent")
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
	}))

	_, err := ListInstance(context.Background(), client, ListInstanceInput{
		CreatedAfter:  "not-a-date",
		CreatedBefore: "also-bad",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestListGroup_WithDateFilter verifies the ListGroup_WithDateFilter handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListGroup_WithDateFilter(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("created_after") == "" {
			t.Error("expected created_after query param")
		}
		if q.Get("created_before") == "" {
			t.Error("expected created_before query param")
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
	}))

	_, err := ListGroup(context.Background(), client, ListGroupInput{
		GroupID:       "5",
		CreatedAfter:  "2026-01-01",
		CreatedBefore: "2026-12-31",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestListProject_WithDateFilter verifies the ListProject_WithDateFilter handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListProject_WithDateFilter(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("created_after") == "" {
			t.Error("expected created_after query param")
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
	}))

	_, err := ListProject(context.Background(), client, ListProjectInput{
		ProjectID:    "42",
		CreatedAfter: "2026-06-01",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- toOutput detail coverage ---

// TestListInstance_AllDetails verifies the ListInstance_AllDetails handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListInstance_AllDetails(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[{
			"id": 10,
			"author_id": 20,
			"entity_id": 30,
			"entity_type": "Project",
			"event_name": "project_access_granted",
			"event_type": "security",
			"details": {
				"custom_message": "Granted access",
				"author_name": "admin",
				"target_id": "42",
				"target_type": "User",
				"target_details": "user@example.com",
				"ip_address": "10.0.0.1",
				"entity_path": "group/project"
			},
			"created_at": "2026-03-15T08:30:00Z"
		}]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	}))

	out, err := ListInstance(context.Background(), client, ListInstanceInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.AuditEvents) != 1 {
		t.Fatalf("got %d events, want 1", len(out.AuditEvents))
	}
	e := out.AuditEvents[0]
	if e.ID != 10 {
		t.Errorf("ID = %d, want 10", e.ID)
	}
	if e.AuthorID != 20 {
		t.Errorf("AuthorID = %d, want 20", e.AuthorID)
	}
	if e.EntityID != 30 {
		t.Errorf("EntityID = %d, want 30", e.EntityID)
	}
	if e.EntityType != "Project" {
		t.Errorf("EntityType = %q, want %q", e.EntityType, "Project")
	}
	if e.EventName != "project_access_granted" {
		t.Errorf("EventName = %q, want %q", e.EventName, "project_access_granted")
	}
	if e.CreatedAt == "" {
		t.Error("CreatedAt should not be empty")
	}
	if e.Details.CustomMessage != "Granted access" {
		t.Errorf("CustomMessage = %q, want %q", e.Details.CustomMessage, "Granted access")
	}
	if e.Details.AuthorName != "admin" {
		t.Errorf("AuthorName = %q, want %q", e.Details.AuthorName, "admin")
	}
	if e.Details.TargetID != "42" {
		t.Errorf("TargetID = %q, want %q", e.Details.TargetID, "42")
	}
	if e.Details.TargetType != "User" {
		t.Errorf("TargetType = %q, want %q", e.Details.TargetType, "User")
	}
	if e.Details.TargetDetails != "user@example.com" {
		t.Errorf("TargetDetails = %q, want %q", e.Details.TargetDetails, "user@example.com")
	}
	if e.Details.IPAddress != "10.0.0.1" {
		t.Errorf("IPAddress = %q, want %q", e.Details.IPAddress, "10.0.0.1")
	}
	if e.Details.EntityPath != "group/project" {
		t.Errorf("EntityPath = %q, want %q", e.Details.EntityPath, "group/project")
	}
}

// TestGetInstance_NilCreatedAt verifies the GetInstance_NilCreatedAt handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGetInstance_NilCreatedAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK,
			`{"id":5,"author_id":1,"entity_id":0,"entity_type":"User","event_name":"login","event_type":"auth","details":{}}`)
	}))

	out, err := GetInstance(context.Background(), client, GetInstanceInput{EventID: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ID != 5 {
		t.Errorf("ID = %d, want 5", out.ID)
	}
	if out.CreatedAt != "" {
		t.Errorf("CreatedAt should be empty for nil time, got %q", out.CreatedAt)
	}
}

// TestListInstance_MultipleEvents verifies the ListInstance_MultipleEvents handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListInstance_MultipleEvents(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"id":1,"author_id":10,"entity_id":0,"entity_type":"User","event_name":"login","event_type":"auth","details":{},"created_at":"2026-01-01T00:00:00Z"},
			{"id":2,"author_id":11,"entity_id":0,"entity_type":"User","event_name":"logout","event_type":"auth","details":{},"created_at":"2026-01-02T00:00:00Z"},
			{"id":3,"author_id":12,"entity_id":5,"entity_type":"Group","event_name":"group_create","event_type":"admin","details":{},"created_at":"2026-01-03T00:00:00Z"}
		]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "3", TotalPages: "1"})
	}))

	out, err := ListInstance(context.Background(), client, ListInstanceInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.AuditEvents) != 3 {
		t.Fatalf("got %d events, want 3", len(out.AuditEvents))
	}
	if out.AuditEvents[0].ID != 1 {
		t.Errorf("first event ID = %d, want 1", out.AuditEvents[0].ID)
	}
	if out.AuditEvents[2].EventName != "group_create" {
		t.Errorf("third event name = %q, want %q", out.AuditEvents[2].EventName, "group_create")
	}
}

// TestListInstance_EmptyResult verifies the ListInstance_EmptyResult handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListInstance_EmptyResult(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
	}))

	out, err := ListInstance(context.Background(), client, ListInstanceInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.AuditEvents) != 0 {
		t.Errorf("got %d events, want 0", len(out.AuditEvents))
	}
}

// --- Markdown formatter tests ---

// eventHints is the guidance section every single-event card closes with.
const eventHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'audit_event.list_project' to browse a project's audit events\n" +
	"- Use action 'audit_event.list_group' to browse a group's audit events\n" +
	"- Use action 'audit_event.list_instance' to browse the instance's audit events\n"

// TestFormatMarkdown_Full verifies that one audit event renders as a card: one
// list item per field, the address in a code span, and the instant in the
// display form. The whole render is compared, since a substring assertion
// passes on a row that landed outside the block it was meant for.
func TestFormatMarkdown_Full(t *testing.T) {
	e := Output{
		ID:         42,
		AuthorID:   10,
		EntityID:   5,
		EntityType: "Project",
		EventName:  "project_update",
		CreatedAt:  "2026-06-15T12:00:00Z",
		Details: DetailsOutput{
			AuthorName:    "admin",
			TargetType:    "Setting",
			TargetDetails: "visibility_level",
			IPAddress:     "192.168.1.1",
			EntityPath:    "group/project",
		},
	}

	want := "## Audit Event #42\n\n" +
		"- **ID**: 42\n" +
		"- **Event Name**: project_update\n" +
		"- **Entity Type**: Project\n" +
		"- **Entity ID**: 5\n" +
		"- **Entity Path**: group/project\n" +
		"- **Created**: 15 Jun 2026 12:00 UTC\n" +
		"- **Author ID**: 10\n" +
		"- **Author Name**: admin\n" +
		"- **Target Type**: Setting\n" +
		"- **Target Details**: visibility_level\n" +
		"- **IP Address**: `192.168.1.1`\n" +
		eventHints

	if got := FormatMarkdown(e); got != want {
		t.Errorf("FormatMarkdown() =\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatMarkdown_SingularDetailFields verifies that every singular detail
// field reaches the card. They are what a particular audit event says about
// itself, and the card used to show five of them and drop the rest, leaving a
// reader with an event name and nothing about what changed.
func TestFormatMarkdown_SingularDetailFields(t *testing.T) {
	e := Output{
		ID:         7,
		AuthorID:   3,
		EntityType: "Group",
		EventName:  "group_settings_updated",
		Details: DetailsOutput{
			With:          "standard",
			Add:           "deploy_key",
			As:            "Maintainer",
			Change:        "visibility",
			From:          "private",
			To:            "internal",
			Remove:        "user_access",
			CustomMessage: "Changed by the compliance bot",
			AuthorEmail:   "admin@example.com",
			AuthorClass:   "User",
			TargetID:      "99",
			FailedLogin:   "STANDARD",
			EventName:     "group_visibility_changed",
		},
	}

	want := "## Audit Event #7\n\n" +
		"- **ID**: 7\n" +
		"- **Event Name**: group_settings_updated\n" +
		"- **Detail Event Name**: group_visibility_changed\n" +
		"- **Entity Type**: Group\n" +
		"- **Entity ID**: 0\n" +
		"- **Author ID**: 3\n" +
		"- **Author Email**: admin@example.com\n" +
		"- **Author Class**: User\n" +
		"- **Target ID**: 99\n" +
		"- **Failed Login**: STANDARD\n" +
		"- **With**: standard\n" +
		"- **As**: Maintainer\n" +
		"- **Add**: deploy_key\n" +
		"- **Remove**: user_access\n" +
		"- **Change**: visibility\n" +
		"- **From**: private\n" +
		"- **To**: internal\n" +
		"- **Custom Message**: Changed by the compliance bot\n" +
		eventHints

	if got := FormatMarkdown(e); got != want {
		t.Errorf("FormatMarkdown() =\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatMarkdown_ChangesAndChangeObject verifies that the plural changes
// array renders as the nested collection it is, under its own heading, and
// that an object-valued change renders inside a fence sized to the document.
func TestFormatMarkdown_ChangesAndChangeObject(t *testing.T) {
	e := Output{
		ID:         8,
		EntityType: "Project",
		EventName:  "project_group_link_updated",
		Details: DetailsOutput{
			Changes: []ChangeEntry{
				{Change: "visibility", From: "private", To: "internal"},
				{Change: "description", From: "old", To: "new"},
			},
			ChangeObject: map[string]any{"group_access": "30"},
		},
	}

	want := "## Audit Event #8\n\n" +
		"- **ID**: 8\n" +
		"- **Event Name**: project_group_link_updated\n" +
		"- **Entity Type**: Project\n" +
		"- **Entity ID**: 0\n" +
		"- **Author ID**: 0\n\n" +
		"### Changes\n\n" +
		"| Change | From | To |\n" +
		"| --- | --- | --- |\n" +
		"| visibility | private | internal |\n" +
		"| description | old | new |\n\n" +
		"### Change (object)\n\n" +
		"```json\n{\"group_access\":\"30\"}\n```\n" +
		eventHints

	if got := FormatMarkdown(e); got != want {
		t.Errorf("FormatMarkdown() =\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatMarkdown_Minimal verifies that an event GitLab sent no details for
// writes no detail row at all: an absent value is never a label with nothing
// after it.
func TestFormatMarkdown_Minimal(t *testing.T) {
	e := Output{
		ID:         1,
		AuthorID:   2,
		EntityID:   0,
		EntityType: "User",
		EventName:  "login",
	}

	want := "## Audit Event #1\n\n" +
		"- **ID**: 1\n" +
		"- **Event Name**: login\n" +
		"- **Entity Type**: User\n" +
		"- **Entity ID**: 0\n" +
		"- **Author ID**: 2\n" +
		eventHints

	if got := FormatMarkdown(e); got != want {
		t.Errorf("FormatMarkdown() =\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatMarkdown_HostileDetail verifies that a detail value cannot open a
// heading, a list item or a link of its own: the row escaper neutralizes the
// pipe, the tag and the bracket, and the card's structure is unchanged.
func TestFormatMarkdown_HostileDetail(t *testing.T) {
	e := Output{
		ID:        2,
		EventName: "login",
		Details:   DetailsOutput{TargetDetails: "a|b\n## injected\n- **State**: closed"},
	}

	want := "## Audit Event #2\n\n" +
		"- **ID**: 2\n" +
		"- **Event Name**: login\n" +
		"- **Entity ID**: 0\n" +
		"- **Author ID**: 0\n" +
		"- **Target Details**: a&#124;b ## injected - **State**: closed\n" +
		eventHints

	if got := FormatMarkdown(e); got != want {
		t.Errorf("FormatMarkdown() =\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdown_WithEvents verifies that a page of audit events
// renders as one table under a heading counting what the response reports,
// with the summary line above the rows rather than after them, where it was
// absorbed as one more row.
func TestFormatListMarkdown_WithEvents(t *testing.T) {
	out := ListOutput{
		AuditEvents: []Output{
			{ID: 1, EventName: "login", EntityType: "User", EntityID: 0, AuthorID: 10, CreatedAt: "2026-01-01T00:00:00Z"},
			{ID: 2, EventName: "logout", EntityType: "User", EntityID: 0, AuthorID: 11, CreatedAt: "2026-01-02T00:00:00Z"},
		},
		Pagination: toolutil.PaginationOutput{Page: 1, PerPage: 20, TotalItems: 2, TotalPages: 1},
	}

	want := "## Audit Events (2)\n\n" +
		"| ID | Event Name | Entity Type | Entity ID | Author ID | Created |\n" +
		"| --- | --- | --- | --- | --- | --- |\n" +
		"| 1 | login | User | 0 | 10 | 1 Jan 2026 00:00 UTC |\n" +
		"| 2 | logout | User | 0 | 11 | 2 Jan 2026 00:00 UTC |\n\n" +
		"Page 1 of 1 | 2 items total | 20 per page\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'audit_event.get_project' to read one project event in full, details included\n" +
		"- Use action 'audit_event.get_group' to read one group event in full\n" +
		"- Use action 'audit_event.get_instance' to read one instance event in full\n"

	got := FormatListMarkdown(out)
	if got != want {
		t.Errorf("FormatListMarkdown() =\n%s\nwant:\n%s", got, want)
	}
	if strings.Contains(got, "clickable [text](url) links") {
		t.Error("the list tells the model to keep links a table without links cannot have")
	}
}

// TestFormatListMarkdown_MultiPage verifies that the summary line sits between
// the heading and the table, where it is a paragraph of its own, and that the
// heading counts what GitLab reported rather than the page length.
func TestFormatListMarkdown_MultiPage(t *testing.T) {
	out := ListOutput{
		AuditEvents: []Output{{ID: 1, EventName: "login", EntityType: "User", AuthorID: 10}},
		Pagination:  toolutil.PaginationOutput{Page: 1, PerPage: 1, TotalItems: 45, TotalPages: 45, HasMore: true, NextPage: 2},
	}

	want := "## Audit Events (45)\n\n" +
		"Showing 1 of 45 results (page 1 of 45)\n\n" +
		"| ID | Event Name | Entity Type | Entity ID | Author ID | Created |\n" +
		"| --- | --- | --- | --- | --- | --- |\n" +
		"| 1 | login | User | 0 | 10 |  |\n\n" +
		"Page 1 of 45 | 45 items total | 1 per page\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'audit_event.get_project' to read one project event in full, details included\n" +
		"- Use action 'audit_event.get_group' to read one group event in full\n" +
		"- Use action 'audit_event.get_instance' to read one instance event in full\n"

	if got := FormatListMarkdown(out); got != want {
		t.Errorf("FormatListMarkdown() =\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdown_Empty verifies that an empty page renders the one
// empty-list sentence and nothing else.
func TestFormatListMarkdown_Empty(t *testing.T) {
	if got, want := FormatListMarkdown(ListOutput{AuditEvents: []Output{}}), "No audit events found.\n"; got != want {
		t.Errorf("FormatListMarkdown() = %q, want %q", got, want)
	}
}

// TestGetInstance_ChangesArray_MapsThrough verifies that a plural "changes"
// array in details is mapped through to DetailsOutput.Changes with each entry's
// change/from/to fields preserved (client-go AuditEventChange, SDK !2949).
// The mock GitLab API at /api/v4/audit_events/7 (GET) returns an event whose
// details include a two-element changes array.
func TestGetInstance_ChangesArray_MapsThrough(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/audit_events/7" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{
			"id":7,"author_id":10,"entity_id":0,"entity_type":"Group","event_name":"group_settings_updated","event_type":"settings",
			"details":{"changes":[
				{"change":"visibility","from":"private","to":"internal"},
				{"change":"description","from":"old","to":"new"}
			]},
			"created_at":"2026-01-15T10:00:00Z"
		}`)
	}))

	out, err := GetInstance(context.Background(), client, GetInstanceInput{EventID: 7})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Details.Changes) != 2 {
		t.Fatalf("got %d changes, want 2", len(out.Details.Changes))
	}
	if out.Details.Changes[0].Change != "visibility" ||
		out.Details.Changes[0].From != "private" ||
		out.Details.Changes[0].To != "internal" {
		t.Errorf("changes[0] mismatch: %+v", out.Details.Changes[0])
	}
	if out.Details.Changes[1].Change != "description" ||
		out.Details.Changes[1].From != "old" ||
		out.Details.Changes[1].To != "new" {
		t.Errorf("changes[1] mismatch: %+v", out.Details.Changes[1])
	}
}

// TestGetInstance_ObjectValuedChange_LandsInChangeObject verifies that when the
// API returns "change" as a JSON object rather than a plain string (e.g.
// project_group_link_updated), the object is preserved in DetailsOutput.ChangeObject
// as raw JSON, while the plain-string Change field stays empty (SDK !2949 custom
// AuditEventDetails.UnmarshalJSON).
func TestGetInstance_ObjectValuedChange_LandsInChangeObject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/audit_events/8" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{
			"id":8,"author_id":10,"entity_id":0,"entity_type":"Project","event_name":"project_group_link_updated","event_type":"settings",
			"details":{"change":{"group_access":{"from":10,"to":30}}},
			"created_at":"2026-01-15T10:00:00Z"
		}`)
	}))

	out, err := GetInstance(context.Background(), client, GetInstanceInput{EventID: 8})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Details.Change != "" {
		t.Errorf("plain-string Change should be empty for object-valued change, got %q", out.Details.Change)
	}
	if out.Details.ChangeObject == nil {
		t.Fatal("ChangeObject should hold the object-valued change, got nil")
	}
	raw, err := json.Marshal(out.Details.ChangeObject)
	if err != nil {
		t.Fatalf("marshal ChangeObject: %v", err)
	}
	if !strings.Contains(string(raw), "group_access") {
		t.Errorf("ChangeObject missing expected content, got %s", string(raw))
	}
}

// TestFormatMarkdown_RawChangeObject verifies that an object-valued change
// still decoded as json.RawMessage, which is what the SDK hands the converter
// before it unmarshals, reaches the fence as the document it is.
func TestFormatMarkdown_RawChangeObject(t *testing.T) {
	e := Output{
		ID:        9,
		EventName: "project_group_link_updated",
		Details: DetailsOutput{
			ChangeObject: json.RawMessage(`{"group_access":{"from":10,"to":30}}`),
		},
	}

	want := "## Audit Event #9\n\n" +
		"- **ID**: 9\n" +
		"- **Event Name**: project_group_link_updated\n" +
		"- **Entity ID**: 0\n" +
		"- **Author ID**: 0\n\n" +
		"### Change (object)\n\n" +
		"```json\n{\"group_access\":{\"from\":10,\"to\":30}}\n```\n" +
		eventHints

	if got := FormatMarkdown(e); got != want {
		t.Errorf("FormatMarkdown() =\n%s\nwant:\n%s", got, want)
	}
}
