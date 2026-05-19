// action_specs_test.go contains canonical-route tests for pipeline trigger actions.
package pipelinetriggers

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const triggerActionJSON = `{"id":10,"description":"deploy","token":"abc123","owner":{"id":1,"name":"Admin"},"created_at":"2026-01-01T00:00:00Z"}`

const triggerActionPipelineJSON = `{"id":99,"sha":"abc","ref":"main","status":"created","web_url":"https://gl/p/1/-/pipelines/99","created_at":"2026-01-01T00:00:00Z"}`

// TestActionSpecs_CallAllRoutes exercises every pipeline trigger tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := pipelineTriggersSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, pipelineTriggersActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_pipeline_trigger_list", map[string]any{"project_id": "1"}},
		{"gitlab_pipeline_trigger_get", map[string]any{"project_id": "1", "trigger_id": 10}},
		{"gitlab_pipeline_trigger_create", map[string]any{"project_id": "1", "description": "test trigger"}},
		{"gitlab_pipeline_trigger_update", map[string]any{"project_id": "1", "trigger_id": 10, "description": "updated"}},
		{"gitlab_pipeline_trigger_delete", map[string]any{"project_id": "1", "trigger_id": 10}},
		{"gitlab_pipeline_trigger_run", map[string]any{"project_id": "1", "ref": "main", "token": "tok123"}},
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

// TestActionSpecs_DeleteError verifies that the delete route propagates backend errors.
func TestActionSpecs_DeleteError(t *testing.T) {
	byTool := pipelineTriggersSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))))

	_, err := byTool["gitlab_pipeline_trigger_delete"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "trigger_id": 1})
	if err == nil {
		t.Fatal("expected error from delete with failing backend")
	}
}

// TestActionSpecs_DeleteOutput verifies the delete route preserves its success message.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	byTool := pipelineTriggersSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, pipelineTriggersActionHandler())))

	result, err := byTool["gitlab_pipeline_trigger_delete"].Route.Handler(t.Context(), map[string]any{"project_id": "1", "trigger_id": 10})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_pipeline_trigger_delete) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_pipeline_trigger_delete) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted pipeline trigger." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
	byTool := pipelineTriggersSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_pipeline_trigger_delete"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test pipeline trigger destructive confirmation.",
		Icons:       toolutil.IconPipeline,
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
		Name:      "gitlab_pipeline_trigger_delete",
		Arguments: map[string]any{"project_id": "42", "trigger_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool returned transport error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result when confirmation is declined")
	}
}

func pipelineTriggersActionHandler() http.Handler {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/projects/1/triggers", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+triggerActionJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/projects/1/triggers/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, triggerActionJSON)
	})
	handler.HandleFunc("POST /api/v4/projects/1/triggers", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, triggerActionJSON)
	})
	handler.HandleFunc("PUT /api/v4/projects/1/triggers/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, triggerActionJSON)
	})
	handler.HandleFunc("DELETE /api/v4/projects/1/triggers/10", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("POST /api/v4/projects/1/trigger/pipeline", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, triggerActionPipelineJSON)
	})
	return handler
}

func pipelineTriggersSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
