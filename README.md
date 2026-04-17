# GitLab MCP Server

A **Model Context Protocol (MCP) server** that exposes GitLab operations as MCP tools, resources, and prompts for AI assistants. Written in Go — single static binary per platform.

## Highlights

- **1004 MCP tools** — complete GitLab REST API v4 coverage across 162 domain sub-packages: projects, branches, tags, releases, merge requests, issues, pipelines, jobs, groups, users, wikis, environments, deployments, packages, container registry, runners, feature flags, CI/CD variables, templates, admin settings, access tokens, deploy keys, and more
- **42 meta-tools** (57 with `GITLAB_ENTERPRISE=true`) — domain-grouped dispatchers that reduce token overhead for LLMs (optional, enabled by default). 15 additional enterprise meta-tools available for Premium/Ultimate features
- **11 sampling tools** — LLM-assisted code review, issue analysis, pipeline failure diagnosis, security review, release notes, milestone reports, and more via MCP sampling capability
- **4 elicitation tools** — interactive creation wizards (issue, MR, release, project) with step-by-step user prompts
- **24 MCP resources** — read-only data: user, groups, group members, group projects, projects, issues, pipelines, members, labels, milestones, branches, MRs, releases, tags, workspace roots, and 5 workflow best-practice guides
- **38 MCP prompts** — AI-optimized: code review, pipeline status, risk assessment, release notes, standup, workload, user stats, team management, cross-project dashboards, analytics, milestones, audit
- **6 MCP capabilities** — logging, completions, roots, progress, sampling, elicitation
- **43 tool icons** — SVG data-URI icons on all tools, resources, and prompts for visual identification in MCP clients
- **Pagination** on all list endpoints with metadata (total items, pages, next/prev)
- **Transports**: stdio (default for desktop AI) and HTTP (Streamable HTTP for remote clients)
- **Cross-platform**: Windows, Linux & macOS, amd64 & arm64
- **Self-hosted GitLab** with self-signed TLS certificate support

## Example Prompts

Once connected, just talk to your AI assistant in natural language:

> "List my GitLab projects"
> "Show me open merge requests in my-app"
> "Create a merge request from feature-login to main"
> "Review merge request !15 — is it safe to merge?"
> "List open issues assigned to me"
> "What's the pipeline status for project 42?"
> "Why did the last pipeline fail?"
> "Generate release notes from v1.0 to v2.0"

The server handles the translation from natural language to GitLab API calls. You do not need to know project IDs, API endpoints, or JSON syntax — the AI assistant figures that out for you. See [Usage Examples](docs/examples/usage-examples.md) for more scenarios.

## Quick Start

### 1. Download

Download the latest binary for your platform from [GitHub Releases](../../releases) and make it executable:

```bash
chmod +x gitlab-mcp-server-*  # Linux/macOS only
```

### 2. Configure your MCP client

**Recommended**: Run the built-in setup wizard — it configures your GitLab connection and MCP client in one step:

```bash
./gitlab-mcp-server --setup
```

> **Tip**: The wizard supports Web UI, Terminal UI, and plain CLI modes. On Windows, double-click the `.exe` to launch the wizard automatically.

**Or configure manually** — expand your client below:

<details>
<summary><strong>VS Code (GitHub Copilot)</strong></summary>

Add to `.vscode/mcp.json` in your workspace:

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

</details>

<details>
<summary><strong>Claude Desktop</strong></summary>

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

</details>

<details>
<summary><strong>Cursor</strong></summary>

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

</details>

<details>
<summary><strong>Claude Code</strong></summary>

```bash
claude mcp add gitlab /path/to/gitlab-mcp-server \
  -e GITLAB_URL=https://gitlab.example.com \
  -e GITLAB_TOKEN=glpat-xxxxxxxxxxxxxxxxxxxx
```

</details>

<details>
<summary><strong>Windsurf</strong></summary>

Add to `~/.codeium/windsurf/mcp_config.json`:

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

</details>

<details>
<summary><strong>JetBrains IDEs</strong></summary>

Add to the MCP configuration in **Settings → Tools → AI Assistant → MCP Servers**:

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

</details>

<details>
<summary><strong>Zed</strong></summary>

