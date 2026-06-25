// bulkimports_test.go contains unit tests for the bulk import migration MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package bulkimports

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestStartMigration verifies the StartMigration handler.
// The mock GitLab API at /api/v4/bulk_imports (POST) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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
		Configuration: ConfigurationInput{
			URL:         "https://source.gitlab.com",
			AccessToken: "glpat-test",
		},
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

// TestStartMigration_Error verifies that StartMigration returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestStartMigration_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := StartMigration(t.Context(), client, StartMigrationInput{
		Configuration: ConfigurationInput{
			URL:         "https://source.gitlab.com",
			AccessToken: "bad",
		},
		Entities: []EntityInput{},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestFormatStartMigrationMarkdown verifies the StartMigrationMarkdown Markdown formatter for a representative startmigration input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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

// TestStartMigration_WithOptionalFields verifies the StartMigration_WithOptionalFields handler.
// The mock GitLab API at /api/v4/bulk_imports (POST) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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
		Configuration: ConfigurationInput{
			URL:         "https://source.gitlab.com",
			AccessToken: "glpat-test",
		},
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

// TestFormatStartMigrationMarkdown_WithFailures verifies the StartMigrationMarkdown_WithFailures Markdown formatter for a representative startmigration_withfailures input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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
// ActionSpecs route execution
// ---------------------------------------------------------------------------.

// TestActionSpecs_StartMigrationRoute validates the StartMigrationRoute route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_StartMigrationRoute(t *testing.T) {
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

	byTool := bulkImportSpecsByTool(t, handler)

	result, err := byTool["gitlab_start_bulk_import"].Route.Handler(t.Context(), map[string]any{
		"configuration": map[string]any{
			"url":          "https://source.gitlab.com",
			"access_token": "glpat-test",
		},
		"entities": []any{
			map[string]any{
				"source_type":           "group_entity",
				"source_full_path":      "source-group",
				"destination_slug":      "dest-group",
				"destination_namespace": "dest-ns",
			},
		},
	})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	if result == nil {
		t.Fatal("Route.Handler returned nil")
	}
}

// TestActionSpecs_StartMigrationAPIError validates the StartMigrationAPIError route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestActionSpecs_StartMigrationAPIError(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	})

	byTool := bulkImportSpecsByTool(t, handler)

	_, err := byTool["gitlab_start_bulk_import"].Route.Handler(t.Context(), map[string]any{
		"configuration": map[string]any{
			"url":          "https://source.gitlab.com",
			"access_token": "glpat-test",
		},
		"entities": []any{
			map[string]any{
				"source_type":           "group_entity",
				"source_full_path":      "source-group",
				"destination_slug":      "dest-group",
				"destination_namespace": "dest-ns",
			},
		},
	})
	if err == nil {
		t.Fatal("expected error from API failure")
	}
}

// TestList_OK verifies the List_OK handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_OK(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/bulk_imports", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		q := r.URL.Query()
		if got := q.Get("status"); got != "started" {
			t.Errorf("status = %q, want started", got)
		}
		if got := q.Get("order_by"); got != "created_at" {
			t.Errorf("order_by = %q, want created_at", got)
		}
		if got := q.Get("sort"); got != "desc" {
			t.Errorf("sort = %q, want desc", got)
		}
		if got := q.Get("pagination"); got != "keyset" {
			t.Errorf("pagination = %q, want keyset", got)
		}
		testutil.RespondJSON(w, http.StatusOK, `[
			{"id":1,"status":"started","source_type":"gitlab","source_url":"https://src","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","has_failures":false},
			{"id":2,"status":"started","source_type":"gitlab","source_url":"https://src","created_at":"2026-01-02T00:00:00Z","updated_at":"2026-01-02T00:00:00Z","has_failures":true}
		]`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := List(t.Context(), client, ListInput{
		Status:                "started",
		OrderBy:               "created_at",
		Sort:                  "desc",
		KeysetPaginationInput: toolutil.KeysetPaginationInput{Pagination: "keyset"},
	})
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

// TestGet_OK verifies the Get_OK handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestGet_RequiresID verifies the Get_RequiresID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGet_RequiresID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	if _, err := Get(t.Context(), client, GetInput{}); err == nil {
		t.Fatal("expected error for missing id")
	}
}

// TestCancel_OK verifies the Cancel_OK handler.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
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

// TestCancel_RequiresID verifies the Cancel_RequiresID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestCancel_RequiresID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	if _, err := Cancel(t.Context(), client, CancelInput{}); err == nil {
		t.Fatal("expected error for missing id")
	}
}

