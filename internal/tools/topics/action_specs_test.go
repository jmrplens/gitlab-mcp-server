// action_specs_test.go contains canonical-route tests for topic actions.
package topics

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_Metadata verifies canonical metadata for topic actions.
func TestActionSpecs_Metadata(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/topics", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+topicJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/topics/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, topicJSON)
	})
	handler.HandleFunc("POST /api/v4/topics", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, topicJSON)
	})
	handler.HandleFunc("PUT /api/v4/topics/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, topicJSON)
	})
	handler.HandleFunc("DELETE /api/v4/topics/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	byTool := topicsSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, handler)))

	if len(byTool) != 5 {
		t.Fatalf("unique individual tools = %d, want 5", len(byTool))
	}
	for toolName, spec := range byTool {
		if spec.OwnerPackage != "topics" {
			t.Fatalf("OwnerPackage for %s = %q, want topics", toolName, spec.OwnerPackage)
		}
		if spec.Usage == "" {
			t.Fatalf("Usage for %s should not be empty", toolName)
		}
		if len(spec.Aliases) == 0 {
			t.Fatalf("Aliases for %s should not be empty", toolName)
		}
	}
	if byTool["gitlab_get_topic"].ParameterGuidance["topic_id"].SemanticRole == "" {
		t.Fatal("gitlab_get_topic should define topic_id parameter guidance")
	}
	if !byTool["gitlab_delete_topic"].Route.Destructive {
		t.Fatal("gitlab_delete_topic should be destructive")
	}
}

// TestActionSpecs_CallAllRoutes exercises every topic tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/topics", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+topicJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/topics/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, topicJSON)
	})
	handler.HandleFunc("POST /api/v4/topics", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, topicJSON)
	})
	handler.HandleFunc("PUT /api/v4/topics/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, topicJSON)
	})
	handler.HandleFunc("DELETE /api/v4/topics/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	byTool := topicsSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, handler)))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_list_topics", map[string]any{}},
		{"gitlab_get_topic", map[string]any{"topic_id": 1}},
		{"gitlab_create_topic", map[string]any{"name": "go"}},
		{"gitlab_update_topic", map[string]any{"topic_id": 1, "name": "golang"}},
		{"gitlab_delete_topic", map[string]any{"topic_id": 1}},
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
	byTool := topicsSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))))

	_, err := byTool["gitlab_delete_topic"].Route.Handler(t.Context(), map[string]any{"topic_id": 1})
	if err == nil {
		t.Fatal("expected error from delete with failing backend")
	}
}

// TestActionSpecs_DeleteOutput verifies the delete route preserves its success message.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/topics/1" || r.Method != http.MethodDelete {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	byTool := topicsSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_delete_topic"].Route.Handler(t.Context(), map[string]any{"topic_id": 1})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_delete_topic) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_delete_topic) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted topic." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
	byTool := topicsSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_delete_topic"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test topic destructive confirmation.",
		Icons:       toolutil.IconLabel,
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
		Name:      "gitlab_delete_topic",
		Arguments: map[string]any{"topic_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool returned transport error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result when confirmation is declined")
	}
}

func topicsSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
