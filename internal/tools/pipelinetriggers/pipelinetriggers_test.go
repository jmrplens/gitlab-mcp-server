// pipelinetriggers_test.go contains unit tests for the pipeline trigger MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package pipelinetriggers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const errExpMissingProjectID = "expected error for missing project_id"

const fmtUnexpErr = "unexpected error: %v"

// ----------------------------------------------
// ListTriggers
// ----------------------------------------------.

// TestListTriggers_Success verifies that ListTriggers handles the success scenario correctly.
func TestListTriggers_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/1/triggers", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":10,"description":"deploy","token":"abc123","owner":{"id":1,"name":"Admin"}}]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"},
		)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListTriggers(context.Background(), client, ListInput{
		ProjectID: "1",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Triggers) != 1 {
		t.Fatalf("triggers = %d, want 1", len(out.Triggers))
	}
	if out.Triggers[0].ID != 10 {
		t.Errorf("id = %d, want 10", out.Triggers[0].ID)
	}
	if out.Triggers[0].Description != "deploy" {
		t.Errorf("description = %q, want %q", out.Triggers[0].Description, "deploy")
	}
	if out.Triggers[0].Token != "abc123" {
		t.Errorf("token = %q, want %q", out.Triggers[0].Token, "abc123")
	}
	if out.Triggers[0].OwnerName != "Admin" {
		t.Errorf("owner_name = %q, want %q", out.Triggers[0].OwnerName, "Admin")
	}
}

// TestListTriggers_MissingProjectID verifies that ListTriggers handles the missing project i d scenario correctly.
func TestListTriggers_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ListTriggers(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal(errExpMissingProjectID)
	}
}

// ----------------------------------------------
// GetTrigger
// ----------------------------------------------.

// TestGetTrigger_Success verifies that GetTrigger handles the success scenario correctly.
func TestGetTrigger_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/1/triggers/10", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":10,"description":"deploy","token":"abc123"}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := GetTrigger(context.Background(), client, GetInput{ProjectID: "1", TriggerID: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 10 {
		t.Errorf("id = %d, want 10", out.ID)
	}
	if out.Description != "deploy" {
		t.Errorf("description = %q, want %q", out.Description, "deploy")
	}
}

// TestGetTrigger_MissingProjectID verifies that GetTrigger handles the missing project i d scenario correctly.
func TestGetTrigger_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := GetTrigger(context.Background(), client, GetInput{TriggerID: 10})
	if err == nil {
		t.Fatal(errExpMissingProjectID)
	}
}

// TestGetTrigger_MissingTriggerID verifies that GetTrigger handles the missing trigger i d scenario correctly.
func TestGetTrigger_MissingTriggerID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := GetTrigger(context.Background(), client, GetInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error for missing trigger_id")
	}
}

// ----------------------------------------------
// CreateTrigger
// ----------------------------------------------.

// TestCreateTrigger_Success verifies that CreateTrigger handles the success scenario correctly.
func TestCreateTrigger_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/projects/1/triggers", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":11,"description":"test trigger","token":"xyz789"}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := CreateTrigger(context.Background(), client, CreateInput{
		ProjectID:   "1",
		Description: "test trigger",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 11 {
		t.Errorf("id = %d, want 11", out.ID)
	}
	if out.Description != "test trigger" {
		t.Errorf("description = %q, want %q", out.Description, "test trigger")
	}
}

// TestCreateTrigger_MissingDescription verifies that CreateTrigger handles the missing description scenario correctly.
func TestCreateTrigger_MissingDescription(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := CreateTrigger(context.Background(), client, CreateInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error for missing description")
	}
}

// ----------------------------------------------
// UpdateTrigger
// ----------------------------------------------.

// TestUpdateTrigger_Success verifies that UpdateTrigger handles the success scenario correctly.
func TestUpdateTrigger_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/v4/projects/1/triggers/10", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":10,"description":"updated","token":"abc123"}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := UpdateTrigger(context.Background(), client, UpdateInput{
		ProjectID:   "1",
		TriggerID:   10,
		Description: "updated",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Description != "updated" {
		t.Errorf("description = %q, want %q", out.Description, "updated")
	}
}

// TestUpdateTrigger_MissingTriggerID verifies that UpdateTrigger handles the missing trigger i d scenario correctly.
func TestUpdateTrigger_MissingTriggerID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := UpdateTrigger(context.Background(), client, UpdateInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error for missing trigger_id")
	}
}

// ----------------------------------------------
// DeleteTrigger
// ----------------------------------------------.

