// register_test.go contains integration tests for the custom emoji tool
// closures in register.go. Tests exercise mutation error paths via an
// in-memory MCP session with a mock GitLab API.
package customemoji

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// TestRegisterTools_NoPanic verifies that RegisterTools registers all custom
// emoji tools without panicking.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// TestRegisterTools_CallThroughMCP verifies all registered custom emoji tools
// can be called through MCP in-memory transport, covering the handler closures.
func TestRegisterTools_CallThroughMCP(t *testing.T) {
	handler := testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"customEmoji": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"group": {
					"customEmoji": {
						"nodes": [`+sampleEmojiNode+`],
						"pageInfo": {"hasNextPage": false, "hasPreviousPage": false, "endCursor": null, "startCursor": null}
					}
				}
			}`)
		},
		"createCustomEmoji": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"createCustomEmoji": {
					"customEmoji": `+sampleEmojiNode+`,
					"errors": []
				}
			}`)
		},
		"destroyCustomEmoji": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{
				"destroyCustomEmoji": {
					"customEmoji": `+sampleEmojiNode+`,
					"errors": []
				}
			}`)
		},
	})
	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, connectErr := mcpClient.Connect(ctx, ct, nil)
	if connectErr != nil {
		t.Fatalf("client connect: %v", connectErr)
	}
	t.Cleanup(func() { session.Close() })

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_list_custom_emoji", map[string]any{"group_path": "my-group"}},
		{"gitlab_create_custom_emoji", map[string]any{"group_path": "my-group", "name": "test", "url": "https://example.com/e.png"}},
		{"gitlab_delete_custom_emoji", map[string]any{"id": "gid://gitlab/CustomEmoji/1"}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("CallTool(%s) returned nil", tt.name)
			}
		})
	}
}

// TestFormatCreateMarkdown_ExternalEmoji verifies that FormatCreateMarkdown
// correctly shows "Yes" for the External field when the emoji is external.
func TestFormatCreateMarkdown_ExternalEmoji(t *testing.T) {
	out := CreateOutput{
		Emoji: Item{
			ID:        "gid://gitlab/CustomEmoji/2",
			Name:      "shipit",
			URL:       "https://example.com/shipit.png",
			External:  true,
			CreatedAt: "2026-06-15T14:30:00Z",
		},
	}
	md := FormatCreateMarkdown(out)
	if !strings.Contains(md, "| External | Yes |") {
		t.Errorf("expected External=Yes in markdown, got:\n%s", md)
	}
}

// TestRegisterTools_DeleteError covers the if-err branch after Delete()
// in the gitlab_custom_emoji_delete closure in register.go.
func TestRegisterTools_DeleteError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "graphql") {
			testutil.RespondJSON(w, http.StatusOK, `{"data":{"destroyCustomEmoji":{"errors":["server error"]}}}`)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	})
	client := testutil.NewTestClient(t, mux)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	_, _ = server.Connect(ctx, st, nil)
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, connectErr := mcpClient.Connect(ctx, ct, nil)
	if connectErr != nil {
		t.Fatalf("client connect: %v", connectErr)
	}
	t.Cleanup(func() { session.Close() })

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_delete_custom_emoji",
		Arguments: map[string]any{"id": "gid://gitlab/CustomEmoji/1"},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Error("expected error result from gitlab_delete_custom_emoji")
	}
}

// TestRegisterTools_DeleteConfirmDeclined covers the ConfirmAction early-return
// branch in the gitlab_delete_custom_emoji handler when the user declines.
func TestRegisterTools_DeleteConfirmDeclined(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "decline"}, nil
		},
	})
	session, connectErr := mcpClient.Connect(ctx, ct, nil)
	if connectErr != nil {
		t.Fatalf("client connect: %v", connectErr)
	}
	t.Cleanup(func() { session.Close() })

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_delete_custom_emoji",
		Arguments: map[string]any{"id": "gid://gitlab/CustomEmoji/1"},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			if tc.Text == "" {
				t.Error("expected non-empty cancellation message")
			}
			return
		}
	}
	t.Error("expected text content in cancellation result")
}
