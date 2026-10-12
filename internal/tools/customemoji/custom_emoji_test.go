// custom_emoji_test.go contains unit tests for GitLab custom emoji operations.
// Tests use httptest to mock the GitLab Custom Emoji API.
package customemoji

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const sampleEmojiNode = `{
	"id": "gid://gitlab/CustomEmoji/1",
	"name": "party_parrot",
	"url": "https://example.com/party_parrot.gif",
	"external": false,
	"createdAt": "2026-06-01T10:00:00Z"
}`

const sampleEmojiNode2 = `{
	"id": "gid://gitlab/CustomEmoji/2",
	"name": "shipit",
	"url": "https://example.com/shipit.png",
	"external": true,
	"createdAt": "2026-06-15T14:30:00Z"
}`

// graphqlMux returns an [http.Handler] that routes GraphQL requests to the
// appropriate handler based on the query operation name.
func graphqlMux(handlers map[string]http.HandlerFunc) http.Handler {
	return testutil.GraphQLHandler(handlers)
}

// List handler tests.

// TestList_Success verifies that List succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_Success(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"customEmoji": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"group": {
					"customEmoji": {
						"nodes": [`+sampleEmojiNode+`, `+sampleEmojiNode2+`],
						"pageInfo": {"hasNextPage": false, "hasPreviousPage": false, "endCursor": null, "startCursor": null}
					}
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := List(context.Background(), client, ListInput{GroupPath: "my-group"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(out.Emoji) != 2 {
		t.Fatalf("expected 2 emoji, got %d", len(out.Emoji))
	}

	e := out.Emoji[0]
	if e.ID != "gid://gitlab/CustomEmoji/1" {
		t.Errorf("emoji[0].ID = %q, want %q", e.ID, "gid://gitlab/CustomEmoji/1")
	}
	if e.Name != "party_parrot" {
		t.Errorf("emoji[0].Name = %q, want %q", e.Name, "party_parrot")
	}
	if e.URL != "https://example.com/party_parrot.gif" {
		t.Errorf("emoji[0].URL = %q, want %q", e.URL, "https://example.com/party_parrot.gif")
	}
	if e.External {
		t.Error("emoji[0].External = true, want false")
	}
	if e.CreatedAt != "2026-06-01T10:00:00Z" {
		t.Errorf("emoji[0].CreatedAt = %q, want %q", e.CreatedAt, "2026-06-01T10:00:00Z")
	}

	e2 := out.Emoji[1]
	if e2.Name != "shipit" {
		t.Errorf("emoji[1].Name = %q, want %q", e2.Name, "shipit")
	}
	if !e2.External {
		t.Error("emoji[1].External = false, want true")
	}
}

// TestList_EmptyGroup verifies the List_EmptyGroup handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_EmptyGroup(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"customEmoji": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"group": {
					"customEmoji": {
						"nodes": [],
						"pageInfo": {"hasNextPage": false, "hasPreviousPage": false, "endCursor": null, "startCursor": null}
					}
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := List(context.Background(), client, ListInput{GroupPath: "my-group"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(out.Emoji) != 0 {
		t.Fatalf("expected 0 emoji, got %d", len(out.Emoji))
	}
}

// TestList_IncludeAncestorGroups_SentOnlyWhenAskedFor verifies that
// include_ancestor_groups reaches GitLab as includeAncestorGroups when the
// caller sets it, and that the argument is left off otherwise, so GitLab's own
// default, false, decides as it does for every other client (issue 1342).
//
// It also holds the note the formatter reads: a subgroup listed without its
// ancestors is marked as such, and neither a top-level group nor a subgroup
// listed with them is, since only the first leaves out emoji the group can
// use.
func TestList_IncludeAncestorGroups_SentOnlyWhenAskedFor(t *testing.T) {
	cases := []struct {
		name        string
		input       ListInput
		wantSent    bool
		wantLeftOut bool
	}{
		{name: "subgroup, default", input: ListInput{GroupPath: "parent/sub"}, wantLeftOut: true},
		{name: "subgroup, ancestors asked for", input: ListInput{GroupPath: "parent/sub", IncludeAncestorGroups: true}, wantSent: true},
		{name: "top-level group, default", input: ListInput{GroupPath: "parent"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := graphqlMux(map[string]http.HandlerFunc{
				"customEmoji": func(w http.ResponseWriter, r *http.Request) {
					vars, err := testutil.ParseGraphQLVariables(r)
					if err != nil {
						t.Errorf("ParseGraphQLVariables error: %v", err)
						return
					}
					value, sent := vars["includeAncestorGroups"]
					if sent != tc.wantSent {
						t.Errorf("includeAncestorGroups sent = %v (%v), want sent = %v", sent, value, tc.wantSent)
					}
					if sent && value != true {
						t.Errorf("includeAncestorGroups = %v, want true", value)
					}
					testutil.RespondGraphQL(w, http.StatusOK, `{"group": {"customEmoji": {"nodes": [], "pageInfo": {"hasNextPage": false, "hasPreviousPage": false}}}}`)
				},
			})
			out, err := List(context.Background(), testutil.NewTestClient(t, handler), tc.input)
			if err != nil {
				t.Fatalf("List() error = %v", err)
			}
			if out.ancestorsLeftOut != tc.wantLeftOut {
				t.Errorf("ancestorsLeftOut = %v, want %v", out.ancestorsLeftOut, tc.wantLeftOut)
			}
		})
	}
}

// TestList_GroupNotFound verifies that List_GroupNotFound returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestList_GroupNotFound(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"customEmoji": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{"group": null}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	_, err := List(context.Background(), client, ListInput{GroupPath: "does/not-exist"})
	if err == nil {
		t.Fatal("expected error for nil group, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "not found")
	}
}

// TestList_MissingGroupPath verifies that List_MissingGroupPath returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestList_MissingGroupPath(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error for empty group_path, got nil")
	}
	if !strings.Contains(err.Error(), "group_path is required") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "group_path is required")
	}
}

