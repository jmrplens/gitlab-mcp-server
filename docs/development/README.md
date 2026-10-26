# Development

**Contributor documentation**: everything you need to *change* gitlab-mcp-server.

This is the home for developer-facing material. Documentation for people who
install, configure and use the server lives only on the
[documentation site](https://docs.jmrp.io/gitlab-mcp-server/), in English and
Spanish; a page here links to it rather than repeating it. Start with the
[Development Guide](development.md) for environment setup, building, testing, and
the workflow for adding a new tool. The remaining pages go deep on the internal
architecture, error handling, the GraphQL integration, the capability APIs,
the tool surfaces, the static-analysis and godoc gates, the dynamic search
ranker, token accounting, the live-test fixtures, how the release artifacts are
built and published, and the release-pipeline settings that live on GitHub
rather than in this repository. Architectural Decision Records and the full
testing reference live in subfolders here.

> **Diátaxis type**: How-to & Reference · **Audience**: 🛠️ Contributors & maintainers

| Document                                                                  | Purpose                                                                                                   |
| ------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| [Development Guide](development.md)                                       | Setup, building, testing, and adding new tools                                                            |
| [Repository Map](repository-map.md)                                       | The main directories, every command, and the packages under `internal/` and `cmd/internal/`               |
| [Environment Variables and Flags](configuration.md)                       | Every setting, the code that reads it, its bounds and the register row behind it                          |
| [Internal Architecture](architecture.md)                                  | The packages, what each owns, the path a call takes, and the handler patterns                             |
| [HTTP Mode Invariants](http-invariants.md)                                | Admission, per-action write gating, routing, and how a call is bound to its credential                    |
| [Resource Subscriptions](resource-subscriptions.md)                       | How subscriptions are honored by polling, and how a watch is renewed and ended                            |
| [Tier Gating](tier-gating.md)                                             | What each licensing tier adds to the catalog and the schemas, surface by surface                          |
| [Tool Surfaces & Canonical Action Core](tool-surfaces-and-action-core.md) | How individual, meta, and dynamic surfaces project from the shared catalog                                |
| [Error Handling](error-handling.md)                                       | Classification, the wrapping functions, not-found results, and testing errors                             |
| [GraphQL Integration](graphql.md)                                         | When a domain uses GraphQL, the shared helpers, and the pinned schema it is held to                       |
| [Capabilities and Icons](capabilities.md)                                 | The progress, elicitation and completion APIs, and how the icons are built                                |
| [Catalog-First Individual Tools](catalog-first-individual-tools.md)       | Evaluation of generating individual tools from the canonical catalog                                      |
| [Dynamic Search Ranker](dynamic-search-ranker.md)                         | How `gitlab_find_action` ranks and matches queries                                                        |
| [Command-Line Utilities](cmd-utilities.md)                                | The `cmd/` developer tools (generators and auditors)                                                      |
| [Static Analysis](static-analysis.md)                                     | The golangci-lint, govulncheck, and markdownlint gates                                                    |
| [Godoc Compliance](godoc.md)                                              | The godoc audit workflow for packages, symbols, and tests                                                 |
| [Token Footprint](token-footprint.md)                                     | Token accounting across tiers, surfaces, and schema modes                                                 |
| [Resource Hot Spots](resource-hot-spots.md)                               | What a pooled credential costs in memory, what is shared, and what remains                                |
| [Tenant Policy](tenant-policy-spec.md)                                    | Who a caller is, what each key costs to mint, and the invariants every limit meets                        |
| [Adding or Changing a Limit](adding-a-limit.md)                           | The procedure a new limit, or a change to one, follows under the tenant policy register                   |
| [Request Inventory](cmd-utilities.md#gen_request_inventory)               | What a row of the generated `request-inventory.json` means, and what it does not                          |
| [Request Paths and Fine-Grained Grants](request-paths.md)                 | R-PATH, R-PAGE and R-GRANT: the audit dimensions that read the requests a handler sends                   |
| [Orbit Live Test Fixtures](orbit-fixtures.md)                             | Fixtures, setup, and the indexer caveat for GitLab.com live tests                                         |
| [Enterprise Schema Checks](enterprise-schema-checks.md)                   | The unlicensed weekly re-probe, and the licensed pre-release run                                          |
| [Testing](testing/README.md)                                              | Unit, E2E, and AI model-evaluation documentation                                                          |
| [Test Infrastructure](test-infrastructure.md)                             | The shared test transport, and the rules every unit test is held to                                       |
| [Architecture Decision Records](adr/README.md)                            | The recorded architectural decisions (ADRs)                                                               |
| [Upstream Bugs and Gaps](upstream-bugs.md)                                | Defects found in dependencies, and what we contributed back                                               |
| [Documentation Audit, September 2026](documentation-audit-2026-09.md)     | The five end-to-end paths executed, the organisation review, and the plan                                 |
| [Security Review, October 2026](security-review-2026-10.md)               | SECURITY.md's requirements and trust boundaries checked against the code, the tools run, and the findings |
| [Distribution](distribution.md)                                           | How the Claude Desktop bundles and the package channels are built and published                           |
| [Release Process](release-process.md)                                     | How a release is rehearsed, cut and published on every channel                                            |
| [Repository Settings](repository-settings.md)                             | Settings that live outside the tree: on GitHub, the package registries and Cloudflare                     |

**Looking for something else?**
[Architecture](https://docs.jmrp.io/gitlab-mcp-server/architecture/) for design rationale ·
[Tool reference](https://docs.jmrp.io/gitlab-mcp-server/reference/tools/) and
[CLI reference](https://docs.jmrp.io/gitlab-mcp-server/reference/cli/) for tool/flag details ·
[../../CONTRIBUTING.md](../../CONTRIBUTING.md) for contribution mechanics.
