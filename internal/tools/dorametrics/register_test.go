// Package dorametrics register_test exercises RegisterTools closures and the
// init() markdown registry formatter via MCP roundtrip and MarkdownForResult.
package dorametrics

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const registerMetricsJSON = `[{"date":"2026-01-01","value":42.5}]`

// TestRegisterTools_NoPanic2 verifies RegisterTools registers all tools without panicking
// (complementary to the existing NoPanic test in dorametrics_test.go).
func TestRegisterTools_NoPanic2(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// TestRegisterTools_CallThroughMCP verifies both DORA metrics tools can
// be called through MCP in-memory transport.
func TestRegisterTools_CallThroughMCP(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/dora/metrics") {
			testutil.RespondJSON(w, http.StatusOK, registerMetricsJSON)
		} else {
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
		{"gitlab_get_project_dora_metrics", map[string]any{"project_id": "42", "metric": "deployment_frequency"}},
		{"gitlab_get_group_dora_metrics", map[string]any{"group_id": "42", "metric": "deployment_frequency"}},
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

// TestMarkdownHints_Output verifies the init()-registered markdown formatter
// for Output produces non-nil content via MarkdownForResult.
func TestMarkdownHints_Output(t *testing.T) {
	md := toolutil.MarkdownForResult(Output{
		Metrics: []MetricOutput{{Date: "2026-01-01", Value: 42.5}},
	})
	if md == nil {
		t.Fatal("expected non-nil result from MarkdownForResult(Output{})")
	}
}
