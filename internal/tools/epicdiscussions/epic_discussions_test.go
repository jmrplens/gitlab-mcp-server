// epic_discussions_test.go contains unit tests for the epic discussion MCP tool handlers.
// Tests use httptest to mock GitLab GraphQL API responses and verify success, error,
// and edge-case paths.
package epicdiscussions

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const testFullPath = "my-group"

// The refusals toolutil.ErrRequiredInt64 writes, asserted in full rather than
// by the field name alone.
//
// Every handler's API-failure hint also names the field it validates ("verify
// full_path + iid with gitlab_epic_list", "verify note_id with
// gitlab_list_epic_discussions"), so a case asserting only "iid" or "note_id"
// passes whether the guard refused the value or the request went out and came
// back an error. That is what let a guard reading `input.IID < 0` instead of
// `<= 0` sit under a green suite: zero reached GitLab and the failure that came
// back carried the word the assertion was looking for.
const (
	wantEpicIIDRequired = "epic_iid is required (must be > 0)"
	wantNoteIDRequired  = "note_id is required (must be > 0)"
)

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

const gqlDiscussionsNoWidget = `{
	"namespace": {
		"workItem": {
			"id": "gid://gitlab/WorkItem/1",
			"widgets": [{}]
		}
	}
}`

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

// TestDiscussionIDHelpers verifies the DiscussionIDHelpers handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDiscussionIDHelpers(t *testing.T) {
	if got := extractDiscussionHex("plain-id"); got != "plain-id" {
		t.Fatalf("extractDiscussionHex() = %q, want plain-id", got)
	}
	fullGID := "gid://gitlab/Discussion/d1hex"
	if got := formatDiscussionGID(fullGID); got != fullGID {
		t.Fatalf("formatDiscussionGID(full) = %q, want %q", got, fullGID)
	}
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
			_, err := resolveWorkItemGID(t.Context(), client, testFullPath, 5)
			if err == nil {
				t.Fatalf("expected error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// --------------------------------------------------------------------------
// List
// --------------------------------------------------------------------------

// TestListWith_UndeclaredPaginationVariable verifies that the list handler
// refuses to run an operation that declares fewer variables than its input can
// send, instead of sending one GitLab would discard.
//
// This is the defect the shared helper's signature exists to prevent: a
// variable an operation does not declare is ignored rather than rejected, so
// the caller would be answered with a page it did not ask for and no error.
// The discussions connection is forward-only upstream, so the document dropped
// here is the forward cursor rather than the backward pair.
//
// The shortened document is passed in rather than assigned over the package
// constant, so nothing a parallel neighbor reads changes underneath it.
func TestListWith_UndeclaredPaginationVariable(t *testing.T) {
	document := strings.Replace(queryListDiscussions, ", $after: String", "", 1)

	handler := graphqlMux(map[string]http.HandlerFunc{"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
		t.Error("listWith() ran an operation that cannot receive its own cursor")
		testutil.RespondGraphQL(w, http.StatusOK, gqlDiscussionsData)
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
		GetInput{FullPath: testFullPath, IID: 5, DiscussionID: "abc123"})
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
			gqlDiscussionsData,
			`"pageInfo": {"hasNextPage": false, "hasPreviousPage": false, "endCursor": "abc", "startCursor": "xyz"}`,
			`"pageInfo": {"hasNextPage": true, "hasPreviousPage": true, "endCursor": "abc", "startCursor": "xyz"}`,
			1,
		))
	}})

	out, err := List(context.Background(), testutil.NewTestClient(t, handler), ListInput{FullPath: testFullPath, IID: 5})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if !out.Pagination.HasNextPage || out.Pagination.EndCursor != "abc" {
		t.Errorf("Pagination = %+v, want the forward half preserved", out.Pagination)
	}
	if md := FormatListMarkdownString(out); strings.Contains(md, "xyz") {
		t.Errorf("markdown = %q, want no start cursor on a forward-only connection", md)
	}
}

