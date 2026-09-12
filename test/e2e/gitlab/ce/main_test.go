//go:build e2e

// main_test.go declares what this package needs of the instance it runs on:
// no license. The harness probes the instance when the first test asks and
// refuses the whole package, naming the target that provides what it needs,
// when it finds one installed.

package ce

import (
	"os"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestMain hands the package to the harness with the Free requirement.
func TestMain(m *testing.M) {
	os.Exit(harness.Main(m, harness.Free))
}
