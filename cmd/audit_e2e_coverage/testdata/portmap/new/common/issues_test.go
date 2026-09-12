//go:build e2e

package common

import "testing"

// TestIssue_Lifecycle covers issue creation to deletion on every surface.
//
// Replaces: TestMeta_Issues, TestIndividual_Issues/Create
func TestIssue_Lifecycle(t *testing.T) {
	_ = t
}

// TestIssue_Ghost names an old test that never existed.
// Replaces: TestMeta_Ghost
func TestIssue_Ghost(t *testing.T) {
	_ = t
}

// TestIssue_Both replaces a test that is also declared dropped.
// Replaces: TestBoth
func TestIssue_Both(t *testing.T) {
	_ = t
}

// helper carries a Replaces line that counts for nothing, since it is not a
// test.
// Replaces: TestMeta_Unresolved
func helper(t *testing.T) {
	_ = t
}
