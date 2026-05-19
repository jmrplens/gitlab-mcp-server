// action_specs_test.go contains canonical-route tests for project mirror actions.
package projectmirrors

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_CallAllRoutes exercises every project mirror tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := projectMirrorSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, projectMirrorActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_list_project_mirrors", map[string]any{"project_id": testProjectID}},
		{"gitlab_get_project_mirror", map[string]any{"project_id": testProjectID, "mirror_id": 42}},
		{"gitlab_get_project_mirror_public_key", map[string]any{"project_id": testProjectID, "mirror_id": 42}},
		{"gitlab_add_project_mirror", map[string]any{"project_id": testProjectID, "url": "https://example.com/repo.git"}},
		{"gitlab_edit_project_mirror", map[string]any{"project_id": testProjectID, "mirror_id": 42}},
		{"gitlab_delete_project_mirror", map[string]any{"project_id": testProjectID, "mirror_id": 42}},
		{"gitlab_force_push_mirror_update", map[string]any{"project_id": testProjectID, "mirror_id": 42}},
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

// TestActionSpecs_DeleteAndForcePushOutputs verifies legacy success messages remain stable.
func TestActionSpecs_DeleteAndForcePushOutputs(t *testing.T) {
	byTool := projectMirrorSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, projectMirrorActionHandler())))

	deleteResult, err := byTool["gitlab_delete_project_mirror"].Route.Handler(t.Context(), map[string]any{"project_id": testProjectID, "mirror_id": 42})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_delete_project_mirror) error: %v", err)
	}
	deleteOut, ok := deleteResult.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_delete_project_mirror) returned %T, want toolutil.DeleteOutput", deleteResult)
	}
	if deleteOut.Message != "Successfully deleted mirror 42 from project myproject." {
		t.Fatalf("delete message = %q", deleteOut.Message)
	}

	forcePushResult, err := byTool["gitlab_force_push_mirror_update"].Route.Handler(t.Context(), map[string]any{"project_id": testProjectID, "mirror_id": 42})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_force_push_mirror_update) error: %v", err)
	}
	forcePushOut, ok := forcePushResult.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_force_push_mirror_update) returned %T, want toolutil.DeleteOutput", forcePushResult)
	}
	if forcePushOut.Message != "Force push update triggered for mirror 42 in project myproject" {
		t.Fatalf("force-push message = %q", forcePushOut.Message)
	}
}

// TestActionSpecs_ErrorPaths verifies destructive routes propagate backend failures.
func TestActionSpecs_ErrorPaths(t *testing.T) {
	byTool := projectMirrorSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_delete_project_mirror", map[string]any{"project_id": testProjectID, "mirror_id": 42}},
		{"gitlab_force_push_mirror_update", map[string]any{"project_id": testProjectID, "mirror_id": 42}},
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
	byTool := projectMirrorSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_delete_project_mirror"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test project mirror destructive confirmation.",
		Icons:       toolutil.IconInfra,
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
		Name:      "gitlab_delete_project_mirror",
		Arguments: map[string]any{"project_id": testProjectID, "mirror_id": 42},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

func projectMirrorActionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && path == pathMirrors:
			testutil.RespondJSONWithPagination(w, http.StatusOK, "["+mirrorJSON+"]",
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
		case r.Method == http.MethodGet && path == pathMirrorKey42:
			testutil.RespondJSON(w, http.StatusOK, publicKeyJSON)
		case r.Method == http.MethodGet && path == pathMirror42:
			testutil.RespondJSON(w, http.StatusOK, mirrorJSON)
		case r.Method == http.MethodPost && path == pathMirrors:
			testutil.RespondJSON(w, http.StatusCreated, mirrorJSON)
		case r.Method == http.MethodPut && path == pathMirror42:
			testutil.RespondJSON(w, http.StatusOK, mirrorJSON)
		case r.Method == http.MethodDelete && path == pathMirror42:
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && path == pathMirrorSync42:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
}

func projectMirrorSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
