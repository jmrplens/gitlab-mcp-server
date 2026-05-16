// action_specs_test.go contains canonical-route tests for issue actions.
package issues

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestActionSpecs_CallRoutes exercises issue actions through their canonical routes.
func TestActionSpecs_CallRoutes(t *testing.T) {
	byTool := issueSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))))
	pid := testProjectID

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_issue_create", map[string]any{"project_id": pid, "title": "Test"}},
		{"gitlab_issue_get", map[string]any{"project_id": pid, "issue_iid": 10}},
		{"gitlab_issue_get_by_id", map[string]any{"issue_id": 10}},
		{"gitlab_issue_list", map[string]any{"project_id": pid}},
		{"gitlab_issue_list_all", map[string]any{}},
		{"gitlab_issue_list_group", map[string]any{"group_id": "99"}},
		{"gitlab_issue_update", map[string]any{"project_id": pid, "issue_iid": 10, "title": "Updated"}},
		{"gitlab_issue_delete", map[string]any{"project_id": pid, "issue_iid": 10}},
		{"gitlab_issue_reorder", map[string]any{"project_id": pid, "issue_iid": 10, "move_after_id": 5}},
		{"gitlab_issue_move", map[string]any{"project_id": pid, "issue_iid": 10, "to_project_id": 99}},
		{"gitlab_issue_subscribe", map[string]any{"project_id": pid, "issue_iid": 10}},
		{"gitlab_issue_unsubscribe", map[string]any{"project_id": pid, "issue_iid": 10}},
		{"gitlab_issue_create_todo", map[string]any{"project_id": pid, "issue_iid": 10}},
		{"gitlab_issue_time_estimate_set", map[string]any{"project_id": pid, "issue_iid": 10, "duration": "3h"}},
		{"gitlab_issue_time_estimate_reset", map[string]any{"project_id": pid, "issue_iid": 10}},
		{"gitlab_issue_spent_time_add", map[string]any{"project_id": pid, "issue_iid": 10, "duration": "1h"}},
		{"gitlab_issue_spent_time_reset", map[string]any{"project_id": pid, "issue_iid": 10}},
		{"gitlab_issue_time_stats_get", map[string]any{"project_id": pid, "issue_iid": 10}},
		{"gitlab_issue_participants", map[string]any{"project_id": pid, "issue_iid": 10}},
		{"gitlab_issue_mrs_closing", map[string]any{"project_id": pid, "issue_iid": 10}},
		{"gitlab_issue_mrs_related", map[string]any{"project_id": pid, "issue_iid": 10}},
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

// TestActionSpecs_DeleteOutput verifies issue delete preserves its success message.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	byTool := issueSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(issueMockHandler))))

	result, err := byTool["gitlab_issue_delete"].Route.Handler(t.Context(), map[string]any{"project_id": testProjectID, "issue_iid": 10})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_issue_delete) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_issue_delete) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted issue #10 from project 42." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestActionSpecs_DeleteErrorPropagates verifies issue delete backend errors propagate.
func TestActionSpecs_DeleteErrorPropagates(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		http.NotFound(w, r)
	}))
	byTool := issueSpecsByTool(t, ActionSpecs(client))

	_, err := byTool["gitlab_issue_delete"].Route.Handler(t.Context(), map[string]any{"project_id": testProjectID, "issue_iid": 10})
	if err == nil {
		t.Fatal("Route.Handler(gitlab_issue_delete) expected error")
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
	byTool := issueSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_issue_delete"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test issue destructive confirmation.",
		Icons:       toolutil.IconIssue,
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

	result, callErr := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_issue_delete",
		Arguments: map[string]any{"project_id": testProjectID, "issue_iid": 10},
	})
	if callErr != nil {
		t.Fatalf("CallTool error: %v", callErr)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

func issueSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
