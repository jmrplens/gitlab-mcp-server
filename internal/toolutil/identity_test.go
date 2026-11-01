// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// identity_test.go contains unit tests for user identity resolution helpers:
// UserIdentity.IsAuthenticated, IdentityToContext/IdentityFromContext
// round-tripping, and ResolveIdentity priority (OAuth TokenInfo > context).
package toolutil_test

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/oauth"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestUserIdentity_IsAuthenticated verifies that IsAuthenticated reports true
// only when UserID is non-empty. Table-driven subtests cover an empty identity,
// an identity with only UserID set, one with only Username set, and one with
// both fields set. It asserts that the return value matches the expected bool.
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

// TestIdentityToContext_RoundTrip verifies that a UserIdentity stored in a
// context via IdentityToContext can be retrieved unchanged by IdentityFromContext.
// It asserts that all fields (UserID, Username) are preserved through the
// context round-trip.
func TestIdentityToContext_RoundTrip(t *testing.T) {
	id := toolutil.UserIdentity{UserID: "42", Username: "admin"}
	ctx := toolutil.IdentityToContext(context.Background(), id)
	got := toolutil.IdentityFromContext(ctx)
	if got != id {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, id)
	}
}

// TestIdentityFromContext_Empty verifies that IdentityFromContext returns an
// unauthenticated zero-value UserIdentity when no identity has been stored
// in the context.
func TestIdentityFromContext_Empty(t *testing.T) {
	got := toolutil.IdentityFromContext(context.Background())
	if got.IsAuthenticated() {
		t.Error("expected unauthenticated from empty context")
	}
}

// TestResolveIdentity_TokenInfoPriority verifies that ResolveIdentity prefers
// the OAuth TokenInfo from the MCP request over a context-stored identity.
// The test injects a context identity (UserID "1") and a request with TokenInfo
// (UserID "99") and asserts that the resolved identity matches the token.
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

// TestResolveIdentity_TokenInfo_KeepsTheInstanceItWasVerifiedAgainst pins the
// tenant of an OAuth tool call as the pair (instance, user id), which issue 953
// found it was not (F-28). The verifier's admission carries the instance it was
// verified against under oauth.InstanceKey, and ResolveIdentity used to return
// the id and the name alone from it, so user 7 of two published instances
// resolved to one identity while the gate's context identity beside it named
// the instance. An admission that carries no instance resolves to none, rather
// than borrowing the context's.
func TestResolveIdentity_TokenInfo_KeepsTheInstanceItWasVerifiedAgainst(t *testing.T) {
	admitted := func(instance any) *mcp.CallToolRequest {
		extra := map[string]any{"username": "seven"}
		if instance != nil {
			extra[oauth.InstanceKey] = instance
		}
		return &mcp.CallToolRequest{Extra: &mcp.RequestExtra{TokenInfo: &auth.TokenInfo{UserID: "7", Extra: extra}}}
	}
	ctx := toolutil.IdentityToContext(context.Background(),
		toolutil.UserIdentity{UserID: "7", Username: "seven", Instance: "https://context.example.com"})

	first := toolutil.ResolveIdentity(ctx, admitted("https://gitlab-a.example.com"))
	second := toolutil.ResolveIdentity(ctx, admitted("https://gitlab-b.example.com"))
	if want := (toolutil.UserIdentity{UserID: "7", Username: "seven", Instance: "https://gitlab-a.example.com"}); first != want {
		t.Errorf("ResolveIdentity() = %+v, want %+v", first, want)
	}
	if first == second {
		t.Errorf("user 7 of two instances resolved to one identity %+v", first)
	}
	for name, instance := range map[string]any{"absent": nil, "not a string": 7} {
		t.Run(name, func(t *testing.T) {
			if got := toolutil.ResolveIdentity(ctx, admitted(instance)); got.Instance != "" || got.UserID != "7" {
				t.Errorf("ResolveIdentity() = %+v, want user 7 with no instance", got)
			}
		})
	}
}

// TestResolveIdentity_ContextFallback verifies that ResolveIdentity falls back
// to the context-stored identity when the MCP request contains no TokenInfo.
// It asserts that the returned identity matches the context value exactly.
func TestResolveIdentity_ContextFallback(t *testing.T) {
	ctxID := toolutil.UserIdentity{UserID: "1", Username: "stdio-user"}
	ctx := toolutil.IdentityToContext(context.Background(), ctxID)

	req := &mcp.CallToolRequest{}

	got := toolutil.ResolveIdentity(ctx, req)
	if got != ctxID {
		t.Errorf("expected context identity %+v, got %+v", ctxID, got)
	}
}

// TestResolveIdentity_NilRequest verifies that ResolveIdentity handles a nil
// request gracefully by falling back to the context-stored identity.
func TestResolveIdentity_NilRequest(t *testing.T) {
	ctxID := toolutil.UserIdentity{UserID: "5", Username: "from-ctx"}
	ctx := toolutil.IdentityToContext(context.Background(), ctxID)

	got := toolutil.ResolveIdentity(ctx, nil)
	if got != ctxID {
		t.Errorf("expected context identity for nil request, got %+v", got)
	}
}

// TestResolveIdentity_ExtraWithoutTokenInfo_FallsBackToTheContext verifies a
// request that carries request extras but no token info, which is what a stdio
// call looks like, is answered from the identity stored in the context.
func TestResolveIdentity_ExtraWithoutTokenInfo_FallsBackToTheContext(t *testing.T) {
	ctxID := toolutil.UserIdentity{UserID: "6", Username: "from-ctx"}
	ctx := toolutil.IdentityToContext(context.Background(), ctxID)

	got := toolutil.ResolveIdentity(ctx, &mcp.CallToolRequest{Extra: &mcp.RequestExtra{}})
	if got != ctxID {
		t.Errorf("ResolveIdentity() = %+v, want the context identity %+v", got, ctxID)
	}
}

// TestResolveIdentity_NoIdentity verifies that ResolveIdentity returns an
// unauthenticated zero-value identity when neither the context nor the request
// carries any identity information.
func TestResolveIdentity_NoIdentity(t *testing.T) {
	got := toolutil.ResolveIdentity(context.Background(), &mcp.CallToolRequest{})
	if got.IsAuthenticated() {
		t.Error("expected unauthenticated when no identity source available")
	}
}

// TestResolveIdentity_StdioScenario verifies the stdio mode identity path:
// an identity injected into context at startup (no TokenInfo in request)
// is resolved correctly. The test asserts that UserID, Username, and
// IsAuthenticated all match the injected identity.
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
