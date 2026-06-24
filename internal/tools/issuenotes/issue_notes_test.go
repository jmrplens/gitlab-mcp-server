package issuenotes

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Test endpoint paths and JSON response fixtures for issue note operation tests.
const (
	pathIssueNotes        = "/api/v4/projects/42/issues/10/notes"
	noteJSONSimple        = `{"id":100,"body":"Looks good to me","author":{"username":"alice"},"system":false,"internal":false,"created_at":"2026-01-15T10:00:00Z","updated_at":"2026-01-15T10:00:00Z"}`
	noteJSONInternal      = `{"id":101,"body":"Internal note","author":{"username":"bob"},"system":false,"internal":true,"created_at":"2026-01-15T11:00:00Z","updated_at":"2026-01-15T11:00:00Z"}`
	noteJSONSystem        = `{"id":102,"body":"changed the description","author":{"username":"admin"},"system":true,"internal":false,"created_at":"2026-01-15T12:00:00Z","updated_at":"2026-01-15T12:00:00Z"}`
	testNoteLGTM          = "Looks good to me"
	fmtIssueNoteListErr   = "List() unexpected error: %v"
	fmtIssueNoteCreateErr = "Create() unexpected error: %v"
	testProjectID         = "42"
	fmtBodyWant           = "out.Body = %q, want %q"
	testUpdatedText       = "Updated text"
)

// TestIssueNoteCreate_Success verifies IssueNoteCreate when success.
func TestIssueNoteCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathIssueNotes {
			testutil.RespondJSON(w, http.StatusCreated, noteJSONSimple)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID: testProjectID,
		IssueIID:  10,
		Body:      testNoteLGTM,
	})
	if err != nil {
		t.Fatalf(fmtIssueNoteCreateErr, err)
	}
	if out.ID != 100 {
		t.Errorf("out.ID = %d, want 100", out.ID)
	}
	if out.Body != testNoteLGTM {
		t.Errorf(fmtBodyWant, out.Body, testNoteLGTM)
	}
	if out.Author == nil || out.Author.Username != "alice" {
		t.Errorf("out.Author = %+v, want username alice", out.Author)
	}
	if out.Internal {
		t.Error("out.Internal = true, want false")
	}
	if out.System {
		t.Error("out.System = true, want false")
	}
}

// TestIssueNote_CreateInternal verifies IssueNote when create internal.
func TestIssueNote_CreateInternal(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathIssueNotes {
			testutil.RespondJSON(w, http.StatusCreated, noteJSONInternal)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID: testProjectID,
		IssueIID:  10,
		Body:      "Internal note",
		Internal:  new(true),
	})
	if err != nil {
		t.Fatalf(fmtIssueNoteCreateErr, err)
	}
	if !out.Internal {
		t.Error("out.Internal = false, want true")
	}
}

// TestIssueNoteCreate_APIError verifies IssueNoteCreate when API error.
func TestIssueNoteCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	_, err := Create(context.Background(), client, CreateInput{
		ProjectID: testProjectID,
		IssueIID:  10,
		Body:      "test",
	})
	if err == nil {
		t.Fatal("Create() expected error for 403, got nil")
	}
}

// TestIssueNoteCreate_CancelledContext verifies IssueNoteCreate when cancelled context.
func TestIssueNoteCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Create(ctx, client, CreateInput{
		ProjectID: testProjectID,
		IssueIID:  10,
		Body:      "test",
	})
	if err == nil {
		t.Fatal("Create() expected error for canceled context, got nil")
	}
}

// TestIssueNoteList_Success verifies IssueNoteList when success.
func TestIssueNoteList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathIssueNotes {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+noteJSONSimple+`,`+noteJSONSystem+`]`, testutil.PaginationHeaders{
				Page: "1", PerPage: "20", Total: "2", TotalPages: "1",
			})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID: testProjectID,
		IssueIID:  10,
	})
	if err != nil {
		t.Fatalf(fmtIssueNoteListErr, err)
	}
	if len(out.Notes) != 2 {
		t.Fatalf("len(out.Notes) = %d, want 2", len(out.Notes))
	}
	if out.Notes[0].ID != 100 {
		t.Errorf("out.Notes[0].ID = %d, want 100", out.Notes[0].ID)
	}
	if out.Notes[1].System != true {
		t.Error("out.Notes[1].System = false, want true")
	}
}

