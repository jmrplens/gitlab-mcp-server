// resourceevents_test.go contains unit tests for the resource event MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package resourceevents

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

const (
	errNoReachAPI   = "should not reach API"
	fmtWantOneEvent = "got %d events, want 1"
	fmtGotStateWant = "got state %q, want %q"
	fmtGotWant      = "got %q, want %q"
)

// TestListIssueLabelEvents_Success_DetailedFields verifies ListIssueLabelEvents returns correct fields.
func TestListIssueLabelEvents_Success_DetailedFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/issues/1/resource_label_events" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"id":10,"action":"add","created_at":"2026-01-15T10:00:00Z","resource_type":"Issue","resource_id":1,"user":{"id":5,"username":"alice"},"label":{"id":100,"name":"bug","color":"#ff0000","text_color":"#ffffff"}}
		]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	}))

	out, err := ListIssueLabelEvents(context.Background(), client, ListIssueLabelEventsInput{ProjectID: "42", IssueIID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Events) != 1 {
		t.Fatalf(fmtWantOneEvent, len(out.Events))
	}
	if out.Events[0].Action != "add" {
		t.Errorf("got action %q, want %q", out.Events[0].Action, "add")
	}
	if out.Events[0].Label.Name != "bug" {
		t.Errorf("got label %q, want %q", out.Events[0].Label.Name, "bug")
	}
	if out.Events[0].Username != "alice" {
		t.Errorf("got username %q, want %q", out.Events[0].Username, "alice")
	}
}

// TestGetIssueLabelEvent_Success_DetailedFields verifies GetIssueLabelEvent returns correct fields.
func TestGetIssueLabelEvent_Success_DetailedFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/issues/1/resource_label_events/10" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{"id":10,"action":"add","created_at":"2026-01-15T10:00:00Z","resource_type":"Issue","resource_id":1,"user":{"id":5,"username":"alice"},"label":{"id":100,"name":"bug","color":"#ff0000"}}`)
	}))

	out, err := GetIssueLabelEvent(context.Background(), client, GetIssueLabelEventInput{ProjectID: "42", IssueIID: 1, LabelEventID: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 10 {
		t.Errorf("got ID %d, want 10", out.ID)
	}
	if out.Label.Name != "bug" {
		t.Errorf("got label %q, want %q", out.Label.Name, "bug")
	}
}

// TestListIssueLabelEvents_ValidationError verifies the behavior of list issue label events validation error.
func TestListIssueLabelEvents_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))
	_, err := ListIssueLabelEvents(context.Background(), client, ListIssueLabelEventsInput{IssueIID: 1})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

// TestListMRLabelEvents_Success_DetailedFields verifies ListMRLabelEvents returns correct fields.
func TestListMRLabelEvents_Success_DetailedFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/merge_requests/5/resource_label_events" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"id":20,"action":"remove","created_at":"2026-02-01T12:00:00Z","resource_type":"MergeRequest","resource_id":5,"user":{"id":6,"username":"bob"},"label":{"id":101,"name":"feature"}}
		]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	}))

	out, err := ListMRLabelEvents(context.Background(), client, ListMRLabelEventsInput{ProjectID: "42", MRIID: 5})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Events) != 1 {
		t.Fatalf(fmtWantOneEvent, len(out.Events))
	}
	if out.Events[0].Action != "remove" {
		t.Errorf("got action %q, want %q", out.Events[0].Action, "remove")
	}
}

// TestListIssueMilestoneEvents_Success_DetailedFields verifies ListIssueMilestoneEvents returns correct fields.
func TestListIssueMilestoneEvents_Success_DetailedFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/issues/1/resource_milestone_events" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"id":30,"action":"add","created_at":"2026-03-01T08:00:00Z","resource_type":"Issue","resource_id":1,"user":{"id":5,"username":"alice"},"milestone":{"id":200,"title":"v1.0"}}
		]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	}))

	out, err := ListIssueMilestoneEvents(context.Background(), client, ListIssueMilestoneEventsInput{ProjectID: "42", IssueIID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Events) != 1 {
		t.Fatalf(fmtWantOneEvent, len(out.Events))
	}
	if out.Events[0].MilestoneTitle != "v1.0" {
		t.Errorf("got milestone %q, want %q", out.Events[0].MilestoneTitle, "v1.0")
	}
}

