package resourceevents

import (
	"context"
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// TestListIssueIterationEvents_Success verifies ListIssueIterationEvents returns correct fields.
func TestListIssueIterationEvents_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/issues/1/resource_iteration_events" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"id":1,"action":"add","created_at":"2024-01-15T10:00:00Z","resource_type":"Issue","resource_id":1,"user":{"id":5,"username":"alice"},"iteration":{"id":10,"iid":1,"sequence":1,"group_id":5,"title":"Sprint 1","state":3,"web_url":"https://gitlab.example.com/iterations/10"}}
		]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	}))

	out, err := ListIssueIterationEvents(context.Background(), client, ListIssueIterationEventsInput{ProjectID: "42", IssueIID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Events) != 1 {
		t.Fatalf("got %d events, want 1", len(out.Events))
	}
	if out.Events[0].Action != "add" {
		t.Errorf("got action %q, want %q", out.Events[0].Action, "add")
	}
	if out.Events[0].Username != "alice" {
		t.Errorf("got username %q, want %q", out.Events[0].Username, "alice")
	}
	if out.Events[0].Iteration.Title != "Sprint 1" {
		t.Errorf("got iteration title %q, want %q", out.Events[0].Iteration.Title, "Sprint 1")
	}
}

// TestListIssueIterationEvents_ValidationError verifies error when ProjectID is empty.
func TestListIssueIterationEvents_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))
	_, err := ListIssueIterationEvents(context.Background(), client, ListIssueIterationEventsInput{IssueIID: 1})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

// TestGetIssueIterationEvent_Success verifies GetIssueIterationEvent returns correct fields.
func TestGetIssueIterationEvent_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/issues/1/resource_iteration_events/1" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"action":"add","created_at":"2024-01-15T10:00:00Z","resource_type":"Issue","resource_id":1,"user":{"id":5,"username":"alice"},"iteration":{"id":10,"iid":1,"sequence":1,"group_id":5,"title":"Sprint 1","state":3,"web_url":"https://gitlab.example.com/iterations/10"}}`)
	}))

	out, err := GetIssueIterationEvent(context.Background(), client, GetIssueIterationEventInput{ProjectID: "42", IssueIID: 1, IterationEventID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 1 {
		t.Errorf("got ID %d, want 1", out.ID)
	}
	if out.Iteration.Title != "Sprint 1" {
		t.Errorf("got iteration title %q, want %q", out.Iteration.Title, "Sprint 1")
	}
	if out.Username != "alice" {
		t.Errorf("got username %q, want %q", out.Username, "alice")
	}
}

// TestGetIssueIterationEvent_ValidationError_MissingEventID verifies error when IterationEventID is 0.
func TestGetIssueIterationEvent_ValidationError_MissingEventID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))
	_, err := GetIssueIterationEvent(context.Background(), client, GetIssueIterationEventInput{ProjectID: "42", IssueIID: 1, IterationEventID: 0})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

// TestListIssueWeightEvents_Success verifies ListIssueWeightEvents returns correct fields.
func TestListIssueWeightEvents_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/issues/1/resource_weight_events" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"id":1,"created_at":"2024-01-15T10:00:00Z","resource_type":"Issue","resource_id":1,"state":"weight_changed","issue_id":1,"weight":5,"user":{"id":5,"username":"alice"}}
		]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	}))

	out, err := ListIssueWeightEvents(context.Background(), client, ListIssueWeightEventsInput{ProjectID: "42", IssueIID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Events) != 1 {
		t.Fatalf("got %d events, want 1", len(out.Events))
	}
	if out.Events[0].Weight != 5 {
		t.Errorf("got weight %d, want 5", out.Events[0].Weight)
	}
	if out.Events[0].Username != "alice" {
		t.Errorf("got username %q, want %q", out.Events[0].Username, "alice")
	}
}

// TestListIssueWeightEvents_ValidationError verifies error when ProjectID is empty.
func TestListIssueWeightEvents_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))
	_, err := ListIssueWeightEvents(context.Background(), client, ListIssueWeightEventsInput{IssueIID: 1})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}
