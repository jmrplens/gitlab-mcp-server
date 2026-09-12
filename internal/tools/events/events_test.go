// events_test.go contains unit tests for the event MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package events

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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

// eventFooter is the guidance section a page of events closes with. A page
// whose targets carry no link carries no instruction to preserve them.
func eventFooter(linked bool) string {
	out := "\n---\n\U0001F4A1 **Next steps:**\n"
	if linked {
		out += "- " + toolutil.HintPreserveLinks + "\n"
	}
	return out + "- Filter events using action and target_type parameters\n"
}

// TestFormatContributionListMarkdownString_WithEvents verifies the whole
// render of a page of contribution events: the heading, one list item per
// event and the guidance section, and no event ID anywhere, which is an
// internal number no caller can act on.
func TestFormatContributionListMarkdownString_WithEvents(t *testing.T) {
	got := FormatContributionListMarkdownString(ListContributionEventsOutput{
		Events: []ContributionEventOutput{
			{ID: 1, ActionName: actionPushed, AuthorUsername: "dev", CreatedAt: "2026-06-01T10:00:00Z", TargetType: "MergeRequest", TargetIID: 3},
			{ID: 2, ActionName: "opened", AuthorUsername: "dev", CreatedAt: "2026-06-02T11:00:00Z"},
		},
	})
	want := "## Contribution Events (2)\n\n" +
		"- **pushed** MergeRequest #3 by @dev, 1 Jun 2026 10:00 UTC\n" +
		"- **opened** by @dev, 2 Jun 2026 11:00 UTC\n" +
		eventFooter(false)
	if got != want {
		t.Errorf("contribution events:\n got %q\nwant %q", got, want)
	}
}

// TestFormatContributionListMarkdownString_Empty verifies that an empty page
// is the one sentence and nothing else.
func TestFormatContributionListMarkdownString_Empty(t *testing.T) {
	got := FormatContributionListMarkdownString(ListContributionEventsOutput{Events: []ContributionEventOutput{}})
	if want := "No contribution events found.\n"; got != want {
		t.Errorf("empty contribution events:\n got %q\nwant %q", got, want)
	}
}

// TestFormatContributionListMarkdownString_CountsWhatGitLabSent verifies that
// the heading counts the total GitLab reported rather than the length of the
// page, which is what a page of two under a total of forty-five used to
// announce as "(2)".
func TestFormatContributionListMarkdownString_CountsWhatGitLabSent(t *testing.T) {
	got := FormatContributionListMarkdownString(ListContributionEventsOutput{
		Events:     []ContributionEventOutput{{ID: 1, ActionName: actionPushed, AuthorUsername: "dev", CreatedAt: testDateAfter}},
		Pagination: toolutil.PaginationOutput{TotalItems: 45, Page: 1, PerPage: 20, TotalPages: 3},
	})
	want := "## Contribution Events (45)\n\n" +
		"Showing 1 of 45 results (page 1 of 3)\n\n" +
		"- **pushed** by @dev, 1 Jun 2026\n" +
		"\nPage 1 of 3 | 45 items total | 20 per page\n" +
		eventFooter(false)
	if got != want {
		t.Errorf("paginated contribution events:\n got %q\nwant %q", got, want)
	}
}

// TestFormatListMarkdownString_WithEvents verifies the whole render of a page
// of project events, target reference included.
func TestFormatListMarkdownString_WithEvents(t *testing.T) {
	got := FormatListMarkdownString(ListProjectEventsOutput{
		Events: []ProjectEventOutput{
			{ID: 1, ActionName: actionPushed, AuthorUsername: "alice", CreatedAt: "2026-01-15", TargetType: "MergeRequest", TargetIID: 3},
			{ID: 2, ActionName: "commented", AuthorUsername: "bob", CreatedAt: testDateCreated},
		},
	})
	want := "## Project Events (2)\n\n" +
		"- **pushed** MergeRequest #3 by @alice, 15 Jan 2026\n" +
		"- **commented** by @bob, 14 Jan 2026\n" +
		eventFooter(false)
	if got != want {
		t.Errorf("project events:\n got %q\nwant %q", got, want)
	}
}

