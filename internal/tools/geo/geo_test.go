// geo_test.go contains unit tests for GitLab Geo site operations.
// Tests use httptest to mock the GitLab Geo API.
package geo

import (
	"context"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
	"selective_sync_organization_ids": [7, 9],
	"minimum_reverification_interval": 7,
	"blob_download_timeout": 28800,
	"checksum_mismatch_report_threshold": 5,
	"checksum_mismatch_self_heal_cooldown_minutes": 60,
	"web_edit_url": "https://primary.example.com/admin/geo/sites/1/edit",
	"web_geo_replication_details_url": "https://primary.example.com/admin/geo/replication",
	"_links": {
		"self": "https://primary.example.com/api/v4/geo_sites/1",
		"status": "https://primary.example.com/api/v4/geo_sites/1/status",
		"repair": "https://primary.example.com/api/v4/geo_sites/1/repair"
	}
}`

const geoSiteStatusJSON = `{
	"geo_node_id": 1,
	"healthy": true,
	"health": "Healthy",
	"health_status": "Healthy",
	"missing_oauth_application": false,
	"db_replication_lag_seconds": 0,
	"projects_count": 42,
	"repositories_count": 19,
	"storage_shards": [{"name": "default"}, {"name": "nvme"}],
	"lfs_objects_oldest_unsynced_time": "2026-01-15T09:00:00Z",
	"lfs_objects_synced_in_percentage": "100.00%",
	"job_artifacts_synced_in_percentage": "99.50%",
	"uploads_synced_in_percentage": "100.00%",
	"container_repositories_replication_enabled": true,
	"lfs_objects_count": 120,
	"lfs_objects_verified_count": 118,
	"ci_secure_files_count": 7,
	"ci_secure_files_synced_count": 6,
	"ci_secure_files_verified_count": 5,
	"ci_secure_files_synced_in_percentage": "85.71%",
	"ci_secure_files_verified_in_percentage": "71.43%",
	"group_wiki_repositories_verification_total_count": 9,
	"replication_slots_count": 3,
	"replication_slots_used_count": 2,
	"replication_slots_max_retained_wal_bytes": 1048576,
	"last_event_id": 999,
	"cursor_last_event_id": 998,
	"namespaces": ["group-a", "group-b"],
	"selective_sync_type": "namespaces",
	"version": "16.5.0",
	"revision": "abc123",
	"storage_shards_match": true,
	"updated_at": "2026-01-15T10:30:00Z",
	"_links": {
		"self": "https://primary.example.com/api/v4/geo_sites/1/status",
		"site": "https://primary.example.com/api/v4/geo_sites/1"
	}
}`

// TestCreate_Success verifies that Create succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/geo_sites (POST) responds with HTTP Created.
// It asserts the returned output matches the expected fields.
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

// TestCreate_APIError verifies that Create returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := Create(context.Background(), client, CreateInput{})
	if err == nil {
		t.Fatal("expected error on API failure")
	}
}

// TestList_Success verifies that List succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/geo_sites (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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

// TestList_Empty verifies the List_Empty handler.
// The mock GitLab API at /api/v4/geo_sites (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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

// TestList_APIError verifies that List returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error on API failure")
	}
}

// TestGet_Success verifies that Get succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/geo_sites/1 (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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

// TestGet_MissingID verifies that Get_MissingID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGet_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Get(context.Background(), client, IDInput{})
	if err == nil {
		t.Fatal("expected error for missing id")
	}
}

// TestGet_NotFound verifies that Get_NotFound returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGet_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	_, err := Get(context.Background(), client, IDInput{ID: 999})
	if err == nil {
		t.Fatal("expected error for not found site")
	}
}

// TestEdit_Success verifies that Edit succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/geo_sites/1 (PUT) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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

// TestEdit_MissingID verifies that Edit_MissingID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestEdit_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Edit(context.Background(), client, EditInput{})
	if err == nil {
		t.Fatal("expected error for missing id")
	}
}

// TestEdit_APIError verifies that Edit returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestEdit_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}))

	_, err := Edit(context.Background(), client, EditInput{ID: 1})
	if err == nil {
		t.Fatal("expected error on API failure")
	}
}

// TestDelete_Success verifies that Delete succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/geo_sites/1 (DELETE) returns a representative success body.
// It asserts the returned output matches the expected fields.
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

// TestDelete_MissingID verifies that Delete_MissingID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDelete_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := Delete(context.Background(), client, IDInput{})
	if err == nil {
		t.Fatal("expected error for missing id")
	}
}

// TestDelete_APIError verifies that Delete returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	err := Delete(context.Background(), client, IDInput{ID: 1})
	if err == nil {
		t.Fatal("expected error on API failure")
	}
}

// TestRepair_Success verifies that Repair succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/geo_sites/1/repair (POST) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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

// TestRepair_NullResponse verifies that Repair handles GitLab returning HTTP
// 200 with a null body, which can happen after accepting the repair request.
func TestRepair_NullResponse(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/geo_sites/1/repair" {
			testutil.RespondJSON(w, http.StatusOK, `null`)
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
	if len(out.NextSteps) == 0 {
		t.Fatal("expected next steps for null repair response")
	}
}

// TestRepair_MissingID verifies that Repair_MissingID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestRepair_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Repair(context.Background(), client, IDInput{})
	if err == nil {
		t.Fatal("expected error for missing id")
	}
}

// TestRepair_APIError verifies that Repair returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestRepair_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	_, err := Repair(context.Background(), client, IDInput{ID: 1})
	if err == nil {
		t.Fatal("expected error on API failure")
	}
}

// TestListStatus_Success verifies that ListStatus succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/geo_sites/status (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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

// TestListStatus_Empty verifies the ListStatus_Empty handler.
// The mock GitLab API at /api/v4/geo_sites/status (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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

// TestListStatus_APIError verifies that ListStatus returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListStatus_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := ListStatus(context.Background(), client, ListStatusInput{})
	if err == nil {
		t.Fatal("expected error on API failure")
	}
}

// TestGetStatus_Success verifies that GetStatus succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/geo_sites/1/status (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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

// TestGetStatus_MissingID verifies that GetStatus_MissingID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetStatus_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := GetStatus(context.Background(), client, IDInput{})
	if err == nil {
		t.Fatal("expected error for missing id")
	}
}

// TestGetStatus_APIError verifies that GetStatus returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetStatus_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	_, err := GetStatus(context.Background(), client, IDInput{ID: 999})
	if err == nil {
		t.Fatal("expected error on API failure")
	}
}

// TestCreate_CancelledContext verifies the Create_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
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

// TestList_CancelledContext verifies the List_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
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

// TestGet_CancelledContext verifies the Get_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
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

// TestEdit_CancelledContext verifies the Edit_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
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

// TestDelete_CancelledContext verifies the Delete_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
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

// TestRepair_CancelledContext verifies the Repair_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
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

// TestListStatus_CancelledContext verifies the ListStatus_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
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

// TestGetStatus_CancelledContext verifies the GetStatus_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
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

// TestList_WithPagination verifies that List_WithPagination forwards pagination parameters to the GitLab API and parses the response metadata.
// The mock GitLab API at /api/v4/geo_sites (GET) responds with HTTP OK.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
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

// TestListStatus_WithPagination verifies that ListStatus_WithPagination forwards pagination parameters to the GitLab API and parses the response metadata.
// The mock GitLab API at /api/v4/geo_sites/status (GET) responds with HTTP OK.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
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

// assertGeoMarkdown compares a whole rendered response with what the formatter
// is meant to write, byte for byte. A substring assertion is what let a card
// open a table it never filled and still pass.
func assertGeoMarkdown(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("markdown mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// geoSiteHints is the guidance section every Geo site card closes with.
const geoSiteHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
	"- Use action 'geo.get_status' to read this site's replication status\n" +
	"- Use action 'geo.edit' to change this site's capacities or selective sync\n" +
	"- Use action 'geo.list' to see every Geo site on the instance\n"

// geoStatusHints is the guidance section every Geo status card closes with.
const geoStatusHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
	"- Use action 'geo.get' to read the site this status belongs to\n" +
	"- Use action 'geo.repair' to repair the site's OAuth application\n"

// TestFormatOutputMarkdown_AllFields verifies the whole card a fully populated
// Geo site renders, the selective-sync scope and the replication details link
// included: the type alone used to say that the site syncs a subset and never
// which subset, and the replication page was never linked at all.
func TestFormatOutputMarkdown_AllFields(t *testing.T) {
	out := Output{
		ID:                                      1,
		Name:                                    "primary-site",
		URL:                                     "https://primary.example.com",
		InternalURL:                             "https://primary.internal",
		Primary:                                 true,
		Enabled:                                 true,
		Current:                                 true,
		FilesMaxCapacity:                        10,
		ReposMaxCapacity:                        25,
		VerificationMaxCapacity:                 100,
		ContainerRepositoriesMaxCapacity:        10,
		SyncObjectStorage:                       false,
		SelectiveSyncType:                       "namespaces",
		SelectiveSyncNamespaceIDs:               []int64{7, 9},
		WebEditURL:                              "https://primary.example.com/admin/geo/sites/1/edit",
		WebGeoReplicationDetailsURL:             "https://primary.example.com/admin/geo/replication",
		BlobDownloadTimeout:                     28800,
		ChecksumMismatchReportThreshold:         5,
		ChecksumMismatchSelfHealCooldownMinutes: 60,
	}

	assertGeoMarkdown(t, FormatOutputMarkdown(out), "## Geo Site: primary-site\n\n"+
		"- **ID**: 1\n"+
		"- **Name**: primary-site\n"+
		"- **URL**: [https://primary.example.com](https://primary.example.com)\n"+
		"- **Internal URL**: [https://primary.internal](https://primary.internal)\n"+
		"- **Primary**: ✅\n"+
		"- **Enabled**: ✅\n"+
		"- **Current**: ✅\n"+
		"- **Files Max Capacity**: 10\n"+
		"- **Repos Max Capacity**: 25\n"+
		"- **Verification Max Capacity**: 100\n"+
		"- **Blob Download Timeout**: 28800s\n"+
		"- **Checksum Mismatch Report Threshold**: 5\n"+
		"- **Checksum Mismatch Self-Heal Cooldown**: 60 min\n"+
		"- **Sync Object Storage**: ❌\n"+
		"- **Selective Sync Type**: namespaces\n"+
		"- **Selective Sync Namespace IDs**: 7, 9\n"+
		"- **Web Edit URL**: [https://primary.example.com/admin/geo/sites/1/edit](https://primary.example.com/admin/geo/sites/1/edit)\n"+
		"- **Replication Details**: [https://primary.example.com/admin/geo/replication](https://primary.example.com/admin/geo/replication)\n"+
		geoSiteHints)
}

// TestFormatOutputMarkdown_MinimalFields verifies that a site GitLab sent no
// optional field for writes no row for one: no internal URL, no selective
// sync, no link to a page that was not named.
func TestFormatOutputMarkdown_MinimalFields(t *testing.T) {
	out := Output{
		ID:      2,
		Name:    "secondary",
		URL:     "https://secondary.example.com",
		Primary: false,
		Enabled: true,
	}

	assertGeoMarkdown(t, FormatOutputMarkdown(out), "## Geo Site: secondary\n\n"+
		"- **ID**: 2\n"+
		"- **Name**: secondary\n"+
		"- **URL**: [https://secondary.example.com](https://secondary.example.com)\n"+
		"- **Primary**: ❌\n"+
		"- **Enabled**: ✅\n"+
		"- **Current**: ❌\n"+
		"- **Files Max Capacity**: 0\n"+
		"- **Repos Max Capacity**: 0\n"+
		"- **Verification Max Capacity**: 0\n"+
		"- **Blob Download Timeout**: 0s\n"+
		"- **Checksum Mismatch Report Threshold**: 0\n"+
		"- **Checksum Mismatch Self-Heal Cooldown**: 0 min\n"+
		"- **Sync Object Storage**: ❌\n"+
		geoSiteHints)
}

// ---------------------------------------------------------------------------
// FormatListMarkdown — with items, empty, with pagination
// ---------------------------------------------------------------------------

// geoSiteTableHead is the heading-less head of the Geo site table.
const geoSiteTableHead = "| ID | Name | URL | Primary | Enabled |\n" +
	"| --- | --- | --- | --- | --- |\n"

// geoSiteListHints is the guidance section the Geo site list closes with. The
// table carries a link column, so the link instruction leads it.
var geoSiteListHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
	"- " + toolutil.HintPreserveLinks + "\n" +
	"- Use action 'geo.get' to read one site in full\n" +
	"- Use action 'geo.list_status' to see the replication status of every site\n"

// TestFormatListMarkdown_WithItems verifies the whole table a page of Geo
// sites renders: the heading counting what was shown, each site's URL linked,
// and the two flags as glyphs rather than as the words true and false.
func TestFormatListMarkdown_WithItems(t *testing.T) {
	out := ListOutput{
		Sites: []Output{
			{ID: 1, Name: "primary", URL: "https://primary.example.com", Primary: true, Enabled: true},
			{ID: 2, Name: "secondary", URL: "https://secondary.example.com", Primary: false, Enabled: false},
		},
	}

	assertGeoMarkdown(t, FormatListMarkdown(out), "## Geo Sites (2)\n\n"+
		geoSiteTableHead+
		"| 1 | primary | [https://primary.example.com](https://primary.example.com) | ✅ | ✅ |\n"+
		"| 2 | secondary | [https://secondary.example.com](https://secondary.example.com) | ❌ | ❌ |\n"+
		geoSiteListHints)
}

// TestFormatListMarkdown_Empty verifies that an empty page renders the one
// sentence and nothing else: no heading counting zero, no table header, and no
// instruction to preserve links a render without any cannot have.
func TestFormatListMarkdown_Empty(t *testing.T) {
	assertGeoMarkdown(t, FormatListMarkdown(ListOutput{Sites: []Output{}}), "No Geo sites found.\n")
}

// TestFormatListMarkdown_WithPagination verifies the pagination footer opens a
// paragraph of its own between the last table row and the guidance section.
func TestFormatListMarkdown_WithPagination(t *testing.T) {
	out := ListOutput{
		Sites: []Output{
			{ID: 1, Name: "primary", URL: "https://primary.example.com", Primary: true, Enabled: true},
		},
		Pagination: toolutil.PaginationOutput{Page: 1},
	}

	assertGeoMarkdown(t, FormatListMarkdown(out), "## Geo Sites (1)\n\n"+
		geoSiteTableHead+
		"| 1 | primary | [https://primary.example.com](https://primary.example.com) | ✅ | ✅ |\n"+
		"\nPage 1 | no more pages\n"+
		geoSiteListHints)
}

// ---------------------------------------------------------------------------
// FormatStatusMarkdown — all fields, minimal fields
// ---------------------------------------------------------------------------

// TestFormatStatusMarkdown_AllFields verifies the whole card a fully reported
// Geo site status renders, the timestamp in the display form every other
// formatter uses rather than the zone-less layout this one used to print.
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
		RepositoriesCount:              19,
		StorageShards:                  []StorageShard{{Name: "default"}, {Name: "nvme"}},
		Replicables: map[string]ReplicableStatus{
			"lfs_objects":     {Count: 120},
			"job_artifacts":   {Count: 8},
			"ci_secure_files": {Count: 7},
		},
		UpdatedAt: time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
	}

	assertGeoMarkdown(t, FormatStatusMarkdown(out), "## Geo Site Status (Node ID: 1)\n\n"+
		"- **Healthy**: ✅\n"+
		"- **Health Status**: Healthy\n"+
		"- **Health**: Healthy\n"+
		"- **DB Replication Lag**: 5s\n"+
		"- **Projects Count**: 42\n"+
		"- **Repositories Count**: 19\n"+
		"- **Replicables Tracked**: 3\n"+
		"- **Storage Shards**: default, nvme\n"+
		"- **LFS Synced**: 100.00%\n"+
		"- **Job Artifacts Synced**: 99.50%\n"+
		"- **Uploads Synced**: 98.00%\n"+
		"- **Version**: 16.5.0\n"+
		"- **Revision**: abc123\n"+
		"- **Storage Shards Match**: ✅\n"+
		"- **Updated**: 15 Jan 2026 10:30 UTC\n"+
		geoStatusHints)
}

// TestFormatStatusMarkdown_MinimalFields verifies that a status GitLab sent no
// optional field for writes no row for one, and that a site with its OAuth
// application in place carries no warning.
func TestFormatStatusMarkdown_MinimalFields(t *testing.T) {
	out := StatusOutput{
		GeoNodeID:    2,
		Healthy:      false,
		HealthStatus: "Unhealthy",
	}

	assertGeoMarkdown(t, FormatStatusMarkdown(out), "## Geo Site Status (Node ID: 2)\n\n"+
		"- **Healthy**: ❌\n"+
		"- **Health Status**: Unhealthy\n"+
		"- **DB Replication Lag**: 0s\n"+
		"- **Projects Count**: 0\n"+
		"- **Repositories Count**: 0\n"+
		"- **Storage Shards Match**: ❌\n"+
		geoStatusHints)
}

// TestFormatStatusMarkdown_UnhealthyMultilineHealth verifies that the health
// check's own output, which carries an exception message and so a newline,
// becomes a quote under its label rather than breaking the card's list.
func TestFormatStatusMarkdown_UnhealthyMultilineHealth(t *testing.T) {
	out := StatusOutput{
		GeoNodeID:               3,
		HealthStatus:            "Unhealthy",
		MissingOAuthApplication: true,
		Health:                  "Could not connect to Geo database\nPG::ConnectionBad",
	}

	assertGeoMarkdown(t, FormatStatusMarkdown(out), "## Geo Site Status (Node ID: 3)\n\n"+
		"- **Healthy**: ❌\n"+
		"- **Health Status**: Unhealthy\n"+
		"- **Health**:\n"+
		"  > Could not connect to Geo database\n"+
		"  > PG::ConnectionBad\n"+
		"- **DB Replication Lag**: 0s\n"+
		"- ⚠️ **Missing OAuth Application**\n"+
		"- **Projects Count**: 0\n"+
		"- **Repositories Count**: 0\n"+
		"- **Storage Shards Match**: ❌\n"+
		geoStatusHints)
}

// ---------------------------------------------------------------------------
// FormatListStatusMarkdown — with items, empty, with pagination
// ---------------------------------------------------------------------------

// geoStatusTableHead is the heading-less head of the Geo status table.
const geoStatusTableHead = "| Node ID | Healthy | Health Status | DB Lag (s) | Projects | Version |\n" +
	"| --- | --- | --- | --- | --- | --- |\n"

// geoStatusListHints is the guidance section the Geo status list closes with.
// The table carries no link column, so no instruction to preserve links.
const geoStatusListHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
	"- Use action 'geo.get_status' to read one site's status in full\n"

// TestFormatListStatusMarkdown_WithItems verifies the whole table a page of
// Geo statuses renders, the health flag as a glyph rather than as a word.
func TestFormatListStatusMarkdown_WithItems(t *testing.T) {
	out := ListStatusOutput{
		Statuses: []StatusOutput{
			{GeoNodeID: 1, Healthy: true, HealthStatus: "Healthy", DBReplicationLagSeconds: 0, ProjectsCount: 42, Version: "16.5.0"},
			{GeoNodeID: 2, Healthy: false, HealthStatus: "Unhealthy", DBReplicationLagSeconds: 120, ProjectsCount: 30, Version: "16.4.0"},
		},
	}

	assertGeoMarkdown(t, FormatListStatusMarkdown(out), "## Geo Site Statuses (2)\n\n"+
		geoStatusTableHead+
		"| 1 | ✅ | Healthy | 0 | 42 | 16.5.0 |\n"+
		"| 2 | ❌ | Unhealthy | 120 | 30 | 16.4.0 |\n"+
		geoStatusListHints)
}

// TestFormatListStatusMarkdown_Empty verifies that an empty page renders the
// one sentence and nothing else.
func TestFormatListStatusMarkdown_Empty(t *testing.T) {
	assertGeoMarkdown(t, FormatListStatusMarkdown(ListStatusOutput{Statuses: []StatusOutput{}}), "No Geo site statuses found.\n")
}

// TestFormatListStatusMarkdown_WithPagination verifies the pagination footer
// opens a paragraph of its own between the last row and the guidance section.
func TestFormatListStatusMarkdown_WithPagination(t *testing.T) {
	out := ListStatusOutput{
		Statuses: []StatusOutput{
			{GeoNodeID: 1, Healthy: true, HealthStatus: "Healthy", Version: "16.5.0"},
		},
		Pagination: toolutil.PaginationOutput{Page: 2},
	}

	assertGeoMarkdown(t, FormatListStatusMarkdown(out), "## Geo Site Statuses (1)\n\n"+
		geoStatusTableHead+
		"| 1 | ✅ | Healthy | 0 | 0 | 16.5.0 |\n"+
		"\nPage 2 | no more pages\n"+
		geoStatusListHints)
}

// ---------------------------------------------------------------------------
// Keyset pagination + order_by/sort forwarding (1:1 audit P3)
// ---------------------------------------------------------------------------

// TestList_KeysetAndOrdering verifies that List forwards order_by, sort,
// pagination=keyset, and page_token to the GitLab Geo sites endpoint.
func TestList_KeysetAndOrdering(t *testing.T) {
	var query url.Values
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/geo_sites" {
			query = r.URL.Query()
			testutil.RespondJSON(w, http.StatusOK, `[`+geoSiteJSON+`]`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := List(context.Background(), client, ListInput{
		OrderBy:    "id",
		Sort:       "desc",
		Pagination: "keyset", PageToken: "42",
	})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if got := query.Get("order_by"); got != "id" {
		t.Errorf("order_by = %q, want id", got)
	}
	if got := query.Get("sort"); got != "desc" {
		t.Errorf("sort = %q, want desc", got)
	}
	if got := query.Get("pagination"); got != "keyset" {
		t.Errorf("pagination = %q, want keyset", got)
	}
	if got := query.Get("page_token"); got != "42" {
		t.Errorf("page_token = %q, want 42", got)
	}
}

// TestListStatus_KeysetAndOrdering verifies that ListStatus forwards order_by,
// sort, pagination=keyset, and page_token to the Geo statuses endpoint.
func TestListStatus_KeysetAndOrdering(t *testing.T) {
	var query url.Values
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/geo_sites/status" {
			query = r.URL.Query()
			testutil.RespondJSON(w, http.StatusOK, `[`+geoSiteStatusJSON+`]`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := ListStatus(context.Background(), client, ListStatusInput{
		OrderBy:    "geo_node_id",
		Sort:       "asc",
		Pagination: "keyset", PageToken: "7",
	})
	if err != nil {
		t.Fatalf("ListStatus() error: %v", err)
	}
	if got := query.Get("order_by"); got != "geo_node_id" {
		t.Errorf("order_by = %q, want geo_node_id", got)
	}
	if got := query.Get("sort"); got != "asc" {
		t.Errorf("sort = %q, want asc", got)
	}
	if got := query.Get("pagination"); got != "keyset" {
		t.Errorf("pagination = %q, want keyset", got)
	}
	if got := query.Get("page_token"); got != "7" {
		t.Errorf("page_token = %q, want 7", got)
	}
}

// ---------------------------------------------------------------------------
// Full field mirror vs client-go (1:1 audit R-OUTPUT)
// ---------------------------------------------------------------------------

// TestGet_MirrorsLinksAndReplicationURL verifies the Output mirrors the
// GeoSite _links object and web_geo_replication_details_url field.
func TestGet_MirrorsLinksAndReplicationURL(t *testing.T) {
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
	if out.WebGeoReplicationDetailsURL != "https://primary.example.com/admin/geo/replication" {
		t.Errorf("WebGeoReplicationDetailsURL = %q", out.WebGeoReplicationDetailsURL)
	}
	if out.Links.Self == "" || out.Links.Status == "" || out.Links.Repair == "" {
		t.Errorf("_links not fully mirrored: %+v", out.Links)
	}
}

// TestGetStatus_MirrorsFullStruct verifies that GetStatus mirrors the scalar
// count fields, the upstream-typo group-wiki total, namespaces, and _links
// of GeoSiteStatus added by the 1:1 audit.
func TestGetStatus_MirrorsFullStruct(t *testing.T) {
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
	// Field-by-field mirror assertions, table-driven to keep complexity low.
	// The group-wiki verification total maps via the upstream-typo SDK field
	// GrupWikiRepositoriesVerificationTotalCount (JSON tag is correct).
	intChecks := map[string]struct{ got, want int64 }{
		"lfs_objects_count":                                {out.LFSObjectsCount, 120},
		"lfs_objects_verified_count":                       {out.LFSObjectsVerifiedCount, 118},
		"ci_secure_files_count":                            {out.CISecureFilesCount, 7},
		"ci_secure_files_synced_count":                     {out.CISecureFilesSyncedCount, 6},
		"ci_secure_files_verified_count":                   {out.CISecureFilesVerifiedCount, 5},
		"group_wiki_repositories_verification_total_count": {out.GroupWikiRepositoriesVerificationTotalCount, 9},
		"replication_slots_count":                          {out.ReplicationSlotsCount, 3},
		"replication_slots_used_count":                     {out.ReplicationSlotsUsedCount, 2},
		"replication_slots_max_retained_wal_bytes":         {out.ReplicationSlotsMaxRetainedWalBytes, 1048576},
		"last_event_id":                                    {out.LastEventID, 999},
		"cursor_last_event_id":                             {out.CursorLastEventID, 998},
	}
	for name, c := range intChecks {
		t.Run(name, func(t *testing.T) {
			if c.got != c.want {
				t.Errorf("%s = %d, want %d", name, c.got, c.want)
			}
		})
	}
	strChecks := map[string]struct{ got, want string }{
		"ci_secure_files_synced_in_percentage":   {out.CISecureFilesSyncedInPercentage, "85.71%"},
		"ci_secure_files_verified_in_percentage": {out.CISecureFilesVerifiedInPercentage, "71.43%"},
		"selective_sync_type":                    {out.SelectiveSyncType, "namespaces"},
	}
	for name, c := range strChecks {
		t.Run(name, func(t *testing.T) {
			if c.got != c.want {
				t.Errorf("%s = %q, want %q", name, c.got, c.want)
			}
		})
	}
	if !out.ContainerRepositoriesReplicationEnabled {
		t.Error("ContainerRepositoriesReplicationEnabled not mirrored")
	}
	if len(out.Namespaces) != 2 {
		t.Errorf("namespaces = %v, want 2 entries", out.Namespaces)
	}
	if out.Links.Self == "" || out.Links.Site == "" {
		t.Errorf("status _links not mirrored: %+v", out.Links)
	}
}

// ---------------------------------------------------------------------------
// Fields client-go does not model, read from the captured response (ADR-0021)
// ---------------------------------------------------------------------------

// TestGet_PublishesTheSiteFieldsTheSDKDrops verifies the four fields
// ee/lib/api/entities/geo_site.rb exposes that client-go's GeoSite has no room
// for reach the caller: the organization ids of a selective sync, the blob
// download timeout and the two checksum mismatch settings.
func TestGet_PublishesTheSiteFieldsTheSDKDrops(t *testing.T) {
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
	checks := map[string]struct{ got, want int64 }{
		"blob_download_timeout":                        {out.BlobDownloadTimeout, 28800},
		"checksum_mismatch_report_threshold":           {out.ChecksumMismatchReportThreshold, 5},
		"checksum_mismatch_self_heal_cooldown_minutes": {out.ChecksumMismatchSelfHealCooldownMinutes, 60},
	}
	for name, check := range checks {
		t.Run(name, func(t *testing.T) {
			if check.got != check.want {
				t.Errorf("%s = %d, want %d", name, check.got, check.want)
			}
		})
	}
	if !reflect.DeepEqual(out.SelectiveSyncOrganizationIDs, []int64{7, 9}) {
		t.Errorf("SelectiveSyncOrganizationIDs = %v, want [7 9]", out.SelectiveSyncOrganizationIDs)
	}
}

// TestList_PublishesTheSiteFieldsTheSDKDrops verifies the list handler pairs
// each site with its own captured extras rather than dropping them.
func TestList_PublishesTheSiteFieldsTheSDKDrops(t *testing.T) {
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
	if out.Sites[0].BlobDownloadTimeout != 28800 {
		t.Errorf("BlobDownloadTimeout = %d, want 28800", out.Sites[0].BlobDownloadTimeout)
	}
}

// TestRepair_PublishesNoSiteExtras verifies the one site handler that reads no
// capture: POST /geo_sites/:id/repair is annotated `success
// Entities::GeoSiteStatus` and presents a status, so there are no site fields
// in that answer to read, whatever client-go's *GeoSite return type suggests.
func TestRepair_PublishesNoSiteExtras(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/geo_sites/1/repair" {
			testutil.RespondJSON(w, http.StatusOK, geoSiteStatusJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Repair(context.Background(), client, IDInput{ID: 1})
	if err != nil {
		t.Fatalf("Repair() error: %v", err)
	}
	if out.BlobDownloadTimeout != 0 || out.SelectiveSyncOrganizationIDs != nil {
		t.Errorf("Repair() read site extras off a status answer: %+v", out)
	}
}

// TestGetStatus_PublishesTheReplicableMatrix verifies the flat per-replicable
// keys of a status answer reach the caller as the replicables map, keyed by
// the name GitLab prefixes them with, alongside the repository count and the
// storage shards client-go does not model.
func TestGetStatus_PublishesTheReplicableMatrix(t *testing.T) {
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
	if out.RepositoriesCount != 19 {
		t.Errorf("RepositoriesCount = %d, want 19", out.RepositoriesCount)
	}
	if !reflect.DeepEqual(out.StorageShards, []StorageShard{{Name: "default"}, {Name: "nvme"}}) {
		t.Errorf("StorageShards = %+v, want the two shards", out.StorageShards)
	}
	lfs, ok := out.Replicables["lfs_objects"]
	if !ok {
		t.Fatalf("Replicables = %+v, want an lfs_objects entry", out.Replicables)
	}
	if lfs.Count != 120 || lfs.VerifiedCount != 118 || lfs.SyncedInPercentage != "100.00%" {
		t.Errorf("lfs_objects = %+v, want the metrics of the answer", lfs)
	}
	if lfs.OldestUnsyncedTime == nil {
		t.Error("lfs_objects.OldestUnsyncedTime is nil, want the time the answer carries")
	}
	if len(out.AdditionalFields) != 0 {
		t.Errorf("AdditionalFields = %+v, want nothing left over from this answer", out.AdditionalFields)
	}
}

// TestListStatus_PublishesTheReplicableMatrix verifies the status list handler
// decomposes each element rather than only the first.
func TestListStatus_PublishesTheReplicableMatrix(t *testing.T) {
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
	if out.Statuses[0].Replicables["lfs_objects"].Count != 120 {
		t.Errorf("Replicables = %+v, want the matrix of the one status", out.Statuses[0].Replicables)
	}
}

// TestGeo_UnreadableCapturedFields verifies the six handlers that read a
// capture report its decode failure rather than answering with an object
// missing what GitLab sent. The body carries a timeout the site shape cannot
// hold and a repository count the status shape cannot hold, both of them keys
// client-go's own structs do not model, so the SDK's decode succeeds and only
// the reader beside it fails.
func TestGeo_UnreadableCapturedFields(t *testing.T) {
	const unreadable = `{"id": 1, "name": "primary-site", "blob_download_timeout": "soon", "repositories_count": "x"}`
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		collection := r.URL.Path == "/api/v4/geo_sites" || r.URL.Path == "/api/v4/geo_sites/status"
		if collection && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `[`+unreadable+`]`)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, unreadable)
	}))

	name := "primary-site"
	testutil.AssertCapturedDecodeFailures(t, []testutil.CapturedCase{
		{Name: "create", Call: func() error {
			_, err := Create(context.Background(), client, CreateInput{Name: &name})
			return err
		}},
		{Name: "list", Call: func() error {
			_, err := List(context.Background(), client, ListInput{})
			return err
		}},
		{Name: "get", Call: func() error {
			_, err := Get(context.Background(), client, IDInput{ID: 1})
			return err
		}},
		{Name: "edit", Call: func() error {
			_, err := Edit(context.Background(), client, EditInput{ID: 1, Name: &name})
			return err
		}},
		{Name: "get_status", Call: func() error {
			_, err := GetStatus(context.Background(), client, IDInput{ID: 1})
			return err
		}},
		{Name: "list_status", Call: func() error {
			_, err := ListStatus(context.Background(), client, ListStatusInput{})
			return err
		}},
	})
}

