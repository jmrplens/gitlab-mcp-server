package epicissues

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const registerEpicIssueListJSON = `[{"id":42,"iid":1,"title":"Test Issue","state":"opened","epic_issue_id":10}]`
const registerEpicIssueAssignJSON = `{"id":10,"epic":{"id":1},"issue":{"id":42}}`
const registerEpicIssueUpdateJSON = `[{"id":42,"iid":1,"title":"Test Issue","state":"opened","epic_issue_id":10}]`

// TestRegisterTools_NoPanic verifies that RegisterTools registers all epic-issue
// tools without panicking.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// TestRegisterTools_CallThroughMCP verifies all 4 epic-issue tools can be called
// through MCP in-memory transport, covering every handler closure in register.go.
func TestRegisterTools_CallThroughMCP(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/issues"):
			testutil.RespondJSON(w, http.StatusOK, registerEpicIssueListJSON)
		case r.Method == http.MethodPost:
			testutil.RespondJSON(w, http.StatusCreated, registerEpicIssueAssignJSON)
		case r.Method == http.MethodDelete:
			testutil.RespondJSON(w, http.StatusOK, registerEpicIssueAssignJSON)
		case r.Method == http.MethodPut:
			testutil.RespondJSON(w, http.StatusOK, registerEpicIssueUpdateJSON)
		default:
			http.NotFound(w, r)
		}
	})
	client := testutil.NewTestClient(t, mux)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_epic_issue_list", map[string]any{"group_id": "mygroup", "epic_iid": 1}},
		{"gitlab_epic_issue_assign", map[string]any{"group_id": "mygroup", "epic_iid": 1, "issue_id": 42}},
		{"gitlab_epic_issue_remove", map[string]any{"group_id": "mygroup", "epic_iid": 1, "epic_issue_id": 10}},
		{"gitlab_epic_issue_update", map[string]any{"group_id": "mygroup", "epic_iid": 1, "epic_issue_id": 10}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("CallTool(%s) returned nil", tt.name)
			}
		})
	}
}

// TestMarkdownInit_Registry verifies the init() markdown formatters are registered.
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
