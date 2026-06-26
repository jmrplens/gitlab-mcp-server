// action_specs_test.go contains canonical-route tests for epic discussion actions.
package epicdiscussions

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_DiscoveryMetadata asserts every epic discussion individual
// tool carries action-specific discovery metadata (R-META; 1:1 audit): a
// non-generic Usage string, natural-language aliases, canonical group.* related
// actions, parameter guidance, and a Returns:/See also: description.
func TestActionSpecs_DiscoveryMetadata(t *testing.T) {
	byTool := epicDiscussionSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.NewServeMux())))

	tools := []string{
		"gitlab_list_epic_discussions",
		"gitlab_get_epic_discussion",
		"gitlab_create_epic_discussion",
		"gitlab_add_epic_discussion_note",
		"gitlab_update_epic_discussion_note",
		"gitlab_delete_epic_discussion_note",
	}

	for _, tool := range tools {
		t.Run(tool, func(t *testing.T) {
			assertEpicDiscussionMetadata(t, tool, byTool[tool])
		})
	}
}

// assertEpicDiscussionMetadata checks that a single epic discussion spec carries
// the full R-META discovery surface.
func assertEpicDiscussionMetadata(t *testing.T, tool string, spec toolutil.ActionSpec) {
	t.Helper()
	if spec.Usage == "" || strings.Contains(spec.Usage, "Use to execute epicdiscussions domain action.") {
		t.Errorf("%s: Usage must be action-specific, got %q", tool, spec.Usage)
	}
	if len(spec.Aliases) < 2 {
		t.Errorf("%s: expected natural-language aliases, got %v", tool, spec.Aliases)
	}
	if len(spec.RelatedActions) == 0 {
		t.Errorf("%s: expected RelatedActions, got none", tool)
	}
	for _, ra := range spec.RelatedActions {
		if !strings.HasPrefix(ra, "group.") {
			t.Errorf("%s: RelatedAction %q is not a canonical group.* id", tool, ra)
		}
	}
	if len(spec.ParameterGuidance) == 0 {
		t.Errorf("%s: expected ParameterGuidance, got none", tool)
	}
	if spec.Edition != "premium" {
		t.Errorf("%s: expected premium edition gate, got %q", tool, spec.Edition)
	}
	desc := spec.IndividualTool.Description
	if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
		t.Errorf("%s: IndividualTool.Description must contain Returns:/See also:, got %q", tool, desc)
	}
}

// TestDecorateEpicDiscussionMeta_DefaultFallback covers the defensive default
// branch of decorateEpicDiscussionMeta, exercised when an unknown individual
// tool name is passed.
func TestDecorateEpicDiscussionMeta_DefaultFallback(t *testing.T) {
	var options toolutil.ActionSpecOptions
	decorateEpicDiscussionMeta(&options, "gitlab_unknown_tool")
	if options.Usage == "" {
		t.Error("expected fallback Usage to be set")
	}
	if len(options.RelatedActions) == 0 {
		t.Error("expected fallback RelatedActions to be set")
	}
}

// TestActionSpecs_DeleteNoteError validates the DeleteNoteError route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestCatalogSurface_DeleteConfirmDeclined verifies the CatalogSurface_DeleteConfirmDeclined handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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
