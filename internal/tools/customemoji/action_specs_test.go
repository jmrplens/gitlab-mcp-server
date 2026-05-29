// action_specs_test.go contains canonical-route tests for custom emoji behavior.
package customemoji

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_Metadata verifies canonical metadata for custom emoji actions.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	byTool := customEmojiSpecsByTool(t, specs)

	if len(specs) != 3 {
		t.Fatalf("len(ActionSpecs) = %d, want 3", len(specs))
	}
	if len(byTool) != len(specs) {
		t.Fatalf("unique individual tools = %d, want %d", len(byTool), len(specs))
	}
	if !byTool["gitlab_delete_custom_emoji"].Route.Destructive {
		t.Fatal("gitlab_delete_custom_emoji should be destructive")
	}
	if byTool["gitlab_list_custom_emoji"].Usage == "" {
		t.Fatal("gitlab_list_custom_emoji should define usage")
	}
	if len(byTool["gitlab_create_custom_emoji"].Aliases) == 0 {
		t.Fatal("gitlab_create_custom_emoji should define aliases")
	}
	if byTool["gitlab_delete_custom_emoji"].ParameterGuidance["id"].SemanticRole == "" {
		t.Fatal("gitlab_delete_custom_emoji should define id parameter guidance")
	}
}

// TestActionSpecs_CallAllRoutes verifies custom emoji routes through canonical specs.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
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
	byTool := customEmojiSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, handler)))

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
			result, err := byTool[tt.name].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.name)
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

// TestActionSpecs_DeleteError covers the error branch after Delete.
func TestActionSpecs_DeleteError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "graphql") {
			testutil.RespondJSON(w, http.StatusOK, `{"data":{"destroyCustomEmoji":{"errors":["server error"]}}}`)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	})
	client := testutil.NewTestClient(t, mux)
	byTool := customEmojiSpecsByTool(t, ActionSpecs(client))

	_, err := byTool["gitlab_delete_custom_emoji"].Route.Handler(t.Context(), map[string]any{"id": "gid://gitlab/CustomEmoji/1"})
	if err == nil {
		t.Fatal("expected error from gitlab_delete_custom_emoji")
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)
	byTool := customEmojiSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_delete_custom_emoji"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test custom emoji destructive confirmation.",
		Icons:       toolutil.IconLabel,
	})

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
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
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})

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

func customEmojiSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		toolName := spec.IndividualTool.Name
		if toolName == "" {
			t.Fatalf("spec %s missing IndividualTool.Name", spec.Name)
		}
		if _, exists := byTool[toolName]; exists {
			t.Fatalf("duplicate individual tool %q", toolName)
		}
		byTool[toolName] = spec
	}
	return byTool
}
