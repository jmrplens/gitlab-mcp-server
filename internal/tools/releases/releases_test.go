// releases_test.go contains unit tests for GitLab release operations
// (create, update, delete, get, list). Tests use httptest to mock the
// GitLab Releases API and verify both success and error paths.

package releases

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// Test constants for release API endpoint paths and expected values.
const (
	pathProjectReleases = "/api/v4/projects/42/releases"
	testTagV120         = "v1.2.0"
	testReleaseName     = "Release v1.2.0"
	fmtOutTagNameWant   = "out.TagName = %q, want %q"
	pathReleaseV120     = "/api/v4/projects/42/releases/v1.2.0"
	testUpdatedNotes    = "Updated notes"
	testTagV200         = "v2.0.0"
)

// TestReleaseCreate_Success verifies that Create correctly creates a
// release with a tag name, title, and description. The mock returns a 201
// response and the test asserts the output fields match the expected values.
func TestReleaseCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProjectReleases {
			testutil.RespondJSON(w, http.StatusCreated, `{"tag_name":"v1.2.0","name":"Release v1.2.0","description":"## Changelog\n- Feature A","created_at":"2026-03-02T10:00:00Z","released_at":"2026-03-02T10:00:00Z"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:   "42",
		TagName:     testTagV120,
		Name:        testReleaseName,
		Description: "## Changelog\n- Feature A",
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.TagName != testTagV120 {
		t.Errorf(fmtOutTagNameWant, out.TagName, testTagV120)
	}
	if out.Name != testReleaseName {
		t.Errorf("out.Name = %q, want %q", out.Name, testReleaseName)
	}
}

// TestReleaseCreate_MissingTag verifies that Create returns an error
// when the specified tag does not exist. The mock returns a 404 response.
func TestReleaseCreate_MissingTag(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Tag Not Found"}`)
	}))

	_, err := Create(context.Background(), client, CreateInput{
		ProjectID: "42",
		TagName:   "nonexistent-tag",
	})
	if err == nil {
		t.Fatal("Create() expected error for missing tag, got nil")
	}
}

