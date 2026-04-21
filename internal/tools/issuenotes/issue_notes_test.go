// Package issuenotes contains unit tests for GitLab issue note (comment)
// operations (create and list). Tests use httptest to mock the GitLab API
// and verify success paths, internal notes, system notes, pagination, and
// error handling.
package issuenotes

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"
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

// TestIssueNoteCreate_Success verifies the behavior of issue note create success.
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
	if out.Author != "alice" {
		t.Errorf("out.Author = %q, want %q", out.Author, "alice")
	}
	if out.Internal {
		t.Error("out.Internal = true, want false")
	}
	if out.System {
		t.Error("out.System = true, want false")
	}
}

// TestIssueNote_CreateInternal verifies the behavior of issue note create internal.
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

// TestIssueNoteCreate_APIError verifies the behavior of issue note create a p i error.
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

// TestIssueNoteCreate_CancelledContext verifies the behavior of issue note create cancelled context.
func TestIssueNoteCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Create(ctx, client, CreateInput{
		ProjectID: testProjectID,
		IssueIID:  10,
		Body:      "test",
	})
	if err == nil {
		t.Fatal("Create() expected error for canceled context, got nil")
	}
}

// TestIssueNoteList_Success verifies the behavior of issue note list success.
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

// TestIssueNoteList_Empty verifies the behavior of issue note list empty.
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

// TestIssueNoteList_Pagination verifies the behavior of issue note list pagination.
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

// TestIssueNoteCreate_SuccessEnrichedFields verifies the behavior of issue note create success enriched fields.
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

// TestIssueNoteList_APIError verifies the behavior of issue note list a p i error.
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

const pathIssueNote100 = "/api/v4/projects/42/issues/10/notes/100"

// TestGetNote_Success verifies the behavior of get note success.
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

// TestGetNote_MissingProjectID verifies the behavior of get note missing project i d.
func TestGetNote_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := GetNote(context.Background(), client, GetInput{IssueIID: 10, NoteID: 100})
	if err == nil {
		t.Fatal("GetNote() expected error for missing project_id, got nil")
	}
}

// TestUpdate_Success verifies the behavior of update success.
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

// TestUpdate_MissingProjectID verifies the behavior of update missing project i d.
func TestUpdate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Update(context.Background(), client, UpdateInput{IssueIID: 10, NoteID: 100, Body: "test"})
	if err == nil {
		t.Fatal("Update() expected error for missing project_id, got nil")
	}
}

// TestDelete_Success verifies the behavior of delete success.
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

// TestDelete_MissingProjectID verifies the behavior of delete missing project i d.
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

// TestFormatOutputMarkdown_Populated validates format output markdown populated across multiple scenarios using table-driven subtests.
func TestFormatOutputMarkdown_Populated(t *testing.T) {
	out := Output{
		ID:         200,
		Body:       "Full note body",
		Author:     "fulluser",
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

// TestFormatOutputMarkdown_ResolvableUnresolved verifies the behavior of format output markdown resolvable unresolved.
func TestFormatOutputMarkdown_ResolvableUnresolved(t *testing.T) {
	out := Output{
		ID:         201,
		Body:       "Unresolved note",
		Author:     "reviewer",
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

// TestFormatOutputMarkdown_Minimal verifies the behavior of format output markdown minimal.
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

// TestFormatListMarkdown_Populated validates format list markdown populated across multiple scenarios using table-driven subtests.
func TestFormatListMarkdown_Populated(t *testing.T) {
	out := ListOutput{
		Notes: []Output{
			{ID: 1, Author: "alice", CreatedAt: "2026-01-01T00:00:00Z", System: false, Internal: false},
			{ID: 2, Author: "bob", CreatedAt: "2026-01-02T00:00:00Z", System: true, Internal: true},
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

// TestFormatListMarkdown_Empty verifies the behavior of format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No issue notes found.") {
		t.Error("missing empty-state message")
	}
}

// ---------------------------------------------------------------------------
// ToOutput - full fields
// ---------------------------------------------------------------------------.

// TestToOutput_AllFields verifies the behavior of to output all fields.
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
	if out.Author != "fulluser" {
		t.Errorf("Author = %q", out.Author)
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

// TestToOutput_NilTimestamps verifies the behavior of to output nil timestamps.
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

// TestList_CancelledContext verifies the behavior of list cancelled context.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := List(ctx, client, ListInput{ProjectID: "42", IssueIID: 10})
	if err == nil {
		t.Fatal("List() expected error for canceled context, got nil")
	}
}

// TestGetNote_CancelledContext verifies the behavior of get note cancelled context.
func TestGetNote_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, noteJSONSimple)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := GetNote(ctx, client, GetInput{ProjectID: "42", IssueIID: 10, NoteID: 100})
	if err == nil {
		t.Fatal("GetNote() expected error for canceled context, got nil")
	}
}

// TestUpdate_CancelledContext verifies the behavior of update cancelled context.
func TestUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, noteJSONSimple)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Update(ctx, client, UpdateInput{ProjectID: "42", IssueIID: 10, NoteID: 100, Body: "test"})
	if err == nil {
		t.Fatal("Update() expected error for canceled context, got nil")
	}
}

// TestDelete_CancelledContext verifies the behavior of delete cancelled context.
func TestDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Delete(ctx, client, DeleteInput{ProjectID: "42", IssueIID: 10, NoteID: 100})
	if err == nil {
		t.Fatal("Delete() expected error for canceled context, got nil")
	}
}

