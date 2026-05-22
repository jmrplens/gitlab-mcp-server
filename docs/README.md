# Documentation

Project documentation for gitlab-mcp-server — a Model Context Protocol server for GitLab.

## Guides

| Document                                        | Description                                                                                       |
| ----------------------------------------------- | ------------------------------------------------------------------------------------------------- |
| [Getting Started](getting-started.md)           | Step-by-step tutorial: download, configure, first query (~5 min)                                  |
| [Architecture](architecture.md)                 | System architecture with C4 diagrams, component details, and data flow                            |
| [Development](development/development.md)       | Developer guide: setup, building, testing, adding new tools                                       |
| [Configuration](configuration.md)               | Environment variables, transport modes, and `.env` setup                                          |
| [Error Handling](error-handling.md)             | Error types, classification, Markdown formatting, and actionable diagnostics                      |
| [Security](security.md)                         | Authentication, TLS, input validation, and transport security                                     |
| [HTTP Server Mode](http-server-mode.md)         | Multi-user HTTP transport with per-token+URL server pool                                          |
| [OAuth App Setup](oauth-app-setup.md)           | Creating GitLab OAuth applications for MCP clients                                                |
| [IDE Configuration](ide-configuration.md)       | Per-IDE MCP JSON configuration (stdio, HTTP legacy, HTTP OAuth)                                   |
| [CI/CD Usage](ci-cd.md)                         | Using gitlab-mcp-server in CI/CD pipelines (with or without LLM)                                  |
| [Auto-Update](auto-update.md)                   | Self-update mechanism, modes, MCP tools, and release requirements                                 |
| [Resource Consumption](resource-consumption.md) | Memory footprint, scaling limits, and optimization strategies                                     |
| [Meta-Tools](meta-tools.md)                     | Domain-level meta-tool reference with action mappings                                             |
| [Dynamic Toolset](dynamic-tools.md)             | Low-token find/execute mode with canonical action catalog and migration guidance                  |
| [Output Format](output-format.md)               | How tool responses are structured: Markdown + JSON, annotations, clickable links, next-step hints |
| [Testing](testing/)                             | Unit, E2E, and AI model evaluation documentation                                                  |
| [GraphQL Integration](graphql.md)               | When and how the server uses GitLab's GraphQL API                                                 |
| [Troubleshooting](troubleshooting.md)           | Common issues and solutions for connection, TLS, tools, and transport                             |

## Development

| Document                                                                                   | Description                                                                                      |
| ------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------ |
| [Development Guide](development/development.md)                                            | Developer guide: setup, building, testing, adding new tools                                      |
| [Tool Surfaces And Canonical Action Core](development/tool-surfaces-and-action-core.md)    | Developer architecture for individual tools, meta-tools, dynamic mode, and shared action catalog |
| [Catalog-First Individual Tools Evaluation](development/catalog-first-individual-tools.md) | Evaluation of whether individual tools should be generated from the canonical action catalog     |
| [Testing](testing/testing.md)                                                              | Test suite overview, coverage breakdown, and per-package statistics                              |
| [AI Model Evaluation](testing/model-evaluation.md)                                         | How model evaluations validate MCP tool use against schema and Docker GitLab                     |
| [AI Model Evaluation Developer Guide](testing/model-evaluation-developer.md)               | Commands, fixtures, traces, and maintenance workflow for model evaluation                        |
| [AI Model Evaluation Results](testing/model-results.md)                                    | Curated model compatibility and benchmark snapshots                                              |
| [Static Analysis](development/static-analysis.md)                                          | Consolidated static analysis with golangci-lint, govulncheck, and markdownlint                   |
| [Godoc Compliance](development/godoc.md)                                                   | Godoc audit workflow for packages, exported symbols, and test functions                          |

## Reference

| Document                                      | Description                                                                      |
| --------------------------------------------- | -------------------------------------------------------------------------------- |
| [CLI Reference](cli-reference.md)             | Complete command-line flags and usage examples                                   |
| [Environment Variables](env-reference.md)     | All environment variables with defaults and descriptions                         |
| [tools/](tools/)                              | Per-domain tool documentation (25 domain docs)                                   |
| [Resources Reference](resources-reference.md) | MCP resources and URI templates, including the surface-aware tool manifest       |
| [Prompts Reference](prompts-reference.md)     | All 37 prompts with arguments and output format                                  |
| [Capabilities](capabilities/)                 | 6 MCP capabilities: logging, completions, roots, progress, sampling, elicitation |
| [Usage Examples](examples/usage-examples.md)  | Real-world MCP usage scenarios                                                   |
| [adr/](adr/)                                  | Architectural Decision Records                                                   |

## Learning

| Document                                                            | Description                                                     |
| ------------------------------------------------------------------- | --------------------------------------------------------------- |
| [MCP Specification](https://modelcontextprotocol.io/specification/) | Official Model Context Protocol specification and documentation |

## Quick Links

- [CLAUDE.md](../CLAUDE.md) — AI development context and agent catalog
- [README.md](../README.md) — Project overview and quickstart
- [CONTRIBUTING.md](../CONTRIBUTING.md) — Contribution guidelines
