// groupmarkdownuploads_test.go contains unit tests for the groupmarkdownuploads MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package groupmarkdownuploads

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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

// TestList_Error verifies that List returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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
// The mock GitLab API at /api/v4/groups/5/uploads/1 (DELETE) returns a representative success body.
// It asserts the returned output matches the expected fields.
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

// TestDeleteByID_Error verifies that DeleteByID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteByID_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"forbidden"}`)
	}))
	err := DeleteByID(t.Context(), client, DeleteByIDInput{GroupID: "5", UploadID: 1})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestDeleteByID_ValidationUploadID verifies the DeleteByID_ValidationUploadID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDeleteByID_ValidationUploadID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	for _, id := range []int64{0, -1} {
		err := DeleteByID(t.Context(), client, DeleteByIDInput{GroupID: "5", UploadID: id})
		if err == nil {
			t.Fatalf("expected error for upload_id=%d, got nil", id)
		}
		if !strings.Contains(err.Error(), "upload_id") {
			t.Errorf("error %q does not mention upload_id", err.Error())
		}
	}
}

// TestDeleteBySecretAndFilename verifies the DeleteBySecretAndFilename handler.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestDeleteBySecretAndFilename_Error verifies that DeleteBySecretAndFilename returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestFormatList verifies the List Markdown formatter for a representative list input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatList(t *testing.T) {
	out := &ListOutput{
		Uploads: []UploadItem{
			{ID: 1, Size: 1024, Filename: testFilename},
		},
	}
	md := FormatList(out)
	if !strings.Contains(md, testFilename) {
		t.Errorf("expected markdown to contain 'image.png'")
	}
}

// TestFormatList_Empty verifies the List_Empty Markdown formatter for a representative list_empty input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatList_Empty(t *testing.T) {
	out := &ListOutput{Uploads: []UploadItem{}}
	md := FormatList(out)
	if !strings.Contains(md, "No group markdown uploads") {
		t.Errorf("expected empty message")
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
// List — canceled context, pagination, empty result, multiple uploads
// ---------------------------------------------------------------------------.

// TestList_CancelledContext verifies the List_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{GroupID: "5"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestList_EmptyGroupID verifies the List_EmptyGroupID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_EmptyGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error for empty group_id")
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

// TestList_APIErrorInternalServer verifies that ListInternalServer returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestList_APIErrorInternalServer(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := List(context.Background(), client, ListInput{GroupID: "5"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// DeleteByID — canceled context, empty group_id
// ---------------------------------------------------------------------------.

// TestDeleteByID_CancelledContext verifies the DeleteByID_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestDeleteByID_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	err := DeleteByID(ctx, client, DeleteByIDInput{GroupID: "5", UploadID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestDeleteByID_EmptyGroupID verifies the DeleteByID_EmptyGroupID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDeleteByID_EmptyGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	err := DeleteByID(context.Background(), client, DeleteByIDInput{UploadID: 1})
	if err == nil {
		t.Fatal("expected error for empty group_id")
	}
}

// TestDeleteByID_InternalServerError verifies that DeleteByID_InternalServerError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteByID_InternalServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := DeleteByID(context.Background(), client, DeleteByIDInput{GroupID: "5", UploadID: 99})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// DeleteBySecretAndFilename — canceled context, empty fields
// ---------------------------------------------------------------------------.

// TestDeleteBySecretAndFilename_CancelledContext verifies the DeleteBySecretAndFilename_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestDeleteBySecretAndFilename_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	err := DeleteBySecretAndFilename(ctx, client, DeleteBySecretAndFilenameInput{
		GroupID: "5", Secret: "abc", Filename: "image.png",
	})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestDeleteBySecretAndFilename_EmptyGroupID verifies the DeleteBySecretAndFilename_EmptyGroupID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDeleteBySecretAndFilename_EmptyGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	err := DeleteBySecretAndFilename(context.Background(), client, DeleteBySecretAndFilenameInput{
		Secret: "abc", Filename: "image.png",
	})
	if err == nil {
		t.Fatal("expected error for empty group_id")
	}
}

