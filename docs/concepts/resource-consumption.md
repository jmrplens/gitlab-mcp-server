# Resource Consumption

This document describes the memory and CPU cost of gitlab-mcp-server in both stdio and HTTP modes, helping operators plan capacity for deployments. The figures are measured, not estimated: they come from `make bench-resources` on one machine and one build, published with their charts in the [Resource benchmark](../reference/resource-benchmark.md). The shape of the numbers carries to other hosts; the absolute values will not.

> **Diátaxis type**: Reference
> **Audience**: ⚙️ Server administrators
> **Prerequisites**: [HTTP Server Mode](../guides/http-server-mode.md), [Configuration](../reference/configuration.md)

---

## Baseline: Binary Footprint

The gitlab-mcp-server binary is a statically compiled Go executable:

| Metric                                       | Value                               |
| -------------------------------------------- | ----------------------------------- |
| Binary size (stripped release build)         | ~49 MB                              |
| HTTP process idle, before any credential     | ~40 MiB RSS                         |
| stdio process with one client, catalog built | 190 to 330 MiB RSS, by tool surface |

Resident set is read from the kernel rather than from Go's heap accounting, because the resident set is what a container limit is measured against and the two differ by more than a factor of two.

## Stdio Mode

In stdio mode, each AI client (VS Code, Cursor, Copilot CLI, OpenCode) spawns its own server process. The process starts building its tool catalog on a background goroutine as soon as it is executed, so there is no idle state: it reaches its working size within about two seconds, and the surface decides where in the range it lands.

| Tool surface | One process | Each further process |
| ------------ | ----------: | -------------------: |
| `dynamic`    |    ~211 MiB |             ~217 MiB |
| `meta`       |    ~193 MiB |             ~196 MiB |
| `individual` |    ~331 MiB |             ~296 MiB |

Four stdio clients on one host cost 0.8 to 1.2 GiB together. `dynamic` is the largest per process despite registering two tools, because it builds a search index the other two surfaces do not.

## HTTP Mode

In HTTP mode, a single process serves all clients. Idle, before any credential has arrived, it holds about 40 MiB and no tool catalog at all. The first request from each distinct token builds one, so memory follows the number of live credentials rather than the number of sessions or requests.

### Per-Token Pool Entry Cost