// ---------------------------------------------------------------------------
// API error - handlers not yet covered
// ---------------------------------------------------------------------------.

// TestGetNote_APIError verifies the behavior of get note a p i error.
func TestGetNote_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, "{\"message\":\"500 Internal Server Error\"}")
	}))
	_, err := GetNote(context.Background(), client, GetInput{ProjectID: "42", IssueIID: 10, NoteID: 100})
	if err == nil {
		t.Fatal("GetNote() expected error for API error, got nil")
	}
}

// TestUpdate_APIError verifies the behavior of update a p i error.
func TestUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, "{\"message\":\"500 Internal Server Error\"}")
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "42", IssueIID: 10, NoteID: 100, Body: "x"})
	if err == nil {
		t.Fatal("Update() expected error for API error, got nil")
	}
}

// TestDelete_APIError verifies the behavior of delete a p i error.
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

// TestList_AllOptionalParams verifies the behavior of list all optional params.
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

// TestCreate_MissingProjectID verifies the behavior of create missing project i d.
func TestCreate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(context.Background(), client, CreateInput{IssueIID: 10, Body: "test"})
	if err == nil {
		t.Fatal("Create() expected error for missing project_id, got nil")
	}
}

// TestList_MissingProjectID verifies the behavior of list missing project i d.
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
// MCP integration - RegisterTools + CallTool
// ---------------------------------------------------------------------------.

// newIssueNotesMCPSession is an internal helper for the issuenotes package.
func newIssueNotesMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodPost && path == pathIssueNotes:
			testutil.RespondJSON(w, http.StatusCreated, noteJSONSimple)
		case r.Method == http.MethodGet && path == pathIssueNotes:
			testutil.RespondJSONWithPagination(w, http.StatusOK, "["+noteJSONSimple+"]",
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
		case r.Method == http.MethodGet && path == pathIssueNote100:
			testutil.RespondJSON(w, http.StatusOK, noteJSONSimple)
		case r.Method == http.MethodPut && path == pathIssueNote100:
			testutil.RespondJSON(w, http.StatusOK, noteJSONSimple)
		case r.Method == http.MethodDelete && path == pathIssueNote100:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))

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

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newIssueNotesMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_issue_note_create", map[string]any{"project_id": "42", "issue_iid": 10, "body": "test note"}},
		{"gitlab_issue_note_list", map[string]any{"project_id": "42", "issue_iid": 10}},
		{"gitlab_issue_note_get", map[string]any{"project_id": "42", "issue_iid": 10, "note_id": 100}},
		{"gitlab_issue_note_update", map[string]any{"project_id": "42", "issue_iid": 10, "note_id": 100, "body": "updated"}},
		{"gitlab_issue_note_delete", map[string]any{"project_id": "42", "issue_iid": 10, "note_id": 100}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.name,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.name, err)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(%s) returned error: %s", tt.name, tc.Text)
					}
				}
				t.Fatalf("CallTool(%s) returned IsError=true", tt.name)
			}
		})
	}
}

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------.

// mustParseTime is an internal helper for the issuenotes package.
func mustParseTime(t *testing.T, s string) *time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("mustParseTime(%q): %v", s, err)
	}
	return &ts
}
