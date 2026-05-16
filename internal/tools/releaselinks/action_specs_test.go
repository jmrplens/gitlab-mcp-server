// action_specs_test.go contains canonical-route tests for release asset link actions.
package releaselinks

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const releaseLinkActionJSON = `{"id":10,"name":"Binary amd64","url":"https://example.com/bin/amd64","link_type":"package","external":true,"direct_asset_url":""}`

// TestActionSpecs_CallAllRoutes exercises every release link tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := releaseLinkSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, releaseLinksActionHandler())))

	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_release_link_list", map[string]any{"project_id": "42", "tag_name": "v1.0.0"}},
		{"get", "gitlab_release_link_get", map[string]any{"project_id": "42", "tag_name": "v1.0.0", "link_id": 10}},
		{"create", "gitlab_release_link_create", map[string]any{"project_id": "42", "tag_name": "v1.0.0", "name": "Binary", "url": "https://example.com/bin"}},
		{"update", "gitlab_release_link_update", map[string]any{"project_id": "42", "tag_name": "v1.0.0", "link_id": 10, "name": "Updated"}},
		{"delete", "gitlab_release_link_delete", map[string]any{"project_id": "42", "tag_name": "v1.0.0", "link_id": 10}},
		{"create_batch", "gitlab_release_link_create_batch", map[string]any{"project_id": "42", "tag_name": "v1.0.0", "links": []any{map[string]any{"name": "Binary", "url": "https://example.com/bin"}}}},
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

// TestActionSpecs_MutationErrors verifies canonical routes propagate backend errors.
func TestActionSpecs_MutationErrors(t *testing.T) {
	byTool := releaseLinkSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
		default:
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
		}
	}))))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_release_link_get", map[string]any{"project_id": "42", "tag_name": "v1.0.0", "link_id": 999}},
		{"gitlab_release_link_delete", map[string]any{"project_id": "42", "tag_name": "v1.0.0", "link_id": 1}},
		{"gitlab_release_link_create", map[string]any{"project_id": "42", "tag_name": "v1.0.0", "name": "asset", "url": "https://example.com/file"}},
		{"gitlab_release_link_update", map[string]any{"project_id": "42", "tag_name": "v1.0.0", "link_id": 1, "name": "new-name"}},
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
	byTool := releaseLinkSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_release_link_delete"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test release link destructive confirmation.",
		Icons:       toolutil.IconLink,
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
		Name:      "gitlab_release_link_delete",
		Arguments: map[string]any{"project_id": "42", "tag_name": "v1.0.0", "link_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

func releaseLinksActionHandler() http.Handler {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/projects/42/releases/v1.0.0/assets/links", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+releaseLinkActionJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/projects/42/releases/v1.0.0/assets/links/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, releaseLinkActionJSON)
	})
	handler.HandleFunc("POST /api/v4/projects/42/releases/v1.0.0/assets/links", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, releaseLinkActionJSON)
	})
	handler.HandleFunc("PUT /api/v4/projects/42/releases/v1.0.0/assets/links/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, releaseLinkActionJSON)
	})
	handler.HandleFunc("DELETE /api/v4/projects/42/releases/v1.0.0/assets/links/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, releaseLinkActionJSON)
	})
	return handler
}

func releaseLinkSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
