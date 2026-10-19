//go:build e2e

// milestones_test.go covers a project's milestones through the server:
// create one, read it, describe and close it through an update, delete it
// and check the read is then refused, once per surface on one shared
// project; and the promotion of a milestone to the group its project sits
// in.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupmilestones"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/milestones"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestProjectMilestones_Lifecycle_CreateGetCloseDelete creates a milestone
// on every surface, reads it back active, closes it with a new description
// through one update, deletes it and checks the read answers not found.
//
// Replaces: TestIndividual_Milestones, TestMeta_Milestones
func TestProjectMilestones_Lifecycle_CreateGetCloseDelete(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("milestones"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}
		title := e.Name("milestone")

		created := harness.Do[milestones.Output](s, actionProjectMilestoneCreate, withParams(params, map[string]any{
			"title": title, "description": "created by the e2e suite",
		}))
		if created.IID == 0 || created.Title != title {
			e.T.Fatalf("milestone_create answered %+v, want the milestone %q with an iid", created, title)
		}
		milestoneParams := withParams(params, map[string]any{"milestone_iid": created.IID})

		got := harness.Do[milestones.Output](s, actionProjectMilestoneGet, milestoneParams)
		if got.IID != created.IID || got.Title != title || got.State != "active" {
			e.T.Errorf("milestone_get answered %+v, want the active milestone %d titled %q", got, created.IID, title)
		}

		closed := harness.Do[milestones.Output](s, actionProjectMilestoneUpdate, withParams(milestoneParams, map[string]any{
			"description": "closed by the e2e suite", "state_event": "close",
		}))
		if closed.IID != created.IID || closed.State != "closed" || closed.Description != "closed by the e2e suite" {
			e.T.Errorf("milestone_update answered %+v, want milestone %d closed with the new description", closed, created.IID)
		}

		harness.DoVoid(s, actionProjectMilestoneDelete, milestoneParams)
		refused := harness.Refused(s, actionProjectMilestoneGet, milestoneParams, harness.FailureNotFound)
		e.T.Logf("the read of the deleted milestone was refused: %s", firstLine(refused))
	})
}

// TestProjectMilestones_Promote_MovesTheMilestoneToTheGroup promotes a
// milestone of a group's project on every surface and finds it among the
// group's milestones under its title, while the project milestone is gone;
// the promotion of a personal project's milestone is refused, with the hint
// that the project has to belong to a group.
func TestProjectMilestones_Promote_MovesTheMilestoneToTheGroup(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)

		personal := fixture.NewProject(e, fixture.WithNamePrefix("personalmilestone"))
		stranded := fixture.NewMilestone(e, personal, "stranded")
		refused := harness.ExpectToolError(s, actionProjectMilestonePromote,
			map[string]any{"project_id": personal.IDParam(), "milestone_iid": stranded.IID}, "belong to a group")
		e.T.Logf("the promotion of a personal project's milestone is refused: %s", firstLine(refused))

		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("milestonepromotion"))
		project := fixture.NewProject(e, fixture.WithNamePrefix("groupedmilestone"), fixture.InGroup(group))
		milestone := fixture.NewMilestone(e, project, "promotable")
		params := map[string]any{"project_id": project.IDParam(), "milestone_iid": milestone.IID}

		promoted := harness.Do[milestones.PromoteOutput](s, actionProjectMilestonePromote, params)
		if promoted.Title != milestone.Title {
			e.T.Errorf("milestone_promote answered %+v, want the title %q", promoted, milestone.Title)
		}
		listed := harness.Do[groupmilestones.ListOutput](s, actionGroupMilestoneList,
			map[string]any{"group_id": group.IDParam(), "title": milestone.Title})
		if len(listed.Milestones) != 1 || listed.Milestones[0].Title != milestone.Title {
			e.T.Errorf("the group's milestones titled %q are %+v, want the promoted one", milestone.Title, listed.Milestones)
		}
		gone := harness.Refused(s, actionProjectMilestoneGet, params, harness.FailureNotFound)
		e.T.Logf("the read of the promoted project milestone was refused: %s", firstLine(gone))
	})
}