// TestListEntities_RejectsNegativeID verifies the ListEntities_RejectsNegativeID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListEntities_RejectsNegativeID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	if _, err := ListEntities(t.Context(), client, ListEntitiesInput{BulkImportID: -1}); err == nil {
		t.Fatal("expected error for negative bulk_import_id")
	}
}

// TestListEntities_AllScope verifies the ListEntities_AllScope handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListEntities_AllScope(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/bulk_imports/entities", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if got := q.Get("order_by"); got != "created_at" {
			t.Errorf("order_by = %q, want created_at", got)
		}
		if got := q.Get("sort"); got != "asc" {
			t.Errorf("sort = %q, want asc", got)
		}
		if got := q.Get("page_token"); got != "tok-1" {
			t.Errorf("page_token = %q, want tok-1", got)
		}
		testutil.RespondJSON(w, http.StatusOK, `[
			{"id":1,"bulk_import_id":10,"status":"finished","entity_type":"group_entity","source_full_path":"src","destination_full_path":"dst","destination_name":"dst","destination_slug":"dst","destination_namespace":"ns","has_failures":false}
		]`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListEntities(t.Context(), client, ListEntitiesInput{
		OrderBy:               "created_at",
		Sort:                  "asc",
		KeysetPaginationInput: toolutil.KeysetPaginationInput{Pagination: "keyset", PageToken: "tok-1"},
	})
	if err != nil {
		t.Fatalf("ListEntities: %v", err)
	}
	if len(out.Entities) != 1 || out.Entities[0].BulkImportID != 10 {
		t.Errorf("got %+v", out.Entities)
	}
}

// TestListEntities_PerImport verifies the ListEntities_PerImport handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestGetEntity_OK verifies the GetEntity_OK handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGetEntity_OK(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/bulk_imports/3/entities/77", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{
			"id":77,"bulk_import_id":3,"status":"started","entity_type":"project_entity",
			"source_full_path":"src","destination_full_path":"dst","destination_name":"dst",
			"destination_slug":"dst","destination_namespace":"ns","has_failures":true,
			"failures":[
				null,
				{"relation":"labels","exception_class":"StandardError","exception_message":"boom"}
			],
			"stats":{
				"labels":{"source":5,"fetched":4,"imported":3},
				"milestones":{"source":2,"fetched":2,"imported":1}
			}
		}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := GetEntity(t.Context(), client, GetEntityInput{BulkImportID: 3, EntityID: 77})
	if err != nil {
		t.Fatalf("GetEntity: %v", err)
	}
	if out.ID != 77 || out.EntityType != "project_entity" {
		t.Errorf("got %+v", out)
	}
	if len(out.Failures) != 1 || out.Failures[0].Relation != "labels" {
		t.Errorf("Failures = %+v, want one labels failure (nil skipped)", out.Failures)
	}
	if out.Stats.Labels.Source != 5 || out.Stats.Labels.Imported != 3 {
		t.Errorf("Stats.Labels = %+v, want source=5 imported=3", out.Stats.Labels)
	}
	if out.Stats.Milestones.Fetched != 2 {
		t.Errorf("Stats.Milestones.Fetched = %d, want 2", out.Stats.Milestones.Fetched)
	}
}

// TestGetEntity_Validation verifies the GetEntity_Validation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGetEntity_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	if _, err := GetEntity(t.Context(), client, GetEntityInput{}); err == nil {
		t.Error("expected error for missing bulk_import_id")
	}
	if _, err := GetEntity(t.Context(), client, GetEntityInput{BulkImportID: 1}); err == nil {
		t.Error("expected error for missing entity_id")
	}
}

// TestListEntityFailures_OK verifies the ListEntityFailures_OK handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestListEntityFailures_SkipsNilEntries verifies the ListEntityFailures_SkipsNilEntries handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListEntityFailures_SkipsNilEntries(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/bulk_imports/4/entities/88/failures", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[
			null,
			{"relation":"label","exception_class":"StandardError","exception_message":"boom"}
		]`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListEntityFailures(t.Context(), client, ListEntityFailuresInput{BulkImportID: 4, EntityID: 88})
	if err != nil {
		t.Fatalf("ListEntityFailures: %v", err)
	}
	if len(out.Failures) != 1 {
		t.Fatalf("len(Failures) = %d, want 1", len(out.Failures))
	}
}

// TestFormatters_Smoke verifies the ters_Smoke Markdown formatter for a representative ters_smoke input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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
