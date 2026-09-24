// Package goprogram is the one go/packages front end for the gates that
// type-check this repository's own source.
//
// Nine packages load through it. cmd/audit_md_escaping walks the calls that
// interpolate a GitLab-authored value into Markdown, cmd/audit_readonly_graphql
// walks the calls a read-only action can reach, cmd/internal/graphqldocs folds
// every raw GraphQL document to the one string GitLab would receive, and
// cmd/audit_graphql_shapes pairs each of those documents with the struct that
// decodes it. cmd/audit_action_ids folds the action IDs the server publishes
// to a model, cmd/audit_catalog_first resolves the calls that aggregate each
// package's ActionSpecs, cmd/audit_dead_consts holds every unexported constant
// to something that reads it, cmd/audit_sdk_context holds every call into
// client-go to the caller's context, and cmd/audit_e2e_coverage's static check
// reads the end-to-end test packages. They ask different questions of the
// loaded program and index it different ways, which is why only the front end
// lives here: the load mode, the config, and the rule that a package which did
// not type-check stops the run.
//
// That last rule is why this package exists rather than being a copy per
// gate. Every gate here answers "cannot tell" for anything it cannot resolve,
// and a partially typed package resolves nothing: the escaping audit would
// classify every value as unfollowable, the read-only audit would find no
// handlers, and the document collector and the shape audit would fold no
// constants. Each would then report a clean run over source it never
// understood, which is the one failure mode a gate must not have. The first
// four gates wrote the refusal four times with four wordings, so a change to
// it was a four-file edit with one file easy to forget.
//
// What stays with each gate is everything above the load: its own indexers,
// its own detectors, its own question and its own binary.
//
// cmd/audit_e2e_coverage's static check reads test packages that exist only
// behind the e2e build tag, and cmd/audit_dead_consts reads test variants
// too. [LoadWith] takes the [Options] those loads need, test variants and
// build tags, and [Load] is the same call with neither, so the loads that
// read production source alone are unchanged.
//
// [Options.Env] is there for the one load that is not of this repository at
// all: cmd/internal/graphqldocs reads the GraphQL documents client-go builds,
// which means loading a module directory in the module cache, and two
// toolchain settings decide whether that works. The settings and their reasons
// stay with that caller, since they are a property of loading somebody else's
// module rather than of this front end.
//
// [github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/shared.LoadToolPackages]
// is deliberately not folded in. Its mode is now this one less
// NeedCompiledGoFiles, which it has no reader for, but its contract is not
// this one: it refuses more widely than [Load] does, collecting every error
// of every package it loaded and aborting on all of them at once where [Load]
// stops at the first error of a package the caller asked for; it returns a
// subset rather than what it loaded, keeping the packages under internal/tools
// and dropping the rest; and it memoizes that result per root. That is a
// different contract, not a different wording of this one.
//
// It did load with NeedDeps, and the reason it no longer does is the one
// written above: nothing in the audit reads a dependency's syntax, so
// type-checking the tree from source cost about half of every load and
// changed no answer in any of the six reports.
package goprogram
