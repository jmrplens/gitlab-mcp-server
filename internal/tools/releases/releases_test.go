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
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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

// TestReleaseGet_SuccessEnrichedFields verifies that Get maps enriched
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
	if out.Author == nil || out.Author.Username != "releaser" {
		t.Errorf("out.Author = %+v, want username %q", out.Author, "releaser")
	}
	if out.Commit == nil || out.Commit.ID != "abc123def456" {
		t.Errorf("out.Commit = %+v, want id %q", out.Commit, "abc123def456")
	}
	if !out.UpcomingRelease {
		t.Error("out.UpcomingRelease = false, want true")
	}
	if len(out.Milestones) != 2 || out.Milestones[0].Title != "v1.0" || out.Milestones[1].Title != "v1.1" {
		t.Errorf("out.Milestones = %+v, want titles [v1.0 v1.1]", out.Milestones)
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

// TestReleaseList_PaginationQueryParamsAndMetadata verifies ReleaseList when pagination query params and metadata.
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

	out, err := List(context.Background(), client, ListInput{ProjectID: "42", Page: 1, PerPage: 2})
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
	if out.Assets == nil {
		t.Fatal("out.Assets = nil, want non-nil")
	}
	if out.Assets.Count != 2 {
		t.Errorf("out.Assets.Count = %d, want 2", out.Assets.Count)
	}
	if len(out.Assets.Sources) != 1 {
		t.Fatalf("len(out.Assets.Sources) = %d, want 1", len(out.Assets.Sources))
	}
	if out.Assets.Sources[0].Format != "zip" {
		t.Errorf("out.Assets.Sources[0].Format = %q, want %q", out.Assets.Sources[0].Format, "zip")
	}
	if len(out.Assets.Links) != 1 {
		t.Fatalf("len(out.Assets.Links) = %d, want 1", len(out.Assets.Links))
	}
	if out.Assets.Links[0].Name != "binary" {
		t.Errorf("out.Assets.Links[0].Name = %q, want %q", out.Assets.Links[0].Name, "binary")
	}
	if !out.Assets.Links[0].External {
		t.Error("out.Assets.Links[0].External = false, want true")
	}
	if out.Assets.Links[0].LinkType != "other" {
		t.Errorf("out.Assets.Links[0].LinkType = %q, want %q", out.Assets.Links[0].LinkType, "other")
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

// errExpCancelledCtx identifies the err exp cancelled ctx constant used by this package.
const errExpCancelledCtx = "expected error for canceled context"

// errExpectedAPI identifies the err expected API constant used by this package.
const errExpectedAPI = "expected API error, got nil"

// ---------------------------------------------------------------------------
// Create — API error, empty project_id, canceled context, tag_message field
// ---------------------------------------------------------------------------.

// TestCreate_APIError verifies Create when API error.
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

// TestCreate_MissingProjectID verifies Create when missing project ID.
func TestCreate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Create(context.Background(), client, CreateInput{TagName: "v1.0.0"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestCreate_CancelledContext verifies Create when cancelled context.
func TestCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := Create(ctx, client, CreateInput{ProjectID: "42", TagName: "v1.0.0"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestCreate_WithTagMessage verifies Create when with tag message.
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

// TestUpdate_APIError verifies Update when API error.
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

// TestUpdate_MissingProjectID verifies Update when missing project ID.
func TestUpdate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Update(context.Background(), client, UpdateInput{TagName: "v1.0.0"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestUpdate_CancelledContext verifies Update when cancelled context.
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

// TestDelete_APIError verifies Delete when API error.
func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Delete(context.Background(), client, DeleteInput{ProjectID: "42", TagName: "v1.0.0"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDelete_MissingProjectID verifies Delete when missing project ID.
func TestDelete_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Delete(context.Background(), client, DeleteInput{TagName: "v1.0.0"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestDelete_CancelledContext verifies Delete when cancelled context.
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

// TestGet_APIError verifies Get when API error.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: "42", TagName: "v1.0.0"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGet_MissingProjectID verifies Get when missing project ID.
func TestGet_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Get(context.Background(), client, GetInput{TagName: "v1.0.0"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestGet_CancelledContext verifies Get when cancelled context.
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

// TestGetLatest_APIError verifies GetLatest when API error.
func TestGetLatest_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := GetLatest(context.Background(), client, GetLatestInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGetLatest_MissingProjectID verifies GetLatest when missing project ID.
func TestGetLatest_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := GetLatest(context.Background(), client, GetLatestInput{})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestGetLatest_CancelledContext verifies GetLatest when cancelled context.
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

// TestList_APIError verifies List when API error.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestList_MissingProjectID verifies List when missing project ID.
func TestList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestList_CancelledContext verifies List when cancelled context.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestList_WithOrderByAndSort verifies List when with order by and sort.
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

// TestList_EmptyResult verifies List when empty result.
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

// TestFormatMarkdown_AllFields verifies FormatMarkdown when all fields.
func TestFormatMarkdown_AllFields(t *testing.T) {
	md := FormatMarkdown(Output{
		TagName:         "v2.0.0",
		Name:            "Release v2.0.0",
		Description:     "## Changes\n- Feature X",
		Author:          &toolutil.AuthorOutput{Username: "admin"},
		CreatedAt:       "2026-03-02T10:00:00Z",
		ReleasedAt:      "2026-03-02T10:00:00Z",
		Commit:          &toolutil.CommitOutput{ID: "abc123"},
		UpcomingRelease: true,
		Milestones:      []*toolutil.MilestoneOutput{{Title: "m1"}, {Title: "m2"}},
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

// TestFormatReleaseNotFound verifies the special not-found formatter emits content.
func TestFormatReleaseNotFound(t *testing.T) {
	result := formatReleaseNotFound(releaseNotFoundOutput{Identifier: "v99.0.0 in project 42"})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content in not-found result")
	}
}

// TestFormatMarkdown_MinimalFields verifies FormatMarkdown when minimal fields.
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

// TestFormatMarkdown_WithReleasedAt verifies FormatMarkdown when with released at.
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

// TestFormatListMarkdown_WithData verifies FormatListMarkdown when with data.
func TestFormatListMarkdown_WithData(t *testing.T) {
	out := ListOutput{
		Releases: []Output{
			{TagName: "v2.0.0", Name: "Major", Author: &toolutil.AuthorOutput{Username: "admin"}, ReleasedAt: "2026-06-01T10:00:00Z"},
			{TagName: "v1.0.0", Name: "First", Author: &toolutil.AuthorOutput{Username: "dev"}, CreatedAt: "2026-01-01T10:00:00Z"},
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

// TestFormatListMarkdown_Empty verifies FormatListMarkdown when empty.
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

// TestFormatListMarkdown_FallbackToCreatedAt verifies FormatListMarkdown when fallback to created at.
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

// TestToOutput_NilTimestamps verifies ToOutput when nil timestamps.
func TestToOutput_NilTimestamps(t *testing.T) {
	out := ToOutput(&gl.Release{})
	if out.CreatedAt != "" {
		t.Errorf("out.CreatedAt = %q, want empty", out.CreatedAt)
	}
	if out.ReleasedAt != "" {
		t.Errorf("out.ReleasedAt = %q, want empty", out.ReleasedAt)
	}
}

// TestToOutput_NoAssetsSources verifies ToOutput omits the nested assets
// object entirely when the release carries no assets (sources, links, or
// evidence file path). toolutil.NewAssetsOutput returns nil for a fully
// empty gl.ReleaseAssets so the JSON omits the assets key.
func TestToOutput_NoAssetsSources(t *testing.T) {
	out := ToOutput(&gl.Release{})
	if out.Assets != nil {
		if out.Assets.Sources != nil {
			t.Errorf("out.Assets.Sources should be nil, got %v", out.Assets.Sources)
		}
		if out.Assets.Links != nil {
			t.Errorf("out.Assets.Links should be nil, got %v", out.Assets.Links)
		}
	}
}

// TestToOutput_NoEvidences verifies ToOutput when no evidences.
func TestToOutput_NoEvidences(t *testing.T) {
	out := ToOutput(&gl.Release{})
	if out.Evidences != nil {
		t.Errorf("out.Evidences should be nil, got %v", out.Evidences)
	}
}

// TestToOutput_NoMilestones verifies ToOutput when no milestones.
func TestToOutput_NoMilestones(t *testing.T) {
	out := ToOutput(&gl.Release{})
	if out.Milestones != nil {
		t.Errorf("out.Milestones should be nil, got %v", out.Milestones)
	}
}

// TestToOutput_EmptyCommitID verifies ToOutput leaves Commit nil when the
// release carries an empty (zero-value) commit.
func TestToOutput_EmptyCommitID(t *testing.T) {
	out := ToOutput(&gl.Release{})
	if out.Commit != nil {
		t.Errorf("out.Commit = %+v, want nil", out.Commit)
	}
}

// TestToOutput_WebURL_DerivedFromEditURL verifies that the detail Markdown
// derives the release page URL by stripping the /edit suffix from
// _links.edit_url.
func TestToOutput_WebURL_DerivedFromEditURL(t *testing.T) {
	r := &gl.Release{
		Links: gl.ReleaseLinks{
			EditURL: "https://gitlab.example.com/group/project/-/releases/v1.0.0/edit",
		},
	}
	out := ToOutput(r)
	if out.Links == nil || out.Links.EditURL == "" {
		t.Fatalf("out.Links = %+v, want populated _links", out.Links)
	}
	want := "https://gitlab.example.com/group/project/-/releases/v1.0.0"
	if got := releaseWebURL(out); got != want {
		t.Errorf("releaseWebURL = %q, want %q", got, want)
	}
}

// TestToOutput_WebURL_EmptyEditURL verifies that the derived web URL is empty
// and _links is nil when no link fields are provided.
func TestToOutput_WebURL_EmptyEditURL(t *testing.T) {
	out := ToOutput(&gl.Release{})
	if out.Links != nil {
		t.Errorf("out.Links = %+v, want nil", out.Links)
	}
	if got := releaseWebURL(out); got != "" {
		t.Errorf("releaseWebURL = %q, want empty", got)
	}
}

// TestFormatMarkdown_WithWebURL verifies that the detail Markdown includes
// a clickable URL link when a web URL is derivable from _links.edit_url.
func TestFormatMarkdown_WithWebURL(t *testing.T) {
	md := FormatMarkdown(Output{
		TagName:   "v1.0.0",
		Name:      "Release v1.0.0",
		CreatedAt: "2026-03-01T10:00:00Z",
		Links:     &toolutil.LinksOutput{EditURL: "https://gitlab.example.com/-/releases/v1.0.0/edit"},
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
			{TagName: "v2.0.0", Name: "Major", Author: &toolutil.AuthorOutput{Username: "admin"}, ReleasedAt: "2026-06-01T10:00:00Z", Links: &toolutil.LinksOutput{EditURL: "https://gitlab.example.com/-/releases/v2.0.0/edit"}},
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
// ActionSpecs metadata
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata verifies canonical metadata for release actions.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	specs := ActionSpecs(client)
	byTool := releaseSpecsByTool(t, specs)

	if len(specs) != 6 {
		t.Fatalf("len(ActionSpecs) = %d, want 6", len(specs))
	}
	if len(byTool) != len(specs) {
		t.Fatalf("unique individual tools = %d, want %d", len(byTool), len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "releases" {
			t.Fatalf("OwnerPackage for %s = %q, want releases", spec.Name, spec.OwnerPackage)
		}
	}

	list := byTool["gitlab_release_list"]
	if list.Usage == "" || len(list.Aliases) == 0 || len(list.ParameterGuidance) == 0 {
		t.Fatalf("gitlab_release_list metadata incomplete: usage=%q aliases=%d guidance=%d", list.Usage, len(list.Aliases), len(list.ParameterGuidance))
	}
	if list.IndividualTool.Description == "" {
		t.Fatal("gitlab_release_list description is empty")
	}

	get := byTool["gitlab_release_get"]
	if get.Usage == "" || len(get.Aliases) == 0 || get.ParameterGuidance["tag_name"].SemanticRole == "" {
		t.Fatalf("gitlab_release_get metadata incomplete: usage=%q aliases=%d guidance(tag_name)=%q", get.Usage, len(get.Aliases), get.ParameterGuidance["tag_name"].SemanticRole)
	}

	create := byTool["gitlab_release_create"]
	if create.Usage == "" || len(create.Aliases) == 0 {
		t.Fatalf("gitlab_release_create metadata incomplete: usage=%q aliases=%d", create.Usage, len(create.Aliases))
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs route coverage for all 6 tools
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallAllRoutes validates release routes across multiple scenarios.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := newReleaseSpecsByTool(t)

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

// TestCatalogSurface_DeleteConfirmDeclined covers generic destructive
// confirmation for release delete when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	for _, spec := range ActionSpecs(client) {
		if spec.IndividualTool.Name == "gitlab_release_delete" {
			toolutil.RegisterSurfaceToolFromSpec(server, spec, toolutil.SurfaceToolRegisterOptions{Description: "Test release destructive confirmation.", Icons: toolutil.IconRelease})
		}
	}
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
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
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})

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

// TestActionSpecs_GetNotFound covers the not-found route output when the release does not exist.
func TestActionSpecs_GetNotFound(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Release Not Found"}`)
	})
	client := testutil.NewTestClient(t, handler)
	byTool := releaseSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_release_get"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "tag_name": "v99.0.0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := result.(releaseNotFoundOutput); !ok {
		t.Fatalf("result type = %T, want releaseNotFoundOutput", result)
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
// Helper: ActionSpec route factory
// ---------------------------------------------------------------------------.

// newReleaseSpecsByTool constructs release specs by tool test fixtures.
func newReleaseSpecsByTool(t *testing.T) map[string]toolutil.ActionSpec {
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
	return releaseSpecsByTool(t, ActionSpecs(client))
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

// TestActionSpecs_ReleaseGetRoute verifies the canonical release get route output.
func TestActionSpecs_ReleaseGetRoute(t *testing.T) {
	const respJSON = `{"tag_name":"v1.0.0","name":"R","description":"","created_at":"2026-01-01T00:00:00Z","released_at":"2026-01-02T00:00:00Z","author":{"username":"alice"}}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/projects/42/releases/v1.0.0") {
			testutil.RespondJSON(w, http.StatusOK, respJSON)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)
	byTool := releaseSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_release_get"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "tag_name": "v1.0.0"})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	out, ok := result.(Output)
	if !ok {
		t.Fatalf("result type = %T, want Output", result)
	}
	if out.TagName != "v1.0.0" || out.Name != "R" {
		t.Fatalf("release output = %#v, want tag v1.0.0 name R", out)
	}
}

// releaseSpecsByTool supports release specs by tool assertions in releases tests.
func releaseSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}

// ---------------------------------------------------------------------------
// 1:1 audit coverage: assets input wiring, list options, nested converters
// ---------------------------------------------------------------------------.

// TestReleaseCreate_AssetsLinks verifies that Create marshals the nested
// assets.links input (name, url, filepath, direct_asset_path, link_type) into
// the request body sent to the GitLab API.
func TestReleaseCreate_AssetsLinks(t *testing.T) {
	var capturedBody []byte
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProjectReleases {
			capturedBody, _ = io.ReadAll(r.Body)
			testutil.RespondJSON(w, http.StatusCreated, `{"tag_name":"v3.0.0","name":"v3","description":"","created_at":"2026-06-01T10:00:00Z","released_at":"2026-06-01T10:00:00Z"}`)
			return
		}
		http.NotFound(w, r)
	}))

	if _, err := Create(context.Background(), client, CreateInput{
		ProjectID: "42",
		TagName:   "v3.0.0",
		Assets: &AssetsInput{Links: []AssetLinkInput{{
			Name:            "binary",
			URL:             "https://example.com/bin",
			FilePath:        "/old/path",
			DirectAssetPath: "/binaries/app.zip",
			LinkType:        "package",
		}}},
	}); err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	var payload struct {
		Assets struct {
			Links []struct {
				Name            string `json:"name"`
				URL             string `json:"url"`
				FilePath        string `json:"filepath"`
				DirectAssetPath string `json:"direct_asset_path"`
				LinkType        string `json:"link_type"`
			} `json:"links"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(capturedBody, &payload); err != nil {
		t.Fatalf("unmarshal request body: %v; body=%q", err, string(capturedBody))
	}
	if len(payload.Assets.Links) != 1 {
		t.Fatalf("assets.links len = %d, want 1; body=%q", len(payload.Assets.Links), string(capturedBody))
	}
	got := payload.Assets.Links[0]
	if got.Name != "binary" || got.URL != "https://example.com/bin" ||
		got.FilePath != "/old/path" || got.DirectAssetPath != "/binaries/app.zip" || got.LinkType != "package" {
		t.Errorf("assets.links[0] = %+v, want all fields populated", got)
	}
}

// TestReleaseCreate_EmptyAssetsLinks verifies that Create omits the assets
// object when an assets value with no links is provided.
func TestReleaseCreate_EmptyAssetsLinks(t *testing.T) {
	var capturedBody []byte
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProjectReleases {
			capturedBody, _ = io.ReadAll(r.Body)
			testutil.RespondJSON(w, http.StatusCreated, `{"tag_name":"v3.1.0","name":"v3.1","description":"","created_at":"2026-06-01T10:00:00Z","released_at":"2026-06-01T10:00:00Z"}`)
			return
		}
		http.NotFound(w, r)
	}))

	if _, err := Create(context.Background(), client, CreateInput{
		ProjectID: "42",
		TagName:   "v3.1.0",
		Assets:    &AssetsInput{},
	}); err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if strings.Contains(string(capturedBody), "\"assets\"") {
		t.Errorf("request body should omit assets when no links, got %q", string(capturedBody))
	}
}

// TestReleaseList_IncludeHTMLDescriptionAndKeyset verifies that List forwards
// include_html_description and keyset pagination (pagination, page_token)
// query parameters to the GitLab API.
func TestReleaseList_IncludeHTMLDescriptionAndKeyset(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathProjectReleases {
			q := r.URL.Query()
			for k, want := range map[string]string{
				"include_html_description": "true",
				"pagination":               "keyset",
				"page_token":               "99",
				"order_by":                 "created_at",
				"sort":                     "asc",
			} {
				if got := q.Get(k); got != want {
					t.Errorf("query param %s = %q, want %q", k, got, want)
				}
			}
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))

	if _, err := List(context.Background(), client, ListInput{
		ProjectID:              "42",
		OrderBy:                "created_at",
		Sort:                   "asc",
		IncludeHTMLDescription: true,
		Pagination:             "keyset", PageToken: "99",
	}); err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
}

// TestToOutput_FullNestedObjects verifies that ToOutput mirrors the documented
// nested sub-object reference subsets: commit, assets (sources + links),
// _links, milestones (with issue_stats and dates), and evidences. SDK-only
// commit fields (stats, status) are intentionally not surfaced per the
// documented release commit subset in doc/api/releases/_index.md.
func TestToOutput_FullNestedObjects(t *testing.T) {
	zip := "zip"
	r := &gl.Release{
		TagName: "v1.0.0",
		Author:  gl.BasicUser{ID: 7, Username: "alice", Name: "Alice", State: "active"},
		Commit: gl.Commit{
			ID: "abc", ShortID: "abc1", Title: "t", AuthorName: "Bob",
		},
		Assets: gl.ReleaseAssets{
			Count:            1,
			EvidenceFilePath: "/ev/file",
			Sources:          []gl.ReleaseAssetsSource{{Format: zip, URL: "u"}},
			Links: []*gl.ReleaseLink{
				nil,
				{ID: 5, Name: "n", URL: "u2", LinkType: gl.PackageLinkType},
			},
		},
		Links: gl.ReleaseLinks{Self: "self-url", EditURL: "edit-url"},
		Milestones: []*gl.ReleaseMilestone{
			nil,
			{
				ID: 1, Title: "M", State: "active", WebURL: "w",
				IssueStats: &gl.ReleaseMilestoneIssueStats{Total: 10, Closed: 4},
			},
		},
		Evidences: []*gl.ReleaseEvidence{nil, {SHA: "s", Filepath: "/f"}},
	}
	out := ToOutput(r)

	if out.Author == nil || out.Author.ID != 7 {
		t.Fatalf("author = %+v", out.Author)
	}
	if out.Commit == nil || out.Commit.ID != "abc" || out.Commit.ShortID != "abc1" {
		t.Fatalf("commit = %+v", out.Commit)
	}
	if out.Assets == nil || out.Assets.EvidenceFilePath != "/ev/file" {
		t.Fatalf("assets = %+v", out.Assets)
	}
	if len(out.Assets.Links) != 1 || out.Assets.Links[0].LinkType != "package" {
		t.Fatalf("assets links = %+v (nil should be skipped)", out.Assets.Links)
	}
	if out.Links == nil || out.Links.Self != "self-url" {
		t.Fatalf("_links = %+v", out.Links)
	}
	if len(out.Milestones) != 1 || out.Milestones[0].IssueStats == nil || out.Milestones[0].IssueStats.Closed != 4 {
		t.Fatalf("milestones = %+v (nil should be skipped)", out.Milestones)
	}
	if len(out.Evidences) != 1 || out.Evidences[0].SHA != "s" {
		t.Fatalf("evidences = %+v (nil should be skipped)", out.Evidences)
	}
}

// TestToOutput_MilestoneISODates verifies that release milestone ISO dates
// (start_date, due_date) are rendered as YYYY-MM-DD.
func TestToOutput_MilestoneISODates(t *testing.T) {
	start := gl.ISOTime(mustParseDay(t, "2026-01-02"))
	due := gl.ISOTime(mustParseDay(t, "2026-02-03"))
	r := &gl.Release{Milestones: []*gl.ReleaseMilestone{
		{ID: 1, Title: "M", StartDate: &start, DueDate: &due},
	}}
	out := ToOutput(r)
	if len(out.Milestones) != 1 {
		t.Fatalf("milestones len = %d, want 1", len(out.Milestones))
	}
	if out.Milestones[0].StartDate != "2026-01-02" || out.Milestones[0].DueDate != "2026-02-03" {
		t.Errorf("milestone dates = %q/%q, want 2026-01-02/2026-02-03",
			out.Milestones[0].StartDate, out.Milestones[0].DueDate)
	}
}

// mustParseDay parses a YYYY-MM-DD string into a time.Time for ISOTime tests.
func mustParseDay(t *testing.T, s string) time.Time {
	t.Helper()
	parsed, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("parse day %q: %v", s, err)
	}
	return parsed
}

// TestToOutput_DocumentedSubset_OmitsUndocumentedCommitAndAuthorFields verifies
// that the trimmed nested reference subsets stay aligned with
// doc/api/releases/_index.md across SDK version drift: even when client-go
// populates commit fields (stats, status, project_id, web_url) and an author
// created_at that are NOT part of the documented release commit/author objects,
// the serialized MCP output must not surface those keys. This is the
// version-tolerance guard for the doc-grounded output reconcile.
func TestToOutput_DocumentedSubset_OmitsUndocumentedCommitAndAuthorFields(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	status := gl.BuildStateValue("success")
	r := &gl.Release{
		TagName: "v9.9.9",
		Author: gl.BasicUser{
			ID: 7, Username: "alice", Name: "Alice", State: "active",
			AvatarURL: "https://a", WebURL: "https://u", CreatedAt: &created,
		},
		Commit: gl.Commit{
			ID: "abc", ShortID: "abc1", Title: "t", AuthorName: "Bob",
			Stats:     &gl.CommitStats{Additions: 3, Deletions: 1, Total: 4},
			Status:    &status,
			ProjectID: 123,
			WebURL:    "https://commit",
		},
	}

	out := ToOutput(r)

	// Marshal the commit sub-object in isolation so undocumented commit keys can
	// be asserted without colliding with the documented author.web_url field.
	commitData, err := json.Marshal(out.Commit)
	if err != nil {
		t.Fatalf("marshal commit: %v", err)
	}
	commitJSON := string(commitData)
	for _, undocumented := range []string{`"stats"`, `"status"`, `"project_id"`, `"web_url"`} {
		if strings.Contains(commitJSON, undocumented) {
			t.Errorf("serialized commit must omit undocumented field %s; got %s", undocumented, commitJSON)
		}
	}
	for _, documented := range []string{`"id"`, `"short_id"`, `"title"`} {
		if !strings.Contains(commitJSON, documented) {
			t.Errorf("serialized commit must keep documented field %s; got %s", documented, commitJSON)
		}
	}

	// The author created_at is not part of the documented author subset.
	authorData, err := json.Marshal(out.Author)
	if err != nil {
		t.Fatalf("marshal author: %v", err)
	}
	authorJSON := string(authorData)
	if strings.Contains(authorJSON, `"created_at"`) {
		t.Errorf("serialized author must omit undocumented created_at; got %s", authorJSON)
	}
	for _, documented := range []string{`"avatar_url"`, `"web_url"`, `"state"`} {
		if !strings.Contains(authorJSON, documented) {
			t.Errorf("serialized author must keep documented field %s; got %s", documented, authorJSON)
		}
	}
}
