// -check-release: whether a run may go ahead on the GitLab release it found,
// measured against the release the fine-grained action table was recorded
// from (issue 1155).
//
// test/e2e/scripts/run-docker-e2e.sh asks it right after the instance answers
// its version and before go test starts, so a Docker run against a release the
// table has fallen behind stops after the boot rather than after the whole
// suite has passed in phase A. The rule is internal/testutil/e2erelease's, the
// one the harness refuses a self-hosted run with in its environment setup, so
// the two stop on the same releases with the same words.

package main

import (
	"fmt"
	"io"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2erelease"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actiongrants"
)

// runCheckRelease runs -check-release. A release the run goes ahead on (the
// table's own, the prerelease right after it, or an older one, which is a
// deliberate pin) prints one line on stdout and exits 0. A newer release is a
// finding, exit 1, with the text that names both releases and the steps to
// re-record the table. A version that does not parse, and a call naming none,
// is a run the gate could not judge, exit 2, since the script hands it what
// the instance answered and an answer it cannot read must stop the run rather
// than pass it.
func runCheckRelease(opts options, stdout, stderr io.Writer) int {
	if opts.gitlabVersion == "" {
		fmt.Fprintln(stderr, "audit_e2e_coverage: -check-release needs -gitlab-version, the release the instance under test reported")
		return exitUsage
	}
	table := opts.releaseTable
	if table == nil {
		table = actiongrants.Table()
	}
	decision := e2erelease.Decide(opts.gitlabVersion, table)
	if decision.Runs() {
		fmt.Fprintln(stdout, "release: "+decision.Text())
		return exitOK
	}
	fmt.Fprintln(stderr, "release: "+decision.Text())
	if decision.Verdict == e2erelease.Newer {
		return exitFindings
	}
	return exitUsage
}
