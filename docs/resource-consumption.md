# Resource Consumption

This document provides memory and CPU estimates for gitlab-mcp-server in both stdio and HTTP modes, helping operators plan capacity for deployments.

> **Diataxis type**: Reference
> **Audience**: ⚙️ Server administrators
> **Prerequisites**: [HTTP Server Mode](http-server-mode.md), [Configuration](configuration.md)

---

## Baseline: Binary Footprint

The gitlab-mcp-server binary is a statically compiled Go executable:

| Metric | Value |
| --- | --- |
| Binary size (stripped) | ~25 MB |
| Go runtime overhead at startup | ~15 MB RSS |
| Initial heap after config load | ~20 MB |

## Stdio Mode

In stdio mode, each AI client (VS Code, Cursor, Copilot CLI, OpenCode) spawns its own server process. The resource footprint is straightforward:

| Component | Memory |
| --- | --- |
| Go runtime + binary | ~20 MB |
| Config + GitLab client | ~2 MB |
| MCP server + registered tools | ~25 MB |
| Tool execution working memory | ~5 MB |
| **Total per process** | **~50 MB** |

## HTTP Mode

In HTTP mode, a single process serves all clients. The base process uses ~50 MB (same as stdio). Each unique token adds a pool entry with its own MCP server and GitLab client.

### Per-Token Pool Entry Cost

| Component | Approximate Size |
| --- | --- |
| `*mcp.Server` instance (struct + options + session map) | ~40 KB |
| `*gitlabclient.Client` via `gl.NewClient()` (HTTP client + auth) | ~8 KB |
| Tool registrations (1004 individual or 40/59 meta) | ~80 KB |
| Resource registrations (19) | ~5 KB |
| Prompt registrations (38) | ~5 KB |
| **Total per unique token** | **~130 KB** |

### Scaling in HTTP Mode

| Unique Tokens | Pool Memory | Total Process Memory | Notes |
| --- | --- | --- | --- |
| 1 | ~130 KB | ~50 MB | Equivalent to stdio |
| 10 | ~1.3 MB | ~51 MB | Minimal overhead |
| 50 | ~6.5 MB | ~57 MB | Comfortable for a team |
| 100 (default max) | ~13 MB | ~63 MB | Default `--max-http-clients` |
| 500 | ~65 MB | ~115 MB | Requires `--max-http-clients=500` |
| 1000 | ~130 MB | ~180 MB | Large deployment |

### CPU Usage

CPU usage depends on request throughput, not pool size:

| Scenario | CPU Impact |
| --- | --- |
| Idle pool entries | Zero — no goroutines, no timers |
| Active MCP session (per client) | ~2 goroutines (read + write on transport) |
| Tool execution | 1 goroutine per concurrent tool call |
| GitLab API calls | Blocked on network I/O, minimal CPU |

100 active sessions ≈ 200 goroutines — negligible for the Go runtime.

### Goroutine Count

| Component | Goroutines |
| --- | --- |
| Go runtime (GC, scheduler, etc.) | ~5 |
| HTTP server listener | 1 |
| Per active MCP session | ~2 |
| Per concurrent tool call | 1 |

A server with 100 active sessions and 10 concurrent tool calls: ~220 goroutines total.

## What Counts as a "Connected Client"

Understanding the terminology is important for capacity planning:

| Term | Definition | Resource Impact |
| --- | --- | --- |
| **Configured client** | User has the MCP server in their IDE config but hasn't sent requests | Zero — no session, no pool entry |
| **Connected client** | Client has sent a POST and received a `Mcp-Session-Id` | 1 session (~2 goroutines) |
| **Active client** | Connected client currently executing tool calls | Session + tool goroutines |
| **Idle client** | Connected but no recent requests | Session goroutines only |
| **Unique token** | Distinct GitLab PAT in the pool | 1 pool entry (~130 KB) |

**Key insight**: Multiple sessions from the same token share one pool entry. A user with 3 IDE windows using the same token = 3 sessions, 1 pool entry.

## Memory Pressure Sources

Memory growth comes from:

1. **Unique tokens in pool** — each adds ~130 KB (bounded by `--max-http-clients`)
2. **Active MCP sessions** — minimal per-session overhead managed by the SDK
3. **Tool execution** — temporary allocations during GitLab API calls (GC reclaims)
4. **Large API responses** — paginated list results with many items

The pool is the only source of **persistent** memory growth, and it is bounded.

## GitLab API Rate Limits

Each token has its own rate limit on the GitLab side:

| Token Type | Rate Limit (Default) |
| --- | --- |
| Personal Access Token | 300 requests/minute |
| Project Access Token | 300 requests/minute |
| Group Access Token | 300 requests/minute |

The server pool does NOT aggregate tokens — each client is independently rate-limited by GitLab. A server with 100 unique tokens can collectively make up to 30,000 requests/minute to GitLab.

## Capacity Planning Recommendations

### Small Team (5-20 developers)

```bash
gitlab-mcp-server --http \
  --gitlab-url=https://gitlab.example.com \
  --max-http-clients=50 \
  --session-timeout=30m \
  --http-addr=:8080
```

- Memory: ~57 MB
- CPU: Negligible

### Medium Team (20-100 developers)

```bash
gitlab-mcp-server --http \
  --gitlab-url=https://gitlab.example.com \
  --max-http-clients=200 \
  --session-timeout=1h \
  --http-addr=:8080
```

- Memory: ~76 MB
- CPU: Minimal

### Large Deployment (100+ developers)

```bash
gitlab-mcp-server --http \
  --gitlab-url=https://gitlab.example.com \
  --max-http-clients=1000 \
  --session-timeout=1h \
  --http-addr=:8080
```

- Memory: ~180 MB
- CPU: Light — Go handles 1000+ goroutines efficiently

---

## Further Reading

- [HTTP Server Mode](http-server-mode.md) — architecture and configuration
- [Configuration](configuration.md) — all configuration options
- [Architecture](architecture.md) — system architecture overview
