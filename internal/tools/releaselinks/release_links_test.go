// release_links_test.go contains unit tests for GitLab release asset link
// operations (create, delete, list). Tests use httptest to mock the GitLab
// Release Links API and verify both success and error paths.
package releaselinks

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"
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

	out, err := List(context.Background(), client, ListInput{ProjectID: "42", TagName: testTagV120, PaginationInput: toolutil.PaginationInput{Page: 1, PerPage: 5}})
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
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

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
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

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
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

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
			testutil.RespondJSON(w, http.StatusCreated, `{"id":`+string(rune('0'+callCount))+`,"name":"link","url":"https://example.com","link_type":"package","external":true}`)
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

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
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

const errExpCancelledCtx = "expected error for canceled context"

const errExpectedAPI = "expected API error, got nil"

const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// Create — API error, missing project_id, canceled context, no link type
// ---------------------------------------------------------------------------.

// TestCreate_APIError verifies the behavior of create a p i error.
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

// TestCreate_MissingProjectID verifies the behavior of create missing project i d.
func TestCreate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Create(context.Background(), client, CreateInput{
		TagName: "v1.0.0", Name: "link", URL: "https://example.com",
	})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestCreate_CancelledContext verifies the behavior of create cancelled context.
func TestCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Create(ctx, client, CreateInput{
		ProjectID: "42", TagName: "v1.0.0", Name: "link", URL: "https://example.com",
	})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestCreate_WithoutLinkType verifies the behavior of create without link type.
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

// TestDelete_APIError verifies the behavior of delete a p i error.
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

// TestDelete_MissingProjectID verifies the behavior of delete missing project i d.
func TestDelete_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Delete(context.Background(), client, DeleteInput{
		TagName: "v1.0.0", LinkID: 1,
	})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestDelete_CancelledContext verifies the behavior of delete cancelled context.
func TestDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
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

// TestGet_APIError verifies the behavior of get a p i error.
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

// TestGet_MissingProjectID verifies the behavior of get missing project i d.
func TestGet_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Get(context.Background(), client, GetInput{
		TagName: "v1.0.0", LinkID: 1,
	})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestGet_CancelledContext verifies the behavior of get cancelled context.
func TestGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
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

// TestUpdate_APIError verifies the behavior of update a p i error.
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

// TestUpdate_MissingProjectID verifies the behavior of update missing project i d.
func TestUpdate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Update(context.Background(), client, UpdateInput{
		TagName: "v1.0.0", LinkID: 1, Name: "x",
	})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestUpdate_CancelledContext verifies the behavior of update cancelled context.
func TestUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Update(ctx, client, UpdateInput{
		ProjectID: "42", TagName: "v1.0.0", LinkID: 1, Name: "x",
	})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestUpdate_AllOptionalFields verifies the behavior of update all optional fields.
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

