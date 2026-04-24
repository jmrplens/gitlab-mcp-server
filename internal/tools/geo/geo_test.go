// geo_test.go contains unit tests for GitLab Geo site operations.
// Tests use httptest to mock the GitLab Geo API.

package geo

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const geoSiteJSON = `{
	"id": 1,
	"name": "primary-site",
	"url": "https://primary.example.com",
	"internal_url": "https://primary.internal",
	"primary": true,
	"enabled": true,
	"current": true,
	"files_max_capacity": 10,
	"repos_max_capacity": 25,
	"verification_max_capacity": 100,
	"container_repositories_max_capacity": 10,
	"sync_object_storage": false,
	"selective_sync_type": "",
	"minimum_reverification_interval": 7,
	"web_edit_url": "https://primary.example.com/admin/geo/sites/1/edit"
}`

const geoSiteStatusJSON = `{
	"geo_node_id": 1,
	"healthy": true,
	"health": "Healthy",
	"health_status": "Healthy",
	"missing_oauth_application": false,
	"db_replication_lag_seconds": 0,
	"projects_count": 42,
	"lfs_objects_synced_in_percentage": "100.00%",
	"job_artifacts_synced_in_percentage": "99.50%",
	"uploads_synced_in_percentage": "100.00%",
	"version": "16.5.0",
	"revision": "abc123",
	"storage_shards_match": true,
	"updated_at": "2026-01-15T10:30:00Z"
}`

// TestCreate_Success verifies that Create posts to /api/v4/geo_sites and
// returns the created Geo site with the expected ID, name and primary flag.
func TestCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/geo_sites" {
			testutil.RespondJSON(w, http.StatusCreated, geoSiteJSON)
			return
		}
		http.NotFound(w, r)
	}))

	name := "primary-site"
	out, err := Create(context.Background(), client, CreateInput{Name: &name})
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("expected ID 1, got %d", out.ID)
	}
	if out.Name != "primary-site" {
		t.Errorf("expected name primary-site, got %s", out.Name)
	}
	if !out.Primary {
		t.Error("expected primary to be true")
	}
}

// TestCreate_APIError verifies that Create returns an error when the GitLab
// Geo API responds with 403 Forbidden.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := Create(context.Background(), client, CreateInput{})
	if err == nil {
		t.Fatal("expected error on API failure")
	}
}

