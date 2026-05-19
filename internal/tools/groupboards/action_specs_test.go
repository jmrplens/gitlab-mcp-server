// action_specs_test.go contains canonical-route tests for group issue board actions.
package groupboards

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_CallAllRoutes exercises every group board tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	boardJSON := `{"id":1,"name":"Development","group":{"id":42,"name":"mygroup"},"milestone":{"id":5,"title":"v1.0"},"labels":[{"name":"bug"}],"lists":[{"id":10,"label":{"id":20,"name":"To Do"},"position":0}]}`
	boardListJSON := `{"id":10,"label":{"id":20,"name":"To Do"},"position":0,"max_issue_count":10}`
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/groups/42/boards", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+boardJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/groups/42/boards/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, boardJSON)
	})
	handler.HandleFunc("POST /api/v4/groups/42/boards", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, boardJSON)
	})
	handler.HandleFunc("PUT /api/v4/groups/42/boards/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, boardJSON)
	})
	handler.HandleFunc("DELETE /api/v4/groups/42/boards/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("GET /api/v4/groups/42/boards/1/lists", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+boardListJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/groups/42/boards/1/lists/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, boardListJSON)
	})
	handler.HandleFunc("POST /api/v4/groups/42/boards/1/lists", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, boardListJSON)
	})
	handler.HandleFunc("PUT /api/v4/groups/42/boards/1/lists/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":10,"label":{"id":20,"name":"To Do"},"position":2}]`)
	})
	handler.HandleFunc("DELETE /api/v4/groups/42/boards/1/lists/10", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	byTool := groupBoardSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, handler)))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_group_board_list", map[string]any{"group_id": "42"}},
		{"gitlab_group_board_get", map[string]any{"group_id": "42", "board_id": 1}},
		{"gitlab_group_board_create", map[string]any{"group_id": "42", "name": "New Board"}},
		{"gitlab_group_board_update", map[string]any{"group_id": "42", "board_id": 1, "name": "Updated"}},
		{"gitlab_group_board_delete", map[string]any{"group_id": "42", "board_id": 1}},
		{"gitlab_group_board_list_lists", map[string]any{"group_id": "42", "board_id": 1}},
		{"gitlab_group_board_list_get", map[string]any{"group_id": "42", "board_id": 1, "list_id": 10}},
		{"gitlab_group_board_list_create", map[string]any{"group_id": "42", "board_id": 1, "label_id": 5}},
		{"gitlab_group_board_list_update", map[string]any{"group_id": "42", "board_id": 1, "list_id": 10, "position": 2}},
		{"gitlab_group_board_list_delete", map[string]any{"group_id": "42", "board_id": 1, "list_id": 10}},
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

// TestActionSpecs_DeleteErrors verifies destructive routes propagate backend errors.
func TestActionSpecs_DeleteErrors(t *testing.T) {
	byTool := groupBoardSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_group_board_delete", map[string]any{"group_id": "my-group", "board_id": 1}},
		{"gitlab_group_board_list_delete", map[string]any{"group_id": "my-group", "board_id": 1, "list_id": 100}},
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
	handler.HandleFunc("DELETE /api/v4/groups/42/boards/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("DELETE /api/v4/groups/42/boards/1/lists/10", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	byTool := groupBoardSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, handler)))

	tests := []struct {
		tool    string
		args    map[string]any
		message string
	}{
		{"gitlab_group_board_delete", map[string]any{"group_id": "42", "board_id": 1}, "Successfully deleted group board."},
		{"gitlab_group_board_list_delete", map[string]any{"group_id": "42", "board_id": 1, "list_id": 10}, "Successfully deleted group board list."},
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
	byTool := groupBoardSpecsByTool(t, ActionSpecs(client))

	for _, tt := range []struct {
		name string
		args map[string]any
	}{
		{"gitlab_group_board_delete", map[string]any{"group_id": "g", "board_id": 1}},
		{"gitlab_group_board_list_delete", map[string]any{"group_id": "g", "board_id": 1, "list_id": 1}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
			toolutil.RegisterSurfaceToolFromSpec(server, byTool[tt.name], toolutil.SurfaceToolRegisterOptions{
				Description: "Test group board destructive confirmation.",
				Icons:       toolutil.IconBoard,
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

func groupBoardSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
