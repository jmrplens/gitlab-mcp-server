# Configuration

gitlab-mcp-server is configured through environment variables. It also reads `~/.gitlab-mcp-server.env`, and the file `GITLAB_MCP_ENV_FILE` names, for values its environment does not already carry; the home file is the usual place to keep the token out of client config files. A `.env` in the current directory is not read, for the reason given under [Configuration Loading](#configuration-loading).

> **Diátaxis type**: Reference
> **Audience**: 👤🔧 All users
> **Prerequisites**: A running GitLab instance with a Personal Access Token
> 📖 **User documentation**: See the [Configuration](https://jmrp.io/docs/gitlab-mcp-server/configuration/) on the documentation site for a user-friendly version.
>
> **Using in CI/CD?** See the [CI/CD Usage](../guides/ci-cd.md) guide for pipeline-specific setup with Project Access Tokens.

---

## Variable naming

Settings this project defines are read as `GITLAB_MCP_<NAME>` from 2.8.0.

A stdio MCP server runs in whatever shell its client was started from, next to
every other tool that person uses. Names as generic as `LOG_LEVEL`, `AUTH_MODE`
or `RATE_LIMIT_RPS` may already be owned by something else there, and the
collision is silent: the server reads a value nobody gave it and behaves in a
way nobody configured.

The unprefixed spelling still works and is removed in v3. When both are set the
prefixed one wins, and a warning at startup names the one being ignored.

Some names stay bare on purpose:

| Names                                                                                                                        | Why they were not renamed                                                                                                                               |
| ---------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GITLAB_URL`, `GITLAB_TOKEN`                                                                                                 | GitLab's own convention. Every existing configuration sets them, and they are the two most likely to be written into a client configuration from memory |
| `GITLAB_SKIP_TLS_VERIFY`, `GITLAB_TIER`, `GITLAB_ENTERPRISE`, `GITLAB_READ_ONLY`, `GITLAB_SAFE_MODE`, `GITLAB_IGNORE_SCOPES` | Already namespaced by `GITLAB_`. A second prefix would churn every existing configuration and protect against nothing                                   |
| `OTEL_*`                                                                                                                     | Owned by the OpenTelemetry specification. The exporters read those names themselves and would never see a prefixed spelling                             |
| `YOLO_MODE`, `AUTOPILOT`                                                                                                     | Conventions other agent tooling sets. Honoring the name another tool already uses is the entire point of reading them                                   |
| `EVAL_SURFACE_*`                                                                                                             | The surface evaluator's own variables, set by `make` targets in this repository. They never appear beside another tool's variables in a user's shell    |

The tables below always give the name to set, so read the name rather than
deriving it.

---

## Personal Setup

These are the settings every user needs to get started.

### Required Variables

| Variable       | Description                            | Example                      |
| -------------- | -------------------------------------- | ---------------------------- |
| `GITLAB_TOKEN` | Personal Access Token with `api` scope | `glpat-xxxxxxxxxxxxxxxxxxxx` |

### Common Options

| Variable                          | Default              | Description                                                                                                                                                                                                                                                                                                                                                                                                          |
| --------------------------------- | -------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GITLAB_URL`                      | `https://gitlab.com` | GitLab instance base URL. Set this for self-managed instances                                                                                                                                                                                                                                                                                                                                                        |
| `GITLAB_SKIP_TLS_VERIFY`          | `false`              | Skip TLS certificate verification for self-signed certs                                                                                                                                                                                                                                                                                                                                                              |
| `GITLAB_MCP_TOOL_SURFACE`         | `dynamic`            | Canonical tool catalog selector: `dynamic`, `meta`, or `individual`                                                                                                                                                                                                                                                                                                                                                  |
| `GITLAB_MCP_META_TOOLS`           | *(legacy)*           | Deprecated compatibility selector. Accepted values map to `GITLAB_MCP_TOOL_SURFACE`: `true` -> `meta`, `false` -> `individual`, and `dynamic` -> `dynamic`. Ignored when `GITLAB_MCP_TOOL_SURFACE` is set                                                                                                                                                                                                            |
| `GITLAB_MCP_CAPABILITY_SURFACE`   | `full`               | Resource and prompt catalog selector: `full` preserves all resources, workflow guides, prompts, and the surface-aware `gitlab://tools` manifest; `minimal` keeps the `gitlab://tools` manifest only                                                                                                                                                                                                                  |
| `GITLAB_MCP_META_PARAM_SCHEMA`    | `opaque`             | Meta-tool input-schema strategy: `opaque` (compact envelope), `compact`, or `full`. Applies to meta-tool `tools/list` schemas only. See [Environment Variables](env.md)                                                                                                                                                                                                                                              |
| `GITLAB_TIER`                     | *(detected)*         | Licensing tier: `free`/`ce`, `premium`, or `ultimate`. When set, used verbatim with no license check. When unset, detected from the instance license (`GET /license` → plan), fallback `free`. In HTTP mode use `--tier`; when omitted the tier is detected per token+URL pool entry. Enterprise/Premium tools are gated when the resolved tier is Premium or Ultimate                                               |
| `GITLAB_ENTERPRISE`               | *(unset)*            | **Deprecated** — use `GITLAB_TIER`. Honored only when `GITLAB_TIER` is unset: `true` → `ultimate`, `false` → `free`. Emits a deprecation warning                                                                                                                                                                                                                                                                     |
| `GITLAB_READ_ONLY`                | `false`              | Read-only mode: removes every mutating operation at startup while read operations keep working. Filtering is per action, so a meta-tool or `gitlab_execute_action` that serves both reads and writes keeps serving its reads                                                                                                                                                                                         |
| `GITLAB_SAFE_MODE`                | `false`              | Safe mode: intercepts mutating operations and returns a JSON preview instead of executing. Interception is per action, so read operations keep working on every surface and the preview names the canonical action (e.g. `issue.create`). If `GITLAB_READ_ONLY=true`, it takes precedence                                                                                                                            |
| `GITLAB_MCP_EMBEDDED_RESOURCES`   | `true`               | Embed the canonical `gitlab://` MCP resource URI as an `EmbeddedResource` content block in the get results that carry one (twenty-two actions, listed in [Output Format](output-format.md#embedded-resources)). Set `false` to disable for clients that don't tolerate duplicate content blocks                                                                                                                      |
| `GITLAB_MCP_EXCLUDE_TOOLS`        | *(empty)*            | Comma-separated tool names, group names or action IDs, excluded from the tool surface and from the resources and subscriptions that return the same objects, so the removal holds on every request path                                                                                                                                                                                                              |
| `GITLAB_IGNORE_SCOPES`            | `false`              | Skip PAT scope detection and register all tools regardless of token permissions                                                                                                                                                                                                                                                                                                                                      |
| `GITLAB_MCP_UPLOAD_MAX_FILE_SIZE` | `2GB`                | Maximum file size for upload and file tools. Supports human-friendly suffixes (`KB`, `MB`, `GB`, case-insensitive). Upper bound: 1 TB                                                                                                                                                                                                                                                                                |
| `GITLAB_MCP_CLIENT_COMPAT`        | `auto`               | Per-client response compatibility: OpenAI Codex sessions get the float `priority` in annotations rounded to 0 or 1 (their bundled parser rejects non-integer values); everything else is delivered unchanged. Set `off` to disable. Read from the process environment in both stdio and HTTP modes; the `--client-compat` flag sets the same variable. See [Client Compatibility](../guides/client-compatibility.md) |
| `GITLAB_MCP_LOG_LEVEL`            | `info`               | Logging verbosity: `debug`, `info`, `warn`, `error`. The `--log-level` flag sets the same variable                                                                                                                                                                                                                                                                                                                   |

### Dotenv File Example

Write this to `~/.gitlab-mcp-server.env`, or to any path you then name in `GITLAB_MCP_ENV_FILE`:

```env
GITLAB_TOKEN=glpat-xxxxxxxxxxxxxxxxxxxx
GITLAB_SKIP_TLS_VERIFY=false
GITLAB_MCP_TOOL_SURFACE=dynamic
GITLAB_READ_ONLY=false
GITLAB_SAFE_MODE=false
GITLAB_MCP_EMBEDDED_RESOURCES=true
GITLAB_MCP_UPLOAD_MAX_FILE_SIZE=2GB
GITLAB_MCP_LOG_LEVEL=info
```

For self-managed GitLab, add `GITLAB_URL=https://gitlab.example.com`.

> **Security**: Never commit tokens or credentials, and restrict whichever file holds the token to its owner (`chmod 600` on Unix).

---

## First run without configuration

Started by hand with `GITLAB_TOKEN` or `GITLAB_URL` still unset once the dotenv
files are read (either one missing is enough), and with a terminal attached,
the server prints what it is and what it needs, then waits for Enter. Double-clicking the binary on Windows reaches the same screen, and
the wait is why the window stays open long enough to read it.

An MCP client never sees it. A client connects pipes rather than a terminal,
which is the test the server uses to tell the two apart.

There used to be an interactive setup wizard here, with a browser UI, a
full-screen terminal UI and a prompt fallback, which wrote
`~/.gitlab-mcp-server.env` and generated config for ten MCP clients. It was
removed. MCP configuration belongs in the client's own JSON, which is what the
[client configuration](#mcp-client-configuration) section below gives you, and
a wizard writing a dotfile on one machine could not put it there.

---

## MCP Client Configuration

For per-client setup instructions (VS Code, Claude Desktop, Cursor, Claude Code, Windsurf, JetBrains, Zed, Kiro), see [Getting Started](../getting-started.md).

For HTTP mode (remote/multi-user), see [HTTP Server Mode](../guides/http-server-mode.md).

---

## Secure Token Configuration

Instead of hardcoding `GITLAB_TOKEN` directly in the MCP client JSON configuration, you can use the secure mechanisms provided by each client.

### VS Code — Input Variables

VS Code [input variables](https://code.visualstudio.com/docs/copilot/reference/mcp-configuration#_input-variables-for-sensitive-data) prompt you for the token on first server start and store the value securely. Use `${input:variable-id}` in any `env` value:

```jsonc
{
  "inputs": [
    {
      "type": "promptString",
      "id": "gitlab-token",
      "description": "GitLab Personal Access Token",
      "password": true
    }
  ],
  "servers": {
    "gitlab": {
      "type": "stdio",
      "command": "/usr/local/bin/gitlab-mcp-server",
      "env": {
        "GITLAB_TOKEN": "${input:gitlab-token}",
        "GITLAB_MCP_TOOL_SURFACE": "meta"
      }
    }
  }
}
```

### VS Code — Environment File (`envFile`)

VS Code supports loading all environment variables from a file on disk via the `envFile` property. This keeps secrets out of the JSON entirely:

```jsonc
{
  "servers": {
    "gitlab": {
      "type": "stdio",
      "command": "/usr/local/bin/gitlab-mcp-server",
      "envFile": "${userHome}/.gitlab-mcp-server.env"
    }
  }
}
```

Where `~/.gitlab-mcp-server.env` (or any path you choose) contains:

```env
GITLAB_TOKEN=glpat-xxxxxxxxxxxxxxxxxxxx
GITLAB_SKIP_TLS_VERIFY=true
GITLAB_MCP_TOOL_SURFACE=meta
```

Add `GITLAB_URL=https://gitlab.example.com` for self-managed GitLab.

> **Tip**: You can combine `envFile` with `env` — values in `env` override those from `envFile`.

### Copilot CLI — System Environment Variables

Copilot CLI reads MCP server configuration from environment variables. Set the token at the OS level:

**Linux / macOS** — add to `~/.bashrc`, `~/.zshrc`, or equivalent:

```bash
export GITLAB_TOKEN="glpat-xxxxxxxxxxxxxxxxxxxx"
```

**Windows** — set via PowerShell (persistent, user-level):

```powershell
[Environment]::SetEnvironmentVariable('GITLAB_TOKEN', 'glpat-xxxxxxxxxxxxxxxxxxxx', 'User')
```

Then restart your terminal. The MCP server inherits system environment variables.

### OpenCode

OpenCode uses its own MCP configuration file. Add the server with environment variables inline, or set the token as a system environment variable (see above) to keep it out of the config file.

### Cursor

Cursor uses the `mcpServers` configuration format. Set the token as a system environment variable (see above) and omit it from `.cursor/mcp.json`, or keep it hardcoded for local-only use.

See [Security](../concepts/security.md) for additional token management best practices.

---

## Server Administration

These settings are for operators deploying the server for a team or managing advanced behaviors. Most users can skip this section entirely.

### Advanced Variables

This table summarizes the most common operational variables. For the complete source-of-truth list, see [Environment Variable Reference](env.md); for HTTP flags, see [CLI Reference](cli.md).

| Variable                      | Default   | Description                                                                                                                                                                                                                                                                                             |
| ----------------------------- | --------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GITLAB_MCP_ENV_FILE`         | *(empty)* | Absolute path of one dotenv file to load besides `~/.gitlab-mcp-server.env`; the `--env-file` flag sets the same thing and wins over it. Read from the process environment only, so a loaded file cannot nominate another                                                                               |
| `YOLO_MODE`                   | `false`   | Skip destructive action confirmation prompts                                                                                                                                                                                                                                                            |
| `AUTOPILOT`                   | `false`   | Same as `YOLO_MODE` — skip confirmation prompts                                                                                                                                                                                                                                                         |
| `GITLAB_MCP_AUTH_MODE`        | `legacy`  | HTTP mode authentication: `legacy` (per-request header) or `oauth` (RFC 9728 Bearer token verification)                                                                                                                                                                                                 |
| `GITLAB_MCP_OAUTH_CACHE_TTL`  | `15m`     | TTL for verified OAuth token identity cache (min 1m, max 2h)                                                                                                                                                                                                                                            |
| `GITLAB_MCP_OAUTH_CLIENT_UID` | *(empty)* | Comma-separated GitLab OAuth application uids whose tokens are admitted; empty admits any credential the instance accepts                                                                                                                                                                               |
| `GITLAB_MCP_ACTION_TIMEOUT`   | `65m`     | Cancel an action still running after this long; `0` disables it (upper bound 24h). Above the longest wait any action offers. Both transports; HTTP mode also has `--action-timeout`                                                                                                                     |
| `GITLAB_MCP_DRAIN_DELAY`      | `0`       | HTTP mode: after `SIGTERM`, keep the listener open and answer `/health` with `503 draining` for this long before closing it, so a balancer that polls `/health` removes the instance before the close (upper bound 5m). Also `--drain-delay`                                                            |
| `GITLAB_MCP_RATE_LIMIT_RPS`   | `0`       | Per-server rate limit, in requests/second, on every call that reaches GitLab: `tools/call`, `resources/read`, `resources/subscribe`, `subscriptions/listen`, `prompts/get` (`0` disables it). Both transports: the default is `0` in stdio and `10` in HTTP mode, where `--rate-limit-rps` overrides it |
| `GITLAB_MCP_RATE_LIMIT_BURST` | `40`      | Token-bucket burst size when `GITLAB_MCP_RATE_LIMIT_RPS` > 0                                                                                                                                                                                                                                            |

### Tool Modes

| Mode                          | Variable                             | Tools Exposed                                                                 | Best For                                                                      |
| ----------------------------- | ------------------------------------ | ----------------------------------------------------------------------------- | ----------------------------------------------------------------------------- |
| **Dynamic toolset** (default) | `GITLAB_MCP_TOOL_SURFACE=dynamic`    | `gitlab_find_action`, `gitlab_execute_action`                                 | Most users — lowest startup context while retaining full catalog reachability |
| **Meta-tools**                | `GITLAB_MCP_TOOL_SURFACE=meta`       | 32 base / 49 self-managed enterprise / 50 GitLab.com Enterprise               | Clients that prefer consolidated domain dispatchers with `action` parameters  |
| **Individual tools**          | `GITLAB_MCP_TOOL_SURFACE=individual` | 854 CE / 1073 self-managed enterprise / 1079 GitLab.com Enterprise with Orbit | Clients that need granular tool selection                                     |

Use the default dynamic surface for normal low-token deployments. Set `GITLAB_MCP_TOOL_SURFACE=meta` only when a client or workflow prefers domain meta-tools. `GITLAB_MCP_META_TOOLS` remains accepted for compatibility only and should appear only in migration guidance.

See [Meta-Tools](../concepts/meta-tools.md) for the complete domain-action mapping and [Dynamic Toolset](../concepts/dynamic-tools.md) for the low-token find/execute workflow.

### Meta Parameter Schema

`GITLAB_MCP_META_PARAM_SCHEMA` controls only the visible `inputSchema` of meta-tool dispatchers in `tools/list`. It does not change handler validation, execution, dynamic find output, or the `gitlab://tools` manifest contents.

| Tool surface | Visible tool schema impact                                                                                                           | Tool manifest availability                                    | Dynamic describe behavior                                        | Token impact                                                   | Recommended use                                      |
| ------------ | ------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------- | ---------------------------------------------------------------- | -------------------------------------------------------------- | ---------------------------------------------------- |
| `meta`       | Applies to every visible domain meta-tool. `opaque` shows `{action, params}`; `compact` and `full` inline per-action `oneOf` schemas | `gitlab://tools` and `gitlab://tools/{id}` in full or minimal | Not applicable                                                   | `full` is 18.0x larger than `opaque`; `compact` is 8.1x larger | Keep `opaque`; use `gitlab://tools` for exact params |
| `dynamic`    | Does not change the two dynamic tool schemas                                                                                         | `gitlab://tools` and `gitlab://tools/{id}` in full or minimal | `gitlab_find_action` returns discovery and schema details inline | No practical startup benefit for Dynamic tool schemas          | Keep `opaque`; use find or `gitlab://tools`          |
| `individual` | Ignored because individual tools expose one operation per tool with direct typed schemas                                             | `gitlab://tools` and `gitlab://tools/{id}` in full or minimal | Not applicable                                                   | None                                                           | Leave unset                                          |

The evaluated modes remain `opaque`, `compact`, and `full`. The setting name remains valid for the final architecture because it describes the meta-tool dispatcher envelope, while the action catalog remains the source of the underlying per-action schemas.

### Capability Surface

`GITLAB_MCP_CAPABILITY_SURFACE=full` is the default and preserves the existing MCP resources and prompts catalog. `GITLAB_MCP_CAPABILITY_SURFACE=minimal` is a non-default low-token mode: it keeps `gitlab://tools` for surface-aware action discovery, and omits static GitLab data resources, workflow guide resources, and prompt templates. Dynamic execution still works without reading resources because `gitlab_find_action` returns exact action schemas inline. Because the minimal handshake declares no `prompts` capability, `prompts/list` and `prompts/get` answer JSON-RPC `-32601` (method not found) rather than a hollow empty page — the wire surface stays in step with the declared capabilities. `logging/setLevel` answers `-32601` on every surface for the same reason: the server logs to stderr and never declares the `logging` capability.

Measured startup context is the reason this setting keeps only two modes for now: full resources plus prompts cost about 18.2k tokens, while minimal keeps the shared capability overhead low by advertising only the unified tool manifest. Candidate intermediate modes such as `schemas`, `resources`, or `docs` would add another configuration axis without beating the existing low-token workflows. Reconsider an intermediate mode only if future audits show a concrete client that needs more resources but cannot tolerate prompts or static resources.

### HTTP Server Mode

When running the server for multiple users, use HTTP mode. Configuration comes from CLI flags instead of environment variables:

| Flag                       | Default        | Description                                                                                                                                                                                                                                                                                                                  |
| -------------------------- | -------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `--http`                   | *(off)*        | Enable HTTP transport mode                                                                                                                                                                                                                                                                                                   |
| `--http-addr`              | `:8080`        | Listen address. `host:port` binds TCP; a value containing a path separator (e.g. `/run/gitlab-mcp.sock`) binds a unix socket instead, which removes the network hop to a same-machine proxy rather than encrypting it                                                                                                        |
| `--gitlab-url`             | *(required)*   | GitLab instance URL. Required in HTTP mode unless `--allow-any-gitlab-url` is passed. Repeatable, and a single comma-separated value spells the same list: with several published, the `GITLAB-URL` header is required and selects among them, and a value naming anything else is refused rather than ignored               |
| `--allow-any-gitlab-url`   | `false`        | Start with no instance published and let `GITLAB-URL` name any host. For a single-user local deployment where the operator is the caller; it warns at startup, has no environment variable on purpose, and must not be used on a listener anyone else can reach                                                              |
| `--http-socket-mode`       | `0660`         | Permission mode, in octal, for a unix socket named by `--http-addr`. The default lets owner and group connect and nobody else, so a reverse proxy reaches the server by sharing a group with it                                                                                                                              |
| `--tls-cert`               | *(empty)*      | PEM certificate file; serves HTTPS on the listener itself. Requires `--tls-key`, and the pair is loaded at startup so a wrong path fails there rather than at the first handshake. TLS 1.2 is the floor, 1.3 when the client offers it                                                                                       |
| `--tls-key`                | *(empty)*      | PEM private key file matching `--tls-cert`                                                                                                                                                                                                                                                                                   |
| `--public-url`             | *(empty)*      | Externally reachable origin of this deployment (`https://host[:port][/path]`, no trailing slash; `http` only for loopback development). **Required with `--auth-mode=oauth`** — it is the RFC 9728 protected-resource identifier. In legacy mode it is optional, and its origin is trusted for cross-origin browser requests |
| `--resource-documentation` | *(empty)*      | https URL published as RFC 9728 `resource_documentation`; point it at a page describing your own OAuth application (its client ID and registered redirect URIs). Empty publishes this project's HTTP server mode page                                                                                                        |
| `--trusted-origins`        | *(empty)*      | Comma-separated absolute origins allowed to make cross-origin browser requests. `*` accepts any origin and disables the protection; empty rejects every cross-origin browser POST. The `--public-url` origin is trusted regardless                                                                                           |
| `--skip-tls-verify`        | `false`        | Skip TLS certificate verification                                                                                                                                                                                                                                                                                            |
| `--tool-surface`           | `dynamic`      | Canonical tool catalog selector: `dynamic`, `meta`, or `individual`                                                                                                                                                                                                                                                          |
| `--meta-tools`             | *(unset)*      | Deprecated compatibility flag. Use `--tool-surface=individual` instead of `--meta-tools=false`                                                                                                                                                                                                                               |
| `--capability-surface`     | `full`         | Resource and prompt catalog selector: `full` or `minimal`                                                                                                                                                                                                                                                                    |
| `--meta-param-schema`      | `opaque`       | Meta-tool input-schema strategy: `opaque`, `compact`, or `full`                                                                                                                                                                                                                                                              |
| `--tier`                   | *(detected)*   | Force the licensing tier: `free`, `ce`, `premium`, or `ultimate`. When set, used verbatim with no license check. When omitted, HTTP mode detects the tier per token+URL pool entry from the instance license (fallback `free`)                                                                                               |
| `--max-http-clients`       | `100`          | Maximum unique (token, GitLab URL) server entries kept in the pool; bounds pooled entries, not sessions or concurrent requests                                                                                                                                                                                               |
| `--session-timeout`        | `30m`          | Idle MCP session timeout; applies to `--stateless=false` only — under the default stateless transport each POST's session ends with its response                                                                                                                                                                             |
| `--http-idle-timeout`      | `0` (disabled) | HTTP server idle connection timeout. Default `0` disables idle connection closure, so `--session-timeout` is the effective session lifetime; set a positive duration to recycle idle connections sooner                                                                                                                      |
| `--auth-mode`              | `legacy`       | Authentication mode: `legacy` (per-request header) or `oauth` (RFC 9728 Bearer token verification)                                                                                                                                                                                                                           |
| `--oauth-client-uid`       | *(empty)*      | Comma-separated GitLab OAuth application uids whose tokens are admitted. Empty admits any credential the instance accepts; setting it also refuses personal access tokens, which belong to no application. See [ADR-0019](../development/adr/adr-0019-audience-binding-unavailable-at-the-authorization-server.md)           |
| `--oauth-cache-ttl`        | `15m`          | TTL for verified OAuth token cache (1m–2h)                                                                                                                                                                                                                                                                                   |
| `--action-timeout`         | `65m`          | Cancel an action still running after this long; `0` disables it (upper bound 24h). Falls back to `GITLAB_MCP_ACTION_TIMEOUT`                                                                                                                                                                                                 |
| `--drain-delay`            | `0`            | After `SIGTERM`, keep the listener open and answer `/health` with `503 draining` for this long before closing it, so a balancer that polls `/health` removes the instance before the close (upper bound 5m); `0` closes at once. Falls back to `GITLAB_MCP_DRAIN_DELAY`                                                      |
| `--pool-idle-timeout`      | `1h`           | Reclaim a pooled per-token-and-URL server entry after this long unused (`0` keeps entries until the pool size bound evicts them; upper bound 24h)                                                                                                                                                                            |
| `--revalidate-interval`    | `15m`          | Token re-validation interval for pooled entries (upper bound 24h). `0` stops the periodic check, but an entry whose credential is older than 1h is still rebuilt, which re-runs the probe                                                                                                                                    |
| `--trusted-proxies`        | *(empty)*      | Addresses or CIDR ranges of the reverse proxies whose `--trusted-proxy-header` is believed (e.g. `127.0.0.1,10.0.0.0/8`). From any other peer the header is ignored and the peer itself is charged. Required with `--trusted-proxy-header`, and refused without it                                                           |
| `--trusted-proxy-header`   | *(empty)*      | Header containing the real client IP when behind a reverse proxy (e.g. `CF-Connecting-IP`, `X-Real-IP`, `X-Forwarded-For`). Used by the per-IP auth rate limiter; believed only from `--trusted-proxies`                                                                                                                     |
| `--stateless`              | `true`         | Sessionless streamable HTTP (SEP-2567 / protocol 2026-07-28): no `Mcp-Session-Id` tracking, every POST is self-contained, GET/DELETE return `405`. Use `--stateless=false` for legacy stateful sessions                                                                                                                      |
| `--json-response`          | `false`        | Return `application/json` response bodies instead of `text/event-stream` (SSE)                                                                                                                                                                                                                                               |
| `--max-request-body-bytes` | `0`            | Maximum streamable HTTP request body size in bytes; `0` uses the SDK default (4 MiB). Oversized bodies are rejected with `413`; negative values are rejected at startup                                                                                                                                                      |
| `--read-only`              | `false`        | Expose only read-only tools                                                                                                                                                                                                                                                                                                  |
| `--safe-mode`              | `false`        | Intercept mutating tools, return preview                                                                                                                                                                                                                                                                                     |
| `--embedded-resources`     | `true`         | Embed canonical `gitlab://` MCP resource URIs as `EmbeddedResource` content blocks in `gitlab_*_get` tool results                                                                                                                                                                                                            |
| `--rate-limit-rps`         | `10`           | Per-server rate limit, in req/s, on every call that reaches GitLab: `tools/call`, `resources/read`, `resources/subscribe`, `subscriptions/listen`, `prompts/get` (`0` disables it). On by default because an HTTP deployment is shared: one looping client's calls are charged to its egress address                         |
| `--rate-limit-burst`       | `40`           | Token-bucket burst size when `--rate-limit-rps` > 0                                                                                                                                                                                                                                                                          |
| `--exclude-tools`          | *(empty)*      | Comma-separated tool names, group names or action IDs, excluded from the tool surface and from the resources and subscriptions that return the same objects, so the removal holds on every request path                                                                                                                      |
| `--ignore-scopes`          | `false`        | Skip PAT scope detection                                                                                                                                                                                                                                                                                                     |

No `GITLAB_TOKEN` is needed at startup — each client provides its own token per-request via `PRIVATE-TOKEN` header or `Authorization: Bearer`. The `GITLAB-URL` header follows how many instances the server published: with none (possible only with `--allow-any-gitlab-url`) it selects the instance per request, and a request that omits it is refused with `400` rather than resolved to `https://gitlab.com`; with exactly one `--gitlab-url` that instance is authoritative and the header is ignored and logged; with several (`--gitlab-url` repeated, or one comma-separated value) the header is required (`400` without it) and selects among them, and a value naming an unpublished instance is refused — `403` in OAuth mode, `400` in legacy mode — rather than silently served the first.

### OAuth Mode Configuration

To enable server-side token verification, set `--auth-mode=oauth`:

```bash
gitlab-mcp-server --http \
  --gitlab-url=https://gitlab.com \
  --auth-mode=oauth \
  --public-url=https://mcp.example.com \
  --oauth-cache-ttl=15m
```

Replace `https://gitlab.com` with your self-managed GitLab URL when needed.

With OAuth mode:

- All tokens are verified against GitLab's `/api/v4/user` endpoint before reaching the MCP handler
- Verified tokens are cached (SHA-256 hashed, keyed on instance and token) for `--oauth-cache-ttl` duration (default 15m, range 1m–2h)
- An RFC 9728 metadata endpoint is served at `/.well-known/oauth-protected-resource`, enabling MCP clients with OAuth 2.1 support to discover the GitLab authorization server automatically
- OAuth mode is Bearer-only: a `PRIVATE-TOKEN` header is not accepted, so that what the `WWW-Authenticate` challenge advertises is exactly what is accepted. Legacy mode reads both headers

For a complete guide on creating GitLab OAuth applications for your MCP clients, see [OAuth App Setup](../guides/oauth-app-setup.md).

See [HTTP Server Mode](../guides/http-server-mode.md) for architecture and deployment details.

## Automatic Behaviors

These features are always active and require no configuration:

| Feature                 | Description                                                                                                                                                                                                                                       |
| ----------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Content annotations** | All Markdown content is annotated with `audience` and `priority` — `ContentList` (priority 0.4), `ContentDetail` (0.6), `ContentMutate` (0.8). This helps MCP clients optimize display and prevents raw Markdown from duplicating the JSON output |
| **Clickable links**     | List results across 71 tool packages include `[text](url)` links to GitLab entities (MRs, issues, pipelines, etc.), with a hint asking the model to preserve them                                                                                 |
| **Next-step hints**     | Every list/detail/mutation response includes `💡 Next steps` suggestions. In meta-tool mode, these are also injected into the JSON `structuredContent` as a `next_steps` array                                                                     |
| **Formatted dates**     | All timestamps are displayed in readable format (`2025-01-15 10:30`) instead of raw ISO 8601                                                                                                                                                      |

See [Output Format](output-format.md) for details.

## Configuration Loading

Configuration is loaded by `internal/config/` in this order, highest priority first:

1. Command-line flags (`--http`, `--http-addr`)
2. Environment variables, which is what the MCP client passed to the server
3. The dotenv file `GITLAB_MCP_ENV_FILE` names, if the process environment names one
4. `~/.gitlab-mcp-server.env` in the user's home directory

> **Note**: `godotenv` never overwrites a variable that is already set, so an earlier step always wins over a later one.

A `.env` in the current working directory is not loaded. A stdio server inherits that directory from its client, so the file arrives with whatever repository or archive was opened, and loading it first let two lines chosen by someone else redirect the token, disable certificate verification and rewrite the tool descriptions the model reads, before any tool call. One is still looked for and reported at `WARN` with its absolute path and the keys it wanted to set, so a repository-local file that stopped taking effect shows up in the startup log instead of being debugged.

`GITLAB_MCP_ENV_FILE` is what the working-directory load becomes when someone actually wants it: name the file, by absolute path, in the client configuration or the launching shell. It is read from the process environment only, so a file this server loads cannot nominate another. A relative value is resolved against the working directory the client chose, which reintroduces exactly what this replaced, so startup warns about one.

The `config.Load()` function validates that `GITLAB_TOKEN` is set and defaults `GITLAB_URL` to `https://gitlab.com` when it is omitted (stdio mode only). In HTTP mode, configuration comes from CLI flags and no token is required at startup — each client provides its own token per-request via `PRIVATE-TOKEN` or `Authorization: Bearer` headers. Clients can provide `GITLAB-URL` whenever the server published no instance (which takes `--allow-any-gitlab-url`) or more than one — with a single `--gitlab-url` the header is ignored and logged, and with several it may only name one of the published instances; all other MCP server settings are process policy and cannot be overridden per request. When `--auth-mode=oauth`, the server validates tokens against the GitLab `/api/v4/user` endpoint and caches verified identities with a configurable TTL (see [HTTP Server Mode — OAuth Mode](../guides/http-server-mode.md#oauth-mode)).
