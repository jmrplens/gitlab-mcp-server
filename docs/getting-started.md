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

> **Headless / CI usage?** See the [CI/CD Usage](guides/ci-cd.md) guide for running in pipelines without an interactive client.

---

## Fastest path

If you just want it running, pick one of these and skip the manual steps below.

### One-click buttons

Each registers a **Docker**-based server (auto-pulls the image on first run; needs [Docker](https://www.docker.com/)). VS Code prompts for your token; Cursor / LM Studio / Kiro add a `YOUR_GITLAB_TOKEN` placeholder you replace.

[![Install in VS Code](https://img.shields.io/badge/Install_in-VS_Code-0098FF?style=flat-square&logo=visualstudiocode&logoColor=white)](https://insiders.vscode.dev/redirect/mcp/install?name=gitlab&config=%7B%22command%22%3A%22docker%22%2C%22args%22%3A%5B%22run%22%2C%22-i%22%2C%22--rm%22%2C%22-e%22%2C%22GITLAB_TOKEN%22%2C%22ghcr.io%2Fjmrplens%2Fgitlab-mcp-server%3Alatest%22%2C%22--http%3Dfalse%22%5D%2C%22env%22%3A%7B%22GITLAB_TOKEN%22%3A%22%24%7Binput%3Agitlab_token%7D%22%7D%2C%22inputs%22%3A%5B%7B%22id%22%3A%22gitlab_token%22%2C%22type%22%3A%22promptString%22%2C%22description%22%3A%22GitLab%20Personal%20Access%20Token%20%28api%20scope%29%22%2C%22password%22%3Atrue%7D%5D%7D)
[![Install in Cursor](https://cursor.com/deeplink/mcp-install-dark.svg)](https://cursor.com/install-mcp?name=gitlab&config=eyJjb21tYW5kIjoiZG9ja2VyIiwiYXJncyI6WyJydW4iLCItaSIsIi0tcm0iLCItZSIsIkdJVExBQl9UT0tFTiIsImdoY3IuaW8vam1ycGxlbnMvZ2l0bGFiLW1jcC1zZXJ2ZXI6bGF0ZXN0IiwiLS1odHRwPWZhbHNlIl0sImVudiI6eyJHSVRMQUJfVE9LRU4iOiJZT1VSX0dJVExBQl9UT0tFTiJ9fQ%3D%3D)
[![Add to LM Studio](https://files.lmstudio.ai/deeplink/mcp-install-dark.svg)](https://lmstudio.ai/install-mcp?name=gitlab&config=eyJjb21tYW5kIjoiZG9ja2VyIiwiYXJncyI6WyJydW4iLCItaSIsIi0tcm0iLCItZSIsIkdJVExBQl9UT0tFTiIsImdoY3IuaW8vam1ycGxlbnMvZ2l0bGFiLW1jcC1zZXJ2ZXI6bGF0ZXN0IiwiLS1odHRwPWZhbHNlIl0sImVudiI6eyJHSVRMQUJfVE9LRU4iOiJZT1VSX0dJVExBQl9UT0tFTiJ9fQ%3D%3D)
[![Add to Kiro](https://kiro.dev/images/add-to-kiro.svg)](https://kiro.dev/launch/mcp/add?name=gitlab&config=%7B%22command%22%3A%22docker%22%2C%22args%22%3A%5B%22run%22%2C%22-i%22%2C%22--rm%22%2C%22-e%22%2C%22GITLAB_TOKEN%22%2C%22ghcr.io%2Fjmrplens%2Fgitlab-mcp-server%3Alatest%22%2C%22--http%3Dfalse%22%5D%2C%22env%22%3A%7B%22GITLAB_TOKEN%22%3A%22YOUR_GITLAB_TOKEN%22%7D%7D)

### Claude Code (`claude mcp add`)

Docker (no install — pulls the image on first run):

```bash
claude mcp add gitlab --env GITLAB_TOKEN=glpat-xxxx --transport stdio \
  -- docker run -i --rm -e GITLAB_TOKEN ghcr.io/jmrplens/gitlab-mcp-server:latest --http=false
```

### One-line installer (native binary)

```bash
# macOS/Linux (Homebrew)
brew install jmrplens/tap/gitlab-mcp-server
# Linux/macOS (script)
curl -fsSL https://raw.githubusercontent.com/jmrplens/gitlab-mcp-server/main/scripts/install.sh | sh
# Windows (winget)
winget install --id jmrplens.gitlab-mcp-server -e
# Windows (PowerShell)
irm https://raw.githubusercontent.com/jmrplens/gitlab-mcp-server/main/scripts/install.ps1 | iex

claude mcp add gitlab --env GITLAB_TOKEN=glpat-xxxx -- gitlab-mcp-server
```

### Claude Desktop one-click extension (.mcpb)

Claude Desktop users can skip all of the above: download
[`gitlab-mcp-server.mcpb`](https://github.com/jmrplens/gitlab-mcp-server/releases/latest/download/gitlab-mcp-server.mcpb)
and open it with Claude Desktop — see the
[Claude Desktop Extension guide](guides/claude-desktop-extension.md).

Self-managed GitLab? Add `--env GITLAB_URL=https://gitlab.example.com` (and `--env GITLAB_SKIP_TLS_VERIFY=true` for self-signed certs). Prefer a guided flow with no JSON to edit? Run `gitlab-mcp-server --setup` after installing the binary (see [Step 2](#step-2-run-the-setup-wizard)).

---

## Step 1: Download the Binary

> Prefer the fast paths above? Skip to [Step 3](#step-3-open-your-ai-client). This section is the manual walkthrough.

Download the latest release for your platform from the project [Releases](https://github.com/jmrplens/gitlab-mcp-server/releases) page:

| Platform            | Binary                                |
| ------------------- | ------------------------------------- |
| Linux amd64         | `gitlab-mcp-server-linux-amd64`       |
| Linux arm64         | `gitlab-mcp-server-linux-arm64`       |
| Windows amd64       | `gitlab-mcp-server-windows-amd64.exe` |
| Windows arm64       | `gitlab-mcp-server-windows-arm64.exe` |
| macOS Intel         | `gitlab-mcp-server-darwin-amd64`      |
| macOS Apple Silicon | `gitlab-mcp-server-darwin-arm64`      |
| macOS universal     | `gitlab-mcp-server-darwin-all`        |

On Linux/macOS, make it executable:

```bash
chmod +x gitlab-mcp-server-linux-amd64
```

---

## Step 2: Run the Setup Wizard

The easiest way to configure everything is the built-in Setup Wizard. It configures your GitLab connection and writes the MCP client config files in one step.

> **Stdio only**: The wizard configures the **stdio MCP server** only. For shared or remote HTTP deployments, use [HTTP Server Mode](guides/http-server-mode.md) instead.

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
gitlab-mcp-server --setup --setup-mode web   # Browser UI (best for first-time setup; has inline help tooltips)
gitlab-mcp-server --setup --setup-mode tui   # Terminal UI (keyboard-driven, Bubble Tea)
gitlab-mcp-server --setup --setup-mode cli   # Plain text prompts (headless / SSH)
```

The wizard will ask for:

1. **GitLab URL** — your instance base URL (e.g., `https://gitlab.example.com`)
2. **Personal Access Token** — a `glpat-...` token with `api` scope
3. **MCP client** — which AI client(s) to configure (VS Code, Claude Desktop, Cursor, etc.)
4. **Advanced options** — tool surface, log level, rate limits, auto-update, etc. The Web UI attaches an inline help tooltip to every advanced option explaining what the setting does and its default. The CLI asks once whether to configure them. In the TUI, press `Ctrl+O` on the GitLab step to open the advanced options step.

It then writes:

- `~/.gitlab-mcp-server.env` — your credentials (with `0600` Unix permissions, never in client config)
- Client-specific config files (e.g., `.vscode/mcp.json`, `claude_desktop_config.json`)

**Re-running the wizard**: If `~/.gitlab-mcp-server.env` already exists, every UI mode (Web, TUI, CLI) pre-loads the saved values. You can change only the fields you need — the token is masked, and leaving it blank keeps the stored one. Use this to add a new client, switch from `dynamic` to `meta`, or rotate the token without re-typing everything.

> **Skip the wizard?** See [Manual Configuration](#alternative-manual-configuration) below.

---

## Step 3: Open Your AI Client

Open your configured AI client (e.g., VS Code with GitHub Copilot). The MCP server starts automatically when the client connects.

You should see the GitLab MCP server listed in your client's MCP server panel. In VS Code, check the **MCP Servers** section in the Copilot chat sidebar.

---

## Step 4: Make Your First Query

Type a natural language request in the AI chat:

> **"List my GitLab projects"**

In the default dynamic mode, the AI assistant finds the project-list action with `gitlab_find_action`, executes it with `gitlab_execute_action`, and returns a formatted list of your projects with names, URLs, and descriptions. In meta-tool mode it calls `gitlab_project`; in individual mode it calls `gitlab_project_list`.

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

By default, the server registers the **dynamic find/execute surface**: `gitlab_find_action` and `gitlab_execute_action`. The same canonical GitLab action catalog remains reachable, and `gitlab_find_action` returns exact schemas before execution. Set `TOOL_SURFACE=meta` to use **32 meta-tools** (48 on self-managed Enterprise/Premium, 49 on GitLab.com Enterprise/Premium with Orbit).

To register the complete individual tool set instead (one tool per GitLab operation; up to 1067 on GitLab.com Enterprise/Premium), set:

```env
TOOL_SURFACE=individual
```

To switch away from the default dynamic surface and register the consolidated meta-tool catalog instead, set:

```env
TOOL_SURFACE=meta
```

For the smallest startup surface with the default dynamic mode, also set `CAPABILITY_SURFACE=minimal`. This keeps the `gitlab://tools` manifest, and omits optional GitLab data resources, prompts, and workflow guides. Dynamic action find and execute remain available because dynamic discovery returns action schemas inline.

See [Dynamic Tools](concepts/dynamic-tools.md) for the default find/execute workflow and [Meta-Tools](concepts/meta-tools.md) for the explicit meta-tool catalog reference.

---

## Alternative: Open Plugins (Cursor / Claude Code)

The repository ships an [Open Plugins](https://open-plugins.com/) v1.0.0 manifest (`.plugin/plugin.json` + `mcp.json`) so the server can be installed in a single step on conformant hosts:

```bash
/plugin install jmrplens/gitlab-mcp-server
```

The bundled `mcp.json` runs the published Docker image `ghcr.io/jmrplens/gitlab-mcp-server:latest`, so [Docker](https://docs.docker.com/get-docker/) must be installed. It is configured for stdio MCP clients and passes `--http=false` after the image name to override the Docker image's HTTP default. Keep that override if you copy the Docker configuration into VS Code or another stdio client; otherwise the container will start an HTTP listener and the client will wait forever for a stdio `initialize` response.

The host passes these environment variables through to the container:

| Variable                         | Required | Description                                                                                      |
| -------------------------------- | -------- | ------------------------------------------------------------------------------------------------ |
| `GITLAB_URL`                     | No       | GitLab instance URL. Defaults to `https://gitlab.com`; set for self-managed instances            |
| `GITLAB_TOKEN`                   | Yes      | Personal Access Token                                                                            |
| `GITLAB_SKIP_TLS_VERIFY`         | No       | `true` for self-signed certs (default `false`)                                                   |
| `TOOL_SURFACE`                   | No       | Canonical tool catalog selector: `dynamic`, `meta`, or `individual` (default `dynamic`)          |
| `CAPABILITY_SURFACE`             | No       | Resource and prompt catalog selector: `full` or `minimal` (default `full`)                       |
| `META_PARAM_SCHEMA`              | No       | Meta-tool input schema detail: `opaque`, `compact`, or `full` (default `opaque`)                 |
| `GITLAB_TIER`                    | No       | Licensing tier: `free`/`ce`, `premium`, `ultimate`; unset detects from license (fallback `free`) |
| `GITLAB_READ_ONLY`               | No       | Disable mutating tools (default `false`)                                                         |
| `GITLAB_SAFE_MODE`               | No       | Preview mutating tool inputs (default `false`)                                                   |
| `EMBEDDED_RESOURCES`             | No       | Append embedded MCP resource links to get-style tool results (default `true`)                    |
| `EXCLUDE_TOOLS`                  | No       | Comma-separated tool names to exclude from registration                                          |
| `GITLAB_IGNORE_SCOPES`           | No       | Skip token scope detection and register all tools (default `false`)                              |
| `UPLOAD_MAX_FILE_SIZE`           | No       | Maximum attachment upload size in bytes (default `2147483648`)                                   |
| `GITLAB_MCP_ALLOWED_IMPORT_DIRS` | No       | Extra path-list-separated directories allowed for local GitLab import archives                   |
| `RATE_LIMIT_RPS`                 | No       | Per-server tools/call rate limit in requests per second; `0` disables it (default `0`)           |
| `RATE_LIMIT_BURST`               | No       | Token-bucket burst size when `RATE_LIMIT_RPS` is greater than `0` (default `40`)                 |
| `AUTO_UPDATE`                    | No       | Auto-update mode in the container; default is `false` for Open Plugins installs                  |
| `AUTO_UPDATE_REPO`               | No       | GitHub repository slug for release assets (default `jmrplens/gitlab-mcp-server`)                 |
| `AUTO_UPDATE_INTERVAL`           | No       | Periodic update check interval in HTTP mode (default `1h`)                                       |
| `AUTO_UPDATE_TIMEOUT`            | No       | Startup/background update timeout (default `60s`)                                                |
| `LOG_LEVEL`                      | No       | `debug`, `info`, `warn`, `error` (default `info`)                                                |

Prefer `TOOL_SURFACE` for new configurations: `dynamic` is the default two-tool low-token find/execute surface, `meta` exposes consolidated domain dispatchers, and `individual` exposes every tool separately. The server still supports the deprecated `META_TOOLS` selector for compatibility, but the Open Plugins config forwards the explicit `TOOL_SURFACE` variable.

The Open Plugins spec starts every entry in the referenced MCP config automatically and does not support runtime variants, so the manifest ships with a single Docker stdio entry. To use the native binary instead, locate the installed `gitlab-mcp-server` plugin directory from your host's plugin UI or installation output, then edit its local `mcp.json` (commonly under `.agents/plugins/gitlab-mcp-server/`) and replace `command` / `args` with the path to the binary downloaded from [GitHub Releases](https://github.com/jmrplens/gitlab-mcp-server/releases/latest).

For detached HTTP deployments, do not use a stdio client entry. Run the Docker image in HTTP mode and configure the MCP client with `type: "http"` and a URL such as `http://localhost:8080/mcp`. See [HTTP Server Mode](guides/http-server-mode.md).

---

## Alternative: Manual Configuration

If you prefer not to use the wizard, create a `.env` file next to the binary:

```env
GITLAB_TOKEN=glpat-xxxxxxxxxxxxxxxxxxxx
```

For self-managed GitLab, add `GITLAB_URL=https://gitlab.example.com`.

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
        "GITLAB_TOKEN": "glpat-xxxxxxxxxxxxxxxxxxxx"
      }
    }
  }
}
```

> See [Configuration](reference/configuration.md) for all supported clients and HTTP mode setup.

---

## HTTP Mode (Team Deployment)

For shared server deployments, run in HTTP mode:

```bash
# Use https://gitlab.com for GitLab.com, or replace it with your self-managed URL.
gitlab-mcp-server --http --gitlab-url=https://gitlab.com --http-addr=:8080
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
  --gitlab-url=https://gitlab.com \
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
> **Prerequisite**: A GitLab OAuth Application must be created. See [OAuth App Setup](guides/oauth-app-setup.md) for a step-by-step guide and [IDE Configuration](guides/ide-configuration.md) for per-client examples.

See [HTTP Server Mode](guides/http-server-mode.md) for the full architecture and deployment details.

---

## Next Steps

- [Configuration](reference/configuration.md) — all environment variables and client setup options
- [Meta-Tools](concepts/meta-tools.md) — domain meta-tool reference with action mappings
- [Usage Examples](guides/examples/usage-examples.md) — real-world scenarios
- [Tools Reference](reference/tools/README.md) — all individual tools, including GitLab.com-only Orbit
- [Troubleshooting](guides/troubleshooting.md) — common issues and solutions
