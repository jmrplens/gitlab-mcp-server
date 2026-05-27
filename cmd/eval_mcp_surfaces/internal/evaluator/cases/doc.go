// Package cases contains the typed model-evaluation case catalog.
//
// Files in this package are intentionally limited to case definitions: prompts,
// expected tool steps, preset membership, and fixture references by name. The
// evaluator package resolves those fixture names to runtime builders and runs the
// cases against mock or Docker-backed GitLab surfaces.
package cases
