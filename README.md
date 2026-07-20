<p align="center">
  <img alt="GitLab MCP Server — let your AI assistant drive GitLab in plain language" src="https://raw.githubusercontent.com/jmrplens/gitlab-mcp-server/main/site/src/assets/banner-dark.svg" width="840">
</p>

# GitLab MCP Server

<p align="center">

[![GitHub Release](https://img.shields.io/github/v/release/jmrplens/gitlab-mcp-server?style=flat&logo=github&label=Release)](https://github.com/jmrplens/gitlab-mcp-server/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
![Platform](https://img.shields.io/badge/Windows%20%7C%20Linux%20%7C%20macOS-amd64%20%26%20arm64-lightgrey?style=flat&logo=windows-terminal&logoColor=white)
[![Quality Gate](https://sonarcloud.io/api/project_badges/measure?project=jmrplens_gitlab-mcp-server&metric=alert_status)](https://sonarcloud.io/summary/overall?id=jmrplens_gitlab-mcp-server)
[![Coverage](https://sonarcloud.io/api/project_badges/measure?project=jmrplens_gitlab-mcp-server&metric=coverage)](https://sonarcloud.io/summary/overall?id=jmrplens_gitlab-mcp-server)
[![Go Report Card](https://goreportcard.com/badge/github.com/jmrplens/gitlab-mcp-server)](https://goreportcard.com/report/github.com/jmrplens/gitlab-mcp-server)
[![Go Reference](https://pkg.go.dev/badge/github.com/jmrplens/gitlab-mcp-server/v2.svg)](https://pkg.go.dev/github.com/jmrplens/gitlab-mcp-server/v2)
[![Glama MCP Score](https://glama.ai/mcp/servers/jmrplens/gitlab-mcp-server/badges/score.svg)](https://glama.ai/mcp/servers/jmrplens/gitlab-mcp-server)
[![MCP Badge](https://lobehub.com/badge/mcp/jmrplens-gitlab-mcp-server)](https://lobehub.com/mcp/jmrplens-gitlab-mcp-server)

</p>

**Connect your AI assistant to GitLab so it can review merge requests, triage pipelines, manage issues, and draft releases — in plain language.** One static binary (or a container), [1000+ GitLab tools](#tool-surfaces) over the full REST + GraphQL API, working with Claude, Cursor, VS Code, and any MCP client.

You talk to your AI assistant; it does the GitLab work. No project IDs, API endpoints, or JSON to remember.

> "Review merge request !15 — is it safe to merge?" · "Why did the last pipeline fail?" · "List open issues assigned to me" · "Generate release notes from v1.0 to v2.0"

---

> 🤖 **Using an AI assistant?** Give it this repository URL and ask it to install the server for your client. Everything a model needs to do it headlessly — the declarative per-client config, `claude mcp add` one-liners, and defaults — is in [`llms.txt`](llms.txt) (no interactive wizard required).

## Install in 60 seconds

Pick one. Each path ends with you typing a prompt to your assistant.

### One-click install

<table>
  <tr>
    <th align="left">Client</th>
    <th align="left">One-click button</th>
    <th align="left">Token step</th>
  </tr>
  <tr>
    <td><b>VS Code</b></td>
    <td><a href="https://insiders.vscode.dev/redirect/mcp/install?name=gitlab&amp;config=%7B%22command%22%3A%22docker%22%2C%22args%22%3A%5B%22run%22%2C%22-i%22%2C%22--rm%22%2C%22-e%22%2C%22GITLAB_TOKEN%22%2C%22ghcr.io%2Fjmrplens%2Fgitlab-mcp-server%3Alatest%22%2C%22--http%3Dfalse%22%5D%2C%22env%22%3A%7B%22GITLAB_TOKEN%22%3A%22%24%7Binput%3Agitlab_token%7D%22%7D%2C%22inputs%22%3A%5B%7B%22id%22%3A%22gitlab_token%22%2C%22type%22%3A%22promptString%22%2C%22description%22%3A%22GitLab%20Personal%20Access%20Token%20%28api%20scope%29%22%2C%22password%22%3Atrue%7D%5D%7D"><img alt="Install in VS Code" src="https://img.shields.io/badge/Install_in-VS_Code-0098FF?style=flat-square&amp;logo=visualstudiocode&amp;logoColor=white" /></a></td>
    <td>prompts you (masked)</td>
  </tr>
  <tr>
    <td><b>VS Code Insiders</b></td>
    <td><a href="https://insiders.vscode.dev/redirect/mcp/install?name=gitlab&amp;config=%7B%22command%22%3A%22docker%22%2C%22args%22%3A%5B%22run%22%2C%22-i%22%2C%22--rm%22%2C%22-e%22%2C%22GITLAB_TOKEN%22%2C%22ghcr.io%2Fjmrplens%2Fgitlab-mcp-server%3Alatest%22%2C%22--http%3Dfalse%22%5D%2C%22env%22%3A%7B%22GITLAB_TOKEN%22%3A%22%24%7Binput%3Agitlab_token%7D%22%7D%2C%22inputs%22%3A%5B%7B%22id%22%3A%22gitlab_token%22%2C%22type%22%3A%22promptString%22%2C%22description%22%3A%22GitLab%20Personal%20Access%20Token%20%28api%20scope%29%22%2C%22password%22%3Atrue%7D%5D%7D&amp;quality=insiders"><img alt="Install in VS Code Insiders" src="https://img.shields.io/badge/Install_in-VS_Code_Insiders-24bfa5?style=flat-square&amp;logo=visualstudiocode&amp;logoColor=white" /></a></td>
    <td>prompts you (masked)</td>
  </tr>
  <tr>
    <td><b>Cursor</b></td>
    <td><a href="https://cursor.com/install-mcp?name=gitlab&amp;config=eyJjb21tYW5kIjoiZG9ja2VyIiwiYXJncyI6WyJydW4iLCItaSIsIi0tcm0iLCItZSIsIkdJVExBQl9UT0tFTiIsImdoY3IuaW8vam1ycGxlbnMvZ2l0bGFiLW1jcC1zZXJ2ZXI6bGF0ZXN0IiwiLS1odHRwPWZhbHNlIl0sImVudiI6eyJHSVRMQUJfVE9LRU4iOiJZT1VSX0dJVExBQl9UT0tFTiJ9fQ%3D%3D"><img alt="Install in Cursor" src="https://cursor.com/deeplink/mcp-install-dark.svg" height="28" /></a></td>
    <td>edit <code>YOUR_GITLAB_TOKEN</code></td>
  </tr>
  <tr>
    <td><b>LM Studio</b></td>
    <td><a href="https://lmstudio.ai/install-mcp?name=gitlab&amp;config=eyJjb21tYW5kIjoiZG9ja2VyIiwiYXJncyI6WyJydW4iLCItaSIsIi0tcm0iLCItZSIsIkdJVExBQl9UT0tFTiIsImdoY3IuaW8vam1ycGxlbnMvZ2l0bGFiLW1jcC1zZXJ2ZXI6bGF0ZXN0IiwiLS1odHRwPWZhbHNlIl0sImVudiI6eyJHSVRMQUJfVE9LRU4iOiJZT1VSX0dJVExBQl9UT0tFTiJ9fQ%3D%3D"><img alt="Add to LM Studio" src="https://files.lmstudio.ai/deeplink/mcp-install-dark.svg" height="28" /></a></td>
    <td>edit <code>YOUR_GITLAB_TOKEN</code></td>
  </tr>
  <tr>
    <td><b>Kiro</b></td>
    <td><a href="https://kiro.dev/launch/mcp/add?name=gitlab&amp;config=%7B%22command%22%3A%22docker%22%2C%22args%22%3A%5B%22run%22%2C%22-i%22%2C%22--rm%22%2C%22-e%22%2C%22GITLAB_TOKEN%22%2C%22ghcr.io%2Fjmrplens%2Fgitlab-mcp-server%3Alatest%22%2C%22--http%3Dfalse%22%5D%2C%22env%22%3A%7B%22GITLAB_TOKEN%22%3A%22YOUR_GITLAB_TOKEN%22%7D%7D"><img alt="Add to Kiro" src="https://kiro.dev/images/add-to-kiro.svg" height="28" /></a></td>
    <td>edit <code>YOUR_GITLAB_TOKEN</code></td>
  </tr>
  <tr>
    <td><b>Claude Desktop</b></td>
    <td><a href="https://github.com/jmrplens/gitlab-mcp-server/releases/latest/download/gitlab-mcp-server.mcpb"><img alt="Download .mcpb extension" src="https://img.shields.io/badge/Download-.mcpb_extension-d97757?style=flat-square&amp;logo=claude&amp;logoColor=white" /></a></td>
    <td>settings UI (keychain)</td>
  </tr>
</table>

Each button registers the **Docker**-based server (auto-pulls the image on first run; you need [Docker](https://www.docker.com/) installed). The **Claude Desktop** row instead downloads a native [.mcpb desktop extension](docs/guides/claude-desktop-extension.md) (macOS universal + Windows, no Docker) — open it with Claude Desktop and fill in the settings. Need a token? [Create a Personal Access Token](https://docs.gitlab.com/ee/user/profile/personal_access_tokens.html) with the **`api`** scope. Self-managed GitLab? Add a `GITLAB_URL` env var in your client's MCP config after install.

### Claude Code (`claude mcp add`)

Docker (no install — pulls the image on first run):

```bash
claude mcp add gitlab --env GITLAB_TOKEN=glpat-xxxx --transport stdio \
  -- docker run -i --rm -e GITLAB_TOKEN ghcr.io/jmrplens/gitlab-mcp-server:latest --http=false
```

Or install the native binary first, then register it:

```bash
# macOS/Linux (Homebrew)
brew install jmrplens/tap/gitlab-mcp-server
# Linux/macOS (script)
curl -fsSL https://raw.githubusercontent.com/jmrplens/gitlab-mcp-server/main/scripts/install.sh | sh
# Windows (PowerShell)
irm https://raw.githubusercontent.com/jmrplens/gitlab-mcp-server/main/scripts/install.ps1 | iex

claude mcp add gitlab --env GITLAB_TOKEN=glpat-xxxx -- gitlab-mcp-server
```

Self-managed GitLab? Add `--env GITLAB_URL=https://gitlab.example.com` (and `--env GITLAB_SKIP_TLS_VERIFY=true` for self-signed certs).

### Guided setup (any client, no flags to remember)

The binary ships a **setup wizard** that collects your GitLab token and configures your MCP client for you — ideal if you'd rather not edit JSON:

```bash
gitlab-mcp-server --setup
```

It auto-detects VS Code, Claude Desktop, Claude Code, Cursor, and Windsurf and writes the right config. On Windows, double-click the `.exe` to launch it.

### Manual JSON (Claude Desktop, Cursor, VS Code, …)

<details>
<summary>Show JSON config for native binary and Docker</summary>

Native binary (Claude Desktop `mcpServers`, Cursor, etc.):

```json
{
  "mcpServers": {
    "gitlab": {
      "command": "/path/to/gitlab-mcp-server",
      "env": { "GITLAB_TOKEN": "glpat-xxxxxxxxxxxxxxxxxxxx" }
    }
  }
}
```

VS Code (`.vscode/mcp.json`, note `servers` + `type`):

```json
{
  "servers": {
    "gitlab": {
      "type": "stdio",
      "command": "/path/to/gitlab-mcp-server",
      "env": { "GITLAB_TOKEN": "glpat-xxxxxxxxxxxxxxxxxxxx" }
    }
  }
}
```

Docker variant — replace `"command"`/`"args"` with:

```json
"command": "docker",
"args": ["run", "-i", "--rm", "-e", "GITLAB_TOKEN", "ghcr.io/jmrplens/gitlab-mcp-server:latest", "--http=false"]
```

For a shared, long-running HTTP deployment instead of per-user stdio, see [HTTP Server Mode](docs/guides/http-server-mode.md).

</details>

**Then just ask:** open your AI client and try _"List my GitLab projects."_ See the [Getting Started guide](https://jmrplens.github.io/gitlab-mcp-server/getting-started/) for per-client details and [more example prompts](docs/guides/examples/usage-examples.md).

---

## Why this server

- 🗣️ **Plain-language GitLab.** The AI translates "is MR !15 safe to merge?" into the right API calls. You don't touch endpoints, IDs, or JSON.
- 🧰 **The whole platform — [1000+ tools](#tool-surfaces).** Broad GitLab REST v4 + GraphQL coverage: projects, branches, tags, releases, merge requests, issues, pipelines, jobs, groups, users, wikis, environments, deployments, packages, container registry, runners, feature flags, CI/CD variables, security, admin, tokens, and more.
- 🪶 **Low-token by default.** The default **dynamic** surface exposes just 2 tools (`find` + `execute`) while reaching the full catalog — so it fits any client's context window. ([Token footprint →](#token-footprint))
- ✅ **Proven with real models.** An automated evaluator runs Anthropic, Google, OpenAI, and Qwen against live GitLab instances: **99.5% aggregate success** across thousands of operations. ([Results →](#ai-model-tool-use-evaluation))
- 🔒 **Safe by design.** Read-only mode, safe mode (dry-run preview of every mutation), TLS options for self-hosted GitLab, and continuous [SonarCloud](https://sonarcloud.io/summary/overall?id=jmrplens_gitlab-mcp-server) quality/security gates.
- 🖥️ **Runs anywhere.** One static binary or container; Windows, Linux & macOS; amd64 & arm64; stdio (desktop) and HTTP (remote).

<details>
<summary>More: resources, prompts, and capabilities</summary>

- **45 MCP resources** (read-only data: projects, issues, pipelines, MRs, branches, members, the surface-aware `gitlab://tools` manifest, and workflow best-practice guides).
- **37 MCP prompts** (code review, pipeline status, risk assessment, release notes, standup, analytics, audit, and more).
- **4 elicitation wizards** (interactive issue/MR/release/project creation).
- **3 MCP capabilities** (completions, progress, elicitation) and **50 SVG tool icons** for visual identification in MCP clients.
- **Pagination** on every list endpoint with full metadata.

</details>

## Tool surfaces

The server can present GitLab in three shapes, controlled by `TOOL_SURFACE`. The default needs no configuration.

| Surface                       | Visible tools                                     | Best for                                                         |
| ----------------------------- | ------------------------------------------------- | ---------------------------------------------------------------- |
| **Dynamic** (default)         | 2 (`gitlab_find_action`, `gitlab_execute_action`) | Lowest token cost; reaches the full catalog via find/execute.    |
| **Meta-tools** (`meta`)       | 32 base / 49 Ultimate / 50 GitLab.com Ultimate    | Domain-grouped dispatchers with an `action` parameter.           |
| **Individual** (`individual`) | ~847 Free/CE · ~999 Premium · 1065–1071 Ultimate  | One MCP tool per GitLab operation; needs a large context window. |

Tool counts scale with your GitLab edition (`GITLAB_TIER`); higher tiers expose more actions. See [Dynamic Toolset](docs/concepts/dynamic-tools.md) and [Meta-Tools Reference](docs/concepts/meta-tools.md) for the ranking model, safety guards, and full catalogs. For dynamic runs where resources dominate context, set `CAPABILITY_SURFACE=minimal`.

### Token Footprint

<!-- START TOKEN FOOTPRINT -->

Measured with `go run ./cmd/audit_tokens/ -footprint` against the current catalog. Totals estimate startup context visible to an MCP client: visible tool schemas plus shared resources and prompts, using the cl100k_base tokenizer (GPT-4/GPT-3.5 encoding). For the full matrix (meta and individual surfaces, all `META_PARAM_SCHEMA` modes), see [Token Footprint Reference](docs/development/token-footprint.md).

**Default configuration**: with `TOOL_SURFACE` unset or `TOOL_SURFACE=dynamic`, `CAPABILITY_SURFACE=full`, `META_TOOLS` unset, `META_PARAM_SCHEMA=opaque`, and `GITLAB_TIER` unset (detected, fallback `free`), the server uses the **dynamic find/execute surface**. Use `TOOL_SURFACE=meta` only when you explicitly want domain meta-tools; use `TOOL_SURFACE=individual` only when your client can handle the full tool catalog.

| Configuration (`TOOL_SURFACE` / `CAPABILITY_SURFACE`) | Tier     | Visible tools | Reachable actions | `META_PARAM_SCHEMA` | Tool schema tokens | Shared tokens | Total tokens |
| ----------------------------------------------------- | -------- | ------------: | ----------------: | ------------------- | -----------------: | ------------: | -----------: |
| `dynamic` / `full` (default)                          | Free/CE  |             2 |               851 | n/a                 |              2,180 |        31,758 |       33,938 |
| `dynamic` / `minimal`                                 | Free/CE  |             2 |               851 | n/a                 |              2,180 |         1,088 |        3,268 |
| `dynamic` / `full` (default)                          | Premium  |             2 |             1,003 | n/a                 |              2,180 |        31,758 |       33,938 |
| `dynamic` / `minimal`                                 | Premium  |             2 |             1,003 | n/a                 |              2,180 |         1,088 |        3,268 |
| `dynamic` / `full` (default)                          | Ultimate |             2 |             1,069 | n/a                 |              2,180 |        31,758 |       33,938 |
| `dynamic` / `minimal`                                 | Ultimate |             2 |             1,069 | n/a                 |              2,180 |         1,088 |        3,268 |

Rows use the base Community Edition catalog unless the Tier column says otherwise. `GITLAB_TIER` controls which actions are available; higher tiers expose more tools and thus more reachable actions.

<!-- END TOKEN FOOTPRINT -->

## Compatibility

| MCP Capability  | Support                            |
| --------------- | ---------------------------------- |
| **Tools**       | Up to 1071 individual / 32–50 meta |
| **Resources**   | 45 (static + templates)            |
| **Prompts**     | 37 templates                       |
| **Completions** | Project, user, group, branch, tag  |
| **Logging**     | Structured (text/JSON) to stderr   |
| **Progress**    | Tool execution progress reporting  |
| **Elicitation** | 4 interactive creation wizards     |

Tested with: VS Code + GitHub Copilot, Claude Desktop, Claude Code, Cursor, Windsurf, JetBrains IDEs, Zed, Kiro, Cline. See the full [Compatibility Matrix](https://jmrplens.github.io/gitlab-mcp-server/compatibility/).

## AI Model Tool-Use Evaluation

The project includes an automated evaluator for model-facing MCP quality. It runs schema-only checks against the tool catalog or executes validated model tool calls through MCP against Docker GitLab CE or licensed Enterprise instances populated with fixtures. It measures whether each model chooses the correct action, sends valid parameters, recovers from actionable GitLab errors, and respects destructive-action safeguards — across Anthropic, Google, OpenAI, and Qwen.

<!-- START MODEL EVAL DYNAMIC SUMMARY -->
Current published result: **Docker CE dynamic 20260627-232303**.

| Provider  | Model                       | Compatibility | Tool accuracy |      Recovery | Docker live status          |
| --------- | --------------------------- | ------------- | ------------: | ------------: | --------------------------- |
| Anthropic | `claude-haiku-4-5-20251001` | OK            |        100.0% |  100.0% (2/2) | 100.0% final across 555 ops |
| Google    | `gemini-flash-latest`       | OK            |        100.0% |  100.0% (4/4) | 100.0% final across 555 ops |
| OpenAI    | `gpt-5.4-nano`              | Review        |         99.3% | 84.6% (11/13) | 98.0% final across 555 ops  |
| Qwen      | `qwen3.6-flash`             | OK            |        100.0% |  100.0% (5/5) | 100.0% final across 555 ops |

The published model-evaluation set covers 596 task attempts and 2220 expected MCP operations. Across the selected reports, models emitted 2265 tool calls over 2265 model requests, with 99.5% aggregate final success. See [AI Model Evaluation Results](docs/development/testing/model-results.md) for the detailed current matrix.
<!-- END MODEL EVAL DYNAMIC SUMMARY -->

<details>
<summary>Enterprise meta &amp; dynamic evaluation results</summary>

<!-- START MODEL EVAL ENTERPRISE META SUMMARY -->
Current published result: **Docker Enterprise meta 20260527**.

| Provider  | Model                       | Compatibility | Tool accuracy |     Recovery | Docker live status         |
| --------- | --------------------------- | ------------- | ------------: | -----------: | -------------------------- |
| Anthropic | `claude-haiku-4-5-20251001` | OK            |        100.0% | 100.0% (1/1) | 100.0% final across 84 ops |
| Google    | `gemini-flash-latest`       | Review        |         78.2% | 100.0% (7/7) | 100.0% final across 84 ops |
| OpenAI    | `gpt-5.4-nano`              | Review        |        100.0% | 100.0% (4/4) | 100.0% final across 84 ops |
| Qwen      | `qwen3.6-flash`             | OK            |        100.0% | 100.0% (1/1) | 100.0% final across 84 ops |

The published model-evaluation set covers 92 task attempts and 336 expected MCP operations. Across the selected reports, models emitted 345 tool calls over 350 model requests, with 100.0% aggregate final success. See [AI Model Evaluation Results](docs/development/testing/model-results.md) for the detailed current matrix.
<!-- END MODEL EVAL ENTERPRISE META SUMMARY -->

<!-- START MODEL EVAL ENTERPRISE DYNAMIC SUMMARY -->
Current published result: **Docker Enterprise dynamic 20260628-015421**.

| Provider  | Model                       | Compatibility | Tool accuracy |     Recovery | Docker live status          |
| --------- | --------------------------- | ------------- | ------------: | -----------: | --------------------------- |
| Anthropic | `claude-haiku-4-5-20251001` | OK            |        100.0% | 100.0% (1/1) | 100.0% final across 202 ops |
| Google    | `gemini-flash-latest`       | OK            |        100.0% | 100.0% (2/2) | 100.0% final across 202 ops |
| OpenAI    | `gpt-5.4-nano`              | OK            |        100.0% |   No repairs | 100.0% final across 202 ops |
| Qwen      | `qwen3.6-flash`             | OK            |        100.0% | 100.0% (1/1) | 100.0% final across 202 ops |

The published model-evaluation set covers 124 task attempts and 808 expected MCP operations. Across the selected reports, models emitted 817 tool calls over 817 model requests, with 100.0% aggregate final success. See [AI Model Evaluation Results](docs/development/testing/model-results.md) for the detailed current matrix.
<!-- END MODEL EVAL ENTERPRISE DYNAMIC SUMMARY -->

</details>

## Documentation

Full documentation is at **[jmrplens.github.io/gitlab-mcp-server](https://jmrplens.github.io/gitlab-mcp-server/)**. Use this map for the source-of-truth reference on a specific area:

| Document                                              | Description                                                                            |
| ----------------------------------------------------- | -------------------------------------------------------------------------------------- |
| [Getting Started](docs/getting-started.md)            | Download, setup wizard, per-client configuration                                       |
| [IDE Configuration](docs/guides/ide-configuration.md) | Per-client stdio, HTTP legacy, and HTTP OAuth examples                                 |
| [Configuration](docs/reference/configuration.md)      | Environment variables, transport modes, TLS                                            |
| [Environment Variables](docs/reference/env.md)        | Exhaustive environment variable table with defaults and examples                       |
| [CLI Reference](docs/reference/cli.md)                | All command-line flags, exit codes, and runtime examples                               |
| [HTTP Server Mode](docs/guides/http-server-mode.md)   | Shared HTTP deployments, authentication, server pool isolation                         |
| [Tools Reference](docs/reference/tools/README.md)     | All individual tools with input/output schemas, including GitLab.com-only Orbit        |
| [Meta-Tools](docs/concepts/meta-tools.md)             | 32/48/49 domain meta-tools with action dispatching                                     |
| [Dynamic Toolset](docs/concepts/dynamic-tools.md)     | 2-tool low-token mode with canonical action catalog, safety model, and examples        |
| [Resources](docs/reference/resources.md)              | All 45 resources with URI templates                                                    |
| [Prompts](docs/reference/prompts.md)                  | All 37 prompts with arguments and output format                                        |
| [Auto-Update](docs/guides/auto-update.md)             | Self-update mechanism, modes, and release format                                       |
| [Testing](docs/development/testing/README.md)         | Unit, E2E, schema model evaluation, Docker model evaluation, and curated model results |
| [Security](docs/concepts/security.md)                 | Security model, token scopes, input validation                                         |
| [Architecture](docs/concepts/architecture.md)         | System architecture, component design, data flow                                       |
| [Development Guide](docs/development/development.md)  | Building, testing, CI/CD, contributing                                                 |
| [Troubleshooting](docs/guides/troubleshooting.md)     | Common startup, token, TLS, transport, and tool-discovery issues                       |

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

Both Community Edition (CE) and Enterprise Edition (EE). Set `GITLAB_TIER=premium` or `GITLAB_TIER=ultimate` in stdio mode to enable additional tools for Premium/Ultimate features (DORA metrics, vulnerabilities, compliance, etc.); leave it unset to detect the tier from the instance license (fallback `free`). In HTTP mode, `--tier` can force the tier, otherwise it is detected per token+URL pool entry from the license.
</details>

<details>
<summary><strong>How does it handle rate limiting?</strong></summary>

The server includes retry logic with backoff for GitLab API rate limits. Errors are classified as transient (retryable) or permanent, with actionable hints in error messages.
</details>

<details>
<summary><strong>Which AI clients are supported?</strong></summary>

Any MCP-compatible client: VS Code + GitHub Copilot, Claude Desktop, Cursor, Claude Code, Windsurf, JetBrains IDEs, Zed, Kiro, and others. The built-in setup wizard can auto-configure most clients.
</details>

## Building from Source

```bash
git clone https://github.com/jmrplens/gitlab-mcp-server.git
cd gitlab-mcp-server
make build
```

The published container image is `ghcr.io/jmrplens/gitlab-mcp-server:latest`. See the [Development Guide](docs/development/development.md) for cross-compilation, Docker Compose, and contributing guidelines.

| Component     | Technology                                       |
| ------------- | ------------------------------------------------ |
| Language      | Go 1.26+                                         |
| MCP SDK       | `github.com/modelcontextprotocol/go-sdk` v1.6.1  |
| GitLab Client | `gitlab.com/gitlab-org/api/client-go/v2` v2.51.0 |
| Transport     | stdio (default), HTTP (Streamable HTTP)          |

## Privacy Policy

The server runs entirely on your machine and has **no telemetry, analytics, or
backend of its own** — data flows only between your MCP client and the GitLab
instance you configure (plus an optional signed-binary update check against
GitHub Releases). Your token is used solely to authenticate GitLab requests
and is never logged. Full details: [PRIVACY.md](PRIVACY.md).

## Contributing & Security

- **Contributing**: see [CONTRIBUTING.md](CONTRIBUTING.md) for development guidelines, branch naming, commit conventions, and the PR process.
- **Security**: see [SECURITY.md](SECURITY.md) for the security policy and vulnerability reporting.
- **Code of Conduct**: see [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) ([Contributor Covenant v2.1](https://www.contributor-covenant.org/version/2/1/code_of_conduct/)).

> **Repository mirror**: GitHub is the canonical repository. A read-only mirror is available on [GitLab.com](https://gitlab.com/jmrp/gitlab-mcp-server) for discoverability; please open contributions on GitHub.

<details>
<summary><strong>Unnecessary statistics</strong> — numbers nobody asked for</summary>

<!-- START STATS -->

### File counts

| Category                 |     Files |       Lines |
| ------------------------ | --------: | ----------: |
| Source (`.go`, non-test) |       966 |     192,969 |
| Unit tests (`_test.go`)  |       535 |     298,256 |
| End-to-end tests         |       169 |      43,893 |
| **Total**                | **1,670** | **535,118** |

### Functions

| Category                        |  Count |
| ------------------------------- | -----: |
| Source functions                |  7,394 |
| — exported (public)             |  2,590 |
| — unexported (private)          |  4,804 |
| Unit test functions (`TestXxx`) | 11,552 |
| Subtests (`t.Run(...)`)         |  2,887 |
| End-to-end test functions       |    376 |

### Ratios worth noting

| Observation                        |                      Value |
| ---------------------------------- | -------------------------: |
| Test lines vs source lines         | 1.55× more tests than code |
| Average source file length         |                 ~199 lines |
| Average test file length           |                 ~557 lines |
| Comment lines in source            |  20,989 (~10.9% of source) |
| Test functions per source function |                       1.6× |

### Code patterns

| Pattern                            | Count |
| ---------------------------------- | ----: |
| `if err != nil` checks             | 6,613 |
| `defer` statements                 |   828 |
| `struct` types defined             | 2,711 |
| `//nolint` suppressions            |   206 |
| `TODO` / `FIXME` / `HACK` comments |     3 |

### Project

| Metric                         | Value |
| ------------------------------ | ----: |
| Go packages                    |   227 |
| Direct dependencies (`go.mod`) |    13 |
| Indirect dependencies          |    50 |
| Git commits                    |   256 |
| Unique contributors            |     4 |

### Hall of fame

| Record              | File                                                     |
| ------------------- | -------------------------------------------------------- |
| Longest source file | `internal/tools/projects/projects.go` — 3,827 lines      |
| Longest test file   | `internal/tools/projects/projects_test.go` — 8,095 lines |

### Because why not

| Fact                                 | Value                                                                                                |
| ------------------------------------ | ---------------------------------------------------------------------------------------------------- |
| Source code printed at 55 lines/page | ~3,508 pages of A4                                                                                   |
| Source lines mentioning `"gitlab"`   | 12,489 (impossible to avoid)                                                                         |
| Longest function name in source      | `assertDynamicCompatibilityPolicyOwnedByActionCompat` (51 chars)                                     |
| Longest test function name           | `TestRequiredMissingAndUnknownParamNames_SchemaValidation_ReturnsSortedMissingAndUnknown` (87 chars) |

<!-- END STATS -->

</details>
