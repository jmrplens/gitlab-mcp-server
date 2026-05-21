// action_specs_test.go contains canonical-route tests for pipeline schedule actions.
package pipelineschedules

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionSpecScheduleJSON = `{"id":1,"description":"Nightly","ref":"main","cron":"0 1 * * *","cron_timezone":"UTC","active":true,"owner":{"username":"admin"}}`
	actionSpecVariableJSON = `{"key":"K","value":"V","variable_type":"env_var"}`
)

// TestActionSpecs_CallAllRoutes exercises every pipeline schedule tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := pipelineScheduleSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, pipelineScheduleActionHandler())))

	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_pipeline_schedule_list", map[string]any{"project_id": "1"}},
		{"get", "gitlab_pipeline_schedule_get", map[string]any{"project_id": "1", "schedule_id": 1}},
		{"create", "gitlab_pipeline_schedule_create", map[string]any{"project_id": "1", "description": "nightly", "ref": "main", "cron": "0 1 * * *"}},
		{"update", "gitlab_pipeline_schedule_update", map[string]any{"project_id": "1", "schedule_id": 1, "description": "updated"}},
		{"delete", "gitlab_pipeline_schedule_delete", map[string]any{"project_id": "1", "schedule_id": 1}},
		{"run", "gitlab_pipeline_schedule_run", map[string]any{"project_id": "1", "schedule_id": 1}},
		{"take_ownership", "gitlab_pipeline_schedule_take_ownership", map[string]any{"project_id": "1", "schedule_id": 1}},
		{"create_variable", "gitlab_pipeline_schedule_create_variable", map[string]any{"project_id": "1", "schedule_id": 1, "key": "K", "value": "V"}},
		{"edit_variable", "gitlab_pipeline_schedule_edit_variable", map[string]any{"project_id": "1", "schedule_id": 1, "key": "K", "value": "V2"}},
		{"delete_variable", "gitlab_pipeline_schedule_delete_variable", map[string]any{"project_id": "1", "schedule_id": 1, "key": "K"}},
		{"list_triggered_pipelines", "gitlab_pipeline_schedule_list_triggered_pipelines", map[string]any{"project_id": "1", "schedule_id": 1}},
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

// TestActionSpecs_VariableValueGuidance verifies schedule variable actions keep
// value guidance in their catalog metadata and schemas.
func TestActionSpecs_VariableValueGuidance(t *testing.T) {
	byTool := pipelineScheduleSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, pipelineScheduleActionHandler())))

	for _, toolName := range []string{"gitlab_pipeline_schedule_create_variable", "gitlab_pipeline_schedule_edit_variable"} {
		guidance := byTool[toolName].ParameterGuidance["value"]
		if guidance.SemanticRole != "pipeline_schedule_variable_value" {
			t.Fatalf("%s value SemanticRole = %q, want pipeline_schedule_variable_value", toolName, guidance.SemanticRole)
		}
		if !strings.Contains(guidance.ValueSource, "supply an explicit value") {
			t.Fatalf("%s value ValueSource = %q, want explicit value guidance", toolName, guidance.ValueSource)
		}
		description := schemaPropertyDescription(t, byTool[toolName].Route.InputSchema, "value")
		if !strings.Contains(description, "Required") {
			t.Fatalf("%s value schema description = %q, want required guidance", toolName, description)
		}
	}
}