// TestList_APIError verifies the behavior of list a p i error.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "42", TagName: "v1.0.0"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestList_MissingProjectID verifies the behavior of list missing project i d.
func TestList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := List(context.Background(), client, ListInput{TagName: "v1.0.0"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestList_CancelledContext verifies the behavior of list cancelled context.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := List(ctx, client, ListInput{ProjectID: "42", TagName: "v1.0.0"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestList_EmptyResult verifies the behavior of list empty result.
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

// TestFormatOutputMarkdown_WithData verifies the behavior of format output markdown with data.
func TestFormatOutputMarkdown_WithData(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		ID:             10,
		Name:           "Binary amd64",
		URL:            "https://example.com/bin/amd64",
		LinkType:       "package",
		External:       true,
		DirectAssetURL: "https://direct.example.com",
	})

	for _, want := range []string{
		"## Release Link: Binary amd64",
		"- **ID**: 10",
		"- **URL**: [https://example.com/bin/amd64](https://example.com/bin/amd64)",
		"- **Type**: package",
		"- **External**: true",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatOutputMarkdown_Empty verifies the behavior of format output markdown empty.
func TestFormatOutputMarkdown_Empty(t *testing.T) {
	md := FormatOutputMarkdown(Output{})
	if !strings.Contains(md, "## Release Link:") {
		t.Errorf("expected header in empty output:\n%s", md)
	}
	if !strings.Contains(md, "- **ID**: 0") {
		t.Errorf("expected zero ID:\n%s", md)
	}
}

// TestFormatOutputMarkdown_ExternalFalse verifies the behavior of format output markdown external false.
func TestFormatOutputMarkdown_ExternalFalse(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		ID:       5,
		Name:     "Runbook",
		URL:      "https://example.com/runbook",
		LinkType: "runbook",
		External: false,
	})
	if !strings.Contains(md, "- **External**: false") {
		t.Errorf("expected External=false:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithLinks verifies the behavior of format list markdown with links.
func TestFormatListMarkdown_WithLinks(t *testing.T) {
	out := ListOutput{
		Links: []Output{
			{ID: 10, Name: "Binary amd64", LinkType: "package", URL: "https://example.com/amd64"},
			{ID: 11, Name: "Binary arm64", LinkType: "package", URL: "https://example.com/arm64"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatListMarkdown(out)

	for _, want := range []string{
		"## Release Links (2)",
		"| ID |",
		"| --- |",
		"| 10 |",
		"| 11 |",
		"Binary amd64",
		"Binary arm64",
		"package",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatListMarkdown_Empty verifies the behavior of format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No release links found") {
		t.Errorf("expected empty message:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// TestFormatListMarkdown_SingleLink verifies the behavior of format list markdown single link.
func TestFormatListMarkdown_SingleLink(t *testing.T) {
	out := ListOutput{
		Links: []Output{
			{ID: 1, Name: "Image", LinkType: "image", URL: "https://example.com/img"},
		},
	}
	md := FormatListMarkdown(out)
	if !strings.Contains(md, "## Release Links (1)") {
		t.Errorf("expected count 1:\n%s", md)
	}
	if !strings.Contains(md, "| 1 |") {
		t.Errorf("expected row with ID 1:\n%s", md)
	}
}

// TestFormatListMarkdown_ContainsHints verifies that FormatListMarkdown includes
// next-step hints for both link_create (single) and link_create_batch (bulk).
func TestFormatListMarkdown_ContainsHints(t *testing.T) {
	out := ListOutput{
		Links: []Output{
			{ID: 1, Name: "Binary", LinkType: "package", URL: "https://example.com/bin"},
		},
	}
	md := FormatListMarkdown(out)
	for _, want := range []string{
		"link_create'",
		"link_create_batch'",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("FormatListMarkdown missing hint containing %q:\n%s", want, md)
		}
	}
}

// ---------------------------------------------------------------------------
// FormatBatchMarkdown
// ---------------------------------------------------------------------------.

// TestFormatBatchMarkdown_WithCreatedAndFailed verifies that FormatBatchMarkdown
// renders both the created links table and the failures list when both are present.
func TestFormatBatchMarkdown_WithCreatedAndFailed(t *testing.T) {
	out := CreateBatchOutput{
		Created: []Output{
			{ID: 1, Name: "Binary amd64", LinkType: "package", URL: "https://example.com/amd64"},
			{ID: 2, Name: "Binary arm64", LinkType: "package", URL: "https://example.com/arm64"},
		},
		Failed: []string{"checksum.txt: 409 Conflict"},
	}
	md := FormatBatchMarkdown(out)
	checks := []string{
		"## Release Links Created (2)",
		"| ID | Name | Type | URL |",
		"| 1 |", "Binary amd64", "package",
		"| 2 |", "Binary arm64",
		"### Failures (1)",
		"checksum.txt: 409 Conflict",
	}
	for _, c := range checks {
		if !strings.Contains(md, c) {
			t.Errorf("missing %q in:\n%s", c, md)
		}
	}
}

// TestFormatBatchMarkdown_AllCreated verifies rendering when all links
// are created successfully with no failures.
func TestFormatBatchMarkdown_AllCreated(t *testing.T) {
	out := CreateBatchOutput{
		Created: []Output{
			{ID: 5, Name: "Source", LinkType: "other", URL: "https://example.com/src"},
		},
	}
	md := FormatBatchMarkdown(out)
	if !strings.Contains(md, "## Release Links Created (1)") {
		t.Errorf("missing header:\n%s", md)
	}
	if strings.Contains(md, "Failures") {
		t.Errorf("unexpected Failures section:\n%s", md)
	}
}

// TestFormatBatchMarkdown_Empty verifies rendering when no links were created.
func TestFormatBatchMarkdown_Empty(t *testing.T) {
	out := CreateBatchOutput{Created: []Output{}}
	md := FormatBatchMarkdown(out)
	if !strings.Contains(md, "## Release Links Created (0)") {
		t.Errorf("missing header:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Errorf("unexpected table for empty output:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// ToOutput
// ---------------------------------------------------------------------------.

// TestToOutput_AllFields verifies the behavior of to output all fields.
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
	if !out.External {
		t.Error("expected External=true")
	}
	if out.DirectAssetURL != "https://direct.example.com" {
		t.Errorf("DirectAssetURL = %q, want %q", out.DirectAssetURL, "https://direct.example.com")
	}
}

// TestToOutput_ZeroValue verifies the behavior of to output zero value.
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
// RegisterToolsCallAllThroughMCP — full MCP roundtrip for all 5 tools
// ---------------------------------------------------------------------------.

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newReleaseLinksMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_release_link_list", map[string]any{"project_id": "42", "tag_name": "v1.0.0"}},
		{"get", "gitlab_release_link_get", map[string]any{"project_id": "42", "tag_name": "v1.0.0", "link_id": 10}},
		{"create", "gitlab_release_link_create", map[string]any{"project_id": "42", "tag_name": "v1.0.0", "name": "Binary", "url": "https://example.com/bin"}},
		{"update", "gitlab_release_link_update", map[string]any{"project_id": "42", "tag_name": "v1.0.0", "link_id": 10, "name": "Updated"}},
		{"delete", "gitlab_release_link_delete", map[string]any{"project_id": "42", "tag_name": "v1.0.0", "link_id": 10}},
		{"create_batch", "gitlab_release_link_create_batch", map[string]any{"project_id": "42", "tag_name": "v1.0.0", "links": []any{map[string]any{"name": "Binary", "url": "https://example.com/bin"}}}},
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

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------.

// newReleaseLinksMCPSession is an internal helper for the releaselinks package.
func newReleaseLinksMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	linkJSON := `{"id":10,"name":"Binary amd64","url":"https://example.com/bin/amd64","link_type":"package","external":true,"direct_asset_url":""}`

	handler := http.NewServeMux()

	// List release links
	handler.HandleFunc("GET /api/v4/projects/42/releases/v1.0.0/assets/links", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+linkJSON+`]`)
	})

	// Get release link
	handler.HandleFunc("GET /api/v4/projects/42/releases/v1.0.0/assets/links/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, linkJSON)
	})

	// Create release link
	handler.HandleFunc("POST /api/v4/projects/42/releases/v1.0.0/assets/links", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, linkJSON)
	})

	// Update release link
	handler.HandleFunc("PUT /api/v4/projects/42/releases/v1.0.0/assets/links/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, linkJSON)
	})

	// Delete release link
	handler.HandleFunc("DELETE /api/v4/projects/42/releases/v1.0.0/assets/links/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, linkJSON)
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
