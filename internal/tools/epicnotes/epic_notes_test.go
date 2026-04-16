// Package epicnotes tests validate all epic note MCP tool handlers:
// List, Get, Create, Update, and Delete. Tests cover success paths, input
// validation (missing group_id, epic_iid, note_id, body), API error responses
// (403, 404, 500), cancelled contexts, pagination options, empty results,
// and markdown formatting for both single output and list output.
package epicnotes

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const (
	pathEpicNotes   = "/api/v4/groups/mygroup/epics/1/notes"
	pathEpicNote100 = "/api/v4/groups/mygroup/epics/1/notes/100"

	noteJSON = `{
		"id": 100,
		"body": "This looks good",
		"author": {"username": "alice"},
		"system": false,
		"noteable_type": "Epic",
		"noteable_id": 1,
		"created_at": "2026-01-15T10:00:00Z",
		"updated_at": "2026-01-15T10:00:00Z"
	}`

	noteSystemJSON = `{
		"id": 101,
		"body": "changed the description",
		"author": {"username": "admin"},
		"system": true,
		"noteable_type": "Epic",
		"noteable_id": 1,
		"created_at": "2026-01-15T12:00:00Z",
		"updated_at": "2026-01-15T12:00:00Z"
	}`

	testGroupID = "mygroup"
)

