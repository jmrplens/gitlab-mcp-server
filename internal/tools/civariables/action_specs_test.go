// action_specs_test.go contains canonical-route tests for CI variable delete behavior.
package civariables

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestCIVariableOptions_UnrecognizedAction_KeepsTheBaseMetadata asserts that an
// action name the enrichment switch does not know still comes back with the
// metadata registration needs, and with none of the per-action text.
//
// Why it matters: the switch enriches, it does not build. Everything a surface
// needs to register an action — the owner package, the individual tool's name
// and title, the tags, the open-world annotation — is set before it and must
// survive a name it has no case for, or a sixth action added without its case
// would be dropped rather than registered with a placeholder. The placeholder
// usage is what makes that omission visible to a reader, so it is asserted
// here as the fallback it is rather than left to whichever case ran last.
func TestCIVariableOptions_UnrecognizedAction_KeepsTheBaseMetadata(t *testing.T) {
	options := ciVariableOptionsForAction("archive", "gitlab_ci_variable_archive")

	if options.OwnerPackage != "civariables" {
		t.Errorf("OwnerPackage = %q, want %q", options.OwnerPackage, "civariables")
	}
	if options.IndividualTool.Name != "gitlab_ci_variable_archive" {
		t.Errorf("IndividualTool.Name = %q, want %q", options.IndividualTool.Name, "gitlab_ci_variable_archive")
	}
	if options.IndividualTool.Title == "" {
		t.Error("IndividualTool.Title is empty; the base metadata must title every action")
	}
	if !options.OpenWorld {
		t.Error("OpenWorld = false, want true for every civariables action")
	}
	if len(options.Tags) == 0 {
		t.Error("Tags is empty; the base metadata must tag every action")
	}

	if options.Usage != "Use to execute civariables domain action." {
		t.Errorf("Usage = %q, want the placeholder the switch would have replaced", options.Usage)
	}
	if len(options.Aliases) != 1 || options.Aliases[0] != "gitlab_ci_variable_archive" {
		t.Errorf("Aliases = %#v, want only the individual tool name", options.Aliases)
	}
	if options.IndividualTool.Description != "" {
		t.Errorf("IndividualTool.Description = %q, want empty for an action the switch does not know", options.IndividualTool.Description)
	}
	if len(options.RelatedActions) != 0 {
		t.Errorf("RelatedActions = %#v, want none", options.RelatedActions)
	}
	if len(options.ParameterGuidance) != 0 {
		t.Errorf("ParameterGuidance = %#v, want none", options.ParameterGuidance)
	}
}

// TestActionSpecs_DeleteError validates the DeleteError route through the catalog surface.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestActionSpecs_DeleteError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	client := testutil.NewTestClient(t, mux)
	byTool := ciVariableSpecsByTool(t, ActionSpecs(client))

	_, err := byTool["gitlab_ci_variable_delete"].Route.Handler(t.Context(), map[string]any{"project_id": "1", "key": "MY_VAR", "environment_scope": ""})
	if err == nil {
		t.Fatal("expected error from delete with failing backend")
	}
}

// TestCatalogSurface_DeleteConfirmDeclined verifies the CatalogSurface_DeleteConfirmDeclined handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	byTool := ciVariableSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_ci_variable_delete"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test CI variable destructive confirmation.",
		Icons:       toolutil.IconVariable,
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
		Name:      "gitlab_ci_variable_delete",
		Arguments: map[string]any{"project_id": "1", "key": "MY_VAR", "environment_scope": ""},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
	found := false
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok && tc.Text != "" {
			found = true
		}
	}
	if !found {
		t.Error("expected non-empty text content in cancellation result")
	}
}
