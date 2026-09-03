# GitLab MCP Server

GitLab for your AI assistant: a [Model Context Protocol](https://modelcontextprotocol.io) server exposing GitLab REST API v4 and GraphQL operations as MCP tools. Projects, issues, merge requests, pipelines, CI/CD, wikis, releases, users, groups, search and more, against GitLab.com or any self-managed instance.

This package wraps the native `gitlab-mcp-server` binary (written in Go) in a platform wheel, the same distribution model used by `uv`, `ruff` and `ziglang`: `pip` selects the wheel for your OS and architecture, and a tiny launcher hands over to the binary. No Go toolchain, no runtime downloads, no install scripts.

mcp-name: io.github.jmrplens/gitlab-mcp-server

## Quick start

Run it directly with [uv](https://docs.astral.sh/uv/):

```bash
GITLAB_TOKEN=glpat-xxxxxxxxxxxxxxxxxxxx uvx jmrplens-gitlab-mcp-server
```

Or install it on your PATH:

```bash
pipx install jmrplens-gitlab-mcp-server   # or: pip install jmrplens-gitlab-mcp-server
```

Either way the installed command is `gitlab-mcp-server` (the native binary); the `jmrplens-` prefixed command also works, which is what lets `uvx` resolve it by distribution name. About the name: the unprefixed `gitlab-mcp-server` project on PyPI is an empty registration held by an unrelated account and is under a PEP 541 reclamation request; this author-prefixed distribution is the official one meanwhile.

Typical MCP client configuration (stdio):

```json
{
  "mcpServers": {
    "gitlab": {
      "command": "uvx",
      "args": ["jmrplens-gitlab-mcp-server"],
      "env": {
        "GITLAB_URL": "https://gitlab.com",
        "GITLAB_TOKEN": "glpat-xxxxxxxxxxxxxxxxxxxx"
      }
    }
  }
}
```

`GITLAB_TOKEN` is the only required setting. `GITLAB_URL` defaults to `https://gitlab.com`; point it at your own host for self-managed instances.

## Configuration

Everything is configured through environment variables: `GITLAB_MCP_TOOL_SURFACE` (dynamic, meta or individual tool catalogs), `GITLAB_READ_ONLY`, `GITLAB_SAFE_MODE`, `GITLAB_TIER`, rate limiting, telemetry and more. See the [configuration guide](https://jmrp.io/docs/gitlab-mcp-server/configuration/) for the full reference.

## Platform support

Wheels are published for Linux (glibc), macOS and Windows on amd64 and arm64. On musl systems such as Alpine, use the container image `ghcr.io/jmrplens/gitlab-mcp-server` instead.

## Links

- [Documentation](https://jmrp.io/docs/gitlab-mcp-server/)
- [Source repository](https://github.com/jmrplens/gitlab-mcp-server)
- [Changelog and releases](https://github.com/jmrplens/gitlab-mcp-server/releases)
- [Security policy](https://github.com/jmrplens/gitlab-mcp-server/blob/main/SECURITY.md)

## License

MIT
