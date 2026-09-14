//go:build e2e

// boards_test.go covers a project issue board through its life and the
// label columns on it: the second column is what makes the reorder mean
// something, since GitLab refuses to move a board's only column, and that
// refusal is asserted too.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/boards"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// projectBoardIDs lists the ids of a board listing.
func projectBoardIDs(listed []boards.BoardOutput) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, board := range listed {
		ids = append(ids, board.ID)
	}
	return ids
}

// projectBoardListIDs lists the ids of a column listing.
func projectBoardListIDs(lists []boards.BoardListOutput) []int64 {
	ids := make([]int64, 0, len(lists))
	for _, list := range lists {
		ids = append(ids, list.ID)
	}
	return ids
}

// TestProjectBoards_Lifecycle_BoardAndLabelColumns creates a board in a
// project of each surface's own, reads it by id and in the listing,
// renames it, adds a label column, reads it, shows the reorder of the only
// column refused, adds a second column and moves it to the front, deletes
// the first column, and deletes the board.
//
// Replaces: TestMeta_Boards, TestMeta_ProjectBoardsDeep
func TestProjectBoards_Lifecycle_BoardAndLabelColumns(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		project := fixture.NewProject(e, fixture.WithNamePrefix("boards"))
		params := map[string]any{"project_id": project.IDParam()}
		name := e.Name("board")

		created := harness.Do[boards.BoardOutput](s, actionProjectBoardCreate, withParams(params, map[string]any{"name": name}))
		if created.ID == 0 || created.Name != name {
			e.T.Fatalf("board_create answered %+v, want the board %q with an ID", created, name)
		}
		board := withParams(params, map[string]any{"board_id": created.ID})

		listed := harness.Do[boards.ListBoardsOutput](s, actionProjectBoardList, params)
		if !containsID(projectBoardIDs(listed.Boards), created.ID) {
			e.T.Errorf("the project lists the boards %v, want board %d among them", projectBoardIDs(listed.Boards), created.ID)
		}
		got := harness.Do[boards.BoardOutput](s, actionProjectBoardGet, board)
		if got.ID != created.ID || got.Name != name {
			e.T.Errorf("board_get answered board %d %q, want %d %q", got.ID, got.Name, created.ID, name)
		}
		renamed := harness.Do[boards.BoardOutput](s, actionProjectBoardUpdate, withParams(board, map[string]any{"name": "Renamed " + name}))
		if renamed.ID != created.ID || renamed.Name != "Renamed "+name {
			e.T.Errorf("board_update answered board %d %q, want %d renamed", renamed.ID, renamed.Name, created.ID)
		}

		assertBoardColumnsRoundTrip(e, s, project, board)

		harness.DoVoid(s, actionProjectBoardDelete, board)
		remaining := harness.Do[boards.ListBoardsOutput](s, actionProjectBoardList, params)
		if containsID(projectBoardIDs(remaining.Boards), created.ID) {
			e.T.Errorf("the project still lists board %d after its delete", created.ID)
		}
	})
}

// assertBoardColumnsRoundTrip drives the label columns of one board: one
// column, its read, the refused reorder of a lone column, a second column
// moved to the front, and the delete of the first.
func assertBoardColumnsRoundTrip(e *harness.Env, s *harness.Session, project fixture.Project, board map[string]any) {
	e.T.Helper()
	first := fixture.NewLabel(e, project, "first")
	second := fixture.NewLabel(e, project, "second")

	firstColumn := harness.Do[boards.BoardListOutput](s, actionProjectBoardListCreate, withParams(board, map[string]any{"label_id": first.ID}))
	if firstColumn.ID == 0 || firstColumn.Label == nil || firstColumn.Label.Name != first.Name {
		e.T.Fatalf("board_list_create answered %+v, want a column for the label %q with an ID", firstColumn, first.Name)
	}
	got := harness.Do[boards.BoardListOutput](s, actionProjectBoardListGet, withParams(board, map[string]any{"list_id": firstColumn.ID}))
	if got.ID != firstColumn.ID {
		e.T.Errorf("board_list_get answered column %d, want %d", got.ID, firstColumn.ID)
	}
	columns := harness.Do[boards.ListBoardListsOutput](s, actionProjectBoardListList, board)
	if !containsID(projectBoardListIDs(columns.Lists), firstColumn.ID) {
		e.T.Errorf("the board lists the columns %v, want column %d among them", projectBoardListIDs(columns.Lists), firstColumn.ID)
	}

	refused := harness.ExpectToolError(s, actionProjectBoardListUpdate, withParams(board, map[string]any{"list_id": firstColumn.ID, "position": firstPosition}), "could not be moved")
	e.T.Logf("the reorder of a board's only column is refused: %s", firstLine(refused))

	secondColumn := harness.Do[boards.BoardListOutput](s, actionProjectBoardListCreate, withParams(board, map[string]any{"label_id": second.ID}))
	if secondColumn.ID == 0 {
		e.T.Fatalf("board_list_create answered %+v for the second column, want one with an ID", secondColumn)
	}
	moved := harness.Do[boards.BoardListOutput](s, actionProjectBoardListUpdate, withParams(board, map[string]any{"list_id": secondColumn.ID, "position": firstPosition}))
	if moved.ID != secondColumn.ID || moved.Position != firstPosition {
		e.T.Errorf("board_list_update answered column %d at %d, want %d at the front", moved.ID, moved.Position, secondColumn.ID)
	}

	harness.DoVoid(s, actionProjectBoardListDelete, withParams(board, map[string]any{"list_id": firstColumn.ID}))
	after := harness.Do[boards.ListBoardListsOutput](s, actionProjectBoardListList, board)
	if containsID(projectBoardListIDs(after.Lists), firstColumn.ID) {
		e.T.Errorf("the board still lists column %d after its delete", firstColumn.ID)
	}
}
