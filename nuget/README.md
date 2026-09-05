# GitLab MCP Server

GitLab for your AI assistant: a [Model Context Protocol](https://modelcontextprotocol.io) server exposing GitLab REST API v4 and GraphQL operations as MCP tools. Projects, issues, merge requests, pipelines, CI/CD, wikis, releases, users, groups, search and more, against GitLab.com or any self-managed instance.

This package is a .NET tool whose entry point is the native `gitlab-mcp-server` binary (written in Go). It is a pointer package plus one package per runtime identifier, the layout the .NET 10 SDK uses for tools that ship a platform-specific executable: `dotnet tool install` and `dnx` resolve the package for your operating system and architecture and run the binary directly. No .NET runtime is involved once the server is running, nothing compiles, and nothing is downloaded at install time beyond the two packages.

<!-- mcp-name: io.github.jmrplens/gitlab-mcp-server -->

## Quick start

Run it directly with `dnx` (the .NET 10 SDK's tool runner):

```bash
GITLAB_TOKEN=glpat-xxxxxxxxxxxxxxxxxxxx dnx gitlab-mcp-server
```

Or install it on your PATH:

```bash
dotnet tool install -g gitlab-mcp-server
```

Either way the command is `gitlab-mcp-server`. Two things about `dnx` worth knowing: it reads its own options anywhere on the command line, so arguments meant for the server go after `--` (`dnx gitlab-mcp-server -- --version`; without the separator, `--version` is read as `dnx`'s own option and prints its usage); and the .NET 10 SDK's `dnx` installs the tool without asking when its standard input is not a terminal, which is how an MCP client starts it, so a client configuration needs no extra flag.

Typical MCP client configuration (stdio):

```json
{
  "mcpServers": {
    "gitlab": {
      "command": "dnx",
      "args": ["gitlab-mcp-server"],
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

Everything is configured through environment variables: `GITLAB_MCP_TOOL_SURFACE` (dynamic, meta or individual tool catalogs), `GITLAB_MCP_READ_ONLY`, `GITLAB_MCP_SAFE_MODE`, `GITLAB_MCP_TIER`, rate limiting, telemetry and more. See the [configuration guide](https://jmrp.io/docs/gitlab-mcp-server/configuration/) for the full reference.

## Platform support

Packages are published for `linux-x64`, `linux-arm64`, `osx-x64`, `osx-arm64`, `win-x64` and `win-arm64`. The Linux binaries need glibc; on musl systems such as Alpine, use the container image `ghcr.io/jmrplens/gitlab-mcp-server` instead. Installing or running the tool needs the .NET 10 SDK or newer; the tool manifest format the runtime-specific packages use does not exist in older SDKs.

## Links

- [Documentation](https://jmrp.io/docs/gitlab-mcp-server/)
- [Source repository](https://github.com/jmrplens/gitlab-mcp-server)
- [Changelog and releases](https://github.com/jmrplens/gitlab-mcp-server/releases)
- [Security policy](https://github.com/jmrplens/gitlab-mcp-server/blob/main/SECURITY.md)

## License

MIT
