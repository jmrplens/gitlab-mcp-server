//go:build e2e

// runners_test.go covers what a session can say about the instance runner
// the Docker stack registers: list it at every scope, read it and its
// managers, and show what happens when a project tries to claim it, which
// an instance runner refuses, and to release it, which a runner never held
// answers as a not-found.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/runners"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// missingGroupID is a group nothing on an instance has, spelled as the
// group_id parameter takes it.
const missingGroupID = "0"

// TestRunners_DockerRunner_ListedReadAndNotClaimable finds the Docker
// runner in the instance listing on every surface, reads it and its
// managers, lists a project's and a group's runners, and shows a project
// can neither claim nor release an instance runner.
//
// Replaces: TestEE_MetaRunnerManagement
func TestRunners_DockerRunner_ListedReadAndNotClaimable(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin, harness.NeedRunner))
	runnerID := fixture.DockerRunnerID(e)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("runners"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("runners"))

		all := harness.Do[runners.ListOutput](s, actionRunnerListAll, nil)
		found := false
		for _, runner := range all.Runners {
			if runner.ID == runnerID {
				found = true
			}
		}
		if !found {
			e.T.Errorf("the instance listing of %d runner(s) does not hold the Docker runner %d", len(all.Runners), runnerID)
		}

		ofProject := harness.Do[runners.ListOutput](s, actionRunnerListProject, map[string]any{"project_id": project.IDParam()})
		e.T.Logf("project %s sees %d runner(s)", project.Path, len(ofProject.Runners))
		ofGroup := harness.Do[runners.ListOutput](s, actionRunnerListGroup, map[string]any{"group_id": group.IDParam()})
		e.T.Logf("group %s sees %d runner(s)", group.Path, len(ofGroup.Runners))
		refused := harness.Refused(s, actionRunnerListGroup, map[string]any{"group_id": missingGroupID}, harness.FailureNotFound)
		e.T.Logf("the runners of a missing group are refused: %s", firstLine(refused))

		got := harness.Do[runners.DetailsOutput](s, actionRunnerGet, map[string]any{"runner_id": runnerID})
		if got.ID != runnerID {
			e.T.Errorf("get answered runner %d, want %d", got.ID, runnerID)
		}
		managers := harness.Do[runners.ManagerListOutput](s, actionRunnerListManagers, map[string]any{"runner_id": runnerID})
		e.T.Logf("runner %d has %d manager(s)", runnerID, len(managers.Managers))

		claim := map[string]any{"project_id": project.IDParam(), "runner_id": runnerID}
		refused = harness.ExpectToolError(s, actionRunnerEnableProject, claim, "runner")
		e.T.Logf("claiming the instance runner for a project is refused: %s", firstLine(refused))
		refused = harness.Refused(s, actionRunnerDisableProject, claim, harness.FailureNotFound)
		e.T.Logf("releasing a runner the project never held is refused: %s", firstLine(refused))
	})
}