// TestHandlers_IdentifierAtZero_RefusedWithoutReachingGitLab verifies that
// every handler taking a positive identifier refuses zero itself, and that
// nothing leaves for GitLab when it does.
//
// Zero is what an omitted JSON field deserializes to, so it is the value a
// model produces when it forgets epic_iid or note_id, and `<= 0` rather than
// `< 0` is the whole of what keeps it out. The suite could not tell the two
// spellings apart before: each case asserted the bare field name, which every
// handler's API-failure hint carries as well ("verify full_path + iid with
// gitlab_epic_list"), so a guard that admitted zero still produced an error
// with the word in it and the case passed for the wrong reason. Asserting that
// no request was made states what the guard is for, rather than the shape of
// whatever came back from letting the value through.
func TestHandlers_IdentifierAtZero_RefusedWithoutReachingGitLab(t *testing.T) {
	cases := []struct {
		name string
		want string
		call func(t *testing.T, handler http.Handler) error
	}{
		{
			name: "list rejects epic_iid",
			want: wantEpicIIDRequired,
			call: func(t *testing.T, handler http.Handler) error {
				t.Helper()
				_, err := List(t.Context(), testutil.NewTestClient(t, handler),
					ListInput{FullPath: testFullPath, IID: 0})
				return err
			},
		},
		{
			name: "get rejects epic_iid",
			want: wantEpicIIDRequired,
			call: func(t *testing.T, handler http.Handler) error {
				t.Helper()
				_, err := Get(t.Context(), testutil.NewTestClient(t, handler),
					GetInput{FullPath: testFullPath, IID: 0, DiscussionID: "d1hex"})
				return err
			},
		},
		{
			name: "create rejects epic_iid",
			want: wantEpicIIDRequired,
			call: func(t *testing.T, handler http.Handler) error {
				t.Helper()
				_, err := Create(t.Context(), testutil.NewTestClient(t, handler),
					CreateInput{FullPath: testFullPath, IID: 0, Body: "test"})
				return err
			},
		},
		{
			name: "add_note rejects epic_iid",
			want: wantEpicIIDRequired,
			call: func(t *testing.T, handler http.Handler) error {
				t.Helper()
				_, err := AddNote(t.Context(), testutil.NewTestClient(t, handler),
					AddNoteInput{FullPath: testFullPath, IID: 0, DiscussionID: "d1hex", Body: "test"})
				return err
			},
		},
		{
			name: "update_note rejects epic_iid",
			want: wantEpicIIDRequired,
			call: func(t *testing.T, handler http.Handler) error {
				t.Helper()
				_, err := UpdateNote(t.Context(), testutil.NewTestClient(t, handler),
					UpdateNoteInput{FullPath: testFullPath, IID: 0, NoteID: 100, Body: "test"})
				return err
			},
		},
		{
			name: "update_note rejects note_id",
			want: wantNoteIDRequired,
			call: func(t *testing.T, handler http.Handler) error {
				t.Helper()
				_, err := UpdateNote(t.Context(), testutil.NewTestClient(t, handler),
					UpdateNoteInput{FullPath: testFullPath, IID: 5, NoteID: 0, Body: "test"})
				return err
			},
		},
		{
			name: "delete_note rejects epic_iid",
			want: wantEpicIIDRequired,
			call: func(t *testing.T, handler http.Handler) error {
				t.Helper()
				return DeleteNote(t.Context(), testutil.NewTestClient(t, handler),
					DeleteNoteInput{FullPath: testFullPath, IID: 0, NoteID: 100})
			},
		},
		{
			name: "delete_note rejects note_id",
			want: wantNoteIDRequired,
			call: func(t *testing.T, handler http.Handler) error {
				t.Helper()
				return DeleteNote(t.Context(), testutil.NewTestClient(t, handler),
					DeleteNoteInput{FullPath: testFullPath, IID: 5, NoteID: 0})
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var reached atomic.Bool
			handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				reached.Store(true)
				testutil.RespondGraphQL(w, http.StatusOK, gqlDiscussionsData)
			})

			err := tc.call(t, handler)
			if err == nil {
				t.Fatalf("error = nil, want one containing %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want one containing %q", err, tc.want)
			}
			if reached.Load() {
				t.Error("a request reached GitLab; the identifier guard let zero through")
			}
		})
	}
}

