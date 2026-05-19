// freezeperiods_test.go contains unit tests for the freeze period MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package freezeperiods

import (
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

const (
	// fmtUnexpErr identifies the fmt unexp err constant used by this package.
	fmtUnexpErr = "unexpected error: %v"
	// testCronFreezeStart identifies the test cron freeze start constant used by this package.
	testCronFreezeStart = "0 23 * * 5"
	// testCronUpdatedStart identifies the test cron updated start constant used by this package.
	testCronUpdatedStart = "0 0 * * 5"
	// errMissingFreezePeriodID identifies the err missing freeze period ID constant used by this package.
	errMissingFreezePeriodID = "expected error for missing freeze_period_id"
)

// TestList_Success verifies List when success.
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

// TestList_MissingProjectID verifies List when missing project ID.
func TestList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := List(t.Context(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestGet_Success verifies Get when success.
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

// TestCreate_Success verifies Create when success.
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

// TestUpdate_Success verifies Update when success.
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

// TestDelete_Success verifies Delete when success.
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

// TestGet_APIError verifies Get when API error.
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

// TestFormatMarkdownString verifies FormatMarkdownString.
func TestFormatMarkdownString(t *testing.T) {
	out := Output{
		ID:           1,
		FreezeStart:  testCronFreezeStart,
		FreezeEnd:    "0 7 * * 1",
		CronTimezone: "UTC",
		CreatedAt:    "2026-01-01T00:00:00Z",
	}
	md := FormatMarkdownString(out)
	if md == "" {
		t.Fatal("expected non-empty markdown")
	}
}

// TestFormatListMarkdownString_Empty verifies FormatListMarkdownString when empty.
func TestFormatListMarkdownString_Empty(t *testing.T) {
	md := FormatListMarkdownString(ListOutput{})
	if md != "No freeze periods found.\n" {
		t.Errorf("got %q, want empty message", md)
	}
}

// TestGet_MissingFreezePeriodID verifies Get when missing freeze period ID.
func TestGet_MissingFreezePeriodID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := Get(t.Context(), client, GetInput{ProjectID: "1", FreezePeriodID: 0})
	if err == nil {
		t.Fatal(errMissingFreezePeriodID)
	}
}

// TestUpdate_MissingFreezePeriodID verifies Update when missing freeze period ID.
func TestUpdate_MissingFreezePeriodID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := Update(t.Context(), client, UpdateInput{ProjectID: "1", FreezePeriodID: 0})
	if err == nil {
		t.Fatal(errMissingFreezePeriodID)
	}
}

// TestDelete_MissingFreezePeriodID verifies Delete when missing freeze period ID.
func TestDelete_MissingFreezePeriodID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	err := Delete(t.Context(), client, DeleteInput{ProjectID: "1", FreezePeriodID: 0})
	if err == nil {
		t.Fatal(errMissingFreezePeriodID)
	}
}

// TestList_APIError verifies that List returns an error when the API fails.
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

// TestCreate_APIError verifies that Create returns an error when the API fails.
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

// TestCreate_Forbidden verifies create permission hints.
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

// TestCreate_WithTimezone verifies Create sends the optional CronTimezone.
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
		CronTimezone: "America/New_York",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.CronTimezone != "America/New_York" {
		t.Errorf("CronTimezone = %q, want %q", out.CronTimezone, "America/New_York")
	}
}

// TestCreate_MissingProjectID verifies that Create returns an error for empty project_id.
func TestCreate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := Create(t.Context(), client, CreateInput{FreezeStart: "0 23 * * 5", FreezeEnd: "0 7 * * 1"})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestUpdate_APIError verifies that Update returns an error when the API fails.
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

// TestUpdate_Forbidden verifies update permission hints.
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

// TestUpdate_AllFields verifies Update sends all optional fields when specified.
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

// TestUpdate_MissingProjectID verifies that Update returns an error for empty project_id.
func TestUpdate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := Update(t.Context(), client, UpdateInput{FreezePeriodID: 5, FreezeStart: "0 0 * * 5"})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestDelete_APIError verifies that Delete returns an error when the API responds with error.
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

// TestDelete_MissingProjectID verifies that Delete returns error for empty project_id.
func TestDelete_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	err := Delete(t.Context(), client, DeleteInput{FreezePeriodID: 5})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestGetAPIError_NotFound verifies that Get returns an error when the API responds with error.
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

// TestFormatListMarkdownString_WithItems verifies Markdown output with items.
func TestFormatListMarkdownString_WithItems(t *testing.T) {
	out := ListOutput{
		FreezePeriods: []Output{
			{ID: 1, FreezeStart: "0 23 * * 5", FreezeEnd: "0 7 * * 1", CronTimezone: "UTC"},
			{ID: 2, FreezeStart: "0 0 * * 6", FreezeEnd: "0 0 * * 1", CronTimezone: "Europe/London"},
		},
	}
	md := FormatListMarkdownString(out)
	if md == "" {
		t.Fatal("expected non-empty Markdown")
	}
	if !containsStr(md, "Freeze Periods (2)") {
		t.Error("expected header with count")
	}
	if !containsStr(md, "ID 1") {
		t.Error("expected ID 1 in output")
	}
	if !containsStr(md, "ID 2") {
		t.Error("expected ID 2 in output")
	}
	if !containsStr(md, "Europe/London") {
		t.Error("expected timezone in output")
	}
}

// TestFormatListMarkdown_Wrapper verifies the MCP CallToolResult wrapper.
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

// TestFormatMarkdown_Wrapper verifies the MCP CallToolResult wrapper for a single item.
func TestFormatMarkdown_Wrapper(t *testing.T) {
	out := Output{ID: 5, FreezeStart: "0 23 * * 5", FreezeEnd: "0 7 * * 1", CronTimezone: "UTC"}
	result := FormatMarkdown(out)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// TestFormatMarkdownString_AllFields verifies Markdown includes all optional fields.
func TestFormatMarkdownString_AllFields(t *testing.T) {
	out := Output{
		ID:           5,
		FreezeStart:  "0 23 * * 5",
		FreezeEnd:    "0 7 * * 1",
		CronTimezone: "America/New_York",
		CreatedAt:    "2026-01-01T00:00:00Z",
	}
	md := FormatMarkdownString(out)
	if !containsStr(md, "America/New_York") {
		t.Error("expected timezone in output")
	}
	if !containsStr(md, "1 Jan 2026 00:00 UTC") {
		t.Error("expected created_at in output")
	}
}

// TestGet_MissingProjectID verifies that Get returns an error for empty project_id.
func TestGet_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := Get(t.Context(), client, GetInput{FreezePeriodID: 5})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestGet_SuccessWithTimestamps verifies Get returns timestamps mapped by toOutput.
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

// containsStr is a helper to check substring presence.
func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
