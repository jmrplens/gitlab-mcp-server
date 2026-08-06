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

When run without flags and a `GITLAB_TOKEN` is set, the server starts in **stdio mode**. When no token is available and the terminal is interactive, the **Setup Wizard** launches automatically.

---

## Flags

### General

| Flag           | Type   | Default   | Description                                                          |
| -------------- | ------ | --------- | -------------------------------------------------------------------- |
| `-h`           | bool   | `false`   | Show full help with flags, environment variables, and JSON examples  |
| `-version`     | bool   | `false`   | Print version and commit hash, then exit                             |
| `-shutdown`    | bool   | `false`   | Terminate all running instances and exit (used by external updaters) |
| `-setup`       | bool   | `false`   | Run the interactive Setup Wizard                                     |
| `-setup-mode`  | string | `auto`    | Setup UI mode: `auto` (cascade), `web`, `tui`, or `cli`              |
| `-tool-search` | string | _(empty)_ | Search registered tools by name or description, then exit            |

### HTTP Transport Mode

| Flag                      | Type     | Default                    | Description                                                                                                                                                                                                                                                                                   |
| ------------------------- | -------- | -------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `-http`                   | bool     | `false`                    | Enable HTTP transport mode (default is stdio)                                                                                                                                                                                                                                                 |
| `-http-addr`              | string   | `:8080`                    | HTTP listen address (e.g. `localhost:8080`, `:9090`)                                                                                                                                                                                                                                          |
| `-gitlab-url`             | string   | _(optional)_               | Fixed GitLab instance URL. Omit it to require each client to send `GITLAB-URL` per request                                                                                                                                                                                                    |
| `-skip-tls-verify`        | bool     | `false`                    | Skip TLS certificate verification for self-signed certs                                                                                                                                                                                                                                       |
| `-tool-surface`           | string   | `dynamic`                  | Canonical tool catalog selector: `meta`, `individual`, or `dynamic`                                                                                                                                                                                                                           |
| `-meta-tools`             | bool     | _(unset; effective false)_ | Deprecated compatibility flag ignored when `--tool-surface` is set. Leave unset for the default dynamic surface; use `--tool-surface=individual` when migrating old `--meta-tools=false` configs.                                                                                             |
| `-capability-surface`     | string   | `full`                     | Resource and prompt catalog selector: `full` or `minimal`. Minimal keeps the `gitlab://tools` manifest, and disables optional GitLab data resources, workflow guides, and prompts                                                                                                             |
| `-meta-param-schema`      | string   | `opaque`                   | Meta-tool input-schema strategy: `opaque` (default), `compact`, or `full`. Applies to meta-tool schemas only. See [Environment Variables](env.md)                                                                                                                                             |
| `-tier`                   | string   | _(detected)_               | Force the licensing tier (`free`, `ce`, `premium`, `ultimate`) when explicitly set. When omitted, HTTP mode detects the tier per token+URL pool entry from the instance license (fallback `free`)                                                                                             |
| `-read-only`              | bool     | `false`                    | Read-only mode: removes every mutating operation, per action, while read operations keep working on all surfaces                                                                                                                                                                              |
| `-safe-mode`              | bool     | `false`                    | Safe mode: intercepts mutating operations per action and returns a JSON preview instead of executing; reads keep working. If `--read-only` is also set, it takes precedence                                                                                                                   |
| `-embedded-resources`     | bool     | `true`                     | Embed canonical `gitlab://` MCP resource URIs as `EmbeddedResource` content blocks in `gitlab_*_get` tool results. Set `false` to disable for clients that don't tolerate duplicate content blocks                                                                                            |
| `-exclude-tools`          | string   | _(empty)_                  | Comma-separated list of tool names to exclude from registration                                                                                                                                                                                                                               |
| `-ignore-scopes`          | bool     | `false`                    | Skip PAT scope detection and register all tools regardless of token permissions                                                                                                                                                                                                               |
| `-max-http-clients`       | int      | `100`                      | Maximum concurrent client sessions (upper bound: 10,000)                                                                                                                                                                                                                                      |
| `-session-timeout`        | duration | `30m`                      | Idle MCP session timeout (upper bound: 24h)                                                                                                                                                                                                                                                   |
| `-revalidate-interval`    | duration | `15m`                      | Token re-validation interval; `0` to disable (upper bound: 24h)                                                                                                                                                                                                                               |
| `-http-idle-timeout`      | duration | `0` (disabled)             | HTTP server idle connection timeout. Default `0` disables idle connection closure entirely, so `--session-timeout` is the effective session lifetime. Set a positive duration to recycle idle connections sooner                                                                              |
| `-auth-mode`              | string   | `legacy`                   | Authentication mode: `legacy` (PRIVATE-TOKEN header passthrough) or `oauth` (RFC 9728 Bearer token verification via GitLab API). See [HTTP Server Mode — OAuth Mode](../guides/http-server-mode.md#oauth-mode)                                                                                |
| `-oauth-cache-ttl`        | duration | `15m`                      | TTL for verified OAuth token identity cache. Range: 1m–2h. Only applies when `--auth-mode=oauth`                                                                                                                                                                                              |
| `-trusted-proxy-header`   | string   | _(empty)_                  | HTTP header containing the real client IP when behind a reverse proxy (e.g. `CF-Connecting-IP`, `X-Forwarded-For`, `X-Real-IP`). Required for accurate rate limiting behind proxies                                                                                                           |
| `-rate-limit-rps`         | float    | `0`                        | Per-server `tools/call` rate limit in requests/second. `0` disables. See [Security — Rate Limiting Model](../concepts/security.md#rate-limiting-model)                                                                                                                                        |
| `-rate-limit-burst`       | int      | `40`                       | Token-bucket burst size when `--rate-limit-rps > 0`. Must be ≥ 1                                                                                                                                                                                                                              |
| `-stateless`              | bool     | `true`                     | Sessionless streamable HTTP (SEP-2567 / protocol 2026-07-28): no `Mcp-Session-Id` tracking, every POST is self-contained, GET/DELETE return `405`. Use `-stateless=false` for legacy stateful sessions. See [HTTP Server Mode — Stateless Mode](../guides/http-server-mode.md#stateless-mode) |
| `-json-response`          | bool     | `false`                    | Return `application/json` response bodies instead of `text/event-stream` (SSE)                                                                                                                                                                                                                |
| `-max-request-body-bytes` | int64    | `0`                        | Maximum streamable HTTP request body size in bytes; `0` uses the SDK default (4 MiB). Oversized bodies are rejected with `413`; negative values are rejected at startup                                                                                                                       |

### Auto-Update

| Flag                    | Type     | Default                      | Description                                                                   |
| ----------------------- | -------- | ---------------------------- | ----------------------------------------------------------------------------- |
| `-auto-update`          | string   | `true`                       | Auto-update mode: `true` (auto-apply), `check` (log-only), `false` (disabled) |
| `-auto-update-repo`     | string   | `jmrplens/gitlab-mcp-server` | GitHub repository slug (owner/repo) for update release assets                 |
| `-auto-update-interval` | duration | `1h`                         | How often to check for new releases (HTTP mode periodic checks)               |
| `-auto-update-timeout`  | duration | `60s`                        | Timeout for startup/background update checks (range: 5s–10m)                  |

---

## Modes of Operation

### Stdio Mode (Default)

The server reads configuration from environment variables and communicates via stdin/stdout JSON-RPC. This is the standard mode for MCP clients like VS Code, Claude Desktop, and Cursor.

```bash
# Configuration via environment variables
export GITLAB_TOKEN="glpat-xxxxxxxxxxxxxxxxxxxx"
gitlab-mcp-server
```

Set `GITLAB_URL` only for self-managed instances; stdio mode defaults to `https://gitlab.com`.

```bash
# Configuration via .env file in current directory
gitlab-mcp-server
```

### HTTP Mode

The server listens on an HTTP endpoint. Each client provides its own GitLab token per-request via `PRIVATE-TOKEN` header or `Authorization: Bearer`. When the server starts without `--gitlab-url`, clients also specify a `GITLAB-URL` header to target a specific GitLab instance per request. No `GITLAB_TOKEN` is needed at startup.

```bash
# Single GitLab.com instance (all clients use the fixed URL; replace for self-managed GitLab)
gitlab-mcp-server --http --gitlab-url=https://gitlab.com
gitlab-mcp-server --http --gitlab-url=https://gitlab.com --http-addr=localhost:9090
gitlab-mcp-server --http --gitlab-url=https://gitlab.com --max-http-clients=50 --session-timeout=1h
gitlab-mcp-server --http --gitlab-url=https://gitlab.com --auth-mode=oauth --oauth-cache-ttl=15m

# Stateless streamable HTTP (SEP-2567, the default) with plain JSON responses
gitlab-mcp-server --http --gitlab-url=https://gitlab.com --json-response

# Legacy stateful sessions (opt-out) with a 1 MiB request body cap
gitlab-mcp-server --http --gitlab-url=https://gitlab.com --stateless=false --max-request-body-bytes=1048576

# Multi-instance (each client specifies their GitLab URL via GITLAB-URL header)
gitlab-mcp-server --http --http-addr=:8080
```

### Setup Wizard

The interactive wizard configures the binary, GitLab connection, and MCP client files. The wizard is **stdio-only**: it does not configure the HTTP server (use [HTTP Server Mode](../guides/http-server-mode.md) for that).

```bash
gitlab-mcp-server --setup                    # Auto-detect UI mode (web → tui → cli)
gitlab-mcp-server --setup --setup-mode web   # Browser-based UI (best for first-time setup)
gitlab-mcp-server --setup --setup-mode tui   # Terminal UI (Bubble Tea, keyboard-driven)
gitlab-mcp-server --setup --setup-mode cli   # Plain text prompts (headless / SSH)
```

The `--setup-mode` flag controls which user interface the wizard uses:

- `auto` (default) — cascade: try Web UI first (when a graphical display is detected), then Bubble Tea TUI (when stdin is a TTY), then plain CLI prompts. The cascade is transparent and falls back automatically on headless or terminal-only environments.
- `web` — local HTTP server on `127.0.0.1`, opens your default browser, and waits until you submit the form. Advanced options in this UI carry inline help tooltips.
- `tui` — full-screen terminal interface built with Bubble Tea. Keyboard navigation (Tab/Shift+Tab, Space, Enter, arrow keys, `Ctrl+O` for advanced options, `Esc` to cancel).
- `cli` — line-by-line prompts read from stdin. Safest choice for SSH sessions, CI, or any environment where browsers and TUI rendering are unavailable.

When `~/.gitlab-mcp-server.env` already exists, every UI mode pre-loads the saved values, so re-running the wizard lets you change only the fields you need. Leaving the token field blank keeps the previously stored token. See [Configuration — Setup Wizard](configuration.md#setup-wizard-recommended) for the full list of fields and their env-var mappings.

On Windows, double-clicking the `.exe` when no `GITLAB_TOKEN` is set launches the wizard automatically.

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

---

## Examples

```bash
# Print version
gitlab-mcp-server -version

# Show help with all flags and JSON configuration examples
gitlab-mcp-server -h

# Start stdio server (reads .env from current directory)
gitlab-mcp-server

# Start HTTP server with custom address
gitlab-mcp-server --http --gitlab-url=https://gitlab.com --http-addr=:9090

# Start HTTP server without fixed URL (clients must send GITLAB-URL header)
gitlab-mcp-server --http --http-addr=:8080

# Start HTTP server for self-managed GitLab with TLS skip and custom session timeout
gitlab-mcp-server --http --gitlab-url=https://gitlab.example.com --skip-tls-verify --session-timeout=2h

# Start HTTP server with individual tools
gitlab-mcp-server --http --gitlab-url=https://gitlab.com --tool-surface=individual

# Start HTTP server with the dynamic toolset (reduces token usage for LLM context)
gitlab-mcp-server --http --gitlab-url=https://gitlab.com --tool-surface=dynamic

# Start HTTP server with the dynamic toolset and reduced non-tool capabilities
gitlab-mcp-server --http --gitlab-url=https://gitlab.com --tool-surface=dynamic --capability-surface=minimal

# Start with auto-update in check-only mode
gitlab-mcp-server --http --gitlab-url=https://gitlab.com --auto-update=check

# Terminate all running instances (used by external updaters)
gitlab-mcp-server --shutdown
```

See [Dynamic Tools](../concepts/dynamic-tools.md) for how `dynamic` relates.

---

## Exit Codes

| Code | Meaning                                                                         |
| ---- | ------------------------------------------------------------------------------- |
| `0`  | Normal exit (signal-based shutdown, `-version`, `-h`, or `--shutdown`)          |
| `1`  | Configuration error, connection failure, runtime error, or `--shutdown` failure |

---

## See Also

- [Configuration](configuration.md) — Environment variables and `.env` files
- [HTTP Server Mode](../guides/http-server-mode.md) — Architecture and deployment details
- [Dynamic Toolset](../concepts/dynamic-tools.md) — Low-token find/execute mode
- [Auto-Update](../guides/auto-update.md) — Update modes, release requirements, troubleshooting
- [Getting Started](../getting-started.md) — Step-by-step tutorial
