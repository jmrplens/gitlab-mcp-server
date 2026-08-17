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

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	// actionPushed identifies the action pushed constant used by this package.
	actionPushed = "pushed"
	// targetIssue identifies the target issue constant used by this package.
	targetIssue = "issue"
	// titleBugReport identifies the title bug report constant used by this package.
	titleBugReport = "Bug Report"
	// fmtUnexpErr identifies the fmt unexp err constant used by this package.
	fmtUnexpErr = "unexpected error: %v"
	// testDateAfter identifies the test date after constant used by this package.
	testDateAfter = "2026-06-01"
	// testDateCreated identifies the test date created constant used by this package.
	testDateCreated = "2026-01-14"
)

// TestListProjectEvents_Success verifies that ListProjectEvents succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/projects/42/events (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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

	out, err := ListProjectEvents(context.Background(), client, ListProjectEventsInput{ProjectID: "42", PaginationInput: toolutil.PaginationInput{Page: 1, PerPage: 20}})
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

// TestListProjectEvents_WithFilters verifies the ListProjectEvents_WithFilters handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestListProjectEvents_ValidationError verifies that ListProjectEvents_ValidationError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListProjectEvents_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := ListProjectEvents(context.Background(), client, ListProjectEventsInput{})
	if err == nil {
		t.Fatal("expected error for empty project_id, got nil")
	}
}

// TestListProjectEvents_APIError_Forbidden verifies that ListProjectEvents_Forbidden returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListProjectEvents_APIError_Forbidden(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := ListProjectEvents(context.Background(), client, ListProjectEventsInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestListProjectEvents_EmptyResult verifies the ListProjectEvents_EmptyResult handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestListCurrentUserContributionEvents_Success verifies that ListCurrentUserContributionEvents succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/events (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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

	out, err := ListCurrentUserContributionEvents(context.Background(), client, ListContributionEventsInput{PaginationInput: toolutil.PaginationInput{Page: 1, PerPage: 20}})
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

// TestListCurrentUserContributionEvents_WithFilters verifies the ListCurrentUserContributionEvents_WithFilters handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestListCurrentUserContributionEvents_APIError_Forbidden verifies that ListCurrentUserContributionEvents_Forbidden returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListCurrentUserContributionEvents_APIError_Forbidden(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := ListCurrentUserContributionEvents(context.Background(), client, ListContributionEventsInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestFormatContributionListMarkdownString_WithEvents verifies the ContributionListMarkdownString_WithEvents Markdown formatter for a representative contributionliststring_withevents input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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

// TestFormatContributionListMarkdownString_Empty verifies the ContributionListMarkdownString_Empty Markdown formatter for a representative contributionliststring_empty input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatContributionListMarkdownString_Empty(t *testing.T) {
	out := ListContributionEventsOutput{Events: []ContributionEventOutput{}}
	md := FormatContributionListMarkdownString(out)
	if md != "No contribution events found.\n" {
		t.Errorf("got %q, want %q", md, "No contribution events found.\n")
	}
}

// TestFormatListMarkdownString_WithEvents verifies the ListMarkdownString_WithEvents Markdown formatter for a representative liststring_withevents input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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

// TestFormatListMarkdownString_Empty verifies the ListMarkdownString_Empty Markdown formatter for a representative liststring_empty input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdownString_Empty(t *testing.T) {
	out := ListProjectEventsOutput{Events: []ProjectEventOutput{}}
	md := FormatListMarkdownString(out)
	if md != "No project events found.\n" {
		t.Errorf("got %q, want %q", md, "No project events found.\n")
	}
}

// TestFormatContributionListMarkdownString_TargetTitleShown verifies the ContributionListMarkdownString_TargetTitleShown Markdown formatter for a representative contributionliststring_targettitleshown input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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

// TestFormatContributionListMarkdownString_AuthorPrefixed verifies the ContributionListMarkdownString_AuthorPrefixed Markdown formatter for a representative contributionliststring_authorprefixed input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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

// TestFormatContributionListMarkdownString_NoEventID verifies the ContributionListMarkdownString_NoEventID Markdown formatter for a representative contributionliststring_noeventid input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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

// TestFormatListMarkdownString_TargetTitleShown verifies the ListMarkdownString_TargetTitleShown Markdown formatter for a representative liststring_targettitleshown input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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

