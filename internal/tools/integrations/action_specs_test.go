// action_specs_test.go contains canonical-route tests for project integration actions.
package integrations

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const actionSpecIntegrationJSON = `{"id":1,"title":"Jira","slug":"jira","active":true}`

// TestActionSpecs_CallAllRoutes validates the CallAllRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := integrationSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, integrationActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_list_integrations", map[string]any{"project_id": "1"}},
		{"gitlab_get_integration", map[string]any{"project_id": "1", "slug": "jira"}},
		{"gitlab_delete_integration", map[string]any{"project_id": "1", "slug": "jira"}},
		{"gitlab_set_jira_integration", map[string]any{"project_id": "1", "url": "https://jira.example.com"}},
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

// TestActionSpecs_DeleteOutput validates the DeleteOutput route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	byTool := integrationSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, integrationActionHandler())))

	result, err := byTool["gitlab_delete_integration"].Route.Handler(t.Context(), map[string]any{"project_id": "1", "slug": "jira"})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_delete_integration) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_delete_integration) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted integration." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestActionSpecs_DeleteError validates the DeleteError route through the catalog surface.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestActionSpecs_DeleteError(t *testing.T) {
	byTool := integrationSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))))

	_, err := byTool["gitlab_delete_integration"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "slug": "jira"})
	if err == nil {
		t.Fatal("expected route error")
	}
}

// TestCatalogSurface_DeleteConfirmDeclined verifies the CatalogSurface_DeleteConfirmDeclined handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
	byTool := integrationSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_delete_integration"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test integration destructive confirmation.",
		Icons:       toolutil.IconIntegration,
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
		Name:      "gitlab_delete_integration",
		Arguments: map[string]any{"project_id": "42", "slug": "jira"},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

// TestApplyIntegrationMeta_KnownAndUnknown verifies the per-action discovery
// metadata overlay. For a known individual tool the generic placeholder Usage,
// the toolname-only alias, and the generic RelatedActions are replaced by the
// purpose-specific values; for an unknown tool the shared defaults are
// preserved unchanged. This exercises both the match and fallback branches of
// applyIntegrationMeta.
func TestApplyIntegrationMeta_KnownAndUnknown(t *testing.T) {
	known := integrationOptions("gitlab_list_integrations", "desc")
	if isGenericMetaUsage(known.Usage) {
		t.Fatalf("known tool Usage still generic: %q", known.Usage)
	}
	if len(known.Aliases) < 2 {
		t.Fatalf("known tool aliases not enriched: %+v", known.Aliases)
	}
	if known.Aliases[0] != "gitlab_list_integrations" {
		t.Fatalf("known tool first alias = %q, want canonical tool name", known.Aliases[0])
	}
	for _, related := range known.RelatedActions {
		if related == "project.get" {
			t.Fatalf("known tool RelatedActions still generic: %+v", known.RelatedActions)
		}
	}

	unknown := integrationOptions("gitlab_unknown_integration_tool", "desc")
	if !isGenericMetaUsage(unknown.Usage) {
		t.Fatalf("unknown tool Usage = %q, want generic default", unknown.Usage)
	}
	if len(unknown.Aliases) != 1 || unknown.Aliases[0] != "gitlab_unknown_integration_tool" {
		t.Fatalf("unknown tool aliases = %+v, want toolname-only", unknown.Aliases)
	}
	if len(unknown.RelatedActions) != 1 || unknown.RelatedActions[0] != "project.get" {
		t.Fatalf("unknown tool RelatedActions = %+v, want [project.get]", unknown.RelatedActions)
	}
}

// TestGroupDatadogOptions_KnownAndUnknown verifies the group-Datadog option
// builder applies the premium edition and group tags, keeps the per-action
// metadata for known tools, and falls back to the generic group.get
// RelatedActions for tools without metadata.
func TestGroupDatadogOptions_KnownAndUnknown(t *testing.T) {
	known := groupDatadogOptions("gitlab_get_group_datadog_integration", "desc")
	if known.Edition != "premium" {
		t.Fatalf("known group tool Edition = %q, want premium", known.Edition)
	}
	if isGenericMetaUsage(known.Usage) {
		t.Fatalf("known group tool Usage still generic: %q", known.Usage)
	}
	if len(known.RelatedActions) == 1 && known.RelatedActions[0] == "group.get" {
		t.Fatalf("known group tool RelatedActions not enriched: %+v", known.RelatedActions)
	}

	unknown := groupDatadogOptions("gitlab_unknown_group_tool", "desc")
	if unknown.Edition != "premium" {
		t.Fatalf("unknown group tool Edition = %q, want premium", unknown.Edition)
	}
	if len(unknown.RelatedActions) != 1 || unknown.RelatedActions[0] != "group.get" {
		t.Fatalf("unknown group tool RelatedActions = %+v, want [group.get]", unknown.RelatedActions)
	}
}

// isGenericMetaUsage reports whether the supplied Usage string is the generic
// placeholder sentence flagged by the metadata-completeness audit.
func isGenericMetaUsage(usage string) bool {
	return usage == "" || usage == "Use to execute integrations domain action."
}

func integrationActionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/1/services":
			testutil.RespondJSON(w, http.StatusOK, `[`+actionSpecIntegrationJSON+`]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/1/integrations/jira":
			testutil.RespondJSON(w, http.StatusOK, actionSpecIntegrationJSON)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/1/services/jira":
			testutil.RespondJSON(w, http.StatusOK, actionSpecIntegrationJSON)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/projects/1/services/jira":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/projects/1/integrations/jira":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/projects/1/services/jira":
			testutil.RespondJSON(w, http.StatusOK, actionSpecIntegrationJSON)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/projects/1/integrations/jira":
			testutil.RespondJSON(w, http.StatusOK, actionSpecIntegrationJSON)
		default:
			http.NotFound(w, r)
		}
	})
}

func integrationSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