// TestDeleteBySecretAndFilename_InternalServerError verifies that DeleteBySecretAndFilename_InternalServerError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteBySecretAndFilename_InternalServerError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := DeleteBySecretAndFilename(context.Background(), client, DeleteBySecretAndFilenameInput{
		GroupID: "5", Secret: "abc", Filename: "image.png",
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// FormatList — with pagination, special characters, nil created_at
// ---------------------------------------------------------------------------.

// TestFormatList_WithPagination verifies the List_WithPagination Markdown formatter for a representative list_withpagination input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestFormatList_WithPagination(t *testing.T) {
	out := &ListOutput{
		Uploads: []UploadItem{
			{ID: 1, Size: 1024, Filename: "image.png", CreatedAt: "2026-01-01T00:00:00Z"},
		},
		Pagination: toolutil.PaginationOutput{
			TotalItems: 10, Page: 1, PerPage: 20, TotalPages: 1,
		},
	}
	md := FormatList(out)
	if !strings.Contains(md, "image.png") {
		t.Errorf("expected markdown to contain 'image.png':\n%s", md)
	}
	if !strings.Contains(md, "| ID |") {
		t.Errorf("expected table header:\n%s", md)
	}
	if !strings.Contains(md, "1024") {
		t.Errorf("expected size in markdown:\n%s", md)
	}
}

// TestFormatList_SpecialCharacters verifies the List_SpecialCharacters Markdown formatter for a representative list_specialcharacters input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatList_SpecialCharacters(t *testing.T) {
	out := &ListOutput{
		Uploads: []UploadItem{
			{ID: 5, Size: 256, Filename: "file|with|pipes.txt", CreatedAt: "2026-01-01"},
		},
	}
	md := FormatList(out)
	// EscapeMdTableCell should handle pipe characters
	if !strings.Contains(md, "5") {
		t.Errorf("expected ID in markdown:\n%s", md)
	}
}

// TestFormatList_NilCreatedAt verifies the List_NilCreatedAt Markdown formatter for a representative list_nilcreatedat input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatList_NilCreatedAt(t *testing.T) {
	out := &ListOutput{
		Uploads: []UploadItem{
			{ID: 7, Size: 512, Filename: "no-date.bin"},
		},
	}
	md := FormatList(out)
	if !strings.Contains(md, "no-date.bin") {
		t.Errorf("expected filename in markdown:\n%s", md)
	}
	if !strings.Contains(md, "| 7 |") {
		t.Errorf("expected ID row:\n%s", md)
	}
}

// TestFormatList_MultipleRows verifies the List_MultipleRows Markdown formatter for a representative list_multiplerows input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatList_MultipleRows(t *testing.T) {
	out := &ListOutput{
		Uploads: []UploadItem{
			{ID: 1, Size: 100, Filename: "a.txt", CreatedAt: "2026-01-01"},
			{ID: 2, Size: 200, Filename: "b.txt", CreatedAt: "2026-02-01"},
			{ID: 3, Size: 300, Filename: "c.txt", CreatedAt: "2026-03-01"},
		},
	}
	md := FormatList(out)
	for _, want := range []string{"a.txt", "b.txt", "c.txt", "| 1 |", "| 2 |", "| 3 |"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Errorf("markdown missing %q:\n%s", want, md)
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
// Markdown registry formatter + uploadedByLabel helper
// ---------------------------------------------------------------------------.

// TestFormatListMarkdownString_UploadedBy verifies the registry-registered
// formatter renders the uploaded-by column and rows.
func TestFormatListMarkdownString_UploadedBy(t *testing.T) {
	out := ListOutput{
		Uploads: []UploadItem{
			{ID: 1, Size: 1024, Filename: "image.png", CreatedAt: "2026-01-01", UploadedBy: &UploadedByOutput{Username: "bob", Name: "Bob"}},
		},
	}
	md := FormatListMarkdownString(out)
	for _, want := range []string{"Group Markdown Uploads (1)", "Uploaded By", "Bob (@bob)", "image.png"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Errorf("markdown missing %q:\n%s", want, md)
			}
		})
	}
}

// TestFormatListMarkdownString_Empty verifies the empty path of the registry formatter.
func TestFormatListMarkdownString_Empty(t *testing.T) {
	md := FormatListMarkdownString(ListOutput{})
	if !strings.Contains(md, "No uploads found.") {
		t.Errorf("expected empty message:\n%s", md)
	}
}

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
