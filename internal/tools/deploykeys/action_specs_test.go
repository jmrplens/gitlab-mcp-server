// action_specs_test.go contains route and catalog-surface tests for behavior that
// used to live in register.go: mutation error paths and destructive confirmation.
package deploykeys

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_DeleteError verifies that the delete route returns an error
// when the GitLab API fails.
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
	byTool := deployKeySpecsByTool(t, ActionSpecs(client))

	_, err := byTool["gitlab_deploy_key_delete"].Route.Handler(t.Context(), map[string]any{
		"project_id":    "42",
		"deploy_key_id": float64(1),
	})
	if err == nil {
		t.Fatal("expected error from delete with failing backend")
	}
}

// TestActionSpecs_DeployKeyIDGuidance verifies deploy-key actions warn against
// confusing deploy key IDs with deploy token IDs.
func TestActionSpecs_DeployKeyIDGuidance(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	byTool := deployKeySpecsByTool(t, ActionSpecs(client))

	listSpec := byTool["gitlab_deploy_key_list_project"]
	if !strings.Contains(listSpec.Usage, "SSH deploy keys") || !deployKeyContainsText(listSpec.Aliases, "project deploy keys") {
		t.Fatalf("gitlab_deploy_key_list_project metadata = usage %q aliases %v, want project deploy key guidance", listSpec.Usage, listSpec.Aliases)
	}

	for _, toolName := range []string{"gitlab_deploy_key_get", "gitlab_deploy_key_update", "gitlab_deploy_key_delete", "gitlab_deploy_key_enable"} {
		spec := byTool[toolName]
		if !strings.Contains(spec.Usage, "deploy_key_id") || !strings.Contains(spec.Usage, "deploy_token_id") {
			t.Fatalf("%s Usage = %q, want deploy_key_id vs deploy_token_id guidance", toolName, spec.Usage)
		}
		guidance := spec.ParameterGuidance["deploy_key_id"]
		if guidance.SemanticRole != "deploy_key" || !deployKeyContainsText(guidance.CommonConfusions, "deploy_token_id") {
			t.Fatalf("%s deploy_key_id guidance = %+v, want deploy_token_id warning", toolName, guidance)
		}
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers generic destructive
// confirmation for deploy key delete when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	for _, spec := range ActionSpecs(client) {
		if spec.IndividualTool.Name == "gitlab_deploy_key_delete" {
			toolutil.RegisterSurfaceToolFromSpec(server, spec, toolutil.SurfaceToolRegisterOptions{Description: "Test deploy key destructive confirmation.", Icons: toolutil.IconKey})
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
		Name:      "gitlab_deploy_key_delete",
		Arguments: map[string]any{"project_id": "42", "deploy_key_id": float64(1)},
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

func deployKeyContainsText(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
