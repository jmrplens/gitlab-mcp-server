---
title: "ADR-0012: Action catalog package name"
status: "Accepted"
date: "2026-05-12"
authors: "jmrplens, GitHub Copilot"
tags: ["architecture", "decision", "mcp", "tool-surfaces", "action-catalog"]
supersedes: ""
superseded_by: ""
---

# ADR-0012: Action Catalog Package Name

## Status

Status: Accepted.

## Context

The project now has an explicit canonical action core shared by meta-tools and dynamic tools. Earlier implementation work
placed the core data model under `internal/tools/actionregistry`, which accurately described a lookup structure but did
not describe the architectural role of the package.

After removing the production dependency on legacy meta-registration side effects, the package is no longer just a registry-like helper. It
owns the canonical action catalog data model, deterministic action ordering, `domain.action` IDs, catalog filters, and
compatibility adapters used by schema resources, audits, meta registration, and dynamic discovery.

The package name should make that ownership obvious without implying that individual MCP tools are registered through
the same path.

## Decision

Rename `internal/tools/actionregistry` to `internal/tools/actioncatalog`.

Keep the package under `internal/tools` because its current consumers are tool-surface builders, audits, and command
helpers that already depend on the `internal/tools` boundary. Do not move it to `internal/actioncatalog` until a stronger
cross-domain consumer exists outside the tool-surface architecture.

Keep exported type names stable: `Catalog`, `Group`, `Action`, `ActionID`, `GroupOptions`, and `FilterOptions` remain
unchanged. The change is a package-path and package-name clarification, not an API redesign.

## Consequences

### Positive

- **POS-001**: The import path now matches the established term `canonical action catalog` used in developer docs.
- **POS-002**: The package is less likely to be confused with individual MCP tool registration.
- **POS-003**: The rename is mechanical and keeps exported type names stable, reducing downstream churn inside the repo.
- **POS-004**: The package remains close to `internal/tools` route builders, dynamic adapters, and meta registration.

### Negative

- **NEG-001**: The rename touches imports, selector names, docs, and AI context files even though behavior is unchanged.
- **NEG-002**: Local branches that imported `actionregistry` must rebase and update package references.
- **NEG-003**: The package still depends on `internal/toolutil`, so it remains coupled to route metadata until a later
  builder-extraction chapter reduces that coupling.

## Alternatives Considered

### Keep `internal/tools/actionregistry`

- **ALT-001**: **Description**: Preserve the current package name and rely on documentation to explain the canonical
  catalog role.
- **ALT-002**: **Rejection Reason**: The name reinforces the old mental model of a generic registry and does not match the
  architecture language now used by meta and dynamic surfaces.

### Rename To `internal/tools/actioncatalog`

- **ALT-003**: **Description**: Rename only the package path and package declaration while keeping exported types stable.
- **ALT-004**: **Selection Reason**: This option gives the clearest name with limited mechanical churn and preserves the
  existing `internal/tools` ownership boundary.

### Move To `internal/actioncatalog`

- **ALT-005**: **Description**: Move the package out of `internal/tools` to make it appear as a standalone core package.
- **ALT-006**: **Rejection Reason**: Current consumers are still tool-surface-specific. Moving the package higher would
  imply a broader cross-domain contract before one exists and could obscure its dependency on route metadata.

## Implementation Notes

- **IMP-001**: Move the package directory to `internal/tools/actioncatalog`.
- **IMP-002**: Rename `registry.go` and `registry_test.go` to `catalog.go` and `catalog_test.go`.
- **IMP-003**: Update imports and selectors from `actionregistry` to `actioncatalog`.
- **IMP-004**: Update developer docs and AI context files that reference the old package path.
- **IMP-005**: Run targeted tests for `internal/tools/actioncatalog`, `internal/tools`, `internal/tools/dynamic`, and
  command packages that construct dynamic catalog metrics.

## References

- **REF-001**: [ADR-0005: Meta-tool consolidation](adr-0005-meta-tool-consolidation.md)
- **REF-002**: [ADR-0011: Low-token dynamic toolset mode](adr-0011-low-token-dynamic-toolset.md)
- **REF-003**: [Tool Surfaces And Action Core](../development/tool-surfaces-and-action-core.md)
