<p align="center">
  <img alt="" src="site/src/assets/banner-dark.svg" width="840">
</p>

# GitLab MCP Server

<p align="center">

[![GitHub Release](https://img.shields.io/github/v/release/jmrplens/gitlab-mcp-server?style=flat&logo=github&label=Release)](https://github.com/jmrplens/gitlab-mcp-server/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/jmrplens/gitlab-mcp-server)](https://goreportcard.com/report/github.com/jmrplens/gitlab-mcp-server)
[![Go Reference](https://pkg.go.dev/badge/github.com/jmrplens/gitlab-mcp-server.svg)](https://pkg.go.dev/github.com/jmrplens/gitlab-mcp-server)
[![Glama MCP Score](https://glama.ai/mcp/servers/jmrplens/gitlab-mcp-server/badges/score.svg)](https://glama.ai/mcp/servers/jmrplens/gitlab-mcp-server)

</p>

<p align="center">

[![Quality Gate](https://sonarcloud.io/api/project_badges/measure?project=jmrplens_gitlab-mcp-server&metric=alert_status)](https://sonarcloud.io/summary/overall?id=jmrplens_gitlab-mcp-server)
[![Coverage](https://sonarcloud.io/api/project_badges/measure?project=jmrplens_gitlab-mcp-server&metric=coverage)](https://sonarcloud.io/summary/overall?id=jmrplens_gitlab-mcp-server)
![Platform](https://img.shields.io/badge/Windows%20%7C%20Linux%20%7C%20macOS-amd64%20%26%20arm64-lightgrey?style=flat&logo=windows-terminal&logoColor=white)

</p>

A **Model Context Protocol (MCP) server** that exposes the entire GitLab API as MCP tools, resources, and prompts for AI assistants. Single static binary — zero dependencies.

> **Security first**: Continuously monitored on [SonarCloud](https://sonarcloud.io/summary/overall?id=jmrplens_gitlab-mcp-server) with quality gates, coverage, and security scanning. Supports read-only mode, safe mode (dry-run preview), and self-hosted GitLab with TLS verification.

## Highlights

- **1006 MCP tools** — broad GitLab REST API v4 + GraphQL coverage across 162 domain sub-packages: projects, branches, tags, releases, merge requests, issues, pipelines, jobs, groups, users, wikis, environments, deployments, packages, container registry, runners, feature flags, CI/CD variables, templates, admin settings, access tokens, deploy keys, and more
- **32 meta-tools** (47 with `GITLAB_ENTERPRISE=true`) — domain-grouped dispatchers that reduce token overhead for LLMs (optional, enabled by default). 15 additional enterprise meta-tools available for Premium/Ultimate features
- **11 sampling actions** — LLM-assisted code review, issue analysis, pipeline failure diagnosis, security review, release notes, milestone reports, and more via `gitlab_analyze` meta-tool (MCP sampling capability)
- **4 elicitation tools** — interactive creation wizards (issue, MR, release, project) with step-by-step user prompts
- **44 MCP resources** — read-only data: user, groups, group members, group projects, projects, issues, pipelines, members, labels, milestones, branches, MRs, releases, tags, commits, file blobs, wiki pages, MR notes, MR discussions, single-entity templates (issue, MR, branch, tag, release, label, milestone, commit, wiki page, deployment, environment, job, board, snippet, deploy key, feature flag, group label, group milestone), workspace roots, and 5 workflow best-practice guides
- **38 MCP prompts** — AI-optimized: code review, pipeline status, risk assessment, release notes, standup, workload, user stats, team management, cross-project dashboards, analytics, milestones, audit
- **6 MCP capabilities** — logging, completions, roots, progress, sampling, elicitation
- **50 tool icons** — base64-encoded SVG icons (`Sizes: ["any"]`) on all tools, resources, and prompts for visual identification in MCP clients
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

<details>
<summary><strong>Cline</strong></summary>

Open the Cline sidebar in VS Code → click the MCP servers icon → **Edit Global MCP**, or edit `cline_mcp_settings.json` directly:

- **macOS**: `~/Library/Application Support/Code/User/globalStorage/saoudrizwan.claude-dev/settings/cline_mcp_settings.json`
- **Linux**: `~/.config/Code/User/globalStorage/saoudrizwan.claude-dev/settings/cline_mcp_settings.json`
- **Windows**: `%APPDATA%\Code\User\globalStorage\saoudrizwan.claude-dev\settings\cline_mcp_settings.json`

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
<summary><strong>Roo Code</strong></summary>

Open the Roo Code sidebar in VS Code → MCP servers icon → **Edit Global MCP** (or **Edit Project MCP** to create `.roo/mcp.json`):

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

### 3. Verify

Open your AI client and try:

> _"List my GitLab projects"_

See the [Getting Started guide](https://jmrplens.github.io/gitlab-mcp-server/getting-started/) for detailed setup instructions.

## Tool Modes

Two registration modes, controlled by the `META_TOOLS` environment variable:

| Mode | Tools | Description |
|------|-------|-------------|
| **Meta-Tools** (default) | 32 base / 47 enterprise | Domain-grouped dispatchers with `action` parameter. Lower token usage. |
| **Individual** | 1006 | Every GitLab operation as a separate MCP tool. |

Meta-tool summary:

<!-- START TOOLS -->

| Meta-Tool | Actions | Description |
|-----------|:-------:|-------------|
| `gitlab_access` | 48 | Manage GitLab access credentials: access tokens (project/group/personal), deploy tokens, deploy keys, access requests, and invitations. |
| `gitlab_admin` | 88 | GitLab self-managed instance administration: topics, settings, appearance, broadcast messages, instance feature flags, license, system hooks, Sidekiq metrics, plan limits, usage data, DB migrations, OAuth applications, custom attributes, bulk imports (GitLab→GitLab migrations), error tracking, alert metric images, secure files, Terraform states, cluster agents, dependency proxy cache, and external imports. |
| `gitlab_analyze` | 11 | LLM-assisted analysis of GitLab data via MCP sampling. |
| `gitlab_branch` | 11 | Manage Git branches and branch protections in a project, plus aggregated branch rules (GraphQL). |
| `gitlab_ci_catalog` | 2 | Discover and inspect CI/CD Catalog resources (reusable pipeline components and templates published by groups for import into .gitlab-ci.yml). |
| `gitlab_ci_variable` | 15 | Manage GitLab CI/CD variables at instance, group, and project scope. |
| `gitlab_custom_emoji` | 3 | Manage group-level custom emoji via GraphQL. |
| `gitlab_discover_project` | — | Resolve a git remote URL to a GitLab project and return its project_id and metadata. |
| `gitlab_environment` | 23 | Manage GitLab deployment environments, protected environments, freeze (deploy block) periods, and the deployment record audit trail. |
| `gitlab_feature_flags` | 10 | Manage project feature flags and feature-flag user lists for gradual rollouts. |
| `gitlab_group` | 130 | Manage GitLab groups: CRUD, subgroups, members, labels, milestones, webhooks, badges, boards, uploads, and import/export. |
| `gitlab_interactive_issue_create` | — | Interactively create a GitLab issue: step-by-step prompts collect title (required), description (optional multiline), comma-separated labels, and confidentiality (boolean), then ask for confirmation before calling the GitLab API. |
| `gitlab_interactive_mr_create` | — | Interactively create a GitLab merge request: step-by-step prompts collect source branch (required), target branch (required), title (required), description (optional multiline), comma-separated labels, squash-on-merge flag, and remove-source-branch flag, then ask for confirmation before calling the GitLab API. |
| `gitlab_interactive_project_create` | — | Interactively create a GitLab project: step-by-step prompts collect name (required), description (optional), visibility (private/internal/public), initialize-with-README flag, and default branch name, then ask for confirmation before calling the GitLab API. |
| `gitlab_interactive_release_create` | — | Interactively create a GitLab release: step-by-step prompts collect tag name (required, must reference an existing tag), release name (optional, defaults to tag name), and release notes/description (optional multiline), then ask for confirmation before calling the GitLab API. |
| `gitlab_issue` | 63 | Manage GitLab issues: CRUD, notes, discussions, links, time tracking, work items, award emoji, statistics, and resource events. |
| `gitlab_job` | 25 | Manage GitLab CI/CD jobs: list, get, retry, cancel, erase, play manual jobs, wait for completion, download artifacts/logs, and manage CI/CD job token scope. |
| `gitlab_merge_request` | 58 | Manage GitLab merge requests: create, list, get, update, merge, approve, rebase, delete. |
| `gitlab_model_registry` | 1 | Download ML model package files from the GitLab Model Registry. |
| `gitlab_mr_review` | 23 | Review and comment on GitLab merge requests: notes, threaded discussions (inline + general), code diffs, draft notes (batch review), diff versions, and the per-version diff payload. |
| `gitlab_package` | 24 | Manage GitLab package registry, container registry, and protection rules. |
| `gitlab_pipeline` | 33 | Manage GitLab CI/CD pipelines: list, get, create, retry, cancel, delete, and wait for completion. |
| `gitlab_project` | 122 | Manage GitLab projects: CRUD, settings, members, labels, milestones, webhooks, badges, boards, integrations, uploads, Pages, avatars, approval rules, mirrors, and import/export. |
| `gitlab_release` | 12 | Manage GitLab releases and their asset links (binaries, packages, runbooks). |
| `gitlab_repository` | 41 | Browse and manage GitLab repository content: file tree, read/write/delete files, commits, diffs, cherry-pick, revert, blame, compare branches, contributors, archives, changelogs, submodules, render markdown, and commit discussions. |
| `gitlab_runner` | 34 | Manage GitLab CI/CD runners and runner controllers: instance / group / project runner CRUD, registration tokens, job assignments, and runner controllers (admin, experimental). |
| `gitlab_search` | 10 | Search GitLab by scope (instance / group / project) for code, MRs, issues, commits, milestones, notes, projects, snippets, users, or wiki pages. |
| `gitlab_snippet` | 34 | Manage GitLab snippets (personal, project-scoped, and explore feed): CRUD snippet metadata and content, threaded discussions, notes (project snippets only), and award emoji on snippets and snippet notes. |
| `gitlab_tag` | 9 | Manage Git tags and tag protections in a project, plus GPG signature inspection. |
| `gitlab_template` | 12 | Browse GitLab built-in templates (gitignore, CI/CD YAML, Dockerfile, license, project scaffolding) and lint CI configuration. |
| `gitlab_user` | 74 | Manage GitLab users: CRUD, SSH/GPG keys, emails, PATs, impersonation tokens, status, todos, events, notifications, namespaces, and avatars. |
| `gitlab_wiki` | 6 | CRUD project wiki pages and upload attachments to wikis. |
| `gitlab_attestation` 🏢 | 2 | List and download build attestations (SLSA provenance) for project artifacts. |
| `gitlab_audit_event` 🏢 | 6 | List and get GitLab audit events at instance, group, and project levels for compliance tracking. |
| `gitlab_compliance_policy` 🏢 | 2 | Get and update admin compliance policy settings (CSP namespace configuration). |
| `gitlab_dependency` 🏢 | 4 | List project dependencies and create/download SBOM exports (CycloneDX). |
| `gitlab_dora_metrics` 🏢 | 2 | Get DORA metrics: deployment frequency, lead time, MTTR, change failure rate. |
| `gitlab_enterprise_user` 🏢 | 4 | Manage enterprise users for a GitLab group: list, get, disable 2FA, delete. |
| `gitlab_external_status_check` 🏢 | 8 | Manage external status checks for MRs and projects. |
| `gitlab_geo` 🏢 | 8 | Manage Geo replication sites: CRUD, repair OAuth, and check replication status (admin, Premium/Ultimate). |
| `gitlab_group_scim` 🏢 | 4 | Manage SCIM identities for GitLab group provisioning. |
| `gitlab_member_role` 🏢 | 6 | Manage custom member roles at instance or group level. |
| `gitlab_merge_train` 🏢 | 4 | Manage GitLab merge trains (automated merge queues). |
| `gitlab_project_alias` 🏢 | 4 | CRUD project aliases: short names that redirect to projects (admin, Premium/Ultimate). |
| `gitlab_security_finding` 🏢 | 1 | List pipeline security report findings via GraphQL (Premium/Ultimate). |
| `gitlab_storage_move` 🏢 | 18 | Manage repository storage moves for projects, groups, and snippets (admin only). |
| `gitlab_vulnerability` 🏢 | 8 | List, triage, and summarize project vulnerabilities (Premium/Ultimate, GraphQL). |

**32 base** / **47 with enterprise** meta-tools. See [Meta-Tools Reference](docs/meta-tools.md) for the complete list with actions and examples.

<!-- END TOOLS -->

## Compatibility

| MCP Capability | Support |
|----------------|---------|
| **Tools** | 1006 individual / 32–47 meta |
| **Resources** | 44 (static + templates) |
| **Prompts** | 38 templates |
| **Completions** | Project, user, group, branch, tag |
| **Logging** | Structured (text/JSON) + MCP notifications |
| **Progress** | Tool execution progress reporting |
| **Sampling** | 11 LLM-powered analysis actions via `gitlab_analyze` |
| **Elicitation** | 4 interactive creation wizards |
| **Roots** | Workspace root tracking |

Tested with: VS Code + GitHub Copilot, Claude Desktop, Claude Code, Cursor, Windsurf, JetBrains IDEs, Zed, Kiro, Cline, Roo Code.

See the full [Compatibility Matrix](https://jmrplens.github.io/gitlab-mcp-server/compatibility/) for detailed client support.

## Documentation

Full documentation is available at **[jmrplens.github.io/gitlab-mcp-server](https://jmrplens.github.io/gitlab-mcp-server/)**.

| Document | Description |
|----------|-------------|
| [Getting Started](docs/getting-started.md) | Download, setup wizard, per-client configuration |
| [Configuration](docs/configuration.md) | Environment variables, transport modes, TLS |
| [Tools Reference](docs/tools/README.md) | All 1006 individual tools with input/output schemas |
| [Meta-Tools](docs/meta-tools.md) | 32/47 domain meta-tools with action dispatching |
| [Resources](docs/resources-reference.md) | All 44 resources with URI templates |
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
| GitLab Client | `gitlab.com/gitlab-org/api/client-go/v2` v2.20.1 |
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

# Single-instance mode (fixed GitLab URL for all clients)
docker run -d --name gitlab-mcp-server -p 8080:8080 \
  -e GITLAB_URL=https://gitlab.example.com \
  -e GITLAB_SKIP_TLS_VERIFY=true \
  ghcr.io/jmrplens/gitlab-mcp-server:latest

# Multi-instance mode (clients send GITLAB-URL header per request)
docker run -d --name gitlab-mcp-server -p 8080:8080 \
  ghcr.io/jmrplens/gitlab-mcp-server:latest
```

Clients authenticate via `PRIVATE-TOKEN` or `Authorization: Bearer` headers. In multi-instance mode, clients must also send a `GITLAB-URL` header to target a specific GitLab instance. See [HTTP Server Mode](docs/http-server-mode.md) and [Docker documentation](docs/development/development.md#docker) for Docker Compose and configuration options.

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

Alternatively, set `GITLAB_SAFE_MODE=true` for a dry-run mode: mutating tools remain visible but return a structured JSON preview instead of executing. Useful for auditing, training, or reviewing what an AI assistant would do.
</details>

<details>
<summary><strong>What GitLab editions are supported?</strong></summary>

Both Community Edition (CE) and Enterprise Edition (EE). Set `GITLAB_ENTERPRISE=true` in stdio mode to enable additional tools for Premium/Ultimate features (DORA metrics, vulnerabilities, compliance, etc.). In HTTP mode, `--enterprise` can force the Enterprise/Premium catalog, otherwise CE/EE is detected per token+URL pool entry when GitLab reports edition.
</details>

<details>
<summary><strong>How does it handle rate limiting?</strong></summary>

The server includes retry logic with backoff for GitLab API rate limits. Errors are classified as transient (retryable) or permanent, with actionable hints in error messages.
</details>

<details>
<summary><strong>Which AI clients are supported?</strong></summary>

Any MCP-compatible client: VS Code + GitHub Copilot, Claude Desktop, Cursor, Claude Code, Windsurf, JetBrains IDEs, Zed, Kiro, and others. The built-in setup wizard can auto-configure most clients.
</details>

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development guidelines, branch naming, commit conventions, and pull request process.

## Security

See [SECURITY.md](SECURITY.md) for the security policy and vulnerability reporting.

## Code of Conduct

See [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md). This project follows the [Contributor Covenant v2.1](https://www.contributor-covenant.org/version/2/1/code_of_conduct/).
