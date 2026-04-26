---
goal: Roll out targeted WrapErrWithHint coverage across all 153 tool sub-packages and (stretch) capture per-action OutputSchema for llms-full.txt enrichment
version: 1.0
date_created: 2026-04-26
last_updated: 2026-04-26
owner: gitlab-mcp-server maintainers
status: 'In Progress'
tags: [feature, llm-ergonomics, error-handling, observability, refactor]
---

# Introduction

![Status: Planned](https://img.shields.io/badge/status-Planned-blue)

The first ergonomics audit (commits `97cea0eb`, `b1f09dbb`, `8f5db3f5`) covered only 9 hand-picked
handlers in `issues`, `mergerequests`, `projects`. The remaining **1227 `WrapErrWithMessage` call
sites** across **152 sub-packages** still return the generic GitLab status message without
actionable next-step guidance for the LLM. This plan rolls out targeted `WrapErrWithHint` coverage
across all of them, one handler at a time, with full context for every change.

A stretch Phase 7 captures per-action `OutputSchema` data on `ActionRoute` (without polluting
`tools/list`) so `cmd/gen_llms` can enrich `llms-full.txt` with per-action result shapes — making
the file a genuinely complete LLM-discoverable contract.

## 1. Requirements & Constraints

- **REQ-001**: Every handler currently calling `toolutil.WrapErrWithMessage` must be inspected; if a
  high-confidence corrective action exists for a specific HTTP status, the call must be wrapped in
  an `if toolutil.IsHTTPStatus(err, code) { return ..., toolutil.WrapErrWithHint(...) }` branch
  with the generic `WrapErrWithMessage` retained as fallback.
- **REQ-002**: Hints MUST start with an imperative verb and reference a concrete corrective MCP
  tool when applicable (e.g., `"verify project_id with gitlab_project_get"`).
- **REQ-003**: Read-only get/list handlers receive 404 hints only (no destructive guidance).
- **REQ-004**: Mutating handlers (Create/Update/Delete/Approve/etc.) receive hints for the relevant
  subset of {400, 403, 404, 405, 409, 422} only when a high-confidence next action exists.
- **REQ-005**: When no high-confidence hint exists for a status code, leave `WrapErrWithMessage` —
  do not invent guesses.
- **REQ-006**: All snapshot tests (`TestToolSnapshots_*`) must be regenerated and committed at the
  end of each phase that touches tool output.
- **REQ-007**: `cmd/audit_tools` and `cmd/audit_output` must continue to report 0 violations after
  every phase.
- **SEC-001**: Hints must NEVER expose internal implementation details, tokens, request bodies, or
  user PII. They reference tool names and required parameters only.
- **CON-001**: One file = one logical commit. No cross-domain commits. Commit messages follow
  conventional commits format `feat(mcp): hint rollout in {domain} (LLM ergonomics)`.
- **CON-002**: No bulk regex sweeps that ignore handler context. Every change is read-and-edit.
- **CON-003**: `go vet ./...` and `go test ./internal/tools/{domain}/ -count=1` must pass after
  every commit; do not stack failing commits.
- **GUD-001**: Reuse existing patterns from the seed commit `97cea0eb` (issues/MR/projects).
- **GUD-002**: Prefer `IsHTTPStatus` checks over `ContainsAny` substring matching unless the GitLab
  message has a distinctive marker the status code can't disambiguate.
- **GUD-003**: When 404 means "either resource missing OR Premium-tier feature", say so in the
  hint (e.g. `"MR not found or approval features require GitLab Premium"`).
- **PAT-001**: Reuse common phrasings as inline string literals. We do NOT create global constants
  for hints (avoid distant action-at-a-distance edits).
- **PAT-002**: For two-step or stateful flows (delayed deletion, restore), point to the inspection
  tool that lets the LLM see the current state (e.g., `gitlab_project_get`'s
  `marked_for_deletion_on`).

## 2. Implementation Steps

### Implementation Phase 1 — Foundation (no rollout yet)

- GOAL-001: Verify the helper surface is sufficient for all forthcoming hints; add only the helpers
  that are mathematically reused enough to justify a global symbol.

| Task | Description | Completed | Date |
|------|-------------|-----------|------|
| TASK-001 | Audit `internal/toolutil/errors.go` to confirm `WrapErrWithHint`, `WrapErrWithMessage`, `IsHTTPStatus`, `ContainsAny`, `NotFoundResult` cover every pattern needed in Phases 2-6. Document any gap as a follow-up TASK before Phase 2 begins. | ✅ | 2026-04-26 |
| TASK-002 | Decide whether to add a tiny `WrapErrWithStatusHint(op, err, code, hint)` convenience that compresses the 5-line `if IsHTTPStatus { return WrapErrWithHint } else { return WrapErrWithMessage }` pattern. Implement only if at least 50 future call sites would benefit (count first via grep against the proposed code list). | ✅ | 2026-04-26 |
| TASK-003 | If TASK-002 chooses to add the helper, add unit tests in `internal/toolutil/errors_test.go` and document it in `docs/error-handling.md`. | ✅ | 2026-04-26 |

**Phase 1 acceptance:** helpers ready, no production code changed unless TASK-002 is approved.

### Implementation Phase 2 — Tier 1: Core domain mutators (≥20 call sites)

- GOAL-002: Rich hints across the 9 highest-traffic domain files. Each task = one commit.

| Task | Description | Completed | Date |
|------|-------------|-----------|------|
| TASK-004 | `internal/tools/awardemoji/awardemoji.go` (48 sites). Award emoji is attached to issues/MRs/snippets/notes; 404 means parent resource missing. Add 404 hints pointing at parent's get tool, 403 hints (read-only awards). | ✅ | 2026-04-26 |
| TASK-005 | `internal/tools/projects/projects.go` (46 sites). Cover Get/List remaining handlers + Delete (3 sites — delayed deletion + permanently_remove flow), Restore (pending_delete state check), Archive/Unarchive (already-archived hint), Fork (target namespace permissions), Star/Unstar. | ✅ | 2026-04-26 |
| TASK-006 | `internal/tools/runners/runners.go` (38 sites). Runner registration tokens, scope (project/group/instance), ListAll vs ListProject. 404 = runner deleted; 403 = runner is locked/instance-scoped requiring admin. | ✅ | 2026-04-26 |
| TASK-007 | `internal/tools/accesstokens/accesstokens.go` (37 sites). Personal/project/group access tokens. 401 = token already revoked; 403 = token scope insufficient; 422 = invalid scopes/expiration. | ✅ | 2026-04-26 |
| TASK-008 | `internal/tools/groupboards/groupboards.go` (34 sites). Mirror of `boards` for groups. 403 = Premium-tier feature. | ✅ | 2026-04-26 |
| TASK-009 | `internal/tools/mergerequests/mergerequests.go` (32 sites). Cover the remaining handlers not touched in `97cea0eb`: Commits, Pipelines, Participants, Rebase (already partial), CherryPickMR/RevertMR if present, Discussions list, Time-tracking handlers. | ✅ | 2026-04-26 |
| TASK-010 | `internal/tools/boards/boards.go` (32 sites). Issue boards (project-level). 404 = board missing or Premium tier; 422 = invalid label/milestone scope. | ✅ | 2026-04-26 |
| TASK-011 | `internal/tools/groupmembers/groupmembers.go` (23 sites). Group member CRUD + share. 404 = group/user missing; 403 = inherited membership cannot be deleted; 409 = already a member. | ✅ | 2026-04-26 |
| TASK-012 | `internal/tools/issues/issues.go` (21 sites). Cover the remaining handlers not touched in `97cea0eb`: ListGroup, ListAll, GetByID, Reorder, Move, Participants, related-merge-requests, closed-by-MRs, ListAwardEmojis (if not in awardemoji). | ✅ | 2026-04-26 |

**Phase 2 acceptance:** 311 sites reviewed. `go test ./internal/tools/{awardemoji,projects,runners,accesstokens,groupboards,mergerequests,boards,groupmembers,issues}/ -count=1` passes.

### Implementation Phase 3 — Tier 2: Mid-volume domains (10-19 call sites)

- GOAL-003: Apply the same per-handler discipline to 23 mid-volume files.

| Task | Description | Completed | Date |
|------|-------------|-----------|------|
| TASK-013 | `internal/tools/pipelinetriggers/pipelinetriggers.go` (19) | ✅ | 2026-04-26 |
| TASK-014 | `internal/tools/jobs/jobs.go` (19) — Cancel/Retry/Play/Erase + 404 vs job-not-cancellable hint. | ✅ | 2026-04-26 |
| TASK-015 | `internal/tools/pipelineschedules/pipelineschedules.go` (18) | ✅ | 2026-04-26 |
| TASK-016 | `internal/tools/featureflags/featureflags.go` (16) | ✅ | 2026-04-26 |
| TASK-017 | `internal/tools/resourceevents/resourceevents.go` (15) | ✅ | 2026-04-26 |
| TASK-018 | `internal/tools/projectmirrors/project_mirrors.go` (14) | ✅ | 2026-04-26 |
| TASK-019 | `internal/tools/ffuserlists/ffuserlists.go` (14) | ✅ | 2026-04-26 |
| TASK-020 | `internal/tools/pipelines/pipelines.go` (13) — Cancel/Retry/Delete + already-finished hint. | ✅ | 2026-04-26 |
| TASK-021 | `internal/tools/groups/groups.go` (13) — Transfer/Restore/Search subgroups. | ✅ | 2026-04-26 |
| TASK-022 | `internal/tools/commits/commits.go` (13) — CherryPick/Revert/CreateCommit + diverged-branch hint. | ✅ | 2026-04-26 |
| TASK-023 | `internal/tools/badges/badges.go` (12) | ✅ | 2026-04-26 |
| TASK-024 | `internal/tools/deployments/deployments.go` (11) | ✅ | 2026-04-26 |
| TASK-025 | `internal/tools/runnercontrollertokens/runnercontrollertokens.go` (10) | ✅ | 2026-04-26 |
| TASK-026 | `internal/tools/runnercontrollers/runnercontrollers.go` (10) | ✅ | 2026-04-26 |
| TASK-027 | `internal/tools/runnercontrollerscopes/runnercontrollerscopes.go` (10) | ✅ | 2026-04-26 |
| TASK-028 | `internal/tools/protectedenvs/protectedenvs.go` (10) | ✅ | 2026-04-26 |
| TASK-029 | `internal/tools/notifications/notifications.go` (10) | ✅ | 2026-04-26 |
| TASK-030 | `internal/tools/invites/invites.go` (10) — invite already accepted/expired hints. | ✅ | 2026-04-26 |
| TASK-031 | `internal/tools/instancevariables/instancevariables.go` (10) | ✅ | 2026-04-26 |
| TASK-032 | `internal/tools/groupvariables/groupvariables.go` (10) | ✅ | 2026-04-26 |
| TASK-033 | `internal/tools/freezeperiods/freezeperiods.go` (10) | ✅ | 2026-04-26 |
| TASK-034 | `internal/tools/civariables/civariables.go` (10) | ✅ | 2026-04-26 |
| TASK-035 | `internal/tools/branches/branches.go` (10) — protected-branch hint already exists for delete; expand to Create/CherryPickHead. | ✅ | 2026-04-26 |

**Phase 3 acceptance:** 287 sites reviewed. Cumulative ~598 sites covered.

### Implementation Phase 4 — Tier 3: 5-9 call site files (70 files)

- GOAL-004: Steady-state rollout. Group commits 5 files at a time within thematic clusters to keep
  cognitive load reasonable while still keeping the per-handler review discipline.

Sub-phases (one cluster = one commit, ~5 files each):

| Task | Description | Completed | Date |
|------|-------------|-----------|------|
| TASK-036 | Cluster 4A — Notes & discussions: `issuenotes`, `mrnotes`, `issuediscussions`, `mrdiscussions`, `commitdiscussions` | ✅ | 2026-04-26 |
| TASK-037 | Cluster 4B — Repo content: `repository`, `tags`, `files`, `repositoryfiles` (if exists), `wikis` | ✅ | 2026-04-26 |
| TASK-038 | Cluster 4C — Group features: `groupmilestones`, `grouplabels`, `groupboards` (if not in T1), `groupwikis`, `grouphooks` | ✅ | 2026-04-26 |
| TASK-039 | Cluster 4D — Issue tracking: `labels`, `milestones`, `issuelinks`, `epics`, `epicissues` | ✅ | 2026-04-26 |
| TASK-040 | Cluster 4E — Snippets & uploads: `snippets`, `snippetdiscussions`, `uploads`, `groupmarkdownuploads` | ✅ | 2026-04-26 |
| TASK-041 | Cluster 4F — Releases: `releases`, `releaselinks`, `tags` (if not in 4B) | ✅ | 2026-04-26 |
| TASK-042 | Cluster 4G — Packages & registries: `packages`, `containerregistry`, `dependencyproxy`, `protectedpackages` | ✅ | 2026-04-26 |
| TASK-043 | Cluster 4H — User-scoped: `users`, `user_admin`, `usergpgkeys`, `usersshkeys`, `useremails`, `userimpersonationtokens`, `userstatus` | ✅ | 2026-04-26 |
| TASK-044 | Cluster 4I — CI surface: `cilint`, `cicatalog`, `environments`, `joblogs`, `jobartifacts`, `jobtokenscope` | ✅ | 2026-04-26 |
| TASK-045 | Cluster 4J — Deployments & infra: `deploykeys`, `deploytokens`, `pages`, `terraformstates`, `clusteragents` | ✅ | 2026-04-26 |
| TASK-046 | Cluster 4K — Settings & admin: `settings`, `applicationsettings`, `geo`, `keys`, `members`, `accessrequests` | ✅ | 2026-04-26 |
| TASK-047 | Cluster 4L — Remaining 5-9 site files (sweep, ~20 files left) | ✅ | 2026-04-26 |

**Phase 4 acceptance:** 470 sites reviewed. Cumulative ~1068 sites covered.

### Implementation Phase 5 — Tier 4: Long tail (1-4 call sites, 69 files)

- GOAL-005: Close out the remaining 168 sites. Group ~10 files per commit since each file is small.

| Task | Description | Completed | Date |
|------|-------------|-----------|------|
| TASK-048 | Long-tail commit 5A (10 files) | | |
| TASK-049 | Long-tail commit 5B (10 files) | | |
| TASK-050 | Long-tail commit 5C (10 files) | | |
| TASK-051 | Long-tail commit 5D (10 files) | | |
| TASK-052 | Long-tail commit 5E (10 files) | | |
| TASK-053 | Long-tail commit 5F (10 files) | | |
| TASK-054 | Long-tail commit 5G (~9 files, final) | | |

**Phase 5 acceptance:** 168 sites reviewed. **All 1236 original sites accounted for** (either with hint or deliberately left as `WrapErrWithMessage`).

### Implementation Phase 6 — Validation & snapshot regeneration

- GOAL-006: Lock in correctness and refresh tooling-derived artifacts.

| Task | Description | Completed | Date |
|------|-------------|-----------|------|
| TASK-055 | Run `make analyze-fix` over the entire repo. Verify zero diffs after format. | | |
| TASK-056 | Run `go vet ./...` — must pass with zero output. | | |
| TASK-057 | Run `go test ./internal/... ./cmd/... -count=1` — only `TestToolSnapshots_*` may fail. | | |
| TASK-058 | Run `UPDATE_TOOLSNAPS=true go test ./internal/tools/ -run TestToolSnapshots -count=1` to regenerate snapshots. | | |
| TASK-059 | Run `go test ./internal/tools/ -run TestToolSnapshots -count=1` — must pass. | | |
| TASK-060 | Run `go run ./cmd/audit_tools/` and `go run ./cmd/audit_output/` — must report 0 findings each. | | |
| TASK-061 | Run `go run ./cmd/gen_llms/` to regenerate `llms.txt` and `llms-full.txt`. Commit changes. | | |
| TASK-062 | Run E2E suite (Docker mode) to confirm no behavior regression: `make test-e2e-docker`. | | |
| TASK-063 | Update `docs/error-handling.md` with the rollout summary (counts, coverage, examples). | | |

**Phase 6 acceptance:** snapshots clean, all audits pass, E2E green.

### Implementation Phase 7 — STRETCH: Per-action OutputSchema on ActionRoute (for `gen_llms` only)

- GOAL-007: Capture typed output schemas per route without exposing them in `tools/list`. Consume
  them in `cmd/gen_llms` to enrich `llms-full.txt` with per-action result shapes. Optionally consume
  them in `cmd/audit_output` to assert each route declares a schema.

| Task | Description | Completed | Date |
|------|-------------|-----------|------|
| TASK-064 | Add `OutputSchema map[string]any` field to `toolutil.ActionRoute`. Default nil for void variants. | | |
| TASK-065 | Modify `RouteAction[T,R]`, `DestructiveAction[T,R]`, `RouteActionWithRequest[T,R]`, `DestructiveActionWithRequest[T,R]` to populate `OutputSchema` via `jsonschema.For[R](nil)` and `json.Marshal` to `map[string]any`. Cache by `reflect.Type` to avoid repeated work. | | |
| TASK-066 | Verify `RouteVoidAction` / `DestructiveVoidAction` keep `OutputSchema = nil`. | | |
| TASK-067 | Confirm `addMetaTool` and `addReadOnlyMetaTool` continue to emit only the permissive envelope schema in `tools/list` (the per-route data must NOT be exposed there). Add a regression test that asserts `Tool.OutputSchema` is the envelope, not a discriminated union. | | |
| TASK-068 | Modify `cmd/gen_llms/main.go` to emit a per-action output-shape section per meta-tool in `llms-full.txt`. Format: collapsible Markdown block with the JSON schema for each action. | | |
| TASK-069 | Modify `cmd/audit_output/main.go` to also report routes whose `OutputSchema` is missing (excluding void routes by design). | | |
| TASK-070 | Regenerate `llms-full.txt` and verify size and structure are reasonable (target: <2 MB total). | | |
| TASK-071 | Add unit tests in `internal/toolutil/metatool_test.go` covering schema population for `RouteAction[T,R]` with a representative struct. | | |
| TASK-072 | Document the new field and its consumers in `docs/architecture.md` or a new short ADR. | | |

**Phase 7 acceptance:** `llms-full.txt` enriched, audits report 0, no change to `tools/list`
output, snapshots unchanged.

## 3. Alternatives

- **ALT-001**: Bulk regex sweep replacing every `WrapErrWithMessage` with a status-aware switch.
  Rejected — loses per-handler context and produces wrong/misleading hints in edge cases (e.g.,
  Subscribe-on-already-subscribed, Delete-on-already-marked-for-deletion).
- **ALT-002**: Define a global hint table keyed by `(domain, action, status)`. Rejected — same
  context-loss problem and adds action-at-a-distance editing surface that nobody will keep current.
- **ALT-003**: Centralize all hints in YAML/JSON loaded at boot. Rejected — adds runtime parsing,
  loses Go compile-time checks for typos.
- **ALT-004 (Phase 7)**: Embed per-action schemas as `oneOf` in `tools/list`. Already rejected by
  prior commit `23b89ee4` due to token bloat. Phase 7 deliberately stays out of `tools/list`.

## 4. Dependencies

- **DEP-001**: `internal/toolutil/errors.go` — `WrapErrWithHint`, `IsHTTPStatus`, `ContainsAny`,
  `NotFoundResult`. Already in place; verified in TASK-001.
- **DEP-002**: `cmd/audit_output` and `cmd/audit_tools` — must continue to report 0 findings.
- **DEP-003** (Phase 7): `github.com/google/jsonschema-go` v0.4.2 — `For[T any](opts) (*Schema, error)`.
- **DEP-004**: Docker environment for E2E (`make test-e2e-docker`) at end of Phase 6.

## 5. Files

- **FILE-001**: `internal/toolutil/errors.go` — Phase 1 helper additions if approved.
- **FILE-002**: `internal/tools/{domain}/*.go` × 152 sub-packages — Phase 2-5 hint rollout.
- **FILE-003**: `internal/tools/testdata/tools_individual.json`, `tools_meta.json` — Phase 6 snapshot
  regeneration.
- **FILE-004**: `llms.txt`, `llms-full.txt` — Phase 6 doc regeneration.
- **FILE-005**: `docs/error-handling.md` — Phase 6 documentation update.
- **FILE-006** (Phase 7): `internal/toolutil/metatool.go` — `ActionRoute` field addition + helper
  changes.
- **FILE-007** (Phase 7): `cmd/gen_llms/main.go`, `cmd/audit_output/main.go` — consumers.

## 6. Testing

- **TEST-001**: After each phase, `go test ./internal/tools/{domain}/ -count=1` for every touched
  domain — must pass.
- **TEST-002**: After each phase, `go vet ./...` and `go build ./...` — must pass.
- **TEST-003**: After Phases 2-5 complete, `go test ./internal/... -count=1` — only snapshot tests
  may fail.
- **TEST-004**: After Phase 6, `UPDATE_TOOLSNAPS=true go test` regenerates snapshots and
  re-running without the env var passes.
- **TEST-005**: After Phase 6, `make test-e2e-docker` — full E2E suite green.
- **TEST-006**: `go run ./cmd/audit_tools/` and `go run ./cmd/audit_output/` — 0 findings at every
  phase boundary.
- **TEST-007** (Phase 7): New unit test in `internal/toolutil/metatool_test.go` asserting
  `OutputSchema` is populated for typed routes and nil for void routes.

## 7. Risks & Assumptions

- **RISK-001**: A hint that's "obviously correct" for a common case may mislead in an edge case
  (e.g., 404 on `gitlab_project_get` could mean private project, not missing). Mitigation: phrase
  hints in inclusive terms ("verify project_id OR check that your token has access").
- **RISK-002**: Snapshot regeneration creates a large diff at the end of Phases 2-5. Mitigation:
  isolate snapshot regen into its own commit (Phase 6, TASK-058).
- **RISK-003**: 152 sub-packages is a long road; partial rollout is itself useful but creates
  inconsistent UX. Mitigation: phase boundaries are themselves shippable; merging after Phase 3 is
  acceptable.
- **RISK-004** (Phase 7): `jsonschema.For[R]` may fail at startup for exotic types. Mitigation:
  log-and-skip with `nil` schema; do not crash.
- **ASSUMPTION-001**: Each domain handler file changes once per phase commit; no merge conflicts
  from parallel work.
- **ASSUMPTION-002**: Existing `WrapErrWithHint` test patterns in `errors_test.go` are sufficient;
  no new test infrastructure required.
- **ASSUMPTION-003** (Phase 7): `cmd/gen_llms`'s output is deterministic — output shape diffs only
  when struct fields change.

## 8. Related Specifications / Further Reading

- [MCP Tools spec 2025-11-25 — Output Schema](https://modelcontextprotocol.io/specification/2025-11-25/server/tools#output-schema)
- [ADR-0007 Rich error semantics](../docs/adr/adr-0007-rich-error-semantics.md)
- [docs/error-handling.md](../docs/error-handling.md)
- Seed commit `97cea0eb feat(mcp): targeted WrapErrWithHint rollout in mutating handlers (LLM ergonomics)`
- Reverted commit `23b89ee4 revert: drop per-action OutputSchema anyOf for meta-tools` — context for
  why Phase 7 stays out of `tools/list`.
- [llms.txt convention](https://llmstxt.org/) — rationale for Phase 7's `llms-full.txt` enrichment.
