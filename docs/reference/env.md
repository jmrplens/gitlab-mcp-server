# Environment Variable Reference

> **Diátaxis type**: Reference
> **Audience**: 👤🔧 All users
> **Prerequisites**: gitlab-mcp-server binary installed
>
> Complete environment variable reference for gitlab-mcp-server stdio mode.

---

## Required

| Variable       | Description                            | Example                      |
| -------------- | -------------------------------------- | ---------------------------- |
| `GITLAB_TOKEN` | Personal Access Token with `api` scope | `glpat-xxxxxxxxxxxxxxxxxxxx` |

---

## Optional — Connection

| Variable                 | Default              | Description                                                                                             |
| ------------------------ | -------------------- | ------------------------------------------------------------------------------------------------------- |
| `GITLAB_URL`             | `https://gitlab.com` | GitLab instance base URL (must use `http://` or `https://` scheme). Set this for self-managed instances |
| `GITLAB_SKIP_TLS_VERIFY` | `false`              | Skip TLS certificate verification (`true`/`false`). Use for self-signed certs                           |

---

## Optional — Server Behavior

| Variable               | Default      | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| ---------------------- | ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `TOOL_SURFACE`         | `dynamic`    | Canonical tool catalog selector: `dynamic`, `meta`, or `individual`. `dynamic` exposes find/execute                                                                                                                                                                                                                                                                                                                                                                                         |
| `META_TOOLS`           | *(legacy)*   | Deprecated compatibility selector. Accepted values map to `TOOL_SURFACE`: `true` -> `meta`, `false` -> `individual`, and `dynamic` -> `dynamic`. Ignored when `TOOL_SURFACE` is set                                                                                                                                                                                                                                                                                                         |
| `CAPABILITY_SURFACE`   | `full`       | Resource and prompt catalog selector: `full` keeps the complete catalog; `minimal` keeps the surface-aware `gitlab://tools` manifest and disables optional GitLab data resources, workflow guides, prompts, and [resource subscriptions](capabilities/subscriptions.md) — `resources/subscribe` is only advertised on `full`. Dynamic describe/find still returns action schemas inline with `minimal`                                                                                      |
| `META_PARAM_SCHEMA`    | `opaque`     | Meta-tool input-schema strategy: `opaque` (compact `{action, params:any}` envelope, default), `compact` (oneOf with property names + types only, 6.5x opaque size) or `full` (oneOf with full per-action JSON Schemas, 11.9x opaque size). Applies to visible meta-tool schemas in `meta`; has no practical effect on `dynamic` or `individual` tool schemas. Per-action call shapes and JSON Schemas are discoverable through `gitlab://tools` and `gitlab://tools/{id}` for every surface |
| `GITLAB_TIER`          | *(detected)* | Licensing tier: `free`/`ce`, `premium`, or `ultimate`. When set, used verbatim with no license check. When unset, detected from the instance license (`GET /license` → plan), fallback `free`. In HTTP mode use `--tier`; when omitted the tier is detected per token+URL pool entry. Enterprise/Premium tools are gated when the resolved tier is Premium or Ultimate                                                                                                                      |
| `GITLAB_ENTERPRISE`    | *(unset)*    | **Deprecated** — use `GITLAB_TIER`. Honored only when `GITLAB_TIER` is unset: `true` → `ultimate`, `false` → `free`. Emits a deprecation warning                                                                                                                                                                                                                                                                                                                                            |
| `LOG_LEVEL`            | `info`       | Logging verbosity: `debug`, `info`, `warn`, `error`                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `GITLAB_READ_ONLY`     | `false`      | Read-only mode: disables all mutating tools at startup. Only tools with `ReadOnlyHint=true` remain available (`true`/`false`)                                                                                                                                                                                                                                                                                                                                                               |
| `GITLAB_SAFE_MODE`     | `false`      | Safe mode: intercepts mutating tools and returns a structured JSON preview instead of executing. Read-only tools work normally. If `GITLAB_READ_ONLY=true`, it takes precedence (`true`/`false`)                                                                                                                                                                                                                                                                                            |
| `EMBEDDED_RESOURCES`   | `true`       | Embed canonical `gitlab://` MCP resource URIs as `EmbeddedResource` content blocks in `gitlab_*_get` tool results. Set to `false` to disable for clients that don't tolerate duplicate content blocks (`true`/`false`)                                                                                                                                                                                                                                                                      |
| `EXCLUDE_TOOLS`        | *(empty)*    | Comma-separated list of tool names to exclude from registration (e.g. `gitlab_admin,gitlab_runner`)                                                                                                                                                                                                                                                                                                                                                                                         |
| `GITLAB_IGNORE_SCOPES` | `false`      | Skip PAT scope detection and register all tools regardless of token permissions (`true`/`false`)                                                                                                                                                                                                                                                                                                                                                                                            |