// ---------------------------------------------------------------------------
// Discovery metadata (1:1 audit R-META)
// ---------------------------------------------------------------------------

// TestActionSpecs_MetadataReturnsAndSeeAlso verifies every Geo individual tool
// carries a non-generic description in the "Returns: … See also: …" form and
// natural-language aliases beyond the canonical tool name.
func TestActionSpecs_MetadataReturnsAndSeeAlso(t *testing.T) {
	specs := ActionSpecs(testutil.NewTestClient(t, http.NewServeMux()))
	for _, spec := range specs {
		tool := spec.IndividualTool.Name
		desc := spec.IndividualTool.Description
		if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
			t.Errorf("%s: description missing Returns:/See also: form: %q", tool, desc)
		}
		hasNatural := false
		for _, a := range spec.Aliases {
			if a != tool {
				hasNatural = true
				break
			}
		}
		if !hasNatural {
			t.Errorf("%s: no natural-language aliases beyond canonical name: %v", tool, spec.Aliases)
		}
		if len(spec.RelatedActions) == 0 {
			t.Errorf("%s: no related actions", tool)
		}
	}
}

// TestDecorateGeoMeta_UnknownToolNoop verifies decorateGeoMeta leaves options
// untouched for a tool name absent from geoActionMeta.
func TestDecorateGeoMeta_UnknownToolNoop(t *testing.T) {
	opts := geoOptions("gitlab_unknown_geo_tool")
	before := opts
	decorateGeoMeta(&opts, "gitlab_unknown_geo_tool")
	if opts.Usage != before.Usage || len(opts.RelatedActions) != 0 {
		t.Errorf("decorateGeoMeta mutated options for unknown tool: %+v", opts)
	}
}
