# CLI Reference

> **Diátaxis type**: Reference
> **Audience**: 👤🔧 All users
> **Prerequisites**: gitlab-mcp-server binary installed
>
> Complete command-line interface reference for gitlab-mcp-server.

---

## Synopsis

```text
gitlab-mcp-server [flags]
```

When run without flags and a `GITLAB_TOKEN` is set, the server starts in **stdio mode**. With an interactive terminal and either `GITLAB_URL` or `GITLAB_TOKEN` still missing after the dotenv files are read, it prints what it needs and waits, rather than starting a session it cannot serve.

---

## Flags

### General

| Flag                         | Type   | Default   | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| ---------------------------- | ------ | --------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `-h`, `-help`                | bool   | `false`   | Show full help with flags, environment variables, and JSON examples                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `-env-file`                  | string | _(empty)_ | Dotenv file to load besides `~/.gitlab-mcp-server.env`; the same setting as `GITLAB_MCP_ENV_FILE`, and wins over it                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `-version`                   | bool   | `false`   | Print version and commit hash, then exit                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `-shutdown`                  | bool   | `false`   | Terminate all running instances and exit (used by external updaters)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `-probe`                     | bool   | `false`   | Ask the running instance's `/health` and exit 0 when it answers; the image's `HEALTHCHECK`. An optional target after the flag (URL, `unix:<path>`, `host:port`) is probed instead of the discovered listener                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `-tool-search`               | string | _(empty)_ | Search the **action catalog** by canonical ID, tool name, alias, tag or description, then exit. Every term must match, case-insensitively. Each row is the canonical action ID, which is what `gitlab_execute_action` takes, beside the tool the configured surface names it: the individual tool name, or the meta group tool with its `action` argument. The surface and the tier come from `--tool-surface` and `--tier` when passed, and otherwise from `GITLAB_MCP_TOOL_SURFACE` and `GITLAB_MCP_TIER` (with the legacy `GITLAB_MCP_META_TOOLS` and `GITLAB_ENTERPRISE` spellings), so a stdio deployment searches what it serves. No GitLab credentials are needed |
| `-transport`                 | string | _(empty)_ | Transport to serve: `stdio`, `http` or `auto`. Empty defers to `--http`; given both, `--transport` wins. `auto` reads file descriptor 0 and serves HTTP only when stdin is the null device, which is what a container started without `-i` gives, and stdio for the pipe an MCP client connects. Any other value exits 2                                                                                                                                                                                                                                                                                                                                                 |
| `-log-level`                 | string | _(empty)_ | Logging verbosity: `debug`, `info` (the effective default), `warn` or `error`. Sets `GITLAB_MCP_LOG_LEVEL`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| `-client-compat`             | string | _(empty)_ | Per-client response compatibility: `auto` (the effective default) or `off`. Sets `GITLAB_MCP_CLIENT_COMPAT`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| `-upload-max-file-size`      | string | _(empty)_ | Maximum size for upload and file-read tools, `KB`/`MB`/`GB` suffixes accepted (`2GB` when unset, 1 TB ceiling). Sets `GITLAB_MCP_UPLOAD_MAX_FILE_SIZE`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `-yolo-mode`                 | string | _(empty)_ | `true` skips the confirmation prompt on destructive actions. Sets `GITLAB_MCP_YOLO_MODE`, which takes precedence over `AUTOPILOT`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| `-description-substitutions` | string | _(empty)_ | Comma-separated `old=new` pairs applied in order to every listed description and title, for strict MCP gateway validators (backslash escapes `\,` `\=` `\\`). Sets `GITLAB_MCP_DESCRIPTION_SUBSTITUTIONS`; a malformed value refuses startup                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `-pprof-addr`                | string | _(empty)_ | Serve Go's profiling handlers (`net/http/pprof`) on this address, on a listener of their own started before the transport, so a CPU profile of startup can be taken. The host must be loopback (`127.0.0.1:6060`, `[::1]:6060`, `localhost:6060`): anything else is refused at startup, because a heap profile is a copy of the process's memory. Sets `GITLAB_MCP_PPROF_ADDR`                                                                                                                                                                                                                                                                                           |

