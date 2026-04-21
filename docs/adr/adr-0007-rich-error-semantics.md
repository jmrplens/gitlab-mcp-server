---
title: "ADR-0007: Rich Error Semantics for LLM-Actionable Diagnostics"
status: "Accepted"
date: "2026-04-06"
authors: "jmrplens"
tags: ["architecture", "decision", "error-handling", "llm"]
superseded_by: ""
---

# ADR-0007: Rich Error Semantics for LLM-Actionable Diagnostics

## Status

**Accepted** — extends the existing error classification system with context-rich, LLM-actionable error messages.

## Context

The original error handling pipeline (`WrapErr` → `ClassifyError` → `ClassifyHTTPStatus`) produced generic messages based solely on HTTP status codes. For example, a 400 on `fileCreate` would yield:

```text
fileCreate: bad request — check your input parameters: POST .../files: 400
```

This tells the LLM *what* happened but not *why* or *what to do next*. The GitLab API response body often contains the specific reason (e.g., "A file with this name already exists"), but this detail was discarded by `WrapErr`.

LLMs using MCP tools need actionable error messages to self-correct without human intervention. A message like "A file with this name already exists — use gitlab_file_update to modify it" lets the LLM recover immediately.

### Options considered

#### Option 1: Enhance ClassifyError to include glErr.Message (rejected)

- **Pros**: Single function change, backward compatible
- **Cons**: Mixes responsibilities — `ClassifyError` is used for both read-only and mutating operations. Read-only operations rarely benefit from the extra detail, and the verbose message would clutter simple error displays.

#### Option 2: Centralized error registry mapping (domain, operation, status) → message (rejected)

- **Pros**: Single source of truth for all error messages
- **Cons**: Creates a massive central file disconnected from tool code. Violates the modular sub-package architecture (ADR-0004). Hard to maintain as tools evolve independently.

#### Option 3: Generic status-code dispatch helper `DiagnoseByStatus` (rejected)

- **Pros**: Reusable pattern for any tool
- **Cons**: The dispatch table approach forces all error hints into a rigid `map[int]func` structure. In practice, many hints need to inspect the error message content (not just status code) — e.g., distinguishing "branch already exists" from "invalid ref" on the same 400 status. Domain-specific inline logic is more precise.

#### Option 4: Layered error functions with domain-specific inline hints (accepted)

- **Pros**: Composable functions (`WrapErr` → `WrapErrWithMessage` → `WrapErrWithHint`) at increasing detail levels. Domain-specific hint logic stays in each sub-package. Read-only operations remain clean with `WrapErr`. Signature-compatible for easy migration.
- **Cons**: Requires manual per-domain work for specific hints. Some duplication of hint patterns across domains.

## Decision

Introduce three new functions in `internal/toolutil/errors.go` that layer on top of the existing `WrapErr`:

| Function | Purpose | When to use |
| --- | --- | --- |
| `ExtractGitLabMessage(err)` | Extracts specific detail from `gl.ErrorResponse.Message` | Building block for other functions |
| `WrapErrWithMessage(op, err)` | Like `WrapErr` but includes the GitLab error message | Mutating operations (default) |
| `WrapErrWithHint(op, err, hint)` | Like `WrapErrWithMessage` plus an actionable suggestion | When a specific corrective action is known |

### Error format progression

```text
WrapErr:            "op: classification: <original>"
WrapErrWithMessage: "op: classification — specific detail: <original>"
WrapErrWithHint:    "op: classification — specific detail. Suggestion: hint: <original>"
```

### Migration strategy

1. **Phase 1**: Implement infrastructure functions in `toolutil`
2. **Phases 2-3**: Upgrade high/medium priority mutating operations with `WrapErrWithHint` for common error codes (409, 400, 403, 404)
3. **Phase 4**: Bulk upgrade all remaining `WrapErr` calls in mutating operations to `WrapErrWithMessage` (signature-compatible, mechanical replacement)
4. **Keep `WrapErr`** for read-only operations where the generic classification suffices

## Consequences

### Positive

- **POS-001**: LLMs can self-correct from error messages without human intervention (e.g., "use gitlab_file_update" after a 400 on file create)
- **POS-002**: Preserves backward compatibility — `WrapErr` still works, `errors.As`/`errors.Is` chains unbroken
- **POS-003**: Domain-specific hints stay in each sub-package, following ADR-0004's modular architecture
- **POS-004**: Incremental adoption — each tool can be upgraded independently
- **POS-005**: `ExtractGitLabMessage` is defensive (truncation, format normalization, nil-safe)

### Negative

- **NEG-001**: Error messages are longer, which may increase token usage in LLM contexts
- **NEG-002**: Hint text must be maintained manually — stale hints (e.g., referencing renamed tools) won't be caught by the compiler
- **NEG-003**: `gl.ErrorResponse.Message` format varies across GitLab versions — `ExtractGitLabMessage` must be defensive

### Mitigations

- **NEG-001**: Messages are concise (one sentence diagnosis + one sentence suggestion). The extra tokens are worth the self-correction capability.
- **NEG-002**: E2E tests exercise the full error flow against a real GitLab instance. Stale tool references would surface as test failures.
- **NEG-003**: `ExtractGitLabMessage` falls back to the raw message when parsing fails, and truncates at 300 characters.

## References

- [Error Handling Documentation](../error-handling.md)
- [Development Guide: Error Handling](../development/development.md#error-handling-in-tool-handlers)
- [ADR-0004: Modular tools sub-packages](adr-0004-modular-tools-subpackages.md)
- `internal/toolutil/errors.go` — implementation
- `internal/toolutil/errors_test.go` — 40+ test cases
