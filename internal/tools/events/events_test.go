// events_test.go contains unit tests for the event MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package events

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

const (
	actionPushed    = "pushed"
	targetIssue     = "issue"
	titleBugReport  = "Bug Report"
	fmtUnexpErr     = "unexpected error: %v"
	testDateAfter   = "2026-06-01"
	testDateCreated = "2026-01-14"
)

// TestListProjectEvents_Success verifies the behavior of list project events success.
func TestListProjectEvents_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/events" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"id":1,"project_id":42,"action_name":"pushed","author_id":10,"author_username":"alice","created_at":"2026-01-15","target_type":"","target_iid":0},
			{"id":2,"project_id":42,"action_name":"commented","author_id":11,"author_username":"bob","created_at":"2026-01-14","target_type":"Note","target_iid":5,"target_title":"Fix bug"}
		]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "2", TotalPages: "1"})
	}))

	out, err := ListProjectEvents(context.Background(), client, ListProjectEventsInput{ProjectID: "42", Page: 1, PerPage: 20})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Events) != 2 {
		t.Fatalf("got %d events, want 2", len(out.Events))
	}
	if out.Events[0].ActionName != actionPushed {
		t.Errorf("got action %q, want %q", out.Events[0].ActionName, "pushed")
	}
	if out.Events[0].AuthorUsername != "alice" {
		t.Errorf("got author %q, want %q", out.Events[0].AuthorUsername, "alice")
	}
	if out.Events[1].TargetTitle != "Fix bug" {
		t.Errorf("got target_title %q, want %q", out.Events[1].TargetTitle, "Fix bug")
	}
	if out.Pagination.TotalItems != 2 {
		t.Errorf("got total %d, want 2", out.Pagination.TotalItems)
	}
}

// TestListProjectEvents_WithFilters verifies the behavior of list project events with filters.
func TestListProjectEvents_WithFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/events") {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		if q.Get("action") != actionPushed {
			t.Errorf("expected action=pushed, got %q", q.Get("action"))
		}
		if q.Get("target_type") != targetIssue {
			t.Errorf("expected target_type=issue, got %q", q.Get("target_type"))
		}
		if q.Get("before") != testDateAfter {
			t.Errorf("expected before=2026-06-01, got %q", q.Get("before"))
		}
		if q.Get("after") != "2026-01-01" {
			t.Errorf("expected after=2026-01-01, got %q", q.Get("after"))
		}
		if q.Get("sort") != "asc" {
			t.Errorf("expected sort=asc, got %q", q.Get("sort"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"id":10,"project_id":42,"action_name":"pushed","author_id":1,"author_username":"dev","created_at":"2026-03-01","target_type":"Issue","target_iid":7}
		]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	}))

	out, err := ListProjectEvents(context.Background(), client, ListProjectEventsInput{
		ProjectID:  "42",
		Action:     actionPushed,
		TargetType: targetIssue,
		Before:     testDateAfter,
		After:      "2026-01-01",
		Sort:       "asc",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Events) != 1 {
		t.Fatalf("got %d events, want 1", len(out.Events))
	}
	if out.Events[0].TargetType != "Issue" {
		t.Errorf("got target_type %q, want %q", out.Events[0].TargetType, "Issue")
	}
}

// TestListProjectEvents_ValidationError verifies the behavior of list project events validation error.
func TestListProjectEvents_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called for validation error")
	}))

	_, err := ListProjectEvents(context.Background(), client, ListProjectEventsInput{})
	if err == nil {
		t.Fatal("expected error for empty project_id, got nil")
	}
}

// TestListProjectEvents_APIError_Forbidden verifies ListProjectEvents returns an error on HTTP 403.
func TestListProjectEvents_APIError_Forbidden(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := ListProjectEvents(context.Background(), client, ListProjectEventsInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestListProjectEvents_EmptyResult verifies the behavior of list project events empty result.
func TestListProjectEvents_EmptyResult(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
	}))

	out, err := ListProjectEvents(context.Background(), client, ListProjectEventsInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Events) != 0 {
		t.Fatalf("got %d events, want 0", len(out.Events))
	}
}

