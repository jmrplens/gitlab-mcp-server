// geo_test.go contains unit tests for GitLab Geo site operations.
// Tests use httptest to mock the GitLab Geo API.
package geo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
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

// TestGeo_StatusHint_AttachesOnlyToTheStatusItDescribes verifies each
// handler's suggestion is attached on the one status it was written for and
// withheld on any other: a get answered 404 says to verify the id, and the
// same get answered 500 says nothing about the id, since that advice would be
// wrong there. The status each hint is keyed on is a literal no mutator
// touches, so this is the only thing holding it.
func TestGeo_StatusHint_AttachesOnlyToTheStatusItDescribes(t *testing.T) {
	name := "primary-site"
	handlers := []struct {
		name   string
		status int
		hint   string
		call   func(client *gitlabclient.Client) error
	}{
		{name: "create", status: http.StatusBadRequest, hint: "only one site may have primary=true", call: func(client *gitlabclient.Client) error {
			_, err := Create(context.Background(), client, CreateInput{Name: &name})
			return err
		}},
		{name: "list", status: http.StatusForbidden, hint: "ensure the instance is configured for Geo", call: func(client *gitlabclient.Client) error {
			_, err := List(context.Background(), client, ListInput{})
			return err
		}},
		{name: "get", status: http.StatusNotFound, hint: "verify id with geo.list; requires admin access", call: func(client *gitlabclient.Client) error {
			_, err := Get(context.Background(), client, IDInput{ID: 1})
			return err
		}},
		{name: "edit", status: http.StatusBadRequest, hint: "cannot toggle primary status (recreate site instead)", call: func(client *gitlabclient.Client) error {
			_, err := Edit(context.Background(), client, EditInput{ID: 1, Name: &name})
			return err
		}},
		{name: "delete", status: http.StatusForbidden, hint: "cannot delete the primary site while secondaries exist", call: func(client *gitlabclient.Client) error {
			return Delete(context.Background(), client, IDInput{ID: 1})
		}},
		{name: "repair", status: http.StatusNotFound, hint: "repair re-creates the OAuth application for the secondary site", call: func(client *gitlabclient.Client) error {
			_, err := Repair(context.Background(), client, IDInput{ID: 1})
			return err
		}},
		{name: "list_status", status: http.StatusForbidden, hint: "status data is collected by the primary site", call: func(client *gitlabclient.Client) error {
			_, err := ListStatus(context.Background(), client, ListStatusInput{})
			return err
		}},
		{name: "get_status", status: http.StatusNotFound, hint: "the site must have reported status at least once", call: func(client *gitlabclient.Client) error {
			_, err := GetStatus(context.Background(), client, IDInput{ID: 1})
			return err
		}},
	}
	for _, handler := range handlers {
		t.Run(handler.name, func(t *testing.T) {
			answers := []struct {
				name   string
				status int
				hinted bool
			}{
				{name: "on its status", status: handler.status, hinted: true},
				{name: "on another status", status: http.StatusInternalServerError, hinted: false},
			}
			for _, answer := range answers {
				t.Run(answer.name, func(t *testing.T) {
					client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						w.WriteHeader(answer.status)
					}))
					err := handler.call(client)
					if err == nil {
						t.Fatalf("no error on a %d answer", answer.status)
					}
					if strings.Contains(err.Error(), handler.hint) != answer.hinted {
						t.Errorf("error = %v, want the hint %q carried: %t", err, handler.hint, answer.hinted)
					}
				})
			}
		})
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

// TestList_Empty verifies an instance with no Geo site is answered as an
// empty list rather than an error.
// The mock GitLab API at /api/v4/geo_sites (GET) responds with HTTP OK and [].
// It asserts List returns no error and no site.
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

