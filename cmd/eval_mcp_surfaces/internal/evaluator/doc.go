// Package evaluator runs model evaluations against GitLab MCP tool surfaces.
//
// The package owns provider execution, MCP bridge handlers, fixture preparation,
// validation, reporting, and publication. Static case definitions live in the
// cases subpackage so new evaluator tasks can be added without sorting through
// runtime code.
package evaluator