// TestListCurrentUserContributionEvents_Success verifies the behavior of list current user contribution events success.
func TestListCurrentUserContributionEvents_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/events" {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"id":100,"title":"Pushed to main","project_id":5,"action_name":"pushed","target_id":0,"target_iid":0,"target_type":"","author_id":1,"target_title":"","created_at":"2026-06-01T10:00:00Z","author_username":"dev"},
			{"id":101,"title":"Opened issue","project_id":5,"action_name":"opened","target_id":42,"target_iid":7,"target_type":"Issue","author_id":1,"target_title":"Bug Report","created_at":"2026-06-02T11:30:00Z","author_username":"dev"}
		]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "2", TotalPages: "1"})
	}))

	out, err := ListCurrentUserContributionEvents(context.Background(), client, ListContributionEventsInput{Page: 1, PerPage: 20})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Events) != 2 {
		t.Fatalf("got %d events, want 2", len(out.Events))
	}
	if out.Events[0].ActionName != actionPushed {
		t.Errorf("got action %q, want %q", out.Events[0].ActionName, "pushed")
	}
	if out.Events[1].TargetType != "Issue" {
		t.Errorf("got target_type %q, want %q", out.Events[1].TargetType, "Issue")
	}
	if out.Events[1].TargetTitle != titleBugReport {
		t.Errorf("got target_title %q, want %q", out.Events[1].TargetTitle, titleBugReport)
	}
	if out.Pagination.TotalItems != 2 {
		t.Errorf("got total %d, want 2", out.Pagination.TotalItems)
	}
}

// TestListCurrentUserContributionEvents_WithFilters verifies the behavior of list current user contribution events with filters.
func TestListCurrentUserContributionEvents_WithFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/events") {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		if q.Get("action") != actionPushed {
			t.Errorf("expected action=pushed, got %q", q.Get("action"))
		}
		if q.Get("target_type") != targetIssue {
			t.Errorf("expected target_type=issue, got %q", q.Get("target_type"))
		}
		if q.Get("scope") != "all" {
			t.Errorf("expected scope=all, got %q", q.Get("scope"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"id":200,"title":"Opened issue","project_id":9,"action_name":"pushed","target_id":1,"target_iid":3,"target_type":"Issue","author_id":1,"created_at":"2026-03-01T08:00:00Z","author_username":"dev"}
		]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	}))

	out, err := ListCurrentUserContributionEvents(context.Background(), client, ListContributionEventsInput{
		Action:     actionPushed,
		TargetType: targetIssue,
		Scope:      "all",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Events) != 1 {
		t.Fatalf("got %d events, want 1", len(out.Events))
	}
}

