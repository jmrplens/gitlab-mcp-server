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
// # The evaluated surface
//
// The catalog a model is scored against is assembled by the same functions
// cmd/server assembles its own with: dynamiccatalog.Build for the dynamic
// surface and tools.SharedMetaCatalog for the meta surface, both given a
// config.ServerConfig built from --server-mode. Assembling an equivalent
// catalog here instead would measure a surface the product does not serve:
// the filters and the standalone actions have an order, and the bookkeeping
// they produce is what tells a model that a withheld write exists and is not
// available, rather than that the server cannot do it at all.
//
// The catalog is what this mirrors, and only the catalog. cmd/server runs one
// further pass after registration, over the tools registered outside the
// catalog: in read-only mode it removes every registered tool without a
// read-only hint, and in safe mode it wraps the rest with previews. The meta
// surface registers the gitlab_interactive_* creation flows that way, so in a
// read-only or safe-mode meta evaluation those flows keep their real handlers
// here while the product would have withdrawn or previewed them. Closing that
// means the pass moving out of cmd/server, not a second copy of it here.
//
// # Public API
//
// [Run] is the CLI entry point. [AllEvalCases], [CaseByID], [CasesByPreset],
// and [ValidateEvalCaseRegistry] expose the case catalog to tooling. The
// remaining exported types ([EvalCase], [ExpectedStep], [CaseFixtureSpec],
// [CaseAssertion], [PreparedCase], [FixtureContext]) describe the shape of
// the catalog and the runtime fixture engine.
package evaluator
