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

## Token Footprint

Measured with `go run ./cmd/audit_tokens/` against the current catalog. Totals estimate startup context visible to an MCP client: tool schemas plus shared resources and prompts, using a 200K-token context window as a reference.

**Default configuration**: with `TOOL_SURFACE` unset, `META_TOOLS` unset or `true`, and `GITLAB_ENTERPRISE` unset or `false`, the server uses **base meta-tools**. That means 33 visible tools, 855 reachable actions, and no Enterprise/Premium-only catalog.

| Mode / configuration | Visible tools | Reachable actions | Tool tokens | Shared tokens | Total tokens |
| --- | ---: | ---: | ---: | ---: | ---: |
| Individual tools (Enterprise/Premium catalog) | 1006 | 1006 | 532,774 | 17,622 | 550,396 |
| Meta-tools (base catalog + MCP helpers, **default**) | 33 | 855 | 57,018 | 18,198 | 75,216 |
| Meta-tools (Enterprise/Premium catalog + MCP helpers) | 48 | 1010 | 71,837 | 18,198 | 90,035 |
| Dynamic-3 (`TOOL_SURFACE=dynamic` or `dynamic-3`) | 3 | 855 / 1010 | 2,029 | 18,198 | 20,227 |
| Dynamic-3 + minimal capabilities (`CAPABILITY_SURFACE=minimal`) | 3 | 855 / 1010 | 2,029 | 184 | 2,213 |

Reachable actions include the five standalone utility actions (`gitlab_discover_project` plus four interactive creation flows). They are visible standalone tools in meta mode and folded into the dynamic catalog, which is why the catalog-only route count is 850 / 1005 while the comparable reachable-action count is 855 / 1010. `dynamic` and `dynamic-3` expose the same current three-tool search/describe/execute surface. The experimental `dynamic-2` comparison surface uses two visible tools and measures about **19,521 tokens** with full capabilities or **1,507 tokens** with `CAPABILITY_SURFACE=minimal`.

## Highlights

- **1006 MCP tools** on self-managed Enterprise/Premium, or **1011 on GitLab.com Enterprise/Premium** with experimental Orbit Knowledge Graph support — broad GitLab REST API v4 + GraphQL coverage across 163 domain sub-packages: projects, branches, tags, releases, merge requests, issues, pipelines, jobs, groups, users, wikis, environments, deployments, packages, container registry, runners, feature flags, CI/CD variables, templates, admin settings, access tokens, deploy keys, Orbit, and more
- **32 meta-tools** (47 on self-managed Enterprise/Premium, 48 on GitLab.com Enterprise/Premium) — domain-grouped dispatchers that reduce token overhead for LLMs (optional, enabled by default). A low-token dynamic mode can expose only `gitlab_search_tools`, `gitlab_describe_tools`, and `gitlab_execute_tool` while keeping the same canonical GitLab action catalog
- **AI model tool-use evaluation** — automated schema-only and Docker-backed runs against a populated GitLab CE instance measure tool/action selection, parameter shaping, recovery from GitLab errors, and destructive-action safety across Anthropic, Google, OpenAI, and Qwen. The current reviewed result is published in the managed evaluation block below; see [AI Model Evaluation Results](docs/testing/model-results.md)
- **11 sampling actions** — LLM-assisted code review, issue analysis, pipeline failure diagnosis, security review, release notes, milestone reports, and more via `gitlab_analyze` meta-tool (MCP sampling capability)
- **4 elicitation tools** — interactive creation wizards (issue, MR, release, project) with step-by-step user prompts
- **46 MCP resources** — read-only data: user, groups, group members, group projects, projects, issues, pipelines, members, labels, milestones, branches, MRs, releases, tags, commits, file blobs, wiki pages, MR notes, MR discussions, meta-tool JSON Schemas, single-entity templates (issue, MR, branch, tag, release, label, milestone, commit, wiki page, deployment, environment, job, board, snippet, deploy key, feature flag, group label, group milestone), workspace roots, and 5 workflow best-practice guides
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

### 1. Get the server

Download the latest binary for your platform from [GitHub Releases](../../releases) and make it executable:

