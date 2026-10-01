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

## Verify what you run

The `gitlab-mcp-server` command a wheel installs is the release binary byte for byte, so the build provenance attestation GitHub holds for that release asset verifies it directly with the [GitHub CLI](https://cli.github.com/) (releases after 2.7.5):

```bash
# pip, pipx or uv tool install: the command on your PATH is the binary
gh attestation verify "$(command -v gitlab-mcp-server)" -R jmrplens/gitlab-mcp-server \
  --signer-workflow jmrplens/gitlab-mcp-server/.github/workflows/release.yml

# uvx: the binary sits in uv's cache, and the package says where
gh attestation verify "$(uvx --from jmrplens-gitlab-mcp-server python -c 'import gitlab_mcp_server as m; print(m.find_binary())')" \
  -R jmrplens/gitlab-mcp-server --signer-workflow jmrplens/gitlab-mcp-server/.github/workflows/release.yml
```

`--signer-workflow` holds the attestation to the release workflow, since `-R` alone accepts one any workflow of the repository minted; add `--source-ref refs/tags/v<version>` to hold it to the release you installed. On Windows, `(Get-Command gitlab-mcp-server).Source` gives the path for the first form. For a pinned `uvx`, name the same version in `--from` (`jmrplens-gitlab-mcp-server==<version>`). The wheels also carry PyPI's own publish attestation (PEP 740), shown on each file's page on PyPI. The release signature and the other channels are covered in [release integrity](https://jmrp.io/docs/gitlab-mcp-server/operations/security/#verifying-release-integrity).

## Configuration

Everything is configured through environment variables: `GITLAB_MCP_TOOL_SURFACE` (dynamic, meta or individual tool catalogs), `GITLAB_MCP_READ_ONLY`, `GITLAB_MCP_SAFE_MODE`, `GITLAB_MCP_TIER`, rate limiting, telemetry and more. See the [configuration guide](https://jmrp.io/docs/gitlab-mcp-server/configuration/) for the full reference.

## Platform support

Wheels are published for Linux (glibc), macOS and Windows on amd64 and arm64. On musl systems such as Alpine, use the container image `ghcr.io/jmrplens/gitlab-mcp-server` instead.

## Links

- [Documentation](https://jmrp.io/docs/gitlab-mcp-server/)
- [Source repository](https://github.com/jmrplens/gitlab-mcp-server)
- [Changelog and releases](https://github.com/jmrplens/gitlab-mcp-server/releases)
- [Security policy](https://github.com/jmrplens/gitlab-mcp-server/blob/main/SECURITY.md)

## License

MIT
