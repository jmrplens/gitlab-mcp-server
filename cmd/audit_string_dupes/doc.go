// Command audit_string_dupes scans Go source files for duplicated string literals
// that appear three or more times and are not already declared as constants.
// It uses the go/ast parser to inspect string literals and filters out short
// strings (< 3 chars) and JSON field names.
//
// Usage:
//
//	go run ./cmd/audit_string_dupes/ <dir|file>...
package main
