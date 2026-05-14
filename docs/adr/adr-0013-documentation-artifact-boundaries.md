---
title: "ADR-0013: Documentation artifact boundaries"
status: "Accepted"
date: "2026-05-13"
authors: "jmrplens, GitHub Copilot"
tags: ["documentation", "decision", "testing", "evaluation", "ai-guidance"]
supersedes: ""
superseded_by: ""
---

# ADR-0013: Documentation Artifact Boundaries

## Status

Status: Accepted.

## Context

The project has several documentation surfaces with different lifecycles:

- Stable developer documentation under `docs/`.
- User-facing Starlight documentation under `site/src/content/docs/`.
- AI guidance files under `.github/`, `AGENTS.md`, `CLAUDE.md`, `llms.txt`, and `llms-full.txt`.
- Generated current-state blocks in `README.md`, `docs/testing/testing.md`, and `docs/testing/model-results.md`.
- Temporary implementation plans, local evaluation reports, traces, and progress artifacts.

The documentation should remain useful as product and architecture reference material. At the same time, command-managed
blocks are part of the supported project workflow and must remain updateable by the generators in `cmd/`.

Without an explicit boundary, handwritten documentation can drift into a mixture of durable reference content,
implementation progress notes, local test outputs, PR status, and one-off evaluation evidence. That makes the docs harder
to maintain and can confuse users and AI assistants about what is stable project behavior.

## Decision

Keep stable documentation focused on durable behavior, architecture, reference material, and reproducible procedures.

Allow command-managed generated blocks in documentation when they are intentionally maintained by repository tools in
`cmd/`. These blocks may contain current counts, model evaluation summaries, unit test statistics, E2E coverage summaries,
and tool catalog statistics. The source of truth for those sections is the generator command, not hand editing.

Move or omit transient progress artifacts from stable documentation. Examples include ad hoc local test output, benchmark
snapshots tied to a workstation, PR-by-PR completion logs, one-off model run traces, and temporary report filenames.
Those artifacts belong in one of these places:

- `plan/` when they are part of an implementation roadmap or decision gate.
- An ADR when they justify an architectural decision.
- Ignored `dist/` reports when they are generated evidence or local evaluation output.
- Commit or pull request discussion when they are review context.

Prefer Mermaid diagrams over ASCII diagrams in documentation and AI guidance files when a diagram clarifies architecture,
flows, or workflows. ASCII diagrams may remain only when the exact text layout is itself the artifact being documented.

## Consequences

### Positive

- **POS-001**: Stable docs stay readable as reference material instead of becoming progress logs.
- **POS-002**: Generated stats and model-result blocks remain part of the supported release and evaluation workflow.
- **POS-003**: AI assistants get clearer signals about what is stable project behavior versus local implementation history.
- **POS-004**: Mermaid diagrams improve readability in GitHub, Starlight, and editor previews.

### Negative

- **NEG-001**: Some historical evidence must be preserved elsewhere when it is still useful.
- **NEG-002**: Contributors must distinguish generated blocks from handwritten documentation before editing.
- **NEG-003**: Mermaid diagrams require Markdown tooling and Starlight rendering paths to support code fences correctly.

## Alternatives Considered

### Remove All Results From Documentation

- **ALT-001**: **Description**: Delete model evaluation summaries, test statistics, and tool catalog stats from docs and
  README.
- **ALT-002**: **Rejection Reason**: Those sections are intentionally managed by repository commands and are part of the
  normal validation and publishing workflow.

### Keep All Progress Evidence In Place

- **ALT-003**: **Description**: Allow local outputs, PR progress, benchmark snapshots, and generated blocks anywhere in
  stable docs.
- **ALT-004**: **Rejection Reason**: This makes documentation noisy and short-lived, and it weakens the distinction between
  reference docs and implementation notes.

### Separate Generated Blocks From Transient Notes

- **ALT-005**: **Description**: Preserve command-managed generated blocks while moving transient progress evidence to
  plans, ADRs, ignored reports, or review discussions.
- **ALT-006**: **Selection Reason**: This keeps supported automation intact while improving documentation quality and
  reducing stale context.

## Implementation Notes

- **IMP-001**: Do not remove `<!-- START ... -->` and `<!-- END ... -->` managed blocks unless the owning generator is
  intentionally changed.
- **IMP-002**: Update generators in `cmd/` when generated content needs structural changes.
- **IMP-003**: Convert architecture and workflow ASCII diagrams to Mermaid as docs are touched.
- **IMP-004**: Keep local evaluation reports and traces under ignored output directories such as `dist/`.
- **IMP-005**: Use `plan/` for implementation status and ADRs for durable decisions.

## References

- **REF-001**: [Testing documentation](../testing/testing.md)
- **REF-002**: [Model evaluation results](../testing/model-results.md)
- **REF-003**: [Tool surfaces and action core](../development/tool-surfaces-and-action-core.md)
- **REF-004**: [README](../../README.md)
