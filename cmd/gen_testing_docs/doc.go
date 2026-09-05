// Command gen_testing_docs regenerates the managed test metrics section in
// docs/development/testing/testing.md.
//
// It discovers Go packages, counts Test* functions by parsing _test.go files,
// runs unit-test coverage for ./internal/... and ./cmd/..., and replaces the
// generated Markdown block in the testing reference document.
//
// Usage:
//
//	go run ./cmd/gen_testing_docs/
//	go run ./cmd/gen_testing_docs/ --check
package main
