//go:build e2e

// snippetstoragemoves_test.go covers a snippet's repository storage moves,
// the snippet half of the Free storage move family: the listings of a real
// snippet answer, and everything asked about a snippet or a move that does
// not exist is a not-found.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/snippetstoragemoves"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// missingSnippetID is a snippet nothing on a fresh instance has.
const missingSnippetID = int64(999999)

// TestSnippetStorageMoves_SingleStorage_ListsAndRefusesTheMissing lists
// the moves of a snippet of the surface's own and of every snippet, then
// reads and schedules against a snippet and a move that do not exist.
//
// Replaces: TestMeta_StorageMoves, TestMeta_SnippetStorageMoves_Graceful404
func TestSnippetStorageMoves_SingleStorage_ListsAndRefusesTheMissing(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		snippet := fixture.NewSnippet(e)

		mine := harness.Do[snippetstoragemoves.ListOutput](s, actionStorageMoveRetrieveSnippet, map[string]any{"snippet_id": snippet.ID})
		if len(mine.Moves) != 0 {
			e.T.Errorf("a fresh snippet lists %d storage moves: %+v", len(mine.Moves), mine.Moves)
		}
		all := harness.Do[snippetstoragemoves.ListOutput](s, actionStorageMoveRetrieveAllSnippet, nil)
		e.T.Logf("%d snippet storage move(s) on the instance", len(all.Moves))

		refused := harness.Refused(s, actionStorageMoveRetrieveSnippet, map[string]any{"snippet_id": missingSnippetID}, harness.FailureNotFound)
		e.T.Logf("the moves of a missing snippet are refused: %s", firstLine(refused))
		refused = harness.Refused(s, actionStorageMoveGetSnippet, map[string]any{"id": missingMoveID}, harness.FailureNotFound)
		e.T.Logf("the read of a missing move is refused: %s", firstLine(refused))
		refused = harness.Refused(s, actionStorageMoveGetSnippetForSnippet, map[string]any{"snippet_id": snippet.ID, "id": missingMoveID}, harness.FailureNotFound)
		e.T.Logf("the read of a missing move of the snippet is refused: %s", firstLine(refused))
		refused = harness.Refused(s, actionStorageMoveScheduleSnippet, map[string]any{"snippet_id": missingSnippetID}, harness.FailureNotFound)
		e.T.Logf("a move of a missing snippet is refused: %s", firstLine(refused))

		refused = harness.ExpectToolError(s, actionStorageMoveScheduleAllSnippet, map[string]any{
			"source_storage_name": missingStorage + "-source", "destination_storage_name": missingStorage,
		}, "storage")
		e.T.Logf("a move of every snippet between storages the instance lacks is refused: %s", firstLine(refused))
	})
}
