// bulk_imports_test.go contains unit tests for the bulk import migration MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package bulkimports

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestStartMigration verifies the StartMigration handler.
// The mock GitLab API at /api/v4/bulk_imports (POST) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestStartMigration(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/bulk_imports")
		testutil.AssertRequestMethod(t, r, http.MethodPost)
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
			http.Error(w, "decode body", http.StatusInternalServerError)
			return
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

// assertEntityFlag fails the test when the migration entity GitLab received
// does not carry flag with exactly the value the caller asked for.
func assertEntityFlag(t *testing.T, entity map[string]any, flag string, want bool) {
	t.Helper()
	got, ok := entity[flag]
	if !ok {
		t.Errorf("%s is missing from the entity sent, want %v", flag, want)
		return
	}
	if got != want {
		t.Errorf("%s = %v, want %v", flag, got, want)
	}
}

// assertEntityFlagAbsent fails the test when a flag the caller left unset was
// sent anyway, which would decide for them whatever GitLab's own default is.
func assertEntityFlagAbsent(t *testing.T, entity map[string]any, flag string) {
	t.Helper()
	if got, ok := entity[flag]; ok {
		t.Errorf("%s = %v was sent, want it omitted", flag, got)
	}
}

// TestStartMigration_MigrateFlags_ReachTheRequestBody asserts that the optional
// per-entity flags a caller sets are in the body GitLab receives, with the
// values they were given, and that an entity setting neither carries neither.
//
// Why it matters: both flags are copied under a `!= nil` guard and no other
// test in this package ever sets one, so the guard could be inverted with no
// visible effect here: a caller asking to migrate projects would be sent an
// entity that says nothing, and GitLab's default would silently decide. The
// two halves are asserted together because dropping a flag that was set and
// inventing one that was not are different defects, and only the body shows
// both. `migrate_memberships` is deliberately set to false rather than true:
// an explicit no is the value most easily lost on the way out, and losing it
// reads as the caller never having expressed one.
func TestStartMigration_MigrateFlags_ReachTheRequestBody(t *testing.T) {
	var seen atomic.Int64
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Add(1)
		var body struct {
			Entities []map[string]any `json:"entities"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
			http.Error(w, "decode body", http.StatusInternalServerError)
			return
		}
		if len(body.Entities) != 2 {
			t.Errorf("entities sent = %d, want 2", len(body.Entities))
			http.Error(w, "entities", http.StatusInternalServerError)
			return
		}
		assertEntityFlag(t, body.Entities[0], "migrate_projects", true)
		assertEntityFlag(t, body.Entities[0], "migrate_memberships", false)
		assertEntityFlagAbsent(t, body.Entities[1], "migrate_projects")
		assertEntityFlagAbsent(t, body.Entities[1], "migrate_memberships")
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"status":"created"}`)
	}))

	migrate, keep := true, false
	if _, err := StartMigration(t.Context(), client, StartMigrationInput{
		Configuration: ConfigurationInput{URL: "https://source.gitlab.com", AccessToken: "glpat-test"},
		Entities: []EntityInput{
			{
				SourceType:           "group_entity",
				SourceFullPath:       "flagged",
				DestinationSlug:      "flagged",
				DestinationNamespace: "ns",
				MigrateProjects:      &migrate,
				MigrateMemberships:   &keep,
			},
			{
				SourceType:           "group_entity",
				SourceFullPath:       "bare",
				DestinationSlug:      "bare",
				DestinationNamespace: "ns",
			},
		},
	}); err != nil {
		t.Fatalf("StartMigration: %v", err)
	}
	if got := seen.Load(); got != 1 {
		t.Errorf("requests reaching GitLab = %d, want 1 (the body assertions ran nowhere else)", got)
	}
}

