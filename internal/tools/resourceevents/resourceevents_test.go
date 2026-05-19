// resourceevents_test.go contains unit tests for the resource event MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package resourceevents

import (
	"context"
	"net/http"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	// errNoReachAPI identifies the err no reach API constant used by this package.
	errNoReachAPI = "should not reach API"
	// fmtWantOneEvent identifies the fmt want one event constant used by this package.
	fmtWantOneEvent = "got %d events, want 1"
	// fmtGotStateWant identifies the fmt got state want constant used by this package.
	fmtGotStateWant = "got state %q, want %q"
	// fmtGotWant identifies the fmt got want constant used by this package.
	fmtGotWant = "got %q, want %q"
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

// TestListIssueLabelEvents_ValidationError verifies ListIssueLabelEvents when validation error.
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

// assertErrContains checks err contains invariants for tests.
func assertErrContains(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", substr)
	}
	if !contains(err.Error(), substr) {
		t.Errorf("error %q does not contain %q", err.Error(), substr)
	}
}

// contains reports whether contains.
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestListIssueLabelEvents_InvalidIID verifies ListIssueLabelEvents when invalid IID.
func TestListIssueLabelEvents_InvalidIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := ListIssueLabelEvents(context.Background(), client, ListIssueLabelEventsInput{ProjectID: "p", IssueIID: 0})
	assertErrContains(t, err, "issue_iid")
}

// TestGetIssueLabelEvent_InvalidIDs verifies GetIssueLabelEvent when invalid IDs.
func TestGetIssueLabelEvent_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := GetIssueLabelEvent(context.Background(), client, GetIssueLabelEventInput{ProjectID: "p", IssueIID: 0, LabelEventID: 1})
	assertErrContains(t, err, "issue_iid")
	_, err = GetIssueLabelEvent(context.Background(), client, GetIssueLabelEventInput{ProjectID: "p", IssueIID: 1, LabelEventID: 0})
	assertErrContains(t, err, "label_event_id")
}

// TestListIssueMilestoneEvents_InvalidIID verifies ListIssueMilestoneEvents when invalid IID.
func TestListIssueMilestoneEvents_InvalidIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := ListIssueMilestoneEvents(context.Background(), client, ListIssueMilestoneEventsInput{ProjectID: "p", IssueIID: 0})
	assertErrContains(t, err, "issue_iid")
}

// TestGetIssueMilestoneEvent_InvalidIDs verifies GetIssueMilestoneEvent when invalid IDs.
func TestGetIssueMilestoneEvent_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := GetIssueMilestoneEvent(context.Background(), client, GetIssueMilestoneEventInput{ProjectID: "p", IssueIID: 0, MilestoneEventID: 1})
	assertErrContains(t, err, "issue_iid")
	_, err = GetIssueMilestoneEvent(context.Background(), client, GetIssueMilestoneEventInput{ProjectID: "p", IssueIID: 1, MilestoneEventID: 0})
	assertErrContains(t, err, "milestone_event_id")
}

// TestListIssueStateEvents_InvalidIID verifies ListIssueStateEvents when invalid IID.
func TestListIssueStateEvents_InvalidIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := ListIssueStateEvents(context.Background(), client, ListIssueStateEventsInput{ProjectID: "p", IssueIID: 0})
	assertErrContains(t, err, "issue_iid")
}

// TestGetIssueStateEvent_InvalidIDs verifies GetIssueStateEvent when invalid IDs.
func TestGetIssueStateEvent_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := GetIssueStateEvent(context.Background(), client, GetIssueStateEventInput{ProjectID: "p", IssueIID: 0, StateEventID: 1})
	assertErrContains(t, err, "issue_iid")
	_, err = GetIssueStateEvent(context.Background(), client, GetIssueStateEventInput{ProjectID: "p", IssueIID: 1, StateEventID: 0})
	assertErrContains(t, err, "state_event_id")
}