// TestList_ServerError verifies that List_ServerError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestList_ServerError(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"customEmoji": func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "bad request", http.StatusBadRequest)
		},
	})

	client := testutil.NewTestClient(t, handler)
	_, err := List(context.Background(), client, ListInput{GroupPath: "my-group"})
	if err == nil {
		t.Fatal("expected error for server error, got nil")
	}
}

// TestList_Pagination verifies that List forwards pagination parameters to the GitLab API and parses the response metadata.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestList_Pagination(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"customEmoji": func(w http.ResponseWriter, r *http.Request) {
			vars, _ := testutil.ParseGraphQLVariables(r)
			after, _ := vars["after"].(string)
			if after == "cursor1" {
				testutil.RespondGraphQL(w, http.StatusOK, `{
					"group": {
						"customEmoji": {
							"nodes": [`+sampleEmojiNode2+`],
							"pageInfo": {"hasNextPage": false, "hasPreviousPage": true, "endCursor": "cursor2", "startCursor": "cursor1"}
						}
					}
				}`)
				return
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"group": {
					"customEmoji": {
						"nodes": [`+sampleEmojiNode+`],
						"pageInfo": {"hasNextPage": true, "hasPreviousPage": false, "endCursor": "cursor1", "startCursor": null}
					}
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)

	// First page.
	out, err := List(context.Background(), client, ListInput{GroupPath: "my-group"})
	if err != nil {
		t.Fatalf("List() page 1 error = %v", err)
	}
	if len(out.Emoji) != 1 {
		t.Fatalf("page 1: expected 1 emoji, got %d", len(out.Emoji))
	}
	if out.Emoji[0].Name != "party_parrot" {
		t.Errorf("page 1: emoji name = %q, want %q", out.Emoji[0].Name, "party_parrot")
	}
	if !out.Pagination.HasNextPage {
		t.Error("page 1: expected HasNextPage = true")
	}

	// Second page.
	out2, err := List(context.Background(), client, ListInput{
		GroupPath: "my-group",
		After:     "cursor1",
	})
	if err != nil {
		t.Fatalf("List() page 2 error = %v", err)
	}
	if len(out2.Emoji) != 1 {
		t.Fatalf("page 2: expected 1 emoji, got %d", len(out2.Emoji))
	}
	if out2.Emoji[0].Name != "shipit" {
		t.Errorf("page 2: emoji name = %q, want %q", out2.Emoji[0].Name, "shipit")
	}
	if out2.Pagination.HasNextPage {
		t.Error("page 2: expected HasNextPage = false")
	}
}

