# Development

**Contributor documentation** — everything you need to *change* gitlab-mcp-server.

This is the home for developer-facing material. Start with the
[Development Guide](development.md) for environment setup, building, testing, and
the workflow for adding a new tool. The remaining pages go deep on the runtime
architecture, the static-analysis and godoc gates, the dynamic search ranker,
token accounting, and the live-test fixtures. Architectural Decision Records and
the full testing reference live in subfolders here.

> **Diátaxis type**: How-to & Reference · **Audience**: 🛠️ Contributors & maintainers

| Document                                                                  | Purpose                                                                    |
| ------------------------------------------------------------------------- | -------------------------------------------------------------------------- |
| [Development Guide](development.md)                                       | Setup, building, testing, and adding new tools                             |
| [Tool Surfaces & Canonical Action Core](tool-surfaces-and-action-core.md) | How individual, meta, and dynamic surfaces project from the shared catalog |
| [Catalog-First Individual Tools](catalog-first-individual-tools.md)       | Evaluation of generating individual tools from the canonical catalog       |
| [Dynamic Search Ranker](dynamic-search-ranker.md)                         | How `gitlab_find_action` ranks and matches queries                         |
| [Command-Line Utilities](cmd-utilities.md)                                | The `cmd/` developer tools (generators and auditors)                       |
| [Static Analysis](static-analysis.md)                                     | The golangci-lint, govulncheck, and markdownlint gates                     |
| [Godoc Compliance](godoc.md)                                              | The godoc audit workflow for packages, symbols, and tests                  |
| [Token Footprint](token-footprint.md)                                     | Token accounting across tiers, surfaces, and schema modes                  |
| [Orbit Live Test Fixtures](orbit-fixtures.md)                             | Fixtures, setup, and the indexer caveat for GitLab.com live tests          |
| [Testing](testing/README.md)                                              | Unit, E2E, and AI model-evaluation documentation                           |
| [Architecture Decision Records](adr/README.md)                            | The recorded architectural decisions (ADRs)                                |

**Looking for something else?**
[Concepts](../concepts/README.md) for design rationale ·
[Reference](../reference/README.md) for tool/flag details ·
[../../CONTRIBUTING.md](../../CONTRIBUTING.md) for contribution mechanics.
