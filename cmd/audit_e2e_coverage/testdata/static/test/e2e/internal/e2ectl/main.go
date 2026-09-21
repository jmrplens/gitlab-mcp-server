//go:build e2e

// Package main is a command beside the harness. It is loaded like every other
// package under test/e2e/internal, and it is here because a main package is
// passed over by the scan only when it is the test main go test generates: a
// command of the suite's own is a consumer of the harness like any other.
package main

import "example.com/e2efake/test/e2e/internal/harness"

func main() { _ = harness.Version }
