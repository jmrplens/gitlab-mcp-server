//go:build e2e

// groupboards_test.go covers the Free half of the group board family: the
// reads and the rename of a board, and the label columns on one. The board
// itself comes from the fixture library, which makes it the way the boards
// page does, since every group may have one board on every edition and
// only a second one is Premium; the Premium create and delete live in the
// ee package.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupboards"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// firstPosition is where the reorder puts the second column: the front.
const firstPosition = int64(0)

// groupBoardIDs lists the ids of a board listing.
func groupBoardIDs(boards []groupboards.GroupBoardOutput) []int64 {
	ids := make([]int64, 0, len(boards))
	for _, board := range boards {
		ids = append(ids, board.ID)
	}
	return ids
}

// boardListIDs lists the ids of a column listing.
func boardListIDs(lists []groupboards.BoardListOutput) []int64 {
	ids := make([]int64, 0, len(lists))
	for _, list := range lists {
		ids = append(ids, list.ID)
	}
	return ids
}

// boardFixture is a group with the one board the fixture library made in
// it and two labels for its columns.
type boardFixture struct {
	group  fixture.Group
	board  fixture.GroupBoard
	first  fixture.GroupLabel
	second fixture.GroupLabel
}

// buildBoardFixture creates the group, the board and the labels.
func buildBoardFixture(e *harness.Env) boardFixture {
	group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("board"))
	return boardFixture{
		group:  group,
		board:  fixture.NewGroupBoard(e, group, "board"),
		first:  fixture.NewGroupLabel(e, group, "first"),
		second: fixture.NewGroupLabel(e, group, "second"),
	}
}

// TestGroupBoards_FixtureBoard_ListGetAndRename lists the boards of a
// group holding one, reads it back and renames it, on every surface
// against a board of the surface's own.
//
// Replaces: TestEE_MetaGroupEnterpriseOperations
func TestGroupBoards_FixtureBoard_ListGetAndRename(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("boardread"))
		board := fixture.NewGroupBoard(e, group, "board")
		params := map[string]any{"group_id": group.IDParam()}
		byID := withParams(params, map[string]any{"board_id": board.ID})

		listed := harness.Do[groupboards.ListGroupBoardsOutput](s, actionGroupBoardList, params)
		if ids := groupBoardIDs(listed.Boards); len(ids) != 1 || ids[0] != board.ID {
			e.T.Errorf("the group lists the boards %v, want exactly the fixture's %d", ids, board.ID)
		}
		got := harness.Do[groupboards.GroupBoardOutput](s, actionGroupBoardGet, byID)
		if got.ID != board.ID || got.Name != board.Name {
			e.T.Errorf("group_board_get answered board %d %q, want %d %q", got.ID, got.Name, board.ID, board.Name)
		}

		renamed := harness.Do[groupboards.GroupBoardOutput](s, actionGroupBoardUpdate, withParams(byID, map[string]any{"name": "Renamed " + board.Name}))
		if renamed.ID != board.ID || renamed.Name != "Renamed "+board.Name {
			e.T.Errorf("group_board_update answered board %d %q, want %d renamed", renamed.ID, renamed.Name, board.ID)
		}
	})
}

// TestGroupBoardLists_LabelColumns_CreateGetReorderDelete adds two label
// columns to the fixture board per surface, reads the first back, moves
// the second to the front, deletes the first and checks the column listing
// lets it go.
//
// Replaces: TestMeta_GroupBoardListColumns
func TestGroupBoardLists_LabelColumns_CreateGetReorderDelete(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		f := buildBoardFixture(e)
		board := map[string]any{"group_id": f.group.IDParam(), "board_id": f.board.ID}

		first := harness.Do[groupboards.BoardListOutput](s, actionGroupBoardCreateList, withParams(board, map[string]any{"label_id": f.first.ID}))
		if first.ID == 0 {
			e.T.Fatalf("group_board_create_list answered %+v, want a column with an ID", first)
		}
		// A second column is what makes the reorder mean something: GitLab
		// refuses to move a board's only column, having nothing to move it
		// against.
		second := harness.Do[groupboards.BoardListOutput](s, actionGroupBoardCreateList, withParams(board, map[string]any{"label_id": f.second.ID}))
		if second.ID == 0 {
			e.T.Fatalf("group_board_create_list answered %+v for the second column, want one with an ID", second)
		}

		got := harness.Do[groupboards.BoardListOutput](s, actionGroupBoardGetList, withParams(board, map[string]any{"list_id": first.ID}))
		if got.ID != first.ID {
			e.T.Errorf("group_board_get_list answered column %d, want %d", got.ID, first.ID)
		}
		columns := harness.Do[groupboards.ListBoardListsOutput](s, actionGroupBoardListLists, board)
		if ids := boardListIDs(columns.Lists); !containsID(ids, first.ID) || !containsID(ids, second.ID) {
			e.T.Errorf("the board lists the columns %v, want the two just created, %d and %d", ids, first.ID, second.ID)
		}

		moved := harness.Do[groupboards.BoardListOutput](s, actionGroupBoardUpdateList, withParams(board, map[string]any{"list_id": second.ID, "position": firstPosition}))
		if moved.ID != second.ID || moved.Position != firstPosition {
			e.T.Errorf("group_board_update_list answered column %d at %d, want %d at the front", moved.ID, moved.Position, second.ID)
		}

		harness.DoVoid(s, actionGroupBoardDeleteList, withParams(board, map[string]any{"list_id": first.ID}))
		after := harness.Do[groupboards.ListBoardListsOutput](s, actionGroupBoardListLists, board)
		if containsID(boardListIDs(after.Lists), first.ID) {
			e.T.Errorf("the board still lists column %d after its delete", first.ID)
		}
	})
}
