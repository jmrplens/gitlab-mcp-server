//go:build e2e

package hidden

import "testing"

// TestIssue_Hidden sits under a dot-directory, which the recursive walk
// prunes because it is not this repository's own source: the parallel-agent
// tooling puts a whole worktree of this repository under one, and reading it
// would fold another branch's Replaces lines into this map.
// Replaces: TestMeta_Unresolved
func TestIssue_Hidden(t *testing.T) {
	_ = t
}
