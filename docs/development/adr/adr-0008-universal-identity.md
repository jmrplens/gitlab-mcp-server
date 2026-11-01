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

**Accepted.** The unified resolution API exists and every transport populates it: stdio at startup, HTTP OAuth through `req.Extra.TokenInfo`, and HTTP legacy through the identity the gate attaches to the request context (see the Superseded note under Decision). Both HTTP modes resolve the instance with the user since [issue 953](https://github.com/jmrplens/gitlab-mcp-server/issues/953), which also bound multi-round-trip request state to the caller it was issued to and left a request no credential was bound to with no budget to share (see the amendment of 2026-10 under Identity Types).

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

- **Rejected because**: Too invasive — requires modifying all 163 sub-package registrations. The context + middleware approach is transparent to handlers.

#### Option 3: Per-request GitLab API call in tool handlers (rejected)

Each tool handler calls `client.CurrentUser(ctx)` on demand to resolve identity.

- **Rejected because**: Adds latency to every tool call. No caching. Couples identity resolution to tool logic.

#### Option 4: RequireBearerToken for HTTP + context fallback for stdio (accepted)

- HTTP modes: Use `auth.RequireBearerToken` middleware with a `NormalizeAuthHeader` adapter for legacy PRIVATE-TOKEN headers. The SDK automatically populates `req.Extra.TokenInfo`. *(The adapter was later removed — see the Superseded note below.)*
- Stdio mode: Resolve identity once at startup via `CurrentUser`, store in context via `IdentityToContext(ctx, identity)`.
- Unified resolution: `ResolveIdentity(ctx, req)` checks `req.Extra.TokenInfo` first, falls back to context.

## Decision

Implement Option 4 with the following architecture:

### Identity Types (`internal/toolutil/identity.go`)

- `UserIdentity{UserID string, Username string}` — canonical identity struct.

> **Amended (2026-08)**: the struct carries an optional `Instance` as well. A
> GitLab user id is unique within an instance and means nothing across them, so
> a deployment publishing several through a repeated `--gitlab-url` logs two
> different people as `user_id 7` with nothing separating the lines. This
> decision names audit logging as its first motivation and did not record that
> limit, which is the one place an ambiguous subject matters.
>
> It is filled where it is already known rather than resolved again: the HTTP
> gate computes the instance per request to choose a pool entry, and used to
> discard it. It stays empty, and out of the log line, where there is nothing to
> distinguish: stdio resolves one identity against one instance at startup, and
> a deployment that pins a single `--gitlab-url` has only the one, so the field
> would repeat a constant rather than give something to group by.
>
> What remains missing is a per-call correlation id. Two concurrent calls from
> one user on one instance are still indistinguishable in the log. The SDK's own
> JSON-RPC id cannot serve: `idContextKey` is unexported with no accessor, and
> the id is client-chosen, so two clients routinely both send `1`. Minting one
> server-side is the only viable form and is not done here.
>
> **Amended (2026-10)**: issue 953 closed the three gaps the amendment above
> left between this decision and the tenant the tenant policy specification
> defines, the pair (canonical instance URL, GitLab user id).
>
> The OAuth admission carries the instance the verifier checked the token
> against (`oauth.InstanceKey`), so `ResolveIdentity` returns it in oauth mode
> as it already did in legacy mode, and the telemetry identity policy records
> the pair: `gitlab_mcp.instance` beside the user under `full`, and the
> instance inside the digest under `pseudonymous`, which changed every such
> digest once.
>
> Multi-round-trip request state is bound to the principal its request ran
> under, the pool entry's random owner token, inside its MACs rather than as a
> readable claim, so state issued under one credential yields no answers under
> another.
>
> A request no credential was bound to resolves no identity, and on a server
> shared by a configuration shape it now runs under no credential state either:
> the shell holds no default bucket or listen counter for it, and the two
> methods no handler refuses, `tools/list` and `subscriptions/listen`, are
> refused right after the binding (`refuseUnattributed`, register row
> `IDN-007`).

- `IdentityToContext(ctx, identity)` / `IdentityFromContext(ctx)` — context storage for stdio mode.
- `ResolveIdentity(ctx, req)` — unified resolution: TokenInfo priority, context fallback.

### HTTP Modes (`cmd/server/main.go`)

- Single `getServer` callback reads `auth.TokenInfoFromContext(r.Context())` — works for both OAuth and legacy modes.
- `auth.RequireBearerToken(verifier)` applied to all HTTP modes.
- `oauth.NormalizeAuthHeader` middleware wraps only non-OAuth mode, converting `PRIVATE-TOKEN` header to `Authorization: Bearer`.

> **Superseded (2026-08)**: the normalization adapter is gone. OAuth mode is Bearer-only — what the `WWW-Authenticate` challenge advertises is exactly what is accepted — and legacy mode reads both headers directly through `serverpool.ExtractToken`, never through this package. A personal access token sent as `Authorization: Bearer` works in either mode, so nothing is lost.
>
> `oauth.NormalizeAuthHeader` survived as unmounted code for a while after that, with this document still describing it as wired. It was deleted in 2026-08; the surviving statements about it below record what was decided in April 2026, not how the server behaves.
>
> Two further corrections to the description above: `auth.RequireBearerToken` is mounted in **oauth mode only**, not "all HTTP modes": legacy mode authenticates in `mcpServerGate` alone, which is why `req.Extra.TokenInfo` is unset there. That used to leave legacy HTTP mode with the empty identity; since 2026-08 the gate attaches the user the pool resolved when it built the entry to the request context (`mcpServerGate.withIdentity`), with the instance since 2026-09, and `ResolveIdentity` falls back to it. And in oauth mode the SDK middleware is no longer the outermost layer: `bearerGuard` runs in front of it to rate-limit, cache rejections, and answer with RFC 6750 error codes.

### Stdio Mode (`cmd/server/main.go`)

- After `client.Initialize(ctx)`, call `client.CurrentUser(ctx)` to resolve `{UserID, Username}`.
- Create `UserIdentity` and store via `IdentityToContext(ctx, identity)`.
- Pass enriched context to `server.Run(identityCtx, transport)`.

> **Amended (2026-09)**: the last step no longer holds as written. The GitLab
> probe now runs after the transport is connected, so that `initialize` can be
> answered while the catalog is still being built, and the context handed to
> `server.Run` carries no identity. The resolved identity is stored in a
> `deferredIdentity` (`cmd/server/readiness.go`) and overlaid onto every
> request context by a receiving middleware, which is the same
> `IdentityToContext` value handlers read through `ResolveIdentity`. Resolution
> still happens once, at startup, so NEG-001 is unchanged.

### Token Caching (`internal/oauth/cache.go`)

- `TokenCache` with `sync.RWMutex` + `map[string]cacheEntry`.
- Keys: SHA-256 hex of raw tokens (never stored in plain text). [Since 2026-08 the hash covers the instance URL and the token together, so a credential verified against one published instance cannot pass as identity on another; `Get` and `Put` take both.]
- TTL from `TokenInfo.Expiration` set by the verifier.
- `NewGitLabVerifier` accepts optional `*TokenCache` parameter (nil = no caching).

## Consequences

### Positive

- **POS-001**: ~~All tool handlers can access user identity via `ResolveIdentity(ctx, req)` regardless of transport mode.~~ Narrowed, then restored: the call is uniform, and so is the coverage now. It resolves an identity in every mode; in HTTP legacy mode, where nothing populates `req.Extra.TokenInfo`, it reads the identity the gate put on the context, and in HTTP OAuth mode the one it reads from `TokenInfo`, both with the instance since [issue 953](https://github.com/jmrplens/gitlab-mcp-server/issues/953). Its consumers are log enrichment in `toolutil/logging.go` and the telemetry identity of a tool call in `cmd/server/telemetry_identity.go`.
- **POS-002**: HTTP legacy mode now validates tokens against GitLab (was previously unvalidated).
- **POS-003**: Caching prevents redundant GitLab API calls — single validation per token per TTL period.
- **POS-004**: ~~The `NormalizeAuthHeader` adapter enables legacy clients to work without code changes.~~ Withdrawn: the adapter was removed. Legacy clients keep working because the legacy gate reads `PRIVATE-TOKEN` directly.
- **POS-005**: Clean API — tool handlers call one function, no mode-specific branching.

### Negative

- **NEG-001**: Stdio mode identity is resolved once at startup. If the token is revoked mid-session, the identity remains stale until restart.
- **NEG-002**: ~~The `NormalizeAuthHeader` middleware adds minimal overhead to every HTTP legacy request (header copy).~~ Withdrawn with POS-004: there is no such middleware.

### Neutral

- **NEU-001**: `UserFromRequest` removed entirely (no published API, no backward compatibility needed).
- **NEU-002**: The unified middleware chain simplifies `serveHTTP()` from two code paths to one.
