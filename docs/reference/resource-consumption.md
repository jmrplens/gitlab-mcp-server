# Resource Consumption

This document describes the memory and CPU cost of gitlab-mcp-server in both stdio and HTTP modes, helping operators plan capacity for deployments. The figures are measured, not estimated: they come from `make bench-resources` on one machine and one build, published with their charts in the [Resource benchmark](resource-benchmark.md). That machine is an Intel i5-14400 with 16 logical CPUs and 62 GiB of RAM, on linux/amd64, kernel 6.12 and Go 1.27.1; the shape of the numbers carries to other hosts, the absolute values will not.

**Every figure here is a snapshot of the current build.** The reference host is re-measured for each release and these values are replaced wholesale, with no comparison against what they were: a ratio against a run whose code no longer exists says nothing about the instance you are sizing. What moved between releases is [Resource Hot Spots](../development/resource-hot-spots.md).

**Two costs, not one.** Holding a credential and serving one are separate questions here, and the answers are three orders of magnitude apart: a pooled credential is tens of kilobytes, while a credential with requests in flight is megabytes. Size an instance from how many callers will be calling at the same moment. `--max-http-clients` bounds the first of those and is not a memory setting.

> **Diátaxis type**: Reference
> **Audience**: ⚙️ Server administrators
> **Prerequisites**: [HTTP Server Mode](../guides/http-server-mode.md), [Configuration](configuration.md)

---

## Baseline: Binary Footprint

The gitlab-mcp-server binary is a statically compiled Go executable:

| Metric                                       | Value                               |
| -------------------------------------------- | ----------------------------------- |
| Binary size (stripped release build)         | ~55 MB                              |
| HTTP process idle, before any credential     | 36 to 40 MiB RSS                    |
| stdio process with one client, catalog built | 108 to 254 MiB RSS, by tool surface |

Resident set is read from the kernel rather than from Go's heap accounting, because the resident set is what a container limit is measured against and the two differ by more than a factor of two.

## Stdio Mode

In stdio mode, each AI client (VS Code, Cursor, Copilot CLI, OpenCode) spawns its own server process. The process starts building its tool catalog on a background goroutine as soon as it is executed, so there is no idle state: it reaches its working size within about two seconds, and the surface decides where in the range it lands.

| Tool surface | One process | Each further process |
| ------------ | ----------: | -------------------: |
| `dynamic`    |    ~108 MiB |             ~110 MiB |
| `meta`       |    ~109 MiB |             ~109 MiB |
| `individual` |    ~254 MiB |             ~270 MiB |

The cost is a straight line in the client count: nothing is shared between processes, and a stdio process builds its own catalog and pays for it, which is the whole difference between this transport and HTTP mode. The benchmark runs eight of them per scenario, and its "all clients" column is what eight weigh together. `dynamic` and `meta` cost the same per process to within a mebibyte; `individual` is the outlier, at roughly two and a half times either, because its registered surface is about a thousand tools with their schemas.

## HTTP Mode

In HTTP mode, a single process serves all clients. Idle, before any credential has arrived, it holds 36 to 40 MiB and no tool catalog at all. The first credential of a configuration builds one, and every later credential of that configuration finds it ready, so memory follows the requests in flight rather than the number of credentials, sessions or tokens.

### Per-Token Pool Entry Cost

A pool entry is credential state: a GitLab client, a rate-limit bucket, a listen counter, a watcher set, and the sessions that credential holds open. The catalog and its schemas are built once per configuration and shared, and the MCP server itself is built once per configuration shape and shared by every credential that hashes to it ([ADR-0020](../development/adr/adr-0020-one-server-per-configuration-shape.md)). What a credential costs to hold is therefore independent of which tools are registered.

The concurrency series measures it directly, at every step from one credential to a thousand, as a live heap read with the load stopped and a collection forced:

| Tool surface | Each further credential, settled live heap | A thousand credentials, settled live heap | Settled resident set |
| ------------ | -----------------------------------------: | ----------------------------------------: | -------------------: |
| `dynamic`    |                                   50.9 KiB |                                  88.7 MiB |             0.31 GiB |
| `meta`       |                                   52.1 KiB |                                  83.1 MiB |             0.24 GiB |
| `individual` |                                   29.1 KiB |                                 116.6 MiB |             0.31 GiB |

The three surfaces agree to within about 20 KiB, across registered tool counts that differ by a factor of five hundred. That is the whole point of the figure: what a credential holds is its client and its bookkeeping, and the tools are somewhere else.

