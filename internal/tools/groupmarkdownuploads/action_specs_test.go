// action_specs_test.go contains canonical-route tests for group markdown upload actions.
package groupmarkdownuploads

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const registerUploadListJSON = `[{"id":1,"size":1024,"filename":"image.png","created_at":"2026-01-01T00:00:00Z"}]`

// TestActionSpecs_CallAllRoutes drives every route this domain publishes
// through the catalog surface, with the mux routing on the whole path.
// A route that builds the wrong path reaches the mux's 404 and fails here,
// which is what holds the secret and the filename in that order.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/groups/5/uploads", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, registerUploadListJSON)
	})
	handler.HandleFunc("DELETE /api/v4/groups/5/uploads/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("DELETE /api/v4/groups/5/uploads/abc123/image.png", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	byTool := groupMarkdownUploadSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, handler)))

	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_list_group_markdown_uploads", map[string]any{"group_id": "5"}},
		{"delete_by_id", "gitlab_delete_group_markdown_upload_by_id", map[string]any{"group_id": "5", "upload_id": 1}},
		{"delete_by_secret", "gitlab_delete_group_markdown_upload_by_secret", map[string]any{"group_id": "5", "secret": "abc123", "filename": "image.png"}},
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

// TestActionSpecs_ErrorPaths verifies that a 403 from GitLab reaches the caller
// as an error through every route rather than as an empty success.
// What the error says is asserted in
// TestHandlers_NotFound_CarryTheRouteThatRecovers, not here.
func TestActionSpecs_ErrorPaths(t *testing.T) {
	byTool := groupMarkdownUploadSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"forbidden"}`)
	}))))

	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_list_group_markdown_uploads", map[string]any{"group_id": "5"}},
		{"delete_by_id", "gitlab_delete_group_markdown_upload_by_id", map[string]any{"group_id": "5", "upload_id": 1}},
		{"delete_by_secret", "gitlab_delete_group_markdown_upload_by_secret", map[string]any{"group_id": "5", "secret": "abc", "filename": "file.png"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err == nil {
				t.Fatalf("Route.Handler(%s) expected error", tt.tool)
			}
		})
	}
}

// TestActionSpecs_DeleteOutput verifies that both delete routes answer a 204
// with a toolutil.DeleteOutput carrying the confirmation a model reads,
// rather than with a bare nil the surface would render as nothing.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("DELETE /api/v4/groups/5/uploads/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("DELETE /api/v4/groups/5/uploads/abc123/image.png", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	byTool := groupMarkdownUploadSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, handler)))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_delete_group_markdown_upload_by_id", map[string]any{"group_id": "5", "upload_id": 1}},
		{"gitlab_delete_group_markdown_upload_by_secret", map[string]any{"group_id": "5", "secret": "abc123", "filename": "image.png"}},
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
			if out.Message != "Successfully deleted group markdown upload." {
				t.Fatalf("delete message = %q", out.Message)
			}
		})
	}
}

// TestCatalogSurface_DeleteConfirmDeclined verifies that a declined
// elicitation ends each destructive route with a result rather than a
// transport error, and that nothing reaches GitLab: the client is built on
// testutil.ForbiddenHandler, which fails the test if a request arrives.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	byTool := groupMarkdownUploadSpecsByTool(t, ActionSpecs(client))

	for _, tt := range []struct {
		name string
		args map[string]any
	}{
		{"gitlab_delete_group_markdown_upload_by_id", map[string]any{"group_id": "42", "upload_id": 1}},
		{"gitlab_delete_group_markdown_upload_by_secret", map[string]any{"group_id": "42", "secret": "s", "filename": "f.png"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
			toolutil.RegisterSurfaceToolFromSpec(server, byTool[tt.name], toolutil.SurfaceToolRegisterOptions{
				Description: "Test group markdown upload destructive confirmation.",
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

// The formatter itself is covered whole-output in markdown_test.go.

// TestMarkdownInit_Registry verifies that this package's init registered a
// formatter for ListOutput, so the registry can find one at runtime.
// It contacts no GitLab: the registry lookup is the whole subject.
func TestMarkdownInit_Registry(t *testing.T) {
	out := toolutil.MarkdownForResult(ListOutput{})
	if out == nil {
		t.Fatal("expected non-nil result for ListOutput")
	}
}

func groupMarkdownUploadSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