// TestGetIssueMilestoneEvent_Success_DetailedFields verifies GetIssueMilestoneEvent returns correct fields.
func TestGetIssueMilestoneEvent_Success_DetailedFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/issues/1/resource_milestone_events/30" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{"id":30,"action":"add","created_at":"2026-03-01T08:00:00Z","resource_type":"Issue","resource_id":1,"user":{"id":5,"username":"alice"},"milestone":{"id":200,"title":"v1.0"}}`)
	}))

	out, err := GetIssueMilestoneEvent(context.Background(), client, GetIssueMilestoneEventInput{ProjectID: "42", IssueIID: 1, MilestoneEventID: 30})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.MilestoneTitle != "v1.0" {
		t.Errorf("got milestone %q, want %q", out.MilestoneTitle, "v1.0")
	}
}

// TestListIssueStateEvents_Success_DetailedFields verifies ListIssueStateEvents returns correct fields.
func TestListIssueStateEvents_Success_DetailedFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/issues/1/resource_state_events" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"id":40,"state":"closed","created_at":"2026-04-01T14:00:00Z","resource_type":"Issue","resource_id":1,"user":{"id":5,"username":"alice"}}
		]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	}))

	out, err := ListIssueStateEvents(context.Background(), client, ListIssueStateEventsInput{ProjectID: "42", IssueIID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Events) != 1 {
		t.Fatalf(fmtWantOneEvent, len(out.Events))
	}
	if out.Events[0].State != "closed" {
		t.Errorf(fmtGotStateWant, out.Events[0].State, "closed")
	}
}

// TestGetIssueStateEvent_Success_DetailedFields verifies GetIssueStateEvent returns correct fields.
func TestGetIssueStateEvent_Success_DetailedFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/issues/1/resource_state_events/40" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{"id":40,"state":"closed","created_at":"2026-04-01T14:00:00Z","resource_type":"Issue","resource_id":1,"user":{"id":5,"username":"alice"}}`)
	}))

	out, err := GetIssueStateEvent(context.Background(), client, GetIssueStateEventInput{ProjectID: "42", IssueIID: 1, StateEventID: 40})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.State != "closed" {
		t.Errorf(fmtGotStateWant, out.State, "closed")
	}
}

// TestListMRStateEvents_Success_DetailedFields verifies ListMRStateEvents returns correct fields.
func TestListMRStateEvents_Success_DetailedFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/merge_requests/5/resource_state_events" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"id":50,"state":"merged","created_at":"2026-05-01T16:00:00Z","resource_type":"MergeRequest","resource_id":5,"user":{"id":6,"username":"bob"}}
		]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	}))

	out, err := ListMRStateEvents(context.Background(), client, ListMRStateEventsInput{ProjectID: "42", MRIID: 5})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Events) != 1 {
		t.Fatalf(fmtWantOneEvent, len(out.Events))
	}
	if out.Events[0].State != "merged" {
		t.Errorf(fmtGotStateWant, out.Events[0].State, "merged")
	}
}

// TestListIssueLabelEvents_APIError_Forbidden verifies ListIssueLabelEvents returns error on HTTP 403.
func TestListIssueLabelEvents_APIError_Forbidden(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	_, err := ListIssueLabelEvents(context.Background(), client, ListIssueLabelEventsInput{ProjectID: "42", IssueIID: 1})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestFormatLabelEventsMarkdown_Empty_NoEvents verifies FormatLabelEventsMarkdown with empty slice.
func TestFormatLabelEventsMarkdown_Empty_NoEvents(t *testing.T) {
	out := ListLabelEventsOutput{Events: []LabelEventOutput{}}
	md := FormatLabelEventsMarkdown(out)
	if md != "No label events found.\n" {
		t.Errorf(fmtGotWant, md, "No label events found.\n")
	}
}

// TestFormatMilestoneEventsMarkdown_Empty_NoEvents verifies FormatMilestoneEventsMarkdown with empty slice.
func TestFormatMilestoneEventsMarkdown_Empty_NoEvents(t *testing.T) {
	out := ListMilestoneEventsOutput{Events: []MilestoneEventOutput{}}
	md := FormatMilestoneEventsMarkdown(out)
	if md != "No milestone events found.\n" {
		t.Errorf(fmtGotWant, md, "No milestone events found.\n")
	}
}

// TestFormatStateEventsMarkdown_Empty_NoEvents verifies FormatStateEventsMarkdown with empty slice.
func TestFormatStateEventsMarkdown_Empty_NoEvents(t *testing.T) {
	out := ListStateEventsOutput{Events: []StateEventOutput{}}
	md := FormatStateEventsMarkdown(out)
	if md != "No state events found.\n" {
		t.Errorf(fmtGotWant, md, "No state events found.\n")
	}
}

// Int64 validation tests.

// assertErrContains is an internal helper for the resourceevents package.
func assertErrContains(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", substr)
	}
	if !contains(err.Error(), substr) {
		t.Errorf("error %q does not contain %q", err.Error(), substr)
	}
}

