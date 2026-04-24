// group_iterations_test.go contains unit tests for GitLab group iteration
// operations. Tests use httptest to mock the GitLab Group Iterations API.
package groupiterations

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

const fmtUnexpErr = "unexpected error: %v"

// TestList_Success verifies List returns correct group iteration fields
// including id, iid, title, state, and web_url from a well-formed API response.
func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, "/api/v4/groups/10/iterations")
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"id":2,"iid":1,"sequence":1,"group_id":10,"title":"Sprint 2","state":1,"web_url":"https://gitlab.example.com/iterations/2"}
		]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: "10"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Iterations) != 1 {
		t.Fatalf("got %d iterations, want 1", len(out.Iterations))
	}
	it := out.Iterations[0]
	if it.Title != "Sprint 2" {
		t.Errorf("got title %q, want %q", it.Title, "Sprint 2")
	}
	if it.State != 1 {
		t.Errorf("got state %d, want 1", it.State)
	}
	if it.IID != 1 {
		t.Errorf("got IID %d, want 1", it.IID)
	}
	if it.ID != 2 {
		t.Errorf("got ID %d, want 2", it.ID)
	}
	if it.GroupID != 10 {
		t.Errorf("got GroupID %d, want 10", it.GroupID)
	}
	if it.WebURL != "https://gitlab.example.com/iterations/2" {
		t.Errorf("got WebURL %q, want non-empty URL", it.WebURL)
	}
}

// TestList_ValidationError_MissingGroupID verifies List returns error when GroupID is empty.
func TestList_ValidationError_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be called")
	}))
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

// TestList_QueryParams verifies List passes state, search, and include_ancestors
// parameters correctly to the GitLab API query string.
func TestList_QueryParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/groups/5/iterations")
		testutil.AssertQueryParam(t, r, "state", "opened")
		testutil.AssertQueryParam(t, r, "search", "sprint")
		testutil.AssertQueryParam(t, r, "include_ancestors", "true")
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
	}))

	_, err := List(context.Background(), client, ListInput{
		GroupID:          "5",
		State:            "opened",
		Search:           "sprint",
		IncludeAncestors: true,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestList_EmptyResult verifies List returns an empty slice when the API
// returns no iterations, ensuring no nil-slice issues.
func TestList_EmptyResult(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: "10"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Iterations) != 0 {
		t.Errorf("got %d iterations, want 0", len(out.Iterations))
	}
}

// TestList_APIError verifies List wraps and returns errors from the GitLab API
// for non-200 responses (404, 500).
func TestList_APIError(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{
			name:   "returns error on 404 not found",
			status: http.StatusNotFound,
			body:   `{"message":"404 Group Not Found"}`,
		},
		{
			name:   "returns error on 500 internal server error",
			status: http.StatusForbidden,
			body:   `{"message":"Internal Server Error"}`,
		},
		{
			name:   "returns error on 403 forbidden",
			status: http.StatusForbidden,
			body:   `{"message":"403 Forbidden"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, tt.status, tt.body)
			}))

			_, err := List(context.Background(), client, ListInput{GroupID: "999"})
			if err == nil {
				t.Fatal("expected error from API, got nil")
			}
		})
	}
}

// TestList_Pagination verifies List correctly propagates pagination metadata
// from the GitLab response headers.
func TestList_Pagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"id":1,"iid":1,"title":"Sprint 1","state":1,"group_id":10}
		]`, testutil.PaginationHeaders{
			Page: "1", PerPage: "1", Total: "3", TotalPages: "3", NextPage: "2",
		})
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: "10"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Pagination.TotalItems != 3 {
		t.Errorf("pagination total_items = %d, want 3", out.Pagination.TotalItems)
	}
	if out.Pagination.TotalPages != 3 {
		t.Errorf("pagination total_pages = %d, want 3", out.Pagination.TotalPages)
	}
	if out.Pagination.NextPage != 2 {
		t.Errorf("pagination next_page = %d, want 2", out.Pagination.NextPage)
	}
}

// TestList_ContextCancelled verifies List returns an error when the context
// is cancelled before the API call completes.
func TestList_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := List(ctx, client, ListInput{GroupID: "10"})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestList_WithDates verifies List correctly parses start_date, due_date,
// created_at, and updated_at from the API response into string fields.
func TestList_WithDates(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[{
			"id":5,"iid":2,"sequence":2,"group_id":10,"title":"Sprint 3","state":3,
			"start_date":"2026-01-01","due_date":"2026-01-14",
			"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-10T12:00:00Z"
		}]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: "10"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Iterations) != 1 {
		t.Fatalf("got %d iterations, want 1", len(out.Iterations))
	}
	it := out.Iterations[0]
	if it.StartDate == "" {
		t.Error("expected non-empty StartDate")
	}
	if it.DueDate == "" {
		t.Error("expected non-empty DueDate")
	}
	if it.CreatedAt == "" {
		t.Error("expected non-empty CreatedAt")
	}
	if it.UpdatedAt == "" {
		t.Error("expected non-empty UpdatedAt")
	}
}

