// Package golist holds the row shape two commands ask `go list` for, and the
// one answer to which `go` they ask it of.
//
// Two commands enumerate this module's packages from the toolchain rather than
// from the git index: cmd/godoc_tool audits every package's doc comments, so
// it lists `./...` and needs each package's directory to parse and its import
// path to name a finding, and cmd/gen_testing_docs describes the packages the
// testing reference documents, so it lists three patterns with the e2e build
// tags and needs the same three fields for its rows. Both had written the same
// `-f` template, the same tab-separated parse, the same "unexpected go list
// row" refusal and the same struct, in a different field order each, and both
// had a copy of the decision below about which `go` binary to run.
//
// That decision is the reason this is a package rather than two tidy copies.
// Neither command may resolve `go` through PATH ([Executable] joins it out of
// [runtime.GOROOT] instead), because a lookup in a directory list the
// environment controls is what Sonar's go:S4036 refuses, and the same rule is
// why the exec call sites carry a `#nosec G204` note saying the program name
// is fixed. Written twice, that is a rule that holds until one of the two
// copies is edited by somebody who did not read the other; written once, with
// its `//nolint` comment and the Windows suffix beside it, it is a rule.
//
// What stays with each command is everything that makes its listing its own.
// godoc_tool keeps `./...`, its 30-second bound and the seam its tests feed
// malformed rows through; gen_testing_docs keeps its three patterns, the
// e2e build tags that reveal the tagged suites, and the runner that pins
// GOTOOLCHAIN to go.mod and merges stderr into the output so a warning row
// reaches the same parser as a package row. Running the command is
// deliberately not shared: one wants a single listing's stdout under a
// deadline, the other runs `go test` and `go tool cover` through the same
// runner and reports a failure with the tail of its combined output.
//
// cmd/gen_stats is not a member and cannot become one. It discovers packages
// through `git ls-files`, on purpose, so that `make check-stats` is a function
// of what is committed rather than of what is on disk; sharing a listing with
// it would be sharing the wrong universe, not sharing a parse.
package golist
