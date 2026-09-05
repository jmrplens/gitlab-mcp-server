// Package evalrun provides small helpers shared by the live evaluation
// command: deterministic unique suffixes for ephemeral GitLab resources and
// environment-driven run configuration used across e2e fixtures.
//
// The package keeps these helpers in a shared location so the evaluator CLI
// and the live fixture preparers can agree on a single naming convention and
// a single context-aware wait helper.
package evalrun