// TestList_NoteTimestamps_ArePublishedAsGitLabSentThem verifies that a note's
// createdAt reaches created_at, that an updatedAt GitLab sent reaches
// updated_at, and that a null updatedAt publishes nothing.
//
// Both timestamps are optional in the schema and are dereferenced behind a nil
// check, so inverting either check publishes an empty string for every note
// that carries the field and dereferences a nil pointer for every note that
// does not. Nothing asserted created_at at all until this test: it appeared
// only in Markdown cases that set it on an output literal, which exercises the
// renderer and never the decode, so the timestamp a caller uses to order a
// thread could have gone missing with the suite still green.
func TestList_NoteTimestamps_ArePublishedAsGitLabSentThem(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondGraphQL(w, http.StatusOK, gqlDiscussionsData)
	}})

	out, err := List(context.Background(), testutil.NewTestClient(t, handler),
		ListInput{FullPath: testFullPath, IID: 5})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if len(out.Discussions) != 1 || len(out.Discussions[0].Notes) != 2 {
		t.Fatalf("List() = %+v, want one discussion carrying two notes", out.Discussions)
	}

	notes := out.Discussions[0].Notes
	if notes[0].CreatedAt != "2026-01-01T00:00:00Z" {
		t.Errorf("notes[0].CreatedAt = %q, want the createdAt GitLab sent", notes[0].CreatedAt)
	}
	if notes[0].UpdatedAt != "2026-01-01T00:00:00Z" {
		t.Errorf("notes[0].UpdatedAt = %q, want the updatedAt GitLab sent", notes[0].UpdatedAt)
	}
	if notes[1].CreatedAt != "2026-01-02T00:00:00Z" {
		t.Errorf("notes[1].CreatedAt = %q, want its own createdAt rather than its sibling's", notes[1].CreatedAt)
	}
	if notes[1].UpdatedAt != "" {
		t.Errorf("notes[1].UpdatedAt = %q, want nothing for a note GitLab reports no updatedAt for", notes[1].UpdatedAt)
	}
}

// TestList_NoteWithoutACreatedAt_PublishesNothingRatherThanFailing verifies
// that a note GitLab sends with a null createdAt publishes an empty created_at
// and keeps the rest of the note.
//
// createdAt is nullable on the schema even though a real note always carries
// one, so the nil check is what stands between a null and a panic in a handler
// serving a whole thread. Every fixture here carried the field, which left the
// guard evaluated one way only: it could have been dropped entirely and no
// test would have noticed until an instance sent a note without it.
func TestList_NoteWithoutACreatedAt_PublishesNothingRatherThanFailing(t *testing.T) {
	body := strings.Replace(gqlDiscussionsData, `"createdAt": "2026-01-02T00:00:00Z"`, `"createdAt": null`, 1)
	handler := graphqlMux(map[string]http.HandlerFunc{"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondGraphQL(w, http.StatusOK, body)
	}})

	out, err := List(context.Background(), testutil.NewTestClient(t, handler),
		ListInput{FullPath: testFullPath, IID: 5})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if len(out.Discussions) != 1 || len(out.Discussions[0].Notes) != 2 {
		t.Fatalf("List() = %+v, want one discussion carrying two notes", out.Discussions)
	}

	note := out.Discussions[0].Notes[1]
	if note.CreatedAt != "" {
		t.Errorf("CreatedAt = %q, want nothing for a note GitLab sent no createdAt for", note.CreatedAt)
	}
	if note.Body != "reply note" || note.Author != "bob" {
		t.Errorf("note = %+v, want the rest of it published regardless", note)
	}
}