// TestList_Success verifies that List returns the Geo sites returned by
// GET /api/v4/geo_sites, preserving site names and fields.
func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/geo_sites" {
			testutil.RespondJSON(w, http.StatusOK, `[`+geoSiteJSON+`]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(out.Sites) != 1 {
		t.Fatalf("expected 1 site, got %d", len(out.Sites))
	}
	if out.Sites[0].Name != "primary-site" {
		t.Errorf("expected name primary-site, got %s", out.Sites[0].Name)
	}
}

// TestList_Empty verifies that List returns an empty slice when the Geo
// API responds with an empty array.
func TestList_Empty(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/geo_sites" {
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(out.Sites) != 0 {
		t.Fatalf("expected 0 sites, got %d", len(out.Sites))
	}
}

// TestList_APIError verifies that List returns an error when the GitLab
// Geo API responds with 400 Bad Request.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error on API failure")
	}
}

// TestGet_Success verifies that Get retrieves a single Geo site by ID and
// returns the expected URL and identifier.
func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/geo_sites/1" {
			testutil.RespondJSON(w, http.StatusOK, geoSiteJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, IDInput{ID: 1})
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("expected ID 1, got %d", out.ID)
	}
	if out.URL != "https://primary.example.com" {
		t.Errorf("expected URL https://primary.example.com, got %s", out.URL)
	}
}

// TestGet_MissingID verifies that Get returns a validation error when ID
// is zero.
func TestGet_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Get(context.Background(), client, IDInput{})
	if err == nil {
		t.Fatal("expected error for missing id")
	}
}

// TestGet_NotFound verifies that Get returns an error when the Geo API
// responds with 404 for an unknown site ID.
func TestGet_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	_, err := Get(context.Background(), client, IDInput{ID: 999})
	if err == nil {
		t.Fatal("expected error for not found site")
	}
}

// TestEdit_Success verifies that Edit issues PUT /api/v4/geo_sites/:id and
// returns the updated site.
func TestEdit_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/geo_sites/1" {
			testutil.RespondJSON(w, http.StatusOK, geoSiteJSON)
			return
		}
		http.NotFound(w, r)
	}))

	newName := "updated-site"
	out, err := Edit(context.Background(), client, EditInput{ID: 1, Name: &newName})
	if err != nil {
		t.Fatalf("Edit() error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("expected ID 1, got %d", out.ID)
	}
}

// TestEdit_MissingID verifies that Edit returns a validation error when
// ID is zero.
func TestEdit_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Edit(context.Background(), client, EditInput{})
	if err == nil {
		t.Fatal("expected error for missing id")
	}
}

// TestEdit_APIError verifies that Edit returns an error when the Geo API
// responds with 422 Unprocessable Entity.
func TestEdit_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}))

	_, err := Edit(context.Background(), client, EditInput{ID: 1})
	if err == nil {
		t.Fatal("expected error on API failure")
	}
}

// TestDelete_Success verifies that Delete issues DELETE /api/v4/geo_sites/:id
// and returns no error on 204 No Content.
func TestDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v4/geo_sites/1" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, IDInput{ID: 1})
	if err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
}

// TestDelete_MissingID verifies that Delete returns a validation error
// when ID is zero.
func TestDelete_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := Delete(context.Background(), client, IDInput{})
	if err == nil {
		t.Fatal("expected error for missing id")
	}
}

// TestDelete_APIError verifies that Delete returns an error when the Geo
// API responds with 403 Forbidden.
func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	err := Delete(context.Background(), client, IDInput{ID: 1})
	if err == nil {
		t.Fatal("expected error on API failure")
	}
}

// TestRepair_Success verifies that Repair issues POST /api/v4/geo_sites/:id/repair
// and returns the repaired site.
func TestRepair_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/geo_sites/1/repair" {
			testutil.RespondJSON(w, http.StatusOK, geoSiteJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Repair(context.Background(), client, IDInput{ID: 1})
	if err != nil {
		t.Fatalf("Repair() error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("expected ID 1, got %d", out.ID)
	}
}

// TestRepair_MissingID verifies that Repair returns a validation error
// when ID is zero.
func TestRepair_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Repair(context.Background(), client, IDInput{})
	if err == nil {
		t.Fatal("expected error for missing id")
	}
}

// TestRepair_APIError verifies that Repair returns an error when the Geo
// API responds with 400 Bad Request.
func TestRepair_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	_, err := Repair(context.Background(), client, IDInput{ID: 1})
	if err == nil {
		t.Fatal("expected error on API failure")
	}
}

// TestListStatus_Success verifies that ListStatus returns the Geo site
// statuses from /api/v4/geo_sites/status including health and version.
func TestListStatus_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/geo_sites/status" {
			testutil.RespondJSON(w, http.StatusOK, `[`+geoSiteStatusJSON+`]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListStatus(context.Background(), client, ListStatusInput{})
	if err != nil {
		t.Fatalf("ListStatus() error: %v", err)
	}
	if len(out.Statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(out.Statuses))
	}
	if out.Statuses[0].GeoNodeID != 1 {
		t.Errorf("expected geo_node_id 1, got %d", out.Statuses[0].GeoNodeID)
	}
	if !out.Statuses[0].Healthy {
		t.Error("expected healthy to be true")
	}
	if out.Statuses[0].Version != "16.5.0" {
		t.Errorf("expected version 16.5.0, got %s", out.Statuses[0].Version)
	}
}

