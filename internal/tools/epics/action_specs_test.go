// action_specs_test.go contains canonical-route tests for epic actions.
package epics

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_CallAllRoutes exercises every non-delete epic tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/epics") {
			testutil.RespondJSON(w, http.StatusOK, `[`+epicLinkJSON+`]`)
			return
		}
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/epics/") {
			testutil.RespondJSON(w, http.StatusOK, `[`+epicLinkJSON+`]`)
			return
		}
		if r.Method == http.MethodPost {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			query := string(body)
			switch {
			case strings.Contains(query, "ListWorkItems"):
				testutil.RespondJSON(w, http.StatusOK, listResponseJSON)
			case strings.Contains(query, "GetWorkItemID"):
				testutil.RespondJSON(w, http.StatusOK, deleteGIDResponseJSON)
			case strings.Contains(query, "GetWorkItem"):
				testutil.RespondJSON(w, http.StatusOK, getResponseJSON)
			case strings.Contains(query, "workItemCreate"):
				testutil.RespondJSON(w, http.StatusOK, createResponseJSON)
			case strings.Contains(query, "workItemUpdate"):
				testutil.RespondJSON(w, http.StatusOK, updateResponseJSON)
			case strings.Contains(query, "workItemDelete"):
				testutil.RespondJSON(w, http.StatusOK, deleteDeleteResponseJSON)
			default:
				t.Fatalf("unexpected GraphQL query: %s", query)
			}
			return
		}
		http.NotFound(w, r)
	})
	byTool := epicSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, mux)))

	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_epic_list", map[string]any{"full_path": testFullPath}},
		{"get", "gitlab_epic_get", map[string]any{"full_path": testFullPath, "epic_iid": float64(1)}},
		{"get_links", "gitlab_epic_get_links", map[string]any{"full_path": testFullPath, "epic_iid": float64(1)}},
		{"create", "gitlab_epic_create", map[string]any{"full_path": testFullPath, "title": "New Epic"}},
		{"update", "gitlab_epic_update", map[string]any{"full_path": testFullPath, "epic_iid": float64(1), "title": "Updated"}},
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

// TestActionSpecs_DeleteOutput verifies the delete route returns the canonical success message.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	call := 0
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf(fmtUnexpMethod, r.Method)
		}
		call++
		switch call {
		case 1:
			testutil.RespondJSON(w, http.StatusOK, deleteGIDResponseJSON)
		default:
			testutil.RespondJSON(w, http.StatusOK, deleteDeleteResponseJSON)
		}
	}))
	byTool := epicSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_epic_delete"].Route.Handler(t.Context(), map[string]any{
		"full_path": testFullPath,
		"epic_iid":  int64(1),
	})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_epic_delete) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_epic_delete) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted epic &1 from group my-group." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestActionSpecs_DeleteError verifies that the delete route propagates backend errors.
func TestActionSpecs_DeleteError(t *testing.T) {
	byTool := epicSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))))

	_, err := byTool["gitlab_epic_delete"].Route.Handler(t.Context(), map[string]any{
		"full_path": testFullPath,
		"epic_iid":  float64(1),
	})
	if err == nil {
		t.Fatal("expected error from delete with failing backend")
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
	byTool := epicSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_epic_delete"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test epic destructive confirmation.",
		Icons:       toolutil.IconEpic,
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
		Name: "gitlab_epic_delete",
		Arguments: map[string]any{
			"full_path": testFullPath,
			"epic_iid":  float64(1),
		},
	})
	if err != nil {
		t.Fatalf("CallTool returned transport error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result when confirmation is declined")
	}
	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			if textContent.Text == "" {
				t.Error("expected non-empty cancellation message")
			}
			return
		}
	}
	t.Error("expected text content in cancellation result")
}

func epicSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
