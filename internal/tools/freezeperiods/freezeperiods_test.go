// freezeperiods_test.go contains unit tests for the freeze period MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package freezeperiods

import (
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const (
	fmtUnexpErr              = "unexpected error: %v"
	testCronFreezeStart      = "0 23 * * 5"
	testCronUpdatedStart     = "0 0 * * 5"
	errMissingFreezePeriodID = "expected error for missing freeze_period_id"
)

// TestList_Success verifies the behavior of list success.
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

// TestList_MissingProjectID verifies the behavior of list missing project i d.
func TestList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := List(t.Context(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestGet_Success verifies the behavior of get success.
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

// TestCreate_Success verifies the behavior of create success.
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

// TestUpdate_Success verifies the behavior of update success.
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

// TestDelete_Success verifies the behavior of delete success.
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

// TestGet_APIError verifies the behavior of get a p i error.
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

// TestFormatMarkdownString verifies the behavior of format markdown string.
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

// TestFormatListMarkdownString_Empty verifies the behavior of format list markdown string empty.
func TestFormatListMarkdownString_Empty(t *testing.T) {
	md := FormatListMarkdownString(ListOutput{})
	if md != "No freeze periods found.\n" {
		t.Errorf("got %q, want empty message", md)
	}
}

// TestGet_MissingFreezePeriodID verifies the behavior of get missing freeze period i d.
func TestGet_MissingFreezePeriodID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := Get(t.Context(), client, GetInput{ProjectID: "1", FreezePeriodID: 0})
	if err == nil {
		t.Fatal(errMissingFreezePeriodID)
	}
}

// TestUpdate_MissingFreezePeriodID verifies the behavior of update missing freeze period i d.
func TestUpdate_MissingFreezePeriodID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := Update(t.Context(), client, UpdateInput{ProjectID: "1", FreezePeriodID: 0})
	if err == nil {
		t.Fatal(errMissingFreezePeriodID)
	}
}

// TestDelete_MissingFreezePeriodID verifies the behavior of delete missing freeze period i d.
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

// TestGet_APIError verifies that Get returns an error when the API responds with error.
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

// TestRegisterTools_NoPanic verifies that RegisterTools does not panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// TestRegisterMeta_NoPanic verifies that RegisterMeta does not panic.
func TestRegisterMeta_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)
}

// TestRegisterTools_CallThroughMCP verifies that all registered tools can be
// called through MCP in-memory transport, covering the handler closures of
// RegisterTools. Each tool is called with valid inputs and a mock handler
// that returns appropriate JSON responses.
func TestRegisterTools_CallThroughMCP(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"freeze_start":"0 23 * * 5","freeze_end":"0 7 * * 1","cron_timezone":"UTC"}]`)
		case http.MethodPost:
			testutil.RespondJSON(w, http.StatusCreated, `{"id":2,"freeze_start":"0 23 * * 5","freeze_end":"0 7 * * 1","cron_timezone":"UTC"}`)
		case http.MethodPut:
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"freeze_start":"0 0 * * 1","freeze_end":"0 7 * * 1","cron_timezone":"UTC"}`)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	client := testutil.NewTestClient(t, mux)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := t.Context()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_list_freeze_periods", map[string]any{"project_id": "1"}},
		{"gitlab_get_freeze_period", map[string]any{"project_id": "1", "freeze_period_id": 1}},
		{"gitlab_create_freeze_period", map[string]any{"project_id": "1", "freeze_start": "0 23 * * 5", "freeze_end": "0 7 * * 1"}},
		{"gitlab_update_freeze_period", map[string]any{"project_id": "1", "freeze_period_id": 1, "freeze_start": "0 0 * * 1"}},
		{"gitlab_delete_freeze_period", map[string]any{"project_id": "1", "freeze_period_id": 1}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			var result *mcp.CallToolResult
			result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("CallTool(%s) returned nil result", tt.name)
			}
		})
	}
}
