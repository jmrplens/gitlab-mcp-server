// Command gen_testing_docs regenerates the managed test metrics section in
// docs/development/testing/testing.md.
//
// It discovers Go packages, counts Test* functions by parsing _test.go files,
// runs unit-test coverage for ./internal/... and ./cmd/..., and replaces the
// generated Markdown block in the testing reference document.
//
// A check never runs coverage unless it is asked to. The coverage pass is what
// takes the minutes, and the numbers it produces depend on the machine as much
// as on the tree, so --check carries the values already recorded in the
// document forward and holds everything a checkout determines. Measuring under
// a check is available and explicit: --check -skip-coverage=false.
//
// Usage:
//
//	go run ./cmd/gen_testing_docs/
//	go run ./cmd/gen_testing_docs/ --check
//	go run ./cmd/gen_testing_docs/ --check -skip-coverage=false
package main