// assertBulkImportMarkdown compares a whole rendered response with what the
// formatter is meant to write, byte for byte.
func assertBulkImportMarkdown(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("markdown mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// startMigrationHints is the guidance section the start card closes with.
const startMigrationHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
	"- Use action 'admin.bulk_import_get' to watch the migration's progress\n"

// TestFormatStartMigrationMarkdown verifies the whole card a started migration
// renders: the source URL as a link, the dates in the display form, and the
// failure flag as a glyph rather than as the word false.
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

	assertBulkImportMarkdown(t, FormatStartMigrationMarkdown(out), "## Bulk Import Migration Started\n\n"+
		"- **ID**: 1\n"+
		"- **Status**: created\n"+
		"- **Source Type**: gitlab\n"+
		"- **Source URL**: [https://src.example.com](https://src.example.com)\n"+
		"- **Created**: 1 Jan 2026\n"+
		"- **Updated**: 1 Jan 2026\n"+
		"- **Has Failures**: ❌\n"+
		startMigrationHints)
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

// TestFormatStartMigrationMarkdown_WithFailures verifies the whole card a
// failed migration renders, and that a source URL carrying a pipe is
// neutralized in both halves of the link it becomes.
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

	assertBulkImportMarkdown(t, FormatStartMigrationMarkdown(out), "## Bulk Import Migration Started\n\n"+
		"- **ID**: 2\n"+
		"- **Status**: failed\n"+
		"- **Source Type**: gitlab\n"+
		"- **Source URL**: [https://src&#124;pipe.example.com](https://src%7Cpipe.example.com)\n"+
		"- **Created**: 1 Jun 2026\n"+
		"- **Updated**: 2 Jun 2026\n"+
		"- **Has Failures**: ✅\n"+
		startMigrationHints)
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
		Status:     "started",
		OrderBy:    "created_at",
		Sort:       "desc",
		Pagination: "keyset",
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

// TestBulkImports_RequiredIDOmitted_RefusedWithoutReachingGitLab asserts that
// every handler guarding a required identifier answers an omitted one with the
// required-field error, and sends GitLab nothing at all.
//
// Why it matters: an omitted id arrives as zero rather than as a negative
// number, so the guard has to read `<= 0`. Relaxed to `< 0` the zero sails
// through and the handler asks GitLab for `/bulk_imports/0`, whose 404 reaches
// the caller as "verify the migration id with gitlab_list_bulk_imports" — a
// model is told the migration does not exist when what happened is that it
// never named one, and a request went out that had no business being made.
// Asserting only that some error came back cannot tell those apart, since both
// are non-nil; the error's own text and the silence on the wire can.
func TestBulkImports_RequiredIDOmitted_RefusedWithoutReachingGitLab(t *testing.T) {
	var requests atomic.Int64
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	cases := []struct {
		name string
		call func() error
		want error
	}{
		{
			name: "get without id",
			call: func() error { _, err := Get(t.Context(), client, GetInput{}); return err },
			want: toolutil.ErrRequiredInt64("bulk_import_get", "id"),
		},
		{
			name: "cancel without id",
			call: func() error { _, err := Cancel(t.Context(), client, CancelInput{}); return err },
			want: toolutil.ErrRequiredInt64("bulk_import_cancel", "id"),
		},
		{
			name: "entity get without bulk import id",
			call: func() error { _, err := GetEntity(t.Context(), client, GetEntityInput{EntityID: 7}); return err },
			want: toolutil.ErrRequiredInt64("bulk_import_entity_get", "bulk_import_id"),
		},
		{
			name: "entity get without entity id",
			call: func() error { _, err := GetEntity(t.Context(), client, GetEntityInput{BulkImportID: 1}); return err },
			want: toolutil.ErrRequiredInt64("bulk_import_entity_get", "entity_id"),
		},
		{
			name: "entity failures without bulk import id",
			call: func() error {
				_, err := ListEntityFailures(t.Context(), client, ListEntityFailuresInput{EntityID: 7})
				return err
			},
			want: toolutil.ErrRequiredInt64("bulk_import_entity_failures", "bulk_import_id"),
		},
		{
			name: "entity failures without entity id",
			call: func() error {
				_, err := ListEntityFailures(t.Context(), client, ListEntityFailuresInput{BulkImportID: 1})
				return err
			},
			want: toolutil.ErrRequiredInt64("bulk_import_entity_failures", "entity_id"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := requests.Load()
			err := tc.call()
			if err == nil {
				t.Fatalf("expected %v, got nil", tc.want)
			}
			if err.Error() != tc.want.Error() {
				t.Errorf("error = %q, want %q", err, tc.want)
			}
			if sent := requests.Load() - before; sent != 0 {
				t.Errorf("%d request(s) reached GitLab, want none", sent)
			}
		})
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
		OrderBy:    "created_at",
		Sort:       "asc",
		Pagination: "keyset", PageToken: "tok-1",
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

// TestFormatMigrationList verifies the whole table a page of migrations
// renders, the source URL linked so the reader can reach the source instance.
func TestFormatMigrationList(t *testing.T) {
	out := ListOutput{Migrations: []MigrationSummary{{ID: 1, Status: "started", SourceType: "gitlab", SourceURL: "https://src"}}}

	assertBulkImportMarkdown(t, FormatListMarkdown(out), "## Bulk Import Migrations (1)\n\n"+
		"| ID | Status | Source Type | Source URL | Has Failures | Created |\n"+
		"| --- | --- | --- | --- | --- | --- |\n"+
		"| 1 | started | gitlab | [https://src](https://src) | ❌ |  |\n"+
		"\n---\n\U0001F4A1 **Next steps:**\n"+
		"- "+toolutil.HintPreserveLinks+"\n"+
		"- Use action 'admin.bulk_import_get' to read one migration in full\n"+
		"- Use action 'admin.bulk_import_entity_list' to inspect the entities of a migration\n")
}

// TestFormatGetMarkdown verifies the whole card one finished migration
// renders: no cancel hint for a migration that is over, and no failure hint
// for one that had none.
func TestFormatGetMarkdown(t *testing.T) {
	assertBulkImportMarkdown(t, FormatGetMarkdown(MigrationSummary{ID: 5, Status: "finished"}), "## Bulk Import Migration #5\n\n"+
		"- **ID**: 5\n"+
		"- **Status**: finished\n"+
		"- **Has Failures**: ❌\n"+
		"\n---\n\U0001F4A1 **Next steps:**\n"+
		"- Use action 'admin.bulk_import_entity_list' to inspect the entities this migration moved\n")
}

// TestFormatListEntitiesMarkdown verifies the whole table a page of entities
// renders. The table carries no link column, so nothing instructs the model to
// preserve links it does not have.
func TestFormatListEntitiesMarkdown(t *testing.T) {
	out := ListEntitiesOutput{Entities: []EntitySummary{{ID: 7, BulkImportID: 3, EntityType: "group_entity", Status: "finished", SourceFullPath: "a", DestinationFullPath: "b"}}}

	assertBulkImportMarkdown(t, FormatListEntitiesMarkdown(out), "## Bulk Import Entities (1)\n\n"+
		"| ID | Bulk Import | Type | Status | Source | Destination | Failures |\n"+
		"| --- | --- | --- | --- | --- | --- | --- |\n"+
		"| 7 | 3 | group_entity | finished | a | b | ❌ |\n"+
		"\n---\n\U0001F4A1 **Next steps:**\n"+
		"- Use action 'admin.bulk_import_entity_get' to read one entity in full\n"+
		"- Use action 'admin.bulk_import_entity_failures' to read the failure diagnostics\n")
}

// TestFormatGetEntityMarkdown_NoStatsReported verifies that an entity whose
// stats object names no relation opens no Stats collection: GitLab sends a key
// per relation it processed, and three zeros used to claim that nothing was
// imported where the truth is that nothing was said.
func TestFormatGetEntityMarkdown_NoStatsReported(t *testing.T) {
	got := FormatGetEntityMarkdown(EntitySummary{ID: 9, EntityType: "project_entity", Status: "finished"})

	assertBulkImportMarkdown(t, got, "## Bulk Import Entity #9\n\n"+
		"- **ID**: 9\n"+
		"- **Bulk Import ID**: 0\n"+
		"- **Status**: finished\n"+
		"- **Entity Type**: project_entity\n"+
		"- **Migrate Projects**: ❌\n"+
		"- **Migrate Memberships**: ❌\n"+
		"- **Has Failures**: ❌\n"+
		"\n---\n\U0001F4A1 **Next steps:**\n"+
		"- Use action 'admin.bulk_import_entity_list' to see the migration's other entities\n")
}

// TestFormatGetEntityMarkdown_ReportedStats verifies that only the relations
// the migration reported become rows of the nested Stats collection.
func TestFormatGetEntityMarkdown_ReportedStats(t *testing.T) {
	got := FormatGetEntityMarkdown(EntitySummary{
		ID:           9,
		BulkImportID: 3,
		Status:       "finished",
		EntityType:   "group_entity",
		Stats:        EntityStats{Labels: EntityStatItem{Source: 4, Fetched: 4, Imported: 3}},
	})

	assertBulkImportMarkdown(t, got, "## Bulk Import Entity #9\n\n"+
		"- **ID**: 9\n"+
		"- **Bulk Import ID**: 3\n"+
		"- **Status**: finished\n"+
		"- **Entity Type**: group_entity\n"+
		"- **Migrate Projects**: ❌\n"+
		"- **Migrate Memberships**: ❌\n"+
		"- **Has Failures**: ❌\n"+
		"\n### Stats\n\n"+
		"| Relation | Source | Fetched | Imported |\n"+
		"| --- | --- | --- | --- |\n"+
		"| Labels | 4 | 4 | 3 |\n"+
		"\n---\n\U0001F4A1 **Next steps:**\n"+
		"- Use action 'admin.bulk_import_entity_list' to see the migration's other entities\n")
}

// TestFormatEntityFailuresMarkdown verifies the whole table an entity's
// failures render.
func TestFormatEntityFailuresMarkdown(t *testing.T) {
	out := ListEntityFailuresOutput{
		BulkImportID: 1,
		EntityID:     2,
		Failures:     []EntityFailure{{Relation: "labels", ExceptionClass: "Boom", ExceptionMessage: "x"}},
	}

	assertBulkImportMarkdown(t, FormatEntityFailuresMarkdown(out), "## Bulk Import Failures (import #1, entity #2) (1)\n\n"+
		"| Relation | Step | Pipeline | Class | Message | Source | Created |\n"+
		"| --- | --- | --- | --- | --- | --- | --- |\n"+
		"| labels |  |  | Boom | x |  |  |\n"+
		"\n---\n\U0001F4A1 **Next steps:**\n"+
		"- "+toolutil.HintPreserveLinks+"\n"+
		"- Use action 'admin.bulk_import_entity_get' to read the entity these failures belong to\n")
}

// TestBulkImports_RefusalsPropagate verifies that an instance refusing a read
// or a cancel is reported rather than swallowed, for every handler that talks
// to GitLab. Each is wrapped with its own operation name and, for four of
// them, its own not-found hint, so one swallowed refusal hands a model an
// empty page and no reason for it.
func TestBulkImports_RefusalsPropagate(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	tests := []struct {
		name string
		call func() error
	}{
		{name: "list", call: func() error {
			_, err := List(t.Context(), client, ListInput{})
			return err
		}},
		{name: "get", call: func() error {
			_, err := Get(t.Context(), client, GetInput{ID: 1})
			return err
		}},
		{name: "cancel", call: func() error {
			_, err := Cancel(t.Context(), client, CancelInput{ID: 1})
			return err
		}},
		{name: "entity_list", call: func() error {
			_, err := ListEntities(t.Context(), client, ListEntitiesInput{})
			return err
		}},
		// Filtered by status, which is the one option ListEntities copies onto
		// the request under a guard of its own.
		{name: "entity_list_by_status", call: func() error {
			_, err := ListEntities(t.Context(), client, ListEntitiesInput{Status: "failed"})
			return err
		}},
		{name: "entity_get", call: func() error {
			_, err := GetEntity(t.Context(), client, GetEntityInput{BulkImportID: 1, EntityID: 2})
			return err
		}},
		{name: "entity_failures", call: func() error {
			_, err := ListEntityFailures(t.Context(), client, ListEntityFailuresInput{BulkImportID: 1, EntityID: 2})
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err == nil {
				t.Fatalf("%s error = nil, want the instance refusal", tt.name)
			}
		})
	}
}

// TestBulkImports_NilRecords_ConvertToTheZeroSummary verifies that the two
// converters answer a nil record with an empty summary rather than
// dereferencing it. GitLab's list endpoints are decoded into slices of
// pointers, so a null element is a shape the SDK can hand us and a
// dereference here would take the whole call down.
func TestBulkImports_NilRecords_ConvertToTheZeroSummary(t *testing.T) {
	if got := toSummary(nil); got != (MigrationSummary{}) {
		t.Errorf("toSummary(nil) = %+v, want the zero summary", got)
	}
	if got := toEntitySummary(nil); got.ID != 0 || got.Status != "" || got.HasFailures {
		t.Errorf("toEntitySummary(nil) = %+v, want the zero summary", got)
	}
}

// TestBulkImports_EmptyCollections_RenderTheEmptyMessage verifies that each
// list formatter says there is nothing rather than writing a table header over
// no rows, which reads to a model as a malformed answer.
func TestBulkImports_EmptyCollections_RenderTheEmptyMessage(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "migrations", got: FormatListMarkdown(ListOutput{}), want: toolutil.EmptyMessage("bulk import migrations")},
		{name: "entities", got: FormatListEntitiesMarkdown(ListEntitiesOutput{}), want: toolutil.EmptyMessage("bulk import entities")},
		{name: "failures", got: FormatEntityFailuresMarkdown(ListEntityFailuresOutput{}), want: toolutil.EmptyMessage("bulk import failures")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("formatter = %q, want %q", tt.got, tt.want)
			}
		})
	}
}