// TestIssueNoteList_Empty verifies IssueNoteList when empty.
func TestIssueNoteList_Empty(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathIssueNotes {
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID: testProjectID,
		IssueIID:  10,
	})
	if err != nil {
		t.Fatalf(fmtIssueNoteListErr, err)
	}
	if len(out.Notes) != 0 {
		t.Errorf("len(out.Notes) = %d, want 0", len(out.Notes))
	}
}

// TestIssueNoteList_Pagination verifies IssueNoteList when pagination.
func TestIssueNoteList_Pagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathIssueNotes {
			q := r.URL.Query()
			if q.Get("page") != "2" {
				t.Errorf("expected page=2, got %q", q.Get("page"))
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+noteJSONSimple+`]`, testutil.PaginationHeaders{
				Page: "2", PerPage: "10", Total: "11", TotalPages: "2", PrevPage: "1",
			})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID:       testProjectID,
		IssueIID:        10,
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 10},
	})
	if err != nil {
		t.Fatalf(fmtIssueNoteListErr, err)
	}
	if out.Pagination.Page != 2 {
		t.Errorf("out.Pagination.Page = %d, want 2", out.Pagination.Page)
	}
	if out.Pagination.TotalPages != 2 {
		t.Errorf("out.Pagination.TotalPages = %d, want 2", out.Pagination.TotalPages)
	}
}

// TestIssueNoteCreate_SuccessEnrichedFields verifies IssueNoteCreate when success enriched fields.
func TestIssueNoteCreate_SuccessEnrichedFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathIssueNotes {
			testutil.RespondJSON(w, http.StatusCreated, `{
"id":150,"body":"Threaded comment",
"author":{"username":"charlie"},
"system":false,"internal":false,
"created_at":"2026-02-01T10:00:00Z","updated_at":"2026-02-01T10:00:00Z",
"resolvable":true,"resolved":false,
"noteable_type":"Issue",
"type":"DiscussionNote"
}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID: testProjectID,
		IssueIID:  10,
		Body:      "Threaded comment",
	})
	if err != nil {
		t.Fatalf(fmtIssueNoteCreateErr, err)
	}
	if !out.Resolvable {
		t.Error("out.Resolvable = false, want true")
	}
	if out.Resolved {
		t.Error("out.Resolved = true, want false")
	}
	if out.NoteableType != "Issue" {
		t.Errorf("out.NoteableType = %q, want %q", out.NoteableType, "Issue")
	}
	if out.Type != "DiscussionNote" {
		t.Errorf("out.Type = %q, want %q", out.Type, "DiscussionNote")
	}
}

// TestIssueNoteList_APIError verifies IssueNoteList when API error.
func TestIssueNoteList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	_, err := List(context.Background(), client, ListInput{
		ProjectID: testProjectID,
		IssueIID:  10,
	})
	if err == nil {
		t.Fatal("List() expected error for API error, got nil")
	}
}

// Tests for GetNote, Update, Delete.

// pathIssueNote100 identifies the path issue note 100 constant used by this package.
const pathIssueNote100 = "/api/v4/projects/42/issues/10/notes/100"

// TestGetNote_Success verifies GetNote when success.
func TestGetNote_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathIssueNote100 {
			testutil.RespondJSON(w, http.StatusOK, noteJSONSimple)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetNote(context.Background(), client, GetInput{
		ProjectID: testProjectID,
		IssueIID:  10,
		NoteID:    100,
	})
	if err != nil {
		t.Fatalf("GetNote() unexpected error: %v", err)
	}
	if out.ID != 100 {
		t.Errorf("out.ID = %d, want 100", out.ID)
	}
	if out.Body != testNoteLGTM {
		t.Errorf(fmtBodyWant, out.Body, testNoteLGTM)
	}
}

// TestGetNote_MissingProjectID verifies GetNote when missing project ID.
func TestGetNote_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := GetNote(context.Background(), client, GetInput{IssueIID: 10, NoteID: 100})
	if err == nil {
		t.Fatal("GetNote() expected error for missing project_id, got nil")
	}
}

// TestUpdate_Success verifies Update when success.
func TestUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathIssueNote100 {
			testutil.RespondJSON(w, http.StatusOK, `{"id":100,"body":"Updated text","author":{"username":"alice"},"system":false,"internal":false,"created_at":"2026-01-15T10:00:00Z","updated_at":"2026-01-15T14:00:00Z"}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID: testProjectID,
		IssueIID:  10,
		NoteID:    100,
		Body:      testUpdatedText,
	})
	if err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}
	if out.Body != testUpdatedText {
		t.Errorf(fmtBodyWant, out.Body, testUpdatedText)
	}
	if out.UpdatedAt != "2026-01-15T14:00:00Z" {
		t.Errorf("out.UpdatedAt = %q, want %q", out.UpdatedAt, "2026-01-15T14:00:00Z")
	}
}

