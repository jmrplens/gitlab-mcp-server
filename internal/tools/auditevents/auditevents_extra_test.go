package auditevents

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// --- Context cancellation tests ---

// TestListInstance_ContextCancelled verifies ListInstance returns an error
// when the context is already cancelled before the API call.
func TestListInstance_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called on cancelled context")
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ListInstance(ctx, client, ListInstanceInput{})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestGetInstance_ContextCancelled verifies GetInstance returns an error
// when the context is already cancelled.
func TestGetInstance_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := GetInstance(ctx, client, GetInstanceInput{EventID: 1})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestListGroup_ContextCancelled verifies ListGroup returns an error
// when the context is already cancelled.
func TestListGroup_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ListGroup(ctx, client, ListGroupInput{GroupID: "5"})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestGetGroup_ContextCancelled verifies GetGroup returns an error
// when the context is already cancelled.
func TestGetGroup_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := GetGroup(ctx, client, GetGroupInput{GroupID: "5", EventID: 1})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestListProject_ContextCancelled verifies ListProject returns an error
// when the context is already cancelled.
func TestListProject_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ListProject(ctx, client, ListProjectInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestGetProject_ContextCancelled verifies GetProject returns an error
// when the context is already cancelled.
func TestGetProject_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := GetProject(ctx, client, GetProjectInput{ProjectID: "42", EventID: 1})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// --- API error tests ---

// TestListInstance_APIError verifies ListInstance wraps API errors correctly.
func TestListInstance_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"500 Internal Server Error"}`)
	}))

	_, err := ListInstance(context.Background(), client, ListInstanceInput{})
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
	if !strings.Contains(err.Error(), "auditListInstance") {
		t.Errorf("error should contain tool name, got: %v", err)
	}
}

// TestGetInstance_APIError verifies GetInstance wraps API errors correctly.
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

// TestListGroup_APIError verifies ListGroup wraps API errors correctly.
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

// TestGetGroup_APIError verifies GetGroup wraps API errors correctly.
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

// TestListProject_APIError verifies ListProject wraps API errors correctly.
func TestListProject_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"500 Internal Server Error"}`)
	}))

	_, err := ListProject(context.Background(), client, ListProjectInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
	if !strings.Contains(err.Error(), "auditListProject") {
		t.Errorf("error should contain tool name, got: %v", err)
	}
}

// TestGetProject_APIError verifies GetProject wraps API errors correctly.
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

// TestGetGroup_ValidationError_MissingEvent verifies GetGroup rejects event_id=0.
func TestGetGroup_ValidationError_MissingEvent(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))

	_, err := GetGroup(context.Background(), client, GetGroupInput{GroupID: "5", EventID: 0})
	if err == nil {
		t.Fatal("expected validation error for event_id=0, got nil")
	}
}

// TestGetProject_ValidationError_MissingEvent verifies GetProject rejects event_id=0.
func TestGetProject_ValidationError_MissingEvent(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))

	_, err := GetProject(context.Background(), client, GetProjectInput{ProjectID: "42", EventID: 0})
	if err == nil {
		t.Fatal("expected validation error for event_id=0, got nil")
	}
}

// TestGetInstance_NegativeEventID verifies GetInstance rejects negative event_id.
func TestGetInstance_NegativeEventID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))

	_, err := GetInstance(context.Background(), client, GetInstanceInput{EventID: -1})
	if err == nil {
		t.Fatal("expected validation error for negative event_id, got nil")
	}
}

// --- buildListOpts and date/pagination parameter tests ---

// TestListInstance_WithCreatedBefore verifies created_before filter is passed to the API.
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

// TestListInstance_WithPagination verifies page and per_page are passed to the API.
func TestListInstance_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertQueryParam(t, r, "page", "2")
		testutil.AssertQueryParam(t, r, "per_page", "10")
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{Page: "2", PerPage: "10", Total: "15", TotalPages: "2"})
	}))

	out, err := ListInstance(context.Background(), client, ListInstanceInput{
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 10},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Pagination.Page != 2 {
		t.Errorf("pagination page = %d, want 2", out.Pagination.Page)
	}
}

// TestListInstance_InvalidDateSilentlyIgnored verifies invalid dates are silently ignored.
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

// TestListGroup_WithDateFilter verifies date filters are applied for group listing.
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

