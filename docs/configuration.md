# Configuration

gitlab-mcp-server is configured through environment variables. A `.env` file in the current directory is loaded automatically (via `godotenv`), and the server also loads `~/.gitlab-mcp-server.env` as a fallback for secrets written by the Setup Wizard.

> **Diataxis type**: Reference
> **Audience**: 👤🔧 All users
> **Prerequisites**: A running GitLab instance with a Personal Access Token
> 📖 **User documentation**: See the [Configuration](https://jmrplens.github.io/gitlab-mcp-server/configuration/) on the documentation site for a user-friendly version.

---

## Personal Setup

These are the settings every user needs to get started.

### Required Variables

| Variable | Description | Example |
| --- | --- | --- |
| `GITLAB_URL` | GitLab instance base URL | `https://gitlab.example.com` |
| `GITLAB_TOKEN` | Personal Access Token with `api` scope | `glpat-xxxxxxxxxxxxxxxxxxxx` |

### Common Options

| Variable | Default | Description |
| --- | --- | --- |
| `GITLAB_SKIP_TLS_VERIFY` | `false` | Skip TLS certificate verification for self-signed certs |
| `META_TOOLS` | `true` | Enable domain-level meta-tools (42 base / 57 enterprise instead of 1004) |
| `GITLAB_ENTERPRISE` | `false` | Enable Enterprise/Premium tools: gates 35 individual tool sub-packages and 15 dedicated meta-tools for GitLab Premium/Ultimate |
| `GITLAB_READ_ONLY` | `false` | Read-only mode: disables all mutating tools at startup |
| `LOG_LEVEL` | `info` | Logging verbosity: `debug`, `info`, `warn`, `error` |

### .env File Example

```env
GITLAB_URL=https://gitlab.example.com
GITLAB_TOKEN=glpat-xxxxxxxxxxxxxxxxxxxx
GITLAB_SKIP_TLS_VERIFY=true
META_TOOLS=true
GITLAB_READ_ONLY=false
LOG_LEVEL=info
```

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

**Secure secret storage**: The wizard writes `GITLAB_URL`, `GITLAB_TOKEN`, and `GITLAB_SKIP_TLS_VERIFY` to `~/.gitlab-mcp-server.env` (with `0600` permissions on Unix). Client config files only contain non-secret preferences like `META_TOOLS` and `LOG_LEVEL` — tokens never appear in JSON. VS Code additionally gets a native `envFile` reference for direct loading.

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
        "GITLAB_URL": "https://gitlab.example.com",
        "GITLAB_TOKEN": "${input:gitlab-token}",
        "META_TOOLS": "true"
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
GITLAB_URL=https://gitlab.example.com
GITLAB_TOKEN=glpat-xxxxxxxxxxxxxxxxxxxx
GITLAB_SKIP_TLS_VERIFY=true
META_TOOLS=true
```

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

| Variable | Default | Description |
| --- | --- | --- |
| `AUTO_UPDATE` | `true` | Enable automatic binary updates (`true`/`check`/`false`) |
| `AUTO_UPDATE_REPO` | `jmrplens/gitlab-mcp-server` | GitHub repository for release assets |
| `AUTO_UPDATE_INTERVAL` | `1h` | Interval between periodic update checks |
| `ISSUE_REPORTS` | `false` | Enable automatic issue report generation on unrecoverable errors |
| `YOLO_MODE` | `false` | Skip destructive action confirmation prompts |
| `AUTOPILOT` | `false` | Same as `YOLO_MODE` — skip confirmation prompts |

See [Auto-Update](auto-update.md) for detailed documentation on update modes, MCP tools, release requirements, and troubleshooting.

### Tool Modes

| Mode | Variable | Tools Exposed | Best For |
| --- | --- | --- | --- |
| **Meta-tools** (default) | `META_TOOLS=true` | 42 base / 57 enterprise | Most users — lower token usage |
| **Individual tools** | `META_TOOLS=false` | 1004 separate tools | Clients that need granular tool selection |

See [Meta-Tools](meta-tools.md) for the complete domain-action mapping.

### HTTP Server Mode

When running the server for multiple users, use HTTP mode. Configuration comes from CLI flags instead of environment variables:

| Flag | Default | Description |
| --- | --- | --- |
| `--http` | _(off)_ | Enable HTTP transport mode |
| `--http-addr` | `localhost:8080` | HTTP listen address |
| `--gitlab-url` | _(required)_ | GitLab instance URL |
| `--skip-tls-verify` | `false` | Skip TLS certificate verification |
| `--meta-tools` | `true` | Enable meta-tools |
| `--enterprise` | `false` | Enable Enterprise/Premium meta-tools (15 additional) |
| `--max-http-clients` | `100` | Maximum concurrent client sessions |
| `--session-timeout` | `30m` | Idle session timeout |
| `--auto-update` | `true` | Enable automatic binary updates |
| `--auto-update-repo` | `jmrplens/gitlab-mcp-server` | GitHub repository for release assets |
| `--auto-update-interval` | `1h` | Interval between periodic update checks |

No `GITLAB_TOKEN` is needed at startup — each client provides its own token per-request via `PRIVATE-TOKEN` header or `Authorization: Bearer`.

See [HTTP Server Mode](http-server-mode.md) for architecture and deployment details.

## Automatic Behaviors

These features are always active and require no configuration:

| Feature | Description |
| --- | --- |
| **Content annotations** | All Markdown content is annotated with `audience` and `priority` — `ContentList` (priority 0.4), `ContentDetail` (0.6), `ContentMutate` (0.8). This helps MCP clients optimize display and prevents raw Markdown from duplicating the JSON output |
| **Clickable links** | List results in 14 domains include `[text](url)` links to GitLab entities (MRs, issues, pipelines, etc.) |
| **Next-step hints** | Every list/detail/mutation response includes `💡 Next steps` suggestions. In meta-tool mode, these are also injected into the JSON `structuredContent` as a `next_steps` array |
| **Formatted dates** | All timestamps are displayed in readable format (`2025-01-15 10:30`) instead of raw ISO 8601 |

See [Output Format](output-format.md) for details.

## Configuration Loading

Configuration is loaded by `internal/config/` in this order:

1. `.env` file in the current directory (if present) via `godotenv`
2. `~/.gitlab-mcp-server.env` in the user's home directory (fallback for wizard-managed secrets)
3. Environment variables (override both `.env` files)
4. Command-line flags (`--http`, `--http-addr`)

> **Note**: `godotenv` does not overwrite existing variables, so values from step 1 take precedence over step 2, and explicit environment variables (step 3) override both.

The `config.Load()` function validates that `GITLAB_URL` and `GITLAB_TOKEN` are set (used by stdio mode only). In HTTP mode, configuration comes from CLI flags and no token is required at startup — each client provides its own token per-request via `PRIVATE-TOKEN` or `Authorization: Bearer` headers.
