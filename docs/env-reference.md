# Environment Variable Reference

> **Diátaxis type**: Reference
> **Audience**: 👤🔧 All users
> **Prerequisites**: gitlab-mcp-server binary installed
>
> Complete environment variable reference for gitlab-mcp-server stdio mode.

---

## Required

| Variable | Description | Example |
| --- | --- | --- |
| `GITLAB_URL` | GitLab instance base URL (must use `http://` or `https://` scheme) | `https://gitlab.example.com` |
| `GITLAB_TOKEN` | Personal Access Token with `api` scope | `glpat-xxxxxxxxxxxxxxxxxxxx` |

---

## Optional — Connection

| Variable | Default | Description |
| --- | --- | --- |
| `GITLAB_SKIP_TLS_VERIFY` | `false` | Skip TLS certificate verification (`true`/`false`). Use for self-signed certs |

---

## Optional — Server Behavior

| Variable | Default | Description |
| --- | --- | --- |
| `META_TOOLS` | `true` | Enable domain-level meta-tools: `true` (32 base / 47 enterprise) or `false` (1006 individual tools) |
| `META_PARAM_SCHEMA` | `opaque` | Meta-tool input-schema strategy: `opaque` (compact `{action, params:any}` envelope, default), `compact` (oneOf with property names + types only, ~5x size) or `full` (oneOf with full per-action JSON Schemas, ~10x size). Independent of `META_TOOLS`. The per-action JSON Schema is always discoverable via the `gitlab://schema/meta/{tool}/{action}` MCP resource regardless of mode |
| `GITLAB_ENTERPRISE` | `false` | Enable Enterprise/Premium tools for GitLab Premium/Ultimate features. Gates 35 individual tool sub-packages and 15 dedicated meta-tools (plus enterprise routes in 3 base meta-tools) |
| `LOG_LEVEL` | `info` | Logging verbosity: `debug`, `info`, `warn`, `error` |
| `GITLAB_READ_ONLY` | `false` | Read-only mode: disables all mutating tools at startup. Only tools with `ReadOnlyHint=true` remain available (`true`/`false`) |
| `GITLAB_SAFE_MODE` | `false` | Safe mode: intercepts mutating tools and returns a structured JSON preview instead of executing. Read-only tools work normally. If `GITLAB_READ_ONLY=true`, it takes precedence (`true`/`false`) |
| `EMBEDDED_RESOURCES` | `true` | Embed canonical `gitlab://` MCP resource URIs as `EmbeddedResource` content blocks in `gitlab_*_get` tool results. Set to `false` to disable for clients that don't tolerate duplicate content blocks (`true`/`false`) |
| `EXCLUDE_TOOLS` | *(empty)* | Comma-separated list of tool names to exclude from registration (e.g. `gitlab_admin,gitlab_runner`) |
| `GITLAB_IGNORE_SCOPES` | `false` | Skip PAT scope detection and register all tools regardless of token permissions (`true`/`false`) |

---

## Optional — Destructive Action Confirmation

| Variable | Default | Description |
| --- | --- | --- |
| `YOLO_MODE` | `false` | Skip confirmation prompts for destructive actions (delete, force-push) |
| `AUTOPILOT` | `false` | Same as `YOLO_MODE` — skip all confirmation prompts |

These are checked by the elicitation subsystem. When the MCP client supports elicitation, destructive tools ask for user confirmation unless one of these is `true`.

---

## Optional — Upload

| Variable | Default | Description |
| --- | --- | --- |
| `UPLOAD_MAX_FILE_SIZE` | `2GB` | Maximum file size for upload tools. Supports human-friendly suffixes: `KB`, `MB`, `GB` (case-insensitive). Upper bound: 1 TB |

---

## Optional — HTTP Mode (Server Pool)

These variables configure the HTTP server pool when running in HTTP mode. In stdio mode, they are parsed but only used if the configuration is shared with HTTP mode logic.