// TestUpdate_MissingProjectID verifies Update when missing project ID.
func TestUpdate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Update(context.Background(), client, UpdateInput{IssueIID: 10, NoteID: 100, Body: "test"})
	if err == nil {
		t.Fatal("Update() expected error for missing project_id, got nil")
	}
}

// TestDelete_Success verifies Delete when success.
func TestDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathIssueNote100 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{
		ProjectID: testProjectID,
		IssueIID:  10,
		NoteID:    100,
	})
	if err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}
}

// TestDelete_MissingProjectID verifies Delete when missing project ID.
func TestDelete_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := Delete(context.Background(), client, DeleteInput{IssueIID: 10, NoteID: 100})
	if err == nil {
		t.Fatal("Delete() expected error for missing project_id, got nil")
	}
}

// ---------------------------------------------------------------------------
// assertContains verifies that err is non-nil and its message contains substr.
// ---------------------------------------------------------------------------.
func assertContains(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Errorf("error %q does not contain %q", err.Error(), substr)
	}
}

// TestIssueIIDRequired_Validation ensures all handlers that accept issue_iid
// reject zero/negative values before making any API call.
func TestIssueIIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called when issue_iid is invalid")
	}))
	ctx := context.Background()
	const pid = "my/project"

	tests := []struct {
		name string
		fn   func() error
	}{
		{"Create", func() error {
			_, e := Create(ctx, client, CreateInput{ProjectID: pid, IssueIID: 0, Body: "x"})
			return e
		}},
		{"List", func() error { _, e := List(ctx, client, ListInput{ProjectID: pid, IssueIID: 0}); return e }},
		{"GetNote", func() error { _, e := GetNote(ctx, client, GetInput{ProjectID: pid, IssueIID: 0, NoteID: 1}); return e }},
		{"Update", func() error {
			_, e := Update(ctx, client, UpdateInput{ProjectID: pid, IssueIID: 0, NoteID: 1, Body: "x"})
			return e
		}},
		{"Delete", func() error { return Delete(ctx, client, DeleteInput{ProjectID: pid, IssueIID: 0, NoteID: 1}) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertContains(t, tt.fn(), "issue_iid")
		})
	}
}

