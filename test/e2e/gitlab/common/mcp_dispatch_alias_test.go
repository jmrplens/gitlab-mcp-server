//go:build e2e

// mcp_dispatch_alias_test.go covers an action the server answers by running
// another one, which is the case the call recorder is strict about.
//
// The recorder holds every call to the route its span says ran, and fails the
// test when they differ: a call that named issue.list and ran issue.get proves
// nothing about issue.list, and catching that is most of why the record exists.
// Some rewrites are deliberate, though, and repository.file_history is one —
// it is a discovery alias whose route is the commit listing, registered so a
// model looking for a file's history finds something. A rewrite nobody declared
// is a defect; one a test declares is the behavior under test.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// actionRepositoryFileHistory is the alias: its own catalog ID, answered by the
// commit listing's route.
const actionRepositoryFileHistory harness.ActionID = "repository.file_history"

// TestDispatchAlias_FileHistory_RunsTheCommitListingRoute calls the alias and
// declares the route it really runs.
//
// Two things are asserted and the second is the one worth having: that the
// alias answers with a commit listing, and — through ExpectDispatch — that the
// route which ran is the commit listing's rather than one of its own. Without
// the declaration the recorder fails the call, so this is also the only way the
// suite can cover an alias at all.
//
// The World's shared project is used rather than a new one: the alias reads
// history, and a freshly created project has exactly the one commit its
// initialization made, which is enough and costs nothing.
func TestDispatchAlias_FileHistory_RunsTheCommitListingRoute(t *testing.T) {
	e := harness.New(t)
	world := fixture.SharedWorld(e)
	s := e.On(harness.SurfaceDynamic)

	projectID, bound := world.BindPromptArgument("project_id")
	if !bound {
		t.Skip("the World binds no project_id, so there is no repository to read history from")
	}

	listed := harness.Do[commits.ListOutput](s, actionRepositoryFileHistory,
		map[string]any{"project_id": projectID},
		harness.ExpectDispatch(actionRepositoryCommitList))

	if len(listed.Commits) == 0 {
		t.Error("the alias answered an empty history for a project that has commits")
	}
}
