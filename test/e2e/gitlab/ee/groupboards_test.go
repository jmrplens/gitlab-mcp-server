//go:build e2e

// groupboards_test.go covers the two Premium actions of the group board
// family, the create and the delete, and the two Free reads that prove what
// they did: the listing holds the board the create made and the read answers
// it, and after the delete the listing no longer holds it. It also covers
// what only a licensed instance sends on a group board, the assignee and
// weight it is scoped to and the metric each limited column is counted by,
// which every answer carrying a board reads off the captured response (issue
// 1283), and the text of the board listing, which names each limited column
// with its ceilings and its metric (issue 1284). The column actions, and the
// reads and the rename of a board the fixture library built, are Free and
// live in the common package.

package ee

import (
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupboards"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// groupBoardWeightScope is the weight the scenario scopes its board to. It is
// not 0, so a reader that decoded the null of an unscoped board as the number
// 0 cannot pass by accident.
const groupBoardWeightScope = int64(5)

// groupBoardIDs lists the ids of a board listing.
func groupBoardIDs(boards []groupboards.GroupBoardOutput) []int64 {
	ids := make([]int64, 0, len(boards))
	for _, board := range boards {
		ids = append(ids, board.ID)
	}
	return ids
}

// TestGroupBoards_Create_ListsReadsAndDeletesTheBoard creates a group issue
// board on every surface, in a group of the surface's own since the create
// is what a second board on one group needs the license for, checks the
// listing holds exactly it, reads it back and deletes it.
//
// Replaces: TestEE_MetaGroupEnterpriseOperations
func TestGroupBoards_Create_ListsReadsAndDeletesTheBoard(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("boards"))
		params := map[string]any{"group_id": group.IDParam()}
		name := e.Name("board")

		created := harness.Do[groupboards.GroupBoardOutput](s, actionGroupBoardCreate, withParams(params, map[string]any{"name": name}))
		if created.ID == 0 || created.Name != name {
			e.T.Fatalf("group_board_create answered %+v, want a board named %q with an ID", created, name)
		}
		board := withParams(params, map[string]any{"board_id": created.ID})

		// The listing is asserted exactly: the group is fresh, so the board
		// just created is the only one it can hold. The old suite's Enterprise
		// copy of this step accepted an empty listing on the theory that the
		// list might lag the create, and that hedge is what let a listing that
		// answered nothing pass.
		listed := harness.Do[groupboards.ListGroupBoardsOutput](s, actionGroupBoardList, params)
		if ids := groupBoardIDs(listed.Boards); len(ids) != 1 || ids[0] != created.ID {
			e.T.Errorf("the fresh group lists the boards %v, want exactly the created %d", ids, created.ID)
		}

		got := harness.Do[groupboards.GroupBoardOutput](s, actionGroupBoardGet, board)
		if got.ID != created.ID || got.Name != name {
			e.T.Errorf("group_board_get answered board %d %q, want %d %q", got.ID, got.Name, created.ID, name)
		}

		harness.DoVoid(s, actionGroupBoardDelete, board)
		after := harness.Do[groupboards.ListGroupBoardsOutput](s, actionGroupBoardList, params)
		if containsID(groupBoardIDs(after.Boards), created.ID) {
			e.T.Errorf("the group still lists board %d after its delete", created.ID)
		}
	})
}

// groupBoardKeys is what the scenario below holds a board answer to: the
// keys client-go's GroupIssueBoard does not model, and the metric of each
// column it limited.
type groupBoardKeys struct {
	hideBacklog, hideClosed bool
	assigneeID              int64
	weight                  *int64
	metrics                 map[int64]string
}