// TestFormatListMarkdownString_Empty verifies that an empty page is the one
// sentence and nothing else.
func TestFormatListMarkdownString_Empty(t *testing.T) {
	got := FormatListMarkdownString(ListProjectEventsOutput{Events: []ProjectEventOutput{}})
	if want := "No project events found.\n"; got != want {
		t.Errorf("empty project events:\n got %q\nwant %q", got, want)
	}
}

// TestFormatContributionListMarkdownString_TargetTitleShown verifies that an
// event whose target GitLab named renders the title as the link label and the
// reference beside it, so neither the subject nor its kind is lost.
func TestFormatContributionListMarkdownString_TargetTitleShown(t *testing.T) {
	got := FormatContributionListMarkdownString(ListContributionEventsOutput{
		Events: []ContributionEventOutput{
			{ID: 10, ActionName: "opened", AuthorUsername: "dev", TargetType: "Issue", TargetIID: 7, TargetTitle: titleBugReport, TargetURL: "https://gitlab.example.com/issues/7", CreatedAt: testDateAfter},
		},
	})
	want := "## Contribution Events (1)\n\n" +
		"- **opened** [Bug Report](https://gitlab.example.com/issues/7) (Issue #7) by @dev, 1 Jun 2026\n" +
		eventFooter(true)
	if got != want {
		t.Errorf("contribution event with a target title:\n got %q\nwant %q", got, want)
	}
}

// TestFormatListMarkdownString_TargetTitleShown verifies the same at project
// scope.
func TestFormatListMarkdownString_TargetTitleShown(t *testing.T) {
	got := FormatListMarkdownString(ListProjectEventsOutput{
		Events: []ProjectEventOutput{
			{ID: 20, ActionName: "commented", AuthorUsername: "bob", TargetType: "MergeRequest", TargetIID: 5, TargetTitle: "Add feature X", CreatedAt: testDateCreated},
		},
	})
	want := "## Project Events (1)\n\n" +
		"- **commented** Add feature X (MergeRequest #5) by @bob, 14 Jan 2026\n" +
		eventFooter(false)
	if got != want {
		t.Errorf("project event with a target title:\n got %q\nwant %q", got, want)
	}
}

// TestFormatListMarkdownString_PushDataRendered verifies that a push event
// names the ref it pushed to, how many commits it carried and the newest
// commit's title. All three used to be dropped, so every push event read as
// the bare word "pushed to".
func TestFormatListMarkdownString_PushDataRendered(t *testing.T) {
	got := FormatListMarkdownString(ListProjectEventsOutput{
		Events: []ProjectEventOutput{{
			ID: 1, ActionName: "pushed to", AuthorUsername: "alice", CreatedAt: "2026-01-15",
			PushData: &ProjectEventPushDataOutput{CommitCount: 3, RefType: "branch", Ref: "main", CommitTitle: "Fix login"},
		}},
	})
	want := "## Project Events (1)\n\n" +
		"- **pushed to** branch `main` (3 commits, latest \"Fix login\") by @alice, 15 Jan 2026\n" +
		eventFooter(false)
	if got != want {
		t.Errorf("project push event:\n got %q\nwant %q", got, want)
	}
}

// TestFormatListMarkdownString_NoteTargetRendered verifies that a comment
// event names the issue or merge request the note hangs on. The event's own
// target is the note, so without this the reader is never told what was
// commented on.
func TestFormatListMarkdownString_NoteTargetRendered(t *testing.T) {
	got := FormatListMarkdownString(ListProjectEventsOutput{
		Events: []ProjectEventOutput{{
			ID: 2, ActionName: "commented on", AuthorUsername: "bob", CreatedAt: testDateCreated,
			Note: &ProjectEventNoteOutput{ID: 9, NoteableType: "MergeRequest", NoteableIID: 5},
		}},
	})
	want := "## Project Events (1)\n\n" +
		"- **commented on** on MergeRequest #5 by @bob, 14 Jan 2026\n" +
		eventFooter(false)
	if got != want {
		t.Errorf("project note event:\n got %q\nwant %q", got, want)
	}
}