// TestToOutput_NilInput verifies toOutput returns a zero-value Output for nil input.
func TestToOutput_NilInput(t *testing.T) {
	out := toOutput(nil)
	if out.ID != 0 || out.Title != "" {
		t.Errorf("expected zero Output for nil, got %+v", out)
	}
}

// TestToOutput_AllFields verifies toOutput maps all GroupIteration fields
// including dates to the Output struct.
func TestToOutput_AllFields(t *testing.T) {
	now := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	startDate := gl.ISOTime(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	dueDate := gl.ISOTime(time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC))

	it := &gl.GroupIteration{
		ID:          42,
		IID:         7,
		Sequence:    3,
		GroupID:     10,
		Title:       "Sprint 7",
		Description: "Iteration description",
		State:       2,
		WebURL:      "https://gitlab.example.com/iterations/42",
		StartDate:   &startDate,
		DueDate:     &dueDate,
		CreatedAt:   &now,
		UpdatedAt:   &now,
	}

	out := toOutput(it)
	if out.ID != 42 {
		t.Errorf("ID = %d, want 42", out.ID)
	}
	if out.IID != 7 {
		t.Errorf("IID = %d, want 7", out.IID)
	}
	if out.Sequence != 3 {
		t.Errorf("Sequence = %d, want 3", out.Sequence)
	}
	if out.GroupID != 10 {
		t.Errorf("GroupID = %d, want 10", out.GroupID)
	}
	if out.Title != "Sprint 7" {
		t.Errorf("Title = %q, want %q", out.Title, "Sprint 7")
	}
	if out.Description != "Iteration description" {
		t.Errorf("Description = %q, want %q", out.Description, "Iteration description")
	}
	if out.State != 2 {
		t.Errorf("State = %d, want 2", out.State)
	}
	if out.WebURL != "https://gitlab.example.com/iterations/42" {
		t.Errorf("WebURL = %q, want non-empty", out.WebURL)
	}
	if out.StartDate == "" {
		t.Error("expected non-empty StartDate")
	}
	if out.DueDate == "" {
		t.Error("expected non-empty DueDate")
	}
	if out.CreatedAt == "" {
		t.Error("expected non-empty CreatedAt")
	}
	if out.UpdatedAt == "" {
		t.Error("expected non-empty UpdatedAt")
	}
}

// TestToOutput_NilDates verifies toOutput leaves date fields empty when the
// source GroupIteration has nil date pointers.
func TestToOutput_NilDates(t *testing.T) {
	it := &gl.GroupIteration{
		ID:    1,
		Title: "No dates",
		State: 1,
	}

	out := toOutput(it)
	if out.StartDate != "" {
		t.Errorf("StartDate = %q, want empty", out.StartDate)
	}
	if out.DueDate != "" {
		t.Errorf("DueDate = %q, want empty", out.DueDate)
	}
	if out.CreatedAt != "" {
		t.Errorf("CreatedAt = %q, want empty", out.CreatedAt)
	}
	if out.UpdatedAt != "" {
		t.Errorf("UpdatedAt = %q, want empty", out.UpdatedAt)
	}
}