// TestNoteIDRequired_Validation ensures GetNote, Update, Delete reject
// zero/negative note_id before making any API call.
func TestNoteIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called when note_id is invalid")
	}))
	ctx := context.Background()
	const pid = "my/project"

	tests := []struct {
		name string
		fn   func() error
	}{
		{"GetNote", func() error {
			_, e := GetNote(ctx, client, GetInput{ProjectID: pid, IssueIID: 10, NoteID: 0})
			return e
		}},
		{"Update", func() error {
			_, e := Update(ctx, client, UpdateInput{ProjectID: pid, IssueIID: 10, NoteID: -1, Body: "x"})
			return e
		}},
		{"Delete", func() error { return Delete(ctx, client, DeleteInput{ProjectID: pid, IssueIID: 10, NoteID: 0}) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertContains(t, tt.fn(), "note_id")
		})
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// FormatOutputMarkdown
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdown_Populated covers FormatOutputMarkdown with table-driven subtests for populated.
func TestFormatOutputMarkdown_Populated(t *testing.T) {
	out := Output{
		ID:         200,
		Body:       "Full note body",
		Author:     &NoteUserOutput{Username: "fulluser"},
		CreatedAt:  "2026-03-01T09:00:00Z",
		System:     true,
		Internal:   true,
		Resolvable: true,
		Resolved:   true,
	}
	md := FormatOutputMarkdown(out)

	checks := []struct {
		label string
		want  string
	}{
		{"header", "## Issue Note #200"},
		{"author", "**Author**: fulluser"},
		{"created", "**Created**: 1 Mar 2026 09:00 UTC"},
		{"system", "**System note**"},
		{"internal", "**Internal note**"},
		{"resolvable resolved", "**Resolvable**: resolved"},
		{"body", "Full note body"},
	}
	for _, c := range checks {
		if !strings.Contains(md, c.want) {
			t.Errorf("%s: missing %q in:\n%s", c.label, c.want, md)
		}
	}
}

// TestFormatOutputMarkdown_ResolvableUnresolved verifies FormatOutputMarkdown when resolvable unresolved.
func TestFormatOutputMarkdown_ResolvableUnresolved(t *testing.T) {
	out := Output{
		ID:         201,
		Body:       "Unresolved note",
		Author:     &NoteUserOutput{Username: "reviewer"},
		CreatedAt:  "2026-03-02T09:00:00Z",
		Resolvable: true,
		Resolved:   false,
	}
	md := FormatOutputMarkdown(out)

	if !strings.Contains(md, "**Resolvable**: unresolved") {
		t.Errorf("expected unresolved, got:\n%s", md)
	}
	if strings.Contains(md, "**System note**") {
		t.Error("should not contain System note")
	}
	if strings.Contains(md, "**Internal note**") {
		t.Error("should not contain Internal note")
	}
}

// TestFormatOutputMarkdown_Minimal verifies FormatOutputMarkdown when minimal.
func TestFormatOutputMarkdown_Minimal(t *testing.T) {
	md := FormatOutputMarkdown(Output{})
	if !strings.Contains(md, "## Issue Note #0") {
		t.Error("missing header for empty output")
	}
	if strings.Contains(md, "**System note**") {
		t.Error("should not contain System note for default")
	}
	if strings.Contains(md, "**Internal note**") {
		t.Error("should not contain Internal note for default")
	}
	if strings.Contains(md, "**Resolvable**") {
		t.Error("should not contain Resolvable for default")
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_Populated covers FormatListMarkdown with table-driven subtests for populated.
func TestFormatListMarkdown_Populated(t *testing.T) {
	out := ListOutput{
		Notes: []Output{
			{ID: 1, Author: &NoteUserOutput{Username: "alice"}, CreatedAt: "2026-01-01T00:00:00Z", System: false, Internal: false},
			{ID: 2, Author: &NoteUserOutput{Username: "bob"}, CreatedAt: "2026-01-02T00:00:00Z", System: true, Internal: true},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatListMarkdown(out)

	checks := []struct {
		label string
		want  string
	}{
		{"header", "## Issue Notes (2)"},
		{"table header", "| ID | Author | Created | System | Internal |"},
		{"alice row", "| 1 | alice |"},
		{"bob row", "| 2 | bob |"},
	}
	for _, c := range checks {
		if !strings.Contains(md, c.want) {
			t.Errorf("%s: missing %q in:\n%s", c.label, c.want, md)
		}
	}
}

// TestFormatListMarkdown_Empty verifies FormatListMarkdown when empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No issue notes found.") {
		t.Error("missing empty-state message")
	}
}

// ---------------------------------------------------------------------------
// ToOutput - full fields
// ---------------------------------------------------------------------------.

// TestToOutput_AllFields verifies ToOutput when all fields.
func TestToOutput_AllFields(t *testing.T) {
	ts1 := mustParseTime(t, "2026-03-01T09:00:00Z")
	ts2 := mustParseTime(t, "2026-03-01T10:00:00Z")

	n := &gl.Note{
		ID:           200,
		Body:         "Full note body",
		Author:       gl.NoteAuthor{Username: "fulluser"},
		System:       true,
		Internal:     true,
		Resolvable:   true,
		Resolved:     true,
		NoteableType: "Issue",
		NoteableID:   10,
		CommitID:     "abc123",
		Type:         "DiffNote",
		NoteableIID:  5,
		ProjectID:    42,
		CreatedAt:    ts1,
		UpdatedAt:    ts2,
	}

	out := ToOutput(n)

	if out.ID != 200 {
		t.Errorf("ID = %d, want 200", out.ID)
	}
	if out.Body != "Full note body" {
		t.Errorf("Body = %q", out.Body)
	}
	if out.Author == nil || out.Author.Username != "fulluser" {
		t.Errorf("Author = %+v, want username fulluser", out.Author)
	}
	if !out.System {
		t.Error("System = false, want true")
	}
	if !out.Internal {
		t.Error("Internal = false, want true")
	}
	if !out.Resolvable {
		t.Error("Resolvable = false, want true")
	}
	if !out.Resolved {
		t.Error("Resolved = false, want true")
	}
	if out.NoteableType != "Issue" {
		t.Errorf("NoteableType = %q", out.NoteableType)
	}
	if out.NoteableID != 10 {
		t.Errorf("NoteableID = %d", out.NoteableID)
	}
	if out.CommitID != "abc123" {
		t.Errorf("CommitID = %q", out.CommitID)
	}
	if out.Type != "DiffNote" {
		t.Errorf("Type = %q", out.Type)
	}
	if out.NoteableIID != 5 {
		t.Errorf("NoteableIID = %d", out.NoteableIID)
	}
	if out.ProjectID != 42 {
		t.Errorf("ProjectID = %d", out.ProjectID)
	}
	if !out.Confidential {
		t.Error("Confidential = false, want true (mirrors Internal)")
	}
	if out.CreatedAt != "2026-03-01T09:00:00Z" {
		t.Errorf("CreatedAt = %q", out.CreatedAt)
	}
	if out.UpdatedAt != "2026-03-01T10:00:00Z" {
		t.Errorf("UpdatedAt = %q", out.UpdatedAt)
	}
}

// TestToOutput_NilTimestamps verifies ToOutput when nil timestamps.
func TestToOutput_NilTimestamps(t *testing.T) {
	n := &gl.Note{
		ID:     300,
		Body:   "body",
		Author: gl.NoteAuthor{Username: "user"},
	}
	out := ToOutput(n)
	if out.CreatedAt != "" {
		t.Errorf("CreatedAt = %q, want empty", out.CreatedAt)
	}
	if out.UpdatedAt != "" {
		t.Errorf("UpdatedAt = %q, want empty", out.UpdatedAt)
	}
}

// ---------------------------------------------------------------------------
// Context cancellation - handlers not yet covered
// ---------------------------------------------------------------------------.

// TestList_CancelledContext verifies List when cancelled context.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{ProjectID: "42", IssueIID: 10})
	if err == nil {
		t.Fatal("List() expected error for canceled context, got nil")
	}
}

// TestGetNote_CancelledContext verifies GetNote when cancelled context.
func TestGetNote_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, noteJSONSimple)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetNote(ctx, client, GetInput{ProjectID: "42", IssueIID: 10, NoteID: 100})
	if err == nil {
		t.Fatal("GetNote() expected error for canceled context, got nil")
	}
}

