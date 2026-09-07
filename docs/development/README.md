# Development

**Contributor documentation** — everything you need to *change* gitlab-mcp-server.

This is the home for developer-facing material. Start with the
[Development Guide](development.md) for environment setup, building, testing, and
the workflow for adding a new tool. The remaining pages go deep on the runtime
architecture, the static-analysis and godoc gates, the dynamic search ranker,
token accounting, the live-test fixtures, and the release-pipeline settings
that live on GitHub rather than in this repository. Architectural Decision Records and
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
| [Resource Hot Spots](resource-hot-spots.md)                               | What a pooled credential costs in memory, what is shared, and what remains |
| [Orbit Live Test Fixtures](orbit-fixtures.md)                             | Fixtures, setup, and the indexer caveat for GitLab.com live tests          |
| [Enterprise Schema Checks](enterprise-schema-checks.md)                   | The unlicensed weekly re-probe, and the licensed pre-release run           |
| [Testing](testing/README.md)                                              | Unit, E2E, and AI model-evaluation documentation                           |
| [Architecture Decision Records](adr/README.md)                            | The recorded architectural decisions (ADRs)                                |
| [Upstream Bugs and Gaps](upstream-bugs.md)                                | Defects found in dependencies, and what we contributed back                |
| [Documentation Audit, September 2026](documentation-audit-2026-09.md)     | The five end-to-end paths executed, the organisation review, and the plan  |
| [Repository Settings](repository-settings.md)                             | Release-pipeline settings that live on GitHub rather than in the tree      |

**Looking for something else?**
[Concepts](../concepts/README.md) for design rationale ·
[Reference](../reference/README.md) for tool/flag details ·
[../../CONTRIBUTING.md](../../CONTRIBUTING.md) for contribution mechanics.
