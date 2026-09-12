//go:build e2e

package suite

import "testing"

// TestMain is on the port map like every other Test function.
func TestMain(m *testing.M) {
	_ = m
}

// TestMeta_Issues is replaced by one new test.
func TestMeta_Issues(t *testing.T) {
	_ = t
}

// TestIndividual_Issues is replaced through a subtest reference.
func TestIndividual_Issues(t *testing.T) {
	_ = t
}

// TestMeta_Legacy is declared dropped.
func TestMeta_Legacy(t *testing.T) {
	_ = t
}

// TestMeta_Unresolved has neither a successor nor a drop.
func TestMeta_Unresolved(t *testing.T) {
	_ = t
}

// helperNotATest is not on the map.
func helperNotATest(t *testing.T) {
	_ = t
}

// TestBoth is both replaced and dropped, which the map reports.
func TestBoth(t *testing.T) {
	_ = t
}
