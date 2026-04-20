// Package epicnotes tests validate all epic note MCP tool handlers:
// List, Get, Create, Update, and Delete using the Work Items GraphQL API.
// Tests cover success paths, input validation (missing full_path, iid,
// note_id, body), API error responses, cancelled contexts, pagination,
// empty results, and markdown formatting for both single and list output.
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
	testFullPath = "my-group"

	// GraphQL response for a notes widget with two notes across one discussion.
	gqlNotesData = `{
		"namespace": {
			"workItem": {
				"id": "gid://gitlab/WorkItem/1",
				"widgets": [{
					"discussions": {
						"pageInfo": {"hasNextPage": false, "hasPreviousPage": false, "endCursor": null, "startCursor": null},
						"nodes": [{
							"notes": {
								"nodes": [
									{"id": "gid://gitlab/Note/100", "body": "This looks good", "author": {"username": "alice"}, "system": false, "createdAt": "2026-01-15T10:00:00Z", "updatedAt": "2026-01-15T10:00:00Z"},
									{"id": "gid://gitlab/Note/101", "body": "changed the description", "author": {"username": "admin"}, "system": true, "createdAt": "2026-01-15T12:00:00Z", "updatedAt": "2026-01-15T12:00:00Z"}
								]
							}
						}]
					}
				}]
			}
		}
	}`

	// GraphQL response with no notes.
	gqlNotesEmptyData = `{
		"namespace": {
			"workItem": {
				"id": "gid://gitlab/WorkItem/1",
				"widgets": [{
					"discussions": {
						"pageInfo": {"hasNextPage": false, "hasPreviousPage": false, "endCursor": null, "startCursor": null},
						"nodes": []
					}
				}]
			}
		}
	}`

	// GraphQL response for namespace not found.
	gqlNamespaceNull = `{"namespace": null}`

	// GraphQL response for createNote mutation.
	gqlCreateNoteData = `{
		"createNote": {
			"note": {"id": "gid://gitlab/Note/200", "body": "New comment", "author": {"username": "alice"}, "system": false, "createdAt": "2026-01-16T10:00:00Z", "updatedAt": "2026-01-16T10:00:00Z"},
			"errors": []
		}
	}`

	// GraphQL response for updateNote mutation.
	gqlUpdateNoteData = `{
		"updateNote": {
			"note": {"id": "gid://gitlab/Note/100", "body": "Updated comment", "author": {"username": "alice"}, "system": false, "createdAt": "2026-01-15T10:00:00Z", "updatedAt": "2026-01-16T11:00:00Z"},
			"errors": []
		}
	}`

	// GraphQL response for destroyNote mutation.
	gqlDestroyNoteData = `{
		"destroyNote": {
			"note": {"id": "gid://gitlab/Note/100"},
			"errors": []
		}
	}`

	// GraphQL response for resolveWorkItemGID.
	gqlWorkItemGIDData = `{
		"namespace": {
			"workItem": {
				"id": "gid://gitlab/WorkItem/1"
			}
		}
	}`
)

// graphqlMux creates a handler that routes GraphQL requests by query content.
func graphqlMux(handlers map[string]http.HandlerFunc) http.Handler {
	return testutil.GraphQLHandler(handlers)
}