Add to Zed settings (`settings.json`):

```json
{
  "context_servers": {
    "gitlab": {
      "command": "/path/to/gitlab-mcp-server",
      "args": [],
      "env": {
        "GITLAB_URL": "https://gitlab.example.com",
        "GITLAB_TOKEN": "glpat-xxxxxxxxxxxxxxxxxxxx"
      }
    }
  }
}
```

</details>

<details>
<summary><strong>Kiro</strong></summary>

Add to `.kiro/settings/mcp.json`:

```json
{
  "mcpServers": {
    "gitlab": {
      "command": "/path/to/gitlab-mcp-server",
      "args": [],
      "env": {
        "GITLAB_URL": "https://gitlab.example.com",
        "GITLAB_TOKEN": "glpat-xxxxxxxxxxxxxxxxxxxx"
      }
    }
  }
}
```

</details>

### 3. Verify

Open your AI client and try:

> _"List my GitLab projects"_

See the [Getting Started guide](https://jmrplens.github.io/gitlab-mcp-server/getting-started/) for detailed setup instructions.

## Tool Modes

Two registration modes, controlled by the `META_TOOLS` environment variable:

| Mode | Tools | Description |
|------|-------|-------------|
| **Meta-Tools** (default) | 42 base / 57 enterprise | Domain-grouped dispatchers with `action` parameter. Lower token usage. |
| **Individual** | 1004 | Every GitLab operation as a separate MCP tool. |

Meta-tool summary:

| Meta-Tool | Domain |
|-----------|--------|
| `gitlab_project` | Projects, uploads, hooks, badges, boards, import/export, pages |
| `gitlab_branch` | Branches, protected branches |
| `gitlab_tag` | Tags, protected tags |
| `gitlab_release` | Releases, release links |
| `gitlab_merge_request` | MR CRUD, approvals, context-commits |
| `gitlab_mr_review` | MR notes, discussions, drafts, changes |
| `gitlab_repository` | Repository tree/compare, commit discussions, files |
| `gitlab_group` | Groups, members, labels, milestones, boards, uploads |
| `gitlab_issue` | Issues, notes, discussions, links, statistics, emoji, events |
| `gitlab_pipeline` | Pipelines, pipeline triggers |
| `gitlab_job` | Jobs, job token scope |
| `gitlab_user` | Users, events, notifications, keys, namespaces |
| `gitlab_wiki` | Project/group wikis |
| `gitlab_environment` | Environments, protected envs, freeze periods |
| `gitlab_deployment` | Deployments |
| `gitlab_ci_variable` | CI/CD variables (project, group, instance) |
| `gitlab_search` | Global, project, group search |

Plus 11 sampling tools, 4 elicitation tools, and additional domain tools. See [Meta-Tools Reference](docs/meta-tools.md) for the complete list with actions and examples.

## Compatibility

| MCP Capability | Support |
|----------------|---------|
| **Tools** | 1004 individual / 42–57 meta |
| **Resources** | 24 (static + templates) |
| **Prompts** | 38 templates |
| **Completions** | Project, user, group, branch, tag |
| **Logging** | Structured (text/JSON) + MCP notifications |
| **Progress** | Tool execution progress reporting |
| **Sampling** | 11 LLM-powered analysis tools |
| **Elicitation** | 4 interactive creation wizards |
| **Roots** | Workspace root tracking |

Tested with: VS Code + GitHub Copilot, Claude Desktop, Claude Code, Cursor, Windsurf, JetBrains IDEs, Zed, Kiro.

See the full [Compatibility Matrix](https://jmrplens.github.io/gitlab-mcp-server/compatibility/) for detailed client support.

## Documentation

Full documentation is available at **[jmrplens.github.io/gitlab-mcp-server](https://jmrplens.github.io/gitlab-mcp-server/)**.

| Document | Description |
|----------|-------------|
| [Getting Started](docs/getting-started.md) | Download, setup wizard, per-client configuration |
| [Configuration](docs/configuration.md) | Environment variables, transport modes, TLS |
| [Tools Reference](docs/tools/README.md) | All 1004 individual tools with input/output schemas |
| [Meta-Tools](docs/meta-tools.md) | 42/57 domain meta-tools with action dispatching |
| [Resources](docs/resources-reference.md) | All 24 resources with URI templates |
| [Prompts](docs/prompts-reference.md) | All 38 prompts with arguments and output format |
| [Auto-Update](docs/auto-update.md) | Self-update mechanism, modes, and release format |
| [Security](docs/security.md) | Security model, token scopes, input validation |
| [Architecture](docs/architecture.md) | System architecture, component design, data flow |
| [Development Guide](docs/development/development.md) | Building, testing, CI/CD, contributing |

## Tech Stack

| Component | Technology |
|-----------|------------|
| Language | Go 1.26+ |
| MCP SDK | `github.com/modelcontextprotocol/go-sdk` v1.5.0 |
| GitLab Client | `gitlab.com/gitlab-org/api/client-go/v2` v2.17.0 |
| Transport | stdio (default), HTTP (Streamable HTTP) |

## Building from Source

```bash
git clone https://github.com/jmrplens/gitlab-mcp-server.git
cd gitlab-mcp-server
make build
```

See the [Development Guide](docs/development/development.md) for cross-compilation and contributing guidelines.

## Docker

```bash
docker pull ghcr.io/jmrplens/gitlab-mcp-server:latest

docker run -d --name gitlab-mcp-server -p 8080:8080 \
  -e GITLAB_URL=https://gitlab.example.com \
  -e GITLAB_SKIP_TLS_VERIFY=true \
  ghcr.io/jmrplens/gitlab-mcp-server:latest
```

Clients authenticate via `PRIVATE-TOKEN` or `Authorization: Bearer` headers. See [HTTP Server Mode](docs/http-server-mode.md) and [Docker documentation](docs/development/development.md#docker) for Docker Compose and configuration options.

## FAQ

<details>
<summary><strong>Does it work with self-hosted GitLab?</strong></summary>

Yes. Set `GITLAB_URL` to your instance URL. Self-signed TLS certificates are supported via `GITLAB_SKIP_TLS_VERIFY=true`.
</details>

<details>
<summary><strong>Is my data safe?</strong></summary>

The server runs locally on your machine (stdio mode) or on your own infrastructure (HTTP mode). No data is sent to third parties — all API calls go directly to your GitLab instance. See <a href="SECURITY.md">SECURITY.md</a> for details.
</details>

<details>
<summary><strong>Can I use it in read-only mode?</strong></summary>

Yes. Set `GITLAB_READ_ONLY=true` to disable all mutating tools (create, update, delete). Only read operations will be available.
</details>

<details>
<summary><strong>What GitLab editions are supported?</strong></summary>

Both Community Edition (CE) and Enterprise Edition (EE). Set `GITLAB_ENTERPRISE=true` to enable 15 additional tools for Premium/Ultimate features (DORA metrics, vulnerabilities, compliance, etc.).
</details>

<details>
<summary><strong>How does it handle rate limiting?</strong></summary>

The server includes retry logic with backoff for GitLab API rate limits. Errors are classified as transient (retryable) or permanent, with actionable hints in error messages.
</details>

<details>
<summary><strong>Which AI clients are supported?</strong></summary>

Any MCP-compatible client: VS Code + GitHub Copilot, Claude Desktop, Cursor, Claude Code, Windsurf, JetBrains IDEs, Zed, Kiro, and others. The built-in setup wizard can auto-configure most clients.
</details>

## Related Projects

- [Redmine MCP Server](https://github.com/jmrplens/redmine-mcp-server) — Sister project: MCP server for Redmine with 152 tools
- [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk) — The official Go SDK this server is built on
- [Model Context Protocol](https://modelcontextprotocol.io/) — The MCP specification
- [Awesome MCP Servers](https://github.com/punkpeye/awesome-mcp-servers) — Curated list of MCP servers

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development guidelines, branch naming, commit conventions, and pull request process.

## Security

See [SECURITY.md](SECURITY.md) for the security policy and vulnerability reporting.

## Code of Conduct

See [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md). This project follows the [Contributor Covenant v2.1](https://www.contributor-covenant.org/version/2/1/code_of_conduct/).