> **The figures in this section predate the sharing work and overstate what a
> credential now costs.** They were measured when a pool entry was the whole
> catalog for that token: its own MCP server, GitLab client, registered tools,
> resources and prompts. Since then the catalog and its schemas are built once
> per configuration and shared, and the MCP server itself is built once per
> configuration shape and shared by every credential that hashes to it
> ([ADR-0020](../development/adr/adr-0020-one-server-per-configuration-shape.md)).
> A pool entry is now credential state: a GitLab client, a rate-limit bucket, a
> listen counter, a watcher set, and the sessions that credential holds open.
>
> Measured locally on the development machine, the live heap an idle process
> holds per additional credential fell from 434 KiB to 7.7 on `dynamic`, 815 to
> 8.3 on `meta` and 1,487 to 8.5 on `individual`: the three surfaces now cost
> the same, because what a credential costs no longer depends on which tools are
> registered. The before and after runs are written up in
> [Resource hot spots](../development/resource-hot-spots.md#what-the-shared-server-measured).
> The tables below stay as they were until the reference host they were taken on
> is re-run.

| Tool surface | Process with one credential | Each further credential |
| ------------ | --------------------------: | ----------------------: |
| `dynamic`    |                    ~219 MiB |                 ~71 MiB |
| `meta`       |                    ~197 MiB |                 ~35 MiB |
| `individual` |                    ~334 MiB |                 ~36 MiB |

Memory per credential used to go the other way from what the tool counts suggest: `individual` cost the least per additional credential despite registering about a thousand tools, because pooled entries already shared their tool schemas, while `dynamic` cost the most because every entry carried its own search index. Both of those causes are gone.

### Scaling in HTTP Mode

| Live credentials  | Added by the pool | Total process memory                              | Notes                                                           |
| ----------------- | ----------------- | ------------------------------------------------- | --------------------------------------------------------------- |
| 1                 | 35 to 71 MiB      | ~200 to 335 MiB                                   | Equivalent to one stdio process                                 |
| 8                 | 0.3 to 0.6 GiB    | 445 to 719 MiB at rest, 0.6 to 1.1 GiB under load | Measured, all eight calling at once for the peak                |
| 20                | 0.7 to 1.4 GiB    | ~1 to 2 GiB                                       | A small team                                                    |
| 100 (default max) | 3.5 to 7 GiB      | ~4 to 8 GiB                                       | Default `--max-http-clients`; a pool no small instance can hold |

Those totals are the pre-sharing ones and are now an upper bound rather than an estimate. What a credential adds at rest is small; what still grows with the credential count is the work in flight, since each live client's requests allocate while they are served. Size from concurrent load rather than from the number of tokens, and treat `--max-http-clients` as a bound on pooled credentials rather than as the memory setting it used to be. `--pool-idle-timeout` (default 1h) reclaims entries nobody has used, so the live count is what matters, not how many tokens have ever connected; an entry rebuilt after reclamation no longer pays for a catalog build unless it is the only credential of its configuration, because the built server is shared. An entry with a live subscription is exempt from that sweep and is preferred over by size pressure, since its watcher polls GitLab directly and its listen is one request the client never repeats, so nothing about it would look busy to the pool.

### CPU Usage

CPU usage depends on request throughput and active resource
subscriptions, not pool size:

| Scenario                                         | CPU Impact                                                                       |
| ------------------------------------------------ | -------------------------------------------------------------------------------- |
| Idle pool entries                                | Zero — unless a still-connected session holds resource subscriptions (see below) |
| Catalog build (first request of each credential) | About 2 seconds of one core on `dynamic` and `meta`, 4 to 5 on `individual`      |
| Open session (`--stateless=false`)               | ~2 goroutines (read + write on transport)                                        |
| Tool execution                                   | 1 goroutine per concurrent tool call                                             |
| GitLab API calls                                 | Blocked on network I/O, minimal CPU                                              |

Under the default stateless transport a session lasts exactly one POST, so there is no per-client goroutine cost between requests.

### Goroutine Count

| Component                                                         | Goroutines           |
| ----------------------------------------------------------------- | -------------------- |
| Go runtime plus server baseline (measured at rest on stdio)       | ~26 to 28            |
| HTTP server listener                                              | 1                    |
| Per open stateful session                                         | ~2                   |
| Per concurrent tool call                                          | 1                    |
| Per watched URI (subscriptions to the same URI share one watcher) | 1, max 10 per server |

An HTTP process holding eight credentials measured 52 to 75 goroutines at rest; add one per in-flight call and one per watched URI.

### Resource Subscription Watchers

On `GITLAB_MCP_CAPABILITY_SURFACE=full` (the default), a session that subscribes to a
resource ([subscriptions reference](../reference/capabilities/subscriptions.md))
starts — or joins — a watcher goroutine that polls GitLab in the background;
subscriptions to the same URI share one watcher, so goroutines count per
distinct watched URI — the one
kind of work this server performs without a request in flight:

- Up to **10 watchers per credential** (per token+URL pool entry in HTTP mode,
  which is what owns them even where several credentials share one MCP server),
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

| Term                  | Definition                                                                                                                                              | Resource Impact                                                                                 |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------- |
| **Configured client** | User has the MCP server in their IDE config but hasn't sent requests                                                                                    | Zero — no session, no pool entry                                                                |
| **Connected client**  | Client has sent a POST; under the default stateless transport its session ends with the response, under `--stateless=false` it holds a `Mcp-Session-Id` | 1 pool entry for its token; a session (~2 goroutines) only in stateful mode                     |
| **Active client**     | Connected client currently executing tool calls                                                                                                         | Pool entry + tool goroutines                                                                    |
| **Idle client**       | Connected but no recent requests                                                                                                                        | Session goroutines in stateful mode, plus up to 10 watcher goroutines if it holds subscriptions |
| **Unique token**      | Distinct GitLab PAT in the pool                                                                                                                         | 1 pool entry: a client, a rate-limit bucket, a counter and its watchers, not a tool catalog     |

**Key insight**: Multiple sessions from the same token share one pool entry. A user with 3 IDE windows using the same token = 3 sessions, 1 pool entry.

## Memory Pressure Sources

Memory growth comes from:

1. **Distinct configurations served**. Each builds one tool catalog and one MCP server, shared by every credential that hashes to it. In a deployment that pins `--tier` and publishes one instance there is exactly one
2. **Unique tokens in pool**. Each adds credential state only, bounded by `--max-http-clients` and reclaimed by `--pool-idle-timeout`, which exempts an entry that is still serving a subscription
3. **Active MCP sessions** — minimal per-session overhead managed by the SDK
4. **Tool execution** — temporary allocations during GitLab API calls (GC reclaims)
5. **Large API responses** — paginated list results with many items

The catalogs are the persistent growth, and there are as many of them as there are configurations; the pool is bounded and no longer carries one each.

## GitLab API Rate Limits

Each token has its own rate limit on the GitLab side:

| Token Type            | Rate Limit (Default) |
| --------------------- | -------------------- |
| Personal Access Token | 300 requests/minute  |
| Project Access Token  | 300 requests/minute  |
| Group Access Token    | 300 requests/minute  |

The server pool does NOT aggregate tokens — each client is independently rate-limited by GitLab. A server with 100 unique tokens can collectively make up to 30,000 requests/minute to GitLab.

## Capacity Planning Recommendations

Size from the load rather than from the token count: one built catalog per configuration served, plus what the concurrent requests allocate while they run. The figures above are the pre-sharing ones and are now an upper bound. `--session-timeout` only matters with `--stateless=false`.

### Small Team (5-20 developers)

```bash
gitlab-mcp-server --http \
  --gitlab-url=https://gitlab.example.com \
  --max-http-clients=20 \
  --http-addr=:8080
```

- Memory: 1 to 2 GiB with every developer live at once
- CPU: Negligible between requests; about 2 seconds of one core per catalog build

### Medium Team (20-100 developers)

```bash
gitlab-mcp-server --http \
  --gitlab-url=https://gitlab.example.com \
  --max-http-clients=100 \
  --http-addr=:8080
```

- Memory: 4 to 8 GiB at the pool bound; `--pool-idle-timeout` keeps the live count below it in practice
- CPU: Minimal

### Large Deployment (100+ developers)

Rather than one process with a thousand-entry pool, run several instances behind a balancer that routes on the credential (a hash of the bearer token), because the pool and the OAuth identity cache are per instance and a client that lands on a different instance rebuilds its entry there.

```bash
gitlab-mcp-server --http \
  --gitlab-url=https://gitlab.example.com \
  --max-http-clients=100 \
  --http-addr=:8080
```

- Memory: 4 to 8 GiB per instance at the pool bound
- CPU: Light — Go handles thousands of lightweight request handlers efficiently

---

## Further Reading

- [HTTP Server Mode](../guides/http-server-mode.md) — architecture and configuration
- [Configuration](../reference/configuration.md) — all configuration options
- [Architecture](architecture.md) — system architecture overview
