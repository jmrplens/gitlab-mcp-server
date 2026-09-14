//go:build e2e

// actions_b5_test.go names the catalog action the search family calls
// here, as a typed constant for the push-time static gate to read.

package ce

import "github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"

// actionSearchCode is the code scope of the search group, which without a
// project is a global search.
const actionSearchCode harness.ActionID = "search.code"
