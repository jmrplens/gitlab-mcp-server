// Command audit_test_names scans all Go test files and classifies test
// function names by their naming pattern. It outputs a CSV report with
// columns: file, current_name, pattern, suggested_name.
//
// With -apply it renames test functions in place to match the suggested
// names. With -dry-run it prints what would change without writing.
//
// Usage:
//
//	go run ./cmd/audit_test_names/ <dir>...
//	go run ./cmd/audit_test_names/ -apply <dir>...
//	go run ./cmd/audit_test_names/ -apply -dry-run <dir>...
package main