// TestListStatus_Empty verifies that ListStatus returns an empty slice
// when the Geo status endpoint responds with an empty array.
func TestListStatus_Empty(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/geo_sites/status" {
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListStatus(context.Background(), client, ListStatusInput{})
	if err != nil {
		t.Fatalf("ListStatus() error: %v", err)
	}
	if len(out.Statuses) != 0 {
		t.Fatalf("expected 0 statuses, got %d", len(out.Statuses))
	}
}

// TestListStatus_APIError verifies that ListStatus returns an error when
// the Geo status endpoint responds with 403 Forbidden.
func TestListStatus_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := ListStatus(context.Background(), client, ListStatusInput{})
	if err == nil {
		t.Fatal("expected error on API failure")
	}
}

// TestGetStatus_Success verifies that GetStatus retrieves the status for
// a single Geo site including health, version and project count.
func TestGetStatus_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/geo_sites/1/status" {
			testutil.RespondJSON(w, http.StatusOK, geoSiteStatusJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetStatus(context.Background(), client, IDInput{ID: 1})
	if err != nil {
		t.Fatalf("GetStatus() error: %v", err)
	}
	if out.GeoNodeID != 1 {
		t.Errorf("expected geo_node_id 1, got %d", out.GeoNodeID)
	}
	if out.HealthStatus != "Healthy" {
		t.Errorf("expected health_status Healthy, got %s", out.HealthStatus)
	}
	if out.ProjectsCount != 42 {
		t.Errorf("expected projects_count 42, got %d", out.ProjectsCount)
	}
}

// TestGetStatus_MissingID verifies that GetStatus returns a validation
// error when ID is zero.
func TestGetStatus_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := GetStatus(context.Background(), client, IDInput{})
	if err == nil {
		t.Fatal("expected error for missing id")
	}
}

// TestGetStatus_APIError verifies that GetStatus returns an error when
// the Geo status endpoint responds with 404 Not Found.
func TestGetStatus_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	_, err := GetStatus(context.Background(), client, IDInput{ID: 999})
	if err == nil {
		t.Fatal("expected error on API failure")
	}
}

// TestCreate_CancelledContext verifies that Create returns an error when
// invoked with an already-cancelled context.
func TestCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Create(ctx, client, CreateInput{})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// TestList_CancelledContext verifies that List returns an error when
// invoked with an already-cancelled context.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := List(ctx, client, ListInput{})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// ---------------------------------------------------------------------------
// Context cancellation — Get, Edit, Delete, Repair, ListStatus, GetStatus
// ---------------------------------------------------------------------------

// TestGet_CancelledContext verifies Get returns an error when the context
// is cancelled before the API call.
func TestGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Get(ctx, client, IDInput{ID: 1})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// TestEdit_CancelledContext verifies Edit returns an error when the context
// is cancelled before the API call.
func TestEdit_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Edit(ctx, client, EditInput{ID: 1})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// TestDelete_CancelledContext verifies Delete returns an error when the context
// is cancelled before the API call.
func TestDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	err := Delete(ctx, client, IDInput{ID: 1})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// TestRepair_CancelledContext verifies Repair returns an error when the context
// is cancelled before the API call.
func TestRepair_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Repair(ctx, client, IDInput{ID: 1})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// TestListStatus_CancelledContext verifies ListStatus returns an error when the
// context is cancelled before the API call.
func TestListStatus_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := ListStatus(ctx, client, ListStatusInput{})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// TestGetStatus_CancelledContext verifies GetStatus returns an error when the
// context is cancelled before the API call.
func TestGetStatus_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := GetStatus(ctx, client, IDInput{ID: 1})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// ---------------------------------------------------------------------------
// Pagination — List and ListStatus with pagination headers
// ---------------------------------------------------------------------------

