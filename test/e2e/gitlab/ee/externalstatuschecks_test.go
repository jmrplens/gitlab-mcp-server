//go:build e2e

// externalstatuschecks_test.go covers the external status check lifecycle
// on a project and on one of its merge requests: create, list twice (the
// current listing and the deprecated one), update, set the check's status
// on the request, retry, and delete.
//
// The retry is the one step that refuses: a check can only be retried from
// the failed state, and the fixture sets it to passed a moment before, so
// the refusal is the assertion and names the listing to read the state from.

package ee

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/externalstatuschecks"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// statusCheckFixture is a project with a mergeable merge request whose head
// commit the status is set on.
type statusCheckFixture struct {
	project fixture.Project
	mr      fixture.MergeRequest
	commit  fixture.Commit
}

// buildStatusCheckFixture creates the project, a branch with one commit and
// the merge request from it.
func buildStatusCheckFixture(e *harness.Env) statusCheckFixture {
	project := fixture.NewProject(e, fixture.WithNamePrefix("esc"))
	branch := fixture.NewBranch(e, project, e.Name("esc"))
	commit := fixture.CommitFile(e, project, branch.Name, "external-status.txt", "external status check\n", "add the external status check fixture")
	mr := fixture.NewMergeRequest(e, project, branch.Name, project.DefaultBranch, "external status check fixture")
	return statusCheckFixture{project: project, mr: mr, commit: commit}
}

// mergeStatusCheckStatus returns the status a merge request listing reports
// for one check, and whether the check is listed at all.
func mergeStatusCheckStatus(items []externalstatuschecks.MergeStatusCheckOutput, checkID int64) (string, bool) {
	for _, item := range items {
		if item.ID == checkID {
			return item.Status, true
		}
	}
	return "", false
}

// TestExternalStatusChecks_Lifecycle_CreatesSetsRetriesAndDeletes walks one
// check per surface through its whole life on the shared merge request.
//
// Replaces: TestMeta_ExternalStatusChecks
func TestExternalStatusChecks_Lifecycle_CreatesSetsRetriesAndDeletes(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.Tier(edition.Ultimate)))

	harness.SurfacesWith(e, buildStatusCheckFixture, func(e *harness.Env, surface harness.Surface, f statusCheckFixture) {
		s := e.On(surface)
		project := f.project.IDParam()

		// The project is this scenario's own, so the deprecated listing has
		// nothing to show before the create; an item here is a decoder or an
		// endpoint answering for somebody else.
		deprecated := harness.Do[externalstatuschecks.ListProjectStatusCheckOutput](s, actionStatusCheckListProjectChecks, map[string]any{"project_id": project})
		if len(deprecated.Items) != 0 {
			e.T.Errorf("a fresh project lists %d external status check(s) before the create: %+v", len(deprecated.Items), deprecated.Items)
		}

		name := e.Name("check")
		created := harness.Do[externalstatuschecks.ProjectStatusCheckOutput](s, actionStatusCheckCreateProject, map[string]any{
			"project_id": project, "name": name, "external_url": "https://example.com/e2e/" + name,
		})
		if created.ID == 0 || created.Name != name {
			e.T.Fatalf("create answered %+v, want a check named %q with an ID", created, name)
		}

		listed := harness.Do[externalstatuschecks.ListProjectStatusCheckOutput](s, actionStatusCheckListProject, map[string]any{"project_id": project})
		ids := make([]int64, 0, len(listed.Items))
		for _, item := range listed.Items {
			ids = append(ids, item.ID)
		}
		if !containsID(ids, created.ID) {
			e.T.Errorf("the project listing does not hold the created check %d: %v", created.ID, ids)
		}

		updated := harness.Do[externalstatuschecks.ProjectStatusCheckOutput](s, actionStatusCheckUpdateProject, map[string]any{
			"project_id": project, "check_id": created.ID, "name": name + "-updated", "external_url": "https://example.com/e2e/" + name + "-updated",
		})
		if updated.ID != created.ID || updated.Name != name+"-updated" {
			e.T.Errorf("update answered %+v, want check %d renamed to %q", updated, created.ID, name+"-updated")
		}

		onRequest := harness.Do[externalstatuschecks.ListMergeStatusCheckOutput](s, actionStatusCheckListProjectMRChecks, map[string]any{
			"project_id": project, "merge_request_iid": f.mr.IID, "page": 1, "per_page": 10,
		})
		if _, listedOnRequest := mergeStatusCheckStatus(onRequest.Items, created.ID); !listedOnRequest {
			e.T.Errorf("merge request !%d does not list check %d: %+v", f.mr.IID, created.ID, onRequest.Items)
		}

		harness.DoVoid(s, actionStatusCheckSetProjectMRStatus, map[string]any{
			"project_id": project, "merge_request_iid": f.mr.IID, "sha": f.commit.SHA,
			"external_status_check_id": created.ID, "status": "passed",
		})
		afterSet := harness.Do[externalstatuschecks.ListMergeStatusCheckOutput](s, actionStatusCheckListProjectMRChecks, map[string]any{
			"project_id": project, "merge_request_iid": f.mr.IID,
		})
		if status, listedOnRequest := mergeStatusCheckStatus(afterSet.Items, created.ID); !listedOnRequest || status != "passed" {
			e.T.Errorf("check %d on merge request !%d reads %q (listed=%t) after being set to passed", created.ID, f.mr.IID, status, listedOnRequest)
		}

		// A check that passed cannot be retried: only a failed one can.
		refused := harness.ExpectToolError(s, actionStatusCheckRetryProject, map[string]any{
			"project_id": project, "merge_request_iid": f.mr.IID, "check_id": created.ID,
		}, "failed")
		assertMentions(e, "the retry of a passed check", refused, "gitlab_list_project_mr_external_status_checks")

		harness.DoVoid(s, actionStatusCheckDeleteProject, map[string]any{"project_id": project, "check_id": created.ID})
		remaining := harness.Do[externalstatuschecks.ListProjectStatusCheckOutput](s, actionStatusCheckListProject, map[string]any{"project_id": project})
		for _, item := range remaining.Items {
			if item.ID == created.ID {
				e.T.Errorf("check %d is still listed after its delete", created.ID)
			}
		}
	})
}