// TestListProject_WithDateFilter verifies date filters are applied for project listing.
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

// TestListInstance_AllDetails verifies toOutput maps all detail fields including
// TargetID, CustomMessage, and EntityPath from the API response.
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
	if e.EventType != "security" {
		t.Errorf("EventType = %q, want %q", e.EventType, "security")
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

// TestGetInstance_NilCreatedAt verifies toOutput handles nil created_at gracefully.
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

// TestListInstance_MultipleEvents verifies ListInstance handles multiple events.
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

// TestListInstance_EmptyResult verifies ListInstance handles zero events.
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

// TestFormatMarkdown_Full verifies FormatMarkdown renders all detail fields.
func TestFormatMarkdown_Full(t *testing.T) {
	e := Output{
		ID:         42,
		AuthorID:   10,
		EntityID:   5,
		EntityType: "Project",
		EventName:  "project_update",
		EventType:  "admin",
		CreatedAt:  "2026-06-15T12:00:00Z",
		Details: DetailsOutput{
			AuthorName:    "admin",
			TargetType:    "Setting",
			TargetDetails: "visibility_level",
			IPAddress:     "192.168.1.1",
			EntityPath:    "group/project",
		},
	}
	md := FormatMarkdown(e)
	checks := []string{
		"## Audit Event #42",
		"| ID | 42 |",
		"| Author ID | 10 |",
		"| Entity ID | 5 |",
		"| Entity Type | Project |",
		"| Event Name | project_update |",
		"| Author Name | admin |",
		"| Target Type | Setting |",
		"| Target Details | visibility_level |",
		"| IP Address | 192.168.1.1 |",
		"| Entity Path | group/project |",
	}
	for _, want := range checks {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q", want)
		}
	}
}

// TestFormatMarkdown_Minimal verifies FormatMarkdown omits empty detail fields.
func TestFormatMarkdown_Minimal(t *testing.T) {
	e := Output{
		ID:         1,
		AuthorID:   2,
		EntityID:   0,
		EntityType: "User",
		EventName:  "login",
		EventType:  "auth",
	}
	md := FormatMarkdown(e)
	if !strings.Contains(md, "## Audit Event #1") {
		t.Error("markdown missing header")
	}
	if strings.Contains(md, "Author Name") {
		t.Error("markdown should not contain Author Name for empty details")
	}
	if strings.Contains(md, "Target Type") {
		t.Error("markdown should not contain Target Type for empty details")
	}
	if strings.Contains(md, "IP Address") {
		t.Error("markdown should not contain IP Address for empty details")
	}
	if strings.Contains(md, "Entity Path") {
		t.Error("markdown should not contain Entity Path for empty details")
	}
}

// TestFormatListMarkdown_WithEvents verifies FormatListMarkdown renders a table.
func TestFormatListMarkdown_WithEvents(t *testing.T) {
	out := ListOutput{
		AuditEvents: []Output{
			{ID: 1, EventName: "login", EntityType: "User", EntityID: 0, AuthorID: 10, CreatedAt: "2026-01-01T00:00:00Z"},
			{ID: 2, EventName: "logout", EntityType: "User", EntityID: 0, AuthorID: 11, CreatedAt: "2026-01-02T00:00:00Z"},
		},
		Pagination: toolutil.PaginationOutput{Page: 1, PerPage: 20, TotalItems: 2, TotalPages: 1},
	}
	md := FormatListMarkdown(out)
	if !strings.Contains(md, "## Audit Events") {
		t.Error("markdown missing header")
	}
	if !strings.Contains(md, "| 1 |") {
		t.Error("markdown missing event ID 1")
	}
	if !strings.Contains(md, "| 2 |") {
		t.Error("markdown missing event ID 2")
	}
	if strings.Contains(md, "No audit events found") {
		t.Error("markdown should not contain empty message when events exist")
	}
}

// TestFormatListMarkdown_Empty verifies FormatListMarkdown shows placeholder for no events.
func TestFormatListMarkdown_Empty(t *testing.T) {
	out := ListOutput{
		AuditEvents: []Output{},
	}
	md := FormatListMarkdown(out)
	if !strings.Contains(md, "No audit events found") {
		t.Error("markdown missing 'No audit events found' for empty list")
	}
}
