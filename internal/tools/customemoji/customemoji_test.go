package customemoji

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const sampleEmojiNode = `{
	"id": "gid://gitlab/CustomEmoji/1",
	"name": "party_parrot",
	"url": "https://example.com/party_parrot.gif",
	"external": false,
	"createdAt": "2025-06-01T10:00:00Z"
}`

const sampleEmojiNode2 = `{
	"id": "gid://gitlab/CustomEmoji/2",
	"name": "shipit",
	"url": "https://example.com/shipit.png",
	"external": true,
	"createdAt": "2025-06-15T14:30:00Z"
}`

// graphqlMux returns an [http.Handler] that routes GraphQL requests to the
// appropriate handler based on the query operation name.
func graphqlMux(handlers map[string]http.HandlerFunc) http.Handler {
	return testutil.GraphQLHandler(handlers)
}

// List handler tests.

// TestList_Success verifies that listing custom emoji returns the expected
// items when the GraphQL API responds with valid emoji data.
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
	if e.CreatedAt != "2025-06-01T10:00:00Z" {
		t.Errorf("emoji[0].CreatedAt = %q, want %q", e.CreatedAt, "2025-06-01T10:00:00Z")
	}

	e2 := out.Emoji[1]
	if e2.Name != "shipit" {
		t.Errorf("emoji[1].Name = %q, want %q", e2.Name, "shipit")
	}
	if !e2.External {
		t.Error("emoji[1].External = false, want true")
	}
}

// TestList_EmptyGroup verifies that listing custom emoji for a group
// with no emoji returns an empty result set.
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

// TestList_GroupNotFound verifies that listing custom emoji returns an
// error when the specified group does not exist.
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

// TestList_MissingGroupPath verifies that listing custom emoji returns
// a validation error when the required group_path parameter is missing.
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

// TestList_ServerError verifies that listing custom emoji propagates
// errors when the GraphQL API returns a server error.
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

// TestList_Pagination verifies that cursor-based pagination parameters
// are correctly forwarded to the GraphQL API and page info is returned.
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
		GroupPath:              "my-group",
		GraphQLPaginationInput: toolutil.GraphQLPaginationInput{After: "cursor1"},
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

// TestList_NullCreatedAt verifies that custom emoji with null created_at
// fields are handled without errors.
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

// TestCreate_Success verifies that creating a custom emoji returns the
// expected item when the GraphQL mutation succeeds.
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

// TestCreate_MissingGroupPath verifies that creating a custom emoji returns
// a validation error when the required group_path parameter is missing.
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

// TestCreate_MissingName verifies that creating a custom emoji returns
// a validation error when the required name parameter is missing.
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

// TestCreate_MissingURL verifies that creating a custom emoji returns
// a validation error when the required url parameter is missing.
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

// TestCreate_MutationErrors verifies that GraphQL mutation-level errors
// (e.g., duplicate name) are properly propagated as tool errors.
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

// TestCreate_ServerError verifies that creating a custom emoji propagates
// errors when the GraphQL API returns a server error.
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

// TestCreate_NullEmoji verifies that a null customEmoji in the mutation
// response is handled as an error.
func TestCreate_NullEmoji(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"createCustomEmoji": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"createCustomEmoji": {
					"customEmoji": null,
					"errors": []
				}
			}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	_, err := Create(context.Background(), client, CreateInput{
		GroupPath: "my-group",
		Name:      "test",
		URL:       "https://example.com/test.png",
	})
	if err == nil {
		t.Fatal("expected error for null emoji, got nil")
	}
	if !strings.Contains(err.Error(), "no emoji returned") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "no emoji returned")
	}
}

// Delete handler tests.

// TestDelete_Success verifies that deleting a custom emoji succeeds when
// the GraphQL mutation returns without errors.
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

// TestDelete_MissingID verifies that deleting a custom emoji returns
// a validation error when the required id parameter is missing.
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

// TestDelete_MutationErrors verifies that GraphQL mutation-level errors
// on delete are properly propagated as tool errors.
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

// TestDelete_ServerError verifies that deleting a custom emoji propagates
// errors when the GraphQL API returns a server error.
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

// Markdown formatter tests.

// TestFormatListMarkdown_Empty verifies that formatting an empty custom
// emoji list produces the expected no-results Markdown message.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No custom emoji found") {
		t.Error("empty output should contain 'No custom emoji found'")
	}
}

// TestFormatListMarkdown_WithEmoji verifies that formatting custom emoji
// produces a Markdown table with name, URL, and external status.
func TestFormatListMarkdown_WithEmoji(t *testing.T) {
	out := ListOutput{
		Emoji: []Item{
			{
				ID:        "gid://gitlab/CustomEmoji/1",
				Name:      "party_parrot",
				URL:       "https://example.com/party_parrot.gif",
				External:  false,
				CreatedAt: "2025-06-01T10:00:00Z",
			},
			{
				ID:        "gid://gitlab/CustomEmoji/2",
				Name:      "shipit",
				URL:       "https://example.com/shipit.png",
				External:  true,
				CreatedAt: "2025-06-15T14:30:00Z",
			},
		},
		Pagination: toolutil.GraphQLPaginationOutput{HasNextPage: false},
	}

	md := FormatListMarkdown(out)
	if !strings.Contains(md, ":party_parrot:") {
		t.Error("should contain ':party_parrot:'")
	}
	if !strings.Contains(md, ":shipit:") {
		t.Error("should contain ':shipit:'")
	}
	if !strings.Contains(md, "Yes") {
		t.Error("should contain 'Yes' for external emoji")
	}
	if !strings.Contains(md, "2025-06-01") {
		t.Error("should contain created date")
	}
}

// TestFormatCreateMarkdown verifies that formatting a created custom emoji
// produces the expected Markdown detail block.
func TestFormatCreateMarkdown(t *testing.T) {
	out := CreateOutput{
		Emoji: Item{
			ID:        "gid://gitlab/CustomEmoji/1",
			Name:      "party_parrot",
			URL:       "https://example.com/party_parrot.gif",
			External:  false,
			CreatedAt: "2025-06-01T10:00:00Z",
		},
	}

	md := FormatCreateMarkdown(out)
	if !strings.Contains(md, "Custom emoji created") {
		t.Error("should contain success message")
	}
	if !strings.Contains(md, "party_parrot") {
		t.Error("should contain emoji name")
	}
	if !strings.Contains(md, "gid://gitlab/CustomEmoji/1") {
		t.Error("should contain emoji ID")
	}
	if !strings.Contains(md, "https://example.com/party_parrot.gif") {
		t.Error("should contain emoji URL")
	}
}
