// action_specs_test.go contains canonical-route tests for issue link actions.
package issuelinks

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_CallAllRoutes exercises every issue link tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := issueLinkSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, issueLinksActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_issue_link_list", map[string]any{"project_id": "42", "issue_iid": 10}},
		{"gitlab_issue_link_get", map[string]any{"project_id": "42", "issue_iid": 10, "issue_link_id": 99}},
		{"gitlab_issue_link_create", map[string]any{"project_id": "42", "issue_iid": 10, "target_project_id": "42", "target_issue_iid": "20", "link_type": "relates_to"}},
		{"gitlab_issue_link_delete", map[string]any{"project_id": "42", "issue_iid": 10, "issue_link_id": 99}},
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

// TestActionSpecs_ErrorPaths verifies canonical routes propagate backend errors.
func TestActionSpecs_ErrorPaths(t *testing.T) {
	byTool := issueLinkSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_issue_link_list", map[string]any{"project_id": "1", "issue_iid": 1}},
		{"gitlab_issue_link_get", map[string]any{"project_id": "1", "issue_iid": 1, "issue_link_id": 1}},
		{"gitlab_issue_link_create", map[string]any{"project_id": "1", "issue_iid": 1, "target_project_id": "2", "target_issue_iid": 3}},
		{"gitlab_issue_link_delete", map[string]any{"project_id": "1", "issue_iid": 1, "issue_link_id": 1}},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			_, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err == nil {
				t.Fatal("expected route error")
			}
		})
	}
}

// TestActionSpecs_GetNotFound verifies the get route reports missing links as errors.
func TestActionSpecs_GetNotFound(t *testing.T) {
	byTool := issueLinkSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))))

	_, err := byTool["gitlab_issue_link_get"].Route.Handler(t.Context(), map[string]any{
		"project_id": "42", "issue_iid": 1, "issue_link_id": 999,
	})
	if err == nil {
		t.Fatal("expected route error for missing issue link")
	}
}

// TestActionSpecs_DeleteOutput verifies the delete route preserves its success message.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	byTool := issueLinkSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, issueLinksActionHandler())))

	result, err := byTool["gitlab_issue_link_delete"].Route.Handler(t.Context(), map[string]any{
		"project_id": "42", "issue_iid": 10, "issue_link_id": 99,
	})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_issue_link_delete) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_issue_link_delete) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted issue link." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
	byTool := issueLinkSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_issue_link_delete"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test issue link destructive confirmation.",
		Icons:       toolutil.IconLink,
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
		Name:      "gitlab_issue_link_delete",
		Arguments: map[string]any{"project_id": "42", "issue_iid": 1, "issue_link_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

func issueLinksActionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && path == pathIssueLinks:
			testutil.RespondJSON(w, http.StatusOK, issueRelationJSON)
		case r.Method == http.MethodGet && path == pathIssueLink99:
			testutil.RespondJSON(w, http.StatusOK, issueLinkJSON)
		case r.Method == http.MethodPost && path == pathIssueLinks:
			testutil.RespondJSON(w, http.StatusCreated, issueLinkJSON)
		case r.Method == http.MethodDelete && path == pathIssueLink99:
			testutil.RespondJSON(w, http.StatusOK, issueLinkJSON)
		default:
			http.NotFound(w, r)
		}
	})
}

func issueLinkSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
