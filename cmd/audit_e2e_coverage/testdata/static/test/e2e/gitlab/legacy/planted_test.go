//go:build e2e

package legacy

import (
	"testing"

	"example.com/e2efake/test/e2e/internal/harness"
)

// TestPlanted_UndeclaredPlacement_Reported names a Free action from a package
// under test/e2e/gitlab that is none of common, ce or ee. The id itself is
// fine; the package is the finding.
func TestPlanted_UndeclaredPlacement_Reported(t *testing.T) {
	s := harness.New(t).Session()
	harness.DoVoid(s, "project.get", nil)
}
