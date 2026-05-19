// action_specs_test.go contains canonical-route tests for snippet discussion actions.
package snippetdiscussions

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_CallAllRoutes exercises every snippet discussion tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := snippetDiscussionsSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, snippetDiscussionsActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_list_snippet_discussions", map[string]any{"project_id": "1", "snippet_id": 5}},
		{"gitlab_get_snippet_discussion", map[string]any{"project_id": "1", "snippet_id": 5, "discussion_id": "d1"}},
		{"gitlab_create_snippet_discussion", map[string]any{"project_id": "1", "snippet_id": 5, "body": "test"}},
		{"gitlab_add_snippet_discussion_note", map[string]any{"project_id": "1", "snippet_id": 5, "discussion_id": "d1", "body": "reply"}},
		{"gitlab_update_snippet_discussion_note", map[string]any{"project_id": "1", "snippet_id": 5, "discussion_id": "d1", "note_id": 1, "body": "updated"}},
		{"gitlab_delete_snippet_discussion_note", map[string]any{"project_id": "1", "snippet_id": 5, "discussion_id": "d1", "note_id": 1}},
	}

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			result, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
			}
		})
	}
}

// TestActionSpecs_DeleteNoteError verifies that the delete route propagates backend errors.
func TestActionSpecs_DeleteNoteError(t *testing.T) {
	byTool := snippetDiscussionsSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))))

	_, err := byTool["gitlab_delete_snippet_discussion_note"].Route.Handler(t.Context(), map[string]any{
		"project_id": "42", "snippet_id": 1, "discussion_id": "abc123", "note_id": 10,
	})
	if err == nil {
		t.Fatal("expected error from delete with failing backend")
	}
}

// TestActionSpecs_DeleteOutput verifies the delete route preserves its success message.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	byTool := snippetDiscussionsSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, snippetDiscussionsActionHandler())))

	result, err := byTool["gitlab_delete_snippet_discussion_note"].Route.Handler(t.Context(), map[string]any{
		"project_id": "1", "snippet_id": 5, "discussion_id": "d1", "note_id": 1,
	})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_delete_snippet_discussion_note) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_delete_snippet_discussion_note) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted snippet discussion note." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
	byTool := snippetDiscussionsSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_delete_snippet_discussion_note"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test snippet discussion destructive confirmation.",
		Icons:       toolutil.IconDiscussion,
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
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "gitlab_delete_snippet_discussion_note",
		Arguments: map[string]any{
			"project_id": "1", "snippet_id": 5, "discussion_id": "d1", "note_id": 1,
		},
	})
	if err != nil {
		t.Fatalf("CallTool returned transport error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result when confirmation is declined")
	}
}

func snippetDiscussionsActionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/notes"):
			testutil.RespondJSON(w, http.StatusCreated, covNoteJSON)
		case r.Method == http.MethodPost:
			testutil.RespondJSON(w, http.StatusCreated, covDiscussionJSON)
		case r.Method == http.MethodPut:
			testutil.RespondJSON(w, http.StatusOK, covNoteJSON)
		case strings.Contains(r.URL.Path, "/discussions/d1"):
			testutil.RespondJSON(w, http.StatusOK, covDiscussionJSON)
		default:
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[`+covDiscussionJSON+`]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
		}
	})
}

func snippetDiscussionsSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
