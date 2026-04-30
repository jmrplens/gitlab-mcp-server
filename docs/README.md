# Documentation

Project documentation for gitlab-mcp-server — a Model Context Protocol server for GitLab.

## Guides

| Document | Description |
| --- | --- |
| [Getting Started](getting-started.md) | Step-by-step tutorial: download, configure, first query (~5 min) |
| [Architecture](architecture.md) | System architecture with C4 diagrams, component details, and data flow |
| [Development](development/development.md) | Developer guide: setup, building, testing, adding new tools |
| [Configuration](configuration.md) | Environment variables, transport modes, and `.env` setup |
| [Error Handling](error-handling.md) | Error types, classification, Markdown formatting, and issue reporting |
| [Security](security.md) | Authentication, TLS, input validation, and transport security |
| [HTTP Server Mode](http-server-mode.md) | Multi-user HTTP transport with per-token+URL server pool |
| [OAuth App Setup](oauth-app-setup.md) | Creating GitLab OAuth applications for MCP clients |
| [IDE Configuration](ide-configuration.md) | Per-IDE MCP JSON configuration (stdio, HTTP legacy, HTTP OAuth) |
| [CI/CD Usage](ci-cd.md) | Using gitlab-mcp-server in CI/CD pipelines (with or without LLM) |
| [Auto-Update](auto-update.md) | Self-update mechanism, modes, MCP tools, and release requirements |
| [Resource Consumption](resource-consumption.md) | Memory footprint, scaling limits, and optimization strategies |
| [Meta-Tools](meta-tools.md) | Domain-level meta-tool reference with action mappings |
| [Output Format](output-format.md) | How tool responses are structured: Markdown + JSON, annotations, clickable links, next-step hints |
| [GraphQL Integration](graphql.md) | When and how the server uses GitLab's GraphQL API |
| [Troubleshooting](troubleshooting.md) | Common issues and solutions for connection, TLS, tools, and transport |

## Development

| Document | Description |
| --- | --- |
| [Development Guide](development/development.md) | Developer guide: setup, building, testing, adding new tools |
| [Testing](development/testing.md) | Test suite overview, coverage breakdown, and per-package statistics |
| [Static Analysis](development/static-analysis.md) | Static analysis tools: vet, modernize, golangci-lint, gosec, staticcheck, govulncheck |

## Reference

| Document | Description |
| --- | --- |
| [CLI Reference](cli-reference.md) | Complete command-line flags and usage examples |
| [Environment Variables](env-reference.md) | All environment variables with defaults and descriptions |
| [tools/](tools/) | Per-domain tool documentation (25 domain docs) |
| [Resources Reference](resources-reference.md) | All 46 MCP resources with URI templates |
| [Prompts Reference](prompts-reference.md) | All 38 prompts with arguments and output format |
| [Capabilities](capabilities/) | 6 MCP capabilities: logging, completions, roots, progress, sampling, elicitation |
| [Usage Examples](examples/usage-examples.md) | Real-world MCP usage scenarios |
| [adr/](adr/) | Architectural Decision Records |

## Learning

| Document | Description |
| --- | --- |
| [MCP Specification](https://modelcontextprotocol.io/specification/) | Official Model Context Protocol specification and documentation |

## Quick Links

- [CLAUDE.md](../CLAUDE.md) — AI development context and agent catalog
- [README.md](../README.md) — Project overview and quickstart
- [CONTRIBUTING.md](../CONTRIBUTING.md) — Contribution guidelines
