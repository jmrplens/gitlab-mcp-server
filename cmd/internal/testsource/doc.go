// Package testsource answers the three questions every command that reads
// _test.go files in this repository used to answer for itself: whether a
// function name is a Go test entry point, which naming bucket that name falls
// in, and which files a scan of the tree may look at.
//
// The three had drifted. Two generators counted test functions with rules that
// disagreed about a name whose first rune after "Test" is neither upper nor
// lower case, and the naming auditor skipped every name starting with the
// "TestMain" prefix, so every test named TestMain_Something was counted by the
// generators and invisible to the auditor. Go's own rule (testing.isTest) is
// what IsTestFunction implements, and it decides for all of them: the prefix
// "Test", the next rune not lower case, and exactly "TestMain" excluded as the
// framework entry point rather than a test.
//
// The walk is here for the same reason. Four commands walked the same tree
// with two skip lists and two descents that skipped nothing, so "is testdata
// part of the corpus" had two answers and no recorded reason. SkipDir is that
// answer, written once: generated and vendored trees (node_modules, dist), a
// tool's own fixtures (testdata, whose Go files are inputs to a test rather
// than source this repository holds to its conventions) and every
// dot-directory.
//
// One predicate deliberately stays where it is. cmd/godoc_tool asks which
// functions need a test-form doc comment, not which functions the testing
// package runs, so it keeps TestMain and the lower-case Test-prefixed helpers
// that IsTestFunction excludes; routing it through here would drop those
// findings from the documentation audit.
//
// Discovery of the corpus itself is deliberately not here. cmd/gen_stats asks
// git for the tracked files so that its --check is a function of what is
// committed, and cmd/gen_testing_docs enumerates packages through go list
// because it describes packages; sharing the predicate is what those two
// needed, and sharing the input universe would break both.
package testsource
