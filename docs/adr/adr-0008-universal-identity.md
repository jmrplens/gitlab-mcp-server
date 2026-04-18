---
title: "ADR-0008: Universal User Identity Across All MCP Transport Modes"
status: "Accepted"
date: "2026-04-18"
authors: "jmrplens"
tags: ["architecture", "decision", "identity", "security", "oauth"]
superseded_by: ""
---

# ADR-0008: Universal User Identity Across All MCP Transport Modes

## Status

**Accepted** — implements unified identity resolution across stdio, HTTP legacy, and HTTP OAuth transport modes.

## Context

The MCP server supports three transport modes: stdio (standalone binary), HTTP legacy (PRIVATE-TOKEN header), and HTTP OAuth (Bearer token with GitLab validation). Before this decision, only HTTP OAuth mode populated `req.Extra.TokenInfo` — the other two modes provided no per-request identity to tool handlers. This meant audit logging, access control decisions, and user-aware tool behavior were impossible in stdio and HTTP legacy modes.

### SDK Constraints

1. **`auth.tokenInfoKey` is unexported** (`type tokenInfoKey struct{}`): Only the SDK's `auth.RequireBearerToken` middleware can store `TokenInfo` in the HTTP request context. External code cannot set it directly.
2. **`StreamableHTTPHandler` reads TokenInfo from context** (`auth.TokenInfoFromContext(req.Context())`) and sets `req.Extra.TokenInfo`. If `TokenInfo` is present in context via `RequireBearerToken`, the SDK propagates it automatically.
3. **Stdio `req.Extra` is always nil**: The SDK's `StdioTransport` reads JSON-RPC from stdin and never populates `req.Extra`. There is no middleware layer for stdio.
4. **Context from `server.Run(ctx)` propagates to tool handlers**: Verified by tracing `server.Run(ctx)` → `server.Connect(ctx)` → tool handler dispatch. Values stored in the base context are accessible in handlers.

### Options Considered

#### Option 1: Custom context key for all modes (rejected)

Store identity via a custom context key in all modes, bypassing `req.Extra.TokenInfo`. Tool handlers would check our custom key instead.

- **Rejected because**: The SDK's `StreamableHTTPHandler` reads `auth.TokenInfoFromContext` to populate `req.Extra.TokenInfo`. A custom key would not be read by the SDK, creating two code paths for HTTP vs stdio mode.

#### Option 2: Wrap every tool handler with identity injection (rejected)

Register a wrapper around every tool handler that resolves identity and passes it via closure or struct.

- **Rejected because**: Too invasive — requires modifying all 162 sub-package registrations. The context + middleware approach is transparent to handlers.

#### Option 3: Per-request GitLab API call in tool handlers (rejected)

Each tool handler calls `client.CurrentUser(ctx)` on demand to resolve identity.

- **Rejected because**: Adds latency to every tool call. No caching. Couples identity resolution to tool logic.

#### Option 4: RequireBearerToken for HTTP + context fallback for stdio (accepted)

- HTTP modes: Use `auth.RequireBearerToken` middleware with a `NormalizeAuthHeader` adapter for legacy PRIVATE-TOKEN headers. The SDK automatically populates `req.Extra.TokenInfo`.
- Stdio mode: Resolve identity once at startup via `CurrentUser`, store in context via `IdentityToContext(ctx, identity)`.
- Unified resolution: `ResolveIdentity(ctx, req)` checks `req.Extra.TokenInfo` first, falls back to context.

## Decision

Implement Option 4 with the following architecture:

### Identity Types (`internal/toolutil/identity.go`)

- `UserIdentity{UserID string, Username string}` — canonical identity struct.
- `IdentityToContext(ctx, identity)` / `IdentityFromContext(ctx)` — context storage for stdio mode.
- `ResolveIdentity(ctx, req)` — unified resolution: TokenInfo priority, context fallback.

### HTTP Modes (`cmd/server/main.go`)

- Single `getServer` callback reads `auth.TokenInfoFromContext(r.Context())` — works for both OAuth and legacy modes.
- `auth.RequireBearerToken(verifier)` applied to all HTTP modes.
- `oauth.NormalizeAuthHeader` middleware wraps only non-OAuth mode, converting `PRIVATE-TOKEN` header to `Authorization: Bearer`.

### Stdio Mode (`cmd/server/main.go`)

- After `client.Initialize(ctx)`, call `client.CurrentUser(ctx)` to resolve `{UserID, Username}`.
- Create `UserIdentity` and store via `IdentityToContext(ctx, identity)`.
- Pass enriched context to `server.Run(identityCtx, transport)`.

### Token Caching (`internal/oauth/cache.go`)

- `TokenCache` with `sync.RWMutex` + `map[string]cacheEntry`.
- Keys: SHA-256 hex of raw tokens (never stored in plain text).
- TTL from `TokenInfo.Expiration` set by the verifier.
- `NewGitLabVerifier` accepts optional `*TokenCache` parameter (nil = no caching).

## Consequences

### Positive

- **POS-001**: All tool handlers can access user identity via `ResolveIdentity(ctx, req)` regardless of transport mode.
- **POS-002**: HTTP legacy mode now validates tokens against GitLab (was previously unvalidated).
- **POS-003**: Caching prevents redundant GitLab API calls — single validation per token per TTL period.
- **POS-004**: The `NormalizeAuthHeader` adapter enables legacy clients to work without code changes.
- **POS-005**: Clean API — tool handlers call one function, no mode-specific branching.

### Negative

- **NEG-001**: Stdio mode identity is resolved once at startup. If the token is revoked mid-session, the identity remains stale until restart.
- **NEG-002**: The `NormalizeAuthHeader` middleware adds minimal overhead to every HTTP legacy request (header copy).

### Neutral

- **NEU-001**: `UserFromRequest` removed entirely (no published API, no backward compatibility needed).
- **NEU-002**: The unified middleware chain simplifies `serveHTTP()` from two code paths to one.
