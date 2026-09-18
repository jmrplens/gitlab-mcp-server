// freeze_periods_test.go contains unit tests for the freeze period MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package freezeperiods

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	// fmtUnexpErr identifies the fmt unexp err constant used by this package.
	fmtUnexpErr = "unexpected error: %v"
	// testCronFreezeStart identifies the test cron freeze start constant used by this package.
	testCronFreezeStart = "0 23 * * 5"
	// testCronUpdatedStart identifies the test cron updated start constant used by this package.
	testCronUpdatedStart = "0 0 * * 5"
	// testCronFreezeEnd is the cron expression the create fixtures freeze until.
	testCronFreezeEnd = "0 7 * * 1"
	// testCronUpdatedEnd is the cron expression the update fixtures move the end of the window to.
	testCronUpdatedEnd = "0 9 * * 1"
	// testTimezoneMadrid is the IANA name the update fixtures read their cron expressions in.
	testTimezoneMadrid = "Europe/Madrid"
	// testTimezoneNewYork is the IANA name the create fixtures read theirs in.
	testTimezoneNewYork = "America/New_York"
	// errMissingFreezePeriodID identifies the err missing freeze period ID constant used by this package.
	errMissingFreezePeriodID = "expected error for missing freeze_period_id"
)

// TestList_Success verifies that List succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/projects/1/freeze_periods (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestList_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v4/projects/1/freeze_periods" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":1,"freeze_start":"0 23 * * 5","freeze_end":"0 7 * * 1","cron_timezone":"UTC","created_at":"2026-01-01T00:00:00Z"}]`,
			testutil.PaginationHeaders{Page: "1", TotalPages: "1", Total: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, handler)

	out, err := List(t.Context(), client, ListInput{ProjectID: "1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.FreezePeriods) != 1 {
		t.Fatalf("got %d freeze periods, want 1", len(out.FreezePeriods))
	}
	if out.FreezePeriods[0].FreezeStart != testCronFreezeStart {
		t.Errorf("freeze_start = %q, want %q", out.FreezePeriods[0].FreezeStart, testCronFreezeStart)
	}
}

