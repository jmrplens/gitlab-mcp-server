// epicdiscussions_test.go contains unit tests for the epic discussion MCP tool handlers.
// Tests use httptest to mock GitLab GraphQL API responses and verify success, error,
// and edge-case paths.

package epicdiscussions

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const testFullPath = "my-group"

// GraphQL response fixtures.
const gqlDiscussionsData = `{
  "namespace": {
    "workItem": {
      "id": "gid://gitlab/WorkItem/1",
      "widgets": [{
        "discussions": {
          "pageInfo": {"hasNextPage": false, "hasPreviousPage": false, "endCursor": "abc", "startCursor": "xyz"},
          "nodes": [{
            "id": "gid://gitlab/Discussion/d1hex",
            "notes": {
              "nodes": [
                {"id": "gid://gitlab/Note/100", "body": "first note", "author": {"username": "alice"}, "system": false, "createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-01-01T00:00:00Z"},
                {"id": "gid://gitlab/Note/101", "body": "reply note", "author": {"username": "bob"}, "system": false, "createdAt": "2026-01-02T00:00:00Z", "updatedAt": null}
              ]
            }
          }]
        }
      }]
    }
  }
}`

const gqlDiscussionsEmpty = `{
  "namespace": {
    "workItem": {
      "id": "gid://gitlab/WorkItem/1",
      "widgets": [{
        "discussions": {
          "pageInfo": {"hasNextPage": false, "hasPreviousPage": false, "endCursor": "", "startCursor": ""},
          "nodes": []
        }
      }]
    }
  }
}`

const gqlNamespaceNull = `{"namespace": null}`

const gqlCreateNoteData = `{
  "createNote": {
    "note": {
      "id": "gid://gitlab/Note/200",
      "body": "new thread",
      "author": {"username": "carol"},
      "system": false,
      "createdAt": "2026-01-03T00:00:00Z",
      "updatedAt": null,
      "discussion": {"id": "gid://gitlab/Discussion/d2hex"}
    },
    "errors": []
  }
}`

const gqlCreateNoteReplyData = `{
  "createNote": {
    "note": {
      "id": "gid://gitlab/Note/201",
      "body": "reply body",
      "author": {"username": "dave"},
      "system": false,
      "createdAt": "2026-01-04T00:00:00Z",
      "updatedAt": null
    },
    "errors": []
  }
}`

const gqlUpdateNoteData = `{
  "updateNote": {
    "note": {
      "id": "gid://gitlab/Note/100",
      "body": "updated body",
      "author": {"username": "alice"},
      "system": false,
      "createdAt": "2026-01-01T00:00:00Z",
      "updatedAt": "2026-01-05T00:00:00Z"
    },
    "errors": []
  }
}`

const gqlDestroyNoteData = `{
  "destroyNote": {
    "note": {"id": "gid://gitlab/Note/100"},
    "errors": []
  }
}`

const gqlWorkItemGIDData = `{
  "namespace": {
    "workItem": {"id": "gid://gitlab/WorkItem/1"}
  }
}`

// graphqlMux creates an http.Handler that routes GraphQL requests by query content.
func graphqlMux(handlers map[string]http.HandlerFunc) http.Handler {
	return testutil.GraphQLHandler(handlers)
}

// --------------------------------------------------------------------------
// List
// --------------------------------------------------------------------------

