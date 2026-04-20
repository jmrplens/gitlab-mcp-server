---
title: "ADR-0009: Progressive GraphQL migration strategy"
status: "Accepted"
date: "2026-04-20"
authors: "jmrplens"
tags: ["architecture", "decision", "graphql", "migration", "rest-api"]
superseded_by: ""
---

# ADR-0009: Progressive GraphQL migration strategy

## Status

**Accepted** — extends ADR-0006 (raw GraphQL.Do()) with a migration governance framework.

## Context

GitLab maintains two API surfaces: REST API v4 and GraphQL API. The project currently uses REST for ~155 domains and GraphQL for 7 domains (ADR-0006). GitLab has begun deprecating certain REST endpoints in favor of GraphQL equivalents:

- **Epics REST API**: deprecated since GitLab 17.0, removal planned for 19.0
- **Security Findings REST**: deprecated in favor of GraphQL `Pipeline.securityReportFindings`
- **Future deprecations**: GitLab's stated direction is GraphQL-first for new features

The question is whether to proactively migrate all domains to GraphQL, migrate reactively as deprecations occur, or maintain a hybrid approach.

### Options Considered

#### Option 1: Proactive full migration to GraphQL (rejected)

- **POS-001**: Unified API strategy, single query language
- **NEG-001**: Massive effort (~155 domains) with no functional benefit for stable REST endpoints
- **NEG-002**: `client-go` typed wrappers exist for REST but not for most GraphQL domains — migration would reduce type safety
- **NEG-003**: `CI_JOB_TOKEN` cannot authenticate GraphQL, breaking CI pipeline use cases
- **NEG-004**: GraphQL responses require manual struct maintenance without compile-time schema validation

#### Option 2: Never migrate — keep REST until removed (rejected)

- **POS-001**: Zero migration effort, stable codebase
- **NEG-001**: Deprecated endpoints emit warnings and may degrade
- **NEG-002**: Sudden removal at major GitLab versions forces emergency migrations
- **NEG-003**: Misses GraphQL-only features and performance benefits (aggregation)

#### Option 3: Progressive migration with defined triggers (accepted)

- **POS-001**: Migrations happen only when justified (deprecation, feature gap, performance)
- **POS-002**: Deprecation notices give users transition time before tool removal
- **POS-003**: Allows both REST and GraphQL tools to coexist during transition periods
- **POS-004**: Avoids unnecessary work on stable, fully-functional REST domains
- **NEG-001**: Requires monitoring GitLab deprecation announcements
- **NEG-002**: Temporary tool duplication during transition (old REST + new GraphQL)

## Decision

**Adopt a progressive, trigger-based migration strategy.** Domains migrate from REST to GraphQL only when one of these triggers occurs:

1. **REST endpoint deprecated** with a published removal version
2. **REST endpoint removed** in the target GitLab version
3. **New feature is GraphQL-only** (no REST equivalent)
4. **Significant functional gap** where GraphQL provides capabilities REST lacks
5. **Performance benefit** where GraphQL aggregation eliminates multiple REST calls

When migrating:

- Add deprecation notices to old REST tools immediately (reference migration version)
- Keep deprecated REST tools functional alongside new GraphQL tools during transition
- Remove deprecated REST tools only in a subsequent major version or when GitLab removes the endpoint
- Prefer `client-go` typed GraphQL wrappers when available; fall back to raw `GraphQL.Do()` per ADR-0006

## Consequences

- **POS-001**: Migrations are deliberate and justified, avoiding unnecessary churn
- **POS-002**: Users get advance notice via deprecation messages in tool descriptions
- **POS-003**: CI pipeline users retain REST fallback where `CI_JOB_TOKEN` is needed
- **POS-004**: New GraphQL-only features can be added immediately without waiting for REST
- **NEG-001**: Project must track GitLab deprecation announcements at each major release
- **NEG-002**: During transitions, tool inventory temporarily increases (deprecated + replacement tools)

## Migration Priority

Current priority queue based on known deprecation timelines:

| Priority | Domain | Trigger | Timeline |
| -------- | ------ | ------- | -------- |
| ✅ | Epics (6 tools) | REST deprecated 17.0, removal 19.0 | Migrated to Work Items API via client-go `WorkItems` service |
| ✅ | Epic Issues (4 tools) | REST deprecated 17.0, removal 19.0 | Migrated to Work Items children/parent widgets |
| ✅ | Epic Notes (5 tools) | REST deprecated 17.0, removal 19.0 | Migrated to Work Items notes widgets |
| ✅ | Epic Discussions (7 tools) | REST deprecated 17.0, removal 19.0 | Migrated to Work Items discussions widgets |
| P3 | Iterations | Feature gap | Migrate when client-go adds GraphQL wrapper |

## References

- [ADR-0006](adr-0006-raw-graphql-for-uncovered-domains.md) — raw GraphQL.Do() pattern
- [GraphQL Integration](../graphql.md) — current GraphQL patterns and utilities
- [GitLab REST API Deprecations](https://docs.gitlab.com/ee/api/rest/deprecations.html) — deprecation tracker
