// release_links_test.go contains unit tests for GitLab release asset link
// operations (create, delete, list). Tests use httptest to mock the GitLab
// Release Links API and verify both success and error paths.
package releaselinks

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Test endpoint path for release asset link operations.
const (
	errNoReachAPI       = "should not reach API"
	pathReleaseLinks    = "/api/v4/projects/42/releases/v1.2.0/assets/links"
	pathReleaseLinkByID = "/api/v4/projects/42/releases/v1.2.0/assets/links/10"
	testTagV120         = "v1.2.0"
	testBinaryAmd64     = "Binary amd64"
	testUpdatedBinary   = "Updated Binary"
	testLinkID          = "link_id"
	fmtWantID10         = "out.ID = %d, want 10"
	fmtErrWantContain   = "error = %q, want it to contain %q"
)

// TestReleaseLinkCreate_Success verifies that Create correctly adds
// an asset link with the specified name, URL, and link type. The mock returns
// a 201 response and the test asserts the output ID and link type match.
func TestReleaseLinkCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathReleaseLinks {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":10,"name":"Binary amd64","url":"https://example.com/bin/amd64","link_type":"package","external":true}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID: "42",
		TagName:   testTagV120,
		Name:      testBinaryAmd64,
		URL:       "https://example.com/bin/amd64",
		LinkType:  "package",
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.ID != 10 {
		t.Errorf(fmtWantID10, out.ID)
	}
	if out.LinkType != "package" {
		t.Errorf("out.LinkType = %q, want %q", out.LinkType, "package")
	}
}

// TestReleaseLinkCreate_MissingRelease verifies that Create returns
// an error when the specified release does not exist. The mock returns a 404.
func TestReleaseLinkCreate_MissingRelease(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"Release Not Found"}`)
	}))

	_, err := Create(context.Background(), client, CreateInput{
		ProjectID: "42",
		TagName:   "nonexistent",
		Name:      "link",
		URL:       "https://example.com",
	})
	if err == nil {
		t.Fatal("Create() expected error for missing release, got nil")
	}
}

// TestReleaseLinkDelete_Success verifies that Delete removes an
// asset link by ID and returns its details. The mock handles the DELETE
// request and the test confirms the deleted link's ID is preserved.
func TestReleaseLinkDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathReleaseLinkByID {
			testutil.RespondJSON(w, http.StatusOK, `{"id":10,"name":"Binary amd64","url":"https://example.com/bin/amd64","link_type":"package","external":true}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Delete(context.Background(), client, DeleteInput{
		ProjectID: "42",
		TagName:   testTagV120,
		LinkID:    10,
	})
	if err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}
	if out.ID != 10 {
		t.Errorf(fmtWantID10, out.ID)
	}
}

// TestReleaseLinkList_Success verifies that List returns all asset
// links for a release. The mock returns two links and the test asserts the
// output slice length.
func TestReleaseLinkList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathReleaseLinks {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":10,"name":"Binary amd64","url":"https://example.com/amd64","link_type":"package","external":true},{"id":11,"name":"Binary arm64","url":"https://example.com/arm64","link_type":"package","external":true}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: "42", TagName: testTagV120})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Links) != 2 {
		t.Errorf("len(out.Links) = %d, want 2", len(out.Links))
	}
}

// TestReleaseLinkList_PaginationQueryParamsAndMetadata verifies that
// List sends page and per_page query parameters and correctly
// parses pagination metadata from the response headers.
func TestReleaseLinkList_PaginationQueryParamsAndMetadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathReleaseLinks {
			if got := r.URL.Query().Get("page"); got != "1" {
				t.Errorf("query param page = %q, want %q", got, "1")
			}
			if got := r.URL.Query().Get("per_page"); got != "5" {
				t.Errorf("query param per_page = %q, want %q", got, "5")
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"id":10,"name":"Binary","url":"https://example.com/bin","link_type":"package","external":true}]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "5", Total: "3", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: "42", TagName: testTagV120, Page: 1, PerPage: 5})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if out.Pagination.TotalItems != 3 {
		t.Errorf("Pagination.TotalItems = %d, want 3", out.Pagination.TotalItems)
	}
	if out.Pagination.TotalPages != 1 {
		t.Errorf("Pagination.TotalPages = %d, want 1", out.Pagination.TotalPages)
	}
}