// TestList_NoteIDGitLabSpelledUnparseably_KeepsTheRestOfTheThread verifies that
// a note whose GID this server cannot parse publishes id 0 and keeps its body,
// author and the notes around it.
//
// The parse is read through `err == nil` before the id is assigned, and every
// fixture handed it a well-formed GID, so the failing side of that check was
// never taken. What it decides matters more than the id it drops: the
// alternative to publishing zero would be failing the whole call, which loses
// every other note in the thread over one identifier GitLab spelled in a way
// this server did not expect.
func TestList_NoteIDGitLabSpelledUnparseably_KeepsTheRestOfTheThread(t *testing.T) {
	body := strings.Replace(gqlDiscussionsData, "gid://gitlab/Note/100", "gid://gitlab/Note/not-a-number", 1)
	handler := graphqlMux(map[string]http.HandlerFunc{"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondGraphQL(w, http.StatusOK, body)
	}})

	out, err := List(context.Background(), testutil.NewTestClient(t, handler),
		ListInput{FullPath: testFullPath, IID: 5})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if len(out.Discussions) != 1 || len(out.Discussions[0].Notes) != 2 {
		t.Fatalf("List() = %+v, want one discussion carrying two notes", out.Discussions)
	}

	notes := out.Discussions[0].Notes
	if notes[0].ID != 0 {
		t.Errorf("notes[0].ID = %d, want 0 for a GID that carries no integer", notes[0].ID)
	}
	if notes[0].Body != "first note" {
		t.Errorf("notes[0].Body = %q, want the note published regardless of its id", notes[0].Body)
	}
	if notes[1].ID != 101 {
		t.Errorf("notes[1].ID = %d, want 101; one unparseable id must not cost its siblings", notes[1].ID)
	}
}

// TestList_DiscussionID_IsWhatFollowsTheLastSeparator verifies that the
// discussion id published is the part after the final slash of the GID,
// whatever the GID looks like on either side of it.
//
// The id published here is the one a caller hands straight back to
// gitlab_get_epic_discussion and gitlab_add_epic_discussion_note, so a
// separator left on the front of it would be prefixed again into
// gid://gitlab/Discussion//d1hex and match no thread. The check that decides
// this reads `idx >= 0`, and the separator-at-the-first-character case is the
// only one where reading it as `idx > 0` answers differently: everything else
// either has the separator further along or has none at all.
func TestList_DiscussionID_IsWhatFollowsTheLastSeparator(t *testing.T) {
	cases := []struct {
		name string
		gid  string
	}{
		{name: "a full GitLab GID", gid: "gid://gitlab/Discussion/d1hex"},
		{name: "a separator at the first character", gid: "/d1hex"},
		{name: "no separator at all", gid: "d1hex"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := strings.Replace(gqlDiscussionsData, "gid://gitlab/Discussion/d1hex", tc.gid, 1)
			handler := graphqlMux(map[string]http.HandlerFunc{"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondGraphQL(w, http.StatusOK, body)
			}})

			out, err := List(context.Background(), testutil.NewTestClient(t, handler),
				ListInput{FullPath: testFullPath, IID: 5})
			if err != nil {
				t.Fatalf("List() error = %v, want nil", err)
			}
			if len(out.Discussions) != 1 {
				t.Fatalf("List() = %+v, want one discussion", out.Discussions)
			}
			if got := out.Discussions[0].ID; got != "d1hex" {
				t.Errorf("discussion ID = %q, want d1hex from GID %q", got, tc.gid)
			}
		})
	}
}

// TestList_NamespaceWithoutAWorkItem_ReportsTheEpicMissing verifies that a
// namespace GitLab resolved but whose work item is null is reported as an epic
// that is not there.
//
// GitLab answers a group that exists and an IID that does not with a namespace
// object carrying a null workItem, which is a different body from the null
// namespace a missing group produces. Only the second was ever driven here, so
// the work item half of the guard was never evaluated true and could have been
// dropped without a failing test, leaving a nil dereference on the ordinary
// case of a caller mistyping an epic number.
func TestList_NamespaceWithoutAWorkItem_ReportsTheEpicMissing(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondGraphQL(w, http.StatusOK, `{"namespace": {"workItem": null}}`)
	}})

	out, err := List(context.Background(), testutil.NewTestClient(t, handler),
		ListInput{FullPath: testFullPath, IID: 999})
	if err == nil {
		t.Fatalf("List() = %+v, want an error naming the epic", out)
	}
	if !strings.Contains(err.Error(), "epic not found") {
		t.Errorf("List() error = %v, want it to report the epic missing", err)
	}
}

