# Getting Started

A step-by-step tutorial to get gitlab-mcp-server running and make your first GitLab query through an AI assistant.

> **Diátaxis type**: Tutorial
> **Audience**: New users
> **Time**: ~5 minutes
> 📖 **User documentation**: See the [Getting Started](https://jmrplens.github.io/gitlab-mcp-server/getting-started/) on the documentation site for a user-friendly version.

---

## Prerequisites

- A GitLab instance (self-hosted or gitlab.com)
- A Personal Access Token with `api` scope ([how to create one](https://docs.gitlab.com/ee/user/profile/personal_access_tokens.html))
- An MCP-compatible AI client (VS Code with GitHub Copilot, Claude Desktop, Cursor, etc.)

> **Headless / CI usage?** See the [CI/CD Usage](ci-cd.md) guide for running in pipelines without an interactive client.

---

## Step 1: Download the Binary

Download the latest release for your platform from the project [Releases](../../releases) page:

| Platform            | Binary                            |
| ------------------- | --------------------------------- |
| Linux amd64         | `gitlab-mcp-server-linux-amd64`       |
| Linux arm64         | `gitlab-mcp-server-linux-arm64`       |
| Windows amd64       | `gitlab-mcp-server-windows-amd64.exe` |
| Windows arm64       | `gitlab-mcp-server-windows-arm64.exe` |
| macOS Intel         | `gitlab-mcp-server-darwin-amd64`      |
| macOS Apple Silicon | `gitlab-mcp-server-darwin-arm64`      |

On Linux/macOS, make it executable:

```bash
chmod +x gitlab-mcp-server-linux-amd64
```

---

## Step 2: Run the Setup Wizard

The easiest way to configure everything is the built-in Setup Wizard. It configures your GitLab connection and writes the MCP client config files in one step.

### Windows

Double-click the `.exe` file — the wizard opens automatically in your browser.

Or from a terminal:

```powershell
.\gitlab-mcp-server-windows-amd64.exe --setup
```

### Linux / macOS

```bash
./gitlab-mcp-server-linux-amd64 --setup
```

The wizard auto-detects the best UI: **Web** (browser) → **TUI** (terminal) → **CLI** (plain text). You can force a specific mode:

```bash
gitlab-mcp-server --setup --setup-mode web   # Browser UI
gitlab-mcp-server --setup --setup-mode tui   # Terminal UI
gitlab-mcp-server --setup --setup-mode cli   # Plain text
```

The wizard will ask for:

1. **GitLab URL** — your instance base URL (e.g., `https://gitlab.example.com`)
2. **Personal Access Token** — a `glpat-...` token with `api` scope
3. **MCP client** — which AI client(s) to configure (VS Code, Claude Desktop, Cursor, etc.)

It then writes:

- `~/.gitlab-mcp-server.env` — your credentials (secure permissions, never in client config)
- Client-specific config files (e.g., `.vscode/mcp.json`, `claude_desktop_config.json`)

> **Skip the wizard?** See [Manual Configuration](#alternative-manual-configuration) below.

---

## Step 3: Open Your AI Client

Open your configured AI client (e.g., VS Code with GitHub Copilot). The MCP server starts automatically when the client connects.

You should see the GitLab MCP server listed in your client's MCP server panel. In VS Code, check the **MCP Servers** section in the Copilot chat sidebar.

---

## Step 4: Make Your First Query

Type a natural language request in the AI chat:

> **"List my GitLab projects"**

The AI assistant calls the `gitlab_project` meta-tool (or `gitlab_project_list` in individual mode) and returns a formatted list of your projects with names, URLs, and descriptions.

### Expected Output

The response includes a Markdown table like:

```text
| # | Project | Visibility | Stars |
|---|---------|------------|-------|
| 1 | [my-app](https://gitlab.example.com/user/my-app) | private | 3 |
| 2 | [api-service](https://gitlab.example.com/user/api-service) | internal | 1 |
```

Plus structured JSON data for the AI to process programmatically.

---

## Step 5: Try More Operations

Here are some things to try next:

**Browse a project:**

> "Show me the branches in project my-app"

**Check merge requests:**

> "List open merge requests in project 42"

**Read a file:**

> "Show me the contents of README.md in project my-app"

**Create an issue:**

> "Create an issue in project my-app titled 'Fix login bug' with label 'bug'"

**Pipeline status:**

> "What's the latest pipeline status for project my-app?"

The server handles all GitLab API calls. You do not need to know project IDs, endpoints, or JSON syntax — the AI figures that out.

---

## Tool Modes

By default, the server registers **32 meta-tools** (47 with `GITLAB_ENTERPRISE=true`) — domain-grouped dispatchers that reduce token overhead. Each meta-tool handles multiple actions via an `action` parameter.

To register all **1006 individual tools** instead (one per GitLab operation), set:

```env
META_TOOLS=false
```

See [Meta-Tools](meta-tools.md) for the full reference.

---

## Alternative: Open Plugins (Cursor / Claude Code)

The repository ships an [Open Plugins](https://open-plugins.com/) v1.0.0 manifest (`.plugin/plugin.json` + `.mcp.json`) so the server can be installed in a single step on conformant hosts:

```bash
/plugin install jmrplens/gitlab-mcp-server
```

The bundled `.mcp.json` runs the published Docker image `ghcr.io/jmrplens/gitlab-mcp-server:latest`, so [Docker](https://docs.docker.com/get-docker/) must be installed. The host passes these environment variables through to the container:

| Variable                 | Required | Description                                              |
| ------------------------ | -------- | -------------------------------------------------------- |
| `GITLAB_URL`             | Yes      | GitLab instance URL                                      |
| `GITLAB_TOKEN`           | Yes      | Personal Access Token                                    |
| `GITLAB_SKIP_TLS_VERIFY` | No       | `true` for self-signed certs (default `false`)           |
| `META_TOOLS`             | No       | Group tools per domain (default `true`)                  |
| `GITLAB_ENTERPRISE`      | No       | Enable Premium/Ultimate tools (default `false`)          |
| `GITLAB_READ_ONLY`       | No       | Disable mutating tools (default `false`)                 |
| `GITLAB_SAFE_MODE`       | No       | Preview mutating tool inputs (default `false`)           |
| `LOG_LEVEL`              | No       | `debug`, `info`, `warn`, `error` (default `info`)        |

The Open Plugins spec starts every entry in `.mcp.json` automatically and does not support runtime variants, so the manifest ships with a single Docker entry. To use the native binary instead, edit the local copy of `.mcp.json` (typically under `.agents/plugins/gitlab-mcp-server/`) after installation and replace `command` / `args` with the path to the binary downloaded from [GitHub Releases](https://github.com/jmrplens/gitlab-mcp-server/releases/latest).

---

## Alternative: Manual Configuration

If you prefer not to use the wizard, create a `.env` file next to the binary:

```env
GITLAB_URL=https://gitlab.example.com
GITLAB_TOKEN=glpat-xxxxxxxxxxxxxxxxxxxx
```

Then add the server to your MCP client config manually.

### VS Code / GitHub Copilot

Add to `.vscode/mcp.json` in your project:

```json
{
  "servers": {
    "gitlab": {
      "type": "stdio",
      "command": "/path/to/gitlab-mcp-server",
      "env": {
        "GITLAB_URL": "https://gitlab.example.com",
        "GITLAB_TOKEN": "glpat-xxxxxxxxxxxxxxxxxxxx"
      }
    }
  }
}
```

### Claude Desktop

Add to `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "gitlab": {
      "command": "/path/to/gitlab-mcp-server",
      "env": {
        "GITLAB_URL": "https://gitlab.example.com",
        "GITLAB_TOKEN": "glpat-xxxxxxxxxxxxxxxxxxxx"
      }
    }
  }
}
```

### Cursor

Add to `.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "gitlab": {
      "command": "/path/to/gitlab-mcp-server",
      "env": {
        "GITLAB_URL": "https://gitlab.example.com",
        "GITLAB_TOKEN": "glpat-xxxxxxxxxxxxxxxxxxxx"
      }
    }
  }
}
```

> See [Configuration](configuration.md) for all supported clients and HTTP mode setup.

---

## HTTP Mode (Team Deployment)

For shared server deployments, run in HTTP mode:

```bash
gitlab-mcp-server --http --gitlab-url=https://gitlab.example.com --http-addr=:8080
```

Each client provides its own token via HTTP header:

```json
{
  "servers": {
    "gitlab": {
      "type": "http",
      "url": "http://your-server:8080/mcp",
      "headers": {
        "PRIVATE-TOKEN": "glpat-your-token"
      }
    }
  }
}
```

### OAuth Mode (Recommended for Production)

For production deployments, enable server-side token verification with OAuth mode:

```bash
gitlab-mcp-server --http \
  --gitlab-url=https://gitlab.example.com \
  --auth-mode=oauth \
  --oauth-cache-ttl=15m
```

OAuth mode validates every Bearer token against GitLab's `/api/v4/user` endpoint and serves an RFC 9728 metadata endpoint that allows MCP clients with OAuth 2.1 support to discover the GitLab authorization server automatically.

MCP clients that support OAuth can connect by providing the `clientId` from the GitLab OAuth Application:

```json
{
  "servers": {
    "gitlab": {
      "type": "http",
      "url": "http://your-server:8080/mcp",
      "oauth": {
        "clientId": "YOUR_GITLAB_APPLICATION_ID",
        "scopes": ["api"]
      }
    }
  }
}
```

The client discovers the GitLab authorization server via `/.well-known/oauth-protected-resource` and handles token acquisition through the standard OAuth 2.1 PKCE flow.

> **Important**: Without `clientId`, clients fall back to Dynamic Client Registration (DCR). GitLab's DCR assigns the `mcp` scope instead of `api`, causing most operations to fail. Always configure `clientId` explicitly.
>
> **Prerequisite**: A GitLab OAuth Application must be created. See [OAuth App Setup](oauth-app-setup.md) for a step-by-step guide and [IDE Configuration](ide-configuration.md) for per-client examples.

See [HTTP Server Mode](http-server-mode.md) for the full architecture and deployment details.

---

## Next Steps

- [Configuration](configuration.md) — all environment variables and client setup options
- [Meta-Tools](meta-tools.md) — domain meta-tool reference with action mappings
- [Usage Examples](examples/usage-examples.md) — real-world scenarios
- [Tools Reference](tools/README.md) — all 1006 individual tools
- [Troubleshooting](troubleshooting.md) — common issues and solutions
