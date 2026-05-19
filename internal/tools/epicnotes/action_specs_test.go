// action_specs_test.go contains canonical-route tests for epic note actions.
package epicnotes

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_CallAllRoutes invokes every individual tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := epicNoteSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, graphqlSessionMux())))
	tests := []struct {
		name string
		tool string
		args map[string]any
		want string
	}{
		{"list", "gitlab_epic_note_list", map[string]any{"full_path": testFullPath, "epic_iid": float64(1)}, ""},
		{"get", "gitlab_epic_note_get", map[string]any{"full_path": testFullPath, "epic_iid": float64(1), "note_id": float64(100)}, ""},
		{"create", "gitlab_epic_note_create", map[string]any{"full_path": testFullPath, "epic_iid": float64(1), "body": "comment"}, ""},
		{"update", "gitlab_epic_note_update", map[string]any{"full_path": testFullPath, "epic_iid": float64(1), "note_id": float64(100), "body": "updated"}, ""},
		{"delete", "gitlab_epic_note_delete", map[string]any{"full_path": testFullPath, "epic_iid": float64(1), "note_id": float64(100)}, "Successfully deleted note 100 from epic &1 in group my-group."},
	}

	for _, tt := range tests {
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

// TestActionSpecs_DeleteError verifies the delete route returns an error when GraphQL rejects it.
func TestActionSpecs_DeleteError(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"destroyNote": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{"destroyNote":{"errors":["server error"]}}`)
		},
	})
	byTool := epicNoteSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, handler)))

	_, err := byTool["gitlab_epic_note_delete"].Route.Handler(t.Context(), map[string]any{
		"full_path": testFullPath,
		"epic_iid":  float64(1),
		"note_id":   float64(100),
	})
	if err == nil {
		t.Fatal("expected error from delete with failing backend")
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	byTool := epicNoteSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_epic_note_delete"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test epic note destructive confirmation.",
		Icons:       toolutil.IconEpic,
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
		Name: "gitlab_epic_note_delete",
		Arguments: map[string]any{
			"full_path": testFullPath,
			"epic_iid":  float64(1),
			"note_id":   float64(100),
		},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			if textContent.Text == "" {
				t.Error("expected non-empty cancellation message")
			}
			return
		}
	}
	t.Error("expected text content in cancellation result")
}

func graphqlSessionMux() http.Handler {
	return graphqlMux(map[string]http.HandlerFunc{
		"WorkItemWidgetNotes": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, gqlNotesData)
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

func epicNoteSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