// TestGet_NamespaceWithoutAWorkItem_ReportsTheEpicMissing verifies that Get
// answers a resolved namespace with a null work item the way List does.
//
// Get runs the same document through the same envelope and reaches the same
// pair of nil checks, so the work item half was unevaluated there for the same
// reason and would fail the same way: a caller who mistyped an epic number
// would take the process down rather than be told the epic is not there.
func TestGet_NamespaceWithoutAWorkItem_ReportsTheEpicMissing(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondGraphQL(w, http.StatusOK, `{"namespace": {"workItem": null}}`)
	}})

	out, err := Get(context.Background(), testutil.NewTestClient(t, handler),
		GetInput{FullPath: testFullPath, IID: 999, DiscussionID: "d1hex"})
	if err == nil {
		t.Fatalf("Get() = %+v, want an error naming the epic", out)
	}
	if !strings.Contains(err.Error(), "epic not found") {
		t.Errorf("Get() error = %v, want it to report the epic missing", err)
	}
}

// TestCreate_CreatedNoteWithoutItsDiscussion_PublishesTheNoteAndAnEmptyThreadID
// verifies that a createNote answer omitting the discussion it opened leaves
// the thread id empty and still publishes the note.
//
// The discussion is what makes this handler a thread opener rather than a note
// adder, and it is the one field only this document selects, so it is read
// through a nil check before the GID is split. Every fixture carried it, which
// left the check evaluated one way only: dereferencing a null discussion would
// panic inside a mutating handler, after GitLab has already written the note,
// which is the worst moment for this server to stop being able to answer.
func TestCreate_CreatedNoteWithoutItsDiscussion_PublishesTheNoteAndAnEmptyThreadID(t *testing.T) {
	body := strings.Replace(gqlCreateNoteData, `"discussion": {"id": "gid://gitlab/Discussion/d2hex"}`, `"discussion": null`, 1)
	handler := graphqlMux(map[string]http.HandlerFunc{
		"workItem(iid": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, gqlWorkItemGIDData)
		},
		"createNote": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, body)
		},
	})

	out, err := Create(context.Background(), testutil.NewTestClient(t, handler),
		CreateInput{FullPath: testFullPath, IID: 5, Body: "new thread"})
	if err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
	if out.ID != "" {
		t.Errorf("Create() ID = %q, want nothing when GitLab named no discussion", out.ID)
	}
	if len(out.Notes) != 1 || out.Notes[0].ID != 200 {
		t.Errorf("Create() Notes = %+v, want the created note published regardless", out.Notes)
	}
}

// TestList verifies the List handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList(t *testing.T) {
	tests := []listCase{
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
			name:  "skips widgets without discussions",
			input: ListInput{FullPath: testFullPath, IID: 5},
			handler: graphqlMux(map[string]http.HandlerFunc{"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondGraphQL(w, http.StatusOK, gqlDiscussionsNoWidget)
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
			wantErr: wantEpicIIDRequired,
		},
		{
			name:    "returns error when iid is negative",
			input:   ListInput{FullPath: testFullPath, IID: -1},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: wantEpicIIDRequired,
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
		t.Run(tt.name, func(t *testing.T) { runListCase(t, tt) })
	}
}

type listCase struct {
	name    string
	input   ListInput
	handler http.Handler
	wantErr string
	check   func(t *testing.T, out ListOutput)
}

func runListCase(t *testing.T, tt listCase) {
	t.Helper()
	client := testutil.NewTestClient(t, tt.handler)
	ctx := t.Context()
	if tt.name == "returns error on cancelled context" {
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(ctx)
		cancel()
	}
	out, err := List(ctx, client, tt.input)
	assertListCaseResult(t, out, err, tt.wantErr, tt.check)
}

func assertListCaseResult(t *testing.T, out ListOutput, err error, wantErr string, check func(t *testing.T, out ListOutput)) {
	t.Helper()
	if wantErr != "" {
		if err == nil {
			t.Fatalf("expected error containing %q, got nil", wantErr)
		}
		if !strings.Contains(err.Error(), wantErr) {
			t.Fatalf("error %q does not contain %q", err.Error(), wantErr)
		}
		return
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if check != nil {
		check(t, out)
	}
}

// --------------------------------------------------------------------------
// Get
// --------------------------------------------------------------------------

// TestGet verifies the Get handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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
			wantErr: wantEpicIIDRequired,
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
			name:  "returns error when widgets have no discussions",
			input: GetInput{FullPath: testFullPath, IID: 5, DiscussionID: "d1hex"},
			handler: graphqlMux(map[string]http.HandlerFunc{"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondGraphQL(w, http.StatusOK, gqlDiscussionsNoWidget)
			}}),
			wantErr: "discussion",
		},
		{
			name:  "returns error on API server error",
			input: GetInput{FullPath: testFullPath, IID: 5, DiscussionID: "d1hex"},
			handler: graphqlMux(map[string]http.HandlerFunc{"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "forbidden", http.StatusForbidden)
			}}),
			wantErr: "epicDiscussionGet",
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