// TestListCurrentUserContributionEvents_APIError_Forbidden verifies ListCurrentUserContributionEvents returns error on HTTP 403.
func TestListCurrentUserContributionEvents_APIError_Forbidden(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := ListCurrentUserContributionEvents(context.Background(), client, ListContributionEventsInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestFormatContributionListMarkdownString_WithEvents verifies the behavior of format contribution list markdown string with events.
func TestFormatContributionListMarkdownString_WithEvents(t *testing.T) {
	out := ListContributionEventsOutput{
		Events: []ContributionEventOutput{
			{ID: 1, ActionName: actionPushed, AuthorUsername: "dev", CreatedAt: "2026-06-01T10:00:00Z", TargetType: "MergeRequest", TargetIID: 3},
			{ID: 2, ActionName: "opened", AuthorUsername: "dev", CreatedAt: "2026-06-02T11:00:00Z"},
		},
	}
	md := FormatContributionListMarkdownString(out)
	if md == "" {
		t.Fatal("expected non-empty markdown")
	}
	if !contains(md, actionPushed) || !contains(md, "dev") {
		t.Errorf("markdown missing expected content: %s", md)
	}
}

// TestFormatContributionListMarkdownString_Empty verifies the behavior of format contribution list markdown string empty.
func TestFormatContributionListMarkdownString_Empty(t *testing.T) {
	out := ListContributionEventsOutput{Events: []ContributionEventOutput{}}
	md := FormatContributionListMarkdownString(out)
	if md != "No contribution events found.\n" {
		t.Errorf("got %q, want %q", md, "No contribution events found.\n")
	}
}

// TestFormatListMarkdownString_WithEvents verifies the behavior of format list markdown string with events.
func TestFormatListMarkdownString_WithEvents(t *testing.T) {
	out := ListProjectEventsOutput{
		Events: []ProjectEventOutput{
			{ID: 1, ActionName: actionPushed, AuthorUsername: "alice", CreatedAt: "2026-01-15", TargetType: "MergeRequest", TargetIID: 3},
			{ID: 2, ActionName: "commented", AuthorUsername: "bob", CreatedAt: testDateCreated},
		},
	}
	md := FormatListMarkdownString(out)
	if md == "" {
		t.Fatal("expected non-empty markdown")
	}
	if !contains(md, actionPushed) || !contains(md, "alice") {
		t.Errorf("markdown missing expected content: %s", md)
	}
	if !contains(md, "MergeRequest #3") {
		t.Errorf("markdown missing target info: %s", md)
	}
}

// TestFormatListMarkdownString_Empty verifies the behavior of format list markdown string empty.
func TestFormatListMarkdownString_Empty(t *testing.T) {
	out := ListProjectEventsOutput{Events: []ProjectEventOutput{}}
	md := FormatListMarkdownString(out)
	if md != "No project events found.\n" {
		t.Errorf("got %q, want %q", md, "No project events found.\n")
	}
}

// TestFormatContributionListMarkdownString_TargetTitleShown verifies the behavior of format contribution list markdown string target title shown.
func TestFormatContributionListMarkdownString_TargetTitleShown(t *testing.T) {
	out := ListContributionEventsOutput{
		Events: []ContributionEventOutput{
			{ID: 10, ActionName: "opened", AuthorUsername: "dev", TargetType: "Issue", TargetIID: 7, TargetTitle: titleBugReport, CreatedAt: testDateAfter},
		},
	}
	md := FormatContributionListMarkdownString(out)
	if !contains(md, `Issue #7 "Bug Report"`) {
		t.Errorf("expected TargetTitle in output, got: %s", md)
	}
}

// TestFormatContributionListMarkdownString_AuthorPrefixed verifies the behavior of format contribution list markdown string author prefixed.
func TestFormatContributionListMarkdownString_AuthorPrefixed(t *testing.T) {
	out := ListContributionEventsOutput{
		Events: []ContributionEventOutput{
			{ID: 10, ActionName: actionPushed, AuthorUsername: "alice", CreatedAt: testDateAfter},
		},
	}
	md := FormatContributionListMarkdownString(out)
	if !contains(md, "@alice") {
		t.Errorf("expected @alice in output, got: %s", md)
	}
}

// TestFormatContributionListMarkdownString_NoEventID verifies the behavior of format contribution list markdown string no event i d.
func TestFormatContributionListMarkdownString_NoEventID(t *testing.T) {
	out := ListContributionEventsOutput{
		Events: []ContributionEventOutput{
			{ID: 99, ActionName: actionPushed, AuthorUsername: "dev", CreatedAt: testDateAfter},
		},
	}
	md := FormatContributionListMarkdownString(out)
	if contains(md, "(ID: 99)") {
		t.Errorf("event ID should not appear in markdown, got: %s", md)
	}
}

// TestFormatListMarkdownString_TargetTitleShown verifies the behavior of format list markdown string target title shown.
func TestFormatListMarkdownString_TargetTitleShown(t *testing.T) {
	out := ListProjectEventsOutput{
		Events: []ProjectEventOutput{
			{ID: 20, ActionName: "commented", AuthorUsername: "bob", TargetType: "MergeRequest", TargetIID: 5, TargetTitle: "Add feature X", CreatedAt: testDateCreated},
		},
	}
	md := FormatListMarkdownString(out)
	if !contains(md, `MergeRequest #5 "Add feature X"`) {
		t.Errorf("expected TargetTitle in output, got: %s", md)
	}
}

// TestFormatListMarkdownString_AuthorPrefixed verifies the behavior of format list markdown string author prefixed.
func TestFormatListMarkdownString_AuthorPrefixed(t *testing.T) {
	out := ListProjectEventsOutput{
		Events: []ProjectEventOutput{
			{ID: 20, ActionName: actionPushed, AuthorUsername: "bob", CreatedAt: testDateCreated},
		},
	}
	md := FormatListMarkdownString(out)
	if !contains(md, "@bob") {
		t.Errorf("expected @bob in output, got: %s", md)
	}
}

// TestFormatListMarkdownString_NoEventID verifies the behavior of format list markdown string no event i d.
func TestFormatListMarkdownString_NoEventID(t *testing.T) {
	out := ListProjectEventsOutput{
		Events: []ProjectEventOutput{
			{ID: 88, ActionName: actionPushed, AuthorUsername: "alice", CreatedAt: "2026-01-15"},
		},
	}
	md := FormatListMarkdownString(out)
	if contains(md, "(ID: 88)") {
		t.Errorf("event ID should not appear in markdown, got: %s", md)
	}
}

// TestFormatAuthor validates format author across multiple scenarios using table-driven subtests.
func TestFormatAuthor(t *testing.T) {
	tests := []struct {
		name     string
		username string
		want     string
	}{
		{"with username", "alice", "@alice"},
		{"empty username", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatAuthor(tc.username)
			if got != tc.want {
				t.Errorf("formatAuthor(%q) = %q, want %q", tc.username, got, tc.want)
			}
		})
	}
}

// contains is an internal helper for the events package.
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsSubstring(s, sub))
}

