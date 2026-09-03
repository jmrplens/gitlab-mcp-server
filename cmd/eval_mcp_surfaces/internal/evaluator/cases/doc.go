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
// # Case identifiers
//
// A case ID is unique across the whole catalog, not per partition or per case
// set. Partitions are not namespaces: [All] concatenates them into one slice,
// and everything downstream keys on the bare ID. The --task flag selects by
// ID, [github.com/jmrplens/gitlab-mcp-server/v2/cmd/eval_mcp_surfaces/internal/evaluator.CaseByID]
// resolves by ID, and report rows are labeled by ID. A reused ID therefore
// does not merely look untidy: --task runs both cases, the lookup can only
// ever return the first, and a report cannot tell the two results apart.
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
