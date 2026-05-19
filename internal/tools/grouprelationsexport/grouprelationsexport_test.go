// grouprelationsexport_test.go contains unit tests for the grouprelationsexport MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package grouprelationsexport

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// TestScheduleExport verifies ScheduleExport.
func TestScheduleExport(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasPrefix(r.URL.Path, "/api/v4/groups/") {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	err := ScheduleExport(t.Context(), client, ScheduleExportInput{GroupID: "10"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestScheduleExport_Error verifies ScheduleExport when error.
func TestScheduleExport_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	err := ScheduleExport(t.Context(), client, ScheduleExportInput{GroupID: "10"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestListExportStatus verifies ListExportStatus.
func TestListExportStatus(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"relation":"project","status":1,"batched":false,"batches_count":0,"updated_at":"2026-01-01T00:00:00Z"}]`)
	}))
	out, err := ListExportStatus(t.Context(), client, ListExportStatusInput{GroupID: "10"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(out.Statuses))
	}
	if out.Statuses[0].Relation != "project" {
		t.Errorf("expected relation 'project', got %q", out.Statuses[0].Relation)
	}
}

// TestListExportStatus_Error verifies ListExportStatus when error.
func TestListExportStatus_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	_, err := ListExportStatus(t.Context(), client, ListExportStatusInput{GroupID: "10"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestFormatListExportStatus verifies FormatListExportStatus.
func TestFormatListExportStatus(t *testing.T) {
	out := &ListExportStatusOutput{
		Statuses: []ExportStatusItem{
			{Relation: "project", Status: 1, Batched: false, BatchesCount: 0},
		},
	}
	md := FormatListExportStatus(out)
	if !strings.Contains(md, "project") {
		t.Errorf("expected markdown to contain 'project'")
	}
}

// TestFormatListExportStatus_Empty verifies FormatListExportStatus when empty.
func TestFormatListExportStatus_Empty(t *testing.T) {
	out := &ListExportStatusOutput{Statuses: []ExportStatusItem{}}
	md := FormatListExportStatus(out)
	if !strings.Contains(md, "No export statuses") {
		t.Errorf("expected empty message")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// ScheduleExport — canceled context, with batched option
// ---------------------------------------------------------------------------.

// TestScheduleExport_CancelledContext verifies ScheduleExport when cancelled context.
func TestScheduleExport_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	err := ScheduleExport(ctx, client, ScheduleExportInput{GroupID: "10"})
	if err == nil {
		t.Fatal("expected error for canceled context, got nil")
	}
}

// TestScheduleExport_WithBatched verifies ScheduleExport when with batched.
func TestScheduleExport_WithBatched(t *testing.T) {
	batched := true
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasPrefix(r.URL.Path, "/api/v4/groups/") {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	err := ScheduleExport(t.Context(), client, ScheduleExportInput{GroupID: "10", Batched: &batched})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestScheduleExport_EmptyGroupID verifies ScheduleExport when empty group ID.
func TestScheduleExport_EmptyGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	err := ScheduleExport(t.Context(), client, ScheduleExportInput{GroupID: ""})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// ---------------------------------------------------------------------------
// ListExportStatus — canceled context, with relation filter, empty group_id, pagination
// ---------------------------------------------------------------------------.

// TestListExportStatus_CancelledContext verifies ListExportStatus when cancelled context.
func TestListExportStatus_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListExportStatus(ctx, client, ListExportStatusInput{GroupID: "10"})
	if err == nil {
		t.Fatal("expected error for canceled context, got nil")
	}
}

// TestListExportStatus_EmptyGroupID verifies ListExportStatus when empty group ID.
func TestListExportStatus_EmptyGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	_, err := ListExportStatus(t.Context(), client, ListExportStatusInput{GroupID: ""})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestListExportStatus_WithRelationFilter verifies ListExportStatus when with relation filter.
func TestListExportStatus_WithRelationFilter(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"relation":"milestones","status":0,"batched":true,"batches_count":2,"updated_at":"2026-06-15T10:00:00Z"}]`)
	}))
	out, err := ListExportStatus(t.Context(), client, ListExportStatusInput{
		GroupID:  "10",
		Relation: "milestones",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(out.Statuses))
	}
	if out.Statuses[0].Relation != "milestones" {
		t.Errorf("expected relation 'milestones', got %q", out.Statuses[0].Relation)
	}
	if !out.Statuses[0].Batched {
		t.Error("expected Batched=true")
	}
	if out.Statuses[0].BatchesCount != 2 {
		t.Errorf("expected BatchesCount=2, got %d", out.Statuses[0].BatchesCount)
	}
}