// TestList validates List handler across success, validation, API error,
// pagination, empty results, and context cancellation scenarios.
func TestList(t *testing.T) {
	tests := []struct {
		name     string
		input    ListInput
		handler  http.HandlerFunc
		cancelFn bool
		wantErr  bool
		validate func(t *testing.T, out ListOutput)
	}{
		{
			name:  "returns notes with correct fields",
			input: ListInput{GroupID: testGroupID, EpicIID: 1},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, pathEpicNotes)
				testutil.RespondJSON(w, http.StatusOK, "["+noteJSON+","+noteSystemJSON+"]")
			},
			validate: func(t *testing.T, out ListOutput) {
				t.Helper()
				if len(out.Notes) != 2 {
					t.Fatalf("len(Notes) = %d, want 2", len(out.Notes))
				}
				if out.Notes[0].ID != 100 {
					t.Errorf("Notes[0].ID = %d, want 100", out.Notes[0].ID)
				}
				if out.Notes[0].Body != "This looks good" {
					t.Errorf("Notes[0].Body = %q, want %q", out.Notes[0].Body, "This looks good")
				}
				if out.Notes[0].Author != "alice" {
					t.Errorf("Notes[0].Author = %q, want %q", out.Notes[0].Author, "alice")
				}
				if out.Notes[1].System != true {
					t.Error("Notes[1].System = false, want true")
				}
				if out.Notes[1].ID != 101 {
					t.Errorf("Notes[1].ID = %d, want 101", out.Notes[1].ID)
				}
			},
		},
		{
			name: "passes pagination and ordering query params",
			input: ListInput{
				GroupID: testGroupID,
				EpicIID: 1,
				OrderBy: "updated_at",
				Sort:    "desc",
				PaginationInput: toolutil.PaginationInput{
					Page:    2,
					PerPage: 10,
				},
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertQueryParam(t, r, "order_by", "updated_at")
				testutil.AssertQueryParam(t, r, "sort", "desc")
				testutil.AssertQueryParam(t, r, "page", "2")
				testutil.AssertQueryParam(t, r, "per_page", "10")
				testutil.RespondJSONWithPagination(w, http.StatusOK, "["+noteJSON+"]", testutil.PaginationHeaders{
					Page: "2", PerPage: "10", Total: "15", TotalPages: "2",
				})
			},
			validate: func(t *testing.T, out ListOutput) {
				t.Helper()
				if len(out.Notes) != 1 {
					t.Fatalf("len(Notes) = %d, want 1", len(out.Notes))
				}
				if out.Pagination.TotalItems != 15 {
					t.Errorf("TotalItems = %d, want 15", out.Pagination.TotalItems)
				}
			},
		},
		{
			name:  "returns empty list when no notes exist",
			input: ListInput{GroupID: testGroupID, EpicIID: 1},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, "[]")
			},
			validate: func(t *testing.T, out ListOutput) {
				t.Helper()
				if len(out.Notes) != 0 {
					t.Errorf("len(Notes) = %d, want 0", len(out.Notes))
				}
			},
		},
		{
			name:    "returns error when group_id is empty",
			input:   ListInput{EpicIID: 1},
			handler: func(w http.ResponseWriter, _ *http.Request) {},
			wantErr: true,
		},
		{
			name:    "returns error when epic_iid is zero",
			input:   ListInput{GroupID: testGroupID},
			handler: func(w http.ResponseWriter, _ *http.Request) {},
			wantErr: true,
		},
		{
			name:    "returns error when epic_iid is negative",
			input:   ListInput{GroupID: testGroupID, EpicIID: -1},
			handler: func(w http.ResponseWriter, _ *http.Request) {},
			wantErr: true,
		},
		{
			name:  "returns error on API 500",
			input: ListInput{GroupID: testGroupID, EpicIID: 1},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"500 Internal Server Error"}`)
			},
			wantErr: true,
		},
		{
			name:     "returns error on cancelled context",
			input:    ListInput{GroupID: testGroupID, EpicIID: 1},
			handler:  func(w http.ResponseWriter, _ *http.Request) {},
			cancelFn: true,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			ctx := context.Background()
			if tt.cancelFn {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			out, err := List(ctx, client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("List() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.validate != nil {
				tt.validate(t, out)
			}
		})
	}
}

// TestGet validates Get handler across success, all validation errors,
// API errors, and context cancellation.
func TestGet(t *testing.T) {
	tests := []struct {
		name     string
		input    GetInput
		handler  http.HandlerFunc
		cancelFn bool
		wantErr  bool
		validate func(t *testing.T, out Output)
	}{
		{
			name:  "returns note with all fields populated",
			input: GetInput{GroupID: testGroupID, EpicIID: 1, NoteID: 100},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, pathEpicNote100)
				testutil.RespondJSON(w, http.StatusOK, noteJSON)
			},
			validate: func(t *testing.T, out Output) {
				t.Helper()
				if out.ID != 100 {
					t.Errorf("ID = %d, want 100", out.ID)
				}
				if out.Author != "alice" {
					t.Errorf("Author = %q, want %q", out.Author, "alice")
				}
				if out.Body != "This looks good" {
					t.Errorf("Body = %q, want %q", out.Body, "This looks good")
				}
				if out.NoteableType != "Epic" {
					t.Errorf("NoteableType = %q, want %q", out.NoteableType, "Epic")
				}
				if out.NoteableID != 1 {
					t.Errorf("NoteableID = %d, want 1", out.NoteableID)
				}
				if out.CreatedAt == "" {
					t.Error("CreatedAt is empty, want non-empty")
				}
			},
		},
		{
			name:    "returns error when group_id is empty",
			input:   GetInput{EpicIID: 1, NoteID: 100},
			handler: func(w http.ResponseWriter, _ *http.Request) {},
			wantErr: true,
		},
		{
			name:    "returns error when epic_iid is zero",
			input:   GetInput{GroupID: testGroupID, NoteID: 100},
			handler: func(w http.ResponseWriter, _ *http.Request) {},
			wantErr: true,
		},
		{
			name:    "returns error when note_id is zero",
			input:   GetInput{GroupID: testGroupID, EpicIID: 1},
			handler: func(w http.ResponseWriter, _ *http.Request) {},
			wantErr: true,
		},
		{
			name:  "returns error on API 404",
			input: GetInput{GroupID: testGroupID, EpicIID: 1, NoteID: 999},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
			},
			wantErr: true,
		},
		{
			name:     "returns error on cancelled context",
			input:    GetInput{GroupID: testGroupID, EpicIID: 1, NoteID: 100},
			handler:  func(w http.ResponseWriter, _ *http.Request) {},
			cancelFn: true,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			ctx := context.Background()
			if tt.cancelFn {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			out, err := Get(ctx, client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Get() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.validate != nil {
				tt.validate(t, out)
			}
		})
	}
}

// TestCreate validates Create handler across success, all validation errors,
// API errors, and context cancellation.
func TestCreate(t *testing.T) {
	tests := []struct {
		name     string
		input    CreateInput
		handler  http.HandlerFunc
		cancelFn bool
		wantErr  bool
		validate func(t *testing.T, out Output)
	}{
		{
			name:  "creates note and returns output",
			input: CreateInput{GroupID: testGroupID, EpicIID: 1, Body: "This looks good"},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPost)
				testutil.AssertRequestPath(t, r, pathEpicNotes)
				testutil.RespondJSON(w, http.StatusCreated, noteJSON)
			},
			validate: func(t *testing.T, out Output) {
				t.Helper()
				if out.ID != 100 {
					t.Errorf("ID = %d, want 100", out.ID)
				}
				if out.Body != "This looks good" {
					t.Errorf("Body = %q, want %q", out.Body, "This looks good")
				}
			},
		},
		{
			name:    "returns error when group_id is empty",
			input:   CreateInput{EpicIID: 1, Body: "note"},
			handler: func(w http.ResponseWriter, _ *http.Request) {},
			wantErr: true,
		},
		{
			name:    "returns error when epic_iid is zero",
			input:   CreateInput{GroupID: testGroupID, Body: "note"},
			handler: func(w http.ResponseWriter, _ *http.Request) {},
			wantErr: true,
		},
		{
			name:    "returns error when body is empty",
			input:   CreateInput{GroupID: testGroupID, EpicIID: 1},
			handler: func(w http.ResponseWriter, _ *http.Request) {},
			wantErr: true,
		},
		{
			name:  "returns error on API 403",
			input: CreateInput{GroupID: testGroupID, EpicIID: 1, Body: "note"},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
			},
			wantErr: true,
		},
		{
			name:     "returns error on cancelled context",
			input:    CreateInput{GroupID: testGroupID, EpicIID: 1, Body: "note"},
			handler:  func(w http.ResponseWriter, _ *http.Request) {},
			cancelFn: true,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			ctx := context.Background()
			if tt.cancelFn {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			out, err := Create(ctx, client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.validate != nil {
				tt.validate(t, out)
			}
		})
	}
}

// TestUpdate validates Update handler across success, all validation errors,
// API errors, and context cancellation.
func TestUpdate(t *testing.T) {
	tests := []struct {
		name     string
		input    UpdateInput
		handler  http.HandlerFunc
		cancelFn bool
		wantErr  bool
		validate func(t *testing.T, out Output)
	}{
		{
			name:  "updates note and returns output",
			input: UpdateInput{GroupID: testGroupID, EpicIID: 1, NoteID: 100, Body: "Updated"},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPut)
				testutil.AssertRequestPath(t, r, pathEpicNote100)
				testutil.RespondJSON(w, http.StatusOK, noteJSON)
			},
			validate: func(t *testing.T, out Output) {
				t.Helper()
				if out.ID != 100 {
					t.Errorf("ID = %d, want 100", out.ID)
				}
			},
		},
		{
			name:    "returns error when group_id is empty",
			input:   UpdateInput{EpicIID: 1, NoteID: 100, Body: "x"},
			handler: func(w http.ResponseWriter, _ *http.Request) {},
			wantErr: true,
		},
		{
			name:    "returns error when epic_iid is zero",
			input:   UpdateInput{GroupID: testGroupID, NoteID: 100, Body: "x"},
			handler: func(w http.ResponseWriter, _ *http.Request) {},
			wantErr: true,
		},
		{
			name:    "returns error when note_id is zero",
			input:   UpdateInput{GroupID: testGroupID, EpicIID: 1, Body: "x"},
			handler: func(w http.ResponseWriter, _ *http.Request) {},
			wantErr: true,
		},
		{
			name:  "returns error on API 500",
			input: UpdateInput{GroupID: testGroupID, EpicIID: 1, NoteID: 100, Body: "x"},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"500 Internal Server Error"}`)
			},
			wantErr: true,
		},
		{
			name:     "returns error on cancelled context",
			input:    UpdateInput{GroupID: testGroupID, EpicIID: 1, NoteID: 100, Body: "x"},
			handler:  func(w http.ResponseWriter, _ *http.Request) {},
			cancelFn: true,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			ctx := context.Background()
			if tt.cancelFn {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			out, err := Update(ctx, client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Update() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.validate != nil {
				tt.validate(t, out)
			}
		})
	}
}