// TestUpdate_CancelledContext verifies Update when cancelled context.
func TestUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, noteJSONSimple)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Update(ctx, client, UpdateInput{ProjectID: "42", IssueIID: 10, NoteID: 100, Body: "test"})
	if err == nil {
		t.Fatal("Update() expected error for canceled context, got nil")
	}
}

// TestDelete_CancelledContext verifies Delete when cancelled context.
func TestDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx := testutil.CancelledCtx(t)
	err := Delete(ctx, client, DeleteInput{ProjectID: "42", IssueIID: 10, NoteID: 100})
	if err == nil {
		t.Fatal("Delete() expected error for canceled context, got nil")
	}
}

// ---------------------------------------------------------------------------
// API error - handlers not yet covered
// ---------------------------------------------------------------------------.

// TestGetNote_APIError verifies GetNote when API error.
func TestGetNote_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, "{\"message\":\"500 Internal Server Error\"}")
	}))
	_, err := GetNote(context.Background(), client, GetInput{ProjectID: "42", IssueIID: 10, NoteID: 100})
	if err == nil {
		t.Fatal("GetNote() expected error for API error, got nil")
	}
}

// TestUpdate_APIError verifies Update when API error.
func TestUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, "{\"message\":\"500 Internal Server Error\"}")
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "42", IssueIID: 10, NoteID: 100, Body: "x"})
	if err == nil {
		t.Fatal("Update() expected error for API error, got nil")
	}
}

