# GitLab MCP Server

**Connect your AI assistant to GitLab so it can review merge requests, triage pipelines, manage issues, and draft releases — in plain language.** One static binary (or this container), [1000+ GitLab tools](https://github.com/jmrplens/gitlab-mcp-server#tool-surfaces) over the full REST + GraphQL API, working with Claude, Cursor, VS Code, and any MCP client.

You talk to your AI assistant; it does the GitLab work. No project IDs, API endpoints, or JSON to remember.

> "Review merge request !15 — is it safe to merge?" · "Why did the last pipeline fail?" · "List open issues assigned to me" · "Generate release notes from v1.0 to v2.0"

## Run with Docker

Stdio transport (for desktop MCP clients such as Claude Desktop, Cursor, or VS Code):

```json
{
  "mcpServers": {
    "gitlab": {
      "command": "docker",
      "args": ["run", "-i", "--rm", "-e", "GITLAB_URL", "-e", "GITLAB_TOKEN", "jmrplens/gitlab-mcp-server:latest", "--http=false"],
      "env": {
        "GITLAB_URL": "https://gitlab.com",
        "GITLAB_TOKEN": "glpat-xxxxxxxxxxxx"
      }
    }
  }
}
```

HTTP transport (remote/shared use — the container's default mode, listening on `:8080`). Name the GitLab instance this deployment serves, or it refuses to start rather than making requests to whatever host a caller names:

```bash
docker run --rm -p 8080:8080 jmrplens/gitlab-mcp-server:latest \
  --http --http-addr=0.0.0.0:8080 --gitlab-url=https://gitlab.com
```

Images are multi-arch (`linux/amd64`, `linux/arm64`), published for every release with provenance and SBOM attestations, and signed with Cosign. The same image is also available as `ghcr.io/jmrplens/gitlab-mcp-server`.

## Why this server

- 🗣️ **Plain-language GitLab.** The AI translates "is MR !15 safe to merge?" into the right API calls. You don't touch endpoints, IDs, or JSON.
- 🧰 **The whole platform — 1000+ tools.** Broad GitLab REST v4 + GraphQL coverage: projects, branches, tags, releases, merge requests, issues, pipelines, jobs, groups, users, wikis, environments, deployments, packages, container registry, runners, feature flags, CI/CD variables, security, admin, tokens, and more.
- 🪶 **Low-token by default.** The default **dynamic** surface exposes just 2 tools (`find` + `execute`) while reaching the full catalog — so it fits any client's context window.
- ✅ **Proven with real models.** An automated evaluator runs Anthropic, Google, OpenAI, and Qwen against live GitLab instances: **99.5% aggregate success** across thousands of operations.
- 🔒 **Safe by design.** Read-only mode, safe mode (dry-run preview of every mutation), TLS options for self-hosted GitLab, and continuous SonarCloud quality/security gates.
- 🖥️ **Runs anywhere.** One static binary or container; Windows, Linux & macOS; amd64 & arm64; stdio (desktop) and HTTP (remote).

## Try it without installing anything

A public instance runs at **`https://mcp.jmrp.io/gitlab`** — nothing to install, no account beyond your own GitLab token (`Authorization: Bearer <token>`, sent per request and never stored; it runs in OAuth mode, so a client that speaks the OAuth flow needs no header at all). It is stateless streamable HTTP: `POST` is the transport, a `GET` answers `405` by design. For private self-managed instances, run the container locally so your credentials never leave your machine.

## Documentation

- [Repository and full README](https://github.com/jmrplens/gitlab-mcp-server)
- [Getting Started guide](https://jmrp.io/docs/gitlab-mcp-server)
- [Releases and signed binaries](https://github.com/jmrplens/gitlab-mcp-server/releases)

---

Maintained by [José M. Requena Plens](https://jmrp.io/) ·
[Project page](https://jmrp.io/projects/) ·
Hosted instance: [mcp.jmrp.io/gitlab](https://mcp.jmrp.io/gitlab)
