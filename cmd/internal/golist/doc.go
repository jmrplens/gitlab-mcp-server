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
// the running toolchain's GOROOT instead), because a lookup in a directory
// list the environment controls is what Sonar's go:S4036 refuses, and the
// same rule is why the exec call sites carry a `#nosec G204` note saying the
// program name is fixed. Written twice, that is a rule that holds until one
// of the two copies is edited by somebody who did not read the other; written
// once, with the reasoning and the Windows suffix beside it, it is a rule.
// Three more commands ask [Executable] for the same reason:
// cmd/audit_binary_vulns runs `go build` for every release target,
// cmd/gen_third_party_notices runs `go env` to find GOROOT and the module
// cache, and cmd/measure_conditions lists the module under every release
// target to find the packages whose files differ between them. The last two
// list with a format of their own, gen_third_party_notices with `go list
// -deps` and rows naming each package's module and its replacement rather
// than its directory, for the packages a release binary links, and
// measure_conditions with rows naming the files each package builds.
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
package golist
