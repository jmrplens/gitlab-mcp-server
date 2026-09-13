//go:build e2e

// groupboards_test.go covers the one Premium action of the group board
// family, the create, and the three Free reads and writes that prove what
// it made: the listing holds the board, the read answers it, the delete
// takes it out of the listing. The column actions and the reads on a board
// the fixture library built are Free and live in the common package.

package ee

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupboards"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

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
