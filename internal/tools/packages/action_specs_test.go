// action_specs_test.go contains canonical-route tests for package actions.
package packages

import (
	"context"
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestActionSpecs_CallAllRoutes exercises every package tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := packageSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, packageActionHandler())))
	content64 := base64.StdEncoding.EncodeToString([]byte("test-data"))
	outDir := t.TempDir()

	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"publish", "gitlab_package_publish", map[string]any{
			"project_id": "1", "package_name": testPackageName, "package_version": "1.0.0",
			"file_name": testFileName, "content_base64": content64,
		}},
		{"download", "gitlab_package_download", map[string]any{
			"project_id": "1", "package_name": testPackageName, "package_version": "1.0.0",
			"file_name": testFileName, "output_path": filepath.Join(outDir, "dl.bin"),
		}},
		{"list", "gitlab_package_list", map[string]any{"project_id": "1"}},
		{"file_list", "gitlab_package_file_list", map[string]any{"project_id": "1", "package_id": "10"}},
		{"delete", "gitlab_package_delete", map[string]any{"project_id": "1", "package_id": "10"}},
		{"file_delete", "gitlab_package_file_delete", map[string]any{"project_id": "1", "package_id": "10", "package_file_id": "20"}},
		{"publish_and_link", "gitlab_package_publish_and_link", map[string]any{
			"project_id": "1", "package_name": testPackageName, "package_version": "1.0.0",
			"file_name": testFileName, "content_base64": content64, "tag_name": "v1.0.0",
		}},
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

	t.Run("publish_directory", func(t *testing.T) {
		pubDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(pubDir, "test.bin"), []byte("data"), 0644); err != nil {
			t.Fatalf("write package fixture: %v", err)
		}
		result, err := byTool["gitlab_package_publish_directory"].Route.Handler(t.Context(), map[string]any{
			"project_id":      "1",
			"package_name":    testPackageName,
			"package_version": "1.0.0",
			"directory_path":  pubDir,
		})
		if err != nil {
			t.Fatalf("Route.Handler(gitlab_package_publish_directory) error: %v", err)
		}
		if result == nil {
			t.Fatal("Route.Handler(gitlab_package_publish_directory) returned nil")
		}
	})
}

// TestActionSpecs_DeleteOutputs verifies delete routes preserve their success messages.
func TestActionSpecs_DeleteOutputs(t *testing.T) {
	byTool := packageSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, packageActionHandler())))

	packageResult, err := byTool["gitlab_package_delete"].Route.Handler(t.Context(), map[string]any{"project_id": "1", "package_id": "10"})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_package_delete) error: %v", err)
	}
	packageOut, ok := packageResult.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_package_delete) returned %T, want toolutil.DeleteOutput", packageResult)
	}
	if packageOut.Message != "Successfully deleted package 10 from project 1." {
		t.Fatalf("package delete message = %q", packageOut.Message)
	}

	fileResult, err := byTool["gitlab_package_file_delete"].Route.Handler(t.Context(), map[string]any{"project_id": "1", "package_id": "10", "package_file_id": "20"})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_package_file_delete) error: %v", err)
	}
	fileOut, ok := fileResult.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_package_file_delete) returned %T, want toolutil.DeleteOutput", fileResult)
	}
	if fileOut.Message != "Successfully deleted file 20 from package 10 in project 1." {
		t.Fatalf("file delete message = %q", fileOut.Message)
	}
}

// TestActionSpecs_DeleteError verifies delete route failures propagate.
func TestActionSpecs_DeleteError(t *testing.T) {
	byTool := packageSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))))

	_, err := byTool["gitlab_package_delete"].Route.Handler(t.Context(), map[string]any{"project_id": "1", "package_id": "10"})
	if err == nil {
		t.Fatal("expected route error")
	}

	_, err = byTool["gitlab_package_file_delete"].Route.Handler(t.Context(), map[string]any{"project_id": "1", "package_id": "10", "package_file_id": "20"})
	if err == nil {
		t.Fatal("expected file delete route error")
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
	byTool := packageSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_package_delete"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test package destructive confirmation.",
		Icons:       toolutil.IconPackage,
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
		Name:      "gitlab_package_delete",
		Arguments: map[string]any{"project_id": "1", "package_id": "10"},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

func packageActionHandler() http.Handler {
	handler := http.NewServeMux()
	handler.HandleFunc(pathPutPkg1, func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{
			"id": 1, "package_id": 10, "file_name": "app.tar.gz",
			"size": 1024, "file_sha256": "abc", "file_md5": "md5",
			"file_sha1": "sha1", "file_store": 1,
			"created_at": "2026-01-01T00:00:00Z"
		}`)
	})
	handler.HandleFunc("PUT /api/v4/projects/1/packages/generic/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{
			"id": 2, "package_id": 10, "file_name": "test.bin",
			"size": 4, "file_sha256": "def", "file_md5": "md5",
			"file_sha1": "sha1", "file_store": 1
		}`)
	})
	handler.HandleFunc("GET /api/v4/projects/1/packages/generic/my-pkg/1.0.0/app.tar.gz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(hdrContentType, mimeOctetStream)
		w.Write([]byte("file-data"))
	})
	handler.HandleFunc("GET /api/v4/projects/1/packages", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":10,"name":"my-pkg","version":"1.0.0","package_type":"generic","status":"default"}]`)
	})
	handler.HandleFunc("GET /api/v4/projects/1/packages/10/package_files", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":20,"package_id":10,"file_name":"app.tar.gz","size":1024,"file_sha256":"abc"}]`)
	})
	handler.HandleFunc("DELETE /api/v4/projects/1/packages/10", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("DELETE /api/v4/projects/1/packages/10/package_files/20", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("POST /api/v4/projects/1/releases/v1.0.0/assets/links", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{
			"id": 50, "name": "app.tar.gz",
			"url": "https://example.com/pkg", "link_type": "package", "external": true
		}`)
	})
	return handler
}

func packageSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
