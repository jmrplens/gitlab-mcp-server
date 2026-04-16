package geo

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const registerGeoJSON = `{"id":1,"name":"primary","url":"https://primary.example.com","primary":true,"enabled":true,"internal_url":"https://primary.internal"}`
const geoStatusJSON = `{"geo_node_id":1,"healthy":true,"health":"Healthy","health_status":"Healthy","replication_slots_used_count":1,"replication_slots_count":1}`

// TestRegisterTools_NoPanic verifies that RegisterTools registers all Geo site
// tools without panicking.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// TestRegisterTools_CallThroughMCP verifies all registered Geo site tools
// can be called through MCP in-memory transport, covering the handler closures.
func TestRegisterTools_CallThroughMCP(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testutil.RespondJSON(w, http.StatusOK, `[`+registerGeoJSON+`]`)
		case http.MethodPost:
			testutil.RespondJSON(w, http.StatusCreated, registerGeoJSON)
		case http.MethodPut:
			testutil.RespondJSON(w, http.StatusOK, registerGeoJSON)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
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
		{"gitlab_create_geo_site", map[string]any{"name": "secondary", "url": "https://secondary.example.com"}},
		{"gitlab_list_geo_sites", map[string]any{}},
		{"gitlab_get_geo_site", map[string]any{"id": 1}},
		{"gitlab_edit_geo_site", map[string]any{"id": 1, "enabled": true}},
		{"gitlab_delete_geo_site", map[string]any{"id": 1}},
		{"gitlab_repair_geo_site", map[string]any{"id": 1}},
		{"gitlab_list_status_all_geo_sites", map[string]any{}},
		{"gitlab_get_status_geo_site", map[string]any{"id": 1}},
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
