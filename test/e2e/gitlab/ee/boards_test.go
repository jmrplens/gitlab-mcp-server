//go:build e2e

// boards_test.go covers what the project board answers say about a column
// whose work-in-progress limit is set, which only a licensed instance has:
// every answer carrying a column carries the metric that column is counted
// by, limit_metric, the one key of a column client-go's BoardList does not
// model (issue 1256). The board and column lifecycle is Free and lives in the
// common package.

package ee

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/boards"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// frontPosition is the position a column is moved to, the first.
const frontPosition = int64(0)

// The two limits the scenario sets, one per metric GitLab's board settings
// panel offers, so an answer that published one column's metric on the other
// is caught: a total weight of 4 on the column counted by weight, held in
// max_issue_weight, and 3 issues on the column counted by issues, held in
// max_issue_count.
const (
	weightMetric  = "issue_weights"
	weightCeiling = int64(4)
	countMetric   = "issue_count"
	countCeiling  = int64(3)
)

// TestProjectBoards_LimitedColumns_EveryAnswerCarriesTheMetric creates a
// board with two label columns on every surface, limits one to a total issue
// weight and the other to a number of issues through GitLab's own mutation
// (the REST update of a column takes its position alone), and holds every
// answer that carries a column to the metric that column is counted by: the
// board read, the board listing, the board update, the column listing, the
// column read and the column update. Until issue 1256 six of the eight
// project board answers published every column without its metric: the board
// listing, create and update and the column read, create and update. The two
// create answers cannot be held here, since a new board has no column and a
// new column no limit; the unit tests hold them.
func TestProjectBoards_LimitedColumns_EveryAnswerCarriesTheMetric(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		project := fixture.NewProject(e, fixture.WithNamePrefix("boards"))
		params := map[string]any{"project_id": project.IDParam()}

		created := harness.Do[boards.BoardOutput](s, actionProjectBoardCreate, withParams(params, map[string]any{"name": e.Name("board")}))
		if created.ID == 0 {
			e.T.Fatalf("board_create answered %+v, want a board with an ID", created)
		}
		board := withParams(params, map[string]any{"board_id": created.ID})

		weighted := limitedColumn(e, s, project, board, "weighted", weightMetric, weightCeiling)
		counted := limitedColumn(e, s, project, board, "counted", countMetric, countCeiling)
		both := map[int64]string{weighted: weightMetric, counted: countMetric}

		read := harness.Do[boards.BoardOutput](s, actionProjectBoardGet, board)
		assertColumnMetrics(e, "board_get", read.Lists, both)

		listed := harness.Do[boards.ListBoardsOutput](s, actionProjectBoardList, params)
		assertColumnMetrics(e, "board_list", boardColumns(e, listed.Boards, created.ID), both)

		renamed := harness.Do[boards.BoardOutput](s, actionProjectBoardUpdate, withParams(board, map[string]any{"name": e.Name("renamed")}))
		assertColumnMetrics(e, "board_update", renamed.Lists, both)

		columns := harness.Do[boards.ListBoardListsOutput](s, actionProjectBoardListList, board)
		assertColumnMetrics(e, "board_list_list", columns.Lists, both)

		column := harness.Do[boards.BoardListOutput](s, actionProjectBoardListGet, withParams(board, map[string]any{"list_id": weighted}))
		assertColumnMetrics(e, "board_list_get", []boards.BoardListOutput{column}, map[int64]string{weighted: weightMetric})
		if column.MaxIssueWeight != weightCeiling || column.MaxIssueCount != 0 {
			e.T.Errorf("board_list_get answered column %d with a weight ceiling of %d and an issue ceiling of %d, want %d and none",
				weighted, column.MaxIssueWeight, column.MaxIssueCount, weightCeiling)
		}

		moved := harness.Do[boards.BoardListOutput](s, actionProjectBoardListUpdate, withParams(board, map[string]any{"list_id": counted, "position": frontPosition}))
		assertColumnMetrics(e, "board_list_update", []boards.BoardListOutput{moved}, map[int64]string{counted: countMetric})
		if moved.Position != frontPosition || moved.MaxIssueCount != countCeiling || moved.MaxIssueWeight != 0 {
			e.T.Errorf("board_list_update answered column %d at %d with an issue ceiling of %d and a weight ceiling of %d, want it at %d with %d and none",
				counted, moved.Position, moved.MaxIssueCount, moved.MaxIssueWeight, frontPosition, countCeiling)
		}
	})
}

// limitedColumn adds a label column to the board through the tool and limits
// it to ceiling, counted by metric, returning the column's ID.
func limitedColumn(e *harness.Env, s *harness.Session, project fixture.Project, board map[string]any, prefix, metric string, ceiling int64) int64 {
	e.T.Helper()
	label := fixture.NewLabel(e, project, prefix)
	column := harness.Do[boards.BoardListOutput](s, actionProjectBoardListCreate, withParams(board, map[string]any{"label_id": label.ID}))
	if column.ID == 0 {
		e.T.Fatalf("board_list_create answered %+v for the label %q, want a column with an ID", column, label.Name)
	}
	fixture.LimitBoardList(e, column.ID, metric, ceiling)
	return column.ID
}

// boardColumns returns the columns of the board the listing holds under id.
func boardColumns(e *harness.Env, listed []boards.BoardOutput, id int64) []boards.BoardListOutput {
	e.T.Helper()
	for _, board := range listed {
		if board.ID == id {
			return board.Lists
		}
	}
	e.T.Errorf("the project lists no board %d", id)
	return nil
}

// assertColumnMetrics checks that each column of want is among the columns an
// answer carried and is published with the metric want gives it. Columns
// beyond want are left alone, since a board answer may carry columns the
// scenario did not limit.
func assertColumnMetrics(e *harness.Env, action string, columns []boards.BoardListOutput, want map[int64]string) {
	e.T.Helper()
	got := make(map[int64]string, len(columns))
	for _, column := range columns {
		got[column.ID] = column.LimitMetric
	}
	for id, metric := range want {
		published, carried := got[id]
		switch {
		case !carried:
			e.T.Errorf("%s answered no column %d", action, id)
		case published != metric:
			e.T.Errorf("%s answered column %d counted by %q, want %q", action, id, published, metric)
		}
	}
}
