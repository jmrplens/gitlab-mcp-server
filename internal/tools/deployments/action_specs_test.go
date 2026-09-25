// action_specs_test.go contains route and catalog-surface tests for behavior that
// used to live in register.go: mutation errors, not-found output, and destructive confirmation.
package deployments

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestActionSpecs_MutationErrors asserts how each route answers an instance
// that refuses it: the four writes report the error, and the read alone
// answers a 404 with the not-found card, which is the one route that turns a
// refusal into a result rather than an error.
func TestActionSpecs_MutationErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
		default:
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
		}
	})
	client := testutil.NewTestClient(t, mux)
	byTool := deploymentSpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name        string
		args        map[string]any
		expectError bool
	}{
		// JSON numbers of eight digits, which reach the route as float64s:
		// %v named them 3.1234567e+07 and 1.2345678e+07.
		{"gitlab_deployment_get", map[string]any{"project_id": float64(12345678), "deployment_id": float64(31234567)}, false},
		{"gitlab_deployment_create", map[string]any{"project_id": "42", "environment": "prod", "ref": "main", "sha": "abc123", "tag": false, "status": "created"}, true},
		{"gitlab_deployment_update", map[string]any{"project_id": "42", "deployment_id": 1, "status": "failed"}, true},
		{"gitlab_deployment_delete", map[string]any{"project_id": "42", "deployment_id": 1}, true},
		{"gitlab_deployment_approve_or_reject", map[string]any{"project_id": "42", "deployment_id": 1, "status": "approved"}, true},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := byTool[tt.name].Route.Handler(t.Context(), tt.args)
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error from %s", tt.name)
				}
				return
			}
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.name, err)
			}
			if notFound, ok := result.(deploymentNotFoundOutput); !ok || notFound.Identifier != "ID 31234567 in project 12345678" {
				t.Fatalf("result = %#v, want deploymentNotFoundOutput naming ID 31234567 in project 12345678", result)
			}
		})
	}
}

// TestCatalogSurface_DeleteConfirmDeclined drives the delete over a real MCP
// session whose client declines the confirmation, and asserts the call is
// answered rather than left hanging. The mock GitLab is an empty mux on
// purpose: a declined confirmation must reach no endpoint at all.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	for _, spec := range ActionSpecs(client) {
		if spec.IndividualTool.Name == "gitlab_deployment_delete" {
			toolutil.RegisterSurfaceToolFromSpec(server, spec, toolutil.SurfaceToolRegisterOptions{Description: "Test deployment destructive confirmation.", Icons: toolutil.IconDeploy})
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
	session, connectErr := mcpClient.Connect(ctx, ct, nil)
	if connectErr != nil {
		t.Fatalf("client connect: %v", connectErr)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_deployment_delete",
		Arguments: map[string]any{"project_id": "42", "deployment_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}