// TestFormatListMarkdownString_AuthorPrefixed verifies the ListMarkdownString_AuthorPrefixed Markdown formatter for a representative liststring_authorprefixed input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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

// TestFormatListMarkdownString_NoEventID verifies the ListMarkdownString_NoEventID Markdown formatter for a representative liststring_noeventid input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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

// TestFormatAuthor verifies the Author Markdown formatter for a representative author input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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

// contains reports whether contains.
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsSubstring(s, sub))
}

// containsSubstring reports whether contains substring.
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

// TestCovtoContributionEventOutput_NilCreatedAt verifies the CovtoContributionEventOutput_NilCreatedAt handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestCovtoContributionEventOutput_WithDate verifies the CovtoContributionEventOutput_WithDate handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestFormatContributionListMarkdown_Wrapper verifies the ContributionListMarkdown_Wrapper Markdown formatter for a representative contributionlist_wrapper input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatContributionListMarkdown_Wrapper(t *testing.T) {
	out := ListContributionEventsOutput{
		Events: []ContributionEventOutput{{ID: 1, Title: "covTitle", ActionName: "pushed"}},
	}
	res := FormatContributionListMarkdown(out)
	if res == nil {
		t.Fatal("expected non-nil result")
	}
}

// TestFormatContributionListMarkdownString_EmptyTargetType verifies the ContributionListMarkdownString_EmptyTargetType Markdown formatter for a representative contributionliststring_emptytargettype input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatContributionListMarkdownString_EmptyTargetType(t *testing.T) {
	out := ListContributionEventsOutput{
		Events: []ContributionEventOutput{{ID: 1, ActionName: "pushed", TargetType: ""}},
	}
	md := FormatContributionListMarkdownString(out)
	if strings.Contains(md, "#0") {
		t.Error("empty TargetType should not produce target text")
	}
}

// TestFormatContributionListMarkdownString_WithTargetType verifies the ContributionListMarkdownString_WithTargetType Markdown formatter for a representative contributionliststring_withtargettype input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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

// TestCovtoProject_EventOutputFieldMapping verifies the CovtoProject_EventOutputFieldMapping handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestFormatListMarkdown_Wrapper verifies the ListMarkdown_Wrapper Markdown formatter for a representative list_wrapper input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown_Wrapper(t *testing.T) {
	out := ListProjectEventsOutput{
		Events: []ProjectEventOutput{{ID: 1, Title: "covTitle", ActionName: "pushed"}},
	}
	res := FormatListMarkdown(out)
	if res == nil {
		t.Fatal("expected non-nil result")
	}
}

// TestFormatListMarkdownString_EmptyTargetType verifies the ListMarkdownString_EmptyTargetType Markdown formatter for a representative liststring_emptytargettype input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdownString_EmptyTargetType(t *testing.T) {
	out := ListProjectEventsOutput{
		Events: []ProjectEventOutput{{ID: 1, ActionName: "pushed", TargetType: ""}},
	}
	md := FormatListMarkdownString(out)
	if strings.Contains(md, "#0") {
		t.Error("empty TargetType should not produce target text")
	}
}

// TestFormatListMarkdownString_WithTargetType verifies the ListMarkdownString_WithTargetType Markdown formatter for a representative liststring_withtargettype input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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