// TestListExportStatus_WithPagination verifies ListExportStatus when with pagination.
func TestListExportStatus_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"relation":"project","status":1,"batched":false,"batches_count":0,"updated_at":"2026-01-01T00:00:00Z"},
			{"relation":"milestones","status":0,"batched":true,"batches_count":3,"updated_at":"2026-01-02T00:00:00Z"}
		]`, testutil.PaginationHeaders{
			Page:       "1",
			PerPage:    "2",
			Total:      "5",
			TotalPages: "3",
			NextPage:   "2",
		})
	}))
	out, err := ListExportStatus(t.Context(), client, ListExportStatusInput{
		GroupID: "10",
		Page:    1,
		PerPage: 2,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(out.Statuses))
	}
	if out.Pagination.TotalPages != 3 {
		t.Errorf("TotalPages = %d, want 3", out.Pagination.TotalPages)
	}
	if out.Pagination.NextPage != 2 {
		t.Errorf("NextPage = %d, want 2", out.Pagination.NextPage)
	}
}

// TestListExportStatus_EmptyResponse verifies ListExportStatus when empty response.
func TestListExportStatus_EmptyResponse(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	out, err := ListExportStatus(t.Context(), client, ListExportStatusInput{GroupID: "10"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Statuses) != 0 {
		t.Fatalf("expected 0 statuses, got %d", len(out.Statuses))
	}
}

// TestListExportStatus_WithErrorField verifies ListExportStatus when with error field.
func TestListExportStatus_WithErrorField(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"relation":"project","status":-1,"error":"export failed","batched":false,"batches_count":0,"updated_at":"2026-06-15T10:00:00Z"}]`)
	}))
	out, err := ListExportStatus(t.Context(), client, ListExportStatusInput{GroupID: "10"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Statuses[0].Error != "export failed" {
		t.Errorf("expected Error='export failed', got %q", out.Statuses[0].Error)
	}
}

// ---------------------------------------------------------------------------
// FormatScheduleExport
// ---------------------------------------------------------------------------.

// TestFormatScheduleExport_Message verifies FormatScheduleExport when message.
func TestFormatScheduleExport_Message(t *testing.T) {
	md := FormatScheduleExport()
	if !strings.Contains(md, "scheduled successfully") {
		t.Errorf("expected success message, got %q", md)
	}
}

// ---------------------------------------------------------------------------
// FormatListExportStatus — multiple items, with error field, markdown escaping
// ---------------------------------------------------------------------------.

// TestFormatListExportStatus_MultipleItems verifies FormatListExportStatus when multiple items.
func TestFormatListExportStatus_MultipleItems(t *testing.T) {
	out := &ListExportStatusOutput{
		Statuses: []ExportStatusItem{
			{Relation: "project", Status: 1, Batched: false, BatchesCount: 0, UpdatedAt: "2026-01-01T00:00:00Z"},
			{Relation: "milestones", Status: 0, Error: "timeout", Batched: true, BatchesCount: 3, UpdatedAt: "2026-01-02T00:00:00Z"},
		},
	}
	md := FormatListExportStatus(out)
	for _, want := range []string{
		"| Relation |",
		"|---|",
		"project",
		"milestones",
		"timeout",
		"true",
		"false",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatListExportStatus_WithPipeInRelation verifies FormatListExportStatus when with pipe in relation.
func TestFormatListExportStatus_WithPipeInRelation(t *testing.T) {
	out := &ListExportStatusOutput{
		Statuses: []ExportStatusItem{
			{Relation: "test|pipe", Status: 1, Batched: false, BatchesCount: 0},
		},
	}
	md := FormatListExportStatus(out)
	// Pipe character should be escaped in markdown table
	if strings.Contains(md, "| test|pipe |") {
		t.Errorf("pipe char should be escaped in markdown table:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// ---------------------------------------------------------------------------.

// ---------------------------------------------------------------------------
// ActionSpecs route execution for both tools
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallAllRoutes validates both group relations export routes.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	client := testutil.NewTestClient(t, groupRelationsExportHandler())
	byTool := groupRelationsSpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"schedule_export", "gitlab_schedule_group_relations_export", map[string]any{"group_id": "10"}},
		{"list_export_status", "gitlab_list_group_relations_export_status", map[string]any{"group_id": "10"}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs route — schedule export returns error from API
// ---------------------------------------------------------------------------.

// TestActionSpecs_ScheduleExportAPIError verifies schedule export route API errors.
func TestActionSpecs_ScheduleExportAPIError(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("POST /api/v4/groups/10/export_relations", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	handler.HandleFunc("GET /api/v4/groups/10/export_relations/status", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})

	client := testutil.NewTestClient(t, handler)
	spec := groupRelationsSpecsByTool(t, ActionSpecs(client))["gitlab_schedule_group_relations_export"]

	_, err := spec.Route.Handler(t.Context(), map[string]any{"group_id": "10"})
	if err == nil {
		t.Fatal("Route.Handler expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs route — list export status returns error from API
// ---------------------------------------------------------------------------.

// TestActionSpecs_ListExportStatusAPIError verifies list status route API errors.
func TestActionSpecs_ListExportStatusAPIError(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("POST /api/v4/groups/10/export_relations", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	handler.HandleFunc("GET /api/v4/groups/10/export_relations/status", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})

	client := testutil.NewTestClient(t, handler)
	spec := groupRelationsSpecsByTool(t, ActionSpecs(client))["gitlab_list_group_relations_export_status"]

	_, err := spec.Route.Handler(t.Context(), map[string]any{"group_id": "10"})
	if err == nil {
		t.Fatal("Route.Handler expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Helper: MCP session factory (default happy-path)
// ---------------------------------------------------------------------------.

// groupRelationsExportHandler returns a default happy-path GitLab API mock.
func groupRelationsExportHandler() *http.ServeMux {
	handler := http.NewServeMux()

	handler.HandleFunc("POST /api/v4/groups/10/export_relations", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})

	handler.HandleFunc("GET /api/v4/groups/10/export_relations/status", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"relation":"project","status":1,"batched":false,"batches_count":0,"updated_at":"2026-01-01T00:00:00Z"}]`)
	})

	return handler
}