// TestList_GraphQLErrorsAreReported verifies that a document GitLab refused is
// answered with its errors rather than as a missing group.
//
// GitLab returns HTTP 200 with a top-level errors array and a null group, which
// client-go leaves for the caller to notice, so the nil check below would
// otherwise blame the group path for a fault in the query.
func TestList_GraphQLErrorsAreReported(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"customEmoji": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQLError(w, http.StatusOK, "Field 'customEmoji' doesn't accept argument 'last'")
		},
	})

	out, err := List(context.Background(), testutil.NewTestClient(t, handler), ListInput{GroupPath: "my-group"})
	if err == nil {
		t.Fatalf("List() = %+v, want the GraphQL errors reported", out)
	}
	if !strings.Contains(err.Error(), "doesn't accept argument") {
		t.Errorf("List() error = %v, want it to carry the GitLab message", err)
	}
	if strings.Contains(err.Error(), "not found") {
		t.Errorf("List() error = %v, want it not to blame the group path", err)
	}
}

// TestList_ConnectionNulledByGitLab_IsAnErrorNotAnEmptyPage verifies that a
// customEmoji connection GitLab answered null under a group that resolved is
// reported as an error, never as a group with no custom emoji (issue 1298).
//
// Group.customEmoji is nullable in the pinned schema, and GitLab nulls it with
// the reason in errors[] under its path when the resolver raised. Decoded as a
// value, the null read as an empty page and the card told the caller the
// group has none, which is what leads a model to create a duplicate.
func TestList_ConnectionNulledByGitLab_IsAnErrorNotAnEmptyPage(t *testing.T) {
	cases := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "with an error",
			body: `{"data":{"group":{"customEmoji":null}},"errors":[{"message":"Internal server error","path":["group","customEmoji"]}]}`,
			want: []string{"list_custom_emoji", "Internal server error"},
		},
		{
			name: "without an error",
			body: `{"data":{"group":{"customEmoji":null}}}`,
			want: []string{"list_custom_emoji", "customEmoji", "null"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := graphqlMux(map[string]http.HandlerFunc{
				"customEmoji": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondJSON(w, http.StatusOK, tc.body)
				},
			})
			out, err := List(context.Background(), testutil.NewTestClient(t, handler), ListInput{GroupPath: "my-group"})
			if err == nil {
				t.Fatalf("List() = %+v, want an error rather than an empty page", out)
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("List() error = %q, want it to contain %q", err, want)
				}
			}
		})
	}
}