// TestActionSpecs_DeleteOutputs verifies delete routes preserve their success messages.
func TestActionSpecs_DeleteOutputs(t *testing.T) {
	byTool := pipelineScheduleSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, pipelineScheduleActionHandler())))

	scheduleResult, err := byTool["gitlab_pipeline_schedule_delete"].Route.Handler(t.Context(), map[string]any{"project_id": "1", "schedule_id": 1})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_pipeline_schedule_delete) error: %v", err)
	}
	scheduleOut, ok := scheduleResult.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_pipeline_schedule_delete) returned %T, want toolutil.DeleteOutput", scheduleResult)
	}
	if scheduleOut.Message != "Successfully deleted pipeline schedule." {
		t.Fatalf("schedule delete message = %q", scheduleOut.Message)
	}

	variableResult, err := byTool["gitlab_pipeline_schedule_delete_variable"].Route.Handler(t.Context(), map[string]any{"project_id": "1", "schedule_id": 1, "key": "K"})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_pipeline_schedule_delete_variable) error: %v", err)
	}
	variableOut, ok := variableResult.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_pipeline_schedule_delete_variable) returned %T, want toolutil.DeleteOutput", variableResult)
	}
	if variableOut.Message != "Successfully deleted pipeline schedule variable \"K\"." {
		t.Fatalf("variable delete message = %q", variableOut.Message)
	}
}

// TestActionSpecs_MutationErrors verifies mutating route failures propagate.
func TestActionSpecs_MutationErrors(t *testing.T) {
	byTool := pipelineScheduleSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_pipeline_schedule_delete", map[string]any{"project_id": "42", "schedule_id": 1}},
		{"gitlab_pipeline_schedule_delete_variable", map[string]any{"project_id": "42", "schedule_id": 1, "key": "VAR"}},
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

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
	byTool := pipelineScheduleSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	for _, toolName := range []string{"gitlab_pipeline_schedule_delete", "gitlab_pipeline_schedule_delete_variable"} {
		toolutil.RegisterSurfaceToolFromSpec(server, byTool[toolName], toolutil.SurfaceToolRegisterOptions{
			Description: "Test pipeline schedule destructive confirmation.",
			Icons:       toolutil.IconSchedule,
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
		{"gitlab_pipeline_schedule_delete", map[string]any{"project_id": "42", "schedule_id": 1}},
		{"gitlab_pipeline_schedule_delete_variable", map[string]any{"project_id": "42", "schedule_id": 1, "key": "VAR"}},
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

func pipelineScheduleActionHandler() http.Handler {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/projects/1/pipeline_schedules", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+actionSpecScheduleJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/projects/1/pipeline_schedules/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, actionSpecScheduleJSON)
	})
	handler.HandleFunc("POST /api/v4/projects/1/pipeline_schedules", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, actionSpecScheduleJSON)
	})
	handler.HandleFunc("PUT /api/v4/projects/1/pipeline_schedules/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, actionSpecScheduleJSON)
	})
	handler.HandleFunc("DELETE /api/v4/projects/1/pipeline_schedules/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("POST /api/v4/projects/1/pipeline_schedules/1/play", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	handler.HandleFunc("POST /api/v4/projects/1/pipeline_schedules/1/take_ownership", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, actionSpecScheduleJSON)
	})
	handler.HandleFunc("POST /api/v4/projects/1/pipeline_schedules/1/variables", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, actionSpecVariableJSON)
	})
	handler.HandleFunc("PUT /api/v4/projects/1/pipeline_schedules/1/variables/K", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"key":"K","value":"V2","variable_type":"env_var"}`)
	})
	handler.HandleFunc("DELETE /api/v4/projects/1/pipeline_schedules/1/variables/K", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, actionSpecVariableJSON)
	})
	handler.HandleFunc("GET /api/v4/projects/1/pipeline_schedules/1/pipelines", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":100,"iid":10,"ref":"main","sha":"abc","status":"success","source":"schedule","web_url":"https://example.com/p/100"}]`)
	})
	return handler
}

func pipelineScheduleSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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

func schemaPropertyDescription(t *testing.T, schema map[string]any, propertyName string) string {
	t.Helper()
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties = %T, want map[string]any", schema["properties"])
	}
	property, ok := properties[propertyName].(map[string]any)
	if !ok {
		t.Fatalf("schema property %q = %T, want map[string]any", propertyName, properties[propertyName])
	}
	description, ok := property["description"].(string)
	if !ok {
		t.Fatalf("schema property %q description = %T, want string", propertyName, property["description"])
	}
	return description
}