// TestDeleteTrigger_Success verifies that DeleteTrigger handles the success scenario correctly.
func TestDeleteTrigger_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v4/projects/1/triggers/10", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)

	err := DeleteTrigger(context.Background(), client, DeleteInput{ProjectID: "1", TriggerID: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeleteTrigger_MissingProjectID verifies that DeleteTrigger handles the missing project i d scenario correctly.
func TestDeleteTrigger_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := DeleteTrigger(context.Background(), client, DeleteInput{TriggerID: 10})
	if err == nil {
		t.Fatal(errExpMissingProjectID)
	}
}

// ----------------------------------------------
// RunTrigger
// ----------------------------------------------.

// TestRunTrigger_Success verifies that RunTrigger handles the success scenario correctly.
func TestRunTrigger_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/projects/1/trigger/pipeline", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":99,"sha":"abc","ref":"main","status":"created","web_url":"https://gl/p/1/-/pipelines/99"}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := RunTrigger(context.Background(), client, RunInput{
		ProjectID: "1",
		Ref:       "main",
		Token:     "tok123",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.PipelineID != 99 {
		t.Errorf("pipeline_id = %d, want 99", out.PipelineID)
	}
	if out.Status != "created" {
		t.Errorf("status = %q, want %q", out.Status, "created")
	}
}

// TestRunTrigger_WithVariables verifies that RunTrigger handles the with variables scenario correctly.
func TestRunTrigger_WithVariables(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/projects/1/trigger/pipeline", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":100,"sha":"def","ref":"main","status":"created","web_url":"https://gl/p/1/-/pipelines/100"}`)
	})
	client := testutil.NewTestClient(t, mux)

	vars, err := json.Marshal(map[string]string{"ENV": "prod"})
	if err != nil {
		t.Fatalf("marshal variables: %v", err)
	}
	out, err := RunTrigger(context.Background(), client, RunInput{
		ProjectID: "1",
		Ref:       "main",
		Token:     "tok123",
		Variables: string(vars),
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.PipelineID != 100 {
		t.Errorf("pipeline_id = %d, want 100", out.PipelineID)
	}
}

// TestRunTrigger_InvalidVariablesJSON verifies that RunTrigger handles the invalid variables j s o n scenario correctly.
func TestRunTrigger_InvalidVariablesJSON(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := RunTrigger(context.Background(), client, RunInput{
		ProjectID: "1",
		Ref:       "main",
		Token:     "tok",
		Variables: "not-json",
	})
	if err == nil {
		t.Fatal("expected error for invalid variables JSON")
	}
}

// TestRunTrigger_MissingRef verifies that RunTrigger handles the missing ref scenario correctly.
func TestRunTrigger_MissingRef(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := RunTrigger(context.Background(), client, RunInput{ProjectID: "1", Token: "tok"})
	if err == nil {
		t.Fatal("expected error for missing ref")
	}
}

// TestRunTrigger_MissingToken verifies that RunTrigger handles the missing token scenario correctly.
func TestRunTrigger_MissingToken(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := RunTrigger(context.Background(), client, RunInput{ProjectID: "1", Ref: "main"})
	if err == nil {
		t.Fatal("expected error for missing token")
	}
}

// ----------------------------------------------
// Markdown formatters
// ----------------------------------------------.

// TestFormatTriggerMarkdown verifies the behavior of format trigger markdown.
func TestFormatTriggerMarkdown(t *testing.T) {
	md := FormatTriggerMarkdown(Output{ID: 10, Description: "deploy", Token: "abc123", OwnerName: "Admin", CreatedAt: "2026-01-01T00:00:00Z"})
	if md == "" {
		t.Error("expected non-empty markdown")
	}
}

// TestFormatListTriggersMarkdown_Empty verifies that FormatListTriggersMarkdown handles the empty scenario correctly.
func TestFormatListTriggersMarkdown_Empty(t *testing.T) {
	md := FormatListTriggersMarkdown(ListOutput{})
	if !contains(md, "No pipeline triggers found") {
		t.Error("expected empty-state message")
	}
}

// TestFormatListTriggersMarkdown_WithData verifies that FormatListTriggersMarkdown handles the with data scenario correctly.
func TestFormatListTriggersMarkdown_WithData(t *testing.T) {
	md := FormatListTriggersMarkdown(ListOutput{
		Triggers: []Output{{ID: 1, Description: "test", Token: "tok"}},
		Pagination: toolutil.PaginationOutput{
			Page: 1, PerPage: 20, TotalItems: 1, TotalPages: 1,
		},
	})
	if md == "" {
		t.Error("expected non-empty markdown")
	}
}

// TestFormatRunOutputMarkdown verifies the behavior of format run output markdown.
func TestFormatRunOutputMarkdown(t *testing.T) {
	md := FormatRunOutputMarkdown(RunOutput{PipelineID: 99, SHA: "abc", Ref: "main", Status: "created", WebURL: "https://gl/p/1"})
	if md == "" {
		t.Error("expected non-empty markdown")
	}
}

// contains is an internal helper for the pipelinetriggers package.
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsHelper(s, sub))
}

// containsHelper is an internal helper for the pipelinetriggers package.
func containsHelper(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ---------- Tests consolidated from coverage_test.go ----------.

const errExpCancelledCtx = "expected error for canceled context"

const errExpectedAPI = "expected API error, got nil"

// ---------------------------------------------------------------------------
// ListTriggers — API error, canceled context
// ---------------------------------------------------------------------------.

// TestListTriggers_APIError verifies the behavior of list triggers a p i error.
func TestListTriggers_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := ListTriggers(context.Background(), client, ListInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestListTriggers_CancelledContext verifies the behavior of list triggers cancelled context.
func TestListTriggers_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListTriggers(ctx, client, ListInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestListTriggers_WithPagination verifies the behavior of list triggers with pagination.
func TestListTriggers_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/1/triggers" {
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"id":1,"description":"t1","token":"a"},{"id":2,"description":"t2","token":"b"}]`,
				testutil.PaginationHeaders{Page: "2", PerPage: "2", Total: "5", TotalPages: "3", NextPage: "3", PrevPage: "1"},
			)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := ListTriggers(context.Background(), client, ListInput{
		ProjectID:       "1",
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 2},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Triggers) != 2 {
		t.Fatalf("len(Triggers) = %d, want 2", len(out.Triggers))
	}
	if out.Pagination.TotalPages != 3 {
		t.Errorf("TotalPages = %d, want 3", out.Pagination.TotalPages)
	}
}