**Two at-rest figures appear in this documentation and they measure different things.** The end-to-end test `TestSharedServer_LiveHeapDoesNotGrowWithTheNumberOfCredentials`, run on every push, reports about 8 KiB per credential; the series above reports 29 to 52 KiB. The test's clients complete their `tools/list` and disconnect, so it measures a credential **with no connection open**: the pool entry alone. The benchmark driver keeps its sockets open across steps, four per credential on `dynamic` and `meta` and two on `individual`, so its figure is the pool entry plus the buffers behind those connections, at roughly 11 KiB apiece. Size against the series figure, since a credential you are sizing for is a connected one.

### Scaling in HTTP Mode

Two numbers per row, because holding credentials and serving them are different costs. The tenancy column is the settled live heap; the load column is the peak resident set with every credential keeping two to four requests in flight, which is the figure a container limit has to survive.

| Live credentials  | Tenancy, added to the live heap | Peak resident set with all of them calling | Notes                                                                    |
| ----------------- | ------------------------------- | ------------------------------------------ | ------------------------------------------------------------------------ |
| 1                 | Nothing measurable              | 147 to 301 MiB                             | Equivalent to one stdio process                                          |
| 20                | ~1 MiB                          | 271 to 843 MiB                             | A small team                                                             |
| 64                | 2 to 3 MiB                      | Read the point scenarios' peak column      | The point scenarios' ladder, admitted one at a time and then all calling |
| 100 (default max) | 3 to 5 MiB                      | 0.5 to 2.0 GiB                             | Default `--max-http-clients`                                             |
| 1000              | 28 to 51 MiB                    | 1.0 to 4.4 GiB                             | Measured; the series runs every step to this count on all three surfaces |

**`--max-http-clients` is not a memory setting.** At 50 KiB per entry its default of 100 bounds five mebibytes of tenancy, so sizing an instance against it is wrong in both directions: it neither reserves that memory nor limits what the callers behind those credentials allocate while their requests are served. Size from how many callers will have requests in flight at the same moment, which is the load column above. `--pool-idle-timeout` (default 1h) reclaims entries nobody has used, so the live count is what matters, not how many tokens have ever connected; an entry rebuilt after reclamation no longer pays for a catalog build unless it is the only credential of its configuration, because the built server is shared. An entry with a live subscription is exempt from that sweep and is preferred over by size pressure, since its watcher polls GitLab directly and its listen is one request the client never repeats, so nothing about it would look busy to the pool.

### CPU Usage

CPU usage depends on request throughput and active resource
subscriptions, not pool size:

| Scenario                                            | CPU Impact                                                                                                                                                                          |
| --------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Idle pool entries                                   | Zero — unless a still-connected session holds resource subscriptions (see below)                                                                                                    |
| Catalog build (first credential of a configuration) | The first `tools/list` took 0.39 s on `dynamic`, 0.41 s on `meta` and 1.27 s on `individual`                                                                                        |
| Serving one call                                    | About 8 ms of processor time on `dynamic` and `meta`; 105 ms for the average call on `individual`, where every second call is a `tools/list` serialising three megabytes of schemas |
| Open session (`--stateless=false`)                  | ~2 goroutines (read + write on transport)                                                                                                                                           |
| Tool execution                                      | 1 goroutine per concurrent tool call                                                                                                                                                |
| GitLab API calls                                    | Blocked on network I/O, minimal CPU                                                                                                                                                 |

Under the default stateless transport a session lasts exactly one POST, so there is no per-client goroutine cost between requests.

### Goroutine Count

| Component                                                               | Goroutines           |
| ----------------------------------------------------------------------- | -------------------- |
| Go runtime plus server baseline (HTTP, one credential, series step one) | 18 to 20             |
| HTTP server listener                                                    | 1                    |
| Per open stateful session                                               | ~2                   |
| Per concurrent tool call                                                | 1                    |
| Per watched URI (subscriptions to the same URI share one watcher)       | 1, max 10 per server |

The point scenarios read the count off a traceback signal with every credential attached; the concurrency series makes the shape plain, because it holds four requests in flight per credential: 4,015 goroutines at a thousand credentials on `dynamic` and `meta`, and 2,015 on `individual`, where the driver holds two. Goroutines track the requests in flight, not the pool.

### Resource Subscription Watchers

On `GITLAB_MCP_CAPABILITY_SURFACE=full` (the default), a session that subscribes to a
resource ([subscriptions reference](capabilities/subscriptions.md))
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

