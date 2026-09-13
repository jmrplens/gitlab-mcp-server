//go:build e2e

// mergerequests_test.go covers the three writes that open a merge request
// through the server: a branch created from the default one, a commit made
// on it, and the merge request from it. The old suite made these calls to
// build the merge request its Enterprise scenarios stood on, and never as a
// scenario of their own.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/branches"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestMergeRequest_Create_FromABranchWithACommit creates a branch on every
// surface in a shared project, commits a file on it and opens a merge
// request from it, reading each object back off the answer that made it.
//
// Replaces: TestMeta_MergeTrains, TestMeta_ExternalStatusChecks, TestIndividual_MRDependenciesList
func TestMergeRequest_Create_FromABranchWithACommit(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("mrcreate"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_id": project.IDParam()}
		branch := "feature/" + e.Name("mr")

		created := harness.Do[branches.Output](s, actionBranchCreate, withParams(params, map[string]any{"branch_name": branch, "ref": project.DefaultBranch}))
		if created.Name != branch {
			e.T.Fatalf("branch create answered %+v, want the branch %q", created, branch)
		}

		commit := harness.Do[commits.Output](s, actionRepositoryCommitCreate, withParams(params, map[string]any{
			"branch": branch, "commit_message": "docs: add the merge request file",
			"actions": []map[string]any{{"action": "create", "file_path": "docs/" + string(surface) + ".md", "content": "# " + string(surface) + "\n"}},
		}))
		if commit.ID == "" || commit.Title != "docs: add the merge request file" {
			e.T.Fatalf("commit create answered %+v, want a commit with an ID carrying the message", commit)
		}

		opened := harness.Do[mergerequests.Output](s, actionMergeRequestCreate, withParams(params, map[string]any{
			"source_branch": branch, "target_branch": project.DefaultBranch, "title": e.Name("mr"),
		}))
		if opened.IID == 0 || opened.SourceBranch != branch || opened.TargetBranch != project.DefaultBranch || opened.State != "opened" {
			e.T.Errorf("merge request create answered %+v, want an opened merge request from %q into %q", opened, branch, project.DefaultBranch)
		}
	})
}
