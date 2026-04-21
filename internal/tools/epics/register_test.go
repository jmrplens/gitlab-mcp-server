package epics

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const registerWorkItemJSON = `{"data":{"namespace":{"workItem":` + workItemEpicJSON + `}}}`
const registerListJSON = `{"data":{"namespace":{"workItems":{"nodes":[` + workItemEpicJSON + `],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}}`
const registerCreateJSON = `{"data":{"workItemCreate":{"workItem":` + workItemEpicJSON + `}}}`
const registerGIDJSON = `{"data":{"namespace":{"workItem":{"id":"gid://gitlab/WorkItem/101"}}}}`
const registerUpdateJSON = `{"data":{"workItemUpdate":{"workItem":` + workItemEpicJSON + `}}}`
const registerDeleteJSON = `{"data":{"workItemDelete":{"errors":[]}}}`
const registerEpicLinksJSON = `[` + epicLinkJSON + `]`

func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

func TestRegisterTools_CallThroughMCP(t *testing.T) {
	var graphQLCalls atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// GetLinks is REST (GET /groups/.../epics/.../epics)
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/epics/") {
			testutil.RespondJSON(w, http.StatusOK, registerEpicLinksJSON)
			return
		}
		// All other handlers use GraphQL (POST)
		if r.Method == http.MethodPost {
			n := graphQLCalls.Add(1)
			switch {
			case n <= 1:
				// List
				testutil.RespondJSON(w, http.StatusOK, registerListJSON)
			case n <= 2:
				// Get
				testutil.RespondJSON(w, http.StatusOK, registerWorkItemJSON)
			case n <= 3:
				// Create
				testutil.RespondJSON(w, http.StatusOK, registerCreateJSON)
			case n <= 4:
				// Update: GID resolution
				testutil.RespondJSON(w, http.StatusOK, registerGIDJSON)
			case n <= 5:
				// Update: mutation
				testutil.RespondJSON(w, http.StatusOK, registerUpdateJSON)
			case n <= 6:
				// Delete: GID resolution
				testutil.RespondJSON(w, http.StatusOK, registerGIDJSON)
			default:
				// Delete: mutation
				testutil.RespondJSON(w, http.StatusOK, registerDeleteJSON)
			}
			return
		}
		http.NotFound(w, r)
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
		{"gitlab_epic_list", map[string]any{"full_path": "my-group"}},
		{"gitlab_epic_get", map[string]any{"full_path": "my-group", "iid": float64(1)}},
		{"gitlab_epic_get_links", map[string]any{"full_path": "my-group", "iid": float64(1)}},
		{"gitlab_epic_create", map[string]any{"full_path": "my-group", "title": "New Epic"}},
		{"gitlab_epic_update", map[string]any{"full_path": "my-group", "iid": float64(1), "title": "Updated"}},
		{"gitlab_epic_delete", map[string]any{"full_path": "my-group", "iid": float64(1)}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool(%s) transport error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("CallTool(%s) returned nil result", tt.name)
			}
			if result.IsError {
				t.Errorf("CallTool(%s) returned error result: %v", tt.name, result.Content)
			}
		})
	}
}

func TestRegisterTools_DeleteError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
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

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_epic_delete",
		Arguments: map[string]any{"full_path": "my-group", "iid": float64(1)},
	})
	if err != nil {
		t.Fatalf("CallTool returned transport error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Error("expected error result from delete with failing backend")
	}
}

func TestRegisterTools_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
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

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_epic_delete",
		Arguments: map[string]any{"full_path": "my-group", "iid": float64(1), "_confirm": "no"},
	})
	if err != nil {
		t.Fatalf("CallTool returned transport error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Error("expected error result when confirmation is declined")
	}
}

// Package epics register_test exercises all RegisterTools closures
// via MCP in-memory transport, covering every handler wired in register.go.
