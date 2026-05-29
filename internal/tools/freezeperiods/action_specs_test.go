// action_specs_test.go contains canonical-route tests for deploy freeze period actions.
package freezeperiods

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_Metadata verifies canonical metadata for freeze period actions.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	byTool := freezePeriodSpecsByTool(t, specs)

	if len(specs) != 5 {
		t.Fatalf("len(ActionSpecs) = %d, want 5", len(specs))
	}
	if len(byTool) != len(specs) {
		t.Fatalf("unique individual tools = %d, want %d", len(byTool), len(specs))
	}
	if !byTool["gitlab_delete_freeze_period"].Route.Destructive {
		t.Fatal("gitlab_delete_freeze_period should be destructive")
	}
	if byTool["gitlab_list_freeze_periods"].Usage == "" {
		t.Fatal("gitlab_list_freeze_periods should define usage")
	}
	if len(byTool["gitlab_get_freeze_period"].Aliases) == 0 {
		t.Fatal("gitlab_get_freeze_period should define aliases")
	}
	if byTool["gitlab_update_freeze_period"].ParameterGuidance["freeze_period_id"].SemanticRole == "" {
		t.Fatal("gitlab_update_freeze_period should define freeze_period_id parameter guidance")
	}
}

// TestActionSpecs_CallAllRoutes exercises every deploy freeze period tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/projects/1/freeze_periods", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"freeze_start":"0 23 * * 5","freeze_end":"0 7 * * 1","cron_timezone":"UTC"}]`)
	})
	handler.HandleFunc("GET /api/v4/projects/1/freeze_periods/5", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":5,"freeze_start":"0 23 * * 5","freeze_end":"0 7 * * 1","cron_timezone":"UTC"}`)
	})
	handler.HandleFunc("POST /api/v4/projects/1/freeze_periods", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":2,"freeze_start":"0 23 * * 5","freeze_end":"0 7 * * 1","cron_timezone":"UTC"}`)
	})
	handler.HandleFunc("PUT /api/v4/projects/1/freeze_periods/5", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":5,"freeze_start":"0 0 * * 1","freeze_end":"0 7 * * 1","cron_timezone":"UTC"}`)
	})
	handler.HandleFunc("DELETE /api/v4/projects/1/freeze_periods/5", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	byTool := freezePeriodSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, handler)))

	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_list_freeze_periods", map[string]any{"project_id": "1"}},
		{"get", "gitlab_get_freeze_period", map[string]any{"project_id": "1", "freeze_period_id": 5}},
		{"create", "gitlab_create_freeze_period", map[string]any{"project_id": "1", "freeze_start": "0 23 * * 5", "freeze_end": "0 7 * * 1"}},
		{"update", "gitlab_update_freeze_period", map[string]any{"project_id": "1", "freeze_period_id": 5, "freeze_start": "0 0 * * 1"}},
		{"delete", "gitlab_delete_freeze_period", map[string]any{"project_id": "1", "freeze_period_id": 5}},
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

// TestActionSpecs_DeleteError verifies that the delete route propagates backend errors.
func TestActionSpecs_DeleteError(t *testing.T) {
	byTool := freezePeriodSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))))

	_, err := byTool["gitlab_delete_freeze_period"].Route.Handler(t.Context(), map[string]any{
		"project_id":       "42",
		"freeze_period_id": 1,
	})
	if err == nil {
		t.Fatal("expected error from delete with failing backend")
	}
}

// TestActionSpecs_DeleteOutput verifies the delete route preserves its success message.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/freeze_periods/5" || r.Method != http.MethodDelete {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	byTool := freezePeriodSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_delete_freeze_period"].Route.Handler(t.Context(), map[string]any{
		"project_id":       "1",
		"freeze_period_id": 5,
	})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_delete_freeze_period) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_delete_freeze_period) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted freeze period." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
	byTool := freezePeriodSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_delete_freeze_period"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test freeze period destructive confirmation.",
		Icons:       toolutil.IconSchedule,
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
		Name: "gitlab_delete_freeze_period",
		Arguments: map[string]any{
			"project_id":       "1",
			"freeze_period_id": float64(5),
		},
	})
	if err != nil {
		t.Fatalf("CallTool returned transport error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result when confirmation is declined")
	}
	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			if textContent.Text == "" {
				t.Error("expected non-empty cancellation message")
			}
			return
		}
	}
	t.Error("expected text content in cancellation result")
}

func freezePeriodSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