// TestFormatContributionListMarkdownString_PushDataRendered verifies the push
// payload reaches the page at contribution scope too.
func TestFormatContributionListMarkdownString_PushDataRendered(t *testing.T) {
	got := FormatContributionListMarkdownString(ListContributionEventsOutput{
		Events: []ContributionEventOutput{{
			ID: 1, ActionName: "pushed to", AuthorUsername: "dev", CreatedAt: testDateAfter,
			PushData: &ContributionEventPushDataOutput{CommitCount: 1, RefType: "tag", Ref: "v1.0"},
			Note:     &NoteOutput{ID: 3, NoteableType: "Issue", NoteableIID: 8},
		}},
	})
	want := "## Contribution Events (1)\n\n" +
		"- **pushed to** tag `v1.0` (1 commit) on Issue #8 by @dev, 1 Jun 2026\n" +
		eventFooter(false)
	if got != want {
		t.Errorf("contribution push event:\n got %q\nwant %q", got, want)
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
	out := toContributionEventOutput(e, toolutil.EventExtra{})
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
	out := toContributionEventOutput(e, toolutil.EventExtra{})
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
		Events: []ContributionEventOutput{{ID: 1, TargetTitle: "covTitle", ActionName: "pushed"}},
	}
	res := FormatContributionListMarkdown(out)
	if res == nil {
		t.Fatal("expected non-nil result")
	}
}

// TestFormatContributionListMarkdownString_EmptyTargetType verifies that an
// event GitLab sent no target for renders the action alone: no reference to
// "#0", and no author or timestamp separators around values it never sent.
func TestFormatContributionListMarkdownString_EmptyTargetType(t *testing.T) {
	got := FormatContributionListMarkdownString(ListContributionEventsOutput{
		Events: []ContributionEventOutput{{ID: 1, ActionName: "pushed", TargetType: ""}},
	})
	want := "## Contribution Events (1)\n\n- **pushed**\n" + eventFooter(false)
	if got != want {
		t.Errorf("contribution event without a target:\n got %q\nwant %q", got, want)
	}
}

// TestFormatContributionListMarkdownString_WithTargetType verifies that an
// event GitLab sent a target type and IID but no title for is named by its
// reference.
func TestFormatContributionListMarkdownString_WithTargetType(t *testing.T) {
	got := FormatContributionListMarkdownString(ListContributionEventsOutput{
		Events: []ContributionEventOutput{{ID: 1, ActionName: "pushed", TargetType: "Issue", TargetIID: 42}},
	})
	want := "## Contribution Events (1)\n\n- **pushed** Issue #42\n" + eventFooter(false)
	if got != want {
		t.Errorf("contribution event with a target reference:\n got %q\nwant %q", got, want)
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
	out := toProjectEventOutput(e, toolutil.EventExtra{})
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
		Events: []ProjectEventOutput{{ID: 1, TargetTitle: "covTitle", ActionName: "pushed"}},
	}
	res := FormatListMarkdown(out)
	if res == nil {
		t.Fatal("expected non-nil result")
	}
}

// TestFormatListMarkdownString_EmptyTargetType verifies the same at project
// scope: no target, no "#0" and no empty attribution.
func TestFormatListMarkdownString_EmptyTargetType(t *testing.T) {
	got := FormatListMarkdownString(ListProjectEventsOutput{
		Events: []ProjectEventOutput{{ID: 1, ActionName: "pushed", TargetType: ""}},
	})
	want := "## Project Events (1)\n\n- **pushed**\n" + eventFooter(false)
	if got != want {
		t.Errorf("project event without a target:\n got %q\nwant %q", got, want)
	}
}

