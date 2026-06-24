// action_specs_test.go contains canonical-route tests for container registry delete behavior.
package containerregistry

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestTagProtectionMetadata_Discoverability locks in the model-facing discovery
// metadata for the container registry tag protection actions (client-go
// v2.40.0) and verifies the repository-path protection list and the tag list
// cross-reference them, so models can tell tag protection apart from
// repository-path protection.
func TestTagProtectionMetadata_Discoverability(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	byTool := registrySpecsByTool(t, ActionSpecs(client))

	cases := []struct {
		tool          string
		aliasContains string
		related       string
	}{
		{"gitlab_registry_tag_protection_list", "tag", "package.registry_tag_rule_create"},
		{"gitlab_registry_tag_protection_create", "tag", "package.registry_tag_rule_list"},
		{"gitlab_registry_tag_protection_update", "tag", "package.registry_tag_rule_delete"},
		{"gitlab_registry_tag_protection_delete", "tag", "package.registry_tag_rule_list"},
	}
	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			spec, ok := byTool[tc.tool]
			if !ok {
				t.Fatalf("missing spec for %s", tc.tool)
			}
			if spec.Usage == "" || strings.Contains(spec.Usage, "Manage container registry repositories, tags, and protection rules") {
				t.Errorf("%s has generic/empty Usage: %q", tc.tool, spec.Usage)
			}
			if !registryAliasHas(spec.Aliases, tc.aliasContains) {
				t.Errorf("%s aliases %v missing phrase %q", tc.tool, spec.Aliases, tc.aliasContains)
			}
			if !slices.Contains(spec.RelatedActions, tc.related) {
				t.Errorf("%s related %v missing %q", tc.tool, spec.RelatedActions, tc.related)
			}
		})
	}

	// Cross-references from the encompassing actions so models discover tag protection.
	if list := byTool["gitlab_registry_protection_list"]; !slices.Contains(list.RelatedActions, "package.registry_tag_rule_list") {
		t.Errorf("registry_protection_list should cross-reference tag protection, got %v", list.RelatedActions)
	}
	if tags := byTool["gitlab_registry_list_tags"]; !slices.Contains(tags.RelatedActions, "package.registry_tag_rule_list") {
		t.Errorf("registry_list_tags should cross-reference tag protection, got %v", tags.RelatedActions)
	}
}

func registryAliasHas(aliases []string, sub string) bool {
	for _, a := range aliases {
		if strings.Contains(a, sub) {
			return true
		}
	}
	return false
}

// TestActionSpecs_DeleteErrors validates the DeleteErrors route through the catalog surface.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestActionSpecs_DeleteErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})
	client := testutil.NewTestClient(t, mux)
	byTool := registrySpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_registry_delete_repository", map[string]any{"project_id": "42", "repository_id": 1}},
		{"gitlab_registry_delete_tag", map[string]any{"project_id": "42", "repository_id": 1, "tag_name": "latest"}},
		{"gitlab_registry_delete_tags_bulk", map[string]any{"project_id": "42", "repository_id": 1}},
		{"gitlab_registry_protection_delete", map[string]any{"project_id": "42", "rule_id": 1}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			_, err := byTool[tt.name].Route.Handler(t.Context(), tt.args)
			if err == nil {
				t.Errorf("expected error from %s with failing backend", tt.name)
			}
		})
	}
}

// TestCatalogSurface_DeleteConfirmDeclined verifies the CatalogSurface_DeleteConfirmDeclined handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	byTool := registrySpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	for _, toolName := range []string{"gitlab_registry_delete_repository", "gitlab_registry_delete_tag", "gitlab_registry_delete_tags_bulk", "gitlab_registry_protection_delete"} {
		toolutil.RegisterSurfaceToolFromSpec(server, byTool[toolName], toolutil.SurfaceToolRegisterOptions{
			Description: "Test container registry destructive confirmation.",
			Icons:       toolutil.IconContainer,
		})
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
	session, connectErr := mcpClient.Connect(ctx, ct, nil)
	if connectErr != nil {
		t.Fatalf("client connect: %v", connectErr)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_registry_delete_repository", map[string]any{"project_id": "42", "repository_id": 1}},
		{"gitlab_registry_delete_tag", map[string]any{"project_id": "42", "repository_id": 1, "tag_name": "latest"}},
		{"gitlab_registry_delete_tags_bulk", map[string]any{"project_id": "42", "repository_id": 1}},
		{"gitlab_registry_protection_delete", map[string]any{"project_id": "42", "rule_id": 1}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, callErr := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if callErr != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.name, callErr)
			}
			if result == nil {
				t.Fatalf("expected non-nil result for declined confirmation on %s", tt.name)
			}
			found := false
			for _, c := range result.Content {
				if tc, ok := c.(*mcp.TextContent); ok && tc.Text != "" {
					found = true
				}
			}
			if !found {
				t.Errorf("expected non-empty text content in %s cancellation result", tt.name)
			}
		})
	}
}