// TestList_WithPagination verifies that List correctly parses pagination headers
// from the GitLab API response into the output pagination metadata.
func TestList_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/geo_sites" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+geoSiteJSON+`]`, testutil.PaginationHeaders{
				Page:       "1",
				PerPage:    "20",
				Total:      "50",
				TotalPages: "3",
				NextPage:   "2",
			})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(out.Sites) != 1 {
		t.Fatalf("expected 1 site, got %d", len(out.Sites))
	}
	if out.Pagination.Page != 1 {
		t.Errorf("expected page 1, got %d", out.Pagination.Page)
	}
	if out.Pagination.TotalPages != 3 {
		t.Errorf("expected total_pages 3, got %d", out.Pagination.TotalPages)
	}
	if out.Pagination.TotalItems != 50 {
		t.Errorf("expected total 50, got %d", out.Pagination.TotalItems)
	}
	if out.Pagination.NextPage != 2 {
		t.Errorf("expected next_page 2, got %d", out.Pagination.NextPage)
	}
}

// TestListStatus_WithPagination verifies that ListStatus correctly parses pagination
// headers from the GitLab API response.
func TestListStatus_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/geo_sites/status" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+geoSiteStatusJSON+`]`, testutil.PaginationHeaders{
				Page:       "2",
				PerPage:    "10",
				Total:      "15",
				TotalPages: "2",
				PrevPage:   "1",
			})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListStatus(context.Background(), client, ListStatusInput{})
	if err != nil {
		t.Fatalf("ListStatus() error: %v", err)
	}
	if len(out.Statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(out.Statuses))
	}
	if out.Pagination.Page != 2 {
		t.Errorf("expected page 2, got %d", out.Pagination.Page)
	}
	if out.Pagination.TotalPages != 2 {
		t.Errorf("expected total_pages 2, got %d", out.Pagination.TotalPages)
	}
}

// ---------------------------------------------------------------------------
// FormatOutputMarkdown — all fields, minimal fields
// ---------------------------------------------------------------------------

// TestFormatOutputMarkdown_AllFields verifies the Markdown output includes all
// populated fields including optional InternalURL, SelectiveSyncType, and WebEditURL.
func TestFormatOutputMarkdown_AllFields(t *testing.T) {
	out := Output{
		ID:                               1,
		Name:                             "primary-site",
		URL:                              "https://primary.example.com",
		InternalURL:                      "https://primary.internal",
		Primary:                          true,
		Enabled:                          true,
		Current:                          true,
		FilesMaxCapacity:                 10,
		ReposMaxCapacity:                 25,
		VerificationMaxCapacity:          100,
		ContainerRepositoriesMaxCapacity: 10,
		SyncObjectStorage:                false,
		SelectiveSyncType:                "namespaces",
		WebEditURL:                       "https://primary.example.com/admin/geo/sites/1/edit",
	}
	md := FormatOutputMarkdown(out)

	checks := []string{
		"## Geo Site: primary-site",
		"| ID | 1 |",
		"| Name | primary-site |",
		"| URL | https://primary.example.com |",
		"| Internal URL | https://primary.internal |",
		"| Primary | true |",
		"| Enabled | true |",
		"| Current | true |",
		"| Files Max Capacity | 10 |",
		"| Repos Max Capacity | 25 |",
		"| Verification Max Capacity | 100 |",
		"| Sync Object Storage | false |",
		"| Selective Sync Type | namespaces |",
		"| Web Edit URL | [Edit](https://primary.example.com/admin/geo/sites/1/edit) |",
	}
	for _, c := range checks {
		if !strings.Contains(md, c) {
			t.Errorf("expected markdown to contain %q:\n%s", c, md)
		}
	}
}