---

## Optional — Destructive Action Confirmation

| Variable    | Default | Description                                                            |
| ----------- | ------- | ---------------------------------------------------------------------- |
| `YOLO_MODE` | `false` | Skip confirmation prompts for destructive actions (delete, force-push) |
| `AUTOPILOT` | `false` | Same as `YOLO_MODE` — skip all confirmation prompts                    |

These are checked by the elicitation subsystem. When the MCP client supports elicitation, destructive tools ask for user confirmation unless one of these is `true`.

---

## Optional — Upload

| Variable               | Default | Description                                                                                                                  |
| ---------------------- | ------- | ---------------------------------------------------------------------------------------------------------------------------- |
| `UPLOAD_MAX_FILE_SIZE` | `2GB`   | Maximum file size for upload tools. Supports human-friendly suffixes: `KB`, `MB`, `GB` (case-insensitive). Upper bound: 1 TB |

## Optional — Local Import Files

| Variable                         | Default   | Description                                                                                                                                                                                                                                                                                                    |
| -------------------------------- | --------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GITLAB_MCP_ALLOWED_IMPORT_DIRS` | *(empty)* | Additional OS path-list-separated directories allowed for `file_path`/`file` project and group import archives. The current working directory and OS temp directory are always allowed. Import archives must resolve inside an allowed directory after symlink resolution and must use the `.tar.gz` extension |

---

## Optional — HTTP Mode (Server Pool)

These variables configure the HTTP server pool. Each has a CLI flag counterpart, and the flag wins when it is passed explicitly — see [HTTP Mode Equivalents](#http-mode-equivalents) below. In stdio mode they are parsed but unused, since stdio runs a single server with no pool.

| Variable                      | Default   | Description                                                                                                                                                                  |
| ----------------------------- | --------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `MAX_HTTP_CLIENTS`            | `100`     | Maximum concurrent client sessions in the server pool. Upper bound: 10,000                                                                                                   |
| `SESSION_TIMEOUT`             | `30m`     | Idle MCP session timeout. Upper bound: 24h                                                                                                                                   |
| `POOL_IDLE_TIMEOUT`           | `1h`      | Reclaim a pooled per-token-and-URL server entry after this long unused; `0` keeps entries until the pool size bound evicts them. Upper bound: 24h                            |
| `SESSION_REVALIDATE_INTERVAL` | `15m`     | Token re-validation interval; `0` to disable. Upper bound: 24h                                                                                                               |
| `AUTH_MODE`                   | `legacy`  | Authentication mode: `legacy` (PRIVATE-TOKEN header passthrough) or `oauth` (RFC 9728 Bearer token verification via GitLab API)                                              |
| `OAUTH_CACHE_TTL`             | `15m`     | TTL for verified OAuth token identity cache. Range: 1m–2h                                                                                                                    |
| `PUBLIC_URL`                  | *(empty)* | Externally reachable origin of this deployment. Required with `AUTH_MODE=oauth` (RFC 9728 resource identifier); in legacy mode its origin seeds the trusted-origins list     |
| `RATE_LIMIT_RPS`              | `0`       | Per-server `tools/call` rate limit in requests/second. `0` disables the limiter (default). See [Security — Rate Limiting Model](../concepts/security.md#rate-limiting-model) |
| `RATE_LIMIT_BURST`            | `40`      | Token-bucket burst size when `RATE_LIMIT_RPS > 0`. Must be ≥ 1                                                                                                               |

---

## Optional — Auto-Update

| Variable               | Default                      | Description                                                                      |
| ---------------------- | ---------------------------- | -------------------------------------------------------------------------------- |
| `AUTO_UPDATE`          | `true`                       | Update mode: `true` (download and apply), `check` (log only), `false` (disabled) |
| `AUTO_UPDATE_REPO`     | `jmrplens/gitlab-mcp-server` | GitHub repository slug (owner/repo) for release assets                           |
| `AUTO_UPDATE_INTERVAL` | `1h`                         | Periodic update check interval (HTTP mode background checks)                     |
| `AUTO_UPDATE_TIMEOUT`  | `60s`                        | Startup/background update timeout (range: 5s–10m)                                |

> **Note**: Auto-update uses the GitHub Releases API via `AUTO_UPDATE_REPO`. See [Auto-Update](../guides/auto-update.md) for details.

---

## Configuration Loading Order

Configuration is loaded by `internal/config/` in this precedence order (higher wins):

1. **`.env` file** in the current working directory (loaded via `godotenv`)
2. **`~/.gitlab-mcp-server.env`** in the user's home directory (wizard-generated fallback)
3. **Environment variables** (override both `.env` files)

> **Note**: `godotenv` does not overwrite existing variables, so step 1 values take precedence over step 2, and explicit environment variables (step 3) override both.

---

## .env File Example

```env
# Required
GITLAB_TOKEN=glpat-xxxxxxxxxxxxxxxxxxxx

