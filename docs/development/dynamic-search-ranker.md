# Dynamic Search Ranker

This document describes the current dynamic action search ranker and records the baseline used before the field-aware ranking improvement work.

## Current Behavior

Dynamic mode builds the executable action registry eagerly when an MCP server instance is created. In stdio mode this happens during server startup. In HTTP mode this happens per token and GitLab URL server-pool entry, after enterprise detection, token-scope filtering, read-only mode, safe mode, and excluded-tool filtering have been applied.

The current registry stores each visible action as an `actionEntry` with:

- Canonical `domain.action` ID.
- Backing meta-tool name.
- Domain and action names.
- Compatibility aliases.
- Curated and schema-derived tags.
- Required and optional params.
- Field-aware `searchDocument` metadata built from ID words, tool name, domain, action, aliases, tags, required params, optional params, schema property names, schema enum values, and compact schema description terms.
- Flat lower-case `SearchText` kept as a compatibility fallback during the ranker migration.
- Tokenized `SearchTokens` for fuzzy fallback.

Search currently scans the visible action entries and scores each normalized query term against typed action metadata, falling back to flat text for compatibility. Exact canonical IDs score highest, followed by aliases, tags, exact domain or action names, partial ID matches, partial domain or action matches, typed field matches, and broader flat text matches. Synonyms and verb alternatives are expanded before scoring.

Callers may pass `explain:true` to `gitlab_search_tools` or `gitlab_find_action` to include deterministic scoring explanations. The default remains compact and omits explanations. Explanation mode reuses the same scoring path as non-explanation mode, so enabling explanations does not change ranking.

Search and describe results also include curated `related_actions` for workflows where ordering matters, such as comparing refs before generating release notes or checking tag/release state before deletion. These relationships are intentionally sparse and live with the action UX metadata next to usage hints.

No-match searches return a small `suggestions` list built from nearby indexed tokens plus common domains. This gives recovery guidance without exposing or dumping the full catalog.

Fuzzy matching is conservative. It runs when lexical search returns no matches or only low-confidence matches, ignores query tokens shorter than three characters, and uses bounded Levenshtein distance with a maximum of two edits. Fuzzy recovery is filtered so weak typo matches do not elevate destructive actions.

`describe` and `execute` accept canonical action IDs and unambiguous aliases. Ambiguous aliases are rejected with repair guidance listing the valid canonical action IDs. Dynamic `execute` normalizes schema-safe parameter aliases before dispatch, logs name-only normalization metadata at debug level, and reports unknown or missing parameters with valid schema fields before calling the route handler. It does not silently drop unsupported security-sensitive fields such as `masked` or `protected`; callers receive validation feedback and must retry with fields accepted by the selected action schema. Destructive actions still require explicit `confirm:true` before execution.

## Ranking Weights

| Signal | Weight |
| --- | ---: |
| Exact canonical action ID | 120 |
| Exact alias | 100 |
| Exact tag | 90 |
| Exact domain or action | 80 |
| Split domain or action word | 65 |
| Partial canonical ID | 55 |
| Partial domain or action | 45 |
| Required param match | 35 |
| Schema enum match | 28 |
| Typed field or raw flat-text match | 25 |
| Optional param match | 22 |
| Synonym or verb alternative flat-text match | 18 |
| Schema description match | 12 |

## Alias Metadata

Dynamic aliases have explicit source metadata:

| Source | Meaning |
| --- | --- |
| `catalog` | Native alias supplied by the canonical action catalog. |
| `compatibility` | Backward-compatible alias maintained by the dynamic registry. |
| `provider_observed` | Alias observed in model output and kept for repair tolerance. |
| `standalone` | Alias associated with standalone dynamic-only actions. |
| `deprecated` | Alias retained only for temporary migration compatibility. |

Aliases also carry a `Searchable` flag. Searchable aliases influence ranking and appear in the field-aware search document. Non-searchable aliases still canonicalize in `describe` and `execute`, but they do not influence search ranking. This is useful for compatibility aliases such as `repository_tree` that are safe to accept as input but too broad for discovery ranking.

Run the alias audit with:

```bash
go run ./cmd/audit_dynamic_aliases/
```

## Schema Signals And Param Repair

The dynamic search document keeps compact schema-derived terms for discovery:

- Required params score above optional params.
- Enum values can help queries that mention concrete states such as `opened` or `closed`.
- Property descriptions contribute low-weight terms so useful wording can help discovery without overpowering canonical action metadata.

Dynamic execution performs two param-normalization passes before dispatch:

- Common schema-safe aliases from `toolutil.NormalizeParamAliasesForSchemaWithExplanation`, such as `search` to `query` or `mr_iid` to `merge_request_iid`.
- Action-scoped aliases from `NormalizeActionScopedParamsWithExplanation`, such as `branch` to `ref` for `repository.file_get`, `status` to `scope` for `job.list`, and role-name conversion for access-level fields.

The explanation data records parameter names, source, and notes only. It never records parameter values. Normal dynamic output does not include this metadata; it is used for debug logging and tests.

