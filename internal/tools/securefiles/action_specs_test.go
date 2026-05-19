// action_specs_test.go contains canonical-route tests for secure file actions.
package securefiles

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const secureFileActionJSON = `{"id":1,"name":"key.pem","checksum":"abc","checksum_algorithm":"sha256"}`

// TestActionSpecs_CallAllRoutes exercises every secure file tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := secureFilesSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, secureFilesActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_list_secure_files", map[string]any{"project_id": "1", "page": 1, "per_page": 20}},
		{"gitlab_show_secure_file", map[string]any{"project_id": "1", "file_id": 1}},
		{"gitlab_create_secure_file", map[string]any{"project_id": "1", "name": "cert.pem", "content_base64": "ZGF0YQ=="}},
		{"gitlab_remove_secure_file", map[string]any{"project_id": "1", "file_id": 1}},
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

// TestActionSpecs_RouteErrors verifies canonical routes propagate backend errors.
func TestActionSpecs_RouteErrors(t *testing.T) {
	byTool := secureFilesSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))))

	for _, tt := range []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_list_secure_files", map[string]any{"project_id": "1", "page": 1, "per_page": 20}},
		{"gitlab_show_secure_file", map[string]any{"project_id": "1", "file_id": 1}},
		{"gitlab_create_secure_file", map[string]any{"project_id": "1", "name": "x", "content_base64": "ZGF0YQ=="}},
		{"gitlab_remove_secure_file", map[string]any{"project_id": "1", "file_id": 1}},
	} {
		t.Run(tt.tool, func(t *testing.T) {
			_, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err == nil {
				t.Fatal("expected error from failing backend")
			}
		})
	}
}

// TestActionSpecs_DeleteOutput verifies the delete route preserves its success message.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	byTool := secureFilesSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, secureFilesActionHandler())))

	result, err := byTool["gitlab_remove_secure_file"].Route.Handler(t.Context(), map[string]any{"project_id": "1", "file_id": 1})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_remove_secure_file) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_remove_secure_file) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted secure file." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
	byTool := secureFilesSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_remove_secure_file"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test secure file destructive confirmation.",
		Icons:       toolutil.IconSecurity,
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
		Name:      "gitlab_remove_secure_file",
		Arguments: map[string]any{"project_id": "1", "file_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool returned transport error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result when confirmation is declined")
	}
}

func secureFilesActionHandler() http.Handler {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/projects/1/secure_files", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+secureFileActionJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/projects/1/secure_files/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, secureFileActionJSON)
	})
	handler.HandleFunc("POST /api/v4/projects/1/secure_files", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":2,"name":"cert.pem","checksum":"def","checksum_algorithm":"sha256"}`)
	})
	handler.HandleFunc("DELETE /api/v4/projects/1/secure_files/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	return handler
}

func secureFilesSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
