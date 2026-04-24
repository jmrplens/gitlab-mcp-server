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

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `-h` | bool | `false` | Show full help with flags, environment variables, and JSON examples |
| `-version` | bool | `false` | Print version and commit hash, then exit |
| `-shutdown` | bool | `false` | Terminate all running instances and exit (used by external updaters) |
| `-setup` | bool | `false` | Run the interactive Setup Wizard |
| `-setup-mode` | string | `auto` | Setup UI mode: `auto`, `web`, `tui`, `cli` |

### HTTP Transport Mode

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `-http` | bool | `false` | Enable HTTP transport mode (default is stdio) |
| `-http-addr` | string | `:8080` | HTTP listen address (e.g. `localhost:8080`, `:9090`) |
| `-gitlab-url` | string | _(optional)_ | Default GitLab instance URL. Per-request override via `GITLAB-URL` header |
| `-skip-tls-verify` | bool | `false` | Skip TLS certificate verification for self-signed certs |
| `-meta-tools` | bool | `true` | Enable domain-level meta-tools (28 base / 43 enterprise instead of 1005) |
| `-enterprise` | bool | `false` | Enable Enterprise/Premium meta-tools (15 additional) |
| `-read-only` | bool | `false` | Read-only mode: disables all mutating tools. Only tools with `ReadOnlyHint=true` remain available |
| `-safe-mode` | bool | `false` | Safe mode: intercepts mutating tools and returns a JSON preview instead of executing. If `--read-only` is also set, it takes precedence |
| `-max-http-clients` | int | `100` | Maximum concurrent client sessions (upper bound: 10,000) |
| `-session-timeout` | duration | `30m` | Idle MCP session timeout (upper bound: 24h) |
| `-revalidate-interval` | duration | `15m` | Token re-validation interval; `0` to disable (upper bound: 24h) |
| `-auth-mode` | string | `legacy` | Authentication mode: `legacy` (PRIVATE-TOKEN header passthrough) or `oauth` (RFC 9728 Bearer token verification via GitLab API). See [HTTP Server Mode — OAuth Mode](http-server-mode.md#oauth-mode) |
| `-oauth-cache-ttl` | duration | `15m` | TTL for verified OAuth token identity cache. Range: 1m–2h. Only applies when `--auth-mode=oauth` |
| `-trusted-proxy-header` | string | _(empty)_ | HTTP header containing the real client IP when behind a reverse proxy (e.g. `Fly-Client-IP`, `X-Forwarded-For`, `X-Real-IP`). Required for accurate rate limiting behind proxies |

### Auto-Update

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `-auto-update` | string | `true` | Auto-update mode: `true` (auto-apply), `check` (log-only), `false` (disabled) |
| `-auto-update-repo` | string | `jmrplens/gitlab-mcp-server` | GitHub repository slug (owner/repo) for update release assets |
| `-auto-update-interval` | duration | `1h` | How often to check for new releases (HTTP mode periodic checks) |
| `-auto-update-timeout` | duration | `60s` | Timeout for pre-start update download (range: 5s–10m) |

---

## Modes of Operation

### Stdio Mode (Default)

The server reads configuration from environment variables and communicates via stdin/stdout JSON-RPC. This is the standard mode for MCP clients like VS Code, Claude Desktop, and Cursor.

```bash
# Configuration via environment variables
export GITLAB_URL="https://gitlab.example.com"
export GITLAB_TOKEN="glpat-xxxxxxxxxxxxxxxxxxxx"
gitlab-mcp-server
```

```bash
# Configuration via .env file in current directory
gitlab-mcp-server
```

### HTTP Mode

The server listens on an HTTP endpoint. Each client provides its own GitLab token per-request via `PRIVATE-TOKEN` header or `Authorization: Bearer`. Clients can also specify a `GITLAB-URL` header to target a specific GitLab instance per-request. No `GITLAB_TOKEN` is needed at startup.

```bash
# Single GitLab instance (all clients use the same default)
gitlab-mcp-server --http --gitlab-url=https://gitlab.example.com
gitlab-mcp-server --http --gitlab-url=https://gitlab.example.com --http-addr=localhost:9090
gitlab-mcp-server --http --gitlab-url=https://gitlab.example.com --max-http-clients=50 --session-timeout=1h
gitlab-mcp-server --http --gitlab-url=https://gitlab.example.com --auth-mode=oauth --oauth-cache-ttl=15m

# Multi-instance (each client specifies their GitLab URL via GITLAB-URL header)
gitlab-mcp-server --http --http-addr=:8080
```

### Setup Wizard

The interactive wizard configures the binary, GitLab connection, and MCP client files.

```bash
gitlab-mcp-server --setup                    # Auto-detect UI mode
gitlab-mcp-server --setup -setup-mode web    # Browser-based UI
gitlab-mcp-server --setup -setup-mode tui    # Terminal UI (Bubble Tea)
gitlab-mcp-server --setup -setup-mode cli    # Plain text fallback
```

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
gitlab-mcp-server --http --gitlab-url=https://gitlab.example.com --http-addr=:9090

# Start HTTP server without default URL (clients must send GITLAB-URL header)
gitlab-mcp-server --http --http-addr=:8080

# Start HTTP server with TLS skip and custom session timeout
gitlab-mcp-server --http --gitlab-url=https://gitlab.example.com --skip-tls-verify --session-timeout=2h

# Start HTTP server with individual tools (no meta-tools)
gitlab-mcp-server --http --gitlab-url=https://gitlab.example.com --meta-tools=false

# Start with auto-update in check-only mode
gitlab-mcp-server --http --gitlab-url=https://gitlab.example.com --auto-update=check

# Terminate all running instances (used by external updaters)
gitlab-mcp-server --shutdown
```

---

## Exit Codes

| Code | Meaning |
| --- | --- |
| `0` | Normal exit (signal-based shutdown, `-version`, `-h`, or `--shutdown`) |
| `1` | Configuration error, connection failure, runtime error, or `--shutdown` failure |

---

## See Also

- [Configuration](configuration.md) — Environment variables and `.env` files
- [HTTP Server Mode](http-server-mode.md) — Architecture and deployment details
- [Auto-Update](auto-update.md) — Update modes, release requirements, troubleshooting
- [Getting Started](getting-started.md) — Step-by-step tutorial