// TestListMRLabelEvents_InvalidIID verifies ListMRLabelEvents when invalid IID.
func TestListMRLabelEvents_InvalidIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := ListMRLabelEvents(context.Background(), client, ListMRLabelEventsInput{ProjectID: "p", MRIID: 0})
	assertErrContains(t, err, "merge_request_iid")
}

// TestGetMRLabelEvent_InvalidIDs verifies GetMRLabelEvent when invalid IDs.
func TestGetMRLabelEvent_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := GetMRLabelEvent(context.Background(), client, GetMRLabelEventInput{ProjectID: "p", MRIID: 0, LabelEventID: 1})
	assertErrContains(t, err, "merge_request_iid")
	_, err = GetMRLabelEvent(context.Background(), client, GetMRLabelEventInput{ProjectID: "p", MRIID: 1, LabelEventID: 0})
	assertErrContains(t, err, "label_event_id")
}

// TestListMRMilestoneEvents_InvalidIID verifies ListMRMilestoneEvents when invalid IID.
func TestListMRMilestoneEvents_InvalidIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := ListMRMilestoneEvents(context.Background(), client, ListMRMilestoneEventsInput{ProjectID: "p", MRIID: 0})
	assertErrContains(t, err, "merge_request_iid")
}

// TestGetMRMilestoneEvent_InvalidIDs verifies GetMRMilestoneEvent when invalid IDs.
func TestGetMRMilestoneEvent_InvalidIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := GetMRMilestoneEvent(context.Background(), client, GetMRMilestoneEventInput{ProjectID: "p", MRIID: 0, MilestoneEventID: 1})
	assertErrContains(t, err, "merge_request_iid")
	_, err = GetMRMilestoneEvent(context.Background(), client, GetMRMilestoneEventInput{ProjectID: "p", MRIID: 1, MilestoneEventID: 0})
	assertErrContains(t, err, "milestone_event_id")
}

// TestListMRStateEvents_InvalidIID verifies ListMRStateEvents when invalid IID.
func TestListMRStateEvents_InvalidIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))
	_, err := ListMRStateEvents(context.Background(), client, ListMRStateEventsInput{ProjectID: "p", MRIID: 0})
	assertErrContains(t, err, "merge_request_iid")
}

// TestGetMRStateEvent_InvalidIDs verifies GetMRStateEvent when invalid IDs.
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

// covPID supports cov pid assertions in resourceevents tests.
func covPID() toolutil.StringOrInt { return toolutil.StringOrInt("42") }

// covBadHandler supports cov bad handler assertions in resourceevents tests.
func covBadHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad"}`)
	})
}

// ======================== Label Events ========================.

