//go:build e2e

// b7_issue_safemode_test.go covers the issue listing under safe mode, which
// the old CE suite asserted on the individual surface and the rebuilt suite
// asserts on no surface at all.
//
// It is the half of safe mode mcp_modes_test.go does not say: that file
// holds a write to its preview and reads one project back, and this one
// holds a listing to the issue it is supposed to find. Safe mode intercepts
// mutating operations only, so a read must come back as the read's own
// answer rather than as a card describing it, which is what Do asserts
// before this test looks at anything, since a preview is a refusal to it.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestIssueList_UnderSafeMode_StillAnswersTheListing lists the issues of a
// project holding exactly one, on a safe-mode session of every surface, and
// holds the answer to that issue.
func TestIssueList_UnderSafeMode_StillAnswersTheListing(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) issueFixture {
		return newIssueFixture(e, "issuesafelist")
	}, func(e *harness.Env, surface harness.Surface, f issueFixture) {
		s := e.Session(harness.ServerConfig{Surface: surface, Mode: harness.ModeSafe})

		listed := harness.Do[issues.ListOutput](s, actionIssueList,
			map[string]any{"project_id": f.project.IDParam(), "state": "opened"})
		if len(listed.Issues) != 1 || listed.Issues[0].IID != f.issue.IID {
			e.T.Errorf("the safe-mode listing answered %v, want the one fixture issue #%d",
				issueIIDs(listed.Issues), f.issue.IID)
		}
	})
}
