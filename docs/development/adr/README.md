# Architectural Decision Records

This directory contains Architectural Decision Records (ADRs) for gitlab-mcp-server.

> **Diátaxis type**: Explanation
> **Audience**: Developers, architects
> **Prerequisites**: Familiarity with the project architecture

---

## ADR Index

| ADR                                                                              | Title                                                              | Status                                                  | Classification                                  | Date       |
| -------------------------------------------------------------------------------- | ------------------------------------------------------------------ | ------------------------------------------------------- | ----------------------------------------------- | ---------- |
| ADR-0001                                                                         | Go as implementation language                                      | Implicit founding decision (not formally recorded)      | Current                                         | —          |
| ADR-0002                                                                         | stdio as primary MCP transport                                     | Implicit founding decision (not formally recorded)      | Current                                         | —          |
| ADR-0003                                                                         | GitLab REST API v4 via official client                             | Implicit founding decision (not formally recorded)      | Current                                         | —          |
| [ADR-0004](adr-0004-modular-tools-subpackages.md)                                | Modular sub-packages under `internal/tools/{domain}/`              | Accepted; runtime registration superseded by ADR-0014   | Historical but useful                           | 2026-02-15 |
| [ADR-0005](adr-0005-meta-tool-consolidation.md)                                  | Meta-tool consolidation from 68 to a compact domain catalog        | Accepted; registration mechanics superseded by ADR-0014 | Historical but useful                           | 2026-03-06 |
| [ADR-0006](adr-0006-raw-graphql-for-uncovered-domains.md)                        | Raw GraphQL.Do() for domains without client-go service wrappers    | Accepted                                                | Current                                         | 2026-03-23 |
| [ADR-0007](adr-0007-rich-error-semantics.md)                                     | Rich error semantics for LLM-actionable diagnostics                | Accepted                                                | Current                                         | 2026-04-06 |
| [ADR-0008](adr-0008-universal-identity.md)                                       | Universal identity system                                          | Accepted, partially unimplemented (HTTP legacy mode)    | Current                                         | 2026-04-18 |
| [ADR-0009](adr-0009-progressive-graphql-migration.md)                            | Progressive GraphQL migration strategy                             | Accepted                                                | Current                                         | 2026-04-20 |
| [ADR-0010](adr-0010-no-resource-subscribe.md)                                    | No resource subscribe capability                                   | Superseded by ADR-0015                                  | Historical but useful                           | 2026-04-26 |
| [ADR-0011](adr-0011-low-token-dynamic-toolset.md)                                | Low-token dynamic toolset mode                                     | Accepted; catalog source refined by ADR-0014            | Current, with catalog-first terminology updates | 2026-05-07 |
| [ADR-0012](adr-0012-action-catalog-package-name.md)                              | Action catalog package name                                        | Accepted                                                | Current                                         | 2026-05-12 |
| [ADR-0013](adr-0013-documentation-artifact-boundaries.md)                        | Documentation artifact boundaries                                  | Accepted                                                | Current                                         | 2026-05-13 |
| [ADR-0014](adr-0014-catalog-first-runtime-architecture.md)                       | Catalog-first runtime architecture                                 | Accepted                                                | Current                                         | 2026-05-15 |
| [ADR-0015](adr-0015-polled-resource-subscriptions.md)                            | Polled resource subscriptions (supersedes ADR-0010)                | Accepted                                                | Current                                         | 2026-08-24 |
| [ADR-0016](adr-0016-no-webhook-ingestion.md)                                     | No webhook ingestion                                               | Accepted                                                | Current                                         | 2026-08-25 |
| [ADR-0017](adr-0017-pull-safe-event-sources-surveyed.md)                         | Pull-safe event sources surveyed and declined                      | Accepted                                                | Current                                         | 2026-08-25 |
| [ADR-0018](adr-0018-authorization-admits-per-action-gating.md)                   | Authorization admits at the minimum scope; writes gated per action | Accepted                                                | Current                                         | 2026-08-28 |
| [ADR-0019](adr-0019-audience-binding-unavailable-at-the-authorization-server.md) | Audience binding is unavailable at the authorization server        | Accepted                                                | Current                                         | 2026-08-29 |

## About Missing ADRs

ADR-0001 through ADR-0003 were founding decisions made at project inception and not formally recorded as ADR documents. Their outcomes are reflected throughout the codebase:

- **ADR-0001 (Go)**: Go 1.27+ is the sole implementation language — see [go.mod](../../../go.mod)
- **ADR-0002 (stdio transport)**: stdio is the primary transport — see [cmd/server/main.go](../../../cmd/server/main.go)
- **ADR-0003 (GitLab REST API v4)**: Uses `gitlab.com/gitlab-org/api/client-go/v2` — see [go.mod](../../../go.mod)

ADR-0004 is now a standalone document. It was previously referenced only in the [Architecture](../../concepts/architecture.md) documentation.

## ADR Format

New ADRs follow the template in `.github/skills/create-architectural-decision-record/`. Each ADR includes:

- YAML front matter (title, status, date, authors, tags). ADR-0010 and ADR-0015 through ADR-0019 carry none and state their status and date in the Status section instead
- Context, decision drivers, and options considered
- Decision outcome with positive/negative consequences
- Compliance checklist

## AI Guidance: ADRs Are Not Absolute

ADRs capture the reasoning behind decisions **at the time they were made**. They are context, not commandments.

When working on improvements or new features, AI assistants should:

- **Prioritize current knowledge over ADR prescriptions.** If a better approach exists today (e.g., new SDK capabilities, improved patterns, lessons learned), prefer the better approach.
- **Treat ADRs as historical context**, not as immutable rules. They explain *why* a decision was made, but that reasoning may no longer apply.
- **Propose superseding an ADR** when a change contradicts it. Create a new ADR that references and supersedes the old one, documenting why the new approach is better.
- **Never blindly follow an ADR** that conflicts with observable best practices, test results, or measurable improvements in the current codebase.