// contains is an internal helper for the resourceevents package.
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestListIssueLabelEvents_InvalidIID verifies the behavior of list issue label events invalid i i d.
func TestListIssueLabelEvents_InvalidIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := ListIssueLabelEvents(context.Background(), client, ListIssueLabelEventsInput{ProjectID: "p", IssueIID: 0})
	assertErrContains(t, err, "issue_iid")
}

// TestGetIssueLabelEvent_InvalidIDs verifies the behavior of get issue label event invalid i ds.
func TestGetIssueLabelEvent_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := GetIssueLabelEvent(context.Background(), client, GetIssueLabelEventInput{ProjectID: "p", IssueIID: 0, LabelEventID: 1})
	assertErrContains(t, err, "issue_iid")
	_, err = GetIssueLabelEvent(context.Background(), client, GetIssueLabelEventInput{ProjectID: "p", IssueIID: 1, LabelEventID: 0})
	assertErrContains(t, err, "label_event_id")
}

// TestListIssueMilestoneEvents_InvalidIID verifies the behavior of list issue milestone events invalid i i d.
func TestListIssueMilestoneEvents_InvalidIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := ListIssueMilestoneEvents(context.Background(), client, ListIssueMilestoneEventsInput{ProjectID: "p", IssueIID: 0})
	assertErrContains(t, err, "issue_iid")
}

// TestGetIssueMilestoneEvent_InvalidIDs verifies the behavior of get issue milestone event invalid i ds.
func TestGetIssueMilestoneEvent_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := GetIssueMilestoneEvent(context.Background(), client, GetIssueMilestoneEventInput{ProjectID: "p", IssueIID: 0, MilestoneEventID: 1})
	assertErrContains(t, err, "issue_iid")
	_, err = GetIssueMilestoneEvent(context.Background(), client, GetIssueMilestoneEventInput{ProjectID: "p", IssueIID: 1, MilestoneEventID: 0})
	assertErrContains(t, err, "milestone_event_id")
}

// TestListIssueStateEvents_InvalidIID verifies the behavior of list issue state events invalid i i d.
func TestListIssueStateEvents_InvalidIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := ListIssueStateEvents(context.Background(), client, ListIssueStateEventsInput{ProjectID: "p", IssueIID: 0})
	assertErrContains(t, err, "issue_iid")
}

// TestGetIssueStateEvent_InvalidIDs verifies the behavior of get issue state event invalid i ds.
func TestGetIssueStateEvent_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := GetIssueStateEvent(context.Background(), client, GetIssueStateEventInput{ProjectID: "p", IssueIID: 0, StateEventID: 1})
	assertErrContains(t, err, "issue_iid")
	_, err = GetIssueStateEvent(context.Background(), client, GetIssueStateEventInput{ProjectID: "p", IssueIID: 1, StateEventID: 0})
	assertErrContains(t, err, "state_event_id")
}

// TestListMRLabelEvents_InvalidIID verifies the behavior of list m r label events invalid i i d.
func TestListMRLabelEvents_InvalidIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := ListMRLabelEvents(context.Background(), client, ListMRLabelEventsInput{ProjectID: "p", MRIID: 0})
	assertErrContains(t, err, "merge_request_iid")
}

// TestGetMRLabelEvent_InvalidIDs verifies the behavior of get m r label event invalid i ds.
func TestGetMRLabelEvent_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := GetMRLabelEvent(context.Background(), client, GetMRLabelEventInput{ProjectID: "p", MRIID: 0, LabelEventID: 1})
	assertErrContains(t, err, "merge_request_iid")
	_, err = GetMRLabelEvent(context.Background(), client, GetMRLabelEventInput{ProjectID: "p", MRIID: 1, LabelEventID: 0})
	assertErrContains(t, err, "label_event_id")
}

// TestListMRMilestoneEvents_InvalidIID verifies the behavior of list m r milestone events invalid i i d.
func TestListMRMilestoneEvents_InvalidIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := ListMRMilestoneEvents(context.Background(), client, ListMRMilestoneEventsInput{ProjectID: "p", MRIID: 0})
	assertErrContains(t, err, "merge_request_iid")
}

// TestGetMRMilestoneEvent_InvalidIDs verifies the behavior of get m r milestone event invalid i ds.
func TestGetMRMilestoneEvent_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := GetMRMilestoneEvent(context.Background(), client, GetMRMilestoneEventInput{ProjectID: "p", MRIID: 0, MilestoneEventID: 1})
	assertErrContains(t, err, "merge_request_iid")
	_, err = GetMRMilestoneEvent(context.Background(), client, GetMRMilestoneEventInput{ProjectID: "p", MRIID: 1, MilestoneEventID: 0})
	assertErrContains(t, err, "milestone_event_id")
}