// TestDelete_APIError verifies Delete when API error.
func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, "{\"message\":\"500 Internal Server Error\"}")
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: "42", IssueIID: 10, NoteID: 100})
	if err == nil {
		t.Fatal("Delete() expected error for API error, got nil")
	}
}

// ---------------------------------------------------------------------------
// List with all optional parameters
// ---------------------------------------------------------------------------.

// TestList_AllOptionalParams verifies List when all optional params.
func TestList_AllOptionalParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("order_by") != "updated_at" {
			t.Errorf("order_by = %q, want updated_at", q.Get("order_by"))
		}
		if q.Get("sort") != "desc" {
			t.Errorf("sort = %q, want desc", q.Get("sort"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, "["+noteJSONSimple+"]",
			testutil.PaginationHeaders{Page: "2", PerPage: "5", Total: "10", TotalPages: "2"})
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID:       "42",
		IssueIID:        10,
		OrderBy:         "updated_at",
		Sort:            "desc",
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 5},
	})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Notes) != 1 {
		t.Errorf("len(Notes) = %d, want 1", len(out.Notes))
	}
}

// ---------------------------------------------------------------------------
// Missing project_id for Create and List
// ---------------------------------------------------------------------------.

// TestCreate_MissingProjectID verifies Create when missing project ID.
func TestCreate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(context.Background(), client, CreateInput{IssueIID: 10, Body: "test"})
	if err == nil {
		t.Fatal("Create() expected error for missing project_id, got nil")
	}
}