// TestCreate verifies the Create handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreate(t *testing.T) {
	tests := []createDiscussionCase{
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
			wantErr: wantEpicIIDRequired,
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
			name:  "returns error when resolving epic fails",
			input: CreateInput{FullPath: testFullPath, IID: 5, Body: "test"},
			handler: graphqlMux(map[string]http.HandlerFunc{"workItem(iid": func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondGraphQL(w, http.StatusOK, gqlNamespaceNull)
			}}),
			wantErr: "epicDiscussionCreate",
		},
		{
			name:  "returns error on createNote API error",
			input: CreateInput{FullPath: testFullPath, IID: 5, Body: "test"},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"workItem(iid": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlWorkItemGIDData)
				},
				"createNote": func(w http.ResponseWriter, _ *http.Request) {
					http.Error(w, "forbidden", http.StatusForbidden)
				},
			}),
			wantErr: "epicDiscussionCreate",
		},
		{
			name:  "returns error when createNote returns no note",
			input: CreateInput{FullPath: testFullPath, IID: 5, Body: "test"},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"workItem(iid": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlWorkItemGIDData)
				},
				"createNote": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, `{"createNote":{"note":null,"errors":[]}}`)
				},
			}),
			wantErr: "no note returned",
		},
		{
			name:    "returns error on cancelled context",
			input:   CreateInput{FullPath: testFullPath, IID: 5, Body: "test"},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: "context canceled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { runCreateCase(t, tt) })
	}
}

type createDiscussionCase struct {
	name    string
	input   CreateInput
	handler http.Handler
	wantErr string
	check   func(t *testing.T, out Output)
}

func runCreateCase(t *testing.T, tt createDiscussionCase) {
	t.Helper()
	client := testutil.NewTestClient(t, tt.handler)
	ctx := t.Context()
	if tt.name == "returns error on cancelled context" {
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(ctx)
		cancel()
	}
	out, err := Create(ctx, client, tt.input)
	assertCreateCaseResult(t, out, err, tt.wantErr, tt.check)
}

func assertCreateCaseResult(t *testing.T, out Output, err error, wantErr string, check func(t *testing.T, out Output)) {
	t.Helper()
	if wantErr != "" {
		if err == nil {
			t.Fatalf("expected error containing %q, got nil", wantErr)
		}
		if !strings.Contains(err.Error(), wantErr) {
			t.Fatalf("error %q does not contain %q", err.Error(), wantErr)
		}
		return
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if check != nil {
		check(t, out)
	}
}

// --------------------------------------------------------------------------
// AddNote
// --------------------------------------------------------------------------

// TestAddNote verifies the AddNote handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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
			wantErr: wantEpicIIDRequired,
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
			name:  "returns error when resolving epic fails",
			input: AddNoteInput{FullPath: testFullPath, IID: 5, DiscussionID: "d1hex", Body: "test"},
			handler: graphqlMux(map[string]http.HandlerFunc{"workItem(iid": func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondGraphQL(w, http.StatusOK, gqlNamespaceNull)
			}}),
			wantErr: "epicDiscussionAddNote",
		},
		{
			name:  "returns error on createNote API error",
			input: AddNoteInput{FullPath: testFullPath, IID: 5, DiscussionID: "d1hex", Body: "test"},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"workItem(iid": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlWorkItemGIDData)
				},
				"createNote": func(w http.ResponseWriter, _ *http.Request) {
					http.Error(w, "forbidden", http.StatusForbidden)
				},
			}),
			wantErr: "epicDiscussionAddNote",
		},
		{
			name:  "returns error when createNote returns no note",
			input: AddNoteInput{FullPath: testFullPath, IID: 5, DiscussionID: "d1hex", Body: "test"},
			handler: graphqlMux(map[string]http.HandlerFunc{
				"workItem(iid": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, gqlWorkItemGIDData)
				},
				"createNote": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, `{"createNote":{"note":null,"errors":[]}}`)
				},
			}),
			wantErr: "no note returned",
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

