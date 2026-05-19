// action_specs_test.go contains canonical-route tests for epic discussion actions.
package epicdiscussions

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_DeleteNoteError verifies the delete note route returns an error when GraphQL rejects it.
func TestActionSpecs_DeleteNoteError(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"destroyNote": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{"destroyNote":{"errors":["server error"]}}`)
		},
	})
	byTool := epicDiscussionSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, handler)))

	_, err := byTool["gitlab_delete_epic_discussion_note"].Route.Handler(t.Context(), map[string]any{
		"full_path": testFullPath,
		"epic_iid":  1,
		"note_id":   10,
	})
	if err == nil {
		t.Fatal("expected error from delete with failing backend")
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	byTool := epicDiscussionSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_delete_epic_discussion_note"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test epic discussion note destructive confirmation.",
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
		Name: "gitlab_delete_epic_discussion_note",
		Arguments: map[string]any{
			"full_path": testFullPath,
			"epic_iid":  float64(5),
			"note_id":   float64(100),
		},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

func epicDiscussionSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