// containsSubstring is an internal helper for the events package.
func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ---------- Tests consolidated from coverage_test.go ----------.

// toContributionEventOutput.

// TestCovtoContributionEventOutput_NilCreatedAt verifies the behavior of covto contribution event output nil created at.
func TestCovtoContributionEventOutput_NilCreatedAt(t *testing.T) {
	e := &gl.ContributionEvent{
		ID:             1,
		Title:          "covTitle",
		ProjectID:      2,
		ActionName:     "covAction",
		TargetID:       3,
		TargetIID:      4,
		TargetType:     "covType",
		AuthorID:       5,
		TargetTitle:    "covTargetTitle",
		CreatedAt:      nil,
		AuthorUsername: "covUser",
	}
	out := toContributionEventOutput(e)
	if out.CreatedAt != "" {
		t.Errorf("expected empty CreatedAt, got %q", out.CreatedAt)
	}
	if out.ID != 1 || out.AuthorUsername != "covUser" {
		t.Error("field mapping failed")
	}
}

// TestCovtoContributionEventOutput_WithDate verifies the behavior of covto contribution event output with date.
func TestCovtoContributionEventOutput_WithDate(t *testing.T) {
	ts := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	e := &gl.ContributionEvent{
		ID:             11,
		Title:          "covTitle",
		ProjectID:      22,
		ActionName:     "covAction",
		TargetID:       33,
		TargetIID:      44,
		TargetType:     "covType",
		AuthorID:       55,
		TargetTitle:    "covTargetTitle",
		CreatedAt:      &ts,
		AuthorUsername: "covUser",
	}
	out := toContributionEventOutput(e)
	if !strings.Contains(out.CreatedAt, "2026-03-07") {
		t.Errorf("expected date in CreatedAt, got %q", out.CreatedAt)
	}
}

// FormatContributionListMarkdown.

// TestFormatContributionListMarkdown_Wrapper verifies the behavior of cov format contribution list markdown wrapper.
func TestFormatContributionListMarkdown_Wrapper(t *testing.T) {
	out := ListContributionEventsOutput{
		Events: []ContributionEventOutput{{ID: 1, Title: "covTitle", ActionName: "pushed"}},
	}
	res := FormatContributionListMarkdown(out)
	if res == nil {
		t.Fatal("expected non-nil result")
	}
}

// TestFormatContributionListMarkdownString_EmptyTargetType verifies the behavior of cov format contribution list markdown string empty target type.
func TestFormatContributionListMarkdownString_EmptyTargetType(t *testing.T) {
	out := ListContributionEventsOutput{
		Events: []ContributionEventOutput{{ID: 1, ActionName: "pushed", TargetType: ""}},
	}
	md := FormatContributionListMarkdownString(out)
	if strings.Contains(md, "#0") {
		t.Error("empty TargetType should not produce target text")
	}
}

// TestFormatContributionListMarkdownString_WithTargetType verifies the behavior of cov format contribution list markdown string with target type.
func TestFormatContributionListMarkdownString_WithTargetType(t *testing.T) {
	out := ListContributionEventsOutput{
		Events: []ContributionEventOutput{{ID: 1, ActionName: "pushed", TargetType: "Issue", TargetIID: 42}},
	}
	md := FormatContributionListMarkdownString(out)
	if !strings.Contains(md, "Issue #42") {
		t.Error("expected target type in markdown")
	}
}

// toProjectEventOutput.

// TestCovtoProject_EventOutputFieldMapping verifies the behavior of covto project event output field mapping.
func TestCovtoProject_EventOutputFieldMapping(t *testing.T) {
	e := &gl.ProjectEvent{
		ID:             101,
		Title:          "covTitle",
		ProjectID:      202,
		ActionName:     "covAction",
		TargetID:       303,
		TargetIID:      404,
		TargetType:     "covType",
		AuthorID:       505,
		TargetTitle:    "covTargetTitle",
		CreatedAt:      "2026-03-07T12:34:56Z",
		AuthorUsername: "covUser",
	}
	out := toProjectEventOutput(e)
	if out.ID != 101 || out.ProjectID != 202 || out.ActionName != "covAction" {
		t.Errorf("field mapping failed: %+v", out)
	}
	if out.CreatedAt != "2026-03-07T12:34:56Z" {
		t.Errorf("expected CreatedAt passthrough, got %q", out.CreatedAt)
	}
}

// FormatListMarkdown.

