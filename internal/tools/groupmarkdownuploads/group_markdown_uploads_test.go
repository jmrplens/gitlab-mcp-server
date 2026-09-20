// group_markdown_uploads_test.go contains unit tests for the groupmarkdownuploads MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package groupmarkdownuploads

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// errExpectedErr identifies the err expected err constant used by this package.
const errExpectedErr = "expected error"

// testFilename identifies the test filename constant used by this package.
const testFilename = "image.png"

// TestList verifies the List handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"size":1024,"filename":"image.png","created_at":"2026-01-01T00:00:00Z"}]`)
	}))
	out, err := List(t.Context(), client, ListInput{GroupID: "5"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Uploads) != 1 {
		t.Fatalf("expected 1 upload, got %d", len(out.Uploads))
	}
	if out.Uploads[0].Filename != testFilename {
		t.Errorf("expected filename 'image.png', got %q", out.Uploads[0].Filename)
	}
}

// TestList_Error verifies that List returns an error when the GitLab API
// answers 404.
// The hint that 404 carries is asserted in
// TestHandlers_NotFound_CarryTheRouteThatRecovers, not here.
func TestList_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	_, err := List(t.Context(), client, ListInput{GroupID: "5"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestDeleteByID verifies the DeleteByID handler.
// The mock answers 204 No Content at /api/v4/groups/5/uploads/1 (DELETE) and
// 404 anywhere else, so the assertion that no error came back is also the
// assertion that the handler asked for that exact path.
func TestDeleteByID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v4/groups/5/uploads/1" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	err := DeleteByID(t.Context(), client, DeleteByIDInput{GroupID: "5", UploadID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeleteByID_Error verifies that DeleteByID returns an error when the
// GitLab API answers 403 to the DELETE.
// Which hint the wrapper attaches to which status is asserted in
// TestHandlers_NotFound_CarryTheRouteThatRecovers, not here.
func TestDeleteByID_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"forbidden"}`)
	}))
	err := DeleteByID(t.Context(), client, DeleteByIDInput{GroupID: "5", UploadID: 1})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestDeleteByID_ValidationUploadID verifies that DeleteByID rejects a
// non-positive upload_id before reaching the API, naming the field in the
// error.
func TestDeleteByID_ValidationUploadID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	cases := []struct {
		name string
		id   int64
	}{
		{"zero", 0},
		{"negative", -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := DeleteByID(t.Context(), client, DeleteByIDInput{GroupID: "5", UploadID: tc.id})
			if err == nil {
				t.Fatalf("expected error for upload_id=%d, got nil", tc.id)
			}
			if !strings.Contains(err.Error(), "upload_id") {
				t.Errorf("error %q does not mention upload_id", err.Error())
			}
		})
	}
}

// TestDeleteBySecretAndFilename verifies the DeleteBySecretAndFilename handler.
// The mock accepts any DELETE, so this asserts only that a 204 reaches the
// caller as success; that the secret and the filename land in that order is
// asserted in action_specs_test.go, whose mux routes on the whole path.
func TestDeleteBySecretAndFilename(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	err := DeleteBySecretAndFilename(t.Context(), client, DeleteBySecretAndFilenameInput{
		GroupID:  "5",
		Secret:   "abc123",
		Filename: testFilename,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeleteBySecretAndFilename_Error verifies that DeleteBySecretAndFilename
// returns an error when the GitLab API answers 404 to the DELETE.
// The hint that 404 carries is asserted in
// TestHandlers_NotFound_CarryTheRouteThatRecovers, not here.
func TestDeleteBySecretAndFilename_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	err := DeleteBySecretAndFilename(t.Context(), client, DeleteBySecretAndFilenameInput{
		GroupID:  "5",
		Secret:   "abc123",
		Filename: testFilename,
	})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// The Markdown formatter is covered whole-output in markdown_test.go, beside
// the list vocabulary it now writes.

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpCancelledCtx identifies the err exp cancelled ctx constant used by this package.
const errExpCancelledCtx = "expected error for canceled context"

// errExpectedAPI identifies the err expected API constant used by this package.
const errExpectedAPI = "expected API error, got nil"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// msgServerError is the body GitLab sends with a 500. It is spelled as a
// constant because the three tests that drive that status used to interpolate
// a bare identifier into their JSON (`{"message":msgServerError}`), which is
// not a document any decoder reads.
const msgServerError = `{"message":"500 Internal Server Error"}`

// ---------------------------------------------------------------------------
// List — canceled context, pagination, empty result, multiple uploads
// ---------------------------------------------------------------------------.

// TestList_CancelledContext verifies that a canceled context aborts List
// without contacting GitLab.
// The mock is testutil.ForbiddenHandler, which fails the test if a request
// arrives, so the second half of that sentence is asserted rather than assumed.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{GroupID: "5"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestList_EmptyResult verifies the List_EmptyResult handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_EmptyResult(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := List(context.Background(), client, ListInput{GroupID: "5"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Uploads) != 0 {
		t.Fatalf("expected 0 uploads, got %d", len(out.Uploads))
	}
}

// TestList_MultipleUploads verifies the List_MultipleUploads handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_MultipleUploads(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id":1,"size":1024,"filename":"image.png","created_at":"2026-01-01T00:00:00Z"},
				{"id":2,"size":2048,"filename":"doc.pdf"},
				{"id":3,"size":512,"filename":"readme.md","created_at":"2026-06-15T10:30:00Z"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := List(context.Background(), client, ListInput{GroupID: "5"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Uploads) != 3 {
		t.Fatalf("expected 3 uploads, got %d", len(out.Uploads))
	}
	if out.Uploads[0].Size != 1024 {
		t.Errorf("Uploads[0].Size = %d, want 1024", out.Uploads[0].Size)
	}
	if out.Uploads[1].Filename != "doc.pdf" {
		t.Errorf("Uploads[1].Filename = %q, want %q", out.Uploads[1].Filename, "doc.pdf")
	}
	if out.Uploads[2].ID != 3 {
		t.Errorf("Uploads[2].ID = %d, want 3", out.Uploads[2].ID)
	}
}

// TestList_WithPagination verifies that List_WithPagination forwards pagination parameters to the GitLab API and parses the response metadata.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestList_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"id":10,"size":4096,"filename":"file10.zip"}]`,
				testutil.PaginationHeaders{
					Page: "2", PerPage: "1", Total: "5", TotalPages: "5", NextPage: "3", PrevPage: "1",
				})
			return
		}
		http.NotFound(w, r)
	}))
	out, err := List(context.Background(), client, ListInput{
		GroupID: "5",
		Page:    2, PerPage: 1,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Uploads) != 1 {
		t.Fatalf("expected 1 upload, got %d", len(out.Uploads))
	}
	if out.Pagination.TotalPages != 5 {
		t.Errorf("TotalPages = %d, want 5", out.Pagination.TotalPages)
	}
	if out.Pagination.TotalItems != 5 {
		t.Errorf("TotalItems = %d, want 5", out.Pagination.TotalItems)
	}
}