The last six flags are environment-backed: each one writes its variable before anything reads configuration, so there is exactly one reader per setting and an explicitly passed flag beats an exported variable. `GITLAB_TOKEN` deliberately has no flag: a token on a command line is visible to every user on the machine through `ps` and lands in shell history, so the environment is the only way to supply it (or a per-request header in HTTP mode).

### Telemetry (both transports)

| Flag                           | Type   | Default   | Description                                                                                                                                                                                                                                                                                                                                                                                               |
| ------------------------------ | ------ | --------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `-telemetry`                   | bool   | `false`   | Export OpenTelemetry traces, metrics and logs over OTLP. Off for privacy: the endpoint, credentials, sampling and batching come from the standard `OTEL_EXPORTER_OTLP_*` variables the exporters read themselves, and `OTEL_SDK_DISABLED=true` vetoes it regardless. Falls back to `GITLAB_MCP_TELEMETRY`. See [Telemetry](../guides/telemetry.md)                                                        |
| `-telemetry-identity`          | string | `none`    | How much telemetry records about who made a call: `none` records nobody, `pseudonymous` a per-process HMAC digest that correlates one caller's calls without naming them, `full` the GitLab user id and username. Falls back to `GITLAB_MCP_TELEMETRY_IDENTITY`. The pseudonymisation secret, `GITLAB_MCP_TELEMETRY_IDENTITY_KEY`, has no flag on purpose: process arguments are readable through `/proc` |
| `-telemetry-identity-rotation` | string | _(empty)_ | How long a generated pseudonymisation key lives, e.g. `24h`; empty or `0` keeps it for the life of the process, 30 days is the ceiling. Ignored, with a warning, when a key is configured. Falls back to `GITLAB_MCP_TELEMETRY_IDENTITY_ROTATION`                                                                                                                                                         |
| `-telemetry-tool-name`         | string | `auto`    | Whether `gen_ai.tool.name` is a metric dimension: `auto` keeps it on the dynamic and meta surfaces and drops it on individual, where about a thousand tools would exhaust the SDK's cardinality limit; `on` and `off` force it. Falls back to `GITLAB_MCP_TELEMETRY_TOOL_NAME`                                                                                                                            |

### HTTP Transport Mode