// TestListMRStateEvents_InvalidIID verifies the behavior of list m r state events invalid i i d.
func TestListMRStateEvents_InvalidIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := ListMRStateEvents(context.Background(), client, ListMRStateEventsInput{ProjectID: "p", MRIID: 0})
	assertErrContains(t, err, "merge_request_iid")
}

// TestGetMRStateEvent_InvalidIDs verifies the behavior of get m r state event invalid i ds.
func TestGetMRStateEvent_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := GetMRStateEvent(context.Background(), client, GetMRStateEventInput{ProjectID: "p", MRIID: 0, StateEventID: 1})
	assertErrContains(t, err, "merge_request_iid")
	_, err = GetMRStateEvent(context.Background(), client, GetMRStateEventInput{ProjectID: "p", MRIID: 1, StateEventID: 0})
	assertErrContains(t, err, "state_event_id")
}

// ---------- Tests consolidated from coverage_test.go ----------.

// Mock JSON responses.
const (
	errExpectedValidation = "expected validation error"
	fmtUnexpErr           = "unexpected error: %v"
	covLabelEventJSON     = `[{"id":10,"action":"add","created_at":"2026-01-15T10:00:00Z","resource_type":"Issue","resource_id":1,"user":{"id":5,"username":"alice"},"label":{"id":100,"name":"bug","color":"#f00","text_color":"#fff","description":"Bug label"}}]`
	covLabelEventSingle   = `{"id":10,"action":"add","created_at":"2026-01-15T10:00:00Z","resource_type":"Issue","resource_id":1,"user":{"id":5,"username":"alice"},"label":{"id":100,"name":"bug","color":"#f00","text_color":"#fff","description":"Bug label"}}`
	covMilestoneEventJSON = `[{"id":30,"action":"add","created_at":"2026-03-01T08:00:00Z","resource_type":"Issue","resource_id":1,"user":{"id":5,"username":"alice"},"milestone":{"id":200,"title":"v1.0"}}]`
	covMilestoneSingle    = `{"id":30,"action":"add","created_at":"2026-03-01T08:00:00Z","resource_type":"Issue","resource_id":1,"user":{"id":5,"username":"alice"},"milestone":{"id":200,"title":"v1.0"}}`
	covStateEventJSON     = `[{"id":40,"state":"closed","created_at":"2026-04-01T14:00:00Z","resource_type":"Issue","resource_id":1,"user":{"id":5,"username":"alice"}}]`
	covStateSingle        = `{"id":40,"state":"closed","created_at":"2026-04-01T14:00:00Z","resource_type":"Issue","resource_id":1,"user":{"id":5,"username":"alice"}}`
)

// covPID is an internal helper for the resourceevents package.
func covPID() toolutil.StringOrInt { return toolutil.StringOrInt("42") }

// covBadHandler is an internal helper for the resourceevents package.
func covBadHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad"}`)
	})
}

// ======================== Label Events ========================.

// TestListIssueLabelEvents_Validation verifies the behavior of cov list issue label events validation.
func TestListIssueLabelEvents_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListIssueLabelEvents(t.Context(), client, ListIssueLabelEventsInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestListIssueLabelEvents_APIError verifies the behavior of cov list issue label events a p i error.
func TestListIssueLabelEvents_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListIssueLabelEvents(t.Context(), client, ListIssueLabelEventsInput{ProjectID: covPID(), IssueIID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestListIssueLabelEvents_Success verifies the behavior of cov list issue label events success.
func TestListIssueLabelEvents_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covLabelEventJSON)
	}))
	out, err := ListIssueLabelEvents(t.Context(), client, ListIssueLabelEventsInput{ProjectID: covPID(), IssueIID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Events) != 1 || out.Events[0].ID != 10 {
		t.Errorf("unexpected output: %+v", out)
	}
}

