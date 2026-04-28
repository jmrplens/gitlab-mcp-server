// bulkimports_test.go contains unit tests for the bulk import migration MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.

package bulkimports

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestStartMigration verifies the behavior of start migration.
func TestStartMigration(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/bulk_imports" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		testutil.RespondJSON(w, http.StatusOK, `{
			"id": 42,
			"status": "created",
			"source_type": "gitlab",
			"source_url": "https://source.gitlab.com",
			"created_at": "2026-01-01T00:00:00Z",
			"updated_at": "2026-01-01T00:00:00Z",
			"has_failures": false
		}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := StartMigration(t.Context(), client, StartMigrationInput{
		URL:         "https://source.gitlab.com",
		AccessToken: "glpat-test",
		Entities: []EntityInput{
			{
				SourceType:           "group_entity",
				SourceFullPath:       "source-group",
				DestinationSlug:      "dest-group",
				DestinationNamespace: "dest-ns",
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ID != 42 {
		t.Errorf("ID = %d, want 42", out.ID)
	}
	if out.Status != "created" {
		t.Errorf("Status = %q, want created", out.Status)
	}
	if out.HasFailures {
		t.Error("HasFailures = true, want false")
	}
}

// TestStartMigration_Error verifies that StartMigration handles the error scenario correctly.
func TestStartMigration_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := StartMigration(t.Context(), client, StartMigrationInput{
		URL:         "https://source.gitlab.com",
		AccessToken: "bad",
		Entities:    []EntityInput{},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestFormatStartMigrationMarkdown verifies the behavior of format start migration markdown.
func TestFormatStartMigrationMarkdown(t *testing.T) {
	out := MigrationOutput{
		ID:          1,
		Status:      "created",
		SourceType:  "gitlab",
		SourceURL:   "https://src.example.com",
		CreatedAt:   "2026-01-01",
		UpdatedAt:   "2026-01-01",
		HasFailures: false,
	}
	md := FormatStartMigrationMarkdown(out)
	if !strings.Contains(md, "Bulk Import") {
		t.Error("missing title")
	}
	if !strings.Contains(md, "created") {
		t.Error("missing status")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// StartMigration — with optional fields
// ---------------------------------------------------------------------------.

// TestStartMigration_WithOptionalFields verifies the behavior of start migration with optional fields.
func TestStartMigration_WithOptionalFields(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/bulk_imports" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id": 100,
				"status": "created",
				"source_type": "gitlab",
				"source_url": "https://source.gitlab.com",
				"created_at": "2026-01-01T00:00:00Z",
				"updated_at": "2026-01-01T00:00:00Z",
				"has_failures": false
			}`)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)

	migrateProjects := true
	migrateMemberships := false
	out, err := StartMigration(t.Context(), client, StartMigrationInput{
		URL:         "https://source.gitlab.com",
		AccessToken: "glpat-test",
		Entities: []EntityInput{
			{
				SourceType:           "group_entity",
				SourceFullPath:       "source-group",
				DestinationSlug:      "dest-group",
				DestinationNamespace: "dest-ns",
				MigrateProjects:      &migrateProjects,
				MigrateMemberships:   &migrateMemberships,
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ID != 100 {
		t.Errorf("ID = %d, want 100", out.ID)
	}
}

// ---------------------------------------------------------------------------
// FormatStartMigrationMarkdown — with failures
// ---------------------------------------------------------------------------.

// TestFormatStartMigrationMarkdown_WithFailures verifies the behavior of format start migration markdown with failures.
func TestFormatStartMigrationMarkdown_WithFailures(t *testing.T) {
	out := MigrationOutput{
		ID:          2,
		Status:      "failed",
		SourceType:  "gitlab",
		SourceURL:   "https://src|pipe.example.com",
		CreatedAt:   "2026-06-01",
		UpdatedAt:   "2026-06-02",
		HasFailures: true,
	}
	md := FormatStartMigrationMarkdown(out)
	if !strings.Contains(md, "failed") {
		t.Error("missing status")
	}
	if !strings.Contains(md, "true") {
		t.Error("missing has_failures=true")
	}
}

// ---------------------------------------------------------------------------
// RegisterTools — no panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// MCP round-trip
// ---------------------------------------------------------------------------.

// TestMCPRound_Trip verifies the behavior of m c p round trip.
func TestMCPRound_Trip(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("POST /api/v4/bulk_imports", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{
			"id": 42,
			"status": "created",
			"source_type": "gitlab",
			"source_url": "https://source.gitlab.com",
			"created_at": "2026-01-01T00:00:00Z",
			"updated_at": "2026-01-01T00:00:00Z",
			"has_failures": false
		}`)
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

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "gitlab_start_bulk_import",
		Arguments: map[string]any{
			"url":          "https://source.gitlab.com",
			"access_token": "glpat-test",
			"entities": []any{
				map[string]any{
					"source_type":           "group_entity",
					"source_full_path":      "source-group",
					"destination_slug":      "dest-group",
					"destination_namespace": "dest-ns",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result.IsError {
		t.Fatal("CallTool returned IsError=true")
	}
}

// TestMCPRound_TripAPIError verifies the register handler returns an error
// when the StartMigration API call fails.
func TestMCPRound_TripAPIError(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
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

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "gitlab_start_bulk_import",
		Arguments: map[string]any{
			"url":          "https://source.gitlab.com",
			"access_token": "glpat-test",
			"entities": []any{
				map[string]any{
					"source_type":           "group_entity",
					"source_full_path":      "source-group",
					"destination_slug":      "dest-group",
					"destination_namespace": "dest-ns",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result from API failure")
	}
}

// TestList_OK validates that List parses the GitLab response into the typed
// MigrationSummary slice and propagates pagination metadata.
func TestList_OK(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/bulk_imports", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if got := r.URL.Query().Get("status"); got != "started" {
			t.Errorf("status = %q, want started", got)
		}
		testutil.RespondJSON(w, http.StatusOK, `[
			{"id":1,"status":"started","source_type":"gitlab","source_url":"https://src","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","has_failures":false},
			{"id":2,"status":"started","source_type":"gitlab","source_url":"https://src","created_at":"2026-01-02T00:00:00Z","updated_at":"2026-01-02T00:00:00Z","has_failures":true}
		]`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := List(t.Context(), client, ListInput{Status: "started"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(out.Migrations) != 2 {
		t.Fatalf("len = %d, want 2", len(out.Migrations))
	}
	if out.Migrations[0].ID != 1 || out.Migrations[1].ID != 2 {
		t.Errorf("unexpected ids: %+v", out.Migrations)
	}
}

// TestGet_OK validates that Get retrieves a single migration by ID.
func TestGet_OK(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/bulk_imports/7", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":7,"status":"finished","source_type":"gitlab","source_url":"https://src","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-02T00:00:00Z","has_failures":false}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := Get(t.Context(), client, GetInput{ID: 7})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if out.ID != 7 || out.Status != "finished" {
		t.Errorf("got %+v", out)
	}
}

// TestGet_RequiresID validates required field check.
func TestGet_RequiresID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	if _, err := Get(t.Context(), client, GetInput{}); err == nil {
		t.Fatal("expected error for missing id")
	}
}

// TestCancel_OK validates that Cancel posts to the cancel endpoint and parses
// the returned MigrationSummary.
func TestCancel_OK(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/bulk_imports/9/cancel", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"id":9,"status":"canceled","source_type":"gitlab","source_url":"https://src","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-02T00:00:00Z","has_failures":false}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := Cancel(t.Context(), client, CancelInput{ID: 9})
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if out.ID != 9 || out.Status != "canceled" {
		t.Errorf("got %+v", out)
	}
}

// TestCancel_RequiresID validates required field check.
func TestCancel_RequiresID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	if _, err := Cancel(t.Context(), client, CancelInput{}); err == nil {
		t.Fatal("expected error for missing id")
	}
}

// TestListEntities_RejectsNegativeID ensures that a negative bulk_import_id is
// rejected rather than silently widening the query to all imports.
func TestListEntities_RejectsNegativeID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	if _, err := ListEntities(t.Context(), client, ListEntitiesInput{BulkImportID: -1}); err == nil {
		t.Fatal("expected error for negative bulk_import_id")
	}
}

// TestListEntities_AllScope hits /bulk_imports/entities when no bulk_import_id
// is supplied.
func TestListEntities_AllScope(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/bulk_imports/entities", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[
			{"id":1,"bulk_import_id":10,"status":"finished","entity_type":"group_entity","source_full_path":"src","destination_full_path":"dst","destination_name":"dst","destination_slug":"dst","destination_namespace":"ns","has_failures":false}
		]`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListEntities(t.Context(), client, ListEntitiesInput{})
	if err != nil {
		t.Fatalf("ListEntities: %v", err)
	}
	if len(out.Entities) != 1 || out.Entities[0].BulkImportID != 10 {
		t.Errorf("got %+v", out.Entities)
	}
}

// TestListEntities_PerImport hits /bulk_imports/{id}/entities when the import
// id is supplied.
func TestListEntities_PerImport(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/bulk_imports/55/entities", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListEntities(t.Context(), client, ListEntitiesInput{BulkImportID: 55})
	if err != nil {
		t.Fatalf("ListEntities: %v", err)
	}
	if len(out.Entities) != 0 {
		t.Errorf("len = %d, want 0", len(out.Entities))
	}
}

// TestGetEntity_OK validates retrieval of a single entity.
func TestGetEntity_OK(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/bulk_imports/3/entities/77", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":77,"bulk_import_id":3,"status":"started","entity_type":"project_entity","source_full_path":"src","destination_full_path":"dst","destination_name":"dst","destination_slug":"dst","destination_namespace":"ns","has_failures":false}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := GetEntity(t.Context(), client, GetEntityInput{BulkImportID: 3, EntityID: 77})
	if err != nil {
		t.Fatalf("GetEntity: %v", err)
	}
	if out.ID != 77 || out.EntityType != "project_entity" {
		t.Errorf("got %+v", out)
	}
}

// TestGetEntity_Validation validates required field checks.
func TestGetEntity_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	if _, err := GetEntity(t.Context(), client, GetEntityInput{}); err == nil {
		t.Error("expected error for missing bulk_import_id")
	}
	if _, err := GetEntity(t.Context(), client, GetEntityInput{BulkImportID: 1}); err == nil {
		t.Error("expected error for missing entity_id")
	}
}

// TestListEntityFailures_OK validates retrieval of failed import records.
func TestListEntityFailures_OK(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/bulk_imports/4/entities/88/failures", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[
			{"relation":"label","exception_class":"StandardError","exception_message":"boom","correlation_id_value":"abc","source_url":"https://src/path","pipeline_class":"Pipe","pipeline_step":"step1"}
		]`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListEntityFailures(t.Context(), client, ListEntityFailuresInput{BulkImportID: 4, EntityID: 88})
	if err != nil {
		t.Fatalf("ListEntityFailures: %v", err)
	}
	if len(out.Failures) != 1 || out.Failures[0].Relation != "label" {
		t.Errorf("got %+v", out.Failures)
	}
}

// TestFormatters_Smoke renders each formatter to ensure non-empty markdown.
func TestFormatters_Smoke(t *testing.T) {
	listOut := ListOutput{Migrations: []MigrationSummary{{ID: 1, Status: "started", SourceType: "gitlab", SourceURL: "https://src"}}}
	if md := FormatListMarkdown(listOut); !strings.Contains(md, "started") {
		t.Error("list markdown missing status")
	}
	if md := FormatGetMarkdown(MigrationSummary{ID: 5, Status: "finished"}); !strings.Contains(md, "finished") {
		t.Error("get markdown missing status")
	}
	entOut := ListEntitiesOutput{Entities: []EntitySummary{{ID: 7, BulkImportID: 3, EntityType: "group_entity", Status: "finished", SourceFullPath: "a", DestinationFullPath: "b"}}}
	if md := FormatListEntitiesMarkdown(entOut); !strings.Contains(md, "group_entity") {
		t.Error("entities markdown missing type")
	}
	if md := FormatGetEntityMarkdown(EntitySummary{ID: 9, EntityType: "project_entity", Status: "finished"}); !strings.Contains(md, "project_entity") {
		t.Error("get entity markdown missing type")
	}
	failOut := ListEntityFailuresOutput{Failures: []EntityFailure{{Relation: "labels", ExceptionClass: "Boom", ExceptionMessage: "x"}}}
	if md := FormatEntityFailuresMarkdown(failOut); !strings.Contains(md, "labels") {
		t.Error("failures markdown missing relation")
	}
}