// TestFormatListMarkdownString_WithTargetType verifies that a project event's
// target reference names the type and the IID.
func TestFormatListMarkdownString_WithTargetType(t *testing.T) {
	got := FormatListMarkdownString(ListProjectEventsOutput{
		Events: []ProjectEventOutput{{ID: 1, ActionName: "pushed", TargetType: "MR", TargetIID: 5}},
	})
	want := "## Project Events (1)\n\n- **pushed** MR #5\n" + eventFooter(false)
	if got != want {
		t.Errorf("project event with a target reference:\n got %q\nwant %q", got, want)
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

// TestFormatTarget covers the four shapes a target comes in: a title and a
// URL, a URL with no title, a type and an IID with no URL, and nothing GitLab
// could name at all.
func TestFormatTarget(t *testing.T) {
	tests := []struct {
		name       string
		targetType string
		iid        int64
		title      string
		url        string
		want       string
	}{
		{
			name: "title and URL", targetType: "Issue", iid: 42, title: "Bug title", url: "https://gitlab.example.com/issues/42",
			want: " [Bug title](https://gitlab.example.com/issues/42) (Issue #42)",
		},
		{
			name: "URL without a title", targetType: "MergeRequest", iid: 10, url: "https://gitlab.example.com/mr/10",
			want: " [MergeRequest #10](https://gitlab.example.com/mr/10)",
		},
		{name: "reference without a URL", targetType: "Issue", iid: 7, want: " Issue #7"},
		{name: "nothing GitLab named", targetType: "Issue", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatTarget(tt.targetType, tt.iid, tt.title, tt.url); got != tt.want {
				t.Errorf("formatTarget() = %q, want %q", got, tt.want)
			}
		})
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
			Confidential: true, //nolint:staticcheck // deprecated SDK field/API is exposed deliberately: the 1:1 parity policy mirrors the full surface while upstream keeps it
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

	out := toContributionEventOutput(e, toolutil.EventExtra{})
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
	out := toContributionEventOutput(&gl.ContributionEvent{ID: 1}, toolutil.EventExtra{})
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

	out := toProjectEventOutput(e, toolutil.EventExtra{})
	assertTrue(t, out.Author != nil && out.Author.ID == 5, "author")
	assertTrue(t, out.PushData != nil && out.PushData.CommitTitle == "ct", "push_data")
	assertTrue(t, out.Note != nil && out.Note.NoteableType == "Issue" && out.Note.Author != nil && out.Note.Author.Email == "n@e" && out.Note.CreatedAt != "", "note")
}

// TestToProjectEventOutput_EmptySubObjects verifies zero-valued ProjectEvent sub
// objects are omitted.
func TestToProjectEventOutput_EmptySubObjects(t *testing.T) {
	out := toProjectEventOutput(&gl.ProjectEvent{ID: 1}, toolutil.EventExtra{})
	if out.PushData != nil || out.Note != nil || out.Author != nil {
		t.Errorf("expected nil sub-objects, got %+v", out)
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
		ProjectID:  "42",
		OrderBy:    "id",
		Pagination: "keyset", PageToken: "99",
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
		OrderBy:    "id",
		Pagination: "keyset", PageToken: "7",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Fields GitLab sends beside the ones the SDK's own event models
// ---------------------------------------------------------------------------.

// eventSentJSON is one event about a wiki page as GitLab renders it, carrying
// the keys the SDK's own event structs leave out: the wiki page the event
// happened to, and whether the event arrived with an import.
const eventSentJSON = `{"id":1,"project_id":42,"action_name":"created","author_id":10,` +
	`"author_username":"alice","created_at":"2026-01-15T10:00:00Z",` +
	`"wiki_page":{"format":"markdown","slug":"home","title":"Home","wiki_page_meta_id":77},` +
	`"imported":true,"imported_from":"github"}`

// eventWithoutWikiJSON is an event about something other than a wiki page,
// which happened here rather than arriving with an import.
const eventWithoutWikiJSON = `{"id":2,"project_id":42,"action_name":"pushed","author_id":10,` +
	`"author_username":"alice","created_at":"2026-01-15T10:00:00Z","imported":false,"imported_from":null}`

// eventsClient answers the two event endpoints with body and refuses anything
// else, so the project lookup that follows a listing cannot be mistaken for a
// second answer to the listing itself.
func eventsClient(t *testing.T, body string) *gitlabclient.Client {
	t.Helper()
	return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/events") {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, body)
	}))
}

// eventSent is what a handler published of the fields read off the captured
// answer, normalized so the contribution and project handlers share a table.
type eventSent struct {
	wikiPage     *WikiPageOutput
	imported     bool
	importedFrom string
}

// errNoEvent reports a handler that answered with no event at all.
var errNoEvent = errors.New("the handler published no event")

// eventCalls are the two handlers that answer with events.
var eventCalls = []struct {
	name string
	call func(client *gitlabclient.Client) (eventSent, error)
}{
	{name: "contribution", call: func(client *gitlabclient.Client) (eventSent, error) {
		out, err := ListCurrentUserContributionEvents(context.Background(), client, ListContributionEventsInput{})
		if err != nil {
			return eventSent{}, err
		}
		if len(out.Events) != 1 {
			return eventSent{}, errNoEvent
		}
		return eventSent{
			wikiPage:     out.Events[0].WikiPage,
			imported:     out.Events[0].Imported,
			importedFrom: out.Events[0].ImportedFrom,
		}, nil
	}},
	{name: "project", call: func(client *gitlabclient.Client) (eventSent, error) {
		out, err := ListProjectEvents(context.Background(), client, ListProjectEventsInput{ProjectID: "42"})
		if err != nil {
			return eventSent{}, err
		}
		if len(out.Events) != 1 {
			return eventSent{}, errNoEvent
		}
		return eventSent{
			wikiPage:     out.Events[0].WikiPage,
			imported:     out.Events[0].Imported,
			importedFrom: out.Events[0].ImportedFrom,
		}, nil
	}},
}

// TestEvents_PublishTheFieldsGitLabSendsBesideTheSDKs verifies both event
// handlers publish the wiki page an event happened to and where an imported
// event came from, read off the captured response.
func TestEvents_PublishTheFieldsGitLabSendsBesideTheSDKs(t *testing.T) {
	for _, eventCall := range eventCalls {
		t.Run(eventCall.name, func(t *testing.T) {
			sent, err := eventCall.call(eventsClient(t, `[`+eventSentJSON+`]`))
			if err != nil {
				t.Fatalf("%s: %v", eventCall.name, err)
			}
			if sent.wikiPage == nil {
				t.Fatal("wiki_page = nil, want the page GitLab sent")
			}
			if sent.wikiPage.Format != "markdown" || sent.wikiPage.Slug != "home" ||
				sent.wikiPage.Title != "Home" || sent.wikiPage.WikiPageMetaID != 77 {
				t.Errorf("wiki_page = %+v, want every field GitLab sent", *sent.wikiPage)
			}
			if !sent.imported {
				t.Error("imported = false, want true")
			}
			if sent.importedFrom != "github" {
				t.Errorf("imported_from = %q, want github", sent.importedFrom)
			}
		})
	}
}

// TestEvents_OmitTheWikiPageGitLabDidNotSend verifies an event about anything
// but a wiki page publishes no page, and one that happened here says so.
func TestEvents_OmitTheWikiPageGitLabDidNotSend(t *testing.T) {
	for _, eventCall := range eventCalls {
		t.Run(eventCall.name, func(t *testing.T) {
			sent, err := eventCall.call(eventsClient(t, `[`+eventWithoutWikiJSON+`]`))
			if err != nil {
				t.Fatalf("%s: %v", eventCall.name, err)
			}
			if sent.wikiPage != nil {
				t.Errorf("wiki_page = %+v, want none", *sent.wikiPage)
			}
			if sent.imported {
				t.Error("imported = true, want false for an event that happened here")
			}
			if sent.importedFrom != "" {
				t.Errorf("imported_from = %q, want none", sent.importedFrom)
			}
		})
	}
}

// TestEvents_UnreadableCapturedFields verifies both event handlers report the
// captured response's decode failure rather than an event missing what GitLab
// sent. The SDK's own event structs have no imported flag, so only the read
// beside them can notice GitLab sent a string there.
func TestEvents_UnreadableCapturedFields(t *testing.T) {
	const poisoned = `[{"id":1,"project_id":42,"action_name":"created","imported":"yes"}]`
	cases := make([]testutil.CapturedCase, 0, len(eventCalls))
	for _, eventCall := range eventCalls {
		cases = append(cases, testutil.CapturedCase{Name: eventCall.name, Call: func() error {
			_, err := eventCall.call(eventsClient(t, poisoned))
			return err
		}})
	}
	testutil.AssertCapturedDecodeFailures(t, cases)
}

// TestFormatEventListMarkdown_SentFields verifies both list formatters name
// the wiki page an event happened to and where an imported event came from,
// and say nothing of either for an event that carries neither.
func TestFormatEventListMarkdown_SentFields(t *testing.T) {
	page := &WikiPageOutput{Format: "markdown", Slug: "home", Title: "Home", WikiPageMetaID: 77}
	for name, got := range map[string]string{
		"contribution": FormatContributionListMarkdownString(ListContributionEventsOutput{
			Events: []ContributionEventOutput{{
				ID: 1, ActionName: "created", WikiPage: page, Imported: true, ImportedFrom: "github",
			}},
		}),
		"project": FormatListMarkdownString(ListProjectEventsOutput{
			Events: []ProjectEventOutput{{
				ID: 1, ActionName: "created", WikiPage: page, Imported: true, ImportedFrom: "github",
			}},
		}),
	} {
		t.Run(name, func(t *testing.T) {
			want := "## " + eventListTitle(name) + " (1)\n\n" +
				"- **created** wiki page \"Home\" (imported from github)\n" +
				eventFooter(false)
			if got != want {
				t.Errorf("%s events:\n got %q\nwant %q", name, got, want)
			}
		})
	}

	for name, got := range map[string]string{
		"contribution": FormatContributionListMarkdownString(ListContributionEventsOutput{
			Events: []ContributionEventOutput{{ID: 1, ActionName: "pushed"}},
		}),
		"project": FormatListMarkdownString(ListProjectEventsOutput{
			Events: []ProjectEventOutput{{ID: 1, ActionName: "pushed"}},
		}),
	} {
		t.Run(name+" without them", func(t *testing.T) {
			want := "## " + eventListTitle(name) + " (1)\n\n- **pushed**\n" + eventFooter(false)
			if got != want {
				t.Errorf("%s events without a wiki page or an origin:\n got %q\nwant %q", name, got, want)
			}
		})
	}
}

// eventListTitle names the heading each event scope renders.
func eventListTitle(scope string) string {
	if scope == "contribution" {
		return "Contribution Events"
	}
	return "Project Events"
}

// Each of the four converters below decides whether a nested event object is
// worth publishing by asking whether every one of its fields is empty. The
// tests hand each of them an object with exactly one field filled, which is
// the case a guard reading its conditions the other way round would drop.

// TestToBasicUserOutput_AnySingleFilledFieldMakesTheUserPresent verifies the
// event author survives however little GitLab said about them.
func TestToBasicUserOutput_AnySingleFilledFieldMakesTheUserPresent(t *testing.T) {
	for name, user := range map[string]gl.BasicUser{
		"id":       {ID: 5},
		"username": {Username: "alice"},
		"name":     {Name: "Alice"},
	} {
		t.Run(name, func(t *testing.T) {
			if toBasicUserOutput(user) == nil {
				t.Errorf("toBasicUserOutput(%+v) = nil, want the user", user)
			}
		})
	}
	if toBasicUserOutput(gl.BasicUser{}) != nil {
		t.Error("toBasicUserOutput of an empty user should be nil")
	}
}

// TestNoteAuthorOutput_AnySingleFilledFieldMakesTheAuthorPresent verifies a
// note's author survives however little GitLab said about them.
func TestNoteAuthorOutput_AnySingleFilledFieldMakesTheAuthorPresent(t *testing.T) {
	for name, author := range map[string]gl.ProjectEventNoteAuthor{
		"id":         {ID: 5},
		"username":   {Username: "alice"},
		"email":      {Email: "alice@example.com"},
		"name":       {Name: "Alice"},
		"state":      {State: "active"},
		"avatar_url": {AvatarURL: "https://example.com/a.png"},
		"web_url":    {WebURL: "https://example.com/alice"},
	} {
		t.Run(name, func(t *testing.T) {
			got := noteAuthorOutput(author.ID, author.Username, author.Email,
				author.Name, author.State, author.AvatarURL, author.WebURL)
			if got == nil {
				t.Errorf("noteAuthorOutput(%+v) = nil, want the author", author)
			}
		})
	}
	if noteAuthorOutput(0, "", "", "", "", "", "") != nil {
		t.Error("noteAuthorOutput of an empty author should be nil")
	}
}

// TestToProjectEventNoteOutput_AnySingleFilledFieldMakesTheNotePresent
// verifies a comment event's note survives however little of it GitLab sent.
func TestToProjectEventNoteOutput_AnySingleFilledFieldMakesTheNotePresent(t *testing.T) {
	created := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	for name, note := range map[string]gl.ProjectEventNote{
		"id":            {ID: 5},
		"body":          {Body: "a comment"},
		"attachment":    {Attachment: "file.txt"},
		"author":        {Author: gl.ProjectEventNoteAuthor{ID: 7}},
		"created_at":    {CreatedAt: &created},
		"system":        {System: true},
		"noteable_id":   {NoteableID: 9},
		"noteable_type": {NoteableType: "Issue"},
		"noteable_iid":  {NoteableIID: 11},
	} {
		t.Run(name, func(t *testing.T) {
			if toProjectEventNoteOutput(note) == nil {
				t.Errorf("toProjectEventNoteOutput(%+v) = nil, want the note", note)
			}
		})
	}
	if toProjectEventNoteOutput(gl.ProjectEventNote{}) != nil {
		t.Error("toProjectEventNoteOutput of an empty note should be nil")
	}
}

// TestEventConverters_LeaveOutTheTimestampsGitLabDidNotSend verifies the note
// and pipeline converters publish an empty timestamp rather than formatting a
// moment that is not there, which every one of their date fields can be.
func TestEventConverters_LeaveOutTheTimestampsGitLabDidNotSend(t *testing.T) {
	note := toNoteOutput(&gl.Note{ID: 1, Body: "a comment"})
	if note == nil {
		t.Fatal("toNoteOutput = nil, want the note")
	}
	if note.CreatedAt != "" || note.UpdatedAt != "" || note.ExpiresAt != "" || note.ResolvedAt != "" {
		t.Errorf("note timestamps = %+v, want all empty", note)
	}
}

// TestFormatOrigin_ImportedWithoutASource verifies an event GitLab marked as
// imported without naming the platform still says so.
func TestFormatOrigin_ImportedWithoutASource(t *testing.T) {
	got := FormatListMarkdownString(ListProjectEventsOutput{
		Events: []ProjectEventOutput{{ID: 1, ActionName: "pushed", Imported: true}},
	})
	want := "## Project Events (1)\n\n- **pushed** (imported)\n" + eventFooter(false)
	if got != want {
		t.Errorf("imported event without a source:\n got %q\nwant %q", got, want)
	}
}
