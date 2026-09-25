//go:build e2e

// orphans_test.go is the one entry point of the on-demand sweep, which a
// person runs on purpose through make e2e-clean-orphans after a run that could
// not clean up after itself, such as one killed before its exit sweep.
//
// It is a test rather than a command because the fixture library is
// importable only from test/e2e, by Go's internal rule, and a test binary is
// what that tree already builds. It does nothing unless asked: a run of the
// fixture package's tests with neither E2E_SWEEP_MIN_AGE nor E2E_SWEEP_PREFIX
// set skips it, so nothing is deleted by accident, which is what the suite
// this replaces did at every start. Which sweep runs when one is set is
// resolveOrphanScope's to decide, where a test can reach it.

package fixture

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// The instance the on-demand sweep reads from the process environment, and
// what to say when it is asked for without one.
const (
	sweepURLEnv      = "GITLAB_URL"
	sweepTokenEnv    = "GITLAB_TOKEN"
	sweepSkipTLSEnv  = "GITLAB_MCP_SKIP_TLS_VERIFY"
	sweepDefaultHint = "set " + sweepMinAgeEnv + " to how long ago a run must have started, such as 2h, or " +
		sweepPrefixEnv + " to a prefix to sweep by, and " + sweepURLEnv + " and " + sweepTokenEnv + " to the instance"
)

// TestSweepOrphans_Leftovers_OnDemand permanently deletes what earlier runs
// left on the instance GITLAB_URL and GITLAB_TOKEN name: every owned project
// and group, every personal snippet of the token's user and, with an
// administrator's token, every user, whose name carries the identifier of a
// run that started more than E2E_SWEEP_MIN_AGE ago, or whose name opens with
// E2E_SWEEP_PREFIX when that is set instead. It fails on anything it could
// not remove, and skips unless one of the two is set.
//
// The token user's own snippets, projects and groups are reached like the
// suite's: on a self-hosted instance where the token is a person's account,
// an object of theirs that matches is deleted too.
func TestSweepOrphans_Leftovers_OnDemand(t *testing.T) {
	scope, asked, err := resolveOrphanScope(os.Getenv, time.Now())
	if !asked {
		t.Skipf("nothing to sweep: %s", sweepDefaultHint)
	}
	if err != nil {
		t.Fatalf("%v", err)
	}
	url, token := os.Getenv(sweepURLEnv), os.Getenv(sweepTokenEnv)
	if url == "" || token == "" {
		t.Fatalf("a sweep is asked for and the instance is not: %s", sweepDefaultHint)
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

	report := scope.run(ctx, client, user.IsAdmin)

	t.Logf("sweep of %s on %s as %s (admin=%t): %s", scope, url, user.Username, user.IsAdmin, report)
	for _, removed := range report.Removed() {
		t.Logf("removed %s", removed)
	}
	if sweepErr := report.Err(); sweepErr != nil {
		t.Fatalf("the sweep could not remove everything it found: %v", sweepErr)
	}
}
