# @jmrplens/gitlab-mcp-server

A [Model Context Protocol](https://modelcontextprotocol.io) server that exposes
the GitLab REST v4 and GraphQL APIs as tools for AI assistants. It runs as a
local binary over stdio (default) or HTTP.

This package is a thin launcher. The actual server is a prebuilt Go binary that
ships inside a per-platform package; npm installs only the one matching your
operating system and CPU, so there is nothing to compile and nothing is
downloaded at install time.

## Run without installing

```bash
npx @jmrplens/gitlab-mcp-server
```

Most MCP clients are configured to launch the server this way. For example:

```json
{
  "mcpServers": {
    "gitlab": {
      "command": "npx",
      "args": ["-y", "@jmrplens/gitlab-mcp-server"],
      "env": {
        "GITLAB_URL": "https://gitlab.com",
        "GITLAB_TOKEN": "glpat-…"
      }
    }
  }
}
```

## Install

```bash
npm install -g @jmrplens/gitlab-mcp-server
gitlab-mcp-server --help
```

## Configuration

`GITLAB_URL` and `GITLAB_TOKEN` are the two required environment variables in
stdio mode. Every flag and variable — HTTP mode, OAuth, read-only and safe
modes, tool surfaces, tiers — is documented in the
[configuration reference](https://jmrp.io/docs/gitlab-mcp-server/reference/configuration).

## Auto-update is off under npm

The server can update itself in place, but that is disabled here: npm owns the
binary, and letting it replace itself would desynchronize what npm has
recorded. Update with your package manager instead (`npm update -g
@jmrplens/gitlab-mcp-server`). Setting `AUTO_UPDATE` explicitly still takes
effect if you have a reason to override it.

## Supported platforms

Linux, macOS and Windows, on x64 and arm64. On any other platform the launcher
exits with a message pointing to the
[release binaries](https://github.com/jmrplens/gitlab-mcp-server/releases) and
the option to build from source.

## Links

- Documentation: <https://jmrp.io/docs/gitlab-mcp-server>
- Source and issues: <https://github.com/jmrplens/gitlab-mcp-server>
- License: MIT
