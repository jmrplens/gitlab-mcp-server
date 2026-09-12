//go:build e2e

package harness

import "testing"

// TestFake_Listed is an in-package test of the fake harness. It exists so
// the dead-export scan is shown reading the plain package and not its test
// variant: a Test function is exported by Go's rules and used by nothing,
// and listing it would make every test of the real harness a dead export.
func TestFake_Listed(t *testing.T) {
	if New(t) == nil {
		t.Fatal("New returned nil")
	}
}