# Optional
GITLAB_SKIP_TLS_VERIFY=false
TOOL_SURFACE=dynamic
LOG_LEVEL=info
UPLOAD_MAX_FILE_SIZE=500MB
AUTO_UPDATE=true
```

For self-managed GitLab, add `GITLAB_URL=https://gitlab.example.com`.

> **Security**: The `.env` file is gitignored. Never commit tokens or credentials. The Setup Wizard writes secrets to `~/.gitlab-mcp-server.env` with `0600` permissions on Unix.

---

## HTTP Mode Equivalents

In HTTP mode, configuration resolves in three layers, highest first:

1. **A CLI flag passed explicitly on the command line.** Always wins.
2. **The environment variable**, when the corresponding flag was not passed.
3. **The built-in default.**

Passing a flag whose value happens to equal the default still counts as choosing it, so the environment cannot override it. See [CLI Reference](cli.md) for the full flag list.

Nothing in this table is reachable from a request. Clients control only their GitLab token and, when the server was started without `--gitlab-url`, the `GITLAB-URL` header; every other configuration header is ignored and reported in the `ignored_options` log field. See [Configuration Precedence](../guides/http-server-mode.md#configuration-precedence).

| Environment Variable          | CLI Flag                 | Notes                                                                                                                                                                                                                                                                    |
| ----------------------------- | ------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `GITLAB_URL`                  | `--gitlab-url`           | Optional in stdio mode; defaults to `https://gitlab.com`. Optional in HTTP mode unless `--auth-mode=oauth` is used, which requires a fixed `--gitlab-url`. When set in HTTP mode, it fixes the GitLab instance; when omitted, clients must send `GITLAB-URL` per request |
| `GITLAB_TOKEN`                | *(none)*                 | Not needed in HTTP mode — clients provide tokens per-request                                                                                                                                                                                                             |
| `GITLAB_SKIP_TLS_VERIFY`      | `--skip-tls-verify`      |                                                                                                                                                                                                                                                                          |
| `TOOL_SURFACE`                | `--tool-surface`         | Canonical selector: `meta`, `individual`, or `dynamic`                                                                                                                                                                                                                   |
| `META_TOOLS`                  | `--meta-tools`           | Deprecated compatibility selector. Use `TOOL_SURFACE` or `--tool-surface` for new configs                                                                                                                                                                                |
| `CAPABILITY_SURFACE`          | `--capability-surface`   | Explicit selector: `full` or `minimal`                                                                                                                                                                                                                                   |
| `META_PARAM_SCHEMA`           | `--meta-param-schema`    |                                                                                                                                                                                                                                                                          |
| `MAX_HTTP_CLIENTS`            | `--max-http-clients`     |                                                                                                                                                                                                                                                                          |
| `SESSION_TIMEOUT`             | `--session-timeout`      |                                                                                                                                                                                                                                                                          |
| *(none)*                      | `--http-idle-timeout`    | CLI-only; HTTP server idle connection timeout. Default `0` disables idle closure so `--session-timeout` is the effective lifetime                                                                                                                                        |
| `POOL_IDLE_TIMEOUT`           | `--pool-idle-timeout`    |                                                                                                                                                                                                                                                                          |
| `SESSION_REVALIDATE_INTERVAL` | `--revalidate-interval`  |                                                                                                                                                                                                                                                                          |
| `AUTH_MODE`                   | `--auth-mode`            |                                                                                                                                                                                                                                                                          |
| `OAUTH_CACHE_TTL`             | `--oauth-cache-ttl`      |                                                                                                                                                                                                                                                                          |
| `PUBLIC_URL`                  | `--public-url`           |                                                                                                                                                                                                                                                                          |
| `RATE_LIMIT_RPS`              | `--rate-limit-rps`       |                                                                                                                                                                                                                                                                          |
| `RATE_LIMIT_BURST`            | `--rate-limit-burst`     |                                                                                                                                                                                                                                                                          |
| *(none)*                      | `--trusted-proxy-header` | CLI-only; HTTP header with real client IP for rate limiting behind proxies                                                                                                                                                                                               |
| *(none)*                      | `--trusted-origins`      | CLI-only; origins allowed to make cross-origin browser requests (`*` accepts any)                                                                                                                                                                                        |
| `AUTO_UPDATE`                 | `--auto-update`          |                                                                                                                                                                                                                                                                          |
| `AUTO_UPDATE_REPO`            | `--auto-update-repo`     |                                                                                                                                                                                                                                                                          |
| `AUTO_UPDATE_INTERVAL`        | `--auto-update-interval` |                                                                                                                                                                                                                                                                          |
| `AUTO_UPDATE_TIMEOUT`         | `--auto-update-timeout`  |                                                                                                                                                                                                                                                                          |
| `GITLAB_TIER`                 | `--tier`                 | In HTTP mode, an explicit flag forces the tier; when omitted, the tier is detected from the instance license per token+URL pool entry (fallback `free`)                                                                                                                  |
| `GITLAB_READ_ONLY`            | `--read-only`            |                                                                                                                                                                                                                                                                          |
| `GITLAB_SAFE_MODE`            | `--safe-mode`            |                                                                                                                                                                                                                                                                          |
| `EMBEDDED_RESOURCES`          | `--embedded-resources`   |                                                                                                                                                                                                                                                                          |
| `EXCLUDE_TOOLS`               | `--exclude-tools`        | Comma-separated list                                                                                                                                                                                                                                                     |
| `GITLAB_IGNORE_SCOPES`        | `--ignore-scopes`        |                                                                                                                                                                                                                                                                          |

---

## See Also

- [CLI Reference](cli.md) — Command-line flags for HTTP mode
- [Configuration](configuration.md) — Setup wizard, client config, secure token management
- [Dynamic Toolset](../concepts/dynamic-tools.md) — Low-token find/execute workflow and migration guidance
- [Auto-Update](../guides/auto-update.md) — Update modes and release requirements
- [Security](../concepts/security.md) — Token management best practices
- [HTTP Server Mode](../guides/http-server-mode.md) — OAuth mode architecture and deployment
- [OAuth App Setup](../guides/oauth-app-setup.md) — Creating GitLab OAuth applications
