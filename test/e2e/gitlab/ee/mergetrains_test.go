//go:build e2e

// mergetrains_test.go covers the merge train actions on a project that has
// trains switched on: the project listing, the branch listing, adding a
// merge request and reading its entry.
//
// The project lives in a group, because GitLab keeps merge_trains_enabled
// only on a project whose namespace carries the licensed feature; on a
// personal namespace the edit answers 200 and the flag is dropped, which is
// how the suite this replaces spent its first runs asserting on a project
// with no train. Even with trains on, a merge request with no pipeline of
// its own cannot board: GitLab refuses the add, and the entry read for it
// is a not-found. Both are the deterministic answers a project without CI
// gives, and both are asserted as such rather than tolerated.

package ee

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mergetrains"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// mergeTrainFixture is a group-scoped project with merge trains enabled.
type mergeTrainFixture struct {
	project fixture.Project
}

// buildMergeTrainFixture creates the group, the project in it, and turns
// the trains on. The project lives in a group so that GitLab keeps both
// switches; a project whose switches did not stick would make the scenario
// assert the refusals of a project without a train, which is not what it
// claims to cover, so that is a failure rather than a note.
func buildMergeTrainFixture(e *harness.Env) mergeTrainFixture {
	group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("mt"))
	project := fixture.NewProject(e, fixture.WithNamePrefix("mt"), fixture.InGroup(group))
	if !fixture.EnableMergeTrains(e, project) {
		e.T.Fatalf("GitLab did not keep merge trains enabled on %s, which is a group project on a licensed instance", project.Path)
	}
	return mergeTrainFixture{project: project}
}

// TestMergeTrains_ProjectInGroup_ListsAndRefusesAnUnpipelinedRequest lists
// the project's and the target branch's trains on every surface, opens a
// merge request of the surface's own, and shows that a request with no
// pipeline is refused boarding and reported as not on a train.
//
// Replaces: TestMeta_MergeTrains, TestMeta_MergeTrainGet
func TestMergeTrains_ProjectInGroup_ListsAndRefusesAnUnpipelinedRequest(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, buildMergeTrainFixture, func(e *harness.Env, surface harness.Surface, f mergeTrainFixture) {
		s := e.On(surface)
		project := f.project.IDParam()

		listed := harness.Do[mergetrains.ListOutput](s, actionMergeTrainListProject, map[string]any{"project_id": project})
		if len(listed.Trains) != 0 {
			e.T.Errorf("a project nothing has boarded lists %d train entries: %+v", len(listed.Trains), listed.Trains)
		}

		branch := fixture.NewBranch(e, f.project, e.Name("mt"))
		fixture.CommitFile(e, f.project, branch.Name, "merge-train.txt", "merge train fixture\n", "add the merge train fixture")
		mr := fixture.NewMergeRequest(e, f.project, branch.Name, f.project.DefaultBranch, "merge train fixture")

		// No pipeline ever ran for this request, so GitLab cannot board it.
		// The refusal is asserted by the operation it names rather than by
		// GitLab's wording, which differs between a train that is off and a
		// request that cannot board one.
		refused := harness.ExpectToolError(s, actionMergeTrainAdd,
			map[string]any{"project_id": project, "merge_request_iid": mr.IID}, "merge_train")
		e.T.Logf("add refused for merge request !%d as expected: %s", mr.IID, firstLine(refused))

		onBranch := harness.Do[mergetrains.ListOutput](s, actionMergeTrainListBranch,
			map[string]any{"project_id": project, "target_branch": f.project.DefaultBranch})
		if len(onBranch.Trains) != 0 {
			e.T.Errorf("the target branch lists %d train entries after a refused add: %+v", len(onBranch.Trains), onBranch.Trains)
		}

		notFound := harness.Refused(s, actionMergeTrainGet,
			map[string]any{"project_id": project, "merge_request_iid": mr.IID}, harness.FailureNotFound)
		assertMentions(e, "the entry read for a request that never boarded", notFound, "merge train")
	})
}