// TestList uses table-driven subtests to exercise List across success, pagination, empty result, missing-parent, validation, and API-error scenarios.
func TestList(t *testing.T) {
	tests := []struct {
		name    string
		input   ListInput
		handler http.Handler
		wantErr string
		check   func(t *testing.T, out ListOutput)
	}{
		{
			name:  "returns discussions with correct fields",
			input: ListInput{FullPath: testFullPath, IID: 5},
			handler: graphqlMux(map[string]http.HandlerFunc{"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondGraphQL(w, http.StatusOK, gqlDiscussionsData)
			}}),
			check: func(t *testing.T, out ListOutput) {
				t.Helper()
				if len(out.Discussions) != 1 {
					t.Fatalf("got %d discussions, want 1", len(out.Discussions))
				}
				d := out.Discussions[0]
				if d.ID != "d1hex" {
					t.Errorf("got ID=%q, want d1hex", d.ID)
				}
				if len(d.Notes) != 2 {
					t.Fatalf("got %d notes, want 2", len(d.Notes))
				}
				if d.Notes[0].ID != 100 {
					t.Errorf("note[0] ID=%d, want 100", d.Notes[0].ID)
				}
				if d.Notes[0].Author != "alice" {
					t.Errorf("note[0] Author=%q, want alice", d.Notes[0].Author)
				}
			},
		},
		{
			name:  "returns empty list when no discussions exist",
			input: ListInput{FullPath: testFullPath, IID: 5},
			handler: graphqlMux(map[string]http.HandlerFunc{"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondGraphQL(w, http.StatusOK, gqlDiscussionsEmpty)
			}}),
			check: func(t *testing.T, out ListOutput) {
				t.Helper()
				if len(out.Discussions) != 0 {
					t.Fatalf("got %d discussions, want 0", len(out.Discussions))
				}
			},
		},
		{
			name:  "returns error when epic not found",
			input: ListInput{FullPath: testFullPath, IID: 999},
			handler: graphqlMux(map[string]http.HandlerFunc{"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondGraphQL(w, http.StatusOK, gqlNamespaceNull)
			}}),
			wantErr: "epic not found",
		},
		{
			name:    "returns error when full_path is empty",
			input:   ListInput{IID: 5},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "full_path is required",
		},
		{
			name:    "returns error when iid is zero",
			input:   ListInput{FullPath: testFullPath, IID: 0},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "iid",
		},
		{
			name:    "returns error when iid is negative",
			input:   ListInput{FullPath: testFullPath, IID: -1},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "iid",
		},
		{
			name:  "returns error on API server error",
			input: ListInput{FullPath: testFullPath, IID: 5},
			handler: graphqlMux(map[string]http.HandlerFunc{"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "server error", http.StatusForbidden)
			}}),
			wantErr: "epicDiscussionList",
		},
		{
			name:    "returns error on cancelled context",
			input:   ListInput{FullPath: testFullPath, IID: 5},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "context canceled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			ctx := t.Context()
			if tt.name == "returns error on cancelled context" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			out, err := List(ctx, client, tt.input)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, out)
			}
		})
	}
}

// --------------------------------------------------------------------------
// Get
// --------------------------------------------------------------------------

// TestGet uses table-driven subtests to exercise Get across success, not-found, validation, and API-error scenarios.
func TestGet(t *testing.T) {
	tests := []struct {
		name    string
		input   GetInput
		handler http.Handler
		wantErr string
		check   func(t *testing.T, out Output)
	}{
		{
			name:  "returns discussion with all notes",
			input: GetInput{FullPath: testFullPath, IID: 5, DiscussionID: "d1hex"},
			handler: graphqlMux(map[string]http.HandlerFunc{"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondGraphQL(w, http.StatusOK, gqlDiscussionsData)
			}}),
			check: func(t *testing.T, out Output) {
				t.Helper()
				if out.ID != "d1hex" {
					t.Errorf("got ID=%q, want d1hex", out.ID)
				}
				if len(out.Notes) != 2 {
					t.Fatalf("got %d notes, want 2", len(out.Notes))
				}
			},
		},
		{
			name:  "returns error when discussion not found",
			input: GetInput{FullPath: testFullPath, IID: 5, DiscussionID: "nonexistent"},
			handler: graphqlMux(map[string]http.HandlerFunc{"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondGraphQL(w, http.StatusOK, gqlDiscussionsData)
			}}),
			wantErr: "discussion",
		},
		{
			name:    "returns error when full_path is empty",
			input:   GetInput{IID: 5, DiscussionID: "d1hex"},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "full_path is required",
		},
		{
			name:    "returns error when iid is zero",
			input:   GetInput{FullPath: testFullPath, IID: 0, DiscussionID: "d1hex"},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "iid",
		},
		{
			name:    "returns error when discussion_id is empty",
			input:   GetInput{FullPath: testFullPath, IID: 5},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "discussion_id is required",
		},
		{
			name:  "returns error when epic not found",
			input: GetInput{FullPath: testFullPath, IID: 999, DiscussionID: "d1hex"},
			handler: graphqlMux(map[string]http.HandlerFunc{"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondGraphQL(w, http.StatusOK, gqlNamespaceNull)
			}}),
			wantErr: "epic not found",
		},
		{
			name:    "returns error on cancelled context",
			input:   GetInput{FullPath: testFullPath, IID: 5, DiscussionID: "d1hex"},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "context canceled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			ctx := t.Context()
			if tt.name == "returns error on cancelled context" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			out, err := Get(ctx, client, tt.input)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, out)
			}
		})
	}
}