// TestFormatListMarkdown_Wrapper verifies the behavior of cov format list markdown wrapper.
func TestFormatListMarkdown_Wrapper(t *testing.T) {
	out := ListProjectEventsOutput{
		Events: []ProjectEventOutput{{ID: 1, Title: "covTitle", ActionName: "pushed"}},
	}
	res := FormatListMarkdown(out)
	if res == nil {
		t.Fatal("expected non-nil result")
	}
}

// TestFormatListMarkdownString_EmptyTargetType verifies the behavior of cov format list markdown string empty target type.
func TestFormatListMarkdownString_EmptyTargetType(t *testing.T) {
	out := ListProjectEventsOutput{
		Events: []ProjectEventOutput{{ID: 1, ActionName: "pushed", TargetType: ""}},
	}
	md := FormatListMarkdownString(out)
	if strings.Contains(md, "#0") {
		t.Error("empty TargetType should not produce target text")
	}
}

// TestFormatListMarkdownString_WithTargetType verifies the behavior of cov format list markdown string with target type.
func TestFormatListMarkdownString_WithTargetType(t *testing.T) {
	out := ListProjectEventsOutput{
		Events: []ProjectEventOutput{{ID: 1, ActionName: "pushed", TargetType: "MR", TargetIID: 5}},
	}
	md := FormatListMarkdownString(out)
	if !strings.Contains(md, "MR #5") {
		t.Error("expected target type in markdown")
	}
}

// API error paths.

// TestListCurrentUserContributionEvents_APIError verifies the behavior of cov list current user contribution events a p i error.
func TestListCurrentUserContributionEvents_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad"}`)
	}))
	_, err := ListCurrentUserContributionEvents(t.Context(), client, ListContributionEventsInput{})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestListCurrentUserContributionEvents_AllFilters verifies the behavior of cov list current user contribution events all filters.
func TestListCurrentUserContributionEvents_AllFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	out, err := ListCurrentUserContributionEvents(t.Context(), client, ListContributionEventsInput{
		Action:     "pushed",
		TargetType: "issue",
		Before:     "2026-01-01",
		After:      "2026-01-01",
		Sort:       "asc",
		Scope:      "all",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Events) != 0 {
		t.Error("expected empty events")
	}
}

// TestListCurrentUserContributionEvents_InvalidDates verifies the behavior of cov list current user contribution events invalid dates.
func TestListCurrentUserContributionEvents_InvalidDates(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := ListCurrentUserContributionEvents(t.Context(), client, ListContributionEventsInput{
		Before: "not-a-date",
		After:  "not-a-date",
	})
	if err != nil {
		t.Errorf("invalid dates should not error, got %v", err)
	}
}

// TestListProjectEvents_APIError verifies the behavior of cov list project events a p i error.
func TestListProjectEvents_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad"}`)
	}))
	_, err := ListProjectEvents(t.Context(), client, ListProjectEventsInput{ProjectID: "proj"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestListProjectEvents_EmptyProjectID verifies the behavior of cov list project events empty project i d.
func TestListProjectEvents_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	_, err := ListProjectEvents(t.Context(), client, ListProjectEventsInput{ProjectID: ""})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

// TestListProjectEvents_AllFilters verifies the behavior of cov list project events all filters.
func TestListProjectEvents_AllFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	out, err := ListProjectEvents(t.Context(), client, ListProjectEventsInput{
		ProjectID:  "proj",
		Action:     "created",
		TargetType: "merge_request",
		Before:     "2026-01-01",
		After:      "2026-01-01",
		Sort:       "desc",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Events) != 0 {
		t.Error("expected empty events")
	}
}

// TestListProjectEvents_InvalidDates verifies the behavior of cov list project events invalid dates.
func TestListProjectEvents_InvalidDates(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := ListProjectEvents(t.Context(), client, ListProjectEventsInput{
		ProjectID: "proj",
		Before:    "nope",
		After:     "nope",
	})
	if err != nil {
		t.Errorf("invalid dates should not error, got %v", err)
	}
}

// RegisterTools / RegisterMeta.

// TestRegisterTools_NoPanic verifies the behavior of cov register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	RegisterTools(server, client)
}

// TestRegisterMeta_NoPanic verifies the behavior of cov register meta no panic.
func TestRegisterMeta_NoPanic(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	RegisterMeta(server, client)
}

// MCP round-trip.

// TestMCPRound_Trip validates cov m c p round trip across multiple scenarios using table-driven subtests.
func TestMCPRound_Trip(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, handler)
	RegisterTools(server, client)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	go server.Connect(ctx, st, nil)

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	tests := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_project_event_list", map[string]any{"project_id": "proj"}},
		{"gitlab_user_contribution_event_list", map[string]any{}},
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
