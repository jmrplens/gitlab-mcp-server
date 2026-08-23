# HTTP Server Mode

This document describes how gitlab-mcp-server operates in HTTP server mode, where multiple AI clients connect to a single shared server process over the network.

> **Diátaxis type**: Explanation
> **Audience**: ⚙️ Server administrators
> **Prerequisites**: [Configuration](../reference/configuration.md), [Architecture](../concepts/architecture.md)
> 📖 **User documentation**: See the [HTTP Server Mode](https://jmrplens.github.io/gitlab-mcp-server/operations/http-server/) on the documentation site for a user-friendly version.

---

## Overview

By default, gitlab-mcp-server runs in **stdio mode** — each AI client (VS Code, Cursor, Copilot CLI, OpenCode) spawns its own server process that communicates via stdin/stdout. This is simple but means each user runs a separate binary.

**HTTP mode** is an alternative transport where a single gitlab-mcp-server process listens on a network address and serves multiple clients simultaneously. Each client authenticates with its own GitLab Personal Access Token, and the server maintains an isolated MCP server instance per unique token.

### When to Use HTTP Mode

| Scenario                          | Recommended Mode                       |
| --------------------------------- | -------------------------------------- |
| Single developer, local AI client | stdio                                  |
| Team sharing one server instance  | **HTTP**                               |
| Remote/headless server deployment | **HTTP**                               |
| CI/CD integration with MCP        | **HTTP** (see [CI/CD Usage](ci-cd.md)) |
| Testing with curl or HTTP clients | **HTTP**                               |

## Starting the HTTP Server

HTTP mode is configured entirely via CLI flags — no environment variables are needed:

```bash
# Single GitLab.com instance (all clients use the same instance; replace for self-managed GitLab)
gitlab-mcp-server --http \
  --gitlab-url=https://gitlab.com \
  --http-addr=:8080

# Multi-instance (each client specifies their GitLab URL via GITLAB-URL header)
gitlab-mcp-server --http \
  --http-addr=:8080
```

### CLI Flags

| Flag                       | Default        | Description                                                                                                                                                                                                                                    |
| -------------------------- | -------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `--http`                   | _(off)_        | Enable HTTP transport mode                                                                                                                                                                                                                     |
| `--gitlab-url`             | _(optional)_   | Fixed GitLab instance URL. Omit it to require each client to send `GITLAB-URL` per request                                                                                                                                                     |
| `--http-addr`              | `:8080`        | HTTP listen address (host:port)                                                                                                                                                                                                                |
| `--skip-tls-verify`        | `false`        | Skip TLS certificate verification for self-signed certs                                                                                                                                                                                        |
| `--tool-surface`           | `dynamic`      | Canonical tool catalog selector; see [Tool and capability surface options](#tool-and-capability-surface-options)                                                                                                                               |
| `--meta-tools`             | _(unset)_      | Deprecated compatibility flag. Use `--tool-surface=individual` instead of `--meta-tools=false`                                                                                                                                                 |
| `--capability-surface`     | `full`         | Resource and prompt selector; see [Tool and capability surface options](#tool-and-capability-surface-options)                                                                                                                                  |
| `--meta-param-schema`      | `opaque`       | Meta-tool input schema mode: `opaque`, `compact`, or `full`                                                                                                                                                                                    |
| `--tier`                   | _(detected)_   | Force the licensing tier (`free`, `ce`, `premium`, `ultimate`) when explicitly set. When omitted, HTTP mode detects the tier per token+URL pool entry from the instance license (fallback `free`)                                              |
| `--read-only`              | `false`        | Expose only read-only tools                                                                                                                                                                                                                    |
| `--safe-mode`              | `false`        | Intercept mutating operations per action and return a JSON preview instead of executing them; reads keep working                                                                                                                               |
| `--embedded-resources`     | `true`         | Embed canonical MCP resource URIs in get_* tool results                                                                                                                                                                                        |
| `--exclude-tools`          | _(empty)_      | Comma-separated tool names to exclude from registration                                                                                                                                                                                        |
| `--ignore-scopes`          | `false`        | Skip PAT scope detection and register all tools allowed by the configured catalog                                                                                                                                                              |
| `--max-http-clients`       | `100`          | Maximum unique token+URL entries in the server pool                                                                                                                                                                                            |
| `--session-timeout`        | `30m`          | Idle MCP session timeout                                                                                                                                                                                                                       |
| `--auth-mode`              | `legacy`       | Authentication mode: `legacy` (PRIVATE-TOKEN) or `oauth` (Bearer token verified via GitLab API)                                                                                                                                                |
| `--oauth-cache-ttl`        | `15m`          | How long verified OAuth tokens are cached before re-validation (1m–2h)                                                                                                                                                                         |
| `--pool-idle-timeout`      | `1h`           | Reclaim a pooled per-token-and-URL server entry after this long unused; `0` keeps entries until the pool size bound evicts them (upper bound: 24h)                                                                                             |
| `--revalidate-interval`    | `15m`          | Token re-validation interval; `0` to disable (upper bound: 24h)                                                                                                                                                                                |
| `--http-idle-timeout`      | `0` (disabled) | HTTP server idle connection timeout. Default `0` disables idle connection closure entirely, so `--session-timeout` is the effective session lifetime. Set a positive duration to recycle idle connections sooner                               |
| `--trusted-proxy-header`   | _(empty)_      | HTTP header containing the real client IP (e.g. `CF-Connecting-IP`, `X-Forwarded-For`). Required for rate limiting behind reverse proxies                                                                                                      |
| `--rate-limit-rps`         | `0`            | Per-server `tools/call` rate limit in requests per second (`0` = disabled)                                                                                                                                                                     |
| `--rate-limit-burst`       | `40`           | Token-bucket burst size when `--rate-limit-rps` > 0                                                                                                                                                                                            |
| `--stateless`              | `true`         | Sessionless streamable HTTP (SEP-2567 / protocol 2026-07-28): no `Mcp-Session-Id` tracking, every POST is self-contained, GET/DELETE return `405`. Use `--stateless=false` for legacy stateful sessions. See [Stateless Mode](#stateless-mode) |
| `--json-response`          | `false`        | Return `application/json` response bodies instead of `text/event-stream` (SSE)                                                                                                                                                                 |
| `--max-request-body-bytes` | `0`            | Maximum streamable HTTP request body size in bytes. `0` uses the SDK default (4 MiB); oversized bodies are rejected with `413`. Negative values are rejected at startup                                                                        |

> **Note**: `--gitlab-url` is optional. When omitted, each client must provide the `GITLAB-URL` header. When set, it is authoritative: any client-provided `GITLAB-URL` header is ignored, the configured URL is used, and the request logs `ignored_options` for that client.

### Tool and capability surface options

`--tool-surface` selects the visible MCP tool catalog for every HTTP server-pool entry:

- `meta`: domain-level meta-tools, the consolidated catalog.
- `individual`: every GitLab operation is exposed as its own tool.
- `dynamic`: the current low-token two-tool surface with `gitlab_find_action` and `gitlab_execute_action`.

`--capability-surface` controls resources and prompts independently of tools: `full` registers all resources, workflow guides, prompts, and the surface-aware `gitlab://tools` manifest, while `minimal` keeps `gitlab://tools` and omits prompts, workflow guides, and optional GitLab data resources. Dynamic schema discovery still works with `minimal` because `gitlab_find_action` returns schemas inline.

`--meta-param-schema` affects visible domain meta-tool `inputSchema` only. Keep the default `opaque` unless a client needs `compact` or `full` schemas in `tools/list`; exact call shapes remain available through `gitlab://tools/{id}`. Current audit metrics show `compact` is 6.5x larger than `opaque`, and `full` is 11.9x larger.

### Stateless Mode

Stateless is the **default** transport model. It follows the sessionless design
introduced by [SEP-2567](https://github.com/modelcontextprotocol/modelcontextprotocol/pull/2567)
(MCP protocol `2026-07-28`):

- The server neither reads nor sets the `Mcp-Session-Id` header. Every POST is
  a self-contained JSON-RPC exchange — no `initialize` round-trip is required.
- GET and DELETE on the MCP endpoint return `405 Method Not Allowed`
  (`Allow: POST`). `/health` and `/.well-known/*` endpoints are unaffected.
- Synchronous server-initiated requests are rejected by the SDK because no
  client channel outlives the request. Clients on protocol `2026-07-28` keep
  full elicitation through multi-round-trip requests (MRTR), which travel in
  the tool result itself; only legacy-protocol clients fall back to the
  non-interactive error hints and the `confirm` parameter. See
  [Elicitation](../reference/capabilities/elicitation.md).
- `--session-timeout` has no effect: no session outlives its request.
- The per-token-and-URL server pool still applies: repeated requests with the same
  token and GitLab URL reuse a cached `*mcp.Server`, so stateless mode does
  not pay client-rebuild costs per request.

Stateless mode suits load-balanced deployments where requests from one client
may land on different replicas. Combine with `--json-response` for clients or
gateways that prefer plain JSON bodies over SSE:

```bash
gitlab-mcp-server --http --gitlab-url=https://gitlab.example.com \
  --json-response
```

Validate a deployment with `make validate-http-stateless` (compiled binary) or
`make validate-http-stateless-docker` (Docker image), both backed by
`scripts/validate-http-stateless.sh`.

#### Legacy stateful mode

`--stateless=false` restores the session-based transport: the server issues
`Mcp-Session-Id` on `initialize`, GET opens the standalone SSE stream, DELETE
terminates the session, and `--session-timeout` governs idle session lifetime.
This is a compatibility mode for clients that cannot yet negotiate protocol
`2026-07-28`; such sessions negotiate `2025-11-25` or older and use synchronous
elicitation. The server logs a warning at startup when it is enabled. The
intent is to remove it once client ecosystems have migrated, so treat new
deployments as stateless.

#### Cache hints

Every cacheable result carries [SEP-2549](https://github.com/modelcontextprotocol/modelcontextprotocol/pull/2549)
hints. All of them use `cacheScope: private`, because catalogs and resource
content are filtered by the caller's token scopes and licensing tier and must
never be served from a shared intermediary cache:

| Result                                                                                        | `ttlMs`              |
| --------------------------------------------------------------------------------------------- | -------------------- |
| `tools/list`, `prompts/list`, `resources/list`, `resources/templates/list`, `server/discover` | `300000` (5 minutes) |
| `resources/read` of static content (guides, schemas, `gitlab://tools` manifests)              | `3600000` (1 hour)   |
| `resources/read` of live GitLab data                                                          | `0` (always fresh)   |

#### Request cancellation

Client aborts are always propagated into handler contexts, so an abandoned POST
cancels its in-flight GitLab API calls. The SDK applies this to protocol
`2026-07-28` requests only; legacy-protocol clients are unaffected.

#### Gateway routing annotation

On the dynamic tool surface, the `action` property of `gitlab_execute_action`
carries the [SEP-2243](https://github.com/modelcontextprotocol/modelcontextprotocol/pull/2243)
`x-mcp-header` annotation with the value `Mcp-Param-Action`. MCP-aware gateways
can therefore route, rate-limit, and observe calls by canonical action ID from
the request header, without parsing the JSON-RPC body.

### Configuration Precedence

HTTP mode has a narrow request-controlled surface. GitLab identity always comes from the request token, and the GitLab instance comes from `GITLAB-URL` only when the server was started without `--gitlab-url`. All other MCP server settings are process policy and cannot be changed per user, per session, or per JSON-RPC request.

Process policy itself resolves in three layers, highest first: a CLI flag passed explicitly, then the matching environment variable, then the built-in default. Passing a flag whose value happens to equal the default still counts as choosing it, so a stray environment variable cannot displace a deliberate command line. A client has no way to reach any of those layers, so one user can never change behaviour for themselves or for anyone else.

| Configuration area                  | Source of truth (an explicitly passed flag wins over its environment variable)                                                                                                                                                                                                                                                                                                                                       | Can a client override it?                   | Behavior when a client sends a matching header                                                                                                                                                                                       |
| ----------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| GitLab token                        | `PRIVATE-TOKEN` or `Authorization: Bearer` request header                                                                                                                                                                                                                                                                                                                                                            | Yes, this is the per-user identity boundary | Accepted and used to select/create the pooled server entry                                                                                                                                                                           |
| GitLab URL                          | `--gitlab-url` or `GITLAB_URL`, or the `GITLAB-URL` header only when neither is set                                                                                                                                                                                                                                                                                                                                  | Conditional                                 | If `--gitlab-url` is set, `GITLAB-URL` is ignored and logged in `ignored_options`                                                                                                                                                    |
| Tool catalog and behavior           | `--tool-surface`, deprecated `--meta-tools`, `--capability-surface`, `--meta-param-schema`, `--tier`, `--read-only`, `--safe-mode`, `--embedded-resources`, `--exclude-tools`, `--ignore-scopes`, each falling back to `TOOL_SURFACE`, `META_TOOLS`, `CAPABILITY_SURFACE`, `META_PARAM_SCHEMA`, `GITLAB_TIER`, `GITLAB_READ_ONLY`, `GITLAB_SAFE_MODE`, `EMBEDDED_RESOURCES`, `EXCLUDE_TOOLS`, `GITLAB_IGNORE_SCOPES` | No                                          | Ignored and logged in `ignored_options` when sent as config-like headers such as `TOOL-SURFACE`, `META-TOOLS`, `CAPABILITY-SURFACE`, `META-PARAM-SCHEMA`, or `GITLAB-SAFE-MODE`; `META-TOOLS` is also logged in `deprecated_options` |
| Rate limits and HTTP pool policy    | `--rate-limit-rps`, `--rate-limit-burst`, `--max-http-clients`, `--session-timeout`, `--revalidate-interval` and `--pool-idle-timeout`, each falling back to `RATE_LIMIT_RPS`, `RATE_LIMIT_BURST`, `MAX_HTTP_CLIENTS`, `SESSION_TIMEOUT`, `SESSION_REVALIDATE_INTERVAL` and `POOL_IDLE_TIMEOUT`; `--http-idle-timeout`, `--trusted-proxy-header`, `--stateless` and `--json-response` are CLI-only                   | No                                          | Ignored and logged in `ignored_options` when sent as config-like headers such as `RATE-LIMIT-RPS`                                                                                                                                    |
| Authentication mode and OAuth cache | `--auth-mode` or `AUTH_MODE`, `--oauth-cache-ttl` or `OAUTH_CACHE_TTL`                                                                                                                                                                                                                                                                                                                                               | No                                          | Ignored and logged in `ignored_options`                                                                                                                                                                                              |
| Update policy and logging           | `--auto-update`, `--auto-update-repo`, `--auto-update-interval` and `--auto-update-timeout`, each falling back to `AUTO_UPDATE`, `AUTO_UPDATE_REPO`, `AUTO_UPDATE_INTERVAL` and `AUTO_UPDATE_TIMEOUT`; `LOG_LEVEL` is environment-only                                                                                                                                                                               | No                                          | Ignored and logged in `ignored_options`                                                                                                                                                                                              |

This means options that affect the size of MCP schemas, such as `--meta-param-schema`, are fixed when each `*mcp.Server` instance is created. Options that affect throttling, such as `--rate-limit-rps` and `--rate-limit-burst`, are also copied into each pooled server entry; clients cannot increase, disable, or replace those limits through request headers or MCP parameters.

## Architecture

### Server Pool

The core of HTTP mode is the **Server Pool** (`internal/serverpool`), a bounded cache of MCP server instances keyed by the SHA-256 hash of each client's token **and** GitLab URL.

```mermaid
graph TD
    subgraph "HTTP Mode Architecture"
        REQ1[Client A<br/>Token: glpat-aaa<br/>URL: gitlab.com] --> HANDLER[StreamableHTTPHandler]
        REQ2[Client B<br/>Token: glpat-bbb<br/>URL: gitlab.com] --> HANDLER
        REQ3[Client C<br/>Token: glpat-aaa<br/>URL: self-hosted.example.com] --> HANDLER

        HANDLER --> GS[getServer callback]
        GS --> EXTRACT[ExtractToken + ExtractGitLabURL]
        EXTRACT --> POOL[ServerPool]

        POOL --> ENTRY1["hash(glpat-aaa + gitlab.com)<br/>*mcp.Server + *gitlab.Client"]
        POOL --> ENTRY2["hash(glpat-bbb + gitlab.com)<br/>*mcp.Server + *gitlab.Client"]
        POOL --> ENTRY3["hash(glpat-aaa + self-hosted)<br/>*mcp.Server + *gitlab.Client"]
    end

    ENTRY1 --> GL1[GitLab API<br/>as user A @ gitlab.com]
    ENTRY2 --> GL2[GitLab API<br/>as user B @ gitlab.com]
    ENTRY3 --> GL3[GitLab API<br/>as user A @ self-hosted]
```

**Key properties:**

- Clients with the **same token and same GitLab URL** share the same `*mcp.Server` instance
- Clients with **different tokens** or **different GitLab URLs** get completely isolated server instances
- Each server instance has its own GitLab client authenticated with that specific token against that specific GitLab instance
- Each server instance stores its own server configuration snapshot, derived from global process settings plus detected per-token/per-instance data
- Each server instance detects token scopes and the licensing tier independently, so multi-instance deployments can serve Free, Premium, and Ultimate GitLab instances from the same process. The tier is detected from the instance license (`GET /license` → plan; fallback `free`). If `--tier` is explicitly set, that configured value wins and tier detection is disabled.
- Zero cross-contamination between clients by construction

### Session Key Isolation

The pool key is `SHA-256(token + "\x00" + gitlabURL)`, never the raw values. This means:

- Raw tokens and URLs are never stored in memory beyond the initial request
- The same token against different GitLab instances produces different pool entries
- Log messages show only the last 4 characters of the token (e.g., `...a1b2`) for debugging
- Even if the pool's internal state were somehow exposed, tokens remain protected

## Client Authentication

Clients must provide their GitLab Personal Access Token on every HTTP request using one of two headers.

When the server starts without `--gitlab-url`, clients must specify which GitLab instance to target using the `GITLAB-URL` header:

```text
GITLAB-URL: https://gitlab.example.com
```

When `--gitlab-url` is set, the server always uses the configured URL. If a client still sends `GITLAB-URL`, the header is ignored and logged as a request option overridden by MCP configuration. If both `--gitlab-url` and `GITLAB-URL` are absent, the request is rejected.

### Token Headers

#### Option 1: PRIVATE-TOKEN Header (Recommended)

```text
PRIVATE-TOKEN: glpat-xxxxxxxxxxxxxxxxxxxx
```

This is the standard GitLab authentication header and takes precedence over Bearer.

#### Option 2: Authorization Bearer Header

```text
Authorization: Bearer glpat-xxxxxxxxxxxxxxxxxxxx
```

Standard OAuth2-style bearer token, supported for compatibility.

#### Precedence

If both headers are present, `PRIVATE-TOKEN` wins.

#### Missing Token

A request with no token is rejected with `401 Unauthorized`, an RFC 9110 challenge, and a JSON-RPC error body naming both accepted headers:

```http
HTTP/1.1 401 Unauthorized
WWW-Authenticate: Bearer realm="gitlab-mcp-server"
Content-Type: application/json

{"jsonrpc":"2.0","id":null,"error":{"code":-40100,"message":"Authentication required: send a GitLab personal access token as 'Authorization: Bearer <glpat-...>' or 'PRIVATE-TOKEN: <glpat-...>'. ..."}}
```

The challenge deliberately omits the `resource_metadata` parameter. Clients discover an OAuth authorization server through that parameter, and legacy mode has none — advertising it would start a discovery flow that cannot complete. Use `--auth-mode=oauth` for a challenge that does point at [RFC 9728](https://datatracker.ietf.org/doc/html/rfc9728) metadata.

Each rejection is also logged server-side:

```json
{"level":"INFO","msg":"request rejected: missing authentication token (set PRIVATE-TOKEN header or Authorization: Bearer)"}
```

#### Rejection Status Codes

Every request that cannot be served is classified before it reaches the MCP handler, so the status distinguishes the cause:

| Condition                                              | Status | JSON-RPC code | Notable headers    |
| ------------------------------------------------------ | ------ | ------------- | ------------------ |
| No `PRIVATE-TOKEN` and no `Authorization: Bearer`      | `401`  | `-40100`      | `WWW-Authenticate` |
| GitLab answered `401`/`403` to the credential          | `401`  | `-40100`      | `WWW-Authenticate` |
| `GITLAB-URL` header is not a parseable URL             | `400`  | `-32600`      | —                  |
| More than 10 auth failures from one IP within a minute | `429`  | `-42900`      | `Retry-After`      |
| GitLab session could not be built for the token        | `503`  | `-50300`      | —                  |

All four return `Content-Type: application/json` with a JSON-RPC error response. This matters beyond readability: protocol revision 2026-07-28 tells a client that receives a `400` whose body is _not_ a recognised JSON-RPC error to conclude the server is initialization-era and downgrade, so a plain-text `400` would turn a missing header into a false protocol diagnosis.

#### Credential Verification

A token is verified against the instance once, when its pooled session is first built — never on subsequent requests, which are served from the pool. The probe is `GET /api/v4/user`, and only an explicit `401` or `403` rejects it.

Verifying is what stops an unauthenticated caller from obtaining a working session with any non-empty string, and stops a stream of invented tokens from churning the session pool. Token _format_ is deliberately not checked: GitLab lets self-managed administrators change the `glpat-` prefix, so a prefix rule would reject legitimate self-hosted tokens while still admitting any well-shaped fake.

Every other outcome — a transport error, a `5xx`, a `404` from an instance that does not expose the endpoint — means no verdict was obtained, and the session is admitted. Failing closed whenever GitLab is unreachable would turn an instance outage into a total denial of service. The probe does not retry and is bounded at 5 seconds.

Codes are allocated outside the JSON-RPC reserved range (`-32768` to `-32000`), as the MCP specification requires for application-defined errors, and mirror their HTTP status. `GET` and `DELETE` are not gated: they continue to receive `405 Method Not Allowed`, the answer protocol 2026-07-28 prescribes for them.

## Authentication Modes

HTTP mode supports two authentication modes controlled by `--auth-mode`:

### Mode Comparison

| Feature                    | Legacy (`--auth-mode=legacy`)                       | OAuth (`--auth-mode=oauth`)                                      |
| -------------------------- | --------------------------------------------------- | ---------------------------------------------------------------- |
| Token headers              | `PRIVATE-TOKEN` or `Authorization: Bearer`          | `Authorization: Bearer` (auto-converted from `PRIVATE-TOKEN`)    |
| Server-side validation     | None — token passed directly to GitLab API          | Verified via `GET /api/v4/user` before MCP handler               |
| Token caching              | No caching — every API call uses the token directly | SHA-256 hashed cache with configurable TTL (default 15m)         |
| Invalid token handling     | Errors appear at GitLab API call time               | Rejected immediately with HTTP 401                               |
| RFC 9728 metadata          | Not served                                          | Served at `/.well-known/oauth-protected-resource`                |
| MCP client OAuth discovery | Not supported                                       | Supported — clients can discover the GitLab authorization server |
| Best for                   | Simple setups, internal networks                    | Production deployments, MCP clients with OAuth 2.1 support       |

### Legacy Mode (default)

The default `--auth-mode=legacy` accepts tokens via `PRIVATE-TOKEN` or `Authorization: Bearer` headers without server-side verification. The token is passed directly to the GitLab API client. This mode is simple but the server trusts any token format — validation only happens when GitLab rejects an API call.

### OAuth Mode

`--auth-mode=oauth` enables server-side token verification using the go-sdk's `auth.RequireBearerToken` middleware. Every request is validated against GitLab's `/api/v4/user` endpoint before reaching the MCP handler.

```bash
gitlab-mcp-server --http \
  --gitlab-url=https://gitlab.com \
  --auth-mode=oauth \
  --oauth-cache-ttl=15m
```

**How it works:**

```mermaid
sequenceDiagram
    participant Client as MCP Client
    participant MW as NormalizeAuthHeader
    participant Auth as RequireBearerToken
    participant Cache as Token Cache
    participant GL as GitLab API
    participant MCP as MCP Handler

    Client->>MW: POST /mcp<br/>PRIVATE-TOKEN: glpat-xxx
    MW->>MW: Convert to Authorization: Bearer
    MW->>Auth: Forward request
    Auth->>Cache: Lookup SHA-256(token)

    alt Cache hit (not expired)
        Cache-->>Auth: TokenInfo (cached)
    else Cache miss or expired
        Auth->>GL: GET /api/v4/user<br/>Authorization: Bearer glpat-xxx
        GL-->>Auth: 200 OK + user JSON
        Auth->>Cache: Store TokenInfo (TTL)
    end

    Auth->>MCP: Authenticated request
    MCP-->>Client: MCP response
```

1. Client sends a request with `Authorization: Bearer <token>` (or `PRIVATE-TOKEN`, which is auto-converted)
2. The server calls `GET /api/v4/user` on GitLab with the token
3. If GitLab returns a valid user, the token is cached for `--oauth-cache-ttl` (default 15 minutes)
4. Subsequent requests with the same token skip the GitLab call until the cache expires
5. Invalid or expired tokens return HTTP 401

**Protected Resource Metadata ([RFC 9728](https://datatracker.ietf.org/doc/html/rfc9728)):**

In OAuth mode, the server exposes `/.well-known/oauth-protected-resource` with metadata for MCP clients that implement OAuth discovery:

```bash
curl http://localhost:8080/.well-known/oauth-protected-resource
```

```json
{
  "resource": "http://localhost:8080",
  "authorization_servers": ["https://gitlab.example.com"],
  "bearer_methods_supported": ["header"],
  "scopes_supported": ["api", "read_api"]
}
```

**Token caching:**

- Cache key: SHA-256 hash of the token (raw token never stored)
- Default TTL: 15 minutes (configurable via `--oauth-cache-ttl`)
- Bounds: minimum 1 minute, maximum 2 hours
- Expired entries are evicted by a background goroutine every 5 minutes

## Client Configuration Examples

### Legacy Mode

#### VS Code / GitHub Copilot

Add to `.vscode/mcp.json`:

```json
{
  "servers": {
    "gitlab": {
      "type": "http",
      "url": "http://your-internal-server:8080/mcp",
      "headers": {
        "PRIVATE-TOKEN": "glpat-your-token"
      }
    }
  }
}
```

#### OpenCode

Add to your OpenCode MCP configuration:

```json
{
  "mcpServers": {
    "gitlab": {
      "url": "http://your-internal-server:8080/mcp",
      "headers": {
        "PRIVATE-TOKEN": "glpat-your-token"
      }
    }
  }
}
```

### OAuth Mode

When the server runs with `--auth-mode=oauth`, MCP clients that support OAuth 2.1 can discover the GitLab authorization server automatically via the RFC 9728 metadata endpoint and handle token acquisition through the standard OAuth flow.

#### VS Code / GitHub Copilot (OAuth)

VS Code supports MCP servers with OAuth authentication natively. Add to `.vscode/mcp.json`:

```json
{
  "servers": {
    "gitlab": {
      "type": "http",
      "url": "http://your-internal-server:8080/mcp",
      "oauth": {
        "clientId": "YOUR_GITLAB_APPLICATION_ID",
        "scopes": ["api"]
      }
    }
  }
}
```

VS Code discovers the GitLab authorization server via `/.well-known/oauth-protected-resource`, initiates the OAuth 2.1 PKCE flow, and sends the acquired token automatically as `Authorization: Bearer`.

> **Important**: Without `clientId`, VS Code falls back to Dynamic Client Registration (DCR). GitLab's DCR assigns the `mcp` scope instead of `api`, causing most operations to fail. Always configure `clientId` with your GitLab OAuth Application ID.
>
> **Note**: For OAuth to work, a GitLab OAuth Application must be configured. See [OAuth App Setup](oauth-app-setup.md) for a step-by-step guide, and [IDE Configuration](ide-configuration.md) for per-client examples.

### curl (Testing)

```bash
# Initialize a session
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "PRIVATE-TOKEN: glpat-your-token" \
  -d '{"jsonrpc":"2.0","method":"tools/list","id":1}'
```

## Public Hosted Endpoint

A ready-to-use instance of this server runs at `https://mcp.jmrp.io/gitlab`. It is deployed out of band from the [mcp.jmrp.io](https://github.com/jmrplens/mcp.jmrp.io) host — this repository publishes artifacts only, so nothing here deploys or configures it.

```json
{
  "mcpServers": {
    "gitlab": {
      "type": "http",
      "url": "https://mcp.jmrp.io/gitlab",
      "headers": { "PRIVATE-TOKEN": "glpat-xxxxxxxxxxxx" }
    }
  }
}
```

| Property     | Value                                                             |
| ------------ | ----------------------------------------------------------------- |
| Transport    | Stateless streamable HTTP (`--stateless`); `GET`/`DELETE` → `405` |
| Tool surface | `dynamic` (`gitlab_find_action`, `gitlab_execute_action`)         |
| Auth mode    | `legacy` — `PRIVATE-TOKEN` per request, never stored server-side  |
| `GITLAB-URL` | Optional; defaults to `https://gitlab.com`                        |
| Health       | `GET https://mcp.jmrp.io/gitlab/health` → `ok`                    |

Because it is multi-tenant, each distinct token+URL pair gets its own pooled MCP server (see [Server Pool](#server-pool)). Self-managed GitLab instances must be reachable from the public internet, and every request traverses a third-party host — for private instances, run one of the deployments described above instead.

The endpoint is listed at [mcp.jmrp.io](https://mcp.jmrp.io/), a directory of the MCP servers maintained by this author; [`https://mcp.jmrp.io/servers.json`](https://mcp.jmrp.io/servers.json) is the same list in machine-readable form.

## Session Lifecycle

This section describes **stateful** mode (`--stateless=false`). Under the
default `--stateless=true` — which the public hosted endpoint runs — no
`Mcp-Session-Id` is issued and no MCP session state is carried between
requests; every POST is self-contained. The server pool below is orthogonal to
that: it is keyed on `(token, GitLab URL)` and is reused in both modes.

### 1. First Request

When a client sends its first HTTP POST to `/mcp`:

1. `StreamableHTTPHandler` calls the `getServer` callback
2. `ExtractToken()` reads the token and `ResolveRequestOptions()` applies MCP configuration precedence to resolve the effective GitLab URL
3. `ServerPool.GetOrCreate(token, gitlabURL)` hashes `(token, url)` and checks the pool
4. If the `(token, url)` pair is new: creates a GitLab client + MCP server, registers all tools/resources/prompts
5. Returns the `*mcp.Server` for that `(token, url)` pair
6. SDK establishes an MCP session and returns a `Mcp-Session-Id` header

### 2. Subsequent Requests

Subsequent requests with the same `(token, GitLab URL)` pair:

1. Token and URL are extracted and hashed into a composite key
2. Pool finds the existing entry and promotes it in the LRU list
3. Same `*mcp.Server` is returned — session state is preserved

### 3. Session Timeout

If a client is idle for longer than `--session-timeout` (default: 30 minutes):

1. The MCP SDK closes the idle session (HTTP transport level)
2. The pool entry (server + client) **remains** in the pool
3. Next request from the same `(token, url)` pair creates a new MCP session on the existing server

### Timeout model: HTTP layer vs MCP session

Two independent layers govern how long a connection and a session live:

| Layer                | Setting               | Default          | What it bounds                                                                                          |
| -------------------- | --------------------- | ---------------- | ------------------------------------------------------------------------------------------------------- |
| MCP session          | `--session-timeout`   | `30m`            | Idle lifetime of the MCP session (SDK transport level)                                                  |
| HTTP idle connection | `--http-idle-timeout` | `0` (disabled)   | Maximum time to wait for the next request on a keep-alive connection before the `http.Server` closes it |
| HTTP response write  | fixed / disabled      | `60s` / disabled | Maximum write time for a response; kept at `60s` for standard endpoints and disabled for SSE streams    |

The server speaks the modern **Streamable HTTP** transport, which uses Server-Sent
Events (`text/event-stream`) for streamed POST responses and the standalone GET
stream that carries server-initiated notifications and 30s keep-alive pings. An
active SSE response is bounded by `WriteTimeout` (not `IdleTimeout`, which only
limits the wait between requests on an idle keep-alive connection) — and the
go-sdk SSE writer never resets the write deadline.

To avoid severing those streams without weakening protection for everything else,
the global `WriteTimeout` is kept at a safe `60s` (guarding standard endpoints such
as `/health` from slow-write attacks), and any request that negotiates SSE
(`Accept: text/event-stream`) — both the standalone GET stream and streamed POST
responses — disables its own write deadline dynamically. Because the default
`--http-idle-timeout` is `0`
(disabled), the HTTP layer also does not close idle connections, so `--session-timeout`
is the effective idle lifetime. If you set a low `--http-idle-timeout`, expect
long-lived MCP sessions to drop when the connection idles past that value.

> **Behind a reverse proxy / edge**: when idle closure is disabled on the server, the
> proxy's own read/idle timeout becomes the limiting factor. Raise it (e.g. to a few
> minutes or more) for long-running MCP streams. See
> [Troubleshooting](troubleshooting.md).

### 4. Pool Eviction

When the pool is full (`--max-http-clients` reached) and a new `(token, url)` pair arrives:

1. The **least recently used** pool entry is evicted
2. The evicted server and GitLab client are removed from the pool
3. A new entry is created for the new `(token, url)` pair
4. If the evicted client reconnects, a fresh server is created

```mermaid
sequenceDiagram
    participant Client as HTTP Client
    participant Handler as StreamableHTTPHandler
    participant GS as getServer callback
    participant Pool as ServerPool
    participant GL as GitLab API

    Client->>Handler: POST /mcp<br/>PRIVATE-TOKEN: glpat-abc123<br/>GITLAB-URL: https://gitlab.example.com
    Handler->>GS: getServer(req)
    GS->>GS: ExtractToken(req) → "glpat-abc123"
    GS->>GS: ExtractGitLabURL(req, default) → "https://gitlab.example.com"
    GS->>Pool: GetOrCreate("glpat-abc123", "https://gitlab.example.com")

    alt (token, url) not in pool
        Pool->>Pool: Check maxSize, evict LRU if full
        Pool->>Pool: NewClientWithToken(url, token, skipTLS)
        Pool->>Pool: factory(client) → *mcp.Server
        Pool-->>GS: *mcp.Server (new)
    else (token, url) already in pool
        Pool->>Pool: Move to front of LRU
        Pool-->>GS: *mcp.Server (cached)
    end

    GS-->>Handler: *mcp.Server
    Handler->>Handler: server.Connect() → new session
    Handler-->>Client: 200 OK + Mcp-Session-Id
```

## Shared Configuration

The following settings are **server-wide** — they apply to all clients regardless of their token or GitLab URL:

| Setting                     | Source                                                   | Description                                                                                                                                                                            |
| --------------------------- | -------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Fixed GitLab URL            | `--gitlab-url`                                           | Authoritative GitLab instance for all clients when set. If omitted, clients must send `GITLAB-URL`                                                                                     |
| TLS verification            | `--skip-tls-verify`                                      | Applied to all GitLab client connections                                                                                                                                               |
| Tool and capability surface | `--meta-tools`, `--tool-surface`, `--capability-surface` | Same tool catalog and resource/prompt exposure for all clients (`meta`/`individual`/`dynamic`; `full`/`minimal` capabilities); scope and CE/EE filtering still happen per server entry |
| Upload limits               | Compile-time defaults                                    | Max file size                                                                                                                                                                          |

The **GitLab token** always varies per client. The **GitLab URL** can vary per client only when `--gitlab-url` is omitted. Each unique `(token, URL)` pair creates a separate server-pool entry.

## Comparison with Stdio Mode

| Aspect                    | Stdio Mode                     | HTTP Mode                              |
| ------------------------- | ------------------------------ | -------------------------------------- |
| Configuration source      | Environment variables / `.env` | CLI flags                              |
| Token required at startup | Yes (`GITLAB_TOKEN`)           | No — per-request                       |
| Clients per process       | 1                              | Many (bounded by `--max-http-clients`) |
| Process lifecycle         | AI client spawns/kills         | Long-running daemon                    |
| Memory per client         | ~50 MB (full process)          | ~130 KB (pool entry)                   |
| Client isolation          | Process-level                  | Pool entry-level (same guarantees)     |
| Network requirement       | None (stdio pipes)             | TCP/HTTP                               |
| Session management        | SDK handles                    | SDK + server pool                      |

## Monitoring

### Server Logs

The server logs key events to stderr in JSON format:

```json
{"level":"INFO","msg":"starting MCP server in HTTP mode","addr":":8080","max_clients":100,"session_timeout":"30m0s"}
{"level":"INFO","msg":"server pool: created new entry","pool_size":1,"gitlab_url":"https://gitlab.com","enterprise":false,"enterprise_source":"detected","scopes_detected":true,"token_suffix":"...a1b2"}
{"level":"INFO","msg":"server pool: created new entry","pool_size":2,"gitlab_url":"https://gitlab.example.com","enterprise":true,"enterprise_source":"configured","scopes_detected":true,"token_suffix":"...c3d4"}
{"level":"WARN","msg":"request options ignored due to MCP configuration","ignored_options":["GITLAB-URL"],"token_suffix":"...a1b2"}
{"level":"INFO","msg":"server pool: evicted LRU entry","pool_size":99,"gitlab_url":"https://gitlab.com","enterprise":false}
{"level":"INFO","msg":"request rejected: missing authentication token (set PRIVATE-TOKEN header or Authorization: Bearer)"}
```

### Health Check

`GET /health` needs no credentials and answers `200` with a JSON body:

```bash
curl -s http://localhost:8080/health
```

```json
{
  "status": "ok",
  "version": "2.6.6",
  "commit": "318f49c1",
  "started_at": "2026-08-22T09:14:03Z",
  "uptime_seconds": 1209600
}
```

| Field            | Meaning                                                                                 |
| ---------------- | --------------------------------------------------------------------------------------- |
| `status`         | Always `ok` when the process is serving; the HTTP status carries liveness               |
| `version`        | Build version                                                                           |
| `commit`         | Build commit                                                                            |
| `started_at`     | Process start instant, RFC 3339 in UTC                                                  |
| `uptime_seconds` | Whole seconds since `started_at`, truncated (never a second that has not fully elapsed) |

Liveness is reported both ways on purpose. `started_at` is the stable fact — it is byte-identical across probes, so a monitor can cache it and detect a restart by noticing it moved, which is why Prometheus exposes `process_start_time_seconds` rather than an uptime counter. `uptime_seconds` is the derived convenience value, in the unit the [IETF health check draft](https://datatracker.ietf.org/doc/html/draft-inadarei-api-health-check-06) uses for it (`"observedUnit": "s"`).

`/health` reports only that the process is up; it performs no GitLab round-trip. To verify end-to-end connectivity for a specific token, call an authenticated MCP method:

```bash
curl -s -o /dev/null -w "%{http_code}" \
  -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "PRIVATE-TOKEN: glpat-your-token" \
  -d '{"jsonrpc":"2.0","method":"tools/list","id":1}'
# Expected: 200
```

## Security Considerations

- **Tokens in transit**: Use HTTPS in production or ensure the network is trusted
- **Tokens at rest**: Only SHA-256 hashes are stored in the pool; raw tokens are never persisted
- **Token logging**: Only the last 4 characters appear in logs
- **Pool isolation**: Each token gets a completely independent `*mcp.Server` — no shared state
- **Rate limiting**: Each client's GitLab token has its own rate limit bucket on the GitLab side (typically 300 req/min). The server also includes a per-IP authentication failure rate limiter (10 failures/min). When running behind a reverse proxy, configure `--trusted-proxy-header` so the rate limiter sees real client IPs instead of the proxy IP
- **No fallback token**: If a request has no token, it is rejected — there is no server-level default

## Troubleshooting

| Problem                                              | Cause                                                                                     | Solution                                                                                                                          |
| ---------------------------------------------------- | ----------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| `400 Bad Request`                                    | Missing or empty token header                                                             | Ensure `PRIVATE-TOKEN` or `Authorization: Bearer` is set                                                                          |
| `400 Bad Request`                                    | Invalid GitLab URL                                                                        | Verify `--gitlab-url` is correct and reachable                                                                                    |
| Tool errors after connecting                         | Invalid or expired token                                                                  | Verify the token has `api` scope and is not expired                                                                               |
| Pool eviction too frequent                           | Too many unique tokens                                                                    | Increase `--max-http-clients`                                                                                                     |
| Sessions expiring                                    | MCP idle timeout                                                                          | Increase `--session-timeout`                                                                                                      |
| Sessions drop every ~2 min / `keepalive ping failed` | HTTP idle/write timeout closing SSE streams (only if a low `--http-idle-timeout` was set) | Default `--http-idle-timeout=0` disables this; if set low, raise it or set `0`. Behind a proxy, also raise the proxy read timeout |
| Rate limiter blocks all clients                      | Behind reverse proxy, all requests share proxy IP                                         | Set `--trusted-proxy-header` to the header your proxy sets (e.g. `X-Forwarded-For`, `CF-Connecting-IP`)                           |

---

## Further Reading

- [Configuration](../reference/configuration.md) — full configuration reference
- [Architecture](../concepts/architecture.md) — system architecture with diagrams
- [Resource Consumption](../concepts/resource-consumption.md) — memory and CPU analysis at scale
- [Security](../concepts/security.md) — security model and best practices
- [OAuth App Setup](oauth-app-setup.md) — creating GitLab OAuth applications for MCP clients
- [IDE Configuration](ide-configuration.md) — per-IDE MCP JSON configuration examples
