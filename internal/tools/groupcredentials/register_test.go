package groupcredentials

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const registerPATJSON = `[{"id":99,"name":"test-pat","scopes":["api"],"state":"active","created_at":"2026-01-01T00:00:00Z","expires_at":"2026-01-01"}]`
const registerSSHKeyJSON = `[{"id":5,"title":"test-key","key":"ssh-rsa AAAA...","created_at":"2026-01-01T00:00:00Z"}]`

// TestRegisterTools_NoPanic verifies that RegisterTools registers all group
// credential tools without panicking.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// TestRegisterTools_CallThroughMCP verifies all 4 group credential tools can be
// called through MCP in-memory transport, covering handler closures in register.go.
func TestRegisterTools_CallThroughMCP(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.Contains(path, "/personal_access_tokens"):
			testutil.RespondJSON(w, http.StatusOK, registerPATJSON)
		case r.Method == http.MethodGet && strings.Contains(path, "/ssh_keys"):
			testutil.RespondJSON(w, http.StatusOK, registerSSHKeyJSON)
		case r.Method == http.MethodDelete:
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
	session, connectErr := mcpClient.Connect(ctx, ct, nil)
	if connectErr != nil {
		t.Fatalf("client connect: %v", connectErr)
	}
	t.Cleanup(func() { session.Close() })

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_list_group_personal_access_tokens", map[string]any{"group_id": "mygroup"}},
		{"gitlab_list_group_ssh_keys", map[string]any{"group_id": "mygroup"}},
		{"gitlab_revoke_group_personal_access_token", map[string]any{"group_id": "mygroup", "token_id": 99}},
		{"gitlab_delete_group_ssh_key", map[string]any{"group_id": "mygroup", "key_id": 5}},
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

// TestRegisterTools_ConfirmDeclined covers the ConfirmAction early-return
// branches in revoke PAT and delete SSH key handlers when user declines.
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
		{"gitlab_revoke_group_personal_access_token", map[string]any{"group_id": "g", "token_id": 1}},
		{"gitlab_delete_group_ssh_key", map[string]any{"group_id": "g", "key_id": 1}},
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

// TestToPATOutput_OptionalFields covers the optional timestamp branches
// (LastUsedAt, ExpiresAt) in toPATOutput and the nil input guard.
func TestToPATOutput_OptionalFields(t *testing.T) {
	// Nil guard
	out := toPATOutput(nil)
	if out.ID != 0 {
		t.Error("expected zero ID for nil input")
	}

	now := time.Now()
	expires := gl.ISOTime(now)
	pat := &gl.GroupPersonalAccessToken{
		ID:         1,
		Name:       "test",
		LastUsedAt: &now,
		ExpiresAt:  &expires,
	}
	out = toPATOutput(pat)
	if out.LastUsedAt == "" {
		t.Error("expected non-empty LastUsedAt")
	}
	if out.ExpiresAt == "" {
		t.Error("expected non-empty ExpiresAt")
	}
	if out.State != "inactive" {
		t.Errorf("State = %q, want %q for non-active non-revoked token", out.State, "inactive")
	}
}

// TestToSSHKeyOutput_OptionalFields covers the optional timestamp branches
// (ExpiresAt, LastUsedAt) in toSSHKeyOutput.
func TestToSSHKeyOutput_OptionalFields(t *testing.T) {
	nilOut := toSSHKeyOutput(nil)
	if nilOut.ID != 0 {
		t.Error("expected zero ID for nil input")
	}

	now := time.Now()
	k := &gl.GroupSSHKey{
		ID:         1,
		Title:      "test",
		CreatedAt:  &now,
		ExpiresAt:  &now,
		LastUsedAt: &now,
	}
	out := toSSHKeyOutput(k)
	if out.CreatedAt == "" {
		t.Error("expected non-empty CreatedAt")
	}
	if out.ExpiresAt == "" {
		t.Error("expected non-empty ExpiresAt")
	}
	if out.LastUsedAt == "" {
		t.Error("expected non-empty LastUsedAt")
	}
}

// TestRegisterTools_ErrorPaths covers the error branches in RegisterTools closures
// for tools that wrap API errors when the server returns 500.
func TestRegisterTools_ErrorPaths(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"server error"}`)
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
		{"gitlab_list_group_personal_access_tokens", map[string]any{"group_id": "42"}},
		{"gitlab_list_group_ssh_keys", map[string]any{"group_id": "42"}},
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
