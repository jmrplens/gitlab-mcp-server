package epicnotes

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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

	// GraphQL response for a group that exists and holds no epic with that IID,
	// which is the other half of the not-found check: GitLab answers the
	// namespace and leaves the work item null.
	gqlWorkItemNull = `{"namespace": {"workItem": null}}`

	// GraphQL response carrying the values the converters have a fallback for:
	// a note whose id is not a global ID and which carries no timestamps, whose
	// author's id does not parse either, beside a note whose author GitLab sent
	// with no id at all.
	gqlNotesSparseData = `{
		"namespace": {
			"workItem": {
				"id": "gid://gitlab/WorkItem/1",
				"widgets": [{
					"discussions": {
						"pageInfo": {"hasNextPage": false, "endCursor": null},
						"nodes": [{
							"notes": {
								"nodes": [
									{"id": "not-a-global-id", "body": "unparseable identifiers", "author": {"id": "also-not-a-global-id", "name": "Nobody", "username": "nobody"}, "system": false, "createdAt": null, "updatedAt": null},
									{"id": "gid://gitlab/Note/103", "body": "author without an id", "author": {"id": "", "name": "Ghost", "username": "ghost"}, "system": false, "createdAt": "2026-01-18T09:00:00Z", "updatedAt": "2026-01-18T09:30:00Z"}
								]
							}
						}]
					}
				}]
			}
		}
	}`

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

