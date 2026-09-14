//go:build e2e

// milestones_test.go covers a project's milestones through the server:
// create one, read it, describe and close it through an update, delete it
// and check the read is then refused, once per surface on one shared
// project.

package common

import (
	"testing"

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
