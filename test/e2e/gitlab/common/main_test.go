//go:build e2e

// main_test.go declares what this package needs of the instance it runs on:
// nothing in particular. The harness resolves the configuration here and
// contacts GitLab only when the first test asks it to, so a filtered run and
// a -list never reach an instance.

package common

import (
	"os"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestMain hands the package to the harness with the Any requirement: the
// tests here hold on every runtime.
func TestMain(m *testing.M) {
	os.Exit(harness.Main(m, harness.Any))
}
