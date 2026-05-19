// action_specs_test.go contains canonical-route tests for MR approval actions.
package mrapprovals

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_CallRoutes exercises MR approval actions through canonical routes.
func TestActionSpecs_CallRoutes(t *testing.T) {
	byTool := approvalSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, approvalActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_mr_approval_state", map[string]any{"project_id": "42", "merge_request_iid": 1}},
		{"gitlab_mr_approval_rules", map[string]any{"project_id": "42", "merge_request_iid": 1}},
		{"gitlab_mr_approval_config", map[string]any{"project_id": "42", "merge_request_iid": 1}},
		{"gitlab_mr_approval_reset", map[string]any{"project_id": "42", "merge_request_iid": 1}},
		{"gitlab_mr_approval_rule_create", map[string]any{"project_id": "42", "merge_request_iid": 1, "name": "R", "approvals_required": 1}},
		{"gitlab_mr_approval_rule_update", map[string]any{"project_id": "42", "merge_request_iid": 1, "approval_rule_id": 5, "name": "U", "approvals_required": 1}},
		{"gitlab_mr_approval_rule_delete", map[string]any{"project_id": "42", "merge_request_iid": 1, "approval_rule_id": 5}},
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

// TestActionSpecs_DeleteOutputs verifies reset and delete routes preserve success messages.
func TestActionSpecs_DeleteOutputs(t *testing.T) {
	byTool := approvalSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, approvalActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
		want string
	}{
		{"gitlab_mr_approval_reset", map[string]any{"project_id": "42", "merge_request_iid": 1}, "Successfully deleted MR approvals."},
		{"gitlab_mr_approval_rule_delete", map[string]any{"project_id": "42", "merge_request_iid": 1, "approval_rule_id": 5}, "Successfully deleted approval rule."},
	}

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			result, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			out, ok := result.(toolutil.DeleteOutput)
			if !ok {
				t.Fatalf("Route.Handler(%s) returned %T, want toolutil.DeleteOutput", tt.tool, result)
			}
			if out.Message != tt.want {
				t.Fatalf("delete message = %q", out.Message)
			}
		})
	}
}

// TestActionSpecs_DeleteErrorsPropagate verifies destructive route backend errors propagate.
func TestActionSpecs_DeleteErrorsPropagate(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete || r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	byTool := approvalSpecsByTool(t, ActionSpecs(client))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_mr_approval_reset", map[string]any{"project_id": "42", "merge_request_iid": 1}},
		{"gitlab_mr_approval_rule_delete", map[string]any{"project_id": "42", "merge_request_iid": 1, "approval_rule_id": 5}},
	}

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			_, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err == nil {
				t.Fatalf("Route.Handler(%s) expected error", tt.tool)
			}
		})
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
	byTool := approvalSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	for _, toolName := range []string{"gitlab_mr_approval_reset", "gitlab_mr_approval_rule_delete"} {
		toolutil.RegisterSurfaceToolFromSpec(server, byTool[toolName], toolutil.SurfaceToolRegisterOptions{
			Description: "Test approval destructive confirmation.",
			Icons:       toolutil.IconMR,
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
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})

	tests := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_mr_approval_reset", map[string]any{"project_id": "42", "merge_request_iid": 1}},
		{"gitlab_mr_approval_rule_delete", map[string]any{"project_id": "42", "merge_request_iid": 1, "approval_rule_id": 5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, callErr := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if callErr != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.name, callErr)
			}
			if result == nil {
				t.Fatalf("expected non-nil result for declined confirmation on %s", tt.name)
			}
		})
	}
}

func approvalActionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/approval_state"):
			testutil.RespondJSON(w, http.StatusOK, `{
				"approval_rules_overwritten": false,
				"rules": [{"id":1,"name":"Default","rule_type":"any_approver","approvals_required":1,"approved":true}]
			}`)
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/approval_rules"):
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"name":"Default","rule_type":"any_approver","approvals_required":1,"approved":true}]`)
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/approval_rules"):
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id":5,"name":"R","rule_type":"regular","approvals_required":1,"approved":false,
				"approved_by":[],"eligible_approvers":[],"users":[],"groups":[]
			}`)
		case r.Method == http.MethodPut && strings.Contains(path, "/approval_rules/"):
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":5,"name":"U","rule_type":"regular","approvals_required":1,"approved":false,
				"approved_by":[],"eligible_approvers":[],"users":[],"groups":[]
			}`)
		case r.Method == http.MethodDelete && strings.Contains(path, "/approval_rules/"):
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/approvals"):
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":1,"iid":1,"project_id":42,"title":"MR","state":"opened",
				"approved":false,"approvals_required":1,"approvals_left":1,
				"approvals_before_merge":0,"has_approval_rules":true,
				"user_has_approved":false,"user_can_approve":true,
				"approved_by":[],"suggested_approvers":[]
			}`)
		case r.Method == http.MethodPut && strings.HasSuffix(path, "/reset_approvals"):
			w.WriteHeader(http.StatusAccepted)
		default:
			http.NotFound(w, r)
		}
	})
}

func approvalSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
