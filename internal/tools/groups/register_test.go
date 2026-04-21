package groups

import (
	"context"
	"net/http"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// TestRegisterTools_ConfirmDeclined covers the ConfirmAction early-return
// branches in group delete and webhook delete handlers when the user declines.
func TestRegisterTools_ConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "decline"}, nil
		},
	})
	session, connectErr := mcpClient.Connect(ctx, ct, nil)
	if connectErr != nil {
		t.Fatalf("client connect: %v", connectErr)
	}
	t.Cleanup(func() { session.Close() })

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_group_delete", map[string]any{"group_id": "42"}},
		{"gitlab_group_hook_delete", map[string]any{"group_id": "42", "hook_id": 1}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool error: %v", err)
			}
			if result == nil {
				t.Fatal("expected non-nil result for declined confirmation")
			}
		})
	}
}

// TestRegisterTools_GetNotFound covers the NotFoundResult branch in the
// gitlab_group_get handler when the API returns 404.
func TestRegisterTools_GetNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Group Not Found"}`)
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

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_group_get",
		Arguments: map[string]any{"group_id": "999"},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected IsError result for 404")
	}
}

// TestMemberToOutput_OptionalFields covers the optional field branches
// (CreatedAt, ExpiresAt, GroupSAMLIdentity, MemberRole) in MemberToOutput.
func TestMemberToOutput_OptionalFields(t *testing.T) {
	now := time.Now()
	expires := gl.ISOTime(now)
	m := &gl.GroupMember{
		ID:          1,
		Username:    "user",
		AccessLevel: gl.DeveloperPermissions,
		CreatedAt:   &now,
		ExpiresAt:   &expires,
		GroupSAMLIdentity: &gl.GroupMemberSAMLIdentity{
			Provider: "saml-provider",
		},
		MemberRole: &gl.MemberRole{
			Name: "custom-role",
		},
	}
	out := MemberToOutput(m)
	if out.CreatedAt == "" {
		t.Error("expected non-empty CreatedAt")
	}
	if out.ExpiresAt == "" {
		t.Error("expected non-empty ExpiresAt")
	}
	if out.GroupSAMLProvider != "saml-provider" {
		t.Errorf("GroupSAMLProvider = %q, want %q", out.GroupSAMLProvider, "saml-provider")
	}
	if out.MemberRoleName != "custom-role" {
		t.Errorf("MemberRoleName = %q, want %q", out.MemberRoleName, "custom-role")
	}
}

// TestRegisterTools_ErrorPaths covers the error branches in RegisterTools
// closures for tools that wrap API errors (non-destructive and non-404 paths).
func TestRegisterTools_ErrorPaths(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
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
		{"gitlab_group_list", map[string]any{}},
		{"gitlab_group_members_list", map[string]any{"group_id": "42"}},
		{"gitlab_group_create", map[string]any{"name": "x", "path": "x"}},
		{"gitlab_group_update", map[string]any{"group_id": "42"}},
		{"gitlab_group_restore", map[string]any{"group_id": "42"}},
		{"gitlab_group_archive", map[string]any{"group_id": "42"}},
		{"gitlab_group_unarchive", map[string]any{"group_id": "42"}},
		{"gitlab_group_search", map[string]any{"search": "x"}},
		{"gitlab_group_transfer_project", map[string]any{"group_id": "42", "project_id": 1}},
		{"gitlab_group_projects", map[string]any{"group_id": "42"}},
		{"gitlab_group_hook_list", map[string]any{"group_id": "42"}},
		{"gitlab_group_hook_get", map[string]any{"group_id": "42", "hook_id": 1}},
		{"gitlab_group_hook_add", map[string]any{"group_id": "42", "url": "http://x"}},
		{"gitlab_group_hook_edit", map[string]any{"group_id": "42", "hook_id": 1}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool error: %v", err)
			}
			if result == nil || !result.IsError {
				t.Fatal("expected IsError result for server error response")
			}
		})
	}
}