// TestListIssueLabelEvents_Validation verifies ListIssueLabelEvents when validation.
func TestListIssueLabelEvents_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListIssueLabelEvents(t.Context(), client, ListIssueLabelEventsInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestListIssueLabelEvents_APIError verifies ListIssueLabelEvents when API error.
func TestListIssueLabelEvents_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListIssueLabelEvents(t.Context(), client, ListIssueLabelEventsInput{ProjectID: covPID(), IssueIID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestListIssueLabelEvents_Success verifies ListIssueLabelEvents when success.
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

// TestGetIssueLabelEvent_Validation verifies GetIssueLabelEvent when validation.
func TestGetIssueLabelEvent_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetIssueLabelEvent(t.Context(), client, GetIssueLabelEventInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestGetIssueLabelEvent_APIError verifies GetIssueLabelEvent when API error.
func TestGetIssueLabelEvent_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetIssueLabelEvent(t.Context(), client, GetIssueLabelEventInput{ProjectID: covPID(), IssueIID: 1, LabelEventID: 10})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestGetIssueLabelEvent_Success verifies GetIssueLabelEvent when success.
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

// TestListMRLabelEvents_Validation verifies ListMRLabelEvents when validation.
func TestListMRLabelEvents_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListMRLabelEvents(t.Context(), client, ListMRLabelEventsInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestListMRLabelEvents_APIError verifies ListMRLabelEvents when API error.
func TestListMRLabelEvents_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListMRLabelEvents(t.Context(), client, ListMRLabelEventsInput{ProjectID: covPID(), MRIID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestListMRLabelEvents_Success verifies ListMRLabelEvents when success.
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

// TestGetMRLabelEvent_Validation verifies GetMRLabelEvent when validation.
func TestGetMRLabelEvent_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetMRLabelEvent(t.Context(), client, GetMRLabelEventInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestGetMRLabelEvent_APIError verifies GetMRLabelEvent when API error.
func TestGetMRLabelEvent_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetMRLabelEvent(t.Context(), client, GetMRLabelEventInput{ProjectID: covPID(), MRIID: 1, LabelEventID: 10})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestGetMRLabelEvent_Success verifies GetMRLabelEvent when success.
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

// TestListIssueMilestoneEvents_Validation verifies ListIssueMilestoneEvents when validation.
func TestListIssueMilestoneEvents_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListIssueMilestoneEvents(t.Context(), client, ListIssueMilestoneEventsInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestListIssueMilestoneEvents_APIError verifies ListIssueMilestoneEvents when API error.
func TestListIssueMilestoneEvents_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListIssueMilestoneEvents(t.Context(), client, ListIssueMilestoneEventsInput{ProjectID: covPID(), IssueIID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestListIssueMilestoneEvents_Success verifies ListIssueMilestoneEvents when success.
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

// TestGetIssueMilestoneEvent_Validation verifies GetIssueMilestoneEvent when validation.
func TestGetIssueMilestoneEvent_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetIssueMilestoneEvent(t.Context(), client, GetIssueMilestoneEventInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestGetIssueMilestoneEvent_APIError verifies GetIssueMilestoneEvent when API error.
func TestGetIssueMilestoneEvent_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetIssueMilestoneEvent(t.Context(), client, GetIssueMilestoneEventInput{ProjectID: covPID(), IssueIID: 1, MilestoneEventID: 30})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestGetIssueMilestoneEvent_Success verifies GetIssueMilestoneEvent when success.
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

// TestListMRMilestoneEvents_Validation verifies ListMRMilestoneEvents when validation.
func TestListMRMilestoneEvents_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListMRMilestoneEvents(t.Context(), client, ListMRMilestoneEventsInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestListMRMilestoneEvents_APIError verifies ListMRMilestoneEvents when API error.
func TestListMRMilestoneEvents_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListMRMilestoneEvents(t.Context(), client, ListMRMilestoneEventsInput{ProjectID: covPID(), MRIID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestListMRMilestoneEvents_Success verifies ListMRMilestoneEvents when success.
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

// TestGetMRMilestoneEvent_Validation verifies GetMRMilestoneEvent when validation.
func TestGetMRMilestoneEvent_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetMRMilestoneEvent(t.Context(), client, GetMRMilestoneEventInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestGetMRMilestoneEvent_APIError verifies GetMRMilestoneEvent when API error.
func TestGetMRMilestoneEvent_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetMRMilestoneEvent(t.Context(), client, GetMRMilestoneEventInput{ProjectID: covPID(), MRIID: 1, MilestoneEventID: 30})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestGetMRMilestoneEvent_Success verifies GetMRMilestoneEvent when success.
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

// TestListIssueStateEvents_Validation verifies ListIssueStateEvents when validation.
func TestListIssueStateEvents_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListIssueStateEvents(t.Context(), client, ListIssueStateEventsInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestListIssueStateEvents_APIError verifies ListIssueStateEvents when API error.
func TestListIssueStateEvents_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListIssueStateEvents(t.Context(), client, ListIssueStateEventsInput{ProjectID: covPID(), IssueIID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestListIssueStateEvents_Success verifies ListIssueStateEvents when success.
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

// TestGetIssueStateEvent_Validation verifies GetIssueStateEvent when validation.
func TestGetIssueStateEvent_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetIssueStateEvent(t.Context(), client, GetIssueStateEventInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestGetIssueStateEvent_APIError verifies GetIssueStateEvent when API error.
func TestGetIssueStateEvent_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetIssueStateEvent(t.Context(), client, GetIssueStateEventInput{ProjectID: covPID(), IssueIID: 1, StateEventID: 40})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestGetIssueStateEvent_Success verifies GetIssueStateEvent when success.
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

// TestListMRStateEvents_Validation verifies ListMRStateEvents when validation.
func TestListMRStateEvents_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListMRStateEvents(t.Context(), client, ListMRStateEventsInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestListMRStateEvents_APIError verifies ListMRStateEvents when API error.
func TestListMRStateEvents_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := ListMRStateEvents(t.Context(), client, ListMRStateEventsInput{ProjectID: covPID(), MRIID: 1})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestListMRStateEvents_Success verifies ListMRStateEvents when success.
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

// TestGetMRStateEvent_Validation verifies GetMRStateEvent when validation.
func TestGetMRStateEvent_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetMRStateEvent(t.Context(), client, GetMRStateEventInput{})
	if err == nil {
		t.Fatal(errExpectedValidation)
	}
}

// TestGetMRStateEvent_APIError verifies GetMRStateEvent when API error.
func TestGetMRStateEvent_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, covBadHandler())
	_, err := GetMRStateEvent(t.Context(), client, GetMRStateEventInput{ProjectID: covPID(), MRIID: 1, StateEventID: 40})
	if err == nil {
		t.Fatal("expected API error")
	}
}

// TestGetMRStateEvent_Success verifies GetMRStateEvent when success.
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

// TestCovtoLabelEventOutput_Nil verifies CovtoLabelEventOutput when nil.
func TestCovtoLabelEventOutput_Nil(t *testing.T) {
	out := toLabelEventOutput(nil)
	if out.ID != 0 {
		t.Error("expected zero value for nil event")
	}
}

// TestCovtoMilestoneEventOutput_Nil verifies CovtoMilestoneEventOutput when nil.
func TestCovtoMilestoneEventOutput_Nil(t *testing.T) {
	out := toMilestoneEventOutput(nil)
	if out.ID != 0 {
		t.Error("expected zero value for nil event")
	}
}

// TestCovtoMilestoneEventOutput_NilUserAndMilestone verifies CovtoMilestoneEventOutput when nil user and milestone.
func TestCovtoMilestoneEventOutput_NilUserAndMilestone(t *testing.T) {
	e := &gl.MilestoneEvent{ID: 1, Action: "add"}
	out := toMilestoneEventOutput(e)
	if out.UserID != 0 || out.MilestoneTitle != "" {
		t.Error("expected zero values for nil user/milestone")
	}
}

// TestCovtoStateEventOutput_Nil verifies CovtoStateEventOutput when nil.
func TestCovtoStateEventOutput_Nil(t *testing.T) {
	out := toStateEventOutput(nil)
	if out.ID != 0 {
		t.Error("expected zero value for nil event")
	}
}

// TestCovtoStateEventOutput_NilUser verifies CovtoStateEventOutput when nil user.
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

// TestFormatLabelEventsMarkdown_Empty verifies FormatLabelEventsMarkdown when empty.
func TestFormatLabelEventsMarkdown_Empty(t *testing.T) {
	md := FormatLabelEventsMarkdown(ListLabelEventsOutput{})
	if !strings.Contains(md, "No label events found") {
		t.Error("expected empty label events message")
	}
}

// TestFormatLabelEventsMarkdown_WithEvents verifies FormatLabelEventsMarkdown when with events.
func TestFormatLabelEventsMarkdown_WithEvents(t *testing.T) {
	out := ListLabelEventsOutput{
		Events: []LabelEventOutput{{ID: 1, Action: "add", Label: LabelEventLabelOutput{Name: "bug"}, Username: "alice"}},
	}
	md := FormatLabelEventsMarkdown(out)
	if !strings.Contains(md, "bug") || !strings.Contains(md, "alice") {
		t.Error("expected label and user in markdown")
	}
}

// TestFormatLabelEventMarkdown verifies FormatLabelEventMarkdown.
func TestFormatLabelEventMarkdown(t *testing.T) {
	out := LabelEventOutput{ID: 10, Action: "add", Label: LabelEventLabelOutput{Name: "bug"}, Username: "alice", ResourceType: "Issue", ResourceID: 1}
	md := FormatLabelEventMarkdown(out)
	if !strings.Contains(md, "Label Event #10") || !strings.Contains(md, "bug") {
		t.Error("expected label event details")
	}
}

// TestFormatMilestoneEventsMarkdown_Empty verifies FormatMilestoneEventsMarkdown when empty.
func TestFormatMilestoneEventsMarkdown_Empty(t *testing.T) {
	md := FormatMilestoneEventsMarkdown(ListMilestoneEventsOutput{})
	if !strings.Contains(md, "No milestone events found") {
		t.Error("expected empty milestone events message")
	}
}

// TestFormatMilestoneEventsMarkdown_WithEvents verifies FormatMilestoneEventsMarkdown when with events.
func TestFormatMilestoneEventsMarkdown_WithEvents(t *testing.T) {
	out := ListMilestoneEventsOutput{
		Events: []MilestoneEventOutput{{ID: 1, Action: "add", MilestoneTitle: "v1.0", Username: "alice"}},
	}
	md := FormatMilestoneEventsMarkdown(out)
	if !strings.Contains(md, "v1.0") || !strings.Contains(md, "alice") {
		t.Error("expected milestone and user in markdown")
	}
}

// TestFormatMilestoneEventMarkdown verifies FormatMilestoneEventMarkdown.
func TestFormatMilestoneEventMarkdown(t *testing.T) {
	out := MilestoneEventOutput{ID: 30, Action: "add", MilestoneTitle: "v1.0", MilestoneID: 200, Username: "alice", ResourceType: "Issue", ResourceID: 1}
	md := FormatMilestoneEventMarkdown(out)
	if !strings.Contains(md, "Milestone Event #30") || !strings.Contains(md, "v1.0") {
		t.Error("expected milestone event details")
	}
}

// TestFormatStateEventsMarkdown_Empty verifies FormatStateEventsMarkdown when empty.
func TestFormatStateEventsMarkdown_Empty(t *testing.T) {
	md := FormatStateEventsMarkdown(ListStateEventsOutput{})
	if !strings.Contains(md, "No state events found") {
		t.Error("expected empty state events message")
	}
}

// TestFormatStateEventsMarkdown_WithEvents verifies FormatStateEventsMarkdown when with events.
func TestFormatStateEventsMarkdown_WithEvents(t *testing.T) {
	out := ListStateEventsOutput{
		Events: []StateEventOutput{{ID: 1, State: "closed", Username: "alice", ResourceType: "Issue", ResourceID: 1}},
	}
	md := FormatStateEventsMarkdown(out)
	if !strings.Contains(md, "closed") || !strings.Contains(md, "alice") {
		t.Error("expected state and user in markdown")
	}
}

// TestFormatStateEventMarkdown verifies FormatStateEventMarkdown.
func TestFormatStateEventMarkdown(t *testing.T) {
	out := StateEventOutput{ID: 40, State: "closed", Username: "alice", ResourceType: "Issue", ResourceID: 1}
	md := FormatStateEventMarkdown(out)
	if !strings.Contains(md, "State Event #40") || !strings.Contains(md, "closed") {
		t.Error("expected state event details")
	}
}

// ======================== ActionSpecs Route Round-trip ========================.

// TestActionSpecs_CoverageRoundTrip validates route calls across core resource event tools.
func TestActionSpecs_CoverageRoundTrip(t *testing.T) {
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

	client := testutil.NewTestClient(t, mux)
	byTool := resourceEventSpecsByTool(t, append(IssueActionSpecs(client), MergeRequestActionSpecs(client)...))

	tests := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_issue_label_event_list", map[string]any{"project_id": "42", "issue_iid": int64(1)}},
		{"gitlab_issue_label_event_get", map[string]any{"project_id": "42", "issue_iid": int64(1), "label_event_id": int64(10)}},
		{"gitlab_mr_label_event_list", map[string]any{"project_id": "42", "merge_request_iid": int64(1)}},
		{"gitlab_mr_label_event_get", map[string]any{"project_id": "42", "merge_request_iid": int64(1), "label_event_id": int64(10)}},
		{"gitlab_issue_milestone_event_list", map[string]any{"project_id": "42", "issue_iid": int64(1)}},
		{"gitlab_issue_milestone_event_get", map[string]any{"project_id": "42", "issue_iid": int64(1), "milestone_event_id": int64(30)}},
		{"gitlab_mr_milestone_event_list", map[string]any{"project_id": "42", "merge_request_iid": int64(1)}},
		{"gitlab_mr_milestone_event_get", map[string]any{"project_id": "42", "merge_request_iid": int64(1), "milestone_event_id": int64(30)}},
		{"gitlab_issue_state_event_list", map[string]any{"project_id": "42", "issue_iid": int64(1)}},
		{"gitlab_issue_state_event_get", map[string]any{"project_id": "42", "issue_iid": int64(1), "state_event_id": int64(40)}},
		{"gitlab_mr_state_event_list", map[string]any{"project_id": "42", "merge_request_iid": int64(1)}},
		{"gitlab_mr_state_event_get", map[string]any{"project_id": "42", "merge_request_iid": int64(1), "state_event_id": int64(40)}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := byTool[tc.name].Route.Handler(t.Context(), tc.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s): %v", tc.name, err)
			}
			if res == nil {
				t.Fatalf("nil result for %s", tc.name)
			}
		})
	}
}

// TestListIssueIterationEvents_Success verifies ListIssueIterationEvents returns correct fields.
func TestListIssueIterationEvents_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/issues/1/resource_iteration_events" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"id":1,"action":"add","created_at":"2026-01-15T10:00:00Z","resource_type":"Issue","resource_id":1,"user":{"id":5,"username":"alice"},"iteration":{"id":10,"iid":1,"sequence":1,"group_id":5,"title":"Sprint 1","state":3,"web_url":"https://gitlab.example.com/iterations/10"}}
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

// TestListIssueIterationEvents_MissingIssueIID verifies error when IssueIID is 0.
func TestListIssueIterationEvents_MissingIssueIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))
	_, err := ListIssueIterationEvents(context.Background(), client, ListIssueIterationEventsInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected validation error for missing issue_iid, got nil")
	}
}

// TestGetIssueIterationEvent_MissingProjectID verifies error when ProjectID is empty.
func TestGetIssueIterationEvent_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))
	_, err := GetIssueIterationEvent(context.Background(), client, GetIssueIterationEventInput{IssueIID: 1, IterationEventID: 1})
	if err == nil {
		t.Fatal("expected validation error for missing project_id, got nil")
	}
}

// TestGetIssueIterationEvent_MissingIssueIID verifies error when IssueIID is 0.
func TestGetIssueIterationEvent_MissingIssueIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))
	_, err := GetIssueIterationEvent(context.Background(), client, GetIssueIterationEventInput{ProjectID: "42", IterationEventID: 1})
	if err == nil {
		t.Fatal("expected validation error for missing issue_iid, got nil")
	}
}

// TestListIssueWeightEvents_MissingIssueIID verifies error when IssueIID is 0.
func TestListIssueWeightEvents_MissingIssueIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))
	_, err := ListIssueWeightEvents(context.Background(), client, ListIssueWeightEventsInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected validation error for missing issue_iid, got nil")
	}
}

// TestGetIssueIterationEvent_Success verifies GetIssueIterationEvent returns correct fields.
func TestGetIssueIterationEvent_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/issues/1/resource_iteration_events/1" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"action":"add","created_at":"2026-01-15T10:00:00Z","resource_type":"Issue","resource_id":1,"user":{"id":5,"username":"alice"},"iteration":{"id":10,"iid":1,"sequence":1,"group_id":5,"title":"Sprint 1","state":3,"web_url":"https://gitlab.example.com/iterations/10"}}`)
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
			{"id":1,"created_at":"2026-01-15T10:00:00Z","resource_type":"Issue","resource_id":1,"state":"weight_changed","issue_id":1,"weight":5,"user":{"id":5,"username":"alice"}}
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

// TestListIssueIterationEvents_APIError verifies ListIssueIterationEvents wraps API errors.
func TestListIssueIterationEvents_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))
	_, err := ListIssueIterationEvents(context.Background(), client, ListIssueIterationEventsInput{ProjectID: "42", IssueIID: 1})
	if err == nil {
		t.Fatal("expected error for 500, got nil")
	}
}

