<p align="center">
  <img alt="GitLab MCP Server: GitLab for your AI assistant, one action catalog, three MCP tool surfaces" src="https://raw.githubusercontent.com/jmrplens/gitlab-mcp-server/main/.github/brand/banner.webp" width="100%">
</p>

# GitLab MCP Server

<p align="center">

[![GitHub Release](https://img.shields.io/github/v/release/jmrplens/gitlab-mcp-server?style=flat&logo=github&label=Release)](https://github.com/jmrplens/gitlab-mcp-server/releases/latest)
[![npm](https://img.shields.io/npm/v/@jmrp.io/gitlab-mcp-server?style=flat&logo=npm&label=npm)](https://www.npmjs.com/package/@jmrp.io/gitlab-mcp-server)
[![PyPI](https://img.shields.io/pypi/v/jmrplens-gitlab-mcp-server?style=flat&logo=pypi&label=PyPI)](https://pypi.org/project/jmrplens-gitlab-mcp-server/)
[![NuGet](https://img.shields.io/nuget/v/gitlab-mcp-server?style=flat&logo=nuget&label=NuGet)](https://www.nuget.org/packages/gitlab-mcp-server)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
![Platform](https://img.shields.io/badge/Windows%20%7C%20Linux%20%7C%20macOS-amd64%20%26%20arm64-lightgrey?style=flat&logo=windows-terminal&logoColor=white)
[![CI](https://img.shields.io/github/actions/workflow/status/jmrplens/gitlab-mcp-server/ci.yml?branch=main&style=flat&logo=githubactions&logoColor=white&label=CI)](https://github.com/jmrplens/gitlab-mcp-server/actions/workflows/ci.yml)
[![Quality Gate](https://sonarcloud.io/api/project_badges/measure?project=jmrplens_gitlab-mcp-server&metric=alert_status)](https://sonarcloud.io/summary/overall?id=jmrplens_gitlab-mcp-server)
[![Coverage](https://sonarcloud.io/api/project_badges/measure?project=jmrplens_gitlab-mcp-server&metric=coverage)](https://sonarcloud.io/summary/overall?id=jmrplens_gitlab-mcp-server)
[![Go Reference](https://pkg.go.dev/badge/github.com/jmrplens/gitlab-mcp-server/v3.svg)](https://pkg.go.dev/github.com/jmrplens/gitlab-mcp-server/v3)

</p>

<p align="center">

[![Glama MCP Score](https://glama.ai/mcp/servers/jmrplens/gitlab-mcp-server/badges/score.svg)](https://glama.ai/mcp/servers/jmrplens/gitlab-mcp-server)
<!-- The ?v= is a cache key for GitHub's image proxy, not a LobeHub parameter: the
     proxy keyed a red "not listed" badge for a day, and only a changed URL evicts
     it. Bump the number if the badge ever goes stale again. -->
[![MCP Badge](https://lobehub.com/badge/mcp/jmrplens-gitlab-mcp-server?v=2)](https://lobehub.com/mcp/jmrplens-gitlab-mcp-server)
[![MCP Toplist](https://mcptoplist.com/badge/io.github.jmrplens%2Fgitlab-mcp-server.svg)](https://mcptoplist.com/server/io.github.jmrplens%2Fgitlab-mcp-server)
[![Cursor Directory](https://img.shields.io/badge/Cursor-Directory-000000?logo=cursor&logoColor=white)](https://cursor.directory/plugins/gitlab-mcp-server)
[![Hosted endpoint](https://img.shields.io/badge/Hosted-mcp.jmrp.io%2Fgitlab-6d28d9?style=flat&logo=icloud&logoColor=white)](https://mcp.jmrp.io/)

</p>

**All of GitLab for your AI assistant.** Up to 1,098 actions over GitLab's REST and GraphQL APIs, from Free to Ultimate and on GitLab.com, in one static binary that works with Claude, Cursor, VS Code, Codex and any other MCP client. You ask in plain language; it does the GitLab work.

<!-- START TOKEN CLAIM -->

**10,476 tokens of startup context by default, the same on every GitLab tier (1,794 with `GITLAB_MCP_CAPABILITY_SURFACE=minimal`).** Two tools reach the whole catalog, where listing every tool as its own costs from 559,765 to 712,396 tokens. Measured with the cl100k_base tokenizer and checked in CI. [How it is measured](https://jmrp.io/docs/gitlab-mcp-server/tools/dynamic-tools/#how-much-startup-context-does-dynamic-mode-save)

<!-- END TOKEN CLAIM -->

> "Review merge request !15: is it safe to merge?" · "Why did the last pipeline fail?" · "List open issues assigned to me" · "Generate release notes from v1.0 to v2.0"

## Why this server

- **Every tier and every instance.** Free/CE, Premium and Ultimate, self-managed or GitLab.com with its Orbit knowledge graph. The tier is detected and the catalog follows it.
- **The token you already have.** A personal, project or group access token with `api`, or `read_api` for a read-only surface; a fine-grained token, judged action by action from its grant; OAuth in HTTP mode.
- **Guard rails.** Read-only mode, safe mode (a preview of each write instead of the write), tools excluded by name, and a confirmation before anything destructive.
- **Three surfaces, two transports.** Two discovery tools by default, one tool per domain or one per action; stdio for a desktop client, or HTTP for a shared deployment that keeps each credential apart. 45 resources and 37 prompts besides.
- **Releases you can verify.** Signed checksums, an SBOM per binary and build provenance for every artifact.

## Install

The buttons register the Docker image, so they need [Docker](https://www.docker.com/); the Claude Desktop row downloads a native extension instead. Create a [personal access token](https://docs.gitlab.com/user/profile/personal_access_tokens/) with the `api` scope (or `read_api` for read-only use).

<table>
  <tr>
    <th align="left">Client</th>
    <th align="left">One-click button</th>
    <th align="left">Token step</th>
  </tr>
  <tr>
    <td><b>VS Code</b></td>
    <td><a href="https://insiders.vscode.dev/redirect/mcp/install?name=gitlab&amp;config=%7B%22command%22%3A%22docker%22%2C%22args%22%3A%5B%22run%22%2C%22-i%22%2C%22--rm%22%2C%22-e%22%2C%22GITLAB_TOKEN%22%2C%22ghcr.io%2Fjmrplens%2Fgitlab-mcp-server%3Alatest%22%5D%2C%22env%22%3A%7B%22GITLAB_TOKEN%22%3A%22%24%7Binput%3Agitlab_token%7D%22%7D%2C%22inputs%22%3A%5B%7B%22id%22%3A%22gitlab_token%22%2C%22type%22%3A%22promptString%22%2C%22description%22%3A%22GitLab%20Personal%20Access%20Token%20%28api%20scope%29%22%2C%22password%22%3Atrue%7D%5D%7D"><img alt="Install in VS Code" src="https://img.shields.io/badge/Install_in-VS_Code-0098FF?style=flat-square&amp;logo=visualstudiocode&amp;logoColor=white" /></a> <a href="https://insiders.vscode.dev/redirect/mcp/install?name=gitlab&amp;config=%7B%22command%22%3A%22docker%22%2C%22args%22%3A%5B%22run%22%2C%22-i%22%2C%22--rm%22%2C%22-e%22%2C%22GITLAB_TOKEN%22%2C%22ghcr.io%2Fjmrplens%2Fgitlab-mcp-server%3Alatest%22%5D%2C%22env%22%3A%7B%22GITLAB_TOKEN%22%3A%22%24%7Binput%3Agitlab_token%7D%22%7D%2C%22inputs%22%3A%5B%7B%22id%22%3A%22gitlab_token%22%2C%22type%22%3A%22promptString%22%2C%22description%22%3A%22GitLab%20Personal%20Access%20Token%20%28api%20scope%29%22%2C%22password%22%3Atrue%7D%5D%7D&amp;quality=insiders"><img alt="Install in VS Code Insiders" src="https://img.shields.io/badge/Install_in-VS_Code_Insiders-24bfa5?style=flat-square&amp;logo=visualstudiocode&amp;logoColor=white" /></a></td>
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
    <td><a href="https://github.com/jmrplens/gitlab-mcp-server/releases/latest/download/gitlab-mcp-server.mcpb"><img alt="Download the .mcpb extension" src="https://img.shields.io/badge/Download-.mcpb-d97757?style=flat-square&amp;logo=claude&amp;logoColor=white" /></a></td>
    <td>settings UI</td>
  </tr>
</table>

**Claude Code.** The registration command never carries the token: `-e GITLAB_TOKEN` forwards it from the environment Claude Code hands `docker`.

```bash
export GITLAB_TOKEN=glpat-xxxx
claude mcp add gitlab --transport stdio \
  -- docker run -i --rm -e GITLAB_TOKEN ghcr.io/jmrplens/gitlab-mcp-server:latest
```

**Any other client** runs one of these as the server's command, with `GITLAB_TOKEN` in the environment it passes:

```bash
npx -y @jmrp.io/gitlab-mcp-server                   # npm, nothing installed first
uvx jmrplens-gitlab-mcp-server                      # PyPI, nothing installed first
dnx gitlab-mcp-server                               # NuGet (.NET 10 SDK), nothing installed first
brew install jmrplens/tap/gitlab-mcp-server         # Homebrew, macOS and Linux
winget install --id jmrplens.gitlab-mcp-server -e   # winget, Windows
docker run -i --rm -e GITLAB_TOKEN ghcr.io/jmrplens/gitlab-mcp-server:latest
```

For a self-managed instance, set `GITLAB_URL=https://gitlab.example.com` beside the token; with the Docker image, also forward it by adding `-e GITLAB_URL` to the `docker` arguments, since the container receives only the variables named there ([Docker install](https://jmrp.io/docs/gitlab-mcp-server/install/docker/#configure-your-client)). Every channel, per-client configuration and verification step is in the [installation guide](https://jmrp.io/docs/gitlab-mcp-server/install/overview/), and an assistant installing it for you will find the same in [`llms.txt`](llms.txt).

**To try it first**, a public instance runs at `https://mcp.jmrp.io/gitlab`, and the [browser inspector](https://mcp.jmrp.io/inspector/?server=gitlab) calls it read-only after an OAuth sign-in. Your token and every request pass through that machine, so run it yourself to keep using it; the [hosted endpoint page](https://jmrp.io/docs/gitlab-mcp-server/install/hosted/) says what it is and is not.

## Documentation

Everything else is at **[jmrp.io/docs/gitlab-mcp-server](https://jmrp.io/docs/gitlab-mcp-server)**, in English and Spanish: [getting started](https://jmrp.io/docs/gitlab-mcp-server/getting-started/), [configuration](https://jmrp.io/docs/gitlab-mcp-server/configuration/), [tool surfaces](https://jmrp.io/docs/gitlab-mcp-server/tools/overview/), [HTTP server mode](https://jmrp.io/docs/gitlab-mcp-server/operations/http-server/), [security](https://jmrp.io/docs/gitlab-mcp-server/operations/security/), [privacy](https://jmrp.io/docs/gitlab-mcp-server/operations/privacy/) and [troubleshooting](https://jmrp.io/docs/gitlab-mcp-server/operations/troubleshooting/).

## Contributing, security and licence

Contributions are welcome here on GitHub; [CONTRIBUTING.md](CONTRIBUTING.md) describes the workflow, and [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) applies. Report a vulnerability privately, as [SECURITY.md](SECURITY.md) describes. The server is MIT licensed ([LICENSE](LICENSE)).

---

Maintained by [José M. Requena Plens](https://jmrp.io/) · [Project page](https://jmrp.io/projects/) · Hosted instance: [mcp.jmrp.io/gitlab](https://mcp.jmrp.io/gitlab)
