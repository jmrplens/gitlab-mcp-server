//go:build e2e

package common

import "testing"

// TestIssue_NotATestFile carries a Replaces line in a file that is not a
// _test.go, so the walk never parses it and the claim retires nothing. A
// non-test file beside the tests is ordinary: the new suite keeps its
// fixtures and helpers in one.
// Replaces: TestMeta_Unresolved
func TestIssue_NotATestFile(t *testing.T) {
	_ = t
}
