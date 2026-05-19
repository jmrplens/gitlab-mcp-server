// action_specs_test.go contains canonical-route tests for project upload actions.
package uploads

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_CallAllRoutes exercises every project upload tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := uploadSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, uploadsActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_project_upload", map[string]any{"project_id": "42", "filename": "file.txt", "content_base64": base64.StdEncoding.EncodeToString([]byte("hello"))}},
		{"gitlab_project_upload_list", map[string]any{"project_id": "42"}},
		{"gitlab_project_upload_delete", map[string]any{"project_id": "42", "upload_id": 1}},
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

// TestActionSpecs_ErrorPaths verifies canonical routes propagate backend errors.
func TestActionSpecs_ErrorPaths(t *testing.T) {
	byTool := uploadSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_project_upload", map[string]any{"project_id": "p", "content_base64": base64.StdEncoding.EncodeToString([]byte("data")), "filename": "f.txt"}},
		{"gitlab_project_upload_list", map[string]any{"project_id": "p"}},
		{"gitlab_project_upload_delete", map[string]any{"project_id": "p", "upload_id": 1}},
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

// TestActionSpecs_DeleteOutput verifies the delete route preserves its success message.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	byTool := uploadSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, uploadsActionHandler())))

	result, err := byTool["gitlab_project_upload_delete"].Route.Handler(t.Context(), map[string]any{
		"project_id": "42", "upload_id": 1,
	})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_project_upload_delete) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_project_upload_delete) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted upload 1 from project 42." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
	byTool := uploadSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_project_upload_delete"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test project upload destructive confirmation.",
		Icons:       toolutil.IconUpload,
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
		Name:      "gitlab_project_upload_delete",
		Arguments: map[string]any{"project_id": "p", "upload_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

func uploadsActionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/uploads"):
			testutil.RespondJSON(w, http.StatusCreated,
				`{"alt":"file.txt","url":"/uploads/a1/file.txt","full_path":"/uploads/a1/file.txt","markdown":"![file.txt](/uploads/a1/file.txt)"}`)
		case r.Method == http.MethodGet:
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"size":100,"filename":"file.txt"}]`)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
}

func uploadSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