// TestList validates List handler across success, validation, API error,
// empty results, and context cancellation scenarios.
func TestList(t *testing.T) {
	tests := []struct {
		name     string
		input    ListInput
		handler  http.Handler
		cancelFn bool
		wantErr  bool
		validate func(t *testing.T, out ListOutput)
	}{
		{
			name:  "returns notes with correct fields",
			input: ListInput{FullPath: testFullPath, IID: 1},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlNotesData)
				},
			}),
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
			name:  "returns empty list when no notes exist",
			input: ListInput{FullPath: testFullPath, IID: 1},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlNotesEmptyData)
				},
			}),
			validate: func(t *testing.T, out ListOutput) {
				t.Helper()
				if len(out.Notes) != 0 {
					t.Errorf("len(Notes) = %d, want 0", len(out.Notes))
				}
			},
		},
		{
			name:  "returns error when epic not found",
			input: ListInput{FullPath: testFullPath, IID: 999},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlNamespaceNull)
				},
			}),
			wantErr: true,
		},
		{
			name:    "returns error when full_path is empty",
			input:   ListInput{IID: 1},
			handler: http.NotFoundHandler(),
			wantErr: true,
		},
		{
			name:    "returns error when iid is zero",
			input:   ListInput{FullPath: testFullPath},
			handler: http.NotFoundHandler(),
			wantErr: true,
		},
		{
			name:    "returns error when iid is negative",
			input:   ListInput{FullPath: testFullPath, IID: -1},
			handler: http.NotFoundHandler(),
			wantErr: true,
		},
		{
			name:  "returns error on API server error",
			input: ListInput{FullPath: testFullPath, IID: 1},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
					http.Error(w, "internal error", http.StatusInternalServerError)
				},
			}),
			wantErr: true,
		},
		{
			name:     "returns error on cancelled context",
			input:    ListInput{FullPath: testFullPath, IID: 1},
			handler:  http.NotFoundHandler(),
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
		handler  http.Handler
		cancelFn bool
		wantErr  bool
		validate func(t *testing.T, out Output)
	}{
		{
			name:  "returns note with all fields populated",
			input: GetInput{FullPath: testFullPath, IID: 1, NoteID: 100},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlNotesData)
				},
			}),
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
				if out.CreatedAt == "" {
					t.Error("CreatedAt is empty, want non-empty")
				}
			},
		},
		{
			name:  "returns error when note not found",
			input: GetInput{FullPath: testFullPath, IID: 1, NoteID: 999},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlNotesData)
				},
			}),
			wantErr: true,
		},
		{
			name:    "returns error when full_path is empty",
			input:   GetInput{IID: 1, NoteID: 100},
			handler: http.NotFoundHandler(),
			wantErr: true,
		},
		{
			name:    "returns error when iid is zero",
			input:   GetInput{FullPath: testFullPath, NoteID: 100},
			handler: http.NotFoundHandler(),
			wantErr: true,
		},
		{
			name:    "returns error when note_id is zero",
			input:   GetInput{FullPath: testFullPath, IID: 1},
			handler: http.NotFoundHandler(),
			wantErr: true,
		},
		{
			name:  "returns error when epic not found",
			input: GetInput{FullPath: testFullPath, IID: 999, NoteID: 100},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlNamespaceNull)
				},
			}),
			wantErr: true,
		},
		{
			name:     "returns error on cancelled context",
			input:    GetInput{FullPath: testFullPath, IID: 1, NoteID: 100},
			handler:  http.NotFoundHandler(),
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
		handler  http.Handler
		cancelFn bool
		wantErr  bool
		validate func(t *testing.T, out Output)
	}{
		{
			name:  "creates note and returns output",
			input: CreateInput{FullPath: testFullPath, IID: 1, Body: "New comment"},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"workItem(iid": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlWorkItemGIDData)
				},
				"createNote": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlCreateNoteData)
				},
			}),
			validate: func(t *testing.T, out Output) {
				t.Helper()
				if out.ID != 200 {
					t.Errorf("ID = %d, want 200", out.ID)
				}
				if out.Body != "New comment" {
					t.Errorf("Body = %q, want %q", out.Body, "New comment")
				}
			},
		},
		{
			name:    "returns error when full_path is empty",
			input:   CreateInput{IID: 1, Body: "note"},
			handler: http.NotFoundHandler(),
			wantErr: true,
		},
		{
			name:    "returns error when iid is zero",
			input:   CreateInput{FullPath: testFullPath, Body: "note"},
			handler: http.NotFoundHandler(),
			wantErr: true,
		},
		{
			name:    "returns error when body is empty",
			input:   CreateInput{FullPath: testFullPath, IID: 1},
			handler: http.NotFoundHandler(),
			wantErr: true,
		},
		{
			name:  "returns error on GraphQL mutation errors",
			input: CreateInput{FullPath: testFullPath, IID: 1, Body: "note"},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"workItem(iid": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlWorkItemGIDData)
				},
				"createNote": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, `{"createNote": {"note": null, "errors": ["Body is too short"]}}`)
				},
			}),
			wantErr: true,
		},
		{
			name:     "returns error on cancelled context",
			input:    CreateInput{FullPath: testFullPath, IID: 1, Body: "note"},
			handler:  http.NotFoundHandler(),
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
		handler  http.Handler
		cancelFn bool
		wantErr  bool
		validate func(t *testing.T, out Output)
	}{
		{
			name:  "updates note and returns output",
			input: UpdateInput{FullPath: testFullPath, IID: 1, NoteID: 100, Body: "Updated"},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"updateNote": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlUpdateNoteData)
				},
			}),
			validate: func(t *testing.T, out Output) {
				t.Helper()
				if out.ID != 100 {
					t.Errorf("ID = %d, want 100", out.ID)
				}
				if out.Body != "Updated comment" {
					t.Errorf("Body = %q, want %q", out.Body, "Updated comment")
				}
			},
		},
		{
			name:    "returns error when full_path is empty",
			input:   UpdateInput{IID: 1, NoteID: 100, Body: "x"},
			handler: http.NotFoundHandler(),
			wantErr: true,
		},
		{
			name:    "returns error when iid is zero",
			input:   UpdateInput{FullPath: testFullPath, NoteID: 100, Body: "x"},
			handler: http.NotFoundHandler(),
			wantErr: true,
		},
		{
			name:    "returns error when note_id is zero",
			input:   UpdateInput{FullPath: testFullPath, IID: 1, Body: "x"},
			handler: http.NotFoundHandler(),
			wantErr: true,
		},
		{
			name:    "returns error when body is empty",
			input:   UpdateInput{FullPath: testFullPath, IID: 1, NoteID: 100},
			handler: http.NotFoundHandler(),
			wantErr: true,
		},
		{
			name:  "returns error on GraphQL mutation errors",
			input: UpdateInput{FullPath: testFullPath, IID: 1, NoteID: 100, Body: "x"},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"updateNote": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, `{"updateNote": {"note": null, "errors": ["Permission denied"]}}`)
				},
			}),
			wantErr: true,
		},
		{
			name:     "returns error on cancelled context",
			input:    UpdateInput{FullPath: testFullPath, IID: 1, NoteID: 100, Body: "x"},
			handler:  http.NotFoundHandler(),
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
		handler  http.Handler
		cancelFn bool
		wantErr  bool
	}{
		{
			name:  "deletes note successfully",
			input: DeleteInput{FullPath: testFullPath, IID: 1, NoteID: 100},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"destroyNote": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlDestroyNoteData)
				},
			}),
		},
		{
			name:    "returns error when full_path is empty",
			input:   DeleteInput{IID: 1, NoteID: 100},
			handler: http.NotFoundHandler(),
			wantErr: true,
		},
		{
			name:    "returns error when iid is zero",
			input:   DeleteInput{FullPath: testFullPath, NoteID: 100},
			handler: http.NotFoundHandler(),
			wantErr: true,
		},
		{
			name:    "returns error when note_id is zero",
			input:   DeleteInput{FullPath: testFullPath, IID: 1},
			handler: http.NotFoundHandler(),
			wantErr: true,
		},
		{
			name:  "returns error on GraphQL mutation errors",
			input: DeleteInput{FullPath: testFullPath, IID: 1, NoteID: 100},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"destroyNote": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, `{"destroyNote": {"errors": ["Permission denied"]}}`)
				},
			}),
			wantErr: true,
		},
		{
			name:     "returns error on cancelled context",
			input:    DeleteInput{FullPath: testFullPath, IID: 1, NoteID: 100},
			handler:  http.NotFoundHandler(),
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
				Pagination: toolutil.GraphQLPaginationOutput{HasNextPage: false},
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
				Pagination: toolutil.GraphQLPaginationOutput{},
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
