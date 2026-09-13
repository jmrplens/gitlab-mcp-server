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

// TestIssue_Retired replaces a test the old tree no longer declares, which
// the retired list keeps on the map.
// Replaces: TestMeta_Retired
func TestIssue_Retired(t *testing.T) {
	_ = t
}

// helper carries a Replaces line that counts for nothing, since it is not a
// test.
// Replaces: TestMeta_Unresolved
func helper(t *testing.T) {
	_ = t
}

// Testhelper carries the same claim under a name go test does not run, which
// counts for nothing on the same terms.
// Replaces: TestMeta_Unresolved
func Testhelper(t *testing.T) {
	_ = t
}