// TestFormatOutputMarkdown_MinimalFields verifies that the Markdown output
// omits optional fields (InternalURL, SelectiveSyncType, WebEditURL) when empty.
func TestFormatOutputMarkdown_MinimalFields(t *testing.T) {
	out := Output{
		ID:      2,
		Name:    "secondary",
		URL:     "https://secondary.example.com",
		Primary: false,
		Enabled: true,
	}
	md := FormatOutputMarkdown(out)

	if !strings.Contains(md, "## Geo Site: secondary") {
		t.Errorf("expected heading:\n%s", md)
	}
	if !strings.Contains(md, "| ID | 2 |") {
		t.Errorf("expected ID row:\n%s", md)
	}
	if strings.Contains(md, "Internal URL") {
		t.Error("should not contain Internal URL when empty")
	}
	if strings.Contains(md, "Selective Sync Type") {
		t.Error("should not contain Selective Sync Type when empty")
	}
	if strings.Contains(md, "Web Edit URL") {
		t.Error("should not contain Web Edit URL when empty")
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown — with items, empty, with pagination
// ---------------------------------------------------------------------------

// TestFormatListMarkdown_WithItems verifies the list Markdown output contains
// a table row for each site and the HintPreserveLinks header.
func TestFormatListMarkdown_WithItems(t *testing.T) {
	out := ListOutput{
		Sites: []Output{
			{ID: 1, Name: "primary", URL: "https://primary.example.com", Primary: true, Enabled: true},
			{ID: 2, Name: "secondary", URL: "https://secondary.example.com", Primary: false, Enabled: false},
		},
	}
	md := FormatListMarkdown(out)

	checks := []string{
		"## Geo Sites",
		"| ID | Name | URL | Primary | Enabled |",
		"| 1 | primary | https://primary.example.com | true | true |",
		"| 2 | secondary | https://secondary.example.com | false | false |",
	}
	for _, c := range checks {
		if !strings.Contains(md, c) {
			t.Errorf("expected markdown to contain %q:\n%s", c, md)
		}
	}
}

// TestFormatListMarkdown_Empty verifies the list output for an empty site list
// contains the heading but no data rows.
func TestFormatListMarkdown_Empty(t *testing.T) {
	out := ListOutput{Sites: []Output{}}
	md := FormatListMarkdown(out)

	if !strings.Contains(md, "## Geo Sites") {
		t.Errorf("expected heading:\n%s", md)
	}
	if strings.Contains(md, "| 1 |") {
		t.Error("should not contain data rows for empty list")
	}
}

// TestFormatListMarkdown_WithPagination verifies the pagination footer appears
// when pagination page is set.
func TestFormatListMarkdown_WithPagination(t *testing.T) {
	out := ListOutput{
		Sites: []Output{
			{ID: 1, Name: "primary", URL: "https://primary.example.com", Primary: true, Enabled: true},
		},
		Pagination: toolutil.PaginationOutput{Page: 1},
	}
	md := FormatListMarkdown(out)

	if !strings.Contains(md, "_Page 1, 1 sites shown._") {
		t.Errorf("expected pagination footer:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// FormatStatusMarkdown — all fields, minimal fields
// ---------------------------------------------------------------------------

// TestFormatStatusMarkdown_AllFields verifies that all status fields including
// optional Health and UpdatedAt are present in the Markdown output.
func TestFormatStatusMarkdown_AllFields(t *testing.T) {
	out := StatusOutput{
		GeoNodeID:                      1,
		Healthy:                        true,
		Health:                         "Healthy",
		HealthStatus:                   "Healthy",
		MissingOAuthApplication:        false,
		DBReplicationLagSeconds:        5,
		ProjectsCount:                  42,
		LFSObjectsSyncedInPercentage:   "100.00%",
		JobArtifactsSyncedInPercentage: "99.50%",
		UploadsSyncedInPercentage:      "98.00%",
		Version:                        "16.5.0",
		Revision:                       "abc123",
		StorageShardsMatch:             true,
	}
	// Set UpdatedAt to exercise the non-zero branch
	out.UpdatedAt = out.UpdatedAt.AddDate(2026, 0, 15)

	md := FormatStatusMarkdown(out)

	checks := []string{
		"## Geo Site Status (Node ID: 1)",
		"| Healthy | true |",
		"| Health Status | Healthy |",
		"| Health | Healthy |",
		"| DB Replication Lag | 5s |",
		"| Missing OAuth App | false |",
		"| Projects Count | 42 |",
		"| LFS Synced | 100.00% |",
		"| Job Artifacts Synced | 99.50% |",
		"| Uploads Synced | 98.00% |",
		"| Version | 16.5.0 |",
		"| Revision | abc123 |",
		"| Storage Shards Match | true |",
		"| Updated At |",
	}
	for _, c := range checks {
		if !strings.Contains(md, c) {
			t.Errorf("expected markdown to contain %q:\n%s", c, md)
		}
	}
}

// TestFormatStatusMarkdown_MinimalFields verifies that optional fields (Health,
// UpdatedAt) are omitted when empty or zero.
func TestFormatStatusMarkdown_MinimalFields(t *testing.T) {
	out := StatusOutput{
		GeoNodeID:    2,
		Healthy:      false,
		HealthStatus: "Unhealthy",
	}
	md := FormatStatusMarkdown(out)

	if !strings.Contains(md, "## Geo Site Status (Node ID: 2)") {
		t.Errorf("expected heading:\n%s", md)
	}
	if !strings.Contains(md, "| Healthy | false |") {
		t.Errorf("expected healthy row:\n%s", md)
	}
	if strings.Contains(md, "| Health |") {
		t.Error("should not contain Health row when empty")
	}
	if strings.Contains(md, "Updated At") {
		t.Error("should not contain Updated At when zero")
	}
}

// ---------------------------------------------------------------------------
// FormatListStatusMarkdown — with items, empty, with pagination
// ---------------------------------------------------------------------------

// TestFormatListStatusMarkdown_WithItems verifies the list status Markdown output
// contains a table row for each status.
func TestFormatListStatusMarkdown_WithItems(t *testing.T) {
	out := ListStatusOutput{
		Statuses: []StatusOutput{
			{GeoNodeID: 1, Healthy: true, HealthStatus: "Healthy", DBReplicationLagSeconds: 0, ProjectsCount: 42, Version: "16.5.0"},
			{GeoNodeID: 2, Healthy: false, HealthStatus: "Unhealthy", DBReplicationLagSeconds: 120, ProjectsCount: 30, Version: "16.4.0"},
		},
	}
	md := FormatListStatusMarkdown(out)

	checks := []string{
		"## Geo Site Statuses",
		"| Node ID | Healthy | Health Status | DB Lag (s) | Projects | Version |",
		"| 1 | true | Healthy | 0 | 42 | 16.5.0 |",
		"| 2 | false | Unhealthy | 120 | 30 | 16.4.0 |",
	}
	for _, c := range checks {
		if !strings.Contains(md, c) {
			t.Errorf("expected markdown to contain %q:\n%s", c, md)
		}
	}
}

// TestFormatListStatusMarkdown_Empty verifies the output for an empty status list.
func TestFormatListStatusMarkdown_Empty(t *testing.T) {
	out := ListStatusOutput{Statuses: []StatusOutput{}}
	md := FormatListStatusMarkdown(out)

	if !strings.Contains(md, "## Geo Site Statuses") {
		t.Errorf("expected heading:\n%s", md)
	}
	if strings.Contains(md, "| 1 |") {
		t.Error("should not contain data rows for empty list")
	}
}

// TestFormatListStatusMarkdown_WithPagination verifies the pagination footer
// appears when pagination page is set.
func TestFormatListStatusMarkdown_WithPagination(t *testing.T) {
	out := ListStatusOutput{
		Statuses: []StatusOutput{
			{GeoNodeID: 1, Healthy: true, HealthStatus: "Healthy", Version: "16.5.0"},
		},
		Pagination: toolutil.PaginationOutput{Page: 2},
	}
	md := FormatListStatusMarkdown(out)

	if !strings.Contains(md, "_Page 2, 1 statuses shown._") {
		t.Errorf("expected pagination footer:\n%s", md)
	}
}
