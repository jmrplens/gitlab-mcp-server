//go:build e2e

// main_test.go declares what this package needs of the instance it runs on:
// nothing in particular. The harness resolves the configuration here and
// contacts GitLab only when the first test asks it to, so a run that measures
// nothing never reaches an instance and never spends a token.

package modeleval

import (
	"os"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestMain hands the package to the harness with the Any requirement. Which
// cases a run may attempt is decided by their own declared needs, case by
// case, and not by a requirement over the whole package: a corpus holding a
// licensed case would otherwise refuse the whole run on a CE instance instead
// of skipping that case with a reason.
func TestMain(m *testing.M) {
	os.Exit(harness.Main(m, harness.Any))
}