// TestList_OffsetPagination_ReachesTheRequest verifies that the page and
// per_page a caller asks for are on the query string of the request List makes.
//
// Why it matters: TestList_WithPagination sets both and then asserts only on
// the block List built from the response headers, which the mock chose for
// itself and which the handler would publish whatever it had sent. Handing
// toolutil.ApplyListOptions an empty PaginationInput therefore left the whole
// suite green: nothing held the page a model asked for to the request. The
// query is parsed rather than substring-matched, because "page=3" is also a
// substring of "per_page=3".
func TestList_OffsetPagination_ReachesTheRequest(t *testing.T) {
	var gotQuery string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		gotQuery = r.URL.RawQuery
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	if _, err := List(t.Context(), client, ListInput{GroupID: "5", Page: 3, PerPage: 7}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	sent, err := url.ParseQuery(gotQuery)
	if err != nil {
		t.Fatalf("query %q does not parse: %v", gotQuery, err)
	}
	for _, want := range []struct{ key, value string }{
		{"page", "3"},
		{"per_page", "7"},
	} {
		t.Run(want.key, func(t *testing.T) {
			if got := sent.Get(want.key); got != want.value {
				t.Errorf("request carried %s=%q, want %q (whole query %q)", want.key, got, want.value, gotQuery)
			}
		})
	}
}

// TestList_APIErrorInternalServer verifies that List returns an error when
// GitLab answers 500.
// The test exercises the GET path of the underlying GitLab API call.
func TestList_APIErrorInternalServer(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusInternalServerError, msgServerError)
	}))
	_, err := List(context.Background(), client, ListInput{GroupID: "5"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// DeleteByID — canceled context, empty group_id
// ---------------------------------------------------------------------------.

// TestDeleteByID_CancelledContext verifies that a canceled context aborts
// DeleteByID without contacting GitLab, which matters more for a delete than
// for a read: a request that leaves is a deletion nobody is waiting on.
// testutil.ForbiddenHandler fails the test if one arrives.
func TestDeleteByID_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := testutil.CancelledCtx(t)
	err := DeleteByID(ctx, client, DeleteByIDInput{GroupID: "5", UploadID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestDeleteByID_InternalServerError verifies that DeleteByID returns an error
// when GitLab answers 500.
// The test exercises the DELETE path of the underlying GitLab API call.
func TestDeleteByID_InternalServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusInternalServerError, msgServerError)
	}))
	err := DeleteByID(context.Background(), client, DeleteByIDInput{GroupID: "5", UploadID: 99})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// DeleteBySecretAndFilename — canceled context, empty fields