// TestList_MissingProjectID verifies List when missing project ID.
func TestList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := List(context.Background(), client, ListInput{IssueIID: 10})
	if err == nil {
		t.Fatal("List() expected error for missing project_id, got nil")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------.

// TestCreate_CreatedAtBackdated verifies the created_at input is parsed and
// forwarded to the GitLab API as the backdated note timestamp.
func TestCreate_CreatedAtBackdated(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathIssueNotes {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if !strings.Contains(string(body), `"created_at":"2026-01-15T10:00:00Z"`) {
				t.Errorf("body = %s, want created_at 2026-01-15T10:00:00Z", body)
			}
			testutil.RespondJSON(w, http.StatusCreated, noteJSONSimple)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := Create(context.Background(), client, CreateInput{
		ProjectID: testProjectID,
		IssueIID:  10,
		Body:      testNoteLGTM,
		CreatedAt: "2026-01-15T10:00:00Z",
	})
	if err != nil {
		t.Fatalf(fmtIssueNoteCreateErr, err)
	}
}

// TestCreate_CreatedAtUnparseableIgnored verifies an unparseable created_at is
// silently dropped (ParseOptionalTime returns nil) and no created_at is sent.
func TestCreate_CreatedAtUnparseableIgnored(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathIssueNotes {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if strings.Contains(string(body), "created_at") {
				t.Errorf("body = %s, want no created_at", body)
			}
			testutil.RespondJSON(w, http.StatusCreated, noteJSONSimple)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := Create(context.Background(), client, CreateInput{
		ProjectID: testProjectID,
		IssueIID:  10,
		Body:      testNoteLGTM,
		CreatedAt: "not-a-timestamp",
	})
	if err != nil {
		t.Fatalf(fmtIssueNoteCreateErr, err)
	}
}

// TestList_KeysetPagination verifies the keyset pagination inputs (pagination
// and page_token) are forwarded to the GitLab API.
func TestList_KeysetPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathIssueNotes {
			q := r.URL.Query()
			if q.Get("pagination") != "keyset" {
				t.Errorf("pagination = %q, want keyset", q.Get("pagination"))
			}
			if q.Get("page_token") != "cursor99" {
				t.Errorf("page_token = %q, want cursor99", q.Get("page_token"))
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK, "["+noteJSONSimple+"]",
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID:             testProjectID,
		IssueIID:              10,
		KeysetPaginationInput: toolutil.KeysetPaginationInput{Pagination: "keyset", PageToken: "cursor99"},
	})
	if err != nil {
		t.Fatalf(fmtIssueNoteListErr, err)
	}
	if len(out.Notes) != 1 {
		t.Errorf("len(Notes) = %d, want 1", len(out.Notes))
	}
}

// TestToOutput_NestedObjects verifies the additive author/resolved_by/position
// sub-objects mirror the full gl.Note nested structs.
func TestToOutput_NestedObjects(t *testing.T) {
	resolvedAt := mustParseTime(t, "2026-03-02T08:00:00Z")
	expiresAt := mustParseTime(t, "2026-04-01T00:00:00Z")
	n := &gl.Note{
		ID:   400,
		Body: "Diff comment",
		Author: gl.NoteAuthor{
			ID: 7, Username: "alice", Email: "alice@example.com", Name: "Alice",
			State: "active", AvatarURL: "https://a/av.png", WebURL: "https://a/alice",
		},
		Attachment: "attach.png",
		Title:      "note title",
		FileName:   "note.txt",
		ExpiresAt:  expiresAt,
		Resolvable: true,
		Resolved:   true,
		ResolvedAt: resolvedAt,
		ResolvedBy: gl.NoteResolvedBy{ID: 9, Username: "bob", Name: "Bob"},
		Position: &gl.NotePosition{
			BaseSHA: "base", StartSHA: "start", HeadSHA: "head", PositionType: "text",
			NewPath: "main.go", NewLine: 12, OldPath: "main.go", OldLine: 10,
			LineRange: &gl.LineRange{
				StartRange: &gl.LinePosition{LineCode: "abc_1_2", Type: "new", OldLine: 0, NewLine: 12},
				EndRange:   &gl.LinePosition{LineCode: "abc_3_4", Type: "new", OldLine: 0, NewLine: 14},
			},
		},
	}

	out := ToOutput(n)

	if out.Author == nil || out.Author.ID != 7 || out.Author.Email != "alice@example.com" {
		t.Fatalf("Author = %+v", out.Author)
	}
	if out.Attachment != "attach.png" || out.Title != "note title" || out.FileName != "note.txt" {
		t.Errorf("string fields = %q/%q/%q", out.Attachment, out.Title, out.FileName)
	}
	if out.ExpiresAt != "2026-04-01T00:00:00Z" {
		t.Errorf("ExpiresAt = %q", out.ExpiresAt)
	}
	if out.ResolvedAt != "2026-03-02T08:00:00Z" {
		t.Errorf("ResolvedAt = %q", out.ResolvedAt)
	}
	if out.ResolvedBy == nil || out.ResolvedBy.Username != "bob" {
		t.Fatalf("ResolvedBy = %+v", out.ResolvedBy)
	}
	if out.Position == nil {
		t.Fatal("Position = nil, want populated")
	}
	if out.Position.NewPath != "main.go" || out.Position.NewLine != 12 {
		t.Errorf("Position = %+v", out.Position)
	}
	assertFullLineRange(t, out.Position.LineRange)
}

// assertFullLineRange checks the canonical start/end sub-objects of a fully
// populated line range.
func assertFullLineRange(t *testing.T, lr *LineRangeOutput) {
	t.Helper()
	// Canonical-key start/end objects mirror gl.LineRange.StartRange/EndRange.
	if lr == nil || lr.Start == nil || lr.End == nil {
		t.Fatalf("canonical start/end = %+v", lr)
	}
	if lr.Start.LineCode != "abc_1_2" || lr.Start.NewLine != 12 {
		t.Errorf("Start = %+v", lr.Start)
	}
	if lr.End.LineCode != "abc_3_4" || lr.End.NewLine != 14 {
		t.Errorf("End = %+v", lr.End)
	}
}

// TestToOutput_EmptyNestedObjects verifies nil/empty nested sources produce nil
// sub-objects (resolved_by absent, position absent) while the always-present
// author object is still populated.
func TestToOutput_EmptyNestedObjects(t *testing.T) {
	out := ToOutput(&gl.Note{ID: 1, Author: gl.NoteAuthor{Username: "u"}})
	if out.Author == nil || out.Author.Username != "u" {
		t.Errorf("Author = %+v, want non-nil with username u", out.Author)
	}
	if out.ResolvedBy != nil {
		t.Errorf("ResolvedBy = %+v, want nil", out.ResolvedBy)
	}
	if out.Position != nil {
		t.Errorf("Position = %+v, want nil", out.Position)
	}
}

// TestNotePositionOutput_LineRangeNilSubRanges verifies a LineRange with both
// sub-ranges nil collapses to a nil line_range object.
func TestNotePositionOutput_LineRangeNilSubRanges(t *testing.T) {
	out := notePositionOutput(&gl.NotePosition{
		PositionType: "text",
		LineRange:    &gl.LineRange{},
	})
	if out == nil {
		t.Fatal("position = nil, want populated")
	}
	if out.LineRange != nil {
		t.Errorf("LineRange = %+v, want nil (both sub-ranges empty)", out.LineRange)
	}
}

// TestNotePositionOutput_LineRangeStartOnly verifies a LineRange carrying only a
// start sub-range is preserved with a nil end.
func TestNotePositionOutput_LineRangeStartOnly(t *testing.T) {
	out := notePositionOutput(&gl.NotePosition{
		LineRange: &gl.LineRange{StartRange: &gl.LinePosition{LineCode: "x", NewLine: 3}},
	})
	if out == nil || out.LineRange == nil || out.LineRange.Start == nil {
		t.Fatalf("position/line range = %+v", out)
	}
	// Canonical-key Start mirrors the present sub-range; End is nil (nil-pointer
	// branch of linePositionOutput).
	if out.LineRange.Start == nil || out.LineRange.Start.LineCode != "x" || out.LineRange.Start.NewLine != 3 {
		t.Errorf("Start = %+v", out.LineRange.Start)
	}
	if out.LineRange.End != nil {
		t.Errorf("End = %+v, want nil", out.LineRange.End)
	}
}

// TestNotePositionOutput_NoLineRange verifies a position with no line range
// produces a populated position object whose line_range is nil.
func TestNotePositionOutput_NoLineRange(t *testing.T) {
	out := notePositionOutput(&gl.NotePosition{PositionType: "text", NewPath: "f.go"})
	if out == nil {
		t.Fatal("position = nil, want populated")
	}
	if out.LineRange != nil {
		t.Errorf("LineRange = %+v, want nil", out.LineRange)
	}
}

// TestNotePositionOutput_LineRangeEndOnly verifies a LineRange carrying only an
// end sub-range is preserved with a nil start.
func TestNotePositionOutput_LineRangeEndOnly(t *testing.T) {
	out := notePositionOutput(&gl.NotePosition{
		LineRange: &gl.LineRange{EndRange: &gl.LinePosition{LineCode: "y", NewLine: 5}},
	})
	if out == nil || out.LineRange == nil || out.LineRange.End == nil {
		t.Fatalf("position/line range = %+v", out)
	}
	// Canonical-key End mirrors the present sub-range; Start is nil (nil-pointer
	// branch of linePositionOutput).
	if out.LineRange.End == nil || out.LineRange.End.LineCode != "y" || out.LineRange.End.NewLine != 5 {
		t.Errorf("End = %+v", out.LineRange.End)
	}
	if out.LineRange.Start != nil {
		t.Errorf("Start = %+v, want nil", out.LineRange.Start)
	}
}

// mustParseTime prepares parse time test fixtures and fails the test on error.
func mustParseTime(t *testing.T, s string) *time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("mustParseTime(%q): %v", s, err)
	}
	return &ts
}
