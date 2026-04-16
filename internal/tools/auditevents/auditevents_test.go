package auditevents

import (
	"context"
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const fmtUnexpErr = "unexpected error: %v"

// TestListInstance_Success verifies listing instance audit events returns correct results.
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

// TestGetInstance_Success verifies getting a single instance audit event.
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

// TestGetInstance_ValidationError verifies GetInstance rejects invalid event_id.
func TestGetInstance_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))
	_, err := GetInstance(context.Background(), client, GetInstanceInput{EventID: 0})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

// TestListGroup_Success verifies listing group audit events.
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

// TestListGroup_ValidationError verifies ListGroup rejects empty group_id.
func TestListGroup_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))
	_, err := ListGroup(context.Background(), client, ListGroupInput{})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

// TestGetGroup_Success verifies getting a single group audit event.
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

// TestGetGroup_ValidationError_MissingGroup verifies GetGroup rejects empty group_id.
func TestGetGroup_ValidationError_MissingGroup(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))
	_, err := GetGroup(context.Background(), client, GetGroupInput{EventID: 1})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

// TestListProject_Success verifies listing project audit events.
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

// TestListProject_ValidationError verifies ListProject rejects empty project_id.
func TestListProject_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))
	_, err := ListProject(context.Background(), client, ListProjectInput{})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

// TestGetProject_Success verifies getting a single project audit event.
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

// TestGetProject_ValidationError_MissingProject verifies GetProject rejects empty project_id.
func TestGetProject_ValidationError_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))
	_, err := GetProject(context.Background(), client, GetProjectInput{EventID: 1})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

// TestListInstance_WithDateFilter verifies date filtering is applied.
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
