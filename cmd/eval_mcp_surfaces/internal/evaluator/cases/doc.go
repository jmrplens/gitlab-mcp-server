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
package cases
