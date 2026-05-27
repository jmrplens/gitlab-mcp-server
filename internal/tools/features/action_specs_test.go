// action_specs_test.go contains canonical-route tests for feature flag actions.
package features

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const actionSpecFeatureJSON = `{"name":"flag1","state":"on","gates":[{"key":"boolean","value":true}]}`

// TestActionSpecs_Metadata verifies canonical metadata for feature actions.
func TestActionSpecs_Metadata(t *testing.T) {
	byTool := featureSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, featureActionHandler())))
	if len(byTool) != 4 {
		t.Fatalf("unique individual tools = %d, want 4", len(byTool))
	}
	for toolName, spec := range byTool {
		if spec.OwnerPackage != "features" {
			t.Fatalf("OwnerPackage for %s = %q, want features", toolName, spec.OwnerPackage)
		}
		if spec.Usage == "" {
			t.Fatalf("Usage for %s should not be empty", toolName)
		}
		if len(spec.Aliases) == 0 {
			t.Fatalf("Aliases for %s should not be empty", toolName)
		}
	}
	if byTool["gitlab_set_feature_flag"].ParameterGuidance["name"].SemanticRole == "" {
		t.Fatal("gitlab_set_feature_flag should define name parameter guidance")
	}
	if !byTool["gitlab_delete_feature_flag"].Route.Destructive {
		t.Fatal("gitlab_delete_feature_flag should be destructive")
	}
}

// TestActionSpecs_CallRoutes exercises feature tools through their canonical routes.
func TestActionSpecs_CallRoutes(t *testing.T) {
	byTool := featureSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, featureActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_list_features", map[string]any{}},
		{"gitlab_list_feature_definitions", map[string]any{}},
		{"gitlab_set_feature_flag", map[string]any{"name": "flag1", "value": true}},
		{"gitlab_delete_feature_flag", map[string]any{"name": "flag1"}},
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

// TestActionSpecs_SetRouteSchema verifies the feature value schema preserves multiple accepted types.
func TestActionSpecs_SetRouteSchema(t *testing.T) {
	byTool := featureSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, featureActionHandler())))

	properties, ok := byTool["gitlab_set_feature_flag"].Route.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties schema has type %T", byTool["gitlab_set_feature_flag"].Route.InputSchema["properties"])
	}
	value, ok := properties["value"].(map[string]any)
	if !ok {
		t.Fatalf("value schema has type %T", properties["value"])
	}
	oneOf, ok := value["oneOf"].([]any)
	if !ok {
		t.Fatalf("value oneOf has type %T", value["oneOf"])
	}
	if len(oneOf) != 3 {
		t.Fatalf("value oneOf length = %d, want 3", len(oneOf))
	}
}

// TestActionSpecs_DeleteOutput verifies delete preserves its success message.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	byTool := featureSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, featureActionHandler())))

	result, err := byTool["gitlab_delete_feature_flag"].Route.Handler(t.Context(), map[string]any{"name": "flag1"})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_delete_feature_flag) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_delete_feature_flag) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted feature flag." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestActionSpecs_DeleteOutputError verifies delete output propagates backend errors.
func TestActionSpecs_DeleteOutputError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"forbidden"}`)
	}))
	_, err := deleteOutput(t.Context(), client, DeleteInput{Name: "flag1"})
	if err == nil {
		t.Fatal("deleteOutput() error = nil, want backend error")
	}
}

// TestActionSpecs_ErrorsPropagate verifies route backend errors propagate directly.
func TestActionSpecs_ErrorsPropagate(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))
	byTool := featureSpecsByTool(t, ActionSpecs(client))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_list_features", map[string]any{}},
		{"gitlab_set_feature_flag", map[string]any{"name": "test_flag", "value": "true"}},
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
	byTool := featureSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_delete_feature_flag"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test feature flag destructive confirmation.",
		Icons:       toolutil.IconConfig,
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
		Name:      "gitlab_delete_feature_flag",
		Arguments: map[string]any{"name": "test_flag"},
	})
	if callErr != nil {
		t.Fatalf("CallTool error: %v", callErr)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

func featureActionHandler() http.Handler {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/features", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+actionSpecFeatureJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/features/definitions", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"name":"def1","type":"development","group":"group::ide","milestone":"15.0","default_enabled":true,"log_state_changes":false}]`)
	})
	handler.HandleFunc("POST /api/v4/features/flag1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, actionSpecFeatureJSON)
	})
	handler.HandleFunc("DELETE /api/v4/features/flag1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	return handler
}

func featureSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