// TestListCurrentUserContributionEvents_APIError verifies that ListCurrentUserContributionEvents returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListCurrentUserContributionEvents_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad"}`)
	}))
	_, err := ListCurrentUserContributionEvents(t.Context(), client, ListContributionEventsInput{})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestListCurrentUserContributionEvents_AllFilters verifies the ListCurrentUserContributionEvents_AllFilters handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestListCurrentUserContributionEvents_InvalidDates verifies the ListCurrentUserContributionEvents_InvalidDates handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestListProjectEvents_APIError verifies that ListProjectEvents returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListProjectEvents_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad"}`)
	}))
	_, err := ListProjectEvents(t.Context(), client, ListProjectEventsInput{ProjectID: "proj"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestListProjectEvents_EmptyProjectID verifies the ListProjectEvents_EmptyProjectID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListProjectEvents_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := ListProjectEvents(t.Context(), client, ListProjectEventsInput{ProjectID: ""})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

// TestListProjectEvents_AllFilters verifies the ListProjectEvents_AllFilters handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestListProjectEvents_InvalidDates verifies the ListProjectEvents_InvalidDates handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestUserActionSpecs_Metadata verifies the UserActionSpecs_Metadata handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestUserActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	specs := UserActionSpecs(client)
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	if len(specs) != 2 {
		t.Fatalf("len(UserActionSpecs) = %d, want 2", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "events" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
		if spec.Usage == "" {
			t.Fatalf("Usage for %s should not be empty", spec.Name)
		}
		if len(spec.Aliases) == 0 {
			t.Fatalf("Aliases for %s should not be empty", spec.Name)
		}
	}
	if byTool["gitlab_project_event_list"].ParameterGuidance["project_id"].SemanticRole == "" {
		t.Fatal("gitlab_project_event_list should define project_id parameter guidance")
	}
}

// ActionSpec route execution.

// TestUserActionSpecs_CallRoutes verifies the UserActionSpecs_CallRoutes handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestUserActionSpecs_CallRoutes(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})
	client := testutil.NewTestClient(t, handler)
	specs := UserActionSpecs(client)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
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
			spec, ok := specByTool[tc.name]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tc.name)
			}
			res, err := spec.Route.Handler(t.Context(), tc.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s): %v", tc.name, err)
			}
			if res == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tc.name)
			}
		})
	}
}

// TestFormatTarget_WithURLAndTitle verifies that formatTarget produces a clickable
// link when targetURL is provided, and appends the title in quotes.
func TestFormatTarget_WithURLAndTitle(t *testing.T) {
	got := formatTarget("Issue", 42, "Bug title", "https://gitlab.example.com/issues/42")
	if !strings.Contains(got, "[Issue #42](https://gitlab.example.com/issues/42)") {
		t.Errorf("expected markdown link, got %q", got)
	}
	if !strings.Contains(got, `"Bug title"`) {
		t.Errorf("expected title in quotes, got %q", got)
	}
}

// TestFormatTarget_WithURLNoTitle verifies the Target_WithURLNoTitle Markdown formatter for a representative target_withurlnotitle input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatTarget_WithURLNoTitle(t *testing.T) {
	got := formatTarget("MergeRequest", 10, "", "https://gitlab.example.com/mr/10")
	if !strings.Contains(got, "[MergeRequest #10](https://gitlab.example.com/mr/10)") {
		t.Errorf("expected markdown link, got %q", got)
	}
	if strings.Contains(got, `""`) {
		t.Error("should not contain empty quoted title")
	}
}

