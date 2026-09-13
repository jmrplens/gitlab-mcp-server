//go:build e2e

// b7_commit_create_error_test.go covers what a commit create answers when
// GitLab refuses the change it carries.
//
// The happy path is driven in mergerequests_test.go and commits_test.go, and
// a successful call says nothing about the refusal: a create over a path the
// branch already holds is answered by GitLab with a 400 rather than a 404 or
// a 403, so it is a plain tool error, and what the handler adds to it is the
// hint naming the three things that make a commit action valid. The old CE
// suite reached this error path on the individual surface by accident, while
// building a catalog fixture; here it is the subject.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// readmeFilePath is the file every fixture project is initialized with, and
// so the one path a create is certain to collide with.
const readmeFilePath = "README.md"

// TestCommitCreate_PathAlreadyInTheBranch_RefusedWithTheCommitActionHint asks
// a commit create to create the file the project was initialized with.
//
// It runs on the dynamic, meta and individual surfaces against one project
// built for all three, since a refused commit writes nothing and leaves the
// repository as it found it. It asserts that the call comes back as a tool
// error rather than a refusal class of its own, that the error carries
// GitLab's own words for the collision, and that the handler's hint names
// what a caller has to fix.
func TestCommitCreate_PathAlreadyInTheBranch_RefusedWithTheCommitActionHint(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("commiterr"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)

		refused := harness.ExpectToolError(s, actionRepositoryCommitCreate, map[string]any{
			"project_id":     project.IDParam(),
			"branch":         project.DefaultBranch,
			"commit_message": "docs: create a file the branch already holds",
			"actions": []map[string]any{{
				"action": "create", "file_path": readmeFilePath, "content": "# this never lands\n",
			}},
		}, "already exists")
		assertMentions(e, "the refusal of a create over a path the branch holds", refused, "file paths are valid")
	})
}