// ---------------------------------------------------------------------------
// GetTrigger — API error, canceled context
// ---------------------------------------------------------------------------.

// TestGetTrigger_APIError verifies the behavior of get trigger a p i error.
func TestGetTrigger_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := GetTrigger(context.Background(), client, GetInput{ProjectID: "1", TriggerID: 10})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGetTrigger_CancelledContext verifies the behavior of get trigger cancelled context.
func TestGetTrigger_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetTrigger(ctx, client, GetInput{ProjectID: "1", TriggerID: 10})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// CreateTrigger — API error, missing project_id, canceled context
// ---------------------------------------------------------------------------.

// TestCreateTrigger_APIError verifies the behavior of create trigger a p i error.
func TestCreateTrigger_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := CreateTrigger(context.Background(), client, CreateInput{
		ProjectID: "1", Description: "test",
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCreateTrigger_MissingProjectID verifies the behavior of create trigger missing project i d.
func TestCreateTrigger_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := CreateTrigger(context.Background(), client, CreateInput{Description: "test"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestCreateTrigger_CancelledContext verifies the behavior of create trigger cancelled context.
func TestCreateTrigger_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := CreateTrigger(ctx, client, CreateInput{ProjectID: "1", Description: "test"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// UpdateTrigger — API error, missing project_id, canceled context
// ---------------------------------------------------------------------------.

// TestUpdateTrigger_APIError verifies the behavior of update trigger a p i error.
func TestUpdateTrigger_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := UpdateTrigger(context.Background(), client, UpdateInput{
		ProjectID: "1", TriggerID: 10, Description: "updated",
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestUpdateTrigger_MissingProjectID verifies the behavior of update trigger missing project i d.
func TestUpdateTrigger_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := UpdateTrigger(context.Background(), client, UpdateInput{TriggerID: 10})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestUpdateTrigger_CancelledContext verifies the behavior of update trigger cancelled context.
func TestUpdateTrigger_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := UpdateTrigger(ctx, client, UpdateInput{ProjectID: "1", TriggerID: 10})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestUpdateTrigger_WithoutDescription verifies the behavior of update trigger without description.
func TestUpdateTrigger_WithoutDescription(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/projects/1/triggers/10" {
			testutil.RespondJSON(w, http.StatusOK, `{"id":10,"description":"original","token":"abc123"}`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := UpdateTrigger(context.Background(), client, UpdateInput{
		ProjectID: "1", TriggerID: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Description != "original" {
		t.Errorf("description = %q, want %q", out.Description, "original")
	}
}

// ---------------------------------------------------------------------------
// DeleteTrigger — API error, missing trigger_id, canceled context
// ---------------------------------------------------------------------------.

// TestDeleteTrigger_APIError verifies the behavior of delete trigger a p i error.
func TestDeleteTrigger_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := DeleteTrigger(context.Background(), client, DeleteInput{ProjectID: "1", TriggerID: 10})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDeleteTrigger_MissingTriggerID verifies the behavior of delete trigger missing trigger i d.
func TestDeleteTrigger_MissingTriggerID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	err := DeleteTrigger(context.Background(), client, DeleteInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error for missing trigger_id")
	}
}

// TestDeleteTrigger_CancelledContext verifies the behavior of delete trigger cancelled context.
func TestDeleteTrigger_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	err := DeleteTrigger(ctx, client, DeleteInput{ProjectID: "1", TriggerID: 10})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// RunTrigger — API error, missing project_id, canceled context
// ---------------------------------------------------------------------------.

// TestRunTrigger_APIError verifies the behavior of run trigger a p i error.
func TestRunTrigger_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := RunTrigger(context.Background(), client, RunInput{
		ProjectID: "1", Ref: "main", Token: "tok123",
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestRunTrigger_MissingProjectID verifies the behavior of run trigger missing project i d.
func TestRunTrigger_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := RunTrigger(context.Background(), client, RunInput{Ref: "main", Token: "tok"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestRunTrigger_CancelledContext verifies the behavior of run trigger cancelled context.
func TestRunTrigger_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := RunTrigger(ctx, client, RunInput{
		ProjectID: "1", Ref: "main", Token: "tok",
	})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// FormatTriggerMarkdown — all optional fields, minimal fields
// ---------------------------------------------------------------------------.

// TestFormatTriggerMarkdown_AllFields verifies the behavior of format trigger markdown all fields.
func TestFormatTriggerMarkdown_AllFields(t *testing.T) {
	md := FormatTriggerMarkdown(Output{
		ID:          10,
		Description: "deploy trigger",
		Token:       "abc123",
		OwnerName:   "Admin",
		OwnerID:     1,
		CreatedAt:   "2026-01-01T00:00:00Z",
		UpdatedAt:   "2026-06-01T00:00:00Z",
		LastUsed:    "2026-12-01T00:00:00Z",
	})

	for _, want := range []string{
		"## Pipeline Trigger",
		"| ID | 10 |",
		"deploy trigger",
		"abc123",
		"Admin",
		"1 Jan 2026 00:00 UTC",
		"2026-12-01T00:00:00Z",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatTriggerMarkdown_MinimalFields verifies the behavior of format trigger markdown minimal fields.
func TestFormatTriggerMarkdown_MinimalFields(t *testing.T) {
	md := FormatTriggerMarkdown(Output{
		ID:          5,
		Description: "minimal",
		Token:       "tok",
	})
	if !strings.Contains(md, "## Pipeline Trigger") {
		t.Errorf("missing header:\n%s", md)
	}
	for _, absent := range []string{
		"| Owner |",
		"| Created |",
		"| Last Used |",
	} {
		if strings.Contains(md, absent) {
			t.Errorf("should not contain %q for minimal output:\n%s", absent, md)
		}
	}
}

// ---------------------------------------------------------------------------
// FormatListTriggersMarkdown — detailed checks
// ---------------------------------------------------------------------------.

// TestFormatListTriggersMarkdown_DetailedContent verifies the behavior of format list triggers markdown detailed content.
func TestFormatListTriggersMarkdown_DetailedContent(t *testing.T) {
	out := ListOutput{
		Triggers: []Output{
			{ID: 1, Description: "Trigger A", Token: "tokA", OwnerName: "admin", LastUsed: "2026-01-01T00:00:00Z"},
			{ID: 2, Description: "Trigger B", Token: "tokB", OwnerName: "user1", LastUsed: ""},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatListTriggersMarkdown(out)

	for _, want := range []string{
		"## Pipeline Triggers",
		"| ID | Description | Token | Owner | Last Used |",
		"| 1 |",
		"| 2 |",
		"Trigger A",
		"Trigger B",
		"tokA",
		"tokB",
		"admin",
		"user1",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// ---------------------------------------------------------------------------
// FormatRunOutputMarkdown — without web URL, empty values
// ---------------------------------------------------------------------------.

// TestFormatRunOutputMarkdown_WithoutWebURL verifies the behavior of format run output markdown without web u r l.
func TestFormatRunOutputMarkdown_WithoutWebURL(t *testing.T) {
	md := FormatRunOutputMarkdown(RunOutput{
		PipelineID: 50,
		SHA:        "deadbeef",
		Ref:        "develop",
		Status:     "pending",
	})
	for _, want := range []string{
		"## Pipeline Triggered",
		"| Pipeline ID | 50 |",
		"| SHA | deadbeef |",
		"| Ref | develop |",
		"| Status | pending |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "| URL |") {
		t.Errorf("should not contain URL row when WebURL is empty:\n%s", md)
	}
}

// TestFormatRunOutputMarkdown_AllFields verifies the behavior of format run output markdown all fields.
func TestFormatRunOutputMarkdown_AllFields(t *testing.T) {
	md := FormatRunOutputMarkdown(RunOutput{
		PipelineID: 99,
		SHA:        "abc",
		Ref:        "main",
		Status:     "created",
		WebURL:     "https://gl/p/1/-/pipelines/99",
		CreatedAt:  "2026-06-01T00:00:00Z",
	})
	for _, want := range []string{
		"## Pipeline Triggered",
		"| Pipeline ID | 99 |",
		"| URL | [Pipeline #99](https://gl/p/1/-/pipelines/99) |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestGet_WithAllTimestamps verifies convertTrigger covers UpdatedAt and LastUsed nil guards.
func TestGet_WithAllTimestamps(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{
			"id":10,"description":"deploy","token":"abc",
			"owner":{"id":1,"name":"Admin"},
			"created_at":"2026-01-15T10:00:00Z",
			"updated_at":"2026-02-01T12:00:00Z",
			"last_used":"2026-03-01T08:30:00Z"
		}`)
	}))
	out, err := GetTrigger(context.Background(), client, GetInput{ProjectID: "42", TriggerID: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.UpdatedAt == "" {
		t.Error("expected UpdatedAt to be set")
	}
	if out.LastUsed == "" {
		t.Error("expected LastUsed to be set")
	}
}

// ---------------------------------------------------------------------------
// RegisterTools — no panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// RegisterMeta — no panic
// ---------------------------------------------------------------------------.

// TestRegisterMeta_NoPanic verifies the behavior of register meta no panic.
func TestRegisterMeta_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)
}

// ---------------------------------------------------------------------------
// RegisterToolsCallAllThroughMCP — full MCP roundtrip for all 6 tools
// ---------------------------------------------------------------------------.

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newTriggersMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_pipeline_trigger_list", map[string]any{"project_id": "1"}},
		{"get", "gitlab_pipeline_trigger_get", map[string]any{"project_id": "1", "trigger_id": 10}},
		{"create", "gitlab_pipeline_trigger_create", map[string]any{"project_id": "1", "description": "test trigger"}},
		{"update", "gitlab_pipeline_trigger_update", map[string]any{"project_id": "1", "trigger_id": 10, "description": "updated"}},
		{"delete", "gitlab_pipeline_trigger_delete", map[string]any{"project_id": "1", "trigger_id": 10}},
		{"run", "gitlab_pipeline_trigger_run", map[string]any{"project_id": "1", "ref": "main", "token": "tok123"}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.tool,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.tool, err)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(%s) returned error: %s", tt.tool, tc.Text)
					}
				}
				t.Fatalf("CallTool(%s) returned IsError=true", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helper: MCP session factory
// ---------------------------------------------------------------------------.

// newTriggersMCPSession is an internal helper for the pipelinetriggers package.
func newTriggersMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	triggerJSON := `{"id":10,"description":"deploy","token":"abc123","owner":{"id":1,"name":"Admin"},"created_at":"2026-01-01T00:00:00Z"}`
	pipelineJSON := `{"id":99,"sha":"abc","ref":"main","status":"created","web_url":"https://gl/p/1/-/pipelines/99","created_at":"2026-01-01T00:00:00Z"}`

	handler := http.NewServeMux()

	// List pipeline triggers
	handler.HandleFunc("GET /api/v4/projects/1/triggers", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+triggerJSON+`]`)
	})

	// Get pipeline trigger
	handler.HandleFunc("GET /api/v4/projects/1/triggers/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, triggerJSON)
	})

	// Create pipeline trigger
	handler.HandleFunc("POST /api/v4/projects/1/triggers", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, triggerJSON)
	})

	// Update pipeline trigger
	handler.HandleFunc("PUT /api/v4/projects/1/triggers/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, triggerJSON)
	})

	// Delete pipeline trigger
	handler.HandleFunc("DELETE /api/v4/projects/1/triggers/10", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Run (trigger) pipeline
	handler.HandleFunc("POST /api/v4/projects/1/trigger/pipeline", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, pipelineJSON)
	})

	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}
