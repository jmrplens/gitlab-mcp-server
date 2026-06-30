# Guides

**How-to guides** — task-oriented instructions for people *running* gitlab-mcp-server.

Each guide assumes you already have the server installed (if not, start with
[Getting Started](../getting-started.md)) and walks you through one concrete
operational task end to end: wiring it into your editor, exposing it to a team
over HTTP, wiring it into CI, keeping it current, or getting unstuck.

> **Diátaxis type**: How-to · **Audience**: 👤 Users & 🔧 operators

| Guide                                     | What it helps you do                                                                                       |
| ----------------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| [IDE Configuration](ide-configuration.md) | Configure the MCP server in VS Code, Cursor, JetBrains, and other clients (stdio, HTTP legacy, HTTP OAuth) |
| [HTTP Server Mode](http-server-mode.md)   | Run the multi-user HTTP transport with a per-token+URL server pool                                         |
| [OAuth App Setup](oauth-app-setup.md)     | Create a GitLab OAuth application so MCP clients can authenticate                                          |
| [CI/CD Usage](ci-cd.md)                   | Use the server inside CI/CD pipelines, with or without an LLM in the loop                                  |
| [Auto-Update](auto-update.md)             | Enable, configure, or disable the self-update mechanism                                                    |
| [Troubleshooting](troubleshooting.md)     | Diagnose common connection, TLS, tool, and transport problems                                              |
| [Examples](examples/usage-examples.md)    | Walk through real-world, multi-step usage scenarios                                                        |

**Looking for something else?**
[Reference](../reference/README.md) for exact flags and variables ·
[Concepts](../concepts/README.md) to understand how the server works ·
[Development](../development/README.md) to contribute.
