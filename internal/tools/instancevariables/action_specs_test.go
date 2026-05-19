// action_specs_test.go contains canonical-route tests for instance CI/CD variable actions.
package instancevariables

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_CallAllRoutes exercises every instance variable tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET "+pathInstanceVars, func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+varJSON+`]`)
	})
	handler.HandleFunc("GET "+pathVar1, func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, varJSON)
	})
	handler.HandleFunc("POST "+pathInstanceVars, func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"key":"NEW_VAR","value":"new-val","variable_type":"env_var","protected":false,"masked":false,"raw":false,"description":""}`)
	})
	handler.HandleFunc("PUT "+pathVar1, func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"key":"MY_VAR","value":"updated-val","variable_type":"env_var","protected":false,"masked":false,"raw":false,"description":""}`)
	})
	handler.HandleFunc("DELETE "+pathVar1, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	byTool := instanceVariableSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, handler)))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_instance_variable_list", map[string]any{}},
		{"gitlab_instance_variable_get", map[string]any{"key": "MY_VAR"}},
		{"gitlab_instance_variable_create", map[string]any{"key": "NEW_VAR", "value": "new-val", "description": "", "variable_type": "", "protected": false, "masked": false, "raw": false}},
		{"gitlab_instance_variable_update", map[string]any{"key": "MY_VAR", "value": "updated-val", "description": "", "variable_type": "", "protected": false, "masked": false, "raw": false}},
		{"gitlab_instance_variable_delete", map[string]any{"key": "MY_VAR"}},
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
	byTool := instanceVariableSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))))

	_, err := byTool["gitlab_instance_variable_delete"].Route.Handler(t.Context(), map[string]any{"key": "MY_VAR"})
	if err == nil {
		t.Fatal("expected error from delete with failing backend")
	}
}

// TestActionSpecs_DeleteOutput verifies the delete route preserves its success message.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathVar1 || r.Method != http.MethodDelete {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	byTool := instanceVariableSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_instance_variable_delete"].Route.Handler(t.Context(), map[string]any{"key": "MY_VAR"})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_instance_variable_delete) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_instance_variable_delete) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted instance CI/CD variable." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
	byTool := instanceVariableSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_instance_variable_delete"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test instance variable destructive confirmation.",
		Icons:       toolutil.IconVariable,
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
		Name:      "gitlab_instance_variable_delete",
		Arguments: map[string]any{"key": "MY_VAR"},
	})
	if err != nil {
		t.Fatalf("CallTool returned transport error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result when confirmation is declined")
	}
}

func instanceVariableSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