// TestGetIssueLabelEvent_Validation verifies the behavior of cov get issue label event validation.
func TestGetIssueLabelEvent_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetIssueLabelEvent(t.Context(), client, GetIssueLabelEventInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestGetIssueLabelEvent_APIError verifies the behavior of cov get issue label event a p i error.
func TestGetIssueLabelEvent_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetIssueLabelEvent(t.Context(), client, GetIssueLabelEventInput{ProjectID: covPID(), IssueIID: 1, LabelEventID: 10})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestGetIssueLabelEvent_Success verifies the behavior of cov get issue label event success.
func TestGetIssueLabelEvent_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covLabelEventSingle)
	}))
	out, err := GetIssueLabelEvent(t.Context(), client, GetIssueLabelEventInput{ProjectID: covPID(), IssueIID: 1, LabelEventID: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 10 || out.Label.Name != "bug" {
		t.Errorf("unexpected output: %+v", out)
	}
}

// TestListMRLabelEvents_Validation verifies the behavior of cov list m r label events validation.
func TestListMRLabelEvents_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListMRLabelEvents(t.Context(), client, ListMRLabelEventsInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestListMRLabelEvents_APIError verifies the behavior of cov list m r label events a p i error.
func TestListMRLabelEvents_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListMRLabelEvents(t.Context(), client, ListMRLabelEventsInput{ProjectID: covPID(), MRIID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestListMRLabelEvents_Success verifies the behavior of cov list m r label events success.
func TestListMRLabelEvents_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covLabelEventJSON)
	}))
	out, err := ListMRLabelEvents(t.Context(), client, ListMRLabelEventsInput{ProjectID: covPID(), MRIID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Events) != 1 {
		t.Error("expected 1 event")
	}
}

// TestGetMRLabelEvent_Validation verifies the behavior of cov get m r label event validation.
func TestGetMRLabelEvent_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetMRLabelEvent(t.Context(), client, GetMRLabelEventInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestGetMRLabelEvent_APIError verifies the behavior of cov get m r label event a p i error.
func TestGetMRLabelEvent_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetMRLabelEvent(t.Context(), client, GetMRLabelEventInput{ProjectID: covPID(), MRIID: 1, LabelEventID: 10})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestGetMRLabelEvent_Success verifies the behavior of cov get m r label event success.
func TestGetMRLabelEvent_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covLabelEventSingle)
	}))
	out, err := GetMRLabelEvent(t.Context(), client, GetMRLabelEventInput{ProjectID: covPID(), MRIID: 1, LabelEventID: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 10 {
		t.Error("unexpected ID")
	}
}

// ======================== Milestone Events ========================.

// TestListIssueMilestoneEvents_Validation verifies the behavior of cov list issue milestone events validation.
func TestListIssueMilestoneEvents_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListIssueMilestoneEvents(t.Context(), client, ListIssueMilestoneEventsInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestListIssueMilestoneEvents_APIError verifies the behavior of cov list issue milestone events a p i error.
func TestListIssueMilestoneEvents_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListIssueMilestoneEvents(t.Context(), client, ListIssueMilestoneEventsInput{ProjectID: covPID(), IssueIID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestListIssueMilestoneEvents_Success verifies the behavior of cov list issue milestone events success.
func TestListIssueMilestoneEvents_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covMilestoneEventJSON)
	}))
	out, err := ListIssueMilestoneEvents(t.Context(), client, ListIssueMilestoneEventsInput{ProjectID: covPID(), IssueIID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Events) != 1 || out.Events[0].ID != 30 {
		t.Errorf("unexpected output: %+v", out)
	}
}

// TestGetIssueMilestoneEvent_Validation verifies the behavior of cov get issue milestone event validation.
func TestGetIssueMilestoneEvent_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetIssueMilestoneEvent(t.Context(), client, GetIssueMilestoneEventInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestGetIssueMilestoneEvent_APIError verifies the behavior of cov get issue milestone event a p i error.
func TestGetIssueMilestoneEvent_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetIssueMilestoneEvent(t.Context(), client, GetIssueMilestoneEventInput{ProjectID: covPID(), IssueIID: 1, MilestoneEventID: 30})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestGetIssueMilestoneEvent_Success verifies the behavior of cov get issue milestone event success.
func TestGetIssueMilestoneEvent_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covMilestoneSingle)
	}))
	out, err := GetIssueMilestoneEvent(t.Context(), client, GetIssueMilestoneEventInput{ProjectID: covPID(), IssueIID: 1, MilestoneEventID: 30})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.MilestoneTitle != "v1.0" {
		t.Errorf("expected v1.0, got %q", out.MilestoneTitle)
	}
}

// TestListMRMilestoneEvents_Validation verifies the behavior of cov list m r milestone events validation.
func TestListMRMilestoneEvents_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListMRMilestoneEvents(t.Context(), client, ListMRMilestoneEventsInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestListMRMilestoneEvents_APIError verifies the behavior of cov list m r milestone events a p i error.
func TestListMRMilestoneEvents_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListMRMilestoneEvents(t.Context(), client, ListMRMilestoneEventsInput{ProjectID: covPID(), MRIID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestListMRMilestoneEvents_Success verifies the behavior of cov list m r milestone events success.
func TestListMRMilestoneEvents_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covMilestoneEventJSON)
	}))
	out, err := ListMRMilestoneEvents(t.Context(), client, ListMRMilestoneEventsInput{ProjectID: covPID(), MRIID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Events) != 1 {
		t.Error("expected 1 event")
	}
}

