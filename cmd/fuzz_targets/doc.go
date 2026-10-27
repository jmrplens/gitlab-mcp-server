// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Command fuzz_targets lists this module's fuzz targets as the matrix that
// .github/workflows/fuzz.yml fans out over.
//
// go test runs a fuzz target's seed corpus as one more test and varies no
// input unless it is given -fuzz, and until the workflow existed nothing here
// gave it: every target was run on its seeds alone. The workflow fuzzes each
// target for a fixed time once a week, and this command is its plan:
//
//	fuzz_targets [-dir <module root>]
//
// It lists every package of the module with the toolchain's own go, reads the
// test files the go command builds for each on this platform, and prints one
// `matrix=<json>` line for $GITHUB_OUTPUT with a cell per target: the package
// as ./ and its directory, the target's name, and the name of the artifact a
// failing input is uploaded under. A target is a top-level function, in a test
// file the go command builds here, whose name is Fuzz or starts with Fuzz
// followed by a character that is not a lower-case letter, which is the rule
// the go command reads a fuzz target by (FuzzMeterFor is one, Fuzzy is not).
// A file behind a build tag the listing is not given, such as the e2e suite's,
// is not built here and is not read, and the workflow could not fuzz a target
// in it either.
//
// The targets are found anew on every run rather than written into the
// workflow, so a target added to any package is fuzzed from the next run on.
//
// It exits 0 when it printed the matrix, 1 when the module could not be
// listed, a test file could not be parsed, two targets would upload under the
// same artifact name, or no target was found at all, since a plan with nothing
// in it would pass the workflow while it fuzzed nothing, and 2 on a usage
// error.
package main