// TestUpdateNote verifies the UpdateNote handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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
			wantErr: wantEpicIIDRequired,
		},
		{
			name:    "returns error when note_id is zero",
			input:   UpdateNoteInput{FullPath: testFullPath, IID: 5, NoteID: 0, Body: "test"},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: wantNoteIDRequired,
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
			name:  "returns error on updateNote API error",
			input: UpdateNoteInput{FullPath: testFullPath, IID: 5, NoteID: 100, Body: "test"},
			handler: graphqlMux(map[string]http.HandlerFunc{"updateNote": func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "forbidden", http.StatusForbidden)
			}}),
			wantErr: "epicDiscussionUpdateNote",
		},
		{
			name:  "returns error when updateNote returns no note",
			input: UpdateNoteInput{FullPath: testFullPath, IID: 5, NoteID: 100, Body: "test"},
			handler: graphqlMux(map[string]http.HandlerFunc{"updateNote": func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondGraphQL(w, http.StatusOK, `{"updateNote":{"note":null,"errors":[]}}`)
			}}),
			wantErr: "no note returned",
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

// TestDeleteNote verifies the DeleteNote handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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
			wantErr: wantEpicIIDRequired,
		},
		{
			name:    "returns error when note_id is zero",
			input:   DeleteNoteInput{FullPath: testFullPath, IID: 5, NoteID: 0},
			handler: graphqlMux(map[string]http.HandlerFunc{}),
			wantErr: wantNoteIDRequired,
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
			name:  "returns error on destroyNote API error",
			input: DeleteNoteInput{FullPath: testFullPath, IID: 5, NoteID: 100},
			handler: graphqlMux(map[string]http.HandlerFunc{"destroyNote": func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "forbidden", http.StatusForbidden)
			}}),
			wantErr: "epicDiscussionDeleteNote",
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

// The guidance sections the three shared renderers end with, so each
// expectation below can pin the whole rendered document.
const (
	listHintsBlock = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use `gitlab_get_epic_discussion` to view full discussion details\n"
	threadHintsBlock = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use `gitlab_add_epic_discussion_note` to reply to this discussion\n"
	noteHintsBlock = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use `gitlab_update_epic_discussion_note` to edit this note\n"
)

