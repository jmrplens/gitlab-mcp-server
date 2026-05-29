---
title: "ADR-0011: Low-token dynamic toolset mode"
status: "Accepted"
date: "2026-05-07"
authors: "jmrplens, GitHub Copilot"
tags: ["architecture", "decision", "mcp", "meta-tools", "tokens", "tool-discovery"]
supersedes: ""
superseded_by: "ADR-0014 for catalog source and compatibility ownership"
---

# ADR-0011: Low-token dynamic toolset mode

## Status

Status: Accepted.

## Context

ADR-0005 consolidated the GitLab MCP meta-tool catalog from 68 tools to 33 base tools, with 49 self-managed
Enterprise/Premium tools and 50 GitLab.com Enterprise/Premium tools when all gated capabilities are visible. This remains
the most compatible general-purpose mode and already reduces advertised tool-definition cost by about 89.5% versus the
individual tool catalog.

The May 2026 MCP ecosystem now includes stronger patterns for very large API surfaces:

- Cloudflare Code Mode exposes about 2,594 endpoints through 2 tools and reports about 1,069 initial tokens.
- Speakeasy Dynamic Toolsets expose `search_tools`, `describe_tools`, and `execute_action` and report about 90-96% token
  reductions in benchmarked workflows.
- Solo.io Agentgateway and Bifrost demonstrate gateway-side progressive disclosure and code-mode execution.
- MCP protocol proposals and SEP discussions point toward lazy schema hydration and scope-filtered discovery.

Local research on this repository shows that `META_PARAM_SCHEMA=compact` and `META_PARAM_SCHEMA=full` increase upfront
input schema cost by 6.5x and 12.2x respectively versus opaque mode. The next major reduction therefore requires a smaller
directly exposed tool surface and model-controlled discovery, not a larger upfront schema.

## Decision

Introduce a new low-token dynamic toolset mode as the primary candidate for further token reduction. The mode exposes
two plain MCP tools:

- `gitlab_find_action`: search the canonical GitLab action catalog and return exact schemas, examples, safety metadata, and output summaries for matching actions.
- `gitlab_execute_action`: execute one selected action by canonical `domain.action` ID with strict runtime validation.

The existing domain meta-tool mode and individual-tool mode remain available. The dynamic toolset mode starts behind an
explicit configuration flag and must pass evaluation gates before it can become the default.

## Consequences

### Positive

- **POS-001**: Reduces the initial visible tool count from 33/49/50 to 2 in low-token mode.
- **POS-002**: Keeps plain MCP compatibility because discovery and execution remain ordinary tool calls.
- **POS-003**: Reuses canonical `ActionSpec`/catalog routes, per-action schemas, handlers, markdown formatters, destructive flags,
  read-only mode, safe mode, and scope filtering.
- **POS-004**: Avoids arbitrary generated-code execution and the sandbox burden of Code Mode.
- **POS-005**: Aligns with Speakeasy-style dynamic toolsets, MCP lazy hydration direction, and provider-native tool search.
- **POS-006**: Preserves rollback because current meta-tools remain available.

### Negative

- **NEG-001**: Adds an additional discovery layer and likely increases tool calls per task.
- **NEG-002**: Search quality becomes a core product behavior and needs evaluation, ranking tests, and telemetry.
- **NEG-003**: Models may skip discovery and call `gitlab_execute_action` with invented action IDs.
- **NEG-004**: Action aliases and canonical `domain.action` naming add migration and documentation complexity.
- **NEG-005**: The low-token mode requires a new evaluation path before it can be trusted as default.

## Alternatives Considered

### Keep Current Domain Meta-Tools Only

- **ALT-001**: **Description**: Continue optimizing descriptions and schema resources within the 33/49/50 meta-tool model.
- **ALT-002**: **Rejection Reason**: This preserves compatibility but cannot reach the 2-4 visible tool target.

### Unified Dispatcher

- **ALT-003**: **Description**: Expose one `gitlab` dispatcher with `domain.action` values and optional schema actions.
- **ALT-004**: **Rejection Reason**: It has lower discoverability than explicit find/execute and may increase
  invented action IDs.

### Server-Side Code Mode

- **ALT-005**: **Description**: Expose `search` and `execute` tools that run generated code against a GitLab facade.
- **ALT-006**: **Rejection Reason**: Token upside is strongest, but security risk is much higher. It requires a sandbox ADR,
  threat model, and adversarial tests before production implementation.

### Gateway-Only Optimization

- **ALT-007**: **Description**: Recommend Agentgateway, Bifrost, Gram, or another proxy instead of changing this server.
- **ALT-008**: **Rejection Reason**: Useful for some deployments, but it does not improve the direct stdio/local default
  experience.

### Compact Or Full Meta Schemas By Default

- **ALT-009**: **Description**: Use `META_PARAM_SCHEMA=compact` or `META_PARAM_SCHEMA=full` as the default.
- **ALT-010**: **Rejection Reason**: Local audits show these modes increase upfront input schema size by 6.5x and 12.2x
  respectively.

## Implementation Notes

- **IMP-001**: Add the dynamic toolset behind the explicit `TOOL_SURFACE=dynamic`
  selector. Legacy `META_TOOLS=true|false` remains only as a compatibility fallback when `TOOL_SURFACE` is absent.
- **IMP-002**: Build the dynamic action view from the canonical action catalog shared with meta-tools, then apply
  enterprise, GitLab.com, exclude-tools, token-scope, read-only, and safe-mode behavior without constructing a separate
  MCP server.
- **IMP-003**: Use canonical `domain.action` IDs and keep aliases searchable but non-canonical.
- **IMP-004**: Use `toolutil.LookupMetaActionSchema` or equivalent deep-cloned schema lookup for find output and
  runtime validation.
- **IMP-005**: Return repairable validation failures as tool results with `isError: true`.
- **IMP-006**: Require `confirm:true` for destructive execution and preserve safe mode previews.
- **IMP-007**: Extend `cmd/eval_mcp_surfaces` to compare current meta-tools and the dynamic toolset.
- **IMP-008**: Add observability for find query, selected action, validation failure, policy block, and
  destructive confirmation events.

## Compliance Checklist

- **CHK-001**: Existing meta-tool mode remains available.
- **CHK-002**: Existing individual-tool mode remains available.
- **CHK-003**: Low-token mode exposes exactly 2 tools for `dynamic`.
- **CHK-004**: Low-token mode preserves destructive-action safety.
- **CHK-005**: Low-token mode works over stdio and Streamable HTTP.
- **CHK-006**: Evaluation shows no more than 2 percentage-point task success regression before default rollout.

## References

- **REF-001**: [ADR-0005: Meta-tool consolidation](adr-0005-meta-tool-consolidation.md)
- **REF-002**: Local research artifacts under `plan/architecture-tool-surface-token-reduction-research-1.md` and
  `plan/tool-surface-token-reduction-research/`.
- **REF-003**: [Cloudflare Code Mode MCP](https://blog.cloudflare.com/code-mode-mcp/)
- **REF-004**: [Speakeasy Dynamic Toolsets v2](https://www.speakeasy.com/blog/how-we-reduced-token-usage-by-100x-dynamic-toolsets-v2)
