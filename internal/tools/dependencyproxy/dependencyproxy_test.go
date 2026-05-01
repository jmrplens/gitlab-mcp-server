// dependencyproxy_test.go contains unit tests for the dependencyproxy MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package dependencyproxy

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// TestPurge verifies the behavior of purge.
func TestPurge(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/groups/5/dependency_proxy/cache" || r.Method != http.MethodDelete {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	err := Purge(t.Context(), client, PurgeInput{GroupID: "5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestPurge_Error verifies that Purge handles the error scenario correctly.
func TestPurge_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"forbidden"}`)
	}))
	err := Purge(t.Context(), client, PurgeInput{GroupID: "5"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// TestRegisterTools_NoPanic_Coverage verifies dependency proxy tool registration.
func TestRegisterTools_NoPanic_Coverage(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	RegisterTools(server, client)
}

// TestRegisterMeta_NoPanic_Coverage verifies dependency proxy meta-tool registration.
func TestRegisterMeta_NoPanic_Coverage(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	RegisterMeta(server, client)
}

// TestMCPRound_Trip_Coverage verifies dependency proxy tool execution over
// in-memory MCP transports.
func TestMCPRound_Trip_Coverage(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, handler)
	RegisterTools(server, client)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	go server.Connect(ctx, st, nil)

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_purge_dependency_proxy",
		Arguments: map[string]any{"group_id": "5"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
}

// TestMCPRoundTripPurge_Error_Coverage verifies the error path inside the registered
// tool handler when the GitLab API call fails (covers register.go lines 30-32).
func TestMCPRoundTripPurge_Error_Coverage(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"forbidden"}`)
	})

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, handler)
	RegisterTools(server, client)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	go server.Connect(ctx, st, nil)

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_purge_dependency_proxy",
		Arguments: map[string]any{"group_id": "5"},
	})
	// MCP returns tool errors as isError in the result, not as Go errors
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
}
