//go:build e2e

// b7_issue_timetracking_test.go covers an issue's time tracking through the
// server: the estimate set and cleared, the time logged against it and
// cleared, and the totals read back between the two. The old CE suite
// asserted all five routes and the rebuilt suite reached none of them.
//
// The durations are asserted as the seconds GitLab parsed them into rather
// than as the human strings it echoes back, because the seconds are what the
// API promises and the spacing of "3h 30m" is GitLab's own presentation.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The two durations this file logs, spelled as the parameter takes them and
// as the seconds GitLab answers with.
const (
	issueEstimateDuration = "3h30m"
	issueEstimateSeconds  = 12600
	issueSpentDuration    = "1h"
	issueSpentSeconds     = 3600
)

// TestIssueTimeTracking_Lifecycle_EstimateSpendReadAndReset sets an estimate
// on an issue of its own on every surface, logs an hour against it, reads
// the totals back, then clears the estimate and the logged time, holding
// every answer to the seconds GitLab parsed each duration into.
func TestIssueTimeTracking_Lifecycle_EstimateSpendReadAndReset(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("issuetime"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		// An issue per surface: the totals are the issue's own, so three
		// surfaces logging time against one would each read the others' hours.
		issue := fixture.NewIssue(e, project, "time tracking fixture for the "+string(surface)+" surface")
		params := map[string]any{"project_id": project.IDParam(), "issue_iid": issue.IID}

		estimated := harness.Do[issues.TimeStatsOutput](s, actionIssueTimeEstimateSet,
			withParams(params, map[string]any{"duration": issueEstimateDuration}))
		if estimated.TimeEstimate != issueEstimateSeconds {
			e.T.Fatalf("time_estimate_set answered %+v, want %s read as %d seconds", estimated, issueEstimateDuration, issueEstimateSeconds)
		}

		spent := harness.Do[issues.TimeStatsOutput](s, actionIssueSpentTimeAdd,
			withParams(params, map[string]any{"duration": issueSpentDuration, "summary": "logged by the e2e suite"}))
		if spent.TotalTimeSpent != issueSpentSeconds {
			e.T.Errorf("spent_time_add answered %+v, want %s read as %d seconds spent", spent, issueSpentDuration, issueSpentSeconds)
		}

		stats := harness.Do[issues.TimeStatsOutput](s, actionIssueTimeStatsGet, params)
		if stats.TimeEstimate != issueEstimateSeconds || stats.TotalTimeSpent != issueSpentSeconds {
			e.T.Errorf("time_stats_get answered %+v, want %d estimated and %d spent", stats, issueEstimateSeconds, issueSpentSeconds)
		}
		if stats.HumanTimeEstimate == "" || stats.HumanTotalTimeSpent == "" {
			e.T.Errorf("time_stats_get answered %+v, want the human-readable totals beside the seconds", stats)
		}

		clearedEstimate := harness.Do[issues.TimeStatsOutput](s, actionIssueTimeEstimateReset, params)
		if clearedEstimate.TimeEstimate != 0 {
			e.T.Errorf("time_estimate_reset answered %+v, want the estimate cleared", clearedEstimate)
		}

		clearedSpent := harness.Do[issues.TimeStatsOutput](s, actionIssueSpentTimeReset, params)
		if clearedSpent.TotalTimeSpent != 0 {
			e.T.Errorf("spent_time_reset answered %+v, want the logged time cleared", clearedSpent)
		}
	})
}
