# Guides

**How-to guides** — task-oriented instructions for people *running* gitlab-mcp-server.

Each guide walks you through one concrete operational task end to end:
installing the server through the channel that fits your machine, wiring it
into your editor, exposing it to a team over HTTP, wiring it into CI, keeping it
current, or getting unstuck. New here? Start with
[Getting Started](../getting-started.md).

> **Diátaxis type**: How-to · **Audience**: 👤 Users & 🔧 operators

| Guide                                                   | What it helps you do                                                                                                                                            |
| ------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [Installation](installation.md)                         | Install through any channel (binary, Homebrew, winget, Docker, npm, PyPI, NuGet, `.mcpb`, Agent Plugins, hosted endpoint), then verify, upgrade or uninstall it |
| [IDE Configuration](ide-configuration.md)               | Configure the MCP server in VS Code, Cursor, JetBrains, and other clients (stdio, HTTP legacy, HTTP OAuth)                                                      |
| [Claude Desktop Extension](claude-desktop-extension.md) | Install or build the one-click `.mcpb` desktop extension for Claude Desktop                                                                                     |
| [HTTP Server Mode](http-server-mode.md)                 | Run the multi-user HTTP transport with a per-token+URL server pool                                                                                              |
| [Remote Deployment](remote-deployment.md)               | Stand it up for other people: OS service, Docker, reverse proxies, TLS, and balancing several instances                                                         |
| [OAuth App Setup](oauth-app-setup.md)                   | Create a GitLab OAuth application so MCP clients can authenticate                                                                                               |
| [CI/CD Usage](ci-cd.md)                                 | Use the server inside CI/CD pipelines, with or without an LLM in the loop                                                                                       |
| [Client Compatibility](client-compatibility.md)         | Understand per-client response profiles (OpenAI Codex) and known client-side limits                                                                             |
| [Telemetry](telemetry.md)                               | Export OpenTelemetry traces, metrics and logs over OTLP, and choose what is recorded about callers                                                              |
| [Troubleshooting](troubleshooting.md)                   | Diagnose common connection, TLS, tool, and transport problems                                                                                                   |
| [Examples](examples/README.md)                          | Walk through real-world, multi-step usage scenarios and skill templates                                                                                         |

**Looking for something else?**
[Reference](../reference/README.md) for exact flags and variables ·
[Concepts](../concepts/README.md) to understand how the server works ·
[Development](../development/README.md) to contribute.
