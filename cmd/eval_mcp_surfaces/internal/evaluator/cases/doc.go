// Package cases contains the typed model-evaluation case catalog.
//
// Files in this package are intentionally limited to case definitions: prompts,
// expected tool steps, preset membership, and fixture references by name. The
// evaluator package resolves those fixture names to runtime builders and runs
// the cases against mock or Docker-backed GitLab surfaces.
//
// Add new cases by extending the partition-specific files (read.go,
// mutating.go, destructive.go, enterprise_*.go, etc.) and aggregating them
// through the helper they expose (for example [mutatingEvalCases]). The
// registry helper [All] merges every partition into a single slice that
// callers clone before mutating.
//
// # Writing a case
//
// Write a case's text, and its live template, as the person who wants the
// outcome would: say what to do to which thing, and never name the action, tool
// or parameter the scorer checks for, because make check-eval-prompts reads the
// text a run actually sends and refuses a case that carries its own answer
// unless prompt_declarations.go records why that literal is the request itself.
//
// Two things about that rule are easy to get wrong. The text a run sends is the
// PromptTemplate where a case has one, not the plain Prompt, so a template is
// held to the same standard and is the half a live run is judged on. And a
// guardrail is allowed: a sentence that keeps a model from wandering is fine as
// long as it constrains the task without naming the answer, which is the
// difference between "the path is exact and the file exists, so delete it as
// given" and "call repository.file_delete directly".
//
// # Case identifiers
//
// A case ID is unique across the whole catalog, not per partition or per case
// set. Partitions are not namespaces: [All] concatenates them into one slice,
// and everything downstream keys on the bare ID. The --task flag selects by
// ID, [github.com/jmrplens/gitlab-mcp-server/v3/cmd/eval_mcp_surfaces/internal/evaluator.CaseByID]
// resolves by ID, and report rows are labeled by ID. A reused ID therefore
// does not merely look untidy: --task runs both cases, the lookup can only
// ever return the first, and a report cannot tell the two results apart.
//
// The prefix sorts the catalog and decides nothing. MT is one operation, MS is
// a workflow, MF is a fault; MS-ENT-DYN-* and MS-ENV-DEP-* are two older
// families that kept their names. Nothing reads a prefix to decide how a case
// behaves: the edition, the presets, the partition and the destructive flag are
// all fields on [Case], and the two predicates that used to infer an edition
// and a partition from the ID were replaced by constructors that state them.
// That is what makes the allocation rule below safe to follow, since an
// identifier that decided where a case ran would be one nobody could renumber.
//
// Allocate a new ID above the highest number already present, and never reuse
// a retired one. Retired numbers stay retired because raw run reports outlive
// the catalog: reusing MT-039 or MT-093..MT-098 would make an old report and a
// new one disagree about what that identifier means.
//
// This rule was written after thirteen collisions were found in the shipped
// catalog (https://github.com/jmrplens/gitlab-mcp-server/issues/361). Read
// cases added on 2026-05-29 were numbered MT-110..MT-122, a range the
// destructive and Enterprise partitions had already occupied since 2026-05-06,
// while the catalog's high-water mark was MT-179. The read cases were renumbered
// to MT-199..MT-211; the older identifiers kept their meaning.
package cases
