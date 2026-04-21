// Package groupepicboards register_test exercises all RegisterTools closures
// via MCP in-memory transport and covers the nil label/list branches in toOutput.
package groupepicboards

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const registerBoardJSON = `{"id":1,"name":"Board","labels":[{"name":"bug"},null],"lists":[{"id":1,"position":0,"label":{"id":10,"name":"To Do"}},{"id":2,"position":1,"label":null},null]}`
const registerBoardsJSON = `[{"id":1,"name":"Board","labels":[{"name":"bug"}],"lists":[{"id":1,"position":0,"label":{"id":10,"name":"To Do"}}]}]`

// TestRegisterTools_NoPanic verifies RegisterTools registers all tools without panicking.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// TestRegisterTools_CallThroughMCP verifies both group epic board tools can
// be called through MCP in-memory transport, including nil label/list edge cases
// in toOutput (the board JSON includes nulls in labels and lists arrays).
func TestRegisterTools_CallThroughMCP(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/epic_boards"):
			testutil.RespondJSON(w, http.StatusOK, registerBoardsJSON)
		case r.Method == http.MethodGet && strings.Contains(path, "/epic_boards/"):
			// Return board with nil entries to cover nil-check branches in toOutput
			testutil.RespondJSON(w, http.StatusOK, registerBoardJSON)
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
	session, connectErr := mcpClient.Connect(ctx, ct, nil)
	if connectErr != nil {
		t.Fatalf("client connect: %v", connectErr)
	}
	t.Cleanup(func() { session.Close() })

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_group_epic_board_list", map[string]any{"group_id": "42"}},
		{"gitlab_group_epic_board_get", map[string]any{"group_id": "42", "board_id": 1}},
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
