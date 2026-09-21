//go:build e2e

package ce

import (
	"testing"

	"example.com/e2efake/test/e2e/internal/harness"
)

// TestPlanted_FreeInCE_Clean names a Free action from the package that runs
// only on an unlicensed instance, which is a declared placement and earns no
// finding. The action is one common names too, so this package adds a
// placement and not a scenario the ratchet would credit.
func TestPlanted_FreeInCE_Clean(t *testing.T) {
	s := harness.New(t).Session()
	harness.DoVoid(s, "server.status", nil)
}

// TestPlanted_SharedName_FoldsBothPackages shares its name with a test in
// common, which names another id; the map from a test to its ids is keyed by
// the name alone and carries both.
func TestPlanted_SharedName_FoldsBothPackages(t *testing.T) {
	harness.DoVoid(harness.New(t).Session(), "server.status", nil)
}