// TestGeo_MissingID_RefusedBeforeGitLabIsAsked verifies the five handlers that
// take a site id refuse a zero one themselves, naming the field a caller has
// to fill. The mock forbids every request, so the refusal can only be the
// handler's: the earlier form of these tests answered 404, which is an error
// whichever layer produced it.
func TestGeo_MissingID_RefusedBeforeGitLabIsAsked(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	handlers := []struct {
		name string
		call func() error
	}{
		{name: "get", call: func() error {
			_, err := Get(context.Background(), client, IDInput{})
			return err
		}},
		{name: "edit", call: func() error {
			_, err := Edit(context.Background(), client, EditInput{})
			return err
		}},
		{name: "delete", call: func() error {
			return Delete(context.Background(), client, IDInput{})
		}},
		{name: "repair", call: func() error {
			_, err := Repair(context.Background(), client, IDInput{})
			return err
		}},
		{name: "get_status", call: func() error {
			_, err := GetStatus(context.Background(), client, IDInput{})
			return err
		}},
	}
	for _, handler := range handlers {
		t.Run(handler.name, func(t *testing.T) {
			err := handler.call()
			if err == nil || err.Error() != "id is required" {
				t.Errorf("error = %v, want the field named as required", err)
			}
		})
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

// TestDelete_Success verifies that Delete succeeds when the GitLab API accepts the removal.
// The mock GitLab API at /api/v4/geo_sites/1 (DELETE) answers 204 with no body.
// It asserts Delete returns no error.
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

// TestListStatus_Empty verifies an instance with no Geo site is answered as
// an empty status list rather than an error.
// The mock GitLab API at /api/v4/geo_sites/status (GET) responds with HTTP OK and [].
// It asserts ListStatus returns no error and no status.
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

// TestGeo_CancelledContext_RefusedBeforeGitLabIsAsked verifies every handler
// answers a cancelled context with the cancellation itself and sends nothing.
// The mock forbids every request: the earlier form of these tests said the
// call happened "without contacting GitLab" over a mock that would have
// answered 404 had it been contacted, so that half was claimed and never
// checked.
func TestGeo_CancelledContext_RefusedBeforeGitLabIsAsked(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := testutil.CancelledCtx(t)
	name := "primary-site"
	handlers := []struct {
		name string
		call func() error
	}{
		{name: "create", call: func() error {
			_, err := Create(ctx, client, CreateInput{Name: &name})
			return err
		}},
		{name: "list", call: func() error {
			_, err := List(ctx, client, ListInput{})
			return err
		}},
		{name: "get", call: func() error {
			_, err := Get(ctx, client, IDInput{ID: 1})
			return err
		}},
		{name: "edit", call: func() error {
			_, err := Edit(ctx, client, EditInput{ID: 1, Name: &name})
			return err
		}},
		{name: "delete", call: func() error {
			return Delete(ctx, client, IDInput{ID: 1})
		}},
		{name: "repair", call: func() error {
			_, err := Repair(ctx, client, IDInput{ID: 1})
			return err
		}},
		{name: "list_status", call: func() error {
			_, err := ListStatus(ctx, client, ListStatusInput{})
			return err
		}},
		{name: "get_status", call: func() error {
			_, err := GetStatus(ctx, client, IDInput{ID: 1})
			return err
		}},
	}
	for _, handler := range handlers {
		t.Run(handler.name, func(t *testing.T) {
			if err := handler.call(); !errors.Is(err, context.Canceled) {
				t.Errorf("error = %v, want the cancellation", err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Pagination — List and ListStatus with pagination headers
// ---------------------------------------------------------------------------

// TestList_WithPagination verifies that List forwards the offset page the
// caller asked for to the GitLab API and parses the response metadata.
// The mock GitLab API at /api/v4/geo_sites (GET) responds with HTTP OK.
// It asserts the page and per_page reach the query, and the response headers
// are propagated to the [toolutil.PaginationOutput].
func TestList_WithPagination(t *testing.T) {
	var query url.Values
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/geo_sites" {
			query = r.URL.Query()
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

	out, err := List(context.Background(), client, ListInput{Page: 3, PerPage: 25})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if got := query.Get("page"); got != "3" {
		t.Errorf("page = %q, want 3", got)
	}
	if got := query.Get("per_page"); got != "25" {
		t.Errorf("per_page = %q, want 25", got)
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

// TestListStatus_WithPagination verifies that ListStatus forwards the offset
// page the caller asked for to the GitLab API and parses the response metadata.
// The mock GitLab API at /api/v4/geo_sites/status (GET) responds with HTTP OK.
// It asserts the page and per_page reach the query, and the response headers
// are propagated to the [toolutil.PaginationOutput].
func TestListStatus_WithPagination(t *testing.T) {
	var query url.Values
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/geo_sites/status" {
			query = r.URL.Query()
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

	out, err := ListStatus(context.Background(), client, ListStatusInput{Page: 2, PerPage: 10})
	if err != nil {
		t.Fatalf("ListStatus() error: %v", err)
	}
	if got := query.Get("page"); got != "2" {
		t.Errorf("page = %q, want 2", got)
	}
	if got := query.Get("per_page"); got != "10" {
		t.Errorf("per_page = %q, want 10", got)
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
// which subset, and the replication page was never linked at all. Both scope
// rows are filled although a site carries one or the other, so a dropped row
// shows.
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
		SelectiveSyncShards:                     []string{"default", "nvme"},
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
		"- **Selective Sync Shards**: default, nvme\n"+
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
// each site with its own captured extras rather than dropping them, or
// handing every site the first one's: two sites with different timeouts, so
// the pairing is observable.
func TestList_PublishesTheSiteFieldsTheSDKDrops(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/geo_sites" {
			testutil.RespondJSON(w, http.StatusOK, `[`+geoSiteJSON+`,`+distinctGeoSiteJSON+`]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(out.Sites) != 2 {
		t.Fatalf("expected 2 sites, got %d", len(out.Sites))
	}
	if out.Sites[0].ID != 1 || out.Sites[0].BlobDownloadTimeout != 28800 {
		t.Errorf("Sites[0] = id %d, BlobDownloadTimeout %d; want 1 and 28800", out.Sites[0].ID, out.Sites[0].BlobDownloadTimeout)
	}
	if out.Sites[1].ID != 11 || out.Sites[1].BlobDownloadTimeout != 26 {
		t.Errorf("Sites[1] = id %d, BlobDownloadTimeout %d; want 11 and 26", out.Sites[1].ID, out.Sites[1].BlobDownloadTimeout)
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
// decomposes each element and pairs it with its own matrix rather than the
// first one's: two statuses with different LFS counts, so the pairing is
// observable.
func TestListStatus_PublishesTheReplicableMatrix(t *testing.T) {
	second := strings.NewReplacer(`"geo_node_id": 1`, `"geo_node_id": 2`, `"lfs_objects_count": 120`, `"lfs_objects_count": 240`).Replace(geoSiteStatusJSON)
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/geo_sites/status" {
			testutil.RespondJSON(w, http.StatusOK, `[`+geoSiteStatusJSON+`,`+second+`]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListStatus(context.Background(), client, ListStatusInput{})
	if err != nil {
		t.Fatalf("ListStatus() error: %v", err)
	}
	if len(out.Statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(out.Statuses))
	}
	if out.Statuses[0].GeoNodeID != 1 || out.Statuses[0].Replicables["lfs_objects"].Count != 120 {
		t.Errorf("Statuses[0] = node %d, lfs_objects %+v; want node 1 with 120", out.Statuses[0].GeoNodeID, out.Statuses[0].Replicables["lfs_objects"])
	}
	if out.Statuses[1].GeoNodeID != 2 || out.Statuses[1].Replicables["lfs_objects"].Count != 240 {
		t.Errorf("Statuses[1] = node %d, lfs_objects %+v; want node 2 with 240", out.Statuses[1].GeoNodeID, out.Statuses[1].Replicables["lfs_objects"])
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
// Every field under its own name: the converters and the request bodies
// ---------------------------------------------------------------------------
//
// Both gates score branches, and a converter that copies a field from its
// neighbor, or a request that drops one of the caller's values, has no branch
// to flip. Only an assertion naming each field against a fixture in which no
// two values agree can catch that, and geoSiteJSON cannot serve: it sends 10
// as both files_max_capacity and container_repositories_max_capacity, 1 as the
// id, and true for three of the four flags.

// distinctGeoSiteJSON is a site in which no two values agree, the four flags
// aside, which the tests below drive one at a time.
const distinctGeoSiteJSON = `{
	"id": 11,
	"name": "distinct-site",
	"url": "https://distinct.example.com",
	"internal_url": "https://distinct.internal",
	"primary": false,
	"enabled": false,
	"current": false,
	"files_max_capacity": 21,
	"repos_max_capacity": 22,
	"verification_max_capacity": 23,
	"container_repositories_max_capacity": 24,
	"sync_object_storage": false,
	"selective_sync_type": "shards",
	"selective_sync_shards": ["shard-a", "shard-b"],
	"selective_sync_namespace_ids": [31, 32],
	"selective_sync_organization_ids": [41, 42],
	"minimum_reverification_interval": 25,
	"blob_download_timeout": 26,
	"checksum_mismatch_report_threshold": 27,
	"checksum_mismatch_self_heal_cooldown_minutes": 28,
	"web_edit_url": "https://distinct.example.com/admin/geo/sites/11/edit",
	"web_geo_replication_details_url": "https://distinct.example.com/admin/geo/replication",
	"_links": {
		"self": "https://distinct.example.com/api/v4/geo_sites/11",
		"status": "https://distinct.example.com/api/v4/geo_sites/11/status",
		"repair": "https://distinct.example.com/api/v4/geo_sites/11/repair"
	}
}`

// distinctGeoSite is the Output distinctGeoSiteJSON has to arrive as, every
// flag off.
func distinctGeoSite() Output {
	return Output{
		ID:                                      11,
		Name:                                    "distinct-site",
		URL:                                     "https://distinct.example.com",
		InternalURL:                             "https://distinct.internal",
		FilesMaxCapacity:                        21,
		ReposMaxCapacity:                        22,
		VerificationMaxCapacity:                 23,
		ContainerRepositoriesMaxCapacity:        24,
		SelectiveSyncType:                       "shards",
		SelectiveSyncShards:                     []string{"shard-a", "shard-b"},
		SelectiveSyncNamespaceIDs:               []int64{31, 32},
		SelectiveSyncOrganizationIDs:            []int64{41, 42},
		MinimumReverificationInterval:           25,
		BlobDownloadTimeout:                     26,
		ChecksumMismatchReportThreshold:         27,
		ChecksumMismatchSelfHealCooldownMinutes: 28,
		WebEditURL:                              "https://distinct.example.com/admin/geo/sites/11/edit",
		WebGeoReplicationDetailsURL:             "https://distinct.example.com/admin/geo/replication",
		Links: Links{
			Self:   "https://distinct.example.com/api/v4/geo_sites/11",
			Status: "https://distinct.example.com/api/v4/geo_sites/11/status",
			Repair: "https://distinct.example.com/api/v4/geo_sites/11/repair",
		},
	}
}

// TestGet_EveryFieldArrivesUnderItsOwnName pins which source field each
// published site field is read from, the three links included, which the
// mirror test above only holds to being non-empty. The four flags are driven
// one at a time: four bools take two values, so setting them together always
// leaves a pair agreeing, and a swap of that pair would pass.
func TestGet_EveryFieldArrivesUnderItsOwnName(t *testing.T) {
	cases := []struct {
		name string
		flag string
		want func(*Output)
	}{
		{name: "no flag set", want: func(*Output) {}},
		{name: "primary", flag: "primary", want: func(o *Output) { o.Primary = true }},
		{name: "enabled", flag: "enabled", want: func(o *Output) { o.Enabled = true }},
		{name: "current", flag: "current", want: func(o *Output) { o.Current = true }},
		{name: "sync_object_storage", flag: "sync_object_storage", want: func(o *Output) { o.SyncObjectStorage = true }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			body := distinctGeoSiteJSON
			if testCase.flag != "" {
				body = strings.Replace(body, `"`+testCase.flag+`": false`, `"`+testCase.flag+`": true`, 1)
			}
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && r.URL.Path == "/api/v4/geo_sites/11" {
					testutil.RespondJSON(w, http.StatusOK, body)
					return
				}
				http.NotFound(w, r)
			}))

			got, err := Get(context.Background(), client, IDInput{ID: 11})
			if err != nil {
				t.Fatalf("Get() error: %v", err)
			}
			want := distinctGeoSite()
			testCase.want(&want)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("Get() = %+v\nwant %+v", got, want)
			}
		})
	}
}

// statusFieldName is the json name a field of StatusOutput publishes under,
// or "" for the embed and for a field that publishes none.
func statusFieldName(field reflect.StructField) string {
	if field.Anonymous {
		return ""
	}
	name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
	if name == "-" {
		return ""
	}
	return name
}

// distinctStatusAnswer builds a status answer in which no two values agree,
// keyed by the names StatusOutput publishes: every count its own number, every
// text its own string, every flag false, and the two maps derived from the
// rest left out. Built off the struct rather than written down, because two
// hundred keys spelled by hand would be checked by nobody.
func distinctStatusAnswer(t *testing.T) map[string]any {
	t.Helper()
	answer := map[string]any{}
	n := int64(0)
	for field := range reflect.TypeFor[StatusOutput]().Fields() {
		name := statusFieldName(field)
		if name == "" {
			continue
		}
		n++
		switch field.Type {
		case reflect.TypeFor[int64]():
			answer[name] = 1000 + n
		case reflect.TypeFor[string]():
			answer[name] = fmt.Sprintf("%s %d", name, n)
		case reflect.TypeFor[bool]():
			answer[name] = false
		case reflect.TypeFor[time.Time]():
			answer[name] = "2026-01-15T10:30:00Z"
		case reflect.TypeFor[[]string]():
			answer[name] = []string{fmt.Sprintf("%s-%d-a", name, n), fmt.Sprintf("%s-%d-b", name, n)}
		case reflect.TypeFor[StatusLinks]():
			answer[name] = StatusLinks{Self: "https://distinct.example.com/status", Site: "https://distinct.example.com/site"}
		case reflect.TypeFor[[]StorageShard]():
			answer[name] = []StorageShard{{Name: "shard-a"}, {Name: "shard-b"}}
		case reflect.TypeFor[map[string]ReplicableStatus](), reflect.TypeFor[map[string]any]():
			// Folded from the keys above by the handler, never sent.
		default:
			t.Fatalf("no distinct value for %s of type %s", name, field.Type)
		}
	}
	return answer
}

// statusFlagNames is the json name of every flag StatusOutput publishes.
func statusFlagNames() []string {
	var names []string
	for field := range reflect.TypeFor[StatusOutput]().Fields() {
		if name := statusFieldName(field); name != "" && field.Type == reflect.TypeFor[bool]() {
			names = append(names, name)
		}
	}
	return names
}

// assertStatusFields holds every published field of got to want by name, the
// two derived from the rest apart: the matrix has to hold the fifteen
// resources the type spells out, under the counts they were folded from, and
// nothing may be left over from an answer whose every key the type publishes.
func assertStatusFields(t *testing.T, got, want StatusOutput) {
	t.Helper()
	gotValue, wantValue := reflect.ValueOf(got), reflect.ValueOf(want)
	for field := range reflect.TypeFor[StatusOutput]().Fields() {
		name := statusFieldName(field)
		if name == "" || name == "replicables" || name == "additional_fields" {
			continue
		}
		g, w := gotValue.FieldByIndex(field.Index).Interface(), wantValue.FieldByIndex(field.Index).Interface()
		if instant, ok := g.(time.Time); ok {
			if !instant.Equal(w.(time.Time)) {
				t.Errorf("%s = %v, want %v", name, instant, w)
			}
			continue
		}
		if !reflect.DeepEqual(g, w) {
			t.Errorf("%s = %v, want %v", name, g, w)
		}
	}
	if len(got.Replicables) != 15 {
		t.Errorf("Replicables has %d entries, want the fifteen resources StatusOutput spells out", len(got.Replicables))
	}
	if lfs := got.Replicables["lfs_objects"]; lfs.Count != want.LFSObjectsCount || lfs.VerifiedInPercentage != want.LFSObjectsVerifiedInPercentage {
		t.Errorf("Replicables[lfs_objects] = %+v, want count %d and verified %q", lfs, want.LFSObjectsCount, want.LFSObjectsVerifiedInPercentage)
	}
	if wiki := got.Replicables["project_wiki_repositories"]; wiki.VerificationFailedCount != want.ProjectWikiRepositoriesVerificationFailedCount {
		t.Errorf("Replicables[project_wiki_repositories] = %+v, want verification failed %d", wiki, want.ProjectWikiRepositoriesVerificationFailedCount)
	}
	if len(got.AdditionalFields) != 0 {
		t.Errorf("AdditionalFields = %+v, want nothing: every key sent is one the type publishes", got.AdditionalFields)
	}
}

// TestGetStatus_EveryFieldArrivesUnderItsOwnName pins which source field each
// of the two hundred published status fields is read from, against an answer
// in which no two values agree: with geoSiteStatusJSON a count copied from its
// neighbor is invisible, since most of them are absent there and read as the
// same zero. What each field should hold is the same answer decoded through
// StatusOutput's own tags, so the check is the handler's path against the
// type's declaration, and a name client-go spells differently from this type
// is reported as the zero it decodes to. The four flags are driven one at a
// time, since two flags carrying one value cannot tell a swap apart.
func TestGetStatus_EveryFieldArrivesUnderItsOwnName(t *testing.T) {
	flags := []string{""}
	flags = append(flags, statusFlagNames()...)
	for _, flag := range flags {
		name := flag
		if name == "" {
			name = "no flag set"
		}
		t.Run(name, func(t *testing.T) {
			answer := distinctStatusAnswer(t)
			if flag != "" {
				answer[flag] = true
			}
			body, err := json.Marshal(answer)
			if err != nil {
				t.Fatalf("marshal the answer: %v", err)
			}
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && r.URL.Path == "/api/v4/geo_sites/1/status" {
					testutil.RespondJSON(w, http.StatusOK, string(body))
					return
				}
				http.NotFound(w, r)
			}))

			got, err := GetStatus(context.Background(), client, IDInput{ID: 1})
			if err != nil {
				t.Fatalf("GetStatus() error: %v", err)
			}
			var want StatusOutput
			if err = json.Unmarshal(body, &want); err != nil {
				t.Fatalf("decode the answer through StatusOutput: %v", err)
			}
			assertStatusFields(t, got, want)
		})
	}
}

// decodeGeoRequest reads the JSON body a site write sent GitLab into the
// option struct client-go encoded it from. Every option is a pointer with
// omitempty, so a nil pointer after the decode is the assertion that the
// handler declined to send that key.
func decodeGeoRequest(t *testing.T, body []byte, into any) {
	t.Helper()
	if err := json.Unmarshal(body, into); err != nil {
		t.Fatalf("request body is not JSON: %v (body=%q)", err, body)
	}
}

// TestCreate_EveryFieldReachesTheRequestUnderItsOwnName verifies the create
// handler forwards each input to the option GitLab reads it from, with the
// caller's own value, and sends no key the caller left unset: the success test
// discards the body, so a value copied from its neighbor or dropped never
// reached an assertion. The three flags are driven one at a time.
func TestCreate_EveryFieldReachesTheRequestUnderItsOwnName(t *testing.T) {
	cases := []struct {
		name string
		in   CreateInput
		want gl.CreateGeoSitesOptions
	}{
		{
			name: "every value supplied",
			in: CreateInput{
				Name:                             new("distinct-site"),
				URL:                              new("https://distinct.example.com"),
				InternalURL:                      new("https://distinct.internal"),
				FilesMaxCapacity:                 new(int64(21)),
				ReposMaxCapacity:                 new(int64(22)),
				VerificationMaxCapacity:          new(int64(23)),
				ContainerRepositoriesMaxCapacity: new(int64(24)),
				SelectiveSyncType:                new("shards"),
				SelectiveSyncShards:              new([]string{"shard-a", "shard-b"}),
				SelectiveSyncNamespaceIDs:        new([]int64{31, 32}),
				MinimumReverificationInterval:    new(int64(25)),
			},
			want: gl.CreateGeoSitesOptions{
				Name:                             new("distinct-site"),
				URL:                              new("https://distinct.example.com"),
				InternalURL:                      new("https://distinct.internal"),
				FilesMaxCapacity:                 new(int64(21)),
				ReposMaxCapacity:                 new(int64(22)),
				VerificationMaxCapacity:          new(int64(23)),
				ContainerRepositoriesMaxCapacity: new(int64(24)),
				SelectiveSyncType:                new("shards"),
				SelectiveSyncShards:              new([]string{"shard-a", "shard-b"}),
				SelectiveSyncNamespaceIDs:        new([]int64{31, 32}),
				MinimumReverificationInterval:    new(int64(25)),
			},
		},
		{name: "primary alone", in: CreateInput{Primary: new(true)}, want: gl.CreateGeoSitesOptions{Primary: new(true)}},
		{name: "enabled alone", in: CreateInput{Enabled: new(true)}, want: gl.CreateGeoSitesOptions{Enabled: new(true)}},
		{name: "sync_object_storage alone", in: CreateInput{SyncObjectStorage: new(true)}, want: gl.CreateGeoSitesOptions{SyncObjectStorage: new(true)}},
		{name: "nothing supplied"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var body []byte
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost && r.URL.Path == "/api/v4/geo_sites" {
					body, _ = io.ReadAll(r.Body)
					testutil.RespondJSON(w, http.StatusCreated, geoSiteJSON)
					return
				}
				http.NotFound(w, r)
			}))

			if _, err := Create(context.Background(), client, testCase.in); err != nil {
				t.Fatalf("Create() error: %v", err)
			}
			var sent gl.CreateGeoSitesOptions
			decodeGeoRequest(t, body, &sent)
			if !reflect.DeepEqual(sent, testCase.want) {
				t.Errorf("request = %s, want the caller's values under their own keys and nothing else", body)
			}
		})
	}
}

// TestEdit_EveryFieldReachesTheRequestUnderItsOwnName is Create's assertion
// for the PUT. Edit builds its own copy of the mapping, so a field the create
// side forwards is no evidence about this one.
func TestEdit_EveryFieldReachesTheRequestUnderItsOwnName(t *testing.T) {
	cases := []struct {
		name string
		in   EditInput
		want gl.EditGeoSiteOptions
	}{
		{
			name: "every value supplied",
			in: EditInput{
				ID:                               11,
				Name:                             new("distinct-site"),
				URL:                              new("https://distinct.example.com"),
				InternalURL:                      new("https://distinct.internal"),
				FilesMaxCapacity:                 new(int64(21)),
				ReposMaxCapacity:                 new(int64(22)),
				VerificationMaxCapacity:          new(int64(23)),
				ContainerRepositoriesMaxCapacity: new(int64(24)),
				SelectiveSyncType:                new("namespaces"),
				SelectiveSyncShards:              new([]string{"shard-a", "shard-b"}),
				SelectiveSyncNamespaceIDs:        new([]int64{31, 32}),
				MinimumReverificationInterval:    new(int64(25)),
			},
			want: gl.EditGeoSiteOptions{
				Name:                             new("distinct-site"),
				URL:                              new("https://distinct.example.com"),
				InternalURL:                      new("https://distinct.internal"),
				FilesMaxCapacity:                 new(int64(21)),
				ReposMaxCapacity:                 new(int64(22)),
				VerificationMaxCapacity:          new(int64(23)),
				ContainerRepositoriesMaxCapacity: new(int64(24)),
				SelectiveSyncType:                new("namespaces"),
				SelectiveSyncShards:              new([]string{"shard-a", "shard-b"}),
				SelectiveSyncNamespaceIDs:        new([]int64{31, 32}),
				MinimumReverificationInterval:    new(int64(25)),
			},
		},
		{name: "enabled alone", in: EditInput{ID: 11, Enabled: new(true)}, want: gl.EditGeoSiteOptions{Enabled: new(true)}},
		{name: "nothing supplied", in: EditInput{ID: 11}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var body []byte
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut && r.URL.Path == "/api/v4/geo_sites/11" {
					body, _ = io.ReadAll(r.Body)
					testutil.RespondJSON(w, http.StatusOK, distinctGeoSiteJSON)
					return
				}
				http.NotFound(w, r)
			}))

			if _, err := Edit(context.Background(), client, testCase.in); err != nil {
				t.Fatalf("Edit() error: %v", err)
			}
			var sent gl.EditGeoSiteOptions
			decodeGeoRequest(t, body, &sent)
			if !reflect.DeepEqual(sent, testCase.want) {
				t.Errorf("request = %s, want the caller's values under their own keys and nothing else", body)
			}
		})
	}
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
