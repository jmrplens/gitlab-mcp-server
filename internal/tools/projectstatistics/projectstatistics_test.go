// projectstatistics_test.go contains unit tests for the project statistics MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package projectstatistics

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// TestGet verifies the behavior of get.
func TestGet(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/statistics" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"fetches":{"total":42,"days":[{"count":5,"date":"2024-01-01"}]}}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Get(t.Context(), client, GetInput{ProjectID: "1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.TotalFetches != 42 {
		t.Errorf("TotalFetches = %d", out.TotalFetches)
	}
	if len(out.Days) != 1 {
		t.Fatalf("Days len = %d", len(out.Days))
	}
}

// TestGet_Error verifies that Get handles the error scenario correctly.
func TestGet_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	_, err := Get(t.Context(), client, GetInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestFormatMarkdown verifies the behavior of format markdown.
func TestFormatMarkdown(t *testing.T) {
	md := FormatMarkdown(GetOutput{TotalFetches: 42, Days: []DayStat{{Date: "2024-01-01", Count: 5}}})
	if !strings.Contains(md, "42") || !strings.Contains(md, "1 Jan 2024") {
		t.Error("missing content")
	}
}

// TestFormatMarkdown_Empty verifies the formatter handles empty days.
func TestFormatMarkdown_Empty(t *testing.T) {
	md := FormatMarkdown(GetOutput{TotalFetches: 0})
	if !strings.Contains(md, "0") {
		t.Error("expected zero total fetches")
	}
}

// TestGet_MissingProjectID verifies Get returns error for empty project_id.
func TestGet_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := Get(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestRegisterTools_NoPanic verifies that RegisterTools does not panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// TestRegisterMeta_NoPanic verifies that RegisterMeta does not panic.
func TestRegisterMeta_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)
}

// TestRegisterTools_CallThroughMCP verifies all registered tools can be called
// through MCP in-memory transport.
func TestRegisterTools_CallThroughMCP(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"fetches":{"total":42,"days":[{"count":5,"date":"2024-01-01"}]}}`)
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
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_get_project_statistics",
		Arguments: map[string]any{"project_id": "1"},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}
