# Resource Consumption

This document provides memory and CPU estimates for gitlab-mcp-server in both stdio and HTTP modes, helping operators plan capacity for deployments.

> **Diátaxis type**: Reference
> **Audience**: ⚙️ Server administrators
> **Prerequisites**: [HTTP Server Mode](../guides/http-server-mode.md), [Configuration](../reference/configuration.md)

---

## Baseline: Binary Footprint

The gitlab-mcp-server binary is a statically compiled Go executable:

| Metric                         | Value      |
| ------------------------------ | ---------- |
| Binary size (stripped)         | ~25 MB     |
| Go runtime overhead at startup | ~15 MB RSS |
| Initial heap after config load | ~20 MB     |

## Stdio Mode

In stdio mode, each AI client (VS Code, Cursor, Copilot CLI, OpenCode) spawns its own server process. The resource footprint is straightforward:

| Component                     | Memory     |
| ----------------------------- | ---------- |
| Go runtime + binary           | ~20 MB     |
| Config + GitLab client        | ~2 MB      |
| MCP server + registered tools | ~25 MB     |
| Tool execution working memory | ~5 MB      |
| **Total per process**         | **~50 MB** |

## HTTP Mode

In HTTP mode, a single process serves all clients. The base process uses ~50 MB (same as stdio). Each unique token adds a pool entry with its own MCP server and GitLab client.

### Per-Token Pool Entry Cost

| Component                                                        | Approximate Size |
| ---------------------------------------------------------------- | ---------------- |
| `*mcp.Server` instance (struct + options + session map)          | ~40 KB           |
| `*gitlabclient.Client` via `gl.NewClient()` (HTTP client + auth) | ~8 KB            |
| Tool registrations (854/1072/1078 individual or 32/49/50 meta)   | ~80 KB           |
| Resource registrations (45)                                      | ~8 KB            |
| Prompt registrations (37)                                        | ~5 KB            |
| **Total per unique token**                                       | **~130 KB**      |

### Scaling in HTTP Mode

| Unique Tokens     | Pool Memory | Total Process Memory | Notes                             |
| ----------------- | ----------- | -------------------- | --------------------------------- |
| 1                 | ~130 KB     | ~50 MB               | Equivalent to stdio               |
| 10                | ~1.3 MB     | ~51 MB               | Minimal overhead                  |
| 50                | ~6.5 MB     | ~57 MB               | Comfortable for a team            |
| 100 (default max) | ~13 MB      | ~63 MB               | Default `--max-http-clients`      |
| 500               | ~65 MB      | ~115 MB              | Requires `--max-http-clients=500` |
| 1000              | ~130 MB     | ~180 MB              | Large deployment                  |

### CPU Usage

CPU usage depends on request throughput and active resource
subscriptions, not pool size:

| Scenario                        | CPU Impact                                                                       |
| ------------------------------- | -------------------------------------------------------------------------------- |
| Idle pool entries               | Zero — unless a still-connected session holds resource subscriptions (see below) |
| Active MCP session (per client) | ~2 goroutines (read + write on transport)                                        |
| Tool execution                  | 1 goroutine per concurrent tool call                                             |
| GitLab API calls                | Blocked on network I/O, minimal CPU                                              |

100 active sessions ≈ 200 goroutines — negligible for the Go runtime.

### Goroutine Count

| Component                        | Goroutines |
| -------------------------------- | ---------- |
| Go runtime (GC, scheduler, etc.) | ~5         |
| HTTP server listener             | 1          |
| Per active MCP session           | ~2         |
| Per concurrent tool call         | 1          |

| Per watched URI (subscriptions to the same URI share one watcher) | 1, max 10 per server |

A server with 100 active sessions, 10 concurrent tool calls, and 10 watched URIs: ~230 goroutines total.

### Resource Subscription Watchers

On `GITLAB_MCP_CAPABILITY_SURFACE=full` (the default), a session that subscribes to a
resource ([subscriptions reference](../reference/capabilities/subscriptions.md))
starts — or joins — a watcher goroutine that polls GitLab in the background;
subscriptions to the same URI share one watcher, so goroutines count per
distinct watched URI — the one
kind of work this server performs without a request in flight:

- Up to **10 watchers per server** (per token+URL pool entry in HTTP mode),
  each one GitLab read per tick at an adaptive cadence: 5s while the
  resource is busy, 15s default, 60s settled, 10min once lease-demoted.
- Worst case **120 requests/minute per token** (10 watchers at the 5s
  floor), counted against that token's own GitLab rate limit.
- Watchers never outlive their subscribers: they stop when the last
  subscribing session disconnects, on a 401/403/404, at the 24h lifetime
  cap, or when evicted at the 10-watcher cap. A pool entry with **zero
  connected sessions runs zero watchers** — the non-zero idle case is a
  connected-but-quiet session that holds subscriptions, which demote to a
  10-minute poll after 30 minutes without traffic.

## What Counts as a "Connected Client"

Understanding the terminology is important for capacity planning:

| Term                  | Definition                                                           | Resource Impact                                                                |
| --------------------- | -------------------------------------------------------------------- | ------------------------------------------------------------------------------ |
| **Configured client** | User has the MCP server in their IDE config but hasn't sent requests | Zero — no session, no pool entry                                               |
| **Connected client**  | Client has sent a POST and received a `Mcp-Session-Id`               | 1 session (~2 goroutines)                                                      |
| **Active client**     | Connected client currently executing tool calls                      | Session + tool goroutines                                                      |
| **Idle client**       | Connected but no recent requests                                     | Session goroutines, plus up to 10 watcher goroutines if it holds subscriptions |
| **Unique token**      | Distinct GitLab PAT in the pool                                      | 1 pool entry (~130 KB)                                                         |

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

| Token Type            | Rate Limit (Default) |
| --------------------- | -------------------- |
| Personal Access Token | 300 requests/minute  |
| Project Access Token  | 300 requests/minute  |
| Group Access Token    | 300 requests/minute  |

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
- CPU: Light — Go handles thousands of lightweight request handlers efficiently

---

## Further Reading

- [HTTP Server Mode](../guides/http-server-mode.md) — architecture and configuration
- [Configuration](../reference/configuration.md) — all configuration options
- [Architecture](architecture.md) — system architecture overview
