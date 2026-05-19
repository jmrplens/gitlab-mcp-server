// action_specs_test.go contains canonical-route tests for group member actions.
package groupmembers

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_CallAllRoutes exercises every group member tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	memberJSON := `{"id":10,"username":"dev","name":"Developer","state":"active","access_level":30}`
	groupJSON := `{"id":5,"name":"MyGroup","path":"mygroup","web_url":"https://gl/groups/mygroup"}`
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/groups/5/members/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, memberJSON)
	})
	handler.HandleFunc("GET /api/v4/groups/5/members/all/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, memberJSON)
	})
	handler.HandleFunc("POST /api/v4/groups/5/members", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":20,"username":"newuser","name":"New User","state":"active","access_level":30}`)
	})
	handler.HandleFunc("PUT /api/v4/groups/5/members/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":10,"username":"dev","name":"Developer","state":"active","access_level":40}`)
	})
	handler.HandleFunc("DELETE /api/v4/groups/5/members/10", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("POST /api/v4/groups/5/share", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, groupJSON)
	})
	handler.HandleFunc("DELETE /api/v4/groups/5/share/10", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	byTool := groupMemberSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, handler)))

	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"get", "gitlab_group_member_get", map[string]any{"group_id": "5", "user_id": 10}},
		{"get_inherited", "gitlab_group_member_get_inherited", map[string]any{"group_id": "5", "user_id": 10}},
		{"add", "gitlab_group_member_add", map[string]any{"group_id": "5", "user_id": 20, "access_level": 30}},
		{"edit", "gitlab_group_member_edit", map[string]any{"group_id": "5", "user_id": 10, "access_level": 40}},
		{"remove", "gitlab_group_member_remove", map[string]any{"group_id": "5", "user_id": 10}},
		{"share", "gitlab_group_share", map[string]any{"group_id": "5", "share_group_id": 10, "group_access": 30}},
		{"unshare", "gitlab_group_unshare", map[string]any{"group_id": "5", "share_group_id": 10}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

// TestActionSpecs_DeleteErrors verifies destructive routes propagate backend errors.
func TestActionSpecs_DeleteErrors(t *testing.T) {
	byTool := groupMemberSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_group_member_remove", map[string]any{"group_id": "42", "user_id": 1}},
		{"gitlab_group_unshare", map[string]any{"group_id": "42", "share_group_id": 99}},
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

// TestActionSpecs_DeleteOutputs verifies destructive routes preserve their success messages.
func TestActionSpecs_DeleteOutputs(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("DELETE /api/v4/groups/5/members/10", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("DELETE /api/v4/groups/5/share/10", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	byTool := groupMemberSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, handler)))

	tests := []struct {
		tool    string
		args    map[string]any
		message string
	}{
		{"gitlab_group_member_remove", map[string]any{"group_id": "5", "user_id": 10}, "Successfully deleted group member."},
		{"gitlab_group_unshare", map[string]any{"group_id": "5", "share_group_id": 10}, "Successfully deleted group share."},
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
			if out.Message != tt.message {
				t.Fatalf("delete message = %q", out.Message)
			}
		})
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
	byTool := groupMemberSpecsByTool(t, ActionSpecs(client))

	for _, tt := range []struct {
		name string
		args map[string]any
	}{
		{"gitlab_group_member_remove", map[string]any{"group_id": "42", "user_id": 1}},
		{"gitlab_group_unshare", map[string]any{"group_id": "42", "share_group_id": 99}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
			toolutil.RegisterSurfaceToolFromSpec(server, byTool[tt.name], toolutil.SurfaceToolRegisterOptions{
				Description: "Test group member destructive confirmation.",
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
			session, err := mcpClient.Connect(ctx, ct, nil)
			if err != nil {
				t.Fatalf("client connect: %v", err)
			}
			t.Cleanup(func() {
				session.Close()
				_ = serverSession.Wait()
			})

			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool returned transport error: %v", err)
			}
			if result == nil {
				t.Fatal("expected non-nil result when confirmation is declined")
			}
		})
	}
}

func groupMemberSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