// TestIterationState verifies iterationState maps state integers to the correct
// human-readable strings for all known states and unknown values.
func TestIterationState(t *testing.T) {
	tests := []struct {
		name  string
		state int64
		want  string
	}{
		{name: "opened", state: 1, want: "opened"},
		{name: "upcoming", state: 2, want: "upcoming"},
		{name: "current", state: 3, want: "current"},
		{name: "closed", state: 4, want: "closed"},
		{name: "unknown zero", state: 0, want: "unknown(0)"},
		{name: "unknown high", state: 99, want: "unknown(99)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := iterationState(tt.state)
			if got != tt.want {
				t.Errorf("iterationState(%d) = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}

// TestFormatListMarkdown_Empty verifies FormatListMarkdown returns the
// "no iterations found" message for an empty list.
func TestFormatListMarkdown_Empty(t *testing.T) {
	got := FormatListMarkdown(ListOutput{})
	if !strings.Contains(got, "No group iterations found") {
		t.Errorf("expected 'No group iterations found' message, got:\n%s", got)
	}
}

// TestFormatListMarkdown_WithIterations verifies FormatListMarkdown produces
// a Markdown table with ID, IID, title, state, dates, and URL columns.
func TestFormatListMarkdown_WithIterations(t *testing.T) {
	out := ListOutput{
		Iterations: []Output{
			{ID: 1, IID: 1, Title: "Sprint 1", State: 1, StartDate: "2026-01-01", DueDate: "2026-01-14", WebURL: "https://gitlab.example.com/it/1"},
			{ID: 2, IID: 2, Title: "Sprint 2", State: 4, StartDate: "2026-01-15", DueDate: "2026-01-28", WebURL: ""},
		},
	}
	got := FormatListMarkdown(out)

	if !strings.Contains(got, "## Group Iterations") {
		t.Error("expected '## Group Iterations' header")
	}
	if !strings.Contains(got, "Sprint 1") {
		t.Error("expected 'Sprint 1' in output")
	}
	if !strings.Contains(got, "Sprint 2") {
		t.Error("expected 'Sprint 2' in output")
	}
	if !strings.Contains(got, "opened") {
		t.Error("expected 'opened' state in output")
	}
	if !strings.Contains(got, "closed") {
		t.Error("expected 'closed' state in output")
	}
	// Verify URL is rendered as link for first iteration
	if !strings.Contains(got, "[opened](https://gitlab.example.com/it/1)") {
		t.Error("expected clickable link for iteration with web_url")
	}
}

// TestFormatOutputMarkdown_Full verifies FormatOutputMarkdown renders all fields
// including description and URL for a fully populated iteration.
func TestFormatOutputMarkdown_Full(t *testing.T) {
	out := Output{
		ID:          42,
		IID:         7,
		Title:       "Sprint 7",
		State:       3,
		GroupID:     10,
		StartDate:   "2026-03-01",
		DueDate:     "2026-03-14",
		WebURL:      "https://gitlab.example.com/iterations/42",
		CreatedAt:   "2026-03-01T00:00:00Z",
		Description: "This is the iteration description.",
	}
	got := FormatOutputMarkdown(out)

	if !strings.Contains(got, "## Iteration #7") {
		t.Error("expected iteration header with IID")
	}
	if !strings.Contains(got, "Sprint 7") {
		t.Error("expected title in output")
	}
	if !strings.Contains(got, "current") {
		t.Error("expected 'current' state")
	}
	if !strings.Contains(got, "https://gitlab.example.com/iterations/42") {
		t.Error("expected web URL in output")
	}
	if !strings.Contains(got, "### Description") {
		t.Error("expected description section")
	}
	if !strings.Contains(got, "iteration description") {
		t.Error("expected description text in output")
	}
}

// TestFormatOutputMarkdown_NoDescription verifies FormatOutputMarkdown omits
// the Description section when the description field is empty.
func TestFormatOutputMarkdown_NoDescription(t *testing.T) {
	out := Output{
		ID:    1,
		IID:   1,
		Title: "Minimal",
		State: 1,
	}
	got := FormatOutputMarkdown(out)

	if strings.Contains(got, "### Description") {
		t.Error("expected no description section for empty description")
	}
	if !strings.Contains(got, "Minimal") {
		t.Error("expected title in output")
	}
}

// TestFormatOutputMarkdown_NoWebURL verifies FormatOutputMarkdown omits the
// URL row when WebURL is empty.
func TestFormatOutputMarkdown_NoWebURL(t *testing.T) {
	out := Output{
		ID:    1,
		IID:   1,
		Title: "No URL",
		State: 2,
	}
	got := FormatOutputMarkdown(out)

	if strings.Contains(got, "| URL |") {
		t.Error("expected no URL row when WebURL is empty")
	}
}

// TestList_MultipleIterations verifies List correctly converts multiple
// iterations from the API response.
func TestList_MultipleIterations(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"id":1,"iid":1,"title":"Sprint 1","state":1,"group_id":10},
			{"id":2,"iid":2,"title":"Sprint 2","state":2,"group_id":10},
			{"id":3,"iid":3,"title":"Sprint 3","state":3,"group_id":10}
		]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "3", TotalPages: "1"})
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: "10"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Iterations) != 3 {
		t.Fatalf("got %d iterations, want 3", len(out.Iterations))
	}
	for i, want := range []string{"Sprint 1", "Sprint 2", "Sprint 3"} {
		if out.Iterations[i].Title != want {
			t.Errorf("iteration[%d].Title = %q, want %q", i, out.Iterations[i].Title, want)
		}
	}
}

// TestRegisterTools_CallThroughMCP verifies that RegisterTools registers
// the tool and it can be called through MCP in-memory transport.
func TestRegisterTools_CallThroughMCP(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":1,"iid":1,"title":"Sprint 1","state":1,"group_id":10}]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"},
		)
	})
	client := testutil.NewTestClient(t, mux)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_list_group_iterations",
		Arguments: map[string]any{"group_id": "10"},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool returned tool error")
	}
}
