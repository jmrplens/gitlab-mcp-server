//go:build e2e

// grouprelationsexport_test.go covers the relations export of a group, the
// ndjson export direct transfer reads: scheduled on a fresh group, then
// its per-relation status listed. The Docker stack enables the bulk import
// setting the endpoints need; an instance that has it off answers 404 to
// the schedule, and the test skips with that reason.

package common

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/grouprelationsexport"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestGroupRelationsExport_ScheduleAndStatus_AnswerTheGroup schedules the
// relations export of a group of each surface's own and lists the export
// status, whose rows, when Sidekiq has written any, each name a relation.
//
// Replaces: TestMeta_GroupRelationsExport
func TestGroupRelationsExport_ScheduleAndStatus_AnswerTheGroup(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("relations"))
		params := map[string]any{"group_id": group.IDParam()}

		scheduled, err := harness.Try[toolutil.VoidOutput](s, actionGroupRelationsSchedule, params)
		if err != nil && strings.Contains(err.Error(), "404") {
			e.Skipf("the relations export needs the bulk_import setting, which this instance has off: %s", firstLine(err.Error()))
		}
		if err != nil || scheduled.Status != voidStatusSuccess {
			e.T.Fatalf("group_relations_schedule answered %+v, %v; want a %s status", scheduled, err, voidStatusSuccess)
		}

		// How many rows the listing holds is Sidekiq's business: the export
		// is asynchronous and the rows appear as it runs. That the listing
		// answers for the group, and that each row it holds names a
		// relation, is what is assertable here.
		statuses := harness.Do[grouprelationsexport.ListExportStatusOutput](s, actionGroupRelationsListStatus, params)
		for _, status := range statuses.Statuses {
			if status.Relation == "" {
				e.T.Errorf("the relations export lists a status naming no relation: %+v", status)
			}
		}
		e.T.Logf("the relations export of group %d reports %d relation status(es)", group.ID, len(statuses.Statuses))
	})
}
