# HTTP Server Mode

This document describes how gitlab-mcp-server operates in HTTP server mode, where multiple AI clients connect to a single shared server process over the network.

> **Diataxis type**: Explanation
> **Audience**: ⚙️ Server administrators
> **Prerequisites**: [Configuration](configuration.md), [Architecture](architecture.md)
> 📖 **User documentation**: See the [HTTP Server Mode](https://jmrplens.github.io/gitlab-mcp-server/operations/http-server/) on the documentation site for a user-friendly version.

---

## Overview

By default, gitlab-mcp-server runs in **stdio mode** — each AI client (VS Code, Cursor, Copilot CLI, OpenCode) spawns its own server process that communicates via stdin/stdout. This is simple but means each user runs a separate binary.

**HTTP mode** is an alternative transport where a single gitlab-mcp-server process listens on a network address and serves multiple clients simultaneously. Each client authenticates with its own GitLab Personal Access Token, and the server maintains an isolated MCP server instance per unique token.

### When to Use HTTP Mode

| Scenario | Recommended Mode |
| --- | --- |
| Single developer, local AI client | stdio |
| Team sharing one server instance | **HTTP** |
| Remote/headless server deployment | **HTTP** |
| CI/CD integration with MCP | **HTTP** (see [CI/CD Usage](ci-cd.md)) |
| Testing with curl or HTTP clients | **HTTP** |

## Starting the HTTP Server

HTTP mode is configured entirely via CLI flags — no environment variables are needed:

```bash
# Single GitLab instance (all clients use the same instance)
gitlab-mcp-server --http \
  --gitlab-url=https://gitlab.example.com \
  --http-addr=:8080

# Multi-instance (each client specifies their GitLab URL via GITLAB-URL header)
gitlab-mcp-server --http \
  --http-addr=:8080
```

### CLI Flags

| Flag | Default | Description |
| --- | --- | --- |
| `--http` | _(off)_ | Enable HTTP transport mode |
| `--gitlab-url` | _(optional)_ | Default GitLab instance URL. Can be overridden per-request via `GITLAB-URL` header |
| `--http-addr` | `:8080` | HTTP listen address (host:port) |
| `--skip-tls-verify` | `false` | Skip TLS certificate verification for self-signed certs |
| `--meta-tools` | `true` | Enable domain-level meta-tools (32 base, or 47 for Enterprise/Premium entries) instead of individual tools (1006) |
| `--enterprise` | `false` | Fallback for Enterprise/Premium tools when GitLab's version endpoint does not report edition. HTTP mode auto-detects CE/EE per token+URL pool entry when possible |
| `--max-http-clients` | `100` | Maximum unique token+URL entries in the server pool |
| `--session-timeout` | `30m` | Idle MCP session timeout |
| `--auth-mode` | `legacy` | Authentication mode: `legacy` (PRIVATE-TOKEN) or `oauth` (Bearer token verified via GitLab API) |
| `--oauth-cache-ttl` | `15m` | How long verified OAuth tokens are cached before re-validation (1m–2h) |
| `--revalidate-interval` | `15m` | Token re-validation interval; `0` to disable (upper bound: 24h) |
| `--trusted-proxy-header` | _(empty)_ | HTTP header containing the real client IP (e.g. `Fly-Client-IP`, `X-Forwarded-For`). Required for rate limiting behind reverse proxies |

> **Note**: `--gitlab-url` is optional. When omitted, each client must provide the `GITLAB-URL` header. When set, it serves as the default for clients that don't send the header.

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
- Each server instance detects token scopes and CE/EE edition independently, so multi-instance deployments can serve CE and EE GitLab instances from the same process when the version endpoint reports `enterprise`
- Zero cross-contamination between clients by construction

### Session Key Isolation

The pool key is `SHA-256(token + "\x00" + gitlabURL)`, never the raw values. This means:

- Raw tokens and URLs are never stored in memory beyond the initial request
- The same token against different GitLab instances produces different pool entries
- Log messages show only the last 4 characters of the token (e.g., `...a1b2`) for debugging
- Even if the pool's internal state were somehow exposed, tokens remain protected

## Client Authentication

Clients must provide their GitLab Personal Access Token on every HTTP request using one of two headers.

Optionally, clients can also specify which GitLab instance to target using the `GITLAB-URL` header:

```text
GITLAB-URL: https://gitlab.example.com
```

If omitted, the server uses the default `--gitlab-url` value. If `--gitlab-url` was also not set at startup, the request is rejected.

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

Requests without a valid token are rejected — the server returns no MCP session. The error is logged server-side:

```json
{"level":"ERROR","msg":"request rejected: missing authentication token (set PRIVATE-TOKEN header or Authorization: Bearer)"}
```

## Authentication Modes

HTTP mode supports two authentication modes controlled by `--auth-mode`:

### Mode Comparison

| Feature | Legacy (`--auth-mode=legacy`) | OAuth (`--auth-mode=oauth`) |
| --- | --- | --- |
| Token headers | `PRIVATE-TOKEN` or `Authorization: Bearer` | `Authorization: Bearer` (auto-converted from `PRIVATE-TOKEN`) |
| Server-side validation | None — token passed directly to GitLab API | Verified via `GET /api/v4/user` before MCP handler |
| Token caching | No caching — every API call uses the token directly | SHA-256 hashed cache with configurable TTL (default 15m) |
| Invalid token handling | Errors appear at GitLab API call time | Rejected immediately with HTTP 401 |
| RFC 9728 metadata | Not served | Served at `/.well-known/oauth-protected-resource` |
| MCP client OAuth discovery | Not supported | Supported — clients can discover the GitLab authorization server |
| Best for | Simple setups, internal networks | Production deployments, MCP clients with OAuth 2.1 support |

### Legacy Mode (default)

The default `--auth-mode=legacy` accepts tokens via `PRIVATE-TOKEN` or `Authorization: Bearer` headers without server-side verification. The token is passed directly to the GitLab API client. This mode is simple but the server trusts any token format — validation only happens when GitLab rejects an API call.

### OAuth Mode

`--auth-mode=oauth` enables server-side token verification using the go-sdk's `auth.RequireBearerToken` middleware. Every request is validated against GitLab's `/api/v4/user` endpoint before reaching the MCP handler.

```bash
gitlab-mcp-server --http \
  --gitlab-url=https://gitlab.example.com \
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

## Session Lifecycle

### 1. First Request

When a client sends its first HTTP POST to `/mcp`:

1. `StreamableHTTPHandler` calls the `getServer` callback
2. `ExtractToken()` reads the token and `ExtractGitLabURL()` resolves the GitLab URL (from the `GITLAB-URL` header or the `--gitlab-url` default)
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

| Setting | Source | Description |
| --- | --- | --- |
| Default GitLab URL | `--gitlab-url` | Fallback when the client does not send a `GITLAB-URL` header |
| TLS verification | `--skip-tls-verify` | Applied to all GitLab client connections |
| Meta-tools mode | `--meta-tools` | Same tool set for all clients |
| Upload limits | Compile-time defaults | Max file size |

Both the **GitLab token** and **GitLab URL** can vary per client. Each unique `(token, URL)` pair creates a separate server-pool entry.

## Comparison with Stdio Mode

| Aspect | Stdio Mode | HTTP Mode |
| --- | --- | --- |
| Configuration source | Environment variables / `.env` | CLI flags |
| Token required at startup | Yes (`GITLAB_TOKEN`) | No — per-request |
| Clients per process | 1 | Many (bounded by `--max-http-clients`) |
| Process lifecycle | AI client spawns/kills | Long-running daemon |
| Memory per client | ~50 MB (full process) | ~130 KB (pool entry) |
| Client isolation | Process-level | Pool entry-level (same guarantees) |
| Network requirement | None (stdio pipes) | TCP/HTTP |
| Session management | SDK handles | SDK + server pool |

## Monitoring

### Server Logs

The server logs key events to stderr in JSON format:

```json
{"level":"INFO","msg":"starting MCP server in HTTP mode","addr":":8080","max_clients":100,"session_timeout":"30m0s"}
{"level":"INFO","msg":"server pool: created new entry","pool_size":1,"token_suffix":"...a1b2"}
{"level":"INFO","msg":"server pool: created new entry","pool_size":2,"token_suffix":"...c3d4"}
{"level":"INFO","msg":"server pool: evicted LRU entry","pool_size":99"}
{"level":"ERROR","msg":"request rejected: missing authentication token (set PRIVATE-TOKEN header or Authorization: Bearer)"}
```

### Health Check

In HTTP mode, you can use the tools/list endpoint to verify the server is running:

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

| Problem | Cause | Solution |
| --- | --- | --- |
| `400 Bad Request` | Missing or empty token header | Ensure `PRIVATE-TOKEN` or `Authorization: Bearer` is set |
| `400 Bad Request` | Invalid GitLab URL | Verify `--gitlab-url` is correct and reachable |
| Tool errors after connecting | Invalid or expired token | Verify the token has `api` scope and is not expired |
| Pool eviction too frequent | Too many unique tokens | Increase `--max-http-clients` |
| Sessions expiring | Idle timeout | Increase `--session-timeout` |
| Rate limiter blocks all clients | Behind reverse proxy, all requests share proxy IP | Set `--trusted-proxy-header` to the header your proxy sets (e.g. `X-Forwarded-For`, `Fly-Client-IP`) |

---

## Further Reading

- [Configuration](configuration.md) — full configuration reference
- [Architecture](architecture.md) — system architecture with diagrams
- [Resource Consumption](resource-consumption.md) — memory and CPU analysis at scale
- [Security](security.md) — security model and best practices
- [OAuth App Setup](oauth-app-setup.md) — creating GitLab OAuth applications for MCP clients
- [IDE Configuration](ide-configuration.md) — per-IDE MCP JSON configuration examples
