// register_test.go contains integration tests for the bulk import tool
// closures in register.go. Tests exercise success and error paths via an
// in-memory MCP session with a mock GitLab API.

package bulkimports

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// newSession boots an in-memory MCP server with bulkimports tools wired to a
// mock GitLab API and returns an MCP client session ready for tool calls.
func newSession(t *testing.T, mux *http.ServeMux) *mcp.ClientSession {
	t.Helper()
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
	return session
}

// TestRegisterTools_StartMigrationError covers the if-err branch after
// StartMigration() in the register.go closure.
func TestRegisterTools_StartMigrationError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	session := newSession(t, mux)

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "gitlab_start_bulk_import",
		Arguments: map[string]any{
			"url":          "https://gitlab.example.com",
			"access_token": "glpat-test",
			"entities":     []any{map[string]any{"source_type": "group_entity", "source_full_path": "my-group", "destination_slug": "my-group", "destination_namespace": "root"}},
		},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Error("expected error result from gitlab_start_bulk_import")
	}
}

// TestRegisterTools_SuccessPaths exercises the happy path of every tool
// closure registered in register.go. This validates marshaling, markdown
// formatting, and structured-content emission for each tool.
func TestRegisterTools_SuccessPaths(t *testing.T) {
	mux := http.NewServeMux()
	migrationJSON := `{"id":1,"status":"started","source_type":"gitlab","source_url":"https://src","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","has_failures":false}`
	entityJSON := `{"id":7,"bulk_import_id":1,"status":"started","entity_type":"group_entity","source_full_path":"src","destination_full_path":"dst","destination_name":"dst","destination_slug":"dst","destination_namespace":"ns","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","migrate_projects":true,"migrate_memberships":true,"has_failures":false,"stats":{"labels":{"source":1,"fetched":1,"imported":1},"milestones":{"source":2,"fetched":2,"imported":2}}}`
	failureJSON := `[{"relation":"issues","exception_message":"boom","exception_class":"StandardError","correlation_id_value":"cid","source_url":"https://src/i/1","source_title":"oops","step":"extract","created_at":"2026-01-01T00:00:00Z","pipeline_class":"Pipeline","pipeline_step":"step1"}]`

	mux.HandleFunc("/api/v4/bulk_imports", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			testutil.RespondJSON(w, http.StatusOK, migrationJSON)
		default:
			testutil.RespondJSON(w, http.StatusOK, "["+migrationJSON+"]")
		}
	})
	mux.HandleFunc("/api/v4/bulk_imports/entities", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "["+entityJSON+"]")
	})
	mux.HandleFunc("/api/v4/bulk_imports/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, migrationJSON)
	})
	mux.HandleFunc("/api/v4/bulk_imports/1/cancel", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, migrationJSON)
	})
	mux.HandleFunc("/api/v4/bulk_imports/1/entities", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "["+entityJSON+"]")
	})
	mux.HandleFunc("/api/v4/bulk_imports/1/entities/7", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, entityJSON)
	})
	mux.HandleFunc("/api/v4/bulk_imports/1/entities/7/failures", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, failureJSON)
	})

	session := newSession(t, mux)
	ctx := context.Background()

	cases := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_start_bulk_import", map[string]any{
			"url":          "https://gitlab.example.com",
			"access_token": "glpat-test",
			"entities":     []any{map[string]any{"source_type": "group_entity", "source_full_path": "g", "destination_slug": "g", "destination_namespace": "root"}},
		}},
		{"gitlab_list_bulk_imports", map[string]any{}},
		{"gitlab_get_bulk_import", map[string]any{"id": 1}},
		{"gitlab_cancel_bulk_import", map[string]any{"id": 1}},
		{"gitlab_list_bulk_import_entities", map[string]any{"bulk_import_id": 1}},
		{"gitlab_list_bulk_import_entities", map[string]any{}},
		{"gitlab_get_bulk_import_entity", map[string]any{"bulk_import_id": 1, "entity_id": 7}},
		{"gitlab_list_bulk_import_entity_failures", map[string]any{"bulk_import_id": 1, "entity_id": 7}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tc.name, err)
			}
			if result == nil {
				t.Fatalf("CallTool(%s) returned nil result", tc.name)
			}
			if result.IsError {
				t.Fatalf("CallTool(%s) unexpected error result: %+v", tc.name, result.Content)
			}
		})
	}
}

// TestFormatGetMarkdown_FailuresAndStatusHints exercises both conditional
// hint branches: HasFailures=true and Status=started/created.
func TestFormatGetMarkdown_FailuresAndStatusHints(t *testing.T) {
	got := FormatGetMarkdown(MigrationSummary{ID: 1, Status: "started", HasFailures: true})
	if !strings.Contains(got, "Failures detected") {
		t.Errorf("expected failures hint; got %q", got)
	}
	if !strings.Contains(got, "gitlab_cancel_bulk_import") {
		t.Errorf("expected cancel hint for in-progress migration; got %q", got)
	}

	got = FormatGetMarkdown(MigrationSummary{ID: 2, Status: "created"})
	if !strings.Contains(got, "gitlab_cancel_bulk_import") {
		t.Errorf("expected cancel hint for created status; got %q", got)
	}
}

// TestFormatGetEntityMarkdown_HasFailures covers the HasFailures hint branch
// inside FormatGetEntityMarkdown.
func TestFormatGetEntityMarkdown_HasFailures(t *testing.T) {
	got := FormatGetEntityMarkdown(EntitySummary{ID: 1, BulkImportID: 2, HasFailures: true})
	if !strings.Contains(got, "Failures detected") {
		t.Errorf("expected failures hint; got %q", got)
	}
}