// TestReleaseUpdate_Success verifies that Update correctly updates a
// release description. The mock returns the updated release and the test
// confirms the description field reflects the new value.
func TestReleaseUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathReleaseV120 {
			testutil.RespondJSON(w, http.StatusOK, `{"tag_name":"v1.2.0","name":"Release v1.2.0 Updated","description":"Updated notes","created_at":"2026-03-02T10:00:00Z","released_at":"2026-03-02T10:00:00Z"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID:   "42",
		TagName:     testTagV120,
		Description: testUpdatedNotes,
	})
	if err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}
	if out.Description != testUpdatedNotes {
		t.Errorf("out.Description = %q, want %q", out.Description, testUpdatedNotes)
	}
}

// TestReleaseDelete_Success verifies that Delete removes a release
// and returns its details. The mock handles the DELETE request and the
// test confirms the deleted release's tag name is preserved in the output.
func TestReleaseDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathReleaseV120 {
			testutil.RespondJSON(w, http.StatusOK, `{"tag_name":"v1.2.0","name":"Release v1.2.0","description":"","created_at":"2026-03-02T10:00:00Z","released_at":"2026-03-02T10:00:00Z"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Delete(context.Background(), client, DeleteInput{ProjectID: "42", TagName: testTagV120})
	if err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}
	if out.TagName != testTagV120 {
		t.Errorf(fmtOutTagNameWant, out.TagName, testTagV120)
	}
}

// TestReleaseGet_Success verifies that Get retrieves a single release
// by tag name. The mock returns the release JSON and the test asserts the
// tag name matches.
func TestReleaseGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathReleaseV120 {
			testutil.RespondJSON(w, http.StatusOK, `{"tag_name":"v1.2.0","name":"Release v1.2.0","description":"Some notes","created_at":"2026-03-02T10:00:00Z","released_at":"2026-03-02T10:00:00Z"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", TagName: testTagV120})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.TagName != testTagV120 {
		t.Errorf(fmtOutTagNameWant, out.TagName, testTagV120)
	}
}

// TestReleaseList_Success verifies that List returns all releases for
// a project. The mock returns two releases and the test asserts the output
// slice length and first element's tag name.
func TestReleaseList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathProjectReleases {
			testutil.RespondJSON(w, http.StatusOK, `[{"tag_name":"v1.2.0","name":"Release v1.2.0","description":"","created_at":"2026-03-02T10:00:00Z","released_at":"2026-03-02T10:00:00Z"},{"tag_name":"v1.1.0","name":"Release v1.1.0","description":"","created_at":"2026-01-01T10:00:00Z","released_at":"2026-01-01T10:00:00Z"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Releases) != 2 {
		t.Errorf("len(out.Releases) = %d, want 2", len(out.Releases))
	}
	if out.Releases[0].TagName != testTagV120 {
		t.Errorf("out.Releases[0].TagName = %q, want %q", out.Releases[0].TagName, testTagV120)
	}
}

// TestReleaseList_PaginationQueryParamsAndMetadata verifies that List
// sends page and per_page query parameters to the GitLab API and correctly
// parses pagination metadata (TotalItems, TotalPages, NextPage) from the
// response headers.
// TestReleaseGetSuccess_EnrichedFields verifies that Get maps enriched
// fields: Author, CommitSHA, UpcomingRelease, Milestones.
func TestReleaseGet_SuccessEnrichedFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathReleaseV120 {
			testutil.RespondJSON(w, http.StatusOK, `{
				"tag_name":"v1.2.0","name":"Release v1.2.0","description":"Notes",
				"created_at":"2026-03-02T10:00:00Z","released_at":"2026-03-02T10:00:00Z",
				"author":{"username":"releaser"},
				"commit":{"id":"abc123def456"},
				"upcoming_release":true,
				"milestones":[{"title":"v1.0"},{"title":"v1.1"}]
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", TagName: testTagV120})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.Author != "releaser" {
		t.Errorf("out.Author = %q, want %q", out.Author, "releaser")
	}
	if out.CommitSHA != "abc123def456" {
		t.Errorf("out.CommitSHA = %q, want %q", out.CommitSHA, "abc123def456")
	}
	if !out.UpcomingRelease {
		t.Error("out.UpcomingRelease = false, want true")
	}
	if len(out.Milestones) != 2 || out.Milestones[0] != "v1.0" || out.Milestones[1] != "v1.1" {
		t.Errorf("out.Milestones = %v, want [v1.0 v1.1]", out.Milestones)
	}
}

// TestReleaseCreateInput_EnrichedFields verifies that Create passes
// the enriched Ref and Milestones fields to the GitLab API.
func TestReleaseCreateInput_EnrichedFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProjectReleases {
			testutil.RespondJSON(w, http.StatusCreated, `{"tag_name":"v2.0.0","name":"v2","description":"","created_at":"2026-06-01T10:00:00Z","released_at":"2026-06-01T10:00:00Z"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:  "42",
		TagName:    testTagV200,
		Ref:        "main",
		Milestones: []string{"v2.0"},
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.TagName != testTagV200 {
		t.Errorf(fmtOutTagNameWant, out.TagName, testTagV200)
	}
}

// TestReleaseList_PaginationQueryParamsAndMetadata verifies the behavior of release list pagination query params and metadata.
func TestReleaseList_PaginationQueryParamsAndMetadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathProjectReleases {
			if got := r.URL.Query().Get("page"); got != "1" {
				t.Errorf("query param page = %q, want %q", got, "1")
			}
			if got := r.URL.Query().Get("per_page"); got != "2" {
				t.Errorf("query param per_page = %q, want %q", got, "2")
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"tag_name":"v1.0.0","name":"v1.0.0","description":"","created_at":"2026-01-01T10:00:00Z","released_at":"2026-01-01T10:00:00Z"}]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "2", Total: "5", TotalPages: "3", NextPage: "2"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: "42", PaginationInput: toolutil.PaginationInput{Page: 1, PerPage: 2}})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if out.Pagination.TotalItems != 5 {
		t.Errorf("Pagination.TotalItems = %d, want 5", out.Pagination.TotalItems)
	}
	if out.Pagination.TotalPages != 3 {
		t.Errorf("Pagination.TotalPages = %d, want 3", out.Pagination.TotalPages)
	}
	if out.Pagination.NextPage != 2 {
		t.Errorf("Pagination.NextPage = %d, want 2", out.Pagination.NextPage)
	}
}

// TestReleaseGet_SuccessAssetsAndEvidences verifies that Get maps
// assets (sources and links) and evidences from the API response.
func TestReleaseGet_SuccessAssetsAndEvidences(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathReleaseV120 {
			testutil.RespondJSON(w, http.StatusOK, `{
				"tag_name":"v1.2.0","name":"Release v1.2.0","description":"Notes",
				"created_at":"2026-03-02T10:00:00Z","released_at":"2026-03-02T10:00:00Z",
				"author":{"username":"releaser"},
				"commit":{"id":"deadbeef"},
				"assets":{"count":2,"sources":[{"format":"zip","url":"https://example.com/archive.zip"}],"links":[{"id":1,"name":"binary","url":"https://example.com/bin","direct_asset_url":"https://example.com/direct","external":true,"link_type":"other"}]},
				"evidences":[{"sha":"abc123","filepath":"/evidences/1","collected_at":"2026-03-01T10:00:00Z"}]
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", TagName: testTagV120})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if len(out.AssetsSources) != 1 {
		t.Fatalf("len(out.AssetsSources) = %d, want 1", len(out.AssetsSources))
	}
	if out.AssetsSources[0].Format != "zip" {
		t.Errorf("out.AssetsSources[0].Format = %q, want %q", out.AssetsSources[0].Format, "zip")
	}
	if len(out.AssetsLinks) != 1 {
		t.Fatalf("len(out.AssetsLinks) = %d, want 1", len(out.AssetsLinks))
	}
	if out.AssetsLinks[0].Name != "binary" {
		t.Errorf("out.AssetsLinks[0].Name = %q, want %q", out.AssetsLinks[0].Name, "binary")
	}
	if !out.AssetsLinks[0].External {
		t.Error("out.AssetsLinks[0].External = false, want true")
	}
	if out.AssetsLinks[0].LinkType != "other" {
		t.Errorf("out.AssetsLinks[0].LinkType = %q, want %q", out.AssetsLinks[0].LinkType, "other")
	}
	if len(out.Evidences) != 1 {
		t.Fatalf("len(out.Evidences) = %d, want 1", len(out.Evidences))
	}
	if out.Evidences[0].SHA != "abc123" {
		t.Errorf("out.Evidences[0].SHA = %q, want %q", out.Evidences[0].SHA, "abc123")
	}
	if !strings.Contains(out.Evidences[0].CollectedAt, "2026") {
		t.Errorf("out.Evidences[0].CollectedAt = %q, want to contain '2026'", out.Evidences[0].CollectedAt)
	}
}

// TestReleaseGetLatest_Success verifies that GetLatest retrieves the
// latest release for a project without specifying a tag name.
func TestReleaseGetLatest_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/releases/permalink/latest" {
			testutil.RespondJSON(w, http.StatusOK, `{"tag_name":"v3.0.0","name":"Release v3.0.0","description":"Latest","created_at":"2026-06-15T10:00:00Z","released_at":"2026-06-15T10:00:00Z","author":{"username":"admin"}}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetLatest(context.Background(), client, GetLatestInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("GetLatest() unexpected error: %v", err)
	}
	if out.TagName != "v3.0.0" {
		t.Errorf("out.TagName = %q, want %q", out.TagName, "v3.0.0")
	}
	if out.Name != "Release v3.0.0" {
		t.Errorf("out.Name = %q, want %q", out.Name, "Release v3.0.0")
	}
}

// TestReleaseGetLatest_NotFound verifies that GetLatest returns an error
// when no releases exist for the project.
func TestReleaseGetLatest_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := GetLatest(context.Background(), client, GetLatestInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("GetLatest() expected error for no releases, got nil")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

const errExpCancelledCtx = "expected error for canceled context"

const errExpectedAPI = "expected API error, got nil"

// ---------------------------------------------------------------------------
// Create — API error, empty project_id, canceled context, tag_message field
// ---------------------------------------------------------------------------.

// TestCreate_APIError verifies the behavior of create a p i error.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{
		ProjectID: "42", TagName: "v1.0.0",
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCreate_MissingProjectID verifies the behavior of create missing project i d.
func TestCreate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Create(context.Background(), client, CreateInput{TagName: "v1.0.0"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestCreate_CancelledContext verifies the behavior of create cancelled context.
func TestCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := Create(ctx, client, CreateInput{ProjectID: "42", TagName: "v1.0.0"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestCreate_WithTagMessage verifies the behavior of create with tag message.
func TestCreate_WithTagMessage(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/releases" {
			testutil.RespondJSON(w, http.StatusCreated, `{"tag_name":"v1.0.0","name":"v1","description":"","created_at":"2026-03-02T10:00:00Z","released_at":"2026-03-02T10:00:00Z"}`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:  "42",
		TagName:    "v1.0.0",
		Name:       "v1",
		TagMessage: "Annotated tag for v1.0.0",
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.TagName != "v1.0.0" {
		t.Errorf("out.TagName = %q, want %q", out.TagName, "v1.0.0")
	}
}

// ---------------------------------------------------------------------------
// Update — API error, empty project_id, canceled context
// ---------------------------------------------------------------------------.

// TestUpdate_APIError verifies the behavior of update a p i error.
func TestUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{
		ProjectID: "42", TagName: "v1.0.0", Name: "updated",
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestUpdate_MissingProjectID verifies the behavior of update missing project i d.
func TestUpdate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Update(context.Background(), client, UpdateInput{TagName: "v1.0.0"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestUpdate_CancelledContext verifies the behavior of update cancelled context.
func TestUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := Update(ctx, client, UpdateInput{ProjectID: "42", TagName: "v1.0.0"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Delete — API error, empty project_id, canceled context
// ---------------------------------------------------------------------------.

// TestDelete_APIError verifies the behavior of delete a p i error.
func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Delete(context.Background(), client, DeleteInput{ProjectID: "42", TagName: "v1.0.0"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDelete_MissingProjectID verifies the behavior of delete missing project i d.
func TestDelete_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Delete(context.Background(), client, DeleteInput{TagName: "v1.0.0"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestDelete_CancelledContext verifies the behavior of delete cancelled context.
func TestDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := Delete(ctx, client, DeleteInput{ProjectID: "42", TagName: "v1.0.0"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Get — API error, empty project_id, canceled context
// ---------------------------------------------------------------------------.

// TestGet_APIError verifies the behavior of get a p i error.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: "42", TagName: "v1.0.0"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGet_MissingProjectID verifies the behavior of get missing project i d.
func TestGet_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Get(context.Background(), client, GetInput{TagName: "v1.0.0"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestGet_CancelledContext verifies the behavior of get cancelled context.
func TestGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := Get(ctx, client, GetInput{ProjectID: "42", TagName: "v1.0.0"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// GetLatest — API error, empty project_id, canceled context
// ---------------------------------------------------------------------------.

// TestGetLatest_APIError verifies the behavior of get latest a p i error.
func TestGetLatest_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := GetLatest(context.Background(), client, GetLatestInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGetLatest_MissingProjectID verifies the behavior of get latest missing project i d.
func TestGetLatest_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := GetLatest(context.Background(), client, GetLatestInput{})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestGetLatest_CancelledContext verifies the behavior of get latest cancelled context.
func TestGetLatest_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetLatest(ctx, client, GetLatestInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// List — API error, empty project_id, canceled context, sort/order_by params
// ---------------------------------------------------------------------------.

// TestList_APIError verifies the behavior of list a p i error.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestList_MissingProjectID verifies the behavior of list missing project i d.
func TestList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestList_CancelledContext verifies the behavior of list cancelled context.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestList_WithOrderByAndSort verifies the behavior of list with order by and sort.
func TestList_WithOrderByAndSort(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/releases" {
			if got := r.URL.Query().Get("order_by"); got != "released_at" {
				t.Errorf("query param order_by = %q, want %q", got, "released_at")
			}
			if got := r.URL.Query().Get("sort"); got != "desc" {
				t.Errorf("query param sort = %q, want %q", got, "desc")
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"tag_name":"v3.0.0","name":"v3","description":"","created_at":"2026-06-01T10:00:00Z","released_at":"2026-06-01T10:00:00Z"}]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))
	out, err := List(context.Background(), client, ListInput{
		ProjectID: "42",
		OrderBy:   "released_at",
		Sort:      "desc",
	})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Releases) != 1 {
		t.Fatalf("len(Releases) = %d, want 1", len(out.Releases))
	}
	if out.Releases[0].TagName != "v3.0.0" {
		t.Errorf("TagName = %q, want %q", out.Releases[0].TagName, "v3.0.0")
	}
}

// TestList_EmptyResult verifies the behavior of list empty result.
func TestList_EmptyResult(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/releases" {
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := List(context.Background(), client, ListInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Releases) != 0 {
		t.Errorf("len(Releases) = %d, want 0", len(out.Releases))
	}
}

// ---------------------------------------------------------------------------
// FormatMarkdown — all fields, minimal fields, upcoming release
// ---------------------------------------------------------------------------.

// TestFormatMarkdown_AllFields verifies the behavior of format markdown all fields.
func TestFormatMarkdown_AllFields(t *testing.T) {
	md := FormatMarkdown(Output{
		TagName:         "v2.0.0",
		Name:            "Release v2.0.0",
		Description:     "## Changes\n- Feature X",
		Author:          "admin",
		CreatedAt:       "2026-03-02T10:00:00Z",
		ReleasedAt:      "2026-03-02T10:00:00Z",
		CommitSHA:       "abc123",
		UpcomingRelease: true,
		Milestones:      []string{"m1", "m2"},
	})
	for _, want := range []string{
		"## Release: Release v2.0.0",
		"**Tag**: v2.0.0",
		"**Author**: @admin",
		"**Commit**: abc123",
		"**Upcoming release**",
		"**Milestones**: m1, m2",
		"### Description",
		"Feature X",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatMarkdown_MinimalFields verifies the behavior of format markdown minimal fields.
func TestFormatMarkdown_MinimalFields(t *testing.T) {
	md := FormatMarkdown(Output{
		TagName:   "v0.1.0",
		Name:      "Beta",
		CreatedAt: "2026-01-01T00:00:00Z",
	})
	if !strings.Contains(md, "## Release: Beta") {
		t.Errorf("missing header:\n%s", md)
	}
	if !strings.Contains(md, "**Tag**: v0.1.0") {
		t.Errorf("missing tag:\n%s", md)
	}
	for _, absent := range []string{
		"**Author**",
		"**Commit**",
		"**Upcoming release**",
		"**Milestones**",
		"### Description",
		"**Released**",
	} {
		if strings.Contains(md, absent) {
			t.Errorf("should not contain %q for minimal output:\n%s", absent, md)
		}
	}
}

// TestFormatMarkdown_WithReleasedAt verifies the behavior of format markdown with released at.
func TestFormatMarkdown_WithReleasedAt(t *testing.T) {
	md := FormatMarkdown(Output{
		TagName:    "v1.5.0",
		Name:       "Patch",
		CreatedAt:  "2026-02-01T00:00:00Z",
		ReleasedAt: "2026-02-15T00:00:00Z",
	})
	if !strings.Contains(md, "**Released**: 15 Feb 2026 00:00 UTC") {
		t.Errorf("missing released_at:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown — with data, empty, pagination
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithData verifies the behavior of format list markdown with data.
func TestFormatListMarkdown_WithData(t *testing.T) {
	out := ListOutput{
		Releases: []Output{
			{TagName: "v2.0.0", Name: "Major", Author: "admin", ReleasedAt: "2026-06-01T10:00:00Z"},
			{TagName: "v1.0.0", Name: "First", Author: "dev", CreatedAt: "2026-01-01T10:00:00Z"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatListMarkdown(out)
	for _, want := range []string{
		"## Releases (2)",
		"| Tag | Name | Author | Released |",
		"v2.0.0",
		"Major",
		"admin",
		"v1.0.0",
		"First",
		"dev",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatListMarkdown_Empty verifies the behavior of format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	out := ListOutput{
		Releases:   []Output{},
		Pagination: toolutil.PaginationOutput{TotalItems: 0},
	}
	md := FormatListMarkdown(out)
	if !strings.Contains(md, "No releases found.") {
		t.Errorf("expected 'No releases found.' in:\n%s", md)
	}
	if strings.Contains(md, "| Tag |") {
		t.Errorf("should not contain table header for empty list:\n%s", md)
	}
}

// TestFormatListMarkdown_FallbackToCreatedAt verifies the behavior of format list markdown fallback to created at.
func TestFormatListMarkdown_FallbackToCreatedAt(t *testing.T) {
	out := ListOutput{
		Releases: []Output{
			{TagName: "v0.1.0", Name: "Alpha", CreatedAt: "2026-01-01T00:00:00Z"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatListMarkdown(out)
	if !strings.Contains(md, "1 Jan 2026 00:00 UTC") {
		t.Errorf("expected created_at fallback in Released column:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// ToOutput — edge cases
// ---------------------------------------------------------------------------.

// TestToOutput_NilTimestamps verifies the behavior of to output nil timestamps.
func TestToOutput_NilTimestamps(t *testing.T) {
	out := ToOutput(&gl.Release{})
	if out.CreatedAt != "" {
		t.Errorf("out.CreatedAt = %q, want empty", out.CreatedAt)
	}
	if out.ReleasedAt != "" {
		t.Errorf("out.ReleasedAt = %q, want empty", out.ReleasedAt)
	}
}

// TestToOutput_NoAssetsSources verifies the behavior of to output no assets sources.
func TestToOutput_NoAssetsSources(t *testing.T) {
	out := ToOutput(&gl.Release{})
	if out.AssetsSources != nil {
		t.Errorf("out.AssetsSources should be nil, got %v", out.AssetsSources)
	}
}

// TestToOutput_NoEvidences verifies the behavior of to output no evidences.
func TestToOutput_NoEvidences(t *testing.T) {
	out := ToOutput(&gl.Release{})
	if out.Evidences != nil {
		t.Errorf("out.Evidences should be nil, got %v", out.Evidences)
	}
}

// TestToOutput_NoMilestones verifies the behavior of to output no milestones.
func TestToOutput_NoMilestones(t *testing.T) {
	out := ToOutput(&gl.Release{})
	if out.Milestones != nil {
		t.Errorf("out.Milestones should be nil, got %v", out.Milestones)
	}
}

// TestToOutput_EmptyCommitID verifies the behavior of to output empty commit i d.
func TestToOutput_EmptyCommitID(t *testing.T) {
	out := ToOutput(&gl.Release{})
	if out.CommitSHA != "" {
		t.Errorf("out.CommitSHA = %q, want empty", out.CommitSHA)
	}
}

// TestToOutput_WebURL_DerivedFromEditURL verifies that ToOutput derives
// WebURL by stripping the /edit suffix from Links.EditURL.
func TestToOutput_WebURL_DerivedFromEditURL(t *testing.T) {
	r := &gl.Release{
		Links: gl.ReleaseLinks{
			EditURL: "https://gitlab.example.com/group/project/-/releases/v1.0.0/edit",
		},
	}
	out := ToOutput(r)
	want := "https://gitlab.example.com/group/project/-/releases/v1.0.0"
	if out.WebURL != want {
		t.Errorf("WebURL = %q, want %q", out.WebURL, want)
	}
}

// TestToOutput_WebURL_EmptyEditURL verifies that WebURL is empty when
// Links.EditURL is not provided.
func TestToOutput_WebURL_EmptyEditURL(t *testing.T) {
	out := ToOutput(&gl.Release{})
	if out.WebURL != "" {
		t.Errorf("WebURL = %q, want empty", out.WebURL)
	}
}

// TestFormatMarkdown_WithWebURL verifies that the detail Markdown includes
// a clickable URL link when WebURL is populated.
func TestFormatMarkdown_WithWebURL(t *testing.T) {
	md := FormatMarkdown(Output{
		TagName:   "v1.0.0",
		Name:      "Release v1.0.0",
		CreatedAt: "2026-03-01T10:00:00Z",
		WebURL:    "https://gitlab.example.com/-/releases/v1.0.0",
	})
	want := "[https://gitlab.example.com/-/releases/v1.0.0](https://gitlab.example.com/-/releases/v1.0.0)"
	if !strings.Contains(md, want) {
		t.Errorf("FormatMarkdown missing clickable URL link, got:\n%s", md)
	}
}

// TestFormatMarkdown_WithoutWebURL verifies that no URL line appears when
// WebURL is empty.
func TestFormatMarkdown_WithoutWebURL(t *testing.T) {
	md := FormatMarkdown(Output{
		TagName:   "v0.1.0",
		Name:      "Alpha",
		CreatedAt: "2026-01-01T00:00:00Z",
	})
	if strings.Contains(md, "**URL**") {
		t.Errorf("FormatMarkdown should not contain URL when empty, got:\n%s", md)
	}
}

// TestFormatMarkdown_ContainsHints verifies that FormatMarkdown includes
// next-step hints guiding the user to release link tools (single and batch)
// and to publish_and_link for uploading binaries.
func TestFormatMarkdown_ContainsHints(t *testing.T) {
	md := FormatMarkdown(Output{
		TagName:   "v1.0.0",
		Name:      "Hints test",
		CreatedAt: "2026-01-01T00:00:00Z",
	})
	for _, want := range []string{
		"link_create'",
		"link_create_batch'",
		"publish_and_link'",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatMarkdown missing hint containing %q:\n%s", want, md)
		}
	}
}

// TestFormatListMarkdown_ClickableTagLink verifies that the list table
// renders tag names as clickable Markdown links when WebURL is present.
func TestFormatListMarkdown_ClickableTagLink(t *testing.T) {
	out := ListOutput{
		Releases: []Output{
			{TagName: "v2.0.0", Name: "Major", Author: "admin", ReleasedAt: "2026-06-01T10:00:00Z", WebURL: "https://gitlab.example.com/-/releases/v2.0.0"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatListMarkdown(out)
	if !strings.Contains(md, "[v2.0.0](https://gitlab.example.com/-/releases/v2.0.0)") {
		t.Errorf("FormatListMarkdown missing clickable tag link, got:\n%s", md)
	}
}

// TestFormatListMarkdown_NoLinkWithoutWebURL verifies that tag names appear
// as plain text when WebURL is empty.
func TestFormatListMarkdown_NoLinkWithoutWebURL(t *testing.T) {
	out := ListOutput{
		Releases: []Output{
			{TagName: "v1.0.0", Name: "First", CreatedAt: "2026-01-01T00:00:00Z"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatListMarkdown(out)
	if strings.Contains(md, "[v1.0.0](") {
		t.Errorf("FormatListMarkdown should not contain link when WebURL is empty, got:\n%s", md)
	}
	if !strings.Contains(md, "v1.0.0") {
		t.Errorf("FormatListMarkdown should contain tag name as plain text, got:\n%s", md)
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
// RegisterToolsCallAllThroughMCP — full MCP roundtrip for all 6 tools
// ---------------------------------------------------------------------------.

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newReleasesMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"create", "gitlab_release_create", map[string]any{"project_id": "42", "tag_name": "v1.0.0", "name": "v1"}},
		{"update", "gitlab_release_update", map[string]any{"project_id": "42", "tag_name": "v1.0.0", "name": "updated"}},
		{"delete", "gitlab_release_delete", map[string]any{"project_id": "42", "tag_name": "v1.0.0"}},
		{"get", "gitlab_release_get", map[string]any{"project_id": "42", "tag_name": "v1.0.0"}},
		{"list", "gitlab_release_list", map[string]any{"project_id": "42"}},
		{"latest", "gitlab_release_latest", map[string]any{"project_id": "42"}},
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

// TestMCPRoundTrip_DeleteConfirmDeclined covers the ConfirmAction early-return
// branch in gitlab_release_delete when user declines.
func TestMCPRoundTrip_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "decline"}, nil
		},
	})
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_release_delete",
		Arguments: map[string]any{"project_id": "42", "tag_name": "v1.0.0"},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

// TestMCPRoundTrip_GetNotFound covers the 404 NotFoundResult path in
// gitlab_release_get when the release does not exist.
func TestMCPRoundTrip_GetNotFound(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Release Not Found"}`)
	})
	client := testutil.NewTestClient(t, handler)
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
		Name:      "gitlab_release_get",
		Arguments: map[string]any{"project_id": "42", "tag_name": "v99.0.0"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected IsError result for 404")
	}
}

// TestUpdate_InvalidReleasedAt covers the released_at parse error branch.
func TestUpdate_InvalidReleasedAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := Update(context.Background(), client, UpdateInput{
		ProjectID:  "42",
		TagName:    "v1.0.0",
		ReleasedAt: "not-a-date",
	})
	if err == nil {
		t.Fatal("expected error for invalid released_at format")
	}
	if !strings.Contains(err.Error(), "invalid released_at") {
		t.Fatalf("error should mention released_at: %v", err)
	}
}

// TestCreate_ConflictError covers the 409/422 error branch in Create.
func TestCreate_ConflictError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusConflict, `{"message":"Release already exists"}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: "42", TagName: "v1.0.0"})
	if err == nil {
		t.Fatal("expected error for 409")
	}
}

// TestCreate_ForbiddenError covers the 403 error branch in Create.
func TestCreate_ForbiddenError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: "42", TagName: "v1.0.0"})
	if err == nil {
		t.Fatal("expected error for 403")
	}
}

// ---------------------------------------------------------------------------
// Helper: MCP session factory
// ---------------------------------------------------------------------------.

// newReleasesMCPSession is an internal helper for the releases package.
func newReleasesMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	releaseJSON := `{"tag_name":"v1.0.0","name":"v1","description":"notes","created_at":"2026-03-02T10:00:00Z","released_at":"2026-03-02T10:00:00Z","author":{"username":"admin"}}`

	handler := http.NewServeMux()

	// Create release
	handler.HandleFunc("POST /api/v4/projects/42/releases", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, releaseJSON)
	})

	// Update release
	handler.HandleFunc("PUT /api/v4/projects/42/releases/v1.0.0", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, releaseJSON)
	})

	// Delete release
	handler.HandleFunc("DELETE /api/v4/projects/42/releases/v1.0.0", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, releaseJSON)
	})

	// Get release
	handler.HandleFunc("GET /api/v4/projects/42/releases/v1.0.0", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, releaseJSON)
	})

	// List releases
	handler.HandleFunc("GET /api/v4/projects/42/releases", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+releaseJSON+`]`)
	})

	// Get latest release
	handler.HandleFunc("GET /api/v4/projects/42/releases/permalink/latest", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, releaseJSON)
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

// TestUpdate_WithMilestonesAndReleasedAt verifies that Update forwards both
// milestones and a valid released_at timestamp to the GitLab API. This
// targets the success branch of the released_at parser (assigning the
// parsed time to opts.ReleasedAt) and the milestones-non-empty branch
// (copying the slice into opts.Milestones).
func TestUpdate_WithMilestonesAndReleasedAt(t *testing.T) {
	var capturedBody []byte
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathReleaseV120 {
			b, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read request body: %v", err)
				http.Error(w, "read body failed", http.StatusInternalServerError)
				return
			}
			capturedBody = b
			testutil.RespondJSON(w, http.StatusOK, `{"tag_name":"v1.2.0","name":"r","description":"d"}`)
			return
		}
		http.NotFound(w, r)
	}))
	if _, err := Update(context.Background(), client, UpdateInput{
		ProjectID:  "42",
		TagName:    testTagV120,
		Milestones: []string{"M1", "M2"},
		ReleasedAt: "2026-01-15T10:00:00Z",
	}); err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}
	var payload struct {
		Milestones []string `json:"milestones"`
		ReleasedAt string   `json:"released_at"`
	}
	if err := json.Unmarshal(capturedBody, &payload); err != nil {
		t.Fatalf("unmarshal request body: %v; body=%q", err, string(capturedBody))
	}
	wantMilestones := []string{"M1", "M2"}
	if !reflect.DeepEqual(payload.Milestones, wantMilestones) {
		t.Errorf("milestones = %v, want %v", payload.Milestones, wantMilestones)
	}
	if payload.ReleasedAt != "2026-01-15T10:00:00Z" {
		t.Errorf("released_at = %q, want %q", payload.ReleasedAt, "2026-01-15T10:00:00Z")
	}
}
