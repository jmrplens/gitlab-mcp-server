// register_test.go contains integration tests for the epic issue link tool
// closures in register.go. Tests exercise mutation error paths via an
// in-memory MCP session with a mock GitLab API.
package epicissues

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

func TestRegisterTools_CallThroughMCP(t *testing.T) {
	handler := graphqlMux(map[string]http.HandlerFunc{
		"WorkItemWidgetHierarchy": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, gqlChildrenData)
		},
		"workItem(iid": func(w http.ResponseWriter, r *http.Request) {
			vars, _ := testutil.ParseGraphQLVariables(r)
			fp, _ := vars["fullPath"].(string)
			if fp == "child-group/child-project" {
				testutil.RespondGraphQL(w, http.StatusOK, `{"namespace":{"workItem":{"id":"gid://gitlab/WorkItem/42"}}}`)
			} else {
				testutil.RespondGraphQL(w, http.StatusOK, gqlWorkItemGIDData)
			}
		},
		"workItemUpdate(": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, gqlAddChildData)
		},
	})

	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, connectErr := mcpClient.Connect(ctx, ct, nil)
	if connectErr != nil {
		t.Fatalf("client connect: %v", connectErr)
	}
	t.Cleanup(func() { session.Close() })

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_epic_issue_list", map[string]any{"full_path": testFullPath, "iid": 1}},
		{"gitlab_epic_issue_assign", map[string]any{"full_path": testFullPath, "iid": 1, "child_project_path": "child-group/child-project", "child_iid": 42}},
		{"gitlab_epic_issue_remove", map[string]any{"full_path": testFullPath, "iid": 1, "child_project_path": "child-group/child-project", "child_iid": 42}},
		{"gitlab_epic_issue_update", map[string]any{"full_path": testFullPath, "iid": 1, "child_id": "gid://gitlab/WorkItem/10", "adjacent_id": "gid://gitlab/WorkItem/20", "relative_position": "BEFORE"}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("CallTool(%s) returned nil result", tt.name)
			}
			if result.IsError {
				t.Fatalf("CallTool(%s) returned IsError=true", tt.name)
			}
		})
	}
}

func TestMarkdownInit_Registry(t *testing.T) {
	out := toolutil.MarkdownForResult(ListOutput{})
	if out == nil {
		t.Fatal("expected non-nil result for ListOutput")
	}
	out2 := toolutil.MarkdownForResult(AssignOutput{})
	if out2 == nil {
		t.Fatal("expected non-nil result for AssignOutput")
	}
}
