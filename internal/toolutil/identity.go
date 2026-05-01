package toolutil

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// UserIdentity holds the authenticated user's identity.
// Populated from OAuth TokenInfo (HTTP modes) or from the startup-resolved
// identity stored in context (stdio mode).
type UserIdentity struct {
	UserID   string
	Username string
}

// IsAuthenticated returns true if the identity contains a non-empty UserID.
func (u UserIdentity) IsAuthenticated() bool {
	return u.UserID != ""
}

// identityContextKey is the context key for storing a startup-resolved
// UserIdentity (used by stdio mode where req.Extra.TokenInfo is unavailable).
type identityContextKey struct{}

// IdentityToContext stores a UserIdentity in the context.
// Used at startup in stdio mode to make the identity available to all tool handlers.
func IdentityToContext(ctx context.Context, id UserIdentity) context.Context {
	return context.WithValue(ctx, identityContextKey{}, id)
}

// IdentityFromContext retrieves the UserIdentity stored in the context.
// Returns a zero-value UserIdentity if none was stored.
func IdentityFromContext(ctx context.Context) UserIdentity {
	id, _ := ctx.Value(identityContextKey{}).(UserIdentity)
	return id
}

// ResolveIdentity returns the authenticated user's identity by checking
// two sources in priority order:
//  1. req.Extra.TokenInfo (populated by SDK in HTTP modes via RequireBearerToken)
//  2. Context-stored identity (populated at startup in stdio mode)
//
// Returns a zero-value UserIdentity if neither source has identity.
func ResolveIdentity(ctx context.Context, req *mcp.CallToolRequest) UserIdentity {
	if req != nil && req.Extra != nil && req.Extra.TokenInfo != nil {
		info := req.Extra.TokenInfo
		username, _ := info.Extra["username"].(string)
		return UserIdentity{
			UserID:   info.UserID,
			Username: username,
		}
	}
	return IdentityFromContext(ctx)
}