// TestGetMRMilestoneEvent_Validation verifies the behavior of cov get m r milestone event validation.
func TestGetMRMilestoneEvent_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetMRMilestoneEvent(t.Context(), client, GetMRMilestoneEventInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestGetMRMilestoneEvent_APIError verifies the behavior of cov get m r milestone event a p i error.
func TestGetMRMilestoneEvent_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetMRMilestoneEvent(t.Context(), client, GetMRMilestoneEventInput{ProjectID: covPID(), MRIID: 1, MilestoneEventID: 30})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestGetMRMilestoneEvent_Success verifies the behavior of cov get m r milestone event success.
func TestGetMRMilestoneEvent_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covMilestoneSingle)
	}))
	out, err := GetMRMilestoneEvent(t.Context(), client, GetMRMilestoneEventInput{ProjectID: covPID(), MRIID: 1, MilestoneEventID: 30})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 30 {
		t.Error("unexpected id")
	}
}

// ======================== State Events ========================.

// TestListIssueStateEvents_Validation verifies the behavior of cov list issue state events validation.
func TestListIssueStateEvents_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListIssueStateEvents(t.Context(), client, ListIssueStateEventsInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestListIssueStateEvents_APIError verifies the behavior of cov list issue state events a p i error.
func TestListIssueStateEvents_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListIssueStateEvents(t.Context(), client, ListIssueStateEventsInput{ProjectID: covPID(), IssueIID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestListIssueStateEvents_Success verifies the behavior of cov list issue state events success.
func TestListIssueStateEvents_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covStateEventJSON)
	}))
	out, err := ListIssueStateEvents(t.Context(), client, ListIssueStateEventsInput{ProjectID: covPID(), IssueIID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Events) != 1 || out.Events[0].State != "closed" {
		t.Errorf("unexpected output: %+v", out)
	}
}

// TestGetIssueStateEvent_Validation verifies the behavior of cov get issue state event validation.
func TestGetIssueStateEvent_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetIssueStateEvent(t.Context(), client, GetIssueStateEventInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestGetIssueStateEvent_APIError verifies the behavior of cov get issue state event a p i error.
func TestGetIssueStateEvent_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetIssueStateEvent(t.Context(), client, GetIssueStateEventInput{ProjectID: covPID(), IssueIID: 1, StateEventID: 40})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestGetIssueStateEvent_Success verifies the behavior of cov get issue state event success.
func TestGetIssueStateEvent_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covStateSingle)
	}))
	out, err := GetIssueStateEvent(t.Context(), client, GetIssueStateEventInput{ProjectID: covPID(), IssueIID: 1, StateEventID: 40})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.State != "closed" {
		t.Errorf("expected closed, got %q", out.State)
	}
}

// TestListMRStateEvents_Validation verifies the behavior of cov list m r state events validation.
func TestListMRStateEvents_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListMRStateEvents(t.Context(), client, ListMRStateEventsInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestListMRStateEvents_APIError verifies the behavior of cov list m r state events a p i error.
func TestListMRStateEvents_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListMRStateEvents(t.Context(), client, ListMRStateEventsInput{ProjectID: covPID(), MRIID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestListMRStateEvents_Success verifies the behavior of cov list m r state events success.
func TestListMRStateEvents_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covStateEventJSON)
	}))
	out, err := ListMRStateEvents(t.Context(), client, ListMRStateEventsInput{ProjectID: covPID(), MRIID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Events) != 1 {
		t.Error("expected 1 event")
	}
}

// TestGetMRStateEvent_Validation verifies the behavior of cov get m r state event validation.
func TestGetMRStateEvent_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetMRStateEvent(t.Context(), client, GetMRStateEventInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestGetMRStateEvent_APIError verifies the behavior of cov get m r state event a p i error.
func TestGetMRStateEvent_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetMRStateEvent(t.Context(), client, GetMRStateEventInput{ProjectID: covPID(), MRIID: 1, StateEventID: 40})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestGetMRStateEvent_Success verifies the behavior of cov get m r state event success.
func TestGetMRStateEvent_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covStateSingle)
	}))
	out, err := GetMRStateEvent(t.Context(), client, GetMRStateEventInput{ProjectID: covPID(), MRIID: 1, StateEventID: 40})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 40 {
		t.Error("unexpected ID")
	}
}

// ======================== Converters: Edge cases ========================.

// TestCovtoLabelEventOutput_Nil verifies the behavior of covto label event output nil.
func TestCovtoLabelEventOutput_Nil(t *testing.T) {
	out := toLabelEventOutput(nil)
	if out.ID != 0 {
		t.Error("expected zero value for nil event")
	}
}