// TestList_PageAnsweredInPart_SaysSoAndLeavesNullItemsOut verifies that a
// page GitLab answered beside errors[] is read as a page answered in part
// (issue 1298). GitLab nulls a node or a field it could not resolve where it
// sits and explains it in errors[], so a nulled emoji decoded as a row of
// zero values and the page read as complete. The nulled node is left out,
// the card says GitLab answered the page in part and names its errors, and a
// page left with no node at all is reported as the error it is rather than
// as a group with no custom emoji, which is what leads a model to create a
// duplicate.
func TestList_PageAnsweredInPart_SaysSoAndLeavesNullItemsOut(t *testing.T) {
	const internal = `[{"message":"Internal server error","path":["group","customEmoji","nodes",1,"url"]}]`
	cases := []partialPageCase{
		{name: "a node nulled with an error", nodes: sampleEmojiNode + `, null`, errors: internal, wantItems: 1, wantHint: "Internal server error"},
		{name: "a field nulled with an error", nodes: sampleEmojiNode, errors: internal, wantItems: 1, wantHint: "Internal server error"},
		{name: "a node nulled with no error", nodes: sampleEmojiNode + `, null`, wantItems: 1, wantHint: "GitLab answered 1 of the page's items null with no error"},
		{name: "every node nulled with an error", nodes: `null`, errors: internal, wantErr: "list_custom_emoji GraphQL errors: Internal server error"},
		{name: "every node nulled with no error", nodes: `null, null`, wantErr: "list_custom_emoji: GitLab answered 2 of the page's items null with no error"},
		{name: "a page answered whole", nodes: sampleEmojiNode, wantItems: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"data":{"group":{"customEmoji":{"nodes":[` + tc.nodes + `],"pageInfo":{"hasNextPage":false,"hasPreviousPage":false,"endCursor":"","startCursor":""}}}}` + tc.errorsMember() + `}`
			handler := graphqlMux(map[string]http.HandlerFunc{
				"customEmoji": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondJSON(w, http.StatusOK, body)
				},
			})
			out, err := List(context.Background(), testutil.NewTestClient(t, handler), ListInput{GroupPath: "my-group"})
			assertPartialPage(t, tc, len(out.Emoji), func() string { return FormatListMarkdown(out) }, err)
		})
	}
}

// partialPageCase is one way GitLab answers a page of a connection beside
// errors[] or with nodes nulled: the nodes it sends, its errors member, and
// either the error List must return or the items it keeps and the account
// the card names (an empty one for a page answered whole).
type partialPageCase struct {
	name      string
	nodes     string
	errors    string
	wantItems int
	wantErr   string
	wantHint  string
}

// errorsMember is the case's errors member of the response, empty for none.
func (tc partialPageCase) errorsMember() string {
	if tc.errors == "" {
		return ""
	}
	return `,"errors":` + tc.errors
}

// assertPartialPage holds what List answered to tc: the error it wants, or
// the items it keeps and a card that says it was answered in part, naming
// tc.wantHint, exactly when tc wants a hint.
func assertPartialPage(t *testing.T, tc partialPageCase, items int, card func() string, err error) {
	t.Helper()
	if tc.wantErr != "" {
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Fatalf("List() answered %d items and the error %v, want the error %q rather than a page", items, err, tc.wantErr)
		}
		return
	}
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if items != tc.wantItems {
		t.Fatalf("List() answered %d items, want %d", items, tc.wantItems)
	}
	text := card()
	partial := strings.Contains(text, "GitLab answered this page in part")
	if partial != (tc.wantHint != "") || !strings.Contains(text, tc.wantHint) {
		t.Errorf("the card says it was answered in part: %t, want %t naming %q:\n%s", partial, tc.wantHint != "", tc.wantHint, text)
	}
}

// TestList_BackwardPagination verifies that a caller following start_cursor
// backwards is sent before and last, and no first.
//
// The absence of first is the assertion that matters. GitLab's keyset
// connections refuse first beside last outright, and the ones backed by an
// array answer the pair with the head of the list, so a request carrying both
// is either an error or the page the caller had already read.
func TestList_BackwardPagination(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"customEmoji": func(w http.ResponseWriter, r *http.Request) {
			assertBackwardCursor(t, r)
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"group": {
					"customEmoji": {
						"nodes": [`+sampleEmojiNode+`],
						"pageInfo": {"hasNextPage": true, "hasPreviousPage": false, "endCursor": "cursor1", "startCursor": null}
					}
				}
			}`)
		},
	})

	out, err := List(context.Background(), testutil.NewTestClient(t, handler), ListInput{
		GroupPath: "my-group",
		Last:      new(5),
		Before:    "cursor1",
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(out.Emoji) != 1 {
		t.Errorf("got %d emoji, want 1", len(out.Emoji))
	}
}

// TestList_ContradictoryPageSizes verifies that naming both first and last is
// refused before a request is made. GitLab answers the pair with "Can only
// provide either first or last, not both", so guessing which one the caller
// meant would only turn a clear refusal into a wrong page.
func TestList_ContradictoryPageSizes(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"customEmoji": func(w http.ResponseWriter, _ *http.Request) {
			t.Error("List() reached GitLab with a contradictory page request")
			testutil.RespondGraphQL(w, http.StatusOK, `{"group": {"customEmoji": {"nodes": []}}}`)
		},
	})

	_, err := List(context.Background(), testutil.NewTestClient(t, handler), ListInput{
		GroupPath: "my-group",
		First:     new(10),
		Last:      new(5),
	})
	if err == nil {
		t.Fatal("List() error = nil, want a refusal naming the conflict")
	}
	if !strings.Contains(err.Error(), "first and last cannot be combined") {
		t.Errorf("List() error = %v, want it to name the conflict", err)
	}
}

// assertBackwardCursor checks the variables a backward page request puts on the
// wire. It reports rather than aborting, because it runs on the server's
// goroutine.
func assertBackwardCursor(t *testing.T, r *http.Request) {
	t.Helper()
	vars, err := testutil.ParseGraphQLVariables(r)
	if err != nil {
		t.Errorf("ParseGraphQLVariables error: %v", err)
		return
	}
	if last, ok := vars["last"].(float64); !ok || int(last) != 5 {
		t.Errorf("last = %v, want 5", vars["last"])
	}
	if before, ok := vars["before"].(string); !ok || before != "cursor1" {
		t.Errorf("before = %v, want cursor1", vars["before"])
	}
	if first, ok := vars["first"]; ok {
		t.Errorf("first = %v, want it unset when the caller paged backwards", first)
	}
}

