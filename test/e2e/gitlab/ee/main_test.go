//go:build e2e

// main_test.go declares what this package needs of the instance it runs on:
// a Premium or Ultimate license. Against an unlicensed instance the harness
// refuses the whole package before any test writes anything, and names the
// target that provides one; E2E_RUNTIME_MISMATCH=skip turns that refusal
// into skips.

package ee

import (
	"os"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestMain hands the package to the harness with the Licensed requirement.
func TestMain(m *testing.M) {
	os.Exit(harness.Main(m, harness.Licensed))
}