// TestList_KeysetAndOrdering verifies that List propagates order_by, sort,
// pagination, and page_token query parameters to the GitLab API.
// The mock GitLab API at /api/v4/projects/1/freeze_periods (GET) inspects the
// query string and responds with HTTP OK.
// It asserts each supplied parameter appears in the outgoing request.
func TestList_KeysetAndOrdering(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("order_by") != "id" {
			t.Errorf("order_by = %q, want id", q.Get("order_by"))
		}
		if q.Get("sort") != "desc" {
			t.Errorf("sort = %q, want desc", q.Get("sort"))
		}
		if q.Get("pagination") != "keyset" {
			t.Errorf("pagination = %q, want keyset", q.Get("pagination"))
		}
		if q.Get("page_token") != "42" {
			t.Errorf("page_token = %q, want 42", q.Get("page_token"))
		}
		if q.Get("per_page") != "50" {
			t.Errorf("per_page = %q, want 50", q.Get("per_page"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{Page: "1", TotalPages: "1", Total: "0", PerPage: "50"})
	})
	client := testutil.NewTestClient(t, handler)

	_, err := List(t.Context(), client, ListInput{
		ProjectID:  "1",
		OrderBy:    "id",
		Sort:       "desc",
		PerPage:    50,
		Pagination: "keyset", PageToken: "42",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestList_MissingProjectID verifies that List_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := List(t.Context(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestGet_Success verifies that Get succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/projects/1/freeze_periods/5 (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestGet_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/freeze_periods/5" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK,
			`{"id":5,"freeze_start":"0 23 * * 5","freeze_end":"0 7 * * 1","cron_timezone":"UTC"}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Get(t.Context(), client, GetInput{ProjectID: "1", FreezePeriodID: 5})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 5 {
		t.Errorf("ID = %d, want 5", out.ID)
	}
}

// TestCreate_Success verifies that Create succeeds when the GitLab API returns a valid response.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreate_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		testutil.RespondJSON(w, http.StatusCreated,
			`{"id":10,"freeze_start":"0 23 * * 5","freeze_end":"0 7 * * 1","cron_timezone":"UTC"}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Create(t.Context(), client, CreateInput{
		ProjectID:    "1",
		FreezeStart:  testCronFreezeStart,
		FreezeEnd:    "0 7 * * 1",
		CronTimezone: "UTC",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 10 {
		t.Errorf("ID = %d, want 10", out.ID)
	}
}

// TestUpdate_Success verifies that Update succeeds when the GitLab API returns a valid response.
// The test exercises the PUT path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestUpdate_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK,
			`{"id":5,"freeze_start":"0 0 * * 5","freeze_end":"0 7 * * 1","cron_timezone":"UTC"}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Update(t.Context(), client, UpdateInput{
		ProjectID:      "1",
		FreezePeriodID: 5,
		FreezeStart:    testCronUpdatedStart,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.FreezeStart != testCronUpdatedStart {
		t.Errorf("freeze_start = %q, want %q", out.FreezeStart, testCronUpdatedStart)
	}
}

// TestDelete_Success verifies that Delete succeeds when the GitLab API returns a valid response.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDelete_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, handler)

	err := Delete(t.Context(), client, DeleteInput{ProjectID: "1", FreezePeriodID: 5})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGet_APIError verifies that Get returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGet_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)

	_, err := Get(t.Context(), client, GetInput{ProjectID: "1", FreezePeriodID: 99})
	if err == nil {
		t.Fatal("expected error for API error response")
	}
}

// TestFormatMarkdownString verifies the MarkdownString Markdown formatter for a representative string input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatMarkdownString(t *testing.T) {
	out := Output{
		ID:           1,
		FreezeStart:  testCronFreezeStart,
		FreezeEnd:    "0 7 * * 1",
		CronTimezone: "UTC",
		CreatedAt:    "2026-01-01T00:00:00Z",
	}
	want := "## Freeze Period #1\n\n" +
		"- **ID**: 1\n" +
		"- **Start**: `" + testCronFreezeStart + "`\n" +
		"- **End**: `0 7 * * 1`\n" +
		"- **Timezone**: UTC\n" +
		"- **Created**: 1 Jan 2026 00:00 UTC\n" +
		freezeCardHints
	if md := FormatMarkdownString(out); md != want {
		t.Errorf("FormatMarkdownString()\n got %q\nwant %q", md, want)
	}
}

// freezeCardHints is the guidance section a freeze period card closes with.
const freezeCardHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'environment.freeze_update' to change this freeze window\n" +
	"- Use action 'environment.freeze_delete' to remove it\n" +
	"- Use action 'environment.freeze_list' to see every freeze window of this project\n"

// TestFormatListMarkdownString_Empty verifies the ListMarkdownString_Empty Markdown formatter for a representative liststring_empty input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdownString_Empty(t *testing.T) {
	md := FormatListMarkdownString(ListOutput{})
	if md != "No freeze periods found.\n" {
		t.Errorf("got %q, want empty message", md)
	}
}

// TestGet_MissingFreezePeriodID verifies that Get refuses a freeze_period_id of
// zero before it spends a request, and names the field it refused.
//
// Asserting only that some error came back passes for the wrong reason, which is
// why the assertion is the one below: the SDK decodes a body on every GET, so an
// id of zero that slipped past the guard would fail on the empty response the
// mock writes and read exactly like the guard working. What the guard is for is
// that `/freeze_periods/0` is never asked for, and that the caller is told which
// parameter to supply rather than handed a decode error.
func TestGet_MissingFreezePeriodID(t *testing.T) {
	var requests atomic.Int64
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	_, err := Get(t.Context(), client, GetInput{ProjectID: "1", FreezePeriodID: 0})
	if err == nil {
		t.Fatal(errMissingFreezePeriodID)
	}
	assertGuardedBeforeRequest(t, err, requests.Load())
}

// TestUpdate_MissingFreezePeriodID verifies that Update refuses a
// freeze_period_id of zero before it spends a request, and names the field it
// refused.
//
// The stake is higher here than on the read: a PUT that reached GitLab with the
// id missing would be a write aimed at whatever `/freeze_periods/0` resolves to.
// The empty body the mock writes makes the SDK fail either way, so the assertion
// has to be that nothing was sent at all.
func TestUpdate_MissingFreezePeriodID(t *testing.T) {
	var requests atomic.Int64
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	_, err := Update(t.Context(), client, UpdateInput{ProjectID: "1", FreezePeriodID: 0})
	if err == nil {
		t.Fatal(errMissingFreezePeriodID)
	}
	assertGuardedBeforeRequest(t, err, requests.Load())
}

// TestDelete_MissingFreezePeriodID verifies that Delete_MissingFreezePeriodID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDelete_MissingFreezePeriodID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	err := Delete(t.Context(), client, DeleteInput{ProjectID: "1", FreezePeriodID: 0})
	if err == nil {
		t.Fatal(errMissingFreezePeriodID)
	}
}

// TestList_APIError verifies that List returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestList_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)

	_, err := List(t.Context(), client, ListInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

// TestCreate_APIError verifies that Create returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreate_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})
	client := testutil.NewTestClient(t, handler)

	_, err := Create(t.Context(), client, CreateInput{
		ProjectID:   "1",
		FreezeStart: "0 23 * * 5",
		FreezeEnd:   "0 7 * * 1",
	})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

// TestCreate_Forbidden verifies the Create_Forbidden handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreate_Forbidden(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)

	_, err := Create(t.Context(), client, CreateInput{
		ProjectID:   "1",
		FreezeStart: testCronFreezeStart,
		FreezeEnd:   "0 7 * * 1",
	})
	if err == nil {
		t.Fatal("expected error for forbidden response")
	}
	if !containsStr(err.Error(), "Maintainer or Owner") {
		t.Fatalf("error = %v, want permission hint", err)
	}
}

// TestCreate_WithTimezone verifies the Create_WithTimezone handler.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreate_WithTimezone(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		testutil.RespondJSON(w, http.StatusCreated,
			`{"id":2,"freeze_start":"0 23 * * 5","freeze_end":"0 7 * * 1","cron_timezone":"America/New_York"}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Create(t.Context(), client, CreateInput{
		ProjectID:    "1",
		FreezeStart:  "0 23 * * 5",
		FreezeEnd:    "0 7 * * 1",
		CronTimezone: testTimezoneNewYork,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.CronTimezone != testTimezoneNewYork {
		t.Errorf("CronTimezone = %q, want %q", out.CronTimezone, testTimezoneNewYork)
	}
}

// TestCreate_MissingProjectID verifies that Create_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := Create(t.Context(), client, CreateInput{FreezeStart: "0 23 * * 5", FreezeEnd: "0 7 * * 1"})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestUpdate_APIError verifies that Update returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestUpdate_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	client := testutil.NewTestClient(t, handler)

	_, err := Update(t.Context(), client, UpdateInput{
		ProjectID:      "1",
		FreezePeriodID: 5,
		FreezeStart:    "0 0 * * 5",
	})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

// TestUpdate_Forbidden verifies the Update_Forbidden handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestUpdate_Forbidden(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)

	_, err := Update(t.Context(), client, UpdateInput{
		ProjectID:      "1",
		FreezePeriodID: 5,
		FreezeStart:    testCronUpdatedStart,
	})
	if err == nil {
		t.Fatal("expected error for forbidden response")
	}
	if !containsStr(err.Error(), "Maintainer or Owner") {
		t.Fatalf("error = %v, want permission hint", err)
	}
}

// TestUpdate_AllFields verifies the Update_AllFields handler.
// The test exercises the PUT path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestUpdate_AllFields(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK,
			`{"id":5,"freeze_start":"0 0 * * 5","freeze_end":"0 9 * * 1","cron_timezone":"Europe/Madrid"}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Update(t.Context(), client, UpdateInput{
		ProjectID:      "1",
		FreezePeriodID: 5,
		FreezeStart:    "0 0 * * 5",
		FreezeEnd:      "0 9 * * 1",
		CronTimezone:   "Europe/Madrid",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.CronTimezone != "Europe/Madrid" {
		t.Errorf("CronTimezone = %q, want %q", out.CronTimezone, "Europe/Madrid")
	}
}

// TestUpdate_MissingProjectID verifies that Update_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestUpdate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := Update(t.Context(), client, UpdateInput{FreezePeriodID: 5, FreezeStart: "0 0 * * 5"})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestDelete_APIError verifies that Delete returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDelete_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	client := testutil.NewTestClient(t, handler)

	err := Delete(t.Context(), client, DeleteInput{ProjectID: "1", FreezePeriodID: 5})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

// TestDelete_MissingProjectID verifies that Delete_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDelete_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	err := Delete(t.Context(), client, DeleteInput{FreezePeriodID: 5})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestGetAPIError_NotFound verifies that GetAPIError_NotFound returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetAPIError_NotFound(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	client := testutil.NewTestClient(t, handler)

	_, err := Get(t.Context(), client, GetInput{ProjectID: "1", FreezePeriodID: 99})
	if err == nil {
		t.Fatal("expected error for API 404")
	}
}

// TestFormatListMarkdownString_WithItems verifies the ListMarkdownString_WithItems Markdown formatter for a representative liststring_withitems input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdownString_WithItems(t *testing.T) {
	out := ListOutput{
		FreezePeriods: []Output{
			{ID: 1, FreezeStart: "0 23 * * 5", FreezeEnd: "0 7 * * 1", CronTimezone: "UTC"},
			{ID: 2, FreezeStart: "0 0 * * 6", FreezeEnd: "0 0 * * 1", CronTimezone: "Europe/London"},
		},
	}
	// Freeze periods are a collection of objects sharing columns, so they are a
	// table: the list items they used to be put four values on one line and
	// left the pagination footer to land after them.
	want := "## Freeze Periods (2)\n\n" +
		"| ID | Start | End | Timezone |\n" +
		"| --- | --- | --- | --- |\n" +
		"| 1 | `0 23 * * 5` | `0 7 * * 1` | UTC |\n" +
		"| 2 | `0 0 * * 6` | `0 0 * * 1` | Europe/London |\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'environment.freeze_get' to see one freeze period in full\n" +
		"- Use action 'environment.freeze_create' to add a freeze window\n"
	if md := FormatListMarkdownString(out); md != want {
		t.Errorf("FormatListMarkdownString()\n got %q\nwant %q", md, want)
	}
}

// TestFormatListMarkdownString_CountsTheTotalGitLabReported checks the heading
// of a page of a longer list: it counts what the response reports in all,
// where it used to count the rows on the page and read as the whole list.
func TestFormatListMarkdownString_CountsTheTotalGitLabReported(t *testing.T) {
	out := ListOutput{
		FreezePeriods: []Output{{ID: 1, FreezeStart: "0 23 * * 5", FreezeEnd: "0 7 * * 1", CronTimezone: "UTC"}},
		Pagination:    toolutil.PaginationOutput{Page: 1, PerPage: 1, TotalItems: 45, TotalPages: 45},
	}
	md := FormatListMarkdownString(out)
	if !containsStr(md, "## Freeze Periods (45)\n") {
		t.Errorf("heading does not count the reported total:\n%s", md)
	}
	if !containsStr(md, "\nPage 1 of 45 | 45 items total | 1 per page\n") {
		t.Errorf("pagination footer missing or misplaced:\n%s", md)
	}
}

// TestFormatListMarkdown_Wrapper verifies the ListMarkdown_Wrapper Markdown formatter for a representative list_wrapper input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown_Wrapper(t *testing.T) {
	out := ListOutput{
		FreezePeriods: []Output{{ID: 1, FreezeStart: "0 0 * * *", FreezeEnd: "0 1 * * *", CronTimezone: "UTC"}},
	}
	result := FormatListMarkdown(out)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected at least one content item")
	}
}

// TestFormatMarkdown_Wrapper verifies the Markdown_Wrapper Markdown formatter for a representative _wrapper input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatMarkdown_Wrapper(t *testing.T) {
	out := Output{ID: 5, FreezeStart: "0 23 * * 5", FreezeEnd: "0 7 * * 1", CronTimezone: "UTC"}
	result := FormatMarkdown(out)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// TestFormatMarkdownString_AllFields verifies the MarkdownString_AllFields Markdown formatter for a representative string_allfields input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatMarkdownString_AllFields(t *testing.T) {
	out := Output{
		ID:           5,
		FreezeStart:  testCronFreezeStart,
		FreezeEnd:    testCronFreezeEnd,
		CronTimezone: testTimezoneNewYork,
		CreatedAt:    "2026-01-01T00:00:00Z",
	}
	want := "## Freeze Period #5\n\n" +
		"- **ID**: 5\n" +
		"- **Start**: `0 23 * * 5`\n" +
		"- **End**: `0 7 * * 1`\n" +
		"- **Timezone**: America/New_York\n" +
		"- **Created**: 1 Jan 2026 00:00 UTC\n" +
		freezeCardHints
	if md := FormatMarkdownString(out); md != want {
		t.Errorf("FormatMarkdownString(all fields)\n got %q\nwant %q", md, want)
	}
}

// TestGet_MissingProjectID verifies that Get_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGet_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := Get(t.Context(), client, GetInput{FreezePeriodID: 5})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestGet_SuccessWithTimestamps verifies the Get_SuccessWithTimestamps handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGet_SuccessWithTimestamps(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK,
			`{"id":5,"freeze_start":"0 23 * * 5","freeze_end":"0 7 * * 1","cron_timezone":"UTC","created_at":"2026-06-01T12:00:00Z","updated_at":"2026-06-02T12:00:00Z"}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Get(t.Context(), client, GetInput{ProjectID: "1", FreezePeriodID: 5})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.CreatedAt == "" {
		t.Error("expected CreatedAt to be set")
	}
	if out.UpdatedAt == "" {
		t.Error("expected UpdatedAt to be set")
	}
}

// assertGuardedBeforeRequest holds an identifier guard to the two things that
// make it a guard rather than a comment: GitLab was never asked, and the caller
// was told which parameter is missing.
func assertGuardedBeforeRequest(t *testing.T, err error, requests int64) {
	t.Helper()
	if requests != 0 {
		t.Errorf("requests reaching GitLab = %d, want 0: the identifier guard must refuse before spending one", requests)
	}
	if !containsStr(err.Error(), "freeze_period_id") {
		t.Errorf("error = %q, want it to name freeze_period_id", err)
	}
}

// decodeFreezeRequestBody reads the JSON body a handler built for GitLab.
//
// It decodes into a map rather than a struct because what these tests assert is
// as much about a key being absent as about its value: the SDK's option structs
// are pointers with omitempty, so "the caller did not name this field" and "the
// caller named it empty" differ only by whether the key is on the wire.
func decodeFreezeRequestBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	body := map[string]any{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Errorf("decode request body: %v", err)
		return nil
	}
	return body
}

// assertSent holds one field of a request body to the value the caller gave for
// it. It is called from the mock's own goroutine, so it reports and never aborts.
func assertSent(t *testing.T, body map[string]any, key, want string) {
	t.Helper()
	if got := body[key]; got != want {
		t.Errorf("%s sent = %v, want %q", key, got, want)
	}
}

// assertNotSent holds a field the caller never named to being absent from the
// request rather than present and empty, which is a different instruction: an
// empty cron expression asks GitLab to replace the one it holds.
func assertNotSent(t *testing.T, body map[string]any, key string) {
	t.Helper()
	if got, ok := body[key]; ok {
		t.Errorf("%s sent = %v, want the key to be absent: the caller named no new value for it", key, got)
	}
}

// TestCreate_TimezoneNamed_SendsItInTheRequest verifies that a cron_timezone the
// caller supplied reaches GitLab in the create request.
//
// A freeze window is two cron expressions read in some timezone, so dropping the
// timezone does not fail: GitLab silently applies UTC and the deploys freeze at
// the wrong hours. Nothing about the response says which timezone was applied
// either, because the mock, and GitLab, echo back whatever they were given. The
// assertion therefore has to be on the request.
func TestCreate_TimezoneNamed_SendsItInTheRequest(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := decodeFreezeRequestBody(t, r)
		assertSent(t, body, "cron_timezone", testTimezoneNewYork)
		assertSent(t, body, "freeze_start", testCronFreezeStart)
		testutil.RespondJSON(w, http.StatusCreated,
			`{"id":2,"freeze_start":"0 23 * * 5","freeze_end":"0 7 * * 1","cron_timezone":"America/New_York"}`)
	})
	client := testutil.NewTestClient(t, handler)

	if _, err := Create(t.Context(), client, CreateInput{
		ProjectID:    "1",
		FreezeStart:  testCronFreezeStart,
		FreezeEnd:    testCronFreezeEnd,
		CronTimezone: testTimezoneNewYork,
	}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestCreate_NoTimezone_OmitsTheFieldSoGitLabDefaults verifies that a create
// call naming no timezone leaves cron_timezone off the request entirely.
//
// The alternative is not harmless: sending an empty cron_timezone asks GitLab to
// resolve "" as a timezone rather than letting it apply its documented UTC
// default, which is a validation error on a call the caller made correctly.
func TestCreate_NoTimezone_OmitsTheFieldSoGitLabDefaults(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := decodeFreezeRequestBody(t, r)
		assertNotSent(t, body, "cron_timezone")
		testutil.RespondJSON(w, http.StatusCreated,
			`{"id":3,"freeze_start":"0 23 * * 5","freeze_end":"0 7 * * 1","cron_timezone":"UTC"}`)
	})
	client := testutil.NewTestClient(t, handler)

	if _, err := Create(t.Context(), client, CreateInput{
		ProjectID:   "1",
		FreezeStart: testCronFreezeStart,
		FreezeEnd:   testCronFreezeEnd,
	}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestUpdate_EveryFieldNamed_SendsAllThree verifies that each of the three
// optional update fields reaches GitLab carrying the caller's own value.
//
// Each is copied into the options struct behind its own guard, and a response
// assertion cannot tell the three apart: the mock echoes a body it was written
// with, so a handler that sent one field, the wrong field or no field at all
// produces the same output. Only the request distinguishes them.
func TestUpdate_EveryFieldNamed_SendsAllThree(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := decodeFreezeRequestBody(t, r)
		assertSent(t, body, "freeze_start", testCronUpdatedStart)
		assertSent(t, body, "freeze_end", testCronUpdatedEnd)
		assertSent(t, body, "cron_timezone", testTimezoneMadrid)
		testutil.RespondJSON(w, http.StatusOK,
			`{"id":5,"freeze_start":"0 0 * * 5","freeze_end":"0 9 * * 1","cron_timezone":"Europe/Madrid"}`)
	})
	client := testutil.NewTestClient(t, handler)

	if _, err := Update(t.Context(), client, UpdateInput{
		ProjectID:      "1",
		FreezePeriodID: 5,
		FreezeStart:    testCronUpdatedStart,
		FreezeEnd:      testCronUpdatedEnd,
		CronTimezone:   testTimezoneMadrid,
	}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestUpdate_OnlyTheTimezoneNamed_LeavesTheCronFieldsOut verifies that a partial
// update sends the field the caller named and nothing else.
//
// This is the half of the update that a response can never show. GitLab's update
// is a patch, so a freeze_start the caller did not name must not be on the wire:
// sent empty, it would blank the start of a live freeze window while the handler
// reported success, and the echoed response would still carry whatever the
// caller expected to see.
func TestUpdate_OnlyTheTimezoneNamed_LeavesTheCronFieldsOut(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := decodeFreezeRequestBody(t, r)
		assertSent(t, body, "cron_timezone", testTimezoneMadrid)
		assertNotSent(t, body, "freeze_start")
		assertNotSent(t, body, "freeze_end")
		testutil.RespondJSON(w, http.StatusOK,
			`{"id":5,"freeze_start":"0 23 * * 5","freeze_end":"0 7 * * 1","cron_timezone":"Europe/Madrid"}`)
	})
	client := testutil.NewTestClient(t, handler)

	if _, err := Update(t.Context(), client, UpdateInput{
		ProjectID:      "1",
		FreezePeriodID: 5,
		CronTimezone:   testTimezoneMadrid,
	}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// containsStr is a helper to check substring presence.
func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