// --------------------------------------------------------------------------
// Create
// --------------------------------------------------------------------------

// TestCreate uses table-driven subtests to exercise Create across success, validation, mutation-error, and cancellation scenarios.
func TestCreate(t *testing.T) {
	tests := []struct {
		name    string
		input   CreateInput
		handler http.Handler
		wantErr string
		check   func(t *testing.T, out Output)
	}{
		{
			name:  "creates discussion and returns output",
			input: CreateInput{FullPath: testFullPath, IID: 5, Body: "new thread"},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"workItem(iid": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlWorkItemGIDData)
				},
				"createNote": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlCreateNoteData)
				},
			}),
			check: func(t *testing.T, out Output) {
				t.Helper()
				if out.ID != "d2hex" {
					t.Errorf("got ID=%q, want d2hex", out.ID)
				}
				if len(out.Notes) != 1 {
					t.Fatalf("got %d notes, want 1", len(out.Notes))
				}
				if out.Notes[0].ID != 200 {
					t.Errorf("note ID=%d, want 200", out.Notes[0].ID)
				}
			},
		},
		{
			name:    "returns error when full_path is empty",
			input:   CreateInput{IID: 5, Body: "test"},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "full_path is required",
		},
		{
			name:    "returns error when iid is zero",
			input:   CreateInput{FullPath: testFullPath, IID: 0, Body: "test"},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "iid",
		},
		{
			name:    "returns error when body is empty",
			input:   CreateInput{FullPath: testFullPath, IID: 5},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "body is required",
		},
		{
			name:  "returns error on GraphQL mutation errors",
			input: CreateInput{FullPath: testFullPath, IID: 5, Body: "test"},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"workItem(iid": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlWorkItemGIDData)
				},
				"createNote": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, `{"createNote":{"note":null,"errors":["permission denied"]}}`)
				},
			}),
			wantErr: "permission denied",
		},
		{
			name:    "returns error on cancelled context",
			input:   CreateInput{FullPath: testFullPath, IID: 5, Body: "test"},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "context canceled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			ctx := t.Context()
			if tt.name == "returns error on cancelled context" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			out, err := Create(ctx, client, tt.input)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, out)
			}
		})
	}
}

// --------------------------------------------------------------------------
// AddNote
// --------------------------------------------------------------------------

// TestAddNote uses table-driven subtests to exercise AddNote across success, validation, mutation errors, and cancellation.
func TestAddNote(t *testing.T) {
	tests := []struct {
		name    string
		input   AddNoteInput
		handler http.Handler
		wantErr string
		check   func(t *testing.T, out NoteOutput)
	}{
		{
			name:  "adds note and returns output",
			input: AddNoteInput{FullPath: testFullPath, IID: 5, DiscussionID: "d1hex", Body: "reply body"},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"workItem(iid": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlWorkItemGIDData)
				},
				"createNote": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlCreateNoteReplyData)
				},
			}),
			check: func(t *testing.T, out NoteOutput) {
				t.Helper()
				if out.ID != 201 {
					t.Errorf("got ID=%d, want 201", out.ID)
				}
				if out.Author != "dave" {
					t.Errorf("got Author=%q, want dave", out.Author)
				}
			},
		},
		{
			name:    "returns error when full_path is empty",
			input:   AddNoteInput{IID: 5, DiscussionID: "d1hex", Body: "test"},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "full_path is required",
		},
		{
			name:    "returns error when iid is zero",
			input:   AddNoteInput{FullPath: testFullPath, IID: 0, DiscussionID: "d1hex", Body: "test"},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "iid",
		},
		{
			name:    "returns error when discussion_id is empty",
			input:   AddNoteInput{FullPath: testFullPath, IID: 5, Body: "test"},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "discussion_id is required",
		},
		{
			name:    "returns error when body is empty",
			input:   AddNoteInput{FullPath: testFullPath, IID: 5, DiscussionID: "d1hex"},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "body is required",
		},
		{
			name:  "returns error on GraphQL mutation errors",
			input: AddNoteInput{FullPath: testFullPath, IID: 5, DiscussionID: "d1hex", Body: "test"},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"workItem(iid": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlWorkItemGIDData)
				},
				"createNote": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, `{"createNote":{"note":null,"errors":["forbidden"]}}`)
				},
			}),
			wantErr: "forbidden",
		},
		{
			name:    "returns error on cancelled context",
			input:   AddNoteInput{FullPath: testFullPath, IID: 5, DiscussionID: "d1hex", Body: "test"},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "context canceled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			ctx := t.Context()
			if tt.name == "returns error on cancelled context" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			out, err := AddNote(ctx, client, tt.input)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, out)
			}
		})
	}
}