// TestGroupBoards_ScopedBoardWithLimitedColumns_EveryBoardAnswerCarriesTheCapturedKeys
// creates a group board on every surface, gives it two label columns limited
// through GitLab's own mutation, one counted by weight and one by issues
// (the REST update of a column takes its position alone), scopes the board to
// the caller and to a weight and hides its Open list while leaving its Closed
// one shown, and holds each answer that carries the board to all of it: the
// create, the update, the read and the listing. client-go's GroupIssueBoard
// models none of the two switches, the assignee and the weight, and its
// BoardList no metric, so every one of them reaches the answer only from the
// captured response (issue 1283). The create can hold only what a new board
// has, both lists shown and no scope; the two switches differ everywhere
// after it, so an answer that read one into the other is caught. The
// listing's text is held as well, to a Column Limits row for each of the two
// columns, since until issue 1284 it said only how many columns the board
// has. Scoped boards and work-in-progress limits are licensed, which is why
// the scenario lives here.
func TestGroupBoards_ScopedBoardWithLimitedColumns_EveryBoardAnswerCarriesTheCapturedKeys(t *testing.T) {
	e := harness.New(t)

	me, _, err := e.Client().GL().Users.CurrentUser(gl.WithContext(e.Ctx))
	if err != nil {
		e.T.Fatalf("reading the token's user: %v", err)
	}

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("boardscope"))
		params := map[string]any{"group_id": group.IDParam()}

		created := harness.Do[groupboards.GroupBoardOutput](s, actionGroupBoardCreate, withParams(params, map[string]any{"name": e.Name("board")}))
		if created.ID == 0 {
			e.T.Fatalf("group_board_create answered %+v, want a board with an ID", created)
		}
		assertGroupBoardKeys(e, "group_board_create", created, groupBoardKeys{})
		board := withParams(params, map[string]any{"board_id": created.ID})

		weighted := limitedGroupColumn(e, s, group, board, "weighted", weightMetric, weightCeiling)
		counted := limitedGroupColumn(e, s, group, board, "counted", countMetric, countCeiling)
		want := groupBoardKeys{
			hideBacklog: true,
			assigneeID:  me.ID,
			weight:      new(groupBoardWeightScope),
			metrics:     map[int64]string{weighted: weightMetric, counted: countMetric},
		}

		scoped := harness.Do[groupboards.GroupBoardOutput](s, actionGroupBoardUpdate, withParams(board, map[string]any{
			"hide_backlog_list": true, "hide_closed_list": false, "assignee_id": me.ID, "weight": groupBoardWeightScope,
		}))
		assertGroupBoardKeys(e, "group_board_update", scoped, want)

		read := harness.Do[groupboards.GroupBoardOutput](s, actionGroupBoardGet, board)
		assertGroupBoardKeys(e, "group_board_get", read, want)

		listed, said := harness.DoAnswer[groupboards.ListGroupBoardsOutput](s, actionGroupBoardList, params)
		assertColumnLimitRows(e, "group_board_list", said, created.ID, want.metrics)
		for _, candidate := range listed.Boards {
			if candidate.ID == created.ID {
				assertGroupBoardKeys(e, "group_board_list", candidate, want)
				return
			}
		}
		e.T.Errorf("group_board_list answered the boards %v, want the created %d among them", groupBoardIDs(listed.Boards), created.ID)
	})
}

// limitedGroupColumn adds a label column to the group board through the tool
// and limits it to ceiling, counted by metric, returning the column's ID.
func limitedGroupColumn(e *harness.Env, s *harness.Session, group fixture.Group, board map[string]any, prefix, metric string, ceiling int64) int64 {
	e.T.Helper()
	label := fixture.NewGroupLabel(e, group, prefix)
	column := harness.Do[groupboards.BoardListOutput](s, actionGroupBoardCreateList, withParams(board, map[string]any{"label_id": label.ID}))
	if column.ID == 0 {
		e.T.Fatalf("group_board_create_list answered %+v for the label %q, want a column with an ID", column, label.Name)
	}
	fixture.LimitBoardList(e, column.ID, metric, ceiling)
	return column.ID
}

// assertGroupBoardKeys checks a board answer against want: the two list
// switches, the assignee and weight scope, absent where want has none, and
// the metric of each column want names, which has to be among the columns the
// answer carried.
func assertGroupBoardKeys(e *harness.Env, action string, got groupboards.GroupBoardOutput, want groupBoardKeys) {
	e.T.Helper()
	if got.HideBacklogList != want.hideBacklog || got.HideClosedList != want.hideClosed {
		e.T.Errorf("%s answered hide_backlog_list %t and hide_closed_list %t, want %t and %t",
			action, got.HideBacklogList, got.HideClosedList, want.hideBacklog, want.hideClosed)
	}
	switch {
	case want.assigneeID == 0 && got.Assignee != nil:
		e.T.Errorf("%s answered the assignee scope %+v, want none", action, got.Assignee)
	case want.assigneeID != 0 && (got.Assignee == nil || got.Assignee.ID != want.assigneeID):
		e.T.Errorf("%s answered the assignee scope %+v, want user %d", action, got.Assignee, want.assigneeID)
	}
	switch {
	case want.weight == nil && got.Weight != nil:
		e.T.Errorf("%s answered the weight scope %d, want none", action, *got.Weight)
	case want.weight != nil && (got.Weight == nil || *got.Weight != *want.weight):
		e.T.Errorf("%s answered the weight scope %v, want %d", action, got.Weight, *want.weight)
	}
	columns := make(map[int64]string, len(got.Lists))
	for _, column := range got.Lists {
		columns[column.ID] = column.LimitMetric
	}
	for id, metric := range want.metrics {
		published, carried := columns[id]
		switch {
		case !carried:
			e.T.Errorf("%s answered no column %d", action, id)
		case published != metric:
			e.T.Errorf("%s answered column %d counted by %q, want %q", action, id, published, metric)
		}
	}
}
