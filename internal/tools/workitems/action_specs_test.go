// action_specs_test.go contains canonical-route tests for work item actions.
package workitems

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

const (
	actionSpecWorkItemGraphQLResponse       = `{"data":{"namespace":{"workItem":{"id":"gid://gitlab/WorkItem/10","iid":"10","workItemType":{"name":"Issue"},"state":"OPEN","title":"ActionSpec test","author":{"username":"dev"},"widgets":[]}}}}`
	actionSpecWorkItemsListGraphQLResponse  = `{"data":{"namespace":{"workItems":{"nodes":[{"id":"gid://gitlab/WorkItem/10","iid":"10","workItemType":{"name":"Issue"},"state":"OPEN","title":"ActionSpec test","author":{"username":"dev"},"widgets":[]}],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}}`
	actionSpecWorkItemCreateGraphQLResponse = `{"data":{"workItemCreate":{"workItem":{"id":"gid://gitlab/WorkItem/10","iid":"10","workItemType":{"name":"Issue"},"state":"OPEN","title":"ActionSpec test","author":{"username":"dev"},"widgets":[]}}}}`
	actionSpecWorkItemUpdateGraphQLResponse = `{"data":{"workItemUpdate":{"workItem":{"id":"gid://gitlab/WorkItem/10","iid":"10","workItemType":{"name":"Issue"},"state":"OPEN","title":"ActionSpec updated","author":{"username":"dev"},"widgets":[]}}}}`
	actionSpecWorkItemDeleteGraphQLResponse = `{"data":{"workItemDelete":{"errors":[]}}}`
)

// TestActionSpecs_CallAllRoutes exercises every work item tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := workItemSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, workItemActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_get_work_item", map[string]any{"full_path": testProjectPath, "work_item_iid": 10}},
		{"gitlab_list_work_items", map[string]any{"full_path": testProjectPath}},
		{"gitlab_create_work_item", map[string]any{"full_path": testProjectPath, "work_item_type_id": testTypeGID, "title": "Test"}},
		{"gitlab_update_work_item", map[string]any{"full_path": testProjectPath, "work_item_iid": 10, "title": "Updated"}},
		{"gitlab_delete_work_item", map[string]any{"full_path": testProjectPath, "work_item_iid": 10}},
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

// TestActionSpecs_DeleteOutput verifies the delete route preserves its success message.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	byTool := workItemSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, workItemActionHandler())))

	result, err := byTool["gitlab_delete_work_item"].Route.Handler(t.Context(), map[string]any{"full_path": testProjectPath, "work_item_iid": 10})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_delete_work_item) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_delete_work_item) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted work item #10 from ns/proj." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestActionSpecs_DeleteError verifies delete route failures propagate.
func TestActionSpecs_DeleteError(t *testing.T) {
	byTool := workItemSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusForbidden, `{"errors":[{"message":"server error"}]}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))))

	_, err := byTool["gitlab_delete_work_item"].Route.Handler(t.Context(), map[string]any{"full_path": testProjectPath, "work_item_iid": 10})
	if err == nil {
		t.Fatal("expected route error")
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
	byTool := workItemSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_delete_work_item"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test work item destructive confirmation.",
		Icons:       toolutil.IconIssue,
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
		Name:      "gitlab_delete_work_item",
		Arguments: map[string]any{"full_path": testProjectPath, "work_item_iid": 10},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

func workItemActionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}

		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			testutil.RespondJSON(w, http.StatusBadRequest, `{"errors":[{"message":"bad body"}]}`)
			return
		}
		body := string(bodyBytes)

		switch {
		case strings.Contains(body, "workItemCreate"):
			testutil.RespondJSON(w, http.StatusOK, actionSpecWorkItemCreateGraphQLResponse)
		case strings.Contains(body, "workItemUpdate"):
			testutil.RespondJSON(w, http.StatusOK, actionSpecWorkItemUpdateGraphQLResponse)
		case strings.Contains(body, "workItemDelete"):
			testutil.RespondJSON(w, http.StatusOK, actionSpecWorkItemDeleteGraphQLResponse)
		case strings.Contains(body, "workItems"):
			testutil.RespondJSON(w, http.StatusOK, actionSpecWorkItemsListGraphQLResponse)
		case strings.Contains(body, "workItem"):
			testutil.RespondJSON(w, http.StatusOK, actionSpecWorkItemGraphQLResponse)
		default:
			testutil.RespondJSON(w, http.StatusOK, actionSpecWorkItemGraphQLResponse)
		}
	})
}

func workItemSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
