<p align="center">
  <img alt="GitLab MCP Server — GitLab for your AI assistant: one action catalog, three MCP tool surfaces" src="https://raw.githubusercontent.com/jmrplens/gitlab-mcp-server/main/.github/brand/banner.webp" width="100%">
</p>

# GitLab MCP Server

<p align="center">

<!-- Package -->

[![GitHub Release](https://img.shields.io/github/v/release/jmrplens/gitlab-mcp-server?style=flat&logo=github&label=Release)](https://github.com/jmrplens/gitlab-mcp-server/releases/latest)
[![npm](https://img.shields.io/npm/v/@jmrp.io/gitlab-mcp-server?style=flat&logo=npm&label=npm)](https://www.npmjs.com/package/@jmrp.io/gitlab-mcp-server)
[![PyPI](https://img.shields.io/pypi/v/jmrplens-gitlab-mcp-server?style=flat&logo=pypi&label=PyPI)](https://pypi.org/project/jmrplens-gitlab-mcp-server/)
[![NuGet](https://img.shields.io/nuget/v/gitlab-mcp-server?style=flat&logo=nuget&label=NuGet)](https://www.nuget.org/packages/gitlab-mcp-server)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
![Platform](https://img.shields.io/badge/Windows%20%7C%20Linux%20%7C%20macOS-amd64%20%26%20arm64-lightgrey?style=flat&logo=windows-terminal&logoColor=white)
<!-- Quality -->

[![CI](https://img.shields.io/github/actions/workflow/status/jmrplens/gitlab-mcp-server/ci.yml?branch=main&style=flat&logo=githubactions&logoColor=white&label=CI)](https://github.com/jmrplens/gitlab-mcp-server/actions/workflows/ci.yml)
[![Quality Gate](https://sonarcloud.io/api/project_badges/measure?project=jmrplens_gitlab-mcp-server&metric=alert_status)](https://sonarcloud.io/summary/overall?id=jmrplens_gitlab-mcp-server)
[![Coverage](https://sonarcloud.io/api/project_badges/measure?project=jmrplens_gitlab-mcp-server&metric=coverage)](https://sonarcloud.io/summary/overall?id=jmrplens_gitlab-mcp-server)
[![Go Reference](https://pkg.go.dev/badge/github.com/jmrplens/gitlab-mcp-server/v3.svg)](https://pkg.go.dev/github.com/jmrplens/gitlab-mcp-server/v3)

</p>

<p align="center">

<!-- Listings -->

[![Glama MCP Score](https://glama.ai/mcp/servers/jmrplens/gitlab-mcp-server/badges/score.svg)](https://glama.ai/mcp/servers/jmrplens/gitlab-mcp-server)
<!-- The ?v= is a cache key for GitHub's image proxy, not a LobeHub parameter: the
     proxy keyed a red "not listed" badge for a day, and only a changed URL evicts
     it. Bump the number if the badge ever goes stale again. -->
[![MCP Badge](https://lobehub.com/badge/mcp/jmrplens-gitlab-mcp-server?v=2)](https://lobehub.com/mcp/jmrplens-gitlab-mcp-server)
[![MCP Toplist](https://mcptoplist.com/badge/io.github.jmrplens%2Fgitlab-mcp-server.svg)](https://mcptoplist.com/server/io.github.jmrplens%2Fgitlab-mcp-server)
[![Cursor Directory](https://img.shields.io/badge/Cursor-Directory-000000?logo=cursor&logoColor=white)](https://cursor.directory/plugins/gitlab-mcp-server)
[![Hosted endpoint](https://img.shields.io/badge/Hosted-mcp.jmrp.io%2Fgitlab-6d28d9?style=flat&logo=icloud&logoColor=white)](https://mcp.jmrp.io/)

</p>

**Connect your AI assistant to GitLab so it can review merge requests, triage pipelines, manage issues, and draft releases — in plain language.** One static binary (or a container), [1000+ GitLab tools](#tool-surfaces) over the full REST + GraphQL API, working with Claude, Cursor, VS Code, and any MCP client.

You talk to your AI assistant; it does the GitLab work. No project IDs, API endpoints, or JSON to remember.

<!-- START TOKEN CLAIM -->

**10,359 tokens of startup context by default, the same on every GitLab tier (1,694 with `GITLAB_MCP_CAPABILITY_SURFACE=minimal`).** Two tools reach the whole catalog; measured with the cl100k_base tokenizer and verified in CI on every commit. [How it is measured](#token-footprint)

<!-- END TOKEN CLAIM -->

> "Review merge request !15 — is it safe to merge?" · "Why did the last pipeline fail?" · "List open issues assigned to me" · "Generate release notes from v1.0 to v2.0"

---

> 🤖 **Using an AI assistant?** Give it this repository URL and ask it to install the server for your client. Everything a model needs to do it headlessly — the declarative per-client config, `claude mcp add` one-liners, and defaults — is in [`llms.txt`](llms.txt) (no interactive wizard required).

## Install in 60 seconds

Pick one. Each path ends with you typing a prompt to your assistant. Every channel has a full guide: [Installation](https://jmrp.io/docs/gitlab-mcp-server/install/overview/).

> **Want to look before installing?** The [browser inspector](https://mcp.jmrp.io/inspector/?server=gitlab) signs in with OAuth and calls the [hosted endpoint](#try-it-without-installing-anything-hosted-endpoint) read-only from a browser tab — nothing downloaded. Running it yourself is still the way to keep using it.

### One-click install

<table>
  <tr>
    <th align="left">Client</th>
    <th align="left">One-click button</th>
    <th align="left">Token step</th>
  </tr>
  <tr>
    <td><b>VS Code</b></td>
    <td><a href="https://insiders.vscode.dev/redirect/mcp/install?name=gitlab&amp;config=%7B%22command%22%3A%22docker%22%2C%22args%22%3A%5B%22run%22%2C%22-i%22%2C%22--rm%22%2C%22-e%22%2C%22GITLAB_TOKEN%22%2C%22ghcr.io%2Fjmrplens%2Fgitlab-mcp-server%3Alatest%22%5D%2C%22env%22%3A%7B%22GITLAB_TOKEN%22%3A%22%24%7Binput%3Agitlab_token%7D%22%7D%2C%22inputs%22%3A%5B%7B%22id%22%3A%22gitlab_token%22%2C%22type%22%3A%22promptString%22%2C%22description%22%3A%22GitLab%20Personal%20Access%20Token%20%28api%20scope%29%22%2C%22password%22%3Atrue%7D%5D%7D"><img alt="Install in VS Code" src="https://img.shields.io/badge/Install_in-VS_Code-0098FF?style=flat-square&amp;logo=visualstudiocode&amp;logoColor=white" /></a></td>
    <td>prompts you (masked)</td>
  </tr>
  <tr>
    <td><b>VS Code Insiders</b></td>
    <td><a href="https://insiders.vscode.dev/redirect/mcp/install?name=gitlab&amp;config=%7B%22command%22%3A%22docker%22%2C%22args%22%3A%5B%22run%22%2C%22-i%22%2C%22--rm%22%2C%22-e%22%2C%22GITLAB_TOKEN%22%2C%22ghcr.io%2Fjmrplens%2Fgitlab-mcp-server%3Alatest%22%5D%2C%22env%22%3A%7B%22GITLAB_TOKEN%22%3A%22%24%7Binput%3Agitlab_token%7D%22%7D%2C%22inputs%22%3A%5B%7B%22id%22%3A%22gitlab_token%22%2C%22type%22%3A%22promptString%22%2C%22description%22%3A%22GitLab%20Personal%20Access%20Token%20%28api%20scope%29%22%2C%22password%22%3Atrue%7D%5D%7D&amp;quality=insiders"><img alt="Install in VS Code Insiders" src="https://img.shields.io/badge/Install_in-VS_Code_Insiders-24bfa5?style=flat-square&amp;logo=visualstudiocode&amp;logoColor=white" /></a></td>
    <td>prompts you (masked)</td>
  </tr>
  <tr>
    <td><b>Cursor</b></td>
    <td><a href="https://cursor.com/install-mcp?name=gitlab&amp;config=eyJjb21tYW5kIjoiZG9ja2VyIiwiYXJncyI6WyJydW4iLCItaSIsIi0tcm0iLCItZSIsIkdJVExBQl9UT0tFTiIsImdoY3IuaW8vam1ycGxlbnMvZ2l0bGFiLW1jcC1zZXJ2ZXI6bGF0ZXN0Il0sImVudiI6eyJHSVRMQUJfVE9LRU4iOiJZT1VSX0dJVExBQl9UT0tFTiJ9fQ%3D%3D"><img alt="Install in Cursor" src="https://cursor.com/deeplink/mcp-install-dark.svg" height="28" /></a></td>
    <td>edit <code>YOUR_GITLAB_TOKEN</code></td>
  </tr>
  <tr>
    <td><b>LM Studio</b></td>
    <td><a href="https://lmstudio.ai/install-mcp?name=gitlab&amp;config=eyJjb21tYW5kIjoiZG9ja2VyIiwiYXJncyI6WyJydW4iLCItaSIsIi0tcm0iLCItZSIsIkdJVExBQl9UT0tFTiIsImdoY3IuaW8vam1ycGxlbnMvZ2l0bGFiLW1jcC1zZXJ2ZXI6bGF0ZXN0Il0sImVudiI6eyJHSVRMQUJfVE9LRU4iOiJZT1VSX0dJVExBQl9UT0tFTiJ9fQ%3D%3D"><img alt="Add to LM Studio" src="https://files.lmstudio.ai/deeplink/mcp-install-dark.svg" height="28" /></a></td>
    <td>edit <code>YOUR_GITLAB_TOKEN</code></td>
  </tr>
  <tr>
    <td><b>Kiro</b></td>
    <td><a href="https://kiro.dev/launch/mcp/add?name=gitlab&amp;config=%7B%22command%22%3A%22docker%22%2C%22args%22%3A%5B%22run%22%2C%22-i%22%2C%22--rm%22%2C%22-e%22%2C%22GITLAB_TOKEN%22%2C%22ghcr.io%2Fjmrplens%2Fgitlab-mcp-server%3Alatest%22%5D%2C%22env%22%3A%7B%22GITLAB_TOKEN%22%3A%22YOUR_GITLAB_TOKEN%22%7D%7D"><img alt="Add to Kiro" src="https://kiro.dev/images/add-to-kiro.svg" height="28" /></a></td>
    <td>edit <code>YOUR_GITLAB_TOKEN</code></td>
  </tr>
  <tr>
    <td><b>Claude Desktop</b></td>
    <td><a href="https://github.com/jmrplens/gitlab-mcp-server/releases/latest/download/gitlab-mcp-server.mcpb"><img alt="Download .mcpb extension" src="https://img.shields.io/badge/Download-.mcpb_extension-d97757?style=flat-square&amp;logo=claude&amp;logoColor=white" /></a></td>
    <td>settings UI</td>
  </tr>
</table>

Each button registers the **Docker**-based server (auto-pulls the image on first run; you need [Docker](https://www.docker.com/) installed). The **Claude Desktop** row instead downloads a native [.mcpb desktop extension](docs/guides/claude-desktop-extension.md) (macOS universal, Windows, and Linux amd64 and arm64; no Docker). Open it with Claude Desktop, or on Linux use **Extensions > Install Extension...**, and fill in the settings. Need a token? [Create a Personal Access Token](https://docs.gitlab.com/ee/user/profile/personal_access_tokens.html) with the **`api`** scope. Self-managed GitLab? Add a `GITLAB_URL` env var in your client's MCP config after install.

### Claude Code (`claude mcp add`)

Docker (no install — pulls the image on first run):

```bash
claude mcp add gitlab --env GITLAB_TOKEN=glpat-xxxx --transport stdio \
  -- docker run -i --rm -e GITLAB_TOKEN ghcr.io/jmrplens/gitlab-mcp-server:latest
```

Or install the native binary first, then register it:

```bash
# Any platform (npm/pnpm) — downloads only your platform's prebuilt binary
npx -y @jmrp.io/gitlab-mcp-server          # zero install; clients launch it directly
npm install -g @jmrp.io/gitlab-mcp-server  # or install globally (npm)
pnpm add -g @jmrp.io/gitlab-mcp-server     # or globally (pnpm)
# Any platform (Python: uv/pipx/pip) — platform wheel carrying the same native binary
uvx jmrplens-gitlab-mcp-server             # zero install; clients launch it directly
pipx install jmrplens-gitlab-mcp-server    # or install globally (pipx)
pip install jmrplens-gitlab-mcp-server     # or into the active environment (pip)
# Linux wheels need glibc; on musl systems such as Alpine use the Docker image instead
# Any platform (.NET 10 SDK) — a .NET tool whose entry point is the same native binary
dnx gitlab-mcp-server                      # zero install; clients launch it directly
dotnet tool install -g gitlab-mcp-server   # or install globally (dotnet tool)
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

Clients that launch servers with `npx`, `uvx` or `dnx` need no install at all — point them at
`npx -y @jmrp.io/gitlab-mcp-server`, `uvx jmrplens-gitlab-mcp-server` or `dnx gitlab-mcp-server`.

Self-managed GitLab? Add `--env GITLAB_URL=https://gitlab.example.com` (and, for a self-signed certificate, mount the CA and set `--env SSL_CERT_FILE=/path/to/ca-bundle.crt`; `GITLAB_MCP_SKIP_TLS_VERIFY=true` is the blunt alternative, and OAuth mode refuses it for a non-loopback instance).

### Run it once to check the install

Started in a terminal, or double-clicked on Windows, with no `GITLAB_TOKEN` set,
the binary prints what it is and what it needs and waits for Enter, so you can
confirm the install before configuring anything. Configuration itself lives in
your MCP client's JSON, below.

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
"args": ["run", "-i", "--rm", "-e", "GITLAB_TOKEN", "ghcr.io/jmrplens/gitlab-mcp-server:latest"]
```

Cline (VS Code) — open the Cline sidebar → MCP servers icon → **Edit Global MCP**, or edit the settings file directly:

- **macOS**: `~/Library/Application Support/Code/User/globalStorage/saoudrizwan.claude-dev/settings/cline_mcp_settings.json`
- **Linux**: `~/.config/Code/User/globalStorage/saoudrizwan.claude-dev/settings/cline_mcp_settings.json`
- **Windows**: `%APPDATA%\Code\User\globalStorage\saoudrizwan.claude-dev\settings\cline_mcp_settings.json`

Cline uses the `mcpServers` shape shown above for the native binary.

For a shared, long-running HTTP deployment instead of per-user stdio, see [HTTP Server Mode](docs/guides/http-server-mode.md).

</details>

### Try it without installing anything (hosted endpoint)

A public instance runs at **`https://mcp.jmrp.io/gitlab`** — nothing to install, no account beyond your own GitLab token. Point any HTTP-capable MCP client at it:

```json
{
  "mcpServers": {
    "gitlab": {
      "type": "http",
      "url": "https://mcp.jmrp.io/gitlab",
      "headers": { "Authorization": "Bearer glpat-xxxxxxxxxxxx" }
    }
  }
}
```

The endpoint runs in OAuth mode, so the credential travels as `Authorization: Bearer` — a GitLab personal access token works there, verified exactly like an OAuth one, which is what keeps clients with no OAuth flow (and headless use) working. It travels per request and is never stored on the server. A client that speaks the OAuth flow needs no header at all: the `401` carries an RFC 9728 challenge it follows to authorize in the browser. `PRIVATE-TOKEN` is the legacy-mode header and is **not** accepted here; the instance is fixed to `https://gitlab.com`, so `GITLAB-URL` is ignored.

A `read_api` token is accepted and served a read-only tool surface — the write check is per action, so a credential that cannot break anything is a supported way to use the endpoint rather than a rejected one.

Two pages make it easier still. The [server card](https://mcp.jmrp.io/servers/gitlab/) lists the whole catalog with no credential at all and carries copy-paste config for Claude Code, Cursor and VS Code — including the OAuth client ID those clients need. The [browser inspector](https://mcp.jmrp.io/inspector/?server=gitlab) calls the same endpoint read-only from a browser tab: sign in with OAuth, pick a tool, read the raw JSON-RPC it returns — nothing installed.

It is the fastest way to try the server, and the right way to keep using it is still **locally** (any option above) — for one concrete reason, not as a disclaimer: **your token and every request pass through someone else's machine.** Running it locally means your credentials and your GitLab traffic never leave your computer, which also makes it the only sensible option for a private self-managed instance.

The endpoint is **stateless streamable HTTP** on the default `dynamic` surface: `POST` is the transport and an _authenticated_ `GET` answers `405` by design; with no credential, any method answers `401` carrying the RFC 6750 challenge an OAuth client follows — a bare `curl` that gets `401` is the endpoint working, not failing. `https://mcp.jmrp.io/gitlab/health` needs no credential and answers `200` with `{"status":"ok",…}`. A self-hosted HTTP deployment can also run `--auth-mode=oauth --gitlab-url=https://gitlab.com --public-url=https://mcp.example.com/mcp` (both are required: OAuth needs a fixed instance, and `--public-url` is the RFC 9728 resource identifier — pass exactly the URL your clients are configured with, since a client discards metadata naming a different one), where clients discover GitLab as the authorization server through that metadata and authorize in the browser instead of copying tokens — see [OAuth App Setup](docs/guides/oauth-app-setup.md). It is one of the servers listed at **[mcp.jmrp.io](https://mcp.jmrp.io/)**, a directory of the MCP servers I maintain, each reachable at its own endpoint; [`https://mcp.jmrp.io/servers.json`](https://mcp.jmrp.io/servers.json) is the same list for automated clients.

It is a personal service, run by one person and offered as-is: no SLA, no support channel, and no promise it is unchanged next week. It adds no quota of its own — every call spends GitLab.com's own limits, under your own token. And it moves on its own, normally to the newest release, so what it serves is never a pinned version.

**Then just ask:** open your AI client and try _"List my GitLab projects."_ See the [Getting Started guide](https://jmrp.io/docs/gitlab-mcp-server/getting-started/) for per-client details and [more example prompts](docs/guides/examples/usage-examples.md).

---

## Why this server

- **Plain-language GitLab.** The AI translates "is MR !15 safe to merge?" into the right API calls. You don't touch endpoints, IDs, or JSON.
- **The whole platform — [1000+ tools](#tool-surfaces).** Broad GitLab REST v4 + GraphQL coverage: projects, branches, tags, releases, merge requests, issues, pipelines, jobs, groups, users, wikis, environments, deployments, packages, container registry, runners, feature flags, CI/CD variables, security, admin, tokens, and more.
- **Low-token by default.** The default **dynamic** surface exposes just 2 tools (`find` + `execute`) while reaching the full catalog — so it fits any client's context window. ([Token footprint →](#token-footprint))
- **Safe by design.** Read-only mode, safe mode (dry-run preview of every mutation), TLS options for self-hosted GitLab, and continuous [SonarCloud](https://sonarcloud.io/summary/overall?id=jmrplens_gitlab-mcp-server) quality/security gates.
- **Runs anywhere.** One static binary or container; Windows, Linux & macOS; amd64 & arm64; stdio (desktop) and HTTP (remote).

<details>
<summary>More: resources, prompts, and capabilities</summary>

- **45 MCP resources** (read-only data: projects, issues, pipelines, MRs, branches, members, the surface-aware `gitlab://tools` manifest, and workflow best-practice guides). 26 resource kinds, single objects plus three single-parent lists, are also [subscribable](docs/reference/capabilities/subscriptions.md).
- **37 MCP prompts** (code review, pipeline status, risk assessment, release notes, standup, analytics, audit, and more).
- **4 elicitation wizards** (interactive issue/MR/release/project creation).
- **4 MCP capabilities** (completions, progress, elicitation, and [resource subscriptions](docs/reference/capabilities/subscriptions.md) — live `resources/updated` notifications, honored by polling) and **51 tool icons** (50 domain icons plus the project mark) for visual identification in MCP clients.
- **Pagination** on every list endpoint with full metadata.

</details>

## Tool surfaces

The server can present GitLab in three shapes, controlled by `GITLAB_MCP_TOOL_SURFACE`. The default needs no configuration.

| Surface                       | Visible tools                                     | Best for                                                         |
| ----------------------------- | ------------------------------------------------- | ---------------------------------------------------------------- |
| **Dynamic** (default)         | 2 (`gitlab_find_action`, `gitlab_execute_action`) | Lowest token cost; reaches the full catalog via find/execute.    |
| **Meta-tools** (`meta`)       | 34 base / 51 Ultimate / 52 GitLab.com Ultimate    | Domain-grouped dispatchers with an `action` parameter.           |
| **Individual** (`individual`) | ~866 Free/CE · ~1020 Premium · 1086–1092 Ultimate | One MCP tool per GitLab operation; needs a large context window. |

Tool counts scale with your GitLab edition (`GITLAB_MCP_TIER`); higher tiers expose more actions. See [Dynamic Toolset](docs/concepts/dynamic-tools.md) and [Meta-Tools Reference](docs/concepts/meta-tools.md) for the ranking model, safety guards, and full catalogs. For dynamic runs where resources dominate context, set `GITLAB_MCP_CAPABILITY_SURFACE=minimal`.

### Token Footprint

<!-- START TOKEN FOOTPRINT -->

Measured with `go run ./cmd/audit_tokens/ -footprint` against the current catalog. Totals estimate startup context visible to an MCP client: visible tool schemas plus shared resources and prompts, using the cl100k_base tokenizer (GPT-4/GPT-3.5 encoding). For the full matrix (meta and individual surfaces, all `GITLAB_MCP_META_PARAM_SCHEMA` modes), see [Token Footprint Reference](docs/development/token-footprint.md).

**Default configuration**: with `GITLAB_MCP_TOOL_SURFACE` unset or `GITLAB_MCP_TOOL_SURFACE=dynamic`, `GITLAB_MCP_CAPABILITY_SURFACE=full`, `GITLAB_MCP_META_PARAM_SCHEMA=opaque`, and `GITLAB_MCP_TIER` unset (detected, fallback `free`), the server uses the **dynamic find/execute surface**. Use `GITLAB_MCP_TOOL_SURFACE=meta` only when you explicitly want domain meta-tools; use `GITLAB_MCP_TOOL_SURFACE=individual` only when your client can handle the full tool catalog.

| Configuration (`GITLAB_MCP_TOOL_SURFACE` / `GITLAB_MCP_CAPABILITY_SURFACE`) | Tier     | Visible tools | Reachable actions | `GITLAB_MCP_META_PARAM_SCHEMA` | Tool schema tokens | Shared tokens | Total tokens |
| --------------------------------------------------------------------------- | -------- | ------------: | ----------------: | ------------------------------ | -----------------: | ------------: | -----------: |
| `dynamic` / `full` (default)                                                | Free/CE  |             2 |               870 | n/a                            |              1,524 |         8,835 |       10,359 |
| `dynamic` / `minimal`                                                       | Free/CE  |             2 |               870 | n/a                            |              1,524 |           170 |        1,694 |
| `dynamic` / `full` (default)                                                | Premium  |             2 |             1,024 | n/a                            |              1,524 |         8,835 |       10,359 |
| `dynamic` / `minimal`                                                       | Premium  |             2 |             1,024 | n/a                            |              1,524 |           170 |        1,694 |
| `dynamic` / `full` (default)                                                | Ultimate |             2 |             1,090 | n/a                            |              1,524 |         8,835 |       10,359 |
| `dynamic` / `minimal`                                                       | Ultimate |             2 |             1,090 | n/a                            |              1,524 |           170 |        1,694 |

Rows use the base Community Edition catalog unless the Tier column says otherwise. `GITLAB_MCP_TIER` controls which actions are available; higher tiers expose more tools and thus more reachable actions.

<!-- END TOKEN FOOTPRINT -->

## Compatibility

| MCP Capability    | Support                                                                                                                             |
| ----------------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| **Tools**         | Up to 1092 individual / 34–52 meta                                                                                                  |
| **Resources**     | 45 (static + templates)                                                                                                             |
| **Prompts**       | 37 templates                                                                                                                        |
| **Completions**   | 18 argument names, among them projects, groups, users, branches, tags, MRs, issues, pipelines, jobs, labels, milestones and SHAs    |
| **Server logs**   | Structured (text/JSON) to stderr — not the MCP `logging` capability, which is deprecated (SEP-2577) and deliberately not advertised |
| **Progress**      | Tool execution progress reporting                                                                                                   |
| **Elicitation**   | 4 interactive creation wizards                                                                                                      |
| **Subscriptions** | `resources/updated` by polling, 26 resource kinds                                                                                   |

Tested with: VS Code + GitHub Copilot, Claude Desktop, Claude Code, Cursor, Windsurf, JetBrains IDEs, Zed, Kiro, Cline. See the full [Compatibility Matrix](https://jmrp.io/docs/gitlab-mcp-server/compatibility/).

## AI Model Tool-Use Evaluation

The project includes an automated evaluator for model-facing MCP quality, and **it currently publishes no result.** Every figure this section used to carry has been withdrawn, including the 99.5% aggregate success this README led with.

They were withdrawn because the measurement did not measure what it claimed. One struct fed the stimulus the model was given, the environment it acted in, the scorer that graded it and the report at the same time, so parts of the corpus put the expected call in the prompt the scorer then checked against, repairs were made from an answer key the harness supplied, and the scorer compared parameter names rather than their values. A run could not have failed for the reasons it was meant to catch. The tables as they last stood can be read at commit `4587cbfb3`; the account of what was wrong with them is kept in [AI Model Evaluation Results](docs/development/testing/model-results.md).

The replacement is being built as a tagged package under `test/e2e/` on the end-to-end harness, which already keeps the stimulus, the environment and the record apart. Numbers return here when that harness produces them.

<!-- START MODEL EVAL DYNAMIC SUMMARY -->

Withdrawn. The CE dynamic table published here, last refreshed from a Docker run dated 20260627-232303, is readable at commit `4587cbfb3` and is not reproduced because the measurement behind it was unsound.
<!-- END MODEL EVAL DYNAMIC SUMMARY -->

<details>
<summary>Meta-tools and Enterprise evaluation results</summary>

<!-- START MODEL EVAL META SUMMARY -->

Withdrawn. No CE meta-tools table was ever published here, and none will be until the rebuilt harness produces one.
<!-- END MODEL EVAL META SUMMARY -->

<!-- START MODEL EVAL ENTERPRISE META SUMMARY -->

Withdrawn. The Enterprise meta table published here, last refreshed from a Docker run dated 20260527, is readable at commit `4587cbfb3` and is not reproduced because the measurement behind it was unsound.
<!-- END MODEL EVAL ENTERPRISE META SUMMARY -->

<!-- START MODEL EVAL ENTERPRISE DYNAMIC SUMMARY -->

Withdrawn. The Enterprise dynamic table published here, last refreshed from a Docker run dated 20260628-015421, is readable at commit `4587cbfb3` and is not reproduced because the measurement behind it was unsound.
<!-- END MODEL EVAL ENTERPRISE DYNAMIC SUMMARY -->

</details>

## Documentation

Full documentation is at **[jmrp.io/docs/gitlab-mcp-server](https://jmrp.io/docs/gitlab-mcp-server)**. Use this map for the source-of-truth reference on a specific area:

| Document                                              | Description                                                                                                                                     |
| ----------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| [Getting Started](docs/getting-started.md)            | Install paths, first query, per-client configuration                                                                                            |
| [Installation](docs/guides/installation.md)           | Every install channel (binary, Homebrew, winget, Docker, npm, PyPI, NuGet, `.mcpb`, Agent Plugins, hosted), verification, upgrade and uninstall |
| [IDE Configuration](docs/guides/ide-configuration.md) | Per-client stdio, HTTP legacy, and HTTP OAuth examples                                                                                          |
| [Configuration](docs/reference/configuration.md)      | Environment variables, transport modes, TLS                                                                                                     |
| [Environment Variables](docs/reference/env.md)        | Exhaustive environment variable table with defaults and examples                                                                                |
| [CLI Reference](docs/reference/cli.md)                | All command-line flags, exit codes, and runtime examples                                                                                        |
| [HTTP Server Mode](docs/guides/http-server-mode.md)   | Shared HTTP deployments, authentication, server pool isolation                                                                                  |
| [OAuth App Setup](docs/guides/oauth-app-setup.md)     | GitLab OAuth application, scopes, redirect URIs, and which clients can complete a flow                                                          |
| [CI/CD](docs/guides/ci-cd.md)                         | Running the server inside GitLab CI and GitHub Actions pipelines                                                                                |
| [Output Format](docs/reference/output-format.md)      | The response contract every tool follows: content blocks, pagination, next steps                                                                |
| [Error Handling](docs/concepts/error-handling.md)     | Error classification, GitLab message extraction, and the hints tools return                                                                     |
| [Tools Reference](docs/reference/tools/README.md)     | All individual tools with input/output schemas, including GitLab.com-only Orbit                                                                 |
| [Meta-Tools](docs/concepts/meta-tools.md)             | 34/51/52 domain meta-tools with action dispatching                                                                                              |
| [Dynamic Toolset](docs/concepts/dynamic-tools.md)     | 2-tool low-token mode with canonical action catalog, safety model, and examples                                                                 |
| [Resources](docs/reference/resources.md)              | All 45 resources with URI templates                                                                                                             |
| [Prompts](docs/reference/prompts.md)                  | All 37 prompts with arguments and output format                                                                                                 |
| [Testing](docs/development/testing/README.md)         | Unit, E2E, schema model evaluation, Docker model evaluation, and curated model results                                                          |
| [Security](docs/concepts/security.md)                 | Security model, token scopes, input validation                                                                                                  |
| [Architecture](docs/concepts/architecture.md)         | System architecture, component design, data flow                                                                                                |
| [Development Guide](docs/development/development.md)  | Building, testing, CI/CD, contributing                                                                                                          |
| [Troubleshooting](docs/guides/troubleshooting.md)     | Common startup, token, TLS, transport, and tool-discovery issues                                                                                |

## FAQ

<details>
<summary><strong>Does it work with self-hosted GitLab?</strong></summary>

Yes. Set `GITLAB_URL` to your instance URL. When `GITLAB_URL` is omitted, stdio mode uses `https://gitlab.com`. Self-signed TLS certificates are supported by installing the CA in the system trust store or pointing `SSL_CERT_FILE` at a bundle; `GITLAB_MCP_SKIP_TLS_VERIFY=true` skips verification instead, and `--auth-mode=oauth` refuses it for a non-loopback instance.
</details>

<details>
<summary><strong>Is my data safe?</strong></summary>

When you run it yourself, locally over stdio or on your own infrastructure over HTTP, every request goes to your GitLab instance and nowhere else. There is no update check, no license check and no telemetry: your instance is the only host this server contacts.

The exception is the <a href="#try-it-without-installing-anything-hosted-endpoint">hosted endpoint</a>: using <code>https://mcp.jmrp.io/gitlab</code> means your token and every request pass through that machine. Nothing is stored there, but it is someone else's server, which is why the hosted section says to keep using it locally.

See <a href="PRIVACY.md">PRIVACY.md</a> for the full data-flow statement, and <a href="SECURITY.md">SECURITY.md</a> for the security model.
</details>

<details>
<summary><strong>Can I use it in read-only mode?</strong></summary>

Yes. Set `GITLAB_MCP_READ_ONLY=true` to disable all mutating tools (create, update, delete). Only read operations will be available.

Alternatively, set `GITLAB_MCP_SAFE_MODE=true` for a dry-run mode: mutating tools remain visible but return a structured JSON preview instead of executing. Useful for auditing, training, or reviewing what an AI assistant would do.
</details>

<details>
<summary><strong>What GitLab editions are supported?</strong></summary>

Both Community Edition (CE) and Enterprise Edition (EE). Set `GITLAB_MCP_TIER=premium` or `GITLAB_MCP_TIER=ultimate` in stdio mode to enable additional tools for Premium/Ultimate features (DORA metrics, vulnerabilities, compliance, etc.); leave it unset to detect the tier from the instance license (fallback `free`). In HTTP mode, `--tier` can force the tier, otherwise it is detected per token+URL pool entry from the license.
</details>

<details>
<summary><strong>How does it handle rate limiting?</strong></summary>

The server includes retry logic with backoff for GitLab API rate limits. Errors are classified as transient (retryable) or permanent, with actionable hints in error messages.
</details>

<details>
<summary><strong>Which AI clients are supported?</strong></summary>

Any MCP-compatible client: VS Code + GitHub Copilot, Claude Desktop, Cursor, Claude Code, Windsurf, JetBrains IDEs, Zed, Kiro, and others. Each one's configuration snippet is in [Getting Started](docs/getting-started.md), and the one-click buttons above cover the most common ones.
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
| Language      | Go 1.27+                                         |
| MCP SDK       | `github.com/modelcontextprotocol/go-sdk` v1.8.0  |
| GitLab Client | `gitlab.com/gitlab-org/api/client-go/v3` v3.14.0 |
| Transport     | stdio (default), HTTP (Streamable HTTP)          |

## Privacy Policy

The server runs entirely on your machine and has **no telemetry, analytics, or
backend of its own** — data flows only between your MCP client and the GitLab
instance you configure (plus an optional signed-binary update check against
GitHub Releases). Your token is used solely to authenticate GitLab requests
and is never logged. Full details: [PRIVACY.md](PRIVACY.md).

## Upstream contributions

This server is built on GitLab's REST and GraphQL APIs, on
[`client-go`](https://gitlab.com/gitlab-org/api/client-go) and on the
[MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk). When building it
turns up a gap in one of them, the fix goes upstream rather than staying a
workaround here, so every other user of those projects gets it too:

- **client-go**: response fields GitLab sends that the SDK's structs did not
  model, found by holding each struct against what a running GitLab actually
  sends, and fixes such as a panic decoding an issue with no id
  ([merge requests](https://gitlab.com/gitlab-org/api/client-go/-/merge_requests?scope=all&state=all&author_username=jmrp)).
- **GitLab**: API documentation and response annotations corrected where they
  disagreed with what the API sends, a fix so a revoked GPG identity no longer
  verifies commits, and proposed additions such as cancelling an automatic merge
  ([merge requests](https://gitlab.com/gitlab-org/gitlab/-/merge_requests?scope=all&state=all&author_username=jmrp)).
- **MCP Go SDK**: protocol conformance fixes around cancellation, protocol
  version negotiation and the initialize handshake
  ([pull requests](https://github.com/modelcontextprotocol/go-sdk/pulls?q=is%3Apr+author%3Ajmrplens)).

Every gap is tracked in
[docs/development/upstream-bugs.md](docs/development/upstream-bugs.md): the
upstream issue or merge request, whether it has merged and in which release,
and the workaround this server carries until it ships.

## Contributing & Security

- **Contributing**: see [CONTRIBUTING.md](CONTRIBUTING.md) for development guidelines, branch naming, commit conventions, and the PR process.
- **Security**: see [SECURITY.md](SECURITY.md) for the security policy and vulnerability reporting.
- **Code of Conduct**: see [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) ([Contributor Covenant v2.1](https://www.contributor-covenant.org/version/2/1/code_of_conduct/)).

> **Repository mirror**: GitHub is the canonical repository. A read-only mirror is available on [GitLab.com](https://gitlab.com/jmrp/gitlab-mcp-server) for discoverability; please open contributions on GitHub.

<details>
<summary><strong>Unnecessary statistics</strong> — numbers nobody asked for</summary>

<!-- START STATS -->

### File counts

> Counted over every git-tracked `.go` file, which includes the fixture trees under `cmd/audit_e2e_coverage/testdata` that exist to be read by the coverage audit rather than to run. `docs/development/testing/testing.md` counts the packages `go list` returns instead, so its unit-test figures are lower. Both are correct answers to different questions.

| Category                 |     Files |         Lines |
| ------------------------ | --------: | ------------: |
| Source (`.go`, non-test) |     1,348 |       303,182 |
| Unit tests (`_test.go`)  |       924 |       600,270 |
| End-to-end tests         |       497 |       103,199 |
| **Total**                | **2,769** | **1,006,651** |

### Functions

| Category                        |  Count |
| ------------------------------- | -----: |
| Source functions                | 10,541 |
| . Exported (public)             |  3,238 |
| . Unexported (private)          |  7,303 |
| Unit test functions (`TestXxx`) | 17,922 |
| Subtests (`t.Run(...)`)         |  6,464 |
| End-to-end test functions       |  1,345 |

### Ratios worth noting

| Observation                        |                      Value |
| ---------------------------------- | -------------------------: |
| Test lines vs source lines         | 1.98× more tests than code |
| Average source file length         |                 ~225 lines |
| Average test file length           |                 ~650 lines |
| Comment lines in source            |  71,648 (~23.6% of source) |
| Test functions per source function |                       1.7× |

### Code patterns

| Pattern                            | Count |
| ---------------------------------- | ----: |
| `if err != nil` checks             | 9,467 |
| `defer` statements                 | 1,171 |
| `struct` types defined             | 3,359 |
| `//nolint` suppressions            |   238 |
| `TODO` / `FIXME` / `HACK` comments |     1 |

### Project

| Metric                         | Value |
| ------------------------------ | ----: |
| Go packages                    |   299 |
| Direct dependencies (`go.mod`) |    34 |
| Indirect dependencies          |    37 |

### Hall of fame

| Record              | File                                    |
| ------------------- | --------------------------------------- |
| Longest source file | `cmd/server/main.go`. 4,831 lines       |
| Longest test file   | `cmd/server/main_test.go`. 11,523 lines |

### Because why not

| Fact                                 | Value                                                                                                |
| ------------------------------------ | ---------------------------------------------------------------------------------------------------- |
| Source code printed at 55 lines/page | ~5,512 pages of A4                                                                                   |
| Source lines mentioning `"gitlab"`   | 14,056 (impossible to avoid)                                                                         |
| Longest function name in source      | `assertDynamicCompatibilityPolicyOwnedByActionCompat` (51 chars)                                     |
| Longest test function name           | `TestNewOperationIndex_TwoRoutesMountedAtOnePath_KeepTheFirstAnswerAndMergeThePagination` (87 chars) |

<!-- END STATS -->

</details>

---

Maintained by [José M. Requena Plens](https://jmrp.io/) ·
[Project page](https://jmrp.io/projects/) ·
Hosted instance: [mcp.jmrp.io/gitlab](https://mcp.jmrp.io/gitlab)