// --------------------------------------------------------------------------
// UpdateNote
// --------------------------------------------------------------------------

// TestUpdateNote uses table-driven subtests to exercise UpdateNote across success, validation, mutation errors, and cancellation.
func TestUpdateNote(t *testing.T) {
	tests := []struct {
		name    string
		input   UpdateNoteInput
		handler http.Handler
		wantErr string
		check   func(t *testing.T, out NoteOutput)
	}{
		{
			name:  "updates note and returns output",
			input: UpdateNoteInput{FullPath: testFullPath, IID: 5, NoteID: 100, Body: "updated body"},
			handler: graphqlMux(map[string]http.HandlerFunc{"updateNote": func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondGraphQL(w, http.StatusOK, gqlUpdateNoteData)
			}}),
			check: func(t *testing.T, out NoteOutput) {
				t.Helper()
				if out.Body != "updated body" {
					t.Errorf("got Body=%q, want 'updated body'", out.Body)
				}
				if out.UpdatedAt == "" {
					t.Error("expected UpdatedAt to be set")
				}
			},
		},
		{
			name:    "returns error when full_path is empty",
			input:   UpdateNoteInput{IID: 5, NoteID: 100, Body: "test"},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "full_path is required",
		},
		{
			name:    "returns error when iid is zero",
			input:   UpdateNoteInput{FullPath: testFullPath, IID: 0, NoteID: 100, Body: "test"},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "iid",
		},
		{
			name:    "returns error when note_id is zero",
			input:   UpdateNoteInput{FullPath: testFullPath, IID: 5, NoteID: 0, Body: "test"},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "note_id",
		},
		{
			name:    "returns error when body is empty",
			input:   UpdateNoteInput{FullPath: testFullPath, IID: 5, NoteID: 100},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "body is required",
		},
		{
			name:  "returns error on GraphQL mutation errors",
			input: UpdateNoteInput{FullPath: testFullPath, IID: 5, NoteID: 100, Body: "test"},
			handler: graphqlMux(map[string]http.HandlerFunc{"updateNote": func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondGraphQL(w, http.StatusOK, `{"updateNote":{"note":null,"errors":["not found"]}}`)
			}}),
			wantErr: "not found",
		},
		{
			name:    "returns error on cancelled context",
			input:   UpdateNoteInput{FullPath: testFullPath, IID: 5, NoteID: 100, Body: "test"},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "context canceled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			ctx := t.Context()
			if tt.name == "returns error on cancelled context" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			out, err := UpdateNote(ctx, client, tt.input)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, out)
			}
		})
	}
}

// --------------------------------------------------------------------------
// DeleteNote
// --------------------------------------------------------------------------

