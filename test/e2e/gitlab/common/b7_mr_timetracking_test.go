//go:build e2e

// b7_mr_timetracking_test.go covers a merge request's time tracking through
// the server: the estimate is set and cleared, spent time is logged and reset,
// and the totals are read back between the two.
//
// Every one of the five actions answers with the same time-tracking totals, so
// each step is held to the seconds GitLab should have recorded rather than to
// the human-readable string beside them, which is a rendering GitLab is free
// to change.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The durations the scenario logs, and what GitLab records them as. Hours and
// minutes are exact seconds whatever the instance's hours-per-day setting is,
// which a duration spelled in days or weeks would not be.
const (
	mrTimeEstimateDuration = "3h30m"
	mrTimeEstimateSeconds  = int64(3*3600 + 30*60)
	mrSpentTimeDuration    = "1h"
	mrSpentTimeSeconds     = int64(3600)
)

// TestMergeRequestTimeTracking_EstimateAndSpentTime_SetReadAndReset sets an
// estimate on a merge request of its own on every surface, logs spent time
// against it, reads the totals back through the time stats action, then resets
// the spent time and the estimate in turn, holding each answer to the totals
// that step should have left behind.
func TestMergeRequestTimeTracking_EstimateAndSpentTime_SetReadAndReset(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("mrtime"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		f := newMergeRequestIn(e, project, "mrtime")
		params := f.params()

		estimate := harness.Do[mergerequests.TimeStatsOutput](s, actionMergeRequestTimeEstimateSet,
			withParams(params, map[string]any{"duration": mrTimeEstimateDuration}))
		if estimate.TimeEstimate != mrTimeEstimateSeconds {
			e.T.Fatalf("time_estimate_set answered %+v, want an estimate of %d seconds", estimate, mrTimeEstimateSeconds)
		}
		e.T.Logf("the estimate reads %q", estimate.HumanTimeEstimate)

		// Only the duration is sent: summary is an input this server offers and
		// the live API record does not show the endpoint declaring, so a call
		// that carried one would be asserting something the oracle cannot
		// confirm the endpoint reads.
		spent := harness.Do[mergerequests.TimeStatsOutput](s, actionMergeRequestSpentTimeAdd,
			withParams(params, map[string]any{"duration": mrSpentTimeDuration}))
		if spent.TotalTimeSpent != mrSpentTimeSeconds {
			e.T.Fatalf("spent_time_add answered %+v, want %d seconds spent", spent, mrSpentTimeSeconds)
		}
		e.T.Logf("the spent time reads %q", spent.HumanTotalTimeSpent)

		stats := harness.Do[mergerequests.TimeStatsOutput](s, actionMergeRequestTimeStats, params)
		if stats.TimeEstimate != mrTimeEstimateSeconds || stats.TotalTimeSpent != mrSpentTimeSeconds {
			e.T.Errorf("time_stats answered %+v, want %d seconds estimated and %d spent",
				stats, mrTimeEstimateSeconds, mrSpentTimeSeconds)
		}

		// The spent time is reset first, so the estimate the answer carries
		// says the reset touched one total and left the other alone.
		spentReset := harness.Do[mergerequests.TimeStatsOutput](s, actionMergeRequestSpentTimeReset, params)
		if spentReset.TotalTimeSpent != 0 || spentReset.TimeEstimate != mrTimeEstimateSeconds {
			e.T.Errorf("spent_time_reset answered %+v, want no spent time and the estimate of %d seconds left standing",
				spentReset, mrTimeEstimateSeconds)
		}

		estimateReset := harness.Do[mergerequests.TimeStatsOutput](s, actionMergeRequestTimeEstimateReset, params)
		if estimateReset.TimeEstimate != 0 || estimateReset.TotalTimeSpent != 0 {
			e.T.Errorf("time_estimate_reset answered %+v, want both totals back at zero", estimateReset)
		}
	})
}
