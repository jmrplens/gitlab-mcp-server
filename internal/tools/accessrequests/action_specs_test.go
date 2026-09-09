// action_specs_test.go contains catalog-surface tests for access request actions.
package accessrequests

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestActionSpecs_DenyErrors validates the DenyErrors route through the catalog surface.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestActionSpecs_DenyErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})
	client := testutil.NewTestClient(t, mux)
	byTool := accessRequestSpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_access_request_deny_project", map[string]any{"project_id": "42", "user_id": 1}},
		{"gitlab_access_request_deny_group", map[string]any{"group_id": "5", "user_id": 1}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			_, err := byTool[tt.name].Route.Handler(t.Context(), tt.args)
			if err == nil {
				t.Fatalf("Route.Handler(%s) expected error, got nil", tt.name)
			}
		})
	}
}

// TestCatalogSurface_DenyConfirmDeclined verifies the CatalogSurface_DenyConfirmDeclined handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCatalogSurface_DenyConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	byTool := accessRequestSpecsByTool(t, ActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_access_request_deny_project", map[string]any{"project_id": "42", "user_id": 1}},
		{"gitlab_access_request_deny_group", map[string]any{"group_id": "42", "user_id": 1}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
			toolutil.RegisterSurfaceToolFromSpec(server, byTool[tt.name], toolutil.SurfaceToolRegisterOptions{
				Description: "Test access request destructive confirmation.",
				Icons:       toolutil.IconUser,
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

			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("expected non-nil result for declined confirmation on %s", tt.name)
			}
		})
	}
}

// TestAccessRequestOptionsForAction_UnknownActionKeepsTheBase verifies the
// per-action switch leaves an action it does not name at the base options
// every access-request spec starts from.
//
// It is the other side of the last case: without it nothing ever reaches the
// end of that switch, so a case label could be wrong and no test would say so.
func TestAccessRequestOptionsForAction_UnknownActionKeepsTheBase(t *testing.T) {
	options := accessRequestOptionsForAction("no_such_action", "gitlab_access_request_no_such_action")

	if options.Usage != "Use to execute accessrequests domain action." {
		t.Errorf("Usage = %q, want the base usage", options.Usage)
	}
	if options.IndividualTool.Name != "gitlab_access_request_no_such_action" {
		t.Errorf("IndividualTool.Name = %q, want the name it was given", options.IndividualTool.Name)
	}
	if len(options.Aliases) != 1 || options.Aliases[0] != "gitlab_access_request_no_such_action" {
		t.Errorf("Aliases = %v, want only the tool name", options.Aliases)
	}
	if options.ParameterGuidance != nil {
		t.Errorf("ParameterGuidance = %v, want none for an action the switch does not name", options.ParameterGuidance)
	}
}
