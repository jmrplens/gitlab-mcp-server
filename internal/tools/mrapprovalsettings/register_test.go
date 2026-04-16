package mrapprovalsettings

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const approvalSettingsJSON = `{
	"allow_author_approval":true,
	"allow_committer_approval":true,
	"allow_overrides_to_approver_list_per_merge_request":false,
	"retain_approvals_on_push":true,
	"selective_code_owner_removals":false,
	"require_password_to_approve":false,
	"require_reauthentication_to_approve":false
}`

// TestRegisterTools_NoPanic verifies that RegisterTools registers all MR approval
// settings tools without panicking.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// TestRegisterTools_CallThroughMCP verifies all registered MR approval settings tools
// can be called through MCP in-memory transport, covering the handler closures.
func TestRegisterTools_CallThroughMCP(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testutil.RespondJSON(w, http.StatusOK, approvalSettingsJSON)
		case http.MethodPut:
			testutil.RespondJSON(w, http.StatusOK, approvalSettingsJSON)
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
		{"gitlab_get_group_mr_approval_settings", map[string]any{"group_id": "42"}},
		{"gitlab_update_group_mr_approval_settings", map[string]any{"group_id": "42"}},
		{"gitlab_get_project_mr_approval_settings", map[string]any{"project_id": "42"}},
		{"gitlab_update_project_mr_approval_settings", map[string]any{"project_id": "42"}},
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

// TestFormatOutputMarkdown_EmptyScope verifies that FormatOutputMarkdown handles
// an empty scope string correctly, covering the init() function's registered formatter.
func TestFormatOutputMarkdown_EmptyScope(t *testing.T) {
	out := Output{
		AllowAuthorApproval:             SettingOutput{Value: true, Locked: false},
		AllowCommitterApproval:          SettingOutput{Value: false, Locked: true, InheritedFrom: "group"},
		RequireReauthenticationToApprove: SettingOutput{Value: true, Locked: false},
	}
	md := FormatOutputMarkdown(out, "")
	if md == "" {
		t.Fatal("expected non-empty markdown output for empty scope")
	}
}