// TestDeleteNote uses table-driven subtests to exercise DeleteNote across success, validation, mutation errors, and cancellation.
func TestDeleteNote(t *testing.T) {
	tests := []struct {
		name    string
		input   DeleteNoteInput
		handler http.Handler
		wantErr string
	}{
		{
			name:  "deletes note successfully",
			input: DeleteNoteInput{FullPath: testFullPath, IID: 5, NoteID: 100},
			handler: graphqlMux(map[string]http.HandlerFunc{"destroyNote": func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondGraphQL(w, http.StatusOK, gqlDestroyNoteData)
			}}),
		},
		{
			name:    "returns error when full_path is empty",
			input:   DeleteNoteInput{IID: 5, NoteID: 100},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "full_path is required",
		},
		{
			name:    "returns error when iid is zero",
			input:   DeleteNoteInput{FullPath: testFullPath, IID: 0, NoteID: 100},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "iid",
		},
		{
			name:    "returns error when note_id is zero",
			input:   DeleteNoteInput{FullPath: testFullPath, IID: 5, NoteID: 0},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "note_id",
		},
		{
			name:  "returns error on GraphQL mutation errors",
			input: DeleteNoteInput{FullPath: testFullPath, IID: 5, NoteID: 100},
			handler: graphqlMux(map[string]http.HandlerFunc{"destroyNote": func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondGraphQL(w, http.StatusOK, `{"destroyNote":{"errors":["forbidden"]}}`)
			}}),
			wantErr: "forbidden",
		},
		{
			name:    "returns error on cancelled context",
			input:   DeleteNoteInput{FullPath: testFullPath, IID: 5, NoteID: 100},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "context canceled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			ctx := t.Context()
			if tt.name == "returns error on cancelled context" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			err := DeleteNote(ctx, client, tt.input)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// --------------------------------------------------------------------------
// Formatters
// --------------------------------------------------------------------------

// TestFormatListMarkdown uses table-driven subtests to verify that FormatListMarkdown renders a table for populated inputs and an empty-state message otherwise.
func TestFormatListMarkdown(t *testing.T) {
	tests := []struct {
		name    string
		input   ListOutput
		wantSub string
	}{
		{
			name: "renders table with discussions",
			input: ListOutput{
				Discussions: []Output{
					{
						ID: "d1hex",
						Notes: []NoteOutput{
							{ID: 100, Body: "Hello", Author: "alice", CreatedAt: "2026-01-01T00:00:00Z"},
						},
					},
				},
				Pagination: toolutil.GraphQLPaginationOutput{},
			},
			wantSub: "d1hex",
		},
		{
			name:    "renders empty state when no discussions",
			input:   ListOutput{},
			wantSub: "No epic discussions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatListMarkdown(tt.input)
			if result == nil {
				t.Fatal("expected non-nil result")
			}
			text := result.Content[0].(*mcp.TextContent).Text
			if !strings.Contains(text, tt.wantSub) {
				t.Errorf("output %q does not contain %q", text, tt.wantSub)
			}
		})
	}
}

// TestFormatMarkdown uses table-driven subtests to verify that FormatMarkdown renders a discussion with notes and handles an empty discussion.
func TestFormatMarkdown(t *testing.T) {
	tests := []struct {
		name    string
		input   Output
		wantSub string
	}{
		{
			name:    "renders discussion with notes",
			input:   Output{ID: "d1hex", Notes: []NoteOutput{{ID: 1, Body: "note body", Author: "bob", CreatedAt: "2026-01-01T00:00:00Z"}}},
			wantSub: "bob",
		},
		{
			name:    "renders empty discussion",
			input:   Output{ID: "d1hex"},
			wantSub: "d1hex",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatMarkdown(tt.input)
			if result == nil {
				t.Fatal("expected non-nil result")
			}
			text := result.Content[0].(*mcp.TextContent).Text
			if !strings.Contains(text, tt.wantSub) {
				t.Errorf("output %q does not contain %q", text, tt.wantSub)
			}
		})
	}
}

// TestFormatNoteMarkdown verifies that FormatNoteMarkdown renders a note with its author in the output.
func TestFormatNoteMarkdown(t *testing.T) {
	result := FormatNoteMarkdown(NoteOutput{ID: 1, Body: "test note", Author: "carol", CreatedAt: "2026-01-01T00:00:00Z"})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "carol") {
		t.Errorf("expected author 'carol' in output, got %q", text)
	}
}

// --------------------------------------------------------------------------
// RegisterTools + RegisterMeta — no panic
// --------------------------------------------------------------------------

