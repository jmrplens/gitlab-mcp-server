# Documentation

Documentation for **gitlab-mcp-server** — a Model Context Protocol server that
exposes GitLab's REST and GraphQL APIs as MCP tools for AI assistants.

The docs are organized **by what you're trying to do**. Pick the entry point that
matches your goal:

| If you want to…                                                                         | Go to                                                                    |
| --------------------------------------------------------------------------------------- | ------------------------------------------------------------------------ |
| **Get running in ~5 minutes**                                                           | [Getting Started](getting-started.md) — download, configure, first query |
| **Do a specific task** (installation, IDE setup, HTTP mode, CI, OAuth, fixing problems) | [Guides](guides/README.md)                                               |
| **Look up an exact flag, variable, tool, resource, or prompt**                          | [Reference](reference/README.md)                                         |
| **Understand how the server works and why**                                             | [Concepts](concepts/README.md)                                           |
| **Contribute to the codebase**                                                          | [Development](development/README.md)                                     |

## Sections

### 📘 [Guides](guides/README.md) — how-to

Task-oriented instructions for running the server: installation through any
channel, IDE configuration, HTTP server mode, remote and enterprise deployment,
OAuth app setup, CI/CD usage, troubleshooting, and worked examples.

### 📖 [Reference](reference/README.md) — look-up

Precise descriptions of every surface: configuration, environment variables, CLI
flags, output format, and the full inventory of tools, resources, prompts, and
capabilities. Several reference pages are generated from the codebase and validated in CI.

### 💡 [Concepts](concepts/README.md) — explanation

The ideas behind the system: architecture, the meta-tool and dynamic-tool surfaces,
GraphQL integration, security model, resource consumption, and error handling.

### 🛠️ [Development](development/README.md) — contributor

Building, testing, and extending the server: the development guide, tool-surface
architecture, static-analysis gates, the testing reference, and Architectural
Decision Records.

## Quick links

- [README.md](../README.md) — project overview and quickstart
- [CONTRIBUTING.md](../CONTRIBUTING.md) — contribution guidelines
- [CLAUDE.md](../CLAUDE.md) — AI development context and agent catalog
- [MCP Specification](https://modelcontextprotocol.io/specification/) — the official protocol spec
