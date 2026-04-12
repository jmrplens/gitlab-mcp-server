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
| `GITLAB_USER` | _(auto-detected)_ | GitLab username; used by prompts and resources. Auto-detected from token if not set |
| `GITLAB_SKIP_TLS_VERIFY` | `false` | Skip TLS certificate verification (`true`/`false`). Use for self-signed certs |

---

## Optional — Server Behavior

| Variable | Default | Description |
| --- | --- | --- |
| `META_TOOLS` | `true` | Enable domain-level meta-tools: `true` (40 base / 59 enterprise) or `false` (1004 individual tools) |
| `GITLAB_ENTERPRISE` | `false` | Enable Enterprise/Premium meta-tools: 14 additional domain tools for Premium/Ultimate features |
| `LOG_LEVEL` | `info` | Logging verbosity: `debug`, `info`, `warn`, `error` |
| `ISSUE_REPORTS` | `false` | Generate GitLab issue reports on unrecoverable tool errors (`true`/`false`) |
| `GITLAB_READ_ONLY` | `false` | Read-only mode: disables all mutating tools at startup. Only tools with `ReadOnlyHint=true` remain available (`true`/`false`) |

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

---

## Optional — Auto-Update

| Variable | Default | Description |
| --- | --- | --- |
| `AUTO_UPDATE` | `true` | Update mode: `true` (download and apply), `check` (log only), `false` (disabled) |
| `AUTO_UPDATE_REPO` | `jmrplens/gitlab-mcp-server` | GitHub repository slug (owner/repo) for release assets |
| `AUTO_UPDATE_INTERVAL` | `1h` | Periodic update check interval (HTTP mode background checks) |

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
GITLAB_USER=myusername
GITLAB_SKIP_TLS_VERIFY=true
META_TOOLS=true
LOG_LEVEL=info
ISSUE_REPORTS=false
UPLOAD_MAX_FILE_SIZE=500MB
AUTO_UPDATE=true
```

> **Security**: The `.env` file is gitignored. Never commit tokens or credentials. The Setup Wizard writes secrets to `~/.gitlab-mcp-server.env` with `0600` permissions on Unix.

---

## HTTP Mode Equivalents

In HTTP mode, configuration comes from CLI flags instead of environment variables. See [CLI Reference](cli-reference.md) for the full flag list.

| Environment Variable | CLI Flag | Notes |
| --- | --- | --- |
| `GITLAB_URL` | `--gitlab-url` | Required in both modes |
| `GITLAB_TOKEN` | _(none)_ | Not needed in HTTP mode — clients provide tokens per-request |
| `GITLAB_SKIP_TLS_VERIFY` | `--skip-tls-verify` | |
| `META_TOOLS` | `--meta-tools` | |
| `MAX_HTTP_CLIENTS` | `--max-http-clients` | |
| `SESSION_TIMEOUT` | `--session-timeout` | |
| `SESSION_REVALIDATE_INTERVAL` | `--revalidate-interval` | |
| `AUTO_UPDATE` | `--auto-update` | |
| `AUTO_UPDATE_REPO` | `--auto-update-repo` | |
| `AUTO_UPDATE_INTERVAL` | `--auto-update-interval` | |

---

## See Also

- [CLI Reference](cli-reference.md) — Command-line flags for HTTP mode
- [Configuration](configuration.md) — Setup wizard, client config, secure token management
- [Auto-Update](auto-update.md) — Update modes and release requirements
- [Security](security.md) — Token management best practices
