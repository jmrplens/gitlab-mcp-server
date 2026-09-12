//go:build e2e

// orphans_test.go is the one entry point of the prefix-wide sweep, which a
// person runs on purpose through make e2e-clean-orphans after a run on a
// self-hosted instance could not clean up after itself.
//
// It is a test rather than a command because the fixture library is
// importable only from test/e2e, by Go's internal rule, and a test binary is
// what that tree already builds. It does nothing unless asked: a run of the
// fixture package's tests with no prefix configured skips it, so nothing
// deletes by prefix by accident, which is what the suite this replaces did
// at every start.

package fixture

import (
	"context"
	"os"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// The configuration the on-demand sweep reads from the process environment:
// the instance, its credential, and the prefix that turns the sweep on.
const (
	sweepPrefixEnv   = "E2E_SWEEP_PREFIX"
	sweepURLEnv      = "GITLAB_URL"
	sweepTokenEnv    = "GITLAB_TOKEN"
	sweepSkipTLSEnv  = "GITLAB_MCP_SKIP_TLS_VERIFY"
	sweepDefaultHint = "set " + sweepPrefixEnv + " to the prefix to sweep, such as e2e-, and " + sweepURLEnv + " and " + sweepTokenEnv + " to the instance"
)

// TestSweepPrefix_Orphans_OnDemand permanently deletes every project, group
// and, with an administrator's token, every user whose name opens with
// E2E_SWEEP_PREFIX on the instance GITLAB_URL and GITLAB_TOKEN name, and
// fails on anything it could not remove. It skips unless the prefix is set.
func TestSweepPrefix_Orphans_OnDemand(t *testing.T) {
	prefix := strings.TrimSpace(os.Getenv(sweepPrefixEnv))
	if prefix == "" {
		t.Skipf("no prefix to sweep: %s", sweepDefaultHint)
	}
	url, token := os.Getenv(sweepURLEnv), os.Getenv(sweepTokenEnv)
	if url == "" || token == "" {
		t.Fatalf("%s is set and the instance is not: %s", sweepPrefixEnv, sweepDefaultHint)
	}

	client, err := gitlabclient.NewClientWithToken(url, token, strings.EqualFold(os.Getenv(sweepSkipTLSEnv), "true"))
	if err != nil {
		t.Fatalf("building a client for %s: %v", url, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), sweepBudget)
	defer cancel()

	user, _, err := client.GL().Users.CurrentUser(gl.WithContext(ctx))
	if err != nil {
		t.Fatalf("reading the authenticated user of %s: %v", url, err)
	}

	report := SweepPrefix(ctx, client, prefix, user.IsAdmin)

	t.Logf("sweep of %q on %s as %s (admin=%t): %s", prefix, url, user.Username, user.IsAdmin, report)
	for _, path := range report.Projects {
		t.Logf("removed project %s", path)
	}
	for _, path := range report.Groups {
		t.Logf("removed group %s", path)
	}
	for _, username := range report.Users {
		t.Logf("removed user %s", username)
	}
	if sweepErr := report.Err(); sweepErr != nil {
		t.Fatalf("the sweep could not remove everything it found: %v", sweepErr)
	}
}