// TestReleaseLinkGet_Success verifies that Get retrieves a single release
// link by its ID, returning the correct name, URL, and type.
func TestReleaseLinkGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathReleaseLinkByID {
			testutil.RespondJSON(w, http.StatusOK, `{"id":10,"name":"Binary amd64","url":"https://example.com/bin/amd64","link_type":"package","external":true,"direct_asset_url":"https://example.com/direct"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{
		ProjectID: "42",
		TagName:   testTagV120,
		LinkID:    10,
	})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.ID != 10 {
		t.Errorf(fmtWantID10, out.ID)
	}
	if out.Name != testBinaryAmd64 {
		t.Errorf("out.Name = %q, want %q", out.Name, testBinaryAmd64)
	}
	if out.DirectAssetURL != "https://example.com/direct" {
		t.Errorf("out.DirectAssetURL = %q, want %q", out.DirectAssetURL, "https://example.com/direct")
	}
}

// TestReleaseLinkGet_NotFound verifies that Get returns an error when the
// link does not exist.
func TestReleaseLinkGet_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := Get(context.Background(), client, GetInput{
		ProjectID: "42",
		TagName:   testTagV120,
		LinkID:    999,
	})
	if err == nil {
		t.Fatal("Get() expected error for missing link, got nil")
	}
}

// TestReleaseLinkUpdate_Success verifies that Update modifies an existing
// release link's name, URL, and type.
func TestReleaseLinkUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathReleaseLinkByID {
			testutil.RespondJSON(w, http.StatusOK, `{"id":10,"name":"Updated Binary","url":"https://example.com/bin/v2","link_type":"runbook","external":false,"direct_asset_url":""}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID: "42",
		TagName:   testTagV120,
		LinkID:    10,
		Name:      testUpdatedBinary,
		URL:       "https://example.com/bin/v2",
		LinkType:  "runbook",
	})
	if err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}
	if out.ID != 10 {
		t.Errorf(fmtWantID10, out.ID)
	}
	if out.Name != testUpdatedBinary {
		t.Errorf("out.Name = %q, want %q", out.Name, testUpdatedBinary)
	}
	if out.LinkType != "runbook" {
		t.Errorf("out.LinkType = %q, want %q", out.LinkType, "runbook")
	}
}

// TestReleaseLink_GetRequiresLinkID verifies that Get returns an error
// when link_id is zero.
func TestReleaseLink_GetRequiresLinkID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := Get(context.Background(), client, GetInput{
		ProjectID: "42",
		TagName:   testTagV120,
		LinkID:    0,
	})
	if err == nil {
		t.Fatal("Get() expected error for zero link_id, got nil")
	}
	if got := err.Error(); !strings.Contains(got, testLinkID) {
		t.Errorf(fmtErrWantContain, got, testLinkID)
	}
}

// TestReleaseLink_UpdateRequiresLinkID verifies that Update returns an error
// when link_id is zero.
func TestReleaseLink_UpdateRequiresLinkID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := Update(context.Background(), client, UpdateInput{
		ProjectID: "42",
		TagName:   testTagV120,
		LinkID:    0,
	})
	if err == nil {
		t.Fatal("Update() expected error for zero link_id, got nil")
	}
	if got := err.Error(); !strings.Contains(got, testLinkID) {
		t.Errorf(fmtErrWantContain, got, testLinkID)
	}
}

// TestReleaseLink_DeleteRequiresLinkID verifies that Delete returns an error
// when link_id is zero.
func TestReleaseLink_DeleteRequiresLinkID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := Delete(context.Background(), client, DeleteInput{
		ProjectID: "42",
		TagName:   testTagV120,
		LinkID:    0,
	})
	if err == nil {
		t.Fatal("Delete() expected error for zero link_id, got nil")
	}
	if got := err.Error(); !strings.Contains(got, testLinkID) {
		t.Errorf(fmtErrWantContain, got, testLinkID)
	}
}

// TestReleaseLinkUpdate_NotFound verifies that Update returns an error
// when the link does not exist.
func TestReleaseLinkUpdate_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := Update(context.Background(), client, UpdateInput{
		ProjectID: "42",
		TagName:   testTagV120,
		LinkID:    999,
		Name:      "nope",
	})
	if err == nil {
		t.Fatal("Update() expected error for missing link, got nil")
	}
}

// ---------------------------------------------------------------------------
// CreateBatch tests
// ---------------------------------------------------------------------------.