// TestRegisterTools_NoPanic verifies that RegisterTools registers all tools on the MCP server without panicking.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// TestRegisterMeta_NoPanic verifies that RegisterMeta registers the meta-tool on the MCP server without panicking.
func TestRegisterMeta_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)
}

// --------------------------------------------------------------------------
// MCP round-trip — all tools
// --------------------------------------------------------------------------

// TestMCPRoundTrip_AllTools uses table-driven subtests to invoke every individual tool via an in-memory MCP session and verify that none returns an error result.
func TestMCPRoundTrip_AllTools(t *testing.T) {
	session := newEpicDiscussionsMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_list_epic_discussions", map[string]any{"full_path": testFullPath, "epic_iid": float64(5)}},
		{"get", "gitlab_get_epic_discussion", map[string]any{"full_path": testFullPath, "epic_iid": float64(5), "discussion_id": "d1hex"}},
		{"create", "gitlab_create_epic_discussion", map[string]any{"full_path": testFullPath, "epic_iid": float64(5), "body": "new thread"}},
		{"add_note", "gitlab_add_epic_discussion_note", map[string]any{"full_path": testFullPath, "epic_iid": float64(5), "discussion_id": "d1hex", "body": "reply"}},
		{"update_note", "gitlab_update_epic_discussion_note", map[string]any{"full_path": testFullPath, "epic_iid": float64(5), "note_id": float64(100), "body": "updated"}},
		{"delete_note", "gitlab_delete_epic_discussion_note", map[string]any{"full_path": testFullPath, "epic_iid": float64(5), "note_id": float64(100)}},
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

// --------------------------------------------------------------------------
// MCP round-trip — meta tool
// --------------------------------------------------------------------------

// TestMCPRoundTrip_MetaTool uses table-driven subtests to invoke each meta-tool action via an in-memory MCP session and verify successful dispatch.
func TestMCPRoundTrip_MetaTool(t *testing.T) {
	session := newEpicDiscussionsMetaMCPSession(t)
	ctx := context.Background()

	actions := []struct {
		name   string
		action string
		params map[string]any
	}{
		{"list", "list", map[string]any{"full_path": testFullPath, "epic_iid": float64(5)}},
		{"get", "get", map[string]any{"full_path": testFullPath, "epic_iid": float64(5), "discussion_id": "d1hex"}},
		{"create", "create", map[string]any{"full_path": testFullPath, "epic_iid": float64(5), "body": "new thread"}},
		{"add_note", "add_note", map[string]any{"full_path": testFullPath, "epic_iid": float64(5), "discussion_id": "d1hex", "body": "reply"}},
		{"update_note", "update_note", map[string]any{"full_path": testFullPath, "epic_iid": float64(5), "note_id": float64(100), "body": "updated"}},
		{"delete_note", "delete_note", map[string]any{"full_path": testFullPath, "epic_iid": float64(5), "note_id": float64(100)}},
	}

	for _, tt := range actions {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name: "gitlab_epic_discussion",
				Arguments: map[string]any{
					"action": tt.action,
					"params": tt.params,
				},
			})
			if err != nil {
				t.Fatalf("CallTool(gitlab_epic_discussion/%s) error: %v", tt.action, err)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(gitlab_epic_discussion/%s) returned error: %s", tt.action, tc.Text)
					}
				}
				t.Fatalf("CallTool(gitlab_epic_discussion/%s) returned IsError=true", tt.action)
			}
		})
	}
}

// --------------------------------------------------------------------------
// Helpers: MCP session factories
// --------------------------------------------------------------------------

// graphqlSessionMux creates a GraphQL handler for MCP round-trip tests.
func graphqlSessionMux() http.Handler {
	return graphqlMux(map[string]http.HandlerFunc{
		"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, gqlDiscussionsData)
		},
		"workItem(iid": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, gqlWorkItemGIDData)
		},
		"createNote": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, gqlCreateNoteData)
		},
		"updateNote": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, gqlUpdateNoteData)
		},
		"destroyNote": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, gqlDestroyNoteData)
		},
	})
}

func newEpicDiscussionsMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	client := testutil.NewTestClient(t, graphqlSessionMux())
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

func newEpicDiscussionsMetaMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	client := testutil.NewTestClient(t, graphqlSessionMux())
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)

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
