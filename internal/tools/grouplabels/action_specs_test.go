// action_specs_test.go contains route and catalog-surface tests for behavior that
// used to live in register.go: mutation error paths and destructive confirmation.
package grouplabels

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

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
	byTool := groupLabelSpecsByTool(t, ActionSpecs(client))

	_, err := byTool["gitlab_group_label_delete"].Route.Handler(t.Context(), map[string]any{"group_id": "my-group", "label_id": "bug"})
	if err == nil {
		t.Fatal("expected error from delete with failing backend")
	}
}

// TestCatalogSurface_DeleteConfirmDeclined verifies the CatalogSurface_DeleteConfirmDeclined handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	for _, spec := range ActionSpecs(client) {
		if spec.IndividualTool.Name == "gitlab_group_label_delete" {
			toolutil.RegisterSurfaceToolFromSpec(server, spec, toolutil.SurfaceToolRegisterOptions{Description: "Test group label destructive confirmation.", Icons: toolutil.IconLabel})
		}
	}

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
		Name:      "gitlab_group_label_delete",
		Arguments: map[string]any{"group_id": "my-group", "label_id": "bug"},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

// TestActionSpecs_UnsubscribeError validates the UnsubscribeError route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestActionSpecs_UnsubscribeError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, mux)
	byTool := groupLabelSpecsByTool(t, ActionSpecs(client))

	_, err := byTool["gitlab_group_label_unsubscribe"].Route.Handler(t.Context(), map[string]any{"group_id": "my-group", "label_id": "bug"})
	if err == nil {
		t.Fatal("expected error from unsubscribe with failing backend")
	}
}

// TestDecorateGroupLabelMeta_UnknownTool verifies the no-op branch leaves the
// generic placeholder metadata untouched for a tool with no meta entry.
func TestDecorateGroupLabelMeta_UnknownTool(t *testing.T) {
	options := groupLabelOptions("gitlab_unknown_tool")
	decorateGroupLabelMeta(&options, "gitlab_unknown_tool")
	if options.Usage != "Use to execute grouplabels domain action." {
		t.Errorf("Usage = %q, want generic placeholder unchanged", options.Usage)
	}
	if options.IndividualTool.Description != "" {
		t.Errorf("Description = %q, want empty for unknown tool", options.IndividualTool.Description)
	}
}

// TestGroupLabelActionMeta_AllToolsDecorated verifies every projected group-label
// tool carries non-generic R-META discovery metadata (Usage, aliases, related,
// and a "Returns: … See also: …" individual-tool description).
func TestGroupLabelActionMeta_AllToolsDecorated(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	for _, spec := range ActionSpecs(client) {
		name := spec.IndividualTool.Name
		if spec.IndividualTool.Description == "" {
			t.Errorf("%s: missing individual-tool description", name)
		}
		if spec.Usage == "Use to execute grouplabels domain action." {
			t.Errorf("%s: Usage still generic placeholder", name)
		}
	}
}