// ---------------------------------------------------------------------------.

// TestDeleteBySecretAndFilename_CancelledContext verifies that a canceled
// context aborts DeleteBySecretAndFilename without contacting GitLab.
// testutil.ForbiddenHandler fails the test if a request arrives.
func TestDeleteBySecretAndFilename_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := testutil.CancelledCtx(t)
	err := DeleteBySecretAndFilename(ctx, client, DeleteBySecretAndFilenameInput{
		GroupID: "5", Secret: "abc", Filename: "image.png",
	})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestDeleteBySecretAndFilename_InternalServerError verifies that
// DeleteBySecretAndFilename returns an error when GitLab answers 500.
// The test exercises the DELETE path of the underlying GitLab API call.
func TestDeleteBySecretAndFilename_InternalServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusInternalServerError, msgServerError)
	}))
	err := DeleteBySecretAndFilename(context.Background(), client, DeleteBySecretAndFilenameInput{
		GroupID: "5", Secret: "abc", Filename: "image.png",
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestHandlers_EmptyGroupID_RefusedWithoutReachingGitLab verifies that each of
// the three handlers refuses an empty group_id itself, names the field in the
// error, and issues no request at all.
//
// Why it matters: the three tests this replaces pointed their mock at a
// handler that answered 400 to everything and then asserted only that some
// error came back, so the refusal they observed could just as well have been
// GitLab's. Deleting all three guards left the whole suite green. Driving them
// through testutil.ForbiddenHandler, which fails the test the moment a request
// arrives, is what makes the refusal attributable to the handler; naming
// group_id is what distinguishes it from any other refusal the handler makes.
func TestHandlers_EmptyGroupID_RefusedWithoutReachingGitLab(t *testing.T) {
	cases := []struct {
		name string
		call func(t *testing.T) error
	}{
		{
			name: "list",
			call: func(t *testing.T) error {
				t.Helper()
				_, err := List(t.Context(), testutil.NewTestClient(t, testutil.ForbiddenHandler(t)), ListInput{})
				return err
			},
		},
		{
			name: "delete_by_id",
			call: func(t *testing.T) error {
				t.Helper()
				return DeleteByID(t.Context(), testutil.NewTestClient(t, testutil.ForbiddenHandler(t)),
					DeleteByIDInput{UploadID: 1})
			},
		},
		{
			name: "delete_by_secret",
			call: func(t *testing.T) error {
				t.Helper()
				return DeleteBySecretAndFilename(t.Context(), testutil.NewTestClient(t, testutil.ForbiddenHandler(t)),
					DeleteBySecretAndFilenameInput{Secret: "abc123", Filename: testFilename})
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call(t)
			if err == nil {
				t.Fatal("expected a refusal for an empty group_id")
			}
			if !strings.Contains(err.Error(), "group_id") {
				t.Errorf("refusal %q does not name group_id", err.Error())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// List — uploaded_by mapping and keyset/order_by/sort forwarding (1:1 audit)
// ---------------------------------------------------------------------------.

// TestList_UploadedBy verifies that the uploaded_by user embed in the GitLab
// response is mapped onto the documented UploadedByOutput short-user subset
// (id, username, name per doc/api/group_markdown_uploads.md). Extra SDK fields
// present in the response (state, avatar_url, web_url) are tolerated and
// dropped.
func TestList_UploadedBy(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"size":1024,"filename":"image.png","created_at":"2026-01-01T00:00:00Z","uploaded_by":{"id":42,"username":"alice","name":"Alice Example","state":"active","avatar_url":"https://gitlab.example.com/avatar/42.png","web_url":"https://gitlab.example.com/alice"}}]`)
	}))
	out, err := List(t.Context(), client, ListInput{GroupID: "5"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Uploads) != 1 {
		t.Fatalf("expected 1 upload, got %d", len(out.Uploads))
	}
	ub := out.Uploads[0].UploadedBy
	if ub == nil {
		t.Fatal("expected uploaded_by to be populated")
	}
	if ub.ID != 42 || ub.Username != "alice" || ub.Name != "Alice Example" {
		t.Errorf("uploaded_by mapped incorrectly: %+v", ub)
	}
}

// TestList_UploadedByAbsent verifies that a missing uploaded_by field maps to a
// nil UploadedByOutput pointer.
func TestList_UploadedByAbsent(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"size":1024,"filename":"image.png"}]`)
	}))
	out, err := List(t.Context(), client, ListInput{GroupID: "5"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Uploads[0].UploadedBy != nil {
		t.Errorf("expected nil uploaded_by, got %+v", out.Uploads[0].UploadedBy)
	}
}

// TestList_KeysetAndOrdering verifies that order_by, sort, and keyset
// pagination parameters are forwarded to the GitLab API query string.
func TestList_KeysetAndOrdering(t *testing.T) {
	var gotQuery string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		gotQuery = r.URL.RawQuery
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := List(t.Context(), client, ListInput{
		GroupID:    "5",
		OrderBy:    "created_at",
		Sort:       "desc",
		Pagination: "keyset", PageToken: "tok123",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	for _, want := range []string{"order_by=created_at", "sort=desc", "pagination=keyset", "page_token=tok123"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(gotQuery, want) {
				t.Errorf("query %q missing %q", gotQuery, want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// uploadedByLabel helper
// ---------------------------------------------------------------------------.

// TestUploadedByLabel verifies every branch of the uploadedByLabel helper.
func TestUploadedByLabel(t *testing.T) {
	cases := []struct {
		name string
		in   *UploadedByOutput
		want string
	}{
		{"nil", nil, ""},
		{"both", &UploadedByOutput{Name: "Alice", Username: "alice"}, "Alice (@alice)"},
		{"username_only", &UploadedByOutput{Username: "alice"}, "@alice"},
		{"name_only", &UploadedByOutput{Name: "Alice"}, "Alice"},
		{"empty", &UploadedByOutput{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := uploadedByLabel(tc.in); got != tc.want {
				t.Errorf("%s: uploadedByLabel = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// What a caller is handed: the timestamp form, and the empty list
// ---------------------------------------------------------------------------.

// TestList_CreatedAt_IsTheWireFormTheDisplayHelperReads verifies that the
// timestamp List publishes names the instant GitLab sent, in the RFC 3339 form
// [toolutil.FormatTime] can parse, and that an upload GitLab dated nowhere
// carries an empty string rather than the year one.
//
// Why it matters: the conversion in List exists because the handler once
// published time.Time.String(), "2026-01-02 03:04:05 +0000 UTC", which no
// display helper parses, so the Created column printed the raw text.
// Everything else in this file fed a created_at and then asserted on the
// filename, and the formatter's own test hand-writes an RFC 3339 string into
// the struct, so the handler could publish any form at all and the package
// would stay green. Both halves are stated here: what the field holds, and
// that the helper the table calls can read it back.
func TestList_CreatedAt_IsTheWireFormTheDisplayHelperReads(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `[
			{"id":1,"size":1024,"filename":"image.png","created_at":"2026-01-02T03:04:05Z"},
			{"id":2,"size":16,"filename":"undated.bin"}
		]`)
	}))
	out, err := List(t.Context(), client, ListInput{GroupID: "5"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Uploads) != 2 {
		t.Fatalf("expected 2 uploads, got %d", len(out.Uploads))
	}

	published := out.Uploads[0].CreatedAt
	got, err := time.Parse(time.RFC3339, published)
	if err != nil {
		t.Fatalf("created_at %q is not the RFC 3339 wire form: %v", published, err)
	}
	if want := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC); !got.Equal(want) {
		t.Errorf("created_at names %s, want the instant GitLab sent, %s", got, want)
	}
	// FormatTime hands back its own argument when it cannot parse it, so a
	// rendering that differs from the stored value is the proof that the
	// display helper read the timestamp rather than gave up on it.
	if display := toolutil.FormatTime(published); display == published {
		t.Errorf("FormatTime(%q) returned it unchanged, so the Created column shows the wire form", published)
	}
	if undated := out.Uploads[1].CreatedAt; undated != "" {
		t.Errorf("created_at of an upload GitLab dated nowhere = %q, want empty", undated)
	}
}

// TestList_NoUploads_PublishesAnEmptyArray verifies that a group with no
// uploads is answered with an empty JSON array rather than a null.
//
// Why it matters: uploads carries no omitempty, so whatever slice List builds
// is what the client decodes. Building it with var instead of make publishes
// "uploads":null, which a model has to special-case before it can iterate, and
// no length assertion can see the difference: len(nil) is 0 as well. The
// assertion is therefore on the encoded document, which is what a caller gets.
func TestList_NoUploads_PublishesAnEmptyArray(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	out, err := List(t.Context(), client, ListInput{GroupID: "5"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal output: %v", err)
	}
	if !strings.Contains(string(encoded), `"uploads":[]`) {
		t.Errorf("an empty group encodes to %s, want an empty uploads array", encoded)
	}
}

// ---------------------------------------------------------------------------
// The hint each handler attaches to the status it names
// ---------------------------------------------------------------------------.

// statusHandler answers every request with code and a GitLab-shaped body, so a
// handler's own status branch is what the test is choosing.
func statusHandler(code int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, code, `{"message":"nope"}`)
	})
}

// TestHandlers_NotFound_CarryTheRouteThatRecovers verifies that each handler's
// 404 reaches the caller as a hinted error naming the action to recover
// through, and that a status the handler did not name carries no suggestion at
// all.
//
// Why it matters: all three handlers route their failures through
// toolutil.WrapErrWithStatusHint, which attaches the hint only when the status
// matches the one that handler named and otherwise wraps without one. Naming
// the wrong status, or passing an empty hint, still returns an error, so every
// "expected error, got nil" assertion in this file goes on passing while a
// model is handed a refusal with nothing to do about it. The assertion is on
// the recovery route the hint names, not on its wording, so rephrasing a hint
// does not fail the test and dropping it does.
func TestHandlers_NotFound_CarryTheRouteThatRecovers(t *testing.T) {
	cases := []struct {
		name string
		// recovery is the tool or argument the hint must point the caller at.
		recovery string
		call     func(t *testing.T, code int) error
	}{
		{
			name:     "list",
			recovery: "gitlab_group_get",
			call: func(t *testing.T, code int) error {
				t.Helper()
				_, err := List(t.Context(), testutil.NewTestClient(t, statusHandler(code)), ListInput{GroupID: "5"})
				return err
			},
		},
		{
			name:     "delete_by_id",
			recovery: "gitlab_list_group_markdown_uploads",
			call: func(t *testing.T, code int) error {
				t.Helper()
				return DeleteByID(t.Context(), testutil.NewTestClient(t, statusHandler(code)),
					DeleteByIDInput{GroupID: "5", UploadID: 7})
			},
		},
		{
			name:     "delete_by_secret",
			recovery: "secret",
			call: func(t *testing.T, code int) error {
				t.Helper()
				return DeleteBySecretAndFilename(t.Context(), testutil.NewTestClient(t, statusHandler(code)),
					DeleteBySecretAndFilenameInput{GroupID: "5", Secret: "abc123", Filename: testFilename})
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("not_found_suggests_the_recovery", func(t *testing.T) {
				err := tc.call(t, http.StatusNotFound)
				if err == nil {
					t.Fatal(errExpectedErr)
				}
				if !strings.Contains(err.Error(), "Suggestion: ") {
					t.Errorf("404 error %q carries no suggestion", err.Error())
				}
				if !strings.Contains(err.Error(), tc.recovery) {
					t.Errorf("404 error %q does not name %q as the way back", err.Error(), tc.recovery)
				}
			})
			t.Run("other_status_is_left_unhinted", func(t *testing.T) {
				err := tc.call(t, http.StatusForbidden)
				if err == nil {
					t.Fatal(errExpectedErr)
				}
				if strings.Contains(err.Error(), "Suggestion: ") {
					t.Errorf("403 error %q carries the 404 hint, which advises about a group that does exist", err.Error())
				}
			})
		})
	}
}
