# Configuration

gitlab-mcp-server is configured through environment variables. A `.env` file in the current directory is loaded automatically (via `godotenv`), and the server also loads `~/.gitlab-mcp-server.env` as a fallback for secrets written by the Setup Wizard.

> **Diátaxis type**: Reference
> **Audience**: 👤🔧 All users
> **Prerequisites**: A running GitLab instance with a Personal Access Token
> 📖 **User documentation**: See the [Configuration](https://jmrplens.github.io/gitlab-mcp-server/configuration/) on the documentation site for a user-friendly version.
>
> **Using in CI/CD?** See the [CI/CD Usage](ci-cd.md) guide for pipeline-specific setup with Project Access Tokens.

---

## Personal Setup

These are the settings every user needs to get started.

### Required Variables

| Variable       | Description                            | Example                      |
| -------------- | -------------------------------------- | ---------------------------- |
| `GITLAB_TOKEN` | Personal Access Token with `api` scope | `glpat-xxxxxxxxxxxxxxxxxxxx` |

### Common Options

| Variable                 | Default              | Description                                                                                                                                                                                                                                  |
| ------------------------ | -------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GITLAB_URL`             | `https://gitlab.com` | GitLab instance base URL. Set this for self-managed instances                                                                                                                                                                                |
| `GITLAB_SKIP_TLS_VERIFY` | `false`              | Skip TLS certificate verification for self-signed certs                                                                                                                                                                                      |
| `TOOL_SURFACE`           | `dynamic`            | Canonical tool catalog selector: `dynamic`, `meta`, or `individual`                                                                                                                                                                          |
| `META_TOOLS`             | *(legacy)*           | Deprecated compatibility selector. Accepted values map to `TOOL_SURFACE`: `true` -> `meta`, `false` -> `individual`, and `dynamic` -> `dynamic`. Ignored when `TOOL_SURFACE` is set                                                          |
| `CAPABILITY_SURFACE`     | `full`               | Resource and prompt catalog selector: `full` preserves all resources, workflow guides, prompts, and the surface-aware `gitlab://tools` manifest; `minimal` keeps `gitlab://workspace/roots` plus `gitlab://tools` only                       |
| `GITLAB_ENTERPRISE`      | `false`              | Enable Enterprise/Premium tools in stdio mode. In HTTP mode, `--enterprise` explicitly forces the Enterprise/Premium catalog; when omitted, CE/EE is auto-detected per token+URL pool entry when GitLab reports edition in `/api/v4/version` |
| `GITLAB_READ_ONLY`       | `false`              | Read-only mode: disables all mutating tools at startup                                                                                                                                                                                       |
| `GITLAB_SAFE_MODE`       | `false`              | Safe mode: intercepts mutating tools and returns a JSON preview instead of executing. Read-only tools work normally. If `GITLAB_READ_ONLY=true`, it takes precedence                                                                         |
| `EXCLUDE_TOOLS`          | *(empty)*            | Comma-separated list of tool names to exclude from registration                                                                                                                                                                              |
| `GITLAB_IGNORE_SCOPES`   | `false`              | Skip PAT scope detection and register all tools regardless of token permissions                                                                                                                                                              |
| `LOG_LEVEL`              | `info`               | Logging verbosity: `debug`, `info`, `warn`, `error`                                                                                                                                                                                          |

### .env File Example

```env
GITLAB_TOKEN=glpat-xxxxxxxxxxxxxxxxxxxx
GITLAB_SKIP_TLS_VERIFY=false
TOOL_SURFACE=dynamic
GITLAB_READ_ONLY=false
GITLAB_SAFE_MODE=false
LOG_LEVEL=info
```

For self-managed GitLab, add `GITLAB_URL=https://gitlab.example.com`.

> **Security**: The `.env` file is gitignored. Never commit tokens or credentials.

---

## Setup Wizard (Recommended)

The easiest way to configure gitlab-mcp-server is through the built-in **Setup Wizard**. It installs the binary, configures your GitLab connection, and writes MCP client config files — all in one step.

```bash
# Run the wizard (auto-detects best UI: web → TUI → CLI)
gitlab-mcp-server --setup

# Or force a specific UI mode
gitlab-mcp-server --setup -setup-mode web   # Opens browser-based UI
gitlab-mcp-server --setup -setup-mode tui   # Terminal UI (Bubble Tea)
gitlab-mcp-server --setup -setup-mode cli   # Plain text fallback
```

On **Windows**, double-click the `.exe` — if no `GITLAB_TOKEN` is set, the wizard starts automatically.

The wizard supports 10 MCP clients: VS Code (GitHub Copilot), Claude Desktop, Claude Code (CLI), Cursor, Windsurf (Codeium), JetBrains IDEs, Copilot CLI, OpenCode, Crush (Charm), and Zed.

**Secure secret storage**: The wizard writes the stdio server configuration, including `GITLAB_URL`, `GITLAB_TOKEN`, TLS, catalog, safety, upload, rate-limit, and auto-update options, to `~/.gitlab-mcp-server.env` (with `0600` permissions on Unix). Most client config files only contain non-secret launch preferences and references to that env file where supported. `GenerateEntry(ClientJetBrains, ...)` is the compatibility exception: JetBrains cannot reference the shared env file, so the display-only JSON snippet includes the full env map, including secrets. Prefer an auto-written client config when possible; if you use the JetBrains snippet, store it with local-only permissions and rotate the token if the snippet is exposed.

---

## MCP Client Configuration

For per-client setup instructions (VS Code, Claude Desktop, Cursor, Claude Code, Windsurf, JetBrains, Zed, Kiro), see [Getting Started](getting-started.md).

For HTTP mode (remote/multi-user), see [HTTP Server Mode](http-server-mode.md).

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
        "TOOL_SURFACE": "meta"
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
TOOL_SURFACE=meta
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

See [Security](security.md) for additional token management best practices.

---

## Server Administration

These settings are for operators deploying the server for a team or managing advanced behaviors. Most users can skip this section entirely.

### Advanced Variables

This table summarizes the most common operational variables. For the complete source-of-truth list, see [Environment Variable Reference](env-reference.md); for HTTP flags, see [CLI Reference](cli-reference.md).

| Variable               | Default                      | Description                                                                                             |
| ---------------------- | ---------------------------- | ------------------------------------------------------------------------------------------------------- |
| `AUTO_UPDATE`          | `true`                       | Enable automatic binary updates (`true`/`check`/`false`)                                                |
| `AUTO_UPDATE_REPO`     | `jmrplens/gitlab-mcp-server` | GitHub repository for release assets                                                                    |
| `AUTO_UPDATE_INTERVAL` | `1h`                         | Interval between periodic update checks                                                                 |
| `YOLO_MODE`            | `false`                      | Skip destructive action confirmation prompts                                                            |
| `AUTOPILOT`            | `false`                      | Same as `YOLO_MODE` — skip confirmation prompts                                                         |
| `AUTH_MODE`            | `legacy`                     | HTTP mode authentication: `legacy` (per-request header) or `oauth` (RFC 9728 Bearer token verification) |
| `OAUTH_CACHE_TTL`      | `15m`                        | TTL for verified OAuth token identity cache (min 1m, max 2h)                                            |
| `RATE_LIMIT_RPS`       | `0`                          | Per-server tools/call rate limit in requests/second (`0` = disabled)                                    |
| `RATE_LIMIT_BURST`     | `40`                         | Token-bucket burst size when `RATE_LIMIT_RPS` > 0                                                       |

See [Auto-Update](auto-update.md) for detailed documentation on update modes, MCP tools, release requirements, and troubleshooting.

### Tool Modes

| Mode                          | Variable                  | Tools Exposed                                                      | Best For                                                                      |
| ----------------------------- | ------------------------- | ------------------------------------------------------------------ | ----------------------------------------------------------------------------- |
| **Dynamic toolset** (default) | `TOOL_SURFACE=dynamic`    | `gitlab_find_action`, `gitlab_execute_action`                      | Most users — lowest startup context while retaining full catalog reachability |
| **Meta-tools**                | `TOOL_SURFACE=meta`       | 33 base / 49 self-managed enterprise / 50 GitLab.com Enterprise    | Clients that prefer consolidated domain dispatchers with `action` parameters  |
| **Individual tools**          | `TOOL_SURFACE=individual` | 867 CE / 1027 self-managed enterprise / 1033 GitLab.com Enterprise | Clients that need granular tool selection                                     |

Use the default dynamic surface for normal low-token deployments. Set `TOOL_SURFACE=meta` only when a client or workflow prefers domain meta-tools. `META_TOOLS` remains accepted for compatibility only and should appear only in migration guidance.

See [Meta-Tools](meta-tools.md) for the complete domain-action mapping and [Dynamic Toolset](dynamic-tools.md) for the low-token find/execute workflow.

### Meta Parameter Schema

`META_PARAM_SCHEMA` controls only the visible `inputSchema` of meta-tool dispatchers in `tools/list`. It does not change handler validation, execution, dynamic find output, or the `gitlab://tools` manifest contents.

| Tool surface | Visible tool schema impact                                                                                                           | Tool manifest availability                                    | Dynamic describe behavior                                        | Token impact                                                   | Recommended use                                      |
| ------------ | ------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------- | ---------------------------------------------------------------- | -------------------------------------------------------------- | ---------------------------------------------------- |
| `meta`       | Applies to every visible domain meta-tool. `opaque` shows `{action, params}`; `compact` and `full` inline per-action `oneOf` schemas | `gitlab://tools` and `gitlab://tools/{id}` in full or minimal | Not applicable                                                   | `full` is 11.9x larger than `opaque`; `compact` is 6.5x larger | Keep `opaque`; use `gitlab://tools` for exact params |
| `dynamic`    | Does not change the two dynamic tool schemas                                                                                         | `gitlab://tools` and `gitlab://tools/{id}` in full or minimal | `gitlab_find_action` returns discovery and schema details inline | No practical startup benefit for Dynamic tool schemas          | Keep `opaque`; use find or `gitlab://tools`          |
| `individual` | Ignored because individual tools expose one operation per tool with direct typed schemas                                             | `gitlab://tools` and `gitlab://tools/{id}` in full or minimal | Not applicable                                                   | None                                                           | Leave unset                                          |

The evaluated modes remain `opaque`, `compact`, and `full`. The setting name remains valid for the final architecture because it describes the meta-tool dispatcher envelope, while the action catalog remains the source of the underlying per-action schemas.

### Capability Surface

`CAPABILITY_SURFACE=full` is the default and preserves the existing MCP resources and prompts catalog. `CAPABILITY_SURFACE=minimal` is a non-default low-token mode: it keeps `gitlab://workspace/roots` for project discovery plus `gitlab://tools` for surface-aware action discovery, and omits static GitLab data resources, workflow guide resources, and prompt templates. Dynamic execution still works without reading resources because `gitlab_find_action` returns exact action schemas inline.

Measured startup context is the reason this setting keeps only two modes for now: full resources plus prompts cost about 18.2k tokens, while minimal keeps the shared capability overhead low by advertising only roots plus the unified tool manifest. Candidate intermediate modes such as `schemas`, `resources`, or `docs` would add another configuration axis without beating the existing low-token workflows. Reconsider an intermediate mode only if future audits show a concrete client that needs more resources but cannot tolerate prompts or static resources.

### HTTP Server Mode

When running the server for multiple users, use HTTP mode. Configuration comes from CLI flags instead of environment variables:

| Flag                     | Default                      | Description                                                                                                                                                                         |
| ------------------------ | ---------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `--http`                 | *(off)*                      | Enable HTTP transport mode                                                                                                                                                          |
| `--http-addr`            | `:8080`                      | HTTP listen address                                                                                                                                                                 |
| `--gitlab-url`           | *(optional)*                 | Fixed GitLab instance URL. Omit it to require each client to send `GITLAB-URL` per request                                                                                          |
| `--skip-tls-verify`      | `false`                      | Skip TLS certificate verification                                                                                                                                                   |
| `--tool-surface`         | `dynamic`                    | Canonical tool catalog selector: `dynamic`, `meta`, or `individual`                                                                                                                 |
| `--meta-tools`           | *(unset)*                    | Deprecated compatibility flag. Use `--tool-surface=individual` instead of `--meta-tools=false`                                                                                      |
| `--capability-surface`   | `full`                       | Resource and prompt catalog selector: `full` or `minimal`                                                                                                                           |
| `--meta-param-schema`    | `opaque`                     | Meta-tool input-schema strategy: `opaque`, `compact`, or `full`                                                                                                                     |
| `--enterprise`           | `false`                      | Force the Enterprise/Premium tool catalog when explicitly set. When omitted, HTTP mode auto-detects CE/EE per token+URL pool entry when GitLab reports edition in `/api/v4/version` |
| `--max-http-clients`     | `100`                        | Maximum concurrent client sessions                                                                                                                                                  |
| `--session-timeout`      | `30m`                        | Idle session timeout                                                                                                                                                                |
| `--auth-mode`            | `legacy`                     | Authentication mode: `legacy` (per-request header) or `oauth` (RFC 9728 Bearer token verification)                                                                                  |
| `--oauth-cache-ttl`      | `15m`                        | TTL for verified OAuth token cache (1m–2h)                                                                                                                                          |
| `--revalidate-interval`  | `15m`                        | Interval for OAuth token re-validation against GitLab (`0` disables; upper bound 24h)                                                                                               |
| `--trusted-proxy-header` | *(empty)*                    | Header containing the real client IP when behind a reverse proxy (e.g. `Fly-Client-IP`, `X-Real-IP`, `X-Forwarded-For`). Used by the per-IP auth rate limiter                       |
| `--auto-update`          | `true`                       | Enable automatic binary updates                                                                                                                                                     |
| `--auto-update-repo`     | `jmrplens/gitlab-mcp-server` | GitHub repository for release assets                                                                                                                                                |
| `--auto-update-interval` | `1h`                         | Interval between periodic update checks                                                                                                                                             |
| `--auto-update-timeout`  | `60s`                        | Timeout for startup/background update checks (range: 5s–10m)                                                                                                                        |
| `--read-only`            | `false`                      | Expose only read-only tools                                                                                                                                                         |
| `--safe-mode`            | `false`                      | Intercept mutating tools, return preview                                                                                                                                            |
| `--embedded-resources`   | `true`                       | Embed canonical `gitlab://` MCP resource URIs as `EmbeddedResource` content blocks in `gitlab_*_get` tool results                                                                   |
| `--rate-limit-rps`       | `0`                          | Per-server tools/call rate limit in req/s (`0` = disabled)                                                                                                                          |
| `--rate-limit-burst`     | `40`                         | Token-bucket burst size when `--rate-limit-rps` > 0                                                                                                                                 |
| `--exclude-tools`        | *(empty)*                    | Comma-separated tool names to exclude                                                                                                                                               |
| `--ignore-scopes`        | `false`                      | Skip PAT scope detection                                                                                                                                                            |

No `GITLAB_TOKEN` is needed at startup — each client provides its own token per-request via `PRIVATE-TOKEN` header or `Authorization: Bearer`. Clients can specify a `GITLAB-URL` header only when the server starts without `--gitlab-url`; when `--gitlab-url` is configured, it is authoritative and client-provided `GITLAB-URL` values are ignored and logged.

### OAuth Mode Configuration

To enable server-side token verification, set `--auth-mode=oauth`:

```bash
gitlab-mcp-server --http \
  --gitlab-url=https://gitlab.com \
  --auth-mode=oauth \
  --oauth-cache-ttl=15m
```

Replace `https://gitlab.com` with your self-managed GitLab URL when needed.

With OAuth mode:

- All tokens are verified against GitLab's `/api/v4/user` endpoint before reaching the MCP handler
- Verified tokens are cached (SHA-256 hashed) for `--oauth-cache-ttl` duration (default 15m, range 1m–2h)
- An RFC 9728 metadata endpoint is served at `/.well-known/oauth-protected-resource`, enabling MCP clients with OAuth 2.1 support to discover the GitLab authorization server automatically
- `PRIVATE-TOKEN` headers are auto-converted to `Authorization: Bearer` for compatibility

For a complete guide on creating GitLab OAuth applications for your MCP clients, see [OAuth App Setup](oauth-app-setup.md).

See [HTTP Server Mode](http-server-mode.md) for architecture and deployment details.

## Automatic Behaviors

These features are always active and require no configuration:

| Feature                 | Description                                                                                                                                                                                                                                       |
| ----------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Content annotations** | All Markdown content is annotated with `audience` and `priority` — `ContentList` (priority 0.4), `ContentDetail` (0.6), `ContentMutate` (0.8). This helps MCP clients optimize display and prevents raw Markdown from duplicating the JSON output |
| **Clickable links**     | List results in 14 domains include `[text](url)` links to GitLab entities (MRs, issues, pipelines, etc.)                                                                                                                                          |
| **Next-step hints**     | Every list/detail/mutation response includes `💡 Next steps` suggestions. In meta-tool mode, these are also injected into the JSON `structuredContent` as a `next_steps` array                                                                     |
| **Formatted dates**     | All timestamps are displayed in readable format (`2025-01-15 10:30`) instead of raw ISO 8601                                                                                                                                                      |

See [Output Format](output-format.md) for details.

## Configuration Loading

Configuration is loaded by `internal/config/` in this order:

1. `.env` file in the current directory (if present) via `godotenv`
2. `~/.gitlab-mcp-server.env` in the user's home directory (fallback for wizard-managed secrets)
3. Environment variables (override both `.env` files)
4. Command-line flags (`--http`, `--http-addr`)

> **Note**: `godotenv` does not overwrite existing variables, so values from step 1 take precedence over step 2, and explicit environment variables (step 3) override both.

The `config.Load()` function validates that `GITLAB_TOKEN` is set and defaults `GITLAB_URL` to `https://gitlab.com` when it is omitted (stdio mode only). In HTTP mode, configuration comes from CLI flags and no token is required at startup — each client provides its own token per-request via `PRIVATE-TOKEN` or `Authorization: Bearer` headers. Clients can provide `GITLAB-URL` only in multi-instance mode, when the server starts without `--gitlab-url`; all other MCP server settings are process policy and cannot be overridden per request. When `--auth-mode=oauth`, the server validates tokens against the GitLab `/api/v4/user` endpoint and caches verified identities with a configurable TTL (see [HTTP Server Mode — OAuth Mode](http-server-mode.md#oauth-mode)).
