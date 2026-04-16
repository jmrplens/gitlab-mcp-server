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
| CI/CD integration with MCP | **HTTP** |
| Testing with curl or HTTP clients | **HTTP** |

## Starting the HTTP Server

HTTP mode is configured entirely via CLI flags — no environment variables are needed:

```bash
gitlab-mcp-server --http \
  --gitlab-url=https://gitlab.example.com \
  --skip-tls-verify \
  --meta-tools \
  --http-addr=:8080
```

### CLI Flags

| Flag | Default | Description |
| --- | --- | --- |
| `--http` | _(off)_ | Enable HTTP transport mode |
| `--gitlab-url` | _(required)_ | GitLab instance base URL |
| `--http-addr` | `:8080` | HTTP listen address (host:port) |
| `--skip-tls-verify` | `false` | Skip TLS certificate verification for self-signed certs |
| `--meta-tools` | `true` | Enable domain-level meta-tools (40, or 59 with --enterprise) instead of individual tools (1004) |
| `--enterprise` | `false` | Enable Enterprise/Premium meta-tools (19 additional domain tools) |
| `--max-http-clients` | `100` | Maximum unique tokens in the server pool |
| `--session-timeout` | `30m` | Idle MCP session timeout |

> **Note**: `--gitlab-url` is the only required flag. All others have sensible defaults.

## Architecture

### Server Pool

The core of HTTP mode is the **Server Pool** (`internal/serverpool`), a bounded cache of MCP server instances keyed by the SHA-256 hash of each client's GitLab token.

```mermaid
graph TD
    subgraph "HTTP Mode Architecture"
        REQ1[Client A<br/>Token: glpat-aaa] --> HANDLER[StreamableHTTPHandler]
        REQ2[Client B<br/>Token: glpat-bbb] --> HANDLER
        REQ3[Client C<br/>Token: glpat-aaa] --> HANDLER

        HANDLER --> GS[getServer callback]
        GS --> EXTRACT[ExtractToken from headers]
        EXTRACT --> POOL[ServerPool]

        POOL --> ENTRY1["Entry: hash(glpat-aaa)<br/>*mcp.Server + *gitlab.Client"]
        POOL --> ENTRY2["Entry: hash(glpat-bbb)<br/>*mcp.Server + *gitlab.Client"]
    end

    ENTRY1 --> GL1[GitLab API<br/>as user A]
    ENTRY2 --> GL2[GitLab API<br/>as user B]
```

**Key properties:**

- Clients with the **same token** share the same `*mcp.Server` instance (Client A and Client C above)
- Clients with **different tokens** get completely isolated server instances
- Each server instance has its own GitLab client authenticated with that specific token
- Zero cross-contamination between clients by construction

### Token Isolation

The pool key is `SHA-256(token)`, never the raw token. This means:

- Raw tokens are never stored in memory beyond the initial request
- Log messages show only the last 4 characters (e.g., `...a1b2`) for debugging
- Even if the pool's internal state were somehow exposed, tokens remain protected

## Client Authentication

Clients must provide their GitLab Personal Access Token on every HTTP request using one of two headers:

### Option 1: PRIVATE-TOKEN Header (Recommended)

```text
PRIVATE-TOKEN: glpat-xxxxxxxxxxxxxxxxxxxx
```

This is the standard GitLab authentication header and takes precedence over Bearer.

### Option 2: Authorization Bearer Header

```text
Authorization: Bearer glpat-xxxxxxxxxxxxxxxxxxxx
```

Standard OAuth2-style bearer token, supported for compatibility.

### Precedence

If both headers are present, `PRIVATE-TOKEN` wins.

### Missing Token

Requests without a valid token are rejected — the server returns no MCP session. The error is logged server-side:

```json
{"level":"ERROR","msg":"request rejected: missing authentication token (set PRIVATE-TOKEN header or Authorization: Bearer)"}
```

## Client Configuration Examples

### VS Code / GitHub Copilot

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

### OpenCode

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
2. `ExtractToken()` reads the token from request headers
3. `ServerPool.GetOrCreate()` hashes the token and checks the pool
4. If the token is new: creates a GitLab client + MCP server, registers all tools/resources/prompts
5. Returns the `*mcp.Server` for that token
6. SDK establishes an MCP session and returns a `Mcp-Session-Id` header

### 2. Subsequent Requests

Subsequent requests with the same token:

1. Token is extracted and hashed
2. Pool finds the existing entry and promotes it in the LRU list
3. Same `*mcp.Server` is returned — session state is preserved

### 3. Session Timeout

If a client is idle for longer than `--session-timeout` (default: 30 minutes):

1. The MCP SDK closes the idle session (HTTP transport level)
2. The pool entry (server + client) **remains** in the pool
3. Next request from the same token creates a new MCP session on the existing server

### 4. Pool Eviction

When the pool is full (`--max-http-clients` reached) and a new token arrives:

1. The **least recently used** pool entry is evicted
2. The evicted server and GitLab client are removed from the pool
3. A new entry is created for the new token
4. If the evicted client reconnects, a fresh server is created

```mermaid
sequenceDiagram
    participant Client as HTTP Client
    participant Handler as StreamableHTTPHandler
    participant GS as getServer callback
    participant Pool as ServerPool
    participant GL as GitLab API

    Client->>Handler: POST /mcp<br/>PRIVATE-TOKEN: glpat-abc123
    Handler->>GS: getServer(req)
    GS->>GS: ExtractToken(req) → "glpat-abc123"
    GS->>Pool: GetOrCreate("glpat-abc123")

    alt Token not in pool
        Pool->>Pool: Check maxSize, evict LRU if full
        Pool->>Pool: NewClientWithToken(url, token, skipTLS)
        Pool->>Pool: factory(client) → *mcp.Server
        Pool-->>GS: *mcp.Server (new)
    else Token already in pool
        Pool->>Pool: Move to front of LRU
        Pool-->>GS: *mcp.Server (cached)
    end

    GS-->>Handler: *mcp.Server
    Handler->>Handler: server.Connect() → new session
    Handler-->>Client: 200 OK + Mcp-Session-Id
```

## Shared Configuration

The following settings are **server-wide** — they apply to all clients regardless of their token:

| Setting | Source | Description |
| --- | --- | --- |
| GitLab URL | `--gitlab-url` | All clients connect to the same GitLab instance |
| TLS verification | `--skip-tls-verify` | Applied to all GitLab client connections |
| Meta-tools mode | `--meta-tools` | Same tool set for all clients |
| Upload limits | Compile-time defaults | Max file size |

Only the **GitLab token** varies per client.

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
- **Rate limiting**: Each client's GitLab token has its own rate limit bucket on the GitLab side (typically 300 req/min)
- **No fallback token**: If a request has no token, it is rejected — there is no server-level default

## Troubleshooting

| Problem | Cause | Solution |
| --- | --- | --- |
| `400 Bad Request` | Missing or empty token header | Ensure `PRIVATE-TOKEN` or `Authorization: Bearer` is set |
| `400 Bad Request` | Invalid GitLab URL | Verify `--gitlab-url` is correct and reachable |
| Tool errors after connecting | Invalid or expired token | Verify the token has `api` scope and is not expired |
| Pool eviction too frequent | Too many unique tokens | Increase `--max-http-clients` |
| Sessions expiring | Idle timeout | Increase `--session-timeout` |

---

## Further Reading

- [Configuration](configuration.md) — full configuration reference
- [Architecture](architecture.md) — system architecture with diagrams
- [Resource Consumption](resource-consumption.md) — memory and CPU analysis at scale
- [Security](security.md) — security model and best practices
