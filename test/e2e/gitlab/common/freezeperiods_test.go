//go:build e2e

// freezeperiods_test.go covers a project's deploy freeze period through
// its life on every surface: created from two cron expressions, listed,
// read, moved to another time zone, and deleted.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/freezeperiods"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The freeze window: Friday evening to Monday morning, first in UTC and
// then moved to New York.
const (
	freezeStart         = "0 23 * * 5"
	freezeEnd           = "0 7 * * 1"
	freezeTimezone      = "UTC"
	freezeMovedTimezone = "America/New_York"
)

// freezePeriodIDs lists the ids of a freeze period listing.
func freezePeriodIDs(listed []freezeperiods.Output) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, period := range listed {
		ids = append(ids, period.ID)
	}
	return ids
}

// TestFreezePeriods_Lifecycle_CreateListGetUpdateDelete creates a freeze
// period in a project of each surface's own, finds it in the listing,
// reads it, changes its time zone, deletes it and checks the listing lets
// it go.
//
// Replaces: TestMeta_FreezePeriods
func TestFreezePeriods_Lifecycle_CreateListGetUpdateDelete(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		project := fixture.NewProject(e, fixture.WithNamePrefix("freeze"))
		params := map[string]any{"project_id": project.IDParam()}

		created := harness.Do[freezeperiods.Output](s, actionFreezePeriodCreate, withParams(params, map[string]any{
			"freeze_start": freezeStart, "freeze_end": freezeEnd, "cron_timezone": freezeTimezone,
		}))
		if created.ID == 0 || created.FreezeStart != freezeStart || created.FreezeEnd != freezeEnd || created.CronTimezone != freezeTimezone {
			e.T.Fatalf("freeze_create answered %+v, want the window %q to %q in %s with an ID", created, freezeStart, freezeEnd, freezeTimezone)
		}
		period := withParams(params, map[string]any{"freeze_period_id": created.ID})

		listed := harness.Do[freezeperiods.ListOutput](s, actionFreezePeriodList, params)
		if !containsID(freezePeriodIDs(listed.FreezePeriods), created.ID) {
			e.T.Errorf("the project lists the freeze periods %v, want %d among them", freezePeriodIDs(listed.FreezePeriods), created.ID)
		}
		got := harness.Do[freezeperiods.Output](s, actionFreezePeriodGet, period)
		if got.ID != created.ID || got.FreezeStart != freezeStart {
			e.T.Errorf("freeze_get answered %+v, want period %d starting %q", got, created.ID, freezeStart)
		}

		moved := harness.Do[freezeperiods.Output](s, actionFreezePeriodUpdate, withParams(period, map[string]any{"cron_timezone": freezeMovedTimezone}))
		if moved.ID != created.ID || moved.CronTimezone != freezeMovedTimezone {
			e.T.Errorf("freeze_update answered %+v, want period %d in %s", moved, created.ID, freezeMovedTimezone)
		}

		harness.DoVoid(s, actionFreezePeriodDelete, period)
		remaining := harness.Do[freezeperiods.ListOutput](s, actionFreezePeriodList, params)
		if containsID(freezePeriodIDs(remaining.FreezePeriods), created.ID) {
			e.T.Errorf("the project still lists freeze period %d after its delete", created.ID)
		}
	})
}