// TestCovtoMilestoneEventOutput_Nil verifies the behavior of covto milestone event output nil.
func TestCovtoMilestoneEventOutput_Nil(t *testing.T) {
	out := toMilestoneEventOutput(nil)
	if out.ID != 0 {
		t.Error("expected zero value for nil event")
	}
}

// TestCovtoMilestoneEventOutput_NilUserAndMilestone verifies the behavior of covto milestone event output nil user and milestone.
func TestCovtoMilestoneEventOutput_NilUserAndMilestone(t *testing.T) {
	e := &gl.MilestoneEvent{ID: 1, Action: "add"}
	out := toMilestoneEventOutput(e)
	if out.UserID != 0 || out.MilestoneTitle != "" {
		t.Error("expected zero values for nil user/milestone")
	}
}

// TestCovtoStateEventOutput_Nil verifies the behavior of covto state event output nil.
func TestCovtoStateEventOutput_Nil(t *testing.T) {
	out := toStateEventOutput(nil)
	if out.ID != 0 {
		t.Error("expected zero value for nil event")
	}
}

// TestCovtoStateEventOutput_NilUser verifies the behavior of covto state event output nil user.
func TestCovtoStateEventOutput_NilUser(t *testing.T) {
	e := &gl.StateEvent{ID: 1, State: "opened"}
	out := toStateEventOutput(e)
	if out.UserID != 0 {
		t.Error("expected zero UserID for nil user")
	}
	if out.State != "opened" {
		t.Errorf("expected opened, got %q", out.State)
	}
}

// ======================== Formatters ========================.

// TestFormatLabelEventsMarkdown_Empty verifies the behavior of cov format label events markdown empty.
func TestFormatLabelEventsMarkdown_Empty(t *testing.T) {
	md := FormatLabelEventsMarkdown(ListLabelEventsOutput{})
	if !strings.Contains(md, "No label events found") {
		t.Error("expected empty label events message")
	}
}

// TestFormatLabelEventsMarkdown_WithEvents verifies the behavior of cov format label events markdown with events.
func TestFormatLabelEventsMarkdown_WithEvents(t *testing.T) {
	out := ListLabelEventsOutput{
		Events: []LabelEventOutput{{ID: 1, Action: "add", Label: LabelEventLabelOutput{Name: "bug"}, Username: "alice"}},
	}
	md := FormatLabelEventsMarkdown(out)
	if !strings.Contains(md, "bug") || !strings.Contains(md, "alice") {
		t.Error("expected label and user in markdown")
	}
}

// TestFormatLabelEventMarkdown verifies the behavior of cov format label event markdown.
func TestFormatLabelEventMarkdown(t *testing.T) {
	out := LabelEventOutput{ID: 10, Action: "add", Label: LabelEventLabelOutput{Name: "bug"}, Username: "alice", ResourceType: "Issue", ResourceID: 1}
	md := FormatLabelEventMarkdown(out)
	if !strings.Contains(md, "Label Event #10") || !strings.Contains(md, "bug") {
		t.Error("expected label event details")
	}
}

// TestFormatMilestoneEventsMarkdown_Empty verifies the behavior of cov format milestone events markdown empty.
func TestFormatMilestoneEventsMarkdown_Empty(t *testing.T) {
	md := FormatMilestoneEventsMarkdown(ListMilestoneEventsOutput{})
	if !strings.Contains(md, "No milestone events found") {
		t.Error("expected empty milestone events message")
	}
}

// TestFormatMilestoneEventsMarkdown_WithEvents verifies the behavior of cov format milestone events markdown with events.
func TestFormatMilestoneEventsMarkdown_WithEvents(t *testing.T) {
	out := ListMilestoneEventsOutput{
		Events: []MilestoneEventOutput{{ID: 1, Action: "add", MilestoneTitle: "v1.0", Username: "alice"}},
	}
	md := FormatMilestoneEventsMarkdown(out)
	if !strings.Contains(md, "v1.0") || !strings.Contains(md, "alice") {
		t.Error("expected milestone and user in markdown")
	}
}

// TestFormatMilestoneEventMarkdown verifies the behavior of cov format milestone event markdown.
func TestFormatMilestoneEventMarkdown(t *testing.T) {
	out := MilestoneEventOutput{ID: 30, Action: "add", MilestoneTitle: "v1.0", MilestoneID: 200, Username: "alice", ResourceType: "Issue", ResourceID: 1}
	md := FormatMilestoneEventMarkdown(out)
	if !strings.Contains(md, "Milestone Event #30") || !strings.Contains(md, "v1.0") {
		t.Error("expected milestone event details")
	}
}