// TestGetIssueIterationEvent_APIError verifies GetIssueIterationEvent wraps API errors.
func TestGetIssueIterationEvent_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))
	_, err := GetIssueIterationEvent(context.Background(), client, GetIssueIterationEventInput{ProjectID: "42", IssueIID: 1, IterationEventID: 1})
	if err == nil {
		t.Fatal("expected error for 500, got nil")
	}
}

// TestListIssueWeightEvents_APIError verifies ListIssueWeightEvents wraps API errors.
func TestListIssueWeightEvents_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))
	_, err := ListIssueWeightEvents(context.Background(), client, ListIssueWeightEventsInput{ProjectID: "42", IssueIID: 1})
	if err == nil {
		t.Fatal("expected error for 500, got nil")
	}
}

// TestIterationEvent_WithStartDateAndDueDate verifies the converter handles
// iteration StartDate and DueDate fields.
func TestIterationEvent_WithStartDateAndDueDate(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{
			"id":1,"action":"add","created_at":"2026-01-15T10:00:00Z",
			"resource_type":"Issue","resource_id":1,
			"user":{"id":5,"username":"alice"},
			"iteration":{
				"id":10,"iid":1,"sequence":1,"group_id":5,
				"title":"Sprint 1","state":3,
				"start_date":"2026-01-01","due_date":"2026-01-14",
				"web_url":"https://gitlab.example.com/iterations/10"
			}
		}`)
	}))
	out, err := GetIssueIterationEvent(context.Background(), client, GetIssueIterationEventInput{
		ProjectID: "42", IssueIID: 1, IterationEventID: 1,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Iteration.StartDate == "" {
		t.Error("expected Iteration.StartDate to be set")
	}
	if out.Iteration.DueDate == "" {
		t.Error("expected Iteration.DueDate to be set")
	}
}

// TestToIterationEventOutput_Nil verifies that a nil IterationEvent input
// returns a zero-value IterationEventOutput without panicking.
func TestToIterationEventOutput_Nil(t *testing.T) {
	out := toIterationEventOutput(nil)
	if out.ID != 0 {
		t.Errorf("expected zero ID, got %d", out.ID)
	}
}

// TestToWeightEventOutput_Nil verifies that a nil WeightEvent input
// returns a zero-value WeightEventOutput without panicking.
func TestToWeightEventOutput_Nil(t *testing.T) {
	out := toWeightEventOutput(nil)
	if out.ID != 0 {
		t.Errorf("expected zero ID, got %d", out.ID)
	}
}