// TestFormatListMarkdownString uses table-driven subtests to pin the whole
// document the shared discussion list renderer writes: a thread per section with
// its notes quoted under their authors and the cursor line below them, and one
// sentence for an epic with no discussions.
func TestFormatListMarkdownString(t *testing.T) {
	tests := []struct {
		name  string
		input ListOutput
		want  string
	}{
		{
			name: "renders the threads with the cursor line",
			input: ListOutput{
				Discussions: []Output{
					{
						ID: "d1hex",
						Notes: []NoteOutput{
							{ID: 100, Body: "Hello", Author: "alice", CreatedAt: "2026-01-01T00:00:00Z"},
						},
					},
				},
				Pagination: toolutil.GraphQLForwardPaginationOutput{},
			},
			want: "## Epic Discussions (1)\n\n" +
				"### Discussion d1hex\n" +
				"- **@alice** (1 Jan 2026 00:00 UTC, note 100):\n" +
				"  > Hello\n\n" +
				"Showing 1 items | no more pages\n" +
				listHintsBlock,
		},
		{
			name:  "renders empty state when no discussions",
			input: ListOutput{},
			want:  "No epic discussions found.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatListMarkdownString(tt.input); got != tt.want {
				t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

// TestFormatMarkdownString uses table-driven subtests to pin the whole card of a
// thread with notes and of one GitLab answered with none.
func TestFormatMarkdownString(t *testing.T) {
	tests := []struct {
		name  string
		input Output
		want  string
	}{
		{
			name:  "renders discussion with notes",
			input: Output{ID: "d1hex", Notes: []NoteOutput{{ID: 1, Body: "note body", Author: "bob", CreatedAt: "2026-01-01T00:00:00Z"}}},
			want: "## Discussion d1hex\n\n" +
				"- **@bob** (1 Jan 2026 00:00 UTC, note 1):\n" +
				"  > note body\n" +
				threadHintsBlock,
		},
		{
			name:  "renders empty discussion",
			input: Output{ID: "d1hex"},
			want:  "## Discussion d1hex\n" + threadHintsBlock,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatMarkdownString(tt.input); got != tt.want {
				t.Errorf("thread card mismatch:\ngot:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

// TestFormatNoteMarkdownString pins the whole card of a note somebody wrote.
func TestFormatNoteMarkdownString(t *testing.T) {
	got := FormatNoteMarkdownString(NoteOutput{ID: 1, Body: "test note", Author: "carol", CreatedAt: "2026-01-01T00:00:00Z"})

	want := "## Discussion Note #1\n\n" +
		"- **Author**: @carol\n" +
		"- **Created**: 1 Jan 2026 00:00 UTC\n" +
		"- **Body**: test note\n" +
		noteHintsBlock
	if got != want {
		t.Errorf("note card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatNoteMarkdownString_SystemNote pins the card of a system note: the
// marker is written, where the view model used to drop the flag and a record
// GitLab wrote itself read as a comment somebody typed.
func TestFormatNoteMarkdownString_SystemNote(t *testing.T) {
	got := FormatNoteMarkdownString(NoteOutput{ID: 2, Body: "changed title", Author: "carol", CreatedAt: "2026-01-01T00:00:00Z", System: true})

	want := "## Discussion Note #2\n\n" +
		"- **Author**: @carol\n" +
		"- **Created**: 1 Jan 2026 00:00 UTC\n" +
		"- **System note**\n" +
		"- **Body**: changed title\n" +
		noteHintsBlock
	if got != want {
		t.Errorf("note card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// --------------------------------------------------------------------------
// --------------------------------------------------------------------------

// TestActionSpecs_CallAllRoutes validates the CallAllRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := epicDiscussionSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, graphqlSessionMux())))
	tools := []struct {
		name string
		tool string
		args map[string]any
		want string
	}{
		{"list", "gitlab_list_epic_discussions", map[string]any{"full_path": testFullPath, "epic_iid": float64(5)}, ""},
		{"get", "gitlab_get_epic_discussion", map[string]any{"full_path": testFullPath, "epic_iid": float64(5), "discussion_id": "d1hex"}, ""},
		{"create", "gitlab_create_epic_discussion", map[string]any{"full_path": testFullPath, "epic_iid": float64(5), "body": "new thread"}, ""},
		{"add_note", "gitlab_add_epic_discussion_note", map[string]any{"full_path": testFullPath, "epic_iid": float64(5), "discussion_id": "d1hex", "body": "reply"}, ""},
		{"update_note", "gitlab_update_epic_discussion_note", map[string]any{"full_path": testFullPath, "epic_iid": float64(5), "note_id": float64(100), "body": "updated"}, ""},
		{"delete_note", "gitlab_delete_epic_discussion_note", map[string]any{"full_path": testFullPath, "epic_iid": float64(5), "note_id": float64(100)}, "Successfully deleted epic discussion note."},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
			}
			if tt.want != "" {
				out, ok := result.(toolutil.DeleteOutput)
				if !ok {
					t.Fatalf("Route.Handler(%s) returned %T, want toolutil.DeleteOutput", tt.tool, result)
				}
				if out.Message != tt.want {
					t.Fatalf("delete message = %q, want %q", out.Message, tt.want)
				}
			}
		})
	}
}

// --------------------------------------------------------------------------
// MCP round-trip — meta tool
// --------------------------------------------------------------------------

// graphqlSessionMux creates a GraphQL handler for canonical route tests.
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