// assertGraphQLVariables holds the variables of the request the mock received to
// the whole map the handler should have sent, label naming the call that sent
// it.
//
// It is the one place this package reads a request rather than a response, and
// every handler here needs it for the same reason: each builds its variables
// from two inputs of one type, so a pair written into each other's key is a
// request GitLab would act on for another object while the mock, which answers
// from a fixture, replies exactly as it did before. The pinned schema the test
// transport validates against cannot see it either, since it judges the
// document and the names of its variables and never the values under them.
//
// The whole map is compared rather than one key at a time, so a variable
// dropped or invented fails here as well as one carrying the wrong value.
//
// It runs on the httptest server's goroutine, so it reports with t.Errorf and
// leaves the caller to answer the request; a t.Fatal here would abort that
// goroutine rather than the test.
func assertGraphQLVariables(t *testing.T, r *http.Request, label string, want map[string]any) {
	t.Helper()
	var body struct {
		Variables map[string]any `json:"variables"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Errorf("%s: decode body: %v", label, err)
		return
	}
	if !reflect.DeepEqual(body.Variables, want) {
		t.Errorf("%s variables = %v, want %v", label, body.Variables, want)
	}
}

// TestResolveWorkItemGID_ErrorPaths verifies that ResolveWorkItemGIDPaths returns a wrapped error when the GitLab API responds with an error status.
// Epics are work items, so the lookup is a GraphQL POST rather than a REST GET.
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

// TestListWith_UndeclaredPaginationVariable verifies that the list handler
// refuses to run an operation that declares fewer variables than its input can
// send, instead of sending one GitLab would discard.
//
// This is the defect the shared helper's signature exists to prevent: a
// variable an operation does not declare is ignored rather than rejected, so
// the caller would be answered with a page it did not ask for and no error.
// The discussions connection this query pages is forward-only upstream, so the
// document dropped here is the forward cursor rather than the backward pair.
//
// The shortened document is passed in rather than assigned over the package
// constant, so nothing a parallel neighbor reads changes underneath it.
func TestListWith_UndeclaredPaginationVariable(t *testing.T) {
	document := strings.Replace(queryListWorkItemNotes, ", $after: String", "", 1)

	handler := graphqlMux(map[string]http.HandlerFunc{"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
		t.Error("listWith() ran an operation that cannot receive its own cursor")
		testutil.RespondGraphQL(w, http.StatusOK, gqlNotesData)
	}})

	_, err := listWith(context.Background(), testutil.NewTestClient(t, handler), document,
		ListInput{FullPath: testFullPath, IID: 5})
	if err == nil {
		t.Fatal("listWith() error = nil, want a refusal naming the missing declaration")
	}
	if !strings.Contains(err.Error(), "$after") {
		t.Errorf("listWith() error = %v, want it to name $after", err)
	}
}

// TestList_SendsTheVariablesTheCallerNamed verifies that the group path, the
// epic IID and the cursor pair reach GitLab as the document declares them: the
// IID as a String, and the page size and cursor the pagination input resolved
// to.
//
// Nothing else in the layered defenses can see this. A variable map has no
// branch, so neither the mutation nor the condition gate notices a path written
// into iid or a cursor left behind, and the validating transport judges the
// document and the names of its variables rather than the values carried under
// them.
func TestList_SendsTheVariablesTheCallerNamed(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{"WorkItemWidgetNotes": func(w http.ResponseWriter, r *http.Request) {
		assertGraphQLVariables(t, r, "List()", map[string]any{
			"fullPath": testFullPath,
			"iid":      "5",
			"first":    float64(7),
			"after":    "cursor-1",
		})
		testutil.RespondGraphQL(w, http.StatusOK, gqlNotesData)
	}})

	_, err := List(context.Background(), testutil.NewTestClient(t, handler), ListInput{
		FullPath: testFullPath,
		IID:      5,
		First:    new(7),
		After:    "cursor-1",
	})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
}

// TestList_GraphQLErrorsAreReported verifies that a document GitLab refused is
// answered with its errors rather than as a missing epic.
//
// GitLab returns HTTP 200 with a top-level errors array and a null namespace,
// which client-go leaves for the caller to notice, so the nil check would
// otherwise blame the group path and IID for a fault in the query.
func TestList_GraphQLErrorsAreReported(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondGraphQLError(w, http.StatusOK, "Field 'discussions' doesn't accept argument 'before'")
	}})

	out, err := List(context.Background(), testutil.NewTestClient(t, handler), ListInput{FullPath: testFullPath, IID: 5})
	if err == nil {
		t.Fatalf("List() = %+v, want the GraphQL errors reported", out)
	}
	if !strings.Contains(err.Error(), "doesn't accept argument") {
		t.Errorf("List() error = %v, want it to carry the GitLab message", err)
	}
	if strings.Contains(err.Error(), "not found") {
		t.Errorf("List() error = %v, want it not to blame the epic", err)
	}
}

// TestGet_GraphQLErrorsAreReported verifies that Get reports a refused document
// the same way List does.
//
// Get runs the same document through the same envelope, so a query the
// instance refused arrives at the same nil check and used to be reported as a
// missing epic, sending a caller to verify a path that was never the problem.
func TestGet_GraphQLErrorsAreReported(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondGraphQLError(w, http.StatusOK, "Field 'discussions' doesn't accept argument 'before'")
	}})

	out, err := Get(context.Background(), testutil.NewTestClient(t, handler),
		GetInput{FullPath: testFullPath, IID: 5, NoteID: 7})
	if err == nil {
		t.Fatalf("Get() = %+v, want the GraphQL errors reported", out)
	}
	if !strings.Contains(err.Error(), "doesn't accept argument") {
		t.Errorf("Get() error = %v, want it to carry the GitLab message", err)
	}
	if strings.Contains(err.Error(), "not found") {
		t.Errorf("Get() error = %v, want it not to blame the epic", err)
	}
}

// TestList_ForwardOnlyPaginationDropsPreviousPage verifies that a previous page
// reported by GitLab reaches neither the output nor the Markdown summary.
//
// The work item notes widget refuses before and last while paginating by
// keyset, so from its second page on it reports hasPreviousPage and a real
// startCursor. This tool has no parameter that could spend that cursor, so
// naming it would offer a model a page it can never ask for.
func TestList_ForwardOnlyPaginationDropsPreviousPage(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondGraphQL(w, http.StatusOK, strings.Replace(
			gqlNotesData,
			`"pageInfo": {"hasNextPage": false, "hasPreviousPage": false, "endCursor": null, "startCursor": null}`,
			`"pageInfo": {"hasNextPage": true, "hasPreviousPage": true, "endCursor": "next", "startCursor": "prev"}`,
			1,
		))
	}})

	out, err := List(context.Background(), testutil.NewTestClient(t, handler), ListInput{FullPath: testFullPath, IID: 5})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if !out.Pagination.HasNextPage || out.Pagination.EndCursor != "next" {
		t.Errorf("Pagination = %+v, want the forward half preserved", out.Pagination)
	}
	md := FormatListMarkdown(out)
	if strings.Contains(md, "prev page cursor") || strings.Contains(md, "prev") {
		t.Errorf("markdown = %q, want no previous page on a forward-only connection", md)
	}
}

// TestList verifies the List handler.
// Every note this package reads comes over GraphQL, so the call under test is
// a POST of the notes widget document.
// It asserts the returned output matches the expected fields.
//
// Every case the handler must refuse before reaching GitLab is served by
// [testutil.ForbiddenHandler] and names the message it wants: with a mock that
// answers anything, a guard loosened from "must be > 0" to "must not be
// negative" still produced an error, because the zero IID reached GitLab and
// GitLab said no.
func TestList(t *testing.T) {
	tests := []struct {
		name            string
		input           ListInput
		handler         http.Handler
		cancelFn        bool
		wantErr         bool
		wantErrContains string
		validate        func(t *testing.T, out ListOutput)
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
			name:  "surfaces a note whose ids and timestamps GitLab left unset",
			input: ListInput{FullPath: testFullPath, IID: 1},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlNotesSparseData)
				},
			}),
			validate: func(t *testing.T, out ListOutput) {
				t.Helper()
				if len(out.Notes) != 2 {
					t.Fatalf("len(Notes) = %d, want 2", len(out.Notes))
				}
				assertEpicNote(t, "Notes[0]", out.Notes[0], Output{
					Body:   "unparseable identifiers",
					Author: &NoteUserOutput{Username: "nobody", Name: "Nobody"},
				})
				assertEpicNote(t, "Notes[1]", out.Notes[1], Output{
					ID:        103,
					Body:      "author without an id",
					Author:    &NoteUserOutput{Username: "ghost", Name: "Ghost"},
					CreatedAt: "2026-01-18T09:00:00Z",
					UpdatedAt: "2026-01-18T09:30:00Z",
				})
			},
		},
		{
			name:  "returns error when the namespace does not exist",
			input: ListInput{FullPath: testFullPath, IID: 999},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlNamespaceNull)
				},
			}),
			wantErr:         true,
			wantErrContains: `epic not found in group "my-group" with IID 999`,
		},
		{
			name:  "returns error when the group holds no epic with that iid",
			input: ListInput{FullPath: testFullPath, IID: 999},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlWorkItemNull)
				},
			}),
			wantErr:         true,
			wantErrContains: `epic not found in group "my-group" with IID 999`,
		},
		{
			name:            "returns error when full_path is empty",
			input:           ListInput{IID: 1},
			handler:         testutil.ForbiddenHandler(t),
			wantErr:         true,
			wantErrContains: "full_path is required",
		},
		{
			name:            "returns error when iid is zero",
			input:           ListInput{FullPath: testFullPath},
			handler:         testutil.ForbiddenHandler(t),
			wantErr:         true,
			wantErrContains: "epic_iid is required",
		},
		{
			name:            "returns error when iid is negative",
			input:           ListInput{FullPath: testFullPath, IID: -1},
			handler:         testutil.ForbiddenHandler(t),
			wantErr:         true,
			wantErrContains: "epic_iid is required",
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
			name:            "returns error on cancelled context",
			input:           ListInput{FullPath: testFullPath, IID: 1},
			handler:         testutil.ForbiddenHandler(t),
			cancelFn:        true,
			wantErr:         true,
			wantErrContains: "context canceled",
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
			if err != nil && tt.wantErrContains != "" && !strings.Contains(err.Error(), tt.wantErrContains) {
				t.Errorf("List() error = %v, want it to contain %q", err, tt.wantErrContains)
			}
			if tt.validate != nil {
				tt.validate(t, out)
			}
		})
	}
}

// assertEpicNotesList pins every value the two-note fixture carries, and pins
// it whole.
//
// A converter that reads a neighboring key has no branch for either gate to
// flip, so the only thing that can catch it is an expectation naming every
// field: rotating the author's name, web URL and avatar URL through each other
// left the whole suite green while the assertions here were a username and an
// ID. The author's four values differ from each other for that reason. The two
// timestamps in this fixture do not, since GitLab writes both at once on a note
// nobody has edited; the pair that a swapped assignment shows up in is the
// update mutation's, whose created and updated moments are a day apart.
func assertEpicNotesList(t *testing.T, out ListOutput) {
	t.Helper()
	if len(out.Notes) != 2 {
		t.Fatalf("len(Notes) = %d, want 2", len(out.Notes))
	}
	assertEpicNote(t, "Notes[0]", out.Notes[0], Output{
		ID:   100,
		Body: "This looks good",
		Author: &NoteUserOutput{
			ID:        5,
			Username:  "alice",
			Name:      "Alice Example",
			WebURL:    "https://gitlab.example.com/alice",
			AvatarURL: "https://gitlab.example.com/avatar/alice.png",
		},
		CreatedAt: "2026-01-15T10:00:00Z",
		UpdatedAt: "2026-01-15T10:00:00Z",
		System:    false,
	})
	assertEpicNote(t, "Notes[1]", out.Notes[1], Output{
		ID:        101,
		Body:      "changed the description",
		Author:    &NoteUserOutput{ID: 1, Username: "admin", Name: "Administrator"},
		CreatedAt: "2026-01-15T12:00:00Z",
		UpdatedAt: "2026-01-15T12:00:00Z",
		System:    true,
	})
}

// authorObj builds a canonical author object carrying just a username, for use
// in Markdown formatter test fixtures.
func authorObj(username string) *NoteUserOutput {
	return &NoteUserOutput{Username: username}
}

// assertEpicNote compares one converted note with the whole value it should
// hold, label naming which note failed. Every field is asserted, including the
// two timestamps, which are what a swapped pair of assignments shows up in.
func assertEpicNote(t *testing.T, label string, got, want Output) {
	t.Helper()
	if got.ID != want.ID {
		t.Errorf("%s.ID = %d, want %d", label, got.ID, want.ID)
	}
	if got.Body != want.Body {
		t.Errorf("%s.Body = %q, want %q", label, got.Body, want.Body)
	}
	if got.CreatedAt != want.CreatedAt {
		t.Errorf("%s.CreatedAt = %q, want %q", label, got.CreatedAt, want.CreatedAt)
	}
	if got.UpdatedAt != want.UpdatedAt {
		t.Errorf("%s.UpdatedAt = %q, want %q", label, got.UpdatedAt, want.UpdatedAt)
	}
	if got.System != want.System {
		t.Errorf("%s.System = %v, want %v", label, got.System, want.System)
	}
	assertEpicNoteAuthor(t, label, got.Author, want.Author)
}

// assertEpicNoteAuthor compares the canonical author object whole, so a field
// read from the wrong GraphQL key fails here rather than passing on a username
// that happened to be checked alone.
func assertEpicNoteAuthor(t *testing.T, label string, got, want *NoteUserOutput) {
	t.Helper()
	switch {
	case got == nil && want == nil:
	case got == nil:
		t.Errorf("%s.Author = nil, want %+v", label, *want)
	case want == nil:
		t.Errorf("%s.Author = %+v, want nil", label, *got)
	case *got != *want:
		t.Errorf("%s.Author = %+v, want %+v", label, *got, *want)
	}
}

// TestGet_SendsTheVariablesTheCallerNamed verifies that the group path and the
// epic IID reach GitLab under the names the document declares, beside the page
// size Get pins for itself because it reads the whole widget and matches the
// note locally.
//
// Get sends both identifiers as strings, so the two written into each other's
// key is a document the schema still accepts and a request GitLab would answer
// for a group named after a number. Nothing else here can see it: the matching
// loop reads the fixture the mock replies with whatever was asked, so the note
// is found and every assertion downstream of it holds.
func TestGet_SendsTheVariablesTheCallerNamed(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{"WorkItemWidgetNotes": func(w http.ResponseWriter, r *http.Request) {
		assertGraphQLVariables(t, r, "Get()", map[string]any{
			"fullPath": testFullPath,
			"iid":      "5",
			"first":    float64(toolutil.GraphQLMaxFirst),
		})
		testutil.RespondGraphQL(w, http.StatusOK, gqlNotesData)
	}})

	_, err := Get(context.Background(), testutil.NewTestClient(t, handler),
		GetInput{FullPath: testFullPath, IID: 5, NoteID: 100})
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
}

// TestGet verifies the Get handler.
// Get runs the list document and matches by note ID, so the call under test is
// the same GraphQL POST TestList drives.
// It asserts the returned output matches the expected fields.
func TestGet(t *testing.T) {
	tests := []struct {
		name            string
		input           GetInput
		handler         http.Handler
		cancelFn        bool
		wantErr         bool
		wantErrContains string
		validate        func(t *testing.T, out Output)
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
				assertEpicNote(t, "Get()", out, Output{
					ID:   100,
					Body: "This looks good",
					Author: &NoteUserOutput{
						ID:        5,
						Username:  "alice",
						Name:      "Alice Example",
						WebURL:    "https://gitlab.example.com/alice",
						AvatarURL: "https://gitlab.example.com/avatar/alice.png",
					},
					CreatedAt: "2026-01-15T10:00:00Z",
					UpdatedAt: "2026-01-15T10:00:00Z",
				})
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
			wantErr:         true,
			wantErrContains: `note 999 not found on epic &1 in group "my-group"`,
		},
		{
			name:            "returns error when full_path is empty",
			input:           GetInput{IID: 1, NoteID: 100},
			handler:         testutil.ForbiddenHandler(t),
			wantErr:         true,
			wantErrContains: "full_path is required",
		},
		{
			name:            "returns error when iid is zero",
			input:           GetInput{FullPath: testFullPath, NoteID: 100},
			handler:         testutil.ForbiddenHandler(t),
			wantErr:         true,
			wantErrContains: "epic_iid is required",
		},
		{
			name:            "returns error when note_id is zero",
			input:           GetInput{FullPath: testFullPath, IID: 1},
			handler:         testutil.ForbiddenHandler(t),
			wantErr:         true,
			wantErrContains: "note_id is required",
		},
		{
			name:  "returns error when the namespace does not exist",
			input: GetInput{FullPath: testFullPath, IID: 999, NoteID: 100},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlNamespaceNull)
				},
			}),
			wantErr:         true,
			wantErrContains: `epic not found in group "my-group" with IID 999`,
		},
		{
			name:  "returns error when the group holds no epic with that iid",
			input: GetInput{FullPath: testFullPath, IID: 999, NoteID: 100},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlWorkItemNull)
				},
			}),
			wantErr:         true,
			wantErrContains: `epic not found in group "my-group" with IID 999`,
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
			name:            "returns error on cancelled context",
			input:           GetInput{FullPath: testFullPath, IID: 1, NoteID: 100},
			handler:         testutil.ForbiddenHandler(t),
			cancelFn:        true,
			wantErr:         true,
			wantErrContains: "context canceled",
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
			if err != nil && tt.wantErrContains != "" && !strings.Contains(err.Error(), tt.wantErrContains) {
				t.Errorf("Get() error = %v, want it to contain %q", err, tt.wantErrContains)
			}
			if tt.validate != nil {
				tt.validate(t, out)
			}
		})
	}
}

// TestCreate_SendsTheResolvedEpicAndTheBody verifies that the createNote
// mutation carries the work item GID the lookup resolved under noteableId and
// the caller's text under body.
//
// NoteableID is a custom scalar, so both values are strings the schema accepts
// in either position: swapped, the mutation would attach a note whose text is
// an epic's GID to a noteable named after the caller's comment, and the mock
// would answer with the same created note either way.
func TestCreate_SendsTheResolvedEpicAndTheBody(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"workItem(iid": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, gqlWorkItemGIDData)
		},
		"createNote": func(w http.ResponseWriter, r *http.Request) {
			assertGraphQLVariables(t, r, "Create()", map[string]any{
				"noteableId": "gid://gitlab/WorkItem/1",
				"body":       "New comment",
			})
			testutil.RespondGraphQL(w, http.StatusOK, gqlCreateNoteData)
		},
	})

	_, err := Create(context.Background(), testutil.NewTestClient(t, handler),
		CreateInput{FullPath: testFullPath, IID: 5, Body: "New comment"})
	if err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
}

// TestCreate verifies the Create handler.
// The call under test is two GraphQL POSTs: the work item lookup that resolves
// the epic's GID, then the createNote mutation.
// It asserts the returned output matches the expected fields.
func TestCreate(t *testing.T) {
	tests := []struct {
		name            string
		input           CreateInput
		handler         http.Handler
		cancelFn        bool
		wantErr         bool
		wantErrContains string
		validate        func(t *testing.T, out Output)
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
				assertEpicNote(t, "Create()", out, Output{
					ID:        200,
					Body:      "New comment",
					Author:    &NoteUserOutput{ID: 5, Username: "alice", Name: "Alice Example"},
					CreatedAt: "2026-01-16T10:00:00Z",
					UpdatedAt: "2026-01-16T10:00:00Z",
				})
			},
		},
		{
			name:            "returns error when full_path is empty",
			input:           CreateInput{IID: 1, Body: "note"},
			handler:         testutil.ForbiddenHandler(t),
			wantErr:         true,
			wantErrContains: "full_path is required",
		},
		{
			name:            "returns error when iid is zero",
			input:           CreateInput{FullPath: testFullPath, Body: "note"},
			handler:         testutil.ForbiddenHandler(t),
			wantErr:         true,
			wantErrContains: "epic_iid is required",
		},
		{
			name:            "returns error when body is empty",
			input:           CreateInput{FullPath: testFullPath, IID: 1},
			handler:         testutil.ForbiddenHandler(t),
			wantErr:         true,
			wantErrContains: "body is required",
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
			name:            "returns error on cancelled context",
			input:           CreateInput{FullPath: testFullPath, IID: 1, Body: "note"},
			handler:         testutil.ForbiddenHandler(t),
			cancelFn:        true,
			wantErr:         true,
			wantErrContains: "context canceled",
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
			if err != nil && tt.wantErrContains != "" && !strings.Contains(err.Error(), tt.wantErrContains) {
				t.Errorf("Create() error = %v, want it to contain %q", err, tt.wantErrContains)
			}
			if tt.validate != nil {
				tt.validate(t, out)
			}
		})
	}
}

// TestUpdate_SendsTheNoteTheCallerNamed verifies that the updateNote mutation
// edits the note note_id names and carries the caller's text under body.
//
// The input holds two identifiers of one type and only one of them belongs in
// the GID, so a mutation built from the epic IID beside it would edit whatever
// note happens to carry that number. The IID and the note ID differ here for
// that reason, and the mock answers with its fixture whichever GID it was sent.
func TestUpdate_SendsTheNoteTheCallerNamed(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{"updateNote": func(w http.ResponseWriter, r *http.Request) {
		assertGraphQLVariables(t, r, "Update()", map[string]any{
			"id":   "gid://gitlab/Note/100",
			"body": "Updated",
		})
		testutil.RespondGraphQL(w, http.StatusOK, gqlUpdateNoteData)
	}})

	_, err := Update(context.Background(), testutil.NewTestClient(t, handler),
		UpdateInput{FullPath: testFullPath, IID: 5, NoteID: 100, Body: "Updated"})
	if err != nil {
		t.Fatalf("Update() error = %v, want nil", err)
	}
}

// TestUpdate verifies the Update handler.
// The call under test is the updateNote GraphQL mutation, which takes the note
// GID directly and so needs no work item lookup.
// It asserts the returned output matches the expected fields.
func TestUpdate(t *testing.T) {
	tests := []struct {
		name            string
		input           UpdateInput
		handler         http.Handler
		cancelFn        bool
		wantErr         bool
		wantErrContains string
		validate        func(t *testing.T, out Output)
	}{
		{
			// The fixture's two timestamps differ, so an edited note keeps the
			// moment it was written and reports the moment it was changed.
			name:  "updates note and returns output",
			input: UpdateInput{FullPath: testFullPath, IID: 1, NoteID: 100, Body: "Updated"},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"updateNote": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlUpdateNoteData)
				},
			}),
			validate: func(t *testing.T, out Output) {
				t.Helper()
				assertEpicNote(t, "Update()", out, Output{
					ID:        100,
					Body:      "Updated comment",
					Author:    &NoteUserOutput{ID: 5, Username: "alice", Name: "Alice Example"},
					CreatedAt: "2026-01-15T10:00:00Z",
					UpdatedAt: "2026-01-16T11:00:00Z",
				})
			},
		},
		{
			name:            "returns error when full_path is empty",
			input:           UpdateInput{IID: 1, NoteID: 100, Body: "x"},
			handler:         testutil.ForbiddenHandler(t),
			wantErr:         true,
			wantErrContains: "full_path is required",
		},
		{
			name:            "returns error when iid is zero",
			input:           UpdateInput{FullPath: testFullPath, NoteID: 100, Body: "x"},
			handler:         testutil.ForbiddenHandler(t),
			wantErr:         true,
			wantErrContains: "epic_iid is required",
		},
		{
			name:            "returns error when note_id is zero",
			input:           UpdateInput{FullPath: testFullPath, IID: 1, Body: "x"},
			handler:         testutil.ForbiddenHandler(t),
			wantErr:         true,
			wantErrContains: "note_id is required",
		},
		{
			name:            "returns error when body is empty",
			input:           UpdateInput{FullPath: testFullPath, IID: 1, NoteID: 100},
			handler:         testutil.ForbiddenHandler(t),
			wantErr:         true,
			wantErrContains: "body is required",
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
			name:            "returns error on cancelled context",
			input:           UpdateInput{FullPath: testFullPath, IID: 1, NoteID: 100, Body: "x"},
			handler:         testutil.ForbiddenHandler(t),
			cancelFn:        true,
			wantErr:         true,
			wantErrContains: "context canceled",
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
			if err != nil && tt.wantErrContains != "" && !strings.Contains(err.Error(), tt.wantErrContains) {
				t.Errorf("Update() error = %v, want it to contain %q", err, tt.wantErrContains)
			}
			if tt.validate != nil {
				tt.validate(t, out)
			}
		})
	}
}

// TestDelete_SendsTheNoteTheCallerNamed verifies that the destroyNote mutation
// removes the note note_id names.
//
// It is the Update case with less around it: this handler decodes the
// mutation's errors and no note at all, so a GID built from the epic IID beside
// note_id destroys another note and the caller is still told the deletion
// succeeded. The IID and the note ID differ here so the request says which of
// them was read.
func TestDelete_SendsTheNoteTheCallerNamed(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{"destroyNote": func(w http.ResponseWriter, r *http.Request) {
		assertGraphQLVariables(t, r, "Delete()", map[string]any{"id": "gid://gitlab/Note/100"})
		testutil.RespondGraphQL(w, http.StatusOK, gqlDestroyNoteData)
	}})

	if err := Delete(context.Background(), testutil.NewTestClient(t, handler),
		DeleteInput{FullPath: testFullPath, IID: 5, NoteID: 100}); err != nil {
		t.Fatalf("Delete() error = %v, want nil", err)
	}
}

// TestDelete verifies the Delete handler.
// The call under test is the destroyNote GraphQL mutation.
// It asserts that a refused deletion is reported and an accepted one is not.
func TestDelete(t *testing.T) {
	tests := []struct {
		name            string
		input           DeleteInput
		handler         http.Handler
		cancelFn        bool
		wantErr         bool
		wantErrContains string
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
			name:            "returns error when full_path is empty",
			input:           DeleteInput{IID: 1, NoteID: 100},
			handler:         testutil.ForbiddenHandler(t),
			wantErr:         true,
			wantErrContains: "full_path is required",
		},
		{
			name:            "returns error when iid is zero",
			input:           DeleteInput{FullPath: testFullPath, NoteID: 100},
			handler:         testutil.ForbiddenHandler(t),
			wantErr:         true,
			wantErrContains: "epic_iid is required",
		},
		{
			name:            "returns error when note_id is zero",
			input:           DeleteInput{FullPath: testFullPath, IID: 1},
			handler:         testutil.ForbiddenHandler(t),
			wantErr:         true,
			wantErrContains: "note_id is required",
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
			name:            "returns error on cancelled context",
			input:           DeleteInput{FullPath: testFullPath, IID: 1, NoteID: 100},
			handler:         testutil.ForbiddenHandler(t),
			cancelFn:        true,
			wantErr:         true,
			wantErrContains: "context canceled",
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
			if err != nil && tt.wantErrContains != "" && !strings.Contains(err.Error(), tt.wantErrContains) {
				t.Errorf("Delete() error = %v, want it to contain %q", err, tt.wantErrContains)
			}
		})
	}
}

// The two guidance sections an epic note result ends with, so each expectation
// below can pin the whole rendered document.
const (
	noteHintsBlock = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'epic_note_update' with note_id to edit this note\n" +
		"- Use action 'epic_note_delete' with note_id to remove this note\n"
	listHintsBlock = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'epic_note_get' with note_id to read a specific note\n" +
		"- Use action 'epic_note_create' to add a new note to this epic\n"
)

// TestFormatOutputMarkdown uses table-driven subtests to pin the whole note
// card: the author as a handle, the system marker where it holds, and no author
// row at all for a note GitLab answered with none, where the card used to write
// a label with nothing after it.
func TestFormatOutputMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input Output
		want  string
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
			want: "## Epic Note #100\n\n" +
				"- **Author**: @alice\n" +
				"- **Created**: 15 Jan 2026 10:00 UTC\n" +
				"- **Body**: This looks good\n" +
				noteHintsBlock,
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
			want: "## Epic Note #101\n\n" +
				"- **Author**: @admin\n" +
				"- **Created**: 15 Jan 2026 12:00 UTC\n" +
				"- **System note**\n" +
				"- **Body**: changed the description\n" +
				noteHintsBlock,
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
			want: "## Epic Note #102\n\n" +
				"- **Created**: 15 Jan 2026 13:00 UTC\n" +
				"- **System note**\n" +
				"- **Body**: anonymous system entry\n" +
				noteHintsBlock,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatOutputMarkdown(tt.input); got != tt.want {
				t.Errorf("note card mismatch:\ngot:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

// TestFormatListMarkdown uses table-driven subtests to pin the whole list
// document: the table with the system flag as a glyph rather than the word
// "false", the cursor line the keyset connection ends with, and one sentence for
// an epic with no notes.
func TestFormatListMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input ListOutput
		want  string
	}{
		{
			name: "renders table with notes",
			input: ListOutput{
				Notes: []Output{
					{ID: 100, Author: authorObj("alice"), CreatedAt: "2026-01-15T10:00:00Z", System: false},
					{ID: 101, Author: authorObj("admin"), CreatedAt: "2026-01-15T12:00:00Z", System: true},
				},
				Pagination: toolutil.GraphQLForwardPaginationOutput{HasNextPage: false},
			},
			want: "## Epic Notes (2)\n\n" +
				"| ID | Author | Created | System |\n" +
				"| --- | --- | --- | --- |\n" +
				"| 100 | alice | 15 Jan 2026 10:00 UTC | ❌ |\n" +
				"| 101 | admin | 15 Jan 2026 12:00 UTC | ✅ |\n\n" +
				"Showing 2 items | no more pages\n" +
				listHintsBlock,
		},
		{
			name: "renders the next-page cursor when one follows",
			input: ListOutput{
				Notes:      []Output{{ID: 100, Author: authorObj("alice"), CreatedAt: "2026-01-15T10:00:00Z"}},
				Pagination: toolutil.GraphQLForwardPaginationOutput{HasNextPage: true, EndCursor: "eyJpZCI6IjEwMCJ9"},
			},
			want: "## Epic Notes (1)\n\n" +
				"| ID | Author | Created | System |\n" +
				"| --- | --- | --- | --- |\n" +
				"| 100 | alice | 15 Jan 2026 10:00 UTC | ❌ |\n\n" +
				"Showing 1 items | next page cursor: `eyJpZCI6IjEwMCJ9`\n" +
				listHintsBlock,
		},
		{
			name: "renders empty state when no notes",
			input: ListOutput{
				Notes:      []Output{},
				Pagination: toolutil.GraphQLForwardPaginationOutput{},
			},
			want: "No epic notes found.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatListMarkdown(tt.input); got != tt.want {
				t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}