// TestList_NullCreatedAt verifies the List_NullCreatedAt handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_NullCreatedAt(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"customEmoji": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"group": {
					"customEmoji": {
						"nodes": [{
							"id": "gid://gitlab/CustomEmoji/99",
							"name": "test_emoji",
							"url": "https://example.com/test.png",
							"external": false,
							"createdAt": null
						}],
						"pageInfo": {"hasNextPage": false, "hasPreviousPage": false, "endCursor": null, "startCursor": null}
					}
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := List(context.Background(), client, ListInput{GroupPath: "my-group"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(out.Emoji) != 1 {
		t.Fatalf("expected 1 emoji, got %d", len(out.Emoji))
	}
	if out.Emoji[0].CreatedAt != "" {
		t.Errorf("CreatedAt = %q, want empty", out.Emoji[0].CreatedAt)
	}
}

// Create handler tests.

// TestCreate_Success verifies that Create succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreate_Success(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"createCustomEmoji": func(w http.ResponseWriter, r *http.Request) {
			vars, _ := testutil.ParseGraphQLVariables(r)
			name, _ := vars["name"].(string)
			if name != "party_parrot" {
				t.Errorf("expected name %q, got %q", "party_parrot", name)
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"createCustomEmoji": {
					"customEmoji": `+sampleEmojiNode+`,
					"errors": []
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := Create(context.Background(), client, CreateInput{
		GroupPath: "my-group",
		Name:      "party_parrot",
		URL:       "https://example.com/party_parrot.gif",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if out.Emoji.ID != "gid://gitlab/CustomEmoji/1" {
		t.Errorf("Emoji.ID = %q, want %q", out.Emoji.ID, "gid://gitlab/CustomEmoji/1")
	}
	if out.Emoji.Name != "party_parrot" {
		t.Errorf("Emoji.Name = %q, want %q", out.Emoji.Name, "party_parrot")
	}
	if out.Emoji.URL != "https://example.com/party_parrot.gif" {
		t.Errorf("Emoji.URL = %q, want %q", out.Emoji.URL, "https://example.com/party_parrot.gif")
	}
}

// TestCreate_MissingGroupPath verifies that Create_MissingGroupPath returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreate_MissingGroupPath(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	_, err := Create(context.Background(), client, CreateInput{Name: "test", URL: "https://example.com/test.png"})
	if err == nil {
		t.Fatal("expected error for empty group_path, got nil")
	}
	if !strings.Contains(err.Error(), "group_path is required") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "group_path is required")
	}
}

// TestCreate_MissingName verifies that Create_MissingName returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreate_MissingName(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	_, err := Create(context.Background(), client, CreateInput{GroupPath: "my-group", URL: "https://example.com/test.png"})
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "name is required")
	}
}

// TestCreate_MissingURL verifies that Create_MissingURL returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreate_MissingURL(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	_, err := Create(context.Background(), client, CreateInput{GroupPath: "my-group", Name: "test"})
	if err == nil {
		t.Fatal("expected error for empty url, got nil")
	}
	if !strings.Contains(err.Error(), "url is required") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "url is required")
	}
}

// TestCreate_MutationErrors verifies that Create_MutationErrors returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreate_MutationErrors(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"createCustomEmoji": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"createCustomEmoji": {
					"customEmoji": null,
					"errors": ["Name has already been taken"]
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	_, err := Create(context.Background(), client, CreateInput{
		GroupPath: "my-group",
		Name:      "party_parrot",
		URL:       "https://example.com/party_parrot.gif",
	})
	if err == nil {
		t.Fatal("expected error for mutation errors, got nil")
	}
	if !strings.Contains(err.Error(), "Name has already been taken") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "Name has already been taken")
	}
}

// TestCreate_ServerError verifies that Create_ServerError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreate_ServerError(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"createCustomEmoji": func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "bad request", http.StatusBadRequest)
		},
	})

	client := testutil.NewTestClient(t, handler)
	_, err := Create(context.Background(), client, CreateInput{
		GroupPath: "my-group",
		Name:      "test",
		URL:       "https://example.com/test.png",
	})
	if err == nil {
		t.Fatal("expected error for server error, got nil")
	}
}