// TestDelete validates Delete handler across success, all validation errors,
// API errors, and context cancellation.
func TestDelete(t *testing.T) {
	tests := []struct {
		name     string
		input    DeleteInput
		handler  http.HandlerFunc
		cancelFn bool
		wantErr  bool
	}{
		{
			name:  "deletes note successfully",
			input: DeleteInput{GroupID: testGroupID, EpicIID: 1, NoteID: 100},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodDelete)
				testutil.AssertRequestPath(t, r, pathEpicNote100)
				w.WriteHeader(http.StatusNoContent)
			},
		},
		{
			name:    "returns error when group_id is empty",
			input:   DeleteInput{EpicIID: 1, NoteID: 100},
			handler: func(w http.ResponseWriter, _ *http.Request) {},
			wantErr: true,
		},
		{
			name:    "returns error when epic_iid is zero",
			input:   DeleteInput{GroupID: testGroupID, NoteID: 100},
			handler: func(w http.ResponseWriter, _ *http.Request) {},
			wantErr: true,
		},
		{
			name:    "returns error when note_id is zero",
			input:   DeleteInput{GroupID: testGroupID, EpicIID: 1},
			handler: func(w http.ResponseWriter, _ *http.Request) {},
			wantErr: true,
		},
		{
			name:  "returns error on API 403",
			input: DeleteInput{GroupID: testGroupID, EpicIID: 1, NoteID: 100},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
			},
			wantErr: true,
		},
		{
			name:     "returns error on cancelled context",
			input:    DeleteInput{GroupID: testGroupID, EpicIID: 1, NoteID: 100},
			handler:  func(w http.ResponseWriter, _ *http.Request) {},
			cancelFn: true,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			ctx := context.Background()
			if tt.cancelFn {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			err := Delete(ctx, client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Delete() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestFormatOutputMarkdown validates Markdown rendering of a single epic
// note, covering both regular and system notes.
func TestFormatOutputMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    Output
		contains []string
	}{
		{
			name: "renders regular note with author and body",
			input: Output{
				ID:        100,
				Body:      "This looks good",
				Author:    "alice",
				CreatedAt: "2026-01-15T10:00:00Z",
				System:    false,
			},
			contains: []string{
				"## Epic Note #100",
				"alice",
				"This looks good",
				"epic_note_update",
				"epic_note_delete",
			},
		},
		{
			name: "renders system note with system flag",
			input: Output{
				ID:        101,
				Body:      "changed the description",
				Author:    "admin",
				CreatedAt: "2026-01-15T12:00:00Z",
				System:    true,
			},
			contains: []string{
				"## Epic Note #101",
				"**System note**",
				"changed the description",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := FormatOutputMarkdown(tt.input)
			for _, want := range tt.contains {
				if !strings.Contains(md, want) {
					t.Errorf("markdown missing %q\ngot:\n%s", want, md)
				}
			}
		})
	}
}

// TestFormatListMarkdown validates Markdown rendering of a paginated list
// of epic notes, including the zero-notes scenario.
func TestFormatListMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    ListOutput
		contains []string
	}{
		{
			name: "renders table with notes",
			input: ListOutput{
				Notes: []Output{
					{ID: 100, Author: "alice", CreatedAt: "2026-01-15T10:00:00Z", System: false},
					{ID: 101, Author: "admin", CreatedAt: "2026-01-15T12:00:00Z", System: true},
				},
				Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
			},
			contains: []string{
				"## Epic Notes (2)",
				"| ID | Author | Created | System |",
				"| 100 |",
				"| 101 |",
				"alice",
				"admin",
				"epic_note_get",
				"epic_note_create",
			},
		},
		{
			name: "renders empty state when no notes",
			input: ListOutput{
				Notes:      []Output{},
				Pagination: toolutil.PaginationOutput{TotalItems: 0, Page: 1, PerPage: 20, TotalPages: 0},
			},
			contains: []string{
				"## Epic Notes (0)",
				"No epic notes found.",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := FormatListMarkdown(tt.input)
			for _, want := range tt.contains {
				if !strings.Contains(md, want) {
					t.Errorf("markdown missing %q\ngot:\n%s", want, md)
				}
			}
		})
	}
}
