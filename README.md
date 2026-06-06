<p align="center">
  <img alt="" src="site/src/assets/banner-dark.svg" width="840">
</p>

# GitLab MCP Server

<p align="center">

[![GitHub Release](https://img.shields.io/github/v/release/jmrplens/gitlab-mcp-server?style=flat&logo=github&label=Release)](https://github.com/jmrplens/gitlab-mcp-server/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/jmrplens/gitlab-mcp-server)](https://goreportcard.com/report/github.com/jmrplens/gitlab-mcp-server)
[![Go Reference](https://pkg.go.dev/badge/github.com/jmrplens/gitlab-mcp-server/v2.svg)](https://pkg.go.dev/github.com/jmrplens/gitlab-mcp-server/v2)
[![Glama MCP Score](https://glama.ai/mcp/servers/jmrplens/gitlab-mcp-server/badges/score.svg)](https://glama.ai/mcp/servers/jmrplens/gitlab-mcp-server)
[![GitLab Mirror](https://img.shields.io/badge/GitLab-mirror-FC6D26?logo=gitlab&logoColor=white)](https://gitlab.com/jmrp/gitlab-mcp-server)

</p>

<p align="center">

[![Quality Gate](https://sonarcloud.io/api/project_badges/measure?project=jmrplens_gitlab-mcp-server&metric=alert_status)](https://sonarcloud.io/summary/overall?id=jmrplens_gitlab-mcp-server)
[![Coverage](https://sonarcloud.io/api/project_badges/measure?project=jmrplens_gitlab-mcp-server&metric=coverage)](https://sonarcloud.io/summary/overall?id=jmrplens_gitlab-mcp-server)
![Platform](https://img.shields.io/badge/Windows%20%7C%20Linux%20%7C%20macOS-amd64%20%26%20arm64-lightgrey?style=flat&logo=windows-terminal&logoColor=white)

</p>

A **Model Context Protocol (MCP) server** that exposes the entire GitLab API as MCP tools, resources, and prompts for AI assistants. Single static binary — zero dependencies.

> **Security first**: Continuously monitored on [SonarCloud](https://sonarcloud.io/summary/overall?id=jmrplens_gitlab-mcp-server) with quality gates, coverage, and security scanning. Supports read-only mode, safe mode (dry-run preview), and self-hosted GitLab with TLS verification.
>
> **Repository mirror**: GitHub is the canonical repository. A read-only mirror of the code and releases is available on [GitLab.com](https://gitlab.com/jmrp/gitlab-mcp-server) for discoverability; please open code contributions on GitHub.

## Token Footprint

<!-- START TOKEN FOOTPRINT -->

Measured with `go run ./cmd/gen_readme/` against the current base catalog. Totals estimate startup context visible to an MCP client: visible tool schemas plus shared resources and prompts, using the same byte/4 token heuristic as `cmd/audit_tokens`.

**Default configuration**: with `TOOL_SURFACE` unset or `TOOL_SURFACE=dynamic`, `CAPABILITY_SURFACE=full`, `META_TOOLS` unset, `META_PARAM_SCHEMA=opaque`, and `GITLAB_ENTERPRISE` unset or `false`, the server uses the **dynamic find/execute surface**. Use `TOOL_SURFACE=meta` only when you explicitly want domain meta-tools; use `TOOL_SURFACE=individual` only when your client can handle the full tool catalog.

| Configuration (`TOOL_SURFACE` / `CAPABILITY_SURFACE`) | Visible tools | Reachable actions | `META_PARAM_SCHEMA` | Tool schema tokens | Shared tokens | Total tokens |
| ----------------------------------------------------- | ------------: | ----------------: | ------------------- | -----------------: | ------------: | -----------: |
| `dynamic` / `full` (default)                          |             2 |               871 | n/a                 |              2,204 |        18,284 |       20,488 |
| `dynamic` / `minimal`                                 |             2 |               871 | n/a                 |              2,204 |           740 |        2,944 |
| `meta` / `full`                                       |            34 |               871 | `opaque`            |             87,392 |        18,284 |      105,676 |
| `meta` / `minimal`                                    |            34 |               871 | `opaque`            |             87,392 |           740 |       88,132 |
| `individual` / `full`                                 |           867 |               867 | n/a                 |            473,799 |        18,284 |      492,083 |

Rows use the base Community Edition catalog (`GITLAB_ENTERPRISE=false`). `META_PARAM_SCHEMA=opaque` affects only visible meta-tool input schemas; dynamic mode gets exact action schemas from `gitlab_find_action`, and every surface advertises `gitlab://tools` plus `gitlab://tools/{id}` for on-demand action browsing and input schemas. Individual mode already exposes one schema per tool.

<!-- END TOKEN FOOTPRINT -->

## Highlights

- **1027 MCP tools** on self-managed Enterprise/Premium, or **1033 on GitLab.com Enterprise/Premium** with experimental Orbit Knowledge Graph support — broad GitLab REST API v4 + GraphQL coverage across 176 packages under `internal/tools`: projects, branches, tags, releases, merge requests, issues, pipelines, jobs, groups, users, wikis, environments, deployments, packages, container registry, runners, feature flags, CI/CD variables, security attributes, security categories, templates, admin settings, access tokens, deploy keys, Orbit, and more
- **Default dynamic toolset** — exposes only `gitlab_find_action` and `gitlab_execute_action` while keeping the same canonical GitLab action catalog. Optional domain meta-tools remain available with `TOOL_SURFACE=meta`: 33 base, 49 on self-managed Enterprise/Premium, or 50 on GitLab.com Enterprise/Premium
- **AI model tool-use evaluation** — automated schema-only and Docker-backed runs against populated GitLab CE and licensed Enterprise instances measure tool/action selection, parameter shaping, recovery from GitLab errors, and destructive-action safety across Anthropic, Google, OpenAI, and Qwen. Published summaries appear in the managed evaluation block below; see [AI Model Evaluation Results](docs/testing/model-results.md)
- **11 sampling actions** — LLM-assisted code review, issue analysis, pipeline failure diagnosis, security review, release notes, milestone reports, and more via `gitlab_analyze` meta-tool (MCP sampling capability)
- **4 elicitation tools** — interactive creation wizards (issue, MR, release, project) with step-by-step user prompts
- **46 MCP resources** in default dynamic/full mode — read-only data: user, groups, group members, group projects, projects, issues, pipelines, members, labels, milestones, branches, MRs, releases, tags, commits, file blobs, wiki pages, MR notes, MR discussions, single-entity templates (issue, MR, branch, tag, release, label, milestone, commit, wiki page, deployment, environment, job, board, snippet, deploy key, feature flag, group label, group milestone), the surface-aware `gitlab://tools` manifest and `gitlab://tools/{id}` detail template, workspace roots, and 5 workflow best-practice guides
- **37 MCP prompts** — AI-optimized: code review, pipeline status, risk assessment, release notes, standup, workload, user stats, team management, cross-project dashboards, analytics, milestones, Git workflow quality, audit
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

Download the latest binary for your platform from [GitHub Releases](https://github.com/jmrplens/gitlab-mcp-server/releases) and make it executable:

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

Three registration modes, controlled by `TOOL_SURFACE`:

| Mode                          | Tools                                                                              | Description                                                                                                                                                       |
| ----------------------------- | ---------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Dynamic Toolset** (default) | 2 visible tools                                                                    | Low-token find/execute surface over the canonical action catalog.                                                                                                 |
| **Meta-Tools**                | 33 base GitLab/interactive tools; `gitlab_server` is a separate maintenance helper | Domain-grouped dispatchers with `action` parameter. Enable with `TOOL_SURFACE=meta`; see the full 33/49/50 catalog in [Meta-Tools Reference](docs/meta-tools.md). |
| **Individual**                | 867 CE / 1027 self-managed enterprise / 1033 GitLab.com Enterprise                 | Every GitLab operation as a separate MCP tool.                                                                                                                    |

For dynamic experiments where resources and prompts dominate initial context, set `CAPABILITY_SURFACE=minimal` (stdio) or `--capability-surface=minimal` (HTTP). Minimal keeps `gitlab://workspace/roots` plus the surface-aware `gitlab://tools` manifest so dynamic, meta, and individual deployments can still read accepted call shapes. The default remains `full`.

Dynamic mode is now the default low-token find/execute surface; see [Dynamic Toolset](docs/dynamic-tools.md) for the field-aware ranking model, fuzzy fallback, response shapes, workflow diagrams, and migration guidance. Set `TOOL_SURFACE=meta` to use the consolidated domain meta-tool catalog.

The detailed meta-tool catalog now lives in [Meta-Tools Reference](docs/meta-tools.md), including action counts, Enterprise/Premium markers, and examples.

## Compatibility

| MCP Capability  | Support                                              |
| --------------- | ---------------------------------------------------- |
| **Tools**       | Up to 1033 individual / 33–50 meta                   |
| **Resources**   | 46 (static + templates)                              |
| **Prompts**     | 37 templates                                         |
| **Completions** | Project, user, group, branch, tag                    |
| **Logging**     | Structured (text/JSON) + MCP notifications           |
| **Progress**    | Tool execution progress reporting                    |
| **Sampling**    | 11 LLM-powered analysis actions via `gitlab_analyze` |
| **Elicitation** | 4 interactive creation wizards                       |
| **Roots**       | Workspace root tracking                              |

Tested with: VS Code + GitHub Copilot, Claude Desktop, Claude Code, Cursor, Windsurf, JetBrains IDEs, Zed, Kiro, Cline, Roo Code.

See the full [Compatibility Matrix](https://jmrplens.github.io/gitlab-mcp-server/compatibility/) for detailed client support.

## AI Model Tool-Use Evaluation

The project includes an automated evaluator for model-facing MCP quality. It can
run schema-only checks against the tool catalog or execute validated model tool
calls through MCP against Docker GitLab CE or licensed Enterprise instances
populated with fixtures.
The evaluator measures whether each model chooses the correct meta-tool and
action, sends valid parameters, recovers from actionable GitLab errors, and
respects destructive-action safeguards.

<!-- START MODEL EVAL META SUMMARY -->
Current published result: **Docker CE-on-Enterprise meta 20260527**.

| Provider  | Model                       | Compatibility | Tool accuracy |       Recovery | Docker live status          |
| --------- | --------------------------- | ------------- | ------------: | -------------: | --------------------------- |
| Anthropic | `claude-haiku-4-5-20251001` | OK            |        100.0% |     No repairs | 100.0% final across 274 ops |
| Google    | `gemini-flash-latest`       | Review        |         74.3% | 100.0% (36/36) | 100.0% final across 274 ops |
| OpenAI    | `gpt-5.4-nano`              | Review        |         99.3% |   100.0% (6/6) | 100.0% final across 274 ops |
| Qwen      | `qwen3.6-flash`             | OK            |        100.0% |   100.0% (5/5) | 100.0% final across 274 ops |

The published model-evaluation set covers 560 task attempts and 1096 expected MCP operations. Across the selected reports, models emitted 1109 tool calls over 1145 model requests, with 100.0% aggregate final success. See [AI Model Evaluation Results](docs/testing/model-results.md) for the detailed current matrix.
<!-- END MODEL EVAL META SUMMARY -->

<!-- START MODEL EVAL DYNAMIC SUMMARY -->
Current published result: **Docker CE dynamic 20260606**.

| Provider  | Model                       | Compatibility | Tool accuracy |      Recovery | Docker live status          |
| --------- | --------------------------- | ------------- | ------------: | ------------: | --------------------------- |
| Anthropic | `claude-haiku-4-5-20251001` | OK            |        100.0% |  100.0% (6/6) | 100.0% final across 573 ops |
| Google    | `gemini-flash-latest`       | Review        |        100.0% |   80.0% (4/5) | 99.3% final across 573 ops  |
| OpenAI    | `gpt-5.4-nano`              | Review        |         99.4% | 95.8% (23/24) | 97.4% final across 573 ops  |
| Qwen      | `qwen3.6-flash`             | Review        |        100.0% | 90.9% (10/11) | 99.3% final across 573 ops  |

The published model-evaluation set covers 620 task attempts and 2292 expected MCP operations. Across the selected reports, models emitted 2367 tool calls over 2369 model requests, with 99.0% aggregate final success. See [AI Model Evaluation Results](docs/testing/model-results.md) for the detailed current matrix.
<!-- END MODEL EVAL DYNAMIC SUMMARY -->

<!-- START MODEL EVAL ENTERPRISE META SUMMARY -->
Current published result: **Docker Enterprise meta 20260527**.

| Provider  | Model                       | Compatibility | Tool accuracy |     Recovery | Docker live status         |
| --------- | --------------------------- | ------------- | ------------: | -----------: | -------------------------- |
| Anthropic | `claude-haiku-4-5-20251001` | OK            |        100.0% | 100.0% (1/1) | 100.0% final across 84 ops |
| Google    | `gemini-flash-latest`       | Review        |         78.2% | 100.0% (7/7) | 100.0% final across 84 ops |
| OpenAI    | `gpt-5.4-nano`              | Review        |        100.0% | 100.0% (4/4) | 100.0% final across 84 ops |
| Qwen      | `qwen3.6-flash`             | OK            |        100.0% | 100.0% (1/1) | 100.0% final across 84 ops |

The published model-evaluation set covers 92 task attempts and 336 expected MCP operations. Across the selected reports, models emitted 345 tool calls over 350 model requests, with 100.0% aggregate final success. See [AI Model Evaluation Results](docs/testing/model-results.md) for the detailed current matrix.
<!-- END MODEL EVAL ENTERPRISE META SUMMARY -->

<!-- START MODEL EVAL ENTERPRISE DYNAMIC SUMMARY -->
Current published result: **Docker Enterprise dynamic 20260605 (Enterprise)**.

| Provider  | Model                       | Compatibility | Tool accuracy |     Recovery | Docker live status          |
| --------- | --------------------------- | ------------- | ------------: | -----------: | --------------------------- |
| Anthropic | `claude-haiku-4-5-20251001` | OK            |        100.0% |   No repairs | 100.0% final across 202 ops |
| Google    | `gemini-flash-latest`       | OK            |        100.0% |   No repairs | 100.0% final across 202 ops |
| OpenAI    | `gpt-5.4-nano`              | OK            |        100.0% | 100.0% (3/3) | 100.0% final across 202 ops |
| Qwen      | `qwen3.6-flash`             | OK            |        100.0% |   No repairs | 100.0% final across 202 ops |

The published model-evaluation set covers 124 task attempts and 808 expected MCP operations. Across the selected reports, models emitted 817 tool calls over 817 model requests, with 100.0% aggregate final success. See [AI Model Evaluation Results](docs/testing/model-results.md) for the detailed current matrix.
<!-- END MODEL EVAL ENTERPRISE DYNAMIC SUMMARY -->

## Documentation

Full documentation is available at **[jmrplens.github.io/gitlab-mcp-server](https://jmrplens.github.io/gitlab-mcp-server/)**. Use this map when you need the source-of-truth reference for a specific area:

| Document                                             | Description                                                                            |
| ---------------------------------------------------- | -------------------------------------------------------------------------------------- |
| [Getting Started](docs/getting-started.md)           | Download, setup wizard, per-client configuration                                       |
| [IDE Configuration](docs/ide-configuration.md)       | Per-client stdio, HTTP legacy, and HTTP OAuth examples                                 |
| [Configuration](docs/configuration.md)               | Environment variables, transport modes, TLS                                            |
| [Environment Variables](docs/env-reference.md)       | Exhaustive environment variable table with defaults and examples                       |
| [CLI Reference](docs/cli-reference.md)               | All command-line flags, exit codes, and runtime examples                               |
| [HTTP Server Mode](docs/http-server-mode.md)         | Shared HTTP deployments, authentication, server pool isolation                         |
| [Tools Reference](docs/tools/README.md)              | All individual tools with input/output schemas, including GitLab.com-only Orbit        |
| [Meta-Tools](docs/meta-tools.md)                     | 33/49/50 domain meta-tools with action dispatching                                     |
| [Dynamic Toolset](docs/dynamic-tools.md)             | 2-tool low-token mode with canonical action catalog, safety model, and examples        |
| [Resources](docs/resources-reference.md)             | All 46 resources with URI templates                                                    |
| [Prompts](docs/prompts-reference.md)                 | All 37 prompts with arguments and output format                                        |
| [Auto-Update](docs/auto-update.md)                   | Self-update mechanism, modes, and release format                                       |
| [Testing](docs/testing/README.md)                    | Unit, E2E, schema model evaluation, Docker model evaluation, and curated model results |
| [Security](docs/security.md)                         | Security model, token scopes, input validation                                         |
| [Architecture](docs/architecture.md)                 | System architecture, component design, data flow                                       |
| [Development Guide](docs/development/development.md) | Building, testing, CI/CD, contributing                                                 |
| [Troubleshooting](docs/troubleshooting.md)           | Common startup, token, TLS, transport, and tool-discovery issues                       |

## Tech Stack

| Component     | Technology                                       |
| ------------- | ------------------------------------------------ |
| Language      | Go 1.26+                                         |
| MCP SDK       | `github.com/modelcontextprotocol/go-sdk` v1.6.0  |
| GitLab Client | `gitlab.com/gitlab-org/api/client-go/v2` v2.29.0 |
| Transport     | stdio (default), HTTP (Streamable HTTP)          |

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

| Category                 |     Files |       Lines |
| ------------------------ | --------: | ----------: |
| Source (`.go`, non-test) |       912 |     154,685 |
| Unit tests (`_test.go`)  |       490 |     255,666 |
| End-to-end tests         |       139 |      31,477 |
| **Total**                | **1,541** | **441,828** |

### Functions

| Category                        |  Count |
| ------------------------------- | -----: |
| Source functions                |  6,508 |
| — exported (public)             |  2,467 |
| — unexported (private)          |  4,041 |
| Unit test functions (`TestXxx`) | 10,331 |
| Subtests (`t.Run(...)`)         |  2,503 |
| End-to-end test functions       |    279 |

### Ratios worth noting

| Observation                        |                      Value |
| ---------------------------------- | -------------------------: |
| Test lines vs source lines         | 1.65× more tests than code |
| Average source file length         |                 ~169 lines |
| Average test file length           |                 ~521 lines |
| Comment lines in source            |   12,059 (~7.8% of source) |
| Test functions per source function |                       1.6× |

### Code patterns

| Pattern                            | Count |
| ---------------------------------- | ----: |
| `if err != nil` checks             | 6,095 |
| `defer` statements                 |   782 |
| `struct` types defined             | 2,339 |
| `//nolint` suppressions            |    76 |
| `TODO` / `FIXME` / `HACK` comments |     1 |

### Project

| Metric                         | Value |
| ------------------------------ | ----: |
| Go packages                    |   219 |
| Direct dependencies (`go.mod`) |    11 |
| Indirect dependencies          |    50 |
| Git commits                    |   187 |
| Unique contributors            |     2 |

### Hall of fame

| Record              | File                                                     |
| ------------------- | -------------------------------------------------------- |
| Longest source file | `internal/tools/dynamic/register.go` — 3,740 lines       |
| Longest test file   | `internal/tools/projects/projects_test.go` — 7,099 lines |

### Because why not

| Fact                                 | Value                                                                                                |
| ------------------------------------ | ---------------------------------------------------------------------------------------------------- |
| Source code printed at 55 lines/page | ~2,812 pages of A4                                                                                   |
| Source lines mentioning `"gitlab"`   | 9,343 (impossible to avoid)                                                                          |
| Longest function name in source      | `assertDynamicCompatibilityPolicyOwnedByActionCompat` (51 chars)                                     |
| Longest test function name           | `TestRequiredMissingAndUnknownParamNames_SchemaValidation_ReturnsSortedMissingAndUnknown` (87 chars) |

<!-- END STATS -->
