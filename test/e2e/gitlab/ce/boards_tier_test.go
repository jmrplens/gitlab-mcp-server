// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

// boards_tier_test.go covers a param the tier prunes from a served action's
// schema on an unlicensed instance (issue 1239): a board update scoping the
// board to an assignee, which GitLab serves on Premium and dropped in silence
// here while the server reported the board scoped.

package ce

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/boards"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The board actions the scenario drives.
const (
	actionProjectBoardCreate harness.ActionID = "project.board_create"
	actionProjectBoardUpdate harness.ActionID = "project.board_update"
	actionProjectBoardGet    harness.ActionID = "project.board_get"
)

// TestProjectBoardUpdate_AssigneeScopeOnTheFreeTier_IsRefusedWithItsTier
// creates a board on every surface, asks to scope it to an assignee, which the
// Free schema leaves out, and holds the answer to the tier it needs, said
// before anything is sent, and the board to being left unscoped. The meta
// surface used to send the param, which GitLab dropped in silence, and answer
// with a success card; the other two refused it as a param they did not know.
func TestProjectBoardUpdate_AssigneeScopeOnTheFreeTier_IsRefusedWithItsTier(t *testing.T) {
	e := harness.New(t)
	build := func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("boards-tier"))
	}
	harness.SurfacesWith(e, build, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		created := harness.Do[boards.BoardOutput](s, actionProjectBoardCreate, map[string]any{"project_id": project.IDParam(), "name": e.Name("board")})
		board := map[string]any{"project_id": project.IDParam(), "board_id": created.ID}

		scoped := map[string]any{"project_id": project.IDParam(), "board_id": created.ID, "assignee_id": 1}
		refusal := harness.ExpectToolError(s, actionProjectBoardUpdate, scoped, "needs GitLab Premium or Ultimate")
		for _, want := range []string{`"assignee_id"`, "nothing was sent", "GITLAB_MCP_TIER"} {
			e.T.Run(want, func(t *testing.T) {
				if !strings.Contains(refusal, want) {
					t.Errorf("the refusal does not say %q: %s", want, refusal)
				}
			})
		}
		if got := harness.Do[boards.BoardOutput](s, actionProjectBoardGet, board); got.Assignee != nil {
			e.T.Errorf("board %d is scoped to assignee %d after the refused update, want it unscoped", created.ID, got.Assignee.ID)
		}
	})
}