// nullEmojiHandler answers a creation the way GitLab answers one whose
// returned emoji it nulled: the payload with no emoji and no error.
func nullEmojiHandler() http.Handler {
	return graphqlMux(map[string]http.HandlerFunc{
		"createCustomEmoji": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"createCustomEmoji": {
					"customEmoji": null,
					"errors": []
				}
			}`)
		},
	})
}

// TestCreate_NullEmoji verifies that a classic session whose creation GitLab
// answered with no emoji and no error gets the handler's own error, and is
// never told the write was probably committed: that sentence is a
// fine-grained token's alone (issue 1103).
func TestCreate_NullEmoji(t *testing.T) {
	client := testutil.NewTestClient(t, nullEmojiHandler())
	_, err := Create(context.Background(), client, CreateInput{
		GroupPath: "my-group",
		Name:      "test",
		URL:       "https://example.com/test.png",
	})
	if err == nil {
		t.Fatal("expected error for null emoji, got nil")
	}
	if err.Error() != "create_custom_emoji: no emoji returned" {
		t.Errorf("error = %q, want the handler's own %q", err.Error(), "create_custom_emoji: no emoji returned")
	}
	if errors.Is(err, finegrained.ErrUnconfirmedWrite) {
		t.Errorf("a classic session's error reads as an unconfirmed write: %v", err)
	}
}

// TestCreate_NullEmoji_FineGrainedSessionIsToldTheWriteProbablyCommitted
// verifies the answer a fine-grained session gets to the same creation:
// GitLab checks the emoji a creation returns only after it created it, and
// CustomEmoji declares no fine-grained permission at 19.4.1, so the emoji
// exists and the answer says so rather than reading as a creation that did
// not happen, which a model would repeat (issue 1103).
func TestCreate_NullEmoji_FineGrainedSessionIsToldTheWriteProbablyCommitted(t *testing.T) {
	client := testutil.NewTestClient(t, nullEmojiHandler())
	client.SetAuthority(finegrained.Unevaluated(&finegrained.Table{Version: "19.4.1-ee"}, finegrained.FallbackNone, ""))

	_, err := Create(context.Background(), client, CreateInput{
		GroupPath: "my-group",
		Name:      "test",
		URL:       "https://example.com/test.png",
	})

	if !errors.Is(err, finegrained.ErrUnconfirmedWrite) {
		t.Fatalf("Create error = %v, want it to be an unconfirmed write", err)
	}
	if want := "create_custom_emoji: GitLab answered without the custom emoji this write returns."; !strings.HasPrefix(err.Error(), want) {
		t.Errorf("error = %q, want it to open with %q", err.Error(), want)
	}
	if !strings.Contains(err.Error(), "probably committed") {
		t.Errorf("error = %q, want it to say the write was probably committed", err.Error())
	}
}

// Delete handler tests.

// TestDelete_Success verifies that Delete succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDelete_Success(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"destroyCustomEmoji": func(w http.ResponseWriter, r *http.Request) {
			vars, _ := testutil.ParseGraphQLVariables(r)
			id, _ := vars["id"].(string)
			if id != "gid://gitlab/CustomEmoji/1" {
				t.Errorf("expected id %q, got %q", "gid://gitlab/CustomEmoji/1", id)
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"destroyCustomEmoji": {
					"customEmoji": {"id": "gid://gitlab/CustomEmoji/1", "name": "party_parrot"},
					"errors": []
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	err := Delete(context.Background(), client, DeleteInput{ID: "gid://gitlab/CustomEmoji/1"})
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
}

// TestDelete_MissingID verifies that Delete_MissingID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDelete_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	err := Delete(context.Background(), client, DeleteInput{})
	if err == nil {
		t.Fatal("expected error for empty id, got nil")
	}
	if !strings.Contains(err.Error(), "id is required") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "id is required")
	}
}

// TestDelete_MutationErrors verifies that Delete_MutationErrors returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDelete_MutationErrors(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"destroyCustomEmoji": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"destroyCustomEmoji": {
					"customEmoji": null,
					"errors": ["Custom emoji not found"]
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	err := Delete(context.Background(), client, DeleteInput{ID: "gid://gitlab/CustomEmoji/999"})
	if err == nil {
		t.Fatal("expected error for mutation errors, got nil")
	}
	if !strings.Contains(err.Error(), "Custom emoji not found") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "Custom emoji not found")
	}
}

// TestDelete_ServerError verifies that Delete_ServerError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDelete_ServerError(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"destroyCustomEmoji": func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "bad request", http.StatusBadRequest)
		},
	})

	client := testutil.NewTestClient(t, handler)
	err := Delete(context.Background(), client, DeleteInput{ID: "gid://gitlab/CustomEmoji/1"})
	if err == nil {
		t.Fatal("expected error for server error, got nil")
	}
}

// Refused mutations.

// TestMutations_Refused_IsAnErrorNamingGitLabsReason verifies Create and
// Delete against a mutation GitLab refused. GitLab answers it with HTTP 200,
// the mutation's field null and the reason in the top-level errors, which
// client-go does not turn into an error; Delete used to read the null payload
// as a deletion that had happened, and Create as an emoji GitLab did not
// return, dropping GitLab's reason. Three answers are held for each: a
// fine-grained token not granted the permission, whose sentence the errors
// layer then describes as the missing permission it is, GitLab's generic
// refusal, and a null payload with no reason at all, which is an error rather
// than a success too.
func TestMutations_Refused_IsAnErrorNamingGitLabsReason(t *testing.T) {
	handlers := []struct {
		name       string
		op         string
		field      string
		permission string
		call       func(ctx context.Context, t *testing.T, handler http.Handler) error
	}{
		{
			name:       "Create",
			op:         "create_custom_emoji",
			field:      "createCustomEmoji",
			permission: "Custom Emoji: Create",
			call: func(ctx context.Context, t *testing.T, handler http.Handler) error {
				t.Helper()
				_, err := Create(ctx, testutil.NewTestClient(t, handler), CreateInput{GroupPath: "my-group", Name: "party_parrot", URL: "https://example.com/party_parrot.gif"})
				return err
			},
		},
		{
			name:       "Delete",
			op:         "delete_custom_emoji",
			field:      "destroyCustomEmoji",
			permission: "Custom Emoji: Delete",
			call: func(ctx context.Context, t *testing.T, handler http.Handler) error {
				t.Helper()
				return Delete(ctx, testutil.NewTestClient(t, handler), DeleteInput{ID: "gid://gitlab/CustomEmoji/1"})
			},
		},
	}
	for _, h := range handlers {
		fineGrained := "Access denied: This operation requires a fine-grained personal access token with the following group permissions: [" + h.permission + "]."
		answers := []struct {
			name string
			body string
			want string
		}{
			{
				name: "a fine-grained token without the permission",
				body: `{"data":{"` + h.field + `":null},"errors":[{"message":"` + fineGrained + `","path":["` + h.field + `"]}]}`,
				want: h.op + " GraphQL errors: " + fineGrained,
			},
			{
				name: "GitLab's generic refusal",
				body: `{"data":{"` + h.field + `":null},"errors":[{"message":"The resource that you are attempting to access does not exist or you don't have permission to perform this action"}]}`,
				want: "you don't have permission to perform this action",
			},
			{
				name: "no payload and no reason",
				body: `{"data":{"` + h.field + `":null}}`,
				want: h.op + ": GitLab answered with no " + h.field + " payload",
			},
		}
		for _, answer := range answers {
			t.Run(h.name+"/"+answer.name, func(t *testing.T) {
				handler := graphqlMux(map[string]http.HandlerFunc{
					h.field: func(w http.ResponseWriter, _ *http.Request) {
						testutil.RespondJSON(w, http.StatusOK, answer.body)
					},
				})
				err := h.call(context.Background(), t, handler)
				if err == nil {
					t.Fatalf("%s() = nil, want the refusal reported as an error", h.name)
				}
				if !strings.Contains(err.Error(), answer.want) {
					t.Errorf("%s() error = %q, want it to contain %q", h.name, err, answer.want)
				}
			})
		}
		t.Run(h.name+"/the errors layer names the missing permission", func(t *testing.T) {
			handler := graphqlMux(map[string]http.HandlerFunc{
				h.field: func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondJSON(w, http.StatusOK, `{"data":{"`+h.field+`":null},"errors":[{"message":"`+fineGrained+`"}]}`)
				},
			})
			err := h.call(context.Background(), t, handler)
			want := "access denied: this call needs the fine-grained group permission [" + h.permission + "]"
			if got := toolutil.SanitizeError(err).Error(); !strings.HasPrefix(got, want) {
				t.Errorf("SanitizeError(%s()) = %q, want it to begin %q", h.name, got, want)
			}
		})
	}
}

// The Markdown formatters are asserted whole in markdown_test.go.
