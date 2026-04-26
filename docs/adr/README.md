# Architectural Decision Records

This directory contains Architectural Decision Records (ADRs) for gitlab-mcp-server.

> **Diátaxis type**: Explanation
> **Audience**: Developers, architects
> **Prerequisites**: Familiarity with the project architecture

---

## ADR Index

| ADR | Title | Status | Date |
| --- | --- | --- | --- |
| ADR-0001 | Go as implementation language | Implicit founding decision (not formally recorded) | — |
| ADR-0002 | stdio as primary MCP transport | Implicit founding decision (not formally recorded) | — |
| ADR-0003 | GitLab REST API v4 via official client | Implicit founding decision (not formally recorded) | — |
| [ADR-0004](adr-0004-modular-tools-subpackages.md) | Modular sub-packages under `internal/tools/{domain}/` | Accepted | 2026-02-15 |
| [ADR-0005](adr-0005-meta-tool-consolidation.md) | Meta-tool consolidation from 70 to 27 domain tools | Accepted | 2026-03-06 |
| [ADR-0006](adr-0006-raw-graphql-for-uncovered-domains.md) | Raw GraphQL.Do() for domains without client-go service wrappers | Accepted | 2026-03-23 |
| [ADR-0007](adr-0007-rich-error-semantics.md) | Rich error semantics for LLM-actionable diagnostics | Accepted | 2026-04-06 |
| [ADR-0008](adr-0008-universal-identity.md) | Universal identity system | Accepted | 2026-04-13 |
| [ADR-0009](adr-0009-progressive-graphql-migration.md) | Progressive GraphQL migration strategy | Accepted | 2026-04-20 |
| [ADR-0010](adr-0010-no-resource-subscribe.md) | No resource subscribe capability | Accepted | 2026-04-26 |

## About Missing ADRs

ADR-0001 through ADR-0003 were founding decisions made at project inception and not formally recorded as ADR documents. Their outcomes are reflected throughout the codebase:

- **ADR-0001 (Go)**: Go 1.26+ is the sole implementation language — see [go.mod](../../go.mod)
- **ADR-0002 (stdio transport)**: stdio is the primary transport — see [cmd/server/main.go](../../cmd/server/main.go)
- **ADR-0003 (GitLab REST API v4)**: Uses `gitlab.com/gitlab-org/api/client-go/v2` — see [go.mod](../../go.mod)

ADR-0004 is now a standalone document. It was previously referenced only in the [Architecture](../architecture.md) documentation.

## ADR Format

New ADRs follow the template in `.github/skills/create-architectural-decision-record/`. Each ADR includes:

- YAML front matter (title, status, date, authors, tags)
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