```bash
chmod +x gitlab-mcp-server-*  # Linux/macOS only
```

Or pull the published container image:

```bash
docker pull ghcr.io/jmrplens/gitlab-mcp-server:latest
```

### 2. Configure GitLab access

**Recommended**: Run the built-in setup wizard — it configures your GitLab connection and MCP client in one step:

```bash
./gitlab-mcp-server --setup
```

> **Tip**: The wizard supports Web UI, Terminal UI, and plain CLI modes. On Windows, double-click the `.exe` to launch the wizard automatically.

Manual setup only needs a GitLab Personal Access Token with `api` scope:

```env
GITLAB_TOKEN=glpat-xxxxxxxxxxxxxxxxxxxx
```

`GITLAB_URL` defaults to `https://gitlab.com`; add it only when you connect to a self-managed GitLab instance.

```env
GITLAB_URL=https://gitlab.example.com
```

### 3. Connect your MCP client

Most desktop clients use stdio: the client starts one local MCP server process and talks to it over stdin/stdout. Choose one of these runtime patterns.

#### Native binary (stdio)

VS Code and Cursor-style MCP configuration:

Add to `.vscode/mcp.json` in your workspace:

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

Claude Desktop uses the same server command under `mcpServers`:

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

For client-specific paths, secure token prompts, HTTP OAuth, and extra IDEs, see [IDE Configuration](docs/ide-configuration.md).

#### Docker launched by an IDE (stdio)

If an IDE starts Docker as the MCP server process, keep `docker run -i` and pass `--http=false` after the image name. Do not publish port 8080 in this mode.

```json
{
  "servers": {
    "gitlab": {
      "type": "stdio",
      "command": "docker",
      "args": [
        "run",
        "-i",
        "--rm",
        "-e",
        "GITLAB_TOKEN",
        "-e",
        "GITLAB_URL",
        "-e",
        "GITLAB_SKIP_TLS_VERIFY",
        "ghcr.io/jmrplens/gitlab-mcp-server:latest",
        "--http=false"
      ],
      "env": {
        "GITLAB_TOKEN": "glpat-xxxxxxxxxxxxxxxxxxxx",
        "GITLAB_URL": "https://gitlab.com",
        "GITLAB_SKIP_TLS_VERIFY": "false"
      }
    }
  }
}
```

#### Docker or binary as an HTTP MCP server

Use HTTP mode for shared, remote, or multi-user deployments. The Docker image
starts in HTTP mode by default, but the flags are shown explicitly here for
clarity. These examples publish the container port on host loopback only;
`--http-addr=0.0.0.0:8080` binds inside the container.

```bash
# Fixed GitLab instance for all clients
docker run -d --name gitlab-mcp-server -p 127.0.0.1:8080:8080 \
  ghcr.io/jmrplens/gitlab-mcp-server:latest \
  --http \
  --http-addr=0.0.0.0:8080 \
  --gitlab-url=https://gitlab.com

# Multi-instance mode: clients send GITLAB-URL per request
docker run -d --name gitlab-mcp-server -p 127.0.0.1:8080:8080 \
  ghcr.io/jmrplens/gitlab-mcp-server:latest \
  --http \
  --http-addr=0.0.0.0:8080
```

HTTP clients authenticate each request with `PRIVATE-TOKEN` or `Authorization: Bearer`:

```jsonc
{
  "servers": {
    "gitlab": {
      "type": "http",
      "url": "http://localhost:8080/mcp",
      "headers": {
        "PRIVATE-TOKEN": "glpat-xxxxxxxxxxxxxxxxxxxx"
      }
    }
  }
}
```

In multi-instance mode, clients must also send `GITLAB-URL`. See [HTTP Server Mode](docs/http-server-mode.md) for OAuth, reverse proxy, rate limit, and server-pool details.

### 4. Verify

Open your AI client and try:

> _"List my GitLab projects"_

