// action_specs_test.go contains canonical-route tests for epic note actions.
package epicnotes

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestActionSpecs_CallAllRoutes validates the CallAllRoutes route through the catalog surface.
// Every epic note action speaks GraphQL, so one mux answers the query and the
// three mutations the five routes send.
// It asserts the route returns the expected error or result.
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

// TestActionSpecs_DeleteError validates the DeleteError route through the catalog surface.
// The destroyNote mutation answers HTTP 200 with a payload error, which is how
// GitLab refuses a deletion.
// It asserts that the route reports that refusal as an error.
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

// TestCatalogSurface_DeleteConfirmDeclined verifies the CatalogSurface_DeleteConfirmDeclined handler.
// The client declines the confirmation, so no request reaches GitLab at all.
// It asserts the caller is answered with a non-empty cancellation message.
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

// TestActionSpecs_EveryActionCarriesItsOwnMetadata pins the five individual
// tool names this package publishes and holds each spec to metadata of its own.
//
// Those five names are what makes decorateEpicNoteMeta's last case
// unfalsifiable, since it is reached only when the four before it missed, which
// is exactly the delete tool. The property worth holding is the one behind
// that: a sixth action added here without a case of its own would ship the
// placeholder usage, an empty description and no related actions, and fails
// here rather than reaching a model.
func TestActionSpecs_EveryActionCarriesItsOwnMetadata(t *testing.T) {
	const placeholderUsage = "Use to execute epicnotes domain action."

	wantTools := []string{
		"gitlab_epic_note_list",
		"gitlab_epic_note_get",
		"gitlab_epic_note_create",
		"gitlab_epic_note_update",
		"gitlab_epic_note_delete",
	}
	byTool := epicNoteSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, testutil.ForbiddenHandler(t))))
	if len(byTool) != len(wantTools) {
		t.Fatalf("ActionSpecs() published %d tools, want %d: %v", len(byTool), len(wantTools), byTool)
	}

	for _, tool := range wantTools {
		t.Run(tool, func(t *testing.T) {
			spec, ok := byTool[tool]
			if !ok {
				t.Fatalf("ActionSpecs() publishes no %s", tool)
			}
			if spec.Usage == "" || spec.Usage == placeholderUsage {
				t.Errorf("Usage = %q, want the action's own text", spec.Usage)
			}
			if spec.IndividualTool.Description == "" {
				t.Error("IndividualTool.Description is empty, want the action's own description")
			}
			if len(spec.RelatedActions) == 0 {
				t.Error("RelatedActions is empty, want the action's own cross-links")
			}
			if len(spec.ParameterGuidance) == 0 {
				t.Error("ParameterGuidance is empty, want guidance for the action's inputs")
			}
		})
	}
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
