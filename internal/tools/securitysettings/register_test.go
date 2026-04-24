// register_test.go contains integration tests for the security settings tool
// closures in register.go. Tests exercise mutation error paths via an
// in-memory MCP session with a mock GitLab API.

package securitysettings

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const registerProjectSecJSON = `{
	"project_id": 42,
	"auto_fix_container_scanning": false,
	"auto_fix_dast": false,
	"auto_fix_dependency_scanning": true,
	"auto_fix_sast": true,
	"continuous_vulnerability_scans_enabled": false,
	"container_scanning_for_registry_enabled": true,
	"secret_push_protection_enabled": true
}`

const registerGroupSecJSON = `{
	"secret_push_protection_enabled": true
}`

// TestRegisterTools_NoPanic verifies that RegisterTools registers all security
// settings tools without panicking.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// TestRegisterTools_CallThroughMCP verifies all registered security settings tools
// can be called through MCP in-memory transport, covering the handler closures.
func TestRegisterTools_CallThroughMCP(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testutil.RespondJSON(w, http.StatusOK, registerProjectSecJSON)
		case http.MethodPatch, http.MethodPut:
			testutil.RespondJSON(w, http.StatusOK, registerGroupSecJSON)
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
		{"gitlab_get_project_security_settings", map[string]any{"project_id": "42"}},
		{"gitlab_update_project_secret_push_protection", map[string]any{"project_id": "42", "secret_push_protection_enabled": true}},
		{"gitlab_update_group_secret_push_protection", map[string]any{"group_id": "my-group", "secret_push_protection_enabled": true}},
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

// TestToProjectOutput_Nil verifies that toProjectOutput handles nil input
// gracefully by returning a zero-value ProjectOutput.
func TestToProjectOutput_Nil(t *testing.T) {
	out := toProjectOutput(nil)
	if out.ProjectID != 0 {
		t.Errorf("expected zero ProjectID for nil input, got %d", out.ProjectID)
	}
}

// TestToGroupOutput_Nil verifies that toGroupOutput handles nil input
// gracefully by returning a zero-value GroupOutput.
func TestToGroupOutput_Nil(t *testing.T) {
	out := toGroupOutput(nil)
	if out.SecretPushProtectionEnabled {
		t.Error("expected false SecretPushProtectionEnabled for nil input")
	}
	if len(out.Errors) != 0 {
		t.Errorf("expected empty errors for nil input, got %d", len(out.Errors))
	}
}
