// Package evaluator runs model evaluations against GitLab MCP tool surfaces.
//
// The package owns provider execution, MCP bridge handlers, fixture preparation,
// validation, reporting, and publication. Static case definitions live in the
// cases subpackage so new evaluator tasks can be added without sorting through
// runtime code.
//
// # Workflow
//
//  1. CLI flags are parsed into [options].
//  2. The case registry is loaded via [AllEvalCases].
//  3. Selected cases run through [modelRunner.evaluatePreparedCase], which
//     orchestrates provider calls, capability bridge calls, and validation.
//  4. Results are aggregated into a Markdown report and optional trace
//     artifacts.
//
// # Public API
//
// [Run] is the CLI entry point. [AllEvalCases], [CaseByID], [CasesByPreset],
// and [ValidateEvalCaseRegistry] expose the case catalog to tooling. The
// remaining exported types ([EvalCase], [ExpectedStep], [CaseFixtureSpec],
// [CaseAssertion], [PreparedCase], [FixtureContext]) describe the shape of
// the catalog and the runtime fixture engine.
package evaluator
