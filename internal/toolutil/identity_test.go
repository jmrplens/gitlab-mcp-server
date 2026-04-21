package toolutil_test

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

func TestUserIdentity_IsAuthenticated(t *testing.T) {
	tests := []struct {
		name string
		user toolutil.UserIdentity
		want bool
	}{
		{"empty", toolutil.UserIdentity{}, false},
		{"with_user_id", toolutil.UserIdentity{UserID: "1"}, true},
		{"username_only", toolutil.UserIdentity{Username: "admin"}, false},
		{"both", toolutil.UserIdentity{UserID: "1", Username: "admin"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.user.IsAuthenticated(); got != tt.want {
				t.Errorf("IsAuthenticated() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIdentityToContext_RoundTrip(t *testing.T) {
	id := toolutil.UserIdentity{UserID: "42", Username: "admin"}
	ctx := toolutil.IdentityToContext(context.Background(), id)
	got := toolutil.IdentityFromContext(ctx)
	if got != id {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, id)
	}
}

func TestIdentityFromContext_Empty(t *testing.T) {
	got := toolutil.IdentityFromContext(context.Background())
	if got.IsAuthenticated() {
		t.Error("expected unauthenticated from empty context")
	}
}

func TestResolveIdentity_TokenInfoPriority(t *testing.T) {
	ctxID := toolutil.UserIdentity{UserID: "1", Username: "ctx-user"}
	ctx := toolutil.IdentityToContext(context.Background(), ctxID)

	req := &mcp.CallToolRequest{
		Extra: &mcp.RequestExtra{
			TokenInfo: &auth.TokenInfo{
				UserID: "99",
				Extra:  map[string]any{"username": "oauth-user"},
			},
		},
	}

	got := toolutil.ResolveIdentity(ctx, req)
	if got.UserID != "99" || got.Username != "oauth-user" {
		t.Errorf("expected TokenInfo identity, got %+v", got)
	}
}

func TestResolveIdentity_ContextFallback(t *testing.T) {
	ctxID := toolutil.UserIdentity{UserID: "1", Username: "stdio-user"}
	ctx := toolutil.IdentityToContext(context.Background(), ctxID)

	req := &mcp.CallToolRequest{}

	got := toolutil.ResolveIdentity(ctx, req)
	if got != ctxID {
		t.Errorf("expected context identity %+v, got %+v", ctxID, got)
	}
}

func TestResolveIdentity_NilRequest(t *testing.T) {
	ctxID := toolutil.UserIdentity{UserID: "5", Username: "from-ctx"}
	ctx := toolutil.IdentityToContext(context.Background(), ctxID)

	got := toolutil.ResolveIdentity(ctx, nil)
	if got != ctxID {
		t.Errorf("expected context identity for nil request, got %+v", got)
	}
}

func TestResolveIdentity_NoIdentity(t *testing.T) {
	got := toolutil.ResolveIdentity(context.Background(), &mcp.CallToolRequest{})
	if got.IsAuthenticated() {
		t.Error("expected unauthenticated when no identity source available")
	}
}

func TestResolveIdentity_StdioScenario(t *testing.T) {
	// Simulates stdio mode: identity injected into context at startup,
	// tool handler receives a request without TokenInfo.
	identity := toolutil.UserIdentity{UserID: "42", Username: "stdio-admin"}
	ctx := toolutil.IdentityToContext(context.Background(), identity)

	// Request with no Extra (stdio mode has no OAuth middleware)
	req := &mcp.CallToolRequest{}

	got := toolutil.ResolveIdentity(ctx, req)
	if got.UserID != "42" {
		t.Errorf("UserID = %q, want %q", got.UserID, "42")
	}
	if got.Username != "stdio-admin" {
		t.Errorf("Username = %q, want %q", got.Username, "stdio-admin")
	}
	if !got.IsAuthenticated() {
		t.Error("expected authenticated identity in stdio scenario")
	}
}