| Flag                      | Type     | Default                    | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| ------------------------- | -------- | -------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `-http`                   | bool     | `false`                    | Enable HTTP transport mode (default is stdio)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `-http-addr`              | string   | `:8080`                    | Listen address. `host:port` (e.g. `localhost:8080`, `:9090`) binds TCP; a value containing a path separator (e.g. `/run/gitlab-mcp.sock`) binds a unix socket instead, which removes the network hop to a same-machine proxy rather than encrypting it                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| `-http-socket-mode`       | string   | `0660`                     | Permission mode for a unix socket named by `--http-addr`, in octal. The default lets owner and group connect and nobody else, so a reverse proxy reaches the server by sharing a group with it                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `-tls-cert`               | string   | _(empty)_                  | PEM certificate file. Serves HTTPS on the listener itself, for a deployment whose proxy does not share the machine. Requires `--tls-key`; the pair is loaded at startup so a typo fails there instead of at the first handshake. TLS 1.2 is the floor and 1.3 is negotiated with any client that supports it                                                                                                                                                                                                                                                                                                                                                                                              |
| `-tls-key`                | string   | _(empty)_                  | PEM private key file matching `--tls-cert`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| `-gitlab-url`             | string   | _(required)_               | GitLab instance URL. **Required in HTTP mode** unless `--allow-any-gitlab-url` is passed: a deployment that has not said which GitLab it serves makes its requests to whatever host the caller named in `GITLAB-URL`, with whatever token that caller supplied. **Repeatable** (or comma-separated): publishing several instances lists them all in the RFC 9728 `authorization_servers` field and makes `GITLAB-URL` a choice **among them**, required rather than optional, since picking for the caller would send their token to an instance they never named; a value naming anything else is refused, not ignored                                                                                   |
| `-allow-any-gitlab-url`   | bool     | `false`                    | Start with no instance published and let `GITLAB-URL` name any host, including private and link-local addresses. The response comes back to the caller, so this makes the server a proxy for whoever can reach the listener: it is for the single-user local deployment where the operator **is** the caller. It warns at startup, and it has no environment variable on purpose, so that a deployment running with it says so in its own command line                                                                                                                                                                                                                                                    |
| `-skip-tls-verify`        | bool     | `false`                    | Skip TLS certificate verification for self-signed certs                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `-tool-surface`           | string   | `dynamic`                  | Canonical tool catalog selector: `meta`, `individual`, or `dynamic`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `-meta-tools`             | bool     | _(unset; effective false)_ | Deprecated compatibility flag ignored when `--tool-surface` is set. Leave unset for the default dynamic surface; use `--tool-surface=individual` when migrating old `--meta-tools=false` configs.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `-capability-surface`     | string   | `full`                     | Resource and prompt catalog selector: `full` or `minimal`. Minimal keeps the `gitlab://tools` manifest, and disables optional GitLab data resources, workflow guides, and prompts                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `-meta-param-schema`      | string   | `opaque`                   | Meta-tool input-schema strategy: `opaque` (default), `compact`, or `full`. Applies to meta-tool schemas only. See [Environment Variables](env.md)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `-tier`                   | string   | _(detected)_               | Force the licensing tier (`free`, `ce`, `premium`, `ultimate`) when explicitly set. When omitted, HTTP mode detects the tier per token+URL pool entry from the instance license (fallback `free`)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `-read-only`              | bool     | `false`                    | Read-only mode: removes every mutating operation, per action, while read operations keep working on all surfaces                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `-safe-mode`              | bool     | `false`                    | Safe mode: intercepts mutating operations per action and returns a JSON preview instead of executing; reads keep working. If `--read-only` is also set, it takes precedence                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| `-embedded-resources`     | bool     | `true`                     | Embed canonical `gitlab://` MCP resource URIs as `EmbeddedResource` content blocks in `gitlab_*_get` tool results. Set `false` to disable for clients that don't tolerate duplicate content blocks                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| `-exclude-tools`          | string   | _(empty)_                  | Comma-separated tool names, group names or canonical action IDs, excluded from the tool surface and from the resources and subscriptions that return the same objects, so the removal holds on every request path                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `-ignore-scopes`          | bool     | `false`                    | Skip PAT scope detection and register all tools regardless of token permissions                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| `-max-http-clients`       | int      | `100`                      | Maximum unique (token, GitLab URL) server entries kept in the pool; bounds pooled entries, not sessions or concurrent requests (upper bound: 10,000)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `-session-timeout`        | duration | `30m`                      | Idle MCP session timeout; applies to `--stateless=false` only, since under the default stateless transport each POST's session ends with its response (upper bound: 24h)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| `-action-timeout`         | duration | `65m`                      | Cancel an action still running after this long; `0` disables it (upper bound: 24h). The timeout applies in both transports, but this flag is the HTTP spelling: stdio builds its configuration from the environment, so a stdio deployment sets `GITLAB_MCP_ACTION_TIMEOUT` and passing the flag without `--http` does nothing. Above the longest wait any action offers (a pipeline wait caps itself at 3600 s), so it ends a handler nobody else bounds rather than a legitimate call                                                                                                                                                                                                                   |
| `-drain-delay`            | duration | `0`                        | After `SIGTERM`, keep the listener open and answer `/health` with `503 draining` for this long before closing it, so a balancer that polls `/health` takes the instance out of rotation before the close (upper bound: 5m). `0` closes the listener at once. Set it to at least one probe interval. Falls back to `GITLAB_MCP_DRAIN_DELAY`                                                                                                                                                                                                                                                                                                                                                                |
| `-pool-idle-timeout`      | duration | `1h`                       | Reclaim a pooled per-token-and-URL credential entry after this long unused; `0` keeps entries until the pool size bound evicts them (upper bound: 24h). An entry with a live subscription is never idle by this measure                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `-revalidate-interval`    | duration | `15m`                      | Token re-validation interval for pooled entries (upper bound: 24h). `0` stops the periodic check, but an entry whose credential is older than 1h is still rebuilt, which re-runs the probe and ends any stateful session on it                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `-http-idle-timeout`      | duration | `0` (disabled)             | HTTP server idle connection timeout. Default `0` disables idle connection closure entirely, so `--session-timeout` is the effective session lifetime. Set a positive duration to recycle idle connections sooner                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `-auth-mode`              | string   | `legacy`                   | Authentication mode: `legacy` (PRIVATE-TOKEN header passthrough) or `oauth` (RFC 9728 Bearer token verification via GitLab API; requires an https `--gitlab-url` and a valid `--public-url`). See [HTTP Server Mode — OAuth Mode](../guides/http-server-mode.md#oauth-mode)                                                                                                                                                                                                                                                                                                                                                                                                                               |
| `-oauth-client-uid`       | string   | _(empty)_                  | Comma-separated GitLab OAuth application uids whose tokens this deployment admits. Empty admits any credential the instance accepts; setting it also refuses personal access tokens, which belong to no application. See [ADR-0019](../development/adr/adr-0019-audience-binding-unavailable-at-the-authorization-server.md)                                                                                                                                                                                                                                                                                                                                                                              |
| `-oauth-cache-ttl`        | duration | `15m`                      | TTL for verified OAuth token identity cache. Range: 1m–2h. Only applies when `--auth-mode=oauth`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `-public-url`             | string   | _(empty)_                  | Externally reachable origin of this deployment (`scheme://host[:port][/path]`, no trailing slash). **Required with `--auth-mode=oauth`**: it is the RFC 9728 protected-resource identifier and the metadata URL is derived from it. In legacy mode it is optional, and when set its origin is added to `--trusted-origins`                                                                                                                                                                                                                                                                                                                                                                                |
| `-resource-documentation` | string   | _(empty)_                  | https URL published as RFC 9728 `resource_documentation`. Point it at a page describing **your** OAuth application — its client ID and registered redirect URIs — so a client arriving from a `401` challenge finds what it needs. Empty publishes this project's HTTP server mode page. RFC 9728 defines no field carrying a client identifier, so this is the only sanctioned way to lead a client to one                                                                                                                                                                                                                                                                                               |
| `-resource-policy-uri`    | string   | _(empty)_                  | https URL published as RFC 9728 `resource_policy_uri`. Point it at **your** page describing what this deployment does with the data reached through the tokens it accepts. Empty omits the field, which is the right default for a deployment with no such page: an absent optional field is better than a dead link on a consent screen                                                                                                                                                                                                                                                                                                                                                                  |
| `-resource-tos-uri`       | string   | _(empty)_                  | https URL published as RFC 9728 `resource_tos_uri`. Point it at **your** terms of service. Empty omits the field                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `-trusted-origins`        | string   | _(empty)_                  | Comma-separated absolute origins (`scheme://host[:port]`, an IP is fine for local deployments) allowed to make cross-origin browser requests. `*` accepts any origin and disables the protection. Empty rejects every cross-origin browser POST. The `--public-url` origin is trusted automatically. See [Security — Cross-Origin Protection](../concepts/security.md)                                                                                                                                                                                                                                                                                                                                    |
| `-trusted-proxies`        | string   | _(empty)_                  | Comma-separated addresses or CIDR ranges of the reverse proxies whose `-trusted-proxy-header` is believed (e.g. `127.0.0.1,10.0.0.0/8`). From any other peer the header is ignored and the peer itself is charged. For `X-Forwarded-For` the value is read from the right, skipping hops that are themselves listed, so the first unlisted hop is the client; a hop that is not an address charges the peer. Required with `-trusted-proxy-header`, and refused without it                                                                                                                                                                                                                                |
| `-trusted-proxy-header`   | string   | _(empty)_                  | HTTP header containing the real client IP when behind a reverse proxy (e.g. `CF-Connecting-IP`, `X-Forwarded-For`, `X-Real-IP`). Believed only on a connection from an address in `-trusted-proxies`, which it requires                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `-rate-limit-rps`         | float    | `10`                       | Per-credential rate limit, in requests/second, on every call that reaches GitLab (`tools/call`, `resources/read`, `resources/subscribe`, `subscriptions/listen`, `prompts/get`), plus `tools/list` on a bucket of its own refilled a tenth as fast: that one reaches no GitLab and spends the processor every tenant shares instead. Its burst is the configured one, undivided, so a fleet of clients on one credential can still all discover at once. `0` disables it. On by default in HTTP mode: the deployment is shared, so one looping client's calls are charged to its egress address. Stdio defaults to `0`. See [Security — Rate Limiting Model](../concepts/security.md#rate-limiting-model) |
| `-rate-limit-burst`       | int      | `40`                       | Token-bucket burst size when `--rate-limit-rps > 0`. Must be ≥ 1                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `-stateless`              | bool     | `true`                     | Sessionless streamable HTTP (SEP-2567 / protocol 2026-07-28): no `Mcp-Session-Id` tracking, every POST is self-contained, GET/DELETE return `405`. Use `-stateless=false` for legacy stateful sessions. See [HTTP Server Mode — Stateless Mode](../guides/http-server-mode.md#stateless-mode)                                                                                                                                                                                                                                                                                                                                                                                                             |
| `-json-response`          | bool     | `false`                    | Return `application/json` response bodies instead of `text/event-stream` (SSE)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `-max-request-body-bytes` | int64    | `0`                        | Maximum streamable HTTP request body size in bytes; `0` uses the SDK default (4 MiB). Oversized bodies are rejected with `413`; negative values are rejected at startup                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |

---

## Modes of Operation

### Stdio Mode (Default)

The server reads configuration from environment variables and communicates via stdin/stdout JSON-RPC. This is the standard mode for MCP clients like VS Code, Claude Desktop, and Cursor.

```bash
# Configuration via environment variables
export GITLAB_TOKEN="glpat-xxxxxxxxxxxxxxxxxxxx"
gitlab-mcp-server
```

Most flags on this page fall back to an environment variable when they are not
passed, and the settings this project defines are named `GITLAB_MCP_<NAME>`
from 2.8.0. The unprefixed spelling still works and is removed in v3; when
both are set the prefixed one wins and a startup warning names the one being
ignored. `GITLAB_URL`, `GITLAB_TOKEN`, the `GITLAB_`-prefixed switches and
every `OTEL_*` variable keep their bare names. The flags with no variable at
all, the transport and listener ones in particular, are marked _(none)_ in
[HTTP Mode Equivalents](env.md#http-mode-equivalents). See
[Environment Variables](env.md#variable-naming) for the full rule.

Set `GITLAB_URL` only for self-managed instances; stdio mode defaults to `https://gitlab.com`.

```bash
# Configuration via ~/.gitlab-mcp-server.env, or a file named in GITLAB_MCP_ENV_FILE
gitlab-mcp-server
```

A `.env` in the current directory is not read: the directory belongs to
whatever repository the client opened, not to the operator. One that exists is
named at `WARN` on startup so it is clear why it stopped taking effect. See
[Configuration](configuration.md#configuration-loading).

### HTTP Mode

The server listens on an HTTP endpoint. Each client provides its own GitLab token per-request via `PRIVATE-TOKEN` header or `Authorization: Bearer`, so no `GITLAB_TOKEN` is needed at startup. `--gitlab-url` is required: name the instance, or the instances, this deployment serves. Publishing several makes the `GITLAB-URL` header a required choice among them. Publishing none is possible only with `--allow-any-gitlab-url`, which lets the header name any host at all.

```bash
# Single GitLab.com instance (all clients use the fixed URL; replace for self-managed GitLab)
gitlab-mcp-server --http --gitlab-url=https://gitlab.com
gitlab-mcp-server --http --gitlab-url=https://gitlab.com --http-addr=localhost:9090
gitlab-mcp-server --http --gitlab-url=https://gitlab.com --max-http-clients=50 --session-timeout=1h
gitlab-mcp-server --http --gitlab-url=https://gitlab.com --auth-mode=oauth --public-url=https://mcp.example.com --oauth-cache-ttl=15m

# Stateless streamable HTTP (SEP-2567, the default) with plain JSON responses
gitlab-mcp-server --http --gitlab-url=https://gitlab.com --json-response

# Legacy stateful sessions (opt-out) with a 1 MiB request body cap
gitlab-mcp-server --http --gitlab-url=https://gitlab.com --stateless=false --max-request-body-bytes=1048576

# Several instances (the GITLAB-URL header is then required, and must name one of them)
gitlab-mcp-server --http --gitlab-url=https://gitlab.com,https://gitlab.example.com
```

### First run without configuration

Started by hand in a terminal, or double-clicked, with `GITLAB_TOKEN` or
`GITLAB_URL` still unset once the dotenv files are read (either one missing is
enough), the server prints what it is and what it needs, then waits for Enter
before exiting. The wait matters on Windows, where a double-clicked
console program closes its window the instant it returns, so a message printed
and returned from is a message nobody reads.

An MCP client never reaches that screen: a client connects pipes rather than a
terminal, which is the test the server uses.

This replaced an interactive setup wizard with web, terminal and prompt modes
that wrote `~/.gitlab-mcp-server.env`. MCP configuration lives in the client's
own JSON, which is where [Getting Started](../getting-started.md) puts it and
where a wizard writing a dotfile on this machine could not help.

### Shutdown Mode

The `--shutdown` flag terminates all running instances of this binary and exits. It is designed for external updaters (like pe-agnostic-store) to cleanly stop running servers before replacing the binary on disk.

```bash
# Terminate all running gitlab-mcp-server instances
gitlab-mcp-server --shutdown
```

Behavior:

1. Finds all processes matching the binary name (cross-platform, user-scoped)
2. Sends graceful termination signal (SIGTERM on Unix, TerminateProcess on Windows)
3. Waits up to 5 seconds for processes to exit
4. Force-kills any remaining processes
5. Exits with code 0 on success

Output (stderr):

- `shutdown: found N running instance(s)` — on discovery
- `shutdown: all instances terminated` — on success
- `shutdown: force-killed M instance(s)` — if force-kill was needed

### Probe Mode

The `--probe` flag is the container image's `HEALTHCHECK`. It asks the running instance's `/health` and exits 0 when it answers 200, without being told where that instance listens.

```bash
# What the image runs: find the server in this container and probe its listener
gitlab-mcp-server --probe

# Probe a known listener instead: a URL, a unix socket, or host:port
gitlab-mcp-server --probe --tls-cert=/etc/ssl/mcp.crt https://127.0.0.1:8443
gitlab-mcp-server --probe unix:/run/gitlab-mcp/server.sock
gitlab-mcp-server --probe 127.0.0.1:9090
```

Behavior without a target:

1. Finds the other instances of this binary, with the same lookup `--shutdown` uses, and skips probes, shutdowns and other utility invocations
2. Reads `--http-addr`, `--tls-cert`, `--transport` and `--http` off each instance's command line, lowest pid first
3. Settles `--transport auto` the way the server did, by reading the instance's file descriptor 0 from procfs: `/dev/null` means HTTP, anything else means stdio. Where procfs is unavailable, HTTP is assumed and the connection decides
4. An instance serving stdio has nothing to probe and is reported healthy while it runs
5. An HTTP instance is probed where it listens: an unspecified host such as `:8080` or `0.0.0.0:8080` is reached on loopback, a path is dialed as a unix socket, and `--tls-cert` means HTTPS. A TLS listener is verified by pinning: it must present the very certificate its `--tls-cert` names, which the probe reads from the same file, so a self-signed certificate on a loopback address is probeable without trusting whatever answers there. A given `https://` target takes its pin from the probe's own `--tls-cert`, placed before the target, and gets the standard verification without one
6. The first instance that answers 200 makes the probe healthy

Each attempt is bounded to three seconds, inside the image's five-second `HEALTHCHECK` timeout.

Exit codes: `0` healthy, `1` nothing answered (or no instance is running), `2` a given target that does not parse.

---

## Examples

```bash
# Print version
gitlab-mcp-server -version

# Show help with all flags and JSON configuration examples
gitlab-mcp-server -h

# Start stdio server (reads ~/.gitlab-mcp-server.env for what the environment lacks)
gitlab-mcp-server

# Start HTTP server with custom address
gitlab-mcp-server --http --gitlab-url=https://gitlab.com --http-addr=:9090

# Several instances (clients pick one with the GITLAB-URL header)
gitlab-mcp-server --http --gitlab-url=https://gitlab.com,https://gitlab.example.com --http-addr=:8080

# Single-user local deployment: no instance published, GITLAB-URL may name any host
gitlab-mcp-server --http --allow-any-gitlab-url --http-addr=127.0.0.1:8080

# Start HTTP server for self-managed GitLab with TLS skip and custom session timeout
gitlab-mcp-server --http --gitlab-url=https://gitlab.example.com --skip-tls-verify --session-timeout=2h

# Start HTTP server with individual tools
gitlab-mcp-server --http --gitlab-url=https://gitlab.com --tool-surface=individual

# Start HTTP server with the dynamic toolset (reduces token usage for LLM context)
gitlab-mcp-server --http --gitlab-url=https://gitlab.com --tool-surface=dynamic

# Start HTTP server with the dynamic toolset and reduced non-tool capabilities
gitlab-mcp-server --http --gitlab-url=https://gitlab.com --tool-surface=dynamic --capability-surface=minimal

# Find the actions that list issues, on whatever surface is configured
gitlab-mcp-server --tool-search "issue list"

# Search the Ultimate catalog from a Free deployment
gitlab-mcp-server --tier=ultimate --tool-search "epic"

# Terminate all running instances (used by external updaters)
gitlab-mcp-server --shutdown

# Container health check: probe the running instance where it listens
gitlab-mcp-server --probe
```

See [Dynamic Tools](../concepts/dynamic-tools.md) for how `dynamic` relates.

---

## Exit Codes

| Code | Meaning                                                                                                                                       |
| ---- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| `0`  | Normal exit (signal-based shutdown, `-version`, `-h`, `--shutdown`, or a `--probe` that was answered)                                         |
| `1`  | Configuration error, connection failure, runtime error, `--shutdown` failure, or a `--probe` nothing answered                                 |
| `2`  | `--probe` given a target that is not a URL, a socket path or `host:port`, or `--transport` given a value other than `stdio`, `http` or `auto` |

---

## See Also

- [Configuration](configuration.md) — Environment variables and dotenv files
- [HTTP Server Mode](../guides/http-server-mode.md) — Architecture and deployment details
- [Dynamic Toolset](../concepts/dynamic-tools.md) — Low-token find/execute mode
- [Getting Started](../getting-started.md) — Step-by-step tutorial