// TestFormatStateEventsMarkdown_Empty verifies the behavior of cov format state events markdown empty.
func TestFormatStateEventsMarkdown_Empty(t *testing.T) {
	md := FormatStateEventsMarkdown(ListStateEventsOutput{})
	if !strings.Contains(md, "No state events found") {
		t.Error("expected empty state events message")
	}
}

// TestFormatStateEventsMarkdown_WithEvents verifies the behavior of cov format state events markdown with events.
func TestFormatStateEventsMarkdown_WithEvents(t *testing.T) {
	out := ListStateEventsOutput{
		Events: []StateEventOutput{{ID: 1, State: "closed", Username: "alice", ResourceType: "Issue", ResourceID: 1}},
	}
	md := FormatStateEventsMarkdown(out)
	if !strings.Contains(md, "closed") || !strings.Contains(md, "alice") {
		t.Error("expected state and user in markdown")
	}
}

// TestFormatStateEventMarkdown verifies the behavior of cov format state event markdown.
func TestFormatStateEventMarkdown(t *testing.T) {
	out := StateEventOutput{ID: 40, State: "closed", Username: "alice", ResourceType: "Issue", ResourceID: 1}
	md := FormatStateEventMarkdown(out)
	if !strings.Contains(md, "State Event #40") || !strings.Contains(md, "closed") {
		t.Error("expected state event details")
	}
}

// ======================== Register ========================.

// TestRegisterTools_NoPanic verifies the behavior of cov register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, covBadHandler())
	RegisterTools(server, client)
}

// TestRegisterMeta_NoPanic verifies the behavior of cov register meta no panic.
func TestRegisterMeta_NoPanic(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, covBadHandler())
	RegisterMeta(server, client)
}

// ======================== MCP Round-trip ========================.

// TestMCPRound_Trip validates cov m c p round trip across multiple scenarios using table-driven subtests.
func TestMCPRound_Trip(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.Contains(path, "label"):
			if strings.Contains(path, "events/") {
				testutil.RespondJSON(w, http.StatusOK, covLabelEventSingle)
			} else {
				testutil.RespondJSON(w, http.StatusOK, covLabelEventJSON)
			}
		case strings.Contains(path, "milestone"):
			if strings.Contains(path, "events/") {
				testutil.RespondJSON(w, http.StatusOK, covMilestoneSingle)
			} else {
				testutil.RespondJSON(w, http.StatusOK, covMilestoneEventJSON)
			}
		case strings.Contains(path, "state"):
			if strings.Contains(path, "events/") {
				testutil.RespondJSON(w, http.StatusOK, covStateSingle)
			} else {
				testutil.RespondJSON(w, http.StatusOK, covStateEventJSON)
			}
		default:
			testutil.RespondJSON(w, http.StatusOK, `[]`)
		}
	})

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, mux)
	RegisterTools(server, client)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	go server.Connect(ctx, st, nil)

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	tests := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_issue_label_event_list", map[string]any{"project_id": "42", "issue_iid": 1}},
		{"gitlab_issue_label_event_get", map[string]any{"project_id": "42", "issue_iid": 1, "label_event_id": 10}},
		{"gitlab_mr_label_event_list", map[string]any{"project_id": "42", "merge_request_iid": 1}},
		{"gitlab_mr_label_event_get", map[string]any{"project_id": "42", "merge_request_iid": 1, "label_event_id": 10}},
		{"gitlab_issue_milestone_event_list", map[string]any{"project_id": "42", "issue_iid": 1}},
		{"gitlab_issue_milestone_event_get", map[string]any{"project_id": "42", "issue_iid": 1, "milestone_event_id": 30}},
		{"gitlab_mr_milestone_event_list", map[string]any{"project_id": "42", "merge_request_iid": 1}},
		{"gitlab_mr_milestone_event_get", map[string]any{"project_id": "42", "merge_request_iid": 1, "milestone_event_id": 30}},
		{"gitlab_issue_state_event_list", map[string]any{"project_id": "42", "issue_iid": 1}},
		{"gitlab_issue_state_event_get", map[string]any{"project_id": "42", "issue_iid": 1, "state_event_id": 40}},
		{"gitlab_mr_state_event_list", map[string]any{"project_id": "42", "merge_request_iid": 1}},
		{"gitlab_mr_state_event_get", map[string]any{"project_id": "42", "merge_request_iid": 1, "state_event_id": 40}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var res *mcp.CallToolResult
			res, err = session.CallTool(ctx, &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
			if err != nil {
				t.Fatalf("CallTool %s: %v", tc.name, err)
			}
			if res == nil {
				t.Fatalf("nil result for %s", tc.name)
			}
		})
	}
}
