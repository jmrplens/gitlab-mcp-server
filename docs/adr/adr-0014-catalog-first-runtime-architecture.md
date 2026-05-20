---
title: "ADR-0014: Catalog-first runtime architecture"
status: "Accepted"
date: "2026-05-15"
authors: "jmrplens, GitHub Copilot"
tags: ["architecture", "decision", "action-spec", "action-catalog", "tool-surfaces"]
supersedes: "ADR-0004 runtime registration mechanics; ADR-0005 meta registration mechanics; ADR-0011 dynamic catalog source wording"
superseded_by: ""
---

# ADR-0014: Catalog-First Runtime Architecture

## Status

Status: Accepted.

This ADR supersedes the runtime registration mechanics described in ADR-0004 and ADR-0005 while preserving their domain package and domain meta-tool decisions. It also refines ADR-0011 by making the canonical action catalog, not `ActionMap` route capture, the source for Dynamic controller surfaces.

## Context

The project now exposes several MCP tool surfaces over the same GitLab API behavior:

- `meta`: compact domain meta-tools with action dispatch.
- `individual`: one visible MCP tool per GitLab operation.
- `dynamic`: find and execute over the action catalog.
- Standalone surface tools for project discovery, interactive elicitation, server maintenance, and other non-standard flows.

Earlier architecture evolved through package-local registration functions and captured meta-tool definitions. That created a hybrid runtime where metadata could drift between individual tools, meta-tools, Dynamic discovery, tool manifests, generated LLM files, and audits.

## Decision

Use the canonical `ActionSpec` and action catalog as the source of truth for ordinary GitLab API actions.

Domain packages own handlers, typed input/output structs, Markdown formatters, and `ActionSpecs`. Catalog group specs own the visible meta-tool grouping and surface metadata. `BuildActionCatalog` consumes those specs directly and validates ownership, schema presence, action identity, compatibility policy, and surface classification.

Root runtime registration is catalog-backed:

- `RegisterAllMeta` registers catalog-projected domain meta-tools plus approved standalone surface specs.
- `RegisterAll` registers individual tools through catalog projection.
- Dynamic find/execute builds its registry from the same catalog.
- Tool manifest resources, LLM files, audits, metrics, and evaluation tooling read the same catalog.

Package-local `RegisterTools` files have been removed from ordinary GitLab API domains. New ordinary GitLab actions must use domain-local `ActionSpecs` and catalog-backed projection rather than introducing package-local registration functions. Package-level `RegisterMeta` is not an approved path for ordinary GitLab API actions.

`TOOL_SURFACE` is the canonical tool selector. `META_TOOLS` remains a deprecated compatibility fallback for one compatibility window when `TOOL_SURFACE` is absent.

`META_PARAM_SCHEMA=opaque|compact|full` remains a meta-tool `tools/list` schema strategy only. It does not change handler validation, the `gitlab://tools` manifest, dynamic discovery output, or individual tool schemas.

`CAPABILITY_SURFACE=full|minimal` remains a separate resource and prompt exposure axis. `minimal` removes optional resources, prompts, and workflow guides while preserving `gitlab://workspace/roots` plus the surface-aware `gitlab://tools` manifest. `gitlab_find_action` still returns schemas inline for dynamic minimal deployments.

Action-specific aliases and parameter aliases belong to the spec/catalog compatibility policy through `internal/tools/actioncompat`. Dynamic may own generic search, typo tolerance, ranking, and execution flow, but it must not become a second home for action-owned compatibility data.

## Consequences

### Positive

- **POS-001**: One catalog feeds meta, individual, Dynamic, schemas, docs, audits, and model evaluations.
- **POS-002**: Runtime behavior no longer depends on capturing registration side effects.
- **POS-003**: Adding an ordinary GitLab API action has one metadata path: handler plus `ActionSpec` plus audited catalog aggregation.
- **POS-004**: Compatibility aliases and parameter aliases have explicit ownership, source, searchability, deprecation, and audit coverage.
- **POS-005**: `TOOL_SURFACE` expresses all visible tool modes without overloading a legacy boolean.
- **POS-006**: Standalone tools have a first-class surface policy instead of informal exceptions.

### Negative

- **NEG-001**: The catalog aggregation path is still a central build artifact and must remain audited or generated to avoid drift.
- **NEG-002**: Stale examples can still imply package-local registration is valid unless AI guidance and docs stay current.
- **NEG-003**: Model-backed validation remains necessary because catalog correctness does not guarantee model selection quality.

## Implementation Notes

- **IMP-001**: `internal/tools/action_specs_manifest_gen.go` is the audited aggregation point for domain spec builders.
- **IMP-002**: `cmd/audit_action_spec_coverage` enforces catalog coverage, no production legacy bridge calls, and no stale AI instruction guidance that recommends package-level `RegisterMeta` or domain `RegisterTools` as final patterns.
- **IMP-003**: `cmd/audit_dynamic_aliases` enforces Dynamic compatibility alias ownership.
- **IMP-004**: Model-backed evaluation reports live under ignored `dist/evaluation/mcp-surfaces/`; published baselines are generated into `docs/testing/model-results.md` only after review.

## Compliance Checklist

- [x] `BuildActionCatalog` does not call `toolutil.CaptureMetaToolDefinitions`, `registerAllMetaGroups`, or package-level `RegisterMeta`.
- [x] `RegisterAll` does not call per-domain `RegisterTools` for ordinary GitLab actions.
- [x] Ordinary GitLab API domains no longer define package-local `RegisterTools` functions.
- [x] Meta-tools, Dynamic, tool manifest resources, and individual projection consume the canonical catalog.
- [x] `TOOL_SURFACE` is documented as canonical; `META_TOOLS` is compatibility only.
- [x] Dynamic compatibility aliases and parameter aliases are catalog/spec policy data.
- [x] AI instructions, skills, and ADR index point future contributors to the catalog-first workflow.

## References

- **REF-001**: [Tool surfaces and action core](../development/tool-surfaces-and-action-core.md)
- **REF-002**: [Catalog-first individual tools](../development/catalog-first-individual-tools.md)
- **REF-003**: [ADR-0004: Modular sub-packages](adr-0004-modular-tools-subpackages.md)
- **REF-004**: [ADR-0005: Meta-tool consolidation](adr-0005-meta-tool-consolidation.md)
- **REF-005**: [ADR-0011: Low-token dynamic toolset mode](adr-0011-low-token-dynamic-toolset.md)
