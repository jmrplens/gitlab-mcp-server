# LLM Ergonomics Audit — `audit/mcp-spec-compliance`

> Audit + implement everything actionable until it hurts.

## Scope

Eight axes of "how well does this server work for an LLM client":

1. Tool descriptions (clarity, imperative voice, completeness)
2. Error messages (actionable hints, classification)
3. Response size / token economy
4. Next-step hints (LLM continuation guidance)
5. Argument naming and required-flag discipline
6. Markdown ergonomics (tables, links, hint blocks)
7. Discoverability (meta-tools, enums in schemas)
8. Pagination UX (constraints + has_more flags)

## Findings & actions

| # | Axis | Finding | Action | Status |
| - | ---- | ------- | ------ | ------ |
| E-1 | Discoverability | `MetaToolInput.Action` already had a JSON Schema `enum` from `MetaToolSchema(routes)` — so LLMs *do* see valid actions in `tools/list`. No regression. Description was generic. | Tightened description: "Pick exactly one of the values in `enum`. Each action expects its own `params` object — see the tool description for the per-action parameter list." | ✅ Done |
| E-2 | Discoverability | `params` description on meta-tools was vague ("See the tool description for required/optional fields per action"). | Replaced with: "Action-specific parameters as a JSON object. Required and optional fields differ per action; consult this tool's description for the chosen action. Send only the fields documented for that action — unrelated keys are ignored by the underlying handler." | ✅ Done |
| E-3 | Pagination UX | `PaginationInput.Page` / `PerPage` carried only prose ("default 20, max 100") — no JSON Schema constraints. LLMs that try `per_page=500` only learn after a 422. | New middleware `EnrichPaginationConstraints` (sibling of F-4 `LockdownInputSchemas`): walks every `tools/list` response and injects `minimum:1` on `page` and `minimum:1, maximum:100` on `per_page`. Preserves existing constraints. Skips non-numeric properties. Three unit tests covering: bounds applied, existing values preserved, non-numeric `page` ignored. | ✅ Done |
| E-4 | Pagination UX | `PaginationOutput` carried `next_page` / `prev_page` but no boolean. LLMs had to compute "is there more?". | Added `HasMore bool` field (`json:"has_more"`) populated from `NextPage > 0` in both `PaginationFromResponse` and `AdjustPagination`. Existing tests extended with `wantHasMore` column on `TestAdjustPagination` (7 cases) plus assertions on the three `TestPaginationFromResponse_*` tests. | ✅ Done |
| E-5 | Pagination UX | `PaginationInput` jsonschema descriptions were terse. | Rewrote to imperative form with explicit constraint hints and a forward-pagination tip referencing `next_page`. | ✅ Done |
| E-6 | Errors | `WrapErrWithMessage` (1241 call sites) carries a generic semantic classification + GitLab message. Only 4.5% of mutating handlers use the more specific `WrapErrWithHint`. | **Deferred** — central error machinery already classifies HTTP 401/403/404/409/422/429 with diagnostic prose via `ClassifyHTTPStatus`. A 1241-handler sweep would be high churn for marginal LLM benefit; the existing classification carries the bulk of the value. Targeted `WrapErrWithHint` rollout is the right tool, scoped to a future, smaller PR. | 🟡 Deferred |
| E-7 | Tool descriptions | Earlier audit signal of "96% weak openers" was a false positive: pattern matched mid-description text. Sample inspection shows real descriptions are imperative-led ("List…", "Get…", "Protect…"). | No change. Descriptions are LLM-friendly already; mass rewrite carries regression risk for zero ergonomic gain. | 🟢 No-op (validated) |
| E-8 | Output schemas | 42 of 47 meta-tools still lack a typed `OutputSchema` (audit_output emits 42 findings). | **Deferred** to F-1 follow-up. The meta-tool dispatcher serializes the underlying typed result through `markdownForResult`; adding a tagged-union `OutputSchema` per meta-tool requires per-action variants. Out of scope for this ergonomics pass. | 🟡 Deferred (F-1) |
| E-9 | Markdown / hints | `HintPreserveLinks` already drives the project-wide convention. Most list/detail formatters carry it. | No central change required. | 🟢 No-op |
| E-10 | Token economy | Meta-tool registration saves ~9.5× tokens over individual tools (already documented). | No change required. | 🟢 No-op |
| E-11 | Argument naming | Required-int64/string helpers already produce LLM-friendly errors guiding to exact parameter names ("Ensure you use the exact parameter name '%s' as documented…"). | No change required. | 🟢 No-op |

## Files touched

- `internal/toolutil/pagination.go` — `HasMore` field + improved `PaginationInput` schema descriptions; constraint comment on `PaginationInput`.
- `internal/toolutil/pagination_test.go` — `HasMore` assertions across `TestPaginationFromResponse_*` and `TestAdjustPagination`.
- `internal/toolutil/schema_pagination.go` — **new** `EnrichPaginationConstraints` receiving middleware (~100 LoC).
- `internal/toolutil/schema_pagination_test.go` — **new** three table-driven tests covering bounds injection, preservation of explicit values, non-numeric skip.
- `internal/toolutil/metatool.go` — tighter `action` / `params` descriptions in `MetaToolSchema`.
- `cmd/server/main.go` — `EnrichPaginationConstraints(server)` registered immediately after `LockdownInputSchemas(server)`.

## Validation

- `go vet ./internal/toolutil/ ./cmd/server/` — clean.
- `go test ./internal/toolutil/ -count=1` — 899 tests green (3 new in `schema_pagination_test.go`, additions in `pagination_test.go`).
- `go build ./...` — success.
- `go run ./cmd/audit_tools/` — 0 violations.
- `go run ./cmd/audit_output/` — 42 findings, unchanged from baseline (F-1 carryover, see E-8).

## Deferred / out-of-scope

- **F-1**: 42 meta-tools without typed `OutputSchema` — needs per-action discriminated unions.
- **Glama TDQS**: separate audit (next session).
- **Targeted hint rollout**: replace `WrapErrWithMessage` with `WrapErrWithHint` in the top mutating handlers (issues/MRs/projects) when a high-confidence corrective action is known. Best done as a focused, smaller PR.

## How LLMs benefit (concrete)

- Meta-tool calls now expose a complete, schema-level `enum` of valid actions and a stricter description, so first-try action selection succeeds without prose-mining.
- A pagination call with `per_page=200` now fails fast at validation (with the bound shown in `tools/list`) instead of producing a 422 round trip.
- `has_more` removes the "do I paginate?" ambiguity in list outputs — the LLM reads one boolean instead of comparing `next_page > 0`.

## Branch / commit

Branch: `audit/mcp-spec-compliance` (not pushed — awaiting user confirmation).
Commit: `feat(mcp): pagination has_more + schema bounds + meta-tool description polish (LLM ergonomics)`.
