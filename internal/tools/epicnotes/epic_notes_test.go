package epicnotes

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
									{"id": "gid://gitlab/Note/100", "body": "This looks good", "author": {"id": "gid://gitlab/User/5", "name": "Alice Example", "username": "alice", "webUrl": "https://gitlab.example.com/alice", "avatarUrl": "https://gitlab.example.com/avatar/alice.png"}, "system": false, "createdAt": "2026-01-15T10:00:00Z", "updatedAt": "2026-01-15T10:00:00Z"},
									{"id": "gid://gitlab/Note/101", "body": "changed the description", "author": {"id": "gid://gitlab/User/1", "name": "Administrator", "username": "admin"}, "system": true, "createdAt": "2026-01-15T12:00:00Z", "updatedAt": "2026-01-15T12:00:00Z"}
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

	// GraphQL response whose work item has a widget without discussions.
	gqlNotesNoDiscussionWidget = `{
		"namespace": {
			"workItem": {
				"id": "gid://gitlab/WorkItem/1",
				"widgets": [{}]
			}
		}
	}`

	// GraphQL response for namespace not found.
	gqlNamespaceNull = `{"namespace": null}`

	// GraphQL response for createNote mutation.
	gqlCreateNoteData = `{
		"createNote": {
			"note": {"id": "gid://gitlab/Note/200", "body": "New comment", "author": {"id": "gid://gitlab/User/5", "name": "Alice Example", "username": "alice"}, "system": false, "createdAt": "2026-01-16T10:00:00Z", "updatedAt": "2026-01-16T10:00:00Z"},
			"errors": []
		}
	}`

	// GraphQL response for updateNote mutation.
	gqlUpdateNoteData = `{
		"updateNote": {
			"note": {"id": "gid://gitlab/Note/100", "body": "Updated comment", "author": {"id": "gid://gitlab/User/5", "name": "Alice Example", "username": "alice"}, "system": false, "createdAt": "2026-01-15T10:00:00Z", "updatedAt": "2026-01-16T11:00:00Z"},
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

// TestResolveWorkItemGID_ErrorPaths verifies that ResolveWorkItemGIDPaths returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestResolveWorkItemGID_ErrorPaths(t *testing.T) {
	tests := []struct {
		name    string
		handler http.Handler
		wantErr string
	}{
		{
			name: "graphql error",
			handler: graphqlMux(map[string]http.HandlerFunc{"workItem(iid": func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "forbidden", http.StatusForbidden)
			}}),
			wantErr: "forbidden",
		},
		{
			name: "missing epic",
			handler: graphqlMux(map[string]http.HandlerFunc{"workItem(iid": func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondGraphQL(w, http.StatusOK, gqlNamespaceNull)
			}}),
			wantErr: "epic not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			_, err := resolveWorkItemGID(t.Context(), client, testFullPath, 1)
			if err == nil {
				t.Fatalf("expected error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestList verifies the List handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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
				assertEpicNotesList(t, out)
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
			name:  "skips widgets without discussions",
			input: ListInput{FullPath: testFullPath, IID: 1},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlNotesNoDiscussionWidget)
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
					http.Error(w, "internal error", http.StatusForbidden)
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

func assertEpicNotesList(t *testing.T, out ListOutput) {
	t.Helper()
	if len(out.Notes) != 2 {
		t.Fatalf("len(Notes) = %d, want 2", len(out.Notes))
	}
	assertEpicNote(t, out.Notes[0], 100, "This looks good", "alice", false, 0)
	assertEpicNote(t, out.Notes[1], 101, "", "", true, 1)
}

// authorObj builds a canonical author object carrying just a username, for use
// in Markdown formatter test fixtures.
func authorObj(username string) *NoteUserOutput {
	return &NoteUserOutput{Username: username}
}

// noteAuthorUsernameOrEmpty returns the canonical author username, or "" when the
// author object is nil. It lets assertions compare against the migrated
// *NoteUserOutput author object.
func noteAuthorUsernameOrEmpty(got Output) string {
	if got.Author != nil {
		return got.Author.Username
	}
	return ""
}

func assertEpicNote(t *testing.T, got Output, wantID int64, wantBody, wantAuthor string, wantSystem bool, index int) {
	t.Helper()
	if got.ID != wantID {
		t.Errorf("Notes[%d].ID = %d, want %d", index, got.ID, wantID)
	}
	if wantBody != "" && got.Body != wantBody {
		t.Errorf("Notes[%d].Body = %q, want %q", index, got.Body, wantBody)
	}
	if wantAuthor != "" && noteAuthorUsernameOrEmpty(got) != wantAuthor {
		t.Errorf("Notes[%d].Author.Username = %q, want %q", index, noteAuthorUsernameOrEmpty(got), wantAuthor)
	}
	if got.System != wantSystem {
		t.Errorf("Notes[%d].System = %v, want %v", index, got.System, wantSystem)
	}
}

// TestGet verifies the Get handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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
				if got := noteAuthorUsernameOrEmpty(out); got != "alice" {
					t.Errorf("Author.Username = %q, want %q", got, "alice")
				}
				if out.Author == nil || out.Author.ID == 0 {
					t.Errorf("Author object should carry a non-zero ID, got %+v", out.Author)
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
			name:  "returns error when widgets have no discussions",
			input: GetInput{FullPath: testFullPath, IID: 1, NoteID: 100},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlNotesNoDiscussionWidget)
				},
			}),
			wantErr: true,
		},
		{
			name:  "returns error on API server error",
			input: GetInput{FullPath: testFullPath, IID: 1, NoteID: 100},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
					http.Error(w, "forbidden", http.StatusForbidden)
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

// TestCreate verifies the Create handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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
			name:  "returns error when resolving epic fails",
			input: CreateInput{FullPath: testFullPath, IID: 1, Body: "note"},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"workItem(iid": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlNamespaceNull)
				},
			}),
			wantErr: true,
		},
		{
			name:  "returns error on createNote API error",
			input: CreateInput{FullPath: testFullPath, IID: 1, Body: "note"},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"workItem(iid": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlWorkItemGIDData)
				},
				"createNote": func(w http.ResponseWriter, _ *http.Request) {
					http.Error(w, "forbidden", http.StatusForbidden)
				},
			}),
			wantErr: true,
		},
		{
			name:  "returns error when createNote returns no note",
			input: CreateInput{FullPath: testFullPath, IID: 1, Body: "note"},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"workItem(iid": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlWorkItemGIDData)
				},
				"createNote": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, `{"createNote":{"note":null,"errors":[]}}`)
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

// TestUpdate verifies the Update handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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
			name:  "returns error on updateNote API error",
			input: UpdateInput{FullPath: testFullPath, IID: 1, NoteID: 100, Body: "x"},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"updateNote": func(w http.ResponseWriter, _ *http.Request) {
					http.Error(w, "forbidden", http.StatusForbidden)
				},
			}),
			wantErr: true,
		},
		{
			name:  "returns error when updateNote returns no note",
			input: UpdateInput{FullPath: testFullPath, IID: 1, NoteID: 100, Body: "x"},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"updateNote": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, `{"updateNote":{"note":null,"errors":[]}}`)
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

// TestDelete verifies the Delete handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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
			name:  "returns error on destroyNote API error",
			input: DeleteInput{FullPath: testFullPath, IID: 1, NoteID: 100},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"destroyNote": func(w http.ResponseWriter, _ *http.Request) {
					http.Error(w, "forbidden", http.StatusForbidden)
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

// TestFormatOutputMarkdown verifies the OutputMarkdown Markdown formatter for a representative output input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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
				Author:    authorObj("alice"),
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
				Author:    authorObj("admin"),
				CreatedAt: "2026-01-15T12:00:00Z",
				System:    true,
			},
			contains: []string{
				"## Epic Note #101",
				"**System note**",
				"changed the description",
			},
		},
		{
			name: "renders note with nil author object",
			input: Output{
				ID:        102,
				Body:      "anonymous system entry",
				Author:    nil,
				CreatedAt: "2026-01-15T13:00:00Z",
				System:    true,
			},
			contains: []string{
				"## Epic Note #102",
				"anonymous system entry",
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

// TestFormatListMarkdown verifies the ListMarkdown Markdown formatter for a representative list input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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
					{ID: 100, Author: authorObj("alice"), CreatedAt: "2026-01-15T10:00:00Z", System: false},
					{ID: 101, Author: authorObj("admin"), CreatedAt: "2026-01-15T12:00:00Z", System: true},
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
