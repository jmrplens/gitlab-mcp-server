//go:build e2e

// b7_issue_moves_test.go covers the two writes that change where an issue
// sits: the move to another project, and the reorder within its own list.
// Neither was reached by the rebuilt suite.
//
// The two are asserted differently on purpose, and the difference is what
// the old CE suite did with each. The move is a happy path it asserted. The
// reorder it only ever reached on an error path, and that is what is
// asserted here: a reorder that names neither neighbor is one GitLab
// refuses, and the refusal is worth a test because the message is what tells
// a model which parameter it left out.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// issueMoveFixture is the project an issue is moved out of and the one it is
// moved into. Both are shared by the surfaces; the issue each surface moves
// is its own, since a move leaves the source project without it.
type issueMoveFixture struct {
	source fixture.Project
	target fixture.Project
}

// TestIssueMove_ToAnotherProject_AnswersTheIssueInTheTarget creates an issue
// of its own on every surface in the source project and moves it into the
// target project, reading the new home off the answer.
func TestIssueMove_ToAnotherProject_AnswersTheIssueInTheTarget(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) issueMoveFixture {
		return issueMoveFixture{
			source: fixture.NewProject(e, fixture.WithNamePrefix("issuemovefrom")),
			target: fixture.NewProject(e, fixture.WithNamePrefix("issuemoveto")),
		}
	}, func(e *harness.Env, surface harness.Surface, f issueMoveFixture) {
		s := e.On(surface)
		title := "move fixture for the " + string(surface) + " surface"
		issue := fixture.NewIssue(e, f.source, title)

		moved := harness.Do[issues.Output](s, actionIssueMove, map[string]any{
			"project_id": f.source.IDParam(), "issue_iid": issue.IID, "to_project_id": f.target.ID,
		})
		if moved.ProjectID != f.target.ID {
			e.T.Errorf("move answered an issue of project %d, want it in project %d: %+v", moved.ProjectID, f.target.ID, moved)
		}
		if moved.IID == 0 || moved.Title != title {
			e.T.Errorf("move answered %+v, want the issue titled %q with an iid of its own in the target", moved, title)
		}
	})
}

// TestIssueReorder_WithoutANeighbour_IsRefusedNamingBothParameters asks for a
// reorder that names neither the issue to sit after nor the one to sit
// before, on every surface, and holds GitLab's refusal to a message that
// names both parameters and the project they have to share.
//
// The happy path is not driven here: the old CE suite never reached it
// either, and a reorder that works needs a second issue positioned relative
// to the first, which is state this scenario would have to build only to
// leave the refusal untested.
func TestIssueReorder_WithoutANeighbor_IsRefusedNamingBothParameters(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) issueFixture {
		return newIssueFixture(e, "issuereorder")
	}, func(e *harness.Env, surface harness.Surface, f issueFixture) {
		s := e.On(surface)

		refused := harness.ExpectToolError(s, actionIssueReorder, f.params(), "move_after_id")
		assertMentions(e, "the reorder refusal", refused, "move_before_id", "same project")
		e.T.Logf("the reorder naming no neighbor was refused: %s", firstLine(refused))
	})
}