Memory growth comes from, in the order the benchmark's heap profiles rank them at a thousand credentials:

1. **Requests in flight**: the JSON being decoded and encoded, and the read and write buffer of every open connection. At a thousand credentials with four requests each, those buffers alone were 29.6 MiB of live heap on `dynamic`, the largest single term that grows at all
2. **Unique tokens in pool**. Each adds credential state only, principally its GitLab client at about 5.6 KiB: 50.9 KiB per credential of settled live heap on `dynamic` including the connections it holds open. Bounded by `--max-http-clients` and reclaimed by `--pool-idle-timeout`, which exempts an entry that is still serving a subscription
3. **Distinct configurations served**. Each builds one tool catalog and one MCP server, shared by every credential that hashes to it. In a deployment that pins `--tier` and publishes one instance there is exactly one, and it does not grow: `toolutil.cloneSchemaMap` holds 8.50 MiB in the heap profile at one credential and 8.50 MiB at a thousand
4. **Active MCP sessions** — minimal per-session overhead managed by the SDK
5. **Large API responses** — paginated list results with many items

The catalogs are the persistent cost and there are as many of them as there are configurations, but they are a fixed cost rather than growth; what grows is the concurrent work, and the pool contributes kilobytes per credential to it.

## GitLab API Rate Limits

Each token has its own rate limit on the GitLab side:

| Token Type            | Rate Limit (Default) |
| --------------------- | -------------------- |
| Personal Access Token | 300 requests/minute  |
| Project Access Token  | 300 requests/minute  |
| Group Access Token    | 300 requests/minute  |

The server pool does NOT aggregate tokens — each client is independently rate-limited by GitLab. A server with 100 unique tokens can collectively make up to 30,000 requests/minute to GitLab.

## Capacity Planning Recommendations

Size from the load rather than from the token count: one built catalog per configuration served, plus what the concurrent requests allocate while they run. The developer count in each heading is the number of people who might be **calling at the same moment**, which is what decides the memory; the number of tokens on the books decides almost nothing, since a pooled credential is about 50 KiB. `--session-timeout` only matters with `--stateless=false`.

### Small Team (5-20 developers)

```bash
gitlab-mcp-server --http \
  --gitlab-url=https://gitlab.example.com \
  --max-http-clients=20 \
  --http-addr=:8080
```

- Memory: 512 MiB is enough on `dynamic` or `meta`, where twenty credentials all calling at once peaked at 316 and 271 MiB; allow 1 GiB on `individual`, which peaked at 843 MiB
- CPU: Negligible between requests; the first `tools/list` of the first credential of a configuration builds the catalog, at 0.4 s on `dynamic` and `meta` and 1.3 s on `individual`

### Medium Team (20-100 developers)

```bash
gitlab-mcp-server --http \
  --gitlab-url=https://gitlab.example.com \
  --max-http-clients=100 \
  --http-addr=:8080
```

- Memory: 1 GiB on `dynamic` or `meta` and 2.5 GiB on `individual` covers a hundred credentials calling at once, which measured peaks of 659, 481 and 2083 MiB. The pool bound itself costs about 5 MiB, so `--max-http-clients` is not what this figure is for
- CPU: Minimal between requests; a call costs about 8 ms of processor time on `dynamic` and `meta`, so a sixteen-thread host has headroom well past a hundred credentials

### Large Deployment (100+ developers)

Rather than one process with a thousand-entry pool, run several instances behind a balancer that routes on the credential (a hash of the bearer token), because the pool and the OAuth identity cache are per instance and a client that lands on a different instance rebuilds its entry there.

```bash
gitlab-mcp-server --http \
  --gitlab-url=https://gitlab.example.com \
  --max-http-clients=100 \
  --http-addr=:8080
```

- Memory: size each instance for the callers it will hold at once, not for its pool. One process reached a thousand credentials on every surface in the series, at 1.0 GiB on `meta`, 3.9 on `dynamic` and 4.4 on `individual` with all thousand calling; what makes several instances the better shape at this size is latency rather than memory, since a saturated host queues
- CPU: Light between requests, and the ceiling is processor time per call: on a sixteen-thread host the series stopped gaining throughput at about five concurrently calling credentials, and everything past that point is queueing

---

## Further Reading

- [HTTP Server Mode](../guides/http-server-mode.md) — architecture and configuration
- [Configuration](configuration.md) — all configuration options
- [Architecture](../concepts/architecture.md) — system architecture overview
