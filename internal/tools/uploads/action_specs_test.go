// action_specs_test.go contains canonical-route tests for project upload actions.
package uploads

import (
	"context"
	"encoding/base64"
	"net/http"
	"slices"
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
		{"gitlab_project_upload_delete_by_secret", map[string]any{"project_id": "42", "secret": "abc123", "filename": "file.txt"}},
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
		{"gitlab_project_upload_delete_by_secret", map[string]any{"project_id": "p", "secret": "abc123", "filename": "f.txt"}},
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

	secretResult, err := byTool["gitlab_project_upload_delete_by_secret"].Route.Handler(t.Context(), map[string]any{
		"project_id": "42", "secret": "abc123", "filename": "file.txt",
	})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_project_upload_delete_by_secret) error: %v", err)
	}
	secretOut, ok := secretResult.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_project_upload_delete_by_secret) returned %T, want toolutil.DeleteOutput", secretResult)
	}
	if secretOut.Message != "Successfully deleted upload abc123/file.txt from project 42." {
		t.Fatalf("delete-by-secret message = %q", secretOut.Message)
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

// TestActionSpecs_DistinctiveAliases verifies every project-upload spec carries
// distinctive natural-language aliases beyond its canonical tool name, satisfying
// the 1:1 audit metadata norm (no aliases_only_toolname findings).
func TestActionSpecs_DistinctiveAliases(t *testing.T) {
	byTool := uploadSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, uploadsActionHandler())))

	for tool, want := range uploadActionAliases {
		spec, ok := byTool[tool]
		if !ok {
			t.Fatalf("missing spec for %q", tool)
		}
		extra := 0
		for _, alias := range spec.Aliases {
			if strings.TrimSpace(alias) != tool {
				extra++
			}
		}
		if extra < 2 || extra > 4 {
			t.Fatalf("%s: want 2-4 natural-language aliases, got %d (%v)", tool, extra, spec.Aliases)
		}
		for _, w := range want {
			if !slices.Contains(spec.Aliases, w) {
				t.Fatalf("%s: missing alias %q in %v", tool, w, spec.Aliases)
			}
		}
	}
}

// TestDecorateUploadMeta_UnknownToolNoOp verifies the decorator leaves options
// untouched for a tool name absent from the alias map.
func TestDecorateUploadMeta_UnknownToolNoOp(t *testing.T) {
	options := toolutil.ActionSpecOptions{Aliases: []string{"gitlab_unknown_tool"}}
	decorateUploadMeta(&options, "gitlab_unknown_tool")
	if len(options.Aliases) != 1 {
		t.Fatalf("expected no-op for unknown tool, got aliases %v", options.Aliases)
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