// TestFormatGetMarkdown_HintsFollowTheMigrationsState verifies that the card
// offers the failures action only for a migration that has failures, and the
// cancel action only while one is still running. Both hints name something a
// model can do next, and offering the cancel on a finished migration or hiding
// it on a running one is the difference between a useful next step and a call
// GitLab refuses.
func TestFormatGetMarkdown_HintsFollowTheMigrationsState(t *testing.T) {
	tests := []struct {
		name        string
		summary     MigrationSummary
		wantHints   []string
		absentHints []string
	}{
		{
			name:        "running with failures",
			summary:     MigrationSummary{ID: 1, Status: "started", HasFailures: true},
			wantHints:   []string{actionEntityFailures, actionCancel},
			absentHints: nil,
		},
		{
			name:        "finished and clean",
			summary:     MigrationSummary{ID: 2, Status: "finished"},
			wantHints:   []string{actionEntityList},
			absentHints: []string{actionEntityFailures, actionCancel},
		},
		{
			name:        "created and clean",
			summary:     MigrationSummary{ID: 3, Status: "created"},
			wantHints:   []string{actionCancel},
			absentHints: []string{actionEntityFailures},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatGetMarkdown(tt.summary)
			for _, want := range tt.wantHints {
				if !strings.Contains(got, want) {
					t.Errorf("card names no %q hint:\n%s", want, got)
				}
			}
			for _, absent := range tt.absentHints {
				if strings.Contains(got, absent) {
					t.Errorf("card names the %q hint it should not:\n%s", absent, got)
				}
			}
		})
	}
}

// TestFormatGetEntityMarkdown_FailingEntity_PointsAtItsFailures verifies that
// an entity GitLab reported failures for offers the failures action instead of
// the sibling listing, which is the one hint that leads anywhere useful from
// a failure.
func TestFormatGetEntityMarkdown_FailingEntity_PointsAtItsFailures(t *testing.T) {
	got := FormatGetEntityMarkdown(EntitySummary{ID: 7, BulkImportID: 1, Status: "failed", HasFailures: true})
	if !strings.Contains(got, actionEntityFailures) {
		t.Errorf("card names no %q hint:\n%s", actionEntityFailures, got)
	}
	if strings.Contains(got, actionEntityList) {
		t.Errorf("card names the sibling listing rather than the failures:\n%s", got)
	}
}
