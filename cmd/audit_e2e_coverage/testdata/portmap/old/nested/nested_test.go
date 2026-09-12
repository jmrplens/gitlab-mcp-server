//go:build e2e

package nested

import "testing"

// TestNested_Ignored sits one directory down, which the old suite's flat read
// does not descend into.
func TestNested_Ignored(t *testing.T) {
	_ = t
}
