// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package conditions runs gobco over one package, over exactly the files the
// go command builds. It is the recipe behind `make coverage-conditions`, the
// condition steps of CI's cross-platform legs and the measurement
// cmd/measure_conditions takes, and it was scripts/coverage-conditions.sh
// until issue 1114, whose recorded runs (testdata/conditions_runs.json) its
// tests replay.
//
// gobco reads a package by handing every file whose name ends in .go to
// go/parser.ParseDir and type-checking all of them together (instrument and
// resolveTypes in instrumenter.go, v1.3.4, the latest release; its default
// branch at 7a09995 does the same). Neither step consults a build constraint,
// a //go:build line and a _windows.go name alike, so a package that declares
// a function twice, once per platform, stops with a redeclaration panic
// before anything is measured:
//
//	panic: probe_linux.go:8:6: peerStdinIsNull redeclared in this block
//
// Three packages here are in that shape: cmd/server (peerStdinIsNull,
// bindUnixSocket and isConnRefused, each declared once for Linux or Unix and
// once for the rest), internal/toolutil (syncDirectory, openLeafNoFollow and
// createNewLeafNoFollow, each in a unix and an other file) and
// test/e2e/internal/harness (serverBuildArgs, in server_race.go and
// server_norace.go). None had had a condition measured (issue 1017).
//
// So a package whose directory holds a .go file the go command does not build
// is staged: the module is copied into a temporary directory, every such file
// is removed from the package's copy, and gobco is run on the package there.
// gobco then parses one build context, the one its own `go test` compiles, and
// every figure it prints is about the files that build on this platform under
// the tags given. The files left out are named, because this run does not
// measure their conditions: only a run on a platform that builds them can,
// which is what the Windows and macOS legs of CI's cross-platform job run this
// for, gated as GOBCO_GATE below describes.
//
// The copy is of the module rather than of the package, because anything else
// changes what the tests see. gobco bridges an external test package to the
// instrumented one through the import path those tests already name, so a
// copy of the package under another path hands them the original,
// uninstrumented package and the build fails (internal/toolutil: undefined:
// toolutil.GobcoCover). Tests also read files by paths relative to their own
// directory (docs/, the root's server.json, their testdata) and some check
// the directory's own name (the harness's TestPackageName_...). In a copy of
// the module every import path, relative path and directory name is the
// original's. It leaves out .git, .claude/worktrees (complete worktrees of
// this repository), node_modules and dist, which no package's test reads and
// which gobco would otherwise copy again into its own work directory, and it
// lives outside the tree, so a run killed before it cleans up leaves a copy
// in the temporary directory rather than a second copy of every package in
// the tree. A symbolic link is copied as a link, so a file of the package
// that is one is replaced in the copy rather than written through, and a
// package reached through one is refused: a run that measures edits nothing
// of what it measures.
//
// The files to remove are the complement of what go list says it builds
// (GoFiles, CgoFiles, TestGoFiles, XTestGoFiles), not its IgnoredGoFiles:
// ParseDir also reads a file whose name starts with a dot or an underscore,
// which the go command never builds and IgnoredGoFiles never lists. go list
// is asked under the same tags the gobco run passes to its `go test`, since
// without them every file of a package behind a tag reads as left out.
//
// gobco decides which parsed files to instrument with a go/build.Context
// holding only GOOS and GOARCH (shouldBuild). That context reads a constraint
// naming nothing but operating systems, architectures and `unix` exactly as
// the go command does, and rejects one naming anything else it is not given:
// a tag, a release (go1.22), cgo, the compiler. A file the go command builds
// under such a constraint would be compiled and its conditions left out of
// the report, which then reads as complete. So the copy blanks the constraint
// lines of those files, and only of those, line for line, so every position
// gobco reports is the original's and the file is instrumented like the rest.
// A package holding such a file is staged for that reason alone.
//
// An external test package (package x_test) is the third shape gobco cannot
// read, in two ways. gobco type-checks it through the source importer of
// go/importer, which builds the package under test from its non-test files
// alone, so a name the package's own test files declare for it, which is what
// an export_test.go is for, is undefined there, and gobco panics before
// anything is measured:
//
//	panic: action_specs_test.go:52:27: undefined: todos.PublishedActionIDs
//
// That was 27 packages here when the whole tree was first measured
// (rillig/gobco#42, with the fix in review as rillig/gobco#43). The copy makes
// each of the package's own test files that declares no test, benchmark, fuzz
// test or example a file of the package, renamed without its _test suffix and
// opened with //go:build gc: the go command satisfies that constraint, so the
// file is compiled into the test binary as before and its names reach the
// external test package, and gobco's narrower build context does not, so it
// leaves the file uninstrumented, as it leaves a test file. And gobco bridges
// the external test package to the instrumented one through an import of the
// package it finds in the directory, so an external test package that imports
// nothing of its package, as a test that reaches it through the catalog does,
// gets a bridge with an empty import path, which does not build (three
// packages, rillig/gobco#33). The copy adds a test file to that package which
// imports the package and reads one name a file of the package declares
// behind the same constraint. Neither change moves a position gobco reports.
//
// This staging of an external test package is TEMPORARY. The maintainer
// decided on 2026-10-27 to keep it, superseding the decision of 2026-10-06
// (issue 1099) to leave both limits to gobco, until gobco publishes a release
// with the fixes of https://github.com/rillig/gobco/pull/43 (for
// https://github.com/rillig/gobco/issues/42) and
// https://github.com/rillig/gobco/pull/39 (for
// https://github.com/rillig/gobco/issues/33); it goes then.
//
// A package with none of these, a constraint on the platform alone included
// (a _unix_test.go behind //go:build !windows with no Windows counterpart), is
// run where it is, exactly as the recipe did before, so the figures of every
// package it could already measure do not move.
//
// Blanking has a cost the run cannot see: a test that reads its own files'
// headers finds them blank. The e2e harness checks that every one of its
// sources carries exactly //go:build e2e, so under TAGS=e2e its run fails
// there, and a failed staged run names the blanked files as the likely cause.
// That package waits for gobco to honor build tags (issue 1017).
//
// A report that measured no condition at all, `Condition coverage: 0/0`, is
// refused rather than passed on. It is what gobco prints for a package whose
// test binary never wrote its counts (no test ran) and for one whose every
// file it declined to instrument, and both read like a package with nothing
// left to test.
//
// The report is a report, and passes whatever it found, unless GOBCO_GATE
// makes it a gate. A condition not evaluated both ways is one gobco names as
// "never true", "never false" or "never evaluated", and the gate fails on it:
//
//	GOBCO_GATE=all                every such condition fails the run, which is
//	                              the gate a touched package is held to before
//	                              it is committed
//	GOBCO_GATE=beyond:GOOS/GOARCH only those in a file this build compiles and
//	                              that platform does not; every other one is
//	                              printed, counted and passed, since a run on
//	                              that platform measures its file
//
// The first is also what the Windows and macOS legs of CI run on cmd/server
// and internal/toolutil (no CI step runs gobco on Linux, where a touched
// package passes it before it is committed): a condition either package
// leaves one-way on that platform fails the leg, in a file only that platform
// builds or in one Linux builds too. Until issue 1212 the legs ran the
// second, with linux/amd64, which held only the files they alone build,
// because tests those platforms skipped left a few conditions of shared files
// one-way there while the Linux run measured them both ways; each of those is
// now driven on every platform. The second remains for a measurement that
// wants only a platform's own files. Any other value is refused before gobco
// runs.
//
// A run the gate fails reports 3 ([GateHeld]), and no other path does. Every
// other failure means nothing was measured and reports 1 or 2, never 3: 2 for
// a gate or a mode (GOBCO_MODE, below) it cannot read; 1 for a package go
// cannot load, a 0/0 report, a report without the figure of its mode, and a
// gobco or test failure, since `go run` reports any non-zero exit of the
// program it ran as 1 and the recipe reports one of gobco run from PATH
// (below) the same way. An interrupt ends a run with 130 or 143. Every caller
// that only asks whether the run passed reads any non-zero status as a
// failure; cmd/measure_conditions, which calls [Measure] in its own process,
// reads 3 as a figure taken and the rest as no figure at all, which is the
// one difference it needs. Through `go run`, as the Makefile and CI run
// cmd/coverage_recipes, every failure reads as 1.
//
// GOBCO_TEST_FLAGS carries go test flags for gobco's go test, separated by
// spaces and each written in one word (-timeout=30m), since gobco runs it
// with go test's own ten-minute timeout and cmd/server takes about three
// times as long on Windows as on Linux.
//
// GOBCO_MODE chooses what gobco counts (markConds in instrumenter.go):
//
//	condition (the default)  gobco's own default: every atomic condition,
//	                         which is each operand of &&, || and ! taken
//	                         apart, and each comparison wherever it is
//	                         written, a return statement or an assignment as
//	                         much as an if; the report opens its figure with
//	                         "Condition coverage"
//	branch                   gobco's -branch: the whole controlling condition
//	                         of each if and each for, and each case of a
//	                         switch, and nothing else; the report opens its
//	                         figure with "Branch coverage"
//
// A case of a switch with a tag is counted alike in both, as the tag compared
// with the case. Every condition a report names, and the gate above, read the
// same in either mode, since gobco names a branch taken one way in the words
// it uses for a condition. A report whose figure is in the other mode's words
// is refused like a 0/0, and any other value of the mode is refused before
// gobco runs.
//
// gobco is run from PATH when the gobco PATH finds reports the release pinned
// here ([Release], by gobco -version), and through `go run` of that release
// otherwise. go run asks the module proxy for the module's versions on every
// run, to report a deprecation, even with the module in the cache, so a run
// through it needs the network at that moment: a macOS runner that lost name
// resolution failed CI's condition step twice in two days without measuring
// anything (issue 1515). gobco run from PATH needs no network. A gobco of
// another release is not run, since every figure is taken with the pinned
// one.
//
// [Install] puts it there, and measures nothing: go install of the pinned
// release, in at most three attempts with GOBCO_INSTALL_PAUSE seconds between
// them (10 unless set; anything but a whole number, an empty value included,
// is refused before anything runs), skipped when PATH already finds the
// pinned release. The gobco PATH finds is then held to that release, so an
// install that left it where PATH does not look, or behind another release,
// fails there rather than leaving every measurement after it to go run. It
// reports 0 once PATH finds the pinned release, 1 when it does not, and 2 for
// a pause it cannot read. Every workflow job that measures runs it once
// first, so no measurement of the job asks the module proxy for gobco; the
// package's own dependencies still come from the module cache.
package conditions