// TestReleaseLinkCreateBatch_Success verifies that CreateBatch creates
// multiple asset links in a single call. The mock returns a 201 for each
// POST and the test asserts all links are created.
func TestReleaseLinkCreateBatch_Success(t *testing.T) {
	var callCount int
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathReleaseLinks {
			callCount++
			testutil.RespondJSON(w, http.StatusCreated, `{"id":`+strconv.Itoa(callCount)+`,"name":"link","url":"https://example.com","link_type":"package","external":true}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := CreateBatch(context.Background(), client, CreateBatchInput{
		ProjectID: "42",
		TagName:   testTagV120,
		Links: []LinkEntry{
			{Name: "Binary amd64", URL: "https://example.com/amd64", LinkType: "package"},
			{Name: "Binary arm64", URL: "https://example.com/arm64", LinkType: "package"},
			{Name: "Checksum", URL: "https://example.com/sha256", LinkType: "other"},
		},
	})
	if err != nil {
		t.Fatalf("CreateBatch() unexpected error: %v", err)
	}
	if len(out.Created) != 3 {
		t.Errorf("len(out.Created) = %d, want 3", len(out.Created))
	}
	if len(out.Failed) != 0 {
		t.Errorf("len(out.Failed) = %d, want 0", len(out.Failed))
	}
	if callCount != 3 {
		t.Errorf("API call count = %d, want 3", callCount)
	}
}

// TestReleaseLinkType_DefaultsGenericPackageURLs verifies package registry URLs
// are classified as package links unless the caller sets an explicit type.
func TestReleaseLinkType_DefaultsGenericPackageURLs(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		explicit string
		want     gl.LinkTypeValue
	}{
		{name: "generic package", url: "https://gitlab.example.com/api/v4/projects/1/packages/generic/pkg/1.0.0/bin", want: gl.PackageLinkType},
		{name: "explicit wins", url: "https://gitlab.example.com/api/v4/projects/1/packages/generic/pkg/1.0.0/bin", explicit: "other", want: gl.OtherLinkType},
		{name: "ordinary url", url: "https://example.com/bin", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := releaseLinkType(tt.url, tt.explicit); got != tt.want {
				t.Fatalf("releaseLinkType() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestReleaseLinkCreateBatch_PartialFailure verifies that CreateBatch
// continues creating links after one fails, collecting errors in Failed.
func TestReleaseLinkCreateBatch_PartialFailure(t *testing.T) {
	var callCount int
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathReleaseLinks {
			callCount++
			if callCount == 2 {
				testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"duplicate link"}`)
				return
			}
			testutil.RespondJSON(w, http.StatusCreated, `{"id":10,"name":"link","url":"https://example.com","link_type":"package","external":true}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := CreateBatch(context.Background(), client, CreateBatchInput{
		ProjectID: "42",
		TagName:   testTagV120,
		Links: []LinkEntry{
			{Name: "Link 1", URL: "https://example.com/1"},
			{Name: "Link 2", URL: "https://example.com/2"},
			{Name: "Link 3", URL: "https://example.com/3"},
		},
	})
	if err != nil {
		t.Fatalf("CreateBatch() unexpected error: %v", err)
	}
	if len(out.Created) != 2 {
		t.Errorf("len(out.Created) = %d, want 2", len(out.Created))
	}
	if len(out.Failed) != 1 {
		t.Errorf("len(out.Failed) = %d, want 1", len(out.Failed))
	}
}

// TestReleaseLinkCreateBatch_SkipsInvalidEntries verifies that entries
// missing required name or url fields are skipped with a failure message.
func TestReleaseLinkCreateBatch_SkipsInvalidEntries(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathReleaseLinks {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":10,"name":"ok","url":"https://example.com","link_type":"other","external":true}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := CreateBatch(context.Background(), client, CreateBatchInput{
		ProjectID: "42",
		TagName:   testTagV120,
		Links: []LinkEntry{
			{Name: "", URL: "https://example.com"},
			{Name: "Valid", URL: "https://example.com"},
			{Name: "NoURL", URL: ""},
		},
	})
	if err != nil {
		t.Fatalf("CreateBatch() unexpected error: %v", err)
	}
	if len(out.Created) != 1 {
		t.Errorf("len(out.Created) = %d, want 1", len(out.Created))
	}
	if len(out.Failed) != 2 {
		t.Errorf("len(out.Failed) = %d, want 2", len(out.Failed))
	}
}

// TestReleaseLinkCreateBatch_EmptyLinks verifies that CreateBatch returns
// an error when the links array is empty.
func TestReleaseLinkCreateBatch_EmptyLinks(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error(errNoReachAPI)
		http.NotFound(w, nil)
	}))

	_, err := CreateBatch(context.Background(), client, CreateBatchInput{
		ProjectID: "42",
		TagName:   testTagV120,
		Links:     []LinkEntry{},
	})
	if err == nil {
		t.Fatal("CreateBatch() expected error for empty links, got nil")
	}
}

// TestReleaseLinkCreateBatch_MissingProject verifies that CreateBatch returns
// an error when project_id is empty.
func TestReleaseLinkCreateBatch_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error(errNoReachAPI)
		http.NotFound(w, nil)
	}))

	_, err := CreateBatch(context.Background(), client, CreateBatchInput{
		TagName: testTagV120,
		Links:   []LinkEntry{{Name: "a", URL: "https://example.com"}},
	})
	if err == nil {
		t.Fatal("CreateBatch() expected error for empty project_id, got nil")
	}
}

// TestReleaseLinkCreateBatch_CancelledContext verifies that CreateBatch
// returns an error immediately for a cancelled context.
func TestReleaseLinkCreateBatch_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error(errNoReachAPI)
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)
	_, err := CreateBatch(ctx, client, CreateBatchInput{
		ProjectID: "42",
		TagName:   testTagV120,
		Links:     []LinkEntry{{Name: "a", URL: "https://example.com"}},
	})
	if err == nil {
		t.Fatal("CreateBatch() expected error for cancelled context, got nil")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpCancelledCtx identifies the err exp cancelled ctx constant used by this package.
const errExpCancelledCtx = "expected error for canceled context"

// errExpectedAPI identifies the err expected API constant used by this package.
const errExpectedAPI = "expected API error, got nil"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// Create — API error, missing project_id, canceled context, no link type
// ---------------------------------------------------------------------------.

// TestCreate_APIError verifies Create when API error.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{
		ProjectID: "42", TagName: "v1.0.0", Name: "link", URL: "https://example.com",
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCreate_MissingProjectID verifies Create when missing project ID.
func TestCreate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Create(context.Background(), client, CreateInput{
		TagName: "v1.0.0", Name: "link", URL: "https://example.com",
	})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestCreate_CancelledContext verifies Create when cancelled context.
func TestCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := Create(ctx, client, CreateInput{
		ProjectID: "42", TagName: "v1.0.0", Name: "link", URL: "https://example.com",
	})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestCreate_WithoutLinkType verifies Create when without link type.
func TestCreate_WithoutLinkType(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/releases/v1.0.0/assets/links" {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":1,"name":"Docs","url":"https://docs.example.com","link_type":"other","external":true}`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Create(context.Background(), client, CreateInput{
		ProjectID: "42", TagName: "v1.0.0", Name: "Docs", URL: "https://docs.example.com",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 1 {
		t.Errorf("out.ID = %d, want 1", out.ID)
	}
}

// ---------------------------------------------------------------------------
// Delete — API error, missing project_id, canceled context
// ---------------------------------------------------------------------------.

// TestDelete_APIError verifies Delete when API error.
func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Delete(context.Background(), client, DeleteInput{
		ProjectID: "42", TagName: "v1.0.0", LinkID: 1,
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDelete_MissingProjectID verifies Delete when missing project ID.
func TestDelete_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Delete(context.Background(), client, DeleteInput{
		TagName: "v1.0.0", LinkID: 1,
	})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestDelete_CancelledContext verifies Delete when cancelled context.
func TestDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := Delete(ctx, client, DeleteInput{
		ProjectID: "42", TagName: "v1.0.0", LinkID: 1,
	})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Get — API error, missing project_id, canceled context
// ---------------------------------------------------------------------------.

// TestGet_APIError verifies Get when API error.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Get(context.Background(), client, GetInput{
		ProjectID: "42", TagName: "v1.0.0", LinkID: 1,
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGet_MissingProjectID verifies Get when missing project ID.
func TestGet_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Get(context.Background(), client, GetInput{
		TagName: "v1.0.0", LinkID: 1,
	})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestGet_CancelledContext verifies Get when cancelled context.
func TestGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := Get(ctx, client, GetInput{
		ProjectID: "42", TagName: "v1.0.0", LinkID: 1,
	})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Update — API error, missing project_id, canceled context, all optional fields
// ---------------------------------------------------------------------------.

// TestUpdate_APIError verifies Update when API error.
func TestUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{
		ProjectID: "42", TagName: "v1.0.0", LinkID: 1, Name: "x",
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestUpdate_MissingProjectID verifies Update when missing project ID.
func TestUpdate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Update(context.Background(), client, UpdateInput{
		TagName: "v1.0.0", LinkID: 1, Name: "x",
	})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestUpdate_CancelledContext verifies Update when cancelled context.
func TestUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := Update(ctx, client, UpdateInput{
		ProjectID: "42", TagName: "v1.0.0", LinkID: 1, Name: "x",
	})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestUpdate_AllOptionalFields verifies Update when all optional fields.
func TestUpdate_AllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/projects/42/releases/v1.0.0/assets/links/10" {
			testutil.RespondJSON(w, http.StatusOK, `{"id":10,"name":"New Name","url":"https://new.example.com","link_type":"image","external":false,"direct_asset_url":"https://direct.example.com/pkg"}`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID:       "42",
		TagName:         "v1.0.0",
		LinkID:          10,
		Name:            "New Name",
		URL:             "https://new.example.com",
		FilePath:        "/binaries/linux-amd64",
		DirectAssetPath: "/direct/path",
		LinkType:        "image",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Name != "New Name" {
		t.Errorf("Name = %q, want %q", out.Name, "New Name")
	}
	if out.LinkType != "image" {
		t.Errorf("LinkType = %q, want %q", out.LinkType, "image")
	}
	if out.DirectAssetURL != "https://direct.example.com/pkg" {
		t.Errorf("DirectAssetURL = %q, want %q", out.DirectAssetURL, "https://direct.example.com/pkg")
	}
}

// ---------------------------------------------------------------------------
// List — API error, missing project_id, canceled context
// ---------------------------------------------------------------------------.

// TestList_APIError verifies List when API error.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "42", TagName: "v1.0.0"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestList_MissingProjectID verifies List when missing project ID.
func TestList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := List(context.Background(), client, ListInput{TagName: "v1.0.0"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestList_CancelledContext verifies List when cancelled context.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{ProjectID: "42", TagName: "v1.0.0"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestList_EmptyResult verifies List when empty result.
func TestList_EmptyResult(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/releases/v1.0.0/assets/links" {
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := List(context.Background(), client, ListInput{ProjectID: "42", TagName: "v1.0.0"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Links) != 0 {
		t.Errorf("len(Links) = %d, want 0", len(out.Links))
	}
}

// ---------------------------------------------------------------------------
// FormatOutputMarkdown
// ---------------------------------------------------------------------------.

// cardHints is the guidance section a link card closes with, and deletedHints
// the one the deletion card carries instead: a removed link can be neither
// updated nor deleted again.
const (
	cardHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'release.link_update' to change this link's name, URL or type\n" +
		"- Use action 'release.link_delete' to remove this link from the release\n"
	deletedHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'release.link_list' to see the links the release still has\n" +
		"- Use action 'release.link_create' to link another asset to the release\n"
)

// listHints is the guidance section the listing closes with, the preserve-links
// instruction first because the URL column carries links.
const listHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
	"- " + toolutil.HintPreserveLinks + "\n" +
	"- Use action 'release.link_get' to see one link on its own\n" +
	"- Use action 'release.link_create' to add a new release asset link\n" +
	"- Use action 'release.link_create_batch' to add several asset links in one call\n"

const linkHeader = "| ID | Name | Type | URL |\n| --- | --- | --- | --- |\n"

// TestFormatOutputMarkdown_WithData pins the whole card of an asset link:
// every field GitLab sends, both URLs as links, and the hints a link that
// still exists allows.
func TestFormatOutputMarkdown_WithData(t *testing.T) {
	got := FormatOutputMarkdown(Output{
		ID:             10,
		Name:           "Binary amd64",
		URL:            "https://example.com/bin/amd64",
		LinkType:       "package",
		DirectAssetURL: "https://direct.example.com",
	})

	want := "## Release Link: Binary amd64\n\n" +
		"- **ID**: 10\n" +
		"- **Type**: package\n" +
		"- **URL**: [https://example.com/bin/amd64](https://example.com/bin/amd64)\n" +
		"- **Direct Asset URL**: [https://direct.example.com](https://direct.example.com)\n" +
		cardHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatOutputMarkdown_Empty pins the card of a zero value: the ID row,
// which is an answer at zero, and no label with an empty value under it.
func TestFormatOutputMarkdown_Empty(t *testing.T) {
	got := FormatOutputMarkdown(Output{})

	want := "## Release Link: \n\n" +
		"- **ID**: 0\n" +
		cardHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatOutputMarkdown_LinkType pins the link type GitLab sends. It used to
// assert the `external` flag beside it, which no Grape entity has exposed
// since 16.0.
func TestFormatOutputMarkdown_LinkType(t *testing.T) {
	got := FormatOutputMarkdown(Output{
		ID:       5,
		Name:     "Runbook",
		URL:      "https://example.com/runbook",
		LinkType: "runbook",
	})

	want := "## Release Link: Runbook\n\n" +
		"- **ID**: 5\n" +
		"- **Type**: runbook\n" +
		"- **URL**: [https://example.com/runbook](https://example.com/runbook)\n" +
		cardHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatDeletedMarkdown pins what the delete action now answers with: a
// card that says the link is gone and offers the two actions that still make
// sense. Rendered as an ordinary link card, the same response used to invite
// the reader to update and delete a link that no longer existed.
func TestFormatDeletedMarkdown(t *testing.T) {
	got := FormatDeletedMarkdown(DeletedOutput{
		ID:       10,
		Name:     "Binary amd64",
		URL:      "https://example.com/bin/amd64",
		LinkType: "package",
	})

	want := "## Release Link Deleted: Binary amd64\n\n" +
		"- **ID**: 10\n" +
		"- **Type**: package\n" +
		"- **URL**: [https://example.com/bin/amd64](https://example.com/bin/amd64)\n" +
		"\nThe link is removed from the release. The file or package it pointed at is untouched.\n" +
		deletedHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithLinks pins the whole listing: the heading
// counting what GitLab reported, the URL column labeled with the URL rather
// than repeating the name beside it, and one guidance section at the end.
func TestFormatListMarkdown_WithLinks(t *testing.T) {
	got := FormatListMarkdown(ListOutput{
		Links: []Output{
			{ID: 10, Name: "Binary amd64", LinkType: "package", URL: "https://example.com/amd64"},
			{ID: 11, Name: "Binary arm64", LinkType: "package", URL: "https://example.com/arm64"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	})

	want := "## Release Links (2)\n\n" +
		linkHeader +
		"| 10 | Binary amd64 | package | [https://example.com/amd64](https://example.com/amd64) |\n" +
		"| 11 | Binary arm64 | package | [https://example.com/arm64](https://example.com/arm64) |\n" +
		"\nPage 1 of 1 | 2 items total | 20 per page\n" +
		listHints

	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdown_HeadingCountsTheResponseTotal pins the count the
// heading carries when a page is one of several: the total GitLab reported,
// not the two rows shown, which is what the heading used to say.
func TestFormatListMarkdown_HeadingCountsTheResponseTotal(t *testing.T) {
	got := FormatListMarkdown(ListOutput{
		Links: []Output{
			{ID: 10, Name: "Binary amd64", LinkType: "package", URL: "https://example.com/amd64"},
			{ID: 11, Name: "Binary arm64", LinkType: "package", URL: "https://example.com/arm64"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 45, Page: 1, PerPage: 2, TotalPages: 23},
	})

	want := "## Release Links (45)\n\n" +
		"Showing 2 of 45 results (page 1 of 23)\n\n" +
		linkHeader +
		"| 10 | Binary amd64 | package | [https://example.com/amd64](https://example.com/amd64) |\n" +
		"| 11 | Binary arm64 | package | [https://example.com/arm64](https://example.com/arm64) |\n" +
		"\nPage 1 of 23 | 45 items total | 2 per page\n" +
		listHints

	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdown_Empty pins that a release with no links renders the
// one sentence and nothing else.
func TestFormatListMarkdown_Empty(t *testing.T) {
	got := FormatListMarkdown(ListOutput{})

	if want := "No release links found.\n"; got != want {
		t.Errorf("empty list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdown_SingleLink pins the listing of a response GitLab sent
// no pagination with: the heading counts the row shown and no pagination line
// is written at all.
func TestFormatListMarkdown_SingleLink(t *testing.T) {
	got := FormatListMarkdown(ListOutput{
		Links: []Output{{ID: 1, Name: "Image", LinkType: "image", URL: "https://example.com/img"}},
	})

	want := "## Release Links (1)\n\n" +
		linkHeader +
		"| 1 | Image | image | [https://example.com/img](https://example.com/img) |\n" +
		listHints

	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatBatchMarkdown
// ---------------------------------------------------------------------------.

// TestFormatBatchMarkdown_WithCreatedAndFailed pins the whole batch result:
// the links created as a table, the ones GitLab refused under a heading of
// their own, and one guidance section closing the document.
func TestFormatBatchMarkdown_WithCreatedAndFailed(t *testing.T) {
	got := FormatBatchMarkdown(CreateBatchOutput{
		Created: []Output{
			{ID: 1, Name: "Binary amd64", LinkType: "package", URL: "https://example.com/amd64"},
			{ID: 2, Name: "Binary arm64", LinkType: "package", URL: "https://example.com/arm64"},
		},
		Failed: []string{"checksum.txt: 409 Conflict"},
	})

	want := "## Release Links Created (2)\n\n" +
		linkHeader +
		"| 1 | Binary amd64 | package | [https://example.com/amd64](https://example.com/amd64) |\n" +
		"| 2 | Binary arm64 | package | [https://example.com/arm64](https://example.com/arm64) |\n" +
		"\n### Failures (1)\n\n" +
		"- checksum.txt: 409 Conflict\n" +
		"\n---\n\U0001F4A1 **Next steps:**\n" +
		"- " + toolutil.HintPreserveLinks + "\n" +
		"- Use action 'release.link_list' to see every link the release now has\n"

	if got != want {
		t.Errorf("batch mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatBatchMarkdown_AllCreated pins the result of a batch GitLab
// accepted whole: no failures heading under it.
func TestFormatBatchMarkdown_AllCreated(t *testing.T) {
	got := FormatBatchMarkdown(CreateBatchOutput{
		Created: []Output{{ID: 5, Name: "Source", LinkType: "other", URL: "https://example.com/src"}},
	})

	want := "## Release Links Created (1)\n\n" +
		linkHeader +
		"| 5 | Source | other | [https://example.com/src](https://example.com/src) |\n" +
		"\n---\n\U0001F4A1 **Next steps:**\n" +
		"- " + toolutil.HintPreserveLinks + "\n" +
		"- Use action 'release.link_list' to see every link the release now has\n"

	if got != want {
		t.Errorf("batch mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatBatchMarkdown_Empty pins the result of a batch that created
// nothing: no table, and no instruction to preserve links a render with none
// cannot have.
func TestFormatBatchMarkdown_Empty(t *testing.T) {
	got := FormatBatchMarkdown(CreateBatchOutput{Created: []Output{}})

	want := "## Release Links Created (0)\n" +
		"\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'release.link_list' to see every link the release now has\n"

	if got != want {
		t.Errorf("batch mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// ---------------------------------------------------------------------------
// ToOutput
// ---------------------------------------------------------------------------.

// TestToOutput_AllFields verifies ToOutput when all fields.
func TestToOutput_AllFields(t *testing.T) {
	rl := mockReleaseLink(20, "Pkg", "https://example.com/pkg", "package", true, "https://direct.example.com")
	out := ToOutput(&rl)
	if out.ID != 20 {
		t.Errorf("ID = %d, want 20", out.ID)
	}
	if out.Name != "Pkg" {
		t.Errorf("Name = %q, want %q", out.Name, "Pkg")
	}
	if out.URL != "https://example.com/pkg" {
		t.Errorf("URL = %q, want %q", out.URL, "https://example.com/pkg")
	}
	if out.LinkType != "package" {
		t.Errorf("LinkType = %q, want %q", out.LinkType, "package")
	}
	if out.DirectAssetURL != "https://direct.example.com" {
		t.Errorf("DirectAssetURL = %q, want %q", out.DirectAssetURL, "https://direct.example.com")
	}
}

// TestToOutput_ZeroValue verifies ToOutput when zero value.
func TestToOutput_ZeroValue(t *testing.T) {
	rl := mockReleaseLink(0, "", "", "", false, "")
	out := ToOutput(&rl)
	if out.ID != 0 {
		t.Errorf("ID = %d, want 0", out.ID)
	}
	if out.Name != "" {
		t.Errorf("Name = %q, want empty", out.Name)
	}
}

// TestCreateBatch_EmptyTagName verifies CreateBatch returns error when tag_name is empty.
func TestCreateBatch_EmptyTagName(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := CreateBatch(context.Background(), client, CreateBatchInput{
		ProjectID: "42",
		TagName:   "",
		Links:     []LinkEntry{{Name: "x", URL: "https://example.com"}},
	})
	if err == nil {
		t.Fatal("expected error for empty tag_name, got nil")
	}
}

// TestCreateBatch_ContextCancelledMidLoop verifies CreateBatch respects context
// cancellation between link iterations. The context is cancelled by the mock
// handler after the first API call, so the second link triggers ctx.Err().
func TestCreateBatch_ContextCancelledMidLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		testutil.RespondJSON(w, http.StatusCreated, `{"id":1,"name":"a","url":"https://a.com"}`)
		if calls >= 1 {
			cancel()
		}
	}))
	out, err := CreateBatch(ctx, client, CreateBatchInput{
		ProjectID: "42",
		TagName:   "v1.0.0",
		Links: []LinkEntry{
			{Name: "a", URL: "https://a.com"},
			{Name: "b", URL: "https://b.com"},
		},
	})
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
	// First link may or may not succeed depending on timing; the important
	// thing is the loop's ctx.Err() check between iterations triggers.
	_ = out
}

// mockReleaseLink builds a minimal gl.ReleaseLink for unit tests.
func mockReleaseLink(id int64, name, url, linkType string, external bool, directURL string) gl.ReleaseLink {
	return gl.ReleaseLink{
		ID:             id,
		Name:           name,
		URL:            url,
		LinkType:       gl.LinkTypeValue(linkType),
		External:       external,
		DirectAssetURL: directURL,
	}
}

// TestReleaseLinkCreate_DirectAssetPathAndFilePath verifies that Create
// forwards direct_asset_path and the deprecated filepath to the GitLab API.
func TestReleaseLinkCreate_DirectAssetPathAndFilePath(t *testing.T) {
	var body string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathReleaseLinks {
			b, _ := io.ReadAll(r.Body)
			body = string(b)
			testutil.RespondJSON(w, http.StatusCreated, `{"id":10,"name":"Binary","url":"https://example.com/bin","link_type":"other","external":false,"direct_asset_url":"https://example.com/x/releases/v1.2.0/downloads/bin"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:       "42",
		TagName:         testTagV120,
		Name:            "Binary",
		URL:             "https://example.com/bin",
		DirectAssetPath: "/downloads/bin",
		FilePath:        "/legacy/bin",
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.ID != 10 {
		t.Errorf(fmtWantID10, out.ID)
	}
	if !strings.Contains(body, "direct_asset_path") || !strings.Contains(body, "/downloads/bin") {
		t.Errorf("request body = %q, want direct_asset_path", body)
	}
	if !strings.Contains(body, "filepath") || !strings.Contains(body, "/legacy/bin") {
		t.Errorf("request body = %q, want filepath", body)
	}
}

// TestReleaseLinkCreateBatch_DirectAssetPathAndFilePath verifies that
// CreateBatch forwards per-entry direct_asset_path and filepath.
func TestReleaseLinkCreateBatch_DirectAssetPathAndFilePath(t *testing.T) {
	var bodies []string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathReleaseLinks {
			b, _ := io.ReadAll(r.Body)
			bodies = append(bodies, string(b))
			testutil.RespondJSON(w, http.StatusCreated, `{"id":1,"name":"link","url":"https://example.com","link_type":"package","external":true}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := CreateBatch(context.Background(), client, CreateBatchInput{
		ProjectID: "42",
		TagName:   testTagV120,
		Links: []LinkEntry{
			{Name: "a", URL: "https://a.com", DirectAssetPath: "/dap/a"},
			{Name: "b", URL: "https://b.com", FilePath: "/fp/b"},
		},
	})
	if err != nil {
		t.Fatalf("CreateBatch() unexpected error: %v", err)
	}
	if len(out.Created) != 2 {
		t.Fatalf("len(out.Created) = %d, want 2", len(out.Created))
	}
	if len(bodies) != 2 {
		t.Fatalf("len(bodies) = %d, want 2", len(bodies))
	}
	if !strings.Contains(bodies[0], "direct_asset_path") || !strings.Contains(bodies[0], "/dap/a") {
		t.Errorf("body[0] = %q, want direct_asset_path", bodies[0])
	}
	if !strings.Contains(bodies[1], "filepath") || !strings.Contains(bodies[1], "/fp/b") {
		t.Errorf("body[1] = %q, want filepath", bodies[1])
	}
}

// TestReleaseLinkList_KeysetAndOrdering verifies that List forwards keyset
// pagination, page_token, order_by, and sort query parameters.
func TestReleaseLinkList_KeysetAndOrdering(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathReleaseLinks {
			q := r.URL.Query()
			if got := q.Get("pagination"); got != "keyset" {
				t.Errorf("pagination = %q, want keyset", got)
			}
			if got := q.Get("page_token"); got != "abc123" {
				t.Errorf("page_token = %q, want abc123", got)
			}
			if got := q.Get("order_by"); got != "created_at" {
				t.Errorf("order_by = %q, want created_at", got)
			}
			if got := q.Get("sort"); got != "desc" {
				t.Errorf("sort = %q, want desc", got)
			}
			testutil.RespondJSON(w, http.StatusOK, `[{"id":10,"name":"Binary","url":"https://example.com/bin","link_type":"package","external":true}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID:  "42",
		TagName:    testTagV120,
		OrderBy:    "created_at",
		Sort:       "desc",
		Pagination: "keyset", PageToken: "abc123",
	})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Links) != 1 {
		t.Errorf("len(out.Links) = %d, want 1", len(out.Links))
	}
}

// TestReleaseLinkDescriptions_RMeta verifies every release-link action exposes
// a "Returns: … See also: …" individual-tool description (R-META; 1:1 audit).
func TestReleaseLinkDescriptions_RMeta(t *testing.T) {
	byTool := releaseLinkSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, releaseLinksActionHandler())))
	for _, name := range []string{
		"gitlab_release_link_create",
		"gitlab_release_link_create_batch",
		"gitlab_release_link_get",
		"gitlab_release_link_list",
		"gitlab_release_link_update",
		"gitlab_release_link_delete",
	} {
		t.Run(name, func(t *testing.T) {
			desc := byTool[name].IndividualTool.Description
			if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
				t.Errorf("%s description = %q, want Returns:/See also: form", name, desc)
			}
		})
	}
}