See the [Getting Started guide](https://jmrplens.github.io/gitlab-mcp-server/getting-started/) for detailed setup instructions.

## Tool Modes

Three registration modes, controlled by `META_TOOLS` or `TOOL_SURFACE`:

| Mode | Tools | Description |
|------|-------|-------------|
| **Meta-Tools** (default) | 32 base / 47 self-managed enterprise / 48 GitLab.com Enterprise | Domain-grouped dispatchers with `action` parameter. Lower token usage. |
| **Dynamic Toolset** | 3 visible tools | Low-token search/describe/execute surface over the canonical action catalog. Enable with `TOOL_SURFACE=dynamic` or `META_TOOLS=dynamic`. `dynamic-3` is the explicit current candidate and `dynamic-2` remains the parked find/execute comparison surface. |
| **Individual** | 863 CE / 1006 self-managed enterprise / 1011 GitLab.com Enterprise | Every GitLab operation as a separate MCP tool. |

For dynamic experiments where resources and prompts dominate initial context, set `CAPABILITY_SURFACE=minimal` (stdio) or `--capability-surface=minimal` (HTTP) to keep only `gitlab://workspace/roots` and omit optional MCP resources and prompts. The default remains `full`.

Meta-tools remain the default today. Dynamic mode is the current low-token candidate for a future default; see [Dynamic Toolset](docs/dynamic-tools.md) for the search/describe/execute workflow, diagrams, and migration guidance.

Meta-tool summary:

<!-- START TOOLS -->

| Meta-Tool | Actions | Description |
|-----------|:-------:|-------------|
| `gitlab_access` | 48 | Manage GitLab access credentials: access tokens (project/group/personal), deploy tokens, deploy keys, access requests, and invitations. |
| `gitlab_admin` | 88 | GitLab self-managed instance administration: settings, license, broadcast messages, system hooks, Sidekiq monitoring, plan limits, OAuth applications, secure files, Terraform states, cluster agents, dependency proxy cache, plus bulk imports (GitLab→GitLab migrations) and external imports (GitHub/Bitbucket). |
| `gitlab_analyze` | 11 | LLM-assisted analysis of GitLab data via MCP sampling. |
| `gitlab_branch` | 11 | Manage Git branches and branch protections in a project, plus aggregated branch rules (GraphQL). |
| `gitlab_ci_catalog` | 2 | Discover and inspect CI/CD Catalog resources (reusable pipeline components and templates published by groups for import into .gitlab-ci.yml). |
| `gitlab_ci_variable` | 15 | Manage GitLab CI/CD variables at instance, group, and project scope. |
| `gitlab_custom_emoji` | 3 | Manage group-level custom emoji via GraphQL. |
| `gitlab_discover_project` | — | Resolve a full git remote URL to a GitLab project and return its project_id and metadata. |
| `gitlab_environment` | 23 | Manage GitLab deployment environments, protected environments, freeze (deploy block) periods, and the deployment record audit trail. |
| `gitlab_feature_flags` | 10 | Manage project feature flags and feature-flag user lists for gradual rollouts. |
| `gitlab_group` | 130 | Manage GitLab groups: CRUD, subgroups, members, labels, milestones, webhooks, badges, boards, uploads, and import/export. |
| `gitlab_interactive_issue_create` | — | Create a GitLab issue through step-by-step prompts, with explicit confirmation before calling the GitLab API. |
| `gitlab_interactive_mr_create` | — | Create a GitLab merge request through step-by-step prompts, with explicit confirmation before calling the GitLab API. |
| `gitlab_interactive_project_create` | — | Create a GitLab project through step-by-step prompts, with explicit confirmation before calling the GitLab API. |
| `gitlab_interactive_release_create` | — | Create a GitLab release through step-by-step prompts, with explicit confirmation before calling the GitLab API. |
| `gitlab_issue` | 63 | Manage GitLab issues: CRUD, notes, discussions, links, time tracking, work items, award emoji, statistics, and resource events. |
| `gitlab_job` | 25 | Manage GitLab CI/CD jobs and the CI/CD job token scope: lifecycle, manual play, log/artifact retrieval, and inbound trust boundaries. |
| `gitlab_merge_request` | 58 | Manage GitLab merge request lifecycle plus approval rules and settings, time tracking, subscriptions, context commits, MR dependencies (blocking MRs), todos, related issues, award emoji, and resource events. |
| `gitlab_model_registry` | 1 | Download ML model package files from the GitLab Model Registry. |
| `gitlab_mr_review` | 23 | Review and comment on GitLab merge requests: notes, threaded discussions (inline + general), code diffs, draft notes (batch review), diff versions, and the per-version diff payload. |
| `gitlab_package` | 24 | Manage GitLab package registry, container registry, and protection rules. |
| `gitlab_pipeline` | 33 | Manage GitLab CI/CD pipelines plus trigger tokens, resource groups (mutual-exclusion locks), JUnit test reports, and pipeline schedules. |
| `gitlab_project` | 122 | Manage GitLab projects end-to-end: lifecycle (create/fork/transfer/archive/delete), visibility & access (members, share, approval rules, integrations, webhooks), and advanced features (mirrors, Pages, badges, boards, labels, milestones, uploads, avatars, import/export, housekeeping). |
| `gitlab_release` | 12 | Manage GitLab releases and their asset links (binaries, packages, runbooks). |
| `gitlab_repository` | 41 | Browse and manage GitLab repository content: file tree, read/write/delete files, commits, diffs, cherry-pick, revert, blame, compare branches, contributors, archives, changelogs, submodules, render markdown, and commit discussions. |
| `gitlab_runner` | 34 | Manage GitLab CI/CD runners (instance, group, project) and runner controllers (admin, experimental): CRUD, registration tokens, and job assignments. |
| `gitlab_search` | 10 | Search GitLab by scope (instance / group / project) for code, MRs, issues, commits, milestones, notes, projects, snippets, users, or wiki pages. |
| `gitlab_snippet` | 34 | Manage GitLab snippets (personal, project-scoped, and explore feed): CRUD snippet metadata and content, threaded discussions, notes (project snippets only), and award emoji on snippets and snippet notes. |
| `gitlab_tag` | 9 | Manage Git tags and tag protections in a project, plus GPG signature inspection. |
| `gitlab_template` | 12 | Browse GitLab built-in templates (gitignore, CI/CD YAML, Dockerfile, license, project scaffolding) and lint CI configuration. |
| `gitlab_user` | 74 | User management for GitLab: full user account CRUD plus SSH/GPG keys, emails, personal access tokens (PATs), impersonation tokens, user status, todos, contribution events, notification settings, namespaces, and avatars. |
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
| `gitlab_orbit` 🏢 | 5 | Experimental GitLab.com-only Orbit Knowledge Graph operations. |
| `gitlab_project_alias` 🏢 | 4 | CRUD project aliases: short names that redirect to projects (admin, Premium/Ultimate). |
| `gitlab_security_finding` 🏢 | 1 | List pipeline security report findings via GraphQL (Premium/Ultimate). |
| `gitlab_storage_move` 🏢 | 18 | Manage repository storage moves for projects, groups, and snippets (admin only). |
| `gitlab_vulnerability` 🏢 | 8 | List, triage, and summarize project vulnerabilities (Premium/Ultimate, GraphQL). |

**32 base** / **47 self-managed enterprise** / **48 GitLab.com Enterprise** meta-tools. Rows marked 🏢 require the Enterprise/Premium catalog; `gitlab_orbit` additionally requires GitLab.com. See [Meta-Tools Reference](docs/meta-tools.md) for the complete list with actions and examples.

<!-- END TOOLS -->

## Compatibility

| MCP Capability | Support |
|----------------|---------|
| **Tools** | Up to 1011 individual / 32–48 meta |
| **Resources** | 46 (static + templates) |
| **Prompts** | 38 templates |
| **Completions** | Project, user, group, branch, tag |
| **Logging** | Structured (text/JSON) + MCP notifications |
| **Progress** | Tool execution progress reporting |
| **Sampling** | 11 LLM-powered analysis actions via `gitlab_analyze` |
| **Elicitation** | 4 interactive creation wizards |
| **Roots** | Workspace root tracking |

Tested with: VS Code + GitHub Copilot, Claude Desktop, Claude Code, Cursor, Windsurf, JetBrains IDEs, Zed, Kiro, Cline, Roo Code.

See the full [Compatibility Matrix](https://jmrplens.github.io/gitlab-mcp-server/compatibility/) for detailed client support.

## AI Model Tool-Use Evaluation

The project includes an automated evaluator for model-facing MCP quality. It can
run schema-only checks against the tool catalog or execute validated model tool
calls through MCP against a Docker GitLab CE instance populated with fixtures.
The evaluator measures whether each model chooses the correct meta-tool and
action, sends valid parameters, recovers from actionable GitLab errors, and
respects destructive-action safeguards.

<!-- START MODEL EVAL META SUMMARY -->
Current published result: **2026-05-05 Full Docker Economy Run**.

| Provider | Model | Compatibility | Tool accuracy | Recovery | Docker live status |
| --- | --- | --- | ---: | ---: | --- |
| Anthropic | `claude-haiku-4-5-20251001` | OK | 100.0% | No repairs | 100.0% final across 240 ops |
| Google | `gemini-3.1-flash-lite-preview` | OK | 100.0% | 100.0% (1/1) | 100.0% final across 240 ops |
| OpenAI | `gpt-5.4-nano` | OK | 100.0% | 50.0% (2/4) | 100.0% final across 240 ops |
| Qwen | `qwen3.6-flash` | OK | 100.0% | No repairs | 100.0% final across 84 ops |

The published model-evaluation set covers 419 task attempts and 804 expected MCP operations. Across the selected reports, models emitted 809 tool calls over 809 model requests, with 100.0% aggregate final success. See [AI Model Evaluation Results](docs/testing/model-results.md) for the detailed current matrix.
<!-- END MODEL EVAL META SUMMARY -->

<!-- START MODEL EVAL DYNAMIC2 SUMMARY -->
No Dynamic 2-tool evaluation summary has been published yet.
<!-- END MODEL EVAL DYNAMIC2 SUMMARY -->

<!-- START MODEL EVAL DYNAMIC3 SUMMARY -->
Current published result: **Dynamic 3-tool all-provider Docker run 2026-05-09**.

| Provider | Model | Compatibility | Tool accuracy | Recovery | Docker live status |
| --- | --- | --- | ---: | ---: | --- |
| Anthropic | `claude-haiku-4-5-20251001` | OK | 100.0% | 44.4% (4/9) | 100.0% final across 256 ops |
| Google | `gemini-3.1-flash-lite-preview` | OK | 100.0% | 8.3% (1/12) | 100.0% final across 256 ops |
| OpenAI | `gpt-5.4-nano` | OK | 100.0% | 44.4% (12/27) | 100.0% final across 256 ops |
| Qwen | `qwen3.6-flash` | OK | 100.0% | 25.0% (1/4) | 100.0% final across 256 ops |

The published model-evaluation set covers 512 task attempts and 1024 expected MCP operations. Across the selected reports, models emitted 1397 tool calls over 1397 model requests, with 100.0% aggregate final success. See [AI Model Evaluation Results](docs/testing/model-results.md) for the detailed current matrix.
<!-- END MODEL EVAL DYNAMIC3 SUMMARY -->

## Documentation

Full documentation is available at **[jmrplens.github.io/gitlab-mcp-server](https://jmrplens.github.io/gitlab-mcp-server/)**.

| Document | Description |
|----------|-------------|
| [Getting Started](docs/getting-started.md) | Download, setup wizard, per-client configuration |
| [IDE Configuration](docs/ide-configuration.md) | Per-client stdio, HTTP legacy, and HTTP OAuth examples |
| [Configuration](docs/configuration.md) | Environment variables, transport modes, TLS |
| [HTTP Server Mode](docs/http-server-mode.md) | Shared HTTP deployments, authentication, server pool isolation |
| [Tools Reference](docs/tools/README.md) | All individual tools with input/output schemas, including GitLab.com-only Orbit |
| [Meta-Tools](docs/meta-tools.md) | 32/47/48 domain meta-tools with action dispatching |
| [Dynamic Toolset](docs/dynamic-tools.md) | 3-tool low-token mode with canonical action catalog, safety model, and examples |
| [Resources](docs/resources-reference.md) | All 46 resources with URI templates |
| [Prompts](docs/prompts-reference.md) | All 38 prompts with arguments and output format |
| [Auto-Update](docs/auto-update.md) | Self-update mechanism, modes, and release format |
| [Testing](docs/testing/README.md) | Unit, E2E, schema model evaluation, Docker model evaluation, and curated model results |
| [Security](docs/security.md) | Security model, token scopes, input validation |
| [Architecture](docs/architecture.md) | System architecture, component design, data flow |
| [Development Guide](docs/development/development.md) | Building, testing, CI/CD, contributing |

## Tech Stack

| Component | Technology |
|-----------|------------|
| Language | Go 1.26+ |
| MCP SDK | `github.com/modelcontextprotocol/go-sdk` v1.6.0 |
| GitLab Client | `gitlab.com/gitlab-org/api/client-go/v2` v2.24.1 |
| Transport | stdio (default), HTTP (Streamable HTTP) |

## Building from Source

```bash
git clone https://github.com/jmrplens/gitlab-mcp-server.git
cd gitlab-mcp-server
make build
```

See the [Development Guide](docs/development/development.md) for cross-compilation and contributing guidelines.

## Container Image

The published image is `ghcr.io/jmrplens/gitlab-mcp-server:latest`. Runtime examples live in [Quick Start](#quick-start) next to MCP client configuration, and Docker Compose/source-build details live in the [Development Guide](docs/development/development.md#docker).

## FAQ

<details>
<summary><strong>Does it work with self-hosted GitLab?</strong></summary>

Yes. Set `GITLAB_URL` to your instance URL. When `GITLAB_URL` is omitted, stdio mode uses `https://gitlab.com`. Self-signed TLS certificates are supported via `GITLAB_SKIP_TLS_VERIFY=true`.
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

## Unnecessary Statistics

Numbers nobody asked for, but here they are anyway.

<!-- START STATS -->

### File counts

| Category | Files | Lines |
| --- | ---: | ---: |
| Source (`.go`, non-test) | 655 | 138,992 |
| Unit tests (`_test.go`) | 418 | 223,956 |
| End-to-end tests | 111 | 24,022 |
| **Total** | **1,184** | **386,970** |

### Functions

| Category | Count |
| --- | ---: |
| Source functions | 4,199 |
| — exported (public) | 2,265 |
| — unexported (private) | 1,934 |
| Unit test functions (`TestXxx`) | 9,264 |
| Subtests (`t.Run(...)`) | 2,012 |
| End-to-end test functions | 248 |

### Ratios worth noting

| Observation | Value |
| --- | ---: |
| Test lines vs source lines | 1.61× more tests than code |
| Average source file length | ~212 lines |
| Average test file length | ~535 lines |
| Comment lines in source | 10,642 (~7.7% of source) |
| Test functions per source function | 2.2× |

### Code patterns

| Pattern | Count |
| --- | ---: |
| `if err != nil` checks | 5,896 |
| `defer` statements | 732 |
| `struct` types defined | 2,062 |
| `//nolint` suppressions | 54 |
| `TODO` / `FIXME` / `HACK` comments | 1 |

### Project

| Metric | Value |
| --- | ---: |
| Go packages | 198 |
| Direct dependencies (`go.mod`) | 11 |
| Indirect dependencies | 47 |
| Git commits | 122 |
| Unique contributors | 2 |

### Hall of fame

| Record | File |
| --- | --- |
| Longest source file | `cmd/eval_meta_tools/main.go` — 7,136 lines |
| Longest test file | `internal/tools/projects/projects_test.go` — 6,428 lines |

### Because why not

| Fact | Value |
| --- | --- |
| Source code printed at 55 lines/page | ~2,527 pages of A4 |
| Source lines mentioning `"gitlab"` | 11,136 (impossible to avoid) |
| Longest function name in source | `ensureLiveCommitDiscussionNoteDeleteTarget` (42 chars) |
| Longest test function name | `TestRequiredMissingAndUnknownParamNames_SchemaValidation_ReturnsSortedMissingAndUnknown` (87 chars) |

<!-- END STATS -->