// TestListEntities_StatusFilter covers the Status pointer branch in
// ListEntities (input.Status != "").
func TestListEntities_StatusFilter(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/bulk_imports/entities", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("status") != "started" {
			t.Errorf("expected status=started filter, got %q", r.URL.RawQuery)
		}
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})
	client := testutil.NewTestClient(t, mux)
	if _, err := ListEntities(t.Context(), client, ListEntitiesInput{Status: "started"}); err != nil {
		t.Fatalf("ListEntities: %v", err)
	}
}

// TestListEntityFailures_Success covers the success path of ListEntityFailures
// including the per-failure conversion loop with a non-nil entry.
func TestListEntityFailures_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/bulk_imports/3/entities/4/failures", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[
			{"relation":"issues","exception_message":"boom","exception_class":"E","step":"extract","created_at":"2026-01-01T00:00:00Z","pipeline_class":"P","pipeline_step":"s1"}
		]`)
	})
	client := testutil.NewTestClient(t, mux)
	out, err := ListEntityFailures(t.Context(), client, ListEntityFailuresInput{BulkImportID: 3, EntityID: 4})
	if err != nil {
		t.Fatalf("ListEntityFailures: %v", err)
	}
	if len(out.Failures) != 1 || out.Failures[0].Relation != "issues" || out.Failures[0].CreatedAt != "2026-01-01T00:00:00Z" {
		t.Errorf("unexpected failures payload: %+v", out.Failures)
	}
}

// TestListEntityFailures_Validation covers the BulkImportID and EntityID
// non-positive validation branches.
func TestListEntityFailures_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	if _, err := ListEntityFailures(t.Context(), client, ListEntityFailuresInput{}); err == nil {
		t.Fatal("expected error for missing bulk_import_id")
	}
	if _, err := ListEntityFailures(t.Context(), client, ListEntityFailuresInput{BulkImportID: 1}); err == nil {
		t.Fatal("expected error for missing entity_id")
	}
}

// TestRegisterTools_NotFoundResults verifies that gitlab_get_bulk_import and
// gitlab_get_bulk_import_entity return informational NotFoundResult (IsError
// with hints) rather than transport errors when the API returns 404.
func TestRegisterTools_NotFoundResults(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	})
	session := newSession(t, mux)
	ctx := context.Background()

	cases := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_get_bulk_import", map[string]any{"id": 999}},
		{"gitlab_get_bulk_import_entity", map[string]any{"bulk_import_id": 999, "entity_id": 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
			if err != nil {
				t.Fatalf("CallTool error: %v", err)
			}
			if result == nil || !result.IsError {
				t.Fatalf("expected NotFoundResult IsError=true; got %+v", result)
			}
		})
	}
}

// TestRegisterTools_ErrorPaths covers the if-err branches in every list/get/
// cancel/failures closure when the GitLab API responds with a non-404 error.
// This exercises the LogToolCallAll(err) + return err paths that 404 short-
// circuit handling does not reach.
func TestRegisterTools_ErrorPaths(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"forbidden"}`)
	})
	session := newSession(t, mux)
	ctx := context.Background()

	cases := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_list_bulk_imports", map[string]any{}},
		{"gitlab_get_bulk_import", map[string]any{"id": 1}},
		{"gitlab_cancel_bulk_import", map[string]any{"id": 1}},
		{"gitlab_list_bulk_import_entities", map[string]any{"bulk_import_id": 1}},
		{"gitlab_get_bulk_import_entity", map[string]any{"bulk_import_id": 1, "entity_id": 7}},
		{"gitlab_list_bulk_import_entity_failures", map[string]any{"bulk_import_id": 1, "entity_id": 7}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
			if err != nil {
				t.Fatalf("CallTool error: %v", err)
			}
			if result == nil || !result.IsError {
				t.Fatalf("expected error result; got %+v", result)
			}
		})
	}
}

// TestToSummary_Nil ensures the nil guard in toSummary returns the zero value
// instead of panicking.
func TestToSummary_Nil(t *testing.T) {
	if got := toSummary(nil); got != (MigrationSummary{}) {
		t.Errorf("toSummary(nil) = %+v, want zero value", got)
	}
}

// TestToEntitySummary_Nil ensures the nil guard in toEntitySummary returns the
// zero value instead of panicking.
func TestToEntitySummary_Nil(t *testing.T) {
	if got := toEntitySummary(nil); got != (EntitySummary{}) {
		t.Errorf("toEntitySummary(nil) = %+v, want zero value", got)
	}
}

// TestFormatListMarkdown_Empty covers the early-return branch when no
// migrations are present.
func TestFormatListMarkdown_Empty(t *testing.T) {
	got := FormatListMarkdown(ListOutput{})
	if !strings.Contains(got, "_No migrations found._") {
		t.Errorf("expected empty placeholder; got %q", got)
	}
}

// TestFormatListEntitiesMarkdown_Empty covers the early-return branch when no
// entities are present.
func TestFormatListEntitiesMarkdown_Empty(t *testing.T) {
	got := FormatListEntitiesMarkdown(ListEntitiesOutput{})
	if !strings.Contains(got, "_No entities found._") {
		t.Errorf("expected empty placeholder; got %q", got)
	}
}

// TestFormatEntityFailuresMarkdown_Empty covers the early-return branch when no
// failures are present.
func TestFormatEntityFailuresMarkdown_Empty(t *testing.T) {
	got := FormatEntityFailuresMarkdown(ListEntityFailuresOutput{BulkImportID: 1, EntityID: 2})
	if !strings.Contains(got, "_No failures recorded._") {
		t.Errorf("expected empty placeholder; got %q", got)
	}
}