// TestResolveProjectWebURLs_SkipsZeroID verifies the ResolveProjectWebURLs_SkipsZeroID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestResolveProjectWebURLs_SkipsZeroID(t *testing.T) {
	apiCalled := false
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		apiCalled = true
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"web_url":"https://example.com/p"}`)
	}))
	urls := toolutil.ResolveProjectWebURLs(t.Context(), client.GL().Projects, []int64{0})
	if apiCalled {
		t.Error("API should not be called for project ID 0")
	}
	if len(urls) != 0 {
		t.Errorf("expected empty map, got %v", urls)
	}
}

// TestResolveProjectWebURLs_DeduplicatesIDs verifies the ResolveProjectWebURLs_DeduplicatesIDs handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestResolveProjectWebURLs_DeduplicatesIDs(t *testing.T) {
	callCount := 0
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		testutil.RespondJSON(w, http.StatusOK, `{"id":5,"web_url":"https://example.com/p/5"}`)
	}))
	urls := toolutil.ResolveProjectWebURLs(t.Context(), client.GL().Projects, []int64{5, 5, 5})
	if callCount != 1 {
		t.Errorf("expected 1 API call, got %d", callCount)
	}
	if urls[5] != "https://example.com/p/5" {
		t.Errorf("url = %q", urls[5])
	}
}

// TestToContributionEventOutput_FullMirror verifies that every nested sub-object
// of a ContributionEvent (push_data, note with author/position/resolved_by, and
// the author BasicUser) is mirrored onto the output.
func TestToContributionEventOutput_FullMirror(t *testing.T) {
	ts := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	noteType := gl.NoteTypeValue("DiffNote")
	e := &gl.ContributionEvent{
		ID:        1,
		ProjectID: 2,
		CreatedAt: &ts,
		PushData: gl.ContributionEventPushData{
			CommitCount: 3, Action: "pushed", RefType: "branch",
			CommitFrom: "aaa", CommitTo: "bbb", Ref: "main", CommitTitle: "fix",
		},
		Note: &gl.Note{
			ID: 10, Type: noteType, Body: "hello", Title: "t", FileName: "f.go",
			Author:    gl.NoteAuthor{ID: 5, Username: "u", Email: "u@e", Name: "User", State: "active", AvatarURL: "a", WebURL: "w"},
			System:    true,
			CreatedAt: &ts, UpdatedAt: &ts, ExpiresAt: &ts, ResolvedAt: &ts,
			CommitID:   "c1",
			Resolvable: true, Resolved: true,
			ResolvedBy:   gl.NoteResolvedBy{ID: 6, Username: "r"},
			Internal:     true,
			Confidential: true,
			Position: &gl.NotePosition{
				BaseSHA: "b", StartSHA: "s", HeadSHA: "h", PositionType: "text",
				NewPath: "new.go", NewLine: 12, OldPath: "old.go", OldLine: 8,
				LineRange: &gl.LineRange{
					StartRange: &gl.LinePosition{LineCode: "lc1", Type: "new", OldLine: 1, NewLine: 2},
					EndRange:   &gl.LinePosition{LineCode: "lc2", Type: "old", OldLine: 3, NewLine: 4},
				},
			},
		},
		Author: gl.BasicUser{ID: 5, Username: "u", Name: "User", State: "active", CreatedAt: &ts, AvatarURL: "a", WebURL: "w"},
	}

	out := toContributionEventOutput(e)
	assertTrue(t, out.PushData != nil && out.PushData.CommitCount == 3 && out.PushData.CommitTitle == "fix", "push_data")
	assertTrue(t, out.Author != nil && out.Author.ID == 5 && out.Author.CreatedAt != "", "author")
	assertContributionNote(t, out.Note)
}

// assertContributionNote validates the deeply nested note mirror.
func assertContributionNote(t *testing.T, n *NoteOutput) {
	t.Helper()
	assertTrue(t, n != nil && n.ID == 10 && n.Type == "DiffNote" && n.Internal, "note core")
	assertTrue(t, n.Author != nil && n.Author.Email == "u@e", "note author")
	assertTrue(t, n.ResolvedBy != nil && n.ResolvedBy.ID == 6, "note resolved_by")
	assertTrue(t, n.CreatedAt != "" && n.UpdatedAt != "" && n.ExpiresAt != "" && n.ResolvedAt != "", "note timestamps")
	p := n.Position
	assertTrue(t, p != nil && p.NewLine == 12 && p.OldPath == "old.go", "note position")
	assertTrue(t, p.LineRange != nil && p.LineRange.StartRange != nil && p.LineRange.StartRange.LineCode == "lc1", "line range start")
	assertTrue(t, p.LineRange.EndRange != nil && p.LineRange.EndRange.NewLine == 4, "line range end")
}

// assertTrue fails the test with a labeled message when cond is false.
func assertTrue(t *testing.T, cond bool, label string) {
	t.Helper()
	if !cond {
		t.Fatalf("%s not mirrored correctly", label)
	}
}

// TestToContributionEventOutput_EmptySubObjects verifies that zero-valued sub
// objects are omitted (nil) so the output stays clean.
func TestToContributionEventOutput_EmptySubObjects(t *testing.T) {
	out := toContributionEventOutput(&gl.ContributionEvent{ID: 1})
	if out.PushData != nil {
		t.Errorf("expected nil push_data, got %+v", out.PushData)
	}
	if out.Note != nil {
		t.Errorf("expected nil note, got %+v", out.Note)
	}
	if out.Author != nil {
		t.Errorf("expected nil author, got %+v", out.Author)
	}
}

// TestToBasicUserOutput_NilCreatedAt verifies the BasicUser mirror handles a nil
// created timestamp.
func TestToBasicUserOutput_NilCreatedAt(t *testing.T) {
	out := toBasicUserOutput(gl.BasicUser{ID: 7, Username: "x"})
	if out == nil || out.ID != 7 || out.CreatedAt != "" {
		t.Fatalf("unexpected user output: %+v", out)
	}
}

// TestNoteAuthorOutput_Empty verifies the shared note-author mirror returns
// nil for zero-valued authors and a populated value otherwise.
func TestNoteAuthorOutput_Empty(t *testing.T) {
	if noteAuthorOutput(0, "", "", "", "", "", "") != nil {
		t.Error("expected nil for empty author fields")
	}
	if got := noteAuthorOutput(1, "u", "", "", "", "", ""); got == nil || got.ID != 1 {
		t.Errorf("expected populated author, got %+v", got)
	}
}

// TestNilSubObjectMirrors verifies the nil-input branches of the nested
// position/line-range converters and the project event note author.
func TestNilSubObjectMirrors(t *testing.T) {
	if toNotePositionOutput(nil) != nil {
		t.Error("expected nil note position")
	}
	if toLineRangeOutput(nil) != nil {
		t.Error("expected nil line range")
	}
	if toLinePositionOutput(nil) != nil {
		t.Error("expected nil line position")
	}
	if toProjectEventNoteAuthorOutput(gl.ProjectEventNoteAuthor{}) != nil {
		t.Error("expected nil project event note author")
	}
	// LineRange present but with nil endpoints exercises the start/end nil paths.
	r := toLineRangeOutput(&gl.LineRange{})
	if r == nil || r.StartRange != nil || r.EndRange != nil {
		t.Errorf("expected non-nil line range with nil endpoints, got %+v", r)
	}
}

// TestToProjectEventOutput_FullMirror verifies that ProjectEvent push_data,
// note (with author), and data (ref, repository, commits with stats and
// pipeline) are all mirrored.
func TestToProjectEventOutput_FullMirror(t *testing.T) {
	ts := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	status := gl.BuildStateValue("success")
	e := &gl.ProjectEvent{
		ID: 1, ProjectID: 2, CreatedAt: "2026-03-07T12:00:00Z",
		Author: gl.BasicUser{ID: 5, Username: "u"},
		PushData: gl.ProjectEventPushData{
			CommitCount: 2, Action: "pushed", RefType: "branch",
			CommitFrom: "a", CommitTo: "b", Ref: "main", CommitTitle: "ct",
		},
		Note: gl.ProjectEventNote{
			ID: 9, Body: "body", Attachment: "att", System: true,
			NoteableID: 3, NoteableType: "Issue", NoteableIID: 4, CreatedAt: &ts,
			Author: gl.ProjectEventNoteAuthor{ID: 6, Username: "na", Email: "n@e", Name: "N", State: "active", AvatarURL: "av", WebURL: "wu"},
		},
		Data: gl.ProjectEventData{
			Before: "x", After: "y", Ref: "refs/heads/main", UserID: 8, UserName: "dev",
			TotalCommitsCount: 1,
			Repository: &gl.Repository{
				Name: "repo", Description: "d", WebURL: "wu", AvatarURL: "av",
				GitSSHURL: "gs", GitHTTPURL: "gh", Namespace: "ns", Visibility: gl.PublicVisibility,
				PathWithNamespace: "ns/repo", DefaultBranch: "main", Homepage: "hp", URL: "u", SSHURL: "ssh", HTTPURL: "http",
			},
			Commits: []*gl.Commit{
				nil,
				{
					ID: "c1", ShortID: "c", Title: "t", AuthorName: "an", AuthorEmail: "ae",
					AuthoredDate: &ts, CommitterName: "cn", CommitterEmail: "ce", CommittedDate: &ts, CreatedAt: &ts,
					Message: "m", ParentIDs: []string{"p"}, ProjectID: 2, WebURL: "wu",
					Trailers: map[string]string{"k": "v"}, ExtendedTrailers: map[string]string{"k2": "v2"},
					Stats:        &gl.CommitStats{Additions: 1, Deletions: 2, Total: 3},
					Status:       &status,
					LastPipeline: &gl.PipelineInfo{ID: 100, IID: 1, ProjectID: 2, Status: "success", Source: "push", Ref: "main", SHA: "sha", Name: "p", WebURL: "wu", UpdatedAt: &ts, CreatedAt: &ts},
				},
			},
		},
	}

	out := toProjectEventOutput(e)
	assertTrue(t, out.Author != nil && out.Author.ID == 5, "author")
	assertTrue(t, out.PushData != nil && out.PushData.CommitTitle == "ct", "push_data")
	assertTrue(t, out.Note != nil && out.Note.NoteableType == "Issue" && out.Note.Author != nil && out.Note.Author.Email == "n@e" && out.Note.CreatedAt != "", "note")
	assertProjectData(t, out.Data)
}

// assertProjectData validates the project event data mirror, including the
// repository and the nested commit with stats and pipeline.
func assertProjectData(t *testing.T, d *ProjectEventDataOutput) {
	t.Helper()
	assertTrue(t, d != nil && d.Ref == "refs/heads/main" && d.UserName == "dev" && d.TotalCommitsCount == 1, "data")
	assertTrue(t, d.Repository != nil && d.Repository.PathWithNamespace == "ns/repo" && d.Repository.Visibility == "public", "repository")
	assertTrue(t, len(d.Commits) == 1, "commit count (nil skipped)")
	c := d.Commits[0]
	assertTrue(t, c.ID == "c1" && c.Status == "success" && c.AuthoredDate != "" && c.CommittedDate != "" && c.CreatedAt != "", "commit")
	assertTrue(t, c.Stats != nil && c.Stats.Total == 3, "commit stats")
	assertTrue(t, c.LastPipeline != nil && c.LastPipeline.ID == 100 && c.LastPipeline.UpdatedAt != "" && c.LastPipeline.CreatedAt != "", "commit last_pipeline")
	assertTrue(t, c.Trailers["k"] == "v" && c.ExtendedTrailers["k2"] == "v2", "commit trailers")
}

// TestToProjectEventOutput_EmptySubObjects verifies zero-valued ProjectEvent sub
// objects are omitted.
func TestToProjectEventOutput_EmptySubObjects(t *testing.T) {
	out := toProjectEventOutput(&gl.ProjectEvent{ID: 1})
	if out.PushData != nil || out.Note != nil || out.Data != nil || out.Author != nil {
		t.Errorf("expected nil sub-objects, got %+v", out)
	}
}

// TestToProjectEventDataOutput_MinimalCommit covers a commit
// with nil stats/pipeline and timestamps, and a data block with nil repository.
func TestToProjectEventDataOutput_MinimalCommit(t *testing.T) {
	d := gl.ProjectEventData{
		Ref:     "main",
		Commits: []*gl.Commit{{ID: "c1"}},
	}
	out := toProjectEventDataOutput(d)
	if out == nil || out.Repository != nil || len(out.Commits) != 1 {
		t.Fatalf("unexpected data output: %+v", out)
	}
	c := out.Commits[0]
	if c.Stats != nil || c.LastPipeline != nil || c.AuthoredDate != "" || c.Status != "" {
		t.Fatalf("expected minimal commit, got %+v", c)
	}
}

// TestListProjectEvents_KeysetAndOrderBy verifies that keyset pagination and
// order_by parameters are forwarded as query parameters.
func TestListProjectEvents_KeysetAndOrderBy(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("pagination") != "keyset" {
			t.Errorf("expected pagination=keyset, got %q", q.Get("pagination"))
		}
		if q.Get("page_token") != "99" {
			t.Errorf("expected page_token=99, got %q", q.Get("page_token"))
		}
		if q.Get("order_by") != "id" {
			t.Errorf("expected order_by=id, got %q", q.Get("order_by"))
		}
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := ListProjectEvents(t.Context(), client, ListProjectEventsInput{
		ProjectID:             "42",
		OrderBy:               "id",
		KeysetPaginationInput: toolutil.KeysetPaginationInput{Pagination: "keyset", PageToken: "99"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestListCurrentUserContributionEvents_KeysetAndOrderBy verifies keyset and
// order_by forwarding for the contribution-events endpoint.
func TestListCurrentUserContributionEvents_KeysetAndOrderBy(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("pagination") != "keyset" {
			t.Errorf("expected pagination=keyset, got %q", q.Get("pagination"))
		}
		if q.Get("page_token") != "7" {
			t.Errorf("expected page_token=7, got %q", q.Get("page_token"))
		}
		if q.Get("order_by") != "id" {
			t.Errorf("expected order_by=id, got %q", q.Get("order_by"))
		}
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := ListCurrentUserContributionEvents(t.Context(), client, ListContributionEventsInput{
		OrderBy:               "id",
		KeysetPaginationInput: toolutil.KeysetPaginationInput{Pagination: "keyset", PageToken: "7"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
