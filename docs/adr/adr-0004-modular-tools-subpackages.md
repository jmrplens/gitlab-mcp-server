---
title: "ADR-0004: Modular sub-packages under internal/tools/{domain}/"
status: "Accepted"
date: "2026-02-15"
authors: "jmrplens"
tags: ["architecture", "decision", "modular", "sub-packages", "domain-isolation"]
superseded_by: "ADR-0014 for runtime registration mechanics"
---

# ADR-0004: Modular sub-packages under internal/tools/{domain}/

## Status

**Accepted** — refined by [ADR-0005](adr-0005-meta-tool-consolidation.md) (meta-tool consolidation). Runtime registration mechanics are superseded by the catalog-first architecture in [tool surfaces and action core](../development/tool-surfaces-and-action-core.md): domain sub-packages remain the ownership boundary, but root tool surfaces are projected from canonical `ActionSpec` metadata.

## Context

As the gitlab-mcp-server project grew from a handful of GitLab tools to hundreds, the original monolithic `internal/tools/` package became untenable:

1. **Single package bloat**: All tool types, handlers, formatters, and registration functions lived in one package. With 100+ tools, the package exceeded 15,000 lines and dozens of files with no clear ownership boundaries.
2. **Name collisions**: Tool input/output structs required domain prefixes to avoid conflicts (e.g., `BranchListInput`, `BranchCreateInput`, `MRListInput`, `MRCreateInput`). This made type names verbose and error-prone.
3. **Testing isolation**: All tool tests ran in the same package, sharing test helpers and mock servers. A change in one domain's test could affect another's. Test runs were slow because all tests compiled together.
4. **Import cycles**: As shared utilities grew, it became difficult to factor out helpers without creating circular dependencies within the single package.
5. **Discoverability**: New contributors had to navigate a flat list of 50+ files to find the right handler.

### Decision Drivers

- Each GitLab API domain (branches, issues, MRs, pipelines, etc.) is functionally independent
- Tools within a domain share types but rarely share types across domains
- Independent testability is critical for a project with 750+ tools
- The Go ecosystem favors small, focused packages over large monolithic ones

## Options Considered

### Option A: Keep monolithic package with naming conventions

Continue with a single `internal/tools/` package but enforce strict naming conventions (`{Domain}{Action}Input`, etc.) and file grouping (`branches_*.go`, `issues_*.go`).

- **POS-001**: No structural change needed
- **NEG-001**: Name collisions worsen as tool count grows
- **NEG-002**: All tests still compile and run together
- **NEG-003**: No package-level encapsulation

### Option B: Domain sub-packages (selected)

Split `internal/tools/` into domain sub-packages: `internal/tools/branches/`, `internal/tools/issues/`, etc. Each sub-package owns its types, handlers, Markdown formatters, and canonical action specs.

- **POS-001**: Package namespace eliminates domain prefixes (`branches.Output` vs `BranchOutput`)
- **POS-002**: Independent compilation and testing per domain
- **POS-003**: Clear ownership and discoverability
- **POS-004**: Zero import cycles — sub-packages import `toolutil/`, never each other
- **NEG-001**: More directories and files to navigate
- **NEG-002**: Orchestration layer needed to wire all sub-packages into runtime tool surfaces

### Option C: Separate Go modules per domain

Publish each domain as a separate Go module (`go.mod` per domain) for fully independent versioning.

- **POS-001**: Fully independent builds and releases
- **NEG-001**: Massive Go module management overhead for 100+ modules
- **NEG-002**: Shared infrastructure (`toolutil/`) becomes an external dependency
- **NEG-003**: Overkill — all tools ship in a single binary

## Decision

**Option B: domain sub-packages under `internal/tools/{domain}/`**.

Each domain gets its own sub-package with a standard structure:

```text
internal/tools/{domain}/
├── {domain}.go          # Types (Input/Output structs) and handler functions
├── {domain}_test.go     # Table-driven tests with httptest mocks
├── action_specs.go      # ActionSpecs(client) — canonical route metadata
├── markdown.go          # Markdown formatters with content annotations
```

The current orchestration layer in `internal/tools/` builds surfaces from catalog metadata:

- `register.go` → projects individual tools from the canonical action catalog
- `register_meta.go` → registers catalog-backed meta groups and standalone surface tools
- `markdown.go` → delegates to the type-based Markdown registry

Package-local `RegisterTools` functions have since been removed for ordinary GitLab API domains. See [ADR-0014](adr-0014-catalog-first-runtime-architecture.md) for the catalog-first runtime that supersedes the original registration mechanics.

### Conventions

- **No domain prefix on types**: Use `branches.Output`, not `BranchOutput`. The package provides the namespace.
- **Sub-packages import only from `toolutil/`**: Never from sibling sub-packages. Shared logic goes in `toolutil/`.
- **Each sub-package is independently testable**: Uses `testutil.NewTestClient()` and `httptest` for isolated mocking.
- **Standard file layout**: Every sub-package follows the same structure for consistency.

## Consequences

### Positive

- **POS-001**: Package-level namespace eliminates verbose type prefixes (saves ~5 characters per type)
- **POS-002**: Independent test compilation — `go test ./internal/tools/branches/` runs only branch tests
- **POS-003**: Clear domain boundaries — each sub-package is self-contained and independently reviewable
- **POS-004**: Zero import cycles enforced by Go compiler — sub-packages cannot import each other
- **POS-005**: Scales to 160+ sub-packages and 1000+ Enterprise/Premium tools without package-level congestion
- **POS-006**: New tools follow a repeatable, discoverable pattern

### Negative

- **NEG-001**: Directory count increased from 1 to 157+ under `internal/tools/`
- **NEG-002**: Catalog aggregation and Markdown registration must be updated when adding new domains
- **NEG-003**: Cross-domain operations (rare) require shared types in `toolutil/`

## Compliance Checklist

- [x] Sub-packages never import sibling sub-packages
- [x] All shared types live in `toolutil/` or `testutil/`
- [x] Runtime-visible actions have `ActionSpec` coverage and tests
- [x] Root registration is catalog-backed rather than a per-domain registration loop
- [x] Standard file layout followed across all 163 sub-packages