## Search Flow Pseudocode

Registry build:

```text
for each visible catalog route:
	create canonical domain.action entry
	collect searchable aliases and unsearchable compatibility aliases
	build searchDocument from IDs, aliases, tags, schema fields, backend metadata
	register entry by canonical ID and unambiguous aliases
build lightweight inverted index over aliases, domain, action, and document tokens
```

Candidate generation and scoring:

```text
normalize query into terms and synonym alternatives
candidate indexes = inverted-index matches or full catalog fallback
for each candidate:
	score exact ID, aliases, tags, domain/action, params, schema enums, schema text
	subtract action-specificity penalties for unmatched action words
	add compound workflow boosts such as release link or pipeline trigger
sort by score, destructive safety, and canonical ID stability
```

Fuzzy, ambiguity, and metrics:

```text
if lexical results are empty or low-confidence:
	run bounded fuzzy matching over tokenized entries
	suppress weak destructive fuzzy matches
if query is an ambiguous alias:
	annotate results and require canonical IDs for execute
record search runtime counters and debug log metadata without raw query text
```

## Observability

Dynamic search emits a structured debug log for each search with query length, result count, whether fuzzy recovery contributed results, whether the top result is low confidence, whether the query matched an ambiguous alias, the number of destructive fuzzy matches suppressed, and the top action ID. The raw query text is not logged.

Process-local runtime counters are available through `SearchRuntimeMetricsSnapshot()` for tests and future diagnostics. They count total searches, zero-result searches, fuzzy fallback searches, ambiguous alias queries, low-confidence searches, and destructive fuzzy suppressions.

Static registry metrics are available through `Registry.Metrics()` and are reported by:

```bash
go run ./cmd/audit_metrics/
```

The metrics include dynamic action count, search index token/posting counts, total aliases, searchable aliases, unsearchable aliases, and ambiguous aliases. The visible dynamic MCP tool count remains unchanged at three tools.

## Regression Corpus

The deterministic search corpus lives in `internal/tools/dynamic/testdata/dynamic_search_queries.json`. It covers exact canonical IDs, aliases, provider-invented aliases, natural language, typo recovery, ambiguous aliases, long multi-intent prompts, destructive prompts, schema-param prompts, and no-match prompts.

Run it after ranker changes with:

```bash
go test ./internal/tools/dynamic/ -run TestDynamicSearchCorpus -count=1
```

## Backend Metadata

The internal search document includes backend-oriented fields: `Backend`, `Capability`, `Resource`, `Operation`, and `Scope`. Current GitLab entries default `Backend` to `gitlab`, infer a coarse capability from the domain, and derive scope from schema fields such as `project_id` or `group_id`.

This does not expose non-GitLab actions. It prepares the ranker for future DADL-like or cross-backend catalogs by giving each action a stable place for provider, resource, operation, and scope metadata. Backend terms such as `github`, `jira`, `pull request`, `merge request`, `pr`, `mr`, `issue`, and `ticket` normalize to the current GitLab catalog vocabulary so searches remain useful while action IDs stay GitLab-only.

## Baseline Checks

The following checks passed before ranker refactoring began:

```bash
go test ./internal/tools/dynamic/ -run 'Test.*Search|Test.*Describe|Test.*Execute|Test.*Fuzzy' -count=1
```

Result:

```text
ok github.com/jmrplens/gitlab-mcp-server/internal/tools/dynamic 1.177s
```

## Baseline Benchmark

The current benchmark command is:

```bash
go test ./internal/tools/dynamic/ -bench BenchmarkSearch_BaselineMetaCatalog -benchmem -run '^$'
```

Baseline on Linux amd64 with Intel Core Ultra 7 255H:

| Query | Time | Bytes | Allocations |
| --- | ---: | ---: | ---: |
| `merge request list open author project` | 16.22 ms/op | 1,192,905 B/op | 765 allocs/op |
| `list open issues` | 2.30 ms/op | 477,497 B/op | 465 allocs/op |
| `pipeline run trigger` | 1.93 ms/op | 61,904 B/op | 149 allocs/op |
| `ci variable secret` | 2.79 ms/op | 117,907 B/op | 204 allocs/op |
| `project delete` | 0.22 ms/op | 76,409 B/op | 109 allocs/op |
| `discover project from remote` | 0.87 ms/op | 3,002 B/op | 28 allocs/op |
| `merje requesy` | 1.89 ms/op | 1,193,185 B/op | 22,238 allocs/op |

These numbers show that the current linear scan is acceptable for the current catalog size but allocates heavily for some lexical and fuzzy paths. Field-aware candidate generation and a lightweight inverted index should preserve accuracy while reducing unnecessary scoring work.

## Known Limitations

- Search explanations are opt-in and intentionally compact; they explain the strongest deterministic matches rather than every internal scoring adjustment.
- Candidate generation uses a lightweight in-memory index with full-scan fallback; it is not persisted and is rebuilt per dynamic registry.
- Param normalization explanations are debug-oriented and are not returned in normal `gitlab_execute_tool` responses.