| Variable | Default | Description |
| --- | --- | --- |
| `MAX_HTTP_CLIENTS` | `100` | Maximum concurrent client sessions in the server pool. Upper bound: 10,000 |
| `SESSION_TIMEOUT` | `30m` | Idle MCP session timeout. Upper bound: 24h |
| `SESSION_REVALIDATE_INTERVAL` | `15m` | Token re-validation interval; `0` to disable. Upper bound: 24h |
| `AUTH_MODE` | `legacy` | Authentication mode: `legacy` (PRIVATE-TOKEN header passthrough) or `oauth` (RFC 9728 Bearer token verification via GitLab API) |
| `OAUTH_CACHE_TTL` | `15m` | TTL for verified OAuth token identity cache. Range: 1m–2h |
| `RATE_LIMIT_RPS` | `0` | Per-server `tools/call` rate limit in requests/second. `0` disables the limiter (default). See [Security — Rate Limiting Model](security.md#rate-limiting-model) |
| `RATE_LIMIT_BURST` | `40` | Token-bucket burst size when `RATE_LIMIT_RPS > 0`. Must be ≥ 1 |

---

## Optional — Auto-Update

| Variable | Default | Description |
| --- | --- | --- |
| `AUTO_UPDATE` | `true` | Update mode: `true` (download and apply), `check` (log only), `false` (disabled) |
| `AUTO_UPDATE_REPO` | `jmrplens/gitlab-mcp-server` | GitHub repository slug (owner/repo) for release assets |
| `AUTO_UPDATE_INTERVAL` | `1h` | Periodic update check interval (HTTP mode background checks) |
| `AUTO_UPDATE_TIMEOUT` | `60s` | Pre-start download timeout (range: 5s–10m) |

> **Note**: Auto-update uses the GitHub Releases API via `AUTO_UPDATE_REPO`. See [Auto-Update](auto-update.md) for details.

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
GITLAB_URL=https://gitlab.example.com
GITLAB_TOKEN=glpat-xxxxxxxxxxxxxxxxxxxx

# Optional
GITLAB_SKIP_TLS_VERIFY=true
META_TOOLS=true
LOG_LEVEL=info
UPLOAD_MAX_FILE_SIZE=500MB
AUTO_UPDATE=true
```

> **Security**: The `.env` file is gitignored. Never commit tokens or credentials. The Setup Wizard writes secrets to `~/.gitlab-mcp-server.env` with `0600` permissions on Unix.

---

## HTTP Mode Equivalents

In HTTP mode, configuration comes from CLI flags instead of environment variables. See [CLI Reference](cli-reference.md) for the full flag list.

| Environment Variable | CLI Flag | Notes |
| --- | --- | --- |
| `GITLAB_URL` | `--gitlab-url` | Required in stdio mode. Optional in HTTP mode (per-request override via `GITLAB-URL` header) |
| `GITLAB_TOKEN` | *(none)* | Not needed in HTTP mode — clients provide tokens per-request |
| `GITLAB_SKIP_TLS_VERIFY` | `--skip-tls-verify` | |
| `META_TOOLS` | `--meta-tools` | |
| `META_PARAM_SCHEMA` | `--meta-param-schema` | |
| `MAX_HTTP_CLIENTS` | `--max-http-clients` | |
| `SESSION_TIMEOUT` | `--session-timeout` | |
| `SESSION_REVALIDATE_INTERVAL` | `--revalidate-interval` | |
| `AUTH_MODE` | `--auth-mode` | |
| `OAUTH_CACHE_TTL` | `--oauth-cache-ttl` | |
| `RATE_LIMIT_RPS` | `--rate-limit-rps` | |
| `RATE_LIMIT_BURST` | `--rate-limit-burst` | |
| *(none)* | `--trusted-proxy-header` | CLI-only; HTTP header with real client IP for rate limiting behind proxies |
| `AUTO_UPDATE` | `--auto-update` | |
| `AUTO_UPDATE_REPO` | `--auto-update-repo` | |
| `AUTO_UPDATE_INTERVAL` | `--auto-update-interval` | |
| `AUTO_UPDATE_TIMEOUT` | `--auto-update-timeout` | |
| `GITLAB_ENTERPRISE` | `--enterprise` | |
| `GITLAB_READ_ONLY` | `--read-only` | |
| `GITLAB_SAFE_MODE` | `--safe-mode` | |
| `EMBEDDED_RESOURCES` | `--embedded-resources` | |
| `EXCLUDE_TOOLS` | `--exclude-tools` | Comma-separated list |
| `GITLAB_IGNORE_SCOPES` | `--ignore-scopes` | |

---

## See Also

- [CLI Reference](cli-reference.md) — Command-line flags for HTTP mode
- [Configuration](configuration.md) — Setup wizard, client config, secure token management
- [Auto-Update](auto-update.md) — Update modes and release requirements
- [Security](security.md) — Token management best practices
- [HTTP Server Mode](http-server-mode.md) — OAuth mode architecture and deployment
- [OAuth App Setup](oauth-app-setup.md) — Creating GitLab OAuth applications
